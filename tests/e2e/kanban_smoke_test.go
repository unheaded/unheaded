// Package e2e provides E2E integration tests for the Kanban application.
// Story 5.2: E2E Smoke Test - THE META MOMENT
//
// This test file verifies the full Kanban flow:
// - Start kanban-app (mock for unit testing)
// - Create/move/complete tasks via API
// - Verify persistence
// - Verify Wotan events are published
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/wotan-client/mock"
)

// ============================================================================
// KANBAN E2E SMOKE TEST - Story 5.2
// ============================================================================

// KanbanTask mirrors the Task struct from kanban-app for testing
type KanbanTask struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Type        string    `json:"type,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Progress    int       `json:"progress,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// KanbanTestHarness provides a coordinated test environment for Kanban E2E tests
type KanbanTestHarness struct {
	t *testing.T

	// Mock Wotan client for event verification
	MockWotan *mock.MockClient

	// Event tracking
	mu              sync.RWMutex
	publishedEvents []PublishedEvent

	// Task storage (simulates server-side persistence)
	tasks   map[string]*KanbanTask
	tasksMu sync.RWMutex

	// HTTP test server
	server *httptest.Server
}

// PublishedEvent tracks events published to Wotan
type PublishedEvent struct {
	Topic     string
	Payload   []byte
	Timestamp time.Time
}

// NewKanbanTestHarness creates a new Kanban E2E test harness
func NewKanbanTestHarness(t *testing.T) *KanbanTestHarness {
	t.Helper()

	h := &KanbanTestHarness{
		t:               t,
		MockWotan:      mock.NewMockClient(mock.WithAutoApprove()),
		publishedEvents: make([]PublishedEvent, 0),
		tasks:           make(map[string]*KanbanTask),
	}

	// Setup HTTP test server with handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tasks", h.handleTasks)
	mux.HandleFunc("/api/v1/tasks/", h.handleTaskByID)
	mux.HandleFunc("/api/v1/health", h.handleHealth)

	h.server = httptest.NewServer(mux)

	return h
}

// Cleanup cleans up test resources
func (h *KanbanTestHarness) Cleanup() {
	if h.server != nil {
		h.server.Close()
	}
}

// BaseURL returns the test server URL
func (h *KanbanTestHarness) BaseURL() string {
	return h.server.URL
}

// GetPublishedEvents returns all events published to Wotan
func (h *KanbanTestHarness) GetPublishedEvents() []PublishedEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]PublishedEvent{}, h.publishedEvents...)
}

// GetEventsByTopic returns events filtered by topic
func (h *KanbanTestHarness) GetEventsByTopic(topic string) []PublishedEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var filtered []PublishedEvent
	for _, e := range h.publishedEvents {
		if e.Topic == topic {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// ============================================================================
// HTTP HANDLERS (Simulate Kanban App)
// ============================================================================

func (h *KanbanTestHarness) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGetTasks(w, r)
	case http.MethodPost:
		h.handleCreateTask(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *KanbanTestHarness) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	h.tasksMu.RLock()
	defer h.tasksMu.RUnlock()

	tasks := make([]*KanbanTask, 0, len(h.tasks))
	for _, task := range h.tasks {
		tasks = append(tasks, task)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	})
}

func (h *KanbanTestHarness) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var task KanbanTask
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate
	if task.ID == "" || task.Title == "" || task.Status == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Set timestamps
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now

	// Store task
	h.tasksMu.Lock()
	h.tasks[task.ID] = &task
	h.tasksMu.Unlock()

	// Publish to Wotan
	h.publishEvent("tasks.created", &task)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"task": task})
}

func (h *KanbanTestHarness) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	// Extract task ID from path
	path := r.URL.Path
	prefix := "/api/v1/tasks/"
	if len(path) <= len(prefix) {
		http.Error(w, "Task ID required", http.StatusBadRequest)
		return
	}
	taskID := path[len(prefix):]

	switch r.Method {
	case http.MethodGet:
		h.handleGetTaskByID(w, taskID)
	case http.MethodPut:
		h.handleUpdateTaskByID(w, r, taskID)
	case http.MethodDelete:
		h.handleDeleteTaskByID(w, taskID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *KanbanTestHarness) handleGetTaskByID(w http.ResponseWriter, taskID string) {
	h.tasksMu.RLock()
	task, exists := h.tasks[taskID]
	h.tasksMu.RUnlock()

	if !exists {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (h *KanbanTestHarness) handleUpdateTaskByID(w http.ResponseWriter, r *http.Request, taskID string) {
	var task KanbanTask
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	task.ID = taskID

	h.tasksMu.Lock()
	existing, exists := h.tasks[taskID]
	if !exists {
		h.tasksMu.Unlock()
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Preserve created_at, update updated_at
	task.CreatedAt = existing.CreatedAt
	task.UpdatedAt = time.Now()

	// Detect status change (column move)
	oldStatus := existing.Status
	newStatus := task.Status

	h.tasks[taskID] = &task
	h.tasksMu.Unlock()

	// Determine event type
	eventTopic := "tasks.updated"
	if oldStatus != newStatus && newStatus == "done" {
		eventTopic = "tasks.completed"
	}

	// Publish to Wotan
	h.publishEvent(eventTopic, &task)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"task": task})
}

func (h *KanbanTestHarness) handleDeleteTaskByID(w http.ResponseWriter, taskID string) {
	h.tasksMu.Lock()
	task, exists := h.tasks[taskID]
	if !exists {
		h.tasksMu.Unlock()
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	delete(h.tasks, taskID)
	h.tasksMu.Unlock()

	// Publish to Wotan
	h.publishEvent("tasks.deleted", task)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "task_id": taskID})
}

func (h *KanbanTestHarness) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// publishEvent records an event and publishes to mock Wotan
func (h *KanbanTestHarness) publishEvent(topic string, data interface{}) {
	payload, _ := json.Marshal(data)

	h.mu.Lock()
	h.publishedEvents = append(h.publishedEvents, PublishedEvent{
		Topic:     topic,
		Payload:   payload,
		Timestamp: time.Now(),
	})
	h.mu.Unlock()

	// Also publish to mock Wotan
	ctx := context.Background()
	h.MockWotan.Subscribe(ctx, topic, "test")
	h.MockWotan.ApproveSubscription(topic)
	h.MockWotan.Publish(ctx, topic, payload)
}

// ============================================================================
// E2E SMOKE TESTS
// ============================================================================

// TestKanbanE2E_SmokeTest tests the complete Kanban workflow
// Story 5.2: E2E Smoke Test
func TestKanbanE2E_SmokeTest(t *testing.T) {
	harness := NewKanbanTestHarness(t)
	defer harness.Cleanup()

	ctx := context.Background()

	// ===== STEP 1: Verify Health Endpoint =====
	t.Log("Step 1: Verify health endpoint...")

	resp, err := http.Get(harness.BaseURL() + "/api/v1/health")
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected health status 200, got %d", resp.StatusCode)
	}

	var healthResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&healthResp)
	if healthResp["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", healthResp["status"])
	}

	t.Log("Health check passed")

	// ===== STEP 2: Create a Task (TODO column) =====
	t.Log("Step 2: Create a new task...")

	task := &KanbanTask{
		ID:          "e2e-smoke-task-1",
		Title:       "E2E Smoke Test Task",
		Description: "This task was created by the E2E smoke test",
		Status:      "todo",
		Type:        "task",
		Owner:       "E2E Test",
		Progress:    0,
	}

	taskPayload, _ := json.Marshal(task)
	resp, err = http.Post(harness.BaseURL()+"/api/v1/tasks", "application/json", bytes.NewReader(taskPayload))
	if err != nil {
		t.Fatalf("Create task failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected create status 201, got %d", resp.StatusCode)
	}

	var createResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&createResp)
	createdTask := createResp["task"].(map[string]interface{})
	if createdTask["id"] != task.ID {
		t.Errorf("Expected task ID %s, got %v", task.ID, createdTask["id"])
	}

	t.Logf("Task created: %s", task.ID)

	// Verify Wotan event was published
	createdEvents := harness.GetEventsByTopic("tasks.created")
	if len(createdEvents) == 0 {
		t.Error("Expected tasks.created event to be published")
	}

	// ===== STEP 3: Move Task to In-Progress =====
	t.Log("Step 3: Move task to in-progress column...")

	task.Status = "in-progress"
	task.Progress = 25
	taskPayload, _ = json.Marshal(task)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, harness.BaseURL()+"/api/v1/tasks/"+task.ID, bytes.NewReader(taskPayload))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Update task failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected update status 200, got %d", resp.StatusCode)
	}

	t.Logf("Task moved to in-progress")

	// Verify Wotan update event
	updatedEvents := harness.GetEventsByTopic("tasks.updated")
	if len(updatedEvents) == 0 {
		t.Error("Expected tasks.updated event to be published")
	}

	// ===== STEP 4: Complete the Task (Move to Done) =====
	t.Log("Step 4: Complete the task (move to done)...")

	task.Status = "done"
	task.Progress = 100
	taskPayload, _ = json.Marshal(task)

	req, _ = http.NewRequestWithContext(ctx, http.MethodPut, harness.BaseURL()+"/api/v1/tasks/"+task.ID, bytes.NewReader(taskPayload))
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Complete task failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected complete status 200, got %d", resp.StatusCode)
	}

	t.Logf("Task completed")

	// Verify Wotan completed event
	completedEvents := harness.GetEventsByTopic("tasks.completed")
	if len(completedEvents) == 0 {
		t.Error("Expected tasks.completed event to be published")
	}

	// ===== STEP 5: Verify Persistence =====
	t.Log("Step 5: Verify task persistence...")

	resp, err = http.Get(harness.BaseURL() + "/api/v1/tasks/" + task.ID)
	if err != nil {
		t.Fatalf("Get task failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected get status 200, got %d", resp.StatusCode)
	}

	var persistedTask KanbanTask
	json.NewDecoder(resp.Body).Decode(&persistedTask)

	if persistedTask.Status != "done" {
		t.Errorf("Expected persisted status 'done', got %s", persistedTask.Status)
	}
	if persistedTask.Progress != 100 {
		t.Errorf("Expected persisted progress 100, got %d", persistedTask.Progress)
	}

	t.Logf("Persistence verified: status=%s, progress=%d", persistedTask.Status, persistedTask.Progress)

	// ===== STEP 6: Verify All Wotan Events =====
	t.Log("Step 6: Verify all Wotan events...")

	allEvents := harness.GetPublishedEvents()
	t.Logf("Total events published: %d", len(allEvents))

	expectedTopics := map[string]bool{
		"tasks.created":   false,
		"tasks.updated":   false,
		"tasks.completed": false,
	}

	for _, event := range allEvents {
		if _, ok := expectedTopics[event.Topic]; ok {
			expectedTopics[event.Topic] = true
		}
		t.Logf("  - %s at %s", event.Topic, event.Timestamp.Format(time.RFC3339))
	}

	for topic, found := range expectedTopics {
		if !found {
			t.Errorf("Missing expected event: %s", topic)
		}
	}

	// ===== STEP 7: Get All Tasks =====
	t.Log("Step 7: Get all tasks...")

	resp, err = http.Get(harness.BaseURL() + "/api/v1/tasks")
	if err != nil {
		t.Fatalf("Get all tasks failed: %v", err)
	}
	defer resp.Body.Close()

	var tasksResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tasksResp)

	count := int(tasksResp["count"].(float64))
	if count < 1 {
		t.Error("Expected at least 1 task in the list")
	}

	t.Logf("Total tasks: %d", count)

	// ===== COMPLETE =====
	t.Log("E2E Smoke Test PASSED - All steps verified")
}

// TestKanbanE2E_TaskLifecycle tests full task lifecycle with events
func TestKanbanE2E_TaskLifecycle(t *testing.T) {
	harness := NewKanbanTestHarness(t)
	defer harness.Cleanup()

	ctx := context.Background()
	client := &http.Client{}

	// Create multiple tasks
	tasks := []KanbanTask{
		{ID: "lifecycle-1", Title: "Task 1", Status: "todo", Type: "feature"},
		{ID: "lifecycle-2", Title: "Task 2", Status: "todo", Type: "bug"},
		{ID: "lifecycle-3", Title: "Task 3", Status: "todo", Type: "task"},
	}

	t.Log("Creating tasks...")
	for _, task := range tasks {
		payload, _ := json.Marshal(task)
		resp, err := http.Post(harness.BaseURL()+"/api/v1/tasks", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("Create task %s failed: %v", task.ID, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Create task %s: expected 201, got %d", task.ID, resp.StatusCode)
		}
	}

	// Verify all tasks created
	createdEvents := harness.GetEventsByTopic("tasks.created")
	if len(createdEvents) != 3 {
		t.Errorf("Expected 3 tasks.created events, got %d", len(createdEvents))
	}

	// Move tasks through columns
	t.Log("Moving tasks through columns...")

	// Task 1: todo -> in-progress -> done
	for _, update := range []struct {
		id       string
		status   string
		progress int
	}{
		{"lifecycle-1", "in-progress", 50},
		{"lifecycle-1", "done", 100},
		{"lifecycle-2", "in-progress", 25},
	} {
		payload, _ := json.Marshal(map[string]interface{}{
			"title":    "Task",
			"status":   update.status,
			"progress": update.progress,
		})

		req, _ := http.NewRequestWithContext(ctx, http.MethodPut,
			harness.BaseURL()+"/api/v1/tasks/"+update.id, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Update task %s failed: %v", update.id, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Update task %s: expected 200, got %d", update.id, resp.StatusCode)
		}
	}

	// Verify events
	updatedEvents := harness.GetEventsByTopic("tasks.updated")
	completedEvents := harness.GetEventsByTopic("tasks.completed")

	t.Logf("Update events: %d, Completed events: %d", len(updatedEvents), len(completedEvents))

	if len(completedEvents) < 1 {
		t.Error("Expected at least 1 tasks.completed event")
	}

	// Delete a task
	t.Log("Deleting a task...")

	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete,
		harness.BaseURL()+"/api/v1/tasks/lifecycle-3", nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Delete task failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Delete task: expected 200, got %d", resp.StatusCode)
	}

	// Verify delete event
	deletedEvents := harness.GetEventsByTopic("tasks.deleted")
	if len(deletedEvents) != 1 {
		t.Errorf("Expected 1 tasks.deleted event, got %d", len(deletedEvents))
	}

	// Summary
	allEvents := harness.GetPublishedEvents()
	t.Logf("Total events: %d", len(allEvents))
	t.Log("Task lifecycle test PASSED")
}

// TestKanbanE2E_ConcurrentOperations tests concurrent task operations
func TestKanbanE2E_ConcurrentOperations(t *testing.T) {
	harness := NewKanbanTestHarness(t)
	defer harness.Cleanup()

	const numTasks = 20
	var wg sync.WaitGroup
	errors := make(chan error, numTasks*2)

	// Concurrently create tasks
	t.Logf("Creating %d tasks concurrently...", numTasks)
	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			task := KanbanTask{
				ID:     fmt.Sprintf("concurrent-%d", n),
				Title:  fmt.Sprintf("Concurrent Task %d", n),
				Status: "todo",
				Type:   "task",
			}

			payload, _ := json.Marshal(task)
			resp, err := http.Post(harness.BaseURL()+"/api/v1/tasks", "application/json", bytes.NewReader(payload))
			if err != nil {
				errors <- fmt.Errorf("create task %d: %w", n, err)
				return
			}
			resp.Body.Close()

			if resp.StatusCode != http.StatusCreated {
				errors <- fmt.Errorf("create task %d: unexpected status %d", n, resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Error(err)
	}

	// Verify all events were published
	createdEvents := harness.GetEventsByTopic("tasks.created")
	if len(createdEvents) != numTasks {
		t.Errorf("Expected %d tasks.created events, got %d", numTasks, len(createdEvents))
	}

	// Verify all tasks are retrievable
	resp, _ := http.Get(harness.BaseURL() + "/api/v1/tasks")
	var tasksResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tasksResp)
	resp.Body.Close()

	count := int(tasksResp["count"].(float64))
	if count != numTasks {
		t.Errorf("Expected %d tasks, got %d", numTasks, count)
	}

	t.Logf("Concurrent operations test PASSED: %d tasks created", count)
}

// TestKanbanE2E_WotanEventPayload tests that Wotan events have correct payload
func TestKanbanE2E_WotanEventPayload(t *testing.T) {
	harness := NewKanbanTestHarness(t)
	defer harness.Cleanup()

	// Create a task
	task := &KanbanTask{
		ID:          "payload-test-1",
		Title:       "Payload Test Task",
		Description: "Testing event payload",
		Status:      "todo",
		Type:        "feature",
		Owner:       "Test Owner",
		Progress:    0,
	}

	taskPayload, _ := json.Marshal(task)
	resp, _ := http.Post(harness.BaseURL()+"/api/v1/tasks", "application/json", bytes.NewReader(taskPayload))
	resp.Body.Close()

	// Get the published event
	events := harness.GetEventsByTopic("tasks.created")
	if len(events) == 0 {
		t.Fatal("No tasks.created event found")
	}

	event := events[0]

	// Parse the event payload
	var eventTask KanbanTask
	if err := json.Unmarshal(event.Payload, &eventTask); err != nil {
		t.Fatalf("Failed to unmarshal event payload: %v", err)
	}

	// Verify payload contents
	if eventTask.ID != task.ID {
		t.Errorf("Event payload ID: expected %s, got %s", task.ID, eventTask.ID)
	}
	if eventTask.Title != task.Title {
		t.Errorf("Event payload Title: expected %s, got %s", task.Title, eventTask.Title)
	}
	if eventTask.Status != task.Status {
		t.Errorf("Event payload Status: expected %s, got %s", task.Status, eventTask.Status)
	}
	if eventTask.Owner != task.Owner {
		t.Errorf("Event payload Owner: expected %s, got %s", task.Owner, eventTask.Owner)
	}

	// Verify timestamps are set
	if eventTask.CreatedAt.IsZero() {
		t.Error("Event payload CreatedAt should not be zero")
	}
	if eventTask.UpdatedAt.IsZero() {
		t.Error("Event payload UpdatedAt should not be zero")
	}

	t.Log("Wotan event payload test PASSED")
}

// TestKanbanE2E_ErrorHandling tests error cases
func TestKanbanE2E_ErrorHandling(t *testing.T) {
	harness := NewKanbanTestHarness(t)
	defer harness.Cleanup()

	ctx := context.Background()
	client := &http.Client{}

	// Test: Get non-existent task
	t.Log("Testing: Get non-existent task...")
	resp, _ := http.Get(harness.BaseURL() + "/api/v1/tasks/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Get non-existent: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test: Update non-existent task
	t.Log("Testing: Update non-existent task...")
	payload, _ := json.Marshal(map[string]string{"title": "Test", "status": "todo"})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPut, harness.BaseURL()+"/api/v1/tasks/nonexistent", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Update non-existent: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test: Delete non-existent task
	t.Log("Testing: Delete non-existent task...")
	req, _ = http.NewRequestWithContext(ctx, http.MethodDelete, harness.BaseURL()+"/api/v1/tasks/nonexistent", nil)
	resp, _ = client.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Delete non-existent: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test: Create task with missing fields
	t.Log("Testing: Create task with missing fields...")
	payload, _ = json.Marshal(map[string]string{"title": "No ID or Status"})
	resp, _ = http.Post(harness.BaseURL()+"/api/v1/tasks", "application/json", bytes.NewReader(payload))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Create with missing fields: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Test: Create task with invalid JSON
	t.Log("Testing: Create task with invalid JSON...")
	resp, _ = http.Post(harness.BaseURL()+"/api/v1/tasks", "application/json", bytes.NewReader([]byte("invalid json{")))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Create with invalid JSON: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	t.Log("Error handling test PASSED")
}

// TestKanbanE2E_TimelinePersistence tests that timeline persists across operations
func TestKanbanE2E_TimelinePersistence(t *testing.T) {
	harness := NewKanbanTestHarness(t)
	defer harness.Cleanup()

	// Create a series of tasks representing a mini-timeline
	timelineItems := []KanbanTask{
		{ID: "tl-phase-1", Title: "[PHASE] Alpha Release", Status: "in-progress", Type: "epic", Progress: 50},
		{ID: "tl-ms-1", Title: "Core Services", Status: "done", Type: "milestone", Progress: 100},
		{ID: "tl-ms-2", Title: "Dashboard UI", Status: "in-progress", Type: "milestone", Progress: 30},
		{ID: "tl-task-1", Title: "API Implementation", Status: "done", Type: "task", Progress: 100},
		{ID: "tl-task-2", Title: "Frontend Components", Status: "todo", Type: "task", Progress: 0},
	}

	t.Log("Creating timeline items...")
	for _, item := range timelineItems {
		payload, _ := json.Marshal(item)
		resp, _ := http.Post(harness.BaseURL()+"/api/v1/tasks", "application/json", bytes.NewReader(payload))
		resp.Body.Close()
	}

	// Verify all were created
	resp, _ := http.Get(harness.BaseURL() + "/api/v1/tasks")
	var tasksResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&tasksResp)
	resp.Body.Close()

	count := int(tasksResp["count"].(float64))
	if count != len(timelineItems) {
		t.Errorf("Expected %d timeline items, got %d", len(timelineItems), count)
	}

	// Verify each item is retrievable
	for _, item := range timelineItems {
		resp, _ := http.Get(harness.BaseURL() + "/api/v1/tasks/" + item.ID)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Failed to retrieve timeline item %s: status %d", item.ID, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Verify events were published for all
	events := harness.GetPublishedEvents()
	if len(events) != len(timelineItems) {
		t.Errorf("Expected %d events, got %d", len(timelineItems), len(events))
	}

	t.Logf("Timeline persistence test PASSED: %d items persisted", count)
}

// ============================================================================
// WOTAN INTEGRATION TESTS
// ============================================================================

// TestKanbanE2E_WotanPublishCount verifies publish counts
func TestKanbanE2E_WotanPublishCount(t *testing.T) {
	harness := NewKanbanTestHarness(t)
	defer harness.Cleanup()

	ctx := context.Background()
	client := &http.Client{}

	// Perform a series of operations
	operations := []struct {
		method string
		path   string
		body   interface{}
		topic  string
	}{
		{http.MethodPost, "/api/v1/tasks", KanbanTask{ID: "count-1", Title: "Task 1", Status: "todo"}, "tasks.created"},
		{http.MethodPost, "/api/v1/tasks", KanbanTask{ID: "count-2", Title: "Task 2", Status: "todo"}, "tasks.created"},
		{http.MethodPut, "/api/v1/tasks/count-1", map[string]interface{}{"title": "Updated", "status": "in-progress"}, "tasks.updated"},
		{http.MethodPut, "/api/v1/tasks/count-1", map[string]interface{}{"title": "Completed", "status": "done", "progress": 100}, "tasks.completed"},
		{http.MethodDelete, "/api/v1/tasks/count-2", nil, "tasks.deleted"},
	}

	expectedCounts := map[string]int{
		"tasks.created":   2,
		"tasks.updated":   1,
		"tasks.completed": 1,
		"tasks.deleted":   1,
	}

	for _, op := range operations {
		var req *http.Request
		if op.body != nil {
			payload, _ := json.Marshal(op.body)
			req, _ = http.NewRequestWithContext(ctx, op.method, harness.BaseURL()+op.path, bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, _ = http.NewRequestWithContext(ctx, op.method, harness.BaseURL()+op.path, nil)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Operation %s %s failed: %v", op.method, op.path, err)
		}
		resp.Body.Close()
	}

	// Verify counts
	for topic, expectedCount := range expectedCounts {
		events := harness.GetEventsByTopic(topic)
		if len(events) != expectedCount {
			t.Errorf("Topic %s: expected %d events, got %d", topic, expectedCount, len(events))
		}
	}

	// Verify total
	total := harness.GetPublishedEvents()
	expectedTotal := 0
	for _, c := range expectedCounts {
		expectedTotal += c
	}
	if len(total) != expectedTotal {
		t.Errorf("Total events: expected %d, got %d", expectedTotal, len(total))
	}

	// Verify mock Wotan also recorded publishes
	if harness.MockWotan.GetPublishCount() == 0 {
		t.Error("Expected MockWotan to record publishes")
	}

	t.Logf("Wotan publish count test PASSED: %d total events", len(total))
}

// ============================================================================
// MOCK WOTAN CLIENT VERIFICATION
// ============================================================================

// TestKanbanE2E_MockWotanIntegration verifies the mock Wotan client works correctly
func TestKanbanE2E_MockWotanIntegration(t *testing.T) {
	mockClient := mock.NewMockClient(mock.WithAutoApprove())

	ctx := context.Background()

	// Test subscription
	sub, err := mockClient.Subscribe(ctx, "tasks.*", "test-app")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	if sub.Topic != "tasks.*" {
		t.Errorf("Expected topic tasks.*, got %s", sub.Topic)
	}

	// Subscribe to the specific topic before publishing (required by mock client)
	_, err = mockClient.Subscribe(ctx, "tasks.created", "test-publisher")
	if err != nil {
		t.Fatalf("Subscribe to tasks.created failed: %v", err)
	}

	// Test publish
	testPayload := []byte(`{"id":"test","title":"Test Task"}`)
	err = mockClient.Publish(ctx, "tasks.created", testPayload)
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Verify publish was recorded
	if mockClient.GetPublishCount() != 1 {
		t.Errorf("Expected 1 publish, got %d", mockClient.GetPublishCount())
	}

	// Test message streaming
	mockClient.Subscribe(ctx, "tasks.test", "test-streamer")
	mockClient.ApproveSubscription("tasks.test")

	msgCh, err := mockClient.StreamMessages(ctx, "tasks.test")
	if err != nil {
		t.Fatalf("StreamMessages failed: %v", err)
	}

	// Inject a test message
	mockClient.InjectMessage("tasks.test", string(testPayload))

	// Read the message
	select {
	case msg := <-msgCh:
		if msg.Topic != "tasks.test" {
			t.Errorf("Expected message topic tasks.test, got %s", msg.Topic)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for streamed message")
	}

	t.Log("Mock Wotan integration test PASSED")
}
