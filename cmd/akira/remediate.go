// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"math"
	"regexp"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/health"
	"unheaded/pkg/metrics"
)

// topicRemediate carries Akira's remediation suggestions (ADR-029): one
// message each time a service's consensus enters a band at or above
// CRITICAL (62.5 %). It is a suggestion, not a command: nothing acts on it
// unless an operator, Zhen's Champion or Akira's own --remediate does.
const topicRemediate = "system.health.remediate"

// remediateSuggestion is the system.health.remediate payload.
type remediateSuggestion struct {
	Service   string          `json:"service"`
	Host      string          `json:"host"` // the Akira node that tallied it
	Band      health.Severity `json:"band"`
	Pct       float64         `json:"pct"` // failing reporters, percent (2 decimals)
	Suggested string          `json:"suggested"`
	At        int64           `json:"at"` // Unix ms
}

// unitSafe is what a service name must look like to be put into a
// systemctl command line: ballot names arrive from other reporters through
// Wotan, so they are not trusted to be unit names.
var unitSafe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// otherServices is the metric label for every name outside this node's
// targets: a report must not be able to mint a label value.
const otherServices = "other"

// suggestion is the last band a service got a suggestion for, and when.
type suggestion struct {
	band health.Severity
	at   time.Time
}

// remediator publishes a suggestion once per entry into CRITICAL or PANIC.
// A verdict that goes quiet sends no change, so an entry here can outlive its
// name on the ballot: names outside own take at most maxRefused places, and
// an entry refusedQuiet old gives its place back.
type remediator struct {
	node      string
	own       map[string]bool // this node's targets
	publish   func(topic string, v interface{})
	suggested *metrics.CounterVec
	now       func() time.Time

	mu   sync.Mutex
	band map[string]suggestion // last suggestion, per service
}

func newRemediator(node string, own map[string]bool, publish func(string, interface{}), reg *metrics.Registry) *remediator {
	cv := metrics.NewCounterVec("akira_remediate_suggested_total",
		"Remediation suggestions Akira published to system.health.remediate, one per entry into CRITICAL or PANIC.",
		nil, []string{"service"})
	if err := reg.Register(cv); err != nil {
		log.Error().Err(err).Msg("akira_remediate_suggested_total not registered")
	}
	return &remediator{
		node: node, own: own, publish: publish, suggested: cv, now: time.Now,
		band: map[string]suggestion{},
	}
}

// remediable reports whether s is at or above CRITICAL.
func remediable(s health.Severity) bool {
	return s == health.SeverityCritical || s == health.SeverityPanic
}

// onChange is Akira's OnSeverityChange hook.
func (r *remediator) onChange(v health.Verdict) {
	r.mu.Lock()
	prev, had := r.band[v.Service]
	if !remediable(v.Severity) {
		delete(r.band, v.Service)
		r.mu.Unlock()
		return
	}
	if had && prev.band == v.Severity {
		r.mu.Unlock()
		return
	}
	now := r.now()
	if !had && !r.own[v.Service] && !r.roomForOther(now) {
		r.mu.Unlock()
		return
	}
	r.band[v.Service] = suggestion{band: v.Severity, at: now}
	r.mu.Unlock()

	s := remediateSuggestion{
		Service: v.Service,
		Host:    r.node,
		Band:    v.Severity,
		Pct:     math.Round(v.FailureRate*10000) / 100,
		At:      now.UnixMilli(),
	}
	if unitSafe.MatchString(v.Service) {
		s.Suggested = "systemctl restart unheaded-" + v.Service + ".service"
	}
	label := otherServices
	if r.own[v.Service] {
		label = v.Service
	}
	r.suggested.WithLabelValues(label).Inc()
	r.publish(topicRemediate, s)
}

// roomForOther drops entries for other names that have gone stale and
// reports whether one more fits. Caller holds r.mu.
func (r *remediator) roomForOther(now time.Time) bool {
	others := 0
	for name, s := range r.band {
		if r.own[name] {
			continue
		}
		if now.Sub(s.at) >= refusedQuiet {
			delete(r.band, name)
			continue
		}
		others++
	}
	return others < maxRefused
}
