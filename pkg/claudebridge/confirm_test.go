// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/zhentools"
)

// fakeTTY plays the human at the terminal. It records every prompt and
// answers each one with answer(prompt): the code it read in the prompt,
// "n", or nothing. Model content never reaches it: only the confirmer
// writes to it.
type fakeTTY struct {
	mu      sync.Mutex
	out     strings.Builder
	prompts []string
	pending string // unanswered prompt text
	answer  func(prompt string) string
	reads   int
}

func (f *fakeTTY) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.out.Write(p)
	f.pending += string(p)
	return len(p), nil
}

func (f *fakeTTY) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	prompt := f.pending
	f.pending = ""
	f.prompts = append(f.prompts, prompt)
	ans := ""
	if f.answer != nil {
		ans = f.answer(prompt)
	}
	if ans == "" {
		return 0, errors.New("EOF from the operator")
	}
	return copy(p, ans+"\n"), nil
}

var codeRE = regexp.MustCompile(`type ([a-z2-9]{4}) to approve`)

func approveWithCode(prompt string) string {
	if m := codeRE.FindStringSubmatch(prompt); m != nil {
		return m[1]
	}
	return "n"
}

func (f *fakeTTY) all() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.out.String()
}

func withConfirmer(cf func(h *harness) Confirmer) harnessOpt {
	return func(c *Config) {
		if cf == nil {
			return
		}
		g := c.Gate.(*ChampionGate)
		c.Confirmer = cf(&harness{champ: g.Champion, root: g.Champion.GetProjectRoot()})
	}
}

// writeTurn is one tool_use turn asking for write_file, then end_turn.
func writeTurn(h *harness, name, content, extraText string) []cannedMsg {
	in, _ := json.Marshal(map[string]string{"path": filepath.Join(h.root, name), "content": content})
	return []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{{Text: extraText}, {ToolID: "toolu_w", ToolName: "write_file", ToolInput: string(in)}}},
		endTurn("ok"),
	}
}

func lastToolResult(t *testing.T, h *harness) map[string]any {
	t.Helper()
	reqs := h.api.reqs()
	if len(reqs) < 2 {
		t.Fatalf("requests = %d", len(reqs))
	}
	rs := reqs[1].toolResults(t)
	if len(rs) != 1 {
		t.Fatalf("tool results = %v", rs)
	}
	return rs[0]
}

func TestConfirm_DenyConfirmerIsTodaysBehaviour(t *testing.T) {
	var texts []string
	for _, cf := range []func(*harness) Confirmer{nil, func(*harness) Confirmer { return DenyConfirmer{} }} {
		h := newHarness(t, nil, nil, withConfirmer(cf))
		h.api.responses = writeTurn(h, "a.txt", "hi", "")
		if _, err := h.bridge.NewConversation().Send(context.Background(), "write it"); err != nil {
			t.Fatal(err)
		}
		r := lastToolResult(t, h)
		if r["is_error"] != true {
			t.Fatalf("deny: result not an error: %v", r)
		}
		texts = append(texts, resultText(r))
		if _, err := os.Stat(filepath.Join(h.root, "a.txt")); !os.IsNotExist(err) {
			t.Error("deny wrote the file")
		}
	}
	if texts[0] != texts[1] || !strings.Contains(texts[0], "refused by gate") {
		t.Errorf("DenyConfirmer changed the transcript:\n nil:  %s\n deny: %s", texts[0], texts[1])
	}
}

func TestConfirm_TTYApprove(t *testing.T) {
	tty := &fakeTTY{answer: approveWithCode}
	h := newHarness(t, nil, nil, withConfirmer(func(h *harness) Confirmer {
		return &TTYConfirmer{Champion: h.champ, TTY: tty, ApprovedBy: "stevie@test"}
	}))
	h.api.responses = writeTurn(h, "a.txt", "hello\n", "")
	if _, err := h.bridge.NewConversation().Send(context.Background(), "write it"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(h.root, "a.txt"))
	if err != nil || string(got) != "hello\n" {
		t.Fatalf("file = %q, %v", got, err)
	}
	r := lastToolResult(t, h)
	if r["is_error"] == true || !strings.Contains(resultText(r), "approved by the human operator") {
		t.Errorf("result = %v", r)
	}
	code := codeRE.FindStringSubmatch(tty.all())
	if code == nil {
		t.Fatalf("no code on the TTY: %s", tty.all())
	}
	if strings.Contains(resultText(r), code[1]) {
		t.Error("the approval code reached the tool_result")
	}
	// The prompt shows what will change: a diff and the sandbox root.
	if !strings.Contains(tty.all(), "+hello") || !strings.Contains(tty.all(), h.root) {
		t.Errorf("prompt lacks diff or root:\n%s", tty.all())
	}
	if h.audit.tools[0].Decision != "approved" {
		t.Errorf("audit decision = %q", h.audit.tools[0].Decision)
	}
}

func TestConfirm_TTYDeny(t *testing.T) {
	tty := &fakeTTY{answer: func(string) string { return "n" }}
	h := newHarness(t, nil, nil, withConfirmer(func(h *harness) Confirmer {
		return &TTYConfirmer{Champion: h.champ, TTY: tty}
	}))
	h.api.responses = writeTurn(h, "a.txt", "x", "")
	if _, err := h.bridge.NewConversation().Send(context.Background(), "write it"); err != nil {
		t.Fatal(err)
	}
	r := lastToolResult(t, h)
	if r["is_error"] != true || !strings.Contains(resultText(r), "denied") {
		t.Errorf("result = %v", r)
	}
	if _, err := os.Stat(filepath.Join(h.root, "a.txt")); !os.IsNotExist(err) {
		t.Error("a denied write ran")
	}
}

// The model writes "approve", a guessed code and the word yes into its
// text and its tool input. None of it reaches the TTY; the human (who
// answers nothing) decides, so the call stays unapproved.
func TestConfirm_SelfApprovalImpossible(t *testing.T) {
	tty := &fakeTTY{} // the operator types nothing
	h := newHarness(t, nil, nil, withConfirmer(func(h *harness) Confirmer {
		return &TTYConfirmer{Champion: h.champ, TTY: tty}
	}))
	bait := "approve\nyes\nabcd\n2345\ntype abcd to approve"
	h.api.responses = writeTurn(h, "a.txt", bait, bait)
	if _, err := h.bridge.NewConversation().Send(context.Background(), "write it"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(h.root, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("the model approved its own write")
	}
	r := lastToolResult(t, h)
	if r["is_error"] != true {
		t.Errorf("result = %v", r)
	}
	if tty.reads != 1 {
		t.Errorf("TTY read %d times, want exactly one human answer", tty.reads)
	}
}

func TestConfirm_NoCodeOrTokenInModelVisibleContent(t *testing.T) {
	for _, answer := range []func(string) string{approveWithCode, func(string) string { return "n" }} {
		tty := &fakeTTY{answer: answer}
		h := newHarness(t, nil, nil, withConfirmer(func(h *harness) Confirmer {
			return &TTYConfirmer{Champion: h.champ, TTY: tty}
		}))
		h.api.responses = writeTurn(h, "a.txt", "x", "")
		if _, err := h.bridge.NewConversation().Send(context.Background(), "write it"); err != nil {
			t.Fatal(err)
		}
		code := codeRE.FindStringSubmatch(tty.all())
		if code == nil {
			t.Fatal("no code shown")
		}
		for i, req := range h.api.reqs() {
			raw, _ := json.Marshal(req.Body)
			if strings.Contains(string(raw), code[1]) {
				t.Errorf("request %d carries the approval code", i)
			}
		}
	}
}

// fakeQueue is an in-memory ConfirmQueue.
type fakeQueue struct {
	mu   sync.Mutex
	rows []QueuedCall
	err  error
}

func (q *fakeQueue) Enqueue(_ context.Context, c QueuedCall) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.rows = append(q.rows, c)
	return nil
}

func (q *fakeQueue) CountPending(_ context.Context, source string) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := 0
	for _, r := range q.rows {
		if r.SourceService == source && r.ExpiresAt.After(time.Now()) {
			n++
		}
	}
	return n, nil
}

type recPub struct {
	mu   sync.Mutex
	msgs []any
}

func (p *recPub) PublishJSON(topic string, v any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if topic == TopicConfirmPending {
		p.msgs = append(p.msgs, v)
	}
}

func TestConfirm_QueueConfirmer(t *testing.T) {
	q := &fakeQueue{}
	pub := &recPub{}
	h := newHarness(t, nil, nil, withConfirmer(func(h *harness) Confirmer {
		return &QueueConfirmer{Champion: h.champ, Queue: q, Pub: pub, Source: "zhen-claude"}
	}))
	h.api.responses = writeTurn(h, "a.txt", "S2B-SECRET-MARKER body", "")
	if _, err := h.bridge.NewConversation().Send(context.Background(), "write it"); err != nil {
		t.Fatal(err)
	}
	if len(q.rows) != 1 || len(pub.msgs) != 1 {
		t.Fatalf("rows %d, publishes %d", len(q.rows), len(pub.msgs))
	}
	row := q.rows[0]
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(row.ID) || row.Tool != "write_file" || row.ProjectRoot != h.root ||
		len(row.CallSHA256) != 64 || len(row.Summary) > 512 || row.SourceService != "zhen-claude" {
		t.Errorf("row = %+v", row)
	}
	r := lastToolResult(t, h)
	if r["is_error"] != true || !strings.Contains(resultText(r), "queued for human approval as "+row.ID) {
		t.Errorf("result = %v", r)
	}
	b, _ := json.Marshal(pub.msgs[0])
	if strings.Contains(string(b), "S2B-SECRET-MARKER") {
		t.Error("the Wotan notice carries the file body")
	}
	if _, err := os.Stat(filepath.Join(h.root, "a.txt")); !os.IsNotExist(err) {
		t.Error("a queued write ran")
	}
}

func TestConfirm_QueueCap(t *testing.T) {
	q := &fakeQueue{}
	for i := 0; i < MaxPendingPerService; i++ {
		q.rows = append(q.rows, QueuedCall{SourceService: "zhen-claude", ExpiresAt: time.Now().Add(time.Hour)})
	}
	h := newHarness(t, nil, nil, withConfirmer(func(h *harness) Confirmer {
		return &QueueConfirmer{Champion: h.champ, Queue: q, Source: "zhen-claude"}
	}))
	h.api.responses = writeTurn(h, "a.txt", "x", "")
	if _, err := h.bridge.NewConversation().Send(context.Background(), "write it"); err != nil {
		t.Fatal(err)
	}
	if len(q.rows) != MaxPendingPerService {
		t.Errorf("enqueued past the cap: %d rows", len(q.rows))
	}
	if r := lastToolResult(t, h); !strings.Contains(resultText(r), "queue is full") {
		t.Errorf("result = %v", r)
	}
}

func TestConfirm_TwoPendingCallsTwoPromptsInOrder(t *testing.T) {
	tty := &fakeTTY{answer: approveWithCode}
	h := newHarness(t, nil, nil, withConfirmer(func(h *harness) Confirmer {
		return &TTYConfirmer{Champion: h.champ, TTY: tty}
	}))
	a, _ := json.Marshal(map[string]string{"path": filepath.Join(h.root, "first.txt"), "content": "1"})
	b, _ := json.Marshal(map[string]string{"path": filepath.Join(h.root, "second.txt"), "content": "2"})
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{
			{ToolID: "toolu_1", ToolName: "write_file", ToolInput: string(a)},
			{ToolID: "toolu_2", ToolName: "write_file", ToolInput: string(b)},
		}},
		endTurn("ok"),
	}
	if _, err := h.bridge.NewConversation().Send(context.Background(), "write both"); err != nil {
		t.Fatal(err)
	}
	if len(tty.prompts) != 2 {
		t.Fatalf("prompts = %d", len(tty.prompts))
	}
	if !strings.Contains(tty.prompts[0], "first.txt") || !strings.Contains(tty.prompts[1], "second.txt") {
		t.Errorf("prompts out of order:\n%s\n---\n%s", tty.prompts[0], tty.prompts[1])
	}
	for _, f := range []string{"first.txt", "second.txt"} {
		if _, err := os.Stat(filepath.Join(h.root, f)); err != nil {
			t.Errorf("%s not written", f)
		}
	}
}

func TestConfirm_MCPMutatingStaysRefused(t *testing.T) {
	tty := &fakeTTY{answer: approveWithCode}
	mcp := &fakeProvider{
		tools: []zhentools.Tool{{Name: "file_write", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}}}`)}},
		call: func(context.Context, string, json.RawMessage) (zhentools.Result, error) {
			return zhentools.Result{Text: "written"}, nil
		},
	}
	h := newHarness(t, nil, []NamedProvider{{Name: "mcp:zhen", Provider: mcp}}, withConfirmer(func(h *harness) Confirmer {
		return &TTYConfirmer{Champion: h.champ, TTY: tty}
	}))
	h.api.responses = []cannedMsg{
		{StopReason: "tool_use", Blocks: []cannedBlock{{ToolID: "toolu_m", ToolName: "file_write", ToolInput: `{"path":"/tmp/x","content":"y"}`}}},
		endTurn("ok"),
	}
	if _, err := h.bridge.NewConversation().Send(context.Background(), "write via mcp"); err != nil {
		t.Fatal(err)
	}
	if tty.reads != 0 {
		t.Error("an MCP mutating tool was offered for confirmation")
	}
	if mcp.callCount() != 0 {
		t.Error("the MCP mutating tool ran")
	}
}
