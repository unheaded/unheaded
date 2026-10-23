// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package lichtools exposes the Lich red-team framework (tomb/lich/) as a set
// of tools a model may drive through the Champion gate, confined to Stevie's
// own sandboxes and local harnesses. Scope is enforced here, server-side, from
// a config file — never from a tool argument the model controls. A jailbroken
// driver cannot widen it.
//
// See docs/battle-plans/S-LICH-AWAKENS-battle-plan.md and ADR-062/063/098.
package lichtools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Default confinement limits — deliberately conservative. A target with no
// explicit limit gets these (never unbounded).
const (
	DefaultCPUQuota       = 1.0              // 1 CPU
	DefaultMemBytes       = 512 << 20        // 512 MiB
	DefaultPidsMax        = 256              // fork-bomb ceiling
	DefaultWallTimeout    = 60 * time.Second // per-exec wall clock
	DefaultMaxLiveSandbox = 2                // host-resource cap
	DefaultMinFuzzSeconds = 5
	DefaultMaxFuzzSeconds = 300 // server clamps a driver's requested duration into [min,max]
	// DefaultMaxDailyFuzzSeconds caps total fuzz seconds a driver can spend per
	// day — a compute-budget control parallel to the dollar cap.
	DefaultMaxDailyFuzzSeconds = 3600
	DefaultMemFitSafety        = 1.2 // require free RAM >= MemBytes*safety before Spawn
	DefaultDailyCapUSD         = 5.0
	DefaultSandboxPrefix       = "lich-sbx-"
	DefaultHarnessDir          = "tomb/lich/harnesses"
	DefaultKillSwitchFile      = "/run/lich-mcp.halt"
	DefaultNetNS               = "lich-noegress"
	DefaultResultsDir          = "tomb/lich/results"
	DefaultRegisterRelPath     = "compliance/findings/register.yaml"
	DefaultPendingDirRelPath   = "var/lich/pending"
	DefaultSeedsDir            = "tomb/lich/seeds"
	// DefaultLocalLLMURL is the already-running llama-server (OpenAI-compatible);
	// the local driver asks it and never starts or swaps a model.
	DefaultLocalLLMURL = "http://127.0.0.1:8081"
)

// ErrSecretInConfig is returned when the config references a credential. Per
// CLAUDE.md, credentials are NEVER in the repo or its config — fail loud.
var ErrSecretInConfig = errors.New("lichtools: config references a credential; credentials are never stored in config (set via environment)")

// Limits are the per-sandbox resource bounds.
type Limits struct {
	CPUQuota    float64       `yaml:"cpu"`
	MemBytes    int64         `yaml:"mem_bytes"`
	PidsMax     int           `yaml:"pids"`
	WallTimeout time.Duration `yaml:"-"`
	WallSeconds int           `yaml:"timeout_seconds"`
	// ASBytes is the RLIMIT_AS (virtual address space) cap. >0 enforces it;
	// <=0 skips it. It defaults to MemBytes, but run_harness skips it because
	// the Go runtime reserves a large virtual arena that a tight AS cap kills.
	ASBytes int64 `yaml:"-"`
}

// Config is the server-side policy for the Lich MCP server. Everything that
// bounds what a driver can do lives here and is read at startup; a tool
// argument can never override it.
type Config struct {
	// ProjectRoot is the checkout root; harness/results/register paths resolve
	// against it. Set by LoadConfig, not from the YAML.
	ProjectRoot string `yaml:"-"`

	Scope struct {
		SandboxPrefix string `yaml:"sandbox_prefix"`
		HarnessDir    string `yaml:"harness_dir"`
		ResultsDir    string `yaml:"results_dir"`
		SeedsDir      string `yaml:"seeds_dir"`
	} `yaml:"scope"`

	// PendingDir holds out-of-band confirmation tickets (Phase 4). It is the
	// operator's own 0700 directory; a tool argument can never point elsewhere.
	PendingDir string `yaml:"pending_dir"`

	// LocalLLMURL is the already-running llama-server the local_llm tool and the
	// local-model driver (Phase 5) ask. It is never started or swapped here.
	LocalLLMURL   string `yaml:"local_llm_url"`
	LocalLLMModel string `yaml:"local_llm_model"`

	Limits Limits `yaml:"limits"`

	NetNS               string `yaml:"netns"`
	KillSwitchFile      string `yaml:"kill_switch_file"`
	MaxLiveSandboxes    int    `yaml:"max_live_sandboxes"`
	MinFuzzSeconds      int    `yaml:"min_fuzz_seconds"`
	MaxFuzzSeconds      int    `yaml:"max_fuzz_seconds"`
	MaxDailyFuzzSeconds int    `yaml:"max_daily_fuzz_seconds"`

	MemFitSafety float64 `yaml:"mem_fit_safety"`
	DailyCapUSD  float64 `yaml:"daily_cap_usd"`

	Findings struct {
		AutoPublish bool   `yaml:"auto_publish"`
		Register    string `yaml:"register"`
	} `yaml:"findings"`

	// APIKey / Secret / Token: tripwires. If any is set in the file, fail loud.
	APIKey string `yaml:"api_key"`
	Secret string `yaml:"secret"`
	Token  string `yaml:"token"`
}

// LoadConfig reads the YAML policy at path and resolves it against projectRoot.
// Unset fields get conservative defaults. A credential field is a hard error.
func LoadConfig(path, projectRoot string) (*Config, error) {
	if projectRoot == "" {
		return nil, errors.New("lichtools: LoadConfig needs a project root")
	}
	b, err := os.ReadFile(path) // #nosec G304 -- operator-supplied config path
	if err != nil {
		return nil, fmt.Errorf("lichtools: read config %s: %w", path, err)
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true) // additionalProperties:false — reject unknown keys
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("lichtools: parse config %s: %w", path, err)
	}
	c.ProjectRoot = projectRoot
	if err := c.applyDefaultsAndValidate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaultsAndValidate() error {
	if c.APIKey != "" || c.Secret != "" || c.Token != "" {
		return ErrSecretInConfig
	}
	if c.Scope.SandboxPrefix == "" {
		c.Scope.SandboxPrefix = DefaultSandboxPrefix
	}
	if c.Scope.HarnessDir == "" {
		c.Scope.HarnessDir = DefaultHarnessDir
	}
	if c.Scope.ResultsDir == "" {
		c.Scope.ResultsDir = DefaultResultsDir
	}
	if c.Scope.SeedsDir == "" {
		c.Scope.SeedsDir = DefaultSeedsDir
	}
	if c.PendingDir == "" {
		c.PendingDir = DefaultPendingDirRelPath
	}
	if c.LocalLLMURL == "" {
		c.LocalLLMURL = DefaultLocalLLMURL
	}
	if c.NetNS == "" {
		c.NetNS = DefaultNetNS
	}
	if c.KillSwitchFile == "" {
		c.KillSwitchFile = DefaultKillSwitchFile
	}
	if c.MaxLiveSandboxes <= 0 {
		c.MaxLiveSandboxes = DefaultMaxLiveSandbox
	}
	if c.MinFuzzSeconds <= 0 {
		c.MinFuzzSeconds = DefaultMinFuzzSeconds
	}
	if c.MaxFuzzSeconds <= 0 {
		c.MaxFuzzSeconds = DefaultMaxFuzzSeconds
	}
	if c.MinFuzzSeconds > c.MaxFuzzSeconds {
		return fmt.Errorf("lichtools: min_fuzz_seconds (%d) > max_fuzz_seconds (%d)", c.MinFuzzSeconds, c.MaxFuzzSeconds)
	}
	if c.MaxDailyFuzzSeconds <= 0 {
		c.MaxDailyFuzzSeconds = DefaultMaxDailyFuzzSeconds
	}
	if c.MemFitSafety < 1.0 {
		c.MemFitSafety = DefaultMemFitSafety
	}
	if c.DailyCapUSD <= 0 {
		c.DailyCapUSD = DefaultDailyCapUSD
	}
	if c.Findings.Register == "" {
		c.Findings.Register = DefaultRegisterRelPath
	}
	c.Limits.applyDefaults()
	return nil
}

func (l *Limits) applyDefaults() {
	if l.CPUQuota <= 0 {
		l.CPUQuota = DefaultCPUQuota
	}
	if l.MemBytes <= 0 {
		l.MemBytes = DefaultMemBytes
	}
	if l.PidsMax <= 0 {
		l.PidsMax = DefaultPidsMax
	}
	if l.ASBytes == 0 {
		l.ASBytes = l.MemBytes
	}
	if l.WallSeconds > 0 {
		l.WallTimeout = time.Duration(l.WallSeconds) * time.Second
	}
	if l.WallTimeout <= 0 {
		l.WallTimeout = DefaultWallTimeout
	}
}

// HarnessDirAbs resolves the configured harness dir against the project root.
func (c *Config) HarnessDirAbs() string {
	return c.resolve(c.Scope.HarnessDir)
}

// ResultsDirAbs resolves the configured results dir against the project root.
func (c *Config) ResultsDirAbs() string {
	return c.resolve(c.Scope.ResultsDir)
}

// RegisterAbs resolves the findings register path against the project root.
func (c *Config) RegisterAbs() string {
	return c.resolve(c.Findings.Register)
}

// PendingDirAbs resolves the confirmation-ticket directory against the root.
func (c *Config) PendingDirAbs() string {
	return c.resolve(c.PendingDir)
}

// SeedsDirAbs resolves the seed-corpus directory against the root.
func (c *Config) SeedsDirAbs() string {
	return c.resolve(c.Scope.SeedsDir)
}

func (c *Config) resolve(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(c.ProjectRoot, p))
}
