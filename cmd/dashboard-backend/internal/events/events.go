// Package events provides event streaming integration with Wotan message bus.
// It subscribes to relevant topics and maintains an event buffer for the dashboard.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/logger"
)

var (
	// ErrNilConfig indicates nil configuration
	ErrNilConfig = errors.New("config cannot be nil")
	// ErrNotConnected indicates not connected to Wotan
	ErrNotConnected = errors.New("not connected to wotan")
	// ErrStreamStopped indicates stream has been stopped
	ErrStreamStopped = errors.New("stream has been stopped")
)

// EventType represents the type of event
type EventType string

const (
	// EventTypeMetrics represents metrics-related events
	EventTypeMetrics EventType = "metrics"
	// EventTypeHealth represents health-related events
	EventTypeHealth EventType = "health"
	// EventTypeAlert represents alert events
	EventTypeAlert EventType = "alert"
	// EventTypeTimeline represents timeline events
	EventTypeTimeline EventType = "timeline"
	// EventTypeTask represents task events
	EventTypeTask EventType = "task"
	// EventTypeDecision represents decision events
	EventTypeDecision EventType = "decision"
	// EventTypeSystem represents system events
	EventTypeSystem EventType = "system"
)

// Severity represents event severity
type Severity string

const (
	// SeverityInfo is informational
	SeverityInfo Severity = "info"
	// SeverityWarning is a warning
	SeverityWarning Severity = "warning"
	// SeverityError is an error
	SeverityError Severity = "error"
	// SeverityCritical is critical
	SeverityCritical Severity = "critical"
)

// Config holds event streamer configuration
type Config struct {
	// WotanAddr is the Wotan server address
	WotanAddr string
	// ServiceName is the name of this service
	ServiceName string
	// Topics are the topics to subscribe to
	Topics []string
	// BufferSize is the event buffer size
	BufferSize int
	// RetentionPeriod is how long to keep events
	RetentionPeriod time.Duration
	// ReconnectInterval is the reconnection interval
	ReconnectInterval time.Duration
}

// DefaultConfig returns default event streamer configuration
func DefaultConfig() *Config {
	return &Config{
		WotanAddr:  "localhost:9090",
		ServiceName: "dashboard-backend",
		Topics: []string{
			"metrics.*",
			"health.*",
			"alerts.*",
			"timeline.*",
			"tasks.*",
			"decisions.*",
			"state.*",
		},
		BufferSize:        1000,
		RetentionPeriod:   1 * time.Hour,
		ReconnectInterval: 5 * time.Second,
	}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c == nil {
		return ErrNilConfig
	}
	if c.WotanAddr == "" {
		return errors.New("wotan address cannot be empty")
	}
	if c.ServiceName == "" {
		c.ServiceName = "dashboard-backend"
	}
	if len(c.Topics) == 0 {
		c.Topics = []string{"*"}
	}
	if c.BufferSize == 0 {
		c.BufferSize = 1000
	}
	if c.RetentionPeriod == 0 {
		c.RetentionPeriod = 1 * time.Hour
	}
	if c.ReconnectInterval == 0 {
		c.ReconnectInterval = 5 * time.Second
	}
	return nil
}

// Event represents a dashboard event
type Event struct {
	ID        string                 `json:"id"`
	Type      EventType              `json:"type"`
	Topic     string                 `json:"topic"`
	Severity  Severity               `json:"severity"`
	Source    string                 `json:"source"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Raw       json.RawMessage        `json:"raw,omitempty"`
}

// EventFilter defines criteria for filtering events
type EventFilter struct {
	Types      []EventType `json:"types,omitempty"`
	Topics     []string    `json:"topics,omitempty"`
	Severities []Severity  `json:"severities,omitempty"`
	Sources    []string    `json:"sources,omitempty"`
	Since      time.Time   `json:"since,omitempty"`
	Until      time.Time   `json:"until,omitempty"`
	Limit      int         `json:"limit,omitempty"`
}

// Matches checks if an event matches the filter
func (f *EventFilter) Matches(e *Event) bool {
	// Type filter
	if len(f.Types) > 0 {
		found := false
		for _, t := range f.Types {
			if e.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Severity filter
	if len(f.Severities) > 0 {
		found := false
		for _, s := range f.Severities {
			if e.Severity == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Source filter
	if len(f.Sources) > 0 {
		found := false
		for _, s := range f.Sources {
			if e.Source == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Time range filter
	if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && e.Timestamp.After(f.Until) {
		return false
	}

	return true
}

// EventBuffer stores events with size limit
type EventBuffer struct {
	events  []Event
	maxSize int
	mu      sync.RWMutex
}

// NewEventBuffer creates a new event buffer
func NewEventBuffer(maxSize int) *EventBuffer {
	return &EventBuffer{
		events:  make([]Event, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add adds an event to the buffer
func (b *EventBuffer) Add(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events = append(b.events, event)
	if len(b.events) > b.maxSize {
		b.events = b.events[len(b.events)-b.maxSize:]
	}
}

// GetRecent returns the most recent N events
func (b *EventBuffer) GetRecent(n int) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n > len(b.events) {
		n = len(b.events)
	}
	if n == 0 {
		return nil
	}

	events := make([]Event, n)
	start := len(b.events) - n
	copy(events, b.events[start:])

	// Reverse so newest first
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	return events
}

// GetFiltered returns events matching the filter
func (b *EventBuffer) GetFiltered(filter *EventFilter) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []Event
	for i := len(b.events) - 1; i >= 0; i-- {
		e := b.events[i]
		if filter.Matches(&e) {
			result = append(result, e)
			if filter.Limit > 0 && len(result) >= filter.Limit {
				break
			}
		}
	}

	return result
}

// Cleanup removes events older than cutoff
func (b *EventBuffer) Cleanup(cutoff time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var valid []Event
	for _, e := range b.events {
		if e.Timestamp.After(cutoff) {
			valid = append(valid, e)
		}
	}
	b.events = valid
}

// Count returns the number of events
func (b *EventBuffer) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.events)
}

// Streamer streams events from Wotan to the dashboard
type Streamer struct {
	config  *Config
	log     *logger.Logger
	client  *wotanClient.Client
	buffer  *EventBuffer

	running  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	runMu    sync.RWMutex
	wg       sync.WaitGroup

	// Event listeners
	listeners []func(Event)
	listenersMu sync.RWMutex

	// Connection state
	connected bool
	connMu    sync.RWMutex

	// Stats
	eventsReceived int64
	eventsDropped  int64
	statsMu        sync.RWMutex
}

// NewStreamer creates a new event streamer
func NewStreamer(config *Config, log *logger.Logger) (*Streamer, error) {
	if config == nil {
		config = DefaultConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if log == nil {
		log = logger.New(nil)
	}

	return &Streamer{
		config:  config,
		log:     log,
		buffer:  NewEventBuffer(config.BufferSize),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}, nil
}

// Start starts the event streamer
func (s *Streamer) Start(ctx context.Context) error {
	s.runMu.Lock()
	if s.running {
		s.runMu.Unlock()
		return errors.New("streamer already running")
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.runMu.Unlock()

	s.wg.Add(2)
	go s.connectionLoop(ctx)
	go s.cleanupLoop(ctx)

	s.log.Info().
		Str("wotan", s.config.WotanAddr).
		Strs("topics", s.config.Topics).
		Msg("event streamer started")

	return nil
}

// Stop stops the event streamer
func (s *Streamer) Stop() error {
	s.runMu.Lock()
	if !s.running {
		s.runMu.Unlock()
		return nil
	}
	s.running = false
	close(s.stopCh)
	s.runMu.Unlock()

	s.wg.Wait()

	if s.client != nil {
		s.client.Close()
	}

	close(s.doneCh)

	s.log.Info().Msg("event streamer stopped")
	return nil
}

// connectionLoop manages Wotan connection and streaming
func (s *Streamer) connectionLoop(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
		}

		// Connect to Wotan
		if err := s.connect(ctx); err != nil {
			s.log.Error().Err(err).Msg("failed to connect to wotan")
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case <-time.After(s.config.ReconnectInterval):
				continue
			}
		}

		// Stream messages
		s.streamMessages(ctx)

		// Disconnect and reconnect
		s.disconnect()

		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-time.After(s.config.ReconnectInterval):
			continue
		}
	}
}

// connect establishes connection to Wotan
func (s *Streamer) connect(ctx context.Context) error {
	client, err := wotanClient.NewClient(s.config.WotanAddr)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	s.client = client

	// Subscribe to topics
	for _, topic := range s.config.Topics {
		sub, err := client.Subscribe(ctx, topic, s.config.ServiceName)
		if err != nil {
			s.log.Warn().
				Err(err).
				Str("topic", topic).
				Msg("failed to subscribe to topic")
			continue
		}

		s.log.Info().
			Str("topic", topic).
			Str("subscriber_id", sub.SubscriberID).
			Str("status", sub.Status).
			Msg("subscribed to topic")
	}

	s.connMu.Lock()
	s.connected = true
	s.connMu.Unlock()

	s.log.Info().Msg("connected to wotan")
	return nil
}

// disconnect closes Wotan connection
func (s *Streamer) disconnect() {
	s.connMu.Lock()
	s.connected = false
	s.connMu.Unlock()

	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
}

// streamMessages streams messages from subscribed topics
func (s *Streamer) streamMessages(ctx context.Context) {
	for _, topic := range s.config.Topics {
		s.wg.Add(1)
		go func(t string) {
			defer s.wg.Done()
			s.streamTopic(ctx, t)
		}(topic)
	}
}

// streamTopic streams messages from a single topic
func (s *Streamer) streamTopic(ctx context.Context, topic string) {
	if s.client == nil {
		return
	}

	msgCh, err := s.client.StreamMessages(ctx, topic)
	if err != nil {
		s.log.Warn().
			Err(err).
			Str("topic", topic).
			Msg("failed to stream topic")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			if msg == nil {
				continue
			}

			event := s.parseMessage(msg)
			s.processEvent(event)
		}
	}
}

// parseMessage parses a Wotan message into an Event
func (s *Streamer) parseMessage(msg *wotanClient.Message) Event {
	event := Event{
		ID:        msg.MessageID,
		Topic:     msg.Topic,
		Source:    msg.SenderID,
		Timestamp: msg.CreatedAt,
		Raw:       json.RawMessage(msg.Payload),
	}

	// Determine event type from topic
	event.Type = s.topicToEventType(msg.Topic)
	event.Severity = SeverityInfo

	// Parse payload
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(msg.Payload), &payload); err == nil {
		event.Data = payload

		// Extract title and message
		if title, ok := payload["title"].(string); ok {
			event.Title = title
		}
		if message, ok := payload["message"].(string); ok {
			event.Message = message
		}
		if severity, ok := payload["severity"].(string); ok {
			event.Severity = Severity(severity)
		}
	}

	// Default title if not set
	if event.Title == "" {
		event.Title = fmt.Sprintf("%s event from %s", event.Type, event.Source)
	}

	return event
}

// topicToEventType maps a topic to an event type
func (s *Streamer) topicToEventType(topic string) EventType {
	switch {
	case len(topic) >= 7 && topic[:7] == "metrics":
		return EventTypeMetrics
	case len(topic) >= 6 && topic[:6] == "health":
		return EventTypeHealth
	case len(topic) >= 6 && topic[:6] == "alerts":
		return EventTypeAlert
	case len(topic) >= 8 && topic[:8] == "timeline":
		return EventTypeTimeline
	case len(topic) >= 5 && topic[:5] == "tasks":
		return EventTypeTask
	case len(topic) >= 9 && topic[:9] == "decisions":
		return EventTypeDecision
	default:
		return EventTypeSystem
	}
}

// processEvent processes an incoming event
func (s *Streamer) processEvent(event Event) {
	// Add to buffer
	s.buffer.Add(event)

	// Update stats
	s.statsMu.Lock()
	s.eventsReceived++
	s.statsMu.Unlock()

	// Notify general listeners
	s.listenersMu.RLock()
	listeners := make([]func(Event), len(s.listeners))
	copy(listeners, s.listeners)
	s.listenersMu.RUnlock()

	for _, listener := range listeners {
		go listener(event)
	}

	// Notify topic-specific callbacks
	s.notifyTopicCallbacks(event)
}

// cleanupLoop runs periodic cleanup of old events
func (s *Streamer) cleanupLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.RetentionPeriod / 10)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-s.config.RetentionPeriod)
			s.buffer.Cleanup(cutoff)
			s.log.Debug().Time("cutoff", cutoff).Msg("event cleanup complete")
		}
	}
}

// AddListener adds an event listener
func (s *Streamer) AddListener(fn func(Event)) {
	s.listenersMu.Lock()
	defer s.listenersMu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// GetEvents returns events matching the filter
func (s *Streamer) GetEvents(filter *EventFilter) []Event {
	if filter == nil {
		return s.buffer.GetRecent(100)
	}
	return s.buffer.GetFiltered(filter)
}

// GetRecentEvents returns the most recent N events
func (s *Streamer) GetRecentEvents(n int) []Event {
	return s.buffer.GetRecent(n)
}

// GetEventsByType returns events of a specific type
func (s *Streamer) GetEventsByType(eventType EventType, limit int) []Event {
	filter := &EventFilter{
		Types: []EventType{eventType},
		Limit: limit,
	}
	return s.buffer.GetFiltered(filter)
}

// GetStats returns streamer statistics
func (s *Streamer) GetStats() map[string]interface{} {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()

	s.connMu.RLock()
	connected := s.connected
	s.connMu.RUnlock()

	return map[string]interface{}{
		"connected":       connected,
		"events_received": s.eventsReceived,
		"events_dropped":  s.eventsDropped,
		"buffer_size":     s.buffer.Count(),
		"buffer_max":      s.config.BufferSize,
		"topics":          s.config.Topics,
	}
}

// IsConnected returns whether connected to Wotan
func (s *Streamer) IsConnected() bool {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return s.connected
}

// IsRunning returns whether the streamer is running
func (s *Streamer) IsRunning() bool {
	s.runMu.RLock()
	defer s.runMu.RUnlock()
	return s.running
}

// EventCount returns the number of buffered events
func (s *Streamer) EventCount() int {
	return s.buffer.Count()
}

// PublishEvent publishes an internal event (for local events)
func (s *Streamer) PublishEvent(event Event) {
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Severity == "" {
		event.Severity = SeverityInfo
	}
	if event.Source == "" {
		event.Source = s.config.ServiceName
	}

	s.processEvent(event)
}

// CreateEvent creates a new event
func CreateEvent(eventType EventType, severity Severity, source, title, message string) Event {
	return Event{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Type:      eventType,
		Severity:  severity,
		Source:    source,
		Title:     title,
		Message:   message,
		Timestamp: time.Now(),
	}
}

// EventSummary provides a summary of events by type
type EventSummary struct {
	TotalEvents   int            `json:"total_events"`
	EventsByType  map[string]int `json:"events_by_type"`
	EventsBySev   map[string]int `json:"events_by_severity"`
	RecentEvents  []Event        `json:"recent_events,omitempty"`
	OldestEvent   time.Time      `json:"oldest_event,omitempty"`
	NewestEvent   time.Time      `json:"newest_event,omitempty"`
}

// GetSummary returns an event summary
func (s *Streamer) GetSummary() *EventSummary {
	events := s.buffer.GetRecent(s.config.BufferSize)

	summary := &EventSummary{
		TotalEvents:  len(events),
		EventsByType: make(map[string]int),
		EventsBySev:  make(map[string]int),
	}

	for _, e := range events {
		summary.EventsByType[string(e.Type)]++
		summary.EventsBySev[string(e.Severity)]++
	}

	// Recent events (last 10)
	if len(events) > 10 {
		summary.RecentEvents = events[:10]
	} else {
		summary.RecentEvents = events
	}

	// Time range
	if len(events) > 0 {
		summary.NewestEvent = events[0].Timestamp
		summary.OldestEvent = events[len(events)-1].Timestamp
	}

	return summary
}

// EventTimeline represents events organized by time
type EventTimeline struct {
	Intervals []TimelineInterval `json:"intervals"`
}

// TimelineInterval represents events in a time interval
type TimelineInterval struct {
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Events []Event   `json:"events"`
	Count  int       `json:"count"`
}

// GetTimeline returns events organized by time intervals
func (s *Streamer) GetTimeline(duration time.Duration, intervals int) *EventTimeline {
	events := s.buffer.GetRecent(s.config.BufferSize)

	now := time.Now()
	intervalDuration := duration / time.Duration(intervals)

	timeline := &EventTimeline{
		Intervals: make([]TimelineInterval, intervals),
	}

	for i := 0; i < intervals; i++ {
		end := now.Add(-time.Duration(i) * intervalDuration)
		start := end.Add(-intervalDuration)

		interval := TimelineInterval{
			Start:  start,
			End:    end,
			Events: make([]Event, 0),
		}

		for _, e := range events {
			if e.Timestamp.After(start) && e.Timestamp.Before(end) {
				interval.Events = append(interval.Events, e)
				interval.Count++
			}
		}

		timeline.Intervals[intervals-1-i] = interval
	}

	return timeline
}

// GroupEventsBySource groups events by source
func (s *Streamer) GroupEventsBySource() map[string][]Event {
	events := s.buffer.GetRecent(s.config.BufferSize)

	groups := make(map[string][]Event)
	for _, e := range events {
		groups[e.Source] = append(groups[e.Source], e)
	}

	// Sort events within each group
	for _, evts := range groups {
		sort.Slice(evts, func(i, j int) bool {
			return evts[i].Timestamp.After(evts[j].Timestamp)
		})
	}

	return groups
}

// TopicCallback represents a callback function for a specific topic
type TopicCallback func(topic string, event Event)

// topicCallbacks maps topics to their callbacks
var topicCallbacks = struct {
	sync.RWMutex
	callbacks map[string][]TopicCallback
}{
	callbacks: make(map[string][]TopicCallback),
}

// OnTopic registers a callback for a specific topic pattern
// The pattern can be a specific topic like "metrics.cpu" or a wildcard like "metrics.*"
func (s *Streamer) OnTopic(topicPattern string, callback TopicCallback) {
	topicCallbacks.Lock()
	defer topicCallbacks.Unlock()
	topicCallbacks.callbacks[topicPattern] = append(topicCallbacks.callbacks[topicPattern], callback)
}

// notifyTopicCallbacks notifies registered topic-specific callbacks
func (s *Streamer) notifyTopicCallbacks(event Event) {
	topicCallbacks.RLock()
	defer topicCallbacks.RUnlock()

	for pattern, callbacks := range topicCallbacks.callbacks {
		if s.matchTopicPattern(pattern, event.Topic) {
			for _, cb := range callbacks {
				go cb(event.Topic, event)
			}
		}
	}
}

// matchTopicPattern checks if a topic matches a pattern (supports * wildcard)
func (s *Streamer) matchTopicPattern(pattern, topic string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == topic {
		return true
	}
	// Support wildcards like "metrics.*"
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(topic) >= len(prefix) && topic[:len(prefix)] == prefix
	}
	return false
}

// PublishToWotan publishes an event to the Wotan message bus
// Returns error if not connected or publish fails
func (s *Streamer) PublishToWotan(ctx context.Context, topic string, event Event) error {
	s.connMu.RLock()
	connected := s.connected
	client := s.client
	s.connMu.RUnlock()

	if !connected || client == nil {
		return ErrNotConnected
	}

	// Prepare event payload
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Source == "" {
		event.Source = s.config.ServiceName
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	if err := client.Publish(ctx, topic, payload); err != nil {
		s.log.Warn().
			Err(err).
			Str("topic", topic).
			Msg("failed to publish event to wotan")
		return fmt.Errorf("publish to wotan: %w", err)
	}

	s.log.Debug().
		Str("topic", topic).
		Str("event_id", event.ID).
		Msg("published event to wotan")

	return nil
}

// PublishMetricEvent publishes a metrics event to Wotan
func (s *Streamer) PublishMetricEvent(ctx context.Context, serviceName string, metricName string, value float64, labels map[string]interface{}) error {
	event := Event{
		Type:     EventTypeMetrics,
		Severity: SeverityInfo,
		Source:   serviceName,
		Title:    fmt.Sprintf("Metric: %s", metricName),
		Data: map[string]interface{}{
			"metric_name": metricName,
			"value":       value,
			"labels":      labels,
		},
	}

	topic := fmt.Sprintf("metrics.%s", serviceName)
	return s.PublishToWotan(ctx, topic, event)
}

// PublishHealthEvent publishes a health event to Wotan
func (s *Streamer) PublishHealthEvent(ctx context.Context, serviceName string, status string, message string) error {
	severity := SeverityInfo
	switch status {
	case "unhealthy", "error":
		severity = SeverityError
	case "degraded", "warning":
		severity = SeverityWarning
	case "critical":
		severity = SeverityCritical
	}

	event := Event{
		Type:     EventTypeHealth,
		Severity: severity,
		Source:   serviceName,
		Title:    fmt.Sprintf("Health: %s is %s", serviceName, status),
		Message:  message,
		Data: map[string]interface{}{
			"status":  status,
			"service": serviceName,
		},
	}

	topic := fmt.Sprintf("health.%s", serviceName)
	return s.PublishToWotan(ctx, topic, event)
}

// PublishAlertEvent publishes an alert event to Wotan
func (s *Streamer) PublishAlertEvent(ctx context.Context, severity Severity, title, message string, data map[string]interface{}) error {
	event := Event{
		Type:     EventTypeAlert,
		Severity: severity,
		Source:   s.config.ServiceName,
		Title:    title,
		Message:  message,
		Data:     data,
	}

	topic := "alerts." + string(severity)
	return s.PublishToWotan(ctx, topic, event)
}

// GetWotanClient returns the underlying Wotan client (if connected)
// This allows direct access for advanced operations
func (s *Streamer) GetWotanClient() *wotanClient.Client {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return s.client
}

// SubscribeToTopic subscribes to an additional topic after startup
func (s *Streamer) SubscribeToTopic(ctx context.Context, topic string) error {
	s.connMu.RLock()
	connected := s.connected
	client := s.client
	s.connMu.RUnlock()

	if !connected || client == nil {
		return ErrNotConnected
	}

	sub, err := client.Subscribe(ctx, topic, s.config.ServiceName)
	if err != nil {
		return fmt.Errorf("subscribe to topic %s: %w", topic, err)
	}

	s.log.Info().
		Str("topic", topic).
		Str("subscriber_id", sub.SubscriberID).
		Str("status", sub.Status).
		Msg("subscribed to additional topic")

	// Add to topics list
	s.config.Topics = append(s.config.Topics, topic)

	// Start streaming the new topic
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.streamTopic(ctx, topic)
	}()

	return nil
}
