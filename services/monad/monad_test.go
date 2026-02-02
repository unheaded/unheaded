package monad

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
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
	if config.BusboyTopic != "monad.operations" {
		t.Errorf("Expected BusboyTopic 'monad.operations', got %s", config.BusboyTopic)
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
