// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package sword

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type fakeHold struct {
	mu   sync.Mutex
	held bool
	why  string
}

func (f *fakeHold) Hold() (bool, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.held, f.why
}

func (f *fakeHold) set(held bool, why string) {
	f.mu.Lock()
	f.held, f.why = held, why
	f.mu.Unlock()
}

// TestDeployRefusedWhileCritical (ADR-147 Phase 8, D2 = B): no deploy
// starts while a critical alert fires (409, counted); a running deploy is
// not stopped; held false (ok, or unknown) starts.
func TestDeployRefusedWhileCritical(t *testing.T) {
	h := &fakeHold{}
	fx := newFixture(t, func(c *Config) { c.Hold = h })
	fx.f.Mu.Lock()
	fx.f.HealthyAfter = 20 // slow enough to be running when the alert fires
	fx.f.Mu.Unlock()
	first := fx.do(t, http.MethodPost, "/api/v1/deployments", reqBody("v1", "recreate"))
	if first.Code != http.StatusAccepted {
		t.Fatalf("first = %d", first.Code)
	}
	var running Deployment
	_ = json.Unmarshal(first.Body.Bytes(), &running)

	h.set(true, "critical alert firing: WotanDown")
	rec := fx.do(t, http.MethodPost, "/api/v1/deployments", reqBody("v2", "recreate"))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "critical alert firing: WotanDown") {
		t.Fatalf("while critical = %d %s, want 409 naming the alert", rec.Code, rec.Body.String())
	}
	if v := fx.s.heldC.Value(); v != 1 {
		t.Errorf("sword_deploys_held_total = %v, want 1", v)
	}
	if d := fx.wait(t, running.ID); d.Status != StatusSucceeded {
		t.Errorf("the running deploy ended %s; a hold does not stop it", d.Status)
	}

	h.set(false, "")
	fx.wait(t, fx.deploy(t, "v3", "recreate").ID)
}
