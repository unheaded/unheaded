// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package champion

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
//
// T19 (S-CHAMPION-HARDENING Step 216): before the swap it reads which key
// is serving (switch-model.sh --json --status) and the target's documented
// load time; the swap gets 4x that plus the GPU lease wait (swapBudget).
// A swap that runs out of time, or exits 1 (the script had stopped the old
// server), is followed by a swap back to the previous key, unless that key
// is still serving. The error says what happened: errors.Is(err,
// ErrSwapTimeout) holds after a timeout either way.
func (c *Champion) ModelSwap(ctx context.Context, key string) (res *ModelSwapResult, err error) {
	start := time.Now()
	ev := ModelSwapEvent{Key: key, Outcome: "failed"}
	if !allowlistKey.MatchString(key) {
		ev.Key = "invalid"
	}
	defer func() {
		if err == nil {
			ev.Outcome = "ok"
		}
		ev.Err = err
		ev.Duration = time.Since(start)
		c.observeModelSwap(ev)
	}()
	intent := fmt.Sprintf("Switch llama-server model to: %s", key)
	actionID, auditErr := c.logMutation(ctx, "model_switch", intent, "user")
	if auditErr != nil {
		return nil, auditErr
	}

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

	// What serves now, and how long the target is documented to take. A
	// script without --json leaves both unknown: the legacy budget, no
	// rollback.
	before, _ := c.ModelStatus(ctx)
	if before != nil {
		ev.From = before.Active
	}
	budget := swapBudget(before.loadSeconds(key))

	output, timedOut, runErr := c.runSwitchScript(ctx, scriptPath, key, budget)
	elapsed := time.Since(start)

	if timedOut || exitCode(runErr) == 1 {
		// The old server may be gone: put the previous model back.
		note, rolled := c.rollbackModel(ctx, scriptPath, key, before)
		if rolled {
			ev.Outcome = "rolled_back"
		}
		if timedOut {
			msg := fmt.Sprintf("after %s; %s", budget.Round(time.Millisecond), note)
			c.completeAction(ctx, actionID, "failed", output, ErrSwapTimeout.Error()+" "+msg, elapsed)
			return nil, fmt.Errorf("%w %s", ErrSwapTimeout, msg)
		}
		c.completeAction(ctx, actionID, "failed", "", "subprocess: "+runErr.Error()+" — "+lastLine(output)+"; "+note, elapsed)
		return nil, fmt.Errorf("model_switch subprocess: %w; %s", runErr, note)
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

// ModelSwapEvent is what a ModelSwap observer sees once per call.
type ModelSwapEvent struct {
	Key      string // the requested key; "invalid" when it fails the key format
	From     string // the key serving before the swap, "" if unknown
	Outcome  string // ok | failed | rolled_back
	Duration time.Duration
	Err      error
}

// WithModelSwapObserver sets a function called after every ModelSwap
// (zhen-agentd's swap metrics).
func (c *Champion) WithModelSwapObserver(f func(ModelSwapEvent)) *Champion {
	c.swapObserver = f
	return c
}

func (c *Champion) observeModelSwap(ev ModelSwapEvent) {
	if c.swapObserver != nil {
		c.swapObserver(ev)
	}
}

const (
	// legacySwapTimeout is the budget when the load time is unknown:
	// cold-cache deepseek-cpu took 142 s; ROCm cold-init has a long tail.
	legacySwapTimeout = 6 * time.Minute
	// defaultSwapLeaseWait is the GPU lease wait zhen-agentd allows a swap
	// (switch-model.sh's own default is 600 s); GPU_LEASE_TIMEOUT overrides.
	defaultSwapLeaseWait = 60 * time.Second
	// swapOutputCap keeps the last 8 KiB of the script's output.
	swapOutputCap = 8 * 1024
)

// swapLeaseWait is GPU_LEASE_TIMEOUT if set, else defaultSwapLeaseWait.
func swapLeaseWait() time.Duration {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("GPU_LEASE_TIMEOUT"))); err == nil && v >= 0 {
		return time.Duration(v) * time.Second
	}
	return defaultSwapLeaseWait
}

// swapBudget is ADR-060 T19's hard timeout: 4x the documented load time,
// plus the lease wait; legacySwapTimeout when the load time is unknown.
func swapBudget(loadSeconds float64) time.Duration {
	if loadSeconds <= 0 {
		return legacySwapTimeout
	}
	return time.Duration(4*loadSeconds*float64(time.Second)) + swapLeaseWait()
}

// runSwitchScript runs switch-model.sh <key> in its own process group with
// the given budget. On timeout the whole group is killed, including a
// llama-server the script had started that never became healthy.
func (c *Champion) runSwitchScript(ctx context.Context, scriptPath, key string, budget time.Duration) (output string, timedOut bool, err error) {
	cmdCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, scriptPath, key) // #nosec G204 — scriptPath is fixed under projectRoot, key passed allowlist check
	cmd.Dir = c.config.ProjectRoot
	cmd.Env = append(os.Environ(), "GPU_LEASE_TIMEOUT="+strconv.Itoa(int(swapLeaseWait()/time.Second)))
	killProcessGroupOnCancel(cmd)
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	output = string(out)
	if len(output) > swapOutputCap {
		output = "[…earlier output truncated]\n" + output[len(output)-swapOutputCap:]
	}
	return output, errors.Is(cmdCtx.Err(), context.DeadlineExceeded), err
}

// rollbackModel starts the previous key again after a failed swap to key.
// It returns what happened, for the error and the audit row, and whether
// a rollback swap succeeded. It runs even if the caller has gone away.
func (c *Champion) rollbackModel(ctx context.Context, scriptPath, key string, before *ModelStatus) (string, bool) {
	if before == nil || before.Active == "" {
		return "nothing to roll back to (no model was serving, or the script has no --json)", false
	}
	prev := before.Active
	if prev == key {
		return "the previous model was " + key + " itself; not restarted", false
	}
	bg := context.WithoutCancel(ctx)
	if now, err := c.ModelStatus(bg); err == nil && now.Active == prev {
		return prev + " is still serving; no rollback needed", false
	}
	out, timedOut, err := c.runSwitchScript(bg, scriptPath, prev, swapBudget(before.loadSeconds(prev)))
	switch {
	case timedOut:
		return "rollback to " + prev + " also timed out; nothing is serving", false
	case err != nil:
		return "rollback to " + prev + " failed: " + err.Error() + " — " + lastLine(out), false
	}
	return "rolled back to " + prev, true
}

// exitCode is err's process exit code, or -1.
func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
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
