// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/metrics/prom"
)

func TestNewServiceMetrics(t *testing.T) {
	// Use a unique name to avoid Prometheus duplicate registration
	m := NewServiceMetrics("test_svc_" + t.Name())
	if m == nil {
		t.Fatal("nil")
	}
	if m.RequestsTotal == nil {
		t.Error("RequestsTotal nil")
	}
	if m.RequestDuration == nil {
		t.Error("RequestDuration nil")
	}
}

// Servers are built more than once per process (tests do it constantly); the
// second build must share the first one's series, not panic re-registering.
func TestNewServiceMetrics_SameServiceTwice(t *testing.T) {
	a := NewServiceMetrics("t_twice")
	b := NewServiceMetrics("t_twice")
	if a != b {
		t.Fatal("second call built a second set of metrics")
	}
	if NewServiceMetrics("t_twice_other") == a {
		t.Fatal("different services share metrics")
	}
}

func TestStatusRecorder_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: w, status: 200}

	rec.WriteHeader(http.StatusNotFound)
	rec.WriteHeader(http.StatusTeapot) // superfluous; the first one stands
	if rec.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.status)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("underlying writer code = %d, want 404", w.Code)
	}
}

// scrapeService returns the default registry's lines for one service's
// const label, so tests sharing the registry cannot see each other.
func scrapeService(t *testing.T, svc string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	prom.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	var out []string
	for _, l := range strings.Split(rec.Body.String(), "\n") {
		if strings.Contains(l, `service="`+svc+`"`) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// withAuthLikeCopy stands in for auth.WrapHandler: it hands the mux a copy
// of the request, and rejects one path before the mux sees it.
func withAuthLikeCopy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), struct{}{}, 1)))
	})
}

func TestInstrument_CountsByPatternNotRawPath(t *testing.T) {
	svc := "t_pattern"
	m := NewServiceMetrics(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/items/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	mux.HandleFunc("/api/v1/secret", func(http.ResponseWriter, *http.Request) {})
	h := m.Instrument(mux, withAuthLikeCopy(mux))

	for _, p := range []string{"/api/v1/items/1", "/api/v1/items/2", "/api/v1/items/3"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil))
	out := scrapeService(t, svc)

	for _, want := range []string{
		`unheaded_http_requests_total{method="GET",path="/api/v1/items/{id}",service="t_pattern",status="418"} 3`,
		`unheaded_http_request_duration_seconds_count{method="GET",path="/api/v1/items/{id}",service="t_pattern"} 3`,
		// rejected by the middleware, still counted under the route it targeted
		`unheaded_http_requests_total{method="GET",path="/api/v1/secret",service="t_pattern",status="401"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/api/v1/items/1") {
		t.Fatalf("raw path leaked into a label:\n%s", out)
	}
}

// Label values an attacker controls must fold into a fixed set, or every
// request can mint a new series.
func TestInstrument_BoundsAttackerControlledLabels(t *testing.T) {
	svc := "t_bounds"
	m := NewServiceMetrics(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("/known", func(http.ResponseWriter, *http.Request) {})
	h := m.Instrument(mux, mux)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/random-1", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/random-2", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("BREW", "/known", nil))
	out := scrapeService(t, svc)

	for _, want := range []string{
		`unheaded_http_requests_total{method="GET",path="unmatched",service="t_bounds",status="404"} 2`,
		`unheaded_http_requests_total{method="OTHER",path="/known",service="t_bounds",status="200"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "random") || strings.Contains(out, "BREW") {
		t.Fatalf("attacker-controlled value leaked into a label:\n%s", out)
	}
}

func TestStatusRecorder_KeepsFlushHijackUnwrap(t *testing.T) {
	var w http.ResponseWriter = &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: 200}
	if _, ok := w.(http.Flusher); !ok {
		t.Error("recorder hides http.Flusher")
	}
	if _, ok := w.(http.Hijacker); !ok {
		t.Error("recorder hides http.Hijacker")
	}
	if err := http.NewResponseController(w).Flush(); err != nil {
		t.Errorf("ResponseController.Flush: %v", err)
	}
	// httptest.ResponseRecorder cannot hijack: the error must surface, and
	// the status must not claim a protocol switch that did not happen.
	rec := w.(*statusRecorder)
	if _, _, err := rec.Hijack(); err == nil {
		t.Error("Hijack on a non-hijacker succeeded")
	}
	if rec.status == http.StatusSwitchingProtocols {
		t.Error("failed hijack recorded 101")
	}
}

func TestRequestError_Error(t *testing.T) {
	err := &RequestError{
		Status:  400,
		Code:    "BAD",
		Message: "bad request",
	}
	if err.Error() != "bad request" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestHandleRequestError_RequestError(t *testing.T) {
	w := httptest.NewRecorder()
	err := &RequestError{
		Status:  http.StatusBadRequest,
		Code:    "INVALID",
		Message: "invalid input",
	}
	HandleRequestError(w, err)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil || resp.Error.Code != "INVALID" {
		t.Errorf("error code = %v", resp.Error)
	}
}

func TestHandleRequestError_GenericError(t *testing.T) {
	w := httptest.NewRecorder()
	HandleRequestError(w, fmt.Errorf("something broke"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestIsRequestError_NotRequestError(t *testing.T) {
	_, ok := IsRequestError(fmt.Errorf("plain error"))
	if ok {
		t.Error("should return false for plain error")
	}
}

func TestDecodeJSONBody_ValidJSON(t *testing.T) {
	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	var dst struct {
		Name string `json:"name"`
	}
	if err := DecodeJSONBody(req, 0, &dst); err != nil {
		t.Fatalf("DecodeJSONBody: %v", err)
	}
	if dst.Name != "test" {
		t.Errorf("Name = %q", dst.Name)
	}
}

// Against a real server, where hijacking and deadlines exist: a websocket
// upgrade counts as 101, and ResponseController reaches the real writer.
func TestInstrument_RealServerHijackAndDeadline(t *testing.T) {
	svc := "t_realsrv"
	m := NewServiceMetrics(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, _ *http.Request) {
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()
		rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: x\r\n\r\n")
		rw.Flush()
	})
	mux.HandleFunc("/deadline", func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Minute)); err != nil {
			t.Errorf("SetWriteDeadline: %v", err)
		}
	})
	srv := httptest.NewServer(m.Instrument(mux, mux))
	defer srv.Close()

	for _, p := range []string{"/ws", "/deadline"} {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		resp.Body.Close()
	}
	out := scrapeService(t, svc)
	for _, want := range []string{
		`unheaded_http_requests_total{method="GET",path="/ws",service="t_realsrv",status="101"} 1`,
		`unheaded_http_requests_total{method="GET",path="/deadline",service="t_realsrv",status="200"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
}

type failingHijacker struct{ *httptest.ResponseRecorder }

func (failingHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("hijack refused")
}

// A hijack that fails switched nothing, so it must not be counted as 101.
func TestStatusRecorder_FailedHijackIsNot101(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: failingHijacker{httptest.NewRecorder()}, status: http.StatusOK}
	if _, _, err := rec.Hijack(); err == nil {
		t.Fatal("error swallowed")
	}
	if rec.status != http.StatusOK {
		t.Errorf("status = %d after failed hijack, want 200", rec.status)
	}
}
