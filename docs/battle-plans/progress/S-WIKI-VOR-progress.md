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
| 1-10 | Phase 0 gate PASS: both trees clean, Go 1.27.3, baselines green, "before" numbers above | (this commit) |

## Parked

Nothing yet.

## Stuck

Nothing yet.

## For Stevie

Phase 8 (install on west) is attended and not started.
