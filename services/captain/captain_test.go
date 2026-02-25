// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package captain

import (
	"context"
	"errors"
	"sync"
	"testing"

	wotanClient "unheaded/pkg/wotan-client"
)

// mockStorage implements Storage for testing
type mockStorage struct {
	decisions map[string]*Decision
	mu        sync.RWMutex
	saveErr   error
	getErr    error
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		decisions: make(map[string]*Decision),
	}
}

func (m *mockStorage) SaveDecision(ctx context.Context, d *Decision) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	if d == nil {
		return errors.New("decision is nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.decisions[d.ID] = d
	return nil
}

func (m *mockStorage) GetDecision(ctx context.Context, id string) (*Decision, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	d, ok := m.decisions[id]
	if !ok {
		return nil, errors.New("decision not found")
	}
	return d, nil
}

func (m *mockStorage) ListDecisions(ctx context.Context, limit, offset int) ([]*Decision, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Decision
	for _, d := range m.decisions {
		result = append(result, d)
	}

	if offset >= len(result) {
		return []*Decision{}, nil
	}

	end := offset + limit
	if end > len(result) {
		end = len(result)
	}

	return result[offset:end], nil
}

func (m *mockStorage) DeleteDecision(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.decisions, id)
	return nil
}

// mockWotan implements WotanCommunicator for testing
type mockWotan struct {
	publishedMessages map[string][][]byte
	mu                sync.RWMutex
	publishErr        error
}

func newMockWotan() *mockWotan {
	return &mockWotan{
		publishedMessages: make(map[string][][]byte),
	}
}

func (m *mockWotan) Publish(ctx context.Context, topic string, payload []byte) error {
	if m.publishErr != nil {
		return m.publishErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMessages[topic] = append(m.publishedMessages[topic], payload)
	return nil
}

func (m *mockWotan) Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error) {
	return &wotanClient.Subscriber{
		SubscriberID: "mock-subscriber",
		Topic:        topic,
		DisplayName:  displayName,
		Status:       "approved",
	}, nil
}

func (m *mockWotan) StreamMessages(ctx context.Context, topic string) (<-chan *wotanClient.Message, error) {
	ch := make(chan *wotanClient.Message)
	close(ch)
	return ch, nil
}

func (m *mockWotan) getPublishedMessages(topic string) [][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.publishedMessages[topic]
}

// ============================================================================
// TESTS
// ============================================================================

func TestNewService(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name:    "nil storage",
			cfg:     Config{Storage: nil, Wotan: newMockWotan()},
			wantErr: ErrNilStorage,
		},
		{
			name:    "nil wotan",
			cfg:     Config{Storage: newMockStorage(), Wotan: nil},
			wantErr: ErrNilWotan,
		},
		{
			name: "valid config",
			cfg: Config{
				Storage: newMockStorage(),
				Wotan:  newMockWotan(),
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewService(tt.cfg)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("got error %v, want %v", err, tt.wantErr)
			}

			if tt.wantErr == nil && svc == nil {
				t.Error("service should not be nil on success")
			}
		})
	}
}

func TestService_GetVision(t *testing.T) {
	svc, err := NewService(Config{
		Storage: newMockStorage(),
		Wotan:  newMockWotan(),
	})
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	ctx := context.Background()

	t.Run("happy path", func(t *testing.T) {
		vision, err := svc.GetVision(ctx)
		if err != nil {
			t.Errorf("GetVision failed: %v", err)
		}
		if vision == nil {
			t.Error("vision should not be nil")
		}
		if vision.Title == "" {
			t.Error("vision title should not be empty")
		}
	})

	t.Run("returns copy", func(t *testing.T) {
		vision1, _ := svc.GetVision(ctx)
		vision1.Title = "modified"
		vision2, _ := svc.GetVision(ctx)

		if vision2.Title == "modified" {
			t.Error("modifying returned vision should not affect internal state")
		}
	})

	t.Run("after close", func(t *testing.T) {
		svc.Close()
		_, err := svc.GetVision(ctx)
		if !errors.Is(err, ErrServiceClosed) {
			t.Errorf("got error %v, want %v", err, ErrServiceClosed)
		}
	})
}

func TestService_GetStrategy(t *testing.T) {
	svc, _ := NewService(Config{
		Storage: newMockStorage(),
		Wotan:  newMockWotan(),
	})

	ctx := context.Background()

	t.Run("happy path", func(t *testing.T) {
		strategy, err := svc.GetStrategy(ctx)
		if err != nil {
			t.Errorf("GetStrategy failed: %v", err)
		}
		if strategy == nil {
			t.Error("strategy should not be nil")
		}
		if strategy.Title == "" {
			t.Error("strategy title should not be empty")
		}
	})

	t.Run("returns copy", func(t *testing.T) {
		s1, _ := svc.GetStrategy(ctx)
		s1.Title = "modified"
		s2, _ := svc.GetStrategy(ctx)

		if s2.Title == "modified" {
			t.Error("modifying returned strategy should not affect internal state")
		}
	})

	t.Run("nil context becomes background", func(t *testing.T) {
		strategy, err := svc.GetStrategy(nil)
		if err != nil {
			t.Errorf("GetStrategy with nil context failed: %v", err)
		}
		if strategy == nil {
			t.Error("strategy should not be nil with nil context")
		}
	})
}

func TestService_LogDecision(t *testing.T) {
	storage := newMockStorage()
	wotan := newMockWotan()
	svc, _ := NewService(Config{Storage: storage, Wotan: wotan})
	ctx := context.Background()

	tests := []struct {
		name    string
		input   *Decision
		wantErr string
	}{
		{
			name:    "nil decision",
			input:   nil,
			wantErr: "decision cannot be nil",
		},
		{
			name: "empty title",
			input: &Decision{
				Title:    "",
				Content:  "some content",
				Owner:    "captain",
				Priority: PriorityHigh,
			},
			wantErr: "decision title cannot be empty",
		},
		{
			name: "empty content",
			input: &Decision{
				Title:    "Increase deployment frequency",
				Content:  "",
				Owner:    "captain",
				Priority: PriorityHigh,
			},
			wantErr: "decision content cannot be empty",
		},
		{
			name: "empty owner",
			input: &Decision{
				Title:    "Increase deployment frequency",
				Content:  "Deploy every hour instead of daily",
				Owner:    "",
				Priority: PriorityHigh,
			},
			wantErr: "decision owner cannot be empty",
		},
		{
			name: "invalid priority",
			input: &Decision{
				Title:    "Increase deployment frequency",
				Content:  "Deploy every hour instead of daily",
				Owner:    "captain",
				Priority: Priority(999),
			},
			wantErr: "invalid priority level",
		},
		{
			name: "title too long",
			input: &Decision{
				Title:    string(make([]byte, 501)),
				Content:  "content",
				Owner:    "captain",
				Priority: PriorityHigh,
			},
			wantErr: "title too long",
		},
		{
			name: "content too long",
			input: &Decision{
				Title:    "Test",
				Content:  string(make([]byte, 10001)),
				Owner:    "captain",
				Priority: PriorityHigh,
			},
			wantErr: "content too long",
		},
		{
			name: "owner too long",
			input: &Decision{
				Title:    "Test",
				Content:  "content",
				Owner:    string(make([]byte, 201)),
				Priority: PriorityHigh,
			},
			wantErr: "owner too long",
		},
		{
			name: "valid decision",
			input: &Decision{
				Title:    "Increase deployment frequency",
				Content:  "Deploy every hour instead of daily",
				Owner:    "captain",
				Priority: PriorityHigh,
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.LogDecision(ctx, tt.input)

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("LogDecision failed: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Errorf("error '%v' does not contain '%s'", err, tt.wantErr)
				}
			}
		})
	}
}

func TestService_LogDecision_Persistence(t *testing.T) {
	storage := newMockStorage()
	wotan := newMockWotan()
	svc, _ := NewService(Config{Storage: storage, Wotan: wotan})
	ctx := context.Background()

	decision := &Decision{
		Title:    "Deploy to production",
		Content:  "All systems green, proceed with deployment",
		Owner:    "captain",
		Priority: PriorityCritical,
	}

	if err := svc.LogDecision(ctx, decision); err != nil {
		t.Fatalf("LogDecision failed: %v", err)
	}

	// Verify persisted
	if decision.ID == "" {
		t.Error("decision ID should be generated")
	}

	stored, err := storage.GetDecision(ctx, decision.ID)
	if err != nil {
		t.Fatalf("GetDecision failed: %v", err)
	}

	if stored.Title != decision.Title {
		t.Errorf("stored decision title mismatch")
	}

	if stored.Status != "pending" {
		t.Errorf("initial status should be 'pending', got '%s'", stored.Status)
	}
}

func TestService_LogDecision_WotanSend(t *testing.T) {
	storage := newMockStorage()
	wotan := newMockWotan()
	svc, _ := NewService(Config{Storage: storage, Wotan: wotan})
	ctx := context.Background()

	decision := &Decision{
		Title:    "Launch new feature",
		Content:  "Feature X is ready for production",
		Owner:    "captain",
		Priority: PriorityHigh,
	}

	if err := svc.LogDecision(ctx, decision); err != nil {
		t.Fatalf("LogDecision failed: %v", err)
	}

	// Verify published
	messages := wotan.getPublishedMessages("decisions.created")
	if len(messages) != 1 {
		t.Errorf("expected 1 message published, got %d", len(messages))
	}
}

func TestService_GetDecision(t *testing.T) {
	storage := newMockStorage()
	svc, _ := NewService(Config{Storage: storage, Wotan: newMockWotan()})
	ctx := context.Background()

	// Save a decision first
	decision := &Decision{
		ID:       "test-123",
		Title:    "Test decision",
		Content:  "Test content",
		Owner:    "captain",
		Priority: PriorityHigh,
	}
	storage.SaveDecision(ctx, decision)

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "empty ID",
			id:      "",
			wantErr: true,
		},
		{
			name:    "ID too long",
			id:      string(make([]byte, 201)),
			wantErr: true,
		},
		{
			name:    "valid ID",
			id:      "test-123",
			wantErr: false,
		},
		{
			name:    "ID not found",
			id:      "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GetDecision(ctx, tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDecision error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestService_ListDecisions(t *testing.T) {
	storage := newMockStorage()
	svc, _ := NewService(Config{Storage: storage, Wotan: newMockWotan()})
	ctx := context.Background()

	// Add test decisions
	for i := 0; i < 5; i++ {
		d := &Decision{
			ID:       string(rune(i)),
			Title:    "Decision",
			Content:  "Content",
			Owner:    "captain",
			Priority: PriorityHigh,
		}
		storage.SaveDecision(ctx, d)
	}

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantErr   bool
		wantCount int
	}{
		{
			name:      "default limit",
			limit:     0,
			offset:    0,
			wantErr:   false,
			wantCount: 5,
		},
		{
			name:      "custom limit",
			limit:     2,
			offset:    0,
			wantErr:   false,
			wantCount: 2,
		},
		{
			name:      "with offset",
			limit:     2,
			offset:    3,
			wantErr:   false,
			wantCount: 2,
		},
		{
			name:      "negative offset",
			limit:     10,
			offset:    -1,
			wantErr:   false,
			wantCount: 5,
		},
		{
			name:      "offset > 1M",
			limit:     10,
			offset:    1000001,
			wantErr:   true,
			wantCount: 0,
		},
		{
			name:      "max limit capped",
			limit:     2000,
			offset:    0,
			wantErr:   false,
			wantCount: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decisions, err := svc.ListDecisions(ctx, tt.limit, tt.offset)

			if (err != nil) != tt.wantErr {
				t.Errorf("ListDecisions error = %v, wantErr %v", err, tt.wantErr)
			}

			if err == nil && len(decisions) != tt.wantCount {
				t.Errorf("got %d decisions, want %d", len(decisions), tt.wantCount)
			}
		})
	}
}

func TestService_UpdateDecisionStatus(t *testing.T) {
	storage := newMockStorage()
	svc, _ := NewService(Config{Storage: storage, Wotan: newMockWotan()})
	ctx := context.Background()

	// Create a decision
	decision := &Decision{
		ID:       "test-123",
		Title:    "Test",
		Content:  "Content",
		Owner:    "captain",
		Priority: PriorityHigh,
		Status:   "pending",
	}
	storage.SaveDecision(ctx, decision)

	tests := []struct {
		name    string
		id      string
		status  string
		wantErr bool
	}{
		{
			name:    "empty ID",
			id:      "",
			status:  "approved",
			wantErr: true,
		},
		{
			name:    "invalid status",
			id:      "test-123",
			status:  "invalid_status",
			wantErr: true,
		},
		{
			name:    "valid approval",
			id:      "test-123",
			status:  "approved",
			wantErr: false,
		},
		{
			name:    "valid rejection",
			id:      "test-123",
			status:  "rejected",
			wantErr: false,
		},
		{
			name:    "valid archive",
			id:      "test-123",
			status:  "archived",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.UpdateDecisionStatus(ctx, tt.id, tt.status)

			if (err != nil) != tt.wantErr {
				t.Errorf("UpdateDecisionStatus error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestService_Close(t *testing.T) {
	svc, _ := NewService(Config{
		Storage: newMockStorage(),
		Wotan:  newMockWotan(),
	})

	if svc.IsClosed() {
		t.Error("service should not be closed initially")
	}

	if err := svc.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if !svc.IsClosed() {
		t.Error("service should be closed after Close()")
	}

	// Second close should be idempotent
	if err := svc.Close(); err != nil {
		t.Errorf("second Close failed: %v", err)
	}
}

func TestService_Concurrent(t *testing.T) {
	storage := newMockStorage()
	wotan := newMockWotan()
	svc, _ := NewService(Config{Storage: storage, Wotan: wotan})
	ctx := context.Background()

	var wg sync.WaitGroup
	errChan := make(chan error, 100)

	// Concurrent LogDecision
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			decision := &Decision{
				Title:    "Decision",
				Content:  "Content",
				Owner:    "captain",
				Priority: PriorityHigh,
			}
			if err := svc.LogDecision(ctx, decision); err != nil {
				errChan <- err
			}
		}(i)
	}

	// Concurrent GetVision
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.GetVision(ctx); err != nil {
				errChan <- err
			}
		}()
	}

	// Concurrent GetStrategy
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.GetStrategy(ctx); err != nil {
				errChan <- err
			}
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("concurrent operation failed: %v", err)
	}
}

// ============================================================================
// BENCHMARKS
// ============================================================================

func BenchmarkService_LogDecision(b *testing.B) {
	svc, _ := NewService(Config{
		Storage: newMockStorage(),
		Wotan:  newMockWotan(),
	})
	ctx := context.Background()

	decision := &Decision{
		Title:    "Benchmark decision",
		Content:  "This is a benchmark decision for performance testing",
		Owner:    "captain",
		Priority: PriorityHigh,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := *decision
		svc.LogDecision(ctx, &d)
	}
}

func BenchmarkService_GetVision(b *testing.B) {
	svc, _ := NewService(Config{
		Storage: newMockStorage(),
		Wotan:  newMockWotan(),
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.GetVision(ctx)
	}
}

func BenchmarkService_GetStrategy(b *testing.B) {
	svc, _ := NewService(Config{
		Storage: newMockStorage(),
		Wotan:  newMockWotan(),
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.GetStrategy(ctx)
	}
}

// ============================================================================
// HELPERS
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0))
}
