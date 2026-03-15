// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package wotan provides a publish-subscribe message wotan for real-time message distribution.
//
// The wotan implements a thread-safe pub/sub pattern that allows multiple clients (subscribers)
// to receive live updates when messages are created or deleted in rooms. This is the core
// component that enables real-time streaming via gRPC.
//
// Architecture:
//   - Each room has zero or more subscriptions (clients listening for updates)
//   - When a message event occurs, it's broadcast to all subscribers of that room
//   - Subscriptions are buffered channels to prevent slow clients from blocking publishers
//   - Thread-safe: uses RWMutex to protect concurrent access
//
// Usage:
//
//	wotan := wotan.NewWotan()
//
//	// Client subscribes to a room
//	sub := wotan.Subscribe("general", memberID, 100)
//	go func() {
//	    for event := range sub.Channel {
//	        // Process event (MESSAGE_CREATED or MESSAGE_DELETED)
//	    }
//	}()
//
//	// Server publishes events
//	wotan.PublishMessageCreated(msg)
//	wotan.PublishMessageDeleted(roomID, messageID)
//
//	// Client unsubscribes when done
//	wotan.Unsubscribe(sub)
package wotan

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"unheaded/services/wotan/internal/ringbuffer"
)

// EventType represents the type of message event that occurred.
// Events are broadcast to subscribers to inform them of changes in the room.
type EventType string

const (
	// EventMessageCreated indicates a new message was added to the room.
	// Subscribers should display or store this message.
	EventMessageCreated EventType = "MESSAGE_CREATED"

	// EventMessageDeleted indicates a message was removed from the room.
	// Subscribers should remove this message from their display/storage.
	EventMessageDeleted EventType = "MESSAGE_DELETED"
)

// MessageEvent represents a single event that occurred in a room.
// Events are sent to all active subscribers of the room through their subscription channels.
//
// Fields:
//   - Type: The kind of event (created or deleted)
//   - Message: The message that was created/deleted (may have partial data for deletion events)
//   - Timestamp: When the event occurred (server time)
type MessageEvent struct {
	Type      EventType              // Type of event (MESSAGE_CREATED or MESSAGE_DELETED)
	Message   *ringbuffer.Message    // The message involved in the event
	Timestamp time.Time              // Server timestamp when event occurred
}

// Subscription represents a client's subscription to a room's message events.
// Each subscription has a buffered channel that receives events as they occur.
//
// The subscription is unidirectional: events flow from the wotan to the client.
// Clients read from the Channel to receive events. When the subscription is cancelled,
// the cancel channel is closed and the event Channel is drained and closed.
//
// Fields:
//   - ID: Unique identifier for this subscription
//   - RoomID: The room this subscription is listening to
//   - MemberID: The member who owns this subscription
//   - Channel: Buffered channel that receives MessageEvent instances
//   - cancel: Internal channel used to signal subscription cancellation
type Subscription struct {
	ID       uuid.UUID            // Unique subscription ID
	RoomID   string               // Room being subscribed to
	MemberID uuid.UUID            // Member who owns this subscription
	Channel  chan *MessageEvent   // Channel receiving message events (buffered)
	cancel   chan struct{}        // Internal cancellation signal (closed on unsubscribe)
}

// Wotan handles pub/sub for room messages, enabling real-time message distribution.
//
// The wotan maintains a map of room IDs to their active subscriptions. When an event
// is published to a room, it's broadcast to all subscribers of that room through their
// buffered channels.
//
// Thread Safety:
// All methods are thread-safe and can be called concurrently. The wotan uses an RWMutex
// to protect the subscriptions map:
//   - Subscribe/Unsubscribe acquire write locks (modify the map)
//   - Publish/Get* methods acquire read locks (read the map)
//
// Performance Characteristics:
//   - Subscribe/Unsubscribe: O(1) for map operations, O(n) for array operations where n = subscriptions per room
//   - Publish: O(n) where n = number of subscribers to the room (non-blocking sends)
//   - GetSubscriptionCount: O(1) for specific room
//   - GetTotalSubscriptions: O(r) where r = number of rooms with subscriptions
//
// Memory Management:
// The wotan keeps subscriptions in memory until they're explicitly unsubscribed or
// the wotan is closed. Empty room subscription lists are automatically cleaned up.
type Wotan struct {
	mu            sync.RWMutex                    // Protects subscriptions map
	subscriptions map[string][]*Subscription      // roomID -> list of active subscriptions
}

// NewWotan creates a new message wotan instance.
//
// The returned wotan is ready to use and has no active subscriptions.
// All methods are thread-safe from the moment the wotan is created.
//
// Returns:
//   - A new Wotan instance with initialized internal state
//
// Example:
//
//	wotan := wotan.NewWotan()
//	defer wotan.Close() // Clean up when done
func NewWotan() *Wotan {
	return &Wotan{
		subscriptions: make(map[string][]*Subscription),
	}
}

// Subscribe creates a new subscription for a room.
//
// The subscription allows a client to receive real-time events (message creations and deletions)
// for the specified room. Events are sent through the subscription's buffered channel.
//
// Parameters:
//   - roomID: The room to subscribe to (does not need to exist yet)
//   - memberID: The member creating the subscription (for tracking/metrics)
//   - bufferSize: Size of the event channel buffer (recommended: 100-1000)
//
// Buffer Size Considerations:
//   - Too small: Risk of dropped events if client reads slowly
//   - Too large: Higher memory usage per subscription
//   - Recommended: 100 for typical chat, 1000 for high-frequency rooms
//
// Returns:
//   - A new Subscription that will receive events for the room
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Example:
//
//	sub := wotan.Subscribe("general", memberID, 100)
//	go func() {
//	    for event := range sub.Channel {
//	        fmt.Printf("Event: %s - %s\n", event.Type, event.Message.Content)
//	    }
//	}()
//	// Later: wotan.Unsubscribe(sub)
func (b *Wotan) Subscribe(roomID string, memberID uuid.UUID, bufferSize int) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Create new subscription with buffered event channel
	sub := &Subscription{
		ID:       uuid.New(),
		RoomID:   roomID,
		MemberID: memberID,
		Channel:  make(chan *MessageEvent, bufferSize),
		cancel:   make(chan struct{}),
	}

	// Add to room's subscription list
	b.subscriptions[roomID] = append(b.subscriptions[roomID], sub)

	return sub
}

// Unsubscribe removes a subscription and cleans up its resources.
//
// This method:
//   1. Removes the subscription from the room's subscriber list
//   2. Closes the cancel channel to signal cancellation
//   3. Closes the event channel (no more events will be sent)
//   4. Cleans up the room's subscription list if it becomes empty
//
// After unsubscribing, the subscription's Channel will be closed and no more events
// will be received. The client should stop reading from the channel.
//
// Parameters:
//   - sub: The subscription to remove (obtained from Subscribe)
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Idempotency:
// Safe to call multiple times with the same subscription (no-op if already removed).
//
// Example:
//
//	sub := wotan.Subscribe("general", memberID, 100)
//	// ... use subscription ...
//	wotan.Unsubscribe(sub) // Clean up when done
func (b *Wotan) Unsubscribe(sub *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, exists := b.subscriptions[sub.RoomID]
	if !exists {
		return
	}

	// Find and remove subscription from the list
	for i, s := range subs {
		if s.ID == sub.ID {
			// Signal cancellation and close channels
			close(s.cancel)
			close(s.Channel)

			// Remove from slice
			b.subscriptions[sub.RoomID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}

	// Clean up empty room subscription lists to prevent memory leaks
	if len(b.subscriptions[sub.RoomID]) == 0 {
		delete(b.subscriptions, sub.RoomID)
	}
}

// Publish broadcasts a message event to all subscribers of a room.
//
// This is the core distribution mechanism. When called, the event is sent to the
// Channel of every active subscription for the room. Sends are non-blocking:
//   - If a subscription's channel has space, the event is delivered immediately
//   - If a subscription's channel is full, the event is dropped for that subscriber
//   - If a subscription is cancelled, it's skipped
//
// This non-blocking behavior ensures that slow or disconnected clients cannot
// block message publishing for other clients.
//
// Parameters:
//   - roomID: The room to publish to
//   - event: The event to broadcast (MESSAGE_CREATED or MESSAGE_DELETED)
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Performance:
// O(n) where n = number of subscribers to the room. The actual send is non-blocking
// (uses select with default case), so this method returns quickly even with many subscribers.
//
// Example:
//
//	event := &wotan.MessageEvent{
//	    Type: wotan.EventMessageCreated,
//	    Message: msg,
//	    Timestamp: time.Now(),
//	}
//	wotan.Publish("general", event)
func (b *Wotan) Publish(roomID string, event *MessageEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, exists := b.subscriptions[roomID]
	if !exists {
		return
	}

	// Broadcast to all subscribers (non-blocking)
	for _, sub := range subs {
		select {
		case sub.Channel <- event:
			// Event sent successfully to subscriber
		case <-sub.cancel:
			// Subscription was cancelled, skip this subscriber
		default:
			// Channel buffer is full, drop event for this subscriber
			// This prevents slow clients from blocking the publisher
			// In production, this could be logged/metered for monitoring
		}
	}
}

// PublishMessageCreated publishes a message creation event to a room.
//
// This is a convenience wrapper around Publish that creates a MESSAGE_CREATED event
// with the current timestamp. All subscribers to the room will receive this event.
//
// Parameters:
//   - msg: The message that was created (must have RoomID set)
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Typical Usage:
// Call this immediately after successfully adding a message to a room's ring buffer.
//
// Example:
//
//	// After adding message to room
//	room.AddMessage(msg)
//	wotan.PublishMessageCreated(msg)
func (b *Wotan) PublishMessageCreated(msg *ringbuffer.Message) {
	event := &MessageEvent{
		Type:      EventMessageCreated,
		Message:   msg,
		Timestamp: time.Now(),
	}
	b.Publish(msg.RoomID, event)
}

// PublishMessageDeleted publishes a message deletion event to a room.
//
// This is a convenience wrapper around Publish that creates a MESSAGE_DELETED event
// with the current timestamp. All subscribers to the room will receive this event.
//
// Note: The event contains a partial Message struct with only ID and RoomID populated,
// since the full message has already been deleted from the ring buffer.
//
// Parameters:
//   - roomID: The room the message was deleted from
//   - messageID: The ID of the deleted message
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Typical Usage:
// Call this immediately after successfully deleting a message from a room's ring buffer.
//
// Example:
//
//	// After deleting message from room
//	room.DeleteMessage(messageID)
//	wotan.PublishMessageDeleted(roomID, messageID)
func (b *Wotan) PublishMessageDeleted(roomID string, messageID uuid.UUID) {
	event := &MessageEvent{
		Type: EventMessageDeleted,
		Message: &ringbuffer.Message{
			ID:     messageID,
			RoomID: roomID,
		},
		Timestamp: time.Now(),
	}
	b.Publish(roomID, event)
}

// GetSubscriptionCount returns the number of active subscriptions for a specific room.
//
// This is useful for metrics and monitoring to track room activity.
//
// Parameters:
//   - roomID: The room to check
//
// Returns:
//   - The number of active subscriptions (0 if room has no subscribers)
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Example:
//
//	count := wotan.GetSubscriptionCount("general")
//	fmt.Printf("Room 'general' has %d subscribers\n", count)
func (b *Wotan) GetSubscriptionCount(roomID string) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscriptions[roomID])
}

// GetTotalSubscriptions returns the total number of active subscriptions across all rooms.
//
// This is useful for metrics and monitoring to track overall system load.
//
// Returns:
//   - The total number of active subscriptions
//
// Thread Safety:
// This method is thread-safe and can be called concurrently.
//
// Performance:
// O(r) where r = number of rooms with subscriptions
//
// Example:
//
//	total := wotan.GetTotalSubscriptions()
//	fmt.Printf("Total active subscriptions: %d\n", total)
func (b *Wotan) GetTotalSubscriptions() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	total := 0
	for _, subs := range b.subscriptions {
		total += len(subs)
	}
	return total
}

// Close cancels all subscriptions and cleans up all resources.
//
// This method should be called when shutting down the server to ensure graceful cleanup:
//   1. All subscription cancel channels are closed (signals clients to stop)
//   2. All subscription event channels are closed (no more events)
//   3. All subscription maps are cleared (free memory)
//
// After calling Close, the wotan should not be used anymore. Any pending Publish
// calls will complete, but new Subscribe calls will behave unexpectedly.
//
// Thread Safety:
// This method is thread-safe and can be called concurrently (though typically called
// only once during shutdown).
//
// Example:
//
//	wotan := wotan.NewWotan()
//	defer wotan.Close() // Ensure cleanup on shutdown
//
//	// ... use wotan ...
func (b *Wotan) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Close all subscriptions and clean up
	for roomID, subs := range b.subscriptions {
		for _, sub := range subs {
			close(sub.cancel)
			close(sub.Channel)
		}
		delete(b.subscriptions, roomID)
	}
}
