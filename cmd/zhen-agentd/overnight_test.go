// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/claudebridge"
	"unheaded/pkg/zhenagent"
)

// S-TWO-BRAINS Steps 238-241: overnight mode runs unattended, so every
// limit is a stop condition checked before each card, and each stop has
// a typed reason that lands in the morning report.

func TestParseWindow(t *testing.T) {
	day := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	at := func(h, m int) time.Time { return day.Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute) }
	w, err := parseWindow("01:00-07:00")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		t    time.Time
		want bool
	}{{at(0, 59), false}, {at(1, 0), true}, {at(6, 59), true}, {at(7, 0), false}, {at(13, 0), false}} {
		if got := w.contains(c.t); got != c.want {
			t.Errorf("01:00-07:00 at %s: %v", c.t.Format("15:04"), got)
		}
	}
	// Crossing midnight.
	w, err = parseWindow("22:30-02:00")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		t    time.Time
		want bool
	}{{at(22, 29), false}, {at(22, 30), true}, {at(23, 59), true}, {at(0, 0), true}, {at(1, 59), true}, {at(2, 0), false}, {at(12, 0), false}} {
		if got := w.contains(c.t); got != c.want {
			t.Errorf("22:30-02:00 at %s: %v", c.t.Format("15:04"), got)
		}
	}
	if end := w.endAfter(at(23, 0)); !end.Equal(at(26, 0)) {
		t.Errorf("end after 23:00 = %v", end)
	}
	for _, bad := range []string{"", "1-7", "25:00-07:00", "01:00-01:00", "01:00_07:00", "01:60-02:00"} {
		if _, err := parseWindow(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestMemAvailable_NeverCountsSwap(t *testing.T) {
	p := filepath.Join(t.TempDir(), "meminfo")
	content := "MemTotal:       15007000 kB\nMemFree:          200000 kB\nMemAvailable:    1048576 kB\nSwapTotal:      35651584 kB\nSwapFree:       31457280 kB\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := memAvailable(p)
	if err != nil || got != 1<<30 {
		t.Fatalf("memAvailable = %d, %v", got, err)
	}
	if err := os.WriteFile(p, []byte("SwapFree: 31457280 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := memAvailable(p); err == nil {
		t.Error("no MemAvailable line accepted (and swap must never stand in for it)")
	}
}

// nightRig is a worker plus the night's knobs, on a fake clock.
type nightRig struct {
	m      *zhenagent.MemStore
	r      *fakeRunner
	w      *worker
	oc     overnightConfig
	now    time.Time
	ledger *claudebridge.Ledger
}

func newNightRig(t *testing.T, cards int) *nightRig {
	t.Helper()
	m := zhenagent.NewMemStore()
	for i := range cards {
		m.AddCard(zhenagent.Card{ID: "card-" + string(rune('a'+i)), Title: "x", Description: goodDesc, Status: "todo", Type: "task", Owner: "zhen"})
	}
	r := &fakeRunner{write: true, out: passOutcome()}
	w, _, _, _ := newTestWorker(t, m, r)
	rig := &nightRig{m: m, r: r, w: w, now: time.Date(2026, 10, 23, 1, 0, 0, 0, time.Local)}
	ledger, err := claudebridge.NewLedger(filepath.Join(t.TempDir(), "spend-overnight.json"), 3_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	rig.ledger = ledger
	win, _ := parseWindow("01:00-07:00")
	meminfo := filepath.Join(t.TempDir(), "meminfo")
	if err := os.WriteFile(meminfo, []byte("MemAvailable: 9000000 kB\nSwapFree: 30000000 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rig.oc = overnightConfig{
		Window: win, MaxCards: 5, Ledger: ledger, StopFile: filepath.Join(t.TempDir(), "zhen-agentd.stop"),
		MemFloor: defaultMemFloor, MemInfo: meminfo, Poll: time.Minute,
		Now:   func() time.Time { return rig.now },
		Sleep: func(_ context.Context, d time.Duration) error { rig.now = rig.now.Add(d); return nil },
	}
	w.cfg.Now = rig.oc.Now
	return rig
}

func TestNight_MaxCards(t *testing.T) {
	rig := newNightRig(t, 8)
	rep := runNight(context.Background(), rig.w, rig.oc)
	if rep.Stop != stopMaxCards || rep.Tried != 5 || rep.Succeeded != 5 {
		t.Errorf("report = %+v", rep)
	}
}

func TestNight_WindowEnd(t *testing.T) {
	rig := newNightRig(t, 0)
	rep := runNight(context.Background(), rig.w, rig.oc)
	if rep.Stop != stopWindowEnd || rep.Tried != 0 {
		t.Errorf("report = %+v", rep)
	}
	if !rig.now.Before(time.Date(2026, 10, 23, 7, 1, 0, 0, time.Local)) {
		t.Errorf("polled past the window: %v", rig.now)
	}
}

func TestNight_OutsideWindowDoesNothing(t *testing.T) {
	rig := newNightRig(t, 3)
	rig.now = time.Date(2026, 10, 23, 12, 0, 0, 0, time.Local)
	rep := runNight(context.Background(), rig.w, rig.oc)
	if rep.Stop != stopOutsideWindow || len(rig.r.tasks) != 0 {
		t.Errorf("report = %+v", rep)
	}
}

func TestNight_StopFile(t *testing.T) {
	rig := newNightRig(t, 3)
	rig.r.hook = func() {
		if len(rig.r.tasks) == 2 {
			_ = os.WriteFile(rig.oc.StopFile, nil, 0o600)
		}
	}
	rig.w.cfg.Stop = rig.oc.stopRequested
	rep := runNight(context.Background(), rig.w, rig.oc)
	if rep.Stop != stopStopFile {
		t.Fatalf("report = %+v", rep)
	}
	// The card in flight when the file appeared finished its check but
	// committed nothing; no further card was started.
	if rep.Succeeded != 1 || rep.Cancelled != 1 || len(rig.r.tasks) != 2 {
		t.Errorf("report = %+v, tasks %d", rep, len(rig.r.tasks))
	}
}

func TestNight_SpendCap(t *testing.T) {
	rig := newNightRig(t, 3)
	r, err := rig.ledger.Reserve(0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Commit(3_000_000_000); err != nil {
		t.Fatal(err)
	}
	rep := runNight(context.Background(), rig.w, rig.oc)
	if rep.Stop != stopSpendCap || len(rig.r.tasks) != 0 {
		t.Errorf("report = %+v", rep)
	}
}

func TestNight_MemFloorSwapNotCounted(t *testing.T) {
	rig := newNightRig(t, 2)
	// 1 GiB available, 30 GiB of free swap: refused.
	if err := os.WriteFile(rig.oc.MemInfo, []byte("MemAvailable: 1048576 kB\nSwapFree: 31457280 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep := runNight(context.Background(), rig.w, rig.oc)
	if len(rig.r.tasks) != 0 || rep.Deferred == 0 || rep.Deferrals[deferMemFloor] == 0 {
		t.Errorf("a card ran under the memory floor: %+v", rep)
	}
	if n, _ := rig.m.Attempts(context.Background(), "card-a"); n != 0 {
		t.Error("a card was claimed under the memory floor")
	}
}

func TestNight_MemFloorDefault(t *testing.T) {
	// 2 GiB of headroom plus run_check's MemoryMax (2G).
	if defaultMemFloor != 4<<30 {
		t.Errorf("defaultMemFloor = %d", defaultMemFloor)
	}
}

func TestNight_TwoInfraFailuresStop(t *testing.T) {
	rig := newNightRig(t, 4)
	rig.r.err = errors.New("sandbox: systemd-run missing")
	rep := runNight(context.Background(), rig.w, rig.oc)
	if rep.Stop != stopInfraFailures || rep.Tried != 2 {
		t.Errorf("report = %+v", rep)
	}
}

func TestNight_ModelFailuresDoNotStop(t *testing.T) {
	rig := newNightRig(t, 3)
	out := passOutcome()
	out.Check = "fail"
	rig.r.out = out
	rep := runNight(context.Background(), rig.w, rig.oc)
	if rep.Stop == stopInfraFailures || rep.Failed < 3 {
		t.Errorf("report = %+v", rep)
	}
}

func TestNight_GPUBusyDefers(t *testing.T) {
	rig := newNightRig(t, 1)
	var calls atomic.Int32
	rig.w.cfg.GPUBusy = func() bool { return calls.Add(1) <= 3 }
	rep := runNight(context.Background(), rig.w, rig.oc)
	if rep.Deferrals[deferGPU] != 3 || rep.Succeeded != 1 {
		t.Errorf("report = %+v", rep)
	}
}

func TestNight_ContextCancelIsSignal(t *testing.T) {
	rig := newNightRig(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if rep := runNight(ctx, rig.w, rig.oc); rep.Stop != stopSignal {
		t.Errorf("report = %+v", rep)
	}
}

func TestOvernightFlags_ForceStrictPrivacy(t *testing.T) {
	wf := workerFlags{privacyMode: "standard", overnight: true}
	if err := applyOvernightDefaults(&wf, map[string]bool{"privacy": true}, "/state"); err != nil {
		t.Fatal(err)
	}
	if wf.privacyMode != "strict" || !wf.enabled {
		t.Errorf("overnight flags = %+v", wf)
	}
	if !strings.HasSuffix(wf.ledger, "zhen-claude/spend-overnight.json") || wf.dailyCapUSD != 3 {
		t.Errorf("ledger %q cap %v", wf.ledger, wf.dailyCapUSD)
	}
	// An explicit ledger or cap is kept; the interactive ledger is refused.
	wf = workerFlags{overnight: true, ledger: "/x/spend.json", dailyCapUSD: 1}
	if err := applyOvernightDefaults(&wf, map[string]bool{"ledger": true, "daily-cap-usd": true}, "/state"); err != nil || wf.ledger != "/x/spend.json" || wf.dailyCapUSD != 1 {
		t.Errorf("explicit: %+v %v", wf, err)
	}
	wf = workerFlags{overnight: true, ledger: "/state/zhen-claude/spend.json"}
	if err := applyOvernightDefaults(&wf, map[string]bool{"ledger": true}, "/state"); err == nil {
		t.Error("the overnight run may share the interactive ledger")
	}
}
