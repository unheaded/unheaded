// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"reflect"
	"testing"

	"unheaded/pkg/health"
)

func testRestarter(targets []health.ServiceTarget, ran *[]string) *restarter {
	return &restarter{
		noUnit:  execTargetNames(targets),
		units:   restartUnits(targets),
		enabled: true,
		counts:  map[string]int{},
		run: func(unit string) ([]byte, error) {
			*ran = append(*ran, unit)
			return nil, nil
		},
	}
}

// A verdict's service comes from reports any Wotan publisher can send, so
// only a name in this node's own configuration reaches systemctl.
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

// Unbounded names on the ballot must not grow the attempt counts.
func TestRestart_UnknownServiceLeavesNoCount(t *testing.T) {
	t.Parallel()
	var ran []string
	r := testRestarter([]health.ServiceTarget{{Name: "wotan", Port: 18000}}, &ran)
	r.onAlert(health.Verdict{Service: "not-watched-here", Remediate: true})
	if len(r.counts) != 0 {
		t.Fatalf("counts = %v", r.counts)
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
