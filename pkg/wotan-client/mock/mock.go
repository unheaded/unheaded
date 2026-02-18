// Package mock provides a mock Wotan client for testing.
// It implements the same interface as the real client without network calls.
package mock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// MockClient provides a testable Wotan client implementation
type MockClient struct {
	// Configuration
	autoApprove bool
	latency     time.Duration

	// State
	subscribers map[string]*wotanClient.Subscriber
	messages    map[string][]*wotanClient.Message
	channels    map[string]chan *wotanClient.Message
	closed      bool

	// Counters for assertions
	publishCount   int64
	subscribeCount int64

	// Error injection
	subscribeErr error
	publishErr   error
	getMessagesErr error

	mu sync.RWMutex
}

// Option configures the mock client
type Option func(*MockClient)

// WithAutoApprove automatically approves subscriptions
func WithAutoApprove() Option {
	return func(m *MockClient) {
		m.autoApprove = true
	}
}

// WithLatency adds simulated latency to operations
func WithLatency(d time.Duration) Option {
	return func(m *MockClient) {
		m.latency = d
	}
}

// WithSubscribeError injects an error for Subscribe calls
func WithSubscribeError(err error) Option {
	return func(m *MockClient) {
		m.subscribeErr = err
	}
}

// WithPublishError injects an error for Publish calls
func WithPublishError(err error) Option {
	return func(m *MockClient) {
		m.publishErr = err
	}
}

// WithGetMessagesError injects an error for GetMessages calls
func WithGetMessagesError(err error) Option {
	return func(m *MockClient) {
		m.getMessagesErr = err
	}
}

// NewMockClient creates a new mock Wotan client
func NewMockClient(opts ...Option) *MockClient {
	m := &MockClient{
		subscribers: make(map[string]*wotanClient.Subscriber),
		messages:    make(map[string][]*wotanClient.Message),
		channels:    make(map[string]chan *wotanClient.Message),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Close closes the mock client
func (m *MockClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	for topic, ch := range m.channels {
		close(ch)
		delete(m.channels, topic)
	}
	return nil
}

// Subscribe mocks subscribing to a topic
func (m *MockClient) Subscribe(ctx context.Context, topic, displayName string) (*wotanClient.Subscriber, error) {
	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}

	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}
	if displayName == "" {
		return nil, errors.New("display name cannot be empty")
	}

	m.simulateLatency()
	atomic.AddInt64(&m.subscribeCount, 1)

	m.mu.Lock()
	defer m.mu.Unlock()

	status := "pending"
	if m.autoApprove {
		status = "approved"
	}

	sub := &wotanClient.Subscriber{
		SubscriberID: generateID(),
		Topic:        topic,
		DisplayName:  displayName,
		Status:       status,
		RequestedAt:  time.Now(),
	}

	m.subscribers[topic] = sub
	return sub, nil
}

// Publish mocks publishing a message
func (m *MockClient) Publish(ctx context.Context, topic string, payload []byte) error {
	if m.publishErr != nil {
		return m.publishErr
	}

	if topic == "" {
		return errors.New("topic cannot be empty")
	}
	if len(payload) == 0 {
		return errors.New("payload cannot be empty")
	}

	m.mu.RLock()
	sub, ok := m.subscribers[topic]
	var status, subscriberID string
	if ok {
		status = sub.Status
		subscriberID = sub.SubscriberID
	}
	m.mu.RUnlock()

	if !ok {
		return errors.New("not subscribed to topic")
	}
	if status != "approved" {
		return wotanClient.ErrSubscriptionPending
	}

	m.simulateLatency()
	atomic.AddInt64(&m.publishCount, 1)

	m.mu.Lock()
	defer m.mu.Unlock()

	msg := &wotanClient.Message{
		MessageID: generateID(),
		Topic:     topic,
		SenderID:  subscriberID,
		CreatedAt: time.Now(),
		Seq:       int64(len(m.messages[topic]) + 1),
		Payload:   string(payload),
	}

	m.messages[topic] = append(m.messages[topic], msg)

	// Deliver to any active streams
	if ch, ok := m.channels[topic]; ok {
		select {
		case ch <- msg:
		default:
			// Channel full, drop message (same as real behavior with bounded buffer)
		}
	}

	return nil
}

// GetMessages mocks retrieving messages
func (m *MockClient) GetMessages(ctx context.Context, topic string, afterSeq int64, limit int) ([]*wotanClient.Message, error) {
	if m.getMessagesErr != nil {
		return nil, m.getMessagesErr
	}

	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}

	m.simulateLatency()

	m.mu.RLock()
	defer m.mu.RUnlock()

	msgs := m.messages[topic]
	var result []*wotanClient.Message

	for _, msg := range msgs {
		if msg.Seq > afterSeq {
			result = append(result, msg)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// StreamMessages mocks message streaming
func (m *MockClient) StreamMessages(ctx context.Context, topic string) (<-chan *wotanClient.Message, error) {
	if topic == "" {
		return nil, errors.New("topic cannot be empty")
	}

	m.mu.RLock()
	sub, ok := m.subscribers[topic]
	var status string
	if ok {
		status = sub.Status
	}
	m.mu.RUnlock()

	if !ok {
		return nil, errors.New("not subscribed to topic")
	}
	if status != "approved" {
		return nil, wotanClient.ErrSubscriptionPending
	}

	ch := make(chan *wotanClient.Message, 100)

	m.mu.Lock()
	m.channels[topic] = ch
	m.mu.Unlock()

	// Close channel when context is cancelled
	go func() {
		<-ctx.Done()
		m.mu.Lock()
		if c, ok := m.channels[topic]; ok && c == ch {
			close(ch)
			delete(m.channels, topic)
		}
		m.mu.Unlock()
	}()

	return ch, nil
}

// GetSubscriber returns the subscriber for a topic
func (m *MockClient) GetSubscriber(topic string) (*wotanClient.Subscriber, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sub, ok := m.subscribers[topic]
	return sub, ok
}

// IsApproved checks if subscription is approved
func (m *MockClient) IsApproved(topic string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sub, ok := m.subscribers[topic]
	return ok && sub.Status == "approved"
}

// ============================================================================
// TEST HELPERS
// ============================================================================

// ApproveSubscription manually approves a pending subscription
func (m *MockClient) ApproveSubscription(topic string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subscribers[topic]
	if !ok {
		return errors.New("no subscription for topic")
	}
	sub.Status = "approved"
	return nil
}

// DenySubscription denies a subscription
func (m *MockClient) DenySubscription(topic string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subscribers[topic]
	if !ok {
		return errors.New("no subscription for topic")
	}
	sub.Status = "denied"
	return nil
}

// InjectMessage adds a message directly (for testing receivers)
func (m *MockClient) InjectMessage(topic string, payload string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg := &wotanClient.Message{
		MessageID: generateID(),
		Topic:     topic,
		SenderID:  "injected",
		CreatedAt: time.Now(),
		Seq:       int64(len(m.messages[topic]) + 1),
		Payload:   payload,
	}

	m.messages[topic] = append(m.messages[topic], msg)

	if ch, ok := m.channels[topic]; ok {
		select {
		case ch <- msg:
		default:
		}
	}
}

// GetPublishCount returns number of Publish calls
func (m *MockClient) GetPublishCount() int64 {
	return atomic.LoadInt64(&m.publishCount)
}

// GetSubscribeCount returns number of Subscribe calls
func (m *MockClient) GetSubscribeCount() int64 {
	return atomic.LoadInt64(&m.subscribeCount)
}

// Reset clears all state
func (m *MockClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ch := range m.channels {
		close(ch)
	}

	m.subscribers = make(map[string]*wotanClient.Subscriber)
	m.messages = make(map[string][]*wotanClient.Message)
	m.channels = make(map[string]chan *wotanClient.Message)
	atomic.StoreInt64(&m.publishCount, 0)
	atomic.StoreInt64(&m.subscribeCount, 0)
}

// SetError sets error injection for a specific method
func (m *MockClient) SetError(method string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch method {
	case "subscribe":
		m.subscribeErr = err
	case "publish":
		m.publishErr = err
	case "getMessages":
		m.getMessagesErr = err
	}
}

// ClearErrors removes all injected errors
func (m *MockClient) ClearErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.subscribeErr = nil
	m.publishErr = nil
	m.getMessagesErr = nil
}

// ============================================================================
// INTERNAL
// ============================================================================

func (m *MockClient) simulateLatency() {
	if m.latency > 0 {
		time.Sleep(m.latency)
	}
}

var idCounter int64

func generateID() string {
	return time.Now().Format("20060102150405") + "-" +
		string(rune('a'+atomic.AddInt64(&idCounter, 1)%26))
}
