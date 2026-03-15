// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package logagg

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"unheaded/pkg/transport"
)

// mockSubConnection is a mock transport.Connection for subscriber tests.
type mockSubConnection struct {
	ch chan []byte
}

func (m *mockSubConnection) Type() transport.Type { return "mock" }

func (m *mockSubConnection) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (m *mockSubConnection) Subscribe(_ context.Context, _ string) (<-chan []byte, error) {
	return m.ch, nil
}

func (m *mockSubConnection) Close() error { return nil }

func (m *mockSubConnection) Healthy() bool { return true }

func TestSubscriber_ReceivesAndStores(t *testing.T) {
	ch := make(chan []byte, 10)
	conn := &mockSubConnection{ch: ch}
	buf := NewRingBuffer(100)
	sub := NewSubscriber(conn, buf)

	ctx, cancel := context.WithCancel(context.Background())

	// Send entries
	entry := LogEntry{
		Timestamp: time.Now(),
		Service:   "test-svc",
		Level:     "info",
		Message:   "hello from subscriber",
	}
	data, _ := json.Marshal(entry)
	ch <- data

	entry2 := LogEntry{
		Timestamp: time.Now(),
		Service:   "test-svc",
		Level:     "error",
		Message:   "something broke",
	}
	data2, _ := json.Marshal(entry2)
	ch <- data2

	// Start subscriber in background
	done := make(chan error, 1)
	go func() {
		done <- sub.Start(ctx)
	}()

	// Wait for entries to be processed
	time.Sleep(100 * time.Millisecond)

	if buf.Len() != 2 {
		t.Errorf("buffer.Len() = %d, want 2", buf.Len())
	}

	results := buf.Query(LogQuery{})
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Message != "hello from subscriber" {
		t.Errorf("first message = %q", results[0].Message)
	}

	cancel()
	<-done
}

func TestSubscriber_SkipsMalformed(t *testing.T) {
	ch := make(chan []byte, 10)
	conn := &mockSubConnection{ch: ch}
	buf := NewRingBuffer(100)
	sub := NewSubscriber(conn, buf)

	ctx, cancel := context.WithCancel(context.Background())

	// Send malformed data followed by valid data
	ch <- []byte("not json")
	entry := LogEntry{Timestamp: time.Now(), Service: "svc", Level: "info", Message: "valid"}
	data, _ := json.Marshal(entry)
	ch <- data

	done := make(chan error, 1)
	go func() {
		done <- sub.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	if buf.Len() != 1 {
		t.Errorf("buffer.Len() = %d, want 1 (malformed skipped)", buf.Len())
	}

	cancel()
	<-done
}

func TestSubscriber_StopsOnChannelClose(t *testing.T) {
	ch := make(chan []byte, 10)
	conn := &mockSubConnection{ch: ch}
	buf := NewRingBuffer(100)
	sub := NewSubscriber(conn, buf)

	done := make(chan error, 1)
	go func() {
		done <- sub.Start(context.Background())
	}()

	time.Sleep(50 * time.Millisecond)
	close(ch)

	select {
	case <-done:
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not stop after channel close")
	}
}

func TestSubscriber_RunningStatus(t *testing.T) {
	ch := make(chan []byte)
	conn := &mockSubConnection{ch: ch}
	buf := NewRingBuffer(100)
	sub := NewSubscriber(conn, buf)

	if sub.Running() {
		t.Error("should not be running before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- sub.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	if !sub.Running() {
		t.Error("should be running after Start")
	}

	cancel()
	<-done

	time.Sleep(50 * time.Millisecond)
	if sub.Running() {
		t.Error("should not be running after Stop")
	}
}
