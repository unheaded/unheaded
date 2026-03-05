// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package rules provides the rule evaluation engine
package rules

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// MatchResult represents the result of a rule match
type MatchResult struct {
	Rule    *Rule
	Target  Target
	Matched string
	Input   string
}

// EvaluationResult represents the complete evaluation result
type EvaluationResult struct {
	Matches     []*MatchResult
	TotalScore  int
	HighestSeverity Severity
	Action      Action
}

// Engine evaluates rules against HTTP requests
type Engine struct {
	ruleSets []*RuleSet
	mu       sync.RWMutex

	// Thresholds
	blockThreshold int
	logThreshold   int
}

// NewEngine creates a new rule evaluation engine
func NewEngine() *Engine {
	return &Engine{
		ruleSets:       make([]*RuleSet, 0),
		blockThreshold: 15, // Default threshold for blocking
		logThreshold:   5,  // Default threshold for logging
	}
}

// SetBlockThreshold sets the anomaly score threshold for blocking
func (e *Engine) SetBlockThreshold(threshold int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.blockThreshold = threshold
}

// SetLogThreshold sets the anomaly score threshold for logging
func (e *Engine) SetLogThreshold(threshold int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.logThreshold = threshold
}

// AddRuleSet adds a rule set to the engine
func (e *Engine) AddRuleSet(rs *RuleSet) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ruleSets = append(e.ruleSets, rs)
}

// RemoveRuleSet removes a rule set by name
func (e *Engine) RemoveRuleSet(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, rs := range e.ruleSets {
		if rs.Name == name {
			e.ruleSets = append(e.ruleSets[:i], e.ruleSets[i+1:]...)
			return true
		}
	}
	return false
}

// Evaluate evaluates all rules against an HTTP request
func (e *Engine) Evaluate(ctx context.Context, req *http.Request) (*EvaluationResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := &EvaluationResult{
		Matches:         make([]*MatchResult, 0),
		HighestSeverity: SeverityInfo,
		Action:          ActionAllow,
	}

	// Extract request data
	requestData := e.extractRequestData(req)

	// Evaluate all rule sets
	for _, rs := range e.ruleSets {
		for _, rule := range rs.ListRules() {
			if !rule.IsEnabled() {
				continue
			}

			// Check each target
			for _, target := range rule.Targets {
				inputs := e.getTargetInputs(target, requestData)
				for _, input := range inputs {
					if rule.Match(input) {
						match := &MatchResult{
							Rule:    rule,
							Target:  target,
							Matched: strings.Join(rule.FindMatches(input), ", "),
							Input:   truncate(input, 200),
						}
						result.Matches = append(result.Matches, match)
						result.TotalScore += rule.Score

						if rule.Severity > result.HighestSeverity {
							result.HighestSeverity = rule.Severity
						}

						// Immediate action rules
						if rule.Action == ActionBlock {
							result.Action = ActionBlock
						} else if rule.Action == ActionRedirect && result.Action != ActionBlock {
							result.Action = ActionRedirect
						}
					}
				}
			}

			// Check context cancellation
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			default:
			}
		}
	}

	// Determine action based on score thresholds
	if result.Action == ActionAllow {
		if result.TotalScore >= e.blockThreshold {
			result.Action = ActionBlock
		} else if result.TotalScore >= e.logThreshold {
			result.Action = ActionLog
		}
	}

	return result, nil
}

// requestData holds extracted request data
type requestData struct {
	URL       string
	Path      string
	Query     string
	QueryMap  url.Values
	Body      string
	Headers   http.Header
	Cookies   []*http.Cookie
	Method    string
	IP        string
	UserAgent string
}

// extractRequestData extracts all relevant data from a request
func (e *Engine) extractRequestData(req *http.Request) *requestData {
	data := &requestData{
		URL:       req.URL.String(),
		Path:      req.URL.Path,
		Query:     req.URL.RawQuery,
		QueryMap:  req.URL.Query(),
		Headers:   req.Header,
		Cookies:   req.Cookies(),
		Method:    req.Method,
		UserAgent: req.UserAgent(),
	}

	// Extract client IP — use RemoteAddr as primary (cannot be spoofed).
	// Only trust X-Forwarded-For / X-Real-IP when RemoteAddr is a private
	// (internal proxy) IP, preventing IP spoofing via crafted headers.
	data.IP = extractSecureClientIP(req)

	// Read body (limited)
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(req.Body, 1024*1024)) // 1MB limit
		if err == nil {
			data.Body = string(bodyBytes)
		}
	}

	return data
}

// getTargetInputs returns the input strings for a given target
func (e *Engine) getTargetInputs(target Target, data *requestData) []string {
	switch target {
	case TargetURL:
		return []string{data.URL}
	case TargetPath:
		return []string{data.Path}
	case TargetQuery:
		inputs := []string{data.Query}
		for key, values := range data.QueryMap {
			inputs = append(inputs, key)
			inputs = append(inputs, values...)
		}
		return inputs
	case TargetBody:
		return []string{data.Body}
	case TargetHeaders:
		var inputs []string
		for key, values := range data.Headers {
			inputs = append(inputs, key)
			inputs = append(inputs, values...)
		}
		return inputs
	case TargetCookies:
		var inputs []string
		for _, cookie := range data.Cookies {
			inputs = append(inputs, cookie.Name)
			inputs = append(inputs, cookie.Value)
		}
		return inputs
	case TargetMethod:
		return []string{data.Method}
	case TargetIP:
		return []string{data.IP}
	case TargetUserAgent:
		return []string{data.UserAgent}
	case TargetAll:
		var inputs []string
		inputs = append(inputs, data.URL, data.Path, data.Query, data.Body, data.Method, data.IP, data.UserAgent)
		for _, values := range data.Headers {
			inputs = append(inputs, values...)
		}
		for key, values := range data.QueryMap {
			inputs = append(inputs, key)
			inputs = append(inputs, values...)
		}
		return inputs
	default:
		return nil
	}
}

// truncate truncates a string to the specified length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GetAllRules returns all rules from all rule sets
func (e *Engine) GetAllRules() []*Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var rules []*Rule
	for _, rs := range e.ruleSets {
		rules = append(rules, rs.ListRules()...)
	}
	return rules
}

// FindRule finds a rule by ID across all rule sets
func (e *Engine) FindRule(id string) *Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, rs := range e.ruleSets {
		if rule := rs.GetRule(id); rule != nil {
			return rule
		}
	}
	return nil
}

// RemoveRule removes a rule by ID from all rule sets
func (e *Engine) RemoveRule(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, rs := range e.ruleSets {
		if rs.RemoveRule(id) {
			return true
		}
	}
	return false
}

// extractSecureClientIP returns the client IP using RemoteAddr as primary source.
// X-Forwarded-For and X-Real-IP are only trusted when the direct peer is
// an internal (private/loopback) IP — i.e. traffic arrived through our own proxy.
func extractSecureClientIP(req *http.Request) string {
	remoteIP := req.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteIP); err == nil {
		remoteIP = host
	}

	// Only trust forwarded headers from internal proxies
	if ip := net.ParseIP(remoteIP); ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			if idx := strings.Index(xff, ","); idx != -1 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}
		if xri := req.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}
