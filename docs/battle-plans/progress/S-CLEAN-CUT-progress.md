<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-CLEAN-CUT progress

## For Stevie (oldest first)

- Phase 1 (Steps 12): the plan's `sed` range for the `waf/` tree removed only the `pkg/waf` block in `docs/architecture/PROJECT_STRUCTURE.md`. It also removed the `cmd/waf` header line, so the four lines `│       └── src/`, `│           ├── main.rs`, `│           └── rules/` (old lines 73-76) are now orphaned under `cmd/unheaded/`. Left as the plan wrote it (no improvised fix). Needs a 3-line delete by a human or Librarian.

## Stuck steps

## Evidence
- Steps 16-19 (Phase 2, lxd/containers): Step 16 `39` / `rc=1`; Step 17 `GONE`; Step 18 `1` / `rc=1`; Step 19 committed `748e42bf` (signed, 43 paths: 39 deletions + 4 edits). No stuck steps.
- Phase 3 (Steps 20-28): committed `bf5788d7`. Step 28 `check-compose-hardening.sh`: `[PASS] 52 of 53 services hardened across 7 compose files; 0 + 1 baselined exceptions`. `make -n -C docker help` OK, `docker/Makefile` vllm/host grep 0. Not committed here: this evidence line (the coordinator folds it).
- Phase 6 (Steps 38-43), commit 0ec25e07 (signed): Step 38 Doom AF_XDP grep 0, three Cargo files :0. Step 39 rc=1 twice. Step 40 removed four AF_XDP dirs (0, rc=1). Step 41 sub.py 1 block, lock entries removed 2, grep 0. Step 42 XSKMAP comment kept value 17, fuzz seed list cut, go vet ./pkg/ebpf/ clean, rc=1. No [STUCK].
- Phase 1 (Steps 8-15): committed `f9c66764` (signed). Step 8 `rc=1` twice, no lines. Step 9 `GONE`. Step 10 `2 block(s) applied`. Step 11 `ok  	unheaded/tests/s77`. Step 12 `1 block(s) applied`, then `0`. Step 13 `1 block(s) applied`. Step 14 `0`. Step 15 gate `git grep -l 'pkg/waf' -- '*.go'` printed nothing, `committed f9c66764`.
