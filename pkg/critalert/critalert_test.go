// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package critalert

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// clock is an injectable time source.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// logBuf collects log lines.
type logBuf struct {
	mu    sync.Mutex
	lines []string
}

func (l *logBuf) printf(format string, args ...any) {
	l.mu.Lock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
	l.mu.Unlock()
}

func (l *logBuf) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.lines)
}

func newTestTracker(t *testing.T) (*Tracker, *clock, *logBuf) {
	t.Helper()
	c := &clock{now: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}
	l := &logBuf{}
	tr := newTracker(c.Now, l.printf)
	return tr, c, l
}

func msg(payload string) *wotanClient.Message {
	return &wotanClient.Message{Topic: Topic, Payload: payload}
}

func alertJSON(fp, status string) string {
	return `{"fingerprint":"` + fp + `","alertname":"WotanDown","severity":"critical","status":"` + status + `","host":"west","starts_at":"2026-11-15T11:59:00Z"}`
}

// seriesValue reads one series from a family's exposition text.
func seriesValue(t *testing.T, family interface{ Write(io.Writer) error }, name, labels string) (float64, bool) {
	t.Helper()
	var buf bytes.Buffer
	if err := family.Write(&buf); err != nil {
		t.Fatal(err)
	}
	prefix := name + " "
	if labels != "" {
		prefix = name + "{" + labels + "} "
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, prefix) {
			v, err := strconv.ParseFloat(strings.TrimPrefix(line, prefix), 64)
			if err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			return v, true
		}
	}
	return 0, false
}

func TestFiringThenResolved(t *testing.T) {
	tr, _, _ := newTestTracker(t)
	tr.Handle(msg(alertJSON("fp1", "firing")))
	if got := tr.Active(); len(got) != 1 || got[0].Fingerprint != "fp1" || got[0].Name != "WotanDown" || got[0].Host != "west" {
		t.Fatalf("Active() = %+v, want fp1", got)
	}
	if !tr.Firing() || tr.State() != Firing {
		t.Fatalf("Firing() = %v, State() = %v", tr.Firing(), tr.State())
	}
	if v, _ := seriesValue(t, activeGauge, "unheaded_critical_alerts_active", ""); v != 1 {
		t.Errorf("unheaded_critical_alerts_active = %v, want 1", v)
	}
	tr.Handle(msg(alertJSON("fp1", "resolved")))
	if got := tr.Active(); len(got) != 0 {
		t.Fatalf("Active() after resolved = %+v", got)
	}
	if tr.Firing() || tr.State() != OK {
		t.Fatalf("after resolved: Firing() = %v, State() = %v", tr.Firing(), tr.State())
	}
	if v, _ := seriesValue(t, activeGauge, "unheaded_critical_alerts_active", ""); v != 0 {
		t.Errorf("unheaded_critical_alerts_active = %v, want 0", v)
	}
	if h := tr.Health(); h["state"] != "ok" || h["active"] != 0 {
		t.Errorf("Health() = %v", h)
	}
}

func TestSnapshotDropsWhatItOmits(t *testing.T) {
	tr, _, _ := newTestTracker(t)
	tr.Handle(msg(alertJSON("fp1", "firing")))
	tr.Handle(msg(alertJSON("fp2", "firing")))
	tr.Handle(msg(`{"snapshot":["fp2"]}`))
	got := tr.Active()
	if len(got) != 1 || got[0].Fingerprint != "fp2" {
		t.Fatalf("Active() after a snapshot naming fp2 = %+v", got)
	}
	tr.Handle(msg(`{"snapshot":[]}`))
	if len(tr.Active()) != 0 || tr.State() != OK {
		t.Fatalf("an empty snapshot left %+v (%v)", tr.Active(), tr.State())
	}
}

func TestSilentPublisherIsUnknown(t *testing.T) {
	tr, c, l := newTestTracker(t)
	tr.Handle(msg(alertJSON("fp1", "firing")))
	before := l.count()
	c.add(SilentAfter + time.Second)
	if s := tr.State(); s != Unknown {
		t.Fatalf("State() after %v of silence = %v, want unknown", SilentAfter, s)
	}
	if tr.Firing() {
		t.Error("Firing() true while unknown: unknown allows")
	}
	if v, _ := seriesValue(t, stateGauge, "unheaded_critical_alerts_state", `state="unknown"`); v != 1 {
		t.Errorf(`unheaded_critical_alerts_state{state="unknown"} = %v, want 1`, v)
	}
	tr.State()
	tr.State()
	if n := l.count() - before; n != 1 {
		t.Errorf("going unknown logged %d lines, want 1", n)
	}
	// A snapshot brings it back.
	tr.Handle(msg(`{"snapshot":["fp1"]}`))
	if s := tr.State(); s != Firing {
		t.Errorf("State() after a snapshot = %v, want firing", s)
	}
}

func TestBounded(t *testing.T) {
	tr, _, _ := newTestTracker(t)
	before, _ := seriesValue(t, refused, "unheaded_critical_alerts_refused_total", `reason="full"`)
	for i := 0; i < MaxActive+1; i++ {
		tr.Handle(msg(alertJSON("fp"+strconv.Itoa(i), "firing")))
	}
	if n := len(tr.Active()); n != MaxActive {
		t.Fatalf("Active() holds %d, want %d", n, MaxActive)
	}
	after, _ := seriesValue(t, refused, "unheaded_critical_alerts_refused_total", `reason="full"`)
	if after-before != 1 {
		t.Errorf(`refused{reason="full"} rose %v, want 1`, after-before)
	}
}

func TestBadPayloadCounted(t *testing.T) {
	long := strings.Repeat("x", MaxField+1)
	cases := map[string]struct{ payload, reason string }{
		"not json":      {`not json`, "decode"},
		"unknown field": {`{"fingerprint":"fp","alertname":"A","severity":"critical","status":"firing","host":"west","starts_at":"2026-11-15T11:59:00Z","summary":"text"}`, "decode"},
		"bad status":    {alertJSON("fp", "pending"), "status"},
		"no fingerprint": {`{"fingerprint":"","alertname":"A","severity":"critical","status":"firing","host":"west","starts_at":"2026-11-15T11:59:00Z"}`,
			"field"},
		"long field":     {alertJSON(long, "firing"), "field"},
		"long snapshot":  {`{"snapshot":["` + long + `"]}`, "field"},
		"mixed snapshot": {`{"snapshot":[],"fingerprint":"fp"}`, "field"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tr, _, _ := newTestTracker(t)
			tr.Handle(msg(alertJSON("keep", "firing")))
			before, _ := seriesValue(t, refused, "unheaded_critical_alerts_refused_total", `reason="`+tc.reason+`"`)
			tr.Handle(msg(tc.payload))
			after, _ := seriesValue(t, refused, "unheaded_critical_alerts_refused_total", `reason="`+tc.reason+`"`)
			if after-before != 1 {
				t.Errorf(`refused{reason=%q} rose %v, want 1`, tc.reason, after-before)
			}
			if got := tr.Active(); len(got) != 1 || got[0].Fingerprint != "keep" {
				t.Errorf("state changed: %+v", got)
			}
		})
	}
	tr, _, _ := newTestTracker(t)
	tr.Handle(nil) // must not panic
	if tr.State() != OK {
		t.Error("a nil message changed the state")
	}
}

func TestOneLogLinePerChange(t *testing.T) {
	tr, _, l := newTestTracker(t)
	for i := 0; i < 10; i++ {
		tr.Handle(msg(alertJSON("fp1", "firing")))
	}
	if n := l.count(); n != 1 {
		t.Fatalf("the same firing alert 10 times logged %d lines, want 1: %v", n, l.lines)
	}
	tr.Handle(msg(alertJSON("fp1", "resolved")))
	tr.Handle(msg(alertJSON("fp1", "resolved")))
	if n := l.count(); n != 2 {
		t.Errorf("one resolve logged %d lines in all, want 2", n)
	}
}

// fakeSub records the subscription Attach makes.
type fakeSub struct {
	mu     sync.Mutex
	topics []string
	onMsg  func(*wotanClient.Message)
}

func (f *fakeSub) SubscribeWithRetry(_ context.Context, topic, _ string, onMsg func(*wotanClient.Message)) error {
	f.mu.Lock()
	f.topics = append(f.topics, topic)
	f.onMsg = onMsg
	f.mu.Unlock()
	return nil
}

func TestAttachSubscribes(t *testing.T) {
	f := &fakeSub{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr := Attach(ctx, f, "timeguru")
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.mu.Lock()
		on := f.onMsg
		f.mu.Unlock()
		if on != nil {
			on(msg(alertJSON("fp1", "firing")))
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Attach did not subscribe")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if f.topics[0] != Topic || !tr.Firing() {
		t.Fatalf("topics %v, firing %v", f.topics, tr.Firing())
	}
	// A nil client gives a tracker that never fires and does not panic.
	if Attach(ctx, nil, "x").Firing() {
		t.Error("a tracker with no client fires")
	}
}

func TestHold(t *testing.T) {
	tr, c, _ := newTestTracker(t)
	if held, _ := tr.Hold(); held {
		t.Fatal("held with nothing firing")
	}
	tr.Handle(msg(alertJSON("fp1", "firing")))
	held, why := tr.Hold()
	if !held || !strings.Contains(why, "WotanDown") {
		t.Fatalf("Hold() = %v %q while WotanDown fires", held, why)
	}
	c.add(SilentAfter + time.Second)
	if held, _ := tr.Hold(); held {
		t.Error("held while unknown; unknown allows")
	}
	var none *Tracker
	if held, _ := none.Hold(); held {
		t.Error("a nil tracker holds")
	}
}
