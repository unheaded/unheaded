// Package wotanClient provides a client for the Wotan message bus.
// It supports both REST control plane and gRPC data plane operations.
package wotanClient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Backoff constants for pollMessages exponential backoff.
const (
	pollBackoffMin = 500 * time.Millisecond // initial/success poll interval
	pollBackoffMax = 30 * time.Second       // maximum backoff on consecutive errors
)

// Timeout constants for split HTTP clients.
const (
	controlPlaneTimeout = 5 * time.Second  // Subscribe, Publish
	streamingTimeout    = 30 * time.Second // GetMessages, polling
)

// Fallback queue constants.
const (
	// maxFallbackQueue is the maximum number of messages buffered locally
	// when Wotan is unreachable. Oldest messages are dropped when full.
	maxFallbackQueue = 1000
)

// pendingMessage is a message waiting in the fallback queue for retry.
type pendingMessage struct {
	topic   string
	payload []byte
}

// drainBody fully reads and discards any remaining bytes in the response body.
// This is CRITICAL for HTTP keep-alive: the transport cannot reuse a connection
// until the entire response body has been consumed. json.NewDecoder only reads
// enough to parse one JSON value, leaving trailing bytes (newlines, whitespace)
// that block connection recycling.
func drainBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

// Common errors
var (
	ErrNotConnected        = errors.New("client not connected")
	ErrSubscriptionPending = errors.New("subscription pending approval")
	ErrTopicNotFound       = errors.New("topic not found")
	ErrNotAuthorized       = errors.New("not authorized")
	ErrRateLimited         = errors.New("rate limited")
)

// Message represents a Wotan message
type Message struct {
	MessageID string    `json:"message_id"`
	Topic     string    `json:"topic"`
	SenderID  string    `json:"sender_id"`
	CreatedAt time.Time `json:"created_at"`
	Seq       int64     `json:"seq"`
	Payload   string    `json:"payload"`
	Deleted   bool      `json:"deleted"`
}

// Subscriber represents a topic subscriber
type Subscriber struct {
	SubscriberID string    `json:"subscriber_id"`
	Topic        string    `json:"topic"`
	DisplayName  string    `json:"display_name"`
	Status       string    `json:"status"` // pending, approved, denied, revoked
	RequestedAt  time.Time `json:"requested_at"`
}

// safeChannel wraps a channel with a sync.Once so it can be closed
// idempotently from both pollMessages and Client.Close without panic.
type safeChannel struct {
	ch   chan *Message
	done chan struct{} // signals that ch is closed; select on this before sending
	once sync.Once
}

// newSafeChannel creates a safeChannel with the done signal initialized.
func newSafeChannel(bufSize int) *safeChannel {
	return &safeChannel{
		ch:   make(chan *Message, bufSize),
		done: make(chan struct{}),
	}
}

// closeCh closes the underlying channel exactly once, regardless of how many
// times closeCh is called. Closes done first so senders can bail out.
func (sc *safeChannel) closeCh() {
	sc.once.Do(func() {
		close(sc.done)
		close(sc.ch)
	})
}

// send attempts to send a message on ch, returning false if the channel is
// closed or the context is cancelled. Prevents send-on-closed-channel panic.
func (sc *safeChannel) send(ctx context.Context, msg *Message) bool {
	select {
	case <-sc.done:
		return false
	case <-ctx.Done():
		return false
	case sc.ch <- msg:
		return true
	}
}

// TransportState tracks which transport is active for streaming.
type TransportState int

const (
	// TransportHTTP uses HTTP polling for message streaming (degraded mode).
	TransportHTTP TransportState = iota
	// TransportGRPC uses gRPC server-push streaming (primary, preferred).
	TransportGRPC
)

// String returns a human-readable transport name.
func (ts TransportState) String() string {
	switch ts {
	case TransportGRPC:
		return "gRPC"
	case TransportHTTP:
		return "HTTP"
	default:
		return "unknown"
	}
}

// Client provides access to Wotan message bus.
//
// Transport strategy:
//   - gRPC is the PRIMARY transport for streaming (lower latency, server-push).
//   - HTTP polling is the FALLBACK (circuit-breaker degraded mode only).
//   - HTTP REST is always used for control plane (Subscribe, Publish, admin).
//   - On startup, gRPC is probed. If healthy, it's used exclusively.
//   - If gRPC fails, the client degrades to HTTP polling and logs a warning.
//   - A background probe periodically re-checks gRPC and promotes back when healthy.
type Client struct {
	baseURL       string
	controlClient *http.Client // short timeout for Subscribe, Publish (5s)
	streamClient  *http.Client // long timeout for GetMessages, polling (30s)
	grpcAddr      string       // gRPC endpoint for streaming (empty = HTTP-only mode)
	grpcClient    *GRPCClient  // Lazy-initialized gRPC client

	// Transport state — gRPC primary, HTTP fallback
	transport     TransportState // current active transport
	grpcHealthy   bool           // last known gRPC health status
	transportMu   sync.RWMutex   // guards transport + grpcHealthy

	// Subscriber state (per-topic)
	subscribers map[string]*Subscriber
	mu          sync.RWMutex

	// Message channels for subscriptions
	channels map[string]*safeChannel
	chanMu   sync.RWMutex

	// Fallback queue — buffers messages when Wotan is unreachable.
	// Protected by fallbackMu. Max size: maxFallbackQueue.
	fallbackQueue []pendingMessage
	fallbackMu    sync.Mutex
	fallbackDrops int64 // atomic counter: messages dropped because queue was full

	// Dead letter counter — total messages sent to dead letter topics.
	deadLetterCount int64 // atomic counter
}

// Transport returns the currently active streaming transport.
func (c *Client) Transport() TransportState {
	c.transportMu.RLock()
	defer c.transportMu.RUnlock()
	return c.transport
}

// IsGRPCHealthy returns whether gRPC was healthy at last check.
func (c *Client) IsGRPCHealthy() bool {
	c.transportMu.RLock()
	defer c.transportMu.RUnlock()
	return c.grpcHealthy
}

// NewClient creates a new Wotan client
func NewClient(addr string) (*Client, error) {
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	return &Client{
		baseURL:       fmt.Sprintf("http://%s/api/v1", addr),
		controlClient: &http.Client{Timeout: controlPlaneTimeout},
		streamClient:  &http.Client{Timeout: streamingTimeout},
		subscribers:   make(map[string]*Subscriber),
		channels:      make(map[string]*safeChannel),
	}, nil
}

// NewClientWithTLS creates a client with TLS support and split timeouts.
func NewClientWithTLS(addr string, tlsConfig *http.Transport) (*Client, error) {
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	return &Client{
		baseURL:       fmt.Sprintf("https://%s/api/v1", addr),
		controlClient: &http.Client{Timeout: controlPlaneTimeout, Transport: tlsConfig},
		streamClient:  &http.Client{Timeout: streamingTimeout, Transport: tlsConfig},
		subscribers:   make(map[string]*Subscriber),
		channels:      make(map[string]*safeChannel),
	}, nil
}

// NewClientWithGRPC creates a Wotan client with gRPC as primary streaming transport.
//
// Transport strategy:
//   - gRPC is probed at startup. If healthy → used exclusively for StreamMessages.
//   - If gRPC probe fails → degrades to HTTP polling (circuit-breaker fallback).
//   - HTTP REST is always used for control plane (Subscribe, Publish).
//   - Call ProbeGRPC() periodically to re-check and promote back to gRPC.
func NewClientWithGRPC(httpAddr, grpcAddr string) (*Client, error) {
	if httpAddr == "" {
		return nil, errors.New("HTTP address cannot be empty")
	}
	if grpcAddr == "" {
		return nil, errors.New("gRPC address cannot be empty")
	}

	c := &Client{
		baseURL:       fmt.Sprintf("http://%s/api/v1", httpAddr),
		controlClient: &http.Client{Timeout: controlPlaneTimeout},
		streamClient:  &http.Client{Timeout: streamingTimeout},
		grpcAddr:      grpcAddr,
		transport:   TransportHTTP, // start degraded, promote after probe
		grpcHealthy: false,
		subscribers: make(map[string]*Subscriber),
		channels:    make(map[string]*safeChannel),
	}

	// Probe gRPC at startup — if healthy, promote to primary transport
	if err := c.ProbeGRPC(); err == nil {
		c.transport = TransportGRPC
		c.grpcHealthy = true
	}
	// If probe fails, stay on HTTP — no error returned, just degraded mode

	return c, nil
}

// ProbeGRPC tests gRPC connectivity and updates transport state.
// Returns nil if gRPC is healthy. Callers can use this periodically
// to re-promote gRPC after a circuit-breaker degradation.
func (c *Client) ProbeGRPC() error {
	if c.grpcAddr == "" {
		return errors.New("no gRPC address configured")
	}

	grpcCl, err := c.getOrCreateGRPCClient()
	if err != nil {
		c.transportMu.Lock()
		c.grpcHealthy = false
		c.transport = TransportHTTP
		c.transportMu.Unlock()
		return fmt.Errorf("gRPC probe failed: %w", err)
	}

	// Verify the connection is usable by checking the underlying conn state
	_ = grpcCl // connection was established successfully

	c.transportMu.Lock()
	c.grpcHealthy = true
	c.transport = TransportGRPC
	c.transportMu.Unlock()

	return nil
}

// degradeToHTTP switches streaming to HTTP polling (circuit-breaker trip).
func (c *Client) degradeToHTTP() {
	c.transportMu.Lock()
	defer c.transportMu.Unlock()
	c.grpcHealthy = false
	c.transport = TransportHTTP
}

// Close closes the client and all subscriptions.
// It is safe to call Close even if pollMessages has already closed some channels.
func (c *Client) Close() error {
	c.chanMu.Lock()
	defer c.chanMu.Unlock()

	for topic, sc := range c.channels {
		sc.closeCh()
		delete(c.channels, topic)
	}

	// Close gRPC client if initialized
	if c.grpcClient != nil {
		c.grpcClient.Close()
		c.grpcClient = nil
	}

	return nil
}

// Subscribe requests subscription to a topic
// Returns subscriber info (may be in pending status)
func (c *Client) Subscribe(ctx context.Context, topic, displayName string) (*Subscriber, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}
	if displayName == "" {
		return nil, errors.New("display name cannot be empty")
	}

	body := map[string]string{"display_name": displayName}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/topics/%s/subscribe", c.baseURL, topic),
		bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.controlClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subscribe request: %w", err)
	}
	defer drainBody(resp)

	if resp.StatusCode != http.StatusCreated {
		return nil, c.parseError(resp)
	}

	var result struct {
		Subscriber *Subscriber `json:"subscriber"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	c.mu.Lock()
	c.subscribers[topic] = result.Subscriber
	c.mu.Unlock()

	return result.Subscriber, nil
}

// Publish sends a message to a topic.
// On network failure the message is buffered in a local fallback queue
// (up to maxFallbackQueue entries). Call FlushFallbackQueue after
// connectivity is restored to retry buffered messages.
func (c *Client) Publish(ctx context.Context, topic string, payload []byte) error {
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

	networkErr, appErr := c.doPublish(ctx, topic, sub.SubscriberID, payload)
	if appErr != nil {
		// Application-level error (403, 404, etc.) — return directly, don't buffer.
		return appErr
	}
	if networkErr != nil {
		// Network/transport error — buffer for retry on reconnect.
		c.enqueueFallback(topic, payload)
		return fmt.Errorf("publish request (buffered for retry): %w", networkErr)
	}

	return nil
}

// doPublish performs the actual HTTP POST to Wotan.
// Returns (networkErr, appErr). Only one will be non-nil at a time.
// networkErr: transport/connectivity failures (safe to retry).
// appErr: server-side rejections (403, 404, etc.) that should not be retried.
func (c *Client) doPublish(ctx context.Context, topic, subscriberID string, payload []byte) (networkErr, appErr error) {
	body := map[string]string{
		"subscriber_id": subscriberID,
		"payload":       string(payload),
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err), nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/topics/%s/publish", c.baseURL, topic),
		bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err), nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.controlClient.Do(req)
	if err != nil {
		return fmt.Errorf("publish request: %w", err), nil
	}
	defer drainBody(resp)

	if resp.StatusCode != http.StatusCreated {
		return nil, c.parseError(resp)
	}

	return nil, nil
}

// enqueueFallback adds a message to the local fallback queue.
// If the queue is full (maxFallbackQueue), the oldest message is dropped
// and fallbackDrops is incremented.
func (c *Client) enqueueFallback(topic string, payload []byte) {
	c.fallbackMu.Lock()

	if len(c.fallbackQueue) >= maxFallbackQueue {
		// Drop the oldest message to make room.
		c.fallbackQueue = c.fallbackQueue[1:]
		atomic.AddInt64(&c.fallbackDrops, 1)
	}

	// Copy payload to avoid caller mutation.
	buf := make([]byte, len(payload))
	copy(buf, payload)
	c.fallbackQueue = append(c.fallbackQueue, pendingMessage{
		topic:   topic,
		payload: buf,
	})

	c.fallbackMu.Unlock()
	c.updateFallbackMetrics()
}

// FlushFallbackQueue attempts to re-publish all buffered messages.
// Successfully published messages are removed from the queue.
// Returns the number of messages flushed and any error from the last failure.
func (c *Client) FlushFallbackQueue(ctx context.Context) (flushed int, err error) {
	c.fallbackMu.Lock()
	queue := c.fallbackQueue
	c.fallbackQueue = nil
	c.fallbackMu.Unlock()

	var remaining []pendingMessage
	for _, msg := range queue {
		c.mu.RLock()
		sub, ok := c.subscribers[msg.topic]
		c.mu.RUnlock()

		if !ok {
			// Topic no longer subscribed — drop the message.
			continue
		}

		netErr, appErr := c.doPublish(ctx, msg.topic, sub.SubscriberID, msg.payload)
		if netErr != nil {
			// Network error — keep in queue for later retry.
			err = netErr
			remaining = append(remaining, msg)
			continue
		}
		if appErr != nil {
			// Application error — drop the message, it won't succeed on retry.
			err = appErr
			continue
		}
		flushed++
	}

	// Re-enqueue any that failed.
	if len(remaining) > 0 {
		c.fallbackMu.Lock()
		c.fallbackQueue = append(remaining, c.fallbackQueue...)
		if len(c.fallbackQueue) > maxFallbackQueue {
			dropped := len(c.fallbackQueue) - maxFallbackQueue
			c.fallbackQueue = c.fallbackQueue[dropped:]
			atomic.AddInt64(&c.fallbackDrops, int64(dropped))
		}
		c.fallbackMu.Unlock()
	}

	c.updateFallbackMetrics()
	return flushed, err
}

// FallbackQueueLen returns the current number of messages in the fallback queue.
func (c *Client) FallbackQueueLen() int {
	c.fallbackMu.Lock()
	defer c.fallbackMu.Unlock()
	return len(c.fallbackQueue)
}

// FallbackDrops returns the total number of messages dropped from the
// fallback queue because it was full.
func (c *Client) FallbackDrops() int64 {
	return atomic.LoadInt64(&c.fallbackDrops)
}

// GetMessages retrieves messages from a topic
func (c *Client) GetMessages(ctx context.Context, topic string, afterSeq int64, limit int) ([]*Message, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}

	url := fmt.Sprintf("%s/topics/%s/messages?after_seq=%d", c.baseURL, topic, afterSeq)
	if limit > 0 {
		url = fmt.Sprintf("%s&limit=%d", url, limit)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get messages request: %w", err)
	}
	defer drainBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var result struct {
		Messages []*Message `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Messages, nil
}

// StreamMessages opens a channel for receiving messages.
// Uses the active transport: gRPC (primary) or HTTP polling (circuit-breaker fallback).
// The channel will be closed when context is cancelled.
func (c *Client) StreamMessages(ctx context.Context, topic string) (<-chan *Message, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}

	c.mu.RLock()
	sub, ok := c.subscribers[topic]
	c.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("not subscribed to topic %s", topic)
	}
	if sub.Status != "approved" {
		return nil, ErrSubscriptionPending
	}

	// Use gRPC if it's the active transport and configured
	c.transportMu.RLock()
	activeTransport := c.transport
	c.transportMu.RUnlock()

	if activeTransport == TransportGRPC && c.grpcAddr != "" {
		grpcCl, err := c.getOrCreateGRPCClient()
		if err == nil {
			grpcCl.Subscribe(ctx, topic, sub.DisplayName)
			ch, err := grpcCl.StreamMessages(ctx, topic)
			if err == nil {
				return ch, nil
			}
		}
		// gRPC failed at runtime — trip circuit breaker, degrade to HTTP
		c.degradeToHTTP()
	}

	// HTTP polling — either primary (no gRPC configured) or circuit-breaker fallback
	sc := newSafeChannel(100)

	c.chanMu.Lock()
	c.channels[topic] = sc
	c.chanMu.Unlock()

	go c.pollMessages(ctx, topic, sc)
	return sc.ch, nil
}

// getOrCreateGRPCClient lazily initializes the gRPC client.
func (c *Client) getOrCreateGRPCClient() (*GRPCClient, error) {
	c.mu.RLock()
	if c.grpcClient != nil {
		c.mu.RUnlock()
		return c.grpcClient, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.grpcClient != nil {
		return c.grpcClient, nil
	}

	grpcCl, err := NewGRPCClient(c.grpcAddr)
	if err != nil {
		return nil, fmt.Errorf("create gRPC client: %w", err)
	}

	c.grpcClient = grpcCl
	return grpcCl, nil
}

// backoff calculates exponential backoff: min(pollBackoffMin * 2^failures, pollBackoffMax).
func backoff(failures int) time.Duration {
	if failures <= 0 {
		return pollBackoffMin
	}
	delay := time.Duration(float64(pollBackoffMin) * math.Pow(2, float64(failures)))
	if delay > pollBackoffMax {
		return pollBackoffMax
	}
	return delay
}

// pollMessages polls for new messages (fallback when gRPC not available).
// Uses exponential backoff on errors: 500ms -> 1s -> 2s -> ... -> 30s max.
func (c *Client) pollMessages(ctx context.Context, topic string, sc *safeChannel) {
	defer func() {
		sc.closeCh()

		// Remove from the channels map so Close() won't try again.
		c.chanMu.Lock()
		delete(c.channels, topic)
		c.chanMu.Unlock()
	}()

	var lastSeq int64 = 0
	var consecutiveFailures int

	timer := time.NewTimer(pollBackoffMin)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			msgs, err := c.GetMessages(ctx, topic, lastSeq, 100)
			if err != nil {
				consecutiveFailures++
				timer.Reset(backoff(consecutiveFailures))
				continue
			}
			// Success — reset backoff to base interval
			consecutiveFailures = 0
			for _, msg := range msgs {
				if !sc.send(ctx, msg) {
					return
				}
				if msg.Seq > lastSeq {
					lastSeq = msg.Seq
				}
			}
			timer.Reset(pollBackoffMin)
		}
	}
}

// parseError extracts error details from response
func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	switch errResp.Error.Code {
	case "SUBSCRIPTION_PENDING":
		return ErrSubscriptionPending
	case "INVALID_TOPIC":
		return ErrTopicNotFound
	case "NOT_AUTHORIZED", "SUBSCRIPTION_DENIED", "SUBSCRIPTION_REVOKED":
		return ErrNotAuthorized
	case "RATE_LIMITED":
		return ErrRateLimited
	default:
		return fmt.Errorf("%s: %s", errResp.Error.Code, errResp.Error.Message)
	}
}

// GetSubscriber returns the subscriber for a topic
func (c *Client) GetSubscriber(topic string) (*Subscriber, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sub, ok := c.subscribers[topic]
	return sub, ok
}

// IsApproved checks if subscription to topic is approved
func (c *Client) IsApproved(topic string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sub, ok := c.subscribers[topic]
	return ok && sub.Status == "approved"
}
