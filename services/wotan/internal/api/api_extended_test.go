// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/services/wotan/internal/member"
	"unheaded/services/wotan/internal/room"
	"unheaded/services/wotan/internal/wotan"
)

// setupTestServerWithTopics creates a server with topic support initialized.
func setupTestServerWithTopics() *Server {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	msgWotan := wotan.NewWotan()
	srv := NewServer(roomMgr, memberMgr, msgWotan, 5*time.Minute)
	srv.AdminAPIKey = testAdminAPIKey
	srv.InitTopics()
	return srv
}

// ============================================================
// InitTopics Tests
// ============================================================

func TestInitTopics(t *testing.T) {
	srv := setupTestServer()
	if srv.topicSeqs != nil {
		t.Error("topicSeqs should be nil before InitTopics")
	}
	if srv.topicSubs != nil {
		t.Error("topicSubs should be nil before InitTopics")
	}

	srv.InitTopics()

	if srv.topicSeqs == nil {
		t.Error("topicSeqs should be set after InitTopics")
	}
	if srv.topicSubs == nil {
		t.Error("topicSubs should be set after InitTopics")
	}
}

// ============================================================
// topicSeqCounter Tests
// ============================================================

func TestTopicSeqCounter_Next(t *testing.T) {
	counter := newTopicSeqCounter()

	// First call for a topic should return 1
	seq := counter.next("alerts")
	if seq != 1 {
		t.Errorf("first seq = %d, want 1", seq)
	}

	// Second call should return 2
	seq = counter.next("alerts")
	if seq != 2 {
		t.Errorf("second seq = %d, want 2", seq)
	}

	// Different topic starts at 1
	seq = counter.next("events")
	if seq != 1 {
		t.Errorf("first seq for different topic = %d, want 1", seq)
	}

	// Original topic continues from 2
	seq = counter.next("alerts")
	if seq != 3 {
		t.Errorf("third seq for alerts = %d, want 3", seq)
	}
}

func TestTopicSeqCounter_ConcurrentAccess(t *testing.T) {
	counter := newTopicSeqCounter()
	done := make(chan struct{})

	// Run 10 goroutines, each incrementing 100 times
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				counter.next("concurrent-topic")
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Final value should be 1001 (1000 increments + 1)
	final := counter.next("concurrent-topic")
	if final != 1001 {
		t.Errorf("final seq = %d, want 1001", final)
	}
}

// ============================================================
// topicSubscribers Tests
// ============================================================

func TestTopicSubscribers_AddAndGet(t *testing.T) {
	subs := newTopicSubscribers()

	sub := &TopicSubscriber{
		SubscriberID: "sub-1",
		Topic:        "alerts",
		DisplayName:  "test-service",
		Status:       "approved",
		RequestedAt:  time.Now(),
	}

	subs.add(sub)

	got, ok := subs.get("alerts", "sub-1")
	if !ok {
		t.Fatal("expected subscriber to be found")
	}
	if got.SubscriberID != "sub-1" {
		t.Errorf("SubscriberID = %s, want sub-1", got.SubscriberID)
	}
	if got.DisplayName != "test-service" {
		t.Errorf("DisplayName = %s, want test-service", got.DisplayName)
	}
}

func TestTopicSubscribers_GetNotFound(t *testing.T) {
	subs := newTopicSubscribers()

	// Non-existent topic
	_, ok := subs.get("nonexistent", "sub-1")
	if ok {
		t.Error("expected not found for non-existent topic")
	}

	// Existing topic but non-existent subscriber
	subs.add(&TopicSubscriber{
		SubscriberID: "sub-1",
		Topic:        "alerts",
	})

	_, ok = subs.get("alerts", "sub-999")
	if ok {
		t.Error("expected not found for non-existent subscriber")
	}
}

func TestTopicSubscribers_IsSubscribed(t *testing.T) {
	subs := newTopicSubscribers()

	subs.add(&TopicSubscriber{
		SubscriberID: "sub-1",
		Topic:        "alerts",
	})

	if !subs.isSubscribed("alerts", "sub-1") {
		t.Error("expected isSubscribed to return true")
	}
	if subs.isSubscribed("alerts", "sub-999") {
		t.Error("expected isSubscribed to return false for unknown subscriber")
	}
	if subs.isSubscribed("nonexistent", "sub-1") {
		t.Error("expected isSubscribed to return false for unknown topic")
	}
}

// ============================================================
// extractTopic Tests
// ============================================================

func TestExtractTopic(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		prefix string
		suffix string
		want   string
	}{
		{
			name:   "subscribe path",
			path:   "/api/v1/topics/alerts/subscribe",
			prefix: "/api/v1/topics/",
			suffix: "/subscribe",
			want:   "alerts",
		},
		{
			name:   "publish path",
			path:   "/api/v1/topics/events.critical/publish",
			prefix: "/api/v1/topics/",
			suffix: "/publish",
			want:   "events.critical",
		},
		{
			name:   "messages path",
			path:   "/api/v1/topics/my-topic/messages",
			prefix: "/api/v1/topics/",
			suffix: "/messages",
			want:   "my-topic",
		},
		{
			name:   "with underscores",
			path:   "/api/v1/topics/my_topic/subscribe",
			prefix: "/api/v1/topics/",
			suffix: "/subscribe",
			want:   "my_topic",
		},
		{
			name:   "with wildcard chars",
			path:   "/api/v1/topics/alerts.*/subscribe",
			prefix: "/api/v1/topics/",
			suffix: "/subscribe",
			want:   "alerts.*",
		},
		{
			name:   "with hash wildcard",
			path:   "/api/v1/topics/alerts.#/subscribe",
			prefix: "/api/v1/topics/",
			suffix: "/subscribe",
			want:   "alerts.#",
		},
		{
			name:   "wrong prefix",
			path:   "/api/v2/topics/alerts/subscribe",
			prefix: "/api/v1/topics/",
			suffix: "/subscribe",
			want:   "",
		},
		{
			name:   "wrong suffix",
			path:   "/api/v1/topics/alerts/unknown",
			prefix: "/api/v1/topics/",
			suffix: "/subscribe",
			want:   "",
		},
		{
			name:   "empty topic",
			path:   "/api/v1/topics//subscribe",
			prefix: "/api/v1/topics/",
			suffix: "/subscribe",
			want:   "",
		},
		{
			name:   "invalid chars in topic",
			path:   "/api/v1/topics/alert space/subscribe",
			prefix: "/api/v1/topics/",
			suffix: "/subscribe",
			want:   "",
		},
		{
			name:   "empty suffix",
			path:   "/api/v1/topics/alerts",
			prefix: "/api/v1/topics/",
			suffix: "",
			want:   "alerts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTopic(tt.path, tt.prefix, tt.suffix)
			if got != tt.want {
				t.Errorf("extractTopic(%q, %q, %q) = %q, want %q", tt.path, tt.prefix, tt.suffix, got, tt.want)
			}
		})
	}
}

// ============================================================
// isTopicChar Tests
// ============================================================

func TestIsTopicChar(t *testing.T) {
	valid := []rune{'a', 'z', 'A', 'Z', '0', '9', '.', '-', '_', '*', '#'}
	for _, c := range valid {
		if !isTopicChar(c) {
			t.Errorf("isTopicChar(%q) = false, want true", c)
		}
	}

	invalid := []rune{' ', '!', '@', '/', '\\', '(', ')', '{', '}', '+', '='}
	for _, c := range invalid {
		if isTopicChar(c) {
			t.Errorf("isTopicChar(%q) = true, want false", c)
		}
	}
}

// ============================================================
// ringbufferToTopicMessage Tests
// ============================================================

func TestRingbufferToTopicMessage(t *testing.T) {
	srv := setupTestServerWithTopics()

	rm := srv.RoomManager.Create("test-topic", "Test Topic")
	mbr := srv.MemberManager.RequestJoin("test-topic", "publisher", "", 5*time.Minute)
	srv.MemberManager.Approve(mbr.ID, "admin")

	msg, _ := rm.SendMessage(mbr.ID, "test payload")

	topicMsg := ringbufferToTopicMessage(msg, "test-topic", 42)

	if topicMsg.MessageID != msg.ID.String() {
		t.Errorf("MessageID = %s, want %s", topicMsg.MessageID, msg.ID.String())
	}
	if topicMsg.Topic != "test-topic" {
		t.Errorf("Topic = %s, want test-topic", topicMsg.Topic)
	}
	if topicMsg.SenderID != mbr.ID.String() {
		t.Errorf("SenderID = %s, want %s", topicMsg.SenderID, mbr.ID.String())
	}
	if topicMsg.Seq != 42 {
		t.Errorf("Seq = %d, want 42", topicMsg.Seq)
	}
	if topicMsg.Payload != "test payload" {
		t.Errorf("Payload = %s, want 'test payload'", topicMsg.Payload)
	}
	if topicMsg.Deleted {
		t.Error("Deleted = true, want false")
	}
	if topicMsg.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

// ============================================================
// SubscribeTopic Tests
// ============================================================

func TestSubscribeTopic_Success(t *testing.T) {
	srv := setupTestServerWithTopics()

	body, _ := json.Marshal(map[string]string{"display_name": "my-service"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/subscribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.SubscribeTopic(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	sub, ok := resp["subscriber"].(map[string]interface{})
	if !ok {
		t.Fatal("response missing subscriber field")
	}
	if sub["topic"] != "alerts" {
		t.Errorf("topic = %v, want alerts", sub["topic"])
	}
	if sub["display_name"] != "my-service" {
		t.Errorf("display_name = %v, want my-service", sub["display_name"])
	}
	if sub["status"] != "approved" {
		t.Errorf("status = %v, want approved", sub["status"])
	}
}

func TestSubscribeTopic_DefaultDisplayName(t *testing.T) {
	srv := setupTestServerWithTopics()

	// Empty body should default display_name to "service"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/events/subscribe", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.SubscribeTopic(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	sub := resp["subscriber"].(map[string]interface{})
	if sub["display_name"] != "service" {
		t.Errorf("display_name = %v, want 'service'", sub["display_name"])
	}
}

func TestSubscribeTopic_InvalidBody(t *testing.T) {
	srv := setupTestServerWithTopics()

	// Invalid JSON body should still succeed with default display_name
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/events/subscribe", bytes.NewReader([]byte("not-json")))
	rec := httptest.NewRecorder()

	srv.SubscribeTopic(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d (invalid body should default to 'service')", rec.Code, http.StatusCreated)
	}
}

func TestSubscribeTopic_MethodNotAllowed(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/alerts/subscribe", nil)
	rec := httptest.NewRecorder()

	srv.SubscribeTopic(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestSubscribeTopic_EmptyTopic(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics//subscribe", nil)
	rec := httptest.NewRecorder()

	srv.SubscribeTopic(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSubscribeTopic_NilTopicSubs(t *testing.T) {
	// Server without InitTopics called
	srv := setupTestServer()

	body, _ := json.Marshal(map[string]string{"display_name": "svc"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/subscribe", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.SubscribeTopic(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d (should work without topic subs initialized)", rec.Code, http.StatusCreated)
	}
}

// ============================================================
// PublishTopic Tests
// ============================================================

func TestPublishTopic_Success(t *testing.T) {
	srv := setupTestServerWithTopics()

	// First subscribe to get a subscriber ID
	subBody, _ := json.Marshal(map[string]string{"display_name": "publisher"})
	subReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/subscribe", bytes.NewReader(subBody))
	subReq.Header.Set("Content-Type", "application/json")
	subRec := httptest.NewRecorder()
	srv.SubscribeTopic(subRec, subReq)

	var subResp map[string]interface{}
	json.NewDecoder(subRec.Body).Decode(&subResp)
	sub := subResp["subscriber"].(map[string]interface{})
	subscriberID := sub["subscriber_id"].(string)

	// Now publish
	pubBody, _ := json.Marshal(map[string]string{
		"subscriber_id": subscriberID,
		"payload":       "alert: CPU high",
	})
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/publish", bytes.NewReader(pubBody))
	pubReq.Header.Set("Content-Type", "application/json")
	pubRec := httptest.NewRecorder()

	srv.PublishTopic(pubRec, pubReq)

	if pubRec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", pubRec.Code, http.StatusCreated)
	}

	var pubResp map[string]interface{}
	json.NewDecoder(pubRec.Body).Decode(&pubResp)

	if pubResp["topic"] != "alerts" {
		t.Errorf("topic = %v, want alerts", pubResp["topic"])
	}
	if pubResp["message_id"] == nil || pubResp["message_id"] == "" {
		t.Error("message_id is missing or empty")
	}
	if seq, ok := pubResp["seq"].(float64); !ok || seq != 1 {
		t.Errorf("seq = %v, want 1", pubResp["seq"])
	}
}

func TestPublishTopic_MethodNotAllowed(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/alerts/publish", nil)
	rec := httptest.NewRecorder()

	srv.PublishTopic(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestPublishTopic_EmptyTopic(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics//publish", nil)
	rec := httptest.NewRecorder()

	srv.PublishTopic(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPublishTopic_InvalidJSON(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/publish", bytes.NewReader([]byte("bad json")))
	rec := httptest.NewRecorder()

	srv.PublishTopic(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPublishTopic_EmptyPayload(t *testing.T) {
	srv := setupTestServerWithTopics()

	body, _ := json.Marshal(map[string]string{
		"subscriber_id": "some-id",
		"payload":       "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/publish", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	srv.PublishTopic(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPublishTopic_InvalidSubscriberID(t *testing.T) {
	srv := setupTestServerWithTopics()

	body, _ := json.Marshal(map[string]string{
		"subscriber_id": "not-a-uuid",
		"payload":       "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/publish", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	srv.PublishTopic(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPublishTopic_SubscriberNotApproved(t *testing.T) {
	srv := setupTestServerWithTopics()

	// Create a member that is pending (not approved)
	mbr := srv.MemberManager.RequestJoin("alerts", "pending-svc", "", 5*time.Minute)

	body, _ := json.Marshal(map[string]string{
		"subscriber_id": mbr.ID.String(),
		"payload":       "test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/publish", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	srv.PublishTopic(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestPublishTopic_NilWotan(t *testing.T) {
	roomMgr := room.NewManager(100)
	memberMgr := member.NewManager()
	srv := NewServer(roomMgr, memberMgr, nil, 5*time.Minute)
	srv.AdminAPIKey = testAdminAPIKey
	srv.InitTopics()

	// Subscribe
	subBody, _ := json.Marshal(map[string]string{"display_name": "svc"})
	subReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/subscribe", bytes.NewReader(subBody))
	subRec := httptest.NewRecorder()
	srv.SubscribeTopic(subRec, subReq)

	var subResp map[string]interface{}
	json.NewDecoder(subRec.Body).Decode(&subResp)
	sub := subResp["subscriber"].(map[string]interface{})
	subscriberID := sub["subscriber_id"].(string)

	// Publish with nil Wotan should still work
	pubBody, _ := json.Marshal(map[string]string{
		"subscriber_id": subscriberID,
		"payload":       "test payload",
	})
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/publish", bytes.NewReader(pubBody))
	pubRec := httptest.NewRecorder()

	srv.PublishTopic(pubRec, pubReq)

	if pubRec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d with nil wotan", pubRec.Code, http.StatusCreated)
	}
}

func TestPublishTopic_NilTopicSeqs(t *testing.T) {
	// Server without InitTopics: topicSeqs is nil
	srv := setupTestServer()

	// Subscribe (creates the member and auto-approves)
	subBody, _ := json.Marshal(map[string]string{"display_name": "svc"})
	subReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/subscribe", bytes.NewReader(subBody))
	subRec := httptest.NewRecorder()
	srv.SubscribeTopic(subRec, subReq)

	var subResp map[string]interface{}
	json.NewDecoder(subRec.Body).Decode(&subResp)
	sub := subResp["subscriber"].(map[string]interface{})
	subscriberID := sub["subscriber_id"].(string)

	// Publish with nil topicSeqs should return seq=0
	pubBody, _ := json.Marshal(map[string]string{
		"subscriber_id": subscriberID,
		"payload":       "test",
	})
	pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/publish", bytes.NewReader(pubBody))
	pubRec := httptest.NewRecorder()

	srv.PublishTopic(pubRec, pubReq)

	if pubRec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", pubRec.Code, http.StatusCreated)
	}

	var resp map[string]interface{}
	json.NewDecoder(pubRec.Body).Decode(&resp)
	if seq, ok := resp["seq"].(float64); !ok || seq != 0 {
		t.Errorf("seq = %v, want 0 when topicSeqs is nil", resp["seq"])
	}
}

// ============================================================
// GetTopicMessages Tests
// ============================================================

func TestGetTopicMessages_Success(t *testing.T) {
	srv := setupTestServerWithTopics()

	// Subscribe and publish messages
	subBody, _ := json.Marshal(map[string]string{"display_name": "publisher"})
	subReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/events/subscribe", bytes.NewReader(subBody))
	subRec := httptest.NewRecorder()
	srv.SubscribeTopic(subRec, subReq)

	var subResp map[string]interface{}
	json.NewDecoder(subRec.Body).Decode(&subResp)
	sub := subResp["subscriber"].(map[string]interface{})
	subscriberID := sub["subscriber_id"].(string)

	// Publish two messages
	for _, payload := range []string{"event-1", "event-2"} {
		pubBody, _ := json.Marshal(map[string]string{
			"subscriber_id": subscriberID,
			"payload":       payload,
		})
		pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/events/publish", bytes.NewReader(pubBody))
		pubRec := httptest.NewRecorder()
		srv.PublishTopic(pubRec, pubReq)
	}

	// Get messages
	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/events/messages", nil)
	rec := httptest.NewRecorder()

	srv.GetTopicMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	msgs, ok := resp["messages"].([]interface{})
	if !ok {
		t.Fatal("response missing messages field")
	}
	if len(msgs) != 2 {
		t.Errorf("message count = %d, want 2", len(msgs))
	}
}

func TestGetTopicMessages_EmptyTopic(t *testing.T) {
	srv := setupTestServerWithTopics()

	// Topic that doesn't exist should return empty messages
	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/nonexistent/messages", nil)
	rec := httptest.NewRecorder()

	srv.GetTopicMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	msgs := resp["messages"].([]interface{})
	if len(msgs) != 0 {
		t.Errorf("message count = %d, want 0 for nonexistent topic", len(msgs))
	}
}

func TestGetTopicMessages_MethodNotAllowed(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/events/messages", nil)
	rec := httptest.NewRecorder()

	srv.GetTopicMessages(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestGetTopicMessages_BadTopic(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics//messages", nil)
	rec := httptest.NewRecorder()

	srv.GetTopicMessages(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ============================================================
// ListTopics Tests
// ============================================================

func TestListTopics_Empty(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics", nil)
	rec := httptest.NewRecorder()

	srv.ListTopics(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	count := resp["count"].(float64)
	if count != 0 {
		t.Errorf("count = %v, want 0", count)
	}
}

func TestListTopics_WithTopics(t *testing.T) {
	srv := setupTestServerWithTopics()

	// Create some rooms/topics
	srv.RoomManager.Create("alerts", "Alerts")
	srv.RoomManager.Create("events", "Events")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics", nil)
	rec := httptest.NewRecorder()

	srv.ListTopics(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	count := resp["count"].(float64)
	if count != 2 {
		t.Errorf("count = %v, want 2", count)
	}

	topics := resp["topics"].([]interface{})
	if len(topics) != 2 {
		t.Errorf("topics length = %d, want 2", len(topics))
	}
}

func TestListTopics_MethodNotAllowed(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics", nil)
	rec := httptest.NewRecorder()

	srv.ListTopics(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// ============================================================
// TopicRouter Tests
// ============================================================

func TestTopicRouter_ListTopics(t *testing.T) {
	srv := setupTestServerWithTopics()

	tests := []string{"/api/v1/topics", "/api/v1/topics/"}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()

			srv.TopicRouter(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("Status = %d, want %d for path %s", rec.Code, http.StatusOK, path)
			}
		})
	}
}

func TestTopicRouter_Subscribe(t *testing.T) {
	srv := setupTestServerWithTopics()

	body, _ := json.Marshal(map[string]string{"display_name": "svc"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/subscribe", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	srv.TopicRouter(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestTopicRouter_Publish(t *testing.T) {
	srv := setupTestServerWithTopics()

	// Subscribe first
	subBody, _ := json.Marshal(map[string]string{"display_name": "svc"})
	subReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/subscribe", bytes.NewReader(subBody))
	subRec := httptest.NewRecorder()
	srv.TopicRouter(subRec, subReq)

	var subResp map[string]interface{}
	json.NewDecoder(subRec.Body).Decode(&subResp)
	sub := subResp["subscriber"].(map[string]interface{})
	subscriberID := sub["subscriber_id"].(string)

	// Publish via router
	pubBody, _ := json.Marshal(map[string]string{
		"subscriber_id": subscriberID,
		"payload":       "routed message",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/publish", bytes.NewReader(pubBody))
	rec := httptest.NewRecorder()

	srv.TopicRouter(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestTopicRouter_Messages(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/alerts/messages", nil)
	rec := httptest.NewRecorder()

	srv.TopicRouter(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestTopicRouter_UnknownEndpoint(t *testing.T) {
	srv := setupTestServerWithTopics()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics/alerts/unknown", nil)
	rec := httptest.NewRecorder()

	srv.TopicRouter(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ============================================================
// Admin auth via ApproveMember/DenyMember error paths
// ============================================================

func TestApproveMember_AdminAuthMissing(t *testing.T) {
	srv := setupTestServer()

	body, _ := json.Marshal(map[string]string{"member_id": "some-id"})
	req := httptest.NewRequest(http.MethodPost, "/admin/approve", bytes.NewReader(body))
	// No auth header
	rec := httptest.NewRecorder()

	srv.ApproveMember(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestDenyMember_AdminAuthMissing(t *testing.T) {
	srv := setupTestServer()

	body, _ := json.Marshal(map[string]string{"member_id": "some-id"})
	req := httptest.NewRequest(http.MethodPost, "/admin/deny", bytes.NewReader(body))
	// No auth header
	rec := httptest.NewRecorder()

	srv.DenyMember(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestApproveAllMembers_AdminAuthMissing(t *testing.T) {
	srv := setupTestServer()

	req := httptest.NewRequest(http.MethodPost, "/admin/approve-all", nil)
	// No auth header
	rec := httptest.NewRecorder()

	srv.ApproveAllMembers(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// ============================================================
// Full Topic Integration Flow
// ============================================================

func TestIntegration_TopicPubSubFlow(t *testing.T) {
	srv := setupTestServerWithTopics()

	// 1. List topics (empty)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/topics", nil)
	rec := httptest.NewRecorder()
	srv.TopicRouter(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListTopics failed: %d", rec.Code)
	}

	// 2. Subscribe to a topic
	subBody, _ := json.Marshal(map[string]string{"display_name": "integration-svc"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/topics/integration-test/subscribe", bytes.NewReader(subBody))
	rec = httptest.NewRecorder()
	srv.TopicRouter(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("Subscribe failed: %d", rec.Code)
	}

	var subResp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&subResp)
	sub := subResp["subscriber"].(map[string]interface{})
	subscriberID := sub["subscriber_id"].(string)

	// 3. Publish messages
	for i := 0; i < 3; i++ {
		pubBody, _ := json.Marshal(map[string]string{
			"subscriber_id": subscriberID,
			"payload":       "integration message",
		})
		req = httptest.NewRequest(http.MethodPost, "/api/v1/topics/integration-test/publish", bytes.NewReader(pubBody))
		rec = httptest.NewRecorder()
		srv.TopicRouter(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("Publish %d failed: %d", i, rec.Code)
		}
	}

	// 4. Get messages
	req = httptest.NewRequest(http.MethodGet, "/api/v1/topics/integration-test/messages", nil)
	rec = httptest.NewRecorder()
	srv.TopicRouter(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetMessages failed: %d", rec.Code)
	}

	var msgResp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&msgResp)
	msgs := msgResp["messages"].([]interface{})
	if len(msgs) != 3 {
		t.Errorf("message count = %d, want 3", len(msgs))
	}

	// 5. Verify topics appear in list
	req = httptest.NewRequest(http.MethodGet, "/api/v1/topics", nil)
	rec = httptest.NewRecorder()
	srv.TopicRouter(rec, req)

	var listResp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&listResp)
	if listResp["count"].(float64) < 1 {
		t.Error("expected at least 1 topic after publishing")
	}
}

// ============================================================
// Sequence number monotonicity
// ============================================================

func TestPublishTopic_SequenceNumbers(t *testing.T) {
	srv := setupTestServerWithTopics()

	// Subscribe
	subBody, _ := json.Marshal(map[string]string{"display_name": "seq-test"})
	subReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/seq-topic/subscribe", bytes.NewReader(subBody))
	subRec := httptest.NewRecorder()
	srv.SubscribeTopic(subRec, subReq)

	var subResp map[string]interface{}
	json.NewDecoder(subRec.Body).Decode(&subResp)
	sub := subResp["subscriber"].(map[string]interface{})
	subscriberID := sub["subscriber_id"].(string)

	// Publish 5 messages and verify monotonic sequence
	var lastSeq float64
	for i := 0; i < 5; i++ {
		pubBody, _ := json.Marshal(map[string]string{
			"subscriber_id": subscriberID,
			"payload":       "seq test",
		})
		pubReq := httptest.NewRequest(http.MethodPost, "/api/v1/topics/seq-topic/publish", bytes.NewReader(pubBody))
		pubRec := httptest.NewRecorder()
		srv.PublishTopic(pubRec, pubReq)

		var pubResp map[string]interface{}
		json.NewDecoder(pubRec.Body).Decode(&pubResp)

		seq := pubResp["seq"].(float64)
		if seq <= lastSeq {
			t.Errorf("seq %v is not greater than previous %v", seq, lastSeq)
		}
		lastSeq = seq
	}

	if lastSeq != 5 {
		t.Errorf("final seq = %v, want 5", lastSeq)
	}
}
