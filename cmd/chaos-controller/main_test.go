// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// newTestController constructs a ChaosController with a discarding logger
// and nil Wotan (the controller's `if wotan != nil` checks make Wotan
// optional, which is what enables unit-testing without infrastructure).
func newTestController(t *testing.T) *ChaosController {
	t.Helper()
	return NewChaosController(logger.New(io.Discard), nil)
}

// ─── InjectRule ──────────────────────────────────────────────────────────────

func TestInjectRule_HappyPath(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	rule := &ChaosFaultRule{
		FaultType:    FaultDrop,
		SrcServiceID: 1,
		DstServiceID: 2,
		DropRatePPM:  10000,
		Duration:     5 * time.Second,
		CreatedBy:    "test",
	}
	if err := cc.InjectRule(rule); err != nil {
		t.Fatalf("InjectRule: %v", err)
	}
	if rule.ID == "" {
		t.Errorf("InjectRule should populate ID")
	}
	if !strings.HasPrefix(rule.ID, "chaos-") {
		t.Errorf("ID should start with chaos-, got %q", rule.ID)
	}
	if !rule.Active {
		t.Errorf("rule should be Active after injection")
	}
	if rule.CreatedAt.IsZero() {
		t.Errorf("CreatedAt should be set")
	}
	if !rule.ExpiresAt.After(rule.CreatedAt) {
		t.Errorf("ExpiresAt should be after CreatedAt")
	}
}

func TestInjectRule_RejectsEmptyFaultType(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	err := cc.InjectRule(&ChaosFaultRule{Duration: time.Second})
	if err == nil {
		t.Fatal("expected error for empty fault_type")
	}
	if !strings.Contains(err.Error(), "fault_type is required") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestInjectRule_RejectsUnknownFaultType(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	err := cc.InjectRule(&ChaosFaultRule{
		FaultType: "BANANA",
		Duration:  time.Second,
	})
	if err == nil {
		t.Fatal("expected error for unknown fault_type")
	}
	if !strings.Contains(err.Error(), "unknown fault_type") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestInjectRule_RejectsZeroOrNegativeDuration(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	for _, d := range []time.Duration{0, -1 * time.Second} {
		err := cc.InjectRule(&ChaosFaultRule{
			FaultType: FaultDelay,
			Duration:  d,
		})
		if err == nil {
			t.Errorf("duration %v: expected error, got nil", d)
		}
	}
}

func TestInjectRule_AcceptsAllFourFaultTypes(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	for _, ft := range []FaultType{FaultDrop, FaultDelay, FaultCorrupt, FaultDuplicate} {
		err := cc.InjectRule(&ChaosFaultRule{
			FaultType: ft,
			Duration:  time.Second,
		})
		if err != nil {
			t.Errorf("FaultType %s should be accepted, got error: %v", ft, err)
		}
	}
}

// ─── RemoveRule ──────────────────────────────────────────────────────────────

func TestRemoveRule_RemovesExistingRule(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	rule := &ChaosFaultRule{FaultType: FaultDrop, Duration: time.Second}
	if err := cc.InjectRule(rule); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if err := cc.RemoveRule(rule.ID); err != nil {
		t.Fatalf("RemoveRule: %v", err)
	}
	if rule.Active {
		t.Errorf("rule should be marked Inactive after removal")
	}
	if got := cc.ListRules(); len(got) != 0 {
		t.Errorf("ListRules after removal: got %d, want 0", len(got))
	}
}

func TestRemoveRule_UnknownIDErrors(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	err := cc.RemoveRule("chaos-does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown rule")
	}
	if !strings.Contains(err.Error(), "rule not found") {
		t.Errorf("wrong error: %v", err)
	}
}

// ─── ListRules / RecentEvents ────────────────────────────────────────────────

func TestListRules_EmptyByDefault(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	if got := cc.ListRules(); len(got) != 0 {
		t.Errorf("fresh controller ListRules = %d, want 0", len(got))
	}
}

func TestListRules_AfterMultipleInjections(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	for i := 0; i < 3; i++ {
		err := cc.InjectRule(&ChaosFaultRule{FaultType: FaultDrop, Duration: time.Second})
		if err != nil {
			t.Fatalf("inject %d: %v", i, err)
		}
	}
	if got := len(cc.ListRules()); got != 3 {
		t.Errorf("ListRules after 3 injects = %d, want 3", got)
	}
}

func TestRecentEvents_TracksInjectionsAndRemovals(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	rule := &ChaosFaultRule{FaultType: FaultDelay, Duration: time.Second}
	if err := cc.InjectRule(rule); err != nil {
		t.Fatal(err)
	}
	if err := cc.RemoveRule(rule.ID); err != nil {
		t.Fatal(err)
	}
	events := cc.RecentEvents(10)
	if len(events) < 2 {
		t.Fatalf("expected >= 2 events (inject + remove), got %d", len(events))
	}
	// Verify both an "injected" and a "removed" event appear.
	var injected, removed bool
	for _, ev := range events {
		if ev.Action == "injected" && ev.RuleID == rule.ID {
			injected = true
		}
		if ev.Action == "removed" && ev.RuleID == rule.ID {
			removed = true
		}
	}
	if !injected || !removed {
		t.Errorf("missing inject/remove events: injected=%v removed=%v", injected, removed)
	}
}

func TestRecentEvents_LimitRespected(t *testing.T) {
	t.Parallel()
	cc := newTestController(t)
	for i := 0; i < 5; i++ {
		err := cc.InjectRule(&ChaosFaultRule{FaultType: FaultDrop, Duration: time.Second})
		if err != nil {
			t.Fatal(err)
		}
	}
	got := cc.RecentEvents(2)
	if len(got) > 2 {
		t.Errorf("RecentEvents(2) returned %d, expected ≤2", len(got))
	}
}
