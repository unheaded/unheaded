// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type fakeHandler struct {
	calls []CallToolParams
}

func (f *fakeHandler) ListTools(context.Context) ([]Tool, error) {
	return []Tool{{Name: "echo", Description: "echo back", InputSchema: json.RawMessage(`{"type":"object"}`)}}, nil
}

func (f *fakeHandler) CallTool(_ context.Context, p CallToolParams) CallToolResult {
	f.calls = append(f.calls, p)
	if p.Name != "echo" {
		return ErrorResult("Unknown tool: " + p.Name)
	}
	return TextResult(string(p.Arguments))
}

func run(t *testing.T, h Handler, lines ...string) []map[string]any {
	t.Helper()
	s := &Server{Name: "t", Version: "0", Handler: h}
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resps []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("response %q: %v", l, err)
		}
		resps = append(resps, m)
	}
	return resps
}

func TestInitializeNegotiatesVersion(t *testing.T) {
	for _, tc := range []struct{ asked, want string }{
		{"2025-06-18", "2025-06-18"},
		{"2025-11-25", "2025-11-25"},
		{"1999-01-01", LatestProtocolVersion},
		{"", LatestProtocolVersion},
	} {
		params := `{}`
		if tc.asked != "" {
			params = `{"protocolVersion":"` + tc.asked + `"}`
		}
		r := run(t, &fakeHandler{}, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":`+params+`}`)
		res := r[0]["result"].(map[string]any)
		if got := res["protocolVersion"]; got != tc.want {
			t.Errorf("asked %q: protocolVersion %v, want %s", tc.asked, got, tc.want)
		}
		if res["serverInfo"].(map[string]any)["name"] != "t" {
			t.Errorf("serverInfo = %v", res["serverInfo"])
		}
		if _, ok := res["capabilities"].(map[string]any)["tools"]; !ok {
			t.Errorf("capabilities without tools: %v", res["capabilities"])
		}
	}
}

func TestToolsListUsesWireNames(t *testing.T) {
	r := run(t, &fakeHandler{}, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := r[0]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", tools)
	}
	tl := tools[0].(map[string]any)
	if tl["name"] != "echo" || tl["inputSchema"] == nil {
		t.Errorf("tool on the wire = %v (want name and inputSchema)", tl)
	}
}

func TestToolsCallResultAndIsError(t *testing.T) {
	h := &fakeHandler{}
	r := run(t, h,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"a":1}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nope"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{}}`,
	)
	if len(r) != 3 {
		t.Fatalf("responses = %v", r)
	}
	ok := r[0]["result"].(map[string]any)
	if ok["isError"] != false || ok["content"].([]any)[0].(map[string]any)["text"] != `{"a":1}` {
		t.Errorf("echo result = %v", ok)
	}
	bad := r[1]["result"].(map[string]any)
	if bad["isError"] != true {
		t.Errorf("unknown tool result = %v, want isError", bad)
	}
	if e, _ := r[2]["error"].(map[string]any); e == nil || e["code"].(float64) != CodeInvalidParams {
		t.Errorf("call without a name = %v, want invalid params", r[2])
	}
	if len(h.calls) != 2 || string(h.calls[1].Arguments) != "{}" {
		t.Errorf("handler saw %+v (missing arguments must arrive as {})", h.calls)
	}
}

func TestNotificationsGetNoReplyAndUnknownMethodErrors(t *testing.T) {
	r := run(t, &fakeHandler{},
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":6,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":7,"method":"no/such"}`,
		`not json`,
	)
	if len(r) != 3 {
		t.Fatalf("want 3 replies (ping, unknown, parse error), got %v", r)
	}
	if r[1]["error"].(map[string]any)["code"].(float64) != CodeMethodNotFound {
		t.Errorf("unknown method = %v", r[1])
	}
	if r[2]["error"].(map[string]any)["code"].(float64) != CodeParseError {
		t.Errorf("garbage line = %v", r[2])
	}
}

func TestOversizedLineEndsTheStream(t *testing.T) {
	s := &Server{Name: "t", Version: "0", Handler: &fakeHandler{}, MaxMessageBytes: 64}
	in := strings.Repeat("x", 200) + "\n" + `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
	var out bytes.Buffer
	err := s.Serve(context.Background(), strings.NewReader(in), &out)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Serve = %v, want a size error", err)
	}
	if out.Len() != 0 {
		t.Errorf("no reply may follow an oversized line, got %q", out.String())
	}
}

func TestServeNeedsAHandler(t *testing.T) {
	s := &Server{Name: "t"}
	if err := s.Serve(context.Background(), strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("Serve without a handler must fail")
	}
}
