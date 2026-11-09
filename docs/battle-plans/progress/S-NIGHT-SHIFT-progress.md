<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-NIGHT-SHIFT progress

Plan: `docs/battle-plans/S-NIGHT-SHIFT-battle-plan.md`. Resume at the first step not listed as done below.

## Log

- Steps 1-8 done 2026-11-09: develop clean at `5fddaa5b`, signing OK, /var 1.1T free, user slice 4096 tasks / 12 GiB, watchdog running.
- Steps 9-20 resolved without a migration: the `wotan_*` tables lived only in the legacy database `unheaded`, which was dumped (`~/backups/well/2026-10-15/unheaded-legacy-final.dump`) and dropped on 2026-10-15 (`docs/battle-plans/WELL-ERDA-AUDIT-2026-10-13.md` item 3.6, `wiki/The-Well.md`). No database the MANIFEST routes holds them (2026-10-15 snapshots). Migration 050 not written; S-E1-LIVE unchanged. Plan amendment: Appendix C's "Apply migrations 047, 049, 050" reads 047 and 049.
- Steps 21-32 done: `da9a8725` removes the 68-byte TRACE_MAP and STATS decoders and their tests; `FuzzDecodeKernelPacketEvent` replaces the dead fuzz target (20 s, no crash); race tests, build, man, script tests OK. Found on the way: check-nosec-ratchet red on develop from the syslog merge (`865e53ff`): `ab8dd7e0` (test file) and `45ea7023` (port range check instead of two G115 annotations; pinned gosec clean); ratchet PASS at 863.
- Steps 33-52 done: pinned trivy 0.76.1 in `/var/tmp/unheaded-sprint/bin/trivy-pin` (checksum-verified). wotan and kanban images build; the 3 HIGH (musl, musl-utils, zlib) came from a stale cached `alpine:3.24` (3.24.2); with `--pull` (3.24.4) both scan clean at CRITICAL/HIGH. `wotan --version` answers (no flag dump) but said `dev`: `9b9072e2` stamps `pkg/cmdline.Version` and docker.yml passes the tag or sha. ci-verdicts self-test 100/100.
- Steps 53-66 done: `0179d49e` syft 1.56.0, grype 0.121.1 (sha256 matched publishers' checksum files, installed and run). check-trivy-misconfig on the pinned 0.76.1: PASS, 11 at baseline. govulncheck: none. cargo audit (ebpf): rc 0. pip-audit: 2 accepted. **Step 55 [STUCK] for ansible-core**: 2.22.0 needs Python >= 3.13 and every CI job pins 3.12, so `check-tool-freshness.sh` stays red on that row until the CI Python moves.
- Steps 67-78 done: `7de98417` wiki ADR rows 136-137; SBOM REVIEW verdict carries Stevie's 2026-11-06 ruling from the generator (clears nothing by itself; every package still named), `sbom/SBOM.md` regenerated `--report-only`, THIRD_PARTY.md date follows; test-generate-sbom all pass. Releases apt line corrected (LAN listener; nginx root moves at Step 192). `84942beb` on `sprint/hop-enforce-refusal`: ebpf/README names `hop-enforce`.
- Steps 79-81 done: clean clone at `d95fc52d`; `go build ./...`, `go vet ./...`, `go test -race -shuffle=on -count=1 ./...`: 225 packages ok, 0 fail (`/var/tmp/unheaded-sprint/nightshift-race.txt`). Step 83 (full meta-gate) running.

## STUCK

- Step 55 (ansible-core 2.22.0): needs CI Python >= 3.13; parked under For Stevie. Downstream: nothing blocked; Phase 6's meta-gate may show check-tool-freshness red on this row only.

## For Stevie

- **CI Python 3.12 -> 3.13 (or 3.14)?** ansible-core 2.22.0 requires >= 3.13, so the tool-freshness gate stays red on ansible-core until every workflow's `python-version` and the lock image move. Recommend: move CI to 3.13 in one commit with a relock, since the compile-against-latest rule points that way; it touches every Python job, so it waits for your word.
- Answer 26 (drop Wotan's old tables) needs nothing: they were dropped with the legacy `unheaded` database on 2026-10-15.

## Unsigned

(none)

## Marshal

- Phase 0 gate: PASS. Mission S-NIGHT-SHIFT, unattended only; out of scope = Appendix C. Clearance: PROCEED to Phase 1.
- Phase 1 gate: PASS (moot, evidence above). Plan deviation S1, justified: the premise (tables still exist) was false; no work invented to replace it. PROCEED to Phase 2.
- Phase 2 gate: PASS. Citation S2 (coordinator): today's merges `865e53ff` and `9da03ff4` were committed without the nosec ratchet, and a chained `gate | tail && commit` hid a red gate once. Corrected: gates are read on their own line before any commit. Phase 6 runs every row. PROCEED to Phase 3.
- Phase 3 gate: PASS. The version fix is inside Step 41's remit (the image's `--version`). PROCEED to Phase 4.
- Phase 4 gate: PASS with one STUCK row (ansible-core). Scope check: moving CI Python is ADJACENT, parked, not done. PROCEED to Phase 5.
- Phase 5 gate: PASS. PROCEED to Phase 6 (alone: no other builders).
