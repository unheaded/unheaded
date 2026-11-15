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
| 93-114 | Phase 5 gate PASS. The six hand-built call sites (zhen-agentd, zhen-agent, zhen-cli, zhen-mcp, zhen-rag, `pkg/vorquery`) read vor through `pkg/vorclient`; the Step 93 grep is empty outside comments. A topic is read for the category its hit names (`vorclient.TopicFor`). Retrieval before and after on west's vor, `zhen-mcp` `corpus_search`, five questions: see "Retrieval before and after" | `44e2519c` |
| 115-124 | `vor-export-source.sh <dest> [ref]` with the hash in `.vor-export` and the stamp `<dest>.exported` written last by rename; the hook exports develop. Both test scripts green (T1-T5, T1-T7). The live export on west was not touched (still 2026-11-12 06:43Z) | `31d0577d` |
| 125-135 | Phase 6 gate PASS. uid 16758 rechecked free in `pkg/uids`, sysusers, west's and east's passwd, then taken for `unheaded-vor-refresh`; polkit rule; path and service units. `systemd-analyze verify` clean; `check-systemd-units.sh` 51 units, `check-unit-ceilings.sh` OK | `25032ed7` |
| 136-140 | Phase 7 gate PASS. `wiki/Microservices.md`, `wiki/Zhen-AI.md`, `deploy/systemd/README.md`; link gate green, JS lint 0 | `463a1766` |

## Differences from the plan's grounding

- **Step 27 as written fails for govan.** `./vor serve` in the vor repository reads `~/.config/vor/sources/unheaded`, a link to `~/tmp/unheaded`, and stops on `configs/certs/dev/akira: permission denied`. The scratch vor was started with `HOME` set to an empty scratch directory, so it loads the built-in sheets only. That is enough for Step 28.
- **Step 42, "gofmt prints nothing".** `gofmt -l cmd internal` in vor lists 12 files under `internal/`, all of them unformatted at `ff4c7e4` already (`internal/registry/registry.go` among them). `gofmt -l cmd` prints nothing and no line this plan added is in gofmt's diff. The 12 are left alone (parked).
- **Step 64, `/sheet/../x` and `/sheet//bash`.** The Go mux cleans both with a 307 before the handler runs (to `/x` and `/sheet/bash`), so they are redirects, not 404s. The test pins the redirect. The escaped forms (`/sheet/%2e%2e/bash`) reach the handler and are 404.
- **4a, a sheet no URL can name.** A category or name with a `/` in it (none among today's 844) cannot be `/sheet/<category>/<name>`. Such a sheet is neither listed nor linked, so every link the server writes opens.
- **4d reads "relative href" as "an address this server has no route for".** The wiki's 129 slug links (`[x](Port-Registry)`) are relative and resolve, so the gate resolves each address against the page's own address and then checks it. An image and a bare `..` are caught (planted in `TestDeadWikiLinks_Finds`).
- **With west's vor still the old binary** (until Phase 8), the new wiki-server asks for `?category=` and the old vor ignores it: `TopicIn` then answers "not found" for the sheet that loses the name, never the other sheet. Unique names open as before.

- **Phase 5 is one commit, not three.** `vorquery.NewIndex` changes its signature at Step 95, so zhen-agent, zhen-cli, zhen-mcp and zhen-rag do not build until Step 107. Three commits would have left two that do not build.
- **`TopicIn` cannot name a repository file's category.** vor refuses a `/` in `?category=` (Step 18) and `TopicIn` refuses the same (Step 49), and a repository file's category is its directory (`docs/adr`, or `.` at the top). Step 98's "`topic` → `TopicIn(hit.Category, hit.Topic)`" would have dropped every repository doc from Zhen's retrieval. `vorclient.TopicFor` asks by category when the category is one segment and by name alone otherwise, which is today's behaviour for those. Two repository files of one name in two directories are therefore still one name to vor. Allowing `/` in vor's `?category=` would close that; it is a change to Steps 18 and 49 and is parked for Stevie.
- **`vorquery.Select` dedups by name**, unchanged as the plan says. When a search returns both `auth/saml` and `security/saml`, the first hit's sheet is read and the second never is.
- **zhen-cli `/source <topic>` is exact now** (it used to get vor's nearest match). A near miss says "topic not found".
- **Step 121 as written would have skipped the export when `bin/zhen-codeindex` is missing** (the hook exits early without it). The hook now runs each job on its own condition.
- **The export takes a lock** (on the destination's parent directory, no lock file) and swaps with `mv -T`. Two commits in a row start two hooks; without it the second could move its tree inside the first's. Six concurrent exports into a scratch directory leave one whole export and the newest hash in the stamp.
- **The hook test sets `VOR_EXPORT_DEST` for every case.** Unset, the hook's default is the live export, and the test repository copies the exporter in: T1 would have replaced west's corpus with a test tree.
- **`make test-hooks` runs the new export test** (one line in the Makefile, not in Step 124's list), so something runs it.
- **The Kingdom services tier is full.** 16758 was its last free number (16759 is `all-in-one-dev`). The comment in `pkg/uids` names the next free ones: 16764 on (Gnostic tier) and 16714-16719. S-ZHEN-FRONT planned 16758 for `unheaded-zhen-app` and must take another.
- **`wiki/Systemd-Units.md` is not edited.** Step 138 adds the refresh units after Phase 8 installs them, and the `unheaded-vor.service` row is S-RESIDUE's (D-28, running in parallel).
- **`docs/security/nosec-baseline.txt`: `cmd/zhen-mcp/tools.go` 2 → 1** by hand, in the Phase 7 commit. The hand-written vor GET and its `#nosec` are gone, and `--update` refuses to run while other files are above their baseline (below).

## Retrieval before and after (Step 112-113, west's vor, still the old binary)

`zhen-mcp` built at `b3828cc3` (before) and at the Phase 5 tree (after), `corpus_search` with k=5:

| Question | Before | After |
|---|---|---|
| what is wotan | wiki/Service-Wotan, docs/battle-plans/tools/wotan-cloud-battle-plan, docs/protocol/WOTAN_TOPICS, wiki/Wotan-Memory-Model, wiki/Wotan-Topic-Signing | same |
| getting the ccna | auth/kerberos | same |
| jncia ospf areas | juniper/junos-monitoring, juniper/junos-routing-fundamentals | same |
| saml | security/saml, ramp-up/saml-eli5 | same |
| sbom | sbom/SBOM, docs/archive/.../S37-LICENSE-SBOM-BATTLE-PLAN, **sbom/SBOM again**, sbom-results/SBOM-CONSOLIDATED, sbom-results/SBOM-SUMMARY | sbom/SBOM, S37-LICENSE-SBOM-BATTLE-PLAN, sbom-results/SBOM-CONSOLIDATED, sbom-results/SBOM-SUMMARY, docs/policy/sbom-review-rulings |

The second `sbom/SBOM` was the hit `security/sbom` answered with the text of `sbom/SBOM`. After, that hit is read for its own category; west's old vor ignores the category, so the read is refused and the slot goes to the next topic. With the new vor (Phase 8) `security/sbom` itself is read. `saml` is unchanged because the hit, `security/saml`, is also the sheet that wins the name. `getting the ccna` still returns the Kerberos sheet here: `corpus_search` reads vor's order without `vorquery.Select`'s word check, and the Zhen callers stay on substring search (plan, Marshal).

## Found, not this plan's

- **`scripts/check-nosec-ratchet.sh` is red on develop** for five files above their baseline: `cmd/dashboard-backend/internal/server/verdicts.go` (1, baseline 0), `cmd/dashboard-backend/internal/verdicts/verdicts_test.go` (1, 0), `cmd/logstore-writer/main.go` (1, 0), `pkg/bff/nav.go` (1, 0), `cmd/wiki-server/main.go` (4, 1). `cmd/wiki-server/main.go` had the same count at `65e55858`; this plan added no suppression.
- **West has two copies of `unheaded.conf`:** `/etc/sysusers.d/unheaded.conf` (2026-11-10) and `/usr/lib/sysusers.d/unheaded.conf` (2026-11-11). The one in `/etc` wins. Step 141 installs to `/usr/lib`, which would change nothing on west.

## Parked

- vor: 12 files under `internal/` are not gofmt-clean (see above).
- vor: `~/.config/vor/sources/unheaded` for govan points at the live working tree, which holds a directory govan cannot read; `vor serve` as govan fails on it.

## Stuck

Nothing yet.

## For Stevie

Phase 8 (install on west) is attended and not started. Before it:

1. **The start limit and a burst of commits.** The plan's unit starts at most 6 times in 10 minutes. Tried in the user manager on west (systemd 257) with a scratch unit: at the next start inside the window the path unit goes to `failed (unit-start-limit-hit)` and stops watching until `reset-failed`. Seven commits or merges on develop in ten minutes do that, and vor then stays on the old export with nothing saying so. The unit is written as the plan specifies and its header and `deploy/systemd/README.md` say how to reset it. Choices: leave it and alert on the failed path unit; raise the burst; or have the service wait some seconds before the restart so a burst becomes one restart. Which one is yours (the limit is part of D1's reasoning).
2. **Step 141's path.** Install `unheaded.conf` to `/etc/sysusers.d/` on west, or remove the older copy there (see "Found").
3. **The hook is live on merge.** `.git/hooks/post-commit` and `post-merge` are links into the checkout, so once this branch is on develop every commit and merge there exports develop to `/var/zhen/vor-source/unheaded` as govan. Until Phase 8 installs the path unit, vor is not restarted and keeps serving the export it started with.
4. **vor commits are local.** `~/tmp/projects/vor` `main` is 3 ahead of origin (`64068d4`, `7d056e2`, `537ea38`); push them with Unheaded's.
5. **Whether vor's `?category=` should take a directory** (`docs/adr`), so same-named repository files can be told apart (see "Differences").
