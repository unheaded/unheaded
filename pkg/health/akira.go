// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

// Package health implements the Kingdom's immune system — every node
// is a akira that health-checks services and participates in
// consensus-based auto-remediation.
//
// See ADR-029 for the full design.
package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
}

// HealthReport is a single health check result.
type HealthReport struct {
	Service   string        `json:"service"`
	Reporter  string        `json:"reporter"`
	Healthy   bool          `json:"healthy"`
	Latency   time.Duration `json:"latency_ns"`
	Error     string        `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// ConsensusState tracks the aggregated health state of a service.
type ConsensusState struct {
	Service          string
	TotalReporters   int
	FailingReporters int
	FailureRate      float64
	AutoRestarts     int
	LastRestart      time.Time
	Reports          []HealthReport
}

// Akira monitors services and publishes health reports.
type Akira struct {
	nodeID  string
	targets []ServiceTarget
	client  *http.Client
	logger  zerolog.Logger

	mu      sync.RWMutex
	states  map[string]*ConsensusState // service name → state
	reports []HealthReport             // recent reports buffer

	// Callbacks
	onReport func(HealthReport)   // called for each health check
	onAlert  func(ConsensusState) // called when consensus threshold hit
}

// NewAkira creates a new akira for the given node.
func NewAkira(nodeID string, targets []ServiceTarget) *Akira {
	return &Akira{
		nodeID:  nodeID,
		targets: targets,
		client:  &http.Client{Timeout: 5 * time.Second},
		logger:  log.With().Str("component", "akira").Str("node", nodeID).Logger(),
		states:  make(map[string]*ConsensusState),
	}
}

// OnReport sets a callback for each health check result.
func (w *Akira) OnReport(fn func(HealthReport)) {
	w.onReport = fn
}

// OnAlert sets a callback for consensus threshold triggers.
func (w *Akira) OnAlert(fn func(ConsensusState)) {
	w.onAlert = fn
}

// CheckService performs a single health check on a service.
func (w *Akira) CheckService(target ServiceTarget) HealthReport {
	path := target.HealthPath
	if path == "" {
		path = "/health"
	}
	url := fmt.Sprintf("http://%s:%d%s", target.Host, target.Port, path)

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
		resp.Body.Close()
		report.Healthy = resp.StatusCode == http.StatusOK
		if !report.Healthy {
			report.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
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

		// Update consensus state
		w.mu.Lock()
		state, ok := w.states[target.Name]
		if !ok {
			state = &ConsensusState{Service: target.Name}
			w.states[target.Name] = state
		}
		state.Reports = append(state.Reports, report)
		// Keep only last 10 reports
		if len(state.Reports) > 10 {
			state.Reports = state.Reports[len(state.Reports)-10:]
		}
		w.mu.Unlock()
	}
	return reports
}

// EvaluateConsensus checks if any service has crossed the failure threshold.
func (w *Akira) EvaluateConsensus() []ConsensusState {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var alerts []ConsensusState
	for _, state := range w.states {
		if len(state.Reports) == 0 {
			continue
		}

		// Count recent failures (last 5 reports)
		recent := state.Reports
		if len(recent) > 5 {
			recent = recent[len(recent)-5:]
		}

		failing := 0
		for _, r := range recent {
			if !r.Healthy {
				failing++
			}
		}

		state.TotalReporters = len(recent)
		state.FailingReporters = failing
		state.FailureRate = float64(failing) / float64(len(recent))

		if state.FailureRate >= ConsensusThreshold {
			alerts = append(alerts, *state)
			if w.onAlert != nil {
				w.onAlert(*state)
			}
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
					Int("restarts", alert.AutoRestarts).
					Msg("CONSENSUS THRESHOLD — service needs remediation")
			}
		}
	}
}

// GetStates returns current consensus states for all services.
func (w *Akira) GetStates() map[string]ConsensusState {
	w.mu.RLock()
	defer w.mu.RUnlock()

	result := make(map[string]ConsensusState, len(w.states))
	for k, v := range w.states {
		result[k] = *v
	}
	return result
}
