// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/anthropics/anthropic-sdk-go/option"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/claudebridge/privacy"
	"unheaded/pkg/lichtools"
)

// driveMain is `lich-mcp drive` — the Claude driver (S-LICH-AWAKENS Phase 6).
// It runs one prompt through a claudebridge.Bridge whose tools ARE the Lich
// tools, each still behind the Champion gate and the out-of-band confirmation
// flow. Model is picked by weight: Haiku for MINOR-weight volume drafting, Opus
// for HEAVY triage and finding synthesis. A hard daily dollar cap holds across
// runs; -dry-run (the default) prints the request without sending; the
// external-data (privacy) filter is applied to everything that would go to
// Claude. No prompt or response text is ever logged — only sizes and hashes.
const lichDriveSystem = `You are a red-team assistant driving the Lich tools on Stevie Bellis's own Unheaded project (GPL-3.0). Every target is one of Stevie's own local fuzz harnesses or sandboxes — never a third-party or network target.
Your tools are gated by the Champion. Reading campaigns, crashes and running a confined harness is allowed. Filing a finding to the register is MUTATING: it is refused pending an out-of-band human approval and comes back as a tool error that names a confirmation ticket. When a call is refused, do NOT rephrase it, rename it, or try another tool to achieve the same write — say what you wanted to do and that it needs the operator's confirmation. You are not a trusted caller; you cannot approve your own writes.
Findings must be complete: include the dedup key and the repro input SHA-256 so a human reviews a whole artifact. Answer plainly and briefly.`

type driveOpts struct {
	cfgPath     string
	root        string
	route       string
	model       string
	haiku       string
	opus        string
	capUSD      float64
	dryRun      bool
	effort      string
	maxTokens   int64
	ledgerPath  string
	prompt      string
	emittedBy   string
	privacyMode string
	scanner     string
	baseURL     string // test/proxy hook; empty uses the real API
}

func driveMain(args []string) int { return runDrive(args, os.Stdout, os.Stderr) }

func runDrive(args []string, stdout, stderr io.Writer) int {
	var o driveOpts
	fs := flag.NewFlagSet("lich-mcp drive", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.cfgPath, "config", "configs/lich-mcp.yaml", "path to the Lich MCP server policy")
	fs.StringVar(&o.root, "project-root", "", "checkout root; defaults to $PWD")
	fs.StringVar(&o.route, "route", "auto", "route: auto|heavy|minor (auto = classify by the prompt)")
	fs.StringVar(&o.model, "model", "", "override the model (else chosen by weight)")
	fs.StringVar(&o.haiku, "haiku", "claude-haiku-5-5", "volume model for MINOR-weight work")
	fs.StringVar(&o.opus, "opus", "claude-opus-5-5", "triage model for HEAVY-weight work")
	fs.Float64Var(&o.capUSD, "daily-cap-usd", 3.0, "hard daily spend cap (USD)")
	fs.BoolVar(&o.dryRun, "dry-run", true, "print the request without sending (default on; -dry-run=false to send)")
	fs.StringVar(&o.effort, "effort", "", "override effort (low|medium|high|xhigh|max); default low for Haiku, medium for Opus")
	fs.Int64Var(&o.maxTokens, "max-tokens", claudebridge.DefaultMaxTokens, "max output tokens per request")
	fs.StringVar(&o.ledgerPath, "ledger", "", "spend ledger path; empty uses the default state path")
	fs.StringVar(&o.prompt, "p", "", "the task prompt")
	fs.StringVar(&o.emittedBy, "emitted-by", "lich-drive", "audit identity")
	fs.StringVar(&o.privacyMode, "privacy", "strict", "privacy mode: standard|strict (strict withholds all ops data; the default for unattended runs)")
	fs.StringVar(&o.scanner, "secret-scanner", "", "secret scanner binary (default: gitleaks on PATH)")
	fs.StringVar(&o.baseURL, "base-url", "", "override the API base URL (proxy/testing)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: lich-mcp drive [flags] -p <prompt>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if o.prompt == "" {
		fmt.Fprintln(stderr, "drive: -p <prompt> is required")
		return 2
	}

	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	code, err := drive(context.Background(), o, stdout, logger)
	if err != nil {
		fmt.Fprintf(stderr, "drive: %v\n", err)
	}
	return code
}

func drive(ctx context.Context, o driveOpts, stdout io.Writer, logger *slog.Logger) (int, error) {
	projectRoot := o.root
	if projectRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return 1, err
		}
		projectRoot = wd
	}
	cfg, err := lichtools.LoadConfig(o.cfgPath, projectRoot)
	if err != nil {
		return 1, err
	}

	// Build the Lich provider with its confinement, local tool, and the
	// confirm store for the out-of-band flow.
	provider, err := lichtools.NewProvider(cfg)
	if err != nil {
		return 1, err
	}
	provider.WithLocalLLM(cfg.LocalLLMURL, cfg.LocalLLMModel, cfg.MaxDailyLocalGens)
	store, err := lichtools.NewPendingStore(cfg.PendingDirAbs())
	if err != nil {
		return 1, err
	}

	// Classify the weight and pick the model + effort.
	weight, err := routeWeight(ctx, o)
	if err != nil {
		return 2, err
	}
	model := o.model
	if model == "" {
		model = claudebridge.ModelForWeight(weight, o.haiku, o.opus)
	}
	effort := o.effort
	if effort == "" {
		if weight == claudebridge.WeightHeavy {
			effort = "medium"
		} else {
			effort = "low"
		}
	}

	// The gate: Champion gate wrapped by the out-of-band confirmation flow.
	gate, err := buildDriveGate(store, projectRoot, logger)
	if err != nil {
		return 1, err
	}

	capNano, err := claudebridge.USDToNano(o.capUSD)
	if err != nil {
		return 1, err
	}
	ledgerPath := o.ledgerPath
	if ledgerPath == "" && !o.dryRun {
		if p, derr := claudebridge.DefaultLedgerPath(); derr == nil {
			ledgerPath = p
		}
	}
	ledger, err := claudebridge.NewLedger(ledgerPath, capNano)
	if err != nil {
		return 1, err
	}
	prices, err := claudebridge.DefaultPrices()
	if err != nil {
		return 1, err
	}

	bcfg := claudebridge.Config{
		Model:       model,
		Effort:      effort,
		MaxTokens:   o.maxTokens,
		System:      lichDriveSystem,
		Providers:   []claudebridge.NamedProvider{{Name: "lich", Provider: provider}},
		Gate:        gate,
		Prices:      prices,
		Ledger:      ledger,
		Auditor:     claudebridge.SlogAuditor{Log: logger},
		ProjectRoot: projectRoot,
	}
	if o.baseURL != "" {
		bcfg.ClientOptions = []option.RequestOption{option.WithBaseURL(o.baseURL), option.WithAPIKey("test-key")}
		bcfg.Credentials = func() (string, error) { return "test(base-url)", nil }
	}

	// The external-data filter: applied to the prompt and every tool result
	// before anything reaches Claude. Required for a live send (heavy route);
	// in dry-run we never send, so a missing scanner must not block the preview.
	filter, ferr := buildDrivePrivacy(o, projectRoot, cfg)
	if ferr != nil {
		if !o.dryRun {
			return 1, fmt.Errorf("external-data filter: %w (nothing is sent to Claude without it)", ferr)
		}
		logger.Warn("external-data filter unavailable; dry-run only", "err", ferr.Error())
	} else {
		bcfg.Privacy = filter
	}

	logger.Info("drive", "route", o.route, "weight", string(weight), "model", model, "effort", effort, "dry_run", o.dryRun, "cap_usd", o.capUSD)

	if o.dryRun {
		if bcfg.Credentials == nil {
			bcfg.Credentials = func() (string, error) { return "dry-run (no send)", nil }
		}
		b, err := claudebridge.New(ctx, bcfg)
		if err != nil {
			return 1, err
		}
		body, err := b.NewConversation().RequestJSON(o.prompt)
		if err != nil {
			return 1, err
		}
		fmt.Fprintf(stdout, "# DRY RUN — not sent. route=%s weight=%s model=%s effort=%s tools=%v\n", o.route, weight, model, effort, b.ToolNames())
		_, _ = stdout.Write(body)
		_, _ = io.WriteString(stdout, "\n")
		return 0, nil
	}

	b, err := claudebridge.New(ctx, bcfg)
	if err != nil {
		return 1, err
	}
	conv := b.NewConversation()
	conv.SetRoute(string(weight), "lich-"+o.route)
	reply, err := conv.Send(ctx, o.prompt)
	if reply != nil {
		logger.Info("drive_reply", "stop_reason", reply.StopReason, "served_by", reply.ServedBy, "requests", reply.Requests, "cost_usd", reply.Cost.USD())
	}
	if err != nil {
		return 1, err
	}
	_, _ = io.WriteString(stdout, reply.Text)
	_, _ = io.WriteString(stdout, "\n")
	return 0, nil
}

// routeWeight resolves the -route flag to a weight (auto classifies the prompt).
func routeWeight(ctx context.Context, o driveOpts) (claudebridge.Weight, error) {
	switch o.route {
	case "heavy":
		return claudebridge.WeightHeavy, nil
	case "minor":
		return claudebridge.WeightMinor, nil
	case "auto", "":
		return claudebridge.AutoRouter{}.Route(ctx, claudebridge.Task{Prompt: o.prompt})
	default:
		return "", fmt.Errorf("unknown -route %q (want auto|heavy|minor)", o.route)
	}
}

// buildDriveGate returns the Champion gate wrapped by the confirm flow.
func buildDriveGate(store *lichtools.PendingStore, projectRoot string, logger *slog.Logger) (claudebridge.Gate, error) {
	champ, err := champion.New(champion.Config{ProjectRoot: projectRoot, SessionID: "lich-drive"}, nil)
	if err != nil {
		return nil, err
	}
	cg, err := claudebridge.NewChampionGate(champ)
	if err != nil {
		return nil, err
	}
	gate := newConfirmGate(cg, store, projectRoot, func(event, id, tool string) {
		logger.Info("lich confirm", "event", event, "ticket", id, "tool", tool)
	})
	return gate, nil
}

// buildDrivePrivacy builds the external-data filter (strict by default). A
// missing scanner is an error the caller treats as fatal for a live send.
func buildDrivePrivacy(o driveOpts, projectRoot string, cfg *lichtools.Config) (*privacy.Filter, error) {
	r, err := privacy.NewRedactor(privacy.RedactorConfig{Scanner: o.scanner})
	if err != nil {
		return nil, err
	}
	local := &claudebridge.LocalLLM{BaseURL: cfg.LocalLLMURL, Model: cfg.LocalLLMModel, MaxTokens: privacy.DefaultSummaryTokens}
	return &privacy.Filter{
		Redactor:   r,
		Classifier: privacy.Classifier{},
		Local:      claudebridge.LocalCompleter{L: local},
		Mode:       privacy.Mode(o.privacyMode),
	}, nil
}
