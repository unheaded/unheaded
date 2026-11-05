// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// L6: the gateway binds loopback unless told otherwise, and the compose file
// that does tell it publishes on the host's loopback only.
func TestListenBind(t *testing.T) {
	t.Setenv("SOPHIA_GATEWAY_BIND", "")
	sg := NewSophiaGateway()
	if got := sg.listenAddr("20105"); got != "127.0.0.1:20105" {
		t.Fatalf("default = %q", got)
	}
	sg.gatewayBind = "0.0.0.0"
	if got := sg.listenAddr("9090"); got != "0.0.0.0:9090" {
		t.Fatalf("explicit = %q", got)
	}

	// The same instance: its metrics register once.
	big := `{"model":"` + strings.Repeat("a", maxChatBodyBytes+1) + `"}`
	w := httptest.NewRecorder()
	sg.handleChat(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(big)))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("chat body over the limit: %d, want 413", w.Code)
	}

	raw, err := os.ReadFile("../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(raw)
	for _, want := range []string{`SOPHIA_GATEWAY_BIND: "0.0.0.0"`, `"127.0.0.1:20106:9090"`, `"127.0.0.1:20107:20105"`} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %s", want)
		}
	}
}
