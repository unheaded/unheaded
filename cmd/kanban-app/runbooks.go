// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

// The board sees runbooks and agent runs (S-CHAMPION-HARDENING Phase 12,
// M-128):
//
//   GET  /api/v1/runbooks             the committed runbooks: every entry of
//                                     runbooks/MANIFEST.sha256 with its
//                                     front matter, whether its bytes still
//                                     match the manifest, and the latest
//                                     per-step progress seen on Wotan
//   POST /api/v1/tasks/from-runbook   {"runbook": "category/name"} makes a
//                                     card (type runbook) in the board's
//                                     store, The Well in production, the
//                                     same path as POST /api/v1/tasks
//   GET  /api/v1/tasks/{id}           also carries agent_run (the newest
//                                     zhen_agent_runs row for the card) and,
//                                     for a runbook card, its runbook and
//                                     progress
//
// Cards still come only from the store (memory feedback_kanban_cards_from_well):
// nothing here holds a card. Progress comes from scripts/run-runbook.py on
// Wotan runbooks.progress (RUNBOOK_PROGRESS_WOTAN; publisher runbook-runner);
// the board keeps the latest event per runbook in memory and forwards each
// one to open boards as SSE "runbook.progress".

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	// TopicRunbooksProgress carries run-runbook.py's per-step events.
	TopicRunbooksProgress = "runbooks.progress"
	// RunbookCardType is the type of a card made from a runbook.
	RunbookCardType = "runbook"
	// RunbooksDirEnv names the runbooks/ directory (default "runbooks",
	// relative to the working directory: /app/runbooks in the image).
	RunbooksDirEnv = "KANBAN_RUNBOOKS_DIR"

	runbookManifestFile = "MANIFEST.sha256"
	runbookCatalogTTL   = 30 * time.Second
	maxRunbookBytes     = 256 << 10
	maxManifestBytes    = 1 << 20
	maxRunbookDescRunes = 400
	maxTrackedRunbooks  = 1024
	maxProgressSteps    = 10000
	maxProgressName     = 128
	maxFromRunbookBody  = 4 << 10
	maxOwnerLen         = 64
	agentRunLookupLimit = 2 * time.Second
)

var (
	runbookRefRE   = regexp.MustCompile(`^[a-z0-9-]{1,64}/[a-z0-9-]{1,128}$`)
	runbookBlockRE = regexp.MustCompile("(?m)^```runbook\n([a-z0-9/-]{3,193})\n```")
	agentBranchRE  = regexp.MustCompile(`^zhen/[a-z0-9-]{1,64}$`)
	commitRE       = regexp.MustCompile(`^[0-9a-f]{40}$`)
	progressStates = map[string]bool{"running": true, "ok": true, "failed": true, "manual": true,
		"skipped": true, "stopped": true, "done": true}
)

// ============================================================================
// CATALOG
// ============================================================================

// RunbookInfo is one runbook as the board lists it.
type RunbookInfo struct {
	Ref               string           `json:"ref"` // category/name, what runbook_execute takes
	Name              string           `json:"name"`
	Category          string           `json:"category"`
	Description       string           `json:"description,omitempty"`
	Status            string           `json:"status,omitempty"` // metadata.status: draft, approved, stub, or none
	Risk              string           `json:"risk,omitempty"`
	EstimatedDuration string           `json:"estimated_duration,omitempty"`
	Steps             int              `json:"steps"`
	ManifestOK        bool             `json:"manifest_ok"` // the file's sha256 is the manifest's
	Progress          *RunbookProgress `json:"progress,omitempty"`
}

// runbookCatalog reads runbooks/ through an os.Root (a symlink or a path out
// of the directory is refused) and caches the list for runbookCatalogTTL.
type runbookCatalog struct {
	dir string
	now func() time.Time

	mu     sync.Mutex
	loaded time.Time
	list   []RunbookInfo
	err    error
}

func newRunbookCatalog(dir string) *runbookCatalog {
	return &runbookCatalog{dir: dir, now: time.Now}
}

func newRunbookCatalogFromEnv() *runbookCatalog {
	return newRunbookCatalog(getEnv(RunbooksDirEnv, "runbooks"))
}

// List returns the runbooks, sorted by ref; a copy.
func (c *runbookCatalog) List() ([]RunbookInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded.IsZero() || c.now().Sub(c.loaded) >= runbookCatalogTTL {
		c.list, c.err = loadRunbooks(c.dir)
		c.loaded = c.now()
	}
	if c.err != nil {
		return nil, c.err
	}
	return append([]RunbookInfo(nil), c.list...), nil
}

// Get returns one runbook by ref.
func (c *runbookCatalog) Get(ref string) (RunbookInfo, bool, error) {
	list, err := c.List()
	if err != nil {
		return RunbookInfo{}, false, err
	}
	i := sort.Search(len(list), func(i int) bool { return list[i].Ref >= ref })
	if i < len(list) && list[i].Ref == ref {
		return list[i], true, nil
	}
	return RunbookInfo{}, false, nil
}

type runbookFrontMatter struct {
	Metadata struct {
		Name              string `yaml:"name"`
		Description       string `yaml:"description"`
		Status            string `yaml:"status"`
		Risk              string `yaml:"risk"`
		EstimatedDuration string `yaml:"estimated_duration"`
	} `yaml:"metadata"`
	Steps []yaml.Node `yaml:"steps"`
}

func loadRunbooks(dir string) ([]RunbookInfo, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("runbooks directory: %w", err)
	}
	defer root.Close() // #nosec G307 -- read-only directory handle
	manifest, err := readRootFile(root, runbookManifestFile, maxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("runbooks/%s: %w", runbookManifestFile, err)
	}
	sums, err := parseManifest(manifest)
	if err != nil {
		return nil, err
	}
	out := make([]RunbookInfo, 0, len(sums))
	for file, sum := range sums {
		ref := strings.TrimSuffix(file, ".yaml")
		if !strings.HasSuffix(file, ".yaml") || !runbookRefRE.MatchString(ref) {
			log.Warn().Str("file", file).Msg("runbooks: manifest entry is not category/name.yaml; not listed")
			continue
		}
		body, err := readRootFile(root, file, maxRunbookBytes)
		if err != nil {
			log.Warn().Err(err).Str("runbook", ref).Msg("runbooks: unreadable; not listed")
			continue
		}
		var fm runbookFrontMatter
		if err := yaml.Unmarshal(body, &fm); err != nil {
			log.Warn().Err(err).Str("runbook", ref).Msg("runbooks: unparsable; not listed")
			continue
		}
		got := sha256.Sum256(body)
		category, name, _ := strings.Cut(ref, "/")
		out = append(out, RunbookInfo{
			Ref: ref, Name: name, Category: category,
			Description:       clipText(fm.Metadata.Description, maxRunbookDescRunes),
			Status:            clipText(fm.Metadata.Status, 32),
			Risk:              clipText(fm.Metadata.Risk, 32),
			EstimatedDuration: clipText(fm.Metadata.EstimatedDuration, 32),
			Steps:             len(fm.Steps),
			ManifestOK:        hex.EncodeToString(got[:]) == sum,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out, nil
}

// readRootFile reads one regular file under root, at most limit bytes.
func readRootFile(root *os.Root, name string, limit int64) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close() // #nosec G307 -- read-only file
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", name)
	}
	if st.Size() > limit {
		return nil, fmt.Errorf("%s: %d bytes, limit %d", name, st.Size(), limit)
	}
	return io.ReadAll(io.LimitReader(f, limit))
}

// parseManifest reads sha256sum output ("<hex>  <path>", "<hex> *<path>"),
// paths relative to runbooks/; blank lines and # comments are skipped.
func parseManifest(raw []byte) (map[string]string, error) {
	m := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sum, rest, ok := strings.Cut(line, " ")
		if _, err := hex.DecodeString(sum); !ok || len(sum) != sha256.Size*2 || err != nil {
			return nil, fmt.Errorf("runbooks/%s line %d: want \"<sha256>  <path>\"", runbookManifestFile, n)
		}
		name := strings.TrimPrefix(strings.TrimPrefix(rest, " "), "*")
		if name == "" || path.IsAbs(name) || path.Clean(name) != name || strings.HasPrefix(name, "..") {
			return nil, fmt.Errorf("runbooks/%s line %d: path %q must be clean and relative", runbookManifestFile, n, name)
		}
		m[name] = strings.ToLower(sum)
	}
	return m, sc.Err()
}

// clipText keeps printable text, newlines folded to spaces, at most n runes.
func clipText(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case !unicode.IsPrint(r):
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// ============================================================================
// PROGRESS (Wotan runbooks.progress)
// ============================================================================

// RunbookProgress is one run-runbook.py event.
type RunbookProgress struct {
	Runbook string `json:"runbook"`
	Step    int    `json:"step"`
	Total   int    `json:"total"`
	Name    string `json:"name,omitempty"`
	State   string `json:"state"`
	DryRun  bool   `json:"dry_run"`
	TS      int64  `json:"ts"` // Unix ms, the publisher's clock
}

// parseRunbookProgress checks an event from the bus: any publisher that
// got onto the topic can send one, so everything is validated.
func parseRunbookProgress(payload []byte) (RunbookProgress, error) {
	var p RunbookProgress
	if len(payload) > 4<<10 {
		return p, errors.New("runbooks.progress: event too large")
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return p, fmt.Errorf("runbooks.progress: %w", err)
	}
	switch {
	case !runbookRefRE.MatchString(p.Runbook):
		return p, fmt.Errorf("runbooks.progress: runbook %q is not category/name", p.Runbook)
	case !progressStates[p.State]:
		return p, fmt.Errorf("runbooks.progress: state %q", p.State)
	case p.Total < 0 || p.Total > maxProgressSteps || p.Step < 0 || p.Step > p.Total:
		return p, fmt.Errorf("runbooks.progress: step %d of %d", p.Step, p.Total)
	case p.TS <= 0:
		return p, errors.New("runbooks.progress: ts missing")
	}
	p.Name = clipText(p.Name, maxProgressName)
	return p, nil
}

// runbookProgressTracker keeps the newest event per runbook.
type runbookProgressTracker struct {
	mu     sync.RWMutex
	latest map[string]RunbookProgress
}

func newRunbookProgressTracker() *runbookProgressTracker {
	return &runbookProgressTracker{latest: map[string]RunbookProgress{}}
}

// Record keeps p unless an event for the runbook with a later ts is already
// held, or p would be a new runbook past maxTrackedRunbooks.
func (t *runbookProgressTracker) Record(p RunbookProgress) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cur, ok := t.latest[p.Runbook]; ok {
		if cur.TS > p.TS {
			return false
		}
	} else if len(t.latest) >= maxTrackedRunbooks {
		return false
	}
	t.latest[p.Runbook] = p
	return true
}

// Get returns the newest event for ref.
func (t *runbookProgressTracker) Get(ref string) (*RunbookProgress, bool) {
	if t == nil {
		return nil, false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	p, ok := t.latest[ref]
	if !ok {
		return nil, false
	}
	return &p, true
}

// handleRunbookProgress records a bus event and forwards it to open boards.
func (tm *TaskManager) handleRunbookProgress(payload []byte) error {
	p, err := parseRunbookProgress(payload)
	if err != nil {
		return err
	}
	if tm.runbookProgress.Record(p) {
		tm.broadcast("runbook.progress", p)
	}
	return nil
}

// subscribeToRunbookProgress joins runbooks.progress and streams it for the
// TaskManager's lifetime. Non-fatal: without it the board has no step k/n.
func (tm *TaskManager) subscribeToRunbookProgress(ctx context.Context) error {
	if _, err := tm.client.Subscribe(ctx, TopicRunbooksProgress, "kanban-app"); err != nil {
		return fmt.Errorf("%w: %v", ErrSubscribeFailed, err)
	}
	go tm.streamRunbookProgress(tm.streamCtx)
	return nil
}

func (tm *TaskManager) streamRunbookProgress(ctx context.Context) {
	msgCh, err := tm.client.StreamMessages(ctx, TopicRunbooksProgress)
	if err != nil {
		log.Warn().Err(err).Msg("runbooks.progress stream did not start")
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-msgCh:
			if !ok {
				log.Warn().Msg("runbooks.progress channel closed")
				return
			}
			if msg == nil {
				continue
			}
			if err := tm.handleRunbookProgress([]byte(msg.Payload)); err != nil {
				log.Warn().Err(err).Str("message_id", msg.MessageID).Msg("runbooks.progress event refused")
			}
		}
	}
}

// runbookProgress returns the tracker, nil when the board runs without Wotan.
func (s *Server) runbookProgress() *runbookProgressTracker {
	if s.taskManager == nil {
		return nil
	}
	return s.taskManager.runbookProgress
}

// ============================================================================
// AGENT RUNS (zhen_agent_runs, migrations 032/037/041)
// ============================================================================

// AgentRun is the newest zhen_agent_runs row for a card, as the modal shows it.
type AgentRun struct {
	RunID        string     `json:"run_id"`
	Status       string     `json:"status"`
	Mode         string     `json:"mode"` // interactive, overnight, scheduled
	Family       string     `json:"family"`
	Branch       string     `json:"branch,omitempty"` // zhen/<card>; none for a scheduled run
	HeadCommit   string     `json:"head_commit,omitempty"`
	ReviewNote   string     `json:"review_note,omitempty"` // path of the worker's note on the agentd host
	Check        string     `json:"check,omitempty"`
	ReviewReason string     `json:"review_reason,omitempty"`
	ClaimedAt    time.Time  `json:"claimed_at"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
}

// agentRunSource is a store that can say which agent run worked a card.
// Only the Well has zhen_agent_runs; SQLite and memory stores do not.
type agentRunSource interface {
	LatestAgentRun(ctx context.Context, cardID string) (*AgentRun, error)
}

// LatestAgentRun implements agentRunSource: nil, nil when the card has no
// run. Reads only the columns 041 grants app_kanban.
func (p *PgStore) LatestAgentRun(ctx context.Context, cardID string) (*AgentRun, error) {
	var r AgentRun
	var branch, head, note, check, reason sql.NullString
	var reviewed sql.NullTime
	err := p.db.QueryRowContext(ctx, `
		SELECT id, status, mode, family, branch, head_commit, route_summary->>'review_note',
		       route_summary->>'check', review_reason, claimed_at, reviewed_at
		  FROM zhen_agent_runs WHERE card_id = $1 ORDER BY claimed_at DESC LIMIT 1`, cardID).
		Scan(&r.RunID, &r.Status, &r.Mode, &r.Family, &branch, &head, &note, &check, &reason, &r.ClaimedAt, &reviewed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Branch, r.HeadCommit, r.ReviewNote, r.Check, r.ReviewReason = branch.String, strings.TrimSpace(head.String), note.String, check.String, reason.String
	if reviewed.Valid {
		t := reviewed.Time
		r.ReviewedAt = &t
	}
	return &r, nil
}

// sanitize drops what does not have the shape migrations 032/037 give it:
// the modal renders these as text, but nothing odd should reach it.
func (r *AgentRun) sanitize() {
	if !agentBranchRE.MatchString(r.Branch) {
		r.Branch = ""
	}
	if !commitRE.MatchString(r.HeadCommit) {
		r.HeadCommit = ""
	}
	r.ReviewNote = clipText(r.ReviewNote, 512)
	r.Check = clipText(r.Check, 32)
	r.ReviewReason = clipText(r.ReviewReason, 1000)
	r.Status, r.Mode, r.Family = clipText(r.Status, 32), clipText(r.Mode, 32), clipText(r.Family, 64)
}

// agentRunWarned keeps a refused read (038 not applied yet) to one warning.
var agentRunWarned atomic.Bool

func (s *Server) agentRunFor(ctx context.Context, cardID string) *AgentRun {
	src, ok := s.store.(agentRunSource)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, agentRunLookupLimit)
	defer cancel()
	run, err := src.LatestAgentRun(ctx, cardID)
	if err != nil {
		if agentRunWarned.CompareAndSwap(false, true) {
			log.Warn().Err(err).Msg("zhen_agent_runs not readable (migration 038 applied?): cards show no agent run")
		}
		return nil
	}
	if run != nil {
		run.sanitize()
	}
	return run
}

// ============================================================================
// HANDLERS
// ============================================================================

// registerRunbookRoutes adds the runbook routes to mux.
func (s *Server) registerRunbookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/runbooks", s.handleRunbooks)
	mux.HandleFunc("/api/v1/tasks/from-runbook", s.handleCreateFromRunbook)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}

// handleRunbooks serves GET /api/v1/runbooks.
func (s *Server) handleRunbooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.runbooks == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runbooks not configured"})
		return
	}
	list, err := s.runbooks.List()
	if err != nil {
		log.Warn().Err(err).Msg("runbooks: catalog unavailable")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runbooks unavailable on this host"})
		return
	}
	tracker := s.runbookProgress()
	for i := range list {
		if p, ok := tracker.Get(list[i].Ref); ok {
			list[i].Progress = p
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runbooks": list, "count": len(list)})
}

type fromRunbookRequest struct {
	Runbook string `json:"runbook"`
	Owner   string `json:"owner,omitempty"`
}

// handleCreateFromRunbook serves POST /api/v1/tasks/from-runbook: a card
// for a committed runbook, written through the board's store.
func (s *Server) handleCreateFromRunbook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.refuseIfReadOnly(w, r) {
		return
	}
	var req fromRunbookRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, maxFromRunbookBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: want {\"runbook\": \"category/name\"}", http.StatusBadRequest)
		return
	}
	if !runbookRefRE.MatchString(req.Runbook) {
		http.Error(w, "runbook must be category/name", http.StatusBadRequest)
		return
	}
	if len(req.Owner) > maxOwnerLen || clipText(req.Owner, maxOwnerLen) != req.Owner {
		http.Error(w, "owner: printable, at most 64 bytes", http.StatusBadRequest)
		return
	}
	if s.runbooks == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runbooks not configured"})
		return
	}
	rb, ok, err := s.runbooks.Get(req.Runbook)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runbooks unavailable on this host"})
		return
	}
	if !ok {
		http.Error(w, "no such runbook in runbooks/MANIFEST.sha256", http.StatusNotFound)
		return
	}
	task, err := runbookCard(rb, req.Owner)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if code, err := s.insertTask(r.Context(), task); err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"task": task})
}

// runbookCard is the card for one runbook; the ref travels in the
// description's ```runbook block (kanban_tasks has no metadata column).
func runbookCard(rb RunbookInfo, owner string) (*Task, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("Run runbook %s (risk %s, %d steps, status %s).", rb.Ref, orDash(rb.Risk), rb.Steps, orDash(rb.Status))
	if rb.Description != "" {
		desc += "\n\n" + rb.Description
	}
	desc += "\n\n```runbook\n" + rb.Ref + "\n```"
	return &Task{
		ID:          "rb-" + strings.ReplaceAll(rb.Ref, "/", "-") + "-" + hex.EncodeToString(b[:]),
		Title:       clipText("Runbook: "+rb.Ref, 200),
		Description: desc,
		Status:      "todo",
		Type:        RunbookCardType,
		Owner:       owner,
	}, nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// runbookRefOf returns a runbook card's runbook, "" for any other card.
func runbookRefOf(t *Task) string {
	if t == nil || t.Type != RunbookCardType {
		return ""
	}
	m := runbookBlockRE.FindStringSubmatch(t.Description)
	if m == nil || !runbookRefRE.MatchString(m[1]) {
		return ""
	}
	return m[1]
}

// taskDetail is GET /api/v1/tasks/{id}: the card's own fields, flattened as
// before, plus what the modal shows beside them.
type taskDetail struct {
	*Task
	Runbook         string           `json:"runbook,omitempty"`
	RunbookProgress *RunbookProgress `json:"runbook_progress,omitempty"`
	AgentRun        *AgentRun        `json:"agent_run,omitempty"`
}

func (s *Server) writeTaskDetail(w http.ResponseWriter, r *http.Request, t *Task) {
	d := taskDetail{Task: t, Runbook: runbookRefOf(t), AgentRun: s.agentRunFor(r.Context(), t.ID)}
	if d.Runbook != "" {
		d.RunbookProgress, _ = s.runbookProgress().Get(d.Runbook)
	}
	writeJSON(w, http.StatusOK, d)
}

// insertTask writes a new card through the TaskManager or the store,
// refusing a duplicate id; the status code goes with the error.
// errInternal is the body for a 500 from insertTask; the cause is logged, not sent.
var errInternal = errors.New("internal server error")

func (s *Server) insertTask(ctx context.Context, task *Task) (int, error) {
	if s.taskManager != nil {
		if err := s.taskManager.CreateTask(ctx, task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to create task")
			// errors.Is: TaskManager wraps ErrInvalidTask with the reason.
			switch {
			case errors.Is(err, ErrTaskAlreadyExists):
				return http.StatusConflict, err
			case errors.Is(err, ErrInvalidTask):
				return http.StatusBadRequest, err
			}
			return http.StatusInternalServerError, errInternal
		}
		return 0, nil
	}
	if s.store != nil {
		now := time.Now()
		task.CreatedAt, task.UpdatedAt = now, now
		// SaveTask upserts: a lookup error taken for "absent" let a create
		// overwrite an existing task.
		existing, err := s.store.GetTask(task.ID)
		if err != nil && !errors.Is(err, ErrStoreNotFound) {
			log.Error().Err(err).Str("task_id", task.ID).Msg("task store read failed")
			return http.StatusInternalServerError, errInternal
		}
		if existing != nil {
			return http.StatusConflict, errors.New("task already exists")
		}
		if err := s.store.SaveTask(task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to save task to store")
			return http.StatusInternalServerError, errInternal
		}
		return 0, nil
	}
	// In-memory only: reached by tests; refuseIfReadOnly stops it otherwise.
	now := time.Now()
	task.CreatedAt, task.UpdatedAt = now, now
	s.tasksMu.Lock()
	defer s.tasksMu.Unlock()
	for _, t := range s.tasks {
		if t.ID == task.ID {
			return http.StatusConflict, errors.New("task already exists")
		}
	}
	s.tasks = append(s.tasks, *task)
	return 0, nil
}
