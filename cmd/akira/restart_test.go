// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"unheaded/pkg/health"
)

func testRestarter(targets []health.ServiceTarget, ran *[]string) *restarter {
	return &restarter{
		noUnit:  execTargetNames(targets),
		units:   restartUnits(targets),
		enabled: true,
		counts:  map[string]int{},
		refused: map[string]time.Time{},
		now:     time.Now,
		run: func(unit string) ([]byte, error) {
			*ran = append(*ran, unit)
			return nil, nil
		},
	}
}

// A verdict's service is only what the reports said, so only a name in this
// node's own configuration reaches systemctl.
func TestRestart_OnlyAConfiguredTarget(t *testing.T) {
	t.Parallel()
	targets := []health.ServiceTarget{
		{Name: "wotan", Port: 18000},
		{Name: "cert-expiry", Exec: &health.ExecCheck{Argv: []string{"/bin/true"}}},
	}
	var ran []string
	r := testRestarter(targets, &ran)
	for _, svc := range []string{"not-watched-here", "*", "wotan.service\nx", "../wotan", "cert-expiry"} {
		r.onAlert(health.Verdict{Service: svc, Remediate: true})
	}
	if len(ran) != 0 {
		t.Fatalf("restarted units no target names: %q", ran)
	}
	r.onAlert(health.Verdict{Service: "wotan", Remediate: true})
	if want := []string{"unheaded-wotan"}; !reflect.DeepEqual(ran, want) {
		t.Fatalf("restarted %q, want %q", ran, want)
	}
}

// captureLog collects what onAlert logs. The tests that use it must not be
// parallel: the logger is global.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = old })
	return &buf
}

// Names on the ballot are unbounded. What this node keeps and logs about the
// ones it does not watch must not be, with --remediate on or off.
func TestRestart_UnknownNamesAreBounded(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("remediate=%v", enabled), func(t *testing.T) {
			buf := captureLog(t)
			var ran []string
			r := testRestarter([]health.ServiceTarget{{Name: "wotan", Port: 18000}}, &ran)
			r.enabled = enabled
			for i := 0; i < 1000; i++ {
				r.onAlert(health.Verdict{Service: fmt.Sprintf("name-%d", i), Remediate: true})
			}
			if len(r.counts) != 0 {
				t.Errorf("%d names in the attempt counts", len(r.counts))
			}
			if lines := strings.Count(buf.String(), "\n"); lines > 64 {
				t.Errorf("%d log lines for names this node does not watch", lines)
			}
			if len(ran) != 0 {
				t.Errorf("restarted %q", ran)
			}
		})
	}
}

// One long outage on a peer is one line, so it cannot silence the next one.
func TestRestart_AnOutageOnAPeerLogsOnce(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("remediate=%v", enabled), func(t *testing.T) {
			buf := captureLog(t)
			var ran []string
			r := testRestarter([]health.ServiceTarget{{Name: "wotan", Port: 18000}}, &ran)
			r.enabled = enabled
			for i := 0; i < 200; i++ {
				r.onAlert(health.Verdict{Service: "peer-a", Remediate: true})
			}
			if n := strings.Count(buf.String(), "peer-a"); n != 1 {
				t.Errorf("%d lines for one outage, want 1", n)
			}
			r.onAlert(health.Verdict{Service: "peer-b", Remediate: true})
			if !strings.Contains(buf.String(), "peer-b") {
				t.Error("the next outage logged nothing")
			}
		})
	}
}

// Names that went quiet give their place back, so stale ones cannot silence
// this node for good, and a long outage is mentioned again.
func TestRestart_RefusedNamesExpire(t *testing.T) {
	buf := captureLog(t)
	var ran []string
	r := testRestarter([]health.ServiceTarget{{Name: "wotan", Port: 18000}}, &ran)
	clock := time.Unix(1000, 0)
	r.now = func() time.Time { return clock }
	for i := 0; i < maxRefused; i++ {
		r.onAlert(health.Verdict{Service: fmt.Sprintf("stale-%d", i), Remediate: true})
	}
	r.onAlert(health.Verdict{Service: "peer-a", Remediate: true})
	if strings.Contains(buf.String(), "peer-a") {
		t.Fatal("logged past the cap")
	}
	clock = clock.Add(refusedQuiet)
	r.onAlert(health.Verdict{Service: "peer-a", Remediate: true})
	if !strings.Contains(buf.String(), "peer-a") {
		t.Fatal("stale names silenced a real outage")
	}
	if len(r.refused) != 1 {
		t.Fatalf("%d names held, want 1", len(r.refused))
	}
}

// A configured target with --remediate off is told once, as before.
func TestRestart_OffStillTellsAHumanOnce(t *testing.T) {
	buf := captureLog(t)
	var ran []string
	r := testRestarter([]health.ServiceTarget{{Name: "wotan", Port: 18000}}, &ran)
	r.enabled = false
	for i := 0; i < 5; i++ {
		r.onAlert(health.Verdict{Service: "wotan", Remediate: true})
	}
	if n := strings.Count(buf.String(), "remediation off"); n != 1 {
		t.Fatalf("%d lines, want 1", n)
	}
	if len(ran) != 0 {
		t.Fatalf("restarted %q with --remediate off", ran)
	}
}

// The ceiling still holds for a target the node does watch.
func TestRestart_StopsAtTheCeiling(t *testing.T) {
	t.Parallel()
	var ran []string
	r := testRestarter([]health.ServiceTarget{{Name: "wotan", Port: 18000}}, &ran)
	for i := 0; i < health.MaxAutoRestarts+3; i++ {
		r.onAlert(health.Verdict{Service: "wotan", Remediate: true})
	}
	if len(ran) != health.MaxAutoRestarts {
		t.Fatalf("%d restarts, want %d", len(ran), health.MaxAutoRestarts)
	}
}
