// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
	"unheaded/pkg/zhenagent"
)

// `zhen-agentd -scheduler` (S-CHAMPION-HARDENING Phase 8, ADR-028
// superseded): approved runbooks on a schedule, inside the daemon that
// already owns the Champion gate, the audit log, a unit and a user.
//
// The schedule is configs/zhen-schedules.yaml (installed as
// /etc/unheaded/zhen/schedules.yaml, root-owned, read-only to the
// daemon). Changes made at run time (the web UI's `schedule`/`unschedule`
// commands, POST /api/v1/scheduler/*) go through the gate as the mutating
// tool schedule_update and persist to -schedules-state, an overlay of
// added and removed jobs; the config file is never written.
//
// A 60 s ticker. Each tick beats (gauge zhen_scheduler_heartbeat_timestamp_
// seconds, Wotan zhen.scheduler.heartbeat) and starts the jobs that are
// due, one after another in one goroutine, so a slow runbook delays the
// next job but never the heartbeat, and a job is never run twice at once.
// A job that came due while another ran starts on the next tick.
//
// Each run: one Kanban card (type scheduled-run, owner zhen-scheduler,
// through kanban_create), one zhen_agent_runs row (mode scheduled,
// migration 037; in memory when the Well is down or the card could not be
// made), the runbook through runbook_execute (manifest check, destructive
// scan, audit row), then the row finished and the card moved: done on
// success, review on failure. A runbook that exits non-zero (a step or its
// verification section failed) is a failed run. Every call carries one
// direct-user reference naming the schedule: the job is Stevie's standing
// instruction, written in a root-owned file or added by him through the
// gate. Only runbooks that are approved, or carry no status at all, may
// be scheduled (ADR-024 "Authorship and approval"); the status is checked
// when a job is loaded or added and again before every run.

const (
	topicSchedulerHeartbeat = "zhen.scheduler.heartbeat"
	// schedulerEmitter is the tool calls' EmittedBy. It must not start
	// with "zhen-agent" (isAgentLoop): the scheduler is not a model.
	schedulerEmitter     = "scheduler"
	defaultSchedulerTick = 60 * time.Second
	// defaultScheduledLease outlives RunbookExecute's 10-minute cap; a run
	// whose lease ran out was cut off by a dead daemon and is abandoned.
	defaultScheduledLease = 15 * time.Minute
	maxScheduleFileBytes  = 64 << 10
	maxRunErrorTail       = 500
)

var (
	schedulerHeartbeat = auto.NewGauge(prom.GaugeOpts{
		Name: "zhen_scheduler_heartbeat_timestamp_seconds",
		Help: "Unix time of zhen-agentd's last scheduler tick (every 60 s, paused or not). Absent or stale: the scheduler is not running (ZhenSchedulerStalled).",
	})
	schedulerRuns = auto.NewCounterVec(prom.CounterOpts{
		Name: "zhen_scheduler_runs_total",
		Help: "Scheduled runbook runs by job and outcome (succeeded, failed).",
	}, []string{"job", "outcome"})
	schedulerRecordFailures = auto.NewCounterVec(prom.CounterOpts{
		Name: "zhen_scheduler_record_failures_total",
		Help: "Scheduled-run records the Well or the board refused, by what (run, finish, card); the run itself still ran.",
	}, []string{"what"})
)

var (
	jobNameRE    = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	runbookRefRE = regexp.MustCompile(`^[a-z0-9-]{1,64}/[a-z0-9-]{1,128}$`)
	hhmmRE       = regexp.MustCompile(`^([01][0-9]|2[0-3]):([0-5][0-9])$`)
)

// scheduleJob is one job of the schedule file (and of the state overlay).
type scheduleJob struct {
	Name    string `yaml:"name" json:"name"`
	Runbook string `yaml:"runbook" json:"runbook"`
	Every   string `yaml:"every" json:"every"`
	Enabled *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

func (j scheduleJob) isEnabled() bool { return j.Enabled == nil || *j.Enabled }

type scheduleFile struct {
	Version int           `yaml:"version"`
	Jobs    []scheduleJob `yaml:"jobs"`
}

// parseSchedules reads a schedule file strictly: unknown keys, a missing
// version, bad names, runbook references outside "category/name" and
// unknown cadences are refused.
func parseSchedules(data []byte) ([]scheduleJob, error) {
	if len(data) > maxScheduleFileBytes {
		return nil, fmt.Errorf("schedule file over %d bytes", maxScheduleFileBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var f scheduleFile
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("schedule file: %w", err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("schedule file: version %d (want 1)", f.Version)
	}
	seen := map[string]bool{}
	for _, j := range f.Jobs {
		if err := validateJob(j); err != nil {
			return nil, err
		}
		if seen[j.Name] {
			return nil, fmt.Errorf("schedule file: job %q twice", j.Name)
		}
		seen[j.Name] = true
	}
	return f.Jobs, nil
}

func validateJob(j scheduleJob) error {
	if !jobNameRE.MatchString(j.Name) {
		return fmt.Errorf("job name %q: want [a-z0-9-]{1,64}", j.Name)
	}
	if !runbookRefRE.MatchString(j.Runbook) {
		return fmt.Errorf("job %s: runbook %q: want category/name under runbooks/", j.Name, j.Runbook)
	}
	if _, err := parseCadence(j.Every); err != nil {
		return fmt.Errorf("job %s: %w", j.Name, err)
	}
	return nil
}

// cadence is when a job comes due: every <N>m|<N>h, daily@HH:MM, or
// weekly@<day>@HH:MM (local time of the daemon).
type cadence struct {
	every   time.Duration // interval; 0 for daily/weekly
	weekly  bool
	weekday time.Weekday
	hour    int
	minute  int
}

var weekdays = map[string]time.Weekday{"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday}

func parseCadence(s string) (cadence, error) {
	bad := fmt.Errorf("every %q: want <N>m, <N>h, daily@HH:MM or weekly@<sun..sat>@HH:MM", s)
	hhmm := func(v string) (int, int, bool) {
		m := hhmmRE.FindStringSubmatch(v)
		if m == nil {
			return 0, 0, false
		}
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		return h, mi, true
	}
	switch {
	case strings.HasPrefix(s, "daily@"):
		h, m, ok := hhmm(strings.TrimPrefix(s, "daily@"))
		if !ok {
			return cadence{}, bad
		}
		return cadence{hour: h, minute: m}, nil
	case strings.HasPrefix(s, "weekly@"):
		parts := strings.Split(strings.TrimPrefix(s, "weekly@"), "@")
		if len(parts) != 2 {
			return cadence{}, bad
		}
		wd, ok := weekdays[parts[0]]
		h, m, ok2 := hhmm(parts[1])
		if !ok || !ok2 {
			return cadence{}, bad
		}
		return cadence{weekly: true, weekday: wd, hour: h, minute: m}, nil
	case strings.HasSuffix(s, "m") || strings.HasSuffix(s, "h"):
		n, err := strconv.Atoi(s[:len(s)-1])
		if err != nil || n <= 0 {
			return cadence{}, bad
		}
		d := time.Duration(n) * time.Minute
		if strings.HasSuffix(s, "h") {
			d = time.Duration(n) * time.Hour
		}
		if d < 5*time.Minute || d > 31*24*time.Hour {
			return cadence{}, fmt.Errorf("every %q: between 5m and 744h", s)
		}
		return cadence{every: d}, nil
	}
	return cadence{}, bad
}

// next is the first due time strictly after t. An interval counts from t
// (no catch-up burst after downtime); daily and weekly use t's location.
func (c cadence) next(t time.Time) time.Time {
	if c.every > 0 {
		return t.Add(c.every)
	}
	d := time.Date(t.Year(), t.Month(), t.Day(), c.hour, c.minute, 0, 0, t.Location())
	if c.weekly {
		d = d.AddDate(0, 0, (int(c.weekday)-int(d.Weekday())+7)%7)
		if !d.After(t) {
			d = d.AddDate(0, 0, 7)
		}
		return d
	}
	if !d.After(t) {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// runbookApproved is the scheduling rule: a runbook with no
// metadata.status (the reviewed pre-policy set) or status approved.
// draft, stub and anything else are refused.
func runbookApproved(body string) error {
	var doc struct {
		Metadata struct {
			Status *string `yaml:"status"`
		} `yaml:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return fmt.Errorf("unparsable runbook: %w", err)
	}
	if doc.Metadata.Status == nil || *doc.Metadata.Status == "approved" {
		return nil
	}
	return fmt.Errorf("status %q: only an approved runbook (or one with no status) may be scheduled (ADR-024)", *doc.Metadata.Status)
}

// schedChampion is what the scheduler needs of the Champion.
type schedChampion interface {
	Dispatch(ctx context.Context, tc champion.ToolCall) (any, error)
	RunbookShow(ctx context.Context, name string) (string, error)
	RunbookList(ctx context.Context) ([]string, error)
}

type schedulerConfig struct {
	Champion  schedChampion
	Path      string // the schedule file
	StatePath string // overlay of run-time changes; "" keeps them in memory
	// Runs records each run (zhen_agent_runs as app_zhen); nil keeps runs
	// in memory only.
	Runs   zhenagent.ScheduledStore
	Events runEvents                  // audit_events + zhen.agent.runs; nil: none
	Pub    claudebridge.JSONPublisher // heartbeat on Wotan; nil: none
	Log    *slog.Logger
	Now    func() time.Time
	Lease  time.Duration
}

type jobState struct {
	scheduleJob
	cad         cadence
	source      string // config or added
	next        time.Time
	lastRun     string
	lastStatus  string
	lastError   string
	lastStarted time.Time
}

// scheduleState is the -schedules-state overlay.
type scheduleState struct {
	Added   []scheduleJob `json:"added,omitempty"`
	Removed []string      `json:"removed,omitempty"`
	Paused  bool          `json:"paused,omitempty"`
}

type scheduler struct {
	cfg  schedulerConfig
	mem  *zhenagent.MemStore // runs the Well could not take
	refs func(job string) []champion.Reference

	mu       sync.Mutex
	jobs     map[string]*jobState
	state    scheduleState
	paused   bool
	busy     bool
	lastTick time.Time
	wg       sync.WaitGroup
}

func newScheduler(ctx context.Context, cfg schedulerConfig) (*scheduler, error) {
	if cfg.Champion == nil {
		return nil, errors.New("scheduler: no Champion")
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Lease <= 0 {
		cfg.Lease = defaultScheduledLease
	}
	abs, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, err
	}
	cfg.Path = abs
	s := &scheduler{cfg: cfg, mem: zhenagent.NewMemStore(), jobs: map[string]*jobState{}}
	if s.cfg.Runs == nil {
		s.cfg.Runs = s.mem
	}
	s.refs = func(job string) []champion.Reference {
		return []champion.Reference{{
			Topic: "schedule/" + job, Category: "scheduler", SourceKind: "user-custom", SourceTrust: "direct-user",
			SourcePath: abs, SourceLabel: "zhen-agentd -scheduler",
		}}
	}
	data, err := os.ReadFile(abs) // #nosec G304 -- the operator's -schedules flag
	if err != nil {
		return nil, fmt.Errorf("schedule file: %w", err)
	}
	jobs, err := parseSchedules(data)
	if err != nil {
		return nil, err
	}
	if cfg.StatePath != "" {
		if b, err := os.ReadFile(cfg.StatePath); err == nil { // #nosec G304 -- the operator's -schedules-state flag
			if err := json.Unmarshal(b, &s.state); err != nil {
				return nil, fmt.Errorf("schedule state %s: %w", cfg.StatePath, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("schedule state: %w", err)
		}
	}
	removed := map[string]bool{}
	for _, n := range s.state.Removed {
		removed[n] = true
	}
	now := cfg.Now()
	for _, j := range jobs {
		if removed[j.Name] {
			continue
		}
		if err := s.checkRunbook(ctx, j.Runbook); err != nil {
			return nil, fmt.Errorf("job %s: %w", j.Name, err)
		}
		cad, _ := parseCadence(j.Every)
		s.jobs[j.Name] = &jobState{scheduleJob: j, cad: cad, source: "config", next: cad.next(now)}
	}
	// Added jobs were checked when added; one whose runbook is no longer
	// schedulable is skipped (logged), not fatal: the file is the base.
	for _, j := range s.state.Added {
		if err := validateJob(j); err != nil {
			cfg.Log.Warn("scheduler_state_job_skipped", "job", j.Name, "error", err.Error())
			continue
		}
		if err := s.checkRunbook(ctx, j.Runbook); err != nil {
			cfg.Log.Warn("scheduler_state_job_skipped", "job", j.Name, "error", err.Error())
			continue
		}
		cad, _ := parseCadence(j.Every)
		s.jobs[j.Name] = &jobState{scheduleJob: j, cad: cad, source: "added", next: cad.next(now)}
	}
	s.paused = s.state.Paused
	return s, nil
}

// checkRunbook reads the runbook through the Champion (path rules, symlink
// refusal) and applies the scheduling rule.
func (s *scheduler) checkRunbook(ctx context.Context, name string) error {
	body, err := s.cfg.Champion.RunbookShow(ctx, name)
	if err != nil {
		return fmt.Errorf("runbook %s: %w", name, err)
	}
	if err := runbookApproved(body); err != nil {
		return fmt.Errorf("runbook %s: %w", name, err)
	}
	return nil
}

// loop ticks until ctx ends, then waits for a run in flight.
func (s *scheduler) loop(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = defaultSchedulerTick
	}
	t := time.NewTicker(every)
	defer t.Stop()
	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			s.wait()
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

// wait blocks until the jobs started by earlier ticks have finished.
func (s *scheduler) wait() { s.wg.Wait() }

type dueJob struct{ name, runbook string }

// tick beats and starts the due jobs.
func (s *scheduler) tick(ctx context.Context) {
	now := s.cfg.Now()
	s.mu.Lock()
	s.lastTick = now
	var due []dueJob
	if !s.paused && !s.busy {
		for _, name := range s.namesLocked() {
			j := s.jobs[name]
			if j.isEnabled() && !now.Before(j.next) {
				due = append(due, dueJob{name: j.Name, runbook: j.Runbook})
				j.next = j.cad.next(now)
			}
		}
		if len(due) > 0 {
			s.busy = true
			s.wg.Add(1)
		}
	}
	nJobs, paused, busy := len(s.jobs), s.paused, s.busy
	s.mu.Unlock()

	schedulerHeartbeat.Set(float64(now.UnixNano()) / 1e9)
	if s.cfg.Pub != nil {
		s.cfg.Pub.PublishJSON(topicSchedulerHeartbeat, map[string]any{
			"ts": now.UTC().Format(time.RFC3339Nano), "source": "zhen-agentd", "jobs": nJobs, "paused": paused,
			"running": busy, "started": len(due),
		})
	}
	if ex, err := s.cfg.Runs.ExpireLeases(ctx, time.Now()); err != nil {
		s.cfg.Log.Warn("scheduler_expire_leases", "error", err.Error())
	} else {
		for _, r := range ex {
			s.cfg.Log.Warn("scheduler_lease_expired", "run", r.ID, "card", r.CardID, "mode", r.Mode)
		}
	}
	if len(due) == 0 {
		return
	}
	go func() {
		defer s.wg.Done()
		for _, j := range due {
			s.runJob(ctx, j)
		}
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()
}

func (s *scheduler) namesLocked() []string {
	names := make([]string, 0, len(s.jobs))
	for n := range s.jobs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// runJob is one scheduled run: card, run row, runbook, outcome.
func (s *scheduler) runJob(ctx context.Context, j dueJob) {
	runID := zhenagent.NewRunID()
	cardID := "sched-" + j.name + "-" + runID
	started := s.cfg.Now()
	refs := s.refs(j.name)
	call := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return s.cfg.Champion.Dispatch(ctx, champion.ToolCall{Name: tool, Args: args, Justification: refs, EmittedBy: schedulerEmitter})
	}
	log := s.cfg.Log.With("job", j.name, "run", runID, "runbook", j.runbook)

	_, cardErr := call(ctx, "kanban_create", map[string]any{"task": map[string]any{
		"id": cardID, "title": fmt.Sprintf("Scheduled: %s (%s)", j.name, j.runbook),
		"description": fmt.Sprintf("zhen-agentd -scheduler run %s of job %s: runbook %s, started %s.", runID, j.name, j.runbook, started.UTC().Format(time.RFC3339)),
		"status":      "in-progress", "type": zhenagent.ScheduledCardType, "owner": zhenagent.ScheduledCardOwner, "progress": 0,
	}})
	if cardErr != nil {
		schedulerRecordFailures.WithLabelValues("card").Inc()
		log.Warn("scheduler_card", "error", cardErr.Error())
	}

	run := zhenagent.Run{ID: runID, CardID: cardID, Family: j.name, LeaseUntil: time.Now().Add(s.cfg.Lease)}
	store := s.cfg.Runs
	if cardErr != nil {
		store = s.mem // the Well's foreign key needs the card
	}
	if err := store.BeginScheduled(ctx, run); err != nil {
		if store != zhenagent.ScheduledStore(s.mem) {
			schedulerRecordFailures.WithLabelValues("run").Inc()
			log.Warn("scheduler_record_run", "error", err.Error())
			store = s.mem
			err = store.BeginScheduled(ctx, run)
		}
		if err != nil {
			log.Error("scheduler_record_run_memory", "error", err.Error())
		}
	}
	s.event(ctx, runEvent{Kind: "agent_run", Run: runID, Card: cardID, Mode: zhenagent.ModeScheduled, Family: j.name, To: zhenagent.StatusRunning})

	runErr := s.execute(ctx, j, call)
	status, check, errText := zhenagent.StatusSucceeded, "pass", ""
	if runErr != nil {
		status, check, errText = zhenagent.StatusFailed, "fail", runErr.Error()
	}

	// The outcome is recorded even when the daemon is stopping.
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := store.Finish(fctx, runID, zhenagent.Outcome{Status: status, Error: errText,
		Summary: zhenagent.RouteSummary{Route: "runbook", Check: check}}); err != nil {
		schedulerRecordFailures.WithLabelValues("finish").Inc()
		log.Warn("scheduler_finish_run", "error", err.Error())
	}
	if cardErr == nil {
		updates := map[string]any{"status": "done", "progress": 100}
		if runErr != nil {
			updates = map[string]any{"status": "review", "description": fmt.Sprintf("zhen-agentd -scheduler run %s of job %s FAILED: %s", runID, j.name, truncateRunes(errText, 800))}
		}
		if _, err := call(fctx, "kanban_update", map[string]any{"id": cardID, "updates": updates}); err != nil {
			schedulerRecordFailures.WithLabelValues("card").Inc()
			log.Warn("scheduler_card_update", "error", err.Error())
		}
	}
	s.event(fctx, runEvent{Kind: "agent_run", Run: runID, Card: cardID, Mode: zhenagent.ModeScheduled, Family: j.name,
		From: zhenagent.StatusRunning, To: status, Check: check})
	schedulerRuns.WithLabelValues(j.name, status).Inc()
	if runErr != nil {
		log.Warn("scheduler_run_failed", "error", errText)
	} else {
		log.Info("scheduler_run_succeeded")
	}

	s.mu.Lock()
	if js := s.jobs[j.name]; js != nil {
		js.lastRun, js.lastStatus, js.lastStarted = runID, status, started
		js.lastError = errText
	}
	s.mu.Unlock()
}

// execute re-checks the runbook's status and runs it through the gate.
func (s *scheduler) execute(ctx context.Context, j dueJob, call func(context.Context, string, map[string]any) (any, error)) error {
	if err := s.checkRunbook(ctx, j.runbook); err != nil {
		return err
	}
	out, err := call(ctx, "runbook_execute", map[string]any{"name": j.runbook})
	if err != nil {
		return err
	}
	res, ok := out.(*champion.RunbookExecuteResult)
	if !ok || res == nil {
		return fmt.Errorf("runbook_execute returned %T", out)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("exit %d: %s", res.ExitCode, tail(res.Output, maxRunErrorTail))
	}
	return nil
}

func (s *scheduler) event(ctx context.Context, ev runEvent) {
	if s.cfg.Events != nil {
		s.cfg.Events.RunEvent(ctx, ev)
	}
}

// tail keeps the last n bytes of s (on a rune boundary), trimmed.
func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	i := len(s) - n
	for i < len(s) && s[i]&0xC0 == 0x80 {
		i++
	}
	return "…" + s[i:]
}

func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

// jobView is one job as /api/v1/scheduler/list shows it.
type jobView struct {
	Name        string     `json:"name"`
	Runbook     string     `json:"runbook"`
	Every       string     `json:"every"`
	Enabled     bool       `json:"enabled"`
	Source      string     `json:"source"` // config or added
	NextDue     time.Time  `json:"next_due"`
	LastRun     string     `json:"last_run,omitempty"`
	LastStatus  string     `json:"last_status,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	LastStarted *time.Time `json:"last_started,omitempty"`
}

type schedulerView struct {
	Jobs     []jobView  `json:"jobs"`
	Paused   bool       `json:"paused"`
	Running  bool       `json:"running"`
	LastTick *time.Time `json:"last_tick,omitempty"`
	Config   string     `json:"config"`
}

func (s *scheduler) view() schedulerView {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := schedulerView{Jobs: []jobView{}, Paused: s.paused, Running: s.busy, Config: s.cfg.Path}
	if !s.lastTick.IsZero() {
		t := s.lastTick
		v.LastTick = &t
	}
	for _, n := range s.namesLocked() {
		v.Jobs = append(v.Jobs, s.jobs[n].viewLocked())
	}
	return v
}

func (j *jobState) viewLocked() jobView {
	jv := jobView{Name: j.Name, Runbook: j.Runbook, Every: j.Every, Enabled: j.isEnabled(), Source: j.source, NextDue: j.next,
		LastRun: j.lastRun, LastStatus: j.lastStatus, LastError: j.lastError}
	if !j.lastStarted.IsZero() {
		t := j.lastStarted
		jv.LastStarted = &t
	}
	return jv
}

// UpdateSchedule implements champion.ScheduleUpdater: schedule_update
// after the gate. add {name, runbook?, every, enabled?}, remove {name},
// pause {paused}.
func (s *scheduler) UpdateSchedule(ctx context.Context, op string, args map[string]any) (any, error) {
	str := func(k string) string { v, _ := args[k].(string); return strings.TrimSpace(v) }
	switch op {
	case "add":
		j := scheduleJob{Name: str("name"), Runbook: str("runbook"), Every: str("every")}
		if v, ok := args["enabled"].(bool); ok {
			j.Enabled = &v
		}
		if strings.Contains(j.Name, "/") && j.Runbook == "" {
			j.Runbook, j.Name = j.Name, j.Name[strings.LastIndex(j.Name, "/")+1:]
		}
		if j.Runbook == "" || !strings.Contains(j.Runbook, "/") {
			rb, err := s.resolveRunbook(ctx, firstNonEmpty(j.Runbook, j.Name))
			if err != nil {
				return nil, err
			}
			j.Runbook = rb
		}
		if err := validateJob(j); err != nil {
			return nil, err
		}
		if err := s.checkRunbook(ctx, j.Runbook); err != nil {
			return nil, err
		}
		cad, _ := parseCadence(j.Every)
		s.mu.Lock()
		defer s.mu.Unlock()
		js := &jobState{scheduleJob: j, cad: cad, source: "added", next: cad.next(s.cfg.Now())}
		if old := s.jobs[j.Name]; old != nil {
			js.lastRun, js.lastStatus, js.lastError, js.lastStarted = old.lastRun, old.lastStatus, old.lastError, old.lastStarted
		}
		s.jobs[j.Name] = js
		s.state.Added = append(withoutJob(s.state.Added, j.Name), j)
		s.state.Removed = withoutName(s.state.Removed, j.Name)
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
		return js.viewLocked(), nil
	case "remove":
		name := str("name")
		s.mu.Lock()
		defer s.mu.Unlock()
		js := s.jobs[name]
		if js == nil {
			return nil, fmt.Errorf("no scheduled job %q", name)
		}
		delete(s.jobs, name)
		s.state.Added = withoutJob(s.state.Added, name)
		if js.source == "config" {
			s.state.Removed = append(withoutName(s.state.Removed, name), name)
		}
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
		return map[string]any{"removed": name}, nil
	case "pause":
		p, ok := args["paused"].(bool)
		if !ok {
			return nil, errors.New(`pause needs "paused": true or false`)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.paused, s.state.Paused = p, p
		if err := s.persistLocked(); err != nil {
			return nil, err
		}
		return map[string]any{"paused": p}, nil
	}
	return nil, fmt.Errorf("unknown schedule op %q (add, remove, pause)", op)
}

// resolveRunbook turns a bare runbook name into its one category/name.
func (s *scheduler) resolveRunbook(ctx context.Context, name string) (string, error) {
	if !jobNameRE.MatchString(name) {
		return "", fmt.Errorf("runbook %q: want category/name or a bare name", name)
	}
	all, err := s.cfg.Champion.RunbookList(ctx)
	if err != nil {
		return "", err
	}
	var hits []string
	for _, rb := range all {
		if strings.HasSuffix(rb, "/"+name) {
			hits = append(hits, rb)
		}
	}
	sort.Strings(hits)
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return "", fmt.Errorf("no runbook named %q", name)
	}
	return "", fmt.Errorf("runbook %q is ambiguous (%s): name it as category/name", name, strings.Join(hits, ", "))
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func withoutJob(js []scheduleJob, name string) []scheduleJob {
	out := js[:0:0]
	for _, j := range js {
		if j.Name != name {
			out = append(out, j)
		}
	}
	return out
}

func withoutName(ns []string, name string) []string {
	out := ns[:0:0]
	for _, n := range ns {
		if n != name {
			out = append(out, n)
		}
	}
	return out
}

// persistLocked writes the overlay atomically (0600); a no-op without
// -schedules-state.
func (s *scheduler) persistLocked() error {
	if s.cfg.StatePath == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.cfg.StatePath), ".schedules-state-*")
	if err != nil {
		return fmt.Errorf("schedule state: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("schedule state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("schedule state: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.cfg.StatePath); err != nil {
		return fmt.Errorf("schedule state: %w", err)
	}
	return nil
}

// openScheduledStore records runs in the Well as app_zhen when
// WELL_APP_ZHEN_DSN is set and answers; otherwise in memory (logged).
func openScheduledStore(ctx context.Context, log *slog.Logger) (zhenagent.ScheduledStore, func()) {
	dsn := strings.TrimSpace(os.Getenv("WELL_APP_ZHEN_DSN"))
	if dsn == "" {
		log.Warn("scheduler_runs_in_memory", "reason", "WELL_APP_ZHEN_DSN not set")
		return nil, func() {}
	}
	warnIfInsecureDSN(dsn)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Warn("scheduler_runs_in_memory", "reason", err.Error())
		return nil, func() {}
	}
	db.SetMaxOpenConns(2)
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		_ = db.Close()
		log.Warn("scheduler_runs_in_memory", "reason", "the Well is unreachable: "+err.Error())
		return nil, func() {}
	}
	return zhenagent.SQLStore{DB: db}, func() { _ = db.Close() }
}
