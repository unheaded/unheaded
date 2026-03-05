// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

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
