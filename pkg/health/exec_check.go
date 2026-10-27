// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package health

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Exec checks (S-IRON-GATE Phase 10, ADR-034 Phase 7): a target that runs a
// local command instead of an HTTP GET. Exit 0 is a healthy vote, anything
// else (non-zero exit, timeout, a program that cannot start) a failing one.
// The command is argv only, never a shell; it runs as Akira's own uid with a
// minimal environment, stdin from /dev/null, at most MaxExecTimeout, and
// keeps the last ExecOutputCap bytes of its stdout+stderr.

const (
	// ExecOutputCap is how much of a check's combined output is kept (the tail).
	ExecOutputCap = 4096
	// DefaultExecTimeout applies when a check sets none; it is also the ceiling.
	DefaultExecTimeout = 30 * time.Second
	// MaxExecTimeout is the longest a check may run before it is killed.
	MaxExecTimeout = 30 * time.Second
	// execErrorTail is how much of the output a failing report carries.
	execErrorTail = 1024
	// execPath is the PATH a check sees (its own argv[0] is absolute).
	execPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ExecCheck is a command Akira runs as a health check.
type ExecCheck struct {
	// Argv is the program and its arguments; Argv[0] must be absolute.
	Argv []string
	// Timeout kills the command's whole process group; 0 means DefaultExecTimeout.
	Timeout time.Duration
	// Interval between runs; between runs each sweep re-votes the last result.
	// 0 runs it on every sweep.
	Interval time.Duration
	// Env is added to the minimal environment (PATH, LANG). Akira's own
	// environment (its key paths, auth settings) is never passed on.
	Env map[string]string
}

// Validate reports a configuration Akira must refuse to start with.
func (c ExecCheck) Validate() error {
	if len(c.Argv) == 0 || c.Argv[0] == "" {
		return errors.New("exec check: argv is empty")
	}
	if !filepath.IsAbs(c.Argv[0]) {
		return fmt.Errorf("exec check: argv[0] %q must be an absolute path (no PATH lookup)", c.Argv[0])
	}
	for _, a := range c.Argv {
		if strings.ContainsRune(a, 0) {
			return errors.New("exec check: NUL in argv")
		}
	}
	if c.Timeout < 0 || c.Timeout > MaxExecTimeout {
		return fmt.Errorf("exec check: timeout %v is not within 0-%v", c.Timeout, MaxExecTimeout)
	}
	if c.Interval < 0 {
		return fmt.Errorf("exec check: interval %v is negative", c.Interval)
	}
	for k, v := range c.Env {
		if !envName.MatchString(k) {
			return fmt.Errorf("exec check: env name %q is not [A-Za-z_][A-Za-z0-9_]*", k)
		}
		if strings.ContainsRune(v, 0) {
			return fmt.Errorf("exec check: NUL in env %s", k)
		}
	}
	return nil
}

func (c ExecCheck) timeout() time.Duration {
	if c.Timeout <= 0 {
		return DefaultExecTimeout
	}
	return c.Timeout
}

func (c ExecCheck) environ() []string {
	env := map[string]string{"PATH": execPath, "LANG": "C.UTF-8"}
	for k, v := range c.Env {
		env[k] = v
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// ExecResult is one run of an ExecCheck.
type ExecResult struct {
	ExitCode  int           // -1 when the command did not exit on its own
	Output    []byte        // last ExecOutputCap bytes of stdout+stderr
	Truncated bool          // output was longer than ExecOutputCap
	TimedOut  bool          // killed at the timeout
	Err       error         // could not run, or did not exit 0
	Duration  time.Duration // wall time
}

// Healthy is the vote: exit 0, on time, nothing else wrong.
func (r ExecResult) Healthy() bool {
	return r.Err == nil && r.ExitCode == 0 && !r.TimedOut
}

// summary is the failing report's Error: the cause and the output's tail.
func (r ExecResult) summary() string {
	var cause string
	switch {
	case r.TimedOut:
		cause = "timed out"
	case r.ExitCode > 0:
		cause = fmt.Sprintf("exit %d", r.ExitCode)
	case r.Err != nil:
		cause = r.Err.Error()
	default:
		cause = "failed"
	}
	out := strings.TrimSpace(string(r.Output))
	if len(out) > execErrorTail {
		out = "..." + out[len(out)-execErrorTail:]
	}
	if out == "" {
		return cause
	}
	return cause + ": " + out
}

// tailBuffer keeps the last n bytes written to it. It never returns an
// error, so a chatty check is not killed by a write failure.
type tailBuffer struct {
	mu        sync.Mutex
	n         int
	buf       []byte
	truncated bool
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.n; over > 0 {
		b.buf = append(b.buf[:0], b.buf[over:]...)
		b.truncated = true
	}
	return len(p), nil
}

func (b *tailBuffer) bytes() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf...), b.truncated
}

// RunExec runs the check once. It never starts a shell.
func RunExec(ctx context.Context, c ExecCheck) ExecResult {
	if err := c.Validate(); err != nil {
		return ExecResult{ExitCode: -1, Err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()

	out := &tailBuffer{n: ExecOutputCap}
	// #nosec G204 -- argv comes from Akira's operator-owned config, is validated (absolute argv[0]) and is never passed through a shell
	cmd := exec.CommandContext(ctx, c.Argv[0], c.Argv[1:]...)
	cmd.Env = c.environ()
	cmd.Dir = "/"
	cmd.Stdin = nil
	cmd.Stdout = out
	cmd.Stderr = out
	killProcessGroup(cmd)
	// Children that outlive the kill (or keep the pipes open) are not waited
	// on beyond this.
	cmd.WaitDelay = 2 * time.Second

	start := time.Now()
	err := cmd.Run()
	res := ExecResult{Duration: time.Since(start), ExitCode: -1}
	res.Output, res.Truncated = out.bytes()
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.Err = fmt.Errorf("exec check: killed after %v", c.timeout())
		return res
	}
	if err != nil {
		res.Err = err
	}
	return res
}
