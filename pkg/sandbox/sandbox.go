// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package sandbox runs a check over model-written code in a transient
// systemd user unit: no network, the home directory hidden, a private
// /tmp, only the given directory (and caches) writable, memory, task and
// time limits. No shell is involved: the argv is passed to systemd-run
// as is. It needs a user systemd session (`systemd-run --user`); it does
// not need root (S-TWO-BRAINS Steps 21, 137, 161-170).
package sandbox

import (
	"bytes"
	"context"
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

// DefaultHidden are host paths a check never sees, on top of the hidden
// home: the Kingdom's service config holds credentials, and on west some
// of it is world-readable (S-TWO-BRAINS Step 169 finding). Missing paths
// are ignored.
var DefaultHidden = []string{"/etc/unheaded", "/var/lib/unheaded", "/srv/backups"}

// Defaults.
const (
	DefaultMemoryMax   = "2G"
	DefaultRuntimeMax  = 600 * time.Second
	DefaultTasksMax    = 256
	DefaultOutputLimit = 16 << 10
)

// Config is one run's confinement.
type Config struct {
	// Dir is the working directory and the main writable path. It must
	// be absolute and exist.
	Dir string
	// Writable are extra writable paths (build caches). Absolute.
	Writable []string
	// ReadOnly are paths made visible read-only (a toolchain or module
	// cache under the hidden home). Absolute.
	ReadOnly []string
	// Hidden are paths made inaccessible. nil: DefaultHidden.
	Hidden []string
	// Env is the environment, KEY=VALUE, passed with --setenv. Nothing
	// else of the caller's environment reaches the unit.
	Env []string
	// MemoryMax (systemd syntax, e.g. "2G"), RuntimeMax and TasksMax
	// default to the constants above.
	MemoryMax  string
	RuntimeMax time.Duration
	TasksMax   int
	// OutputLimit cuts combined stdout+stderr. Default 16 KiB.
	OutputLimit int
	// SystemdRun is the systemd-run binary. Default: from PATH.
	SystemdRun string
}

// Result is what the check did.
type Result struct {
	Exit      int // the unit's exit status; -1 if it did not run to an exit
	Output    string
	Truncated bool
	Duration  time.Duration
}

var (
	envKeyRE = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	memRE    = regexp.MustCompile(`^[0-9]+[KMGT]?$`)
)

// Args returns the systemd-run argv for cfg and the command argv (for
// tests and logs; no secrets are ever part of it).
func Args(cfg Config, argv []string) ([]string, error) {
	if len(argv) == 0 {
		return nil, errors.New("sandbox: empty command")
	}
	if !filepath.IsAbs(cfg.Dir) {
		return nil, fmt.Errorf("sandbox: dir %q is not absolute", cfg.Dir)
	}
	if st, err := os.Stat(cfg.Dir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("sandbox: dir %q does not exist", cfg.Dir)
	}
	mem := cfg.MemoryMax
	if mem == "" {
		mem = DefaultMemoryMax
	}
	if !memRE.MatchString(mem) {
		return nil, fmt.Errorf("sandbox: MemoryMax %q", mem)
	}
	rt := cfg.RuntimeMax
	if rt <= 0 {
		rt = DefaultRuntimeMax
	}
	tasks := cfg.TasksMax
	if tasks <= 0 {
		tasks = DefaultTasksMax
	}
	args := []string{
		"--user", "--wait", "--pipe", "--quiet", "--collect",
		"-p", "PrivateNetwork=yes",
		"-p", "ProtectHome=tmpfs",
		"-p", "PrivateTmp=yes",
		"-p", "NoNewPrivileges=yes",
		"-p", "MemoryMax=" + mem,
		"-p", "MemorySwapMax=0",
		"-p", "RuntimeMaxSec=" + strconv.Itoa(int(rt.Seconds())),
		"-p", "TasksMax=" + strconv.Itoa(tasks),
		"-p", "BindPaths=" + cfg.Dir,
		"-p", "WorkingDirectory=" + cfg.Dir,
	}
	for _, w := range cfg.Writable {
		if !filepath.IsAbs(w) || strings.ContainsAny(w, " \t\n:") {
			return nil, fmt.Errorf("sandbox: writable path %q", w)
		}
		args = append(args, "-p", "BindPaths="+w)
	}
	hidden := cfg.Hidden
	if hidden == nil {
		hidden = DefaultHidden
	}
	for _, h := range hidden {
		if !filepath.IsAbs(h) || strings.ContainsAny(h, " \t\n:") {
			return nil, fmt.Errorf("sandbox: hidden path %q", h)
		}
		args = append(args, "-p", "InaccessiblePaths=-"+h)
	}
	for _, r := range cfg.ReadOnly {
		if !filepath.IsAbs(r) || strings.ContainsAny(r, " \t\n:") {
			return nil, fmt.Errorf("sandbox: read-only path %q", r)
		}
		args = append(args, "-p", "BindReadOnlyPaths="+r)
	}
	for _, kv := range cfg.Env {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || !envKeyRE.MatchString(k) || strings.ContainsAny(kv, "\n\x00") {
			return nil, fmt.Errorf("sandbox: env entry %q", kv)
		}
		args = append(args, "--setenv="+kv)
	}
	if strings.ContainsAny(cfg.Dir, " \t\n:") {
		return nil, fmt.Errorf("sandbox: dir %q has characters systemd-run cannot bind", cfg.Dir)
	}
	args = append(args, "--")
	return append(args, argv...), nil
}

// limitedBuffer keeps the first n bytes and counts the rest.
type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	n         int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.n - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
			b.truncated = true
		} else {
			b.buf.Write(p)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

// Run runs argv confined by cfg. A non-zero exit is a Result, not an
// error; the error is for when the sandbox itself could not run.
func Run(ctx context.Context, cfg Config, argv []string) (Result, error) {
	args, err := Args(cfg, argv)
	if err != nil {
		return Result{Exit: -1}, err
	}
	bin := cfg.SystemdRun
	if bin == "" {
		if bin, err = exec.LookPath("systemd-run"); err != nil {
			return Result{Exit: -1}, errors.New("sandbox: systemd-run not found")
		}
	}
	limit := cfg.OutputLimit
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	out := &limitedBuffer{n: limit}
	// #nosec G204 -- fixed binary; argv built by Args from validated parts, no shell
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.Stdin = nil
	cmd.WaitDelay = 5 * time.Second
	t0 := time.Now()
	runErr := cmd.Run()
	res := Result{Exit: 0, Duration: time.Since(t0)}
	out.mu.Lock()
	res.Output, res.Truncated = out.buf.String(), out.truncated
	out.mu.Unlock()
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) && ctx.Err() == nil {
			res.Exit = ee.ExitCode()
			return res, nil
		}
		res.Exit = -1
		if ctx.Err() != nil {
			return res, fmt.Errorf("sandbox: %w", ctx.Err())
		}
		return res, fmt.Errorf("sandbox: %w", runErr)
	}
	return res, nil
}

// Available reports whether a user systemd session can start units.
func Available(ctx context.Context) error {
	dir, err := os.MkdirTemp("", "sbx-probe-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r, err := Run(ctx, Config{Dir: dir, RuntimeMax: 20 * time.Second}, []string{"/bin/true"})
	if err != nil {
		return err
	}
	if r.Exit != 0 {
		return fmt.Errorf("sandbox: probe exited %d: %s", r.Exit, strings.TrimSpace(r.Output))
	}
	return nil
}
