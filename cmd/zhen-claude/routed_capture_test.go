// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// S-CLEAN-SLATE M-042: a routed turn that resolves on the local model
// (`-route local -family -worktree`, or `-route auto` whose local draft
// passes) is written as one redacted zhen-capture record, route=local, with
// the family, so the data trigger counts it. Claude text is never part of it.

func routedAs(args []string, route string) []string {
	for i, a := range args {
		if a == "auto" && i > 0 && args[i-1] == "-route" {
			args[i] = route
		}
	}
	return args
}

func TestRouted_LocalTurnCaptured(t *testing.T) {
	for _, route := range []string{"local", "auto"} {
		t.Run(route, func(t *testing.T) {
			agent, wt := agentFixture(t)
			llama, _ := fakeLlamaSeq(t, "Here:\n```file:lib.py\n"+fixedLib+"```")
			claude := newFakeClaudeTools(t, filepath.Join(wt, "lib.py"))
			noCredentials(t)
			t.Setenv("ANTHROPIC_BASE_URL", claude.srv.URL)
			t.Setenv("ANTHROPIC_API_KEY", "test-not-a-key")
			dir := filepath.Join(t.TempDir(), "capture")
			args := routedAs(routedArgs(t, agent, wt, llama.URL, "-capture", dir), route)
			var out, errb bytes.Buffer
			if code := run(args, strings.NewReader(""), &out, &errb); code != 0 {
				t.Fatalf("exit %d: %s", code, errb.String())
			}
			recs := readCapture(t, dir)
			if len(recs) != 1 {
				t.Fatalf("%d capture records, want 1: %v", len(recs), recs)
			}
			r := recs[0]
			if r["route"] != "local" || r["task_family"] != "repo-task" || r["accepted_by"] != "local" || r["source"] != "zhen-claude" {
				t.Errorf("record = %v", r)
			}
			if r["prompt"] != "double(x) must return 2*x." {
				t.Errorf("prompt = %q, want the task as given", r["prompt"])
			}
			local, _ := r["local"].(map[string]any)
			if local == nil || !strings.Contains(fmt.Sprint(local["answer"]), "return 2 * x") {
				t.Errorf("local leg = %v", local)
			}
			if !strings.Contains(fmt.Sprint(r["final"]), "return 2 * x") {
				t.Errorf("final = %v", r["final"])
			}
			if _, ok := r["claude"]; ok || claude.hits != 0 {
				t.Errorf("claude leg %v, claude hits %d", r["claude"], claude.hits)
			}
		})
	}
}

// A local-only turn whose draft fails the check is still captured (the
// failing draft is a training negative), and not accepted.
func TestRouted_LocalFailureCapturedNotAccepted(t *testing.T) {
	agent, wt := agentFixture(t)
	llama, _ := fakeLlamaSeq(t, "```file:lib.py\n"+brokenLib+"```")
	noCredentials(t)
	dir := filepath.Join(t.TempDir(), "capture")
	args := routedAs(routedArgs(t, agent, wt, llama.URL, "-capture", dir), "local")
	var out, errb bytes.Buffer
	_ = run(args, strings.NewReader(""), &out, &errb)
	recs := readCapture(t, dir)
	if len(recs) != 1 {
		t.Fatalf("%d capture records, want 1", len(recs))
	}
	r := recs[0]
	if r["route"] != "local" || r["accepted_by"] != "" || r["final"] != "" {
		t.Errorf("record = %v", r)
	}
	if local, _ := r["local"].(map[string]any); local == nil || !strings.Contains(fmt.Sprint(local["answer"]), "x + x + 1") {
		t.Errorf("local leg = %v, want the failing draft", r["local"])
	}
}

// An escalated routed turn writes no capture record from this path: the
// Claude leg's text is never captured here.
func TestRouted_EscalatedTurnNotCaptured(t *testing.T) {
	agent, wt := agentFixture(t)
	llama, _ := fakeLlamaSeq(t, "```file:lib.py\n"+brokenLib+"```")
	claude := newFakeClaudeTools(t, filepath.Join(wt, "lib.py"))
	noCredentials(t)
	t.Setenv("ANTHROPIC_BASE_URL", claude.srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "test-not-a-key")
	dir := filepath.Join(t.TempDir(), "capture")
	var out, errb bytes.Buffer
	if code := run(routedArgs(t, agent, wt, llama.URL, "-capture", dir), strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if recs := readCapture(t, dir); len(recs) != 0 {
		t.Errorf("escalated turn captured: %v", recs)
	}
}
