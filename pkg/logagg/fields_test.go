// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"encoding/json"
	"io"
	"testing"

	"unheaded/pkg/logger"
)

// pkg/logger hands hooks the whole JSON line (Entry.Raw); the hook used to
// forward only the message, so every structured field — the error, the
// request path, the container name — was missing from the aggregated log,
// and a search for it found nothing.
func TestPublisher_LoggerHookForwardsFields(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := NewPublisher("svc", conn)
	defer p.Close()

	l := logger.New(io.Discard).AddHook(p.LoggerHook())
	l.Warn().Str("container", "kanban").Int("attempt", 3).Msg("restart failed")
	waitFor(t, func() bool { return conn.count() == 1 })

	conn.mu.Lock()
	data := conn.published[0].data
	conn.mu.Unlock()
	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if entry.Message != "restart failed" || entry.Level != "warn" {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Fields["container"] != "kanban" {
		t.Errorf("container field = %v, want kanban (fields %v)", entry.Fields["container"], entry.Fields)
	}
	if entry.Fields["attempt"] != float64(3) {
		t.Errorf("attempt field = %v, want 3 (fields %v)", entry.Fields["attempt"], entry.Fields)
	}
	// Level, message and time already have their own LogEntry fields.
	for _, dup := range []string{"level", "message", "time"} {
		if _, ok := entry.Fields[dup]; ok {
			t.Errorf("field %q duplicated into Fields: %v", dup, entry.Fields)
		}
	}
}

// A line that is not JSON (or no line at all) still forwards the message.
func TestPublisher_LoggerHookWithoutRaw(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := NewPublisher("svc", conn)
	defer p.Close()

	e := loggerEntry("info", "plain")
	e.Raw = []byte("not json")
	_ = p.LoggerHook().Run(e)
	waitFor(t, func() bool { return conn.count() == 1 })
	if got := conn.messages(t); got[0] != "info:plain" {
		t.Fatalf("published %v", got)
	}
}
