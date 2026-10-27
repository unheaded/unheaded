// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/mcpserver"
)

var readOnlyTools = []string{"corpus_search", "file_read", "local_llm", "runbook_list", "runbook_show", "service_health"}

// testRoot is a throwaway project root: a go.mod, two runbooks, a file to
// read and a stand-in run-runbook.py that prints its argv.
func testRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module unheaded\n")
	write("README.md", "hello from the sandbox\n")
	write("runbooks/infra/service-health-sweep.yaml", "apiVersion: runbook/v1\nkind: Runbook\nmetadata:\n  name: service-health-sweep\n")
	write("runbooks/observe/slo-check.yaml", "apiVersion: runbook/v1\nkind: Runbook\nmetadata:\n  name: slo-check\n")
	write("scripts/run-runbook.py", "import sys\nprint('ran', ' '.join(sys.argv[1:]))\n")
	return root
}

// deadURL is an http URL nothing listens on.
func deadURL(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	u := "http://" + ln.Addr().String()
	_ = ln.Close()
	return u
}

func testConfig(t *testing.T, root string, env map[string]string) config {
	t.Helper()
	base := map[string]string{
		envProjectRoot: root,
		envLocalLLMURL: deadURL(t),
		envVorURL:      deadURL(t),
	}
	for k, v := range env {
		base[k] = v
	}
	cfg, err := configFromEnv(func(k string) string { return base[k] })
	if err != nil {
		t.Fatalf("configFromEnv: %v", err)
	}
	cfg.services = nil // never probe the host's real ports from a unit test
	return cfg
}

func testServer(t *testing.T, root string, env map[string]string) *server {
	t.Helper()
	srv, err := newServer(testConfig(t, root, env), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	return srv
}

func call(t *testing.T, srv *server, name, args string) mcpserver.CallToolResult {
	t.Helper()
	res := srv.CallTool(context.Background(), mcpserver.CallToolParams{Name: name, Arguments: json.RawMessage(args)})
	if len(res.Content) != 1 || res.Content[0].Type != "text" {
		t.Fatalf("%s: result %+v is not one text block", name, res)
	}
	return res
}

func toolNames(t *testing.T, srv *server) []string {
	t.Helper()
	tools, err := srv.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range tools {
		if !json.Valid(tl.InputSchema) || tl.Description == "" {
			t.Errorf("%s: bad schema or empty description", tl.Name)
		}
		names = append(names, tl.Name)
	}
	sort.Strings(names)
	return names
}

func TestToolsListReadOnlyByDefault(t *testing.T) {
	got := toolNames(t, testServer(t, testRoot(t), nil))
	if strings.Join(got, ",") != strings.Join(readOnlyTools, ",") {
		t.Fatalf("tools = %v, want %v", got, readOnlyTools)
	}
}

func TestToolsListFlagsAddGatedTools(t *testing.T) {
	root := testRoot(t)
	for _, tc := range []struct {
		env  map[string]string
		want []string
	}{
		{map[string]string{envAllowWrite: "1"}, []string{"file_patch", "file_write"}},
		{map[string]string{envAllowExec: "1"}, []string{"runbook_execute"}},
		{map[string]string{envAllowWrite: "1", envAllowExec: "1"}, []string{"file_patch", "file_write", "runbook_execute"}},
		// Only exactly "1" opts in.
		{map[string]string{envAllowWrite: "true", envAllowExec: "yes"}, nil},
		{map[string]string{envAllowWrite: " 1"}, nil},
	} {
		want := append(append([]string{}, readOnlyTools...), tc.want...)
		sort.Strings(want)
		got := toolNames(t, testServer(t, root, tc.env))
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("env %v: tools = %v, want %v", tc.env, got, want)
		}
	}
}

func TestFileReadInsideRoot(t *testing.T) {
	root := testRoot(t)
	res := call(t, testServer(t, root, nil), "file_read", `{"path":"`+filepath.Join(root, "README.md")+`"}`)
	if res.IsError || !strings.Contains(res.Content[0].Text, "hello from the sandbox") {
		t.Fatalf("file_read = %+v", res)
	}
}

func TestFileReadOutsideRootIsAccessDenied(t *testing.T) {
	srv := testServer(t, testRoot(t), nil)
	for _, p := range []string{"/etc/shadow", "/etc/hostname", "../../etc/passwd"} {
		res := call(t, srv, "file_read", `{"path":"`+p+`"}`)
		if !res.IsError || !strings.Contains(res.Content[0].Text, "ACCESS DENIED") {
			t.Errorf("file_read %s = %+v, want isError ACCESS DENIED", p, res)
		}
	}
}

func TestFileReadSymlinkOutIsDenied(t *testing.T) {
	root := testRoot(t)
	if err := os.Symlink("/etc/hostname", filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	res := call(t, testServer(t, root, nil), "file_read", `{"path":"`+filepath.Join(root, "escape")+`"}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "ACCESS DENIED") {
		t.Fatalf("symlink out = %+v", res)
	}
}

func TestFileReadNotFoundIsError(t *testing.T) {
	root := testRoot(t)
	res := call(t, testServer(t, root, nil), "file_read", `{"path":"`+filepath.Join(root, "nope.txt")+`"}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "not found") {
		t.Fatalf("not found = %+v", res)
	}
}

func TestFileReadBadArgumentsIsError(t *testing.T) {
	srv := testServer(t, testRoot(t), nil)
	for _, args := range []string{`{}`, `{"path":7}`, `[1]`, `{"path":""}`} {
		if res := call(t, srv, "file_read", args); !res.IsError {
			t.Errorf("file_read %s = %+v, want isError", args, res)
		}
	}
}

func TestGatedToolsDisabledAreErrors(t *testing.T) {
	root := testRoot(t)
	srv := testServer(t, root, nil)
	target := filepath.Join(root, "new.txt")
	for name, args := range map[string]string{
		"file_write":      `{"path":"` + target + `","content":"x"}`,
		"file_patch":      `{"path":"` + filepath.Join(root, "README.md") + `","old_text":"hello","new_text":"bye"}`,
		"runbook_execute": `{"name":"slo-check","dry_run":true}`,
	} {
		res := call(t, srv, name, args)
		if !res.IsError || !strings.Contains(res.Content[0].Text, "disabled") {
			t.Errorf("%s with no flag = %+v, want isError disabled", name, res)
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("a disabled file_write wrote %s (err %v)", target, err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "README.md")); !strings.Contains(string(b), "hello") {
		t.Fatal("a disabled file_patch changed README.md")
	}
}

func TestFileWriteEnabledGoesThroughTheChampion(t *testing.T) {
	root := testRoot(t)
	srv := testServer(t, root, map[string]string{envAllowWrite: "1"})
	target := filepath.Join(root, "sub", "new.txt")
	res := call(t, srv, "file_write", `{"path":"`+target+`","content":"written"}`)
	if res.IsError {
		t.Fatalf("file_write = %+v", res)
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "written" {
		t.Fatalf("file on disk = %q, %v", b, err)
	}
	res = call(t, srv, "file_patch", `{"path":"`+target+`","old_text":"written","new_text":"patched"}`)
	if res.IsError {
		t.Fatalf("file_patch = %+v", res)
	}
	if b, _ := os.ReadFile(target); string(b) != "patched" {
		t.Fatalf("after patch: %q", b)
	}
	// The Champion's rules still apply with the flag on.
	for _, tc := range []struct{ args, want string }{
		{`{"path":"/tmp/zhen-mcp-outside.txt","content":"x"}`, "ACCESS DENIED"},
		{`{"path":"` + filepath.Join(root, "x.sh") + `","content":"rm -rf /"}`, "destructive"},
		{`{"path":"` + filepath.Join(root, ".env") + `","content":"x"}`, "ACCESS DENIED"},
	} {
		res := call(t, srv, "file_write", tc.args)
		if !res.IsError || !strings.Contains(res.Content[0].Text, tc.want) {
			t.Errorf("file_write %s = %+v, want isError %q", tc.args, res, tc.want)
		}
	}
	if _, err := os.Stat("/tmp/zhen-mcp-outside.txt"); err == nil {
		t.Fatal("a write outside the root landed")
	}
}

func TestRunbookListAndShow(t *testing.T) {
	srv := testServer(t, testRoot(t), nil)
	res := call(t, srv, "runbook_list", `{}`)
	if res.IsError || !strings.HasPrefix(res.Content[0].Text, "Available Runbooks (2)") ||
		!strings.Contains(res.Content[0].Text, "### infra/") || !strings.Contains(res.Content[0].Text, "  - slo-check") {
		t.Fatalf("runbook_list = %+v", res)
	}
	for _, name := range []string{"slo-check", "observe/slo-check"} {
		res = call(t, srv, "runbook_show", `{"name":"`+name+`"}`)
		if res.IsError || !strings.Contains(res.Content[0].Text, "name: slo-check") {
			t.Errorf("runbook_show %s = %+v", name, res)
		}
	}
	for _, args := range []string{`{}`, `{"name":"nope"}`, `{"name":"../../etc/passwd"}`, `{"name":"*"}`, `{"name":"/etc/passwd"}`} {
		if res := call(t, srv, "runbook_show", args); !res.IsError {
			t.Errorf("runbook_show %s = %+v, want isError", args, res)
		}
	}
}

func TestRunbookShowSymlinkOutOfRunbooksIsError(t *testing.T) {
	root := testRoot(t)
	if err := os.Symlink("/etc/hostname", filepath.Join(root, "runbooks", "infra", "evil.yaml")); err != nil {
		t.Fatal(err)
	}
	if res := call(t, testServer(t, root, nil), "runbook_show", `{"name":"evil"}`); !res.IsError {
		t.Fatalf("runbook_show through a symlink out = %+v", res)
	}
}

func TestRunbookExecuteEnabledDryRun(t *testing.T) {
	root := testRoot(t)
	srv := testServer(t, root, map[string]string{envAllowExec: "1"})
	res := call(t, srv, "runbook_execute", `{"name":"slo-check","dry_run":true}`)
	if res.IsError || !strings.Contains(res.Content[0].Text, "--dry-run") || !strings.Contains(res.Content[0].Text, "SUCCESS") {
		t.Fatalf("runbook_execute dry run = %+v", res)
	}
	if res := call(t, srv, "runbook_execute", `{"name":"bad name!"}`); !res.IsError {
		t.Fatalf("bad runbook name = %+v", res)
	}
}

func TestUnknownToolIsError(t *testing.T) {
	res := call(t, testServer(t, testRoot(t), nil), "no_such_tool", `{}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "Unknown tool: no_such_tool") {
		t.Fatalf("unknown tool = %+v", res)
	}
}

func TestLocalLLMDownIsErrorAndStartsNothing(t *testing.T) {
	srv := testServer(t, testRoot(t), nil)
	start := time.Now()
	res := call(t, srv, "local_llm", `{"prompt":"hello"}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "local LLM unavailable") || !strings.Contains(res.Content[0].Text, "nothing was started") {
		t.Fatalf("local_llm with nothing listening = %+v", res)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("local_llm against a dead port took too long")
	}
}

// fakeLlama is an OpenAI chat-completions endpoint that records requests.
func fakeLlama(t *testing.T, status int, body string) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var seen []map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		seen = append(seen, m)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(ts.Close)
	return ts, &seen
}

func TestLocalLLMAnswerAndRequestShape(t *testing.T) {
	ts, seen := fakeLlama(t, 200, `{"model":"qwen","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"4"}}]}`)
	srv := testServer(t, testRoot(t), map[string]string{envLocalLLMURL: ts.URL})
	res := call(t, srv, "local_llm", `{"prompt":"2+2?","system":"be terse","max_tokens":9999,"temperature":0}`)
	if res.IsError || !strings.HasSuffix(res.Content[0].Text, "\n---\n4") || !strings.Contains(res.Content[0].Text, "Untrusted model output") {
		t.Fatalf("local_llm = %+v", res)
	}
	if len(*seen) != 1 {
		t.Fatalf("requests = %d", len(*seen))
	}
	req := (*seen)[0]
	if req["max_tokens"].(float64) != localLLMMaxTokensCap || req["temperature"].(float64) != 0 || req["stream"] != false {
		t.Errorf("request = %v", req)
	}
	msgs := req["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["content"] != "2+2?" {
		t.Errorf("messages = %v", msgs)
	}
}

func TestLocalLLMBadArgumentsNeverReachTheServer(t *testing.T) {
	ts, seen := fakeLlama(t, 200, `{}`)
	srv := testServer(t, testRoot(t), map[string]string{envLocalLLMURL: ts.URL})
	for _, args := range []string{
		`{}`, `{"prompt":""}`, `{"prompt":"  "}`, `{"prompt":7}`, `{"prompt":"x","system":3}`,
		`{"prompt":"x","max_tokens":0}`, `{"prompt":"x","max_tokens":1.5}`, `{"prompt":"x","max_tokens":true}`,
		`{"prompt":"x","temperature":3}`, `{"prompt":"x","temperature":"hot"}`,
		`{"prompt":"` + strings.Repeat("a", localLLMMaxPromptBytes+1) + `"}`,
	} {
		if res := call(t, srv, "local_llm", args); !res.IsError {
			t.Errorf("local_llm %.60s = %+v, want isError", args, res)
		}
	}
	if len(*seen) != 0 {
		t.Fatalf("%d bad requests reached the server", len(*seen))
	}
}

func TestLocalLLMHTTPErrorAndBadBodies(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{
		{500, "boom"},
		{200, "not json"},
		{200, `{"choices":[]}`},
		{200, `{"choices":[{"message":{"content":"   "}}]}`},
	} {
		ts, _ := fakeLlama(t, tc.status, tc.body)
		srv := testServer(t, testRoot(t), map[string]string{envLocalLLMURL: ts.URL})
		if res := call(t, srv, "local_llm", `{"prompt":"x"}`); !res.IsError {
			t.Errorf("status %d body %q = %+v, want isError", tc.status, tc.body, res)
		}
	}
}

func TestLocalLLMLongAnswerIsCut(t *testing.T) {
	long := strings.Repeat("y", localLLMMaxAnswerChars+50)
	ts, _ := fakeLlama(t, 200, `{"choices":[{"message":{"content":"`+long+`"}}]}`)
	srv := testServer(t, testRoot(t), map[string]string{envLocalLLMURL: ts.URL})
	res := call(t, srv, "local_llm", `{"prompt":"x"}`)
	if res.IsError || !strings.HasSuffix(res.Content[0].Text, "[truncated by zhen-mcp]") {
		t.Fatalf("long answer = %.80s… isError=%v", res.Content[0].Text, res.IsError)
	}
}

func TestLocalLLMBadConfigIsErrorAtCallTime(t *testing.T) {
	for _, env := range []map[string]string{
		{envLocalLLMURL: "ftp://127.0.0.1:1"},
		{envLocalLLMTimeout: "soon"},
		{envLocalLLMTimeout: "0"},
		{envLocalLLMTimeout: "601"},
	} {
		srv := testServer(t, testRoot(t), env)
		if res := call(t, srv, "local_llm", `{"prompt":"x"}`); !res.IsError {
			t.Errorf("env %v: local_llm = %+v, want isError", env, res)
		}
	}
}

func TestLocalLLMCaptureOptIn(t *testing.T) {
	// Built at run time: a literal key-shaped string trips gitleaks.
	key := "ghp_" + strings.Repeat("a", 36)
	ts, _ := fakeLlama(t, 200, `{"model":"qwen","choices":[{"message":{"content":"the token is `+key+`"}}]}`)
	dir := filepath.Join(t.TempDir(), "cap")
	srv := testServer(t, testRoot(t), map[string]string{envLocalLLMURL: ts.URL, envCaptureDir: dir})
	res := call(t, srv, "local_llm", `{"prompt":"x","task_family":"coding-review"}`)
	if res.IsError || strings.Contains(res.Content[0].Text, "capture failed") {
		t.Fatalf("local_llm = %+v", res)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "zhen-capture-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("capture files = %v", files)
	}
	b, _ := os.ReadFile(files[0])
	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(b), &rec); err != nil {
		t.Fatalf("capture line: %v", err)
	}
	if rec["source"] != "zhen-mcp/local_llm" || rec["route"] != "local" || rec["task_family"] != "coding-review" || rec["conversation_id"] == "" {
		t.Errorf("capture record = %v", rec)
	}
	if strings.Contains(string(b), key) {
		t.Error("capture holds an unredacted token")
	}
	if !strings.Contains(res.Content[0].Text, key) {
		t.Error("only the capture is redacted; the caller gets the answer as is")
	}

	// Inside a git work tree the capture is refused; the answer still comes back.
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	srv = testServer(t, testRoot(t), map[string]string{envLocalLLMURL: ts.URL, envCaptureDir: filepath.Join(repo, "caps")})
	res = call(t, srv, "local_llm", `{"prompt":"x"}`)
	if res.IsError || !strings.Contains(res.Content[0].Text, "capture failed") {
		t.Fatalf("capture inside a repo = %+v", res)
	}
}

func TestServiceHealthProbesConfiguredServices(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
		}
	}))
	defer up.Close()
	srv := testServer(t, testRoot(t), nil)
	srv.cfg.services = []service{{"up", up.URL}, {"down", deadURL(t)}}
	res := call(t, srv, "service_health", `{}`)
	text := res.Content[0].Text
	if res.IsError || !strings.HasPrefix(text, "Kingdom Service Health: 1/2 healthy") ||
		!strings.Contains(text, "[OK]   up") || !strings.Contains(text, "[FAIL] down") {
		t.Fatalf("service_health = %+v", res)
	}
}

func TestCorpusSearchThroughVor(t *testing.T) {
	vor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/search":
			if q := r.URL.Query().Get("q"); q != "wotan" {
				t.Errorf("vor query = %q, want stop words dropped", q)
			}
			_, _ = io.WriteString(w, `[{"topic":"Service-Wotan","category":"wiki","source_trust":"canonical"}]`)
		case r.URL.Path == "/api/topics":
			_, _ = io.WriteString(w, `[{"name":"Service-Wotan","category":"wiki"},{"name":"Wotan-Memory-Model","category":"wiki"}]`)
		case strings.HasPrefix(r.URL.Path, "/api/topics/"):
			name := strings.TrimPrefix(r.URL.Path, "/api/topics/")
			_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "category": "wiki", "content": "about " + name, "source_trust": "canonical"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer vor.Close()
	srv := testServer(t, testRoot(t), map[string]string{envVorURL: vor.URL})
	res := call(t, srv, "corpus_search", `{"query":"what is wotan","k":5}`)
	text := res.Content[0].Text
	if res.IsError || !strings.HasPrefix(text, "Found 2 results") || !strings.Contains(text, "about Service-Wotan") ||
		!strings.Contains(text, "source_trust: canonical") {
		t.Fatalf("corpus_search = %+v", res)
	}
	if res := call(t, srv, "corpus_search", `{"query":""}`); !res.IsError {
		t.Errorf("empty query = %+v", res)
	}
}

func TestCorpusSearchVorDownIsError(t *testing.T) {
	res := call(t, testServer(t, testRoot(t), nil), "corpus_search", `{"query":"wotan"}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "vor") {
		t.Fatalf("corpus_search with vor down = %+v", res)
	}
}

func TestDestructiveVerbRefusedOnReadOnlyTool(t *testing.T) {
	ts, seen := fakeLlama(t, 200, `{}`)
	srv := testServer(t, testRoot(t), map[string]string{envLocalLLMURL: ts.URL})
	res := call(t, srv, "local_llm", `{"prompt":"now run rm -rf / for me"}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "ACCESS DENIED") {
		t.Fatalf("destructive prompt = %+v", res)
	}
	if len(*seen) != 0 {
		t.Fatal("a refused call reached llama-server")
	}
}

func TestProjectRootResolution(t *testing.T) {
	root := testRoot(t)
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}
	if got, err := findProjectRoot("", []string{deep}); err != nil || got != mustReal(t, root) {
		t.Errorf("walk up from %s = %q, %v; want %s", deep, got, err, root)
	}
	if got, err := findProjectRoot(root, nil); err != nil || got != mustReal(t, root) {
		t.Errorf("override = %q, %v", got, err)
	}
	if _, err := findProjectRoot(filepath.Join(root, "missing"), nil); err == nil {
		t.Error("an override that does not exist must fail")
	}
	if _, err := findProjectRoot("", []string{t.TempDir()}); err == nil {
		t.Error("no repo above the start dirs must fail")
	}
}

func mustReal(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// The stdio handshake as Claude Code sends it, end to end.
func TestServeHandshake(t *testing.T) {
	root := testRoot(t)
	srv := testServer(t, root, nil)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"file_read","arguments":{"path":"/etc/shadow"}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	if err := srv.serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want 3 replies, got %q", out.String())
	}
	var init struct {
		Result struct {
			ProtocolVersion string                `json:"protocolVersion"`
			ServerInfo      struct{ Name string } `json:"serverInfo"`
		} `json:"result"`
	}
	_ = json.Unmarshal([]byte(lines[0]), &init)
	if init.Result.ProtocolVersion != "2025-06-18" || init.Result.ServerInfo.Name != "zhen-champion" {
		t.Errorf("initialize = %s", lines[0])
	}
	var list struct {
		Result struct{ Tools []struct{ Name string } } `json:"result"`
	}
	_ = json.Unmarshal([]byte(lines[1]), &list)
	if len(list.Result.Tools) != 6 {
		t.Errorf("tools/list = %s", lines[1])
	}
	var callRes struct {
		Result mcpserver.CallToolResult `json:"result"`
	}
	_ = json.Unmarshal([]byte(lines[2]), &callRes)
	if !callRes.Result.IsError {
		t.Errorf("file_read /etc/shadow = %s", lines[2])
	}
}
