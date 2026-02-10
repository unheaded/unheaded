// Package notify provides Busboy event notification channel implementation.
// Busboy is the Kingdom's internal event bus for inter-service communication.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"unheaded/pkg/alerting"
)

// BusboyConfig holds Busboy channel configuration.
type BusboyConfig struct {
	ChannelConfig
	Topic          string            `json:"topic"`
	Partition      int               `json:"partition"`
	QueueSize      int               `json:"queue_size"`
	BatchSize      int               `json:"batch_size"`
	FlushInterval  time.Duration     `json:"flush_interval"`
	RetryCount     int               `json:"retry_count"`
	RetryDelay     time.Duration     `json:"retry_delay"`
	Headers        map[string]string `json:"headers"`
}

// BusboyEvent represents an event sent to the Busboy event bus.
type BusboyEvent struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Source      string            `json:"source"`
	Subject     string            `json:"subject"`
	Time        time.Time         `json:"time"`
	DataSchema  string            `json:"data_schema"`
	ContentType string            `json:"content_type"`
	Data        *BusboyAlertData  `json:"data"`
	Headers     map[string]string `json:"headers"`
}

// BusboyAlertData represents the alert data payload.
type BusboyAlertData struct {
	GroupKey     string            `json:"group_key"`
	Status       string            `json:"status"`
	Receiver     string            `json:"receiver"`
	GroupLabels  map[string]string `json:"group_labels"`
	CommonLabels map[string]string `json:"common_labels"`
	Alerts       []BusboyAlert     `json:"alerts"`
}

// BusboyAlert represents an alert in the Busboy payload.
type BusboyAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	Severity     string            `json:"severity"`
	Value        float64           `json:"value"`
	Threshold    float64           `json:"threshold"`
	StartsAt     time.Time         `json:"starts_at"`
	EndsAt       time.Time         `json:"ends_at"`
	Fingerprint  string            `json:"fingerprint"`
	GeneratorURL string            `json:"generator_url"`
	Acknowledged bool              `json:"acknowledged"`
}

// BusboyPublisher is the interface for publishing to the event bus.
type BusboyPublisher interface {
	Publish(ctx context.Context, topic string, event *BusboyEvent) error
	PublishBatch(ctx context.Context, topic string, events []*BusboyEvent) error
	Close() error
}

// BusboyChannel implements the NotificationChannel interface for Busboy.
type BusboyChannel struct {
	config    BusboyConfig
	publisher BusboyPublisher
	queue     chan *BusboyEvent
	mu        sync.Mutex
	closed    bool
	wg        sync.WaitGroup
}

// NewBusboyChannel creates a new Busboy notification channel.
func NewBusboyChannel(config BusboyConfig, publisher BusboyPublisher) *BusboyChannel {
	if config.QueueSize == 0 {
		config.QueueSize = 1000
	}
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.FlushInterval == 0 {
		config.FlushInterval = 5 * time.Second
	}
	if config.RetryCount == 0 {
		config.RetryCount = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	}
	if config.Topic == "" {
		config.Topic = "kingdom.alerts"
	}

	ch := &BusboyChannel{
		config:    config,
		publisher: publisher,
		queue:     make(chan *BusboyEvent, config.QueueSize),
	}

	// Start background flusher
	ch.wg.Add(1)
	go ch.flushLoop()

	return ch
}

// Name returns the channel name.
func (b *BusboyChannel) Name() string {
	return b.config.Name
}

// Send sends alert notifications to Busboy.
func (b *BusboyChannel) Send(ctx context.Context, alerts []*alerting.Alert) error {
	if len(alerts) == 0 {
		return nil
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return fmt.Errorf("%w: channel is closed", alerting.ErrNotificationError)
	}
	b.mu.Unlock()

	event := b.buildEvent(alerts)

	select {
	case b.queue <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Queue is full, try direct publish
		return b.publishWithRetry(ctx, event)
	}
}

// buildEvent builds a Busboy event from alerts.
func (b *BusboyChannel) buildEvent(alerts []*alerting.Alert) *BusboyEvent {
	busboyAlerts := make([]BusboyAlert, len(alerts))
	commonLabels := make(map[string]string)
	groupLabels := make(map[string]string)

	status := "resolved"
	for i, alert := range alerts {
		if alert.State == alerting.AlertStateFiring {
			status = "firing"
		}

		busboyAlerts[i] = BusboyAlert{
			Status:       string(alert.State),
			Labels:       alert.Labels,
			Annotations:  alert.Annotations,
			Severity:     string(alert.Severity),
			Value:        alert.Value,
			Threshold:    alert.Threshold,
			StartsAt:     alert.StartsAt,
			EndsAt:       alert.EndsAt,
			Fingerprint:  alert.Fingerprint,
			GeneratorURL: alert.GeneratorURL,
			Acknowledged: alert.Acknowledged,
		}

		// Calculate common labels
		if i == 0 {
			for k, v := range alert.Labels {
				commonLabels[k] = v
			}
		} else {
			for k, v := range commonLabels {
				if alert.Labels[k] != v {
					delete(commonLabels, k)
				}
			}
		}
	}

	for k, v := range commonLabels {
		groupLabels[k] = v
	}

	groupKey := ""
	if len(alerts) > 0 {
		groupKey = alerts[0].Fingerprint
	}

	return &BusboyEvent{
		ID:          fmt.Sprintf("alert-%d", time.Now().UnixNano()),
		Type:        "kingdom.alert.notification",
		Source:      "alerting-service",
		Subject:     groupKey,
		Time:        time.Now(),
		DataSchema:  "https://kingdom.unheaded.io/schemas/alert/v1",
		ContentType: "application/json",
		Data: &BusboyAlertData{
			GroupKey:     groupKey,
			Status:       status,
			Receiver:     b.config.Name,
			GroupLabels:  groupLabels,
			CommonLabels: commonLabels,
			Alerts:       busboyAlerts,
		},
		Headers: b.config.Headers,
	}
}

// publishWithRetry publishes an event with retry logic.
func (b *BusboyChannel) publishWithRetry(ctx context.Context, event *BusboyEvent) error {
	var lastErr error

	for attempt := 0; attempt <= b.config.RetryCount; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(b.config.RetryDelay):
			}
		}

		err := b.publisher.Publish(ctx, b.config.Topic, event)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("%w: %v", alerting.ErrNotificationError, lastErr)
}

// flushLoop runs the background flush loop.
func (b *BusboyChannel) flushLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.config.FlushInterval)
	defer ticker.Stop()

	batch := make([]*BusboyEvent, 0, b.config.BatchSize)

	for {
		select {
		case event, ok := <-b.queue:
			if !ok {
				// Channel closed, flush remaining
				if len(batch) > 0 {
					_ = b.flushBatch(context.Background(), batch)
				}
				return
			}

			batch = append(batch, event)
			if len(batch) >= b.config.BatchSize {
				_ = b.flushBatch(context.Background(), batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				_ = b.flushBatch(context.Background(), batch)
				batch = batch[:0]
			}
		}
	}
}

// flushBatch publishes a batch of events.
func (b *BusboyChannel) flushBatch(ctx context.Context, events []*BusboyEvent) error {
	if len(events) == 0 {
		return nil
	}

	return b.publisher.PublishBatch(ctx, b.config.Topic, events)
}

// Test tests the Busboy connection.
func (b *BusboyChannel) Test(ctx context.Context) error {
	testAlert := &alerting.Alert{
		ID:          "test-alert",
		Name:        "Test Alert",
		Description: "Test notification from Unheaded Kingdom alerting system",
		State:       alerting.AlertStateFiring,
		Severity:    alerting.SeverityInfo,
		Labels:      map[string]string{"alertname": "TestAlert", "test": "true"},
		Annotations: map[string]string{"summary": "Test notification"},
		StartsAt:    time.Now(),
		Fingerprint: "test-fingerprint",
	}

	return b.Send(ctx, []*alerting.Alert{testAlert})
}

// Close closes the channel and flushes remaining events.
func (b *BusboyChannel) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	close(b.queue)
	b.mu.Unlock()

	b.wg.Wait()
	return b.publisher.Close()
}

// MockBusboyPublisher is a mock publisher for testing.
type MockBusboyPublisher struct {
	mu       sync.Mutex
	Events   []*BusboyEvent
	Batches  [][]*BusboyEvent
	Error    error
}

// NewMockBusboyPublisher creates a new mock publisher.
func NewMockBusboyPublisher() *MockBusboyPublisher {
	return &MockBusboyPublisher{
		Events:  make([]*BusboyEvent, 0),
		Batches: make([][]*BusboyEvent, 0),
	}
}

// Publish records the event.
func (m *MockBusboyPublisher) Publish(ctx context.Context, topic string, event *BusboyEvent) error {
	if m.Error != nil {
		return m.Error
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Events = append(m.Events, event)
	return nil
}

// PublishBatch records the batch.
func (m *MockBusboyPublisher) PublishBatch(ctx context.Context, topic string, events []*BusboyEvent) error {
	if m.Error != nil {
		return m.Error
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Batches = append(m.Batches, events)
	for _, e := range events {
		m.Events = append(m.Events, e)
	}
	return nil
}

// Close does nothing for the mock.
func (m *MockBusboyPublisher) Close() error {
	return nil
}

// EventCount returns the number of recorded events (thread-safe).
func (m *MockBusboyPublisher) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Events)
}

// BatchCount returns the number of recorded batches (thread-safe).
func (m *MockBusboyPublisher) BatchCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Batches)
}

// GetEvents returns a copy of recorded events as JSON for inspection.
func (m *MockBusboyPublisher) GetEventsJSON() ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return json.Marshal(m.Events)
}
