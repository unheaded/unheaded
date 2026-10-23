// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"unheaded/pkg/localllm"
)

// Local model defaults (llama-server, OpenAI-compatible API).
const (
	DefaultLocalURL       = localllm.DefaultURL
	DefaultLocalModel     = localllm.DefaultModel
	DefaultLocalMaxTokens = localllm.DefaultMaxTokens
	DefaultLocalTimeout   = localllm.DefaultTimeout
)

// DefaultLocalSystem is the system prompt for the local leg of -route
// auto. The local model gets no tools.
const DefaultLocalSystem = `You are Zhen, the local assistant for Stevie Bellis on Unheaded, a GPL-3.0 configuration management automation platform.
You have no tools and cannot read files. Answer plainly and briefly. If you are not sure, say so instead of guessing.`

// ErrLocalTruncated: the local answer hit max_tokens (finish_reason
// "length"), so it is incomplete.
var ErrLocalTruncated = errors.New("local answer cut off at max_tokens")

// LocalLLM asks llama-server one question through pkg/localllm. It asks;
// it never starts, stops or swaps a model.
type LocalLLM struct {
	BaseURL     string // default DefaultLocalURL
	Model       string // default DefaultLocalModel
	MaxTokens   int    // default DefaultLocalMaxTokens
	Temperature float64
	Seed        *int          // nil: not sent
	Timeout     time.Duration // per call; default DefaultLocalTimeout
	HTTP        *http.Client  // default: a client with no extra settings
	// GPULock / LeaseTimeout: each call holds a shared GPU lease
	// (localllm.Client.GPULock); "" = no lease.
	GPULock      string
	LeaseTimeout time.Duration
}

// LocalAnswer is one local completion.
type LocalAnswer struct {
	Model        string // as reported by the server, else the requested one
	Text         string
	FinishReason string
	InputTokens  int64
	OutputTokens int64
}

func (l *LocalLLM) client() *localllm.Client {
	return &localllm.Client{BaseURL: l.BaseURL, Model: l.Model, MaxTokens: l.MaxTokens, Timeout: l.Timeout, HTTP: l.HTTP,
		GPULock: l.GPULock, LeaseTimeout: l.LeaseTimeout}
}

func (l *LocalLLM) request(system, prompt string) localllm.Request {
	var msgs []localllm.Message
	if system != "" {
		msgs = append(msgs, localllm.Message{Role: "system", Content: system})
	}
	msgs = append(msgs, localllm.Message{Role: "user", Content: prompt})
	return localllm.Request{Messages: msgs, Temperature: l.Temperature, Seed: l.Seed}
}

// RequestJSON returns the request body Complete would send.
func (l *LocalLLM) RequestJSON(system, prompt string) ([]byte, error) {
	return l.client().Body(l.request(system, prompt))
}

// Endpoint is the chat-completions URL, or an error for a base URL that
// is not http(s).
func (l *LocalLLM) Endpoint() (string, error) { return l.client().Endpoint() }

// Complete asks the local model once. A down server, a non-200 status, a
// malformed or empty answer, or finish_reason "length" (ErrLocalTruncated,
// returned with the partial answer) is an error.
func (l *LocalLLM) Complete(ctx context.Context, system, prompt string) (LocalAnswer, error) {
	a, err := l.client().Chat(ctx, l.request(system, prompt))
	if err != nil {
		return LocalAnswer{}, err
	}
	ans := LocalAnswer{Model: a.Model, Text: a.Text, FinishReason: a.FinishReason, InputTokens: a.PromptTokens, OutputTokens: a.CompletionTokens}
	if strings.TrimSpace(ans.Text) == "" {
		return ans, errors.New("local LLM: empty answer")
	}
	if a.Truncated() {
		return ans, ErrLocalTruncated
	}
	return ans, nil
}

// CheckResult is the outcome of an acceptance check.
type CheckResult struct {
	Argv     []string
	Exit     int // -1 when the command did not run to an exit status
	Accepted bool
	Err      string
	Duration time.Duration
}

// RunCheck runs argv (no shell) in dir with answer on stdin and accepts
// the answer when it exits 0. Its output is discarded. It is killed at
// timeout.
func RunCheck(ctx context.Context, argv []string, dir, answer string, timeout time.Duration) CheckResult {
	res := CheckResult{Argv: argv, Exit: -1}
	if len(argv) == 0 {
		res.Err = "empty check command"
		return res
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t0 := time.Now()
	// #nosec G204 -- argv is the operator's own -check flag, run without a shell.
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(answer)
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	res.Duration = time.Since(t0)
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		res.Exit, res.Accepted = 0, true
	case ctx.Err() != nil:
		res.Err = fmt.Sprintf("check timed out after %s", timeout)
	case errors.As(err, &exitErr):
		res.Exit = exitErr.ExitCode()
		if res.Exit < 0 {
			res.Err = err.Error()
		}
	default:
		res.Err = err.Error()
	}
	return res
}

// LocalCompleter adapts LocalLLM to privacy.Completer (local summaries).
type LocalCompleter struct{ L *LocalLLM }

// Complete asks the local model once, at temperature 0.
func (c LocalCompleter) Complete(ctx context.Context, system, prompt string) (string, error) {
	if c.L == nil {
		return "", errors.New("claudebridge: no local model")
	}
	l := *c.L
	l.Temperature = 0
	ans, err := l.Complete(ctx, system, prompt)
	if err != nil {
		return "", err
	}
	return ans.Text, nil
}
