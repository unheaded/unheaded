// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// ErrSpendCap is returned (wrapped in *SpendCapError) when a request
// would take the day's spend past the cap. The request is not sent.
var ErrSpendCap = errors.New("daily spend cap reached")

// SpendCapError says how far the day has got and what was asked for.
type SpendCapError struct {
	Day     string
	Spent   NanoUSD // already recorded today
	Pending NanoUSD // reserved by requests in flight in this process
	Need    NanoUSD // worst-case cost of the refused request
	Cap     NanoUSD
}

func (e *SpendCapError) Error() string {
	return fmt.Sprintf("%v: %s spent on %s (+%s in flight), next request could cost up to %s, cap %s",
		ErrSpendCap, e.Spent.USD(), e.Day, e.Pending.USD(), e.Need.USD(), e.Cap.USD())
}

func (e *SpendCapError) Unwrap() error { return ErrSpendCap }

// Ledger is the hard per-day (UTC) spend cap. Before a request is sent the
// caller reserves its worst-case cost; after the response it commits what
// usage says was spent. The day's total lives in a JSON file so the cap
// holds across one-shot CLI runs; a file that cannot be read refuses
// every request (fail closed).
//
// Concurrency: reservations are tracked in memory, so two requests in one
// process cannot both slip under the cap. Two processes sharing a file
// each see the other's committed spend (the file is flock'd for each
// read-modify-write) but not its in-flight reservations, so concurrent
// processes can overshoot by at most one reservation each.
type Ledger struct {
	path string // "" keeps the total in memory only (tests)
	cap  NanoUSD
	now  func() time.Time

	mu      sync.Mutex
	pending NanoUSD
	mem     ledgerState
}

type ledgerState struct {
	Day      string  `json:"day"`
	Spent    NanoUSD `json:"spent_nano_usd"`
	Requests int64   `json:"requests"`
}

// NewLedger opens a ledger at path with the given daily cap. A zero cap
// refuses everything, which is the right reading of "no budget".
func NewLedger(path string, dailyCap NanoUSD) (*Ledger, error) {
	if dailyCap < 0 {
		return nil, errors.New("ledger: negative cap")
	}
	l := &Ledger{path: path, cap: dailyCap, now: time.Now}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("ledger: %w", err)
		}
		if _, err := l.readLocked(); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// DefaultLedgerPath is $XDG_STATE_HOME/unheaded/zhen-claude/spend.json, or
// ~/.local/state/... when XDG_STATE_HOME is unset.
func DefaultLedgerPath() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", errors.New("ledger: cannot resolve a home directory; pass a ledger path")
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "unheaded", "zhen-claude", "spend.json"), nil
}

// Cap returns the daily cap.
func (l *Ledger) Cap() NanoUSD { return l.cap }

// Spent returns today's committed spend.
func (l *Ledger) Spent() (NanoUSD, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, err := l.readLocked()
	return st.Spent, err
}

// Remaining returns what today's cap has left after the committed spend
// and this process's in-flight reservations; never negative.
func (l *Ledger) Remaining() (NanoUSD, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	st, err := l.readLocked()
	if err != nil {
		return 0, err
	}
	return max(l.cap-st.Spent-l.pending, 0), nil
}

// WorstCase is the most one request can cost: promptTokens at inRate plus
// maxTokens at outRate (rates in milli-USD per million tokens, as in
// Price), times hops (2 when a server-side refusal fallback may run the
// request a second time).
func WorstCase(promptTokens, inRate, maxTokens, outRate, hops int64) NanoUSD {
	return NanoUSD(hops * (promptTokens*inRate + maxTokens*outRate))
}

// FitMaxTokens is the pre-check behind a capped request (S-CHAMPION-
// HARDENING Step 114): the largest max_tokens, at most want, whose
// WorstCase fits avail. 0 means not even floor fits, so the request must
// be refused before it is sent.
func FitMaxTokens(avail NanoUSD, promptTokens, inRate, outRate, hops, want, floor int64) int64 {
	if avail <= 0 || outRate <= 0 || hops <= 0 || want <= 0 {
		return 0
	}
	room := int64(avail)/hops - promptTokens*inRate
	if room <= 0 {
		return 0
	}
	n := min(want, room/outRate)
	if n < max(floor, 1) {
		return 0
	}
	return n
}

// Reservation is an amount held against the cap for one request.
type Reservation struct {
	l      *Ledger
	amount NanoUSD
	done   bool
}

// Reserve holds amount against today's cap, or refuses with a
// *SpendCapError if the committed spend plus everything in flight plus
// amount would exceed it.
func (l *Ledger) Reserve(amount NanoUSD) (*Reservation, error) {
	if amount < 0 {
		return nil, errors.New("ledger: negative reservation")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	st, err := l.readLocked()
	if err != nil {
		return nil, err
	}
	if st.Spent+l.pending+amount > l.cap {
		return nil, &SpendCapError{Day: st.Day, Spent: st.Spent, Pending: l.pending, Need: amount, Cap: l.cap}
	}
	l.pending += amount
	return &Reservation{l: l, amount: amount}, nil
}

// Commit releases the reservation and records what the request actually
// cost. It returns the day's new total. Commit with zero for a request
// that failed before anything was billed. Calling it twice is a no-op.
func (r *Reservation) Commit(actual NanoUSD) (NanoUSD, error) {
	l := r.l
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.done {
		return 0, nil
	}
	r.done = true
	l.pending -= r.amount
	if actual < 0 {
		actual = 0
	}
	return l.addLocked(actual)
}

func (l *Ledger) today() string { return l.now().UTC().Format("2006-01-02") }

// readLocked returns today's state; a previous day's file reads as zero.
func (l *Ledger) readLocked() (ledgerState, error) {
	day := l.today()
	if l.path == "" {
		if l.mem.Day != day {
			l.mem = ledgerState{Day: day}
		}
		return l.mem, nil
	}
	var st ledgerState
	err := l.withFileLock(func() error {
		var rerr error
		st, rerr = l.readFile()
		return rerr
	})
	if err != nil {
		return ledgerState{}, err
	}
	if st.Day != day {
		st = ledgerState{Day: day}
	}
	return st, nil
}

func (l *Ledger) addLocked(actual NanoUSD) (NanoUSD, error) {
	day := l.today()
	if l.path == "" {
		if l.mem.Day != day {
			l.mem = ledgerState{Day: day}
		}
		l.mem.Spent += actual
		l.mem.Requests++
		return l.mem.Spent, nil
	}
	var total NanoUSD
	err := l.withFileLock(func() error {
		st, err := l.readFile()
		if err != nil {
			return err
		}
		if st.Day != day {
			st = ledgerState{Day: day}
		}
		st.Spent += actual
		st.Requests++
		total = st.Spent
		return l.writeFile(st)
	})
	return total, err
}

func (l *Ledger) readFile() (ledgerState, error) {
	b, err := os.ReadFile(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return ledgerState{}, nil
	}
	if err != nil {
		return ledgerState{}, fmt.Errorf("ledger: read %s: %w", l.path, err)
	}
	var st ledgerState
	if err := json.Unmarshal(b, &st); err != nil {
		return ledgerState{}, fmt.Errorf("ledger: %s is corrupt, refusing to guess the day's spend: %w", l.path, err)
	}
	if st.Spent < 0 {
		return ledgerState{}, fmt.Errorf("ledger: %s has a negative total", l.path)
	}
	return st, nil
}

func (l *Ledger) writeFile(st ledgerState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), ".spend-*.json")
	if err != nil {
		return fmt.Errorf("ledger: %w", err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }() // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ledger: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ledger: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("ledger: %w", err)
	}
	if err := os.Rename(name, l.path); err != nil {
		return fmt.Errorf("ledger: %w", err)
	}
	return nil
}

// withFileLock runs fn holding an exclusive flock on path+".lock".
func (l *Ledger) withFileLock(fn func() error) error {
	f, err := os.OpenFile(l.path+".lock", os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- operator-supplied ledger path
	if err != nil {
		return fmt.Errorf("ledger: lock: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil { // #nosec G115 -- fd fits in int
		return fmt.Errorf("ledger: lock: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }() // #nosec G115 -- fd fits in int
	return fn()
}
