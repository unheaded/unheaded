// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Command lich-mcp is the Lich MCP server: it exposes the Lich red-team tools
// (pkg/lichtools) over MCP stdio, behind the Champion gate, confined to
// Stevie's own sandboxes and local harnesses. Scope is enforced server-side
// from configs/lich-mcp.yaml — never from a tool argument a model controls.
//
// Attack surface is stdio only: the server binds no TCP port. Start/stop is
// part of the kill switch. See docs/battle-plans/S-LICH-AWAKENS-battle-plan.md.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/cmdline"
	"unheaded/pkg/lichtools"
	"unheaded/pkg/mcpserver"
	"unheaded/pkg/zhentools"
)

const (
	serverName    = "lich-mcp"
	serverVersion = "0.1.0"
	// maxMessageBytes bounds a single JSON-RPC line; a longer line ends the
	// stream (it can no longer be trusted).
	maxMessageBytes = mcpserver.DefaultMaxMessageBytes
)

func main() {
	// Subcommands share the server's policy file but do not serve. `confirm`
	// is the out-of-band approval CLI (Phase 4); `drive` is the Claude driver
	// (Phase 6). Both are part of the one binary the kill switch owns.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "confirm":
			os.Exit(confirmMain(os.Args[2:]))
		case "drive":
			os.Exit(driveMain(os.Args[2:]))
		}
	}

	cfgPath := flag.String("config", "configs/lich-mcp.yaml", "path to the Lich MCP server policy (scope, limits, kill switch)")
	root := flag.String("project-root", "", "checkout root; defaults to $PWD")
	emittedBy := flag.String("emitted-by", "lich-mcp/stdio", "audit identity for the caller driving these tools")
	cmdline.Parse(cmdline.Spec{
		Name:     "lich-mcp",
		Purpose:  "Serve the Lich red-team tools over MCP stdio behind the Champion gate, confined to local sandboxes.",
		Commands: []cmdline.Command{{Name: "confirm", Purpose: "out-of-band approval CLI for gated tool calls"}, {Name: "drive", Purpose: "drive the server from Claude"}},
		Operands: "[COMMAND]",
		MaxArgs:  1,
		Examples: []string{"lich-mcp --config configs/lich-mcp.yaml", "lich-mcp confirm --help", "lich-mcp drive --help"},
	})
	if flag.NArg() > 0 {
		cmdline.UnknownCommand(cmdline.Spec{Name: "lich-mcp"}, flag.Arg(0))
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	projectRoot := *root
	if projectRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			logger.Error("resolve project root", "err", err)
			os.Exit(1)
		}
		projectRoot = wd
	}

	srv, err := newServer(*cfgPath, projectRoot, *emittedBy, logger)
	if err != nil {
		logger.Error("lich-mcp startup", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Watch the halt file; SIGTERM/SIGINT cancel ctx (below) and also trip.
	go srv.killSwitch.Watch(ctx)

	logger.Info("lich-mcp ready", "transport", "stdio", "config", *cfgPath, "scope_prefix", srv.provider.Scope().SandboxPrefix())
	err = srv.serve(ctx, os.Stdin, os.Stdout)
	// On any shutdown (clean EOF, signal, error), trip the kill switch so no
	// sandbox is left running.
	srv.killSwitch.Trip("server shutdown")
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("lich-mcp serve", "err", err)
		os.Exit(1)
	}
}

// server holds the gated tool surface. The provider, gate and scope are fixed
// after construction; a model's input never mutates them.
type server struct {
	provider   *lichtools.Provider
	gate       claudebridge.Gate
	auditor    claudebridge.Auditor
	killSwitch *lichtools.KillSwitch
	spend      *claudebridge.Ledger // daily $ cap hook for the Claude driver (later phase)
	emittedBy  string
	log        *slog.Logger
}

// SpendLedger exposes the daily spend cap so the Claude driver (a later phase)
// can reserve against it before each model call.
func (s *server) SpendLedger() *claudebridge.Ledger { return s.spend }

func newServer(cfgPath, projectRoot, emittedBy string, logger *slog.Logger) (*server, error) {
	cfg, err := lichtools.LoadConfig(cfgPath, projectRoot)
	if err != nil {
		return nil, err
	}
	provider, err := lichtools.NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	// A Champion with no ActionStore (nil) logs to its own slog path only; the
	// gate's refusal logic does not need The Well. ProjectRoot bounds its path
	// allowlist to the checkout.
	champ, err := champion.New(champion.Config{ProjectRoot: projectRoot, SessionID: "lich-mcp"}, nil)
	if err != nil {
		return nil, err
	}
	championGate, err := claudebridge.NewChampionGate(champ)
	if err != nil {
		return nil, err
	}
	// Out-of-band confirmation (Phase 4): a refused-pending mutating tool
	// (lich_file_finding) parks a ticket an operator approves with
	// `lich-mcp confirm approve <id>`. The approval binds to the exact call.
	store, err := lichtools.NewPendingStore(cfg.PendingDirAbs())
	if err != nil {
		return nil, err
	}
	var gate claudebridge.Gate = newConfirmGate(championGate, store, projectRoot, func(event, id, tool string) {
		logger.Info("lich confirm", "event", event, "ticket", id, "tool", tool)
	})

	// Confinement: the unprivileged netns sandbox (LXD is an [ESCALATE], not
	// used). The results tree is the only writable path; everything else is
	// read-only inside the sandbox. If confinement is unavailable, run_harness
	// fails closed — the server still serves the read-only tools.
	sem := lichtools.NewSemaphore(cfg.MaxLiveSandboxes)
	var sandbox lichtools.Sandbox
	if sb, serr := lichtools.NewNetnsSandbox(cfg.MemFitSafety, cfg.ResultsDirAbs()); serr != nil {
		logger.Warn("confinement unavailable; run_harness will fail closed", "err", serr)
	} else {
		sandbox = sb
	}
	ks := lichtools.NewKillSwitch(cfg.KillSwitchFile, lichtools.DefaultHaltGrace, sandbox, func(reason string) {
		logger.Warn("lich-mcp HALTED", "reason", reason)
	})
	provider.WithConfinement(sandbox, sem, ks)
	// The local-model tool (Phase 4): MINOR-weight drafting against the already-
	// running llama-server. Free, on-host; never starts or swaps a model.
	provider.WithLocalLLM(cfg.LocalLLMURL, cfg.LocalLLMModel, cfg.MaxDailyLocalGens)

	// Spend-cap hook (ADR-104): the daily dollar cap the Claude driver (a later
	// phase) reserves against before each model call. It is in-memory here (no
	// model calls happen in this server); the driver phase wires a persistent
	// ledger path. Local-model calls are free and skip it. Not reached in P0–P3.
	capNano, cerr := claudebridge.USDToNano(cfg.DailyCapUSD)
	if cerr != nil {
		return nil, fmt.Errorf("lich-mcp: daily cap: %w", cerr)
	}
	spend, serr := claudebridge.NewLedger("", capNano)
	if serr != nil {
		return nil, fmt.Errorf("lich-mcp: spend ledger: %w", serr)
	}

	return &server{
		provider:   provider,
		gate:       gate,
		auditor:    claudebridge.SlogAuditor{Log: logger},
		killSwitch: ks,
		spend:      spend,
		emittedBy:  emittedBy,
		log:        logger,
	}, nil
}

// --- MCP stdio transport (pkg/mcpserver) ---

// The wire shapes live in pkg/mcpserver, shared with cmd/zhen-mcp.
type (
	callToolResult = mcpserver.CallToolResult
	callToolParams = mcpserver.CallToolParams
)

// serve runs the newline-delimited JSON-RPC loop over r/w until EOF, a
// too-large line, or ctx cancellation.
func (s *server) serve(ctx context.Context, r io.Reader, w io.Writer) error {
	ms := &mcpserver.Server{
		Name:            serverName,
		Version:         serverVersion,
		Instructions:    "Lich red-team tools, confined to this operator's own sandboxes and local harnesses. Scope is enforced server-side; findings are never auto-published.",
		Handler:         lichHandler{s},
		MaxMessageBytes: maxMessageBytes,
	}
	return ms.Serve(ctx, r, w)
}

// lichHandler adapts the server to mcpserver.Handler.
type lichHandler struct{ s *server }

func (h lichHandler) ListTools(ctx context.Context) ([]mcpserver.Tool, error) {
	tools, err := h.s.provider.Tools(ctx)
	if err != nil {
		return nil, err
	}
	wire := make([]mcpserver.Tool, 0, len(tools))
	for _, t := range tools {
		wire = append(wire, mcpserver.Tool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return wire, nil
}

func (h lichHandler) CallTool(ctx context.Context, p mcpserver.CallToolParams) mcpserver.CallToolResult {
	return h.s.callTool(ctx, p)
}

// callTool runs one tool: scope pre-check (refuse off-allowlist BEFORE the
// gate), then the Champion gate, then the provider. Every call is audited.
func (s *server) callTool(ctx context.Context, p callToolParams) callToolResult {
	start := time.Now()
	args := p.Arguments
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	sum := sha256.Sum256(args)
	rec := claudebridge.ToolRecord{
		ToolUseID:   "",
		Name:        p.Name,
		Provider:    serverName,
		InputBytes:  len(args),
		InputSHA256: hex.EncodeToString(sum[:]),
	}

	// Scope pre-check, before the gate. A target that fails scope is refused
	// here and never reaches the gate or the provider.
	if target, ok, terr := s.provider.TargetFor(p.Name, args); terr != nil {
		rec.Decision, rec.Reason, rec.IsError = "bad_input", terr.Error(), true
		s.audit(ctx, rec, start)
		return errResult(terr.Error())
	} else if ok {
		if verr := s.provider.Scope().Validate(target); verr != nil {
			rec.Decision, rec.Reason, rec.IsError = "refused_scope", verr.Error(), true
			s.audit(ctx, rec, start)
			return errResult("out of scope (refused before gate): " + verr.Error())
		}
	}

	call := claudebridge.ToolCall{Name: p.Name, Input: args, Provider: serverName, EmittedBy: s.emittedBy}
	res, err := s.gate.Execute(ctx, call, s.provider)
	if err != nil {
		var ref *claudebridge.RefusedError
		if errors.As(err, &ref) {
			rec.Decision, rec.Reason, rec.Pending, rec.IsError = "refused", ref.Reason, ref.PendingConfirmation, true
			s.audit(ctx, rec, start)
			msg := "refused by gate: " + ref.Reason
			if ref.PendingConfirmation {
				msg += " (pending out-of-band confirmation)"
			}
			return errResult(msg)
		}
		rec.Decision, rec.Reason, rec.IsError = "error", err.Error(), true
		s.audit(ctx, rec, start)
		return errResult(err.Error())
	}

	rec.Decision, rec.IsError, rec.OutputBytes = "allowed", res.IsError, len(res.Text)
	if res.IsError {
		rec.Decision = "tool_error"
	}
	s.audit(ctx, rec, start)
	return callToolResult{Content: []mcpserver.ContentBlock{{Type: "text", Text: res.Text}}, IsError: res.IsError}
}

func (s *server) audit(ctx context.Context, rec claudebridge.ToolRecord, start time.Time) {
	rec.Duration = time.Since(start)
	s.auditor.ToolCall(ctx, rec)
}

func errResult(msg string) callToolResult {
	return mcpserver.ErrorResult(msg)
}

// compile-time assertion that the provider satisfies the gate's Provider type.
var _ zhentools.Provider = (*lichtools.Provider)(nil)
