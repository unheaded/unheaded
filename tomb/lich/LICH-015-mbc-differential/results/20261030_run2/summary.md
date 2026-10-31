# LICH-015 differential run

- programs: 50000
- seed0: 0xc3c35eed
- unique divergences (cart-allowlist opcode set): 0
- arm A: monad_mbc::ExecCpu (reference interpreter)
- arm B: monad_common::cart::step (= monad-cpu-ebpf; BPF==reference on live kernel, S5 Phase 11)

Verdict: **CLEAN** — no divergence over the cart-allowlist opcode set.

Out-of-allowlist opcodes (MULHU, MRET/SRET/LR.W/SC.W/FENCE) are not driven through arm B in-process; see `diff_harness mulhu-probe` and the east kernel-attached arm (ADR-062 LICH-005).
