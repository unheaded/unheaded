// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeLXD answers the slice of LXD's REST API that lxdREST uses, shaped
// like west's LXD 5.21 (probed 2026-10-23).
type fakeLXD struct {
	mu        sync.Mutex
	instances map[string]map[string]any
	calls     []string // "METHOD path"
	created   map[string]any
	opErr     string // when set, every operation fails with it
	project   string // the only project that exists

	// PUT /1.0/instances/{name}: the ETag GET hands out, every PUT body and
	// If-Match seen, and how many PUTs to answer 412 before accepting.
	etag        string
	puts        []map[string]any
	ifMatch     []string
	precondFail int
}

func newFakeLXD() *fakeLXD {
	return &fakeLXD{project: "default", instances: map[string]map[string]any{
		"pihole": instance("pihole", "Running", nil),
	}}
}

func instance(name, status string, config map[string]string) map[string]any {
	if config == nil {
		config = map[string]string{}
	}
	return map[string]any{
		"name": name, "status": status, "type": "container", "config": config,
		"state": map[string]any{"network": map[string]any{
			"lo":   map[string]any{"addresses": []map[string]string{{"family": "inet", "address": "127.0.0.1", "scope": "local"}}},
			"eth0": map[string]any{"host_name": "veth" + name, "addresses": []map[string]string{{"family": "inet6", "address": "fd00::5", "scope": "global"}, {"family": "inet", "address": "10.10.20.5", "scope": "global"}}},
		}},
	}
}

func (f *fakeLXD) handler() http.Handler {
	mux := http.NewServeMux()
	sync := func(w http.ResponseWriter, md any) {
		json.NewEncoder(w).Encode(map[string]any{"type": "sync", "status_code": 200, "metadata": md})
	}
	async := func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]any{"type": "async", "status_code": 100, "operation": "/1.0/operations/op-1"})
	}
	fail := func(w http.ResponseWriter, code int, msg string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]any{"type": "error", "error_code": code, "error": msg})
	}
	mux.HandleFunc("GET /1.0", func(w http.ResponseWriter, r *http.Request) {
		sync(w, map[string]any{"auth": "trusted", "environment": map[string]any{"server_version": "5.21.8", "project": f.project}})
	})
	mux.HandleFunc("GET /1.0/instances", func(w http.ResponseWriter, r *http.Request) {
		list := []any{}
		for _, in := range f.instances {
			list = append(list, in)
		}
		sync(w, list)
	})
	mux.HandleFunc("GET /1.0/instances/{name}", func(w http.ResponseWriter, r *http.Request) {
		in, ok := f.instances[r.PathValue("name")]
		if !ok {
			fail(w, 404, "Instance not found")
			return
		}
		if f.etag != "" {
			w.Header().Set("ETag", f.etag)
		}
		sync(w, in)
	})
	mux.HandleFunc("PUT /1.0/instances/{name}", func(w http.ResponseWriter, r *http.Request) {
		in, ok := f.instances[r.PathValue("name")]
		if !ok {
			fail(w, 404, "Instance not found")
			return
		}
		f.ifMatch = append(f.ifMatch, r.Header.Get("If-Match"))
		if f.precondFail > 0 {
			f.precondFail--
			fail(w, http.StatusPreconditionFailed, "ETag doesn't match")
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.puts = append(f.puts, body)
		if devs, ok := body["devices"]; ok {
			in["devices"] = devs
		}
		async(w)
	})
	mux.HandleFunc("POST /1.0/instances", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.created = body
		cfg := map[string]string{}
		for k, v := range body["config"].(map[string]any) {
			cfg[k] = v.(string)
		}
		in := instance(body["name"].(string), "Stopped", cfg)
		if devs, ok := body["devices"]; ok {
			in["devices"] = devs
		}
		f.instances[body["name"].(string)] = in
		async(w)
	})
	mux.HandleFunc("PUT /1.0/instances/{name}/state", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Action string }
		json.NewDecoder(r.Body).Decode(&body)
		in := f.instances[r.PathValue("name")]
		if body.Action == "stop" {
			in["status"] = "Stopped"
		} else {
			in["status"] = "Running"
		}
		async(w)
	})
	mux.HandleFunc("DELETE /1.0/instances/{name}", func(w http.ResponseWriter, r *http.Request) {
		if f.instances[r.PathValue("name")]["status"] != "Stopped" {
			fail(w, 400, "Instance is running")
			return
		}
		delete(f.instances, r.PathValue("name"))
		async(w)
	})
	mux.HandleFunc("GET /1.0/operations/{id}/wait", func(w http.ResponseWriter, r *http.Request) {
		if f.opErr != "" {
			sync(w, map[string]any{"status_code": 400, "err": f.opErr})
			return
		}
		sync(w, map[string]any{"status_code": 200})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		if p := r.URL.Query().Get("project"); p != "" && p != f.project {
			fail(w, 404, "Project not found")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (f *fakeLXD) callLog() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, "\n")
}

// unixLXD serves f on a unix socket, like LXD's.
func unixLXD(t *testing.T, f *fakeLXD, project string) *lxdREST {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "unix.socket")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &httptest.Server{Listener: ln, Config: &http.Server{Handler: f.handler()}}
	srv.Start()
	t.Cleanup(srv.Close)
	c, err := newLXDREST(LXDRESTConfig{Socket: sock, Project: project})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLXDREST_ListMapsInstances(t *testing.T) {
	f := newFakeLXD()
	f.instances["web"] = instance("web", "Stopped", map[string]string{ManagedLabel: "true"})
	c := unixLXD(t, f, "")
	got, err := c.ListContainers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	p, w := got["pihole"], got["web"]
	if p == nil || p.Status != "running" || p.Managed || p.IP != "10.10.20.5" {
		t.Errorf("pihole: %+v (want running, unmanaged, global IPv4 over IPv6 and lo)", p)
	}
	if w == nil || w.Status != "stopped" || !w.Managed || w.Health != "unknown" {
		t.Errorf("web: %+v", w)
	}
}

// Cuirass reports each instance's host-side veth (LXD state
// network.<nic>.host_name), which Phase 11 netem shapes. lo has none, so
// the bridged NIC's host_name is reported; a NIC with no host_name (a
// non-bridged device) leaves HostVeth empty and the instance is not
// chaos-eligible (D-21 override).
func TestLXDREST_ReportsHostVeth(t *testing.T) {
	f := newFakeLXD()
	f.instances["web"] = instance("web", "Running", map[string]string{ManagedLabel: "true"})
	// An instance whose only NIC reports no host_name: HostVeth stays empty.
	nohost := instance("nohost", "Running", map[string]string{ManagedLabel: "true"})
	nohost["state"] = map[string]any{"network": map[string]any{
		"eth0": map[string]any{"addresses": []map[string]string{{"family": "inet", "address": "10.10.20.9", "scope": "global"}}},
	}}
	f.instances["nohost"] = nohost
	c := unixLXD(t, f, "")
	got, err := c.ListContainers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if w := got["web"]; w == nil || w.HostVeth != "vethweb" {
		t.Errorf("web host veth = %q, want vethweb", hostVethOf(w))
	}
	if n := got["nohost"]; n == nil || n.HostVeth != "" {
		t.Errorf("nohost host veth = %q, want empty", hostVethOf(n))
	}
	// lo (no host_name) must not be chosen even when sorted first.
	if p := got["pihole"]; p == nil || p.HostVeth != "vethpihole" {
		t.Errorf("pihole host veth = %q, want vethpihole", hostVethOf(p))
	}
}

func hostVethOf(c *ContainerState) string {
	if c == nil {
		return "<nil>"
	}
	return c.HostVeth
}

// The client refuses unmanaged instances itself, whatever the reconciler
// planned, and sends LXD nothing that changes them.
func TestLXDREST_RefusesUnmanaged(t *testing.T) {
	f := newFakeLXD()
	c := unixLXD(t, f, "")
	ctx := t.Context()
	for name, op := range map[string]func() error{
		"start":   func() error { return c.StartContainer(ctx, "pihole") },
		"stop":    func() error { return c.StopContainer(ctx, "pihole", true) },
		"restart": func() error { return c.RestartContainer(ctx, "pihole") },
		"delete":  func() error { return c.DeleteContainer(ctx, "pihole", true) },
	} {
		if err := op(); !errors.Is(err, errLXDNotManaged) {
			t.Errorf("%s pihole: err = %v, want errLXDNotManaged", name, err)
		}
	}
	for _, line := range strings.Split(f.callLog(), "\n") {
		if !strings.HasPrefix(line, "GET ") {
			t.Errorf("sent a write for an unmanaged instance: %s", line)
		}
	}
	if f.instances["pihole"]["status"] != "Running" {
		t.Error("pihole changed")
	}
}

func TestLXDREST_CreateSetsManagedLabel(t *testing.T) {
	f := newFakeLXD()
	c := unixLXD(t, f, "")
	spec := &ContainerSpec{ID: "web", Image: "images:debian/13", CPU: 2, Memory: 1 << 30, Labels: map[string]string{"tier": "app"}}
	if err := c.CreateContainer(t.Context(), spec); err != nil {
		t.Fatal(err)
	}
	cfg := f.created["config"].(map[string]any)
	src := f.created["source"].(map[string]any)
	if cfg[ManagedLabel] != "true" || cfg["limits.cpu"] != "2" || cfg["limits.memory"] != "1073741824B" || cfg["user.unheaded.label.tier"] != "app" {
		t.Errorf("config = %v", cfg)
	}
	if src["alias"] != "debian/13" || src["server"] != "https://images.lxd.canonical.com" || src["protocol"] != "simplestreams" {
		t.Errorf("source = %v", src)
	}
	// A label cannot unset the managed label.
	spec = &ContainerSpec{ID: "web2", Image: "local-alias", Labels: map[string]string{"..": "x"}}
	if err := c.CreateContainer(t.Context(), spec); err == nil {
		t.Error("accepted label key \"..\"")
	}
}

func TestLXDREST_RejectsBadInputBeforeSending(t *testing.T) {
	f := newFakeLXD()
	c := unixLXD(t, f, "")
	ctx := t.Context()
	for _, name := range []string{"", "-x", "1web", "a/b", "../1.0", strings.Repeat("a", 64), "web?x=1"} {
		if err := c.CreateContainer(ctx, &ContainerSpec{ID: name, Image: "x"}); err == nil {
			t.Errorf("create %q accepted", name)
		}
		if err := c.StartContainer(ctx, name); err == nil {
			t.Errorf("start %q accepted", name)
		}
	}
	for _, img := range []string{"", "evil:alias", "images:", "a b"} {
		if err := c.CreateContainer(ctx, &ContainerSpec{ID: "web", Image: img}); err == nil {
			t.Errorf("image %q accepted", img)
		}
	}
	if log := f.callLog(); log != "" {
		t.Errorf("sent requests for invalid input:\n%s", log)
	}
}

func TestLXDREST_ForceDeleteStopsFirst(t *testing.T) {
	f := newFakeLXD()
	f.instances["old"] = instance("old", "Running", map[string]string{ManagedLabel: "true"})
	c := unixLXD(t, f, "")
	if err := c.DeleteContainer(t.Context(), "old", true); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.instances["old"]; ok {
		t.Fatal("old still exists")
	}
	if !strings.Contains(f.callLog(), "PUT /1.0/instances/old/state\nGET /1.0/operations/op-1/wait\nDELETE /1.0/instances/old") {
		t.Errorf("want stop, wait, delete:\n%s", f.callLog())
	}
}

func TestLXDREST_OperationFailureIsAnError(t *testing.T) {
	f := newFakeLXD()
	f.opErr = "Failed creating instance: image not found"
	c := unixLXD(t, f, "")
	err := c.CreateContainer(t.Context(), &ContainerSpec{ID: "web", Image: "nixos"})
	if err == nil || !strings.Contains(err.Error(), "image not found") {
		t.Fatalf("err = %v", err)
	}
}

// A project that does not exist must fail, not fall back to default.
func TestLXDREST_MissingProjectFails(t *testing.T) {
	f := newFakeLXD()
	c := unixLXD(t, f, "unheaded")
	if _, err := c.ListContainers(t.Context()); !errors.Is(err, errLXDNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
	if !strings.Contains(f.callLog(), "GET /1.0/instances") {
		t.Fatal("no request sent")
	}
}

func TestLXDREST_WaitHonoursContext(t *testing.T) {
	f := newFakeLXD()
	c := unixLXD(t, f, "")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.wait(ctx, &lxdResponse{Type: "async", Operation: "/1.0/operations/op-1"}); err == nil {
		t.Fatal("wait on a cancelled context succeeded")
	}
	for _, op := range []string{"", "/1.0/operations/", "/1.0/instances/x", "/1.0/operations/a/../../x"} {
		if err := c.wait(t.Context(), &lxdResponse{Type: "async", Operation: op}); err == nil {
			t.Errorf("operation %q accepted", op)
		}
	}
}

func selfSigned(t *testing.T, dir, name string) (certPath, keyPath string, pair tls.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kder, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder})
	certPath, keyPath = filepath.Join(dir, name+".crt"), filepath.Join(dir, name+".key")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	pair, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath, pair
}

func TestLXDREST_HTTPSPinsServerCertificate(t *testing.T) {
	dir := t.TempDir()
	serverCrt, _, serverPair := selfSigned(t, dir, "server")
	otherCrt, _, _ := selfSigned(t, dir, "other")
	clientCrt, clientKey, _ := selfSigned(t, dir, "client")

	f := newFakeLXD()
	srv := httptest.NewUnstartedServer(f.handler())
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{serverPair}, ClientAuth: tls.RequireAnyClientCert}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	c, err := newLXDREST(LXDRESTConfig{URL: srv.URL, ClientCert: clientCrt, ClientKey: clientKey, ServerCert: serverCrt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.serverInfo(t.Context()); err != nil {
		t.Fatalf("pinned server refused: %v", err)
	}

	wrong, err := newLXDREST(LXDRESTConfig{URL: srv.URL, ClientCert: clientCrt, ClientKey: clientKey, ServerCert: otherCrt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.ListContainers(t.Context()); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("unpinned server accepted: %v", err)
	}
}

func TestLXDREST_ConfigErrors(t *testing.T) {
	for name, cfg := range map[string]LXDRESTConfig{
		"none":        {},
		"both":        {Socket: "/s", URL: "https://h:8443"},
		"http":        {URL: "http://h:8443", ClientCert: "a", ClientKey: "b", ServerCert: "c"},
		"no pin":      {URL: "https://h:8443", ClientCert: "a", ClientKey: "b"},
		"path":        {URL: "https://h:8443/1.0", ClientCert: "a", ClientKey: "b", ServerCert: "c"},
		"bad project": {Socket: "/s", Project: "a&b"},
	} {
		if _, err := newLXDREST(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// failingLXD is an LXD that cannot be reached.
type failingLXD struct{ *MockLXDClient }

func (*failingLXD) ListContainers(context.Context) (map[string]*ContainerState, error) {
	return nil, errors.New("connection refused")
}

// An unreachable LXD is not an empty one: reconcile must not plan creates
// for everything, and actual state and drift stay as last seen.
func TestReconcile_ListErrorSkipsCycle(t *testing.T) {
	d, _ := safeDaemon(t, true, true)
	d.stateManager.desired["web"] = &ContainerSpec{ID: "web", Image: "nixos"}
	d.reconcile() // mock: creates web
	before := len(d.stateManager.actual)

	f := &failingLXD{}
	d.lxdClient = f
	d.reconcile()
	d.detectDrift()
	if len(d.stateManager.actual) != before {
		t.Errorf("actual state changed on a failed list: %d -> %d", before, len(d.stateManager.actual))
	}
	if d.lxdUp.Load() {
		t.Error("cuirass_lxd_up stayed 1 after a failed list")
	}
}

func TestHandleContainer_PutRejectsInvalidName(t *testing.T) {
	d := newTestDaemon(t)
	d.authEnabled = true
	for _, id := range []string{"1web", "a.b", "-x"} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/containers/"+id, strings.NewReader(`{"image":"nixos"}`))
		rec := httptest.NewRecorder()
		d.handleContainer(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %q: %d, want 400", id, rec.Code)
		}
		if _, ok := d.stateManager.desired[id]; ok {
			t.Errorf("PUT %q stored", id)
		}
	}
}

// Opt-in, read-only: CUIRASS_LIVE_LXD_SOCKET=/var/snap/lxd/common/lxd/unix.socket
// lists a real LXD's instances and checks it answers and trusts us.
func TestLXDREST_LiveReadOnly(t *testing.T) {
	sock := os.Getenv("CUIRASS_LIVE_LXD_SOCKET")
	if sock == "" {
		t.Skip("CUIRASS_LIVE_LXD_SOCKET not set")
	}
	c, err := newLXDREST(LXDRESTConfig{Socket: sock})
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.serverInfo(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.ListContainers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for id, s := range list {
		t.Logf("%s: status=%s ip=%s managed=%v", id, s.Status, s.IP, s.Managed)
	}
	t.Logf("LXD %s, project %s, %d instances", info.Environment.ServerVersion, info.Environment.Project, len(list))
}

// Opt-in, read-only: CUIRASS_LIVE_LXD_URL=https://127.0.0.1:8443 plus
// CUIRASS_LIVE_LXD_{CLIENT_CERT,CLIENT_KEY,SERVER_CERT}. Proves the TLS
// and pin path against a real LXD. An untrusted client certificate still
// completes the handshake and is told it is untrusted, so this needs no
// change on the LXD side.
func TestLXDREST_LiveHTTPS(t *testing.T) {
	u := os.Getenv("CUIRASS_LIVE_LXD_URL")
	if u == "" {
		t.Skip("CUIRASS_LIVE_LXD_URL not set")
	}
	cfg := LXDRESTConfig{URL: u, ClientCert: os.Getenv("CUIRASS_LIVE_LXD_CLIENT_CERT"),
		ClientKey: os.Getenv("CUIRASS_LIVE_LXD_CLIENT_KEY"), ServerCert: os.Getenv("CUIRASS_LIVE_LXD_SERVER_CERT")}
	c, err := newLXDREST(cfg)
	if err != nil {
		t.Fatal(err)
	}
	info, err := c.serverInfo(t.Context())
	switch {
	case err == nil:
		t.Logf("trusted; LXD %s", info.Environment.ServerVersion)
	case info != nil:
		t.Logf("TLS and pin ok; LXD says: %v", err)
	default:
		t.Fatalf("handshake or pin failed: %v", err)
	}

	cfg.ServerCert = cfg.ClientCert // any other certificate
	wrong, err := newLXDREST(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wrong.serverInfo(t.Context()); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("wrong pin accepted: %v", err)
	}
}
