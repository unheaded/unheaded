package logs

import (
	"testing"
	"time"

	"unheaded/pkg/logagg"
)

func TestNewLogStream(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)
	if ls.ListenerCount() != 0 {
		t.Errorf("initial listener count = %d, want 0", ls.ListenerCount())
	}
}

func TestLogStream_Broadcast(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	ch := make(chan logagg.LogEntry, 10)
	ls.mu.Lock()
	ls.listeners[ch] = logStreamFilter{}
	ls.mu.Unlock()

	entry := logagg.LogEntry{
		Timestamp: time.Now(),
		Service:   "test",
		Level:     "info",
		Message:   "hello",
	}
	ls.Broadcast(entry)

	select {
	case received := <-ch:
		if received.Message != "hello" {
			t.Errorf("message = %q, want %q", received.Message, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestLogStream_BroadcastWithServiceFilter(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	ch := make(chan logagg.LogEntry, 10)
	ls.mu.Lock()
	ls.listeners[ch] = logStreamFilter{service: "captain"}
	ls.mu.Unlock()

	// Should not match
	ls.Broadcast(logagg.LogEntry{Service: "timeguru", Level: "info", Message: "skip"})

	// Should match
	ls.Broadcast(logagg.LogEntry{Service: "captain", Level: "info", Message: "match"})

	select {
	case received := <-ch:
		if received.Message != "match" {
			t.Errorf("message = %q, want %q", received.Message, "match")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}

	// Verify no extra messages
	select {
	case extra := <-ch:
		t.Errorf("unexpected extra message: %q", extra.Message)
	default:
		// expected
	}
}

func TestLogStream_BroadcastDropsSlowListener(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	ch := make(chan logagg.LogEntry, 1) // tiny buffer
	ls.mu.Lock()
	ls.listeners[ch] = logStreamFilter{}
	ls.mu.Unlock()

	// Fill the buffer
	ls.Broadcast(logagg.LogEntry{Message: "first"})
	// This should be dropped (non-blocking)
	ls.Broadcast(logagg.LogEntry{Message: "dropped"})

	received := <-ch
	if received.Message != "first" {
		t.Errorf("message = %q, want %q", received.Message, "first")
	}
}

func TestLogStream_ListenerCount(t *testing.T) {
	buf := logagg.NewRingBuffer(100)
	ls := NewLogStream(buf)

	ch1 := make(chan logagg.LogEntry, 10)
	ch2 := make(chan logagg.LogEntry, 10)

	ls.mu.Lock()
	ls.listeners[ch1] = logStreamFilter{}
	ls.listeners[ch2] = logStreamFilter{service: "test"}
	ls.mu.Unlock()

	if ls.ListenerCount() != 2 {
		t.Errorf("listener count = %d, want 2", ls.ListenerCount())
	}

	ls.mu.Lock()
	delete(ls.listeners, ch1)
	ls.mu.Unlock()

	if ls.ListenerCount() != 1 {
		t.Errorf("listener count = %d, want 1", ls.ListenerCount())
	}
}
