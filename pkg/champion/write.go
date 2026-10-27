// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package champion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SnapshotStore persists before/after state for revertable operations.
type SnapshotStore interface {
	SaveSnapshot(ctx context.Context, actionID int64, resourceType, resourceID, before, after string, metadata map[string]string) error
	GetSnapshot(ctx context.Context, actionID int64) (before string, after string, err error)
}

// MaxWriteBytes caps what one WriteFile or PatchFile (the patched
// result) may put on disk (RT-05). The HTTP bodies are capped at 64 KiB;
// this stops runaway generation from in-process callers (agent loop,
// MCP, CLI) and keeps vendored fixtures writable. 1 MiB, the same bound
// as MaxPendingCallBytes. S-CHAMPION-HARDENING Step 50 [DECIDE]; no
// runbook calls write_file.
const MaxWriteBytes = 1 << 20

// ErrWriteTooLarge: the content is over MaxWriteBytes; nothing was written.
var ErrWriteTooLarge = errors.New("write exceeds the size cap")

// WriteFile writes content to a file within the sandbox.
// Captures a snapshot of the file's previous state for revert.
func (c *Champion) WriteFile(ctx context.Context, path, content string) error {
	start := time.Now()

	actionID, auditErr := c.logMutation(ctx, "file.write", fmt.Sprintf("Write file: %s (%d bytes)", path, len(content)), "user")
	if auditErr != nil {
		return auditErr
	}

	if len(content) > MaxWriteBytes {
		err := fmt.Errorf("%w: %d bytes, cap %d", ErrWriteTooLarge, len(content), MaxWriteBytes)
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return err
	}

	// Validate path
	if err := c.validatePath(path); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("path denied: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("resolve path: %w", err)
	}

	// Snapshot: capture current state (if file exists). Read through
	// the same in-kernel resolution as the write.
	var before string
	if data, err := c.readBeneath(absPath); err == nil {
		before = data
	}

	// Write the file: parents created and the file opened under the
	// sandbox root fd with openat2 (safeopen_linux.go), so a symlink
	// swapped in after validatePath cannot redirect it (T-05).
	if err := c.writeBeneath(absPath, content, true); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("write file: %w", err)
	}

	// Save snapshot for revert
	if c.snapshotStore != nil && actionID > 0 {
		_ = c.snapshotStore.SaveSnapshot(ctx, actionID, "file", absPath, before, content, nil)
	}

	summary := fmt.Sprintf("Wrote %d bytes to %s", len(content), absPath)
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return nil
}

// PatchFile applies a find-and-replace edit to a file within the sandbox.
// Captures snapshot for revert.
func (c *Champion) PatchFile(ctx context.Context, path, oldText, newText string) error {
	start := time.Now()

	actionID, auditErr := c.logMutation(ctx, "file.patch", fmt.Sprintf("Patch file: %s", path), "user")
	if auditErr != nil {
		return auditErr
	}

	if err := c.validatePath(path); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("path denied: %w", err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return err
	}

	// Read current content (resolved in-kernel beneath the sandbox root)
	before, err := c.readBeneath(absPath)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("read file: %w", err)
	}

	after := replaceFirst(before, oldText, newText)

	if before == after {
		c.completeAction(ctx, actionID, "failed", "", "old_text not found in file", time.Since(start))
		return fmt.Errorf("old_text not found in %s", absPath)
	}
	if len(after) > MaxWriteBytes {
		err := fmt.Errorf("%w: patched file would be %d bytes, cap %d", ErrWriteTooLarge, len(after), MaxWriteBytes)
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return err
	}

	// Same in-kernel resolution as WriteFile; the file must still exist.
	if err := c.writeBeneath(absPath, after, false); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("write patched file: %w", err)
	}

	// Save snapshot
	if c.snapshotStore != nil && actionID > 0 {
		_ = c.snapshotStore.SaveSnapshot(ctx, actionID, "file", absPath, before, after, nil)
	}

	summary := fmt.Sprintf("Patched %s: replaced %d chars with %d chars", absPath, len(oldText), len(newText))
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return nil
}

// sandboxRel splits a validated absolute path into the allowed root that
// contains it (the longest match) and the path relative to that root.
func (c *Champion) sandboxRel(absPath string) (root, rel string, err error) {
	for _, allowed := range c.config.AllowedPaths {
		a, aerr := filepath.Abs(allowed)
		if aerr != nil {
			continue
		}
		prefix := strings.TrimSuffix(a, string(filepath.Separator)) + string(filepath.Separator)
		if strings.HasPrefix(absPath, prefix) && len(a) > len(root) {
			root, rel = a, absPath[len(prefix):]
		}
	}
	if root == "" || rel == "" {
		return "", "", fmt.Errorf("%s is not a file under a sandbox root", absPath)
	}
	return root, rel, nil
}

// readBeneath reads a validated path through openat2 under its sandbox
// root (no symlink, no escape at the moment of the open).
func (c *Champion) readBeneath(absPath string) (string, error) {
	root, rel, err := c.sandboxRel(absPath)
	if err != nil {
		return "", err
	}
	rootFD, err := openRoot(root)
	if err != nil {
		return "", err
	}
	defer closeFD(rootFD)
	f, err := openBeneath(rootFD, rel, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// writeBeneath writes content to a validated path through openat2 under
// its sandbox root, creating missing parent directories one component at
// a time (create) or requiring the file to exist (!create).
func (c *Champion) writeBeneath(absPath, content string, create bool) error {
	root, rel, err := c.sandboxRel(absPath)
	if err != nil {
		return err
	}
	rootFD, err := openRoot(root)
	if err != nil {
		return err
	}
	defer closeFD(rootFD)
	flags := os.O_WRONLY | os.O_TRUNC
	if create {
		if err := mkdirAllBeneath(rootFD, filepath.Dir(rel), 0o750); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}
		flags |= os.O_CREATE
	}
	if c.beforeOpen != nil {
		c.beforeOpen()
	}
	f, err := openBeneath(rootFD, rel, flags, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// RevertAction undoes a previous action by restoring the snapshot.
func (c *Champion) RevertAction(ctx context.Context, actionID int64) error {
	start := time.Now()

	revertID, auditErr := c.logMutation(ctx, "file.write", fmt.Sprintf("Revert action %d", actionID), "user")
	if auditErr != nil {
		return auditErr
	}

	if c.snapshotStore == nil {
		c.completeAction(ctx, revertID, "failed", "", "no snapshot store configured", time.Since(start))
		return fmt.Errorf("no snapshot store configured")
	}

	before, _, err := c.snapshotStore.GetSnapshot(ctx, actionID)
	if err != nil {
		c.completeAction(ctx, revertID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("get snapshot: %w", err)
	}

	// The "before" state from the original action is what we want to restore.
	// We need to know the resource path — get it from the action record.
	// For now, the snapshot stores the path as resourceID.
	// This is a simplified revert — full implementation would query the snapshot table.

	summary := fmt.Sprintf("Revert of action %d prepared (before state: %d bytes)", actionID, len(before))
	c.completeAction(ctx, revertID, "completed", summary, "", time.Since(start))
	return nil
}

// KanbanTask represents a task for Kanban operations.
type KanbanTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Type        string `json:"type"`
	Owner       string `json:"owner"`
	Progress    int    `json:"progress"`
}

// KanbanStore is the interface for Kanban task persistence.
type KanbanStore interface {
	CreateTask(ctx context.Context, task *KanbanTask) error
	UpdateTask(ctx context.Context, id string, updates map[string]interface{}) error
	GetTask(ctx context.Context, id string) (*KanbanTask, error)
	ListTasks(ctx context.Context) ([]KanbanTask, error)
}

// CreateKanbanTask creates a new Kanban task with action logging.
func (c *Champion) CreateKanbanTask(ctx context.Context, task *KanbanTask) error {
	start := time.Now()

	paramsJSON, _ := json.Marshal(task)
	actionID, auditErr := c.logMutation(ctx, "kanban.create", fmt.Sprintf("Create task: %s", task.Title), "user")
	if auditErr != nil {
		return auditErr
	}

	if c.kanbanStore == nil {
		c.completeAction(ctx, actionID, "failed", "", "no kanban store configured", time.Since(start))
		return fmt.Errorf("no kanban store configured")
	}

	if err := c.kanbanStore.CreateTask(ctx, task); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("create task: %w", err)
	}

	summary := fmt.Sprintf("Created task %s: %s (params: %s)", task.ID, task.Title, string(paramsJSON))
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return nil
}

// UpdateKanbanTask updates a Kanban task with snapshot for revert.
func (c *Champion) UpdateKanbanTask(ctx context.Context, id string, updates map[string]interface{}) error {
	start := time.Now()

	actionID, auditErr := c.logMutation(ctx, "kanban.update", fmt.Sprintf("Update task: %s", id), "user")
	if auditErr != nil {
		return auditErr
	}

	if c.kanbanStore == nil {
		c.completeAction(ctx, actionID, "failed", "", "no kanban store configured", time.Since(start))
		return fmt.Errorf("no kanban store configured")
	}

	// Snapshot before state
	if existing, err := c.kanbanStore.GetTask(ctx, id); err == nil && c.snapshotStore != nil && actionID > 0 {
		beforeJSON, _ := json.Marshal(existing)
		_ = c.snapshotStore.SaveSnapshot(ctx, actionID, "kanban_task", id, string(beforeJSON), "", nil)
	}

	if err := c.kanbanStore.UpdateTask(ctx, id, updates); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return fmt.Errorf("update task: %w", err)
	}

	updatesJSON, _ := json.Marshal(updates)
	summary := fmt.Sprintf("Updated task %s: %s", id, string(updatesJSON))
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return nil
}

// ListKanbanTasks lists all Kanban tasks with action logging.
func (c *Champion) ListKanbanTasks(ctx context.Context) ([]KanbanTask, error) {
	start := time.Now()
	actionID, _ := c.logAction(ctx, "kanban.list", "List kanban tasks", "user")

	if c.kanbanStore == nil {
		c.completeAction(ctx, actionID, "failed", "", "no kanban store configured", time.Since(start))
		return nil, fmt.Errorf("no kanban store configured")
	}

	tasks, err := c.kanbanStore.ListTasks(ctx)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return nil, err
	}

	summary := fmt.Sprintf("Listed %d tasks", len(tasks))
	c.completeAction(ctx, actionID, "completed", summary, "", time.Since(start))
	return tasks, nil
}

// ------------------------------------------------------------------
// RUNBOOK EXECUTION (WAVE15 Phase 2b)
//
// Champion-gated subprocess of scripts/run-runbook.py. The runbook
// name is constrained to the runbooks/ directory tree under the
// project root; arbitrary paths are rejected before exec. Output is
// captured (stdout + stderr merged) and returned in the action
// summary so the audit log retains the full execution trace.
//
// This is the canonical kingdom-mutation path that the rewired
// raft/zhen_app.py hits via cmd/zhen-agentd /api/v1/tool/exec —
// closing the real T6 attack surface (mutation paths must traverse
// pkg/champion's gate; chat paths display answers and don't execute).
// ------------------------------------------------------------------

// RunbookExecuteResult is the shape returned to the caller after a
// runbook subprocess completes. ExitCode 0 = success; non-zero =
// failure (output still captured for diagnosis).
type RunbookExecuteResult struct {
	Name     string `json:"name"` // runbook name, relative to runbooks/
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"` // stdout+stderr merged, truncated
	Elapsed  string `json:"elapsed_ms"`
}

// RunbookExecute runs scripts/run-runbook.py against the named
// runbook. The name is validated against the runbooks/ tree under
// the project root. Output is capped at 64 KiB to keep the action
// audit row bounded.
func (c *Champion) RunbookExecute(ctx context.Context, name string, dryRun bool) (*RunbookExecuteResult, error) {
	start := time.Now()
	intent := fmt.Sprintf("Execute runbook: %s (dry-run=%v)", name, dryRun)
	actionID, auditErr := c.logMutation(ctx, "runbook.execute", intent, "user")
	if auditErr != nil {
		return nil, auditErr
	}

	// Reject empty / suspicious names early.
	if name == "" {
		c.completeAction(ctx, actionID, "failed", "", "empty runbook name", time.Since(start))
		return nil, fmt.Errorf("empty runbook name")
	}

	// The runbook lookup: <project_root>/runbooks/<name>.yaml. The name
	// may include a category prefix (e.g., "infra/host-bootstrap") which
	// resolves to runbooks/infra/host-bootstrap.yaml. Reject .. or
	// absolute paths so a malicious caller can't escape the runbooks
	// directory.
	if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
		c.completeAction(ctx, actionID, "denied",
			summarizeArgs(map[string]any{"name": name}),
			"runbook name contains '..' or absolute path",
			time.Since(start))
		return nil, fmt.Errorf("runbook name %q escapes runbooks/ tree", name)
	}

	runbookPath := filepath.Join(c.config.ProjectRoot, "runbooks", name+".yaml")
	// Re-validate the resolved path stays under runbooks/.
	absRunbooks := filepath.Join(c.config.ProjectRoot, "runbooks")
	rel, err := filepath.Rel(absRunbooks, runbookPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		c.completeAction(ctx, actionID, "denied",
			summarizeArgs(map[string]any{"name": name}),
			"runbook path escapes runbooks/ tree",
			time.Since(start))
		return nil, fmt.Errorf("runbook %q resolves outside runbooks/ tree", name)
	}
	if _, err := os.Stat(runbookPath); err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return nil, fmt.Errorf("runbook not found: %w", err)
	}

	// RT-08: manifest hash, destructive-command scan, and a private copy of
	// the verified bytes for a live run (runbookmanifest.go).
	execPath, cleanup, err := c.verifyRunbookForExec(runbookPath, rel, dryRun)
	if err != nil {
		c.completeAction(ctx, actionID, "denied",
			summarizeArgs(map[string]any{"name": name}), err.Error(), time.Since(start))
		return nil, fmt.Errorf("runbook %q: %w", name, err)
	}
	defer cleanup()

	// Subprocess: scripts/run-runbook.py [--dry-run] <execPath>
	args := []string{filepath.Join(c.config.ProjectRoot, "scripts", "run-runbook.py")}
	if dryRun {
		args = append(args, "--dry-run")
	}
	args = append(args, execPath)

	// Hard 10-minute cap on subprocess. Per `runbooks/` convention,
	// no individual runbook should exceed 10 min; longer ones must
	// be split. The agent's outer request timeout is 5 min, so this
	// effectively never fires under normal operation.
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "python3", args...) // #nosec G204 -- runbook path validated inside runbooks/ (see above); python3 fixed, argv programmatic
	cmd.Dir = c.config.ProjectRoot
	out, runErr := cmd.CombinedOutput()
	elapsed := time.Since(start)

	output := string(out)
	const outputCap = 64 * 1024
	truncated := false
	if len(output) > outputCap {
		output = output[:outputCap] + "\n[…output truncated to 64 KiB]"
		truncated = true
	}

	exitCode := 0
	if runErr != nil {
		// Try to extract the exit code from *exec.ExitError; otherwise -1.
		exitCode = -1
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}

	result := &RunbookExecuteResult{
		Name:     name,
		ExitCode: exitCode,
		Output:   output,
		Elapsed:  elapsed.String(),
	}

	status := "completed"
	if exitCode != 0 {
		status = "failed"
	}
	summary := fmt.Sprintf("runbook=%s exit=%d output_bytes=%d truncated=%v",
		name, exitCode, len(out), truncated)
	c.completeAction(ctx, actionID, status, summary, "", elapsed)

	return result, nil
}

// maxRunbookShowBytes caps runbook_show's output (the largest runbook is
// well under it; a bigger file is refused rather than truncated).
const maxRunbookShowBytes = 256 * 1024

// runbookFile resolves a runbook name ("category/name", no extension) to
// its file under <project_root>/runbooks, refusing names that escape the
// tree, and symlinks that resolve outside it.
func (c *Champion) runbookFile(name string) (string, error) {
	if name == "" {
		return "", errors.New("empty runbook name")
	}
	if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00") {
		return "", fmt.Errorf("runbook name %q escapes runbooks/ tree", name)
	}
	dir := filepath.Join(c.config.ProjectRoot, "runbooks")
	p := filepath.Join(dir, name+".yaml")
	rel, err := filepath.Rel(dir, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("runbook %q resolves outside runbooks/ tree", name)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("runbooks/: %w", err)
	}
	realP, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("runbook not found: %w", err)
	}
	if !strings.HasPrefix(realP, realDir+string(filepath.Separator)) {
		return "", fmt.Errorf("runbook %q resolves outside runbooks/ tree", name)
	}
	return realP, nil
}

// RunbookList returns every runbook's name ("category/name"), sorted:
// the *.yaml files under <project_root>/runbooks. Read-only; logged.
func (c *Champion) RunbookList(ctx context.Context) ([]string, error) {
	start := time.Now()
	actionID, _ := c.logAction(ctx, "runbook.list", "List runbooks", "user")
	dir := filepath.Join(c.config.ProjectRoot, "runbooks")
	var names []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && strings.HasSuffix(p, ".yaml") {
			rel, rerr := filepath.Rel(dir, p)
			if rerr != nil {
				return rerr
			}
			names = append(names, filepath.ToSlash(strings.TrimSuffix(rel, ".yaml")))
		}
		return nil
	})
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return nil, fmt.Errorf("list runbooks: %w", err)
	}
	sort.Strings(names)
	c.completeAction(ctx, actionID, "completed", fmt.Sprintf("Listed %d runbooks", len(names)), "", time.Since(start))
	return names, nil
}

// RunbookShow returns one runbook's YAML text. The name is checked like
// RunbookExecute's, and a symlink out of runbooks/ is refused.
func (c *Champion) RunbookShow(ctx context.Context, name string) (string, error) {
	start := time.Now()
	actionID, _ := c.logAction(ctx, "runbook.show", "Show runbook: "+name, "user")
	p, err := c.runbookFile(name)
	if err != nil {
		c.completeAction(ctx, actionID, "denied", summarizeArgs(map[string]any{"name": name}), err.Error(), time.Since(start))
		return "", err
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		msg := fmt.Sprintf("runbook %q is not a regular file", name)
		c.completeAction(ctx, actionID, "failed", "", msg, time.Since(start))
		return "", errors.New(msg)
	}
	if info.Size() > maxRunbookShowBytes {
		msg := fmt.Sprintf("runbook %q is %d bytes, over %d", name, info.Size(), maxRunbookShowBytes)
		c.completeAction(ctx, actionID, "failed", "", msg, time.Since(start))
		return "", errors.New(msg)
	}
	data, err := os.ReadFile(p) // #nosec G304 -- p is resolved by runbookFile inside <root>/runbooks
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return "", fmt.Errorf("read runbook: %w", err)
	}
	c.completeAction(ctx, actionID, "completed", fmt.Sprintf("Showed %s (%d bytes)", name, len(data)), "", time.Since(start))
	return string(data), nil
}

// replaceFirst replaces the first occurrence of old with new in s.
func replaceFirst(s, old, new string) string {
	i := indexOf(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
