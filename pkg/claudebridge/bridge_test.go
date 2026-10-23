// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"

	"unheaded/pkg/champion"
	"unheaded/pkg/zhentools"
)

// recAuditor keeps every record for assertions.
type recAuditor struct {
	mu    sync.Mutex
	reqs  []RequestRecord
	tools []ToolRecord
}

func (a *recAuditor) Request(_ context.Context, r RequestRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.reqs = append(a.reqs, r)
}

func (a *recAuditor) ToolCall(_ context.Context, r ToolRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tools = append(a.tools, r)
}

// fakeProvider is an MCP-like provider: a fixed tool list and a handler.
type fakeProvider struct {
	tools []zhentools.Tool
	call  func(ctx context.Context, name string, input json.RawMessage) (zhentools.Result, error)

	mu    sync.Mutex
	calls []string
}

func (p *fakeProvider) Tools(context.Context) ([]zhentools.Tool, error) { return p.tools, nil }

func (p *fakeProvider) Call(ctx context.Context, name string, input json.RawMessage) (zhentools.Result, error) {
	p.mu.Lock()
	p.calls = append(p.calls, name)
	p.mu.Unlock()
	return p.call(ctx, name, input)
}

func (p *fakeProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func objSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
}

type harness struct {
	api    *fakeAPI
	bridge *Bridge
	champ  *champion.Champion
	root   string
	ledger *Ledger
	audit  *recAuditor
}

type harnessOpt func(*Config)

func newHarness(t *testing.T, responses []cannedMsg, extra []NamedProvider, opts ...harnessOpt) *harness {
	t.Helper()
	root := t.TempDir()
	champ, err := champion.New(champion.Config{ProjectRoot: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cp, err := champion.NewToolProvider(champ, "test")
	if err != nil {
		t.Fatal(err)
	}
	gate, err := NewChampionGate(champ)
	if err != nil {
		t.Fatal(err)
	}
	prices, err := DefaultPrices()
	if err != nil {
		t.Fatal(err)
	}
	capNano, _ := USDToNano(10)
	ledger, err := NewLedger(filepath.Join(t.TempDir(), "spend.json"), capNano)
	if err != nil {
		t.Fatal(err)
	}
	api := newFakeAPI(t, responses...)
	audit := &recAuditor{}
	cfg := Config{
		Providers:     append([]NamedProvider{{Name: "champion", Provider: cp}}, extra...),
		Gate:          gate,
		Prices:        prices,
		Ledger:        ledger,
		Auditor:       audit,
		Credentials:   func() (string, error) { return "test", nil },
		ClientOptions: []option.RequestOption{option.WithBaseURL(api.srv.URL), option.WithAPIKey("test-not-a-key"), option.WithMaxRetries(0)},
	}
	for _, o := range opts {
		o(&cfg)
	}
	b, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &harness{api: api, bridge: b, champ: champ, root: root, ledger: ledger, audit: audit}
}

func endTurn(text string) cannedMsg {
	return cannedMsg{StopReason: "end_turn", Blocks: []cannedBlock{{Text: text}}}
}

func TestSend_ToolUseLoop(t *testing.T) {
	h := newHarness(t, nil, nil)
	target := filepath.Join(h.root, "notes.txt")
	if err := os.WriteFile(target, []byte("the well is on west"), 0o600); err != nil {
		t.Fatal(err)
	}
	in, _ := json.Marshal(map[string]string{"path": target})
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{{Text: "Reading."}, {ToolID: "toolu_1", ToolName: "read_file", ToolInput: string(in)}}},
		endTurn("The Well runs on west."),
	}

	var streamed strings.Builder
	h.bridge.cfg.OnText = func(s string) { streamed.WriteString(s) }
	conv := h.bridge.NewConversation()
	reply, err := conv.Send(context.Background(), "where is the well?")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text != "The Well runs on west." || reply.StopReason != "end_turn" || reply.Requests != 2 {
		t.Errorf("reply = %+v", reply)
	}
	if streamed.String() != "Reading.The Well runs on west." {
		t.Errorf("streamed %q", streamed.String())
	}

	reqs := h.api.reqs()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d", len(reqs))
	}
	results := reqs[1].toolResults(t)
	if len(results) != 1 || results[0]["tool_use_id"] != "toolu_1" || results[0]["is_error"] == true {
		t.Fatalf("tool results = %v", results)
	}
	if got := resultText(results[0]); got != "the well is on west" {
		t.Errorf("tool result text = %q", got)
	}
	// The assistant turn (with its tool_use) precedes the results.
	msgs := reqs[1].messages(t)
	if len(msgs) != 3 || msgs[1]["role"] != "assistant" {
		t.Errorf("history shape: %v", msgs)
	}

	// Spend: two requests at 1000 in / 200 out on claude-opus-5-5.
	want := 2 * (1000*4000 + 200*20000)
	if spent, _ := h.ledger.Spent(); spent != NanoUSD(want) || reply.Cost != NanoUSD(want) {
		t.Errorf("spent = %d, reply cost = %d, want %d", spent, reply.Cost, want)
	}
	if len(h.audit.reqs) != 2 || len(h.audit.tools) != 1 || h.audit.tools[0].Decision != "allowed" || h.audit.tools[0].Provider != "champion" {
		t.Errorf("audit = %+v / %+v", h.audit.reqs, h.audit.tools)
	}
	if h.audit.tools[0].InputSHA256 == "" || h.audit.tools[0].InputBytes != len(in) {
		t.Errorf("tool audit should record input size and hash: %+v", h.audit.tools[0])
	}
}

func TestSend_RequestShape(t *testing.T) {
	h := newHarness(t, []cannedMsg{endTurn("hi")}, nil)
	if _, err := h.bridge.NewConversation().Send(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	r := h.api.reqs()[0]
	b := r.Body
	if b["model"] != "claude-opus-5-5" || b["max_tokens"] != float64(DefaultMaxTokens) || b["stream"] != true {
		t.Errorf("model/max_tokens/stream: %v %v %v", b["model"], b["max_tokens"], b["stream"])
	}
	if th, _ := b["thinking"].(map[string]any); th["type"] != "adaptive" {
		t.Errorf("thinking = %v", b["thinking"])
	}
	if oc, _ := b["output_config"].(map[string]any); oc["effort"] != "medium" {
		t.Errorf("output_config = %v", b["output_config"])
	}
	if b["fallbacks"] != "default" || !strings.Contains(r.Betas, "server-side-fallback-2026-07-01") {
		t.Errorf("fallbacks = %v, betas = %q", b["fallbacks"], r.Betas)
	}
	if _, ok := b["cache_control"]; !ok {
		t.Error("no top-level cache_control")
	}
	if _, ok := b["temperature"]; ok {
		t.Error("sampling parameters must not be sent")
	}
	if tc, ok := b["tool_choice"]; ok {
		t.Errorf("tool_choice must be left to the default (auto), got %v", tc)
	}
	tools, _ := b["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("tools = %v", tools)
	}
	first := tools[0].(map[string]any)
	schema := first["input_schema"].(map[string]any)
	if first["name"] != "read_file" || schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Errorf("tool[0] = %v", first)
	}
	if r.APIKey != "test-not-a-key" {
		t.Errorf("api key header not sent")
	}
}

func TestSend_ParallelToolCallsOneResultMessage(t *testing.T) {
	// Both calls must be in flight together: each waits for the other.
	var started sync.WaitGroup
	started.Add(2)
	mcp := &fakeProvider{
		tools: []zhentools.Tool{
			{Name: "corpus_search", Description: "search", InputSchema: objSchema()},
			{Name: "service_health", Description: "health", InputSchema: objSchema()},
		},
		call: func(_ context.Context, name string, _ json.RawMessage) (zhentools.Result, error) {
			started.Done()
			done := make(chan struct{})
			go func() { started.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				return zhentools.Result{}, errors.New("calls were not run concurrently")
			}
			return zhentools.Result{Text: name + " ok"}, nil
		},
	}
	h := newHarness(t, []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{
			{ToolID: "toolu_a", ToolName: "corpus_search", ToolInput: `{"q":"wotan"}`},
			{ToolID: "toolu_b", ToolName: "service_health", ToolInput: `{"q":"all"}`},
		}},
		endTurn("both fine"),
	}, []NamedProvider{{Name: "mcp:zhen", Provider: mcp}})

	if _, err := h.bridge.NewConversation().Send(context.Background(), "check"); err != nil {
		t.Fatal(err)
	}
	reqs := h.api.reqs()
	msgs := reqs[1].messages(t)
	if len(msgs) != 3 {
		t.Fatalf("want user, assistant, one user message of results; got %d messages", len(msgs))
	}
	results := reqs[1].toolResults(t)
	if len(results) != 2 || results[0]["tool_use_id"] != "toolu_a" || results[1]["tool_use_id"] != "toolu_b" {
		t.Fatalf("results = %v", results)
	}
	for _, r := range results {
		if r["is_error"] == true || !strings.HasSuffix(resultText(r), " ok") {
			t.Errorf("result %v", r)
		}
	}
}

func TestSend_GateRefusalReachesClaudeAsIsError(t *testing.T) {
	// file_write is an MCP tool the Champion does not know: fail-closed
	// as mutating, refused before the provider is ever called.
	mcp := &fakeProvider{
		tools: []zhentools.Tool{{Name: "file_write", Description: "write", InputSchema: objSchema()}},
		call: func(context.Context, string, json.RawMessage) (zhentools.Result, error) {
			return zhentools.Result{Text: "wrote"}, nil
		},
	}
	h := newHarness(t, nil, []NamedProvider{{Name: "mcp:zhen", Provider: mcp}})
	target := filepath.Join(h.root, "x.go")
	w1, _ := json.Marshal(map[string]string{"path": target, "content": "package x"})
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{
			{ToolID: "toolu_w", ToolName: "write_file", ToolInput: string(w1)},
			{ToolID: "toolu_m", ToolName: "file_write", ToolInput: `{"q":"x"}`},
			{ToolID: "toolu_d", ToolName: "read_file", ToolInput: `{"path":"rm -rf /"}`},
		}},
		endTurn("I need your confirmation to write that file."),
	}
	reply, err := h.bridge.NewConversation().Send(context.Background(), "write x.go")
	if err != nil {
		t.Fatal(err)
	}
	if reply.StopReason != "end_turn" {
		t.Errorf("stop = %s", reply.StopReason)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("write_file ran despite the gate")
	}
	if mcp.callCount() != 0 {
		t.Error("refused MCP tool reached its provider")
	}
	results := h.api.reqs()[1].toolResults(t)
	if len(results) != 3 {
		t.Fatalf("results = %v", results)
	}
	for _, r := range results {
		if r["is_error"] != true || !strings.Contains(resultText(r), "refused by gate") {
			t.Errorf("want is_error refusal, got %v", r)
		}
	}
	decisions := map[string]ToolRecord{}
	for _, tr := range h.audit.tools {
		decisions[tr.ToolUseID] = tr
	}
	if d := decisions["toolu_w"]; d.Decision != "refused" || !d.Pending {
		t.Errorf("write_file audit = %+v", d)
	}
	if d := decisions["toolu_d"]; d.Decision != "refused" || d.Pending || !strings.Contains(d.Reason, "destructive") {
		t.Errorf("destructive read audit = %+v", d)
	}
}

func TestSend_UnknownToolAndBadInput(t *testing.T) {
	h := newHarness(t, []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{
			{ToolID: "toolu_u", ToolName: "launch_missiles", ToolInput: `{}`},
			{ToolID: "toolu_b", ToolName: "read_file", ToolInput: `"just a string"`},
		}},
		endTurn("ok"),
	}, nil)
	if _, err := h.bridge.NewConversation().Send(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	results := h.api.reqs()[1].toolResults(t)
	if len(results) != 2 || results[0]["is_error"] != true || results[1]["is_error"] != true {
		t.Fatalf("results = %v", results)
	}
}

func TestSend_RefusalStopReason(t *testing.T) {
	h := newHarness(t, []cannedMsg{{
		StopReason:  "refusal",
		StopDetails: map[string]any{"type": "refusal", "category": "cyber", "explanation": "declined"},
		Blocks:      []cannedBlock{{Text: "partial "}},
	}}, nil)
	conv := h.bridge.NewConversation()
	reply, err := conv.Send(context.Background(), "something")
	var re *RefusalError
	if !errors.As(err, &re) || re.Category != "cyber" {
		t.Fatalf("err = %v", err)
	}
	if reply.StopReason != "refusal" {
		t.Errorf("reply = %+v", reply)
	}
	if len(conv.messages) != 0 {
		t.Errorf("refused turn left %d messages in history", len(conv.messages))
	}
	if h.audit.reqs[0].RefusalCategory != "cyber" {
		t.Errorf("audit = %+v", h.audit.reqs[0])
	}
	// The refused request is still billed for what it used.
	if spent, _ := h.ledger.Spent(); spent == 0 {
		t.Error("refused request should still be recorded against the cap")
	}
}

func TestSend_MaxTokensDoesNotRunTools(t *testing.T) {
	mcp := &fakeProvider{
		tools: []zhentools.Tool{{Name: "corpus_search", InputSchema: objSchema()}},
		call: func(context.Context, string, json.RawMessage) (zhentools.Result, error) {
			return zhentools.Result{Text: "x"}, nil
		},
	}
	h := newHarness(t, []cannedMsg{{
		StopReason: "max_tokens",
		Blocks:     []cannedBlock{{Text: "Let me"}, {ToolID: "toolu_t", ToolName: "corpus_search", ToolInput: `{"q":"tru`}},
	}}, []NamedProvider{{Name: "mcp:zhen", Provider: mcp}})
	conv := h.bridge.NewConversation()
	_, err := conv.Send(context.Background(), "search")
	if !errors.Is(err, ErrMaxTokens) {
		t.Fatalf("err = %v", err)
	}
	if mcp.callCount() != 0 {
		t.Error("a tool from a max_tokens-truncated turn was run")
	}
	if len(conv.messages) != 0 {
		t.Error("history not rolled back")
	}
}

func TestSend_SpendCapRefusesBeforeSending(t *testing.T) {
	tiny, _ := USDToNano(0.01)
	h := newHarness(t, []cannedMsg{endTurn("never")}, nil, func(c *Config) {
		l, err := NewLedger("", tiny)
		if err != nil {
			t.Fatal(err)
		}
		c.Ledger = l
	})
	_, err := h.bridge.NewConversation().Send(context.Background(), "hello")
	var sce *SpendCapError
	if !errors.Is(err, ErrSpendCap) || !errors.As(err, &sce) || sce.Need <= tiny {
		t.Fatalf("err = %v", err)
	}
	if n := len(h.api.reqs()); n != 0 {
		t.Fatalf("%d request(s) reached the API past the cap", n)
	}
	if len(h.audit.reqs) != 1 || h.audit.reqs[0].Sent || h.audit.reqs[0].Err == "" {
		t.Errorf("cap refusal should be audited as not sent: %+v", h.audit.reqs)
	}
}

func TestSend_SpendCapStopsMidLoop(t *testing.T) {
	// Room for one worst-case reservation (16000 out at $25/MTok, doubled
	// for the fallback hop: $0.80, plus input) but not for a second one on
	// top of the first request's real cost ($0.324).
	capNano, _ := USDToNano(1.00)
	h := newHarness(t, []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{{ToolID: "toolu_1", ToolName: "read_file", ToolInput: `{"path":"/nope"}`}}, Usage: map[string]any{"output_tokens": 16000}},
		endTurn("unreachable"),
	}, nil, func(c *Config) {
		l, _ := NewLedger("", capNano)
		c.Ledger = l
	})
	_, err := h.bridge.NewConversation().Send(context.Background(), "go")
	if !errors.Is(err, ErrSpendCap) {
		t.Fatalf("err = %v", err)
	}
	if n := len(h.api.reqs()); n != 1 {
		t.Fatalf("requests sent = %d, want 1", n)
	}
}

func TestSend_IterationLimit(t *testing.T) {
	loop := cannedMsg{StopReason: "tool_use", Blocks: []cannedBlock{{ToolID: "toolu_x", ToolName: "read_file", ToolInput: `{"path":"/nope"}`}}}
	h := newHarness(t, []cannedMsg{loop, loop, loop}, nil, func(c *Config) { c.MaxIterations = 2 })
	conv := h.bridge.NewConversation()
	_, err := conv.Send(context.Background(), "go")
	if !errors.Is(err, ErrIterationLimit) {
		t.Fatalf("err = %v", err)
	}
	if n := len(h.api.reqs()); n != 2 {
		t.Errorf("requests = %d", n)
	}
	if len(conv.messages) != 0 {
		t.Error("history not rolled back")
	}
}

func TestSend_ConversationContinues(t *testing.T) {
	h := newHarness(t, []cannedMsg{endTurn("one"), endTurn("two")}, nil)
	conv := h.bridge.NewConversation()
	for _, p := range []string{"first", "second"} {
		if _, err := conv.Send(context.Background(), p); err != nil {
			t.Fatal(err)
		}
	}
	msgs := h.api.reqs()[1].messages(t)
	if len(msgs) != 3 || msgs[1]["role"] != "assistant" {
		t.Errorf("second request history = %v", msgs)
	}
}

func TestSend_APIErrorRollsBack(t *testing.T) {
	h := newHarness(t, nil, nil) // no canned response: the fake returns 400
	conv := h.bridge.NewConversation()
	if _, err := conv.Send(context.Background(), "x"); err == nil {
		t.Fatal("want error")
	}
	if len(conv.messages) != 0 {
		t.Error("history not rolled back")
	}
	if spent, _ := h.ledger.Spent(); spent != 0 {
		t.Errorf("a request rejected before message_start was charged %d", spent)
	}
}

func TestToolResultTruncated(t *testing.T) {
	big := &fakeProvider{
		tools: []zhentools.Tool{{Name: "corpus_search", InputSchema: objSchema()}},
		call: func(context.Context, string, json.RawMessage) (zhentools.Result, error) {
			return zhentools.Result{Text: strings.Repeat("é", 1000)}, nil
		},
	}
	h := newHarness(t, []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{{ToolID: "toolu_1", ToolName: "corpus_search", ToolInput: `{}`}}},
		endTurn("ok"),
	}, []NamedProvider{{Name: "mcp", Provider: big}}, func(c *Config) { c.MaxToolResultBytes = 100 })
	if _, err := h.bridge.NewConversation().Send(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	got := resultText(h.api.reqs()[1].toolResults(t)[0])
	if len(got) > 100 || !strings.HasSuffix(got, "[truncated by claudebridge]") || !json.Valid([]byte(`"`+strings.TrimSuffix(got, "\n[truncated by claudebridge]")+`"`)) {
		t.Errorf("truncated result = %q (%d bytes)", got, len(got))
	}
}

func TestNew_Validation(t *testing.T) {
	prices, _ := DefaultPrices()
	ledger, _ := NewLedger("", 1)
	gate := &ChampionGate{}
	creds := func() (string, error) { return "test", nil }
	cases := map[string]Config{
		"no gate":     {Prices: prices, Ledger: ledger, Credentials: creds},
		"no ledger":   {Gate: gate, Prices: prices, Credentials: creds},
		"bad effort":  {Gate: gate, Prices: prices, Ledger: ledger, Credentials: creds, Effort: "extreme"},
		"tiny budget": {Gate: gate, Prices: prices, Ledger: ledger, Credentials: creds, TaskBudget: 100},
		"huge tokens": {Gate: gate, Prices: prices, Ledger: ledger, Credentials: creds, MaxTokens: 1 << 30},
		"dup tools": {Gate: gate, Prices: prices, Ledger: ledger, Credentials: creds, Providers: []NamedProvider{
			{Name: "a", Provider: &fakeProvider{tools: []zhentools.Tool{{Name: "t"}}}},
			{Name: "b", Provider: &fakeProvider{tools: []zhentools.Tool{{Name: "t"}}}},
		}},
		"bad tool name": {Gate: gate, Prices: prices, Ledger: ledger, Credentials: creds, Providers: []NamedProvider{
			{Name: "a", Provider: &fakeProvider{tools: []zhentools.Tool{{Name: "has space"}}}},
		}},
		"array schema": {Gate: gate, Prices: prices, Ledger: ledger, Credentials: creds, Providers: []NamedProvider{
			{Name: "a", Provider: &fakeProvider{tools: []zhentools.Tool{{Name: "t", InputSchema: json.RawMessage(`{"type":"array"}`)}}}},
		}},
	}
	for name, cfg := range cases {
		if _, err := New(context.Background(), cfg); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestNew_MissingCredentialsFailsLoud(t *testing.T) {
	clearCredentialEnv(t)
	prices, _ := DefaultPrices()
	ledger, _ := NewLedger("", 1)
	_, err := New(context.Background(), Config{Gate: &ChampionGate{}, Prices: prices, Ledger: ledger})
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("err = %v", err)
	}
}

func TestTaskBudget(t *testing.T) {
	h := newHarness(t, []cannedMsg{endTurn("ok")}, nil, func(c *Config) { c.TaskBudget = 40000; c.DisableFallbacks = true })
	if _, err := h.bridge.NewConversation().Send(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	r := h.api.reqs()[0]
	oc := r.Body["output_config"].(map[string]any)
	tb, _ := oc["task_budget"].(map[string]any)
	if tb["total"] != float64(40000) || tb["type"] != "tokens" || !strings.Contains(r.Betas, "task-budgets-2026-03-13") {
		t.Errorf("task budget not sent: %v %q", oc, r.Betas)
	}
	if _, ok := r.Body["fallbacks"]; ok || strings.Contains(r.Betas, "server-side-fallback") {
		t.Error("fallbacks sent although disabled")
	}
}

func TestRequestJSON_DryRun(t *testing.T) {
	h := newHarness(t, nil, nil)
	conv := h.bridge.NewConversation()
	body, err := conv.RequestJSON("hello there")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["model"] != DefaultModel || !strings.Contains(string(body), "hello there") {
		t.Errorf("dry run body: %s", body)
	}
	if len(h.api.reqs()) != 0 || len(conv.messages) != 0 {
		t.Error("dry run sent a request or changed history")
	}
}

func TestTruncateUTF8(t *testing.T) {
	s, cut := truncateUTF8("short", 100)
	if s != "short" || cut {
		t.Error("short string changed")
	}
}

func TestAllowTools_FiltersAndRefuses(t *testing.T) {
	mcp := &fakeProvider{
		tools: []zhentools.Tool{
			{Name: "file_read", InputSchema: objSchema()},
			{Name: "file_write", InputSchema: objSchema()},
			{Name: "runbook_execute", InputSchema: objSchema()},
		},
		call: func(context.Context, string, json.RawMessage) (zhentools.Result, error) {
			return zhentools.Result{Text: "ran"}, nil
		},
	}
	p := AllowTools(mcp, MCPReadOnlyTools...)
	tools, err := p.Tools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "file_read" {
		t.Fatalf("tools = %v, %v", tools, err)
	}
	res, err := p.Call(context.Background(), "runbook_execute", json.RawMessage(`{}`))
	if err != nil || !res.IsError || mcp.callCount() != 0 {
		t.Errorf("non-allowlisted call: %+v %v (provider calls %d)", res, err, mcp.callCount())
	}
}

// An allowlisted MCP read still goes through the Champion gate: Rule 1
// (path allowlist) refuses a read outside the sandbox.
func TestSend_MCPReadOnlyStillGated(t *testing.T) {
	mcp := &fakeProvider{
		tools: []zhentools.Tool{{Name: "file_read", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}},
		call: func(context.Context, string, json.RawMessage) (zhentools.Result, error) {
			return zhentools.Result{Text: "contents"}, nil
		},
	}
	h := newHarness(t, nil, []NamedProvider{{Name: "mcp:zhen", Provider: AllowTools(mcp, MCPReadOnlyTools...)}})
	inside, _ := json.Marshal(map[string]string{"path": filepath.Join(h.root, "a.txt")})
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{
			{ToolID: "toolu_in", ToolName: "file_read", ToolInput: string(inside)},
			{ToolID: "toolu_out", ToolName: "file_read", ToolInput: `{"path":"/etc/shadow"}`},
		}},
		endTurn("done"),
	}
	if _, err := h.bridge.NewConversation().Send(context.Background(), "read"); err != nil {
		t.Fatal(err)
	}
	results := h.api.reqs()[1].toolResults(t)
	if results[0]["is_error"] == true || resultText(results[0]) != "contents" {
		t.Errorf("in-sandbox read: %v", results[0])
	}
	if results[1]["is_error"] != true || !strings.Contains(resultText(results[1]), "refused by gate") {
		t.Errorf("out-of-sandbox read: %v", results[1])
	}
	if mcp.callCount() != 1 {
		t.Errorf("provider calls = %d, want 1", mcp.callCount())
	}
}
