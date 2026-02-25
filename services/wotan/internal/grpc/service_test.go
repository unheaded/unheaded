// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package grpc

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	chatpb "unheaded/services/wotan/proto"
	"unheaded/services/wotan/internal/wotan"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/metrics"
	"unheaded/services/wotan/internal/ringbuffer"
	"unheaded/services/wotan/internal/room"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestMain initializes metrics before running tests
func TestMain(m *testing.M) {
	metrics.Initialize("test")
	os.Exit(m.Run())
}

// mockStream implements chatpb.ChatStream_StreamMessagesServer for testing
type mockStream struct {
	ctx      context.Context
	events   []*chatpb.MessageEvent
	sendErr  error
	sendFunc func(*chatpb.MessageEvent) error
}

func newMockStream(ctx context.Context) *mockStream {
	return &mockStream{
		ctx:    ctx,
		events: make([]*chatpb.MessageEvent, 0),
	}
}

func (m *mockStream) Send(event *chatpb.MessageEvent) error {
	if m.sendFunc != nil {
		return m.sendFunc(event)
	}
	if m.sendErr != nil {
		return m.sendErr
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockStream) Context() context.Context {
	return m.ctx
}

func (m *mockStream) SetHeader(md metadata.MD) error  { return nil }
func (m *mockStream) SendHeader(md metadata.MD) error { return nil }
func (m *mockStream) SetTrailer(md metadata.MD)       {}
func (m *mockStream) SendMsg(msg any) error           { return nil }
func (m *mockStream) RecvMsg(msg any) error           { return nil }

// setupTestService creates a service with all dependencies
func setupTestService() *ChatService {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	msgWotan := wotan.NewWotan()
	return NewChatService(roomMgr, memberMgr, msgWotan)
}

// TestNewChatService verifies service creation
func TestNewChatService(t *testing.T) {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	msgWotan := wotan.NewWotan()

	service := NewChatService(roomMgr, memberMgr, msgWotan)

	if service == nil {
		t.Fatal("NewChatService() returned nil")
	}
	if service.roomManager != roomMgr {
		t.Error("roomManager not set correctly")
	}
	if service.memberManager != memberMgr {
		t.Error("memberManager not set correctly")
	}
	if service.wotan != msgWotan {
		t.Error("wotan not set correctly")
	}
}

// TestPing tests the Ping method
func TestPing(t *testing.T) {
	service := setupTestService()

	ctx := context.Background()
	req := &chatpb.PingRequest{}

	resp, err := service.Ping(ctx, req)

	if err != nil {
		t.Fatalf("Ping() returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("Ping() returned nil response")
	}
	if resp.Status != "ok" {
		t.Errorf("Ping() status = %s, want 'ok'", resp.Status)
	}
	if resp.Timestamp == nil {
		t.Error("Ping() timestamp is nil")
	}
}

// TestString tests the String method
func TestString(t *testing.T) {
	service := setupTestService()

	// Add some data
	service.roomManager.Create("room1", "Room 1")
	service.roomManager.Create("room2", "Room 2")
	mbr := service.memberManager.RequestJoin("room1", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	str := service.String()

	if str == "" {
		t.Error("String() returned empty string")
	}

	// Should contain counts
	expected := "ChatService{rooms=2, members=1, subscriptions=0}"
	if str != expected {
		t.Errorf("String() = %s, want %s", str, expected)
	}
}

// TestGetCreatorName tests the getCreatorName helper
func TestGetCreatorName(t *testing.T) {
	service := setupTestService()

	// Test with non-existent member
	name := service.getCreatorName(uuid.New())
	if name != "Unknown" {
		t.Errorf("getCreatorName() = %s, want 'Unknown' for non-existent member", name)
	}

	// Test with existing member
	mbr := service.memberManager.RequestJoin("room1", "Alice", "", 5*time.Minute)
	name = service.getCreatorName(mbr.ID)
	if name != "Alice" {
		t.Errorf("getCreatorName() = %s, want 'Alice'", name)
	}
}

// TestStreamMessages_InvalidMemberID tests invalid member ID
func TestStreamMessages_InvalidMemberID(t *testing.T) {
	service := setupTestService()

	ctx := context.Background()
	stream := newMockStream(ctx)

	req := &chatpb.StreamRequest{
		MemberId: "invalid-uuid",
		RoomId:   "test-room",
	}

	err := service.StreamMessages(req, stream)

	if err == nil {
		t.Fatal("StreamMessages() should return error for invalid member ID")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("Error code = %v, want InvalidArgument", st.Code())
	}
}

// TestStreamMessages_MemberNotFound tests non-existent member
func TestStreamMessages_MemberNotFound(t *testing.T) {
	service := setupTestService()

	ctx := context.Background()
	stream := newMockStream(ctx)

	memberID := uuid.New()
	req := &chatpb.StreamRequest{
		MemberId: memberID.String(),
		RoomId:   "test-room",
	}

	err := service.StreamMessages(req, stream)

	if err == nil {
		t.Fatal("StreamMessages() should return error for non-existent member")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}
	if st.Code() != codes.NotFound {
		t.Errorf("Error code = %v, want NotFound", st.Code())
	}
}

// TestStreamMessages_MemberNotApproved tests unapproved member
func TestStreamMessages_MemberNotApproved(t *testing.T) {
	service := setupTestService()

	ctx := context.Background()
	stream := newMockStream(ctx)

	// Create pending member (not approved)
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
	}

	err := service.StreamMessages(req, stream)

	if err == nil {
		t.Fatal("StreamMessages() should return error for unapproved member")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}
	if st.Code() != codes.PermissionDenied {
		t.Errorf("Error code = %v, want PermissionDenied", st.Code())
	}
}

// TestStreamMessages_RoomNotFound tests non-existent room
func TestStreamMessages_RoomNotFound(t *testing.T) {
	service := setupTestService()

	ctx := context.Background()
	stream := newMockStream(ctx)

	// Create and approve member
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "nonexistent-room",
	}

	err := service.StreamMessages(req, stream)

	if err == nil {
		t.Fatal("StreamMessages() should return error for non-existent room")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}
	if st.Code() != codes.NotFound {
		t.Errorf("Error code = %v, want NotFound", st.Code())
	}
}

// TestStreamMessages_HistoricalMessages tests historical message replay
func TestStreamMessages_HistoricalMessages(t *testing.T) {
	service := setupTestService()

	// Setup room and member
	rm := service.roomManager.Create("test-room", "Test Room")
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	// Add historical messages
	rm.SendMessage(mbr.ID, "Message 1")
	time.Sleep(10 * time.Millisecond)
	rm.SendMessage(mbr.ID, "Message 2")
	time.Sleep(10 * time.Millisecond)
	rm.SendMessage(mbr.ID, "Message 3")

	// Create context that cancels after receiving messages
	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockStream(ctx)

	// Send historical messages, then cancel
	msgCount := 0
	stream.sendFunc = func(event *chatpb.MessageEvent) error {
		msgCount++
		stream.events = append(stream.events, event)
		if msgCount >= 3 {
			cancel()
		}
		return nil
	}

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
		Since:    timestamppb.New(time.Now().Add(-1 * time.Hour)),
	}

	err := service.StreamMessages(req, stream)

	// Should return Canceled after context is cancelled
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}
	if st.Code() != codes.Canceled {
		t.Errorf("Error code = %v, want Canceled", st.Code())
	}

	// Verify historical messages were sent
	if len(stream.events) != 3 {
		t.Errorf("Received %d events, want 3", len(stream.events))
	}

	for _, event := range stream.events {
		if event.Type != chatpb.EventType_MESSAGE_CREATED {
			t.Errorf("Event type = %v, want MESSAGE_CREATED", event.Type)
		}
	}
}

// TestStreamMessages_SendError tests handling of send errors on historical messages
func TestStreamMessages_SendError_Historical(t *testing.T) {
	service := setupTestService()

	// Setup room and member
	rm := service.roomManager.Create("test-room", "Test Room")
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	// Add a message
	rm.SendMessage(mbr.ID, "Message 1")

	ctx := context.Background()
	stream := newMockStream(ctx)
	stream.sendErr = errors.New("send failed")

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
		Since:    timestamppb.New(time.Now().Add(-1 * time.Hour)),
	}

	err := service.StreamMessages(req, stream)

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}
	if st.Code() != codes.Aborted {
		t.Errorf("Error code = %v, want Aborted", st.Code())
	}
}

// TestStreamMessages_LiveMessages tests live message streaming
func TestStreamMessages_LiveMessages(t *testing.T) {
	service := setupTestService()

	// Setup room and member
	service.roomManager.Create("test-room", "Test Room")
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockStream(ctx)

	msgCount := 0
	stream.sendFunc = func(event *chatpb.MessageEvent) error {
		msgCount++
		stream.events = append(stream.events, event)
		if msgCount >= 2 {
			cancel()
		}
		return nil
	}

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
	}

	// Start streaming in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.StreamMessages(req, stream)
	}()

	// Give time for subscription to be set up
	time.Sleep(50 * time.Millisecond)

	// Publish live messages
	msg1 := &ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: mbr.ID,
		RoomID:    "test-room",
		Content:   "Live message 1",
		Timestamp: time.Now(),
	}
	service.wotan.PublishMessageCreated(msg1)

	msg2 := &ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: mbr.ID,
		RoomID:    "test-room",
		Content:   "Live message 2",
		Timestamp: time.Now(),
	}
	service.wotan.PublishMessageCreated(msg2)

	// Wait for stream to end
	err := <-errCh

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}
	if st.Code() != codes.Canceled {
		t.Errorf("Error code = %v, want Canceled", st.Code())
	}

	// Verify live messages were received
	if len(stream.events) < 2 {
		t.Errorf("Received %d events, want at least 2", len(stream.events))
	}
}

// TestStreamMessages_DeletedEvent tests MESSAGE_DELETED events
func TestStreamMessages_DeletedEvent(t *testing.T) {
	service := setupTestService()

	// Setup room and member
	service.roomManager.Create("test-room", "Test Room")
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockStream(ctx)

	stream.sendFunc = func(event *chatpb.MessageEvent) error {
		stream.events = append(stream.events, event)
		cancel()
		return nil
	}

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
	}

	// Start streaming
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.StreamMessages(req, stream)
	}()

	time.Sleep(50 * time.Millisecond)

	// Publish deletion event
	service.wotan.PublishMessageDeleted("test-room", uuid.New())

	<-errCh

	// Verify deletion event type
	if len(stream.events) < 1 {
		t.Fatal("No events received")
	}

	if stream.events[0].Type != chatpb.EventType_MESSAGE_DELETED {
		t.Errorf("Event type = %v, want MESSAGE_DELETED", stream.events[0].Type)
	}
}

// TestStreamMessages_SendErrorLive tests send error during live streaming
func TestStreamMessages_SendErrorLive(t *testing.T) {
	service := setupTestService()

	// Setup room and member
	service.roomManager.Create("test-room", "Test Room")
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	ctx := context.Background()
	stream := newMockStream(ctx)

	// Fail on first send
	stream.sendFunc = func(event *chatpb.MessageEvent) error {
		return errors.New("send failed")
	}

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
	}

	// Start streaming
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.StreamMessages(req, stream)
	}()

	time.Sleep(50 * time.Millisecond)

	// Publish a message to trigger send
	service.wotan.PublishMessageCreated(&ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: mbr.ID,
		RoomID:    "test-room",
		Content:   "Test",
		Timestamp: time.Now(),
	})

	err := <-errCh

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}
	if st.Code() != codes.Aborted {
		t.Errorf("Error code = %v, want Aborted", st.Code())
	}
}

// TestStreamMessages_ChannelClosed tests when subscription channel closes
func TestStreamMessages_ChannelClosed(t *testing.T) {
	service := setupTestService()

	// Setup room and member
	service.roomManager.Create("test-room", "Test Room")
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	ctx := context.Background()
	stream := newMockStream(ctx)

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
	}

	// Start streaming
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.StreamMessages(req, stream)
	}()

	time.Sleep(50 * time.Millisecond)

	// Close the wotan (closes all subscription channels)
	service.wotan.Close()

	err := <-errCh

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("Expected gRPC status error")
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("Error code = %v, want Unavailable", st.Code())
	}
}

// TestStreamMessages_NoSinceTimestamp tests streaming without historical messages
func TestStreamMessages_NoSinceTimestamp(t *testing.T) {
	service := setupTestService()

	// Setup room and member
	rm := service.roomManager.Create("test-room", "Test Room")
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	// Add messages before streaming
	rm.SendMessage(mbr.ID, "Old message")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockStream(ctx)

	stream.sendFunc = func(event *chatpb.MessageEvent) error {
		stream.events = append(stream.events, event)
		cancel()
		return nil
	}

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
		Since:    nil, // No historical request
	}

	// Start streaming
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.StreamMessages(req, stream)
	}()

	time.Sleep(50 * time.Millisecond)

	// Send new message
	service.wotan.PublishMessageCreated(&ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: mbr.ID,
		RoomID:    "test-room",
		Content:   "New message",
		Timestamp: time.Now(),
	})

	<-errCh

	// Should only get the new message, not the old one
	if len(stream.events) != 1 {
		t.Errorf("Received %d events, want 1 (only new message)", len(stream.events))
	}

	if len(stream.events) > 0 && stream.events[0].Content != "New message" {
		t.Errorf("Content = %s, want 'New message'", stream.events[0].Content)
	}
}

// TestStreamMessages_FutureSinceTimestamp tests Since in the future
func TestStreamMessages_FutureSinceTimestamp(t *testing.T) {
	service := setupTestService()

	// Setup room and member
	rm := service.roomManager.Create("test-room", "Test Room")
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	// Add old message
	rm.SendMessage(mbr.ID, "Old message")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockStream(ctx)

	stream.sendFunc = func(event *chatpb.MessageEvent) error {
		stream.events = append(stream.events, event)
		cancel()
		return nil
	}

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
		Since:    timestamppb.New(time.Now().Add(1 * time.Hour)), // Future timestamp
	}

	// Start streaming
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.StreamMessages(req, stream)
	}()

	time.Sleep(50 * time.Millisecond)

	// Send new message
	service.wotan.PublishMessageCreated(&ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: mbr.ID,
		RoomID:    "test-room",
		Content:   "New message",
		Timestamp: time.Now(),
	})

	<-errCh

	// Should only get new message (future Since doesn't replay anything)
	if len(stream.events) != 1 {
		t.Errorf("Received %d events, want 1", len(stream.events))
	}
}

// TestStreamMessages_EventFields tests that event fields are properly populated
func TestStreamMessages_EventFields(t *testing.T) {
	service := setupTestService()

	// Setup room and member
	service.roomManager.Create("test-room", "Test Room")
	mbr := service.memberManager.RequestJoin("test-room", "Alice", "", 5*time.Minute)
	service.memberManager.Approve(mbr.ID, "admin")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockStream(ctx)

	var receivedEvent *chatpb.MessageEvent
	stream.sendFunc = func(event *chatpb.MessageEvent) error {
		receivedEvent = event
		cancel()
		return nil
	}

	req := &chatpb.StreamRequest{
		MemberId: mbr.ID.String(),
		RoomId:   "test-room",
	}

	// Start streaming
	errCh := make(chan error, 1)
	go func() {
		errCh <- service.StreamMessages(req, stream)
	}()

	time.Sleep(50 * time.Millisecond)

	msgID := uuid.New()
	service.wotan.PublishMessageCreated(&ringbuffer.Message{
		ID:        msgID,
		CreatorID: mbr.ID,
		RoomID:    "test-room",
		Content:   "Test content",
		Timestamp: time.Now(),
	})

	<-errCh

	if receivedEvent == nil {
		t.Fatal("No event received")
	}

	// Check all fields
	if receivedEvent.Id != msgID.String() {
		t.Errorf("Id = %s, want %s", receivedEvent.Id, msgID.String())
	}
	if receivedEvent.CreatorId != mbr.ID.String() {
		t.Errorf("CreatorId = %s, want %s", receivedEvent.CreatorId, mbr.ID.String())
	}
	if receivedEvent.RoomId != "test-room" {
		t.Errorf("RoomId = %s, want 'test-room'", receivedEvent.RoomId)
	}
	if receivedEvent.Content != "Test content" {
		t.Errorf("Content = %s, want 'Test content'", receivedEvent.Content)
	}
	if receivedEvent.CreatorName != "Alice" {
		t.Errorf("CreatorName = %s, want 'Alice'", receivedEvent.CreatorName)
	}
	if receivedEvent.Timestamp == nil {
		t.Error("Timestamp is nil")
	}
	if receivedEvent.Type != chatpb.EventType_MESSAGE_CREATED {
		t.Errorf("Type = %v, want MESSAGE_CREATED", receivedEvent.Type)
	}
}
