// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/threatfeed/kev"
)

func TestParseFlagsInterval(t *testing.T) {
	for _, bad := range []string{"1m", "14m59s", "25h", "soon"} {
		if _, err := parseFlags([]string{"--interval", bad}); err == nil {
			t.Errorf("--interval %s accepted", bad)
		}
	}
	for _, good := range []string{"15m", "1h", "6h", "24h"} {
		c, err := parseFlags([]string{"--interval", good, "--node", "n"})
		if err != nil {
			t.Errorf("--interval %s: %v", good, err)
		}
		if c.node != "n" {
			t.Errorf("node = %q", c.node)
		}
	}
	t.Setenv("KEV_PULL_INTERVAL", "6h")
	if c, err := parseFlags(nil); err != nil || c.interval != 6*time.Hour {
		t.Fatalf("env interval: %v %v", c.interval, err)
	}
}

func TestDeltaMessagesChunking(t *testing.T) {
	var out kev.Outcome
	if msgs := deltaMessages("n", out, time.Now()); msgs != nil {
		t.Fatalf("empty delta produced %d messages", len(msgs))
	}
	out.Delta.Removed = []string{"CVE-2020-0001"}
	msgs := deltaMessages("n", out, time.Now())
	if len(msgs) != 1 || len(msgs[0].Removed) != 1 || msgs[0].Added == nil {
		t.Fatalf("removal-only delta: %+v", msgs)
	}
	for i := 0; i < 2*deltaChunk+1; i++ {
		out.Delta.Added = append(out.Delta.Added, kev.Vulnerability{CVEID: fmt.Sprintf("CVE-2026-%04d", i)})
	}
	msgs = deltaMessages("n", out, time.Now())
	if len(msgs) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(msgs))
	}
	total := 0
	for i, m := range msgs {
		total += len(m.Added)
		if m.Chunk != i+1 || m.Chunks != 3 {
			t.Fatalf("chunk %d: %d/%d", i, m.Chunk, m.Chunks)
		}
		if (i == 0) != (len(m.Removed) == 1) {
			t.Fatalf("Removed must ride in the first chunk only (chunk %d)", i+1)
		}
		b, _ := json.Marshal(m)
		if len(b) > 512<<10 {
			t.Fatalf("chunk %d is %d bytes", i+1, len(b))
		}
	}
	if total != 2*deltaChunk+1 {
		t.Fatalf("entries lost across chunks: %d", total)
	}
}

func catalog(version string, ids ...string) []byte {
	c := kev.Catalog{CatalogVersion: version, Count: len(ids)}
	for _, id := range ids {
		c.Vulnerabilities = append(c.Vulnerabilities, kev.Vulnerability{CVEID: id})
	}
	b, _ := json.Marshal(c)
	return b
}

func TestDaemonPollPublishesAndServes(t *testing.T) {
	var mu sync.Mutex
	body := catalog("1", "CVE-2024-0001")
	status := http.StatusOK
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	defer feed.Close()

	f, err := kev.NewFetcher(feed.URL, 5*time.Second, true)
	if err != nil {
		t.Fatal(err)
	}
	type pub struct{ topic, payload string }
	var pubs []pub
	d := &daemon{
		cfg:     config{interval: time.Hour, keep: time.Hour, timeout: 5 * time.Second, node: "test"},
		poller:  &kev.Poller{Fetcher: f, Store: kev.Store{Dir: t.TempDir()}},
		metrics: newPollMetrics(),
		publish: func(topic string, payload []byte) error {
			pubs = append(pubs, pub{topic, string(payload)})
			return nil
		},
	}
	h := d.handler()
	get := func(path string) (int, string) {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		b, _ := io.ReadAll(rr.Body)
		return rr.Code, string(b)
	}

	if code, _ := get("/ready"); code != http.StatusServiceUnavailable {
		t.Fatalf("/ready before any catalog = %d", code)
	}
	if err := d.pollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if code, _ := get("/ready"); code != http.StatusOK {
		t.Fatalf("/ready after a pull = %d", code)
	}
	if len(pubs) != 1 || pubs[0].topic != topicUpdates || !strings.Contains(pubs[0].payload, `"bootstrap":true`) {
		t.Fatalf("bootstrap publishes one update and no delta: %+v", pubs)
	}

	mu.Lock()
	body = catalog("2", "CVE-2024-0001", "CVE-2024-0002")
	mu.Unlock()
	if err := d.pollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pubs) != 3 || pubs[2].topic != topicDelta || !strings.Contains(pubs[2].payload, "CVE-2024-0002") {
		t.Fatalf("change publishes update + delta: %+v", pubs)
	}

	mu.Lock()
	status = http.StatusBadGateway
	mu.Unlock()
	if err := d.pollOnce(context.Background()); err == nil {
		t.Fatal("502 was a success")
	}
	_, status200 := get("/api/v1/kev")
	if !strings.Contains(status200, "HTTP 502") || !strings.Contains(status200, `"count":2`) {
		t.Fatalf("/api/v1/kev after a failure: %s", status200)
	}
	_, m := get("/metrics")
	for _, want := range []string{
		"unheaded_kev_pull_success_total 2",
		"unheaded_kev_pull_failure_total 1",
		"unheaded_kev_total_count 2",
		"unheaded_kev_delta_count 1",
		`unheaded_http_requests_total{method="GET",path="/ready",service="kev-poller",status="200"}`,
	} {
		if !strings.Contains(m, want) {
			t.Errorf("/metrics lacks %q", want)
		}
	}
}
