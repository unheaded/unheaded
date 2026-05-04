// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package champion

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
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
	// "patch_file", "kanban_create", "kanban_update", "kanban_delete",
	// "kanban_list", "runbook_show", "runbook_execute", "corpus_search",
	// "service_health", "system_command".
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
}

// Reference mirrors the cs/vor B1-schema source-provenance fields.
// Populated by the agent layer from vor's /api/topics or /api/search
// JSON response.
type Reference struct {
	Topic       string `json:"topic"`
	Category    string `json:"category"`
	SourceKind  string `json:"source_kind"`        // "embedded" | "user-custom" | "user-source"
	SourceTrust string `json:"source_trust"`       // "canonical" | "local" | "external"
	SourcePath  string `json:"source_path,omitempty"`
	SourceLabel string `json:"source_label,omitempty"`
	Excerpt     string `json:"excerpt,omitempty"`  // 200-char snippet for audit
}

// MutatingTools is the canonical list of tool names that modify state.
// Anything not in this list is treated as read-only by IsMutating.
//
// CONSERVATIVE: an unknown tool name is treated as MUTATING (fail-closed).
// New tools must register here explicitly.
var MutatingTools = map[string]bool{
	"write_file":      true,
	"patch_file":      true,
	"delete_file":     true,
	"kanban_create":   true,
	"kanban_update":   true,
	"kanban_delete":   true,
	"runbook_execute": true,
	"system_command":  true,
	"model_switch":    true, // ADR-060: kills+respawns llama-server subprocess
}

// ReadOnlyTools is the canonical list of read-only tool names. Used to
// distinguish "known safe" from "unknown" in IsMutating's fail-closed
// default.
var ReadOnlyTools = map[string]bool{
	"read_file":      true,
	"kanban_list":    true,
	"runbook_show":   true,
	"runbook_list":   true,
	"corpus_search":  true,
	"service_health": true,
}

// destructivePattern matches any of the destructive shell-verb forms
// that, when appearing in any string-valued argument, trigger Rule 3
// (destructive-verb refusal) regardless of source-trust. Mirrors the
// list applied at the LLM layer in cmd/zhen-rag/main.go's system prompt.
//
// Word boundaries (\b) ensure "rebootstrap" doesn't match "reboot" and
// "format-string" doesn't match "format". (?i) for case-insensitive.
var destructivePattern = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`rm\s+-[rf]+\b`,                  // rm -rf, rm -f, rm -r
	`rm\s+/`,                         // rm / (rooted)
	`\bdrop\s+table\b`,
	`\bdrop\s+database\b`,
	`\bmkfs(\.[a-z0-9]+)?\b`,         // mkfs, mkfs.ext4, mkfs.xfs, ...
	`\bdd\s+(if|of)=`,
	`\bwipe\b`,
	`\bshutdown\b`,
	`\breboot\b`,
	`\bkill\s+-9\b`,
	`\btruncate\b`,
	`\bunlink\b`,
	`git\s+push\s+--force\b`,
	`git\s+reset\s+--hard\b`,
	`\bchmod\s+(-R\s+)?000\b`,
	`>\s*/dev/sd[a-z]\b`,             // > /dev/sda, /dev/sdb, ...
}, "|"))

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

// HasUntrustedJustification reports whether the justification chain
// is untrusted from the gate's perspective. Returns true if either:
//
//   - Any reference has source_trust == "external" (user-symlinked
//     source under ~/.config/cs/sources/ — possibly poisoned), OR
//
//   - The chain is EMPTY and the tool is mutating. Empty justification
//     on a mutating tool is treated as untrusted by default
//     (fail-closed) because:
//
//     1. An attacker can craft a prompt that doesn't match the seed-
//        retrieval pattern (refs ends up empty) AND inlines a poisoned-
//        content reference textually in the user prompt. The model is
//        influenced by the inline content; the gate sees no refs.
//        Without this check the gate is bypassable in that case.
//        See eval/coding-gate/probe-2026-05-02/A2-agent-adversarial.md.
//
//     2. A model that emits a tool call without ANY justification chain
//        has no demonstrable grounding for the call — caller should
//        either supply a justification or explicitly opt into
//        DirectTrusted by populating Justification with a single
//        canonical reference (e.g., {SourceTrust: "direct-user"}).
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
		if r.SourceTrust == "external" {
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
//   for tool calls that include a `path` arg).
// Rule 2: mutating + untrusted justification → refuse pending
//   out-of-band user confirmation. PendingConfirmation=true.
// Rule 3: any string arg contains a destructive shell verb → refuse
//   absolutely (defense-in-depth even when justification is trusted).
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

	// Rule 2: mutating + untrusted justification.
	if tc.IsMutating() && tc.HasUntrustedJustification() {
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

	c.completeAction(ctx, actionID, "accepted",
		summarizeArgs(tc.Args), "",
		time.Since(start))
	return ToolCallDecision{Allow: true}, nil
}

// untrustedRefSummary formats the user-source references in the chain
// for the error message. Caps to first 2 to keep error messages short.
func untrustedRefSummary(refs []Reference) string {
	var parts []string
	for _, r := range refs {
		if r.SourceTrust != "external" {
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
