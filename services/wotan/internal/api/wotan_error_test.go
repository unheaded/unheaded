// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/wotanerr"
)

// S-TRUE-WIRE Step 144: the three refusals a client most needs to tell
// apart carry the wotan-memory-03 structured code in X-Wotan-Error.
func TestHTTPErrorsCarryWotanErrorCode(t *testing.T) {
	srv := setupTestServerWithTopics()
	pending, _ := srv.MemberManager.RequestJoin("alerts", "pending-svc", "", 5*time.Minute)
	pubBody := func(id string) []byte {
		b, _ := json.Marshal(map[string]string{"subscriber_id": id, "payload": "x"})
		return b
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   []byte
		status int
		want   wotanerr.Code
	}{
		{"unauthorized publish", http.MethodPost, "/api/v1/topics/alerts/publish", pubBody(pending.ID.String()), http.StatusForbidden, wotanerr.TopicAccessDenied},
		{"unknown topic endpoint", http.MethodGet, "/api/v1/topics/alerts/bogus", nil, http.StatusNotFound, wotanerr.UnknownTopicEndpoint},
		{"signature required", http.MethodPost, "/api/v1/topics/config.kingdom/publish", pubBody(pending.ID.String()), http.StatusForbidden, wotanerr.SignatureRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.TopicRouter(rec, httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body)))
			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			got, ok := wotanerr.FromHeader(rec.Header())
			if !ok || *got != tc.want {
				t.Fatalf("%s = %q (%v), want %s", wotanerr.HeaderName, rec.Header().Get(wotanerr.HeaderName), got, tc.want.Hex())
			}
		})
	}

	// A success carries no code.
	id := subscribeID(t, srv, "alerts")
	rec := httptest.NewRecorder()
	srv.TopicRouter(rec, httptest.NewRequest(http.MethodPost, "/api/v1/topics/alerts/publish", bytes.NewReader(pubBody(id))))
	if rec.Code != http.StatusCreated || rec.Header().Get(wotanerr.HeaderName) != "" {
		t.Fatalf("publish = %d with %s %q", rec.Code, wotanerr.HeaderName, rec.Header().Get(wotanerr.HeaderName))
	}
}

// The room paths refuse signed topics with the same code as the topic API.
func TestRoomSignedRefusalCarriesCode(t *testing.T) {
	srv := setupTestServerWithTopics()
	mbr, _ := srv.MemberManager.RequestJoin("config.kingdom", "Mallory", "", 5*time.Minute)
	srv.MemberManager.Approve(mbr.ID, "admin")
	body, _ := json.Marshal(SendMessageRequest{MemberID: mbr.ID, RoomID: "config.kingdom", Content: "forged"})
	rec := httptest.NewRecorder()
	srv.SendMessage(rec, httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body)))
	if got, ok := wotanerr.FromHeader(rec.Header()); rec.Code != http.StatusForbidden || !ok || *got != wotanerr.SignatureRequired {
		t.Fatalf("send = %d %v", rec.Code, got)
	}
}
