// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// Local model defaults (llama-server, OpenAI-compatible API).
const (
	DefaultLocalURL       = "http://127.0.0.1:8081"
	DefaultLocalModel     = "local" // llama-server answers with whatever it loaded
	DefaultLocalMaxTokens = 1024
	DefaultLocalTimeout   = 120 * time.Second
	maxLocalBodyBytes     = 1 << 20
)

// DefaultLocalSystem is the system prompt for the local leg of -route
// auto. The local model gets no tools.
const DefaultLocalSystem = `You are Zhen, the local assistant for Stevie Bellis on Unheaded, a GPL-3.0 configuration management automation platform.
You have no tools and cannot read files. Answer plainly and briefly. If you are not sure, say so instead of guessing.`

// ErrLocalTruncated: the local answer hit max_tokens (finish_reason
// "length"), so it is incomplete.
var ErrLocalTruncated = errors.New("local answer cut off at max_tokens")

// LocalLLM is a client for llama-server's /v1/chat/completions. It asks;
// it never starts, stops or swaps a model.
type LocalLLM struct {
	BaseURL     string // default DefaultLocalURL
	Model       string // default DefaultLocalModel
	MaxTokens   int    // default DefaultLocalMaxTokens
	Temperature float64
	Timeout     time.Duration // per call; default DefaultLocalTimeout
	HTTP        *http.Client  // default: a client with no extra settings
}

// LocalAnswer is one local completion.
type LocalAnswer struct {
	Model        string // as reported by the server, else the requested one
	Text         string
	FinishReason string
}

type localMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RequestJSON returns the request body Complete would send.
func (l *LocalLLM) RequestJSON(system, prompt string) ([]byte, error) {
	msgs := []localMessage{}
	if system != "" {
		msgs = append(msgs, localMessage{Role: "system", Content: system})
	}
	msgs = append(msgs, localMessage{Role: "user", Content: prompt})
	maxTokens := l.MaxTokens
	if maxTokens == 0 {
		maxTokens = DefaultLocalMaxTokens
	}
	model := l.Model
	if model == "" {
		model = DefaultLocalModel
	}
	return json.Marshal(map[string]any{
		"model":       model,
		"messages":    msgs,
		"max_tokens":  maxTokens,
		"temperature": l.Temperature,
		"stream":      false,
	})
}

// Endpoint is the chat-completions URL, or an error for a base URL that
// is not http(s).
func (l *LocalLLM) Endpoint() (string, error) {
	base := l.BaseURL
	if base == "" {
		base = DefaultLocalURL
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("local LLM URL %q is not an http(s) URL", base)
	}
	return strings.TrimRight(base, "/") + "/v1/chat/completions", nil
}

// Complete asks the local model once. A down server, a non-200 status, a
// malformed or empty answer, or finish_reason "length" (ErrLocalTruncated,
// returned with the partial answer) is an error.
func (l *LocalLLM) Complete(ctx context.Context, system, prompt string) (LocalAnswer, error) {
	endpoint, err := l.Endpoint()
	if err != nil {
		return LocalAnswer{}, err
	}
	body, err := l.RequestJSON(system, prompt)
	if err != nil {
		return LocalAnswer{}, err
	}
	timeout := l.Timeout
	if timeout <= 0 {
		timeout = DefaultLocalTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return LocalAnswer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := l.HTTP
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(req) // #nosec G704 -- operator-configured local endpoint, scheme checked in Endpoint
	if err != nil {
		return LocalAnswer{}, fmt.Errorf("local LLM unavailable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxLocalBodyBytes+1))
	if err != nil {
		return LocalAnswer{}, fmt.Errorf("local LLM: read: %w", err)
	}
	if len(raw) > maxLocalBodyBytes {
		return LocalAnswer{}, fmt.Errorf("local LLM: response exceeds %d bytes", maxLocalBodyBytes)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := raw
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return LocalAnswer{}, fmt.Errorf("local LLM: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	var cr struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string       `json:"finish_reason"`
			Message      localMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &cr); err != nil || len(cr.Choices) == 0 {
		return LocalAnswer{}, errors.New("local LLM: response is not an OpenAI chat completion")
	}
	ans := LocalAnswer{Model: cr.Model, Text: cr.Choices[0].Message.Content, FinishReason: cr.Choices[0].FinishReason}
	if ans.Model == "" {
		ans.Model = l.Model
	}
	if strings.TrimSpace(ans.Text) == "" {
		return ans, errors.New("local LLM: empty answer")
	}
	if ans.FinishReason == "length" {
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
