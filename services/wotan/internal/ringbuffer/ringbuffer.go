// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package ringbuffer

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Message represents a single message in the ring buffer
type Message struct {
	ID        uuid.UUID `json:"id"`
	CreatorID uuid.UUID `json:"creator_id"`
	RoomID    string    `json:"room_id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// RingBuffer is a fixed-size circular buffer for messages
// When full, oldest messages are overwritten
type RingBuffer struct {
	mu       sync.RWMutex
	buffer   []*Message
	size     int
	head     int // next write position
	count    int // current number of messages
	wrapping bool
}

// New creates a new ring buffer with the specified size
func New(size int) *RingBuffer {
	if size <= 0 {
		size = 1000 // default size
	}
	return &RingBuffer{
		buffer: make([]*Message, size),
		size:   size,
		head:   0,
		count:  0,
	}
}

// Push adds a message to the ring buffer
// Returns the message ID and whether an old message was overwritten
func (rb *RingBuffer) Push(msg *Message) (uuid.UUID, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	overwritten := false
	if rb.count == rb.size {
		overwritten = true
		rb.wrapping = true
	}

	rb.buffer[rb.head] = msg
	rb.head = (rb.head + 1) % rb.size

	if rb.count < rb.size {
		rb.count++
	}

	return msg.ID, overwritten
}

// Get retrieves all messages from the ring buffer in chronological order
func (rb *RingBuffer) Get() []*Message {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return []*Message{}
	}

	result := make([]*Message, rb.count)

	if !rb.wrapping {
		// Buffer hasn't wrapped yet, just copy from start to head
		copy(result, rb.buffer[:rb.count])
	} else {
		// Buffer has wrapped, need to copy in correct order
		// Oldest message is at head position, newest is at head-1
		oldestIdx := rb.head
		for i := 0; i < rb.count; i++ {
			result[i] = rb.buffer[(oldestIdx+i)%rb.size]
		}
	}

	return result
}

// GetSince retrieves messages since a specific timestamp
func (rb *RingBuffer) GetSince(since time.Time) []*Message {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return []*Message{}
	}

	var result []*Message

	if !rb.wrapping {
		for i := 0; i < rb.count; i++ {
			if rb.buffer[i].Timestamp.After(since) {
				result = append(result, rb.buffer[i])
			}
		}
	} else {
		oldestIdx := rb.head
		for i := 0; i < rb.count; i++ {
			msg := rb.buffer[(oldestIdx+i)%rb.size]
			if msg.Timestamp.After(since) {
				result = append(result, msg)
			}
		}
	}

	return result
}

// Delete removes a message by ID if it exists
// Returns true if message was found and deleted
func (rb *RingBuffer) Delete(id uuid.UUID, creatorID uuid.UUID, content string) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.count == 0 {
		return false
	}

	// Triple verification: ID + CreatorID + Content
	if !rb.wrapping {
		for i := 0; i < rb.count; i++ {
			msg := rb.buffer[i]
			if msg != nil && msg.ID == id && msg.CreatorID == creatorID && msg.Content == content {
				// Shift messages to fill the gap
				copy(rb.buffer[i:], rb.buffer[i+1:rb.count])
				rb.buffer[rb.count-1] = nil
				rb.count--
				rb.head = rb.count
				return true
			}
		}
	} else {
		oldestIdx := rb.head
		for i := 0; i < rb.count; i++ {
			actualIdx := (oldestIdx + i) % rb.size
			msg := rb.buffer[actualIdx]
			if msg != nil && msg.ID == id && msg.CreatorID == creatorID && msg.Content == content {
				// Remove by shifting in circular buffer
				for j := i; j < rb.count-1; j++ {
					fromIdx := (oldestIdx + j + 1) % rb.size
					toIdx := (oldestIdx + j) % rb.size
					rb.buffer[toIdx] = rb.buffer[fromIdx]
				}
				lastIdx := (oldestIdx + rb.count - 1) % rb.size
				rb.buffer[lastIdx] = nil
				rb.count--
				rb.head = (rb.head - 1 + rb.size) % rb.size
				if rb.count < rb.size {
					rb.wrapping = false
				}
				return true
			}
		}
	}

	return false
}

// GetByID retrieves a specific message by ID
func (rb *RingBuffer) GetByID(id uuid.UUID) *Message {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return nil
	}

	if !rb.wrapping {
		for i := 0; i < rb.count; i++ {
			if rb.buffer[i].ID == id {
				return rb.buffer[i]
			}
		}
	} else {
		oldestIdx := rb.head
		for i := 0; i < rb.count; i++ {
			msg := rb.buffer[(oldestIdx+i)%rb.size]
			if msg.ID == id {
				return msg
			}
		}
	}

	return nil
}

// Count returns the current number of messages in the buffer
func (rb *RingBuffer) Count() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// Size returns the maximum capacity of the buffer
func (rb *RingBuffer) Size() int {
	return rb.size
}

// IsWrapping returns whether the buffer has started overwriting old messages
func (rb *RingBuffer) IsWrapping() bool {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.wrapping
}
