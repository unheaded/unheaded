// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package server

import (
	"context"
	"encoding/json"
	"errors"

	"unheaded/pkg/auth"
	"unheaded/pkg/database"
	"unheaded/pkg/health"
	"unheaded/pkg/healthreport"
	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	wotanClient "unheaded/pkg/wotan-client"
)

// healthReportGate decides which messages on system.health.reports reach The
// Well: the check Akira applies (healthreport.Admit), so a row is kept only
// under the reporter the publisher's token allows, then the limits of Akira's
// ballot. service_health_current has a row per (service, reporter) and nothing
// expires them, and the host in a reporter is the publisher's word, so a run
// of this process writes rows for at most health.MaxPeerServices services and
// health.MaxPeerReporters reporters on each.
type healthReportGate struct {
	requireVerified bool
	refused         *healthreport.Refusals
	log             *logger.Logger
	rows            map[string]map[string]struct{} // service -> reporters
}

func newHealthReportGate(reg *metrics.Registry, log *logger.Logger) *healthReportGate {
	refused, err := healthreport.NewRefusals("dashboard_health_reports_refused_total",
		"Messages on system.health.reports not stored in The Well, by reason.", reg,
		func(reason, sender string) {
			log.Warn().Str("reason", reason).Str("sender", sender).Msg(healthreport.RefusedMessage)
		})
	if err != nil {
		log.Error().Err(err).Msg("dashboard_health_reports_refused_total not registered")
	}
	return &healthReportGate{
		requireVerified: auth.LoadServiceAuthConfig("dashboard-backend").Enabled,
		refused:         refused,
		log:             log,
		rows:            map[string]map[string]struct{}{},
	}
}

// holdsRow reports whether r may have a row, and notes it if so.
func (g *healthReportGate) holdsRow(r health.HealthReport) bool {
	reporters, known := g.rows[r.Service]
	if !known {
		if len(g.rows) >= health.MaxPeerServices {
			return false
		}
		reporters = map[string]struct{}{}
		g.rows[r.Service] = reporters
	}
	if _, ok := reporters[r.Reporter]; !ok {
		if len(reporters) >= health.MaxPeerReporters {
			return false
		}
		reporters[r.Reporter] = struct{}{}
	}
	return true
}

// consume stores every admitted report from ch until ch closes.
func (g *healthReportGate) consume(ctx context.Context, ch <-chan *wotanClient.Message, store func(context.Context, json.RawMessage) error) {
	for m := range ch {
		r, err := healthreport.Admit(m.VerifiedSender, g.requireVerified, []byte(m.Payload))
		if err != nil {
			g.refused.Refuse(healthreport.Reason(err), m.VerifiedSender)
			continue
		}
		if !g.holdsRow(r) {
			g.refused.Refuse("too_many_rows", m.VerifiedSender)
			continue
		}
		payload, err := json.Marshal(r)
		if err == nil {
			err = store(ctx, payload)
		}
		switch {
		case errors.Is(err, database.ErrBadReport):
			g.refused.Refuse("malformed", m.VerifiedSender)
		case err != nil:
			g.log.Warn().Err(err).Msg("health persistence: report not stored")
		}
	}
}
