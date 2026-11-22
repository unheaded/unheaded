// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package collect

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ProbeEnv is what a probe's JudgeEnv may know beyond its own output: the
// operator's settings, never anything the probed host says.
type ProbeEnv struct {
	// PlannedStopMax: a link of the alert path that is down under a
	// planned-stop mark ending within this long passes alert-route
	// (observability/alerting/policy.yaml planned_stop.probe_max). Zero: no
	// mark counts.
	PlannedStopMax time.Duration
}

// The bounds of planned_stop.probe_max. Past a day the probe's point, a stack
// stopped for days, is lost; under a minute no mark could count.
const (
	plannedStopMaxLow  = time.Minute
	plannedStopMaxHigh = 24 * time.Hour
)

var probeMaxRe = regexp.MustCompile(`^[0-9]+[mh]$`)

// PlannedStopMax reads planned_stop.probe_max from the alerting policy.
// Absent: 0. A value outside 1m..24h, or not <n>m or <n>h, is an error.
func PlannedStopMax(policy []byte) (time.Duration, error) {
	var doc struct {
		PlannedStop struct {
			ProbeMax string `yaml:"probe_max"`
		} `yaml:"planned_stop"`
	}
	if err := yaml.Unmarshal(policy, &doc); err != nil {
		return 0, fmt.Errorf("alerting policy: %w", err)
	}
	v := doc.PlannedStop.ProbeMax
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if !probeMaxRe.MatchString(v) || err != nil || d < plannedStopMaxLow || d > plannedStopMaxHigh {
		return 0, fmt.Errorf("planned_stop.probe_max %q: want <n>m or <n>h, 1m to 24h", v)
	}
	return d, nil
}

// alertRouteLinks: each link of the alert path and the compose container a
// planned-stop mark names for it (scripts/monitoring/planned-stop.sh).
var alertRouteLinks = []struct{ link, container string }{
	{"prometheus", "unheaded-prometheus"},
	{"alertmanager", "unheaded-alertmanager"},
	{"alert-mailer", "unheaded-alert-mailer"},
	{"well", "unheaded-postgres"},
}

var markScopeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)

// alertRouteCommand asks each link directly, then prints the host's clock and
// the planned-stop marks (huginn's textfile directory, planned-stop.sh's
// default; world-readable).
const alertRouteCommand = `for t in prometheus=http://127.0.0.1:9090/-/healthy alertmanager=http://127.0.0.1:9093/-/healthy alert-mailer=http://127.0.0.1:19130/health; do ` +
	`echo "${t%%=*}=$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "${t#*=}")"; done; echo "well=$(ss -Hltn 'sport = :5432' | wc -l)"; ` +
	`echo "now=$(date +%s)"; sed -n 's/^unheaded_planned_stop_until_timestamp_seconds{scope="\([^"]*\)"} \([0-9][0-9]*\)$/mark=\1 \2/p' /var/lib/unheaded/textfile/planned_stop.prom`

// judgeAlertRoute: every link answers, or each one down is under a live mark
// for its container or "all" that ends within env.PlannedStopMax by the
// host's own clock.
func judgeAlertRoute(out string, env ProbeEnv) (bool, string) {
	f := parseKV(out)
	now, nowErr := strconv.ParseInt(f["now"], 10, 64)
	marks := map[string]int64{}
	for _, l := range strings.Split(out, "\n") {
		v, ok := strings.CutPrefix(strings.TrimSpace(l), "mark=")
		if !ok {
			continue
		}
		scope, until, ok := strings.Cut(v, " ")
		u, err := strconv.ParseInt(until, 10, 64)
		if !ok || err != nil || !markScopeRe.MatchString(scope) {
			continue
		}
		if u > marks[scope] {
			marks[scope] = u
		}
	}
	// planned names the mark that covers container, or "".
	planned := func(container string) string {
		if env.PlannedStopMax <= 0 || nowErr != nil {
			return ""
		}
		for _, s := range []string{container, "all"} {
			if u, ok := marks[s]; ok && u > now && time.Duration(u-now)*time.Second <= env.PlannedStopMax {
				return s + " until " + time.Unix(u, 0).UTC().Format("15:04Z")
			}
		}
		return ""
	}
	var down, excused []string
	for _, l := range alertRouteLinks {
		var bad string
		if l.link == "well" {
			if n, err := strconv.Atoi(f["well"]); err != nil || n < 1 {
				bad = "well not listening"
			}
		} else if f[l.link] != "200" {
			bad = l.link + " " + f[l.link]
		}
		if bad == "" {
			continue
		}
		if m := planned(l.container); m != "" {
			excused = append(excused, bad+" (planned stop "+m+")")
			continue
		}
		down = append(down, bad)
	}
	if len(down) > 0 {
		return false, "alert path down: " + strings.Join(down, ", ")
	}
	if len(excused) > 0 {
		return true, "alert path down under a planned stop: " + strings.Join(excused, ", ")
	}
	return true, "prometheus, alertmanager and alert-mailer answer; the Well's port has a listener"
}
