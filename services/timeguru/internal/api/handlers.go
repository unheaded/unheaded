// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	"unheaded/pkg/httputil"
	tsync "unheaded/services/timeguru/internal/sync"
	"unheaded/services/timeguru/internal/timeline"
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
	store  Store
	syncer *tsync.Syncer // optional file syncer, nil if not configured
}

// NewHandler creates a new Handler with defensive validation
func NewHandler(store Store) *Handler {
	if store == nil {
		panic("store cannot be nil")
	}
	return &Handler{store: store}
}

// SetSyncer attaches a file syncer for auto-sync on timeline changes.
func (h *Handler) SetSyncer(s *tsync.Syncer) {
	h.syncer = s
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

// HealthResponse represents health check response (standard Kingdom format)
type HealthResponse struct {
	Service   string    `json:"service"`
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
}

// KanbanTask represents a task in kanban format for the frontend
type KanbanTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"` // todo, in-progress, done
	Type        string `json:"type"`   // milestone, feature, task, bug, tech-debt
	Owner       string `json:"owner,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	Progress    int    `json:"progress,omitempty"`
	Phase       string `json:"phase,omitempty"`
}

// TasksResponse wraps kanban tasks list
type TasksResponse struct {
	Tasks      []*KanbanTask `json:"tasks"`
	TotalCount int           `json:"total_count"`
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

// HandleGetTasks handles GET /api/v1/timeline/tasks
// Transforms milestones and phases into kanban-friendly task format
func (h *Handler) HandleGetTasks(w http.ResponseWriter, r *http.Request) {
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

	tasks := h.transformToKanbanTasks(tl)

	h.writeJSON(w, http.StatusOK, TasksResponse{
		Tasks:      tasks,
		TotalCount: len(tasks),
	})
}

// transformToKanbanTasks converts timeline milestones and phases to kanban tasks
func (h *Handler) transformToKanbanTasks(tl *timeline.Timeline) []*KanbanTask {
	if tl == nil {
		return []*KanbanTask{}
	}

	var tasks []*KanbanTask

	// Build phase ID to name map for reference
	phaseMap := make(map[string]string)
	for _, phase := range tl.Phases {
		if phase != nil {
			phaseMap[phase.ID] = phase.Name
		}
	}

	// Convert milestones to tasks
	for _, m := range tl.Milestones {
		if m == nil {
			continue
		}

		task := &KanbanTask{
			ID:          m.ID,
			Title:       m.Name,
			Description: m.Description,
			Status:      h.mapStatusToKanban(m.Status),
			Type:        h.inferTaskType(m),
			Owner:       m.Owner,
			Progress:    m.Progress,
		}

		// Format due date if present
		if !m.ETA.IsZero() {
			task.DueDate = m.ETA.Format("2006-01-02")
		}

		tasks = append(tasks, task)

		// Also create individual task entries for milestone tasks
		for i, taskName := range m.Tasks {
			subTask := &KanbanTask{
				ID:          fmt.Sprintf("%s-task-%d", m.ID, i+1),
				Title:       taskName,
				Description: "",
				Status:      h.inferSubtaskStatus(m.Status, m.Progress, i, len(m.Tasks)),
				Type:        "task",
				Owner:       m.Owner,
				Phase:       m.ID,
			}
			if !m.ETA.IsZero() {
				subTask.DueDate = m.ETA.Format("2006-01-02")
			}
			tasks = append(tasks, subTask)
		}
	}

	// Also convert phases as high-level feature tasks
	for _, phase := range tl.Phases {
		if phase == nil {
			continue
		}

		task := &KanbanTask{
			ID:          phase.ID,
			Title:       phase.Name,
			Description: phase.Description,
			Status:      h.mapStatusToKanban(phase.Status),
			Type:        "feature",
			Progress:    phase.Progress,
		}

		if !phase.EndDate.IsZero() {
			task.DueDate = phase.EndDate.Format("2006-01-02")
		}

		tasks = append(tasks, task)
	}

	return tasks
}

// mapStatusToKanban converts internal status to kanban status
func (h *Handler) mapStatusToKanban(status string) string {
	switch status {
	case "completed":
		return "done"
	case "in_progress":
		return "in-progress"
	case "pending", "planned", "blocked":
		return "todo"
	default:
		return "todo"
	}
}

// inferTaskType determines task type based on milestone properties
func (h *Handler) inferTaskType(m *timeline.Milestone) string {
	if m == nil {
		return "task"
	}

	// Check name for hints
	name := strings.ToLower(m.Name)
	desc := strings.ToLower(m.Description)

	if strings.Contains(name, "bug") || strings.Contains(desc, "bug") || strings.Contains(desc, "fix") {
		return "bug"
	}
	if strings.Contains(name, "tech debt") || strings.Contains(desc, "tech debt") || strings.Contains(desc, "refactor") {
		return "tech-debt"
	}
	if strings.Contains(name, "feature") || strings.Contains(desc, "feature") {
		return "feature"
	}

	// Default based on presence of subtasks
	if len(m.Tasks) > 0 {
		return "milestone"
	}

	return "task"
}

// inferSubtaskStatus infers subtask status based on parent milestone
func (h *Handler) inferSubtaskStatus(parentStatus string, parentProgress int, taskIndex int, totalTasks int) string {
	if parentStatus == "completed" {
		return "done"
	}

	if parentStatus == "pending" || parentStatus == "planned" {
		return "todo"
	}

	// For in_progress milestones, estimate which tasks are done based on progress
	if totalTasks == 0 {
		return "todo"
	}

	completedTasks := (parentProgress * totalTasks) / 100
	if taskIndex < completedTasks {
		return "done"
	} else if taskIndex == completedTasks {
		return "in-progress"
	}

	return "todo"
}

// ============================================================================
// SSE STREAM ENDPOINT
// ============================================================================

// SSEEvent represents a server-sent event
type SSEEvent struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// HandleTimelineStream handles GET /api/v1/timeline/stream
// Provides Server-Sent Events for real-time timeline updates
func (h *Handler) HandleTimelineStream(w http.ResponseWriter, r *http.Request) {
	// Check if client supports SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "SSE_NOT_SUPPORTED", "streaming not supported", nil)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()

	// Send initial connection event
	h.sendSSEEvent(w, flusher, "connected", map[string]string{
		"message": "Connected to timeline stream",
		"service": "timeguru",
	})

	// Send initial tasks data
	tl, err := h.store.GetTimeline(ctx)
	if err == nil && tl != nil {
		tasks := h.transformToKanbanTasks(tl)
		h.sendSSEEvent(w, flusher, "tasks", TasksResponse{
			Tasks:      tasks,
			TotalCount: len(tasks),
		})
	}

	// Keep connection alive and poll for updates
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Track last update time for change detection
	var lastUpdate time.Time
	if tl != nil {
		lastUpdate = tl.LastUpdated
	}

	// Heartbeat ticker to keep connection alive
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return

		case <-heartbeat.C:
			// Send heartbeat to keep connection alive
			h.sendSSEEvent(w, flusher, "heartbeat", map[string]int64{
				"timestamp": time.Now().Unix(),
			})

		case <-ticker.C:
			// Check for timeline updates
			currentTL, err := h.store.GetTimeline(ctx)
			if err != nil {
				continue
			}

			// If timeline has been updated, send new data
			if currentTL != nil && currentTL.LastUpdated.After(lastUpdate) {
				lastUpdate = currentTL.LastUpdated

				tasks := h.transformToKanbanTasks(currentTL)
				h.sendSSEEvent(w, flusher, "update", TasksResponse{
					Tasks:      tasks,
					TotalCount: len(tasks),
				})
			}
		}
	}
}

// sendSSEEvent sends a Server-Sent Event
func (h *Handler) sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "event: %s\n", eventType)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
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
	// SECURITY: Enforce body size limit (1MB) to prevent denial-of-service
	if err := json.NewDecoder(io.LimitReader(r.Body, 1*1024*1024)).Decode(&req); err != nil {
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

	// Auto-sync to files after milestone update
	h.AutoSync(tl)

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"milestone": milestone,
		"message":   "milestone updated successfully",
	})
}

// HandleHealth handles GET /health
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, HealthResponse{
		Service:   "timeguru",
		Status:    "healthy",
		Version:   "1.0.0",
		Timestamp: time.Now().UTC(),
	})
}

// ReadyResponse represents readiness check response
type ReadyResponse struct {
	Ready   bool   `json:"ready"`
	Service string `json:"service"`
	Checks  struct {
		Store    bool `json:"store"`
		Timeline bool `json:"timeline"`
	} `json:"checks"`
}

// HandleReady handles GET /ready - returns 200 when service is ready to serve traffic
func (h *Handler) HandleReady(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp := ReadyResponse{
		Service: "timeguru",
	}

	// Check 1: Store is accessible
	resp.Checks.Store = h.store != nil

	// Check 2: Timeline is loaded/accessible
	if resp.Checks.Store {
		_, err := h.store.GetTimeline(ctx)
		resp.Checks.Timeline = err == nil
	}

	// Service is ready only if all checks pass
	resp.Ready = resp.Checks.Store && resp.Checks.Timeline

	if resp.Ready {
		h.writeJSON(w, http.StatusOK, resp)
	} else {
		h.writeJSON(w, http.StatusServiceUnavailable, resp)
	}
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
		case strings.Contains(accept, "application/toml"), strings.Contains(accept, "text/toml"):
			format = "toml"
		case strings.Contains(accept, "text/markdown"):
			format = "markdown"
		default:
			format = "json"
		}
	}

	switch strings.ToLower(format) {
	case "yaml", "yml":
		h.writeYAML(w, http.StatusOK, TimelineResponse{Timeline: tl})
	case "toml":
		h.writeTOML(w, http.StatusOK, tl)
	case "markdown", "md":
		h.writeMarkdown(w, http.StatusOK, tl)
	default:
		h.writeJSON(w, http.StatusOK, TimelineResponse{Timeline: tl})
	}
}

// ============================================================================
// SYNC & IMPORT ENDPOINTS
// ============================================================================

// SyncResponse represents the result of a sync operation
type SyncResponse struct {
	FilesWritten []string `json:"files_written"`
	Errors       []string `json:"errors,omitempty"`
}

// HandleSync handles POST /api/v1/timeline/sync - triggers file sync to all formats
func (h *Handler) HandleSync(w http.ResponseWriter, r *http.Request) {
	if h.syncer == nil {
		h.writeError(w, http.StatusServiceUnavailable, "SYNC_NOT_CONFIGURED",
			"file sync is not configured (no output directory)", nil)
		return
	}

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

	result := h.syncer.Sync(tl)
	resp := SyncResponse{
		FilesWritten: result.FilesWritten,
	}
	for _, e := range result.Errors {
		resp.Errors = append(resp.Errors, e.Error())
	}

	if len(result.Errors) > 0 && len(result.FilesWritten) == 0 {
		h.writeJSON(w, http.StatusInternalServerError, resp)
		return
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// HandleImport handles POST /api/v1/timeline/import - imports timeline from JSON/YAML/TOML body
func (h *Handler) HandleImport(w http.ResponseWriter, r *http.Request) {
	// Determine format from query param or Content-Type
	format := r.URL.Query().Get("format")
	if format == "" {
		ct := r.Header.Get("Content-Type")
		switch {
		case strings.Contains(ct, "application/json"):
			format = "json"
		case strings.Contains(ct, "application/yaml"), strings.Contains(ct, "text/yaml"):
			format = "yaml"
		case strings.Contains(ct, "application/toml"), strings.Contains(ct, "text/toml"):
			format = "toml"
		default:
			format = "json"
		}
	}

	f, err := tsync.DetectFormat("dummy." + format)
	if err != nil || f == tsync.FormatMarkdown {
		h.writeError(w, http.StatusBadRequest, "INVALID_FORMAT",
			fmt.Sprintf("unsupported import format %q (use json, yaml, or toml)", format), nil)
		return
	}

	// SECURITY: limit body to 10MB
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "READ_ERROR", "failed to read request body", err)
		return
	}
	defer r.Body.Close()

	tl, err := tsync.Decode(body, f)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "DECODE_ERROR",
			fmt.Sprintf("failed to decode %s: %s", format, err.Error()), nil)
		return
	}

	// Update last_updated timestamp
	tl.LastUpdated = time.Now().UTC()

	ctx := r.Context()
	if err := h.store.SaveTimeline(ctx, tl); err != nil {
		h.writeError(w, http.StatusInternalServerError, "SAVE_ERROR", "failed to save imported timeline", err)
		return
	}

	// Auto-sync to files if syncer is configured
	if h.syncer != nil {
		go func() {
			result := h.syncer.Sync(tl)
			tsync.LogSyncResult(result)
		}()
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "timeline imported successfully",
		"version":    tl.Version,
		"phases":     len(tl.Phases),
		"milestones": len(tl.Milestones),
	})
}

// AutoSync runs a background sync if a syncer is configured. Fire-and-forget.
func (h *Handler) AutoSync(tl *timeline.Timeline) {
	if h.syncer == nil || tl == nil {
		return
	}
	go func() {
		result := h.syncer.Sync(tl)
		tsync.LogSyncResult(result)
	}()
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

// writeTOML writes TOML response
func (h *Handler) writeTOML(w http.ResponseWriter, status int, tl *timeline.Timeline) {
	w.Header().Set("Content-Type", "application/toml")
	w.WriteHeader(status)

	encoder := toml.NewEncoder(w)
	if err := encoder.Encode(tl); err != nil {
		_ = err // Headers already sent
	}
}

// writeMarkdown generates and writes Markdown timeline
func (h *Handler) writeMarkdown(w http.ResponseWriter, status int, tl *timeline.Timeline) {
	w.Header().Set("Content-Type", "text/markdown")
	w.WriteHeader(status)

	md := h.generateMarkdown(tl)
	_, _ = w.Write([]byte(md))
}

// generateMarkdown converts Timeline to Markdown format
func (h *Handler) generateMarkdown(tl *timeline.Timeline) string {
	var sb strings.Builder

	sb.WriteString("# The Unheaded Chronicles\n\n")
	sb.WriteString("## A Living Grimoire of the Kingdom's Journey\n\n")
	fmt.Fprintf(&sb, "**STATUS:** %s\n", cases.Title(language.English).String(tl.Status))
	fmt.Fprintf(&sb, "**LAST UPDATED:** %s\n\n", tl.LastUpdated.Format("January 2, 2006"))
	sb.WriteString("---\n\n")

	if tl.Vision != "" {
		sb.WriteString("## The Founding Vision\n\n")
		fmt.Fprintf(&sb, "*%s*\n\n", tl.Vision)
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

		fmt.Fprintf(&sb, "### Age %d: %s (%s %s)\n\n",
			i, phase.Name, statusEmoji, strings.ToUpper(phase.Status))

		if phase.Description != "" {
			fmt.Fprintf(&sb, "%s\n\n", phase.Description)
		}

		if phase.Progress > 0 {
			fmt.Fprintf(&sb, "**Progress:** %d%%\n\n", phase.Progress)
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

			fmt.Fprintf(&sb, "### %s %s\n\n", statusEmoji, m.Name)

			if m.Description != "" {
				fmt.Fprintf(&sb, "%s\n\n", m.Description)
			}

			// Metadata
			if !m.ETA.IsZero() {
				fmt.Fprintf(&sb, "**ETA:** %s\n", m.ETA.Format("Jan 2, 2006"))
			}
			if m.Owner != "" {
				fmt.Fprintf(&sb, "**Owner:** %s\n", m.Owner)
			}
			if m.Risk != "" {
				fmt.Fprintf(&sb, "**Risk:** %s\n", cases.Title(language.English).String(m.Risk))
			}
			fmt.Fprintf(&sb, "**Progress:** %d%%\n", m.Progress)
			fmt.Fprintf(&sb, "**Status:** %s\n\n", m.Status)

			// Tasks
			if len(m.Tasks) > 0 {
				sb.WriteString("**Tasks:**\n")
				for _, task := range m.Tasks {
					// Assume task is complete if progress is 100
					checkbox := "[ ]"
					if m.Progress == 100 {
						checkbox = "[x]"
					}
					fmt.Fprintf(&sb, "- %s %s\n", checkbox, task)
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("---\n\n")
	sb.WriteString("**THE TIMEGURU KNOWS ALL.**\n")
	sb.WriteString("**THE CIRCLE NEVER BREAKS.**\n\n")
	fmt.Fprintf(&sb, "*Generated: %s*\n", tl.LastUpdated.Format("2006-01-02 15:04:05"))

	return sb.String()
}

// ============================================================================
// HELPERS
// ============================================================================

// writeJSON writes JSON response with defensive error handling
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	httputil.WriteJSON(w, status, data)
}

// writeError writes error response with defensive error handling
func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string, err error) {
	if err != nil {
		httputil.WriteErrorWithDetails(w, status, code, message, err.Error())
		return
	}
	httputil.WriteError(w, status, code, message)
}
