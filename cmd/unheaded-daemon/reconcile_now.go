// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// POST /api/v1/reconcile runs one sync, plan and apply cycle now and
// answers with its actions (S-REAL-STEEL Phase 7). A deploy cutover
// would otherwise wait for the next 30 s tick. It is a write: refused (403)
// unless AUTH_ENABLED=true, like PUT and DELETE. At most one run a second;
// more answer 429 with Retry-After. It obeys the same modes as the tick:
// observe mode plans and takes nothing.

const reconcileNowMinGap = time.Second

func (d *Daemon) handleReconcileNow(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "POST only"})
		return
	}
	if !d.authEnabled {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "reconcile needs AUTH_ENABLED=true"})
		return
	}
	d.reconcileNowMu.Lock()
	if since := time.Since(d.lastReconcileNow); since < reconcileNowMinGap {
		d.reconcileNowMu.Unlock()
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "one reconcile a second"})
		return
	}
	d.lastReconcileNow = time.Now()
	d.reconcileNowMu.Unlock()

	actions, err := d.reconcileOnce()
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if actions == nil {
		actions = []ReconcileAction{}
	}
	d.log.Info().Int("actions", len(actions)).Msg("Reconcile run on request")
	_ = json.NewEncoder(w).Encode(map[string]any{"actions": actions})
}

// proxyRemovalsFirst moves every update-proxy that removes a device ahead
// of the other actions, keeping their order otherwise. Moving the stable
// endpoint is a removal on one instance and an add on another; LXD cannot
// bind the listen address twice, so the removal must land first whichever
// instance name sorts first.
func proxyRemovalsFirst(actions []ReconcileAction) []ReconcileAction {
	removal := func(a ReconcileAction) bool {
		return a.Action == "update-proxy" && a.spec != nil && a.spec.Proxy == nil
	}
	out := make([]ReconcileAction, 0, len(actions))
	for _, first := range []bool{true, false} {
		for _, a := range actions {
			if removal(a) == first {
				out = append(out, a)
			}
		}
	}
	return out
}
