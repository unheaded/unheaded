// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
)

// --- helpers shared by all daemon-side integration tests ---

// scriptedLLM returns canned responses one per call. The agent's loop
// expects either a JSON object {"thought":..., "answer":"..."} for a
// terminal turn, or {"thought":..., "tool_call":{...}} for tool use.
type scriptedLLM struct {
	out   []string
	calls int
}

func (s *scriptedLLM) Complete(_ context.Context, _ []agent.Message, _ int, _ float64, _ int) (string, error) {
	if s.calls >= len(s.out) {
		return "", errors.New("scriptedLLM: out of responses")
	}
	r := s.out[s.calls]
	s.calls++
	return r, nil
}

// constantLLM returns the same response for every call. Useful for
// concurrent tests where scriptedLLM would race on its index counter.
type constantLLM struct {
	out string
}

func (c *constantLLM) Complete(_ context.Context, _ []agent.Message, _ int, _ float64, _ int) (string, error) {
	return c.out, nil
}

// stubRetriever returns a fixed canonical reference set, regardless of
// query. Lets tests skip the vor round-trip entirely.
type stubRetriever struct {
	refs     []champion.Reference
	contents []agent.TopicContent
}

func (r *stubRetriever) Retrieve(_ context.Context, _ string, _ int) ([]champion.Reference, []agent.TopicContent, error) {
	return r.refs, r.contents, nil
}

func canonicalStub() *stubRetriever {
	ref := champion.Reference{
		Topic: "go-error-handling", Category: "languages/go",
		SourceKind: "embedded", SourceTrust: "canonical",
	}
	return &stubRetriever{
		refs:     []champion.Reference{ref},
		contents: []agent.TopicContent{{Ref: ref, Content: "(stub canonical content)"}},
	}
}

// nopActionStore is a no-op ActionStore. Same shape as the production
// stderrActionStore but silent — keeps test logs clean.
type nopActionStore struct{}

func (nopActionStore) LogAction(context.Context, *champion.Action) (int64, error) { return 1, nil }
func (nopActionStore) UpdateAction(context.Context, int64, string, string, string, int) error {
	return nil
}
func (nopActionStore) GetActions(context.Context, string, int) ([]champion.Action, error) {
	return nil, nil
}

// newTestDaemon spins up the full mux + server wrapped in an
// httptest.Server. Returns the server and the Champion-pool tempdir
// (which is also the daemon's defaultRoot). No auth, no rate limit —
// tests assert the handler chain end-to-end, not the middleware
// wrappers (those are unit-tested separately).
func newTestDaemon(t *testing.T, retr agent.Retriever, llm agent.LLM) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	return newTestDaemonWithRoot(t, root, retr, llm), root
}

// newTestDaemonWithRoot is the same as newTestDaemon but lets the
// caller pin the project_root explicitly. Used by tests that need
// the root path embedded in scripted LLM responses.
func newTestDaemonWithRoot(t *testing.T, root string, retr agent.Retriever, llm agent.LLM) *httptest.Server {
	t.Helper()
	store := newMetricsActionStore(nopActionStore{})
	pool := newChampionPool(store)

	srv := &server{
		pool:        pool,
		defaultRoot: root,
		allowed:     map[string]struct{}{root: {}},
		retriever:   retr,
		llm:         llm,
		// vorURL/llamaURL deliberately unreachable so /ready returns 503
		// in the default test setup; tests that need /ready=200 build
		// their own httptest stub backends and rebuild srv.ready.
		vorURL:   "http://127.0.0.1:1",
		llamaURL: "http://127.0.0.1:1",
		ready:    newReadyTracker(&http.Client{}, "http://127.0.0.1:1", "http://127.0.0.1:1"),
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/health", instrument("/health", http.HandlerFunc(srv.handleHealth)))
	mux.Handle("/ready", instrument("/ready", http.HandlerFunc(srv.handleReady)))
	mux.Handle("/api/v1/agent/ask", instrument("/api/v1/agent/ask", http.HandlerFunc(srv.handleAsk)))
	mux.Handle("/api/v1/agent/ask/stream", instrument("/api/v1/agent/ask/stream", http.HandlerFunc(srv.handleAskStream)))
	mux.Handle("/api/v1/agent/confirm", instrument("/api/v1/agent/confirm", http.HandlerFunc(srv.handleConfirm)))
	mux.Handle("/api/v1/openapi.json", instrument("/api/v1/openapi.json", http.HandlerFunc(srv.handleOpenAPI)))

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

func mustPostJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// --- /health ---

func TestIntegration_Health_ReturnsOK(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustGet(t, ts.URL+"/health")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status field: got %q, want %q", body["status"], "ok")
	}
}

// --- /ready ---

func TestIntegration_Ready_503_WhenBackendsUnreachable(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustGet(t, ts.URL+"/ready")
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("status: got %d, want 503", resp.StatusCode)
	}
	var body readyStatus
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Ready {
		t.Fatalf("body.Ready: got true, want false")
	}
	if body.Vor || body.Llama {
		t.Fatalf("backend reachability: got vor=%v llama=%v, want both false", body.Vor, body.Llama)
	}
}

// --- /api/v1/openapi.json ---

func TestIntegration_OpenAPI_ServesSpec(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustGet(t, ts.URL+"/api/v1/openapi.json")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type: got %q, want application/json", got)
	}
	var spec map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if spec["openapi"] != "3.0.3" {
		t.Fatalf("openapi version: got %v, want 3.0.3", spec["openapi"])
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("paths field not an object")
	}
	for _, want := range []string{"/health", "/ready", "/api/v1/agent/ask", "/api/v1/agent/ask/stream", "/api/v1/agent/confirm"} {
		if _, ok := paths[want]; !ok {
			t.Errorf("missing path entry: %s", want)
		}
	}
}

// --- /api/v1/agent/ask ---

func TestIntegration_Ask_HappyPath_FinalAnswer(t *testing.T) {
	llm := &scriptedLLM{out: []string{
		`{"thought":"trivial","answer":"42"}`,
	}}
	ts, _ := newTestDaemon(t, canonicalStub(), llm)
	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask", map[string]any{
		"goal": "what is the answer",
		"seed": 1,
	})
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 200; body=%s", resp.StatusCode, body)
	}
	var ar askResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ar.Answer != "42" {
		t.Fatalf("answer: got %q, want 42", ar.Answer)
	}
	if ar.TurnsUsed != 1 {
		t.Fatalf("turns_used: got %d, want 1", ar.TurnsUsed)
	}
	if ar.BudgetHit {
		t.Fatalf("budget_hit: got true, want false")
	}
	if ar.SessionID == "" {
		t.Fatalf("session_id: empty (expected auto-generated anon-...)")
	}
	if !strings.HasPrefix(ar.SessionID, "anon-") {
		t.Fatalf("session_id: got %q, want anon-* prefix", ar.SessionID)
	}
	if len(ar.Trace) != 1 {
		t.Fatalf("trace len: got %d, want 1", len(ar.Trace))
	}
	if ar.Trace[0].Answer != "42" {
		t.Fatalf("trace[0].answer: got %q, want 42", ar.Trace[0].Answer)
	}
}

func TestIntegration_Ask_RejectsBadProjectRoot(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask", map[string]any{
		"goal":         "anything",
		"project_root": "/var/this-root-is-not-in-the-allowlist-12345",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 403; body=%s", resp.StatusCode, body)
	}
	var er map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(er["error"], "allow-list") {
		t.Fatalf("error message: got %q, want substring 'allow-list'", er["error"])
	}
}

func TestIntegration_Ask_RejectsMissingGoal(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask", map[string]any{
		"goal": "   ",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 400; body=%s", resp.StatusCode, body)
	}
}

func TestIntegration_Ask_RejectsGET(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustGet(t, ts.URL+"/api/v1/agent/ask")
	defer resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Fatalf("status: got %d, want 405", resp.StatusCode)
	}
}

// --- /api/v1/agent/ask/stream ---

// parseSSE consumes a complete SSE response body and returns the
// per-event (name, data) pairs in order. Treats the standard double-
// newline frame separator and dispatches "data:" + "event:" lines.
func parseSSE(t *testing.T, body io.Reader) []sseEvent {
	t.Helper()
	var out []sseEvent
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)

	var cur sseEvent
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if cur.event != "" || cur.data != "" {
				out = append(out, cur)
				cur = sseEvent{}
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.data = strings.TrimPrefix(line, "data: ")
		}
	}
	if cur.event != "" || cur.data != "" {
		out = append(out, cur)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan SSE: %v", err)
	}
	return out
}

type sseEvent struct {
	event string
	data  string
}

func TestIntegration_AskStream_EmitsTurnAndDone(t *testing.T) {
	llm := &scriptedLLM{out: []string{
		`{"thought":"easy","answer":"streamed-ok"}`,
	}}
	ts, _ := newTestDaemon(t, canonicalStub(), llm)

	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask/stream", map[string]any{
		"goal": "stream the answer",
		"seed": 1,
	})
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 200; body=%s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type: got %q, want text/event-stream", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("cache-control: got %q, want no-cache", got)
	}

	events := parseSSE(t, resp.Body)
	if len(events) < 2 {
		t.Fatalf("events: got %d, want >=2 (one turn + done)", len(events))
	}

	// First event must be a turn carrying the model's answer.
	if events[0].event != "turn" {
		t.Fatalf("events[0].event: got %q, want turn", events[0].event)
	}
	var turn traceEntry
	if err := json.Unmarshal([]byte(events[0].data), &turn); err != nil {
		t.Fatalf("decode turn payload: %v (data=%q)", err, events[0].data)
	}
	if turn.Answer != "streamed-ok" {
		t.Fatalf("turn.answer: got %q, want streamed-ok", turn.Answer)
	}

	// Last event must be a done with the final summary.
	last := events[len(events)-1]
	if last.event != "done" {
		t.Fatalf("last event: got %q, want done", last.event)
	}
	var done map[string]any
	if err := json.Unmarshal([]byte(last.data), &done); err != nil {
		t.Fatalf("decode done payload: %v (data=%q)", err, last.data)
	}
	if done["answer"] != "streamed-ok" {
		t.Fatalf("done.answer: got %v, want streamed-ok", done["answer"])
	}
	if v, ok := done["budget_hit"].(bool); !ok || v {
		t.Fatalf("done.budget_hit: got %v, want false", done["budget_hit"])
	}
}

func TestIntegration_AskStream_RejectsBadProjectRoot(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask/stream", map[string]any{
		"goal":         "anything",
		"project_root": "/var/totally-bogus-root-zzz",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 403; body=%s", resp.StatusCode, body)
	}
}

// --- /api/v1/agent/confirm ---

func TestIntegration_Confirm_UnknownToken_Returns400Unknown(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/confirm", map[string]any{
		"token": "deadbeefdeadbeefdeadbeefdeadbeef",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 400; body=%s", resp.StatusCode, body)
	}
	var cr confirmResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cr.Status != "unknown" {
		t.Fatalf("status: got %q, want unknown", cr.Status)
	}
}

func TestIntegration_Confirm_RejectsMissingToken(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/confirm", map[string]any{
		"token": "   ",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestIntegration_Confirm_RejectsBadProjectRoot(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/confirm", map[string]any{
		"token":        "abcd1234",
		"project_root": "/var/no-such-allowlisted-root",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 403; body=%s", resp.StatusCode, body)
	}
}

// --- /api/v1/agent/confirm: happy-path E2E ---

// externalStub returns refs marked as untrusted external sources.
// Drives Champion's Rule 2 (mutating + untrusted justification →
// PendingConfirmation) so /ask issues a confirmation token.
func externalStub() *stubRetriever {
	ref := champion.Reference{
		Topic: "untrusted-doc", Category: ".",
		SourceKind:  "user-source",
		SourceTrust: "external",
		SourceLabel: "external-corpus",
		SourcePath:  "/tmp/external-corpus",
	}
	return &stubRetriever{
		refs:     []champion.Reference{ref},
		contents: []agent.TopicContent{{Ref: ref, Content: "(external content per A2 threat model)"}},
	}
}

// TestIntegration_Confirm_HappyPath exercises the full pending-token
// dance:
//
//  1. /ask is called with an external-ref retriever, so per-turn
//     justification is untrusted.
//  2. Scripted LLM turn 1 emits a write_file tool_call. Champion's
//     gate refuses with PendingConfirmation, token is issued and
//     surfaced in the trace.
//  3. Scripted LLM turn 2 produces a terminal answer (the agent loop
//     keeps running after the refusal — refusal isn't terminal).
//  4. The test extracts the token from the trace and POSTs /confirm.
//  5. Champion redeems: stripped gate (rules 1+3 only) passes, the
//     underlying write_file runs, and the file lands on disk.
//
// This is the load-bearing path for "human in the loop authorized
// this despite the warning" — the security critical confirm flow.
func TestIntegration_Confirm_HappyPath(t *testing.T) {
	// We need the path embedded in the LLM script to land inside the
	// sandbox, but the sandbox root is a tempdir created per-test —
	// so the script is built dynamically.
	root := t.TempDir()
	outPath := filepath.Join(root, "out.txt")
	llm := &scriptedLLM{out: []string{
		// Turn 1: mutating tool call. Will be refused → pending token.
		fmt.Sprintf(`{"thought":"need to write a config","tool_call":{"name":"write_file","args":{"path":%q,"content":"hello"}}}`, outPath),
		// Turn 2: after observation comes back with the refusal, model wraps up.
		`{"thought":"surfaced confirmation","answer":"awaiting user confirmation"}`,
	}}
	ts := newTestDaemonWithRoot(t, root, externalStub(), llm)

	// 1. POST /ask — expect a pending-token in trace.
	resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask", map[string]any{
		"goal": "write the config file",
	})
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("ask status: got %d, want 200; body=%s", resp.StatusCode, body)
	}
	var ar askResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		t.Fatalf("ask decode: %v", err)
	}

	// 2. Find the pending-token entry in the trace.
	var token string
	for _, e := range ar.Trace {
		if e.PendingToken != "" {
			token = e.PendingToken
			if !e.Pending || !e.Refused {
				t.Errorf("pending entry: refused=%v pending=%v, want both true", e.Refused, e.Pending)
			}
			break
		}
	}
	if token == "" {
		t.Fatalf("no pending_token in trace; got trace=%+v", ar.Trace)
	}
	if len(token) < 16 {
		t.Errorf("token too short: %q (want crypto/rand 16-byte hex = 32 chars)", token)
	}

	// 3. POST /confirm with the token.
	confirmResp := mustPostJSON(t, ts.URL+"/api/v1/agent/confirm", map[string]any{
		"token":        token,
		"project_root": root,
	})
	defer confirmResp.Body.Close()
	if confirmResp.StatusCode != 200 {
		body, _ := io.ReadAll(confirmResp.Body)
		t.Fatalf("confirm status: got %d, want 200; body=%s", confirmResp.StatusCode, body)
	}
	var cr confirmResponse
	if err := json.NewDecoder(confirmResp.Body).Decode(&cr); err != nil {
		t.Fatalf("confirm decode: %v", err)
	}
	if cr.Status != "ok" {
		t.Fatalf("confirm status: got %q, want ok (reason=%q)", cr.Status, cr.Reason)
	}

	// 4. Verify side-effect: the file landed at root/out.txt.
	got, err := os.ReadFile(filepath.Join(root, "out.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("file content: got %q, want %q", got, "hello")
	}

	// 5. Replaying the same token must fail as "used".
	replay := mustPostJSON(t, ts.URL+"/api/v1/agent/confirm", map[string]any{
		"token":        token,
		"project_root": root,
	})
	defer replay.Body.Close()
	if replay.StatusCode != 400 {
		t.Fatalf("replay status: got %d, want 400", replay.StatusCode)
	}
	var rcr confirmResponse
	if err := json.NewDecoder(replay.Body).Decode(&rcr); err != nil {
		t.Fatalf("replay decode: %v", err)
	}
	if rcr.Status != "used" {
		t.Fatalf("replay status: got %q, want used", rcr.Status)
	}
}

// --- /metrics ---

func TestIntegration_Metrics_ServesPrometheus(t *testing.T) {
	ts, _ := newTestDaemon(t, canonicalStub(), &scriptedLLM{})
	// Make one request first so there's at least one counter sample.
	mustGet(t, ts.URL+"/health").Body.Close()

	resp := mustGet(t, ts.URL+"/metrics")
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"zhen_agentd_http_requests_total",
		"zhen_agentd_http_request_duration_seconds",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metrics body missing %q", want)
		}
	}
}

// --- concurrency: championPool under parallel load ---

// TestIntegration_Ask_ConcurrentRequests fires N parallel /ask requests
// against the same project_root. Every request shares the same Champion
// (the pool returns the cached entry under a sync.Mutex), so this both
// exercises the pool's locking and the per-request agent-loop code path.
// Race detector earns its keep here — silent under -race is the gate.
func TestIntegration_Ask_ConcurrentRequests(t *testing.T) {
	llm := &constantLLM{out: `{"thought":"const","answer":"shared"}`}
	ts, _ := newTestDaemon(t, canonicalStub(), llm)

	const N = 16
	type result struct {
		err  error
		body askResponse
		code int
	}
	results := make(chan result, N)

	for i := 0; i < N; i++ {
		go func(i int) {
			resp, err := http.Post(
				ts.URL+"/api/v1/agent/ask",
				"application/json",
				strings.NewReader(fmt.Sprintf(`{"goal":"q-%d","session_id":"s-%d"}`, i, i)),
			)
			if err != nil {
				results <- result{err: err}
				return
			}
			defer resp.Body.Close()
			var ar askResponse
			if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
				results <- result{err: fmt.Errorf("decode #%d: %w", i, err)}
				return
			}
			results <- result{body: ar, code: resp.StatusCode}
		}(i)
	}

	for i := 0; i < N; i++ {
		r := <-results
		if r.err != nil {
			t.Errorf("request error: %v", r.err)
			continue
		}
		if r.code != 200 {
			t.Errorf("status: got %d, want 200", r.code)
			continue
		}
		if r.body.Answer != "shared" {
			t.Errorf("answer: got %q, want shared", r.body.Answer)
		}
		if r.body.TurnsUsed != 1 {
			t.Errorf("turns_used: got %d, want 1", r.body.TurnsUsed)
		}
	}
}

// --- multi-request through the pool ---

func TestIntegration_Ask_PoolReusesChampionAcrossRequests(t *testing.T) {
	llm := &scriptedLLM{out: []string{
		`{"thought":"first","answer":"one"}`,
		`{"thought":"second","answer":"two"}`,
	}}
	ts, _ := newTestDaemon(t, canonicalStub(), llm)

	for i, want := range []string{"one", "two"} {
		resp := mustPostJSON(t, ts.URL+"/api/v1/agent/ask", map[string]any{
			"goal":       fmt.Sprintf("question %d", i),
			"session_id": fmt.Sprintf("test-%d", i),
		})
		var ar askResponse
		if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
			resp.Body.Close()
			t.Fatalf("decode #%d: %v", i, err)
		}
		resp.Body.Close()
		if ar.Answer != want {
			t.Fatalf("answer #%d: got %q, want %q", i, ar.Answer, want)
		}
		if ar.SessionID != fmt.Sprintf("test-%d", i) {
			t.Fatalf("session_id #%d: got %q, want test-%d", i, ar.SessionID, i)
		}
	}
}
