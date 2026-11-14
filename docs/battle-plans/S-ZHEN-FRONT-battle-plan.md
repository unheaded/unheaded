<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-ZHEN-FRONT BATTLE PLAN: 13 Phases, 178 Steps

**Date**: 2026-11-12
**Sprint**: S-ZHEN-FRONT. The Zhen page and its units start, log in and answer the same way after a reboot, with no route open before login. Bundle 1 of `docs/sweeps/BACKLOG-SWEEP-2026-11-12.md` (rows D-01 to D-07).
**Prerequisite**: Stevie's answers of 2026-11-12, round 2 (`~/tmp/next.md`, "Session 2026-11-12 evening"): A-16 = close the highlighter and drop `session_id` from `/health`, prove it, then open on the LAN; A-36 = system units for llama-server and the app, each under its own nologin account, enabled but not started until the pause lifts; A-27 = (a) the compliance page reads from the Well.
**Target**: every Zhen route but the page shell, `/health`, `/auth/*` and the stylesheet answers 401/403 without a login (proven by a route list that cannot drift); llama-server and the app run as `unheaded-llama` and `unheaded-zhen-app` system units; the compliance page reports `source="well"`; the five smaller defects (D-02, D-03, D-04, D-06, the front and daemon start-up) are fixed with tests.
**Estimated duration**: 9–12 h over 3 sessions. Phases 1–8 are code (no stack, no sudo). Phases 9–10 are host installs (sudo, attended). Phase 11 needs the pause lifted (Stevie).
**Agent strategy**: Phases 1–6 and 8 touch separate files and may run in parallel `[P]` on one branch if each agent stages only its own files. Phase 7 follows Phase 6 (the unit needs the env-driven paths). Phases 9–12 run one after another on west, in a single coordinator session.
**Commit cadence**: every 5 steps, at every exit gate, before every skip. Signed (`git commit -S`), named files only (never `git add -A <dir>`), with the session's attribution trailer. Stevie pushes.
**Stuck protocol**: skip after 3× the step estimate or two failed debug attempts, mark the step `[STUCK]` and its dependents `[BLOCKED]`, commit, move on.

## Grounding (read before Step 1)

Every row was checked against `develop` at `a31be33b` (2026-11-12). Where the sweep and the tree disagree, the tree wins and the row is corrected here.

| Row | Sweep says | Tree says (checked) | Plan change |
|---|---|---|---|
| D-01 | `/api/v1/highlight` open; `/health` shows `session_id` | `raft/zhen_app.py:215-218` `_LOGIN_OPEN` lists `/api/v1/highlight` and `.css`; `:115` `_SCRIPTED_ROUTES` lists it; `:758-766` `/health` returns `session_id` and the raw `startup_error`. The CSRF tests use the open route in 9 places (`raft/tests/test_zhen_app_gate.py:81-117, 366-391`); `test_zhen_login.py:314` asserts `.css` stays open. The page calls `/api/v1/highlight` only after an answer (`raft/static/index.html:1606-1610`), so after login. | Holds. `/api/v1/highlight.css` **stays open**: a constant stylesheet the login page shell itself loads (`index.html:1035`). `startup_error` on the open route becomes a fixed word; the text moves behind login (same leak class, same route). |
| D-02 | daemon answers show `0 tok`; add CCNA/JNCIA to the quality gate | `cmd/zhen-agentd/main.go:634-640` `askResponse` has no token field; usage is read (`:1417-1440`) but only reaches the audit row. The coding gate's prompts are pre-registered (`eval/two-brains/PREREG.md`; `cmd/zhen-eval/main_test.go:28` pins 56). Retrieval regressions for both questions already exist (`raft/tests/test_zhen_rag_retrieve.py`). | Token count: sum usage per ask, return it. Certification questions are a **grounding** check, not a coding one: a new live probe `eval/grounding/`, not an edit to the pre-registered set. |
| D-03 | login lasts 15 minutes; `?` before login | `cmd/unheaded-auth/issuer.go:34` `tokenLifetime = 15 * time.Minute`; `pkg/bff` sessions end with the token too, so every page has the same 15 minutes. Stats are fetched once at page load (`index.html:2055-2079`, `:2501-2517`) and `/api/v1/stats` needs a login. | Fix the `?` (refetch after login) and the expiry experience (keep the typed question, show the login panel). **Lengthening the session is ADR-114 N5 work** (Well-backed sessions, idle 30 min / absolute 12 h) and D-08's SSO ADR: parked, not in this plan. |
| D-04 | `-gated` starts the old `./bin/zhen-agentd`; stop hint is `pkill -f` | `raft/start-zhen.sh:107` `nohup ./bin/zhen-agentd` (2026-10-29 build; `bin/release/zhen-agentd` is 2026-11-12); `:260` `pkill -f` x3. | Holds. |
| D-05 | units for llama-server and the app; daemon gives up after five quick failures; the front exits when its LAN address is missing | No unit or account exists for either. llama-server's RUNPATH points into `~/tmp/unheaded/llama.cpp/build/bin` (read by `ldd`), unreadable under `ProtectHome=yes`. The app hard-codes `Path.home()` in six places (`zhen_app.py:182-183, 1385, 1889, 1939, 2187, 2277`) and its venv (`~/.venv/zhen`, 8.4 G, torch `2.10.0+cu128`) and the embedder's Hugging Face cache live in govan's home. `zhen-agentd.service` has `RestartSec=5s` and the default `StartLimitBurst=5`/`10s`: the "gives up" claim is a **forecast**, never observed (journal since 03:35Z shows no failed start). `cmd/zhen-ui-front/main.go:640-647` returns on the first failed `Listen`, so a missing address stops loopback too. | Code first (Phase 6): one `ZHEN_PROJECT_ROOT`, `IP_FREEBIND` on non-loopback listens, `StartLimitIntervalSec=0`. Then accounts and units (Phase 7), host install (Phase 9). |
| D-06 | unknown id shows the nearest sheet; Pygments on untrusted sheets | Live: `GET 127.0.0.1:20110/api/topics/nosuchtopic123` answers 200 with a Wotan document. `view_source()` (`zhen_app.py:2615-2645`) returns it. The renderer's 2 s budget (`raft/zhen_markdown.py:47, 611-622`) is checked between blocks, not inside one Pygments call. | App side: exact `name` or 404. `external` sheets render code plain. The vor-side fix is D-11 (S-WIKI-VOR), not here. |
| D-07 | dashboard service has no `WELL_OPS_PASSWORD` | `docker-compose.yml:1285-1341` gives `dashboard-backend` `ops_writer` for another path and no `WELL_OPS_PASSWORD`/`WELL_CONFIG_PASSWORD`; `server.go:716-722` then reads files only (FND-007). Roles `ops_reader` and `config_reader` **already exist** with `SELECT` on the compliance tables (`db/migrations/013_compliance_config.sql:186`, `014_compliance_ops.sql:173`, `025_well_owner_roles.sql:41-43`), their passwords are in `/etc/unheaded/compose.env` and were rotated 2026-11-12. | **No new role and no migration** (Erda, below). Two compose lines with `:?` so the service fails loud. |

**Skill passes.**
- **Marshal** (scope): in lane = D-01..D-07 as corrected above. Parked with a home: session lifetime → ADR-114/D-08; vor exact-id → D-11 (S-WIKI-VOR); `config_admin` for the profile switch → not asked by A-27(a), stays 503 without its password; `/var/zhen` owned by govan and `-audit-well` → A-58; a `pkg/ports` constant for llama's 8081 → not this sprint. Overlap guard: S-RESIDUE owns the `~/.config/cs` text (D-19), including `zhen_app.py:2146` and `raft/scripts/zhen_rag.py:1081, 1096`; this plan does not touch those lines. S-WATCH-TRUE owns `ZhenSchedulerStalled` (D-13); this plan adds no alert rule.
- **Busboy** (coordination): three plans edit the tree at once. Shared files: `wiki/Systemd-Units.md` (S-RESIDUE adds the `unheaded-vor.service` row, D-28) and `deploy/systemd/README.md`. Rule: whichever lands second rebases and keeps both rows. `raft/zhen_app.py` is this plan's alone except the one string S-RESIDUE owns.
- **Micromanager** (QA, DoD): red test before every code change; `go test -race` for Go; the raft suite whole, not one file, before each commit; the route list is a golden file the app is tested against, so the live proof cannot test a stale list; no unit enabled without Stevie's word on boot behaviour (Step 141); isolation: nothing here reads adopter data, the app's Well role stays `app_zhen`.
- **Erda** (Well role): Law 7 is already met: `ops_reader` and `config_reader` are least privilege and exist; adding a role would be a second reader for the same tables. Law 1: no DDL. Gate: after the change, `unheaded_compliance_source{source="well"} == 1` for each kind, and `ComplianceWellFallback` does not fire. Note for later (not this plan): `dashboard-backend` also holds `ops_writer`; which path writes deserves its own look.

**Constraints**: no new third-party code (the runtime venv uses the same packages at the same versions as `~/.venv/zhen` today; Step 117 checks that); secrets are written unseen and never printed; no `pkill -f` (use PIDs or `systemctl`); dated records are not edited; the runbook manifest is `--write` only; the Zhen pause stands until Phase 11, and Phase 11 starts only on Stevie's word.

---

## LEGEND

- `[B]`: bash command, run directly
- `[V]`: verification; it must pass before you go on
- `[D]`: debug step, only if the step before it failed
- `[W]`: write or edit a file
- `[R]`: read or inspect
- `[S]`: needs sudo (attended)
- `[P]`: can run in parallel with other `[P]` steps
- `[C]`: commit checkpoint (signed, named files)
- `[STEVIE]`: Stevie runs it or says yes first
- `[STUCK]` / `[BLOCKED]`: set by the Skip Protocol

`~Nm` is the expected wall-clock time. `WT` below is `/home/govan/tmp/wt/s-zhen-front`.

---

## PHASE 0: PRECONDITIONS (Steps 1–10)

**Goal**: a clean worktree on `sprint/s-zhen-front`, a green baseline, the pause state recorded.
**Prerequisite**: none.
**Time**: 15 min.
**Agent**: Coordinator.

- [ ] **Step 1** [B] ~1m: Confirm develop is clean and current.
  ```bash
  cd /home/govan/tmp/unheaded && git status -sb | head -3 && git log --oneline -1
  ```
- [ ] **Step 2** [B] ~1m: Make the worktree.
  ```bash
  cd /home/govan/tmp/unheaded && git worktree add /home/govan/tmp/wt/s-zhen-front -b sprint/s-zhen-front develop
  ```
- [ ] **Step 3** [V] ~1m: Worktree exists on the new branch.
  ```bash
  git -C /home/govan/tmp/wt/s-zhen-front branch --show-current   # sprint/s-zhen-front
  ```
- [ ] **Step 4** [B] ~3m: Raft baseline.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front/raft && /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests -q 2>&1 | tail -3
  ```
- [ ] **Step 5** [V] ~1m: All pass. If not → Step 6.
- [ ] **Step 6** [D] ~5m: Record the failing test names in the progress file as pre-existing; do not fix them here unless a later step touches them.
- [ ] **Step 7** [B] ~4m: Go baseline for the packages this plan touches.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && go test -race ./cmd/zhen-agentd/... ./cmd/zhen-ui-front/... ./pkg/uids/... ./cmd/dashboard-backend/... 2>&1 | tail -8
  ```
- [ ] **Step 8** [V] ~1m: All `ok`. Failures → record as pre-existing, as Step 6.
- [ ] **Step 9** [R] ~2m: Record the pause state (read-only).
  ```bash
  systemctl is-active zhen-agentd unheaded-zhen-ui-front unheaded-vor; systemctl --user is-active unheaded-llama unheaded-zhen-app 2>/dev/null; ss -ltnH '( sport = :8081 or sport = :20103 or sport = :20111 )' | awk '{print $4}'
  ```
- [ ] **Step 10** [W][C] ~3m: Start `docs/battle-plans/progress/S-ZHEN-FRONT-progress.md` (date, HEAD, baselines, pause state); commit it.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add docs/battle-plans/progress/S-ZHEN-FRONT-progress.md && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 1-10: progress file, baselines"
  ```
  **PHASE 0 EXIT GATE**: worktree on the branch, baselines recorded.

---

## PHASE 1: NO ROUTE OPEN BEFORE LOGIN (D-01) (Steps 11–34)

**Goal**: `POST /api/v1/highlight` needs a login (any role); `/health` carries no session id and no error text; the CSRF suite tests the same guarantees through a logged-in session; a golden route list makes the live proof non-drifting.
**Prerequisite**: Phase 0.
**Time**: 75 min.
**Agent**: Agent [P].

### Tests first

- [ ] **Step 11** [W] ~10m: In `raft/tests/test_zhen_login.py`, keep `test_the_page_and_the_probe_stay_open` (`:314`) as it is and add `test_highlight_needs_a_login`: `POST /api/v1/highlight` with `{'blocks': []}` and no session → 401; with a viewer session → 200 (it only reads).
- [ ] **Step 12** [W] ~10m: In the same file add `test_health_names_no_session_and_no_error_text`: `/health` JSON keys ⊆ `{status, rag_ready, well_connected, error}`; `session_id` absent; with `zhen_app.startup_error` monkeypatched to `'boom /home/x'`, the open answer's `error` is `'startup_failed'` and contains no `/`.
- [ ] **Step 13** [W] ~15m: In `raft/tests/test_zhen_app_gate.py`, the CSRF tests at lines 81–117 and 374–391 call `_log_in(client)` first and keep `/api/v1/highlight` as their target (now a logged-in write-shaped POST). `test_csrf_non_browser_post_is_not_csrf` and `test_scripted_routes_stay_open_to_a_caller_with_no_browser_headers` (`:366`) move to `/auth/login` with an empty JSON body and assert "not 403 for CSRF" (400 or 503 are both fine: no issuer in the test).
- [ ] **Step 14** [W] ~15m: Add `raft/tests/zhen_routes.golden` (one `METHOD PATH` per line, sorted) and `test_route_list_is_current`: builds the list from `zhen_app.app.url_map` (skip `static`), compares to the file; `ZHEN_UPDATE_GOLDEN=1` rewrites it.
- [ ] **Step 15** [W] ~10m: Add `test_every_route_but_the_open_ones_refuses_a_stranger`: for each golden line not in `_LOGIN_OPEN`, request it three ways (no headers; same-origin browser headers with a CSRF cookie but no session; cross-site headers) and assert 401 or 403, and that no handler ran (monkeypatch `rag` to a sentinel that raises if touched).
- [ ] **Step 16** [B] ~2m: Run them: they must fail now (red).
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front/raft && /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests/test_zhen_login.py tests/test_zhen_app_gate.py -q 2>&1 | tail -8
  ```
- [ ] **Step 17** [V] ~1m: Red for the new reasons only (highlight 200 without login; `session_id` present; the golden file missing). Any other failure → Step 18.
- [ ] **Step 18** [D] ~5m: A fixture problem (cookie domain `zhen.test` vs `127.0.0.1`): use the file's own `_log_in`/`LOCAL` helpers; do not change the app to fit a test.

### Code

- [ ] **Step 19** [W] ~5m: `raft/zhen_app.py`: remove `/api/v1/highlight` from `_LOGIN_OPEN` (`:215-218`) and from `_SCRIPTED_ROUTES` (`:115`); add it to `_READ_POSTS` (`:222`) so a viewer login may use it. Update the comment above `_LOGIN_OPEN` ("the highlighter (which holds no data)" goes).
- [ ] **Step 20** [W] ~5m: `/health` (`:758-766`): drop `session_id`; `error` becomes `'startup_failed' if startup_error else None`. Add `startup_error` to `/api/v1/system/state` if it is not there already (behind login).
- [ ] **Step 21** [W] ~3m: `raft/start-zhen.sh:35`: "Login: everything but the page, `/health` and the stylesheet needs one."
- [ ] **Step 22** [B] ~1m: Generate the golden list once.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front/raft && ZHEN_UPDATE_GOLDEN=1 /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests/test_zhen_app_gate.py -q -k route_list_is_current 2>&1 | tail -2
  ```
- [ ] **Step 23** [R] ~5m: Read `raft/tests/zhen_routes.golden` by eye: every route expected, no test-only route. Count lines (the night session counted 45 routes; a different count is fine if each line is a real route).
- [ ] **Step 24** [B] ~3m: Whole raft suite.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front/raft && /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests -q 2>&1 | tail -3
  ```
- [ ] **Step 25** [V] ~1m: Green, nothing newly failing against Step 4.
- [ ] **Step 26** [D] ~5m: A page test reading `/health.session_id`: grep `raft/static/index.html` for `session_id` near `/health` (`:2055`); the page reads `status` only there; fix the test, not the route.
- [ ] **Step 27** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add raft/zhen_app.py raft/start-zhen.sh raft/tests/test_zhen_login.py raft/tests/test_zhen_app_gate.py raft/tests/zhen_routes.golden && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 11-27: highlighter behind the login, /health without session id, golden route list"
  ```

### The live proof script

- [ ] **Step 28** [W] ~20m: `scripts/zhen-lan-proof.sh BASE_URL` (no secrets; reads `raft/tests/zhen_routes.golden`): for each route, three requests with `curl --curves X25519MLKEM768 --cacert /etc/unheaded/tls/ca.pem` (no headers; same-origin `Origin`+`Sec-Fetch-Site` without a session; cross-site); open list `/`, `/health`, `/auth/login`, `/auth/logout`, `/auth/me`, `/api/v1/highlight.css`; every other answer must be 401 or 403; prints one summary line `PASS n/n` or the failing lines; exit 1 on any failure. `/health` body checked for no `session_id`.
- [ ] **Step 29** [W] ~10m: `tests/scripts/test_zhen_lan_proof.sh`: the front needs hybrid TLS, so the test covers the planning part only: a `--dry-run` flag prints the request plan; assert the count is 3 × routes and the open list is honoured.
- [ ] **Step 30** [B] ~1m: `bash -n` and the test.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && bash -n scripts/zhen-lan-proof.sh && bash tests/scripts/test_zhen_lan_proof.sh
  ```
- [ ] **Step 31** [V] ~1m: Pass.
- [ ] **Step 32** [B] ~2m: shellcheck if the CI image has it (it is in the gates); otherwise note it.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && (command -v shellcheck && shellcheck scripts/zhen-lan-proof.sh tests/scripts/test_zhen_lan_proof.sh) || echo "shellcheck not on PATH: CI runs it"
  ```
- [ ] **Step 33** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add scripts/zhen-lan-proof.sh tests/scripts/test_zhen_lan_proof.sh && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 28-33: zhen-lan-proof.sh reads the golden route list"
  ```
- [ ] **Step 34** [V] ~2m: **PHASE 1 EXIT GATE**: Steps 25 and 31 green; `grep -n "'/api/v1/highlight'" /home/govan/tmp/wt/s-zhen-front/raft/zhen_app.py` shows it only in `_READ_POSTS` and the route itself.

---

## PHASE 2: TOKEN COUNTS THROUGH THE DAEMON (D-02) (Steps 35–52)

**Goal**: an answer that went through `zhen-agentd` shows its real token count; the CCNA and JNCIA questions have a live grounding probe.
**Prerequisite**: Phase 0.
**Time**: 60 min.
**Agent**: Agent [P].

- [ ] **Step 35** [R] ~5m: Read `cmd/zhen-agentd/main.go:1396-1440` (`llamaLLM.Complete` and `complete`) and the ask handler near `:700-760`, to find where one ask's turns run.
- [ ] **Step 36** [W] ~15m: Test first, `cmd/zhen-agentd/usage_test.go`: a fake llama server answering two turns with `usage.prompt_tokens`/`completion_tokens` (10/3 then 12/5); `POST /api/v1/agent/ask` → JSON has `tokens_input: 22`, `tokens_output: 8`. A second test: llama returns no usage → both 0, no error.
- [ ] **Step 37** [B] ~2m: Red.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && go test -race ./cmd/zhen-agentd/ -run Usage 2>&1 | tail -5
  ```
- [ ] **Step 38** [W] ~15m: A per-request usage accumulator carried in the request context (unexported key type); `llamaLLM.Complete` adds each call's `llamaUsage` to it when present; `askResponse` gains `TokensInput int64 json:"tokens_input"` and `TokensOutput int64 json:"tokens_output"`; the stream handler's final event carries the same two fields. Update `handleOpenAPI`'s schema if it lists `askResponse` fields.
- [ ] **Step 39** [B] ~3m: Green, whole package, race on.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && go test -race ./cmd/zhen-agentd/... 2>&1 | tail -5
  ```
- [ ] **Step 40** [V] ~1m: `ok`.
- [ ] **Step 41** [W] ~10m: `raft/scripts/zhen_rag.py` `_generate_via_agentd` (`:934-1030`): `tokens_used = int(body.get('tokens_output') or 0)` (same meaning as the direct path, completion tokens, `:903-904`). Test in `raft/tests/test_zhen_agentd_proxy.py`: a fake daemon answer with `tokens_output: 8` → `tokens_used == 8`; missing → 0.
- [ ] **Step 42** [B] ~2m: Raft suite.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front/raft && /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests -q 2>&1 | tail -3
  ```
- [ ] **Step 43** [V] ~1m: Green.
- [ ] **Step 44** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add cmd/zhen-agentd/main.go cmd/zhen-agentd/usage_test.go raft/scripts/zhen_rag.py raft/tests/test_zhen_agentd_proxy.py && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 35-44: zhen-agentd returns token counts per ask; the page shows them"
  ```

### Grounding probe (CCNA, JNCIA)

- [ ] **Step 45** [W] ~10m: `eval/grounding/prompts.jsonl`, two rows: `{"id":"cert-ccna","prompt":"tell me about getting the ccna","expect":"no_source"}` and `{"id":"cert-jncia","prompt":"explain the steps towards Junos, Associate (JNCIA-Junos)","expect_sources_any":["junos-"]}`. `eval/grounding/README.md`: what it checks (the source line, not the answer's facts), why it is not in the pre-registered coding gate, how to run it.
- [ ] **Step 46** [W] ~15m: `eval/grounding/run.sh BASE_URL` reads a session cookie from `ZHEN_SESSION_FILE` (0600 file the operator made by logging in; the script never takes a password), posts each prompt to `/api/v1/query`, checks `sources` against the row, prints `PASS`/`FAIL` per id and the token count (it must be > 0 now, D-02).
- [ ] **Step 47** [B] ~1m: Syntax and a JSONL parse check.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && bash -n eval/grounding/run.sh && python3 -c "import json,sys;[json.loads(l) for l in open('eval/grounding/prompts.jsonl')];print('ok')"
  ```
- [ ] **Step 48** [V] ~1m: `ok`.
- [ ] **Step 49** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add eval/grounding/prompts.jsonl eval/grounding/README.md eval/grounding/run.sh && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 45-49: grounding probe for the CCNA and JNCIA questions"
  ```
- [ ] **Step 50** [R] ~2m: Confirm `cmd/zhen-eval` tests still pass untouched (the coding gate was not edited).
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && go test ./cmd/zhen-eval/... 2>&1 | tail -3
  ```
- [ ] **Step 51** [V] ~1m: `ok`.
- [ ] **Step 52** [V] ~1m: **PHASE 2 EXIT GATE**: Steps 40, 43, 48, 51 pass. The live run of `eval/grounding/run.sh` is Step 160.

---

## PHASE 3: THE PAGE BEFORE AND AFTER LOGIN (D-03) (Steps 53–64)

**Goal**: no `?` for sheets and categories once logged in; an expired session shows the login panel and keeps what the person typed.
**Prerequisite**: Phase 0.
**Time**: 45 min.
**Agent**: Agent [P].

- [ ] **Step 53** [R] ~5m: Read `raft/static/index.html` around `:2055-2080` (stats at load), `:2501-2520` (welcome text), and the login panel's success handler.
- [ ] **Step 54** [W] ~10m: Move the stats fetch into one `refreshStats()`; call it at load and again from the login success handler; before login show "log in to see" rather than `?`.
- [ ] **Step 55** [W] ~10m: In the chat send path, a 401 answer opens the login panel, keeps the question in the input box, and resends nothing by itself.
- [ ] **Step 56** [W] ~10m: Static test in `raft/tests/test_zhen_app_gate.py` (pattern of `test_csrf_page_sends_header_on_same_origin_posts`, `:122`): the page calls `refreshStats` from the login handler, and the 401 path does not clear the input.
- [ ] **Step 57** [B] ~2m: JS lint and raft suite.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && bash scripts/check-js-lint.sh 2>&1 | tail -3 && cd raft && /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests -q 2>&1 | tail -3
  ```
- [ ] **Step 58** [V] ~1m: Both pass.
- [ ] **Step 59** [D] ~5m: Lint fails on an inline handler: the page's CSP forbids inline script; use `addEventListener`.
- [ ] **Step 60** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add raft/static/index.html raft/tests/test_zhen_app_gate.py && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 53-60: stats after login; an expired session keeps the question"
  ```
- [ ] **Step 61** [W] ~5m: In the progress file, record the parked half: the 15-minute lifetime is the issuer's (`cmd/unheaded-auth/issuer.go:34`) and every page's; the fix is ADR-114 N5 and the SSO ADR (D-08).
- [ ] **Step 62** [C] ~1m: Commit the progress file.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add docs/battle-plans/progress/S-ZHEN-FRONT-progress.md && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 61-62: session lifetime parked to ADR-114"
  ```
- [ ] **Step 63** [V] ~1m: Browser check is Step 162 (needs Zhen up).
- [ ] **Step 64** [V] ~1m: **PHASE 3 EXIT GATE**: Step 58 green.

---

## PHASE 4: START-ZHEN.SH (D-04) (Steps 65–74)

**Goal**: `-gated` starts the current daemon build; the stop hint names PIDs, never `pkill -f`.
**Prerequisite**: Phase 0.
**Time**: 25 min.
**Agent**: Agent [P].

- [ ] **Step 65** [W] ~5m: `raft/start-zhen.sh:107`: run `./bin/release/zhen-agentd`; if it is missing, exit 1 with "run `make binaries` first".
- [ ] **Step 66** [W] ~10m: Record each PID the script starts (`zhen_app.py`, `zhen-ui-front`, `zhen-agentd`) in `"$XDG_RUNTIME_DIR/unheaded-zhen/<name>.pid"`; `:260` prints `Stop: kill $(cat <pidfile>) ...` for the ones it started, and `systemctl stop` for units it found running.
- [ ] **Step 67** [W] ~5m: Header comment: when `unheaded-zhen-app.service` or `unheaded-llama.service` is active, the script leaves them alone and says so (same pattern as the front at `:20-21`).
- [ ] **Step 68** [B] ~1m: Syntax, and confirm no `pkill` is left.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && bash -n raft/start-zhen.sh && ! grep -n 'pkill' raft/start-zhen.sh && echo "no pkill"
  ```
- [ ] **Step 69** [V] ~1m: `no pkill`.
- [ ] **Step 70** [B] ~1m: shellcheck (as Step 32).
- [ ] **Step 71** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add raft/start-zhen.sh && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 65-71: start-zhen.sh runs bin/release, stops by PID"
  ```
- [ ] **Step 72** [R] ~2m: `grep -rn "pkill -f zhen\|bin/zhen-agentd" /home/govan/tmp/wt/s-zhen-front/raft /home/govan/tmp/wt/s-zhen-front/scripts /home/govan/tmp/wt/s-zhen-front/wiki | grep -v release` lists other places that teach the old way.
- [ ] **Step 73** [W] ~5m: Fix what Step 72 finds outside dated records; commit with Step 74.
- [ ] **Step 74** [C][V] ~2m: **PHASE 4 EXIT GATE**: Steps 69 and 72 clean; commit any Step 73 files by name.

---

## PHASE 5: SOURCE VIEWER (D-06) (Steps 75–86)

**Goal**: an unknown topic id is a 404; code in `external` sheets is shown plain.
**Prerequisite**: Phase 0.
**Time**: 40 min.
**Agent**: Agent [P].

- [ ] **Step 75** [W] ~10m: Tests in `raft/tests/test_zhen_app_gate.py`: (a) `_vor` answering a topic whose `name` differs from the asked id → `/api/v1/source?id=x` is 404 `{"error":"no such topic"}`; (b) the same name → 200; (c) a payload with no `name` → 404.
- [ ] **Step 76** [W] ~10m: Test: an `external` sheet's fenced block comes back escaped with no `class="hl-`; a `canonical` sheet's still has it. Change `test_source_api_returns_the_sheet_rendered_and_raw` (`:544`), which asserts highlighting on an `external` sheet, to use `source_trust: canonical`; add the external case beside it.
- [ ] **Step 77** [B] ~2m: Red.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front/raft && /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests/test_zhen_app_gate.py -q -k source 2>&1 | tail -5
  ```
- [ ] **Step 78** [W] ~5m: `view_source()` (`zhen_app.py:2633-2640`): after the lookup, `if t.get('name') != source_id: return 404`. Same check on the path branch (`:2645-2655`): the topic fetched must carry the `name` the search hit named.
- [ ] **Step 79** [W] ~10m: `zhen_markdown.render(source, budget_s=..., highlight=True)`; `_source_payload` passes `highlight=(trust != 'external')`. Pygments is not called for `external` text at all.
- [ ] **Step 80** [B] ~2m: Raft suite.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front/raft && /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests -q 2>&1 | tail -3
  ```
- [ ] **Step 81** [V] ~1m: Green.
- [ ] **Step 82** [D] ~5m: `test_zhen_markdown.py` calls `render()` positionally: keep `highlight` keyword-only with default `True`.
- [ ] **Step 83** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add raft/zhen_app.py raft/zhen_markdown.py raft/tests/test_zhen_app_gate.py && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 75-83: source viewer 404s an unknown id; external sheets are not highlighted"
  ```
- [ ] **Step 84** [W] ~3m: Progress file: the vor-side exact-id and word-boundary fixes stay with D-11 (S-WIKI-VOR).
- [ ] **Step 85** [C] ~1m: Commit the progress file by name.
- [ ] **Step 86** [V] ~1m: **PHASE 5 EXIT GATE**: Step 81 green.

---

## PHASE 6: START-UP HARDENING IN CODE (D-05, part 1) (Steps 87–108)

**Goal**: the app finds everything from one `ZHEN_PROJECT_ROOT`, not from a home directory; the front keeps serving loopback when a LAN address is missing; the daemon never stops retrying.
**Prerequisite**: Phase 0.
**Time**: 75 min.
**Agent**: Agent (Phase 7 waits on this).

### The app's paths

- [ ] **Step 87** [W] ~10m: Test `raft/tests/test_zhen_paths.py`: with `ZHEN_PROJECT_ROOT=/tmp/x`, `zhen_app.RUNBOOKS_DIR`, the skills dir, the champion read roots and the `UNHEADED_ROOT` fallback all sit under `/tmp/x`; unset, they keep today's value (`~/tmp/unheaded`). A grep test: `raft/zhen_app.py` contains no `Path.home()` and no `expanduser('~` outside the one function that computes the default.
- [ ] **Step 88** [B] ~2m: Red.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front/raft && /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests/test_zhen_paths.py -q 2>&1 | tail -4
  ```
- [ ] **Step 89** [W] ~15m: `raft/zhen_app.py`: one `_project_root()` (env `ZHEN_PROJECT_ROOT`, default `~/tmp/unheaded` expanded once); lines `182-183` (vestigial `index_dir`/`corpus_dir`, keep the names, derive from it), `1385`, `1889`, `1939`, `2187`, `2277` use it. Do not touch `:2146` (S-RESIDUE owns that string).
- [ ] **Step 90** [B] ~2m: Raft suite.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front/raft && /var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests -q 2>&1 | tail -3
  ```
- [ ] **Step 91** [V] ~1m: Green.
- [ ] **Step 92** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add raft/zhen_app.py raft/tests/test_zhen_paths.py && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 87-92: zhen_app reads one ZHEN_PROJECT_ROOT, no home directory"
  ```

### The front

- [ ] **Step 93** [W] ~10m: Test in `cmd/zhen-ui-front/main_test.go`: `--listen 127.0.0.1:0 --listen 192.0.2.1:<port>` (TEST-NET-1, never on a host) starts, serves on loopback, and logs one warning naming `192.0.2.1`. An IPv6 twin with `2001:db8::1`.
- [ ] **Step 94** [B] ~2m: Red.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && go test -race ./cmd/zhen-ui-front/ -run Freebind 2>&1 | tail -4
  ```
- [ ] **Step 95** [W] ~15m: `cmd/zhen-ui-front/main.go:640`: a `net.ListenConfig` whose `Control` sets `IP_FREEBIND` (IPv4) or `IPV6_FREEBIND` (IPv6, `golang.org/x/sys/unix`, already in `go.mod`) for every non-loopback address. A loopback address that fails still fails the start. Comment why: the LAN address may come after the unit (`zhen_lan.sh` drop-in), and a missing address must not take loopback down.
- [ ] **Step 96** [B] ~3m: Package tests, race.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && go test -race ./cmd/zhen-ui-front/... 2>&1 | tail -4
  ```
- [ ] **Step 97** [V] ~1m: `ok`.
- [ ] **Step 98** [D] ~5m: Freebind in the sandbox: it needs no capability; if the unit test needs `CAP_NET_ADMIN` something else is set. `SocketBindAllow=tcp:20103` in the unit is by port and still holds.
- [ ] **Step 99** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add cmd/zhen-ui-front/main.go cmd/zhen-ui-front/main_test.go && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 93-99: zhen-ui-front binds a missing LAN address with FREEBIND"
  ```

### The daemon's unit

- [ ] **Step 100** [W] ~5m: `deploy/systemd/zhen-agentd.service` `[Unit]`: `StartLimitIntervalSec=0` (retry forever at `RestartSec`), `RestartSec=10s`, `After=docker.service wg-quick@wg0.service` (ordering only, harmless where absent). Comment: compose and `wg0` may come minutes after boot (dockerd took 3 min on 2026-11-12).
- [ ] **Step 101** [B] ~3m: Unit gates.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && bash scripts/check-systemd-units.sh 2>&1 | tail -2 && bash scripts/check-unit-flags.sh 2>&1 | tail -2 && bash scripts/check-unit-ceilings.sh 2>&1 | tail -2
  ```
- [ ] **Step 102** [V] ~1m: Three `[PASS]`.
- [ ] **Step 103** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add deploy/systemd/zhen-agentd.service && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 100-103: zhen-agentd keeps retrying while the Well or wg0 is late"
  ```
- [ ] **Step 104** [R] ~3m: `cmd/zhen-ui-front/units_test.go`: does it assert the unit's `ExecStart` or `[Unit]` keys? Adjust if Step 96 or 101 flagged it.
- [ ] **Step 105** [B] ~3m: `go vet` on the touched packages.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && go vet ./cmd/zhen-ui-front/... ./cmd/zhen-agentd/... 2>&1 | tail -3
  ```
- [ ] **Step 106** [V] ~1m: Clean.
- [ ] **Step 107** [W] ~3m: Progress file: the five-strike give-up was a forecast; Step 172 checks the next boot.
- [ ] **Step 108** [C][V] ~2m: **PHASE 6 EXIT GATE**: Steps 91, 97, 102, 106 pass; commit the progress file.

---

## PHASE 7: ACCOUNTS AND UNITS IN THE REPO (D-05, part 2) (Steps 109–132)

**Goal**: `unheaded-llama` (16792) and `unheaded-zhen-app` (16758) exist in the registry and in sysusers; both units, the socket directory line, the env example and the runtime lock are in the repo and pass every unit gate.
**Prerequisite**: Phase 6.
**Time**: 90 min.
**Agent**: Agent.

### Registry

- [ ] **Step 109** [W] ~10m: `pkg/uids/uids.go`: `ZhenApp = 16758` in the Kingdom tier (comment: host unit `unheaded-zhen-app.service`, the web UI's Flask app; move the "Next free" note from `WikiServer` to say the Kingdom tier is full); `Llama = 16792` in the infrastructure tier after `UPC` (comment: GPU, groups render 992 and video 44, like the reserved vLLM 16781). Add both to the name map near `:245-249`.
- [ ] **Step 110** [W] ~5m: `pkg/uids/uids_test.go` `hostOnly`: `"zhen-app": "unheaded-zhen-app"`, `"llama": "unheaded-llama"`, each with its unit named.
- [ ] **Step 111** [W] ~5m: `deploy/sysusers.d/unheaded.conf`: `u unheaded-zhen-app 16758 "Unheaded Zhen web UI app" / /usr/sbin/nologin`; `u unheaded-llama 16792 "llama-server for Zhen (GPU)" / /usr/sbin/nologin`; `m unheaded-llama render`; `m unheaded-llama video`.
- [ ] **Step 112** [B] ~2m: Registry tests.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && go test -race ./pkg/uids/... 2>&1 | tail -3
  ```
- [ ] **Step 113** [V] ~1m: `ok` (`TestHostOnlySysusersPinned` reads the pins).
- [ ] **Step 114** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add pkg/uids/uids.go pkg/uids/uids_test.go deploy/sysusers.d/unheaded.conf && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 109-114: accounts unheaded-zhen-app (16758) and unheaded-llama (16792)"
  ```

### Runtime lock for the app

- [ ] **Step 115** [W] ~10m: `raft/runtime/requirements.in`: only what the app imports at run time, at today's pins (`Flask==3.1.3`, `requests==2.34.2`, `Pygments==2.21.0`, `psycopg2-binary==2.9.11`, `sentence-transformers==5.7.0`, `numpy==2.4.3`, `torch==2.10.0`, `transformers==5.17.0`, `PyYAML==6.0.3`, plus `faiss-cpu`/`langchain-text-splitters` only if `grep -n "^import\|^from" raft/*.py raft/scripts/zhen_rag.py` shows the app imports them). No `datasets`, `peft`, `trl`, `anthropic`, `pytest`.
- [ ] **Step 116** [B] ~10m: Hash-locked resolution, the house way (in a container, no package code on the host).
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && bash scripts/ci-python-lock.sh raft/runtime 2>&1 | tail -5
  ```
- [ ] **Step 117** [V] ~5m: Every package name in `raft/runtime/requirements.txt` is already in the dependency baseline (ADR-096), and each version equals `~/.venv/zhen`'s.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && d=$(mktemp -d) && ~/.venv/zhen/bin/pip freeze 2>/dev/null | tr 'A-Z_' 'a-z-' | sort > "$d/old" && grep -E '^[A-Za-z0-9_.-]+==' raft/runtime/requirements.txt | tr 'A-Z_' 'a-z-' | sort > "$d/new" && comm -13 "$d/old" "$d/new"; rm -rf "$d"
  ```
  - Empty output → pass. Any line → a version differs or a package is new: Step 118.
- [ ] **Step 118** [D] ~5m: A different version: pin the `.in` to the old one and rerun Step 116. A new package name: STOP and ask Stevie (third-party rule). torch's CUDA build (`+cu128`, several GB of NVIDIA wheels on an AMD host) is what runs today; a CPU-only wheel would come from another index, which is a new source: **[STEVIE]** decides, default is today's build.
- [ ] **Step 119** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add raft/runtime/requirements.in raft/runtime/requirements.txt && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 115-119: hash-locked runtime requirements for the Zhen app (same pins)"
  ```

### Units

- [ ] **Step 120** [W] ~20m: `deploy/systemd/unheaded-llama.service`: `User=`/`Group=unheaded-llama`, `SupplementaryGroups=render video`; `Environment=HIP_VISIBLE_DEVICES=0` and `HSA_OVERRIDE_GFX_VERSION=11.0.1` (without them the model splits over both GPUs and aborts, next.md "Off switches"); `ExecStart=/opt/unheaded/llama/bin/llama-server -m /var/zhen/models/qwen2.5-coder-7b-instruct-q4_k_m.gguf --host 127.0.0.1 --port 8081 --n-gpu-layers 999 --ctx-size 16384 --threads 6`; `MemoryMax=6G`, `MemorySwapMax=0`; `ProtectSystem=strict`, `ProtectHome=yes`, `ReadOnlyPaths=/var/zhen/models`; `PrivateDevices=no`, `DevicePolicy=closed`, `DeviceAllow=/dev/kfd rw`, `DeviceAllow=char-drm rw`; `IPAddressDeny=any`, `IPAddressAllow=localhost`, `SocketBindDeny=any`, `SocketBindAllow=tcp:8081`; `CapabilityBoundingSet=`, `NoNewPrivileges=yes`; `Restart=on-failure`, `RestartSec=10s`. `MemoryDenyWriteExecute` left off with a comment (the ROCm runtime maps code at run time; Step 150 checks).
- [ ] **Step 121** [W] ~20m: `deploy/systemd/unheaded-zhen-app.service`: `User=`/`Group=unheaded-zhen-app`; `After=unheaded-llama.service zhen-agentd.service unheaded-vor.service`, `Wants=unheaded-llama.service`; `WorkingDirectory=/usr/local/lib/unheaded/zhen-app/raft`; `ExecStart=/usr/local/lib/unheaded/zhen-app/venv/bin/python3 /usr/local/lib/unheaded/zhen-app/raft/zhen_app.py`; `EnvironmentFile=/etc/unheaded/zhen-app.env`; `Environment=ZHEN_PROJECT_ROOT=/usr/local/share/unheaded/zhen ZHEN_UI_SOCKET=/run/unheaded-zhen-ui/app.sock HF_HOME=/var/lib/unheaded-zhen-app/hf HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 AUTH_ISSUER_URL=https://auth:19020 UNHEADED_TLS_ENABLED=true UNHEADED_TLS_CA_CERT=/etc/unheaded/tls/ca.pem VOR_URL=http://127.0.0.1:20110 ZHEN_INFERENCE_URL=http://127.0.0.1:8081 ZHEN_AGENTD_URL=http://127.0.0.1:20111`; `StateDirectory=unheaded-zhen-app` (0700); `MemoryMax=3G`, `MemorySwapMax=0`; `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`, `PrivateDevices=yes`; `IPAddressDeny=any`, `IPAddressAllow=localhost`; `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`; `CapabilityBoundingSet=`, `NoNewPrivileges=yes`; `UMask=0007` (the socket is 0660 in a setgid 2750 directory, `deploy/systemd/README.md#zhen-ui-front`); `Restart=on-failure`, `RestartSec=10s`. `MemoryDenyWriteExecute` off with a comment if torch needs it (Step 152 checks).
- [ ] **Step 122** [W] ~5m: `deploy/tmpfiles.d/unheaded-zhen-ui.conf`: `d /run/unheaded-zhen-ui 2750 unheaded-zhen-app unheaded-zhen-ui-front -`. `deploy/systemd/README.md#zhen-ui-front`: the app has its account now; the `<app account>` placeholder becomes this file.
- [ ] **Step 123** [W] ~10m: `deploy/systemd/zhen-app.env.example`: the keys with no values (`ZHEN_DB_PASSWORD=`, `ZHEN_DB_HOST=`, `ZHEN_DB_PORT=5432`, `ZHEN_DB_SSLMODE=verify-full`, `ZHEN_DB_SSLROOTCERT=/etc/unheaded/well-tls/ca.crt`, `ZHEN_ALLOWED_HOSTS=`, `ZHEN_EMBEDDER_SHA256=`, `ZHEN_EMBEDDER_REVISION=`) and a comment per key. No secret.
- [ ] **Step 124** [W] ~15m: `scripts/write-zhen-app-env.sh --db-host HOST:PORT [--allowed-hosts LIST]`: run as root; reads `APP_ZHEN_PASSWORD` from `/etc/unheaded/compose.env` without printing it, writes `/etc/unheaded/zhen-app.env` 0640 `root:unheaded-zhen-app` atomically (temp file + `mv`), prints only the key names it wrote. Refuses if `/etc/unheaded/compose.env` is missing or the key is empty (fail loud).
- [ ] **Step 125** [W] ~5m: Test `tests/scripts/test_write_zhen_app_env.sh`: runs the script against a temp `compose.env` and a temp target (`--compose-env`/`--out` test flags), asserts mode, owner fields requested, that the value is present in the file and absent from stdout and stderr.
- [ ] **Step 126** [B] ~2m: Run it.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && bash tests/scripts/test_write_zhen_app_env.sh
  ```
- [ ] **Step 127** [V] ~1m: Pass.
- [ ] **Step 128** [B] ~4m: Every unit gate, plus the binaries gate (`llama-server` is not under `/usr/local/bin`; if the gate flags it, add `llama-server` to `THIRD_PARTY` in `scripts/build-binaries.sh:57` with a comment).
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && for g in check-systemd-units check-unit-flags check-unit-ceilings check-unit-binaries; do bash scripts/$g.sh 2>&1 | tail -1; done
  ```
- [ ] **Step 129** [V] ~1m: Four `[PASS]`. `check-systemd-units.sh` also checks each `User=` exists in sysusers (Step 111).
- [ ] **Step 130** [D] ~5m: `systemd-analyze verify` complains about `DeviceAllow=char-drm`: use `DeviceAllow=/dev/dri rw` with a comment; rerun Step 128.
- [ ] **Step 131** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add deploy/systemd/unheaded-llama.service deploy/systemd/unheaded-zhen-app.service deploy/tmpfiles.d/unheaded-zhen-ui.conf deploy/systemd/README.md deploy/systemd/zhen-app.env.example scripts/write-zhen-app-env.sh tests/scripts/test_write_zhen_app_env.sh && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 120-131: units for llama-server and the Zhen app, socket dir, env writer"
  ```
- [ ] **Step 132** [V] ~1m: **PHASE 7 EXIT GATE**: Steps 113, 117, 127, 129 pass. (Add `scripts/build-binaries.sh` to the commit if Step 128 changed it.)

---

## PHASE 8: COMPLIANCE PAGE READS THE WELL (D-07) (Steps 133–138)

**Goal**: `dashboard-backend` reads compliance as `ops_reader` and `config_reader` and refuses to start without their passwords.
**Prerequisite**: Phase 0.
**Time**: 20 min.
**Agent**: Agent [P].

- [ ] **Step 133** [W] ~5m: `docker-compose.yml`, `dashboard-backend` environment (`:1333-1338` block): `WELL_OPS_PASSWORD: "${OPS_READER_PASSWORD:?set OPS_READER_PASSWORD in .env (credentials are not stored in this repo)}"` and `WELL_CONFIG_PASSWORD: "${CONFIG_READER_PASSWORD:?set CONFIG_READER_PASSWORD in .env (credentials are not stored in this repo)}"`. Users default to `ops_reader`/`config_reader` (`pkg/compliance/well/ops.go:33-44`); no `WELL_OPS_USER` line. Host, SSL mode and CA come from the existing `WELL_*` keys.
- [ ] **Step 134** [B] ~2m: Compose gates and the resolved config (values are not printed: `config --hash`).
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && bash scripts/check-compose-hardening.sh 2>&1 | tail -1 && bash scripts/check-gitleaks.sh 2>&1 | tail -1
  ```
- [ ] **Step 135** [V] ~1m: Both pass.
- [ ] **Step 136** [B] ~3m: The dashboard's compliance tests still pass (they cover the fallback labels).
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && go test -race ./cmd/dashboard-backend/internal/server/ -run Compliance 2>&1 | tail -3
  ```
- [ ] **Step 137** [C] ~2m: Commit.
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && git add docker-compose.yml && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 133-137: dashboard-backend reads compliance from the Well as ops_reader and config_reader"
  ```
- [ ] **Step 138** [V] ~1m: **PHASE 8 EXIT GATE**: Steps 135, 136 pass. The live check is Step 147.

---

## PHASE 9: MERGE TO DEVELOP (Steps 139–142)

**Goal**: Phases 1–8 on `develop`, signed, with the full gates green.
**Prerequisite**: Phases 1–8 exit gates.
**Time**: 30 min.
**Agent**: Coordinator.

- [ ] **Step 139** [B] ~15m: Full local CI for the touched areas (CI venv last on PATH, as the standing rules say).
  ```bash
  cd /home/govan/tmp/wt/s-zhen-front && PATH="$PATH:/var/tmp/unheaded-sprint/civenv/bin" bash scripts/ci-local.sh 2>&1 | tail -15
  ```
- [ ] **Step 140** [V] ~2m: Read each gate's result on its own line. Any FAIL → fix on the branch first; INCONCLUSIVE because ruff or pip-audit is missing → note it (D-32, S-RESIDUE).
- [ ] **Step 141** [STEVIE] ~1m: Ask once, before any host step: "Enable the two new units now (they would start at the next boot, which ends the pause at that boot), or install them disabled and enable them when you lift the pause?" Record the answer in the progress file. Default without an answer: install disabled. **Answered 2026-11-13: install disabled; Stevie enables them when he lifts the pause.**
- [ ] **Step 142** [B][C] ~3m: Merge, signed, no fast-forward.
  ```bash
  cd /home/govan/tmp/unheaded && git merge --no-ff -S sprint/s-zhen-front -m "Merge sprint/s-zhen-front: Zhen front closed before login, units, compliance from the Well" && git log --oneline -1
  ```
  **PHASE 9 EXIT GATE**: merge commit shows `G` in `git log --show-signature -1`.

---

## PHASE 10: HOST INSTALL ON WEST (attended) (Steps 143–156)

**Goal**: accounts, binaries, venv, project copy, socket directory, env file and units are installed; `dashboard-backend` reads the Well. Nothing Zhen is started.
**Prerequisite**: Phase 9; Stevie present.
**Time**: 75 min (the venv download dominates).
**Agent**: Coordinator.

- [ ] **Step 143** [S] ~2m: Accounts.
  ```bash
  sudo install -m 0644 /home/govan/tmp/unheaded/deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && sudo systemd-sysusers && id unheaded-zhen-app && id unheaded-llama
  ```
- [ ] **Step 144** [V] ~1m: uid 16758 and 16792; `unheaded-llama` in `render` and `video`.
- [ ] **Step 145** [S] ~5m: Dashboard reads the Well: recreate only `dashboard-backend` (compose needs sudo for the env file link).
  ```bash
  cd /home/govan/tmp/unheaded && sudo docker compose up -d --no-deps dashboard-backend && sudo docker compose ps dashboard-backend
  ```
- [ ] **Step 146** [D] ~5m: Compose refuses with `OPS_READER_PASSWORD` unset: the key is missing from `/etc/unheaded/compose.env`. Check by name only: `sudo grep -c '^OPS_READER_PASSWORD=' /etc/unheaded/compose.env` (prints a count, never the value).
- [ ] **Step 147** [V] ~3m: The page reads the Well.
  ```bash
  curl -sk https://127.0.0.1:20000/metrics 2>/dev/null | grep '^unheaded_compliance_source' | grep 'source="well"'
  ```
  - Each kind shows `1` for `source="well"`. `ComplianceWellFallback` is not in `amtool alert query` (or the Alertmanager page) 15 minutes later.
- [ ] **Step 148** [S] ~10m: llama-server installed outside home with a proper RUNPATH (cmake from the existing checkout; no new source).
  ```bash
  cd /home/govan/tmp/unheaded/llama.cpp && sudo cmake --install build --prefix /opt/unheaded/llama && ldd /opt/unheaded/llama/bin/llama-server | grep -E 'llama|ggml|not found'
  ```
- [ ] **Step 149** [V] ~1m: Every `libllama`/`libggml*` resolves under `/opt/unheaded/llama`, none under `/home`, no `not found`. If not → Step 150.
- [ ] **Step 150** [D] ~10m: RUNPATH still points at the build tree: reconfigure with `-DCMAKE_INSTALL_RPATH='$ORIGIN/../lib'` and reinstall; last resort `Environment=LD_LIBRARY_PATH=/opt/unheaded/llama/lib` in the unit (commit that change). If the unit later fails with a W^X error, `MemoryDenyWriteExecute` is already off; a different sandbox denial shows in `journalctl -u unheaded-llama`.
- [ ] **Step 151** [S] ~20m: The app: code copy from `develop`, the venv from the lock (hash-checked, binary only), the project copy (runbooks, skills), the embedder snapshot.
  ```bash
  sudo install -d -m 0755 /usr/local/lib/unheaded/zhen-app /usr/local/share/unheaded/zhen && cd /home/govan/tmp/unheaded && git archive develop raft | sudo tar -x -C /usr/local/lib/unheaded/zhen-app && git archive develop runbooks skills | sudo tar -x -C /usr/local/share/unheaded/zhen && sudo python3 -m venv /usr/local/lib/unheaded/zhen-app/venv && sudo /usr/local/lib/unheaded/zhen-app/venv/bin/pip install --require-hashes --no-deps --only-binary=:all: -r /usr/local/lib/unheaded/zhen-app/raft/runtime/requirements.txt 2>&1 | tail -2
  ```
- [ ] **Step 152** [V] ~3m: The venv imports what the app needs, as the app's account.
  ```bash
  sudo -u unheaded-zhen-app /usr/local/lib/unheaded/zhen-app/venv/bin/python3 -c "import flask, sentence_transformers, psycopg2, pygments, yaml; print('ok')"
  ```
- [ ] **Step 153** [S] ~5m: Embedder snapshot into the app's state directory (offline at run time), then the env file, unseen.
  ```bash
  sudo install -d -m 0700 -o unheaded-zhen-app -g unheaded-zhen-app /var/lib/unheaded-zhen-app /var/lib/unheaded-zhen-app/hf/hub && sudo cp -a "$(readlink -f /home/govan/.cache/huggingface/hub)/models--sentence-transformers--all-MiniLM-L6-v2" /var/lib/unheaded-zhen-app/hf/hub/ && sudo chown -R unheaded-zhen-app:unheaded-zhen-app /var/lib/unheaded-zhen-app/hf && DBHOST="$(sudo awk -F'@' '/^WELL_DSN=/{split($2,a,"/");print a[1];exit}' /etc/zhen-agentd.env)" && sudo bash /home/govan/tmp/unheaded/scripts/write-zhen-app-env.sh --db-host "$DBHOST"
  ```
  The `awk` prints only what follows `@` (host and port), never the password before it.
- [ ] **Step 154** [S] ~3m: Socket directory and units, then reload.
  ```bash
  sudo install -m 0644 /home/govan/tmp/unheaded/deploy/tmpfiles.d/unheaded-zhen-ui.conf /etc/tmpfiles.d/ && sudo systemd-tmpfiles --create /etc/tmpfiles.d/unheaded-zhen-ui.conf && sudo install -m 0644 /home/govan/tmp/unheaded/deploy/systemd/unheaded-llama.service /home/govan/tmp/unheaded/deploy/systemd/unheaded-zhen-app.service /home/govan/tmp/unheaded/deploy/systemd/zhen-agentd.service /etc/systemd/system/ && sudo systemctl daemon-reload && ls -ld /run/unheaded-zhen-ui
  ```
- [ ] **Step 155** [V] ~2m: `drwxr-s--- unheaded-zhen-app unheaded-zhen-ui-front`; `systemd-analyze verify` on the three installed units is clean; `systemctl show -p StartLimitIntervalUSec zhen-agentd` is `0`; both new units `inactive`; enabled only if Step 141 said so (`sudo systemctl enable unheaded-llama unheaded-zhen-app`).
- [ ] **Step 156** [W][C] ~3m: **PHASE 10 EXIT GATE**: progress file records Steps 143–155 (no values), commit by name on develop. Nothing Zhen runs.

---

## PHASE 11: PAUSE LIFTS: START, PROVE, OPEN (Stevie) (Steps 157–170)

**Goal**: Zhen runs from its units; the closed-before-login proof passes on loopback; then it opens on the LAN and passes again from the wifi.
**Prerequisite**: Phase 10; **Stevie lifts the pause**.
**Time**: 45 min.
**Agent**: Coordinator.

- [ ] **Step 157** [STEVIE] ~1m: Stevie's word to start Zhen. Without it, stop here: the plan is complete up to Phase 10.
- [ ] **Step 158** [S] ~3m: Stop any hand-started copies by unit name (never `pkill`), then start the units.
  ```bash
  systemctl --user stop unheaded-zhen-app unheaded-llama 2>/dev/null; sudo systemctl start unheaded-llama && timeout 120 bash -c 'until curl -sf http://127.0.0.1:8081/health >/dev/null; do sleep 3; done' && sudo systemctl start unheaded-zhen-app && sleep 20 && systemctl is-active unheaded-llama unheaded-zhen-app unheaded-zhen-ui-front zhen-agentd
  ```
- [ ] **Step 159** [V] ~2m: Four `active`; `curl -sk --curves X25519MLKEM768 https://127.0.0.1:20103/health` shows `status` and no `session_id`. If `degraded` → Step 160.
- [ ] **Step 160** [D] ~10m: `journalctl -u unheaded-zhen-app -n 50`: an embedder pin mismatch (`ZHEN_EMBEDDER_SHA256`), a Well TLS name mismatch (`ZHEN_DB_HOST` must match the certificate the agentd DSN uses), a socket permission (Step 155's mode). Fix the env file with the writer script, never by hand-printing it.
- [ ] **Step 161** [B] ~3m: The loopback proof.
  ```bash
  bash /home/govan/tmp/unheaded/scripts/zhen-lan-proof.sh https://127.0.0.1:20103
  ```
- [ ] **Step 162** [V] ~5m: `PASS n/n`. Log in as `dev` in a browser (logins file of 2026-11-12): sheets and categories are numbers, not `?`; one answer shows a token count above 0; an unknown `/view/source?id=nosuchtopic123` says not found.
- [ ] **Step 163** [B] ~3m: The grounding probe (session cookie from that login, saved 0600 by the person).
  ```bash
  ZHEN_SESSION_FILE=/home/govan/.local/state/unheaded/zhen-session bash /home/govan/tmp/unheaded/eval/grounding/run.sh https://127.0.0.1:20103
  ```
- [ ] **Step 164** [V] ~1m: `cert-ccna PASS` (no source), `cert-jncia PASS` (a `junos-` sheet), both with tokens > 0. A FAIL is recorded, not fixed here (vor content gap, Stevie's).
- [ ] **Step 165** [S] ~3m: Open on the LAN: the front's drop-in, and the app's allowed host (extend the host script `~/.local/state/unheaded/zhen_lan.sh` `up`/`down` to also write and remove `/etc/systemd/system/unheaded-zhen-app.service.d/lan.conf` with `Environment=ZHEN_ALLOWED_HOSTS=192.168.69.184` and restart the app).
  ```bash
  sudo bash /home/govan/.local/state/unheaded/zhen_lan.sh up
  ```
- [ ] **Step 166** [B] ~3m: The proof against the wireless address.
  ```bash
  bash /home/govan/tmp/unheaded/scripts/zhen-lan-proof.sh https://192.168.69.184:20103
  ```
- [ ] **Step 167** [V] ~2m: `PASS n/n`; and from the wireless side 8081, 20110, 20111, 20112 still refuse (`for p in 8081 20110 20111 20112; do timeout 3 bash -c "</dev/tcp/192.168.69.184/$p" && echo "OPEN $p"; done` prints nothing).
- [ ] **Step 168** [D] ~5m: Any route answers 200: `zhen_lan.sh down` at once, record the route, fix on develop with a test, repeat from Step 161.
- [ ] **Step 169** [STEVIE] ~2m: Stevie opens the page from his laptop once and logs in.
- [ ] **Step 170** [W][C] ~3m: **PHASE 11 EXIT GATE**: Steps 162, 164, 167, 169 recorded in the progress file; commit by name.

---

## PHASE 12: REBOOT SURVIVAL AND RECORDS (Steps 171–178)

**Goal**: after the next boot of west, Zhen comes back by itself; the records say what was built.
**Prerequisite**: Phase 11 (or Phase 10 when the pause has not lifted: then Steps 171–172 wait).
**Time**: 30 min plus the next boot.
**Agent**: Coordinator.

- [ ] **Step 171** [STEVIE] ~5m: At the next planned reboot of west (never a reboot for this plan alone).
- [ ] **Step 172** [V] ~5m: After it: `systemctl is-active unheaded-llama unheaded-zhen-app unheaded-zhen-ui-front zhen-agentd` all `active` (if enabled), `systemctl show -p NRestarts zhen-agentd` says how many retries it took (any number is fine; giving up is not), and Step 161 passes again.
- [ ] **Step 173** [W] ~10m: Wiki: `wiki/Systemd-Units.md` rows for `unheaded-llama.service` and `unheaded-zhen-app.service`, and the `zhen-agentd` row's start-up note (rebase over S-RESIDUE's vor row if it landed first); `wiki/Zhen-AI.md` (or the page that documents the Zhen page): open routes are `/`, `/health`, `/auth/*` and the stylesheet.
- [ ] **Step 174** [W] ~5m: `docs/adr/ADR-138-zhen-ui-hybrid-only-tls-front.md`: a dated section at the end (the record above it is not edited): the app has its own account and unit; the front binds LAN addresses with FREEBIND.
- [ ] **Step 175** [W] ~5m: `~/tmp/next.md`: what was built, what is open (session lifetime → ADR-114; vor exact-id → D-11), the off switches (`sudo systemctl disable --now unheaded-zhen-app unheaded-llama`; `zhen_lan.sh down`).
- [ ] **Step 176** [B] ~2m: Doc gates.
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-gitleaks.sh 2>&1 | tail -1
  ```
- [ ] **Step 177** [C] ~2m: Commit the docs by name, signed.
  ```bash
  cd /home/govan/tmp/unheaded && git add wiki/Systemd-Units.md docs/adr/ADR-138-zhen-ui-hybrid-only-tls-front.md docs/battle-plans/progress/S-ZHEN-FRONT-progress.md && git commit -S -m "[PLAN S-ZHEN-FRONT] Steps 171-177: records for the Zhen units and the closed front"
  ```
- [ ] **Step 178** [V] ~2m: **PHASE 12 EXIT GATE / PLAN DONE**: every phase gate ticked or `[BLOCKED]` with its reason; the worktree removed (`git worktree remove /home/govan/tmp/wt/s-zhen-front`) after the merge.

---

## Appendix A: Emergency procedures

**A1. The page is open to the LAN with a route that answers without login.**
1. `sudo bash /home/govan/.local/state/unheaded/zhen_lan.sh down`
2. `ss -ltnH 'sport = :20103'` shows only `127.0.0.1:20103`.
3. Reproduce with `scripts/zhen-lan-proof.sh https://127.0.0.1:20103`; add the route to the failing test first; fix; rerun Phase 11 from Step 161.

**A2. llama-server unit fails on the GPU.**
1. `journalctl -u unheaded-llama -n 80`. `invalid device function` → the two `-E` values are missing or wrong (`systemctl show -p Environment unheaded-llama`).
2. `Permission denied` on `/dev/kfd` → `id unheaded-llama` must list `render`; `DeviceAllow=/dev/kfd rw` present.
3. Fallback while debugging: Stevie's old `systemd-run --user` line (next.md "Off switches"); the unit stays stopped.

**A3. The app cannot reach the Well.**
1. `journalctl -u unheaded-zhen-app -n 50 | grep -i -E 'ssl|password|connect'` (no values are logged).
2. Rewrite the env file with `scripts/write-zhen-app-env.sh` after the password pass; restart the app.
3. The page still answers without the Well (`well_connected: false`); chat history is not saved meanwhile.

**A4. The dashboard falls back to files after Step 145.**
1. `unheaded_compliance_source{source="file-fallback"} == 1` and `ComplianceWellFallback` fires.
2. `runbooks/observe/compliance-well-fallback.yaml` is the runbook; the usual cause is a stale password after rotation (`sudo docker compose up -d --no-deps dashboard-backend` rereads it).
3. Rollback: revert the two compose lines on develop, recreate `dashboard-backend`; the page goes back to files.

**A5. Rollback of the whole host install.**
`sudo systemctl disable --now unheaded-zhen-app unheaded-llama`; `sudo rm /etc/systemd/system/unheaded-zhen-app.service /etc/systemd/system/unheaded-llama.service /etc/tmpfiles.d/unheaded-zhen-ui.conf`; `sudo systemctl daemon-reload`; the hand-start lines in next.md work as before. Accounts stay (append-only registry).

## Appendix B: Agent assignment matrix

| Phase | Agent | Parallel | Depends on | Time |
|---|---|---|---|---|
| 0 Preconditions | Coordinator | no | — | 15 m |
| 1 Closed before login | Agent | [P] | 0 | 75 m |
| 2 Token counts, grounding probe | Agent | [P] | 0 | 60 m |
| 3 Page before/after login | Agent | [P] | 0 | 45 m |
| 4 start-zhen.sh | Agent | [P] | 0 | 25 m |
| 5 Source viewer | Agent | [P] | 0 | 40 m |
| 6 Start-up code | Agent | no | 0 | 75 m |
| 7 Accounts and units | Agent | no | 6 | 90 m |
| 8 Compliance from the Well | Agent | [P] | 0 | 20 m |
| 9 Merge | Coordinator | no | 1–8 | 30 m |
| 10 Host install | Coordinator, [S] | no | 9, Stevie present | 75 m |
| 11 Start, prove, open | Coordinator, [STEVIE] | no | 10, pause lifted | 45 m |
| 12 Reboot, records | Coordinator | no | 11, next reboot | 30 m |

Phases 1, 3 and 5 all edit `raft/zhen_app.py` or `raft/tests/test_zhen_app_gate.py`: run them in one agent, one after another, or rebase between them.

**Critical path**: 0 → 6 → 7 → 9 → 10 → 11 → 12 (about 6 h of work, plus Stevie's two words and one reboot).

## Appendix C: Quick reference

| Thing | Value |
|---|---|
| Front | `unheaded-zhen-ui-front.service`, TLS 1.3 hybrid only, `127.0.0.1:20103`, metrics `127.0.0.1:20112` (ADR-138) |
| App | `unheaded-zhen-app.service`, uid 16758, socket `/run/unheaded-zhen-ui/app.sock` (dir 2750 app:front) |
| llama-server | `unheaded-llama.service`, uid 16792, `127.0.0.1:8081`, `/opt/unheaded/llama`, model `/var/zhen/models/qwen2.5-coder-7b-instruct-q4_k_m.gguf` |
| Daemon | `zhen-agentd.service`, `127.0.0.1:20111`, scheduler paused (`/schedule resume` is Stevie's) |
| Open routes | `/`, `/health`, `/auth/login`, `/auth/logout`, `/auth/me`, `/api/v1/highlight.css`, static files |
| Golden route list | `raft/tests/zhen_routes.golden`; `ZHEN_UPDATE_GOLDEN=1` rewrites it |
| Live proof | `scripts/zhen-lan-proof.sh BASE_URL` (curl `--curves X25519MLKEM768`, OpenSSL 3.5.3 on west) |
| Well readers | `ops_reader` (unheaded_ops), `config_reader` (unheaded_config); passwords `OPS_READER_PASSWORD`, `CONFIG_READER_PASSWORD` in `/etc/unheaded/compose.env` |
| Compliance source metric | `unheaded_compliance_source{kind,source}` on dashboard `/metrics` |
| LAN switch | `sudo bash ~/.local/state/unheaded/zhen_lan.sh up|down` (host script, west only) |
| Raft tests | `/var/tmp/unheaded-sprint/raftvenv/bin/python -m pytest tests -q` in `raft/` |

---

*S-ZHEN-FRONT Battle Plan: Forged 2026-11-12*
*13 Phases. 178 Steps. Nothing answers a stranger but the door and the sign on it.*
*The page comes back after a reboot, as itself, under its own name.*
