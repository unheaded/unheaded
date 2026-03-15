// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package rules provides rule chaining functionality for the WAF
package rules

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ChainOperator defines the logical operator for chaining rules
type ChainOperator int

const (
	// ChainAND requires all rules in the chain to match
	ChainAND ChainOperator = iota
	// ChainOR requires at least one rule in the chain to match
	ChainOR
	// ChainNOT negates the rule result
	ChainNOT
	// ChainXOR requires exactly one rule to match (exclusive or)
	ChainXOR
)

// String returns the string representation of the chain operator
func (c ChainOperator) String() string {
	switch c {
	case ChainAND:
		return "AND"
	case ChainOR:
		return "OR"
	case ChainNOT:
		return "NOT"
	case ChainXOR:
		return "XOR"
	default:
		return "UNKNOWN"
	}
}

// ChainedRule represents a rule that can be part of a chain
type ChainedRule struct {
	*Rule

	// Chain configuration
	Operator    ChainOperator
	Children    []*ChainedRule
	ParentID    string
	ChainID     string
	ChainPhase  int // Evaluation phase (lower = earlier)

	// Rule versioning
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Changelog   []VersionEntry

	// Priority and ordering
	Priority    int // Higher = evaluated first
	Order       int // Within same priority, lower = first

	// Statistics
	stats       *RuleStats

	// Rate limiting per rule
	RateLimit   *RuleRateLimit

	// Exception handling
	Exceptions  []*ExceptionRule

	mu sync.RWMutex
}

// VersionEntry represents a version history entry
type VersionEntry struct {
	Version   int
	Timestamp time.Time
	Changes   string
	Author    string
}

// RuleStats holds statistics for a rule
type RuleStats struct {
	MatchCount     int64
	BlockCount     int64
	AllowCount     int64
	LastMatch      time.Time
	LastBlock      time.Time
	TotalScore     int64
	AverageLatency float64
	latencySum     int64
	latencyCount   int64
	mu             sync.RWMutex
}

// RuleRateLimit provides per-rule rate limiting
type RuleRateLimit struct {
	Enabled     bool
	MaxRequests int
	Window      time.Duration
	Action      Action // What to do when limit exceeded

	// Internal state
	counters    map[string]*rateLimitCounter
	mu          sync.RWMutex
}

type rateLimitCounter struct {
	count     int
	resetTime time.Time
}

// ExceptionRule defines conditions where a rule should not apply
type ExceptionRule struct {
	ID          string
	Name        string
	Description string

	// Conditions for exception
	SourceIPs   []string // IPs or CIDR ranges to exclude
	Paths       []string // URL paths to exclude
	Methods     []string // HTTP methods to exclude
	Headers     map[string]string // Header conditions
	UserAgents  []string // User agent patterns to exclude
	Countries   []string // Country codes to exclude

	// Time-based exceptions
	StartTime   *time.Time
	EndTime     *time.Time
	DaysOfWeek  []time.Weekday
	TimeRanges  []TimeRange

	Enabled     bool
	Priority    int
}

// TimeRange represents a time range within a day
type TimeRange struct {
	Start time.Duration // Time since midnight
	End   time.Duration
}

// NewChainedRule creates a new chained rule from an existing rule
func NewChainedRule(rule *Rule) *ChainedRule {
	return &ChainedRule{
		Rule:       rule,
		Operator:   ChainAND,
		Children:   make([]*ChainedRule, 0),
		ChainPhase: 1,
		Version:    1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Changelog:  make([]VersionEntry, 0),
		Priority:   0,
		Order:      0,
		stats:      NewRuleStats(),
		Exceptions: make([]*ExceptionRule, 0),
	}
}

// NewRuleStats creates a new rule statistics tracker
func NewRuleStats() *RuleStats {
	return &RuleStats{}
}

// RecordMatch records a rule match
func (s *RuleStats) RecordMatch(blocked bool, score int, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	atomic.AddInt64(&s.MatchCount, 1)
	s.LastMatch = time.Now()

	if blocked {
		atomic.AddInt64(&s.BlockCount, 1)
		s.LastBlock = time.Now()
	} else {
		atomic.AddInt64(&s.AllowCount, 1)
	}

	atomic.AddInt64(&s.TotalScore, int64(score))
	s.latencySum += latency.Nanoseconds()
	s.latencyCount++
	if s.latencyCount > 0 {
		s.AverageLatency = float64(s.latencySum) / float64(s.latencyCount) / 1e6 // Convert to ms
	}
}

// GetStats returns a copy of the statistics
func (s *RuleStats) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"match_count":     s.MatchCount,
		"block_count":     s.BlockCount,
		"allow_count":     s.AllowCount,
		"last_match":      s.LastMatch,
		"last_block":      s.LastBlock,
		"total_score":     s.TotalScore,
		"average_latency": s.AverageLatency,
	}
}

// NewRuleRateLimit creates a new per-rule rate limiter
func NewRuleRateLimit(maxRequests int, window time.Duration, action Action) *RuleRateLimit {
	return &RuleRateLimit{
		Enabled:     true,
		MaxRequests: maxRequests,
		Window:      window,
		Action:      action,
		counters:    make(map[string]*rateLimitCounter),
	}
}

// Allow checks if a request from the given key is allowed
func (rl *RuleRateLimit) Allow(key string) bool {
	if !rl.Enabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	counter, exists := rl.counters[key]
	if !exists || now.After(counter.resetTime) {
		rl.counters[key] = &rateLimitCounter{
			count:     1,
			resetTime: now.Add(rl.Window),
		}
		return true
	}

	if counter.count >= rl.MaxRequests {
		return false
	}

	counter.count++
	return true
}

// Reset resets the rate limiter for a key
func (rl *RuleRateLimit) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.counters, key)
}

// AddChild adds a child rule to the chain
func (cr *ChainedRule) AddChild(child *ChainedRule) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	child.ParentID = cr.ID
	child.ChainID = cr.ChainID
	cr.Children = append(cr.Children, child)
}

// RemoveChild removes a child rule from the chain
func (cr *ChainedRule) RemoveChild(ruleID string) bool {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	for i, child := range cr.Children {
		if child.ID == ruleID {
			cr.Children = append(cr.Children[:i], cr.Children[i+1:]...)
			return true
		}
	}
	return false
}

// AddException adds an exception rule
func (cr *ChainedRule) AddException(exception *ExceptionRule) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.Exceptions = append(cr.Exceptions, exception)
}

// RemoveException removes an exception rule
func (cr *ChainedRule) RemoveException(exceptionID string) bool {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	for i, exc := range cr.Exceptions {
		if exc.ID == exceptionID {
			cr.Exceptions = append(cr.Exceptions[:i], cr.Exceptions[i+1:]...)
			return true
		}
	}
	return false
}

// SetVersion updates the rule version with changelog
func (cr *ChainedRule) SetVersion(version int, changes, author string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	entry := VersionEntry{
		Version:   cr.Version,
		Timestamp: time.Now(),
		Changes:   changes,
		Author:    author,
	}
	cr.Changelog = append(cr.Changelog, entry)
	cr.Version = version
	cr.UpdatedAt = time.Now()
}

// GetStats returns rule statistics
func (cr *ChainedRule) GetStats() *RuleStats {
	return cr.stats
}

// SetRateLimit sets the per-rule rate limit
func (cr *ChainedRule) SetRateLimit(maxRequests int, window time.Duration, action Action) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.RateLimit = NewRuleRateLimit(maxRequests, window, action)
}

// DisableRateLimit disables per-rule rate limiting
func (cr *ChainedRule) DisableRateLimit() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if cr.RateLimit != nil {
		cr.RateLimit.Enabled = false
	}
}

// ChainEvaluationResult represents the result of chain evaluation
type ChainEvaluationResult struct {
	Matched          bool
	MatchedRules     []*ChainedRule
	TotalScore       int
	Action           Action
	RateLimited      bool
	Exception        *ExceptionRule // If an exception was applied
	EvaluationTimeNs int64
}

// EvaluateChain evaluates a chained rule against a request
func (cr *ChainedRule) EvaluateChain(ctx context.Context, req *http.Request, data *requestData) *ChainEvaluationResult {
	startTime := time.Now()
	result := &ChainEvaluationResult{
		MatchedRules: make([]*ChainedRule, 0),
		Action:       ActionAllow,
	}

	// Check rate limit first
	if cr.RateLimit != nil && cr.RateLimit.Enabled {
		clientKey := getClientKey(req)
		if !cr.RateLimit.Allow(clientKey) {
			result.RateLimited = true
			result.Action = cr.RateLimit.Action
			result.EvaluationTimeNs = time.Since(startTime).Nanoseconds()
			return result
		}
	}

	// Check exceptions
	if exception := cr.checkExceptions(req, data); exception != nil {
		result.Exception = exception
		result.EvaluationTimeNs = time.Since(startTime).Nanoseconds()
		return result
	}

	// Evaluate based on operator
	var matched bool
	switch cr.Operator {
	case ChainAND:
		matched = cr.evaluateAND(ctx, req, data, result)
	case ChainOR:
		matched = cr.evaluateOR(ctx, req, data, result)
	case ChainNOT:
		matched = cr.evaluateNOT(ctx, req, data, result)
	case ChainXOR:
		matched = cr.evaluateXOR(ctx, req, data, result)
	default:
		matched = cr.evaluateSingle(ctx, req, data, result)
	}

	result.Matched = matched
	if matched {
		result.MatchedRules = append(result.MatchedRules, cr)
		result.TotalScore += cr.Score
		result.Action = cr.Action

		// Record statistics
		cr.stats.RecordMatch(cr.Action == ActionBlock, cr.Score, time.Since(startTime))
	}

	result.EvaluationTimeNs = time.Since(startTime).Nanoseconds()
	return result
}

// evaluateAND evaluates all children with AND logic
func (cr *ChainedRule) evaluateAND(ctx context.Context, req *http.Request, data *requestData, result *ChainEvaluationResult) bool {
	// First check the rule itself
	if cr.Rule != nil && !cr.evaluateSingle(ctx, req, data, result) {
		return false
	}

	// Then check all children - all must match
	for _, child := range cr.Children {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		childResult := child.EvaluateChain(ctx, req, data)
		if !childResult.Matched {
			return false
		}
		result.MatchedRules = append(result.MatchedRules, childResult.MatchedRules...)
		result.TotalScore += childResult.TotalScore
	}

	return true
}

// evaluateOR evaluates children with OR logic
func (cr *ChainedRule) evaluateOR(ctx context.Context, req *http.Request, data *requestData, result *ChainEvaluationResult) bool {
	// Check the rule itself first
	if cr.Rule != nil && cr.evaluateSingle(ctx, req, data, result) {
		return true
	}

	// Then check children - at least one must match
	for _, child := range cr.Children {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		childResult := child.EvaluateChain(ctx, req, data)
		if childResult.Matched {
			result.MatchedRules = append(result.MatchedRules, childResult.MatchedRules...)
			result.TotalScore += childResult.TotalScore
			return true
		}
	}

	return false
}

// evaluateNOT negates the rule result
func (cr *ChainedRule) evaluateNOT(ctx context.Context, req *http.Request, data *requestData, result *ChainEvaluationResult) bool {
	// NOT of the primary rule
	if cr.Rule != nil {
		return !cr.evaluateSingle(ctx, req, data, result)
	}

	// If only children, NOT of the first child
	if len(cr.Children) > 0 {
		childResult := cr.Children[0].EvaluateChain(ctx, req, data)
		return !childResult.Matched
	}

	return false
}

// evaluateXOR evaluates children with XOR logic
func (cr *ChainedRule) evaluateXOR(ctx context.Context, req *http.Request, data *requestData, result *ChainEvaluationResult) bool {
	matchCount := 0

	// Check the rule itself
	if cr.Rule != nil && cr.evaluateSingle(ctx, req, data, result) {
		matchCount++
	}

	// Check children
	for _, child := range cr.Children {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		childResult := child.EvaluateChain(ctx, req, data)
		if childResult.Matched {
			matchCount++
			result.MatchedRules = append(result.MatchedRules, childResult.MatchedRules...)
			result.TotalScore += childResult.TotalScore
		}
	}

	// XOR: exactly one must match
	return matchCount == 1
}

// evaluateSingle evaluates just this rule (not children)
func (cr *ChainedRule) evaluateSingle(ctx context.Context, req *http.Request, data *requestData, result *ChainEvaluationResult) bool {
	if cr.Rule == nil || !cr.Rule.IsEnabled() {
		return false
	}

	// Get inputs for the rule's targets
	for _, target := range cr.Targets {
		inputs := getTargetInputs(target, data)
		for _, input := range inputs {
			if cr.Rule.Match(input) {
				return true
			}
		}
	}

	return false
}

// checkExceptions checks if any exception rule applies
func (cr *ChainedRule) checkExceptions(req *http.Request, data *requestData) *ExceptionRule {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	for _, exc := range cr.Exceptions {
		if !exc.Enabled {
			continue
		}

		if exc.matches(req, data) {
			return exc
		}
	}

	return nil
}

// matches checks if the exception applies to the request
func (exc *ExceptionRule) matches(req *http.Request, data *requestData) bool {
	// Check time-based conditions
	if !exc.isWithinTimeWindow() {
		return false
	}

	// Check IP
	if len(exc.SourceIPs) > 0 && !exc.matchesIP(data.IP) {
		return false
	}

	// Check path
	if len(exc.Paths) > 0 && !exc.matchesPath(req.URL.Path) {
		return false
	}

	// Check method
	if len(exc.Methods) > 0 && !exc.matchesMethod(req.Method) {
		return false
	}

	// Check headers
	if len(exc.Headers) > 0 && !exc.matchesHeaders(req.Header) {
		return false
	}

	// Check user agent
	if len(exc.UserAgents) > 0 && !exc.matchesUserAgent(req.UserAgent()) {
		return false
	}

	return true
}

// isWithinTimeWindow checks if current time is within exception window
func (exc *ExceptionRule) isWithinTimeWindow() bool {
	now := time.Now()

	// Check start/end time
	if exc.StartTime != nil && now.Before(*exc.StartTime) {
		return false
	}
	if exc.EndTime != nil && now.After(*exc.EndTime) {
		return false
	}

	// Check day of week
	if len(exc.DaysOfWeek) > 0 {
		today := now.Weekday()
		found := false
		for _, day := range exc.DaysOfWeek {
			if day == today {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check time ranges
	if len(exc.TimeRanges) > 0 {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		currentDuration := now.Sub(midnight)

		inRange := false
		for _, tr := range exc.TimeRanges {
			if currentDuration >= tr.Start && currentDuration <= tr.End {
				inRange = true
				break
			}
		}
		if !inRange {
			return false
		}
	}

	return true
}

// matchesIP checks if the IP matches any in the exception list
func (exc *ExceptionRule) matchesIP(ip string) bool {
	for _, excIP := range exc.SourceIPs {
		if excIP == ip {
			return true
		}
		// Could add CIDR support here
	}
	return false
}

// matchesPath checks if the path matches any in the exception list
func (exc *ExceptionRule) matchesPath(path string) bool {
	for _, excPath := range exc.Paths {
		if excPath == path {
			return true
		}
		// Support prefix matching
		if len(excPath) > 0 && excPath[len(excPath)-1] == '*' {
			prefix := excPath[:len(excPath)-1]
			if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}

// matchesMethod checks if the method matches any in the exception list
func (exc *ExceptionRule) matchesMethod(method string) bool {
	for _, excMethod := range exc.Methods {
		if excMethod == method {
			return true
		}
	}
	return false
}

// matchesHeaders checks if headers match the exception conditions
func (exc *ExceptionRule) matchesHeaders(headers http.Header) bool {
	for key, value := range exc.Headers {
		headerValue := headers.Get(key)
		if headerValue != value && value != "*" {
			return false
		}
	}
	return true
}

// matchesUserAgent checks if user agent matches any pattern
func (exc *ExceptionRule) matchesUserAgent(ua string) bool {
	for _, excUA := range exc.UserAgents {
		// Simple substring match
		if contains(ua, excUA) {
			return true
		}
	}
	return false
}

// Helper functions

func getClientKey(req *http.Request) string {
	// Use the secure extraction that prefers RemoteAddr over spoofable headers.
	// extractSecureClientIP is defined in engine.go (same package).
	return extractSecureClientIP(req)
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func getTargetInputs(target Target, data *requestData) []string {
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

// RuleChain represents a complete chain of rules
type RuleChain struct {
	ID          string
	Name        string
	Description string
	Root        *ChainedRule
	Rules       map[string]*ChainedRule
	Version     int
	Enabled     bool
	Priority    int

	mu sync.RWMutex
}

// NewRuleChain creates a new rule chain
func NewRuleChain(id, name string) *RuleChain {
	return &RuleChain{
		ID:       id,
		Name:     name,
		Rules:    make(map[string]*ChainedRule),
		Version:  1,
		Enabled:  true,
		Priority: 0,
	}
}

// SetRoot sets the root rule of the chain
func (rc *RuleChain) SetRoot(rule *ChainedRule) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.Root = rule
	rule.ChainID = rc.ID
	rc.Rules[rule.ID] = rule
}

// AddRule adds a rule to the chain
func (rc *RuleChain) AddRule(rule *ChainedRule) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rule.ChainID = rc.ID
	rc.Rules[rule.ID] = rule
}

// GetRule gets a rule from the chain by ID
func (rc *RuleChain) GetRule(ruleID string) *ChainedRule {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.Rules[ruleID]
}

// RemoveRule removes a rule from the chain
func (rc *RuleChain) RemoveRule(ruleID string) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if _, exists := rc.Rules[ruleID]; exists {
		delete(rc.Rules, ruleID)
		return true
	}
	return false
}

// Evaluate evaluates the entire chain against a request
func (rc *RuleChain) Evaluate(ctx context.Context, req *http.Request, data *requestData) *ChainEvaluationResult {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	if !rc.Enabled || rc.Root == nil {
		return &ChainEvaluationResult{
			Matched:      false,
			MatchedRules: make([]*ChainedRule, 0),
		}
	}

	return rc.Root.EvaluateChain(ctx, req, data)
}

// Enable enables the chain
func (rc *RuleChain) Enable() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.Enabled = true
}

// Disable disables the chain
func (rc *RuleChain) Disable() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.Enabled = false
}

// GetStats returns statistics for all rules in the chain
func (rc *RuleChain) GetStats() map[string]interface{} {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	stats := make(map[string]interface{})
	for ruleID, rule := range rc.Rules {
		stats[ruleID] = rule.GetStats().GetStats()
	}
	return stats
}
