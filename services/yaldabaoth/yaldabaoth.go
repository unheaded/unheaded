// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package yaldabaoth provides chaos engineering and fault injection capabilities.
// In Gnostic philosophy, Yaldabaoth is the Demiurge - a false god who created the flawed material world.
// This service embodies that concept: deliberately introducing chaos, testing resilience,
// and proving the Kingdom can survive adversity. The adversary that makes us stronger.
//
// The Service is the one chaos engine (S-REAL-STEEL D-16) and its safety
// envelope (ADR-107):
//
//   - blast radius: every experiment passes the allowlist file before
//     anything happens, and its target is checked again right before the
//     injection (allowlist.go, D-17);
//   - steady state: real HTTP probes of allowlisted instances plus the
//     Kingdom SLO guard, before (must hold), during (the guard aborts) and
//     after (the verdict); no guard, or a guard that cannot read Victoria,
//     and nothing starts (steadystate.go, D-20);
//   - revert journal: each fault's revert recipe is written to disk before
//     the fault exists, and the fault is reverted at the end, on any abort,
//     by a per-fault TTL, at process start and by `chaos-controller
//     revert-all`; an entry goes only once the revert is verified
//     (journal.go, D-19);
//   - abort: HTTP, Wotan chaos.abort, and the file /run/unheaded/chaos/ABORT,
//     which also refuses new experiments while it exists (abort.go, D-18).
//
// Only registered injectors act. None is built in: an experiment type
// without an injector is refused, never reported as a success.
// DryRunMode (the default) runs every check and injects nothing.
package yaldabaoth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/sloguard"
)

// Experiment represents a chaos engineering experiment.
type Experiment struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Type        ExperimentType    `json:"type"`
	Target      Target            `json:"target"`
	Parameters  Params            `json:"parameters,omitempty"`
	Schedule    *Schedule         `json:"schedule,omitempty"`
	Status      ExperimentStatus  `json:"status"`
	SteadyState *SteadyState      `json:"steady_state,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	Results     *ExperimentResult `json:"results,omitempty"`
	Resolved    *ResolvedTarget   `json:"resolved,omitempty"`
}

// ExperimentType defines the kind of chaos experiment.
type ExperimentType string

const (
	ExpTypeNetworkDelay     ExperimentType = "network_delay"
	ExpTypeNetworkLoss      ExperimentType = "network_loss"
	ExpTypeNetworkPartition ExperimentType = "network_partition"
	ExpTypeCPUStress        ExperimentType = "cpu_stress"
	ExpTypeMemoryStress     ExperimentType = "memory_stress"
	ExpTypeDiskStress       ExperimentType = "disk_stress"
	ExpTypeProcessKill      ExperimentType = "process_kill"
	ExpTypeContainerKill    ExperimentType = "container_kill"
	ExpTypeNodeDrain        ExperimentType = "node_drain"
	ExpTypeDNSFailure       ExperimentType = "dns_failure"
	ExpTypeTimeSkew         ExperimentType = "time_skew"
	ExpTypeHTTPError        ExperimentType = "http_error"
	ExpTypeLatencySpike     ExperimentType = "latency_spike"
	ExpTypeResourceExhaust  ExperimentType = "resource_exhaustion"
	ExpTypeServiceStop      ExperimentType = "service_stop"
	ExpTypeCustom           ExperimentType = "custom"
)

var knownTypes = map[ExperimentType]bool{
	ExpTypeNetworkDelay: true, ExpTypeNetworkLoss: true, ExpTypeNetworkPartition: true, ExpTypeCPUStress: true,
	ExpTypeMemoryStress: true, ExpTypeDiskStress: true, ExpTypeProcessKill: true, ExpTypeContainerKill: true,
	ExpTypeNodeDrain: true, ExpTypeDNSFailure: true, ExpTypeTimeSkew: true, ExpTypeHTTPError: true,
	ExpTypeLatencySpike: true, ExpTypeResourceExhaust: true, ExpTypeServiceStop: true, ExpTypeCustom: true,
}

// ExperimentStatus tracks experiment lifecycle.
type ExperimentStatus string

const (
	StatusRunning   ExperimentStatus = "running"
	StatusCompleted ExperimentStatus = "completed"
	StatusAborted   ExperimentStatus = "aborted"
	StatusFailed    ExperimentStatus = "failed"
)

// Target names what an experiment affects; the allowlist resolves it.
type Target struct {
	Kind TargetKind `json:"kind"`
	ID   string     `json:"id"`
}

// Params are an experiment's numeric knobs (delay_ms, jitter_ms,
// loss_percent), checked against the allowlist limits.
type Params map[string]any

// Schedule holds the experiment's duration.
type Schedule struct {
	Duration time.Duration `json:"duration"`
}

// UnmarshalJSON accepts "120s" as well as nanoseconds.
func (s *Schedule) UnmarshalJSON(b []byte) error {
	var raw struct {
		Duration json.RawMessage `json:"duration"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw.Duration) == 0 {
		return nil
	}
	var str string
	if err := json.Unmarshal(raw.Duration, &str); err == nil {
		d, err := time.ParseDuration(str)
		if err != nil {
			return fmt.Errorf("schedule.duration %q: %w", str, err)
		}
		s.Duration = d
		return nil
	}
	var ns int64
	if err := json.Unmarshal(raw.Duration, &ns); err != nil {
		return errors.New("schedule.duration: want \"120s\" or nanoseconds")
	}
	s.Duration = time.Duration(ns)
	return nil
}

// ExperimentResult captures the outcome.
type ExperimentResult struct {
	Success           bool              `json:"success"`
	HypothesisValid   bool              `json:"hypothesis_valid"`
	Reverted          bool              `json:"reverted"`
	DryRun            bool              `json:"dry_run,omitempty"`
	SteadyStateProbes []ProbeResult     `json:"steady_state_probes,omitempty"`
	GuardBefore       *sloguard.Verdict `json:"guard_before,omitempty"`
	GuardAfter        *sloguard.Verdict `json:"guard_after,omitempty"`
	Findings          []Finding         `json:"findings,omitempty"`
	Duration          time.Duration     `json:"duration"`
	AbortReason       string            `json:"abort_reason,omitempty"`
	Message           string            `json:"message,omitempty"`
}

// Finding represents a discovery from the experiment.
type Finding struct {
	Severity    FindingSeverity `json:"severity"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Remediation string          `json:"remediation,omitempty"`
}

// FindingSeverity indicates importance of finding.
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "critical"
	SeverityHigh     FindingSeverity = "high"
	SeverityMedium   FindingSeverity = "medium"
	SeverityLow      FindingSeverity = "low"
	SeverityInfo     FindingSeverity = "info"
)

// Recipe is how to undo one fault; it is journaled before the fault exists.
type Recipe map[string]string

// Injector injects one kind of fault. Plan describes the revert without
// acting; Inject acts; Revert undoes; Reverted verifies. An injector must
// check the target again itself (Allowlist.CheckTarget) and must never run
// a shell.
type Injector interface {
	Plan(ctx context.Context, t ResolvedTarget, p Params) (Recipe, error)
	Inject(ctx context.Context, t ResolvedTarget, p Params, r Recipe) error
	Revert(ctx context.Context, r Recipe) error
	Reverted(ctx context.Context, r Recipe) (bool, error)
}

// SLOChecker is the Kingdom SLO guard (pkg/sloguard).
type SLOChecker interface {
	Check(ctx context.Context) (sloguard.Verdict, error)
}

// Publisher sends an event (Wotan); errors are logged, never fatal.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// Wotan topics. chaos.* are control topics: never buffered (D-10).
const (
	TopicRefused      = "chaos.refused"
	TopicRevertFailed = "chaos.revert_failed"
	TopicAbort        = "chaos.abort"
	TopicStarted      = "chaos.experiment.started"
	TopicCompleted    = "chaos.experiment.completed"
)

// Config holds Yaldabaoth service configuration.
type Config struct {
	MaxConcurrentExperiments int           `json:"max_concurrent_experiments"` // also capped by the allowlist
	DefaultDuration          time.Duration `json:"default_duration"`
	SafetyCheckInterval      time.Duration `json:"safety_check_interval"`
	DryRunMode               bool          `json:"dry_run_mode"`
	// StateDir holds the revert journal; without it nothing runs.
	StateDir string `json:"state_dir"`
	// AbortFile, while it exists, aborts everything and refuses new runs.
	AbortFile string        `json:"abort_file"`
	AbortPoll time.Duration `json:"abort_poll"`
	// RevertGrace is added to an experiment's duration to make its fault's
	// TTL: the latest moment the fault is reverted whatever else happens.
	RevertGrace time.Duration `json:"revert_grace"`
	// Hostname tells systemd checks which host this is (os.Hostname).
	Hostname func() (string, error) `json:"-"`
}

// DefaultConfig returns safe defaults: dry run on.
func DefaultConfig() *Config {
	return &Config{
		MaxConcurrentExperiments: 1,
		DefaultDuration:          2 * time.Minute,
		SafetyCheckInterval:      10 * time.Second,
		DryRunMode:               true,
		AbortFile:                "/run/unheaded/chaos/ABORT",
		AbortPoll:                time.Second,
		RevertGrace:              30 * time.Second,
	}
}

// Deps are the Service's collaborators.
type Deps struct {
	Allowlist  *Allowlist   // nil refuses everything
	Guard      SLOChecker   // nil refuses everything (D-20)
	Resolver   Resolver     // Cuirass; nil refuses LXD targets
	Publisher  Publisher    // nil: no events
	Subscriber AbortSource  // Wotan chaos.abort; nil: that path is off
	HTTPClient *http.Client // probes; nil: a no-redirect, no-proxy client
	Log        *logger.Logger
}

type running struct {
	cancel context.CancelCauseFunc
	done   chan struct{}
}

// Service is the Yaldabaoth chaos engine.
type Service struct {
	log    *logger.Logger
	config *Config
	deps   Deps
	audit  *AuditTrail

	mu          sync.RWMutex
	experiments map[string]*Experiment
	order       []string
	injectors   map[ExperimentType]Injector
	// refuseMsg gives a custom refusal message for a known type that has no
	// injector (D-22: http_error points at the workload's own error knob).
	refuseMsg  map[ExperimentType]string
	runs       map[string]*running
	expCounter int64
	stopped    bool

	journal  *journal
	revertMu sync.Mutex
	ttlMu    sync.Mutex
	ttl      map[string]*time.Timer

	abortPresent atomic.Bool
	startedAt    time.Time
	seenMu       sync.Mutex
	seenAborts   map[string]bool
	seenOrder    []string
	probeMu      sync.RWMutex
	probeURLFn   func(info *InstanceInfo, p Probe) (string, error)
	client       *http.Client
	refusalLimit *rateLimit

	bg     sync.WaitGroup
	bgStop context.CancelFunc

	registry   *metrics.Registry
	expTotal   *metrics.CounterVec
	activeG    *metrics.Gauge
	abortTotal *metrics.CounterVec
	refusals   *metrics.CounterVec
	abortsMu   sync.Mutex
	aborts     map[string]int

	// testHoldLoop keeps a run from ending on its own timer (TTL tests).
	testHoldLoop bool
}

// NewService builds the engine and loads the revert journal. A damaged
// journal is an error: reading it as empty would forget faults that are
// still in place.
func NewService(cfg *Config, deps Deps) (*Service, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if deps.Log == nil {
		deps.Log = logger.New(io.Discard)
	}
	if cfg.SafetyCheckInterval <= 0 {
		cfg.SafetyCheckInterval = 10 * time.Second
	}
	if cfg.AbortPoll <= 0 {
		cfg.AbortPoll = time.Second
	}
	if cfg.DefaultDuration <= 0 {
		cfg.DefaultDuration = 2 * time.Minute
	}
	if cfg.MaxConcurrentExperiments <= 0 {
		cfg.MaxConcurrentExperiments = 1
	}
	s := &Service{
		log:          deps.Log,
		config:       cfg,
		deps:         deps,
		audit:        NewAuditTrail(deps.Log, 10000),
		experiments:  map[string]*Experiment{},
		injectors:    map[ExperimentType]Injector{},
		refuseMsg:    map[ExperimentType]string{},
		runs:         map[string]*running{},
		ttl:          map[string]*time.Timer{},
		aborts:       map[string]int{},
		refusalLimit: newRateLimit(refusalPublishBurst, time.Minute),
		seenAborts:   map[string]bool{},
		startedAt:    time.Now(),
	}
	s.probeURLFn = defaultProbeURL
	s.client = deps.HTTPClient
	if s.client == nil {
		s.client = &http.Client{
			Transport:     &http.Transport{Proxy: nil},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	if cfg.StateDir != "" {
		j, err := openJournal(cfg.StateDir)
		if err != nil {
			return nil, err
		}
		s.journal = j
	}
	s.registry = metrics.NewRegistry()
	s.expTotal = metrics.NewCounterVec("chaos_experiments_total", "Finished experiments by type and result", nil, []string{"type", "result"})
	s.activeG = metrics.NewGauge("chaos_active_injections", "Faults in place now (revert journal entries)", nil)
	s.abortTotal = metrics.NewCounterVec("chaos_aborts_total", "Aborts by path (http, wotan, file, ttl, guard, probes, gameday)", nil, []string{"path"})
	s.refusals = metrics.NewCounterVec("chaos_refusals_total", "Experiments refused, by reason", nil, []string{"reason"})
	for _, c := range []metrics.Collector{s.expTotal, s.activeG, s.abortTotal, s.refusals} {
		s.registry.MustRegister(c)
	}
	s.updateActive()
	return s, nil
}

// Registry holds the chaos metrics.
func (s *Service) Registry() *metrics.Registry { return s.registry }

// Audit is the audit trail of every start, refusal and revert.
func (s *Service) Audit() *AuditTrail { return s.audit }

func (s *Service) env() CheckEnv {
	return CheckEnv{Resolver: s.deps.Resolver, Hostname: s.config.Hostname, DefaultDuration: s.config.DefaultDuration}
}

// RegisterInjector registers the injector for an experiment type.
func (s *Service) RegisterInjector(t ExperimentType, inj Injector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.injectors[t] = inj
}

// RefuseType records a custom refusal message for a known experiment type
// that has no injector (D-22). The type stays registered as known but every
// request for it is refused with this reason instead of the generic "no
// injector" one.
func (s *Service) RefuseType(t ExperimentType, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refuseMsg[t] = msg
}

// Start reverts whatever a previous controller left in the journal, then
// watches the abort file and, if configured, Wotan's chaos.abort.
func (s *Service) Start(ctx context.Context) error {
	s.startedAt = time.Now()
	if left, err := s.RevertAll(ctx); left > 0 {
		s.log.Error().Int("left", left).Msg("chaos: faults from a previous run could not be reverted; see chaos.revert_failed")
	} else if err != nil {
		s.log.Warn().Err(err).Msg("chaos: replaying the journal")
	}
	bctx, cancel := context.WithCancel(ctx)
	s.bgStop = cancel
	s.bg.Add(1)
	go func() { defer s.bg.Done(); s.watchAbortFile(bctx) }()
	if s.deps.Subscriber != nil {
		s.bg.Add(1)
		go func() { defer s.bg.Done(); s.subscribeAbort(bctx) }()
	}
	return nil
}

// Stop aborts every running experiment, waits until each has reverted,
// and stops the watchers.
func (s *Service) Stop() {
	s.mu.Lock()
	s.stopped = true
	runs := make([]*running, 0, len(s.runs))
	for _, r := range s.runs {
		runs = append(runs, r)
	}
	s.mu.Unlock()
	for _, r := range runs {
		r.cancel(errors.New("controller stopping"))
	}
	for _, r := range runs {
		<-r.done
	}
	if s.bgStop != nil {
		s.bgStop()
	}
	s.bg.Wait()
	s.ttlMu.Lock()
	for id, t := range s.ttl {
		t.Stop()
		delete(s.ttl, id)
	}
	s.ttlMu.Unlock()
}

func copyExperiment(e *Experiment) *Experiment {
	b, _ := json.Marshal(e)
	var out Experiment
	_ = json.Unmarshal(b, &out)
	return &out
}

// Get returns a copy of an experiment.
func (s *Service) Get(id string) (*Experiment, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.experiments[id]
	if !ok {
		return nil, false
	}
	return copyExperiment(e), true
}

// List returns copies of every experiment, oldest first.
func (s *Service) List() []*Experiment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Experiment, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, copyExperiment(s.experiments[id]))
	}
	return out
}

// Wait blocks until the experiment has finished (and reverted).
func (s *Service) Wait(ctx context.Context, id string) (*Experiment, error) {
	s.mu.RLock()
	r := s.runs[id]
	_, known := s.experiments[id]
	s.mu.RUnlock()
	if !known {
		return nil, fmt.Errorf("no experiment %s", id)
	}
	if r != nil {
		select {
		case <-r.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	e, _ := s.Get(id)
	return e, nil
}

// RunAndWait starts exp and waits for it; if ctx ends first the
// experiment is aborted (and reverted) before RunAndWait returns.
func (s *Service) RunAndWait(ctx context.Context, exp *Experiment) (*Experiment, error) {
	started, err := s.StartExperiment(ctx, exp)
	if err != nil {
		return nil, err
	}
	e, err := s.Wait(ctx, started.ID)
	if err != nil {
		s.abortOne(started.ID, "gameday", "game day stopped")
		return s.Wait(context.Background(), started.ID)
	}
	return e, nil
}

const (
	maxNameLen = 128
	// maxProbes bounds the probe traffic one request can cause.
	maxProbes = 10
	// maxHistory bounds the finished experiments kept in memory.
	maxHistory = 500
)

// StartExperiment checks exp against the whole envelope and, if every
// check passes, starts it. A refusal is a *Refusal, audited and counted.
func (s *Service) StartExperiment(ctx context.Context, exp *Experiment) (*Experiment, error) {
	if exp == nil {
		return nil, s.refused(nil, refuse(RefuseInvalid, "no experiment"))
	}
	e := copyExperiment(exp)
	e.Status, e.Results, e.StartedAt, e.CompletedAt, e.Resolved = "", nil, nil, nil, nil
	switch {
	case len(e.Name) > maxNameLen:
		return nil, s.refused(e, refuse(RefuseInvalid, "name longer than %d", maxNameLen))
	case !knownTypes[e.Type]:
		return nil, s.refused(e, refuse(RefuseInvalid, "unknown experiment type %q", e.Type))
	case e.Target.Kind == "" || e.Target.ID == "":
		return nil, s.refused(e, refuse(RefuseInvalid, "target kind and id are required"))
	case e.SteadyState != nil && len(e.SteadyState.Probes) > maxProbes:
		return nil, s.refused(e, refuse(RefuseInvalid, "at most %d probes", maxProbes))
	}
	if s.abortPresent.Load() || s.abortFileExists() {
		return nil, s.refused(e, refuse(RefuseAbortFile, "%s exists: remove it to allow experiments", s.config.AbortFile))
	}
	if s.journal == nil {
		return nil, s.refused(e, refuse(RefuseNoJournal, "no state dir: a fault could not be journaled before it lands"))
	}
	if s.deps.Guard == nil {
		return nil, s.refused(e, refuse(RefuseNoGuard, "no Kingdom SLO guard (VM_URL): no experiment starts without one"))
	}
	s.mu.RLock()
	inj := s.injectors[e.Type]
	refuseMsg := s.refuseMsg[e.Type]
	stopped := s.stopped
	s.mu.RUnlock()
	if stopped {
		return nil, s.refused(e, refuse(RefuseStopped, "the controller is stopping"))
	}
	if inj == nil {
		if refuseMsg != "" {
			return nil, s.refused(e, refuse(RefuseNoInjector, "%s", refuseMsg))
		}
		return nil, s.refused(e, refuse(RefuseNoInjector, "%s is not implemented: no injector", e.Type))
	}
	limit := s.config.MaxConcurrentExperiments
	if s.deps.Allowlist != nil && s.deps.Allowlist.Limits.MaxConcurrent < limit {
		limit = s.deps.Allowlist.Limits.MaxConcurrent
	}
	rt, err := s.deps.Allowlist.Check(ctx, e, s.env())
	if err != nil {
		var r *Refusal
		if !errors.As(err, &r) {
			r = refuse(RefuseAllowlist, "%v", err)
		}
		return nil, s.refused(e, r)
	}
	e.Resolved = &rt

	// Steady state must hold before anything is injected.
	before, gerr := s.deps.Guard.Check(ctx)
	if gerr != nil {
		return nil, s.refused(e, refuse(RefuseSteadyState, "Kingdom SLO guard cannot read Victoria: %v", gerr))
	}
	if !before.OK {
		return nil, s.refused(e, refuse(RefuseSteadyState, "Kingdom SLO guard: %s", before.Reason))
	}
	probes, ratio := s.checkProbes(ctx, e)
	if e.SteadyState != nil && len(e.SteadyState.Probes) > 0 && ratio < e.SteadyState.minRatio() {
		return nil, s.refused(e, refuse(RefuseSteadyState, "probes %.0f%% healthy before injection, need %.0f%%", ratio*100, e.SteadyState.minRatio()*100))
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil, s.refused(e, refuse(RefuseStopped, "the controller is stopping"))
	}
	if n := len(s.runs); n >= limit {
		s.mu.Unlock()
		return nil, s.refused(e, refuse(RefuseConcurrency, "%d experiment(s) running, the limit is %d", n, limit))
	}
	s.expCounter++
	e.ID = fmt.Sprintf("chaos-%d-%d", time.Now().UnixNano(), s.expCounter)
	now := time.Now().UTC()
	e.CreatedAt, e.StartedAt, e.Status = now, &now, StatusRunning
	e.Results = &ExperimentResult{GuardBefore: &before, SteadyStateProbes: probes, DryRun: s.config.DryRunMode}
	rctx, cancel := context.WithCancelCause(context.WithoutCancel(ctx)) // #nosec G118 -- cancel kept in s.runs; Stop, Abort and the run itself call it
	r := &running{cancel: cancel, done: make(chan struct{})}
	s.experiments[e.ID] = e
	s.order = append(s.order, e.ID)
	s.runs[e.ID] = r
	s.pruneLocked()
	out := copyExperiment(e) // e belongs to the run from here on
	s.mu.Unlock()

	s.audit.Record(AuditEntry{Timestamp: now, Action: "start", RuleID: out.ID, RuleSpec: describe(out), Result: "running"})
	s.publish(TopicStarted, map[string]any{"experiment_id": out.ID, "type": out.Type, "target": out.Target, "dry_run": s.config.DryRunMode})
	go s.run(rctx, out.ID, rt, inj, r)
	return out, nil
}

// pruneLocked drops the oldest finished experiments beyond maxHistory;
// the caller holds mu.
func (s *Service) pruneLocked() {
	for len(s.order) > maxHistory {
		dropped := false
		for i, id := range s.order {
			if _, running := s.runs[id]; running {
				continue
			}
			delete(s.experiments, id)
			s.order = append(s.order[:i:i], s.order[i+1:]...)
			dropped = true
			break
		}
		if !dropped {
			return
		}
	}
}

func describe(e *Experiment) string {
	if e == nil {
		return ""
	}
	d := time.Duration(0)
	if e.Schedule != nil {
		d = e.Schedule.Duration
	}
	return fmt.Sprintf("type=%s target=%s/%s duration=%v params=%v", e.Type, e.Target.Kind, e.Target.ID, d, e.Parameters)
}

// refused audits, counts and (at a bounded rate) publishes a refusal.
func (s *Service) refused(e *Experiment, r *Refusal) error {
	s.refusals.WithLabelValues(string(r.Reason)).Inc()
	s.audit.Record(AuditEntry{Timestamp: time.Now().UTC(), Action: "refuse", RuleSpec: describe(e), Result: r.Error()})
	s.log.Warn().Str("reason", string(r.Reason)).Str("detail", r.Detail).Msg("chaos: experiment refused")
	if s.refusalLimit.allow() {
		var target Target
		if e != nil {
			target = e.Target
		}
		s.publish(TopicRefused, map[string]any{"reason": r.Reason, "detail": r.Detail, "target": target})
	}
	return r
}

func (s *Service) duration(e *Experiment) time.Duration {
	if e.Schedule != nil && e.Schedule.Duration > 0 {
		return e.Schedule.Duration
	}
	return s.config.DefaultDuration
}

func (s *Service) update(id string, f func(e *Experiment)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.experiments[id]; ok {
		f(e)
	}
}

// run is one experiment's life: journal, inject, watch, revert, verdict.
func (s *Service) run(ctx context.Context, id string, rt ResolvedTarget, inj Injector, r *running) {
	start := time.Now()
	e, _ := s.Get(id)
	status, abortReason, message := StatusCompleted, "", ""
	reverted, injected := false, false
	var findings []Finding

	defer func() {
		r.cancel(nil)
		s.mu.Lock()
		delete(s.runs, id)
		s.mu.Unlock()
		close(r.done)
	}()

	finish := func() {
		// Steady state after the revert: the verdict.
		after, gerr := s.deps.Guard.Check(context.Background())
		if gerr != nil {
			after = sloguard.Verdict{Reason: "cannot read Victoria: " + gerr.Error()}
		}
		probes, ratio := s.checkProbes(context.Background(), e)
		hyp := after.OK && (e.SteadyState == nil || len(e.SteadyState.Probes) == 0 || ratio >= e.SteadyState.minRatio())
		done := time.Now().UTC()
		dry := s.config.DryRunMode
		s.update(id, func(x *Experiment) {
			x.Status, x.CompletedAt = status, &done
			res := x.Results
			res.Duration = time.Since(start)
			res.GuardAfter = &after
			res.SteadyStateProbes = append(res.SteadyStateProbes, probes...)
			res.HypothesisValid = hyp
			res.Reverted = reverted || (dry && !injected)
			res.AbortReason, res.Message = abortReason, message
			res.Findings = append(res.Findings, findings...)
			res.Success = !dry && status == StatusCompleted && reverted && hyp
		})
		result := string(status)
		if dry {
			result = "dry_run"
		}
		s.expTotal.WithLabelValues(string(e.Type), result).Inc()
		s.audit.Record(AuditEntry{Timestamp: done, Action: "finish", RuleID: id, Result: result + " " + abortReason})
		s.publish(TopicCompleted, map[string]any{"experiment_id": id, "status": status, "reverted": reverted,
			"hypothesis_valid": hyp, "abort_reason": abortReason, "dry_run": dry})
	}
	defer finish()

	if s.config.DryRunMode {
		message = "dry run: allowlist and steady state passed; nothing was injected"
		return
	}

	recipe, err := inj.Plan(ctx, rt, e.Parameters)
	if err != nil {
		status, message = StatusFailed, "plan: "+err.Error()
		return
	}
	d := s.duration(e)
	entry := &JournalEntry{ID: id, Type: e.Type, Target: rt, Recipe: recipe,
		Created: time.Now().UTC(), Expires: time.Now().UTC().Add(d + s.config.RevertGrace)}
	if err := s.journal.add(entry); err != nil {
		status, message = StatusFailed, "journal write failed, nothing injected: "+err.Error()
		return
	}
	s.updateActive()
	s.armTTL(entry)

	revert := func(why string) {
		ok, f := s.revertEntry(id, why)
		reverted = ok
		if f != nil {
			findings = append(findings, *f)
		}
	}

	// The second, independent target check, right before acting.
	if _, err := s.deps.Allowlist.CheckTarget(ctx, e.Target, s.env()); err != nil {
		status, message = StatusFailed, "target no longer allowed: "+err.Error()
		revert("target check failed")
		return
	}
	injected = true
	if err := inj.Inject(ctx, rt, e.Parameters, recipe); err != nil {
		status, message = StatusFailed, "inject: "+err.Error()
		revert("inject failed")
		return
	}

	timer := time.NewTimer(d)
	defer timer.Stop()
	if s.testHoldLoop {
		timer.Stop()
	}
	tick := time.NewTicker(s.config.SafetyCheckInterval)
	defer tick.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			status, abortReason = StatusAborted, context.Cause(ctx).Error()
			break loop
		case <-timer.C:
			break loop
		case <-tick.C:
			if path, why := s.shouldAbort(ctx, e); why != "" {
				s.countAbort(path)
				status, abortReason = StatusAborted, why
				break loop
			}
		}
	}
	revert(string(status))
	if !reverted && message == "" {
		message = "the fault could not be verified as reverted; its journal entry stays"
	}
}

func (s *Service) updateActive() {
	if s.journal == nil {
		s.activeG.Set(0)
		return
	}
	s.activeG.Set(float64(len(s.journal.list())))
}

func (s *Service) publish(topic string, data map[string]any) {
	if s.deps.Publisher == nil {
		return
	}
	data["timestamp"] = time.Now().UnixMilli()
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.deps.Publisher.Publish(ctx, topic, payload); err != nil {
		s.log.Warn().Err(err).Str("topic", topic).Msg("chaos: event not published")
	}
}

// Stats returns service statistics.
func (s *Service) Stats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byStatus := map[string]int{}
	for _, e := range s.experiments {
		byStatus[string(e.Status)]++
	}
	types := make([]string, 0, len(s.injectors))
	for t := range s.injectors {
		types = append(types, string(t))
	}
	sort.Strings(types)
	active := 0
	if s.journal != nil {
		active = len(s.journal.list())
	}
	return map[string]any{
		"total_experiments":   len(s.experiments),
		"running_experiments": len(s.runs),
		"by_status":           byStatus,
		"injectors":           types,
		"active_injections":   active,
		"dry_run_mode":        s.config.DryRunMode,
	}
}

// --- refusal publishing rate ---

// refusalPublishBurst bounds chaos.refused events per minute: refusals are
// all audited and counted, but a flood of bad requests cannot flood Wotan.
const refusalPublishBurst = 10

type rateLimit struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	start  time.Time
	n      int
}

func newRateLimit(max int, window time.Duration) *rateLimit {
	return &rateLimit{max: max, window: window}
}

func (r *rateLimit) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if now.Sub(r.start) > r.window {
		r.start, r.n = now, 0
	}
	if r.n >= r.max {
		return false
	}
	r.n++
	return true
}
