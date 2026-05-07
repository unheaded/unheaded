// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package sophia provides wisdom, knowledge management, and intelligent decision support.
// In Gnostic philosophy, Sophia is divine wisdom - the feminine aspect of the godhead,
// the bridge between the Pleroma (fullness/spiritual realm) and the Kenoma (material world).
// This service embodies that role: providing intelligent insights, managing knowledge graphs,
// and offering decision support through pattern recognition and inference.
package sophia

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// Knowledge represents a unit of wisdom in Sophia's domain.
type Knowledge struct {
	ID         string                 `json:"id"`
	Type       KnowledgeType          `json:"type"`
	Subject    string                 `json:"subject"`
	Predicate  string                 `json:"predicate"`
	Object     string                 `json:"object"`
	Confidence float64                `json:"confidence"`
	Source     string                 `json:"source"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Embeddings []float64              `json:"embeddings,omitempty"`
	Relations  []string               `json:"relations,omitempty"`
}

// KnowledgeType categorizes knowledge.
type KnowledgeType string

const (
	KnowledgeFact       KnowledgeType = "fact"       // Verified truth
	KnowledgeInference  KnowledgeType = "inference"  // Derived knowledge
	KnowledgeHeuristic  KnowledgeType = "heuristic"  // Rule of thumb
	KnowledgePattern    KnowledgeType = "pattern"    // Observed pattern
	KnowledgeConstraint KnowledgeType = "constraint" // System constraint
	KnowledgeGoal       KnowledgeType = "goal"       // Desired outcome
)

// Insight represents a derived understanding from knowledge analysis.
type Insight struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Confidence  float64    `json:"confidence"`
	Evidence    []string   `json:"evidence"`
	Impact      string     `json:"impact"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// Decision represents a recommendation from Sophia.
type Decision struct {
	ID             string                 `json:"id"`
	Question       string                 `json:"question"`
	Options        []Option               `json:"options"`
	Recommendation string                 `json:"recommendation"`
	Reasoning      string                 `json:"reasoning"`
	Confidence     float64                `json:"confidence"`
	Factors        []Factor               `json:"factors"`
	CreatedAt      time.Time              `json:"created_at"`
	Context        map[string]interface{} `json:"context,omitempty"`
}

// Option represents a choice in a decision.
type Option struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Score       float64  `json:"score"`
	Pros        []string `json:"pros,omitempty"`
	Cons        []string `json:"cons,omitempty"`
}

// Factor represents a consideration in decision-making.
type Factor struct {
	Name       string  `json:"name"`
	Weight     float64 `json:"weight"`
	Value      float64 `json:"value"`
	Importance string  `json:"importance"`
}

// Query represents a knowledge query.
type Query struct {
	Type          QueryType              `json:"type"`
	Subject       string                 `json:"subject,omitempty"`
	Predicate     string                 `json:"predicate,omitempty"`
	Object        string                 `json:"object,omitempty"`
	Text          string                 `json:"text,omitempty"`
	Filters       map[string]interface{} `json:"filters,omitempty"`
	Limit         int                    `json:"limit,omitempty"`
	MinConfidence float64                `json:"min_confidence,omitempty"`
}

// QueryType defines the kind of knowledge query.
type QueryType string

const (
	QueryExact    QueryType = "exact"    // Exact match
	QuerySemantic QueryType = "semantic" // Semantic similarity
	QueryGraph    QueryType = "graph"    // Graph traversal
	QueryInfer    QueryType = "infer"    // Inference query
)

// Rule represents an inference rule.
type Rule struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Conditions []string `json:"conditions"`
	Conclusion string   `json:"conclusion"`
	Confidence float64  `json:"confidence"`
	Priority   int      `json:"priority"`
	Enabled    bool     `json:"enabled"`
}

// Service is the main Sophia service for wisdom and knowledge management.
type Service struct {
	log    *logger.Logger
	wotan  *wotanClient.Client
	config *Config

	mu        sync.RWMutex
	knowledge map[string]*Knowledge
	insights  map[string]*Insight
	decisions map[string]*Decision
	rules     map[string]*Rule

	// Indexes for efficient lookup
	subjectIndex   map[string][]string // subject -> knowledge IDs
	predicateIndex map[string][]string // predicate -> knowledge IDs
	typeIndex      map[KnowledgeType][]string

	// Counters
	knowledgeCounter int64
	insightCounter   int64
	decisionCounter  int64

	// Alert handling
	alertsCh chan *wotanClient.Message

	// Degraded mode counter — messages dropped because Wotan is nil
	wotanDrops int64
}

// WotanDrops returns the total number of messages dropped because Wotan was nil.
func (s *Service) WotanDrops() int64 {
	return atomic.LoadInt64(&s.wotanDrops)
}

// Config holds Sophia service configuration.
type Config struct {
	MaxKnowledge      int           `json:"max_knowledge"`
	InsightTTL        time.Duration `json:"insight_ttl"`
	MinConfidence     float64       `json:"min_confidence"`
	EnableInference   bool          `json:"enable_inference"`
	InferenceInterval time.Duration `json:"inference_interval"`
	WotanTopic        string        `json:"wotan_topic"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxKnowledge:      100000,
		InsightTTL:        24 * time.Hour,
		MinConfidence:     0.5,
		EnableInference:   true,
		InferenceInterval: 5 * time.Minute,
		WotanTopic:        "sophia.wisdom",
	}
}

// NewService creates a new Sophia service.
func NewService(log *logger.Logger, wotan *wotanClient.Client, config *Config) *Service {
	if config == nil {
		config = DefaultConfig()
	}

	return &Service{
		log:            log,
		wotan:          wotan,
		config:         config,
		knowledge:      make(map[string]*Knowledge),
		insights:       make(map[string]*Insight),
		decisions:      make(map[string]*Decision),
		rules:          make(map[string]*Rule),
		subjectIndex:   make(map[string][]string),
		predicateIndex: make(map[string][]string),
		typeIndex:      make(map[KnowledgeType][]string),
		alertsCh:       make(chan *wotanClient.Message, 100),
	}
}

// Start initializes the service.
func (s *Service) Start(ctx context.Context) error {
	s.log.Info().Msg("Sophia awakens - wisdom flows through the Kingdom")

	// Register default inference rules
	s.registerDefaultRules()

	// Start inference engine if enabled
	if s.config.EnableInference {
		go s.inferenceLoop(ctx)
	}

	// Subscribe to Wotan topics
	if s.wotan != nil {
		// Subscribe to alerts.critical (required by CLAUDE.md)
		if _, err := s.wotan.Subscribe(ctx, "alerts.critical", "sophia-service"); err != nil {
			s.log.Warn().Err(err).Msg("failed to subscribe to alerts.critical")
		} else {
			s.log.Info().Msg("subscribed to alerts.critical")
		}

		// Subscribe to sophia.wisdom for own topic
		if _, err := s.wotan.Subscribe(ctx, s.config.WotanTopic, "sophia-service"); err != nil {
			s.log.Warn().Err(err).Msg("failed to subscribe to sophia.wisdom")
		} else {
			s.log.Info().Str("topic", s.config.WotanTopic).Msg("subscribed to topic")
		}

		// Start alert listener
		go s.listenForAlerts(ctx)

		// Subscribe to knowledge events
		go s.subscribeToEvents(ctx)
	}

	return nil
}

// Stop gracefully shuts down the service.
func (s *Service) Stop() error {
	s.log.Info().Msg("Sophia rests - wisdom preserved")
	if s.alertsCh != nil {
		close(s.alertsCh)
	}
	return nil
}

// listenForAlerts listens for critical alerts from Wotan
func (s *Service) listenForAlerts(ctx context.Context) {
	if s.wotan == nil {
		s.log.Warn().Msg("wotan unavailable — alert listener disabled (degraded mode)")
		return
	}

	msgCh, err := s.wotan.StreamMessages(ctx, "alerts.critical")
	if err != nil {
		s.log.Warn().Err(err).Msg("failed to stream alerts.critical")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			s.handleCriticalAlert(ctx, msg)
		}
	}
}

// handleCriticalAlert processes critical alerts
func (s *Service) handleCriticalAlert(ctx context.Context, msg *wotanClient.Message) {
	if msg == nil {
		return
	}

	s.log.Warn().
		Str("message_id", msg.MessageID).
		Str("topic", msg.Topic).
		Msg("received critical alert - analyzing for insights")

	// Sophia can analyze alerts and generate insights
	_, _ = s.GenerateInsight(ctx, "alert_analysis", []string{msg.Payload})
}

// Learn adds new knowledge to Sophia's domain.
func (s *Service) Learn(ctx context.Context, k *Knowledge) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate ID if not provided
	if k.ID == "" {
		s.knowledgeCounter++
		k.ID = fmt.Sprintf("k-%d-%d", time.Now().UnixNano(), s.knowledgeCounter)
	}

	// Set timestamps
	now := time.Now()
	if k.CreatedAt.IsZero() {
		k.CreatedAt = now
	}
	k.UpdatedAt = now

	// Validate confidence
	if k.Confidence <= 0 {
		k.Confidence = 0.5
	}
	if k.Confidence > 1 {
		k.Confidence = 1
	}

	// Store knowledge
	s.knowledge[k.ID] = k

	// Update indexes
	s.subjectIndex[k.Subject] = append(s.subjectIndex[k.Subject], k.ID)
	s.predicateIndex[k.Predicate] = append(s.predicateIndex[k.Predicate], k.ID)
	s.typeIndex[k.Type] = append(s.typeIndex[k.Type], k.ID)

	s.log.Debug().
		Str("id", k.ID).
		Str("subject", k.Subject).
		Str("predicate", k.Predicate).
		Float64("confidence", k.Confidence).
		Msg("Knowledge learned")

	// Publish event
	s.publishEvent(ctx, "sophia.knowledge.learned", map[string]interface{}{
		"knowledge_id": k.ID,
		"subject":      k.Subject,
		"predicate":    k.Predicate,
		"type":         k.Type,
	})

	return nil
}

// Query searches Sophia's knowledge base.
func (s *Service) Query(ctx context.Context, q *Query) ([]*Knowledge, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*Knowledge

	switch q.Type {
	case QueryExact:
		results = s.queryExact(q)
	case QuerySemantic:
		results = s.querySemantic(q)
	case QueryGraph:
		results = s.queryGraph(q)
	case QueryInfer:
		results = s.queryInfer(q)
	default:
		results = s.queryExact(q)
	}

	// Apply confidence filter
	if q.MinConfidence > 0 {
		filtered := make([]*Knowledge, 0)
		for _, k := range results {
			if k.Confidence >= q.MinConfidence {
				filtered = append(filtered, k)
			}
		}
		results = filtered
	}

	// Apply limit
	if q.Limit > 0 && len(results) > q.Limit {
		results = results[:q.Limit]
	}

	return results, nil
}

// queryExact performs exact matching.
func (s *Service) queryExact(q *Query) []*Knowledge {
	var candidateIDs []string

	if q.Subject != "" {
		candidateIDs = s.subjectIndex[q.Subject]
	} else if q.Predicate != "" {
		candidateIDs = s.predicateIndex[q.Predicate]
	} else {
		// Return all
		for id := range s.knowledge {
			candidateIDs = append(candidateIDs, id)
		}
	}

	results := make([]*Knowledge, 0)
	for _, id := range candidateIDs {
		k := s.knowledge[id]
		if k == nil {
			continue
		}

		// Match criteria
		if q.Subject != "" && k.Subject != q.Subject {
			continue
		}
		if q.Predicate != "" && k.Predicate != q.Predicate {
			continue
		}
		if q.Object != "" && k.Object != q.Object {
			continue
		}

		results = append(results, k)
	}

	return results
}

// querySemantic performs semantic similarity search.
func (s *Service) querySemantic(q *Query) []*Knowledge {
	// Simple text similarity for now
	// In production, this would use embeddings and vector similarity
	searchText := strings.ToLower(q.Text)
	if searchText == "" {
		searchText = strings.ToLower(q.Subject + " " + q.Predicate + " " + q.Object)
	}

	type scored struct {
		k     *Knowledge
		score float64
	}

	var scoredResults []scored
	for _, k := range s.knowledge {
		text := strings.ToLower(k.Subject + " " + k.Predicate + " " + k.Object)
		score := s.textSimilarity(searchText, text)
		if score > 0.1 {
			scoredResults = append(scoredResults, scored{k: k, score: score})
		}
	}

	// Sort by score
	sort.Slice(scoredResults, func(i, j int) bool {
		return scoredResults[i].score > scoredResults[j].score
	})

	results := make([]*Knowledge, len(scoredResults))
	for i, sr := range scoredResults {
		results[i] = sr.k
	}

	return results
}

// textSimilarity calculates simple Jaccard similarity between texts.
func (s *Service) textSimilarity(a, b string) float64 {
	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)

	setA := make(map[string]bool)
	for _, w := range wordsA {
		setA[w] = true
	}

	setB := make(map[string]bool)
	for _, w := range wordsB {
		setB[w] = true
	}

	var intersection, union int
	for w := range setA {
		if setB[w] {
			intersection++
		}
		union++
	}
	for w := range setB {
		if !setA[w] {
			union++
		}
	}

	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

// queryGraph performs graph traversal.
func (s *Service) queryGraph(q *Query) []*Knowledge {
	// BFS from subject
	if q.Subject == "" {
		return nil
	}

	visited := make(map[string]bool)
	results := make([]*Knowledge, 0)
	queue := []string{q.Subject}

	depth := 0
	maxDepth := 3

	for len(queue) > 0 && depth < maxDepth {
		levelSize := len(queue)
		for i := 0; i < levelSize; i++ {
			subject := queue[0]
			queue = queue[1:]

			if visited[subject] {
				continue
			}
			visited[subject] = true

			// Find all knowledge with this subject
			for _, id := range s.subjectIndex[subject] {
				k := s.knowledge[id]
				if k != nil {
					results = append(results, k)
					// Add object to queue for traversal
					if !visited[k.Object] {
						queue = append(queue, k.Object)
					}
				}
			}
		}
		depth++
	}

	return results
}

// queryInfer performs inference-based query.
func (s *Service) queryInfer(q *Query) []*Knowledge {
	// First get direct results
	directResults := s.queryExact(q)

	// Then apply inference rules
	inferred := s.applyRules(q)

	// Combine results
	results := append(directResults, inferred...)
	return results
}

// applyRules applies inference rules to derive new knowledge.
func (s *Service) applyRules(q *Query) []*Knowledge {
	var inferred []*Knowledge

	for _, rule := range s.rules {
		if !rule.Enabled {
			continue
		}

		// Check if rule conditions are met
		conditionsMet := true
		for _, condition := range rule.Conditions {
			// Simple condition checking
			found := false
			for _, k := range s.knowledge {
				if strings.Contains(k.Subject+" "+k.Predicate+" "+k.Object, condition) {
					found = true
					break
				}
			}
			if !found {
				conditionsMet = false
				break
			}
		}

		if conditionsMet {
			// Create inferred knowledge
			k := &Knowledge{
				ID:         fmt.Sprintf("inferred-%s-%d", rule.ID, time.Now().UnixNano()),
				Type:       KnowledgeInference,
				Subject:    q.Subject,
				Predicate:  "inferred_from_" + rule.Name,
				Object:     rule.Conclusion,
				Confidence: rule.Confidence * 0.8, // Reduce confidence for inferred
				Source:     "inference:" + rule.ID,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			inferred = append(inferred, k)
		}
	}

	return inferred
}

// Decide provides a decision recommendation.
func (s *Service) Decide(ctx context.Context, question string, options []Option, factors []Factor) (*Decision, error) {
	s.mu.Lock()
	s.decisionCounter++
	decisionID := fmt.Sprintf("d-%d-%d", time.Now().UnixNano(), s.decisionCounter)
	s.mu.Unlock()

	// Score each option based on factors
	for i := range options {
		score := s.scoreOption(&options[i], factors)
		options[i].Score = score
	}

	// Sort by score
	sort.Slice(options, func(i, j int) bool {
		return options[i].Score > options[j].Score
	})

	// Build recommendation
	var recommendation string
	var confidence float64
	var reasoning string

	if len(options) > 0 {
		recommendation = options[0].ID
		confidence = options[0].Score

		// Generate reasoning
		reasoning = fmt.Sprintf("Option '%s' scored highest (%.2f) based on %d factors. ",
			options[0].Name, options[0].Score, len(factors))

		if len(options) > 1 {
			gap := options[0].Score - options[1].Score
			if gap < 0.1 {
				reasoning += "However, this is a close decision with the runner-up. "
				confidence *= 0.8
			} else {
				reasoning += "This option clearly outperforms alternatives. "
			}
		}
	}

	decision := &Decision{
		ID:             decisionID,
		Question:       question,
		Options:        options,
		Recommendation: recommendation,
		Reasoning:      reasoning,
		Confidence:     confidence,
		Factors:        factors,
		CreatedAt:      time.Now(),
	}

	s.mu.Lock()
	s.decisions[decisionID] = decision
	s.mu.Unlock()

	s.log.Info().
		Str("decision_id", decisionID).
		Str("question", question).
		Str("recommendation", recommendation).
		Float64("confidence", confidence).
		Msg("Decision made")

	// Publish event
	s.publishEvent(ctx, "sophia.decision.made", map[string]interface{}{
		"decision_id":    decisionID,
		"question":       question,
		"recommendation": recommendation,
		"confidence":     confidence,
	})

	return decision, nil
}

// scoreOption calculates a score for an option based on factors.
func (s *Service) scoreOption(option *Option, factors []Factor) float64 {
	if len(factors) == 0 {
		return 0.5
	}

	var totalWeight float64
	var weightedSum float64

	for _, factor := range factors {
		weight := math.Abs(factor.Weight)
		totalWeight += weight
		weightedSum += factor.Value * weight
	}

	if totalWeight == 0 {
		return 0.5
	}

	return weightedSum / totalWeight
}

// GenerateInsight analyzes knowledge and generates an insight.
func (s *Service) GenerateInsight(ctx context.Context, insightType string, evidence []string) (*Insight, error) {
	s.mu.Lock()
	s.insightCounter++
	insightID := fmt.Sprintf("i-%d-%d", time.Now().UnixNano(), s.insightCounter)
	s.mu.Unlock()

	// Analyze evidence
	var confidence float64
	var description string

	if len(evidence) > 0 {
		// Calculate confidence based on evidence strength
		confidence = math.Min(0.9, 0.3+float64(len(evidence))*0.1)

		// Generate description
		description = fmt.Sprintf("Based on %d pieces of evidence, ", len(evidence))
	} else {
		confidence = 0.3
		description = "Preliminary observation: "
	}

	insight := &Insight{
		ID:          insightID,
		Type:        insightType,
		Title:       fmt.Sprintf("Insight: %s", insightType),
		Description: description,
		Confidence:  confidence,
		Evidence:    evidence,
		Impact:      s.assessImpact(confidence),
		CreatedAt:   time.Now(),
	}

	// Set expiration
	expiry := time.Now().Add(s.config.InsightTTL)
	insight.ExpiresAt = &expiry

	s.mu.Lock()
	s.insights[insightID] = insight
	s.mu.Unlock()

	s.log.Info().
		Str("insight_id", insightID).
		Str("type", insightType).
		Float64("confidence", confidence).
		Msg("Insight generated")

	return insight, nil
}

// assessImpact determines the impact level based on confidence.
func (s *Service) assessImpact(confidence float64) string {
	if confidence >= 0.8 {
		return "high"
	} else if confidence >= 0.5 {
		return "medium"
	}
	return "low"
}

// registerDefaultRules adds built-in inference rules.
func (s *Service) registerDefaultRules() {
	rules := []*Rule{
		{
			ID:         "transitivity",
			Name:       "transitivity",
			Conditions: []string{},
			Conclusion: "transitive_relation_derived",
			Confidence: 0.8,
			Priority:   1,
			Enabled:    true,
		},
		{
			ID:         "type_inheritance",
			Name:       "type_inheritance",
			Conditions: []string{},
			Conclusion: "type_inherited",
			Confidence: 0.9,
			Priority:   2,
			Enabled:    true,
		},
	}

	for _, rule := range rules {
		s.rules[rule.ID] = rule
	}
}

// inferenceLoop runs periodic inference.
func (s *Service) inferenceLoop(ctx context.Context) {
	ticker := time.NewTicker(s.config.InferenceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runInference(ctx)
		}
	}
}

// runInference applies rules to derive new knowledge.
func (s *Service) runInference(ctx context.Context) {
	s.mu.RLock()
	knowledgeCount := len(s.knowledge)
	s.mu.RUnlock()

	s.log.Debug().Int("knowledge_count", knowledgeCount).Msg("Running inference cycle")

	// Apply each rule
	for _, rule := range s.rules {
		if !rule.Enabled {
			continue
		}

		// Check conditions and potentially derive new knowledge
		// This is a simplified implementation
		_ = rule // Inference logic would go here
	}
}

// subscribeToEvents listens for knowledge events.
func (s *Service) subscribeToEvents(ctx context.Context) {
	if s.wotan == nil {
		s.log.Warn().Msg("wotan unavailable — event subscription disabled (degraded mode)")
		return
	}

	// Subscribe to knowledge sharing events
	_, err := s.wotan.Subscribe(ctx, "knowledge.shared", "sophia-service")
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to subscribe to knowledge events")
		return
	}

	s.log.Info().Msg("Subscribed to knowledge events")
}

// publishEvent sends an event to Wotan.
func (s *Service) publishEvent(ctx context.Context, eventType string, data map[string]interface{}) {
	if s.wotan == nil {
		atomic.AddInt64(&s.wotanDrops, 1)
		s.log.Warn().Str("event_type", eventType).Msg("wotan unavailable — event dropped (degraded mode)")
		return
	}

	// Extract trace_id from context or generate one
	traceID := ""
	if tid := ctx.Value("trace_id"); tid != nil {
		traceID = tid.(string)
	}
	if traceID == "" {
		traceID = fmt.Sprintf("sophia-%d", time.Now().UnixNano())
	}

	event := map[string]interface{}{
		"event_type": eventType,
		"timestamp":  time.Now().UnixMilli(),
		"data":       data,
		"trace_id":   traceID,
		"service":    "sophia",
	}

	payload, err := json.Marshal(event)
	if err != nil {
		s.log.Warn().Err(err).Msg("Failed to marshal event")
		return
	}

	// Use a timeout for publishing
	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.wotan.Publish(pubCtx, s.config.WotanTopic, payload); err != nil {
		s.log.Warn().Err(err).Msg("Failed to publish event")
	}
}

// GetKnowledge retrieves knowledge by ID.
func (s *Service) GetKnowledge(id string) (*Knowledge, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.knowledge[id]
	return k, ok
}

// GetInsight retrieves an insight by ID.
func (s *Service) GetInsight(id string) (*Insight, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.insights[id]
	return i, ok
}

// GetDecision retrieves a decision by ID.
func (s *Service) GetDecision(id string) (*Decision, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.decisions[id]
	return d, ok
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Count by type
	typeCounts := make(map[string]int)
	for _, k := range s.knowledge {
		typeCounts[string(k.Type)]++
	}

	return map[string]interface{}{
		"total_knowledge":    len(s.knowledge),
		"total_insights":     len(s.insights),
		"total_decisions":    len(s.decisions),
		"total_rules":        len(s.rules),
		"knowledge_by_type":  typeCounts,
		"indexed_subjects":   len(s.subjectIndex),
		"indexed_predicates": len(s.predicateIndex),
	}
}
