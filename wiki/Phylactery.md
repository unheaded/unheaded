# The Phylactery

*φυλακτήριον, "safeguard"*: the Kingdom's encrypted storage layer. Like a lich's phylactery it holds the most protected thing in the system, and it is built as a Knight in its own armor rather than a plain service.

**Status: design only (2026-02).** No code implements it yet; storage questions are tracked in ADR-011, and the optional third seal in ADR-009 (deferred to Beta). The Kingdom's working persistence today is The Well ([[The-Well]]).

## Two seals

Every read or write needs two independent proofs in the same packet, at the same hop:

| Seal | What it proves | Where it lives | How |
|------|----------------|----------------|-----|
| **Sigil** | This packet is authorized to touch this data | In the protocol: Monad service id + Kingdom Mode extended registers | `HMAC-SHA3-256(master_key, flow_label ‖ trace_id ‖ hop_count)`, truncated; rotated per epoch via Sophia; O(1) BPF map check at each hop |
| **Ward** | Who is asking | Payload envelope `{operation, path, timestamp, nonce, signature}` | Operator's choice: Ed25519, ECDSA P-256 / secp256k1, HMAC-SHA-256, ML-DSA-65 |

```
UNLOCK = Sigil_valid(packet) AND Ward_valid(payload)
Sigil only → reject   Ward only → reject   neither → Shield drops it before the Phylactery sees it
```

Optional third seal: **Binding Circles** (ADR-009): an LPM-trie check that the source address sits in the right RFC 1918 trust zone, before either seal is checked.

The two seals are orthogonal: the Sigil stops replay and path tampering (bound to hop count and trace), the Ward stops a valid-looking packet from an unknown identity.

## Modes

| Mode | Sigil checked | Use |
|------|---------------|-----|
| 1. Per hop | At every hop | Maximum assurance; a compromised hop cannot forge a request |
| 2. Ingress/egress only | At Shield boundaries; interior hops do CRC + Sophia only (~270 ns) | Recommended default |
| 3. Kingdom to Kingdom | Stamped at A's egress, checked at B's ingress | Shared vaults; only over VXLAN-in-WireGuard/IPsec or private links, never the public internet |
| 4. Standalone | At its own Shield | Air-gapped vaults, key escrow, DR seed vaults |

## The Vault

AES-256-GCM at rest, never plaintext; keys via `HKDF-SHA3-256(master_key, path ‖ epoch)`, rotated per epoch; 4 KB blocks, content-addressed (SHA-256) with convergent encryption for dedup; Merkle root of all blocks published in Sophia. Operations are Monad instructions (storage verbs as new Flow Action values) and every one emits an Anamnesis event.

## Planned phases

P1 vault core → P2 Sigil layer → P3 Ward layer → P4 standalone Knight → P5 dashboard integration.

---

> **Source:** [docs/lore/PHYLACTERY.md](../docs/lore/PHYLACTERY.md) (2026-02) · ADR-009 · ADR-011
