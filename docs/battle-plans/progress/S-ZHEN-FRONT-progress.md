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

## Parked

Nothing yet.

## For Stevie

Nothing yet.

## Stuck

Nothing yet.
