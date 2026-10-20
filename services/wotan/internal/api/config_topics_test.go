// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// config.* takes only signed messages over gRPC (ADR-043 #2). The HTTP API
// has no signature fields, so both of its write paths must refuse it; the
// topic publish path used to store config.* messages unchecked.
func TestHTTPRefusesConfigTopics(t *testing.T) {
	srv := setupTestServerWithTopics()
	id := subscribeID(t, srv, "config.kingdom")
	w := topicReq(srv, http.MethodPost, "/api/v1/topics/config.kingdom/publish",
		fmt.Sprintf(`{"subscriber_id":%q,"payload":"{\"evil\":true}"}`, id))
	if w.Code != http.StatusForbidden {
		t.Fatalf("HTTP publish to config.kingdom = %d, want 403: %s", w.Code, w.Body.String())
	}
	if rm, err := srv.RoomManager.Get("config.kingdom"); err == nil && len(rm.GetMessages()) != 0 {
		t.Fatal("a config.* message was stored")
	}

	// The room-message path reaches the same rooms by name.
	srv.RoomManager.Create("config.apply", "config.apply")
	mbr := srv.MemberManager.RequestJoin("config.apply", "Mallory", "", 5*time.Minute)
	srv.MemberManager.Approve(mbr.ID, "admin")
	body, _ := json.Marshal(SendMessageRequest{MemberID: mbr.ID, RoomID: "config.apply", Content: "evil"})
	rec := httptest.NewRecorder()
	srv.SendMessage(rec, httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(body)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("room message to config.apply = %d, want 403", rec.Code)
	}
	rm, _ := srv.RoomManager.Get("config.apply")
	if len(rm.GetMessages()) != 0 {
		t.Fatal("a config.* room message was stored")
	}

	// Nor can a signed message be deleted over HTTP: the triple check uses
	// id, creator and content, which any reader of the room has.
	cfg, _ := srv.RoomManager.GetOrCreate("config.kingdom", "config.kingdom")
	signedMsg, _ := cfg.SendMessage(mbr.ID, "signed config")
	del, _ := json.Marshal(DeleteMessageRequest{MemberID: mbr.ID, RoomID: "config.kingdom", MessageID: signedMsg.ID, Content: "signed config"})
	rec = httptest.NewRecorder()
	srv.DeleteMessage(rec, httptest.NewRequest(http.MethodPost, "/delete", bytes.NewReader(del)))
	if rec.Code != http.StatusForbidden || len(cfg.GetMessages()) != 1 {
		t.Fatalf("delete of a config.* message = %d, %d left", rec.Code, len(cfg.GetMessages()))
	}

	// Other topics are unaffected.
	other := subscribeID(t, srv, "alerts.disk")
	publish(t, srv, "alerts.disk", other, "ok")
}

func TestTrustedSigningKeys(t *testing.T) {
	var c TopicConfig
	c.Topics.SigningKeys = []string{base64.StdEncoding.EncodeToString([]byte("key-one")), " " + base64.StdEncoding.EncodeToString([]byte("key-two")) + " "}
	keys, err := c.TrustedSigningKeys()
	if err != nil || len(keys) != 2 || string(keys[1]) != "key-two" {
		t.Fatalf("keys %q err %v", keys, err)
	}
	c.Topics.SigningKeys = append(c.Topics.SigningKeys, "not base64!")
	if _, err := c.TrustedSigningKeys(); err == nil {
		t.Fatal("a malformed key was skipped instead of refused")
	}
	var none *TopicConfig
	if keys, err := none.TrustedSigningKeys(); err != nil || len(keys) != 0 {
		t.Fatal("nil config must mean no keys")
	}
}
