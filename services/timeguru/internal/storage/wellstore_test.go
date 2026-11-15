// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package storage

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/database"
	mdparser "unheaded/services/timeguru/internal/parser"
	"unheaded/services/timeguru/internal/timeline"
)

const fixture = `# Timeline
**STATUS:** Alpha Development
### Age 1: The Alpha Ascension (IN PROGRESS)
#### Epoch 1.1: The Whispering Void Awakens
**Risk:** Medium
**Owner:** Agent 5
- [ ] packet_marker.bpf
- [x] Initial research
#### Epoch 1.2: The Cuirass Takes Form
**Risk:** Low
- [x] Design complete
`

// fakeWell is timeline_milestones in memory, with an updated_at per row.
type fakeWell struct {
	mu       sync.Mutex
	rows     map[string]database.MilestoneRow
	replaced int
	now      time.Time
}

func newFakeWell() *fakeWell {
	return &fakeWell{rows: map[string]database.MilestoneRow{}, now: time.Unix(1_700_000_000, 0)}
}

func (f *fakeWell) ReplaceMilestones(_ context.Context, rows []database.MilestoneRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replaced++
	keep := map[string]database.MilestoneRow{}
	for _, r := range rows {
		r.UpdatedAt = f.now
		keep[r.Key] = r
	}
	f.rows = keep
	return nil
}

func (f *fakeWell) UpdateMilestone(_ context.Context, key string, progress int, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[key]
	if !ok {
		return database.ErrMilestoneNotFound
	}
	r.ProgressPct, r.Status, r.UpdatedAt = progress, status, f.now
	f.rows[key] = r
	return nil
}

func (f *fakeWell) NewerThan(_ context.Context, t time.Time) (map[string]database.MilestoneRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]database.MilestoneRow{}
	for k, r := range f.rows {
		if r.UpdatedAt.After(t) {
			out[k] = r
		}
	}
	return out, nil
}

func parseFixture(t *testing.T) *timeline.Timeline {
	t.Helper()
	tl, err := (&mdparser.MarkdownParser{}).ParseContent(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(tl.Milestones) < 2 {
		t.Fatalf("fixture parsed to %d milestones", len(tl.Milestones))
	}
	return tl
}

// The file is the source: loading it writes every milestone to the Well.
func TestFileIsTheSource(t *testing.T) {
	tl := parseFixture(t)
	w := newFakeWell()
	s := New()
	if err := s.Attach(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(context.Background(), tl, w.now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(w.rows) != len(tl.Milestones) {
		t.Fatalf("the Well holds %d rows, the file %d milestones", len(w.rows), len(tl.Milestones))
	}
	for _, m := range tl.Milestones {
		r, ok := w.rows[m.ID]
		if !ok || r.Name != m.Name || r.Status != m.Status || r.ProgressPct != m.Progress || r.Risk != m.Risk {
			t.Errorf("milestone %s: row %+v", m.ID, r)
		}
	}
}

// A row updated through the API after the file's modification time keeps
// its status and progress over the file's at start; an older one takes
// the file's.
func TestUpdateSurvivesRestartWhenNewer(t *testing.T) {
	tl := parseFixture(t)
	newer, older := tl.Milestones[0], tl.Milestones[1]
	w := newFakeWell()
	fileMod := w.now
	w.rows[newer.ID] = database.MilestoneRow{Key: newer.ID, Name: newer.Name, Status: "blocked", ProgressPct: 40, UpdatedAt: fileMod.Add(time.Minute)}
	w.rows[older.ID] = database.MilestoneRow{Key: older.ID, Name: older.Name, Status: "blocked", ProgressPct: 40, UpdatedAt: fileMod.Add(-time.Minute)}

	s := New()
	if err := s.Attach(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	if err := s.Load(context.Background(), tl, fileMod); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTimeline(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	gn, _ := got.GetMilestoneByID(newer.ID)
	if gn.Status != "blocked" || gn.Progress != 40 {
		t.Errorf("newer row: %s %d%%, want the API's blocked 40%%", gn.Status, gn.Progress)
	}
	gold, _ := got.GetMilestoneByID(older.ID)
	if gold.Status != older.Status || gold.Progress != older.Progress {
		t.Errorf("older row: %s %d%%, want the file's %s %d%%", gold.Status, gold.Progress, older.Status, older.Progress)
	}
	if r := w.rows[newer.ID]; r.Status != "blocked" || r.ProgressPct != 40 {
		t.Errorf("the Well lost the newer update: %+v", r)
	}
}

// Without the Well, timeguru serves the parsed file, refuses writes, says
// it is waiting, and connects when the Well comes.
func TestWellDownServesReadOnly(t *testing.T) {
	tl := parseFixture(t)
	s := New()
	if err := s.Load(context.Background(), tl, time.Now()); err != nil {
		t.Fatalf("a file load without the Well: %v", err)
	}
	if got, err := s.GetTimeline(context.Background()); err != nil || len(got.Milestones) != len(tl.Milestones) {
		t.Fatalf("GetTimeline without the Well: %v", err)
	}
	if err := s.UpdateMilestone(context.Background(), tl.Milestones[0].ID, 10, "in_progress"); !errors.Is(err, ErrWellUnavailable) {
		t.Errorf("update without the Well: %v, want ErrWellUnavailable", err)
	}
	if err := s.SaveTimeline(context.Background(), tl); !errors.Is(err, ErrWellUnavailable) {
		t.Errorf("import without the Well: %v, want ErrWellUnavailable", err)
	}
	if st := s.WellState(); st != "waiting" {
		t.Errorf("WellState = %q, want waiting", st)
	}

	w := newFakeWell()
	var mu sync.Mutex
	tries := 0
	connect := func(context.Context) (Backend, error) {
		mu.Lock()
		defer mu.Unlock()
		tries++
		if tries < 3 {
			return nil, errors.New("connection refused")
		}
		return w, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.Connect(ctx, connect, time.Millisecond, 4*time.Millisecond)
	if st := s.WellState(); st != "connected" {
		t.Fatalf("after the third try WellState = %q, want connected", st)
	}
	if len(w.rows) != len(tl.Milestones) {
		t.Errorf("on connecting, the Well got %d rows, want the file's %d", len(w.rows), len(tl.Milestones))
	}
	if err := s.UpdateMilestone(context.Background(), tl.Milestones[0].ID, 10, "in_progress"); err != nil {
		t.Errorf("update once connected: %v", err)
	}
	if err := s.UpdateMilestone(context.Background(), "no-such", 10, "in_progress"); !errors.Is(err, ErrMilestoneNotFound) {
		t.Errorf("unknown milestone: %v, want ErrMilestoneNotFound", err)
	}
}

// Policy: timeguru has one home. No non-test file under services/timeguru
// imports the SQLite driver.
func TestNoSQLite(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == "modernc.org/sqlite" {
				t.Errorf("%s imports modernc.org/sqlite", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
