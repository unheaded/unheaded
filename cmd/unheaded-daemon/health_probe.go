// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/metrics"
)

// HTTP health probe for managed instances (S-REAL-STEEL Phase 4). LXD has no
// health check, so without one an instance's health is "unknown" for ever and
// the reconciler's restart-when-unhealthy rule never fires. A desired spec may
// carry Health {Port, Path}; Cuirass then probes http://<instance IP>:<port><path>
// each sync, where the IP is the one LXD reports for the instance. No field
// of the spec names a host, the path is a plain path, redirects are not
// followed, proxies from the environment are not used, and the body is
// dropped after 4 KiB.

// HealthSpec is an HTTP health check on the instance itself.
type HealthSpec struct {
	Port int    `json:"port"`
	Path string `json:"path"`
}

const (
	healthProbeTimeout   = 2 * time.Second
	healthBodyLimit      = 4 << 10
	healthFailsUnhealthy = 3 // consecutive failed probes before "unhealthy"
)

// healthPathRe: an absolute path of unreserved characters and slashes; no
// query, fragment, percent-escape, scheme or authority.
var healthPathRe = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{0,255}$`)

func (h *HealthSpec) validate() error {
	if h.Port < 1 || h.Port > 65535 {
		return fmt.Errorf("health.port %d out of range 1-65535", h.Port)
	}
	if !healthPathRe.MatchString(h.Path) {
		return fmt.Errorf("health.path %q must be an absolute path of letters, digits and ._~/- (at most 256)", h.Path)
	}
	if strings.HasPrefix(h.Path, "//") {
		return fmt.Errorf("health.path %q must not start with //", h.Path)
	}
	for _, seg := range strings.Split(h.Path, "/") {
		if seg == ".." {
			return fmt.Errorf("health.path %q must not contain a .. segment", h.Path)
		}
	}
	return nil
}

// healthProber checks one instance; nil means healthy.
type healthProber interface {
	Probe(ctx context.Context, ip string, port int, path string) error
}

type httpProber struct {
	client *http.Client
}

// newHTTPProber returns a prober with its own transport: no proxy from the
// environment, no redirects, timeout on the whole exchange.
func newHTTPProber(timeout time.Duration) *httpProber {
	return &httpProber{client: &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy:               nil, // never HTTP_PROXY: the probe goes to the instance or nowhere
			DialContext:         (&net.Dialer{Timeout: timeout}).DialContext,
			MaxIdleConns:        0,
			DisableKeepAlives:   true,
			TLSHandshakeTimeout: timeout,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (p *httpProber) Probe(ctx context.Context, ip string, port int, path string) error {
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(ip, strconv.Itoa(port)), Path: path}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, healthBodyLimit)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// probeAddress returns ip if it is an address an instance can legitimately
// be probed at. A container chooses its own interface addresses, so one that
// reports loopback, link-local, unspecified or multicast would aim the probe
// at the host's own services: refused.
func probeAddress(ip string) (string, error) {
	addr := net.ParseIP(ip)
	switch {
	case addr == nil:
		return "", fmt.Errorf("no usable address (%q)", ip)
	case addr.IsLoopback(), addr.IsLinkLocalUnicast(), addr.IsLinkLocalMulticast(),
		addr.IsUnspecified(), addr.IsMulticast(), addr.IsInterfaceLocalMulticast():
		return "", fmt.Errorf("address %s is not an instance address; not probing it", ip)
	}
	return addr.String(), nil
}

// healthRecord is one instance's probe history.
type healthRecord struct {
	fails  int
	status string // healthy, unhealthy, or "" until the first verdict
}

// probeHealth probes every running managed instance whose desired spec has a
// Health check and records the verdict: one pass makes it healthy, three
// failures in a row unhealthy (the plan then restarts it, in enforce mode).
// The caller holds stateManager.mu.
func (d *Daemon) probeHealth(ctx context.Context) {
	if d.health == nil {
		d.health = map[string]*healthRecord{}
	}
	for id := range d.health {
		if spec, ok := d.stateManager.desired[id]; !ok || spec.Health == nil {
			delete(d.health, id)
			d.healthGauge.DeletePartialMatch(metrics.Labels{"id": id})
		}
	}
	prober := d.prober
	if prober == nil {
		prober = newHTTPProber(healthProbeTimeout)
		d.prober = prober
	}
	for id, spec := range d.stateManager.desired {
		if ctx.Err() != nil {
			// Out of time for this sync: the rest keep their verdicts. A probe
			// cut short by the sync's deadline is not the instance's fault.
			return
		}
		actual, ok := d.stateManager.actual[id]
		if spec.Health == nil || !ok || !actual.Managed || actual.Status != "running" {
			continue
		}
		rec := d.health[id]
		if rec == nil {
			rec = &healthRecord{}
			d.health[id] = rec
		}
		var err error
		if ip, aerr := probeAddress(actual.IP); aerr != nil {
			err = aerr
		} else {
			pctx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
			err = prober.Probe(pctx, ip, spec.Health.Port, spec.Health.Path)
			cancel()
			if err != nil && ctx.Err() != nil {
				return // the sync ran out of time, not the instance
			}
		}
		if err == nil {
			rec.fails, rec.status = 0, "healthy"
		} else {
			rec.fails++
			if rec.fails >= healthFailsUnhealthy {
				rec.status = "unhealthy"
			}
			d.log.Warn().Err(err).Str("id", id).Int("consecutive_failures", rec.fails).Msg("Health probe failed")
		}
		d.applyHealth(id, actual)
	}
}

// applyHealth overlays the recorded verdict on an instance's actual state
// (LXD itself says "unknown") and updates cuirass_container_health.
func (d *Daemon) applyHealth(id string, actual *ContainerState) {
	rec := d.health[id]
	if rec == nil || rec.status == "" {
		d.healthGauge.WithLabelValues(id).Set(-1)
		return
	}
	actual.Health = rec.status
	v := 0.0
	if rec.status == "healthy" {
		v = 1
	}
	d.healthGauge.WithLabelValues(id).Set(v)
}

// resetHealth forgets an instance's probe history after Cuirass created,
// restarted or deleted it: the new process gets three fresh chances.
func (d *Daemon) resetHealth(id string) {
	if rec := d.health[id]; rec != nil {
		rec.fails, rec.status = 0, ""
	}
}
