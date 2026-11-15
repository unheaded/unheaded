// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// fakeAide stands in for /usr/bin/aide: it logs its arguments, prints the
// report the test chose, writes a new database on --init and --update, and
// exits with the code the test chose.
const fakeAide = `#!/bin/sh
echo "$*" >>"$FAKE_DIR/calls"
case " $* " in
*" --init "*|*" --update "*) [ -f "$FAKE_DIR/newdb" ] && cp "$FAKE_DIR/newdb" "$FAKE_DIR/db/aide.db.new" ;;
esac
[ -f "$FAKE_DIR/report" ] && cat "$FAKE_DIR/report"
exit "$(cat "$FAKE_DIR/exit" 2>/dev/null || echo 0)"
`

type fixture struct {
	t    *testing.T
	dir  string
	now  time.Time
	read int64
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, dir: t.TempDir(), now: time.Unix(1794700000, 0), read: 123456}
	for _, d := range []string{"db", "state", "textfile"} {
		if err := os.Mkdir(filepath.Join(f.dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.put("aide", fakeAide, 0o755)
	f.put("conf", "# config\n", 0o600)
	t.Setenv("FAKE_DIR", f.dir)
	return f
}

func (f *fixture) put(name, body string, mode os.FileMode) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, name), []byte(body), mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) get(name string) string {
	b, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		return ""
	}
	return string(b)
}

// aide sets what the fake prints and how it exits.
func (f *fixture) aide(report string, exit int) {
	f.put("report", report, 0o644)
	f.put("exit", fmt.Sprint(exit), 0o644)
}

// run runs unheaded-aide with the fixture's paths and returns its exit
// status, stdout and stderr.
func (f *fixture) run(args ...string) (int, string, string) {
	f.t.Helper()
	var out, errw bytes.Buffer
	full := append([]string{"-aide", filepath.Join(f.dir, "aide"), "-config", filepath.Join(f.dir, "conf"),
		"-db-dir", filepath.Join(f.dir, "db"), "-state", filepath.Join(f.dir, "state"),
		"-textfile-dir", filepath.Join(f.dir, "textfile")}, args...)
	code := run(context.Background(), full, &out, &errw, env{
		now:       func() time.Time { f.now = f.now.Add(7 * time.Second); return f.now },
		readBytes: func() int64 { return f.read },
	})
	return code, out.String(), errw.String()
}

func (f *fixture) state() state {
	f.t.Helper()
	var s state
	if err := json.Unmarshal([]byte(f.get("state/state.json")), &s); err != nil {
		f.t.Fatalf("state.json: %v", err)
	}
	return s
}

func (f *fixture) metric(name string) string {
	f.t.Helper()
	m := regexp.MustCompile(`(?m)^` + name + ` (\S+)$`).FindStringSubmatch(f.get("textfile/unheaded_aide.prom"))
	if m == nil {
		f.t.Fatalf("no metric %s in:\n%s", name, f.get("textfile/unheaded_aide.prom"))
	}
	return m[1]
}

func aideJSON(start string, total int, added, removed, changed map[string]string) string {
	r := map[string]any{
		"start_time": start, "aide_version": "0.19.1", "report_level": "list_entries",
		"number_of_entries": map[string]int{"total": total, "added": len(added), "removed": len(removed), "changed": len(changed)},
		"databases":         map[string]any{"/var/lib/aide/aide.db": map[string]string{"sha256": start}},
		"end_time":          start, "run_time_seconds": len(start),
	}
	if len(added)+len(removed)+len(changed) == 0 {
		r["outline"] = "AIDE found NO differences between database and filesystem. Looks okay!!"
		r["number_of_entries"] = map[string]int{"total": total}
	} else {
		r["outline"] = "AIDE found differences between database and filesystem!!"
		r["added"], r["removed"], r["changed"] = added, removed, changed
	}
	b, _ := json.MarshalIndent(r, "", "  ")
	return string(b) + "\n"
}

var (
	diffA = aideJSON("2026-11-15 02:32:00 +0000", 5,
		map[string]string{"/etc/c": "f++++++++++++++++++"}, map[string]string{"/etc/b": "f------------------"},
		map[string]string{"/usr/bin/a": "f > ... mc..H..    ", "/etc": "d = ... mc.. ..    "})
	// The same diff a week later: only times and the run time differ.
	diffALater = aideJSON("2026-11-22 04:10:11 +0000", 5,
		map[string]string{"/etc/c": "f++++++++++++++++++"}, map[string]string{"/etc/b": "f------------------"},
		map[string]string{"/usr/bin/a": "f > ... mc..H..    ", "/etc": "d = ... mc.. ..    "})
	diffB = aideJSON("2026-11-29 04:10:11 +0000", 5, nil, nil, map[string]string{"/usr/bin/a": "f > ... mc..H..    ", "/usr/sbin/x": "f   ... .c.....    "})
	clean = aideJSON("2026-11-15 02:31:50 +0000", 5, nil, nil, nil)
)

func oneLine(t *testing.T, out string) string {
	t.Helper()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Fatalf("stdout is not exactly one line: %q", out)
	}
	return strings.TrimSuffix(out, "\n")
}

func TestCheckClean(t *testing.T) {
	f := newFixture(t)
	f.aide(clean, 0)
	code, out, errw := f.run("check")
	if code != 0 || errw != "" {
		t.Fatalf("exit %d, stderr %q", code, errw)
	}
	if got := oneLine(t, out); got != "aide: clean entries=5 secs=7 read=123456" {
		t.Errorf("line = %q", got)
	}
	if got := strings.TrimSpace(f.get("calls")); got != "--config="+filepath.Join(f.dir, "conf")+" --check" {
		t.Errorf("aide was called with %q", got)
	}
	for name, want := range map[string]string{
		"unheaded_aide_last_run_timestamp_seconds": "1794700014", "unheaded_aide_entries": "5", "unheaded_aide_changed_entries": "0",
		"unheaded_aide_run_seconds": "7", "unheaded_aide_exit_code": "0", "unheaded_aide_new_diff": "0",
	} {
		if got := f.metric(name); got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	if n := strings.Count(f.get("textfile/unheaded_aide.prom"), "\nunheaded_aide_"); n != 6 {
		t.Errorf("%d metrics in the textfile, want 6", n)
	}
	ents, _ := os.ReadDir(filepath.Join(f.dir, "textfile"))
	if len(ents) != 1 {
		t.Errorf("textfile dir holds %d files; a temp file was left", len(ents))
	}
	fi, err := os.Stat(filepath.Join(f.dir, "state", "state.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("state.json: %v, %v; want 0600", err, fi)
	}
	if s := f.state(); s.Pending != nil || s.Entries != 5 || s.LastExit != 0 || s.LastRun != 1794700014 {
		t.Errorf("state = %+v", s)
	}
}

// A new diff is reported once: the same diff on the next run is not new,
// and stays pending with the date it was first seen.
func TestCheckReportsADiffOnce(t *testing.T) {
	f := newFixture(t)
	f.aide(diffA, 7)
	code, out, _ := f.run("check")
	if code != 0 {
		t.Fatalf("a detected change is not an error: exit %d", code)
	}
	line := oneLine(t, out)
	s := f.state()
	if s.Pending == nil || len(s.Pending.Hash) != 64 {
		t.Fatalf("state = %+v", s)
	}
	want := "aide: changed entries=5 added=1 removed=1 changed=2 diff=" + s.Pending.Hash[:12] + " new=1 secs=7 read=123456"
	if line != want {
		t.Errorf("line = %q\nwant   %q", line, want)
	}
	if f.metric("unheaded_aide_new_diff") != "1" || f.metric("unheaded_aide_changed_entries") != "4" || f.metric("unheaded_aide_exit_code") != "7" {
		t.Errorf("metrics:\n%s", f.get("textfile/unheaded_aide.prom"))
	}
	first := *s.Pending
	if len(first.Entries) != 4 || first.Added != 1 || first.Removed != 1 || first.Changed != 2 {
		t.Errorf("pending = %+v", first)
	}

	f.aide(diffALater, 7)
	_, out, _ = f.run("check")
	if line := oneLine(t, out); !strings.Contains(line, " new=0 ") || !strings.Contains(line, "diff="+first.Hash[:12]) {
		t.Errorf("second run of the same diff: %q", line)
	}
	if f.metric("unheaded_aide_new_diff") != "0" {
		t.Error("the same diff alerted twice")
	}
	if s = f.state(); s.Pending == nil || s.Pending.Hash != first.Hash || s.Pending.FirstSeen != first.FirstSeen {
		t.Errorf("pending changed: %+v", s.Pending)
	}

	// A different diff is new.
	f.aide(diffB, 4)
	_, out, _ = f.run("check")
	if line := oneLine(t, out); !strings.Contains(line, " new=1 ") {
		t.Errorf("a different diff: %q", line)
	}
	if s = f.state(); s.Pending.Hash == first.Hash || f.metric("unheaded_aide_new_diff") != "1" {
		t.Error("a different diff kept the old hash")
	}

	// The tree matches the database again: nothing is pending.
	f.aide(clean, 0)
	f.run("check")
	if s = f.state(); s.Pending != nil || f.metric("unheaded_aide_new_diff") != "0" {
		t.Errorf("a clean run left a diff pending: %+v", s.Pending)
	}
}

func TestNormaliseIgnoresTimes(t *testing.T) {
	a, err := parseReport([]byte(diffA))
	if err != nil {
		t.Fatal(err)
	}
	b, err := parseReport([]byte(diffALater))
	if err != nil {
		t.Fatal(err)
	}
	c, err := parseReport([]byte(diffB))
	if err != nil {
		t.Fatal(err)
	}
	if a.hash() != b.hash() {
		t.Error("two reports that differ only in times hash differently")
	}
	if a.hash() == c.hash() {
		t.Error("two different diffs hash the same")
	}
	// A flag is part of the diff; trailing padding is not.
	d, _ := parseReport([]byte(strings.Replace(diffA, "mc..H..    ", "mc..H..", 1)))
	e, _ := parseReport([]byte(strings.Replace(diffA, "mc..H..", "mc.....", 1)))
	if a.hash() != d.hash() || a.hash() == e.hash() {
		t.Error("the hash follows padding, or ignores a changed attribute")
	}
	// An added path cannot be taken for a changed one.
	g, _ := parseReport([]byte(aideJSON("x", 5, map[string]string{"/a": "f"}, nil, nil)))
	h, _ := parseReport([]byte(aideJSON("x", 5, nil, nil, map[string]string{"/a": "f"})))
	if g.hash() == h.hash() {
		t.Error("added and changed hash the same")
	}
}

// An error says so, fails the run, and leaves the state as it was.
func TestCheckErrorsLeaveStateUntouched(t *testing.T) {
	for name, tc := range map[string]struct {
		report string
		exit   int
	}{
		"config error":         {"", 17},
		"io error":             {"", 18},
		"write error":          {"", 14},
		"an undefined code":    {clean, 9},
		"changes with garbage": {"not json", 7},
		"clean with garbage":   {"{", 0},
		"report too large":     {`{"outline":"` + strings.Repeat("x", maxReportBytes) + `"}`, 0},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.aide(diffA, 7)
			f.run("check")
			before := f.get("state/state.json")
			f.aide(tc.report, tc.exit)
			code, out, _ := f.run("check")
			if code != 1 {
				t.Errorf("exit %d, want 1", code)
			}
			if line := oneLine(t, out); !strings.HasPrefix(line, "aide: error exit=") {
				t.Errorf("line = %q", line)
			}
			if f.get("state/state.json") != before {
				t.Error("an error rewrote state.json")
			}
			if f.metric("unheaded_aide_new_diff") != "0" || f.metric("unheaded_aide_exit_code") == "0" {
				t.Errorf("metrics after an error:\n%s", f.get("textfile/unheaded_aide.prom"))
			}
			// The last completed run stays the one before the error.
			if got := f.metric("unheaded_aide_last_run_timestamp_seconds"); got != "1794700014" {
				t.Errorf("last run = %s after an error", got)
			}
		})
	}
	// No aide binary at all.
	f := newFixture(t)
	if err := os.Remove(filepath.Join(f.dir, "aide")); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := f.run("check"); code != 1 || !strings.HasPrefix(oneLine(t, out), "aide: error ") {
		t.Errorf("missing binary: exit %d, %q", code, out)
	}
}

// check reads; it never builds or updates a database.
func TestCheckNeverInitsOrUpdates(t *testing.T) {
	f := newFixture(t)
	for _, r := range []struct {
		report string
		exit   int
	}{{clean, 0}, {diffA, 7}, {diffA, 7}, {"", 17}, {diffB, 4}} {
		f.aide(r.report, r.exit)
		f.run("check")
	}
	for _, l := range strings.Split(strings.TrimSpace(f.get("calls")), "\n") {
		if !strings.HasSuffix(l, " --check") || strings.Contains(l, "init") || strings.Contains(l, "update") {
			t.Errorf("check called aide with %q", l)
		}
	}
	if _, err := os.Stat(filepath.Join(f.dir, "db", "aide.db.new")); err == nil {
		t.Error("check left a new database")
	}
}

func gz(t *testing.T, s string) string {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

const dbText = `@@begin_db
# This file was generated by Aide, version 0.19.1
@@db_spec name perm uid gid size
/usr/bin 40755 0 0 4096
/usr/bin/a 100755 0 0 4
/usr/bin/b 100755 0 0 4
/usr/lib/x/y 100644 0 0 9
/etc 40755 0 0 4096
/etc/passwd 100644 0 0 99
@@end_db
`

func TestInitThenPromote(t *testing.T) {
	f := newFixture(t)
	if code, _, errw := f.run("promote"); code != 1 || !strings.Contains(errw, "no new database") {
		t.Errorf("promote with nothing to promote: exit %d, %q", code, errw)
	}
	f.put("newdb", gz(t, dbText), 0o600)
	f.aide(`{"outline": "AIDE successfully initialized database.", "number_of_entries": {"total": 6}}`+"\n", 0)
	code, out, errw := f.run("init")
	if code != 0 {
		t.Fatalf("init: exit %d, %q", code, errw)
	}
	for _, want := range []string{"entries: 6", "database: ", "aide.db.new", " bytes", "/usr/bin  3", "/etc  2", "/usr/lib  1", "unheaded-aide promote"} {
		if !strings.Contains(out, want) {
			t.Errorf("init summary lacks %q:\n%s", want, out)
		}
	}
	if got := strings.TrimSpace(f.get("calls")); !strings.HasSuffix(got, " --init") {
		t.Errorf("init called aide with %q", got)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "db", "aide.db")); err == nil {
		t.Fatal("init promoted the database itself")
	}
	// A diff found against the old database says nothing about the new one.
	f.put("state/state.json", `{"version":1,"pending":{"hash":"`+strings.Repeat("ab", 32)+`","first_seen":1}}`, 0o600)
	if code, out, errw := f.run("promote"); code != 0 || !strings.Contains(out, "promoted") {
		t.Fatalf("promote: exit %d, %q %q", code, out, errw)
	}
	if got := f.get("db/aide.db"); got != gz(t, dbText) {
		t.Error("aide.db is not the database init wrote")
	}
	if _, err := os.Stat(filepath.Join(f.dir, "db", "aide.db.new")); err == nil {
		t.Error("promote left aide.db.new")
	}
	if f.state().Pending != nil {
		t.Error("promote left a diff pending against the old database")
	}
	// An init that fails promotes nothing and says so.
	f.aide("", 17)
	if code, _, errw := f.run("init"); code != 1 || !strings.Contains(errw, "exit 17") {
		t.Errorf("failed init: exit %d, %q", code, errw)
	}
}

func TestReviewAndAccept(t *testing.T) {
	f := newFixture(t)
	if code, out, _ := f.run("review"); code != 0 || !strings.Contains(out, "no diff is pending") {
		t.Errorf("review with nothing pending: %d %q", code, out)
	}
	f.put("db/aide.db", "old", 0o600)
	f.aide(diffA, 7)
	f.run("check")
	hash := f.state().Pending.Hash
	code, out, _ := f.run("review")
	for _, want := range []string{hash, "added 1, removed 1, changed 2", "added    /etc/c", "removed  /etc/b", "changed  /usr/bin/a", "unheaded-aide accept " + hash} {
		if code != 0 || !strings.Contains(out, want) {
			t.Errorf("review lacks %q:\n%s", want, out)
		}
	}

	before := f.get("state/state.json")
	for _, bad := range []string{strings.Repeat("0", 64), hash[:12], "", "--", hash + "0"} {
		calls := f.get("calls")
		if code, _, errw := f.run("accept", bad); code == 0 || f.get("calls") != calls {
			t.Errorf("accept %q: exit %d, %q; aide was run: %v", bad, code, errw, f.get("calls") != calls)
		}
	}
	if code, _, _ := f.run("accept"); code != 2 {
		t.Errorf("accept with no hash: exit %d, want 2", code)
	}
	if f.get("state/state.json") != before || f.get("db/aide.db") != "old" {
		t.Fatal("a refused accept changed the state or the database")
	}

	// The tree moved on since the review: the update would take in changes
	// nobody read, so it is refused and its database thrown away.
	f.put("newdb", "new", 0o600)
	f.aide(diffB, 4)
	if code, _, errw := f.run("accept", hash); code != 1 || !strings.Contains(errw, "changed since") {
		t.Errorf("accept over a moved tree: exit %d, %q", code, errw)
	}
	if f.get("state/state.json") != before || f.get("db/aide.db") != "old" {
		t.Error("a refused accept changed the state or the database")
	}
	if _, err := os.Stat(filepath.Join(f.dir, "db", "aide.db.new")); err == nil {
		t.Error("a refused accept left aide.db.new")
	}

	f.aide(diffALater, 7)
	code, out, errw := f.run("accept", hash)
	if code != 0 || !strings.Contains(out, "accepted "+hash[:12]) {
		t.Fatalf("accept: exit %d, %q %q", code, out, errw)
	}
	if got := strings.Split(strings.TrimSpace(f.get("calls")), "\n"); !strings.HasSuffix(got[len(got)-1], " --update") {
		t.Errorf("accept called aide with %q", got[len(got)-1])
	}
	s := f.state()
	if s.Pending != nil || len(s.Accepted) != 1 || s.Accepted[0].Hash != hash || f.get("db/aide.db") != "new" {
		t.Errorf("after accept: state %+v, db %q", s, f.get("db/aide.db"))
	}
}

func TestUsage(t *testing.T) {
	f := newFixture(t)
	for _, args := range [][]string{{}, {"frobnicate"}, {"check", "extra"}, {"init", "x"}, {"accept", "a", "b"}} {
		if code, _, errw := f.run(args...); code != 2 || !strings.Contains(errw, "Try 'unheaded-aide --help'") {
			t.Errorf("%v: exit %d, %q", args, code, errw)
		}
	}
	var out, errw bytes.Buffer
	if code := run(context.Background(), []string{"--version"}, &out, &errw, env{}); code != 0 || !strings.HasPrefix(out.String(), "unheaded-aide (unheaded) ") {
		t.Errorf("--version: %d %q", code, out.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"--help"}, &out, &errw, env{}); code != 0 || !strings.Contains(out.String(), "accept <hash>") {
		t.Errorf("--help: %d %q", code, out.String())
	}
}

// Without --textfile-dir the run still reports and keeps its state.
func TestCheckWithoutTextfileDir(t *testing.T) {
	f := newFixture(t)
	f.aide(clean, 0)
	var out, errw bytes.Buffer
	code := run(context.Background(), []string{"-aide", filepath.Join(f.dir, "aide"), "-config", filepath.Join(f.dir, "conf"),
		"-db-dir", filepath.Join(f.dir, "db"), "-state", filepath.Join(f.dir, "state"), "check"}, &out, &errw,
		env{now: time.Now, readBytes: func() int64 { return -1 }})
	if code != 0 || !strings.Contains(out.String(), "read=unknown") {
		t.Errorf("exit %d, %q %q", code, out.String(), errw.String())
	}
	if _, err := os.Stat(filepath.Join(f.dir, "textfile", "unheaded_aide.prom")); err == nil {
		t.Error("wrote metrics nobody asked for")
	}
}
