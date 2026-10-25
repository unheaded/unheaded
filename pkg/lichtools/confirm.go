// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The out-of-band confirmation flow (S-LICH-AWAKENS Phase 4, Steps 103-108).
//
// A mutating Lich tool (lich_file_finding) is refused pending-confirmation by
// the Champion gate: a model's call carries no justification, so it can never
// auto-approve itself (ADR-104 Rule 2). To let a human approve exactly that one
// call — and nothing the model can forge — the server parks a ticket on disk and
// an operator approves it out of band with `lich-mcp confirm approve <id>`.
//
// The approval is bound to a CallHash over {tool, project root, canonical args}:
// re-issuing the SAME call proceeds; any different call (different args, a
// different tool, a different root) does not inherit the approval. The approval
// code never travels through the model — it is the filesystem the operator owns.
//
// Mirrors sprint/two-brains' hash-bound confirmation (pkg/champion/confirm.go,
// confirmcall.go) but file-backed and local, since the Lich MCP server is a
// one-shot stdio process with no shared database.

// DefaultPendingDir is where tickets live, relative to the project root.
const DefaultPendingDir = "var/lich/pending"

// PendingTTL bounds how long an un-redeemed ticket (pending or approved) is
// honoured. A stale ticket is ignored and swept.
const PendingTTL = 24 * time.Hour

// Ticket states.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusUsed     = "used"
)

// ErrNoSuchTicket is returned by Get/Approve for an unknown id.
var ErrNoSuchTicket = errors.New("lichtools: no such pending ticket")

// ErrNotPending is returned by Approve for a ticket that is not in the pending
// state (already approved, already used, or expired).
var ErrNotPending = errors.New("lichtools: ticket is not pending")

// PendingTicket is one mutating call awaiting out-of-band approval. It records
// enough for an operator to see exactly what they are approving — the tool, a
// short arg summary, and the input hash — but the CallHash is what binds the
// approval, so a reworded re-issue cannot ride an old approval.
type PendingTicket struct {
	ID          string          `json:"id"`
	Tool        string          `json:"tool"`
	Summary     string          `json:"summary"`      // human-facing one-liner, redaction-safe
	InputSHA256 string          `json:"input_sha256"` // sha256 of the raw args bytes
	CallHash    string          `json:"call_hash"`    // binds {tool, root, canonical args}
	Status      string          `json:"status"`
	RequestedAt time.Time       `json:"requested_at"`
	ApprovedAt  *time.Time      `json:"approved_at,omitempty"`
	Args        json.RawMessage `json:"args"`
}

func (t *PendingTicket) expired(now time.Time) bool {
	return now.Sub(t.RequestedAt) > PendingTTL
}

// CallHash binds an approval to one exact call. Args are canonicalised (object
// keys sorted at every depth via encoding/json) so that semantically identical
// input always hashes the same, while any change to the tool, the project root,
// or any argument changes the hash and voids an earlier approval.
func CallHash(tool, projectRoot string, args json.RawMessage) string {
	canon := canonicalJSON(args)
	h := sha256.New()
	// Length-prefixed fields so no concatenation collision is possible.
	writeField(h, tool)
	writeField(h, projectRoot)
	writeField(h, string(canon))
	return hex.EncodeToString(h.Sum(nil))
}

func writeField(h interface{ Write([]byte) (int, error) }, s string) {
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(len(s)))
	_, _ = h.Write(n[:])
	_, _ = h.Write([]byte(s))
}

// canonicalJSON re-encodes v with sorted object keys. A non-object / invalid
// input is returned as-is (trimmed) so the hash is still stable for it.
func canonicalJSON(raw json.RawMessage) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return []byte("null")
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return trimmed
	}
	out, err := json.Marshal(v) // encoding/json sorts map keys at every depth
	if err != nil {
		return trimmed
	}
	return out
}

// sha256Hex is the hash of raw bytes, as the audit records it.
func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// PendingStore is the file-backed ticket store at a 0700 directory the operator
// owns. Authentication is filesystem ownership: only a process running as the
// operator can write an approval. No network, no shared state.
type PendingStore struct {
	dir string
	now func() time.Time
}

// NewPendingStore opens (and creates, 0700) the ticket directory.
func NewPendingStore(dir string) (*PendingStore, error) {
	if dir == "" {
		return nil, errors.New("lichtools: NewPendingStore needs a directory")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("lichtools: pending dir: %w", err)
	}
	return &PendingStore{dir: dir, now: time.Now}, nil
}

// Dir is the ticket directory (read-only accessor).
func (s *PendingStore) Dir() string { return s.dir }

func (s *PendingStore) path(id string) string { return filepath.Join(s.dir, id+".json") }

// freshID returns 16 random hex chars — the handle an operator types.
func freshID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("ticket id entropy: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// Park records a new pending ticket for a refused-pending call, OR returns the
// existing live ticket for the same CallHash (so re-issuing a not-yet-approved
// call does not spawn duplicates). If a live approved ticket already matches,
// that is returned too — the caller (the gate) redeems it.
func (s *PendingStore) Park(tool, summary string, args json.RawMessage, projectRoot string) (*PendingTicket, error) {
	callHash := CallHash(tool, projectRoot, args)
	if t, ok, err := s.findLive(callHash); err != nil {
		return nil, err
	} else if ok {
		return t, nil
	}
	id, err := freshID()
	if err != nil {
		return nil, err
	}
	t := &PendingTicket{
		ID:          id,
		Tool:        tool,
		Summary:     safeSummary(summary),
		InputSHA256: sha256Hex(args),
		CallHash:    callHash,
		Status:      StatusPending,
		RequestedAt: s.now().UTC(),
		Args:        append(json.RawMessage(nil), args...),
	}
	if err := s.write(t); err != nil {
		return nil, err
	}
	return t, nil
}

// FindApproved returns a live, approved, unused ticket for callHash, if one
// exists. The gate calls this to redeem an out-of-band approval.
func (s *PendingStore) FindApproved(callHash string) (*PendingTicket, bool, error) {
	t, ok, err := s.findLive(callHash)
	if err != nil || !ok {
		return nil, false, err
	}
	if t.Status != StatusApproved {
		return nil, false, nil
	}
	return t, true, nil
}

func (s *PendingStore) findLive(callHash string) (*PendingTicket, bool, error) {
	tickets, err := s.List()
	if err != nil {
		return nil, false, err
	}
	now := s.now()
	for _, t := range tickets {
		if t.CallHash == callHash && t.Status != StatusUsed && !t.expired(now) {
			return t, true, nil
		}
	}
	return nil, false, nil
}

// Approve moves a pending ticket to approved. Operator-only (filesystem
// ownership). A ticket that is not pending, or has expired, is refused.
func (s *PendingStore) Approve(id string) (*PendingTicket, error) {
	t, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if t.expired(s.now()) {
		return nil, fmt.Errorf("%w: expired", ErrNotPending)
	}
	if t.Status != StatusPending {
		return nil, ErrNotPending
	}
	now := s.now().UTC()
	t.Status = StatusApproved
	t.ApprovedAt = &now
	if err := s.write(t); err != nil {
		return nil, err
	}
	return t, nil
}

// MarkUsed consumes an approved ticket so it cannot be replayed.
func (s *PendingStore) MarkUsed(id string) error {
	t, err := s.Get(id)
	if err != nil {
		return err
	}
	t.Status = StatusUsed
	return s.write(t)
}

// Get loads one ticket by id.
func (s *PendingStore) Get(id string) (*PendingTicket, error) {
	if !ticketIDRe(id) {
		return nil, ErrNoSuchTicket
	}
	b, err := os.ReadFile(s.path(id)) // #nosec G304 -- id validated to 16 hex chars; path is dir+id
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoSuchTicket
	}
	if err != nil {
		return nil, fmt.Errorf("lichtools: read ticket: %w", err)
	}
	var t PendingTicket
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("lichtools: parse ticket %s: %w", id, err)
	}
	return &t, nil
}

// List returns every ticket in the store, newest first. Expired tickets are
// swept opportunistically.
func (s *PendingStore) List() ([]*PendingTicket, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("lichtools: list tickets: %w", err)
	}
	now := s.now()
	var out []*PendingTicket
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		t, err := s.Get(id)
		if err != nil {
			continue // a malformed file is ignored, not fatal to a listing
		}
		if t.expired(now) {
			_ = os.Remove(s.path(id))
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt.After(out[j].RequestedAt) })
	return out, nil
}

func (s *PendingStore) write(t *PendingTicket) error {
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".ticket-*.json")
	if err != nil {
		return fmt.Errorf("lichtools: ticket: %w", err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }() // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("lichtools: ticket: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("lichtools: ticket: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("lichtools: ticket: %w", err)
	}
	if err := os.Rename(name, s.path(t.ID)); err != nil {
		return fmt.Errorf("lichtools: ticket: %w", err)
	}
	return nil
}

// ticketIDRe accepts exactly 16 lowercase hex chars (freshID's shape). Keeps a
// crafted id from escaping the store directory.
func ticketIDRe(id string) bool {
	if len(id) != 16 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// safeSummary strips control characters and bounds the length of an operator-
// facing summary (the model supplies finding fields; none of it is trusted).
func safeSummary(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	const max = 200
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
