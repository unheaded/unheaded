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
- **236 [C]** `50a425cc`.
- **237-238** `PublishSigned` on `GRPCClient` (the gRPC transport) and on
  `Client` (returns `ErrSignedTopicNeedsGRPC` in HTTP-only mode, never
  queues or falls back to HTTP). Put in a new file
  `pkg/wotan-client/publish_signed.go`, not `grpc_client.go`, to stay
  clear of Phase 5's edit at `grpc_client.go:59`; `grpc_client.go` only
  gains one line in `topicEventToMessage`. It publishes under the joined
  display name and signs for it (the S17 d80139c6 rule). Also: `Message`
  carries `Signed` and `VerifySignature(trusted)`, so a subscriber reading
  through wotan-client can re-verify (the phase goal; not a listed step).
- **239-240** Heimdall: `--wotan-grpc`, `--drift-sign-key`,
  `--publish-queue` (256). Body `{node_id, path, kind, expected_sha256,
  actual_sha256, detected_at, manifest_name, manifest_version}`; node IDs
  normalized to `[a-z0-9-]` (fuzzed: always exactly one topic segment);
  bounded drop-oldest queue, `Enqueue` never blocks; counters
  `heimdall_drift_{published,publish_failed,publish_dropped}_total`
  (pkg/metrics counters; Heimdall has no listener, and adding one is out
  of scope, so they are not scraped yet); key separation against
  `--sign-key` and `--pubkey` by gungnir key ID (checked before the
  `--sign-key` branch too); a group-readable key is refused
  (`ErrKeyFilePerms`); `TestNoRestorePath` (go/ast) fails on any
  os write/rename/chmod/remove outside `signManifest`, provoked once with
  a throwaway `os.Rename` file to prove it can fail. Without
  `--wotan-grpc` behaviour is unchanged plus a startup line saying drift
  is not published; `--wotan-grpc` without `--drift-sign-key` is refused.
  Signed sender is the constant display name `heimdall`; the node is in
  the topic and the per-node key. Missing-file alerts now carry the
  expected hash.
- **241 [TEST]** heimdall-daemon + wotan-client green with -race;
  `FuzzNormalizeNodeID` 30 s clean; golangci-lint 0 issues.
- **242 [C]** `bbdf7559`.
- **243 [TEST]** `services/wotan/internal/grpc/drift_e2e_test.go`. Deviation:
  a loopback `127.0.0.1:0` listener inside the test process, not
  `bufconn`, because `wotanClient.NewGRPCClient` dials an address and
  takes no dialer (adding a test-only constructor to the client API was
  not worth it). The L12b refusals also assert the error names the
  signature check, so an allowlist refusal cannot make them pass.
- **244 [TEST]** `TestDriftE2E_SubscriberReverifies`,
  `TestLICH012_L12b_ForgedDriftRefused`,
  `TestLICH012_L12b_ReplayedDriftRefused`: PASS with -race.
- **245 [DOC-UPDATE]** LICH-012 results row for signed drift;
  `wiki/Wotan-Topic-Signing.md` (signed prefixes, scoped keys, sender rule,
  subscriber verification, Heimdall flags).
- **246 [ESCALATE] E5 — not attempted.** Enrol a drift key per node (0600
  seed, separate from the E4 build key and the manifest key), add its
  public key to `drift.`'s `keys` in the live Wotan config, add
  `heimdall` to `topics.auto_approve`, ship a unit for Heimdall (ORPHAN in
  `docs/LIVE-PATHS.md`). Key custody (ADR-043 #4) and a new root-reading
  daemon on the hosts are Stevie's calls. Until then `drift.*` is closed.

- **247 [C]** `a428e186`.
- **248 [V] PHASE 12 EXIT GATE: PASS.**
  `grep -c 'TODO: wire to actual Wotan client' cmd/heimdall-daemon/main.go`
  → `0`; `go test -count=1 -run 'Golden|DriftE2E|LICH012' ./pkg/wotansig/
  ./services/wotan/...` FAIL count → `0`; `topic.pb.go` header still
  protoc v3.21.12 / protoc-gen-go v1.36.11, proto diff = field 12 only;
  E5 logged above.

## DoD (Phase 12)

- Every step executed, E5 [ESCALATE] logged, nothing [STUCK].
- Regression: the baseline package set plus `pkg/wotansig` with -race:
  22 ok, 0 FAIL (baseline 21 ok, 0 FAIL).
- `go vet` clean and `gofmt -l` empty on every touched package/file;
  golangci-lint 0 issues; gosec (the repo's pinned `tools/gosec` build,
  as CI runs it) 0 issues on the touched packages, ratchet PASS;
  govulncheck: 0 vulnerabilities reachable from our code.
  (`go build ./...` from this worktree fails on `cmd/wotan-ctl`'s
  `../projects/doomgeneric` replace, which resolves relative to the
  worktree; environmental and unrelated.)
- No new dependency (circl, yaml.v3, grpc were already in go.mod); no new
  production listener; test keys are generated in memory or in
  `t.TempDir()`; no key printed or committed; `configs/wotan.yaml` trusts no
  drift key.
- Monad v0x01 wire format untouched: the one protocol change is an
  additive field in Wotan's gRPC `TopicEvent` (RFC Editor not required).
- ADR-043 #1 (no restore path) and #4 (key separation, per-prefix keys)
  asserted by tests.

## Parked (seen, not done here)

- Cluster replication (`replication.proto`) does not carry message
  signatures, so an event replayed from a peer node arrives unsigned.
  Additive later; nothing replicates drift today.
- The ring buffer's byte budget counts `Content` only; a signed message
  also holds ~5.3 KB of signature and key. Signed rooms are rare and
  protected from eviction (`SetKeep`).
- Heimdall's counters are not scraped: it has no HTTP listener, and adding
  one is out of this phase.
- `TopicStreamClient` has no `PublishSigned` (Heimdall uses `GRPCClient`).
