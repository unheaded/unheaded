// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package champion implements the Zhen Champion Agent — an active Kingdom agent
// that can read tasks, search the corpus, read files (sandboxed), and check service
// health. Every operation is logged to The Well (PostgreSQL) for full auditability.
//
// Trust Level 1: Read-only operations.
// Trust Level 2 (future): Write operations with snapshot+revert.
//
// See ADR-019 for the full design.
package champion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// Action represents a logged Champion operation in The Well.
type Action struct {
	ID            int64     `json:"id"`
	SessionID     string    `json:"session_id"`
	ActionType    string    `json:"action_type"`
	Status        string    `json:"status"`
	Intent        string    `json:"intent"`
	Parameters    string    `json:"parameters"`
	Result        string    `json:"result"`
	ResultSummary string    `json:"result_summary"`
	Error         string    `json:"error"`
	TriggeredBy   string    `json:"triggered_by"`
	PlannedAt     time.Time `json:"planned_at"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	ElapsedMs     int       `json:"elapsed_ms"`
}

// SearchResult represents a corpus search result.
type SearchResult struct {
	Content   string  `json:"content"`
	Source    string  `json:"source"`
	Relevance float64 `json:"relevance"`
	ChunkType string  `json:"type"`
}

// ServiceStatus represents the health of a Kingdom service.
type ServiceStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Port    int    `json:"port"`
	Latency int    `json:"latency_ms"`
}

// ActionStore is the interface for persisting actions to The Well.
type ActionStore interface {
	LogAction(ctx context.Context, action *Action) (int64, error)
	UpdateAction(ctx context.Context, id int64, status, result, errMsg string, elapsedMs int) error
	GetActions(ctx context.Context, sessionID string, limit int) ([]Action, error)
}

// Config holds Champion configuration.
type Config struct {
	// ProjectRoot is the base path for file operations.
	ProjectRoot string

	// AllowedPaths are additional paths the Champion can read (beyond ProjectRoot).
	AllowedPaths []string

	// DeniedPaths are paths that are always blocked.
	DeniedPaths []string

	// SessionID is the current session identifier.
	SessionID string
}

// Champion is the Zhen Champion Agent — Trust Level 1 (read) + Level 2 (write).
type Champion struct {
	config        Config
	store         ActionStore
	snapshotStore SnapshotStore
	kanbanStore   KanbanStore
	memoryStore   MemoryStore // nil: memory_write refused (memory.go)
	confirmMu     sync.Mutex
	confirmStore  *confirmStore   // lazy-init via confirms(); guarded by confirmMu
	runCheck      *RunCheckConfig // nil: run_check refused (runcheck.go)
	branchSandbox *branchSandbox  // nil: off (branchsandbox.go)
	beforeOpen    func()          // test hook: runs between validation and the open (Step 68)
}

// New creates a new Champion with the given configuration.
func New(cfg Config, store ActionStore) (*Champion, error) {
	if cfg.ProjectRoot == "" {
		return nil, fmt.Errorf("project root is required")
	}
	if cfg.SessionID == "" {
		cfg.SessionID = fmt.Sprintf("session-%d", time.Now().UnixMilli())
	}

	// Default allowed paths
	if len(cfg.AllowedPaths) == 0 {
		cfg.AllowedPaths = []string{
			cfg.ProjectRoot,
			"/var/zhen/",
		}
	}

	// Default denied paths
	if len(cfg.DeniedPaths) == 0 {
		cfg.DeniedPaths = []string{
			"/etc/shadow",
			"/etc/passwd",
			"/.ssh/",
			"/id_rsa",
			"/id_ed25519",
			".env",
			"credentials",
			"secrets",
		}
	}

	return &Champion{config: cfg, store: store}, nil
}

// GetProjectRoot returns the configured project root. Used by callers
// (e.g., agent runtime) that need to construct paths relative to the
// sandbox root without storing the Config separately.
func (c *Champion) GetProjectRoot() string {
	return c.config.ProjectRoot
}

// WithSnapshotStore sets the snapshot store for revertable write operations.
func (c *Champion) WithSnapshotStore(s SnapshotStore) *Champion {
	c.snapshotStore = s
	return c
}

// WithKanbanStore sets the Kanban store for task operations.
func (c *Champion) WithKanbanStore(s KanbanStore) *Champion {
	c.kanbanStore = s
	return c
}

// ReadFile reads a file within the sandbox. Returns content and error.
// Every read is logged to The Well.
func (c *Champion) ReadFile(ctx context.Context, path string) (string, error) {
	start := time.Now()

	// Log intent
	actionID, _ := c.logAction(ctx, "file.read", fmt.Sprintf("Read file: %s", path), "user")

	// Validate path
	if err := c.validatePath(path); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return "", fmt.Errorf("path denied: %w", err)
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return "", fmt.Errorf("resolve path: %w", err)
	}

	// Read file
	data, err := os.ReadFile(absPath) // #nosec G304 -- sandboxed path — validated by validatePath (no .., blocklist, allowed-prefix check)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return "", fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	summary := fmt.Sprintf("Read %d bytes from %s", len(data), absPath)
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))

	return content, nil
}

// validatePath checks if a path is within the sandbox.
func (c *Champion) validatePath(path string) error {
	// Resolve symlinks and normalize
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve path: %w", err)
	}

	// Block traversal attacks
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal blocked: %s", path)
	}

	// Check denied paths
	for _, denied := range c.config.DeniedPaths {
		if strings.Contains(absPath, denied) {
			return fmt.Errorf("path contains denied pattern: %s", denied)
		}
	}

	// Check allowed paths
	inside := false
	for _, allowed := range c.config.AllowedPaths {
		allowedAbs, _ := filepath.Abs(allowed)
		if strings.HasPrefix(absPath, allowedAbs) {
			inside = true
			break
		}
	}
	if !inside {
		return fmt.Errorf("path outside sandbox: %s (allowed: %v)", absPath, c.config.AllowedPaths)
	}

	// The same checks on the real path: a symlink inside the sandbox
	// must not lead a read or write outside it (S-TWO-BRAINS Step 86b).
	// The deepest existing ancestor is resolved, so a file that does not
	// exist yet is judged by the directory it would land in.
	resolved, err := realPath(absPath)
	if err != nil {
		return fmt.Errorf("cannot resolve path: %w", err)
	}
	for _, denied := range c.config.DeniedPaths {
		if strings.Contains(resolved, denied) {
			return fmt.Errorf("path resolves to a denied pattern: %s", denied)
		}
	}
	for _, allowed := range c.config.AllowedPaths {
		allowedAbs, _ := filepath.Abs(allowed)
		allowedReal, err := realPath(allowedAbs)
		if err != nil {
			continue
		}
		if resolved == allowedReal || strings.HasPrefix(resolved, strings.TrimSuffix(allowedReal, string(filepath.Separator))+string(filepath.Separator)) {
			return nil
		}
	}
	return fmt.Errorf("path resolves outside sandbox: %s -> %s", absPath, resolved)
}

// realPath resolves symlinks in the deepest existing ancestor of an
// absolute path and appends the parts that do not exist yet.
func realPath(abs string) (string, error) {
	p := filepath.Clean(abs)
	var rest []string
	for {
		if _, err := os.Lstat(p); err == nil {
			r, err := filepath.EvalSymlinks(p)
			if err != nil {
				return "", err
			}
			for i := len(rest) - 1; i >= 0; i-- {
				r = filepath.Join(r, rest[i])
			}
			return r, nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Clean(abs), nil
		}
		rest = append(rest, filepath.Base(p))
		p = parent
	}
}

// Audit-write failure policy (RT-10, S-CHAMPION-HARDENING Step 60
// [DECIDE]): the action log is ADR-019's review surface, so a mutation
// with no audit row does not run (logMutation → ErrAuditUnavailable);
// reads and gate/confirm bookkeeping rows fail open. Every failed audit
// write, either way, is counted and logged.

// ErrAuditUnavailable: the action log could not record a mutation's
// intent, so the mutation was refused and nothing changed.
var ErrAuditUnavailable = errors.New("audit log unavailable; mutation refused")

// auditWriteFailures counts action-store writes that failed, by stage:
// "log" (the planned row) or "complete" (the status update).
var auditWriteFailures = auto.NewCounterVec(prom.CounterOpts{
	Name: "champion_audit_write_failures_total",
	Help: "Champion action-log writes that failed, by stage (log, complete). A failed log row refuses a mutation.",
}, []string{"stage"})

// logAction logs an action intent to The Well. A store error is counted
// and logged here; the caller decides whether it may proceed (reads and
// bookkeeping rows do; mutations go through logMutation and do not).
func (c *Champion) logAction(ctx context.Context, actionType, intent, triggeredBy string) (int64, error) {
	if c.store == nil {
		return 0, nil
	}
	action := &Action{
		SessionID:   c.config.SessionID,
		ActionType:  actionType,
		Status:      "planned",
		Intent:      intent,
		TriggeredBy: triggeredBy,
		PlannedAt:   time.Now(),
	}
	id, err := c.store.LogAction(ctx, action)
	if err != nil {
		auditWriteFailures.WithLabelValues("log").Inc()
		log.Warn().Str("service", "champion").Str("action_type", actionType).Err(err).Msg("audit log write failed")
	}
	return id, err
}

// logMutation is logAction for a state-changing operation: when the
// planned row cannot be written, it returns ErrAuditUnavailable and the
// caller must not mutate anything.
func (c *Champion) logMutation(ctx context.Context, actionType, intent, triggeredBy string) (int64, error) {
	id, err := c.logAction(ctx, actionType, intent, triggeredBy)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrAuditUnavailable, err)
	}
	return id, nil
}

// completeAction updates an action's status after execution. A failed
// update is counted and logged; the operation already happened (or was
// refused), so it is not undone.
func (c *Champion) completeAction(ctx context.Context, id int64, status, result, errMsg string, elapsed time.Duration) {
	if c.store == nil || id == 0 {
		return
	}
	if err := c.store.UpdateAction(ctx, id, status, result, errMsg, int(elapsed.Milliseconds())); err != nil {
		auditWriteFailures.WithLabelValues("complete").Inc()
		log.Warn().Str("service", "champion").Int64("action_id", id).Str("status", status).Err(err).Msg("audit log status update failed")
	}
}
