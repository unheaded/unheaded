// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/critalert"
	"unheaded/pkg/metrics"
	wotanClient "unheaded/pkg/wotan-client"
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
	// REV-1: the snapshot carries each alert, not only its fingerprint, so
	// a subscriber that missed the firing message can hold on it.
	const one = `{"fingerprint":"%s","alertname":"WotanDown","severity":"critical","host":"west","starts_at":"2026-11-15T12:00:00Z"}`
	if last, want := sent[len(sent)-1], `{"snapshot":[`+fmt.Sprintf(one, "fp1")+`,`+fmt.Sprintf(one, "fp2")+`]}`; last != want {
		t.Errorf("snapshot = %s\nwant       %s", last, want)
	}
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() { p.run(ctx, tick); close(done) }()
	tick <- time.Now()
	cancel()
	<-done
	if n := len(sink.all()); n != len(sent)+2 {
		t.Errorf("run's start and one tick published %d snapshots, want 2", n-len(sent))
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

// amAlerts is what a fake Alertmanager answers on /api/v2/alerts: fp1 is a
// critical alert routed to page, active; the rest must be left out.
const amAlerts = `[
 {"fingerprint":"fp1","labels":{"alertname":"WotanDown","severity":"critical","host":"west"},"startsAt":"2026-11-15T12:00:00Z","endsAt":"2999-01-01T00:00:00Z","receivers":[{"name":"page"}],"status":{"state":"active","silencedBy":[],"inhibitedBy":[]}},
 {"fingerprint":"warn","labels":{"alertname":"Slow","severity":"warning","host":"west"},"startsAt":"2026-11-15T12:00:00Z","receivers":[{"name":"ticket"}],"status":{"state":"active"}},
 {"fingerprint":"silenced","labels":{"alertname":"Muted","severity":"critical","host":"west"},"startsAt":"2026-11-15T12:00:00Z","receivers":[{"name":"page"}],"status":{"state":"suppressed","silencedBy":["s1"]}},
 {"fingerprint":"none","labels":{"alertname":"Unrouted","severity":"critical","host":"west"},"startsAt":"2026-11-15T12:00:00Z","receivers":[{"name":"none"}],"status":{"state":"active"}},
 {"fingerprint":"ended","labels":{"alertname":"Old","severity":"critical","host":"west"},"startsAt":"2026-11-15T10:00:00Z","endsAt":"2026-11-15T11:00:00Z","receivers":[{"name":"ticket"}],"status":{"state":"active"}}
]`

func fakeAlertmanager(t *testing.T, body string, status int) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		if r.URL.Path != "/api/v2/alerts" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &queries
}

// REV-1: after a restart the publisher's set is empty; it rebuilds it from
// Alertmanager before its first snapshot, so a subscriber's tracker keeps
// holding. The snapshot it publishes feeds a real critalert tracker.
func TestRestartRebuildsFromAlertmanager(t *testing.T) {
	am, queries := fakeAlertmanager(t, amAlerts, http.StatusOK)
	sink := &fakeSink{}
	p := newCritPublisher(sink, metrics.NewRegistry())
	p.am = &alertmanagerSource{base: am.URL, client: am.Client()}

	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() { p.run(ctx, tick); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for len(sink.all()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no snapshot published after the start rebuild")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	sent := sink.all()
	if len(sent) != 1 || !strings.Contains(sent[0], `"fingerprint":"fp1"`) {
		t.Fatalf("first snapshot = %v, want fp1 only", sent)
	}
	for _, out := range []string{"warn", "silenced", "none", "ended"} {
		if strings.Contains(sent[0], `"`+out+`"`) {
			t.Errorf("snapshot carries %q: %s", out, sent[0])
		}
	}
	if len(*queries) == 0 || !strings.Contains((*queries)[0], "active=true") {
		t.Errorf("Alertmanager queries %v", *queries)
	}

	// A subscriber that started after the alert fired (or whose tracker
	// was cleared by the restart) holds on the snapshot alone.
	tr := critalert.Attach(context.Background(), nil, "sword")
	tr.Handle(&wotanClient.Message{Topic: critalert.Topic, Payload: sent[0]})
	if held, why := tr.Hold(); !held || !strings.Contains(why, "WotanDown") {
		t.Fatalf("subscriber after the restart: Hold() = %v %q, want held on WotanDown", held, why)
	}
}

// REV-1: an Alertmanager that does not answer at start withholds the
// snapshot (subscribers go unknown after 15 min), never an empty one that
// would clear every tracker.
func TestNoSnapshotUntilRebuilt(t *testing.T) {
	am, _ := fakeAlertmanager(t, `oops`, http.StatusServiceUnavailable)
	sink := &fakeSink{}
	p := newCritPublisher(sink, metrics.NewRegistry())
	p.am = &alertmanagerSource{base: am.URL, client: am.Client()}
	p.Snapshot()
	if n := len(sink.all()); n != 0 {
		t.Fatalf("published %d snapshots before the set was rebuilt", n)
	}
	if v := p.rebuildErrors.Value(); v != 1 {
		t.Errorf("alert_mailer_critical_rebuild_errors_total = %v, want 1", v)
	}
	// A firing notification still goes out on its own.
	p.Observe(critHook("firing", "critical", "fp9"))
	if n := len(sink.all()); n != 1 {
		t.Fatalf("published %d, want the one firing message", n)
	}
}

// A resolve that arrives while the rebuild is in flight is not undone by
// it.
func TestResolveDuringRebuildStays(t *testing.T) {
	sink := &fakeSink{}
	p := newCritPublisher(sink, metrics.NewRegistry())
	p.am = fetchFunc(func(context.Context) ([]critMsg, error) {
		p.Observe(critHook("resolved", "critical", "fp1"))
		return []critMsg{{Fingerprint: "fp1", AlertName: "WotanDown", Severity: "critical", Host: "west"}}, nil
	})
	p.Snapshot()
	sent := sink.all()
	if last := sent[len(sent)-1]; last != `{"snapshot":[]}` {
		t.Fatalf("snapshot after a resolve during the rebuild = %s, want empty", last)
	}
}

type fetchFunc func(context.Context) ([]critMsg, error)

func (f fetchFunc) fetch(ctx context.Context) ([]critMsg, error) { return f(ctx) }

func TestAlertmanagerURL(t *testing.T) {
	for raw, ok := range map[string]bool{
		"":                          true,
		"http://alertmanager:9093":  true,
		"https://am.example:9093/":  true,
		"ftp://alertmanager:9093":   false,
		"http://u:p@alertmanager:1": false,
		"alertmanager:9093":         false,
	} {
		_, err := alertmanagerFromEnv(func(string) string { return raw })
		if (err == nil) != ok {
			t.Errorf("ALERTMANAGER_URL %q: err %v, want ok=%v", raw, err, ok)
		}
	}
}
