// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package localllm is the one client for the local model: llama-server's
// OpenAI-compatible /v1/chat/completions. It asks; it never starts,
// stops or swaps a model (that is scripts/switch-model.sh, with its
// memory fit check). zhen-claude's -route auto, zhen-agent, zhen-agentd
// and zhen-eval all use it (S-TWO-BRAINS Step 135).
package localllm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Defaults.
const (
	DefaultURL       = "http://127.0.0.1:8081"
	DefaultModel     = "local" // llama-server answers with whatever it loaded
	DefaultMaxTokens = 1024
	DefaultTimeout   = 120 * time.Second
	MaxBodyBytes     = 4 << 20
)

// Message is one chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request is one completion.
type Request struct {
	Messages    []Message
	MaxTokens   int     // default Client.MaxTokens, then DefaultMaxTokens
	Temperature float64 // 0 is greedy
	Seed        *int    // nil: not sent
}

// Answer is one completion with what the server reported about it.
type Answer struct {
	Model            string // as reported by the server, else the requested one
	Text             string
	FinishReason     string
	PromptTokens     int64
	CompletionTokens int64
}

// Truncated reports finish_reason "length": the answer hit max_tokens.
func (a Answer) Truncated() bool { return a.FinishReason == "length" }

// Client talks to one llama-server.
type Client struct {
	BaseURL   string        // default DefaultURL
	Model     string        // default DefaultModel
	MaxTokens int           // default DefaultMaxTokens
	Timeout   time.Duration // per call; default DefaultTimeout
	HTTP      *http.Client  // default: a plain client
}

// Endpoint is the chat-completions URL, or an error for a base URL that
// is not http(s).
func (c *Client) Endpoint() (string, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultURL
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("local LLM URL %q is not an http(s) URL", base)
	}
	return strings.TrimRight(base, "/") + "/v1/chat/completions", nil
}

func (c *Client) model() string {
	if c.Model == "" {
		return DefaultModel
	}
	return c.Model
}

// Body returns the JSON body Chat would send.
func (c *Client) Body(r Request) ([]byte, error) {
	maxTokens := r.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.MaxTokens
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	msgs := r.Messages
	if msgs == nil {
		msgs = []Message{}
	}
	body := map[string]any{
		"model":       c.model(),
		"messages":    msgs,
		"max_tokens":  maxTokens,
		"temperature": r.Temperature,
		"stream":      false,
	}
	if r.Seed != nil {
		body["seed"] = *r.Seed
	}
	return json.Marshal(body)
}

// Chat asks once. The error is only for what makes the answer unusable
// as a reply at all: a down server, a non-200 status, a malformed body,
// no choices. An empty or truncated answer is returned as is; callers
// decide what that means (Answer.Truncated).
func (c *Client) Chat(ctx context.Context, r Request) (Answer, error) {
	endpoint, err := c.Endpoint()
	if err != nil {
		return Answer{}, err
	}
	body, err := c.Body(r)
	if err != nil {
		return Answer{}, err
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	resp, err := hc.Do(req) // #nosec G704 -- operator-configured local endpoint, scheme checked in Endpoint
	if err != nil {
		return Answer{}, fmt.Errorf("local LLM unavailable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes+1))
	if err != nil {
		return Answer{}, fmt.Errorf("local LLM: read: %w", err)
	}
	if len(raw) > MaxBodyBytes {
		return Answer{}, fmt.Errorf("local LLM: response exceeds %d bytes", MaxBodyBytes)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := raw
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return Answer{}, fmt.Errorf("local LLM: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	var cr struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string  `json:"finish_reason"`
			Message      Message `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &cr); err != nil || len(cr.Choices) == 0 {
		return Answer{}, errors.New("local LLM: response is not an OpenAI chat completion")
	}
	a := Answer{
		Model: cr.Model, Text: cr.Choices[0].Message.Content, FinishReason: cr.Choices[0].FinishReason,
		PromptTokens: max(cr.Usage.PromptTokens, 0), CompletionTokens: max(cr.Usage.CompletionTokens, 0),
	}
	if a.Model == "" {
		a.Model = c.model()
	}
	return a, nil
}
