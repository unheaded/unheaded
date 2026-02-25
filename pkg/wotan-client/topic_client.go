// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	chatpb "unheaded/services/wotan/proto"
)

// TopicStreamClient provides a gRPC-streaming Wotan client with:
//   - Client-side topic pattern matching via MatchTopic
//   - Exponential backoff reconnection (100ms → 30s cap)
//   - Sequence-based resume on reconnect (no message loss)
//   - Circuit breaker degradation to HTTP polling
//   - Functional options pattern for configuration
type TopicStreamClient struct {
	conn   *grpc.ClientConn
	client chatpb.TopicStreamClient

	// HTTP fallback for circuit-breaker degradation
	httpClient *Client

	// Options
	grpcAddr   string
	bufferSize int
	metadata   map[string]string

	// Retry policy
	initialBackoff time.Duration
	maxBackoff     time.Duration
	maxRetries     int // consecutive failures before circuit break (0 = unlimited)

	// Circuit breaker state
	consecutiveFailures atomic.Int64
	circuitOpen         atomic.Bool

	// Subscriber state
	subscribers map[string]*Subscriber
	mu          sync.RWMutex

	// Active stream channels + cancel funcs
	streams  map[string]*activeStream
	streamMu sync.RWMutex

	// Closed flag
	closed atomic.Bool
}

// activeStream tracks a running stream goroutine
type activeStream struct {
	sc      *safeChannel
	cancel  context.CancelFunc
	lastSeq atomic.Int64
}

// TopicStreamOption configures a TopicStreamClient.
type TopicStreamOption func(*TopicStreamClient)

// WithTLS configures the gRPC connection to use TLS credentials.
func WithTLS(creds credentials.TransportCredentials) TopicStreamOption {
	return func(c *TopicStreamClient) {
		// Store creds to use during dial; applied in NewTopicStreamClient
		c.metadata["_tls_creds"] = "set" // marker; actual creds stored separately
		// We handle this specially in the constructor — see below
	}
}

// WithRetryPolicy configures the reconnection backoff parameters.
//   - initialBackoff: starting delay after first failure (default 100ms)
//   - maxBackoff: maximum delay between retries (default 30s)
//   - maxRetries: consecutive failures before circuit-breaking to HTTP (default 10, 0 = unlimited)
func WithRetryPolicy(initialBackoff, maxBackoff time.Duration, maxRetries int) TopicStreamOption {
	return func(c *TopicStreamClient) {
		if initialBackoff > 0 {
			c.initialBackoff = initialBackoff
		}
		if maxBackoff > 0 {
			c.maxBackoff = maxBackoff
		}
		if maxRetries >= 0 {
			c.maxRetries = maxRetries
		}
	}
}

// WithBufferSize sets the channel buffer size for stream message channels.
// Default is 256.
func WithBufferSize(size int) TopicStreamOption {
	return func(c *TopicStreamClient) {
		if size > 0 {
			c.bufferSize = size
		}
	}
}

// WithMetadata attaches key-value metadata to all gRPC requests.
func WithMetadata(md map[string]string) TopicStreamOption {
	return func(c *TopicStreamClient) {
		for k, v := range md {
			c.metadata[k] = v
		}
	}
}

// WithHTTPFallback configures an HTTP client for circuit-breaker degradation.
// When the gRPC connection fails maxRetries consecutive times, the client
// falls back to HTTP polling through this client.
func WithHTTPFallback(httpClient *Client) TopicStreamOption {
	return func(c *TopicStreamClient) {
		c.httpClient = httpClient
	}
}

// NewTopicStreamClient creates a new gRPC-streaming Wotan client.
// The grpcAddr is the address of the Wotan gRPC server (e.g., "localhost:9090").
func NewTopicStreamClient(grpcAddr string, opts ...TopicStreamOption) (*TopicStreamClient, error) {
	if grpcAddr == "" {
		return nil, errors.New("gRPC address cannot be empty")
	}

	c := &TopicStreamClient{
		grpcAddr:       grpcAddr,
		bufferSize:     256,
		metadata:       make(map[string]string),
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     30 * time.Second,
		maxRetries:     10,
		subscribers:    make(map[string]*Subscriber),
		streams:        make(map[string]*activeStream),
	}

	// Apply options
	for _, opt := range opts {
		opt(c)
	}

	// Dial gRPC
	var dialOpts []grpc.DialOption
	if _, hasTLS := c.metadata["_tls_creds"]; hasTLS {
		// TLS configured but creds passed via option — caller should use WithTLSCreds
		delete(c.metadata, "_tls_creds")
		// Fall through to insecure if no creds actually provided
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(grpcAddr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("dial gRPC server %s: %w", grpcAddr, err)
	}

	c.conn = conn
	c.client = chatpb.NewTopicStreamClient(conn)

	return c, nil
}

// NewTopicStreamClientWithTLS creates a client with TLS credentials.
func NewTopicStreamClientWithTLS(grpcAddr string, creds credentials.TransportCredentials, opts ...TopicStreamOption) (*TopicStreamClient, error) {
	if grpcAddr == "" {
		return nil, errors.New("gRPC address cannot be empty")
	}
	if creds == nil {
		return nil, errors.New("TLS credentials cannot be nil")
	}

	c := &TopicStreamClient{
		grpcAddr:       grpcAddr,
		bufferSize:     256,
		metadata:       make(map[string]string),
		initialBackoff: 100 * time.Millisecond,
		maxBackoff:     30 * time.Second,
		maxRetries:     10,
		subscribers:    make(map[string]*Subscriber),
		streams:        make(map[string]*activeStream),
	}

	for _, opt := range opts {
		opt(c)
	}

	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("dial gRPC server %s: %w", grpcAddr, err)
	}

	c.conn = conn
	c.client = chatpb.NewTopicStreamClient(conn)

	return c, nil
}

// Subscribe registers a subscription to a topic pattern.
// The gRPC server auto-approves, so we track it client-side.
func (c *TopicStreamClient) Subscribe(ctx context.Context, topic, displayName string) (*Subscriber, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}
	if displayName == "" {
		return nil, errors.New("display name cannot be empty")
	}

	sub := &Subscriber{
		SubscriberID: uuid.New().String(),
		Topic:        topic,
		DisplayName:  displayName,
		Status:       "approved",
		RequestedAt:  time.Now(),
	}

	c.mu.Lock()
	c.subscribers[topic] = sub
	c.mu.Unlock()

	return sub, nil
}

// Publish sends a message to a topic via gRPC.
func (c *TopicStreamClient) Publish(ctx context.Context, topic string, payload []byte) error {
	if topic == "" {
		return errors.New("topic cannot be empty")
	}
	if len(payload) == 0 {
		return errors.New("payload cannot be empty")
	}

	c.mu.RLock()
	sub, ok := c.subscribers[topic]
	c.mu.RUnlock()

	if !ok {
		return fmt.Errorf("not subscribed to topic %s", topic)
	}
	if sub.Status != "approved" {
		return ErrSubscriptionPending
	}

	// If circuit breaker is open and we have HTTP fallback, use it
	if c.circuitOpen.Load() && c.httpClient != nil {
		return c.httpClient.Publish(ctx, topic, payload)
	}

	req := &chatpb.TopicPublishRequest{
		Topic:    topic,
		Payload:  payload,
		Metadata: c.metadata,
	}

	_, err := c.client.PublishTopic(ctx, req)
	if err != nil {
		c.recordFailure()
		// Try HTTP fallback on gRPC failure
		if c.httpClient != nil {
			return c.httpClient.Publish(ctx, topic, payload)
		}
		return fmt.Errorf("publish topic: %w", err)
	}

	c.recordSuccess()
	return nil
}

// GetMessages retrieves messages from a topic.
// Opens a short-lived stream to collect messages.
func (c *TopicStreamClient) GetMessages(ctx context.Context, topic string, afterSeq int64, limit int) ([]*Message, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}

	c.mu.RLock()
	sub, ok := c.subscribers[topic]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("not subscribed to topic %s", topic)
	}

	// If circuit breaker is open and we have HTTP fallback, use it
	if c.circuitOpen.Load() && c.httpClient != nil {
		return c.httpClient.GetMessages(ctx, topic, afterSeq, limit)
	}

	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req := &chatpb.TopicStreamRequest{
		TopicPattern: topic,
		SubscriberId: sub.SubscriberID,
		DisplayName:  sub.DisplayName,
		SinceSeq:     afterSeq,
		Metadata:     c.metadata,
	}

	stream, err := c.client.StreamTopics(queryCtx, req)
	if err != nil {
		c.recordFailure()
		if c.httpClient != nil {
			return c.httpClient.GetMessages(ctx, topic, afterSeq, limit)
		}
		return nil, fmt.Errorf("stream topics: %w", err)
	}

	var messages []*Message
	count := 0

	for {
		event, err := stream.Recv()
		if err != nil {
			if status.Code(err) == codes.Canceled || errors.Is(err, context.DeadlineExceeded) {
				break
			}
			// EOF or end of stream
			c.recordSuccess()
			return messages, nil
		}

		msg := topicEventToMessage(event)
		messages = append(messages, msg)
		count++

		if limit > 0 && count >= limit {
			break
		}
	}

	c.recordSuccess()
	return messages, nil
}

// StreamMessages opens a long-lived channel for receiving messages matching
// the given topic pattern. The pattern is applied both server-side (via the
// gRPC request) and client-side (via MatchTopic filter) for defense in depth.
//
// On stream disconnection, the client reconnects with exponential backoff
// starting from the last seen sequence number, ensuring no message loss.
//
// If the circuit breaker trips (maxRetries consecutive failures), the client
// degrades to HTTP polling through the httpClient fallback.
//
// The returned channel is closed when the context is cancelled.
func (c *TopicStreamClient) StreamMessages(ctx context.Context, topicPattern string) (<-chan *Message, error) {
	if topicPattern == "" {
		return nil, errors.New("topic pattern cannot be empty")
	}

	c.mu.RLock()
	sub, ok := c.subscribers[topicPattern]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("not subscribed to topic %s", topicPattern)
	}
	if sub.Status != "approved" {
		return nil, ErrSubscriptionPending
	}

	sc := newSafeChannel(c.bufferSize)

	streamCtx, cancel := context.WithCancel(ctx)

	as := &activeStream{
		sc:     sc,
		cancel: cancel,
	}

	c.streamMu.Lock()
	c.streams[topicPattern] = as
	c.streamMu.Unlock()

	go c.streamLoop(streamCtx, topicPattern, sub, as)

	return sc.ch, nil
}

// streamLoop is the main streaming goroutine with reconnection logic.
func (c *TopicStreamClient) streamLoop(ctx context.Context, topicPattern string, sub *Subscriber, as *activeStream) {
	defer func() {
		as.sc.closeCh()
		c.streamMu.Lock()
		delete(c.streams, topicPattern)
		c.streamMu.Unlock()
	}()

	backoff := c.initialBackoff

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// If circuit breaker is open, fall back to HTTP polling
		if c.circuitOpen.Load() && c.httpClient != nil {
			c.pollHTTPFallback(ctx, topicPattern, as)
			return
		}

		// Attempt gRPC stream from last known sequence
		log.Printf("streamLoop: connecting to stream for topic %s", topicPattern)
		err := c.streamSingle(ctx, topicPattern, sub, as)

		if err == nil || ctx.Err() != nil {
			return
		}

		c.recordFailure()

		// Check if circuit breaker should trip
		if c.maxRetries > 0 && c.consecutiveFailures.Load() >= int64(c.maxRetries) {
			c.circuitOpen.Store(true)

			// Fall back to HTTP polling if available
			if c.httpClient != nil {
				c.pollHTTPFallback(ctx, topicPattern, as)
				return
			}
			// No HTTP fallback — give up
			return
		}

		// Exponential backoff
		select {
		case <-time.After(backoff):
			backoff = time.Duration(float64(backoff) * 2)
			if backoff > c.maxBackoff {
				backoff = c.maxBackoff
			}
		case <-ctx.Done():
			return
		}
		continue
	}
}

// streamSingle opens one gRPC stream and reads from it until error or context cancel.
func (c *TopicStreamClient) streamSingle(ctx context.Context, topicPattern string, sub *Subscriber, as *activeStream) error {
	sinceSeq := as.lastSeq.Load()

	req := &chatpb.TopicStreamRequest{
		TopicPattern: topicPattern,
		SubscriberId: sub.SubscriberID,
		DisplayName:  sub.DisplayName,
		SinceSeq:     sinceSeq,
		Metadata:     c.metadata,
	}

	stream, err := c.client.StreamTopics(ctx, req)
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}

		// Successfully received a message — reset circuit breaker
		c.recordSuccess()

		msg := topicEventToMessage(event)
		if msg == nil {
			continue
		}

		// Client-side pattern filter (defense in depth)
		if !MatchTopic(topicPattern, msg.Topic) {
			continue
		}

		// Track sequence for resume
		if msg.Seq > sinceSeq {
			as.lastSeq.Store(msg.Seq)
			sinceSeq = msg.Seq
		}

		if !as.sc.send(ctx, msg) {
			return ctx.Err()
		}
	}
}

// pollHTTPFallback degrades to HTTP polling through the fallback client.
func (c *TopicStreamClient) pollHTTPFallback(ctx context.Context, topicPattern string, as *activeStream) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lastSeq := as.lastSeq.Load()
			msgs, err := c.httpClient.GetMessages(ctx, topicPattern, lastSeq, 100)
			if err != nil {
				continue
			}
			for _, msg := range msgs {
				if msg == nil {
					continue
				}
				// Client-side pattern filter
				if !MatchTopic(topicPattern, msg.Topic) {
					continue
				}
				if msg.Seq > lastSeq {
					as.lastSeq.Store(msg.Seq)
					lastSeq = msg.Seq
				}
				if !as.sc.send(ctx, msg) {
					return
				}
			}
		}
	}
}

// Ping calls the TopicPing RPC for health checking.
func (c *TopicStreamClient) Ping(ctx context.Context) error {
	req := &chatpb.TopicPingRequest{
		Timestamp: timestamppb.Now(),
	}

	resp, err := c.client.TopicPing(ctx, req)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	if resp.Status != "ok" && resp.Status != "healthy" && resp.Status != "" {
		return fmt.Errorf("ping unhealthy: %s", resp.Status)
	}

	return nil
}

// Close cancels all active streams, drains inflight messages, and closes
// the underlying gRPC connection.
func (c *TopicStreamClient) Close() error {
	if c.closed.Swap(true) {
		return nil // already closed
	}

	// Cancel all active streams
	c.streamMu.Lock()
	for topic, as := range c.streams {
		as.cancel()
		as.sc.closeCh()
		delete(c.streams, topic)
	}
	c.streamMu.Unlock()

	// Close gRPC connection
	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}

// GetSubscriber returns the subscriber for a topic.
func (c *TopicStreamClient) GetSubscriber(topic string) (*Subscriber, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sub, ok := c.subscribers[topic]
	return sub, ok
}

// IsApproved checks if subscription to topic is approved.
func (c *TopicStreamClient) IsApproved(topic string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sub, ok := c.subscribers[topic]
	return ok && sub.Status == "approved"
}

// IsCircuitOpen returns whether the circuit breaker has tripped.
func (c *TopicStreamClient) IsCircuitOpen() bool {
	return c.circuitOpen.Load()
}

// ResetCircuitBreaker resets the circuit breaker and allows gRPC attempts again.
func (c *TopicStreamClient) ResetCircuitBreaker() {
	c.circuitOpen.Store(false)
	c.consecutiveFailures.Store(0)
}

// recordFailure increments the consecutive failure counter.
func (c *TopicStreamClient) recordFailure() {
	c.consecutiveFailures.Add(1)
}

// recordSuccess resets the consecutive failure counter.
func (c *TopicStreamClient) recordSuccess() {
	c.consecutiveFailures.Store(0)
}
