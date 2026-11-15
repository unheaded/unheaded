// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// kanban-app's one home is the Well (ADR-147 Phase 16). Without it the
// board waits, read-only, and says so: the task routes answer 503 {"error":
// "the Well is unavailable"}, /health says "well":"waiting", and the Well is
// retried every 1 s, doubling to 30 s. The SQLite fallback of 2026-10-28
// is gone: it split the board into a second copy whenever the Well was
// slow at start.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"unheaded/pkg/database"
	"unheaded/pkg/httputil"
)

// errWellUnavailable is every store call while the Well is not connected.
var errWellUnavailable = errors.New("the Well is unavailable")

// wellNotMigrated is a connect error retrying cannot fix: the Well answers
// but lacks kanban's migrations. main refuses to start on it.
type wellNotMigrated struct{ err error }

func (e wellNotMigrated) Error() string {
	return "the Well is not migrated for kanban-app: " + e.err.Error()
}
func (e wellNotMigrated) Unwrap() error { return e.err }

// wellWait is the retry's first wait and its cap.
var wellWait = [2]time.Duration{time.Second, 30 * time.Second}

// connectTaskStore opens the Well as app_kanban. Tests replace it.
var connectTaskStore = func(ctx context.Context) (TaskStore, error) {
	db, err := database.Connect(ctx, database.AppKanbanConfig())
	st, serr := wellStore(db, err)
	switch {
	case serr != nil:
		return nil, wellNotMigrated{serr}
	case st == nil && err == nil:
		return nil, errors.New("no store")
	case st == nil:
		return nil, err
	}
	return st, nil
}

// waitForWell calls connect until it returns a store, waiting minWait after
// the first failure and doubling to maxWait. A not-migrated Well, or the
// end of ctx, returns the error.
func waitForWell(ctx context.Context, connect func(context.Context) (TaskStore, error), minWait, maxWait time.Duration) (TaskStore, error) {
	wait := minWait
	for {
		st, err := connect(ctx)
		if err == nil {
			return st, nil
		}
		var nm wellNotMigrated
		if errors.As(err, &nm) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for the Well: %w", ctx.Err())
		case <-time.After(wait):
		}
		wait = min(wait*2, maxWait)
	}
}

// wellGate is the TaskStore the server and the TaskManager hold from the
// start. Until the Well arrives every call answers errWellUnavailable.
type wellGate struct {
	mu      sync.RWMutex
	st      TaskStore
	onReady []func()
}

func newWellGate() *wellGate { return &wellGate{} }

func (g *wellGate) store() TaskStore {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.st
}

// ready reports whether the Well is connected.
func (g *wellGate) ready() bool { return g.store() != nil }

// state is /health's well detail.
func (g *wellGate) state() string {
	if g.ready() {
		return "connected"
	}
	return "waiting"
}

// attach connects the Well and runs the OnReady callbacks.
func (g *wellGate) attach(st TaskStore) {
	g.mu.Lock()
	g.st = st
	fns := g.onReady
	g.onReady = nil
	g.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// OnReady runs fn once the Well is connected (now, if it is).
func (g *wellGate) OnReady(fn func()) {
	g.mu.Lock()
	if g.st == nil {
		g.onReady = append(g.onReady, fn)
		g.mu.Unlock()
		return
	}
	g.mu.Unlock()
	fn()
}

func (g *wellGate) GetAllTasks() ([]Task, error) {
	if st := g.store(); st != nil {
		return st.GetAllTasks()
	}
	return nil, errWellUnavailable
}

func (g *wellGate) GetTask(id string) (*Task, error) {
	if st := g.store(); st != nil {
		return st.GetTask(id)
	}
	return nil, errWellUnavailable
}

func (g *wellGate) SaveTask(task *Task) error {
	if st := g.store(); st != nil {
		return st.SaveTask(task)
	}
	return errWellUnavailable
}

func (g *wellGate) DeleteTask(id string) error {
	if st := g.store(); st != nil {
		return st.DeleteTask(id)
	}
	return errWellUnavailable
}

func (g *wellGate) TaskCount() (int, error) {
	if st := g.store(); st != nil {
		return st.TaskCount()
	}
	return 0, errWellUnavailable
}

func (g *wellGate) Close() error {
	if st := g.store(); st != nil {
		return st.Close()
	}
	return nil
}

// openWell gives the server's gate the Well: once now, and while it does
// not answer, in the background until it does. A Well that answers but is
// not migrated stops the process, as before (no fallback would split the
// board).
func openWell(ctx context.Context, g *wellGate) {
	st, err := connectTaskStore(ctx)
	if err == nil {
		g.attach(st)
		log.Info().Msg("Postgres connected — kanban tasks persisted to The Well")
		return
	}
	var nm wellNotMigrated
	if errors.As(err, &nm) {
		log.Fatal().Err(err).Msg("The Well answers but is not migrated for kanban-app: refusing to start (run well-migrate; no fallback, it would split the board)")
	}
	log.Warn().Err(err).Msg("the Well is not answering: the board waits, read-only, and retries")
	go func() {
		st, err := waitForWell(ctx, connectTaskStore, wellWait[0], wellWait[1])
		if err != nil {
			if errors.As(err, &nm) {
				log.Fatal().Err(err).Msg("The Well answers but is not migrated for kanban-app: refusing to start")
			}
			return // ctx ended
		}
		g.attach(st)
		log.Info().Msg("the Well answered: kanban tasks persisted to The Well")
	}()
}

// refuseWhileWaiting answers 503 {"error":"the Well is unavailable"} while
// the Well is not connected.
func (s *Server) refuseWhileWaiting(w http.ResponseWriter) bool {
	if s.well == nil || s.well.ready() {
		return false
	}
	httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errWellUnavailable.Error()})
	return true
}

// wellState is /health's well detail: "connected", "waiting", or "" for a
// server built without the Well (tests with their own store).
func (s *Server) wellState() string {
	if s.well == nil {
		return ""
	}
	return s.well.state()
}
