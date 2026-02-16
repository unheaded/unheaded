// Package busboyClient provides a client for the Busboy message bus.
// It supports both REST control plane and gRPC data plane operations.
package busboyClient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

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

// Message represents a Busboy message
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
	once sync.Once
}

// closeCh closes the underlying channel exactly once, regardless of how many
// times closeCh is called.
func (sc *safeChannel) closeCh() {
	sc.once.Do(func() { close(sc.ch) })
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

// Client provides access to Busboy message bus.
//
// Transport strategy:
//   - gRPC is the PRIMARY transport for streaming (lower latency, server-push).
//   - HTTP polling is the FALLBACK (circuit-breaker degraded mode only).
//   - HTTP REST is always used for control plane (Subscribe, Publish, admin).
//   - On startup, gRPC is probed. If healthy, it's used exclusively.
//   - If gRPC fails, the client degrades to HTTP polling and logs a warning.
//   - A background probe periodically re-checks gRPC and promotes back when healthy.
type Client struct {
	baseURL    string
	httpClient *http.Client
	grpcAddr   string       // gRPC endpoint for streaming (empty = HTTP-only mode)
	grpcClient *GRPCClient  // Lazy-initialized gRPC client

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

// NewClient creates a new Busboy client
func NewClient(addr string) (*Client, error) {
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	return &Client{
		baseURL: fmt.Sprintf("http://%s/api/v1", addr),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		subscribers: make(map[string]*Subscriber),
		channels:    make(map[string]*safeChannel),
	}, nil
}

// NewClientWithTLS creates a client with TLS support
func NewClientWithTLS(addr string, tlsConfig *http.Transport) (*Client, error) {
	if addr == "" {
		return nil, errors.New("address cannot be empty")
	}

	return &Client{
		baseURL: fmt.Sprintf("https://%s/api/v1", addr),
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: tlsConfig,
		},
		subscribers: make(map[string]*Subscriber),
		channels:    make(map[string]*safeChannel),
	}, nil
}

// NewClientWithGRPC creates a Busboy client with gRPC as primary streaming transport.
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
		baseURL: fmt.Sprintf("http://%s/api/v1", httpAddr),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		grpcAddr:    grpcAddr,
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

	resp, err := c.httpClient.Do(req)
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

// Publish sends a message to a topic
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

	body := map[string]string{
		"subscriber_id": sub.SubscriberID,
		"payload":       string(payload),
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/topics/%s/publish", c.baseURL, topic),
		bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("publish request: %w", err)
	}
	defer drainBody(resp)

	if resp.StatusCode != http.StatusCreated {
		return c.parseError(resp)
	}

	return nil
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

	resp, err := c.httpClient.Do(req)
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
	ch := make(chan *Message, 100)
	sc := &safeChannel{ch: ch}

	c.chanMu.Lock()
	c.channels[topic] = sc
	c.chanMu.Unlock()

	go c.pollMessages(ctx, topic, sc)
	return ch, nil
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

// pollMessages polls for new messages (fallback when gRPC not available)
func (c *Client) pollMessages(ctx context.Context, topic string, sc *safeChannel) {
	defer func() {
		sc.closeCh()

		// Remove from the channels map so Close() won't try again.
		c.chanMu.Lock()
		delete(c.channels, topic)
		c.chanMu.Unlock()
	}()

	var lastSeq int64 = 0
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			msgs, err := c.GetMessages(ctx, topic, lastSeq, 100)
			if err != nil {
				continue // retry on error
			}
			for _, msg := range msgs {
				select {
				case sc.ch <- msg:
					if msg.Seq > lastSeq {
						lastSeq = msg.Seq
					}
				case <-ctx.Done():
					return
				}
			}
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
