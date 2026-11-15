// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

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
// Defaults assume vor at 127.0.0.1:20110 and llama-server at 127.0.0.1:8081.
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
	"os"
	"strings"
	"time"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/vorclient"
	"unheaded/pkg/vorquery"
	"unheaded/pkg/zhenrag"
)

const (
	defaultVorURL        = "http://127.0.0.1:20110"
	defaultLlamaURL      = "http://127.0.0.1:8081"
	defaultModel         = "qwen2.5-coder-7b-instruct"
	defaultK             = 5
	defaultMaxTokens     = 800
	defaultMaxTopicChars = 10000
	httpTimeout          = 5 * time.Minute
)

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

// codeCommentReminder follows the question in every user prompt, and
// codeCommentExample is the worked exchange placed between the system prompt
// and the real question. Both live in pkg/zhenrag, shared with the web UI
// (raft/scripts/zhen_prompt_generated.py, generated from the same source).
const codeCommentReminder = zhenrag.CodeCommentReminder

var codeCommentExample = func() []chatMessage {
	ex := zhenrag.CodeCommentExample()
	out := make([]chatMessage, len(ex))
	for i, m := range ex {
		out[i] = chatMessage{Role: m.Role, Content: m.Content}
	}
	return out
}()

func main() {
	var (
		question      = flag.String("q", "", "question (otherwise reads from stdin)")
		k             = flag.Int("k", defaultK, "number of top-K topics to retrieve full content for")
		maxTokens     = flag.Int("max-tokens", defaultMaxTokens, "llama-server max_tokens for the response")
		temp          = flag.Float64("temperature", 0.4, "sampling temperature (0.0 = greedy)")
		showCtx       = flag.Bool("show-context", false, "print retrieved references before the answer")
		maxChars      = flag.Int("max-topic-chars", defaultMaxTopicChars, "per-topic content cap; oversized topics are truncated to fit the context window")
		seed          = flag.Int("seed", 0, "llama-server seed; nonzero pins sampling for reproducibility (combine with -temperature 0 for greedy determinism)")
		sysPromptFile = flag.String("system-prompt-file", "", "path to a file containing the system prompt; '' = built-in default; '-' = no system message at all (probe baseline)")
	)
	cmdline.Parse(cmdline.Spec{
		Name:     "zhen-rag",
		Purpose:  "RAG glue between vor (cs cheatsheet API) and a local llama-server.",
		Examples: []string{"zhen-rag -q 'how do I rotate the WireGuard PSK?'"},
	})

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

	// 1. retrieve search hits. vorquery drops question words and
	// punctuation (vor returns null for a query ending in `?`, `!`, `.`);
	// the full question still goes to the LLM.
	// vor is read through pkg/vorclient: a deadline on every call and a
	// bound on every body.
	vor, err := vorclient.New(vorURL, client)
	if err != nil {
		fatal("%v", err)
	}
	terms := vorquery.SearchTerms(q)
	hits, err := vor.Search(ctx, strings.Join(terms, " "), vorclient.SearchOptions{})
	if err != nil {
		fatal("vor search: %v", err)
	}
	hitCategory := make(map[string]string) // topic name to the category of its first hit
	for _, h := range hits {
		if _, seen := hitCategory[h.Topic]; !seen {
			hitCategory[h.Topic] = h.Category
		}
	}

	// 2. fetch full content for top-K unique topics: topics named by the
	// question interleaved with vor's hits (see pkg/vorquery).
	order, err := vorquery.NewIndex(vor).Order(ctx, terms, hitTopics(hits))
	if err != nil {
		fmt.Fprintf(os.Stderr, "zhen-rag: topic listing: %v\n", err)
	}
	seen := make(map[string]struct{})
	var topics []vorclient.Topic
	for _, name := range order {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		// The hit says which category its topic is in, so a hit is never
		// answered with the text of another topic of the same name.
		t, err := vor.TopicFor(ctx, hitCategory[name], name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "zhen-rag: skip topic %q: %v\n", name, err)
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
			trust := t.SourceTrust
			if trust == "" {
				trust = "canonical"
			}
			label := ""
			if t.SourceLabel != "" {
				label = " (source: " + t.SourceLabel + ")"
			}
			fmt.Fprintf(os.Stderr, "  • [%s] %s/%s — %s%s\n",
				trust, t.Category, t.Name, t.Title, label)
		}
		fmt.Fprintln(os.Stderr, "────────────────────────────")
	}

	// 3. format prompt
	systemPrompt, err := resolveSystemPrompt(*sysPromptFile)
	if err != nil {
		fatal("read system-prompt-file: %v", err)
	}

	var refs strings.Builder
	for _, t := range topics {
		// Trust label is shown to the model so it can apply the source-
		// trust system-prompt clause. Default to "canonical" if vor's
		// response is from a pre-B1 build (back-compat — older vor
		// servers don't emit source_trust at all).
		trust := t.SourceTrust
		if trust == "" {
			trust = "canonical"
		}
		refs.WriteString("\n\n--- [")
		refs.WriteString(trust)
		refs.WriteString("] ")
		refs.WriteString(t.Category)
		refs.WriteString("/")
		refs.WriteString(t.Name)
		if t.SourceLabel != "" {
			refs.WriteString(" (source: ")
			refs.WriteString(t.SourceLabel)
			refs.WriteString(")")
		}
		refs.WriteString(" ---\n")
		refs.WriteString(clipContent(t.Content, *maxChars))
	}

	userPrompt := fmt.Sprintf(
		"References:%s\n\nQuestion: %s\n\n%s",
		refs.String(), q, codeCommentReminder,
	)

	// 4. call llama-server
	messages := make([]chatMessage, 0, 2+len(codeCommentExample))
	if systemPrompt != "" {
		messages = append(messages, chatMessage{Role: "system", Content: systemPrompt})
	}
	// The worked example reads as the conversation's first exchange.
	messages = append(messages, codeCommentExample...)
	messages = append(messages, chatMessage{Role: "user", Content: userPrompt})
	answer, err := llamaChat(ctx, client, llamaURL, chatRequest{
		Model:       model,
		Messages:    messages,
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

// resolveSystemPrompt maps the -system-prompt-file flag to the system
// message: "" is the built-in prompt (zhenrag.SystemPrompt(zhenrag.Chat),
// whose history is in pkg/zhenrag/prompt.go), "-" is no system message at
// all (probe baseline; user message only), anything else is a file's
// contents.
func resolveSystemPrompt(flagVal string) (string, error) {
	switch flagVal {
	case "":
		return zhenrag.SystemPrompt(zhenrag.Chat), nil
	case "-":
		return "", nil
	}
	buf, err := os.ReadFile(flagVal) // #nosec G304 -- operator-supplied flag path, read-only
	if err != nil {
		return "", err
	}
	return string(buf), nil
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

// hitTopics returns each search hit's topic and category, in vor's order.
func hitTopics(hits []vorclient.Hit) []vorquery.Topic {
	out := make([]vorquery.Topic, len(hits))
	for i, h := range hits {
		out[i] = vorquery.Topic{Name: h.Topic, Category: h.Category}
	}
	return out
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
