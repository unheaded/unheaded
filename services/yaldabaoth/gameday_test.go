// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"strings"
	"testing"
	"time"
)

func scheduler(fx *fixture) *GameDayScheduler {
	return NewGameDayScheduler(nil, fx.s, fx.pub)
}

func stageOf(name string, exps ...Experiment) Stage { return Stage{Name: name, Experiments: exps} }

func waitState(t *testing.T, g *GameDayScheduler, want GameDayState) GameDayStatus {
	t.Helper()
	var st GameDayStatus
	waitFor(t, "game day "+string(want), func() bool {
		st = g.Status()
		return st.State == want
	})
	return st
}

func TestGameDay_RunsStagesAsExperiments(t *testing.T) {
	fx := newFixture(t)
	g := scheduler(fx)
	err := g.Schedule(GameDayScenario{Name: "two", Duration: 10 * time.Second, Stages: []Stage{
		stageOf("first", *delayExp("steel-echo-v2-0", 50*time.Millisecond)),
		stageOf("second", *delayExp("steel-echo-v2-1", 50*time.Millisecond)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, g, StateCompleted)
	if fx.inj.injectedCount() != 2 || fx.inj.activeCount() != 0 {
		t.Fatalf("injected %d, active %d", fx.inj.injectedCount(), fx.inj.activeCount())
	}
	if len(fx.s.List()) != 2 {
		t.Fatal("stages did not run through the Service")
	}
}

func TestGameDay_StageFailureAborts(t *testing.T) {
	fx := newFixture(t)
	g := scheduler(fx)
	if err := g.Schedule(GameDayScenario{Name: "bad", Duration: 10 * time.Second, Stages: []Stage{
		stageOf("refused", *delayExp("pihole", 50*time.Millisecond)),
		stageOf("never", *delayExp("steel-echo-v2-1", 50*time.Millisecond)),
	}}); err != nil {
		t.Fatal(err)
	}
	st := waitState(t, g, StateAborted)
	if !strings.Contains(st.Error, "refused") || fx.inj.injectedCount() != 0 {
		t.Fatalf("status %+v, injected %d", st, fx.inj.injectedCount())
	}
}

func TestGameDay_StopRevertsTheRunningExperiment(t *testing.T) {
	fx := newFixture(t)
	g := scheduler(fx)
	if err := g.Schedule(GameDayScenario{Name: "long", Duration: time.Minute, Stages: []Stage{
		stageOf("long", *delayExp("steel-echo-v2-0", 30*time.Second)),
	}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "injection", func() bool { return fx.inj.activeCount() == 1 })
	g.Stop()
	waitState(t, g, StateAborted)
	waitFor(t, "revert", func() bool { return fx.inj.activeCount() == 0 && len(fx.s.JournalEntries()) == 0 })
}

// The legacy src/dst fault rules name no allowlisted target: they are
// refused, not published as "injected" while nothing happens.
func TestGameDay_LegacyRulesAreRefusedNotFaked(t *testing.T) {
	fx := newFixture(t)
	g := scheduler(fx)
	if err := g.Schedule(GameDayScenario{Name: "legacy", Duration: 10 * time.Second, Stages: []Stage{
		{Name: "rules", Rules: []FaultRule{{SrcServiceID: 1, DstServiceID: 2, FaultType: "DELAY", DelayUS: 50000, Duration: time.Second}}},
	}}); err != nil {
		t.Fatal(err)
	}
	st := waitState(t, g, StateAborted)
	if fx.inj.injectedCount() != 0 || fx.pub.count("chaos.injections") != 0 {
		t.Fatalf("legacy rule: injected %d, injections published %d (%+v)", fx.inj.injectedCount(), fx.pub.count("chaos.injections"), st)
	}
}

func TestRuleExperiment_Translation(t *testing.T) {
	e := RuleExperiment(FaultRule{SrcServiceID: 3, DstServiceID: 4, FaultType: "DROP", DropRatePPM: 250000, Duration: 2 * time.Second})
	if e.Type != ExpTypePacketChaos || e.Target.Kind != KindPacket || e.Parameters["mode"] != "marker" || e.Parameters["loss_percent"] != 25.0 || e.Schedule.Duration != 2*time.Second {
		t.Fatalf("translated %+v", e)
	}
	e = RuleExperiment(FaultRule{FaultType: "DELAY", DelayUS: 150000})
	if e.Type != ExpTypePacketChaos || e.Parameters["mode"] != "delay" || e.Parameters["delay_ms"] != 150.0 {
		t.Fatalf("translated %+v", e)
	}
	if e := RuleExperiment(FaultRule{FaultType: "CORRUPT"}); e.Type != ExpTypePacketChaos || e.Parameters["mode"] != "bitflip" {
		t.Fatalf("CORRUPT translated to %+v", e)
	}
	if e := RuleExperiment(FaultRule{FaultType: "DUPLICATE", DropRatePPM: 100000}); e.Type != ExpTypePacketChaos || e.Parameters["mode"] != "duplicate" || e.Parameters["duplicate_percent"] != 10.0 {
		t.Fatalf("DUPLICATE translated to %+v", e)
	}
	if e := RuleExperiment(FaultRule{FaultType: "MELT"}); e.Type != ExpTypeCustom {
		t.Fatalf("unknown fault translated to %s", e.Type)
	}
}

func TestGameDay_Validation(t *testing.T) {
	fx := newFixture(t)
	g := scheduler(fx)
	ok := stageOf("s", *delayExp("steel-echo-v2-0", time.Second))
	bad := map[string]GameDayScenario{
		"no name":     {Duration: time.Minute, Stages: []Stage{ok}},
		"no stages":   {Name: "x", Duration: time.Minute},
		"no duration": {Name: "x", Stages: []Stage{ok}},
		"empty stage": {Name: "x", Duration: time.Minute, Stages: []Stage{{Name: "e"}}},
		"stage name":  {Name: "x", Duration: time.Minute, Stages: []Stage{{Experiments: ok.Experiments}}},
	}
	for name, sc := range bad {
		if err := g.Schedule(sc); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if st := g.Status(); st.State != StateIdle {
		t.Fatalf("state %s", st.State)
	}
}

func TestGameDay_OneAtATime(t *testing.T) {
	fx := newFixture(t)
	g := scheduler(fx)
	long := GameDayScenario{Name: "a", Duration: time.Minute, Stages: []Stage{stageOf("s", *delayExp("steel-echo-v2-0", 30*time.Second))}}
	if err := g.Schedule(long); err != nil {
		t.Fatal(err)
	}
	if err := g.Schedule(long); err == nil {
		t.Fatal("a second game day started")
	}
	g.Stop()
	waitState(t, g, StateAborted)
}
