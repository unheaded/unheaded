package micromanager

import (
	"context"
	"testing"
	"time"
)

// TestNewService creates a service successfully
func TestNewService(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	if service == nil {
		t.Fatal("NewService returned nil")
	}
	if service.store != store {
		t.Error("service.store not set correctly")
	}
}

// TestGenerateTaskID creates unique IDs
func TestGenerateTaskID(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	id1 := service.GenerateTaskID()
	if id1 == "" {
		t.Fatal("GenerateTaskID returned empty string")
	}

	time.Sleep(1 * time.Millisecond)
	id2 := service.GenerateTaskID()

	if id1 == id2 {
		t.Errorf("GenerateTaskID generated duplicate IDs: %s == %s", id1, id2)
	}

	// Verify ID format
	if len(id1) == 0 {
		t.Error("ID is empty")
	}
}

// TestGenerateTaskID_Sequential generates sequential IDs
func TestGenerateTaskID_Sequential(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	id1 := service.GenerateTaskID()
	id2 := service.GenerateTaskID()
	id3 := service.GenerateTaskID()

	if id1 == id2 || id2 == id3 || id1 == id3 {
		t.Error("IDs are not unique")
	}
}

// TestPublishTaskCreated without busboy (should not error)
func TestPublishTaskCreated_NoBusboy(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil) // No busboy
	task := NewTask("task-1", "Test", "owner")

	// Should not panic or error with nil busboy
	err := service.PublishTaskCreated("task-1", task)
	if err != nil {
		t.Errorf("PublishTaskCreated(nil busboy) = %v, want nil", err)
	}
}

// TestPublishTaskUpdated without busboy (should not error)
func TestPublishTaskUpdated_NoBusboy(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	task := NewTask("task-1", "Test", "owner")

	err := service.PublishTaskUpdated("task-1", task)
	if err != nil {
		t.Errorf("PublishTaskUpdated(nil busboy) = %v, want nil", err)
	}
}

// TestPublishTaskCompleted without busboy (should not error)
func TestPublishTaskCompleted_NoBusboy(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	task := NewTask("task-1", "Test", "owner")
	task.Status = StatusCompleted
	now := time.Now()
	task.CompletedAt = &now

	err := service.PublishTaskCompleted("task-1", task)
	if err != nil {
		t.Errorf("PublishTaskCompleted(nil busboy) = %v, want nil", err)
	}
}

// TestStart without busboy (should not error)
func TestStart_NoBusboy(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Errorf("Start(nil busboy) = %v, want nil", err)
	}
}

// TestStop closes gracefully
func TestStop(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	ctx := context.Background()
	err := service.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}

// TestHealthStatus returns valid status
func TestHealthStatus(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner")
	store.Create(task)

	service := NewService(store, nil)
	status := service.HealthStatus()

	if status == nil {
		t.Fatal("HealthStatus returned nil")
	}

	if status["service"] != "micromanager" {
		t.Errorf("service = %s, want micromanager", status["service"])
	}

	if status["status"] != "healthy" {
		t.Errorf("status = %s, want healthy", status["status"])
	}

	if int(status["tasks_count"].(int)) != 1 {
		t.Errorf("tasks_count = %d, want 1", status["tasks_count"])
	}

	if status["busboy_connected"] != false {
		t.Error("busboy_connected should be false when busboy is nil")
	}
}

// TestHealthStatus_EmptyStore returns zero tasks
func TestHealthStatus_EmptyStore(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	status := service.HealthStatus()

	if int(status["tasks_count"].(int)) != 0 {
		t.Errorf("tasks_count = %d, want 0", status["tasks_count"])
	}
}
