// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

// Package critalert tracks the critical alerts firing in the Kingdom, as
// alert-mailer publishes them on Wotan's alerts.critical (ADR-147 Phase 8,
// D1 = A, D2 = B). Every service that holds a Wotan client attaches one
// Tracker; the state is in its /health under critical_alerts, and a
// service that starts change (Sword, chaos-controller, zhen-agentd) holds
// it while one fires.
//
// A message is metadata only: {"fingerprint","alertname","severity",
// "status":"firing|resolved","host","starts_at"}, or every 5 min
// {"snapshot":[fingerprints]}. Anything else is refused and counted. When
// the publisher has been silent for SilentAfter the state is Unknown,
// which allows change, with one warn line.
package critalert

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
	wotanClient "unheaded/pkg/wotan-client"
)

// Topic is where alert-mailer publishes critical alerts (signed, ML-DSA-65).
const Topic = "alerts.critical"

const (
	// MaxActive bounds the active set; one more is refused ("full").
	MaxActive = 256
	// MaxField bounds every string field and snapshot entry, in bytes.
	MaxField = 128
	// SilentAfter: no message for this long (the snapshot comes every
	// 5 min) and the state is Unknown.
	SilentAfter = 15 * time.Minute
	// maxPayload bounds a message before it is decoded.
	maxPayload = 64 << 10
)

// Alert is one firing critical alert.
type Alert struct {
	Fingerprint string
	Name        string
	Severity    string
	Status      string
	Host        string
	StartsAt    time.Time
}

// State is what a service knows about critical alerts.
type State int

const (
	// OK: the publisher is talking and nothing critical fires.
	OK State = iota
	// Firing: at least one critical alert fires.
	Firing
	// Unknown: the publisher has been silent for SilentAfter.
	Unknown
)

func (s State) String() string {
	switch s {
	case Firing:
		return "firing"
	case Unknown:
		return "unknown"
	default:
		return "ok"
	}
}

// Subscriber is the part of the process's Wotan client Attach needs.
type Subscriber interface {
	SubscribeWithRetry(ctx context.Context, topic, name string, onMsg func(*wotanClient.Message)) error
}

var (
	activeGauge = auto.NewGauge(prom.GaugeOpts{
		Name: "unheaded_critical_alerts_active",
		Help: "Critical alerts firing, as this service last heard on alerts.critical",
	})
	stateGauge = auto.NewGaugeVec(prom.GaugeOpts{
		Name: "unheaded_critical_alerts_state",
		Help: "1 for this service's critical-alert state (ok, firing, unknown), 0 for the others",
	}, []string{"state"})
	refused = auto.NewCounterVec(prom.CounterOpts{
		Name: "unheaded_critical_alerts_refused_total",
		Help: "alerts.critical messages refused, by reason (decode, status, field, full)",
	}, []string{"reason"})
)

// Tracker holds the critical alerts firing, from alerts.critical.
type Tracker struct {
	now  func() time.Time
	logf func(format string, args ...any)

	mu     sync.Mutex
	active map[string]Alert
	last   time.Time // the last accepted message (or the start)
	shown  State     // the state last logged and exported
}

// Attach subscribes the process's client c to Topic as name (the display
// name the service already joins Wotan with) and returns the Tracker it
// feeds. Joining retries in the background until ctx ends. A nil c gives
// a Tracker that is never Firing.
func Attach(ctx context.Context, c Subscriber, name string) *Tracker {
	t := newTracker(time.Now, log.Printf)
	if c == nil {
		return t
	}
	if wc, ok := c.(*wotanClient.Client); ok && wc == nil {
		return t
	}
	go func() { // #nosec G118 -- runs until ctx ends
		_ = c.SubscribeWithRetry(ctx, Topic, name, t.Handle)
	}()
	go t.watch(ctx)
	return t
}

func newTracker(now func() time.Time, logf func(string, ...any)) *Tracker {
	t := &Tracker{now: now, logf: logf, active: map[string]Alert{}, last: now()}
	t.export(OK)
	return t
}

// watch re-evaluates the state each minute, so a silent publisher turns
// Unknown (and says so once) without anyone asking.
func (t *Tracker) watch(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.State()
		}
	}
}

// message is the wire shape; unknown fields are refused.
type message struct {
	Fingerprint string    `json:"fingerprint"`
	AlertName   string    `json:"alertname"`
	Severity    string    `json:"severity"`
	Status      string    `json:"status"`
	Host        string    `json:"host"`
	StartsAt    time.Time `json:"starts_at"`
	Snapshot    *[]string `json:"snapshot"`
}

// Handle applies one alerts.critical message. A message it refuses is
// counted by reason and changes nothing.
func (t *Tracker) Handle(m *wotanClient.Message) {
	if m == nil || m.Deleted {
		return
	}
	if len(m.Payload) > maxPayload {
		refused.WithLabelValues("field").Inc()
		return
	}
	var in message
	dec := json.NewDecoder(bytes.NewReader([]byte(m.Payload)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil || dec.More() {
		refused.WithLabelValues("decode").Inc()
		return
	}
	if in.Snapshot != nil {
		t.snapshot(in)
		return
	}
	t.alert(in)
}

func (t *Tracker) snapshot(in message) {
	if in.Fingerprint != "" || in.AlertName != "" || in.Severity != "" || in.Status != "" || in.Host != "" || !in.StartsAt.IsZero() || len(*in.Snapshot) > MaxActive {
		refused.WithLabelValues("field").Inc()
		return
	}
	keep := make(map[string]bool, len(*in.Snapshot))
	for _, fp := range *in.Snapshot {
		if fp == "" || len(fp) > MaxField {
			refused.WithLabelValues("field").Inc()
			return
		}
		keep[fp] = true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.last = t.now()
	for fp, a := range t.active {
		if !keep[fp] {
			delete(t.active, fp)
			t.logf("[critalert] critical alert %s (%s on %s) is no longer in the snapshot; %d firing", a.Name, fp, a.Host, len(t.active))
		}
	}
	t.update()
}

func (t *Tracker) alert(in message) {
	if in.Status != "firing" && in.Status != "resolved" {
		refused.WithLabelValues("status").Inc()
		return
	}
	for _, f := range []string{in.Fingerprint, in.AlertName, in.Severity, in.Host} {
		if len(f) > MaxField {
			refused.WithLabelValues("field").Inc()
			return
		}
	}
	if in.Fingerprint == "" {
		refused.WithLabelValues("field").Inc()
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, known := t.active[in.Fingerprint]
	if in.Status == "firing" && !known && len(t.active) >= MaxActive {
		refused.WithLabelValues("full").Inc()
		return
	}
	t.last = t.now()
	switch {
	case in.Status == "firing" && !known:
		t.active[in.Fingerprint] = Alert{Fingerprint: in.Fingerprint, Name: in.AlertName, Severity: in.Severity, Status: in.Status, Host: in.Host, StartsAt: in.StartsAt}
		t.logf("[critalert] critical alert firing: %s on %s (%s); %d firing", in.AlertName, in.Host, in.Fingerprint, len(t.active))
	case in.Status == "resolved" && known:
		delete(t.active, in.Fingerprint)
		t.logf("[critalert] critical alert resolved: %s on %s (%s); %d firing", in.AlertName, in.Host, in.Fingerprint, len(t.active))
	}
	t.update()
}

// update exports the state and logs a change to Unknown. t.mu is held.
func (t *Tracker) update() State {
	s := t.stateLocked()
	if s != t.shown {
		if s == Unknown {
			t.logf("[critalert] no alerts.critical message for %s: state unknown, change is allowed", SilentAfter)
		}
		t.shown = s
		t.export(s)
	}
	activeGauge.Set(float64(len(t.active)))
	return s
}

func (t *Tracker) stateLocked() State {
	switch {
	case t.now().Sub(t.last) > SilentAfter:
		return Unknown
	case len(t.active) > 0:
		return Firing
	default:
		return OK
	}
}

func (t *Tracker) export(s State) {
	for _, x := range []State{OK, Firing, Unknown} {
		v := 0.0
		if x == s {
			v = 1
		}
		stateGauge.WithLabelValues(x.String()).Set(v)
	}
}

// State is the current state.
func (t *Tracker) State() State {
	if t == nil {
		return Unknown
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.update()
}

// Firing reports whether a critical alert fires now. Unknown is not
// firing: a silent publisher allows change (with a warn line, once).
func (t *Tracker) Firing() bool { return t.State() == Firing }

// Active lists the firing alerts, by fingerprint.
func (t *Tracker) Active() []Alert {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Alert, 0, len(t.active))
	for _, a := range t.active {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Fingerprint < out[j].Fingerprint })
	return out
}

// Names lists the firing alerts' names, for a refusal message.
func (t *Tracker) Names() []string {
	var out []string
	for _, a := range t.Active() {
		out = append(out, a.Name)
	}
	return out
}

// Health is the /health detail: {"state":"ok|firing|unknown","active":N}.
func (t *Tracker) Health() map[string]any {
	s := t.State()
	return map[string]any{"state": s.String(), "active": len(t.Active())}
}

// Hold reports whether change is held (ADR-147 Phase 8, D2 = B): true while
// a critical alert fires, with "critical alert firing: <names>". Unknown
// allows; the move to unknown is logged once. A nil Tracker never holds.
func (t *Tracker) Hold() (bool, string) {
	if t == nil || !t.Firing() {
		return false, ""
	}
	return true, "critical alert firing: " + strings.Join(t.Names(), ", ")
}
