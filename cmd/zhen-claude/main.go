// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Command zhen-claude is ZhenAI's bridge to the Claude API (ADR-104): a
// one-shot prompt or a REPL against Claude Opus 5.5, with tools from the
// Champion and, optionally, Zhen's MCP server. Every tool call Claude
// makes goes through the Champion gate; every request is held against a
// hard daily spend cap before it is sent; every request and tool call is
// audited as a JSON line on stderr (token counts and cost, no prompt
// text).
//
// Credentials come from the environment (ANTHROPIC_API_KEY) or an `ant
// auth login` profile, never from this repository. Without one the
// command stops before building a request, except under -dry-run, which
// prints the request it would send and sends nothing.
//
//	zhen-claude -p "Summarise ADR-104 in two sentences" -max-tokens 1024
//	zhen-claude -repl -tools champion,mcp
//	zhen-claude -dry-run -p "hello"
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/mcpclient"
)

type options struct {
	prompt        string
	repl          bool
	dryRun        bool
	tools         string
	mcpCmd        string
	mcpMutating   bool
	projectRoot   string
	model         string
	effort        string
	maxTokens     int64
	taskBudget    int64
	noFallback    bool
	maxIterations int
	dailyCapUSD   float64
	ledgerPath    string
	pricesPath    string
	logLevel      string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func parseFlags(args []string, stderr io.Writer) (*options, error) {
	o := &options{}
	fs := flag.NewFlagSet("zhen-claude", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.prompt, "p", "", "one-shot prompt (default: read stdin, unless -repl)")
	fs.BoolVar(&o.repl, "repl", false, "interactive conversation; one line per turn, /quit to leave")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the request that would be sent and exit; needs no credentials, sends nothing")
	fs.StringVar(&o.tools, "tools", "champion", "tool providers, comma separated: champion, mcp (or none)")
	fs.StringVar(&o.mcpCmd, "mcp-cmd", "python3 raft/zhen_mcp_server.py", "MCP stdio server command for -tools mcp (split on spaces, no shell)")
	fs.BoolVar(&o.mcpMutating, "mcp-mutating", false, "also offer the MCP server's mutating tools (file_write, file_patch, runbook_execute); they still go through the Champion gate")
	fs.StringVar(&o.projectRoot, "project-root", "", "Champion sandbox root (default: current directory)")
	fs.StringVar(&o.model, "model", claudebridge.DefaultModel, "Claude model ID")
	fs.StringVar(&o.effort, "effort", claudebridge.DefaultEffort, "effort: low, medium, high, xhigh, max")
	fs.Int64Var(&o.maxTokens, "max-tokens", claudebridge.DefaultMaxTokens, "max_tokens per request (thinking counts toward it)")
	fs.Int64Var(&o.taskBudget, "task-budget", 0, "advisory task budget in tokens per request (beta; 0 = off, minimum 20000)")
	fs.BoolVar(&o.noFallback, "no-fallback", false, "disable the server-side refusal fallback (fallbacks: \"default\")")
	fs.IntVar(&o.maxIterations, "max-iterations", claudebridge.DefaultMaxIterations, "API calls allowed per turn (tool loop bound)")
	fs.Float64Var(&o.dailyCapUSD, "daily-cap-usd", envFloat("ZHEN_CLAUDE_DAILY_CAP_USD", 5), "hard spend cap per UTC day in USD (env ZHEN_CLAUDE_DAILY_CAP_USD)")
	fs.StringVar(&o.ledgerPath, "ledger", "", "spend ledger file (default $XDG_STATE_HOME/unheaded/zhen-claude/spend.json)")
	fs.StringVar(&o.pricesPath, "prices", "", "price table JSON (default: the embedded pkg/claudebridge/prices.json)")
	fs.StringVar(&o.logLevel, "log-level", "info", "audit log level: debug, info, warn, error")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected arguments %q; pass the prompt with -p", fs.Args())
	}
	if o.repl && o.prompt != "" {
		return nil, errors.New("-p and -repl are exclusive")
	}
	if o.repl && o.dryRun {
		return nil, errors.New("-dry-run works on one prompt, not -repl")
	}
	return o, nil
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	o, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		_, _ = fmt.Fprintln(stderr, "zhen-claude:", err)
		return 2
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(o.logLevel)); err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude: -log-level:", err)
		return 2
	}
	log := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: level})).With("service", "zhen-claude")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	b, closeAll, err := build(ctx, o, log, stdout)
	defer closeAll()
	if err != nil {
		log.Error("startup", "error", err.Error())
		return 1
	}

	if o.repl {
		return repl(ctx, b, stdin, stdout, stderr, log)
	}
	prompt := o.prompt
	if prompt == "" {
		raw, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
		if err != nil {
			log.Error("read stdin", "error", err.Error())
			return 1
		}
		prompt = string(raw)
	}
	if strings.TrimSpace(prompt) == "" {
		_, _ = fmt.Fprintln(stderr, "zhen-claude: no prompt (use -p, stdin, or -repl)")
		return 2
	}
	conv := b.NewConversation()
	if o.dryRun {
		return dryRun(conv, b, prompt, stdout, log)
	}
	if !turn(ctx, conv, prompt, stdout, stderr, log) {
		return 1
	}
	return 0
}

// build wires providers, the gate, the cap and the client. closeAll is
// always safe to call.
func build(ctx context.Context, o *options, log *slog.Logger, stdout io.Writer) (*claudebridge.Bridge, func(), error) {
	var closers []func()
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	root := o.projectRoot
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, closeAll, err
		}
		root = wd
	}
	// No ActionStore: the bridge's own audit lines record every decision.
	champ, err := champion.New(champion.Config{ProjectRoot: root}, nil)
	if err != nil {
		return nil, closeAll, err
	}
	gate, err := claudebridge.NewChampionGate(champ)
	if err != nil {
		return nil, closeAll, err
	}

	var providers []claudebridge.NamedProvider
	for _, name := range strings.Split(o.tools, ",") {
		switch strings.TrimSpace(name) {
		case "", "none":
		case "champion":
			cp, err := champion.NewToolProvider(champ, "claude-api/"+o.model)
			if err != nil {
				return nil, closeAll, err
			}
			providers = append(providers, claudebridge.NamedProvider{Name: "champion", Provider: cp})
		case "mcp":
			fields := strings.Fields(o.mcpCmd)
			if len(fields) == 0 {
				return nil, closeAll, errors.New("-mcp-cmd is empty")
			}
			mc, err := mcpclient.Start(ctx, mcpclient.Config{Command: fields[0], Args: fields[1:], Dir: root, ClientName: "zhen-claude"})
			if err != nil {
				return nil, closeAll, fmt.Errorf("start MCP server: %w", err)
			}
			closers = append(closers, func() { _ = mc.Close() })
			var p = claudebridge.AllowTools(mc, claudebridge.MCPReadOnlyTools...)
			if o.mcpMutating {
				log.Warn("mcp_mutating_tools_offered", "note", "every MCP tool is offered; each call still goes through the Champion gate")
				p = mc
			}
			providers = append(providers, claudebridge.NamedProvider{Name: "mcp:" + fields[len(fields)-1], Provider: p})
		default:
			return nil, closeAll, fmt.Errorf("-tools: unknown provider %q (want champion, mcp, none)", name)
		}
	}

	prices, err := claudebridge.DefaultPrices()
	if o.pricesPath != "" {
		prices, err = claudebridge.LoadPrices(o.pricesPath)
	}
	if err != nil {
		return nil, closeAll, err
	}
	if !prices.Has(o.model) {
		log.Warn("model_not_in_price_table", "model", o.model, "note", "priced at the table maximum")
	}
	capNano, err := claudebridge.USDToNano(o.dailyCapUSD)
	if err != nil {
		return nil, closeAll, fmt.Errorf("-daily-cap-usd: %w", err)
	}
	ledgerPath := o.ledgerPath
	if ledgerPath == "" {
		if ledgerPath, err = claudebridge.DefaultLedgerPath(); err != nil {
			return nil, closeAll, err
		}
	}
	ledger, err := claudebridge.NewLedger(ledgerPath, capNano)
	if err != nil {
		return nil, closeAll, err
	}

	cfg := claudebridge.Config{
		Model:            o.model,
		Effort:           o.effort,
		MaxTokens:        o.maxTokens,
		TaskBudget:       o.taskBudget,
		DisableFallbacks: o.noFallback,
		MaxIterations:    o.maxIterations,
		Providers:        providers,
		Gate:             gate,
		Prices:           prices,
		Ledger:           ledger,
		Auditor:          claudebridge.SlogAuditor{Log: log},
		OnText:           func(s string) { _, _ = io.WriteString(stdout, s) },
	}
	if o.dryRun {
		cfg.Credentials = func() (string, error) { return "none (dry run)", nil }
	}
	b, err := claudebridge.New(ctx, cfg)
	if err != nil {
		return nil, closeAll, err
	}
	spent, _ := ledger.Spent()
	log.Info("ready",
		"model", o.model, "effort", o.effort, "max_tokens", o.maxTokens,
		"credentials", b.CredentialSource(), "tools", b.ToolNames(),
		"day_spent_usd", spent.USD(), "daily_cap_usd", ledger.Cap().USD(), "ledger", ledgerPath)
	return b, closeAll, nil
}

func dryRun(conv *claudebridge.Conversation, b *claudebridge.Bridge, prompt string, stdout io.Writer, log *slog.Logger) int {
	body, err := conv.RequestJSON(prompt)
	if err != nil {
		log.Error("dry_run", "error", err.Error())
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "POST /v1/messages\nanthropic-beta: %s\n\n%s\n", strings.Join(b.Betas(), ","), body)
	return 0
}

// turn sends one prompt and reports the outcome; it returns false on error.
func turn(ctx context.Context, conv *claudebridge.Conversation, prompt string, stdout, stderr io.Writer, log *slog.Logger) bool {
	reply, err := conv.Send(ctx, prompt)
	_, _ = io.WriteString(stdout, "\n")
	if reply != nil {
		log.Info("turn_done", "conversation", conv.ID(), "requests", reply.Requests, "cost_usd", reply.Cost.USD(), "served_by", reply.ServedBy, "stop_reason", reply.StopReason)
	}
	if err == nil {
		return true
	}
	var refusal *claudebridge.RefusalError
	switch {
	case errors.As(err, &refusal):
		_, _ = fmt.Fprintf(stderr, "zhen-claude: %v; partial output above (if any) is discarded from the conversation\n", refusal)
	case errors.Is(err, claudebridge.ErrSpendCap):
		_, _ = fmt.Fprintf(stderr, "zhen-claude: %v\n", err)
	case errors.Is(err, claudebridge.ErrMaxTokens):
		_, _ = fmt.Fprintln(stderr, "zhen-claude: the response hit -max-tokens and was cut off; raise it or narrow the prompt")
	default:
		_, _ = fmt.Fprintf(stderr, "zhen-claude: %v\n", err)
	}
	log.Warn("turn_failed", "conversation", conv.ID(), "error", err.Error())
	return false
}

func repl(ctx context.Context, b *claudebridge.Bridge, stdin io.Reader, stdout, stderr io.Writer, log *slog.Logger) int {
	conv := b.NewConversation()
	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for {
		_, _ = io.WriteString(stderr, "> ")
		if !sc.Scan() {
			break
		}
		line := strings.TrimSpace(sc.Text())
		switch line {
		case "":
			continue
		case "/quit", "/exit":
			return 0
		}
		turn(ctx, conv, line, stdout, stderr, log)
		if ctx.Err() != nil {
			return 1
		}
	}
	if err := sc.Err(); err != nil {
		log.Error("read stdin", "error", err.Error())
		return 1
	}
	return 0
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	var f float64
	if _, err := fmt.Sscan(v, &f); err != nil {
		return -1 // rejected by USDToNano with a clear error
	}
	return f
}
