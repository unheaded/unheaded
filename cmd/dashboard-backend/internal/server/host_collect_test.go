// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A trimmed copy of a live host-agent /host-summary from west.
const westSummary = `{"cpu_percent":13.5,"cpu_count":12,"memory_total":15367344128,` +
	`"memory_used":6607654912,"memory_percent":43.0,"swap_total":36507213824,` +
	`"swap_used":1237618688,"swap_percent":3.4,"load_1m":7.4,"load_5m":4.17,` +
	`"load_15m":3.63,"uptime_seconds":78641.33,"disks":[{"mount":"/","filesystem":"/dev/sda1",` +
	`"size_bytes":358736535552,"used_bytes":242513489920,"avail_bytes":1,"use_percent":67.6}],` +
	`"net_connections":{"established":36,"time_wait":6,"close_wait":0},` +
	`"process_total":444,"process_zombie":0,"hostname":"west","kernel":"6.17.0"}`

// fakeAgent serves /host-summary; status and body can change between cycles.
type fakeAgent struct {
	status atomic.Int32
	body   atomic.Value
}

func newFakeAgent(t *testing.T) (*fakeAgent, string) {
	t.Helper()
	a := &fakeAgent{}
	a.status.Store(http.StatusOK)
	a.body.Store(westSummary)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(a.status.Load()))
		io.WriteString(w, a.body.Load().(string))
	}))
	t.Cleanup(ts.Close)
	return a, ts.URL + "/host-summary"
}

func TestCollectHostMetrics_RemoteExportsHostSummary(t *testing.T) {
	_, url := newFakeAgent(t)
	srv := newTestServer(t)
	srv.hosts = []KingdomHost{{ID: "west", MetricsURL: url}}

	srv.doCollectHostMetrics(context.Background())
	out := scrape(t, srv)

	for _, want := range []string{
		`host_cpu_percent{host="west"} 13.5`,
		`host_memory_used_bytes{host="west"} 6.607654912e+09`,
		`host_memory_total_bytes{host="west"} 1.5367344128e+10`,
		`host_swap_used_bytes{host="west"} 1.237618688e+09`,
		`host_load_1m{host="west"} 7.4`,
		`host_load_5m{host="west"} 4.17`,
		`host_load_15m{host="west"} 3.63`,
		`host_uptime_seconds{host="west"} 78641.33`,
		`host_net_established{host="west"} 36`,
		`host_processes_total{host="west"} 444`,
		`host_disk_used_bytes{host="west",mount="/"} 2.4251348992e+11`,
		`host_disk_total_bytes{host="west",mount="/"} 3.58736535552e+11`,
	} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("missing %s", want)
		}
	}
	// A host has no goroutines; the old code published the agent's (or 0).
	if strings.Contains(out, `host_goroutines{host="west"}`) {
		t.Errorf("remote host exported host_goroutines")
	}

	// The series store gets the same values.
	got := srv.scraper.QueryMetrics("host_load_1m", map[string]string{"host": "west"}, time.Time{})
	if len(got) != 1 || got[0].Value != 7.4 || got[0].Service != "system" {
		t.Errorf("store host_load_1m = %+v, want one system sample of 7.4", got)
	}
	disk := srv.scraper.QueryMetrics("host_disk_used_bytes", map[string]string{"host": "west", "mount": "/"}, time.Time{})
	if len(disk) != 1 || disk[0].Value != 242513489920 {
		t.Errorf("store host_disk_used_bytes = %+v", disk)
	}
}

func TestCollectHostMetrics_UnreachableHostIsDropped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int32
		body   string
	}{
		{"http 500 with a JSON body", http.StatusInternalServerError, westSummary},
		{"not JSON", http.StatusOK, "host_cpu_percent 5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			west, westURL := newFakeAgent(t)
			_, eastURL := newFakeAgent(t)
			srv := newTestServer(t)
			srv.hosts = []KingdomHost{
				{ID: "west", MetricsURL: westURL},
				{ID: "east", MetricsURL: eastURL},
			}
			srv.doCollectHostMetrics(context.Background())
			if out := scrape(t, srv); !strings.Contains(out, `host_cpu_percent{host="west"} 13.5`) {
				t.Fatalf("first cycle did not export west:\n%s", out)
			}

			west.status.Store(tc.status)
			west.body.Store(tc.body)
			srv.doCollectHostMetrics(context.Background())

			out := scrape(t, srv)
			if strings.Contains(out, `host="west"`) {
				t.Errorf("west still exported after its agent failed:\n%s", grepLines(out, `host="west"`))
			}
			if !strings.Contains(out, `host_disk_used_bytes{host="east",mount="/"} 2.4251348992e+11`) {
				t.Errorf("east lost when west failed")
			}
		})
	}
}

func TestCollectHostMetrics_PushesRealValuesToVictoriaMetrics(t *testing.T) {
	_, url := newFakeAgent(t)
	var (
		mu   sync.Mutex
		push string
	)
	vm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		push = string(b)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer vm.Close()

	srv := newTestServer(t)
	srv.config.VMUrl = vm.URL
	srv.hosts = []KingdomHost{{ID: "west", MetricsURL: url}}
	srv.doCollectHostMetrics(context.Background())

	mu.Lock()
	defer mu.Unlock()
	for _, want := range []string{
		`host_cpu_percent{host="west"} 13.5`,
		`host_load_1m{host="west"} 7.4`,
		`host_disk_total_bytes{host="west",mount="/"} 3.58736535552e+11`,
	} {
		if !strings.Contains(push, want+"\n") {
			t.Errorf("VM push missing %s in:\n%s", want, push)
		}
	}
	if strings.Contains(push, "host_goroutines") {
		t.Errorf("VM push carried host_goroutines for a remote host")
	}
}

func TestFormatHostLabels_Escapes(t *testing.T) {
	got := formatHostLabels(map[string]string{"mount": "/a\"b\\c\nd", "host": "west"})
	want := `{host="west",mount="/a\"b\\c\nd"}`
	if got != want {
		t.Errorf("formatHostLabels = %s, want %s", got, want)
	}
}

func grepLines(s, sub string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}
