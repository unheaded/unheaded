<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Lich MCP Server (`cmd/lich-mcp`)

An MCP surface that lets a local model and Claude *drive* the existing Lich
red-team framework (`tomb/lich/`) — list/show campaigns, run a harness, read
crashes, triage, file a finding — **confined to this operator's own sandboxes
and local harnesses, with scope enforced server-side, every call through the
Champion gate, and findings never auto-published.**

> Authorization: Stevie's own systems only (the solo GPL-3.0 Kingdom on
> WEST/EAST). No third-party targets, ever. Scope is enforced in the server, so
> a jailbroken driver cannot widen it. See
> `docs/battle-plans/S-LICH-AWAKENS-battle-plan.md`, ADR-062/063/098/104.

## Transport

**stdio only.** The server binds no TCP port; its only input is a
newline-delimited JSON-RPC 2.0 stream on stdin. Start/stop is part of the kill
switch — not running it is the strongest off switch.

```
go run ./cmd/lich-mcp -config configs/lich-mcp.yaml -project-root "$PWD"
```

## The six tools

| Tool | Kind | Gate classification |
|------|------|---------------------|
| `lich_list_campaigns` | read | read-only → passes the gate |
| `lich_show_campaign` | read | read-only → passes the gate |
| `lich_run_harness` | run (confined) | executes a fuzzer, but ONLY inside a confined no-egress sandbox with a server-clamped duration; its control is scope + confinement, not per-call confirmation |
| `lich_read_crash` | read | read-only; never executes the crashing input |
| `lich_triage` | read | read-only analysis (crash-triage.sh), dedup key |
| `lich_file_finding` | mutate | writes the ADR-098 register → always pending-confirmation (Champion Rule 2); **never auto-publishes** |

## The scope model (server-side, never by prompt)

A valid target is ONLY:

1. an LXD container whose name starts with `scope.sandbox_prefix`
   (`lich-sbx-`) — ephemeral containers the server creates; or
2. a three-digit harness under `scope.harness_dir`
   (`tomb/lich/harnesses/`), addressed by id (e.g. `001`), whose file must
   exist and whose symlink-resolved real path stays inside that dir.

Nothing else is a valid target: no network target, no host path outside the
project root or the sandbox. The allowlist lives in `configs/lich-mcp.yaml`,
is read once at startup, and a tool argument a model controls can never widen
it. The `Target`'s `Kind` is chosen by the tool, not by the model. Inputs are
rejected for non-ASCII/homoglyph, embedded newline/NUL, absolute paths, `..`,
and symlink escapes.

## Confinement, resource limits & kill switch

> **Phase 2 fills the sections below.** In Phase 1 `lich_run_harness` passes
> scope + gate and then **fails closed** (no unconfined execution) because the
> confinement substrate is not wired yet.

- **No-egress**: every target runs inside a network namespace with no route off
  the box. Proven unprivileged in preflight H8 (`unshare -rn`: loopback up,
  egress route+connect blocked, no default route). No-egress is a hard gate: if
  it cannot be proven, fuzzing does not run.
- **Resource envelope**: per-sandbox CPU / memory / pids / wall-timeout from
  `configs/lich-mcp.yaml`; a memory fit check runs before every spawn (swap is
  never counted); a host-wide cap bounds live sandboxes.
- **Kill switch**: touch `kill_switch_file` (`/run/lich-mcp.halt`) or send
  SIGTERM/SIGINT → stop accepting calls, reap every live sandbox, refuse new
  spawns. Mirrors ADR-063's Marshal HARD-HALT.
- **Sandbox escape red-team**: results recorded here after Phase 2 Step 67.

## Audit

Every call is audited (tool, decision + reason, pending flag, input size +
SHA-256 — never the input bytes, output size, duration) via the ADR-104
`Auditor` shape, as JSON lines on stderr.
