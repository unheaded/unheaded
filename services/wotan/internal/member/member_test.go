// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package member

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestNewManager verifies manager initialization
func TestNewManager(t *testing.T) {
	mgr := NewManager()

	if mgr == nil {
		t.Fatal("NewManager() returned nil")
	}
	if mgr.Count() != 0 {
		t.Errorf("Count() = %d, want 0", mgr.Count())
	}
	if mgr.members == nil {
		t.Error("members map is nil")
	}
	if mgr.byRoom == nil {
		t.Error("byRoom map is nil")
	}
}

// TestRequestJoin tests creating join requests
func TestRequestJoin(t *testing.T) {
	mgr := NewManager()
	timeout := 5 * time.Minute

	member := mgr.RequestJoin("room1", "Alice", "pubkey123", timeout)

	if member == nil {
		t.Fatal("RequestJoin() returned nil")
	}
	if member.ID == uuid.Nil {
		t.Error("Member.ID is nil UUID")
	}
	if member.Name != "Alice" {
		t.Errorf("Member.Name = %s, want 'Alice'", member.Name)
	}
	if member.Status != StatusPending {
		t.Errorf("Member.Status = %s, want %s", member.Status, StatusPending)
	}
	if member.RoomID != "room1" {
		t.Errorf("Member.RoomID = %s, want 'room1'", member.RoomID)
	}
	if member.PublicKey != "pubkey123" {
		t.Errorf("Member.PublicKey = %s, want 'pubkey123'", member.PublicKey)
	}
	if member.RequestedAt.IsZero() {
		t.Error("Member.RequestedAt is zero")
	}
	if member.ExpiresAt == nil {
		t.Fatal("Member.ExpiresAt is nil")
	}
	if member.ApprovedAt != nil {
		t.Error("Member.ApprovedAt should be nil for pending member")
	}
	if member.ApprovedBy != "" {
		t.Error("Member.ApprovedBy should be empty for pending member")
	}

	// Verify timeout
	expectedExpiry := member.RequestedAt.Add(timeout)
	if !member.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("Member.ExpiresAt = %v, want %v", member.ExpiresAt, expectedExpiry)
	}

	// Verify member is in manager
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}
}

// TestRequestJoin_MultipleRooms tests members across different rooms
func TestRequestJoin_MultipleRooms(t *testing.T) {
	mgr := NewManager()

	member1 := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)
	member2 := mgr.RequestJoin("room2", "Bob", "", 5*time.Minute)
	member3 := mgr.RequestJoin("room1", "Charlie", "", 5*time.Minute)

	if mgr.Count() != 3 {
		t.Errorf("Count() = %d, want 3", mgr.Count())
	}

	// Verify room1 has 2 members
	room1Members := mgr.GetByRoom("room1", "")
	if len(room1Members) != 2 {
		t.Errorf("room1 has %d members, want 2", len(room1Members))
	}

	// Verify room2 has 1 member
	room2Members := mgr.GetByRoom("room2", "")
	if len(room2Members) != 1 {
		t.Errorf("room2 has %d members, want 1", len(room2Members))
	}

	// Verify member IDs are unique
	if member1.ID == member2.ID || member1.ID == member3.ID || member2.ID == member3.ID {
		t.Error("Member IDs are not unique")
	}
}

// TestApprove tests approving pending members
func TestApprove(t *testing.T) {
	mgr := NewManager()
	member := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)

	// Approve the member
	err := mgr.Approve(member.ID, "admin")

	if err != nil {
		t.Fatalf("Approve() returned error: %v", err)
	}

	// Verify status changed
	retrieved, _ := mgr.Get(member.ID)
	if retrieved.Status != StatusApproved {
		t.Errorf("Member.Status = %s, want %s", retrieved.Status, StatusApproved)
	}
	if retrieved.ApprovedAt == nil {
		t.Fatal("Member.ApprovedAt is nil after approval")
	}
	if retrieved.ApprovedBy != "admin" {
		t.Errorf("Member.ApprovedBy = %s, want 'admin'", retrieved.ApprovedBy)
	}
	if retrieved.ApprovedAt.IsZero() {
		t.Error("Member.ApprovedAt is zero after approval")
	}
}

// TestApprove_NonExistentMember tests approving non-existent member
func TestApprove_NonExistentMember(t *testing.T) {
	mgr := NewManager()

	err := mgr.Approve(uuid.New(), "admin")

	if err == nil {
		t.Error("Approve() returned nil error for non-existent member")
	}
	if err != ErrMemberNotFound {
		t.Errorf("Approve() error = %v, want ErrMemberNotFound", err)
	}
}

// TestApprove_AlreadyApproved tests approving already approved member
func TestApprove_AlreadyApproved(t *testing.T) {
	mgr := NewManager()
	member := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)

	// Approve once
	mgr.Approve(member.ID, "admin1")

	// Try to approve again
	err := mgr.Approve(member.ID, "admin2")

	if err == nil {
		t.Error("Approve() returned nil error for already approved member")
	}
	if err != ErrInvalidStatus {
		t.Errorf("Approve() error = %v, want ErrInvalidStatus", err)
	}

	// Verify approvedBy didn't change
	retrieved, _ := mgr.Get(member.ID)
	if retrieved.ApprovedBy != "admin1" {
		t.Errorf("ApprovedBy changed to %s, should remain 'admin1'", retrieved.ApprovedBy)
	}
}

// TestDeny tests denying pending members
func TestDeny(t *testing.T) {
	mgr := NewManager()
	member := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)

	// Deny the member
	err := mgr.Deny(member.ID)

	if err != nil {
		t.Fatalf("Deny() returned error: %v", err)
	}

	// Verify member is removed
	if mgr.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after denial", mgr.Count())
	}

	// Verify Get returns error
	_, err = mgr.Get(member.ID)
	if err == nil {
		t.Error("Get() found denied member")
	}

	// Verify removed from room list
	roomMembers := mgr.GetByRoom("room1", "")
	if len(roomMembers) != 0 {
		t.Errorf("room1 has %d members, want 0 after denial", len(roomMembers))
	}
}

// TestDeny_NonExistentMember tests denying non-existent member
func TestDeny_NonExistentMember(t *testing.T) {
	mgr := NewManager()

	err := mgr.Deny(uuid.New())

	if err == nil {
		t.Error("Deny() returned nil error for non-existent member")
	}
	if err != ErrMemberNotFound {
		t.Errorf("Deny() error = %v, want ErrMemberNotFound", err)
	}
}

// TestDeny_AlreadyApproved tests denying already approved member
func TestDeny_AlreadyApproved(t *testing.T) {
	mgr := NewManager()
	member := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)

	// Approve member
	mgr.Approve(member.ID, "admin")

	// Try to deny
	err := mgr.Deny(member.ID)

	if err == nil {
		t.Error("Deny() returned nil error for approved member")
	}
	if err != ErrInvalidStatus {
		t.Errorf("Deny() error = %v, want ErrInvalidStatus", err)
	}

	// Verify member still exists
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (member should not be removed)", mgr.Count())
	}
}

// TestDeny_MultipleMembers tests denying one of multiple members
func TestDeny_MultipleMembers(t *testing.T) {
	mgr := NewManager()
	member1 := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)
	member2 := mgr.RequestJoin("room1", "Bob", "", 5*time.Minute)
	member3 := mgr.RequestJoin("room1", "Charlie", "", 5*time.Minute)

	// Deny middle member
	err := mgr.Deny(member2.ID)
	if err != nil {
		t.Fatalf("Deny() returned error: %v", err)
	}

	if mgr.Count() != 2 {
		t.Errorf("Count() = %d, want 2", mgr.Count())
	}

	// Verify correct members remain
	roomMembers := mgr.GetByRoom("room1", "")
	if len(roomMembers) != 2 {
		t.Fatalf("room1 has %d members, want 2", len(roomMembers))
	}

	memberIDs := make(map[uuid.UUID]bool)
	for _, m := range roomMembers {
		memberIDs[m.ID] = true
	}

	if !memberIDs[member1.ID] || !memberIDs[member3.ID] {
		t.Error("Wrong members remained after denial")
	}
	if memberIDs[member2.ID] {
		t.Error("Denied member still in room list")
	}
}

// TestApproveAll tests bulk approval of pending members
func TestApproveAll(t *testing.T) {
	mgr := NewManager()

	// Create multiple pending members
	member1 := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)
	member2 := mgr.RequestJoin("room2", "Bob", "", 5*time.Minute)
	member3 := mgr.RequestJoin("room1", "Charlie", "", 5*time.Minute)

	// Approve one manually first
	mgr.Approve(member1.ID, "admin1")

	// ApproveAll should only approve pending members
	count := mgr.ApproveAll("admin2")

	if count != 2 {
		t.Errorf("ApproveAll() returned %d, want 2 (only pending members)", count)
	}

	// Verify all members are approved
	for _, id := range []uuid.UUID{member1.ID, member2.ID, member3.ID} {
		m, _ := mgr.Get(id)
		if m.Status != StatusApproved {
			t.Errorf("Member %s status = %s, want approved", m.Name, m.Status)
		}
	}

	// Verify first member kept original approver
	m1, _ := mgr.Get(member1.ID)
	if m1.ApprovedBy != "admin1" {
		t.Errorf("Member1.ApprovedBy = %s, want 'admin1'", m1.ApprovedBy)
	}

	// Verify bulk approved members have correct approver
	m2, _ := mgr.Get(member2.ID)
	if m2.ApprovedBy != "admin2" {
		t.Errorf("Member2.ApprovedBy = %s, want 'admin2'", m2.ApprovedBy)
	}
}

// TestApproveAll_NoPendingMembers tests ApproveAll with no pending members
func TestApproveAll_NoPendingMembers(t *testing.T) {
	mgr := NewManager()

	count := mgr.ApproveAll("admin")

	if count != 0 {
		t.Errorf("ApproveAll() returned %d, want 0", count)
	}

	// Create and approve a member
	member := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)
	mgr.Approve(member.ID, "admin1")

	// ApproveAll should return 0
	count = mgr.ApproveAll("admin2")
	if count != 0 {
		t.Errorf("ApproveAll() returned %d, want 0 (no pending members)", count)
	}
}

// TestGet tests retrieving members by ID
func TestGet(t *testing.T) {
	mgr := NewManager()
	created := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)

	// Get existing member
	member, err := mgr.Get(created.ID)

	if err != nil {
		t.Fatalf("Get() returned error: %v", err)
	}
	if member.ID != created.ID {
		t.Error("Get() returned different member")
	}

	// Get non-existent member
	_, err = mgr.Get(uuid.New())
	if err == nil {
		t.Error("Get() returned nil error for non-existent member")
	}
	if err != ErrMemberNotFound {
		t.Errorf("Get() error = %v, want ErrMemberNotFound", err)
	}
}

// TestGetByRoom tests retrieving members by room
func TestGetByRoom(t *testing.T) {
	mgr := NewManager()

	member1 := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)
	member2 := mgr.RequestJoin("room1", "Bob", "", 5*time.Minute)
	mgr.RequestJoin("room2", "Charlie", "", 5*time.Minute)

	// Approve one member
	mgr.Approve(member1.ID, "admin")

	// Get all members in room1
	members := mgr.GetByRoom("room1", "")
	if len(members) != 2 {
		t.Errorf("GetByRoom('room1', '') returned %d members, want 2", len(members))
	}

	// Get only pending members in room1
	pendingMembers := mgr.GetByRoom("room1", StatusPending)
	if len(pendingMembers) != 1 {
		t.Fatalf("GetByRoom('room1', pending) returned %d members, want 1", len(pendingMembers))
	}
	if pendingMembers[0].ID != member2.ID {
		t.Error("GetByRoom returned wrong pending member")
	}

	// Get only approved members in room1
	approvedMembers := mgr.GetByRoom("room1", StatusApproved)
	if len(approvedMembers) != 1 {
		t.Fatalf("GetByRoom('room1', approved) returned %d members, want 1", len(approvedMembers))
	}
	if approvedMembers[0].ID != member1.ID {
		t.Error("GetByRoom returned wrong approved member")
	}

	// Get members from empty room
	emptyRoom := mgr.GetByRoom("nonexistent", "")
	if len(emptyRoom) != 0 {
		t.Errorf("GetByRoom('nonexistent', '') returned %d members, want 0", len(emptyRoom))
	}
}

// TestGetAllPending tests retrieving all pending members
func TestGetAllPending(t *testing.T) {
	mgr := NewManager()

	// Empty manager
	pending := mgr.GetAllPending()
	if len(pending) != 0 {
		t.Errorf("GetAllPending() returned %d members, want 0", len(pending))
	}

	// Create multiple members across rooms
	member1 := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)
	member2 := mgr.RequestJoin("room2", "Bob", "", 5*time.Minute)
	member3 := mgr.RequestJoin("room3", "Charlie", "", 5*time.Minute)

	// Approve one
	mgr.Approve(member1.ID, "admin")

	// Get all pending
	pending = mgr.GetAllPending()
	if len(pending) != 2 {
		t.Errorf("GetAllPending() returned %d members, want 2", len(pending))
	}

	// Verify correct members are returned
	pendingIDs := make(map[uuid.UUID]bool)
	for _, m := range pending {
		pendingIDs[m.ID] = true
		if m.Status != StatusPending {
			t.Errorf("GetAllPending() returned member with status %s", m.Status)
		}
	}

	if !pendingIDs[member2.ID] || !pendingIDs[member3.ID] {
		t.Error("GetAllPending() returned wrong members")
	}
	if pendingIDs[member1.ID] {
		t.Error("GetAllPending() returned approved member")
	}
}

// TestIsApproved tests checking approval status
func TestIsApproved(t *testing.T) {
	mgr := NewManager()

	member := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)

	// Pending member should not be approved
	if mgr.IsApproved(member.ID) {
		t.Error("IsApproved() = true for pending member, want false")
	}

	// Approve member
	mgr.Approve(member.ID, "admin")

	// Should now be approved
	if !mgr.IsApproved(member.ID) {
		t.Error("IsApproved() = false for approved member, want true")
	}

	// Non-existent member should return false
	if mgr.IsApproved(uuid.New()) {
		t.Error("IsApproved() = true for non-existent member, want false")
	}
}

// TestCount tests counting total members
func TestCount(t *testing.T) {
	mgr := NewManager()

	if mgr.Count() != 0 {
		t.Errorf("Initial Count() = %d, want 0", mgr.Count())
	}

	mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}

	mgr.RequestJoin("room1", "Bob", "", 5*time.Minute)
	mgr.RequestJoin("room2", "Charlie", "", 5*time.Minute)
	if mgr.Count() != 3 {
		t.Errorf("Count() = %d, want 3", mgr.Count())
	}

	// Deny one member
	members := mgr.GetByRoom("room1", "")
	mgr.Deny(members[0].ID)
	if mgr.Count() != 2 {
		t.Errorf("Count() = %d, want 2 after denial", mgr.Count())
	}
}

// TestCountByRoom tests counting members per room
func TestCountByRoom(t *testing.T) {
	mgr := NewManager()

	member1 := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)
	mgr.RequestJoin("room1", "Bob", "", 5*time.Minute)
	mgr.RequestJoin("room2", "Charlie", "", 5*time.Minute)

	// Count all members in room1
	count := mgr.CountByRoom("room1", "")
	if count != 2 {
		t.Errorf("CountByRoom('room1', '') = %d, want 2", count)
	}

	// Approve one member in room1
	mgr.Approve(member1.ID, "admin")

	// Count pending members in room1
	pendingCount := mgr.CountByRoom("room1", StatusPending)
	if pendingCount != 1 {
		t.Errorf("CountByRoom('room1', pending) = %d, want 1", pendingCount)
	}

	// Count approved members in room1
	approvedCount := mgr.CountByRoom("room1", StatusApproved)
	if approvedCount != 1 {
		t.Errorf("CountByRoom('room1', approved) = %d, want 1", approvedCount)
	}

	// Count in non-existent room
	emptyCount := mgr.CountByRoom("nonexistent", "")
	if emptyCount != 0 {
		t.Errorf("CountByRoom('nonexistent', '') = %d, want 0", emptyCount)
	}
}

// TestCleanupExpiredPending tests removing expired members
func TestCleanupExpiredPending(t *testing.T) {
	mgr := NewManager()

	// Create member with very short timeout
	member1 := mgr.RequestJoin("room1", "Alice", "", 1*time.Millisecond)
	// Create member with long timeout
	member2 := mgr.RequestJoin("room1", "Bob", "", 1*time.Hour)

	// Wait for first member to expire
	time.Sleep(10 * time.Millisecond)

	// Cleanup expired members
	count := mgr.CleanupExpiredPending()

	if count != 1 {
		t.Errorf("CleanupExpiredPending() = %d, want 1", count)
	}

	// Verify member1 is removed
	_, err := mgr.Get(member1.ID)
	if err == nil {
		t.Error("Get() found expired member")
	}

	// Verify member2 still exists
	_, err = mgr.Get(member2.ID)
	if err != nil {
		t.Error("Get() didn't find non-expired member")
	}

	// Verify total count
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1 after cleanup", mgr.Count())
	}

	// Verify room list updated
	roomMembers := mgr.GetByRoom("room1", "")
	if len(roomMembers) != 1 {
		t.Errorf("room1 has %d members, want 1 after cleanup", len(roomMembers))
	}
	if roomMembers[0].ID != member2.ID {
		t.Error("Wrong member remained after cleanup")
	}
}

// TestCleanupExpiredPending_NoExpired tests cleanup with no expired members
func TestCleanupExpiredPending_NoExpired(t *testing.T) {
	mgr := NewManager()

	mgr.RequestJoin("room1", "Alice", "", 1*time.Hour)
	mgr.RequestJoin("room1", "Bob", "", 1*time.Hour)

	count := mgr.CleanupExpiredPending()

	if count != 0 {
		t.Errorf("CleanupExpiredPending() = %d, want 0", count)
	}
	if mgr.Count() != 2 {
		t.Errorf("Count() = %d, want 2 (no members should be removed)", mgr.Count())
	}
}

// TestCleanupExpiredPending_ApprovedNotRemoved tests approved members aren't cleaned
func TestCleanupExpiredPending_ApprovedNotRemoved(t *testing.T) {
	mgr := NewManager()

	member := mgr.RequestJoin("room1", "Alice", "", 1*time.Millisecond)
	mgr.Approve(member.ID, "admin")

	// Wait for what would be expiry
	time.Sleep(10 * time.Millisecond)

	count := mgr.CleanupExpiredPending()

	if count != 0 {
		t.Errorf("CleanupExpiredPending() = %d, want 0 (approved members shouldn't be removed)", count)
	}
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}
}

// TestCleanupExpiredPending_MultipleRooms tests cleanup across multiple rooms
func TestCleanupExpiredPending_MultipleRooms(t *testing.T) {
	mgr := NewManager()

	// Create expired members in different rooms
	mgr.RequestJoin("room1", "Alice", "", 1*time.Millisecond)
	mgr.RequestJoin("room2", "Bob", "", 1*time.Millisecond)
	// Create non-expired member
	member3 := mgr.RequestJoin("room3", "Charlie", "", 1*time.Hour)

	time.Sleep(10 * time.Millisecond)

	count := mgr.CleanupExpiredPending()

	if count != 2 {
		t.Errorf("CleanupExpiredPending() = %d, want 2", count)
	}
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}

	// Verify correct member remains
	m, _ := mgr.Get(member3.ID)
	if m.Name != "Charlie" {
		t.Error("Wrong member remained after cleanup")
	}
}

// TestMemberError tests error type
func TestMemberError(t *testing.T) {
	if ErrMemberNotFound == nil {
		t.Fatal("ErrMemberNotFound is nil")
	}
	if ErrMemberNotFound.Error() != "member not found" {
		t.Errorf("ErrMemberNotFound.Error() = %s, want 'member not found'", ErrMemberNotFound.Error())
	}

	if ErrInvalidStatus == nil {
		t.Fatal("ErrInvalidStatus is nil")
	}
	if ErrInvalidStatus.Error() != "invalid member status for this operation" {
		t.Errorf("ErrInvalidStatus.Error() = %s, want 'invalid member status for this operation'", ErrInvalidStatus.Error())
	}
}

// TestConcurrent_RequestJoin tests concurrent join requests
func TestConcurrent_RequestJoin(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup

	// Concurrent join requests to same room
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			mgr.RequestJoin("room1", "User", "", 5*time.Minute)
		}(i)
	}

	wg.Wait()

	if mgr.Count() != 100 {
		t.Errorf("Count() = %d, want 100", mgr.Count())
	}

	members := mgr.GetByRoom("room1", "")
	if len(members) != 100 {
		t.Errorf("room1 has %d members, want 100", len(members))
	}
}

// TestConcurrent_ApproveAndGet tests concurrent approve and get operations
func TestConcurrent_ApproveAndGet(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup

	// Create members
	memberIDs := make([]uuid.UUID, 50)
	for i := 0; i < 50; i++ {
		member := mgr.RequestJoin("room1", "User", "", 5*time.Minute)
		memberIDs[i] = member.ID
	}

	// Concurrent approves
	for _, id := range memberIDs {
		wg.Add(1)
		go func(memberID uuid.UUID) {
			defer wg.Done()
			mgr.Approve(memberID, "admin")
		}(id)
	}

	// Concurrent gets
	for _, id := range memberIDs {
		wg.Add(1)
		go func(memberID uuid.UUID) {
			defer wg.Done()
			mgr.Get(memberID)
		}(id)
	}

	wg.Wait()

	// Verify all approved
	approved := mgr.GetByRoom("room1", StatusApproved)
	if len(approved) != 50 {
		t.Errorf("Approved members = %d, want 50", len(approved))
	}
}

// TestConcurrent_DenyAndList tests concurrent deny and list operations
func TestConcurrent_DenyAndList(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup

	// Create members
	memberIDs := make([]uuid.UUID, 50)
	for i := 0; i < 50; i++ {
		member := mgr.RequestJoin("room1", "User", "", 5*time.Minute)
		memberIDs[i] = member.ID
	}

	// Concurrent denies (first 25)
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(memberID uuid.UUID) {
			defer wg.Done()
			mgr.Deny(memberID)
		}(memberIDs[i])
	}

	// Concurrent list operations
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.GetByRoom("room1", "")
		}()
	}

	wg.Wait()

	if mgr.Count() != 25 {
		t.Errorf("Count() = %d, want 25", mgr.Count())
	}

	members := mgr.GetByRoom("room1", "")
	if len(members) != 25 {
		t.Errorf("room1 has %d members, want 25", len(members))
	}
}

// TestConcurrent_ApproveAll tests concurrent ApproveAll calls
func TestConcurrent_ApproveAll(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup

	// Create members
	for i := 0; i < 20; i++ {
		mgr.RequestJoin("room1", "User", "", 5*time.Minute)
	}

	// Multiple concurrent ApproveAll calls
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.ApproveAll("admin")
		}()
	}

	wg.Wait()

	// All should be approved
	approved := mgr.GetByRoom("room1", StatusApproved)
	if len(approved) != 20 {
		t.Errorf("Approved members = %d, want 20", len(approved))
	}

	pending := mgr.GetAllPending()
	if len(pending) != 0 {
		t.Errorf("Pending members = %d, want 0", len(pending))
	}
}

// TestConcurrent_IsApproved tests concurrent IsApproved checks
func TestConcurrent_IsApproved(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup

	member := mgr.RequestJoin("room1", "Alice", "", 5*time.Minute)

	// Concurrent IsApproved checks while approving
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.IsApproved(member.ID)
		}()
	}

	// Approve in the middle of checks
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond)
		mgr.Approve(member.ID, "admin")
	}()

	wg.Wait()

	// Should be approved at the end
	if !mgr.IsApproved(member.ID) {
		t.Error("IsApproved() = false, want true after approval")
	}
}

// TestConcurrent_CountOperations tests concurrent count operations
func TestConcurrent_CountOperations(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup

	// Create members
	for i := 0; i < 10; i++ {
		mgr.RequestJoin("room1", "User", "", 5*time.Minute)
		mgr.RequestJoin("room2", "User", "", 5*time.Minute)
	}

	// Concurrent count operations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.Count()
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.CountByRoom("room1", "")
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.CountByRoom("room2", StatusPending)
		}()
	}

	wg.Wait()

	if mgr.Count() != 20 {
		t.Errorf("Count() = %d, want 20", mgr.Count())
	}
}

// TestConcurrent_MixedOperations tests various operations concurrently
func TestConcurrent_MixedOperations(t *testing.T) {
	mgr := NewManager()
	var wg sync.WaitGroup

	operations := 50

	for i := 0; i < operations; i++ {
		// RequestJoin
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.RequestJoin("room1", "User", "", 5*time.Minute)
		}()

		// GetByRoom
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.GetByRoom("room1", "")
		}()

		// GetAllPending
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.GetAllPending()
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
	if count != operations {
		t.Errorf("Count() = %d, want %d", count, operations)
	}
}

// TestStatus_Constants tests status constants
func TestStatus_Constants(t *testing.T) {
	if StatusPending != "pending" {
		t.Errorf("StatusPending = %s, want 'pending'", StatusPending)
	}
	if StatusApproved != "approved" {
		t.Errorf("StatusApproved = %s, want 'approved'", StatusApproved)
	}
	if StatusDenied != "denied" {
		t.Errorf("StatusDenied = %s, want 'denied'", StatusDenied)
	}
}

// TestMember_Fields tests member field initialization
func TestMember_Fields(t *testing.T) {
	mgr := NewManager()
	timeout := 10 * time.Minute

	member := mgr.RequestJoin("test-room", "TestUser", "test-pubkey", timeout)

	// Verify all fields
	if member.ID == uuid.Nil {
		t.Error("Member.ID is nil UUID")
	}
	if member.Name != "TestUser" {
		t.Errorf("Member.Name = %s, want 'TestUser'", member.Name)
	}
	if member.Status != StatusPending {
		t.Errorf("Member.Status = %s, want 'pending'", member.Status)
	}
	if member.RoomID != "test-room" {
		t.Errorf("Member.RoomID = %s, want 'test-room'", member.RoomID)
	}
	if member.PublicKey != "test-pubkey" {
		t.Errorf("Member.PublicKey = %s, want 'test-pubkey'", member.PublicKey)
	}
	if member.RequestedAt.IsZero() {
		t.Error("Member.RequestedAt is zero")
	}
	if member.ExpiresAt == nil {
		t.Error("Member.ExpiresAt is nil")
	}
	if member.ApprovedAt != nil {
		t.Error("Member.ApprovedAt should be nil for pending member")
	}
	if member.ApprovedBy != "" {
		t.Error("Member.ApprovedBy should be empty for pending member")
	}
}
