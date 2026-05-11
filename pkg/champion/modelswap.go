// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package champion

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ModelSwapResult is the shape returned to the caller after a successful
// model swap. NewModel echoes the requested key (post-allowlist),
// LoadSeconds is the wall-time the subprocess took, and Output captures
// the script's last 8 KiB of stdout/stderr for debugging.
type ModelSwapResult struct {
	NewModel    string  `json:"new_model"`
	LoadSeconds float64 `json:"load_seconds"`
	Output      string  `json:"output"`
}

// modelSwapState protects against concurrent swaps. Only one swap can be
// in-flight at a time across all goroutines (T13 — DoS via swap-spam
// mitigation). Acquired non-blocking; second concurrent attempt returns
// ErrSwapInProgress immediately.
type modelSwapState struct {
	mu       sync.Mutex
	inflight bool
	// scriptHash is captured at first ModelSwap call after process start
	// (lazy-init under mu); subsequent calls verify the script hasn't
	// been replaced beneath us (T15 — TOCTOU on script swap).
	scriptHash string
	scriptPath string
}

var swapState = &modelSwapState{}

// Sentinel errors so callers (and tests) can type-check the failure
// reason without string-matching.
var (
	ErrUnknownModel        = fmt.Errorf("model_switch: requested key is not in the allowlist")
	ErrSwapInProgress      = fmt.Errorf("model_switch: another swap is already running")
	ErrScriptModified      = fmt.Errorf("model_switch: switch-model.sh was modified after process start")
	ErrPrivilegeEscalation = fmt.Errorf("model_switch: refusing to run as root (EUID==0)")
	ErrSwapTimeout         = fmt.Errorf("model_switch: subprocess exceeded its time budget")
)

// allowlistKey enforces the security property the ADR-060 T11/T12/T14
// catalog requires: the key must be a short, conservative identifier.
// Anything outside [a-z0-9-] (lower-case letters, digits, hyphen) gets
// rejected before any path/subprocess code runs. This is defence in
// depth on top of the explicit allowlist parsed from switch-model.sh.
var allowlistKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// parseModelKeys reads scripts/switch-model.sh and returns the keys
// declared in its MODEL_FILE bash associative array. The parser is
// deliberately minimal — anything that doesn't match the exact
// `MODEL_FILE[<key>]="..."` line is ignored. Returns the keys in
// declaration order so the UI dropdown stays stable.
func parseModelKeys(scriptPath string) ([]string, error) {
	f, err := os.Open(scriptPath) // #nosec G304 — switchModelScript is a constant within projectRoot, validated by caller
	if err != nil {
		return nil, fmt.Errorf("open switch-model.sh: %w", err)
	}
	defer func() { _ = f.Close() }()

	keyLine := regexp.MustCompile(`^MODEL_FILE\[([a-z0-9][a-z0-9-]*)\]=`)
	var keys []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := keyLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		k := m[1]
		if seen[k] {
			continue // Last definition wins in bash; we keep first for stability.
		}
		seen[k] = true
		keys = append(keys, k)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan switch-model.sh: %w", err)
	}
	return keys, nil
}

// ListModelKeys returns the allowlisted model keys for `cmd/zhen-agentd`
// (and ultimately the UI dropdown). Reads scripts/switch-model.sh under
// the configured project root.
func (c *Champion) ListModelKeys() ([]string, error) {
	scriptPath := filepath.Join(c.config.ProjectRoot, "scripts", "switch-model.sh")
	return parseModelKeys(scriptPath)
}

// hashFile returns the SHA-256 of a file's contents as a hex string.
// Used by ModelSwap's TOCTOU guard.
func hashFile(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 — caller controls path (project script)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	buf := make([]byte, 64*1024)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ModelSwap swaps the running llama-server to the requested model key.
// Returns ErrUnknownModel for any key not in the allowlist, ErrSwapInProgress
// if a concurrent swap is mid-flight, ErrScriptModified if the script was
// mutated after process start, ErrPrivilegeEscalation if running as root,
// or ErrSwapTimeout if the subprocess exceeds its budget.
//
// The audit row in zhen_actions records action_type=model_switch with
// triggered_by from the caller (must be 'direct' for the gate to allow
// this destructive verb at the dispatch layer).
func (c *Champion) ModelSwap(ctx context.Context, key string) (*ModelSwapResult, error) {
	start := time.Now()
	intent := fmt.Sprintf("Switch llama-server model to: %s", key)
	actionID, _ := c.logAction(ctx, "model_switch", intent, "user")

	// T16 — refuse to run as root. The daemon should never have EUID==0;
	// hard fail rather than execute the subprocess with elevated privileges.
	if os.Geteuid() == 0 {
		c.completeAction(ctx, actionID, "denied", "", ErrPrivilegeEscalation.Error(), time.Since(start))
		return nil, ErrPrivilegeEscalation
	}

	// T11/T12 — defence-in-depth pre-allowlist regex check.
	if !allowlistKey.MatchString(key) {
		c.completeAction(ctx, actionID, "denied", "", "key fails [a-z0-9-] format check", time.Since(start))
		return nil, ErrUnknownModel
	}

	// Allowlist parsed at boot from switch-model.sh.
	allowed, err := c.ListModelKeys()
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", "list keys: "+err.Error(), time.Since(start))
		return nil, fmt.Errorf("list model keys: %w", err)
	}
	if !contains(allowed, key) {
		c.completeAction(ctx, actionID, "denied", "",
			fmt.Sprintf("key %q not in allowlist %v", key, allowed),
			time.Since(start))
		return nil, ErrUnknownModel
	}

	scriptPath := filepath.Join(c.config.ProjectRoot, "scripts", "switch-model.sh")

	// T13 — single-flight lock. Try-lock so concurrent attempts get a fast 429,
	// not a queue-up.
	swapState.mu.Lock()
	if swapState.inflight {
		swapState.mu.Unlock()
		c.completeAction(ctx, actionID, "denied", "", "another swap in progress", time.Since(start))
		return nil, ErrSwapInProgress
	}
	swapState.inflight = true

	// T15 — TOCTOU guard. Cache script hash on first call; require match on every subsequent call.
	if swapState.scriptPath == "" {
		swapState.scriptPath = scriptPath
		h, err := hashFile(scriptPath)
		if err != nil {
			swapState.inflight = false
			swapState.mu.Unlock()
			c.completeAction(ctx, actionID, "failed", "", "hash script: "+err.Error(), time.Since(start))
			return nil, fmt.Errorf("hash script: %w", err)
		}
		swapState.scriptHash = h
	} else {
		current, err := hashFile(scriptPath)
		if err != nil {
			swapState.inflight = false
			swapState.mu.Unlock()
			c.completeAction(ctx, actionID, "failed", "", "rehash script: "+err.Error(), time.Since(start))
			return nil, fmt.Errorf("rehash script: %w", err)
		}
		if current != swapState.scriptHash {
			swapState.inflight = false
			swapState.mu.Unlock()
			c.completeAction(ctx, actionID, "failed", "", ErrScriptModified.Error(), time.Since(start))
			return nil, ErrScriptModified
		}
	}
	swapState.mu.Unlock()

	// Release the in-flight flag on any return path.
	defer func() {
		swapState.mu.Lock()
		swapState.inflight = false
		swapState.mu.Unlock()
	}()

	// Subprocess timeout — generous because cold-cache deepseek-cpu took
	// 142 s in our prior session. 6 minutes covers the worst observed +
	// margin; ROCm cold-init has a long tail.
	const swapTimeout = 6 * time.Minute
	cmdCtx, cancel := context.WithTimeout(ctx, swapTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, scriptPath, key) // #nosec G204 — scriptPath is fixed under projectRoot, key passed allowlist check
	cmd.Dir = c.config.ProjectRoot
	out, runErr := cmd.CombinedOutput()
	elapsed := time.Since(start)

	output := string(out)
	const outputCap = 8 * 1024
	if len(output) > outputCap {
		output = output[len(output)-outputCap:]
		output = "[…earlier output truncated]\n" + output
	}

	if cmdCtx.Err() == context.DeadlineExceeded {
		c.completeAction(ctx, actionID, "failed", output, ErrSwapTimeout.Error(), elapsed)
		return nil, ErrSwapTimeout
	}

	if runErr != nil {
		c.completeAction(ctx, actionID, "failed", "", "subprocess: "+runErr.Error()+" — "+lastLine(output), elapsed)
		return nil, fmt.Errorf("model_switch subprocess: %w", runErr)
	}

	result := &ModelSwapResult{
		NewModel:    key,
		LoadSeconds: elapsed.Seconds(),
		Output:      output,
	}
	c.completeAction(ctx, actionID, "completed",
		fmt.Sprintf("Switched to %s in %.1fs", key, elapsed.Seconds()),
		"", elapsed)
	return result, nil
}

// contains is the tiny reusable helper. Generics in stdlib slices.Contains
// would do, but the repo's go.mod is on an older module style; prefer a
// local helper to a dependency tweak.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// lastLine returns the last non-empty line of output, useful for stuffing
// into the audit row's error_details field where 8 KiB is too much.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			if len(l) > 200 {
				return l[:200] + "…"
			}
			return l
		}
	}
	return ""
}
