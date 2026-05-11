// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package detection provides bot detection for THE SHIELD WAF
// Architecture: Go reference implementation with planned Rust acceleration for hot paths.
// See docs/WAF_ARCHITECTURE.md for the two-tier migration strategy.
package detection

import (
	"crypto/md5" //nolint:gosec // bot fingerprint hash, not security primitive
	"encoding/hex"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// BotDetector detects bot traffic using fingerprinting and behavioral analysis
// Future: Rust acceleration planned with ML model inference (tract/candle) for
// advanced bot classification beyond pattern matching.
// See docs/WAF_ARCHITECTURE.md for migration strategy.
type BotDetector struct {
	// Pattern-based detection
	knownBotPatterns   []*regexp.Regexp
	knownGoodBots      map[string]bool
	suspiciousPatterns []*regexp.Regexp

	// Behavioral analysis
	behaviorTracker  *BehaviorTracker
	fingerprintStore *FingerprintStore

	// Configuration
	strict            bool
	enableFingerprint bool
	enableBehavior    bool
}

// BehaviorTracker tracks client behavior patterns
// Performance: Uses exact maps for client tracking in Go reference implementation.
// Rust acceleration planned with probabilistic data structures (HyperLogLog, Bloom filters)
// for memory-efficient tracking at scale. See docs/WAF_ARCHITECTURE.md for migration strategy.
type BehaviorTracker struct {
	clients     map[string]*ClientBehavior
	mu          sync.RWMutex
	windowSize  time.Duration
	maxClients  int
	cleanupTick time.Duration
	stopCh      chan struct{}
}

// ClientBehavior tracks individual client behavior
type ClientBehavior struct {
	FirstSeen       time.Time
	LastSeen        time.Time
	RequestCount    int64
	UniqueEndpoints map[string]int
	Methods         map[string]int
	StatusCodes     map[int]int
	ErrorRate       float64
	RequestRate     float64 // Requests per second
	PathEntropy     float64
	TimingPattern   []int64 // Inter-request timing in milliseconds
	Fingerprint     string
	BotScore        int
	mu              sync.Mutex
}

// FingerprintStore stores browser fingerprints
type FingerprintStore struct {
	fingerprints map[string]*BrowserFingerprint
	mu           sync.RWMutex
}

// BrowserFingerprint represents a browser fingerprint
type BrowserFingerprint struct {
	Hash           string
	UserAgent      string
	AcceptLanguage string
	AcceptEncoding string
	Headers        []string
	HeaderOrder    string
	TLSFingerprint string // JA3 fingerprint if available
	FirstSeen      time.Time
	LastSeen       time.Time
	SeenCount      int
	IsConsistent   bool
	BotProbability float64
}

// BotResult represents the result of bot detection
type BotResult struct {
	IsBot          bool
	BotType        string
	Confidence     float64
	Reasons        []string
	Fingerprint    string
	BehaviorScore  int
	PatternMatches []string
}

// NewBotDetector creates a new bot detector
// Future: Rust acceleration planned with external model loading (ONNX/safetensors)
// for pluggable ML-based bot classification. See docs/WAF_ARCHITECTURE.md for migration strategy.
func NewBotDetector(strict bool) *BotDetector {
	// Known bot patterns (user agent)
	botPatterns := []string{
		// Scanners and security tools
		`(?i)nmap`,
		`(?i)nikto`,
		`(?i)sqlmap`,
		`(?i)acunetix`,
		`(?i)nessus`,
		`(?i)openvas`,
		`(?i)qualys`,
		`(?i)w3af`,
		`(?i)arachni`,
		`(?i)metasploit`,
		`(?i)burp\s*suite`,
		`(?i)owasp`,
		`(?i)zap`,
		`(?i)skipfish`,
		`(?i)wpscan`,
		`(?i)joomscan`,
		`(?i)drupalscan`,
		`(?i)dirbuster`,
		`(?i)gobuster`,
		`(?i)ffuf`,
		`(?i)wfuzz`,
		`(?i)nuclei`,

		// Generic bot patterns
		`(?i)bot\b`,
		`(?i)crawler`,
		`(?i)spider`,
		`(?i)scraper`,
		`(?i)fetcher`,
		`(?i)wget`,
		`(?i)curl\/`,
		`(?i)libwww`,
		`(?i)python-requests`,
		`(?i)python-urllib`,
		`(?i)go-http-client`,
		`(?i)java\/`,
		`(?i)apache-httpclient`,
		`(?i)httpunit`,
		`(?i)httrack`,
		`(?i)telegrambot`,
		`(?i)discordbot`,
		`(?i)slackbot`,

		// Headless browsers
		`(?i)headless`,
		`(?i)phantom`,
		`(?i)puppeteer`,
		`(?i)playwright`,
		`(?i)selenium`,
		`(?i)webdriver`,
		`(?i)chromedriver`,
		`(?i)geckodriver`,

		// Data miners
		`(?i)harvest`,
		`(?i)collector`,
		`(?i)extractor`,
		`(?i)grabber`,

		// Suspicious generic patterns
		`(?i)anonymous`,
		`(?i)masscan`,
		`(?i)shodan`,
		`(?i)censys`,
		`(?i)internetmeasur`,
	}

	// Known good bots (allow these through)
	knownGoodBots := map[string]bool{
		"googlebot":           true,
		"bingbot":             true,
		"yandexbot":           true,
		"duckduckbot":         true,
		"baiduspider":         true,
		"slurp":               true, // Yahoo
		"facebookexternalhit": true,
		"twitterbot":          true,
		"linkedinbot":         true,
		"pinterestbot":        true,
		"applebot":            true,
		"msnbot":              true,
		"uptimerobot":         true,
		"pingdom":             true,
		"newrelicpinger":      true,
		"datadoghq":           true,
	}

	// Suspicious patterns (in request)
	suspiciousPatterns := []string{
		`(?i)union\s+select`,
		`(?i)<script`,
		`(?i)\.\.\/\.\.\/`,
		`(?i)etc\/passwd`,
		`(?i)cmd\.exe`,
		`(?i)\/bin\/bash`,
		`(?i)\$\(.*\)`,
		`(?i)eval\s*\(`,
		`(?i)base64_decode`,
	}

	detector := &BotDetector{
		knownBotPatterns:   make([]*regexp.Regexp, 0),
		knownGoodBots:      knownGoodBots,
		suspiciousPatterns: make([]*regexp.Regexp, 0),
		behaviorTracker:    NewBehaviorTracker(time.Hour, 10000),
		fingerprintStore:   NewFingerprintStore(),
		strict:             strict,
		enableFingerprint:  true,
		enableBehavior:     true,
	}

	for _, p := range botPatterns {
		re, err := regexp.Compile(p)
		if err == nil {
			detector.knownBotPatterns = append(detector.knownBotPatterns, re)
		}
	}

	for _, p := range suspiciousPatterns {
		re, err := regexp.Compile(p)
		if err == nil {
			detector.suspiciousPatterns = append(detector.suspiciousPatterns, re)
		}
	}

	return detector
}

// NewBehaviorTracker creates a new behavior tracker
func NewBehaviorTracker(windowSize time.Duration, maxClients int) *BehaviorTracker {
	bt := &BehaviorTracker{
		clients:     make(map[string]*ClientBehavior),
		windowSize:  windowSize,
		maxClients:  maxClients,
		cleanupTick: time.Minute * 5,
		stopCh:      make(chan struct{}),
	}

	go bt.cleanup()

	return bt
}

// NewFingerprintStore creates a new fingerprint store
func NewFingerprintStore() *FingerprintStore {
	return &FingerprintStore{
		fingerprints: make(map[string]*BrowserFingerprint),
	}
}

// Detect checks if a request is from a bot
func (d *BotDetector) Detect(req *http.Request) *BotResult {
	result := &BotResult{
		Reasons:        make([]string, 0),
		PatternMatches: make([]string, 0),
	}

	ua := req.UserAgent()
	clientIP := getClientIP(req)

	// Check user agent patterns
	d.checkUserAgentPatterns(ua, result)

	// Check if it's a known good bot
	if d.isKnownGoodBot(ua) {
		result.BotType = "good_bot"
		result.IsBot = true
		result.Confidence = 0.95
		return result
	}

	// Browser fingerprinting
	if d.enableFingerprint {
		fingerprint := d.generateFingerprint(req)
		result.Fingerprint = fingerprint.Hash
		d.analyzeFingerprintConsistency(fingerprint, result)
	}

	// Behavioral analysis
	if d.enableBehavior {
		behavior := d.behaviorTracker.Track(clientIP, req)
		d.analyzeBehavior(behavior, result)
		result.BehaviorScore = behavior.BotScore
	}

	// Request content analysis
	d.analyzeRequestContent(req, result)

	// Header analysis
	d.analyzeHeaders(req, result)

	// Calculate final bot score
	d.calculateFinalScore(result)

	return result
}

// checkUserAgentPatterns checks for known bot user agent patterns
func (d *BotDetector) checkUserAgentPatterns(ua string, result *BotResult) {
	for _, pattern := range d.knownBotPatterns {
		if pattern.MatchString(ua) {
			result.PatternMatches = append(result.PatternMatches, pattern.String())
			result.IsBot = true
			result.BotType = "known_bot"
		}
	}

	// Empty or missing user agent
	if ua == "" {
		result.Reasons = append(result.Reasons, "empty_user_agent")
		result.IsBot = true
		result.BotType = "suspicious"
	}

	// Very short user agent
	if len(ua) > 0 && len(ua) < 20 {
		result.Reasons = append(result.Reasons, "short_user_agent")
	}

	// Very long user agent (potential evasion)
	if len(ua) > 500 {
		result.Reasons = append(result.Reasons, "excessive_user_agent_length")
	}
}

// isKnownGoodBot checks if the user agent is from a known good bot
func (d *BotDetector) isKnownGoodBot(ua string) bool {
	lower := strings.ToLower(ua)
	for bot := range d.knownGoodBots {
		if strings.Contains(lower, bot) {
			return true
		}
	}
	return false
}

// generateFingerprint generates a browser fingerprint from request
func (d *BotDetector) generateFingerprint(req *http.Request) *BrowserFingerprint {
	fp := &BrowserFingerprint{
		UserAgent:      req.UserAgent(),
		AcceptLanguage: req.Header.Get("Accept-Language"),
		AcceptEncoding: req.Header.Get("Accept-Encoding"),
		Headers:        make([]string, 0),
		FirstSeen:      time.Now(),
		LastSeen:       time.Now(),
	}

	// Collect header names in order
	for name := range req.Header {
		fp.Headers = append(fp.Headers, strings.ToLower(name))
	}
	sort.Strings(fp.Headers)
	fp.HeaderOrder = strings.Join(fp.Headers, ",")

	// Generate fingerprint hash for grouping similar bot signatures.
	// MD5 is sufficient — this is a non-cryptographic identity bucket,
	// not an integrity or authentication primitive. (gosec G401/G501.)
	data := fp.UserAgent + "|" + fp.AcceptLanguage + "|" + fp.AcceptEncoding + "|" + fp.HeaderOrder
	hash := md5.Sum([]byte(data)) //nolint:gosec // MD5 is fine for fingerprint grouping
	fp.Hash = hex.EncodeToString(hash[:])

	// Store fingerprint
	d.fingerprintStore.Store(fp)

	return fp
}

// analyzeFingerprintConsistency checks fingerprint for bot indicators
func (d *BotDetector) analyzeFingerprintConsistency(fp *BrowserFingerprint, result *BotResult) {
	// Check for missing common headers
	hasAcceptLanguage := fp.AcceptLanguage != ""
	hasAcceptEncoding := fp.AcceptEncoding != ""

	if !hasAcceptLanguage {
		result.Reasons = append(result.Reasons, "missing_accept_language")
	}

	if !hasAcceptEncoding {
		result.Reasons = append(result.Reasons, "missing_accept_encoding")
	}

	// Check header count (browsers typically send 8+ headers)
	if len(fp.Headers) < 5 {
		result.Reasons = append(result.Reasons, "few_headers")
	}

	// Check for uncommon header patterns
	hasReferer := containsHeader(fp.Headers, "referer")
	hasConnection := containsHeader(fp.Headers, "connection")

	if !hasConnection {
		result.Reasons = append(result.Reasons, "missing_connection_header")
	}

	// First request should have referer from same domain or be empty
	// Bots often have random or missing referers
	if !hasReferer && len(fp.Headers) > 8 {
		// Large header set but no referer could indicate scraper
		result.Reasons = append(result.Reasons, "suspicious_header_pattern")
	}
}

// analyzeBehavior analyzes client behavior patterns
func (d *BotDetector) analyzeBehavior(behavior *ClientBehavior, result *BotResult) {
	behavior.mu.Lock()
	defer behavior.mu.Unlock()

	// High request rate
	if behavior.RequestRate > 10 {
		result.Reasons = append(result.Reasons, "high_request_rate")
	}

	// High error rate
	if behavior.ErrorRate > 0.5 {
		result.Reasons = append(result.Reasons, "high_error_rate")
	}

	// Unusual endpoint access patterns
	if len(behavior.UniqueEndpoints) > 100 {
		result.Reasons = append(result.Reasons, "excessive_endpoint_diversity")
	}

	// Analyze timing patterns
	if len(behavior.TimingPattern) >= 5 {
		// Check for machine-like regularity
		if isRegularTiming(behavior.TimingPattern) {
			result.Reasons = append(result.Reasons, "regular_timing_pattern")
		}
	}

	// Only uses specific HTTP methods
	if len(behavior.Methods) == 1 {
		// Using only one method is suspicious for normal browsing
		if _, hasGET := behavior.Methods["GET"]; !hasGET {
			result.Reasons = append(result.Reasons, "unusual_method_distribution")
		}
	}
}

// analyzeRequestContent analyzes request content for suspicious patterns
func (d *BotDetector) analyzeRequestContent(req *http.Request, result *BotResult) {
	// Check path
	path := req.URL.Path
	for _, pattern := range d.suspiciousPatterns {
		if pattern.MatchString(path) {
			result.PatternMatches = append(result.PatternMatches, "path:"+pattern.String())
		}
	}

	// Check query string
	query := req.URL.RawQuery
	for _, pattern := range d.suspiciousPatterns {
		if pattern.MatchString(query) {
			result.PatternMatches = append(result.PatternMatches, "query:"+pattern.String())
		}
	}

	// Suspicious path patterns
	if strings.Contains(path, "..") {
		result.Reasons = append(result.Reasons, "path_traversal_attempt")
	}

	// Admin/sensitive paths without session
	sensitivePathPrefixes := []string{"/admin", "/wp-admin", "/phpmyadmin", "/.git", "/.env"}
	for _, prefix := range sensitivePathPrefixes {
		if strings.HasPrefix(strings.ToLower(path), prefix) {
			result.Reasons = append(result.Reasons, "sensitive_path_access")
			break
		}
	}

	// Check for rapid parameter enumeration
	if len(req.URL.Query()) > 20 {
		result.Reasons = append(result.Reasons, "excessive_parameters")
	}
}

// analyzeHeaders analyzes request headers for bot indicators
func (d *BotDetector) analyzeHeaders(req *http.Request, result *BotResult) {
	// Check for automation headers
	automationHeaders := []string{
		"X-Requested-With", // Often set by automation tools
		"X-Scanner",
		"X-Scan-Memo",
	}

	for _, header := range automationHeaders {
		if req.Header.Get(header) != "" {
			result.Reasons = append(result.Reasons, "automation_header:"+header)
		}
	}

	// Check Accept header
	accept := req.Header.Get("Accept")
	if accept == "*/*" || accept == "" {
		// Generic accept is common in bots
		result.Reasons = append(result.Reasons, "generic_accept_header")
	}

	// Check for Selenium/WebDriver indicators
	if strings.Contains(strings.ToLower(req.UserAgent()), "webdriver") {
		result.Reasons = append(result.Reasons, "webdriver_detected")
		result.BotType = "automation"
	}

	// Check connection header for unusual values
	connection := req.Header.Get("Connection")
	if connection != "" && connection != "keep-alive" && connection != "close" {
		result.Reasons = append(result.Reasons, "unusual_connection_header")
	}
}

// calculateFinalScore calculates the final bot probability
func (d *BotDetector) calculateFinalScore(result *BotResult) {
	score := 0.0

	// Pattern matches are strong indicators
	score += float64(len(result.PatternMatches)) * 0.2

	// Reasons add up
	score += float64(len(result.Reasons)) * 0.1

	// Behavior score contribution
	if result.BehaviorScore > 0 {
		score += float64(result.BehaviorScore) / 100.0 * 0.3
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	result.Confidence = score

	// Set bot status based on threshold
	threshold := 0.5
	if d.strict {
		threshold = 0.3
	}

	if score >= threshold && !result.IsBot {
		result.IsBot = true
		if result.BotType == "" {
			result.BotType = "suspicious"
		}
	}
}

// Track tracks a request for behavior analysis
func (bt *BehaviorTracker) Track(clientID string, req *http.Request) *ClientBehavior {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	behavior, exists := bt.clients[clientID]
	if !exists {
		behavior = &ClientBehavior{
			FirstSeen:       time.Now(),
			UniqueEndpoints: make(map[string]int),
			Methods:         make(map[string]int),
			StatusCodes:     make(map[int]int),
			TimingPattern:   make([]int64, 0),
		}
		bt.clients[clientID] = behavior
	}

	behavior.mu.Lock()
	defer behavior.mu.Unlock()

	now := time.Now()

	// Calculate timing if not first request
	if behavior.RequestCount > 0 {
		delta := now.Sub(behavior.LastSeen).Milliseconds()
		behavior.TimingPattern = append(behavior.TimingPattern, delta)
		// Keep only last 100 timing values
		if len(behavior.TimingPattern) > 100 {
			behavior.TimingPattern = behavior.TimingPattern[1:]
		}
	}

	// Update statistics
	behavior.LastSeen = now
	behavior.RequestCount++
	behavior.UniqueEndpoints[req.URL.Path]++
	behavior.Methods[req.Method]++

	// Calculate request rate
	elapsed := now.Sub(behavior.FirstSeen).Seconds()
	if elapsed > 0 {
		behavior.RequestRate = float64(behavior.RequestCount) / elapsed
	}

	// Calculate bot score based on behavior
	behavior.BotScore = bt.calculateBotScore(behavior)

	return behavior
}

// calculateBotScore calculates a bot score from behavior
func (bt *BehaviorTracker) calculateBotScore(behavior *ClientBehavior) int {
	score := 0

	// High request rate
	if behavior.RequestRate > 5 {
		score += 20
	}
	if behavior.RequestRate > 10 {
		score += 30
	}

	// Many unique endpoints in short time
	endpointCount := len(behavior.UniqueEndpoints)
	if endpointCount > 50 {
		score += 20
	}
	if endpointCount > 100 {
		score += 30
	}

	// Only using one method (usually GET for scrapers)
	if len(behavior.Methods) == 1 {
		score += 10
	}

	// Regular timing patterns (machine-like behavior)
	if len(behavior.TimingPattern) >= 10 && isRegularTiming(behavior.TimingPattern) {
		score += 25
	}

	// High error rate
	if behavior.ErrorRate > 0.3 {
		score += 15
	}
	if behavior.ErrorRate > 0.5 {
		score += 25
	}

	return score
}

// cleanup periodically removes stale entries
func (bt *BehaviorTracker) cleanup() {
	ticker := time.NewTicker(bt.cleanupTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bt.doCleanup()
		case <-bt.stopCh:
			return
		}
	}
}

// doCleanup removes expired entries
func (bt *BehaviorTracker) doCleanup() {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	now := time.Now()
	for clientID, behavior := range bt.clients {
		behavior.mu.Lock()
		if now.Sub(behavior.LastSeen) > bt.windowSize {
			delete(bt.clients, clientID)
		}
		behavior.mu.Unlock()
	}

	// Enforce max clients
	if len(bt.clients) > bt.maxClients {
		// Remove oldest entries
		type clientEntry struct {
			id       string
			lastSeen time.Time
		}

		entries := make([]clientEntry, 0, len(bt.clients))
		for id, behavior := range bt.clients {
			behavior.mu.Lock()
			entries = append(entries, clientEntry{id, behavior.LastSeen})
			behavior.mu.Unlock()
		}

		sort.Slice(entries, func(i, j int) bool {
			return entries[i].lastSeen.Before(entries[j].lastSeen)
		})

		// Remove oldest 10%
		removeCount := len(entries) / 10
		for i := 0; i < removeCount; i++ {
			delete(bt.clients, entries[i].id)
		}
	}
}

// Close stops the behavior tracker
func (bt *BehaviorTracker) Close() {
	close(bt.stopCh)
}

// Store stores a fingerprint
func (fs *FingerprintStore) Store(fp *BrowserFingerprint) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	existing, exists := fs.fingerprints[fp.Hash]
	if exists {
		existing.LastSeen = time.Now()
		existing.SeenCount++
		// Check consistency
		existing.IsConsistent = existing.UserAgent == fp.UserAgent &&
			existing.AcceptLanguage == fp.AcceptLanguage &&
			existing.HeaderOrder == fp.HeaderOrder
	} else {
		fs.fingerprints[fp.Hash] = fp
	}
}

// Get retrieves a fingerprint
func (fs *FingerprintStore) Get(hash string) *BrowserFingerprint {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.fingerprints[hash]
}

// Helper functions

func getClientIP(req *http.Request) string {
	// Use RemoteAddr as primary (cannot be spoofed by clients).
	// Only trust X-Forwarded-For / X-Real-IP from internal proxies.
	remoteIP := req.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteIP); err == nil {
		remoteIP = host
	}

	if ip := net.ParseIP(remoteIP); ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
		if xri := req.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}

func containsHeader(headers []string, name string) bool {
	lower := strings.ToLower(name)
	for _, h := range headers {
		if h == lower {
			return true
		}
	}
	return false
}

// isRegularTiming checks if timing pattern shows machine-like regularity
func isRegularTiming(timings []int64) bool {
	if len(timings) < 5 {
		return false
	}

	// Calculate standard deviation
	var sum int64
	for _, t := range timings {
		sum += t
	}
	mean := float64(sum) / float64(len(timings))

	var varianceSum float64
	for _, t := range timings {
		diff := float64(t) - mean
		varianceSum += diff * diff
	}
	variance := varianceSum / float64(len(timings))

	// Low variance relative to mean indicates regularity
	coefficientOfVariation := variance / (mean * mean)

	// Threshold for "regular" timing - very low CV is suspicious
	return coefficientOfVariation < 0.1
}

// RecordStatusCode records a status code for a client
func (bt *BehaviorTracker) RecordStatusCode(clientID string, statusCode int) {
	bt.mu.RLock()
	behavior, exists := bt.clients[clientID]
	bt.mu.RUnlock()

	if !exists {
		return
	}

	behavior.mu.Lock()
	defer behavior.mu.Unlock()

	behavior.StatusCodes[statusCode]++

	// Calculate error rate
	var errorCount int64
	var totalCount int64
	for code, count := range behavior.StatusCodes {
		totalCount += int64(count)
		if code >= 400 {
			errorCount += int64(count)
		}
	}
	if totalCount > 0 {
		behavior.ErrorRate = float64(errorCount) / float64(totalCount)
	}
}

// GetBehavior retrieves behavior data for a client
func (bt *BehaviorTracker) GetBehavior(clientID string) *ClientBehavior {
	bt.mu.RLock()
	defer bt.mu.RUnlock()
	return bt.clients[clientID]
}

// DetectWithDetails returns detailed bot detection results
func (d *BotDetector) DetectWithDetails(req *http.Request) *DetectionResult {
	botResult := d.Detect(req)

	result := &DetectionResult{
		Type:     "bot",
		Input:    req.URL.String(),
		Detected: botResult.IsBot,
		Matches:  make([]string, 0),
	}

	// Combine all matches
	result.Matches = append(result.Matches, botResult.PatternMatches...)
	result.Matches = append(result.Matches, botResult.Reasons...)

	if result.Detected {
		result.Score = int(botResult.Confidence * 100)
		result.Message = "Bot traffic detected: " + botResult.BotType
	}

	return result
}

// IsGoodBot checks if request is from a known good bot
func (d *BotDetector) IsGoodBot(req *http.Request) bool {
	return d.isKnownGoodBot(req.UserAgent())
}

// SetEnableFingerprinting enables/disables fingerprinting
func (d *BotDetector) SetEnableFingerprinting(enabled bool) {
	d.enableFingerprint = enabled
}

// SetEnableBehavior enables/disables behavioral analysis
func (d *BotDetector) SetEnableBehavior(enabled bool) {
	d.enableBehavior = enabled
}

// Close closes the bot detector
func (d *BotDetector) Close() {
	if d.behaviorTracker != nil {
		d.behaviorTracker.Close()
	}
}

// AddKnownGoodBot adds a known good bot pattern
func (d *BotDetector) AddKnownGoodBot(pattern string) {
	d.knownGoodBots[strings.ToLower(pattern)] = true
}

// AddBotPattern adds a new bot detection pattern
func (d *BotDetector) AddBotPattern(pattern string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	d.knownBotPatterns = append(d.knownBotPatterns, re)
	return nil
}

// GetBehaviorStats returns statistics about tracked behaviors
func (d *BotDetector) GetBehaviorStats() map[string]interface{} {
	d.behaviorTracker.mu.RLock()
	defer d.behaviorTracker.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["tracked_clients"] = len(d.behaviorTracker.clients)

	var totalRequests int64
	var suspiciousCount int
	for _, behavior := range d.behaviorTracker.clients {
		behavior.mu.Lock()
		totalRequests += behavior.RequestCount
		if behavior.BotScore > 50 {
			suspiciousCount++
		}
		behavior.mu.Unlock()
	}

	stats["total_requests"] = totalRequests
	stats["suspicious_clients"] = suspiciousCount

	return stats
}
