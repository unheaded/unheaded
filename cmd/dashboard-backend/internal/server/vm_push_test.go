// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"unheaded/pkg/logger"
)

// L10: the push answers for the address it was given and goes nowhere else.
func TestPushToVictoriaMetrics_FollowsNoRedirect(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhere.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer other.Close()
	vm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/v1/import/prometheus", http.StatusTemporaryRedirect)
	}))
	defer vm.Close()

	srv := newTestServer(t)
	srv.config.VMUrl = vm.URL
	srv.pushToVictoriaMetrics(context.Background(), []string{`host_load_1m{host="west"} 1`})
	if got := elsewhere.Load(); got != 0 {
		t.Fatalf("the push followed a redirect %d time(s)", got)
	}
}

// L10: an error answer of any size costs a bounded read and a bounded log line.
func TestPushToVictoriaMetrics_BoundsTheErrorBody(t *testing.T) {
	vm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(strings.Repeat("x", 4<<20)))
	}))
	defer vm.Close()

	var logged bytes.Buffer
	srv := newTestServer(t)
	srv.log = logger.New(&logged)
	srv.config.VMUrl = vm.URL
	srv.pushToVictoriaMetrics(context.Background(), []string{`host_load_1m{host="west"} 1`})
	if !strings.Contains(logged.String(), "VM push returned non-2xx") {
		t.Fatalf("no warning logged: %.200s", logged.String())
	}
	if got := logged.Len(); got > 2*maxVMErrorBody {
		t.Fatalf("logged %d bytes for one failed push", got)
	}
}
