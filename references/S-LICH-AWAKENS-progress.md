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

## Phase 1 — Lich MCP server skeleton + server-side scope — COMPLETE (2026-10-23)

- `pkg/lichtools`: `config.go` (yaml.v3, no new dep; fails loud on any credential
  field), `scope.go` (server-side allowlist; sandbox-prefix or three-digit harness
  only; Kind is server-chosen, never a model arg), `provider.go` (6 tools, typed
  inputs `additionalProperties:false`, scope-validated before work).
- `cmd/lich-mcp/main.go`: MCP stdio server (no TCP port) → scope pre-check → Champion
  gate (`claudebridge.ChampionGate`) → provider, each call audited (ADR-104 shape).
- `pkg/champion/toolcall.go`: 6 lich tools classified — list/show/read/triage/run_harness
  read-only-to-gate (run_harness confined by scope+sandbox), file_finding mutating
  (always pending-confirmation).
- Gate seam proven: off-allowlist refused BEFORE gate; file_finding pending-confirmation;
  read-only passes; in-scope run_harness passes gate then FAILS CLOSED (no substrate yet).
- Hardened vs symlink-escape, homoglyph, embedded newline/NUL/tab, absolute path, `..`,
  over-long. No new dep (go.mod/go.sum unchanged since base). Tests + -race GREEN; vet/fmt clean.
- Commits: scope `2c4cf6b5`, skeleton `3fc8f3fb`, test `eae55da7`, hardening `b3563f88`.

## Phase 2 — Sandbox substrate: confinement + kill switch — COMPLETE (2026-10-23)

- Substrate decision (Step 49): unprivileged `unshare` netns sandbox is the DEFAULT
  (LXD container creation is an [ESCALATE] per SAFETY brief, not used).
- `sandbox.go` `netnsSandbox`: `unshare -r -n -m --pid --mount-proc --fork` + a mount
  prelude (read-only root + private tmpfs `/tmp` + rw binds only for configured writable
  paths) + `prlimit` (nproc/as/cpu) + wall-timeout with process-group SIGKILL + bounded
  output. `Spawn` memory-fit preflight (swap never counted) → `ErrWontFit`. Fails closed
  (`ErrNoConfinement`) if unshare/user-ns absent — never runs unconfined.
- `killswitch.go`: halt-file + SIGTERM/SIGINT → stop spawns, SIGTERM live sandboxes,
  grace, SIGKILL+Destroy, HALTED audit; idempotent.
- `resource.go`: `Semaphore` host-resource cap → `ErrAtCapacity` (no fork-bomb via run
  requests), race-tested.
- `run_harness` wired Spawn→Exec→Destroy through scope+gate+semaphore+killswitch; bare-metal
  test proves MCP-tool → confined-sandbox path.
- **Escape red-team (Step 67-68): 7 PASS / 0 FAIL** — DNS/TCP/ICMP egress blocked,
  host-FS and /etc writes blocked, PID namespace isolated, /proc/1 is ns init. No escape →
  **no E1**. Recorded in `tomb/lich/LICH-MCP-README.md`.
- Cleanup verified: no stray containers, no named netns, no stray sandbox processes.
- Commits: sandbox `b1fa760f`, killswitch `7addd661`, semaphore `e7de6656`, red-team
  `ba5b3ccb`, run_harness-confined `cdd19d9b`.

## Phase 3 — Lich tools: real behaviour through the gate — COMPLETE (2026-10-23)

- All six tools filled with real behaviour (`pkg/lichtools/tools_real.go`):
  list/show read campaigns from tomb/lich (harnesses + LICH-* dirs, cite ADR-062);
  run_harness runs the REAL lich-runner.sh confined + clamped + budgeted (offline go
  build: GOCACHE→tmpfs, module cache RO, GOPROXY=off; RLIMIT_AS skipped for the go
  toolchain, bounded by fit+wall+pids+cpu+semaphore); read_crash returns hex+size+sha256
  and never executes; triage wraps crash-triage.sh in-sandbox → severity + dedup key;
  file_finding writes a strict-schema-valid ADR-098 entry (dedup/repro in verify text),
  dedups (update-not-duplicate), needs-regen marker, NEVER auto-publishes, YAML-injection-safe.
- Bare-metal: run_harness compiled + fuzzed harness 001 confined (exit 0, no egress).
  (The fuzzer surfaced a crash in the monad-wire harness — pre-existing harness behaviour,
  recorded as baseline, NOT filed: findings are never auto-published.)
- Per-call audit (ADR-104 shape) at the server; daily spend-cap hook (`claudebridge.Ledger`)
  present for the Claude driver (later phase; no model calls here).
- BlackMage tool-surface review: read_crash path-escape, triage out-of-tree dir, file_finding
  YAML injection + out-of-register writes — all refused/contained (tests).
- Register still strict-parses after a filed finding (mirrors cmd/compliance-evidence's
  strict decode); file_finding is exercised only against temp registers — the REAL
  compliance register is never written during this sprint (no real crash to file).
- Commits: real tools `b493b822`, server hook + hardening `82314b58`.

### Phase 3 EXIT GATE — PASS

All six tools real, confined, audited, gated; file_finding writes a strict-valid register
entry only via the (post-confirmation) provider path and never auto-publishes; spend hook
present. `go build ./...` clean; `go test ./...` = 280 ok / 0 fail (zero NEW failures vs the
Phase-0 baseline); golangci-lint (incl. gosec) 0 issues on the new packages; `go test -race`
clean; no new go.mod/go.sum deps; the real ADR-098 register is untouched (still 14 open).

## Sprint 4 chunk (Phases 0-3) — COMPLETE

- Branch `sprint/lich-awakens` = HEAD `82314b58`, based on `f1d1f412`; develop/staging/main
  never touched; nothing pushed. 14 commits (`git commit --no-gpg-sign`).
- **Phases 4+ NOT started** (they wire into Sprint 2's confirmation flow on
  `sprint/two-brains`): Phase 4 (gate/confirm/route wiring), 5/6 (local + Claude drivers),
  7 (differential fuzz), 8 (lifecycle), 9 (meta red-team), 10 (CTF design-only), 11 (docs).
- **`.mcp.json` deliberately NOT modified**: the Lich MCP server is stdio-only and opt-in
  (launched manually), kept off the always-on Zhen MCP surface for blast-radius isolation;
  not auto-registering it is itself part of the kill switch.
- **Pending (Sprint 2, not a blocker for P0-P3)**: the out-of-band confirmation *redemption*
  round trip for lich_file_finding (champion.ConfirmPendingToolCall routing for lich tools)
  belongs to Phase 4 / `sprint/two-brains`. The gate's *refusal* (pending-confirmation) is
  built and tested here.
- No STUCK, no ESCALATE: E0 did not fire (H1 TRUE), E1 did not fire (no-egress proven).
