// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/health"
	"unheaded/pkg/metrics"
	wotanClient "unheaded/pkg/wotan-client"
)

func reportMessage(t *testing.T, sender string, r health.HealthReport) *wotanClient.Message {
	t.Helper()
	r.Timestamp = time.Now()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return &wotanClient.Message{VerifiedSender: sender, Payload: string(b)}
}

func consumeAll(g *reportGate, a *health.Akira, msgs ...*wotanClient.Message) {
	ch := make(chan *wotanClient.Message, len(msgs))
	for _, m := range msgs {
		ch <- m
	}
	close(ch)
	g.consume(ch, a)
}

func verdictOn(t *testing.T, a *health.Akira, service string) health.Verdict {
	t.Helper()
	a.EvaluateConsensus()
	v, ok := a.GetStates()[service]
	if !ok {
		t.Fatalf("no verdict on %s", service)
	}
	return v
}

// H6: one stolen identity cannot out-vote a service's real reporters.
func TestReportGate_OneIdentityCannotOutVoteTheBallot(t *testing.T) {
	a := health.NewAkira("west", nil)
	g := newReportGate("west", true, metrics.NewRegistry())
	msgs := []*wotanClient.Message{
		reportMessage(t, "wotan", health.HealthReport{Service: "wotan", Host: "west", Healthy: true}),
		reportMessage(t, "akira", health.HealthReport{Service: "wotan", Reporter: "east", Healthy: true}),
	}
	for i := range 50 {
		msgs = append(msgs, reportMessage(t, "kev-poller", health.HealthReport{
			Service: "wotan", Reporter: fmt.Sprintf("invented-%d", i), Host: fmt.Sprintf("host-%d", i),
		}))
	}
	consumeAll(g, a, msgs...)
	v := verdictOn(t, a, "wotan")
	if v.Reporters != 2 || v.Failing != 0 {
		t.Fatalf("verdict = %+v, want 2 reporters and none failing", v)
	}
	if got := g.refused.WithLabelValues("not_own_service").Value(); got != 50 {
		t.Fatalf("refused = %v, want 50", got)
	}
}

func TestReportGate_ReporterIsRebuiltFromTheToken(t *testing.T) {
	a := health.NewAkira("west", nil)
	g := newReportGate("west", true, metrics.NewRegistry())
	consumeAll(g, a, reportMessage(t, "kanban-app", health.HealthReport{Service: "kanban", Reporter: "someone-else", Host: "east"}))
	v := verdictOn(t, a, "kanban")
	if len(v.FailingReporters) != 1 || v.FailingReporters[0] != "kanban@east" {
		t.Fatalf("failing reporters = %v, want [kanban@east]", v.FailingReporters)
	}
}

func TestReportGate_AuthOnRefusesAnUnverifiedReport(t *testing.T) {
	a := health.NewAkira("west", nil)
	g := newReportGate("west", true, metrics.NewRegistry())
	consumeAll(g, a, reportMessage(t, "", health.HealthReport{Service: "wotan", Reporter: "wotan@west", Host: "west"}))
	a.EvaluateConsensus()
	if len(a.GetStates()) != 0 {
		t.Fatalf("states = %+v, want none", a.GetStates())
	}
	if got := g.refused.WithLabelValues("unverified").Value(); got != 1 {
		t.Fatalf("refused = %v, want 1", got)
	}
}

func TestReportGate_AuthOffKeepsReportsAsWritten(t *testing.T) {
	a := health.NewAkira("west", nil)
	g := newReportGate("west", false, metrics.NewRegistry())
	consumeAll(g, a, reportMessage(t, "", health.HealthReport{Service: "wotan", Reporter: "east", Healthy: true}))
	if v := verdictOn(t, a, "wotan"); v.Reporters != 1 {
		t.Fatalf("verdict = %+v, want 1 reporter", v)
	}
}

// This node's vote is recorded by CheckAll: its echo through Wotan is not a
// second vote, and no peer can speak as this node.
func TestReportGate_SkipsThisNodesEcho(t *testing.T) {
	a := health.NewAkira("west", nil)
	g := newReportGate("west", true, metrics.NewRegistry())
	consumeAll(g, a, reportMessage(t, "akira", health.HealthReport{Service: "wotan", Reporter: "west"}))
	a.EvaluateConsensus()
	if len(a.GetStates()) != 0 {
		t.Fatalf("states = %+v, want none", a.GetStates())
	}
}

// A full ballot is one more counted refusal, not a growing table.
func TestReportGate_FullBallotIsCounted(t *testing.T) {
	a := health.NewAkira("west", nil)
	g := newReportGate("west", true, metrics.NewRegistry())
	var msgs []*wotanClient.Message
	for i := range health.MaxPeerReporters + 1 {
		msgs = append(msgs, reportMessage(t, "wotan", health.HealthReport{Service: "wotan", Host: fmt.Sprintf("host-%d", i)}))
	}
	consumeAll(g, a, msgs...)
	if v := verdictOn(t, a, "wotan"); v.Reporters != health.MaxPeerReporters {
		t.Fatalf("reporters = %d, want %d", v.Reporters, health.MaxPeerReporters)
	}
	if got := g.refused.WithLabelValues("ballot_full").Value(); got != 1 {
		t.Fatalf("ballot_full = %v, want 1", got)
	}
}

// A name the ballot will never hold is not a full ballot: the two call for
// different things from whoever reads the metric.
func TestReportGate_ANameThatDoesNotFitIsNotAFullBallot(t *testing.T) {
	a := health.NewAkira("west", nil)
	g := newReportGate("west", false, metrics.NewRegistry())
	consumeAll(g, a,
		reportMessage(t, "", health.HealthReport{Service: "wotan", Reporter: strings.Repeat("r", health.MaxPeerNameBytes+1)}),
		reportMessage(t, "", health.HealthReport{Service: "wotan"}),
	)
	if got := g.refused.WithLabelValues("bad_name").Value(); got != 2 {
		t.Fatalf("bad_name = %v, want 2", got)
	}
	if got := g.refused.WithLabelValues("ballot_full").Value(); got != 0 {
		t.Fatalf("ballot_full = %v, want 0", got)
	}
}

// The gate Akira runs takes its setting from AUTH_ENABLED: with auth on a
// report no one verified is no vote, and with it off it is.
func TestReportGateFromEnv_AuthOnRefusesAnUnverifiedReport(t *testing.T) {
	msg := func() *wotanClient.Message {
		return reportMessage(t, "", health.HealthReport{Service: "wotan", Reporter: "east", Healthy: true})
	}

	t.Setenv("AUTH_ENABLED", "true")
	a := health.NewAkira("west", nil)
	g := reportGateFromEnv("west", metrics.NewRegistry())
	consumeAll(g, a, msg())
	a.EvaluateConsensus()
	if len(a.GetStates()) != 0 {
		t.Fatalf("auth on: states = %+v, want none", a.GetStates())
	}
	if got := g.refused.WithLabelValues("unverified").Value(); got != 1 {
		t.Fatalf("unverified = %v, want 1", got)
	}

	t.Setenv("AUTH_ENABLED", "false")
	a = health.NewAkira("west", nil)
	consumeAll(reportGateFromEnv("west", metrics.NewRegistry()), a, msg())
	if v := verdictOn(t, a, "wotan"); v.Reporters != 1 {
		t.Fatalf("auth off: verdict = %+v, want 1 reporter", v)
	}
}

// Every refusal is a counted reason from a fixed set: a report can never
// mint a label value.
func TestReportGate_RefusalReasonsAreFixed(t *testing.T) {
	a := health.NewAkira("west", nil)
	reg := metrics.NewRegistry()
	g := newReportGate("west", true, reg)
	consumeAll(g, a,
		&wotanClient.Message{VerifiedSender: "wotan", Payload: "not json"},
		reportMessage(t, "wotan", health.HealthReport{Service: "wotan", Host: "bad host"}),
	)
	for _, reason := range []string{"malformed", "bad_node"} {
		if got := g.refused.WithLabelValues(reason).Value(); got != 1 {
			t.Errorf("%s = %v, want 1", reason, got)
		}
	}
	var b strings.Builder
	if err := reg.Gather(&b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "bad host") {
		t.Fatalf("a reported name reached a label:\n%s", b.String())
	}
}
