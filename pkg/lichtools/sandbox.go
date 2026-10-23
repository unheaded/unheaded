// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Sandbox is the confined execution substrate. Nothing a driver asks for runs
// outside it. Spawn prepares confinement (and refuses an out-of-scope target or
// a target that will not fit in RAM); Exec runs one bounded command inside the
// confinement; Destroy reaps it. Implementations must be safe for concurrent
// use.
type Sandbox interface {
	Spawn(ctx context.Context, t Target, lim Limits) (*Handle, error)
	Exec(ctx context.Context, h *Handle, argv []string) (ExecResult, error)
	Destroy(h *Handle) error
}

// Errors the substrate returns. Callers test with errors.Is.
var (
	// ErrWontFit: the host does not have enough free RAM for this sandbox's
	// memory limit (× the safety factor). Refused rather than risking OOM.
	ErrWontFit = errors.New("lichtools: sandbox will not fit in available memory")
	// ErrNoConfinement: the unprivileged confinement tools are unavailable, so
	// no no-egress sandbox can be built. Fail closed — never run unconfined.
	ErrNoConfinement = errors.New("lichtools: no confinement substrate available (need unshare + user namespaces)")
	// ErrHalted: the kill switch has tripped; no new sandbox is spawned.
	ErrHalted = errors.New("lichtools: halted by kill switch")
)

// ExecResult is one bounded command run. Output is capped; Truncated says so.
type ExecResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	TimedOut  bool
	Truncated bool
	Duration  time.Duration
}

// Handle is one live sandbox. A netns sandbox holds the currently running
// process (if any) so Destroy and the kill switch can reap it.
type Handle struct {
	ID     string
	Target Target
	Limits Limits

	mu   sync.Mutex
	proc *os.Process // the Exec process-group leader, if running
	done bool
}

// maxExecOutputBytes caps each of stdout/stderr captured from a confined run.
const maxExecOutputBytes = 64 << 10

// netnsSandbox confines each command with an unprivileged user+net+pid+mount
// namespace (no egress — proven in preflight H8) and prlimit resource caps.
// It creates no persistent host state: no LXD container, no named netns, no
// nftables rule. Creating any of those would be an [ESCALATE]; this is the
// unprivileged fallback the sprint's SAFETY brief prefers.
type netnsSandbox struct {
	unshareBin string
	prlimitBin string
	// memAvailKB returns free RAM in KiB; injectable for tests.
	memAvailKB func() (int64, error)
	safety     float64
}

// NewNetnsSandbox builds the unprivileged sandbox. It returns ErrNoConfinement
// if unshare is unavailable, so a caller never silently runs unconfined.
func NewNetnsSandbox(memFitSafety float64) (Sandbox, error) {
	ub, err := exec.LookPath("unshare")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoConfinement, err)
	}
	pb, _ := exec.LookPath("prlimit") // optional; limits degrade gracefully if absent
	if memFitSafety < 1.0 {
		memFitSafety = DefaultMemFitSafety
	}
	return &netnsSandbox{
		unshareBin: ub,
		prlimitBin: pb,
		memAvailKB: readMemAvailableKB,
		safety:     memFitSafety,
	}, nil
}

// Spawn validates the target has passed scope (caller must have validated;
// this is defense in depth: Spawn re-checks nothing it does not own, but it
// does run the memory fit preflight) and reserves a handle.
func (s *netnsSandbox) Spawn(_ context.Context, t Target, lim Limits) (*Handle, error) {
	lim.applyDefaults()
	if err := s.fits(lim.MemBytes); err != nil {
		return nil, err
	}
	return &Handle{ID: newHandleID(t), Target: t, Limits: lim}, nil
}

// fits refuses when free RAM is below MemBytes × safety. Swap is never counted.
func (s *netnsSandbox) fits(memBytes int64) error {
	availKB, err := s.memAvailKB()
	if err != nil {
		return fmt.Errorf("lichtools: cannot read available memory: %w", err)
	}
	need := int64(float64(memBytes) * s.safety)
	if availKB*1024 < need {
		return fmt.Errorf("%w: need %d bytes (%.1f×), have %d bytes free", ErrWontFit, need, s.safety, availKB*1024)
	}
	return nil
}

// Exec runs argv inside the confinement with a wall-clock timeout and bounded
// output. The process runs in its own group so Destroy / the kill switch can
// reap the whole tree.
func (s *netnsSandbox) Exec(ctx context.Context, h *Handle, argv []string) (ExecResult, error) {
	if h == nil {
		return ExecResult{}, errors.New("lichtools: nil handle")
	}
	if len(argv) == 0 {
		return ExecResult{}, errors.New("lichtools: empty argv")
	}
	h.mu.Lock()
	if h.done {
		h.mu.Unlock()
		return ExecResult{}, errors.New("lichtools: handle destroyed")
	}
	h.mu.Unlock()

	timeout := h.Limits.WallTimeout
	if timeout <= 0 {
		timeout = DefaultWallTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	full := s.confineArgv(h.Limits, argv)
	cmd := exec.CommandContext(cctx, full[0], full[1:]...) // #nosec G204 -- argv is server-built: unshare/prlimit wrapper + caller argv; caller argv is scope-gated
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Kill the whole process group, not just the leader.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 5 * time.Second

	outBuf := &cappedBuffer{cap: maxExecOutputBytes}
	errBuf := &cappedBuffer{cap: maxExecOutputBytes}
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf

	start := time.Now()
	runErr := cmd.Start()
	if runErr != nil {
		return ExecResult{}, fmt.Errorf("lichtools: start confined command: %w", runErr)
	}
	h.mu.Lock()
	h.proc = cmd.Process
	h.mu.Unlock()

	waitErr := cmd.Wait()
	dur := time.Since(start)

	h.mu.Lock()
	h.proc = nil
	h.mu.Unlock()

	res := ExecResult{
		Stdout:    outBuf.String(),
		Stderr:    errBuf.String(),
		Truncated: outBuf.truncated || errBuf.truncated,
		Duration:  dur,
	}
	if errors.Is(cctx.Err(), context.DeadlineExceeded) {
		res.TimedOut = true
		res.ExitCode = -1
		return res, nil
	}
	if waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		return res, fmt.Errorf("lichtools: confined command: %w", waitErr)
	}
	res.ExitCode = 0
	return res, nil
}

// confineArgv wraps the caller argv in:
//
//	unshare -r -n --pid --mount-proc --fork -- [prlimit --nproc --as --cpu --] argv...
//
// -r maps the current user to root inside a new user namespace (no real
// privilege); -n is a fresh network namespace with no egress (only an unconfigured
// loopback — H8 proved this blocks the outside world); --pid/--mount-proc/--fork
// isolate the pid space; prlimit caps processes, address space, and CPU seconds.
func (s *netnsSandbox) confineArgv(lim Limits, argv []string) []string {
	out := []string{s.unshareBin, "-r", "-n", "--pid", "--mount-proc", "--fork", "--"}
	if s.prlimitBin != "" {
		cpuSecs := int(lim.WallTimeout/time.Second) + 5 // CPU-seconds backstop beyond the wall timeout
		if cpuSecs < 5 {
			cpuSecs = 5
		}
		out = append(out,
			s.prlimitBin,
			"--nproc="+strconv.Itoa(lim.PidsMax),
			"--as="+strconv.FormatInt(lim.MemBytes, 10),
			"--cpu="+strconv.Itoa(cpuSecs),
			"--",
		)
	}
	return append(out, argv...)
}

// Destroy reaps any running process group for the handle and marks it done.
func (s *netnsSandbox) Destroy(h *Handle) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.proc != nil {
		_ = syscall.Kill(-h.proc.Pid, syscall.SIGKILL)
		h.proc = nil
	}
	h.done = true
	return nil
}

// terminate asks a handle's process group to stop (SIGTERM); the kill switch
// escalates to Destroy after a grace window.
func (h *Handle) terminate() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.proc != nil {
		_ = syscall.Kill(-h.proc.Pid, syscall.SIGTERM)
	}
}

func newHandleID(t Target) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s%s-%s", DefaultSandboxPrefix, t.Name, hex.EncodeToString(b[:]))
}

func readMemAvailableKB() (int64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return strconv.ParseInt(fields[1], 10, 64)
			}
		}
	}
	return 0, errors.New("lichtools: MemAvailable not found in /proc/meminfo")
}

// cappedBuffer collects up to cap bytes, then drops the rest and flags it.
type cappedBuffer struct {
	cap       int
	buf       []byte
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if len(c.buf) >= c.cap {
		c.truncated = true
		return len(p), nil
	}
	room := c.cap - len(c.buf)
	if room >= len(p) {
		c.buf = append(c.buf, p...)
	} else {
		c.buf = append(c.buf, p[:room]...)
		c.truncated = true
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return string(c.buf) }
