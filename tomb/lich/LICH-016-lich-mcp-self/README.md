<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-016 — the Lich MCP server's own surface (the adversary audits the adversary)

**Opened:** 2026-10-23 (S-LICH-AWAKENS Phase 9). **ADRs:** 062 (framework),
104 (gate/cap), 063 (HARD-HALT discipline). **Activation:** ADR-062 rule #2 —
a new stdio endpoint with mutation power and a new auth (confirmation) surface
landed, so a campaign MUST open against it.

## Target

The Lich MCP server (`cmd/lich-mcp`) and its tool provider (`pkg/lichtools`):
the thing this whole sprint built. BlackMage turns on the Lich itself.

## Attack surface enumerated

scope bypass · gate bypass · confirmation-token forgery/replay · dollar-cap
bypass · compute-budget bypass · kill-switch defeat · audit evasion ·
seed-as-code · finding forgery · sandbox escape · path traversal · oversized /
malformed / unicode / nested tool input.

## Harness

The self-fuzz lives in `pkg/lichtools/self_redteam_test.go` (it needs the
package-internal provider harness):

```
go test ./pkg/lichtools/ -run LICH016            # table self-fuzz
go test ./pkg/lichtools/ -run Safety_            # the four safety invariants
go test ./pkg/lichtools/ -fuzz FuzzLichToolInputs -fuzztime 60s   # native fuzz
```

It drives every tool with malformed / oversized / unicode / nested /
path-escape / newline-injection inputs and asserts: **no panic**, **no write
escapes the provider's configured trees** (register/seeds/results/harness), and
**no oversized field reaches the register** (the field bound below).

## Safety invariants proven (Phase 9)

| Invariant | Where proven |
|---|---|
| Scope refuses off-allowlist before the gate | `provider_test.go`, `scope_test.go`, LICH-016 self-fuzz (path traversal) |
| Mutating `file_finding` never runs before confirmation, never replays, and a different call cannot inherit an approval | `cmd/lich-mcp/confirm_test.go`, `self_redteam_test.go` `TestSafety_ConfirmTokenForgery` |
| Kill switch halts new spawns and reaps live sandboxes, idempotently | `killswitch_test.go` |
| Dollar cap holds regardless of route/weight; reserve-before-send, fail-closed | `cmd/lich-mcp/routing_boundary_test.go`, `pkg/claudebridge` Cap/Spend tests |
| No auto-publish: findings land only in the gated register (no Wotan/HTTP/git path) | `self_redteam_test.go` `TestSafety_NoAutoPublishPath` + source grep (below) |
| Zero user data access: audit logs only sizes + SHA-256, prompt text filtered | Phase 6 audit tests; `drive.go` audit records no prompt/response text |

Source grep (no finding-to-public-surface path):

```
grep -rnE 'Publish|http\.Post|git push' pkg/lichtools/*.go | grep -v _test.go
# -> only config.go's `AutoPublish bool` field (defaults false). No code path.
```

## Findings this session

- **FND candidate (hardening, LOW, now fixed): `lich_file_finding` did not bound
  free-text field sizes.** The self-fuzz staged a 64 KiB `summary` verbatim into
  the register. It is post-confirmation (a human reviews the ticket), so it is
  **not** a gate/scope bypass and not an E2 — but the register is a human-reviewed
  triage file, not a blob store. Fixed in this sprint: `clampField` caps summary/
  remediation/plan at 2000 runes and class/host/ids at 64–128. Recorded as a
  proposed finding in the progress file for the lead; **not** auto-filed.
- No scope / gate / cap / kill-switch bypass. No panic. No side effect outside
  the sandbox/register. No auto-publish path.

See `exit-criteria.md`.
