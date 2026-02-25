// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package wotan

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"unheaded/services/wotan/internal/ringbuffer"
)

// Helper function to create a test message
func createTestMessage(roomID, content string) *ringbuffer.Message {
	return &ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: uuid.New(),
		RoomID:    roomID,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// TestNewWotan verifies wotan initialization
func TestNewWotan(t *testing.T) {
	b := NewWotan()

	if b == nil {
		t.Fatal("NewWotan() returned nil")
	}
	if b.subscriptions == nil {
		t.Error("subscriptions map is nil")
	}
	if b.GetTotalSubscriptions() != 0 {
		t.Errorf("GetTotalSubscriptions() = %d, want 0", b.GetTotalSubscriptions())
	}
}

// TestSubscribe_Basic tests basic subscription creation
func TestSubscribe_Basic(t *testing.T) {
	b := NewWotan()
	memberID := uuid.New()

	sub := b.Subscribe("room1", memberID, 10)

	if sub == nil {
		t.Fatal("Subscribe() returned nil")
	}
	if sub.ID == uuid.Nil {
		t.Error("Subscription.ID is nil UUID")
	}
	if sub.RoomID != "room1" {
		t.Errorf("Subscription.RoomID = %s, want 'room1'", sub.RoomID)
	}
	if sub.MemberID != memberID {
		t.Errorf("Subscription.MemberID = %v, want %v", sub.MemberID, memberID)
	}
	if sub.Channel == nil {
		t.Error("Subscription.Channel is nil")
	}
	if sub.cancel == nil {
		t.Error("Subscription.cancel is nil")
	}

	// Verify subscription count
	if b.GetSubscriptionCount("room1") != 1 {
		t.Errorf("GetSubscriptionCount('room1') = %d, want 1", b.GetSubscriptionCount("room1"))
	}
	if b.GetTotalSubscriptions() != 1 {
		t.Errorf("GetTotalSubscriptions() = %d, want 1", b.GetTotalSubscriptions())
	}
}

// TestSubscribe_MultipleRooms tests subscriptions across multiple rooms
func TestSubscribe_MultipleRooms(t *testing.T) {
	b := NewWotan()
	memberID := uuid.New()

	sub1 := b.Subscribe("room1", memberID, 10)
	sub2 := b.Subscribe("room2", memberID, 10)
	sub3 := b.Subscribe("room1", memberID, 10)

	if b.GetSubscriptionCount("room1") != 2 {
		t.Errorf("GetSubscriptionCount('room1') = %d, want 2", b.GetSubscriptionCount("room1"))
	}
	if b.GetSubscriptionCount("room2") != 1 {
		t.Errorf("GetSubscriptionCount('room2') = %d, want 1", b.GetSubscriptionCount("room2"))
	}
	if b.GetTotalSubscriptions() != 3 {
		t.Errorf("GetTotalSubscriptions() = %d, want 3", b.GetTotalSubscriptions())
	}

	// Verify unique IDs
	if sub1.ID == sub2.ID || sub1.ID == sub3.ID || sub2.ID == sub3.ID {
		t.Error("Subscription IDs are not unique")
	}
}

// TestSubscribe_BufferSize tests channel buffer size
func TestSubscribe_BufferSize(t *testing.T) {
	b := NewWotan()
	memberID := uuid.New()

	bufferSize := 5
	sub := b.Subscribe("room1", memberID, bufferSize)

	// Send exactly bufferSize events without reading (should not block)
	msg := createTestMessage("room1", "test")
	for i := 0; i < bufferSize; i++ {
		event := &MessageEvent{
			Type:      EventMessageCreated,
			Message:   msg,
			Timestamp: time.Now(),
		}
		b.Publish("room1", event)
	}

	// Channel should have bufferSize events
	received := 0
	timeout := time.After(100 * time.Millisecond)
	for received < bufferSize {
		select {
		case <-sub.Channel:
			received++
		case <-timeout:
			t.Fatalf("Timeout: received %d events, expected %d", received, bufferSize)
		}
	}

	if received != bufferSize {
		t.Errorf("Received %d events, want %d", received, bufferSize)
	}
}

// TestUnsubscribe_Basic tests basic unsubscription
func TestUnsubscribe_Basic(t *testing.T) {
	b := NewWotan()
	memberID := uuid.New()

	sub := b.Subscribe("room1", memberID, 10)
	b.Unsubscribe(sub)

	if b.GetSubscriptionCount("room1") != 0 {
		t.Errorf("GetSubscriptionCount('room1') = %d, want 0", b.GetSubscriptionCount("room1"))
	}
	if b.GetTotalSubscriptions() != 0 {
		t.Errorf("GetTotalSubscriptions() = %d, want 0", b.GetTotalSubscriptions())
	}

	// Verify channel is closed
	select {
	case _, ok := <-sub.Channel:
		if ok {
			t.Error("Channel is still open after unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel not closed after unsubscribe")
	}
}

// TestUnsubscribe_MultipleSubscriptions tests unsubscribing one of many
func TestUnsubscribe_MultipleSubscriptions(t *testing.T) {
	b := NewWotan()
	memberID := uuid.New()

	sub1 := b.Subscribe("room1", memberID, 10)
	sub2 := b.Subscribe("room1", memberID, 10)
	sub3 := b.Subscribe("room1", memberID, 10)

	// Unsubscribe middle subscription
	b.Unsubscribe(sub2)

	if b.GetSubscriptionCount("room1") != 2 {
		t.Errorf("GetSubscriptionCount('room1') = %d, want 2", b.GetSubscriptionCount("room1"))
	}

	// Verify sub1 and sub3 still work
	msg := createTestMessage("room1", "test")
	b.PublishMessageCreated(msg)

	// sub1 should receive
	select {
	case <-sub1.Channel:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Error("sub1 did not receive event")
	}

	// sub3 should receive
	select {
	case <-sub3.Channel:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Error("sub3 did not receive event")
	}

	// sub2 channel should be closed
	select {
	case _, ok := <-sub2.Channel:
		if ok {
			t.Error("sub2 channel is still open")
		}
	default:
		// OK - channel is closed
	}
}

// TestUnsubscribe_LastSubscription tests cleanup when last subscriber leaves
func TestUnsubscribe_LastSubscription(t *testing.T) {
	b := NewWotan()
	memberID := uuid.New()

	sub := b.Subscribe("room1", memberID, 10)
	b.Unsubscribe(sub)

	// Verify room is cleaned up from subscriptions map
	b.mu.RLock()
	_, exists := b.subscriptions["room1"]
	b.mu.RUnlock()

	if exists {
		t.Error("Room entry not cleaned up after last unsubscribe")
	}
}

// TestUnsubscribe_NonExistent tests unsubscribing non-existent subscription
func TestUnsubscribe_NonExistent(t *testing.T) {
	b := NewWotan()
	memberID := uuid.New()

	// Create and unsubscribe
	sub := b.Subscribe("room1", memberID, 10)
	b.Unsubscribe(sub)

	// Unsubscribe again (should not panic)
	b.Unsubscribe(sub)

	if b.GetTotalSubscriptions() != 0 {
		t.Errorf("GetTotalSubscriptions() = %d, want 0", b.GetTotalSubscriptions())
	}
}

// TestPublish_Basic tests basic event publishing
func TestPublish_Basic(t *testing.T) {
	b := NewWotan()
	memberID := uuid.New()

	sub := b.Subscribe("room1", memberID, 10)

	msg := createTestMessage("room1", "Hello")
	event := &MessageEvent{
		Type:      EventMessageCreated,
		Message:   msg,
		Timestamp: time.Now(),
	}

	b.Publish("room1", event)

	// Verify event received
	select {
	case received := <-sub.Channel:
		if received.Type != EventMessageCreated {
			t.Errorf("Event.Type = %s, want %s", received.Type, EventMessageCreated)
		}
		if received.Message.ID != msg.ID {
			t.Error("Event.Message.ID mismatch")
		}
		if received.Message.Content != "Hello" {
			t.Errorf("Event.Message.Content = %s, want 'Hello'", received.Message.Content)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Event not received")
	}
}

// TestPublish_NoSubscribers tests publishing to room with no subscribers
func TestPublish_NoSubscribers(t *testing.T) {
	b := NewWotan()

	msg := createTestMessage("room1", "Hello")
	event := &MessageEvent{
		Type:      EventMessageCreated,
		Message:   msg,
		Timestamp: time.Now(),
	}

	// Should not panic
	b.Publish("room1", event)
}

// TestPublish_MultipleSubscribers tests fanout to multiple subscribers
func TestPublish_MultipleSubscribers(t *testing.T) {
	b := NewWotan()

	sub1 := b.Subscribe("room1", uuid.New(), 10)
	sub2 := b.Subscribe("room1", uuid.New(), 10)
	sub3 := b.Subscribe("room1", uuid.New(), 10)

	msg := createTestMessage("room1", "Broadcast")
	b.PublishMessageCreated(msg)

	// All three should receive
	for i, sub := range []*Subscription{sub1, sub2, sub3} {
		select {
		case received := <-sub.Channel:
			if received.Message.Content != "Broadcast" {
				t.Errorf("Subscriber %d: wrong message content", i+1)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Subscriber %d: did not receive event", i+1)
		}
	}
}

// TestPublish_RoomIsolation tests that events don't leak to other rooms
func TestPublish_RoomIsolation(t *testing.T) {
	b := NewWotan()

	sub1 := b.Subscribe("room1", uuid.New(), 10)
	sub2 := b.Subscribe("room2", uuid.New(), 10)

	msg := createTestMessage("room1", "Room1 message")
	b.PublishMessageCreated(msg)

	// sub1 should receive
	select {
	case <-sub1.Channel:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sub1 did not receive event")
	}

	// sub2 should NOT receive
	select {
	case <-sub2.Channel:
		t.Error("sub2 received event from different room")
	case <-time.After(50 * time.Millisecond):
		// OK - no event received
	}
}

// TestPublish_FullChannelDropsEvent tests non-blocking behavior with full channel
func TestPublish_FullChannelDropsEvent(t *testing.T) {
	b := NewWotan()

	// Small buffer
	sub := b.Subscribe("room1", uuid.New(), 2)

	msg := createTestMessage("room1", "test")

	// Fill the buffer
	b.PublishMessageCreated(msg)
	b.PublishMessageCreated(msg)

	// This should not block (event will be dropped)
	done := make(chan bool)
	go func() {
		b.PublishMessageCreated(msg)
		done <- true
	}()

	select {
	case <-done:
		// OK - did not block
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked on full channel")
	}

	// Verify only 2 events in channel
	received := 0
	for {
		select {
		case <-sub.Channel:
			received++
		case <-time.After(10 * time.Millisecond):
			goto done
		}
	}
done:
	if received != 2 {
		t.Errorf("Received %d events, want 2 (third should be dropped)", received)
	}
}

// TestPublish_CancelledSubscription tests publishing to cancelled subscription
func TestPublish_CancelledSubscription(t *testing.T) {
	b := NewWotan()

	sub := b.Subscribe("room1", uuid.New(), 10)
	b.Unsubscribe(sub)

	msg := createTestMessage("room1", "test")

	// Should not panic
	b.PublishMessageCreated(msg)
}

// TestPublishMessageCreated tests convenience method
func TestPublishMessageCreated(t *testing.T) {
	b := NewWotan()
	sub := b.Subscribe("room1", uuid.New(), 10)

	msg := createTestMessage("room1", "Created")
	b.PublishMessageCreated(msg)

	select {
	case event := <-sub.Channel:
		if event.Type != EventMessageCreated {
			t.Errorf("Event.Type = %s, want %s", event.Type, EventMessageCreated)
		}
		if event.Message.Content != "Created" {
			t.Errorf("Event.Message.Content = %s, want 'Created'", event.Message.Content)
		}
		if event.Timestamp.IsZero() {
			t.Error("Event.Timestamp is zero")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Event not received")
	}
}

// TestPublishMessageDeleted tests deletion event with partial message
func TestPublishMessageDeleted(t *testing.T) {
	b := NewWotan()
	sub := b.Subscribe("room1", uuid.New(), 10)

	messageID := uuid.New()
	b.PublishMessageDeleted("room1", messageID)

	select {
	case event := <-sub.Channel:
		if event.Type != EventMessageDeleted {
			t.Errorf("Event.Type = %s, want %s", event.Type, EventMessageDeleted)
		}
		if event.Message.ID != messageID {
			t.Error("Event.Message.ID mismatch")
		}
		if event.Message.RoomID != "room1" {
			t.Errorf("Event.Message.RoomID = %s, want 'room1'", event.Message.RoomID)
		}
		// Content should be empty for deletion events
		if event.Message.Content != "" {
			t.Errorf("Event.Message.Content = %s, want empty for deletion", event.Message.Content)
		}
		if event.Timestamp.IsZero() {
			t.Error("Event.Timestamp is zero")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Event not received")
	}
}

// TestGetSubscriptionCount tests counting subscriptions per room
func TestGetSubscriptionCount(t *testing.T) {
	b := NewWotan()

	if b.GetSubscriptionCount("room1") != 0 {
		t.Errorf("Initial count = %d, want 0", b.GetSubscriptionCount("room1"))
	}

	b.Subscribe("room1", uuid.New(), 10)
	if b.GetSubscriptionCount("room1") != 1 {
		t.Errorf("Count after subscribe = %d, want 1", b.GetSubscriptionCount("room1"))
	}

	b.Subscribe("room1", uuid.New(), 10)
	b.Subscribe("room1", uuid.New(), 10)
	if b.GetSubscriptionCount("room1") != 3 {
		t.Errorf("Count = %d, want 3", b.GetSubscriptionCount("room1"))
	}

	// Different room should be 0
	if b.GetSubscriptionCount("room2") != 0 {
		t.Errorf("room2 count = %d, want 0", b.GetSubscriptionCount("room2"))
	}
}

// TestGetTotalSubscriptions tests counting all subscriptions
func TestGetTotalSubscriptions(t *testing.T) {
	b := NewWotan()

	if b.GetTotalSubscriptions() != 0 {
		t.Errorf("Initial total = %d, want 0", b.GetTotalSubscriptions())
	}

	b.Subscribe("room1", uuid.New(), 10)
	b.Subscribe("room1", uuid.New(), 10)
	b.Subscribe("room2", uuid.New(), 10)
	b.Subscribe("room3", uuid.New(), 10)

	if b.GetTotalSubscriptions() != 4 {
		t.Errorf("Total = %d, want 4", b.GetTotalSubscriptions())
	}
}

// TestGetTotalSubscriptions_AfterUnsubscribe tests count after unsubscribe
func TestGetTotalSubscriptions_AfterUnsubscribe(t *testing.T) {
	b := NewWotan()

	sub1 := b.Subscribe("room1", uuid.New(), 10)
	sub2 := b.Subscribe("room2", uuid.New(), 10)

	b.Unsubscribe(sub1)

	if b.GetTotalSubscriptions() != 1 {
		t.Errorf("Total after unsubscribe = %d, want 1", b.GetTotalSubscriptions())
	}

	b.Unsubscribe(sub2)

	if b.GetTotalSubscriptions() != 0 {
		t.Errorf("Total after all unsubscribed = %d, want 0", b.GetTotalSubscriptions())
	}
}

// TestClose tests cleanup of all subscriptions
func TestClose(t *testing.T) {
	b := NewWotan()

	sub1 := b.Subscribe("room1", uuid.New(), 10)
	sub2 := b.Subscribe("room2", uuid.New(), 10)
	sub3 := b.Subscribe("room1", uuid.New(), 10)

	b.Close()

	// All channels should be closed
	for i, sub := range []*Subscription{sub1, sub2, sub3} {
		select {
		case _, ok := <-sub.Channel:
			if ok {
				t.Errorf("Subscription %d: channel still open after Close", i+1)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Subscription %d: channel not closed after Close", i+1)
		}
	}

	// Total count should be 0
	if b.GetTotalSubscriptions() != 0 {
		t.Errorf("Total subscriptions = %d, want 0 after Close", b.GetTotalSubscriptions())
	}
}

// TestClose_Empty tests closing empty wotan
func TestClose_Empty(t *testing.T) {
	b := NewWotan()

	// Should not panic
	b.Close()
}

// TestEventType_Constants tests event type constants
func TestEventType_Constants(t *testing.T) {
	if EventMessageCreated != "MESSAGE_CREATED" {
		t.Errorf("EventMessageCreated = %s, want 'MESSAGE_CREATED'", EventMessageCreated)
	}
	if EventMessageDeleted != "MESSAGE_DELETED" {
		t.Errorf("EventMessageDeleted = %s, want 'MESSAGE_DELETED'", EventMessageDeleted)
	}
}

// TestConcurrent_Subscribe tests concurrent subscriptions
func TestConcurrent_Subscribe(t *testing.T) {
	b := NewWotan()
	var wg sync.WaitGroup

	subscriptionCount := 100

	for i := 0; i < subscriptionCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Subscribe("room1", uuid.New(), 10)
		}()
	}

	wg.Wait()

	if b.GetSubscriptionCount("room1") != subscriptionCount {
		t.Errorf("GetSubscriptionCount('room1') = %d, want %d", b.GetSubscriptionCount("room1"), subscriptionCount)
	}
}

// TestConcurrent_Unsubscribe tests concurrent unsubscriptions
func TestConcurrent_Unsubscribe(t *testing.T) {
	b := NewWotan()

	// Create subscriptions
	subs := make([]*Subscription, 50)
	for i := 0; i < 50; i++ {
		subs[i] = b.Subscribe("room1", uuid.New(), 10)
	}

	// Concurrent unsubscribes
	var wg sync.WaitGroup
	for _, sub := range subs {
		wg.Add(1)
		go func(s *Subscription) {
			defer wg.Done()
			b.Unsubscribe(s)
		}(sub)
	}

	wg.Wait()

	if b.GetTotalSubscriptions() != 0 {
		t.Errorf("Total subscriptions = %d, want 0", b.GetTotalSubscriptions())
	}
}

// TestConcurrent_PublishAndSubscribe tests concurrent publish and subscribe
func TestConcurrent_PublishAndSubscribe(t *testing.T) {
	b := NewWotan()
	var wg sync.WaitGroup

	// Concurrent subscribes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Subscribe("room1", uuid.New(), 100)
		}()
	}

	// Concurrent publishes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := createTestMessage("room1", "concurrent")
			b.PublishMessageCreated(msg)
		}()
	}

	wg.Wait()

	if b.GetSubscriptionCount("room1") != 50 {
		t.Errorf("GetSubscriptionCount('room1') = %d, want 50", b.GetSubscriptionCount("room1"))
	}
}

// TestConcurrent_PublishToMultipleRooms tests concurrent publishing to different rooms
func TestConcurrent_PublishToMultipleRooms(t *testing.T) {
	b := NewWotan()
	var wg sync.WaitGroup

	// Subscribe to multiple rooms
	rooms := []string{"room1", "room2", "room3", "room4", "room5"}
	for _, room := range rooms {
		b.Subscribe(room, uuid.New(), 100)
	}

	// Concurrent publishes to different rooms
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			room := rooms[n%len(rooms)]
			msg := createTestMessage(room, "test")
			b.PublishMessageCreated(msg)
		}(i)
	}

	wg.Wait()
}

// TestConcurrent_GetCounts tests concurrent count operations
func TestConcurrent_GetCounts(t *testing.T) {
	b := NewWotan()
	var wg sync.WaitGroup

	// Create subscriptions
	b.Subscribe("room1", uuid.New(), 10)
	b.Subscribe("room2", uuid.New(), 10)

	// Concurrent count queries
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.GetSubscriptionCount("room1")
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			b.GetTotalSubscriptions()
		}()
	}

	wg.Wait()
}

// TestConcurrent_MixedOperations tests various operations concurrently
func TestConcurrent_MixedOperations(t *testing.T) {
	b := NewWotan()
	var wg sync.WaitGroup

	operations := 50

	for i := 0; i < operations; i++ {
		// Subscribe
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Subscribe("room1", uuid.New(), 50)
		}()

		// Publish
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := createTestMessage("room1", "test")
			b.PublishMessageCreated(msg)
		}()

		// Get counts
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.GetSubscriptionCount("room1")
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			b.GetTotalSubscriptions()
		}()
	}

	wg.Wait()

	if b.GetSubscriptionCount("room1") != operations {
		t.Errorf("GetSubscriptionCount('room1') = %d, want %d", b.GetSubscriptionCount("room1"), operations)
	}
}

// TestConcurrent_SubscribeUnsubscribe tests concurrent subscribe and unsubscribe
func TestConcurrent_SubscribeUnsubscribe(t *testing.T) {
	b := NewWotan()
	var wg sync.WaitGroup

	// Create some initial subscriptions
	initialSubs := make([]*Subscription, 20)
	for i := 0; i < 20; i++ {
		initialSubs[i] = b.Subscribe("room1", uuid.New(), 10)
	}

	// Concurrent subscribe and unsubscribe
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Subscribe("room1", uuid.New(), 10)
		}()
	}

	for _, sub := range initialSubs {
		wg.Add(1)
		go func(s *Subscription) {
			defer wg.Done()
			b.Unsubscribe(s)
		}(sub)
	}

	wg.Wait()

	// Should have 30 subscriptions (20 unsubscribed, 30 added)
	if b.GetSubscriptionCount("room1") != 30 {
		t.Errorf("GetSubscriptionCount('room1') = %d, want 30", b.GetSubscriptionCount("room1"))
	}
}

// TestSubscription_ReceiveMultipleEvents tests receiving multiple events
func TestSubscription_ReceiveMultipleEvents(t *testing.T) {
	b := NewWotan()
	sub := b.Subscribe("room1", uuid.New(), 100)

	eventCount := 10

	// Publish multiple events
	for i := 0; i < eventCount; i++ {
		msg := createTestMessage("room1", "test")
		b.PublishMessageCreated(msg)
	}

	// Receive all events
	received := 0
	timeout := time.After(500 * time.Millisecond)
	for received < eventCount {
		select {
		case <-sub.Channel:
			received++
		case <-timeout:
			t.Fatalf("Timeout: received %d events, expected %d", received, eventCount)
		}
	}

	if received != eventCount {
		t.Errorf("Received %d events, want %d", received, eventCount)
	}
}

// TestSubscription_EventTypes tests receiving different event types
func TestSubscription_EventTypes(t *testing.T) {
	b := NewWotan()
	sub := b.Subscribe("room1", uuid.New(), 10)

	// Publish created event
	msg := createTestMessage("room1", "test")
	b.PublishMessageCreated(msg)

	// Publish deleted event
	b.PublishMessageDeleted("room1", msg.ID)

	// Receive created event
	select {
	case event := <-sub.Channel:
		if event.Type != EventMessageCreated {
			t.Errorf("First event type = %s, want %s", event.Type, EventMessageCreated)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Created event not received")
	}

	// Receive deleted event
	select {
	case event := <-sub.Channel:
		if event.Type != EventMessageDeleted {
			t.Errorf("Second event type = %s, want %s", event.Type, EventMessageDeleted)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Deleted event not received")
	}
}

// TestWotan_EmptyRoomPublish tests publishing to room with removed subscribers
func TestWotan_EmptyRoomPublish(t *testing.T) {
	b := NewWotan()

	sub := b.Subscribe("room1", uuid.New(), 10)
	b.Unsubscribe(sub)

	// Should not panic
	msg := createTestMessage("room1", "test")
	b.PublishMessageCreated(msg)
}

// TestWotan_MultipleClose tests calling Close multiple times
func TestWotan_MultipleClose(t *testing.T) {
	b := NewWotan()

	b.Subscribe("room1", uuid.New(), 10)

	// First close
	b.Close()

	// Second close (should not panic)
	b.Close()
}

// TestMessageEvent_Fields tests message event field population
func TestMessageEvent_Fields(t *testing.T) {
	b := NewWotan()
	sub := b.Subscribe("room1", uuid.New(), 10)

	msg := createTestMessage("room1", "test")
	msg.CreatorID = uuid.New()
	msg.Timestamp = time.Now()

	b.PublishMessageCreated(msg)

	select {
	case event := <-sub.Channel:
		if event.Type != EventMessageCreated {
			t.Errorf("Event.Type = %s, want %s", event.Type, EventMessageCreated)
		}
		if event.Message == nil {
			t.Fatal("Event.Message is nil")
		}
		if event.Message.ID != msg.ID {
			t.Error("Event.Message.ID mismatch")
		}
		if event.Message.CreatorID != msg.CreatorID {
			t.Error("Event.Message.CreatorID mismatch")
		}
		if event.Message.RoomID != msg.RoomID {
			t.Error("Event.Message.RoomID mismatch")
		}
		if event.Message.Content != msg.Content {
			t.Error("Event.Message.Content mismatch")
		}
		if event.Timestamp.IsZero() {
			t.Error("Event.Timestamp is zero")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Event not received")
	}
}

// ============================================================================
// BENCHMARKS
// ============================================================================

// BenchmarkSubscribe measures subscribe performance
func BenchmarkSubscribe(b *testing.B) {
	bb := NewWotan()
	defer bb.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		memberID := uuid.New()
		sub := bb.Subscribe("room1", memberID, 10)
		bb.Unsubscribe(sub)
	}
}

// BenchmarkPublish_SingleSubscriber measures publish with one subscriber
func BenchmarkPublish_SingleSubscriber(b *testing.B) {
	bb := NewWotan()
	defer bb.Close()

	memberID := uuid.New()
	sub := bb.Subscribe("room1", memberID, 1000)
	defer bb.Unsubscribe(sub)

	msg := createTestMessage("room1", "benchmark message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bb.PublishMessageCreated(msg)
		<-sub.Channel // Drain to prevent blocking
	}
}

// BenchmarkPublish_10Subscribers measures fanout to 10 subscribers
func BenchmarkPublish_10Subscribers(b *testing.B) {
	bb := NewWotan()
	defer bb.Close()

	subs := make([]*Subscription, 10)
	for i := 0; i < 10; i++ {
		memberID := uuid.New()
		subs[i] = bb.Subscribe("room1", memberID, 1000)
	}
	defer func() {
		for _, sub := range subs {
			bb.Unsubscribe(sub)
		}
	}()

	msg := createTestMessage("room1", "benchmark message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bb.PublishMessageCreated(msg)
		// Drain all channels
		for _, sub := range subs {
			<-sub.Channel
		}
	}
}

// BenchmarkPublish_100Subscribers measures fanout to 100 subscribers
func BenchmarkPublish_100Subscribers(b *testing.B) {
	bb := NewWotan()
	defer bb.Close()

	subs := make([]*Subscription, 100)
	for i := 0; i < 100; i++ {
		memberID := uuid.New()
		subs[i] = bb.Subscribe("room1", memberID, 1000)
	}
	defer func() {
		for _, sub := range subs {
			bb.Unsubscribe(sub)
		}
	}()

	msg := createTestMessage("room1", "benchmark message")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bb.PublishMessageCreated(msg)
		// Drain all channels
		for _, sub := range subs {
			<-sub.Channel
		}
	}
}

// BenchmarkPublishDeleted measures delete event publishing
func BenchmarkPublishDeleted(b *testing.B) {
	bb := NewWotan()
	defer bb.Close()

	memberID := uuid.New()
	sub := bb.Subscribe("room1", memberID, 1000)
	defer bb.Unsubscribe(sub)

	msgID := uuid.New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bb.PublishMessageDeleted("room1", msgID)
		<-sub.Channel
	}
}

// BenchmarkConcurrentPublish measures concurrent publish performance
func BenchmarkConcurrentPublish(b *testing.B) {
	bb := NewWotan()
	defer bb.Close()

	// Create multiple subscribers
	subs := make([]*Subscription, 10)
	for i := 0; i < 10; i++ {
		memberID := uuid.New()
		subs[i] = bb.Subscribe("room1", memberID, 10000)
	}
	defer func() {
		for _, sub := range subs {
			bb.Unsubscribe(sub)
		}
	}()

	// Drain goroutines
	var wg sync.WaitGroup
	for _, sub := range subs {
		wg.Add(1)
		go func(s *Subscription) {
			defer wg.Done()
			for range s.Channel {
			}
		}(sub)
	}

	msg := createTestMessage("room1", "benchmark message")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			bb.PublishMessageCreated(msg)
		}
	})

	b.StopTimer()
	bb.Close()
	wg.Wait()
}

// BenchmarkGetTotalSubscriptions measures subscription counting
func BenchmarkGetTotalSubscriptions(b *testing.B) {
	bb := NewWotan()
	defer bb.Close()

	// Create subscriptions across multiple rooms
	for i := 0; i < 100; i++ {
		memberID := uuid.New()
		roomID := "room" + string(rune('0'+i%10))
		bb.Subscribe(roomID, memberID, 10)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bb.GetTotalSubscriptions()
	}
}
