// Package waf provides anomaly scoring for THE SHIELD WAF
// TODO: Marked for Rust rebuild for performance - this Go implementation serves as reference
package waf

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ScoringEngine combines multiple detection signals into a final anomaly score
// TODO: Port to Rust with machine learning model integration
type ScoringEngine struct {
	// Weights for different detection types
	weights       map[string]float64

	// Score thresholds
	blockThreshold   int
	logThreshold     int
	warnThreshold    int

	// Historical data for baseline
	baselineTracker  *BaselineTracker

	// Correlation engine
	correlator       *AttackCorrelator

	// Configuration
	enableCorrelation bool
	enableBaseline    bool

	mu sync.RWMutex
}

// BaselineTracker tracks normal behavior baselines per endpoint
// TODO: Rust rebuild should use streaming statistics algorithms
type BaselineTracker struct {
	endpoints     map[string]*EndpointBaseline
	mu            sync.RWMutex
	windowSize    time.Duration
	cleanupTicker *time.Ticker
	stopCh        chan struct{}
}

// EndpointBaseline tracks normal behavior for an endpoint
type EndpointBaseline struct {
	Path              string
	TotalRequests     int64
	NormalScoreMean   float64
	NormalScoreStdDev float64
	RequestRateMean   float64
	LastUpdated       time.Time

	// Rolling statistics
	scores        []int
	requestTimes  []time.Time
	mu            sync.Mutex
}

// AttackCorrelator correlates multiple attack signals
// TODO: Rust rebuild should use graph-based correlation
type AttackCorrelator struct {
	recentAttacks map[string]*AttackContext
	mu            sync.RWMutex
	windowSize    time.Duration
	stopCh        chan struct{}
}

// AttackContext tracks related attack signals
type AttackContext struct {
	SourceIP        string
	FirstSeen       time.Time
	LastSeen        time.Time
	AttackTypes     map[string]int
	TargetEndpoints map[string]int
	TotalScore      int
	RequestCount    int
	Escalating      bool
}

// SignalWeight represents the weight of a detection signal
type SignalWeight struct {
	Type       string
	BaseWeight float64
	Multiplier float64
}

// ScoringResult represents the result of anomaly scoring
type ScoringResult struct {
	TotalScore        int
	NormalizedScore   float64
	Percentile        float64
	Signals           []ScoringSignal
	Recommendation    Action
	Confidence        float64
	IsAnomaly         bool
	CorrelatedAttacks []string
	Explanation       string
}

// ScoringSignal represents a single detection signal
type ScoringSignal struct {
	Type        string
	RawScore    int
	Weight      float64
	WeightedScore float64
	Confidence  float64
	Matches     []string
}

// NewScoringEngine creates a new scoring engine
func NewScoringEngine(blockThreshold, logThreshold int) *ScoringEngine {
	engine := &ScoringEngine{
		weights: map[string]float64{
			"sqli":        1.5,  // SQL injection is critical
			"xss":         1.3,  // XSS is high priority
			"traversal":   1.2,  // Path traversal
			"rce":         2.0,  // RCE is most critical
			"ssrf":        1.4,  // SSRF is high priority
			"bot":         0.8,  // Bot detection is supporting signal
			"rate_limit":  1.0,  // Rate limiting
			"ip_block":    2.0,  // IP blocklist match
			"rule_match":  1.0,  // Generic rule match
		},
		blockThreshold:    blockThreshold,
		logThreshold:      logThreshold,
		warnThreshold:     logThreshold / 2,
		baselineTracker:   NewBaselineTracker(time.Hour * 24),
		correlator:        NewAttackCorrelator(time.Minute * 15),
		enableCorrelation: true,
		enableBaseline:    true,
	}

	return engine
}

// NewBaselineTracker creates a new baseline tracker
func NewBaselineTracker(windowSize time.Duration) *BaselineTracker {
	bt := &BaselineTracker{
		endpoints:     make(map[string]*EndpointBaseline),
		windowSize:    windowSize,
		cleanupTicker: time.NewTicker(time.Hour),
		stopCh:        make(chan struct{}),
	}

	go bt.cleanup()

	return bt
}

// NewAttackCorrelator creates a new attack correlator
func NewAttackCorrelator(windowSize time.Duration) *AttackCorrelator {
	ac := &AttackCorrelator{
		recentAttacks: make(map[string]*AttackContext),
		windowSize:    windowSize,
		stopCh:        make(chan struct{}),
	}

	go ac.cleanup()

	return ac
}

// Score calculates the anomaly score for a request
// TODO: Rust version should support async signal gathering
func (e *ScoringEngine) Score(ctx context.Context, req *http.Request, signals map[string]*SignalData) *ScoringResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := &ScoringResult{
		Signals:           make([]ScoringSignal, 0),
		CorrelatedAttacks: make([]string, 0),
	}

	totalWeightedScore := 0.0
	totalWeight := 0.0

	// Process each signal
	for signalType, data := range signals {
		weight := e.weights[signalType]
		if weight == 0 {
			weight = 1.0 // Default weight
		}

		signal := ScoringSignal{
			Type:        signalType,
			RawScore:    data.Score,
			Weight:      weight,
			Confidence:  data.Confidence,
			Matches:     data.Matches,
		}

		// Apply weight
		signal.WeightedScore = float64(data.Score) * weight * data.Confidence

		totalWeightedScore += signal.WeightedScore
		totalWeight += weight

		result.Signals = append(result.Signals, signal)
	}

	// Calculate total score
	result.TotalScore = int(totalWeightedScore)

	// Normalize score (0-100 scale)
	if totalWeight > 0 {
		result.NormalizedScore = totalWeightedScore / totalWeight
		if result.NormalizedScore > 100 {
			result.NormalizedScore = 100
		}
	}

	// Baseline comparison
	if e.enableBaseline {
		endpoint := req.URL.Path
		percentile := e.baselineTracker.CompareToBaseline(endpoint, result.TotalScore)
		result.Percentile = percentile

		// Track this score
		e.baselineTracker.Track(endpoint, result.TotalScore)
	}

	// Attack correlation
	if e.enableCorrelation {
		clientIP := extractClientIP(req)
		correlated := e.correlator.Correlate(clientIP, result.TotalScore, signals)
		result.CorrelatedAttacks = correlated

		// Boost score if correlated with ongoing attack
		if len(correlated) > 0 {
			result.TotalScore = int(float64(result.TotalScore) * 1.5)
			result.NormalizedScore *= 1.5
			if result.NormalizedScore > 100 {
				result.NormalizedScore = 100
			}
		}
	}

	// Determine recommendation
	result.Recommendation = e.determineAction(result.TotalScore)

	// Calculate overall confidence
	result.Confidence = e.calculateConfidence(result)

	// Check if anomaly
	result.IsAnomaly = result.TotalScore >= e.warnThreshold

	// Generate explanation
	result.Explanation = e.generateExplanation(result)

	return result
}

// SignalData represents data from a detector
type SignalData struct {
	Type       string
	Score      int
	Confidence float64
	Matches    []string
	Metadata   map[string]interface{}
}

// determineAction determines the recommended action based on score
func (e *ScoringEngine) determineAction(score int) Action {
	if score >= e.blockThreshold {
		return ActionBlock
	}
	if score >= e.logThreshold {
		return ActionLog
	}
	return ActionAllow
}

// calculateConfidence calculates overall confidence in the scoring
func (e *ScoringEngine) calculateConfidence(result *ScoringResult) float64 {
	if len(result.Signals) == 0 {
		return 0.0
	}

	// Average confidence of all signals, weighted by their scores
	totalWeightedConfidence := 0.0
	totalWeight := 0.0

	for _, signal := range result.Signals {
		weight := float64(signal.RawScore)
		if weight > 0 {
			totalWeightedConfidence += signal.Confidence * weight
			totalWeight += weight
		}
	}

	if totalWeight == 0 {
		return 0.5 // Default confidence
	}

	return totalWeightedConfidence / totalWeight
}

// generateExplanation generates a human-readable explanation
func (e *ScoringEngine) generateExplanation(result *ScoringResult) string {
	if len(result.Signals) == 0 {
		return "No suspicious activity detected"
	}

	// Sort signals by weighted score
	sort.Slice(result.Signals, func(i, j int) bool {
		return result.Signals[i].WeightedScore > result.Signals[j].WeightedScore
	})

	var parts []string

	// Top signals
	for i, signal := range result.Signals {
		if i >= 3 || signal.WeightedScore <= 0 {
			break
		}

		part := signal.Type + " (score: " + formatFloat(signal.WeightedScore) + ")"
		if len(signal.Matches) > 0 {
			part += ": " + strings.Join(signal.Matches[:min(3, len(signal.Matches))], ", ")
		}
		parts = append(parts, part)
	}

	if len(parts) == 0 {
		return "Low-level suspicious activity detected"
	}

	explanation := "Detected: " + strings.Join(parts, "; ")

	if result.Percentile > 95 {
		explanation += " [HIGHLY ANOMALOUS: top 5% severity]"
	} else if result.Percentile > 90 {
		explanation += " [ANOMALOUS: top 10% severity]"
	}

	return explanation
}

// Track records a score for baseline calculation
func (bt *BaselineTracker) Track(endpoint string, score int) {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	baseline, exists := bt.endpoints[endpoint]
	if !exists {
		baseline = &EndpointBaseline{
			Path:   endpoint,
			scores: make([]int, 0, 1000),
		}
		bt.endpoints[endpoint] = baseline
	}

	baseline.mu.Lock()
	defer baseline.mu.Unlock()

	baseline.TotalRequests++
	baseline.scores = append(baseline.scores, score)
	baseline.requestTimes = append(baseline.requestTimes, time.Now())
	baseline.LastUpdated = time.Now()

	// Keep only recent data
	if len(baseline.scores) > 1000 {
		baseline.scores = baseline.scores[100:]
		baseline.requestTimes = baseline.requestTimes[100:]
	}

	// Recalculate statistics
	bt.recalculateStats(baseline)
}

// recalculateStats recalculates baseline statistics
func (bt *BaselineTracker) recalculateStats(baseline *EndpointBaseline) {
	if len(baseline.scores) == 0 {
		return
	}

	// Calculate mean
	sum := 0
	for _, s := range baseline.scores {
		sum += s
	}
	baseline.NormalScoreMean = float64(sum) / float64(len(baseline.scores))

	// Calculate standard deviation
	varianceSum := 0.0
	for _, s := range baseline.scores {
		diff := float64(s) - baseline.NormalScoreMean
		varianceSum += diff * diff
	}
	variance := varianceSum / float64(len(baseline.scores))
	baseline.NormalScoreStdDev = sqrt(variance)
}

// CompareToBaseline compares a score to the baseline
func (bt *BaselineTracker) CompareToBaseline(endpoint string, score int) float64 {
	bt.mu.RLock()
	baseline, exists := bt.endpoints[endpoint]
	bt.mu.RUnlock()

	if !exists || len(baseline.scores) < 10 {
		// Not enough data, return 50th percentile
		return 50.0
	}

	baseline.mu.Lock()
	defer baseline.mu.Unlock()

	// Calculate percentile
	count := 0
	for _, s := range baseline.scores {
		if s <= score {
			count++
		}
	}

	return float64(count) / float64(len(baseline.scores)) * 100.0
}

// cleanup periodically removes old data
func (bt *BaselineTracker) cleanup() {
	for {
		select {
		case <-bt.cleanupTicker.C:
			bt.doCleanup()
		case <-bt.stopCh:
			return
		}
	}
}

// doCleanup removes stale endpoint data
func (bt *BaselineTracker) doCleanup() {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	now := time.Now()
	for endpoint, baseline := range bt.endpoints {
		baseline.mu.Lock()
		if now.Sub(baseline.LastUpdated) > bt.windowSize {
			delete(bt.endpoints, endpoint)
		}
		baseline.mu.Unlock()
	}
}

// Close stops the baseline tracker
func (bt *BaselineTracker) Close() {
	bt.cleanupTicker.Stop()
	close(bt.stopCh)
}

// Correlate correlates an attack with recent activity
func (ac *AttackCorrelator) Correlate(sourceIP string, score int, signals map[string]*SignalData) []string {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	var correlated []string

	// Get or create attack context
	ctx, exists := ac.recentAttacks[sourceIP]
	if !exists {
		ctx = &AttackContext{
			SourceIP:        sourceIP,
			FirstSeen:       time.Now(),
			AttackTypes:     make(map[string]int),
			TargetEndpoints: make(map[string]int),
		}
		ac.recentAttacks[sourceIP] = ctx
	}

	// Update context
	ctx.LastSeen = time.Now()
	ctx.RequestCount++
	ctx.TotalScore += score

	// Track attack types
	for signalType, data := range signals {
		if data.Score > 0 {
			ctx.AttackTypes[signalType]++
		}
	}

	// Check for escalation
	avgScore := float64(ctx.TotalScore) / float64(ctx.RequestCount)
	if score > int(avgScore*1.5) && ctx.RequestCount > 3 {
		ctx.Escalating = true
		correlated = append(correlated, "escalating_attack")
	}

	// Check for multi-vector attack
	if len(ctx.AttackTypes) >= 2 {
		correlated = append(correlated, "multi_vector_attack")
	}

	// Check for rapid attack
	if ctx.RequestCount > 10 && time.Since(ctx.FirstSeen) < time.Minute {
		correlated = append(correlated, "rapid_attack")
	}

	return correlated
}

// cleanup periodically removes old attack contexts
func (ac *AttackCorrelator) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ac.doCleanup()
		case <-ac.stopCh:
			return
		}
	}
}

// doCleanup removes expired attack contexts
func (ac *AttackCorrelator) doCleanup() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	now := time.Now()
	for ip, ctx := range ac.recentAttacks {
		if now.Sub(ctx.LastSeen) > ac.windowSize {
			delete(ac.recentAttacks, ip)
		}
	}
}

// Close stops the attack correlator
func (ac *AttackCorrelator) Close() {
	close(ac.stopCh)
}

// SetWeight sets the weight for a signal type
func (e *ScoringEngine) SetWeight(signalType string, weight float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.weights[signalType] = weight
}

// GetWeight gets the weight for a signal type
func (e *ScoringEngine) GetWeight(signalType string) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.weights[signalType]
}

// SetThresholds sets the scoring thresholds
func (e *ScoringEngine) SetThresholds(block, log int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.blockThreshold = block
	e.logThreshold = log
	e.warnThreshold = log / 2
}

// EnableCorrelation enables/disables attack correlation
func (e *ScoringEngine) EnableCorrelation(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enableCorrelation = enabled
}

// EnableBaseline enables/disables baseline comparison
func (e *ScoringEngine) EnableBaseline(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enableBaseline = enabled
}

// Close stops the scoring engine
func (e *ScoringEngine) Close() {
	if e.baselineTracker != nil {
		e.baselineTracker.Close()
	}
	if e.correlator != nil {
		e.correlator.Close()
	}
}

// GetStats returns statistics about the scoring engine
func (e *ScoringEngine) GetStats() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := make(map[string]interface{})

	stats["block_threshold"] = e.blockThreshold
	stats["log_threshold"] = e.logThreshold
	stats["weights"] = e.weights

	if e.baselineTracker != nil {
		e.baselineTracker.mu.RLock()
		stats["tracked_endpoints"] = len(e.baselineTracker.endpoints)
		e.baselineTracker.mu.RUnlock()
	}

	if e.correlator != nil {
		e.correlator.mu.RLock()
		stats["active_attack_contexts"] = len(e.correlator.recentAttacks)
		e.correlator.mu.RUnlock()
	}

	return stats
}

// Helper functions

func extractClientIP(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return req.RemoteAddr
}

func formatFloat(f float64) string {
	return formatFloatSimple(f)
}

func formatFloatSimple(f float64) string {
	// Manual float formatting
	intPart := int(f)
	fracPart := int((f - float64(intPart)) * 100)
	if fracPart == 0 {
		return intToStr(intPart)
	}
	return intToStr(intPart) + "." + intToStr(fracPart)
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + intToStr(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x == 0 {
		return 0
	}
	// Newton's method
	z := x / 2
	for i := 0; i < 20; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ScoreFromDetectionResult converts a DetectionResult to SignalData
func ScoreFromDetectionResult(result interface{ GetScore() int; GetMatches() []string }) *SignalData {
	return &SignalData{
		Score:      result.GetScore(),
		Confidence: 0.8, // Default confidence
		Matches:    result.GetMatches(),
	}
}

// AggregateScores aggregates multiple signal data into a single score
func AggregateScores(signals map[string]*SignalData, weights map[string]float64) int {
	totalScore := 0.0
	totalWeight := 0.0

	for signalType, data := range signals {
		weight := weights[signalType]
		if weight == 0 {
			weight = 1.0
		}
		totalScore += float64(data.Score) * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 0
	}

	return int(totalScore)
}

// NormalizeScore normalizes a raw score to 0-100 range
func NormalizeScore(score int, maxExpected int) float64 {
	if maxExpected <= 0 {
		return 0
	}
	normalized := float64(score) / float64(maxExpected) * 100
	if normalized > 100 {
		return 100
	}
	return normalized
}

// CombineConfidences combines multiple confidence values
func CombineConfidences(confidences []float64) float64 {
	if len(confidences) == 0 {
		return 0
	}

	// Use geometric mean for combining confidences
	product := 1.0
	for _, c := range confidences {
		if c > 0 {
			product *= c
		}
	}

	// nth root
	n := float64(len(confidences))
	return nthRoot(product, n)
}

func nthRoot(x, n float64) float64 {
	if x <= 0 || n <= 0 {
		return 0
	}
	// Use Newton's method
	guess := x / n
	for i := 0; i < 20; i++ {
		guess = ((n-1)*guess + x/pow(guess, n-1)) / n
	}
	return guess
}

func pow(base, exp float64) float64 {
	if exp == 0 {
		return 1
	}
	result := base
	for i := 1; i < int(exp); i++ {
		result *= base
	}
	return result
}
