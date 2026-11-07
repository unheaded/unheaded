// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/auth"
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
	refused         *healthreport.Refusals
}

func newReportGate(self string, requireVerified bool, reg *metrics.Registry) *reportGate {
	refused, err := healthreport.NewRefusals("akira_reports_refused_total",
		"Messages on system.health.reports that did not become a vote, by reason.", reg,
		func(reason, sender string) {
			log.Warn().Str("reason", reason).Str("sender", sender).Msg(healthreport.RefusedMessage)
		})
	if err != nil {
		log.Error().Err(err).Msg("akira_reports_refused_total not registered")
	}
	return &reportGate{self: self, requireVerified: requireVerified, refused: refused}
}

// reportGateFromEnv is the gate Akira runs: its own auth setting decides.
func reportGateFromEnv(self string, reg *metrics.Registry) *reportGate {
	requireVerified := auth.LoadServiceAuthConfig("akira").Enabled
	if !requireVerified {
		log.Warn().Msg("auth off: reports carry no verified sender, so reporter names are the reports' own word")
	}
	return newReportGate(self, requireVerified, reg)
}

// consume feeds admitted reports from ch into the ballot until ch closes.
func (g *reportGate) consume(ch <-chan *wotanClient.Message, a *health.Akira) {
	for m := range ch {
		r, err := healthreport.Admit(m.VerifiedSender, g.requireVerified, []byte(m.Payload))
		if err != nil {
			g.refused.Refuse(healthreport.Reason(err), m.VerifiedSender)
			continue
		}
		if r.Reporter == g.self {
			continue // this node's vote is recorded by CheckAll
		}
		if !health.PeerNamesFit(r) {
			g.refused.Refuse("bad_name", m.VerifiedSender)
			continue
		}
		if !a.RecordPeer(r) {
			g.refused.Refuse("ballot_full", m.VerifiedSender)
		}
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
