<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-ZHEN-FRONT progress

Plan: `docs/battle-plans/S-ZHEN-FRONT-battle-plan.md` (13 phases, 178 steps).

## Decisions

Answered by Stevie 2026-11-12 (round 2, and Step 141).

| # | Question | Answer |
|---|---|---|
| A-16 | The open highlighter and `session_id` on `/health` | Close `POST /api/v1/highlight` behind the login, drop `session_id` from `/health`, prove it, then open on the LAN |
| A-36 | Units for llama-server and the app | System units, each under its own nologin account |
| A-27 | Where the compliance page reads from | (a) the Well |
| Step 141 | Boot behaviour of the two new units | Installed disabled until Stevie says |

## Baseline (2026-11-15, worktree `sprint/s-zhen-front` off develop `7b8c09bf`)

- Raft suite: 585 passed, 2 skipped.
- Go (`cmd/zhen-agentd`, `cmd/zhen-ui-front`, `pkg/uids`, `cmd/dashboard-backend`, race on): all ok.
- Pause state: `zhen-agentd`, `unheaded-zhen-ui-front` and `unheaded-vor` are active (system units). No user unit `unheaded-llama` or `unheaded-zhen-app`. Listening: `127.0.0.1:20103`, `127.0.0.1:20111`, `[fd00:dead:beef::1]:20111`. Nothing on 8081: llama-server is down, the pause stands.

## Step log

| Date | Steps | Note |
|---|---|---|
| 2026-11-15 | 1-10 | Phase 0. The coordinator made the worktree (Steps 1-3). Baselines and pause state above. |
| 2026-11-15 | 11-27 | Phase 1 (`99417e5d`). Red first: six new tests failed for the new reasons only. `POST /api/v1/highlight` left `_LOGIN_OPEN` and `_SCRIPTED_ROUTES` and joined `_READ_POSTS`; `/health` lost `session_id` and answers `startup_failed` in place of the error text, which is now on `/api/v1/system/state` (`webui.startup_error`). `raft/tests/zhen_routes.golden` lists 40 routes (the night session counted 45; each of the 40 is a real route). The two login-route CSRF tests ask on loopback, because a password is taken over plain http only there (Step 18). Raft suite 591 passed. |
| 2026-11-15 | 28-34 | Phase 1 (`642e1d84`). `scripts/zhen-lan-proof.sh` and its plan test (120 requests over 40 routes). shellcheck clean. `tests/scripts/test_zhen_lan_proof.py` runs the shell test, so the CI job that runs `tests/scripts` runs it (not in the plan). **Phase 1 gate: pass.** |
| 2026-11-15 | 35-44 | Phase 2 (`eee789ed`). Red first (four tests). `askResponse` and the stream's `done` event carry `tokens_input` and `tokens_output`, summed per ask in the request context; `openapi.json` lists both. The page reads `tokens_output`. `go test -race ./cmd/zhen-agentd/...` ok; raft suite 596 passed. |
| 2026-11-15 | 45-52 | Phase 2 (`c0a16c82`). `eval/grounding/` (two prompts, README, `run.sh`). The session is a 0600 curl cookie jar; the script refuses any other mode. `cmd/zhen-eval` tests untouched and ok. **Phase 2 gate: pass.** The live run is Step 160. |
| 2026-11-15 | 53-64 | Phase 3 (`4fa356a3`). `refreshStats()` is the one place the sidebar and the welcome get stats; before login they say "log in to see". A 401 on the chat send puts the question back in the box. A login after the session ended keeps the page; the first login of a page load still reloads it (the page was drawn for a stranger). The model switch keeps its own stats poll. oxlint passes (it reads `.js` files only; the page's inline script was parsed with `node --check`). Raft suite 598 passed. **Phase 3 gate: pass.** The browser check is Step 162. |
| 2026-11-15 | 65-74 | Phase 4 (`dec17d98`, `f0cd1445`). `-gated` runs `bin/release/zhen-agentd` and exits 1 naming `make binaries` when it is missing. PID files in `$XDG_RUNTIME_DIR/unheaded-zhen/`; the stop line names them, or `systemctl stop` for a unit. The script says so and leaves alone `unheaded-llama.service` and `unheaded-zhen-app.service` when they are active. A PID file is trusted only when that PID is still the named program. Step 72 found two notes in `zhen_app.py` and the stop section of `raft/CLAUDE.md`; fixed. shellcheck: no warning (two `SC1091` infos predate this). **Phase 4 gate: pass.** The script was not run: it needs llama-server, and the pause stands. |
| 2026-11-15 | 75-86 | Phase 5 (`506d7109`). Red first (seven tests). `/api/v1/source` answers 404 `no such topic` unless vor's answer carries the asked `name` (live vor checked read-only: `nosuchtopic123` answers `WOTAN_TOPICS`). `zhen_markdown.render(..., highlight=False)` for `external` sheets; the render cache key carries the flag, with a test that a canonical rendering is not served to an external sheet of the same text. Raft suite 606 passed. **Phase 5 gate: pass.** |

## Parked

- **The 15-minute login.** The lifetime is the issuer's (`cmd/unheaded-auth/issuer.go:34`, `tokenLifetime = 15 * time.Minute`) and every page's, not Zhen's: `pkg/bff` sessions end with the token too. The fix is ADR-114 N5 (Well-backed sessions, idle 30 minutes, absolute 12 hours) and the SSO ADR (backlog row D-08). This plan only makes the expiry kinder: the question stays in the box.
- **vor's own answer for an unknown id** (the nearest sheet) and its substring matching stay with backlog row D-11 (S-WIKI-VOR). The app no longer shows the wrong sheet, whatever vor answers.
- **An attached file is not kept across an expired session.** The page drops the attachment when it sends; only the typed question comes back.

## For Stevie

Nothing yet.

## Stuck

Nothing yet.
