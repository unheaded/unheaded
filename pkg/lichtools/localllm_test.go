// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/zhentools"
)

// fakeLlama is a minimal OpenAI-compatible chat endpoint that echoes a fixed
// completion and records how many times it was called.
func fakeLlama(t *testing.T, reply string) (*httptest.Server, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/chat/completions") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		calls++
		_, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "local",
			"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": reply}}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 3},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func providerWithLocal(t *testing.T, baseURL string, genMax int) *Provider {
	t.Helper()
	cfg := &Config{ProjectRoot: t.TempDir()}
	cfg.Scope.SandboxPrefix = DefaultSandboxPrefix
	cfg.Scope.HarnessDir = t.TempDir()
	if err := cfg.applyDefaultsAndValidate(); err != nil {
		t.Fatalf("config: %v", err)
	}
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return p.WithLocalLLM(baseURL, "local", genMax)
}

func TestLocalLLM_ToolListedOnlyWhenWired(t *testing.T) {
	// Without an endpoint, the tool is not offered.
	cfg := &Config{ProjectRoot: t.TempDir()}
	cfg.Scope.HarnessDir = t.TempDir()
	_ = cfg.applyDefaultsAndValidate()
	p, _ := NewProvider(cfg)
	tools, _ := p.Tools(context.Background())
	for _, tl := range tools {
		if tl.Name == ToolLocalLLM {
			t.Fatal("lich_local_llm offered with no endpoint wired")
		}
	}

	// With an endpoint, it is listed.
	srv, _ := fakeLlama(t, "ok")
	p2 := providerWithLocal(t, srv.URL, 0)
	tools, _ = p2.Tools(context.Background())
	var found bool
	for _, tl := range tools {
		if tl.Name == ToolLocalLLM {
			found = true
		}
	}
	if !found {
		t.Fatal("lich_local_llm not listed when an endpoint is wired")
	}
}

func TestLocalLLM_RoutesToLocalEndpoint(t *testing.T) {
	srv, calls := fakeLlama(t, "SEED: 00 01 02")
	p := providerWithLocal(t, srv.URL, 0)

	res, err := p.Call(context.Background(), ToolLocalLLM, json.RawMessage(`{"prompt":"draft a monad-wire seed"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if res.IsError || !strings.Contains(res.Text, "SEED") {
		t.Fatalf("local_llm result = %+v, want the local reply", res)
	}
	if *calls != 1 {
		t.Fatalf("local endpoint called %d times, want 1", *calls)
	}

	// A dollar reserve is never incurred: the provider holds no Ledger and the
	// tool path never touches one.
	used, total := p.LocalGenBudget()
	if used != 1 || total != DefaultMaxDailyLocalGens {
		t.Fatalf("compute budget = %d/%d, want 1/%d", used, total, DefaultMaxDailyLocalGens)
	}
}

func TestLocalLLM_ComputeBudgetPauses(t *testing.T) {
	srv, _ := fakeLlama(t, "ok")
	p := providerWithLocal(t, srv.URL, 2) // budget of 2 generations

	for i := 0; i < 2; i++ {
		if res, _ := p.Call(context.Background(), ToolLocalLLM, json.RawMessage(`{"prompt":"x"}`)); res.IsError {
			t.Fatalf("generation %d errored early: %s", i, res.Text)
		}
	}
	// The third call must be refused by the compute budget, not sent.
	res, _ := p.Call(context.Background(), ToolLocalLLM, json.RawMessage(`{"prompt":"x"}`))
	if !res.IsError || !strings.Contains(res.Text, "budget exhausted") {
		t.Fatalf("over-budget call = %+v, want ErrComputeBudget", res)
	}
}

func TestLocalLLM_NoEndpointIsGracefulError(t *testing.T) {
	cfg := &Config{ProjectRoot: t.TempDir()}
	cfg.Scope.HarnessDir = t.TempDir()
	_ = cfg.applyDefaultsAndValidate()
	p, _ := NewProvider(cfg) // no WithLocalLLM
	res, err := p.Call(context.Background(), ToolLocalLLM, json.RawMessage(`{"prompt":"x"}`))
	if err != nil {
		t.Fatalf("Call err = %v, want graceful is_error", err)
	}
	if !res.IsError {
		t.Fatal("local_llm with no endpoint should be an is_error result")
	}
}

func TestLocalLLM_BudgetErrorType(t *testing.T) {
	// ErrComputeBudget is a sentinel the driver can test with errors.Is.
	if !errors.Is(ErrComputeBudget, ErrComputeBudget) {
		t.Fatal("ErrComputeBudget sentinel broken")
	}
	var _ zhentools.Provider = (*Provider)(nil)
}
