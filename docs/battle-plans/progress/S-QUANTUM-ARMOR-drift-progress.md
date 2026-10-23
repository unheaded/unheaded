# S-QUANTUM-ARMOR: Phase 12 (signed drift events) progress

Plan: `docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md`, Phase 12 only
(Steps 227-248). Branch based on develop `424edf47`. Run in its own
worktree, in parallel with Phases 0-4 (ADR-105) and 10-11 (Gungnir
evidence seals); this track does not edit their paths.

Skip Protocol: I will skip any step after 3x its estimate or 2 failed debug
attempts, mark it [STUCK] with what I tried, commit known-good state first,
continue with the next step that does not depend on it, and finish with a
STUCK/ESCALATE report ordered by how many steps each item unblocks.
[ESCALATE] steps are never attempted; they are reported.

## Baseline (before any change)

`go test -race -count=1` on pqc, gungnir, wotan-client (+mock), every
`services/wotan` package and `cmd/heimdall-daemon`: 21 ok, 0 FAIL.

## Step log

- **227 [R]** Read. Finding: Wotan never delivered signatures to
  subscribers. `TopicEvent` had `signature`/`public_key`/`algorithm`
  fields (9-11) but nothing set them: the verified `Signed` was used for
  the replay cache and dropped; `ringbuffer.Message` had nowhere to keep
  it. `pqc` had no constructor from an existing key. Proto generator
  versions in the headers are protoc v3.21.12, protoc-gen-go v1.36.11,
  protoc-gen-go-grpc v1.6.1 (the plan's "v6.33.4 / v1.6.0" is wrong; the
  rule "unchanged" is what counts), and those exact versions are
  installed locally.
- **228 [TEST]** Golden vector `cba2c4b1…2b76` for
  `canonicalize("config.x","sender-1",1700000000000,"payload")`, captured
  from the internal code and computed independently (Python SHA-256 over
  the documented layout). Green before the move.
- **229 [CODE]** `pkg/wotansig` (Canonicalize, Signed, TopicSigner,
  CheckKey, VerifySignature, VerifyEvent) + `pqc.NewMLDSASignerFromPrivateKey`.
  `services/wotan/internal/signing` now delegates; its API and error
  values are unchanged (the error texts moved with them). One behaviour
  difference: a malformed signature or key (wrong length) used to return
  pqc's size error; it now returns `ErrInvalidSignature` (bytes from the
  network failing verification are a verification failure).
- **230 [TEST]** Golden test unchanged and green in both packages;
  wotansig, signing, pqc, wotan grpc green with -race. `FuzzVerifyEvent`
  45 s: no crasher, nothing accepted that the key did not sign.
- **231 [C]** `ac963d8e`.
- **232 [TEST]** `signing/policy_test.go`, `api/signed_prefixes_test.go`,
  `grpc/topic_service_drift_test.go`: every case the plan lists. Red first
  (build failures on the missing API).
- **233 [CODE]** `TopicEvent.signed_at_unix_ms = 12`. `generate.sh` with
  the installed pinned tools: `topic.pb.go` header unchanged (protoc
  v3.21.12, protoc-gen-go v1.36.11), diff = the new field only.
  `generate.sh` also rewrites `chat.pb.go`/`chat_grpc.pb.go`, which were
  last generated with protoc v6.33.4 / protoc-gen-go-grpc v1.6.0 (so the
  plan's version note described the chat files); those two were restored
  untouched, not committed.
- **234 [CODE]** `signing.Policy` (built-in `config.` + `drift.`,
  longest-prefix match, per-prefix `TopicVerifier`), `topics.signed_prefixes`
  in `api.TopicConfig` (validated at load: prefix syntax, duplicates,
  `config.` refused, keys must be base64 ML-DSA-65 public keys, unknown
  fields refused), HTTP refuses every signed prefix, gRPC `PublishTopic`
  verifies with the prefix's verifier, the verified signature is stored on
  the ring-buffer message and copied onto every `TopicEvent` (replay and
  live). This is a fix: before, no subscriber ever received a signature.
  `configs/wotan.yaml` ships `drift.` with `keys: []` (fail closed until E5).
- **Choice ADR-105 must record (sender):** the canonical form covers a
  sender string. For a publisher authorized by `display_name` (no
  `sender_id`, how every gRPC client publishes), Wotan used to verify
  against `""` and then store the message under a server-made member
  UUID, so no subscriber could rebuild what was signed. Now the signed
  sender is `sender_id` if given, else the display name Wotan authorized
  (`"service"` by default), and a signed event's `sender_id` is that
  string. Binds the signature to the authorized identity. No production
  `config.*` signer existed (grep), so the only change outside Phase 12
  was one existing test now signing `"service"` instead of `""`.
- **235 [TEST]** All `services/wotan`, `pkg/wotan-client`, `pkg/wotansig`
  packages ok with -race (19 ok, 0 FAIL; baseline 0 FAIL).
  `FuzzLoadTopicConfig` 45 s: no crasher. golangci-lint: 0 issues.
