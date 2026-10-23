// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package zhencapture writes opt-in training captures: one JSON line per
// turn of a local-model or Claude conversation (schema zhen-capture/v1,
// docs/architecture/zhen-capture.md). Every string in a record is passed
// through Redact and cut at a size cap before it is written. The capture
// directory must sit outside any git work tree and is kept 0700; files
// are 0600, appended under flock, one write per record.
package zhencapture

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// Schema is the record's "schema" value. Bump it when a field changes
// meaning; adding an optional field does not need a bump.
const Schema = "zhen-capture/v1"

// DefaultMaxStringBytes caps every string in a record.
const DefaultMaxStringBytes = 64 << 10

// Routes.
const (
	RouteLocal     = "local"     // answered by the local model, accepted
	RouteClaude    = "claude"    // sent to Claude only
	RouteEscalated = "escalated" // local tried, then Claude
)

// Record is one captured turn.
type Record struct {
	Schema     string     `json:"schema"`
	ID         string     `json:"id"`
	Time       time.Time  `json:"ts"`
	Source     string     `json:"source"` // "zhen-claude", "zhen-mcp/local_llm"
	Route      string     `json:"route"`
	System     string     `json:"system,omitempty"`
	Prompt     string     `json:"prompt"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	Local      *Leg       `json:"local,omitempty"`
	Check      *Check     `json:"check,omitempty"`
	Claude     *Leg       `json:"claude,omitempty"`
	Final      string     `json:"final"`
	AcceptedBy string     `json:"accepted_by"` // "local", "claude" or "" when nothing was accepted
	LatencyMS  int64      `json:"latency_ms"`
	CostUSD    string     `json:"cost_usd,omitempty"`
	Redactions int        `json:"redactions"` // set by Write
	Truncated  bool       `json:"truncated"`  // set by Write: some string hit the cap
}

// Leg is one model's attempt.
type Leg struct {
	Model      string `json:"model,omitempty"`
	ServedBy   string `json:"served_by,omitempty"`
	Answer     string `json:"answer,omitempty"`
	Error      string `json:"error,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
	Requests   int    `json:"requests,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
	CostUSD    string `json:"cost_usd,omitempty"`
}

// Check is the acceptance check run on a local answer.
type Check struct {
	Argv      []string `json:"argv"`
	Exit      int      `json:"exit"` // -1 when it did not run to an exit status
	Accepted  bool     `json:"accepted"`
	Error     string   `json:"error,omitempty"`
	LatencyMS int64    `json:"latency_ms"`
}

// ToolCall is one tool call made during the turn.
type ToolCall struct {
	Name     string          `json:"name"`
	Provider string          `json:"provider,omitempty"`
	Decision string          `json:"decision,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Output   string          `json:"output,omitempty"`
	IsError  bool            `json:"is_error"`
}

// Writer appends records to <dir>/zhen-capture-YYYY-MM-DD.jsonl (UTC).
// Safe for concurrent use; several processes may share a directory.
type Writer struct {
	dir            string
	MaxStringBytes int
	now            func() time.Time
	mu             sync.Mutex
}

// Open validates dir and returns a Writer for it. dir is created 0700 if
// missing. It is refused when it (after resolving symlinks) is inside a
// git work tree, when it is not a directory owned by this user, or when
// group or others can access it.
func Open(dir string) (*Writer, error) {
	real, err := CheckDir(dir)
	if err != nil {
		return nil, err
	}
	return &Writer{dir: real, MaxStringBytes: DefaultMaxStringBytes, now: time.Now}, nil
}

// Dir is the resolved capture directory.
func (w *Writer) Dir() string { return w.dir }

// CheckDir applies Open's rules to dir, creating it if needed, and
// returns its resolved path.
func CheckDir(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("zhencapture: empty capture directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("zhencapture: %w", err)
	}
	// Resolve the deepest part that exists; the rest is created below it.
	existing, rest := abs, ""
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("zhencapture: %w", err)
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", fmt.Errorf("zhencapture: no existing ancestor of %s", abs)
		}
		rest = filepath.Join(filepath.Base(existing), rest)
		existing = parent
	}
	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("zhencapture: %w", err)
	}
	if top := gitWorkTree(realExisting); top != "" {
		return "", fmt.Errorf("zhencapture: capture directory %s is inside the git work tree %s; captures hold prompts and answers and must live outside any repository", abs, top)
	}
	real := filepath.Join(realExisting, rest)
	if err := os.MkdirAll(real, 0o700); err != nil {
		return "", fmt.Errorf("zhencapture: %w", err)
	}
	fi, err := os.Lstat(real)
	if err != nil {
		return "", fmt.Errorf("zhencapture: %w", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("zhencapture: %s is not a directory", real)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return "", fmt.Errorf("zhencapture: %s is owned by uid %d, not this user", real, st.Uid)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return "", fmt.Errorf("zhencapture: %s has mode %04o; captures need a private directory (chmod 700 it)", real, perm)
	}
	return real, nil
}

// gitWorkTree returns the nearest ancestor of dir (inclusive) holding a
// .git entry, or "".
func gitWorkTree(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Lstat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// NewID returns a random record id.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Write redacts and caps every string in r, fills Schema, ID, Time,
// Redactions and Truncated, and appends r as one line.
func (w *Writer) Write(r *Record) error {
	r.Schema = Schema
	if r.ID == "" {
		r.ID = NewID()
	}
	if r.Time.IsZero() {
		r.Time = w.now().UTC()
	}
	line, redactions, truncated, err := w.encode(r)
	if err != nil {
		return err
	}
	r.Redactions, r.Truncated = redactions, truncated

	w.mu.Lock()
	defer w.mu.Unlock()
	path := filepath.Join(w.dir, "zhen-capture-"+r.Time.UTC().Format("2006-01-02")+".jsonl")
	// #nosec G304 -- path is the validated capture dir plus a fixed name.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("zhencapture: %w", err)
	}
	defer func() { _ = f.Close() }()
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("zhencapture: %w", err)
	}
	fd := int(f.Fd()) // #nosec G115 -- fd fits in int
	if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
		return fmt.Errorf("zhencapture: lock: %w", err)
	}
	defer func() { _ = syscall.Flock(fd, syscall.LOCK_UN) }()
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("zhencapture: %w", err)
	}
	return nil
}

// encode marshals r with every string value redacted and capped, and
// the redaction count and truncation flag written into the line itself.
func (w *Writer) encode(r *Record) ([]byte, int, bool, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, 0, false, fmt.Errorf("zhencapture: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v map[string]any
	if err := dec.Decode(&v); err != nil {
		return nil, 0, false, fmt.Errorf("zhencapture: %w", err)
	}
	limit := w.MaxStringBytes
	if limit <= 0 {
		limit = DefaultMaxStringBytes
	}
	s := scrubber{limit: limit}
	for k, val := range v {
		if k == "schema" || k == "id" || k == "ts" {
			continue
		}
		v[k] = s.walk(val)
	}
	v["redactions"] = s.redactions
	v["truncated"] = s.truncated
	out, err := json.Marshal(v)
	if err != nil {
		return nil, 0, false, fmt.Errorf("zhencapture: %w", err)
	}
	return append(out, '\n'), s.redactions, s.truncated, nil
}

type scrubber struct {
	limit      int
	redactions int
	truncated  bool
}

func (s *scrubber) walk(v any) any {
	switch x := v.(type) {
	case string:
		red, n := Redact(x)
		s.redactions += n
		cut, t := Truncate(red, s.limit)
		s.truncated = s.truncated || t
		return cut
	case map[string]any:
		for k, sub := range x {
			x[k] = s.walk(sub)
		}
		return x
	case []any:
		for i, sub := range x {
			x[i] = s.walk(sub)
		}
		return x
	default:
		return v
	}
}

// TruncationMarker ends a string Truncate cut.
const TruncationMarker = "\n[truncated by zhencapture]"

// Truncate cuts s to at most n bytes on a rune boundary, marking the cut.
func Truncate(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	cut := n - len(TruncationMarker)
	if cut < 0 {
		cut = 0
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + TruncationMarker, true
}
