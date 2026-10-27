// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

// Package healthreport makes a service a voter on Akira's ballot (ADR-029):
// every Interval it probes its own /health and publishes the result to
// Wotan's system.health.reports as a health.HealthReport, the type Akira
// itself publishes and tallies.
//
// A service reports under Reporter "<service>@<host>", so each running copy
// is one vote, apart from the vote of the Akira on its host. A service that
// stops stops reporting, and its vote goes stale after
// health.ReportFreshness; a service that is up but failing its own /health
// (Wotan stream down, store unreachable) votes "failing".
package healthreport

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
	"unicode/utf8"

	"unheaded/pkg/health"
	pqtls "unheaded/pkg/tls"
)

// Topic is where every report goes (ADR-029).
const Topic = "system.health.reports"

// DefaultInterval is one report per Akira sweep.
const DefaultInterval = health.HealthCheckInterval

// MaxErrorBytes caps a failing probe's message in a report.
const MaxErrorBytes = 200

// Publisher is the part of the Wotan client a reporter needs. The client
// must already be a member of Topic (see StartService).
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// Probe checks the service; nil means healthy.
type Probe func(ctx context.Context) error

// Config describes one reporter.
type Config struct {
	Service  string        // the service's name on Akira's ballot (see BallotName)
	Host     string        // the host it runs on (see HostName)
	Interval time.Duration // between reports; zero means DefaultInterval
	Probe    Probe
}

// Start publishes one report at once and then one per Interval until ctx
// ends; the returned channel closes when it has stopped. A publish error is
// logged (first failure and recovery only) and the next report is tried as
// usual. A nil Publisher or Probe, or an empty Service or Host, is a
// programming error and panics here rather than reporting nothing.
func Start(ctx context.Context, pub Publisher, cfg Config) <-chan struct{} {
	switch {
	case pub == nil:
		panic("healthreport.Start: nil Publisher")
	case cfg.Probe == nil:
		panic("healthreport.Start: nil Probe")
	case cfg.Service == "":
		panic("healthreport.Start: empty Service")
	case cfg.Host == "":
		panic("healthreport.Start: empty Host")
	}
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	done := make(chan struct{})
	go func() { // #nosec G118 -- runs until ctx ends; done closes when it stops
		defer close(done)
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()
		failing := false
		for {
			err := publish(ctx, pub, cfg)
			switch {
			case err != nil && !failing && ctx.Err() == nil:
				log.Printf("[healthreport] %s: cannot publish to %s, retrying each %s: %v", cfg.Service, Topic, cfg.Interval, err)
				failing = true
			case err == nil && failing:
				log.Printf("[healthreport] %s: publishing to %s again", cfg.Service, Topic)
				failing = false
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

// Report runs the probe once and returns the report it makes.
func Report(ctx context.Context, cfg Config) health.HealthReport {
	pctx, cancel := context.WithTimeout(ctx, probeTimeout(cfg.Interval))
	defer cancel()
	start := time.Now()
	err := cfg.Probe(pctx)
	r := health.HealthReport{
		Service:   cfg.Service,
		Reporter:  cfg.Service + "@" + cfg.Host,
		Host:      cfg.Host,
		Healthy:   err == nil,
		Latency:   time.Since(start),
		Timestamp: time.Now(),
	}
	if err != nil {
		r.Error = truncate(err.Error(), MaxErrorBytes)
	}
	return r
}

func publish(ctx context.Context, pub Publisher, cfg Config) error {
	r := Report(ctx, cfg)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return err
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return pub.Publish(pctx, Topic, payload)
}

// probeTimeout bounds one probe: 5 s (Akira's own check timeout), or less
// when reports are more frequent than that.
func probeTimeout(interval time.Duration) time.Duration {
	if interval > 0 && interval < 5*time.Second {
		return interval
	}
	return 5 * time.Second
}

// truncate cuts s to at most n bytes without splitting a UTF-8 rune.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// probeClient never follows a redirect: a /health that redirects is not a
// 200 from this service. With UNHEADED_TLS_ENABLED=true it trusts the
// deployment CA (pkg/tls.ClientFromEnv, S-IRON-GATE Phase 8).
func probeClient() (*http.Client, error) {
	c, err := pqtls.ClientFromEnv(5 * time.Second)
	if err != nil {
		return nil, err
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return c, nil
}

// HTTPProbe probes url with GET; healthy is a 200, as for Akira's own checks.
// A TLS environment the client cannot use fails every probe with its error.
func HTTPProbe(url string) Probe {
	client, cerr := probeClient()
	return func(ctx context.Context) error {
		if cerr != nil {
			return cerr
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close() // #nosec G104 -- status is all a probe reads; a close error changes nothing
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return nil
	}
}

// HealthURL is the service's own /health on the address it listens on:
// loopback when it listens on every interface (or the address cannot be
// parsed), else the address it bound.
func HealthURL(listenAddr string, port int) string {
	host := "127.0.0.1"
	if h, _, err := net.SplitHostPort(listenAddr); err == nil && h != "" {
		if ip := net.ParseIP(h); ip == nil || !ip.IsUnspecified() {
			host = h
		}
	}
	return pqtls.Scheme() + "://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/health"
}

// ballotNames maps a Wotan display name to the name Akira's targets use for
// the same service (cmd/akira defaultTargets, config/akira/akira.yaml), so
// that the service's vote and Akira's check land on one ballot.
var ballotNames = map[string]string{
	"kanban-app":        "kanban",
	"dashboard-backend": "dashboard",
	"unheaded-daemon":   "daemon",
}

// BallotName is $HEALTH_REPORT_SERVICE when set (the compose stack names
// unheaded-daemon "cuirass"), else Akira's name for the display name.
func BallotName(displayName string) string {
	if v := os.Getenv("HEALTH_REPORT_SERVICE"); v != "" {
		return v
	}
	if v, ok := ballotNames[displayName]; ok {
		return v
	}
	return displayName
}

// HostName is $KINGDOM_NODE (the name Akira's --node uses in compose), else
// the hostname, else "localhost".
func HostName() string {
	if v := os.Getenv("KINGDOM_NODE"); v != "" {
		return v
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "localhost"
}
