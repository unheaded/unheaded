// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"unheaded/pkg/sandbox"
)

const fixedLib = "def double(x):\n    return 2 * x\n"
const brokenLib = "def double(x):\n    return x + x + 1\n"

// agentFixture: a zhen/<name> worktree under an agent root holding a tiny
// Python fixture whose check passes only with fixedLib.
func agentFixture(t *testing.T) (agent, wt string) {
	t.Helper()
	agent = t.TempDir()
	wt = filepath.Join(agent, "eval-double")
	if err := os.MkdirAll(wt, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"lib.py":      brokenLib,
		"test_lib.py": "import unittest\nfrom lib import double\n\nclass T(unittest.TestCase):\n    def test(self):\n        self.assertEqual(double(3), 6)\n\nif __name__ == '__main__':\n    unittest.main()\n",
		"check.sh":    "#!/usr/bin/env bash\nset -euo pipefail\ncd \"$(dirname \"$0\")\"\npython3 -B -m unittest -q 2>&1\n",
		"TASK.md":     "double(x) must return 2*x.\n",
	}
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(wt, n), []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "zhen/eval-double"}, {"config", "user.email", "t@example.invalid"}, {"config", "user.name", "t"},
		{"add", "."}, {"commit", "-q", "--no-gpg-sign", "-m", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", wt}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
	}
	return agent, wt
}

// directRun runs the check without systemd (the sandbox has its own tests).
func directRun(ctx context.Context, cfg sandbox.Config, argv []string) (sandbox.Result, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = cfg.Dir
	out, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok {
		return sandbox.Result{Exit: ee.ExitCode(), Output: string(out)}, nil
	}
	if err != nil {
		return sandbox.Result{Exit: -1}, err
	}
	return sandbox.Result{Exit: 0, Output: string(out)}, nil
}

// fakeLlamaSeq answers chat completions from a list.
func fakeLlamaSeq(t *testing.T, answers ...string) (*httptest.Server, *int) {
	t.Helper()
	var mu sync.Mutex
	n := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		mu.Lock()
		a := answers[min(n, len(answers)-1)]
		n++
		mu.Unlock()
		b, _ := json.Marshal(map[string]any{"model": "qwen.gguf", "choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": a}}},
			"usage": map[string]any{"prompt_tokens": 50, "completion_tokens": 20}})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	t.Cleanup(s.Close)
	return s, &n
}

// fakeClaudeTools streams a write_file tool call for path, then end_turn.
type fakeClaudeTools struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits int
	path string
}

func newFakeClaudeTools(t *testing.T, path string) *fakeClaudeTools {
	t.Helper()
	f := &fakeClaudeTools{path: path}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		f.mu.Lock()
		f.hits++
		n := f.hits
		f.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		ev := func(name string, data any) {
			b, _ := json.Marshal(data)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
		}
		ev("message_start", map[string]any{"type": "message_start", "message": map[string]any{
			"id": fmt.Sprintf("msg_%d", n), "type": "message", "role": "assistant", "model": "claude-opus-5-5",
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 100, "output_tokens": 1}}})
		stop := "end_turn"
		if n == 1 {
			in, _ := json.Marshal(map[string]string{"path": f.path, "content": fixedLib})
			ev("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "tool_use", "id": "toolu_1", "name": "write_file", "input": map[string]any{}}})
			ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(in)}})
			stop = "tool_use"
		} else {
			ev("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
			ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": "fixed"}})
		}
		ev("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		ev("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": stop, "stop_sequence": nil},
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 50, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}})
		ev("message_stop", map[string]any{"type": "message_stop"})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func routingFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "routing.yaml")
	doc := "version: 1\nfamilies:\n  repo-task: {route: local_first, local_attempts: 2, escalate_on: [check_fail, parse_fail], max_claude_usd: 0.25}\ndefault: {route: claude_direct, max_claude_usd: 0.25}\n"
	if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func routedArgs(t *testing.T, agent, wt, llama string, extra ...string) []string {
	t.Helper()
	prev := routedSandboxRun
	routedSandboxRun = directRun
	t.Cleanup(func() { routedSandboxRun = prev })
	return append(append(baseArgs(t), "-route", "auto", "-family", "repo-task", "-worktree", wt, "-agent-root", agent,
		"-run-check", "fixture-check", "-routing", routingFile(t), "-checks", "../../configs/zhen-checks.yaml",
		"-local-url", llama, "-effort", "low", "-p", "double(x) must return 2*x."), extra...)
}

func outcome(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &m); err != nil {
		t.Fatalf("stdout %q: %v", stdout, err)
	}
	return m
}

func TestRouted_LocalFailsClaudeFixes(t *testing.T) {
	agent, wt := agentFixture(t)
	llama, calls := fakeLlamaSeq(t, "```file:lib.py\n"+brokenLib+"```")
	claude := newFakeClaudeTools(t, filepath.Join(wt, "lib.py"))
	noCredentials(t)
	t.Setenv("ANTHROPIC_BASE_URL", claude.srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "test-not-a-key")
	var out, errb bytes.Buffer
	code := run(routedArgs(t, agent, wt, llama.URL), strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out.String(), errb.String())
	}
	m := outcome(t, out.String())
	if m["check"] != "pass" || m["escalated"] != true || m["route_reason"] != "escalated_check_fail" || m["local_attempts"] != float64(2) {
		t.Errorf("outcome = %v", m)
	}
	if *calls != 2 || claude.hits != 2 {
		t.Errorf("local calls %d, claude requests %d", *calls, claude.hits)
	}
	if b, _ := os.ReadFile(filepath.Join(wt, "lib.py")); string(b) != fixedLib {
		t.Errorf("lib.py = %q", b)
	}
}

func TestRouted_LocalPassesClaudeNeverCalled(t *testing.T) {
	agent, wt := agentFixture(t)
	llama, _ := fakeLlamaSeq(t, "Here:\n```file:lib.py\n"+fixedLib+"```")
	claude := newFakeClaudeTools(t, filepath.Join(wt, "lib.py"))
	noCredentials(t)
	t.Setenv("ANTHROPIC_BASE_URL", claude.srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "test-not-a-key")
	var out, errb bytes.Buffer
	code := run(routedArgs(t, agent, wt, llama.URL), strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	m := outcome(t, out.String())
	if m["check"] != "pass" || m["escalated"] != false || m["route_reason"] != "local_first" || claude.hits != 0 {
		t.Errorf("outcome = %v, claude hits %d", m, claude.hits)
	}
}

func TestRouted_LocalRouteNeverCallsClaude(t *testing.T) {
	agent, wt := agentFixture(t)
	llama, _ := fakeLlamaSeq(t, "```file:lib.py\n"+brokenLib+"```")
	claude := newFakeClaudeTools(t, filepath.Join(wt, "lib.py"))
	noCredentials(t)
	t.Setenv("ANTHROPIC_BASE_URL", claude.srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "test-not-a-key")
	args := routedArgs(t, agent, wt, llama.URL)
	for i, a := range args {
		if a == "auto" && args[i-1] == "-route" {
			args[i] = "local"
		}
	}
	var out, errb bytes.Buffer
	if code := run(args, strings.NewReader(""), &out, &errb); code != 1 {
		t.Errorf("exit %d: %s", code, errb.String())
	}
	if m := outcome(t, out.String()); m["check"] != "fail" || m["stop"] != "local_only" || claude.hits != 0 {
		t.Errorf("outcome = %v, claude hits %d", m, claude.hits)
	}
}

func TestRouted_ProtectedFileEditFails(t *testing.T) {
	agent, wt := agentFixture(t)
	// The draft tries to replace check.sh with one that always passes.
	llama, _ := fakeLlamaSeq(t, "```file:check.sh\nexit 0\n```")
	noCredentials(t)
	var out, errb bytes.Buffer
	args := routedArgs(t, agent, wt, llama.URL)
	for i, a := range args {
		if a == "auto" && args[i-1] == "-route" {
			args[i] = "local"
		}
	}
	_ = run(args, strings.NewReader(""), &out, &errb)
	if b, _ := os.ReadFile(filepath.Join(wt, "check.sh")); strings.TrimSpace(string(b)) == "exit 0" {
		t.Fatal("a draft replaced check.sh")
	}
	if m := outcome(t, out.String()); m["check"] == "pass" {
		t.Errorf("outcome = %v", m)
	}
}

func TestRouted_NotAnAgentWorktreeRefused(t *testing.T) {
	noCredentials(t)
	dir := t.TempDir()
	var out, errb bytes.Buffer
	code := run(append(baseArgs(t), "-route", "auto", "-family", "repo-task", "-worktree", dir, "-agent-root", t.TempDir(),
		"-routing", routingFile(t), "-p", "x"), strings.NewReader(""), &out, &errb)
	if code != 2 || !strings.Contains(errb.String(), "agent worktree") {
		t.Errorf("exit %d: %s", code, errb.String())
	}
}

func TestRouted_FlagValidation(t *testing.T) {
	noCredentials(t)
	for _, args := range [][]string{
		{"-family", "x", "-p", "q"},                                            // -route claude
		{"-route", "local", "-p", "q"},                                         // local without family
		{"-worktree", "/tmp", "-p", "q"},                                       // worktree without family
		{"-route", "auto", "-family", "x", "-run-check", "go-test", "-p", "q"}, // check without worktree
	} {
		var out, errb bytes.Buffer
		if code := run(append(baseArgs(t), args...), strings.NewReader(""), &out, &errb); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}
