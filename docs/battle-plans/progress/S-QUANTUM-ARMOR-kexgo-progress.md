# S-QUANTUM-ARMOR progress: track KEX-Go (Phases 5-6)

Plan: `docs/battle-plans/S-QUANTUM-ARMOR-battle-plan.md`. Worktree off develop `16bbc3e4`.
Commits `--no-gpg-sign` (lead re-signs at merge). Skip Protocol acknowledged; no step
was skipped, nothing is STUCK.

## Plan amendments (Marshal Pattern 6)

- Go runs with a scratch `GOWORK` that points go.mod's doomgeneric replace at the real
  sibling checkout (it does not resolve inside a worktree). No tracked file changed for it.
- Step 105: the Wotan helpers `httpTLSConfig` / `grpcTLSConfig` live in a new
  `services/wotan/cmd/wotan/tls.go`, so `main.go` changes only in its TLS block (Phase 12
  edits the same file). Step 126 reverts `tls.go` instead of `main.go`.
- Step 107: `pkg/wotan-client.TLSConfig()` is exported so the Wotan gRPC test dials with
  the client's own creds builder.
- `ApplyStrict` / `ApplyPreferred` return the config, so sites read
  `pqtls.ApplyStrict(&tls.Config{...})`. A nil config gets a new TLS 1.3 one.
- Load balancer: unset `min_version` is now 1.3 and `Builder.TLS` defaults to "1.3";
  `min_version`/`max_version` "1.2" stay operator opt-ins, re-applied after
  `ApplyPreferred`. Their two baseline entries are accepted in the baseline header.
- Before Step 119 (from the Phase 0-4 notes): the Go scanner learned the profile calls
  (`4debe926`), with fixtures `m_apply.go` and `pkg/tls/n_bare.go`, red then green.
- Phase 6 provocation, beyond Step 126: `GODEBUG=tlsmlkem=0,tlssecpmlkem=0` makes the
  Preferred surfaces red (it only changes Go's default list); `StrictKEX()` temporarily
  returning `[X25519]` makes every Strict surface red. Both reverted, green with `-race`.

## Phase 5 (Steps 96-122): done

Negotiated group per surface (before -> after; CBOM status after):

| Surface | Before | After |
|---|---|---|
| Wotan HTTP (`-enable-tls`) | X25519 (pin X25519,P-256 turned the hybrid default off) | X25519MLKEM768, X25519 fallback (hybrid, Preferred) |
| Wotan gRPC | Go defaults, TLS 1.2 floor (hybrid-opportunistic) | X25519MLKEM768 only, classical refused (hybrid-strict) |
| wotan-client gRPC TLS | TLS 1.3, default groups (hybrid) | hybrid only (hybrid-strict) |
| pkg/transport/mtls (4 configs) | TLS 1.2 floor (hybrid-opportunistic) | hybrid-strict |
| pkg/mesh (server, client) | TLS 1.2 floor | hybrid-strict |
| pkg/mesh/mtls Manager + Provider | floor from a variable (unknown) | hybrid-strict |
| LB frontends | TLS 1.2 floor | TLS 1.3, hybrid (1.2 = operator opt-in) |
| pkg/http fallback TLS | empty Config (1.2 floor) | TLS 1.3, hybrid |
| LXD REST client | TLS 1.3 (hybrid) | unchanged, now via ApplyPreferred |

- PQ-KEX baseline 13 -> 6. Cleared: `services/wotan/cmd/wotan/main.go curve-pin-classical`,
  `services/wotan/cmd/wotan/main.go tls-floor-below-1.3`, `pkg/http/server.go`,
  `pkg/mesh/mesh.go`, `pkg/mesh/mtls/mtls.go`, `pkg/mesh/mtls/provider.go`,
  `pkg/transport/mtls/mtls.go` (all `tls-floor-below-1.3`). Kept, accepted:
  `pkg/loadbalancer/config.go tls-floor-below-1.3` and `tls-max-below-1.3`. The other four
  belong to Phases 7 and 8.
- Tests that encoded the 1.2 floor were changed to 1.3 (transport/mtls 3, mesh/mtls 3).
- Step 120: `go test -count=1 ./...` 276 ok, 1 FAIL = `pkg/wotan-client
  TestGRPCClient_GetMessages`, a pre-existing flake (plaintext bufconn; 2/20 at the base
  `grpc_client.go`, 1/20 with this change).
- Exit gate: no `CurvePreferences` outside `pkg/tls/pq.go` (the scanner's own source only
  names the field); `check-pq-kex` PASS (6); `check-crypto-inventory` PASS. GATE5_OK.
- DoD: gosec ratchet PASS; gosec, golangci-lint and govulncheck clean on every changed
  package (see below for `cmd/crypto-inventory`).

## Phase 6 (Steps 123-140): done

- Hybrid-asserting tests: pkg/tls 8, wotan 3, transport/mtls 3, mesh 1, mesh/mtls 3,
  loadbalancer 3, http 2, unheaded-daemon 1. Strict surfaces also test that an X25519-only
  peer is refused.
- Step 126: re-pinning X25519/P-256 in `httpTLSConfig` -> FAIL naming X25519; reverted.
- Step 135, OpenSSL 3.5.3 against a loopback Wotan (28000/28001): HTTP hybrid offer
  X25519MLKEM768, classical offer X25519; gRPC hybrid offer X25519MLKEM768, classical
  offer refused (alert 40). Wotan stopped by PID, scratch key removed.
- `scripts/probe-tls-groups.sh` (NOT_A_GATE) reproduces the probe.
- Exit gate: `go test -race -run 'PQ|Hybrid|Handshake' ./pkg/... ./services/wotan/...
  ./cmd/unheaded-daemon/...` -> 0 FAIL. Meta-gate `--quick` on the clean worktree: every
  runnable gate bites (incl. check-pq-kex, check-crypto-inventory); rc=1 only from
  check-ruff / check-python-audit INCONCLUSIVE (tools not installed on west, as Phases 0-4
  recorded).

## For the lead

- `cmd/crypto-inventory` (Phases 3-4 code) has pre-existing findings that CI's gosec
  (`./...`, no exclusions) and golangci-lint will report: G204/G702 (`tree.go` git exec),
  G703 (`tree.go`, `main.go`, `lint.go`, `host.go` file paths), G101 false positive on
  `kindGRPCCreds`, G402 on the `testdata/go/cmd/lich-security/probe.go` fixture, and
  staticcheck QF1001 at `lint.go:254`. Not fixed here: they are outside Phases 5-6 and in
  files other tracks may touch.
- The baseline and CBOM changed here; regenerate both at merge.
