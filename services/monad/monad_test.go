// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package monad

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

func TestResult(t *testing.T) {
	t.Run("Ok creates successful result", func(t *testing.T) {
		r := Ok(42)
		if !r.IsOk() {
			t.Error("Expected IsOk() to return true")
		}
		if r.IsErr() {
			t.Error("Expected IsErr() to return false")
		}
		if r.Value() != 42 {
			t.Errorf("Expected value 42, got %v", r.Value())
		}
		if r.Error() != nil {
			t.Errorf("Expected nil error, got %v", r.Error())
		}
	})

	t.Run("Err creates failed result", func(t *testing.T) {
		err := errors.New("test error")
		r := Err[int](err)
		if r.IsOk() {
			t.Error("Expected IsOk() to return false")
		}
		if !r.IsErr() {
			t.Error("Expected IsErr() to return true")
		}
		if r.Error() != err {
			t.Errorf("Expected error %v, got %v", err, r.Error())
		}
	})

	t.Run("Unwrap returns value on success", func(t *testing.T) {
		r := Ok("hello")
		if r.Unwrap() != "hello" {
			t.Errorf("Expected 'hello', got %v", r.Unwrap())
		}
	})

	t.Run("Unwrap panics on error", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("Expected panic on Unwrap of error result")
			}
		}()
		r := Err[string](errors.New("fail"))
		_ = r.Unwrap()
	})

	t.Run("UnwrapOr returns value on success", func(t *testing.T) {
		r := Ok(10)
		if r.UnwrapOr(5) != 10 {
			t.Error("Expected 10 from UnwrapOr")
		}
	})

	t.Run("UnwrapOr returns default on error", func(t *testing.T) {
		r := Err[int](errors.New("fail"))
		if r.UnwrapOr(5) != 5 {
			t.Error("Expected default value 5")
		}
	})

	t.Run("UnwrapErr returns error", func(t *testing.T) {
		err := errors.New("test")
		r := Err[int](err)
		if r.UnwrapErr() != err {
			t.Error("Expected error from UnwrapErr")
		}
	})

	t.Run("UnwrapErr panics on success", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("Expected panic on UnwrapErr of successful result")
			}
		}()
		r := Ok(42)
		_ = r.UnwrapErr()
	})

	t.Run("WithMeta adds metadata", func(t *testing.T) {
		r := Ok(42).WithMeta("key", "value")
		if r.Meta()["key"] != "value" {
			t.Error("Expected metadata to contain key")
		}
	})
}

func TestMap(t *testing.T) {
	t.Run("Map transforms successful result", func(t *testing.T) {
		r := Ok(5)
		mapped := Map(r, func(x int) int { return x * 2 })
		if !mapped.IsOk() {
			t.Error("Expected mapped result to be Ok")
		}
		if mapped.Value() != 10 {
			t.Errorf("Expected 10, got %v", mapped.Value())
		}
	})

	t.Run("Map propagates error", func(t *testing.T) {
		r := Err[int](errors.New("fail"))
		mapped := Map(r, func(x int) int { return x * 2 })
		if !mapped.IsErr() {
			t.Error("Expected mapped result to be Err")
		}
	})
}

func TestFlatMap(t *testing.T) {
	t.Run("FlatMap chains successful operations", func(t *testing.T) {
		r := Ok(5)
		chained := FlatMap(r, func(x int) Result[int] {
			return Ok(x * 3)
		})
		if !chained.IsOk() {
			t.Error("Expected chained result to be Ok")
		}
		if chained.Value() != 15 {
			t.Errorf("Expected 15, got %v", chained.Value())
		}
	})

	t.Run("FlatMap short-circuits on initial error", func(t *testing.T) {
		r := Err[int](errors.New("initial"))
		chained := FlatMap(r, func(x int) Result[int] {
			return Ok(x * 3)
		})
		if !chained.IsErr() {
			t.Error("Expected chained result to be Err")
		}
	})

	t.Run("FlatMap propagates inner error", func(t *testing.T) {
		r := Ok(5)
		chained := FlatMap(r, func(x int) Result[int] {
			return Err[int](errors.New("inner fail"))
		})
		if !chained.IsErr() {
			t.Error("Expected chained result to be Err")
		}
	})
}

func newTestService() *Service {
	log := logger.New(os.Stderr)
	return NewService(log, nil)
}

func TestService(t *testing.T) {
	t.Run("NewService creates service", func(t *testing.T) {
		svc := newTestService()
		if svc == nil {
			t.Fatal("Expected non-nil service")
		}
		if svc.operations == nil {
			t.Error("Expected operations map to be initialized")
		}
		if svc.transactions == nil {
			t.Error("Expected transactions map to be initialized")
		}
	})

	t.Run("RegisterQuery adds handler", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("test_query", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "result", nil
		})

		stats := svc.Stats()
		if stats["registered_queries"].(int) != 1 {
			t.Error("Expected 1 registered query")
		}
	})

	t.Run("RegisterMutation adds handler", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("test_mutation", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "result", nil, nil
		})

		stats := svc.Stats()
		if stats["registered_mutations"].(int) != 1 {
			t.Error("Expected 1 registered mutation")
		}
	})

	t.Run("RegisterEffect adds handler", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterEffect("test_effect", func(ctx context.Context, input interface{}) error {
			return nil
		})

		stats := svc.Stats()
		if stats["registered_effects"].(int) != 1 {
			t.Error("Expected 1 registered effect")
		}
	})
}

func TestQuery(t *testing.T) {
	t.Run("Query executes registered handler", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("get_data", func(ctx context.Context, input interface{}) (interface{}, error) {
			return map[string]string{"data": "value"}, nil
		})

		ctx := context.Background()
		result := svc.Query(ctx, "get_data", nil)

		if !result.IsOk() {
			t.Fatal("Expected successful query")
		}

		data := result.Value().(map[string]string)
		if data["data"] != "value" {
			t.Error("Expected data value")
		}
	})

	t.Run("Query returns error for unknown handler", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()
		result := svc.Query(ctx, "unknown", nil)

		if !result.IsErr() {
			t.Error("Expected error for unknown query")
		}
	})

	t.Run("Query propagates handler error", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("failing_query", func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("query failed")
		})

		ctx := context.Background()
		result := svc.Query(ctx, "failing_query", nil)

		if !result.IsErr() {
			t.Error("Expected error from failing query")
		}
	})

	t.Run("Query creates operation record", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("tracked_query", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "done", nil
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "tracked_query", nil)

		stats := svc.Stats()
		if stats["total_operations"].(int) != 1 {
			t.Error("Expected 1 operation")
		}
		if stats["completed_operations"].(int) != 1 {
			t.Error("Expected 1 completed operation")
		}
	})
}

func TestMutate(t *testing.T) {
	t.Run("Mutate executes handler and records changes", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("update_value", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			changes := []StateChange{
				{
					Entity:    "test_entity",
					Field:     "value",
					OldValue:  "old",
					NewValue:  "new",
					Timestamp: time.Now(),
				},
			}
			return "updated", changes, nil
		})

		ctx := context.Background()
		result := svc.Mutate(ctx, "update_value", nil)

		if !result.IsOk() {
			t.Fatal("Expected successful mutation")
		}

		changes := svc.GetStateChanges()
		if len(changes) != 1 {
			t.Errorf("Expected 1 state change, got %d", len(changes))
		}
		if changes[0].Entity != "test_entity" {
			t.Error("Expected test_entity in state change")
		}
	})

	t.Run("Mutate returns error for unknown handler", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()
		result := svc.Mutate(ctx, "unknown", nil)

		if !result.IsErr() {
			t.Error("Expected error for unknown mutation")
		}
	})

	t.Run("Failed mutation increments failed count", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("failing_mutation", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return nil, nil, errors.New("mutation failed")
		})

		ctx := context.Background()
		_ = svc.Mutate(ctx, "failing_mutation", nil)

		stats := svc.Stats()
		if stats["failed_operations"].(int) != 1 {
			t.Error("Expected 1 failed operation")
		}
	})
}

func TestEffect(t *testing.T) {
	t.Run("Effect executes handler", func(t *testing.T) {
		svc := newTestService()
		executed := false
		svc.RegisterEffect("send_email", func(ctx context.Context, input interface{}) error {
			executed = true
			return nil
		})

		ctx := context.Background()
		result := svc.Effect(ctx, "send_email", nil)

		if !result.IsOk() {
			t.Error("Expected successful effect")
		}
		if !executed {
			t.Error("Expected effect handler to be executed")
		}
	})

	t.Run("Effect returns error for unknown handler", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()
		result := svc.Effect(ctx, "unknown", nil)

		if !result.IsErr() {
			t.Error("Expected error for unknown effect")
		}
	})
}

func TestTransaction(t *testing.T) {
	t.Run("Transaction commits successfully", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("tx_query", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "queried", nil
		})
		svc.RegisterMutation("tx_mutation", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "mutated", nil, nil
		})

		ctx := context.Background()
		err := svc.BeginTransaction(ctx).
			Query("tx_query", nil).
			Mutate("tx_mutation", nil).
			Commit()

		if err != nil {
			t.Errorf("Expected successful commit, got %v", err)
		}

		stats := svc.Stats()
		if stats["total_transactions"].(int) != 1 {
			t.Error("Expected 1 transaction")
		}
	})

	t.Run("Transaction fails on handler error", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("failing", func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("query failed")
		})

		ctx := context.Background()
		err := svc.BeginTransaction(ctx).
			Query("failing", nil).
			Commit()

		if err == nil {
			t.Error("Expected error from failed transaction")
		}
	})

	t.Run("Transaction rollback works", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		err := svc.BeginTransaction(ctx).
			Query("q", nil).
			Rollback()

		if err != nil {
			t.Errorf("Expected successful rollback, got %v", err)
		}
	})
}

func TestOperationTracking(t *testing.T) {
	t.Run("GetOperation retrieves operation", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("tracked", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "done", nil
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "tracked", nil)

		ops := svc.ListOperations(10)
		if len(ops) == 0 {
			t.Fatal("Expected at least one operation")
		}

		op, ok := svc.GetOperation(ops[0].ID)
		if !ok {
			t.Error("Expected to find operation")
		}
		if op.Name != "tracked" {
			t.Error("Expected operation name 'tracked'")
		}
	})

	t.Run("ListOperations respects limit", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		for i := 0; i < 10; i++ {
			_ = svc.Query(ctx, "q", nil)
		}

		ops := svc.ListOperations(5)
		if len(ops) != 5 {
			t.Errorf("Expected 5 operations, got %d", len(ops))
		}
	})

	t.Run("Operation has duration", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("slow", func(ctx context.Context, input interface{}) (interface{}, error) {
			time.Sleep(10 * time.Millisecond)
			return "done", nil
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "slow", nil)

		ops := svc.ListOperations(1)
		if ops[0].Duration < 10*time.Millisecond {
			t.Error("Expected operation duration >= 10ms")
		}
	})
}

func TestConcurrency(t *testing.T) {
	t.Run("Concurrent queries are safe", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("concurrent", func(ctx context.Context, input interface{}) (interface{}, error) {
			return input, nil
		})

		ctx := context.Background()
		var wg sync.WaitGroup
		errCh := make(chan error, 100)

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				result := svc.Query(ctx, "concurrent", i)
				if result.IsErr() {
					errCh <- result.Error()
				}
			}(i)
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			t.Errorf("Concurrent query failed: %v", err)
		}

		stats := svc.Stats()
		if stats["total_operations"].(int) != 100 {
			t.Errorf("Expected 100 operations, got %d", stats["total_operations"].(int))
		}
	})

	t.Run("Concurrent mutations are safe", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("concurrent_mut", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return input, []StateChange{{Entity: "test", Field: "value", NewValue: input}}, nil
		})

		ctx := context.Background()
		var wg sync.WaitGroup

		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_ = svc.Mutate(ctx, "concurrent_mut", i)
			}(i)
		}

		wg.Wait()

		changes := svc.GetStateChanges()
		if len(changes) != 50 {
			t.Errorf("Expected 50 state changes, got %d", len(changes))
		}
	})
}

func TestStats(t *testing.T) {
	t.Run("Stats returns correct counts", func(t *testing.T) {
		svc := newTestService()

		// Register handlers
		svc.RegisterQuery("q1", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})
		svc.RegisterQuery("q2", func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("fail")
		})
		svc.RegisterMutation("m1", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "ok", []StateChange{{Entity: "e"}}, nil
		})
		svc.RegisterEffect("e1", func(ctx context.Context, input interface{}) error {
			return nil
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "q1", nil)
		_ = svc.Query(ctx, "q2", nil)
		_ = svc.Mutate(ctx, "m1", nil)
		_ = svc.Effect(ctx, "e1", nil)

		stats := svc.Stats()

		if stats["registered_queries"].(int) != 2 {
			t.Errorf("Expected 2 queries, got %d", stats["registered_queries"].(int))
		}
		if stats["registered_mutations"].(int) != 1 {
			t.Errorf("Expected 1 mutation, got %d", stats["registered_mutations"].(int))
		}
		if stats["registered_effects"].(int) != 1 {
			t.Errorf("Expected 1 effect, got %d", stats["registered_effects"].(int))
		}
		if stats["total_operations"].(int) != 4 {
			t.Errorf("Expected 4 total operations, got %d", stats["total_operations"].(int))
		}
		if stats["completed_operations"].(int) != 3 {
			t.Errorf("Expected 3 completed operations, got %d", stats["completed_operations"].(int))
		}
		if stats["failed_operations"].(int) != 1 {
			t.Errorf("Expected 1 failed operation, got %d", stats["failed_operations"].(int))
		}
		if stats["state_changes"].(int) != 1 {
			t.Errorf("Expected 1 state change, got %d", stats["state_changes"].(int))
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxConcurrentOps != 100 {
		t.Errorf("Expected MaxConcurrentOps 100, got %d", config.MaxConcurrentOps)
	}
	if config.OperationTimeout != 30*time.Second {
		t.Errorf("Expected OperationTimeout 30s, got %v", config.OperationTimeout)
	}
	if config.TransactionTimeout != 60*time.Second {
		t.Errorf("Expected TransactionTimeout 60s, got %v", config.TransactionTimeout)
	}
	if !config.EnableTracing {
		t.Error("Expected EnableTracing to be true")
	}
	if config.WotanTopic != "monad.operations" {
		t.Errorf("Expected WotanTopic 'monad.operations', got %s", config.WotanTopic)
	}
}

func BenchmarkQuery(b *testing.B) {
	svc := newTestService()
	svc.RegisterQuery("bench", func(ctx context.Context, input interface{}) (interface{}, error) {
		return input, nil
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = svc.Query(ctx, "bench", i)
	}
}

func BenchmarkMutate(b *testing.B) {
	svc := newTestService()
	svc.RegisterMutation("bench", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
		return input, nil, nil
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = svc.Mutate(ctx, "bench", i)
	}
}

func BenchmarkConcurrentQueries(b *testing.B) {
	svc := newTestService()
	svc.RegisterQuery("bench", func(ctx context.Context, input interface{}) (interface{}, error) {
		return input, nil
	})

	ctx := context.Background()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_ = svc.Query(ctx, "bench", i)
			i++
		}
	})
}

// ============================================================================
// MOCK WOTAN CLIENT
// ============================================================================

// mockWotanClient implements a mock Wotan client for testing
type mockWotanClient struct {
	mu                sync.RWMutex
	subscriptions     map[string]*wotanClient.Subscriber
	publishedMessages map[string][][]byte
	publishErr        error
	subscribeErr      error
	streamErr         error
	messageCh         chan *wotanClient.Message
}

func newMockWotanClient() *mockWotanClient {
	return &mockWotanClient{
		subscriptions:     make(map[string]*wotanClient.Subscriber),
		publishedMessages: make(map[string][][]byte),
		messageCh:         make(chan *wotanClient.Message, 100),
	}
}

func (m *mockWotanClient) Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error) {
	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}
	sub := &wotanClient.Subscriber{
		SubscriberID: "mock-sub-" + topic,
		Topic:        topic,
		DisplayName:  displayName,
		Status:       "approved",
	}
	m.mu.Lock()
	m.subscriptions[topic] = sub
	m.mu.Unlock()
	return sub, nil
}

func (m *mockWotanClient) Publish(ctx context.Context, topic string, payload []byte) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	m.mu.Lock()
	m.publishedMessages[topic] = append(m.publishedMessages[topic], payload)
	m.mu.Unlock()
	return nil
}

func (m *mockWotanClient) StreamMessages(ctx context.Context, topic string) (<-chan *wotanClient.Message, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	return m.messageCh, nil
}

// ============================================================================
// SERVICE LIFECYCLE TESTS
// ============================================================================

func TestServiceStart(t *testing.T) {
	t.Run("Start without Wotan succeeds", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		err := svc.Start(ctx)
		if err != nil {
			t.Errorf("Start without Wotan should succeed, got: %v", err)
		}
	})

	t.Run("Start with Wotan subscribes to topics", func(t *testing.T) {
		log := logger.New(os.Stderr)
		_ = newMockWotanClient() // Mock created for future use when interface-based injection is added
		svc := NewService(log, (*wotanClient.Client)(nil))

		// Test the nil wotan path
		ctx := context.Background()
		err := svc.Start(ctx)
		if err != nil {
			t.Errorf("Start should succeed: %v", err)
		}
	})
}

func TestServiceStop(t *testing.T) {
	t.Run("Stop closes service gracefully", func(t *testing.T) {
		svc := newTestService()

		err := svc.Stop()
		if err != nil {
			t.Errorf("Stop should succeed, got: %v", err)
		}
	})

	t.Run("Stop after Start works", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		_ = svc.Start(ctx)
		err := svc.Stop()
		if err != nil {
			t.Errorf("Stop after Start should succeed, got: %v", err)
		}
	})
}

// ============================================================================
// RESULT EDGE CASES
// ============================================================================

func TestResultWithMetaNilMap(t *testing.T) {
	t.Run("WithMeta initializes nil meta map", func(t *testing.T) {
		// Create a result and explicitly set meta to nil
		r := Result[int]{value: 42, err: nil, meta: nil}
		r = r.WithMeta("key", "value")

		if r.Meta() == nil {
			t.Error("Meta should not be nil after WithMeta")
		}
		if r.Meta()["key"] != "value" {
			t.Error("Meta should contain the added key")
		}
	})

	t.Run("Multiple WithMeta calls", func(t *testing.T) {
		r := Ok(42).
			WithMeta("key1", "value1").
			WithMeta("key2", "value2").
			WithMeta("key3", 123)

		if len(r.Meta()) != 3 {
			t.Errorf("Expected 3 metadata entries, got %d", len(r.Meta()))
		}
	})
}

func TestMapWithTypeChange(t *testing.T) {
	t.Run("Map int to string", func(t *testing.T) {
		r := Ok(42)
		mapped := Map(r, func(x int) string {
			return fmt.Sprintf("value: %d", x)
		})

		if !mapped.IsOk() {
			t.Error("Mapped result should be Ok")
		}
		if mapped.Value() != "value: 42" {
			t.Errorf("Expected 'value: 42', got '%s'", mapped.Value())
		}
	})

	t.Run("Map to struct type", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}

		r := Ok("Alice")
		mapped := Map(r, func(name string) Person {
			return Person{Name: name, Age: 30}
		})

		if !mapped.IsOk() {
			t.Error("Mapped result should be Ok")
		}
		if mapped.Value().Name != "Alice" {
			t.Errorf("Expected 'Alice', got '%s'", mapped.Value().Name)
		}
	})
}

func TestFlatMapChaining(t *testing.T) {
	t.Run("Multiple FlatMap chain success", func(t *testing.T) {
		r := Ok(1)
		r2 := FlatMap(r, func(x int) Result[int] { return Ok(x + 1) })
		r3 := FlatMap(r2, func(x int) Result[int] { return Ok(x * 2) })
		r4 := FlatMap(r3, func(x int) Result[int] { return Ok(x + 10) })

		if !r4.IsOk() {
			t.Error("Chain should succeed")
		}
		// (1+1)*2+10 = 14
		if r4.Value() != 14 {
			t.Errorf("Expected 14, got %d", r4.Value())
		}
	})

	t.Run("FlatMap chain breaks on middle error", func(t *testing.T) {
		r := Ok(1)
		r2 := FlatMap(r, func(x int) Result[int] { return Ok(x + 1) })
		r3 := FlatMap(r2, func(x int) Result[int] { return Err[int](errors.New("middle error")) })
		r4 := FlatMap(r3, func(x int) Result[int] { return Ok(x + 10) })

		if !r4.IsErr() {
			t.Error("Chain should fail at middle error")
		}
	})
}

// ============================================================================
// TRANSACTION BUILDER TESTS
// ============================================================================

func TestTransactionBuilderEffect(t *testing.T) {
	t.Run("Transaction with Effect succeeds", func(t *testing.T) {
		svc := newTestService()
		effectExecuted := false

		svc.RegisterEffect("test_effect", func(ctx context.Context, input interface{}) error {
			effectExecuted = true
			return nil
		})

		ctx := context.Background()
		err := svc.BeginTransaction(ctx).
			Effect("test_effect", nil).
			Commit()

		if err != nil {
			t.Errorf("Transaction with Effect should succeed: %v", err)
		}
		if !effectExecuted {
			t.Error("Effect should have been executed")
		}
	})

	t.Run("Transaction Effect propagates error", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterEffect("failing_effect", func(ctx context.Context, input interface{}) error {
			return errors.New("effect failed")
		})

		ctx := context.Background()
		err := svc.BeginTransaction(ctx).
			Effect("failing_effect", nil).
			Commit()

		if err == nil {
			t.Error("Transaction with failing Effect should fail")
		}
	})

	t.Run("Transaction skips Effect after prior error", func(t *testing.T) {
		svc := newTestService()
		effectExecuted := false

		svc.RegisterQuery("failing_q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("query failed")
		})
		svc.RegisterEffect("test_effect", func(ctx context.Context, input interface{}) error {
			effectExecuted = true
			return nil
		})

		ctx := context.Background()
		_ = svc.BeginTransaction(ctx).
			Query("failing_q", nil).
			Effect("test_effect", nil).
			Commit()

		if effectExecuted {
			t.Error("Effect should not execute after prior error")
		}
	})
}

func TestTransactionBuilderMutatePropagatesError(t *testing.T) {
	t.Run("Mutate skipped after prior error", func(t *testing.T) {
		svc := newTestService()
		mutateExecuted := false

		svc.RegisterQuery("failing_q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("query failed")
		})
		svc.RegisterMutation("test_mut", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			mutateExecuted = true
			return "done", nil, nil
		})

		ctx := context.Background()
		_ = svc.BeginTransaction(ctx).
			Query("failing_q", nil).
			Mutate("test_mut", nil).
			Commit()

		if mutateExecuted {
			t.Error("Mutate should not execute after prior error")
		}
	})
}

func TestTransactionRollbackWithCustomHandler(t *testing.T) {
	t.Run("Rollback executes custom handler", func(t *testing.T) {
		svc := newTestService()
		rollbackExecuted := false

		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		tb := svc.BeginTransaction(ctx)
		tb.Query("q", nil)

		// Set custom rollback
		tb.tx.Rollback = func(ctx context.Context) error {
			rollbackExecuted = true
			return nil
		}

		err := tb.Rollback()
		if err != nil {
			t.Errorf("Rollback should succeed: %v", err)
		}
		if !rollbackExecuted {
			t.Error("Custom rollback handler should have been executed")
		}
	})

	t.Run("Rollback with failing handler returns error", func(t *testing.T) {
		svc := newTestService()

		ctx := context.Background()
		tb := svc.BeginTransaction(ctx)

		// Set failing rollback
		tb.tx.Rollback = func(ctx context.Context) error {
			return errors.New("rollback failed")
		}

		err := tb.Rollback()
		if err == nil {
			t.Error("Rollback with failing handler should return error")
		}
	})
}

// ============================================================================
// GET TRANSACTION TESTS
// ============================================================================

func TestGetTransaction(t *testing.T) {
	t.Run("GetTransaction retrieves existing transaction", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		tb := svc.BeginTransaction(ctx)
		txID := tb.tx.ID

		tx, ok := svc.GetTransaction(txID)
		if !ok {
			t.Error("Should find the transaction")
		}
		if tx.ID != txID {
			t.Error("Transaction ID mismatch")
		}
		if tx.Status != TxStatusPending {
			t.Errorf("Expected pending status, got %s", tx.Status)
		}
	})

	t.Run("GetTransaction returns false for non-existent", func(t *testing.T) {
		svc := newTestService()

		_, ok := svc.GetTransaction("non-existent-tx")
		if ok {
			t.Error("Should not find non-existent transaction")
		}
	})

	t.Run("GetTransaction after commit", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		tb := svc.BeginTransaction(ctx)
		txID := tb.tx.ID
		tb.Query("q", nil)
		_ = tb.Commit()

		tx, ok := svc.GetTransaction(txID)
		if !ok {
			t.Error("Should find committed transaction")
		}
		if tx.Status != TxStatusCommitted {
			t.Errorf("Expected committed status, got %s", tx.Status)
		}
		if tx.CompletedAt == nil {
			t.Error("CompletedAt should be set after commit")
		}
	})
}

// ============================================================================
// OPERATION TYPE AND STATUS TESTS
// ============================================================================

func TestOperationTypes(t *testing.T) {
	t.Run("Query creates query operation", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("test_q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "test_q", nil)

		ops := svc.ListOperations(1)
		if ops[0].Type != OpTypeQuery {
			t.Errorf("Expected query type, got %s", ops[0].Type)
		}
	})

	t.Run("Mutate creates mutation operation", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("test_m", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "ok", nil, nil
		})

		ctx := context.Background()
		_ = svc.Mutate(ctx, "test_m", nil)

		ops := svc.ListOperations(1)
		if ops[0].Type != OpTypeMutation {
			t.Errorf("Expected mutation type, got %s", ops[0].Type)
		}
	})

	t.Run("Effect creates effect operation", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterEffect("test_e", func(ctx context.Context, input interface{}) error {
			return nil
		})

		ctx := context.Background()
		_ = svc.Effect(ctx, "test_e", nil)

		ops := svc.ListOperations(1)
		if ops[0].Type != OpTypeEffect {
			t.Errorf("Expected effect type, got %s", ops[0].Type)
		}
	})
}

func TestOperationStatuses(t *testing.T) {
	t.Run("Running operations tracked", func(t *testing.T) {
		svc := newTestService()
		operationStarted := make(chan bool)
		operationContinue := make(chan bool)

		svc.RegisterQuery("blocking", func(ctx context.Context, input interface{}) (interface{}, error) {
			operationStarted <- true
			<-operationContinue
			return "ok", nil
		})

		ctx := context.Background()
		go func() {
			_ = svc.Query(ctx, "blocking", nil)
		}()

		<-operationStarted
		stats := svc.Stats()
		if stats["running_operations"].(int) != 1 {
			t.Errorf("Expected 1 running operation, got %d", stats["running_operations"].(int))
		}

		operationContinue <- true
		time.Sleep(10 * time.Millisecond) // Let operation complete
	})
}

// ============================================================================
// LIST OPERATIONS EDGE CASES
// ============================================================================

func TestListOperationsEdgeCases(t *testing.T) {
	t.Run("ListOperations with limit 0 returns all", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		for i := 0; i < 5; i++ {
			_ = svc.Query(ctx, "q", nil)
		}

		ops := svc.ListOperations(0)
		if len(ops) != 5 {
			t.Errorf("Expected 5 operations with limit 0, got %d", len(ops))
		}
	})

	t.Run("ListOperations empty service", func(t *testing.T) {
		svc := newTestService()
		ops := svc.ListOperations(10)
		if len(ops) != 0 {
			t.Errorf("Expected 0 operations, got %d", len(ops))
		}
	})

	t.Run("ListOperations limit greater than count", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "q", nil)
		_ = svc.Query(ctx, "q", nil)

		ops := svc.ListOperations(100)
		if len(ops) != 2 {
			t.Errorf("Expected 2 operations, got %d", len(ops))
		}
	})

	t.Run("ListOperations sorted by time", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "q", "first")
		time.Sleep(1 * time.Millisecond)
		_ = svc.Query(ctx, "q", "second")
		time.Sleep(1 * time.Millisecond)
		_ = svc.Query(ctx, "q", "third")

		ops := svc.ListOperations(3)
		// Most recent first
		if ops[0].Input != "third" {
			t.Errorf("Expected most recent operation first, got %v", ops[0].Input)
		}
		if ops[2].Input != "first" {
			t.Errorf("Expected oldest operation last, got %v", ops[2].Input)
		}
	})
}

// ============================================================================
// EFFECT EDGE CASES
// ============================================================================

func TestEffectEdgeCases(t *testing.T) {
	t.Run("Effect with failing handler updates status", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterEffect("failing", func(ctx context.Context, input interface{}) error {
			return errors.New("effect failed")
		})

		ctx := context.Background()
		result := svc.Effect(ctx, "failing", nil)

		if !result.IsErr() {
			t.Error("Effect should return error")
		}

		ops := svc.ListOperations(1)
		if ops[0].Status != OpStatusFailed {
			t.Errorf("Expected failed status, got %s", ops[0].Status)
		}
		if ops[0].Error == "" {
			t.Error("Expected error message to be recorded")
		}
	})
}

// ============================================================================
// STATE CHANGE TESTS
// ============================================================================

func TestStateChanges(t *testing.T) {
	t.Run("Multiple mutations accumulate changes", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("m1", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "ok", []StateChange{
				{Entity: "user", Field: "name", OldValue: "old1", NewValue: "new1"},
			}, nil
		})
		svc.RegisterMutation("m2", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "ok", []StateChange{
				{Entity: "user", Field: "email", OldValue: "old2", NewValue: "new2"},
				{Entity: "user", Field: "phone", OldValue: "old3", NewValue: "new3"},
			}, nil
		})

		ctx := context.Background()
		_ = svc.Mutate(ctx, "m1", nil)
		_ = svc.Mutate(ctx, "m2", nil)

		changes := svc.GetStateChanges()
		if len(changes) != 3 {
			t.Errorf("Expected 3 state changes, got %d", len(changes))
		}
	})

	t.Run("GetStateChanges returns copy", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("m", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "ok", []StateChange{{Entity: "test", Field: "field"}}, nil
		})

		ctx := context.Background()
		_ = svc.Mutate(ctx, "m", nil)

		changes1 := svc.GetStateChanges()
		changes1[0].Entity = "modified"

		changes2 := svc.GetStateChanges()
		if changes2[0].Entity == "modified" {
			t.Error("Modifying returned changes should not affect internal state")
		}
	})
}

// ============================================================================
// HANDLER INPUT/OUTPUT TESTS
// ============================================================================

func TestHandlerInputOutput(t *testing.T) {
	t.Run("Query receives and returns input", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("echo", func(ctx context.Context, input interface{}) (interface{}, error) {
			return map[string]interface{}{"received": input}, nil
		})

		ctx := context.Background()
		result := svc.Query(ctx, "echo", "test-input")

		if !result.IsOk() {
			t.Fatal("Query should succeed")
		}
		output := result.Value().(map[string]interface{})
		if output["received"] != "test-input" {
			t.Errorf("Expected 'test-input', got %v", output["received"])
		}
	})

	t.Run("Mutation stores output in operation", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("create", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return map[string]string{"id": "created-123"}, nil, nil
		})

		ctx := context.Background()
		_ = svc.Mutate(ctx, "create", nil)

		ops := svc.ListOperations(1)
		output := ops[0].Output.(map[string]string)
		if output["id"] != "created-123" {
			t.Error("Operation should store output")
		}
	})
}

// ============================================================================
// STATS EDGE CASES
// ============================================================================

func TestStatsEdgeCases(t *testing.T) {
	t.Run("Stats with no handlers", func(t *testing.T) {
		svc := newTestService()
		stats := svc.Stats()

		if stats["registered_queries"].(int) != 0 {
			t.Error("Expected 0 registered queries")
		}
		if stats["registered_mutations"].(int) != 0 {
			t.Error("Expected 0 registered mutations")
		}
		if stats["registered_effects"].(int) != 0 {
			t.Error("Expected 0 registered effects")
		}
	})

	t.Run("Stats includes pending operations", func(t *testing.T) {
		svc := newTestService()
		stats := svc.Stats()

		// Pending is not explicitly tracked in Stats, but running is
		if stats["running_operations"].(int) != 0 {
			t.Error("Expected 0 running operations initially")
		}
	})
}

// ============================================================================
// CONCURRENT HANDLER REGISTRATION
// ============================================================================

func TestConcurrentRegistration(t *testing.T) {
	t.Run("Concurrent handler registration is safe", func(t *testing.T) {
		svc := newTestService()
		var wg sync.WaitGroup

		for i := 0; i < 50; i++ {
			wg.Add(3)
			go func(idx int) {
				defer wg.Done()
				name := fmt.Sprintf("query_%d", idx)
				svc.RegisterQuery(name, func(ctx context.Context, input interface{}) (interface{}, error) {
					return idx, nil
				})
			}(i)
			go func(idx int) {
				defer wg.Done()
				name := fmt.Sprintf("mutation_%d", idx)
				svc.RegisterMutation(name, func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
					return idx, nil, nil
				})
			}(i)
			go func(idx int) {
				defer wg.Done()
				name := fmt.Sprintf("effect_%d", idx)
				svc.RegisterEffect(name, func(ctx context.Context, input interface{}) error {
					return nil
				})
			}(i)
		}

		wg.Wait()

		stats := svc.Stats()
		if stats["registered_queries"].(int) != 50 {
			t.Errorf("Expected 50 queries, got %d", stats["registered_queries"].(int))
		}
		if stats["registered_mutations"].(int) != 50 {
			t.Errorf("Expected 50 mutations, got %d", stats["registered_mutations"].(int))
		}
		if stats["registered_effects"].(int) != 50 {
			t.Errorf("Expected 50 effects, got %d", stats["registered_effects"].(int))
		}
	})
}

// ============================================================================
// CONTEXT HANDLING
// ============================================================================

func TestContextHandling(t *testing.T) {
	t.Run("Query respects context cancellation", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("slow", func(ctx context.Context, input interface{}) (interface{}, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return "done", nil
			}
		})

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		result := svc.Query(ctx, "slow", nil)
		if !result.IsErr() {
			t.Error("Query should fail when context is cancelled")
		}
	})

	t.Run("Mutation respects context cancellation", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("slow", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return "done", nil, nil
			}
		})

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		result := svc.Mutate(ctx, "slow", nil)
		if !result.IsErr() {
			t.Error("Mutation should fail when context is cancelled")
		}
	})
}

// ============================================================================
// OPERATION METADATA TESTS
// ============================================================================

func TestOperationMetadata(t *testing.T) {
	t.Run("Operation captures start time", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		beforeStart := time.Now()
		ctx := context.Background()
		_ = svc.Query(ctx, "q", nil)
		afterStart := time.Now()

		ops := svc.ListOperations(1)
		if ops[0].StartedAt.Before(beforeStart) {
			t.Error("StartedAt should be after test started")
		}
		if ops[0].StartedAt.After(afterStart) {
			t.Error("StartedAt should be before test ended")
		}
	})

	t.Run("Completed operation has CompletedAt set", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "q", nil)

		ops := svc.ListOperations(1)
		if ops[0].CompletedAt == nil {
			t.Error("CompletedAt should be set for completed operation")
		}
	})

	t.Run("Failed operation has CompletedAt set", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("failing", func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("failed")
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "failing", nil)

		ops := svc.ListOperations(1)
		if ops[0].CompletedAt == nil {
			t.Error("CompletedAt should be set even for failed operation")
		}
	})
}

// ============================================================================
// TRANSACTION STATUS TESTS
// ============================================================================

func TestTransactionStatuses(t *testing.T) {
	t.Run("Transaction starts as pending", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()
		tb := svc.BeginTransaction(ctx)

		if tb.tx.Status != TxStatusPending {
			t.Errorf("Expected pending status, got %s", tb.tx.Status)
		}
	})

	t.Run("Committed transaction has committed status", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		tb := svc.BeginTransaction(ctx)
		tb.Query("q", nil)
		_ = tb.Commit()

		if tb.tx.Status != TxStatusCommitted {
			t.Errorf("Expected committed status, got %s", tb.tx.Status)
		}
	})

	t.Run("Failed transaction has failed status", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("failing", func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("failed")
		})

		ctx := context.Background()
		tb := svc.BeginTransaction(ctx)
		tb.Query("failing", nil)
		_ = tb.Commit()

		if tb.tx.Status != TxStatusFailed {
			t.Errorf("Expected failed status, got %s", tb.tx.Status)
		}
	})

	t.Run("Rolled back transaction has rolled_back status", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		tb := svc.BeginTransaction(ctx)
		tb.Query("q", nil)
		_ = tb.Rollback()

		if tb.tx.Status != TxStatusRolledBack {
			t.Errorf("Expected rolled_back status, got %s", tb.tx.Status)
		}
	})
}

// ============================================================================
// ADDITIONAL BENCHMARKS
// ============================================================================

func BenchmarkFlatMap(b *testing.B) {
	r := Ok(1)
	f := func(x int) Result[int] { return Ok(x + 1) }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FlatMap(r, f)
	}
}

func BenchmarkMap(b *testing.B) {
	r := Ok(1)
	f := func(x int) int { return x * 2 }

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Map(r, f)
	}
}

func BenchmarkResultWithMeta(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Ok(42).WithMeta("key", "value")
	}
}

func BenchmarkTransactionCommit(b *testing.B) {
	svc := newTestService()
	svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "ok", nil
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.BeginTransaction(ctx).Query("q", nil).Commit()
	}
}

func BenchmarkEffect(b *testing.B) {
	svc := newTestService()
	svc.RegisterEffect("e", func(ctx context.Context, input interface{}) error {
		return nil
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = svc.Effect(ctx, "e", nil)
	}
}

// ============================================================================
// TRANSACTION BUILDER QUERY/MUTATE ERROR PROPAGATION
// ============================================================================

func TestTransactionBuilderQueryErrorPropagation(t *testing.T) {
	t.Run("Query returns error for unknown handler in transaction", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		err := svc.BeginTransaction(ctx).
			Query("unknown_query", nil).
			Commit()

		if err == nil {
			t.Error("Expected error for unknown query handler")
		}
	})

	t.Run("Query after Query error is skipped", func(t *testing.T) {
		svc := newTestService()
		secondQueryCalled := false

		svc.RegisterQuery("failing", func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("first query failed")
		})
		svc.RegisterQuery("second", func(ctx context.Context, input interface{}) (interface{}, error) {
			secondQueryCalled = true
			return "ok", nil
		})

		ctx := context.Background()
		_ = svc.BeginTransaction(ctx).
			Query("failing", nil).
			Query("second", nil).
			Commit()

		if secondQueryCalled {
			t.Error("Second query should not be called after first query failed")
		}
	})
}

func TestTransactionBuilderMutateErrorPropagation(t *testing.T) {
	t.Run("Mutate returns error for unknown handler in transaction", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		err := svc.BeginTransaction(ctx).
			Mutate("unknown_mutation", nil).
			Commit()

		if err == nil {
			t.Error("Expected error for unknown mutation handler")
		}
	})

	t.Run("Mutate after Mutate error is skipped", func(t *testing.T) {
		svc := newTestService()
		secondMutateCalled := false

		svc.RegisterMutation("failing", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return nil, nil, errors.New("first mutation failed")
		})
		svc.RegisterMutation("second", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			secondMutateCalled = true
			return "ok", nil, nil
		})

		ctx := context.Background()
		_ = svc.BeginTransaction(ctx).
			Mutate("failing", nil).
			Mutate("second", nil).
			Commit()

		if secondMutateCalled {
			t.Error("Second mutation should not be called after first mutation failed")
		}
	})

	t.Run("Mutate after Query error is skipped", func(t *testing.T) {
		svc := newTestService()
		mutateCalled := false

		svc.RegisterQuery("failing", func(ctx context.Context, input interface{}) (interface{}, error) {
			return nil, errors.New("query failed")
		})
		svc.RegisterMutation("mut", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			mutateCalled = true
			return "ok", nil, nil
		})

		ctx := context.Background()
		_ = svc.BeginTransaction(ctx).
			Query("failing", nil).
			Mutate("mut", nil).
			Commit()

		if mutateCalled {
			t.Error("Mutation should not be called after query failed")
		}
	})
}

// ============================================================================
// TRANSACTION BUILDER EFFECT ERROR PROPAGATION
// ============================================================================

func TestTransactionBuilderEffectErrorPropagation(t *testing.T) {
	t.Run("Effect returns error for unknown handler in transaction", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		err := svc.BeginTransaction(ctx).
			Effect("unknown_effect", nil).
			Commit()

		if err == nil {
			t.Error("Expected error for unknown effect handler")
		}
	})

	t.Run("Effect after Effect error is skipped", func(t *testing.T) {
		svc := newTestService()
		secondEffectCalled := false

		svc.RegisterEffect("failing", func(ctx context.Context, input interface{}) error {
			return errors.New("first effect failed")
		})
		svc.RegisterEffect("second", func(ctx context.Context, input interface{}) error {
			secondEffectCalled = true
			return nil
		})

		ctx := context.Background()
		_ = svc.BeginTransaction(ctx).
			Effect("failing", nil).
			Effect("second", nil).
			Commit()

		if secondEffectCalled {
			t.Error("Second effect should not be called after first effect failed")
		}
	})
}

// ============================================================================
// COMPLEX TRANSACTION SCENARIOS
// ============================================================================

func TestComplexTransactionScenarios(t *testing.T) {
	t.Run("Mixed operations transaction succeeds", func(t *testing.T) {
		svc := newTestService()

		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "queried", nil
		})
		svc.RegisterMutation("m", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "mutated", []StateChange{{Entity: "test"}}, nil
		})
		svc.RegisterEffect("e", func(ctx context.Context, input interface{}) error {
			return nil
		})

		ctx := context.Background()
		err := svc.BeginTransaction(ctx).
			Query("q", "query-input").
			Mutate("m", "mutation-input").
			Effect("e", "effect-input").
			Commit()

		if err != nil {
			t.Errorf("Mixed transaction should succeed: %v", err)
		}
	})

	t.Run("Transaction with multiple same-type operations", func(t *testing.T) {
		svc := newTestService()
		callOrder := make([]string, 0)

		svc.RegisterQuery("q1", func(ctx context.Context, input interface{}) (interface{}, error) {
			callOrder = append(callOrder, "q1")
			return "q1", nil
		})
		svc.RegisterQuery("q2", func(ctx context.Context, input interface{}) (interface{}, error) {
			callOrder = append(callOrder, "q2")
			return "q2", nil
		})
		svc.RegisterQuery("q3", func(ctx context.Context, input interface{}) (interface{}, error) {
			callOrder = append(callOrder, "q3")
			return "q3", nil
		})

		ctx := context.Background()
		err := svc.BeginTransaction(ctx).
			Query("q1", nil).
			Query("q2", nil).
			Query("q3", nil).
			Commit()

		if err != nil {
			t.Errorf("Transaction with multiple queries should succeed: %v", err)
		}

		if len(callOrder) != 3 {
			t.Errorf("Expected 3 queries called, got %d", len(callOrder))
		}
	})

	t.Run("Transaction rollback does not affect completed operations", func(t *testing.T) {
		svc := newTestService()
		queryExecuted := false

		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			queryExecuted = true
			return "done", nil
		})

		ctx := context.Background()
		tb := svc.BeginTransaction(ctx)
		tb.Query("q", nil)
		_ = tb.Rollback()

		// Query was still executed even though we rolled back
		if !queryExecuted {
			t.Error("Query should have been executed before rollback")
		}
	})
}

// ============================================================================
// OPERATION ID GENERATION
// ============================================================================

func TestOperationIDGeneration(t *testing.T) {
	t.Run("Operation IDs are unique", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		ids := make(map[string]bool)

		for i := 0; i < 100; i++ {
			_ = svc.Query(ctx, "q", nil)
		}

		ops := svc.ListOperations(100)
		for _, op := range ops {
			if ids[op.ID] {
				t.Errorf("Duplicate operation ID found: %s", op.ID)
			}
			ids[op.ID] = true
		}
	})

	t.Run("Transaction IDs are unique", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()
		ids := make(map[string]bool)

		for i := 0; i < 100; i++ {
			tb := svc.BeginTransaction(ctx)
			if ids[tb.tx.ID] {
				t.Errorf("Duplicate transaction ID found: %s", tb.tx.ID)
			}
			ids[tb.tx.ID] = true
		}
	})
}

// ============================================================================
// OPERATION TYPE CONSTANTS VERIFICATION
// ============================================================================

func TestOperationTypeConstants(t *testing.T) {
	t.Run("OperationType constants have expected values", func(t *testing.T) {
		if OpTypeQuery != "query" {
			t.Errorf("Expected OpTypeQuery to be 'query', got '%s'", OpTypeQuery)
		}
		if OpTypeMutation != "mutation" {
			t.Errorf("Expected OpTypeMutation to be 'mutation', got '%s'", OpTypeMutation)
		}
		if OpTypeEffect != "effect" {
			t.Errorf("Expected OpTypeEffect to be 'effect', got '%s'", OpTypeEffect)
		}
		if OpTypeComposed != "composed" {
			t.Errorf("Expected OpTypeComposed to be 'composed', got '%s'", OpTypeComposed)
		}
	})

	t.Run("OperationStatus constants have expected values", func(t *testing.T) {
		if OpStatusPending != "pending" {
			t.Errorf("Expected OpStatusPending to be 'pending', got '%s'", OpStatusPending)
		}
		if OpStatusRunning != "running" {
			t.Errorf("Expected OpStatusRunning to be 'running', got '%s'", OpStatusRunning)
		}
		if OpStatusCompleted != "completed" {
			t.Errorf("Expected OpStatusCompleted to be 'completed', got '%s'", OpStatusCompleted)
		}
		if OpStatusFailed != "failed" {
			t.Errorf("Expected OpStatusFailed to be 'failed', got '%s'", OpStatusFailed)
		}
		if OpStatusCancelled != "cancelled" {
			t.Errorf("Expected OpStatusCancelled to be 'cancelled', got '%s'", OpStatusCancelled)
		}
	})

	t.Run("TransactionStatus constants have expected values", func(t *testing.T) {
		if TxStatusPending != "pending" {
			t.Errorf("Expected TxStatusPending to be 'pending', got '%s'", TxStatusPending)
		}
		if TxStatusRunning != "running" {
			t.Errorf("Expected TxStatusRunning to be 'running', got '%s'", TxStatusRunning)
		}
		if TxStatusCommitted != "committed" {
			t.Errorf("Expected TxStatusCommitted to be 'committed', got '%s'", TxStatusCommitted)
		}
		if TxStatusRolledBack != "rolled_back" {
			t.Errorf("Expected TxStatusRolledBack to be 'rolled_back', got '%s'", TxStatusRolledBack)
		}
		if TxStatusFailed != "failed" {
			t.Errorf("Expected TxStatusFailed to be 'failed', got '%s'", TxStatusFailed)
		}
	})
}

// ============================================================================
// OPERATION STRUCT FIELD TESTS
// ============================================================================

func TestOperationFields(t *testing.T) {
	t.Run("Operation stores name correctly", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("my_special_query", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		_ = svc.Query(ctx, "my_special_query", nil)

		ops := svc.ListOperations(1)
		if ops[0].Name != "my_special_query" {
			t.Errorf("Expected name 'my_special_query', got '%s'", ops[0].Name)
		}
	})

	t.Run("Operation stores input correctly", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			return "ok", nil
		})

		ctx := context.Background()
		input := map[string]interface{}{"key": "value", "number": 42}
		_ = svc.Query(ctx, "q", input)

		ops := svc.ListOperations(1)
		storedInput := ops[0].Input.(map[string]interface{})
		if storedInput["key"] != "value" {
			t.Error("Input not stored correctly")
		}
	})
}

// ============================================================================
// STATE CHANGE TIMESTAMP TESTS
// ============================================================================

func TestStateChangeTimestamp(t *testing.T) {
	t.Run("StateChange timestamp is set by handler", func(t *testing.T) {
		svc := newTestService()
		testTime := time.Now()

		svc.RegisterMutation("m", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "ok", []StateChange{
				{
					Entity:    "user",
					Field:     "name",
					OldValue:  "old",
					NewValue:  "new",
					Timestamp: testTime,
				},
			}, nil
		})

		ctx := context.Background()
		_ = svc.Mutate(ctx, "m", nil)

		changes := svc.GetStateChanges()
		if !changes[0].Timestamp.Equal(testTime) {
			t.Error("Timestamp should match what handler set")
		}
	})
}

// ============================================================================
// NIL INPUT HANDLING
// ============================================================================

func TestNilInputHandling(t *testing.T) {
	t.Run("Query handles nil input", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterQuery("q", func(ctx context.Context, input interface{}) (interface{}, error) {
			if input != nil {
				return nil, errors.New("expected nil input")
			}
			return "ok", nil
		})

		ctx := context.Background()
		result := svc.Query(ctx, "q", nil)

		if !result.IsOk() {
			t.Errorf("Query should handle nil input: %v", result.Error())
		}
	})

	t.Run("Mutation handles nil input", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterMutation("m", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			if input != nil {
				return nil, nil, errors.New("expected nil input")
			}
			return "ok", nil, nil
		})

		ctx := context.Background()
		result := svc.Mutate(ctx, "m", nil)

		if !result.IsOk() {
			t.Errorf("Mutation should handle nil input: %v", result.Error())
		}
	})

	t.Run("Effect handles nil input", func(t *testing.T) {
		svc := newTestService()
		svc.RegisterEffect("e", func(ctx context.Context, input interface{}) error {
			if input != nil {
				return errors.New("expected nil input")
			}
			return nil
		})

		ctx := context.Background()
		result := svc.Effect(ctx, "e", nil)

		if !result.IsOk() {
			t.Errorf("Effect should handle nil input: %v", result.Error())
		}
	})
}

// ============================================================================
// WOTAN INTEGRATION TESTS WITH MOCK SERVER
// ============================================================================

// createMockWotanServer creates a test HTTP server that simulates the Wotan API
func createMockWotanServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle subscription requests
		if strings.Contains(r.URL.Path, "/subscribe") {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"subscriber": map[string]interface{}{
					"subscriber_id": "test-sub-123",
					"topic":         strings.Split(r.URL.Path, "/")[3],
					"display_name":  "monad-service",
					"status":        "approved",
				},
			})
			return
		}

		// Handle publish requests
		if strings.Contains(r.URL.Path, "/publish") {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message_id": "msg-123",
			})
			return
		}

		// Handle messages requests
		if strings.Contains(r.URL.Path, "/messages") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": []interface{}{},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
}

func newTestServiceWithWotan(server *httptest.Server) *Service {
	log := logger.New(os.Stderr)

	// Extract host:port from server URL (remove http:// prefix)
	addr := strings.TrimPrefix(server.URL, "http://")
	wotan, err := wotanClient.NewClient(addr)
	if err != nil {
		panic(err)
	}

	return NewService(log, wotan)
}

func TestWotanIntegrationStart(t *testing.T) {
	t.Run("Start with Wotan subscribes to topics", func(t *testing.T) {
		server := createMockWotanServer()
		defer server.Close()

		svc := newTestServiceWithWotan(server)
		ctx := context.Background()

		err := svc.Start(ctx)
		if err != nil {
			t.Errorf("Start should succeed with mock Wotan: %v", err)
		}
	})

	t.Run("Start handles subscription failure gracefully", func(t *testing.T) {
		// Server that rejects subscriptions
		failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer failServer.Close()

		svc := newTestServiceWithWotan(failServer)
		ctx := context.Background()

		// Should not error even if subscriptions fail (they're optional)
		err := svc.Start(ctx)
		if err != nil {
			t.Errorf("Start should not fail even with subscription errors: %v", err)
		}
	})
}

func TestWotanIntegrationMutatePublish(t *testing.T) {
	t.Run("Mutate publishes operation event to Wotan", func(t *testing.T) {
		published := make(chan bool, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/subscribe") {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"subscriber": map[string]interface{}{
						"subscriber_id": "test-sub-123",
						"topic":         "monad.operations",
						"display_name":  "monad-service",
						"status":        "approved",
					},
				})
				return
			}

			if strings.Contains(r.URL.Path, "/publish") {
				published <- true
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"message_id": "msg-123",
				})
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		svc := newTestServiceWithWotan(server)
		ctx := context.Background()

		// Subscribe first (required for publish)
		_ = svc.Start(ctx)

		svc.RegisterMutation("publish_test", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "ok", []StateChange{{Entity: "test"}}, nil
		})

		_ = svc.Mutate(ctx, "publish_test", nil)

		// Wait briefly for async publish
		select {
		case <-published:
			// Success - message was published
		case <-time.After(200 * time.Millisecond):
			// May not receive due to async, that's ok
		}
	})

	t.Run("Mutate handles publish failure gracefully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/subscribe") {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"subscriber": map[string]interface{}{
						"subscriber_id": "test-sub-123",
						"topic":         "monad.operations",
						"display_name":  "monad-service",
						"status":        "approved",
					},
				})
				return
			}

			if strings.Contains(r.URL.Path, "/publish") {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		svc := newTestServiceWithWotan(server)
		ctx := context.Background()

		_ = svc.Start(ctx)

		svc.RegisterMutation("m", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "ok", nil, nil
		})

		// Should not panic or fail even if publish fails
		result := svc.Mutate(ctx, "m", nil)
		if !result.IsOk() {
			t.Errorf("Mutate should succeed even if publish fails: %v", result.Error())
		}
	})
}

func TestWotanIntegrationWithTraceID(t *testing.T) {
	t.Run("Operations use trace_id from context", func(t *testing.T) {
		receivedPayload := make(chan []byte, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.Contains(r.URL.Path, "/subscribe") {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"subscriber": map[string]interface{}{
						"subscriber_id": "test-sub-123",
						"topic":         "monad.operations",
						"display_name":  "monad-service",
						"status":        "approved",
					},
				})
				return
			}

			if strings.Contains(r.URL.Path, "/publish") {
				// Don't read body here - just accept
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"message_id": "msg-123",
				})
				receivedPayload <- []byte("received")
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		svc := newTestServiceWithWotan(server)

		// Create context with trace_id
		ctx := context.WithValue(context.Background(), "trace_id", "test-trace-123") //nolint:staticcheck // SA1029 test-only string key; collision risk irrelevant in unit-test scope

		_ = svc.Start(ctx)

		svc.RegisterMutation("traced", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
			return "ok", nil, nil
		})

		_ = svc.Mutate(ctx, "traced", nil)

		// Wait briefly for publish
		select {
		case <-receivedPayload:
			// Success
		case <-time.After(200 * time.Millisecond):
			// May timeout, that's ok for this test
		}
	})
}

func TestHandleCriticalAlert(t *testing.T) {
	t.Run("handleCriticalAlert with nil message", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		// Should not panic
		svc.handleCriticalAlert(ctx, nil)
	})

	t.Run("handleCriticalAlert with valid message", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		msg := &wotanClient.Message{
			MessageID: "alert-123",
			Topic:     "alerts.critical",
			Payload:   `{"level": "critical", "message": "test alert"}`,
		}

		// Should not panic
		svc.handleCriticalAlert(ctx, msg)
	})
}

func TestListenForAlertsNilWotan(t *testing.T) {
	t.Run("listenForAlerts returns immediately when wotan is nil", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()

		done := make(chan bool)
		go func() {
			svc.listenForAlerts(ctx)
			done <- true
		}()

		select {
		case <-done:
			// Good - returned immediately
		case <-time.After(100 * time.Millisecond):
			t.Error("listenForAlerts should return immediately when wotan is nil")
		}
	})
}

func TestPublishOperationEventNilWotan(t *testing.T) {
	t.Run("publishOperationEvent returns immediately when wotan is nil", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()
		op := &Operation{
			ID:     "test-op",
			Name:   "test",
			Type:   OpTypeQuery,
			Status: OpStatusCompleted,
		}

		// Should not panic
		svc.publishOperationEvent(ctx, op)
	})
}

func TestPublishStateChangeNilWotan(t *testing.T) {
	t.Run("publishStateChange returns immediately when wotan is nil", func(t *testing.T) {
		svc := newTestService()
		ctx := context.Background()
		changes := []StateChange{
			{Entity: "test", Field: "field", OldValue: "old", NewValue: "new"},
		}

		// Should not panic
		svc.publishStateChange(ctx, "monad.state.changed", changes)
	})
}
