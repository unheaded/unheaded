# Protocol Technical Summary

One-page reference for how a packet moves through the protocol: the Monad it carries, the dictionaries that give it meaning, the boundary that adds and removes it, and the events every hop leaves behind. Specs: [[Protocol Foundation|Protocol-Foundation]], [[Sophia Dictionaries|Sophia-Dictionaries]], [[Wotan Memory|Wotan-Memory-Model]], [[Drafts-Index]].

**Wire format v0x01, frozen 2026-02-28.** 12 IANA registries in Foundation draft-06; IPR clear.

## The Monad (20 bytes)

Carried in an IPv6 Hop-by-Hop Options header (24 bytes total with option header and padding) inside the Limited Domain (RFC 8799). Layout from `ebpf/monad-common/src/lib.rs`:

```
 0x00 Version (raw, 0x01)  0x01 Src Service ID   0x02 Dst Service ID   0x03 Hop Count (raw)
 0x04 QoS Class            0x05 Flow Action      0x06 Circuit State    0x07 Flags (raw)
 0x08 Latency Hint µs (raw u16, BE)              0x0A Deploy Ring      0x0B Mesh Flags
 0x0C Src Prefix Lo        0x0D Dst Prefix Lo    0x0E Scratch[0]       0x0F Scratch[1]
 0x10 Scratch[2]           0x11 Scratch[3]       0x12 Checksum: CRC-16/CCITT over 0x00–0x11
```

Fields not marked raw are **exponent-encoded**: one byte = one of 256 meanings, looked up in a Sophia dictionary and replaceable at runtime by a BPF map update. Scratch[0:1] / [2:3] are the accumulator and address register pairs the UPC computes with.

Flags: `CHAOS 0x80` (Yaldabaoth touched it), `CANARY 0x40`, `TRACED 0x20` (every hop emits a full event), `ENCRYPT 0x10`, `SAMPLED 0x08`, `MIRROR 0x04`, `CUSTOM 0x02` (scratch + checksum carry exponent keys), `RSVD 0x01` (must be 0).

## Sophia

Dictionaries are trees: `sophia_root` maps a key to a typed sub-dictionary (`service_identity`, `flow_action`, `qos_class`, `deployment_ring`, `circuit_state`, `mesh_flags`, … `0xFF chaos_marker`), and each sub-dictionary maps byte → meaning. Same maps in the kernel (BPF) and userspace (Wotan decode).

## Shield: the boundary

- **Ingress (XDP), BIRTH:** blocklist, per-source token bucket, optional geo check → drop with a block event, or add the Monad (version 0x01, service IDs classified via Sophia, hop 0, policy QoS, ring lookup, CRC) and emit `anamnesis.birth`.
- **Egress (TC), DEATH:** emit `anamnesis.death` with the final Monad and hop count, strip it, run exfiltration checks. The next host outside the domain sees a clean packet.

## Per hop (the Void)

Verify CRC (mismatch → `anamnesis.anomaly`) → increment hop count → act on Flow Action (trace / sample / mirror / rate-limit) → component logic (Hauberk circuit state, Pauldrons backend select, Vambraces observe) → recompute CRC → pass.

## Anamnesis events

64-byte per-CPU ring buffer records: timestamp_ns, event type (BIRTH 0, COMPUTED 1, CHAOS 4, DEATH 6, ANOMALY 8), hop index, Monad before and after the hop, trace id, Wotan address/value if memory was touched. Collectors decode through Sophia and publish `anamnesis.<type>` ([[Fae Chamber Contracts|Fae-Chamber-Contracts]]). Sizing: ~53 MB/s per 10 Gb/s at one event per packet.

## State

- **Pleroma** (desired): dictionary entries, programs and policies that should be active, declared in Git.
- **Kenoma** (actual): a fold over Anamnesis events per (src, dst, trace), not a database.
- **Drift:** Kenoma ≠ Pleroma → drift event → Wotan pushes Pleroma → BPF map update.
- **Yaldabaoth:** declared chaos (e.g. 0.1% of packets), every touched packet carries `CHAOS`.

---

> **Source:** [docs/protocol/PROTOCOL_TECHNICAL_SUMMARY.md](../docs/protocol/PROTOCOL_TECHNICAL_SUMMARY.md) (2026-03; its §2 layout predates the frozen register file, code above wins) · `ebpf/monad-common/src/lib.rs`
