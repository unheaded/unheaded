// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package storage

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"unheaded/pkg/audit"
)

// Integration: a throwaway unheaded_ops with db/migrations/022 applied, as
// ops_writer (scripts/well/test-compliance-schema.sh --go). Never The Well.
func TestIntegrationDatabaseStorage(t *testing.T) {
	dsn := os.Getenv("WELL_TEST_AUDIT_DSN")
	if dsn == "" {
		t.Skip("WELL_TEST_AUDIT_DSN not set")
	}
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ds, err := NewDatabaseStorage(db, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ds.Close() }()

	// A chain of events built the way producers build them.
	prev := ""
	mk := func(id string, typ audit.EventType, actor string, details map[string]interface{}) *audit.AuditEvent {
		ev := audit.NewEventBuilder().WithID(id).WithType(typ).WithAction("login").
			WithOutcome(audit.OutcomeSuccess).WithSeverity(audit.SeverityWarning).
			WithActor(&audit.Actor{ID: actor, Type: "user", Name: "A"}).Build()
		ev.Details = details
		ev.Source = &audit.Source{IP: "192.0.2.7", Service: "kanban", RequestID: "r-" + id}
		ev.PreviousHash = prev
		ev.Hash = ev.ComputeHash()
		prev = ev.Hash
		time.Sleep(time.Millisecond)
		return ev
	}
	e1 := mk("ev-1", audit.EventTypeAuthentication, "alice", nil)
	e2 := mk("ev-2", audit.EventTypeAuthorization, "bob", map[string]interface{}{"n": 1, "tags": []interface{}{"a", "b"}})
	e3 := mk("ev-3", audit.EventTypeSystem, "alice", map[string]interface{}{})
	if err := ds.Store(ctx, e1); err != nil {
		t.Fatal(err)
	}
	if err := ds.StoreBatch(ctx, []*audit.AuditEvent{e2, e3}); err != nil {
		t.Fatal(err)
	}

	for _, want := range []*audit.AuditEvent{e1, e2, e3} {
		got, err := ds.Get(ctx, want.ID)
		if err != nil {
			t.Fatalf("get %s: %v", want.ID, err)
		}
		if !got.VerifyHash() || got.Hash != want.Hash || got.PreviousHash != want.PreviousHash {
			t.Errorf("%s: hash does not verify after the round trip (timestamp %v vs %v)", want.ID, got.Timestamp, want.Timestamp)
		}
		if got.Type != want.Type || got.Severity != want.Severity || got.Actor.ID != want.Actor.ID || got.Source.RequestID != want.Source.RequestID {
			t.Errorf("%s: fields changed: %+v", want.ID, got)
		}
	}
	last, err := ds.GetLastHash(ctx)
	if err != nil || last != e3.Hash {
		t.Errorf("last hash %q (%v), want e3's", last, err)
	}
	got, err := ds.Query(ctx, &audit.AuditQuery{ActorIDs: []string{"alice"}, OrderBy: "timestamp", Descending: true, Limit: 1})
	if err != nil || len(got) != 1 || got[0].ID != "ev-3" {
		t.Errorf("query newest alice: %v %v", got, err)
	}
	n, err := ds.Count(ctx, &audit.AuditQuery{Types: []audit.EventType{audit.EventTypeAuthentication, audit.EventTypeAuthorization}})
	if err != nil || n != 2 {
		t.Errorf("count: %d %v", n, err)
	}
	// The table is append-only for the writer.
	if _, err := db.ExecContext(ctx, `UPDATE audit_events SET action = 'x'`); err == nil {
		t.Error("ops_writer updated an audit event")
	}
	// The taxonomy is enforced: 004's old types are refused.
	bad := mk("ev-4", "auth.login", "carol", nil)
	if err := ds.Store(ctx, bad); err == nil {
		t.Error("an old-taxonomy event type was stored")
	}
}
