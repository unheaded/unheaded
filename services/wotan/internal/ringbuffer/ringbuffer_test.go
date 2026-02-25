// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package ringbuffer

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Helper function to create a test message
func createTestMessage(roomID, content string) *Message {
	return &Message{
		ID:        uuid.New(),
		CreatorID: uuid.New(),
		RoomID:    roomID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// Helper function to create a test message with specific fields
func createTestMessageWithFields(id, creatorID uuid.UUID, roomID, content string, timestamp time.Time) *Message {
	return &Message{
		ID:        id,
		CreatorID: creatorID,
		RoomID:    roomID,
		Content:   content,
		Timestamp: timestamp,
	}
}

// TestNew verifies ring buffer initialization
func TestNew(t *testing.T) {
	tests := []struct {
		name         string
		size         int
		expectedSize int
	}{
		{
			name:         "valid size",
			size:         100,
			expectedSize: 100,
		},
		{
			name:         "zero size defaults to 1000",
			size:         0,
			expectedSize: 1000,
		},
		{
			name:         "negative size defaults to 1000",
			size:         -50,
			expectedSize: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := New(tt.size)
			if rb == nil {
				t.Fatal("New() returned nil")
			}
			if rb.Size() != tt.expectedSize {
				t.Errorf("Size() = %d, want %d", rb.Size(), tt.expectedSize)
			}
			if rb.Count() != 0 {
				t.Errorf("Count() = %d, want 0", rb.Count())
			}
			if rb.IsWrapping() {
				t.Error("IsWrapping() = true, want false")
			}
		})
	}
}

// TestPush_BasicOperations tests basic push functionality
func TestPush_BasicOperations(t *testing.T) {
	rb := New(5)

	// Push first message
	msg1 := createTestMessage("room1", "message1")
	id, overwritten := rb.Push(msg1)

	if id != msg1.ID {
		t.Errorf("Push() returned ID %v, want %v", id, msg1.ID)
	}
	if overwritten {
		t.Error("Push() overwritten = true, want false for first message")
	}
	if rb.Count() != 1 {
		t.Errorf("Count() = %d, want 1", rb.Count())
	}

	// Push until full
	for i := 2; i <= 5; i++ {
		msg := createTestMessage("room1", "message")
		_, overwritten := rb.Push(msg)
		if overwritten {
			t.Errorf("Push() overwritten = true for message %d, want false", i)
		}
	}

	if rb.Count() != 5 {
		t.Errorf("Count() = %d, want 5", rb.Count())
	}
	if rb.IsWrapping() {
		t.Error("IsWrapping() = true, want false before overwriting")
	}
}

// TestPush_Overwrite tests buffer overwrite behavior
func TestPush_Overwrite(t *testing.T) {
	rb := New(3)

	// Fill the buffer
	msg1 := createTestMessage("room1", "first")
	msg2 := createTestMessage("room1", "second")
	msg3 := createTestMessage("room1", "third")
	rb.Push(msg1)
	rb.Push(msg2)
	rb.Push(msg3)

	// Push one more to trigger overwrite
	msg4 := createTestMessage("room1", "fourth")
	_, overwritten := rb.Push(msg4)

	if !overwritten {
		t.Error("Push() overwritten = false, want true after filling buffer")
	}
	if !rb.IsWrapping() {
		t.Error("IsWrapping() = false, want true after overwriting")
	}
	if rb.Count() != 3 {
		t.Errorf("Count() = %d, want 3", rb.Count())
	}

	// Verify oldest message was overwritten
	messages := rb.Get()
	if messages[0].Content != "second" {
		t.Errorf("First message content = %s, want 'second' (oldest should be overwritten)", messages[0].Content)
	}
}

// TestGet_EmptyBuffer tests Get on empty buffer
func TestGet_EmptyBuffer(t *testing.T) {
	rb := New(10)
	messages := rb.Get()

	if messages == nil {
		t.Fatal("Get() returned nil, want empty slice")
	}
	if len(messages) != 0 {
		t.Errorf("Get() returned %d messages, want 0", len(messages))
	}
}

// TestGet_ChronologicalOrder tests message ordering
func TestGet_ChronologicalOrder(t *testing.T) {
	rb := New(5)

	// Create messages with specific timestamps
	baseTime := time.Now()
	messages := []*Message{
		createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "first", baseTime),
		createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "second", baseTime.Add(1*time.Second)),
		createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "third", baseTime.Add(2*time.Second)),
	}

	for _, msg := range messages {
		rb.Push(msg)
	}

	retrieved := rb.Get()
	if len(retrieved) != 3 {
		t.Fatalf("Get() returned %d messages, want 3", len(retrieved))
	}

	// Verify chronological order
	for i, msg := range retrieved {
		if msg.Content != messages[i].Content {
			t.Errorf("Message[%d].Content = %s, want %s", i, msg.Content, messages[i].Content)
		}
	}
}

// TestGet_AfterWrapping tests Get with wrapped buffer
func TestGet_AfterWrapping(t *testing.T) {
	rb := New(3)

	// Fill buffer and wrap around
	baseTime := time.Now()
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "first", baseTime))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "second", baseTime.Add(1*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "third", baseTime.Add(2*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "fourth", baseTime.Add(3*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "fifth", baseTime.Add(4*time.Second)))

	messages := rb.Get()
	if len(messages) != 3 {
		t.Fatalf("Get() returned %d messages, want 3", len(messages))
	}

	// Should contain second, third, fourth (first and fifth overwrote oldest)
	expectedContents := []string{"third", "fourth", "fifth"}
	for i, msg := range messages {
		if msg.Content != expectedContents[i] {
			t.Errorf("Message[%d].Content = %s, want %s", i, msg.Content, expectedContents[i])
		}
	}
}

// TestGetSince_EmptyBuffer tests GetSince on empty buffer
func TestGetSince_EmptyBuffer(t *testing.T) {
	rb := New(10)
	messages := rb.GetSince(time.Now())

	if messages == nil {
		t.Fatal("GetSince() returned nil, want empty slice")
	}
	if len(messages) != 0 {
		t.Errorf("GetSince() returned %d messages, want 0", len(messages))
	}
}

// TestGetSince_Filtering tests timestamp filtering
func TestGetSince_Filtering(t *testing.T) {
	rb := New(10)

	baseTime := time.Now()
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "old1", baseTime.Add(-10*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "old2", baseTime.Add(-5*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "new1", baseTime.Add(1*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "new2", baseTime.Add(2*time.Second)))

	messages := rb.GetSince(baseTime)

	if len(messages) != 2 {
		t.Fatalf("GetSince() returned %d messages, want 2", len(messages))
	}

	expectedContents := []string{"new1", "new2"}
	for i, msg := range messages {
		if msg.Content != expectedContents[i] {
			t.Errorf("Message[%d].Content = %s, want %s", i, msg.Content, expectedContents[i])
		}
	}
}

// TestGetSince_AfterWrapping tests GetSince with wrapped buffer
func TestGetSince_AfterWrapping(t *testing.T) {
	rb := New(3)

	baseTime := time.Now()
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "old1", baseTime.Add(-10*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "old2", baseTime.Add(-5*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "old3", baseTime.Add(-3*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "new1", baseTime.Add(1*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "new2", baseTime.Add(2*time.Second)))

	messages := rb.GetSince(baseTime)

	if len(messages) != 2 {
		t.Fatalf("GetSince() returned %d messages, want 2", len(messages))
	}

	for _, msg := range messages {
		if !msg.Timestamp.After(baseTime) {
			t.Errorf("Message with content %s has timestamp %v, want after %v", msg.Content, msg.Timestamp, baseTime)
		}
	}
}

// TestGetSince_NoMatchingMessages tests GetSince with no matches
func TestGetSince_NoMatchingMessages(t *testing.T) {
	rb := New(5)

	baseTime := time.Now()
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "old1", baseTime.Add(-10*time.Second)))
	rb.Push(createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "old2", baseTime.Add(-5*time.Second)))

	messages := rb.GetSince(baseTime.Add(1 * time.Second))

	if len(messages) != 0 {
		t.Errorf("GetSince() returned %d messages, want 0", len(messages))
	}
}

// TestDelete_EmptyBuffer tests Delete on empty buffer
func TestDelete_EmptyBuffer(t *testing.T) {
	rb := New(10)
	deleted := rb.Delete(uuid.New(), uuid.New(), "nonexistent")

	if deleted {
		t.Error("Delete() = true, want false for empty buffer")
	}
}

// TestDelete_NonExistent tests Delete with non-existent ID
func TestDelete_NonExistent(t *testing.T) {
	rb := New(10)
	msg := createTestMessage("room1", "test")
	rb.Push(msg)

	deleted := rb.Delete(uuid.New(), msg.CreatorID, msg.Content)

	if deleted {
		t.Error("Delete() = true, want false for non-existent ID")
	}
	if rb.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (message should not be deleted)", rb.Count())
	}
}

// TestDelete_WrongCreatorID tests Delete with wrong creator ID
func TestDelete_WrongCreatorID(t *testing.T) {
	rb := New(10)
	msg := createTestMessage("room1", "test")
	rb.Push(msg)

	deleted := rb.Delete(msg.ID, uuid.New(), msg.Content)

	if deleted {
		t.Error("Delete() = true, want false for wrong creator ID")
	}
	if rb.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (message should not be deleted)", rb.Count())
	}
}

// TestDelete_WrongContent tests Delete with wrong content
func TestDelete_WrongContent(t *testing.T) {
	rb := New(10)
	msg := createTestMessage("room1", "test")
	rb.Push(msg)

	deleted := rb.Delete(msg.ID, msg.CreatorID, "wrong content")

	if deleted {
		t.Error("Delete() = true, want false for wrong content")
	}
	if rb.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (message should not be deleted)", rb.Count())
	}
}

// TestDelete_Success tests successful deletion
func TestDelete_Success(t *testing.T) {
	rb := New(10)
	msg1 := createTestMessage("room1", "first")
	msg2 := createTestMessage("room1", "second")
	msg3 := createTestMessage("room1", "third")
	rb.Push(msg1)
	rb.Push(msg2)
	rb.Push(msg3)

	// Delete middle message
	deleted := rb.Delete(msg2.ID, msg2.CreatorID, msg2.Content)

	if !deleted {
		t.Error("Delete() = false, want true")
	}
	if rb.Count() != 2 {
		t.Errorf("Count() = %d, want 2", rb.Count())
	}

	// Verify remaining messages
	messages := rb.Get()
	if len(messages) != 2 {
		t.Fatalf("Get() returned %d messages, want 2", len(messages))
	}
	if messages[0].ID != msg1.ID || messages[1].ID != msg3.ID {
		t.Error("Remaining messages are not first and third")
	}
}

// TestDelete_FromWrappedBuffer tests deletion after wrapping
func TestDelete_FromWrappedBuffer(t *testing.T) {
	rb := New(3)

	msg1 := createTestMessage("room1", "first")
	msg2 := createTestMessage("room1", "second")
	msg3 := createTestMessage("room1", "third")
	msg4 := createTestMessage("room1", "fourth")

	rb.Push(msg1)
	rb.Push(msg2)
	rb.Push(msg3)
	rb.Push(msg4) // Overwrites msg1

	if !rb.IsWrapping() {
		t.Fatal("Buffer should be wrapping")
	}

	// Delete msg3 (oldest remaining message)
	deleted := rb.Delete(msg3.ID, msg3.CreatorID, msg3.Content)

	if !deleted {
		t.Error("Delete() = false, want true")
	}
	if rb.Count() != 2 {
		t.Errorf("Count() = %d, want 2", rb.Count())
	}

	// Verify wrapping flag reset
	if rb.IsWrapping() {
		t.Error("IsWrapping() = true, want false after deletion brings count below size")
	}
}

// TestDelete_ResetsWrappingFlag tests that wrapping flag resets appropriately
func TestDelete_ResetsWrappingFlag(t *testing.T) {
	rb := New(3)

	// Fill and wrap
	rb.Push(createTestMessage("room1", "m1"))
	rb.Push(createTestMessage("room1", "m2"))
	rb.Push(createTestMessage("room1", "m3"))
	msg4 := createTestMessage("room1", "m4")
	rb.Push(msg4)

	if !rb.IsWrapping() {
		t.Fatal("Buffer should be wrapping before deletion")
	}

	// Delete one message
	deleted := rb.Delete(msg4.ID, msg4.CreatorID, msg4.Content)
	if !deleted {
		t.Fatal("Delete() failed")
	}

	if rb.IsWrapping() {
		t.Error("IsWrapping() = true, want false after deleting from wrapped buffer")
	}
}

// TestGetByID_EmptyBuffer tests GetByID on empty buffer
func TestGetByID_EmptyBuffer(t *testing.T) {
	rb := New(10)
	msg := rb.GetByID(uuid.New())

	if msg != nil {
		t.Error("GetByID() returned non-nil, want nil for empty buffer")
	}
}

// TestGetByID_Found tests successful retrieval by ID
func TestGetByID_Found(t *testing.T) {
	rb := New(10)
	msg1 := createTestMessage("room1", "first")
	msg2 := createTestMessage("room1", "second")
	rb.Push(msg1)
	rb.Push(msg2)

	retrieved := rb.GetByID(msg2.ID)

	if retrieved == nil {
		t.Fatal("GetByID() returned nil, want message")
	}
	if retrieved.ID != msg2.ID {
		t.Errorf("GetByID() returned message with ID %v, want %v", retrieved.ID, msg2.ID)
	}
	if retrieved.Content != msg2.Content {
		t.Errorf("GetByID() returned message with content %s, want %s", retrieved.Content, msg2.Content)
	}
}

// TestGetByID_NotFound tests GetByID with non-existent ID
func TestGetByID_NotFound(t *testing.T) {
	rb := New(10)
	rb.Push(createTestMessage("room1", "test"))

	retrieved := rb.GetByID(uuid.New())

	if retrieved != nil {
		t.Error("GetByID() returned non-nil, want nil for non-existent ID")
	}
}

// TestGetByID_AfterWrapping tests GetByID with wrapped buffer
func TestGetByID_AfterWrapping(t *testing.T) {
	rb := New(3)

	msg1 := createTestMessage("room1", "first")
	msg2 := createTestMessage("room1", "second")
	msg3 := createTestMessage("room1", "third")
	msg4 := createTestMessage("room1", "fourth")

	rb.Push(msg1)
	rb.Push(msg2)
	rb.Push(msg3)
	rb.Push(msg4) // Overwrites msg1

	// msg1 should not be found (overwritten)
	retrieved := rb.GetByID(msg1.ID)
	if retrieved != nil {
		t.Error("GetByID() found overwritten message")
	}

	// msg2, msg3, msg4 should be found
	for _, msg := range []*Message{msg2, msg3, msg4} {
		retrieved := rb.GetByID(msg.ID)
		if retrieved == nil {
			t.Errorf("GetByID() returned nil for message %s", msg.Content)
		} else if retrieved.ID != msg.ID {
			t.Errorf("GetByID() returned wrong message")
		}
	}
}

// TestCount_Accuracy tests Count accuracy through operations
func TestCount_Accuracy(t *testing.T) {
	rb := New(5)

	if rb.Count() != 0 {
		t.Errorf("Initial Count() = %d, want 0", rb.Count())
	}

	// Add messages
	msg1 := createTestMessage("room1", "m1")
	rb.Push(msg1)
	if rb.Count() != 1 {
		t.Errorf("Count() after 1 push = %d, want 1", rb.Count())
	}

	rb.Push(createTestMessage("room1", "m2"))
	rb.Push(createTestMessage("room1", "m3"))
	if rb.Count() != 3 {
		t.Errorf("Count() after 3 pushes = %d, want 3", rb.Count())
	}

	// Delete a message
	rb.Delete(msg1.ID, msg1.CreatorID, msg1.Content)
	if rb.Count() != 2 {
		t.Errorf("Count() after delete = %d, want 2", rb.Count())
	}

	// Fill and wrap
	rb.Push(createTestMessage("room1", "m4"))
	rb.Push(createTestMessage("room1", "m5"))
	rb.Push(createTestMessage("room1", "m6"))
	rb.Push(createTestMessage("room1", "m7"))
	if rb.Count() != 5 {
		t.Errorf("Count() after wrapping = %d, want 5", rb.Count())
	}
}

// TestSize_Immutable tests that Size never changes
func TestSize_Immutable(t *testing.T) {
	rb := New(5)
	initialSize := rb.Size()

	rb.Push(createTestMessage("room1", "m1"))
	if rb.Size() != initialSize {
		t.Errorf("Size() changed after push")
	}

	rb.Push(createTestMessage("room1", "m2"))
	rb.Push(createTestMessage("room1", "m3"))
	rb.Push(createTestMessage("room1", "m4"))
	rb.Push(createTestMessage("room1", "m5"))
	rb.Push(createTestMessage("room1", "m6")) // Wrap
	if rb.Size() != initialSize {
		t.Errorf("Size() changed after wrapping")
	}
}

// TestIsWrapping_Transitions tests wrapping flag transitions
func TestIsWrapping_Transitions(t *testing.T) {
	rb := New(3)

	if rb.IsWrapping() {
		t.Error("IsWrapping() = true initially, want false")
	}

	rb.Push(createTestMessage("room1", "m1"))
	rb.Push(createTestMessage("room1", "m2"))
	if rb.IsWrapping() {
		t.Error("IsWrapping() = true before full, want false")
	}

	rb.Push(createTestMessage("room1", "m3"))
	if rb.IsWrapping() {
		t.Error("IsWrapping() = true when full (not overwritten yet), want false")
	}

	msg4 := createTestMessage("room1", "m4")
	rb.Push(msg4) // First overwrite
	if !rb.IsWrapping() {
		t.Error("IsWrapping() = false after overwrite, want true")
	}

	// Delete to bring count below size
	rb.Delete(msg4.ID, msg4.CreatorID, msg4.Content)
	if rb.IsWrapping() {
		t.Error("IsWrapping() = true after deletion, want false")
	}
}

// TestConcurrent_PushAndGet tests concurrent push and get operations
func TestConcurrent_PushAndGet(t *testing.T) {
	rb := New(100)
	var wg sync.WaitGroup
	pushCount := 50
	getCount := 50

	// Concurrent pushes
	for i := 0; i < pushCount; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			msg := createTestMessage("room1", "concurrent")
			rb.Push(msg)
		}(i)
	}

	// Concurrent gets
	for i := 0; i < getCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.Get()
		}()
	}

	wg.Wait()

	if rb.Count() != pushCount {
		t.Errorf("Count() = %d, want %d after concurrent operations", rb.Count(), pushCount)
	}
}

// TestConcurrent_PushAndDelete tests concurrent push and delete operations
func TestConcurrent_PushAndDelete(t *testing.T) {
	rb := New(100)
	var wg sync.WaitGroup

	// Push initial messages
	messages := make([]*Message, 20)
	for i := 0; i < 20; i++ {
		messages[i] = createTestMessage("room1", "test")
		rb.Push(messages[i])
	}

	// Concurrent deletes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(msg *Message) {
			defer wg.Done()
			rb.Delete(msg.ID, msg.CreatorID, msg.Content)
		}(messages[i])
	}

	// Concurrent pushes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := createTestMessage("room1", "new")
			rb.Push(msg)
		}()
	}

	wg.Wait()

	if rb.Count() != 20 {
		t.Errorf("Count() = %d, want 20 after concurrent push/delete", rb.Count())
	}
}

// TestConcurrent_GetByIDAndPush tests concurrent GetByID and Push operations
func TestConcurrent_GetByIDAndPush(t *testing.T) {
	rb := New(100)
	var wg sync.WaitGroup

	// Push initial message
	targetMsg := createTestMessage("room1", "target")
	rb.Push(targetMsg)

	// Concurrent GetByID calls
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.GetByID(targetMsg.ID)
		}()
	}

	// Concurrent Push calls
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := createTestMessage("room1", "concurrent")
			rb.Push(msg)
		}()
	}

	wg.Wait()

	// Verify target message still exists (buffer is large enough)
	retrieved := rb.GetByID(targetMsg.ID)
	if retrieved == nil {
		t.Error("Target message was lost during concurrent operations")
	}
}

// TestConcurrent_GetSinceAndPush tests concurrent GetSince and Push operations
func TestConcurrent_GetSinceAndPush(t *testing.T) {
	rb := New(100)
	var wg sync.WaitGroup
	timestamp := time.Now()

	// Concurrent GetSince calls
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.GetSince(timestamp)
		}()
	}

	// Concurrent Push calls with timestamps after threshold
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := createTestMessageWithFields(uuid.New(), uuid.New(), "room1", "new", timestamp.Add(1*time.Second))
			rb.Push(msg)
		}()
	}

	wg.Wait()

	messages := rb.GetSince(timestamp)
	if len(messages) != 50 {
		t.Errorf("GetSince() returned %d messages, want 50", len(messages))
	}
}

// TestConcurrent_MultipleOperations tests various operations concurrently
func TestConcurrent_MultipleOperations(t *testing.T) {
	rb := New(50)
	var wg sync.WaitGroup

	// Push initial messages
	initialMessages := make([]*Message, 10)
	for i := 0; i < 10; i++ {
		initialMessages[i] = createTestMessage("room1", "initial")
		rb.Push(initialMessages[i])
	}

	operations := 100

	// Mix of operations
	for i := 0; i < operations; i++ {
		// Push
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := createTestMessage("room1", "concurrent")
			rb.Push(msg)
		}()

		// Get
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.Get()
		}()

		// Count
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.Count()
		}()

		// IsWrapping
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.IsWrapping()
		}()
	}

	wg.Wait()

	// Verify buffer is in valid state
	count := rb.Count()
	if count < 0 || count > rb.Size() {
		t.Errorf("Count() = %d, want between 0 and %d", count, rb.Size())
	}

	messages := rb.Get()
	if len(messages) != count {
		t.Errorf("Get() returned %d messages, Count() = %d (mismatch)", len(messages), count)
	}
}

// TestWrappingBehavior_EdgeCases tests edge cases in wrapping behavior
func TestWrappingBehavior_EdgeCases(t *testing.T) {
	rb := New(2)

	msg1 := createTestMessage("room1", "first")
	msg2 := createTestMessage("room1", "second")
	msg3 := createTestMessage("room1", "third")

	rb.Push(msg1)
	rb.Push(msg2)

	// At this point: buffer full, not wrapping
	if rb.IsWrapping() {
		t.Error("IsWrapping() = true when full but not overwritten, want false")
	}

	rb.Push(msg3)

	// Now wrapping
	if !rb.IsWrapping() {
		t.Error("IsWrapping() = false after overwrite, want true")
	}

	// Verify oldest (msg1) was removed
	retrieved := rb.GetByID(msg1.ID)
	if retrieved != nil {
		t.Error("Oldest message still exists after overwrite")
	}

	// Verify msg2 and msg3 exist
	if rb.GetByID(msg2.ID) == nil {
		t.Error("Second message not found after wrap")
	}
	if rb.GetByID(msg3.ID) == nil {
		t.Error("Third message not found after wrap")
	}
}

// TestDelete_OrderPreservation tests that deletion preserves order
func TestDelete_OrderPreservation(t *testing.T) {
	rb := New(10)

	msg1 := createTestMessage("room1", "first")
	msg2 := createTestMessage("room1", "second")
	msg3 := createTestMessage("room1", "third")
	msg4 := createTestMessage("room1", "fourth")

	rb.Push(msg1)
	rb.Push(msg2)
	rb.Push(msg3)
	rb.Push(msg4)

	// Delete second message
	rb.Delete(msg2.ID, msg2.CreatorID, msg2.Content)

	messages := rb.Get()
	expectedContents := []string{"first", "third", "fourth"}

	if len(messages) != len(expectedContents) {
		t.Fatalf("Get() returned %d messages, want %d", len(messages), len(expectedContents))
	}

	for i, msg := range messages {
		if msg.Content != expectedContents[i] {
			t.Errorf("Message[%d].Content = %s, want %s", i, msg.Content, expectedContents[i])
		}
	}
}

// ============================================================================
// BENCHMARKS
// ============================================================================

// BenchmarkPush measures push performance
func BenchmarkPush(b *testing.B) {
	rb := New(10000)
	msg := createTestMessage("room1", "benchmark message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(msg)
	}
}

// BenchmarkPush_Wrapping measures push performance when buffer wraps
func BenchmarkPush_Wrapping(b *testing.B) {
	rb := New(100) // Small buffer to force wrapping
	msg := createTestMessage("room1", "benchmark message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(msg)
	}
}

// BenchmarkGet measures get performance
func BenchmarkGet(b *testing.B) {
	rb := New(1000)
	// Pre-populate buffer
	for i := 0; i < 1000; i++ {
		rb.Push(createTestMessage("room1", "message"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.Get()
	}
}

// BenchmarkGet_Empty measures get performance on empty buffer
func BenchmarkGet_Empty(b *testing.B) {
	rb := New(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.Get()
	}
}

// BenchmarkGetSince measures GetSince performance
func BenchmarkGetSince(b *testing.B) {
	rb := New(1000)
	// Pre-populate buffer
	for i := 0; i < 1000; i++ {
		rb.Push(createTestMessage("room1", "message"))
	}
	since := time.Now().Add(-1 * time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.GetSince(since)
	}
}

// BenchmarkGetByID measures GetByID performance
func BenchmarkGetByID(b *testing.B) {
	rb := New(1000)
	// Pre-populate and save IDs
	ids := make([]uuid.UUID, 1000)
	for i := 0; i < 1000; i++ {
		msg := createTestMessage("room1", "message")
		ids[i] = msg.ID
		rb.Push(msg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.GetByID(ids[i%1000])
	}
}

// BenchmarkDelete measures delete performance
func BenchmarkDelete(b *testing.B) {
	rb := New(10000)
	creatorID := uuid.New()
	// Pre-populate buffer
	for i := 0; i < 10000; i++ {
		msg := &Message{
			ID:        uuid.New(),
			CreatorID: creatorID,
			RoomID:    "room1",
			Content:   "message",
			Timestamp: time.Now(),
		}
		rb.Push(msg)
	}
	messages := rb.Get()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := i % len(messages)
		rb.Delete(messages[idx].ID, messages[idx].CreatorID, messages[idx].Content)
	}
}

// BenchmarkConcurrentPush measures concurrent push performance
func BenchmarkConcurrentPush(b *testing.B) {
	rb := New(10000)
	msg := createTestMessage("room1", "benchmark message")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rb.Push(msg)
		}
	})
}

// BenchmarkConcurrentPushGet measures concurrent push and get performance
func BenchmarkConcurrentPushGet(b *testing.B) {
	rb := New(10000)
	// Pre-populate
	for i := 0; i < 1000; i++ {
		rb.Push(createTestMessage("room1", "initial"))
	}
	msg := createTestMessage("room1", "benchmark message")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rb.Push(msg)
			_ = rb.Get()
		}
	})
}

// BenchmarkCount measures count performance
func BenchmarkCount(b *testing.B) {
	rb := New(1000)
	for i := 0; i < 500; i++ {
		rb.Push(createTestMessage("room1", "message"))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rb.Count()
	}
}
