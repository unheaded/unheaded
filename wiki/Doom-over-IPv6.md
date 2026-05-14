# Doom over IPv6

*This page is preserved for legacy link-stability. Current canonical reference is **[Doom on UPC](Doom-on-UPC)**.*

Computational-completeness proof for the Monad specification (Section 12). Doom running inside eBPF via the MBC bytecode ISA, dispatched at each IPv6 hop. **PLAYABLE.** 559 frames rendered, 819 M+ instructions executed, zero halts, zero ROM faults at the L3 milestone; stable beyond 5.9 B instructions in extended runs.

This proves that the 20-byte Monad register file, combined with Sophia dictionaries and Wotan memory, constitutes a Turing-complete computational substrate at wire speed. The same XDP program now runs the [Linux on UPC](Linux-on-UPC) ascent (compiled with `--features ascend-linux` to enable the five new privilege/atomic opcodes).

---

> **Sources:** [Doom on UPC](Doom-on-UPC) · [docs/doom/](../docs/doom/) · [docs/doom/ARCHITECTURE.md](../docs/doom/ARCHITECTURE.md) · [docs/doom/COMPUTATIONAL_GENERALITY.md](../docs/doom/COMPUTATIONAL_GENERALITY.md)
