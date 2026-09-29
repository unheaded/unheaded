// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"unheaded/services/wotan/internal/ringbuffer"
)

func topicReq(srv *Server, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	srv.TopicRouter(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func subscribeID(t *testing.T, srv *Server, topic string) string {
	t.Helper()
	w := topicReq(srv, http.MethodPost, "/api/v1/topics/"+topic+"/subscribe", `{"display_name":"test"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("subscribe %s: %d %s", topic, w.Code, w.Body.String())
	}
	var r struct {
		Subscriber TopicSubscriber `json:"subscriber"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	return r.Subscriber.SubscriberID
}

func publish(t *testing.T, srv *Server, topic, id, payload string) {
	t.Helper()
	w := topicReq(srv, http.MethodPost, "/api/v1/topics/"+topic+"/publish",
		fmt.Sprintf(`{"subscriber_id":%q,"payload":%q}`, id, payload))
	if w.Code != http.StatusCreated {
		t.Fatalf("publish %s: %d %s", topic, w.Code, w.Body.String())
	}
}

type msgsPage struct {
	Messages []TopicMessage `json:"messages"`
	LastSeq  int64          `json:"last_seq"`
}

func getMessages(t *testing.T, srv *Server, topic, query string) msgsPage {
	t.Helper()
	w := topicReq(srv, http.MethodGet, "/api/v1/topics/"+topic+"/messages"+query, "")
	if w.Code != http.StatusOK {
		t.Fatalf("messages %s%s: %d %s", topic, query, w.Code, w.Body.String())
	}
	var p msgsPage
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func payloads(p msgsPage) string {
	var out []string
	for _, m := range p.Messages {
		out = append(out, fmt.Sprintf("%s:%s@%d", m.Topic, m.Payload, m.Seq))
	}
	return strings.Join(out, " ")
}

// A pattern reads every matching topic, oldest first by (global) seq, with
// each message labelled by the topic it was published to. It used to look
// up a topic literally named "alerts.*" and return nothing.
func TestGetTopicMessages_Wildcard(t *testing.T) {
	srv := setupTestServerWithTopics()
	id := subscribeID(t, srv, "alerts.critical")
	publish(t, srv, "alerts.critical", id, "a1") // seq 1
	publish(t, srv, "health.wotan", id, "h1")    // seq 2
	publish(t, srv, "alerts.warn", id, "w1")     // seq 3
	publish(t, srv, "alerts.critical", id, "a2") // seq 4
	publish(t, srv, "alerts.deep.x", id, "d1")   // seq 5

	for _, tc := range []struct{ pattern, query, want string }{
		{"alerts.*", "", "alerts.critical:a1@1 alerts.warn:w1@3 alerts.critical:a2@4"},
		{"alerts.#", "", "alerts.critical:a1@1 alerts.warn:w1@3 alerts.critical:a2@4 alerts.deep.x:d1@5"},
		{"*", "", "alerts.critical:a1@1 health.wotan:h1@2 alerts.warn:w1@3 alerts.critical:a2@4 alerts.deep.x:d1@5"},
		{"alerts.*", "?after_seq=1", "alerts.warn:w1@3 alerts.critical:a2@4"},
		{"alerts.*", "?after_seq=1&limit=1", "alerts.warn:w1@3"},
		{"alerts.*", "?after_seq=4", ""},
		{"nothing.*", "", ""},
	} {
		p := getMessages(t, srv, tc.pattern, tc.query)
		if got := payloads(p); got != tc.want {
			t.Errorf("%s%s = %q, want %q", tc.pattern, tc.query, got, tc.want)
		}
		// last_seq is Wotan's newest seq: a cursor above it means Wotan restarted.
		if p.LastSeq != 5 {
			t.Errorf("%s%s last_seq = %d, want 5", tc.pattern, tc.query, p.LastSeq)
		}
	}
}

// Subscribing to a pattern must not create a topic named after it, and
// publishing to a pattern is refused (it names a set of topics).
func TestTopicPatterns_SubscribeAndPublish(t *testing.T) {
	srv := setupTestServerWithTopics()
	id := subscribeID(t, srv, "alerts.*")
	if _, err := srv.RoomManager.Get("alerts.*"); err == nil {
		t.Error("subscribing to alerts.* created a topic named alerts.*")
	}
	w := topicReq(srv, http.MethodPost, "/api/v1/topics/alerts.*/publish", fmt.Sprintf(`{"subscriber_id":%q,"payload":"x"}`, id))
	if w.Code != http.StatusBadRequest {
		t.Errorf("publish to a pattern: %d, want 400", w.Code)
	}
	if _, err := srv.RoomManager.Get("alerts.*"); err == nil {
		t.Error("publishing to alerts.* created a topic named alerts.*")
	}
	// The pattern subscriber can publish to a real topic it matches.
	publish(t, srv, "alerts.critical", id, "ok")
	if p := getMessages(t, srv, "alerts.*", ""); payloads(p) != "alerts.critical:ok@1" {
		t.Errorf("pattern read after publish = %q", payloads(p))
	}
}

// A seq above the counter value read before the scan (a write landing in a
// ring while the pattern read is under way) is left for the next poll.
func TestGetTopicMessages_WildcardUpperBound(t *testing.T) {
	srv := setupTestServerWithTopics()
	id := subscribeID(t, srv, "alerts.a")
	publish(t, srv, "alerts.a", id, "seen") // seq 1; manager LastSeq = 1
	rm := srv.RoomManager.Create("alerts.b", "alerts.b")
	var ahead atomic.Int64
	ahead.Store(1)
	rm.Buffer = ringbuffer.NewShared(10, &ahead) // numbers ahead of the manager
	rm.SendMessage(uuid.New(), "in-flight")      // seq 2, but LastSeq still 1
	if got := payloads(getMessages(t, srv, "alerts.*", "")); got != "alerts.a:seen@1" {
		t.Errorf("pattern read = %q, want only seq 1", got)
	}
}
