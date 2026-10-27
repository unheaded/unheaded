// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package sword is the Kingdom's deploy service (S-REAL-STEEL Phase 8).
//
// A deployment runs a pkg/deploy/strategy strategy (rolling, blue_green,
// canary, recreate) through the Cuirass driver: every instance is created,
// health-checked and deleted by Cuirass, and Sword reports success only
// when Cuirass says the new instances are healthy. A canary is judged by
// the Analysis it is given (analysis.go: the instances' own /metrics plus
// the Kingdom SLO guard). A failed deployment is rolled back to the
// previous successful one through pkg/deploy/rollback; a canary that fails
// analysis rolls itself back.
//
// History (history.go) lives in The Well (unheaded_ops.deploy_history,
// migration 039) when Config.History is a Well history, otherwise in one
// JSON file under the data dir (pkg/statefile, D-07): with neither, every
// mutating route answers 503; damaged history stops Sword at start; a
// failed save of a new request answers 503 and forgets the request.
// Mutating routes answer 403 unless AUTH_ENABLED=true.
//
// Routes:
//
//	POST /api/v1/deployments                 start one (202)
//	GET  /api/v1/deployments                 history, oldest first
//	GET  /api/v1/deployments/{id}            one record
//	POST /api/v1/deployments/{id}/rollback   back to the previous success (202)
//	GET  /health, /ready, /metrics
package sword

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/deploy/cuirassdriver"
	"unheaded/pkg/deploy/rollback"
	"unheaded/pkg/deploy/strategy"
	"unheaded/pkg/httputil"
	"unheaded/pkg/metrics"
	"unheaded/pkg/statefile"
)

// historyTimeout bounds one history write or read.
const historyTimeout = 10 * time.Second

// Status is a deployment's lifecycle state.
type Status string

const (
	StatusPending     Status = "pending"
	StatusInProgress  Status = "in_progress"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusRollingBack Status = "rolling_back"
	StatusRolledBack  Status = "rolled_back"
)

// Rollback reasons.
const (
	ReasonAnalysisFailed = "analysis_failed"
	ReasonDeployFailed   = "deploy_failed"
	ReasonManual         = "manual"
)

// Wotan topics (control topics: never buffered by the client, D-10).
const (
	TopicStarted    = "deploy.started"
	TopicProgress   = "deploy.progress"
	TopicSucceeded  = "deploy.succeeded"
	TopicFailed     = "deploy.failed"
	TopicRolledBack = "deploy.rolled_back"
)

const (
	stateFileName  = "sword.json"
	maxRequestBody = 64 << 10
	maxReplicas    = 20
	maxHistory     = 500
	maxDeadline    = 2 * time.Hour
	maxPause       = time.Hour
	maxInterval    = 10 * time.Minute
)

var (
	serviceRe = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,30}[a-z0-9])?$`)
	// Versions are already in their instance-name form ([a-z0-9-]), so the
	// label Cuirass stores and the version Sword records are the same string.
	versionRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)
)

// Duration is a time.Duration written as "60s" in JSON.
type Duration time.Duration

// MarshalJSON writes "60s".
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON reads "60s" (or 0).
func (d *Duration) UnmarshalJSON(b []byte) error {
	if string(b) == "0" || string(b) == "null" {
		*d = 0
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration: want a string like \"30s\"")
	}
	v, err := time.ParseDuration(s)
	if err != nil || v < 0 {
		return fmt.Errorf("duration %q: want a non-negative Go duration", s)
	}
	*d = Duration(v)
	return nil
}

// StepSpec is one canary step.
type StepSpec struct {
	Weight   int      `json:"weight"`
	Pause    Duration `json:"pause,omitempty"`
	Analysis bool     `json:"analysis,omitempty"`
}

// AnalysisSpec configures canary analysis.
type AnalysisSpec struct {
	ErrorRateThreshold  float64  `json:"error_rate_threshold"`
	LatencyP99Threshold Duration `json:"latency_p99_threshold,omitempty"`
	MinSampleSize       int      `json:"min_sample_size"`
	Interval            Duration `json:"interval"`
}

// StrategySpec is the strategy part of a request.
type StrategySpec struct {
	Type             strategy.Type `json:"type"`
	MaxSurge         int           `json:"max_surge,omitempty"`
	MaxUnavailable   int           `json:"max_unavailable,omitempty"`
	TestDuration     Duration      `json:"test_duration,omitempty"`
	ProgressDeadline Duration      `json:"progress_deadline,omitempty"`
	Steps            []StepSpec    `json:"steps,omitempty"`
	Analysis         *AnalysisSpec `json:"analysis,omitempty"`
}

// DeployRequest is POST /api/v1/deployments.
type DeployRequest struct {
	Service  string                    `json:"service"`
	Version  string                    `json:"version"`
	Replicas int                       `json:"replicas"`
	Strategy StrategySpec              `json:"strategy"`
	Health   *cuirassdriver.HealthSpec `json:"health"`
	Proxy    *cuirassdriver.ProxySpec  `json:"proxy,omitempty"`
	Labels   map[string]string         `json:"labels,omitempty"`
}

// Deployment is one record of the history.
type Deployment struct {
	ID              string                    `json:"id"`
	Service         string                    `json:"service"`
	Version         string                    `json:"version"`
	PreviousVersion string                    `json:"previous_version,omitempty"`
	Replicas        int                       `json:"replicas"`
	Strategy        StrategySpec              `json:"strategy"`
	Health          *cuirassdriver.HealthSpec `json:"health,omitempty"`
	Proxy           *cuirassdriver.ProxySpec  `json:"proxy,omitempty"`
	Labels          map[string]string         `json:"labels,omitempty"`
	Status          Status                    `json:"status"`
	Reason          string                    `json:"reason,omitempty"`
	Message         string                    `json:"message,omitempty"`
	RolledBackTo    string                    `json:"rolled_back_to,omitempty"`
	Analysis        *strategy.AnalysisResult  `json:"analysis,omitempty"`
	CreatedAt       time.Time                 `json:"created_at"`
	StartedAt       *time.Time                `json:"started_at,omitempty"`
	CompletedAt     *time.Time                `json:"completed_at,omitempty"`
}

func (d *Deployment) final() bool {
	switch d.Status {
	case StatusSucceeded, StatusFailed, StatusRolledBack:
		return true
	}
	return false
}

func (d *Deployment) spec() cuirassdriver.ServiceSpec {
	return cuirassdriver.ServiceSpec{Service: d.Service, Health: d.Health, Proxy: d.Proxy, Labels: d.Labels}
}

type stateFile struct {
	Version     int           `json:"version"`
	Deployments []*Deployment `json:"deployments"`
}

// Analysis judges a canary step. port is the instances' health port, where
// their /metrics is served too.
type Analysis interface {
	Analyze(ctx context.Context, req *strategy.AnalysisRequest, port int) (*strategy.AnalysisResult, error)
}

// Publisher sends one event; errors are logged, never fatal.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// Config configures Sword.
type Config struct {
	// History stores the records (NewWellHistory); nil: the file in
	// DataDir. A Well history with DataDir set imports DataDir's file once,
	// into an empty Well.
	History      History
	DataDir      string                // "" (and no History) refuses every write (503)
	AuthEnabled  bool                  // false refuses every write (403)
	Driver       *cuirassdriver.Driver // nil refuses deployments (503)
	Analysis     Analysis              // nil refuses canary deployments (400)
	Publisher    Publisher             // nil: no events
	PollInterval time.Duration         // how often strategies re-ask health (default 2s)
	// NoAutoRollback keeps a failed deployment as it is instead of rolling
	// back to the previous success.
	NoAutoRollback bool
	Log            *zerolog.Logger
}

// Service is Sword.
type Service struct {
	cfg     Config
	history History // nil: read-only
	log     zerolog.Logger

	mu      sync.Mutex
	deps    []*Deployment
	byID    map[string]*Deployment
	busy    map[string]string // service -> deployment being worked on
	counter int64

	rollbacks *rollback.Manager

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	registry  *metrics.Registry
	deploysC  *metrics.CounterVec
	rollbackC *metrics.CounterVec
}

// New loads the history and returns Sword. A damaged state file is an
// error: starting empty would forget what to roll back to.
func New(cfg Config) (*Service, error) {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	s := &Service{
		cfg:     cfg,
		history: cfg.History,
		byID:    map[string]*Deployment{},
		busy:    map[string]string{},
		log:     zerolog.Nop(),
	}
	if s.history == nil {
		s.history = NewFileHistory(cfg.DataDir)
	}
	if cfg.Log != nil {
		s.log = *cfg.Log
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.registry = metrics.NewRegistry()
	s.deploysC = metrics.NewCounterVec("sword_deployments_total", "Finished deployments by strategy and result", nil, []string{"strategy", "result"})
	s.rollbackC = metrics.NewCounterVec("sword_rollbacks_total", "Rollbacks by reason (analysis_failed, deploy_failed, manual)", nil, []string{"reason"})
	s.registry.MustRegister(s.deploysC)
	s.registry.MustRegister(s.rollbackC)

	if s.history != nil {
		if err := s.loadHistory(); err != nil {
			return nil, err
		}
		s.log.Info().Str("history", s.history.Mode()).Int("records", len(s.deps)).Msg("sword: history loaded")
	} else {
		s.log.Info().Str("history", "none").Msg("sword: no history store: read-only")
	}
	if cfg.Driver != nil {
		s.rollbacks = rollback.NewManager(&lookup{s: s},
			&cuirassdriver.Updater{Driver: cfg.Driver, Specs: s.specFor},
			&cuirassdriver.Shifter{Driver: cfg.Driver, Specs: s.specFor})
	}
	return s, nil
}

// loadHistory reads the stored records. A record that was not final when
// Sword stopped is marked failed: nothing runs it any more.
func (s *Service) loadHistory() error {
	ctx, cancel := context.WithTimeout(context.Background(), historyTimeout)
	defer cancel()
	deps, err := s.history.Load(ctx)
	if err != nil {
		return fmt.Errorf("sword: history (%s): %w", s.history.Mode(), err)
	}
	if len(deps) == 0 && s.history.Mode() == "well" {
		if deps, err = s.importFile(ctx); err != nil {
			return err
		}
	}
	var interrupted []*Deployment
	for _, d := range deps {
		if d == nil || d.ID == "" || s.byID[d.ID] != nil {
			return fmt.Errorf("sword: history: a record without a unique id")
		}
		s.deps = append(s.deps, d)
		s.byID[d.ID] = d
		if !d.final() {
			interrupted = append(interrupted, d)
		}
	}
	for _, d := range interrupted {
		// Nothing is running it any more: say so instead of leaving it
		// "in progress" for ever.
		from := d.Status
		d.Status = StatusFailed
		d.Message = "interrupted by a Sword restart; check Cuirass's state before deploying again"
		if err := s.history.Update(ctx, d, from, s.deps); err != nil {
			return fmt.Errorf("sword: history: %w", err)
		}
	}
	return nil
}

// importFile copies DataDir's sword.json into an empty Well once, when a
// Sword is switched from the file to the Well, so the previous successes
// are still there to roll back to. The file is left as it was.
func (s *Service) importFile(ctx context.Context) ([]*Deployment, error) {
	f := NewFileHistory(s.cfg.DataDir)
	if f == nil {
		return nil, nil
	}
	deps, err := f.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("sword: importing %s into the Well: %w", stateFileName, err)
	}
	for i, d := range deps {
		if d == nil {
			return nil, fmt.Errorf("sword: importing %s: an empty record", stateFileName)
		}
		if err := s.history.Record(ctx, d, deps[:i+1]); err != nil {
			return nil, fmt.Errorf("sword: importing %s into the Well: %w", stateFileName, err)
		}
	}
	if len(deps) > 0 {
		s.log.Info().Int("records", len(deps)).Str("from", s.cfg.DataDir).Msg("sword: file history imported into the Well")
	}
	return deps, nil
}

// HistoryMode is "well", "file" or "none".
func (s *Service) HistoryMode() string {
	if s.history == nil {
		return "none"
	}
	return s.history.Mode()
}

// Close stops running work and waits for it.
func (s *Service) Close() {
	s.cancel()
	s.wg.Wait()
}

// Registry holds Sword's own metrics.
func (s *Service) Registry() *metrics.Registry { return s.registry }

func copyDeployment(d *Deployment) *Deployment {
	b, _ := json.Marshal(d)
	var out Deployment
	_ = json.Unmarshal(b, &out)
	return &out
}

// Get returns a copy of one record.
func (s *Service) Get(id string) (*Deployment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.byID[id]
	if !ok {
		return nil, false
	}
	return copyDeployment(d), true
}

// List returns copies of every record, oldest first.
func (s *Service) List() []*Deployment {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Deployment, 0, len(s.deps))
	for _, d := range s.deps {
		out = append(out, copyDeployment(d))
	}
	return out
}

// Wait blocks until the record is final and no work runs on it.
func (s *Service) Wait(ctx context.Context, id string) (*Deployment, error) {
	t := time.NewTicker(10 * time.Millisecond)
	defer t.Stop()
	for {
		s.mu.Lock()
		d, ok := s.byID[id]
		busy := ok && s.busy[d.Service] == id
		var cp *Deployment
		if ok {
			cp = copyDeployment(d)
		}
		s.mu.Unlock()
		if !ok {
			return nil, fmt.Errorf("no deployment %s", id)
		}
		if cp.final() && !busy {
			return cp, nil
		}
		select {
		case <-ctx.Done():
			return cp, ctx.Err()
		case <-t.C:
		}
	}
}

// update changes a record and persists it. A failed save is logged: the
// change already happened in the world (Cuirass), so it cannot be undone.
// Writes happen under mu so the stored order is the order of the changes.
func (s *Service) update(id string, f func(d *Deployment)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.byID[id]
	if !ok {
		return
	}
	from := d.Status
	f(d)
	if s.history == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyTimeout)
	defer cancel()
	if err := s.history.Update(ctx, d, from, s.deps); err != nil {
		s.log.Error().Err(err).Str("id", id).Msg("sword: could not persist a deployment change")
	}
}

// --- requests ---

// errBadRequest marks a validation failure (400).
var errBadRequest = errors.New("bad request")

func badf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errBadRequest, fmt.Sprintf(format, a...))
}

func (r *DeployRequest) validate(haveAnalysis bool) (*strategy.Config, error) {
	if !serviceRe.MatchString(r.Service) {
		return nil, badf("service %q: lowercase letters, digits, hyphens", r.Service)
	}
	if !versionRe.MatchString(r.Version) {
		return nil, badf("version %q: lowercase letters, digits, hyphens (it names the image %s-<version>)", r.Version, r.Service)
	}
	if r.Replicas < 1 || r.Replicas > maxReplicas {
		return nil, badf("replicas %d: want 1..%d", r.Replicas, maxReplicas)
	}
	if r.Health == nil {
		return nil, badf("health is required: Cuirass's health check is how Sword knows an instance is up")
	}
	return r.Strategy.config(haveAnalysis)
}

func (sp *StrategySpec) config(haveAnalysis bool) (*strategy.Config, error) {
	switch sp.Type {
	case strategy.TypeRolling, strategy.TypeBlueGreen, strategy.TypeCanary, strategy.TypeRecreate:
	default:
		return nil, badf("strategy.type %q: want rolling, blue_green, canary or recreate", sp.Type)
	}
	c := strategy.DefaultConfig(sp.Type)
	if d := time.Duration(sp.ProgressDeadline); d > 0 {
		if d > maxDeadline {
			return nil, badf("progress_deadline %v over %v", d, maxDeadline)
		}
		c.ProgressDeadline = d
	}
	switch sp.Type {
	case strategy.TypeRolling:
		c.MaxSurge, c.MaxUnavailable = sp.MaxSurge, sp.MaxUnavailable
		if c.MaxSurge == 0 && c.MaxUnavailable == 0 {
			c.MaxSurge = 1
		}
	case strategy.TypeBlueGreen:
		c.TestDuration = time.Duration(sp.TestDuration)
		if c.TestDuration > maxPause {
			return nil, badf("test_duration %v over %v", c.TestDuration, maxPause)
		}
	case strategy.TypeCanary:
		if !haveAnalysis {
			return nil, badf("canary needs analysis, and this Sword has none configured (VM_URL)")
		}
		if len(sp.Steps) == 0 {
			return nil, badf("canary needs steps")
		}
		c.Steps = nil
		for _, st := range sp.Steps {
			if time.Duration(st.Pause) > maxPause {
				return nil, badf("pause %v over %v", time.Duration(st.Pause), maxPause)
			}
			c.Steps = append(c.Steps, strategy.CanaryStep{Weight: st.Weight, Pause: time.Duration(st.Pause), Analysis: st.Analysis})
		}
		if a := sp.Analysis; a != nil {
			if a.MinSampleSize < 1 {
				return nil, badf("analysis.min_sample_size must be at least 1")
			}
			iv := time.Duration(a.Interval)
			if iv <= 0 || iv > maxInterval {
				return nil, badf("analysis.interval %v: want (0, %v]", iv, maxInterval)
			}
			c.Analysis = &strategy.AnalysisConfig{
				ErrorRateThreshold:  a.ErrorRateThreshold,
				LatencyP99Threshold: time.Duration(a.LatencyP99Threshold),
				MinSampleSize:       a.MinSampleSize,
				Interval:            iv,
			}
		}
	}
	if err := strategy.ValidateConfig(c); err != nil {
		return nil, badf("strategy: %v", err)
	}
	return c, nil
}

// Deploy validates req, records it and starts it.
func (s *Service) Deploy(req *DeployRequest) (*Deployment, int, error) {
	if !s.cfg.AuthEnabled {
		return nil, http.StatusForbidden, errors.New("deployments need AUTH_ENABLED=true")
	}
	if s.history == nil {
		return nil, http.StatusServiceUnavailable, statefile.ErrNoDir
	}
	if s.cfg.Driver == nil {
		return nil, http.StatusServiceUnavailable, errors.New("no Cuirass driver (CUIRASS_URL, CUIRASS_API_KEY)")
	}
	cfg, err := req.validate(s.cfg.Analysis != nil)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	// The driver checks labels and the proxy; ask it now, not mid-deploy.
	if _, err := s.cfg.Driver.Callbacks(cuirassdriver.ServiceSpec{Service: req.Service, Health: req.Health, Proxy: req.Proxy, Labels: req.Labels}, "check", req.Version); err != nil {
		return nil, http.StatusBadRequest, err
	}

	s.mu.Lock()
	if other, busy := s.busy[req.Service]; busy {
		s.mu.Unlock()
		return nil, http.StatusConflict, fmt.Errorf("%s is busy with %s", req.Service, other)
	}
	if len(s.deps) >= maxHistory {
		if s.history.Mode() != "well" || !s.forgetOldestLocked() {
			s.mu.Unlock()
			return nil, http.StatusInsufficientStorage, fmt.Errorf("history holds %d records; prune %s", maxHistory, stateFileName)
		}
	}
	s.counter++
	d := &Deployment{
		ID:        fmt.Sprintf("d%d-%d", time.Now().UnixNano(), s.counter),
		Service:   req.Service,
		Version:   req.Version,
		Replicas:  req.Replicas,
		Strategy:  req.Strategy,
		Health:    req.Health,
		Proxy:     req.Proxy,
		Labels:    req.Labels,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
	}
	s.deps = append(s.deps, d)
	s.byID[d.ID] = d
	ctx, cancel := context.WithTimeout(context.Background(), historyTimeout)
	err = s.history.Record(ctx, d, s.deps)
	cancel()
	if err != nil {
		s.deps = s.deps[:len(s.deps)-1]
		delete(s.byID, d.ID)
		s.mu.Unlock()
		return nil, http.StatusServiceUnavailable, fmt.Errorf("persist: %w", err)
	}
	s.busy[req.Service] = d.ID
	out := copyDeployment(d)
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run(d.ID, cfg)
	return out, http.StatusAccepted, nil
}

// forgetOldestLocked drops the oldest final, idle record from memory (it
// stays in the Well) so a Well-backed Sword keeps its newest maxHistory
// records. The caller holds mu.
func (s *Service) forgetOldestLocked() bool {
	for i, d := range s.deps {
		if d.final() && s.busy[d.Service] != d.ID {
			delete(s.byID, d.ID)
			s.deps = append(s.deps[:i:i], s.deps[i+1:]...)
			return true
		}
	}
	return false
}

func (s *Service) release(service, id string) {
	s.mu.Lock()
	if s.busy[service] == id {
		delete(s.busy, service)
	}
	s.mu.Unlock()
}

// majorityVersion is the version most instances run, or "".
func majorityVersion(insts []strategy.Instance) string {
	count := map[string]int{}
	best := ""
	for _, in := range insts {
		count[in.Version]++
		if count[in.Version] > count[best] {
			best = in.Version
		}
	}
	return best
}

func (s *Service) run(id string, cfg *strategy.Config) {
	defer s.wg.Done()
	d, _ := s.Get(id)
	defer s.release(d.Service, id)
	ctx := s.ctx

	now := time.Now().UTC()
	s.update(id, func(r *Deployment) { r.Status, r.StartedAt = StatusInProgress, &now })
	s.publish(TopicStarted, d, "")

	fail := func(msg string) {
		s.finishFailed(id, d, msg)
	}

	insts, err := s.cfg.Driver.Instances(ctx, d.Service)
	if err != nil {
		fail("cannot read the service's instances from Cuirass: " + err.Error())
		return
	}
	prev := majorityVersion(insts)
	s.update(id, func(r *Deployment) { r.PreviousVersion = prev })

	cb, err := s.cfg.Driver.Callbacks(d.spec(), id, d.Version)
	if err != nil {
		fail(err.Error())
		return
	}
	if s.cfg.Analysis != nil {
		port := d.Health.Port
		cb.OnAnalysis = func(ctx context.Context, req *strategy.AnalysisRequest) (*strategy.AnalysisResult, error) {
			return s.cfg.Analysis.Analyze(ctx, req, port)
		}
	}
	var lastPhase string
	cb.OnProgress = func(_ context.Context, p *strategy.Progress) {
		if p == nil || p.Phase == lastPhase {
			return
		}
		lastPhase = p.Phase
		s.publish(TopicProgress, d, p.Phase+": "+p.Message)
	}
	cfg.HealthCheck.Interval = s.cfg.PollInterval

	strat, err := strategy.GetStrategy(cfg.Type)
	if err != nil {
		fail(err.Error())
		return
	}
	res, err := strat.Execute(ctx, &strategy.ExecuteParams{
		DeploymentID:   id,
		ServiceName:    d.Service,
		CurrentVersion: prev,
		TargetVersion:  d.Version,
		Replicas:       d.Replicas,
		Config:         cfg,
		Instances:      insts,
		Callbacks:      cb,
	})
	msg := ""
	if res != nil {
		msg = res.Message
	}
	if err != nil && msg == "" {
		msg = err.Error()
	}
	switch {
	case err == nil && res != nil && res.Success:
		done := time.Now().UTC()
		s.update(id, func(r *Deployment) {
			r.Status, r.Message, r.CompletedAt, r.Analysis = StatusSucceeded, msg, &done, res.Analysis
		})
		s.deploysC.WithLabelValues(string(cfg.Type), "succeeded").Inc()
		s.publish(TopicSucceeded, d, msg)
	case res != nil && res.RolledBack:
		reason := ReasonDeployFailed
		if res.Analysis != nil && (!res.Analysis.Pass || res.Analysis.ShouldRollback) {
			reason = ReasonAnalysisFailed
		}
		done := time.Now().UTC()
		s.update(id, func(r *Deployment) {
			r.Status, r.Reason, r.Message, r.CompletedAt, r.Analysis, r.RolledBackTo = StatusRolledBack, reason, msg, &done, res.Analysis, prev
		})
		s.deploysC.WithLabelValues(string(cfg.Type), "rolled_back").Inc()
		s.rollbackC.WithLabelValues(reason).Inc()
		s.publish(TopicRolledBack, d, reason+": "+msg)
	default:
		fail(msg)
	}
}

// finishFailed records a failed deployment and, unless turned off, rolls
// the service back to the previous success.
func (s *Service) finishFailed(id string, d *Deployment, msg string) {
	done := time.Now().UTC()
	s.update(id, func(r *Deployment) { r.Status, r.Message, r.CompletedAt = StatusFailed, msg, &done })
	s.deploysC.WithLabelValues(string(d.Strategy.Type), "failed").Inc()
	s.publish(TopicFailed, d, msg)
	if s.cfg.NoAutoRollback || s.rollbacks == nil {
		return
	}
	if _, err := (&lookup{s: s}).GetPrevious(s.ctx, d.Service, id); err != nil {
		return // nothing to go back to: the failure stands
	}
	s.rollbackTo(id, d, ReasonDeployFailed, rollback.RollbackReasonAutomatic)
}

// rollbackTo runs rollback.Manager for record id and records the outcome.
func (s *Service) rollbackTo(id string, d *Deployment, reason string, why rollback.RollbackReason) {
	prevStatus := StatusFailed
	if cur, ok := s.Get(id); ok {
		prevStatus = cur.Status
	}
	s.update(id, func(r *Deployment) { r.Status = StatusRollingBack })
	rb, err := s.rollbacks.Rollback(s.ctx, &rollback.RollbackRequest{DeploymentID: id, Reason: why, InitiatedBy: "sword"})
	if err == nil {
		rb, err = s.rollbacks.Wait(s.ctx, rb.ID)
	}
	done := time.Now().UTC()
	if err != nil || rb == nil || rb.State != rollback.RollbackStateCompleted {
		cause := ""
		switch {
		case err != nil:
			cause = err.Error()
		case rb != nil:
			cause = rb.Error
		}
		s.update(id, func(r *Deployment) {
			r.Status, r.CompletedAt = prevStatus, &done
			r.Message = strings.TrimSpace(r.Message + "; rollback failed: " + cause)
		})
		s.publish(TopicFailed, d, "rollback failed: "+cause)
		return
	}
	s.update(id, func(r *Deployment) {
		r.Status, r.Reason, r.RolledBackTo, r.CompletedAt = StatusRolledBack, reason, rb.ToVersion, &done
	})
	s.rollbackC.WithLabelValues(reason).Inc()
	s.publish(TopicRolledBack, d, fmt.Sprintf("%s: back on %s", reason, rb.ToVersion))
}

// Rollback starts a manual rollback of record id to the previous success.
func (s *Service) Rollback(id string) (*Deployment, int, error) {
	if !s.cfg.AuthEnabled {
		return nil, http.StatusForbidden, errors.New("rollbacks need AUTH_ENABLED=true")
	}
	if s.history == nil {
		return nil, http.StatusServiceUnavailable, statefile.ErrNoDir
	}
	if s.rollbacks == nil {
		return nil, http.StatusServiceUnavailable, errors.New("no Cuirass driver")
	}
	s.mu.Lock()
	d, ok := s.byID[id]
	if !ok {
		s.mu.Unlock()
		return nil, http.StatusNotFound, fmt.Errorf("no deployment %s", id)
	}
	if other, busy := s.busy[d.Service]; busy {
		s.mu.Unlock()
		return nil, http.StatusConflict, fmt.Errorf("%s is busy with %s", d.Service, other)
	}
	if d.Status == StatusRolledBack {
		s.mu.Unlock()
		return nil, http.StatusConflict, fmt.Errorf("%s is already rolled back", id)
	}
	s.mu.Unlock()
	if _, err := (&lookup{s: s}).GetPrevious(s.ctx, d.Service, id); err != nil {
		return nil, http.StatusConflict, err
	}
	s.mu.Lock()
	if _, busy := s.busy[d.Service]; busy {
		s.mu.Unlock()
		return nil, http.StatusConflict, fmt.Errorf("%s became busy", d.Service)
	}
	s.busy[d.Service] = id
	cp := copyDeployment(d)
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.release(cp.Service, id)
		s.rollbackTo(id, cp, ReasonManual, rollback.RollbackReasonManual)
	}()
	cp.Status = StatusRollingBack
	return cp, http.StatusAccepted, nil
}

// specFor is the newest record's spec for a service (rollback adapters).
func (s *Service) specFor(service string) (cuirassdriver.ServiceSpec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.deps) - 1; i >= 0; i-- {
		if s.deps[i].Service == service && s.deps[i].Health != nil {
			return s.deps[i].spec(), nil
		}
	}
	return cuirassdriver.ServiceSpec{}, fmt.Errorf("no deployment record for %s", service)
}

// lookup implements rollback.DeploymentLookup over the history.
type lookup struct{ s *Service }

func info(d *Deployment) *rollback.DeploymentInfo {
	return &rollback.DeploymentInfo{ID: d.ID, ServiceName: d.Service, Version: d.Version,
		ArtifactRef: d.Service + "-" + d.Version, State: string(d.Status), Replicas: d.Replicas}
}

func (l *lookup) Get(_ context.Context, id string) (*rollback.DeploymentInfo, error) {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	d, ok := l.s.byID[id]
	if !ok {
		return nil, rollback.ErrDeploymentNotFound
	}
	return info(d), nil
}

// GetPrevious is the newest succeeded deployment of service, of another
// version, recorded before beforeID.
func (l *lookup) GetPrevious(_ context.Context, service, beforeID string) (*rollback.DeploymentInfo, error) {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	before, ok := l.s.byID[beforeID]
	if !ok {
		return nil, rollback.ErrDeploymentNotFound
	}
	idx := -1
	for i, d := range l.s.deps {
		if d.ID == beforeID {
			idx = i
		}
	}
	for i := idx - 1; i >= 0; i-- {
		d := l.s.deps[i]
		if d.Service == service && d.Status == StatusSucceeded && d.Version != before.Version {
			return info(d), nil
		}
	}
	return nil, fmt.Errorf("%w: no earlier successful deployment of %s", rollback.ErrNoRollbackTarget, service)
}

func (l *lookup) GetByService(_ context.Context, service string, limit int) ([]*rollback.DeploymentInfo, error) {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	var out []*rollback.DeploymentInfo
	for i := len(l.s.deps) - 1; i >= 0 && (limit <= 0 || len(out) < limit); i-- {
		if l.s.deps[i].Service == service {
			out = append(out, info(l.s.deps[i]))
		}
	}
	return out, nil
}

// --- events ---

func (s *Service) publish(topic string, d *Deployment, msg string) {
	if s.cfg.Publisher == nil {
		return
	}
	cur, ok := s.Get(d.ID)
	if !ok {
		cur = d
	}
	payload, err := json.Marshal(map[string]any{
		"deployment_id": cur.ID,
		"service":       cur.Service,
		"version":       cur.Version,
		"strategy":      cur.Strategy.Type,
		"status":        cur.Status,
		"reason":        cur.Reason,
		"message":       msg,
		"timestamp":     time.Now().UnixMilli(),
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ctx), 10*time.Second)
	defer cancel()
	if err := s.cfg.Publisher.Publish(ctx, topic, payload); err != nil {
		s.log.Warn().Err(err).Str("topic", topic).Str("id", d.ID).Msg("sword: event not published")
	}
}

// --- HTTP ---

// Handler serves Sword's routes. The caller adds auth and the house HTTP
// metrics around it.
func (s *Service) Handler() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_ = metrics.GatherAll(w, s.registry, metrics.DefaultRegistry)
	})
	mux.HandleFunc("/api/v1/deployments", s.handleDeployments)
	mux.HandleFunc("/api/v1/deployments/", s.handleDeployment)
	return mux
}

func writeErr(w http.ResponseWriter, code int, err error) {
	httputil.WriteJSON(w, code, map[string]string{"error": err.Error()})
}

func (s *Service) handleDeployments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		httputil.WriteJSON(w, http.StatusOK, s.List())
	case http.MethodPost:
		if !s.cfg.AuthEnabled {
			writeErr(w, http.StatusForbidden, errors.New("deployments need AUTH_ENABLED=true"))
			return
		}
		var req DeployRequest
		if err := httputil.DecodeJSONBody(r, maxRequestBody, &req); err != nil {
			httputil.HandleRequestError(w, err)
			return
		}
		d, code, err := s.Deploy(&req)
		if err != nil {
			writeErr(w, code, err)
			return
		}
		httputil.WriteJSON(w, code, d)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeErr(w, http.StatusMethodNotAllowed, errors.New("GET or POST"))
	}
}

func (s *Service) handleDeployment(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/deployments/")
	if id, ok := strings.CutSuffix(rest, "/rollback"); ok {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeErr(w, http.StatusMethodNotAllowed, errors.New("POST only"))
			return
		}
		d, code, err := s.Rollback(id)
		if err != nil {
			writeErr(w, code, err)
			return
		}
		httputil.WriteJSON(w, code, d)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeErr(w, http.StatusMethodNotAllowed, errors.New("GET only"))
		return
	}
	d, ok := s.Get(rest)
	if !ok {
		writeErr(w, http.StatusNotFound, fmt.Errorf("no deployment %q", rest))
		return
	}
	httputil.WriteJSON(w, http.StatusOK, d)
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "sword"})
}

// handleReady: ready means a deployment could start now: the history
// takes a write (the data dir, or The Well answers) and Cuirass answers.
func (s *Service) handleReady(w http.ResponseWriter, r *http.Request) {
	var problems []string
	if s.history == nil {
		problems = append(problems, "no SWORD_DATA_DIR or WELL_DB (read-only)")
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		err := s.history.Ready(ctx)
		cancel()
		if err != nil {
			problems = append(problems, "history ("+s.history.Mode()+") not writable: "+err.Error())
		}
	}
	if s.cfg.Driver == nil {
		problems = append(problems, "no Cuirass driver")
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.cfg.Driver.Ping(ctx); err != nil {
			problems = append(problems, "Cuirass: "+err.Error())
		}
	}
	if len(problems) > 0 {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"ready": false, "problems": problems})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"ready": true})
}

func probeWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".ready-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}
