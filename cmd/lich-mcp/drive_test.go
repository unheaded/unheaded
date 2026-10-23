// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/lichtools"
)

// --- a minimal fake Messages API (SSE), independent of the claudebridge test
// harness which is not importable from this package. ---

type fakeBlock struct {
	text             string
	toolID, toolName string
	toolInput        string
}

type fakeMsg struct {
	stop   string
	blocks []fakeBlock
}

type fakeMessages struct {
	t    *testing.T
	mu   sync.Mutex
	resp []fakeMsg
	n    int
}

func newFakeMessages(t *testing.T, resp ...fakeMsg) *httptest.Server {
	f := &fakeMessages{t: t, resp: resp}
	srv := httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeMessages) handle(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/v1/messages") {
		http.Error(w, "nf", http.StatusNotFound)
		return
	}
	_, _ = io.ReadAll(r.Body)
	f.mu.Lock()
	if len(f.resp) == 0 {
		f.mu.Unlock()
		http.Error(w, `{"type":"error","error":{"type":"invalid_request_error","message":"no canned response"}}`, http.StatusBadRequest)
		return
	}
	m := f.resp[0]
	f.resp = f.resp[1:]
	f.n++
	id := fmt.Sprintf("msg_%d", f.n)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	ev := func(name string, data any) {
		b, _ := json.Marshal(data)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
	}
	ev("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": id, "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"content": []any{}, "stop_reason": nil, "usage": map[string]any{"input_tokens": 1000, "output_tokens": 1},
	}})
	for i, b := range m.blocks {
		if b.toolName == "" {
			ev("content_block_start", map[string]any{"type": "content_block_start", "index": i, "content_block": map[string]any{"type": "text", "text": ""}})
			ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": i, "delta": map[string]any{"type": "text_delta", "text": b.text}})
		} else {
			ev("content_block_start", map[string]any{"type": "content_block_start", "index": i, "content_block": map[string]any{"type": "tool_use", "id": b.toolID, "name": b.toolName, "input": map[string]any{}}})
			ev("content_block_delta", map[string]any{"type": "content_block_delta", "index": i, "delta": map[string]any{"type": "input_json_delta", "partial_json": b.toolInput}})
		}
		ev("content_block_stop", map[string]any{"type": "content_block_stop", "index": i})
	}
	ev("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": m.stop}, "usage": map[string]any{"input_tokens": 1000, "output_tokens": 200}})
	ev("message_stop", map[string]any{"type": "message_stop"})
}

// recordingAuditor captures the tool records so the test can assert that no
// prompt/response TEXT is recorded — only sizes and hashes.
type recordingAuditor struct {
	mu    sync.Mutex
	tools []claudebridge.ToolRecord
}

func (a *recordingAuditor) Request(context.Context, claudebridge.RequestRecord) {}
func (a *recordingAuditor) ToolCall(_ context.Context, r claudebridge.ToolRecord) {
	a.mu.Lock()
	a.tools = append(a.tools, r)
	a.mu.Unlock()
}

func buildTestBridge(t *testing.T, apiURL string, store *lichtools.PendingStore, provider *lichtools.Provider, aud claudebridge.Auditor) (*claudebridge.Bridge, *claudebridge.Ledger) {
	t.Helper()
	champ, err := champion.New(champion.Config{ProjectRoot: t.TempDir(), SessionID: "test"}, nil)
	if err != nil {
		t.Fatalf("champion: %v", err)
	}
	cg, err := claudebridge.NewChampionGate(champ)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	gate := newConfirmGate(cg, store, t.TempDir(), nil)
	dailyCap, _ := claudebridge.USDToNano(3.0)
	ledger, _ := claudebridge.NewLedger("", dailyCap)
	prices, _ := claudebridge.DefaultPrices()
	b, err := claudebridge.New(context.Background(), claudebridge.Config{
		Model:         "claude-opus-5-5",
		Effort:        "medium",
		Providers:     []claudebridge.NamedProvider{{Name: "lich", Provider: provider}},
		Gate:          gate,
		Prices:        prices,
		Ledger:        ledger,
		Auditor:       aud,
		Credentials:   func() (string, error) { return "test", nil },
		ClientOptions: []option.RequestOption{option.WithBaseURL(apiURL), option.WithAPIKey("k")},
	})
	if err != nil {
		t.Fatalf("bridge: %v", err)
	}
	return b, ledger
}

// Step 149/162: a scripted triage turn — Claude reads a crash (allowed) then
// files a finding (refused pending; parks a ticket with the dedup key + repro
// hash). The loop is bounded; spend is reserved and committed per request.
func TestDrive_TriageTurnParksFinding(t *testing.T) {
	store, err := lichtools.NewPendingStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	cfg := &lichtools.Config{ProjectRoot: t.TempDir()}
	cfg.Scope.SandboxPrefix = lichtools.DefaultSandboxPrefix
	cfg.Scope.HarnessDir = t.TempDir()
	provider, err := lichtools.NewProvider(cfg)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}

	findingInput := `{"severity":"high","summary":"parser OOB read","dedup_key":"abc123","repro_sha256":"deadbeef","harness_id":"001"}`
	api := newFakeMessages(t,
		fakeMsg{stop: "tool_use", blocks: []fakeBlock{{toolID: "t1", toolName: "lich_read_crash", toolInput: `{"results_dir":"nope","crash_id":"x"}`}}},
		fakeMsg{stop: "tool_use", blocks: []fakeBlock{{toolID: "t2", toolName: "lich_file_finding", toolInput: findingInput}}},
		fakeMsg{stop: "end_turn", blocks: []fakeBlock{{text: "done; the finding needs operator confirmation."}}},
	)

	aud := &recordingAuditor{}
	b, ledger := buildTestBridge(t, api.URL, store, provider, aud)
	conv := b.NewConversation()
	reply, err := conv.Send(context.Background(), "triage this crash and file a finding")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if reply.Requests < 3 {
		t.Fatalf("requests = %d, want >=3 (read, file, end)", reply.Requests)
	}

	// file_finding parked exactly one ticket; read_crash parked none.
	tickets, _ := store.List()
	if len(tickets) != 1 || tickets[0].Tool != lichtools.ToolFileFinding {
		t.Fatalf("tickets = %+v, want one file_finding ticket", tickets)
	}
	// The parked finding carries the dedup key + repro hash (a complete artifact).
	var args map[string]any
	_ = json.Unmarshal(tickets[0].Args, &args)
	if args["dedup_key"] != "abc123" || args["repro_sha256"] != "deadbeef" {
		t.Fatalf("parked finding is missing dedup/repro: %v", args)
	}

	// Spend was reserved+committed per request (fake usage bills a nonzero cost).
	if spent, _ := ledger.Spent(); spent <= 0 {
		t.Fatalf("ledger spent = %v, want > 0 (reserve-before-send + commit)", spent)
	}

	// No prompt/response TEXT is in the audit — only sizes and a hash.
	for _, r := range aud.tools {
		if r.InputSHA256 == "" {
			t.Fatal("tool record missing input hash")
		}
		if strings.Contains(r.Reason, "parser OOB read") {
			t.Fatal("audit record leaked finding text")
		}
	}
}

// Step 165: a gate refusal is terminal without approval — re-issuing the same
// refused finding in a later turn parks no second ticket and never runs it.
func TestDrive_RefusalIsTerminalWithoutApproval(t *testing.T) {
	store, _ := lichtools.NewPendingStore(t.TempDir())
	cfg := &lichtools.Config{ProjectRoot: t.TempDir()}
	cfg.Scope.SandboxPrefix = lichtools.DefaultSandboxPrefix
	cfg.Scope.HarnessDir = t.TempDir()
	provider, _ := lichtools.NewProvider(cfg)

	ff := `{"severity":"high","summary":"x","dedup_key":"k","repro_sha256":"h"}`
	api := newFakeMessages(t,
		fakeMsg{stop: "tool_use", blocks: []fakeBlock{{toolID: "a", toolName: "lich_file_finding", toolInput: ff}}},
		fakeMsg{stop: "end_turn", blocks: []fakeBlock{{text: "needs confirmation"}}},
		// Second turn re-issues the identical refused finding.
		fakeMsg{stop: "tool_use", blocks: []fakeBlock{{toolID: "b", toolName: "lich_file_finding", toolInput: ff}}},
		fakeMsg{stop: "end_turn", blocks: []fakeBlock{{text: "still needs confirmation"}}},
	)
	b, _ := buildTestBridge(t, api.URL, store, provider, &recordingAuditor{})
	conv := b.NewConversation()
	if _, err := conv.Send(context.Background(), "file a finding"); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := conv.Send(context.Background(), "please file it now"); err != nil {
		t.Fatalf("turn 2: %v", err)
	}
	// Still exactly one ticket (dedup), still pending — the refusal never became
	// a write just because the model asked twice.
	tickets, _ := store.List()
	if len(tickets) != 1 {
		t.Fatalf("tickets = %d, want 1 (re-ask must not spawn or run a write)", len(tickets))
	}
	if tickets[0].Status != lichtools.StatusPending {
		t.Fatalf("ticket status = %q, want still pending", tickets[0].Status)
	}
}

// Step 153-156: the daily dollar cap reserves worst-case BEFORE any send, so a
// cap already (nearly) spent refuses the next request without hitting the API.
func TestDrive_CapRefusesBeforeSend(t *testing.T) {
	// An API that fails the test if it is ever called.
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the API was called despite the spend cap being exhausted")
	}))
	t.Cleanup(api.Close)

	store, _ := lichtools.NewPendingStore(t.TempDir())
	cfg := &lichtools.Config{ProjectRoot: t.TempDir()}
	cfg.Scope.SandboxPrefix = lichtools.DefaultSandboxPrefix
	cfg.Scope.HarnessDir = t.TempDir()
	provider, _ := lichtools.NewProvider(cfg)

	champ, _ := champion.New(champion.Config{ProjectRoot: t.TempDir(), SessionID: "cap"}, nil)
	cg, _ := claudebridge.NewChampionGate(champ)
	gate := newConfirmGate(cg, store, t.TempDir(), nil)
	ledger, _ := claudebridge.NewLedger("", 1) // 1 nano-USD cap: any request's reservation exceeds it
	prices, _ := claudebridge.DefaultPrices()
	b, err := claudebridge.New(context.Background(), claudebridge.Config{
		Model: "claude-opus-5-5", Effort: "medium",
		Providers:     []claudebridge.NamedProvider{{Name: "lich", Provider: provider}},
		Gate:          gate,
		Prices:        prices,
		Ledger:        ledger,
		Credentials:   func() (string, error) { return "test", nil },
		ClientOptions: []option.RequestOption{option.WithBaseURL(api.URL), option.WithAPIKey("k")},
	})
	if err != nil {
		t.Fatalf("bridge: %v", err)
	}
	_, err = b.NewConversation().Send(context.Background(), "do work")
	if err == nil {
		t.Fatal("Send should be refused by the spend cap before any request")
	}
}

func TestDrive_DryRunNeedsNoCredential(t *testing.T) {
	var out, errb strings.Builder
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	code, err := drive(context.Background(), driveOpts{
		cfgPath: "../../configs/lich-mcp.yaml",
		root:    projectRootForDriveTest(t),
		route:   "minor",
		prompt:  "generate seeds",
		dryRun:  true,
		haiku:   "claude-haiku-5-5",
		opus:    "claude-opus-5-5",
		capUSD:  3,
	}, &out, logger)
	_ = errb
	if err != nil || code != 0 {
		t.Fatalf("dry-run drive = (%d, %v)", code, err)
	}
	if !strings.Contains(out.String(), "claude-haiku-5-5") {
		t.Fatalf("minor route should pick Haiku; got: %s", out.String())
	}
}

// projectRootForDriveTest returns the absolute repo root so LoadConfig resolves
// its harness/results/register paths to absolute paths.
func projectRootForDriveTest(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}
	return abs
}
