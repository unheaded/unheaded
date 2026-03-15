// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
	"io"
)

func testLogger() *logger.Logger {
	return logger.New(io.Discard)
}

func TestNewAuditTrail(t *testing.T) {
	t.Run("positive max entries", func(t *testing.T) {
		at := NewAuditTrail(testLogger(), 100)
		if at == nil {
			t.Fatal("expected non-nil audit trail")
		}
		if at.maxEntries != 100 {
			t.Errorf("expected maxEntries 100, got %d", at.maxEntries)
		}
	})

	t.Run("zero max entries defaults to 10000", func(t *testing.T) {
		at := NewAuditTrail(testLogger(), 0)
		if at.maxEntries != 10000 {
			t.Errorf("expected maxEntries 10000, got %d", at.maxEntries)
		}
	})

	t.Run("negative max entries defaults to 10000", func(t *testing.T) {
		at := NewAuditTrail(testLogger(), -1)
		if at.maxEntries != 10000 {
			t.Errorf("expected maxEntries 10000, got %d", at.maxEntries)
		}
	})
}

func TestAuditTrailRecord(t *testing.T) {
	at := NewAuditTrail(testLogger(), 100)

	t.Run("record with timestamp", func(t *testing.T) {
		ts := time.Now().Add(-1 * time.Hour)
		at.Record(AuditEntry{
			Timestamp: ts,
			UserID:    "user-1",
			Action:    "create_experiment",
			RuleID:    "rule-1",
			Result:    "success",
		})

		if at.Len() != 1 {
			t.Errorf("expected 1 entry, got %d", at.Len())
		}
	})

	t.Run("record without timestamp sets current time", func(t *testing.T) {
		before := time.Now()
		at.Record(AuditEntry{
			Action: "start_experiment",
			Result: "success",
		})
		entries := at.Query(AuditFilter{Action: "start_experiment"})
		if len(entries) == 0 {
			t.Fatal("expected at least one entry")
		}
		if entries[0].Timestamp.Before(before) {
			t.Error("expected timestamp to be set to current time")
		}
	})
}

func TestAuditTrailRecordTrimming(t *testing.T) {
	at := NewAuditTrail(testLogger(), 10)

	for i := 0; i < 15; i++ {
		at.Record(AuditEntry{
			Action: "action",
			Result: "ok",
		})
	}

	// After trimming, should have fewer entries
	if at.Len() > 10 {
		t.Errorf("expected trimmed to max 10, got %d", at.Len())
	}
}

func TestAuditTrailRecordTrimMinimum(t *testing.T) {
	// With max 1, trim = max(1/4, 1) = 1
	at := NewAuditTrail(testLogger(), 1)

	at.Record(AuditEntry{Action: "a1", Result: "ok"})
	at.Record(AuditEntry{Action: "a2", Result: "ok"})

	if at.Len() > 1 {
		t.Errorf("expected trimmed to 1, got %d", at.Len())
	}
}

func TestAuditTrailQuery(t *testing.T) {
	at := NewAuditTrail(testLogger(), 100)

	now := time.Now()
	at.Record(AuditEntry{
		Timestamp: now.Add(-2 * time.Hour),
		UserID:    "alice",
		Action:    "create_experiment",
		Result:    "success",
	})
	at.Record(AuditEntry{
		Timestamp: now.Add(-1 * time.Hour),
		UserID:    "bob",
		Action:    "run_experiment",
		Result:    "success",
	})
	at.Record(AuditEntry{
		Timestamp: now,
		UserID:    "alice",
		Action:    "stop_experiment",
		Result:    "failed",
	})

	t.Run("empty filter returns all", func(t *testing.T) {
		results := at.Query(AuditFilter{})
		if len(results) != 3 {
			t.Errorf("expected 3, got %d", len(results))
		}
	})

	t.Run("filter by user", func(t *testing.T) {
		results := at.Query(AuditFilter{UserID: "alice"})
		if len(results) != 2 {
			t.Errorf("expected 2, got %d", len(results))
		}
	})

	t.Run("filter by action", func(t *testing.T) {
		results := at.Query(AuditFilter{Action: "run_experiment"})
		if len(results) != 1 {
			t.Errorf("expected 1, got %d", len(results))
		}
	})

	t.Run("filter by start time", func(t *testing.T) {
		start := now.Add(-90 * time.Minute)
		results := at.Query(AuditFilter{StartTime: &start})
		if len(results) != 2 {
			t.Errorf("expected 2, got %d", len(results))
		}
	})

	t.Run("filter by end time", func(t *testing.T) {
		end := now.Add(-30 * time.Minute)
		results := at.Query(AuditFilter{EndTime: &end})
		if len(results) != 2 {
			t.Errorf("expected 2, got %d", len(results))
		}
	})

	t.Run("filter by time range", func(t *testing.T) {
		start := now.Add(-90 * time.Minute)
		end := now.Add(-30 * time.Minute)
		results := at.Query(AuditFilter{StartTime: &start, EndTime: &end})
		if len(results) != 1 {
			t.Errorf("expected 1, got %d", len(results))
		}
	})

	t.Run("combined filters", func(t *testing.T) {
		results := at.Query(AuditFilter{UserID: "alice", Action: "stop_experiment"})
		if len(results) != 1 {
			t.Errorf("expected 1, got %d", len(results))
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		results := at.Query(AuditFilter{UserID: "nobody"})
		if len(results) != 0 {
			t.Errorf("expected 0, got %d", len(results))
		}
	})
}

func TestAuditTrailExport(t *testing.T) {
	at := NewAuditTrail(testLogger(), 100)

	at.Record(AuditEntry{Action: "test", Result: "ok"})
	at.Record(AuditEntry{Action: "test2", Result: "ok"})

	data := at.Export()
	if len(data) == 0 {
		t.Fatal("expected non-empty export")
	}

	var exported map[string]interface{}
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatalf("failed to unmarshal export: %v", err)
	}

	if exported["count"].(float64) != 2 {
		t.Errorf("expected count 2, got %v", exported["count"])
	}
	if exported["exported_at"] == nil {
		t.Error("expected exported_at to be set")
	}
}

func TestAuditTrailLen(t *testing.T) {
	at := NewAuditTrail(testLogger(), 100)

	if at.Len() != 0 {
		t.Errorf("expected 0, got %d", at.Len())
	}

	at.Record(AuditEntry{Action: "test", Result: "ok"})
	if at.Len() != 1 {
		t.Errorf("expected 1, got %d", at.Len())
	}
}

func TestAuditTrailClear(t *testing.T) {
	at := NewAuditTrail(testLogger(), 100)

	at.Record(AuditEntry{Action: "test", Result: "ok"})
	at.Record(AuditEntry{Action: "test2", Result: "ok"})

	at.Clear()
	if at.Len() != 0 {
		t.Errorf("expected 0 after clear, got %d", at.Len())
	}
}

func TestAuditTrailConcurrentAccess(t *testing.T) {
	at := NewAuditTrail(testLogger(), 1000)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			at.Record(AuditEntry{Action: "concurrent", Result: "ok"})
			_ = at.Query(AuditFilter{})
			_ = at.Len()
			_ = at.Export()
		}()
	}
	wg.Wait()

	if at.Len() != 50 {
		t.Errorf("expected 50, got %d", at.Len())
	}
}
