// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sword

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/deploy/cuirassdriver"
	"unheaded/pkg/deploy/cuirassdriver/cuirasstest"
	"unheaded/pkg/deploy/strategy"
	wotanClient "unheaded/pkg/wotan-client"
)

// recordingPublisher keeps every event.
type recordingPublisher struct {
	mu     sync.Mutex
	events []event
}

type event struct {
	topic string
	body  map[string]any
}

func (p *recordingPublisher) Publish(_ context.Context, topic string, payload []byte) error {
	var m map[string]any
	_ = json.Unmarshal(payload, &m)
	p.mu.Lock()
	p.events = append(p.events, event{topic, m})
	p.mu.Unlock()
	return nil
}

func (p *recordingPublisher) topics() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, e := range p.events {
		out = append(out, e.topic)
	}
	return out
}

// stubAnalysis answers every analysis with a fixed result.
type stubAnalysis struct{ pass bool }

func (a stubAnalysis) Analyze(_ context.Context, _ *strategy.AnalysisRequest, _ int) (*strategy.AnalysisResult, error) {
	if a.pass {
		return &strategy.AnalysisResult{Pass: true, Message: "ok",
			Metrics: &strategy.AnalysisMetrics{ErrorRate: 0, SuccessRate: 1, RequestCount: 500}}, nil
	}
	return &strategy.AnalysisResult{Pass: false, ShouldRollback: true, Message: "canary error ratio 0.3000 over 300 requests",
		Metrics: &strategy.AnalysisMetrics{ErrorRate: 0.3, SuccessRate: 0.7, RequestCount: 300, ErrorCount: 90}}, nil
}

type fixture struct {
	f   *cuirasstest.Fake
	s   *Service
	pub *recordingPublisher
	dir string
	h   http.Handler
}

func newFixture(t *testing.T, mutate ...func(*Config)) *fixture {
	t.Helper()
	f := cuirasstest.New(t)
	d, err := cuirassdriver.New(cuirassdriver.Config{URL: f.URL, APIKey: cuirasstest.Key,
		PollInterval: 2 * time.Millisecond, Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	fx := &fixture{f: f, pub: &recordingPublisher{}, dir: t.TempDir()}
	cfg := Config{
		DataDir:      fx.dir,
		AuthEnabled:  true,
		Driver:       d,
		Analysis:     stubAnalysis{pass: true},
		Publisher:    fx.pub,
		PollInterval: 2 * time.Millisecond,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	fx.s = s
	fx.h = s.Handler()
	return fx
}

func reqBody(ver, typ string) string {
	strat := `{"type":"` + typ + `"}`
	switch typ {
	case "rolling":
		strat = `{"type":"rolling","max_surge":1,"max_unavailable":0}`
	case "canary":
		strat = `{"type":"canary","steps":[{"weight":33,"analysis":true},{"weight":100}],` +
			`"analysis":{"error_rate_threshold":0.01,"min_sample_size":200,"interval":"1ms"}}`
	}
	return `{"service":"steel-echo","version":"` + ver + `","replicas":3,"strategy":` + strat + `,` +
		`"health":{"port":8080,"path":"/health"},"proxy":{"listen":"tcp:127.0.0.1:26601","connect":"tcp:127.0.0.1:8080"},` +
		`"labels":{"chaos":"allowed"}}`
}

func (fx *fixture) do(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	fx.h.ServeHTTP(rec, r)
	return rec
}

func (fx *fixture) deploy(t *testing.T, ver, typ string) *Deployment {
	t.Helper()
	rec := fx.do(t, http.MethodPost, "/api/v1/deployments", reqBody(ver, typ))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST deployment %s %s = %d %s", ver, typ, rec.Code, rec.Body.String())
	}
	var d Deployment
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return fx.wait(t, d.ID)
}

func (fx *fixture) wait(t *testing.T, id string) *Deployment {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	d, err := fx.s.Wait(ctx, id)
	if err != nil {
		t.Fatalf("wait %s: %v", id, err)
	}
	return d
}

// assertOnly checks Cuirass's own state, not Sword's bookkeeping: exactly n
// instances, all of version ver, all reported healthy by Cuirass, one of
// them holding the stable endpoint.
func assertOnly(t *testing.T, f *cuirasstest.Fake, ver string, n int) {
	t.Helper()
	ids := f.DesiredIDs()
	healthy := f.HealthyVersions()
	if len(ids) != n || len(healthy) != n {
		t.Fatalf("desired %v, healthy %v; want %d healthy %s", ids, healthy, n, ver)
	}
	for _, v := range healthy {
		if v != ver {
			t.Fatalf("healthy versions %v, want only %s", healthy, ver)
		}
	}
	if h := f.ProxyHolders(); len(h) != 1 || !strings.HasPrefix(h[0], "steel-echo-"+ver+"-") {
		t.Fatalf("proxy holders %v, want one %s instance", h, ver)
	}
}

func TestSword_DeployRunsStrategyThroughDriver(t *testing.T) {
	fx := newFixture(t)
	d := fx.deploy(t, "v1", "recreate")
	if d.Status != StatusSucceeded {
		t.Fatalf("v1 recreate = %s: %s", d.Status, d.Message)
	}
	assertOnly(t, fx.f, "v1", 3)

	d = fx.deploy(t, "v2", "rolling")
	if d.Status != StatusSucceeded || d.PreviousVersion != "v1" {
		t.Fatalf("v2 rolling = %+v", d)
	}
	assertOnly(t, fx.f, "v2", 3)

	d = fx.deploy(t, "v3", "blue_green")
	if d.Status != StatusSucceeded {
		t.Fatalf("v3 blue-green = %s: %s", d.Status, d.Message)
	}
	assertOnly(t, fx.f, "v3", 3)
}

func TestSword_RollbackRestoresPreviousVersion(t *testing.T) {
	fx := newFixture(t)
	fx.deploy(t, "v1", "recreate")
	d2 := fx.deploy(t, "v2", "rolling")

	rec := fx.do(t, http.MethodPost, "/api/v1/deployments/"+d2.ID+"/rollback", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rollback = %d %s", rec.Code, rec.Body.String())
	}
	d := fx.wait(t, d2.ID)
	if d.Status != StatusRolledBack || d.Reason != ReasonManual || d.RolledBackTo != "v1" {
		t.Fatalf("after rollback: %+v", d)
	}
	assertOnly(t, fx.f, "v1", 3)
	if got := metricValue(t, fx, `sword_rollbacks_total{reason="manual"}`); got != "1" {
		t.Fatalf("sword_rollbacks_total{reason=manual} = %q", got)
	}
}

func TestSword_FailedDeployRollsBackAutomatically(t *testing.T) {
	fx := newFixture(t)
	fx.deploy(t, "v1", "recreate")
	fx.f.Mu.Lock()
	fx.f.NeverHealthy["steel-echo-v2"] = true
	fx.f.Mu.Unlock()

	d := fx.deploy(t, "v2", "rolling")
	if d.Status != StatusRolledBack || d.Reason != ReasonDeployFailed || d.RolledBackTo != "v1" {
		t.Fatalf("a v2 that never got healthy ended %+v", d)
	}
	assertOnly(t, fx.f, "v1", 3)
	topics := strings.Join(fx.pub.topics(), ",")
	if !strings.Contains(topics, TopicFailed) || !strings.Contains(topics, TopicRolledBack) {
		t.Fatalf("events %s", topics)
	}
}

// DoD: Sword never reports success without a Cuirass-confirmed healthy
// instance. With nothing ever healthy and nothing to roll back to, the
// first deploy fails.
func TestSword_NeverSucceedsWithoutCuirassHealthy(t *testing.T) {
	fx := newFixture(t)
	fx.f.Mu.Lock()
	fx.f.NeverHealthy["steel-echo-v1"] = true
	fx.f.Mu.Unlock()
	d := fx.deploy(t, "v1", "recreate")
	if d.Status != StatusFailed {
		t.Fatalf("status %s, want failed", d.Status)
	}
	if h := fx.f.HealthyVersions(); len(h) != 0 {
		t.Fatalf("Cuirass reports healthy %v", h)
	}
}

func TestSword_CanaryAnalysisFailureRollsBack(t *testing.T) {
	fx := newFixture(t, func(c *Config) { c.Analysis = stubAnalysis{pass: false} })
	fx.deploy(t, "v3", "recreate")
	d := fx.deploy(t, "v4", "canary")
	if d.Status != StatusRolledBack || d.Reason != ReasonAnalysisFailed {
		t.Fatalf("bad canary ended %+v", d)
	}
	if d.Analysis == nil || d.Analysis.Metrics == nil || d.Analysis.Metrics.ErrorRate != 0.3 {
		t.Fatalf("the record lacks the measured error ratio: %+v", d.Analysis)
	}
	assertOnly(t, fx.f, "v3", 3)
	if got := metricValue(t, fx, `sword_rollbacks_total{reason="analysis_failed"}`); got != "1" {
		t.Fatalf("sword_rollbacks_total{reason=analysis_failed} = %q", got)
	}
}

func TestSword_CanaryWithoutAnalysisIsRefused(t *testing.T) {
	fx := newFixture(t, func(c *Config) { c.Analysis = nil })
	rec := fx.do(t, http.MethodPost, "/api/v1/deployments", reqBody("v4", "canary"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("canary with no analysis configured = %d", rec.Code)
	}
}

func TestSword_HistorySurvivesRestart(t *testing.T) {
	fx := newFixture(t)
	d1 := fx.deploy(t, "v1", "recreate")
	fx.s.Close()

	s2, err := New(Config{DataDir: fx.dir, AuthEnabled: true, Driver: fx.s.cfg.Driver, PollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, ok := s2.Get(d1.ID)
	if !ok || got.Status != StatusSucceeded || got.Version != "v1" {
		t.Fatalf("after restart: %+v %v", got, ok)
	}
}

func TestSword_InterruptedDeployIsFailedAfterRestart(t *testing.T) {
	dir := t.TempDir()
	st := stateFile{Version: 1, Deployments: []*Deployment{{ID: "d1-1", Service: "steel-echo", Version: "v1",
		Replicas: 1, Status: StatusInProgress, CreatedAt: time.Now()}}}
	b, _ := json.Marshal(st)
	if err := os.WriteFile(filepath.Join(dir, stateFileName), b, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{DataDir: dir, AuthEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d, _ := s.Get("d1-1")
	if d.Status != StatusFailed || !strings.Contains(d.Message, "restart") {
		t.Fatalf("an in-progress record after restart = %+v", d)
	}
}

func TestSword_DamagedStateIsFatal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{DataDir: dir}); err == nil {
		t.Fatal("a damaged state file was read as empty")
	}
}

func TestSword_NoDataDirIsReadOnly(t *testing.T) {
	fx := newFixture(t, func(c *Config) { c.DataDir = "" })
	if rec := fx.do(t, http.MethodPost, "/api/v1/deployments", reqBody("v1", "recreate")); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST without a data dir = %d, want 503", rec.Code)
	}
	if rec := fx.do(t, http.MethodGet, "/api/v1/deployments", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET without a data dir = %d", rec.Code)
	}
	if ids := fx.f.DesiredIDs(); len(ids) != 0 {
		t.Fatalf("a refused deploy reached Cuirass: %v", ids)
	}
}

func TestSword_FailedSaveIsUndone(t *testing.T) {
	fx := newFixture(t)
	// Make the data dir unwritable for the save.
	if err := os.Chmod(fx.dir, 0o500); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(fx.dir, 0o700) }()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	rec := fx.do(t, http.MethodPost, "/api/v1/deployments", reqBody("v1", "recreate"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST with an unwritable data dir = %d, want 503", rec.Code)
	}
	if n := len(fx.s.List()); n != 0 {
		t.Fatalf("the refused deployment is still listed (%d)", n)
	}
}

func TestSword_MutatingRoutesNeedAuth(t *testing.T) {
	fx := newFixture(t, func(c *Config) { c.AuthEnabled = false })
	for _, p := range []string{"/api/v1/deployments", "/api/v1/deployments/x/rollback"} {
		if rec := fx.do(t, http.MethodPost, p, reqBody("v1", "recreate")); rec.Code != http.StatusForbidden {
			t.Fatalf("POST %s without AUTH_ENABLED = %d", p, rec.Code)
		}
	}
}

func TestSword_RefusesBadRequests(t *testing.T) {
	fx := newFixture(t)
	good := reqBody("v1", "recreate")
	bad := map[string]string{
		"version with dots":    strings.Replace(good, `"v1"`, `"v1.2"`, 1),
		"empty version":        strings.Replace(good, `"v1"`, `""`, 1),
		"zero replicas":        strings.Replace(good, `"replicas":3`, `"replicas":0`, 1),
		"too many replicas":    strings.Replace(good, `"replicas":3`, `"replicas":50`, 1),
		"unknown strategy":     strings.Replace(good, `"recreate"`, `"yolo"`, 1),
		"no health":            strings.Replace(good, `"health":{"port":8080,"path":"/health"},`, ``, 1),
		"bad service":          strings.Replace(good, `"steel-echo"`, `"Steel Echo"`, 1),
		"label owned by sword": strings.Replace(good, `"chaos":"allowed"`, `"svc":"other"`, 1),
		"bad duration":         strings.Replace(reqBody("v1", "canary"), `"1ms"`, `"soon"`, 1),
		"not json":             "{",
	}
	for name, body := range bad {
		if rec := fx.do(t, http.MethodPost, "/api/v1/deployments", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if ids := fx.f.DesiredIDs(); len(ids) != 0 {
		t.Fatalf("a refused request reached Cuirass: %v", ids)
	}
	big := `{"service":"` + strings.Repeat("a", 70<<10) + `"}`
	if rec := fx.do(t, http.MethodPost, "/api/v1/deployments", big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body = %d", rec.Code)
	}
}

func TestSword_OneOperationPerService(t *testing.T) {
	fx := newFixture(t)
	fx.f.Mu.Lock()
	fx.f.HealthyAfter = 50 // slow enough to overlap
	fx.f.Mu.Unlock()
	first := fx.do(t, http.MethodPost, "/api/v1/deployments", reqBody("v1", "recreate"))
	if first.Code != http.StatusAccepted {
		t.Fatalf("first = %d", first.Code)
	}
	if rec := fx.do(t, http.MethodPost, "/api/v1/deployments", reqBody("v2", "recreate")); rec.Code != http.StatusConflict {
		t.Fatalf("second while the first runs = %d, want 409", rec.Code)
	}
	var d Deployment
	_ = json.Unmarshal(first.Body.Bytes(), &d)
	fx.wait(t, d.ID)
}

func TestSword_GetAndList(t *testing.T) {
	fx := newFixture(t)
	d := fx.deploy(t, "v1", "recreate")
	rec := fx.do(t, http.MethodGet, "/api/v1/deployments/"+d.ID, "")
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"succeeded"`)) {
		t.Fatalf("GET one = %d %s", rec.Code, rec.Body.String())
	}
	if rec := fx.do(t, http.MethodGet, "/api/v1/deployments/nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("GET unknown = %d", rec.Code)
	}
	rec = fx.do(t, http.MethodGet, "/api/v1/deployments", "")
	var list []Deployment
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list = %s (%v)", rec.Body.String(), err)
	}
	if rec := fx.do(t, http.MethodPost, "/api/v1/deployments/nope/rollback", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("rollback unknown = %d", rec.Code)
	}
}

func TestSword_ReadyNeedsCuirassAndDataDir(t *testing.T) {
	fx := newFixture(t)
	if rec := fx.do(t, http.MethodGet, "/ready", ""); rec.Code != http.StatusOK {
		t.Fatalf("/ready = %d %s", rec.Code, rec.Body.String())
	}
	fx.f.Mu.Lock()
	fx.f.Reachable = false
	fx.f.Mu.Unlock()
	if rec := fx.do(t, http.MethodGet, "/ready", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready with Cuirass down = %d", rec.Code)
	}
	if rec := fx.do(t, http.MethodGet, "/health", ""); rec.Code != http.StatusOK {
		t.Fatalf("/health = %d (degraded is not dead)", rec.Code)
	}
	fx2 := newFixture(t, func(c *Config) { c.DataDir = "" })
	if rec := fx2.do(t, http.MethodGet, "/ready", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready without a data dir = %d", rec.Code)
	}
}

func TestSword_PublishesDeployEvents(t *testing.T) {
	fw := &fakeWotan{}
	fx := newFixture(t, func(c *Config) { c.Publisher = NewWotanPublisher(fw) })
	d := fx.deploy(t, "v1", "recreate")
	if d.Status != StatusSucceeded {
		t.Fatal(d.Message)
	}
	fw.mu.Lock()
	defer fw.mu.Unlock()
	joined := map[string]bool{}
	published := map[string]int{}
	for _, c := range fw.calls {
		switch c.kind {
		case "join":
			if c.name != "sword" {
				t.Fatalf("joined %s as %q, want sword", c.topic, c.name)
			}
			joined[c.topic] = true
		case "publish":
			if !joined[c.topic] {
				t.Fatalf("published on %s before joining it", c.topic)
			}
			published[c.topic]++
		}
	}
	if published[TopicStarted] != 1 || published[TopicSucceeded] != 1 {
		t.Fatalf("published %v", published)
	}
}

type wotanCall struct{ kind, topic, name string }

type fakeWotan struct {
	mu    sync.Mutex
	calls []wotanCall
}

func (w *fakeWotan) SubscribeWithRetry(_ context.Context, topic, name string, onMsg func(*wotanClient.Message)) error {
	if onMsg != nil {
		return errors.New("sword only joins to publish")
	}
	w.mu.Lock()
	w.calls = append(w.calls, wotanCall{"join", topic, name})
	w.mu.Unlock()
	return nil
}

func (w *fakeWotan) Publish(_ context.Context, topic string, _ []byte) error {
	w.mu.Lock()
	w.calls = append(w.calls, wotanCall{"publish", topic, ""})
	w.mu.Unlock()
	return nil
}

func metricValue(t *testing.T, fx *fixture, series string) string {
	t.Helper()
	rec := fx.do(t, http.MethodGet, "/metrics", "")
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if strings.HasPrefix(line, series+" ") {
			return strings.TrimSpace(strings.TrimPrefix(line, series))
		}
	}
	return ""
}
