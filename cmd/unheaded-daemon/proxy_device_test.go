// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxySpec_Validation(t *testing.T) {
	lab := []string{"192.168.13.2"}
	for _, tc := range []struct {
		name            string
		listen, connect string
		ok              bool
	}{
		{"loopback in band", "tcp:127.0.0.1:26601", "tcp:127.0.0.1:8080", true},
		{"band low edge", "tcp:127.0.0.1:26000", "tcp:127.0.0.1:1", true},
		{"band high edge", "tcp:127.0.0.1:26666", "tcp:127.0.0.1:65535", true},
		{"configured lab address", "tcp:192.168.13.2:26601", "tcp:127.0.0.1:8080", true},
		{"all interfaces", "tcp:0.0.0.0:26601", "tcp:127.0.0.1:8080", false},
		{"ipv6 any", "tcp:[::]:26601", "tcp:127.0.0.1:8080", false},
		{"unlisted address", "tcp:10.0.0.5:26601", "tcp:127.0.0.1:8080", false},
		{"low port", "tcp:127.0.0.1:80", "tcp:127.0.0.1:8080", false},
		{"below band", "tcp:127.0.0.1:25999", "tcp:127.0.0.1:8080", false},
		{"above band", "tcp:127.0.0.1:26667", "tcp:127.0.0.1:8080", false},
		{"udp listen", "udp:127.0.0.1:26601", "tcp:127.0.0.1:8080", false},
		{"unix listen", "unix:/run/x.sock", "tcp:127.0.0.1:8080", false},
		{"connect outside the container", "tcp:127.0.0.1:26601", "tcp:10.10.10.10:18000", false},
		{"connect by name", "tcp:127.0.0.1:26601", "tcp:localhost:8080", false},
		{"connect port 0", "tcp:127.0.0.1:26601", "tcp:127.0.0.1:0", false},
		{"connect port too big", "tcp:127.0.0.1:26601", "tcp:127.0.0.1:65536", false},
		{"connect unix", "tcp:127.0.0.1:26601", "unix:/var/run/docker.sock", false},
		{"port range", "tcp:127.0.0.1:26601-26610", "tcp:127.0.0.1:8080", false},
		{"empty", "", "", false},
	} {
		err := (&ProxySpec{Listen: tc.listen, Connect: tc.connect}).validate(lab)
		if (err == nil) != tc.ok {
			t.Errorf("%s: validate() = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
	if err := (&ProxySpec{Listen: "tcp:192.168.13.2:26601", Connect: "tcp:127.0.0.1:1"}).validate(nil); err == nil {
		t.Error("a lab address is accepted without being configured")
	}
}

func putSpec(t *testing.T, mux *http.ServeMux, id, body string) int {
	t.Helper()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/containers/"+id, strings.NewReader(body)))
	return w.Code
}

// Two services cannot claim the same listen address; re-PUTting the owner is fine.
func TestContainerPut_DuplicateProxyListenConflicts(t *testing.T) {
	d := newTestDaemon(t)
	d.authEnabled = true
	mux := http.NewServeMux()
	d.registerHandlers(mux)
	a := `{"image":"steel-echo-v1","proxy":{"listen":"tcp:127.0.0.1:26601","connect":"tcp:127.0.0.1:8080"}}`
	if c := putSpec(t, mux, "steel-echo-v1-0", a); c != http.StatusOK {
		t.Fatalf("first PUT: %d", c)
	}
	if c := putSpec(t, mux, "steel-echo-v1-0", a); c != http.StatusOK {
		t.Errorf("re-PUT of the owner: %d, want 200", c)
	}
	if c := putSpec(t, mux, "other-0", a); c != http.StatusConflict {
		t.Errorf("second owner of the listen address: %d, want 409", c)
	}
	if c := putSpec(t, mux, "bad-0", `{"image":"x","proxy":{"listen":"tcp:0.0.0.0:26602","connect":"tcp:127.0.0.1:8080"}}`); c != http.StatusBadRequest {
		t.Errorf("proxy on 0.0.0.0: %d, want 400", c)
	}
	if _, ok := d.stateManager.desired["other-0"]; ok {
		t.Error("conflicting spec stored")
	}
}

// A proxy that differs from the desired one (or exists when none is
// desired) plans update-proxy; observe mode only plans it.
func TestReconcile_ProxyDriftPlansUpdate(t *testing.T) {
	want := &ProxySpec{Listen: "tcp:127.0.0.1:26601", Connect: "tcp:127.0.0.1:8080"}
	for _, tc := range []struct {
		name            string
		desired, actual *ProxySpec
		update          bool
	}{
		{"missing", want, nil, true},
		{"different", want, &ProxySpec{Listen: "tcp:127.0.0.1:26602", Connect: "tcp:127.0.0.1:8080"}, true},
		{"same", want, &ProxySpec{Listen: want.Listen, Connect: want.Connect}, false},
		{"not desired any more", nil, want, true},
		{"neither", nil, nil, false},
	} {
		d, m := safeDaemon(t, false, false)
		m.containers["blue"] = &ContainerState{ID: "blue", Status: "running", Managed: true, Proxy: tc.actual}
		d.stateManager.desired["blue"] = &ContainerSpec{ID: "blue", Image: "x", Proxy: tc.desired}
		d.syncActualStateFromLXD(t.Context())
		actions := d.planReconcile()
		got := len(actions) == 1 && actions[0].Action == "update-proxy"
		if got != tc.update || (!tc.update && len(actions) != 0) {
			t.Errorf("%s: plan %+v, want update-proxy=%v", tc.name, actions, tc.update)
		}
		for i := range actions {
			d.applyAction(t.Context(), &actions[i])
			if !actions[i].Planned {
				t.Errorf("%s: observe mode took %s", tc.name, actions[i].Action)
			}
		}
	}
}

// Enforce mode moves the device; a proxy on an unmanaged instance is refused
// at plan time like every other action.
func TestReconcile_EnforceAppliesProxy(t *testing.T) {
	d, m := safeDaemon(t, true, false)
	want := &ProxySpec{Listen: "tcp:127.0.0.1:26601", Connect: "tcp:127.0.0.1:8080"}
	m.containers["blue"] = &ContainerState{ID: "blue", Status: "running", Managed: true}
	d.stateManager.desired["blue"] = &ContainerSpec{ID: "blue", Image: "x", Proxy: want}
	d.reconcile()
	if p := m.containers["blue"].Proxy; p == nil || *p != *want {
		t.Fatalf("proxy after enforce: %+v, want %+v", p, want)
	}
	d.stateManager.desired["pihole"] = &ContainerSpec{ID: "pihole", Image: "x", Proxy: &ProxySpec{Listen: "tcp:127.0.0.1:26602", Connect: "tcp:127.0.0.1:80"}}
	d.reconcile()
	if m.containers["pihole"].Proxy != nil {
		t.Error("a proxy was put on unmanaged pihole")
	}
}

// UpdateProxy changes only the unheaded-proxy device: read-modify-write
// with the ETag, every other device and field kept; 412 retried once.
func TestLXDREST_UpdateProxyTouchesOnlyOurDevice(t *testing.T) {
	f := newFakeLXD()
	f.etag = `"abc123"`
	web := instance("web", "Running", map[string]string{ManagedLabel: "true"})
	web["devices"] = map[string]any{
		"eth0":   map[string]any{"type": "nic", "network": "lxdbr0", "name": "eth0"},
		"mydisk": map[string]any{"type": "disk", "source": "/srv/data", "path": "/data"},
	}
	web["profiles"] = []any{"default"}
	web["description"] = "kept as is"
	f.instances["web"] = web
	c := unixLXD(t, f, "")

	f.precondFail = 1 // first PUT loses a race
	want := &ProxySpec{Listen: "tcp:127.0.0.1:26601", Connect: "tcp:127.0.0.1:8080"}
	if err := c.UpdateProxy(t.Context(), "web", want); err != nil {
		t.Fatalf("UpdateProxy: %v", err)
	}
	if len(f.ifMatch) != 2 || f.ifMatch[0] != `"abc123"` || f.ifMatch[1] != `"abc123"` {
		t.Errorf("If-Match %v, want the GET's ETag on both attempts", f.ifMatch)
	}
	if len(f.puts) != 1 {
		t.Fatalf("%d accepted PUTs, want 1", len(f.puts))
	}
	body := f.puts[0]
	devs := body["devices"].(map[string]any)
	if eth0 := devs["eth0"].(map[string]any); eth0["network"] != "lxdbr0" {
		t.Errorf("eth0 changed: %v", eth0)
	}
	if disk := devs["mydisk"].(map[string]any); disk["source"] != "/srv/data" {
		t.Errorf("user device changed: %v", disk)
	}
	p := devs["unheaded-proxy"].(map[string]any)
	if p["type"] != "proxy" || p["listen"] != want.Listen || p["connect"] != want.Connect {
		t.Errorf("unheaded-proxy = %v", p)
	}
	if body["description"] != "kept as is" || body["profiles"] == nil {
		t.Errorf("other fields not carried over: %v", body)
	}
	for _, k := range []string{"status", "state", "name"} {
		if _, ok := body[k]; ok {
			t.Errorf("PUT carried read-only field %q", k)
		}
	}

	// The listed state shows our device only.
	f.instances["web"]["devices"].(map[string]any)["other-proxy"] = map[string]any{"type": "proxy", "listen": "tcp:0.0.0.0:80", "connect": "tcp:127.0.0.1:80"}
	got, err := c.ListContainers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if gp := got["web"].Proxy; gp == nil || *gp != *want {
		t.Errorf("listed proxy %+v, want ours %+v", gp, want)
	}

	// Removing it leaves the others.
	if err := c.UpdateProxy(t.Context(), "web", nil); err != nil {
		t.Fatal(err)
	}
	devs = f.puts[len(f.puts)-1]["devices"].(map[string]any)
	if _, ok := devs["unheaded-proxy"]; ok {
		t.Error("unheaded-proxy still present after removal")
	}
	if _, ok := devs["eth0"]; !ok {
		t.Error("removal dropped eth0")
	}
}

func TestLXDREST_UpdateProxyGivesUpAfterTwoConflicts(t *testing.T) {
	f := newFakeLXD()
	f.etag = `"e"`
	f.instances["web"] = instance("web", "Running", map[string]string{ManagedLabel: "true"})
	f.precondFail = 2
	c := unixLXD(t, f, "")
	if err := c.UpdateProxy(t.Context(), "web", &ProxySpec{Listen: "tcp:127.0.0.1:26601", Connect: "tcp:127.0.0.1:1"}); err == nil {
		t.Fatal("UpdateProxy succeeded through two 412s")
	}
	if len(f.ifMatch) != 2 {
		t.Errorf("%d PUT attempts, want 2", len(f.ifMatch))
	}
}

func TestLXDREST_UpdateProxyRefusesUnmanaged(t *testing.T) {
	f := newFakeLXD()
	c := unixLXD(t, f, "")
	err := c.UpdateProxy(t.Context(), "pihole", &ProxySpec{Listen: "tcp:127.0.0.1:26601", Connect: "tcp:127.0.0.1:80"})
	if !errors.Is(err, errLXDNotManaged) {
		t.Fatalf("err = %v, want errLXDNotManaged", err)
	}
	for _, line := range strings.Split(f.callLog(), "\n") {
		if !strings.HasPrefix(line, "GET ") {
			t.Errorf("sent a write for unmanaged pihole: %s", line)
		}
	}
}

// Create carries the proxy device, so the endpoint exists from the first start.
func TestLXDREST_CreateCarriesProxy(t *testing.T) {
	f := newFakeLXD()
	c := unixLXD(t, f, "")
	spec := &ContainerSpec{ID: "web", Image: "steel-echo-v1", Proxy: &ProxySpec{Listen: "tcp:127.0.0.1:26601", Connect: "tcp:127.0.0.1:8080"}}
	if err := c.CreateContainer(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	devs, _ := f.created["devices"].(map[string]any)
	p, _ := devs["unheaded-proxy"].(map[string]any)
	if p["type"] != "proxy" || p["listen"] != "tcp:127.0.0.1:26601" {
		t.Errorf("created devices %v", f.created["devices"])
	}
}
