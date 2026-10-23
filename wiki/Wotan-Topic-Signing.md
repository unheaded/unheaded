# Wotan Topic Signing — ML-DSA-65 on `config.*` and `drift.*`

ADR-043 hard condition #2: all messages on `config.*` topics MUST be
ML-DSA-65 signed before [[Mímir's Law|Mimirs-Law]] spike begins.
Implemented as a prerequisite sprint on `main` before the spike branch.

> **Status**: SHIPPED 2026-04-11 (commit `5e0d0400`); **fixed 2026-10-19**: until then the signature was checked against the public key the publisher sent in the same request, so anyone could sign with a key of their own and pass, and the HTTP publish path took `config.*` without any signature. Now Wotan verifies only against the keys in `configs/wotan.yaml` → `topics.signing_keys` (base64 ML-DSA-65 public keys; empty = every `config.*` publish refused), and both HTTP write paths refuse `config.*`.
>
> **2026-10-23 (ADR-105, S-QUANTUM-ARMOR Phase 12)**: `drift.*` is signed too, each signed prefix by its own keys (`topics.signed_prefixes`), and Wotan now delivers the signature with every signed event so subscribers can re-verify it. Before, Wotan checked `config.*` signatures and then dropped them: no subscriber ever received one. The canonical form moved to the importable `pkg/wotansig` unchanged (golden vector).

## What Got Wired

| File | Change |
|---|---|
| `services/wotan/proto/topic.proto` | Added `signature`, `public_key`, `algorithm` fields to `TopicPublishRequest` and `TopicEvent` |
| `services/wotan/internal/signing/signing.go` | NEW — `TopicSigner` + `TopicVerifier` wrapping `pkg/crypto/pqc/ml_dsa.go` |
| `services/wotan/internal/signing/signing_test.go` | NEW — tests covering round-trip, tamper rejection, algorithm rejection, sender rejection, requirement matching, untrusted keys, replay |
| `services/wotan/internal/grpc/topic_service.go` | `PublishTopic()` now enforces ML-DSA-65 signature on any topic matching `config.*` |
| `services/wotan/proto/topic.pb.go` | Regenerated proto bindings |
| `pkg/wotansig/` | 2026-10-23: canonical form, `TopicSigner`, stateless `VerifyEvent` for subscribers (importable; `internal/signing` delegates) |
| `services/wotan/internal/signing/policy.go` | 2026-10-23: per-prefix trusted keys, longest prefix wins |
| `TopicEvent.signed_at_unix_ms` (field 12) | 2026-10-23: with `signature`, `public_key`, `algorithm` (9-11), now populated on every signed event |

## Canonical Form

Sign the SHA-256 of `"wotan/topic-sig/v2" ‖ 0 ‖ topic ‖ 0 ‖ sender_id ‖ 0 ‖ signed_at (int64 big-endian, Unix ms) ‖ payload` (v2, 2026-10-19; v1 had no time).
Deterministic, includes sender to prevent identity spoofing.

The sender is the publish request's `sender_id`, or, for a publisher that
sent none (every gRPC client: it is authorized by `display_name` against
`topics.auto_approve`), that display name (`"service"` when absent). A signed
event's `sender_id` is that same string.

## Topic Policy

Built-in signed prefixes: `config.` and `drift.`. More can be added in
`topics.signed_prefixes`. Each prefix trusts only its own keys:

```yaml
topics:
  signing_keys: []            # config.*
  signed_prefixes:
    - prefix: "drift."        # Heimdall drift alerts, drift.detected.<node_id>
      keys: []                # base64 ML-DSA-65 public keys; empty = closed
```

A `config.*` key verifies nothing under `drift.` and a drift key verifies
nothing under `config.` (ADR-043 #4 key separation). The longest matching
prefix decides. Wotan refuses to start on a broken entry (no trailing dot,
wildcard, duplicate, a `config.` prefix, a key that is not a base64 ML-DSA-65
public key, an unknown field). HTTP refuses every signed prefix. `logs.*`,
`alerts.*` and the rest stay unsigned.

## Subscriber verification

Every signed `TopicEvent` carries `signature`, `public_key`, `algorithm` and
`signed_at_unix_ms`. A subscriber that trusts a key re-verifies with nothing
else, so it does not have to trust Wotan to relay faithfully:

```go
sg := wotansig.Signed{Signature: ev.Signature, PublicKey: ev.PublicKey,
    Algorithm: ev.Algorithm, SignedAtUnixMs: ev.SignedAtUnixMs}
err := wotansig.VerifyEvent(ev.Topic, ev.SenderId, ev.Payload, sg, trustedDriftKeys)
```

Through `pkg/wotan-client`: `msg.VerifySignature(trusted)`. Publishers use
`PublishSigned(ctx, topic, payload, signer)` (gRPC only). `VerifyEvent` has no
clock and no replay cache: freshness and replay are Wotan's checks at publish
time.

## Heimdall drift events

`heimdall-daemon --wotan-grpc ADDR --drift-sign-key SEED` publishes each drift
alert to `drift.detected.<node_id>` (node ID normalized to `[a-z0-9-]`) as
display name `heimdall`, signed with a key that must differ from the manifest
keys (`--sign-key`, `--pubkey`). Body: `node_id, path, kind, expected_sha256,
actual_sha256, detected_at, manifest_name, manifest_version`. Without
`--wotan-grpc` it prints alerts only. Running it for real (a drift key per
node, its public key in `signed_prefixes`, `heimdall` on the allowlist, a unit
file) is open item E5 for Stevie.

## Trusted keys

`config.*` is accepted only over gRPC, signed by a key listed in `topics.signing_keys`. The `public_key` in the request is used only to pick which trusted key to check; a key not on the list is `ErrUntrustedKey` (PermissionDenied). Replays: the request carries `signed_at_unix_ms`, which the signature covers. Wotan refuses it more than 5 minutes from its own clock and refuses a signature it has already accepted within that window (a restart forgets them, so a message signed in the last 5 minutes could be replayed once after one).

## Algorithm Lock

Hard condition: `algorithm == "ml-dsa-65"` ONLY. No algorithm agility.
Blocks downgrade attacks. Testing covers explicit rejection of
`hmac-sha256` substitution.

## Reuse

Sits on top of existing `pkg/crypto/pqc/ml_dsa.go` (`github.com/cloudflare/circl`
v1.6.5 in `go.mod`; not vendored). Zero new external dependencies.

`configs/wotan.yaml` ships `signing_keys: []` and `drift.` with `keys: []`,
so out of the box every `config.*` and `drift.*` publish is refused.

---

> **Source:** [services/wotan/internal/signing/](../services/wotan/internal/signing/) · [pkg/wotansig/](../pkg/wotansig/) · [cmd/heimdall-daemon/drift.go](../cmd/heimdall-daemon/drift.go) · [pkg/crypto/pqc/ml_dsa.go](../pkg/crypto/pqc/ml_dsa.go) · [ADR-043](../docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md)
