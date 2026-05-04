# ADR-060 — Zhenai Multi-Model Selector (UI dropdown for live llama-server swap)

**Status:** In Progress (activated 2026-05-04 mid-WAVE16; implementation lands tonight alongside the model-vetting overnight run)
**Date:** 2026-05-04
**Deciders:** Stevie Bellis + unheaded-blackmage + unheaded-developer (when activated)
**Context owner:** zhenai operator surface
**Triggered by:** Stevie's question 2026-05-04: *"how long does it take to swap/load models? could we have multiple models available to select from on zhenai web ui?"*

---

## Context

Today's model-interchange seam is `scripts/switch-model.sh <key>` (ADR-059-era, shipped 2026-05-04). It atomically swaps which model `llama-server` serves on `:8081` by killing the running process and relaunching with a different `--model` flag. Empirical swap times measured 2026-05-04:

- qwen-7b q4 (16k ctx, all-GPU): ~45 s warm
- gemma-4-E2B fp16 (8k ctx, all-GPU): ~88 s
- deepseek-coder-v2-lite (`--n-cpu-moe 20 --no-mmap`): ~142 s

The script works, but only the operator with shell access can invoke it. A web UI dropdown exposes the same capability to anyone who can reach the Zhenai page — which on the LAN-only deployment posture is just Stevie at his desk, but the seam is what matters for the threat model.

This ADR scopes the work as a **future activation** because:

1. The current single-model default (qwen-7b) is the H0 coding-gate baseline; the gemma + deepseek vetting (`eval/coding-gate/gemma-vet-2026-05-04/LAB-NOTEBOOK.md`) didn't surface a meaningful upgrade. Multiple models on the picker is more valuable AFTER a real second model proves itself in vetting.
2. The mutation surface (a button that runs a shell script as a subprocess) needs deliberate design from the BlackMage lens before it ships. Cheap to write, expensive to recall if it lands wrong.
3. Stevie is also evaluating cloud-rented training (ADR-061) as a parallel path. If that lands first, the "selector" becomes "old base vs Unheaded-tuned" — a different design conversation.

---

## Decision

**Build `cmd/zhen-agentd /api/v1/tool/exec` handler for `model_switch`** + a sidebar `<select>` in `raft/static/index.html` that posts to it. Single binary, single subprocess invocation, Champion-gated like every other mutation in the operator surface. **NOT** a parallel-llama-servers-on-different-ports design (ruled out below).

Activation criteria (any one):

- A second model passes the H0 coding gate ≥ qwen-7b's 12/14 textbook score, making "let me try the other one for this question" a real workflow.
- Stevie wants to A/B-compare a custom-trained model (ADR-061 outcome) against qwen-7b in side-by-side chat.
- A third model genuinely-better-than-qwen lands (e.g., a larger card arrives or zhen_app retrieval is tightened to fit deepseek-cpu cleanly).

Until then: this ADR is the design contract. The implementation is queued.

---

## BlackMage threat model

### Attack surface

The model-switch seam exposes:

1. A **subprocess execution path** in `cmd/zhen-agentd` that takes a string (model key) and runs `scripts/switch-model.sh <key>` as a child process.
2. A **browser-reachable POST endpoint** at `/api/v1/tool/exec` (already shipped as part of T6b closure) that any LAN client can hit.
3. A **trust boundary** between "UI-supplied string" and "shell argument" — historically the most exploited boundary in any web app.

### Pre-ship threat catalog (T-numbered for the application threat model)

| ID | Threat | Mitigation |
|---|---|---|
| **T11** | Shell injection via crafted model key (e.g. `qwen-7b; rm -rf $HOME`) | Champion gate enforces an **enum allowlist** of valid model keys parsed once at boot from `scripts/switch-model.sh`'s `MODEL_FILE` array. Any key not in the allowlist returns 400 *before* `subprocess.run` is called. The allowlist is the source of truth, not the UI dropdown options. |
| **T12** | Path traversal via `key=../../etc/passwd`-style indices | Same allowlist guard catches this; defense-in-depth via `os.path.abspath(...)` check that the resolved script path lives under `<project_root>/scripts/`. |
| **T13** | DoS via rapid swap requests (model load takes 45 s – 3 min × N requests) | Rate-limit at the Champion gate: max 1 in-flight model-switch per 5 minutes. Concurrent requests get 429 + a "swap-in-progress" header pointing at the current operation's poll URL. |
| **T14** | UI-driven trust escalation (chat-driven LLM emits a tool_call with model_switch) | model_switch is a **destructive verb** — Champion's third rule (destructive-verb hard deny) refuses the call unless the justification chain ends in `direct-user`. LLM-emitted tool calls carry `zhen-agent` justification; they're rejected at the gate. |
| **T15** | Time-of-check-to-time-of-use (TOCTOU) race: allowlist check passes, then `switch-model.sh` is replaced before subprocess fork | Verify `scripts/switch-model.sh` file hash matches an at-boot snapshot before each invocation. Mismatch → 503 + alert. |
| **T16** | Script execution surfaces sudo or root capabilities | `scripts/switch-model.sh` runs as the daemon's UID (whatever uid `zhen-agentd` has — currently `govan`). It should NEVER need sudo. Hard-fail at boot if `EUID == 0`. |
| **T17** | Model file substitution (attacker writes a malicious GGUF to a known path) | `MODEL_FILE` paths under `/var/zhen/models/` are owned by `govan:govan` mode 0644. Filesystem ACLs are the trust boundary. Out of scope for the ADR; tracked under existing T6b/B1 source-trust posture. |
| **T18** | UI confusion: dropdown shows "deepseek" but daemon is actually loaded with "qwen" (state drift) | After every swap, the UI polls `/api/v1/stats` until `inference_model` matches the request. Optimistic updates are forbidden — the dropdown reflects *llama-server's actual loaded model*, not "what the user clicked". |
| **T19** | Long-running swap leaves UI in a "loading…" state forever (gemma fp16 on cold cache: 90+ s, deepseek-cpu: 142 s) | Toast UI shows the model's documented expected boot time + a "still working at 60 s, 90 s, 120 s…" progress beat. Hard timeout at 4× the documented load time → 504 + automatic rollback to previously-loaded model. |
| **T20** | Audit trail gap: model swap not in `zhen_actions` | Every swap logs to `zhen_actions` as `action_type='model_switch'` with `triggered_by='direct'` (browser-clicked), captured `intent` (the human-readable label), and `status` (planned/completed/failed). Already fits migration `011_zhen_actions_relax_constraints.sql`. |

### Lich campaign (LICH-013) — pre-registered

Once the feature ships, the Lich runs continuously against `/api/v1/tool/exec` with `tool=model_switch`:

- **Mutation fuzzer**: random byte permutations of valid model-key strings, 1 M reqs over 24h, expect 100% rejection at gate, zero subprocess invocation.
- **Path-traversal grammar**: `../`, `..\\`, URL-encoded equivalents, NUL byte injection. Same expectation.
- **Concurrent-swap fuzzer**: 100 parallel POSTs, expect exactly 1 to succeed and the other 99 to get 429 with sane error bodies.
- **Allowlist drift detector**: if `scripts/switch-model.sh` adds a key without a corresponding ADR amendment to T11, CI fails.

### Trust-level placement (per ADR-019)

`model_switch` is **Trust Level 2** (sandboxed local change agent). It writes to `/sys/fs/bpf/...` (no), runs `git commit` (no), `kubectl` (no), `rm` (no) — but it *does* spawn a long-running subprocess that owns a GPU + 5-10 GB RAM. The subprocess belongs to a pinned set of model files Stevie deliberately curated; swapping among them is bounded.

It is NOT Trust Level 3 (privileged) because it doesn't touch system state outside the daemon's user-owned directories. But it is NOT Trust Level 1 (read-only) either because it kills + spawns processes.

---

## Developer lens — implementation contract

### Files & shapes

```
cmd/zhen-agentd/
  toolexec_modelswap.go     # NEW: handler for tool=model_switch
  toolexec_modelswap_test.go # NEW: 12 tests covering all T11-T20 mitigations
pkg/champion/
  modelswap.go              # NEW: ModelSwap(ctx, key) — runs the script, parses output
  modelswap_test.go         # NEW: golden-output parser tests, allowlist enforcement
scripts/
  switch-model.sh           # MODIFY: emit JSON status line on stdout for parser
                            #         (don't break the existing human-readable output —
                            #          add a `--json` flag that switches to JSON-only)
raft/zhen_app.py
  /api/v1/models            # NEW: GET — returns the allowlisted keys (parsed from
                            #             switch-model.sh at zhen_app boot)
  /api/v1/models/switch     # NEW: POST — proxies to zhen-agentd /api/v1/tool/exec
                            #             with tool=model_switch + direct-user
                            #             justification
raft/static/index.html
  Sidebar <select>          # NEW: populated from /api/v1/models, on-change posts
                            #      to /api/v1/models/switch with optimistic-disable
                            #      UI lock until poll confirms.
db/migrations/012_zhen_actions_model_switch.sql
                            # OPTIONAL — only if the existing zhen_actions schema
                            # doesn't accommodate the new action_type. Migration
                            # 011 already relaxed the CHECK constraint, so likely
                            # no migration needed. Verify before activation.
```

### Tests (TDD-first; written before implementation)

12 unit tests against `pkg/champion/modelswap_test.go`:

1. `TestAllowlistRejectsUnknownKey` — `model_switch("evil")` → returns `ErrUnknownModel` without spawning a subprocess (verified by mocking `exec.Command`).
2. `TestAllowlistRejectsShellInjection` — `model_switch("qwen-7b; rm -rf /")` → same.
3. `TestAllowlistRejectsPathTraversal` — `model_switch("../../etc/passwd")` → same.
4. `TestAllowlistRejectsNulByte` — `model_switch("qwen-7b\x00; payload")` → same.
5. `TestScriptHashMismatchHalts` — flip a byte in `scripts/switch-model.sh` between boot and call → returns `ErrScriptModified`.
6. `TestConcurrentSwapBlocked` — first call holds the lock; second call returns `ErrSwapInProgress` immediately (no queueing).
7. `TestSwapTimeoutRollsBack` — child process exceeds 4× expected → SIGTERM, then SIGKILL, restart with previous model.
8. `TestSwapEmitsZhenActionRow` — successful swap writes a `model_switch` row with `status=completed`.
9. `TestFailedSwapEmitsZhenActionRow` — failed swap writes a `model_switch` row with `status=failed` + stderr in details.
10. `TestSwapRefusesNonRoot…WaitNotRoot` — actually `TestSwapRefusesIfRunningAsRoot` — daemon EUID==0 → swap returns `ErrPrivilegeEscalation` (T16).
11. `TestSwapDispatchAcceptsDirectUserOnly` — `triggered_by='zhen-agent'` → `ErrDestructiveVerbDeny`.
12. `TestSwapAllowsDirectUser` — `triggered_by='direct'` → swap proceeds.

8 integration tests against the live daemon (`cmd/zhen-agentd/toolexec_modelswap_test.go`):

13-20. Same shape but going through HTTP, with the actual subprocess running. CI marks these as `_integration` build-tagged so they only run on the `west` host, not in GitHub Actions.

### Observability

Three Prometheus counters under the existing `zhen_agentd_*` namespace:

- `zhen_agentd_model_swap_total{key,status}` — increments per swap attempt
- `zhen_agentd_model_swap_duration_seconds{key}` — histogram of swap wall-times
- `zhen_agentd_model_swap_inflight` — gauge of currently-loading swaps (should be 0 or 1)

Plus a `model_swap` log line at INFO with structured fields: `from_model`, `to_model`, `triggered_by`, `duration_ms`, `status`.

### Failure modes & UX

| Failure | UI behaviour |
|---|---|
| Allowlist reject (T11/12/14) | Inline toast: "model 'X' is not registered. Refresh to reload available models." |
| Already-in-progress (T13) | Disable dropdown for the duration of the in-flight swap; toast: "swapping… ~Xs estimated" |
| Daemon down | Dropdown grayed; tooltip: "model swap requires zhen-agentd; start it with `./start-zhen.sh -gated`" |
| Subprocess timeout (T19) | Toast: "swap to X timed out at Ys; rolled back to Y. See `/tmp/llama-X.log` for details." |
| Subprocess died with non-zero exit | Toast: "swap failed: <last 200 chars of stderr>. Rolled back." |

### What this ADR explicitly does NOT do

- Multiple llama-servers in parallel on different ports. **Rejected**: VRAM math doesn't work on the 12 GB RX 7700 XT (qwen 6 + gemma 6 + deepseek 11 = 23 GB ≫ 12 GB), and the architecture would couple `pkg/ports` registry changes to every model addition. If a future user has a 24 GB+ card, this ADR can be revised.
- Hot-swap with no chat downtime. **Rejected**: requires either pre-loading both models (rejected above) OR intercepting in-flight requests with retry-after-load — both deserve their own ADR if Stevie wants them.
- A general-purpose "tool" picker. The model selector is the only mutation we currently want surfaced from the UI. Other tools (runbook execution) already have their own UI affordances.

---

## Consequences

### Positive

- Operator can pivot models without leaving the chat tab. With the empirically-measured 45 s – 3 min cost made visible, the workflow becomes "ask qwen → if answer is shaky, switch to deepseek-cpu for a thorough re-check, switch back when done."
- Closes the "where's the model picker" question for visitors. Useful when demoing the LAN-only operator surface.
- Lich campaign LICH-013 codifies the security expectations; the surface gets adversarial testing automatically.

### Negative

- Yet another mutation endpoint. Every endpoint is a new attack surface even when individually well-tested. The T-numbering exists to keep the bar deliberate.
- Adds ~600 LOC of Go (handler + tests) + ~100 LOC of Python (UI endpoints) + ~150 LOC of HTML/JS (sidebar control). Total maintenance burden modest but real.
- Couples `cmd/zhen-agentd` to `scripts/switch-model.sh` semantics. If the script signature changes, the Go parser breaks. **Mitigation**: golden-output parser test at the Go layer + script's `--json` flag for stable output.

### Neutral

- The ADR doesn't alter qwen-7b's status as the default chat model. Default-model selection remains a `ZHEN_MODEL` env var read at zhen_app boot.

---

## References

- ADR-019 — Zhen Champion Agent (the gate that catches T11-T20)
- ADR-059 — Zhenai Interactive CLI (sister ADR; the CLI gets the same selector via slash-command)
- `scripts/switch-model.sh` — the seam this ADR wraps
- `eval/coding-gate/gemma-vet-2026-05-04/LAB-NOTEBOOK.md` — the empirical data backing the activation criteria
- `docs/security/application-threat-model.md` — T1-T10 catalog this ADR's T11-T20 extend
- `cmd/zhen-agentd/toolexec.go` — the existing `/api/v1/tool/exec` endpoint the new handler plugs into
