// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// L15: a subscriber still waiting for approval has made no room.
func TestSubscribe_PendingMakesNoRoom(t *testing.T) {
	w := newAuthedHTTPWotan(t)
	if code, st, _ := w.subscribe(t, w.f.Bearer("kev-poller", "wotan"), "any.name.at.all", "x"); code != http.StatusCreated || st != "pending" {
		t.Fatalf("kev-poller = %d %q, want 201 pending", code, st)
	}
	if got := w.s.RoomManager.Count(); got != 0 {
		t.Fatalf("%d rooms after a pending subscribe, want 0", got)
	}
	if code, st, _ := w.subscribe(t, w.f.Bearer("timeguru", "wotan"), "alerts.critical", "x"); code != http.StatusCreated || st != "approved" {
		t.Fatalf("timeguru = %d %q, want 201 approved", code, st)
	}
	if _, err := w.s.RoomManager.Get("alerts.critical"); err != nil {
		t.Fatalf("an approved subscriber's room: %v", err)
	}
}

// L15: nor has a pending legacy join. Approval makes the room, so the member
// finds it there.
func TestJoin_RoomComesWithApproval(t *testing.T) {
	srv := setupTestServer()
	body, _ := json.Marshal(JoinRoomRequest{RoomID: "chat-room", Name: "alice"})
	rec := httptest.NewRecorder()
	srv.JoinRoom(rec, httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("join = %d", rec.Code)
	}
	if got := srv.RoomManager.Count(); got != 0 {
		t.Fatalf("%d rooms after a pending join, want 0", got)
	}
	var joined JoinRoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &joined); err != nil {
		t.Fatal(err)
	}

	approve, _ := json.Marshal(map[string]string{"member_id": joined.MemberID.String(), "approved_by": "admin"})
	req := httptest.NewRequest(http.MethodPost, "/admin/approve", bytes.NewReader(approve))
	setAdminAuth(req)
	rec = httptest.NewRecorder()
	srv.ApproveMember(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d", rec.Code)
	}
	send, _ := json.Marshal(SendMessageRequest{MemberID: joined.MemberID, RoomID: "chat-room", Content: "hi"})
	rec = httptest.NewRecorder()
	srv.SendMessage(rec, httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(send)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("send after approval = %d, want 201", rec.Code)
	}
}

func TestApproveAll_MakesTheRooms(t *testing.T) {
	srv := setupTestServer()
	if _, err := srv.MemberManager.RequestJoin("room-a", "alice", "", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.MemberManager.RequestJoin("topics.*", "bob", "", time.Minute); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/approve-all", nil)
	setAdminAuth(req)
	rec := httptest.NewRecorder()
	srv.ApproveAllMembers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("approve-all = %d", rec.Code)
	}
	if _, err := srv.RoomManager.Get("room-a"); err != nil {
		t.Fatalf("room-a: %v", err)
	}
	if _, err := srv.RoomManager.Get("topics.*"); err == nil {
		t.Fatal("approval made a room for a pattern")
	}
}

// L15: one rule for a name, on every path that can make a room.
func TestRoomNames_OneRule(t *testing.T) {
	srv := setupTestServer()
	for _, name := range []string{"has space", "semi;colon", "slash/ed", strings.Repeat("a", 256), "a.*"} {
		body, _ := json.Marshal(JoinRoomRequest{RoomID: name, Name: "alice"})
		rec := httptest.NewRecorder()
		srv.JoinRoom(rec, httptest.NewRequest(http.MethodPost, "/join", bytes.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("join %.20q = %d, want 400", name, rec.Code)
		}
	}
	long := strings.Repeat("a", 256)
	if got := extractTopic("/api/v1/topics/"+long+"/publish", "/api/v1/topics/", "/publish"); got != "" {
		t.Errorf("a %d-byte topic passed", len(got))
	}
	if got := extractTopic("/api/v1/topics/alerts.#/subscribe", "/api/v1/topics/", "/subscribe"); got != "alerts.#" {
		t.Errorf("a pattern = %q", got)
	}
}
