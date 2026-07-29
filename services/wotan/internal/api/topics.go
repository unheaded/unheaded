// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"unheaded/pkg/httputil"
	"unheaded/services/wotan/internal/ringbuffer"
	"unheaded/services/wotan/internal/store"
)

// Ensure store import is used
var _ = store.ErrNotFound

// TopicSubscriber represents a subscriber to a topic (maps to wotan-client.Subscriber)
type TopicSubscriber struct {
	SubscriberID string    `json:"subscriber_id"`
	Topic        string    `json:"topic"`
	DisplayName  string    `json:"display_name"`
	Status       string    `json:"status"` // approved (auto-approved for internal services)
	RequestedAt  time.Time `json:"requested_at"`
}

// TopicMessage represents a message in a topic (maps to wotan-client.Message)
type TopicMessage struct {
	MessageID string    `json:"message_id"`
	Topic     string    `json:"topic"`
	SenderID  string    `json:"sender_id"`
	CreatedAt time.Time `json:"created_at"`
	Seq       int64     `json:"seq"`
	Payload   string    `json:"payload"`
	Deleted   bool      `json:"deleted"`
}

// topicSeqCounter provides monotonic sequence numbers per topic
type topicSeqCounter struct {
	mu       sync.RWMutex
	counters map[string]*atomic.Int64
}

func newTopicSeqCounter() *topicSeqCounter {
	return &topicSeqCounter{
		counters: make(map[string]*atomic.Int64),
	}
}

func (t *topicSeqCounter) next(topic string) int64 {
	t.mu.RLock()
	counter, ok := t.counters[topic]
	t.mu.RUnlock()

	if ok {
		return counter.Add(1)
	}

	t.mu.Lock()
	// Double-check after acquiring write lock
	if counter, ok = t.counters[topic]; ok {
		t.mu.Unlock()
		return counter.Add(1)
	}
	counter = &atomic.Int64{}
	t.counters[topic] = counter
	t.mu.Unlock()

	return counter.Add(1)
}

// topicSubscribers tracks subscribers per topic
type topicSubscribers struct {
	mu          sync.RWMutex
	subscribers map[string]map[string]*TopicSubscriber // topic -> subscriberID -> subscriber
}

func newTopicSubscribers() *topicSubscribers {
	return &topicSubscribers{
		subscribers: make(map[string]map[string]*TopicSubscriber),
	}
}

func (ts *topicSubscribers) add(sub *TopicSubscriber) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if _, ok := ts.subscribers[sub.Topic]; !ok {
		ts.subscribers[sub.Topic] = make(map[string]*TopicSubscriber)
	}
	ts.subscribers[sub.Topic][sub.SubscriberID] = sub
}

func (ts *topicSubscribers) get(topic, subscriberID string) (*TopicSubscriber, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	topicSubs, ok := ts.subscribers[topic]
	if !ok {
		return nil, false
	}
	sub, ok := topicSubs[subscriberID]
	return sub, ok
}

func (ts *topicSubscribers) isSubscribed(topic, subscriberID string) bool {
	_, ok := ts.get(topic, subscriberID)
	return ok
}

// InitTopics initializes topic support on the Server.
// Must be called after NewServer and before serving requests.
func (s *Server) InitTopics() {
	s.topicSeqs = newTopicSeqCounter()
	s.topicSubs = newTopicSubscribers()
}

// SubscribeTopic handles POST /api/v1/topics/{topic}/subscribe
//
// Subscribers whose display_name appears in the auto-approve allowlist
// (configs/wotan.yaml → topics.auto_approve) are approved immediately.
// All other subscribers are created with status "pending" and must be
// approved via the admin API (POST /api/v1/admin/approve).
//
// Security note: display_name is self-reported. When mTLS is enabled,
// use verified client certificate CN for identity (see pkg/auth/mtls.go).
func (s *Server) SubscribeTopic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract topic from URL: /api/v1/topics/{topic}/subscribe
	topic := extractTopic(r.URL.Path, "/api/v1/topics/", "/subscribe")
	if topic == "" {
		writeError(w, http.StatusBadRequest, "topic is required")
		return
	}

	// Parse request body
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, httputil.DefaultMaxBodySize)).Decode(&req); err != nil {
		// Allow empty body — display_name is optional for internal services
		req.DisplayName = "service"
	}
	if req.DisplayName == "" {
		req.DisplayName = "service"
	}

	// Create or get the room backing this topic
	s.RoomManager.GetOrCreate(topic, topic)

	// Create a pending member
	newMember := s.MemberManager.RequestJoin(topic, req.DisplayName, "", s.PendingApprovalTimeout)

	// Auto-approve only if display_name is in the allowlist
	status := "pending"
	if s.TopicConfig != nil && s.TopicConfig.IsAutoApproved(req.DisplayName) {
		if err := s.MemberManager.Approve(newMember.ID, "topic-auto-approve"); err != nil {
			log.Error().Err(err).Str("topic", topic).Msg("failed to auto-approve topic subscriber")
			writeError(w, http.StatusInternalServerError, "subscription failed")
			return
		}
		status = "approved"
	} else {
		log.Info().
			Str("topic", topic).
			Str("display_name", req.DisplayName).
			Msg("topic_subscriber_pending_approval")
	}

	sub := &TopicSubscriber{
		SubscriberID: newMember.ID.String(),
		Topic:        topic,
		DisplayName:  req.DisplayName,
		Status:       status,
		RequestedAt:  time.Now(),
	}

	// Track the subscriber
	if s.topicSubs != nil {
		s.topicSubs.add(sub)
	}

	log.Info().
		Str("topic", topic).
		Str("subscriber_id", sub.SubscriberID).
		Str("display_name", req.DisplayName).
		Str("status", status).
		Msg("topic_subscriber_created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"subscriber": sub,
	})
}

// PublishTopic handles POST /api/v1/topics/{topic}/publish
func (s *Server) PublishTopic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	topic := extractTopic(r.URL.Path, "/api/v1/topics/", "/publish")
	if topic == "" {
		writeError(w, http.StatusBadRequest, "topic is required")
		return
	}

	var req struct {
		SubscriberID string `json:"subscriber_id"`
		Payload      string `json:"payload"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, httputil.DefaultMaxBodySize)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Payload == "" {
		writeError(w, http.StatusBadRequest, "payload is required")
		return
	}

	// Validate subscriber ID
	subscriberUUID, err := uuid.Parse(req.SubscriberID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid subscriber_id")
		return
	}

	// Verify subscriber is approved
	if !s.MemberManager.IsApproved(subscriberUUID) {
		writeError(w, http.StatusForbidden, "subscriber not approved")
		return
	}

	// Get or create the room for this topic
	rm := s.RoomManager.GetOrCreate(topic, topic)

	// Send message to room's ring buffer
	msg, _ := rm.SendMessage(subscriberUUID, req.Payload)

	// Persist to store if available (async — non-blocking)
	if s.Store != nil {
		go func() {
			storeMsg := &store.Message{
				ID:        msg.ID,
				CreatorID: msg.CreatorID,
				RoomID:    msg.RoomID,
				Content:   msg.Content,
				Timestamp: msg.Timestamp,
			}
			if _, _, err := s.Store.Push(r.Context(), storeMsg); err != nil {
				log.Warn().Err(err).Str("topic", topic).Msg("failed to persist message to store")
			}
		}()
	}

	// Publish event through wotan pub/sub
	if s.Wotan != nil {
		s.Wotan.PublishMessageCreated(msg)
	}

	// Assign sequence number
	seq := int64(0)
	if s.topicSeqs != nil {
		seq = s.topicSeqs.next(topic)
	}

	log.Debug().
		Str("topic", topic).
		Str("message_id", msg.ID.String()).
		Int64("seq", seq).
		Msg("topic_message_published")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"message_id": msg.ID.String(),
		"topic":      topic,
		"seq":        seq,
		"timestamp":  msg.Timestamp,
	})
}

// GetTopicMessages handles GET /api/v1/topics/{topic}/messages
func (s *Server) GetTopicMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	topic := extractTopic(r.URL.Path, "/api/v1/topics/", "/messages")
	if topic == "" {
		writeError(w, http.StatusBadRequest, "topic is required")
		return
	}

	// Get the room for this topic
	rm, err := s.RoomManager.Get(topic)
	if err != nil {
		// Topic doesn't exist yet — return empty messages
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
			"messages": []TopicMessage{},
		})
		return
	}

	// Get messages from ring buffer
	msgs := rm.GetMessages()

	// Convert to topic message format
	topicMsgs := make([]TopicMessage, 0, len(msgs))
	for i, msg := range msgs {
		topicMsgs = append(topicMsgs, ringbufferToTopicMessage(msg, topic, int64(i+1)))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"messages": topicMsgs,
	})
}

// ListTopics handles GET /api/v1/topics
func (s *Server) ListTopics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rooms := s.RoomManager.List()
	topics := make([]map[string]interface{}, 0, len(rooms))
	for _, rm := range rooms {
		topics = append(topics, map[string]interface{}{
			"topic":         rm.ID,
			"created_at":    rm.CreatedAt,
			"message_count": rm.Buffer.Count(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"topics": topics,
		"count":  len(topics),
	})
}

// extractTopic parses the topic name from a URL path.
// path:   /api/v1/topics/alerts.critical/subscribe
// prefix: /api/v1/topics/
// suffix: /subscribe
// result: alerts.critical
func extractTopic(path, prefix, suffix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if suffix != "" {
		if !strings.HasSuffix(rest, suffix) {
			return ""
		}
		rest = rest[:len(rest)-len(suffix)]
	}
	// Validate topic name: allow alphanumeric, dots, hyphens, underscores
	for _, c := range rest {
		if !isTopicChar(c) {
			return ""
		}
	}
	return rest
}

func isTopicChar(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '.' || c == '-' || c == '_' ||
		c == '*' || c == '#' // wildcard pattern chars for subscriptions
}

func ringbufferToTopicMessage(msg *ringbuffer.Message, topic string, seq int64) TopicMessage {
	return TopicMessage{
		MessageID: msg.ID.String(),
		Topic:     topic,
		SenderID:  msg.CreatorID.String(),
		CreatedAt: msg.Timestamp,
		Seq:       seq,
		Payload:   msg.Content,
		Deleted:   false,
	}
}

// topicRouter dispatches /api/v1/topics/* requests to the correct handler.
// Go 1.22+ ServeMux supports patterns, but for compat we do manual dispatch.
func (s *Server) TopicRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// GET /api/v1/topics — list all topics
	if path == "/api/v1/topics" || path == "/api/v1/topics/" {
		s.ListTopics(w, r)
		return
	}

	// Route based on suffix
	switch {
	case strings.HasSuffix(path, "/subscribe"):
		s.SubscribeTopic(w, r)
	case strings.HasSuffix(path, "/publish"):
		s.PublishTopic(w, r)
	case strings.HasSuffix(path, "/messages"):
		s.GetTopicMessages(w, r)
	default:
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown topic endpoint: %s", path))
	}
}
