// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"unheaded/pkg/httputil"
)

const (
	// One request: Vector's batch is 500 rows or 4 MiB, whichever is first.
	maxBodyBytes   = 8 << 20
	maxLineBytes   = 1 << 20
	maxRowsPerPost = 5000
	// Two requests at once, each holding its body and its rows in memory;
	// the unit's memory ceiling is sized for that.
	maxInFlight   = 2
	minTokenBytes = 32
	insertTimeout = 15 * time.Second

	// The table's CHECK constraints (db/migrations/048_ops_log_events.sql).
	maxHost, maxSource, maxUnit, maxLevel, maxEntry = 253, 32, 256, 16, 128
	maxMessage, maxFields                           = 262144, 262144

	// log_events_maintain() keeps partitions for 13 days back to 2 ahead.
	// The writer takes 13 back to 1 ahead, so a row it accepts just after
	// UTC midnight has a partition before the next maintenance call.
	windowBack, windowAhead = 13, 1
)

// Row is one journal entry as Vector sends it (config/vector/vector.yaml,
// transform journal_rows) and as log_events stores it.
type Row struct {
	TS                                                           time.Time
	Host, Source, Unit, Level, Retention, Message, Entry, Fields string
}

// Store is the Well side: one INSERT per batch, and the table's own
// maintenance function.
type Store interface {
	// Insert adds rows and returns how many were new; a row already there
	// (same host, time and entry) is skipped.
	Insert(ctx context.Context, rows []Row) (int64, error)
	// Maintain creates and drops partitions and returns the bytes kept.
	Maintain(ctx context.Context) (int64, error)
}

// Counters are the writer's metrics, as plain functions so tests can count.
type Counters struct {
	Rows     func(outcome string, n int) // inserted, duplicate, rejected
	Rejected func(reason string, n int)  // bad_json, bad_timestamp, out_of_window, bad_value, too_long
	Refused  func(reason string)         // unauthorized, bad_request, too_large, busy, not_ready, full, rate, store
	Insert   func(seconds float64)
}

// Ingest is the POST /ingest handler and the state behind it.
type Ingest struct {
	store    Store
	tokenSum [sha256.Size]byte
	maxRows  int   // rows a clock minute
	maxBytes int64 // the table's ceiling
	now      func() time.Time
	count    Counters

	inFlight chan struct{}

	mu       sync.Mutex
	minute   int64 // Unix minute the row count belongs to
	rowsUsed int

	stored     atomic.Int64 // bytes, from the last Maintain
	maintained atomic.Int64 // Unix seconds of the last good Maintain; 0 is never
}

// NewIngest refuses a short token: an empty or guessable one would leave the
// Well open to anything on the network.
func NewIngest(store Store, token string, maxRows int, maxBytes int64, now func() time.Time, c Counters) (*Ingest, error) {
	if len(token) < minTokenBytes {
		return nil, errors.New("LOGSTORE_INGEST_TOKEN is unset or shorter than 32 bytes; credentials are not stored in this repo")
	}
	if maxRows < 1 || maxBytes < 1 {
		return nil, errors.New("the row cap and the byte ceiling must be positive")
	}
	return &Ingest{
		store: store, tokenSum: sha256.Sum256([]byte(token)), maxRows: maxRows, maxBytes: maxBytes,
		now: now, count: c, inFlight: make(chan struct{}, maxInFlight),
	}, nil
}

// RunMaintenance calls the table's maintenance function and records what it
// returned. A failure keeps the last byte count: behind a running backup
// the call times out on its lock and the next one catches up.
func (in *Ingest) RunMaintenance(ctx context.Context) error {
	n, err := in.store.Maintain(ctx)
	if err != nil {
		return err
	}
	in.stored.Store(n)
	in.maintained.Store(in.now().Unix())
	return nil
}

// StoredBytes is what the last maintenance call returned.
func (in *Ingest) StoredBytes() int64 { return in.stored.Load() }

// LastMaintained is the Unix time of the last good maintenance call, 0 if none.
func (in *Ingest) LastMaintained() int64 { return in.maintained.Load() }

// Ready reports that the partitions exist. Until the first good maintenance call the
// table accepts nothing.
func (in *Ingest) Ready() bool { return in.maintained.Load() != 0 }

// Full reports that the table is over its ceiling.
func (in *Ingest) Full() bool { return in.stored.Load() > in.maxBytes }

func (in *Ingest) authorized(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return false
	}
	sum := sha256.Sum256([]byte(h[len(prefix):]))
	return subtle.ConstantTimeCompare(sum[:], in.tokenSum[:]) == 1
}

// takeRows counts n rows against this clock minute. When they do not fit it
// takes none and returns the seconds until the next minute.
func (in *Ingest) takeRows(n int) (ok bool, retryAfter int) {
	t := in.now()
	m := t.Unix() / 60
	in.mu.Lock()
	defer in.mu.Unlock()
	if m != in.minute {
		in.minute, in.rowsUsed = m, 0
	}
	if in.rowsUsed+n > in.maxRows {
		return false, 60 - int(t.Unix()%60)
	}
	in.rowsUsed += n
	return true, 0
}

func (in *Ingest) refuse(w http.ResponseWriter, status int, reason, msg string, retryAfter int) {
	in.count.Refused(reason)
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	}
	httputil.WriteError(w, status, reason, msg)
}

type ingestReply struct {
	Received  int `json:"received"`
	Inserted  int `json:"inserted"`
	Duplicate int `json:"duplicate"`
	Rejected  int `json:"rejected"`
}

// ServeHTTP takes newline-delimited JSON rows. A row that can never be
// stored is counted and skipped, and the rest of the batch goes in: 200.
// A batch that could be stored later is refused whole, so Vector retries
// it: 429 past the row cap, 503 while the table is over its ceiling, has no
// partitions yet, or the Well refuses the insert.
//
// Nothing here logs a row or a request: this service's own journal lines
// come back through Vector as rows.
func (in *Ingest) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !in.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="logstore"`)
		in.refuse(w, http.StatusUnauthorized, "unauthorized", "bearer token missing or wrong", 0)
		return
	}
	select {
	case in.inFlight <- struct{}{}:
		defer func() { <-in.inFlight }()
	default:
		in.refuse(w, http.StatusTooManyRequests, "busy", "two batches are already being stored", 1)
		return
	}
	if !in.Ready() {
		in.refuse(w, http.StatusServiceUnavailable, "not_ready", "the table has no partitions yet", 30)
		return
	}
	if in.Full() {
		in.refuse(w, http.StatusServiceUnavailable, "full", "the table is over its byte ceiling", 300)
		return
	}

	rows, rejected, lines, err := readRows(http.MaxBytesReader(w, r.Body, maxBodyBytes), in.now())
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) || errors.Is(err, errTooManyRows) {
			in.refuse(w, http.StatusRequestEntityTooLarge, "too_large", "a batch is at most 8 MiB and 5000 rows", 0)
			return
		}
		in.refuse(w, http.StatusBadRequest, "bad_request", "the body could not be read", 0)
		return
	}
	nRejected := 0
	for reason, n := range rejected {
		in.count.Rejected(reason, n)
		nRejected += n
	}
	if nRejected > 0 {
		in.count.Rows("rejected", nRejected)
	}
	reply := ingestReply{Received: lines, Rejected: nRejected}
	if len(rows) == 0 {
		httputil.WriteJSON(w, http.StatusOK, reply)
		return
	}
	if ok, retry := in.takeRows(len(rows)); !ok {
		in.refuse(w, http.StatusTooManyRequests, "rate", "past the row cap for this minute", retry)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), insertTimeout)
	defer cancel()
	start := time.Now()
	inserted, err := in.store.Insert(ctx, rows)
	in.count.Insert(time.Since(start).Seconds())
	if err != nil {
		in.refuse(w, http.StatusServiceUnavailable, "store", "the Well refused the batch; retry", 5)
		return
	}
	reply.Inserted = int(inserted)
	reply.Duplicate = len(rows) - reply.Inserted
	in.count.Rows("inserted", reply.Inserted)
	if reply.Duplicate > 0 {
		in.count.Rows("duplicate", reply.Duplicate)
	}
	httputil.WriteJSON(w, http.StatusOK, reply)
}

var errTooManyRows = errors.New("too many rows in one request")

// wireRow is the JSON Vector sends. fields is JSON text inside a string,
// as ClickHouse holds it; a bare JSON object is taken as its own text.
type wireRow struct {
	Timestamp *string         `json:"timestamp"`
	Host      *string         `json:"host"`
	Source    *string         `json:"source"`
	Unit      *string         `json:"unit"`
	Level     *string         `json:"level"`
	Retention *string         `json:"retention"`
	Message   *string         `json:"message"`
	Entry     *string         `json:"entry"`
	Fields    json.RawMessage `json:"fields"`
}

// readRows parses the body. It returns the rows that can be stored, the
// count of rows that cannot by reason, and the number of non-empty lines.
func readRows(body io.Reader, now time.Time) (rows []Row, rejected map[string]int, lines int, err error) {
	rejected = map[string]int{}
	today := now.UTC().Truncate(24 * time.Hour)
	from := today.AddDate(0, 0, -windowBack)
	until := today.AddDate(0, 0, windowAhead+1)

	br := bufio.NewReaderSize(body, 64<<10)
	for {
		line, readErr := readLine(br)
		if readErr != nil && readErr != io.EOF && !errors.Is(readErr, errLineTooLong) {
			return nil, nil, 0, readErr
		}
		if line = bytes.TrimSpace(line); len(line) > 0 || errors.Is(readErr, errLineTooLong) {
			lines++
			if lines > maxRowsPerPost {
				return nil, nil, 0, errTooManyRows
			}
			if errors.Is(readErr, errLineTooLong) {
				rejected["too_long"]++
			} else if row, reason := parseRow(line, from, until); reason != "" {
				rejected[reason]++
			} else {
				rows = append(rows, row)
			}
		}
		if readErr == io.EOF {
			return rows, rejected, lines, nil
		}
	}
}

var errLineTooLong = errors.New("line too long")

// readLine returns one line without its newline. A line over maxLineBytes
// is read to its end and reported as errLineTooLong, so the lines after it
// still parse.
func readLine(br *bufio.Reader) ([]byte, error) {
	var line []byte
	tooLong := false
	for {
		chunk, err := br.ReadSlice('\n')
		if !tooLong {
			if len(line)+len(chunk) > maxLineBytes {
				tooLong, line = true, nil
			} else {
				line = append(line, chunk...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if tooLong && (err == nil || err == io.EOF) {
			// At EOF the caller sees the long line first, then EOF on its next call.
			return nil, errLineTooLong
		}
		return line, err
	}
}

// parseRow returns the row, or the reason it can never be stored.
func parseRow(line []byte, from, until time.Time) (Row, string) {
	var wr wireRow
	if err := json.Unmarshal(line, &wr); err != nil {
		return Row{}, "bad_json"
	}
	for _, p := range []*string{wr.Timestamp, wr.Host, wr.Source, wr.Unit, wr.Level, wr.Retention, wr.Message, wr.Entry} {
		if p == nil {
			return Row{}, "bad_json"
		}
	}
	var fields string
	switch {
	case len(wr.Fields) > 0 && wr.Fields[0] == '"':
		if err := json.Unmarshal(wr.Fields, &fields); err != nil {
			return Row{}, "bad_json"
		}
	case len(wr.Fields) > 0 && wr.Fields[0] == '{':
		fields = string(wr.Fields)
	default:
		return Row{}, "bad_json"
	}
	ts, err := time.Parse(time.RFC3339Nano, *wr.Timestamp)
	if err != nil {
		return Row{}, "bad_timestamp"
	}
	// Postgres keeps microseconds; rounding here keeps a row sent twice equal to itself.
	ts = ts.UTC().Truncate(time.Microsecond)
	if ts.Before(from) || !ts.Before(until) {
		return Row{}, "out_of_window"
	}
	row := Row{
		TS: ts, Host: clean(*wr.Host), Source: clean(*wr.Source), Unit: clean(*wr.Unit), Level: clean(*wr.Level),
		Retention: *wr.Retention, Message: clean(*wr.Message), Entry: clean(*wr.Entry), Fields: clean(fields),
	}
	if row.Host == "" || row.Entry == "" || (row.Retention != "audit" && row.Retention != "operational") {
		return Row{}, "bad_value"
	}
	if longer(row.Host, maxHost) || longer(row.Source, maxSource) || longer(row.Unit, maxUnit) || longer(row.Level, maxLevel) ||
		longer(row.Entry, maxEntry) || longer(row.Message, maxMessage) || longer(row.Fields, maxFields) {
		return Row{}, "too_long"
	}
	return row, ""
}

// clean makes a string Postgres text will take: valid UTF-8 (encoding/json
// already replaced bad sequences) and no NUL, which text cannot hold.
func clean(s string) string {
	if strings.IndexByte(s, 0) < 0 {
		return s
	}
	return strings.ReplaceAll(s, "\x00", "�")
}

// longer reports whether s has more than n characters, as length() counts.
func longer(s string, n int) bool {
	return len(s) > n && utf8.RuneCountInString(s) > n
}
