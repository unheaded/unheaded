// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/claudebridge/privacy"
	"unheaded/pkg/localllm"
	"unheaded/pkg/sandbox"
)

// localBrain asks llama-server, greedy (temperature 0, fixed seed).
type localBrain struct {
	c    *localllm.Client
	seed int
	log  *slog.Logger
}

func (b *localBrain) Name() string  { return "local" }
func (b *localBrain) Model() string { return b.c.Model }

func (b *localBrain) Ask(ctx context.Context, system, prompt string, maxTokens int) (Answer, error) {
	seed := b.seed
	t0 := time.Now()
	a, err := b.c.Chat(ctx, localllm.Request{
		Messages:  []localllm.Message{{Role: "system", Content: system}, {Role: "user", Content: prompt}},
		MaxTokens: maxTokens, Temperature: 0, Seed: &seed,
	})
	ans := Answer{Text: a.Text, ServedBy: a.Model, InputTokens: a.PromptTokens, OutputTokens: a.CompletionTokens, Duration: time.Since(t0)}
	if err != nil {
		return ans, err
	}
	if a.Text == "" {
		return ans, errors.New("empty answer")
	}
	return ans, nil
}

// claudeBrain asks Claude through the bridge: no tools, the privacy tier
// on the prompt, a hard daily cap. One bridge per system prompt so the
// prompt cache holds.
type claudeBrain struct {
	model   string
	bridges map[string]*claudebridge.Bridge
}

func (b *claudeBrain) Name() string  { return "claude" }
func (b *claudeBrain) Model() string { return b.model }

func newClaudeBrain(ctx context.Context, o *options, log *slog.Logger) (*claudeBrain, error) {
	prices, err := claudebridge.DefaultPrices()
	if err != nil {
		return nil, err
	}
	capNano, err := claudebridge.USDToNano(o.dailyCapUSD)
	if err != nil {
		return nil, fmt.Errorf("-daily-cap-usd: %w", err)
	}
	path := o.ledger
	if path == "" {
		if path, err = claudebridge.DefaultLedgerPath(); err != nil {
			return nil, err
		}
	}
	ledger, err := claudebridge.NewLedger(path, capNano)
	if err != nil {
		return nil, err
	}
	red, err := privacy.NewRedactor(privacy.RedactorConfig{Scanner: os.Getenv("ZHEN_SECRET_SCANNER")})
	if err != nil {
		return nil, fmt.Errorf("%w (nothing is sent to Claude without the secret scan)", err)
	}
	sandboxRoot, err := os.MkdirTemp("", "zhen-eval-champion-")
	if err != nil {
		return nil, err
	}
	champ, err := champion.New(champion.Config{ProjectRoot: sandboxRoot}, nil)
	if err != nil {
		return nil, err
	}
	gate, err := claudebridge.NewChampionGate(champ)
	if err != nil {
		return nil, err
	}
	b := &claudeBrain{model: o.model, bridges: map[string]*claudebridge.Bridge{}}
	for system, maxTok := range map[string]int64{systemKey: o.claudeKeyTok, systemRepo: o.claudeRepoTok} {
		br, err := claudebridge.New(ctx, claudebridge.Config{
			Model: o.model, Effort: o.effort, MaxTokens: maxTok, DisableFallbacks: true, MaxIterations: 2,
			System: system, Gate: gate, Prices: prices, Ledger: ledger,
			Auditor: claudebridge.SlogAuditor{Log: log},
			Privacy: &privacy.Filter{Redactor: red, Mode: privacy.ModeStrict},
			Warn:    func(m string) { log.Warn("privacy", "note", m) },
		})
		if err != nil {
			return nil, err
		}
		b.bridges[system] = br
	}
	spent, _ := ledger.Spent()
	log.Info("claude_ready", "model", o.model, "effort", o.effort, "day_spent_usd", spent.USD(), "daily_cap_usd", ledger.Cap().USD(), "ledger", path)
	return b, nil
}

func (b *claudeBrain) Ask(ctx context.Context, system, prompt string, _ int) (Answer, error) {
	br, ok := b.bridges[system]
	if !ok {
		return Answer{}, errors.New("no bridge for this system prompt")
	}
	conv := br.NewConversation()
	conv.SetRoute("", "manual")
	t0 := time.Now()
	reply, err := conv.Send(ctx, prompt)
	ans := Answer{Duration: time.Since(t0)}
	if reply != nil {
		ans.Text, ans.ServedBy, ans.Cost = reply.Text, reply.ServedBy, reply.Cost
		ans.InputTokens, ans.OutputTokens = reply.InputTokens, reply.OutputTokens
	}
	return ans, err
}

// sandboxRunner runs check.sh in the no-network user sandbox with a
// shared, writable Go build cache.
type sandboxRunner struct {
	cache string
}

func newSandboxRunner(work string) *sandboxRunner {
	cache := filepath.Join(work, "gocache")
	_ = os.MkdirAll(cache, 0o700)
	return &sandboxRunner{cache: cache}
}

func (s *sandboxRunner) Run(ctx context.Context, dir string) (int, string, error) {
	res, err := sandbox.Run(ctx, sandbox.Config{
		Dir:      dir,
		Writable: []string{s.cache},
		Env: []string{
			"PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin",
			"HOME=" + dir, "TMPDIR=/tmp", "LANG=C.UTF-8",
			"GOCACHE=" + s.cache, "GOPATH=" + filepath.Join(dir, ".gopath"),
			"GOFLAGS=-mod=mod", "GOPROXY=off", "GOTOOLCHAIN=local", "GOWORK=off",
		},
	}, []string{"/bin/bash", "check.sh"})
	return res.Exit, res.Output, err
}
