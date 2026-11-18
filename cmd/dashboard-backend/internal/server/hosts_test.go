// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A trimmed copy of a live huginn /host-summary from west.
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

// /api/v1/hosts reads each host's huginn /host-summary when the page asks
// and keeps nothing (ADR-147 Phase 13): the dashboard's host gauges, its
// series-store copy and its VictoriaMetrics push of the same numbers are
// gone; huginn pushes host_* to VictoriaMetrics itself. A host whose huginn
// does not answer is offline, not zeros.
func TestHostsFromHuginn(t *testing.T) {
	agent, url := newFakeAgent(t)
	srv := newTestServer(t)
	srv.hosts = []KingdomHost{{ID: "west", Addr: "192.0.2.2", Type: "bare-metal", MetricsURL: url}}

	read := func() map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		srv.handleHosts(w, httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil))
		var body struct {
			Hosts []map[string]any `json:"hosts"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Hosts) != 1 {
			t.Fatalf("hosts answer %d %s", w.Code, w.Body.String())
		}
		return body.Hosts[0]
	}
	h := read()
	if h["online"] != true || h["load_1m"] != 7.4 || h["hostname"] != "west" || h["process_total"] != 444.0 {
		t.Errorf("west = %v", h)
	}
	agent.status.Store(http.StatusInternalServerError)
	agent.body.Store("{}")
	if h := read(); h["online"] != false {
		t.Errorf("unreachable huginn: online = %v, want false", h["online"])
	}
}
