// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/claudebridge/privacy"
	"unheaded/pkg/zhentools"
)

func testFilter(t *testing.T, cfg privacy.RedactorConfig, cls privacy.Classifier) *privacy.Filter {
	t.Helper()
	r, err := privacy.NewRedactor(cfg)
	if err != nil {
		t.Skip("secret scanner unavailable:", err)
	}
	return &privacy.Filter{Redactor: r, Classifier: cls}
}

func healthProvider(text string) *fakeProvider {
	return &fakeProvider{
		tools: []zhentools.Tool{{Name: "service_health", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		call: func(context.Context, string, json.RawMessage) (zhentools.Result, error) {
			return zhentools.Result{Text: text}, nil
		},
	}
}

func withPrivacy(f *privacy.Filter, root func(*Config) string) harnessOpt {
	return func(c *Config) {
		c.Privacy = f
		if root != nil {
			c.ProjectRoot = root(c)
		}
	}
}

func TestPrivacy_ToolResultReachesClaudeRedacted(t *testing.T) {
	f := testFilter(t, privacy.RedactorConfig{}, privacy.Classifier{})
	mcp := healthProvider("wotan 10.10.10.10 down on west; peer 192.168.13.2")
	h := newHarness(t, nil, []NamedProvider{{Name: "mcp:zhen", Provider: AllowTools(mcp, MCPReadOnlyTools...)}}, withPrivacy(f, nil))
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{{ToolID: "toolu_h", ToolName: "service_health", ToolInput: `{}`}}},
		endTurn("ok"),
	}
	if _, err := h.bridge.NewConversation().Send(context.Background(), "how is wotan?"); err != nil {
		t.Fatal(err)
	}
	r := lastToolResult(t, h)
	txt := resultText(r)
	for _, bad := range []string{"10.10.10.10", "192.168.13.2", "west"} {
		if strings.Contains(txt, bad) {
			t.Errorf("%s reached Claude: %s", bad, txt)
		}
	}
	if !strings.Contains(txt, "[REDACTED:ipv4#1]") || r["is_error"] == true {
		t.Errorf("result = %v", r)
	}
	if h.audit.tools[0].Redactions != 3 || h.audit.tools[0].Tier != "T1" {
		t.Errorf("tool audit = %+v", h.audit.tools[0])
	}
	// The next request carries the count.
	if h.audit.reqs[1].Redactions != 3 {
		t.Errorf("request redactions = %d", h.audit.reqs[1].Redactions)
	}
}

func TestPrivacy_UserDataWithheld(t *testing.T) {
	var ud string
	h := newHarness(t, nil, nil, func(c *Config) {
		g := c.Gate.(*ChampionGate)
		ud = filepath.Join(g.Champion.GetProjectRoot(), "customers")
		c.Privacy = testFilter(t, privacy.RedactorConfig{}, privacy.Classifier{UserDataRoots: []string{ud}})
		c.ProjectRoot = g.Champion.GetProjectRoot()
	})
	if err := os.MkdirAll(ud, 0o700); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(ud, "list.csv")
	if err := os.WriteFile(f, []byte("alice@example.com,555-0100"), 0o600); err != nil {
		t.Fatal(err)
	}
	in, _ := json.Marshal(map[string]string{"path": f})
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{{ToolID: "toolu_r", ToolName: "read_file", ToolInput: string(in)}}},
		endTurn("ok"),
	}
	if _, err := h.bridge.NewConversation().Send(context.Background(), "read it"); err != nil {
		t.Fatal(err)
	}
	r := lastToolResult(t, h)
	if r["is_error"] != true || !strings.Contains(resultText(r), "withheld: user data") || strings.Contains(resultText(r), "alice") {
		t.Errorf("result = %v", r)
	}
	if h.audit.tools[0].Decision != "withheld" || h.audit.tools[0].Tier != "T2" {
		t.Errorf("audit = %+v", h.audit.tools[0])
	}
}

func TestPrivacy_RedactorErrorIsErrorNeverRawText(t *testing.T) {
	r, err := privacy.NewRedactor(privacy.RedactorConfig{Scanner: "/bin/false"})
	if err != nil {
		t.Fatal(err)
	}
	mcp := healthProvider("RAW-OPS-TEXT 10.10.10.10")
	h := newHarness(t, nil, []NamedProvider{{Name: "mcp:zhen", Provider: AllowTools(mcp, MCPReadOnlyTools...)}},
		withPrivacy(&privacy.Filter{Redactor: r}, nil))
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{{ToolID: "toolu_h", ToolName: "service_health", ToolInput: `{}`}}},
		endTurn("ok"),
	}
	if _, err := h.bridge.NewConversation().Send(context.Background(), "status"); err == nil {
		t.Fatal("the prompt went out although the redactor cannot run")
	}
	if len(h.api.reqs()) != 0 {
		t.Fatal("a request was sent with a broken redactor")
	}
}

func TestPrivacy_ToolRedactorErrorWithheld(t *testing.T) {
	// The prompt passes (good scanner), then the scanner breaks: the tool
	// result must be withheld, not sent raw.
	f := testFilter(t, privacy.RedactorConfig{}, privacy.Classifier{})
	mcp := healthProvider("RAW-OPS-TEXT")
	broken, err := privacy.NewRedactor(privacy.RedactorConfig{Scanner: "/bin/false"})
	if err != nil {
		t.Fatal(err)
	}
	mcp.call = func(context.Context, string, json.RawMessage) (zhentools.Result, error) {
		*f.Redactor = *broken // the scanner breaks after the prompt passed
		return zhentools.Result{Text: "RAW-OPS-TEXT"}, nil
	}
	h := newHarness(t, nil, []NamedProvider{{Name: "mcp:zhen", Provider: AllowTools(mcp, MCPReadOnlyTools...)}}, withPrivacy(f, nil))
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{{ToolID: "toolu_h", ToolName: "service_health", ToolInput: `{}`}}},
		endTurn("ok"),
	}
	if _, err := h.bridge.NewConversation().Send(context.Background(), "status"); err != nil {
		t.Fatal(err)
	}
	r := lastToolResult(t, h)
	if r["is_error"] != true || strings.Contains(resultText(r), "RAW-OPS-TEXT") {
		t.Errorf("result = %v", r)
	}
}

func TestPrivacy_PromptRedactedBeforeSending(t *testing.T) {
	f := testFilter(t, privacy.RedactorConfig{}, privacy.Classifier{})
	var warned []string
	h := newHarness(t, []cannedMsg{endTurn("ok")}, nil, withPrivacy(f, nil), func(c *Config) {
		c.Warn = func(s string) { warned = append(warned, s) }
	})
	if _, err := h.bridge.NewConversation().Send(context.Background(), "why is 10.10.10.10 slow for stevie@bellis.tech?"); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(h.api.reqs()[0].Body)
	if strings.Contains(string(raw), "10.10.10.10") || strings.Contains(string(raw), "stevie@bellis.tech") {
		t.Errorf("the prompt left unredacted: %s", raw)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "2 value") || h.audit.reqs[0].Redactions != 2 {
		t.Errorf("warned %v, redactions %d", warned, h.audit.reqs[0].Redactions)
	}
}
