<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-LICH-AWAKENS — Execution Progress (Sprint 4, Phases 0–3)

**Plan**: `docs/battle-plans/S-LICH-AWAKENS-battle-plan.md`
**Execution base**: worktree base `f1d1f412`; branch `sprint/lich-awakens` (force-updated to
HEAD after every commit). Commits `--no-gpg-sign`. **Never push. Never touch
develop/staging/main.** Phases 4+ are NOT started (they wire into Sprint 2's
`sprint/two-brains` confirmation flow).

**Confinement doctrine (SAFETY, non-negotiable)**: targets are Stevie's own code + local
sandboxes only; scope enforced server-side (allowlist + no-egress + resource limits), never
by prompt. Host changes (LXD containers, nftables, persistent state) are `[ESCALATE]`; this
build uses the unprivileged `unshare` user+net-namespace fallback by default and proves
no-egress with a test that must fail to reach the network.

---

## Phase 0 — Preflight & Prerequisite Verification — COMPLETE (2026-10-23)

- Baseline: `go build ./...` exit 0; `go test ./...` = 207 ok / 0 fail / 19 no-test-files.
- H1 TRUE (Sprint-2 legs all present), H2 TRUE (gate), H3 NO_KEY (driver `-dry-run`),
  H4 TRUE (LXD 5.21.8), H5 TRUE (Lich 001 clean, 0 crashes), H6 TRUE (monad-mbc 67 tests +
  fuzz targets), H7 TRUE (register: 14 open — 2h/4m/8l), **H8 PASS (no-egress proven
  unprivileged via `unshare -rn`)**.
- No E0, no E1, no ESCALATE.
- Topology: separate `cmd/lich-mcp` (stdio only) + `pkg/lichtools` (`zhentools.Provider`).
- Scope lock: targets = LXD name prefix `lich-sbx-` OR harness under `tomb/lich/harnesses/`.
- Findings block + KNOWN FAILURES BASELINE recorded in the plan file.

## Phase 1 — Lich MCP server skeleton + server-side scope — (in progress)

## Phase 2 — Sandbox substrate: confinement + kill switch — (pending)

## Phase 3 — Lich tools: real behaviour through the gate — (pending)
