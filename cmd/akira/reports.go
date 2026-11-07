// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/health"
	"unheaded/pkg/healthreport"
	"unheaded/pkg/metrics"
	wotanClient "unheaded/pkg/wotan-client"
)

// reportGate decides which messages on system.health.reports become votes.
// With a verified sender the reporter comes from the token, not the report
// (healthreport.Admit). With Akira's auth on, a report without one is
// refused: Wotan stamps every publish it verified.
type reportGate struct {
	self            string
	requireVerified bool
	refused         *metrics.CounterVec
	now             func() time.Time
	logged          map[string]time.Time // reason -> last log line
}

func newReportGate(self string, requireVerified bool, reg *metrics.Registry) *reportGate {
	cv := metrics.NewCounterVec("akira_reports_refused_total",
		"Messages on system.health.reports that did not become a vote, by reason.",
		nil, []string{"reason"})
	if err := reg.Register(cv); err != nil {
		log.Error().Err(err).Msg("akira_reports_refused_total not registered")
	}
	return &reportGate{
		self: self, requireVerified: requireVerified, refused: cv,
		now: time.Now, logged: map[string]time.Time{},
	}
}

// refusalReason maps Admit's error to a metric label from a fixed set.
func refusalReason(err error) string {
	switch {
	case errors.Is(err, healthreport.ErrUnverified):
		return "unverified"
	case errors.Is(err, healthreport.ErrNotOwnService):
		return "not_own_service"
	case errors.Is(err, healthreport.ErrBadNode):
		return "bad_node"
	default:
		return "malformed"
	}
}

// refuse counts a refused message and logs each reason at most once per
// refusedQuiet: a flood of reports costs one line an hour. The sender is the
// verified subject, so the line names who did it.
func (g *reportGate) refuse(reason, sender string) {
	g.refused.WithLabelValues(reason).Inc()
	now := g.now()
	if at, ok := g.logged[reason]; ok && now.Sub(at) < refusedQuiet {
		return
	}
	g.logged[reason] = now
	log.Warn().Str("reason", reason).Str("sender", sender).Msg("health report refused; further refusals this hour are only counted")
}

// consume feeds admitted reports from ch into the ballot until ch closes.
func (g *reportGate) consume(ch <-chan *wotanClient.Message, a *health.Akira) {
	for m := range ch {
		r, err := healthreport.Admit(m.VerifiedSender, g.requireVerified, []byte(m.Payload))
		if err != nil {
			g.refuse(refusalReason(err), m.VerifiedSender)
			continue
		}
		if r.Reporter == g.self {
			continue // this node's vote is recorded by CheckAll
		}
		a.Record(r)
	}
}

// consumeReports feeds other reporters' reports from Wotan into the ballot.
func consumeReports(c *wotanClient.Client, g *reportGate, a *health.Akira) {
	ch, err := c.StreamMessages(context.Background(), topicReports)
	if err != nil {
		log.Warn().Err(err).Msg("cannot read system.health.reports; tallying this node's checks only")
		return
	}
	g.consume(ch, a)
}
