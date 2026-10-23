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
| KEX negotiated by west's client | n/a: `ssh localhost` refused (no sshd answering on 127.0.0.1:22) | `kex: algorithm: mlkem768x25519-sha256` |
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
| 178 | done | commit |
