# Wotan Topic Signing — ML-DSA-65 on `config.*`

ADR-043 hard condition #2: all messages on `config.*` topics MUST be
ML-DSA-65 signed before [[Mímir's Law|Mimirs-Law]] spike begins.
Implemented as a prerequisite sprint on `main` before the spike branch.

> **Status**: SHIPPED 2026-04-11 (commit `5e0d0400`); **fixed 2026-10-19**: until then the signature was checked against the public key the publisher sent in the same request, so anyone could sign with a key of their own and pass, and the HTTP publish path took `config.*` without any signature. Now Wotan verifies only against the keys in `configs/wotan.yaml` → `topics.signing_keys` (base64 ML-DSA-65 public keys; empty = every `config.*` publish refused), and both HTTP write paths refuse `config.*`.

## What Got Wired

| File | Change |
|---|---|
| `services/wotan/proto/topic.proto` | Added `signature`, `public_key`, `algorithm` fields to `TopicPublishRequest` and `TopicEvent` |
| `services/wotan/internal/signing/signing.go` | NEW — `TopicSigner` + `TopicVerifier` wrapping `pkg/crypto/pqc/ml_dsa.go` |
| `services/wotan/internal/signing/signing_test.go` | NEW — 6 tests covering round-trip, tamper rejection, algorithm rejection, sender rejection, requirement matching |
| `services/wotan/internal/grpc/topic_service.go` | `PublishTopic()` now enforces ML-DSA-65 signature on any topic matching `config.*` |
| `services/wotan/proto/topic.pb.go` | Regenerated proto bindings |

## Canonical Form

Sign the SHA-256 of `"wotan/topic-sig/v2" ‖ 0 ‖ topic ‖ 0 ‖ sender_id ‖ 0 ‖ signed_at (int64 big-endian, Unix ms) ‖ payload` (v2, 2026-10-19; v1 had no time).
Deterministic, includes sender to prevent identity spoofing.

## Topic Policy

```go
func RequiresSignature(topic string) bool {
    return strings.HasPrefix(topic, "config.")
}
```

Default: `config.*` requires signatures. Other topics (`drift.detected.*`,
`logs.*`, `alerts.*`) currently optional but MAY adopt the same pattern.

## Trusted keys

`config.*` is accepted only over gRPC, signed by a key listed in `topics.signing_keys`. The `public_key` in the request is used only to pick which trusted key to check; a key not on the list is `ErrUntrustedKey` (PermissionDenied). Replays: the request carries `signed_at_unix_ms`, which the signature covers. Wotan refuses it more than 5 minutes from its own clock and refuses a signature it has already accepted within that window (a restart forgets them, so a message signed in the last 5 minutes could be replayed once after one).

## Algorithm Lock

Hard condition: `algorithm == "ml-dsa-65"` ONLY. No algorithm agility.
Blocks downgrade attacks. Testing covers explicit rejection of
`hmac-sha256` substitution.

## Reuse

Sits on top of existing `pkg/crypto/pqc/ml_dsa.go` (60+ tests, circl
v1.6.3 vendored). Zero new external dependencies.

---

> **Source:** [services/wotan/internal/signing/](../services/wotan/internal/signing/) · [pkg/crypto/pqc/ml_dsa.go](../pkg/crypto/pqc/ml_dsa.go) · [ADR-043](../docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md)
