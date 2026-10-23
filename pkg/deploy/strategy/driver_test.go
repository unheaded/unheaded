// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package strategy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// testDriver is an in-memory driver: the only place instances come from in
// these tests. The product path must never invent them (ErrNoDriver).
type testDriver struct {
	mu        sync.Mutex
	n         int
	live      map[string]Instance
	created   []string
	deleted   []string
	shifts    []TrafficShiftParams
	events    []string
	healthy   func(id string) bool
	createErr func(n int) error
	shiftErr  error
	analysis  func(req *AnalysisRequest) (*AnalysisResult, error)
	analyses  []*AnalysisRequest
}

func newTestDriver(initial ...Instance) *testDriver {
	d := &testDriver{live: map[string]Instance{}}
	for _, in := range initial {
		d.live[in.ID] = in
	}
	return d
}

func (d *testDriver) callbacks() *Callbacks {
	return &Callbacks{
		OnInstanceCreate: func(_ context.Context, p *InstanceParams) (*Instance, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.n++
			if d.createErr != nil {
				if err := d.createErr(d.n); err != nil {
					return nil, err
				}
			}
			id := fmt.Sprintf("%s-%s-%d", p.ServiceName, p.Version, d.n)
			in := Instance{ID: id, Version: p.Version, State: InstanceRunning, Labels: p.Labels}
			d.live[id] = in
			d.created = append(d.created, id)
			d.events = append(d.events, "create "+id)
			return &in, nil
		},
		OnInstanceDelete: func(_ context.Context, id string) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			if _, ok := d.live[id]; !ok {
				return fmt.Errorf("no instance %s", id)
			}
			delete(d.live, id)
			d.deleted = append(d.deleted, id)
			d.events = append(d.events, "delete "+id)
			return nil
		},
		OnHealthCheck: func(_ context.Context, id string) (bool, error) {
			d.mu.Lock()
			defer d.mu.Unlock()
			if _, ok := d.live[id]; !ok {
				return false, fmt.Errorf("no instance %s", id)
			}
			if d.healthy != nil {
				return d.healthy(id), nil
			}
			return true, nil
		},
		OnTrafficShift: func(_ context.Context, p *TrafficShiftParams) error {
			d.mu.Lock()
			defer d.mu.Unlock()
			if d.shiftErr != nil {
				return d.shiftErr
			}
			d.shifts = append(d.shifts, *p)
			d.events = append(d.events, fmt.Sprintf("shift %s->%s %d", p.OldVersion, p.NewVersion, p.Weight))
			return nil
		},
		OnAnalysis: func(_ context.Context, req *AnalysisRequest) (*AnalysisResult, error) {
			d.mu.Lock()
			d.analyses = append(d.analyses, req)
			f := d.analysis
			d.mu.Unlock()
			if f == nil {
				return &AnalysisResult{Pass: true, Message: "test driver pass"}, nil
			}
			return f(req)
		},
	}
}

// versions returns the sorted versions of the live instances.
func (d *testDriver) versions() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var vs []string
	for _, in := range d.live {
		vs = append(vs, in.Version)
	}
	sort.Strings(vs)
	return vs
}

func v1Instances(n int) []Instance {
	out := make([]Instance, n)
	for i := range out {
		out[i] = Instance{ID: fmt.Sprintf("svc-a-v1-old%d", i), Version: "v1", State: InstanceRunning, Healthy: true,
			Labels: map[string]string{"svc": "svc-a"}}
	}
	return out
}

func fastConfig(t Type) *Config {
	c := DefaultConfig(t)
	c.TestDuration = 0
	c.ProgressDeadline = 5 * time.Second
	c.HealthCheck.Interval = 5 * time.Millisecond
	if c.Analysis != nil {
		c.Analysis.Interval = time.Millisecond
	}
	for i := range c.Steps {
		c.Steps[i].Pause = 0
	}
	return c
}

func params(t Type, cb *Callbacks, initial []Instance) *ExecuteParams {
	return &ExecuteParams{
		DeploymentID:   "dep-" + string(t),
		ServiceName:    "svc-a",
		CurrentVersion: "v1",
		TargetVersion:  "v2",
		Replicas:       3,
		Config:         fastConfig(t),
		Instances:      initial,
		Callbacks:      cb,
	}
}

func allStrategies() []Strategy {
	return []Strategy{NewRollingStrategy(), NewBlueGreenStrategy(), NewCanaryStrategy(), NewRecreateStrategy()}
}

// Without a driver no strategy may invent an instance: it fails with
// ErrNoDriver before doing anything, and its metrics show nothing created.
func TestStrategies_NilDriverFails(t *testing.T) {
	missing := map[string]func(cb *Callbacks){
		"nil callbacks":       nil,
		"no OnInstanceCreate": func(cb *Callbacks) { cb.OnInstanceCreate = nil },
		"no OnInstanceDelete": func(cb *Callbacks) { cb.OnInstanceDelete = nil },
		"no OnHealthCheck":    func(cb *Callbacks) { cb.OnHealthCheck = nil },
	}
	for _, s := range allStrategies() {
		for name, strip := range missing {
			t.Run(string(s.Type())+"/"+name, func(t *testing.T) {
				d := newTestDriver(v1Instances(3)...)
				var cb *Callbacks
				if strip != nil {
					cb = d.callbacks()
					strip(cb)
				}
				res, err := s.Execute(context.Background(), params(s.Type(), cb, v1Instances(3)))
				if !errors.Is(err, ErrNoDriver) {
					t.Fatalf("err = %v, want ErrNoDriver", err)
				}
				if res != nil && (res.Success || len(res.Instances) > 0 ||
					(res.Metrics != nil && res.Metrics.InstancesCreated > 0)) {
					t.Fatalf("a strategy without a driver reported %+v", res)
				}
				if len(d.created) != 0 || len(d.deleted) != 0 {
					t.Fatalf("driver touched: created %v deleted %v", d.created, d.deleted)
				}
			})
		}
	}
}

func TestBlueGreen_RequiresTrafficShift(t *testing.T) {
	d := newTestDriver(v1Instances(3)...)
	cb := d.callbacks()
	cb.OnTrafficShift = nil
	_, err := NewBlueGreenStrategy().Execute(context.Background(), params(TypeBlueGreen, cb, v1Instances(3)))
	if !errors.Is(err, ErrNoDriver) || !strings.Contains(err.Error(), "OnTrafficShift") {
		t.Fatalf("err = %v, want ErrNoDriver naming OnTrafficShift", err)
	}
	if len(d.created) != 0 {
		t.Fatalf("created %v before refusing", d.created)
	}
}

func TestCanary_RequiresAnalysis(t *testing.T) {
	d := newTestDriver(v1Instances(3)...)
	cb := d.callbacks()
	cb.OnAnalysis = nil
	_, err := NewCanaryStrategy().Execute(context.Background(), params(TypeCanary, cb, v1Instances(3)))
	if !errors.Is(err, ErrNoDriver) || !strings.Contains(err.Error(), "OnAnalysis") {
		t.Fatalf("err = %v, want ErrNoDriver naming OnAnalysis", err)
	}
	if len(d.created) != 0 {
		t.Fatalf("created %v before refusing", d.created)
	}
}

// A health check that never passes is a failure, never "assume healthy".
func TestStrategies_NeverHealthyFails(t *testing.T) {
	for _, s := range allStrategies() {
		t.Run(string(s.Type()), func(t *testing.T) {
			d := newTestDriver(v1Instances(3)...)
			d.healthy = func(id string) bool { return !strings.Contains(id, "-v2-") }
			p := params(s.Type(), d.callbacks(), v1Instances(3))
			p.Config.ProgressDeadline = 200 * time.Millisecond
			res, err := s.Execute(context.Background(), p)
			if err == nil && res != nil && res.Success {
				t.Fatalf("%s succeeded with instances that never became healthy", s.Type())
			}
		})
	}
}

// TestNoSimulationInDeployCore keeps the fake paths from coming back: the
// deploy core's product sources must not simulate, assume a pass, or
// invent instance IDs.
func TestNoSimulationInDeployCore(t *testing.T) {
	banned := regexp.MustCompile(`(?i)simulat|assum(e|ing) pass|would be gathered|in a real implementation`)
	dirs := []string{".", "../rollback", "../cuirassdriver"}
	scanned := 0
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f) // #nosec G304 -- fixed package directories
			if err != nil {
				t.Fatal(err)
			}
			scanned++
			for i, line := range strings.Split(string(b), "\n") {
				if banned.MatchString(line) {
					t.Errorf("%s:%d: %s", f, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
	if scanned < 6 {
		t.Fatalf("scanned only %d files: the deploy core moved?", scanned)
	}
}
