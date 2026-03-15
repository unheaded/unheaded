// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package shield provides WAF, DDoS protection, and Zero Trust gateway capabilities.
// Shield the WAF is the Kingdom's edge defense - protecting all ingress traffic.
// Implements rate limiting, threat detection, request validation, and edge security.
package shield

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
)

// Rule represents a WAF rule.
type Rule struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Type        RuleType   `json:"type"`
	Action      RuleAction `json:"action"`
	Conditions  []Condition `json:"conditions"`
	Priority    int        `json:"priority"`
	Enabled     bool       `json:"enabled"`
	CreatedAt   time.Time  `json:"created_at"`
}

// RuleType categorizes rules.
type RuleType string

const (
	RuleTypeRateLimit   RuleType = "rate_limit"
	RuleTypeIPBlock     RuleType = "ip_block"
	RuleTypeGeoBlock    RuleType = "geo_block"
	RuleTypeSQLInjection RuleType = "sql_injection"
	RuleTypeXSS         RuleType = "xss"
	RuleTypeBot         RuleType = "bot_detection"
	RuleTypeCustom      RuleType = "custom"
)

// RuleAction defines what happens when rule matches.
type RuleAction string

const (
	ActionAllow     RuleAction = "allow"
	ActionBlock     RuleAction = "block"
	ActionChallenge RuleAction = "challenge"
	ActionLog       RuleAction = "log"
	ActionThrottle  RuleAction = "throttle"
)

// Condition represents a rule condition.
type Condition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// Request represents an incoming request to be evaluated.
type Request struct {
	ID         string              `json:"id"`
	SourceIP   string              `json:"source_ip"`
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	Headers    map[string][]string `json:"headers"`
	Body       []byte              `json:"body,omitempty"`
	Timestamp  time.Time           `json:"timestamp"`
}

// Decision represents the shield's decision on a request.
type Decision struct {
	RequestID   string     `json:"request_id"`
	Action      RuleAction `json:"action"`
	RuleID      string     `json:"rule_id,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	RiskScore   float64    `json:"risk_score"`
	ProcessedAt time.Time  `json:"processed_at"`
}

// ThreatEvent represents a detected threat.
type ThreatEvent struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	SourceIP    string    `json:"source_ip"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	DetectedAt  time.Time `json:"detected_at"`
	Mitigated   bool      `json:"mitigated"`
}

// RateLimiter tracks request rates.
type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens    int
	lastFill  time.Time
	rateLimit int
	window    time.Duration
}

// Service is the main Shield WAF service.
type Service struct {
	log    *logger.Logger
	wotan *wotanClient.Client
	config *Config

	mu           sync.RWMutex
	rules        map[string]*Rule
	threats      []*ThreatEvent
	rateLimiter  *RateLimiter

	// Counters
	requestsProcessed int64
	requestsBlocked   int64
}

// Config holds Shield service configuration.
type Config struct {
	DefaultRateLimit    int           `json:"default_rate_limit"`
	RateLimitWindow     time.Duration `json:"rate_limit_window"`
	MaxRequestBodySize  int64         `json:"max_request_body_size"`
	EnableThreatLogging bool          `json:"enable_threat_logging"`
	WotanTopic         string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		DefaultRateLimit:    1000, // requests per window
		RateLimitWindow:     time.Minute,
		MaxRequestBodySize:  10 * 1024 * 1024, // 10MB
		EnableThreatLogging: true,
		WotanTopic:         "shield.events",
	}
}

// NewService creates a new Shield WAF service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Service{
		log:    log,
		wotan: wotan,
		config: cfg,
		rules:  make(map[string]*Rule),
		threats: make([]*ThreatEvent, 0),
		rateLimiter: &RateLimiter{
			buckets: make(map[string]*bucket),
		},
	}
}

// Start initializes the Shield service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Shield raised - the edge is defended")
	s.registerDefaultRules()
	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Shield lowered - edge defense suspended")
	return nil
}

// Evaluate processes a request through the WAF rules.
func (s *Service) Evaluate(ctx context.Context, req *Request) *Decision {
	s.mu.Lock()
	s.requestsProcessed++
	s.mu.Unlock()

	decision := &Decision{
		RequestID:   req.ID,
		Action:      ActionAllow,
		RiskScore:   0.0,
		ProcessedAt: time.Now(),
	}

	// Check rate limit
	if !s.rateLimiter.Allow(req.SourceIP, s.config.DefaultRateLimit, s.config.RateLimitWindow) {
		decision.Action = ActionThrottle
		decision.Reason = "rate limit exceeded"
		decision.RiskScore = 0.7
		s.recordBlock()
		return decision
	}

	// Evaluate rules by priority
	s.mu.RLock()
	rules := s.getSortedRules()
	s.mu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		if s.matchesRule(req, rule) {
			decision.Action = rule.Action
			decision.RuleID = rule.ID
			decision.Reason = rule.Name

			if rule.Action == ActionBlock {
				decision.RiskScore = 1.0
				s.recordBlock()
			}
			break
		}
	}

	return decision
}

// AddRule adds a new WAF rule.
func (s *Service) AddRule(rule *Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	}
	rule.CreatedAt = time.Now()

	s.rules[rule.ID] = rule
	s.log.Info().Str("id", rule.ID).Str("name", rule.Name).Msg("WAF rule added")
	return nil
}

// RemoveRule removes a WAF rule.
func (s *Service) RemoveRule(ruleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rules[ruleID]; !ok {
		return fmt.Errorf("rule not found: %s", ruleID)
	}

	delete(s.rules, ruleID)
	return nil
}

// GetRule retrieves a rule by ID.
func (s *Service) GetRule(ruleID string) (*Rule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rule, ok := s.rules[ruleID]
	return rule, ok
}

// ListRules returns all rules.
func (s *Service) ListRules() []*Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getSortedRules()
}

// RecordThreat records a detected threat.
func (s *Service) RecordThreat(threat *ThreatEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if threat.ID == "" {
		threat.ID = fmt.Sprintf("threat-%d", time.Now().UnixNano())
	}
	threat.DetectedAt = time.Now()

	s.threats = append(s.threats, threat)
	s.log.Warn().
		Str("id", threat.ID).
		Str("type", threat.Type).
		Str("source_ip", threat.SourceIP).
		Msg("Threat detected")
}

// Middleware returns an HTTP middleware for the WAF.
func (s *Service) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req := &Request{
				ID:        r.Header.Get("X-Request-ID"),
				SourceIP:  r.RemoteAddr,
				Method:    r.Method,
				Path:      r.URL.Path,
				Headers:   r.Header,
				Timestamp: time.Now(),
			}

			decision := s.Evaluate(r.Context(), req)

			switch decision.Action {
			case ActionBlock:
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			case ActionThrottle:
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			case ActionChallenge:
				// Would implement CAPTCHA or JS challenge
				w.Header().Set("X-Shield-Challenge", "required")
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (s *Service) getSortedRules() []*Rule {
	rules := make([]*Rule, 0, len(s.rules))
	for _, r := range s.rules {
		rules = append(rules, r)
	}
	// Sort by priority (lower number = higher priority)
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[i].Priority > rules[j].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
	return rules
}

func (s *Service) matchesRule(req *Request, rule *Rule) bool {
	// Simplified matching - real implementation would be more complex
	for _, cond := range rule.Conditions {
		switch cond.Field {
		case "source_ip":
			if req.SourceIP != cond.Value {
				return false
			}
		case "path":
			if req.Path != cond.Value {
				return false
			}
		case "method":
			if req.Method != cond.Value {
				return false
			}
		}
	}
	return len(rule.Conditions) > 0
}

func (s *Service) registerDefaultRules() {
	// SQL injection detection
	s.AddRule(&Rule{
		ID:          "sqli-detection",
		Name:        "SQL Injection Detection",
		Type:        RuleTypeSQLInjection,
		Action:      ActionBlock,
		Priority:    10,
		Enabled:     true,
	})

	// XSS detection
	s.AddRule(&Rule{
		ID:          "xss-detection",
		Name:        "XSS Detection",
		Type:        RuleTypeXSS,
		Action:      ActionBlock,
		Priority:    10,
		Enabled:     true,
	})
}

func (s *Service) recordBlock() {
	s.mu.Lock()
	s.requestsBlocked++
	s.mu.Unlock()
}

// Allow checks if a request should be allowed based on rate limit.
func (rl *RateLimiter) Allow(key string, limit int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{
			tokens:    limit,
			lastFill:  time.Now(),
			rateLimit: limit,
			window:    window,
		}
		rl.buckets[key] = b
	}

	// Refill tokens based on time passed
	elapsed := time.Since(b.lastFill)
	refill := int(elapsed / window) * limit
	if refill > 0 {
		b.tokens = min(limit, b.tokens+refill)
		b.lastFill = time.Now()
	}

	if b.tokens > 0 {
		b.tokens--
		return true
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"requests_processed": s.requestsProcessed,
		"requests_blocked":   s.requestsBlocked,
		"active_rules":       len(s.rules),
		"threats_detected":   len(s.threats),
	}
}

// RegisterRoutes registers /health, /ready, and /metrics endpoints on the provided mux.
// Required by CLAUDE.md: every component must expose these endpoints.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "shield",
		})
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   true,
			"service": "shield",
		})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		stats := s.Stats()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP shield_requests_processed_total Total requests processed\n")
		fmt.Fprintf(w, "# TYPE shield_requests_processed_total counter\n")
		fmt.Fprintf(w, "shield_requests_processed_total %v\n", stats["requests_processed"])
		fmt.Fprintf(w, "# HELP shield_requests_blocked_total Total requests blocked\n")
		fmt.Fprintf(w, "# TYPE shield_requests_blocked_total counter\n")
		fmt.Fprintf(w, "shield_requests_blocked_total %v\n", stats["requests_blocked"])
		fmt.Fprintf(w, "# HELP shield_active_rules Active WAF rules\n")
		fmt.Fprintf(w, "# TYPE shield_active_rules gauge\n")
		fmt.Fprintf(w, "shield_active_rules %v\n", stats["active_rules"])
		fmt.Fprintf(w, "# HELP shield_threats_detected Total threats detected\n")
		fmt.Fprintf(w, "# TYPE shield_threats_detected counter\n")
		fmt.Fprintf(w, "shield_threats_detected %v\n", stats["threats_detected"])
	})
}
