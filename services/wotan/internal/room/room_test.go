// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package room

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestNewManager verifies manager initialization
func TestNewManager(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
	}{
		{
			name:       "standard buffer size",
			bufferSize: 1000,
		},
		{
			name:       "small buffer size",
			bufferSize: 10,
		},
		{
			name:       "large buffer size",
			bufferSize: 100000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewManager(tt.bufferSize)
			if mgr == nil {
				t.Fatal("NewManager() returned nil")
			}
			if mgr.bufferSize != tt.bufferSize {
				t.Errorf("bufferSize = %d, want %d", mgr.bufferSize, tt.bufferSize)
			}
			if mgr.Count() != 0 {
				t.Errorf("Count() = %d, want 0", mgr.Count())
			}
		})
	}
}

// TestManager_Create tests room creation
func TestManager_Create(t *testing.T) {
	mgr := NewManager(100)

	// Create first room
	room := mgr.Create("room1", "Room One")

	if room == nil {
		t.Fatal("Create() returned nil")
	}
	if room.ID != "room1" {
		t.Errorf("Room.ID = %s, want 'room1'", room.ID)
	}
	if room.Name != "Room One" {
		t.Errorf("Room.Name = %s, want 'Room One'", room.Name)
	}
	if room.Buffer == nil {
		t.Error("Room.Buffer is nil")
	}
	if room.KeyValueStore == nil {
		t.Error("Room.KeyValueStore is nil")
	}
	if room.CreatedAt.IsZero() {
		t.Error("Room.CreatedAt is zero")
	}
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}
}

// TestManager_Create_Duplicate tests creating duplicate room
func TestManager_Create_Duplicate(t *testing.T) {
	mgr := NewManager(100)

	room1 := mgr.Create("room1", "Room One")
	room2 := mgr.Create("room1", "Different Name")

	// Should return the same room instance
	if room1 != room2 {
		t.Error("Create() with duplicate ID returned different room")
	}
	if room2.Name != "Room One" {
		t.Errorf("Duplicate create changed room name to %s", room2.Name)
	}
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (duplicate should not create new room)", mgr.Count())
	}
}

// TestManager_Get tests room retrieval
func TestManager_Get(t *testing.T) {
	mgr := NewManager(100)
	created := mgr.Create("room1", "Room One")

	// Get existing room
	room, err := mgr.Get("room1")
	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if room != created {
		t.Error("Get() returned different room instance")
	}

	// Get non-existent room
	_, err = mgr.Get("nonexistent")
	if err == nil {
		t.Error("Get() returned nil error for non-existent room")
	}
	if err != ErrRoomNotFound {
		t.Errorf("Get() error = %v, want ErrRoomNotFound", err)
	}
}

// TestManager_GetOrCreate tests GetOrCreate functionality
func TestManager_GetOrCreate(t *testing.T) {
	mgr := NewManager(100)

	// Create room via GetOrCreate
	room1 := mgr.GetOrCreate("room1", "Room One")
	if room1 == nil {
		t.Fatal("GetOrCreate() returned nil")
	}
	if room1.ID != "room1" {
		t.Errorf("Room.ID = %s, want 'room1'", room1.ID)
	}
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}

	// Get existing room via GetOrCreate
	room2 := mgr.GetOrCreate("room1", "Different Name")
	if room2 != room1 {
		t.Error("GetOrCreate() returned different room instance for existing room")
	}
	if room2.Name != "Room One" {
		t.Error("GetOrCreate() modified existing room name")
	}
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}
}

// TestManager_List tests listing all rooms
func TestManager_List(t *testing.T) {
	mgr := NewManager(100)

	// Empty list
	rooms := mgr.List()
	if rooms == nil {
		t.Fatal("List() returned nil")
	}
	if len(rooms) != 0 {
		t.Errorf("List() returned %d rooms, want 0", len(rooms))
	}

	// Create multiple rooms
	mgr.Create("room1", "Room One")
	mgr.Create("room2", "Room Two")
	mgr.Create("room3", "Room Three")

	rooms = mgr.List()
	if len(rooms) != 3 {
		t.Errorf("List() returned %d rooms, want 3", len(rooms))
	}

	// Verify all rooms are present
	roomIDs := make(map[string]bool)
	for _, room := range rooms {
		roomIDs[room.ID] = true
	}
	for _, id := range []string{"room1", "room2", "room3"} {
		if !roomIDs[id] {
			t.Errorf("List() missing room %s", id)
		}
	}
}

// TestManager_Delete tests room deletion
func TestManager_Delete(t *testing.T) {
	mgr := NewManager(100)
	mgr.Create("room1", "Room One")
	mgr.Create("room2", "Room Two")

	// Delete existing room
	err := mgr.Delete("room1")
	if err != nil {
		t.Errorf("Delete() returned error: %v", err)
	}
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1 after deletion", mgr.Count())
	}

	// Verify room is deleted
	_, err = mgr.Get("room1")
	if err == nil {
		t.Error("Get() found deleted room")
	}

	// Delete non-existent room
	err = mgr.Delete("nonexistent")
	if err == nil {
		t.Error("Delete() returned nil error for non-existent room")
	}
	if err != ErrRoomNotFound {
		t.Errorf("Delete() error = %v, want ErrRoomNotFound", err)
	}
}

// TestManager_Count tests room counting
func TestManager_Count(t *testing.T) {
	mgr := NewManager(100)

	if mgr.Count() != 0 {
		t.Errorf("Initial Count() = %d, want 0", mgr.Count())
	}

	mgr.Create("room1", "Room One")
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}

	mgr.Create("room2", "Room Two")
	mgr.Create("room3", "Room Three")
	if mgr.Count() != 3 {
		t.Errorf("Count() = %d, want 3", mgr.Count())
	}

	mgr.Delete("room2")
	if mgr.Count() != 2 {
		t.Errorf("Count() = %d, want 2 after deletion", mgr.Count())
	}
}

// TestRoom_SendMessage tests sending messages to a room
func TestRoom_SendMessage(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	// Send first message
	msg, overwritten := room.SendMessage(creatorID, "Hello")

	if msg == nil {
		t.Fatal("SendMessage() returned nil message")
	}
	if msg.RoomID != "room1" {
		t.Errorf("Message.RoomID = %s, want 'room1'", msg.RoomID)
	}
	if msg.CreatorID != creatorID {
		t.Errorf("Message.CreatorID = %v, want %v", msg.CreatorID, creatorID)
	}
	if msg.Content != "Hello" {
		t.Errorf("Message.Content = %s, want 'Hello'", msg.Content)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Message.Timestamp is zero")
	}
	if overwritten {
		t.Error("SendMessage() overwritten = true, want false for first message")
	}
}

// TestRoom_GetMessages tests retrieving messages
func TestRoom_GetMessages(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	// Empty room
	messages := room.GetMessages()
	if len(messages) != 0 {
		t.Errorf("GetMessages() returned %d messages, want 0", len(messages))
	}

	// Send multiple messages
	room.SendMessage(creatorID, "Message 1")
	room.SendMessage(creatorID, "Message 2")
	room.SendMessage(creatorID, "Message 3")

	messages = room.GetMessages()
	if len(messages) != 3 {
		t.Fatalf("GetMessages() returned %d messages, want 3", len(messages))
	}

	// Verify order
	expectedContents := []string{"Message 1", "Message 2", "Message 3"}
	for i, msg := range messages {
		if msg.Content != expectedContents[i] {
			t.Errorf("Message[%d].Content = %s, want %s", i, msg.Content, expectedContents[i])
		}
	}
}

// TestRoom_GetMessagesSince tests timestamp filtering
func TestRoom_GetMessagesSince(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	baseTime := time.Now()

	room.SendMessage(creatorID, "Old 1")
	time.Sleep(10 * time.Millisecond)
	room.SendMessage(creatorID, "Old 2")
	time.Sleep(10 * time.Millisecond)

	cutoffTime := time.Now()

	time.Sleep(10 * time.Millisecond)
	room.SendMessage(creatorID, "New 1")
	time.Sleep(10 * time.Millisecond)
	room.SendMessage(creatorID, "New 2")

	messages := room.GetMessagesSince(cutoffTime)

	if len(messages) != 2 {
		t.Fatalf("GetMessagesSince() returned %d messages, want 2", len(messages))
	}

	for _, msg := range messages {
		if !msg.Timestamp.After(cutoffTime) {
			t.Errorf("Message %s has timestamp before cutoff", msg.Content)
		}
	}

	// Test with timestamp before all messages
	allMessages := room.GetMessagesSince(baseTime.Add(-1 * time.Minute))
	if len(allMessages) != 4 {
		t.Errorf("GetMessagesSince(before all) returned %d messages, want 4", len(allMessages))
	}

	// Test with timestamp after all messages
	noMessages := room.GetMessagesSince(time.Now().Add(1 * time.Minute))
	if len(noMessages) != 0 {
		t.Errorf("GetMessagesSince(after all) returned %d messages, want 0", len(noMessages))
	}
}

// TestRoom_DeleteMessage tests message deletion
func TestRoom_DeleteMessage(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	msg1, _ := room.SendMessage(creatorID, "Message 1")
	msg2, _ := room.SendMessage(creatorID, "Message 2")
	msg3, _ := room.SendMessage(creatorID, "Message 3")

	// Delete middle message
	deleted := room.DeleteMessage(msg2.ID, msg2.CreatorID, msg2.Content)
	if !deleted {
		t.Error("DeleteMessage() = false, want true")
	}

	messages := room.GetMessages()
	if len(messages) != 2 {
		t.Fatalf("GetMessages() returned %d messages, want 2", len(messages))
	}

	// Verify remaining messages
	if messages[0].ID != msg1.ID || messages[1].ID != msg3.ID {
		t.Error("Wrong messages remained after deletion")
	}

	// Delete non-existent message
	deleted = room.DeleteMessage(uuid.New(), creatorID, "Nonexistent")
	if deleted {
		t.Error("DeleteMessage() = true for non-existent message")
	}

	// Delete with wrong creator ID
	deleted = room.DeleteMessage(msg1.ID, uuid.New(), msg1.Content)
	if deleted {
		t.Error("DeleteMessage() = true with wrong creator ID")
	}

	// Delete with wrong content
	deleted = room.DeleteMessage(msg1.ID, msg1.CreatorID, "Wrong content")
	if deleted {
		t.Error("DeleteMessage() = true with wrong content")
	}
}

// TestRoom_SetKV tests key-value storage
func TestRoom_SetKV(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")

	room.SetKV("key1", "value1")
	room.SetKV("key2", "value2")

	// Verify values
	val, exists := room.GetKV("key1")
	if !exists {
		t.Error("GetKV() exists = false, want true")
	}
	if val != "value1" {
		t.Errorf("GetKV() = %s, want 'value1'", val)
	}

	val, exists = room.GetKV("key2")
	if !exists {
		t.Error("GetKV() exists = false for key2")
	}
	if val != "value2" {
		t.Errorf("GetKV() = %s, want 'value2'", val)
	}
}

// TestRoom_SetKV_Overwrite tests overwriting existing keys
func TestRoom_SetKV_Overwrite(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")

	room.SetKV("key1", "original")
	room.SetKV("key1", "updated")

	val, exists := room.GetKV("key1")
	if !exists {
		t.Error("GetKV() exists = false")
	}
	if val != "updated" {
		t.Errorf("GetKV() = %s, want 'updated'", val)
	}
}

// TestRoom_GetKV_NonExistent tests getting non-existent key
func TestRoom_GetKV_NonExistent(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")

	val, exists := room.GetKV("nonexistent")
	if exists {
		t.Error("GetKV() exists = true, want false")
	}
	if val != "" {
		t.Errorf("GetKV() = %s, want empty string", val)
	}
}

// TestRoom_DeleteKV tests key-value deletion
func TestRoom_DeleteKV(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")

	room.SetKV("key1", "value1")
	room.SetKV("key2", "value2")

	room.DeleteKV("key1")

	// Verify key1 is deleted
	_, exists := room.GetKV("key1")
	if exists {
		t.Error("GetKV() found deleted key")
	}

	// Verify key2 still exists
	val, exists := room.GetKV("key2")
	if !exists {
		t.Error("GetKV() key2 not found")
	}
	if val != "value2" {
		t.Errorf("GetKV() key2 = %s, want 'value2'", val)
	}

	// Delete non-existent key (should not panic)
	room.DeleteKV("nonexistent")
}

// TestRoom_GetAllKV tests retrieving all key-value pairs
func TestRoom_GetAllKV(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")

	// Empty KV store
	kv := room.GetAllKV()
	if len(kv) != 0 {
		t.Errorf("GetAllKV() returned %d pairs, want 0", len(kv))
	}

	// Add multiple pairs
	room.SetKV("key1", "value1")
	room.SetKV("key2", "value2")
	room.SetKV("key3", "value3")

	kv = room.GetAllKV()
	if len(kv) != 3 {
		t.Fatalf("GetAllKV() returned %d pairs, want 3", len(kv))
	}

	// Verify all pairs
	expected := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for k, v := range expected {
		if kv[k] != v {
			t.Errorf("GetAllKV()[%s] = %s, want %s", k, kv[k], v)
		}
	}
}

// TestRoom_GetAllKV_ImmutableCopy tests that GetAllKV returns a copy
func TestRoom_GetAllKV_ImmutableCopy(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")

	room.SetKV("key1", "value1")

	kv := room.GetAllKV()
	kv["key2"] = "external_modification"
	kv["key1"] = "modified"

	// Verify original is unchanged
	val, _ := room.GetKV("key1")
	if val != "value1" {
		t.Errorf("Original value changed to %s", val)
	}

	_, exists := room.GetKV("key2")
	if exists {
		t.Error("External modification affected room KV store")
	}
}

// TestRoom_Stats tests statistics retrieval
func TestRoom_Stats(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	// Initial stats
	stats := room.Stats()
	if stats == nil {
		t.Fatal("Stats() returned nil")
	}

	if stats["id"] != "room1" {
		t.Errorf("Stats()['id'] = %v, want 'room1'", stats["id"])
	}
	if stats["name"] != "Room One" {
		t.Errorf("Stats()['name'] = %v, want 'Room One'", stats["name"])
	}
	if stats["message_count"] != 0 {
		t.Errorf("Stats()['message_count'] = %v, want 0", stats["message_count"])
	}
	if stats["kv_count"] != 0 {
		t.Errorf("Stats()['kv_count'] = %v, want 0", stats["kv_count"])
	}

	// Add data and verify stats update
	room.SendMessage(creatorID, "Message 1")
	room.SendMessage(creatorID, "Message 2")
	room.SetKV("key1", "value1")

	stats = room.Stats()
	if stats["message_count"] != 2 {
		t.Errorf("Stats()['message_count'] = %v, want 2", stats["message_count"])
	}
	if stats["kv_count"] != 1 {
		t.Errorf("Stats()['kv_count'] = %v, want 1", stats["kv_count"])
	}
}

// TestRoomError tests error type
func TestRoomError(t *testing.T) {
	err := ErrRoomNotFound
	if err == nil {
		t.Fatal("ErrRoomNotFound is nil")
	}
	if err.Error() != "room not found" {
		t.Errorf("Error() = %s, want 'room not found'", err.Error())
	}
}

// TestConcurrent_CreateAndGet tests concurrent room creation and retrieval
func TestConcurrent_CreateAndGet(t *testing.T) {
	mgr := NewManager(100)
	var wg sync.WaitGroup

	// Concurrent creates
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			roomID := uuid.New().String()
			mgr.Create(roomID, "Room")
		}(i)
	}

	// Concurrent gets
	mgr.Create("target", "Target Room")
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.Get("target")
		}()
	}

	wg.Wait()

	if mgr.Count() != 51 { // 50 from loop + 1 target
		t.Errorf("Count() = %d, want 51", mgr.Count())
	}
}

// TestConcurrent_GetOrCreate tests concurrent GetOrCreate calls
func TestConcurrent_GetOrCreate(t *testing.T) {
	mgr := NewManager(100)
	var wg sync.WaitGroup

	// Multiple goroutines try to GetOrCreate the same room
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.GetOrCreate("shared", "Shared Room")
		}()
	}

	wg.Wait()

	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (should only create once)", mgr.Count())
	}
}

// TestConcurrent_ListAndDelete tests concurrent list and delete operations
func TestConcurrent_ListAndDelete(t *testing.T) {
	mgr := NewManager(100)
	var wg sync.WaitGroup

	// Create initial rooms
	roomIDs := make([]string, 20)
	for i := 0; i < 20; i++ {
		roomIDs[i] = uuid.New().String()
		mgr.Create(roomIDs[i], "Room")
	}

	// Concurrent deletes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			mgr.Delete(roomIDs[idx])
		}(i)
	}

	// Concurrent lists
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.List()
		}()
	}

	wg.Wait()

	if mgr.Count() != 10 {
		t.Errorf("Count() = %d, want 10", mgr.Count())
	}
}

// TestConcurrent_RoomMessages tests concurrent message operations on a room
func TestConcurrent_RoomMessages(t *testing.T) {
	mgr := NewManager(1000)
	room := mgr.Create("room1", "Room One")
	var wg sync.WaitGroup

	// Concurrent message sends
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			creatorID := uuid.New()
			room.SendMessage(creatorID, "Concurrent message")
		}()
	}

	// Concurrent message gets
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			room.GetMessages()
		}()
	}

	wg.Wait()

	messages := room.GetMessages()
	if len(messages) != 100 {
		t.Errorf("GetMessages() returned %d messages, want 100", len(messages))
	}
}

// TestConcurrent_RoomKV tests concurrent KV operations on a room
func TestConcurrent_RoomKV(t *testing.T) {
	mgr := NewManager(100)
	room := mgr.Create("room1", "Room One")
	var wg sync.WaitGroup

	// Concurrent sets
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := uuid.New().String()
			room.SetKV(key, "value")
		}(i)
	}

	// Concurrent gets
	room.SetKV("shared", "shared_value")
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			room.GetKV("shared")
		}()
	}

	// Concurrent GetAllKV
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			room.GetAllKV()
		}()
	}

	wg.Wait()

	kv := room.GetAllKV()
	if len(kv) != 51 { // 50 from loop + 1 shared
		t.Errorf("GetAllKV() returned %d pairs, want 51", len(kv))
	}
}

// TestConcurrent_MixedOperations tests various operations concurrently
func TestConcurrent_MixedOperations(t *testing.T) {
	mgr := NewManager(100)
	var wg sync.WaitGroup

	// Create initial rooms
	for i := 0; i < 5; i++ {
		mgr.Create(uuid.New().String(), "Room")
	}

	operations := 50

	for i := 0; i < operations; i++ {
		// Create
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.Create(uuid.New().String(), "Room")
		}()

		// GetOrCreate
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.GetOrCreate(uuid.New().String(), "Room")
		}()

		// List
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.List()
		}()

		// Count
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.Count()
		}()
	}

	wg.Wait()

	count := mgr.Count()
	if count <= 0 {
		t.Errorf("Count() = %d, want > 0", count)
	}
}

// TestManager_BufferSize tests that rooms use the manager's buffer size
func TestManager_BufferSize(t *testing.T) {
	bufferSize := 50
	mgr := NewManager(bufferSize)
	room := mgr.Create("room1", "Room One")

	if room.Buffer.Size() != bufferSize {
		t.Errorf("Room buffer size = %d, want %d", room.Buffer.Size(), bufferSize)
	}

	// Verify buffer actually uses the size
	creatorID := uuid.New()
	for i := 0; i < bufferSize+10; i++ {
		room.SendMessage(creatorID, "Message")
	}

	messages := room.GetMessages()
	if len(messages) != bufferSize {
		t.Errorf("GetMessages() returned %d messages, want %d (buffer size)", len(messages), bufferSize)
	}
}

// TestRoom_MessageOverwrite tests message overwrite behavior in room
func TestRoom_MessageOverwrite(t *testing.T) {
	mgr := NewManager(3)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	// Fill buffer
	room.SendMessage(creatorID, "M1")
	room.SendMessage(creatorID, "M2")
	_, overwritten := room.SendMessage(creatorID, "M3")

	if overwritten {
		t.Error("SendMessage() overwritten = true, want false when buffer is full but not overwritten")
	}

	// Trigger overwrite
	_, overwritten = room.SendMessage(creatorID, "M4")
	if !overwritten {
		t.Error("SendMessage() overwritten = false, want true after overwriting")
	}

	messages := room.GetMessages()
	if len(messages) != 3 {
		t.Errorf("GetMessages() returned %d messages, want 3", len(messages))
	}

	// Verify oldest message was overwritten
	expectedContents := []string{"M2", "M3", "M4"}
	for i, msg := range messages {
		if msg.Content != expectedContents[i] {
			t.Errorf("Message[%d].Content = %s, want %s", i, msg.Content, expectedContents[i])
		}
	}
}

// TestRoom_StatsBufferWrapping tests stats reflect buffer wrapping
func TestRoom_StatsBufferWrapping(t *testing.T) {
	mgr := NewManager(2)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	stats := room.Stats()
	if stats["buffer_wrapping"].(bool) {
		t.Error("Stats()['buffer_wrapping'] = true initially, want false")
	}

	// Fill and wrap buffer
	room.SendMessage(creatorID, "M1")
	room.SendMessage(creatorID, "M2")
	room.SendMessage(creatorID, "M3")

	stats = room.Stats()
	if !stats["buffer_wrapping"].(bool) {
		t.Error("Stats()['buffer_wrapping'] = false after wrap, want true")
	}
}

// ============================================================================
// BENCHMARKS
// ============================================================================

// BenchmarkManager_Create measures room creation performance
func BenchmarkManager_Create(b *testing.B) {
	mgr := NewManager(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.Create("room"+string(rune(i%1000)), "Room")
	}
}

// BenchmarkManager_Get measures room lookup performance
func BenchmarkManager_Get(b *testing.B) {
	mgr := NewManager(1000)
	for i := 0; i < 100; i++ {
		mgr.Create("room"+string(rune(i)), "Room")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mgr.Get("room" + string(rune(i%100)))
	}
}

// BenchmarkManager_GetOrCreate measures get-or-create performance
func BenchmarkManager_GetOrCreate(b *testing.B) {
	mgr := NewManager(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mgr.GetOrCreate("room"+string(rune(i%100)), "Room")
	}
}

// BenchmarkManager_List measures room listing performance
func BenchmarkManager_List(b *testing.B) {
	mgr := NewManager(1000)
	for i := 0; i < 100; i++ {
		mgr.Create("room"+string(rune(i)), "Room")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mgr.List()
	}
}

// BenchmarkRoom_SendMessage measures message sending performance
func BenchmarkRoom_SendMessage(b *testing.B) {
	mgr := NewManager(10000)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.SendMessage(creatorID, "benchmark message")
	}
}

// BenchmarkRoom_GetMessages measures message retrieval performance
func BenchmarkRoom_GetMessages(b *testing.B) {
	mgr := NewManager(1000)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	for i := 0; i < 1000; i++ {
		room.SendMessage(creatorID, "message")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = room.GetMessages()
	}
}

// BenchmarkRoom_GetMessagesSince measures historical retrieval
func BenchmarkRoom_GetMessagesSince(b *testing.B) {
	mgr := NewManager(1000)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	for i := 0; i < 1000; i++ {
		room.SendMessage(creatorID, "message")
	}
	since := time.Now().Add(-1 * time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = room.GetMessagesSince(since)
	}
}

// BenchmarkRoom_SetKV measures key-value set performance
func BenchmarkRoom_SetKV(b *testing.B) {
	mgr := NewManager(1000)
	room := mgr.Create("room1", "Room One")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		room.SetKV("key"+string(rune(i%100)), "value")
	}
}

// BenchmarkRoom_GetKV measures key-value get performance
func BenchmarkRoom_GetKV(b *testing.B) {
	mgr := NewManager(1000)
	room := mgr.Create("room1", "Room One")

	for i := 0; i < 100; i++ {
		room.SetKV("key"+string(rune(i)), "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = room.GetKV("key" + string(rune(i%100)))
	}
}

// BenchmarkRoom_Stats measures stats collection performance
func BenchmarkRoom_Stats(b *testing.B) {
	mgr := NewManager(1000)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	for i := 0; i < 500; i++ {
		room.SendMessage(creatorID, "message")
	}
	for i := 0; i < 50; i++ {
		room.SetKV("key"+string(rune(i)), "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = room.Stats()
	}
}

// BenchmarkConcurrentRoomOps measures concurrent room operations
func BenchmarkConcurrentRoomOps(b *testing.B) {
	mgr := NewManager(10000)
	room := mgr.Create("room1", "Room One")
	creatorID := uuid.New()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		room.SendMessage(creatorID, "initial")
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			room.SendMessage(creatorID, "message")
			_ = room.GetMessages()
		}
	})
}
