// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package micromanager

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/httputil"
)

// API provides HTTP handlers for the micromanager service
type API struct {
	store   *Store
	service *Service
}

// NewAPI creates a new API handler
func NewAPI(store *Store, service *Service) *API {
	return &API{
		store:   store,
		service: service,
	}
}

// TaskRequest represents incoming task creation/update request
type TaskRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Priority    int      `json:"priority"`
	Assignee    string   `json:"assignee,omitempty"`
	Owner       string   `json:"owner"`
	Tags        []string `json:"tags,omitempty"`
	DueDate     string   `json:"due_date,omitempty"`
}

// TaskResponse represents outgoing task response
type TaskResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	CompletedAt string `json:"completed_at,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	Owner       string `json:"owner"`
	Tags        []string `json:"tags,omitempty"`
	DueDate     string   `json:"due_date,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Health checks service health — standard Kingdom format
func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"service":   "micromanager",
		"status":    "healthy",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC(),
	})
}

// GetBacklog returns sprint backlog
func (a *API) GetBacklog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tasks := a.store.ListAll()
	if tasks == nil {
		tasks = []*Task{}
	}

	responses := make([]TaskResponse, len(tasks))
	for i, task := range tasks {
		responses[i] = taskToResponse(task)
	}

	httputil.WriteSuccess(w, http.StatusOK, map[string]interface{}{
		"tasks": responses,
		"count": len(responses),
	})
}

// CreateTask creates a new task
func (a *API) CreateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// SECURITY: Validate Content-Type for POST endpoints
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		a.error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// SECURITY: Enforce body size limit (1MB) to prevent denial-of-service
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
	if err != nil {
		a.error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		a.error(w, "request body cannot be empty", http.StatusBadRequest)
		return
	}

	var req TaskRequest
	if err := json.Unmarshal(body, &req); err != nil {
		a.error(w, "invalid JSON in request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Title == "" {
		a.error(w, "title is required", http.StatusBadRequest)
		return
	}
	if req.Priority < 1 || req.Priority > 5 {
		a.error(w, "priority must be between 1 and 5", http.StatusBadRequest)
		return
	}
	if req.Owner == "" {
		a.error(w, "owner is required", http.StatusBadRequest)
		return
	}

	// Generate ID (use service's ID generator)
	taskID := a.service.GenerateTaskID()

	// Create task
	task := NewTask(taskID, req.Title, req.Owner)
	task.Description = req.Description
	task.Priority = req.Priority
	task.Assignee = req.Assignee
	if len(req.Tags) > 0 {
		task.Tags = req.Tags
	}

	// Store task
	if err := a.store.Create(task); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to create task")
		a.error(w, "failed to create task", http.StatusInternalServerError)
		return
	}

	// Publish event
	go func() {
		if err := a.service.PublishTaskCreated(taskID, task); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("failed to publish task.created event")
		}
	}()

	httputil.WriteSuccess(w, http.StatusCreated, taskToResponse(task))
}

// UpdateTask updates an existing task
func (a *API) UpdateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		a.error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract task ID from path
	taskID := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	if taskID == "" || taskID == r.URL.Path {
		a.error(w, "task id is required", http.StatusBadRequest)
		return
	}

	// Get existing task
	task, err := a.store.Get(taskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			a.error(w, "task not found", http.StatusNotFound)
		} else {
			a.error(w, "failed to retrieve task", http.StatusInternalServerError)
		}
		return
	}

	// SECURITY: Enforce body size limit (1MB) to prevent denial-of-service
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
	if err != nil {
		a.error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		a.error(w, "request body cannot be empty", http.StatusBadRequest)
		return
	}

	var updates map[string]interface{}
	if err := json.Unmarshal(body, &updates); err != nil {
		a.error(w, "invalid JSON in request body", http.StatusBadRequest)
		return
	}

	// Apply updates
	if title, ok := updates["title"].(string); ok && title != "" {
		if err := task.UpdateTitle(title); err != nil {
			a.error(w, "failed to update title", http.StatusBadRequest)
			return
		}
	}

	if status, ok := updates["status"].(string); ok && status != "" {
		if err := task.UpdateStatus(TaskStatus(status)); err != nil {
			a.error(w, "invalid status value", http.StatusBadRequest)
			return
		}
	}

	if priority, ok := updates["priority"].(float64); ok {
		if err := task.UpdatePriority(int(priority)); err != nil {
			a.error(w, "invalid priority value", http.StatusBadRequest)
			return
		}
	}

	if description, ok := updates["description"].(string); ok {
		task.Description = description
	}

	if assignee, ok := updates["assignee"].(string); ok {
		task.Assignee = assignee
	}

	// Store updated task
	if err := a.store.Update(task); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("failed to update task")
		a.error(w, "failed to update task", http.StatusInternalServerError)
		return
	}

	// Publish event
	go func() {
		if err := a.service.PublishTaskUpdated(taskID, task); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("failed to publish task.updated event")
		}
	}()

	httputil.WriteSuccess(w, http.StatusOK, taskToResponse(task))
}

// GetSprintStatus returns sprint status summary
func (a *API) GetSprintStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pending, _ := a.store.ListByStatus(StatusPending)
	inProgress, _ := a.store.ListByStatus(StatusInProgress)
	completed, _ := a.store.ListByStatus(StatusCompleted)
	blocked, _ := a.store.ListByStatus(StatusBlocked)

	httputil.WriteSuccess(w, http.StatusOK, map[string]interface{}{
		"pending":     len(pending),
		"in_progress": len(inProgress),
		"completed":   len(completed),
		"blocked":     len(blocked),
		"total":       a.store.Count(),
	})
}

// error writes an error response using the shared httputil package.
func (a *API) error(w http.ResponseWriter, message string, code int) {
	httputil.WriteError(w, code, http.StatusText(code), message)
}

// taskToResponse converts a Task to TaskResponse
func taskToResponse(t *Task) TaskResponse {
	resp := TaskResponse{
		ID:          t.ID,
		Title:       t.Title,
		Description: t.Description,
		Status:      string(t.Status),
		Priority:    t.Priority,
		CreatedAt:   t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Assignee:    t.Assignee,
		Owner:       t.Owner,
		Tags:        t.Tags,
	}
	if t.CompletedAt != nil {
		resp.CompletedAt = t.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if t.DueDate != nil {
		resp.DueDate = t.DueDate.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}
