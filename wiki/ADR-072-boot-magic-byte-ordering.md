# ADR-072: BOOT_MAGIC Byte-Ordering Convention

**Status:** Accepted
**Date:** 2026-05-10

> Wiki stub generated 2026-05-11 by overnight Marshal shift. See the canonical ADR for full text + decision rationale.

## TL;DR

The BootParamsV2 `magic` field is the canonical 32-bit constant `0x554E4844`:

| View | Interpretation | Where it shows up |
|------|----------------|-------------------|
| MSB-first hex | `0x554E4844` reads as `'U','N','H','D'` (the brand spelling) | Spec text, equality comparisons, conversation |
| Little-endian wire | The 4 bytes at increasing memory addresses are `0x44, 0x48, 0x4E, 0x55` (`D,H,N,U`) | `BOOT_MAGIC.to_le_bytes()`, hex-dumps, byte-iterating consumers |

Both views describe the same constant. Same pattern as ELF magic (`\x7fELF` = `0x464C457F` as a host-endian u32 on LE systems).

## Canonical

[docs/adr/ADR-072-boot-magic-byte-ordering.md](../docs/adr/ADR-072-boot-magic-byte-ordering.md)

## Affected surfaces

- `docs/doom/UPC_BOOT_PROTOCOL.md` §"Magic Value"
- `docs/doom/UPC_BOOT_PROTOCOL_V2.md` lines 113, 140
- `crates/xv6-mbc/src/lib.rs:38-50` (canonical docstring)
- `crates/xv6-mbc/src/lib.rs:65` (intentional-pin test)
- `cmd/upc-bootctl/src/bootparams.rs:118` (intentional-pin test)
- `cmd/upc-bootctl/src/main.rs:111-115, 150` (operator print strings)

## Cross-references

- [ADR Index](ADR-Index.md)
- [ADR-067](ADR-067-mbc-isa-v2-and-upc-abi-v1.md) — MBC ISA v2 + UPC ABI v1
- [Architecture overview](Architecture.md)
