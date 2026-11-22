// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package collect

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/compliance/crosswalk"
)

// alert-route under a planned-stop mark (Stevie, 2026-11-22, option b): a
// link that is down under a live mark for its container or "all" passes only
// when the mark ends within PlannedStopMax. Without a window no mark counts.
func TestAlertRoutePlannedStop(t *testing.T) {
	const now = 1_800_000_000
	am := "prometheus=200\nalertmanager=000\nalert-mailer=200\nwell=1\nnow=1800000000\n"
	mark := func(scope string, until int) string {
		return "mark=" + scope + " " + strconv.Itoa(until) + "\n"
	}
	two := ProbeEnv{PlannedStopMax: 2 * time.Hour}
	for _, tc := range []struct {
		name string
		out  string
		env  ProbeEnv
		pass bool
	}{
		{"all up, no window", "prometheus=200\nalertmanager=200\nalert-mailer=200\nwell=1\nnow=1800000000\n", ProbeEnv{}, true},
		{"down, no mark", am, two, false},
		{"down, own mark ends in 30m", am + mark("unheaded-alertmanager", now+1800), two, true},
		{"down, own mark, no window set", am + mark("unheaded-alertmanager", now+1800), ProbeEnv{}, false},
		{"down, own mark ends in 3h", am + mark("unheaded-alertmanager", now+3*3600), two, false},
		{"down, own mark ended", am + mark("unheaded-alertmanager", now-1), two, false},
		{"down, mark all ends in 1h", am + mark("all", now+3600), two, true},
		{"down, another container's mark", am + mark("unheaded-prometheus", now+1800), two, false},
		{"down, the link's bare name is not a container", am + mark("alertmanager", now+1800), two, false},
		{"down, own mark, no host clock", strings.Replace(am, "now=1800000000\n", "", 1) + mark("unheaded-alertmanager", now+1800), two, false},
		{"down, own mark, bad until", am + "mark=unheaded-alertmanager soon\n", two, false},
		{"well down under the postgres mark", "prometheus=200\nalertmanager=200\nalert-mailer=200\nwell=0\nnow=1800000000\n" + mark("unheaded-postgres", now+600), two, true},
		{"two down, one marked", "prometheus=000\nalertmanager=000\nalert-mailer=200\nwell=1\nnow=1800000000\n" + mark("unheaded-alertmanager", now+600), two, false},
		{"empty", "", two, false},
	} {
		pass, detail := judgeAlertRoute(tc.out, tc.env)
		if pass != tc.pass {
			t.Errorf("%s: pass %v (%s), want %v", tc.name, pass, detail, tc.pass)
		}
		if pass && strings.Contains(tc.out, "=000") && !strings.Contains(detail, "planned stop") {
			t.Errorf("%s: a pass under a mark must say so: %s", tc.name, detail)
		}
	}
	// The map's plain Judge has no window: a mark never counts there.
	if pass, _ := Probes["alert-route"].Judge(am + mark("all", now+60)); pass {
		t.Error("alert-route's Judge honoured a mark without a window")
	}
}

// HostProbes hands its Env to a probe's JudgeEnv.
func TestHostProbesPassEnv(t *testing.T) {
	out := "@@alert-route\nprometheus=200\nalertmanager=000\nalert-mailer=200\nwell=1\nnow=1800000000\nmark=all 1800000600\n"
	for _, tc := range []struct {
		env  ProbeEnv
		want crosswalk.Verdict
	}{
		{ProbeEnv{}, crosswalk.VerdictFail},
		{ProbeEnv{PlannedStopMax: time.Hour}, crosswalk.VerdictPass},
	} {
		p := &HostProbes{Hostname: "west", Env: tc.env, Run: func(_ context.Context, _, script string) (string, error) {
			return tagged(t, script, out), nil
		}}
		recs, err := p.Collect(context.Background(), []crosswalk.Source{{Kind: crosswalk.KindHostProbe, Ref: "west:alert-route"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(recs) != 1 || recs[0].Verdict != tc.want {
			t.Errorf("env %+v: %+v, want one record %s", tc.env, recs, tc.want)
		}
	}
}

func TestPlannedStopMaxFromPolicy(t *testing.T) {
	for _, tc := range []struct {
		yaml string
		want time.Duration
		err  bool
	}{
		{"mailer:\n  mode: send\n", 0, false},
		{"planned_stop:\n  probe_max: 2h\n", 2 * time.Hour, false},
		{"planned_stop:\n  probe_max: 90m\n", 90 * time.Minute, false},
		{"planned_stop:\n  probe_max: 25h\n", 0, true}, // past a day the probe's point is lost
		{"planned_stop:\n  probe_max: 30s\n", 0, true},
		{"planned_stop:\n  probe_max: 1d\n", 0, true},
		{"planned_stop:\n  probe_max: soon\n", 0, true},
		{"planned_stop: [\n", 0, true},
	} {
		got, err := PlannedStopMax([]byte(tc.yaml))
		if (err != nil) != tc.err || got != tc.want {
			t.Errorf("%q: %v, %v; want %v, error %v", tc.yaml, got, err, tc.want, tc.err)
		}
	}
}
