# S-QUANTUM-ARMOR: Hosts track progress (Phase 9, Steps 173-189)

Plan: `docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md`, Phase 9 (sshd + WireGuard).
Branch base: develop `424edf47`. Agent: Hosts track (parallel worktree), Phase 9 only.
Hosts are read-only for this track: every host command below is a read; E2/E3 are ESCALATE.

Skip Protocol acknowledged: any step past 3x its estimate or 2 failed debug attempts is
marked [STUCK] with what was tried, known-good state is committed first, and work continues
with the next independent step.

## Read-only host facts (2026-10-23)

| Fact | west | east |
|---|---|---|
| OpenSSH | 10.0p2 (Ubuntu 5ubuntu5.4), OpenSSL 3.5.3 | 10.0p2 (Ubuntu 5ubuntu5.4), OpenSSL 3.5.3 |
| `sshd -T` kexalgorithms | `mlkem768x25519-sha256,sntrup761x25519-sha512,sntrup761x25519-sha512@openssh.com,curve25519-sha256,curve25519-sha256@libssh.org,ecdh-sha2-nistp256,ecdh-sha2-nistp384,ecdh-sha2-nistp521` | identical |
| sshd port | 7375 (`ssh localhost` on 22 is refused) | 22 |
| KEX negotiated by west's client | `kex: algorithm: mlkem768x25519-sha256` (`localhost -p 7375`) | `kex: algorithm: mlkem768x25519-sha256` |
| `sshd-kex` (CIS allowlist) | pass | pass |
| `sshd-kex-pq` (strict) | FAIL: classical KEX allowed: curve25519-sha256, curve25519-sha256@libssh.org, ecdh-sha2-nistp256, ecdh-sha2-nistp384, ecdh-sha2-nistp521 | same FAIL |
| WireGuard | `wg0` up, PSK set (inline `PresharedKey` in `/etc/wireguard/wg0.conf`), handshake current | `wg0` up, PSK set; `/etc/wireguard/preshared.key` dated 2026-03-25 |
| PSK last set | 2026-03-25 (key files' date; never rotated) | 2026-03-25 |

H10 TRUE (OpenSSH >= 9.9 both, ML-KEM negotiated to east). H11 TRUE (one static PSK since setup).
Deployed overlay differs from `setup-wireguard.sh`: `Address` is a /128 per host (west `::1`, east `::2`).

## Steps

| Step | Status | Note |
|---|---|---|
| 173 | done | `TestSSHDKexPQProbe`, `TestSSHDKexPQProbeOnTodaysHosts` (red first) |
| 174 | done | `sshdHybridKex` + `Probes["sshd-kex-pq"]` in `cisprobes.go` |
| 175 | done | `go test -race ./pkg/compliance/...` all ok; golangci-lint 0 issues; gosec clean |
| 176 | done | runbook mlkem first (strict variant commented, E2); Yggdrasil provisioner + overlay patch sntrup first (bookworm 9.2) |
| 177 | partial | overlay verify PASSED, patch applies in a scratch dir, `bash -n` + shellcheck OK. `scripts/check-pq-kex.sh` and `docs/policy/pq-kex-baseline.txt` are not on this base: [DEFERRED: lead regenerates after Phase 4 merges] (remove the two Yggdrasil sshd lines from the baseline then) |
| 178 | done | commit `740d5dc1` |
| 179 | done | read-only: east and west both negotiate `mlkem768x25519-sha256`; strict probe verdict FAIL on both (classical still allowed), as expected |
| 180 | ESCALATE E2 | procedure with rollback and open-second-session guard: `docs/security/pq-host-escalations.md` |
| 181 | done | `scripts/wireguard/rotate-psk.sh` + `scripts/wireguard/test-rotate-psk.sh` (stubs, throwaway keys) |
| 182 | done | harness all PASS; registered `wireguard/test-rotate-psk` (fast) with `provoke_rotate_psk`; provocation verified by hand: 2 FAIL, rc=1. Real read-only dry run against west/east: preflight passed, rc=0 |
| 183 | done | units in `deploy/systemd/` (not `systemd/`: the units gate and every script-backed unit live there); check-systemd-units PASS (32), check-unit-binaries PASS |
| 184 | done | S77 docs corrected (PSK on, registry/ADR-049 removed, PQ section); also `docs/sprints/S77-WIREGUARD-DESIGN.md`, which repeated the same claims |
| 185 | done | commit `ca9c5b29` |
| 186 | ESCALATE E3 | first live rotation + timer: procedure with rollback in `docs/security/pq-host-escalations.md` |
| 187 | [DEFERRED: lead regenerates after Phase 4 merges] | `cmd/crypto-inventory` (Phase 3) is not on this base; host facts for the CBOM are in the table above |
| 188 | [DEFERRED: lead regenerates after Phase 4 merges] | nothing to commit without 187 |
| 189 | PASS | gate output below |

### Phase 9 exit gate (Step 189)

```
$ grep -c 'wg-peer-registry' docs/WIREGUARD-DESIGN-S77.md
0
$ bash scripts/wireguard/test-rotate-psk.sh >/dev/null 2>&1; echo "harness rc=$?"
harness rc=0
$ ssh govan@east 'sudo -n find /etc/ssh /etc/wireguard -newer /etc/hostname -mmin -300 2>/dev/null | wc -l'
0
west (read-only ls): /etc/wireguard and /etc/ssh/sshd_config.d newest mtime 2026-10-20 (before this session)
$ go test -race -count=1 ./pkg/compliance/...   -> 11 ok, 0 FAIL
```

- `sshd-kex-pq` probe tested; harness PASS and registered in the meta-gate; S77 doc no
  longer claims PSK off or cites the missing registry; E2/E3 written as ESCALATE.
- "repo sshd lines resolved from the lint baseline": DEFERRED with Step 177 (no
  `check-pq-kex.sh` / `pq-kex-baseline.txt` on this base).
- Full meta-gate run not done here (it needs a clean clone and runs every fast gate);
  the new row was proven by hand: provoked rollback no-op -> harness 2 FAIL, rc=1;
  restored -> rc=0. `--list` shows the row. Lead: include it in the post-merge meta-gate run.

### DoD (Phase 9)

- DoD-STD 4: `go vet`, `gofmt -l` clean on `pkg/compliance/crosswalk/collect`; golangci-lint 0 issues; gosec clean. shellcheck (all severities) clean on both new scripts and the provisioner.
- DoD-STD 7: no new listener; no secret on disk in the repo; the only 44-char base64 values printed this session were the two WireGuard PUBLIC keys in the real dry run. No PSK or private key was printed or written.
- No host change: every host command was a read (`sshd -T`, `ssh -vv ... true`, `wg show` with the PSK column masked, `ls`, `grep -c`, the tool's dry run).
- No new dependency.

### Plan amendments (Marshal Pattern 6)

- Step 181: handshake wait is 200 s, not 60 s. Setting a PSK does not end the current
  WireGuard session; the new PSK is used at the next handshake, started only once the
  session is 120 s old (REKEY_AFTER_TIME). A 60 s window would roll back healthy rotations.
- Step 181: the deployed PSK lives inline in `wg0.conf` (`PresharedKey =`), not in a PSK
  file, so the tool persists by rewriting that line (key read from a file by awk, never an
  argument) and also keeps `<if>.psk` (mtime = last rotation, for the FND-016 check).
- Step 181: ssh `-vv` debug lines end in CR LF; caught by the read-only dry run against the
  real hosts, fixed, and the stub now emits CR LF.
- Step 183: unit path `deploy/systemd/` (see above).
- Step 184: third doc corrected (`docs/sprints/S77-WIREGUARD-DESIGN.md`).
