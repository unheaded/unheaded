// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package firewall

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

func TestNewService(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.log != log {
		t.Error("expected logger to be set")
	}
	if svc.policy == nil {
		t.Fatal("expected default policy to be set")
	}
	if svc.policy.DefaultAction != ActionBlock {
		t.Errorf("expected default action BLOCK, got %s", svc.policy.DefaultAction)
	}
	if svc.policy.TimeoutNew != DefaultTimeoutNew {
		t.Errorf("expected timeout_new %d, got %d", DefaultTimeoutNew, svc.policy.TimeoutNew)
	}
	if svc.policy.TimeoutEstablished != DefaultTimeoutEstablished {
		t.Errorf("expected timeout_established %d, got %d", DefaultTimeoutEstablished, svc.policy.TimeoutEstablished)
	}
	if svc.policy.TimeoutClosing != DefaultTimeoutClosing {
		t.Errorf("expected timeout_closing %d, got %d", DefaultTimeoutClosing, svc.policy.TimeoutClosing)
	}
	if len(svc.connections) != 0 {
		t.Error("expected empty connections map")
	}
}

func TestPolicyReload(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("valid policy reload", func(t *testing.T) {
		newPolicy := `{
			"default_action": "ALLOW",
			"timeout_new": 60,
			"timeout_established": 1200,
			"timeout_closing": 20,
			"rules": [
				{
					"src_service": "web",
					"dst_service": "api",
					"ports": [443, 8443],
					"protocol": "tcp",
					"action": "ALLOW",
					"requires_encryption": true
				}
			]
		}`

		req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/reload", strings.NewReader(newPolicy))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		p := svc.GetPolicy()
		if p.DefaultAction != ActionAllow {
			t.Errorf("expected ALLOW, got %s", p.DefaultAction)
		}
		if len(p.Rules) != 1 {
			t.Errorf("expected 1 rule, got %d", len(p.Rules))
		}
		if p.TimeoutNew != 60 {
			t.Errorf("expected timeout_new 60, got %d", p.TimeoutNew)
		}
	})

	t.Run("invalid action rejected", func(t *testing.T) {
		badPolicy := `{"default_action": "INVALID", "rules": []}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/reload", strings.NewReader(badPolicy))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	t.Run("GET on reload endpoint rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/policy/reload", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status 405, got %d", w.Code)
		}
	})

	t.Run("GET current policy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/policy", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var policy FirewallPolicy
		if err := json.NewDecoder(w.Body).Decode(&policy); err != nil {
			t.Fatalf("failed to decode policy: %v", err)
		}
		if policy.DefaultAction == "" {
			t.Error("expected non-empty default_action in response")
		}
	})
}

func TestConntrackAggregation(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	// Add some connections.
	now := time.Now()
	entries := []*ConntrackEntry{
		{SrcIP: "10.0.0.1", DstIP: "10.0.0.100", SrcPort: 40000, DstPort: 443, Protocol: "tcp", State: StateEstablished, PacketCount: 1000, ByteCount: 50000, CreatedAt: now, LastSeen: now},
		{SrcIP: "10.0.0.1", DstIP: "10.0.0.101", SrcPort: 40001, DstPort: 80, Protocol: "tcp", State: StateNew, PacketCount: 5, ByteCount: 200, CreatedAt: now, LastSeen: now},
		{SrcIP: "10.0.0.2", DstIP: "10.0.0.100", SrcPort: 40002, DstPort: 443, Protocol: "tcp", State: StateEstablished, PacketCount: 500, ByteCount: 25000, CreatedAt: now, LastSeen: now},
		{SrcIP: "10.0.0.3", DstIP: "10.0.0.102", SrcPort: 40003, DstPort: 8080, Protocol: "tcp", State: StateClosing, PacketCount: 200, ByteCount: 10000, CreatedAt: now, LastSeen: now},
	}
	for _, entry := range entries {
		svc.AddConnection(entry)
	}

	// Trigger aggregation.
	ctx := context.Background()
	svc.computeConntrackStats(ctx)

	stats := svc.GetConntrackStats()

	if stats.TotalConnections != 4 {
		t.Errorf("expected 4 total connections, got %d", stats.TotalConnections)
	}
	if stats.ActiveByState[StateEstablished] != 2 {
		t.Errorf("expected 2 ESTABLISHED, got %d", stats.ActiveByState[StateEstablished])
	}
	if stats.ActiveByState[StateNew] != 1 {
		t.Errorf("expected 1 NEW, got %d", stats.ActiveByState[StateNew])
	}
	if stats.ActiveByState[StateClosing] != 1 {
		t.Errorf("expected 1 CLOSING, got %d", stats.ActiveByState[StateClosing])
	}
	if len(stats.TopSrcIPs) == 0 {
		t.Error("expected at least one top source IP")
	}
	// 10.0.0.1 should be the top source IP (2 connections).
	if stats.TopSrcIPs[0].IP != "10.0.0.1" || stats.TopSrcIPs[0].Count != 2 {
		t.Errorf("expected top source IP 10.0.0.1 with count 2, got %s:%d", stats.TopSrcIPs[0].IP, stats.TopSrcIPs[0].Count)
	}
}

func TestPolicyRuleMatching(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	svc.SetPolicy(&FirewallPolicy{
		DefaultAction:      ActionBlock,
		TimeoutNew:         DefaultTimeoutNew,
		TimeoutEstablished: DefaultTimeoutEstablished,
		TimeoutClosing:     DefaultTimeoutClosing,
		Rules: []PolicyRule{
			{SrcService: "web", DstService: "api", Ports: []uint16{443, 8443}, Protocol: "tcp", Action: ActionAllow},
			{SrcService: "", DstService: "dns", Ports: []uint16{53}, Protocol: "udp", Action: ActionAllow},
			{SrcService: "malware", DstService: "", Ports: nil, Protocol: "", Action: ActionBlock},
		},
	})

	tests := []struct {
		name     string
		src      string
		dst      string
		port     uint16
		proto    string
		expected string
	}{
		{"web to api on 443", "web", "api", 443, "tcp", ActionAllow},
		{"web to api on 8443", "web", "api", 8443, "tcp", ActionAllow},
		{"web to api on 80", "web", "api", 80, "tcp", ActionBlock},       // Port not matched.
		{"any to dns on 53 udp", "anything", "dns", 53, "udp", ActionAllow},
		{"malware blocked", "malware", "anywhere", 9999, "tcp", ActionBlock},
		{"unknown traffic blocked", "unknown", "unknown", 1234, "tcp", ActionBlock}, // Default action.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.MatchRule(tt.src, tt.dst, tt.port, tt.proto)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestHealthEndpoints(t *testing.T) {
	log := newTestLogger()
	svc := NewService(log, nil)

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	for _, endpoint := range []string{"/health", "/ready"} {
		t.Run(endpoint, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200 for %s, got %d", endpoint, w.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestAddConnection
// ---------------------------------------------------------------------------

func TestAddConnection(t *testing.T) {
	svc := NewService(newTestLogger(), nil)

	t.Run("nil entry does not panic", func(t *testing.T) {
		svc.AddConnection(nil)
	})

	t.Run("add and retrieve connection", func(t *testing.T) {
		now := time.Now()
		svc.AddConnection(&ConntrackEntry{
			SrcIP:       "10.0.0.1",
			DstIP:       "10.0.0.2",
			SrcPort:     40000,
			DstPort:     443,
			Protocol:    "tcp",
			State:       StateNew,
			PacketCount: 10,
			ByteCount:   500,
			CreatedAt:   now,
			LastSeen:    now,
		})

		stats := svc.GetConntrackStats()
		// After adding one connection but before compute, initial stats still show 0
		// We need to compute to see the update
		svc.computeConntrackStats(context.Background())
		stats = svc.GetConntrackStats()
		if stats.TotalConnections != 1 {
			t.Errorf("TotalConnections = %d, want 1", stats.TotalConnections)
		}
	})
}

// ---------------------------------------------------------------------------
// TestSetAndGetPolicy
// ---------------------------------------------------------------------------

func TestSetAndGetPolicy(t *testing.T) {
	svc := NewService(newTestLogger(), nil)

	t.Run("default policy", func(t *testing.T) {
		p := svc.GetPolicy()
		if p.DefaultAction != ActionBlock {
			t.Errorf("DefaultAction = %s, want BLOCK", p.DefaultAction)
		}
		if len(p.Rules) != 0 {
			t.Errorf("Rules = %d, want 0", len(p.Rules))
		}
	})

	t.Run("set nil policy ignored", func(t *testing.T) {
		svc.SetPolicy(nil)
		p := svc.GetPolicy()
		if p.DefaultAction != ActionBlock {
			t.Error("SetPolicy(nil) should not change existing policy")
		}
	})

	t.Run("set new policy", func(t *testing.T) {
		svc.SetPolicy(&FirewallPolicy{
			DefaultAction:      ActionAllow,
			TimeoutNew:         60,
			TimeoutEstablished: 1200,
			TimeoutClosing:     20,
			Rules: []PolicyRule{
				{SrcService: "web", DstService: "api", Action: ActionAllow},
			},
		})
		p := svc.GetPolicy()
		if p.DefaultAction != ActionAllow {
			t.Errorf("DefaultAction = %s, want ALLOW", p.DefaultAction)
		}
		if len(p.Rules) != 1 {
			t.Errorf("Rules = %d, want 1", len(p.Rules))
		}
	})

	t.Run("GetPolicy returns copy", func(t *testing.T) {
		p := svc.GetPolicy()
		p.Rules = append(p.Rules, PolicyRule{SrcService: "extra"})
		p2 := svc.GetPolicy()
		if len(p2.Rules) != 1 {
			t.Errorf("modifying returned policy should not affect service: got %d rules", len(p2.Rules))
		}
	})
}

// ---------------------------------------------------------------------------
// TestWotanDrops
// ---------------------------------------------------------------------------

func TestWotanDrops(t *testing.T) {
	svc := NewService(newTestLogger(), nil)

	if drops := svc.WotanDrops(); drops != 0 {
		t.Errorf("initial WotanDrops = %d, want 0", drops)
	}

	// computeConntrackStats with nil wotan and connections should increment drops
	svc.AddConnection(&ConntrackEntry{
		SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 1, DstPort: 80, Protocol: "tcp", State: StateNew,
		CreatedAt: time.Now(), LastSeen: time.Now(),
	})
	svc.computeConntrackStats(context.Background())

	if drops := svc.WotanDrops(); drops != 1 {
		t.Errorf("WotanDrops = %d, want 1", drops)
	}
}

// ---------------------------------------------------------------------------
// TestHTTPConnections
// ---------------------------------------------------------------------------

func TestHTTPConnections(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	now := time.Now()

	// Add connections with different packet counts
	for i := 0; i < 5; i++ {
		svc.AddConnection(&ConntrackEntry{
			SrcIP:       "10.0.0.1",
			DstIP:       "10.0.0.2",
			SrcPort:     uint16(40000 + i),
			DstPort:     443,
			Protocol:    "tcp",
			State:       StateEstablished,
			PacketCount: uint64((5 - i) * 100),
			ByteCount:   uint64((5 - i) * 1000),
			CreatedAt:   now,
			LastSeen:    now,
		})
	}

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("GET all connections", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/connections", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)
		total := int(resp["total"].(float64))
		if total != 5 {
			t.Errorf("total = %d, want 5", total)
		}
	})

	t.Run("pagination with offset and limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/connections?offset=2&limit=2", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)
		conns := resp["connections"].([]interface{})
		if len(conns) != 2 {
			t.Errorf("connections count = %d, want 2", len(conns))
		}
	})

	t.Run("offset beyond total returns empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/connections?offset=100", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)
		// connections should be null or empty
		conns := resp["connections"]
		if conns != nil {
			arr, ok := conns.([]interface{})
			if ok && len(arr) > 0 {
				t.Errorf("expected empty connections, got %d", len(arr))
			}
		}
	})

	t.Run("negative offset treated as 0", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/connections?offset=-5", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(w.Body).Decode(&resp)
		offset := int(resp["offset"].(float64))
		if offset != 0 {
			t.Errorf("offset = %d, want 0", offset)
		}
	})

	t.Run("POST not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/connections", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPStats
// ---------------------------------------------------------------------------

func TestHTTPStats(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	t.Run("empty stats", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var stats BlockStats
		json.NewDecoder(w.Body).Decode(&stats)
		if stats.BlockRate != 0.0 {
			t.Errorf("BlockRate = %f, want 0.0", stats.BlockRate)
		}
	})

	t.Run("stats after matching rules", func(t *testing.T) {
		svc.SetPolicy(&FirewallPolicy{
			DefaultAction: ActionBlock,
			Rules: []PolicyRule{
				{SrcService: "web", DstService: "api", Action: ActionAllow},
			},
		})
		svc.MatchRule("web", "api", 443, "tcp")  // allow
		svc.MatchRule("evil", "api", 443, "tcp") // block (default)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		var stats BlockStats
		json.NewDecoder(w.Body).Decode(&stats)
		if stats.BlockRate <= 0 {
			t.Error("expected non-zero block rate")
		}
		if stats.AllowRate <= 0 {
			t.Error("expected non-zero allow rate")
		}
	})

	t.Run("POST not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/stats", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want 405", w.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHTTPMetrics
// ---------------------------------------------------------------------------

func TestHTTPMetrics(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	svc.AddConnection(&ConntrackEntry{
		SrcIP: "10.0.0.1", DstIP: "10.0.0.2", SrcPort: 1, DstPort: 80, Protocol: "tcp", State: StateNew,
		CreatedAt: time.Now(), LastSeen: time.Now(),
	})

	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	for _, expected := range []string{
		"unheaded_firewall_blocked_total",
		"unheaded_firewall_allowed_total",
		"unheaded_firewall_connections 1",
		"unheaded_firewall_wotan_drops_total",
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("metrics missing %q", expected)
		}
	}
}

// ---------------------------------------------------------------------------
// TestPolicyReloadValidation
// ---------------------------------------------------------------------------

func TestPolicyReloadValidation(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "invalid JSON",
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid default action",
			body:       `{"default_action": "MAYBE"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid rule action",
			body:       `{"default_action": "ALLOW", "rules": [{"action": "MAYBE"}]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "defaults applied for zero timeouts",
			body:       `{"default_action": "BLOCK", "timeout_new": 0, "timeout_established": 0, "timeout_closing": 0, "rules": []}`,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/policy/reload", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}

	// Verify defaults were applied on the zero-timeout case
	p := svc.GetPolicy()
	if p.TimeoutNew != DefaultTimeoutNew {
		t.Errorf("TimeoutNew = %d, want %d", p.TimeoutNew, DefaultTimeoutNew)
	}
	if p.TimeoutEstablished != DefaultTimeoutEstablished {
		t.Errorf("TimeoutEstablished = %d, want %d", p.TimeoutEstablished, DefaultTimeoutEstablished)
	}
	if p.TimeoutClosing != DefaultTimeoutClosing {
		t.Errorf("TimeoutClosing = %d, want %d", p.TimeoutClosing, DefaultTimeoutClosing)
	}
}

// ---------------------------------------------------------------------------
// TestHTTPPolicyGetMethodNotAllowed
// ---------------------------------------------------------------------------

func TestHTTPPolicyGetMethodNotAllowed(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ---------------------------------------------------------------------------
// TestMatchRuleDefaultAllow
// ---------------------------------------------------------------------------

func TestMatchRuleDefaultAllow(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	svc.SetPolicy(&FirewallPolicy{
		DefaultAction: ActionAllow,
		Rules:         []PolicyRule{},
	})

	result := svc.MatchRule("any", "any", 80, "tcp")
	if result != ActionAllow {
		t.Errorf("expected ALLOW (default), got %s", result)
	}
}

// ---------------------------------------------------------------------------
// TestMatchRuleProtocolFiltering
// ---------------------------------------------------------------------------

func TestMatchRuleProtocolFiltering(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	svc.SetPolicy(&FirewallPolicy{
		DefaultAction: ActionBlock,
		Rules: []PolicyRule{
			{Protocol: "udp", Action: ActionAllow},
		},
	})

	t.Run("matching protocol", func(t *testing.T) {
		result := svc.MatchRule("any", "any", 53, "udp")
		if result != ActionAllow {
			t.Errorf("expected ALLOW, got %s", result)
		}
	})

	t.Run("non-matching protocol", func(t *testing.T) {
		result := svc.MatchRule("any", "any", 53, "tcp")
		if result != ActionBlock {
			t.Errorf("expected BLOCK, got %s", result)
		}
	})
}

// ---------------------------------------------------------------------------
// TestTopNFunctions
// ---------------------------------------------------------------------------

func TestTopNFunctions(t *testing.T) {
	t.Run("topNIPs fewer than N", func(t *testing.T) {
		counts := map[string]int{"10.0.0.1": 5, "10.0.0.2": 3}
		result := topNIPs(counts, 10)
		if len(result) != 2 {
			t.Errorf("expected 2 results, got %d", len(result))
		}
		if result[0].IP != "10.0.0.1" || result[0].Count != 5 {
			t.Errorf("first = %v, want 10.0.0.1:5", result[0])
		}
	})

	t.Run("topNIPs more than N", func(t *testing.T) {
		counts := map[string]int{
			"10.0.0.1": 10, "10.0.0.2": 8, "10.0.0.3": 6,
		}
		result := topNIPs(counts, 2)
		if len(result) != 2 {
			t.Errorf("expected 2 results, got %d", len(result))
		}
	})

	t.Run("topNIPs empty", func(t *testing.T) {
		result := topNIPs(map[string]int{}, 10)
		if len(result) != 0 {
			t.Errorf("expected 0 results, got %d", len(result))
		}
	})

	t.Run("topNServices fewer than N", func(t *testing.T) {
		counts := map[string]int{"web:443": 5, "api:8080": 3}
		result := topNServices(counts, 10)
		if len(result) != 2 {
			t.Errorf("expected 2 results, got %d", len(result))
		}
	})

	t.Run("topNServices more than N", func(t *testing.T) {
		counts := map[string]int{
			"web:443": 10, "api:8080": 8, "db:5432": 6,
		}
		result := topNServices(counts, 2)
		if len(result) != 2 {
			t.Errorf("expected 2 results, got %d", len(result))
		}
	})
}

// ---------------------------------------------------------------------------
// TestConnKey
// ---------------------------------------------------------------------------

func TestConnKey(t *testing.T) {
	key := connKey("10.0.0.1", "10.0.0.2", 40000, 443, "tcp")
	expected := "10.0.0.1:40000->10.0.0.2:443/tcp"
	if key != expected {
		t.Errorf("connKey = %q, want %q", key, expected)
	}
}

// ---------------------------------------------------------------------------
// TestAddrEmpty
// ---------------------------------------------------------------------------

func TestAddrEmpty(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	if addr := svc.Addr(); addr != "" {
		t.Errorf("Addr() = %q, want empty before Start", addr)
	}
}

// ---------------------------------------------------------------------------
// TestComputeConntrackStatsDetailed
// ---------------------------------------------------------------------------

func TestComputeConntrackStatsDetailed(t *testing.T) {
	svc := NewService(newTestLogger(), nil)
	now := time.Now()

	// Add diverse connections
	entries := []*ConntrackEntry{
		{SrcIP: "10.0.0.1", DstIP: "10.0.0.100", SrcPort: 1, DstPort: 443, Protocol: "tcp", State: StateEstablished, PacketCount: 100, ByteCount: 5000, CreatedAt: now, LastSeen: now},
		{SrcIP: "10.0.0.1", DstIP: "10.0.0.100", SrcPort: 2, DstPort: 443, Protocol: "tcp", State: StateEstablished, PacketCount: 200, ByteCount: 10000, CreatedAt: now, LastSeen: now},
		{SrcIP: "10.0.0.2", DstIP: "10.0.0.101", SrcPort: 3, DstPort: 80, Protocol: "tcp", State: StateNew, PacketCount: 5, ByteCount: 200, CreatedAt: now, LastSeen: now},
		{SrcIP: "10.0.0.3", DstIP: "10.0.0.102", SrcPort: 4, DstPort: 8080, Protocol: "tcp", State: StateClosed, PacketCount: 50, ByteCount: 2000, CreatedAt: now, LastSeen: now},
	}
	for _, e := range entries {
		svc.AddConnection(e)
	}

	svc.computeConntrackStats(context.Background())
	stats := svc.GetConntrackStats()

	if stats.TotalConnections != 4 {
		t.Errorf("TotalConnections = %d, want 4", stats.TotalConnections)
	}
	if stats.ActiveByState[StateEstablished] != 2 {
		t.Errorf("ESTABLISHED = %d, want 2", stats.ActiveByState[StateEstablished])
	}
	if stats.ActiveByState[StateNew] != 1 {
		t.Errorf("NEW = %d, want 1", stats.ActiveByState[StateNew])
	}
	if stats.ActiveByState[StateClosed] != 1 {
		t.Errorf("CLOSED = %d, want 1", stats.ActiveByState[StateClosed])
	}
	if len(stats.TopSrcIPs) == 0 {
		t.Fatal("expected at least one top source IP")
	}
	// 10.0.0.1 has 2 connections
	if stats.TopSrcIPs[0].IP != "10.0.0.1" || stats.TopSrcIPs[0].Count != 2 {
		t.Errorf("top src IP = %s:%d, want 10.0.0.1:2", stats.TopSrcIPs[0].IP, stats.TopSrcIPs[0].Count)
	}
	if len(stats.TopDstServices) == 0 {
		t.Error("expected at least one top destination service")
	}
}
