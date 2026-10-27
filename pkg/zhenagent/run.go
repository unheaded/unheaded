// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package zhenagent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Run states (migration 032). They move one way:
//
//	claimed   -> running | abandoned | cancelled
//	running   -> succeeded | failed | abandoned | cancelled
//	succeeded -> accepted | rejected
const (
	StatusClaimed   = "claimed"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusAbandoned = "abandoned"
	StatusCancelled = "cancelled"
	StatusAccepted  = "accepted"
	StatusRejected  = "rejected"
)

// Modes. ModeScheduled is zhen-agentd's scheduler running an approved
// runbook (migration 037): no branch, no base commit, never reviewed.
const (
	ModeInteractive = "interactive"
	ModeOvernight   = "overnight"
	ModeScheduled   = "scheduled"
)

// A scheduled run's card: its own type and owner, so the worker never
// takes it (EligibleTypes, Owner).
const (
	ScheduledCardType  = "scheduled-run"
	ScheduledCardOwner = "zhen-scheduler"
)

// ScheduledStore records the scheduler's runs in zhen_agent_runs (mode
// scheduled). Finish ends them like any run; Review refuses them.
type ScheduledStore interface {
	// BeginScheduled inserts r as mode scheduled, without a branch or
	// base commit, and moves it to running.
	BeginScheduled(ctx context.Context, r Run) error
	Finish(ctx context.Context, id string, o Outcome) error
	// ExpireLeases abandons active runs whose lease ran out (a daemon
	// that died mid-run), whatever their mode.
	ExpireLeases(ctx context.Context, now time.Time) ([]Run, error)
}

// checkScheduled validates a scheduled run before it is recorded (032's
// and 037's CHECKs, so a MemStore refuses what the Well would).
func checkScheduled(r Run, now time.Time) error {
	switch {
	case !ValidRunID(r.ID):
		return fmt.Errorf("zhenagent: bad run id %q", r.ID)
	case r.CardID == "":
		return errors.New("zhenagent: a scheduled run needs its card")
	case !familyRE.MatchString(r.Family):
		return fmt.Errorf("zhenagent: bad family %q", r.Family)
	case !r.LeaseUntil.After(now):
		return errors.New("zhenagent: a scheduled run's lease must end in the future")
	}
	return nil
}

// Lease timing (Step 197).
const (
	DefaultLease = 30 * time.Minute
	DefaultRenew = 5 * time.Minute
)

// MaxErrorBytes bounds a run's error text (032's CHECK).
const MaxErrorBytes = 1000

var transitions = map[string]map[string]bool{
	StatusClaimed:   {StatusRunning: true, StatusAbandoned: true, StatusCancelled: true},
	StatusRunning:   {StatusSucceeded: true, StatusFailed: true, StatusAbandoned: true, StatusCancelled: true},
	StatusSucceeded: {StatusAccepted: true, StatusRejected: true},
}

// CanMove reports whether from -> to is a legal run transition.
func CanMove(from, to string) bool { return transitions[from][to] }

// KeepProgress leaves a card's progress as it is (MoveCard).
const KeepProgress = -1

// Errors.
var (
	ErrClaimConflict = errors.New("card already claimed or no longer eligible")
	ErrNotInState    = errors.New("run is not in the expected state")
	ErrNoRun         = errors.New("no such run")
)

// RouteSummary is what the router did, kept on the run (no text).
type RouteSummary struct {
	Route         string `json:"route,omitempty"`
	RouteReason   string `json:"route_reason,omitempty"`
	LocalAttempts int    `json:"local_attempts,omitempty"`
	Escalated     bool   `json:"escalated,omitempty"`
	Check         string `json:"check,omitempty"`
	Stop          string `json:"stop,omitempty"`
	ReviewNote    string `json:"review_note,omitempty"` // path of the review note on this host
	Changed       int    `json:"files_changed,omitempty"`
}

// Run is one row of zhen_agent_runs.
type Run struct {
	ID            string
	CardID        string
	ClaimedAt     time.Time
	LeaseUntil    time.Time
	Status        string
	Mode          string
	Family        string
	Branch        string
	BaseCommit    string
	HeadCommit    string
	Summary       RouteSummary
	ClaudeNanoUSD int64
	ReviewReason  string
	ReviewedAt    *time.Time
	Error         string
}

// CommitEntry is what the worker appends to a card's commits JSONB (the
// card modal lists them).
type CommitEntry struct {
	Hash       string `json:"hash"`
	Message    string `json:"message"`
	Date       string `json:"date"`
	Branch     string `json:"branch,omitempty"`
	Run        string `json:"run,omitempty"`
	ReviewNote string `json:"review_note,omitempty"`
}

// Outcome is how a run ends (Finish).
type Outcome struct {
	Status        string // succeeded, failed, cancelled, abandoned
	HeadCommit    string
	Summary       RouteSummary
	ClaudeNanoUSD int64
	Error         string
}

// Store is zhen_agent_runs plus the card columns the worker moves.
type Store interface {
	// Candidates are cards that may be eligible: owner zhen, todo, an
	// eligible type, not deleted or archived, no active run and fewer
	// than MaxAttempts counted attempts. The worker still checks each.
	Candidates(ctx context.Context, limit int) ([]Card, error)
	// Attempts counts a card's runs that are not cancelled.
	Attempts(ctx context.Context, cardID string) (int, error)
	// Claim inserts r (status claimed) and moves the card to
	// in-progress, atomically, only while the card is still an eligible
	// todo card; otherwise ErrClaimConflict.
	Claim(ctx context.Context, r Run) error
	// Start moves a claimed run to running.
	Start(ctx context.Context, id string) error
	// Renew extends an active run's lease.
	Renew(ctx context.Context, id string, until time.Time) error
	// Finish ends an active run.
	Finish(ctx context.Context, id string, o Outcome) error
	// MoveCard sets a card's status and progress (KeepProgress: leave it)
	// and appends entry (nil: none) to its commits.
	MoveCard(ctx context.Context, cardID, status string, progress int, entry *CommitEntry) error
	// ExpireLeases turns active runs whose lease ran out into abandoned
	// and returns them.
	ExpireLeases(ctx context.Context, now time.Time) ([]Run, error)
	// List returns the newest runs.
	List(ctx context.Context, limit int) ([]Run, error)
	// Get returns one run (ErrNoRun).
	Get(ctx context.Context, id string) (Run, error)
	// Review moves a succeeded run to accepted or rejected.
	Review(ctx context.Context, id, status, reason string) error
}

// MemStore is an in-memory Store with the same rules as migration 032
// (one active run per card, one-way states), for tests and dry runs.
type MemStore struct {
	mu    sync.Mutex
	Cards map[string]*MemCard
	runs  map[string]*Run
	order []string
}

// MemCard is a card in a MemStore.
type MemCard struct {
	Card
	Progress int
	Commits  []CommitEntry
	Created  time.Time
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{Cards: map[string]*MemCard{}, runs: map[string]*Run{}}
}

// AddCard adds a card.
func (m *MemStore) AddCard(c Card) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Cards[c.ID] = &MemCard{Card: c, Created: time.Now().Add(time.Duration(len(m.Cards)) * time.Millisecond)}
}

// CardState returns a copy of a card.
func (m *MemStore) CardState(id string) (MemCard, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.Cards[id]
	if !ok {
		return MemCard{}, false
	}
	cp := *c
	cp.Commits = append([]CommitEntry(nil), c.Commits...)
	return cp, true
}

func (m *MemStore) activeLocked(cardID string) bool {
	for _, r := range m.runs {
		if r.CardID == cardID && (r.Status == StatusClaimed || r.Status == StatusRunning) {
			return true
		}
	}
	return false
}

func (m *MemStore) attemptsLocked(cardID string) int {
	n := 0
	for _, r := range m.runs {
		if r.CardID == cardID && r.Status != StatusCancelled {
			n++
		}
	}
	return n
}

// Candidates implements Store.
func (m *MemStore) Candidates(_ context.Context, limit int) ([]Card, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var cs []*MemCard
	for _, c := range m.Cards {
		if c.Owner == Owner && c.Status == "todo" && EligibleTypes[c.Type] && !c.Deleted && !c.Archived &&
			!m.activeLocked(c.ID) && m.attemptsLocked(c.ID) < MaxAttempts {
			cs = append(cs, c)
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].Created.Before(cs[j].Created) })
	var out []Card
	for i, c := range cs {
		if i >= limit {
			break
		}
		out = append(out, c.Card)
	}
	return out, nil
}

// Attempts implements Store.
func (m *MemStore) Attempts(_ context.Context, cardID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.attemptsLocked(cardID), nil
}

// Claim implements Store.
func (m *MemStore) Claim(_ context.Context, r Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.Cards[r.CardID]
	if !ok || Eligible(c.Card) != nil || m.activeLocked(r.CardID) {
		return ErrClaimConflict
	}
	if _, dup := m.runs[r.ID]; dup || !ValidRunID(r.ID) {
		return fmt.Errorf("zhenagent: bad run id %q", r.ID)
	}
	r.Status = StatusClaimed
	if r.ClaimedAt.IsZero() {
		r.ClaimedAt = time.Now()
	}
	cp := r
	m.runs[r.ID] = &cp
	m.order = append(m.order, r.ID)
	c.Status = "in-progress"
	return nil
}

func (m *MemStore) move(id, to string, f func(*Run)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return ErrNoRun
	}
	if !CanMove(r.Status, to) {
		return fmt.Errorf("%w: %s -> %s", ErrNotInState, r.Status, to)
	}
	r.Status = to
	if f != nil {
		f(r)
	}
	return nil
}

// Start implements Store.
func (m *MemStore) Start(_ context.Context, id string) error {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return ErrNoRun
	}
	if r.Status != StatusClaimed {
		return fmt.Errorf("%w: %s -> running", ErrNotInState, r.Status)
	}
	return m.move(id, StatusRunning, nil)
}

// Renew implements Store.
func (m *MemStore) Renew(_ context.Context, id string, until time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return ErrNoRun
	}
	if r.Status != StatusClaimed && r.Status != StatusRunning {
		return ErrNotInState
	}
	r.LeaseUntil = until
	return nil
}

// Finish implements Store.
func (m *MemStore) Finish(_ context.Context, id string, o Outcome) error {
	m.mu.Lock()
	r, ok := m.runs[id]
	m.mu.Unlock()
	if !ok {
		return ErrNoRun
	}
	if r.Status != StatusClaimed && r.Status != StatusRunning {
		return fmt.Errorf("%w: %s is not active", ErrNotInState, r.Status)
	}
	return m.move(id, o.Status, func(r *Run) {
		r.HeadCommit, r.Summary, r.ClaudeNanoUSD, r.Error = o.HeadCommit, o.Summary, o.ClaudeNanoUSD, truncate(o.Error, MaxErrorBytes)
	})
}

// MoveCard implements Store.
func (m *MemStore) MoveCard(_ context.Context, cardID, status string, progress int, entry *CommitEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.Cards[cardID]
	if !ok {
		return fmt.Errorf("zhenagent: no card %q", cardID)
	}
	c.Status = status
	if progress != KeepProgress {
		c.Progress = progress
	}
	if entry != nil {
		c.Commits = append(c.Commits, *entry)
	}
	return nil
}

// ExpireLeases implements Store.
func (m *MemStore) ExpireLeases(_ context.Context, now time.Time) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for _, id := range m.order {
		r := m.runs[id]
		if (r.Status == StatusClaimed || r.Status == StatusRunning) && !r.LeaseUntil.After(now) {
			r.Status = StatusAbandoned
			r.Error = "lease expired"
			out = append(out, *r)
		}
	}
	return out, nil
}

// List implements Store.
func (m *MemStore) List(_ context.Context, limit int) ([]Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Run
	for i := len(m.order) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, *m.runs[m.order[i]])
	}
	return out, nil
}

// Get implements Store.
func (m *MemStore) Get(_ context.Context, id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return Run{}, ErrNoRun
	}
	return *r, nil
}

// BeginScheduled implements ScheduledStore. The card is not looked up:
// the MemStore does not model kanban_tasks' foreign key.
func (m *MemStore) BeginScheduled(_ context.Context, r Run) error {
	now := time.Now()
	if err := checkScheduled(r, now); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, dup := m.runs[r.ID]; dup {
		return fmt.Errorf("zhenagent: duplicate run id %q", r.ID)
	}
	if m.activeLocked(r.CardID) {
		return ErrClaimConflict
	}
	r.Status, r.Mode, r.Branch, r.BaseCommit, r.ClaimedAt = StatusRunning, ModeScheduled, "", "", now
	cp := r
	m.runs[r.ID] = &cp
	m.order = append(m.order, r.ID)
	return nil
}

// Review implements Store.
func (m *MemStore) Review(_ context.Context, id, status, reason string) error {
	if status != StatusAccepted && status != StatusRejected {
		return fmt.Errorf("zhenagent: review status %q", status)
	}
	m.mu.Lock()
	r, ok := m.runs[id]
	scheduled := ok && r.Mode == ModeScheduled
	m.mu.Unlock()
	if scheduled {
		return fmt.Errorf("%w: a scheduled run is not reviewed", ErrNotInState)
	}
	now := time.Now()
	return m.move(id, status, func(r *Run) { r.ReviewReason, r.ReviewedAt = reason, &now })
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
