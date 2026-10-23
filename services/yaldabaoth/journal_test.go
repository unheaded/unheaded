// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// The journal entry exists before the fault does; an injector that fails
// after the fault landed still has its fault reverted and the entry removed.
func TestJournal_WrittenBeforeInject(t *testing.T) {
	fx := newFixture(t)
	fx.inj.injectErr = errors.New("tc said no, after half applying")
	exp := fx.start(t, delayExp("steel-echo-v2-0", time.Second))
	e := fx.wait(t, exp.ID)
	if e.Status != StatusFailed {
		t.Fatalf("status %s", e.Status)
	}
	fx.inj.mu.Lock()
	seen := append([]int(nil), fx.inj.journalSeen...)
	reverts := fx.inj.reverts
	fx.inj.mu.Unlock()
	if len(seen) != 1 || seen[0] != 1 {
		t.Fatalf("journal size at injection = %v, want [1]", seen)
	}
	if reverts == 0 || fx.inj.activeCount() != 0 || len(fx.s.JournalEntries()) != 0 {
		t.Fatalf("after a failed injection: reverts %d, active %d, journal %d", reverts, fx.inj.activeCount(), len(fx.s.JournalEntries()))
	}
	// On disk too, 0600.
	fi, err := os.Stat(filepath.Join(fx.dir, journalFileName))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode %v", fi.Mode().Perm())
	}
}

// A controller that died mid-fault leaves a journal; the next one reverts it
// at start, before doing anything else.
func TestJournal_RevertOnStart(t *testing.T) {
	leftover := journalFile{Version: 1, Entries: []*JournalEntry{{
		ID: "chaos-crashed-1", Type: ExpTypeNetworkDelay,
		Target:  ResolvedTarget{Kind: KindLXD, ID: "steel-echo-v2-0", IP: "10.10.10.31"},
		Recipe:  Recipe{"fault": "lxd/steel-echo-v2-0"},
		Created: time.Now().Add(-time.Minute), Expires: time.Now().Add(time.Hour),
	}}}
	b, _ := json.Marshal(leftover)
	inj := newFakeInjector()
	inj.active["lxd/steel-echo-v2-0"] = true // the fault the dead controller left
	cfg := DefaultConfig()
	cfg.StateDir = t.TempDir()
	cfg.AbortFile = filepath.Join(t.TempDir(), "ABORT")
	if err := os.WriteFile(filepath.Join(cfg.StateDir, journalFileName), b, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewService(cfg, Deps{Allowlist: testAllowlist(t), Guard: okGuard(), Resolver: newResolver()})
	if err != nil {
		t.Fatal(err)
	}
	s.RegisterInjector(ExpTypeNetworkDelay, inj)
	if len(s.JournalEntries()) != 1 {
		t.Fatal("journal not loaded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if inj.activeCount() != 0 || len(s.JournalEntries()) != 0 {
		t.Fatalf("leftover fault after start: active %d journal %d", inj.activeCount(), len(s.JournalEntries()))
	}
}

func TestJournal_DamagedIsFatal(t *testing.T) {
	cfg := DefaultConfig()
	cfg.StateDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(cfg.StateDir, journalFileName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(cfg, Deps{}); err == nil {
		t.Fatal("a damaged journal was read as empty: its faults would never be reverted")
	}
}

// The TTL reverts a fault on its own, even if the experiment's own loop
// never gets there.
func TestJournal_TTLReverts(t *testing.T) {
	fx := newFixture(t, func(c *Config, _ *Deps) { c.RevertGrace = 50 * time.Millisecond })
	// Duration 100ms + grace 50ms = TTL 150ms; block the loop from ending
	// by keeping the experiment's own timer long: the TTL must cut it.
	exp := delayExp("steel-echo-v2-0", 100*time.Millisecond)
	fx.s.testHoldLoop = true
	got := fx.start(t, exp)
	waitFor(t, "TTL revert", func() bool { return fx.inj.activeCount() == 0 && fx.inj.injectedCount() == 1 })
	if len(fx.s.JournalEntries()) != 0 {
		t.Fatal("TTL reverted but the entry stayed")
	}
	e := fx.wait(t, got.ID)
	if e.Status != StatusAborted {
		t.Fatalf("status %s, want aborted by the TTL", e.Status)
	}
}

// A revert that cannot be verified keeps the entry and raises
// chaos.revert_failed; nothing pretends the fault is gone.
func TestJournal_UnverifiedRevertKeepsEntryAndAlerts(t *testing.T) {
	fx := newFixture(t)
	fx.inj.stuck = true
	exp := fx.start(t, delayExp("steel-echo-v2-0", 50*time.Millisecond))
	e := fx.wait(t, exp.ID)
	if e.Results.Success || e.Results.Reverted {
		t.Fatalf("an unverified revert counted as success: %+v", e.Results)
	}
	if len(fx.s.JournalEntries()) != 1 {
		t.Fatal("the entry of an unverified revert was dropped")
	}
	if fx.pub.count(TopicRevertFailed) == 0 {
		t.Fatal("no chaos.revert_failed")
	}
	found := false
	for _, f := range e.Results.Findings {
		if f.Severity == SeverityHigh {
			found = true
		}
	}
	if !found {
		t.Fatal("no High finding for the stuck fault")
	}
	// revert-all keeps failing (non-zero) until the fault really goes.
	if left, _ := fx.s.RevertAll(context.Background()); left != 1 {
		t.Fatalf("revert-all left %d, want 1", left)
	}
	fx.inj.mu.Lock()
	fx.inj.stuck = false
	fx.inj.mu.Unlock()
	if left, _ := fx.s.RevertAll(context.Background()); left != 0 {
		t.Fatalf("revert-all left %d after the fault cleared", left)
	}
	// Idempotent: a second run is a no-op.
	if left, err := fx.s.RevertAll(context.Background()); left != 0 || err != nil {
		t.Fatalf("second revert-all: %d %v", left, err)
	}
}

func TestAbort_AllThreePathsRevert(t *testing.T) {
	paths := map[string]func(fx *fixture){
		"http": func(fx *fixture) { fx.s.Abort("http", "operator") },
		"wotan": func(fx *fixture) {
			fx.s.HandleAbortMessage(&wotanClient.Message{Topic: TopicAbort, Payload: `{"reason":"game day over"}`})
		},
		"file": func(fx *fixture) { _ = os.WriteFile(fx.abort, nil, 0o600) },
	}
	for name, abort := range paths {
		t.Run(name, func(t *testing.T) {
			fx := newFixture(t)
			exp := fx.start(t, delayExp("steel-echo-v2-0", 10*time.Second))
			waitFor(t, "injection", func() bool { return fx.inj.activeCount() == 1 })
			abort(fx)
			e := fx.wait(t, exp.ID)
			if e.Status != StatusAborted {
				t.Fatalf("%s abort: status %s", name, e.Status)
			}
			if fx.inj.activeCount() != 0 || len(fx.s.JournalEntries()) != 0 {
				t.Fatalf("%s abort left the fault: active %d journal %d", name, fx.inj.activeCount(), len(fx.s.JournalEntries()))
			}
			if got := fx.s.abortCount(name); got < 1 {
				t.Fatalf("chaos_aborts_total{path=%q} = %d", name, got)
			}
		})
	}
}

func TestAbort_WotanJunkIsStillAnAbort(t *testing.T) {
	// The abort only ever takes chaos away, so a message it cannot parse
	// still aborts (the safe direction).
	fx := newFixture(t)
	exp := fx.start(t, delayExp("steel-echo-v2-0", 10*time.Second))
	waitFor(t, "injection", func() bool { return fx.inj.activeCount() == 1 })
	fx.s.HandleAbortMessage(&wotanClient.Message{Topic: TopicAbort, Payload: "not json"})
	if e := fx.wait(t, exp.ID); e.Status != StatusAborted {
		t.Fatalf("status %s", e.Status)
	}
	fx.s.HandleAbortMessage(nil) // must not panic
}

func TestNoStateDirRefusesEverything(t *testing.T) {
	fx := newFixture(t, func(c *Config, _ *Deps) { c.StateDir = "" })
	if _, err := fx.s.StartExperiment(context.Background(), delayExp("steel-echo-v2-0", time.Second)); !isRefusal(err, RefuseNoJournal) {
		t.Fatalf("no state dir: %v", err)
	}
}
