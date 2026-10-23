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
