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
// -route auto asks the local model (llama-server's OpenAI-compatible API)
// first and escalates to Claude when it is down, errors, cuts off, or its
// answer fails -check: an argv command, run without a shell, that gets
// the answer on stdin and accepts it by exiting 0. Credentials are needed
// only when it escalates. Each decision is a "route_decision" log line.
//
// -capture <dir> (or $ZHEN_CAPTURE_DIR) appends every turn, redacted, to
// a zhen-capture/v1 JSONL file outside the repo
// (docs/architecture/zhen-capture.md).
//
//	zhen-claude -p "Summarise ADR-104 in two sentences" -max-tokens 1024
//	zhen-claude -repl -tools champion,mcp
//	zhen-claude -dry-run -p "hello"
//	zhen-claude -route auto -check "go run ./tools/check-answer" -p "..."
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
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/claudebridge/privacy"
	"unheaded/pkg/cmdline"
	"unheaded/pkg/mcpclient"
	"unheaded/pkg/zhencapture"
	"unheaded/pkg/zhentools"
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

	route          string
	check          string
	checkArgv      []string
	checkTimeout   time.Duration
	localURL       string
	localModel     string
	localMaxTokens int
	localTimeout   time.Duration
	captureDir     string
	localOnly      bool
	configPath     string

	auditWell  bool
	auditWotan bool
	confirm    string
	tty        io.ReadWriteCloser // -confirm tty: the controlling terminal

	privacyMode   string
	userDataRoots string
	secretScanner string

	family     string
	worktree   string
	routing    string
	runCheck   string
	checkPath  string
	checksPath string
	agentRoot  string
}

// Routes.
const (
	routeClaude = "claude"
	routeAuto   = "auto"
	routeLocal  = "local" // with -family: the routed loop, Claude off
)

// captureToolOutputBytes cuts each tool result in a capture record.
const captureToolOutputBytes = 8 << 10

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
	fs.StringVar(&o.mcpCmd, "mcp-cmd", "scripts/zhen-mcp.sh", "MCP stdio server command for -tools mcp (split on spaces, no shell; the default runs bin/zhen-mcp, built by make build-zhen-mcp)")
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
	fs.StringVar(&o.route, "route", routeClaude, "claude: send to Claude; auto: ask the local model first, escalate to Claude on failure or a failed -check")
	fs.StringVar(&o.check, "check", "", "with -route auto: command (split on spaces, no shell) that gets the local answer on stdin; exit 0 accepts it. Without it only a local failure escalates")
	fs.DurationVar(&o.checkTimeout, "check-timeout", 60*time.Second, "time limit for -check")
	fs.StringVar(&o.localURL, "local-url", envString("ZHEN_LOCAL_LLM_URL", claudebridge.DefaultLocalURL), "llama-server base URL for -route auto (env ZHEN_LOCAL_LLM_URL)")
	fs.StringVar(&o.localModel, "local-model", envString("ZHEN_LOCAL_LLM_MODEL", claudebridge.DefaultLocalModel), "model name sent to the local server (env ZHEN_LOCAL_LLM_MODEL)")
	fs.IntVar(&o.localMaxTokens, "local-max-tokens", claudebridge.DefaultLocalMaxTokens, "max_tokens for the local model")
	fs.DurationVar(&o.localTimeout, "local-timeout", claudebridge.DefaultLocalTimeout, "time limit for the local model's answer")
	fs.StringVar(&o.captureDir, "capture", os.Getenv("ZHEN_CAPTURE_DIR"), "append each turn, redacted, to <dir>/zhen-capture-YYYY-MM-DD.jsonl; must be outside any repository (env ZHEN_CAPTURE_DIR)")
	fs.BoolVar(&o.localOnly, "local-only", false, "keep this prompt on this host: ask the local model only, never Claude (implies -route auto without escalation; config local_only)")
	fs.BoolVar(&o.auditWell, "audit-well", false, "also record every request in the Well's llm_requests and every tool decision in audit_events (DSN from env WELL_OPS_DSN); Well unreachable = read-only run")
	fs.BoolVar(&o.auditWotan, "audit-wotan", false, "also publish the audit records on Wotan (zhen.llm.requests, zhen.audit.toolcalls; HTTP host:port from env WOTAN_ADDR)")
	fs.StringVar(&o.confirm, "confirm", "deny", "what happens to a write the gate refuses pending confirmation: deny (refuse, the default), tty (ask the human on /dev/tty with a code the model never sees), queue (queue it in the Well for `zhen-claude confirm approve <id>`; DSN from env WELL_APP_ZHEN_DSN)")
	fs.StringVar(&o.privacyMode, "privacy", "standard", "privacy tier for tool results bound for Claude: standard (redact; summarise large ops output locally) or strict (withhold every Kingdom-ops result; overnight mode)")
	fs.StringVar(&o.userDataRoots, "user-data-roots", os.Getenv("ZHEN_USER_DATA_ROOTS"), "comma-separated paths whose contents never leave the host (T2, withheld; env ZHEN_USER_DATA_ROOTS)")
	fs.StringVar(&o.secretScanner, "secret-scanner", os.Getenv("ZHEN_SECRET_SCANNER"), "secret scanner binary for the privacy tier (gitleaks 8.19+; default: gitleaks on PATH, then ~/.local/bin/gitleaks; env ZHEN_SECRET_SCANNER). Without one nothing is sent")
	fs.StringVar(&o.family, "family", "", "with -route auto|local: task family; routes by -routing (configs/zhen-routing.yaml) through the threshold router")
	fs.StringVar(&o.worktree, "worktree", "", "with -family: the zhen/<name> agent worktree the task works in (drafts are written there)")
	fs.StringVar(&o.routing, "routing", "configs/zhen-routing.yaml", "routing table generated by scripts/two-brains/thresholds.py")
	fs.StringVar(&o.runCheck, "run-check", "", "with -worktree: allowlisted check key (configs/zhen-checks.yaml) that judges each draft, e.g. go-test, fixture-check")
	fs.StringVar(&o.checkPath, "check-path", ".", "path for -run-check, relative to the worktree")
	fs.StringVar(&o.checksPath, "checks", "configs/zhen-checks.yaml", "the run_check allowlist")
	fs.StringVar(&o.agentRoot, "agent-root", defaultAgentRoot(), "agent worktrees live under here (env ZHEN_AGENT_WT_ROOT)")
	fs.StringVar(&o.configPath, "config", "", "config file (default $XDG_CONFIG_HOME/unheaded/zhen-claude.conf; a missing default is fine)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	cfgPath, required := o.configPath, o.configPath != ""
	if !required {
		var err error
		if cfgPath, err = defaultConfigPath(); err != nil {
			return nil, fmt.Errorf("config: %w", err)
		}
	}
	cfg, err := loadConfig(cfgPath, required)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if !set["local-only"] {
		o.localOnly = cfg.LocalOnly
	}
	if o.localOnly {
		if set["route"] && o.route == routeClaude {
			return nil, errors.New("-local-only keeps the prompt on this host; -route claude would send it to Claude")
		}
		if o.repl {
			return nil, errors.New("-local-only works on one prompt, not -repl (the local model keeps no conversation)")
		}
		o.route = routeAuto
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
	switch o.route {
	case routeClaude, routeAuto, routeLocal:
	default:
		return nil, fmt.Errorf("-route %q: want claude, auto or local", o.route)
	}
	if err := validateRoutedFlags(o); err != nil {
		return nil, err
	}
	o.checkArgv = strings.Fields(o.check)
	if o.route != routeAuto && len(o.checkArgv) > 0 {
		return nil, errors.New("-check needs -route auto")
	}
	if o.route == routeAuto && o.repl {
		return nil, errors.New("-route auto works on one prompt, not -repl (the local model keeps no conversation)")
	}
	if o.localMaxTokens < 1 || o.localTimeout <= 0 || o.checkTimeout <= 0 {
		return nil, errors.New("-local-max-tokens, -local-timeout and -check-timeout must be positive")
	}
	if err := checkAuditEnv(o); err != nil {
		return nil, err
	}
	switch o.privacyMode {
	case "standard", "strict":
	default:
		return nil, fmt.Errorf("-privacy %q: want standard or strict", o.privacyMode)
	}
	switch o.confirm {
	case "deny", "tty":
	case "queue":
		if strings.TrimSpace(os.Getenv(envAppZhenDSN)) == "" {
			return nil, fmt.Errorf("-confirm queue needs %s (unheaded_app as app_zhen); it is read from the environment only", envAppZhenDSN)
		}
	default:
		return nil, fmt.Errorf("-confirm %q: want deny, tty or queue", o.confirm)
	}
	if o.confirm != "deny" && o.localOnly {
		return nil, errors.New("-confirm needs Claude's tool calls; -local-only never sends any")
	}
	return o, nil
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 1 && cmdline.IsVersionArg(args[0]) {
		cmdline.PrintVersion(stdout, cmdline.Spec{Name: "zhen-claude"})
		return 0
	}
	if len(args) > 0 && args[0] == "confirm" {
		return runConfirmCmd(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "review" {
		return runReviewCmd(args[1:], stdout, stderr)
	}
	o, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return cmdline.ReportUsageError(stderr, cmdline.Spec{Name: "zhen-claude"}, "%s", cmdline.Translate(err, args))
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(o.logLevel)); err != nil {
		_, _ = fmt.Fprintln(stderr, "zhen-claude: -log-level:", err)
		return 2
	}
	log := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: level})).With("service", "zhen-claude")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	capw, err := openCapture(o, log)
	if err != nil {
		log.Error("startup", "error", err.Error())
		return 1
	}
	tools := &toolCollector{on: capw != nil}

	var prompt string
	if !o.repl {
		if prompt = o.prompt; prompt == "" {
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
	}
	if o.dryRun && o.localOnly {
		return dryRunLocal(o, prompt, stdout, log)
	}
	if o.confirm == "tty" && !o.dryRun {
		tty, err := openTTY()
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "zhen-claude: -confirm tty needs a controlling terminal (/dev/tty): %v\n", err)
			return 2
		}
		o.tty = tty
		defer func() { _ = tty.Close() }()
	}
	aud := setupAudit(ctx, o, log, "zhen-claude")
	defer aud.close()
	if o.family != "" && !o.dryRun {
		return runRouted(ctx, o, prompt, stdout, stderr, log, capw, aud)
	}
	if o.route == routeAuto && !o.dryRun {
		return runAuto(ctx, o, prompt, stdout, stderr, log, capw, tools, aud)
	}

	b, closeAll, err := build(ctx, o, log, stdout, tools, aud)
	defer closeAll()
	if err != nil {
		log.Error("startup", "error", err.Error())
		return 1
	}

	if o.repl {
		return repl(ctx, b, stdin, stdout, stderr, log, capw, tools)
	}
	conv := b.NewConversation()
	if o.dryRun {
		if o.route == routeAuto {
			if code := dryRunLocal(o, prompt, stdout, log); code != 0 {
				return code
			}
		}
		return dryRun(conv, b, prompt, stdout, log)
	}
	t0 := time.Now()
	reply, err := turn(ctx, conv, prompt, stdout, stderr, log)
	captureClaudeTurn(capw, b, conv.ID(), prompt, reply, err, time.Since(t0), tools, log)
	if err != nil {
		return 1
	}
	return 0
}

// build wires providers, the gate, the cap and the client. closeAll is
// always safe to call.
func build(ctx context.Context, o *options, log *slog.Logger, stdout io.Writer, tools *toolCollector, aud *auditSetup) (*claudebridge.Bridge, func(), error) {
	if aud == nil {
		aud = &auditSetup{auditor: claudebridge.SlogAuditor{Log: log}}
	}
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
	confirmer := claudebridge.Confirmer(claudebridge.DenyConfirmer{})
	if !o.dryRun && !aud.readOnly {
		c, closeConfirmer, err := buildConfirmer(ctx, o, champ, aud)
		closers = append(closers, closeConfirmer)
		if err != nil {
			return nil, closeAll, err
		}
		confirmer = c
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
			var p zhentools.Provider = cp
			if aud.readOnly {
				p = claudebridge.AllowTools(cp, readOnlyChampionTools...)
			}
			providers = append(providers, claudebridge.NamedProvider{Name: "champion", Provider: p})
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
			if o.mcpMutating && !aud.readOnly {
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

	var filter *privacy.Filter
	if !o.dryRun {
		if filter, err = buildPrivacy(o, root); err != nil {
			return nil, closeAll, err
		}
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
		Auditor:          aud.auditor,
		Confirmer:        confirmer,
		Privacy:          filter,
		ProjectRoot:      root,
		Warn:             func(msg string) { log.Warn("privacy", "note", msg) },
		OnText:           func(s string) { _, _ = io.WriteString(stdout, s) },
	}
	if tools != nil && tools.on {
		// Content-bearing hook: only wired when -capture is on.
		cfg.OnToolResult = tools.add
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
		"credentials", b.CredentialSource(), "tools", b.ToolNames(), "privacy", o.privacyMode, "confirm", o.confirm,
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

// turn sends one prompt and reports the outcome.
func turn(ctx context.Context, conv *claudebridge.Conversation, prompt string, stdout, stderr io.Writer, log *slog.Logger) (*claudebridge.Reply, error) {
	reply, err := conv.Send(ctx, prompt)
	_, _ = io.WriteString(stdout, "\n")
	if reply != nil {
		log.Info("turn_done", "conversation", conv.ID(), "requests", reply.Requests, "cost_usd", reply.Cost.USD(), "served_by", reply.ServedBy, "stop_reason", reply.StopReason)
	}
	if err == nil {
		return reply, nil
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
	return reply, err
}

func repl(ctx context.Context, b *claudebridge.Bridge, stdin io.Reader, stdout, stderr io.Writer, log *slog.Logger, capw *zhencapture.Writer, tools *toolCollector) int {
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
		t0 := time.Now()
		reply, err := turn(ctx, conv, line, stdout, stderr, log)
		captureClaudeTurn(capw, b, conv.ID(), line, reply, err, time.Since(t0), tools, log)
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

func envString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
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

// buildPrivacy is the privacy tier for this run. A scanner that cannot be
// found is a startup error: there is no mode that sends unscanned text.
func buildPrivacy(o *options, root string) (*privacy.Filter, error) {
	r, err := privacy.NewRedactor(privacy.RedactorConfig{Scanner: o.secretScanner})
	if err != nil {
		return nil, fmt.Errorf("%w (nothing is sent to Claude without the secret scan; see -secret-scanner)", err)
	}
	var uds []string
	for _, p := range strings.Split(o.userDataRoots, ",") {
		if p = strings.TrimSpace(p); p != "" {
			uds = append(uds, p)
		}
	}
	local := &claudebridge.LocalLLM{BaseURL: o.localURL, Model: o.localModel, MaxTokens: privacy.DefaultSummaryTokens, Timeout: o.localTimeout}
	return &privacy.Filter{
		Redactor:   r,
		Classifier: privacy.Classifier{UserDataRoots: uds},
		Local:      claudebridge.LocalCompleter{L: local},
		Mode:       privacy.Mode(o.privacyMode),
	}, nil
}
