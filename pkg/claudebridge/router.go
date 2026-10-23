// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The threshold router (S-TWO-BRAINS Phase 5): configs/zhen-routing.yaml,
// generated from the two-brains eval, says per task family whether the
// local brain answers alone, drafts first with Claude on failure, or the
// task goes to Claude directly. A sandboxed check judges drafts.

// Routes.
const (
	RouteLocalOnly    = "local_only"
	RouteLocalFirst   = "local_first"
	RouteClaudeDirect = "claude_direct"
)

// Escalation triggers.
var escalateKinds = map[string]bool{"check_fail": true, "parse_fail": true, "judge_fail": true, "refusal": true}

// MaxRouteClaudeUSD bounds any family's per-task Claude budget.
const MaxRouteClaudeUSD = 5.0

// FamilyRoute is one family's routing.
type FamilyRoute struct {
	Route         string   `yaml:"route"`
	LocalModel    string   `yaml:"local_model"`
	LocalAttempts int      `yaml:"local_attempts"`
	EscalateOn    []string `yaml:"escalate_on"`
	MaxClaudeUSD  float64  `yaml:"max_claude_usd"`
}

// RoutingConfig is the generated routing table.
type RoutingConfig struct {
	Version  int                    `yaml:"version"`
	Families map[string]FamilyRoute `yaml:"families"`
	Default  FamilyRoute            `yaml:"default"`
}

func (f FamilyRoute) validate(name string) error {
	switch f.Route {
	case RouteLocalOnly, RouteLocalFirst, RouteClaudeDirect:
	default:
		return fmt.Errorf("routing: %s: route %q", name, f.Route)
	}
	if f.LocalAttempts < 0 || f.LocalAttempts > 3 {
		return fmt.Errorf("routing: %s: local_attempts %d out of 0..3", name, f.LocalAttempts)
	}
	for _, k := range f.EscalateOn {
		if !escalateKinds[k] {
			return fmt.Errorf("routing: %s: escalate_on %q", name, k)
		}
	}
	if f.MaxClaudeUSD < 0 || f.MaxClaudeUSD > MaxRouteClaudeUSD {
		return fmt.Errorf("routing: %s: max_claude_usd %v out of 0..%v", name, f.MaxClaudeUSD, MaxRouteClaudeUSD)
	}
	return nil
}

// LoadRoutingConfig reads and validates configs/zhen-routing.yaml.
func LoadRoutingConfig(path string) (*RoutingConfig, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-chosen config file
	if err != nil {
		return nil, err
	}
	var c RoutingConfig
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("routing: %w", err)
	}
	if c.Version != 1 {
		return nil, fmt.Errorf("routing: version %d, want 1", c.Version)
	}
	for name, f := range c.Families {
		if err := f.validate(name); err != nil {
			return nil, err
		}
	}
	if c.Default.Route == "" {
		c.Default = FamilyRoute{Route: RouteClaudeDirect, MaxClaudeUSD: 0.25}
	}
	if err := c.Default.validate("default"); err != nil {
		return nil, err
	}
	return &c, nil
}

// For returns a family's routing, or the default.
func (c *RoutingConfig) For(family string) FamilyRoute {
	if c != nil {
		if f, ok := c.Families[family]; ok {
			return f
		}
		return c.Default
	}
	return FamilyRoute{Route: RouteClaudeDirect, MaxClaudeUSD: 0.25}
}

// FileBlock is one ```file:<path> block of an answer.
type FileBlock struct {
	Path    string
	Content string
}

// ErrNoFileBlocks: an answer that should carry files carries none.
var ErrNoFileBlocks = errors.New("answer has no ```file:<path> blocks")

var fileBlockRE = regexp.MustCompile("(?s)```file:([^\\n`]+)\\n(.*?)```")

// ParseFileBlocks extracts the file blocks of an answer.
func ParseFileBlocks(answer string) ([]FileBlock, error) {
	ms := fileBlockRE.FindAllStringSubmatch(answer, -1)
	if len(ms) == 0 {
		return nil, ErrNoFileBlocks
	}
	out := make([]FileBlock, 0, len(ms))
	for _, m := range ms {
		out = append(out, FileBlock{Path: strings.TrimSpace(m[1]), Content: m[2]})
	}
	return out, nil
}

// CheckOutcome is a check's verdict on the worktree.
type CheckOutcome struct {
	Pass   bool
	Output string
}

// ClaudeResult is the Claude leg's outcome.
type ClaudeResult struct {
	Text string
	Cost NanoUSD
}

// ThresholdRouter runs one task by its family's route (the ADR-053
// routing hook, made concrete by the eval's thresholds).
type ThresholdRouter struct {
	Config *RoutingConfig
	// Local asks the local model for a draft.
	Local func(ctx context.Context, prompt string) (LocalAnswer, error)
	// Apply writes a draft's file blocks into the worktree (nil: answers
	// are text, no files). ErrNoFileBlocks is a parse failure.
	Apply func(ctx context.Context, answer string) error
	// Check judges the worktree (nil: a usable local answer is accepted).
	Check func(ctx context.Context) (CheckOutcome, error)
	// Claude runs the Claude leg within budget, labelled with reason.
	Claude func(ctx context.Context, prompt, reason, family string, budget NanoUSD) (ClaudeResult, error)
	// Redact cleans check output before it goes into any prompt.
	Redact func(ctx context.Context, s string) (string, error)
	// Auditor records the local legs (the Claude leg is audited by its
	// bridge).
	Auditor Auditor
}

// RouteOutcome is what happened to one task.
type RouteOutcome struct {
	Family        string  `json:"family"`
	Route         string  `json:"route"`
	RouteReason   string  `json:"route_reason"`
	LocalAttempts int     `json:"local_attempts"`
	Escalated     bool    `json:"escalated"`
	Check         string  `json:"check"` // pass, fail, error, "" (no check)
	Cost          NanoUSD `json:"-"`
	CostUSD       string  `json:"cost_usd"`
	Stop          string  `json:"stop,omitempty"` // escalation_budget, not_escalated, local_only, no_answer
	Text          string  `json:"-"`
}

const maxCheckOutputInPrompt = 4 << 10

func (r *ThresholdRouter) redact(ctx context.Context, s string) string {
	if len(s) > maxCheckOutputInPrompt {
		s = s[:maxCheckOutputInPrompt] + "\n[truncated]"
	}
	if r.Redact == nil {
		return s
	}
	out, err := r.Redact(ctx, s)
	if err != nil {
		return "(check output withheld: the privacy filter could not run)"
	}
	return out
}

// Run routes one task.
func (r *ThresholdRouter) Run(ctx context.Context, family, task string) (RouteOutcome, error) {
	fr := r.Config.For(family)
	out := RouteOutcome{Family: family, Route: fr.Route}
	defer func() { out.CostUSD = out.Cost.USD() }()
	budget, err := USDToNano(fr.MaxClaudeUSD)
	if err != nil {
		return out, err
	}
	if fr.Route == RouteClaudeDirect {
		out.RouteReason = "claude_direct"
		return r.claudeLeg(ctx, out, task, "claude_direct", family, budget)
	}
	if r.Local == nil {
		return out, errors.New("router: no local model")
	}
	reason := "local_first"
	if fr.Route == RouteLocalOnly {
		reason = "local_only"
	}
	out.RouteReason = reason
	attempts := max(fr.LocalAttempts, 1)
	prompt := task
	var lastDraft, lastCheck, failKind string
	for a := 1; a <= attempts; a++ {
		out.LocalAttempts = a
		t0 := time.Now()
		ans, lerr := r.Local(ctx, prompt)
		r.auditLocal(ctx, family, reason, ans, lerr, time.Since(t0))
		if lerr != nil || strings.TrimSpace(ans.Text) == "" {
			failKind = "parse_fail"
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			continue
		}
		lastDraft = ans.Text
		if r.Check == nil {
			out.Text = ans.Text
			return out, nil
		}
		if r.Apply != nil {
			if err := r.Apply(ctx, ans.Text); err != nil {
				failKind = "parse_fail"
				prompt = task + "\n\nYour previous answer could not be applied (" + err.Error() + "). Reply with the complete changed files as ```file:<path> blocks."
				continue
			}
		}
		co, cerr := r.Check(ctx)
		if cerr != nil {
			out.Check = "error"
			return out, cerr
		}
		if co.Pass {
			out.Check, out.Text = "pass", ans.Text
			return out, nil
		}
		failKind = "check_fail"
		lastCheck = r.redact(ctx, co.Output)
		prompt = task + "\n\nYour previous answer did not pass the check. Check output:\n```\n" + lastCheck + "\n```\nReply again with the complete corrected files."
	}
	if r.Check != nil && failKind == "check_fail" {
		out.Check = "fail"
	}
	if fr.Route == RouteLocalOnly {
		out.Stop = "local_only"
		return out, nil
	}
	if !contains(fr.EscalateOn, failKind) {
		out.Stop = "not_escalated"
		if failKind == "parse_fail" && r.Check == nil {
			out.Stop = "no_answer"
		}
		return out, nil
	}
	reason = "escalated_" + failKind
	out.RouteReason = reason
	esc := "Task:\n" + task + "\n\nA local model drafted this first; it did not pass (" + failKind + ")."
	if lastDraft != "" {
		esc += "\n\nLocal draft:\n" + lastDraft
	}
	if lastCheck != "" {
		esc += "\n\nCheck output (redacted):\n```\n" + lastCheck + "\n```"
	}
	return r.claudeLeg(ctx, out, esc, reason, family, budget)
}

func (r *ThresholdRouter) claudeLeg(ctx context.Context, out RouteOutcome, prompt, reason, family string, budget NanoUSD) (RouteOutcome, error) {
	if r.Claude == nil {
		out.Stop = "not_escalated"
		return out, nil
	}
	out.Escalated = reason != "claude_direct"
	cr, err := r.Claude(ctx, prompt, reason, family, budget)
	out.Cost += cr.Cost
	out.Text = cr.Text
	if err != nil {
		if errors.Is(err, ErrSpendCap) {
			out.Stop = "escalation_budget"
			if r.Check != nil && out.Check == "" {
				out.Check = "fail"
			}
			return out, nil
		}
		return out, err
	}
	if r.Check == nil {
		return out, nil
	}
	co, cerr := r.Check(ctx)
	if cerr != nil {
		out.Check = "error"
		return out, cerr
	}
	out.Check = "fail"
	if co.Pass {
		out.Check = "pass"
	}
	return out, nil
}

func (r *ThresholdRouter) auditLocal(ctx context.Context, family, reason string, ans LocalAnswer, err error, d time.Duration) {
	if r.Auditor == nil {
		return
	}
	rec := RequestRecord{
		Conversation: NewRecordID(), Model: ans.Model, ServedBy: ans.Model, StopReason: ans.FinishReason,
		InputTokens: ans.InputTokens, OutputTokens: ans.OutputTokens, Duration: d, Sent: true,
		Backend: BackendLocal, TaskFamily: family, RouteReason: reason,
	}
	if rec.Model == "" {
		rec.Model = "local"
	}
	if err != nil {
		rec.Err = err.Error()
	}
	r.Auditor.Request(ctx, rec)
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
