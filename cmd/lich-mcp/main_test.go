// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testServer(t *testing.T) *server {
	t.Helper()
	root := t.TempDir()
	hd := filepath.Join(root, "harnesses")
	if err := os.MkdirAll(hd, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hd, "lich_001_x_test.go"), []byte("package x\n"), 0o600); err != nil {
		t.Fatalf("write harness: %v", err)
	}
	cfgPath := filepath.Join(root, "lich-mcp.yaml")
	cfgYAML := "scope:\n  sandbox_prefix: \"lich-sbx-\"\n  harness_dir: \"harnesses\"\n  results_dir: \"results\"\n"
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	srv, err := newServer(cfgPath, root, "test/driver", logger)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	return srv
}

func call(t *testing.T, srv *server, name, args string) callToolResult {
	t.Helper()
	return srv.callTool(context.Background(), callToolParams{Name: name, Arguments: json.RawMessage(args)})
}

// Step 37: out-of-scope run_harness is refused by scope BEFORE any gate.
func TestServer_RunHarness_EvilTarget_RefusedBeforeGate(t *testing.T) {
	srv := testServer(t)
	res := call(t, srv, "lich_run_harness", `{"harness_id":"evil-host"}`)
	if !res.IsError {
		t.Fatalf("evil target must be refused, got %+v", res)
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "out of scope") || !strings.Contains(text, "before gate") {
		t.Fatalf("refusal should be scope-before-gate, got %q", text)
	}
}

// Step 37: lich_file_finding is refused pending-confirmation by the gate.
func TestServer_FileFinding_PendingConfirmation(t *testing.T) {
	srv := testServer(t)
	res := call(t, srv, "lich_file_finding", `{"severity":"low","summary":"benign test finding"}`)
	if !res.IsError {
		t.Fatalf("file_finding must be refused pending confirmation, got %+v", res)
	}
	if !strings.Contains(res.Content[0].Text, "pending") {
		t.Fatalf("file_finding refusal should be pending-confirmation, got %q", res.Content[0].Text)
	}
}

// Step 37: lich_list_campaigns (read-only) passes the gate.
func TestServer_ListCampaigns_PassesGate(t *testing.T) {
	srv := testServer(t)
	res := call(t, srv, "lich_list_campaigns", `{}`)
	if res.IsError {
		t.Fatalf("read-only list_campaigns should pass the gate, got %+v", res)
	}
}

// Step 70-71: in-scope run_harness passes scope + gate and actually runs a
// command inside the confined sandbox, returning its bounded output.
func TestServer_RunHarnessSandbox(t *testing.T) {
	if os.Getenv("LICH_BAREMETAL") != "1" {
		t.Skip("set LICH_BAREMETAL=1 to run the MCP-tool -> confined-sandbox path")
	}
	srv := testServer(t)
	res := call(t, srv, "lich_run_harness", `{"harness_id":"001"}`)
	if strings.Contains(res.Content[0].Text, "out of scope") || strings.Contains(res.Content[0].Text, "refused by gate") {
		t.Fatalf("in-scope run_harness must not be refused: %q", res.Content[0].Text)
	}
	if res.IsError {
		t.Fatalf("confined run should succeed, got %+v", res)
	}
	if !strings.Contains(res.Content[0].Text, "lich-sandbox-ok harness=001") || !strings.Contains(res.Content[0].Text, "confined sandbox") {
		t.Fatalf("run_harness output should show the confined run, got %q", res.Content[0].Text)
	}
}

// The stdio loop answers initialize and lists six tools.
func TestServer_Serve_InitializeAndToolsList(t *testing.T) {
	srv := testServer(t)
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n")
	var out bytes.Buffer
	if err := srv.serve(context.Background(), in, &out); err != nil && err != io.EOF {
		t.Fatalf("serve: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 responses, got %d: %q", len(lines), out.String())
	}
	var listResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatalf("parse tools/list: %v", err)
	}
	if len(listResp.Result.Tools) != 6 {
		t.Fatalf("want 6 tools over the wire, got %d", len(listResp.Result.Tools))
	}
}

// Unknown method returns a JSON-RPC method-not-found error.
func TestServer_Serve_UnknownMethod(t *testing.T) {
	srv := testServer(t)
	in := strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"no/such","params":{}}` + "\n")
	var out bytes.Buffer
	if err := srv.serve(context.Background(), in, &out); err != nil && err != io.EOF {
		t.Fatalf("serve: %v", err)
	}
	if !strings.Contains(out.String(), "method not found") {
		t.Fatalf("want method-not-found, got %q", out.String())
	}
}
