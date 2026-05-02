// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

// Command zhen-agent is the CLI entry point for the Phase D-A agent
// runtime. Wires:
//
//   pkg/agent       — minimal ReAct loop
//   pkg/champion    — Trust L2 tool execution gated by B1+B2 defenses
//   cs/vor          — retrieval (cheatsheets + Unheaded markdown)
//   llama-server    — LLM (qwen2.5-coder-7b-instruct via llama.cpp)
//
// Differences from cmd/zhen-rag (which is one-shot RAG):
//   - Multi-turn loop with tool dispatch
//   - Tool calls go through Champion's gate (B1 trust + B2 destructive-
//     verb + path-allowlist + pending-confirmation)
//   - Bounded by --max-turns (default 8) instead of one-shot
//
// stdlib-only HTTP — same discipline as zhen-rag.
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
	"path/filepath"
	"strings"
	"time"

	"unheaded/pkg/agent"
	"unheaded/pkg/champion"
)

const (
	defaultVorURL    = "http://127.0.0.1:9876"
	defaultLlamaURL  = "http://127.0.0.1:8081"
	defaultModel     = "qwen2.5-coder-7b-instruct"
	defaultK         = 5
	defaultMaxTokens = 600
	defaultMaxTurns  = 8
	defaultMaxChars  = 10000
	httpTimeout      = 5 * time.Minute
)

func main() {
	var (
		goal        = flag.String("q", "", "user goal (otherwise reads from stdin)")
		k           = flag.Int("k", defaultK, "number of top-K topics for seed retrieval")
		maxTokens   = flag.Int("max-tokens", defaultMaxTokens, "llama-server max_tokens per turn")
		maxTurns    = flag.Int("max-turns", defaultMaxTurns, "agent loop budget")
		maxChars    = flag.Int("max-topic-chars", defaultMaxChars, "per-topic content cap")
		temp        = flag.Float64("temperature", 0.4, "sampling temperature")
		seed        = flag.Int("seed", 0, "llama-server seed (nonzero pins sampling)")
		projectRoot = flag.String("project-root", "", "Champion sandbox root (default: cwd)")
		showTrace   = flag.Bool("show-trace", false, "print per-turn trace to stderr")
	)
	flag.Parse()

	vorURL := envOr("VOR_URL", defaultVorURL)
	llamaURL := envOr("LLAMA_URL", defaultLlamaURL)
	modelName := envOr("RAG_MODEL", defaultModel)

	q := strings.TrimSpace(*goal)
	if q == "" {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal("read stdin: %v", err)
		}
		q = strings.TrimSpace(string(buf))
	}
	if q == "" {
		fatal("no goal (use -q or pipe via stdin)")
	}

	root := *projectRoot
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fatal("getwd: %v", err)
		}
		root = cwd
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		fatal("resolve project-root: %v", err)
	}

	champ, err := champion.New(champion.Config{
		ProjectRoot: rootAbs,
		// AllowedPaths default to ProjectRoot.
	}, &stderrActionStore{})
	if err != nil {
		fatal("champion.New: %v", err)
	}

	client := &http.Client{Timeout: httpTimeout}
	a := agent.New(
		agent.Config{
			MaxTurns:      *maxTurns,
			MaxTokens:     *maxTokens,
			RetrieveK:     *k,
			MaxTopicChars: *maxChars,
			Temperature:   *temp,
			Seed:          *seed,
		},
		champ,
		&vorRetriever{client: client, baseURL: vorURL, maxChars: *maxChars},
		&llamaLLM{client: client, baseURL: llamaURL, model: modelName},
	)

	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	res, err := a.Run(ctx, q)
	if err != nil {
		fatal("agent.Run: %v", err)
	}

	if *showTrace {
		fmt.Fprintln(os.Stderr, "─── agent trace ───")
		for i, t := range res.Trace {
			fmt.Fprintf(os.Stderr, "[turn %d] thought: %s\n", i+1, oneLine(t.Thought))
			if t.ToolCall != nil {
				fmt.Fprintf(os.Stderr, "         tool: %s args=%s\n",
					t.ToolCall.Name, briefArgs(t.ToolCall.Args))
				if t.Refused {
					tag := "REFUSED"
					if t.Pending {
						tag = "REFUSED-PENDING"
					}
					fmt.Fprintf(os.Stderr, "         %s\n", tag)
					if t.PendingToken != "" {
						fmt.Fprintf(os.Stderr, "         confirm-token: %s\n", t.PendingToken)
					}
				}
				fmt.Fprintf(os.Stderr, "         observation: %s\n", oneLine(t.Observation))
			}
			if t.Answer != "" {
				fmt.Fprintf(os.Stderr, "         answer: %s\n", oneLine(t.Answer))
			}
		}
		fmt.Fprintf(os.Stderr, "─── %d turn(s) used; budget-hit=%v ───\n",
			res.TurnsUsed, res.BudgetHit)
	}

	fmt.Println(res.Answer)
}

// --- vorRetriever: agent.Retriever wrapping cs/vor /api ---

type vorRetriever struct {
	client   *http.Client
	baseURL  string
	maxChars int
}

func (r *vorRetriever) Retrieve(ctx context.Context, query string, k int) ([]champion.Reference, []agent.TopicContent, error) {
	hits, err := r.search(ctx, searchQuery(query))
	if err != nil {
		return nil, nil, fmt.Errorf("vor search: %w", err)
	}

	seen := make(map[string]struct{})
	var refs []champion.Reference
	var contents []agent.TopicContent
	for _, h := range hits {
		if _, dup := seen[h.Topic]; dup {
			continue
		}
		seen[h.Topic] = struct{}{}
		t, err := r.topic(ctx, h.Topic)
		if err != nil {
			fmt.Fprintf(os.Stderr, "zhen-agent: skip topic %q: %v\n", h.Topic, err)
			continue
		}
		ref := champion.Reference{
			Topic:       t.Name,
			Category:    t.Category,
			SourceKind:  t.SourceKind,
			SourceTrust: t.SourceTrust,
			SourcePath:  t.SourcePath,
			SourceLabel: t.SourceLabel,
		}
		refs = append(refs, ref)
		contents = append(contents, agent.TopicContent{
			Ref:     ref,
			Content: clipContent(t.Content, r.maxChars),
		})
		if len(refs) >= k {
			break
		}
	}
	return refs, contents, nil
}

type vorSearchHit struct {
	Topic       string `json:"topic"`
	Category    string `json:"category"`
	Section     string `json:"section"`
	Line        string `json:"line"`
	SourceKind  string `json:"source_kind"`
	SourceTrust string `json:"source_trust"`
	SourcePath  string `json:"source_path,omitempty"`
	SourceLabel string `json:"source_label,omitempty"`
}

type vorTopic struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	SeeAlso     []string `json:"see_also"`
	HasDetail   bool     `json:"has_detail"`
	SourceKind  string   `json:"source_kind"`
	SourceTrust string   `json:"source_trust"`
	SourcePath  string   `json:"source_path,omitempty"`
	SourceLabel string   `json:"source_label,omitempty"`
}

func (r *vorRetriever) search(ctx context.Context, q string) ([]vorSearchHit, error) {
	endpoint := fmt.Sprintf("%s/api/search?q=%s",
		strings.TrimRight(r.baseURL, "/"), url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
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

func (r *vorRetriever) topic(ctx context.Context, name string) (*vorTopic, error) {
	endpoint := fmt.Sprintf("%s/api/topics/%s",
		strings.TrimRight(r.baseURL, "/"), url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
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

// --- llamaLLM: agent.LLM wrapping llama-server's OpenAI-compatible /v1/chat/completions ---

type llamaLLM struct {
	client  *http.Client
	baseURL string
	model   string
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

func (l *llamaLLM) Complete(ctx context.Context, msgs []agent.Message, maxTokens int, temp float64, seed int) (string, error) {
	cm := make([]chatMessage, len(msgs))
	for i, m := range msgs {
		cm[i] = chatMessage{Role: m.Role, Content: m.Content}
	}
	body, err := json.Marshal(chatRequest{
		Model:       l.model,
		Messages:    cm,
		MaxTokens:   maxTokens,
		Temperature: temp,
		Seed:        seed,
		Stream:      false,
	})
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(l.baseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.client.Do(req)
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

// --- stderrActionStore: ActionStore that prints to stderr (no DB binding) ---

type stderrActionStore struct {
	nextID int64
}

func (s *stderrActionStore) LogAction(_ context.Context, a *champion.Action) (int64, error) {
	s.nextID++
	a.ID = s.nextID
	fmt.Fprintf(os.Stderr, "[champion] log #%d: %s — %s\n",
		s.nextID, a.ActionType, a.Intent)
	return s.nextID, nil
}

func (s *stderrActionStore) UpdateAction(_ context.Context, id int64, status, _, errMsg string, _ int) error {
	if errMsg != "" {
		fmt.Fprintf(os.Stderr, "[champion] log #%d: %s (%s)\n", id, status, errMsg)
	} else {
		fmt.Fprintf(os.Stderr, "[champion] log #%d: %s\n", id, status)
	}
	return nil
}

func (s *stderrActionStore) GetActions(_ context.Context, _ string, _ int) ([]champion.Action, error) {
	return nil, nil
}

// --- helpers shared with zhen-rag ---

func searchQuery(q string) string {
	return strings.TrimRight(q, "?!.")
}

func clipContent(s string, maxChars int) string {
	if maxChars <= 0 || len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "\n\n…[truncated]"
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "zhen-agent: "+format+"\n", args...)
	os.Exit(1)
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

func briefArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	clipped := make(map[string]any, len(args))
	for k, v := range args {
		if s, ok := v.(string); ok && len(s) > 80 {
			clipped[k] = s[:80] + "…"
		} else {
			clipped[k] = v
		}
	}
	b, _ := json.Marshal(clipped)
	return string(b)
}
