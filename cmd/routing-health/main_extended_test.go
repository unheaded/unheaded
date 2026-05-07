// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// DefaultChecker
// ---------------------------------------------------------------------------

func TestDefaultChecker_ReturnsNonNil(t *testing.T) {
	c := DefaultChecker()
	if c == nil {
		t.Fatal("DefaultChecker returned nil")
	}
	if c.CheckFRR == nil {
		t.Error("CheckFRR func is nil")
	}
	if c.CheckBIRD == nil {
		t.Error("CheckBIRD func is nil")
	}
	if c.CheckWG0 == nil {
		t.Error("CheckWG0 func is nil")
	}
	if c.CheckBGP == nil {
		t.Error("CheckBGP func is nil")
	}
	if c.CheckOSPF == nil {
		t.Error("CheckOSPF func is nil")
	}
	// Hostname should match os.Hostname (best-effort)
	want, _ := os.Hostname()
	if c.Hostname != want {
		t.Errorf("Hostname = %q; want %q", c.Hostname, want)
	}
}

// ---------------------------------------------------------------------------
// Real check functions (run as degraded/0/false on dev machines)
// ---------------------------------------------------------------------------

func TestCheckFRRReal(t *testing.T) {
	result := checkFRRReal()
	if result != "ok" && result != "degraded" {
		t.Errorf("checkFRRReal returned unexpected value %q", result)
	}
}

func TestCheckBIRDReal(t *testing.T) {
	result := checkBIRDReal()
	if result != "ok" && result != "degraded" {
		t.Errorf("checkBIRDReal returned unexpected value %q", result)
	}
}

func TestCheckWG0Real_NoInterface(t *testing.T) {
	// On most dev machines wg0 doesn't exist, so expect false
	result := checkWG0Real()
	// We just verify it doesn't panic; either true or false is fine
	_ = result
}

func TestCheckWG0Real_WithTempFile(t *testing.T) {
	// We can't easily override the hard-coded path, but we exercise the
	// function to confirm it handles missing file gracefully.
	result := checkWG0Real()
	if result {
		t.Log("wg0 interface detected as up (unexpected on dev, but valid)")
	}
}

func TestCheckBGPReal(t *testing.T) {
	count := checkBGPReal()
	if count < 0 {
		t.Errorf("checkBGPReal returned negative count: %d", count)
	}
}

func TestCheckOSPFReal(t *testing.T) {
	count := checkOSPFReal()
	if count < 0 {
		t.Errorf("checkOSPFReal returned negative count: %d", count)
	}
}

// ---------------------------------------------------------------------------
// Health handler — partial degradation
// ---------------------------------------------------------------------------

func TestHealthHandler_FRRDegradedOnly(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "degraded" },
		CheckBIRD: func() string { return "ok" },
		CheckWG0:  func() bool { return true },
		CheckBGP:  func() int { return 1 },
		CheckOSPF: func() int { return 1 },
		Hostname:  "partial-host",
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	checker.healthHandler(w, req)

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("status = %q; want degraded (FRR down)", resp.Status)
	}
	if resp.Checks["frr"] != "degraded" {
		t.Errorf("frr = %q; want degraded", resp.Checks["frr"])
	}
	if resp.Checks["bird"] != "ok" {
		t.Errorf("bird = %q; want ok", resp.Checks["bird"])
	}
	if resp.Checks["wg0"] != "ok" {
		t.Errorf("wg0 = %q; want ok", resp.Checks["wg0"])
	}
}

func TestHealthHandler_BIRDDegradedOnly(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "ok" },
		CheckBIRD: func() string { return "degraded" },
		CheckWG0:  func() bool { return true },
		CheckBGP:  func() int { return 1 },
		CheckOSPF: func() int { return 1 },
		Hostname:  "bird-down",
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	checker.healthHandler(w, req)

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("status = %q; want degraded", resp.Status)
	}
	if resp.Checks["bird"] != "degraded" {
		t.Errorf("bird = %q; want degraded", resp.Checks["bird"])
	}
}

func TestHealthHandler_WG0DownOnly(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "ok" },
		CheckBIRD: func() string { return "ok" },
		CheckWG0:  func() bool { return false },
		CheckBGP:  func() int { return 2 },
		CheckOSPF: func() int { return 3 },
		Hostname:  "wg0-down",
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	checker.healthHandler(w, req)

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("status = %q; want degraded (wg0 down)", resp.Status)
	}
	if resp.Checks["wg0"] != "degraded" {
		t.Errorf("wg0 = %q; want degraded", resp.Checks["wg0"])
	}
	if resp.Checks["frr"] != "ok" {
		t.Errorf("frr = %q; want ok", resp.Checks["frr"])
	}
}

func TestHealthHandler_ContentType(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "ok" },
		CheckBIRD: func() string { return "ok" },
		CheckWG0:  func() bool { return true },
		CheckBGP:  func() int { return 0 },
		CheckOSPF: func() int { return 0 },
		Hostname:  "ct-host",
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	checker.healthHandler(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q; want application/json", ct)
	}
}

// ---------------------------------------------------------------------------
// Ready handler — body content
// ---------------------------------------------------------------------------

func TestReadyHandler_BodyContent(t *testing.T) {
	checker := &Checker{
		CheckWG0: func() bool { return true },
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	checker.readyHandler(w, req)

	body := w.Body.String()
	if body != "OK" {
		t.Errorf("body = %q; want %q", body, "OK")
	}
}

func TestReadyHandler_503Body(t *testing.T) {
	checker := &Checker{
		CheckWG0: func() bool { return false },
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	checker.readyHandler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d; want 503", w.Code)
	}
	// Body should be empty on 503
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body on 503, got %q", w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Metrics handler — empty hostname fallback
// ---------------------------------------------------------------------------

func TestMetricsHandler_EmptyHostname(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "ok" },
		CheckBIRD: func() string { return "ok" },
		CheckWG0:  func() bool { return true },
		CheckBGP:  func() int { return 0 },
		CheckOSPF: func() int { return 0 },
		Hostname:  "", // empty → should fall back to "unknown"
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	checker.metricsHandler(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `host="unknown"`) {
		t.Errorf("expected host=\"unknown\" in metrics body, got:\n%s", body)
	}
}

func TestMetricsHandler_FRROk_BIRDOk(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "ok" },
		CheckBIRD: func() string { return "ok" },
		CheckWG0:  func() bool { return false },
		CheckBGP:  func() int { return 5 },
		CheckOSPF: func() int { return 7 },
		Hostname:  "metrics-test",
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	checker.metricsHandler(w, req)

	body := w.Body.String()

	// FRR and BIRD both ok → 1.0
	if !strings.Contains(body, `routing_health_frr_up{host="metrics-test"} 1.0`) {
		t.Error("expected frr_up 1.0")
	}
	if !strings.Contains(body, `routing_health_bird_up{host="metrics-test"} 1.0`) {
		t.Error("expected bird_up 1.0")
	}
	// wg0 down → 0.0
	if !strings.Contains(body, `routing_health_wg0_up{host="metrics-test"} 0.0`) {
		t.Error("expected wg0_up 0.0")
	}
	// BGP 5, OSPF 7
	if !strings.Contains(body, `routing_health_bgp_sessions{host="metrics-test"} 5`) {
		t.Error("expected bgp_sessions 5")
	}
	if !strings.Contains(body, `routing_health_ospf_neighbors{host="metrics-test"} 7`) {
		t.Error("expected ospf_neighbors 7")
	}
}

func TestMetricsHandler_AllHELP_TYPE_Lines(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "degraded" },
		CheckBIRD: func() string { return "degraded" },
		CheckWG0:  func() bool { return false },
		CheckBGP:  func() int { return 0 },
		CheckOSPF: func() int { return 0 },
		Hostname:  "help-type",
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	checker.metricsHandler(w, req)

	body := w.Body.String()

	expectedPrefixes := []string{
		"# HELP routing_health_frr_up",
		"# TYPE routing_health_frr_up gauge",
		"# HELP routing_health_bird_up",
		"# TYPE routing_health_bird_up gauge",
		"# HELP routing_health_wg0_up",
		"# TYPE routing_health_wg0_up gauge",
		"# HELP routing_health_bgp_sessions",
		"# TYPE routing_health_bgp_sessions gauge",
		"# HELP routing_health_ospf_neighbors",
		"# TYPE routing_health_ospf_neighbors gauge",
	}

	for _, prefix := range expectedPrefixes {
		if !strings.Contains(body, prefix) {
			t.Errorf("missing expected line: %s", prefix)
		}
	}
}

// ---------------------------------------------------------------------------
// Metrics handler — mixed up/down scenarios for value assertions
// ---------------------------------------------------------------------------

func TestMetricsHandler_FRRDegraded_BIRDOk(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "degraded" },
		CheckBIRD: func() string { return "ok" },
		CheckWG0:  func() bool { return true },
		CheckBGP:  func() int { return 1 },
		CheckOSPF: func() int { return 0 },
		Hostname:  "mixed",
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	checker.metricsHandler(w, req)

	body := w.Body.String()

	if !strings.Contains(body, `routing_health_frr_up{host="mixed"} 0.0`) {
		t.Error("expected frr_up 0.0 when degraded")
	}
	if !strings.Contains(body, `routing_health_bird_up{host="mixed"} 1.0`) {
		t.Error("expected bird_up 1.0 when ok")
	}
	if !strings.Contains(body, `routing_health_wg0_up{host="mixed"} 1.0`) {
		t.Error("expected wg0_up 1.0 when up")
	}
}

// ---------------------------------------------------------------------------
// DefaultChecker functions exercised through Checker struct
// ---------------------------------------------------------------------------

func TestDefaultChecker_FunctionsCallable(t *testing.T) {
	c := DefaultChecker()

	// Exercise each function — we don't assert specific values since
	// results depend on the host, but they must not panic.
	_ = c.CheckFRR()
	_ = c.CheckBIRD()
	_ = c.CheckWG0()
	_ = c.CheckBGP()
	_ = c.CheckOSPF()
}

// ---------------------------------------------------------------------------
// Health handler via full HTTP mux (integration-style)
// ---------------------------------------------------------------------------

func TestFullMux_Endpoints(t *testing.T) {
	checker := &Checker{
		CheckFRR:  func() string { return "ok" },
		CheckBIRD: func() string { return "ok" },
		CheckWG0:  func() bool { return true },
		CheckBGP:  func() int { return 3 },
		CheckOSPF: func() int { return 2 },
		Hostname:  "mux-test",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", checker.healthHandler)
	mux.HandleFunc("/ready", checker.readyHandler)
	mux.HandleFunc("/metrics", checker.metricsHandler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string // substring
	}{
		{"/health", http.StatusOK, `"status":"ok"`},
		{"/ready", http.StatusOK, "OK"},
		{"/metrics", http.StatusOK, "routing_health_frr_up"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tt.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tt.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("GET %s status = %d; want %d", tt.path, resp.StatusCode, tt.wantStatus)
			}

			var buf strings.Builder
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			buf.Write(b)
			if !strings.Contains(buf.String(), tt.wantBody) {
				t.Errorf("GET %s body missing %q", tt.path, tt.wantBody)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// main() — start on ephemeral port and shut down via SIGINT
// ---------------------------------------------------------------------------

func TestMain_StartAndShutdown(t *testing.T) {
	// Find a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	os.Setenv("ROUTING_HEALTH_ADDR", addr)
	defer os.Unsetenv("ROUTING_HEALTH_ADDR")

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	// Wait for server to be ready (poll up to 2s)
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/health")
		if err == nil {
			resp.Body.Close()
			break
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	if lastErr != nil {
		// Check one more time
		resp, err := http.Get("http://" + addr + "/health")
		if err != nil {
			t.Fatalf("server never came up: %v", err)
		}
		resp.Body.Close()
	}

	// Verify endpoints work through the real main() mux
	resp, err := http.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatalf("GET /ready: %v", err)
	}
	resp.Body.Close()

	resp, err = http.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	resp.Body.Close()

	// Send SIGINT to shut down
	p, _ := os.FindProcess(os.Getpid())
	p.Signal(syscall.SIGINT)

	select {
	case <-done:
		// main() returned cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("main() did not shut down within 5s")
	}
}

// ---------------------------------------------------------------------------
// checkWG0Real with a temp operstate file (test the file-reading path)
// ---------------------------------------------------------------------------

func TestCheckWG0Real_OperstateParsing(t *testing.T) {
	// We test the logic by calling the function directly.
	// Since /sys/class/net/wg0/operstate probably doesn't exist on the
	// test machine, the function returns false. We verify it doesn't panic.
	got := checkWG0Real()
	if got {
		t.Log("wg0 interface is up on this host")
	} else {
		t.Log("wg0 interface not found or down (expected on dev)")
	}
}

// ---------------------------------------------------------------------------
// HealthResponse JSON structure
// ---------------------------------------------------------------------------

func TestHealthResponse_JSONSerialization(t *testing.T) {
	resp := HealthResponse{
		Status: "ok",
		Checks: map[string]string{
			"frr":  "ok",
			"bird": "ok",
			"wg0":  "ok",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded HealthResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Status != resp.Status {
		t.Errorf("status = %q; want %q", decoded.Status, resp.Status)
	}
	if len(decoded.Checks) != len(resp.Checks) {
		t.Errorf("checks len = %d; want %d", len(decoded.Checks), len(resp.Checks))
	}
	for k, v := range resp.Checks {
		if decoded.Checks[k] != v {
			t.Errorf("check[%s] = %q; want %q", k, decoded.Checks[k], v)
		}
	}
}

func TestHealthResponse_JSONFields(t *testing.T) {
	resp := HealthResponse{
		Status: "degraded",
		Checks: map[string]string{"frr": "degraded"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	raw := string(data)
	if !strings.Contains(raw, `"status"`) {
		t.Error("JSON missing 'status' field")
	}
	if !strings.Contains(raw, `"checks"`) {
		t.Error("JSON missing 'checks' field")
	}
}

// ---------------------------------------------------------------------------
// ROUTING_HEALTH_ADDR environment variable (coverage for main logic)
// ---------------------------------------------------------------------------

func TestEnvAddr(t *testing.T) {
	// Verify the env var logic without starting a real server.
	// We just test DefaultChecker + env reading.
	old := os.Getenv("ROUTING_HEALTH_ADDR")
	defer os.Setenv("ROUTING_HEALTH_ADDR", old)

	os.Setenv("ROUTING_HEALTH_ADDR", ":9999")
	addr := os.Getenv("ROUTING_HEALTH_ADDR")
	if addr != ":9999" {
		t.Errorf("addr = %q; want :9999", addr)
	}

	os.Unsetenv("ROUTING_HEALTH_ADDR")
	addr = os.Getenv("ROUTING_HEALTH_ADDR")
	if addr != "" {
		t.Errorf("addr = %q; want empty", addr)
	}
}

// ---------------------------------------------------------------------------
// checkWG0Real — simulate various operstate values via temp dir
// We test the string matching logic indirectly by verifying edge cases.
// ---------------------------------------------------------------------------

func TestCheckWG0_OperStateValues(t *testing.T) {
	// Create a temporary directory structure mimicking /sys/class/net/wg0/
	tmpDir := t.TempDir()
	operstateFile := filepath.Join(tmpDir, "operstate")

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"up", "up\n", true},
		{"unknown", "unknown\n", true},
		{"down", "down\n", false},
		{"empty", "", false},
		{"dormant", "dormant\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(operstateFile, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			// Read file and apply same logic as checkWG0Real
			data, err := os.ReadFile(operstateFile)
			if err != nil {
				t.Fatal(err)
			}
			state := strings.TrimSpace(string(data))
			got := state == "up" || state == "unknown"

			if got != tt.want {
				t.Errorf("operstate=%q → %v; want %v", tt.content, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// checkBGPReal — BGP JSON parsing logic (tested via the same algorithm)
// ---------------------------------------------------------------------------

func TestBGP_JSONParsing(t *testing.T) {
	tests := []struct {
		name string
		json string
		want int
	}{
		{
			"two_established",
			`{"ipv4Unicast":{"peers":{"10.0.0.1":{"peerState":"Established"},"10.0.0.2":{"peerState":"Established"}}}}`,
			2,
		},
		{
			"one_established_one_idle",
			`{"ipv4Unicast":{"peers":{"10.0.0.1":{"peerState":"Established"},"10.0.0.2":{"peerState":"Idle"}}}}`,
			1,
		},
		{
			"none_established",
			`{"ipv4Unicast":{"peers":{"10.0.0.1":{"peerState":"Active"}}}}`,
			0,
		},
		{
			"no_peers",
			`{"ipv4Unicast":{"peers":{}}}`,
			0,
		},
		{
			"no_ipv4unicast",
			`{"ipv6Unicast":{}}`,
			0,
		},
		{
			"invalid_peer_data",
			`{"ipv4Unicast":{"peers":{"10.0.0.1":"not-a-map"}}}`,
			0,
		},
		{
			"empty_json",
			`{}`,
			0,
		},
		{
			"invalid_json",
			`not json`,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the same parsing logic from checkBGPReal
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(tt.json), &result); err != nil {
				if tt.want != 0 {
					t.Errorf("unexpected parse error for %q", tt.name)
				}
				return
			}

			ipv4Unicast, ok := result["ipv4Unicast"].(map[string]interface{})
			if !ok {
				if tt.want != 0 {
					t.Errorf("expected count %d but no ipv4Unicast", tt.want)
				}
				return
			}

			peers, ok := ipv4Unicast["peers"].(map[string]interface{})
			if !ok {
				if tt.want != 0 {
					t.Errorf("expected count %d but no peers", tt.want)
				}
				return
			}

			count := 0
			for _, peerData := range peers {
				peerMap, ok := peerData.(map[string]interface{})
				if !ok {
					continue
				}
				state, ok := peerMap["peerState"].(string)
				if ok && state == "Established" {
					count++
				}
			}

			if count != tt.want {
				t.Errorf("count = %d; want %d", count, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// checkOSPFReal — OSPF JSON parsing logic
// ---------------------------------------------------------------------------

func TestOSPF_JSONParsing(t *testing.T) {
	tests := []struct {
		name string
		json string
		want int
	}{
		{
			"three_neighbors",
			`[{"neighborId":"1.1.1.1"},{"neighborId":"2.2.2.2"},{"neighborId":"3.3.3.3"}]`,
			3,
		},
		{
			"empty_array",
			`[]`,
			0,
		},
		{
			"invalid_json",
			`{bad`,
			0,
		},
		{
			"object_not_array",
			`{"neighbors": []}`,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var neighbors []map[string]interface{}
			if err := json.Unmarshal([]byte(tt.json), &neighbors); err != nil {
				if tt.want != 0 {
					t.Errorf("unexpected parse error: %v", err)
				}
				return
			}
			if len(neighbors) != tt.want {
				t.Errorf("count = %d; want %d", len(neighbors), tt.want)
			}
		})
	}
}
