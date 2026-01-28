package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/unheaded/unheaded/services/timeguru/internal/timeline"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrNotFound          = errors.New("not found")
	ErrMilestoneNotFound = errors.New("milestone not found")
	ErrInvalidRequest    = errors.New("invalid request")
)

// ============================================================================
// INTERFACES
// ============================================================================

// Store interface for timeline persistence
type Store interface {
	GetTimeline(ctx context.Context) (*timeline.Timeline, error)
	SaveTimeline(ctx context.Context, tl *timeline.Timeline) error
	UpdateMilestone(ctx context.Context, id string, progress int, status string) error
	Close() error
}

// ============================================================================
// HANDLER
// ============================================================================

// Handler provides HTTP handlers for the timeguru API
type Handler struct {
	store Store
}

// NewHandler creates a new Handler with defensive validation
func NewHandler(store Store) *Handler {
	if store == nil {
		panic("store cannot be nil")
	}
	return &Handler{store: store}
}

// ============================================================================
// REQUEST/RESPONSE TYPES
// ============================================================================

// TimelineResponse wraps timeline data
type TimelineResponse struct {
	Timeline *timeline.Timeline `json:"timeline"`
}

// MilestonesResponse wraps milestone list
type MilestonesResponse struct {
	Milestones []*timeline.Milestone `json:"milestones"`
}

// UpdateMilestoneRequest represents milestone update payload
type UpdateMilestoneRequest struct {
	Progress int    `json:"progress"`
	Status   string `json:"status"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

// ErrorResponse represents error payload
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details string `json:"details,omitempty"`
}

// ============================================================================
// HANDLERS
// ============================================================================

// HandleGetTimeline handles GET /timeline
func (h *Handler) HandleGetTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tl, err := h.store.GetTimeline(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "NOT_FOUND", "timeline not found", err)
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve timeline", err)
		return
	}

	h.writeJSON(w, http.StatusOK, TimelineResponse{Timeline: tl})
}

// HandleGetMilestones handles GET /milestones
func (h *Handler) HandleGetMilestones(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tl, err := h.store.GetTimeline(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "NOT_FOUND", "timeline not found", err)
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve milestones", err)
		return
	}

	h.writeJSON(w, http.StatusOK, MilestonesResponse{Milestones: tl.Milestones})
}

// HandleUpdateMilestone handles POST /milestones/:id/update
func (h *Handler) HandleUpdateMilestone(w http.ResponseWriter, r *http.Request, milestoneID string) {
	// Defensive: validate ID
	if milestoneID == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "milestone ID cannot be empty", nil)
		return
	}

	// Parse request
	var req UpdateMilestoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", err)
		return
	}
	defer r.Body.Close()

	// Defensive: validate progress
	if req.Progress < 0 || req.Progress > 100 {
		h.writeError(w, http.StatusBadRequest, "INVALID_PROGRESS",
			fmt.Sprintf("progress must be 0-100, got %d", req.Progress), nil)
		return
	}

	// Defensive: validate status
	switch req.Status {
	case "pending", "in_progress", "completed", "blocked":
		// valid
	default:
		h.writeError(w, http.StatusBadRequest, "INVALID_STATUS",
			fmt.Sprintf("invalid status %q", req.Status), nil)
		return
	}

	ctx := r.Context()

	// Update milestone
	err := h.store.UpdateMilestone(ctx, milestoneID, req.Progress, req.Status)
	if err != nil {
		if errors.Is(err, ErrMilestoneNotFound) {
			h.writeError(w, http.StatusNotFound, "NOT_FOUND",
				fmt.Sprintf("milestone %q not found", milestoneID), err)
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update milestone", err)
		return
	}

	// Return updated milestone
	tl, err := h.store.GetTimeline(ctx)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve updated timeline", err)
		return
	}

	milestone, found := tl.GetMilestoneByID(milestoneID)
	if !found {
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "milestone disappeared after update", nil)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"milestone": milestone,
		"message":   "milestone updated successfully",
	})
}

// HandleHealth handles GET /health
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "healthy",
		Service: "timeguru",
		Version: "1.0.0",
	})
}

// ============================================================================
// HELPERS
// ============================================================================

// writeJSON writes JSON response with defensive error handling
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Can't write error response at this point (headers already sent)
		// In production, log this error
		_ = err
	}
}

// writeError writes error response with defensive error handling
func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	errResp := ErrorResponse{
		Error: message,
		Code:  code,
	}

	if err != nil {
		errResp.Details = err.Error()
	}

	if encodeErr := json.NewEncoder(w).Encode(errResp); encodeErr != nil {
		// Can't write error response at this point (headers already sent)
		// In production, log this error
		_ = encodeErr
	}
}
