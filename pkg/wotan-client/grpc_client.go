// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	chatpb "unheaded/services/wotan/proto"
)

// GRPCClient provides a gRPC-based Wotan client
// Implements the WotanClient interface from kanban-app/wotan.go
type GRPCClient struct {
	conn   *grpc.ClientConn
	client chatpb.TopicStreamClient

	// Local subscriber state (same pattern as HTTP Client)
	subscribers map[string]*Subscriber
	mu          sync.RWMutex

	// Active stream channels
	channels map[string]*safeChannel
	chanMu   sync.RWMutex

	// Reconnection settings
	maxRetries int
	retryDelay time.Duration
}

// NewGRPCClient creates a new gRPC-based Wotan client.
// Set UNHEADED_GRPC_TLS=1 to enable TLS 1.3 transport credentials.
func NewGRPCClient(addr string) (*GRPCClient, error) {
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	var creds credentials.TransportCredentials
	if os.Getenv("UNHEADED_GRPC_TLS") == "1" {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS13})
	} else {
		creds = insecure.NewCredentials()
	}

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("dial grpc server: %w", err)
	}

	client := chatpb.NewTopicStreamClient(conn)

	return &GRPCClient{
		conn:        conn,
		client:      client,
		subscribers: make(map[string]*Subscriber),
		channels:    make(map[string]*safeChannel),
		maxRetries:  10,
		retryDelay:  100 * time.Millisecond,
	}, nil
}

// NewGRPCClientWithTLS creates a new gRPC-based Wotan client with TLS credentials
func NewGRPCClientWithTLS(addr string, creds credentials.TransportCredentials) (*GRPCClient, error) {
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}
	if creds == nil {
		return nil, errors.New("credentials cannot be nil")
	}

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("dial grpc server: %w", err)
	}

	client := chatpb.NewTopicStreamClient(conn)

	return &GRPCClient{
		conn:        conn,
		client:      client,
		subscribers: make(map[string]*Subscriber),
		channels:    make(map[string]*safeChannel),
		maxRetries:  10,
		retryDelay:  100 * time.Millisecond,
	}, nil
}

// Subscribe requests subscription to a topic
// The gRPC server auto-approves, so we track it client-side
func (gc *GRPCClient) Subscribe(ctx context.Context, topic, displayName string) (*Subscriber, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}
	if displayName == "" {
		return nil, errors.New("display name cannot be empty")
	}

	// Generate a UUID for the subscriber ID
	subscriberID := uuid.New().String()

	// Create and store subscriber locally (auto-approved)
	sub := &Subscriber{
		SubscriberID: subscriberID,
		Topic:        topic,
		DisplayName:  displayName,
		Status:       "approved",
		RequestedAt:  time.Now(),
	}

	gc.mu.Lock()
	gc.subscribers[topic] = sub
	gc.mu.Unlock()

	return sub, nil
}

// Publish sends a message to a topic
// Must be subscribed first
func (gc *GRPCClient) Publish(ctx context.Context, topic string, payload []byte) error {
	if topic == "" {
		return errors.New("topic cannot be empty")
	}
	if len(payload) == 0 {
		return errors.New("payload cannot be empty")
	}

	gc.mu.RLock()
	sub, ok := gc.subscribers[topic]
	gc.mu.RUnlock()

	if !ok {
		return fmt.Errorf("not subscribed to topic %s", topic)
	}
	if sub.Status != "approved" {
		return ErrSubscriptionPending
	}

	req := &chatpb.TopicPublishRequest{
		Topic:   topic,
		Payload: payload,
		// SenderId intentionally empty — server auto-creates and approves
		// a publisher member. The local subscriber ID is client-side only
		// and not registered with the server's member manager.
	}
	_ = sub // subscriber validated above for status check

	_, err := gc.client.PublishTopic(ctx, req)
	if err != nil {
		return fmt.Errorf("publish topic: %w", err)
	}

	return nil
}

// GetMessages retrieves messages from a topic
// Opens a short-lived stream to collect messages
func (gc *GRPCClient) GetMessages(ctx context.Context, topic string, afterSeq int64, limit int) ([]*Message, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}

	// Create a request context with timeout for this operation
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	gc.mu.RLock()
	sub, ok := gc.subscribers[topic]
	gc.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("not subscribed to topic %s", topic)
	}

	req := &chatpb.TopicStreamRequest{
		TopicPattern: topic,
		SubscriberId: sub.SubscriberID,
		DisplayName:  sub.DisplayName,
		SinceSeq:     afterSeq,
	}

	stream, err := gc.client.StreamTopics(queryCtx, req)
	if err != nil {
		return nil, fmt.Errorf("stream topics: %w", err)
	}

	var messages []*Message
	count := 0

	for {
		event, err := stream.Recv()
		if err != nil {
			// End of stream is not an error
			if status.Code(err) == codes.Canceled {
				break
			}
			if errors.Is(err, context.DeadlineExceeded) {
				break
			}
			// EOF is expected for bounded queries
			return messages, nil
		}

		msg := topicEventToMessage(event)
		messages = append(messages, msg)
		count++

		// Stop after limit is reached
		if limit > 0 && count >= limit {
			break
		}
	}

	return messages, nil
}

// StreamMessages opens a long-lived stream channel for receiving messages
// This is the key method — replaces HTTP polling
// Must be subscribed first
func (gc *GRPCClient) StreamMessages(ctx context.Context, topic string) (<-chan *Message, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}

	gc.mu.RLock()
	sub, ok := gc.subscribers[topic]
	gc.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("not subscribed to topic %s", topic)
	}
	if sub.Status != "approved" {
		return nil, ErrSubscriptionPending
	}

	// Create buffered channel
	sc := newSafeChannel(100)

	gc.chanMu.Lock()
	gc.channels[topic] = sc
	gc.chanMu.Unlock()

	// Start goroutine that handles the stream with reconnection logic
	go gc.streamMessagesWithRetry(ctx, topic, sub, sc)

	return sc.ch, nil
}

// streamMessagesWithRetry handles stream reading with exponential backoff retry
func (gc *GRPCClient) streamMessagesWithRetry(ctx context.Context, topic string, sub *Subscriber, sc *safeChannel) {
	defer func() {
		sc.closeCh()
		gc.chanMu.Lock()
		delete(gc.channels, topic)
		gc.chanMu.Unlock()
	}()

	retryCount := 0
	retryDelay := gc.retryDelay

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Attempt to stream
		err := gc.streamMessagesSingle(ctx, topic, sub, sc)
		if err != nil {
			// Check if it's a transient error
			if isTransientError(err) && retryCount < gc.maxRetries {
				retryCount++
				// Exponential backoff with jitter
				select {
				case <-time.After(retryDelay):
					retryDelay = time.Duration(float64(retryDelay) * 1.5)
					if retryDelay > 10*time.Second {
						retryDelay = 10 * time.Second
					}
					continue
				case <-ctx.Done():
					return
				}
			}
			// Non-transient error or max retries exceeded
			return
		}
		// Successful stream ended normally
		return
	}
}

// streamMessagesSingle opens a single stream and reads from it
func (gc *GRPCClient) streamMessagesSingle(ctx context.Context, topic string, sub *Subscriber, sc *safeChannel) error {
	req := &chatpb.TopicStreamRequest{
		TopicPattern: topic,
		SubscriberId: sub.SubscriberID,
		DisplayName:  sub.DisplayName,
	}

	stream, err := gc.client.StreamTopics(ctx, req)
	if err != nil {
		return fmt.Errorf("stream topics: %w", err)
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("receive from stream: %w", err)
		}

		msg := topicEventToMessage(event)

		if !sc.send(ctx, msg) {
			return ctx.Err()
		}
	}
}

// Close closes the gRPC client and all subscriptions
func (gc *GRPCClient) Close() error {
	gc.chanMu.Lock()
	defer gc.chanMu.Unlock()

	// Close all message channels
	for topic, sc := range gc.channels {
		sc.closeCh()
		delete(gc.channels, topic)
	}

	// Close gRPC connection
	if gc.conn != nil {
		return gc.conn.Close()
	}

	return nil
}

// GetSubscriber returns the subscriber for a topic
func (gc *GRPCClient) GetSubscriber(topic string) (*Subscriber, bool) {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	sub, ok := gc.subscribers[topic]
	return sub, ok
}

// IsApproved checks if subscription to topic is approved
func (gc *GRPCClient) IsApproved(topic string) bool {
	gc.mu.RLock()
	defer gc.mu.RUnlock()
	sub, ok := gc.subscribers[topic]
	return ok && sub.Status == "approved"
}

// ============================================================================
// HELPERS
// ============================================================================

// topicEventToMessage converts a gRPC TopicEvent to a Message
func topicEventToMessage(event *chatpb.TopicEvent) *Message {
	if event == nil {
		return nil
	}

	createdAt := time.Now()
	if event.CreatedAt != nil {
		createdAt = event.CreatedAt.AsTime()
	}

	return &Message{
		MessageID: event.MessageId,
		Topic:     event.Topic,
		SenderID:  event.SenderId,
		CreatedAt: createdAt,
		Seq:       event.Seq,
		Payload:   string(event.Payload),
		Deleted:   event.Type == chatpb.TopicEventType_TOPIC_MESSAGE_DELETED,
	}
}

// isTransientError determines if an error is transient and should trigger a retry
func isTransientError(err error) bool {
	if err == nil {
		return false
	}

	// Check gRPC status code
	st, ok := status.FromError(err)
	if ok {
		switch st.Code() {
		case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded:
			return true
		default:
			return false
		}
	}

	// Check context errors
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	return false
}
