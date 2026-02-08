package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============================================================================
// HTTP HANDLER TESTS - POST /api/v1/timeline/tasks
// ============================================================================

func TestHandleCreateTask_HappyPath(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)

	server := &Server{
		taskManager: tm,
		sseClients:  make(map[chan []byte]bool),
	}

	// Initialize TaskManager
	ctx := context.Background()
	tm.Initialize(ctx)

	// Subscribe to the specific topic that CreateTask publishes to
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)

	task := createTestTask("http-create-1")
	payload, _ := json.Marshal(task)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	taskData, ok := response["task"].(map[string]interface{})
	if !ok {
		t.Fatal("expected task in response")
	}

	if taskData["id"] != task.ID {
		t.Errorf("expected task ID %s, got %v", task.ID, taskData["id"])
	}
}

func TestHandleCreateTask_InvalidJSON_Returns400(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline/tasks", bytes.NewReader([]byte("invalid json{")))
	w := httptest.NewRecorder()

	server.handleCreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleCreateTask_MissingID_Returns400(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	task := createTestTask("test")
	task.ID = ""
	payload, _ := json.Marshal(task)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleCreateTask_MissingTitle_Returns400(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	task := createTestTask("test")
	task.Title = ""
	payload, _ := json.Marshal(task)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleCreateTask_DuplicateID_Returns409(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)

	server := &Server{taskManager: tm}

	ctx := context.Background()
	tm.Initialize(ctx)

	// Subscribe to the specific topic that CreateTask publishes to
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)

	task := createTestTask("duplicate-http")
	tm.CreateTask(ctx, task)

	// Try to create again via HTTP
	payload, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateTask(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected status 409, got %d", w.Code)
	}
}

func TestHandleCreateTask_NoTaskManager_StandaloneMode(t *testing.T) {
	server := NewServer(Config{Port: "0"}) // Standalone mode, no TaskManager

	task := createTestTask("standalone-test")
	payload, _ := json.Marshal(task)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201 in standalone mode, got %d", w.Code)
	}
}

// ============================================================================
// HTTP HANDLER TESTS - PUT /api/v1/timeline/tasks
// ============================================================================

func TestHandleUpdateTask_HappyPath(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)

	server := &Server{taskManager: tm}

	ctx := context.Background()
	tm.Initialize(ctx)

	// Subscribe to the specific topics that CreateTask and UpdateTask publish to
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)
	mockClient.Subscribe(ctx, TopicTasksUpdated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksUpdated)

	// Create task first
	task := createTestTask("http-update-1")
	tm.CreateTask(ctx, task)

	// Update via HTTP
	task.Title = "Updated via HTTP"
	task.Progress = 75
	payload, _ := json.Marshal(task)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/timeline/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleUpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	taskData := response["task"].(map[string]interface{})
	if taskData["title"] != "Updated via HTTP" {
		t.Errorf("expected updated title, got %v", taskData["title"])
	}
}

func TestHandleUpdateTask_NotFound_Returns404(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	ctx := context.Background()
	tm.Initialize(ctx)

	task := createTestTask("nonexistent-http")
	payload, _ := json.Marshal(task)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/timeline/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleUpdateTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleUpdateTask_InvalidJSON_Returns400(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/timeline/tasks", bytes.NewReader([]byte("bad json")))
	w := httptest.NewRecorder()

	server.handleUpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// HTTP HANDLER TESTS - DELETE /api/v1/timeline/tasks
// ============================================================================

func TestHandleDeleteTask_HappyPath(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)

	server := &Server{taskManager: tm}

	ctx := context.Background()
	tm.Initialize(ctx)

	// Subscribe to the specific topics that CreateTask and DeleteTask publish to
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)
	mockClient.Subscribe(ctx, TopicTasksDeleted, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksDeleted)

	// Create task
	task := createTestTask("http-delete-1")
	tm.CreateTask(ctx, task)

	// Delete via HTTP
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/timeline/tasks?id=http-delete-1", nil)
	w := httptest.NewRecorder()

	server.handleDeleteTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	if deleted, ok := response["deleted"].(bool); !ok || !deleted {
		t.Error("expected deleted=true in response")
	}

	// Verify task is deleted
	_, err := tm.GetTask("http-delete-1")
	if err != ErrTaskNotFound {
		t.Error("expected task to be deleted")
	}
}

func TestHandleDeleteTask_MissingID_Returns400(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/timeline/tasks", nil) // No ID
	w := httptest.NewRecorder()

	server.handleDeleteTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleDeleteTask_NotFound_Returns404(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	ctx := context.Background()
	tm.Initialize(ctx)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/timeline/tasks?id=nonexistent", nil)
	w := httptest.NewRecorder()

	server.handleDeleteTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// ============================================================================
// INPUT VALIDATION TESTS
// ============================================================================

func TestValidateTaskInput_HappyPath(t *testing.T) {
	task := createTestTask("valid-input")

	if err := validateTaskInput(task); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateTaskInput_NilTask_ReturnsError(t *testing.T) {
	err := validateTaskInput(nil)
	if err == nil {
		t.Error("expected error for nil task")
	}
}

func TestValidateTaskInput_InvalidID_ReturnsError(t *testing.T) {
	testCases := []struct {
		name string
		id   string
	}{
		{"spaces", "task with spaces"},
		{"special chars", "task@#$%"},
		{"unicode", "task-タスク"},
		{"slash", "task/123"},
		{"backslash", "task\\123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := createTestTask(tc.id)
			err := validateTaskInput(task)
			if err == nil {
				t.Errorf("expected error for invalid ID: %s", tc.id)
			}
		})
	}
}

func TestValidateTaskInput_ValidID_Allowed(t *testing.T) {
	validIDs := []string{
		"task-1",
		"task_2",
		"TASK-3",
		"task123",
		"feature-new-api_v2",
	}

	for _, id := range validIDs {
		t.Run(id, func(t *testing.T) {
			task := createTestTask(id)
			if err := validateTaskInput(task); err != nil {
				t.Errorf("expected no error for valid ID %s, got %v", id, err)
			}
		})
	}
}

func TestValidateTaskInput_TitleTooLong_ReturnsError(t *testing.T) {
	task := createTestTask("test")
	task.Title = string(make([]byte, 201)) // 201 chars

	err := validateTaskInput(task)
	if err == nil {
		t.Error("expected error for title too long")
	}
}

func TestValidateTaskInput_DescriptionTooLong_ReturnsError(t *testing.T) {
	task := createTestTask("test")
	task.Description = string(make([]byte, 1001)) // 1001 chars

	err := validateTaskInput(task)
	if err == nil {
		t.Error("expected error for description too long")
	}
}

func TestValidateTaskInput_EmptyRequired_ReturnsError(t *testing.T) {
	testCases := []struct {
		name     string
		modifier func(*Task)
	}{
		{"empty ID", func(t *Task) { t.ID = "" }},
		{"empty title", func(t *Task) { t.Title = "" }},
		{"empty status", func(t *Task) { t.Status = "" }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task := createTestTask("test")
			tc.modifier(task)

			err := validateTaskInput(task)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

// ============================================================================
// ROUTER TESTS - handleTasks
// ============================================================================

func TestHandleTasks_RouterDispatch(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)

	server := &Server{taskManager: tm}

	ctx := context.Background()
	tm.Initialize(ctx)

	testCases := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{"GET", http.MethodGet, http.StatusOK},
		{"OPTIONS", http.MethodOptions, http.StatusMethodNotAllowed},
		{"HEAD", http.MethodHead, http.StatusMethodNotAllowed},
		{"PATCH", http.MethodPatch, http.StatusMethodNotAllowed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/v1/timeline/tasks", nil)
			w := httptest.NewRecorder()

			server.handleTasks(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("expected status %d for %s, got %d", tc.expectedStatus, tc.method, w.Code)
			}
		})
	}
}

// ============================================================================
// INTEGRATION TESTS - Full HTTP Flow
// ============================================================================

func TestFullHTTPFlow_CreateUpdateDelete(t *testing.T) {
	tm, mockClient := createTestTaskManager(t)
	mockClient.ApproveSubscription(TopicTasks)

	server := &Server{taskManager: tm, sseClients: make(map[chan []byte]bool)}

	ctx := context.Background()
	tm.Initialize(ctx)

	// Subscribe to the specific topics that CreateTask, UpdateTask, and DeleteTask publish to
	mockClient.Subscribe(ctx, TopicTasksCreated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksCreated)
	mockClient.Subscribe(ctx, TopicTasksUpdated, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksUpdated)
	mockClient.Subscribe(ctx, TopicTasksDeleted, "test-publisher")
	mockClient.ApproveSubscription(TopicTasksDeleted)

	// 1. Create task
	task := createTestTask("flow-test")
	payload, _ := json.Marshal(task)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.handleCreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create failed with status %d", w.Code)
	}

	// 2. Update task
	task.Title = "Updated in Flow"
	task.Progress = 50
	payload, _ = json.Marshal(task)

	req = httptest.NewRequest(http.MethodPut, "/api/v1/timeline/tasks", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	server.handleUpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update failed with status %d", w.Code)
	}

	// 3. Get task to verify update
	retrieved, err := tm.GetTask("flow-test")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if retrieved.Title != "Updated in Flow" {
		t.Errorf("expected updated title, got %s", retrieved.Title)
	}

	// 4. Delete task
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/timeline/tasks?id=flow-test", nil)
	w = httptest.NewRecorder()

	server.handleDeleteTask(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delete failed with status %d", w.Code)
	}

	// 5. Verify deletion
	_, err = tm.GetTask("flow-test")
	if err != ErrTaskNotFound {
		t.Error("expected task to be deleted")
	}
}

// ============================================================================
// EDGE CASES
// ============================================================================

func TestHandleGetTasks_FallbackMode_NoTaskManager(t *testing.T) {
	server := NewServer(Config{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/tasks", nil)
	w := httptest.NewRecorder()

	server.handleGetTasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 in fallback mode, got %d", w.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	// Should return hardcoded tasks
	count := int(response["count"].(float64))
	if count == 0 {
		t.Error("expected fallback to hardcoded tasks")
	}
}

func TestHandleGetTasks_WithTaskManager(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	server := &Server{taskManager: tm}

	ctx := context.Background()
	tm.Initialize(ctx)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/timeline/tasks", nil)
	w := httptest.NewRecorder()

	server.handleGetTasks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	json.NewDecoder(w.Body).Decode(&response)

	count := int(response["count"].(float64))
	if count == 0 {
		t.Error("expected tasks from TaskManager")
	}
}
