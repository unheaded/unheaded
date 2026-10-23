<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-016 status

| Field | Value |
|---|---|
| State | ACTIVE (opened 2026-10-23, S-LICH-AWAKENS Phase 9) |
| Harness | `pkg/lichtools/self_redteam_test.go` (table + `FuzzLichToolInputs`) |
| Bypasses found | 0 (scope / gate / cap / kill-switch / auto-publish all held) |
| Panics | 0 |
| Side effects outside sandbox/register | 0 |
| Hardening fixes | 1 — `lich_file_finding` oversized-field bound (`clampField`); LOW, fixed in place |
| Server posture | not held dry-run-only (no CRITICAL self-finding) |
| Re-run trigger | any change to cmd/lich-mcp or pkg/lichtools |

## Log

- 2026-10-23: campaign opened; self-fuzz table + native fuzz entry added; four
  safety-invariant tests green; the one gap found (unbounded register field) was
  a hardening issue, not a bypass — fixed in this sprint and recorded as a
  proposed finding for the lead. Zero-user-data-access confirmed (audit logs only
  sizes + SHA-256).
