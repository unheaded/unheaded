# Post-quantum host changes: E2 (strict sshd KEX) and E3 (WireGuard PSK rotation)

S-QUANTUM-ARMOR Phase 9 (ADR-105, Proposed). These are the two host changes the
sprint prepared but did not make: both touch Stevie's only access paths, so each
waits for his go-ahead. Nothing here has been run on west or east; every host
fact below came from read-only commands on 2026-10-23.

Never print a PSK or private key while following these steps: `wg show` hides
keys by default, never use `wg showconf` or `cat` on `/etc/wireguard/*`.

## State today (read-only, 2026-10-23)

| | west | east |
|---|---|---|
| OpenSSH | 10.0p2 | 10.0p2 |
| sshd `kexalgorithms` | `mlkem768x25519-sha256,sntrup761x25519-sha512,sntrup761x25519-sha512@openssh.com,curve25519-sha256,curve25519-sha256@libssh.org,ecdh-sha2-nistp256,ecdh-sha2-nistp384,ecdh-sha2-nistp521` | same |
| sshd port | 7375 | 22 |
| west's client negotiates | `mlkem768x25519-sha256` (`localhost -p 7375`) | `mlkem768x25519-sha256` |
| drop-ins setting `KexAlgorithms` | none (`50-cloud-init.conf`, `99-unheaded.conf`) | none (`00-unheaded.conf`, `50-cloud-init.conf`, `60-unheaded-well-offsite.conf`) |
| probe `sshd-kex` / `sshd-kex-pq` | pass / FAIL (classical allowed) | pass / FAIL |
| WireGuard `wg0` | up, PSK set, inline in `wg0.conf` | up, PSK set; old copy `/etc/wireguard/preshared.key` (2026-03-25) |
| PSK age | static since 2026-03-25 | same |

## E2: strict hybrid-only sshd KEX on east, then west

**Ask:** make sshd accept only post-quantum hybrid key exchange. Today a current
client gets ML-KEM, but sshd still accepts five classical methods, so an old or
downgrade-capable client gets classical KEX.

**Why it is an escalation:** a client that knows none of the hybrids can no
longer log in. SSH is the only remote path to both hosts (lockout: high).

### Before (Stevie checks every client that logs in)

Each must list `mlkem768x25519-sha256` or `sntrup761x25519-sha512[@openssh.com]`:

- OpenSSH clients on west, east and any laptop: `ssh -Q kex | grep -E 'mlkem768x25519|sntrup761x25519'` (OpenSSH >= 8.5 for sntrup, >= 9.9 for ML-KEM).
- Phone/tablet SSH apps: check each app's KEX list; many lag OpenSSH.
- Repo tooling: the compliance collector, `scripts/backup/git-mirror.sh`, the Well offsite copy (east's `60-unheaded-well-offsite.conf` Match block) and `scripts/wireguard/rotate-psk.sh` all shell out to OpenSSH 10 on west. No Go `x/crypto/ssh` client dials the hosts (`pkg/secrets/rotation` only formats keys); if one is added, `golang.org/x/crypto` v0.58.0 supports `mlkem768x25519-sha256`.
- Anything in `~/.ssh/config` or a `Match` block that sets `KexAlgorithms` on a client.

### Procedure (east first; keep a second session open the whole time)

```bash
# 0. Session A: an open root-capable session to east. Do not close it until step 6 passes.
ssh govan@east
# 1. Session B (east): back up, then write a NEW drop-in (no existing file is edited).
#    sshd uses the FIRST KexAlgorithms it reads; 00-unheaded-kex.conf sorts before
#    00-unheaded.conf and everything else in sshd_config.d.
sudo cp -a /etc/ssh/sshd_config.d "/root/sshd_config.d.bak-$(date +%Y%m%d)"
printf 'KexAlgorithms mlkem768x25519-sha256,sntrup761x25519-sha512,sntrup761x25519-sha512@openssh.com\n' | sudo tee /etc/ssh/sshd_config.d/00-unheaded-kex.conf >/dev/null
sudo chmod 600 /etc/ssh/sshd_config.d/00-unheaded-kex.conf
# 2. Validate before reloading; nothing changes if this fails.
sudo sshd -t && sudo sshd -T | grep '^kexalgorithms'
#    expect exactly the three hybrids
# 3. Reload (existing sessions survive a reload).
sudo systemctl reload ssh
# 4. Session C, NEW, from west: must log in, negotiating a hybrid.
ssh -vv -o BatchMode=yes govan@east true 2>&1 | grep 'kex: algorithm:'
# 5. The refusal is real: a classical-only client must now fail.
ssh -o BatchMode=yes -o KexAlgorithms=curve25519-sha256 govan@east true; echo "rc=$? (want 255)"
# 6. What sshd-kex-pq reads: only the three hybrids.
ssh govan@east 'sudo -n sshd -T' | grep '^kexalgorithms'
#    The next compliance collection records host-probe east:sshd-kex-pq as a pass.
```

Then repeat 0-6 on west: west's sshd listens on port 7375, so session A is a
local console or `ssh -p 7375 localhost` kept open, and steps 4-5 use
`ssh -p 7375 ... localhost`. The west reload is `sudo systemctl reload ssh` too.

### Rollback (any step fails, or a client breaks later)

```bash
sudo rm -f /etc/ssh/sshd_config.d/00-unheaded-kex.conf
sudo sshd -t && sudo systemctl reload ssh
```

From the still-open session A. If every session is gone: console on the host,
same two commands (plan Appendix A.11).

### After

The probe `sshd-kex-pq` (`pkg/compliance/crosswalk/collect/cisprobes.go`) turns
from FAIL to pass on that host; the Phase 13 finding for `host-probe:<host>:sshd-kex-pq`
closes. The repo runbook `runbooks/security/ssh-hardening.yaml` keeps the
opportunistic list (hybrid first plus `curve25519-sha256`) until Stevie decides
the strict list is the default.

## E3: first live WireGuard PSK rotation, then the weekly timer

**Ask:** replace the static PSK (unchanged since 2026-03-25) with a fresh one,
then let the weekly timer keep rotating it.

**Why it is an escalation:** a failed rotation drops the west-east overlay
(cross-host BPF flow graph) until the rollback runs, and the tool has only been
tested against stubs (`scripts/wireguard/test-rotate-psk.sh`). When an outage
of a few minutes is acceptable is Stevie's call.

### Procedure (on west)

```bash
cd ~/tmp/unheaded   # develop, with this change merged
# 1. Dry run: preflight only (hybrid SSH KEX to east, peers, both wg0.conf). Read the plan.
sudo scripts/wireguard/rotate-psk.sh
# 2. Rotate. Takes up to ~2-3 minutes: WireGuard uses the new PSK at its next
#    handshake, which comes once the current session is 120 s old.
sudo scripts/wireguard/rotate-psk.sh --apply; echo "rc=$?"
#    rc=0: rotated. rc=1: no handshake, both hosts are back on the old PSK.
#    rc=3: refused by preflight, nothing changed.
# 3. Health.
scripts/wireguard/verify-wireguard.sh
sudo wg show wg0 latest-handshakes      # recent timestamp; keys are not printed
sudo ls -l --time-style=+%F /etc/wireguard/wg0.psk; ssh govan@east 'sudo ls -l --time-style=+%F /etc/wireguard/wg0.psk'
journalctl -t unheaded-wg-psk -n 5
# 4. The retired PSK still sits in east's /etc/wireguard/preshared.key (setup-era copy). Remove it:
ssh govan@east 'sudo shred -u /etc/wireguard/preshared.key'
```

### Rollback

The tool rolls back by itself on a missing handshake or any failure after east
switched. If it was killed hard (SIGKILL, power) mid-run, `wg0.psk.old` on each
host holds the previous key:

```bash
# the same line on each host (each has exactly one peer)
sudo wg set wg0 peer "$(sudo wg show wg0 peers)" preshared-key /etc/wireguard/wg0.psk.old
```

`wg0.conf` is only rewritten after a successful handshake, so a reboot or
`systemctl restart wg-quick@wg0` also brings back the previous PSK until then.

### Enable the weekly timer (after one good manual rotation)

```bash
sudo install -D -m 0755 scripts/wireguard/rotate-psk.sh /usr/local/lib/unheaded/wireguard/rotate-psk.sh
sudo cp deploy/systemd/unheaded-wg-psk-rotate.service deploy/systemd/unheaded-wg-psk-rotate.timer /etc/systemd/system/
# The unit runs as root: root's ssh must reach govan@east non-interactively and
# east must already be in /root/.ssh/known_hosts (the unit's home is read-only).
# A dedicated key: Environment=WG_PSK_SSH_OPTS=-i /root/.ssh/id_wg_rotate via `systemctl edit`.
sudo systemctl daemon-reload
sudo systemctl start unheaded-wg-psk-rotate.service && journalctl -u unheaded-wg-psk-rotate -n 20
sudo systemctl enable --now unheaded-wg-psk-rotate.timer
```

Disable: `sudo systemctl disable --now unheaded-wg-psk-rotate.timer`.

### After

FND (WireGuard PSK static since setup, Phase 13) closes once the PSK file is
younger than 8 days on both hosts and the timer is enabled.
