// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/logger"
)

// The container-kill injector (S-REAL-STEEL Step 297). It drives Cuirass's
// out-of-band chaos endpoint, which only works when Cuirass is enforcing
// desired state (so the reconciler restarts the instance: that restart is
// the revert). There is deliberately no start backdoor around Cuirass. If
// the instance is not back running and healthy within the MTTR budget, the
// experiment records a High finding and the journal entry stays
// (chaos.revert_failed, D-19).

// defaultMTTR is the budget for an instance to come back running + healthy.
const defaultMTTR = 90 * time.Second

// ContainerController is Cuirass, as the injector needs it.
type ContainerController interface {
	// Modes reports whether Cuirass is enforcing and whether chaos is on.
	Modes(ctx context.Context) (enforce, chaos bool, err error)
	// Chaos stops ("stop") or kills ("kill") an instance out of band.
	Chaos(ctx context.Context, id, action, reason string) error
	// Healthy reports whether the instance is running and healthy.
	Healthy(ctx context.Context, id string) (bool, error)
}

// ContainerInjector kills a managed instance and waits for Cuirass to heal it.
type ContainerInjector struct {
	cuirass ContainerController
	action  string // "kill" or "stop"
	mttr    time.Duration
	poll    time.Duration
	log     *logger.Logger
	now     func() time.Time
}

// NewContainerInjector builds the injector. action is "kill" or "stop"; mttr
// defaults to 90s.
func NewContainerInjector(c ContainerController, action string, mttr time.Duration, log *logger.Logger) *ContainerInjector {
	if action != "stop" && action != "kill" {
		action = "kill"
	}
	if mttr <= 0 {
		mttr = defaultMTTR
	}
	if log == nil {
		log = logger.New(io.Discard)
	}
	return &ContainerInjector{cuirass: c, action: action, mttr: mttr, poll: 2 * time.Second, log: log, now: time.Now}
}

// RevertBudget gives the revert the whole MTTR window plus a poll and grace,
// so the shared revert timeout does not cut recovery short.
func (n *ContainerInjector) RevertBudget() time.Duration { return n.mttr + n.poll + 10*time.Second }

// Plan enforces the precondition: Cuirass must be enforcing and have chaos
// on, or nothing is injected (no self-heal means no revert).
func (n *ContainerInjector) Plan(ctx context.Context, t ResolvedTarget, _ Params) (Recipe, error) {
	if t.Kind != KindLXD || t.ID == "" {
		return nil, fmt.Errorf("container kill needs an lxd target, got %q/%q", t.Kind, t.ID)
	}
	enforce, chaos, err := n.cuirass.Modes(ctx)
	if err != nil {
		return nil, fmt.Errorf("container kill: cannot read Cuirass modes: %w", err)
	}
	if !enforce {
		return nil, errors.New("container kill: Cuirass enforce is off; nothing would restart the instance")
	}
	if !chaos {
		return nil, errors.New("container kill: Cuirass chaos is off (CUIRASS_CHAOS)")
	}
	return Recipe{"id": t.ID, "action": n.action, "injected_at": strconv.FormatInt(n.now().UnixNano(), 10)}, nil
}

// Inject asks Cuirass to stop or kill the instance.
func (n *ContainerInjector) Inject(ctx context.Context, t ResolvedTarget, _ Params, r Recipe) error {
	id := r["id"]
	if id == "" {
		return errors.New("container kill: recipe has no instance id")
	}
	reason := fmt.Sprintf("S-REAL-STEEL chaos %s", r["action"])
	if err := n.cuirass.Chaos(ctx, id, r["action"], reason); err != nil {
		return fmt.Errorf("container kill: %w", err)
	}
	return nil
}

// Revert waits for Cuirass to bring the instance back running and healthy
// (the reconciler does this in enforce mode), up to the MTTR budget.
func (n *ContainerInjector) Revert(ctx context.Context, r Recipe) error {
	id := r["id"]
	if id == "" {
		return errors.New("container kill: recipe has no instance id")
	}
	deadline := n.now().Add(n.mttr)
	tick := time.NewTicker(n.poll)
	defer tick.Stop()
	for {
		ok, err := n.cuirass.Healthy(ctx, id)
		if ok {
			n.recordMTTR(r, id)
			return nil
		}
		if err != nil {
			n.log.Warn().Err(err).Str("id", id).Msg("chaos: waiting for instance to recover")
		}
		if !n.now().Before(deadline) {
			return fmt.Errorf("container kill: %s not running+healthy within MTTR %v", id, n.mttr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// recordMTTR logs how long recovery took (upper bound: measured from the
// recorded inject time to the first healthy observation).
func (n *ContainerInjector) recordMTTR(r Recipe, id string) {
	ns, err := strconv.ParseInt(r["injected_at"], 10, 64)
	if err != nil || ns <= 0 {
		return
	}
	mttr := n.now().Sub(time.Unix(0, ns))
	n.log.Info().Str("id", id).Dur("mttr", mttr).Msg("chaos: instance recovered")
}

// Reverted is true when the instance is running and healthy now.
func (n *ContainerInjector) Reverted(ctx context.Context, r Recipe) (bool, error) {
	id := r["id"]
	if id == "" {
		return false, errors.New("container kill: recipe has no instance id")
	}
	return n.cuirass.Healthy(ctx, id)
}

// --- HTTP client over Cuirass ---

// ContainerChaosClient talks to a Cuirass over HTTP: GET /api/v1/info,
// GET /api/v1/state/actual, POST /api/v1/containers/{id}/chaos (X-API-Key).
type ContainerChaosClient struct {
	base   string
	key    string
	client *http.Client
}

// NewContainerChaosClient builds a client for the Cuirass at baseURL. apiKey
// is sent as X-API-Key on the mutating chaos call.
func NewContainerChaosClient(baseURL, apiKey string) (*ContainerChaosClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("container chaos client: bad URL %q", baseURL)
	}
	return &ContainerChaosClient{
		base: strings.TrimRight(baseURL, "/"),
		key:  apiKey,
		client: &http.Client{
			Timeout:       10 * time.Second,
			Transport:     &http.Transport{Proxy: nil},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *ContainerChaosClient) do(req *http.Request, out any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cuirass %s answered %d: %s", req.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// Modes reads enforce and chaos from /api/v1/info.
func (c *ContainerChaosClient) Modes(ctx context.Context) (bool, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/info", nil)
	if err != nil {
		return false, false, err
	}
	var info struct {
		Enforce bool `json:"enforce"`
		Chaos   bool `json:"chaos"`
	}
	if err := c.do(req, &info); err != nil {
		return false, false, err
	}
	return info.Enforce, info.Chaos, nil
}

// Chaos posts a stop/kill to the instance's chaos endpoint.
func (c *ContainerChaosClient) Chaos(ctx context.Context, id, action, reason string) error {
	payload, err := json.Marshal(map[string]string{"action": action, "reason": reason})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v1/containers/"+id+"/chaos", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("X-API-Key", c.key)
	}
	return c.do(req, nil)
}

// Healthy reads /api/v1/state/actual and reports whether id is running and
// healthy.
func (c *ContainerChaosClient) Healthy(ctx context.Context, id string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/state/actual", nil)
	if err != nil {
		return false, err
	}
	var actual map[string]struct {
		Status string `json:"status"`
		Health string `json:"health"`
	}
	if err := c.do(req, &actual); err != nil {
		return false, err
	}
	s, ok := actual[id]
	if !ok {
		return false, nil
	}
	return s.Status == "running" && s.Health == "healthy", nil
}
