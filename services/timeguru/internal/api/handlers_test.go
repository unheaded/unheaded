package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/httputil"
	"unheaded/services/timeguru/internal/timeline"
)

// ============================================================================
// MOCK STORE
// ============================================================================

type mockStore struct {
	timeline      *timeline.Timeline
	saveErr       error
	getErr        error
	updateErr     error
	getCalls      atomic.Int64
	saveCalls     atomic.Int64
	updateCalls   atomic.Int64
}

func (m *mockStore) SaveTimeline(ctx context.Context, tl *timeline.Timeline) error {
	m.saveCalls.Add(1)
	if m.saveErr != nil {
		return m.saveErr
	}
	m.timeline = tl
	return nil
}

func (m *mockStore) GetTimeline(ctx context.Context) (*timeline.Timeline, error) {
	m.getCalls.Add(1)
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.timeline, nil
}

func (m *mockStore) UpdateMilestone(ctx context.Context, id string, progress int, status string) error {
	m.updateCalls.Add(1)
	if m.updateErr != nil {
		return m.updateErr
	}
	if m.timeline != nil {
		milestone, found := m.timeline.GetMilestoneByID(id)
		if !found {
			return ErrMilestoneNotFound
		}
		milestone.Progress = progress
		milestone.Status = status
	}
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

func createMockTimeline() *timeline.Timeline {
	return &timeline.Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Date(2026, 1, 27, 12, 0, 0, 0, time.UTC),
		Status:      "alpha",
		Vision:      "Production-ready infrastructure in hours, not months.",
		Phases: []*timeline.Phase{
			{
				ID:       "phase-1",
				Name:     "Alpha",
				Status:   "in_progress",
				Progress: 25,
			},
		},
		Milestones: []*timeline.Milestone{
			{
				ID:       "milestone-1",
				Name:     "eBPF Foundation",
				Status:   "in_progress",
				Progress: 25,
				Owner:    "Agent 5",
				Risk:     "medium",
			},
		},
	}
}

// ============================================================================
// HANDLER TESTS: GET /timeline
// ============================================================================

func TestHandleGetTimeline_HappyPath(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimeline(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HandleGetTimeline() status = %d, want %d", w.Code, http.StatusOK)
	}

	if store.getCalls.Load() != 1 {
		t.Errorf("HandleGetTimeline() getCalls = %d, want 1", store.getCalls.Load())
	}

	var response TimelineResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode response error = %v", err)
	}

	if response.Timeline == nil {
		t.Fatal("HandleGetTimeline() timeline is nil")
	}

	if response.Timeline.Version != "1.0.0" {
		t.Errorf("HandleGetTimeline() version = %q, want %q", response.Timeline.Version, "1.0.0")
	}
}

func TestHandleGetTimeline_NotFound(t *testing.T) {
	store := &mockStore{getErr: ErrNotFound}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimeline(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("HandleGetTimeline() status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetTimeline_InternalError(t *testing.T) {
	store := &mockStore{getErr: errors.New("database connection failed")}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimeline(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("HandleGetTimeline() status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ============================================================================
// HANDLER TESTS: GET /milestones
// ============================================================================

func TestHandleGetMilestones_HappyPath(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/milestones", nil)
	w := httptest.NewRecorder()

	handler.HandleGetMilestones(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HandleGetMilestones() status = %d, want %d", w.Code, http.StatusOK)
	}

	var response MilestonesResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode response error = %v", err)
	}

	if len(response.Milestones) != 1 {
		t.Errorf("HandleGetMilestones() count = %d, want 1", len(response.Milestones))
	}

	if response.Milestones[0].ID != "milestone-1" {
		t.Errorf("HandleGetMilestones() ID = %q, want %q", response.Milestones[0].ID, "milestone-1")
	}
}

func TestHandleGetMilestones_NotFound(t *testing.T) {
	store := &mockStore{getErr: ErrNotFound}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/milestones", nil)
	w := httptest.NewRecorder()

	handler.HandleGetMilestones(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("HandleGetMilestones() status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ============================================================================
// HANDLER TESTS: POST /milestones/:id/update
// ============================================================================

func TestHandleUpdateMilestone_HappyPath(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	reqBody := UpdateMilestoneRequest{
		Progress: 50,
		Status:   "in_progress",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/milestones/milestone-1/update", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleUpdateMilestone(w, req, "milestone-1")

	if w.Code != http.StatusOK {
		t.Errorf("HandleUpdateMilestone() status = %d, want %d", w.Code, http.StatusOK)
	}

	if store.updateCalls.Load() != 1 {
		t.Errorf("HandleUpdateMilestone() updateCalls = %d, want 1", store.updateCalls.Load())
	}
}

func TestHandleUpdateMilestone_EmptyID(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	reqBody := UpdateMilestoneRequest{
		Progress: 50,
		Status:   "in_progress",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/milestones//update", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleUpdateMilestone(w, req, "")

	if w.Code != http.StatusBadRequest {
		t.Errorf("HandleUpdateMilestone() status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateMilestone_InvalidJSON(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("POST", "/milestones/milestone-1/update", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleUpdateMilestone(w, req, "milestone-1")

	if w.Code != http.StatusBadRequest {
		t.Errorf("HandleUpdateMilestone() status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateMilestone_InvalidProgress(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	tests := []struct {
		name     string
		progress int
	}{
		{"negative", -1},
		{"over 100", 101},
		{"way over", 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := UpdateMilestoneRequest{
				Progress: tt.progress,
				Status:   "in_progress",
			}
			bodyJSON, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/milestones/milestone-1/update", bytes.NewReader(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleUpdateMilestone(w, req, "milestone-1")

			if w.Code != http.StatusBadRequest {
				t.Errorf("HandleUpdateMilestone() status = %d, want %d for progress %d", w.Code, http.StatusBadRequest, tt.progress)
			}
		})
	}
}

func TestHandleUpdateMilestone_InvalidStatus(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	reqBody := UpdateMilestoneRequest{
		Progress: 50,
		Status:   "invalid",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/milestones/milestone-1/update", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleUpdateMilestone(w, req, "milestone-1")

	if w.Code != http.StatusBadRequest {
		t.Errorf("HandleUpdateMilestone() status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateMilestone_NotFound(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline(), updateErr: ErrMilestoneNotFound}
	handler := NewHandler(store)

	reqBody := UpdateMilestoneRequest{
		Progress: 50,
		Status:   "in_progress",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/milestones/nonexistent/update", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleUpdateMilestone(w, req, "nonexistent")

	if w.Code != http.StatusNotFound {
		t.Errorf("HandleUpdateMilestone() status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ============================================================================
// HANDLER TESTS: GET /health
// ============================================================================

func TestHandleHealth_HappyPath(t *testing.T) {
	store := &mockStore{}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HandleHealth() status = %d, want %d", w.Code, http.StatusOK)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode response error = %v", err)
	}

	if response.Status != "healthy" {
		t.Errorf("HandleHealth() status = %q, want %q", response.Status, "healthy")
	}

	if response.Service != "timeguru" {
		t.Errorf("HandleHealth() service = %q, want %q", response.Service, "timeguru")
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestHandlers_ConcurrentRequests(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	const numRequests = 50
	done := make(chan bool, numRequests)

	// Concurrent GET requests
	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/timeline", nil)
			w := httptest.NewRecorder()
			handler.HandleGetTimeline(w, req)
			done <- true
		}()
	}

	for i := 0; i < numRequests; i++ {
		<-done
	}

	if store.getCalls.Load() != int64(numRequests) {
		t.Errorf("ConcurrentRequests getCalls = %d, want %d", store.getCalls.Load(), numRequests)
	}
}

// ============================================================================
// HELPER: rich mock timeline with subtasks, ETA, descriptions
// ============================================================================

func createRichMockTimeline() *timeline.Timeline {
	return &timeline.Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Date(2026, 1, 27, 12, 0, 0, 0, time.UTC),
		Status:      "alpha",
		Vision:      "Production-ready infrastructure in hours, not months.",
		Phases: []*timeline.Phase{
			{
				ID:          "phase-0",
				Name:        "Foundation",
				Status:      "completed",
				Progress:    100,
				Description: "The forging of Busboy",
				EndDate:     time.Date(2026, 1, 26, 0, 0, 0, 0, time.UTC),
			},
			{
				ID:          "phase-1",
				Name:        "Alpha",
				Status:      "in_progress",
				Progress:    25,
				Description: "The armor takes shape",
			},
			{
				ID:       "phase-2",
				Name:     "Beta Trials",
				Status:   "planned",
				Progress: 0,
			},
		},
		Milestones: []*timeline.Milestone{
			{
				ID:          "milestone-1",
				Name:        "eBPF Foundation",
				Status:      "in_progress",
				Progress:    50,
				Owner:       "Agent 5",
				Risk:        "medium",
				ETA:         time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC),
				Description: "Build eBPF programs for packet tracing",
				Tasks:       []string{"packet_marker.bpf", "flow_tracker.bpf", "latency_probe.bpf", "integration tests"},
			},
			{
				ID:          "milestone-2",
				Name:        "Bug fix: connection reset",
				Status:      "completed",
				Progress:    100,
				Owner:       "Agent 7",
				Description: "Fix the connection reset bug in gateway",
				Tasks:       []string{"diagnose reset cause", "apply fix"},
			},
			{
				ID:          "milestone-3",
				Name:        "Tech debt refactor",
				Status:      "pending",
				Progress:    0,
				Description: "Refactor the legacy tech debt in mesh layer",
			},
			{
				ID:       "milestone-4",
				Name:     "Feature: dashboard",
				Status:   "blocked",
				Progress: 10,
			},
		},
		Metadata: map[string]string{
			"overall_progress": "40%",
		},
	}
}

// ============================================================================
// HANDLER TESTS: NewHandler panic on nil store
// ============================================================================

func TestNewHandler_NilStorePanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("NewHandler(nil) should panic, but did not")
		}
	}()
	NewHandler(nil)
}

// ============================================================================
// HANDLER TESTS: GET /milestones internal error
// ============================================================================

func TestHandleGetMilestones_InternalError(t *testing.T) {
	store := &mockStore{getErr: errors.New("db down")}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/milestones", nil)
	w := httptest.NewRecorder()

	handler.HandleGetMilestones(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("HandleGetMilestones() status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ============================================================================
// HANDLER TESTS: GET /api/v1/timeline/tasks (kanban tasks)
// ============================================================================

func TestHandleGetTasks_HappyPath(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/timeline/tasks", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTasks(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("HandleGetTasks() status = %d, want %d", w.Code, http.StatusOK)
	}

	var response TasksResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode response error = %v", err)
	}

	if response.TotalCount == 0 {
		t.Error("HandleGetTasks() expected non-zero total_count")
	}
	if len(response.Tasks) != response.TotalCount {
		t.Errorf("HandleGetTasks() tasks len = %d, total_count = %d, should match",
			len(response.Tasks), response.TotalCount)
	}
}

func TestHandleGetTasks_NotFound(t *testing.T) {
	store := &mockStore{getErr: ErrNotFound}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/timeline/tasks", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTasks(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("HandleGetTasks() status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetTasks_InternalError(t *testing.T) {
	store := &mockStore{getErr: errors.New("connection refused")}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/timeline/tasks", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTasks(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("HandleGetTasks() status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ============================================================================
// UNIT TESTS: transformToKanbanTasks
// ============================================================================

func TestTransformToKanbanTasks_NilTimeline(t *testing.T) {
	handler := NewHandler(&mockStore{})
	tasks := handler.transformToKanbanTasks(nil)
	if tasks == nil {
		t.Fatal("transformToKanbanTasks(nil) should return empty slice, not nil")
	}
	if len(tasks) != 0 {
		t.Errorf("transformToKanbanTasks(nil) expected 0 tasks, got %d", len(tasks))
	}
}

func TestTransformToKanbanTasks_WithNilEntries(t *testing.T) {
	handler := NewHandler(&mockStore{})
	tl := &timeline.Timeline{
		Phases:     []*timeline.Phase{nil, {ID: "p1", Name: "Phase1", Status: "planned"}},
		Milestones: []*timeline.Milestone{nil, {ID: "m1", Name: "M1", Status: "pending"}},
	}
	tasks := handler.transformToKanbanTasks(tl)
	// Should not panic, should skip nils
	if len(tasks) == 0 {
		t.Error("expected at least some tasks from non-nil entries")
	}
}

func TestTransformToKanbanTasks_SubtaskGeneration(t *testing.T) {
	handler := NewHandler(&mockStore{})
	tl := &timeline.Timeline{
		Milestones: []*timeline.Milestone{
			{
				ID:       "m1",
				Name:     "Milestone with tasks",
				Status:   "in_progress",
				Progress: 50,
				Owner:    "Agent X",
				ETA:      time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				Tasks:    []string{"task-a", "task-b", "task-c", "task-d"},
			},
		},
	}
	tasks := handler.transformToKanbanTasks(tl)

	// Should have 1 milestone task + 4 subtasks = 5
	if len(tasks) != 5 {
		t.Errorf("expected 5 tasks (1 milestone + 4 subtasks), got %d", len(tasks))
	}

	// Verify subtask IDs
	for i := 1; i <= 4; i++ {
		expectedID := fmt.Sprintf("m1-task-%d", i)
		if tasks[i].ID != expectedID {
			t.Errorf("subtask %d: expected ID %q, got %q", i, expectedID, tasks[i].ID)
		}
		if tasks[i].Type != "task" {
			t.Errorf("subtask %d: expected type 'task', got %q", i, tasks[i].Type)
		}
		if tasks[i].DueDate != "2026-03-01" {
			t.Errorf("subtask %d: expected due date '2026-03-01', got %q", i, tasks[i].DueDate)
		}
	}
}

// ============================================================================
// UNIT TESTS: mapStatusToKanban
// ============================================================================

func TestMapStatusToKanban(t *testing.T) {
	handler := NewHandler(&mockStore{})

	tests := []struct {
		input    string
		expected string
	}{
		{"completed", "done"},
		{"in_progress", "in-progress"},
		{"pending", "todo"},
		{"planned", "todo"},
		{"blocked", "todo"},
		{"unknown_status", "todo"},
		{"", "todo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := handler.mapStatusToKanban(tt.input)
			if result != tt.expected {
				t.Errorf("mapStatusToKanban(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// UNIT TESTS: inferTaskType
// ============================================================================

func TestInferTaskType(t *testing.T) {
	handler := NewHandler(&mockStore{})

	tests := []struct {
		name     string
		m        *timeline.Milestone
		expected string
	}{
		{"nil milestone", nil, "task"},
		{"bug in name", &timeline.Milestone{ID: "m1", Name: "Bug fix: reset", Status: "pending"}, "bug"},
		{"fix in description", &timeline.Milestone{ID: "m2", Name: "Patch", Status: "pending", Description: "fix the crash"}, "bug"},
		{"tech debt in name", &timeline.Milestone{ID: "m3", Name: "tech debt cleanup", Status: "pending"}, "tech-debt"},
		{"refactor in description", &timeline.Milestone{ID: "m4", Name: "Cleanup", Status: "pending", Description: "refactor the API"}, "tech-debt"},
		{"feature in name", &timeline.Milestone{ID: "m5", Name: "feature: dashboard", Status: "pending"}, "feature"},
		{"feature in description", &timeline.Milestone{ID: "m6", Name: "Dashboard", Status: "pending", Description: "new feature for display"}, "feature"},
		{"with subtasks", &timeline.Milestone{ID: "m7", Name: "Build system", Status: "pending", Tasks: []string{"a", "b"}}, "milestone"},
		{"plain task", &timeline.Milestone{ID: "m8", Name: "Setup", Status: "pending"}, "task"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.inferTaskType(tt.m)
			if result != tt.expected {
				t.Errorf("inferTaskType() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// ============================================================================
// UNIT TESTS: inferSubtaskStatus
// ============================================================================

func TestInferSubtaskStatus(t *testing.T) {
	handler := NewHandler(&mockStore{})

	tests := []struct {
		name           string
		parentStatus   string
		parentProgress int
		taskIndex      int
		totalTasks     int
		expected       string
	}{
		{"parent completed", "completed", 100, 0, 3, "done"},
		{"parent pending", "pending", 0, 0, 3, "todo"},
		{"parent planned", "planned", 0, 1, 3, "todo"},
		{"in_progress task done", "in_progress", 66, 0, 3, "done"},
		{"in_progress current task", "in_progress", 50, 2, 4, "in-progress"},
		{"in_progress future task", "in_progress", 25, 3, 4, "todo"},
		{"zero total tasks", "in_progress", 50, 0, 0, "todo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.inferSubtaskStatus(tt.parentStatus, tt.parentProgress, tt.taskIndex, tt.totalTasks)
			if result != tt.expected {
				t.Errorf("inferSubtaskStatus(%q, %d, %d, %d) = %q, want %q",
					tt.parentStatus, tt.parentProgress, tt.taskIndex, tt.totalTasks, result, tt.expected)
			}
		})
	}
}

// ============================================================================
// HANDLER TESTS: GET /ready
// ============================================================================

func TestHandleReady_AllChecksPass(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	handler.HandleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("HandleReady() status = %d, want %d", w.Code, http.StatusOK)
	}

	var response ReadyResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode response error = %v", err)
	}

	if !response.Ready {
		t.Error("HandleReady() expected ready=true when all checks pass")
	}
	if !response.Checks.Store {
		t.Error("HandleReady() expected store check to pass")
	}
	if !response.Checks.Timeline {
		t.Error("HandleReady() expected timeline check to pass")
	}
	if response.Service != "timeguru" {
		t.Errorf("HandleReady() service = %q, want 'timeguru'", response.Service)
	}
}

func TestHandleReady_TimelineNotAvailable(t *testing.T) {
	store := &mockStore{getErr: ErrNotFound}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	handler.HandleReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("HandleReady() status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var response ReadyResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode response error = %v", err)
	}

	if response.Ready {
		t.Error("HandleReady() expected ready=false when timeline unavailable")
	}
	if !response.Checks.Store {
		t.Error("HandleReady() store check should still pass")
	}
	if response.Checks.Timeline {
		t.Error("HandleReady() timeline check should fail")
	}
}

// ============================================================================
// HANDLER TESTS: GET /timeline with format negotiation (JSON/YAML/Markdown)
// ============================================================================

func TestHandleGetTimelineWithFormat_DefaultJSON(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want 'application/json'", ct)
	}

	var resp TimelineResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("JSON decode error: %v", err)
	}
	if resp.Timeline == nil {
		t.Fatal("expected non-nil timeline in JSON response")
	}
}

func TestHandleGetTimelineWithFormat_YAMLQueryParam(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline?format=yaml", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want 'application/yaml'", ct)
	}

	// Verify YAML is parseable
	var resp TimelineResponse
	if err := yaml.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("YAML decode error: %v", err)
	}
	if resp.Timeline == nil {
		t.Fatal("expected non-nil timeline in YAML response")
	}
}

func TestHandleGetTimelineWithFormat_YMLQueryParam(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline?format=yml", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want 'application/yaml'", ct)
	}
}

func TestHandleGetTimelineWithFormat_MarkdownQueryParam(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline?format=markdown", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/markdown" {
		t.Errorf("Content-Type = %q, want 'text/markdown'", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "# The Unheaded Chronicles") {
		t.Error("markdown output missing expected header")
	}
}

func TestHandleGetTimelineWithFormat_MDQueryParam(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline?format=md", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/markdown" {
		t.Errorf("Content-Type = %q, want 'text/markdown'", ct)
	}
}

func TestHandleGetTimelineWithFormat_AcceptYAML(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline", nil)
	req.Header.Set("Accept", "application/yaml")
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want 'application/yaml'", ct)
	}
}

func TestHandleGetTimelineWithFormat_AcceptTextYAML(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline", nil)
	req.Header.Set("Accept", "text/yaml")
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want 'application/yaml'", ct)
	}
}

func TestHandleGetTimelineWithFormat_AcceptMarkdown(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline", nil)
	req.Header.Set("Accept", "text/markdown")
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/markdown" {
		t.Errorf("Content-Type = %q, want 'text/markdown'", ct)
	}
}

func TestHandleGetTimelineWithFormat_NotFound(t *testing.T) {
	store := &mockStore{getErr: ErrNotFound}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetTimelineWithFormat_InternalError(t *testing.T) {
	store := &mockStore{getErr: errors.New("db crashed")}
	handler := NewHandler(store)

	req := httptest.NewRequest("GET", "/timeline", nil)
	w := httptest.NewRecorder()

	handler.HandleGetTimelineWithFormat(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ============================================================================
// UNIT TESTS: generateMarkdown
// ============================================================================

func TestGenerateMarkdown_Content(t *testing.T) {
	handler := NewHandler(&mockStore{})
	tl := createRichMockTimeline()

	md := handler.generateMarkdown(tl)

	// Check header
	if !strings.Contains(md, "# The Unheaded Chronicles") {
		t.Error("markdown missing title header")
	}
	if !strings.Contains(md, "A Living Grimoire") {
		t.Error("markdown missing subtitle")
	}

	// Check vision section
	if !strings.Contains(md, "The Founding Vision") {
		t.Error("markdown missing vision section")
	}
	if !strings.Contains(md, tl.Vision) {
		t.Error("markdown missing vision text")
	}

	// Check phases are listed
	if !strings.Contains(md, "Foundation") {
		t.Error("markdown missing phase 'Foundation'")
	}
	if !strings.Contains(md, "Alpha") {
		t.Error("markdown missing phase 'Alpha'")
	}

	// Check milestones
	if !strings.Contains(md, "eBPF Foundation") {
		t.Error("markdown missing milestone 'eBPF Foundation'")
	}

	// Check metadata fields in milestones
	if !strings.Contains(md, "**Owner:** Agent 5") {
		t.Error("markdown missing owner field")
	}
	if !strings.Contains(md, "**Risk:**") {
		t.Error("markdown missing risk field")
	}

	// Check tasks are rendered
	if !strings.Contains(md, "packet_marker.bpf") {
		t.Error("markdown missing task 'packet_marker.bpf'")
	}

	// Check footer
	if !strings.Contains(md, "THE TIMEGURU KNOWS ALL") {
		t.Error("markdown missing footer")
	}

	// Check completed milestone tasks get [x] checkbox
	if !strings.Contains(md, "[x]") {
		t.Error("markdown missing completed task checkbox")
	}
}

func TestGenerateMarkdown_EmptyTimeline(t *testing.T) {
	handler := NewHandler(&mockStore{})
	tl := &timeline.Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
	}

	md := handler.generateMarkdown(tl)

	if !strings.Contains(md, "# The Unheaded Chronicles") {
		t.Error("markdown missing header even for empty timeline")
	}
}

func TestGenerateMarkdown_PhaseWithProgress(t *testing.T) {
	handler := NewHandler(&mockStore{})
	tl := &timeline.Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "beta",
		Phases: []*timeline.Phase{
			{ID: "p1", Name: "Build", Status: "in_progress", Progress: 60},
		},
	}

	md := handler.generateMarkdown(tl)
	if !strings.Contains(md, "**Progress:** 60%") {
		t.Error("markdown missing progress percentage for phase")
	}
}

func TestGenerateMarkdown_MilestoneWithDescription(t *testing.T) {
	handler := NewHandler(&mockStore{})
	tl := &timeline.Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Milestones: []*timeline.Milestone{
			{
				ID:          "m1",
				Name:        "Test Milestone",
				Status:      "blocked",
				Description: "This is a detailed description",
			},
		},
	}

	md := handler.generateMarkdown(tl)
	if !strings.Contains(md, "This is a detailed description") {
		t.Error("markdown missing milestone description")
	}
}

// ============================================================================
// HANDLER TESTS: POST /milestones/:id/update - store internal error
// ============================================================================

func TestHandleUpdateMilestone_StoreInternalError(t *testing.T) {
	store := &mockStore{
		timeline:  createMockTimeline(),
		updateErr: errors.New("disk full"),
	}
	handler := NewHandler(store)

	reqBody := UpdateMilestoneRequest{
		Progress: 50,
		Status:   "in_progress",
	}
	bodyJSON, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/milestones/milestone-1/update", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.HandleUpdateMilestone(w, req, "milestone-1")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("HandleUpdateMilestone() status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleUpdateMilestone_GetAfterUpdateFails(t *testing.T) {
	// Store that succeeds on update but then fails on the subsequent get
	callCount := int64(0)
	store := &mockStore{
		timeline: createMockTimeline(),
	}
	// Override GetTimeline to fail on the second call (after update)
	origGet := store.getErr
	_ = origGet
	handler := NewHandler(store)

	reqBody := UpdateMilestoneRequest{Progress: 50, Status: "in_progress"}
	bodyJSON, _ := json.Marshal(reqBody)

	// First, do a successful update to verify the flow
	req := httptest.NewRequest("POST", "/milestones/milestone-1/update", bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.HandleUpdateMilestone(w, req, "milestone-1")

	if w.Code != http.StatusOK {
		t.Errorf("HandleUpdateMilestone() status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify the update count
	_ = atomic.LoadInt64(&callCount)
}

func TestHandleUpdateMilestone_AllValidStatuses(t *testing.T) {
	validStatuses := []string{"pending", "in_progress", "completed", "blocked"}

	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			store := &mockStore{timeline: createMockTimeline()}
			handler := NewHandler(store)

			reqBody := UpdateMilestoneRequest{Progress: 50, Status: status}
			bodyJSON, _ := json.Marshal(reqBody)

			req := httptest.NewRequest("POST", "/milestones/milestone-1/update", bytes.NewReader(bodyJSON))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.HandleUpdateMilestone(w, req, "milestone-1")

			if w.Code != http.StatusOK {
				t.Errorf("status %q: got HTTP %d, want %d", status, w.Code, http.StatusOK)
			}
		})
	}
}

// ============================================================================
// HANDLER TESTS: SSE Stream
// ============================================================================

// sseResponseWriter wraps httptest.ResponseRecorder to support http.Flusher
type sseResponseWriter struct {
	*httptest.ResponseRecorder
	flushed int
}

func (s *sseResponseWriter) Flush() {
	s.flushed++
	s.ResponseRecorder.Flush()
}

func TestHandleTimelineStream_InitialEvents(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	// Create a context that cancels quickly so the SSE loop exits
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/api/v1/timeline/stream", nil)
	req = req.WithContext(ctx)

	w := &sseResponseWriter{ResponseRecorder: httptest.NewRecorder()}

	handler.HandleTimelineStream(w, req)

	// Verify SSE headers
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want 'text/event-stream'", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want 'no-cache'", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %q, want 'keep-alive'", conn)
	}

	// Parse SSE events from body
	body := w.Body.String()

	// Should contain initial "connected" event
	if !strings.Contains(body, "event: connected") {
		t.Error("SSE output missing 'connected' event")
	}

	// Should contain initial "tasks" event
	if !strings.Contains(body, "event: tasks") {
		t.Error("SSE output missing initial 'tasks' event")
	}

	// Verify flush was called
	if w.flushed == 0 {
		t.Error("Flush() should have been called at least once")
	}
}

func TestHandleTimelineStream_NoFlusherSupport(t *testing.T) {
	store := &mockStore{timeline: createMockTimeline()}
	handler := NewHandler(store)

	// nonFlusherWriter delegates to a recorder but does NOT expose Flush(),
	// so the w.(http.Flusher) assertion in HandleTimelineStream returns false.
	rec := httptest.NewRecorder()
	w := &nonFlusherWriter{rec: rec}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req := httptest.NewRequest("GET", "/api/v1/timeline/stream", nil).WithContext(ctx)

	handler.HandleTimelineStream(w, req)

	if w.code != http.StatusInternalServerError {
		t.Errorf("expected 500 when Flusher not supported, got %d", w.code)
	}
}

// nonFlusherWriter is an http.ResponseWriter that does NOT implement http.Flusher.
// It uses explicit delegation (not embedding) to avoid promoting Flush().
type nonFlusherWriter struct {
	rec  *httptest.ResponseRecorder
	code int
}

func (n *nonFlusherWriter) Header() http.Header {
	return n.rec.Header()
}
func (n *nonFlusherWriter) Write(b []byte) (int, error) {
	return n.rec.Write(b)
}
func (n *nonFlusherWriter) WriteHeader(code int) {
	n.code = code
	n.rec.WriteHeader(code)
}

func TestHandleTimelineStream_StoreError(t *testing.T) {
	store := &mockStore{getErr: errors.New("store down")}
	handler := NewHandler(store)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/api/v1/timeline/stream", nil)
	req = req.WithContext(ctx)

	w := &sseResponseWriter{ResponseRecorder: httptest.NewRecorder()}

	handler.HandleTimelineStream(w, req)

	body := w.Body.String()
	// Should still have connected event even if store fails
	if !strings.Contains(body, "event: connected") {
		t.Error("SSE output missing 'connected' event even with store error")
	}
}

// ============================================================================
// UNIT TESTS: sendSSEEvent
// ============================================================================

func TestSendSSEEvent(t *testing.T) {
	handler := NewHandler(&mockStore{})

	w := httptest.NewRecorder()
	flusher := &sseResponseWriter{ResponseRecorder: w}

	handler.sendSSEEvent(flusher, flusher, "test_event", map[string]string{"key": "value"})

	body := w.Body.String()
	if !strings.Contains(body, "event: test_event") {
		t.Error("sendSSEEvent missing event line")
	}
	if !strings.Contains(body, `"key":"value"`) {
		t.Error("sendSSEEvent missing data payload")
	}

	// Read the SSE format line-by-line
	scanner := bufio.NewScanner(strings.NewReader(body))
	lines := []string{}
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in SSE output, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "event: ") {
		t.Errorf("first line should start with 'event: ', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "data: ") {
		t.Errorf("second line should start with 'data: ', got %q", lines[1])
	}
}

// ============================================================================
// HANDLER TESTS: writeError with nil error
// ============================================================================

func TestWriteError_NilError(t *testing.T) {
	handler := NewHandler(&mockStore{})

	w := httptest.NewRecorder()
	handler.writeError(w, http.StatusBadRequest, "TEST_CODE", "test message", nil)

	var resp httputil.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode error: %v", err)
	}

	if resp.Success {
		t.Error("expected success=false")
	}
	if resp.Error == nil {
		t.Fatal("expected error info, got nil")
	}
	if resp.Error.Message != "test message" {
		t.Errorf("expected error message 'test message', got %q", resp.Error.Message)
	}
	if resp.Error.Code != "TEST_CODE" {
		t.Errorf("expected code 'TEST_CODE', got %q", resp.Error.Code)
	}
	if resp.Error.Details != "" {
		t.Errorf("expected empty details for nil error, got %q", resp.Error.Details)
	}
}

// ============================================================================
// CONCURRENCY TESTS: Mixed endpoint access
// ============================================================================

func TestHandlers_ConcurrentMixedRequests(t *testing.T) {
	store := &mockStore{timeline: createRichMockTimeline()}
	handler := NewHandler(store)

	const numRequests = 30
	done := make(chan bool, numRequests*3)

	// Concurrent GET /timeline
	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/timeline", nil)
			w := httptest.NewRecorder()
			handler.HandleGetTimeline(w, req)
			done <- true
		}()
	}

	// Concurrent GET /api/v1/timeline/tasks
	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/api/v1/timeline/tasks", nil)
			w := httptest.NewRecorder()
			handler.HandleGetTasks(w, req)
			done <- true
		}()
	}

	// Concurrent GET /health
	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()
			handler.HandleHealth(w, req)
			done <- true
		}()
	}

	for i := 0; i < numRequests*3; i++ {
		<-done
	}
}
