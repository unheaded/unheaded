// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package champion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"unheaded/pkg/zhenrag"
)

// ToolCall is the envelope every agent-emitted tool invocation flows
// through. The Champion validates this structure BEFORE dispatching to
// the underlying ReadFile / WriteFile / etc.
//
// See eval/coding-gate/probe-2026-05-02/B2-design-champion-tool-call-gate.md
// for the threat model and gate-rule rationale (probe A1+A2 chained
// source-poison + meta-instruction injection demonstrates the agent
// layer must not blindly execute model-emitted tool calls).
type ToolCall struct {
	// Name is the registered tool name: "read_file", "write_file",
	// "patch_file", "kanban_create", "kanban_update", "kanban_list",
	// "runbook_list", "runbook_show", "runbook_execute", "run_check",
	// "model_switch", "memory_write", or an MCP server's tool ("corpus_search",
	// "service_health", ...). See MutatingTools and ReadOnlyTools.
	Name string `json:"name"`

	// Args is the tool-specific argument map. Keys depend on Name.
	Args map[string]any `json:"args"`

	// Justification is the chain of references that produced this
	// tool call. Each entry corresponds to a retrieval result that the
	// model cited, with its source provenance from B1 (cs/vor's
	// /api/topics + /api/search source_kind / source_trust fields).
	Justification []Reference `json:"justification"`

	// EmittedBy identifies which model + session produced the call —
	// for audit. Free-form string; e.g. "zhen-rag@<gguf-sha>" or
	// "session-uid".
	EmittedBy string `json:"emitted_by"`

	// Principal is the authenticated caller the call acts for, set by
	// the serving layer from its own authentication, never from model
	// output or a request body: "ui:<subject>", "tty:<user>". A pending
	// confirmation is bound to it (RT-03): only the same principal, or
	// "queue:stevie", may confirm; "agent:<model>" never confirms.
	Principal string `json:"principal,omitempty"`
}

// Reference mirrors the cs/vor B1-schema source-provenance fields.
// Populated by the agent layer from vor's /api/topics or /api/search
// JSON response.
//
// Chain contract (chain.go, S-CHAMPION-HARDENING Step 35): a Reference
// with SourceTrust "local" MUST carry Topic (the vor topic id) and Digest
// (lowercase hex SHA-256 of the retrieved text the model was given).
// VerifyChain recomputes every Digest from the text the caller retrieved
// for that topic; a reference that fails is not trustworthy and the agent
// layer downgrades it to "external", so Rule 2 refuses the mutating call
// for the right reason.
type Reference struct {
	Topic       string `json:"topic"`
	Category    string `json:"category"`
	SourceKind  string `json:"source_kind"`  // "embedded" | "user-custom" | "user-source"
	SourceTrust string `json:"source_trust"` // "canonical" | "local" | "direct-user"; anything else is untrusted
	SourcePath  string `json:"source_path,omitempty"`
	SourceLabel string `json:"source_label,omitempty"`
	Excerpt     string `json:"excerpt,omitempty"` // 200-char snippet for audit
	Digest      string `json:"digest,omitempty"`  // sha256 (hex) of the retrieved text; required for "local"
}

// MutatingTools is the canonical list of tool names that modify state.
// Anything not in this list is treated as read-only by IsMutating.
//
// CONSERVATIVE: an unknown tool name is treated as MUTATING (fail-closed).
// New tools must register here explicitly.
//
// Every name here is dispatched (dispatch.go) or run by an MCP server
// that gates through AcceptToolCall (RT-09). system_command, delete_file
// and kanban_delete were listed but never implemented and were removed
// (S-CHAMPION-HARDENING Step 53): an unlisted name is still mutating by
// default here and refused as unknown by Dispatch. system_command must
// not get an implementation; arbitrary shell needs its own ADR.
var MutatingTools = map[string]bool{
	"write_file":      true,
	"patch_file":      true,
	"kanban_create":   true,
	"kanban_update":   true,
	"runbook_execute": true,
	"model_switch":    true, // ADR-060: kills+respawns llama-server subprocess
	// memory_write (S-CHAMPION-HARDENING Step 209): one zhen_memories row,
	// written for zhen-cli /remember through zhen-agentd.
	"memory_write": true,
	// Lich MCP (S-LICH-AWAKENS, ADR-098): writes the findings register, so it
	// is mutating and — with no retrieval justification from a model — always
	// pending-confirmation (Rule 2). Never auto-publishes.
	"lich_file_finding": true,
}

// ReadOnlyTools is the canonical list of read-only tool names. Used to
// distinguish "known safe" from "unknown" in IsMutating's fail-closed
// default.
var ReadOnlyTools = map[string]bool{
	"read_file":      true,
	"file_read":      true, // raft/zhen_mcp_server.py's read; Rule 1 still checks its path
	"kanban_list":    true,
	"runbook_show":   true,
	"runbook_list":   true,
	"corpus_search":  true,
	"service_health": true,
	// raft/zhen_mcp_server.py's local_llm: one completion from the
	// llama-server that is already running. It loads nothing (model
	// loads go through model_switch and switch-model.sh's fit check),
	// writes nothing, and its answer goes back to the caller as text.
	"local_llm": true,
	// Lich MCP (S-LICH-AWAKENS, ADR-062/063). list/show/read/triage read the
	// Lich framework and crash artifacts — plainly read-only. lich_run_harness
	// executes a fuzzer, but ONLY inside a confined, no-egress sandbox with a
	// server-clamped duration (pkg/lichtools): its control is server-side scope
	// + confinement, not per-call human confirmation, so the gate treats it as
	// non-mutating. (lich_file_finding is mutating — see MutatingTools.)
	"lich_list_campaigns": true,
	"lich_show_campaign":  true,
	"lich_read_crash":     true,
	"lich_triage":         true,
	"lich_run_harness":    true,
	// lich_local_llm (S-LICH-AWAKENS Phase 4): one completion from the local
	// llama-server for MINOR-weight seed/mutation drafting. It loads nothing,
	// writes nothing, costs no dollars, and stays on the host — read-only.
	"lich_local_llm": true,
	// run_check (S-TWO-BRAINS Step 161): runs an allowlisted check in a
	// sandbox inside an agent worktree; it changes nothing outside it.
	"run_check": true,
}

// destructivePattern matches any of the destructive shell-verb forms
// that, when appearing in any string-valued argument, trigger Rule 3
// (destructive-verb refusal) regardless of source-trust. It is rendered
// from pkg/zhenrag's verb table, the same table the chat and agent prompts
// list their verbs from, so the gate and the prompts cannot drift.
var destructivePattern = zhenrag.DestructivePattern

// IsMutating reports whether the tool call would modify state. Returns
// true for explicitly-mutating tools, false for explicitly-read-only
// tools, and true (fail-closed) for unknown tool names.
func (tc *ToolCall) IsMutating() bool {
	if MutatingTools[tc.Name] {
		return true
	}
	if ReadOnlyTools[tc.Name] {
		return false
	}
	// Unknown tool name → fail-closed: treat as mutating.
	return true
}

// TrustedSourceLabels is the closed set of SourceTrust values a
// justification may carry. Anything else, including case or whitespace
// variants, is untrusted (RT-06: the old check compared only against
// "external", so a typo passed).
var TrustedSourceLabels = map[string]bool{"canonical": true, "local": true, "direct-user": true}

// HasUntrustedJustification reports whether the justification chain
// is untrusted from the gate's perspective. Returns true if either:
//
//   - Any reference has a source_trust outside TrustedSourceLabels:
//     "external" (user-symlinked source under ~/.config/cs/sources/ —
//     possibly poisoned) or any other unlisted string, OR
//
//   - The chain is EMPTY and the tool is mutating. Empty justification
//     on a mutating tool is treated as untrusted by default
//     (fail-closed) because:
//
//     1. An attacker can craft a prompt that doesn't match the seed-
//     retrieval pattern (refs ends up empty) AND inlines a poisoned-
//     content reference textually in the user prompt. The model is
//     influenced by the inline content; the gate sees no refs.
//     Without this check the gate is bypassable in that case.
//     See eval/coding-gate/probe-2026-05-02/A2-agent-adversarial.md.
//
//     2. A model that emits a tool call without ANY justification chain
//     has no demonstrable grounding for the call — caller should
//     either supply a justification or explicitly opt into
//     DirectTrusted by populating Justification with a single
//     canonical reference (e.g., {SourceTrust: "direct-user"}).
//
// For non-mutating (read-only) tools, empty justification is fine —
// reading without a citation is normal exploration.
//
// Programmatic callers that want to bypass this check (because the
// invocation isn't model-derived at all) should populate Justification
// with a Reference whose SourceTrust is one of the trusted strings
// ("canonical", "local", "direct-user"). The first-class types are
// agent-scoped; "direct-user" is the escape hatch for direct callers
// who can't construct a meaningful retrieval-derived chain.
func (tc *ToolCall) HasUntrustedJustification() bool {
	if len(tc.Justification) == 0 {
		// Empty chain on a mutating tool is fail-closed untrusted.
		// Empty chain on a read-only tool is fine.
		return tc.IsMutating()
	}
	for _, r := range tc.Justification {
		if !TrustedSourceLabels[r.SourceTrust] {
			return true
		}
	}
	return false
}

// HasDestructiveVerb reports whether any string-valued argument contains
// a known destructive shell verb. Recurses into nested maps and slices
// so payloads can't smuggle a destructive command via, e.g., a
// `["sh", "-c", "rm -rf /"]` argv slice.
func (tc *ToolCall) HasDestructiveVerb() bool {
	return containsDestructive(tc.Args)
}

func containsDestructive(v any) bool {
	switch x := v.(type) {
	case string:
		return destructivePattern.MatchString(x)
	case map[string]any:
		for _, sub := range x {
			if containsDestructive(sub) {
				return true
			}
		}
	case []any:
		for _, sub := range x {
			if containsDestructive(sub) {
				return true
			}
		}
	}
	return false
}

// ToolCallDecision summarizes the gate's verdict. AcceptToolCall returns
// nil on Allow; the Reason field is informational and (when non-empty)
// is what the caller surfaces to the user / agent layer as feedback.
type ToolCallDecision struct {
	Allow  bool
	Reason string // empty when Allow is true
	// PendingConfirmation indicates the call was refused due to
	// Rule 2 (untrusted justification on a mutating tool). The
	// agent layer can route it through a separate confirm-with-user
	// flow rather than treating it as a hard rejection.
	PendingConfirmation bool
}

// AcceptToolCall is the Champion's gate. Returns ToolCallDecision and
// (for hard-rejection cases) an error. The decision is logged to
// ActionStore regardless of outcome, so the audit trail captures
// every attempted call.
//
// Rule 1: existing path-allowlist gate (delegates to validatePath
//
//	for tool calls that include a `path` arg).
//
// Rule 2: mutating + untrusted justification → refuse pending
//
//	out-of-band user confirmation. PendingConfirmation=true.
//
// Rule 3: any string arg contains a destructive shell verb → refuse
//
//	absolutely (defense-in-depth even when justification is trusted).
//
// The ordering matters: Rule 3 takes precedence over Rule 2, which
// takes precedence over Rule 1, so a destructive payload is logged
// as such (not as a path-allowlist failure).
func (c *Champion) AcceptToolCall(ctx context.Context, tc ToolCall) (ToolCallDecision, error) {
	start := time.Now()
	intent := fmt.Sprintf("ToolCall: %s", tc.Name)
	actionID, _ := c.logAction(ctx, "tool_call_attempt", intent, tc.EmittedBy)

	// Rule 3: destructive-verb in args (highest priority).
	if tc.HasDestructiveVerb() {
		decision := ToolCallDecision{
			Allow:  false,
			Reason: "tool call args contain destructive shell verbs",
		}
		c.completeAction(ctx, actionID, "denied_destructive",
			summarizeArgs(tc.Args), decision.Reason,
			time.Since(start))
		return decision, fmt.Errorf("tool call denied: %s", decision.Reason)
	}

	// Rule 2: mutating + untrusted justification, unless branch-sandbox
	// mode answers it (branchsandbox.go: write_file/patch_file inside a
	// zhen/<name> agent worktree; Rules 1 and 3 still apply).
	sandboxJustification, sandboxed := c.branchSandboxAllows(tc)
	if tc.IsMutating() && tc.HasUntrustedJustification() && !sandboxed {
		var reason string
		if len(tc.Justification) == 0 {
			reason = fmt.Sprintf("mutating tool %q has empty justification chain; requires out-of-band user confirmation (fail-closed: agent could not produce a retrieval-derived rationale for the call)", tc.Name)
		} else {
			reason = fmt.Sprintf("mutating tool %q has external-trust justification (%s); requires out-of-band user confirmation", tc.Name, untrustedRefSummary(tc.Justification))
		}
		decision := ToolCallDecision{
			Allow:               false,
			Reason:              reason,
			PendingConfirmation: true,
		}
		c.completeAction(ctx, actionID, "denied_untrusted_justification",
			summarizeArgs(tc.Args), decision.Reason,
			time.Since(start))
		return decision, fmt.Errorf("pending confirmation: %s", decision.Reason)
	}

	// Rule 1: existing path-allowlist for path-bearing tools.
	if path, ok := tc.Args["path"].(string); ok {
		if tc.Name == "run_check" {
			path = c.runCheckRulePath(path)
		}
		if err := c.validatePath(path); err != nil {
			decision := ToolCallDecision{
				Allow:  false,
				Reason: fmt.Sprintf("path-allowlist: %v", err),
			}
			c.completeAction(ctx, actionID, "denied_path",
				summarizeArgs(tc.Args), decision.Reason,
				time.Since(start))
			return decision, fmt.Errorf("tool call denied: %s", decision.Reason)
		}
	}

	accepted := summarizeArgs(tc.Args)
	if sandboxed {
		accepted = sandboxJustification + " " + accepted
	}
	c.completeAction(ctx, actionID, "accepted",
		accepted, "",
		time.Since(start))
	return ToolCallDecision{Allow: true}, nil
}

// untrustedRefSummary formats the user-source references in the chain
// for the error message. Caps to first 2 to keep error messages short.
func untrustedRefSummary(refs []Reference) string {
	var parts []string
	for _, r := range refs {
		if TrustedSourceLabels[r.SourceTrust] {
			continue
		}
		label := r.SourceLabel
		if label == "" {
			label = r.SourcePath
		}
		parts = append(parts, fmt.Sprintf("%s/%s [source: %s]",
			r.Category, r.Topic, label))
		if len(parts) >= 2 {
			break
		}
	}
	return strings.Join(parts, ", ")
}

// summarizeArgs produces a concise JSON-ish summary of args for the
// audit log. Avoids serializing huge args payloads — caps each
// string-valued arg at 200 chars.
func summarizeArgs(args map[string]any) string {
	clipped := make(map[string]any, len(args))
	for k, v := range args {
		if s, ok := v.(string); ok && len(s) > 200 {
			clipped[k] = s[:200] + "…[truncated]"
		} else {
			clipped[k] = v
		}
	}
	b, _ := json.Marshal(clipped)
	return string(b)
}
