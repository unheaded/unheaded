---
title: B2 design — Champion tool-call envelope with source-provenance gating
status: design (implementation deferred — depends on B1 + Phase D-A)
date: 2026-05-02
---

# B2 design — Champion tool-call envelope

Drafted under unheaded-marshal lane discipline as the design half of the deferred B2 blocker. Implementation requires B1 to land first AND requires Phase D-A's agent runtime to exist (which is itself blocked behind B1+B2). This doc nails the contract so D-A implementation is not paralyzed by B2 design ambiguity.

## Threat model recap

The chained source-poison + meta-instruction injection (probe A1+A2) demonstrated the model can be coerced into emitting tool calls based on attacker-controlled retrieval content. Today's `pkg/champion/` Trust L2 (`WriteFile`, `PatchFile`, `UpdateKanbanTask`, `RevertAction`) is gated only on path/resource allowlist — there is NO check that the *justification* for the call comes from a trusted source.

Phase D-A wires zhen-rag (retrieval + synthesis) to Champion (tool execution). The agent loop is:

```
user_msg → zhen-rag(user_msg, refs[]) → llm_response → tool_call(args, justification) → Champion.execute → observation → loop
```

The `justification` field is the new contract: it carries the IDs of retrieval references that produced the tool call. Champion uses it to gate.

## Tool-call envelope (proposed)

```go
package champion

// ToolCall is the envelope every agent-emitted tool invocation flows
// through. The Champion validates this structure BEFORE dispatching to
// the underlying ReadFile / WriteFile / etc.
type ToolCall struct {
    // Name is the registered tool name: "read_file", "write_file",
    // "patch_file", "kanban_create", "kanban_update", "kanban_list",
    // "runbook_show", "runbook_execute", "corpus_search", ...
    Name string `json:"name"`

    // Args is the tool-specific argument map.
    Args map[string]any `json:"args"`

    // Justification is the chain of references that produced this
    // tool call. Each entry corresponds to a retrieval result that the
    // model cited, with its source provenance from the B1 schema.
    Justification []Reference `json:"justification"`

    // EmittedBy identifies which model + session produced the call —
    // for audit. ("zhen-rag@<gguf-sha>", "session-uid", etc.)
    EmittedBy string `json:"emitted_by"`
}

// Reference mirrors the cs/vor B1-schema source-provenance fields.
type Reference struct {
    Topic       string `json:"topic"`
    Category    string `json:"category"`
    SourceKind  string `json:"source_kind"`   // "embedded" | "user-custom" | "user-source"
    SourcePath  string `json:"source_path,omitempty"`
    SourceLabel string `json:"source_label,omitempty"`
    Excerpt     string `json:"excerpt,omitempty"` // 200-char snippet for audit
}
```

## Trust gate (Champion side)

```go
// IsMutating reports whether the tool call would modify state.
func (tc *ToolCall) IsMutating() bool {
    switch tc.Name {
    case "write_file", "patch_file", "delete_file",
         "kanban_create", "kanban_update", "kanban_delete",
         "runbook_execute", "system_command":
        return true
    case "read_file", "kanban_list", "runbook_show", "corpus_search",
         "service_health":
        return false
    }
    return true // fail-closed for unknown tools
}

// HasUntrustedJustification reports whether any reference in the
// justification chain has SourceKind == "user-source" (external).
func (tc *ToolCall) HasUntrustedJustification() bool {
    for _, r := range tc.Justification {
        if r.SourceKind == "user-source" {
            return true
        }
    }
    return false
}

// AcceptToolCall is the Champion's gate. Called before any underlying
// dispatch. Returns nil if the call is permitted, or an error
// describing why it was refused.
func (c *Champion) AcceptToolCall(ctx context.Context, tc ToolCall) error {
    // Always log the attempt regardless of outcome.
    actionID, _ := c.actionStore.LogAction(ctx, &Action{
        ActionType:  "tool_call_attempt",
        Intent:      tc.Name,
        Parameters:  marshalArgs(tc.Args),
        TriggeredBy: tc.EmittedBy,
        PlannedAt:   time.Now(),
    })

    // Rule 1: existing path-allowlist gate (unchanged from current
    // ReadFile/WriteFile validation logic).
    if err := c.checkPathAllowlist(tc); err != nil {
        c.actionStore.UpdateAction(ctx, actionID, "denied", "",
            "path_allowlist: "+err.Error(), 0)
        return err
    }

    // Rule 2 (NEW): mutating + user-source justification → deny pending
    // out-of-band confirmation.
    if tc.IsMutating() && tc.HasUntrustedJustification() {
        msg := fmt.Sprintf("tool %q is mutating and justification "+
            "includes user-source content (%s); requires out-of-band "+
            "user confirmation",
            tc.Name, summarizeUntrustedRefs(tc.Justification))
        c.actionStore.UpdateAction(ctx, actionID, "denied_untrusted_justification",
            "", msg, 0)
        return fmt.Errorf("untrusted justification: %s", msg)
    }

    // Rule 3 (NEW): destructive verbs in args (defense-in-depth even
    // for "trusted" justifications). Same verb list as the zhen-rag
    // system prompt's DESTRUCTIVE-VERB FILTER.
    if containsDestructiveVerb(tc.Args) {
        msg := "tool call args contain destructive shell verbs; refused"
        c.actionStore.UpdateAction(ctx, actionID, "denied_destructive",
            "", msg, 0)
        return fmt.Errorf("destructive: %s", msg)
    }

    c.actionStore.UpdateAction(ctx, actionID, "accepted", "", "", 0)
    return nil
}
```

## Out-of-band confirmation (Rule 2 follow-up)

When `AcceptToolCall` denies due to untrusted justification, the agent should NOT silently retry — it should emit a `pending_user_confirmation` event that surfaces:

- The tool call (name + args).
- The untrusted reference summary (`source_label` + 200-char excerpt).
- A confirmation token the user can return out-of-band (CLI prompt, Slack, etc.).

The Champion exposes `ConfirmPendingToolCall(ctx, token)` that, given a valid token, re-executes the tool call without the Rule 2 check. The token is single-use, time-limited (5 min default), and bound to the specific tool call (not transferable).

This is **explicitly NOT** the same as just retrying the call — the user has affirmatively reviewed the untrusted source before approving execution.

## What remains in the agent layer (Phase D-A scope, not B2)

- The actual ReAct loop that decides which tool to call when.
- The system prompt that explains the trust labels to the model.
- The conversation state machine (multi-turn, tool-output-as-observation).
- The escalation policy for repeated failures.

These are Phase D-A's job. B2's job is to ensure Champion can reject any tool call that violates the trust contract regardless of how the agent layer constructs it.

## Implementation phases (post-B1)

1. **Add ToolCall + Reference types** (~half day, `pkg/champion/`).
2. **Implement AcceptToolCall** (~1 day) — the three rules + audit logging.
3. **Add destructive-verb verb-list** (~1 hour, shared between this and zhen-rag's system prompt).
4. **Add pending-confirmation flow** (~1 day) — single-use token, timeout, replay protection.
5. **Tests** (~1 day) — every (mutating, untrusted) combination + audit trail verification.

Total: ~3-4 days of cleanly-scoped Champion work, gated on B1 landing.

## Probe-citation

This design closes the Phase D-A blocker that probe A1+A2 surfaced. The destructive-verb filter at the LLM layer (D-pre.1) is the first defense; the trust gate at the Champion layer (this design) is the second. The two are intentionally redundant — the model can be coerced past D-pre.1 by sufficiently clever prompt-engineering, and Champion's trust gate is the structural fail-safe.
