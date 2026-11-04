<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-WIRE-HONEST progress

## Log

- Phase 0 started from develop `f1cf9670` (the plan measured `ba87679b`). Worktree, symlink and payloads (21) ok; Steps 1-5 committed.
- Step 6: `go build` and `go vet` printed nothing. Race run: 240 ok, 32 no test files, 1 FAIL, none cached.
- Step 6 ran twice. The first run was discarded: `GOTMPDIR=$P/gotmp`, added by this agent and not in the plan, moved `t.TempDir()` under `$P`, which made the `cmd/unheaded-daemon` socket path 111 bytes (limit 107), so `TestLXDREST_UpdateProxyGivesUpAfterTwoConflicts` failed. It passes with no `GOTMPDIR` and with a short one, and fails again with the long one. The recorded baseline used `GOTMPDIR=/var/tmp/swh` and `-count=1`.
- Lanes: never point `GOTMPDIR` or `TMPDIR` at `$P`. Use `/var/tmp/swh`, or that test fails for a reason unrelated to the change.
- Step 8: `[PASS] 36 eBPF programs pass the verifier; no known failures`, run under the shared eBPF lock. It took 34 s, not the estimated 25 m; the ELFs were built during this run, in this worktree's `ebpf/target/load-gate`.
- Step 9: hop_xdp 153835, shield_xdp 1502, shield_tc 976 (default), kernel `6.17.0-41-generic`. All 36 rows match `docs/policy/ebpf-insns-baseline.tsv`; `check-ebpf-insns.sh --from $P/insns-pre.tsv` passes.
- Step 10: ADR-125 to ADR-134 are free; `id: FND-031` count 0.

## Known failures

Later gates fail only on lines not in this list.

```
unheaded/pkg/secrets/store
```

`pkg/secrets/store` is intermittent, not constant: it passed in the discarded run, failed 1 of 12 separate runs afterwards, and one `-count=20` run panicked. A pass at Step 130 is not a change, and neither is a fail. See "For Stevie".

running units: 3

## For Stevie

1. `pkg/secrets/store` has a real crash, not only a load flake (S-TRUE-WIRE's progress file called it that). `notifyWatchers` (`file.go:259-272`) copies the watcher slice under `RLock`, releases it, then sends; `Close` (`file.go:356`) and `removeWatcher` (`file.go:282`) can close those channels in that gap. Stack from the baseline run: `panic: send on closed channel` at `file.go:266`, from `checkForChanges` in the `watchFiles` goroutine, so a running process dies, not just a test. Not fixed here: outside the six DPs. Do you want it fixed, and in which plan?
