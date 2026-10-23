// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotan-client/mock"
)

// TestStreamTaskEventsHaveBoardListeners: every event type the stream sends
// for a task change must be one the board's app.js listens for. The page
// listened for "tasks.created" etc. (the Wotan topic names) while the stream
// sends "task.created", so no viewer saw another's change until a reload.
func TestStreamTaskEventsHaveBoardListeners(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	tm, err := NewTaskManager(mock.NewMockClient(mock.WithAutoApprove()), func(eventType string, _ interface{}) {
		mu.Lock()
		sent = append(sent, eventType)
		mu.Unlock()
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(createTestTask("ui-events"))
	for _, topic := range []string{TopicTasksCreated, TopicTasksUpdated, TopicTasksDeleted} {
		if err := tm.handleMessage(&wotanClient.Message{MessageID: topic, Topic: topic, Payload: string(payload)}); err != nil {
			t.Fatalf("%s: %v", topic, err)
		}
	}

	src, err := staticFiles.ReadFile("static/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 3 {
		t.Fatalf("broadcasts = %v, want 3", sent)
	}
	for _, et := range sent {
		if !strings.Contains(string(src), "WebSocketClient.on('"+et+"'") {
			t.Errorf("stream sends %q but static/js/app.js has no listener for it", et)
		}
	}
}
