// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package shield

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

// ============================================================================
// TEST HELPERS
// ============================================================================

// newTestLogger returns a logger that discards output (avoids noise in tests).
func newTestLogger() *logger.Logger {
	return logger.New(io.Discard)
}

// newTestService creates a Service wired up with a silent logger and no wotan
// client. Suitable for the majority of unit tests that do not exercise wotan.
func newTestService(cfg *Config) *Service {
	return NewService(newTestLogger(), nil, cfg)
}

// newTestRequest builds a Request with sensible defaults; callers may override
// fields after construction.
func newTestRequest(id, ip, method, path string) *Request {
	return &Request{
		ID:        id,
		SourceIP:  ip,
		Method:    method,
		Path:      path,
		Headers:   make(map[string][]string),
		Timestamp: time.Now(),
	}
}

// ============================================================================
// DefaultConfig
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if cfg.DefaultRateLimit != 1000 {
		t.Errorf("DefaultRateLimit = %d, want 1000", cfg.DefaultRateLimit)
	}
	if cfg.RateLimitWindow != time.Minute {
		t.Errorf("RateLimitWindow = %v, want 1m", cfg.RateLimitWindow)
	}
	if cfg.MaxRequestBodySize != 10*1024*1024 {
		t.Errorf("MaxRequestBodySize = %d, want 10MB", cfg.MaxRequestBodySize)
	}
	if !cfg.EnableThreatLogging {
		t.Error("EnableThreatLogging should default to true")
	}
	if cfg.WotanTopic != "shield.events" {
		t.Errorf("WotanTopic = %q, want %q", cfg.WotanTopic, "shield.events")
	}
}

// ============================================================================
// NewService
// ============================================================================

func TestNewService(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "nil config uses defaults",
			cfg:  nil,
		},
		{
			name: "custom config",
			cfg: &Config{
				DefaultRateLimit: 500,
				RateLimitWindow:  30 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(newTestLogger(), nil, tt.cfg)
			if svc == nil {
				t.Fatal("NewService returned nil")
			}
			if svc.rules == nil {
				t.Error("rules map should be initialised")
			}
			if svc.threats == nil {
				t.Error("threats slice should be initialised")
			}
			if svc.rateLimiter == nil {
				t.Error("rateLimiter should be initialised")
			}
			if svc.rateLimiter.buckets == nil {
				t.Error("rateLimiter buckets should be initialised")
			}
		})
	}
}

func TestNewService_NilConfigDefaults(t *testing.T) {
	svc := NewService(newTestLogger(), nil, nil)
	if svc.config.DefaultRateLimit != 1000 {
		t.Errorf("expected default rate limit 1000, got %d", svc.config.DefaultRateLimit)
	}
}

// ============================================================================
// Start / Stop
// ============================================================================

func TestService_StartStop(t *testing.T) {
	svc := newTestService(nil)
	ctx := context.Background()

	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// After Start, default rules should be registered.
	rules := svc.ListRules()
	if len(rules) == 0 {
		t.Error("expected default rules after Start")
	}

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestService_Start_RegistersDefaultRules(t *testing.T) {
	svc := newTestService(nil)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	sqli, ok := svc.GetRule("sqli-detection")
	if !ok {
		t.Fatal("expected sqli-detection rule to exist after Start")
	}
	if sqli.Type != RuleTypeSQLInjection {
		t.Errorf("sqli rule type = %s, want %s", sqli.Type, RuleTypeSQLInjection)
	}
	if sqli.Action != ActionBlock {
		t.Errorf("sqli action = %s, want %s", sqli.Action, ActionBlock)
	}
	if !sqli.Enabled {
		t.Error("sqli rule should be enabled")
	}

	xss, ok := svc.GetRule("xss-detection")
	if !ok {
		t.Fatal("expected xss-detection rule to exist after Start")
	}
	if xss.Type != RuleTypeXSS {
		t.Errorf("xss rule type = %s, want %s", xss.Type, RuleTypeXSS)
	}
}

// ============================================================================
// AddRule
// ============================================================================

func TestService_AddRule(t *testing.T) {
	tests := []struct {
		name       string
		rule       *Rule
		wantID     bool // true = expects auto-generated ID
		expectName string
	}{
		{
			name: "rule with explicit ID",
			rule: &Rule{
				ID:      "explicit-id",
				Name:    "Explicit Rule",
				Type:    RuleTypeCustom,
				Action:  ActionBlock,
				Enabled: true,
			},
			wantID:     false,
			expectName: "Explicit Rule",
		},
		{
			name: "rule without ID gets auto-generated",
			rule: &Rule{
				Name:    "Auto-ID Rule",
				Type:    RuleTypeBot,
				Action:  ActionChallenge,
				Enabled: true,
			},
			wantID:     true,
			expectName: "Auto-ID Rule",
		},
		{
			name: "disabled rule",
			rule: &Rule{
				ID:      "disabled-rule",
				Name:    "Disabled Rule",
				Type:    RuleTypeCustom,
				Action:  ActionBlock,
				Enabled: false,
			},
			wantID:     false,
			expectName: "Disabled Rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(nil)

			err := svc.AddRule(tt.rule)
			if err != nil {
				t.Fatalf("AddRule returned error: %v", err)
			}

			if tt.wantID && tt.rule.ID == "" {
				t.Error("expected auto-generated ID, got empty")
			}

			got, ok := svc.GetRule(tt.rule.ID)
			if !ok {
				t.Fatalf("rule %q not found after AddRule", tt.rule.ID)
			}
			if got.Name != tt.expectName {
				t.Errorf("name = %q, want %q", got.Name, tt.expectName)
			}
			if got.CreatedAt.IsZero() {
				t.Error("CreatedAt should be set by AddRule")
			}
		})
	}
}

func TestService_AddRule_OverwritesExistingID(t *testing.T) {
	svc := newTestService(nil)

	r1 := &Rule{ID: "dup", Name: "First", Type: RuleTypeCustom, Action: ActionLog, Enabled: true}
	r2 := &Rule{ID: "dup", Name: "Second", Type: RuleTypeCustom, Action: ActionBlock, Enabled: true}

	svc.AddRule(r1)
	svc.AddRule(r2)

	got, ok := svc.GetRule("dup")
	if !ok {
		t.Fatal("rule not found")
	}
	if got.Name != "Second" {
		t.Errorf("expected overwrite, got name = %q", got.Name)
	}
}

// ============================================================================
// RemoveRule
// ============================================================================

func TestService_RemoveRule(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(s *Service)
		ruleID  string
		wantErr bool
	}{
		{
			name: "remove existing rule",
			setup: func(s *Service) {
				s.AddRule(&Rule{ID: "rule-1", Name: "R1", Enabled: true})
			},
			ruleID:  "rule-1",
			wantErr: false,
		},
		{
			name:    "remove nonexistent rule",
			setup:   func(s *Service) {},
			ruleID:  "does-not-exist",
			wantErr: true,
		},
		{
			name: "remove already removed rule",
			setup: func(s *Service) {
				s.AddRule(&Rule{ID: "rule-2", Name: "R2", Enabled: true})
				s.RemoveRule("rule-2")
			},
			ruleID:  "rule-2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService(nil)
			tt.setup(svc)

			err := svc.RemoveRule(tt.ruleID)
			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveRule error = %v, wantErr %v", err, tt.wantErr)
			}

			if err == nil {
				if _, ok := svc.GetRule(tt.ruleID); ok {
					t.Error("rule should not be found after removal")
				}
			}
		})
	}
}

// ============================================================================
// GetRule / ListRules
// ============================================================================

func TestService_GetRule_NotFound(t *testing.T) {
	svc := newTestService(nil)
	_, ok := svc.GetRule("no-such-rule")
	if ok {
		t.Error("expected ok=false for non-existent rule")
	}
}

func TestService_ListRules_Empty(t *testing.T) {
	svc := newTestService(nil)
	rules := svc.ListRules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestService_ListRules_SortedByPriority(t *testing.T) {
	svc := newTestService(nil)

	svc.AddRule(&Rule{ID: "low", Name: "Low", Priority: 100, Enabled: true})
	svc.AddRule(&Rule{ID: "high", Name: "High", Priority: 1, Enabled: true})
	svc.AddRule(&Rule{ID: "mid", Name: "Mid", Priority: 50, Enabled: true})

	rules := svc.ListRules()
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	for i := 0; i < len(rules)-1; i++ {
		if rules[i].Priority > rules[i+1].Priority {
			t.Errorf("rules not sorted: [%d].Priority=%d > [%d].Priority=%d",
				i, rules[i].Priority, i+1, rules[i+1].Priority)
		}
	}
}

// ============================================================================
// RateLimiter
// ============================================================================

func TestRateLimiter_Allow(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		requests   int
		wantAllow  int
		wantDeny   int
	}{
		{
			name:      "all within limit",
			limit:     10,
			requests:  5,
			wantAllow: 5,
			wantDeny:  0,
		},
		{
			name:      "exactly at limit",
			limit:     5,
			requests:  5,
			wantAllow: 5,
			wantDeny:  0,
		},
		{
			name:      "exceeds limit",
			limit:     3,
			requests:  7,
			wantAllow: 3,
			wantDeny:  4,
		},
		{
			name:      "zero limit denies all",
			limit:     0,
			requests:  3,
			wantAllow: 0,
			wantDeny:  3,
		},
		{
			name:      "single request with limit 1",
			limit:     1,
			requests:  1,
			wantAllow: 1,
			wantDeny:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := &RateLimiter{buckets: make(map[string]*bucket)}
			window := time.Minute

			allowed := 0
			denied := 0
			for i := 0; i < tt.requests; i++ {
				if rl.Allow("test-key", tt.limit, window) {
					allowed++
				} else {
					denied++
				}
			}

			if allowed != tt.wantAllow {
				t.Errorf("allowed = %d, want %d", allowed, tt.wantAllow)
			}
			if denied != tt.wantDeny {
				t.Errorf("denied = %d, want %d", denied, tt.wantDeny)
			}
		})
	}
}

func TestRateLimiter_SeparateKeys(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*bucket)}
	window := time.Minute

	// Each key gets its own bucket.
	for i := 0; i < 3; i++ {
		if !rl.Allow("key-a", 3, window) {
			t.Error("key-a should be allowed")
		}
		if !rl.Allow("key-b", 3, window) {
			t.Error("key-b should be allowed")
		}
	}

	// key-a exhausted, key-b exhausted, key-c fresh.
	if rl.Allow("key-a", 3, window) {
		t.Error("key-a should be denied after exhausting limit")
	}
	if !rl.Allow("key-c", 3, window) {
		t.Error("key-c should be allowed (first request)")
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*bucket)}
	window := time.Minute
	limit := 100

	var wg sync.WaitGroup
	results := make(chan bool, 200)

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- rl.Allow("concurrent-key", limit, window)
		}()
	}

	wg.Wait()
	close(results)

	allowed := 0
	for r := range results {
		if r {
			allowed++
		}
	}

	if allowed != limit {
		t.Errorf("allowed = %d, want exactly %d", allowed, limit)
	}
}

// ============================================================================
// Evaluate
// ============================================================================

func TestService_Evaluate_AllowByDefault(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 1000,
		RateLimitWindow:  time.Minute,
	})

	req := newTestRequest("req-1", "10.0.0.1", "GET", "/api/v1/health")
	decision := svc.Evaluate(context.Background(), req)

	if decision.Action != ActionAllow {
		t.Errorf("action = %s, want %s", decision.Action, ActionAllow)
	}
	if decision.RequestID != "req-1" {
		t.Errorf("request_id = %q, want %q", decision.RequestID, "req-1")
	}
	if decision.RiskScore != 0.0 {
		t.Errorf("risk_score = %f, want 0.0", decision.RiskScore)
	}
	if decision.ProcessedAt.IsZero() {
		t.Error("processed_at should be set")
	}
}

func TestService_Evaluate_RateLimitExceeded(t *testing.T) {
	cfg := &Config{
		DefaultRateLimit: 3,
		RateLimitWindow:  time.Minute,
	}
	svc := newTestService(cfg)

	ip := "10.0.0.99"
	for i := 0; i < 3; i++ {
		d := svc.Evaluate(context.Background(), newTestRequest("", ip, "GET", "/"))
		if d.Action != ActionAllow {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// Fourth request should be throttled.
	d := svc.Evaluate(context.Background(), newTestRequest("req-4", ip, "GET", "/"))
	if d.Action != ActionThrottle {
		t.Errorf("action = %s, want %s", d.Action, ActionThrottle)
	}
	if d.Reason != "rate limit exceeded" {
		t.Errorf("reason = %q, want %q", d.Reason, "rate limit exceeded")
	}
	if d.RiskScore != 0.7 {
		t.Errorf("risk_score = %f, want 0.7", d.RiskScore)
	}
}

func TestService_Evaluate_RateLimitPerIP(t *testing.T) {
	cfg := &Config{
		DefaultRateLimit: 2,
		RateLimitWindow:  time.Minute,
	}
	svc := newTestService(cfg)

	// Exhaust IP-A.
	svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/"))
	svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/"))

	d := svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/"))
	if d.Action != ActionThrottle {
		t.Error("10.0.0.1 should be throttled")
	}

	// IP-B should still be allowed.
	d = svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.2", "GET", "/"))
	if d.Action != ActionAllow {
		t.Error("10.0.0.2 should be allowed")
	}
}

func TestService_Evaluate_MatchesBlockRule(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID:       "block-admin",
		Name:     "Block admin path",
		Type:     RuleTypeCustom,
		Action:   ActionBlock,
		Enabled:  true,
		Priority: 1,
		Conditions: []Condition{
			{Field: "path", Operator: "equals", Value: "/admin"},
		},
	})

	req := newTestRequest("req-admin", "10.0.0.1", "GET", "/admin")
	d := svc.Evaluate(context.Background(), req)

	if d.Action != ActionBlock {
		t.Errorf("action = %s, want %s", d.Action, ActionBlock)
	}
	if d.RuleID != "block-admin" {
		t.Errorf("rule_id = %q, want %q", d.RuleID, "block-admin")
	}
	if d.RiskScore != 1.0 {
		t.Errorf("risk_score = %f, want 1.0", d.RiskScore)
	}
}

func TestService_Evaluate_DisabledRuleSkipped(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID:       "disabled-block",
		Name:     "Disabled Block",
		Type:     RuleTypeCustom,
		Action:   ActionBlock,
		Enabled:  false,
		Priority: 1,
		Conditions: []Condition{
			{Field: "path", Operator: "equals", Value: "/secret"},
		},
	})

	d := svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/secret"))
	if d.Action != ActionAllow {
		t.Errorf("disabled rule should be skipped, action = %s", d.Action)
	}
}

func TestService_Evaluate_RulePriorityOrder(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	// Lower priority number = higher priority. The allow rule should win.
	svc.AddRule(&Rule{
		ID:       "allow-rule",
		Name:     "Allow VIP",
		Type:     RuleTypeCustom,
		Action:   ActionAllow,
		Enabled:  true,
		Priority: 1,
		Conditions: []Condition{
			{Field: "source_ip", Operator: "equals", Value: "10.0.0.5"},
		},
	})
	svc.AddRule(&Rule{
		ID:       "block-rule",
		Name:     "Block All",
		Type:     RuleTypeCustom,
		Action:   ActionBlock,
		Enabled:  true,
		Priority: 10,
		Conditions: []Condition{
			{Field: "source_ip", Operator: "equals", Value: "10.0.0.5"},
		},
	})

	d := svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.5", "GET", "/"))
	if d.Action != ActionAllow {
		t.Errorf("higher-priority allow rule should win, got %s", d.Action)
	}
	if d.RuleID != "allow-rule" {
		t.Errorf("rule_id = %q, want %q", d.RuleID, "allow-rule")
	}
}

func TestService_Evaluate_NoConditions_NoMatch(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID:         "no-conditions",
		Name:       "Empty",
		Type:       RuleTypeCustom,
		Action:     ActionBlock,
		Enabled:    true,
		Priority:   1,
		Conditions: []Condition{}, // empty conditions
	})

	d := svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/"))
	if d.Action != ActionAllow {
		t.Errorf("rule with no conditions should not match, got %s", d.Action)
	}
}

// ============================================================================
// matchesRule - condition field tests
// ============================================================================

func TestService_matchesRule(t *testing.T) {
	svc := newTestService(nil)

	tests := []struct {
		name       string
		req        *Request
		conditions []Condition
		want       bool
	}{
		{
			name: "match source_ip",
			req:  newTestRequest("", "192.168.1.1", "GET", "/"),
			conditions: []Condition{
				{Field: "source_ip", Operator: "equals", Value: "192.168.1.1"},
			},
			want: true,
		},
		{
			name: "no match source_ip",
			req:  newTestRequest("", "10.0.0.1", "GET", "/"),
			conditions: []Condition{
				{Field: "source_ip", Operator: "equals", Value: "192.168.1.1"},
			},
			want: false,
		},
		{
			name: "match path",
			req:  newTestRequest("", "10.0.0.1", "GET", "/admin"),
			conditions: []Condition{
				{Field: "path", Operator: "equals", Value: "/admin"},
			},
			want: true,
		},
		{
			name: "no match path",
			req:  newTestRequest("", "10.0.0.1", "GET", "/api"),
			conditions: []Condition{
				{Field: "path", Operator: "equals", Value: "/admin"},
			},
			want: false,
		},
		{
			name: "match method",
			req:  newTestRequest("", "10.0.0.1", "POST", "/"),
			conditions: []Condition{
				{Field: "method", Operator: "equals", Value: "POST"},
			},
			want: true,
		},
		{
			name: "no match method",
			req:  newTestRequest("", "10.0.0.1", "GET", "/"),
			conditions: []Condition{
				{Field: "method", Operator: "equals", Value: "DELETE"},
			},
			want: false,
		},
		{
			name: "multiple conditions all match (AND)",
			req:  newTestRequest("", "10.0.0.1", "POST", "/admin"),
			conditions: []Condition{
				{Field: "source_ip", Operator: "equals", Value: "10.0.0.1"},
				{Field: "method", Operator: "equals", Value: "POST"},
				{Field: "path", Operator: "equals", Value: "/admin"},
			},
			want: true,
		},
		{
			name: "multiple conditions partial match = no match",
			req:  newTestRequest("", "10.0.0.1", "GET", "/admin"),
			conditions: []Condition{
				{Field: "source_ip", Operator: "equals", Value: "10.0.0.1"},
				{Field: "method", Operator: "equals", Value: "POST"},
			},
			want: false,
		},
		{
			name:       "empty conditions = no match",
			req:        newTestRequest("", "10.0.0.1", "GET", "/"),
			conditions: []Condition{},
			want:       false,
		},
		{
			name: "unknown field is ignored (no mismatch returned)",
			req:  newTestRequest("", "10.0.0.1", "GET", "/"),
			conditions: []Condition{
				{Field: "unknown_field", Operator: "equals", Value: "anything"},
			},
			want: true, // unknown field falls through without returning false
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := &Rule{Conditions: tt.conditions}
			got := svc.matchesRule(tt.req, rule)
			if got != tt.want {
				t.Errorf("matchesRule = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// IP Blocklist via rules
// ============================================================================

func TestService_IPBlocklist(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	blockedIPs := []string{"192.168.1.100", "10.10.10.99", "172.16.0.1"}
	for i, ip := range blockedIPs {
		svc.AddRule(&Rule{
			ID:       fmt.Sprintf("block-ip-%d", i),
			Name:     "IP Blocklist",
			Type:     RuleTypeIPBlock,
			Action:   ActionBlock,
			Enabled:  true,
			Priority: 1,
			Conditions: []Condition{
				{Field: "source_ip", Operator: "equals", Value: ip},
			},
		})
	}

	t.Run("blocked IPs are rejected", func(t *testing.T) {
		for _, ip := range blockedIPs {
			d := svc.Evaluate(context.Background(), newTestRequest("", ip, "GET", "/"))
			if d.Action != ActionBlock {
				t.Errorf("IP %s should be blocked, got %s", ip, d.Action)
			}
		}
	})

	t.Run("non-blocked IPs are allowed", func(t *testing.T) {
		safeIPs := []string{"192.168.1.1", "10.0.0.1", "8.8.8.8"}
		for _, ip := range safeIPs {
			d := svc.Evaluate(context.Background(), newTestRequest("", ip, "GET", "/"))
			if d.Action != ActionAllow {
				t.Errorf("IP %s should be allowed, got %s", ip, d.Action)
			}
		}
	})
}

// ============================================================================
// XSS / SQLi detection paths
// ============================================================================

func TestService_XSS_SQLi_DefaultRulesExist(t *testing.T) {
	svc := newTestService(nil)
	svc.Start(context.Background())

	sqli, ok := svc.GetRule("sqli-detection")
	if !ok {
		t.Fatal("sqli-detection rule not registered")
	}
	if sqli.Action != ActionBlock {
		t.Errorf("sqli action = %s, want block", sqli.Action)
	}
	if sqli.Priority != 10 {
		t.Errorf("sqli priority = %d, want 10", sqli.Priority)
	}

	xss, ok := svc.GetRule("xss-detection")
	if !ok {
		t.Fatal("xss-detection rule not registered")
	}
	if xss.Action != ActionBlock {
		t.Errorf("xss action = %s, want block", xss.Action)
	}
}

// Because the default sqli/xss rules have no conditions, they do not match
// any request (conditions list is empty). This tests that code path.
func TestService_DefaultSQLiXSS_NoConditions_DoNotBlock(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})
	svc.Start(context.Background())

	payloads := []string{
		"/search?q=<script>alert(1)</script>",
		"/login?user=admin' OR '1'='1",
		"/api?id=1; DROP TABLE users--",
	}

	for _, path := range payloads {
		d := svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", path))
		// Default rules have no conditions so they should NOT match.
		if d.Action != ActionAllow {
			t.Errorf("path %q: expected allow (no conditions on default rules), got %s", path, d.Action)
		}
	}
}

// Test XSS/SQLi when rules are configured with conditions.
func TestService_XSS_SQLi_WithConditions(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	// Simulate a condition-based sqli rule matching a specific path.
	svc.AddRule(&Rule{
		ID:       "sqli-path",
		Name:     "SQLi on login",
		Type:     RuleTypeSQLInjection,
		Action:   ActionBlock,
		Enabled:  true,
		Priority: 5,
		Conditions: []Condition{
			{Field: "path", Operator: "equals", Value: "/login"},
			{Field: "method", Operator: "equals", Value: "POST"},
		},
	})

	d := svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "POST", "/login"))
	if d.Action != ActionBlock {
		t.Errorf("expected block on POST /login, got %s", d.Action)
	}

	// GET /login should not match (method condition fails).
	d = svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/login"))
	if d.Action == ActionBlock {
		t.Error("GET /login should not be blocked by POST-only rule")
	}
}

// ============================================================================
// Request filtering via rules
// ============================================================================

func TestService_RequestFiltering(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	// Block DELETE requests to /api/v1/users
	svc.AddRule(&Rule{
		ID:       "filter-delete-users",
		Name:     "Block user deletion",
		Type:     RuleTypeCustom,
		Action:   ActionBlock,
		Enabled:  true,
		Priority: 5,
		Conditions: []Condition{
			{Field: "method", Operator: "equals", Value: "DELETE"},
			{Field: "path", Operator: "equals", Value: "/api/v1/users"},
		},
	})

	tests := []struct {
		name   string
		method string
		path   string
		want   RuleAction
	}{
		{"DELETE /api/v1/users blocked", "DELETE", "/api/v1/users", ActionBlock},
		{"GET /api/v1/users allowed", "GET", "/api/v1/users", ActionAllow},
		{"DELETE /api/v1/posts allowed", "DELETE", "/api/v1/posts", ActionAllow},
		{"POST /api/v1/users allowed", "POST", "/api/v1/users", ActionAllow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := svc.Evaluate(context.Background(),
				newTestRequest("", "10.0.0.1", tt.method, tt.path))
			if d.Action != tt.want {
				t.Errorf("action = %s, want %s", d.Action, tt.want)
			}
		})
	}
}

// ============================================================================
// RecordThreat
// ============================================================================

func TestService_RecordThreat(t *testing.T) {
	svc := newTestService(nil)

	tests := []struct {
		name   string
		threat *ThreatEvent
	}{
		{
			name: "threat with ID",
			threat: &ThreatEvent{
				ID:          "threat-001",
				Type:        "sql_injection",
				SourceIP:    "192.168.1.100",
				Severity:    "high",
				Description: "SQLi attempt detected",
			},
		},
		{
			name: "threat without ID gets auto-generated",
			threat: &ThreatEvent{
				Type:        "xss",
				SourceIP:    "10.0.0.5",
				Severity:    "medium",
				Description: "XSS attempt",
			},
		},
		{
			name: "threat with mitigated flag",
			threat: &ThreatEvent{
				ID:          "threat-003",
				Type:        "brute_force",
				SourceIP:    "172.16.0.1",
				Severity:    "critical",
				Description: "Brute force login attempt",
				Mitigated:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc.RecordThreat(tt.threat)

			if tt.threat.ID == "" {
				t.Error("ID should be set (auto-generated if empty)")
			}
			if tt.threat.DetectedAt.IsZero() {
				t.Error("DetectedAt should be set")
			}
		})
	}

	// Verify all threats are stored.
	stats := svc.Stats()
	if stats["threats_detected"] != len(tests) {
		t.Errorf("threats_detected = %v, want %d", stats["threats_detected"], len(tests))
	}
}

// ============================================================================
// Stats
// ============================================================================

func TestService_Stats(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	// Initial stats.
	stats := svc.Stats()
	if stats["requests_processed"].(int64) != 0 {
		t.Errorf("initial requests_processed = %v, want 0", stats["requests_processed"])
	}
	if stats["requests_blocked"].(int64) != 0 {
		t.Errorf("initial requests_blocked = %v, want 0", stats["requests_blocked"])
	}
	if stats["active_rules"].(int) != 0 {
		t.Errorf("initial active_rules = %v, want 0", stats["active_rules"])
	}
	if stats["threats_detected"].(int) != 0 {
		t.Errorf("initial threats_detected = %v, want 0", stats["threats_detected"])
	}

	// Process some requests.
	svc.AddRule(&Rule{
		ID: "block-bad", Name: "Block Bad", Type: RuleTypeCustom,
		Action: ActionBlock, Enabled: true, Priority: 1,
		Conditions: []Condition{{Field: "source_ip", Operator: "equals", Value: "bad-ip"}},
	})

	svc.Evaluate(context.Background(), newTestRequest("", "good-ip", "GET", "/"))
	svc.Evaluate(context.Background(), newTestRequest("", "bad-ip", "GET", "/"))
	svc.RecordThreat(&ThreatEvent{Type: "test", SourceIP: "1.1.1.1"})

	stats = svc.Stats()
	if stats["requests_processed"].(int64) != 2 {
		t.Errorf("requests_processed = %v, want 2", stats["requests_processed"])
	}
	if stats["requests_blocked"].(int64) != 1 {
		t.Errorf("requests_blocked = %v, want 1", stats["requests_blocked"])
	}
	if stats["active_rules"].(int) != 1 {
		t.Errorf("active_rules = %v, want 1", stats["active_rules"])
	}
	if stats["threats_detected"].(int) != 1 {
		t.Errorf("threats_detected = %v, want 1", stats["threats_detected"])
	}
}

func TestService_Stats_BlockCountOnThrottle(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 1,
		RateLimitWindow:  time.Minute,
	})

	svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/"))
	svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/"))

	stats := svc.Stats()
	if stats["requests_blocked"].(int64) != 1 {
		t.Errorf("throttled request should count as blocked, got %v", stats["requests_blocked"])
	}
}

// ============================================================================
// Middleware
// ============================================================================

func TestService_Middleware_Allow(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	wrapped := svc.Middleware()(inner)

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rr := httptest.NewRecorder()

	wrapped.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("inner handler should be called when request is allowed")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestService_Middleware_Block(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID: "block-evil", Name: "Block Evil", Type: RuleTypeIPBlock,
		Action: ActionBlock, Enabled: true, Priority: 1,
		Conditions: []Condition{{Field: "source_ip", Operator: "equals", Value: "evil:666"}},
	})

	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	})

	wrapped := svc.Middleware()(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "evil:666"
	rr := httptest.NewRecorder()

	wrapped.ServeHTTP(rr, req)

	if handlerCalled {
		t.Error("inner handler should NOT be called when request is blocked")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestService_Middleware_Throttle(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 1,
		RateLimitWindow:  time.Minute,
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := svc.Middleware()(inner)

	// Use the same RemoteAddr for both requests because the rate limiter
	// keys on the full SourceIP string (which is RemoteAddr in middleware).
	addr := "10.0.0.1:1111"

	// First request allowed.
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = addr
	rr1 := httptest.NewRecorder()
	wrapped.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Errorf("first request status = %d, want %d", rr1.Code, http.StatusOK)
	}

	// Second request throttled (same RemoteAddr, limit=1).
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = addr
	rr2 := httptest.NewRecorder()
	wrapped.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("second request status = %d, want %d", rr2.Code, http.StatusTooManyRequests)
	}
}

func TestService_Middleware_Challenge(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID: "challenge-bot", Name: "Challenge Bot", Type: RuleTypeBot,
		Action: ActionChallenge, Enabled: true, Priority: 1,
		Conditions: []Condition{{Field: "source_ip", Operator: "equals", Value: "bot:80"}},
	})

	handlerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	wrapped := svc.Middleware()(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "bot:80"
	rr := httptest.NewRecorder()

	wrapped.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("inner handler should be called for challenge action (not blocking)")
	}
	if rr.Header().Get("X-Shield-Challenge") != "required" {
		t.Error("X-Shield-Challenge header should be set")
	}
}

func TestService_Middleware_RequestID(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	// Add a rule that will match so we can inspect the decision.
	svc.AddRule(&Rule{
		ID: "log-all", Name: "Log All", Type: RuleTypeCustom,
		Action: ActionLog, Enabled: true, Priority: 1,
		Conditions: []Condition{{Field: "method", Operator: "equals", Value: "GET"}},
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := svc.Middleware()(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "trace-abc-123")
	req.RemoteAddr = "10.0.0.1:5555"
	rr := httptest.NewRecorder()

	wrapped.ServeHTTP(rr, req)

	// The request should pass through since ActionLog does not block.
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ============================================================================
// Concurrent access (race safety)
// ============================================================================

func TestService_ConcurrentEvaluate(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID: "test-rule", Name: "Test", Type: RuleTypeCustom,
		Action: ActionBlock, Enabled: true, Priority: 1,
		Conditions: []Condition{{Field: "source_ip", Operator: "equals", Value: "blocked"}},
	})

	var wg sync.WaitGroup
	errCh := make(chan string, 200)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ip := "good"
			if idx%10 == 0 {
				ip = "blocked"
			}
			d := svc.Evaluate(context.Background(), newTestRequest("", ip, "GET", "/"))
			if ip == "blocked" && d.Action != ActionBlock {
				errCh <- fmt.Sprintf("idx=%d: expected block, got %s", idx, d.Action)
			}
			if ip == "good" && d.Action != ActionAllow {
				errCh <- fmt.Sprintf("idx=%d: expected allow, got %s", idx, d.Action)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for msg := range errCh {
		t.Error(msg)
	}
}

func TestService_ConcurrentRuleManagement(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	var wg sync.WaitGroup

	// Concurrent adds.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			svc.AddRule(&Rule{
				ID:       fmt.Sprintf("concurrent-rule-%d", idx),
				Name:     "Concurrent Rule",
				Type:     RuleTypeCustom,
				Action:   ActionLog,
				Enabled:  true,
				Priority: idx,
			})
		}(i)
	}

	// Concurrent reads.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.ListRules()
		}()
	}

	// Concurrent evaluations.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/"))
		}()
	}

	wg.Wait()

	// Concurrent removes.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			svc.RemoveRule(fmt.Sprintf("concurrent-rule-%d", idx))
		}(i)
	}

	wg.Wait()
}

func TestService_ConcurrentStats(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/"))
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Stats()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.RecordThreat(&ThreatEvent{Type: "test", SourceIP: "1.1.1.1"})
		}()
	}

	wg.Wait()

	stats := svc.Stats()
	if stats["requests_processed"].(int64) != 50 {
		t.Errorf("requests_processed = %v, want 50", stats["requests_processed"])
	}
	if stats["threats_detected"].(int) != 50 {
		t.Errorf("threats_detected = %v, want 50", stats["threats_detected"])
	}
}

// ============================================================================
// Edge cases
// ============================================================================

func TestService_Evaluate_EmptyRequest(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	req := &Request{
		Headers:   make(map[string][]string),
		Timestamp: time.Now(),
	}
	d := svc.Evaluate(context.Background(), req)
	if d.Action != ActionAllow {
		t.Errorf("empty request should default to allow, got %s", d.Action)
	}
}

func TestService_Evaluate_NilHeaders(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	req := &Request{
		ID:       "req-nil-headers",
		SourceIP: "10.0.0.1",
		Method:   "GET",
		Path:     "/",
		Headers:  nil,
	}
	// Should not panic.
	d := svc.Evaluate(context.Background(), req)
	if d.Action != ActionAllow {
		t.Errorf("action = %s, want allow", d.Action)
	}
}

func TestService_Evaluate_LargeBody(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	largeBody := make([]byte, 20*1024*1024)
	req := &Request{
		ID:        "large-body",
		SourceIP:  "10.0.0.1",
		Method:    "POST",
		Path:      "/upload",
		Headers:   make(map[string][]string),
		Body:      largeBody,
		Timestamp: time.Now(),
	}

	// Should not panic or fail.
	d := svc.Evaluate(context.Background(), req)
	if d == nil {
		t.Fatal("decision should not be nil")
	}
}

func TestService_Evaluate_ContextCancellation(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Evaluate with a cancelled context should still return a decision
	// (the current implementation does not check context cancellation).
	d := svc.Evaluate(ctx, newTestRequest("", "10.0.0.1", "GET", "/"))
	if d == nil {
		t.Fatal("decision should not be nil even with cancelled context")
	}
}

func TestService_AddRule_TimestampSet(t *testing.T) {
	svc := newTestService(nil)

	before := time.Now()
	svc.AddRule(&Rule{ID: "ts-test", Name: "TS Test", Enabled: true})
	after := time.Now()

	rule, ok := svc.GetRule("ts-test")
	if !ok {
		t.Fatal("rule not found")
	}

	if rule.CreatedAt.Before(before) || rule.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not between %v and %v", rule.CreatedAt, before, after)
	}
}

func TestService_RecordThreat_AutoID(t *testing.T) {
	svc := newTestService(nil)

	threat := &ThreatEvent{Type: "test", SourceIP: "1.2.3.4"}
	svc.RecordThreat(threat)

	if threat.ID == "" {
		t.Error("threat ID should be auto-generated")
	}
	if !hasPrefix(threat.ID, "threat-") {
		t.Errorf("auto-generated ID = %q, expected prefix 'threat-'", threat.ID)
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestService_RecordThreat_PreservesExistingID(t *testing.T) {
	svc := newTestService(nil)

	threat := &ThreatEvent{ID: "my-custom-id", Type: "test", SourceIP: "1.2.3.4"}
	svc.RecordThreat(threat)

	if threat.ID != "my-custom-id" {
		t.Errorf("ID changed to %q, expected %q", threat.ID, "my-custom-id")
	}
}

// ============================================================================
// min helper
// ============================================================================

func TestMin(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{0, 0, 0},
		{-1, 0, -1},
		{-5, -3, -5},
		{100, 100, 100},
	}

	for _, tt := range tests {
		got := min(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// ============================================================================
// getSortedRules
// ============================================================================

func TestService_getSortedRules_Empty(t *testing.T) {
	svc := newTestService(nil)
	rules := svc.getSortedRules()
	if len(rules) != 0 {
		t.Errorf("expected 0 sorted rules, got %d", len(rules))
	}
}

func TestService_getSortedRules_SingleRule(t *testing.T) {
	svc := newTestService(nil)
	svc.AddRule(&Rule{ID: "only", Name: "Only", Priority: 42, Enabled: true})

	rules := svc.getSortedRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Priority != 42 {
		t.Errorf("priority = %d, want 42", rules[0].Priority)
	}
}

func TestService_getSortedRules_StablePriority(t *testing.T) {
	svc := newTestService(nil)

	// Add several rules with various priorities.
	priorities := []int{50, 10, 30, 20, 40}
	for i, p := range priorities {
		svc.AddRule(&Rule{
			ID:       fmt.Sprintf("r-%d", i),
			Name:     fmt.Sprintf("Rule %d", i),
			Priority: p,
			Enabled:  true,
		})
	}

	rules := svc.getSortedRules()

	for i := 1; i < len(rules); i++ {
		if rules[i-1].Priority > rules[i].Priority {
			t.Errorf("not sorted at index %d: %d > %d",
				i, rules[i-1].Priority, rules[i].Priority)
		}
	}
}

// ============================================================================
// recordBlock
// ============================================================================

func TestService_recordBlock(t *testing.T) {
	svc := newTestService(nil)

	for i := 0; i < 5; i++ {
		svc.recordBlock()
	}

	svc.mu.RLock()
	blocked := svc.requestsBlocked
	svc.mu.RUnlock()

	if blocked != 5 {
		t.Errorf("requestsBlocked = %d, want 5", blocked)
	}
}

// ============================================================================
// Middleware with real HTTP server (integration-style)
// ============================================================================

func TestService_Middleware_IntegrationStyle(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 5,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID: "block-path", Name: "Block Forbidden", Type: RuleTypeCustom,
		Action: ActionBlock, Enabled: true, Priority: 1,
		Conditions: []Condition{{Field: "path", Operator: "equals", Value: "/forbidden"}},
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := httptest.NewServer(svc.Middleware()(inner))
	defer server.Close()

	client := server.Client()

	t.Run("allowed path returns 200", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/api/v1/data")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("blocked path returns 403", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/forbidden")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}
	})
}

// ============================================================================
// RuleType and RuleAction constants validation
// ============================================================================

func TestRuleTypeConstants(t *testing.T) {
	types := map[RuleType]string{
		RuleTypeRateLimit:    "rate_limit",
		RuleTypeIPBlock:      "ip_block",
		RuleTypeGeoBlock:     "geo_block",
		RuleTypeSQLInjection: "sql_injection",
		RuleTypeXSS:          "xss",
		RuleTypeBot:          "bot_detection",
		RuleTypeCustom:       "custom",
	}

	for rt, expected := range types {
		if string(rt) != expected {
			t.Errorf("RuleType = %q, want %q", rt, expected)
		}
	}
}

func TestRuleActionConstants(t *testing.T) {
	actions := map[RuleAction]string{
		ActionAllow:     "allow",
		ActionBlock:     "block",
		ActionChallenge: "challenge",
		ActionLog:       "log",
		ActionThrottle:  "throttle",
	}

	for ra, expected := range actions {
		if string(ra) != expected {
			t.Errorf("RuleAction = %q, want %q", ra, expected)
		}
	}
}

// ============================================================================
// Evaluate: ActionLog and ActionChallenge do not block
// ============================================================================

func TestService_Evaluate_LogAction(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID: "log-only", Name: "Log Only", Type: RuleTypeCustom,
		Action: ActionLog, Enabled: true, Priority: 1,
		Conditions: []Condition{{Field: "path", Operator: "equals", Value: "/watched"}},
	})

	d := svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/watched"))
	if d.Action != ActionLog {
		t.Errorf("action = %s, want %s", d.Action, ActionLog)
	}
	if d.RiskScore == 1.0 {
		t.Error("log action should not set risk_score to 1.0")
	}
}

func TestService_Evaluate_ChallengeAction(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID: "challenge", Name: "Challenge", Type: RuleTypeBot,
		Action: ActionChallenge, Enabled: true, Priority: 1,
		Conditions: []Condition{{Field: "method", Operator: "equals", Value: "POST"}},
	})

	d := svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "POST", "/"))
	if d.Action != ActionChallenge {
		t.Errorf("action = %s, want %s", d.Action, ActionChallenge)
	}
}

// ============================================================================
// Evaluate: Ensure first matching rule wins (break)
// ============================================================================

func TestService_Evaluate_FirstMatchWins(t *testing.T) {
	svc := newTestService(&Config{
		DefaultRateLimit: 10000,
		RateLimitWindow:  time.Minute,
	})

	svc.AddRule(&Rule{
		ID: "first-match", Name: "First", Type: RuleTypeCustom,
		Action: ActionLog, Enabled: true, Priority: 1,
		Conditions: []Condition{{Field: "source_ip", Operator: "equals", Value: "10.0.0.1"}},
	})
	svc.AddRule(&Rule{
		ID: "second-match", Name: "Second", Type: RuleTypeCustom,
		Action: ActionBlock, Enabled: true, Priority: 2,
		Conditions: []Condition{{Field: "source_ip", Operator: "equals", Value: "10.0.0.1"}},
	})

	d := svc.Evaluate(context.Background(), newTestRequest("", "10.0.0.1", "GET", "/"))
	if d.Action != ActionLog {
		t.Errorf("expected first rule (log) to win, got %s", d.Action)
	}
	if d.RuleID != "first-match" {
		t.Errorf("rule_id = %q, want %q", d.RuleID, "first-match")
	}
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkService_Evaluate(b *testing.B) {
	svc := newTestService(&Config{
		DefaultRateLimit: 1000000,
		RateLimitWindow:  time.Minute,
	})
	svc.Start(context.Background())

	req := newTestRequest("bench-1", "10.0.0.1", "GET", "/api/v1/data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Evaluate(context.Background(), req)
	}
}

func BenchmarkService_AddRule(b *testing.B) {
	svc := newTestService(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.AddRule(&Rule{
			Name:    "Bench Rule",
			Type:    RuleTypeCustom,
			Action:  ActionLog,
			Enabled: true,
		})
	}
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	rl := &RateLimiter{buckets: make(map[string]*bucket)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("bench-key", 1000000, time.Minute)
	}
}

func BenchmarkService_Middleware(b *testing.B) {
	svc := newTestService(&Config{
		DefaultRateLimit: 1000000,
		RateLimitWindow:  time.Minute,
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := svc.Middleware()(inner)
	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	req.RemoteAddr = "10.0.0.1:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		wrapped.ServeHTTP(rr, req)
	}
}
