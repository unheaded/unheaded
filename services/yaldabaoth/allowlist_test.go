// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"strings"
	"testing"
	"time"
)

func checkEnv(res Resolver, host string) CheckEnv {
	return CheckEnv{Resolver: res, Hostname: func() (string, error) { return host, nil }, DefaultDuration: time.Minute}
}

func TestAllowlist_SeedParses(t *testing.T) {
	a := testAllowlist(t)
	if a.LXD.Project != "unheaded" || a.Limits.MaxConcurrent != 1 || a.Limits.MaxDuration.D() != 10*time.Minute {
		t.Fatalf("seed = %+v", a)
	}
}

func TestAllowlist_AllowsTheLabelledInstance(t *testing.T) {
	a := testAllowlist(t)
	rt, err := a.Check(context.Background(), delayExp("steel-echo-v2-0", time.Minute), checkEnv(newResolver(), "west"))
	if err != nil {
		t.Fatal(err)
	}
	if rt.Kind != KindLXD || rt.ID != "steel-echo-v2-0" || rt.IP != "10.10.20.31" {
		t.Fatalf("resolved %+v", rt)
	}
}

// Every refusal the plan lists (Step 267).
func TestAllowlist_Refusals(t *testing.T) {
	a := testAllowlist(t)
	res := newResolver()
	type tc struct {
		exp    *Experiment
		host   string
		reason RefusalReason
	}
	sys := func(unit string) *Experiment {
		return &Experiment{Type: ExpTypeProcessKill, Target: Target{Kind: KindSystemd, ID: unit}, Schedule: &Schedule{Duration: time.Minute}}
	}
	withParams := func(p Params) *Experiment {
		e := delayExp("steel-echo-v2-0", time.Minute)
		e.Parameters = p
		return e
	}
	cases := map[string]tc{
		"pihole by name":            {delayExp("pihole", time.Minute), "west", RefuseAllowlist},
		"unmanaged instance":        {delayExp("unmanaged-0", time.Minute), "west", RefuseAllowlist},
		"instance without label":    {delayExp("plain-0", time.Minute), "west", RefuseAllowlist},
		"another project":           {delayExp("other-project-0", time.Minute), "west", RefuseAllowlist},
		"unknown instance":          {delayExp("nope-0", time.Minute), "west", RefuseAllowlist},
		"instance name junk":        {delayExp("../../etc", time.Minute), "west", RefuseAllowlist},
		"unit not listed":           {sys("unheaded-wotan.service"), "east", RefuseAllowlist},
		"listed unit, not east":     {sys("unheaded-timeguru.service"), "west", RefuseAllowlist},
		"a target unit":             {sys("multi-user.target"), "east", RefuseAllowlist},
		"ssh":                       {sys("ssh.service"), "east", RefuseAllowlist},
		"veth named in the request": {&Experiment{Type: ExpTypeNetworkDelay, Target: Target{Kind: KindVeth, ID: "veth1234"}, Parameters: Params{"delay_ms": 10.0}}, "west", RefuseAllowlist},
		"physical NIC as netns":     {&Experiment{Type: ExpTypeNetworkDelay, Target: Target{Kind: KindNetns, ID: "enp16s0"}, Parameters: Params{"delay_ms": 10.0}}, "west", RefuseAllowlist},
		"netns prefix trick":        {&Experiment{Type: ExpTypeNetworkDelay, Target: Target{Kind: KindNetns, ID: "steel1;rm"}, Parameters: Params{"delay_ms": 10.0}}, "west", RefuseAllowlist},
		"packet on a real NIC":      {&Experiment{Type: ExpTypeNetworkLoss, Target: Target{Kind: KindPacket, ID: "enp16s0"}, Parameters: Params{"loss_percent": 1.0}}, "west", RefuseAllowlist},
		"unknown kind":              {&Experiment{Type: ExpTypeNetworkDelay, Target: Target{Kind: "host", ID: "west"}}, "west", RefuseAllowlist},
		"kind does not fit type":    {&Experiment{Type: ExpTypeContainerKill, Target: Target{Kind: KindNetns, ID: "steel1"}}, "west", RefuseAllowlist},
		"duration over the limit":   {delayExp("steel-echo-v2-0", 11*time.Minute), "west", RefuseLimits},
		"loss over the limit":       {withParams(Params{"loss_percent": 51.0}), "west", RefuseLimits},
		"delay over the limit":      {withParams(Params{"delay_ms": 1900.0, "jitter_ms": 200.0}), "west", RefuseLimits},
		"negative delay":            {withParams(Params{"delay_ms": -5.0}), "west", RefuseLimits},
		"delay not a number":        {withParams(Params{"delay_ms": "lots"}), "west", RefuseLimits},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := a.Check(context.Background(), c.exp, checkEnv(res, c.host))
			if !isRefusal(err, c.reason) {
				t.Fatalf("err = %v, want a %s refusal", err, c.reason)
			}
		})
	}
	// The listed unit on east passes.
	if _, err := a.Check(context.Background(), sys("unheaded-timeguru.service"), checkEnv(res, "east")); err != nil {
		t.Fatalf("listed unit on east: %v", err)
	}
}

func TestAllowlist_HardDenyBeatsTheFile(t *testing.T) {
	a, err := ParseAllowlist([]byte(`lxd: {project: unheaded, label: chaos=allowed}
netns: '^steel[0-9]+$'
packet_interfaces: '^steel[0-9]+-h$'
systemd: {host: east, units: [ssh.service, dbus.service, systemd-journald.service, default.target, unheaded-timeguru.service]}
limits: {max_concurrent: 1, max_duration: 10m, max_loss_percent: 50, max_delay: 2s}
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range []string{"ssh.service", "dbus.service", "systemd-journald.service", "default.target"} {
		e := &Experiment{Type: ExpTypeProcessKill, Target: Target{Kind: KindSystemd, ID: unit}}
		if _, err := a.Check(context.Background(), e, checkEnv(newResolver(), "east")); !isRefusal(err, RefuseAllowlist) {
			t.Errorf("%s listed in the file was allowed: %v", unit, err)
		}
	}
}

func TestParseAllowlist_Refuses(t *testing.T) {
	good := "lxd: {project: unheaded, label: chaos=allowed}\nnetns: '^steel[0-9]+$'\npacket_interfaces: '^steel[0-9]+-h$'\n" +
		"systemd: {host: east, units: [unheaded-timeguru.service]}\nlimits: {max_concurrent: 1, max_duration: 10m, max_loss_percent: 50, max_delay: 2s}\n"
	if _, err := ParseAllowlist([]byte(good)); err != nil {
		t.Fatal(err)
	}
	bad := map[string]string{
		"unanchored netns":    strings.Replace(good, `'^steel[0-9]+$'`, `'steel'`, 1),
		"match-all netns":     strings.Replace(good, `'^steel[0-9]+$'`, `'^.*$'`, 1),
		"bad regexp":          strings.Replace(good, `'^steel[0-9]+$'`, `'^(steel$'`, 1),
		"no project":          strings.Replace(good, "project: unheaded, ", "", 1),
		"label without value": strings.Replace(good, "label: chaos=allowed", "label: chaos", 1),
		"default project":     strings.Replace(good, "project: unheaded", "project: default", 1),
		"concurrency 0":       strings.Replace(good, "max_concurrent: 1", "max_concurrent: 0", 1),
		"duration 0":          strings.Replace(good, "max_duration: 10m", "max_duration: 0s", 1),
		"loss over 100":       strings.Replace(good, "max_loss_percent: 50", "max_loss_percent: 101", 1),
		"unknown field":       good + "everything: true\n",
		"not yaml":            "{{",
	}
	for name, body := range bad {
		if _, err := ParseAllowlist([]byte(body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Every refusal is audited, counted and (rate-limited) published.
func TestRefusals_AreAudited(t *testing.T) {
	fx := newFixture(t)
	_, err := fx.s.StartExperiment(context.Background(), delayExp("pihole", time.Minute))
	if !isRefusal(err, RefuseAllowlist) {
		t.Fatalf("err = %v", err)
	}
	entries := fx.s.Audit().Query(AuditFilter{Action: "refuse"})
	if len(entries) != 1 || !strings.Contains(entries[0].RuleSpec, "pihole") {
		t.Fatalf("audit = %+v", entries)
	}
	if fx.pub.count(TopicRefused) != 1 {
		t.Fatalf("chaos.refused published %d times", fx.pub.count(TopicRefused))
	}
	// A flood of refusals is audited in full but published at a bounded rate.
	for i := 0; i < 100; i++ {
		_, _ = fx.s.StartExperiment(context.Background(), delayExp("pihole", time.Minute))
	}
	if n := len(fx.s.Audit().Query(AuditFilter{Action: "refuse"})); n != 101 {
		t.Fatalf("audited %d refusals, want 101", n)
	}
	if n := fx.pub.count(TopicRefused); n > refusalPublishBurst {
		t.Fatalf("published %d refusals, want at most %d", n, refusalPublishBurst)
	}
	if fx.inj.injectedCount() != 0 {
		t.Fatal("a refused experiment injected")
	}
}

func TestStartExperiment_SecondConcurrentIsRefused(t *testing.T) {
	fx := newFixture(t)
	first := fx.start(t, delayExp("steel-echo-v2-0", 2*time.Second))
	_, err := fx.s.StartExperiment(context.Background(), delayExp("steel-echo-v2-1", time.Second))
	if !isRefusal(err, RefuseConcurrency) {
		t.Fatalf("second concurrent experiment: %v", err)
	}
	fx.s.Abort("http", "test over")
	fx.wait(t, first.ID)
}
