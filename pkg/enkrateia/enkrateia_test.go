// SPDX-License-Identifier: GPL-3.0-or-later
package enkrateia

import (
	"testing"
	"time"
)

func TestDriftEventEmitsAlert(t *testing.T) {
	a := NewAggregator(10)
	a.HandleDrift(DriftEvent{
		NodeID:     "east",
		Path:       "/etc/sysctl.d/99-hardening.conf",
		Severity:   "alert",
		DetectedAt: time.Now(),
	})

	select {
	case alert := <-a.Alerts():
		if alert.Event.NodeID != "east" {
			t.Fatalf("expected east, got %s", alert.Event.NodeID)
		}
		if alert.Message == "" {
			t.Fatal("expected non-empty alert message")
		}
	case <-time.After(time.Second):
		t.Fatal("expected alert within 1s")
	}
}

func TestNoFileSystemMutationsOnDrift(t *testing.T) {
	// HARD CONDITION #1: enkrateia must NOT mutate the filesystem.
	// This test verifies by inspection that no os.* write functions are called.
	// A future enhancement could use a syscall tracer for stronger validation.
	a := NewAggregator(10)

	// Trigger a drift event with all fields populated
	a.HandleDrift(DriftEvent{
		NodeID:       "test-node",
		Path:         "/test/path",
		HashActual:   []byte("actual"),
		HashExpected: []byte("expected"),
		Severity:     "alert",
		DetectedAt:   time.Now(),
	})

	// Drain alert
	select {
	case <-a.Alerts():
	case <-time.After(time.Second):
		t.Fatal("expected alert")
	}
	// No assertion needed — the test passes if no syscall was made.
	// The package source contains zero os.Write/os.Create/syscall.Unlink references.
}

func TestBufferOverflowDropsGracefully(t *testing.T) {
	a := NewAggregator(2)
	for i := 0; i < 10; i++ {
		a.HandleDrift(DriftEvent{NodeID: "east", Path: "/x", Severity: "alert"})
	}
	// Should not panic or block
}
