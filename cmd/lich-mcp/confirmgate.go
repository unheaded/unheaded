// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"unheaded/pkg/claudebridge"
	"unheaded/pkg/lichtools"
	"unheaded/pkg/zhentools"
)

// confirmGate wires the out-of-band confirmation flow (S-LICH-AWAKENS Phase 4,
// Steps 103-108) around the Champion gate. It is used both by the MCP server
// (a local model or Claude Code drives the tools over stdio) and by the Phase 6
// in-process Claude driver, so the confirmation semantics cannot drift between
// the two.
//
// The Champion gate refuses lich_file_finding pending-confirmation: a model's
// call carries no justification (ADR-104 Rule 2). confirmGate turns that refusal
// into a durable, hash-bound ticket on disk:
//
//   - If an operator has already approved a ticket for this exact call
//     (same CallHash over {tool, project root, canonical args}), the call is
//     redeemed: the provider runs once, the ticket is consumed, no replay.
//   - Otherwise a pending ticket is parked and the refusal names its id. The
//     operator approves it out of band with `lich-mcp confirm approve <id>`,
//     then the driver re-issues the identical call and it proceeds.
//
// The approval code never travels through the model — only the filesystem the
// operator owns. The redeemed call bypasses only Rule 2 (the human approved the
// mutating write); it is restricted to the known-mutating lich tool whose only
// write target is the server-fixed register path, so no destructive verb or
// out-of-tree path can ride the approval.
type confirmGate struct {
	inner       claudebridge.Gate
	store       *lichtools.PendingStore
	projectRoot string
	log         func(event, id, tool string)
}

func newConfirmGate(inner claudebridge.Gate, store *lichtools.PendingStore, projectRoot string, log func(event, id, tool string)) *confirmGate {
	if log == nil {
		log = func(string, string, string) {}
	}
	return &confirmGate{inner: inner, store: store, projectRoot: projectRoot, log: log}
}

// confirmableMutating reports whether a refused-pending tool participates in the
// out-of-band flow. Only lich_file_finding does: it is the sole mutating Lich
// tool, and its write target is server-fixed (the ADR-098 register).
func confirmableMutating(name string) bool { return name == lichtools.ToolFileFinding }

// Execute implements claudebridge.Gate.
func (g *confirmGate) Execute(ctx context.Context, call claudebridge.ToolCall, p zhentools.Provider) (zhentools.Result, error) {
	res, err := g.inner.Execute(ctx, call, p)

	var ref *claudebridge.RefusedError
	if !errors.As(err, &ref) || !ref.PendingConfirmation || !confirmableMutating(call.Name) {
		return res, err
	}

	callHash := lichtools.CallHash(call.Name, g.projectRoot, call.Input)

	// Redeem an existing operator approval for this exact call.
	if tk, ok, ferr := g.store.FindApproved(callHash); ferr == nil && ok {
		if uerr := g.store.MarkUsed(tk.ID); uerr != nil {
			return zhentools.Result{}, fmt.Errorf("lich confirm: consume ticket %s: %w", tk.ID, uerr)
		}
		g.log("confirm_redeem", tk.ID, call.Name)
		// The human approved: run the provider directly, past Rule 2 only. Scope
		// was validated before the gate; the provider path-validates its own
		// write; nothing destructive is reachable here.
		return p.Call(ctx, call.Name, call.Input)
	}

	// No approval yet: park (or reuse) a pending ticket and tell the driver.
	tk, perr := g.store.Park(call.Name, findingSummary(call.Input), call.Input, g.projectRoot)
	if perr != nil {
		// Parking failed: fall back to the plain refusal, never to running it.
		return res, err
	}
	g.log("confirm_park", tk.ID, call.Name)
	return zhentools.Result{}, &claudebridge.RefusedError{
		Reason: fmt.Sprintf("%s [parked as confirmation ticket %s; an operator must approve it out of band: lich-mcp confirm approve %s]",
			ref.Reason, tk.ID, tk.ID),
		PendingConfirmation: true,
	}
}

// findingSummary builds a short, operator-facing one-liner from a file_finding
// call's arguments for the ticket. It never trusts the model's text as code and
// the store sanitises control characters.
func findingSummary(input json.RawMessage) string {
	var in struct {
		Severity string `json:"severity"`
		Summary  string `json:"summary"`
		Class    string `json:"class"`
	}
	_ = json.Unmarshal(input, &in)
	s := in.Severity
	if in.Class != "" {
		s += "/" + in.Class
	}
	if in.Summary != "" {
		s += ": " + in.Summary
	}
	if s == "" {
		s = "file_finding (no summary)"
	}
	return s
}

// compile-time assertion that confirmGate is a claudebridge.Gate.
var _ claudebridge.Gate = (*confirmGate)(nil)
