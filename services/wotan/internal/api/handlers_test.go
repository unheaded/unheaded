// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"unheaded/pkg/httputil"
	"unheaded/services/wotan/internal/wotan"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/room"
)

const testAdminAPIKey = "test-admin-key"

// setupTestServer creates a new test server with all dependencies
func setupTestServer() *Server {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	msgWotan := wotan.NewWotan()
	srv := NewServer(roomMgr, memberMgr, msgWotan, 5*time.Minute)
	srv.AdminAPIKey = testAdminAPIKey
	return srv
}

// setAdminAuth adds the admin API key header to a test request
func setAdminAuth(req *http.Request) {
	req.Header.Set("X-Admin-API-Key", testAdminAPIKey)
}

// TestNewServer verifies server creation
func TestNewServer(t *testing.T) {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	msgWotan := wotan.NewWotan()

	server := NewServer(roomMgr, memberMgr, msgWotan, 5*time.Minute)

	if server == nil {
		t.Fatal("NewServer() returned nil")
	}
	if server.RoomManager != roomMgr {
		t.Error("RoomManager not set correctly")
	}
	if server.MemberManager != memberMgr {
		t.Error("MemberManager not set correctly")
	}
	if server.Wotan != msgWotan {
		t.Error("Wotan not set correctly")
	}
	if server.PendingApprovalTimeout != 5*time.Minute {
		t.Errorf("PendingApprovalTimeout = %v, want 5m", server.PendingApprovalTimeout)
	}
}

// ============================================================
// JoinRoom Tests
// ============================================================

func TestJoinRoom_Success(t *testing.T) {
	server := setupTestServer()

	reqBody := JoinRoomRequest{
		RoomID:    "test-room",
		Name:      "Alice",
		PublicKey: "pubkey123",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.JoinRoom(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp JoinRoomResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.MemberID == uuid.Nil {
		t.Error("MemberID is nil")
	}
	if resp.Status != member.StatusPending {
		t.Errorf("Status = %s, want %s", resp.Status, member.StatusPending)
	}
	if resp.Message == "" {
		t.Error("Message is empty")
	}

	// Verify room was created
	_, err := server.RoomManager.Get("test-room")
	if err != nil {
		t.Error("Room was not created")
	}

	// Verify member was created
	if server.MemberManager.Count() != 1 {
		t.Errorf("MemberManager.Count() = %d, want 1", server.MemberManager.Count())
	}
}

func TestJoinRoom_MethodNotAllowed(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/join", nil)
	rec := httptest.NewRecorder()

	server.JoinRoom(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestJoinRoom_InvalidJSON(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.JoinRoom(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestJoinRoom_MissingFields(t *testing.T) {
	server := setupTestServer()

	tests := []struct {
		name string
		body JoinRoomRequest
	}{
		{"missing room_id", JoinRoomRequest{Name: "Alice"}},
		{"missing name", JoinRoomRequest{RoomID: "test-room"}},
		{"both missing", JoinRoomRequest{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.JoinRoom(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

// ============================================================
// SendMessage Tests
// ============================================================

func TestSendMessage_Success(t *testing.T) {
	server := setupTestServer()

	// Setup: create room and approved member
	server.RoomManager.Create("test-room", "Test Room")
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	reqBody := SendMessageRequest{
		MemberID: mbr.ID,
		RoomID:   "test-room",
		Content:  "Hello, World!",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	server.SendMessage(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp SendMessageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.MessageID == uuid.Nil {
		t.Error("MessageID is nil")
	}
	if resp.Timestamp.IsZero() {
		t.Error("Timestamp is zero")
	}

	// Verify message was stored
	rm, _ := server.RoomManager.Get("test-room")
	messages := rm.GetMessages()
	if len(messages) != 1 {
		t.Errorf("Room has %d messages, want 1", len(messages))
	}
}

func TestSendMessage_MethodNotAllowed(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/send", nil)
	rec := httptest.NewRecorder()

	server.SendMessage(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestSendMessage_InvalidJSON(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader([]byte("invalid")))
	rec := httptest.NewRecorder()

	server.SendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSendMessage_MissingFields(t *testing.T) {
	server := setupTestServer()

	tests := []struct {
		name string
		body SendMessageRequest
	}{
		{"missing content", SendMessageRequest{RoomID: "room", MemberID: uuid.New()}},
		{"missing room_id", SendMessageRequest{Content: "msg", MemberID: uuid.New()}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			server.SendMessage(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestSendMessage_MemberNotApproved(t *testing.T) {
	server := setupTestServer()
	server.RoomManager.Create("test-room", "Test Room")

	// Member exists but not approved
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)

	reqBody := SendMessageRequest{
		MemberID: mbr.ID,
		RoomID:   "test-room",
		Content:  "Hello",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.SendMessage(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestSendMessage_MemberNotFound(t *testing.T) {
	server := setupTestServer()
	server.RoomManager.Create("test-room", "Test Room")

	reqBody := SendMessageRequest{
		MemberID: uuid.New(), // Non-existent member
		RoomID:   "test-room",
		Content:  "Hello",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.SendMessage(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestSendMessage_RoomNotFound(t *testing.T) {
	server := setupTestServer()

	// Create approved member but no room
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	reqBody := SendMessageRequest{
		MemberID: mbr.ID,
		RoomID:   "nonexistent-room",
		Content:  "Hello",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.SendMessage(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ============================================================
// DeleteMessage Tests
// ============================================================

func TestDeleteMessage_Success(t *testing.T) {
	server := setupTestServer()

	// Setup: create room, approved member, and message
	rm := server.RoomManager.Create("test-room", "Test Room")
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")
	msg, _ := rm.SendMessage(mbr.ID, "Hello, World!")

	reqBody := DeleteMessageRequest{
		MemberID:  mbr.ID,
		MessageID: msg.ID,
		RoomID:    "test-room",
		Content:   "Hello, World!",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodDelete, "/delete", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.DeleteMessage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp DeleteMessageResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Error("Success = false, want true")
	}

	// Verify message was deleted
	messages := rm.GetMessages()
	if len(messages) != 0 {
		t.Errorf("Room has %d messages, want 0", len(messages))
	}
}

func TestDeleteMessage_PostMethodAllowed(t *testing.T) {
	server := setupTestServer()

	rm := server.RoomManager.Create("test-room", "Test Room")
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")
	msg, _ := rm.SendMessage(mbr.ID, "Hello")

	reqBody := DeleteMessageRequest{
		MemberID:  mbr.ID,
		MessageID: msg.ID,
		RoomID:    "test-room",
		Content:   "Hello",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/delete", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.DeleteMessage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d (POST should be allowed)", rec.Code, http.StatusOK)
	}
}

func TestDeleteMessage_MethodNotAllowed(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/delete", nil)
	rec := httptest.NewRecorder()

	server.DeleteMessage(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestDeleteMessage_InvalidJSON(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodDelete, "/delete", bytes.NewReader([]byte("invalid")))
	rec := httptest.NewRecorder()

	server.DeleteMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteMessage_MissingFields(t *testing.T) {
	server := setupTestServer()

	tests := []struct {
		name string
		body DeleteMessageRequest
	}{
		{"missing content", DeleteMessageRequest{RoomID: "room", MemberID: uuid.New(), MessageID: uuid.New()}},
		{"missing room_id", DeleteMessageRequest{Content: "msg", MemberID: uuid.New(), MessageID: uuid.New()}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodDelete, "/delete", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			server.DeleteMessage(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestDeleteMessage_MemberNotApproved(t *testing.T) {
	server := setupTestServer()
	server.RoomManager.Create("test-room", "Test Room")
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)

	reqBody := DeleteMessageRequest{
		MemberID:  mbr.ID,
		MessageID: uuid.New(),
		RoomID:    "test-room",
		Content:   "Hello",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodDelete, "/delete", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.DeleteMessage(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestDeleteMessage_RoomNotFound(t *testing.T) {
	server := setupTestServer()

	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	reqBody := DeleteMessageRequest{
		MemberID:  mbr.ID,
		MessageID: uuid.New(),
		RoomID:    "nonexistent-room",
		Content:   "Hello",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodDelete, "/delete", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.DeleteMessage(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteMessage_VerificationFailed(t *testing.T) {
	server := setupTestServer()

	rm := server.RoomManager.Create("test-room", "Test Room")
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")
	msg, _ := rm.SendMessage(mbr.ID, "Original content")

	// Try to delete with wrong content
	reqBody := DeleteMessageRequest{
		MemberID:  mbr.ID,
		MessageID: msg.ID,
		RoomID:    "test-room",
		Content:   "Wrong content",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodDelete, "/delete", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.DeleteMessage(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var resp DeleteMessageResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Success {
		t.Error("Success = true, want false for verification failure")
	}
}

// ============================================================
// GetMessages Tests
// ============================================================

func TestGetMessages_Success(t *testing.T) {
	server := setupTestServer()

	rm := server.RoomManager.Create("test-room", "Test Room")
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	// Add messages
	rm.SendMessage(mbr.ID, "Message 1")
	rm.SendMessage(mbr.ID, "Message 2")
	rm.SendMessage(mbr.ID, "Message 3")

	req := httptest.NewRequest(http.MethodGet, "/messages?room_id=test-room&member_id="+mbr.ID.String(), nil)
	rec := httptest.NewRecorder()

	server.GetMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp GetMessagesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp.Count != 3 {
		t.Errorf("Count = %d, want 3", resp.Count)
	}
	if len(resp.Messages) != 3 {
		t.Errorf("Messages length = %d, want 3", len(resp.Messages))
	}
}

func TestGetMessages_MethodNotAllowed(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodPost, "/messages", nil)
	rec := httptest.NewRecorder()

	server.GetMessages(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestGetMessages_MissingParams(t *testing.T) {
	server := setupTestServer()

	tests := []struct {
		name string
		url  string
	}{
		{"missing room_id", "/messages?member_id=" + uuid.New().String()},
		{"missing member_id", "/messages?room_id=test-room"},
		{"both missing", "/messages"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			server.GetMessages(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestGetMessages_InvalidMemberID(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/messages?room_id=test-room&member_id=invalid-uuid", nil)
	rec := httptest.NewRecorder()

	server.GetMessages(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetMessages_MemberNotApproved(t *testing.T) {
	server := setupTestServer()
	server.RoomManager.Create("test-room", "Test Room")
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/messages?room_id=test-room&member_id="+mbr.ID.String(), nil)
	rec := httptest.NewRecorder()

	server.GetMessages(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestGetMessages_RoomNotFound(t *testing.T) {
	server := setupTestServer()

	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	req := httptest.NewRequest(http.MethodGet, "/messages?room_id=nonexistent&member_id="+mbr.ID.String(), nil)
	rec := httptest.NewRecorder()

	server.GetMessages(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ============================================================
// GetPendingMembers Tests
// ============================================================

func TestGetPendingMembers_Success(t *testing.T) {
	server := setupTestServer()

	server.MemberManager.RequestJoin("room1", "Alice", "", 5*time.Minute)
	server.MemberManager.RequestJoin("room2", "Bob", "", 5*time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/admin/pending", nil)
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.GetPendingMembers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var pending []*member.Member
	if err := json.NewDecoder(rec.Body).Decode(&pending); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(pending) != 2 {
		t.Errorf("Pending count = %d, want 2", len(pending))
	}
}

func TestGetPendingMembers_MethodNotAllowed(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodPost, "/admin/pending", nil)
	rec := httptest.NewRecorder()

	server.GetPendingMembers(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// ============================================================
// ApproveMember Tests
// ============================================================

func TestApproveMember_Success(t *testing.T) {
	server := setupTestServer()

	mbr := server.MemberManager.RequestJoin("room1", "Alice", "", 5*time.Minute)

	reqBody := map[string]string{
		"member_id":   mbr.ID.String(),
		"approved_by": "admin",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/approve", bytes.NewReader(body))
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.ApproveMember(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify member is approved
	if !server.MemberManager.IsApproved(mbr.ID) {
		t.Error("Member was not approved")
	}
}

func TestApproveMember_MethodNotAllowed(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/admin/approve", nil)
	rec := httptest.NewRecorder()

	server.ApproveMember(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestApproveMember_InvalidJSON(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodPost, "/admin/approve", bytes.NewReader([]byte("invalid")))
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.ApproveMember(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestApproveMember_InvalidMemberID(t *testing.T) {
	server := setupTestServer()

	reqBody := map[string]string{
		"member_id": "invalid-uuid",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/approve", bytes.NewReader(body))
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.ApproveMember(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestApproveMember_MemberNotFound(t *testing.T) {
	server := setupTestServer()

	reqBody := map[string]string{
		"member_id": uuid.New().String(),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/approve", bytes.NewReader(body))
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.ApproveMember(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ============================================================
// DenyMember Tests
// ============================================================

func TestDenyMember_Success(t *testing.T) {
	server := setupTestServer()

	mbr := server.MemberManager.RequestJoin("room1", "Alice", "", 5*time.Minute)

	reqBody := map[string]string{
		"member_id": mbr.ID.String(),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/deny", bytes.NewReader(body))
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.DenyMember(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify member is removed
	_, err := server.MemberManager.Get(mbr.ID)
	if err == nil {
		t.Error("Member was not removed after denial")
	}
}

func TestDenyMember_MethodNotAllowed(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/admin/deny", nil)
	rec := httptest.NewRecorder()

	server.DenyMember(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestDenyMember_InvalidJSON(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodPost, "/admin/deny", bytes.NewReader([]byte("invalid")))
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.DenyMember(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDenyMember_InvalidMemberID(t *testing.T) {
	server := setupTestServer()

	reqBody := map[string]string{
		"member_id": "invalid-uuid",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/deny", bytes.NewReader(body))
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.DenyMember(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDenyMember_MemberNotFound(t *testing.T) {
	server := setupTestServer()

	reqBody := map[string]string{
		"member_id": uuid.New().String(),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/deny", bytes.NewReader(body))
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.DenyMember(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ============================================================
// ApproveAllMembers Tests
// ============================================================

func TestApproveAllMembers_Success(t *testing.T) {
	server := setupTestServer()

	server.MemberManager.RequestJoin("room1", "Alice", "", 5*time.Minute)
	server.MemberManager.RequestJoin("room2", "Bob", "", 5*time.Minute)
	server.MemberManager.RequestJoin("room3", "Charlie", "", 5*time.Minute)

	reqBody := map[string]string{
		"approved_by": "super-admin",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/admin/approve-all", bytes.NewReader(body))
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.ApproveAllMembers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	approvedCount, ok := resp["approved_count"].(float64)
	if !ok {
		t.Fatalf("approved_count not found or wrong type in response: %v", resp)
	}
	if approvedCount != 3 {
		t.Errorf("approved_count = %v, want 3", approvedCount)
	}

	// Verify all are approved
	pending := server.MemberManager.GetAllPending()
	if len(pending) != 0 {
		t.Errorf("Pending count = %d, want 0", len(pending))
	}
}

func TestApproveAllMembers_DefaultApprovedBy(t *testing.T) {
	server := setupTestServer()

	mbr := server.MemberManager.RequestJoin("room1", "Alice", "", 5*time.Minute)

	// Empty body should default to "admin"
	req := httptest.NewRequest(http.MethodPost, "/admin/approve-all", nil)
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.ApproveAllMembers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Verify approved_by is "admin"
	member, _ := server.MemberManager.Get(mbr.ID)
	if member.ApprovedBy != "admin" {
		t.Errorf("ApprovedBy = %s, want 'admin'", member.ApprovedBy)
	}
}

func TestApproveAllMembers_MethodNotAllowed(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/admin/approve-all", nil)
	rec := httptest.NewRecorder()

	server.ApproveAllMembers(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// ============================================================
// Health Tests
// ============================================================

func TestHealth_Success(t *testing.T) {
	server := setupTestServer()

	// Add some data
	server.RoomManager.Create("room1", "Room 1")
	server.RoomManager.Create("room2", "Room 2")
	server.MemberManager.RequestJoin("room1", "Alice", "", 5*time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	server.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Errorf("status = %v, want 'healthy'", resp["status"])
	}
	if resp["service"] != "wotan" {
		t.Errorf("service = %v, want 'wotan'", resp["service"])
	}
	if resp["version"] != "0.1.0" {
		t.Errorf("version = %v, want '0.1.0'", resp["version"])
	}
	if resp["rooms"].(float64) != 2 {
		t.Errorf("rooms = %v, want 2", resp["rooms"])
	}
	if resp["total_members"].(float64) != 1 {
		t.Errorf("total_members = %v, want 1", resp["total_members"])
	}
	if resp["timestamp"] == nil {
		t.Error("timestamp is missing")
	}
}

// ============================================================
// Integration Flow Tests
// ============================================================

func TestIntegration_FullMessageFlow(t *testing.T) {
	server := setupTestServer()

	// 1. Join room
	joinReq := JoinRoomRequest{RoomID: "general", Name: "Alice"}
	joinBody, _ := json.Marshal(joinReq)
	req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(joinBody))
	rec := httptest.NewRecorder()
	server.JoinRoom(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Join failed: %d", rec.Code)
	}

	var joinResp JoinRoomResponse
	json.NewDecoder(rec.Body).Decode(&joinResp)
	memberID := joinResp.MemberID

	// 2. Approve member
	approveReq := map[string]string{"member_id": memberID.String(), "approved_by": "admin"}
	approveBody, _ := json.Marshal(approveReq)
	req = httptest.NewRequest(http.MethodPost, "/admin/approve", bytes.NewReader(approveBody))
	setAdminAuth(req)
	rec = httptest.NewRecorder()
	server.ApproveMember(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Approve failed: %d", rec.Code)
	}

	// 3. Send message
	sendReq := SendMessageRequest{MemberID: memberID, RoomID: "general", Content: "Hello everyone!"}
	sendBody, _ := json.Marshal(sendReq)
	req = httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(sendBody))
	rec = httptest.NewRecorder()
	server.SendMessage(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("Send failed: %d", rec.Code)
	}

	var sendResp SendMessageResponse
	json.NewDecoder(rec.Body).Decode(&sendResp)
	messageID := sendResp.MessageID

	// 4. Get messages
	req = httptest.NewRequest(http.MethodGet, "/messages?room_id=general&member_id="+memberID.String(), nil)
	rec = httptest.NewRecorder()
	server.GetMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetMessages failed: %d", rec.Code)
	}

	var getResp GetMessagesResponse
	json.NewDecoder(rec.Body).Decode(&getResp)

	if getResp.Count != 1 {
		t.Errorf("Message count = %d, want 1", getResp.Count)
	}
	if getResp.Messages[0].Content != "Hello everyone!" {
		t.Errorf("Message content = %s, want 'Hello everyone!'", getResp.Messages[0].Content)
	}

	// 5. Delete message
	deleteReq := DeleteMessageRequest{MemberID: memberID, MessageID: messageID, RoomID: "general", Content: "Hello everyone!"}
	deleteBody, _ := json.Marshal(deleteReq)
	req = httptest.NewRequest(http.MethodDelete, "/delete", bytes.NewReader(deleteBody))
	rec = httptest.NewRecorder()
	server.DeleteMessage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Delete failed: %d", rec.Code)
	}

	// 6. Verify message deleted
	req = httptest.NewRequest(http.MethodGet, "/messages?room_id=general&member_id="+memberID.String(), nil)
	rec = httptest.NewRecorder()
	server.GetMessages(rec, req)

	json.NewDecoder(rec.Body).Decode(&getResp)
	if getResp.Count != 0 {
		t.Errorf("Message count after delete = %d, want 0", getResp.Count)
	}
}

func TestIntegration_MultipleUsers(t *testing.T) {
	server := setupTestServer()

	// Create room
	server.RoomManager.Create("chat", "Chat Room")

	// Multiple users join and get approved
	users := []string{"Alice", "Bob", "Charlie"}
	memberIDs := make([]uuid.UUID, len(users))

	for i, name := range users {
		mbr := server.MemberManager.RequestJoin("chat", name, "", 5*time.Minute)
		server.MemberManager.Approve(mbr.ID, "admin")
		memberIDs[i] = mbr.ID
	}

	// Each user sends a message
	for i, id := range memberIDs {
		sendReq := SendMessageRequest{MemberID: id, RoomID: "chat", Content: users[i] + " says hello"}
		sendBody, _ := json.Marshal(sendReq)
		req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(sendBody))
		rec := httptest.NewRecorder()
		server.SendMessage(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("User %s send failed: %d", users[i], rec.Code)
		}
	}

	// Verify all messages
	req := httptest.NewRequest(http.MethodGet, "/messages?room_id=chat&member_id="+memberIDs[0].String(), nil)
	rec := httptest.NewRecorder()
	server.GetMessages(rec, req)

	var getResp GetMessagesResponse
	json.NewDecoder(rec.Body).Decode(&getResp)

	if getResp.Count != 3 {
		t.Errorf("Message count = %d, want 3", getResp.Count)
	}
}

// ============================================================
// Helper Function Tests
// ============================================================

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	writeJSON(rec, http.StatusOK, data)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["key"] != "value" {
		t.Errorf("Response key = %s, want 'value'", resp["key"])
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()

	writeError(rec, http.StatusBadRequest, "test error message")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var resp httputil.Response
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Success {
		t.Error("expected success=false")
	}
	if resp.Error == nil {
		t.Fatal("expected error info, got nil")
	}
	if resp.Error.Code != "Bad Request" {
		t.Errorf("Error code = %s, want 'Bad Request'", resp.Error.Code)
	}
	if resp.Error.Message != "test error message" {
		t.Errorf("Error message = %s, want 'test error message'", resp.Error.Message)
	}
}

// ============================================================
// SetAdminAPIKey Tests
// ============================================================

func TestSetAdminAPIKey(t *testing.T) {
	server := setupTestServer()
	server.SetAdminAPIKey("new-key-123")
	if server.AdminAPIKey != "new-key-123" {
		t.Errorf("AdminAPIKey = %s, want 'new-key-123'", server.AdminAPIKey)
	}
}

// ============================================================
// requireAdminAuth Tests (via admin endpoints)
// ============================================================

func TestAdminAuth_BearerToken(t *testing.T) {
	server := setupTestServer()

	server.MemberManager.RequestJoin("room1", "Alice", "", 5*time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/admin/pending", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminAPIKey)
	rec := httptest.NewRecorder()

	server.GetPendingMembers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d for Bearer token auth", rec.Code, http.StatusOK)
	}
}

func TestAdminAuth_MissingKey(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/admin/pending", nil)
	// No auth header
	rec := httptest.NewRecorder()

	server.GetPendingMembers(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d for missing key", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminAuth_InvalidKey(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/admin/pending", nil)
	req.Header.Set("X-Admin-API-Key", "wrong-key")
	rec := httptest.NewRecorder()

	server.GetPendingMembers(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d for invalid key", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminAuth_NotConfigured(t *testing.T) {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	msgWotan := wotan.NewWotan()
	server := NewServer(roomMgr, memberMgr, msgWotan, 5*time.Minute)
	// AdminAPIKey is empty string (not configured)

	req := httptest.NewRequest(http.MethodGet, "/admin/pending", nil)
	req.Header.Set("X-Admin-API-Key", "some-key")
	rec := httptest.NewRecorder()

	server.GetPendingMembers(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d for unconfigured admin auth", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminAuth_ShortBearerHeader(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/admin/pending", nil)
	req.Header.Set("Authorization", "Bear") // Too short for "Bearer "
	rec := httptest.NewRecorder()

	server.GetPendingMembers(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d for short auth header", rec.Code, http.StatusUnauthorized)
	}
}

// ============================================================
// secureCompare Tests
// ============================================================

func TestSecureCompare(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"equal strings", "abc", "abc", true},
		{"different strings", "abc", "def", false},
		{"different lengths", "ab", "abc", false},
		{"empty strings", "", "", true},
		{"one empty", "abc", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := secureCompare(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("secureCompare(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ============================================================
// SendMessage with nil Wotan Tests
// ============================================================

func TestSendMessage_NilWotan(t *testing.T) {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	server := NewServer(roomMgr, memberMgr, nil, 5*time.Minute)
	server.AdminAPIKey = testAdminAPIKey

	server.RoomManager.Create("test-room", "Test Room")
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	reqBody := SendMessageRequest{
		MemberID: mbr.ID,
		RoomID:   "test-room",
		Content:  "Hello, World!",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.SendMessage(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d with nil wotan", rec.Code, http.StatusCreated)
	}
}

func TestDeleteMessage_NilWotan(t *testing.T) {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	server := NewServer(roomMgr, memberMgr, nil, 5*time.Minute)
	server.AdminAPIKey = testAdminAPIKey

	rm := server.RoomManager.Create("test-room", "Test Room")
	mbr := server.MemberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")
	msg, _ := rm.SendMessage(mbr.ID, "Hello, World!")

	reqBody := DeleteMessageRequest{
		MemberID:  mbr.ID,
		MessageID: msg.ID,
		RoomID:    "test-room",
		Content:   "Hello, World!",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodDelete, "/delete", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.DeleteMessage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d with nil wotan", rec.Code, http.StatusOK)
	}
}

// ============================================================
// GetPendingMembers admin auth error path
// ============================================================

func TestGetPendingMembers_AdminAuthError(t *testing.T) {
	server := setupTestServer()

	// No auth header at all
	req := httptest.NewRequest(http.MethodGet, "/admin/pending", nil)
	rec := httptest.NewRecorder()

	server.GetPendingMembers(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// ============================================================
// ApproveAllMembers with no pending members
// ============================================================

func TestApproveAllMembers_NoPending(t *testing.T) {
	server := setupTestServer()

	req := httptest.NewRequest(http.MethodPost, "/admin/approve-all", nil)
	setAdminAuth(req)
	rec := httptest.NewRecorder()

	server.ApproveAllMembers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	approvedCount, ok := resp["approved_count"].(float64)
	if !ok {
		t.Fatalf("approved_count not found in response: %v", resp)
	}
	if approvedCount != 0 {
		t.Errorf("approved_count = %v, want 0", approvedCount)
	}
}

// ============================================================================
// BENCHMARKS
// ============================================================================

// BenchmarkJoinRoom measures join room endpoint performance
func BenchmarkJoinRoom(b *testing.B) {
	server := setupTestServer()
	reqBody := JoinRoomRequest{
		RoomID:    "bench-room",
		Name:      "BenchUser",
		PublicKey: "pubkey",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.JoinRoom(rec, req)
	}
}

// BenchmarkSendMessage measures send message endpoint performance
func BenchmarkSendMessage(b *testing.B) {
	server := setupTestServer()

	// Setup: create room and approved member
	server.RoomManager.Create("bench-room", "Bench Room")
	mbr := server.MemberManager.RequestJoin("bench-room", "BenchUser", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	reqBody := SendMessageRequest{
		MemberID: mbr.ID,
		RoomID:   "bench-room",
		Content:  "benchmark message content",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/messages/send", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.SendMessage(rec, req)
	}
}

// BenchmarkGetMessages measures get messages endpoint performance
func BenchmarkGetMessages(b *testing.B) {
	server := setupTestServer()

	// Setup: create room, member, and messages
	server.RoomManager.Create("bench-room", "Bench Room")
	mbr := server.MemberManager.RequestJoin("bench-room", "BenchUser", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	// Add 100 messages
	room, _ := server.RoomManager.Get("bench-room")
	for i := 0; i < 100; i++ {
		room.SendMessage(mbr.ID, "test message")
	}

	url := "/messages?room_id=bench-room&member_id=" + mbr.ID.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		server.GetMessages(rec, req)
	}
}

// BenchmarkGetMessages_1000 measures get messages with 1000 messages
func BenchmarkGetMessages_1000(b *testing.B) {
	server := setupTestServer()

	// Setup with larger buffer
	server.RoomManager = room.NewManager(2000)
	server.RoomManager.Create("bench-room", "Bench Room")
	mbr := server.MemberManager.RequestJoin("bench-room", "BenchUser", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	// Add 1000 messages
	rm, _ := server.RoomManager.Get("bench-room")
	for i := 0; i < 1000; i++ {
		rm.SendMessage(mbr.ID, "test message content here")
	}

	url := "/messages?room_id=bench-room&member_id=" + mbr.ID.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		server.GetMessages(rec, req)
	}
}

// BenchmarkDeleteMessage measures delete message endpoint performance
func BenchmarkDeleteMessage(b *testing.B) {
	server := setupTestServer()

	// Setup
	server.RoomManager.Create("bench-room", "Bench Room")
	mbr := server.MemberManager.RequestJoin("bench-room", "BenchUser", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	rm, _ := server.RoomManager.Get("bench-room")
	msg, _ := rm.SendMessage(mbr.ID, "message to delete")

	reqBody := DeleteMessageRequest{
		MemberID:  mbr.ID,
		MessageID: msg.ID,
		RoomID:    "bench-room",
		Content:   "message to delete",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/messages/delete", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.DeleteMessage(rec, req)
	}
}

// BenchmarkHealth measures health endpoint performance
func BenchmarkHealth(b *testing.B) {
	server := setupTestServer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		server.Health(rec, req)
	}
}

// BenchmarkGetPendingMembers measures admin pending endpoint
func BenchmarkGetPendingMembers(b *testing.B) {
	server := setupTestServer()

	// Add some pending members
	for i := 0; i < 50; i++ {
		server.MemberManager.RequestJoin("room1", "User", "", 5*time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/pending", nil)
		setAdminAuth(req)
		rec := httptest.NewRecorder()
		server.GetPendingMembers(rec, req)
	}
}

// BenchmarkApproveMember measures member approval performance
func BenchmarkApproveMember(b *testing.B) {
	server := setupTestServer()

	// Pre-create pending members
	members := make([]uuid.UUID, b.N)
	for i := 0; i < b.N; i++ {
		mbr := server.MemberManager.RequestJoin("room1", "User", "", 5*time.Minute)
		members[i] = mbr.ID
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reqBody := struct {
		MemberID uuid.UUID `json:"member_id"`
	}{MemberID: members[i]}
		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/admin/approve", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		setAdminAuth(req)
		rec := httptest.NewRecorder()
		server.ApproveMember(rec, req)
	}
}

// BenchmarkWriteJSON measures JSON serialization performance
func BenchmarkWriteJSON(b *testing.B) {
	data := map[string]interface{}{
		"id":        uuid.New().String(),
		"timestamp": time.Now(),
		"message":   "benchmark test message",
		"count":     100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		writeJSON(rec, http.StatusOK, data)
	}
}

// BenchmarkWriteError measures error response performance
func BenchmarkWriteError(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		writeError(rec, http.StatusBadRequest, "benchmark error message")
	}
}

// BenchmarkConcurrentRequests measures concurrent request handling
func BenchmarkConcurrentRequests(b *testing.B) {
	server := setupTestServer()

	// Setup
	server.RoomManager.Create("bench-room", "Bench Room")
	mbr := server.MemberManager.RequestJoin("bench-room", "BenchUser", "", 5*time.Minute)
	server.MemberManager.Approve(mbr.ID, "admin")

	reqBody := SendMessageRequest{
		MemberID: mbr.ID,
		RoomID:   "bench-room",
		Content:  "concurrent benchmark message",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/messages/send", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			server.SendMessage(rec, req)
		}
	})
}

// BenchmarkFullWorkflow measures a complete user workflow
func BenchmarkFullWorkflow(b *testing.B) {
	server := setupTestServer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// 1. Join room
		joinReq := JoinRoomRequest{RoomID: "workflow-room", Name: "User", PublicKey: "pk"}
		joinBody, _ := json.Marshal(joinReq)
		req := httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(joinBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		server.JoinRoom(rec, req)

		var joinResp JoinRoomResponse
		json.NewDecoder(rec.Body).Decode(&joinResp)

		// 2. Approve member
		approveReq := struct {
		MemberID uuid.UUID `json:"member_id"`
	}{MemberID: joinResp.MemberID}
		approveBody, _ := json.Marshal(approveReq)
		req = httptest.NewRequest(http.MethodPost, "/admin/approve", bytes.NewReader(approveBody))
		req.Header.Set("Content-Type", "application/json")
		setAdminAuth(req)
		rec = httptest.NewRecorder()
		server.ApproveMember(rec, req)

		// 3. Send message
		sendReq := SendMessageRequest{MemberID: joinResp.MemberID, RoomID: "workflow-room", Content: "test"}
		sendBody, _ := json.Marshal(sendReq)
		req = httptest.NewRequest(http.MethodPost, "/messages/send", bytes.NewReader(sendBody))
		req.Header.Set("Content-Type", "application/json")
		rec = httptest.NewRecorder()
		server.SendMessage(rec, req)

		// 4. Get messages
		url := "/messages?room_id=workflow-room&member_id=" + joinResp.MemberID.String()
		req = httptest.NewRequest(http.MethodGet, url, nil)
		rec = httptest.NewRecorder()
		server.GetMessages(rec, req)
	}
}
