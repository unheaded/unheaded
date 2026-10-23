// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/zhentools"
)

// Tool names. These are the stable identifiers a driver sees and the Champion
// gate classifies (see pkg/champion/toolcall.go lich block).
const (
	ToolListCampaigns = "lich_list_campaigns"
	ToolShowCampaign  = "lich_show_campaign"
	ToolRunHarness    = "lich_run_harness"
	ToolReadCrash     = "lich_read_crash"
	ToolTriage        = "lich_triage"
	ToolFileFinding   = "lich_file_finding"
)

// Provider exposes the Lich tools as a zhentools.Provider. Every target-bearing
// tool validates against the server-side Scope before doing anything; the Kind
// is fixed by the tool, not by the model. Safe for concurrent use: the Scope
// and Config are read-only after construction and the Phase-1 bodies are stubs.
type Provider struct {
	scope *Scope
	cfg   *Config

	// Confinement (wired by WithConfinement). When sandbox is nil, run_harness
	// fails closed rather than running anything unconfined.
	sandbox Sandbox
	sem     *Semaphore
	ks      *KillSwitch

	// Daily compute budget for fuzz runs (a control parallel to the dollar cap).
	budgetMu   sync.Mutex
	budgetDay  string
	budgetUsed int

	// goEnv is the resolved, read-only go environment for offline in-sandbox
	// builds (GOMODCACHE/GOPATH); lazily populated once.
	goEnvMu   sync.Mutex
	goEnvDone bool
	goModDir  string
	goPathDir string

	// local is the already-running llama-server client for the lich_local_llm
	// tool and the local-model driver (Phase 5). It asks; it never starts or
	// swaps a model. nil disables the tool.
	local *localClient

	// localGen counts local-model generations against the daily compute budget
	// (a control parallel to the dollar cap; free of dollar cost).
	genMu   sync.Mutex
	genDay  string
	genUsed int
	genMax  int
}

// NewProvider builds a Provider from a loaded Config.
func NewProvider(cfg *Config) (*Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("lichtools: NewProvider needs a config")
	}
	s, err := ScopeFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Provider{scope: s, cfg: cfg}, nil
}

// Scope exposes the server-side allowlist (read-only) so the MCP server can run
// a pre-gate scope check — refusing off-allowlist targets before the gate.
func (p *Provider) Scope() *Scope { return p.scope }

// Config exposes the server-side policy (read-only).
func (p *Provider) Config() *Config { return p.cfg }

// WithConfinement wires the sandbox substrate, the host-resource semaphore, and
// the kill switch into run_harness. Any may be nil; a nil sandbox makes
// run_harness fail closed.
func (p *Provider) WithConfinement(sb Sandbox, sem *Semaphore, ks *KillSwitch) *Provider {
	p.sandbox = sb
	p.sem = sem
	p.ks = ks
	return p
}

// TargetFor returns the scope Target a tool acts on, if any, so the server can
// validate scope before the gate. The Kind is chosen here, never from input.
func (p *Provider) TargetFor(name string, input json.RawMessage) (Target, bool, error) {
	switch name {
	case ToolRunHarness:
		var in runHarnessInput
		if err := decodeInput(input, &in); err != nil {
			return Target{}, false, err
		}
		return Target{Kind: KindHarness, Name: in.HarnessID}, true, nil
	default:
		return Target{}, false, nil
	}
}

// Tools implements zhentools.Provider.
func (p *Provider) Tools(_ context.Context) ([]zhentools.Tool, error) {
	tools := []zhentools.Tool{
		{
			Name:        ToolListCampaigns,
			Description: "List the Lich red-team campaigns from tomb/lich/ with id, target, status, verdict, and reference. Read-only.",
			InputSchema: schemaNoArgs,
		},
		{
			Name:        ToolShowCampaign,
			Description: "Show one Lich campaign (README, exit-criteria, status, results summary) by LICH-NNN id. Read-only.",
			InputSchema: schemaShowCampaign,
		},
		{
			Name:        ToolRunHarness,
			Description: "Run one local fuzz harness by three-digit id inside a confined, no-egress sandbox for a server-clamped duration. Confined and bounded; never a network or host target.",
			InputSchema: schemaRunHarness,
		},
		{
			Name:        ToolReadCrash,
			Description: "Read a crash artifact (crashing input hex+size+SHA-256, failing assertion, harness) from a results dir. Read-only; never executes the input.",
			InputSchema: schemaReadCrash,
		},
		{
			Name:        ToolTriage,
			Description: "Triage a results dir via crash-triage.sh, returning severity and a dedup key. Read-only analysis.",
			InputSchema: schemaTriage,
		},
		{
			Name:        ToolFileFinding,
			Description: "File a finding to the ADR-098 register (compliance/findings/register.yaml). Mutating: requires out-of-band confirmation; never auto-publishes.",
			InputSchema: schemaFileFinding,
		},
	}
	// The local-model tool is offered only when an endpoint is wired. It is
	// free (local) and read-only to the gate; used for MINOR-weight drafting.
	if p.local != nil {
		tools = append(tools, zhentools.Tool{
			Name:        ToolLocalLLM,
			Description: "Ask the already-running local llama-server one question (seed/mutation drafting, MINOR-weight work). Free, on-host; never starts or swaps a model. Read-only.",
			InputSchema: schemaLocalLLM,
		})
	}
	return tools, nil
}

// Call implements zhentools.Provider. Phase 1: bodies are stubs that prove the
// shape and the scope refusal. Real behaviour lands in Phases 2-3.
func (p *Provider) Call(ctx context.Context, name string, input json.RawMessage) (zhentools.Result, error) {
	switch name {
	case ToolListCampaigns:
		var in struct{}
		if err := decodeInput(input, &in); err != nil {
			return badInput(err)
		}
		return p.listCampaigns()

	case ToolShowCampaign:
		var in showCampaignInput
		if err := decodeInput(input, &in); err != nil {
			return badInput(err)
		}
		if in.CampaignID == "" {
			return badInput(fmt.Errorf("campaign_id is required"))
		}
		return p.showCampaign(in.CampaignID)

	case ToolRunHarness:
		var in runHarnessInput
		if err := decodeInput(input, &in); err != nil {
			return badInput(err)
		}
		// Scope FIRST — refuse off-allowlist before anything else.
		if err := p.scope.Validate(Target{Kind: KindHarness, Name: in.HarnessID}); err != nil {
			return refused(err), nil
		}
		return p.runHarness(ctx, in)

	case ToolReadCrash:
		var in readCrashInput
		if err := decodeInput(input, &in); err != nil {
			return badInput(err)
		}
		return p.readCrash(in)

	case ToolTriage:
		var in triageInput
		if err := decodeInput(input, &in); err != nil {
			return badInput(err)
		}
		return p.triage(ctx, in)

	case ToolFileFinding:
		var in fileFindingInput
		if err := decodeInput(input, &in); err != nil {
			return badInput(err)
		}
		return p.fileFinding(in)

	case ToolLocalLLM:
		var in localLLMInput
		if err := decodeInput(input, &in); err != nil {
			return badInput(err)
		}
		return p.localLLM(ctx, in)

	default:
		return zhentools.Result{IsError: true, Text: fmt.Sprintf("lichtools: unknown tool %q", name)}, nil
	}
}

// runHarness runs a harness inside the confined sandbox. Phase 2 proves the
// MCP-tool → sandbox path with a trivial no-op; Phase 3 (Step 81) swaps the
// no-op for the real lich-runner.sh invocation. It fails closed if no
// confinement substrate is wired or the kill switch has tripped, and respects
// the host-resource cap.
func (p *Provider) runHarness(ctx context.Context, in runHarnessInput) (zhentools.Result, error) {
	if p.sandbox == nil {
		return zhentools.Result{
			IsError: true,
			Text:    "lich_run_harness: scope OK, but no confinement substrate is available (fail-closed; no unconfined execution)",
		}, nil
	}
	if p.ks != nil && p.ks.Tripped() {
		return zhentools.Result{IsError: true, Text: ErrHalted.Error()}, nil
	}
	if p.sem != nil {
		release, err := p.sem.TryAcquire()
		if err != nil {
			return zhentools.Result{IsError: true, Text: err.Error()}, nil
		}
		defer release()
	}

	target := Target{Kind: KindHarness, Name: in.HarnessID}
	// Defense in depth: Call already validated scope, but re-check at the point
	// of execution so this method is safe for any caller.
	if err := p.scope.Validate(target); err != nil {
		return refused(err), nil
	}

	// Clamp the model-requested duration into [min,max] server-side — the model
	// cannot ask for an unbounded run — and charge it against the daily compute
	// budget.
	dur := p.clampDuration(in.DurationSeconds)
	if err := p.reserveFuzzBudget(dur); err != nil {
		return zhentools.Result{IsError: true, Text: err.Error()}, nil
	}

	// The sandbox wall timeout covers the fuzz duration plus a compile/report
	// overhead budget; a run that hits it is killed (bounded by design).
	lim := p.cfg.Limits
	lim.WallTimeout = time.Duration(dur+harnessOverheadSeconds) * time.Second
	lim.ASBytes = -1 // skip RLIMIT_AS: the Go toolchain needs a large virtual arena
	h, err := p.sandbox.Spawn(ctx, target, lim)
	if err != nil {
		return zhentools.Result{IsError: true, Text: "lich_run_harness: " + err.Error()}, nil
	}
	h.WorkDir = p.cfg.ProjectRoot
	if p.ks != nil {
		p.ks.Register(h)
	}
	defer func() {
		if p.ks != nil {
			p.ks.Unregister(h)
		}
		_ = p.sandbox.Destroy(h)
	}()

	argv := p.harnessArgv(in.HarnessID, dur)
	res, err := p.sandbox.Exec(ctx, h, argv)
	if err != nil {
		return zhentools.Result{IsError: true, Text: "lich_run_harness confined exec: " + err.Error()}, nil
	}
	out := tailLines(res.Stdout, 40)
	text := fmt.Sprintf("ran harness %s for %ds in confined no-egress sandbox %s (exit=%d timed_out=%v truncated=%v)\n--- output ---\n%s",
		in.HarnessID, dur, h.ID, res.ExitCode, res.TimedOut, res.Truncated, out)
	if res.ExitCode != 0 && strings.TrimSpace(res.Stderr) != "" {
		text += "\n--- stderr ---\n" + tailLines(res.Stderr, 20)
	}
	// A fuzz run that hit the wall timeout is normal (bounded by design), not a
	// tool error; only a start/confinement failure is.
	return zhentools.Result{Text: text}, nil
}

// --- input types (additionalProperties:false enforced by decodeInput) ---

type showCampaignInput struct {
	CampaignID string `json:"campaign_id"`
}

type runHarnessInput struct {
	CampaignID      string `json:"campaign_id"`
	HarnessID       string `json:"harness_id"`
	DurationSeconds int    `json:"duration_seconds"`
}

type readCrashInput struct {
	ResultsDir string `json:"results_dir"`
	CrashID    string `json:"crash_id"`
}

type triageInput struct {
	ResultsDir string `json:"results_dir"`
}

type fileFindingInput struct {
	Severity    string `json:"severity"`
	Class       string `json:"class"`
	Host        string `json:"host"`
	Summary     string `json:"summary"`
	Remediation string `json:"remediation"`
	Plan        string `json:"plan"`
	DedupKey    string `json:"dedup_key"`
	ReproSHA256 string `json:"repro_sha256"`
	HarnessID   string `json:"harness_id"`
}

// decodeInput unmarshals a JSON object into v, rejecting unknown fields
// (additionalProperties:false) and a non-object/empty input. A nil/empty input
// decodes as an all-zero struct so no-arg tools accept {}.
func decodeInput(input json.RawMessage, v any) error {
	if len(bytes.TrimSpace(input)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid tool input: %w", err)
	}
	return nil
}

func refused(err error) zhentools.Result {
	return zhentools.Result{IsError: true, Text: err.Error()}
}

func badInput(err error) (zhentools.Result, error) {
	return zhentools.Result{IsError: true, Text: err.Error()}, nil
}

// --- JSON Schemas (passed to the model verbatim) ---

var (
	schemaNoArgs = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)

	schemaShowCampaign = json.RawMessage(`{"type":"object","properties":{` +
		`"campaign_id":{"type":"string","description":"LICH-NNN campaign id"}},` +
		`"required":["campaign_id"],"additionalProperties":false}`)

	schemaRunHarness = json.RawMessage(`{"type":"object","properties":{` +
		`"campaign_id":{"type":"string","description":"LICH-NNN campaign id"},` +
		`"harness_id":{"type":"string","description":"three-digit harness id, e.g. 001"},` +
		`"duration_seconds":{"type":"integer","minimum":1,"description":"requested fuzz duration; server clamps to [min,max]"}},` +
		`"required":["harness_id"],"additionalProperties":false}`)

	schemaReadCrash = json.RawMessage(`{"type":"object","properties":{` +
		`"results_dir":{"type":"string","description":"a results dir under the configured results tree"},` +
		`"crash_id":{"type":"string","description":"crash artifact id within that dir"}},` +
		`"required":["results_dir","crash_id"],"additionalProperties":false}`)

	schemaTriage = json.RawMessage(`{"type":"object","properties":{` +
		`"results_dir":{"type":"string","description":"a results dir under the configured results tree"}},` +
		`"required":["results_dir"],"additionalProperties":false}`)

	schemaFileFinding = json.RawMessage(`{"type":"object","properties":{` +
		`"severity":{"type":"string","enum":["critical","high","medium","low","info"]},` +
		`"class":{"type":"string"},"host":{"type":"string"},` +
		`"summary":{"type":"string"},"remediation":{"type":"string"},"plan":{"type":"string"},` +
		`"dedup_key":{"type":"string"},"repro_sha256":{"type":"string"},"harness_id":{"type":"string"}},` +
		`"required":["severity","summary"],"additionalProperties":false}`)
)
