// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/metrics/prom"
	"unheaded/pkg/zhenagent"
)

// fakeSchedChampion is the Champion as the scheduler sees it: runbooks by
// name (their YAML), and what runbook_execute returns for each.
type fakeSchedChampion struct {
	mu       sync.Mutex
	runbooks map[string]string
	exits    map[string]int
	errs     map[string]error
	calls    []champion.ToolCall
	cards    map[string]map[string]any
	noKanban bool
}

func newFakeSchedChampion() *fakeSchedChampion {
	return &fakeSchedChampion{
		runbooks: map[string]string{
			"observe/service-health-sweep":           "metadata:\n  name: service-health-sweep\nsteps: []\n",
			"data/conversation-memory-maintenance":   "metadata:\n  name: conversation-memory-maintenance\nsteps: []\n",
			"observe/approved-one":                   "metadata:\n  name: approved-one\n  status: approved\n  approved_by: stevie\nsteps: []\n",
			"network/air-gap-egress-validation":      "metadata:\n  name: air-gap-egress-validation\n  status: stub\nsteps: []\n",
			"security/mcp-write-exec-enablement":     "metadata:\n  name: mcp-write-exec-enablement\n  status: draft\n  approved_by: \"\"\nsteps: []\n",
			"wotan/active-active-cutover-unparsable": "metadata: [\n",
		},
		exits: map[string]int{},
		errs:  map[string]error{},
		cards: map[string]map[string]any{},
	}
}

func (f *fakeSchedChampion) Dispatch(_ context.Context, tc champion.ToolCall) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, tc)
	switch tc.Name {
	case "kanban_create":
		if f.noKanban {
			return nil, errors.New("no kanban store configured")
		}
		task, _ := tc.Args["task"].(map[string]any)
		id, _ := task["id"].(string)
		f.cards[id] = task
		return nil, nil
	case "kanban_update":
		if f.noKanban {
			return nil, errors.New("no kanban store configured")
		}
		id, _ := tc.Args["id"].(string)
		up, _ := tc.Args["updates"].(map[string]any)
		for k, v := range up {
			f.cards[id][k] = v
		}
		return nil, nil
	case "runbook_execute":
		name, _ := tc.Args["name"].(string)
		if err := f.errs[name]; err != nil {
			return nil, err
		}
		return &champion.RunbookExecuteResult{Name: name, ExitCode: f.exits[name], Output: "step ok\nverification: 0/1 passed\n"}, nil
	}
	return nil, errors.New("unexpected tool " + tc.Name)
}

func (f *fakeSchedChampion) RunbookShow(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.runbooks[name]
	if !ok {
		return "", errors.New("runbook not found")
	}
	return b, nil
}

func (f *fakeSchedChampion) RunbookList(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k := range f.runbooks {
		out = append(out, k)
	}
	return out, nil
}

func (f *fakeSchedChampion) executed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if c.Name == "runbook_execute" {
			out = append(out, c.Args["name"].(string))
		}
	}
	return out
}

type recPublisher struct {
	mu     sync.Mutex
	topics []string
}

func (p *recPublisher) PublishJSON(topic string, _ any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.topics = append(p.topics, topic)
}

func (p *recPublisher) count(topic string) int {
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

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

func writeSchedules(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "schedules.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

var t0 = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

func newTestScheduler(t *testing.T, body string, f *fakeSchedChampion) (*scheduler, *clock, *recPublisher, *zhenagent.MemStore) {
	t.Helper()
	clk := &clock{t: t0}
	pub := &recPublisher{}
	runs := zhenagent.NewMemStore()
	s, err := newScheduler(context.Background(), schedulerConfig{
		Champion: f, Path: writeSchedules(t, body), Runs: runs, Pub: pub, Now: clk.now,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("newScheduler: %v", err)
	}
	return s, clk, pub, runs
}

const oneJob = `version: 1
jobs:
  - name: service-health-sweep
    runbook: observe/service-health-sweep
    every: 1h
`

func TestParseSchedules(t *testing.T) {
	jobs, err := parseSchedules([]byte(`version: 1
jobs:
  - name: sweep
    runbook: observe/service-health-sweep
    every: 30m
  - name: memory
    runbook: data/conversation-memory-maintenance
    every: weekly@sun@03:00
    enabled: false
  - name: nightly
    runbook: observe/approved-one
    every: daily@02:30
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 || jobs[1].isEnabled() || !jobs[0].isEnabled() || jobs[2].Every != "daily@02:30" {
		t.Fatalf("jobs = %+v", jobs)
	}
	for name, bad := range map[string]string{
		"unknown key":    "version: 1\njobs:\n  - name: a\n    runbook: observe/x\n    every: 1h\n    command: rm\n",
		"no version":     "jobs:\n  - name: a\n    runbook: observe/x\n    every: 1h\n",
		"bad name":       "version: 1\njobs:\n  - name: A B\n    runbook: observe/x\n    every: 1h\n",
		"duplicate":      "version: 1\njobs:\n  - name: a\n    runbook: observe/x\n    every: 1h\n  - name: a\n    runbook: observe/y\n    every: 2h\n",
		"dotdot runbook": "version: 1\njobs:\n  - name: a\n    runbook: ../etc/x\n    every: 1h\n",
		"abs runbook":    "version: 1\njobs:\n  - name: a\n    runbook: /etc/passwd\n    every: 1h\n",
		"no category":    "version: 1\njobs:\n  - name: a\n    runbook: x\n    every: 1h\n",
		"seconds":        "version: 1\njobs:\n  - name: a\n    runbook: observe/x\n    every: 5s\n",
		"cron":           "version: 1\njobs:\n  - name: a\n    runbook: observe/x\n    every: '*/5 * * * *'\n",
		"bad daily":      "version: 1\njobs:\n  - name: a\n    runbook: observe/x\n    every: daily@25:00\n",
		"bad weekday":    "version: 1\njobs:\n  - name: a\n    runbook: observe/x\n    every: weekly@funday@01:00\n",
	} {
		if _, err := parseSchedules([]byte(bad)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestCadenceNext(t *testing.T) {
	for _, c := range []struct {
		every string
		after time.Time
		want  time.Time
	}{
		{"30m", t0, t0.Add(30 * time.Minute)},
		{"6h", t0, t0.Add(6 * time.Hour)},
		{"daily@03:00", t0, time.Date(2026, 10, 28, 3, 0, 0, 0, time.UTC)},
		{"daily@11:15", t0, time.Date(2026, 10, 27, 11, 15, 0, 0, time.UTC)},
		{"daily@10:00", t0, time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)},    // strictly after
		{"weekly@sun@03:00", t0, time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)}, // t0 is a Tuesday
		{"weekly@tue@11:00", t0, time.Date(2026, 10, 27, 11, 0, 0, 0, time.UTC)},
		{"weekly@tue@09:00", t0, time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)},
	} {
		cad, err := parseCadence(c.every)
		if err != nil {
			t.Fatalf("%s: %v", c.every, err)
		}
		if got := cad.next(c.after); !got.Equal(c.want) {
			t.Errorf("%s after %s = %s, want %s", c.every, c.after, got, c.want)
		}
	}
}

// A due job runs once through the Champion: a card, a run row (mode
// scheduled), the runbook, the card closed; ticks before and after it
// change nothing.
func TestScheduler_DueJobRunsOnce(t *testing.T) {
	f := newFakeSchedChampion()
	s, clk, _, runs := newTestScheduler(t, oneJob, f)
	ctx := context.Background()

	clk.set(t0.Add(30 * time.Minute))
	s.tick(ctx)
	s.wait()
	if got := f.executed(); len(got) != 0 {
		t.Fatalf("ran before it was due: %v", got)
	}
	clk.set(t0.Add(time.Hour))
	s.tick(ctx)
	s.wait()
	clk.set(t0.Add(time.Hour + time.Minute))
	s.tick(ctx)
	s.wait()
	if got := f.executed(); len(got) != 1 || got[0] != "observe/service-health-sweep" {
		t.Fatalf("executed = %v, want the runbook once", got)
	}

	// Every call went through the gate with the schedule as a direct-user
	// justification naming the file.
	for _, c := range f.calls {
		if len(c.Justification) != 1 || c.Justification[0].SourceTrust != "direct-user" || !strings.HasSuffix(c.Justification[0].SourcePath, "schedules.yaml") || c.EmittedBy != schedulerEmitter {
			t.Errorf("%s: justification %+v emitted_by %q", c.Name, c.Justification, c.EmittedBy)
		}
	}

	v := s.view()
	if len(v.Jobs) != 1 || v.Jobs[0].LastStatus != zhenagent.StatusSucceeded || v.Jobs[0].LastError != "" || v.Jobs[0].LastRun == "" {
		t.Fatalf("view = %+v", v)
	}
	if !v.Jobs[0].NextDue.Equal(t0.Add(2 * time.Hour)) {
		t.Errorf("next due = %s", v.Jobs[0].NextDue)
	}
	r, err := runs.Get(ctx, v.Jobs[0].LastRun)
	if err != nil {
		t.Fatal(err)
	}
	if r.Mode != zhenagent.ModeScheduled || r.Status != zhenagent.StatusSucceeded || r.Family != "service-health-sweep" || r.Summary.Check != "pass" {
		t.Errorf("run = %+v", r)
	}
	card := f.cards[r.CardID]
	if card == nil || card["type"] != zhenagent.ScheduledCardType || card["owner"] != zhenagent.ScheduledCardOwner || card["status"] != "done" {
		t.Errorf("card %s = %+v", r.CardID, card)
	}
}

// A failing runbook (non-zero exit: a step or the verification section
// failed) and a dispatch error are failed runs with last_error; neither
// blocks the next job in the same tick.
func TestScheduler_FailingJobDoesNotBlockTheNext(t *testing.T) {
	f := newFakeSchedChampion()
	f.exits["observe/service-health-sweep"] = 1
	f.errs["data/conversation-memory-maintenance"] = errors.New("runbook \"x\": manifest mismatch")
	s, clk, _, runs := newTestScheduler(t, `version: 1
jobs:
  - name: a-sweep
    runbook: observe/service-health-sweep
    every: 1h
  - name: b-memory
    runbook: data/conversation-memory-maintenance
    every: 1h
  - name: c-approved
    runbook: observe/approved-one
    every: 1h
`, f)
	ctx := context.Background()
	clk.set(t0.Add(time.Hour))
	s.tick(ctx)
	s.wait()
	if got := f.executed(); len(got) != 3 {
		t.Fatalf("executed = %v", got)
	}
	byName := map[string]jobView{}
	for _, j := range s.view().Jobs {
		byName[j.Name] = j
	}
	if j := byName["a-sweep"]; j.LastStatus != zhenagent.StatusFailed || !strings.Contains(j.LastError, "exit 1") || !strings.Contains(j.LastError, "verification: 0/1 passed") {
		t.Errorf("a-sweep = %+v", j)
	}
	if j := byName["b-memory"]; j.LastStatus != zhenagent.StatusFailed || !strings.Contains(j.LastError, "manifest mismatch") {
		t.Errorf("b-memory = %+v", j)
	}
	if j := byName["c-approved"]; j.LastStatus != zhenagent.StatusSucceeded {
		t.Errorf("c-approved = %+v", j)
	}
	r, _ := runs.Get(ctx, byName["a-sweep"].LastRun)
	if r.Status != zhenagent.StatusFailed || r.Summary.Check != "fail" || !strings.Contains(r.Error, "exit 1") {
		t.Errorf("failed run row = %+v", r)
	}
	if c := f.cards[r.CardID]; c["status"] != "review" {
		t.Errorf("failed run's card = %+v (want review)", c)
	}

	// The error stays until the job next succeeds.
	f.mu.Lock()
	f.exits["observe/service-health-sweep"] = 0
	f.mu.Unlock()
	clk.set(t0.Add(2 * time.Hour))
	s.tick(ctx)
	s.wait()
	for _, j := range s.view().Jobs {
		if j.Name == "a-sweep" && (j.LastStatus != zhenagent.StatusSucceeded || j.LastError != "") {
			t.Errorf("after a success: %+v", j)
		}
	}
}

// Only approved runbooks (or ones that carry no status) may be scheduled:
// a draft or stub is refused at load, and a runbook that went back to
// draft fails its run without executing.
func TestScheduler_OnlyApprovedRunbooks(t *testing.T) {
	f := newFakeSchedChampion()
	for _, rb := range []string{"security/mcp-write-exec-enablement", "network/air-gap-egress-validation", "wotan/active-active-cutover-unparsable", "observe/no-such"} {
		_, err := newScheduler(context.Background(), schedulerConfig{
			Champion: f, Path: writeSchedules(t, "version: 1\njobs:\n  - name: x\n    runbook: "+rb+"\n    every: 1h\n"),
			Runs: zhenagent.NewMemStore(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
		if err == nil {
			t.Errorf("%s: schedule accepted", rb)
		}
	}

	s, clk, _, _ := newTestScheduler(t, oneJob, f)
	f.mu.Lock()
	f.runbooks["observe/service-health-sweep"] = "metadata:\n  name: service-health-sweep\n  status: draft\n  approved_by: \"\"\n"
	f.mu.Unlock()
	clk.set(t0.Add(time.Hour))
	s.tick(context.Background())
	s.wait()
	if got := f.executed(); len(got) != 0 {
		t.Fatalf("a draft runbook ran: %v", got)
	}
	if j := s.view().Jobs[0]; j.LastStatus != zhenagent.StatusFailed || !strings.Contains(j.LastError, "draft") {
		t.Errorf("job = %+v", j)
	}
}

// Every tick beats: the gauge and zhen.scheduler.heartbeat, paused or not.
func TestScheduler_HeartbeatEveryTick(t *testing.T) {
	f := newFakeSchedChampion()
	s, clk, pub, _ := newTestScheduler(t, oneJob, f)
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		clk.set(t0.Add(time.Duration(i) * time.Minute))
		s.tick(ctx)
	}
	s.wait()
	if n := pub.count(topicSchedulerHeartbeat); n != 3 {
		t.Errorf("heartbeats published = %d, want 3", n)
	}
	line := grepMetric(scrape(t), "zhen_scheduler_heartbeat_timestamp_seconds ")
	var got float64
	if _, err := fmt.Sscanf(strings.TrimPrefix(line, "zhen_scheduler_heartbeat_timestamp_seconds "), "%g", &got); err != nil || int64(got) != t0.Add(3*time.Minute).Unix() {
		t.Errorf("gauge %q, want the last tick %d (%v)", line, t0.Add(3*time.Minute).Unix(), err)
	}

	if _, err := s.UpdateSchedule(ctx, "pause", map[string]any{"paused": true}); err != nil {
		t.Fatal(err)
	}
	clk.set(t0.Add(2 * time.Hour))
	s.tick(ctx)
	s.wait()
	if got := f.executed(); len(got) != 0 {
		t.Errorf("paused scheduler ran %v", got)
	}
	if n := pub.count(topicSchedulerHeartbeat); n != 4 {
		t.Errorf("a paused scheduler stopped beating: %d", n)
	}
	if _, err := s.UpdateSchedule(ctx, "pause", map[string]any{"paused": false}); err != nil {
		t.Fatal(err)
	}
	s.tick(ctx)
	s.wait()
	if got := f.executed(); len(got) != 1 {
		t.Errorf("resumed scheduler ran %v", got)
	}
}

func scrape(t *testing.T) string {
	t.Helper()
	rec := httptest.NewRecorder()
	prom.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec.Body.String()
}

func grepMetric(body, prefix string) string {
	var out []string
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// add, remove and pause change the live schedule and survive a restart
// through the state file; the config file itself is never written.
func TestScheduler_UpdateAndPersist(t *testing.T) {
	f := newFakeSchedChampion()
	cfg := writeSchedules(t, oneJob)
	state := filepath.Join(t.TempDir(), "state.json")
	mk := func() *scheduler {
		s, err := newScheduler(context.Background(), schedulerConfig{Champion: f, Path: cfg, StatePath: state, Runs: zhenagent.NewMemStore(),
			Now: func() time.Time { return t0 }, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := mk()
	ctx := context.Background()
	// A bare name resolves to its one runbook.
	if _, err := s.UpdateSchedule(ctx, "add", map[string]any{"name": "approved-one", "every": "daily@04:00"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := s.UpdateSchedule(ctx, "add", map[string]any{"name": "x", "runbook": "security/mcp-write-exec-enablement", "every": "1h"}); err == nil || !strings.Contains(err.Error(), "draft") {
		t.Errorf("draft runbook added: %v", err)
	}
	if _, err := s.UpdateSchedule(ctx, "add", map[string]any{"name": "y", "runbook": "observe/approved-one", "every": "1s"}); err == nil {
		t.Error("bad interval added")
	}
	if _, err := s.UpdateSchedule(ctx, "remove", map[string]any{"name": "service-health-sweep"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := s.UpdateSchedule(ctx, "remove", map[string]any{"name": "nope"}); err == nil {
		t.Error("removing an unknown job succeeded")
	}
	if _, err := s.UpdateSchedule(ctx, "frobnicate", nil); err == nil {
		t.Error("unknown op accepted")
	}
	before, _ := os.ReadFile(cfg)
	if string(before) != oneJob {
		t.Error("the config file was rewritten")
	}

	again := mk().view()
	if len(again.Jobs) != 1 || again.Jobs[0].Name != "approved-one" || again.Jobs[0].Runbook != "observe/approved-one" || again.Jobs[0].Source != "added" {
		t.Fatalf("after restart = %+v", again)
	}
	if fi, err := os.Stat(state); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("state file: %v %v", fi, err)
	}
}

// failingRuns is a Well that refuses every insert.
type failingRuns struct{ *zhenagent.MemStore }

func (failingRuns) BeginScheduled(context.Context, zhenagent.Run) error {
	return errors.New("the Well is unreachable")
}

// A run the Well cannot record is still run and kept in memory.
func TestScheduler_WellDownKeepsTheRunInMemory(t *testing.T) {
	f := newFakeSchedChampion()
	clk := &clock{t: t0}
	s, err := newScheduler(context.Background(), schedulerConfig{Champion: f, Path: writeSchedules(t, oneJob),
		Runs: failingRuns{zhenagent.NewMemStore()}, Now: clk.now, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	clk.set(t0.Add(time.Hour))
	s.tick(context.Background())
	s.wait()
	j := s.view().Jobs[0]
	if j.LastStatus != zhenagent.StatusSucceeded {
		t.Fatalf("job = %+v", j)
	}
	if r, err := s.mem.Get(context.Background(), j.LastRun); err != nil || r.Status != zhenagent.StatusSucceeded {
		t.Errorf("memory run = %+v %v", r, err)
	}
	if !strings.Contains(scrape(t), "zhen_scheduler_record_failures_total") {
		t.Error("no record-failure counter")
	}
}

// The HTTP surface: add/remove/pause go through the Champion gate as
// schedule_update; list reads the schedule; without -scheduler they 503.
func TestSchedulerEndpoints(t *testing.T) {
	srv, root := toolExecServer(t, nil)
	if err := os.MkdirAll(filepath.Join(root, "runbooks", "observe"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "runbooks", "observe", "service-health-sweep.yaml"), []byte("metadata:\n  name: service-health-sweep\nsteps: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	post := func(path string, body map[string]any) (int, map[string]any) {
		b, _ := json.Marshal(body)
		rec := httptest.NewRecorder()
		srv.schedulerMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b)))
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	if code, _ := post("/api/v1/scheduler/list", nil); code != http.StatusServiceUnavailable {
		t.Errorf("list without -scheduler: %d", code)
	}
	if code, _ := post("/api/v1/scheduler/add", map[string]any{"name": "service-health-sweep", "every": "1h", "justification": directUserRefs()}); code != http.StatusServiceUnavailable {
		t.Errorf("add without -scheduler: %d", code)
	}

	champ, err := srv.pool.get(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := newScheduler(context.Background(), schedulerConfig{Champion: champ, Path: writeSchedules(t, "version: 1\njobs: []\n"),
		Runs: zhenagent.NewMemStore(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	srv.attachScheduler(s)

	if code, out := post("/api/v1/scheduler/add", map[string]any{"name": "service-health-sweep", "every": "1h"}); code != http.StatusBadRequest || out["status"] != "denied" {
		t.Errorf("add without justification: %d %v", code, out)
	}
	if code, out := post("/api/v1/scheduler/add", map[string]any{"name": "service-health-sweep", "every": "1h", "emitted_by": "zhen-agent", "justification": directUserRefs()}); code == http.StatusOK {
		t.Errorf("an agent loop added a schedule: %d %v", code, out)
	}
	if len(s.view().Jobs) != 0 {
		t.Fatal("a refused add changed the schedule")
	}
	if code, out := post("/api/v1/scheduler/add", map[string]any{"name": "service-health-sweep", "every": "1h", "emitted_by": "zhen-web-ui", "justification": directUserRefs()}); code != http.StatusOK || out["status"] != "ok" {
		t.Fatalf("direct-user add: %d %v", code, out)
	}
	code, out := post("/api/v1/scheduler/list", nil)
	if code != http.StatusOK {
		t.Fatalf("list: %d %v", code, out)
	}
	jobs, _ := out["jobs"].([]any)
	if len(jobs) != 1 || jobs[0].(map[string]any)["runbook"] != "observe/service-health-sweep" {
		t.Fatalf("list = %v", out)
	}
	if code, out := post("/api/v1/scheduler/pause", map[string]any{"paused": true, "justification": directUserRefs()}); code != http.StatusOK || !s.view().Paused {
		t.Errorf("pause: %d %v", code, out)
	}
	if code, out := post("/api/v1/scheduler/remove", map[string]any{"name": "service-health-sweep", "justification": directUserRefs()}); code != http.StatusOK || len(s.view().Jobs) != 0 {
		t.Errorf("remove: %d %v", code, out)
	}
	if code, _ := post("/api/v1/scheduler/add", map[string]any{"name": "x", "every": "1h", "command": "id", "justification": directUserRefs()}); code != http.StatusBadRequest {
		t.Errorf("an unknown field: %d", code)
	}
}

// The board's writer (app_kanban) may be named apart from the audit DSN:
// app_zhen cannot INSERT kanban_tasks, and a scheduled run's row needs
// its card (foreign key).
func TestKanbanDSN_PrefersTheBoardsWriter(t *testing.T) {
	t.Setenv("WELL_DSN", "host=a user=app_zhen")
	t.Setenv("WELL_KANBAN_DSN", "")
	if got := kanbanDSN(); got != "host=a user=app_zhen" {
		t.Errorf("fallback = %q", got)
	}
	t.Setenv("WELL_KANBAN_DSN", " host=a user=app_kanban ")
	if got := kanbanDSN(); got != "host=a user=app_kanban" {
		t.Errorf("WELL_KANBAN_DSN = %q", got)
	}
}

// The shipped configs/zhen-schedules.yaml loads against the repository's
// runbooks: every job names a runbook that exists and may be scheduled.
func TestShippedSchedules(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	champ, err := champion.New(champion.Config{ProjectRoot: repo}, nopActionStore{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := newScheduler(context.Background(), schedulerConfig{Champion: champ, Path: filepath.Join(repo, "configs", "zhen-schedules.yaml"),
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatalf("configs/zhen-schedules.yaml: %v", err)
	}
	if len(s.view().Jobs) == 0 {
		t.Error("no jobs")
	}
}
