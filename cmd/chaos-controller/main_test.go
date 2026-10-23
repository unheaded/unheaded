// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/logger"
	"unheaded/services/yaldabaoth"
)

// lab is a fake Cuirass and a fake Victoria.
func lab(t *testing.T) config {
	t.Helper()
	cuirass := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/info":
			_, _ = w.Write([]byte(`{"lxd_project":"unheaded"}`))
		case "/api/v1/state/actual":
			_, _ = w.Write([]byte(`{"steel-echo-v2-0":{"status":"running","ip":"10.10.10.31","managed":true,"labels":{"chaos":"allowed"}},` +
				`"pihole":{"status":"running","ip":"10.10.10.2","managed":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(cuirass.Close)
	vm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"result":[{"metric":{"slo":"wotan"},"value":[1,"0"]}]}}`))
	}))
	t.Cleanup(vm.Close)
	policy := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(policy, []byte("slos:\n  - name: wotan\n    objective: 99.5\n    page: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return config{
		allowlist:  filepath.Join("..", "..", "configs", "chaos", "allowlist.yaml"),
		stateDir:   t.TempDir(),
		abortFile:  filepath.Join(t.TempDir(), "ABORT"),
		dryRun:     false,
		cuirassURL: cuirass.URL,
		vmURL:      vm.URL,
		policy:     policy,
	}
}

// testInjector fakes one fault so the routes can be driven end to end.
type testInjector struct {
	mu     sync.Mutex
	active map[string]bool
}

func (f *testInjector) Plan(_ context.Context, t yaldabaoth.ResolvedTarget, _ yaldabaoth.Params) (yaldabaoth.Recipe, error) {
	return yaldabaoth.Recipe{"fault": t.ID}, nil
}
func (f *testInjector) Inject(_ context.Context, _ yaldabaoth.ResolvedTarget, _ yaldabaoth.Params, r yaldabaoth.Recipe) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.active[r["fault"]] = true
	return nil
}
func (f *testInjector) Revert(_ context.Context, r yaldabaoth.Recipe) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.active, r["fault"])
	return nil
}
func (f *testInjector) Reverted(_ context.Context, r yaldabaoth.Recipe) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.active[r["fault"]], nil
}
func (f *testInjector) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.active)
}

func withInjector(t *testing.T, inj *testInjector) {
	t.Helper()
	old := registerInjectors
	registerInjectors = func(s *yaldabaoth.Service) { s.RegisterInjector(yaldabaoth.ExpTypeNetworkDelay, inj) }
	t.Cleanup(func() { registerInjectors = old })
}

type harness struct {
	s   *yaldabaoth.Service
	a   *api
	h   http.Handler
	inj *testInjector
}

func newHarness(t *testing.T, authEnabled bool) *harness {
	t.Helper()
	inj := &testInjector{active: map[string]bool{}}
	withInjector(t, inj)
	s, err := build(lab(t), logger.New(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	a := newAPI(s, nil, authEnabled)
	return &harness{s: s, a: a, h: a.routes(s.Registry()), inj: inj}
}

func (h *harness) do(method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, r)
	return rec
}

const delayBody = `{"name":"rs-netem-1","type":"network_delay","target":{"kind":"lxd","id":"steel-echo-v2-0"},` +
	`"parameters":{"delay_ms":200,"jitter_ms":20},"schedule":{"duration":"%s"}}`

func body(d string) string { return strings.Replace(delayBody, "%s", d, 1) }

func TestBuild_RefusesWithoutAllowlist(t *testing.T) {
	c := lab(t)
	c.allowlist = filepath.Join(t.TempDir(), "missing.yaml")
	if _, err := build(c, logger.New(io.Discard)); err == nil {
		t.Fatal("started without an allowlist")
	}
}

func TestBuild_DamagedJournalIsFatal(t *testing.T) {
	c := lab(t)
	if err := os.WriteFile(filepath.Join(c.stateDir, "journal.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := build(c, logger.New(io.Discard)); err == nil {
		t.Fatal("started over a damaged journal")
	}
}

func TestLoadConfig_DryRunUnlessFalse(t *testing.T) {
	for v, want := range map[string]bool{"": true, "true": true, "0": true, "no": true, "false": false} {
		c := loadConfig(func(k string) string {
			if k == "CHAOS_DRY_RUN" {
				return v
			}
			return ""
		})
		if c.dryRun != want {
			t.Errorf("CHAOS_DRY_RUN=%q: dry run %v", v, c.dryRun)
		}
	}
}

func TestRoutes_WritesNeedAuth(t *testing.T) {
	h := newHarness(t, false)
	for _, p := range []string{"/api/v1/chaos/experiments", "/api/v1/chaos/abort", "/api/v1/chaos/inject", "/api/v1/chaos/gameday"} {
		if rec := h.do(http.MethodPost, p, body("1s")); rec.Code != http.StatusForbidden {
			t.Errorf("POST %s without AUTH_ENABLED = %d", p, rec.Code)
		}
	}
	if h.inj.count() != 0 {
		t.Fatal("injected without auth")
	}
}

func TestRoutes_StartGetListAbort(t *testing.T) {
	h := newHarness(t, true)
	rec := h.do(http.MethodPost, "/api/v1/chaos/experiments", body("30s"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("start = %d %s", rec.Code, rec.Body.String())
	}
	var e yaldabaoth.Experiment
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	deadline := time.Now().Add(5 * time.Second)
	for h.inj.count() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := h.do(http.MethodGet, "/api/v1/chaos/experiments/"+e.ID, ""); got.Code != http.StatusOK {
		t.Fatalf("get = %d", got.Code)
	}
	if got := h.do(http.MethodGet, "/api/v1/chaos/experiments", ""); !strings.Contains(got.Body.String(), e.ID) {
		t.Fatal("list lacks the experiment")
	}
	if got := h.do(http.MethodPost, "/api/v1/chaos/abort", ""); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"aborted":1`) {
		t.Fatalf("abort = %d %s", got.Code, got.Body.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done, err := h.s.Wait(ctx, e.ID)
	if err != nil || done.Status != yaldabaoth.StatusAborted || h.inj.count() != 0 {
		t.Fatalf("after abort: %v %s, %d faults", err, done.Status, h.inj.count())
	}
	if got := h.do(http.MethodGet, "/api/v1/chaos/experiments/nope", ""); got.Code != http.StatusNotFound {
		t.Fatalf("unknown = %d", got.Code)
	}
}

func TestRoutes_RefusalsMapToStatus(t *testing.T) {
	h := newHarness(t, true)
	cases := map[string]struct {
		body string
		code int
	}{
		"pihole":          {strings.Replace(body("1s"), "steel-echo-v2-0", "pihole", 1), http.StatusForbidden},
		"over the limits": {body("11m"), http.StatusForbidden},
		"unknown type":    {strings.Replace(body("1s"), "network_delay", "melt", 1), http.StatusBadRequest},
		"no injector":     {strings.Replace(body("1s"), "network_delay", "container_kill", 1), http.StatusNotImplemented},
		"bad json":        {"{", http.StatusBadRequest},
	}
	for name, c := range cases {
		if rec := h.do(http.MethodPost, "/api/v1/chaos/experiments", c.body); rec.Code != c.code {
			t.Errorf("%s = %d %s, want %d", name, rec.Code, rec.Body.String(), c.code)
		}
	}
	if h.inj.count() != 0 {
		t.Fatal("a refused experiment injected")
	}
	var b bytes.Buffer
	b.WriteString(h.do(http.MethodGet, "/metrics", "").Body.String())
	if !strings.Contains(b.String(), `chaos_refusals_total{reason="allowlist"}`) {
		t.Fatalf("/metrics lacks chaos_refusals_total:\n%s", b.String())
	}
}

// The older fault-rule API no longer stores a rule and announces it as
// injected: the rule runs as an experiment and is refused.
func TestInject_LegacyRuleIsRefused(t *testing.T) {
	h := newHarness(t, true)
	rec := h.do(http.MethodPost, "/api/v1/chaos/inject", `{"src_svc":1,"dst_svc":2,"fault_type":"DELAY","delay_us":50000,"duration":5}`)
	if rec.Code != http.StatusForbidden || h.inj.count() != 0 {
		t.Fatalf("legacy inject = %d %s, %d faults", rec.Code, rec.Body.String(), h.inj.count())
	}
}

func TestGameDay_Route(t *testing.T) {
	h := newHarness(t, true)
	sc := `{"name":"gd","duration_ns":10000000000,"stages":[{"name":"s1","experiments":[` + body("50ms") + `]}]}`
	if rec := h.do(http.MethodPost, "/api/v1/chaos/gameday", sc); rec.Code != http.StatusAccepted {
		t.Fatalf("gameday = %d %s", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := h.a.gameday.Status(); st.State == yaldabaoth.StateCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("game day did not complete: %+v", h.a.gameday.Status())
}

func seedJournal(t *testing.T, dir string) {
	t.Helper()
	j := map[string]any{"version": 1, "entries": []map[string]any{{
		"id": "chaos-left-1", "type": "network_delay",
		"target":  map[string]string{"kind": "lxd", "id": "steel-echo-v2-0"},
		"recipe":  map[string]string{"fault": "steel-echo-v2-0"},
		"created": time.Now().Add(-time.Hour), "expires": time.Now().Add(time.Hour),
	}}}
	b, _ := json.Marshal(j)
	if err := os.WriteFile(filepath.Join(dir, "journal.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRevertAll(t *testing.T) {
	c := lab(t)
	seedJournal(t, c.stateDir)
	var out bytes.Buffer
	// Without an injector for the type the fault cannot be reverted: non-zero.
	if code := revertAll(c, &out); code != 1 {
		t.Fatalf("revert-all with no injector = %d (%s)", code, out.String())
	}
	inj := &testInjector{active: map[string]bool{"steel-echo-v2-0": true}}
	withInjector(t, inj)
	out.Reset()
	if code := revertAll(c, &out); code != 0 || inj.count() != 0 {
		t.Fatalf("revert-all = %d (%s), faults %d", code, out.String(), inj.count())
	}
	// Idempotent.
	if code := revertAll(c, &out); code != 0 {
		t.Fatalf("second revert-all = %d", code)
	}
}
