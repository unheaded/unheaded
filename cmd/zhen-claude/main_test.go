// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noCredentials(t *testing.T) {
	t.Helper()
	for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_PROFILE", "ANTHROPIC_FEDERATION_RULE_ID"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	t.Setenv("ANTHROPIC_CONFIG_DIR", t.TempDir())
	t.Setenv("ZHEN_CAPTURE_DIR", "") // never capture from a test unless it asks
}

func baseArgs(t *testing.T) []string {
	return []string{"-ledger", filepath.Join(t.TempDir(), "spend.json"), "-project-root", t.TempDir()}
}

func TestDryRunNeedsNoCredentialsAndSendsNothing(t *testing.T) {
	noCredentials(t)
	var out, errb bytes.Buffer
	code := run(append(baseArgs(t), "-dry-run", "-p", "hello bridge", "-max-tokens", "1024"), strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	s := out.String()
	if !strings.HasPrefix(s, "POST /v1/messages\nanthropic-beta: server-side-fallback-2026-07-01\n") {
		t.Fatalf("header: %q", s[:min(len(s), 120)])
	}
	body := s[strings.Index(s, "{"):]
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if m["model"] != "claude-opus-5-5" || m["max_tokens"] != float64(1024) || !strings.Contains(body, "hello bridge") {
		t.Errorf("body = %s", body)
	}
	if tools, _ := m["tools"].([]any); len(tools) != 3 {
		t.Errorf("champion tools = %v", m["tools"])
	}
}

func TestMissingCredentialsFailsLoud(t *testing.T) {
	noCredentials(t)
	var out, errb bytes.Buffer
	code := run(append(baseArgs(t), "-p", "hello"), strings.NewReader(""), &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "no Anthropic credentials") {
		t.Fatalf("exit %d, stderr %s", code, errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestUsageErrors(t *testing.T) {
	noCredentials(t)
	for _, args := range [][]string{
		{"-p", "x", "-repl"},
		{"-repl", "-dry-run"},
		{"stray-arg"},
		{"-log-level", "loud", "-p", "x"},
	} {
		var out, errb bytes.Buffer
		if code := run(append(baseArgs(t), args...), strings.NewReader(""), &out, &errb); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	var out, errb bytes.Buffer
	if code := run(append(baseArgs(t), "-dry-run", "-tools", "bogus", "-p", "x"), strings.NewReader(""), &out, &errb); code != 1 {
		t.Errorf("unknown provider: exit %d", code)
	}
	if code := run(append(baseArgs(t), "-dry-run", "-daily-cap-usd", "-3", "-p", "x"), strings.NewReader(""), &out, &errb); code != 1 {
		t.Errorf("negative cap: exit %d", code)
	}
}
