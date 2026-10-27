// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/sloguard"
)

// fakeResolver answers for LXD instances as Cuirass would.
type fakeResolver struct {
	mu   sync.Mutex
	inst map[string]*InstanceInfo
}

func newResolver() *fakeResolver {
	return &fakeResolver{inst: map[string]*InstanceInfo{
		"steel-echo-v2-0": {Name: "steel-echo-v2-0", Status: "running", IP: "10.10.20.31", Project: "unheaded", Managed: true,
			Labels: map[string]string{"chaos": "allowed", "svc": "steel-echo"}},
		"steel-echo-v2-1": {Name: "steel-echo-v2-1", Status: "running", IP: "10.10.20.32", Project: "unheaded", Managed: true,
			Labels: map[string]string{"chaos": "allowed", "svc": "steel-echo"}},
		"plain-0": {Name: "plain-0", Status: "running", IP: "10.10.20.33", Project: "unheaded", Managed: true},
		"unmanaged-0": {Name: "unmanaged-0", Status: "running", IP: "10.10.20.34", Project: "unheaded",
			Labels: map[string]string{"chaos": "allowed"}},
		"other-project-0": {Name: "other-project-0", Status: "running", IP: "10.10.20.35", Project: "default", Managed: true,
			Labels: map[string]string{"chaos": "allowed"}},
		// pihole as a misconfigured resolver might report it: still refused by name.
		"pihole": {Name: "pihole", Status: "running", IP: "10.10.20.2", Project: "unheaded", Managed: true,
			Labels: map[string]string{"chaos": "allowed"}},
	}}
}

func (r *fakeResolver) Instance(_ context.Context, name string) (*InstanceInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	in, ok := r.inst[name]
	if !ok {
		return nil, ErrInstanceNotFound
	}
	cp := *in
	return &cp, nil
}

// fakeGuard is a settable Kingdom SLO guard.
type fakeGuard struct {
	mu  sync.Mutex
	v   sloguard.Verdict
	err error
	n   int
}

func okGuard() *fakeGuard { return &fakeGuard{v: sloguard.Verdict{OK: true}} }

func (g *fakeGuard) set(v sloguard.Verdict, err error) {
	g.mu.Lock()
	g.v, g.err = v, err
	g.mu.Unlock()
}

func (g *fakeGuard) Check(context.Context) (sloguard.Verdict, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return g.v, g.err
}

// fakeInjector keeps an "active fault" set, so a test can see a fault go
// in and come out, and what the journal held at the moment of injection.
type fakeInjector struct {
	mu          sync.Mutex
	svc         *Service
	active      map[string]bool
	injected    int
	reverts     int
	journalSeen []int // journal size when Inject ran
	injectErr   error
	revertErr   error
	stuck       bool // Revert "works" but the fault stays (unverifiable revert)
}

func newFakeInjector() *fakeInjector { return &fakeInjector{active: map[string]bool{}} }

func (f *fakeInjector) Plan(_ context.Context, t ResolvedTarget, _ Params) (Recipe, error) {
	return Recipe{"fault": string(t.Kind) + "/" + t.ID}, nil
}

func (f *fakeInjector) Inject(_ context.Context, t ResolvedTarget, _ Params, r Recipe) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.svc != nil {
		f.journalSeen = append(f.journalSeen, len(f.svc.JournalEntries()))
	}
	f.injected++
	f.active[r["fault"]] = true // the fault lands, then the error (worst case)
	return f.injectErr
}

func (f *fakeInjector) Revert(_ context.Context, r Recipe) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reverts++
	if f.revertErr != nil {
		return f.revertErr
	}
	if !f.stuck {
		delete(f.active, r["fault"])
	}
	return nil
}

func (f *fakeInjector) Reverted(_ context.Context, r Recipe) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.active[r["fault"]], nil
}

func (f *fakeInjector) injectedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.injected
}

func (f *fakeInjector) activeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.active)
}

// recordingPublisher keeps events by topic.
type recordingPublisher struct {
	mu     sync.Mutex
	topics []string
}

func (p *recordingPublisher) Publish(_ context.Context, topic string, _ []byte) error {
	p.mu.Lock()
	p.topics = append(p.topics, topic)
	p.mu.Unlock()
	return nil
}

func (p *recordingPublisher) count(topic string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, t := range p.topics {
		if t == topic {
			n++
		}
	}
	return n
}

type fixture struct {
	s     *Service
	inj   *fakeInjector
	guard *fakeGuard
	res   *fakeResolver
	pub   *recordingPublisher
	dir   string
	abort string
}

func testAllowlist(t *testing.T) *Allowlist {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "configs", "chaos", "allowlist.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := ParseAllowlist(b)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func newFixture(t *testing.T, mutate ...func(*Config, *Deps)) *fixture {
	t.Helper()
	fx := &fixture{inj: newFakeInjector(), guard: okGuard(), res: newResolver(), pub: &recordingPublisher{}, dir: t.TempDir()}
	fx.abort = filepath.Join(t.TempDir(), "ABORT")
	cfg := DefaultConfig()
	cfg.DryRunMode = false
	cfg.StateDir = fx.dir
	cfg.AbortFile = fx.abort
	cfg.AbortPoll = 10 * time.Millisecond
	cfg.SafetyCheckInterval = 10 * time.Millisecond
	cfg.DefaultDuration = 200 * time.Millisecond
	cfg.RevertGrace = time.Second
	cfg.Hostname = func() (string, error) { return "west", nil }
	deps := Deps{Allowlist: testAllowlist(t), Guard: fx.guard, Resolver: fx.res, Publisher: fx.pub}
	for _, m := range mutate {
		m(cfg, &deps)
	}
	s, err := NewService(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	fx.inj.svc = s
	s.RegisterInjector(ExpTypeNetworkDelay, fx.inj)
	s.RegisterInjector(ExpTypeContainerKill, fx.inj)
	ctx, cancel := context.WithCancel(context.Background())
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); s.Stop() })
	fx.s = s
	return fx
}

func delayExp(id string, d time.Duration) *Experiment {
	return &Experiment{Name: "test", Type: ExpTypeNetworkDelay,
		Target:     Target{Kind: KindLXD, ID: id},
		Parameters: Params{"delay_ms": 200.0, "jitter_ms": 20.0},
		Schedule:   &Schedule{Duration: d}}
}

func (fx *fixture) start(t *testing.T, exp *Experiment) *Experiment {
	t.Helper()
	got, err := fx.s.StartExperiment(context.Background(), exp)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return got
}

func (fx *fixture) wait(t *testing.T, id string) *Experiment {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e, err := fx.s.Wait(ctx, id)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	return e
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func isRefusal(err error, reason RefusalReason) bool {
	var r *Refusal
	return errors.As(err, &r) && r.Reason == reason
}

// probeServer answers GET /health with status, after delay.
func probeServer(t *testing.T, status int, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			time.Sleep(delay)
		}
		w.WriteHeader(status)
		fmt.Fprint(w, "x")
	}))
	t.Cleanup(srv.Close)
	return srv
}
