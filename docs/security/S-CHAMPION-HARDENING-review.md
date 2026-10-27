<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-CHAMPION-HARDENING security and compliance review

Phase 14, 2026-10-27, after Phases 1-8 and 10-13 merged on develop (Phase 9 partly: the vor unit waits on a port
decision, E4). Each row: what changed, then the four questions the plan asks.

## Attack-surface delta

| Change | Ledger | New listener | New privilege | New dependency | Fuzz target |
|---|---|---|---|---|---|
| Trust-label allowlist (exact labels, vor chain verifier) | RT-06 closed | no | no | no | `FuzzTrustLabel` (new) |
| Confirm tokens bound to the requesting principal | RT-03 closed | no | no; a `-no-auth` agentd can no longer redeem | no | `FuzzPendingCallFor` (extended) |
| Pending-token gc ticker, store creation locked | RT-04 closed | no | no | no | no |
| Champion write-size cap | RT-05 closed | no | no | no | no |
| Sandbox writes through `openat2(RESOLVE_BENEATH)` | ADR T-05 closed | no | no | `golang.org/x/sys` (already in the build) | no |
| Dispatch truth: unimplemented tools refused before the gate; `system_command`, `delete_file`, `kanban_delete` removed | RT-09 closed | no | fewer tools | no | no |
| Audit-write failure fails closed for mutations (`champion_audit_write_failures_total`) | RT-10 closed | no | no | no | no |
| One prompt + one destructive-verb table (`pkg/zhenrag`), generated Python copy | RT-07/08 closed | no | no | no | no |
| Runbook manifest: live `runbook_execute` needs the sha256 in `runbooks/MANIFEST.sha256`; unapproved runbooks hitting the verb pattern refused; private verified copy executed; symlinks out of `runbooks/` refused | RT-08 | no | no | no | no |
| GGUF + embedder sha256 pins (`switch-model.sh` exit 4; web UI refuses a mismatched embedder) | RT-15 closed | no | no | no | no |
| Routers behind one interface; family caps leave headroom; `max_tokens` fitted to the cap left | F-23/F-24 | no | no | no | no |
| Scheduler in zhen-agentd: `/api/v1/scheduler/{add,remove,pause,list}`, new mutating tool `schedule_update`; only approved or status-less runbooks scheduled | M-130 | loopback API (existing listener); `-metrics-addr` serves `/metrics` alone on the WireGuard address | no (the unit forbids sudo) | no (Python `schedule` pin removed) | no |
| Memory endpoints `/api/v1/memory/{recall,remember}`, new mutating tool `memory_write` (authenticated caller only; Rule 3 verbs) | M-129 | existing loopback listener | writes `zhen_memories` as app_zhen | no | no |
| Model selector: `/api/v1/models`; swap rollback in its own process group | ADR-060 tail | existing listener | no | no | no (LICH-013 campaign file written, not run) |
| MCP server in Go (`cmd/zhen-mcp`, same nine tools, stdio only, every call through the Champion) | M-132 closed | no (stdio) | no; `ZHEN_MCP_ALLOW_*` enabled calls carry a `direct-user` reference (open decision below) | Python `mcp` package dropped | no |
| zhen-cli `/recall /remember /confirm /model`; key file must be 0600 | M-129 closed | no | no | no | no |
| Kanban `GET /api/v1/runbooks` (public read like the board), `POST /api/v1/tasks/from-runbook` (write through the board store; 401 anonymous with auth on, tested) | M-128 | existing listener | `app_kanban` SELECT on chosen `zhen_agent_runs` columns (migration 041) | no | no |
| `run-runbook.py` verification sections executed; `runbooks.progress` on Wotan (opt-in) | M-127 | no | no | no | no |
| `_strip_tool_call_json` now strips payloads at any nesting depth, with braces in strings, and truncated | RT-14/16 | no | no | no | no (9 unit tests) |
| Overnight unit + timer (not enabled), kill-switch runbook, morning-report script | M-126 (E2 open) | no | no | no | no |
| vor unit | M-131 | not built (ports, E4) | — | — | — |

## Gates (Step 239), exit codes read directly

check-gitleaks 0, check-gosec-ratchet 0, check-dependency-baseline 0, check-systemd-units 0, check-unit-binaries 0,
check-runbooks 0, check-prometheus-rules 0, check-manifest-yaml 0, check-python-syntax 0, check-js-lint 0.
Fuzz (Step 238): `FuzzTrustLabel` 45 s and `FuzzPendingCallFor` 45 s, no crasher (the plan's `FuzzCanonicalCall`
does not exist). go.mod since `a7fe3acd`: one addition, `modernc.org/sqlite v1.61.0` (Stevie's requested bump).
SPDX (Step 241): every new Go file carries it (the repo's SPDX gate covers Go); new runbooks and YAML data files
follow the corpus convention (1 of 91 runbooks has a header). Full suite (Step 243): 232 packages ok, 0 FAIL.

## Open security decisions

- **MCP write/exec flags vs. Step 86.** With `ZHEN_MCP_ALLOW_WRITE/EXEC`, enabled write and exec calls carry a
  `direct-user` reference naming the flag; without it the gate refuses every write and the flags do nothing. This
  conflicts with "a model loop never asserts direct-user". Alternative: pending confirmation through a
  lich-mcp-style confirm flow. Default today: the flags are off in `.mcp.json`.
- **Branch-sandbox writes are live with no off switch** (ADR-019 Level 2 amendment, E3).
- **One API key for the Zhen UI** means all UI users share one confirm principal; per-owner `uhk_` keys
  (S-IRON-GATE Phase 3) can now separate them.
