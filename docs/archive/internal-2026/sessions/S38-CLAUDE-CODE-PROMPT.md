# S38 Claude Code CLI Prompt — eBPF Production Sprint

## Launch Command

```bash
cd ~/tmp/unheaded && claude --dangerously-skip-permissions
```

**IMPORTANT**: This sprint REQUIRES bare metal Linux with root access. XDP/TC/tracepoint programs cannot run in containers or VMs without BPF support. Verify kernel >= 5.15 with CONFIG_BPF enabled before starting.

Then paste:

```
You are executing Sprint S38 of the Unheaded project — production eBPF programs. THE CORE PRODUCT DIFFERENTIATOR. (~260K production LOC, Go 1.24 + Rust/Aya for BPF + cilium/ebpf for Go userspace). S36 (Four Pillars) and S37 (LICENSE+SBOM) are complete.

## MANDATORY: Read These Files First

1. `CLAUDE.md` — Agent guide. Sacred Laws.
2. `docs/battle-plans/S38-EBPF-PRODUCTION-BATTLE-PLAN.md` — THE BATTLE PLAN. 165 steps. Read ALL of it.
3. `ebpf/` directory — existing Rust/Aya BPF program sources
4. `crates/` directory — Rust crates (monad-mbc, monad-common)
5. `cmd/trace-collector-go/` — Go userspace loader
6. `docs/protocol/draft-unheaded-foundation-04.md` — Monad wire format (for packet_marker structure)

## YOUR MISSION

Execute ALL 7 phases of the S38 Battle Plan:

**Phase 0** (Steps 1-15): Environment — kernel BPF support, Rust/Aya toolchain, bpftool, verify S36/S37 state
**Phase 1** (Steps 16-40): packet_marker XDP program — THE MAIN EVENT. Mark packets with trace IDs. BPF maps, XDP attachment, traffic testing.
**Phase 2** (Steps 41-69): flow_tracker TC program — 5-tuple connection tracking, TCP state machine
**Phase 3** (Steps 70-105): latency_probe tracepoint — RTT measurement via tcp_probe, ring buffer streaming
**Phase 4** (Steps 106-135): trace-collector Go unification — single binary loads all 3 BPF programs
**Phase 5** (Steps 136-152): Wotan integration — publish traces to Wotan gRPC (18001), batching, health
**Phase 6** (Steps 153-176): Dashboard visualization — trace table, packet flow diagram, latency charts
**Phase 7** (Steps 177-191): E2E verification — inject packets, verify full pipeline, performance baseline

## EXECUTION RULES

- YOU ARE RUNNING IN AUTONOMOUS MODE. Proceed without pausing.
- Auto-commit at every [C] checkpoint.
- Follow the battle plan STEP BY STEP.
- BPF VERIFIER ERRORS ARE EXPECTED. The [D] debug steps have specific fixes for common rejections:
  - Back-edge in loop → add loop bound annotation
  - Unbounded memory access → add bounds check before dereference
  - Invalid map access → verify map type matches program type
  - Stack overflow → reduce local variables, use per-CPU arrays
- When BPF verifier rejects, READ THE ERROR carefully. The line number tells you exactly where.
- Stuck protocol: skip after 3x time or 2 failed attempts. BPF issues are the #1 stuck source.
- Performance targets: <100µs per-packet overhead, >100K packets/sec, <5% CPU

## WHAT NOT TO DO

- DO NOT modify licensing files (S37 handled)
- DO NOT change port assignments (S36 handled)
- DO NOT push to remote
- DO NOT use unsafe BPF helpers without bounds checking
- DO NOT skip BPF verifier compliance — if it doesn't verify, it doesn't ship

When complete, report: "S38 COMPLETE — PRODUCTION eBPF OPERATIONAL" with: programs loaded, maps pinned, traces flowing, performance numbers.

Go.
```
