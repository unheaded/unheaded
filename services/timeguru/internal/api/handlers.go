package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"

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

// HandleGetTimelineWithFormat handles GET /timeline with format negotiation
// Supports: JSON (default), YAML, Markdown via Accept header or ?format= query
func (h *Handler) HandleGetTimelineWithFormat(w http.ResponseWriter, r *http.Request) {
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

	// Determine output format
	format := r.URL.Query().Get("format")
	if format == "" {
		// Check Accept header
		accept := r.Header.Get("Accept")
		switch {
		case strings.Contains(accept, "application/yaml"), strings.Contains(accept, "text/yaml"):
			format = "yaml"
		case strings.Contains(accept, "text/markdown"):
			format = "markdown"
		default:
			format = "json"
		}
	}

	switch strings.ToLower(format) {
	case "yaml", "yml":
		h.writeYAML(w, http.StatusOK, TimelineResponse{Timeline: tl})
	case "markdown", "md":
		h.writeMarkdown(w, http.StatusOK, tl)
	default:
		h.writeJSON(w, http.StatusOK, TimelineResponse{Timeline: tl})
	}
}

// ============================================================================
// MULTI-FORMAT OUTPUT - THE ORACLE'S TONGUES
// ============================================================================

// writeYAML writes YAML response
func (h *Handler) writeYAML(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(status)

	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	if err := encoder.Encode(data); err != nil {
		_ = err // Headers already sent
	}
}

// writeMarkdown generates and writes Markdown timeline
func (h *Handler) writeMarkdown(w http.ResponseWriter, status int, tl *timeline.Timeline) {
	w.Header().Set("Content-Type", "text/markdown")
	w.WriteHeader(status)

	md := h.generateMarkdown(tl)
	w.Write([]byte(md))
}

// generateMarkdown converts Timeline to Markdown format
func (h *Handler) generateMarkdown(tl *timeline.Timeline) string {
	var sb strings.Builder

	sb.WriteString("# The Unheaded Chronicles\n\n")
	sb.WriteString("## A Living Grimoire of the Kingdom's Journey\n\n")
	sb.WriteString(fmt.Sprintf("**STATUS:** %s\n", strings.Title(tl.Status)))
	sb.WriteString(fmt.Sprintf("**LAST UPDATED:** %s\n\n", tl.LastUpdated.Format("January 2, 2006")))
	sb.WriteString("---\n\n")

	if tl.Vision != "" {
		sb.WriteString("## The Founding Vision\n\n")
		sb.WriteString(fmt.Sprintf("*%s*\n\n", tl.Vision))
		sb.WriteString("---\n\n")
	}

	// Phases
	for i, phase := range tl.Phases {
		statusEmoji := "📋"
		switch phase.Status {
		case "completed":
			statusEmoji = "✅"
		case "in_progress":
			statusEmoji = "🚀"
		case "blocked":
			statusEmoji = "🚫"
		}

		sb.WriteString(fmt.Sprintf("### Age %d: %s (%s %s)\n\n",
			i, phase.Name, statusEmoji, strings.ToUpper(phase.Status)))

		if phase.Description != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", phase.Description))
		}

		if phase.Progress > 0 {
			sb.WriteString(fmt.Sprintf("**Progress:** %d%%\n\n", phase.Progress))
		}
	}

	// Milestones
	if len(tl.Milestones) > 0 {
		sb.WriteString("## Milestones\n\n")

		for _, m := range tl.Milestones {
			statusEmoji := "⏳"
			switch m.Status {
			case "completed":
				statusEmoji = "✅"
			case "in_progress":
				statusEmoji = "🔄"
			case "blocked":
				statusEmoji = "🚫"
			}

			sb.WriteString(fmt.Sprintf("### %s %s\n\n", statusEmoji, m.Name))

			if m.Description != "" {
				sb.WriteString(fmt.Sprintf("%s\n\n", m.Description))
			}

			// Metadata
			if !m.ETA.IsZero() {
				sb.WriteString(fmt.Sprintf("**ETA:** %s\n", m.ETA.Format("Jan 2, 2006")))
			}
			if m.Owner != "" {
				sb.WriteString(fmt.Sprintf("**Owner:** %s\n", m.Owner))
			}
			if m.Risk != "" {
				sb.WriteString(fmt.Sprintf("**Risk:** %s\n", strings.Title(m.Risk)))
			}
			sb.WriteString(fmt.Sprintf("**Progress:** %d%%\n", m.Progress))
			sb.WriteString(fmt.Sprintf("**Status:** %s\n\n", m.Status))

			// Tasks
			if len(m.Tasks) > 0 {
				sb.WriteString("**Tasks:**\n")
				for _, task := range m.Tasks {
					// Assume task is complete if progress is 100
					checkbox := "[ ]"
					if m.Progress == 100 {
						checkbox = "[x]"
					}
					sb.WriteString(fmt.Sprintf("- %s %s\n", checkbox, task))
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("**THE TIMEGURU KNOWS ALL.**\n")
	sb.WriteString("**THE CIRCLE NEVER BREAKS.**\n\n")
	sb.WriteString(fmt.Sprintf("*Generated: %s*\n", tl.LastUpdated.Format("2006-01-02 15:04:05")))

	return sb.String()
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
