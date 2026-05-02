// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	Content    string  `json:"content"`
	Source     string  `json:"source"`
	Relevance  float64 `json:"relevance"`
	ChunkType  string  `json:"type"`
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
	confirmStore  *confirmStore // lazy-init via IssuePendingConfirmation
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
	data, err := os.ReadFile(absPath)
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
	for _, allowed := range c.config.AllowedPaths {
		allowedAbs, _ := filepath.Abs(allowed)
		if strings.HasPrefix(absPath, allowedAbs) {
			return nil
		}
	}

	return fmt.Errorf("path outside sandbox: %s (allowed: %v)", absPath, c.config.AllowedPaths)
}

// logAction logs an action intent to The Well.
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
	return c.store.LogAction(ctx, action)
}

// completeAction updates an action's status after execution.
func (c *Champion) completeAction(ctx context.Context, id int64, status, result, errMsg string, elapsed time.Duration) {
	if c.store == nil || id == 0 {
		return
	}
	c.store.UpdateAction(ctx, id, status, result, errMsg, int(elapsed.Milliseconds()))
}
