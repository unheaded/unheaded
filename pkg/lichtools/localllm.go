// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"unheaded/pkg/localllm"
	"unheaded/pkg/zhentools"
)

// ToolLocalLLM is the free, local-model tool for MINOR-weight work (seed and
// mutation drafting, harness-variant text). It asks the already-running
// llama-server one question. It never loads, swaps, or starts a model (that is
// scripts/switch-model.sh's fit-checked job), never leaves the host, and never
// incurs a dollar cost — but every call counts one tick against the daily
// compute budget so a runaway driver still pauses. Read-only to the Champion
// gate. S-LICH-AWAKENS Phase 4, Steps 115-118.
const ToolLocalLLM = "lich_local_llm"

// DefaultMaxDailyLocalGens caps local-model generations per UTC day.
const DefaultMaxDailyLocalGens = 5000

// ErrComputeBudget is returned when the daily local-generation budget is spent.
var ErrComputeBudget = errors.New("lichtools: daily local-generation budget exhausted; the loop pauses until the next UTC day")

// localClient is the Provider's handle on the local llama-server.
type localClient struct {
	c     *localllm.Client
	model string
}

// WithLocalLLM wires the already-running llama-server endpoint for the
// lich_local_llm tool and the local-model driver. baseURL must be an http(s)
// URL (checked on first use). genMax <= 0 uses DefaultMaxDailyLocalGens.
func (p *Provider) WithLocalLLM(baseURL, model string, genMax int) *Provider {
	if model == "" {
		model = localllm.DefaultModel
	}
	p.local = &localClient{
		c:     &localllm.Client{BaseURL: baseURL, Model: model},
		model: model,
	}
	if genMax <= 0 {
		genMax = DefaultMaxDailyLocalGens
	}
	p.genMu.Lock()
	p.genMax = genMax
	p.genMu.Unlock()
	return p
}

// HasLocalLLM reports whether the local-model tool is available.
func (p *Provider) HasLocalLLM() bool { return p.local != nil }

type localLLMInput struct {
	Prompt      string  `json:"prompt"`
	System      string  `json:"system"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
}

var schemaLocalLLM = json.RawMessage(`{"type":"object","properties":{` +
	`"prompt":{"type":"string","description":"the question or generation request for the local model"},` +
	`"system":{"type":"string","description":"optional system prompt"},` +
	`"max_tokens":{"type":"integer","minimum":1,"description":"optional output cap"},` +
	`"temperature":{"type":"number","minimum":0,"description":"optional sampling temperature (0 = greedy)"}},` +
	`"required":["prompt"],"additionalProperties":false}`)

// localLLM runs one local completion. It is free (no dollar reserve) but charges
// one tick against the daily compute budget, so a driver cannot spin forever.
func (p *Provider) localLLM(ctx context.Context, in localLLMInput) (zhentools.Result, error) {
	if p.local == nil {
		return zhentools.Result{IsError: true, Text: ToolLocalLLM + ": no local model endpoint is configured"}, nil
	}
	if in.Prompt == "" {
		return badInput(fmt.Errorf("prompt is required"))
	}
	if err := p.reserveLocalGen(); err != nil {
		return zhentools.Result{IsError: true, Text: err.Error()}, nil
	}
	ans, err := p.local.complete(ctx, in.System, in.Prompt, in.MaxTokens, in.Temperature)
	if err != nil {
		return zhentools.Result{IsError: true, Text: ToolLocalLLM + ": " + err.Error()}, nil
	}
	return zhentools.Result{Text: ans}, nil
}

func (l *localClient) complete(ctx context.Context, system, prompt string, maxTokens int, temp float64) (string, error) {
	var msgs []localllm.Message
	if system != "" {
		msgs = append(msgs, localllm.Message{Role: "system", Content: system})
	}
	msgs = append(msgs, localllm.Message{Role: "user", Content: prompt})
	ans, err := l.c.Chat(ctx, localllm.Request{Messages: msgs, MaxTokens: maxTokens, Temperature: temp})
	if err != nil {
		return "", err
	}
	return ans.Text, nil
}

// reserveLocalGen charges one generation against the daily compute budget. It
// resets per UTC day, like the dollar cap.
func (p *Provider) reserveLocalGen() error {
	day := time.Now().UTC().Format("2006-01-02")
	p.genMu.Lock()
	defer p.genMu.Unlock()
	if p.genDay != day {
		p.genDay, p.genUsed = day, 0
	}
	limit := p.genMax
	if limit <= 0 {
		limit = DefaultMaxDailyLocalGens
	}
	if p.genUsed >= limit {
		return ErrComputeBudget
	}
	p.genUsed++
	return nil
}

// LocalGenBudget reports the day's used and total local-generation budget.
func (p *Provider) LocalGenBudget() (used, total int) {
	p.genMu.Lock()
	defer p.genMu.Unlock()
	total = p.genMax
	if total <= 0 {
		total = DefaultMaxDailyLocalGens
	}
	return p.genUsed, total
}
