// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"errors"

	"unheaded/pkg/champion"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// ADR-060 observability (S-CHAMPION-HARDENING Step 216): every model_switch
// the Champion runs, from /api/v1/tool/exec or a confirmed token.
var (
	modelSwapDuration = auto.NewHistogramVec(
		prom.HistogramOpts{
			Name:    "zhen_model_swap_duration_seconds",
			Help:    "model_switch wall time by requested key and outcome (ok, failed, rolled_back).",
			Buckets: []float64{1, 5, 15, 30, 45, 60, 90, 120, 180, 300, 600, 900},
		},
		[]string{"key", "outcome"},
	)
	modelSwapFailures = auto.NewCounterVec(
		prom.CounterOpts{
			Name: "zhen_model_swap_failures_total",
			Help: "model_switch failures by reason: timeout (rolled back or not), refused (unknown key, swap in progress, script changed, root), error (the script failed).",
		},
		[]string{"reason"},
	)
)

// observeModelSwap is the Champion's swap observer.
func observeModelSwap(ev champion.ModelSwapEvent) {
	modelSwapDuration.WithLabelValues(ev.Key, ev.Outcome).Observe(ev.Duration.Seconds())
	if ev.Err == nil {
		return
	}
	modelSwapFailures.WithLabelValues(swapFailureReason(ev.Err)).Inc()
}

func swapFailureReason(err error) string {
	switch {
	case errors.Is(err, champion.ErrSwapTimeout):
		return "timeout"
	case errors.Is(err, champion.ErrUnknownModel), errors.Is(err, champion.ErrSwapInProgress),
		errors.Is(err, champion.ErrScriptModified), errors.Is(err, champion.ErrPrivilegeEscalation):
		return "refused"
	}
	return "error"
}
