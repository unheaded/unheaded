package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewEnvelope(t *testing.T) {
	tests := []struct {
		name      string
		topic     string
		senderID  string
		payload   interface{}
		wantErr   bool
		errString string
	}{
		{
			name:     "valid envelope",
			topic:    TopicTasksCreated,
			senderID: "micromanager",
			payload:  map[string]string{"task_id": "task-123"},
			wantErr:  false,
		},
		{
			name:      "empty topic",
			topic:     "",
			senderID:  "micromanager",
			payload:   map[string]string{"task_id": "task-123"},
			wantErr:   true,
			errString: "topic cannot be empty",
		},
		{
			name:      "empty sender",
			topic:     TopicTasksCreated,
			senderID:  "",
			payload:   map[string]string{"task_id": "task-123"},
			wantErr:   true,
			errString: "sender cannot be empty",
		},
		{
			name:     "nil payload allowed",
			topic:    TopicTasksCreated,
			senderID: "micromanager",
			payload:  nil,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := NewEnvelope(tt.topic, tt.senderID, tt.payload)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if env.Topic != tt.topic {
				t.Errorf("topic = %q, want %q", env.Topic, tt.topic)
			}
			if env.SenderID != tt.senderID {
				t.Errorf("senderID = %q, want %q", env.SenderID, tt.senderID)
			}
			if env.MessageID == "" {
				t.Error("message_id should not be empty")
			}
			if env.Timestamp.IsZero() {
				t.Error("timestamp should not be zero")
			}
			if env.Version == "" {
				t.Error("version should not be empty")
			}
		})
	}
}

func TestEnvelope_DecodePayload(t *testing.T) {
	type testPayload struct {
		TaskID string `json:"task_id"`
		Title  string `json:"title"`
	}

	original := testPayload{
		TaskID: "task-123",
		Title:  "Test Task",
	}

	env, err := NewEnvelope(TopicTasksCreated, "test", original)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	var decoded testPayload
	if err := env.DecodePayload(&decoded); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}

	if decoded.TaskID != original.TaskID {
		t.Errorf("task_id = %q, want %q", decoded.TaskID, original.TaskID)
	}
	if decoded.Title != original.Title {
		t.Errorf("title = %q, want %q", decoded.Title, original.Title)
	}
}

func TestTaskEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		event   *TaskEvent
		wantErr bool
	}{
		{
			name: "valid task event",
			event: &TaskEvent{
				TaskID:    "task-123",
				Title:     "Test Task",
				Status:    "in-progress",
				Priority:  PriorityHigh,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
		},
		{
			name: "empty task_id",
			event: &TaskEvent{
				TaskID: "",
				Title:  "Test Task",
				Status: "in-progress",
			},
			wantErr: true,
		},
		{
			name: "empty title",
			event: &TaskEvent{
				TaskID: "task-123",
				Title:  "",
				Status: "in-progress",
			},
			wantErr: true,
		},
		{
			name: "empty status",
			event: &TaskEvent{
				TaskID: "task-123",
				Title:  "Test Task",
				Status: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDecisionEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		event   *DecisionEvent
		wantErr bool
	}{
		{
			name: "valid decision event",
			event: &DecisionEvent{
				DecisionID: "decision-123",
				Title:      "Test Decision",
				Content:    "Test content",
				Owner:      "captain",
				Priority:   PriorityHigh,
				Status:     "approved",
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			},
			wantErr: false,
		},
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
		},
		{
			name: "empty decision_id",
			event: &DecisionEvent{
				DecisionID: "",
				Title:      "Test Decision",
				Owner:      "captain",
			},
			wantErr: true,
		},
		{
			name: "empty title",
			event: &DecisionEvent{
				DecisionID: "decision-123",
				Title:      "",
				Owner:      "captain",
			},
			wantErr: true,
		},
		{
			name: "empty owner",
			event: &DecisionEvent{
				DecisionID: "decision-123",
				Title:      "Test Decision",
				Owner:      "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAlertEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		event   *AlertEvent
		wantErr bool
	}{
		{
			name: "valid alert event",
			event: &AlertEvent{
				AlertID:   "alert-123",
				Severity:  SeverityCritical,
				Source:    "vambraces",
				Title:     "Test Alert",
				CreatedAt: time.Now(),
			},
			wantErr: false,
		},
		{
			name:    "nil event",
			event:   nil,
			wantErr: true,
		},
		{
			name: "empty alert_id",
			event: &AlertEvent{
				AlertID: "",
				Source:  "vambraces",
				Title:   "Test Alert",
			},
			wantErr: true,
		},
		{
			name: "empty source",
			event: &AlertEvent{
				AlertID: "alert-123",
				Source:  "",
				Title:   "Test Alert",
			},
			wantErr: true,
		},
		{
			name: "empty title",
			event: &AlertEvent{
				AlertID: "alert-123",
				Source:  "vambraces",
				Title:   "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPriority_String(t *testing.T) {
	tests := []struct {
		priority Priority
		want     string
	}{
		{PriorityLow, "low"},
		{PriorityMedium, "medium"},
		{PriorityHigh, "high"},
		{PriorityCritical, "critical"},
		{Priority(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.priority.String(); got != tt.want {
				t.Errorf("Priority.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarshalUnmarshalEvent(t *testing.T) {
	original := &TaskEvent{
		TaskID:    "task-123",
		Title:     "Test Task",
		Status:    "in-progress",
		Priority:  PriorityHigh,
		CreatedAt: time.Now().Truncate(time.Second), // Truncate for comparison
		UpdatedAt: time.Now().Truncate(time.Second),
	}

	data, err := MarshalEvent(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded TaskEvent
	if err := UnmarshalEvent(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.TaskID != original.TaskID {
		t.Errorf("task_id = %q, want %q", decoded.TaskID, original.TaskID)
	}
	if decoded.Title != original.Title {
		t.Errorf("title = %q, want %q", decoded.Title, original.Title)
	}
	if decoded.Status != original.Status {
		t.Errorf("status = %q, want %q", decoded.Status, original.Status)
	}
	if decoded.Priority != original.Priority {
		t.Errorf("priority = %v, want %v", decoded.Priority, original.Priority)
	}
}

func TestTopicConstants(t *testing.T) {
	// Verify all topics follow the naming convention
	topics := []string{
		TopicTasksCreated,
		TopicTasksUpdated,
		TopicTasksCompleted,
		TopicTimelineUpdates,
		TopicTimelineMilestoneHit,
		TopicDecisionsCreated,
		TopicDecisionsApproved,
		TopicAlertsCritical,
		TopicAlertsWarning,
		TopicStateContainerCreated,
		TopicStateDriftDetected,
	}

	for _, topic := range topics {
		if topic == "" {
			t.Errorf("topic constant is empty")
		}
		// Should contain at least one dot (domain.event format)
		hasDot := false
		for _, c := range topic {
			if c == '.' {
				hasDot = true
				break
			}
		}
		if !hasDot && topic != TopicTasksAll {
			// Wildcards are allowed to not have dots
			t.Errorf("topic %q should contain a dot separator", topic)
		}
	}
}

func TestContainerStateEvent(t *testing.T) {
	event := ContainerStateEvent{
		ContainerID:   "container-123",
		ContainerName: "busboy-1",
		State:         "running",
		PreviousState: "stopped",
		HealthStatus:  "healthy",
		Timestamp:     time.Now(),
		Metadata: map[string]string{
			"image": "unheaded/busboy:latest",
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ContainerStateEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.ContainerID != event.ContainerID {
		t.Errorf("container_id = %q, want %q", decoded.ContainerID, event.ContainerID)
	}
	if decoded.State != event.State {
		t.Errorf("state = %q, want %q", decoded.State, event.State)
	}
	if decoded.HealthStatus != event.HealthStatus {
		t.Errorf("health_status = %q, want %q", decoded.HealthStatus, event.HealthStatus)
	}
}

func TestDriftEvent(t *testing.T) {
	event := DriftEvent{
		DriftID:      "drift-123",
		ContainerID:  "container-456",
		DriftType:    "missing",
		Severity:     "high",
		DesiredState: "running",
		ActualState:  "not_found",
		DetectedAt:   time.Now(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded DriftEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.DriftID != event.DriftID {
		t.Errorf("drift_id = %q, want %q", decoded.DriftID, event.DriftID)
	}
	if decoded.DriftType != event.DriftType {
		t.Errorf("drift_type = %q, want %q", decoded.DriftType, event.DriftType)
	}
}
