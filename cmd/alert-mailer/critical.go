// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/critalert"
	"unheaded/pkg/metrics"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotansig"
)

// alerts.critical (ADR-147 Phase 8, D1 = A): alert-mailer publishes every
// Alertmanager notification with severity="critical", firing and resolved,
// and a snapshot of what fires every snapshotEvery, signed (ML-DSA-65) or
// not at all. Every service on the bus attaches pkg/critalert. The message
// is metadata only: no annotation text, no label outside the six fields.
//
// The snapshot is the whole firing set and replaces each subscriber's. The
// set lives in memory, so at start (and before every snapshot) it is read
// back from Alertmanager's /api/v2/alerts, filtered as the webhooks are:
// critical, active (not silenced or inhibited), routed to page or ticket.
// Until one read succeeds no snapshot goes out: an empty one after a
// restart would clear every hold while the alert still fires.

// snapshotEvery is how often the active set is republished; subscribers
// call a publisher silent for critalert.SilentAfter unknown.
const snapshotEvery = 5 * time.Minute

// critPublishTimeout bounds one publish: the bus never holds up the mail.
const critPublishTimeout = 5 * time.Second

// critSink publishes one payload on alerts.critical, signed.
type critSink interface {
	Publish(ctx context.Context, payload []byte) error
}

// critMsg is the wire shape pkg/critalert decodes.
type critMsg struct {
	Fingerprint string    `json:"fingerprint"`
	AlertName   string    `json:"alertname"`
	Severity    string    `json:"severity"`
	Status      string    `json:"status"`
	Host        string    `json:"host"`
	StartsAt    time.Time `json:"starts_at"`
}

// snapMsg is one alert inside a snapshot (critalert's snapshot entry).
type snapMsg struct {
	Fingerprint string    `json:"fingerprint"`
	AlertName   string    `json:"alertname"`
	Severity    string    `json:"severity"`
	Host        string    `json:"host"`
	StartsAt    time.Time `json:"starts_at"`
}

// alertSource reads the critical alerts firing now.
type alertSource interface {
	fetch(ctx context.Context) ([]critMsg, error)
}

// critPublisher keeps the critical alerts firing and publishes changes.
type critPublisher struct {
	sink          critSink    // nil: no signing key, every publish refused and counted
	am            alertSource // nil: the memory set is trusted as is (tests)
	published     *metrics.Counter
	refused       *metrics.Counter
	errors        *metrics.Counter
	rebuildErrors *metrics.Counter
	firstOK       sync.Once // the first accepted publish is logged

	mu      sync.Mutex
	active  map[string]critMsg // firing, by fingerprint
	rebuilt bool               // the set has been read from Alertmanager once
	touched map[string]string  // while a read is in flight: fingerprint → status seen since
}

func newCritPublisher(sink critSink, reg *metrics.Registry) *critPublisher {
	p := &critPublisher{
		sink:          sink,
		published:     metrics.NewCounter("alert_mailer_critical_published_total", "alerts.critical messages Wotan accepted, signed: each notification and a snapshot every 5 minutes", nil),
		refused:       metrics.NewCounter("alert_mailer_critical_publish_refused_total", "alerts.critical messages not sent because no signing key is set or readable (ALERTS_SIGNING_KEY_FILE)", nil),
		errors:        metrics.NewCounter("alert_mailer_critical_publish_errors_total", "alerts.critical publishes that failed (Wotan down or refused)", nil),
		rebuildErrors: metrics.NewCounter("alert_mailer_critical_rebuild_errors_total", "Reads of Alertmanager's firing alerts that failed; no snapshot goes out until one succeeds", nil),
		active:        map[string]critMsg{},
	}
	reg.MustRegister(p.published)
	reg.MustRegister(p.refused)
	reg.MustRegister(p.errors)
	reg.MustRegister(p.rebuildErrors)
	return p
}

// trim keeps a field inside critalert.MaxField.
func trim(s string) string {
	if len(s) > critalert.MaxField {
		return s[:critalert.MaxField]
	}
	return s
}

// Observe publishes each critical alert of w, after the mail decision; a
// failure is counted and never fails the webhook. A nil p does nothing.
func (p *critPublisher) Observe(w Webhook) {
	if p == nil {
		return
	}
	for _, a := range w.Alerts {
		if a.Labels["severity"] != "critical" || a.Fingerprint == "" {
			continue
		}
		status := "firing"
		if a.Status == "resolved" {
			status = "resolved"
		}
		m := critMsg{
			Fingerprint: trim(a.Fingerprint),
			AlertName:   trim(a.Labels["alertname"]),
			Severity:    "critical",
			Status:      status,
			Host:        trim(a.Labels["host"]),
			StartsAt:    a.StartsAt,
		}
		p.mu.Lock()
		if status == "firing" {
			p.active[m.Fingerprint] = m
		} else {
			delete(p.active, m.Fingerprint)
		}
		if p.touched != nil {
			p.touched[m.Fingerprint] = status
		}
		p.mu.Unlock()
		payload, err := json.Marshal(m)
		if err != nil {
			p.errors.Inc()
			continue
		}
		p.send(payload)
	}
}

// rebuild reads the firing set from Alertmanager and replaces the memory
// set with it; a notification that arrived during the read wins. A failed
// read is counted and leaves the set as it was.
func (p *critPublisher) rebuild(ctx context.Context) {
	if p.am == nil {
		p.mu.Lock()
		p.rebuilt = true
		p.mu.Unlock()
		return
	}
	p.mu.Lock()
	p.touched = map[string]string{}
	p.mu.Unlock()
	got, err := p.am.fetch(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	touched := p.touched
	p.touched = nil
	if err != nil {
		p.rebuildErrors.Inc()
		log.Warn().Err(err).Bool("rebuilt_before", p.rebuilt).Msg("alerts.critical: Alertmanager's firing alerts not read")
		return
	}
	next := make(map[string]critMsg, len(got))
	for _, m := range got {
		next[m.Fingerprint] = m
	}
	for fp, status := range touched {
		if status == "resolved" {
			delete(next, fp)
		} else if m, ok := p.active[fp]; ok {
			next[fp] = m
		}
	}
	if !p.rebuilt {
		log.Info().Int("firing", len(next)).Msg("alerts.critical: firing set read from Alertmanager")
	}
	p.active = next
	p.rebuilt = true
}

// Snapshot re-reads the firing set from Alertmanager, then publishes it
// whole, sorted by fingerprint; an empty list too, so subscribers stay out
// of unknown. Until the set has been read once nothing is published.
func (p *critPublisher) Snapshot() {
	if p == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), amTimeout)
	p.rebuild(ctx)
	cancel()
	p.mu.Lock()
	if !p.rebuilt {
		p.mu.Unlock()
		return
	}
	list := make([]snapMsg, 0, len(p.active))
	for _, m := range p.active {
		list = append(list, snapMsg{Fingerprint: m.Fingerprint, AlertName: m.AlertName, Severity: m.Severity, Host: m.Host, StartsAt: m.StartsAt})
	}
	p.mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].Fingerprint < list[j].Fingerprint })
	if len(list) > critalert.MaxActive {
		list = list[:critalert.MaxActive]
	}
	payload, err := json.Marshal(map[string][]snapMsg{"snapshot": list})
	if err != nil {
		p.errors.Inc()
		return
	}
	p.send(payload)
}

func (p *critPublisher) send(payload []byte) {
	if p.sink == nil {
		p.refused.Inc()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), critPublishTimeout)
	defer cancel()
	if err := p.sink.Publish(ctx, payload); err != nil {
		p.errors.Inc()
		log.Warn().Err(err).Msg("alerts.critical publish failed")
		return
	}
	p.published.Inc()
	p.firstOK.Do(func() { log.Info().Msg("alerts.critical: first message accepted by Wotan") })
}

// run publishes a snapshot at once (after reading the set back from
// Alertmanager) and on every tick until ctx ends.
func (p *critPublisher) run(ctx context.Context, tick <-chan time.Time) {
	p.Snapshot()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			p.Snapshot()
		}
	}
}

// wotanCritSink publishes signed on the process's one Wotan client, as
// alert-mailer, joining the topic first.
type wotanCritSink struct {
	c      *wotanClient.Client
	signer *wotansig.TopicSigner
	mu     sync.Mutex
	joined bool
}

func (w *wotanCritSink) Publish(ctx context.Context, payload []byte) error {
	w.mu.Lock()
	if !w.joined {
		if _, err := w.c.Subscribe(ctx, critalert.Topic, "alert-mailer"); err != nil {
			w.mu.Unlock()
			return fmt.Errorf("join %s: %w", critalert.Topic, err)
		}
		w.joined = true
	}
	w.mu.Unlock()
	return w.c.PublishWithSigner(ctx, critalert.Topic, payload, w.signer)
}

// amTimeout bounds one read of Alertmanager's alerts.
const amTimeout = 10 * time.Second

// amMaxBody bounds Alertmanager's answer.
const amMaxBody = 4 << 20

// alertmanagerSource reads /api/v2/alerts.
type alertmanagerSource struct {
	base   string // http(s)://host:port
	client *http.Client
}

// amAlert is the part of Alertmanager's gettableAlert read here.
type amAlert struct {
	Fingerprint string            `json:"fingerprint"`
	Labels      map[string]string `json:"labels"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Receivers   []struct {
		Name string `json:"name"`
	} `json:"receivers"`
	Status struct {
		State string `json:"state"`
	} `json:"status"`
}

// mailerReceivers are the routes that reach alert-mailer's webhooks
// (monitoring/alertmanager/alertmanager.yml): what it is told of, it
// publishes.
var mailerReceivers = map[string]bool{"page": true, "ticket": true}

func (s *alertmanagerSource) fetch(ctx context.Context) ([]critMsg, error) {
	q := url.Values{}
	q.Set("active", "true")
	q.Set("silenced", "false")
	q.Set("inhibited", "false")
	q.Set("unprocessed", "false")
	q.Add("filter", `severity="critical"`)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.base, "/")+"/api/v2/alerts?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("alertmanager answered %d", resp.StatusCode)
	}
	var in []amAlert
	if err := json.NewDecoder(io.LimitReader(resp.Body, amMaxBody)).Decode(&in); err != nil {
		return nil, fmt.Errorf("alertmanager answer: %w", err)
	}
	now := time.Now()
	out := make([]critMsg, 0, len(in))
	for _, a := range in {
		if a.Labels["severity"] != "critical" || a.Fingerprint == "" || a.Status.State != "active" {
			continue
		}
		if !a.EndsAt.IsZero() && a.EndsAt.Before(now) {
			continue
		}
		routed := false
		for _, r := range a.Receivers {
			routed = routed || mailerReceivers[r.Name]
		}
		if !routed {
			continue
		}
		out = append(out, critMsg{
			Fingerprint: trim(a.Fingerprint),
			AlertName:   trim(a.Labels["alertname"]),
			Severity:    "critical",
			Status:      "firing",
			Host:        trim(a.Labels["host"]),
			StartsAt:    a.StartsAt,
		})
	}
	return out, nil
}

// alertmanagerFromEnv: ALERTMANAGER_URL, default http://alertmanager:9093
// (the monitoring stack's service name); http or https with a host only.
func alertmanagerFromEnv(getenv func(string) string) (*alertmanagerSource, error) {
	raw := strings.TrimSpace(getenv("ALERTMANAGER_URL"))
	if raw == "" {
		raw = "http://alertmanager:9093"
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("ALERTMANAGER_URL %q: want http(s)://host:port", raw)
	}
	return &alertmanagerSource{base: u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/"), client: &http.Client{Timeout: amTimeout}}, nil
}

// critFromEnv builds the publisher: nil without WOTAN_ADDR (no client, no
// counting). WOTAN_GRPC_ADDR defaults to the WOTAN_ADDR host on 18001. A
// missing ALERTS_SIGNING_KEY_FILE leaves the sink nil (refused, counted);
// a bad one is an error, with the publisher still returned so the mail
// path runs.
//
// The link is pkg/wotan-client's, from the process environment: the
// service key is AUTH_KEY_FILE (identity alert-mailer), TLS and its CA are
// UNHEADED_TLS_ENABLED and UNHEADED_TLS_CA_CERT, and WOTAN_TLS_SERVER_NAME
// names the certificate when WOTAN_ADDR is an address it does not carry.
// Those three touch the Wotan client only: this binary's listeners are
// plain net/http (TestListenersDoNotReadTheTLSVariable).
func critFromEnv(getenv func(string) string, reg *metrics.Registry) (*critPublisher, error) {
	addr := strings.TrimSpace(getenv("WOTAN_ADDR"))
	if addr == "" {
		return nil, nil
	}
	grpcAddr := transport.WotanGRPCAddr(addr, strings.TrimSpace(getenv("WOTAN_GRPC_ADDR")))
	wotanClient.InitServiceIdentity("alert-mailer") // ADR-103 §6
	c, err := transport.NewWotanClient(transport.Config{WotanHTTPAddr: addr, WotanGRPCAddr: grpcAddr})
	if err != nil {
		return nil, fmt.Errorf("wotan client: %w", err)
	}
	am, err := alertmanagerFromEnv(getenv)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	keyPath := strings.TrimSpace(getenv("ALERTS_SIGNING_KEY_FILE"))
	if keyPath == "" {
		_ = c.Close() // nothing is published without a key
		log.Warn().Msg("ALERTS_SIGNING_KEY_FILE is not set: alerts.critical is refused, never sent unsigned")
		p := newCritPublisher(nil, reg)
		p.am = am
		return p, nil
	}
	signer, err := wotansig.LoadSignerFile(keyPath)
	if err != nil {
		_ = c.Close()
		p := newCritPublisher(nil, reg)
		p.am = am
		return p, fmt.Errorf("ALERTS_SIGNING_KEY_FILE: %w", err)
	}
	log.Info().Str("wotan", addr).Str("wotan_grpc", grpcAddr).
		Bool("service_key", wotanClient.ServiceIdentity() != nil).
		Msg("alerts.critical: publishing signed, as alert-mailer")
	p := newCritPublisher(&wotanCritSink{c: c, signer: signer}, reg)
	p.am = am
	return p, nil
}
