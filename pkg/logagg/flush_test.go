// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/logger"
)

func loggerEntry(level, msg string) *logger.Entry {
	l, err := logger.ParseLevel(level)
	if err != nil {
		panic(err)
	}
	return &logger.Entry{Level: l, Message: msg}
}

// workerless returns an enabled publisher whose worker never runs: the
// state of a process that is about to exit, where nothing queued will be
// drained unless the caller's own goroutine does it.
func workerless(conn *mockConnection, depth int) *Publisher {
	p := &Publisher{
		serviceName: "svc",
		conn:        conn,
		queue:       make(chan LogEntry, depth),
		done:        make(chan struct{}),
	}
	p.enabled.Store(true)
	return p
}

func (m *mockConnection) messages(t *testing.T) []string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, pm := range m.published {
		var e LogEntry
		if err := json.Unmarshal(pm.data, &e); err != nil {
			t.Fatalf("payload not JSON: %v", err)
		}
		out = append(out, e.Level+":"+e.Message)
	}
	return out
}

// zerolog (and pkg/logger) call os.Exit right after the hooks for a Fatal,
// so an entry left on the queue for the worker was never sent: the one log
// line that says why a service died never reached Wotan. A fatal or panic
// entry is now published before Run returns, after what was queued ahead
// of it.
func TestPublisher_FatalIsPublishedBeforeRunReturns(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := workerless(conn, 8)

	p.Run(nil, zerolog.InfoLevel, "starting")
	p.Run(nil, zerolog.FatalLevel, "cannot bind")

	got := conn.messages(t)
	want := []string{"info:starting", "fatal:cannot bind"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("published %v before Run returned, want %v", got, want)
	}
}

// The same holds through pkg/logger's hook, which several services use.
func TestPublisher_LoggerHookPanicLevelIsSynchronous(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := workerless(conn, 8)

	h := p.LoggerHook()
	_ = h.Run(loggerEntry("panic", "invariant broken"))

	if got := conn.messages(t); len(got) != 1 || got[0] != "panic:invariant broken" {
		t.Fatalf("published %v, want [panic:invariant broken]", got)
	}
}

// A transport that never returns must not keep a dying process alive: the
// flush gives up after its deadline.
func TestPublisher_FatalFlushIsBounded(t *testing.T) {
	conn := &blockingConnection{release: make(chan struct{})}
	defer close(conn.release)
	p := NewPublisher("svc", conn)
	defer p.Close()

	start := time.Now()
	p.Run(nil, zerolog.FatalLevel, "x")
	if took := time.Since(start); took > 4*time.Second {
		t.Fatalf("Run(fatal) took %v against a hung transport", took)
	}
}

// Close used to discard whatever was still queued, uncounted: a graceful
// shutdown lost its last lines ("shutting down", the error that caused it)
// and every counter still read zero. Close now sends them, and counts as
// dropped any it could not.
func TestPublisher_CloseDrainsQueue(t *testing.T) {
	conn := &mockConnection{healthy: true}
	p := workerless(conn, 8)
	p.Run(nil, zerolog.InfoLevel, "one")
	p.Run(nil, zerolog.WarnLevel, "two")

	p.Close()

	got := conn.messages(t)
	if len(got) != 2 || got[0] != "info:one" || got[1] != "warn:two" {
		t.Fatalf("published %v on Close, want [info:one warn:two]", got)
	}
	if p.Dropped() != 0 {
		t.Errorf("Dropped = %d, want 0", p.Dropped())
	}
}

func TestPublisher_CloseCountsWhatItCouldNotSend(t *testing.T) {
	conn := &blockingConnection{release: make(chan struct{})}
	defer close(conn.release)
	p := &Publisher{
		serviceName: "svc",
		conn:        conn,
		queue:       make(chan LogEntry, 8),
		done:        make(chan struct{}),
	}
	p.enabled.Store(true)
	for i := 0; i < 3; i++ {
		p.Run(nil, zerolog.InfoLevel, "stuck")
	}

	p.Close()

	// One entry is in flight in the abandoned publish; the other two never
	// left the queue. Neither kind arrived, and both must be visible.
	if lost := p.Dropped() + p.Failed(); lost < 2 {
		t.Fatalf("dropped %d + failed %d after Close; 3 entries never arrived", p.Dropped(), p.Failed())
	}
}

// panickingConnection panics in Publish, as a transport with a nil
// client inside would.
type panickingConnection struct{ mockConnection }

func (*panickingConnection) Publish(context.Context, string, []byte) error {
	panic("transport bug")
}

// The worker recovers a panicking transport so logging cannot crash the
// service, but the entry it was sending is lost and used to vanish from
// every counter.
func TestPublisher_TransportPanicCountsAsFailed(t *testing.T) {
	p := NewPublisher("svc", &panickingConnection{})
	defer p.Close()

	p.Run(nil, zerolog.ErrorLevel, "x")
	waitFor(t, func() bool { return p.Failed() == 1 })
}
