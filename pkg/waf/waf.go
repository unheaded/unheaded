// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package waf provides a Web Application Firewall implementation
// using only Go standard library - THE SHIELD of the Unheaded Kingdom
// TODO: Marked for Rust rebuild for performance - this Go implementation serves as reference
package waf

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/waf/detection"
	"unheaded/pkg/waf/ip"
	"unheaded/pkg/waf/ratelimit"
	"unheaded/pkg/waf/response"
	"unheaded/pkg/waf/rules"
)

// Action represents what the WAF should do
type Action int

const (
	ActionAllow Action = iota
	ActionBlock
	ActionLog
	ActionRedirect
	ActionChallenge
)

// String returns the string representation of an action
func (a Action) String() string {
	switch a {
	case ActionAllow:
		return "allow"
	case ActionBlock:
		return "block"
	case ActionLog:
		return "log"
	case ActionRedirect:
		return "redirect"
	case ActionChallenge:
		return "challenge"
	default:
		return "unknown"
	}
}

// Rule represents a WAF rule (alias for rules.Rule)
type Rule = rules.Rule

// Decision represents the WAF's decision on a request
type Decision struct {
	Action     Action
	Rule       *Rule
	Score      int
	Reason     string
	RequestID  string
	Matches    []string
	Blocked    bool
	RateLimited bool
}

// WAF is the main WAF interface
type WAF interface {
	Process(ctx context.Context, req *http.Request) (*Decision, error)
	AddRule(rule *Rule) error
	RemoveRule(id string) error
	ListRules() []*Rule
	Block(ip string, duration time.Duration) error
	Allow(ip string) error
}

// Config holds WAF configuration
// TODO: Port config to Rust with hot-reload support
type Config struct {
	// Detection settings
	EnableSQLi      bool
	EnableXSS       bool
	EnableTraversal bool
	EnableRCE       bool
	EnableSSRF      bool
	EnableBot       bool // Bot detection

	// Rate limiting
	EnableRateLimit bool
	RateLimit       int           // Requests per window
	RateWindow      time.Duration // Time window

	// Per-endpoint rate limiting
	EnableEndpointRateLimit bool
	EndpointRateLimits      map[string]int // Path prefix -> rate limit

	// Scoring
	BlockThreshold int // Anomaly score threshold for blocking
	LogThreshold   int // Anomaly score threshold for logging
	EnableScoring  bool // Enable advanced scoring engine

	// Response
	JSONResponse bool
	BlockPage    string
	RedirectURL  string

	// Strict mode enables additional checks
	StrictMode bool

	// SkipDefaultRules skips loading default rules (useful for testing)
	SkipDefaultRules bool
}

// DefaultConfig returns the default WAF configuration
func DefaultConfig() *Config {
	return &Config{
		EnableSQLi:              true,
		EnableXSS:               true,
		EnableTraversal:         true,
		EnableRCE:               true,
		EnableSSRF:              true,
		EnableBot:               true,
		EnableRateLimit:         true,
		RateLimit:               100,
		RateWindow:              time.Minute,
		EnableEndpointRateLimit: false,
		EndpointRateLimits:      make(map[string]int),
		BlockThreshold:          15,
		LogThreshold:            5,
		EnableScoring:           true,
		JSONResponse:            false,
		StrictMode:              false,
	}
}

// Shield is the main WAF implementation - THE SHIELD
// TODO: Port to Rust with zero-copy request processing
type Shield struct {
	config *Config

	// Rule engine
	engine *rules.Engine

	// Detectors
	sqliDetector      *detection.SQLiDetector
	xssDetector       *detection.XSSDetector
	traversalDetector *detection.TraversalDetector
	rceDetector       *detection.RCEDetector
	ssrfDetector      *detection.SSRFDetector
	botDetector       *detection.BotDetector

	// Scoring engine
	scoringEngine *ScoringEngine

	// IP management
	ipManager *ip.IPManager

	// Rate limiting
	rateLimiter     *ratelimit.SlidingWindow
	tokenBucket     *ratelimit.TokenBucket
	endpointLimiter *ratelimit.EndpointLimiter

	// Response handling
	responseHandler *response.Handler
	blockWriter     *response.BlockWriter

	// Logging
	logger Logger

	// Metrics
	metrics *Metrics

	mu sync.RWMutex
}

// Logger interface for WAF logging
type Logger interface {
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, fields map[string]interface{})
}

// Metrics holds WAF metrics
// TODO: Port to Rust with atomic counters
type Metrics struct {
	TotalRequests     int64
	BlockedRequests   int64
	AllowedRequests   int64
	RateLimited       int64
	SQLiDetected      int64
	XSSDetected       int64
	TraversalDetected int64
	RCEDetected       int64
	SSRFDetected      int64
	BotDetected       int64
	ChallengeIssued   int64
	AverageScore      float64
	HighScoreCount    int64 // Requests with score > 50
	mu                sync.RWMutex
}

// NewMetrics creates new metrics
func NewMetrics() *Metrics {
	return &Metrics{}
}

// IncrementTotal increments total requests
func (m *Metrics) IncrementTotal() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalRequests++
}

// IncrementBlocked increments blocked requests
func (m *Metrics) IncrementBlocked() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlockedRequests++
}

// IncrementAllowed increments allowed requests
func (m *Metrics) IncrementAllowed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AllowedRequests++
}

// NewShield creates a new WAF Shield instance
// TODO: Port to Rust with builder pattern
func NewShield(config *Config) *Shield {
	if config == nil {
		config = DefaultConfig()
	}

	shield := &Shield{
		config:          config,
		engine:          rules.NewEngine(),
		ipManager:       ip.NewIPManager(),
		responseHandler: response.NewHandler(),
		metrics:         NewMetrics(),
	}

	// Initialize detectors with strict mode
	strictMode := config.StrictMode

	if config.EnableSQLi {
		shield.sqliDetector = detection.NewSQLiDetector(strictMode)
	}
	if config.EnableXSS {
		shield.xssDetector = detection.NewXSSDetector(strictMode)
	}
	if config.EnableTraversal {
		shield.traversalDetector = detection.NewTraversalDetector(strictMode)
	}
	if config.EnableRCE {
		shield.rceDetector = detection.NewRCEDetector(strictMode)
	}
	if config.EnableSSRF {
		shield.ssrfDetector = detection.NewSSRFDetector(strictMode)
	}
	if config.EnableBot {
		shield.botDetector = detection.NewBotDetector(strictMode)
	}

	// Initialize scoring engine
	if config.EnableScoring {
		shield.scoringEngine = NewScoringEngine(config.BlockThreshold, config.LogThreshold)
	}

	// Initialize rate limiter
	if config.EnableRateLimit {
		shield.rateLimiter = ratelimit.NewSlidingWindow(config.RateLimit, config.RateWindow)
		shield.tokenBucket = ratelimit.NewTokenBucket(float64(config.RateLimit)/config.RateWindow.Seconds(), config.RateLimit)
	}

	// Initialize per-endpoint rate limiter
	if config.EnableEndpointRateLimit && len(config.EndpointRateLimits) > 0 {
		shield.endpointLimiter = ratelimit.NewEndpointLimiter(func() ratelimit.Limiter {
			return ratelimit.NewSlidingWindow(100, time.Minute) // Default per endpoint
		})
	}

	// Configure response handler
	shield.responseHandler.SetJSONResponse(config.JSONResponse)
	if config.BlockPage != "" {
		shield.responseHandler.SetBlockPage(config.BlockPage)
	}
	if config.RedirectURL != "" {
		shield.responseHandler.SetRedirectURL(config.RedirectURL)
	}

	shield.blockWriter = response.NewBlockWriter(shield.responseHandler)

	// Set thresholds
	shield.engine.SetBlockThreshold(config.BlockThreshold)
	shield.engine.SetLogThreshold(config.LogThreshold)

	// Load default rules unless explicitly skipped
	if !config.SkipDefaultRules {
		shield.loadDefaultRules()
	}

	return shield
}

// loadDefaultRules loads the default WAF rules
func (s *Shield) loadDefaultRules() {
	defaultRules := rules.NewRuleSet("default", "Default WAF rules")

	// SQL Injection rules
	sqliPatterns := []struct {
		id      string
		name    string
		pattern string
	}{
		{"SQLI-001", "SQL Union Select", `union\s+(all\s+)?select`},
		{"SQLI-002", "SQL Comment", `--\s*$|\/\*.*\*\/`},
		{"SQLI-003", "SQL Boolean Injection", `'\s*(or|and)\s+['0-9]`},
		{"SQLI-004", "SQL Time-based Blind", `(sleep|benchmark|waitfor|delay)\s*\(`},
		{"SQLI-005", "SQL Information Schema", `information_schema`},
	}

	for _, p := range sqliPatterns {
		rule, err := rules.NewRule(p.id, p.name, p.pattern, []rules.Target{rules.TargetAll}, rules.ActionBlock, rules.SeverityHigh)
		if err == nil {
			rule.Message = "SQL injection attempt detected"
			defaultRules.AddRule(rule)
		}
	}

	// XSS rules
	xssPatterns := []struct {
		id      string
		name    string
		pattern string
	}{
		{"XSS-001", "Script Tag", `<\s*script[^>]*>`},
		{"XSS-002", "Event Handler", `\bon\w+\s*=`},
		{"XSS-003", "JavaScript Protocol", `javascript\s*:`},
		{"XSS-004", "SVG XSS", `<\s*svg[^>]*onload`},
		{"XSS-005", "DOM Manipulation", `document\s*\.\s*(cookie|write)`},
	}

	for _, p := range xssPatterns {
		rule, err := rules.NewRule(p.id, p.name, p.pattern, []rules.Target{rules.TargetAll}, rules.ActionBlock, rules.SeverityHigh)
		if err == nil {
			rule.Message = "Cross-site scripting attempt detected"
			defaultRules.AddRule(rule)
		}
	}

	// Path Traversal rules
	traversalPatterns := []struct {
		id      string
		name    string
		pattern string
	}{
		{"TRAV-001", "Directory Traversal", `\.\.\/|\.\.\\`},
		{"TRAV-002", "URL Encoded Traversal", `%2e%2e%2f|%2e%2e\/`},
		{"TRAV-003", "Sensitive File Access", `\/etc\/passwd|\/etc\/shadow`},
		{"TRAV-004", "Windows Path Traversal", `\\windows\\|\\system32\\`},
	}

	for _, p := range traversalPatterns {
		rule, err := rules.NewRule(p.id, p.name, p.pattern, []rules.Target{rules.TargetPath, rules.TargetQuery}, rules.ActionBlock, rules.SeverityHigh)
		if err == nil {
			rule.Message = "Path traversal attempt detected"
			defaultRules.AddRule(rule)
		}
	}

	// RCE rules - target query, body, and path only (not headers, to avoid false positives)
	rcePatterns := []struct {
		id      string
		name    string
		pattern string
	}{
		{"RCE-001", "Command Chaining", `;\s*\w|\|\s*\w|&&\s*\w`},
		{"RCE-002", "Command Substitution", `\$\([^)]+\)`},
		{"RCE-003", "Dangerous Commands", `\b(cat|ls|rm|wget|curl|bash|sh)\s+`},
		{"RCE-004", "Reverse Shell", `\/dev\/tcp\/|mkfifo`},
	}

	for _, p := range rcePatterns {
		// RCE patterns should not target headers as they can cause false positives
		// on Accept headers with quality values (e.g., "text/html;q=0.9")
		rule, err := rules.NewRule(p.id, p.name, p.pattern, []rules.Target{rules.TargetQuery, rules.TargetBody, rules.TargetPath}, rules.ActionBlock, rules.SeverityCritical)
		if err == nil {
			rule.Message = "Remote code execution attempt detected"
			defaultRules.AddRule(rule)
		}
	}

	// SSRF rules
	ssrfPatterns := []struct {
		id      string
		name    string
		pattern string
	}{
		{"SSRF-001", "Localhost Access", `localhost|127\.0\.0\.1|0\.0\.0\.0`},
		{"SSRF-002", "Private IP Range", `10\.\d{1,3}\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3}`},
		{"SSRF-003", "Cloud Metadata", `169\.254\.169\.254|metadata\.google\.internal`},
		{"SSRF-004", "File Protocol", `file:\/\/`},
	}

	for _, p := range ssrfPatterns {
		rule, err := rules.NewRule(p.id, p.name, p.pattern, []rules.Target{rules.TargetURL, rules.TargetBody}, rules.ActionBlock, rules.SeverityHigh)
		if err == nil {
			rule.Message = "Server-side request forgery attempt detected"
			defaultRules.AddRule(rule)
		}
	}

	s.engine.AddRuleSet(defaultRules)
}

// Process processes an HTTP request and returns a decision
// TODO: Port to Rust with parallel detection and early termination
func (s *Shield) Process(ctx context.Context, req *http.Request) (*Decision, error) {
	s.metrics.IncrementTotal()

	decision := &Decision{
		Action:    ActionAllow,
		RequestID: generateRequestID(),
		Matches:   make([]string, 0),
	}

	// Get client IP
	clientIP := s.ipManager.GetClientIP(req)

	// Check IP allowlist first - if allowlisted, skip all checks
	if s.ipManager.Allowlist().Contains(clientIP) {
		decision.Action = ActionAllow
		decision.Reason = "IP is allowlisted"
		s.metrics.IncrementAllowed()
		return decision, nil
	}

	// Check IP blocklist
	if allowed, reason := s.ipManager.CheckRequest(req); !allowed {
		decision.Action = ActionBlock
		decision.Reason = reason
		decision.Blocked = true
		s.metrics.IncrementBlocked()
		return decision, nil
	}

	// Check rate limit
	if s.config.EnableRateLimit && s.rateLimiter != nil {
		if !s.rateLimiter.Allow(clientIP) {
			decision.Action = ActionBlock
			decision.Reason = "Rate limit exceeded"
			decision.RateLimited = true
			decision.Blocked = true
			s.metrics.mu.Lock()
			s.metrics.RateLimited++
			s.metrics.mu.Unlock()
			return decision, nil
		}
	}

	// Check per-endpoint rate limit
	if s.config.EnableEndpointRateLimit && s.endpointLimiter != nil {
		endpoint := req.URL.Path
		if !s.endpointLimiter.Allow(endpoint, clientIP) {
			decision.Action = ActionBlock
			decision.Reason = "Endpoint rate limit exceeded"
			decision.RateLimited = true
			decision.Blocked = true
			s.metrics.mu.Lock()
			s.metrics.RateLimited++
			s.metrics.mu.Unlock()
			return decision, nil
		}
	}

	// Extract request data for detection
	requestData := s.extractRequestData(req)

	// Collect signals for scoring
	signals := make(map[string]*SignalData)

	// Run detectors
	totalScore := 0

	if s.config.EnableSQLi && s.sqliDetector != nil {
		result := s.sqliDetector.DetectWithDetails(requestData)
		if result.Detected {
			totalScore += result.Score
			decision.Matches = append(decision.Matches, result.Matches...)
			signals["sqli"] = &SignalData{
				Type:       "sqli",
				Score:      result.Score,
				Confidence: 0.9,
				Matches:    result.Matches,
			}
			s.metrics.mu.Lock()
			s.metrics.SQLiDetected++
			s.metrics.mu.Unlock()
		}
	}

	if s.config.EnableXSS && s.xssDetector != nil {
		result := s.xssDetector.DetectWithDetails(requestData)
		if result.Detected {
			totalScore += result.Score
			decision.Matches = append(decision.Matches, result.Matches...)
			signals["xss"] = &SignalData{
				Type:       "xss",
				Score:      result.Score,
				Confidence: 0.9,
				Matches:    result.Matches,
			}
			s.metrics.mu.Lock()
			s.metrics.XSSDetected++
			s.metrics.mu.Unlock()
		}
	}

	if s.config.EnableTraversal && s.traversalDetector != nil {
		result := s.traversalDetector.DetectWithDetails(requestData)
		if result.Detected {
			totalScore += result.Score
			decision.Matches = append(decision.Matches, result.Matches...)
			signals["traversal"] = &SignalData{
				Type:       "traversal",
				Score:      result.Score,
				Confidence: 0.85,
				Matches:    result.Matches,
			}
			s.metrics.mu.Lock()
			s.metrics.TraversalDetected++
			s.metrics.mu.Unlock()
		}
	}

	if s.config.EnableRCE && s.rceDetector != nil {
		result := s.rceDetector.DetectWithDetails(requestData)
		if result.Detected {
			totalScore += result.Score
			decision.Matches = append(decision.Matches, result.Matches...)
			signals["rce"] = &SignalData{
				Type:       "rce",
				Score:      result.Score,
				Confidence: 0.95,
				Matches:    result.Matches,
			}
			s.metrics.mu.Lock()
			s.metrics.RCEDetected++
			s.metrics.mu.Unlock()
		}
	}

	if s.config.EnableSSRF && s.ssrfDetector != nil {
		result := s.ssrfDetector.DetectWithDetails(requestData)
		if result.Detected {
			totalScore += result.Score
			decision.Matches = append(decision.Matches, result.Matches...)
			signals["ssrf"] = &SignalData{
				Type:       "ssrf",
				Score:      result.Score,
				Confidence: 0.9,
				Matches:    result.Matches,
			}
			s.metrics.mu.Lock()
			s.metrics.SSRFDetected++
			s.metrics.mu.Unlock()
		}
	}

	// Bot detection
	if s.config.EnableBot && s.botDetector != nil {
		botResult := s.botDetector.DetectWithDetails(req)
		if botResult.Detected {
			totalScore += botResult.Score
			decision.Matches = append(decision.Matches, botResult.Matches...)
			signals["bot"] = &SignalData{
				Type:       "bot",
				Score:      botResult.Score,
				Confidence: 0.7,
				Matches:    botResult.Matches,
			}
			s.metrics.mu.Lock()
			s.metrics.BotDetected++
			s.metrics.mu.Unlock()
		}
	}

	// Run rule engine
	evalResult, err := s.engine.Evaluate(ctx, req)
	if err != nil {
		return decision, err
	}

	totalScore += evalResult.TotalScore
	for _, match := range evalResult.Matches {
		decision.Matches = append(decision.Matches, match.Matched)
		if match.Rule != nil {
			decision.Rule = match.Rule
		}
	}

	if evalResult.TotalScore > 0 {
		signals["rule_match"] = &SignalData{
			Type:       "rule_match",
			Score:      evalResult.TotalScore,
			Confidence: 1.0,
		}
	}

	// Use scoring engine if enabled
	if s.config.EnableScoring && s.scoringEngine != nil && len(signals) > 0 {
		scoringResult := s.scoringEngine.Score(ctx, req, signals)
		totalScore = scoringResult.TotalScore
		decision.Reason = scoringResult.Explanation

		// Track high scores
		if totalScore > 50 {
			s.metrics.mu.Lock()
			s.metrics.HighScoreCount++
			s.metrics.mu.Unlock()
		}
	}

	decision.Score = totalScore

	// Update average score
	s.metrics.mu.Lock()
	currentTotal := s.metrics.TotalRequests
	currentAvg := s.metrics.AverageScore
	s.metrics.AverageScore = (currentAvg*float64(currentTotal-1) + float64(totalScore)) / float64(currentTotal)
	s.metrics.mu.Unlock()

	// Determine action based on score
	if totalScore >= s.config.BlockThreshold {
		decision.Action = ActionBlock
		decision.Blocked = true
		if decision.Reason == "" {
			decision.Reason = fmt.Sprintf("Anomaly score %d exceeds threshold %d", totalScore, s.config.BlockThreshold)
		}
		s.metrics.IncrementBlocked()
	} else if totalScore >= s.config.LogThreshold {
		decision.Action = ActionLog
		if decision.Reason == "" {
			decision.Reason = fmt.Sprintf("Anomaly score %d exceeds log threshold %d", totalScore, s.config.LogThreshold)
		}
		s.metrics.IncrementAllowed()
	} else {
		decision.Action = ActionAllow
		s.metrics.IncrementAllowed()
	}

	return decision, nil
}

// extractRequestData extracts all relevant data from a request for detection
func (s *Shield) extractRequestData(req *http.Request) string {
	var builder strings.Builder

	// URL
	builder.WriteString(req.URL.String())
	builder.WriteString(" ")

	// Query parameters
	for key, values := range req.URL.Query() {
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(strings.Join(values, ","))
		builder.WriteString(" ")
	}

	// Headers
	for key, values := range req.Header {
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(strings.Join(values, ","))
		builder.WriteString(" ")
	}

	// Body (limited)
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, 1024*100)) // 100KB limit
		if err == nil {
			builder.Write(bodyBytes)
		}
	}

	return builder.String()
}

// AddRule adds a rule to the WAF
func (s *Shield) AddRule(rule *Rule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ruleSet := rules.NewRuleSet("custom", "Custom rules")
	ruleSet.AddRule(rule)
	s.engine.AddRuleSet(ruleSet)
	return nil
}

// RemoveRule removes a rule by ID
func (s *Shield) RemoveRule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.engine.RemoveRule(id) {
		return nil
	}
	return fmt.Errorf("rule not found: %s", id)
}

// ListRules returns all rules
func (s *Shield) ListRules() []*Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.engine.GetAllRules()
}

// Block blocks an IP address
func (s *Shield) Block(ipAddr string, duration time.Duration) error {
	s.ipManager.Blocklist().AddWithDuration(ipAddr, "Manually blocked", duration)
	return nil
}

// Allow allows an IP address (adds to allowlist)
func (s *Shield) Allow(ipAddr string) error {
	s.ipManager.Allowlist().Add(ipAddr, "Manually allowed")
	return nil
}

// Middleware returns an HTTP middleware
func (s *Shield) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision, err := s.Process(r.Context(), r)
		if err != nil {
			s.responseHandler.Block(w, "Internal error", "", decision.RequestID)
			return
		}

		// Set request ID header
		w.Header().Set("X-Request-ID", decision.RequestID)

		switch decision.Action {
		case ActionBlock:
			if decision.RateLimited {
				s.responseHandler.RateLimited(w, 60)
			} else {
				ruleID := ""
				if decision.Rule != nil {
					ruleID = decision.Rule.ID
				}
				s.responseHandler.Block(w, decision.Reason, ruleID, decision.RequestID)
			}
			return
		case ActionRedirect:
			s.responseHandler.Redirect(w, r, s.config.RedirectURL)
			return
		case ActionChallenge:
			s.responseHandler.Challenge(w, decision.RequestID)
			return
		case ActionLog:
			// Log but allow
			if s.logger != nil {
				s.logger.Warn("Request flagged", map[string]interface{}{
					"request_id": decision.RequestID,
					"score":      decision.Score,
					"matches":    decision.Matches,
				})
			}
		}

		next.ServeHTTP(w, r)
	})
}

// SetLogger sets the logger
func (s *Shield) SetLogger(logger Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger = logger
}

// GetMetrics returns the current metrics
func (s *Shield) GetMetrics() *Metrics {
	return s.metrics
}

// IPManager returns the IP manager
func (s *Shield) IPManager() *ip.IPManager {
	return s.ipManager
}

// Close closes the WAF and releases resources
func (s *Shield) Close() error {
	if s.rateLimiter != nil {
		_ = s.rateLimiter.Close()
	}
	if s.tokenBucket != nil {
		_ = s.tokenBucket.Close()
	}
	if s.endpointLimiter != nil {
		_ = s.endpointLimiter.Close()
	}
	if s.botDetector != nil {
		s.botDetector.Close()
	}
	if s.scoringEngine != nil {
		s.scoringEngine.Close()
	}
	_ = s.ipManager.Blocklist().Close()
	return nil
}

// BotDetector returns the bot detector
func (s *Shield) BotDetector() *detection.BotDetector {
	return s.botDetector
}

// ScoringEngine returns the scoring engine
func (s *Shield) ScoringEngine() *ScoringEngine {
	return s.scoringEngine
}

// SetStrictMode enables or disables strict mode
func (s *Shield) SetStrictMode(strict bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.StrictMode = strict
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// New creates a new WAF with default configuration
func New() *Shield {
	return NewShield(DefaultConfig())
}

// NewWithConfig creates a new WAF with custom configuration
func NewWithConfig(config *Config) *Shield {
	return NewShield(config)
}
