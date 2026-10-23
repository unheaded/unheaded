// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/sandbox"
)

func TestParseChecks_RealConfig(t *testing.T) {
	c, err := LoadChecks("../../configs/zhen-checks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"go-test", "go-vet", "gofmt-l", "py-unittest", "bash-n", "shellcheck", "cargo-test", "runbook-check", "fixture-check"} {
		if _, ok := c.Specs[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	if c.Specs["go-test"].Timeout != 600*time.Second || !c.Specs["gofmt-l"].FailOnOutput {
		t.Errorf("specs = %+v", c.Specs)
	}
}

func TestParseChecks_Refuses(t *testing.T) {
	for name, doc := range map[string]string{
		"version":            "version: 2\nchecks: {a: {argv: [go]}}",
		"empty":              "version: 1\nchecks: {}",
		"bad key":            "version: 1\nchecks: {\"A B\": {argv: [go]}}",
		"placeholder first":  "version: 1\nchecks: {a: {argv: [\"{path}\"]}}",
		"other placeholder":  "version: 1\nchecks: {a: {argv: [go, \"{cmd}\"]}}",
		"semicolon":          "version: 1\nchecks: {a: {argv: [go, \"test;rm\"]}}",
		"dollar":             "version: 1\nchecks: {a: {argv: [sh, \"$HOME\"]}}",
		"backtick":           "version: 1\nchecks: {a: {argv: [sh, \"`id`\"]}}",
		"pipe":               "version: 1\nchecks: {a: {argv: [go, \"x|y\"]}}",
		"space in argument":  "version: 1\nchecks: {a: {argv: [sh, \"-c id\"]}}",
		"bad timeout":        "version: 1\nchecks: {a: {argv: [go], timeout: soon}}",
		"empty argv":         "version: 1\nchecks: {a: {argv: []}}",
		"not yaml":           "{{{",
		"redirect":           "version: 1\nchecks: {a: {argv: [go, \">/etc/x\"]}}",
		"glob":               "version: 1\nchecks: {a: {argv: [go, \"*.go\"]}}",
		"command subst {}":   "version: 1\nchecks: {a: {argv: [go, \"${x}\"]}}",
		"newline":            "version: 1\nchecks: {a: {argv: [go, \"a\\nb\"]}}",
		"placeholder in sub": "version: 1\nchecks: {a: {argv: [go, \"{path};id\"]}}",
	} {
		if _, err := ParseChecks([]byte(doc)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	c, err := ParseChecks([]byte("version: 1\nchecks: {a: {argv: [go, test, \"{path}\"], timeout: 2h}}"))
	if err != nil || c.Specs["a"].Timeout != MaxCheckRuntime {
		t.Errorf("timeout not capped: %+v %v", c, err)
	}
}

type fakeSandbox struct {
	cfg  sandbox.Config
	argv []string
	res  sandbox.Result
	err  error
	n    int
}

func (f *fakeSandbox) run(_ context.Context, cfg sandbox.Config, argv []string) (sandbox.Result, error) {
	f.cfg, f.argv, f.n = cfg, argv, f.n+1
	return f.res, f.err
}

// agentChampion is a Champion rooted at a worktree under an agent root.
func agentChampion(t *testing.T, fs *fakeSandbox) (*Champion, *mockStore, string, string) {
	t.Helper()
	agent := t.TempDir()
	wt := filepath.Join(agent, "zhen-card-1")
	if err := os.MkdirAll(filepath.Join(wt, "pkg", "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := &mockStore{}
	c, err := New(Config{ProjectRoot: wt}, store)
	if err != nil {
		t.Fatal(err)
	}
	checks, err := LoadChecks("../../configs/zhen-checks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.WithRunCheck(RunCheckConfig{Checks: checks, AgentWorktreeRoot: agent, Env: []string{"GOFLAGS=-mod=readonly"}, Run: fs.run})
	return c, store, wt, agent
}

func TestRunCheck_ArgvAndPass(t *testing.T) {
	fs := &fakeSandbox{res: sandbox.Result{Exit: 0, Output: "ok  pkg/x"}}
	c, store, wt, _ := agentChampion(t, fs)
	res, err := c.RunCheck(context.Background(), "go-test", "./pkg/x/...", "")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Pass || res.Exit != 0 || res.Output != "ok  pkg/x" {
		t.Errorf("res = %+v", res)
	}
	if strings.Join(fs.argv[1:], " ") != "test -count=1 ./pkg/x/..." || !strings.HasSuffix(fs.argv[0], "go") {
		t.Errorf("argv = %v", fs.argv)
	}
	if fs.cfg.Dir != wt || fs.cfg.RuntimeMax != 600*time.Second || fs.cfg.TasksMax != 256 || fs.cfg.MemoryMax != "2G" || fs.cfg.OutputLimit != 16<<10 {
		t.Errorf("sandbox cfg = %+v", fs.cfg)
	}
	if fs.cfg.Env[0] != "HOME="+wt {
		t.Errorf("env = %v", fs.cfg.Env)
	}
	// Tool config and caches (Go telemetry counters, ...) go to the unit's
	// private /tmp, never into the worktree a commit is made from (Step 218).
	env := strings.Join(fs.cfg.Env, " ")
	if !strings.Contains(env, "XDG_CONFIG_HOME=/tmp/") || !strings.Contains(env, "XDG_CACHE_HOME=/tmp/") || strings.Contains(env, "XDG_CONFIG_HOME="+wt) {
		t.Errorf("env = %v", fs.cfg.Env)
	}
	if len(store.actions) != 1 || store.actions[0].ActionType != "run_check" || store.actions[0].Status != "completed" {
		t.Errorf("actions = %+v", store.actions)
	}
}

func TestRunCheck_FailAndFailOnOutput(t *testing.T) {
	fs := &fakeSandbox{res: sandbox.Result{Exit: 1, Output: "--- FAIL"}}
	c, store, _, _ := agentChampion(t, fs)
	if res, err := c.RunCheck(context.Background(), "go-test", ".", ""); err != nil || res.Pass || res.Exit != 1 {
		t.Errorf("failing check: %+v %v", res, err)
	}
	// A failing check ran to completion: the verdict is in the summary.
	// The status must be one zhen_actions accepts (009's CHECK); the
	// earlier "check_failed" was refused by the Well, so the row stayed
	// 'planned' and the failure was never recorded.
	if a := store.actions[0]; a.Status != "completed" || a.ResultSummary != "exit=1 pass=false" || a.Error != "" {
		t.Errorf("action = status %q summary %q error %q", a.Status, a.ResultSummary, a.Error)
	}
	fs.res = sandbox.Result{Exit: 0, Output: "pkg/x/a.go\n"}
	if res, _ := c.RunCheck(context.Background(), "gofmt-l", "./pkg/x", ""); res.Pass {
		t.Error("gofmt -l listing a file passed")
	}
}

func TestRunCheck_Refusals(t *testing.T) {
	fs := &fakeSandbox{}
	c, _, wt, _ := agentChampion(t, fs)
	ctx := context.Background()
	for name, tc := range map[string]struct{ key, path, crate string }{
		"unknown check":   {"rm", ".", ""},
		"dotdot":          {"go-test", "../other", ""},
		"dotdot inside":   {"go-test", "./pkg/../..", ""},
		"absolute out":    {"go-test", "/etc", ""},
		"denied path":     {"bash-n", ".env", ""},
		"bad crate":       {"cargo-test", ".", "x;id"},
		"crate missing":   {"cargo-test", ".", ""},
		"abs outside wt":  {"go-test", filepath.Dir(wt), ""},
		"destructive arg": {"go-test", "rm -rf /", ""},
	} {
		if _, err := c.RunCheck(ctx, tc.key, tc.path, tc.crate); err == nil {
			t.Errorf("%s: ran", name)
		}
	}
	if fs.n != 0 {
		t.Errorf("the sandbox ran %d times for refused checks", fs.n)
	}
}

func TestRunCheck_MainCheckoutRefused(t *testing.T) {
	fs := &fakeSandbox{}
	store := &mockStore{}
	main := t.TempDir() // a checkout that is not under the agent root
	c, _ := New(Config{ProjectRoot: main}, store)
	checks, _ := LoadChecks("../../configs/zhen-checks.yaml")
	c.WithRunCheck(RunCheckConfig{Checks: checks, AgentWorktreeRoot: t.TempDir(), Run: fs.run})
	if _, err := c.RunCheck(context.Background(), "go-test", ".", ""); err == nil || !strings.Contains(err.Error(), "main checkout") {
		t.Errorf("main checkout: %v", err)
	}
	if fs.n != 0 {
		t.Error("ran in the main checkout")
	}
}

func TestRunCheck_SymlinkedRootRefused(t *testing.T) {
	fs := &fakeSandbox{}
	agent := t.TempDir()
	real := filepath.Join(agent, "real-wt")
	_ = os.MkdirAll(real, 0o700)
	link := filepath.Join(agent, "link-wt")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	c, _ := New(Config{ProjectRoot: link}, &mockStore{})
	checks, _ := LoadChecks("../../configs/zhen-checks.yaml")
	c.WithRunCheck(RunCheckConfig{Checks: checks, AgentWorktreeRoot: agent, Run: fs.run})
	if _, err := c.RunCheck(context.Background(), "go-test", ".", ""); err == nil {
		t.Error("a symlinked worktree root ran")
	}
}

func TestRunCheck_OffByDefault(t *testing.T) {
	c, _ := newTestChampion(t)
	if _, err := c.RunCheck(context.Background(), "go-test", ".", ""); !errors.Is(err, ErrRunCheckOff) {
		t.Errorf("err = %v", err)
	}
	p, _ := NewToolProvider(c, "test")
	tools, _ := p.Tools(context.Background())
	for _, tl := range tools {
		if tl.Name == "run_check" {
			t.Error("run_check listed without a configuration")
		}
	}
}

func TestRunCheck_ThroughDispatchIsReadOnly(t *testing.T) {
	fs := &fakeSandbox{res: sandbox.Result{Exit: 0}}
	c, _, _, _ := agentChampion(t, fs)
	// No justification: a mutating tool would be refused pending
	// confirmation; run_check is read-only, so it runs.
	out, err := c.Dispatch(context.Background(), ToolCall{Name: "run_check", Args: map[string]any{"check": "go-vet", "path": "./pkg/x"}, EmittedBy: "claude-api/m"})
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := out.(RunCheckResult); !ok || !r.Pass {
		t.Errorf("out = %+v", out)
	}
	p, _ := NewToolProvider(c, "test")
	tools, _ := p.Tools(context.Background())
	found := false
	for _, tl := range tools {
		found = found || tl.Name == "run_check"
	}
	if !found {
		t.Error("run_check not listed")
	}
}

func TestRunCheck_SandboxErrorIsAnError(t *testing.T) {
	fs := &fakeSandbox{err: errors.New("systemd-run not found"), res: sandbox.Result{Exit: -1}}
	c, _, _, _ := agentChampion(t, fs)
	if _, err := c.RunCheck(context.Background(), "go-test", ".", ""); err == nil {
		t.Error("a sandbox that could not start counted as a result")
	}
}

func TestRunCheck_Metrics(t *testing.T) {
	fs := &fakeSandbox{res: sandbox.Result{Exit: 0}}
	c, _, _, _ := agentChampion(t, fs)
	pass := checkRunsTotal.WithLabelValues("go-vet", "pass").Value()
	refused := checkRunsTotal.WithLabelValues("unknown", "refused").Value()
	_, _ = c.RunCheck(context.Background(), "go-vet", ".", "")
	_, _ = c.RunCheck(context.Background(), "made-up", ".", "")
	if checkRunsTotal.WithLabelValues("go-vet", "pass").Value() != pass+1 {
		t.Error("pass not counted")
	}
	if checkRunsTotal.WithLabelValues("unknown", "refused").Value() != refused+1 {
		t.Error("an unknown key was not counted as unknown/refused")
	}
}
