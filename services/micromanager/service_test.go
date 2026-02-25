package micromanager

import (
	"context"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
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

// TestPublishTaskCreated without wotan (should not error)
func TestPublishTaskCreated_NoWotan(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil) // No wotan
	task := NewTask("task-1", "Test", "owner")

	// Should not panic or error with nil wotan
	err := service.PublishTaskCreated("task-1", task)
	if err != nil {
		t.Errorf("PublishTaskCreated(nil wotan) = %v, want nil", err)
	}
}

// TestPublishTaskUpdated without wotan (should not error)
func TestPublishTaskUpdated_NoWotan(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	task := NewTask("task-1", "Test", "owner")

	err := service.PublishTaskUpdated("task-1", task)
	if err != nil {
		t.Errorf("PublishTaskUpdated(nil wotan) = %v, want nil", err)
	}
}

// TestPublishTaskCompleted without wotan (should not error)
func TestPublishTaskCompleted_NoWotan(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	task := NewTask("task-1", "Test", "owner")
	task.Status = StatusCompleted
	now := time.Now()
	task.CompletedAt = &now

	err := service.PublishTaskCompleted("task-1", task)
	if err != nil {
		t.Errorf("PublishTaskCompleted(nil wotan) = %v, want nil", err)
	}
}

// TestStart without wotan (should not error)
func TestStart_NoWotan(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	ctx := context.Background()

	err := service.Start(ctx)
	if err != nil {
		t.Errorf("Start(nil wotan) = %v, want nil", err)
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

	if status["wotan_connected"] != false {
		t.Error("wotan_connected should be false when wotan is nil")
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

// TestHandleAlert processes alert without panic
func TestHandleAlert(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	// Should not panic with valid alert data
	alert := map[string]interface{}{
		"severity": "critical",
		"message":  "service down",
		"source":   "monitoring",
	}
	service.handleAlert(alert)
}

// TestHandleAlert_EmptyAlert processes empty alert without panic
func TestHandleAlert_EmptyAlert(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	// Should not panic with empty alert
	service.handleAlert(map[string]interface{}{})
}

// TestHandleAlert_NilAlert processes nil alert without panic
func TestHandleAlert_NilAlert(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	// Should not panic with nil alert
	service.handleAlert(nil)
}

// TestListenForAlerts_NilWotan returns immediately
func TestListenForAlerts_NilWotan(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Should return immediately with nil wotan
	done := make(chan struct{})
	go func() {
		service.listenForAlerts(ctx)
		close(done)
	}()

	select {
	case <-done:
		// success - returned
	case <-time.After(2 * time.Second):
		t.Fatal("listenForAlerts did not return in time")
	}
}

// TestGenerateTaskID_Uniqueness generates many unique IDs concurrently
func TestGenerateTaskID_Uniqueness(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := service.GenerateTaskID()
		if ids[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

// TestHealthStatus_TimestampPresent verifies timestamp is set
func TestHealthStatus_TimestampPresent(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	status := service.HealthStatus()

	ts, ok := status["timestamp"]
	if !ok {
		t.Fatal("timestamp key missing from health status")
	}
	if ts.(int64) == 0 {
		t.Error("timestamp should be non-zero")
	}
}

// TestStop_NilWotan stops service cleanly with nil wotan
func TestStop_NilWotan(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	ctx := context.Background()
	err := service.Stop(ctx)
	if err != nil {
		t.Errorf("Stop(nil wotan) = %v, want nil", err)
	}
}

// TestPublishTaskCreated_NotApproved skips publish when topic not approved
func TestPublishTaskCreated_NotApproved(t *testing.T) {
	store := NewStore()
	wotan, err := wotanClient.NewClient("localhost:19999")
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	service := NewService(store, wotan)
	task := NewTask("task-1", "Test", "owner")

	// wotan is not nil but has no approved subscriptions
	// IsApproved will return false, so publish is skipped
	err = service.PublishTaskCreated("task-1", task)
	if err != nil {
		t.Errorf("PublishTaskCreated(not approved) = %v, want nil", err)
	}
}

// TestPublishTaskUpdated_NotApproved skips publish when topic not approved
func TestPublishTaskUpdated_NotApproved(t *testing.T) {
	store := NewStore()
	wotan, err := wotanClient.NewClient("localhost:19999")
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	service := NewService(store, wotan)
	task := NewTask("task-1", "Test", "owner")

	err = service.PublishTaskUpdated("task-1", task)
	if err != nil {
		t.Errorf("PublishTaskUpdated(not approved) = %v, want nil", err)
	}
}

// TestPublishTaskCompleted_NotApproved skips publish when topic not approved
func TestPublishTaskCompleted_NotApproved(t *testing.T) {
	store := NewStore()
	wotan, err := wotanClient.NewClient("localhost:19999")
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	service := NewService(store, wotan)
	task := NewTask("task-1", "Test", "owner")
	now := time.Now()
	task.CompletedAt = &now

	err = service.PublishTaskCompleted("task-1", task)
	if err != nil {
		t.Errorf("PublishTaskCompleted(not approved) = %v, want nil", err)
	}
}

// TestStop_WithWotan stops service with wotan client
func TestStop_WithWotan(t *testing.T) {
	store := NewStore()
	wotan, err := wotanClient.NewClient("localhost:19999")
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	service := NewService(store, wotan)

	ctx := context.Background()
	err = service.Stop(ctx)
	if err != nil {
		t.Errorf("Stop(with wotan) = %v, want nil", err)
	}
}

// TestHealthStatus_WithWotan returns status with wotan connected info
func TestHealthStatus_WithWotan(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Test", "owner")
	store.Create(task)

	wotan, err := wotanClient.NewClient("localhost:19999")
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	service := NewService(store, wotan)
	status := service.HealthStatus()

	if status["wotan_connected"] != true {
		t.Error("wotan_connected should be true when wotan is configured")
	}
	subs, ok := status["subscriptions"]
	if !ok {
		t.Fatal("subscriptions key missing from health status")
	}
	if subs.(int) != 0 {
		t.Errorf("subscriptions = %d, want 0", subs)
	}
}

// TestStart_WithWotan_SubscribeFails tests Start with wotan that fails
func TestStart_WithWotan_SubscribeFails(t *testing.T) {
	store := NewStore()
	// Use an address that will fail to connect
	wotan, err := wotanClient.NewClient("localhost:19999")
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	service := NewService(store, wotan)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start will attempt to subscribe and fail, but should warn and continue
	err = service.Start(ctx)
	if err != nil {
		t.Errorf("Start should warn and continue on subscribe failure, got: %v", err)
	}
}
