// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

// Command zhen-rag is a RAG (retrieval-augmented generation) glue binary
// connecting `vor` (cs cheatsheet REST API at github.com/bellistech/vor)
// to a local llama.cpp `llama-server` running an OpenAI-compatible
// /v1/chat/completions endpoint.
//
// Flow:
//
//  1. User question on stdin or via -q flag.
//  2. GET <vor>/api/search?q=<q>  → list of (topic, category, section, line)
//  3. For top-K unique topics, GET <vor>/api/topics/<topic> for full content.
//  4. Format prompt: system + retrieved references + user question.
//  5. POST <llama>/v1/chat/completions → answer.
//  6. Print answer to stdout.
//
// stdlib-only — matches the discipline of the cs/vor project we're
// integrating with. No new deps in unheaded for this binary.
//
// Usage:
//
//	zhen-rag -q "What does WAVE14 fix?"
//	echo "How does the Marshal work?" | zhen-rag
//	zhen-rag -k 3 -max-tokens 500 -q "Explain ADR-051"
//
// Defaults assume vor at 127.0.0.1:9876 and llama-server at 127.0.0.1:8081.
// Override via env or flags: VOR_URL, LLAMA_URL, RAG_MODEL.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultVorURL         = "http://127.0.0.1:9876"
	defaultLlamaURL       = "http://127.0.0.1:8081"
	defaultModel          = "qwen2.5-coder-7b-instruct"
	defaultK              = 5
	defaultMaxTokens      = 800
	defaultMaxTopicChars  = 10000
	httpTimeout           = 5 * time.Minute
)

// vorSearchHit matches one row of vor's GET /api/search response.
// Source: cmd/vor/main.go ~line 1130 — `[]struct{topic,category,section,line}`.
type vorSearchHit struct {
	Topic    string `json:"topic"`
	Category string `json:"category"`
	Section  string `json:"section"`
	Line     string `json:"line"`
}

// vorTopic matches GET /api/topics/<name> response shape.
// Source: cmd/vor/main.go ~line 1102 — fields name/category/title/description/content/see_also/has_detail.
type vorTopic struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	SeeAlso     []string `json:"see_also"`
	HasDetail   bool     `json:"has_detail"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Seed        int           `json:"seed,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatChoice struct {
	Index   int         `json:"index"`
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

func main() {
	var (
		question  = flag.String("q", "", "question (otherwise reads from stdin)")
		k         = flag.Int("k", defaultK, "number of top-K topics to retrieve full content for")
		maxTokens = flag.Int("max-tokens", defaultMaxTokens, "llama-server max_tokens for the response")
		temp      = flag.Float64("temperature", 0.4, "sampling temperature (0.0 = greedy)")
		showCtx   = flag.Bool("show-context", false, "print retrieved references before the answer")
		maxChars  = flag.Int("max-topic-chars", defaultMaxTopicChars, "per-topic content cap; oversized topics are truncated to fit the context window")
		seed      = flag.Int("seed", 0, "llama-server seed; nonzero pins sampling for reproducibility (combine with -temperature 0 for greedy determinism)")
	)
	flag.Parse()

	vorURL := envOr("VOR_URL", defaultVorURL)
	llamaURL := envOr("LLAMA_URL", defaultLlamaURL)
	model := envOr("RAG_MODEL", defaultModel)

	q := strings.TrimSpace(*question)
	if q == "" {
		// Read from stdin.
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal("read stdin: %v", err)
		}
		q = strings.TrimSpace(string(buf))
	}
	if q == "" {
		fatal("no question (use -q or pipe via stdin)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	client := &http.Client{Timeout: httpTimeout}

	// 1. retrieve search hits — vor's search returns null when the
	// query ends in punctuation (`?`, `!`, `.`), so we feed it a
	// stripped variant. The full question still goes to the LLM.
	hits, err := vorSearch(ctx, client, vorURL, searchQuery(q))
	if err != nil {
		fatal("vor search: %v", err)
	}

	// 2. fetch full content for top-K unique topics (preserve search order)
	seen := make(map[string]struct{})
	var topics []vorTopic
	for _, h := range hits {
		if _, ok := seen[h.Topic]; ok {
			continue
		}
		seen[h.Topic] = struct{}{}
		t, err := vorTopicFetch(ctx, client, vorURL, h.Topic)
		if err != nil {
			fmt.Fprintf(os.Stderr, "zhen-rag: skip topic %q: %v\n", h.Topic, err)
			continue
		}
		topics = append(topics, *t)
		if len(topics) >= *k {
			break
		}
	}

	if *showCtx {
		fmt.Fprintln(os.Stderr, "─── retrieved references ───")
		for _, t := range topics {
			fmt.Fprintf(os.Stderr, "  • %s/%s — %s\n", t.Category, t.Name, t.Title)
		}
		fmt.Fprintln(os.Stderr, "────────────────────────────")
	}

	// 3. format prompt
	//
	// System prompt design (post Phase C, 2026-05-01):
	// Phase C verdict (eval/coding-gate/results-2026-05-01.md) found that
	// the prior, stricter system prompt over-restricted general-knowledge
	// answers — 3/7 syntax prompts got "the references do not contain the
	// answer" refusals on textbook questions (Go error check, CSS center
	// a div, JS async fetch). The split below distinguishes
	// Unheaded-specific facts (must be grounded in retrieval) from
	// general programming knowledge (model uses training directly).
	systemPrompt := "You are zhen, an assistant for the Unheaded Kingdom.\n\n" +
		"For Unheaded-specific facts — services, runbooks, ADRs, sessions, " +
		"internal naming, architectural decisions, training/eval results — " +
		"the reference docs below are the authoritative source. NEVER invent " +
		"Unheaded specifics. If the refs don't contain the fact, say so " +
		"plainly. When you quote Unheaded specifics, cite the source " +
		"(e.g., 'per docs/adr/ADR-051').\n\n" +
		"For general programming questions — language syntax, idioms, " +
		"well-known patterns and pitfalls, code review on pasted snippets — " +
		"answer directly from your training. You do not need a reference to " +
		"know that `==` should be `===` in JavaScript, or that `.unwrap()` " +
		"panics in Rust, or that an `<img>` needs an `alt=` attribute. The " +
		"refs may add Unheaded house-style hints when relevant.\n\n" +
		"For code-review prompts: identify bugs and bad practices in the " +
		"snippet even when no reference mentions them. Saying 'no issue' on " +
		"code that has a well-known bug is the worst failure mode.\n\n" +
		"Be concise and direct."

	var refs strings.Builder
	for _, t := range topics {
		refs.WriteString("\n\n--- ")
		refs.WriteString(t.Category)
		refs.WriteString("/")
		refs.WriteString(t.Name)
		refs.WriteString(" ---\n")
		refs.WriteString(clipContent(t.Content, *maxChars))
	}

	userPrompt := fmt.Sprintf(
		"References:%s\n\nQuestion: %s",
		refs.String(), q,
	)

	// 4. call llama-server
	answer, err := llamaChat(ctx, client, llamaURL, chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   *maxTokens,
		Temperature: *temp,
		Seed:        *seed,
		Stream:      false,
	})
	if err != nil {
		fatal("llama chat: %v", err)
	}
	fmt.Println(answer)
}

// vorSearch queries GET /api/search?q=<q>.
func vorSearch(ctx context.Context, c *http.Client, base, q string) ([]vorSearchHit, error) {
	endpoint := fmt.Sprintf("%s/api/search?q=%s", strings.TrimRight(base, "/"), url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	var hits []vorSearchHit
	if err := json.NewDecoder(resp.Body).Decode(&hits); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return hits, nil
}

// vorTopicFetch queries GET /api/topics/<name>.
func vorTopicFetch(ctx context.Context, c *http.Client, base, name string) (*vorTopic, error) {
	endpoint := fmt.Sprintf("%s/api/topics/%s", strings.TrimRight(base, "/"), url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	var t vorTopic
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &t, nil
}

// llamaChat POSTs /v1/chat/completions and returns the assistant's reply.
func llamaChat(ctx context.Context, c *http.Client, base string, req chatRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(base, "/") + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("empty response")
	}
	return cr.Choices[0].Message.Content, nil
}

// clipContent truncates topic content to maxChars to fit the context
// window. Cs/vor cheatsheets can exceed 70 KB on a single sheet (e.g.
// the embedded `bash` topic) — left unchecked, a single retrieval can
// blow the llama-server prompt budget. The truncation marker is plain
// text so the model can recognize the cut.
func clipContent(s string, maxChars int) string {
	if maxChars <= 0 || len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "\n\n…[truncated]"
}

// searchQuery returns the question stripped of trailing sentence
// punctuation. vor's full-text search returns null for any query that
// ends in `?`, `!`, or `.`, so we strip them before search. The full
// question still goes to the LLM as-is.
func searchQuery(q string) string {
	return strings.TrimRight(q, "?!.")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "zhen-rag: "+format+"\n", args...)
	os.Exit(1)
}
