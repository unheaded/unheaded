<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-WIKI-VOR BATTLE PLAN: 10 Phases, 142 Steps

**Date**: 2026-11-12
**Sprint**: S-WIKI-VOR, bundle 2 of `docs/sweeps/BACKLOG-SWEEP-2026-11-12.md` (rows D-09 to D-12). The goal is one vor client, exact topic lookups, wiki links that resolve, and a vor corpus that follows develop.
**Prerequisite**: west is up; vor answers on `127.0.0.1:20110`; both repositories are clean (`~/tmp/unheaded` on `develop`, `~/tmp/projects/vor` on `main`). The code phases (1-7) need no answer from Stevie. Phase 8 is attended, because it installs binaries and units on west with sudo.
**Target**: every sheet built into vor is reachable from the wiki under its own category, and an unknown topic is a 404 everywhere. No wiki page links to a page that 404s. No Go code outside `pkg/vorclient` builds a vor URL by hand. A commit on develop is searchable in vor within a minute, with no manual step.
**Estimated duration**: 6-8 h over 2 sessions (Phases 1-7 unattended, about 5 h; Phase 8 attended, about 45 min; Phase 9, 15 min).
**Agent strategy**: Phases 1-2 (vor repo) run before Phase 3 (the client depends on the new API). Phases 4 and 5 can run in parallel after Phase 3; they touch different files. Phase 6 is independent and can run any time after Phase 0. Phase 7 follows 4-6. Phase 8 is the coordinator with Stevie present.
**Commit cadence**: every 5 steps, at every exit gate, and before any skip. Commits are signed and stage named files only (never `git add -A`). Each repository commits its own changes. Stevie pushes both.
**Stuck protocol**: skip after 3× the step estimate or two failed debug attempts, mark the step `[STUCK]` and its dependents `[BLOCKED]`, and keep going on independent phases.

## Grounding (read before Step 1)

Checked on 2026-11-12 against `develop` `a31be33b` and vor `main` `ff4c7e4` (signed), with live reads of west's vor.

**Rows as the sweep wrote them, and what the trees say:**

| Row | Sweep text | Checked | Change to the row |
|---|---|---|---|
| D-09 | four sheets share a name and 404 from search | Live vor has 844 canonical topics with 841 distinct names. **Five** names collide: `chaos-engineering` (testing) and `sbom` (security) lose the name lookup to an external doc, so `/sheet/<name>` 404s. `distributed-systems` (patterns), `saml` (auth) and `vpc` (cloud) lose to another canonical sheet, so their search hit opens **the other category's sheet**, which is a wrong page, not a 404. | Five names, two kinds of failure. The fix is a category in the lookup. |
| D-09 | about 136 dead `[x](Page.md)` links | Counted with a script (read-only): **136** relative links in `wiki/*.md` of the form `Page.md`, all to pages that exist. The server serves `/wiki/Page`, so `/wiki/Page.md` asks for `Page.md.md` and 404s. There are 129 slug links and 511 links into the repository, all of which resolve. No `[[...]]` target is missing. | Confirmed. Fixed in the server, not in the files: the files keep `Page.md` because GitHub's own view of `wiki/` needs it. |
| D-09 | links inside a sheet are not rewritten (relative and `[[...]]`) | vor's sheets hold **179** relative links that resolve to a sheet (`../shell/bash.md`, `group-anagrams.md`, `networking/tcp`) and **41** that name a sheet vor does not have, all in `coding-problems/`. Every `[[` in a sheet is shell code (`[[ $x == foo* ]]`), not a link. | Rewrite the relative links only. **`[[...]]` dropped**: rewriting it would break bash examples. The 41 dead targets are vor content and parked (vor backlog). |
| D-09 | bare `..` and image links | Zero of either in `wiki/*.md` today. | **Dropped.** A regression test (Step 87) catches the first one. |
| D-10 | five hand-written callers | Confirmed: `cmd/zhen-mcp/tools.go:484,507,525`, `cmd/zhen-rag/main.go:279,302`, `cmd/zhen-cli/main.go:728,748`, `cmd/zhen-agent/main.go:288,311`, `cmd/zhen-agentd/main.go:1320,1343`. Found a sixth: `pkg/vorquery/vorquery.go:326` fetches `/api/topics` itself, and all five callers use it. The hand-written calls read unbounded bodies (`json.NewDecoder(resp.Body)` with no limit). | Six call sites, not five; unbounded reads are the defect that gets fixed. |
| D-10 | `cmd/alert-policy/main.go:43` keeps its own blob address | Confirmed, and the same string sits in `cmd/wiki-server/repolinks.go:16` and `scripts/well/test-slo-definitions.sh:85`. | Two Go copies move to one constant. The shell copy is noted and left alone (a test fixture). |
| D-11 | vor: exact id or 404; word-boundary search | `cmd/vor/main.go:1146-1149`: `reg.Get(name)`, then `fuzzyFind` (prefix, substring, Levenshtein). Live: `GET /api/topics/nosuch` answers 200 with `no-unheaded-clause`. Search is `strings.Contains` (`internal/registry/registry.go:269`). `fuzzyFind` is also used by the CLI (`main.go:652, 697, 701, 862`), so it stays there. Nothing in vor's own pages calls `/api/topics/<name>` (grep of `cmd/`, `mobile/`, `CsApp/`). | Confirmed. The API becomes exact; the CLI keeps fuzzy. Word matching is **opt-in** (`match=word`), so callers that rank on substrings (vorquery, the Zhen eval) do not change answers silently. |
| D-12 | nothing reruns the export | Confirmed. `scripts/vor-export-source.sh` exports `HEAD` with `git archive`. vor reads sources only at start (unit comment). Precedent: `scripts/git-hooks/post-commit` already refreshes Zhen's code index when develop moves, and is installed by `make install-hooks`. User-level linger is on for govan. | Mechanism: the existing hook also runs the export of `develop`; a system **path unit** watches a stamp file and restarts vor. Native, nothing third-party. |

**Skill passes:**
- **Marshal** (scope): in scope are D-09 to D-12 as corrected above. Parked as adjacent: the wiki's own copy at `/usr/share/unheaded/wiki` is also a snapshot (same class as D-12, needs root, not in the row); the top-bar links are literal LAN addresses; `docs/LIVE-PATHS.md` lists `cmd/wiki-server` as ORPHAN; wiki-server listens on all interfaces when given `--listen`; 41 dead sheet links in vor's `coding-problems/`; `vor search` (CLI) keeps substring matching. The Zhen callers stay on substring search; switching them would change Zhen's answers and needs the runbook eval (ADR-024). `pkg/zhenrag/prompt.go` "cs" wording belongs to S-RESIDUE D-19.
- **Busboy** (coordination): `wiki/Systemd-Units.md`'s missing `unheaded-vor.service` row is S-RESIDUE D-28. This plan adds rows only for its own new units, and Step 133 adds the vor row if S-RESIDUE has not landed it yet. S-ZHEN-FRONT D-06 (the source viewer shows the nearest sheet) is fixed upstream by Phase 1 here: whichever plan runs second re-checks D-06. `wiki/Zhen-AI.md:97` says `cs serve`; this plan edits only the export sentence on that line.
- **Micromanager** (QA): TDD for every code change, with `-race`. Every phase gate is a command with a number. Live acceptance bars are in Phase 8: 0 wiki links that 404; every canonical sheet answers 200 under its own category; `/sheets` count equals vor's canonical count; an unknown topic is 404; a develop commit is in vor within 60 s.
- **Librarian** (ripple): vor `README.md` (API table), `pkg/vorclient` package doc, `wiki/Microservices.md` (wiki-server row), `wiki/Zhen-AI.md:97` (export), `deploy/systemd/README.md` (vor refresh), `wiki/Systemd-Units.md` (new units). No CLAUDE.md, README or THIRD_PARTY change: no capability or dependency changes.
- **Security note** (for Stevie at Phase 8): the refresh service runs `systemctl try-restart unheaded-vor.service` as root with an empty capability set. govan can trigger it by writing the stamp. govan already owns the corpus vor serves, so this adds only "restart vor", rate-limited by `StartLimitBurst`. The alternative is a dedicated account plus a polkit rule for that one unit. **Decision D1 at Step 118: root with no capabilities (recommended) or polkit.**

**Rules carried in:** no new third-party code. Dated records are never edited (the sweep is not touched; progress goes to a new progress file). No `pkill -f`. Never print secrets. Runbook manifest is `--write` only. Say "VictoriaMetrics", never "the store". Sysadmin knowledge goes in vor sheets, Unheaded facts in the wiki.

## LEGEND

[B] Bash · [V] Verification (must pass) · [D] Debug (only if the prior step failed) · [W] Write/create file · [R] Read · [S] Sudo · [P] Parallelizable · [C] Commit · [STUCK] skipped by Skip Protocol · [BLOCKED] waits on a stuck step · [ATT] attended (Stevie present)

Paths: `U=/home/govan/tmp/unheaded`, `V=/home/govan/tmp/projects/vor`, `S=/home/govan/tmp/unheaded/docs/battle-plans/progress/S-WIKI-VOR-progress.md`.

---

## PHASE 0: PREFLIGHT (Steps 1-10)

**Goal**: both trees clean, toolchains present, baselines recorded.
**Prerequisite**: none. **Time**: 15 min. **Agent**: Solo.

- [ ] **Step 1** [B] ~1m: State of both repositories.
  ```bash
  git -C /home/govan/tmp/unheaded status -sb | head -5; git -C /home/govan/tmp/unheaded log --oneline -1
  git -C /home/govan/tmp/projects/vor status -sb | head -5; git -C /home/govan/tmp/projects/vor log --oneline -1
  ```
- [ ] **Step 2** [V]: unheaded is on `develop` and vor on `main`, and neither has tracked modifications (untracked `demos/doom/` in unheaded is known and left alone).
  - If fail → Step 3.
- [ ] **Step 3** [D]: list what is modified; never stash or reset someone else's edits. Note it in the progress file and STOP for Stevie if a tracked file is dirty.
- [ ] **Step 4** [W] ~2m: create the progress file `S` with a header (date, both HEADs) and a table: Step | Result | Commit.
- [ ] **Step 5** [B] ~1m: toolchains.
  ```bash
  go version; git -C /home/govan/tmp/projects/vor config commit.gpgsign; git -C /home/govan/tmp/unheaded config commit.gpgsign
  ```
- [ ] **Step 6** [V]: Go ≥ 1.27; gpgsign true in both. If the agent is not cached, follow `feedback_unsigned_commits_when_afk` (unsigned, re-sign later with `scripts/resign-unpushed.py`) and record it in `S`.
- [ ] **Step 7** [B] ~5m: baseline tests on the packages this plan touches.
  ```bash
  cd /home/govan/tmp/projects/vor && go test -race -count=1 ./cmd/vor/... ./internal/registry/... 2>&1 | tail -5
  cd /home/govan/tmp/unheaded && go test -race -count=1 ./pkg/vorclient/... ./pkg/vorquery/... ./cmd/wiki-server/... ./cmd/zhen-mcp/... ./cmd/zhen-rag/... ./cmd/zhen-cli/... ./cmd/zhen-agent/... ./cmd/zhen-agentd/... ./cmd/alert-policy/... 2>&1 | tail -15
  bash scripts/git-hooks/test-post-commit.sh 2>&1 | tail -3
  ```
- [ ] **Step 8** [V]: all green. A red baseline is recorded in `S` with its test name and is NOT fixed here (another plan's work). Continue only if the red test is in a package this plan does not touch.
- [ ] **Step 9** [B] ~2m: live baselines (read-only).
  ```bash
  curl -s -o /dev/null -w 'unknown=%{http_code}\n' http://127.0.0.1:20110/api/topics/nosuchtopicxyz
  curl -s http://127.0.0.1:20110/api/topics | python3 -I -c 'import json,sys; t=json.load(sys.stdin); c=[x for x in t if x.get("source_trust")=="canonical"]; print("topics",len(t),"canonical",len(c),"names",len({x["name"].lower() for x in c}))'
  ```
- [ ] **Step 10** [V]: **PHASE 0 EXIT GATE**: record in `S` the numbers `unknown=200`, canonical count and distinct names (844/841 on 2026-11-12). These are the "before" numbers.

---

## PHASE 1: VOR: EXACT TOPIC LOOKUP AND CATEGORY (Steps 11-30)

**Goal**: `GET /api/topics/<name>` answers only the topic of that name (any case) or 404; `?category=<c>` picks the topic in that category. Same for `/detail` and `/related`.
**Prerequisite**: Phase 0. **Time**: 1 h. **Agent**: Solo (vor repo).

- [ ] **Step 11** [R] ~5m: read `V/cmd/vor/main.go:1128-1210` (topic handler), `V/internal/registry/registry.go:120-215` (`byKey`, `sheets`, `Get`), `V/cmd/vor/search_api_test.go` (test style).
- [ ] **Step 12** [W] ~10m: tests first. New `V/internal/registry/getin_test.go`:
  - `GetIn("auth","saml")` returns the auth sheet; `GetIn("security","SAML")` the security sheet (case-insensitive name, exact category).
  - `GetIn("nosuch","saml")` and `GetIn("auth","nosuch")` are nil.
  - `GetIn("","saml")` is nil (category is required for this call).
  Build the registry from an `fstest.MapFS` with two sheets of the same name in two categories.
- [ ] **Step 13** [B] ~1m: `cd /home/govan/tmp/projects/vor && go test -race -count=1 -run GetIn ./internal/registry/ 2>&1 | tail -5`
- [ ] **Step 14** [V]: RED (undefined `GetIn`). If it passes, the test is wrong; fix it.
- [ ] **Step 15** [W] ~10m: implement `func (r *Registry) GetIn(category, name string) *Sheet` over `r.byCategory[category]` with `strings.EqualFold` on the name. No new map.
- [ ] **Step 16** [B] ~1m: rerun Step 13.
- [ ] **Step 17** [V]: GREEN.
- [ ] **Step 18** [W] ~15m: API tests, new `V/cmd/vor/topic_api_test.go`, driving the mux with `httptest`:
  - `/api/topics/nosuchtopicxyz` → 404 (today 200, fuzzy).
  - `/api/topics/sa` → 404 (prefix no longer resolves).
  - `/api/topics/SAML` → 200, name as stored.
  - `/api/topics/saml?category=auth` → 200, category `auth`; `?category=security` → `security`; `?category=nosuch` → 404.
  - `/api/topics/nosuch/detail` and `/related` → 404.
  - `?category=` with `..`, `/`, NUL, or over 256 bytes → 400.
  - Existing traversal cases still 400.
  If the mux is built inline in `main()`, first extract it into a `newMux(reg *registry.Registry) *http.ServeMux` (a pure move, its own commit at Step 21).
- [ ] **Step 19** [B] ~1m: `go test -race -count=1 -run 'Topic' ./cmd/vor/ 2>&1 | tail -8`
- [ ] **Step 20** [V]: RED on the new cases, and only those.
- [ ] **Step 21** [C]: if a mux extraction was needed, commit it alone first.
  ```bash
  cd /home/govan/tmp/projects/vor && git add cmd/vor/main.go && git commit -S -m "refactor(api): build the HTTP mux in newMux, no behaviour change"
  ```
- [ ] **Step 22** [W] ~15m: in the `/api/topics/` handler, replace `reg.Get` + `fuzzyFind` with: `category := r.URL.Query().Get("category")`; validate it (same refusals as the name, plus length ≤ 256); `s = reg.GetIn(category, name)` when set, else `reg.Get(name)`; nil → 404. Leave every CLI use of `fuzzyFind` alone. Comment: "the API answers exact names only (2026-11-12, S-WIKI-VOR); the CLI keeps fuzzy matching for typing".
- [ ] **Step 23** [B] ~2m: `go test -race -count=1 ./cmd/vor/... ./internal/registry/... 2>&1 | tail -8`
- [ ] **Step 24** [V]: GREEN, and no older test broke. If an old test expected fuzzy over HTTP → Step 25.
- [ ] **Step 25** [D]: change that test to the new contract only if it tests the API; a test of the CLI path must still pass unchanged. A CLI test failing means the handler change leaked; fix the handler.
- [ ] **Step 26** [C]:
  ```bash
  cd /home/govan/tmp/projects/vor && git add internal/registry/registry.go internal/registry/getin_test.go cmd/vor/main.go cmd/vor/topic_api_test.go && git commit -S -m "fix(api): exact topic names or 404, and ?category= for names shared across categories"
  ```
- [ ] **Step 27** [B] ~2m: the binary still serves (scratch port, no install).
  ```bash
  cd /home/govan/tmp/projects/vor && make build >/dev/null && (./vor serve 20190 >/tmp/claude-1000/vor-20190.log 2>&1 & echo $! > /tmp/claude-1000/vor-20190.pid); sleep 3
  curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:20190/api/topics/nosuchtopicxyz
  curl -s 'http://127.0.0.1:20190/api/topics/saml?category=auth' | python3 -I -c 'import json,sys; t=json.load(sys.stdin); print(t["category"])'
  ```
- [ ] **Step 28** [V]: `404` and `auth`.
- [ ] **Step 29** [B]: stop the scratch vor by its pid, never by pattern: `kill "$(cat /tmp/claude-1000/vor-20190.pid)"; rm -f /tmp/claude-1000/vor-20190.pid`
- [ ] **Step 30** [V]: **PHASE 1 EXIT GATE**: `go test -race ./...` in vor green; Step 28 passed; commit(s) made.

---

## PHASE 2: VOR: WORD MATCHING IN SEARCH (Steps 31-46)

**Goal**: `GET /api/search?q=...&match=word` keeps a topic only when every term starts a word in it. The default (`match` absent or `substring`) is unchanged.
**Prerequisite**: Phase 1. **Time**: 1 h. **Agent**: Solo (vor repo).

- [ ] **Step 31** [R] ~5m: `V/internal/registry/registry.go:255-400` (Search, `containsAll`, line selection) and `V/cmd/vor/search_api.go` (`searchLimits`).
- [ ] **Step 32** [W] ~15m: tests first, `V/internal/registry/search_word_test.go`:
  - A sheet containing `KRB5CCNAME` and no `ccna` word: `SearchWords("ccna")` returns nothing; `Search("ccna")` still returns it (default unchanged).
  - `SearchWords("wotan")` matches `wotan's` and `Wotan-Memory` (start of word; `-`, `'`, `/`, `_`, `.`, space and line start count as boundaries).
  - `SearchWords("shell-scripting")` keeps the hyphen split (both halves must start a word).
  - Returned lines in word mode carry the term at a word start (strict lines), falling back the same way the substring mode does.
  - The 16-term cap still applies.
- [ ] **Step 33** [B] ~1m: `go test -race -count=1 -run Word ./internal/registry/ 2>&1 | tail -5`
- [ ] **Step 34** [V]: RED.
- [ ] **Step 35** [W] ~20m: implement `SearchWords(queries ...string) []Match`: share Search's body via an unexported `search(match func(hay, term string) bool, queries ...string)`; `Search` passes `strings.Contains`; `SearchWords` passes a `startsWord` that scans `strings.Index` hits and checks that the byte before is not a letter or digit (ASCII; the corpus is lower-cased before the match, as today). No regexp.
- [ ] **Step 36** [B] ~1m: rerun Step 33, then the whole registry package.
- [ ] **Step 37** [V]: GREEN; old Search tests unchanged and green.
- [ ] **Step 38** [W] ~10m: API tests in `V/cmd/vor/search_api_test.go`: `match=word` uses SearchWords; `match=substring` and no `match` use Search; `match=other` → 400 with `{"error":...}`; `match` combines with `category`, `limit`, `per_topic`.
- [ ] **Step 39** [B]: `go test -race -count=1 -run Search ./cmd/vor/ 2>&1 | tail -5` → expect RED.
- [ ] **Step 40** [W] ~10m: read `match` in the `/api/search` handler, validate it, dispatch.
- [ ] **Step 41** [B] ~2m: `go test -race -count=1 ./... 2>&1 | tail -8 && go vet ./... && gofmt -l cmd internal`
- [ ] **Step 42** [V]: all tests green, vet clean, gofmt prints nothing.
- [ ] **Step 43** [W] ~5m: vor `README.md` API table: `/api/topics/:name` is exact (any case) or 404, with `?category=`; `/api/search` gains `&match=word` (a term must start a word; default substring).
- [ ] **Step 44** [B] ~3m: `cd /home/govan/tmp/projects/vor && make lint 2>&1 | tail -5`
- [ ] **Step 45** [C]:
  ```bash
  cd /home/govan/tmp/projects/vor && git add internal/registry/registry.go internal/registry/search_word_test.go cmd/vor/main.go cmd/vor/search_api.go cmd/vor/search_api_test.go README.md && git commit -S -m "feat(api): match=word on /api/search: a term must start a word"
  ```
- [ ] **Step 46** [V]: **PHASE 2 EXIT GATE**: `make test` and `make lint` green in vor; `git -C /home/govan/tmp/projects/vor log --oneline -3` shows the Phase 1-2 commits, all `G` in `%G?`.

---

## PHASE 3: UNHEADED CLIENT: CATEGORY, WORD MATCH, ONE BLOB ADDRESS (Steps 47-62)

**Goal**: `pkg/vorclient` can fetch a topic by category and name and ask for word matching; one constant holds the GitHub blob address.
**Prerequisite**: Phase 2 (API contract fixed). **Time**: 45 min. **Agent**: Solo.

- [ ] **Step 47** [W] ~10m: tests first in `U/pkg/vorclient/vorclient_test.go`:
  - `TopicIn(ctx,"auth","saml")` sends `/api/topics/saml?category=auth` and returns the topic; a reply whose category or name differs (case-insensitive name) is `ErrNotFound`; 404 is `ErrNotFound`; an empty category is an error, not a request.
  - `SearchOptions{Words: true}` adds `match=word`; false adds nothing.
  - `Topic` against a server that answers another name is still `ErrNotFound` (unchanged guard, kept for an old vor).
- [ ] **Step 48** [B]: `cd /home/govan/tmp/unheaded && go test -race -count=1 ./pkg/vorclient/ 2>&1 | tail -5` → RED.
- [ ] **Step 49** [W] ~15m: implement `TopicIn` (same refusals as `Topic` for name and category) and `Words` in `SearchOptions.query`. Update the package doc: exact names, `category`, `match=word`, and that a vor from before 2026-11-12 answers fuzzy (which the name check covers).
- [ ] **Step 50** [B]: rerun Step 48.
- [ ] **Step 51** [V]: GREEN, with `go vet ./pkg/vorclient/` clean.
- [ ] **Step 52** [C]:
  ```bash
  git add pkg/vorclient/vorclient.go pkg/vorclient/vorclient_test.go && git commit -S -m "feat(vorclient): TopicIn by category, Words for match=word"
  ```
- [ ] **Step 53** [W] ~5m: test first, `U/pkg/repolink/repolink_test.go`: `BlobBase` ends in `/blob/develop/`; `Blob("docs/a b.md")` escapes the path; `Blob("")` and paths with `..` return `""`.
- [ ] **Step 54** [W] ~10m: `U/pkg/repolink/repolink.go`: `const BlobBase = "https://github.com/unheaded/unheaded/blob/develop/"` and `Blob(path string) string`. GPL header, package doc.
- [ ] **Step 55** [B]: `go test -race -count=1 ./pkg/repolink/ 2>&1 | tail -3`
- [ ] **Step 56** [V]: GREEN.
- [ ] **Step 57** [W] ~5m: `cmd/alert-policy/main.go:43`: `repoBlob = repolink.BlobBase`; `cmd/wiki-server/repolinks.go:16`: `repoBlobURL = repolink.BlobBase`. Tests keep their literal URLs on purpose (they pin the output).
- [ ] **Step 58** [B] ~3m: `go test -race -count=1 ./cmd/alert-policy/... ./cmd/wiki-server/... 2>&1 | tail -5; grep -rn '"https://github.com/unheaded/unheaded/blob' --include=*.go cmd pkg | grep -v _test`
- [ ] **Step 59** [V]: tests green; grep shows only `pkg/repolink/repolink.go`.
- [ ] **Step 60** [D]: if the grep shows another copy, move it the same way, or note why it stays.
- [ ] **Step 61** [C]:
  ```bash
  git add pkg/repolink/repolink.go pkg/repolink/repolink_test.go cmd/alert-policy/main.go cmd/wiki-server/repolinks.go && git commit -S -m "refactor: one GitHub blob address in pkg/repolink"
  ```
- [ ] **Step 62** [V]: **PHASE 3 EXIT GATE**: `go test -race ./pkg/vorclient/... ./pkg/repolink/... ./cmd/alert-policy/... ./cmd/wiki-server/...` green.

---

## PHASE 4: WIKI-SERVER: LINKS THAT RESOLVE (Steps 63-92)

**Goal**: every canonical sheet has a URL under its category; search links use it; `Page.md` links in wiki pages and relative links in sheets go somewhere that answers.
**Prerequisite**: Phase 3. **Time**: 1.5 h. **Agent**: Solo [P with Phase 5].

### 4a. `/sheet/<category>/<name>`

- [ ] **Step 63** [R] ~5m: `U/cmd/wiki-server/search.go:241-470` (searchResults, handleSheet, listSheets), `search_test.go` fake vor.
- [ ] **Step 64** [W] ~15m: tests first in `search_test.go` (the fake vor learns `?category=`):
  - `/sheet/auth/saml` renders the auth sheet; `/sheet/security/saml` the security one.
  - `/sheet/testing/chaos-engineering` renders the canonical sheet even though an external topic of that name is larger.
  - `/sheet/auth/nosuch` → 404; `/sheet/wiki/Chaos-Engineering` (external) → 404; `/sheet/a/b/c`, `/sheet/../x`, and an over-long path → 404.
  - `/sheet/<name>` (old form) still works for unique names (bookmarks, see-also).
  - Search hit links for canonical sheets are `/sheet/<category>/<name>`, escaped.
  - `/sheets` lists every canonical topic of the fake listing, duplicates included (count equals canonical count), each linked by category.
- [ ] **Step 65** [B]: `go test -race -count=1 -run 'Sheet|Search' ./cmd/wiki-server/ 2>&1 | tail -8` → RED.
- [ ] **Step 66** [W] ~20m: `handleSheet`: two segments → `ws.vor.TopicIn(ctx, cat, name)`; one segment → `ws.vor.Topic` (as today); still refuse non-canonical. `searchResults`: canonical URL `sheetPath + url.PathEscape(h.Category) + "/" + url.PathEscape(h.Topic)`. `listSheets`: drop the per-duplicate fetch (no longer needed); link every canonical row by category. Bound the category with the same byte cap as the name.
- [ ] **Step 67** [B]: rerun Step 65.
- [ ] **Step 68** [V]: GREEN.
- [ ] **Step 69** [W] ~5m: the wiki's search asks vor with `Words: true` (wiki readers search for words). Test: the fake vor sees `match=word` from `/search`.
- [ ] **Step 70** [B]: `go test -race -count=1 ./cmd/wiki-server/ 2>&1 | tail -5`
- [ ] **Step 71** [V]: GREEN.
- [ ] **Step 72** [C]:
  ```bash
  git add cmd/wiki-server/search.go cmd/wiki-server/search_test.go && git commit -S -m "fix(wiki-server): sheets by category, so names shared by two topics open the right one"
  ```

### 4b. `Page.md` links in wiki pages

- [ ] **Step 73** [W] ~10m: tests first in `repolinks_test.go`:
  - `[x](Architecture.md)` → `/wiki/Architecture`; `[x](Architecture.md#wire)` → `/wiki/Architecture#wire`.
  - `[x](Nope.md)` (no such page) is left as written.
  - `[x](../docs/a.md)` → GitHub blob (unchanged); `[x](Architecture)` unchanged; `https://…/x.md` unchanged.
  - `[x](sub/Page.md)` (a subdirectory) is not rewritten.
- [ ] **Step 74** [B]: `go test -race -count=1 -run Link ./cmd/wiki-server/ 2>&1 | tail -5` → RED.
- [ ] **Step 75** [W] ~15m: a `rewriteWikiPageLinks(doc ast.Node, pages map[string]bool)` beside `rewriteRepoLinks`, called from `renderPage` with `ws.pageSlugs()`: a relative destination whose path is `<slug>.md` with no `/`, for an existing slug, becomes `/wiki/<escaped slug>` plus the fragment. Files are not edited.
- [ ] **Step 76** [B]: rerun Step 74.
- [ ] **Step 77** [V]: GREEN.
- [ ] **Step 78** [C]:
  ```bash
  git add cmd/wiki-server/repolinks.go cmd/wiki-server/repolinks_test.go cmd/wiki-server/main.go && git commit -S -m "fix(wiki-server): Page.md links open the page (136 were 404)"
  ```

### 4c. Relative links inside a sheet

- [ ] **Step 79** [W] ~10m: tests first (new `sheetlinks_test.go`), for a sheet in category `shell`:
  - `[t](tmux.md)` → `/sheet/shell/tmux`; `[n](../editors/neovim.md)` → `/sheet/editors/neovim`; `[q](networking/tcp)` → `/sheet/networking/tcp`; with a fragment, the fragment is kept.
  - A target that is not a canonical sheet in the listing: the link is removed and its text kept.
  - `https://…`, `#anchor`, `/abs`, and `../../x.md` (climbs out) are left alone.
  - `[[ $x == y ]]` in a code block comes out byte-identical (no `[[...]]` rewriting in sheets).
- [ ] **Step 80** [B]: `go test -race -count=1 -run SheetLink ./cmd/wiki-server/ 2>&1 | tail -5` → RED.
- [ ] **Step 81** [W] ~20m: `rewriteSheetLinks(doc ast.Node, category string, known func(cat, name string) bool)`. In `handleSheet`, parse with `ws.md.Parser()`, rewrite, then render (as `renderPage` does). `known` reads the cached sheet listing (`ws.sheetsPage`); when the listing is unavailable, links are left as written (never block a page on it).
- [ ] **Step 82** [B]: rerun Step 80, then the whole package.
- [ ] **Step 83** [V]: GREEN.
- [ ] **Step 84** [C]:
  ```bash
  git add cmd/wiki-server/sheetlinks.go cmd/wiki-server/sheetlinks_test.go cmd/wiki-server/search.go && git commit -S -m "fix(wiki-server): links between sheets open the sheet; unknown targets lose the link"
  ```

### 4d. A permanent gate on wiki links

- [ ] **Step 85** [W] ~15m: `cmd/wiki-server/links_gate_test.go`, `TestEveryWikiLinkResolves`: render every `U/wiki/*.md` through `renderPage` against the real directory; collect every `href` in the output; each `/wiki/<x>` must be an existing slug; each relative href is a failure; GitHub blob hrefs must name an existing path in the checkout. Report every failure with page and target, not just the first.
- [ ] **Step 86** [B]: `go test -race -count=1 -run TestEveryWikiLinkResolves ./cmd/wiki-server/ 2>&1 | tail -15`
- [ ] **Step 87** [V]: GREEN, with zero failures. Also add one case each for a bare `..` link and an image link in a temp wiki dir, so that the first one written later shows up in the test.
  - If fail → Step 88.
- [ ] **Step 88** [D]: each failure is either a rewrite gap (fix in 4b) or a real dead link in a page (fix the page; never a dated record: `ADR-*`, sweeps and progress files get a dated note instead, and the test lists them in an explicit allow list with the reason).
- [ ] **Step 89** [B] ~2m: `go vet ./cmd/wiki-server/ && gofmt -l cmd/wiki-server pkg/repolink pkg/vorclient`
- [ ] **Step 90** [V]: clean.
- [ ] **Step 91** [C]:
  ```bash
  git add cmd/wiki-server/links_gate_test.go && git commit -S -m "test(wiki-server): every link in wiki/ resolves"
  ```
- [ ] **Step 92** [V]: **PHASE 4 EXIT GATE**: `go test -race -count=1 ./cmd/wiki-server/...` green; the four Phase 4 commits are in `git log`.

---

## PHASE 5: ONE VOR CLIENT (Steps 93-114)

**Goal**: the six hand-written call sites use `pkg/vorclient`; responses are bounded; Zhen's answers do not change.
**Prerequisite**: Phase 3. **Time**: 1.5 h. **Agent**: Solo [P with Phase 4].

- [ ] **Step 93** [B] ~2m: the list to clear (the gate for this phase).
  ```bash
  cd /home/govan/tmp/unheaded && grep -rnE '"/api/(search|topics)|%s/api/(search|topics)|/api/topics"' --include=*.go cmd pkg | grep -v _test | grep -v '^pkg/vorclient/'
  ```
- [ ] **Step 94** [V]: the hits are the six sites in the Grounding table (plus comments). Record them in `S`.
- [ ] **Step 95** [W] ~15m: `pkg/vorquery`: `NewIndex` takes a `*vorclient.Client` (or a small `topicsLister` interface it satisfies) instead of `(*http.Client, baseURL)`, and reads the listing with `Topics`. Its tests use an `httptest` vor through `vorclient.New`. Keep `Order` and `Select` unchanged.
- [ ] **Step 96** [B]: `go test -race -count=1 ./pkg/vorquery/ 2>&1 | tail -5`
- [ ] **Step 97** [V]: GREEN, including the twin test against `raft/scripts/zhen_rag.py`'s stop words.
- [ ] **Step 98** [W] ~15m: `cmd/zhen-agentd`: `vorRetriever` holds a `*vorclient.Client`; `search` → `Search(ctx, q, vorclient.SearchOptions{})`; `topic` → `TopicIn(ctx, hit.Category, hit.Topic)` (hits carry the category, so the name collision cannot attribute the wrong text); drop `vorSearchHit` and `vorTopic`. `/api/health` probes stay hand-written (they are pings, not reads).
- [ ] **Step 99** [B] ~3m: `go test -race -count=1 ./cmd/zhen-agentd/ 2>&1 | tail -8`
- [ ] **Step 100** [V]: GREEN, including `adv_vor_test.go` and `redteam_test.go` (an oversized vor body now fails bounded, which those tests should assert; add the case if missing).
- [ ] **Step 101** [C]:
  ```bash
  git add pkg/vorquery/vorquery.go pkg/vorquery/vorquery_test.go cmd/zhen-agentd/main.go cmd/zhen-agentd/adv_vor_test.go && git commit -S -m "refactor(zhen-agentd): vor through pkg/vorclient, bounded reads, topics by category"
  ```
- [ ] **Step 102** [W] ~10m: `cmd/zhen-agent/main.go` (288, 311) the same way.
- [ ] **Step 103** [W] ~10m: `cmd/zhen-cli/main.go` (658, 728, 748) the same way; keep its health probe.
- [ ] **Step 104** [B]: `go test -race -count=1 ./cmd/zhen-agent/ ./cmd/zhen-cli/ 2>&1 | tail -8`
- [ ] **Step 105** [V]: GREEN (`phase3_test.go` included).
- [ ] **Step 106** [C]:
  ```bash
  git add cmd/zhen-agent/main.go cmd/zhen-cli/main.go && git commit -S -m "refactor(zhen-agent, zhen-cli): vor through pkg/vorclient"
  ```
- [ ] **Step 107** [W] ~15m: `cmd/zhen-mcp/tools.go` (`vorGet`, 484-530) and `main.go:275`; keep the error text `corpus_search: vor unavailable at <url>` (tests and the MCP clients read it). `cmd/zhen-rag/main.go` (`vorSearch`, `vorTopicFetch`, 59-90 types).
- [ ] **Step 108** [B]: `go test -race -count=1 ./cmd/zhen-mcp/ ./cmd/zhen-rag/ 2>&1 | tail -8`
- [ ] **Step 109** [V]: GREEN.
- [ ] **Step 110** [B]: rerun Step 93.
- [ ] **Step 111** [V]: zero hits outside comments. A comment that names the endpoint is fine.
- [ ] **Step 112** [B] ~10m: no change in Zhen's retrieval on the live corpus (read-only, against west's vor, no model needed). `zhen-rag` has no retrieve-only mode and needs llama-server, so use `zhen-mcp`'s `corpus_search` tool over stdio (the handshake lines are in `scripts/check-zhen-mcp.sh`; read it first). Build `zhen-mcp` twice: from this HEAD into `/tmp/claude-1000/zhen-mcp-after`, and from the commit before Phase 5's first commit, in a throwaway worktree (`git worktree add /tmp/claude-1000/wv-before <hash>`, build, then `git worktree remove /tmp/claude-1000/wv-before`), into `/tmp/claude-1000/zhen-mcp-before`. For each of `what is wotan`, `getting the ccna`, `jncia ospf areas`, `sbom` and `saml`, send `initialize` and one `tools/call` `corpus_search` to each binary with `VOR_URL=http://127.0.0.1:20110`, and save the topic lists to `/tmp/claude-1000/retr-{before,after}.txt`. Then run `diff`.
- [ ] **Step 113** [V]: same topics in the same order for all five questions, except `sbom` and `saml`, where the category now picks the hit's own topic (an intended fix; record before and after in `S`).
- [ ] **Step 114** [C]: **PHASE 5 EXIT GATE**:
  ```bash
  go test -race -count=1 ./pkg/vorquery/... ./cmd/zhen-mcp/... ./cmd/zhen-rag/... ./cmd/zhen-cli/... ./cmd/zhen-agent/... ./cmd/zhen-agentd/... 2>&1 | tail -10
  git add cmd/zhen-mcp/tools.go cmd/zhen-mcp/main.go cmd/zhen-rag/main.go && git commit -S -m "refactor(zhen-mcp, zhen-rag): vor through pkg/vorclient; no hand-built vor URL left"
  ```

---

## PHASE 6: VOR FOLLOWS DEVELOP (Steps 115-130)

**Goal**: a commit or merge on develop exports develop to vor's source and restarts vor, with no one typing anything.
**Prerequisite**: Phase 0. **Time**: 1 h. **Agent**: Solo (code only; installing is Phase 8).

- [ ] **Step 115** [W] ~15m: tests first, new `U/scripts/test-vor-export-source.sh` (temp repo, temp destination, `GIT_CONFIG_GLOBAL=/dev/null` as `test-post-commit.sh` does):
  - T1: export of `HEAD` writes the tree, `.vor-export` holds the full commit hash, and `<dest>.exported` holds the same hash.
  - T2: `vor-export-source.sh <dest> develop` exports develop while HEAD is on another branch.
  - T3: a destination that is not an earlier export is refused (exit 2), no stamp written.
  - T4: the stamp is written after the swap (its mtime is not older than `.vor-export`), and written by rename (no partial stamp: a reader during the run sees the old hash or the new one).
  - T5: an unknown ref exits non-zero and leaves the old export and stamp untouched.
- [ ] **Step 116** [B]: `bash scripts/test-vor-export-source.sh 2>&1 | tail -6` → RED.
- [ ] **Step 117** [W] ~15m: `scripts/vor-export-source.sh`: optional second argument `ref` (default `HEAD`), resolved once with `git rev-parse --verify "$ref^{commit}"`; archive that hash; write it into `.vor-export`; after the `mv`, write `${dest}.exported` via a temp file and `mv`. Usage line updated.
- [ ] **Step 118** [V]: rerun Step 116; GREEN. **D1 recorded** in `S` as "pending, Stevie at Step 136"; the unit below is written for the recommended choice.
- [ ] **Step 119** [W] ~10m: tests first in `scripts/git-hooks/test-post-commit.sh`: T5, a commit on develop with `VOR_EXPORT_DEST` pointing at an earlier temp export, runs the export (the stamp changes within 5 s); T6, on another branch nothing is exported; T7, `VOR_EXPORT_DEST` that is not an export leaves the commit exiting 0 and writes nothing.
- [ ] **Step 120** [B]: `bash scripts/git-hooks/test-post-commit.sh 2>&1 | tail -8` → RED on T5.
- [ ] **Step 121** [W] ~10m: `scripts/git-hooks/post-commit`: after the code-index line, if `${VOR_EXPORT_DEST:-/var/zhen/vor-source/unheaded}/.vor-export` exists, `nohup nice -n 19 "$top/scripts/vor-export-source.sh" "$dest" develop >/dev/null 2>&1 &`. Header comment names both jobs.
- [ ] **Step 122** [B]: rerun Step 120.
- [ ] **Step 123** [V]: T1-T7 green.
- [ ] **Step 124** [C]:
  ```bash
  git add scripts/vor-export-source.sh scripts/test-vor-export-source.sh scripts/git-hooks/post-commit scripts/git-hooks/test-post-commit.sh && git commit -S -m "feat(vor): develop's export refreshes itself from the post-commit hook, with a stamp"
  ```
- [ ] **Step 125** [W] ~10m: `deploy/systemd/unheaded-vor-refresh.path`: `PathChanged=/var/zhen/vor-source/unheaded.exported`, `Unit=unheaded-vor-refresh.service`, `WantedBy=paths.target`. `deploy/systemd/unheaded-vor-refresh.service`: `Type=oneshot`, `ExecStart=/usr/bin/systemctl try-restart unheaded-vor.service`, `StartLimitIntervalSec=10min`, `StartLimitBurst=6`, `CapabilityBoundingSet=`, `NoNewPrivileges=yes`, `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`, `PrivateDevices=yes`, `PrivateNetwork=yes`, `RestrictAddressFamilies=AF_UNIX`, `SystemCallArchitectures=native`, `SystemCallFilter=@system-service`. Header: what it does, the install lines, and the D1 note (root with no capabilities; polkit alternative).
- [ ] **Step 126** [B] ~2m: `systemd-analyze verify deploy/systemd/unheaded-vor-refresh.path deploy/systemd/unheaded-vor-refresh.service 2>&1 | grep -v 'unheaded-vor.service' | head`
- [ ] **Step 127** [V]: no errors (a warning about the not-installed target unit is expected).
- [ ] **Step 128** [B] ~3m: run the repo's unit gates if they exist (`ls scripts | grep -i -E 'unit|systemd'`, e.g. a sandbox-directive check over `deploy/systemd/`), and the sysusers check if it lists units.
- [ ] **Step 129** [V]: green, or the new units are added to the gate's list the way neighbours are.
- [ ] **Step 130** [C]: **PHASE 6 EXIT GATE**: both test scripts green.
  ```bash
  git add deploy/systemd/unheaded-vor-refresh.path deploy/systemd/unheaded-vor-refresh.service && git commit -S -m "feat(deploy): unheaded-vor-refresh path unit restarts vor on a new export"
  ```

---

## PHASE 7: DOCUMENTATION RIPPLE (Steps 131-135)

**Goal**: every doc that names the changed behaviour says the new thing.
**Prerequisite**: Phases 4-6. **Time**: 20 min. **Agent**: Solo.

- [ ] **Step 131** [W] ~5m: `wiki/Microservices.md` wiki-server row: sheets are `/sheet/<category>/<name>` (the old form still answers for unique names); `Page.md` links open the page; search matches words.
- [ ] **Step 132** [W] ~5m: `wiki/Zhen-AI.md:97`, the export sentence only: develop's export refreshes itself after every commit or merge on develop (post-commit hook), and `unheaded-vor-refresh.path` restarts vor; by hand it is still `scripts/vor-export-source.sh <dest> develop`. `deploy/systemd/README.md`: a "vor refresh" section with the same facts and the install lines.
- [ ] **Step 133** [W] ~5m: `wiki/Systemd-Units.md`: add `unheaded-vor-refresh.path` / `.service` under "Installed on west" **after** Phase 8 installs them, with the date. If S-RESIDUE has not yet added the `unheaded-vor.service` row (D-28), add it here and note it in `S` so S-RESIDUE skips it.
- [ ] **Step 134** [B] ~2m: `go test -race -count=1 -run TestEveryWikiLinkResolves ./cmd/wiki-server/ && bash scripts/check-js-lint.sh >/dev/null 2>&1; echo lint=$?`
- [ ] **Step 135** [C]: **PHASE 7 EXIT GATE**: link gate green.
  ```bash
  git add wiki/Microservices.md wiki/Zhen-AI.md deploy/systemd/README.md && git commit -S -m "docs(wiki): sheet URLs, vor follows develop"
  ```

---

## PHASE 8: INSTALL ON WEST (Steps 136-139) [ATT][S]

**Goal**: the new vor, wiki-server, wiki copy and refresh units run on west, and the live bars pass.
**Prerequisite**: Phases 1-7 green; Stevie present; D1 answered. **Time**: 45 min. **Agent**: Coordinator with Stevie.

- [ ] **Step 136** [ATT]: ask D1 (root with no capabilities, recommended, or polkit). With polkit, write the rule and account first as a sub-step list in `S` and re-verify Step 126.
- [ ] **Step 137** [S][ATT] ~15m: install, each with its backup.
  ```bash
  cd /home/govan/tmp/projects/vor && make build && sudo cp -p /usr/local/bin/vor /var/backups/vor-ff4c7e4.bak && sudo install -m 0755 vor /usr/local/bin/vor
  cd /home/govan/tmp/unheaded && go build -trimpath -o /tmp/claude-1000/wiki-server ./cmd/wiki-server && sudo cp -p /usr/local/bin/wiki-server /var/backups/wiki-server-2026-11-11.bak && sudo install -m 0755 /tmp/claude-1000/wiki-server /usr/local/bin/wiki-server
  sudo rsync -a --delete /home/govan/tmp/unheaded/wiki/ /usr/share/unheaded/wiki/
  sudo cp deploy/systemd/unheaded-vor-refresh.path deploy/systemd/unheaded-vor-refresh.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now unheaded-vor-refresh.path
  make install-hooks
  bash scripts/vor-export-source.sh /var/zhen/vor-source/unheaded develop
  sudo systemctl restart unheaded-wiki-server
  ```
  Then wait for the path unit to restart vor: `systemctl show unheaded-vor -p ActiveEnterTimestamp`.
- [ ] **Step 138** [V][ATT] ~15m: **LIVE BARS** (each recorded in `S` with its number):
  1. `curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:20110/api/topics/nosuchtopicxyz` → `404` (was 200).
  2. For `testing/chaos-engineering`, `security/sbom`, `patterns/distributed-systems`, `auth/saml`, `cloud/vpc`: `GET /api/topics/<n>?category=<c>` → that category, canonical.
  3. Through the wiki, logged in as `dev` (cookie from the login, as in the night session's checks): `/sheet/<c>/<n>` → 200 for all five; `/sheets` shows the canonical count from Step 9 (844 or today's number), not 3-5 fewer.
  4. A crawl of every `/wiki/<page>` as `dev`: every `href` starting `/wiki/` or `/sheet/` answers 200. Count of failures = **0** (was 136 `Page.md` links).
  5. `/search?q=ccna` lists no Kerberos sheet; `/search?q=wotan` still lists Service-Wotan.
  6. `systemctl is-enabled unheaded-vor-refresh.path` → enabled; `.vor-export` holds develop's hash.
  - If any fails → Step 139.
- [ ] **Step 139** [D][ATT]: roll back only the failing piece from its backup (`/var/backups/vor-ff4c7e4.bak`, `/var/backups/wiki-server-2026-11-11.bak`, `systemctl disable --now unheaded-vor-refresh.path`), record the failure in `S`, and return to the phase that owns it. Never leave a half-installed host: either the bar passes or the backup is back.

---

## PHASE 9: CLOSE-OUT (Steps 140-142)

**Goal**: the record is complete and the refresh is proven by a real commit.
**Prerequisite**: Phase 8. **Time**: 15 min. **Agent**: Solo.

- [ ] **Step 140** [W][C]: finish `S` (every step, its result, before/after numbers). Add Step 133's Systemd-Units rows. Commit both on develop: `git add docs/battle-plans/progress/S-WIKI-VOR-progress.md wiki/Systemd-Units.md && git commit -S -m "docs(s-wiki-vor): progress record; refresh units installed on west"`. **This commit is the live proof of D-12:** within 60 s, `.vor-export` holds its hash, vor's `ActiveEnterTimestamp` is newer than the commit, and `curl -s 'http://127.0.0.1:20110/api/search?q=S-WIKI-VOR&limit=1'` finds the progress file.
- [ ] **Step 141** [V]: Step 140's proof holds. If not: `journalctl -u unheaded-vor-refresh.service -n 20`, `systemctl status unheaded-vor-refresh.path`, and check the hook symlink (`ls -l .git/hooks/post-commit`).
- [ ] **Step 142** [W]: `~/tmp/next.md`: S-WIKI-VOR done (or STUCK report), the parked items from the Grounding (wiki copy snapshot, LAN literal links, ORPHAN listing, 41 dead vor sheet links, CLI word search), and "push develop and vor main" for Stevie. The sweep file is a dated record and is not edited.

---

## Appendix A: Emergency procedures

1. **vor will not start after the install** (Step 137): `journalctl -u unheaded-vor -n 30`; restore `/var/backups/vor-ff4c7e4.bak` to `/usr/local/bin/vor`; `sudo systemctl restart unheaded-vor`. The wiki shows its "vor is down" page until vor is back, and Zhen's `/ready` reports `vor_reachable: false`.
2. **The path unit restarts vor in a loop**: `StartLimitBurst=6` per 10 min stops it. `sudo systemctl disable --now unheaded-vor-refresh.path`, look at what writes the stamp (`ls -l --time-style=full-iso /var/zhen/vor-source/`), then fix the writer, not the limit.
3. **A commit hangs or fails after `make install-hooks`**: the hook backgrounds both jobs and exits 0; if a commit waits, the hook was edited wrongly. `ln -sf ../../scripts/git-hooks/post-commit .git/hooks/post-commit` points it back; `git commit --no-verify` does NOT skip post-commit, so fix the script.
4. **Wiki pages 500 after the new wiki-server**: restore `/var/backups/wiki-server-2026-11-11.bak`; the wiki copy at `/usr/share/unheaded/wiki` is plain files and needs no rollback.
5. **Zhen answers change after Phase 5**: compare Step 112's two runs; the only intended difference is the category pick for shared names. Anything else is a regression in the migration: revert that caller's commit, not the client.

## Appendix B: Agent assignment matrix

| Phase | Agent | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Solo | no | none | 15 min |
| 1 vor exact lookup | Solo (vor) | no | 0 | 1 h |
| 2 vor word match | Solo (vor) | no | 1 | 1 h |
| 3 client + blob constant | Solo | no | 2 | 45 min |
| 4 wiki links | Solo | with 5 | 3 | 1.5 h |
| 5 one vor client | Solo | with 4 | 3 | 1.5 h |
| 6 export refresh | Solo | with 1-5 | 0 | 1 h |
| 7 docs | Solo | no | 4, 5, 6 | 20 min |
| 8 install on west | Coordinator + Stevie | no | 7, D1 | 45 min |
| 9 close-out | Solo | no | 8 | 15 min |

Critical path: 0 → 1 → 2 → 3 → 4 (or 5) → 7 → 8 → 9, about 6.5 h.

## Appendix C: Quick reference

| Thing | Value |
|---|---|
| vor, live | `http://127.0.0.1:20110` (`unheaded-vor.service`, user `unheaded-vor`) |
| vor, scratch for tests | `127.0.0.1:20190`, stopped by pid file |
| Topic, exact | `GET /api/topics/<name>[?category=<c>]` → 200 or 404 |
| Search, words | `GET /api/search?q=<q>&match=word[&category=&limit=&per_topic=]` |
| Sheet URL | `/sheet/<category>/<name>` (old `/sheet/<name>` still answers for unique names) |
| Export | `scripts/vor-export-source.sh /var/zhen/vor-source/unheaded develop`; stamp `/var/zhen/vor-source/unheaded.exported` |
| Refresh units | `unheaded-vor-refresh.path` → `unheaded-vor-refresh.service` (`systemctl try-restart unheaded-vor`) |
| Hook | `scripts/git-hooks/post-commit` (also `post-merge`), `make install-hooks` |
| Wiki copy served | `/usr/share/unheaded/wiki` (root; refreshed by hand with rsync in Step 137) |
| Backups made in Phase 8 | `/var/backups/vor-ff4c7e4.bak`, `/var/backups/wiki-server-2026-11-11.bak` |
| Collisions on 2026-11-12 | `chaos-engineering` testing/wiki, `sbom` security/external, `distributed-systems` cs-theory/patterns, `saml` auth/security, `vpc` networking/cloud |

---

*S-WIKI-VOR Battle Plan, forged 2026-11-12*
*10 Phases. 142 Steps. One client, exact names, links that go somewhere.*
