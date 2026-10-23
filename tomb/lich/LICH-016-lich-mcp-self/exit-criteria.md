<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-016 exit criteria (ADR-062)

## Coverage threshold

- The self-fuzz exercises every tool (`lich_list_campaigns`, `lich_show_campaign`,
  `lich_run_harness`, `lich_read_crash`, `lich_triage`, `lich_file_finding`) and
  the gate + scope + cap + confirmation paths.

## Crash / divergence budget

- 24 h continuous (`go test -fuzz` on west) with **zero net-new** panics, bypasses
  or out-of-tree writes.

## Severity gate

- **Any bypass is CRITICAL** and keeps the Lich MCP server **dry-run-only** until
  fixed (scope bypass, gate bypass, confirmation-token forgery, cap bypass,
  kill-switch defeat, auto-publish, sandbox escape).
- A hardening gap that is not a bypass (e.g. the oversized-field register bloat)
  is LOW and fixed in place; it does not hold the server dry-run-only.

## Re-run trigger

- Any change to `cmd/lich-mcp` or `pkg/lichtools` re-arms this campaign (a new or
  changed tool, gate, scope rule, cap, or confirmation path).

## Close condition

- All six safety invariants green, the native fuzz clean for the budget, and every
  staged self-finding closed on passing evidence. A CRITICAL self-finding blocks
  non-dry-run live use until closed.
