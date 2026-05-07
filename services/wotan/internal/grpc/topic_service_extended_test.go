// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package grpc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/ringbuffer"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/wotan"
	chatpb "unheaded/services/wotan/proto"
)

// mockTopicStream implements chatpb.TopicStream_StreamTopicsServer for testing
type mockTopicStream struct {
	ctx      context.Context
	events   []*chatpb.TopicEvent
	mu       sync.Mutex
	sendErr  error
	sendFunc func(*chatpb.TopicEvent) error
}

func newMockTopicStream(ctx context.Context) *mockTopicStream {
	return &mockTopicStream{
		ctx:    ctx,
		events: make([]*chatpb.TopicEvent, 0),
	}
}

func (m *mockTopicStream) Send(event *chatpb.TopicEvent) error {
	if m.sendFunc != nil {
		return m.sendFunc(event)
	}
	if m.sendErr != nil {
		return m.sendErr
	}
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()
	return nil
}

func (m *mockTopicStream) Context() context.Context     { return m.ctx }
func (m *mockTopicStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockTopicStream) SendHeader(metadata.MD) error { return nil }
func (m *mockTopicStream) SetTrailer(metadata.MD)       {}
func (m *mockTopicStream) SendMsg(any) error            { return nil }
func (m *mockTopicStream) RecvMsg(any) error            { return nil }

func (m *mockTopicStream) getEvents() []*chatpb.TopicEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*chatpb.TopicEvent, len(m.events))
	copy(cp, m.events)
	return cp
}

// setupTopicService creates a TopicService with all dependencies
func setupTopicService() *TopicService {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	msgWotan := wotan.NewWotan()
	return NewTopicService(roomMgr, memberMgr, msgWotan)
}

// --- TopicSequenceCounter tests ---

func TestNewTopicSequenceCounter(t *testing.T) {
	c := NewTopicSequenceCounter()
	if c == nil {
		t.Fatal("NewTopicSequenceCounter() returned nil")
	}
	if c.counters == nil {
		t.Fatal("counters map is nil")
	}
}

func TestTopicSequenceCounter_Next_NewTopic(t *testing.T) {
	c := NewTopicSequenceCounter()

	seq := c.Next("topicA")
	if seq != 1 {
		t.Errorf("Next(topicA) = %d, want 1", seq)
	}
}

func TestTopicSequenceCounter_Next_Increments(t *testing.T) {
	c := NewTopicSequenceCounter()

	for i := int64(1); i <= 5; i++ {
		seq := c.Next("topicA")
		if seq != i {
			t.Errorf("Next(topicA) call %d = %d, want %d", i, seq, i)
		}
	}
}

func TestTopicSequenceCounter_Next_IndependentTopics(t *testing.T) {
	c := NewTopicSequenceCounter()

	seqA := c.Next("topicA")
	seqB := c.Next("topicB")
	seqA2 := c.Next("topicA")

	if seqA != 1 {
		t.Errorf("topicA first = %d, want 1", seqA)
	}
	if seqB != 1 {
		t.Errorf("topicB first = %d, want 1", seqB)
	}
	if seqA2 != 2 {
		t.Errorf("topicA second = %d, want 2", seqA2)
	}
}

func TestTopicSequenceCounter_Next_Concurrent(t *testing.T) {
	c := NewTopicSequenceCounter()

	const goroutines = 10
	const perGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				c.Next("shared-topic")
			}
		}()
	}
	wg.Wait()

	final := c.Next("shared-topic")
	expected := int64(goroutines*perGoroutine + 1)
	if final != expected {
		t.Errorf("after %d concurrent increments, Next = %d, want %d",
			goroutines*perGoroutine, final, expected)
	}
}

func TestTopicSequenceCounter_Next_ConcurrentNewTopics(t *testing.T) {
	c := NewTopicSequenceCounter()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// All goroutines try to create the same topic concurrently (exercises double-check path)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			c.Next("race-topic")
		}()
	}
	wg.Wait()

	final := c.Next("race-topic")
	expected := int64(goroutines + 1)
	if final != expected {
		t.Errorf("after %d concurrent creates, Next = %d, want %d",
			goroutines, final, expected)
	}
}

// --- NewTopicService / NewTopicServiceWithCounter tests ---

func TestNewTopicService(t *testing.T) {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	msgWotan := wotan.NewWotan()

	svc := NewTopicService(roomMgr, memberMgr, msgWotan)

	if svc == nil {
		t.Fatal("NewTopicService() returned nil")
	}
	if svc.roomManager != roomMgr {
		t.Error("roomManager not set")
	}
	if svc.memberManager != memberMgr {
		t.Error("memberManager not set")
	}
	if svc.wotan != msgWotan {
		t.Error("wotan not set")
	}
	if svc.seqCounter == nil {
		t.Error("seqCounter is nil")
	}
}

func TestNewTopicServiceWithCounter(t *testing.T) {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	msgWotan := wotan.NewWotan()
	counter := NewTopicSequenceCounter()

	svc := NewTopicServiceWithCounter(roomMgr, memberMgr, msgWotan, counter)

	if svc == nil {
		t.Fatal("NewTopicServiceWithCounter() returned nil")
	}
	if svc.seqCounter != counter {
		t.Error("seqCounter not set to provided counter")
	}
}

// --- TopicPing tests ---

func TestTopicPing(t *testing.T) {
	svc := setupTopicService()

	ctx := context.Background()
	resp, err := svc.TopicPing(ctx, &chatpb.TopicPingRequest{})

	if err != nil {
		t.Fatalf("TopicPing() error: %v", err)
	}
	if resp == nil {
		t.Fatal("TopicPing() returned nil")
	}
	if resp.Status != "ok" {
		t.Errorf("Status = %q, want %q", resp.Status, "ok")
	}
	if resp.Timestamp == nil {
		t.Error("Timestamp is nil")
	}
	if resp.ActiveStreams != 0 {
		t.Errorf("ActiveStreams = %d, want 0", resp.ActiveStreams)
	}
	if resp.TotalTopics != 0 {
		t.Errorf("TotalTopics = %d, want 0", resp.TotalTopics)
	}
}

func TestTopicPing_WithData(t *testing.T) {
	svc := setupTopicService()

	// Create some rooms
	svc.roomManager.Create("topic1", "Topic 1")
	svc.roomManager.Create("topic2", "Topic 2")

	resp, err := svc.TopicPing(context.Background(), &chatpb.TopicPingRequest{})
	if err != nil {
		t.Fatalf("TopicPing() error: %v", err)
	}
	if resp.TotalTopics != 2 {
		t.Errorf("TotalTopics = %d, want 2", resp.TotalTopics)
	}
}

// --- TopicService.String tests ---

func TestTopicServiceString(t *testing.T) {
	svc := setupTopicService()
	svc.roomManager.Create("topic1", "Topic 1")

	str := svc.String()
	if str == "" {
		t.Error("String() returned empty")
	}
	expected := "TopicService{topics=1, active_streams=0, subscriptions=0}"
	if str != expected {
		t.Errorf("String() = %q, want %q", str, expected)
	}
}

// --- PublishTopic tests ---

func TestPublishTopic_EmptyTopic(t *testing.T) {
	svc := setupTopicService()

	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "",
		Payload: []byte("data"),
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestPublishTopic_EmptyPayload(t *testing.T) {
	svc := setupTopicService()

	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "test-topic",
		Payload: []byte{},
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestPublishTopic_NilPayload(t *testing.T) {
	svc := setupTopicService()

	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "test-topic",
		Payload: nil,
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestPublishTopic_DefaultSender(t *testing.T) {
	svc := setupTopicService()

	resp, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "test-topic",
		Payload: []byte("hello world"),
	})

	if err != nil {
		t.Fatalf("PublishTopic() error: %v", err)
	}
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.MessageId == "" {
		t.Error("MessageId is empty")
	}
	if resp.Seq <= 0 {
		t.Errorf("Seq = %d, want > 0", resp.Seq)
	}
	if resp.CreatedAt == nil {
		t.Error("CreatedAt is nil")
	}

	// Verify the message ID is a valid UUID
	_, err = uuid.Parse(resp.MessageId)
	if err != nil {
		t.Errorf("MessageId is not a valid UUID: %v", err)
	}
}

func TestPublishTopic_WithSenderID(t *testing.T) {
	svc := setupTopicService()

	// Create and approve a member
	mbr := svc.memberManager.RequestJoin("test-topic", "TestUser", "", 5*time.Minute)
	svc.memberManager.Approve(mbr.ID, "admin")

	resp, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:    "test-topic",
		Payload:  []byte("hello"),
		SenderId: mbr.ID.String(),
	})

	if err != nil {
		t.Fatalf("PublishTopic() error: %v", err)
	}
	if resp.MessageId == "" {
		t.Error("MessageId is empty")
	}
}

func TestPublishTopic_InvalidSenderID(t *testing.T) {
	svc := setupTopicService()

	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:    "test-topic",
		Payload:  []byte("hello"),
		SenderId: "not-a-uuid",
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestPublishTopic_SenderNotApproved(t *testing.T) {
	svc := setupTopicService()

	// Create member but do NOT approve
	mbr := svc.memberManager.RequestJoin("test-topic", "PendingUser", "", 5*time.Minute)

	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:    "test-topic",
		Payload:  []byte("hello"),
		SenderId: mbr.ID.String(),
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.PermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", st.Code())
	}
}

func TestPublishTopic_SenderNotFound(t *testing.T) {
	svc := setupTopicService()

	unknownID := uuid.New()
	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:    "test-topic",
		Payload:  []byte("hello"),
		SenderId: unknownID.String(),
	})

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.PermissionDenied {
		t.Errorf("code = %v, want PermissionDenied", st.Code())
	}
}

func TestPublishTopic_SequenceIncreases(t *testing.T) {
	svc := setupTopicService()

	resp1, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "test-topic",
		Payload: []byte("msg1"),
	})
	if err != nil {
		t.Fatalf("first publish error: %v", err)
	}

	resp2, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "test-topic",
		Payload: []byte("msg2"),
	})
	if err != nil {
		t.Fatalf("second publish error: %v", err)
	}

	if resp2.Seq <= resp1.Seq {
		t.Errorf("seq did not increase: %d <= %d", resp2.Seq, resp1.Seq)
	}
}

func TestPublishTopic_CreatesRoom(t *testing.T) {
	svc := setupTopicService()

	// Verify room doesn't exist
	_, err := svc.roomManager.Get("new-topic")
	if err == nil {
		t.Fatal("room should not exist yet")
	}

	_, err = svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "new-topic",
		Payload: []byte("first message"),
	})
	if err != nil {
		t.Fatalf("PublishTopic() error: %v", err)
	}

	// Verify room was created
	rm, err := svc.roomManager.Get("new-topic")
	if err != nil {
		t.Fatalf("room should exist after publish: %v", err)
	}

	msgs := rm.GetMessages()
	if len(msgs) != 1 {
		t.Errorf("room has %d messages, want 1", len(msgs))
	}
	if msgs[0].Content != "first message" {
		t.Errorf("message content = %q, want %q", msgs[0].Content, "first message")
	}
}

// --- StreamTopics tests ---

func TestStreamTopics_EmptyPattern(t *testing.T) {
	svc := setupTopicService()

	ctx := context.Background()
	stream := newMockTopicStream(ctx)

	err := svc.StreamTopics(&chatpb.TopicStreamRequest{
		TopicPattern: "",
	}, stream)

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

func TestStreamTopics_ContextCanceled(t *testing.T) {
	svc := setupTopicService()

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockTopicStream(ctx)

	// Cancel immediately so the stream loop exits
	cancel()

	err := svc.StreamTopics(&chatpb.TopicStreamRequest{
		TopicPattern: "*",
		DisplayName:  "test-sub",
	}, stream)

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Canceled {
		t.Errorf("code = %v, want Canceled", st.Code())
	}
}

func TestStreamTopics_DefaultDisplayName(t *testing.T) {
	svc := setupTopicService()

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockTopicStream(ctx)

	// Cancel immediately
	cancel()

	err := svc.StreamTopics(&chatpb.TopicStreamRequest{
		TopicPattern: "*",
		// DisplayName left empty — should default to "grpc-subscriber"
	}, stream)

	// Just verify it doesn't crash and returns a proper error
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Canceled {
		t.Errorf("code = %v, want Canceled", st.Code())
	}
}

func TestStreamTopics_LiveMessages(t *testing.T) {
	svc := setupTopicService()

	// Pre-create a topic room
	svc.roomManager.Create("events.user", "User Events")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockTopicStream(ctx)

	received := make(chan struct{}, 1)
	stream.sendFunc = func(event *chatpb.TopicEvent) error {
		stream.mu.Lock()
		stream.events = append(stream.events, event)
		stream.mu.Unlock()
		select {
		case received <- struct{}{}:
		default:
		}
		cancel()
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.StreamTopics(&chatpb.TopicStreamRequest{
			TopicPattern: "events.*",
			DisplayName:  "test-sub",
		}, stream)
	}()

	// Wait for subscription to be established
	time.Sleep(100 * time.Millisecond)

	// Publish a message to the matching topic
	mbr := svc.memberManager.RequestJoin("events.user", "publisher", "", 5*time.Minute)
	svc.memberManager.Approve(mbr.ID, "admin")

	svc.wotan.PublishMessageCreated(&ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: mbr.ID,
		RoomID:    "events.user",
		Content:   "user signed up",
		Timestamp: time.Now(),
	})

	err := <-errCh

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Canceled {
		t.Errorf("code = %v, want Canceled", st.Code())
	}

	evts := stream.getEvents()
	if len(evts) < 1 {
		t.Fatal("expected at least 1 event")
	}

	// The event should be from the matching topic
	if evts[0].Topic != "events.user" {
		t.Errorf("Topic = %q, want %q", evts[0].Topic, "events.user")
	}
}

func TestStreamTopics_SendErrorOnLive(t *testing.T) {
	svc := setupTopicService()

	svc.roomManager.Create("test.topic", "Test Topic")

	ctx := context.Background()
	stream := newMockTopicStream(ctx)

	stream.sendFunc = func(event *chatpb.TopicEvent) error {
		return errors.New("send failed")
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.StreamTopics(&chatpb.TopicStreamRequest{
			TopicPattern: "test.*",
			DisplayName:  "test-sub",
		}, stream)
	}()

	time.Sleep(100 * time.Millisecond)

	// Publish to trigger send error
	mbr := svc.memberManager.RequestJoin("test.topic", "pub", "", 5*time.Minute)
	svc.memberManager.Approve(mbr.ID, "admin")
	svc.wotan.PublishMessageCreated(&ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: mbr.ID,
		RoomID:    "test.topic",
		Content:   "data",
		Timestamp: time.Now(),
	})

	err := <-errCh
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Aborted {
		t.Errorf("code = %v, want Aborted", st.Code())
	}
}

func TestStreamTopics_HistoricalReplay(t *testing.T) {
	svc := setupTopicService()

	// Pre-create topic and add messages
	rm := svc.roomManager.Create("orders.created", "Orders Created")
	creator := svc.memberManager.RequestJoin("orders.created", "sys", "", 5*time.Minute)
	svc.memberManager.Approve(creator.ID, "admin")
	rm.SendMessage(creator.ID, "order-1")
	rm.SendMessage(creator.ID, "order-2")
	rm.SendMessage(creator.ID, "order-3")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockTopicStream(ctx)

	msgCount := 0
	stream.sendFunc = func(event *chatpb.TopicEvent) error {
		stream.mu.Lock()
		stream.events = append(stream.events, event)
		msgCount++
		stream.mu.Unlock()
		// After receiving historical messages, cancel
		if msgCount >= 2 {
			cancel()
		}
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.StreamTopics(&chatpb.TopicStreamRequest{
			TopicPattern: "orders.*",
			SinceSeq:     1, // Replay messages after seq 1
		}, stream)
	}()

	err := <-errCh

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Canceled {
		t.Errorf("code = %v, want Canceled", st.Code())
	}

	evts := stream.getEvents()
	if len(evts) < 2 {
		t.Errorf("got %d historical events, want at least 2", len(evts))
	}
	for _, evt := range evts {
		if evt.Type != chatpb.TopicEventType_TOPIC_MESSAGE_PUBLISHED {
			t.Errorf("historical event type = %v, want TOPIC_MESSAGE_PUBLISHED", evt.Type)
		}
	}
}

func TestStreamTopics_HistoricalSendError(t *testing.T) {
	svc := setupTopicService()

	rm := svc.roomManager.Create("errors.topic", "Errors")
	creator := svc.memberManager.RequestJoin("errors.topic", "sys", "", 5*time.Minute)
	svc.memberManager.Approve(creator.ID, "admin")
	rm.SendMessage(creator.ID, "msg-1")
	rm.SendMessage(creator.ID, "msg-2")

	ctx := context.Background()
	stream := newMockTopicStream(ctx)
	stream.sendErr = errors.New("stream broken")

	err := svc.StreamTopics(&chatpb.TopicStreamRequest{
		TopicPattern: "errors.*",
		SinceSeq:     0, // Replay all
	}, stream)

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Aborted {
		t.Errorf("code = %v, want Aborted", st.Code())
	}
}

func TestStreamTopics_ActiveStreamCounter(t *testing.T) {
	svc := setupTopicService()

	// Verify starts at 0
	if svc.activeStreams.Load() != 0 {
		t.Fatalf("activeStreams = %d, want 0", svc.activeStreams.Load())
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockTopicStream(ctx)

	started := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		// Signal when stream is running
		close(started)
		errCh <- svc.StreamTopics(&chatpb.TopicStreamRequest{
			TopicPattern: "*",
		}, stream)
	}()

	<-started
	time.Sleep(50 * time.Millisecond)

	// Check active streams incremented
	active := svc.activeStreams.Load()
	if active != 1 {
		t.Errorf("activeStreams during stream = %d, want 1", active)
	}

	cancel()
	<-errCh

	// After stream ends, should be back to 0
	time.Sleep(10 * time.Millisecond)
	if svc.activeStreams.Load() != 0 {
		t.Errorf("activeStreams after stream = %d, want 0", svc.activeStreams.Load())
	}
}

func TestStreamTopics_NoMatchingTopics(t *testing.T) {
	svc := setupTopicService()

	// Create topics that don't match the pattern
	svc.roomManager.Create("users.login", "Users Login")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockTopicStream(ctx)

	// Cancel after a short time
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := svc.StreamTopics(&chatpb.TopicStreamRequest{
		TopicPattern: "orders.#",
	}, stream)

	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("expected gRPC status error")
	}
	if st.Code() != codes.Canceled {
		t.Errorf("code = %v, want Canceled", st.Code())
	}

	// No events should have been sent
	evts := stream.getEvents()
	if len(evts) != 0 {
		t.Errorf("got %d events, want 0 (no matching topics)", len(evts))
	}
}

func TestStreamTopics_MultipleMatchingTopics(t *testing.T) {
	svc := setupTopicService()

	svc.roomManager.Create("events.created", "Created")
	svc.roomManager.Create("events.updated", "Updated")
	svc.roomManager.Create("users.login", "Login")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockTopicStream(ctx)

	receivedCount := 0
	stream.sendFunc = func(event *chatpb.TopicEvent) error {
		stream.mu.Lock()
		stream.events = append(stream.events, event)
		receivedCount++
		if receivedCount >= 2 {
			cancel()
		}
		stream.mu.Unlock()
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.StreamTopics(&chatpb.TopicStreamRequest{
			TopicPattern: "events.*",
		}, stream)
	}()

	time.Sleep(100 * time.Millisecond)

	mbr := svc.memberManager.RequestJoin("events.created", "pub", "", 5*time.Minute)
	svc.memberManager.Approve(mbr.ID, "admin")

	// Publish to two different matching topics
	svc.wotan.PublishMessageCreated(&ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: mbr.ID,
		RoomID:    "events.created",
		Content:   "created event",
		Timestamp: time.Now(),
	})
	svc.wotan.PublishMessageCreated(&ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: mbr.ID,
		RoomID:    "events.updated",
		Content:   "updated event",
		Timestamp: time.Now(),
	})

	<-errCh

	evts := stream.getEvents()
	if len(evts) < 2 {
		t.Errorf("got %d events, want at least 2", len(evts))
	}
}

// --- forwardTopicEvents tests ---

func TestForwardTopicEvents_ContextCancel(t *testing.T) {
	svc := setupTopicService()

	ctx, cancel := context.WithCancel(context.Background())
	eventChan := make(chan *topicEvent, 10)

	memberID := uuid.New()
	sub := svc.wotan.Subscribe("test-topic", memberID, 100)

	done := make(chan struct{})
	go func() {
		svc.forwardTopicEvents(ctx, "test-topic", sub, eventChan)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// OK - goroutine exited
	case <-time.After(2 * time.Second):
		t.Fatal("forwardTopicEvents did not exit after context cancel")
	}
}

func TestForwardTopicEvents_ChannelClosed(t *testing.T) {
	svc := setupTopicService()

	ctx := context.Background()
	eventChan := make(chan *topicEvent, 10)

	memberID := uuid.New()
	sub := svc.wotan.Subscribe("test-topic", memberID, 100)

	done := make(chan struct{})
	go func() {
		svc.forwardTopicEvents(ctx, "test-topic", sub, eventChan)
		close(done)
	}()

	// Close the wotan to close subscription channels
	svc.wotan.Close()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("forwardTopicEvents did not exit after channel close")
	}
}

func TestForwardTopicEvents_ForwardsMessages(t *testing.T) {
	svc := setupTopicService()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventChan := make(chan *topicEvent, 10)

	memberID := uuid.New()
	sub := svc.wotan.Subscribe("fwd-topic", memberID, 100)

	go svc.forwardTopicEvents(ctx, "fwd-topic", sub, eventChan)

	// Publish a message
	svc.wotan.PublishMessageCreated(&ringbuffer.Message{
		ID:        uuid.New(),
		CreatorID: memberID,
		RoomID:    "fwd-topic",
		Content:   "forwarded",
		Timestamp: time.Now(),
	})

	select {
	case evt := <-eventChan:
		if evt.topic != "fwd-topic" {
			t.Errorf("topic = %q, want %q", evt.topic, "fwd-topic")
		}
		if evt.pbEvent == nil {
			t.Fatal("pbEvent is nil")
		}
		if evt.pbEvent.Seq <= 0 {
			t.Errorf("Seq = %d, want > 0", evt.pbEvent.Seq)
		}
		if string(evt.pbEvent.Payload) != "forwarded" {
			t.Errorf("Payload = %q, want %q", string(evt.pbEvent.Payload), "forwarded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for forwarded event")
	}
}

// --- Integration-style tests ---

func TestPublishThenStream(t *testing.T) {
	svc := setupTopicService()

	// Publish first, then stream with historical replay
	resp, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "integration.test",
		Payload: []byte("published before stream"),
	})
	if err != nil {
		t.Fatalf("publish error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockTopicStream(ctx)

	stream.sendFunc = func(event *chatpb.TopicEvent) error {
		stream.mu.Lock()
		stream.events = append(stream.events, event)
		stream.mu.Unlock()
		cancel()
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.StreamTopics(&chatpb.TopicStreamRequest{
			TopicPattern: "integration.*",
			SinceSeq:     0, // Replay all (messages after seq 0)
		}, stream)
	}()

	<-errCh

	evts := stream.getEvents()
	if len(evts) < 1 {
		t.Fatal("expected at least 1 replayed event")
	}

	// Verify the replayed message matches what was published
	found := false
	for _, evt := range evts {
		if string(evt.Payload) == "published before stream" {
			found = true
			break
		}
	}
	if !found {
		t.Error("did not find published message in replayed events")
	}
	_ = resp // use resp to verify correlation if needed
}

func TestPublishAndReceiveLive(t *testing.T) {
	svc := setupTopicService()

	// Pre-create the topic room so subscription can be established
	svc.roomManager.Create("live.topic", "Live Topic")

	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockTopicStream(ctx)

	stream.sendFunc = func(event *chatpb.TopicEvent) error {
		stream.mu.Lock()
		stream.events = append(stream.events, event)
		stream.mu.Unlock()
		cancel()
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.StreamTopics(&chatpb.TopicStreamRequest{
			TopicPattern: "live.*",
		}, stream)
	}()

	time.Sleep(100 * time.Millisecond)

	// Publish via the service API
	_, err := svc.PublishTopic(context.Background(), &chatpb.TopicPublishRequest{
		Topic:   "live.topic",
		Payload: []byte("live message"),
	})
	if err != nil {
		t.Fatalf("publish error: %v", err)
	}

	<-errCh

	evts := stream.getEvents()
	if len(evts) < 1 {
		t.Fatal("expected at least 1 live event")
	}
}
