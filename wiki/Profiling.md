# Profiling

CPU and heap profiles for Go services, and hardware counters for BPF programs, with nothing outside the Go toolchain and the kernel (S-LOW-FRUIT Phase 4).

## Go: pprof on loopback

pprof is off by default. Turn it on per process:

| Binary | Switch |
|---|---|
| akira, huginn, heimdall-daemon, upc-tty-bridge | `--pprof 127.0.0.1:6060` |
| wotan, dashboard-backend | `PPROF_ADDR=127.0.0.1:6060` |

It runs on its own listener, never on the service port. Any address that isn't loopback (`127.0.0.0/8`, `::1`, `localhost`) is refused at startup with exit 2: heap and goroutine dumps can hold service tokens and keys.

## Flame graph

```bash
make build-profile                                   # bin/ with symbols (the release build strips them)
./bin/akira --pprof 127.0.0.1:6060 &
scripts/flamegraph.sh 127.0.0.1:6060 bin/akira 30    # opens pprof's web UI: View -> Flame Graph
scripts/flamegraph.sh 127.0.0.1:6060 bin/akira 30 --top
```

Raw profiles go to `var/pprof/` (gitignored). Never commit a `.pb.gz`; an SVG export of the flame graph (symbols only) is fine.

## BPF: per-program counters

```bash
sudo scripts/bpf-profile.sh monad_cpu 10    # cycles, instructions, l1d_loads, llc_misses per run
```

The program has to be loaded, for example inside the UPC boot recipe (`scripts/upc-netns.sh up`, then `upc-bootctl boot` in `upc0`). Some counters are missing inside VMs.

A bpftool built without clang has no `prog profile` (west's doesn't). The script then reports the kernel's run statistics for the window instead (`kernel.bpf_stats_enabled`): runs, run time, ns per run. On west, 2026-10-29: `monad_cpu` 0.39 µs per tick over an 800,000-tick xv6 boot.

See also: [[Observability-Backends]].
