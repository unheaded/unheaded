// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"errors"
	"testing"
)

// A timeline.updates message from timeguru is a notification with no content.
// The Meta Moment only works if receiving one makes the board go and fetch
// the timeline; until 2026-09-21 it did not.
func TestHandleTimelineUpdate_NotificationTriggersRefetch(t *testing.T) {
	tm := NewTimelineManager(nil)
	calls := 0
	tm.SetRefetch(func() error { calls++; return nil })

	payload := []byte(`{"event":"timeline_loaded","timestamp":"2026-09-21T00:00:00Z","source":"timeguru"}`)
	if err := tm.HandleTimelineUpdate(payload); err != nil {
		t.Fatalf("HandleTimelineUpdate: %v", err)
	}
	if calls != 1 {
		t.Fatalf("notification must trigger exactly one refetch, got %d", calls)
	}

	// A refetch failure is logged, not returned — the notification itself was
	// handled, and the next one retries.
	tm.SetRefetch(func() error { calls++; return errors.New("timeguru down") })
	if err := tm.HandleTimelineUpdate(payload); err != nil {
		t.Fatalf("refetch failure must not fail the handler: %v", err)
	}
	if calls != 2 {
		t.Fatalf("second notification must refetch again, got %d calls", calls)
	}

	// A full-timeline payload is applied directly and does not refetch.
	if err := tm.HandleTimelineUpdate([]byte(`{"phases":[]}`)); err != nil {
		t.Fatalf("full timeline payload: %v", err)
	}
	if calls != 2 {
		t.Fatalf("full timeline payload must not trigger refetch, got %d calls", calls)
	}
}
