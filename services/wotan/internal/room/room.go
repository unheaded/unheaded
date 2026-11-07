// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package room

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"unheaded/services/wotan/internal/ringbuffer"
)

// Room represents an isolated message space with member access control
type Room struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	CreatedAt     time.Time              `json:"created_at"`
	Buffer        *ringbuffer.RingBuffer `json:"-"`
	KeyValueStore map[string]string      `json:"kv_store"` // Secure key-value pairs
	creator       string                 // the verified caller it counts against; "" for none
	mu            sync.RWMutex
}

// Manager handles room lifecycle and operations
type Manager struct {
	mu         sync.RWMutex
	rooms      map[string]*Room // room ID -> room
	bufferSize int
	// bufferBytes is each room's byte budget (RingBuffer.SetMaxBytes); 0
	// means none. Count alone does not bound memory: a trace batch is ~32 KB,
	// so 10000 of them is 320 MB in one room.
	bufferBytes int
	// totalBytes is the budget for all rooms' footprints together
	// (EnforceTotal); 0 means none.
	totalBytes int
	// maxRooms caps how many rooms exist (SetMaxRooms); 0 means none.
	maxRooms int
	// roomQuota caps how many rooms one verified caller may have made
	// (SetRoomQuota); 0 means none. made counts them by caller.
	roomQuota int
	made      map[string]int
	// keep names rooms EnforceTotal never evicts from (signed config.*:
	// small, and losing one silently is worse than holding it).
	keep func(roomID string) bool
	// seq numbers messages across every room, so a cursor taken over several
	// rooms (a wildcard read) orders them. In memory: a restart begins at 1.
	seq atomic.Int64
}

// LastSeq is the most recently assigned seq in any room, 0 if none. A
// cursor above it is from before a restart.
func (m *Manager) LastSeq() int64 {
	return m.seq.Load()
}

// NewManager creates a new room manager
func NewManager(bufferSize int) *Manager {
	return &Manager{
		rooms:      make(map[string]*Room),
		made:       make(map[string]int),
		bufferSize: bufferSize,
	}
}

// SetBufferBytes sets the byte budget for rooms created from now on.
func (m *Manager) SetBufferBytes(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bufferBytes = n
}

// SetTotalBytes sets the budget for all rooms together (EnforceTotal); 0
// means none. The per-room budget bounds one topic; this bounds how many
// topics Wotan can hold at once. With only per-room budgets, ~50 log rooms
// of 10,000 small messages each reached 290 MiB of heap (2026-10-19), past
// the 256 MiB container limit, while every room was within its own budget.
func (m *Manager) SetTotalBytes(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n < 0 {
		n = 0
	}
	m.totalBytes = n
}

// SetKeep names rooms EnforceTotal must not evict from. They still count
// toward the total.
func (m *Manager) SetKeep(keep func(roomID string) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keep = keep
}

// EnforceTotal drops the oldest messages of the largest rooms until the
// rooms' combined footprint (ringbuffer.Footprint) is within the total
// budget, skipping rooms SetKeep protects. It takes from the largest room
// until it is no larger than the next one (or the total fits), then
// rescans, so the work is per room, not per message. It returns the
// combined footprint afterwards and how many messages it dropped.
func (m *Manager) EnforceTotal() (total, dropped int) {
	m.mu.RLock()
	budget, keep := m.totalBytes, m.keep
	rooms := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	m.mu.RUnlock()

	sizes := make([]int, len(rooms))
	evictable := make([]bool, len(rooms))
	for i, r := range rooms {
		sizes[i] = r.Buffer.Footprint()
		total += sizes[i]
		evictable[i] = keep == nil || !keep(r.ID)
	}
	for budget > 0 && total > budget {
		big, next := -1, 0
		for i := range sizes {
			if !evictable[i] || sizes[i] == 0 {
				continue
			}
			if big < 0 || sizes[i] > sizes[big] {
				if big >= 0 {
					next = max(next, sizes[big])
				}
				big = i
			} else {
				next = max(next, sizes[i])
			}
		}
		if big < 0 {
			break // only protected or empty rooms left
		}
		for total > budget && sizes[big] > 0 && sizes[big] >= next {
			freed := rooms[big].Buffer.EvictOldest()
			if freed == 0 {
				total -= sizes[big] // emptied meanwhile
				sizes[big] = 0
				break
			}
			sizes[big] -= freed
			total -= freed
			dropped++
		}
	}
	return total, dropped
}

// SetMaxRooms caps the number of rooms (0 = none). Publishers mint a room
// by naming a topic, and rooms are never reaped, so without a cap any
// auto-approved caller could grow Wotan without bound.
func (m *Manager) SetMaxRooms(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxRooms = n
}

// SetRoomQuota caps the rooms one verified caller may have made (0 = none).
// SetMaxRooms alone lets one caller spend the whole cap by naming topics, and
// rooms are never reaped, so everyone else is then refused until a restart.
func (m *Manager) SetRoomQuota(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roomQuota = n
}

// Create creates a new room with the given ID and name, or returns the
// existing one. Past SetMaxRooms it refuses a new room (ErrTooManyRooms).
func (m *Manager) Create(id, name string) (*Room, error) {
	return m.CreateAs("", id, name)
}

// CreateAs is Create on behalf of a verified caller, who is also held to
// SetRoomQuota (ErrRoomQuota). A room that exists costs nothing. With no
// caller only the cap applies: a self-reported name proves nothing.
func (m *Manager) CreateAs(creator, id, name string) (*Room, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if room already exists
	if room, exists := m.rooms[id]; exists {
		return room, nil
	}
	if m.maxRooms > 0 && len(m.rooms) >= m.maxRooms {
		return nil, ErrTooManyRooms
	}
	if creator != "" && m.roomQuota > 0 && m.made[creator] >= m.roomQuota {
		return nil, ErrRoomQuota
	}

	room := &Room{
		ID:            id,
		Name:          name,
		CreatedAt:     time.Now(),
		Buffer:        ringbuffer.NewShared(m.bufferSize, &m.seq),
		KeyValueStore: make(map[string]string),
		creator:       creator,
	}
	room.Buffer.SetMaxBytes(m.bufferBytes)

	m.rooms[id] = room
	if creator != "" {
		m.made[creator]++
	}
	return room, nil
}

// Get retrieves a room by ID
func (m *Manager) Get(id string) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	room, exists := m.rooms[id]
	if !exists {
		return nil, ErrRoomNotFound
	}

	return room, nil
}

// GetOrCreate retrieves a room or creates it if it doesn't exist (within
// SetMaxRooms; see Create).
func (m *Manager) GetOrCreate(id, name string) (*Room, error) {
	return m.GetOrCreateAs("", id, name)
}

// GetOrCreateAs is GetOrCreate on behalf of a verified caller (see CreateAs).
func (m *Manager) GetOrCreateAs(creator, id, name string) (*Room, error) {
	m.mu.RLock()
	room, exists := m.rooms[id]
	m.mu.RUnlock()

	if exists {
		return room, nil
	}

	return m.CreateAs(creator, id, name)
}

// List returns all rooms
func (m *Manager) List() []*Room {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rooms := make([]*Room, 0, len(m.rooms))
	for _, room := range m.rooms {
		rooms = append(rooms, room)
	}

	return rooms
}

// Delete removes a room
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	room, exists := m.rooms[id]
	if !exists {
		return ErrRoomNotFound
	}

	delete(m.rooms, id)
	if room.creator != "" {
		if m.made[room.creator]--; m.made[room.creator] <= 0 {
			delete(m.made, room.creator)
		}
	}
	return nil
}

// Count returns the total number of rooms
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rooms)
}

// Room methods

// SendMessage adds a message to the room's ring buffer
func (r *Room) SendMessage(creatorID uuid.UUID, content string) (*ringbuffer.Message, bool) {
	return r.SendVerifiedMessage(creatorID, "", content, nil)
}

// SendSignedMessage is SendMessage for a message whose signature was
// verified: the signature is stored with it and sent with every event for
// it (ADR-105).
func (r *Room) SendSignedMessage(creatorID uuid.UUID, content string, sig *ringbuffer.Signature) (*ringbuffer.Message, bool) {
	return r.SendVerifiedMessage(creatorID, "", content, sig)
}

// SendVerifiedMessage is SendMessage for a publisher whose token Wotan
// verified: subject goes out with every event for the message, and only
// Wotan can set it. sig is the verified signature of a signed message, or
// nil. An empty subject means nothing was verified.
func (r *Room) SendVerifiedMessage(creatorID uuid.UUID, subject, content string, sig *ringbuffer.Signature) (*ringbuffer.Message, bool) {
	msg := &ringbuffer.Message{
		ID:             uuid.New(),
		CreatorID:      creatorID,
		RoomID:         r.ID,
		Content:        content,
		Timestamp:      time.Now(),
		Signed:         sig,
		VerifiedSender: subject,
	}
	_, overwritten := r.Buffer.Push(msg)
	return msg, overwritten
}

// GetMessages retrieves all messages from the room
func (r *Room) GetMessages() []*ringbuffer.Message {
	return r.Buffer.Get()
}

// GetMessagesSince retrieves messages since a specific timestamp
func (r *Room) GetMessagesSince(since time.Time) []*ringbuffer.Message {
	return r.Buffer.GetSince(since)
}

// DeleteMessage removes a message from the room (with triple verification)
func (r *Room) DeleteMessage(id, creatorID uuid.UUID, content string) bool {
	return r.Buffer.Delete(id, creatorID, content)
}

// SetKV stores a key-value pair in the room
func (r *Room) SetKV(key, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.KeyValueStore[key] = value
}

// GetKV retrieves a value by key from the room
func (r *Room) GetKV(key string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, exists := r.KeyValueStore[key]
	return value, exists
}

// DeleteKV removes a key-value pair from the room
func (r *Room) DeleteKV(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.KeyValueStore, key)
}

// GetAllKV returns all key-value pairs in the room
func (r *Room) GetAllKV() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external mutation
	kv := make(map[string]string, len(r.KeyValueStore))
	for k, v := range r.KeyValueStore {
		kv[k] = v
	}
	return kv
}

// Stats returns statistics about the room
func (r *Room) Stats() map[string]interface{} {
	return map[string]interface{}{
		"id":              r.ID,
		"name":            r.Name,
		"created_at":      r.CreatedAt,
		"message_count":   r.Buffer.Count(),
		"buffer_size":     r.Buffer.Size(),
		"buffer_wrapping": r.Buffer.IsWrapping(),
		"kv_count":        len(r.KeyValueStore),
	}
}

// Errors
var (
	ErrRoomNotFound = &RoomError{msg: "room not found"}
	// ErrTooManyRooms: creating the room would pass SetMaxRooms.
	ErrTooManyRooms = &RoomError{msg: "too many rooms"}
	// ErrRoomQuota: this caller has made as many rooms as SetRoomQuota allows.
	ErrRoomQuota = &RoomError{msg: "room quota reached for this caller"}
)

type RoomError struct {
	msg string
}

func (e *RoomError) Error() string {
	return e.msg
}
