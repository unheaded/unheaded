// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package member

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Status represents the approval status of a member
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDenied   Status = "denied"
)

// Member represents a user in a room
type Member struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Status      Status     `json:"status"`
	RequestedAt time.Time  `json:"requested_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ApprovedAt  *time.Time `json:"approved_at,omitempty"`
	ApprovedBy  string     `json:"approved_by,omitempty"`
	RoomID      string     `json:"room_id"`
	PublicKey   string     `json:"public_key,omitempty"` // For future mTLS
}

// Manager handles member lifecycle and approval workflow
type Manager struct {
	mu      sync.RWMutex
	members map[uuid.UUID]*Member  // member ID -> member
	byRoom  map[string][]uuid.UUID // room ID -> member IDs
}

// NewManager creates a new member manager
func NewManager() *Manager {
	return &Manager{
		members: make(map[uuid.UUID]*Member),
		byRoom:  make(map[string][]uuid.UUID),
	}
}

// RequestJoin creates a new pending member request
func (m *Manager) RequestJoin(roomID, name, publicKey string, timeout time.Duration) *Member {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	expiresAt := now.Add(timeout)

	member := &Member{
		ID:          uuid.New(),
		Name:        name,
		Status:      StatusPending,
		RequestedAt: now,
		ExpiresAt:   &expiresAt,
		RoomID:      roomID,
		PublicKey:   publicKey,
	}

	m.members[member.ID] = member
	m.byRoom[roomID] = append(m.byRoom[roomID], member.ID)

	return member
}

// Approve approves a pending member
func (m *Manager) Approve(memberID uuid.UUID, approvedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	member, exists := m.members[memberID]
	if !exists {
		return ErrMemberNotFound
	}

	if member.Status != StatusPending {
		return ErrInvalidStatus
	}

	now := time.Now()
	member.Status = StatusApproved
	member.ApprovedAt = &now
	member.ApprovedBy = approvedBy

	return nil
}

// Deny denies a pending member and removes them
func (m *Manager) Deny(memberID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	member, exists := m.members[memberID]
	if !exists {
		return ErrMemberNotFound
	}

	if member.Status != StatusPending {
		return ErrInvalidStatus
	}

	// Remove from room list
	roomMembers := m.byRoom[member.RoomID]
	for i, id := range roomMembers {
		if id == memberID {
			m.byRoom[member.RoomID] = append(roomMembers[:i], roomMembers[i+1:]...)
			break
		}
	}

	// Remove member
	delete(m.members, memberID)

	return nil
}

// ApproveAll approves all pending members
func (m *Manager) ApproveAll(approvedBy string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	count := 0

	for _, member := range m.members {
		if member.Status == StatusPending {
			member.Status = StatusApproved
			member.ApprovedAt = &now
			member.ApprovedBy = approvedBy
			count++
		}
	}

	return count
}

// Get retrieves a member by ID
func (m *Manager) Get(memberID uuid.UUID) (*Member, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	member, exists := m.members[memberID]
	if !exists {
		return nil, ErrMemberNotFound
	}

	return member, nil
}

// GetByRoom retrieves all members in a room
func (m *Manager) GetByRoom(roomID string, status Status) []*Member {
	m.mu.RLock()
	defer m.mu.RUnlock()

	memberIDs := m.byRoom[roomID]
	var result []*Member

	for _, id := range memberIDs {
		member := m.members[id]
		if status == "" || member.Status == status {
			result = append(result, member)
		}
	}

	return result
}

// GetAllPending retrieves all pending members across all rooms
func (m *Manager) GetAllPending() []*Member {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Member
	for _, member := range m.members {
		if member.Status == StatusPending {
			result = append(result, member)
		}
	}

	return result
}

// IsApproved checks if a member is approved
func (m *Manager) IsApproved(memberID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	member, exists := m.members[memberID]
	if !exists {
		return false
	}

	return member.Status == StatusApproved
}

// Count returns the total number of members
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.members)
}

// CountByRoom returns the number of members in a room
func (m *Manager) CountByRoom(roomID string, status Status) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	memberIDs := m.byRoom[roomID]
	if status == "" {
		return len(memberIDs)
	}

	count := 0
	for _, id := range memberIDs {
		if m.members[id].Status == status {
			count++
		}
	}

	return count
}

// CleanupExpiredPending removes pending members whose approval timeout has expired
func (m *Manager) CleanupExpiredPending() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	var expiredIDs []uuid.UUID

	// Find all expired pending members
	for id, member := range m.members {
		if member.Status == StatusPending && member.ExpiresAt != nil && member.ExpiresAt.Before(now) {
			expiredIDs = append(expiredIDs, id)
		}
	}

	// Remove expired members
	for _, id := range expiredIDs {
		member := m.members[id]

		// Remove from room list
		roomMembers := m.byRoom[member.RoomID]
		for i, memberID := range roomMembers {
			if memberID == id {
				m.byRoom[member.RoomID] = append(roomMembers[:i], roomMembers[i+1:]...)
				break
			}
		}

		// Remove member
		delete(m.members, id)
	}

	return len(expiredIDs)
}

// Errors
var (
	ErrMemberNotFound = &MemberError{msg: "member not found"}
	ErrInvalidStatus  = &MemberError{msg: "invalid member status for this operation"}
)

type MemberError struct {
	msg string
}

func (e *MemberError) Error() string {
	return e.msg
}
