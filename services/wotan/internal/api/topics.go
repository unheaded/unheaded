// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"unheaded/pkg/httputil"
	"unheaded/services/wotan/internal/ringbuffer"
	"unheaded/services/wotan/internal/store"
	"unheaded/services/wotan/internal/topicpattern"
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

	// Create or get the room backing this topic. A pattern ("alerts.*")
	// names a set of topics, not one: creating a room for it made a topic
	// literally called "alerts.*".
	if !topicpattern.IsPattern(topic) {
		s.RoomManager.GetOrCreate(topic, topic)
	}

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
	if topicpattern.IsPattern(topic) {
		writeError(w, http.StatusBadRequest, "cannot publish to a topic pattern")
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

	// The seq the ring buffer assigned: the same number readers page by.
	seq := msg.Seq

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

// maxTopicMessagesPage caps one GetTopicMessages response.
const maxTopicMessagesPage = 1000

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

	// after_seq and limit page the topic. Both used to be ignored: every
	// poll returned the whole retained buffer (10,000 messages) numbered by
	// buffer position, so a poller re-received all of it every time.
	q := r.URL.Query()
	afterSeq := int64(0)
	if v := q.Get("after_seq"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "after_seq must be a non-negative integer")
			return
		}
		afterSeq = n
	}
	limit := maxTopicMessagesPage
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, maxTopicMessagesPage)
	}

	if topicpattern.IsPattern(topic) {
		s.getPatternMessages(w, topic, afterSeq, limit)
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
			"last_seq": 0,
		})
		return
	}

	lastSeq := rm.Buffer.LastSeq()
	msgs := rm.Buffer.GetAfter(afterSeq, limit)

	topicMsgs := make([]TopicMessage, 0, len(msgs))
	for _, msg := range msgs {
		topicMsgs = append(topicMsgs, ringbufferToTopicMessage(msg, topic))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"messages": topicMsgs,
		// last_seq lets a poller whose cursor is ahead of it see that Wotan
		// restarted (seqs are in memory and start again at 1) and rewind.
		"last_seq": lastSeq,
	})
}

// getPatternMessages serves GET /api/v1/topics/{pattern}/messages: messages
// from every topic matching the pattern with seq > afterSeq, oldest first,
// each labelled with its own topic. Seqs are global across topics, so one
// cursor pages them all.
//
// Only seqs up to the counter value read before scanning are returned. A
// seq is assigned and stored under its ring's lock, so everything at or
// below that value is already in place, while a later one may still be
// landing in a ring scanned earlier; returning it would move the caller's
// cursor past that message for good.
func (s *Server) getPatternMessages(w http.ResponseWriter, pattern string, afterSeq int64, limit int) {
	upper := s.RoomManager.LastSeq()

	var msgs []TopicMessage
	for _, rm := range s.RoomManager.List() {
		if !topicpattern.Match(pattern, rm.ID) {
			continue
		}
		// Each ring returns its oldest `limit` after the cursor, in seq order,
		// so the merged first `limit` are the globally oldest.
		for _, msg := range rm.Buffer.GetAfter(afterSeq, limit) {
			if msg.Seq > upper {
				break
			}
			msgs = append(msgs, ringbufferToTopicMessage(msg, rm.ID))
		}
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].Seq < msgs[j].Seq })
	if len(msgs) > limit {
		msgs = msgs[:limit]
	}
	if msgs == nil {
		msgs = []TopicMessage{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{ // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
		"messages": msgs,
		// Wotan's newest seq: a cursor above it is from before a restart.
		"last_seq": upper,
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

func ringbufferToTopicMessage(msg *ringbuffer.Message, topic string) TopicMessage {
	return TopicMessage{
		MessageID: msg.ID.String(),
		Topic:     topic,
		SenderID:  msg.CreatorID.String(),
		CreatedAt: msg.Timestamp,
		Seq:       msg.Seq,
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
