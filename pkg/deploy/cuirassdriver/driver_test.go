// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cuirassdriver

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/deploy/strategy"
)

var testProxy = &ProxySpec{Listen: "tcp:127.0.0.1:26601", Connect: "tcp:127.0.0.1:8080"}

func testSpec() ServiceSpec {
	return ServiceSpec{
		Service: "steel-echo",
		Health:  &HealthSpec{Port: 8080, Path: "/health"},
		Proxy:   testProxy,
		Labels:  map[string]string{"chaos": "allowed"},
	}
}

func newTestDriver(t *testing.T, url string) *Driver {
	t.Helper()
	d, err := New(Config{URL: url, APIKey: testKey, PollInterval: 2 * time.Millisecond, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func callbacks(t *testing.T, d *Driver, target string) *strategy.Callbacks {
	t.Helper()
	cb, err := d.Callbacks(testSpec(), "dep-1", target)
	if err != nil {
		t.Fatal(err)
	}
	return cb
}

func TestDriver_CreatePutsVersionedInstanceAndWaitsHealthy(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.healthyAfter = 3
	d := newTestDriver(t, srv.URL)
	cb := callbacks(t, d, "v2")

	inst, err := cb.OnInstanceCreate(context.Background(), &strategy.InstanceParams{
		ServiceName: "steel-echo", Version: "v2", Labels: map[string]string{"deployment": "dep-1", "role": "canary"}})
	if err != nil {
		t.Fatal(err)
	}
	if inst.ID != "steel-echo-v2-0" || inst.Version != "v2" || !inst.Healthy || inst.Address == "" {
		t.Fatalf("instance = %+v", inst)
	}
	f.mu.Lock()
	spec := f.desired["steel-echo-v2-0"]
	health := f.actual["steel-echo-v2-0"].Health
	f.mu.Unlock()
	if spec == nil || spec.Image != "steel-echo-v2" {
		t.Fatalf("desired spec = %+v", spec)
	}
	for k, v := range map[string]string{"svc": "steel-echo", "ver": "v2", "deployment": "dep-1", "chaos": "allowed", "role": "canary"} {
		if spec.Labels[k] != v {
			t.Fatalf("label %s = %q, want %q (labels %v)", k, spec.Labels[k], v, spec.Labels)
		}
	}
	if spec.Health == nil || spec.Health.Port != 8080 || spec.Health.Path != "/health" {
		t.Fatalf("health spec not passed through: %+v", spec.Health)
	}
	// First instance of the service: it carries the stable endpoint.
	if spec.Proxy == nil || *spec.Proxy != *testProxy {
		t.Fatalf("first instance has no proxy: %+v", spec.Proxy)
	}
	// Returned only once Cuirass itself said healthy.
	if health != "healthy" {
		t.Fatalf("returned while Cuirass said %q", health)
	}

	// A second instance of the same version takes the next free n and no proxy.
	inst2, err := cb.OnInstanceCreate(context.Background(), &strategy.InstanceParams{ServiceName: "steel-echo", Version: "v2"})
	if err != nil {
		t.Fatal(err)
	}
	if inst2.ID != "steel-echo-v2-1" {
		t.Fatalf("second instance = %s", inst2.ID)
	}
	if got := f.proxyHolders(); len(got) != 1 || got[0] != "steel-echo-v2-0" {
		t.Fatalf("proxy holders = %v", got)
	}
}

func TestDriver_CreateTimesOutWhenNeverHealthy(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.healthyAfter = -1
	d := newTestDriver(t, srv.URL)
	cb := callbacks(t, d, "v2")

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := cb.OnInstanceCreate(ctx, &strategy.InstanceParams{ServiceName: "steel-echo", Version: "v2"})
	if err == nil || !strings.Contains(err.Error(), "steel-echo-v2-0") {
		t.Fatalf("err = %v, want a timeout naming steel-echo-v2-0", err)
	}
	// The half-made instance is not left behind.
	if ids := f.desiredIDs(); len(ids) != 0 {
		t.Fatalf("desired after a failed create = %v", ids)
	}
}

func TestDriver_DeleteRemovesDesired(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.seed(containerSpec{ID: "steel-echo-v1-0", Name: "steel-echo-v1-0", Image: "steel-echo-v1",
		Labels: map[string]string{"svc": "steel-echo", "ver": "v1"}})
	d := newTestDriver(t, srv.URL)
	cb := callbacks(t, d, "v2")

	if err := cb.OnInstanceDelete(context.Background(), "steel-echo-v1-0"); err != nil {
		t.Fatal(err)
	}
	if ids := f.desiredIDs(); len(ids) != 0 {
		t.Fatalf("desired = %v", ids)
	}
	f.mu.Lock()
	_, still := f.actual["steel-echo-v1-0"]
	f.mu.Unlock()
	if still {
		t.Fatal("delete returned before Cuirass removed the instance")
	}
	// Deleting what is already gone is not an error.
	if err := cb.OnInstanceDelete(context.Background(), "steel-echo-v1-0"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestDriver_DeleteRefusesAnotherServicesInstance(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.seed(containerSpec{ID: "other-v1-0", Labels: map[string]string{"svc": "other", "ver": "v1"}})
	f.mu.Lock()
	f.actual["pihole"] = &containerState{ID: "pihole", Status: "running", Managed: false}
	f.mu.Unlock()
	d := newTestDriver(t, srv.URL)
	cb := callbacks(t, d, "v2")
	for _, id := range []string{"other-v1-0", "pihole", "../../state"} {
		if err := cb.OnInstanceDelete(context.Background(), id); err == nil {
			t.Fatalf("deleted %s, which is not a steel-echo instance", id)
		}
	}
	if ids := f.desiredIDs(); len(ids) != 1 {
		t.Fatalf("desired = %v", ids)
	}
	for _, l := range f.logSnapshot() {
		if strings.HasPrefix(l, "DELETE") {
			t.Fatalf("a refused delete reached Cuirass: %s", l)
		}
	}
}

func TestDriver_DeleteMovesTheProxyFirst(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.seed(containerSpec{ID: "steel-echo-v1-0", Labels: map[string]string{"svc": "steel-echo", "ver": "v1"}, Proxy: testProxy})
	f.seed(containerSpec{ID: "steel-echo-v2-0", Labels: map[string]string{"svc": "steel-echo", "ver": "v2"}})
	d := newTestDriver(t, srv.URL)
	cb := callbacks(t, d, "v2")

	if err := cb.OnInstanceDelete(context.Background(), "steel-echo-v1-0"); err != nil {
		t.Fatal(err)
	}
	if got := f.proxyHolders(); len(got) != 1 || got[0] != "steel-echo-v2-0" {
		t.Fatalf("proxy holders after deleting its holder = %v", got)
	}
}

func TestDriver_UpdateIsCreateThenDelete(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.seed(containerSpec{ID: "steel-echo-v1-0", Labels: map[string]string{"svc": "steel-echo", "ver": "v1"}, Proxy: testProxy})
	d := newTestDriver(t, srv.URL)
	cb := callbacks(t, d, "v2")

	if err := cb.OnInstanceUpdate(context.Background(), "steel-echo-v1-0", "v2"); err != nil {
		t.Fatal(err)
	}
	if ids := f.desiredIDs(); len(ids) != 1 || ids[0] != "steel-echo-v2-0" {
		t.Fatalf("desired after update = %v", ids)
	}
	var create, del = -1, -1
	for i, l := range f.logSnapshot() {
		if strings.HasPrefix(l, "PUT steel-echo-v2-0") && create < 0 {
			create = i
		}
		if l == "DELETE steel-echo-v1-0" {
			del = i
		}
	}
	if create < 0 || del < 0 || create > del {
		t.Fatalf("want create before delete, log %v", f.logSnapshot())
	}
	if got := f.proxyHolders(); len(got) != 1 || got[0] != "steel-echo-v2-0" {
		t.Fatalf("proxy holders = %v", got)
	}
}

func TestDriver_TrafficShiftMovesProxyOnce(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.seed(containerSpec{ID: "steel-echo-v2-0", Labels: map[string]string{"svc": "steel-echo", "ver": "v2"}, Proxy: testProxy})
	f.seed(containerSpec{ID: "steel-echo-v3-0", Labels: map[string]string{"svc": "steel-echo", "ver": "v3"}})
	f.seed(containerSpec{ID: "steel-echo-v3-1", Labels: map[string]string{"svc": "steel-echo", "ver": "v3"}})
	d := newTestDriver(t, srv.URL)
	cb := callbacks(t, d, "v3")

	f.mu.Lock()
	f.log = nil
	f.mu.Unlock()
	if err := cb.OnTrafficShift(context.Background(), &strategy.TrafficShiftParams{OldVersion: "v2", NewVersion: "v3", Weight: 100}); err != nil {
		t.Fatal(err)
	}
	var writes []string
	for _, l := range f.logSnapshot() {
		if strings.HasPrefix(l, "PUT") || strings.HasPrefix(l, "POST") {
			writes = append(writes, l)
		}
	}
	want := []string{"PUT steel-echo-v2-0 proxy=false", "PUT steel-echo-v3-0 proxy=true", "POST reconcile"}
	if strings.Join(writes, "|") != strings.Join(want, "|") {
		t.Fatalf("writes = %v, want %v", writes, want)
	}
	// Shifting again to where traffic already is changes nothing.
	f.mu.Lock()
	f.log = nil
	f.mu.Unlock()
	if err := cb.OnTrafficShift(context.Background(), &strategy.TrafficShiftParams{OldVersion: "v2", NewVersion: "v3", Weight: 100}); err != nil {
		t.Fatal(err)
	}
	for _, l := range f.logSnapshot() {
		if strings.HasPrefix(l, "PUT") {
			t.Fatalf("a no-op shift wrote %s", l)
		}
	}
	// A partial weight is the client-side replica-share split (D-04): no write.
	if err := cb.OnTrafficShift(context.Background(), &strategy.TrafficShiftParams{OldVersion: "v3", NewVersion: "v4", Weight: 33}); err != nil {
		t.Fatal(err)
	}
	// All traffic to a version with no instance is an error, not a silent pass.
	err := cb.OnTrafficShift(context.Background(), &strategy.TrafficShiftParams{OldVersion: "v3", NewVersion: "v9", Weight: 100})
	if err == nil {
		t.Fatal("shift to a version with no instance succeeded")
	}
}

func TestDriver_RefusesWithoutAPIKey(t *testing.T) {
	t.Setenv("CUIRASS_API_KEY", "")
	t.Setenv("CUIRASS_URL", "http://127.0.0.1:19006")
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "CUIRASS_API_KEY") {
		t.Fatalf("FromEnv without a key: %v", err)
	}
	if _, err := New(Config{URL: "http://127.0.0.1:19006"}); err == nil {
		t.Fatal("New without a key succeeded")
	}
	t.Setenv("CUIRASS_API_KEY", "k")
	if _, err := FromEnv(); err != nil {
		t.Fatalf("FromEnv with a key: %v", err)
	}
	for _, bad := range []string{"", "ftp://x", "http://user:pw@x", "not a url"} {
		if _, err := New(Config{URL: bad, APIKey: "k"}); err == nil {
			t.Fatalf("New accepted URL %q", bad)
		}
	}
}

func TestDriver_HealthIsCuirassHealth(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.seed(containerSpec{ID: "steel-echo-v1-0", Labels: map[string]string{"svc": "steel-echo", "ver": "v1"}})
	// The instance answers nothing a driver could probe: only Cuirass's word counts.
	d := newTestDriver(t, srv.URL)
	cb := callbacks(t, d, "v2")

	ok, err := cb.OnHealthCheck(context.Background(), "steel-echo-v1-0")
	if err != nil || !ok {
		t.Fatalf("healthy per Cuirass: ok=%v err=%v", ok, err)
	}
	f.mu.Lock()
	f.actual["steel-echo-v1-0"].Health = "unhealthy"
	f.onTick = func(f *fakeCuirass) { f.actual["steel-echo-v1-0"].Health = "unhealthy" }
	f.mu.Unlock()
	if ok, _ := cb.OnHealthCheck(context.Background(), "steel-echo-v1-0"); ok {
		t.Fatal("reported healthy while Cuirass says unhealthy")
	}
	f.mu.Lock()
	f.onTick = func(f *fakeCuirass) {
		f.actual["steel-echo-v1-0"].Health = "healthy"
		f.actual["steel-echo-v1-0"].Status = "stopped"
	}
	f.mu.Unlock()
	if ok, _ := cb.OnHealthCheck(context.Background(), "steel-echo-v1-0"); ok {
		t.Fatal("a stopped instance reported healthy")
	}
	if ok, err := cb.OnHealthCheck(context.Background(), "steel-echo-v9-9"); ok || err == nil {
		t.Fatalf("unknown instance: ok=%v err=%v", ok, err)
	}
}

func TestDriver_CallbacksNeedAHealthSpec(t *testing.T) {
	_, srv := newFakeCuirass(t)
	d := newTestDriver(t, srv.URL)
	spec := testSpec()
	spec.Health = nil
	if _, err := d.Callbacks(spec, "dep-1", "v2"); err == nil {
		t.Fatal("callbacks without a health spec: an instance could never be confirmed healthy")
	}
	spec = testSpec()
	spec.Service = "Bad_Name"
	if _, err := d.Callbacks(spec, "dep-1", "v2"); err == nil {
		t.Fatal("accepted a service name LXD cannot use")
	}
	spec = testSpec()
	spec.Labels = map[string]string{"svc": "other"}
	if _, err := d.Callbacks(spec, "dep-1", "v2"); err == nil {
		t.Fatal("a caller label may not override svc")
	}
}

func TestInstanceName(t *testing.T) {
	good := map[[2]string]string{
		{"steel-echo", "v1"}:     "steel-echo-v1-0",
		{"steel-echo", "V1.2.3"}: "steel-echo-v1-2-3-0",
		{"svc", "2026_10_23"}:    "svc-2026-10-23-0",
	}
	for in, want := range good {
		got, err := InstanceName(in[0], in[1], 0)
		if err != nil || got != want {
			t.Fatalf("InstanceName(%q, %q) = %q, %v; want %q", in[0], in[1], got, err, want)
		}
	}
	for _, in := range [][2]string{{"steel-echo", ""}, {"steel-echo", "..."}, {"9svc", "v1"}, {"svc", strings.Repeat("v", 70)}} {
		if _, err := InstanceName(in[0], in[1], 0); err == nil {
			t.Fatalf("InstanceName(%q, %q) accepted", in[0], in[1])
		}
	}
}

func TestDriver_WriteFailureIsAnError(t *testing.T) {
	f, srv := newFakeCuirass(t)
	f.failPut["steel-echo-v2-0"] = true
	d := newTestDriver(t, srv.URL)
	cb := callbacks(t, d, "v2")
	_, err := cb.OnInstanceCreate(context.Background(), &strategy.InstanceParams{ServiceName: "steel-echo", Version: "v2"})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != http.StatusInternalServerError {
		t.Fatalf("err = %v, want a StatusError 500", err)
	}
}
