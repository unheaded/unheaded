// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
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

// critPublisher keeps the critical alerts firing and publishes changes.
type critPublisher struct {
	sink    critSink // nil: no signing key, every publish refused and counted
	refused *metrics.Counter
	errors  *metrics.Counter

	mu     sync.Mutex
	active map[string]bool // fingerprints firing
}

func newCritPublisher(sink critSink, reg *metrics.Registry) *critPublisher {
	p := &critPublisher{
		sink:    sink,
		refused: metrics.NewCounter("alert_mailer_critical_publish_refused_total", "alerts.critical messages not sent because no signing key is set (ALERTS_SIGNING_KEY_FILE)", nil),
		errors:  metrics.NewCounter("alert_mailer_critical_publish_errors_total", "alerts.critical publishes that failed (Wotan down or refused)", nil),
		active:  map[string]bool{},
	}
	reg.MustRegister(p.refused)
	reg.MustRegister(p.errors)
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
		fp := trim(a.Fingerprint)
		p.mu.Lock()
		if status == "firing" {
			p.active[fp] = true
		} else {
			delete(p.active, fp)
		}
		p.mu.Unlock()
		payload, err := json.Marshal(critMsg{
			Fingerprint: fp,
			AlertName:   trim(a.Labels["alertname"]),
			Severity:    "critical",
			Status:      status,
			Host:        trim(a.Labels["host"]),
			StartsAt:    a.StartsAt,
		})
		if err != nil {
			p.errors.Inc()
			continue
		}
		p.send(payload)
	}
}

// Snapshot publishes the fingerprints firing now, sorted; an empty list
// too, so subscribers stay out of unknown.
func (p *critPublisher) Snapshot() {
	if p == nil {
		return
	}
	p.mu.Lock()
	fps := make([]string, 0, len(p.active))
	for fp := range p.active {
		fps = append(fps, fp)
	}
	p.mu.Unlock()
	sort.Strings(fps)
	payload, err := json.Marshal(map[string][]string{"snapshot": fps})
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
	}
}

// run publishes a snapshot on every tick until ctx ends.
func (p *critPublisher) run(ctx context.Context, tick <-chan time.Time) {
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

// critFromEnv builds the publisher: nil without WOTAN_ADDR (no client, no
// counting). WOTAN_GRPC_ADDR defaults to the WOTAN_ADDR host on 18001. A
// missing ALERTS_SIGNING_KEY_FILE leaves the sink nil (refused, counted);
// a bad one is an error, with the publisher still returned so the mail
// path runs.
func critFromEnv(getenv func(string) string, reg *metrics.Registry) (*critPublisher, error) {
	addr := strings.TrimSpace(getenv("WOTAN_ADDR"))
	if addr == "" {
		return nil, nil
	}
	wotanClient.InitServiceIdentity("alert-mailer") // ADR-103 §6
	c, err := transport.NewWotanClient(transport.Config{
		WotanHTTPAddr: addr,
		WotanGRPCAddr: transport.WotanGRPCAddr(addr, getenv("WOTAN_GRPC_ADDR")),
	})
	if err != nil {
		return nil, fmt.Errorf("wotan client: %w", err)
	}
	keyPath := strings.TrimSpace(getenv("ALERTS_SIGNING_KEY_FILE"))
	if keyPath == "" {
		log.Warn().Msg("ALERTS_SIGNING_KEY_FILE is not set: alerts.critical is refused, never sent unsigned")
		return newCritPublisher(nil, reg), nil
	}
	signer, err := wotansig.LoadSignerFile(keyPath)
	if err != nil {
		return newCritPublisher(nil, reg), fmt.Errorf("ALERTS_SIGNING_KEY_FILE: %w", err)
	}
	return newCritPublisher(&wotanCritSink{c: c, signer: signer}, reg), nil
}
