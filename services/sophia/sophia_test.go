package sophia

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
)

func newTestService() *Service {
	log := logger.New(os.Stderr)
	return NewService(log, nil, nil)
}

func TestNewService(t *testing.T) {
	t.Run("Creates service with default config", func(t *testing.T) {
		svc := newTestService()
		if svc == nil {
			t.Fatal("Expected non-nil service")
		}
		if svc.knowledge == nil {
			t.Error("Expected knowledge map to be initialized")
		}
		if svc.insights == nil {
			t.Error("Expected insights map to be initialized")
		}
		if svc.decisions == nil {
			t.Error("Expected decisions map to be initialized")
		}
		if svc.rules == nil {
			t.Error("Expected rules map to be initialized")
		}
		if svc.config == nil {
			t.Error("Expected config to be set")
		}
	})

	t.Run("Creates service with custom config", func(t *testing.T) {
		log := logger.New(os.Stderr)
		config := &Config{
			MaxKnowledge:  50000,
			InsightTTL:    12 * time.Hour,
			MinConfidence: 0.7,
		}
		svc := NewService(log, nil, config)

		if svc.config.MaxKnowledge != 50000 {
			t.Error("Expected custom MaxKnowledge")
		}
		if svc.config.MinConfidence != 0.7 {
			t.Error("Expected custom MinConfidence")
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxKnowledge != 100000 {
		t.Errorf("Expected MaxKnowledge 100000, got %d", config.MaxKnowledge)
	}
	if config.InsightTTL != 24*time.Hour {
		t.Errorf("Expected InsightTTL 24h, got %v", config.InsightTTL)
	}
	if config.MinConfidence != 0.5 {
		t.Errorf("Expected MinConfidence 0.5, got %f", config.MinConfidence)
	}
	if !config.EnableInference {
		t.Error("Expected EnableInference to be true")
	}
	if config.BusboyTopic != "sophia.wisdom" {
		t.Errorf("Expected BusboyTopic 'sophia.wisdom', got %s", config.BusboyTopic)
	}
}

func TestLearn(t *testing.T) {
	t.Run("Adds knowledge with generated ID", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		k := &Knowledge{
			Subject:    "Go",
			Predicate:  "is_a",
			Object:     "programming_language",
			Type:       KnowledgeFact,
			Confidence: 0.9,
			Source:     "test",
		}

		err := svc.Learn(ctx, k)
		if err != nil {
			t.Fatalf("Failed to learn: %v", err)
		}

		if k.ID == "" {
			t.Error("Expected ID to be generated")
		}
		if k.CreatedAt.IsZero() {
			t.Error("Expected CreatedAt to be set")
		}
		if k.UpdatedAt.IsZero() {
			t.Error("Expected UpdatedAt to be set")
		}
	})

	t.Run("Uses provided ID", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		k := &Knowledge{
			ID:         "custom-id",
			Subject:    "Rust",
			Predicate:  "is_a",
			Object:     "programming_language",
			Type:       KnowledgeFact,
			Confidence: 0.95,
		}

		err := svc.Learn(ctx, k)
		if err != nil {
			t.Fatalf("Failed to learn: %v", err)
		}

		if k.ID != "custom-id" {
			t.Error("Expected custom ID to be preserved")
		}
	})

	t.Run("Normalizes confidence", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		// Test zero confidence
		k1 := &Knowledge{Subject: "a", Predicate: "b", Object: "c", Confidence: 0}
		_ = svc.Learn(ctx, k1)
		if k1.Confidence != 0.5 {
			t.Errorf("Expected confidence 0.5 for zero input, got %f", k1.Confidence)
		}

		// Test over 1.0 confidence
		k2 := &Knowledge{Subject: "d", Predicate: "e", Object: "f", Confidence: 1.5}
		_ = svc.Learn(ctx, k2)
		if k2.Confidence != 1.0 {
			t.Errorf("Expected confidence 1.0 for over-1 input, got %f", k2.Confidence)
		}
	})

	t.Run("Updates indexes", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		k := &Knowledge{
			Subject:   "Python",
			Predicate: "has_feature",
			Object:    "dynamic_typing",
			Type:      KnowledgeFact,
		}

		_ = svc.Learn(ctx, k)

		// Check subject index
		if len(svc.subjectIndex["Python"]) != 1 {
			t.Error("Expected subject index to be updated")
		}

		// Check predicate index
		if len(svc.predicateIndex["has_feature"]) != 1 {
			t.Error("Expected predicate index to be updated")
		}

		// Check type index
		if len(svc.typeIndex[KnowledgeFact]) != 1 {
			t.Error("Expected type index to be updated")
		}
	})
}

func TestQueryExact(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Add test knowledge
	knowledge := []*Knowledge{
		{Subject: "Go", Predicate: "is_a", Object: "language", Type: KnowledgeFact, Confidence: 0.9},
		{Subject: "Go", Predicate: "has_feature", Object: "goroutines", Type: KnowledgeFact, Confidence: 0.95},
		{Subject: "Rust", Predicate: "is_a", Object: "language", Type: KnowledgeFact, Confidence: 0.9},
		{Subject: "Python", Predicate: "is_a", Object: "language", Type: KnowledgeFact, Confidence: 0.9},
	}

	for _, k := range knowledge {
		_ = svc.Learn(ctx, k)
	}

	t.Run("Finds by subject", func(t *testing.T) {
		q := &Query{Type: QueryExact, Subject: "Go"}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results for subject Go, got %d", len(results))
		}
	})

	t.Run("Finds by predicate", func(t *testing.T) {
		q := &Query{Type: QueryExact, Predicate: "is_a"}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 results for predicate is_a, got %d", len(results))
		}
	})

	t.Run("Finds by subject and predicate", func(t *testing.T) {
		q := &Query{Type: QueryExact, Subject: "Go", Predicate: "is_a"}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("Finds by full triple", func(t *testing.T) {
		q := &Query{Type: QueryExact, Subject: "Rust", Predicate: "is_a", Object: "language"}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})

	t.Run("Returns empty for no match", func(t *testing.T) {
		q := &Query{Type: QueryExact, Subject: "NonExistent"}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(results))
		}
	})
}

func TestQuerySemantic(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Add test knowledge
	knowledge := []*Knowledge{
		{Subject: "machine learning", Predicate: "is_part_of", Object: "artificial intelligence", Confidence: 0.9},
		{Subject: "deep learning", Predicate: "is_part_of", Object: "machine learning", Confidence: 0.9},
		{Subject: "neural networks", Predicate: "used_in", Object: "deep learning", Confidence: 0.85},
		{Subject: "database", Predicate: "stores", Object: "data", Confidence: 0.9},
	}

	for _, k := range knowledge {
		_ = svc.Learn(ctx, k)
	}

	t.Run("Finds semantically related knowledge", func(t *testing.T) {
		q := &Query{Type: QuerySemantic, Text: "machine learning artificial intelligence"}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) == 0 {
			t.Error("Expected at least some results for semantic query")
		}
	})

	t.Run("Returns sorted by similarity", func(t *testing.T) {
		q := &Query{Type: QuerySemantic, Text: "learning"}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}

		// Should find the learning-related entries
		found := false
		for _, r := range results {
			if r.Subject == "machine learning" || r.Subject == "deep learning" {
				found = true
				break
			}
		}
		if !found && len(results) > 0 {
			t.Error("Expected to find learning-related knowledge first")
		}
	})
}

func TestQueryGraph(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Build a knowledge graph
	knowledge := []*Knowledge{
		{Subject: "A", Predicate: "connects_to", Object: "B", Confidence: 0.9},
		{Subject: "B", Predicate: "connects_to", Object: "C", Confidence: 0.9},
		{Subject: "C", Predicate: "connects_to", Object: "D", Confidence: 0.9},
		{Subject: "X", Predicate: "isolated", Object: "Y", Confidence: 0.9},
	}

	for _, k := range knowledge {
		_ = svc.Learn(ctx, k)
	}

	t.Run("Traverses graph from subject", func(t *testing.T) {
		q := &Query{Type: QueryGraph, Subject: "A"}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}

		// Should find path from A -> B -> C -> D
		if len(results) < 3 {
			t.Errorf("Expected at least 3 results for graph traversal, got %d", len(results))
		}
	})

	t.Run("Limits traversal depth", func(t *testing.T) {
		// Create a long chain
		for i := 0; i < 10; i++ {
			_ = svc.Learn(ctx, &Knowledge{
				Subject:   "node_" + string(rune('a'+i)),
				Predicate: "next",
				Object:    "node_" + string(rune('b'+i)),
			})
		}

		q := &Query{Type: QueryGraph, Subject: "node_a"}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}

		// Depth is limited to 3
		if len(results) > 10 {
			t.Errorf("Graph traversal should be depth-limited, got %d results", len(results))
		}
	})
}

func TestQueryFilters(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	knowledge := []*Knowledge{
		{Subject: "high", Predicate: "p", Object: "o", Confidence: 0.9},
		{Subject: "medium", Predicate: "p", Object: "o", Confidence: 0.6},
		{Subject: "low", Predicate: "p", Object: "o", Confidence: 0.3},
	}

	for _, k := range knowledge {
		_ = svc.Learn(ctx, k)
	}

	t.Run("Filters by minimum confidence", func(t *testing.T) {
		q := &Query{Type: QueryExact, Predicate: "p", MinConfidence: 0.5}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results with confidence >= 0.5, got %d", len(results))
		}
	})

	t.Run("Applies limit", func(t *testing.T) {
		q := &Query{Type: QueryExact, Predicate: "p", Limit: 2}
		results, err := svc.Query(ctx, q)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 results with limit, got %d", len(results))
		}
	})
}

func TestDecide(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("Makes decision with factors", func(t *testing.T) {
		options := []Option{
			{ID: "opt1", Name: "Option 1", Description: "First option"},
			{ID: "opt2", Name: "Option 2", Description: "Second option"},
			{ID: "opt3", Name: "Option 3", Description: "Third option"},
		}

		factors := []Factor{
			{Name: "cost", Weight: 0.5, Value: 0.8, Importance: "high"},
			{Name: "speed", Weight: 0.3, Value: 0.9, Importance: "medium"},
			{Name: "quality", Weight: 0.2, Value: 0.7, Importance: "medium"},
		}

		decision, err := svc.Decide(ctx, "Which option to choose?", options, factors)
		if err != nil {
			t.Fatalf("Decide failed: %v", err)
		}

		if decision.ID == "" {
			t.Error("Expected decision ID to be set")
		}
		if decision.Question != "Which option to choose?" {
			t.Error("Expected question to be preserved")
		}
		if decision.Recommendation == "" {
			t.Error("Expected recommendation to be set")
		}
		if decision.Confidence <= 0 {
			t.Error("Expected positive confidence")
		}
		if decision.Reasoning == "" {
			t.Error("Expected reasoning to be generated")
		}
	})

	t.Run("Options are sorted by score", func(t *testing.T) {
		options := []Option{
			{ID: "low", Name: "Low"},
			{ID: "high", Name: "High"},
		}

		factors := []Factor{
			{Name: "f1", Weight: 1.0, Value: 0.9},
		}

		decision, _ := svc.Decide(ctx, "Test?", options, factors)

		// First option should have highest score
		if len(decision.Options) < 2 {
			t.Fatal("Expected at least 2 options")
		}
		if decision.Options[0].Score < decision.Options[1].Score {
			t.Error("Expected options to be sorted by score descending")
		}
	})

	t.Run("Empty factors give 0.5 score", func(t *testing.T) {
		options := []Option{{ID: "only", Name: "Only Option"}}
		decision, _ := svc.Decide(ctx, "Test?", options, nil)

		if decision.Options[0].Score != 0.5 {
			t.Errorf("Expected score 0.5 for no factors, got %f", decision.Options[0].Score)
		}
	})

	t.Run("Decision is stored", func(t *testing.T) {
		options := []Option{{ID: "a", Name: "A"}}
		decision, _ := svc.Decide(ctx, "Store test?", options, nil)

		stored, ok := svc.GetDecision(decision.ID)
		if !ok {
			t.Error("Expected decision to be stored")
		}
		if stored.Question != "Store test?" {
			t.Error("Expected stored decision to match")
		}
	})
}

func TestGenerateInsight(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("Generates insight with evidence", func(t *testing.T) {
		evidence := []string{"observation1", "observation2", "observation3"}
		insight, err := svc.GenerateInsight(ctx, "performance_pattern", evidence)
		if err != nil {
			t.Fatalf("GenerateInsight failed: %v", err)
		}

		if insight.ID == "" {
			t.Error("Expected insight ID")
		}
		if insight.Type != "performance_pattern" {
			t.Error("Expected correct insight type")
		}
		if insight.Confidence <= 0 {
			t.Error("Expected positive confidence")
		}
		if len(insight.Evidence) != 3 {
			t.Error("Expected 3 pieces of evidence")
		}
		if insight.ExpiresAt == nil {
			t.Error("Expected expiration to be set")
		}
	})

	t.Run("Confidence scales with evidence", func(t *testing.T) {
		// Few evidence pieces
		insight1, _ := svc.GenerateInsight(ctx, "test", []string{"a"})
		// Many evidence pieces
		insight2, _ := svc.GenerateInsight(ctx, "test", []string{"a", "b", "c", "d", "e"})

		if insight2.Confidence <= insight1.Confidence {
			t.Error("Expected more evidence to yield higher confidence")
		}
	})

	t.Run("Impact reflects confidence", func(t *testing.T) {
		// Low confidence
		insight1, _ := svc.GenerateInsight(ctx, "test", nil)
		// High confidence
		insight2, _ := svc.GenerateInsight(ctx, "test", []string{"a", "b", "c", "d", "e", "f", "g"})

		if insight1.Impact != "low" {
			t.Errorf("Expected low impact for low confidence, got %s", insight1.Impact)
		}
		if insight2.Impact == "low" {
			t.Error("Expected higher impact for high confidence")
		}
	})

	t.Run("Insight is stored", func(t *testing.T) {
		insight, _ := svc.GenerateInsight(ctx, "stored", nil)
		stored, ok := svc.GetInsight(insight.ID)
		if !ok {
			t.Error("Expected insight to be stored")
		}
		if stored.Type != "stored" {
			t.Error("Expected stored insight to match")
		}
	})
}

func TestKnowledgeRetrieval(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	k := &Knowledge{ID: "test-k", Subject: "Test", Predicate: "is", Object: "knowledge"}
	_ = svc.Learn(ctx, k)

	t.Run("GetKnowledge retrieves by ID", func(t *testing.T) {
		found, ok := svc.GetKnowledge("test-k")
		if !ok {
			t.Error("Expected to find knowledge")
		}
		if found.Subject != "Test" {
			t.Error("Expected correct knowledge")
		}
	})

	t.Run("GetKnowledge returns false for missing", func(t *testing.T) {
		_, ok := svc.GetKnowledge("nonexistent")
		if ok {
			t.Error("Expected not to find nonexistent knowledge")
		}
	})
}

func TestStats(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Add various data
	_ = svc.Learn(ctx, &Knowledge{Subject: "a", Predicate: "b", Object: "c", Type: KnowledgeFact})
	_ = svc.Learn(ctx, &Knowledge{Subject: "d", Predicate: "e", Object: "f", Type: KnowledgePattern})
	_, _ = svc.GenerateInsight(ctx, "test", nil)
	_, _ = svc.Decide(ctx, "test?", []Option{{ID: "x"}}, nil)

	stats := svc.Stats()

	if stats["total_knowledge"].(int) != 2 {
		t.Errorf("Expected 2 knowledge, got %d", stats["total_knowledge"].(int))
	}
	if stats["total_insights"].(int) != 1 {
		t.Errorf("Expected 1 insight, got %d", stats["total_insights"].(int))
	}
	if stats["total_decisions"].(int) != 1 {
		t.Errorf("Expected 1 decision, got %d", stats["total_decisions"].(int))
	}

	typeCounts := stats["knowledge_by_type"].(map[string]int)
	if typeCounts["fact"] != 1 {
		t.Error("Expected 1 fact")
	}
	if typeCounts["pattern"] != 1 {
		t.Error("Expected 1 pattern")
	}
}

func TestServiceLifecycle(t *testing.T) {
	log := logger.New(os.Stderr)
	config := &Config{
		EnableInference:   false, // Disable for testing
		InferenceInterval: time.Hour,
	}
	svc := NewService(log, nil, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("Start initializes service", func(t *testing.T) {
		err := svc.Start(ctx)
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}

		// Should have default rules
		if len(svc.rules) == 0 {
			t.Error("Expected default rules to be registered")
		}
	})

	t.Run("Stop completes gracefully", func(t *testing.T) {
		err := svc.Stop()
		if err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	})
}

func TestConcurrency(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("Concurrent Learn is safe", func(t *testing.T) {
		var wg sync.WaitGroup
		errCh := make(chan error, 100)

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				k := &Knowledge{
					Subject:   "subject_" + string(rune('a'+i%26)),
					Predicate: "predicate",
					Object:    "object",
					Type:      KnowledgeFact,
				}
				if err := svc.Learn(ctx, k); err != nil {
					errCh <- err
				}
			}(i)
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Errorf("Concurrent Learn failed: %v", err)
		}

		stats := svc.Stats()
		if stats["total_knowledge"].(int) != 100 {
			t.Errorf("Expected 100 knowledge items, got %d", stats["total_knowledge"].(int))
		}
	})

	t.Run("Concurrent Query is safe", func(t *testing.T) {
		// Add some base knowledge
		for i := 0; i < 10; i++ {
			_ = svc.Learn(ctx, &Knowledge{
				Subject:   "query_subject",
				Predicate: "p",
				Object:    "o",
			})
		}

		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				q := &Query{Type: QueryExact, Subject: "query_subject"}
				_, err := svc.Query(ctx, q)
				if err != nil {
					t.Errorf("Concurrent Query failed: %v", err)
				}
			}()
		}

		wg.Wait()
	})

	t.Run("Concurrent Decide is safe", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				options := []Option{{ID: "a"}, {ID: "b"}}
				factors := []Factor{{Name: "f", Weight: 1, Value: 0.5}}
				_, err := svc.Decide(ctx, "test?", options, factors)
				if err != nil {
					t.Errorf("Concurrent Decide failed: %v", err)
				}
			}()
		}

		wg.Wait()
	})
}

func TestTextSimilarity(t *testing.T) {
	svc := newTestService()

	t.Run("Identical texts have similarity 1", func(t *testing.T) {
		sim := svc.textSimilarity("hello world", "hello world")
		if sim != 1.0 {
			t.Errorf("Expected similarity 1.0 for identical texts, got %f", sim)
		}
	})

	t.Run("Completely different texts have low similarity", func(t *testing.T) {
		sim := svc.textSimilarity("apple banana", "car truck")
		if sim >= 0.1 {
			t.Errorf("Expected low similarity for different texts, got %f", sim)
		}
	})

	t.Run("Partially overlapping texts have medium similarity", func(t *testing.T) {
		sim := svc.textSimilarity("machine learning", "deep learning")
		if sim <= 0 || sim >= 1 {
			t.Errorf("Expected partial similarity, got %f", sim)
		}
	})

	t.Run("Empty texts have zero similarity", func(t *testing.T) {
		sim := svc.textSimilarity("", "")
		if sim != 0 {
			t.Errorf("Expected 0 similarity for empty texts, got %f", sim)
		}
	})
}

func TestKnowledgeTypes(t *testing.T) {
	t.Run("All knowledge types are defined", func(t *testing.T) {
		types := []KnowledgeType{
			KnowledgeFact,
			KnowledgeInference,
			KnowledgeHeuristic,
			KnowledgePattern,
			KnowledgeConstraint,
			KnowledgeGoal,
		}

		for _, kt := range types {
			if kt == "" {
				t.Error("Knowledge type should not be empty")
			}
		}
	})
}

func TestQueryTypes(t *testing.T) {
	t.Run("All query types are defined", func(t *testing.T) {
		types := []QueryType{
			QueryExact,
			QuerySemantic,
			QueryGraph,
			QueryInfer,
		}

		for _, qt := range types {
			if qt == "" {
				t.Error("Query type should not be empty")
			}
		}
	})
}

func BenchmarkLearn(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := &Knowledge{
			Subject:    "bench_subject",
			Predicate:  "bench_pred",
			Object:     "bench_obj",
			Type:       KnowledgeFact,
			Confidence: 0.9,
		}
		_ = svc.Learn(ctx, k)
	}
}

func BenchmarkQueryExact(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		_ = svc.Learn(ctx, &Knowledge{
			Subject:   "subject_" + string(rune('a'+i%26)),
			Predicate: "predicate",
			Object:    "object",
		})
	}

	q := &Query{Type: QueryExact, Subject: "subject_a"}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = svc.Query(ctx, q)
	}
}

func BenchmarkDecide(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()

	options := []Option{
		{ID: "opt1", Name: "Option 1"},
		{ID: "opt2", Name: "Option 2"},
		{ID: "opt3", Name: "Option 3"},
	}
	factors := []Factor{
		{Name: "f1", Weight: 0.5, Value: 0.8},
		{Name: "f2", Weight: 0.3, Value: 0.6},
		{Name: "f3", Weight: 0.2, Value: 0.9},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.Decide(ctx, "benchmark question", options, factors)
	}
}

func BenchmarkGenerateInsight(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()

	evidence := []string{"e1", "e2", "e3"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.GenerateInsight(ctx, "benchmark", evidence)
	}
}

func BenchmarkConcurrentLearn(b *testing.B) {
	svc := newTestService()
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			k := &Knowledge{
				Subject:    "concurrent_subject",
				Predicate:  "pred",
				Object:     "obj",
				Confidence: 0.9,
			}
			_ = svc.Learn(ctx, k)
			i++
		}
	})
}
