// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package auth

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func writeLines(t *testing.T, f *rotatingFile, from, to int) {
	t.Helper()
	for i := from; i < to; i++ {
		if _, err := fmt.Fprintf(f, "line-%04d\n", i); err != nil {
			t.Fatal(err)
		}
	}
}

// Everything held across the live file and its generations, oldest first.
func heldLines(t *testing.T, path string, keep int) string {
	t.Helper()
	var all bytes.Buffer
	for i := keep; i >= 1; i-- {
		b, err := os.ReadFile(fmt.Sprintf("%s.%d", path, i))
		if err != nil {
			continue
		}
		all.Write(b)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	all.Write(b)
	return all.String()
}

// The trail never outgrows its bound, and what it holds is the newest of it,
// in order, with no line split or lost at a rotation.
func TestRotatingFile_BoundsTheTrailAndKeepsTheNewest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	f, err := openRotatingFile(path, 100, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	writeLines(t, f, 0, 1000)

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("files = %d, want the live one and 3 generations", len(entries))
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > 100 {
			t.Errorf("%s is %d bytes, past the 100 limit", e.Name(), info.Size())
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s has mode %o, want 600", e.Name(), info.Mode().Perm())
		}
	}
	held := heldLines(t, path, 3)
	if !strings.HasSuffix(held, "line-0999\n") {
		t.Fatalf("newest line missing: %q", held)
	}
	lines := strings.Split(strings.TrimSuffix(held, "\n"), "\n")
	var first int
	if _, err := fmt.Sscanf(lines[0], "line-%d", &first); err != nil {
		t.Fatalf("line split at a rotation: %q", lines[0])
	}
	for i, l := range lines {
		if want := fmt.Sprintf("line-%04d", first+i); l != want {
			t.Fatalf("line %d = %q, want %q", i, l, want)
		}
	}
}

// A restart counts what the file already holds.
func TestRotatingFile_CountsAnExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 95), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := openRotatingFile(path, 100, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	writeLines(t, f, 0, 1)
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("no rotation past the limit after a restart: %v", err)
	}
}

// An event larger than the limit is still written, whole.
func TestRotatingFile_OversizeEventIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	f, err := openRotatingFile(path, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	big := strings.Repeat("y", 50) + "\n"
	if _, err := f.Write([]byte(big)); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != big {
		t.Fatalf("file = %q", got)
	}
}

// A rotation that cannot happen loses nothing: the events go on landing in
// the current file, and the trail says it could not rotate.
func TestRotatingFile_FailedRotationKeepsWriting(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	buf := withAuditStderr(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	f, err := openRotatingFile(path, 20, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	writeLines(t, f, 0, 10)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(got), "\n") != 10 {
		t.Fatalf("events lost: %q", got)
	}
	if !strings.Contains(buf.String(), "rotate") {
		t.Fatalf("no word on stderr: %q", buf.String())
	}
	if strings.Count(buf.String(), "\n") > 5 {
		t.Fatalf("a failed rotation retried on every write: %q", buf.String())
	}
}

// fillGenerations writes each generation of path with its own number.
func fillGenerations(t *testing.T, path string, keep int) {
	t.Helper()
	for i := 1; i <= keep; i++ {
		if err := os.WriteFile(fmt.Sprintf("%s.%d", path, i), []byte(fmt.Sprintf("gen-%d\n", i)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func generationsHeld(t *testing.T, path string, keep int) string {
	t.Helper()
	var all strings.Builder
	for i := 1; i <= keep; i++ {
		b, err := os.ReadFile(fmt.Sprintf("%s.%d", path, i))
		if err != nil {
			fmt.Fprintf(&all, "(%d missing)\n", i)
			continue
		}
		all.Write(b)
	}
	return all.String()
}

// A new file that cannot be opened, as when the process is out of file
// descriptors, changes nothing however many times it is tried: the live file
// keeps its name, the events stay in it, and no generation goes. The first
// rotation after it clears drops the one oldest, as any rotation does.
func TestRotatingFile_OpenFailureCostsNoGeneration(t *testing.T) {
	buf := withAuditStderr(t)
	path := filepath.Join(t.TempDir(), "audit.log")
	fillGenerations(t, path, 3)
	f, err := openRotatingFile(path, 20, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fail := true
	open := f.open
	f.open = func(p string) (*os.File, error) {
		if fail {
			return nil, errors.New("too many open files")
		}
		return open(p)
	}
	before := generationsHeld(t, path, 3)

	writeLines(t, f, 0, 20)
	if got := generationsHeld(t, path, 3); got != before {
		t.Fatalf("generations moved by a failed rotation:\n%s\nwant:\n%s", got, before)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("live file gone: %v", err)
	}
	if strings.Count(string(got), "\n") != 20 {
		t.Fatalf("events lost: %q", got)
	}
	if !strings.Contains(buf.String(), "too many open files") {
		t.Fatalf("no word on stderr: %q", buf.String())
	}

	fail = false
	writeLines(t, f, 20, 24)
	if got := heldLines(t, path, 3); !strings.Contains(got, "line-0000\n") || !strings.HasSuffix(got, "line-0023\n") {
		t.Fatalf("recovery lost events: %q", got)
	}
	if _, err := os.Stat(path + ".next"); !os.IsNotExist(err) {
		t.Fatalf("spare file left behind: %v", err)
	}
}

// A shift that stops halfway leaves a gap, and the retry fills it without
// dropping another generation.
func TestRotatingFile_RetryAfterAHalfShiftDropsNothing(t *testing.T) {
	withAuditStderr(t)
	path := filepath.Join(t.TempDir(), "audit.log")
	fillGenerations(t, path, 5)
	f, err := openRotatingFile(path, 20, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// What an interrupted shift leaves: gen-4 has gone over the oldest, gen-3
	// has followed it, and <path>.3 is the gap.
	for _, i := range []int{4, 3} {
		if err := os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1)); err != nil {
			t.Fatal(err)
		}
	}
	writeLines(t, f, 0, 3)
	want := map[string]string{".2": "gen-1\n", ".3": "gen-2\n", ".4": "gen-3\n", ".5": "gen-4\n"}
	for suffix, content := range want {
		if got, _ := os.ReadFile(path + suffix); string(got) != content {
			t.Errorf("%s = %q, want %q", suffix, got, content)
		}
	}
}

// If the live file is taken away, the next rotation puts it back: the trail
// resumes and the older generations stay.
func TestRotatingFile_RemovedLiveFileIsPutBack(t *testing.T) {
	buf := withAuditStderr(t)
	path := filepath.Join(t.TempDir(), "audit.log")
	f, err := openRotatingFile(path, 20, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	writeLines(t, f, 0, 6) // one rotation: <path>.1 exists
	generation, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	writeLines(t, f, 6, 10)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("live file not put back: %v (stderr %q)", err, buf.String())
	}
	if !strings.HasSuffix(string(got), "line-0009\n") {
		t.Fatalf("live file = %q", got)
	}
	if b, _ := os.ReadFile(path + ".2"); string(b) != string(generation) {
		t.Fatalf("generation lost: .2 = %q, want %q", b, generation)
	}
	if buf.Len() != 0 {
		t.Fatalf("a recovered rotation reported a failure: %q", buf.String())
	}
}

// The logger a service gets from AUTH_AUDIT_LOG rotates, at the documented
// limits, and writes nothing a reader of the file could not parse.
func TestNewFileAuditLogger_Rotates(t *testing.T) {
	l, err := NewFileAuditLogger(filepath.Join(t.TempDir(), "audit.log"), "svc")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	f, ok := l.w.(*rotatingFile)
	if !ok {
		t.Fatalf("writer = %T, want *rotatingFile", l.w)
	}
	if f.max != auditRotateBytes || f.keep != auditRotateKeep {
		t.Fatalf("limits = %d, %d", f.max, f.keep)
	}
}

// Every packaged service with auth on can write AUTH_AUDIT_LOG under
// /var/log/unheaded: without it, ProtectSystem=strict makes the log
// unopenable and the service refuses to start.
func TestUnitsGiveTheAuditLogAPath(t *testing.T) {
	units, err := filepath.Glob(filepath.Join("..", "..", "deploy", "systemd", "*.service"))
	if err != nil {
		t.Fatal(err)
	}
	logsDir := regexp.MustCompile(`(?m)^LogsDirectory=unheaded/[a-z0-9-]+$`)
	mode := regexp.MustCompile(`(?m)^LogsDirectoryMode=0750$`)
	found := 0
	for _, u := range units {
		b, err := os.ReadFile(u)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(b, []byte("Environment=AUTH_ENABLED=")) {
			continue
		}
		found++
		if !logsDir.Match(b) || !mode.Match(b) {
			t.Errorf("%s has auth on but no LogsDirectory=unheaded/<name> with mode 0750", filepath.Base(u))
		}
	}
	if found < 10 {
		t.Fatalf("found %d units with auth on; the walk is broken", found)
	}
}
