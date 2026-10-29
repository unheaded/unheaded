// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

// Command zhen-cli is the interactive terminal counterpart of the
// raft/zhen_app.py web UI. ADR-059 Phase 1: minimum viable REPL.
//
// Reads from stdin in a loop, calls the same vor (retrieval) +
// llama-server (inference) backends the web UI uses, prints the
// answer, keeps a session-local conversation history, and exits on
// /exit, /quit, EOF, or SIGINT.
//
// When stdin is not a tty (pipe mode), behaves like cmd/zhen-rag:
// reads one question, prints one answer, exits 0.
//
// Phase 1 (shipped 2026-05-03): chat + /help + /exit + /clear + /health.
// Phase 2 (HTTP-only slice, 2026-05-06): /runbook list, /runbook show,
// /runbook exec (or bare /runbook <name>), and /source — routed through
// cmd/zhen-agentd /api/v1/tool/exec or vor topics.
// Phase 2b (2026-10-27, S-CHAMPION-HARDENING Step 209): /recall and
// /remember through zhen-agentd's /api/v1/memory/* (agentd holds The
// Well's DSN; this binary has no database driver).
// Phase 3 (2026-10-27, Steps 209/213): /confirm <token>, /model [key]
// (list from agentd's /api/v1/models, swap through model_switch), and line
// editing with history (lineedit.go). Tab completion is not done.
//
// Auth: ZHEN_API_KEY (or the older ZHEN_AGENTD_API_KEY), else api_key in
// ~/.config/unheaded/zhen-cli.conf (mode 0600, refused otherwise), sent to
// zhen-agentd as Authorization: Bearer and X-API-Key.
//
// T6b closure (Phase 3 in ADR-059): every mutation slash command in
// this CLI hits cmd/zhen-agentd /api/v1/tool/exec, which dispatches
// through pkg/champion.Dispatch — Champion's three-rule gate is in
// the path automatically. The CLI inherits T6b closure by routing,
// not by reimplementation.
//
// No third-party readline: lineedit.go is a small editor over bufio with
// raw mode from golang.org/x/sys/unix (already a module dependency).
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
	"os/signal"
	"strings"
	"syscall"
	"time"

	"unheaded/pkg/cmdline"
	"unheaded/pkg/vorquery"
	"unheaded/pkg/zhenrag"
)

const (
	defaultVorURL        = "http://127.0.0.1:9876"
	defaultLlamaURL      = "http://127.0.0.1:8081"
	defaultAgentdURL     = "http://127.0.0.1:20105"
	defaultModel         = "qwen2.5-coder-7b-instruct"
	defaultK             = 5
	defaultMaxTokens     = 800
	defaultMaxTopicChars = 10000
	defaultHistoryLimit  = 6 // last N user/assistant turn pairs kept in context
	httpTimeout          = 5 * time.Minute

	// ANSI colour codes — only emitted when stdout is a tty.
	clrReset = "\x1b[0m"
	clrDim   = "\x1b[2m"
	clrBold  = "\x1b[1m"
	clrCyan  = "\x1b[36m"
	clrGreen = "\x1b[32m"
	clrYel   = "\x1b[33m"
	clrRed   = "\x1b[31m"
)

// Wire types — duplicated from cmd/zhen-rag/main.go to keep zhen-cli
// stdlib-only and shippable without refactoring zhen-rag. Phase 3 will
// extract a shared pkg/zhenrag/ when consolidating.

type vorSearchHit struct {
	Topic       string `json:"topic"`
	Category    string `json:"category"`
	SourceTrust string `json:"source_trust"`
	SourceLabel string `json:"source_label,omitempty"`
}

type vorTopic struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	SourceTrust string `json:"source_trust"`
	SourceLabel string `json:"source_label,omitempty"`
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
	Stream      bool          `json:"stream"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// toolExecRequest mirrors the request shape of cmd/zhen-agentd
// /api/v1/tool/exec (see cmd/zhen-agentd/toolexec.go for canonical
// wire format). The daemon refuses a direct caller without a
// justification, so the CLI states direct-user: its user typed the
// command (Champion's Rule 2 escape; Rules 1 and 3 still apply).
type toolExecRequest struct {
	Tool          string         `json:"tool"`
	Args          map[string]any `json:"args"`
	ProjectRoot   string         `json:"project_root,omitempty"`
	SessionID     string         `json:"session_id,omitempty"`
	EmittedBy     string         `json:"emitted_by,omitempty"`
	Justification []reference    `json:"justification,omitempty"`
}

// reference mirrors pkg/champion.Reference's wire shape.
type reference struct {
	Topic       string `json:"topic"`
	Category    string `json:"category"`
	SourceKind  string `json:"source_kind"`
	SourceTrust string `json:"source_trust"`
	SourceLabel string `json:"source_label,omitempty"`
}

// toolExecResponse mirrors the response shape — same shape as
// /api/v1/agent/confirm so the same handling applies.
type toolExecResponse struct {
	Status       string `json:"status"`
	Result       any    `json:"result,omitempty"`
	Reason       string `json:"reason,omitempty"`
	PendingToken string `json:"pending_token,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

// session carries everything that lives across REPL turns.
type session struct {
	history   []chatMessage
	histLimit int
	color     bool
	k         int
	maxTokens int
	maxChars  int
	temp      float64

	vorURL    string
	llamaURL  string
	agentdURL string
	agentdKey string // API key for zhen-agentd (loadAPIKey); "" = none
	model     string

	out    io.Writer // nil: os.Stdout
	errOut io.Writer // nil: os.Stderr

	client *http.Client
	index  *vorquery.Index // vor topic names, built on first ask
}

func main() {
	var (
		k         = flag.Int("k", defaultK, "top-K topics to retrieve per turn")
		maxTokens = flag.Int("max-tokens", defaultMaxTokens, "llama-server max_tokens")
		temp      = flag.Float64("temperature", 0.4, "sampling temperature")
		histLimit = flag.Int("history", defaultHistoryLimit, "user/assistant turn pairs kept in LLM context")
		maxChars  = flag.Int("max-topic-chars", defaultMaxTopicChars, "per-topic content cap")
		oneShot   = flag.String("q", "", "one-shot question; print answer and exit (same as piping)")
		noColor   = flag.Bool("no-color", false, "disable ANSI colour even on a tty")
	)
	cmdline.Parse(cmdline.Spec{
		Name:     "zhen-cli",
		Purpose:  "Interactive terminal REPL for Zhenai: vor retrieval plus llama-server inference (ADR-059).",
		Examples: []string{"zhen-cli", "zhen-cli -q 'what port does Akira use?'", "echo 'question' | zhen-cli"},
		Details:  "Backends come from the environment: VOR_URL, LLAMA_URL, ZHEN_AGENTD_URL, RAG_MODEL.",
	})

	s := &session{
		histLimit: *histLimit,
		color:     !*noColor && isTTY(os.Stdout),
		k:         *k,
		maxTokens: *maxTokens,
		maxChars:  *maxChars,
		temp:      *temp,
		vorURL:    envOr("VOR_URL", defaultVorURL),
		llamaURL:  envOr("LLAMA_URL", defaultLlamaURL),
		agentdURL: envOr("ZHEN_AGENTD_URL", defaultAgentdURL),
		model:     envOr("RAG_MODEL", defaultModel),
		client:    &http.Client{Timeout: httpTimeout},
	}
	key, err := loadAPIKey(os.Getenv, defaultConfPath())
	if err != nil {
		fatal("%v", err)
	}
	s.agentdKey = key

	// Pipe / one-shot mode: stdin not a tty OR -q given. Single Q&A, exit.
	if *oneShot != "" {
		s.answerOne(*oneShot)
		return
	}
	if !isTTY(os.Stdin) {
		buf, err := io.ReadAll(os.Stdin)
		if err != nil {
			fatal("read stdin: %v", err)
		}
		q := strings.TrimSpace(string(buf))
		if q == "" {
			fatal("no question on stdin")
		}
		s.answerOne(q)
		return
	}

	// Interactive mode.
	s.banner()

	// Honour Ctrl-C cleanly — exit 0 like /exit. While a line is being
	// edited the terminal is raw and Ctrl-C arrives as a byte instead
	// (readLine returns errInterrupt); the terminal is restored either way.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		restoreTerminal()
		fmt.Fprintln(os.Stderr)
		s.dim("zhen-cli: signal received, exiting.")
		os.Exit(0)
	}()

	read := newLineReader(os.Stdin, os.Stdout)
	for {
		raw, err := read(s.prompt())
		if err != nil {
			fmt.Fprintln(os.Stderr)
			s.dim("zhen-cli: goodbye.")
			return
		}
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			if s.handleSlash(line) {
				return
			}
			continue
		}
		s.chatTurn(line)
	}
}

// banner prints the welcome message on session start.
func (s *session) banner() {
	if s.color {
		fmt.Fprintf(s.w(), "%s%sZhenai CLI%s — interactive terminal (ADR-059)\n", clrBold, clrCyan, clrReset)
	} else {
		fmt.Fprintln(s.w(), "Zhenai CLI — interactive terminal (ADR-059)")
	}
	fmt.Fprintf(s.w(), "  retrieval: %s   inference: %s   model: %s\n",
		s.vorURL, s.llamaURL, s.model)
	fmt.Fprintln(s.w(), "  /help for commands, /exit to quit (Ctrl-C also works).")
}

// prompt returns the input prompt.
func (s *session) prompt() string {
	if s.color {
		return clrBold + clrGreen + ">" + clrReset + " "
	}
	return "> "
}

// handleSlash dispatches /exit-style commands. Returns true to terminate
// the REPL, false to continue.
func (s *session) handleSlash(line string) bool {
	parts := strings.Fields(line)
	cmd := strings.ToLower(parts[0])
	switch cmd {
	case "/exit", "/quit", "/q":
		s.dim("zhen-cli: goodbye.")
		return true
	case "/help", "/?", "/h":
		s.printHelp()
	case "/clear":
		s.history = nil
		s.dim("zhen-cli: conversation history cleared.")
	case "/history":
		if len(s.history) == 0 {
			s.dim("zhen-cli: history is empty.")
		} else {
			for i, m := range s.history {
				s.dim(fmt.Sprintf("  [%d] %s: %.120s", i, m.Role, m.Content))
			}
		}
	case "/health":
		s.printHealth()
	case "/runbook":
		s.handleRunbook(parts[1:])
	case "/source":
		s.handleSource(parts[1:])
	case "/recall":
		s.handleRecall(strings.TrimSpace(strings.TrimPrefix(line, parts[0])))
	case "/remember":
		s.handleRemember(strings.TrimSpace(strings.TrimPrefix(line, parts[0])))
	case "/confirm":
		s.handleConfirm(parts[1:])
	case "/model":
		s.handleModel(parts[1:])
	default:
		s.warn("unknown command: " + cmd + "  (try /help)")
	}
	return false
}

// handleRunbook dispatches /runbook subcommands.
//
//	/runbook                     → equivalent to /runbook list (gentle default)
//	/runbook list                → toolExec("runbook_list", {})
//	/runbook show <name>         → toolExec("runbook_show", {name})
//	/runbook exec <name>         → toolExec("runbook_execute", {name, dry_run: false})
//	/runbook <name>              → /runbook exec <name> (Champion gate still applies)
//
// All three runbook tools are routed through /api/v1/tool/exec, so
// Champion's three-rule gate evaluates each call. runbook_execute is
// the only mutating one — the daemon may respond with status
// "pending_confirmation" and a pending_token; the CLI surfaces both
// and how to redeem it (/confirm <token>).
func (s *session) handleRunbook(args []string) {
	if len(args) == 0 || args[0] == "list" {
		resp, err := s.toolExec("runbook_list", map[string]any{})
		if err != nil {
			s.errf("/runbook list: %v", err)
			return
		}
		s.printToolExecResponse("runbook_list", resp)
		return
	}
	switch strings.ToLower(args[0]) {
	case "show":
		if len(args) < 2 {
			s.warn("/runbook show <name>")
			return
		}
		resp, err := s.toolExec("runbook_show", map[string]any{"name": args[1]})
		if err != nil {
			s.errf("/runbook show: %v", err)
			return
		}
		s.printToolExecResponse("runbook_show", resp)
	case "exec":
		if len(args) < 2 {
			s.warn("/runbook exec <name>")
			return
		}
		resp, err := s.toolExec("runbook_execute", map[string]any{"name": args[1], "dry_run": false})
		if err != nil {
			s.errf("/runbook exec: %v", err)
			return
		}
		s.printToolExecResponse("runbook_execute", resp)
	default:
		// Bare /runbook <name> = exec.
		resp, err := s.toolExec("runbook_execute", map[string]any{"name": args[0], "dry_run": false})
		if err != nil {
			s.errf("/runbook: %v", err)
			return
		}
		s.printToolExecResponse("runbook_execute", resp)
	}
}

// handleSource fetches a single vor topic and prints it.
func (s *session) handleSource(args []string) {
	if len(args) == 0 {
		s.warn("/source <topic>")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t, err := s.vorTopicFetch(ctx, args[0])
	if err != nil {
		s.errf("/source: %v", err)
		return
	}
	trust := t.SourceTrust
	if trust == "" {
		trust = "canonical"
	}
	s.dim(fmt.Sprintf("[%s] %s/%s — %s", trust, t.Category, t.Name, t.Title))
	fmt.Fprintln(s.w(), clipContent(t.Content, s.maxChars))
}

// printToolExecResponse renders a toolExecResponse with severity
// based on status.
func (s *session) printToolExecResponse(tool string, resp *toolExecResponse) {
	switch resp.Status {
	case "ok":
		// Pretty-print the result blob if it's a string or simple shape;
		// fall back to JSON otherwise. Keep it compact.
		switch v := resp.Result.(type) {
		case string:
			fmt.Fprintln(s.w(), v)
		case nil:
			s.ok(fmt.Sprintf("%s: ok (no result body)", tool))
		default:
			b, _ := json.MarshalIndent(v, "", "  ")
			fmt.Fprintln(s.w(), string(b))
		}
	case "pending_confirmation":
		s.warn(fmt.Sprintf("%s: requires confirmation — Champion gate Rule 2.", tool))
		if resp.Reason != "" {
			s.dim("  reason: " + resp.Reason)
		}
		if resp.PendingToken != "" {
			s.dim("  pending_token: " + resp.PendingToken)
			s.dim("  to run it: /confirm " + resp.PendingToken)
		}
	case "denied":
		s.errf("%s: denied — %s", tool, resp.Reason)
	case "unknown", "used", "expired", "error":
		s.errf("%s: %s — %s", tool, resp.Status, resp.Reason)
	default:
		s.warn(fmt.Sprintf("%s: unhandled status %q", tool, resp.Status))
		b, _ := json.MarshalIndent(resp, "", "  ")
		fmt.Fprintln(s.w(), string(b))
	}
}

// toolExec POSTs to cmd/zhen-agentd /api/v1/tool/exec and decodes
// the response. Mutations are gated by pkg/champion.Dispatch; reads
// (runbook_list, runbook_show) pass through but are still gated.
func (s *session) toolExec(tool string, args map[string]any) (*toolExecResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()
	body, err := json.Marshal(s.toolExecBody(tool, args))
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(s.agentdURL, "/") + "/api/v1/tool/exec"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	s.setAuth(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zhen-agentd unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var out toolExecResponse
	// Both 200 and 4xx may carry a JSON body in the documented shape.
	if jerr := json.Unmarshal(raw, &out); jerr != nil {
		return nil, fmt.Errorf("status %d, decode: %w (body: %.200s)", resp.StatusCode, jerr, raw)
	}
	return &out, nil
}

// toolExecBody is a /api/v1/tool/exec request for a command the user typed:
// direct-user, Champion's Rule 2 escape (Rules 1 and 3 still apply).
func (s *session) toolExecBody(tool string, args map[string]any) toolExecRequest {
	return toolExecRequest{
		Tool:      tool,
		Args:      args,
		EmittedBy: "zhen-cli",
		Justification: []reference{{
			Topic: "user-typed-" + tool, Category: "user-action", SourceKind: "user-action",
			SourceTrust: "direct-user", SourceLabel: "zhen-cli",
		}},
	}
}

func (s *session) printHelp() {
	fmt.Fprintln(s.w(), "  /help                     this help")
	fmt.Fprintln(s.w(), "  /exit, /quit              exit the REPL")
	fmt.Fprintln(s.w(), "  /clear                    drop in-session conversation history")
	fmt.Fprintln(s.w(), "  /history                  show what the LLM is seeing as prior turns")
	fmt.Fprintln(s.w(), "  /health                   probe llama-server, vor, zhen-agentd")
	fmt.Fprintln(s.w())
	fmt.Fprintln(s.w(), "  /runbook list             list available runbooks (Champion-gated)")
	fmt.Fprintln(s.w(), "  /runbook show <name>      print a runbook without running it")
	fmt.Fprintln(s.w(), "  /runbook exec <name>      run a runbook (Champion gate Rule 2)")
	fmt.Fprintln(s.w(), "  /runbook <name>           shorthand for /runbook exec <name>")
	fmt.Fprintln(s.w(), "  /source <topic>           fetch one vor topic body")
	fmt.Fprintln(s.w())
	fmt.Fprintln(s.w(), "  /recall <text>            remembered answers matching text, with who stored them")
	fmt.Fprintln(s.w(), "  /remember [text]          store text as a note, or with no text the last exchange")
	fmt.Fprintln(s.w(), "  /confirm <token>          run a call the gate held for confirmation")
	fmt.Fprintln(s.w(), "  /model                    list models (* = serving now)")
	fmt.Fprintln(s.w(), "  /model <key>              swap llama-server to <key> (45 s to minutes; rolls back on timeout)")
	fmt.Fprintln(s.w())
	fmt.Fprintln(s.w(), "  any other line           → question to Zhen (RAG: vor → qwen-coder)")
	fmt.Fprintln(s.w())
	fmt.Fprintln(s.w(), "  pipe-mode:  echo \"...\" | zhen-cli   (one-shot answer + exit)")
	fmt.Fprintln(s.w(), "  one-shot:   zhen-cli -q \"...\"        (one-shot answer + exit)")
}

// printHealth probes the three backends and reports up/down.
func (s *session) printHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	probe := func(label, url string) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := s.client.Do(req)
		if err != nil {
			s.warn(fmt.Sprintf("  %-22s DOWN (%v)", label, err))
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			s.ok(fmt.Sprintf("  %-22s ok", label))
		} else {
			s.warn(fmt.Sprintf("  %-22s status %d", label, resp.StatusCode))
		}
	}
	probe("llama-server (LLM)", s.llamaURL+"/health")
	probe("vor (retrieval)", s.vorURL+"/api/health")
	probe("zhen-agentd (gate)", s.agentdURL+"/health")
}

// answerOne handles non-interactive single-question mode: identical
// to a chatTurn but without history persistence and without the
// "Sources" decoration (pipe consumers usually want just the answer).
func (s *session) answerOne(q string) {
	answer, _, err := s.askLLM(q)
	if err != nil {
		fatal("%v", err)
	}
	fmt.Fprintln(s.w(), answer)
}

// chatTurn runs one user → assistant exchange in the REPL: retrieves,
// calls the LLM with the running history, prints the answer, appends
// to history.
func (s *session) chatTurn(q string) {
	start := time.Now()
	answer, refs, err := s.askLLM(q)
	if err != nil {
		s.errf("%v", err)
		return
	}

	if s.color {
		fmt.Fprintf(s.w(), "%s%szhen:%s %s\n", clrBold, clrCyan, clrReset, answer)
	} else {
		fmt.Fprintln(s.w(), answer)
	}

	if len(refs) > 0 {
		s.dim(fmt.Sprintf("  sources: %s   |   %s", strings.Join(refs, ", "),
			fmt.Sprintf("%.2fs %s", time.Since(start).Seconds(), s.model)))
	}

	// Append turn to in-session history; cap at histLimit pairs (drop oldest).
	s.history = append(s.history, chatMessage{Role: "user", Content: q})
	s.history = append(s.history, chatMessage{Role: "assistant", Content: answer})
	if max := s.histLimit * 2; len(s.history) > max {
		s.history = s.history[len(s.history)-max:]
	}
}

// askLLM runs the full retrieve → format → inference cycle and returns
// (answer, ref-labels, err). The retrieve+format halves are the same
// shape as cmd/zhen-rag. The differences are: (a) we prepend the
// session history, (b) we return ref labels for the REPL footer.
func (s *session) askLLM(q string) (answer string, refs []string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), httpTimeout)
	defer cancel()

	terms := vorquery.SearchTerms(q)
	hits, err := s.vorSearch(ctx, strings.Join(terms, " "))
	if err != nil {
		return "", nil, fmt.Errorf("vor search: %w", err)
	}
	searched := make([]vorquery.Topic, len(hits))
	for i, h := range hits {
		searched[i] = vorquery.Topic{Name: h.Topic, Category: h.Category}
	}
	if s.index == nil {
		s.index = vorquery.NewIndex(s.client, s.vorURL)
	}
	// A listing error only loses the name matches; vor's order stands.
	order, _ := s.index.Order(ctx, terms, searched)

	seen := make(map[string]struct{})
	var topics []vorTopic
	for _, name := range order {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		t, err := s.vorTopicFetch(ctx, name)
		if err != nil {
			continue
		}
		topics = append(topics, *t)
		if len(topics) >= s.k {
			break
		}
	}

	var rb strings.Builder
	for _, t := range topics {
		trust := t.SourceTrust
		if trust == "" {
			trust = "canonical"
		}
		rb.WriteString("\n\n--- [")
		rb.WriteString(trust)
		rb.WriteString("] ")
		rb.WriteString(t.Category)
		rb.WriteString("/")
		rb.WriteString(t.Name)
		rb.WriteString(" ---\n")
		rb.WriteString(clipContent(t.Content, s.maxChars))
		refs = append(refs, t.Category+"/"+t.Name)
	}

	userPrompt := fmt.Sprintf("References:%s\n\nQuestion: %s", rb.String(), q)

	// Prepend system prompt + prior turns + current user turn.
	messages := make([]chatMessage, 0, len(s.history)+2)
	messages = append(messages, chatMessage{Role: "system", Content: systemPrompt()})
	messages = append(messages, s.history...)
	messages = append(messages, chatMessage{Role: "user", Content: userPrompt})

	answer, err = s.llamaChat(ctx, chatRequest{
		Model:       s.model,
		Messages:    messages,
		MaxTokens:   s.maxTokens,
		Temperature: s.temp,
		Stream:      false,
	})
	if err != nil {
		return "", nil, fmt.Errorf("llama: %w", err)
	}
	return answer, refs, nil
}

// systemPrompt is cmd/zhen-rag's chat prompt (zhenrag.SystemPrompt(
// zhenrag.Chat)): the CLI used to carry a shortened paraphrase of it, which
// had no code-comments rule and drifted from the verb list (RT-07/RT-08).
func systemPrompt() string {
	return zhenrag.SystemPrompt(zhenrag.Chat)
}

// HTTP helpers — same shapes as cmd/zhen-rag.

func (s *session) vorSearch(ctx context.Context, q string) ([]vorSearchHit, error) {
	endpoint := fmt.Sprintf("%s/api/search?q=%s",
		strings.TrimRight(s.vorURL, "/"), url.QueryEscape(q))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	resp, err := s.client.Do(req)
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

func (s *session) vorTopicFetch(ctx context.Context, name string) (*vorTopic, error) {
	endpoint := fmt.Sprintf("%s/api/topics/%s",
		strings.TrimRight(s.vorURL, "/"), url.PathEscape(name))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	resp, err := s.client.Do(req)
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

func (s *session) llamaChat(ctx context.Context, req chatRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimRight(s.llamaURL, "/") + "/v1/chat/completions"
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(httpReq)
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

// Small helpers — keep zhen-cli single-file for Phase 1.

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

// isTTY reports whether the given file is a terminal. We avoid pulling
// in golang.org/x/term for Phase 1 — checking the mode of the underlying
// fd via os.File.Stat is enough to distinguish a pipe from a tty on Linux.
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (s *session) dim(msg string) {
	if s.color {
		fmt.Fprintln(s.w(), clrDim+msg+clrReset)
	} else {
		fmt.Fprintln(s.w(), msg)
	}
}

func (s *session) ok(msg string) {
	if s.color {
		fmt.Fprintln(s.w(), clrGreen+msg+clrReset)
	} else {
		fmt.Fprintln(s.w(), msg)
	}
}

func (s *session) warn(msg string) {
	if s.color {
		fmt.Fprintln(s.w(), clrYel+msg+clrReset)
	} else {
		fmt.Fprintln(s.w(), msg)
	}
}

func (s *session) errf(format string, args ...any) {
	msg := fmt.Sprintf("zhen-cli: "+format, args...)
	if s.color {
		fmt.Fprintln(s.ew(), clrRed+msg+clrReset)
	} else {
		fmt.Fprintln(s.ew(), msg)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "zhen-cli: "+format+"\n", args...)
	os.Exit(1)
}
