# Sophia Dictionaries

Sophia gives the 20-byte Monad its meaning. Each exponent-encoded byte in the Monad is a key into a dictionary held in BPF maps on every hop, so one byte carries one of 256 pre-agreed meanings, and the meanings can change at runtime without changing the wire format. Current spec: **draft-bellis-unheaded-sophia-dictionary-03** (2026-03-19). Packet walk-through: [[Protocol Technical Summary|Protocol-Technical-Summary]].

## Trees, not tables

```
root[0x01] "service_identity" ─▶ sub-dict 1:  0x01 captain · 0x02 timeguru · 0x03 architect
root[0x02] "flow_action"      ─▶ sub-dict 2:  0x01 forward · 0x02 trace · 0x03 sample
root[0x03] "qos_class"        ─▶ sub-dict 3:  … 0x03 realtime

the same byte 0x03 = "architect" | "sample" | "realtime", depending on the root key
```

| Root key range | Use |
|----------------|-----|
| `0x00` | Reserved, never used |
| `0x01–0x0F` | Standard categories (`0x01–0x06` MUST exist before any packet is processed) |
| `0x10–0xFE` | Operator-assigned |
| `0xFF` | Chaos injection / vendor testing |

Kernel representation: `sophia_root` (hash, 256 slots) → `sophia_dicts` (array of maps). Lookup is two map reads (~300 ns) plus ~100 ns per nesting level.

## Typed sub-dictionaries (draft-03)

| Type | Meaning |
|------|---------|
| `0x00 LEAF` | Metadata only (draft-02 behaviour) |
| `0x01 BRANCH` | Points at a nested sub-dictionary |
| `0x02 COMPOSITE` | Metadata + nested reference |
| `0x03 ALIAS` | Indirection to another entry |

Nesting stops at 8 levels; deeper lookups abort with `SOPHIA_EVT_MISS`, emit an Anamnesis anomaly and fall back to the field's default. Circular references are rejected at load. Uses: service topology, inherited QoS policy, geographic routing, tenant namespaces.

## Limits

Per flow: 128 entries, 1 MB. Over the limit → reject with error `0x09` (insufficient buffer space); never evict. Global caps bound total memory. Error codes: [[Error Registry|Error-Registry]].

## Distribution

Dictionaries travel over Wotan on `sophia.dictionary.v{N}` as CBOR (RFC 8949), optionally QPACK-compressed (RFC 9204 adapted; static + dynamic tables, decompression-bomb limits) and ML-DSA-65 signed.

Update: build new maps `_v{N+1}` → load every entry → swap the `sophia_dicts` pointers atomically → keep the old version for a 60 s grace period → delete. At least two versions are live at once, so no packet sees a half-written dictionary.

## PQC and UPC extensions

- `PQC_SIG_MAP` / `PQC_KEY_MAP`: signature and key lookup, rotation ([[PQC Authentication|PQC-Authentication]]).
- Opcode dictionary: Sophia-driven instruction decode for the UPC ([[MBC ISA|MBC-ISA-Reference]]).

## Code

`services/sophia/` (service + KEM / HKDF / PQC policy maps), `cmd/sophia` (`:19005`), BPF side in `ebpf/monad-common`. Wiki copy of the draft: [[Draft-Sophia-Dictionary-03]].

---

> **Spec:** [draft-bellis-unheaded-sophia-dictionary-03](../docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md) · earlier: [-00](../docs/protocol/draft-bellis-unheaded-sophia-dictionary-00.md), [-01](../docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md), [-02](../docs/protocol/draft-bellis-unheaded-sophia-dictionary-02.md)
