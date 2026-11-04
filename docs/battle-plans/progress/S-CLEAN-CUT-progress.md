<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-CLEAN-CUT progress

## For Stevie (oldest first)

- Phase 1 (Steps 12): the plan's `sed` range for the `waf/` tree removed only the `pkg/waf` block in `docs/architecture/PROJECT_STRUCTURE.md`. It also removed the `cmd/waf` header line, so the four lines `│       └── src/`, `│           ├── main.rs`, `│           └── rules/` (old lines 73-76) are now orphaned under `cmd/unheaded/`. Left as the plan wrote it (no improvised fix). Needs a 3-line delete by a human or Librarian.
- Phase 5A: `nixos/modules/telemetry.nix` does not parse, and did not before this sprint: `nix-instantiate --parse` on the parent of `604160de` gives the same error (`services.prometheus` set at line 54 and again at line 73). `nixos/tests/frr.nix` also fails (unterminated string at end of file). No phase covers either, so both were left alone. Fix them, or leave them as design files?
- Phase 5A: two `vLLM/ROCm` descriptions remain, `nixos/README.md:9` and `nixos/hosts/host-a/configuration.nix:3`. The Step 36 gate is lowercase `vllm-rocm` and misses them, and no phase edits them. The ROCm packages in host-a were kept: they are GPU compute, not vLLM.
- Phase 5A: `nixos/modules/services/trace-collector.nix` now runs `trace-collector-go` but still sets `TRACE_PORT` and `TRACE_LOG_LEVEL`. `git grep` finds neither name in `cmd/trace-collector-go`, so the module's `port` and `logLevel` options look dead for the Go binary. Not changed: outside Step 36.

## Stuck steps

## Evidence
- Steps 16-19 (Phase 2, lxd/containers): Step 16 `39` / `rc=1`; Step 17 `GONE`; Step 18 `1` / `rc=1`; Step 19 committed `748e42bf` (signed, 43 paths: 39 deletions + 4 edits). No stuck steps.
- Phase 3 (Steps 20-28): committed `bf5788d7`. Step 28 `check-compose-hardening.sh`: `[PASS] 52 of 53 services hardened across 7 compose files; 0 + 1 baselined exceptions`. `make -n -C docker help` OK, `docker/Makefile` vllm/host grep 0. Not committed here: this evidence line (the coordinator folds it).
- Phase 6 (Steps 38-43), commit 0ec25e07 (signed): Step 38 Doom AF_XDP grep 0, three Cargo files :0. Step 39 rc=1 twice. Step 40 removed four AF_XDP dirs (0, rc=1). Step 41 sub.py 1 block, lock entries removed 2, grep 0. Step 42 XSKMAP comment kept value 17, fuzz seed list cut, go vet ./pkg/ebpf/ clean, rc=1. No [STUCK].
- Phase 1 (Steps 8-15): committed `f9c66764` (signed). Step 8 `rc=1` twice, no lines. Step 9 `GONE`. Step 10 `2 block(s) applied`. Step 11 `ok  	unheaded/tests/s77`. Step 12 `1 block(s) applied`, then `0`. Step 13 `1 block(s) applied`. Step 14 `0`. Step 15 gate `git grep -l 'pkg/waf' -- '*.go'` printed nothing, `committed f9c66764`.
- Phase 19 (Steps 85-89, vLLM repo leftovers and the note): Step 85 `GONE`; Step 86 `git grep -c -i vllm -- lxd/README.md` printed nothing, `rc=1`; Step 87 `sub.py: configs/services.yaml: 1 block(s) applied`, `sub.py: docker/telemetry/prometheus/rules/unheaded.yml: 1 block(s) applied`, `ok  	unheaded/pkg/discovery`; Step 88 `NOTE-OK`; Step 89 committed `b0470d93` (signed, `%G?` = G, 6 paths: 2 deletions under lxd/vllm-rocm, 4 edits). No stuck steps. Caveat: Step 87's `go test` ran without the `GOMAXPROCS=4 GOTMPDIR=/var/tmp` export and without the build flock; it is a single-package test. Not committed here: this evidence line (the coordinator folds it).
- Phase 11 (Steps 65-69, tty-bridge): committed `464d2958`. Step 66 red showed `mode = skeleton, want push-ingest`, `version = 0.1.0-skeleton, want 0.1.0`, FAIL. Step 67 `4 block(s) applied`. Step 68 no gofmt line, `ok  	unheaded/cmd/upc-tty-bridge`, `rc=1`. No stuck steps. Not committed here: this evidence line.
- Phase 5A (Steps 36-37): Step 36 printed two `sub.py` lines, then `rc=1`. Step 37 committed `604160de` (signed, 7 paths). No hosts set `services.unheaded.ebpfExporter` or `vllmRocm`, so no option is left dangling. Extra check, not a plan step: `nix-instantiate --parse` passes on both host files and `trace-collector.nix`; `telemetry.nix` fails on a pre-existing error (see For Stevie). Nothing was applied. No stuck steps.
- Phase 8 (Steps 52-56), commit 6db2e17e (signed). Step 52 `7` (seven files moved to docs/archive/retired-2026-11/af-xdp). Step 53 `README-OK`, grep `0` / `rc=1`. Step 54 `sub.py: docs/README.md: 2 block(s) applied`. Step 55 `removed True`; the plan's grep for XSK/AF_XDP/af-xdp in docs/architecture/RUST_COMPONENTS.md leaves 18 lines of prose, recorded here for the docs sweep (not a stuck step). Step 56 `committed 6db2e17e`. No [STUCK].
