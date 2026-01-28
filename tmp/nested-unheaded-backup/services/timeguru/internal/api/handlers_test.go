package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unheaded/unheaded/services/timeguru/internal/timeline"
)

// ============================================================================
// MOCK STORE
// ============================================================================

type mockStore struct {
	timeline      *timeline.Timeline
	saveErr       error
	getErr        error
	updateErr     error
	getCalls      int
	saveCalls     int
	updateCalls   int
}

func (m *mockStore) SaveTimeline(ctx context.Context, tl *timeline.Timeline) error {
	m.saveCalls++
	if m.saveErr != nil {
		return m.saveErr
	}
	m.timeline = tl
	return nil
}

func (m *mockStore) GetTimeline(ctx context.Context) (*timeline.Timeline, error) {
	m.getCalls++
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.timeline, nil
}

func (m *mockStore) UpdateMilestone(ctx context.Context, id string, progress int, status string) error {
	m.updateCalls++
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

	if store.getCalls != 1 {
		t.Errorf("HandleGetTimeline() getCalls = %d, want 1", store.getCalls)
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
	store := &mockStore{getErr: &timeline.Timeline{}}
	// Using type as error to simulate internal error
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

	if store.updateCalls != 1 {
		t.Errorf("HandleUpdateMilestone() updateCalls = %d, want 1", store.updateCalls)
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

	if store.getCalls != numRequests {
		t.Errorf("ConcurrentRequests getCalls = %d, want %d", store.getCalls, numRequests)
	}
}
