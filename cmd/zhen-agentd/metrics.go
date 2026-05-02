// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"unheaded/pkg/champion"
)

// Prometheus metrics for zhen-agentd. promauto handles registration so
// the /metrics endpoint sees these without additional plumbing.
var (
	httpRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "zhen_agentd",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests by endpoint and status code.",
		},
		[]string{"endpoint", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "zhen_agentd",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration by endpoint.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"endpoint"},
	)

	agentRunsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "zhen_agentd",
			Name:      "agent_runs_total",
			Help:      "Agent loop completions by outcome (answer, budget_hit).",
		},
		[]string{"outcome"},
	)

	agentTurns = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "zhen_agentd",
			Name:      "agent_turns_used",
			Help:      "Number of turns consumed per agent run.",
			Buckets:   []float64{1, 2, 3, 4, 5, 6, 7, 8, 12, 16},
		},
	)

	championActionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "zhen_agentd",
			Name:      "champion_actions_total",
			Help:      "Champion ActionStore writes by action_type and final status (gate outcome).",
		},
		[]string{"action_type", "status"},
	)

	confirmTokensIssued = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "zhen_agentd",
			Name:      "confirm_tokens_issued_total",
			Help:      "Pending-confirmation tokens issued.",
		},
	)

	confirmTokensRedeemed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "zhen_agentd",
			Name:      "confirm_tokens_redeemed_total",
			Help:      "Pending-confirmation tokens redeemed by outcome (ok, expired, unknown, used, denied).",
		},
		[]string{"outcome"},
	)

	rateLimitedRequests = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "zhen_agentd",
			Name:      "rate_limited_requests_total",
			Help:      "Requests rejected by the per-IP rate limiter (HTTP 429).",
		},
	)
)

// instrument wraps an http.Handler with metrics — request counter +
// duration histogram, labeled by route. Status code is captured via
// a small ResponseWriter shim.
func instrument(route string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		shim := &statusShim{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(shim, r)
		httpRequests.WithLabelValues(route, strconv.Itoa(shim.status)).Inc()
		httpRequestDuration.WithLabelValues(route).Observe(time.Since(start).Seconds())
	})
}

type statusShim struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusShim) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
		s.ResponseWriter.WriteHeader(code)
	}
}

func (s *statusShim) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.status = http.StatusOK
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

// metricsActionStore decorates a champion.ActionStore — every
// UpdateAction increments the championActionsTotal counter labeled
// by action_type and final status. The action_type is captured at
// LogAction time and stashed in a small per-id map; UpdateAction
// looks it up to label correctly.
type metricsActionStore struct {
	inner champion.ActionStore

	// types maps action.ID → ActionType so UpdateAction can label
	// without rereading the row. Capped to bound memory growth in
	// long-running daemons; eldest-first eviction is good enough.
	types       map[int64]string
	maxRecorded int
}

func newMetricsActionStore(inner champion.ActionStore) *metricsActionStore {
	return &metricsActionStore{
		inner:       inner,
		types:       make(map[int64]string),
		maxRecorded: 4096,
	}
}

func (m *metricsActionStore) LogAction(ctx context.Context, action *champion.Action) (int64, error) {
	id, err := m.inner.LogAction(ctx, action)
	if err == nil && id > 0 {
		// Bound memory: evict roughly half when the cache fills up.
		if len(m.types) >= m.maxRecorded {
			i := 0
			for k := range m.types {
				if i > m.maxRecorded/2 {
					break
				}
				delete(m.types, k)
				i++
			}
		}
		m.types[id] = action.ActionType
	}
	return id, err
}

func (m *metricsActionStore) UpdateAction(ctx context.Context, id int64, status, result, errMsg string, elapsedMs int) error {
	err := m.inner.UpdateAction(ctx, id, status, result, errMsg, elapsedMs)
	if at, ok := m.types[id]; ok {
		championActionsTotal.WithLabelValues(at, status).Inc()
		// Don't grow the map indefinitely.
		delete(m.types, id)
	} else {
		championActionsTotal.WithLabelValues("unknown", status).Inc()
	}
	return err
}

func (m *metricsActionStore) GetActions(ctx context.Context, sessionID string, limit int) ([]champion.Action, error) {
	return m.inner.GetActions(ctx, sessionID, limit)
}
