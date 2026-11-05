// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/health"
)

// A verdict on a service this node does not watch comes back every sweep, and
// reports can carry any number of names. So such a name is logged at most once
// per refusedQuiet, and at most maxRefused names are held at a time: that is
// all the memory and all the log lines they can cost.
const (
	maxRefused   = 64
	refusedQuiet = time.Hour
)

// restarter is what --remediate does with a verdict at the threshold.
type restarter struct {
	noUnit  map[string]bool      // exec targets: nothing to restart
	units   map[string]string    // service -> unit, from restartUnits
	enabled bool                 // --remediate
	counts  map[string]int       // this node's targets: attempts this outage; -1 once a human was told
	refused map[string]time.Time // other names: when each was last logged
	now     func() time.Time
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
	svcUnit, ok := r.units[v.Service]
	if !ok {
		r.refuse(v)
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

// refuse handles a verdict on a peer's service, or on a name nothing here knows.
func (r *restarter) refuse(v health.Verdict) {
	now := r.now()
	for name, at := range r.refused {
		if now.Sub(at) >= refusedQuiet {
			delete(r.refused, name)
		}
	}
	if _, quiet := r.refused[v.Service]; quiet || len(r.refused) >= maxRefused {
		return
	}
	r.refused[v.Service] = now
	msg := "CONSENSUS THRESHOLD on a service this node does not watch — not restarting: needs a human"
	if !r.enabled {
		msg = "CONSENSUS THRESHOLD — remediation off (--remediate=false): needs a human"
	}
	log.Error().Str("service", v.Service).Float64("failure_rate", v.FailureRate).Msg(msg)
}
