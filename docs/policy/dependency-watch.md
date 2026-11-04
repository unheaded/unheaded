# Dependency watch list

**Rule (native first, CLAUDE.md, S-LOW-FRUIT 2026-10-29).** An outside module is a last resort. Adding one needs Stevie's explicit approval of the "needed to function" claim, recorded in the PR or sprint log before it is pulled in. The third-party count only goes down: `scripts/check-dependency-baseline.sh` refuses additions, `scripts/check-sbom-trend.sh` refuses growth, and every sprint removes at least one row below (with its tests ported).

**Unmaintained is a risk class of its own.** A dead module never gets an advisory. A direct module whose latest release is older than 24 months needs a decision in this file (`keep-pinned`, `replace-by <sprint>` or `vendor`); older than 36 months on an input boundary is a findings-register row (ADR-098). A change of maintainer or organisation ("handover") re-opens the row that sprint.

**Bumps.** Dependabot opens grouped weekly PRs (Go, Rust and Actions wait 14 days, images 7); nothing auto-merges. A bump of a STALE or handover module is merged only after `scripts/dep-diff.sh` output has been read and the PR says what changed and why.

Release dates are from `proxy.golang.org/<module>/@latest`, retrieved 2026-10-29. **Decisions marked "proposed" are Stevie's to confirm** (overlaps S-TRUE-WIRE E5: thresholds, yaml.v3, cilium/ebpf, lib/pq).

## Go — direct modules (18; gorilla/mux removed 2026-10-29)

| Module | Pinned | Latest (date) | Months | Boundary | Why it stays | Native path, cost | Decision |
|---|---|---|---|---|---|---|---|
| gopkg.in/yaml.v3 | v3.0.1 | v3.0.1 (2022-05-27) | **53 · STALE · handover · FND-030** | parses config files (operator input) | ~40 callers; YAML is the config format | upstream repo archived; maintained continuation `go.yaml.in/yaml/v3` (YAML org): a path swap + `dep-diff.sh` read, ~1 h | **proposed: replace-by next sprint** (switch to the maintained module path) |
| github.com/google/uuid | v1.6.0 | v1.6.0 (2024-01-23) | **33 · STALE** | parses UUID strings | 19 files; zero deps, Google-maintained, govulncheck-covered; finished, not dead | own 16-byte type with text/SQL marshalling: crypto-adjacent code of ours | proposed: keep-pinned (Step 90: a rewrite adds risk, doesn't remove it) |
| github.com/gorilla/websocket | v1.5.3 | v1.5.3 (2024-06-14) | **28 · STALE** | parses WebSocket frames from browsers | 3 callers (dashboard live tail, doom-bridge, upc-tty-bridge) | stdlib has no WebSocket; own RFC 6455 framing ≈ 400 lines + fuzzing | proposed: keep-pinned, re-check at 36 months (input boundary → finding then) |
| github.com/BurntSushi/toml | v1.6.0 | v1.6.0 (2025-12-18) | 10 | parses Cargo.toml + the timeline .toml mirror | real TOML needed | own TOML subset parser: not worth it | keep-pinned |
| github.com/anthropics/anthropic-sdk-go | v1.82.0 | v1.84.0 (2026-10-20) | 0 | HTTP client to the Claude API | pkg/claudebridge (ADR-104) | ~200-line client once the API surface we use is stable | replace-by: when the surface settles |
| github.com/cilium/ebpf | v0.22.0 | v0.22.0 (2026-06-26) | 4 | kernel (BPF syscalls, ELF) | the Go side of every eBPF loader | none sensible | keep |
| github.com/cloudflare/circl | v1.6.5 | v1.6.5 (2026-08-05) | 2 | crypto (ML-DSA, SLH-DSA) | PQC; never hand-roll crypto | none | keep |
| github.com/lib/pq | v1.12.3 | v1.12.3 (2026-04-03) | 6 | Postgres wire protocol (the Well) | database/sql driver | none (a driver is protocol code) | keep |
| github.com/rs/zerolog | v1.35.1 | v1.35.1 (2026-04-20) | 6 | none (writes logs) | 39 services; `log/slog` exists | slog migration = a sprint (Marshal fenced it out of this one) | replace-by: logger consolidation sprint |
| github.com/yuin/goldmark | v1.8.6 | v1.8.6 (2026-09-03) | 1 | parses Markdown (wiki/docs, ours) | 1 caller (wiki-server) | own Markdown renderer: a sprint | keep |
| golang.org/x/crypto | v0.58.1-0.20261009181718-2f1f834330b2 | v0.58.0 (2026-10-09) | 0 | crypto | Go team | none | keep; untagged pin for GO-2026-5932 (`openpgp/` removed), back to a tag at the next release |
| golang.org/x/sys | v0.49.0 | v0.49.0 (2026-10-06) | 0 | kernel | Go team | none | keep |
| golang.org/x/text | v0.43.0 | v0.43.0 (2026-10-09) | 0 | none | 4 callers (`cases`/`language`); transitively required by gRPC anyway | drop the direct import when touched; SBOM doesn't shrink | keep (transitive) |
| golang.org/x/time | v0.16.0 | v0.16.0 (2026-08-19) | 2 | none | rate limiting (wotan, protocol-api, zhen-agentd: `NewLimiter`, `Allow`, `Tokens`) | own token bucket ≈ 60 lines, 3 sites; but anthropic-sdk-go's go.mod requires it too, so `go list -m all` (the SBOM count) keeps it until that module goes. Our code is its only importer: removal shrinks the build, not the SBOM | proposed: keep (Go team; the rewrite would sit on DoS-protection paths for no SBOM gain); revisit with anthropic-sdk-go's replace-by |
| google.golang.org/grpc | v1.84.0 | v1.85.0 (2026-10-27) | 0 | network protocol | Wotan transport | none | keep |
| google.golang.org/protobuf | v1.36.12 | v1.36.12 (2026-08-10) | 2 | wire decoding | gRPC messages | none | keep |
| modernc.org/sqlite | v1.61.0 | v1.61.0 (2026-10-21) | 0 | parses SQLite files | kanban fallback store, pure Go (no cgo) | none | keep |
| github.com/unheaded/doomgeneric | v0.0.0 (replace → local path) | ours | — | — | ours | — | keep (ours) |

## Rust — watch rows

`clap`, `chrono`, `anyhow`, `axum` (host tools, not the BPF objects). Counted in `docs/policy/sbom-count-baseline.txt` (`rust_components`); `cmd/depaudit` covers Go modules today (`go list -m -json all | go run ./cmd/depaudit`); crates.io release ages for Rust are not wired yet.

## Python

`tomb/grimoire/rag/requirements.txt` (ChromaDB, transformers, sentence-transformers, torch; 6 open Dependabot alerts): retire vs bump is next.md's open decision (E4). Retiring removes them from `python_components`.
