// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/wotansig"
)

// flakyWotan is an HTTP Wotan whose publish endpoint can be switched between
// down (503, as Wotan answers while it cannot take writes) and up. It records
// the payloads it accepted, in order, and refuses a publish from a subscriber
// ID it did not issue since its last restart, like the real one.
type flakyWotan struct {
	srv  *httptest.Server
	down atomic.Bool

	mu       sync.Mutex
	got      []string
	members  map[string]bool
	nextID   int
	restarts int
}

func newFlakyWotan(t *testing.T) *flakyWotan {
	t.Helper()
	fw := &flakyWotan{members: map[string]bool{}}
	fw.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/subscribe"):
			fw.mu.Lock()
			fw.nextID++
			id := "sub-" + string(rune('a'+fw.nextID))
			fw.members[id] = true
			fw.mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"subscriber": map[string]string{"subscriber_id": id, "status": "approved", "display_name": "svc"},
			})
		case strings.HasSuffix(r.URL.Path, "/publish"):
			if fw.down.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":"UNAVAILABLE","message":"wotan starting"}}`))
				return
			}
			var body struct {
				SubscriberID string `json:"subscriber_id"`
				Payload      string `json:"payload"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			fw.mu.Lock()
			known := fw.members[body.SubscriberID]
			if known {
				fw.got = append(fw.got, body.Payload)
			}
			fw.mu.Unlock()
			if !known {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"Forbidden","message":"subscriber not approved"}}`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fw.srv.Close)
	return fw
}

// restart forgets every membership, as a Wotan restart does.
func (fw *flakyWotan) restart() {
	fw.mu.Lock()
	fw.members = map[string]bool{}
	fw.restarts++
	fw.mu.Unlock()
}

func (fw *flakyWotan) accepted() []string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	return append([]string(nil), fw.got...)
}

func (fw *flakyWotan) client(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(strings.TrimPrefix(fw.srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	c.fallbackEvery = 20 * time.Millisecond
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", within, what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Messages buffered while Wotan was down reach it, in the order they were
// published, without the caller doing anything: the queue used to fill to
// 1000 and drop the oldest forever, because nothing called FlushFallbackQueue.
func TestFallback_FlushesInOrderAfterRecovery(t *testing.T) {
	fw := newFlakyWotan(t)
	c := fw.client(t)
	ctx := context.Background()
	if _, err := c.Subscribe(ctx, "metrics.host", "svc"); err != nil {
		t.Fatal(err)
	}

	fw.down.Store(true)
	for _, p := range []string{"m1", "m2", "m3"} {
		if err := c.Publish(ctx, "metrics.host", []byte(p)); err == nil {
			t.Fatalf("publish %s while Wotan is down returned nil", p)
		}
	}
	if n := c.FallbackQueueLen(); n != 3 {
		t.Fatalf("queued %d, want 3", n)
	}

	fw.down.Store(false) // recovery; no further Publish call
	waitFor(t, 5*time.Second, "the background flush", func() bool { return len(fw.accepted()) == 3 })
	if got := fw.accepted(); strings.Join(got, ",") != "m1,m2,m3" {
		t.Errorf("Wotan received %v, want [m1 m2 m3] in order", got)
	}
	waitFor(t, 5*time.Second, "the flush to be counted", func() bool { return c.FallbackStats().Flushed == 3 })
	st := c.FallbackStats()
	if st.Queued != 3 || st.Flushed != 3 || st.Pending != 0 {
		t.Errorf("stats %+v, want queued 3, flushed 3, pending 0", st)
	}
}

// A buffered entry older than the max age is dropped and counted as
// expired, never sent: minutes-old telemetry replayed as if current is noise.
func TestFallback_ExpiresOlderThanMaxAge(t *testing.T) {
	fw := newFlakyWotan(t)
	c := fw.client(t)
	c.fallbackEvery = time.Hour // only the explicit flush below runs
	var now atomic.Int64
	now.Store(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC).UnixNano())
	c.now = func() time.Time { return time.Unix(0, now.Load()) }
	ctx := context.Background()
	if _, err := c.Subscribe(ctx, "metrics.host", "svc"); err != nil {
		t.Fatal(err)
	}

	fw.down.Store(true)
	_ = c.Publish(ctx, "metrics.host", []byte("stale"))
	now.Add(int64(maxFallbackAge + time.Second))
	_ = c.Publish(ctx, "metrics.host", []byte("fresh"))
	fw.down.Store(false)

	flushed, err := c.FlushFallbackQueue(ctx)
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	if flushed != 1 {
		t.Errorf("flushed %d, want 1 (the fresh entry only)", flushed)
	}
	if got := fw.accepted(); len(got) != 1 || got[0] != "fresh" {
		t.Errorf("Wotan received %v, want only [fresh]", got)
	}
	if st := c.FallbackStats(); st.Expired != 1 {
		t.Errorf("expired %d, want 1", st.Expired)
	}
}

// Control topics are never buffered: a chaos.abort or deploy.* replayed
// minutes late is worse than one that fails now, so the caller gets the error
// and nothing is queued.
func TestFallback_ControlTopicsAreNotBuffered(t *testing.T) {
	fw := newFlakyWotan(t)
	c := fw.client(t)
	ctx := context.Background()
	for _, topic := range []string{"chaos.abort", "config.update", "deploy.started", "cuirass.chaos", "deploy.progress"} {
		if _, err := c.Subscribe(ctx, topic, "svc"); err != nil {
			t.Fatal(err)
		}
	}
	fw.down.Store(true)
	// config.* is a signed topic: an unsigned publish never leaves the
	// client, so it is neither buffered nor counted as refused here.
	if err := c.Publish(ctx, "config.update", []byte(`{"x":1}`)); !errors.Is(err, wotansig.ErrSignatureRequired) {
		t.Errorf("unsigned config.update: %v, want ErrSignatureRequired", err)
	}
	for _, topic := range []string{"chaos.abort", "deploy.progress", "deploy.started", "cuirass.chaos"} {
		err := c.Publish(ctx, topic, []byte(`{"x":1}`))
		if err == nil {
			t.Fatalf("publish on %s while Wotan is down returned nil", topic)
		}
		if strings.Contains(err.Error(), "buffered for retry") {
			t.Errorf("%s: error %q says it was buffered", topic, err)
		}
	}
	if n := c.FallbackQueueLen(); n != 0 {
		t.Errorf("queue holds %d control messages, want 0", n)
	}
	if st := c.FallbackStats(); st.Refused != 4 {
		t.Errorf("refused %d, want 4", st.Refused)
	}
}

// Wotan forgets memberships when it restarts, which is when the queue fills.
// The flush joins again and delivers instead of dropping everything as
// "not approved".
func TestFallback_FlushRejoinsAfterWotanRestart(t *testing.T) {
	fw := newFlakyWotan(t)
	c := fw.client(t)
	ctx := context.Background()
	if _, err := c.Subscribe(ctx, "events.audit", "svc"); err != nil {
		t.Fatal(err)
	}
	fw.down.Store(true)
	_ = c.Publish(ctx, "events.audit", []byte("e1"))
	_ = c.Publish(ctx, "events.audit", []byte("e2"))
	fw.restart()
	fw.down.Store(false)

	waitFor(t, 5*time.Second, "the flush after the restart", func() bool { return len(fw.accepted()) == 2 })
	if got := fw.accepted(); strings.Join(got, ",") != "e1,e2" {
		t.Errorf("Wotan received %v, want [e1 e2]", got)
	}
}

// Close stops the flusher: no goroutine is left retrying a Wotan the process
// no longer talks to.
func TestFallback_CloseStopsFlusher(t *testing.T) {
	fw := newFlakyWotan(t)
	settle := func() int {
		var n int
		for i := 0; i < 50; i++ {
			runtime.GC()
			n = runtime.NumGoroutine()
			time.Sleep(10 * time.Millisecond)
			if m := runtime.NumGoroutine(); m == n {
				return n
			}
		}
		return n
	}
	before := settle()

	c, err := NewClient(strings.TrimPrefix(fw.srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	c.fallbackEvery = 10 * time.Millisecond
	ctx := context.Background()
	if _, err := c.Subscribe(ctx, "metrics.host", "svc"); err != nil {
		t.Fatal(err)
	}
	fw.down.Store(true) // stays down, so the flusher keeps running
	_ = c.Publish(ctx, "metrics.host", []byte("m"))
	if !c.flusherRunning() {
		t.Fatal("no flusher started for a non-empty queue")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if c.flusherRunning() {
		t.Error("flusher still running after Close")
	}
	// Idle HTTP keep-alive connections are the client's, not the flusher's.
	c.controlClient.CloseIdleConnections()
	deadline := time.Now().Add(3 * time.Second)
	for {
		after := settle()
		if after <= before {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines %d after Close, %d before the client existed", after, before)
		}
	}
}
