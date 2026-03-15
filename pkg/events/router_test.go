// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package events

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewRouter(t *testing.T) {
	r := NewRouter(nil)
	if r == nil {
		t.Fatal("expected non-nil router")
	}
	if r.closed {
		t.Error("router should not be closed")
	}
	if r.handlers == nil {
		t.Error("handlers map should be initialized")
	}
}

func TestRouter_Register(t *testing.T) {
	r := NewRouter(nil)

	// Valid registration
	err := r.Register("tasks.created", func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Empty topic
	err = r.Register("", func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})
	if err != ErrEmptyTopic {
		t.Errorf("expected ErrEmptyTopic, got %v", err)
	}

	// Nil handler
	err = r.Register("tasks.created", nil)
	if err != ErrNilHandler {
		t.Errorf("expected ErrNilHandler, got %v", err)
	}

	// Multiple handlers for same topic
	err = r.Register("tasks.created", func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})
	if err != nil {
		t.Errorf("should allow multiple handlers: %v", err)
	}
}

func TestRouter_Route(t *testing.T) {
	r := NewRouter(&RouterConfig{DefaultTimeout: 5 * time.Second})

	var handlerCalled int32

	err := r.Register("tasks.created", func(ctx context.Context, topic string, payload []byte) error {
		atomic.AddInt32(&handlerCalled, 1)
		if topic != "tasks.created" {
			t.Errorf("topic = %q, want %q", topic, "tasks.created")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	payload := []byte(`{"task_id": "task-123"}`)
	err = r.Route(context.Background(), "tasks.created", payload)
	if err != nil {
		t.Errorf("route failed: %v", err)
	}

	if atomic.LoadInt32(&handlerCalled) != 1 {
		t.Errorf("handler called %d times, want 1", handlerCalled)
	}
}

func TestRouter_Route_Wildcard(t *testing.T) {
	r := NewRouter(nil)

	var handlerCalled int32

	// Register wildcard handler
	err := r.Register("tasks.*", func(ctx context.Context, topic string, payload []byte) error {
		atomic.AddInt32(&handlerCalled, 1)
		return nil
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Should match
	topics := []string{"tasks.created", "tasks.updated", "tasks.completed", "tasks.deleted"}
	for _, topic := range topics {
		err = r.Route(context.Background(), topic, []byte(`{}`))
		if err != nil {
			t.Errorf("route %q failed: %v", topic, err)
		}
	}

	if atomic.LoadInt32(&handlerCalled) != int32(len(topics)) {
		t.Errorf("handler called %d times, want %d", handlerCalled, len(topics))
	}

	// Should not match
	atomic.StoreInt32(&handlerCalled, 0)
	err = r.Route(context.Background(), "alerts.critical", []byte(`{}`))
	if err != nil {
		t.Errorf("route failed: %v", err)
	}
	if atomic.LoadInt32(&handlerCalled) != 0 {
		t.Errorf("handler should not be called for non-matching topic")
	}
}

func TestRouter_Route_MultipleHandlers(t *testing.T) {
	r := NewRouter(nil)

	var handler1Called, handler2Called int32

	r.Register("tasks.created", func(ctx context.Context, topic string, payload []byte) error {
		atomic.AddInt32(&handler1Called, 1)
		return nil
	})

	r.Register("tasks.created", func(ctx context.Context, topic string, payload []byte) error {
		atomic.AddInt32(&handler2Called, 1)
		return nil
	})

	err := r.Route(context.Background(), "tasks.created", []byte(`{}`))
	if err != nil {
		t.Errorf("route failed: %v", err)
	}

	if atomic.LoadInt32(&handler1Called) != 1 {
		t.Errorf("handler1 called %d times, want 1", handler1Called)
	}
	if atomic.LoadInt32(&handler2Called) != 1 {
		t.Errorf("handler2 called %d times, want 1", handler2Called)
	}
}

func TestRouter_Route_NoHandler(t *testing.T) {
	r := NewRouter(nil)

	// Should not error when no handler exists
	err := r.Route(context.Background(), "unknown.topic", []byte(`{}`))
	if err != nil {
		t.Errorf("should not error on missing handler: %v", err)
	}

	stats := r.Stats()
	if stats["messages_dropped"].(int64) != 1 {
		t.Errorf("should track dropped message")
	}
}

func TestRouter_RouteEnvelope(t *testing.T) {
	r := NewRouter(nil)

	var receivedPayload string

	r.Register("tasks.created", func(ctx context.Context, topic string, payload []byte) error {
		receivedPayload = string(payload)
		return nil
	})

	env, err := NewEnvelope("tasks.created", "test", map[string]string{"task_id": "task-123"})
	if err != nil {
		t.Fatalf("create envelope failed: %v", err)
	}

	err = r.RouteEnvelope(context.Background(), env)
	if err != nil {
		t.Errorf("route envelope failed: %v", err)
	}

	if receivedPayload == "" {
		t.Error("handler should have received payload")
	}
}

func TestRouter_Close(t *testing.T) {
	r := NewRouter(nil)

	r.Register("tasks.created", func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})

	err := r.Close()
	if err != nil {
		t.Errorf("close failed: %v", err)
	}

	// Should error on closed router
	err = r.Register("tasks.updated", func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})
	if err != ErrRouterClosed {
		t.Errorf("expected ErrRouterClosed, got %v", err)
	}

	err = r.Route(context.Background(), "tasks.created", []byte(`{}`))
	if err != ErrRouterClosed {
		t.Errorf("expected ErrRouterClosed, got %v", err)
	}
}

func TestRouter_Stats(t *testing.T) {
	r := NewRouter(nil)

	r.Register("tasks.*", func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})
	r.Register("alerts.critical", func(ctx context.Context, topic string, payload []byte) error {
		return nil
	})

	r.Route(context.Background(), "tasks.created", []byte(`{}`))
	r.Route(context.Background(), "unknown.topic", []byte(`{}`))

	stats := r.Stats()

	if stats["topic_patterns"].(int) != 2 {
		t.Errorf("topic_patterns = %d, want 2", stats["topic_patterns"])
	}
	if stats["messages_routed"].(int64) != 1 {
		t.Errorf("messages_routed = %d, want 1", stats["messages_routed"])
	}
	if stats["messages_dropped"].(int64) != 1 {
		t.Errorf("messages_dropped = %d, want 1", stats["messages_dropped"])
	}
}

func TestRegisterTyped(t *testing.T) {
	r := NewRouter(nil)

	var receivedEvent *TaskEvent

	err := RegisterTyped(r, "tasks.created", func(ctx context.Context, event *TaskEvent) error {
		receivedEvent = event
		return nil
	})
	if err != nil {
		t.Fatalf("register typed failed: %v", err)
	}

	event := &TaskEvent{
		TaskID: "task-123",
		Title:  "Test Task",
		Status: "in-progress",
	}

	payload, _ := json.Marshal(event)
	err = r.Route(context.Background(), "tasks.created", payload)
	if err != nil {
		t.Errorf("route failed: %v", err)
	}

	if receivedEvent == nil {
		t.Fatal("handler should have received event")
	}
	if receivedEvent.TaskID != event.TaskID {
		t.Errorf("task_id = %q, want %q", receivedEvent.TaskID, event.TaskID)
	}
	if receivedEvent.Title != event.Title {
		t.Errorf("title = %q, want %q", receivedEvent.Title, event.Title)
	}
}

func TestMatchesTopic(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		want    bool
	}{
		// Exact matches
		{"tasks.created", "tasks.created", true},
		{"tasks.created", "tasks.updated", false},

		// Wildcard matches
		{"tasks.*", "tasks.created", true},
		{"tasks.*", "tasks.updated", true},
		{"tasks.*", "tasks.completed", true},
		{"tasks.*", "alerts.critical", false},

		// Prefix wildcard
		{"*.critical", "alerts.critical", true},
		{"*.critical", "state.critical", true},
		{"*.critical", "alerts.warning", false},

		// Universal wildcard
		{"*", "anything", true},
		{"*", "tasks.created", true},
		{"*", "", true},

		// No match
		{"tasks.created", "", false},
		{"tasks.created", "tasks", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.topic, func(t *testing.T) {
			got := matchesTopic(tt.pattern, tt.topic)
			if got != tt.want {
				t.Errorf("matchesTopic(%q, %q) = %v, want %v", tt.pattern, tt.topic, got, tt.want)
			}
		})
	}
}

func TestStandardRoutes(t *testing.T) {
	r := NewRouter(nil)

	err := r.StandardRoutes()
	if err != nil {
		t.Fatalf("standard routes failed: %v", err)
	}

	stats := r.Stats()
	if stats["topic_patterns"].(int) < 2 {
		t.Errorf("expected at least 2 patterns, got %d", stats["topic_patterns"])
	}
}

func TestTimeguruRoutes(t *testing.T) {
	r := NewRouter(nil)

	var taskReceived *TaskEvent

	err := TimeguruRoutes(r, func(ctx context.Context, event *TaskEvent) error {
		taskReceived = event
		return nil
	})
	if err != nil {
		t.Fatalf("timeguru routes failed: %v", err)
	}

	event := &TaskEvent{
		TaskID: "task-123",
		Title:  "Completed Task",
		Status: "done",
	}
	payload, _ := json.Marshal(event)

	err = r.Route(context.Background(), TopicTasksCompleted, payload)
	if err != nil {
		t.Errorf("route failed: %v", err)
	}

	if taskReceived == nil {
		t.Fatal("handler should have received task")
	}
	if taskReceived.TaskID != event.TaskID {
		t.Errorf("task_id = %q, want %q", taskReceived.TaskID, event.TaskID)
	}
}

func TestCaptainRoutes(t *testing.T) {
	r := NewRouter(nil)

	var milestoneReceived *MilestoneHitEvent

	err := CaptainRoutes(r, func(ctx context.Context, event *MilestoneHitEvent) error {
		milestoneReceived = event
		return nil
	})
	if err != nil {
		t.Fatalf("captain routes failed: %v", err)
	}

	event := &MilestoneHitEvent{
		MilestoneID:   "milestone-1",
		MilestoneName: "Alpha Complete",
		Phase:         "Age 1",
		CompletedAt:   time.Now(),
	}
	payload, _ := json.Marshal(event)

	err = r.Route(context.Background(), TopicTimelineMilestoneHit, payload)
	if err != nil {
		t.Errorf("route failed: %v", err)
	}

	if milestoneReceived == nil {
		t.Fatal("handler should have received milestone")
	}
	if milestoneReceived.MilestoneID != event.MilestoneID {
		t.Errorf("milestone_id = %q, want %q", milestoneReceived.MilestoneID, event.MilestoneID)
	}
}
