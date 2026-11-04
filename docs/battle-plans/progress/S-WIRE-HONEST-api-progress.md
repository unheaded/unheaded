<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-WIRE-HONEST lane api progress

## Log

- Phase 7 (Steps 54-60) done: red as predicted, green unprivileged (2 PASS, 2 SKIP, ok), pkg/ebpf and trace-collector-go ok. Lane worktree created by lane.sh (did not exist at start). Root proof is Phase 8R.
- Phase 8 (Steps 61-68) done: red as predicted (undefined flowKeySize), protoapi.py all edits applied, green ok, step 66 gave 0, 0 and cilium/ebpf v0.22.0, wiki line appended; default mode still mock. Root proof is Phase 8R.
- Phase 8R (Steps 69-72) done: test binaries built as the normal user at `72c25955`, run under sudo inside the `ebpf-load.lock` flock on kernel 6.17.0-41-generic. Five `--- PASS` (TestOpenPinnedMap_MissingPath, TestPinnedMap_KeyAndValueSizeRefused, TestPinnedMap_RoundTrip, TestOpenPinnedMap_RefusesPerCPU, TestBPFBackend_RealPinnedMap), `PASS` twice, 0 `wire-honest` pins left in /sys/fs/bpf. First run of these root tests since the forge. Binaries removed.

## For Stevie
