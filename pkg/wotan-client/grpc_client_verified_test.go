// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package wotanClient

import (
	"encoding/json"
	"testing"

	chatpb "unheaded/services/wotan/proto"
)

// A subscriber reads the verified sender the same way over either transport.
func TestMessage_CarriesVerifiedSenderOnBothTransports(t *testing.T) {
	if m := topicEventToMessage(&chatpb.TopicEvent{VerifiedSender: "akira"}); m.VerifiedSender != "akira" {
		t.Errorf("gRPC: VerifiedSender = %q", m.VerifiedSender)
	}
	var m Message
	if err := json.Unmarshal([]byte(`{"verified_sender":"akira"}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.VerifiedSender != "akira" {
		t.Errorf("HTTP: VerifiedSender = %q", m.VerifiedSender)
	}
}
