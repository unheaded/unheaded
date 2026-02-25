package micromanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/httputil"
)

// decodeTaskResponse decodes a successful httputil.Response envelope and extracts the
// TaskResponse from its Data field. It fails the test if the envelope indicates failure
// or if the data cannot be converted to TaskResponse.
func decodeTaskResponse(t *testing.T, w *httptest.ResponseRecorder) TaskResponse {
	t.Helper()
	var envelope httputil.Response
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("expected success=true, got false")
	}
	// Re-marshal Data and unmarshal into TaskResponse
	raw, err := json.Marshal(envelope.Data)
	if err != nil {
		t.Fatalf("failed to re-marshal envelope data: %v", err)
	}
	var resp TaskResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("failed to unmarshal TaskResponse from envelope data: %v", err)
	}
	return resp
}

// decodeEnvelopeData decodes a successful httputil.Response envelope and returns
// the Data field as a map[string]interface{}.
func decodeEnvelopeData(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var envelope httputil.Response
	if err := json.NewDecoder(w.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}
	if !envelope.Success {
		t.Fatalf("expected success=true, got false")
	}
	data, ok := envelope.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected envelope data to be map[string]interface{}, got %T", envelope.Data)
	}
	return data
}

// TestHealth returns 200 OK
func TestHealth(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	api.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "healthy" {
		t.Errorf("status = %s, want healthy", resp["status"])
	}
}

// TestGetBacklog_Empty returns empty list
func TestGetBacklog_Empty(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("GET", "/api/v1/backlog", nil)
	w := httptest.NewRecorder()
	api.GetBacklog(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	data := decodeEnvelopeData(t, w)
	if int(data["count"].(float64)) != 0 {
		t.Errorf("count = %d, want 0", int(data["count"].(float64)))
	}
}

// TestGetBacklog_WithTasks returns task list
func TestGetBacklog_WithTasks(t *testing.T) {
	store := NewStore()
	task1 := NewTask("task-1", "First", "owner")
	task2 := NewTask("task-2", "Second", "owner")
	store.Create(task1)
	store.Create(task2)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("GET", "/api/v1/backlog", nil)
	w := httptest.NewRecorder()
	api.GetBacklog(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	data := decodeEnvelopeData(t, w)
	if int(data["count"].(float64)) != 2 {
		t.Errorf("count = %d, want 2", int(data["count"].(float64)))
	}
}

// TestGetBacklog_InvalidMethod rejects non-GET
func TestGetBacklog_InvalidMethod(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("POST", "/api/v1/backlog", nil)
	w := httptest.NewRecorder()
	api.GetBacklog(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestCreateTask_HappyPath creates a task
func TestCreateTask_HappyPath(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	reqBody := TaskRequest{
		Title:    "New task",
		Priority: 3,
		Owner:    "muck",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusCreated)
	}

	resp := decodeTaskResponse(t, w)
	if resp.Title != "New task" {
		t.Errorf("Title = %s, want New task", resp.Title)
	}
	if resp.Owner != "muck" {
		t.Errorf("Owner = %s, want muck", resp.Owner)
	}
}

// TestCreateTask_EmptyBody rejects empty body
func TestCreateTask_EmptyBody(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader([]byte("")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestCreateTask_EmptyTitle rejects empty title
func TestCreateTask_EmptyTitle(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	reqBody := TaskRequest{
		Title:    "",
		Priority: 3,
		Owner:    "owner",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestCreateTask_InvalidPriority rejects out-of-range priority
func TestCreateTask_InvalidPriority(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	reqBody := TaskRequest{
		Title:    "Task",
		Priority: 10,
		Owner:    "owner",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestCreateTask_MissingOwner rejects missing owner
func TestCreateTask_MissingOwner(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	reqBody := TaskRequest{
		Title:    "Task",
		Priority: 3,
		Owner:    "",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestCreateTask_InvalidMethod rejects non-POST
func TestCreateTask_InvalidMethod(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("GET", "/api/v1/tasks", nil)
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestUpdateTask_HappyPath updates a task
func TestUpdateTask_HappyPath(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Original", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"title":    "Updated",
		"priority": 5,
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	resp := decodeTaskResponse(t, w)
	if resp.Title != "Updated" {
		t.Errorf("Title = %s, want Updated", resp.Title)
	}
	if resp.Priority != 5 {
		t.Errorf("Priority = %d, want 5", resp.Priority)
	}
}

// TestUpdateTask_NotFound returns 404
func TestUpdateTask_NotFound(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"title": "Updated",
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/nonexistent", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestUpdateTask_InvalidStatus rejects invalid status
func TestUpdateTask_InvalidStatus(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Task", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"status": "invalid",
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestUpdateTask_InvalidMethod rejects non-PUT
func TestUpdateTask_InvalidMethod(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("GET", "/api/v1/tasks/task-1", nil)
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestUpdateTask_EmptyTaskID rejects empty task ID in path
func TestUpdateTask_EmptyTaskID(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	body, _ := json.Marshal(map[string]interface{}{"title": "Updated"})
	req := httptest.NewRequest("PUT", "/api/v1/tasks/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestUpdateTask_EmptyBody rejects empty request body
func TestUpdateTask_EmptyBody(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Task", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader([]byte("")))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestUpdateTask_InvalidJSON rejects invalid JSON body
func TestUpdateTask_InvalidJSON(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Task", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader([]byte("{invalid")))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestUpdateTask_Description updates description field
func TestUpdateTask_Description(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Task", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"description": "New description",
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	resp := decodeTaskResponse(t, w)
	if resp.Description != "New description" {
		t.Errorf("Description = %s, want 'New description'", resp.Description)
	}
}

// TestUpdateTask_Assignee updates assignee field
func TestUpdateTask_Assignee(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Task", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"assignee": "alice",
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	resp := decodeTaskResponse(t, w)
	if resp.Assignee != "alice" {
		t.Errorf("Assignee = %s, want 'alice'", resp.Assignee)
	}
}

// TestUpdateTask_StatusToCompleted updates status to completed
func TestUpdateTask_StatusToCompleted(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Task", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"status": "completed",
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	resp := decodeTaskResponse(t, w)
	if resp.Status != "completed" {
		t.Errorf("Status = %s, want 'completed'", resp.Status)
	}
	if resp.CompletedAt == "" {
		t.Error("CompletedAt should be set when status is completed")
	}
}

// TestUpdateTask_StatusToBlocked updates status to blocked (blocker detection)
func TestUpdateTask_StatusToBlocked(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Task", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"status": "blocked",
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	resp := decodeTaskResponse(t, w)
	if resp.Status != "blocked" {
		t.Errorf("Status = %s, want 'blocked'", resp.Status)
	}
}

// TestUpdateTask_InvalidPriority rejects invalid priority via update
func TestUpdateTask_InvalidPriority(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Task", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"priority": 99,
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestCreateTask_WithTagsAndDescription creates task with optional fields
func TestCreateTask_WithTagsAndDescription(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	reqBody := TaskRequest{
		Title:       "Feature task",
		Description: "A detailed description",
		Priority:    4,
		Owner:       "muck",
		Assignee:    "bob",
		Tags:        []string{"backend", "urgent"},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusCreated)
	}

	resp := decodeTaskResponse(t, w)
	if resp.Description != "A detailed description" {
		t.Errorf("Description = %s, want 'A detailed description'", resp.Description)
	}
	if resp.Assignee != "bob" {
		t.Errorf("Assignee = %s, want 'bob'", resp.Assignee)
	}
	if len(resp.Tags) != 2 {
		t.Errorf("Tags length = %d, want 2", len(resp.Tags))
	}
	if resp.Priority != 4 {
		t.Errorf("Priority = %d, want 4", resp.Priority)
	}
}

// TestCreateTask_InvalidJSON rejects invalid JSON body
func TestCreateTask_InvalidJSON(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader([]byte("{invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestCreateTask_PriorityZero rejects priority of 0
func TestCreateTask_PriorityZero(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	reqBody := TaskRequest{
		Title:    "Task",
		Priority: 0,
		Owner:    "owner",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestGetSprintStatus_InvalidMethod rejects non-GET
func TestGetSprintStatus_InvalidMethod(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("POST", "/api/v1/sprint/status", nil)
	w := httptest.NewRecorder()
	api.GetSprintStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestGetSprintStatus_WithBlockedTasks includes blocked count
func TestGetSprintStatus_WithBlockedTasks(t *testing.T) {
	store := NewStore()
	task1 := NewTask("task-1", "Blocked", "owner")
	task1.Status = StatusBlocked
	task2 := NewTask("task-2", "Pending", "owner")
	store.Create(task1)
	store.Create(task2)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("GET", "/api/v1/sprint/status", nil)
	w := httptest.NewRecorder()
	api.GetSprintStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	data := decodeEnvelopeData(t, w)
	if int(data["blocked"].(float64)) != 1 {
		t.Errorf("blocked = %d, want 1", int(data["blocked"].(float64)))
	}
	if int(data["total"].(float64)) != 2 {
		t.Errorf("total = %d, want 2", int(data["total"].(float64)))
	}
}

// TestTaskToResponse_WithDueDate verifies DueDate conversion
func TestTaskToResponse_WithDueDate(t *testing.T) {
	task := NewTask("task-1", "Task", "owner")
	due := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	task.DueDate = &due

	resp := taskToResponse(task)
	if resp.DueDate == "" {
		t.Fatal("DueDate should be set in response")
	}
	if resp.DueDate != "2026-03-15T12:00:00Z" {
		t.Errorf("DueDate = %s, want 2026-03-15T12:00:00Z", resp.DueDate)
	}
}

// TestTaskToResponse_WithCompletedAt verifies CompletedAt conversion
func TestTaskToResponse_WithCompletedAt(t *testing.T) {
	task := NewTask("task-1", "Task", "owner")
	task.Status = StatusCompleted
	completed := time.Date(2026, 2, 1, 10, 30, 0, 0, time.UTC)
	task.CompletedAt = &completed

	resp := taskToResponse(task)
	if resp.CompletedAt == "" {
		t.Fatal("CompletedAt should be set in response")
	}
	if resp.CompletedAt != "2026-02-01T10:30:00Z" {
		t.Errorf("CompletedAt = %s, want 2026-02-01T10:30:00Z", resp.CompletedAt)
	}
}

// TestTaskToResponse_WithoutOptionalFields verifies empty optionals
func TestTaskToResponse_WithoutOptionalFields(t *testing.T) {
	task := NewTask("task-1", "Task", "owner")
	resp := taskToResponse(task)

	if resp.CompletedAt != "" {
		t.Errorf("CompletedAt = %s, want empty", resp.CompletedAt)
	}
	if resp.DueDate != "" {
		t.Errorf("DueDate = %s, want empty", resp.DueDate)
	}
	if resp.Assignee != "" {
		t.Errorf("Assignee = %s, want empty", resp.Assignee)
	}
}

// TestTaskToResponse_AllFields verifies full field mapping
func TestTaskToResponse_AllFields(t *testing.T) {
	task := NewTask("task-1", "My Task", "alice")
	task.Description = "Some description"
	task.Assignee = "bob"
	task.Tags = []string{"tag1", "tag2"}
	task.Priority = 5
	task.Status = StatusInProgress

	resp := taskToResponse(task)
	if resp.ID != "task-1" {
		t.Errorf("ID = %s, want task-1", resp.ID)
	}
	if resp.Title != "My Task" {
		t.Errorf("Title = %s, want My Task", resp.Title)
	}
	if resp.Description != "Some description" {
		t.Errorf("Description = %s, want Some description", resp.Description)
	}
	if resp.Status != "in_progress" {
		t.Errorf("Status = %s, want in_progress", resp.Status)
	}
	if resp.Priority != 5 {
		t.Errorf("Priority = %d, want 5", resp.Priority)
	}
	if resp.Owner != "alice" {
		t.Errorf("Owner = %s, want alice", resp.Owner)
	}
	if resp.Assignee != "bob" {
		t.Errorf("Assignee = %s, want bob", resp.Assignee)
	}
	if len(resp.Tags) != 2 {
		t.Errorf("Tags length = %d, want 2", len(resp.Tags))
	}
	if resp.CreatedAt == "" {
		t.Error("CreatedAt should be set")
	}
	if resp.UpdatedAt == "" {
		t.Error("UpdatedAt should be set")
	}
}

// TestUpdateTask_TitleOnly updates only the title
func TestUpdateTask_TitleOnly(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Original", "owner")
	task.Priority = 2
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"title": "Updated Title",
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	resp := decodeTaskResponse(t, w)
	if resp.Title != "Updated Title" {
		t.Errorf("Title = %s, want 'Updated Title'", resp.Title)
	}
	// Priority should remain unchanged
	if resp.Priority != 2 {
		t.Errorf("Priority = %d, want 2 (unchanged)", resp.Priority)
	}
}

// TestUpdateTask_NoTaskIDInPath rejects path without task ID segment
func TestUpdateTask_NoTaskIDInPath(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	body, _ := json.Marshal(map[string]interface{}{"title": "Updated"})
	// Path that doesn't contain /api/v1/tasks/ prefix
	req := httptest.NewRequest("PUT", "/other/path", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestUpdateTask_AllFieldsAtOnce updates all mutable fields in one request
func TestUpdateTask_AllFieldsAtOnce(t *testing.T) {
	store := NewStore()
	task := NewTask("task-1", "Original", "owner")
	store.Create(task)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	updates := map[string]interface{}{
		"title":       "Updated Title",
		"description": "Updated Desc",
		"assignee":    "charlie",
		"status":      "in_progress",
		"priority":    4,
	}
	body, _ := json.Marshal(updates)

	req := httptest.NewRequest("PUT", "/api/v1/tasks/task-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	resp := decodeTaskResponse(t, w)
	if resp.Title != "Updated Title" {
		t.Errorf("Title = %s, want 'Updated Title'", resp.Title)
	}
	if resp.Description != "Updated Desc" {
		t.Errorf("Description = %s, want 'Updated Desc'", resp.Description)
	}
	if resp.Assignee != "charlie" {
		t.Errorf("Assignee = %s, want 'charlie'", resp.Assignee)
	}
	if resp.Status != "in_progress" {
		t.Errorf("Status = %s, want 'in_progress'", resp.Status)
	}
	if resp.Priority != 4 {
		t.Errorf("Priority = %d, want 4", resp.Priority)
	}
}

// TestPrioritySorting_SprintStatus verifies tasks with different priorities appear in sprint status
func TestPrioritySorting_SprintStatus(t *testing.T) {
	store := NewStore()

	// Create tasks with various priorities
	for i := 1; i <= 5; i++ {
		task := NewTask(fmt.Sprintf("task-%d", i), fmt.Sprintf("Priority %d task", i), "owner")
		task.Priority = i
		store.Create(task)
	}

	service := NewService(store, nil)
	api := NewAPI(store, service)

	// Verify through backlog endpoint
	req := httptest.NewRequest("GET", "/api/v1/backlog", nil)
	w := httptest.NewRecorder()
	api.GetBacklog(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	data := decodeEnvelopeData(t, w)
	if int(data["count"].(float64)) != 5 {
		t.Errorf("count = %d, want 5", int(data["count"].(float64)))
	}

	// Verify tasks list contains all priorities
	tasks := data["tasks"].([]interface{})
	priorities := make(map[int]bool)
	for _, tk := range tasks {
		taskMap := tk.(map[string]interface{})
		p := int(taskMap["priority"].(float64))
		priorities[p] = true
	}
	for i := 1; i <= 5; i++ {
		if !priorities[i] {
			t.Errorf("Missing priority %d in backlog", i)
		}
	}
}

// TestBlockerDetection_FullWorkflow tests creating, blocking, and unblocking a task
func TestBlockerDetection_FullWorkflow(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	// Create task
	reqBody := TaskRequest{
		Title:    "Critical feature",
		Priority: 5,
		Owner:    "alice",
	}
	body, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	api.CreateTask(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("Create status = %d, want %d", createW.Code, http.StatusCreated)
	}

	created := decodeTaskResponse(t, createW)
	taskID := created.ID

	// Block it
	blockBody, _ := json.Marshal(map[string]interface{}{"status": "blocked"})
	blockReq := httptest.NewRequest("PUT", "/api/v1/tasks/"+taskID, bytes.NewReader(blockBody))
	blockW := httptest.NewRecorder()
	api.UpdateTask(blockW, blockReq)

	if blockW.Code != http.StatusOK {
		t.Fatalf("Block status = %d, want %d", blockW.Code, http.StatusOK)
	}

	// Verify sprint shows 1 blocked
	statusReq := httptest.NewRequest("GET", "/api/v1/sprint/status", nil)
	statusW := httptest.NewRecorder()
	api.GetSprintStatus(statusW, statusReq)

	statusData := decodeEnvelopeData(t, statusW)
	if int(statusData["blocked"].(float64)) != 1 {
		t.Errorf("blocked = %d, want 1", int(statusData["blocked"].(float64)))
	}

	// Unblock it (move to in_progress)
	unblockBody, _ := json.Marshal(map[string]interface{}{"status": "in_progress"})
	unblockReq := httptest.NewRequest("PUT", "/api/v1/tasks/"+taskID, bytes.NewReader(unblockBody))
	unblockW := httptest.NewRecorder()
	api.UpdateTask(unblockW, unblockReq)

	if unblockW.Code != http.StatusOK {
		t.Fatalf("Unblock status = %d, want %d", unblockW.Code, http.StatusOK)
	}

	// Verify sprint shows 0 blocked
	statusReq2 := httptest.NewRequest("GET", "/api/v1/sprint/status", nil)
	statusW2 := httptest.NewRecorder()
	api.GetSprintStatus(statusW2, statusReq2)

	statusData2 := decodeEnvelopeData(t, statusW2)
	if int(statusData2["blocked"].(float64)) != 0 {
		t.Errorf("blocked after unblock = %d, want 0", int(statusData2["blocked"].(float64)))
	}
	if int(statusData2["in_progress"].(float64)) != 1 {
		t.Errorf("in_progress after unblock = %d, want 1", int(statusData2["in_progress"].(float64)))
	}
}

// TestQAGate_CompletionWorkflow tests the full task lifecycle (QA gate: pending -> in_progress -> completed)
func TestQAGate_CompletionWorkflow(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)
	api := NewAPI(store, service)

	// Create task
	reqBody := TaskRequest{
		Title:    "QA gate feature",
		Priority: 3,
		Owner:    "qa-team",
	}
	body, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest("POST", "/api/v1/tasks", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	api.CreateTask(createW, createReq)

	created := decodeTaskResponse(t, createW)
	taskID := created.ID

	// Move to in_progress
	body1, _ := json.Marshal(map[string]interface{}{"status": "in_progress"})
	req1 := httptest.NewRequest("PUT", "/api/v1/tasks/"+taskID, bytes.NewReader(body1))
	w1 := httptest.NewRecorder()
	api.UpdateTask(w1, req1)

	inProgress := decodeTaskResponse(t, w1)
	if inProgress.Status != "in_progress" {
		t.Errorf("Status = %s, want in_progress", inProgress.Status)
	}

	// Complete task
	body2, _ := json.Marshal(map[string]interface{}{"status": "completed"})
	req2 := httptest.NewRequest("PUT", "/api/v1/tasks/"+taskID, bytes.NewReader(body2))
	w2 := httptest.NewRecorder()
	api.UpdateTask(w2, req2)

	completedResp := decodeTaskResponse(t, w2)
	if completedResp.Status != "completed" {
		t.Errorf("Status = %s, want completed", completedResp.Status)
	}
	if completedResp.CompletedAt == "" {
		t.Error("CompletedAt should be set on completion")
	}

	// Verify sprint status
	statusReq := httptest.NewRequest("GET", "/api/v1/sprint/status", nil)
	statusW := httptest.NewRecorder()
	api.GetSprintStatus(statusW, statusReq)

	statusData := decodeEnvelopeData(t, statusW)
	if int(statusData["completed"].(float64)) != 1 {
		t.Errorf("completed = %d, want 1", int(statusData["completed"].(float64)))
	}
	if int(statusData["pending"].(float64)) != 0 {
		t.Errorf("pending = %d, want 0", int(statusData["pending"].(float64)))
	}
}

// TestProgressTracking_MultipleTaskLifecycles tracks progress across multiple tasks
func TestProgressTracking_MultipleTaskLifecycles(t *testing.T) {
	store := NewStore()
	service := NewService(store, nil)

	// Create 10 tasks
	for i := 0; i < 10; i++ {
		task := NewTask(fmt.Sprintf("track-%d", i), fmt.Sprintf("Task %d", i), "owner")
		task.Priority = (i % 5) + 1
		store.Create(task)
	}

	// Move some to various states
	for i := 0; i < 3; i++ {
		task, _ := store.Get(fmt.Sprintf("track-%d", i))
		task.UpdateStatus(StatusCompleted)
		store.Update(task)
	}
	for i := 3; i < 6; i++ {
		task, _ := store.Get(fmt.Sprintf("track-%d", i))
		task.UpdateStatus(StatusInProgress)
		store.Update(task)
	}
	for i := 6; i < 8; i++ {
		task, _ := store.Get(fmt.Sprintf("track-%d", i))
		task.UpdateStatus(StatusBlocked)
		store.Update(task)
	}
	// 8,9 stay pending

	// Check sprint status
	api := NewAPI(store, service)
	req := httptest.NewRequest("GET", "/api/v1/sprint/status", nil)
	w := httptest.NewRecorder()
	api.GetSprintStatus(w, req)

	statusData := decodeEnvelopeData(t, w)

	if int(statusData["completed"].(float64)) != 3 {
		t.Errorf("completed = %d, want 3", int(statusData["completed"].(float64)))
	}
	if int(statusData["in_progress"].(float64)) != 3 {
		t.Errorf("in_progress = %d, want 3", int(statusData["in_progress"].(float64)))
	}
	if int(statusData["blocked"].(float64)) != 2 {
		t.Errorf("blocked = %d, want 2", int(statusData["blocked"].(float64)))
	}
	if int(statusData["pending"].(float64)) != 2 {
		t.Errorf("pending = %d, want 2", int(statusData["pending"].(float64)))
	}
	if int(statusData["total"].(float64)) != 10 {
		t.Errorf("total = %d, want 10", int(statusData["total"].(float64)))
	}
}

// TestGetSprintStatus returns sprint summary
func TestGetSprintStatus(t *testing.T) {
	store := NewStore()
	task1 := NewTask("task-1", "Pending", "owner")
	task2 := NewTask("task-2", "InProgress", "owner")
	task2.Status = StatusInProgress
	task3 := NewTask("task-3", "Completed", "owner")
	task3.Status = StatusCompleted

	store.Create(task1)
	store.Create(task2)
	store.Create(task3)

	service := NewService(store, nil)
	api := NewAPI(store, service)

	req := httptest.NewRequest("GET", "/api/v1/sprint/status", nil)
	w := httptest.NewRecorder()
	api.GetSprintStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	data := decodeEnvelopeData(t, w)
	if int(data["pending"].(float64)) != 1 {
		t.Errorf("pending = %d, want 1", int(data["pending"].(float64)))
	}
	if int(data["in_progress"].(float64)) != 1 {
		t.Errorf("in_progress = %d, want 1", int(data["in_progress"].(float64)))
	}
	if int(data["completed"].(float64)) != 1 {
		t.Errorf("completed = %d, want 1", int(data["completed"].(float64)))
	}
}
