// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"unheaded/cmd/unheaded-cli/output"
	"unheaded/pkg/logger"
)

// Until 2026-10-23 most of this CLI printed sample data: container commands
// ran against a fresh in-memory mock, status/service listed hardcoded
// services ("architect Pending", uptime "2d 5h 30m") and reported every
// localhost endpoint healthy without checking, network printed firewall
// rules the hosts do not have, and secrets went to an in-memory store.

// kingdom is one fake Cuirass + Akira + dashboard-backend.
type kingdom struct {
	mu       sync.Mutex
	desired  map[string]map[string]any
	requests []string
	auth     []string
	akira    map[string]akiraVerdict
	logsQ    string
}

func newKingdom(t *testing.T) (*kingdom, *Context, *bytes.Buffer) {
	t.Helper()
	k := &kingdom{desired: map[string]map[string]any{}, akira: map[string]akiraVerdict{
		"wotan":    {Service: "wotan", Reporters: 3, Severity: "OK"},
		"timeguru": {Service: "timeguru", Reporters: 3, Failing: 2, FailureRate: 2.0 / 3, Severity: "CRITICAL", FailingReporters: []string{"akira-west", "akira-east"}},
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/containers", func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		for id, spec := range k.desired {
			out = append(out, map[string]any{"id": id, "desired": spec, "actual": nil})
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("PUT /api/v1/containers/{id}", func(w http.ResponseWriter, r *http.Request) {
		var spec map[string]any
		json.NewDecoder(r.Body).Decode(&spec)
		k.desired[r.PathValue("id")] = spec
		json.NewEncoder(w).Encode(map[string]any{"message": "desired state updated"})
	})
	mux.HandleFunc("DELETE /api/v1/containers/{id}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := k.desired[r.PathValue("id")]; !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "container not found in desired state"})
			return
		}
		delete(k.desired, r.PathValue("id"))
	})
	mux.HandleFunc("GET /api/v1/containers/{id}", func(w http.ResponseWriter, r *http.Request) {
		spec, ok := k.desired[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "container not found"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": r.PathValue("id"), "desired": spec,
			"actual": map[string]any{"id": r.PathValue("id"), "status": "running", "ip": "10.10.20.9", "managed": true}})
	})
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(k.akira)
	})
	mux.HandleFunc("GET /api/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		k.logsQ = r.URL.RawQuery
		json.NewEncoder(w).Encode([]map[string]any{{"timestamp": "2026-10-23T05:00:00Z", "service": "wotan", "level": "info", "message": "topic approved"}})
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.mu.Lock()
		defer k.mu.Unlock()
		k.requests = append(k.requests, r.Method+" "+r.URL.Path)
		k.auth = append(k.auth, r.Header.Get("Authorization"))
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	cfg := DefaultConfig()
	cfg.Contexts["local"] = Cluster{Name: "local", Endpoint: srv.URL, Akira: srv.URL, Dashboard: srv.URL}
	return k, testContext(cfg), testBuf
}

var testBuf *bytes.Buffer

func testContext(cfg *Config) *Context {
	testBuf = &bytes.Buffer{}
	w := output.NewWriter().SetOutput(testBuf).SetErrorOutput(testBuf).SetColor(false)
	return &Context{Config: cfg, Output: w, Logger: logger.New(io.Discard)}
}

// run finds the leaf command for path and runs it with args.
func run(t *testing.T, ctx *Context, root *Command, path string, args ...string) error {
	t.Helper()
	c := root
	for _, name := range strings.Fields(path) {
		sub, ok := c.SubCommands[name]
		if !ok {
			t.Fatalf("no command %q under %q", name, c.Name)
		}
		c = sub
	}
	if c.Flags != nil {
		if err := c.Flags.Parse(args); err != nil {
			return err
		}
		args = c.Flags.Args()
	}
	return c.RunFunc(ctx, args)
}

func TestContainer_CreateListDeleteGoThroughCuirass(t *testing.T) {
	k, ctx, out := newKingdom(t)
	t.Setenv("UNHEADED_TOKEN", "tok-1")
	root := NewContainerCommand()

	if err := run(t, ctx, root, "create", "--image", "images:debian/13", "--cpu", "2", "--memory", "1GiB", "--labels", "tier=app", "web"); err != nil {
		t.Fatal(err)
	}
	spec := k.desired["web"]
	if spec["image"] != "images:debian/13" || spec["cpu"] != float64(2) || spec["memory"] != float64(1<<30) {
		t.Errorf("PUT body = %v", spec)
	}
	if k.auth[0] != "Bearer tok-1" {
		t.Errorf("Authorization = %q", k.auth[0])
	}

	out.Reset()
	if err := run(t, ctx, root, "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "web") || !strings.Contains(out.String(), "missing") {
		t.Errorf("list output:\n%s", out)
	}

	if err := run(t, ctx, root, "delete", "web", "nope"); err == nil {
		t.Error("delete of an unknown container reported success")
	}
	if _, ok := k.desired["web"]; ok {
		t.Error("web still desired")
	}
}

func TestContainer_InspectShowsActual(t *testing.T) {
	k, ctx, out := newKingdom(t)
	k.desired["web"] = map[string]any{"id": "web", "image": "nixos"}
	if err := run(t, ctx, NewContainerCommand(), "inspect", "web"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"10.10.20.9", "nixos", "Managed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("inspect lacks %q:\n%s", want, out)
		}
	}
	if err := run(t, ctx, NewContainerCommand(), "inspect", "ghost"); err == nil || !strings.Contains(err.Error(), "not desired") {
		t.Errorf("inspect ghost: %v", err)
	}
}

func TestContainer_CuirassDownIsAnError(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Contexts["local"] = Cluster{Name: "local", Endpoint: "http://127.0.0.1:1"}
	ctx := testContext(cfg)
	if err := run(t, ctx, NewContainerCommand(), "list"); err == nil {
		t.Fatal("list against a dead Cuirass succeeded")
	}
}

func TestService_ListAndStatusFromAkira(t *testing.T) {
	_, ctx, out := newKingdom(t)
	root := NewServiceCommand()
	if err := run(t, ctx, root, "list"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "timeguru") || !strings.Contains(out.String(), "wotan") {
		t.Errorf("list:\n%s", out)
	}
	out.Reset()
	if err := run(t, ctx, root, "status", "timeguru"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "CRITICAL") || !strings.Contains(out.String(), "2 of 3") {
		t.Errorf("status:\n%s", out)
	}
	if err := run(t, ctx, root, "status", "architect"); err == nil {
		t.Error("status of a service Akira does not check succeeded")
	}
}

func TestService_LogsQueryDashboard(t *testing.T) {
	k, ctx, out := newKingdom(t)
	if err := run(t, ctx, NewServiceCommand(), "logs", "--tail", "5", "--level", "error", "wotan"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"service=wotan", "limit=5", "level=error"} {
		if !strings.Contains(k.logsQ, want) {
			t.Errorf("query %q lacks %s", k.logsQ, want)
		}
	}
	if !strings.Contains(out.String(), "topic approved") {
		t.Errorf("logs:\n%s", out)
	}
}

// status used to report every localhost service healthy without asking.
func TestStatus_UnreachableIsReportedNotInvented(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Contexts["local"] = Cluster{Name: "local", Endpoint: "http://127.0.0.1:1", Akira: "http://127.0.0.1:1", Dashboard: "http://127.0.0.1:1"}
	ctx := testContext(cfg)
	s := gatherSystemStatus(ctx)
	if s.ServiceErr == "" || len(s.Services) != 0 {
		t.Errorf("services: err=%q list=%v", s.ServiceErr, s.Services)
	}
	if s.ContainErr == "" || s.Containers != nil {
		t.Errorf("containers: err=%q counts=%v", s.ContainErr, s.Containers)
	}
}

func TestStatus_ReadsAkiraAndCuirass(t *testing.T) {
	k, ctx, _ := newKingdom(t)
	k.desired["web"] = map[string]any{"id": "web"}
	s := gatherSystemStatus(ctx)
	if s.ServiceErr != "" || len(s.Services) != 2 || s.Containers["missing"] != 1 {
		t.Errorf("status = %+v", s)
	}
}

// Commands nothing serves fail, and say what to use instead.
func TestNoAPICommandsFail(t *testing.T) {
	ctx := testContext(DefaultConfig())
	for _, tc := range []struct {
		root *Command
		path string
	}{
		{NewContainerCommand(), "start"}, {NewContainerCommand(), "stop"}, {NewContainerCommand(), "exec"},
		{NewServiceCommand(), "deploy"}, {NewServiceCommand(), "restart"}, {NewServiceCommand(), "scale"},
		{NewNetworkCommand(), "policy list"}, {NewNetworkCommand(), "route list"}, {NewNetworkCommand(), "inspect"},
		{NewSecretCommand(), "get"}, {NewSecretCommand(), "set"}, {NewSecretCommand(), "list"}, {NewSecretCommand(), "generate"},
	} {
		err := run(t, ctx, tc.root, tc.path, "x", "k=v")
		if err == nil || !strings.Contains(err.Error(), "not available") {
			t.Errorf("%s %s: err = %v, want not available", tc.root.Name, tc.path, err)
		}
	}
}

func TestParseBytes(t *testing.T) {
	for in, want := range map[string]int64{"512MiB": 512 << 20, "2GiB": 2 << 30, "1GB": 1e9, "4096": 4096, "10B": 10} {
		if got, err := parseBytes(in); err != nil || got != want {
			t.Errorf("parseBytes(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "-1MiB", "0", "lots", "9999999999TiB"} {
		if _, err := parseBytes(in); err == nil {
			t.Errorf("parseBytes(%q) accepted", in)
		}
	}
}

func TestStatus_FailsWhenASourceIsDown(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Contexts["local"] = Cluster{Name: "local", Endpoint: "http://127.0.0.1:1", Akira: "http://127.0.0.1:1"}
	if err := showStatus(testContext(cfg)); err == nil {
		t.Fatal("status with nothing answering exited 0")
	}
}
