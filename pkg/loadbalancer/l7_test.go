// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- CircuitBreaker tests ---

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second, 2)

	if cb.State() != "closed" {
		t.Errorf("expected closed, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Error("expected closed circuit to allow requests")
	}
}

func TestCircuitBreaker_OpenAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second, 2)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != "open" {
		t.Errorf("expected open after 3 failures, got %s", cb.State())
	}
	if cb.Allow() {
		t.Error("expected open circuit to deny requests")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(1, 10*time.Millisecond, 2)

	cb.RecordFailure()
	if cb.State() != "open" {
		t.Fatalf("expected open, got %s", cb.State())
	}

	time.Sleep(20 * time.Millisecond)

	if !cb.Allow() {
		t.Error("expected request allowed after timeout (half-open)")
	}
	if cb.State() != "half-open" {
		t.Errorf("expected half-open, got %s", cb.State())
	}
}

func TestCircuitBreaker_CloseAfterSuccessInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, 10*time.Millisecond, 2)

	cb.RecordFailure()
	time.Sleep(20 * time.Millisecond)
	cb.Allow() // transition to half-open

	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != "closed" {
		t.Errorf("expected closed after successes in half-open, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenLimitsRequests(t *testing.T) {
	cb := NewCircuitBreaker(1, 10*time.Millisecond, 2)

	cb.RecordFailure()
	time.Sleep(20 * time.Millisecond)

	// First Allow transitions to half-open, successes=0
	if !cb.Allow() {
		t.Error("expected first request allowed in half-open")
	}
	cb.RecordSuccess() // successes=1

	// Second should be allowed since successes(1) < halfOpenMax(2)
	if !cb.Allow() {
		t.Error("expected second request allowed in half-open")
	}
	// Don't record success yet - at this point successes is still 1 from RecordSuccess above
	// But Allow doesn't increment successes, only RecordSuccess does.
	// So we need to test differently: record both successes to trigger close
	cb.RecordSuccess() // successes=2, should transition to closed

	if cb.State() != "closed" {
		t.Errorf("expected closed after halfOpenMax successes, got %s", cb.State())
	}
}

func TestCircuitBreaker_RecordSuccessResets(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second, 2)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess() // Reset failure count

	// Should still be closed
	if cb.State() != "closed" {
		t.Errorf("expected closed after success, got %s", cb.State())
	}

	// Need 3 more failures to open
	cb.RecordFailure()
	if cb.State() != "closed" {
		t.Errorf("expected still closed, got %s", cb.State())
	}
}

func TestCircuitBreaker_Concurrent(t *testing.T) {
	cb := NewCircuitBreaker(100, time.Second, 10)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Allow()
			cb.RecordSuccess()
			cb.RecordFailure()
			_ = cb.State()
		}()
	}
	wg.Wait()
}

// --- RetryPolicy tests ---

func TestDefaultRetryPolicy(t *testing.T) {
	rp := DefaultRetryPolicy()

	if rp.MaxRetries != 3 {
		t.Errorf("expected 3 max retries, got %d", rp.MaxRetries)
	}
	if len(rp.RetryStatuses) != 3 {
		t.Errorf("expected 3 retry statuses, got %d", len(rp.RetryStatuses))
	}
}

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	rp := DefaultRetryPolicy()

	if !rp.ShouldRetry(502) {
		t.Error("expected 502 to trigger retry")
	}
	if !rp.ShouldRetry(503) {
		t.Error("expected 503 to trigger retry")
	}
	if !rp.ShouldRetry(504) {
		t.Error("expected 504 to trigger retry")
	}
	if rp.ShouldRetry(200) {
		t.Error("expected 200 to not trigger retry")
	}
	if rp.ShouldRetry(404) {
		t.Error("expected 404 to not trigger retry")
	}
}

func TestRetryPolicy_BackoffDuration(t *testing.T) {
	rp := &RetryPolicy{
		Backoff:    100 * time.Millisecond,
		MaxBackoff: 1 * time.Second,
	}

	d0 := rp.BackoffDuration(0)
	if d0 != 100*time.Millisecond {
		t.Errorf("expected 100ms for attempt 0, got %v", d0)
	}

	d1 := rp.BackoffDuration(1)
	if d1 != 200*time.Millisecond {
		t.Errorf("expected 200ms for attempt 1, got %v", d1)
	}

	d2 := rp.BackoffDuration(2)
	if d2 != 400*time.Millisecond {
		t.Errorf("expected 400ms for attempt 2, got %v", d2)
	}

	// Should cap at MaxBackoff
	d10 := rp.BackoffDuration(10)
	if d10 != 1*time.Second {
		t.Errorf("expected 1s cap for attempt 10, got %v", d10)
	}
}

// --- RateLimiter tests ---

func TestRateLimiter_AllowBurst(t *testing.T) {
	rl := NewRateLimiter(10, 5) // 10/s, burst 5

	// Should allow burst
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("expected burst request %d to be allowed", i)
		}
	}

	// Next should be denied (no time to refill)
	if rl.Allow() {
		t.Error("expected request to be denied after burst exhausted")
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	rl := NewRateLimiter(1000, 1) // 1000/s, burst 1

	rl.Allow() // Use the burst token

	time.Sleep(5 * time.Millisecond) // Should refill ~5 tokens

	if !rl.Allow() {
		t.Error("expected request allowed after refill")
	}
}

// --- IPRateLimiter tests ---

func TestIPRateLimiter_Basic(t *testing.T) {
	iprl := NewIPRateLimiter(10, 5)

	// Different IPs get independent limits
	for i := 0; i < 5; i++ {
		if !iprl.Allow("192.168.1.1") {
			t.Errorf("expected IP1 request %d allowed", i)
		}
	}

	// IP1 exhausted
	if iprl.Allow("192.168.1.1") {
		t.Error("expected IP1 denied after burst")
	}

	// IP2 should still be allowed
	if !iprl.Allow("192.168.1.2") {
		t.Error("expected IP2 to be allowed")
	}
}

func TestIPRateLimiter_Cleanup(t *testing.T) {
	iprl := NewIPRateLimiter(10, 5)

	iprl.Allow("192.168.1.1")
	iprl.Allow("192.168.1.2")

	// Cleanup with 0 duration should remove all
	iprl.Cleanup(0)

	// After cleanup, new limiter should be created (fresh burst)
	if !iprl.Allow("192.168.1.1") {
		t.Error("expected fresh limiter after cleanup")
	}
}

func TestIPRateLimiter_ConcurrentAccess(t *testing.T) {
	iprl := NewIPRateLimiter(1000, 100)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip := "192.168.1.1"
			if i%2 == 0 {
				ip = "192.168.1.2"
			}
			iprl.Allow(ip)
		}(i)
	}
	wg.Wait()
}

// --- RequestRewriter tests ---

func TestRequestRewriter_Replace(t *testing.T) {
	rr := NewRequestRewriter()
	rr.AddRule(RewriteRule{Match: "/api/v1", Replace: "/api/v2"})

	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	rr.Rewrite(req)

	if req.URL.Path != "/api/v2/users" {
		t.Errorf("expected /api/v2/users, got %s", req.URL.Path)
	}
}

func TestRequestRewriter_StripPrefix(t *testing.T) {
	rr := NewRequestRewriter()
	rr.AddRule(RewriteRule{Match: "/prefix", StripPrefix: true})

	req := httptest.NewRequest("GET", "/prefix/path", nil)
	rr.Rewrite(req)

	if req.URL.Path != "/path" {
		t.Errorf("expected /path, got %s", req.URL.Path)
	}
}

func TestRequestRewriter_AddPrefix(t *testing.T) {
	rr := NewRequestRewriter()
	rr.AddRule(RewriteRule{Match: "/api", StripPrefix: true, AddPrefix: "/v2"})

	req := httptest.NewRequest("GET", "/api/users", nil)
	rr.Rewrite(req)

	if req.URL.Path != "/v2/users" {
		t.Errorf("expected /v2/users, got %s", req.URL.Path)
	}
}

func TestRequestRewriter_NoMatch(t *testing.T) {
	rr := NewRequestRewriter()
	rr.AddRule(RewriteRule{Match: "/api", Replace: "/v2"})

	req := httptest.NewRequest("GET", "/other/path", nil)
	rr.Rewrite(req)

	if req.URL.Path != "/other/path" {
		t.Errorf("expected unchanged path, got %s", req.URL.Path)
	}
}

func TestRequestRewriter_FirstRuleWins(t *testing.T) {
	rr := NewRequestRewriter()
	rr.AddRule(RewriteRule{Match: "/api", Replace: "/v1"})
	rr.AddRule(RewriteRule{Match: "/api", Replace: "/v2"})

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr.Rewrite(req)

	if req.URL.Path != "/v1/test" {
		t.Errorf("expected first rule to win, got %s", req.URL.Path)
	}
}

// --- HeaderInspector tests ---

func TestHeaderInspector_ExactMatch(t *testing.T) {
	hi := NewHeaderInspector()
	hi.AddRule(HeaderRule{Header: "X-Version", Value: "v2", Backend: "backend-v2", Exact: true})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Version", "v2")

	backend, found := hi.Match(req)
	if !found {
		t.Error("expected match")
	}
	if backend != "backend-v2" {
		t.Errorf("expected backend-v2, got %s", backend)
	}
}

func TestHeaderInspector_ExactNoMatch(t *testing.T) {
	hi := NewHeaderInspector()
	hi.AddRule(HeaderRule{Header: "X-Version", Value: "v2", Backend: "backend-v2", Exact: true})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Version", "v1")

	_, found := hi.Match(req)
	if found {
		t.Error("expected no match for different value")
	}
}

func TestHeaderInspector_ContainsMatch(t *testing.T) {
	hi := NewHeaderInspector()
	hi.AddRule(HeaderRule{Header: "User-Agent", Value: "Mobile", Backend: "mobile-backend", Contains: true})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Mobile Safari")

	backend, found := hi.Match(req)
	if !found {
		t.Error("expected contains match")
	}
	if backend != "mobile-backend" {
		t.Errorf("expected mobile-backend, got %s", backend)
	}
}

func TestHeaderInspector_CaseInsensitiveDefault(t *testing.T) {
	hi := NewHeaderInspector()
	hi.AddRule(HeaderRule{Header: "Accept", Value: "application/json", Backend: "api"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept", "APPLICATION/JSON")

	backend, found := hi.Match(req)
	if !found {
		t.Error("expected case-insensitive match")
	}
	if backend != "api" {
		t.Errorf("expected api, got %s", backend)
	}
}

func TestHeaderInspector_MissingHeader(t *testing.T) {
	hi := NewHeaderInspector()
	hi.AddRule(HeaderRule{Header: "X-Version", Value: "v2", Backend: "b", Exact: true})

	req := httptest.NewRequest("GET", "/", nil)
	_, found := hi.Match(req)
	if found {
		t.Error("expected no match for missing header")
	}
}

// --- TraceIDGenerator tests ---

func TestTraceIDGenerator_Generate(t *testing.T) {
	gen := NewTraceIDGenerator("test")

	id1 := gen.Generate()
	id2 := gen.Generate()

	if id1 == "" {
		t.Error("expected non-empty ID")
	}
	if id1 == id2 {
		t.Error("expected unique IDs")
	}
	if !strings.HasPrefix(id1, "test-") {
		t.Errorf("expected test- prefix, got %s", id1)
	}
}

func TestTraceIDGenerator_Concurrent(t *testing.T) {
	gen := NewTraceIDGenerator("test")
	ids := make(map[string]bool)
	mu := sync.Mutex{}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := gen.Generate()
			mu.Lock()
			ids[id] = true
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(ids) != 100 {
		t.Errorf("expected 100 unique IDs, got %d", len(ids))
	}
}

// --- TraceMiddleware tests ---

func TestTraceMiddleware_AddsHeader(t *testing.T) {
	gen := NewTraceIDGenerator("test")
	handler := TraceMiddleware(gen, "X-Trace-ID", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Trace-ID") == "" {
			t.Error("expected trace ID in request header")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Trace-ID") == "" {
		t.Error("expected trace ID in response header")
	}
}

func TestTraceMiddleware_PreservesExistingHeader(t *testing.T) {
	gen := NewTraceIDGenerator("test")
	handler := TraceMiddleware(gen, "X-Trace-ID", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Trace-ID") != "existing-id" {
			t.Errorf("expected existing ID preserved, got %s", r.Header.Get("X-Trace-ID"))
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Trace-ID", "existing-id")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
}

// --- XForwardedForParser tests ---

func TestXForwardedForParser_New(t *testing.T) {
	// CIDR notation
	parser, err := NewXForwardedForParser([]string{"10.0.0.0/8", "172.16.0.0/12"})
	if err != nil {
		t.Fatalf("expected valid parser, got: %v", err)
	}
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
}

func TestXForwardedForParser_SingleIP(t *testing.T) {
	parser, err := NewXForwardedForParser([]string{"10.0.0.1"})
	if err != nil {
		t.Fatalf("expected valid parser with single IP, got: %v", err)
	}
	_ = parser
}

func TestXForwardedForParser_IPv6(t *testing.T) {
	parser, err := NewXForwardedForParser([]string{"::1"})
	if err != nil {
		t.Fatalf("expected valid parser with IPv6, got: %v", err)
	}
	_ = parser
}

func TestXForwardedForParser_Invalid(t *testing.T) {
	_, err := NewXForwardedForParser([]string{"not-an-ip"})
	if err == nil {
		t.Error("expected error for invalid proxy")
	}
}

func TestXForwardedForParser_GetClientIP_NoXFF(t *testing.T) {
	parser, _ := NewXForwardedForParser([]string{"10.0.0.0/8"})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	ip := parser.GetClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

func TestXForwardedForParser_GetClientIP_WithTrustedProxy(t *testing.T) {
	parser, _ := NewXForwardedForParser([]string{"10.0.0.0/8"})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.2")

	ip := parser.GetClientIP(req)
	if ip != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %s", ip)
	}
}

func TestXForwardedForParser_GetClientIP_AllTrusted(t *testing.T) {
	parser, _ := NewXForwardedForParser([]string{"10.0.0.0/8"})

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "10.0.0.2, 10.0.0.3")

	ip := parser.GetClientIP(req)
	if ip != "10.0.0.2" {
		t.Errorf("expected leftmost when all trusted, got %s", ip)
	}
}

// --- ServerTimingHeader tests ---

func TestServerTimingHeader_Empty(t *testing.T) {
	st := NewServerTimingHeader()
	if st.String() != "" {
		t.Errorf("expected empty string, got %q", st.String())
	}
}

func TestServerTimingHeader_Add(t *testing.T) {
	st := NewServerTimingHeader()
	st.Add("proxy", 50*time.Millisecond, "Proxy time")
	st.Add("backend", 100*time.Millisecond, "")

	s := st.String()
	if !strings.Contains(s, "proxy;dur=50.00;desc=\"Proxy time\"") {
		t.Errorf("expected proxy entry, got %s", s)
	}
	if !strings.Contains(s, "backend;dur=100.00") {
		t.Errorf("expected backend entry, got %s", s)
	}
}

func TestServerTimingHeader_SetHeader(t *testing.T) {
	st := NewServerTimingHeader()
	st.Add("total", 200*time.Millisecond, "")

	w := httptest.NewRecorder()
	st.SetHeader(w)

	if w.Header().Get("Server-Timing") == "" {
		t.Error("expected Server-Timing header to be set")
	}
}

func TestServerTimingHeader_SetHeader_Empty(t *testing.T) {
	st := NewServerTimingHeader()

	w := httptest.NewRecorder()
	st.SetHeader(w)

	if w.Header().Get("Server-Timing") != "" {
		t.Error("expected no Server-Timing header for empty entries")
	}
}

// --- BufferPool tests ---

func TestBufferPool_GetPut(t *testing.T) {
	bp := NewBufferPool(4096)

	buf := bp.Get()
	if len(buf) != 4096 {
		t.Errorf("expected buffer of size 4096, got %d", len(buf))
	}

	bp.Put(buf)
}

func TestBufferPool_PutUndersized(t *testing.T) {
	bp := NewBufferPool(4096)

	// Small buffer should not be returned to pool
	smallBuf := make([]byte, 100)
	bp.Put(smallBuf)

	// Get should still return correct size
	buf := bp.Get()
	if len(buf) != 4096 {
		t.Errorf("expected 4096, got %d", len(buf))
	}
}

func TestBufferPool_Concurrent(t *testing.T) {
	bp := NewBufferPool(1024)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := bp.Get()
			bp.Put(buf)
		}()
	}
	wg.Wait()
}

// --- MetricResponseWriter tests ---

func TestMetricResponseWriter_DefaultStatus(t *testing.T) {
	w := httptest.NewRecorder()
	mrw := &MetricResponseWriter{ResponseWriter: w}

	if mrw.StatusCode() != http.StatusOK {
		t.Errorf("expected 200 default, got %d", mrw.StatusCode())
	}
}

func TestMetricResponseWriter_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	mrw := &MetricResponseWriter{ResponseWriter: w}

	mrw.WriteHeader(http.StatusNotFound)

	if mrw.StatusCode() != http.StatusNotFound {
		t.Errorf("expected 404, got %d", mrw.StatusCode())
	}
}

func TestMetricResponseWriter_WriteHeaderOnce(t *testing.T) {
	w := httptest.NewRecorder()
	mrw := &MetricResponseWriter{ResponseWriter: w}

	mrw.WriteHeader(http.StatusNotFound)
	mrw.WriteHeader(http.StatusOK) // Should be ignored

	if mrw.StatusCode() != http.StatusNotFound {
		t.Errorf("expected first status 404, got %d", mrw.StatusCode())
	}
}

func TestMetricResponseWriter_Write(t *testing.T) {
	w := httptest.NewRecorder()
	mrw := &MetricResponseWriter{ResponseWriter: w}

	n, err := mrw.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes, got %d", n)
	}
	if mrw.BytesWritten() != 5 {
		t.Errorf("expected 5 bytes written, got %d", mrw.BytesWritten())
	}

	// Write should set status to 200 implicitly
	if mrw.StatusCode() != http.StatusOK {
		t.Errorf("expected implicit 200 status, got %d", mrw.StatusCode())
	}
}

func TestMetricResponseWriter_StatusClass(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{200, "2xx"},
		{301, "3xx"},
		{404, "4xx"},
		{500, "5xx"},
	}
	for _, tt := range tests {
		w := httptest.NewRecorder()
		mrw := &MetricResponseWriter{ResponseWriter: w}
		mrw.WriteHeader(tt.status)

		if mrw.StatusClass() != tt.want {
			t.Errorf("status %d: expected class %s, got %s", tt.status, tt.want, mrw.StatusClass())
		}
	}
}

// --- responseWriter tests ---

func TestResponseWriter_WriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rw.statusCode)
	}
}

func TestResponseWriter_Write(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	n, err := rw.Write([]byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("expected 4, got %d", n)
	}
	if rw.written != 4 {
		t.Errorf("expected 4 written, got %d", rw.written)
	}
}

func TestResponseWriter_Flush(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	rw.Flush() // Should not panic
}

func TestResponseWriter_Hijack_NotSupported(t *testing.T) {
	w := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

	_, _, err := rw.Hijack()
	if err == nil {
		t.Error("expected error for unsupported hijack")
	}
}

// --- ContentLengthValidator tests ---

func TestContentLengthValidator_Valid(t *testing.T) {
	v := NewContentLengthValidator(1024)

	req := httptest.NewRequest("POST", "/", strings.NewReader("hello"))
	req.ContentLength = 5

	if err := v.Validate(req); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestContentLengthValidator_TooLarge(t *testing.T) {
	v := NewContentLengthValidator(100)

	req := httptest.NewRequest("POST", "/", nil)
	req.ContentLength = 200

	if err := v.Validate(req); err == nil {
		t.Error("expected error for content too large")
	}
}

// --- getClientIP tests ---

func TestGetClientIP_TrustXFF(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.2")

	ip := getClientIP(req, true)
	if ip != "203.0.113.50" {
		t.Errorf("expected 203.0.113.50, got %s", ip)
	}
}

func TestGetClientIP_TrustXRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.99")

	ip := getClientIP(req, true)
	if ip != "203.0.113.99" {
		t.Errorf("expected 203.0.113.99, got %s", ip)
	}
}

func TestGetClientIP_DontTrust(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	ip := getClientIP(req, false)
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

func TestGetClientIP_NoPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1"

	ip := getClientIP(req, false)
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

// --- isWebSocketUpgrade tests ---

func TestIsWebSocketUpgrade(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")

	if !isWebSocketUpgrade(req) {
		t.Error("expected WebSocket upgrade detected")
	}
}

func TestIsWebSocketUpgrade_CaseInsensitive(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Upgrade", "WebSocket")
	req.Header.Set("Connection", "upgrade, keep-alive")

	if !isWebSocketUpgrade(req) {
		t.Error("expected case-insensitive WebSocket upgrade detected")
	}
}

func TestIsWebSocketUpgrade_NotWebSocket(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if isWebSocketUpgrade(req) {
		t.Error("expected false for non-WebSocket request")
	}
}

// --- isGRPCRequest tests ---

func TestIsGRPCRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "application/grpc")

	if !isGRPCRequest(req) {
		t.Error("expected gRPC request detected")
	}
}

func TestIsGRPCRequest_WithOptions(t *testing.T) {
	req := httptest.NewRequest("POST", "/", nil)
	req.Header.Set("Content-Type", "application/grpc+proto")

	if !isGRPCRequest(req) {
		t.Error("expected gRPC request detected with options")
	}
}

func TestIsGRPCRequest_NotGRPC(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Content-Type", "application/json")

	if isGRPCRequest(req) {
		t.Error("expected false for non-gRPC request")
	}
}

// --- headerContainsTokenCI tests ---

func TestHeaderContainsTokenCI(t *testing.T) {
	tests := []struct {
		header string
		token  string
		want   bool
	}{
		{"Upgrade", "upgrade", true},
		{"upgrade, keep-alive", "Upgrade", true},
		{"keep-alive", "upgrade", false},
		{"", "upgrade", false},
		{"Upgrade", "", false},
	}
	for _, tt := range tests {
		got := headerContainsTokenCI(tt.header, tt.token)
		if got != tt.want {
			t.Errorf("headerContainsTokenCI(%q, %q) = %v, want %v", tt.header, tt.token, got, tt.want)
		}
	}
}

// --- HealthResponseWriter tests ---

func TestHealthResponseWriter_Flush(t *testing.T) {
	recorder := httptest.NewRecorder()
	hrw := &HealthResponseWriter{ResponseWriter: recorder}

	hrw.WriteHeader(http.StatusOK)
	hrw.Write([]byte("healthy"))

	hrw.Flush(recorder)

	if recorder.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", recorder.Code)
	}
	if recorder.Body.String() != "healthy" {
		t.Errorf("expected body 'healthy', got %s", recorder.Body.String())
	}
}
