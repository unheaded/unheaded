# Dark Grimoire

BlackMage's attack-surface taxonomy for the Unheaded protocol and the UPC: what can be attacked, how, and the mitigation checklist for each. Campaigns that exercise these classes: [[LICH Campaigns|LICH-Campaigns]].

## Sections (S21 addendum, 2026-02)

| § | Surface | Classes |
|---|---------|---------|
| 1 | MBC / UPC execution (Doom substrate) | D1 instruction decoding, D2 stack under/overflow, D3 memory protection bypass, D4 load-store races, D5 privilege escalation in the sandbox, D6 integer overflow in instruction arithmetic |
| 2 | Cross-document consistency between the drafts | X1 version field interpretation, X2 CRC / integrity scope mismatch, X3 TLV extension parsing, X4 error handling and fallback divergence |
| 3 | IANA registries | Option squatting, allocation gap exploitation, TLV type conflicts |
| 4 | Computational completeness (Monad §12) | DoS via infinite loops, timing side channels, integer overflow as DoS |
| 5 | Concurrency checklist | every `get_ptr_mut()`, map hot-swap, multi-packet flow ops, L1 cache invalidation, WAL atomicity |
| 6 | HTTP/3 / QUIC | 17 classes: version downgrade, stream ID reuse, QPACK, connection migration, path validation, flow control, 0-RTT replay, stateless reset forgery, ACK amplification, packet-number oracle, frame type confusion, settings pollution, pseudo-header injection, priority inversion, loss amplification, handshake amplification, cross-protocol smuggling |

Each class has an attack scenario and a `- [ ]` mitigation checklist with a test. Section 2 findings fed RFC patch text in the drafts ([[Drafts-Index]]); section 1 maps to LICH-007 (MBC fuzzing) and section 5 to LICH-008 (Wotan cache races).

## Related material

| Where | What |
|-------|------|
| `tomb/grimoire/threat-models/` | `kingdom-threat-model.md`, `monad-protocol-threats.md` |
| `tomb/grimoire/mitre-attck/` | MITRE ATT&CK fetch script for the RAG corpus |
| `tomb/dark-mirror/` | Dark Mirror material |
| `docs/security/threat-model-*.md` | Per-component threat models: Champion gate, Sealed Cask, Zhen AI, operator workstation |
| `docs/security/threat-register.md` | Threat register |

---

> **Source:** [docs/security/dark-grimoire-addendum.md](../docs/security/dark-grimoire-addendum.md) · `tomb/grimoire/`
