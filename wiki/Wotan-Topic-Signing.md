# Wotan Topic Signing — ML-DSA-65 on `config.*`

ADR-043 hard condition #2: all messages on `config.*` topics MUST be
ML-DSA-65 signed before [[Mímir's Law|Mimirs-Law]] spike begins.
Implemented as a prerequisite sprint on `main` before the spike branch.

> **Status**: SHIPPED 2026-04-11 (commit `5e0d0400`)

## What Got Wired

| File | Change |
|---|---|
| `services/wotan/proto/topic.proto` | Added `signature`, `public_key`, `algorithm` fields to `TopicPublishRequest` and `TopicEvent` |
| `services/wotan/internal/signing/signing.go` | NEW — `TopicSigner` + `TopicVerifier` wrapping `pkg/crypto/pqc/ml_dsa.go` |
| `services/wotan/internal/signing/signing_test.go` | NEW — 6 tests covering round-trip, tamper rejection, algorithm rejection, sender rejection, requirement matching |
| `services/wotan/internal/grpc/topic_service.go` | `PublishTopic()` now enforces ML-DSA-65 signature on any topic matching `config.*` |
| `services/wotan/proto/topic.pb.go` | Regenerated proto bindings |

## Canonical Form

Sign the SHA-256 of `topic ‖ "\x00" ‖ sender_id ‖ "\x00" ‖ payload`.
Deterministic, includes sender to prevent identity spoofing.

## Topic Policy

```go
func RequiresSignature(topic string) bool {
    return strings.HasPrefix(topic, "config.")
}
```

Default: `config.*` requires signatures. Other topics (`drift.detected.*`,
`logs.*`, `alerts.*`) currently optional but MAY adopt the same pattern.

## Algorithm Lock

Hard condition: `algorithm == "ml-dsa-65"` ONLY. No algorithm agility.
Blocks downgrade attacks. Testing covers explicit rejection of
`hmac-sha256` substitution.

## Reuse

Sits on top of existing `pkg/crypto/pqc/ml_dsa.go` (60+ tests, circl
v1.6.3 vendored). Zero new external dependencies.

---

> **Source:** [services/wotan/internal/signing/](../services/wotan/internal/signing/) · [pkg/crypto/pqc/ml_dsa.go](../pkg/crypto/pqc/ml_dsa.go) · [ADR-043](../docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md)
