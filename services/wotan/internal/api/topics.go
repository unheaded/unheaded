// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"unheaded/pkg/auth"
	"unheaded/pkg/httputil"
	"unheaded/pkg/wotanerr"
	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/metrics"
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

// SubscribeTopic handles POST /api/v1/topics/{topic}/subscribe
//
// Subscribers whose display_name appears in the auto-approve allowlist
// (configs/wotan.yaml → topics.auto_approve) are approved immediately.
// All other subscribers are created with status "pending" and must be
// approved via the admin API (POST /api/v1/admin/approve).
//
// With auth on (ADR-103 §6) the verified service-token subject decides
// approval and names the member; display_name is a label only. With auth
// off the self-reported display_name decides.
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
		if _, err := s.RoomManager.GetOrCreate(topic, topic); err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
	}

	// An allowlisted name gets the one approved member for (topic, name),
	// made on first use. A new member per call, never reaped, left one
	// behind for good each time a client restarted and subscribed again.
	// Any other name gets a pending member per request, which expires.
	identity := auth.IdentityFromContext(r.Context())
	memberName := req.DisplayName
	if identity != nil {
		memberName = identity.Subject
	}
	var newMember *member.Member
	var err error
	status := "pending"
	if s.TopicConfig != nil && s.TopicConfig.IsAutoApproved(identity, req.DisplayName) {
		newMember, err = s.MemberManager.EnsureApproved(topic, memberName, "topic-auto-approve")
		status = "approved"
	} else {
		newMember, err = s.MemberManager.RequestJoin(topic, memberName, "", s.PendingApprovalTimeout)
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if status == "pending" {
		log.Info().
			Str("topic", topic).
			Str("member", memberName).
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
	if s.TopicConfig.RequiresSignature(topic) {
		writeCodedError(w, http.StatusForbidden, wotanerr.SignatureRequired, "signed topics (config.*, drift.*) are published over gRPC with an ML-DSA-65 signature")
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
		writeCodedError(w, http.StatusForbidden, wotanerr.TopicAccessDenied, "subscriber not approved")
		return
	}
	// A subscriber_id is no secret (it is every message's sender_id): with
	// auth on, a caller publishes only through its own members.
	if !s.memberIsCaller(r, subscriberUUID) {
		writeCodedError(w, http.StatusForbidden, wotanerr.TopicAccessDenied, "subscriber belongs to another identity")
		return
	}

	// Get or create the room for this topic
	rm, err := s.RoomManager.GetOrCreate(topic, topic)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Send message to room's ring buffer
	msg, overwritten := rm.SendMessage(subscriberUUID, req.Payload)
	// Counted here as on the gRPC path: this one is where most topics are
	// fed (logs.*, system.health.*), and it was never counted.
	if m := metrics.Get(); m != nil {
		m.RecordMessageCreated(topic, overwritten)
	}

	// Persist to store if available (async — non-blocking)
	if s.Store != nil {
		// Not r.Context(): the server cancels it when this handler returns,
		// which is before the write lands, so every ctx-honouring store
		// failed with "context canceled" after the client got its 201.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), persistTimeout)
		go func() {
			defer cancel()
			storeMsg := &store.Message{
				ID:        msg.ID,
				CreatorID: msg.CreatorID,
				RoomID:    msg.RoomID,
				Content:   msg.Content,
				Timestamp: msg.Timestamp,
			}
			if _, _, err := s.Store.Push(ctx, storeMsg); err != nil {
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

// memberIsCaller reports whether the member belongs to the caller: true
// with no verified identity (auth off), else the member's name must be the
// identity's subject (SubscribeTopic names members by it).
func (s *Server) memberIsCaller(r *http.Request, memberID uuid.UUID) bool {
	identity := auth.IdentityFromContext(r.Context())
	if identity == nil {
		return true
	}
	m, err := s.MemberManager.Get(memberID)
	return err == nil && m.Name == identity.Subject
}

// persistTimeout bounds one background store write for a published message.
const persistTimeout = 10 * time.Second

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

// TopicRouter dispatches /api/v1/topics/* requests to the correct handler.
// Go 1.22+ ServeMux supports patterns, but for compat we do manual dispatch.
func (s *Server) TopicRouter(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if s.RequireIdentity && auth.IdentityFromContext(r.Context()) == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

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
		writeCodedError(w, http.StatusNotFound, wotanerr.UnknownTopicEndpoint, fmt.Sprintf("unknown topic endpoint: %s", path))
	}
}
