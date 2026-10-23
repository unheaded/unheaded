// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package mcpclient

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pythonWithMCP finds a Python that can import the mcp package. Set
// MCPCLIENT_PYTHON to pin one; otherwise the Zhen venvs are tried, then
// python3 on PATH (system Python usually lacks mcp).
func pythonWithMCP(t *testing.T) string {
	t.Helper()
	var candidates []string
	if p := os.Getenv("MCPCLIENT_PYTHON"); p != "" {
		candidates = append(candidates, p)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".venv", "zhen", "bin", "python"),
			filepath.Join(home, ".venv", "zhen-rocm", "bin", "python"),
		)
	}
	if p, err := exec.LookPath("python3"); err == nil {
		candidates = append(candidates, p)
	}
	for _, p := range candidates {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		// #nosec G204 -- test-only probe of a local interpreter path.
		out, err := exec.CommandContext(ctx, p, "-I", "-c",
			"import importlib.metadata as m, mcp.server.stdio; print(m.version('mcp'))").Output()
		cancel()
		if err == nil {
			t.Logf("using %s (mcp %s)", p, strings.TrimSpace(string(out)))
			return p
		}
	}
	t.Skip("no Python with the mcp package; set MCPCLIENT_PYTHON or install raft/requirements.txt's mcp pin into a venv")
	return ""
}

// TestRealZhenMCPServer drives raft/zhen_mcp_server.py. It calls only
// read-only tools: runbook_list, service_health (probes local /health
// ports), runbook_show with bad arguments (refused by schema validation
// before the handler runs), local_llm against a port nothing listens on
// (an isError result, not a hang), and a tool name that does not exist.
func TestRealZhenMCPServer(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns the real Python MCP server")
	}
	server, err := filepath.Abs(filepath.Join("..", "..", "raft", "zhen_mcp_server.py"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(server); err != nil {
		t.Skipf("server script not found: %v", err)
	}
	python := pythonWithMCP(t)

	// local_llm must find nothing listening; captures stay off.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadURL := "http://" + ln.Addr().String()
	_ = ln.Close()
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "ZHEN_LOCAL_LLM_") && !strings.HasPrefix(kv, "ZHEN_CAPTURE_DIR=") {
			env = append(env, kv)
		}
	}
	env = append(env, "ZHEN_LOCAL_LLM_URL="+deadURL, "ZHEN_LOCAL_LLM_TIMEOUT=10")

	ctx := context.Background()
	c, err := Start(ctx, Config{
		Command:     python,
		Args:        []string{"-I", server},
		Env:         env,
		InitTimeout: 60 * time.Second,
		CallTimeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	d := c.Diagnostics()
	t.Logf("server %q %q, protocol %s", d.ServerName, d.ServerVersion, d.ProtocolVersion)
	if d.ProtocolVersion != LatestProtocolVersion {
		t.Errorf("negotiated %s, want %s", d.ProtocolVersion, LatestProtocolVersion)
	}

	tools, err := c.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	got := map[string]bool{}
	for _, tl := range tools {
		got[tl.Name] = true
		if !json.Valid(tl.InputSchema) {
			t.Errorf("%s: invalid schema", tl.Name)
		}
		t.Logf("tool %s: %.80s", tl.Name, tl.Description)
	}
	for _, want := range []string{"corpus_search", "file_read", "service_health", "runbook_list", "runbook_show", "local_llm"} {
		if !got[want] {
			t.Errorf("tool %s missing", want)
		}
	}
	// Mutating tools are listed only when the server's env opts in.
	for tool, flag := range map[string]string{
		"file_write": "ZHEN_MCP_ALLOW_WRITE", "file_patch": "ZHEN_MCP_ALLOW_WRITE",
		"runbook_execute": "ZHEN_MCP_ALLOW_EXEC",
	} {
		if want := os.Getenv(flag) == "1"; got[tool] != want {
			t.Errorf("tool %s listed=%v, want %v (%s=%q)", tool, got[tool], want, flag, os.Getenv(flag))
		}
	}
	if skipped := c.Diagnostics().SkippedTools; len(skipped) > 0 {
		t.Errorf("skipped tools: %v", skipped)
	}

	res, err := c.Call(ctx, "runbook_list", nil)
	if err != nil {
		t.Fatalf("runbook_list: %v", err)
	}
	if res.IsError || !strings.HasPrefix(res.Text, "Available Runbooks (") {
		t.Errorf("runbook_list = %+v", res)
	}
	t.Logf("runbook_list: %.200s", res.Text)

	res, err = c.Call(ctx, "service_health", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("service_health: %v", err)
	}
	if res.IsError || !strings.HasPrefix(res.Text, "Kingdom Service Health: ") {
		t.Errorf("service_health = %+v", res)
	}
	t.Logf("service_health: %.120s", res.Text)

	res, err = c.Call(ctx, "runbook_show", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("runbook_show: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text, "'name' is a required property") {
		t.Errorf("runbook_show without name = %+v", res)
	}
	t.Logf("runbook_show {}: %s (isError=%v)", res.Text, res.IsError)

	res, err = c.Call(ctx, "local_llm", json.RawMessage(`{"prompt":"hello"}`))
	if err != nil {
		t.Fatalf("local_llm: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text, "local LLM unavailable") {
		t.Errorf("local_llm with nothing listening = %+v", res)
	}
	t.Logf("local_llm (down): %.160s (isError=%v)", res.Text, res.IsError)

	res, err = c.Call(ctx, "no_such_tool", nil)
	if err != nil {
		t.Fatalf("no_such_tool: %v", err)
	}
	t.Logf("no_such_tool: %q (isError=%v)", res.Text, res.IsError)
	if !res.IsError || !strings.Contains(res.Text, "Unknown tool: no_such_tool") {
		t.Errorf("no_such_tool = %+v, want isError with \"Unknown tool\"", res)
	}

	if err := c.Ping(ctx); err != nil {
		t.Errorf("Ping: %v", err)
	}
}
