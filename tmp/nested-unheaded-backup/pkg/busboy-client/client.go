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

// Client provides access to Busboy message bus
type Client struct {
	baseURL    string
	httpClient *http.Client

	// Subscriber state (per-topic)
	subscribers map[string]*Subscriber
	mu          sync.RWMutex

	// Message channels for subscriptions
	channels map[string]chan *Message
	chanMu   sync.RWMutex
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
		channels:    make(map[string]chan *Message),
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
		channels:    make(map[string]chan *Message),
	}, nil
}

// Close closes the client and all subscriptions
func (c *Client) Close() error {
	c.chanMu.Lock()
	defer c.chanMu.Unlock()

	for topic, ch := range c.channels {
		close(ch)
		delete(c.channels, topic)
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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

// StreamMessages opens a channel for receiving messages
// The channel will be closed when context is cancelled
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

	ch := make(chan *Message, 100)

	c.chanMu.Lock()
	c.channels[topic] = ch
	c.chanMu.Unlock()

	// Start polling goroutine (TODO: replace with gRPC streaming)
	go c.pollMessages(ctx, topic, ch)

	return ch, nil
}

// pollMessages polls for new messages (fallback when gRPC not available)
func (c *Client) pollMessages(ctx context.Context, topic string, ch chan *Message) {
	defer close(ch)

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
				case ch <- msg:
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
