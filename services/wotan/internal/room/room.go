// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package room

import (
	"sync"
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
	mu            sync.RWMutex
}

// Manager handles room lifecycle and operations
type Manager struct {
	mu         sync.RWMutex
	rooms      map[string]*Room // room ID -> room
	bufferSize int
}

// NewManager creates a new room manager
func NewManager(bufferSize int) *Manager {
	return &Manager{
		rooms:      make(map[string]*Room),
		bufferSize: bufferSize,
	}
}

// Create creates a new room with the given ID and name
func (m *Manager) Create(id, name string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if room already exists
	if room, exists := m.rooms[id]; exists {
		return room
	}

	room := &Room{
		ID:            id,
		Name:          name,
		CreatedAt:     time.Now(),
		Buffer:        ringbuffer.New(m.bufferSize),
		KeyValueStore: make(map[string]string),
	}

	m.rooms[id] = room
	return room
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

// GetOrCreate retrieves a room or creates it if it doesn't exist
func (m *Manager) GetOrCreate(id, name string) *Room {
	m.mu.RLock()
	room, exists := m.rooms[id]
	m.mu.RUnlock()

	if exists {
		return room
	}

	return m.Create(id, name)
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

	if _, exists := m.rooms[id]; !exists {
		return ErrRoomNotFound
	}

	delete(m.rooms, id)
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
	msg := &ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: creatorID,
		RoomID:    r.ID,
		Content:   content,
		Timestamp: time.Now(),
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
)

type RoomError struct {
	msg string
}

func (e *RoomError) Error() string {
	return e.msg
}
