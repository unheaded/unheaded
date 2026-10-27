// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/audit/storage"
	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	wotanClient "unheaded/pkg/wotan-client"
)

// The local brain is audited like Claude (S-TWO-BRAINS Step 56): one
// unheaded_ops.llm_requests row per llama-server call (backend local, cost
// 0, tokens from the OpenAI-compatible usage block) and one audit_events
// row per Champion gate decision. zhen_actions rows stay as they are.

type runIDKey struct{}

// withRunID gives one agent run an id that its llama-server calls share
// as their conversation_id.
func withRunID(ctx context.Context) context.Context {
	return context.WithValue(ctx, runIDKey{}, claudebridge.NewRecordID())
}

func runIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(runIDKey{}).(string); ok && v != "" {
		return v
	}
	return claudebridge.NewRecordID()
}

// agentdAudit is the auditor chain and what must be closed with it.
type agentdAudit struct {
	auditor claudebridge.Auditor // nil: audit off
	well    *claudebridge.WellAuditor
	wotan   *claudebridge.WotanAuditor
	closers []func()
}

func (a *agentdAudit) close() {
	if a == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if a.well != nil {
		_ = a.well.Close(ctx)
	}
	if a.wotan != nil {
		_ = a.wotan.Close(ctx)
	}
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
}

// buildAgentdAudit wires the sinks the flags ask for. The DSN and Wotan
// address come from the environment only; a flag without its variable is
// a startup error. A Well that cannot be reached is a loud warning: the
// zhen_actions trail (action-store) still records every gate decision.
func buildAgentdAudit(ctx context.Context, wantWell, wantWotan bool) (*agentdAudit, error) {
	a := &agentdAudit{}
	var chain claudebridge.MultiAuditor
	if wantWell {
		dsn := strings.TrimSpace(os.Getenv("WELL_OPS_DSN"))
		if dsn == "" {
			return nil, fmt.Errorf("-audit-well needs WELL_OPS_DSN (unheaded_ops as ops_writer)")
		}
		w, err := agentdWellAuditor(ctx, dsn, a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "zhen-agentd: WARNING — -audit-well: the Well is unreachable (%v); llm_requests and audit_events are not recorded\n", err)
		} else {
			a.well = w
			chain = append(chain, w)
		}
	}
	if wantWotan {
		addr := strings.TrimSpace(os.Getenv("WOTAN_ADDR"))
		if addr == "" {
			return nil, fmt.Errorf("-audit-wotan needs WOTAN_ADDR (Wotan's HTTP host:port)")
		}
		wotanClient.InitServiceIdentity("zhen-agentd") // ADR-103 §6: Wotan calls carry zhen-agentd's service token
		c, err := wotanClient.NewClient(addr)
		if err != nil {
			return nil, fmt.Errorf("-audit-wotan: %w", err)
		}
		a.closers = append(a.closers, func() { _ = c.Close() })
		wa, err := claudebridge.NewWotanAuditor(claudebridge.WotanAuditorConfig{
			Pub: claudebridge.WotanClientPublisher{C: c}, Name: "zhen-agentd", Backend: claudebridge.BackendLocal,
		})
		if err != nil {
			return nil, err
		}
		a.wotan = wa
		chain = append(chain, wa)
	}
	if len(chain) > 0 {
		a.auditor = chain
	}
	return a, nil
}

func agentdWellAuditor(ctx context.Context, dsn string, a *agentdAudit) (*claudebridge.WellAuditor, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	a.closers = append(a.closers, func() { _ = db.Close() })
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		return nil, err
	}
	events, err := storage.NewDatabaseStorage(db, "")
	if err != nil {
		return nil, err
	}
	return claudebridge.NewWellAuditor(claudebridge.WellAuditorConfig{
		Source: "zhen-agentd", Backend: claudebridge.BackendLocal,
		Requests: claudebridge.SQLRequestSink{DB: db}, Events: events,
	})
}

// auditActionStore turns each Champion gate decision (a tool_call_attempt
// action and its completion) into one audit event, then passes the call
// on to the real store.
type auditActionStore struct {
	inner   champion.ActionStore
	auditor claudebridge.Auditor

	mu      sync.Mutex
	pending map[int64]champion.Action
}

func newAuditActionStore(inner champion.ActionStore, a claudebridge.Auditor) champion.ActionStore {
	if a == nil {
		return inner
	}
	return &auditActionStore{inner: inner, auditor: a, pending: map[int64]champion.Action{}}
}

const maxPendingGateActions = 4096

func (s *auditActionStore) LogAction(ctx context.Context, action *champion.Action) (int64, error) {
	id, err := s.inner.LogAction(ctx, action)
	if err == nil && id > 0 && action != nil && action.ActionType == "tool_call_attempt" {
		s.mu.Lock()
		if len(s.pending) < maxPendingGateActions {
			s.pending[id] = *action
		}
		s.mu.Unlock()
	}
	return id, err
}

func (s *auditActionStore) UpdateAction(ctx context.Context, id int64, status, result, errMsg string, elapsedMs int) error {
	err := s.inner.UpdateAction(ctx, id, status, result, errMsg, elapsedMs)
	s.mu.Lock()
	act, ok := s.pending[id]
	delete(s.pending, id)
	s.mu.Unlock()
	if ok {
		s.auditor.ToolCall(ctx, gateRecord(act, status, result, errMsg, elapsedMs))
	}
	return err
}

func (s *auditActionStore) GetActions(ctx context.Context, sessionID string, limit int) ([]champion.Action, error) {
	return s.inner.GetActions(ctx, sessionID, limit)
}

// gateRecord maps a completed tool_call_attempt to a ToolRecord. The
// argument summary is hashed, never recorded.
func gateRecord(act champion.Action, status, result, errMsg string, elapsedMs int) claudebridge.ToolRecord {
	name := strings.TrimSpace(strings.TrimPrefix(act.Intent, "ToolCall:"))
	r := claudebridge.ToolRecord{
		Conversation: act.SessionID,
		Name:         name,
		Provider:     "champion",
		Reason:       errMsg,
		Model:        act.TriggeredBy,
		Backend:      claudebridge.BackendLocal,
		InputBytes:   len(result),
		InputSHA256:  claudebridge.SHA256Hex([]byte(result)),
		Duration:     time.Duration(elapsedMs) * time.Millisecond,
	}
	switch status {
	case "accepted":
		r.Decision = "allowed"
	case "denied_untrusted_justification":
		r.Decision, r.Pending, r.IsError = "refused", true, true
	default:
		r.Decision, r.IsError = "refused", true
	}
	return r
}

// buildConfirmQueue opens the Well's confirmation queue as app_zhen when
// -confirm-queue is set. Unlike the audit sink this is not optional once
// asked for: a queue that cannot be reached would silently turn queued
// writes into denials, so startup fails instead.
func buildConfirmQueue(ctx context.Context, want bool) (claudebridge.ConfirmQueue, func(), error) {
	if !want {
		return nil, func() {}, nil
	}
	dsn := strings.TrimSpace(os.Getenv("WELL_APP_ZHEN_DSN"))
	if dsn == "" {
		return nil, func() {}, fmt.Errorf("-confirm-queue needs WELL_APP_ZHEN_DSN (unheaded_app as app_zhen)")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, func() {}, err
	}
	db.SetMaxOpenConns(2)
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		_ = db.Close()
		return nil, func() {}, fmt.Errorf("the Well is unreachable: %w", err)
	}
	return claudebridge.SQLConfirmQueue{DB: db}, func() { _ = db.Close() }, nil
}
