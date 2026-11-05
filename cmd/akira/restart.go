// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"fmt"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/health"
)

// maxRefusedLogs bounds the log lines for verdicts on services this node does
// not watch: the names are not ours, and each one comes back every sweep.
const maxRefusedLogs = 64

// restarter is what --remediate does with a verdict at the threshold.
type restarter struct {
	noUnit  map[string]bool   // exec targets: nothing to restart
	units   map[string]string // service -> unit, from restartUnits
	enabled bool              // --remediate
	counts  map[string]int    // attempts this outage; -1 once a human was told
	refused int               // verdicts logged as not ours to restart
	run     func(unit string) ([]byte, error)
}

// restartUnits maps each service this node's configuration watches to its
// unit. A verdict's service is whatever the reports on system.health.reports
// say, and those are unsigned, so this table is the only way a name reaches
// systemctl. Exec targets have no unit.
func restartUnits(targets []health.ServiceTarget) map[string]string {
	units := make(map[string]string, len(targets))
	for _, t := range targets {
		if t.Exec == nil {
			units[t.Name] = fmt.Sprintf("unheaded-%s", t.Name)
		}
	}
	return units
}

func (r *restarter) onAlert(v health.Verdict) {
	if r.noUnit[v.Service] {
		// An exec check (cert-expiry) has no unit to restart: its own
		// output, in the report's error, says what a human must do.
		if r.counts[v.Service] == 0 {
			log.Error().Str("service", v.Service).Float64("failure_rate", v.FailureRate).
				Msg("CONSENSUS THRESHOLD on an exec check — no unit to restart: needs a human")
			r.counts[v.Service] = -1 // logged for this outage
		}
		return
	}
	if !r.enabled {
		if r.counts[v.Service] == 0 {
			log.Error().Str("service", v.Service).Float64("failure_rate", v.FailureRate).
				Msg("CONSENSUS THRESHOLD — remediation off (--remediate=false): needs a human")
			r.counts[v.Service] = -1 // logged for this outage
		}
		return
	}
	svcUnit, ok := r.units[v.Service]
	if !ok {
		// A peer's service, or a name nothing here knows: no count is kept,
		// so a stream of new names cannot grow the table.
		if r.refused < maxRefusedLogs {
			r.refused++
			log.Warn().Str("service", v.Service).
				Msg("CONSENSUS THRESHOLD on a service this node does not watch — not restarting: needs a human")
		}
		return
	}
	n := r.counts[v.Service]
	if n >= health.MaxAutoRestarts {
		if n == health.MaxAutoRestarts {
			log.Error().Str("service", v.Service).Int("restarts", n).
				Msg("MAX AUTO-RESTARTS EXCEEDED — escalating to human")
			r.counts[v.Service] = n + 1 // escalate once
		}
		return
	}
	r.counts[v.Service] = n + 1
	log.Warn().Str("service", v.Service).Str("unit", svcUnit).Int("attempt", n+1).
		Msg("attempting auto-restart via systemctl")
	if out, err := r.run(svcUnit); err != nil {
		log.Error().Err(err).Str("output", string(out)).Str("service", v.Service).Msg("auto-restart FAILED")
	} else {
		log.Info().Str("service", v.Service).Msg("auto-restart SUCCESS")
	}
}
