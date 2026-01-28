package micromanager

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if int(resp["count"].(float64)) != 0 {
		t.Errorf("count = %d, want 0", int(resp["count"].(float64)))
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

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if int(resp["count"].(float64)) != 2 {
		t.Errorf("count = %d, want 2", int(resp["count"].(float64)))
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
	w := httptest.NewRecorder()
	api.CreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp TaskResponse
	json.NewDecoder(w.Body).Decode(&resp)
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

	var resp TaskResponse
	json.NewDecoder(w.Body).Decode(&resp)
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

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if int(resp["pending"].(float64)) != 1 {
		t.Errorf("pending = %d, want 1", int(resp["pending"].(float64)))
	}
	if int(resp["in_progress"].(float64)) != 1 {
		t.Errorf("in_progress = %d, want 1", int(resp["in_progress"].(float64)))
	}
	if int(resp["completed"].(float64)) != 1 {
		t.Errorf("completed = %d, want 1", int(resp["completed"].(float64)))
	}
}
