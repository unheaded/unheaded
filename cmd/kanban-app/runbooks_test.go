// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/wotan-client/mock"
)

const sweepRunbook = `apiVersion: runbook/v1
kind: Runbook
metadata:
  name: service-health-sweep
  description: Health check every Kingdom HTTP service
  risk: low
  status: approved
  estimated_duration: 2m
steps:
  - name: one
    command: "true"
  - name: two
    command: "true"
  - name: three
    command: "true"
`

const draftRunbook = `apiVersion: runbook/v1
kind: Runbook
metadata:
  name: cutover
  risk: high
  status: draft
steps:
  - name: only
    action: manual
`

// runbooksDir writes runbooks/ with a manifest: observe/sweep matches it,
// wotan/cutover was edited after the manifest was written, and a file the
// manifest does not list.
func runbooksDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	write("observe/sweep.yaml", sweepRunbook)
	write("wotan/cutover.yaml", draftRunbook)
	write("infra/unlisted.yaml", sweepRunbook)
	write(runbookManifestFile, sum(sweepRunbook)+"  observe/sweep.yaml\n"+
		sum("an older body")+"  wotan/cutover.yaml\n"+
		"# a comment\n\n")
	return dir
}

func TestRunbookCatalog_ListsTheManifest(t *testing.T) {
	list, err := newRunbookCatalog(runbooksDir(t)).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d runbooks, want 2 (only manifest entries): %+v", len(list), list)
	}
	sweep, cut := list[0], list[1]
	if sweep.Ref != "observe/sweep" || sweep.Category != "observe" || sweep.Name != "sweep" || sweep.Steps != 3 ||
		sweep.Status != "approved" || sweep.Risk != "low" || sweep.EstimatedDuration != "2m" || !sweep.ManifestOK {
		t.Errorf("sweep = %+v", sweep)
	}
	if cut.Ref != "wotan/cutover" || cut.ManifestOK || cut.Status != "draft" || cut.Steps != 1 {
		t.Errorf("cutover = %+v, want manifest_ok false (edited after the manifest)", cut)
	}
}

func TestRunbookCatalog_RefusesEscapes(t *testing.T) {
	dir := runbooksDir(t)
	outside := filepath.Join(t.TempDir(), "evil.yaml")
	if err := os.WriteFile(outside, []byte(sweepRunbook), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "observe", "linked.yaml")); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(sweepRunbook))
	m := hex.EncodeToString(h[:]) + "  observe/sweep.yaml\n" + hex.EncodeToString(h[:]) + "  observe/linked.yaml\n" +
		hex.EncodeToString(h[:]) + "  Bad/Name.yaml\n"
	if err := os.WriteFile(filepath.Join(dir, runbookManifestFile), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := newRunbookCatalog(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Ref != "observe/sweep" {
		t.Errorf("got %+v; want only observe/sweep (a symlink out and a bad name are not listed)", list)
	}

	for _, bad := range []string{"../x.yaml", "/etc/passwd", "observe/../../x.yaml", "zz  a.yaml"} {
		line := hex.EncodeToString(h[:]) + "  " + bad + "\n"
		if bad == "zz  a.yaml" {
			line = bad + "\n"
		}
		if _, err := parseManifest([]byte(line)); err == nil {
			t.Errorf("manifest line %q accepted", line)
		}
	}
	if _, err := newRunbookCatalog(filepath.Join(dir, "nope")).List(); err == nil {
		t.Error("a missing runbooks/ listed without error")
	}
}

func TestRunbookCatalog_CachesForTTL(t *testing.T) {
	dir := runbooksDir(t)
	c := newRunbookCatalog(dir)
	now := time.Unix(1000, 0)
	c.now = func() time.Time { return now }
	if l, _ := c.List(); len(l) != 2 {
		t.Fatalf("first list: %d", len(l))
	}
	if err := os.Remove(filepath.Join(dir, runbookManifestFile)); err != nil {
		t.Fatal(err)
	}
	if l, err := c.List(); err != nil || len(l) != 2 {
		t.Fatalf("within the TTL the cached list is served: %d %v", len(l), err)
	}
	now = now.Add(runbookCatalogTTL)
	if _, err := c.List(); err == nil {
		t.Fatal("after the TTL the catalog re-reads (and the manifest is gone)")
	}
}

func TestHandleRunbooks(t *testing.T) {
	tm, _ := createTestTaskManager(t)
	s := &Server{runbooks: newRunbookCatalog(runbooksDir(t)), taskManager: tm}
	tm.runbookProgress.Record(RunbookProgress{Runbook: "observe/sweep", Step: 2, Total: 3, Name: "two", State: "running", TS: 5})

	w := httptest.NewRecorder()
	s.handleRunbooks(w, httptest.NewRequest(http.MethodGet, "/api/v1/runbooks", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", w.Code, w.Body.String())
	}
	var got struct {
		Runbooks []RunbookInfo `json:"runbooks"`
		Count    int           `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 || got.Runbooks[0].Progress == nil || got.Runbooks[0].Progress.Step != 2 || got.Runbooks[1].Progress != nil {
		t.Errorf("body = %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	s.handleRunbooks(w, httptest.NewRequest(http.MethodPost, "/api/v1/runbooks", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", w.Code)
	}
	for _, s := range []*Server{{}, {runbooks: newRunbookCatalog(filepath.Join(t.TempDir(), "none"))}} {
		w = httptest.NewRecorder()
		s.handleRunbooks(w, httptest.NewRequest(http.MethodGet, "/api/v1/runbooks", nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("no runbooks: %d, want 503", w.Code)
		}
	}
}

func postFromRunbook(s *Server, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.handleCreateFromRunbook(w, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/from-runbook", strings.NewReader(body)))
	return w
}

func TestCreateFromRunbook_WritesThroughTheStore(t *testing.T) {
	store := newTestSQLiteStore(t)
	s := &Server{store: store, runbooks: newRunbookCatalog(runbooksDir(t)), sseClients: map[chan []byte]bool{}}

	w := postFromRunbook(s, `{"runbook":"observe/sweep","owner":"stevie"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct{ Task Task }
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	c := created.Task
	if !strings.HasPrefix(c.ID, "rb-observe-sweep-") || c.Type != RunbookCardType || c.Status != "todo" ||
		c.Owner != "stevie" || c.Title != "Runbook: observe/sweep" {
		t.Errorf("card = %+v", c)
	}
	if !strings.Contains(c.Description, "```runbook\nobserve/sweep\n```") || !strings.Contains(c.Description, "risk low, 3 steps, status approved") {
		t.Errorf("description = %q", c.Description)
	}
	stored, err := store.GetTask(c.ID)
	if err != nil {
		t.Fatalf("card not in the store: %v", err)
	}
	if runbookRefOf(stored) != "observe/sweep" {
		t.Errorf("stored card's runbook = %q", runbookRefOf(stored))
	}

	// The card is the store's: GET /api/v1/tasks/{id} reads it back with its runbook.
	w = httptest.NewRecorder()
	s.handleTaskByID(w, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+c.ID, nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"runbook":"observe/sweep"`) ||
		!strings.Contains(w.Body.String(), `"id":"`+c.ID+`"`) {
		t.Errorf("GET: %d %s", w.Code, w.Body.String())
	}

	// A second card for the same runbook is a second card.
	if w := postFromRunbook(s, `{"runbook":"observe/sweep"}`); w.Code != http.StatusCreated {
		t.Errorf("second create: %d", w.Code)
	}
	if n, _ := store.TaskCount(); n != 2 {
		t.Errorf("store has %d cards, want 2", n)
	}
}

func TestCreateFromRunbook_Refusals(t *testing.T) {
	s := &Server{store: newTestSQLiteStore(t), runbooks: newRunbookCatalog(runbooksDir(t))}
	for body, want := range map[string]int{
		`{"runbook":"observe/nope"}`:                 http.StatusNotFound,
		`{"runbook":"infra/unlisted"}`:               http.StatusNotFound, // on disk, not in the manifest
		`{"runbook":"../etc/passwd"}`:                http.StatusBadRequest,
		`{"runbook":"observe/sweep","x":1}`:          http.StatusBadRequest,
		`{"runbook":"observe/sweep","owner":"a\nb"}`: http.StatusBadRequest,
		`not json`: http.StatusBadRequest,
		`{"runbook":"` + strings.Repeat("a", 8<<10) + `"}`: http.StatusBadRequest,
	} {
		if w := postFromRunbook(s, body); w.Code != want {
			t.Errorf("%.60s: %d, want %d", body, w.Code, want)
		}
	}
	w := httptest.NewRecorder()
	s.handleCreateFromRunbook(w, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/from-runbook", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d", w.Code)
	}
	// No database: read-only, nothing invented in memory.
	ro := &Server{runbooks: newRunbookCatalog(runbooksDir(t))}
	if w := postFromRunbook(ro, `{"runbook":"observe/sweep"}`); w.Code != http.StatusServiceUnavailable || len(ro.tasks) != 0 {
		t.Errorf("read-only: %d, %d tasks", w.Code, len(ro.tasks))
	}
}

func TestCreateFromRunbook_ThroughTheTaskManager(t *testing.T) {
	store := newTestSQLiteStore(t)
	tm, err := NewTaskManager(mock.NewMockClient(mock.WithAutoApprove()), func(string, interface{}) {}, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tm.Close() })
	s := &Server{taskManager: tm, store: store, runbooks: newRunbookCatalog(runbooksDir(t))}
	w := postFromRunbook(s, `{"runbook":"wotan/cutover"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if n, _ := store.TaskCount(); n != 1 {
		t.Errorf("store has %d cards, want 1", n)
	}
}

// The mux sends /api/v1/tasks/from-runbook to its own handler, not to
// /api/v1/tasks/{id}.
func TestRunbookRoutes_FromRunbookIsNotATaskID(t *testing.T) {
	s := &Server{store: newTestSQLiteStore(t), runbooks: newRunbookCatalog(runbooksDir(t))}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/tasks/", s.handleTaskByID)
	s.registerRunbookRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/v1/tasks/from-runbook", "application/json", bytes.NewBufferString(`{"runbook":"observe/sweep"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("POST from-runbook through the mux: %d", resp.StatusCode)
	}
	resp, err = http.Get(srv.URL + "/api/v1/runbooks")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET runbooks through the mux: %d", resp.StatusCode)
	}
}

func TestRunbookRefOf(t *testing.T) {
	for _, tc := range []struct {
		task Task
		want string
	}{
		{Task{Type: "runbook", Description: "x\n\n```runbook\nobserve/sweep\n```"}, "observe/sweep"},
		{Task{Type: "task", Description: "```runbook\nobserve/sweep\n```"}, ""},
		{Task{Type: "runbook", Description: "```runbook\n../etc\n```"}, ""},
		{Task{Type: "runbook", Description: "no block"}, ""},
	} {
		if got := runbookRefOf(&tc.task); got != tc.want {
			t.Errorf("%q: %q, want %q", tc.task.Description, got, tc.want)
		}
	}
}

// fakeRunStore is a SQLite store that also answers agent-run lookups, the
// way PgStore does over zhen_agent_runs.
type fakeRunStore struct {
	*Store
	run *AgentRun
	err error
}

func (f fakeRunStore) LatestAgentRun(_ context.Context, cardID string) (*AgentRun, error) {
	if f.err != nil || f.run == nil || cardID != "card-1" {
		return nil, f.err
	}
	r := *f.run
	return &r, nil
}

func TestGetTask_CarriesTheAgentRun(t *testing.T) {
	base := newTestSQLiteStore(t)
	if err := base.SaveTask(&Task{ID: "card-1", Title: "Fix it", Status: "review", Type: "task", Owner: "zhen"}); err != nil {
		t.Fatal(err)
	}
	if err := base.SaveTask(&Task{ID: "card-2", Title: "Other", Status: "todo"}); err != nil {
		t.Fatal(err)
	}
	reviewed := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	run := &AgentRun{RunID: "00000000000000a1", Status: "rejected", Mode: "overnight", Family: "repo-task",
		Branch: "zhen/card-1", HeadCommit: strings.Repeat("b", 40), ReviewNote: "/var/lib/zhen/zhen-reviews/card-1.md",
		Check: "pass", ReviewReason: "wrong file", ClaimedAt: reviewed.Add(-time.Hour), ReviewedAt: &reviewed}

	get := func(s *Server, id string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		s.handleTaskByID(w, httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+id, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", id, w.Code, w.Body.String())
		}
		var m map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	s := &Server{store: fakeRunStore{Store: base, run: run}}
	m := get(s, "card-1")
	ar, ok := m["agent_run"].(map[string]any)
	if !ok {
		t.Fatalf("no agent_run: %v", m)
	}
	if ar["branch"] != "zhen/card-1" || ar["review_note"] != run.ReviewNote || ar["review_reason"] != "wrong file" ||
		ar["mode"] != "overnight" || ar["status"] != "rejected" || ar["run_id"] != run.RunID {
		t.Errorf("agent_run = %v", ar)
	}
	if m["id"] != "card-1" || m["title"] != "Fix it" {
		t.Errorf("card fields are no longer flat: %v", m)
	}
	if _, ok := get(s, "card-2")["agent_run"]; ok {
		t.Error("a card without a run shows one")
	}

	// A scheduled run has no branch; an odd branch is not passed on.
	sched := *run
	sched.Mode, sched.Branch, sched.HeadCommit, sched.ReviewedAt = "scheduled", "", "", nil
	if ar := get(&Server{store: fakeRunStore{Store: base, run: &sched}}, "card-1")["agent_run"].(map[string]any); ar["branch"] != nil || ar["mode"] != "scheduled" {
		t.Errorf("scheduled agent_run = %v", ar)
	}
	odd := *run
	odd.Branch = "main; rm -rf /"
	if ar := get(&Server{store: fakeRunStore{Store: base, run: &odd}}, "card-1")["agent_run"].(map[string]any); ar["branch"] != nil {
		t.Errorf("odd branch passed on: %v", ar)
	}

	// The Well refusing the read (041 not applied) is "no run", not an error.
	if _, ok := get(&Server{store: fakeRunStore{Store: base, err: errors.New("permission denied for table zhen_agent_runs")}}, "card-1")["agent_run"]; ok {
		t.Error("a refused read produced an agent_run")
	}
	// SQLite has no agent runs.
	if _, ok := get(&Server{store: base}, "card-1")["agent_run"]; ok {
		t.Error("SQLite store produced an agent_run")
	}
}

func TestParseRunbookProgress(t *testing.T) {
	good := `{"runbook":"observe/sweep","step":2,"total":3,"name":"two","state":"running","dry_run":false,"ts":1}`
	if p, err := parseRunbookProgress([]byte(good)); err != nil || p.Step != 2 || p.Name != "two" {
		t.Fatalf("good event: %+v %v", p, err)
	}
	for _, bad := range []string{
		`{"runbook":"../x","step":1,"total":1,"state":"ok","ts":1}`,
		`{"runbook":"a/b","step":1,"total":1,"state":"exploded","ts":1}`,
		`{"runbook":"a/b","step":2,"total":1,"state":"ok","ts":1}`,
		`{"runbook":"a/b","step":-1,"total":1,"state":"ok","ts":1}`,
		`{"runbook":"a/b","step":1,"total":1,"state":"ok"}`,
		`not json`,
		`{"runbook":"a/b","step":1,"total":1,"state":"ok","ts":1,"name":"` + strings.Repeat("x", 5000) + `"}`,
	} {
		if _, err := parseRunbookProgress([]byte(bad)); err == nil {
			t.Errorf("accepted %.80s", bad)
		}
	}
	p, err := parseRunbookProgress([]byte(`{"runbook":"a/b","step":1,"total":1,"state":"ok","ts":1,"name":"<b>` + strings.Repeat("y", 300) + `\u0007"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(p.Name)) > maxProgressName || strings.ContainsRune(p.Name, '\x07') {
		t.Errorf("name not clipped: %q", p.Name)
	}
}

func TestRunbookProgressTracker(t *testing.T) {
	tr := newRunbookProgressTracker()
	if !tr.Record(RunbookProgress{Runbook: "a/b", Step: 2, Total: 3, State: "running", TS: 10}) {
		t.Fatal("first event refused")
	}
	if tr.Record(RunbookProgress{Runbook: "a/b", Step: 1, Total: 3, State: "running", TS: 9}) {
		t.Error("an older event replaced a newer one")
	}
	if p, _ := tr.Get("a/b"); p.Step != 2 {
		t.Errorf("held %+v", p)
	}
	for i := 0; i < maxTrackedRunbooks+5; i++ {
		tr.Record(RunbookProgress{Runbook: fmt.Sprintf("c/r%d", i), State: "ok", TS: 1})
	}
	if n := len(tr.latest); n != maxTrackedRunbooks {
		t.Errorf("tracker holds %d runbooks, cap %d", n, maxTrackedRunbooks)
	}
	var nilTracker *runbookProgressTracker
	if _, ok := nilTracker.Get("a/b"); ok {
		t.Error("nil tracker answered")
	}
}

// End to end on the bus: an event on runbooks.progress reaches open boards
// as SSE runbook.progress and the Runbooks view.
func TestTaskManager_StreamsRunbookProgress(t *testing.T) {
	client := mock.NewMockClient(mock.WithAutoApprove())
	var mu sync.Mutex
	var got []RunbookProgress
	tm, err := NewTaskManager(client, func(kind string, data interface{}) {
		if kind == "runbook.progress" {
			mu.Lock()
			got = append(got, data.(RunbookProgress))
			mu.Unlock()
		}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tm.Close() })
	if err := tm.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !client.IsApproved(TopicRunbooksProgress) {
		t.Fatal("the board did not join runbooks.progress")
	}
	// The stream goroutine joins the mock's channel asynchronously, and the
	// mock drops a message injected before that: inject until one lands.
	deadline := time.Now().Add(5 * time.Second)
	for {
		client.InjectMessage(TopicRunbooksProgress, `{"runbook":"bogus","step":1,"total":1,"state":"ok","ts":1}`)
		client.InjectMessage(TopicRunbooksProgress, `{"runbook":"observe/sweep","step":3,"total":3,"name":"three","state":"ok","dry_run":false,"ts":7}`)
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("no runbook.progress broadcast")
	}
	for _, p := range got {
		if p.Runbook != "observe/sweep" || p.Step != 3 {
			t.Fatalf("broadcast %+v; want only the valid event", got)
		}
	}
	if p, ok := tm.runbookProgress.Get("observe/sweep"); !ok || p.State != "ok" {
		t.Errorf("tracker: %+v %v", p, ok)
	}
}
