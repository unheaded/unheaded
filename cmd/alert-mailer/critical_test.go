// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/metrics"
)

// fakeSink records signed publishes on alerts.critical.
type fakeSink struct {
	mu   sync.Mutex
	sent []string
	err  error
}

func (f *fakeSink) Publish(_ context.Context, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, string(payload))
	return nil
}

func (f *fakeSink) all() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

func critHook(status, severity, fp string) Webhook {
	return Webhook{Version: "4", Status: status, Alerts: []Alert{{
		Status:      status,
		Labels:      map[string]string{"alertname": "WotanDown", "severity": severity, "host": "west", "instance": "wotan:18000"},
		Annotations: map[string]string{"summary": "free text that must not reach the bus"},
		StartsAt:    time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC),
		Fingerprint: fp,
	}}}
}

func TestCriticalNotificationPublishes(t *testing.T) {
	sink := &fakeSink{}
	p := newCritPublisher(sink, metrics.NewRegistry())
	p.Observe(critHook("firing", "critical", "fp1"))
	sent := sink.all()
	if len(sent) != 1 {
		t.Fatalf("published %d, want 1", len(sent))
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(sent[0]), &got); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"alertname", "fingerprint", "host", "severity", "starts_at", "status"}
	if len(keys) != len(want) {
		t.Fatalf("fields %v, want exactly %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("fields %v, want exactly %v", keys, want)
		}
	}
	if got["status"] != "firing" || got["fingerprint"] != "fp1" || got["alertname"] != "WotanDown" || got["host"] != "west" {
		t.Errorf("payload %v", got)
	}
	p.Observe(critHook("resolved", "critical", "fp1"))
	if sent := sink.all(); len(sent) != 2 || !json.Valid([]byte(sent[1])) || !contains([]string{sent[1]}, sent[1]) {
		t.Fatalf("resolved: %v", sent)
	}
	var res map[string]any
	_ = json.Unmarshal([]byte(sink.all()[1]), &res)
	if res["status"] != "resolved" {
		t.Errorf("resolved payload %v", res)
	}
}

func TestOtherSeveritiesDoNot(t *testing.T) {
	sink := &fakeSink{}
	p := newCritPublisher(sink, metrics.NewRegistry())
	p.Observe(critHook("firing", "warning", "fp1"))
	if n := len(sink.all()); n != 0 {
		t.Fatalf("a warning published %d messages", n)
	}
}

func TestNoKeyRefusesAndCounts(t *testing.T) {
	p := newCritPublisher(nil, metrics.NewRegistry()) // Wotan configured, no signing key
	p.Observe(critHook("firing", "critical", "fp1"))
	if v := p.refused.Value(); v != 1 {
		t.Errorf("alert_mailer_critical_publish_refused_total = %v, want 1", v)
	}
}

func TestSnapshotEvery5m(t *testing.T) {
	sink := &fakeSink{}
	p := newCritPublisher(sink, metrics.NewRegistry())
	p.Snapshot() // none firing: an empty snapshot keeps subscribers out of unknown
	p.Observe(critHook("firing", "critical", "fp2"))
	p.Observe(critHook("firing", "critical", "fp1"))
	p.Snapshot()
	sent := sink.all()
	if sent[0] != `{"snapshot":[]}` {
		t.Errorf("empty snapshot = %s", sent[0])
	}
	if last := sent[len(sent)-1]; last != `{"snapshot":["fp1","fp2"]}` {
		t.Errorf("snapshot = %s", last)
	}
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() { p.run(ctx, tick); close(done) }()
	tick <- time.Now()
	cancel()
	<-done
	if n := len(sink.all()); n != len(sent)+1 {
		t.Errorf("a tick published %d snapshots, want 1", n-len(sent))
	}
	if snapshotEvery != 5*time.Minute {
		t.Errorf("snapshotEvery = %v", snapshotEvery)
	}
}

func TestPublishFailureDoesNotFailTheWebhook(t *testing.T) {
	sink := &fakeSink{err: errors.New("wotan down")}
	p := newCritPublisher(sink, metrics.NewRegistry())
	p.Observe(critHook("firing", "critical", "fp1")) // returns; no panic
	if v := p.errors.Value(); v != 1 {
		t.Errorf("alert_mailer_critical_publish_errors_total = %v, want 1", v)
	}
}

func TestWotanOffIsQuiet(t *testing.T) {
	env := map[string]string{}
	p, err := critFromEnv(func(k string) string { return env[k] }, metrics.NewRegistry())
	if err != nil || p != nil {
		t.Fatalf("no WOTAN_ADDR: %v %v, want nil, nil", p, err)
	}
	var none *critPublisher
	none.Observe(critHook("firing", "critical", "fp1")) // nil-safe
	none.Snapshot()
}
