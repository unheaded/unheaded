// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/metrics"
)

// newInstrumented builds the same shape as NewServer: instrumentation
// outermost, a request-copying middleware in between (as auth does with
// WithContext), and the mux innermost behind capturePattern.
func newInstrumented(t *testing.T, mux *http.ServeMux) (*Server, http.Handler) {
	t.Helper()
	s := &Server{}
	s.initMetrics()
	copying := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), struct{}{}, 1)))
		})
	}
	return s, s.instrumentHTTP(copying(capturePattern(mux)))
}

func scrape(t *testing.T, s *Server) string {
	t.Helper()
	rec := httptest.NewRecorder()
	metrics.HandlerFor(s.metricsRegistry).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec.Body.String()
}

func TestInstrumentHTTP_CountsByPatternNotRawPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/items/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	s, h := newInstrumented(t, mux)

	for _, p := range []string{"/api/v1/items/1", "/api/v1/items/2", "/api/v1/items/3"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}
	out := scrape(t, s)

	want := `dashboard_http_requests_total{method="GET",path="/api/v1/items/{id}",status="418"} 3`
	if !strings.Contains(out, want) {
		t.Fatalf("missing %s in:\n%s", want, out)
	}
	if strings.Contains(out, "/api/v1/items/1") {
		t.Fatalf("raw path leaked into a label:\n%s", out)
	}
	if !strings.Contains(out, `dashboard_http_request_duration_seconds_count{method="GET",path="/api/v1/items/{id}"} 3`) {
		t.Fatalf("duration not observed:\n%s", out)
	}
}

// Label values an attacker controls must fold into a fixed set, or every
// request can mint a new series.
func TestInstrumentHTTP_BoundsAttackerControlledLabels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/known", func(http.ResponseWriter, *http.Request) {})
	s, h := newInstrumented(t, mux)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/random-1", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/random-2", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("BREW", "/known", nil))
	out := scrape(t, s)

	if !strings.Contains(out, `dashboard_http_requests_total{method="GET",path="unmatched",status="404"} 2`) {
		t.Fatalf("unmatched paths not folded:\n%s", out)
	}
	if !strings.Contains(out, `dashboard_http_requests_total{method="OTHER",path="/known",status="200"} 1`) {
		t.Fatalf("non-standard method not folded:\n%s", out)
	}
	if strings.Contains(out, "random") || strings.Contains(out, "BREW") {
		t.Fatalf("attacker-controlled value leaked into a label:\n%s", out)
	}
}

// /ws hijacks and the log stream flushes, both by direct type assertion. A
// recorder that hides either interface breaks them.
func TestInstrumentHTTP_PreservesHijackAndFlush(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/flush", func(w http.ResponseWriter, _ *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Error("Flusher hidden by instrumentation")
			return
		}
		_, _ = w.Write([]byte("x"))
		f.Flush()
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("Hijacker hidden by instrumentation")
			return
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		_, _ = bufrw.WriteString("HTTP/1.1 101 Switching Protocols\r\n\r\n")
		_ = bufrw.Flush()
	})
	s, h := newInstrumented(t, mux)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/flush")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("GET /ws HTTP/1.1\r\nHost: x\r\n\r\n"))
	line, _ := bufio.NewReader(conn).ReadString('\n')
	if !strings.Contains(line, "101") {
		t.Fatalf("hijacked response = %q", line)
	}
	conn.Close()

	// Recorded when the handler returns, which can trail the client's read.
	var out string
	for i := 0; i < 100; i++ {
		if out = scrape(t, s); strings.Contains(out, `path="/ws",status="101"`) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("hijacked request not recorded as 101:\n%s", out)
}

// net/http ignores a WriteHeader after the body has started, so the client
// saw 200. The metric must record what the client saw.
func TestInstrumentHTTP_RecordsStatusTheClientSaw(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/late", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
		w.WriteHeader(http.StatusInternalServerError)
	})
	s, h := newInstrumented(t, mux)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/late", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("client saw %d, test premise broken", rec.Code)
	}
	if out := scrape(t, s); !strings.Contains(out, `path="/late",status="200"} 1`) {
		t.Fatalf("recorded a status the client never saw:\n%s", out)
	}
}
