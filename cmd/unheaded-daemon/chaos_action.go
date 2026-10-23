// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	"unicode"

	"unheaded/pkg/auth"
	"unheaded/pkg/httputil"
)

// Out-of-band chaos stop/kill (S-REAL-STEEL Phase 4), for game days:
//
//	POST /api/v1/containers/{id}/chaos  {"action":"stop"|"kill","reason":"..."}
//
// Refused (403) unless AUTH_ENABLED=true and CUIRASS_CHAOS=true (off by
// default, on only for a game-day window); refused (409) unless the instance
// is Unheaded's and carries the label chaos=allowed, checked twice: by the
// handler against the reconciler's view and by the LXD client against a fresh
// read. stop is a clean stop, kill a forced one. The reconciler is not told:
// in enforce mode its next sync sees the instance stopped and starts it
// again, which is the auto-revert. Every call, allowed or refused, is logged
// once and published on cuirass.chaos.

// TopicCuirassChaos carries one event per chaos request.
const TopicCuirassChaos = "cuirass.chaos"

const (
	chaosLabel       = "chaos"
	chaosLabelValue  = "allowed"
	chaosMaxBody     = 4 << 10
	chaosMaxReason   = 256
	chaosLXDDeadline = 60 * time.Second
)

var errChaosNotAllowed = errors.New("instance does not carry label chaos=allowed; refusing chaos")

type chaosRequest struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func (r *chaosRequest) validate() error {
	if r.Action != "stop" && r.Action != "kill" {
		return fmt.Errorf("action must be stop or kill, got %q", r.Action)
	}
	if r.Reason == "" || len(r.Reason) > chaosMaxReason {
		return fmt.Errorf("reason is required, at most %d bytes", chaosMaxReason)
	}
	for _, c := range r.Reason {
		if !unicode.IsPrint(c) {
			return errors.New("reason must be printable text")
		}
	}
	return nil
}

// ChaosStop stops (force=false) or kills (force=true) a managed instance that
// carries chaos=allowed. It re-reads the instance and checks both itself,
// independent of the handler.
func (c *lxdREST) ChaosStop(ctx context.Context, name string, force bool) error {
	in, err := c.requireManaged(ctx, name)
	if err != nil {
		return err
	}
	if in.Config[lxdLabelPrefix+chaosLabel] != chaosLabelValue {
		return fmt.Errorf("%s: %w", name, errChaosNotAllowed)
	}
	return c.setState(ctx, name, "stop", force)
}

// ChaosStop on the mock: the same two refusals, then stopped.
func (m *MockLXDClient) ChaosStop(_ context.Context, name string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.containers[name]
	if !ok {
		return fmt.Errorf("container %s: %w", name, errLXDNotFound)
	}
	if !c.Managed {
		return fmt.Errorf("%s: %w", name, errLXDNotManaged)
	}
	if c.Labels[chaosLabel] != chaosLabelValue {
		return fmt.Errorf("%s: %w", name, errChaosNotAllowed)
	}
	c.Status, c.Health = "stopped", "unknown"
	m.log.Info().Str("id", name).Bool("force", force).Msg("MockLXD: chaos stop")
	return nil
}

func (d *Daemon) handleChaos(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSONStatus(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
		return
	}
	caller := ""
	if ident := auth.IdentityFromContext(r.Context()); ident != nil {
		caller = ident.Subject
	}
	var req chaosRequest
	status, result, msg := d.chaos(r, id, &req)
	d.log.Info().Str("id", id).Str("action", req.Action).Str("reason", req.Reason).Str("caller", caller).
		Str("result", result).Int("status", status).Str("detail", msg).Msg("Chaos request")
	d.publishChaos(id, req, caller, result, msg)
	body := map[string]string{"id": id, "action": req.Action, "result": result}
	if msg != "" {
		body["detail"] = msg
	}
	writeJSONStatus(w, status, body)
}

// chaos decides and, when allowed, acts. result is "done", "refused" or
// "failed".
func (d *Daemon) chaos(r *http.Request, id string, req *chaosRequest) (int, string, string) {
	// Read the (bounded) body first so even a refusal records what was asked.
	decodeErr := httputil.DecodeJSONBody(r, chaosMaxBody, req)
	if !d.authEnabled {
		return http.StatusForbidden, "refused", "chaos needs AUTH_ENABLED=true"
	}
	if !d.config.Chaos {
		return http.StatusForbidden, "refused", "chaos is off (CUIRASS_CHAOS is not true)"
	}
	if !d.config.Enforce {
		// The revert is the reconciler starting the instance again; in
		// observe mode nothing would, so a kill would stick.
		return http.StatusForbidden, "refused", "chaos needs CUIRASS_ENFORCE=true (nothing would restart the instance)"
	}
	if err := decodeErr; err != nil {
		var re *httputil.RequestError
		if errors.As(err, &re) {
			return re.Status, "refused", re.Message
		}
		return http.StatusBadRequest, "refused", err.Error()
	}
	if err := validLXDName(id); err != nil {
		return http.StatusBadRequest, "refused", err.Error()
	}
	if err := req.validate(); err != nil {
		return http.StatusBadRequest, "refused", err.Error()
	}

	// First check: the reconciler's view.
	d.stateManager.mu.RLock()
	actual, ok := d.stateManager.actual[id]
	managed, allowed := false, false
	if ok {
		managed, allowed = actual.Managed, actual.Labels[chaosLabel] == chaosLabelValue
	}
	d.stateManager.mu.RUnlock()
	switch {
	case !ok:
		return http.StatusNotFound, "refused", "no such instance"
	case !managed:
		return http.StatusConflict, "refused", errLXDNotManaged.Error()
	case !allowed:
		return http.StatusConflict, "refused", errChaosNotAllowed.Error()
	}

	// Second check, and the act: the LXD client against a fresh read.
	ctx, cancel := context.WithTimeout(r.Context(), chaosLXDDeadline)
	defer cancel()
	err := d.lxdClient.ChaosStop(ctx, id, req.Action == "kill")
	switch {
	case err == nil:
		return http.StatusOK, "done", ""
	case errors.Is(err, errLXDNotManaged), errors.Is(err, errChaosNotAllowed):
		return http.StatusConflict, "refused", err.Error()
	case errors.Is(err, errLXDNotFound):
		return http.StatusNotFound, "refused", err.Error()
	default:
		return http.StatusBadGateway, "failed", err.Error()
	}
}

// publishChaos sends one cuirass.chaos event. Best effort: a chaos event is a
// control message and is never buffered for later (wotan-client D-10).
func (d *Daemon) publishChaos(id string, req chaosRequest, caller, result, detail string) {
	payload, err := json.Marshal(map[string]any{
		"node_id": d.config.NodeID, "timestamp": time.Now().UnixMilli(), "component": "cuirass",
		"id": id, "action": req.Action, "reason": req.Reason, "caller": caller,
		"result": result, "detail": detail,
	})
	if err != nil {
		d.log.Error().Err(err).Msg("Chaos event not marshalled")
		return
	}
	publish := d.chaosPublish
	if publish == nil {
		publish = d.publishToWotan
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := publish(ctx, payload); err != nil {
		d.log.Warn().Err(err).Str("id", id).Msg("Chaos event not published to Wotan")
	}
}

// publishToWotan publishes a chaos event if the Wotan client is ready.
func (d *Daemon) publishToWotan(ctx context.Context, payload []byte) error {
	d.wotanMu.RLock()
	client, ready := d.wotanClient, d.wotanReady
	d.wotanMu.RUnlock()
	if !ready || client == nil {
		return errors.New("wotan not ready")
	}
	return client.Publish(ctx, TopicCuirassChaos, payload)
}

func writeJSONStatus(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
