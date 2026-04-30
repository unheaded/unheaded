# WAVE 10D Apr 12-13 Post-Mortem

## Summary

Alleged 24-hour training run (2026-04-12 to 2026-04-13) livelocked west. Root cause: uncommitted `GpuBwAttnWeights` in `src/train.rs` pre-uploaded all 32 layers of Mistral-7B attention weights unconditionally, pushing a 14 GB host into ROCm HMM/SVM paging thrash. System required hard reboot (2026-04-13 20:42 UTC).

## Evidence in this directory

- `boot-1.journal` — `journalctl -b -1` slice covering Apr 12–13. Contains 8,195 `svm_range_restore_work [amdgpu]` >10000 µs warnings and the hung `kworker/2:2` mutex deadlock (19:15 UTC Apr 12).
- `train.rs.diff` — frozen copy of uncommitted `crates/zhenai-forge/src/train.rs` changes at post-mortem time. 189 lines. The proximate cause of the livelock sits in the new `GpuBwAttnWeights` struct and its unbounded upload loop.
- `binary-vs-source.txt` — mtime of `target/release/zhenai-forge` vs `src/train.rs`. Binary absent at post-mortem time (cleaned or never rebuilt against uncommitted source); source last modified 2026-04-11 17:00 UTC.

## Memory budget (first-principles, for Phase B gate constant)

Host: 14 GB RAM, AMD Radeon + ROCm. No dedicated VRAM — shared/HMM.

| Component | Size |
|-----------|------|
| Mistral-7B-Q5_K_M weights | ~5.1 GB |
| LoRA rank-16 adapters (fp32, Q/K/V/O × 32) | ~67 MB |
| Forward activation cache (seq=2048, bs=1, fp32) | ~1.0 GB |
| `bw_attn` per layer (Q/K/V/O weights, fp32) | 0.27 GB |
| Kernel + driver + shell headroom (non-negotiable) | ≥2.0 GB |

**Available for `bw_attn`**: 14 − 5.1 − 0.067 − 1.0 − 2.0 ≈ 5.83 GB → max 21 layers.
**Safety cap**: `BW_ATTN_MAX_LAYERS = 16` (4.3 GB, leaves 2.6 GB host headroom).
**Apr 12 config**: 32 layers × 0.27 GB ≈ 8.6 GB → overflowed budget by 2.7 GB → HMM paging → livelock. Exact match to observed failure.

## Plan

See `/home/govan/.claude/plans/lazy-snuggling-gray.md` for the full Post-Mortem & Hardening Plan (Phases A–E with the Scientist's start-smaller ladder and the Marshal's gating rubric).
