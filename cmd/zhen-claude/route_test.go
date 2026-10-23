// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeLocal is an httptest llama-server: /v1/chat/completions only.
type fakeLocal struct {
	srv    *httptest.Server
	mu     sync.Mutex
	hits   int
	answer string
	finish string
	status int
}

func newFakeLocal(t *testing.T, answer, finish string) *fakeLocal {
	t.Helper()
	f := &fakeLocal{answer: answer, finish: finish, status: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		f.mu.Lock()
		f.hits++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "qwen-test",
			"choices": []any{map[string]any{"finish_reason": f.finish, "message": map[string]any{"role": "assistant", "content": f.answer}}},
		})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLocal) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits
}

// fakeClaude is an httptest Messages API that streams one text answer
// per request. No test reaches the real API.
type fakeClaude struct {
	srv  *httptest.Server
	mu   sync.Mutex
	hits int
	text string
}

func newFakeClaude(t *testing.T, text string) *fakeClaude {
	t.Helper()
	f := &fakeClaude{text: text}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/messages") {
			http.NotFound(w, r)
			return
		}
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
			"id": fmt.Sprintf("msg_fake_%d", n), "type": "message", "role": "assistant", "model": "claude-opus-5-5",
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 1},
		}})
		ev("content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": f.text}})
		ev("content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
		ev("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
			"usage": map[string]any{"input_tokens": 100, "output_tokens": 50, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}})
		ev("message_stop", map[string]any{"type": "message_stop"})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeClaude) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits
}

// withClaude points the SDK at f with a fake key (never a real one).
func withClaude(t *testing.T, f *fakeClaude) {
	t.Helper()
	noCredentials(t)
	t.Setenv("ANTHROPIC_BASE_URL", f.srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "test-not-a-key")
}

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

func autoArgs(t *testing.T, localURL string, extra ...string) []string {
	t.Helper()
	t.Setenv("ZHEN_CAPTURE_DIR", "")
	return append(append(baseArgs(t), "-tools", "none", "-route", "auto", "-local-url", localURL), extra...)
}

// logLine returns the first JSON log line with msg == name.
func logLine(t *testing.T, stderr string, name string) map[string]any {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(stderr))
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) == nil && m["msg"] == name {
			return m
		}
	}
	t.Fatalf("no %q line in:\n%s", name, stderr)
	return nil
}

func TestAutoLocalAcceptedNeedsNoCredentials(t *testing.T) {
	noCredentials(t)
	local := newFakeLocal(t, "forty-two", "stop")
	var out, errb bytes.Buffer
	code := run(autoArgs(t, local.srv.URL, "-p", "meaning?"), strings.NewReader(""), &out, &errb)
	if code != 0 || out.String() != "forty-two\n" {
		t.Fatalf("exit %d out %q stderr %s", code, out.String(), errb.String())
	}
	d := logLine(t, errb.String(), "route_decision")
	if d["decision"] != "local" || d["escalated"] != false || d["why"] != whyLocalOK || d["local_model"] != "qwen-test" || d["claude_cost_usd"] != "0" {
		t.Errorf("route_decision = %v", d)
	}
	if strings.Contains(errb.String(), "meaning?") || strings.Contains(errb.String(), "forty-two") {
		t.Error("prompt or answer text in the log")
	}
}

func TestAutoCheck(t *testing.T) {
	local := newFakeLocal(t, "forty-two", "stop")
	claude := newFakeClaude(t, "Claude says 42")
	withClaude(t, claude)

	var out, errb bytes.Buffer
	if code := run(autoArgs(t, local.srv.URL, "-check", "grep -q forty", "-p", "q"), strings.NewReader(""), &out, &errb); code != 0 || out.String() != "forty-two\n" {
		t.Fatalf("pass: exit %d out %q %s", code, out.String(), errb.String())
	}
	if d := logLine(t, errb.String(), "route_decision"); d["why"] != whyCheckPassed || d["check_exit"] != float64(0) {
		t.Errorf("pass: %v", d)
	}
	if claude.count() != 0 {
		t.Fatalf("Claude called %d times on an accepted local answer", claude.count())
	}

	out.Reset()
	errb.Reset()
	if code := run(autoArgs(t, local.srv.URL, "-check", "grep -q seven", "-p", "q"), strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("fail: exit %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Claude says 42") || strings.Contains(out.String(), "forty-two") {
		t.Errorf("stdout = %q", out.String())
	}
	d := logLine(t, errb.String(), "route_decision")
	if d["why"] != whyCheckFailed || d["check_exit"] != float64(1) || d["escalated"] != true || d["decision"] != "claude" || d["claude_ok"] != true {
		t.Errorf("fail: %v", d)
	}
	if c, _ := d["claude_cost_usd"].(string); c == "" || c == "0" {
		t.Errorf("claude cost = %v", d["claude_cost_usd"])
	}
	if claude.count() != 1 {
		t.Errorf("Claude calls = %d", claude.count())
	}
}

func TestAutoEscalatesOnLocalFailure(t *testing.T) {
	claude := newFakeClaude(t, "from claude")
	withClaude(t, claude)
	cut := newFakeLocal(t, "half an", "length")
	bad := newFakeLocal(t, "x", "stop")
	bad.status = http.StatusServiceUnavailable
	for _, c := range []struct {
		name, url, why string
	}{
		{"down", deadURL(t), whyLocalError},
		{"http 503", bad.srv.URL, whyLocalError},
		{"truncated", cut.srv.URL, whyLocalTruncated},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := run(autoArgs(t, c.url, "-p", "q"), strings.NewReader(""), &out, &errb); code != 0 {
				t.Fatalf("exit %d %s", code, errb.String())
			}
			d := logLine(t, errb.String(), "route_decision")
			if d["why"] != c.why || d["escalated"] != true || d["local_ok"] != false || d["local_error"] == nil {
				t.Errorf("route_decision = %v", d)
			}
			if !strings.Contains(out.String(), "from claude") || strings.Contains(out.String(), "half an") {
				t.Errorf("stdout = %q", out.String())
			}
		})
	}
}

func TestAutoNoLocalNoCredentialsFailsLoud(t *testing.T) {
	noCredentials(t)
	var out, errb bytes.Buffer
	code := run(autoArgs(t, deadURL(t), "-p", "q"), strings.NewReader(""), &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "Claude is unavailable") {
		t.Fatalf("exit %d stderr %s", code, errb.String())
	}
	if d := logLine(t, errb.String(), "route_decision"); d["decision"] != "none" || d["escalated"] != true {
		t.Errorf("route_decision = %v", d)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q", out.String())
	}
}

// The spend cap is the Claude leg's: a local answer goes through under a
// cap too small for any Claude request; an escalation is refused by it.
func TestAutoSpendCapOnlyOnClaudeLeg(t *testing.T) {
	claude := newFakeClaude(t, "from claude")
	withClaude(t, claude)
	local := newFakeLocal(t, "local fine", "stop")
	var out, errb bytes.Buffer
	if code := run(autoArgs(t, local.srv.URL, "-daily-cap-usd", "0.000001", "-p", "q"), strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("local under tiny cap: exit %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := run(autoArgs(t, deadURL(t), "-daily-cap-usd", "0.000001", "-p", "q"), strings.NewReader(""), &out, &errb); code != 1 {
		t.Fatalf("escalation under tiny cap: exit %d", code)
	}
	if !strings.Contains(errb.String(), "spend") || claude.count() != 0 {
		t.Errorf("cap did not stop the Claude leg (calls %d): %s", claude.count(), errb.String())
	}
}

func TestAutoDryRunSendsNothing(t *testing.T) {
	noCredentials(t)
	local := newFakeLocal(t, "x", "stop")
	var out, errb bytes.Buffer
	if code := run(autoArgs(t, local.srv.URL, "-dry-run", "-check", "true", "-p", "hello"), strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("exit %d %s", code, errb.String())
	}
	s := out.String()
	if !strings.Contains(s, "POST "+local.srv.URL+"/v1/chat/completions") || !strings.Contains(s, "# check: true") || !strings.Contains(s, "POST /v1/messages") {
		t.Errorf("dry run output:\n%s", s)
	}
	if local.count() != 0 {
		t.Errorf("local called %d times in a dry run", local.count())
	}
}

func readCapture(t *testing.T, dir string) []map[string]any {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "zhen-capture-*.jsonl"))
	var recs []map[string]any
	for _, p := range files {
		raw, err := os.ReadFile(p) // #nosec G304 -- test temp dir
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("bad line %q", line)
			}
			recs = append(recs, m)
		}
	}
	return recs
}

func TestCaptureEscalatedTurn(t *testing.T) {
	claude := newFakeClaude(t, "claude answer")
	withClaude(t, claude)
	local := newFakeLocal(t, "local draft", "stop")
	dir := filepath.Join(t.TempDir(), "cap")
	secret := "password=" + strings.Repeat("s", 12)
	var out, errb bytes.Buffer
	code := run(autoArgs(t, local.srv.URL, "-capture", dir, "-check", "false", "-p", "fix my "+secret), strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("exit %d %s", code, errb.String())
	}
	recs := readCapture(t, dir)
	if len(recs) != 1 {
		t.Fatalf("records = %d", len(recs))
	}
	r := recs[0]
	if r["schema"] != "zhen-capture/v1" || r["route"] != "escalated" || r["accepted_by"] != "claude" || r["final"] != "claude answer" {
		t.Errorf("record = %v", r)
	}
	if strings.Contains(r["prompt"].(string), "ssssssss") || r["redactions"] != float64(1) {
		t.Errorf("prompt not redacted: %v (%v)", r["prompt"], r["redactions"])
	}
	l, c, chk := r["local"].(map[string]any), r["claude"].(map[string]any), r["check"].(map[string]any)
	if l["answer"] != "local draft" || l["model"] != "qwen-test" || l["system"] == nil || c["model"] != "claude-opus-5-5" || c["answer"] != "claude answer" || chk["exit"] != float64(1) || chk["accepted"] != false {
		t.Errorf("legs: local %v claude %v check %v", l, c, chk)
	}
	if s, _ := r["system"].(string); !strings.Contains(s, "Champion gate") {
		t.Errorf("top-level system should be Claude's: %v", r["system"])
	}
}

func TestCaptureLocalAndClaudeRoutes(t *testing.T) {
	claude := newFakeClaude(t, "claude only")
	withClaude(t, claude)
	local := newFakeLocal(t, "local only", "stop")
	dir := filepath.Join(t.TempDir(), "cap")
	var out, errb bytes.Buffer
	if code := run(autoArgs(t, local.srv.URL, "-capture", dir, "-p", "a"), strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("auto: %d %s", code, errb.String())
	}
	t.Setenv("ZHEN_CAPTURE_DIR", dir) // -capture defaults to it
	if code := run(append(baseArgs(t), "-tools", "none", "-p", "b"), strings.NewReader(""), &out, &errb); code != 0 {
		t.Fatalf("claude: %d %s", code, errb.String())
	}
	recs := readCapture(t, dir)
	if len(recs) != 2 {
		t.Fatalf("records = %d", len(recs))
	}
	byRoute := map[string]map[string]any{}
	for _, r := range recs {
		byRoute[r["route"].(string)] = r
	}
	if r := byRoute["local"]; r == nil || r["final"] != "local only" || r["accepted_by"] != "local" || r["claude"] != nil {
		t.Errorf("local record = %v", r)
	}
	if r := byRoute["claude"]; r == nil || r["final"] != "claude only" || r["local"] != nil || r["cost_usd"] == nil {
		t.Errorf("claude record = %v", r)
	}
}

func TestCaptureRefusedInsideRepo(t *testing.T) {
	noCredentials(t)
	wd, _ := os.Getwd()
	inside := filepath.Join(wd, "capture-should-not-exist")
	var out, errb bytes.Buffer
	code := run(append(baseArgs(t), "-dry-run", "-capture", inside, "-p", "x"), strings.NewReader(""), &out, &errb)
	if code != 1 || !strings.Contains(errb.String(), "outside any repository") {
		t.Fatalf("exit %d stderr %s", code, errb.String())
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Error("capture dir created inside the repo")
	}
}

func TestRouteUsageErrors(t *testing.T) {
	noCredentials(t)
	for _, args := range [][]string{
		{"-route", "bogus", "-p", "x"},
		{"-check", "true", "-p", "x"},
		{"-route", "auto", "-repl"},
		{"-route", "auto", "-local-timeout", "0s", "-p", "x"},
	} {
		var out, errb bytes.Buffer
		if code := run(append(baseArgs(t), args...), strings.NewReader(""), &out, &errb); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}
