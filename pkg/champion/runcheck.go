// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
	"unheaded/pkg/sandbox"
)

// checkRunsTotal counts run_check calls by allowlisted key and result
// (pass, fail, error, refused); unknown keys are counted as "unknown".
var checkRunsTotal = auto.NewCounterVec(prom.CounterOpts{
	Name: "zhen_check_runs_total",
	Help: "run_check calls by check and result.",
}, []string{"check", "result"})

// run_check (S-TWO-BRAINS Step 161): a model may ask for a test to run,
// but never supplies a command. It names a key from configs/zhen-checks.yaml
// (argv templates) and a path inside an agent worktree; the check runs in
// pkg/sandbox (no network, home hidden, only the worktree and caches
// writable, limits). Rule 2 treats it as read-only: it changes nothing
// outside the throwaway worktree. Rule 3 still scans its arguments.

// MaxCheckRuntime caps every check, whatever its config says.
const MaxCheckRuntime = 600 * time.Second

// CheckSpec is one allowlisted check.
type CheckSpec struct {
	Argv         []string      `yaml:"argv"`
	Timeout      time.Duration `yaml:"-"`
	TimeoutRaw   string        `yaml:"timeout"`
	FailOnOutput bool          `yaml:"fail_on_output"`
}

// Checks is the loaded allowlist.
type Checks struct {
	Specs map[string]CheckSpec
}

var (
	checkKeyRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	crateRE     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	shellMetaRE = regexp.MustCompile("[;|&$`<>(){}*?!~\\\\'\"\\s]")
	placeholder = regexp.MustCompile(`\{[a-z]+\}`)
)

// LoadChecks reads and validates the allowlist: argv templates only, no
// shell metacharacters, and only the {path} and {crate} placeholders.
func LoadChecks(path string) (*Checks, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-chosen config file
	if err != nil {
		return nil, err
	}
	return ParseChecks(raw)
}

// ParseChecks validates a checks document.
func ParseChecks(raw []byte) (*Checks, error) {
	var doc struct {
		Version int                  `yaml:"version"`
		Checks  map[string]CheckSpec `yaml:"checks"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("checks: %w", err)
	}
	if doc.Version != 1 || len(doc.Checks) == 0 {
		return nil, errors.New("checks: want version 1 and at least one check")
	}
	out := &Checks{Specs: map[string]CheckSpec{}}
	for key, spec := range doc.Checks {
		if !checkKeyRE.MatchString(key) {
			return nil, fmt.Errorf("checks: bad key %q", key)
		}
		if len(spec.Argv) == 0 || placeholder.MatchString(spec.Argv[0]) {
			return nil, fmt.Errorf("checks: %s: argv must start with a fixed program", key)
		}
		for _, a := range spec.Argv {
			for _, ph := range placeholder.FindAllString(a, -1) {
				if ph != "{path}" && ph != "{crate}" {
					return nil, fmt.Errorf("checks: %s: placeholder %s not allowed", key, ph)
				}
			}
			if shellMetaRE.MatchString(placeholder.ReplaceAllString(a, "")) {
				return nil, fmt.Errorf("checks: %s: argument %q has shell metacharacters", key, a)
			}
		}
		spec.Timeout = MaxCheckRuntime
		if spec.TimeoutRaw != "" {
			d, err := time.ParseDuration(spec.TimeoutRaw)
			if err != nil || d <= 0 {
				return nil, fmt.Errorf("checks: %s: timeout %q", key, spec.TimeoutRaw)
			}
			spec.Timeout = min(d, MaxCheckRuntime)
		}
		out.Specs[key] = spec
	}
	return out, nil
}

// RunCheckConfig turns run_check on for a Champion.
type RunCheckConfig struct {
	Checks *Checks
	// AgentWorktreeRoot: run_check runs only when the Champion's project
	// root is a worktree under it (never the main checkout).
	AgentWorktreeRoot string
	// Env, ReadOnly, Writable go to the sandbox (see GoSandbox).
	Env      []string
	ReadOnly []string
	Writable []string
	// Run is the sandbox runner. Default sandbox.Run.
	Run func(ctx context.Context, cfg sandbox.Config, argv []string) (sandbox.Result, error)
}

// WithRunCheck enables the run_check tool.
func (c *Champion) WithRunCheck(cfg RunCheckConfig) *Champion {
	c.runCheck = &cfg
	return c
}

// RunCheckResult is a check's outcome, for the model and the router.
type RunCheckResult struct {
	Check      string `json:"check"`
	Path       string `json:"path"`
	Pass       bool   `json:"pass"`
	Exit       int    `json:"exit"`
	Output     string `json:"output"`
	Truncated  bool   `json:"truncated"`
	DurationMS int64  `json:"duration_ms"`
}

// ErrRunCheckOff: run_check is not configured for this Champion.
var ErrRunCheckOff = errors.New("run_check is not enabled for this Champion")

// worktreeOf checks that the Champion's root is an agent worktree: a real
// directory (no symlink in its path) strictly under the agent root.
func (c *Champion) worktreeOf() (string, error) {
	rc := c.runCheck
	if rc == nil || rc.Checks == nil || rc.AgentWorktreeRoot == "" {
		return "", ErrRunCheckOff
	}
	root, err := filepath.Abs(c.config.ProjectRoot)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	if resolved != filepath.Clean(root) {
		return "", fmt.Errorf("project root %s is reached through a symlink", root)
	}
	agent, err := filepath.EvalSymlinks(rc.AgentWorktreeRoot)
	if err != nil {
		return "", fmt.Errorf("agent worktree root: %w", err)
	}
	if !strings.HasPrefix(resolved, agent+string(filepath.Separator)) {
		return "", fmt.Errorf("project root %s is not an agent worktree under %s (checks never run in the main checkout)", resolved, agent)
	}
	return resolved, nil
}

// RunCheck runs one allowlisted check on path inside the worktree.
func (c *Champion) RunCheck(ctx context.Context, key, path, crate string) (RunCheckResult, error) {
	start := time.Now()
	res := RunCheckResult{Check: key, Path: path, Exit: -1}
	actionID, _ := c.logAction(ctx, "run_check", fmt.Sprintf("run_check %s %s", key, path), "user")
	label := "unknown"
	if c.runCheck != nil && c.runCheck.Checks != nil {
		if _, ok := c.runCheck.Checks.Specs[key]; ok {
			label = key
		}
	}
	fail := func(err error) (RunCheckResult, error) {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		result := "refused"
		if res.Exit != -1 || strings.HasPrefix(err.Error(), "check could not run") {
			result = "error"
		}
		checkRunsTotal.WithLabelValues(label, result).Inc()
		return res, err
	}
	wt, err := c.worktreeOf()
	if err != nil {
		return fail(err)
	}
	spec, ok := c.runCheck.Checks.Specs[key]
	if !ok {
		return fail(fmt.Errorf("check %q is not on the allowlist", key))
	}
	rel, err := c.checkPath(wt, path)
	if err != nil {
		return fail(err)
	}
	argv := make([]string, len(spec.Argv))
	for i, a := range spec.Argv {
		if strings.Contains(a, "{crate}") {
			if !crateRE.MatchString(crate) {
				return fail(fmt.Errorf("check %s needs a crate name matching %s", key, crateRE))
			}
			a = strings.ReplaceAll(a, "{crate}", crate)
		}
		argv[i] = strings.ReplaceAll(a, "{path}", rel)
	}
	if prog, err := exec.LookPath(argv[0]); err == nil && !strings.Contains(argv[0], "/") {
		argv[0] = prog
	}
	run := c.runCheck.Run
	if run == nil {
		run = sandbox.Run
	}
	ctx, cancel := context.WithTimeout(ctx, spec.Timeout+30*time.Second)
	defer cancel()
	// Tools that keep config or caches under $HOME (Go's telemetry
	// counters in ~/.config/go) write them to the unit's private /tmp, not
	// into the worktree, where they would end up in an agent commit.
	env := append([]string{"HOME=" + wt, "XDG_CONFIG_HOME=/tmp/.config", "XDG_CACHE_HOME=/tmp/.cache"}, c.runCheck.Env...)
	sr, err := run(ctx, sandbox.Config{
		Dir: wt, Writable: c.runCheck.Writable, ReadOnly: c.runCheck.ReadOnly, Env: env,
		RuntimeMax: spec.Timeout, MemoryMax: sandbox.DefaultMemoryMax, TasksMax: sandbox.DefaultTasksMax,
		OutputLimit: sandbox.DefaultOutputLimit,
	}, argv)
	res.Exit, res.Output, res.Truncated, res.DurationMS = sr.Exit, sr.Output, sr.Truncated, time.Since(start).Milliseconds()
	if err != nil {
		return fail(fmt.Errorf("check could not run: %w", err))
	}
	res.Pass = sr.Exit == 0 && (!spec.FailOnOutput || strings.TrimSpace(sr.Output) == "")
	result := "pass"
	if !res.Pass {
		result = "fail"
	}
	checkRunsTotal.WithLabelValues(label, result).Inc()
	// The check ran to completion either way; the verdict is the summary
	// (zhen-flywheel label reads it). zhen_actions.status allows only
	// planned/executing/completed/failed/reverted (009), and "failed" is
	// for a check that could not run.
	c.completeAction(ctx, actionID, "completed", fmt.Sprintf("exit=%d pass=%v", res.Exit, res.Pass), "", time.Since(start))
	return res, nil
}

// runCheckRulePath is the path Rule 1 judges for run_check: relative
// paths are relative to the worktree (not the process's cwd), and a
// trailing Go "/..." pattern is not part of the path.
func (c *Champion) runCheckRulePath(path string) string {
	p := strings.TrimSuffix(strings.TrimSuffix(path, "/..."), "...")
	if p == "" {
		p = "."
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(c.config.ProjectRoot, p)
	}
	return p
}

// checkPathRE: a check path is a plain path, nothing a shell or a flag
// parser could read as more than one.
var checkPathRE = regexp.MustCompile(`^[A-Za-z0-9._/+-]{1,512}$`)

// checkPath validates path (absolute, or relative to the worktree) and
// returns it relative to the worktree. A trailing Go "/..." pattern is
// allowed and kept; ".." anywhere else is refused (Rule 1).
func (c *Champion) checkPath(wt, path string) (string, error) {
	if path == "" {
		path = "."
	}
	if !checkPathRE.MatchString(path) || strings.HasPrefix(path, "-") {
		return "", fmt.Errorf("check path %q is not a plain path", path)
	}
	suffix := ""
	if path == "./..." || path == "..." {
		path, suffix = ".", "/..."
	} else if strings.HasSuffix(path, "/...") {
		path, suffix = strings.TrimSuffix(path, "/..."), "/..."
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(wt, path)
	}
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("path traversal blocked: %s", path)
	}
	if err := c.validatePath(abs); err != nil {
		return "", err
	}
	rel, err := filepath.Rel(wt, filepath.Clean(abs))
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %s is outside the worktree", path)
	}
	if rel == "." {
		return "." + suffix, nil
	}
	return "./" + rel + suffix, nil
}

// GoSandbox is the sandbox environment for Go checks: the host's Go
// toolchain and module cache read-only (they live under the hidden
// home), a dedicated writable build cache, no network fetches.
func GoSandbox(ctx context.Context, cacheDir string) (env, readOnly, writable []string, err error) {
	out, err := exec.CommandContext(ctx, "go", "env", "-json", "GOROOT", "GOMODCACHE").Output()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("go env: %w", err)
	}
	var ge struct{ GOROOT, GOMODCACHE string }
	if err := json.Unmarshal(out, &ge); err != nil || ge.GOROOT == "" {
		return nil, nil, nil, errors.New("go env: no GOROOT")
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, nil, nil, err
	}
	env = []string{
		"PATH=" + filepath.Join(ge.GOROOT, "bin") + ":/usr/local/bin:/usr/bin:/bin",
		"LANG=C.UTF-8", "TMPDIR=/tmp",
		"GOFLAGS=-mod=readonly", "GOPROXY=off", "GOSUMDB=off", "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOWORK=off",
		"GOCACHE=" + cacheDir,
	}
	readOnly = []string{ge.GOROOT}
	if ge.GOMODCACHE != "" {
		if _, err := os.Stat(ge.GOMODCACHE); err == nil {
			env = append(env, "GOMODCACHE="+ge.GOMODCACHE)
			readOnly = append(readOnly, ge.GOMODCACHE)
		}
	}
	return env, readOnly, []string{cacheDir}, nil
}
