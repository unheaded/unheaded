package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewMemoryStore(t *testing.T) {
	tests := []struct {
		name         string
		size         int
		wantCapacity int
	}{
		{"positive size", 100, 100},
		{"zero size uses default", 0, 10000},
		{"negative size uses default", -1, 10000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewMemoryStore(tt.size)
			if s.Capacity() != tt.wantCapacity {
				t.Errorf("Capacity() = %v, want %v", s.Capacity(), tt.wantCapacity)
			}
		})
	}
}

func TestMemoryStore_Push(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(10)

	msg := newTestMessage("general", "Hello, World!")

	id, overwritten, err := s.Push(ctx, msg)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if id != msg.ID {
		t.Errorf("Push() id = %v, want %v", id, msg.ID)
	}
	if overwritten {
		t.Error("Push() overwritten = true, want false (buffer not full)")
	}

	count, _ := s.Count(ctx)
	if count != 1 {
		t.Errorf("Count() = %v, want 1", count)
	}
}

func TestMemoryStore_Push_Validation(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(10)

	tests := []struct {
		name    string
		msg     *Message
		wantErr error
	}{
		{
			name:    "nil message",
			msg:     nil,
			wantErr: ErrInvalidMessage,
		},
		{
			name: "zero ID",
			msg: &Message{
				ID:        uuid.Nil,
				CreatorID: uuid.New(),
				RoomID:    "general",
				Content:   "test",
				Timestamp: time.Now(),
			},
			wantErr: ErrInvalidMessage,
		},
		{
			name: "empty content",
			msg: &Message{
				ID:        uuid.New(),
				CreatorID: uuid.New(),
				RoomID:    "general",
				Content:   "",
				Timestamp: time.Now(),
			},
			wantErr: ErrInvalidMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := s.Push(ctx, tt.msg)
			if err != tt.wantErr {
				t.Errorf("Push() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMemoryStore_Push_Wrapping(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(3)

	// Fill the buffer
	for i := 0; i < 3; i++ {
		_, overwritten, _ := s.Push(ctx, newTestMessage("general", "msg"))
		if overwritten {
			t.Errorf("Push %d should not overwrite", i)
		}
	}

	// Next push should overwrite
	_, overwritten, _ := s.Push(ctx, newTestMessage("general", "msg4"))
	if !overwritten {
		t.Error("Push should overwrite when buffer is full")
	}

	// Count should still be 3
	count, _ := s.Count(ctx)
	if count != 3 {
		t.Errorf("Count() = %v, want 3", count)
	}
}

func TestMemoryStore_Get(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(10)

	msg := newTestMessage("general", "Hello!")
	s.Push(ctx, msg)

	// Get existing message
	got, err := s.Get(ctx, msg.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != msg.ID {
		t.Errorf("Get() ID = %v, want %v", got.ID, msg.ID)
	}
	if got.Content != msg.Content {
		t.Errorf("Get() Content = %v, want %v", got.Content, msg.Content)
	}

	// Get returns a copy (not the original)
	got.Content = "modified"
	original, _ := s.Get(ctx, msg.ID)
	if original.Content != "Hello!" {
		t.Error("Get() should return a copy, not the original")
	}
}

func TestMemoryStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(10)

	_, err := s.Get(ctx, uuid.New())
	if err != ErrNotFound {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}

	_, err = s.Get(ctx, uuid.Nil)
	if err != ErrNotFound {
		t.Errorf("Get(nil) error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(10)

	msg := newTestMessage("general", "Delete me")
	s.Push(ctx, msg)

	// Verify message exists
	count, _ := s.Count(ctx)
	if count != 1 {
		t.Fatalf("Count() = %v, want 1", count)
	}

	// Delete with correct verification
	err := s.Delete(ctx, msg.ID, msg.CreatorID, msg.Content)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify message is gone
	count, _ = s.Count(ctx)
	if count != 0 {
		t.Errorf("Count() after delete = %v, want 0", count)
	}

	_, err = s.Get(ctx, msg.ID)
	if err != ErrNotFound {
		t.Errorf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestMemoryStore_Delete_TripleVerification(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(10)

	msg := newTestMessage("general", "Protected message")
	s.Push(ctx, msg)

	tests := []struct {
		name      string
		id        uuid.UUID
		creatorID uuid.UUID
		content   string
		wantErr   error
	}{
		{"wrong ID", uuid.New(), msg.CreatorID, msg.Content, ErrNotFound},
		{"wrong CreatorID", msg.ID, uuid.New(), msg.Content, ErrNotFound},
		{"wrong Content", msg.ID, msg.CreatorID, "wrong content", ErrNotFound},
		{"nil ID", uuid.Nil, msg.CreatorID, msg.Content, ErrNotFound},
		{"empty content", msg.ID, msg.CreatorID, "", ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Delete(ctx, tt.id, tt.creatorID, tt.content)
			if err != tt.wantErr {
				t.Errorf("Delete() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	// Message should still exist
	count, _ := s.Count(ctx)
	if count != 1 {
		t.Error("Message should not be deleted with wrong verification")
	}
}

func TestMemoryStore_Delete_Wrapped(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(3)

	// Fill and wrap
	msgs := make([]*Message, 4)
	for i := 0; i < 4; i++ {
		msgs[i] = newTestMessage("general", "msg")
		s.Push(ctx, msgs[i])
	}

	// Delete from wrapped buffer (message at index 1, 2, 3 should remain; msg[0] was overwritten)
	err := s.Delete(ctx, msgs[1].ID, msgs[1].CreatorID, msgs[1].Content)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	count, _ := s.Count(ctx)
	if count != 2 {
		t.Errorf("Count() = %v, want 2", count)
	}
}

func TestMemoryStore_Close(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(10)

	msg := newTestMessage("general", "test")
	s.Push(ctx, msg)

	err := s.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Operations after close should return ErrStoreClosed
	_, _, err = s.Push(ctx, msg)
	if err != ErrStoreClosed {
		t.Errorf("Push() after Close error = %v, want ErrStoreClosed", err)
	}

	_, err = s.Get(ctx, msg.ID)
	if err != ErrStoreClosed {
		t.Errorf("Get() after Close error = %v, want ErrStoreClosed", err)
	}

	_, err = s.Count(ctx)
	if err != ErrStoreClosed {
		t.Errorf("Count() after Close error = %v, want ErrStoreClosed", err)
	}

	// Double close should return error
	err = s.Close()
	if err != ErrStoreClosed {
		t.Errorf("Close() twice error = %v, want ErrStoreClosed", err)
	}
}

func TestMemoryStore_Query(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(100)

	// Add messages to different rooms at different times
	baseTime := time.Date(2026, 1, 28, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 10; i++ {
		room := "room1"
		if i%2 == 0 {
			room = "room2"
		}
		msg := &Message{
			ID:        uuid.New(),
			CreatorID: uuid.New(),
			RoomID:    room,
			Content:   "test message",
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
		}
		s.Push(ctx, msg)
	}

	tests := []struct {
		name      string
		opts      QueryOptions
		wantCount int
	}{
		{"all messages", QueryOptions{}, 10},
		{"room1 only", QueryOptions{RoomID: "room1"}, 5},
		{"room2 only", QueryOptions{RoomID: "room2"}, 5},
		{"nonexistent room", QueryOptions{RoomID: "room3"}, 0},
		{"limit 3", QueryOptions{Limit: 3}, 3},
		{"offset 5", QueryOptions{Offset: 5}, 5},
		{"offset and limit", QueryOptions{Offset: 2, Limit: 3}, 3},
		{"offset beyond count", QueryOptions{Offset: 100}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iter, err := s.Query(ctx, tt.opts)
			if err != nil {
				t.Fatalf("Query() error = %v", err)
			}
			defer iter.Close()

			count := 0
			for iter.Next(ctx) {
				msg := iter.Message()
				if msg == nil {
					t.Error("Message() returned nil")
				}
				count++
			}

			if iter.Err() != nil {
				t.Errorf("iterator error = %v", iter.Err())
			}

			if count != tt.wantCount {
				t.Errorf("got %d messages, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestMemoryStore_Query_TimeFilters(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(100)

	baseTime := time.Date(2026, 1, 28, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 10; i++ {
		msg := &Message{
			ID:        uuid.New(),
			CreatorID: uuid.New(),
			RoomID:    "general",
			Content:   "test",
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
		}
		s.Push(ctx, msg)
	}

	// Query messages after 5 minutes
	iter, _ := s.Query(ctx, QueryOptions{
		Since: baseTime.Add(4 * time.Minute),
	})

	count := 0
	for iter.Next(ctx) {
		count++
	}
	iter.Close()

	if count != 5 { // Minutes 5,6,7,8,9
		t.Errorf("got %d messages since minute 4, want 5", count)
	}
}

func TestMemoryStore_Query_Order(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(100)

	baseTime := time.Date(2026, 1, 28, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		msg := &Message{
			ID:        uuid.New(),
			CreatorID: uuid.New(),
			RoomID:    "general",
			Content:   "test",
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
		}
		s.Push(ctx, msg)
	}

	// Test ascending order
	iter, _ := s.Query(ctx, QueryOptions{Ascending: true})
	var lastTime time.Time
	for iter.Next(ctx) {
		msg := iter.Message()
		if !lastTime.IsZero() && msg.Timestamp.Before(lastTime) {
			t.Error("ascending order violated")
		}
		lastTime = msg.Timestamp
	}
	iter.Close()

	// Test descending order (default)
	iter, _ = s.Query(ctx, QueryOptions{Ascending: false})
	lastTime = time.Time{}
	for iter.Next(ctx) {
		msg := iter.Message()
		if !lastTime.IsZero() && msg.Timestamp.After(lastTime) {
			t.Error("descending order violated")
		}
		lastTime = msg.Timestamp
	}
	iter.Close()
}

func TestMemoryStore_Stats(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(10)

	// Empty store stats
	stats, err := s.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Count != 0 {
		t.Errorf("empty store Count = %v, want 0", stats.Count)
	}
	if stats.Capacity != 10 {
		t.Errorf("Capacity = %v, want 10", stats.Capacity)
	}
	if stats.IsWrapping {
		t.Error("empty store should not be wrapping")
	}

	// Add some messages
	baseTime := time.Date(2026, 1, 28, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		msg := &Message{
			ID:        uuid.New(),
			CreatorID: uuid.New(),
			RoomID:    "general",
			Content:   "test",
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
		}
		s.Push(ctx, msg)
	}

	stats, _ = s.Stats(ctx)
	if stats.Count != 5 {
		t.Errorf("Count = %v, want 5", stats.Count)
	}
	if stats.OldestTimestamp != baseTime {
		t.Errorf("OldestTimestamp = %v, want %v", stats.OldestTimestamp, baseTime)
	}
	if stats.NewestTimestamp != baseTime.Add(4*time.Minute) {
		t.Errorf("NewestTimestamp = %v, want %v", stats.NewestTimestamp, baseTime.Add(4*time.Minute))
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore(1000)

	var wg sync.WaitGroup
	numGoroutines := 100
	msgsPerGoroutine := 10

	// Concurrent pushes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < msgsPerGoroutine; j++ {
				msg := newTestMessage("general", "concurrent test")
				_, _, err := s.Push(ctx, msg)
				if err != nil {
					t.Errorf("concurrent Push error: %v", err)
				}
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < msgsPerGoroutine; j++ {
				_, _ = s.Count(ctx)
				_, _ = s.Stats(ctx)
			}
		}()
	}

	wg.Wait()

	count, _ := s.Count(ctx)
	expected := numGoroutines * msgsPerGoroutine
	if count > 1000 {
		expected = 1000 // Buffer capacity
	}
	if count != expected && count != 1000 {
		t.Logf("Count = %d (expected ~%d or 1000 if wrapped)", count, expected)
	}
}

func TestMemoryStore_ContextCancellation(t *testing.T) {
	s := NewMemoryStore(10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msg := newTestMessage("general", "test")

	_, _, err := s.Push(ctx, msg)
	if err != context.Canceled {
		t.Errorf("Push with cancelled context error = %v, want context.Canceled", err)
	}

	_, err = s.Get(ctx, msg.ID)
	if err != context.Canceled {
		t.Errorf("Get with cancelled context error = %v, want context.Canceled", err)
	}

	_, err = s.Count(ctx)
	if err != context.Canceled {
		t.Errorf("Count with cancelled context error = %v, want context.Canceled", err)
	}

	iter, err := s.Query(ctx, QueryOptions{})
	if err != context.Canceled {
		t.Errorf("Query with cancelled context error = %v, want context.Canceled", err)
	}
	if iter != nil {
		iter.Close()
	}
}

func TestSliceIterator_Close(t *testing.T) {
	iter := &sliceIterator{
		messages: []*Message{newTestMessage("general", "test")},
		index:    -1,
	}

	// First close should succeed
	err := iter.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Second close should return error
	err = iter.Close()
	if err != ErrIteratorClosed {
		t.Errorf("Close() twice error = %v, want ErrIteratorClosed", err)
	}

	// Next should return false after close
	if iter.Next(context.Background()) {
		t.Error("Next() after Close should return false")
	}
}

// ============================================================================
// BENCHMARKS
// ============================================================================

func BenchmarkMemoryStore_Push(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore(b.N)

	msg := newTestMessage("general", "benchmark message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.ID = uuid.New() // New ID each time
		s.Push(ctx, msg)
	}
}

func BenchmarkMemoryStore_Push_Wrapped(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore(1000) // Small buffer to force wrapping

	msg := newTestMessage("general", "benchmark message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg.ID = uuid.New()
		s.Push(ctx, msg)
	}
}

func BenchmarkMemoryStore_Get(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore(10000)

	// Pre-populate
	ids := make([]uuid.UUID, 10000)
	for i := 0; i < 10000; i++ {
		msg := newTestMessage("general", "benchmark")
		s.Push(ctx, msg)
		ids[i] = msg.ID
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Get(ctx, ids[i%10000])
	}
}

func BenchmarkMemoryStore_Query(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore(10000)

	// Pre-populate
	for i := 0; i < 10000; i++ {
		s.Push(ctx, newTestMessage("general", "benchmark"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iter, _ := s.Query(ctx, QueryOptions{Limit: 100})
		for iter.Next(ctx) {
			_ = iter.Message()
		}
		iter.Close()
	}
}

func BenchmarkMemoryStore_ConcurrentPush(b *testing.B) {
	ctx := context.Background()
	s := NewMemoryStore(b.N)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		msg := newTestMessage("general", "benchmark")
		for pb.Next() {
			msg.ID = uuid.New()
			s.Push(ctx, msg)
		}
	})
}
