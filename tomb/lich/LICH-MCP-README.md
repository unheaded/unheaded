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

The substrate is the **unprivileged `unshare` sandbox** (`pkg/lichtools.netnsSandbox`).
Creating an LXD container / nftables rule / named netns is an **[ESCALATE]** per the
sprint's SAFETY brief, so it is NOT used; the unprivileged user+net+pid+mount namespace
is the default and needs no root. If `unshare`/user-namespaces are unavailable the
sandbox returns `ErrNoConfinement` and `run_harness` **fails closed** — it never runs
unconfined.

Each confined command runs as:

```
unshare -r -n -m --pid --mount-proc --fork -- sh -c '<prelude>; exec "$@"' sh \
    prlimit --nproc=N --as=BYTES --cpu=SECS -- <argv>
```

- **No-egress** (hard gate): `-n` gives a network namespace with no route off the box.
  Proven unprivileged in preflight H8 and again in the escape red-team below. If it
  cannot be proven, fuzzing does not run.
- **No host filesystem**: the mount namespace remounts `/` **read-only**, gives a private
  tmpfs `/tmp`, and rebinds only the configured writable paths (the results tree) rw. A
  write anywhere else fails.
- **Resource envelope**: per-sandbox CPU-seconds / address-space / pids via `prlimit`,
  plus a wall-clock timeout that SIGKILLs the whole process group. A memory fit check runs
  before every spawn (free RAM ≥ `mem_bytes × mem_fit_safety`; **swap is never counted**).
  A host-wide semaphore (`max_live_sandboxes`) bounds concurrency so a driver cannot
  fork-bomb the host with run requests (`ErrAtCapacity`).
- **Kill switch**: touch `kill_switch_file` (`/run/lich-mcp.halt`) or send SIGTERM/SIGINT →
  stop accepting calls, SIGTERM every live sandbox, and after a grace window SIGKILL +
  destroy them, writing a `HALTED` audit line. Mirrors ADR-063's Marshal HARD-HALT.

### Sandbox escape red-team (2026-10-23, west, unprivileged netns substrate)

Every escape attempt was run *inside* the production-equivalent confinement and must
fail. Result: **7 PASS / 0 FAIL** (Step 68 gate PASS — no escape succeeded, so no E1):

| Attempt | Result |
|---------|--------|
| DNS resolve a public name | blocked |
| TCP connect to `1.1.1.1:53` | blocked |
| ICMP to `1.1.1.1` | blocked |
| write to `$HOME` (host) | blocked (read-only root) |
| write to `/etc` | blocked (read-only root) |
| enumerate host PIDs | isolated (only the ns's own pids visible) |
| `/proc/1` identity | the namespace init, not host `/sbin/init` |
| write to private `/tmp` scratch | allowed (legitimate, not an escape) |

Repro: run `unshare -r -n -m --pid --mount-proc --fork -- bash <probe>` where the probe
applies the prelude above and attempts each escape.

> **Known limitation of the unprivileged fallback**: FS confinement depends on
> user-namespace mount support (proven available here). On a host where
> `mount -o remount,ro,bind /` fails, the in-namespace bootstrap exits 97 and
> `run_harness` fails closed rather than running with a writable root. The no-egress
> property does not depend on the mount step.
>
> **TOCTOU note (for Phase 3)**: scope validates a harness file (incl. symlink
> resolution) at call time; the sandbox re-reads it read-only at exec, closing the
> validate→exec gap for the harness tree.

## Audit

Every call is audited (tool, decision + reason, pending flag, input size +
SHA-256 — never the input bytes, output size, duration) via the ADR-104
`Auditor` shape, as JSON lines on stderr.

## Out-of-band confirmation (Phase 4)

`lich_file_finding` is the one mutating tool. The Champion gate refuses it
pending-confirmation (a model's call carries no justification — ADR-104 Rule 2).
The server does not then run it: it parks a **ticket** under `var/lich/pending/`
(a 0700 directory the operator owns) bound to a `CallHash` over `{tool, project
root, canonical args}`. An operator approves out of band:

```
lich-mcp confirm list                 # pending tickets
lich-mcp confirm show <id>            # tool, input hash, args
lich-mcp confirm approve <id>         # authorises exactly that one call
```

The approval is single-use and bound to the exact call: re-issuing the identical
call proceeds once; any different call (different args, tool, or root) does not
inherit it. The approval code never travels through the model — only the
filesystem. Findings are still never auto-published; approval only lets the
staged register write happen.

## Routing & the two drivers (Phases 4–6)

Work is classified by **weight** (`claudebridge.AutoRouter`, ADR-053): MINOR
churn — seed/mutation/variant generation, status/lookup — stays **local and
free**; HEAVY work — crash triage, finding synthesis, novel-harness authoring,
multi-crash correlation — goes to **Claude**. A hint wins over prompt
heuristics; an ambiguous task defaults MINOR. Classification never bypasses the
gate, the dollar cap, or the external-data filter.

**Local driver (free, on-host).** `lich_local_llm` asks the already-running
llama-server (never starts/swaps a model). `pkg/lichtools` `GenerateSeeds`
drafts candidate inputs in a campaign's wire format, validates each as hex
bytes, dedups, and writes them as **inert** 0600 data files under the campaign's
seed dir only — a seed is bytes a fuzzer reads, never code. `GenerateRunCollect`
then runs a confined, bounded, no-egress harness over the corpus (no model call
on the run side). Each generation charges a daily compute budget (resets per UTC
day).

**Claude driver.** `lich-mcp drive -p <prompt>`:

```
lich-mcp drive -route auto -p "triage this crash and draft a finding"   # -> Opus
lich-mcp drive -route minor -p "draft monad-wire seed variants"         # -> Haiku
```

Haiku for MINOR volume, Opus for HEAVY triage (effort low/medium); the tools are
the Lich tools behind the same gate and confirmation flow, so a drafted finding
parks a ticket. **`-dry-run` is the default** (prints the request, sends
nothing). A hard **daily dollar cap** (`-daily-cap-usd`, default $3) reserves
worst-case before every send and fails closed. The **external-data (privacy)
filter** is required for a live send and redacts the prompt and every tool
result. The audit records only sizes and SHA-256 — never prompt or response
text; a gate refusal stays terminal (the system prompt forbids rephrasing around
it).
