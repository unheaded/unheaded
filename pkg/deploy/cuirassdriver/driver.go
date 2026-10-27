// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cuirassdriver drives deploy strategies through Cuirass, the
// control plane that owns LXD (S-REAL-STEEL Phase 7, D-03..D-05).
//
// A deployed instance is immutable and versioned: <svc>-<ver>-<n>, image
// <svc>-<ver>, labels svc, ver and deployment plus the caller's. "Update"
// is create-then-delete. Health is Cuirass's health (its probe of the
// instance), never the driver's own. The service's stable endpoint is an
// LXD proxy device that Cuirass manages on exactly one instance; moving it
// is the traffic switch (D-04). A partial traffic weight is the client-side
// split by replica share, so it changes nothing here.
//
// Every write carries the Cuirass API key (X-API-Key); the driver will not
// start without one.
package cuirassdriver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"unheaded/pkg/deploy/strategy"
	pqtls "unheaded/pkg/tls"
)

// HealthSpec and ProxySpec mirror Cuirass's ContainerSpec fields.
type HealthSpec struct {
	Port int    `json:"port"`
	Path string `json:"path"`
}

// ProxySpec is the service's stable endpoint (an LXD proxy device).
type ProxySpec struct {
	Listen  string `json:"listen"`
	Connect string `json:"connect"`
}

// containerSpec is Cuirass's desired spec (PUT /api/v1/containers/{id}).
type containerSpec struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	CPU    int               `json:"cpu,omitempty"`
	Memory int64             `json:"memory,omitempty"`
	Labels map[string]string `json:"labels"`
	Health *HealthSpec       `json:"health,omitempty"`
	Proxy  *ProxySpec        `json:"proxy,omitempty"`
}

// containerState is Cuirass's actual state (GET /api/v1/state/actual).
type containerState struct {
	ID      string            `json:"id"`
	Status  string            `json:"status"`
	IP      string            `json:"ip"`
	Health  string            `json:"health"`
	Managed bool              `json:"managed"`
	Proxy   *ProxySpec        `json:"proxy,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// ServiceSpec is what every instance of one service gets.
type ServiceSpec struct {
	Service string
	Health  *HealthSpec       // required: health is how a create is confirmed
	Proxy   *ProxySpec        // optional stable endpoint
	Labels  map[string]string // extra labels (svc, ver, deployment are the driver's)
}

const (
	labelSvc        = "svc"
	labelVer        = "ver"
	labelDeployment = "deployment"

	maxBody             = 4 << 20
	defaultPoll         = 2 * time.Second
	defaultTimeout      = 3 * time.Minute
	requestTimeout      = 10 * time.Second
	reconcileRetries    = 3
	maxInstanceNameLen  = 63
	maxInstancesScanned = 1000
)

var (
	// LXD instance names: a letter first, letters, digits and hyphens, at
	// most 63, no trailing hyphen (the rule Cuirass enforces).
	lxdNameRe  = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
	serviceRe  = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,30}[a-z0-9])?$`)
	labelKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	labelValRe = regexp.MustCompile(`^[A-Za-z0-9._-]{0,63}$`)
	verCleanRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// StatusError is a Cuirass answer other than 2xx.
type StatusError struct {
	Method, Path string
	Code         int
	Body         string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("cuirass %s %s: %d %s", e.Method, e.Path, e.Code, e.Body)
}

// Config configures a Driver.
type Config struct {
	URL    string // Cuirass base URL
	APIKey string // X-API-Key on writes; required unless Authorizer is set
	// Authorizer returns the Authorization header value for a write (an
	// EdDSA service token minted for Cuirass, S-IRON-GATE Phase 5). When
	// set it is used instead of APIKey; an error fails the write unsent.
	Authorizer   func() (string, error)
	Client       *http.Client
	PollInterval time.Duration // how often to re-read Cuirass while waiting
	Timeout      time.Duration // longest wait for one create/delete/switch
}

// Driver talks to one Cuirass.
type Driver struct {
	base    string
	key     string
	authz   func() (string, error)
	client  *http.Client
	poll    time.Duration
	timeout time.Duration
}

// New makes a Driver. A missing API key is an error: credentials fail
// loud, never a default.
func New(cfg Config) (*Driver, error) {
	if cfg.APIKey == "" && cfg.Authorizer == nil {
		return nil, errors.New("cuirassdriver: no credential (a service key via AUTH_KEY_FILE, or CUIRASS_API_KEY)")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("cuirassdriver: bad Cuirass URL %q", cfg.URL)
	}
	client := cfg.Client
	if client == nil {
		// https to Cuirass trusts the deployment CA under
		// UNHEADED_TLS_ENABLED (S-IRON-GATE Phase 8); never via a proxy.
		tr, err := pqtls.TransportFromEnv()
		if err != nil {
			return nil, fmt.Errorf("cuirassdriver: %w", err)
		}
		tr.Proxy = nil
		client = &http.Client{Transport: tr}
	}
	c := *client
	c.Timeout = requestTimeout
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	d := &Driver{base: strings.TrimRight(cfg.URL, "/"), key: cfg.APIKey, authz: cfg.Authorizer, client: &c,
		poll: cfg.PollInterval, timeout: cfg.Timeout}
	if d.poll <= 0 {
		d.poll = defaultPoll
	}
	if d.timeout <= 0 {
		d.timeout = defaultTimeout
	}
	return d, nil
}

// FromEnv makes a Driver from CUIRASS_URL (default the compose Cuirass,
// http://127.0.0.1:19006) and CUIRASS_API_KEY (required).
func FromEnv() (*Driver, error) {
	key := os.Getenv("CUIRASS_API_KEY")
	if key == "" {
		return nil, errors.New("cuirassdriver: CUIRASS_API_KEY is not set; credentials are not stored in the repo")
	}
	u := os.Getenv("CUIRASS_URL")
	if u == "" {
		u = "http://127.0.0.1:19006"
	}
	return New(Config{URL: u, APIKey: key})
}

// InstanceName is <svc>-<ver>-<n> with the version folded to [a-z0-9-].
func InstanceName(svc, ver string, n int) (string, error) {
	if !serviceRe.MatchString(svc) {
		return "", fmt.Errorf("cuirassdriver: service %q: want lowercase letters, digits, hyphens", svc)
	}
	v := cleanVersion(ver)
	if v == "" {
		return "", fmt.Errorf("cuirassdriver: version %q has nothing usable in a name", ver)
	}
	name := fmt.Sprintf("%s-%s-%d", svc, v, n)
	if len(name) > maxInstanceNameLen || !lxdNameRe.MatchString(name) {
		return "", fmt.Errorf("cuirassdriver: instance name %q is not a valid LXD name", name)
	}
	return name, nil
}

func cleanVersion(ver string) string {
	return strings.Trim(verCleanRe.ReplaceAllString(strings.ToLower(ver), "-"), "-")
}

// --- HTTP ---

func (d *Driver) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		if d.authz != nil {
			h, err := d.authz()
			if err != nil {
				return fmt.Errorf("cuirass %s %s: service token: %w", method, path, err)
			}
			req.Header.Set("Authorization", h)
		} else {
			req.Header.Set("X-API-Key", d.key)
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("cuirass %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("cuirass %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return &StatusError{Method: method, Path: path, Code: resp.StatusCode, Body: msg}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("cuirass %s %s: %w", method, path, err)
		}
	}
	return nil
}

func (d *Driver) desired(ctx context.Context) (map[string]*containerSpec, error) {
	var m map[string]*containerSpec
	if err := d.do(ctx, http.MethodGet, "/api/v1/state/desired", nil, &m); err != nil {
		return nil, err
	}
	if len(m) > maxInstancesScanned {
		return nil, fmt.Errorf("cuirass: %d desired instances, more than this driver reads", len(m))
	}
	return m, nil
}

func (d *Driver) actual(ctx context.Context) (map[string]*containerState, error) {
	var m map[string]*containerState
	if err := d.do(ctx, http.MethodGet, "/api/v1/state/actual", nil, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (d *Driver) put(ctx context.Context, spec *containerSpec) error {
	return d.do(ctx, http.MethodPut, "/api/v1/containers/"+url.PathEscape(spec.ID), spec, nil)
}

func (d *Driver) remove(ctx context.Context, id string) error {
	err := d.do(ctx, http.MethodDelete, "/api/v1/containers/"+url.PathEscape(id), nil, nil)
	var se *StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return nil // already not desired
	}
	return err
}

// reconcileNow asks Cuirass to run one sync now instead of waiting for its
// next tick. It is an optimisation: a Cuirass without the endpoint (404) or
// one that keeps answering 429 is left to its own schedule.
func (d *Driver) reconcileNow(ctx context.Context) {
	for i := 0; i < reconcileRetries; i++ {
		err := d.do(ctx, http.MethodPost, "/api/v1/reconcile", nil, nil)
		var se *StatusError
		if err == nil || !errors.As(err, &se) || se.Code != http.StatusTooManyRequests {
			return
		}
		if sleepCtx(ctx, time.Second) != nil {
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// waitFor re-reads Cuirass's actual state until cond holds, for at most
// the driver's timeout (or ctx's, if sooner).
func (d *Driver) waitFor(ctx context.Context, what string, cond func(map[string]*containerState) bool) error {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	var lastErr error
	for {
		actual, err := d.actual(ctx)
		if err == nil && cond(actual) {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		if sleepCtx(ctx, d.poll) != nil {
			if lastErr != nil {
				return fmt.Errorf("cuirassdriver: %s: %w (last error: %v)", what, ctx.Err(), lastErr)
			}
			return fmt.Errorf("cuirassdriver: %s: %w", what, ctx.Err())
		}
	}
}

func isHealthy(s *containerState) bool {
	return s != nil && s.Managed && s.Status == "running" && s.Health == "healthy"
}

// Ping reads Cuirass's /api/v1/info: an error means Cuirass cannot be
// reached (or answers non-2xx), so nothing can be deployed now.
func (d *Driver) Ping(ctx context.Context) error {
	var info map[string]any
	return d.do(ctx, http.MethodGet, "/api/v1/info", nil, &info)
}

// --- the service view ---

// Instances lists the service's instances: Cuirass's desired specs with
// the svc label, with health and address from its actual state.
func (d *Driver) Instances(ctx context.Context, svc string) ([]strategy.Instance, error) {
	desired, err := d.desired(ctx)
	if err != nil {
		return nil, err
	}
	actual, err := d.actual(ctx)
	if err != nil {
		return nil, err
	}
	var out []strategy.Instance
	for _, id := range sortedIDs(desired) {
		spec := desired[id]
		if spec.Labels[labelSvc] != svc {
			continue
		}
		inst := strategy.Instance{ID: id, Version: spec.Labels[labelVer], State: strategy.InstanceStarting, Labels: spec.Labels}
		if a := actual[id]; a != nil {
			inst.Address = a.IP
			inst.Healthy = isHealthy(a)
			if a.Status == "running" {
				inst.State = strategy.InstanceRunning
			}
		}
		out = append(out, inst)
	}
	return out, nil
}

func sortedIDs[T any](m map[string]T) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Callbacks returns the strategy driver for one deployment of spec's
// service towards targetVersion.
func (d *Driver) Callbacks(spec ServiceSpec, deploymentID, targetVersion string) (*strategy.Callbacks, error) {
	if !serviceRe.MatchString(spec.Service) {
		return nil, fmt.Errorf("cuirassdriver: service %q: want lowercase letters, digits, hyphens", spec.Service)
	}
	if spec.Health == nil {
		return nil, errors.New("cuirassdriver: a health spec is required (Cuirass's health is how a new instance is confirmed)")
	}
	for k, v := range spec.Labels {
		if k == labelSvc || k == labelVer || k == labelDeployment {
			return nil, fmt.Errorf("cuirassdriver: label %q is the driver's", k)
		}
		if !labelKeyRe.MatchString(k) || !labelValRe.MatchString(v) {
			return nil, fmt.Errorf("cuirassdriver: label %q=%q is not allowed", k, v)
		}
	}
	if !labelValRe.MatchString(deploymentID) || cleanVersion(targetVersion) == "" {
		return nil, fmt.Errorf("cuirassdriver: bad deployment id %q or version %q", deploymentID, targetVersion)
	}
	s := &session{d: d, spec: spec, deployment: deploymentID, target: targetVersion}
	return &strategy.Callbacks{
		OnInstanceCreate: s.create,
		OnInstanceDelete: s.delete,
		OnInstanceUpdate: s.update,
		OnHealthCheck:    s.health,
		OnTrafficShift:   s.shift,
	}, nil
}

type session struct {
	d          *Driver
	spec       ServiceSpec
	deployment string
	target     string
}

func (s *session) labels(ver string, extra map[string]string) map[string]string {
	l := map[string]string{}
	for k, v := range s.spec.Labels {
		l[k] = v
	}
	for k, v := range extra {
		if k == labelSvc || k == labelVer || k == "version" || !labelKeyRe.MatchString(k) || !labelValRe.MatchString(v) {
			continue
		}
		l[k] = v
	}
	l[labelSvc] = s.spec.Service
	l[labelVer] = cleanVersion(ver)
	l[labelDeployment] = s.deployment
	return l
}

// mine reports whether a desired spec is an instance of this service.
func (s *session) mine(spec *containerSpec) bool {
	return spec != nil && spec.Labels[labelSvc] == s.spec.Service
}

func (s *session) proxyHolder(desired map[string]*containerSpec) string {
	for _, id := range sortedIDs(desired) {
		if spec := desired[id]; s.mine(spec) && spec.Proxy != nil {
			return id
		}
	}
	return ""
}

func (s *session) create(ctx context.Context, p *strategy.InstanceParams) (*strategy.Instance, error) {
	if p == nil {
		return nil, errors.New("cuirassdriver: create without params")
	}
	if p.ServiceName != "" && p.ServiceName != s.spec.Service {
		return nil, fmt.Errorf("cuirassdriver: create for %q in a %q deployment", p.ServiceName, s.spec.Service)
	}
	desired, err := s.d.desired(ctx)
	if err != nil {
		return nil, err
	}
	name := ""
	for n := 0; n < maxInstancesScanned; n++ {
		cand, err := InstanceName(s.spec.Service, p.Version, n)
		if err != nil {
			return nil, err
		}
		if _, taken := desired[cand]; !taken {
			name = cand
			break
		}
	}
	if name == "" {
		return nil, errors.New("cuirassdriver: no free instance name")
	}
	spec := &containerSpec{
		ID:     name,
		Name:   name,
		Image:  s.spec.Service + "-" + cleanVersion(p.Version),
		Labels: s.labels(p.Version, p.Labels),
		Health: s.spec.Health,
	}
	// The first instance of a service carries its stable endpoint; later
	// ones get it only when traffic is switched to them.
	if s.spec.Proxy != nil && s.proxyHolder(desired) == "" {
		spec.Proxy = s.spec.Proxy
	}
	if err := s.d.put(ctx, spec); err != nil {
		return nil, err
	}
	s.d.reconcileNow(ctx)

	var got *containerState
	err = s.d.waitFor(ctx, "instance "+name+" healthy", func(a map[string]*containerState) bool {
		got = a[name]
		return isHealthy(got)
	})
	if err != nil {
		// Do not leave a half-made instance desired.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
		defer cancel()
		if rerr := s.d.remove(cctx, name); rerr != nil {
			err = fmt.Errorf("%w; removing it again failed: %v", err, rerr)
		} else {
			s.d.reconcileNow(cctx)
		}
		return nil, err
	}
	return &strategy.Instance{
		ID:      name,
		Version: p.Version,
		State:   strategy.InstanceRunning,
		Healthy: true,
		Address: got.IP,
		Labels:  spec.Labels,
	}, nil
}

func (s *session) delete(ctx context.Context, id string) error {
	if !lxdNameRe.MatchString(id) {
		return fmt.Errorf("cuirassdriver: %q is not an instance name", id)
	}
	desired, err := s.d.desired(ctx)
	if err != nil {
		return err
	}
	spec, ok := desired[id]
	if !ok {
		actual, err := s.d.actual(ctx)
		if err != nil {
			return err
		}
		if _, exists := actual[id]; exists {
			return fmt.Errorf("cuirassdriver: %s is not a desired %s instance; not deleting it", id, s.spec.Service)
		}
		return nil // already gone
	}
	if !s.mine(spec) {
		return fmt.Errorf("cuirassdriver: %s is not a %s instance; not deleting it", id, s.spec.Service)
	}
	if spec.Proxy != nil {
		// The stable endpoint moves before its holder goes: to the
		// target version if one is running, else to any other instance.
		if to := s.pickProxyTarget(ctx, desired, id); to != "" {
			if err := s.moveProxy(ctx, desired, id, to); err != nil {
				return err
			}
		}
	}
	if err := s.d.remove(ctx, id); err != nil {
		return err
	}
	s.d.reconcileNow(ctx)
	return s.d.waitFor(ctx, "instance "+id+" removed", func(a map[string]*containerState) bool {
		_, still := a[id]
		return !still
	})
}

func (s *session) pickProxyTarget(ctx context.Context, desired map[string]*containerSpec, except string) string {
	actual, err := s.d.actual(ctx)
	if err != nil {
		return ""
	}
	best, bestScore := "", -1
	for _, id := range sortedIDs(desired) {
		spec := desired[id]
		if id == except || !s.mine(spec) {
			continue
		}
		score := 0
		if isHealthy(actual[id]) {
			score += 2
		}
		if spec.Labels[labelVer] == cleanVersion(s.target) {
			score++
		}
		if score > bestScore {
			best, bestScore = id, score
		}
	}
	return best
}

// moveProxy takes the proxy off from and puts it on to, then waits until
// Cuirass reports it on to. Cuirass refuses two specs with one listen
// address, so the order is fixed: remove, add, one reconcile (Cuirass
// applies removals first).
func (s *session) moveProxy(ctx context.Context, desired map[string]*containerSpec, from, to string) error {
	if from != "" {
		old := *desired[from]
		old.Proxy = nil
		if err := s.d.put(ctx, &old); err != nil {
			return err
		}
	}
	next := *desired[to]
	next.Proxy = s.spec.Proxy
	if err := s.d.put(ctx, &next); err != nil {
		return err
	}
	s.d.reconcileNow(ctx)
	return s.d.waitFor(ctx, "proxy on "+to, func(a map[string]*containerState) bool {
		onTo := a[to] != nil && a[to].Proxy != nil
		offFrom := from == "" || a[from] == nil || a[from].Proxy == nil
		return onTo && offFrom
	})
}

func (s *session) update(ctx context.Context, id, version string) error {
	inst, err := s.create(ctx, &strategy.InstanceParams{ServiceName: s.spec.Service, Version: version})
	if err != nil {
		return err
	}
	if err := s.delete(ctx, id); err != nil {
		return fmt.Errorf("cuirassdriver: %s is up but %s was not deleted: %w", inst.ID, id, err)
	}
	return nil
}

func (s *session) health(ctx context.Context, id string) (bool, error) {
	actual, err := s.d.actual(ctx)
	if err != nil {
		return false, err
	}
	a, ok := actual[id]
	if !ok {
		return false, fmt.Errorf("cuirassdriver: Cuirass reports no instance %s", id)
	}
	return isHealthy(a), nil
}

func (s *session) shift(ctx context.Context, p *strategy.TrafficShiftParams) error {
	if p == nil {
		return errors.New("cuirassdriver: shift without params")
	}
	if p.Weight < 100 {
		// D-04: below 100% the split is client-side by replica share;
		// there is nothing to switch on the server.
		return nil
	}
	if s.spec.Proxy == nil {
		return nil // no stable endpoint: every instance is reached directly
	}
	desired, err := s.d.desired(ctx)
	if err != nil {
		return err
	}
	holder := s.proxyHolder(desired)
	want := cleanVersion(p.NewVersion)
	if holder != "" && desired[holder].Labels[labelVer] == want {
		return nil // already there
	}
	actual, err := s.d.actual(ctx)
	if err != nil {
		return err
	}
	to := ""
	for _, id := range sortedIDs(desired) {
		spec := desired[id]
		if s.mine(spec) && spec.Labels[labelVer] == want && isHealthy(actual[id]) {
			to = id
			break
		}
	}
	if to == "" {
		return fmt.Errorf("cuirassdriver: no healthy %s instance of version %s to switch traffic to", s.spec.Service, p.NewVersion)
	}
	return s.moveProxy(ctx, desired, holder, to)
}
