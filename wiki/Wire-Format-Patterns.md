# Wire Format Patterns

Conventions the drafts follow when they describe bytes on the wire, taken from the RFC corpus and applied to the Monad. Use it when writing or reviewing a spec section. Layout itself: [[Protocol Technical Summary|Protocol-Technical-Summary]].

## Diagrams

- 32-bit rows, bit 0 = most significant (leftmost), `+-+-+` at field boundaries; network byte order (big-endian) unless a field says otherwise.
- The Monad is drawn as five rows of four octets (offsets 0x00–0x13) inside an IPv6 Hop-by-Hop Options header: Next Header, Hdr Ext Len (= 2, i.e. 24 bytes), then the option.
- HbH option type byte: top two bits = action if unknown (`00` skip), third bit = may change en route (`1`), giving `001xxxxx`; the suggested value is `0x3E`.

## Encodings

| Pattern | Monad use |
|---------|-----------|
| Fixed width | Version (1 octet, = 1), hop count, latency hint (u16 BE) |
| Exponent | One octet `e`; value = baseᵉ × multiplier, base and multiplier per field from Sophia (default base 2, multiplier 1) |
| Bitfield flags | Flags byte; undefined bits MUST be sent as zero and ignored on receipt |
| TLV | Extensions after the Monad; unknown TLV with critical bit set → error `0x07` |
| Variable-length integer, label stack | Referenced for extensions (QUIC varint style; MPLS-style stacks, RFC 3031) |

## Checksum

```
CRC-16/CCITT-FALSE   poly 0x1021   init 0xFFFF   no reflection   xorout 0x0000
scope: Monad octets 0x00–0x11 (18 bytes)   check("123456789") = 0x29B1
```

Sender computes, receiver verifies and drops on mismatch. The implementation is `Crc16::compute_monad` in `ebpf/monad-common`. When the `CUSTOM` flag is set the two checksum bytes carry exponent keys instead. A spec that defines a checksum states algorithm, parameters, scope, byte order and at least one test vector.

## Extensibility

- Keep structures 8-octet aligned (IPv6 extension header rule).
- Reserved fields: MUST be zero on send, MUST be ignored on receive.
- Version field: unknown version → drop silently (error `0x02`), never guess.
- Grow through IANA registries and next-header / TLV chaining, not by reinterpreting existing bits.

## Field template

Each field in a draft gets: name, offset and size, type (raw / exponent), byte order, valid range, default, sender rules, receiver rules, error on violation, IANA registry if any.

---

> **Source:** [docs/protocol/references/wire-format-patterns.md](../docs/protocol/references/wire-format-patterns.md) (2026-02; full worked examples and RFC excerpts) · `ebpf/monad-common/src/lib.rs`
