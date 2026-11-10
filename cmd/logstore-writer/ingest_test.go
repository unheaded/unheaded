// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/metrics"
)

const testToken = "test-token-0123456789abcdef0123456789abcdef"

var testNow = time.Date(2026, 11, 10, 12, 0, 30, 0, time.UTC)

// fakeStore keeps rows by primary key, as the table does.
type fakeStore struct {
	mu          sync.Mutex
	rows        map[string]Row
	insertErr   error
	maintainErr error
	bytes       int64
	hold        chan struct{} // when set, Insert waits for it
	entered     chan struct{}
}

func (f *fakeStore) Insert(_ context.Context, rows []Row) (int64, error) {
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.hold != nil {
		<-f.hold
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return 0, f.insertErr
	}
	if f.rows == nil {
		f.rows = map[string]Row{}
	}
	var n int64
	for _, r := range rows {
		k := r.Host + "|" + r.TS.Format(time.RFC3339Nano) + "|" + r.Entry
		if _, dup := f.rows[k]; !dup {
			f.rows[k] = r
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) Maintain(context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bytes, f.maintainErr
}

func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

// tally records what the handler counted.
type tally struct {
	mu                      sync.Mutex
	rows, rejected, refused map[string]int
}

func (c *tally) counters() Counters {
	c.rows, c.rejected, c.refused = map[string]int{}, map[string]int{}, map[string]int{}
	add := func(m map[string]int, k string, n int) {
		c.mu.Lock()
		m[k] += n
		c.mu.Unlock()
	}
	return Counters{
		Rows:     func(o string, n int) { add(c.rows, o, n) },
		Rejected: func(r string, n int) { add(c.rejected, r, n) },
		Refused:  func(r string) { add(c.refused, r, 1) },
		Insert:   func(float64) {},
	}
}

type rig struct {
	in    *Ingest
	store *fakeStore
	tally *tally
	now   time.Time
}

// newRig returns a writer that has run maintenance once, so it is ready.
func newRig(t *testing.T, maxRows int, maxBytes int64) *rig {
	t.Helper()
	r := &rig{store: &fakeStore{}, tally: &tally{}, now: testNow}
	in, err := NewIngest(r.store, testToken, maxRows, maxBytes, func() time.Time { return r.now }, r.tally.counters())
	if err != nil {
		t.Fatal(err)
	}
	if err := in.RunMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.in = in
	return r
}

func (r *rig) post(t *testing.T, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.in.ServeHTTP(w, req)
	return w
}

func reply(t *testing.T, w *httptest.ResponseRecorder) ingestReply {
	t.Helper()
	var got ingestReply
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("reply %q: %v", w.Body.String(), err)
	}
	return got
}

// line is one row as Vector's JSON codec writes it.
func line(ts time.Time, entry, message string) string {
	b, _ := json.Marshal(map[string]string{
		"timestamp": ts.Format(time.RFC3339Nano), "host": "west", "source": "journald", "unit": "sshd.service",
		"level": "info", "retention": "operational", "message": message, "entry": entry, "fields": `{"_PID":"1"}`,
	})
	return string(b)
}

// with returns line() with one key replaced, or removed when v is nil.
func with(t *testing.T, key string, v any) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line(testNow, "e1", "m")), &m); err != nil {
		t.Fatal(err)
	}
	if v == nil {
		delete(m, key)
	} else {
		m[key] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestNewIngest_RefusesAShortToken(t *testing.T) {
	c := (&tally{}).counters()
	for _, tok := range []string{"", "k3y-of-9", strings.Repeat("a", 31)} {
		if _, err := NewIngest(&fakeStore{}, tok, 100, 100, time.Now, c); err == nil {
			t.Errorf("token of %d bytes accepted", len(tok))
		} else if tok != "" && strings.Contains(err.Error(), tok) {
			t.Errorf("the error repeats the token")
		}
	}
	if _, err := NewIngest(&fakeStore{}, testToken, 0, 100, time.Now, c); err == nil {
		t.Error("a row cap of 0 accepted")
	}
	if _, err := NewIngest(&fakeStore{}, testToken, 100, 0, time.Now, c); err == nil {
		t.Error("a byte ceiling of 0 accepted")
	}
}

func TestIngest_NeedsTheToken(t *testing.T) {
	r := newRig(t, 100, 1<<30)
	body := line(testNow, "e1", "m")
	for name, tok := range map[string]string{"none": "", "wrong": strings.Repeat("x", 40), "prefix": testToken[:39], "longer": testToken + "x"} {
		w := r.post(t, tok, body)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, w.Code)
		}
		if strings.Contains(w.Body.String(), testToken) {
			t.Errorf("%s: the reply holds the token", name)
		}
	}
	// Basic credentials are not a bearer token.
	req := httptest.NewRequest(http.MethodPost, "/ingest", strings.NewReader(body))
	req.Header.Set("Authorization", "Basic "+testToken)
	w := httptest.NewRecorder()
	r.in.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("basic: status %d, want 401", w.Code)
	}
	if r.store.count() != 0 {
		t.Fatalf("%d row(s) stored without the token", r.store.count())
	}
	if r.tally.refused["unauthorized"] != 5 {
		t.Errorf("unauthorized counted %d, want 5", r.tally.refused["unauthorized"])
	}
	if w := r.post(t, testToken, body); w.Code != http.StatusOK || r.store.count() != 1 {
		t.Fatalf("with the token: status %d, %d row(s)", w.Code, r.store.count())
	}
}

func TestIngest_StoresABatchAndSkipsWhatIsThere(t *testing.T) {
	r := newRig(t, 100, 1<<30)
	body := line(testNow, "e1", "one") + "\n" + line(testNow, "e2", "two") + "\n\n" + line(testNow.Add(-time.Hour), "e3", "three") + "\n"
	w := r.post(t, testToken, body)
	if got := reply(t, w); w.Code != http.StatusOK || got != (ingestReply{Received: 3, Inserted: 3}) {
		t.Fatalf("first: %d %+v", w.Code, got)
	}
	// Vector sends a batch again after a timeout: nothing is stored twice.
	w = r.post(t, testToken, body+line(testNow, "e4", "four"))
	if got := reply(t, w); w.Code != http.StatusOK || got != (ingestReply{Received: 4, Inserted: 1, Duplicate: 3}) {
		t.Fatalf("second: %d %+v", w.Code, got)
	}
	if r.store.count() != 4 || r.tally.rows["inserted"] != 4 || r.tally.rows["duplicate"] != 3 {
		t.Fatalf("rows %d, counted %v", r.store.count(), r.tally.rows)
	}
	for _, row := range r.store.rows {
		if row.Host != "west" || row.Source != "journald" || row.Unit != "sshd.service" || row.Level != "info" ||
			row.Retention != "operational" || row.Fields != `{"_PID":"1"}` || row.TS.Location() != time.UTC {
			t.Fatalf("row stored as %+v", row)
		}
	}
}

func TestIngest_RejectsRowsThatCanNeverBeStoredAndKeepsTheRest(t *testing.T) {
	long := strings.Repeat("x", 300000)
	cases := []struct{ name, row, reason string }{
		{"not JSON", `{"timestamp":`, "bad_json"},
		{"a JSON array", `[1,2]`, "bad_json"},
		{"no entry key", with(t, "entry", nil), "bad_json"},
		{"no fields key", with(t, "fields", nil), "bad_json"},
		{"fields a number", with(t, "fields", 7), "bad_json"},
		{"message a number", with(t, "message", 7), "bad_json"},
		{"timestamp not a time", with(t, "timestamp", "yesterday"), "bad_timestamp"},
		{"14 days old", with(t, "timestamp", "2026-10-27T23:59:59.999999Z"), "out_of_window"},
		{"2 days ahead", with(t, "timestamp", "2026-11-12T00:00:00Z"), "out_of_window"},
		{"empty host", with(t, "host", ""), "bad_value"},
		{"empty entry", with(t, "entry", ""), "bad_value"},
		{"retention outside the two", with(t, "retention", "forever"), "bad_value"},
		{"host too long", with(t, "host", strings.Repeat("h", 254)), "too_long"},
		{"level too long", with(t, "level", strings.Repeat("l", 17)), "too_long"},
		{"entry too long", with(t, "entry", strings.Repeat("e", 129)), "too_long"},
		{"message too long", with(t, "message", long), "too_long"},
		{"fields too long", with(t, "fields", long), "too_long"},
		{"line over 1 MiB", with(t, "message", strings.Repeat("x", maxLineBytes)), "too_long"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, 100, 1<<30)
			body := line(testNow, "before", "m") + "\n" + c.row + "\n" + line(testNow, "after", "m") + "\n"
			w := r.post(t, testToken, body)
			if got := reply(t, w); w.Code != http.StatusOK || got != (ingestReply{Received: 3, Inserted: 2, Rejected: 1}) {
				t.Fatalf("%d %+v", w.Code, got)
			}
			if r.tally.rejected[c.reason] != 1 || len(r.tally.rejected) != 1 || r.tally.rows["rejected"] != 1 {
				t.Fatalf("rejected counted %v, rows %v; want one %s", r.tally.rejected, r.tally.rows, c.reason)
			}
		})
	}
}

// The writer's window sits inside the partitions log_events_maintain()
// keeps (13 days back to 2 ahead), so an accepted row always has one.
func TestIngest_WindowEdges(t *testing.T) {
	r := newRig(t, 100, 1<<30)
	body := strings.Join([]string{
		with(t, "timestamp", "2026-10-28T00:00:00Z"),           // 13 days back, first instant
		with(t, "timestamp", "2026-11-11T23:59:59.999999Z"),    // tomorrow, last instant
		with(t, "timestamp", "2026-11-10T06:00:00.5-06:00"),    // an offset is the same instant
		with(t, "timestamp", "2026-11-10T12:00:00.123456789Z"), // nanoseconds
	}, "\n")
	w := r.post(t, testToken, body)
	if got := reply(t, w); w.Code != http.StatusOK || got.Inserted != 4 || got.Rejected != 0 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	var micro bool
	for _, row := range r.store.rows {
		if row.TS.Equal(time.Date(2026, 11, 10, 12, 0, 0, 123456000, time.UTC)) {
			micro = true
		}
	}
	if !micro {
		t.Error("nanoseconds were not cut to the microseconds Postgres keeps; a resend would not match its own row")
	}
}

func TestIngest_CleansWhatPostgresTextCannotHold(t *testing.T) {
	r := newRig(t, 100, 1<<30)
	// fields as a bare object is taken as its own text.
	body := with(t, "message", "a\x00b") + "\n" + strings.Replace(line(testNow, "e2", "m"), `"fields":"{\"_PID\":\"1\"}"`, `"fields":{"_PID":"2"}`, 1)
	w := r.post(t, testToken, body)
	if got := reply(t, w); w.Code != http.StatusOK || got.Inserted != 2 {
		t.Fatalf("%d %+v", w.Code, got)
	}
	var sawMessage, sawFields bool
	for _, row := range r.store.rows {
		if strings.ContainsRune(row.Message, 0) {
			t.Fatal("a NUL reached the store")
		}
		sawMessage = sawMessage || row.Message == "a�b"
		sawFields = sawFields || row.Fields == `{"_PID":"2"}`
	}
	if !sawMessage || !sawFields {
		t.Fatalf("rows %+v", r.store.rows)
	}
}

// A message of 262,144 two-byte characters is within the table's CHECK,
// which counts characters.
func TestIngest_LengthCountsCharacters(t *testing.T) {
	from, until := testNow.AddDate(0, 0, -1), testNow.AddDate(0, 0, 1)
	if _, reason := parseRow([]byte(with(t, "unit", strings.Repeat("é", maxUnit))), from, until); reason != "" {
		t.Errorf("%d two-byte characters rejected: %s", maxUnit, reason)
	}
	if _, reason := parseRow([]byte(with(t, "unit", strings.Repeat("é", maxUnit+1))), from, until); reason != "too_long" {
		t.Errorf("%d characters: %q, want too_long", maxUnit+1, reason)
	}
}

func TestIngest_RowCapPerMinute(t *testing.T) {
	r := newRig(t, 5, 1<<30)
	batch := func(prefix string, n int) string {
		var b strings.Builder
		for i := range n {
			b.WriteString(line(r.now, fmt.Sprintf("%s%d", prefix, i), "m") + "\n")
		}
		return b.String()
	}
	if w := r.post(t, testToken, batch("a", 3)); w.Code != http.StatusOK {
		t.Fatalf("first batch: %d", w.Code)
	}
	// Three more would make six: refused whole, so Vector sends it again.
	w := r.post(t, testToken, batch("b", 3))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "30" {
		t.Fatalf("over the cap: %d, Retry-After %q (want 429, 30)", w.Code, w.Header().Get("Retry-After"))
	}
	if r.store.count() != 3 || r.tally.refused["rate"] != 1 {
		t.Fatalf("%d rows, refused %v", r.store.count(), r.tally.refused)
	}
	// A refused batch used none of the minute.
	if w := r.post(t, testToken, batch("c", 2)); w.Code != http.StatusOK || r.store.count() != 5 {
		t.Fatalf("two that fit: %d, %d rows", w.Code, r.store.count())
	}
	r.now = r.now.Add(time.Minute)
	if w := r.post(t, testToken, batch("b", 3)); w.Code != http.StatusOK || r.store.count() != 8 {
		t.Fatalf("next minute: %d, %d rows", w.Code, r.store.count())
	}
	// Rows already there still count: the cap is on what reaches the Well.
	if w := r.post(t, testToken, batch("b", 3)); w.Code != http.StatusTooManyRequests {
		t.Fatalf("a resend past the cap: %d", w.Code)
	}
}

func TestIngest_FullTableRefusesUntilItShrinks(t *testing.T) {
	r := newRig(t, 100, 1000)
	r.store.bytes = 1001
	if err := r.in.RunMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	w := r.post(t, testToken, line(testNow, "e1", "m"))
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" || r.store.count() != 0 {
		t.Fatalf("over the ceiling: %d, %d rows", w.Code, r.store.count())
	}
	if !r.in.Full() || r.tally.refused["full"] != 1 {
		t.Fatalf("full %v, refused %v", r.in.Full(), r.tally.refused)
	}
	r.store.bytes = 1000 // at the ceiling is not over it
	if err := r.in.RunMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w := r.post(t, testToken, line(testNow, "e1", "m")); w.Code != http.StatusOK || r.store.count() != 1 {
		t.Fatalf("after a partition dropped: %d, %d rows", w.Code, r.store.count())
	}
}

func TestIngest_NotReadyUntilMaintenanceSucceeds(t *testing.T) {
	store := &fakeStore{maintainErr: errors.New("lock timeout")}
	tl := &tally{}
	in, err := NewIngest(store, testToken, 100, 1<<30, func() time.Time { return testNow }, tl.counters())
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{in: in, store: store, tally: tl, now: testNow}
	if err := in.RunMaintenance(context.Background()); err == nil || in.Ready() {
		t.Fatalf("a failed call made it ready (err %v)", err)
	}
	if w := r.post(t, testToken, line(testNow, "e1", "m")); w.Code != http.StatusServiceUnavailable || store.count() != 0 {
		t.Fatalf("before the partitions exist: %d, %d rows", w.Code, store.count())
	}
	store.maintainErr, store.bytes = nil, 4096
	if err := in.RunMaintenance(context.Background()); err != nil || !in.Ready() {
		t.Fatalf("ready %v, err %v", in.Ready(), err)
	}
	if in.StoredBytes() != 4096 || in.LastMaintained() != testNow.Unix() {
		t.Fatalf("stored %d, last %d", in.StoredBytes(), in.LastMaintained())
	}
	// A later failure (a backup holds the lock) keeps the last answer.
	store.maintainErr, store.bytes = errors.New("lock timeout"), 0
	if err := in.RunMaintenance(context.Background()); err == nil {
		t.Fatal("the failure was swallowed")
	}
	if !in.Ready() || in.StoredBytes() != 4096 {
		t.Fatalf("ready %v, stored %d after a failed call", in.Ready(), in.StoredBytes())
	}
}

func TestIngest_StoreFailureIsARetryAndSaysNothingOfTheRows(t *testing.T) {
	r := newRig(t, 100, 1<<30)
	r.store.insertErr = errors.New(`pq: duplicate key value violates unique constraint (host, ts, entry)=(west, 2026, SECRET-ROW-TEXT)`)
	w := r.post(t, testToken, line(testNow, "e1", "SECRET-ROW-TEXT"))
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "5" {
		t.Fatalf("%d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
	if strings.Contains(w.Body.String(), "SECRET-ROW-TEXT") || strings.Contains(w.Body.String(), "pq:") {
		t.Fatalf("the reply repeats the store's error: %s", w.Body.String())
	}
	if r.tally.refused["store"] != 1 || r.tally.rows["inserted"] != 0 {
		t.Fatalf("refused %v rows %v", r.tally.refused, r.tally.rows)
	}
}

func TestIngest_BodyAndRowLimits(t *testing.T) {
	r := newRig(t, 100000, 1<<30)
	// Over 8 MiB.
	big := strings.Repeat(line(testNow, "e", strings.Repeat("x", 100000))+"\n", 90)
	if w := r.post(t, testToken, big); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("9 MiB body: %d", w.Code)
	}
	var b strings.Builder
	for i := range maxRowsPerPost + 1 {
		b.WriteString(line(testNow, fmt.Sprintf("e%d", i), "m") + "\n")
	}
	if w := r.post(t, testToken, b.String()); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("%d rows: %d", maxRowsPerPost+1, w.Code)
	}
	if r.store.count() != 0 || r.tally.refused["too_large"] != 2 {
		t.Fatalf("%d rows stored, refused %v", r.store.count(), r.tally.refused)
	}
	// An empty body and a body of blank lines store nothing and are not errors.
	for _, body := range []string{"", "\n\n  \n"} {
		if w := r.post(t, testToken, body); w.Code != http.StatusOK || reply(t, w) != (ingestReply{}) {
			t.Fatalf("body %q: %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestIngest_TwoBatchesAtATime(t *testing.T) {
	r := newRig(t, 100, 1<<30)
	r.store.hold, r.store.entered = make(chan struct{}), make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup
	for i := range maxInFlight {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := r.post(t, testToken, line(testNow, fmt.Sprintf("held%d", i), "m")); w.Code != http.StatusOK {
				t.Errorf("held batch %d: %d", i, w.Code)
			}
		}()
	}
	for range maxInFlight {
		<-r.store.entered
	}
	w := r.post(t, testToken, line(testNow, "third", "m"))
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("third batch: %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
	close(r.store.hold)
	wg.Wait()
	if r.store.count() != maxInFlight || r.tally.refused["busy"] != 1 {
		t.Fatalf("%d rows, refused %v", r.store.count(), r.tally.refused)
	}
}

func TestHandler_ProbesMetricsAndMethods(t *testing.T) {
	store := &fakeStore{maintainErr: errors.New("no partitions")}
	reg := metrics.NewRegistry()
	in, err := NewIngest(store, testToken, 100, 1<<30, func() time.Time { return testNow }, counters(reg))
	if err != nil {
		t.Fatal(err)
	}
	h := handler(in, reg)
	do := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if w := do("GET", "/health", "", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"service":"logstore-writer"`) {
		t.Fatalf("/health: %d %s", w.Code, w.Body.String())
	}
	_ = in.RunMaintenance(context.Background())
	if w := do("GET", "/ready", "", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready before the partitions exist: %d", w.Code)
	}
	store.maintainErr = nil
	if err := in.RunMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w := do("GET", "/ready", "", ""); w.Code != http.StatusOK {
		t.Fatalf("/ready: %d", w.Code)
	}
	if w := do("GET", "/ingest", testToken, ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /ingest: %d", w.Code)
	}
	if w := do("POST", "/ingest", "", line(testNow, "e1", "m")); w.Code != http.StatusUnauthorized {
		t.Fatalf("POST /ingest without a token: %d", w.Code)
	}
	if w := do("POST", "/ingest", testToken, line(testNow, "e1", "m")+"\nnot json\n"); w.Code != http.StatusOK {
		t.Fatalf("POST /ingest: %d %s", w.Code, w.Body.String())
	}
	out := do("GET", "/metrics", "", "").Body.String()
	for _, want := range []string{
		`unheaded_logstore_rows_total{outcome="inserted"} 1`,
		`unheaded_logstore_rows_total{outcome="rejected"} 1`,
		`unheaded_logstore_rejected_rows_total{reason="bad_json"} 1`,
		`unheaded_logstore_refused_requests_total{reason="unauthorized"} 1`,
		`unheaded_logstore_insert_duration_seconds_count 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("/metrics lacks %s", want)
		}
	}
	if strings.Contains(out, testToken) {
		t.Error("/metrics holds the token")
	}
}

// Integration: a throwaway unheaded_ops with migration 048, as log_writer,
// from scripts/well/test-log-events.sh --go. Never the Well. log_writer
// cannot read the rows back, so the counts the statement returns are the
// proof.
func TestIntegrationWellStore(t *testing.T) {
	dsn := os.Getenv("LOGSTORE_TEST_DSN")
	if dsn == "" {
		t.Skip("LOGSTORE_TEST_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	s := wellStore{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := s.Maintain(ctx); err != nil {
		t.Fatalf("maintain: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	mk := func(i int, msg string) Row {
		return Row{TS: now.Add(-time.Duration(i) * time.Second), Host: "qa-go", Source: "journald", Unit: "u.service", Level: "info",
			Retention: "audit", Message: msg, Entry: fmt.Sprintf("go-%d", i), Fields: `{"k":"v, \"quoted\" {braces} \\ back"}`}
	}
	// Text an array literal must carry intact: quotes, commas, braces,
	// backslashes, a newline, NULL as a word, non-ASCII, and 200 kB.
	rows := []Row{mk(1, `plain`), mk(2, `"quoted", {a,b} \ back`), mk(3, "two\nlines"), mk(4, "NULL"), mk(5, "héllo ✓"), mk(6, strings.Repeat("x", 200000)), mk(7, "")}
	n, err := s.Insert(ctx, rows)
	if err != nil || n != int64(len(rows)) {
		t.Fatalf("first insert: %d, %v", n, err)
	}
	if n, err = s.Insert(ctx, rows); err != nil || n != 0 {
		t.Fatalf("the same batch again: %d, %v (want 0)", n, err)
	}
	if n, err = s.Insert(ctx, append(rows[:2:2], mk(8, "new"))); err != nil || n != 1 {
		t.Fatalf("two old rows and one new: %d, %v (want 1)", n, err)
	}
	// A row the table refuses fails the whole batch: the writer's own
	// checks must keep such rows out.
	bad := mk(9, "m")
	bad.Retention = "forever"
	if _, err = s.Insert(ctx, []Row{mk(10, "good"), bad}); err == nil {
		t.Fatal("a retention value outside the two was stored")
	}
	if n, err = s.Insert(ctx, []Row{mk(10, "good")}); err != nil || n != 1 {
		t.Fatalf("the good row of the failed batch was kept: %d, %v", n, err)
	}
	bytes, err := s.Maintain(ctx)
	if err != nil || bytes <= 0 {
		t.Fatalf("maintain after rows: %d, %v", bytes, err)
	}
	// The grants hold through this connection too.
	if _, err := db.ExecContext(ctx, `SELECT count(*) FROM log_events`); err == nil {
		t.Fatal("log_writer read the rows back")
	}
}
