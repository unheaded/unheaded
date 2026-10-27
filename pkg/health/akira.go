// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

// Package health implements the Kingdom's immune system — every node
// is a akira that health-checks services and participates in
// consensus-based auto-remediation.
//
// See ADR-029 for the full design.
package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	pqtls "unheaded/pkg/tls"
)

// ConsensusThreshold is the fraction of reporters that must agree
// a service is unhealthy before automatic remediation triggers.
// Two-thirds (Byzantine fault tolerance).
// This is intentionally hardcoded — not configurable.
const ConsensusThreshold = 2.0 / 3.0 // 0.666666...

// MaxAutoRestarts before escalating to human.
const MaxAutoRestarts = 3

// HealthCheckInterval between checks.
const HealthCheckInterval = 30 * time.Second

// ServiceTarget represents a service to health-check.
type ServiceTarget struct {
	Name       string
	Host       string
	Port       int
	HealthPath string // default: /health
	// Scheme is "http" or "https"; empty means pkg/tls.Scheme() (https when
	// UNHEADED_TLS_ENABLED=true). Set "http" for a third-party service that
	// does not serve TLS.
	Scheme string
	// Exec, when set, makes this an exec check (exec_check.go): Host, Port,
	// HealthPath and Scheme are ignored and the command's exit code is the vote.
	Exec *ExecCheck
}

// HealthReport is a single health check result, the payload of every
// system.health.reports message: Akira's checks of other services and each
// service's report on itself (pkg/healthreport) share this one type.
type HealthReport struct {
	Service  string `json:"service"`
	Reporter string `json:"reporter"`
	// Host is the host the reporter runs on (west, east). Akira's own
	// reports leave it empty: their Reporter is already the node.
	Host      string        `json:"host,omitempty"`
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency_ns"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// Akira health-checks services, casts each result as its own vote, and
// tallies the votes of every reporter it hears from (see Ballot).
type Akira struct {
	nodeID  string
	targets []ServiceTarget
	client  *http.Client
	logger  zerolog.Logger

	ballot *Ballot

	mu       sync.RWMutex
	severity map[string]Severity // last severity per service, to report changes
	verdicts map[string]Verdict  // last tally, for GetStates

	execMu   sync.Mutex
	execLast map[string]execRun // last run per exec target, reused within its Interval

	sweeps atomic.Uint64 // completed CheckAll passes; see Sweeps

	// Callbacks
	onReport func(HealthReport) // called for each of this node's checks
	onChange func(Verdict)      // called when a service's severity changes
	onAlert  func(Verdict)      // called while a service is at the remediation threshold
}

// NewAkira creates a new akira for the given node.
func NewAkira(nodeID string, targets []ServiceTarget) *Akira {
	return &Akira{
		nodeID:   nodeID,
		targets:  targets,
		client:   &http.Client{Timeout: 5 * time.Second},
		logger:   log.With().Str("component", "akira").Str("node", nodeID).Logger(),
		ballot:   NewBallot(),
		severity: map[string]Severity{},
		verdicts: map[string]Verdict{},
		execLast: map[string]execRun{},
	}
}

// execRun is an exec target's last run.
type execRun struct {
	at  time.Time
	res ExecResult
}

// SetHTTPClient replaces the checks' client, e.g. with pkg/tls.ClientFromEnv
// so https targets verify against the deployment CA. Call it before the
// first sweep.
func (w *Akira) SetHTTPClient(c *http.Client) {
	if c != nil {
		w.client = c
	}
}

// OnReport sets a callback for each health check result.
func (w *Akira) OnReport(fn func(HealthReport)) {
	w.onReport = fn
}

// OnSeverityChange sets a callback for a service moving between bands.
func (w *Akira) OnSeverityChange(fn func(Verdict)) {
	w.onChange = fn
}

// OnAlert sets a callback for services at the remediation threshold.
func (w *Akira) OnAlert(fn func(Verdict)) {
	w.onAlert = fn
}

// Record casts another reporter's report (e.g. read from Wotan's
// system.health.reports) on the ballot.
func (w *Akira) Record(r HealthReport) {
	w.ballot.Record(r)
}

// CheckService performs a single health check on a service.
func (w *Akira) CheckService(target ServiceTarget) HealthReport {
	if target.Exec != nil {
		return w.checkExec(target)
	}
	path := target.HealthPath
	if path == "" {
		path = "/health"
	}
	scheme := target.Scheme
	if scheme == "" {
		scheme = pqtls.Scheme()
	}
	url := fmt.Sprintf("%s://%s%s", scheme, net.JoinHostPort(target.Host, strconv.Itoa(target.Port)), path)

	start := time.Now()
	resp, err := w.client.Get(url)
	latency := time.Since(start)

	report := HealthReport{
		Service:   target.Name,
		Reporter:  w.nodeID,
		Timestamp: time.Now(),
		Latency:   latency,
	}

	if err != nil {
		report.Healthy = false
		report.Error = err.Error()
	} else {
		resp.Body.Close() // #nosec G104 -- draining and closing a response body; a close error cannot change the outcome
		report.Healthy = resp.StatusCode == http.StatusOK
		if !report.Healthy {
			report.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}

	return report
}

// checkExec runs an exec target when its Interval has passed (or it never
// ran), else re-votes the last result with a fresh timestamp so the vote
// does not expire between runs.
func (w *Akira) checkExec(target ServiceTarget) HealthReport {
	w.execMu.Lock()
	last, ok := w.execLast[target.Name]
	w.execMu.Unlock()
	now := time.Now()
	if !ok || target.Exec.Interval <= 0 || now.Sub(last.at) >= target.Exec.Interval {
		last = execRun{at: now, res: RunExec(context.Background(), *target.Exec)}
		w.execMu.Lock()
		w.execLast[target.Name] = last
		w.execMu.Unlock()
		if !last.res.Healthy() {
			w.logger.Warn().Str("check", target.Name).Int("exit", last.res.ExitCode).
				Bool("timed_out", last.res.TimedOut).Dur("took", last.res.Duration).
				Msg("exec check failing")
		}
	}
	report := HealthReport{
		Service:   target.Name,
		Reporter:  w.nodeID,
		Timestamp: time.Now(),
		Latency:   last.res.Duration,
		Healthy:   last.res.Healthy(),
	}
	if !report.Healthy {
		report.Error = last.res.summary()
	}
	return report
}

// CheckAll performs health checks on all targets.
func (w *Akira) CheckAll() []HealthReport {
	reports := make([]HealthReport, 0, len(w.targets))
	for _, target := range w.targets {
		report := w.CheckService(target)
		reports = append(reports, report)

		if w.onReport != nil {
			w.onReport(report)
		}

		w.ballot.Record(report)
	}
	w.sweeps.Add(1)
	return reports
}

// Sweeps returns how many CheckAll passes have completed. It moves only after
// the last target is checked, so a non-zero value means every target has at
// least one report.
func (w *Akira) Sweeps() uint64 {
	return w.sweeps.Load()
}

// EvaluateConsensus tallies the ballot. It reports each service whose
// severity band changed since the last call, and returns (and reports) the
// services at the remediation threshold.
func (w *Akira) EvaluateConsensus() []Verdict {
	verdicts := w.ballot.Tally(time.Now())

	w.mu.Lock()
	var changed, alerts []Verdict
	current := make(map[string]Verdict, len(verdicts))
	for _, v := range verdicts {
		current[v.Service] = v
		if prev, ok := w.severity[v.Service]; !ok || prev != v.Severity {
			changed = append(changed, v)
		}
		w.severity[v.Service] = v.Severity
		if v.Remediate {
			alerts = append(alerts, v)
		}
	}
	for svc := range w.severity {
		if _, ok := current[svc]; !ok {
			delete(w.severity, svc) // no fresh votes: no verdict
		}
	}
	w.verdicts = current
	w.mu.Unlock()

	for _, v := range changed {
		if w.onChange != nil {
			w.onChange(v)
		}
	}
	for _, v := range alerts {
		if w.onAlert != nil {
			w.onAlert(v)
		}
	}
	return alerts
}

// Run starts the akira loop. Blocks until context is cancelled.
func (w *Akira) Run(ctx context.Context) {
	w.logger.Info().
		Int("targets", len(w.targets)).
		Dur("interval", HealthCheckInterval).
		Float64("threshold", ConsensusThreshold).
		Msg("akira started")

	ticker := time.NewTicker(HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info().Msg("akira stopped")
			return
		case <-ticker.C:
			reports := w.CheckAll()

			healthy := 0
			for _, r := range reports {
				if r.Healthy {
					healthy++
				}
			}

			w.logger.Info().
				Int("healthy", healthy).
				Int("total", len(reports)).
				Msg("health check complete")

			// Evaluate consensus
			alerts := w.EvaluateConsensus()
			for _, alert := range alerts {
				w.logger.Warn().
					Str("service", alert.Service).
					Float64("failure_rate", alert.FailureRate).
					Int("reporters", alert.Reporters).
					Msg("CONSENSUS THRESHOLD — service needs remediation")
			}
		}
	}
}

// GetStates returns the last tally's verdict per service.
func (w *Akira) GetStates() map[string]Verdict {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make(map[string]Verdict, len(w.verdicts))
	for k, v := range w.verdicts {
		result[k] = v
	}
	return result
}
