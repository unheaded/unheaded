<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-WIKI-VOR progress

Plan: `docs/battle-plans/S-WIKI-VOR-battle-plan.md` (10 phases, 148 steps).

Started 2026-11-15. Unheaded: worktree `~/tmp/wt/S-WIKI-VOR`, branch `sprint/s-wiki-vor`, cut from develop `65e55858`. vor: `~/tmp/projects/vor`, `main` at `ff4c7e4`. Commits are signed (gpg cached).

Decision D1 (Stevie, 2026-11-12): the vor refresh runs under its own account with a polkit rule, not root.

## Before (Phase 0, live vor on west, read-only)

| Reading | Value |
|---|---|
| `GET /api/topics/nosuchtopicxyz` | 200 (fuzzy) |
| Topics | 2271 |
| Canonical topics | 844 |
| Distinct canonical names | 841 |

Baseline tests: vor `./cmd/vor/... ./internal/registry/...` green; Unheaded `pkg/vorclient`, `pkg/vorquery`, `cmd/wiki-server`, `cmd/zhen-mcp`, `cmd/zhen-rag`, `cmd/zhen-cli`, `cmd/zhen-agent`, `cmd/zhen-agentd`, `cmd/alert-policy` green with `-race`; `scripts/git-hooks/test-post-commit.sh` passes.

## Step log

| Step | Result | Commit |
|---|---|---|
| 1-10 | Phase 0 gate PASS: both trees clean, Go 1.27.3, baselines green, "before" numbers above | `4b28887d` |
| 11-30 | Phase 1 gate PASS (vor). `Registry.GetIn(category, name)`; the mux moved to `newMux` (pure move, own commit); `/api/topics/<name>` is exact (any case) or 404, with `?category=`; `/detail` and `/related` follow. Scratch vor on 20190: unknown name 404, `saml?category=auth` → auth, `?category=security` → security. `go test -race ./...` green | vor `64068d4`, `7d056e2` |
| 31-46 | Phase 2 gate PASS (vor). `SearchWords` and `match=word` on `/api/search`; the default is unchanged; any other `match` is 400. `make test` and `make lint` green; the three commits are `G` | vor `537ea38` |
| 47-62 | Phase 3 gate PASS. `vorclient.TopicIn`, `SearchOptions.Words`; `pkg/repolink` holds the one blob address; the grep for a second copy shows only `pkg/repolink/repolink.go` | `a73b70ce`, `db077677` |
| 63-72 | 4a. `/sheet/<category>/<name>`; search results and `/sheets` link every sheet by category (one call to vor, no per-name fetch); `/sheet/<name>` still answers; the wiki's search asks for `match=word` | `6ef17c5b` |
| 73-78 | 4b. `Page.md` links open `/wiki/Page`; files not edited | `ed1ea8bb` |
| 79-84 | 4c. Links between sheets open the sheet; a link to a sheet vor does not have keeps its text and loses the link; `[[ ]]` untouched | `af1aadfa` |
| 85-92 | 4d and Phase 4 gate PASS. `TestEveryWikiLinkResolves`: **136 dead links with the `Page.md` rewrite switched off, 0 with it** (186 files in `wiki/`). `go test -race ./cmd/wiki-server/...` green | `60dedd73` |

## Differences from the plan's grounding

- **Step 27 as written fails for govan.** `./vor serve` in the vor repository reads `~/.config/vor/sources/unheaded`, a link to `~/tmp/unheaded`, and stops on `configs/certs/dev/akira: permission denied`. The scratch vor was started with `HOME` set to an empty scratch directory, so it loads the built-in sheets only. That is enough for Step 28.
- **Step 42, "gofmt prints nothing".** `gofmt -l cmd internal` in vor lists 12 files under `internal/`, all of them unformatted at `ff4c7e4` already (`internal/registry/registry.go` among them). `gofmt -l cmd` prints nothing and no line this plan added is in gofmt's diff. The 12 are left alone (parked).
- **Step 64, `/sheet/../x` and `/sheet//bash`.** The Go mux cleans both with a 307 before the handler runs (to `/x` and `/sheet/bash`), so they are redirects, not 404s. The test pins the redirect. The escaped forms (`/sheet/%2e%2e/bash`) reach the handler and are 404.
- **4a, a sheet no URL can name.** A category or name with a `/` in it (none among today's 844) cannot be `/sheet/<category>/<name>`. Such a sheet is neither listed nor linked, so every link the server writes opens.
- **4d reads "relative href" as "an address this server has no route for".** The wiki's 129 slug links (`[x](Port-Registry)`) are relative and resolve, so the gate resolves each address against the page's own address and then checks it. An image and a bare `..` are caught (planted in `TestDeadWikiLinks_Finds`).
- **With west's vor still the old binary** (until Phase 8), the new wiki-server asks for `?category=` and the old vor ignores it: `TopicIn` then answers "not found" for the sheet that loses the name, never the other sheet. Unique names open as before.

## Parked

- vor: 12 files under `internal/` are not gofmt-clean (see above).
- vor: `~/.config/vor/sources/unheaded` for govan points at the live working tree, which holds a directory govan cannot read; `vor serve` as govan fails on it.

## Stuck

Nothing yet.

## For Stevie

Phase 8 (install on west) is attended and not started.
