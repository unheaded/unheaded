// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ringbuffer

import (
	"sync"
	"sync/atomic"
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
	// Seq is assigned by Push: 1, 2, 3, ... per buffer, never reused, and
	// unaffected by wrap-around or deletes. It is the cursor readers page
	// with ("everything after seq N"); a position in the buffer cannot be,
	// because once the buffer is full every position is always occupied.
	Seq int64 `json:"seq"`
}

// RingBuffer is a fixed-size circular buffer for messages
// When full, oldest messages are overwritten. With a byte budget
// (SetMaxBytes), oldest messages are also dropped while the contents exceed
// it: the count alone does not bound memory, since one message can be 32 KB.
type RingBuffer struct {
	mu       sync.RWMutex
	buffer   []*Message
	size     int
	head     int // next write position
	count    int // current number of messages
	wrapping bool
	lastSeq  int64 // Seq of the most recent Push
	maxBytes int   // budget for the sum of len(Content); 0 means none
	bytes    int   // sum of len(Content) held
	// counter, when set, is shared with other buffers (NewShared): seqs are
	// then unique and ordered across all of them, not just within this one.
	counter *atomic.Int64
}

// New creates a new ring buffer with the specified size
func New(size int) *RingBuffer {
	if size <= 0 {
		size = 1000 // default size
	}
	return &RingBuffer{
		// Slots grow as messages arrive (grow), up to size. Allocating all
		// of them up front cost every room 80 KB at the default 10,000, and
		// any approved client can create rooms by naming new topics.
		buffer: make([]*Message, 0, min(size, initialSlots)),
		size:   size,
		head:   0,
		count:  0,
	}
}

// NewShared creates a ring buffer whose seqs come from counter, shared with
// other buffers: each buffer's seqs still increase, with gaps.
func NewShared(size int, counter *atomic.Int64) *RingBuffer {
	rb := New(size)
	rb.counter = counter
	return rb
}

// SetMaxBytes sets the byte budget: after each Push, oldest messages are
// dropped until the contents fit, except the newest, which is always kept.
// n <= 0 removes the budget. Takes effect on the next Push.
func (rb *RingBuffer) SetMaxBytes(n int) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if n < 0 {
		n = 0
	}
	rb.maxBytes = n
}

// Bytes returns the sum of len(Content) over the messages held.
func (rb *RingBuffer) Bytes() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.bytes
}

// oldest is the position of the oldest message. Caller holds rb.mu.
func (rb *RingBuffer) oldest() int {
	return (rb.head - rb.count + rb.size) % rb.size
}

// Push adds a message to the ring buffer
// Returns the message ID and whether an old message was overwritten (or
// dropped for the byte budget)
func (rb *RingBuffer) Push(msg *Message) (uuid.UUID, bool) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	overwritten := false
	if rb.count == rb.size {
		overwritten = true
		rb.wrapping = true
		rb.bytes -= len(rb.buffer[rb.head].Content)
	}

	// Assigned and stored under rb.mu: a reader that sees a shared counter
	// value N and then locks this buffer finds every seq <= N already here.
	if rb.counter != nil {
		rb.lastSeq = rb.counter.Add(1)
	} else {
		rb.lastSeq++
	}
	msg.Seq = rb.lastSeq
	if rb.head == len(rb.buffer) {
		rb.grow()
	}
	rb.buffer[rb.head] = msg
	rb.bytes += len(msg.Content)
	rb.head = (rb.head + 1) % rb.size

	if rb.count < rb.size {
		rb.count++
	}

	for rb.maxBytes > 0 && rb.bytes > rb.maxBytes && rb.count > 1 {
		i := rb.oldest()
		rb.bytes -= len(rb.buffer[i].Content)
		rb.buffer[i] = nil
		rb.count--
		overwritten = true
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
	oldestIdx := rb.oldest()
	for i := 0; i < rb.count; i++ {
		result[i] = rb.buffer[(oldestIdx+i)%rb.size]
	}

	return result
}

// GetAfter returns, oldest first, up to limit messages whose Seq is greater
// than after. limit <= 0 means no limit. A reader that fell behind by more
// than the buffer holds gets the oldest retained message next: the gap is
// visible to it as a jump in Seq.
func (rb *RingBuffer) GetAfter(after int64, limit int) []*Message {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := []*Message{}
	start := rb.oldest()
	for i := 0; i < rb.count; i++ {
		msg := rb.buffer[(start+i)%rb.size]
		if msg.Seq <= after {
			continue
		}
		result = append(result, msg)
		if limit > 0 && len(result) == limit {
			break
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
	oldestIdx := rb.oldest()
	for i := 0; i < rb.count; i++ {
		msg := rb.buffer[(oldestIdx+i)%rb.size]
		if msg.Timestamp.After(since) {
			result = append(result, msg)
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
	oldestIdx := rb.oldest()
	for i := 0; i < rb.count; i++ {
		actualIdx := (oldestIdx + i) % rb.size
		msg := rb.buffer[actualIdx]
		if msg != nil && msg.ID == id && msg.CreatorID == creatorID && msg.Content == content {
			// Remove by shifting the newer messages back one place
			for j := i; j < rb.count-1; j++ {
				fromIdx := (oldestIdx + j + 1) % rb.size
				toIdx := (oldestIdx + j) % rb.size
				rb.buffer[toIdx] = rb.buffer[fromIdx]
			}
			lastIdx := (oldestIdx + rb.count - 1) % rb.size
			rb.buffer[lastIdx] = nil
			rb.bytes -= len(msg.Content)
			rb.count--
			rb.head = (rb.head - 1 + rb.size) % rb.size
			if rb.count < rb.size {
				rb.wrapping = false
			}
			return true
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

	oldestIdx := rb.oldest()
	for i := 0; i < rb.count; i++ {
		msg := rb.buffer[(oldestIdx+i)%rb.size]
		if msg.ID == id {
			return msg
		}
	}

	return nil
}

// LastSeq returns the Seq of the most recent Push, 0 if none. Seqs live in
// memory, so a restarted Wotan counts from 1 again; a reader whose cursor is
// above LastSeq is looking at a restarted server and must rewind.
func (rb *RingBuffer) LastSeq() int64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.lastSeq
}

// Count returns the current number of messages in the buffer
// initialSlots is a new buffer's first allocation.
const initialSlots = 16

// grow adds one slot, doubling the backing array (never past size). It is
// only called while the buffer has not yet wrapped: head can reach size
// only once all size slots exist, so every (oldest+i)%size index below
// count stays inside the slice.
func (rb *RingBuffer) grow() {
	if cap(rb.buffer) == len(rb.buffer) {
		n := min(rb.size, max(initialSlots, 2*cap(rb.buffer)))
		b := make([]*Message, len(rb.buffer), n)
		copy(b, rb.buffer)
		rb.buffer = b
	}
	rb.buffer = rb.buffer[:len(rb.buffer)+1]
}

// MessageOverhead is what a held message costs beyond its Content
// (Message struct, IDs, its slot). Measured 2026-10-19: 500,000 messages of
// ~440 bytes held 210 MiB of content in 290 MiB of heap, ~160 bytes each;
// rounded up. The total budget (room.Manager) counts it.
const MessageOverhead = 192

// Footprint is the memory the held messages take: their content plus
// MessageOverhead each.
func (rb *RingBuffer) Footprint() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.bytes + rb.count*MessageOverhead
}

// EvictOldest drops the oldest message and returns the footprint it held;
// 0 when the buffer is empty.
func (rb *RingBuffer) EvictOldest() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.count == 0 {
		return 0
	}
	i := rb.oldest()
	n := len(rb.buffer[i].Content)
	rb.bytes -= n
	rb.buffer[i] = nil
	rb.count--
	return n + MessageOverhead
}

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
