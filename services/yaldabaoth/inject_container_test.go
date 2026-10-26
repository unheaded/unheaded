// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCuirass is a settable container controller for the injector tests.
type fakeCuirass struct {
	mu           sync.Mutex
	enforce      bool
	chaos        bool
	modesErr     error
	killed       []string // "id action reason"
	killErr      error
	healthy      bool
	healthyAfter int // become healthy after this many Healthy() calls
	healthyCalls int
	healthyErr   error
}

func (c *fakeCuirass) Modes(context.Context) (bool, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enforce, c.chaos, c.modesErr
}

func (c *fakeCuirass) Chaos(_ context.Context, id, action, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.killErr != nil {
		return c.killErr
	}
	c.killed = append(c.killed, id+" "+action+" "+reason)
	return nil
}

func (c *fakeCuirass) Healthy(context.Context, string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.healthyCalls++
	if c.healthyErr != nil {
		return false, c.healthyErr
	}
	if c.healthyAfter > 0 {
		return c.healthyCalls >= c.healthyAfter, nil
	}
	return c.healthy, nil
}

func containerInjector(c *fakeCuirass, mttr time.Duration) *ContainerInjector {
	inj := NewContainerInjector(c, "kill", mttr, nil)
	inj.poll = 2 * time.Millisecond
	return inj
}

func TestContainer_RefusedWhenEnforceOff(t *testing.T) {
	inj := containerInjector(&fakeCuirass{enforce: false, chaos: true}, time.Second)
	if _, err := inj.Plan(t.Context(), ResolvedTarget{Kind: KindLXD, ID: "steel-echo-v2-1"}, nil); err == nil ||
		!strings.Contains(err.Error(), "enforce") {
		t.Fatalf("err = %v, want an enforce refusal", err)
	}
}

func TestContainer_RefusedWhenChaosOff(t *testing.T) {
	inj := containerInjector(&fakeCuirass{enforce: true, chaos: false}, time.Second)
	if _, err := inj.Plan(t.Context(), ResolvedTarget{Kind: KindLXD, ID: "x"}, nil); err == nil ||
		!strings.Contains(err.Error(), "chaos") {
		t.Fatalf("err = %v, want a chaos refusal", err)
	}
}

func TestContainer_InjectPostsKill(t *testing.T) {
	c := &fakeCuirass{enforce: true, chaos: true}
	inj := containerInjector(c, time.Second)
	rt := ResolvedTarget{Kind: KindLXD, ID: "steel-echo-v2-1"}
	r, err := inj.Plan(t.Context(), rt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r["id"] != "steel-echo-v2-1" || r["action"] != "kill" {
		t.Fatalf("recipe = %v", r)
	}
	if err := inj.Inject(t.Context(), rt, nil, r); err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.killed) != 1 || !strings.HasPrefix(c.killed[0], "steel-echo-v2-1 kill ") {
		t.Fatalf("killed = %v", c.killed)
	}
}

func TestContainer_RevertWaitsForHealthy(t *testing.T) {
	c := &fakeCuirass{enforce: true, chaos: true, healthyAfter: 4} // healthy on the 4th poll
	inj := containerInjector(c, time.Second)
	recipe := Recipe{"id": "steel-echo-v2-1", "action": "kill", "injected_at": "0"}
	if err := inj.Revert(t.Context(), recipe); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	gone, err := inj.Reverted(t.Context(), recipe)
	if err != nil || !gone {
		t.Errorf("Reverted = %v, %v; want true, nil", gone, err)
	}
}

func TestContainer_RevertTimesOutKeepsEntry(t *testing.T) {
	c := &fakeCuirass{enforce: true, chaos: true, healthy: false} // never recovers
	inj := containerInjector(c, 20*time.Millisecond)
	recipe := Recipe{"id": "steel-echo-v2-1", "action": "kill", "injected_at": "0"}
	if err := inj.Revert(t.Context(), recipe); err == nil {
		t.Fatal("Revert succeeded though the instance never came back")
	}
	if gone, _ := inj.Reverted(t.Context(), recipe); gone {
		t.Error("Reverted true while the instance is still down")
	}
}

func TestContainer_RevertBudgetCoversMTTR(t *testing.T) {
	inj := NewContainerInjector(&fakeCuirass{}, "kill", 90*time.Second, nil)
	if b := inj.RevertBudget(); b < 90*time.Second {
		t.Errorf("RevertBudget = %v, want >= the 90s MTTR", b)
	}
}

// The HTTP client sends the exact request bodies and headers Cuirass expects.
func TestContainerClient_RequestShapes(t *testing.T) {
	var gotBody map[string]string
	var gotKey, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/info":
			json.NewEncoder(w).Encode(map[string]any{"enforce": true, "chaos": true})
		case r.URL.Path == "/api/v1/state/actual":
			json.NewEncoder(w).Encode(map[string]any{
				"steel-echo-v2-1": map[string]any{"status": "running", "health": "healthy"},
				"down-0":          map[string]any{"status": "stopped", "health": "unknown"},
			})
		case strings.HasSuffix(r.URL.Path, "/chaos"):
			gotPath = r.URL.Path
			gotKey = r.Header.Get("X-API-Key")
			json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"result": "done"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	c, err := NewContainerChaosClient(srv.URL, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	enforce, chaos, err := c.Modes(t.Context())
	if err != nil || !enforce || !chaos {
		t.Fatalf("Modes = %v,%v,%v", enforce, chaos, err)
	}
	if err := c.Chaos(t.Context(), "steel-echo-v2-1", "kill", "game day S-REAL-STEEL"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/containers/steel-echo-v2-1/chaos" {
		t.Errorf("path = %q", gotPath)
	}
	if gotKey != "secret-key" {
		t.Errorf("api key header = %q", gotKey)
	}
	if gotBody["action"] != "kill" || gotBody["reason"] != "game day S-REAL-STEEL" {
		t.Errorf("body = %v", gotBody)
	}
	if ok, err := c.Healthy(t.Context(), "steel-echo-v2-1"); err != nil || !ok {
		t.Errorf("Healthy(up) = %v,%v", ok, err)
	}
	if ok, err := c.Healthy(t.Context(), "down-0"); err != nil || ok {
		t.Errorf("Healthy(down) = %v,%v", ok, err)
	}
}
