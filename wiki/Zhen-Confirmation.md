# Zhen Confirmation

When a model (Claude through `zhen-claude`, or the local brain through `zhen-agentd`) asks the Champion for a write, the gate refuses it **pending confirmation** (Rule 2: a model is not a trusted caller). Confirmation is how Stevie lets one such call through, on a channel the model cannot see or answer.

**The rule:** no approval code, token or handle ever reaches the model. It only ever sees "approved by the human operator", "denied", or "queued for human approval as `<id>`" (the id authorises nothing without Stevie's own Well login).

## Modes (`zhen-claude -confirm`)

| Mode | What happens | Needs |
|---|---|---|
| `deny` (default) | The refusal goes back to Claude; nothing runs. | nothing |
| `tty` | The call is shown on the terminal with a 4-character code; type the code to approve, anything else denies. | a controlling terminal (`/dev/tty`; exits 2 without one). Never reads stdin. |
| `queue` | The call is stored in the Well and announced on Wotan (`zhen.confirm.pending`, id + tool + summary). Claude is told it is queued. | `WELL_APP_ZHEN_DSN` (unheaded_app as `app_zhen`) |

`zhen-agentd -confirm-queue` does the same for agent-loop calls to `/api/v1/tool/exec` (callers whose `emitted_by` starts with `zhen-agent`): they are queued (HTTP 202 + id) instead of getting a token; without the flag they are denied. The web UI's own path (`pending_token` + `/api/v1/agent/confirm`) is unchanged.

## What you see

```
=== zhen: a model asks to run a mutating tool ===
tool:     write_file (Champion)
sandbox:  /home/govan/tmp/unheaded
asked by: claude-api/claude-opus-5-5/<conversation>
sha256:   <hash of tool + args + project root>
args (secrets redacted):
  content: (412 bytes; see the diff)
  path: /home/govan/tmp/unheaded/notes.txt
diff:
--- ... (current)
+++ ... (proposed)
-old line
+new line
type k7qp to approve, anything else denies:
```

## Approving queued calls

```bash
export WELL_APP_ZHEN_DSN=...            # from your shell, never the repo
zhen-claude confirm list
zhen-claude confirm show <id>           # details + diff against the file as it is now
zhen-claude confirm -allowed-roots /home/govan/tmp/unheaded approve <id>
zhen-claude confirm deny <id>
```

`approve` re-reads the row and refuses unless it is still pending, not expired (24 h), its project root is on `-allowed-roots` (default: the current directory), and the stored call still hashes to what was queued. It prints the diff it is about to run (recomputed, not taken from an earlier `show`), marks the row approved with `decided_by = $USER@host`, runs the call through the Champion (Rules 3 and 1 again, symlinks resolved) and records `executed` or `failed`.

## Never confirmable

- anything but the Champion's own `write_file`, `patch_file`, `kanban_create`, `kanban_update` (no `runbook_execute`, `model_switch`, shell, or unknown tools);
- MCP mutating tools (`file_write`, `file_patch`, `runbook_execute` from the Zhen MCP server): they stay refused;
- a destructive verb in any argument (Rule 3) or a path outside the sandbox, a denied path, or a symlink out of it (Rule 1), approval or not.

## Limits and records

- Queue: at most 20 pending per service (code and database), states move one way (`pending` → `approved`/`denied`/`expired`, `approved` → `executed`/`failed`), the call is immutable, rows are never deleted (`db/migrations/031`).
- Every step is an `audit_events` row (`confirm_issue`, `confirm_approve`, `confirm_deny`, `confirm_expire`; see [[Zhen Audit|Zhen-Audit]]).
- A queued Kanban call fails when approved from the CLI until the subcommand gets a Kanban store.

---

> **Source:** [ADR-104 §3](../docs/adr/ADR-104-zhenai-claude-api-bridge.md) · [S-TWO-BRAINS plan](../docs/battle-plans/S-TWO-BRAINS-battle-plan.md) Phase 2 · `pkg/claudebridge/confirm.go` · `pkg/champion/confirmcall.go` · `cmd/zhen-claude/confirm_cmd.go`
