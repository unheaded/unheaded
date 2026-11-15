// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Policy (ADR-147 Phase 16): kanban-app has one home, the Well. No
// non-test file imports the SQLite driver or names kanban.db: without the
// Well the board waits, read-only, instead of splitting into a second copy.
func TestNoSQLiteFallback(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) // #nosec G304 -- this package's own sources
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "kanban.db") {
			t.Errorf("%s names kanban.db", f)
		}
		pf, err := parser.ParseFile(token.NewFileSet(), f, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range pf.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == "modernc.org/sqlite" {
				t.Errorf("%s imports modernc.org/sqlite", f)
			}
		}
	}
}

// A Well that answers late: before it, the task routes answer 503
// {"error":"the Well is unavailable"} and /health says "well":"waiting";
// after it, 200 and "connected".
func TestWellLateArrival(t *testing.T) {
	var mu sync.Mutex
	tries := 0
	release := make(chan struct{})
	mem := &stubStore{}
	connect := func(ctx context.Context) (TaskStore, error) {
		mu.Lock()
		tries++
		n := tries
		mu.Unlock()
		if n <= 2 {
			return nil, errors.New("connection refused")
		}
		<-release
		return mem, nil
	}
	prev := connectTaskStore
	connectTaskStore = connect
	defer func() { connectTaskStore = prev }()
	prevWait := wellWait
	wellWait = [2]time.Duration{time.Millisecond, 4 * time.Millisecond}
	defer func() { wellWait = prevWait }()

	s := NewServer(Config{Port: "0"})
	defer s.cancel()

	get := func(path string) (int, map[string]any) {
		w := httptest.NewRecorder()
		switch path {
		case "/health":
			s.handleHealth(w, httptest.NewRequest(http.MethodGet, path, nil))
		default:
			s.handleTasks(w, httptest.NewRequest(http.MethodGet, path, nil))
		}
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	if code, body := get("/api/v1/tasks"); code != http.StatusServiceUnavailable || body["error"] != "the Well is unavailable" {
		t.Errorf("tasks before the Well: %d %v, want 503 the Well is unavailable", code, body)
	}
	w := httptest.NewRecorder()
	s.handleTasks(w, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"title":"x"}`)))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("create before the Well: %d, want 503", w.Code)
	}
	if _, body := get("/health"); body["well"] != "waiting" {
		t.Errorf("/health before the Well: well = %v, want waiting", body["well"])
	}

	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for s.well.state() != "connected" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if code, _ := get("/api/v1/tasks"); code != http.StatusOK {
		t.Errorf("tasks after the Well: %d, want 200", code)
	}
	if _, body := get("/health"); body["well"] != "connected" {
		t.Errorf("/health after the Well: well = %v, want connected", body["well"])
	}
	mu.Lock()
	defer mu.Unlock()
	if tries < 3 {
		t.Errorf("connected after %d tries, want 3", tries)
	}
}

// The refusal for a Well that answers but is not migrated for kanban-app
// is unchanged: it is not retried (retrying cannot fix it), and main stops.
func TestWellNotMigratedStillFatal(t *testing.T) {
	calls := 0
	notMigrated := wellNotMigrated{errors.New("kanban_tasks: migration 003 not applied")}
	_, err := waitForWell(context.Background(), func(context.Context) (TaskStore, error) {
		calls++
		return nil, notMigrated
	}, time.Millisecond, time.Millisecond)
	var nm wellNotMigrated
	if !errors.As(err, &nm) || calls != 1 {
		t.Errorf("waitForWell = %v after %d calls, want the not-migrated error after 1", err, calls)
	}
	// wellStore still says so for a Well without kanban's tables.
	if _, err := wellStore(nil, nil); err != nil {
		t.Errorf("wellStore(nil, nil) = %v, want no store and no error (not connected)", err)
	}
}
