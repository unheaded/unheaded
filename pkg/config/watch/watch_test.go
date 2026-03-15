// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package watch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Op.String
// ---------------------------------------------------------------------------

func TestOpString(t *testing.T) {
	tests := []struct {
		op   Op
		want string
	}{
		{Create, "CREATE"},
		{Write, "WRITE"},
		{Remove, "REMOVE"},
		{Rename, "RENAME"},
		{Chmod, "CHMOD"},
		{Op(0), "UNKNOWN"},
		{Op(255), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("Op(%d).String() = %q, want %q", tt.op, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// DiffType.String
// ---------------------------------------------------------------------------

func TestDiffTypeString(t *testing.T) {
	tests := []struct {
		dt   DiffType
		want string
	}{
		{DiffAdd, "ADD"},
		{DiffModify, "MODIFY"},
		{DiffRemove, "REMOVE"},
		{DiffType(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		if got := tt.dt.String(); got != tt.want {
			t.Errorf("DiffType(%d).String() = %q, want %q", tt.dt, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Watcher
// ---------------------------------------------------------------------------

func TestNewWatcher_ExistingFile(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "cfg.yaml")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(f)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	// Channels should be available.
	if w.Events() == nil {
		t.Error("Events channel is nil")
	}
	if w.Errors() == nil {
		t.Error("Errors channel is nil")
	}
}

func TestNewWatcher_NonExistentFile(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "doesnotexist.yaml")

	// Should still succeed — path is valid, file just doesn't exist yet.
	w, err := NewWatcher(f)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()
}

func TestWatcher_DetectsWrite(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "data.txt")
	if err := os.WriteFile(f, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcherWithOptions(f, WithInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("NewWatcherWithOptions: %v", err)
	}
	defer w.Close()

	// Give watcher a moment to start and capture initial state.
	time.Sleep(50 * time.Millisecond)

	// Modify the file.
	if err := os.WriteFile(f, []byte("v2-longer"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-w.Events():
		if e.Op != Write {
			t.Errorf("expected WRITE event, got %s", e.Op)
		}
		if e.Path == "" {
			t.Error("event path is empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for write event")
	}
}

func TestWatcher_DetectsRemove(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "remove-me.txt")
	if err := os.WriteFile(f, []byte("bye"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcherWithOptions(f, WithInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("NewWatcherWithOptions: %v", err)
	}
	defer w.Close()

	time.Sleep(50 * time.Millisecond)

	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-w.Events():
		if e.Op != Remove {
			t.Errorf("expected REMOVE event, got %s", e.Op)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for remove event")
	}
}

func TestWatcher_DetectsCreate(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "will-appear.txt")

	// Start watching a file that doesn't exist yet.
	w, err := NewWatcherWithOptions(f, WithInterval(20*time.Millisecond))
	if err != nil {
		t.Fatalf("NewWatcherWithOptions: %v", err)
	}
	defer w.Close()

	time.Sleep(50 * time.Millisecond)

	// Create the file.
	if err := os.WriteFile(f, []byte("new!"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-w.Events():
		if e.Op != Create {
			t.Errorf("expected CREATE event, got %s", e.Op)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for create event")
	}
}

func TestWatcher_AddRemovePath(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "main.txt")
	if err := os.WriteFile(f, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(f)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	extra := filepath.Join(tmp, "extra.txt")
	if err := w.Add(extra); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := w.Remove(extra); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestWatcher_CloseIdempotent(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "c.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcher(f)
	if err != nil {
		t.Fatal(err)
	}

	// First close.
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second close should be safe.
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestWatcherOptions(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "opts.txt")
	if err := os.WriteFile(f, []byte("o"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := NewWatcherWithOptions(f,
		WithInterval(100*time.Millisecond),
		WithRecursive(true),
		WithPatterns([]string{"*.yaml", "*.json"}),
	)
	if err != nil {
		t.Fatalf("NewWatcherWithOptions: %v", err)
	}
	defer w.Close()
}

// ---------------------------------------------------------------------------
// DirectoryWatcher
// ---------------------------------------------------------------------------

func TestNewDirectoryWatcher(t *testing.T) {
	tmp := t.TempDir()

	dw, err := NewDirectoryWatcher(tmp)
	if err != nil {
		t.Fatalf("NewDirectoryWatcher: %v", err)
	}
	defer dw.Close()

	if dw.Events() == nil {
		t.Error("Events channel is nil")
	}
	if dw.Errors() == nil {
		t.Error("Errors channel is nil")
	}
}

func TestNewDirectoryWatcher_NotADirectory(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewDirectoryWatcher(f)
	if err == nil {
		t.Fatal("expected error for non-directory path")
	}
}

func TestNewDirectoryWatcher_NonExistent(t *testing.T) {
	_, err := NewDirectoryWatcher("/tmp/nonexistent_watch_test_dir_abc123")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestDirectoryWatcher_DetectsNewFile(t *testing.T) {
	tmp := t.TempDir()

	dw, err := NewDirectoryWatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer dw.Close()

	// Give the watcher time to start.
	time.Sleep(100 * time.Millisecond)

	// Reduce interval for faster detection.
	// The DirectoryWatcher uses a fixed 500ms interval set in NewDirectoryWatcher,
	// but we can still detect creation within a reasonable timeout.

	f := filepath.Join(tmp, "new-file.txt")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-dw.Events():
		if e.Op != Create {
			t.Errorf("expected CREATE, got %s", e.Op)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for directory create event")
	}
}

func TestDirectoryWatcher_DetectsRemoval(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "delete-me.txt")
	if err := os.WriteFile(f, []byte("bye"), 0644); err != nil {
		t.Fatal(err)
	}

	dw, err := NewDirectoryWatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer dw.Close()

	time.Sleep(100 * time.Millisecond)

	if err := os.Remove(f); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-dw.Events():
		if e.Op != Remove {
			t.Errorf("expected REMOVE, got %s", e.Op)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for directory remove event")
	}
}

func TestDirectoryWatcher_SetRecursive(t *testing.T) {
	tmp := t.TempDir()

	dw, err := NewDirectoryWatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer dw.Close()

	dw.SetRecursive(true)
	dw.SetRecursive(false)
}

func TestDirectoryWatcher_SetPatterns(t *testing.T) {
	tmp := t.TempDir()

	dw, err := NewDirectoryWatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer dw.Close()

	dw.SetPatterns([]string{"*.yaml", "*.json"})
}

func TestDirectoryWatcher_SetIgnores(t *testing.T) {
	tmp := t.TempDir()

	dw, err := NewDirectoryWatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer dw.Close()

	dw.SetIgnores([]string{"*.tmp", "*.bak"})
}

func TestDirectoryWatcher_CloseIdempotent(t *testing.T) {
	tmp := t.TempDir()

	dw, err := NewDirectoryWatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}

	if err := dw.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := dw.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestDirectoryWatcher_PatternsFilter(t *testing.T) {
	tmp := t.TempDir()

	// Seed an initial yaml file so it appears in the initial scan.
	if err := os.WriteFile(filepath.Join(tmp, "init.yaml"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	dw, err := NewDirectoryWatcher(tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer dw.Close()

	// Set pattern to only watch .yaml files.
	dw.SetPatterns([]string{"*.yaml"})

	time.Sleep(100 * time.Millisecond)

	// Create a .txt file — should be ignored by pattern filter.
	if err := os.WriteFile(filepath.Join(tmp, "ignored.txt"), []byte("skip"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a .yaml file — should be detected.
	if err := os.WriteFile(filepath.Join(tmp, "detected.yaml"), []byte("hit"), 0644); err != nil {
		t.Fatal(err)
	}

	// We should get a CREATE for the yaml file. The txt may or may not appear
	// depending on timing, but at minimum the yaml should arrive.
	gotYAML := false
	timeout := time.After(3 * time.Second)
	for !gotYAML {
		select {
		case e := <-dw.Events():
			if filepath.Ext(e.Path) == ".yaml" && e.Op == Create {
				gotYAML = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for .yaml create event")
		}
	}
}

// ---------------------------------------------------------------------------
// MultiWatcher
// ---------------------------------------------------------------------------

func TestNewMultiWatcher(t *testing.T) {
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "a.txt")
	f2 := filepath.Join(tmp, "b.txt")
	for _, f := range []string{f1, f2} {
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mw, err := NewMultiWatcher([]string{f1, f2})
	if err != nil {
		t.Fatalf("NewMultiWatcher: %v", err)
	}
	defer mw.Close()

	if mw.Events() == nil {
		t.Error("Events channel is nil")
	}
	if mw.Errors() == nil {
		t.Error("Errors channel is nil")
	}
}

func TestNewMultiWatcher_EmptyPaths(t *testing.T) {
	mw, err := NewMultiWatcher([]string{})
	if err != nil {
		t.Fatalf("NewMultiWatcher with empty paths: %v", err)
	}
	defer mw.Close()
}

func TestMultiWatcher_Add(t *testing.T) {
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "first.txt")
	if err := os.WriteFile(f1, []byte("1"), 0644); err != nil {
		t.Fatal(err)
	}

	mw, err := NewMultiWatcher([]string{f1})
	if err != nil {
		t.Fatal(err)
	}
	defer mw.Close()

	f2 := filepath.Join(tmp, "second.txt")
	if err := os.WriteFile(f2, []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := mw.Add(f2); err != nil {
		t.Fatalf("Add: %v", err)
	}
}

func TestMultiWatcher_CloseIdempotent(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "m.txt")
	if err := os.WriteFile(f, []byte("m"), 0644); err != nil {
		t.Fatal(err)
	}

	mw, err := NewMultiWatcher([]string{f})
	if err != nil {
		t.Fatal(err)
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Notifier
// ---------------------------------------------------------------------------

func TestNewNotifier(t *testing.T) {
	n := NewNotifier()
	if n == nil {
		t.Fatal("NewNotifier returned nil")
	}
	if n.Count() != 0 {
		t.Errorf("expected 0 callbacks, got %d", n.Count())
	}
}

func TestNewNotifierWithDebounce(t *testing.T) {
	n := NewNotifierWithDebounce(200 * time.Millisecond)
	if n == nil {
		t.Fatal("NewNotifierWithDebounce returned nil")
	}
}

func TestNotifier_OnChange(t *testing.T) {
	n := NewNotifierWithDebounce(0) // no debounce for deterministic tests

	var mu sync.Mutex
	var received []Event

	n.OnChange("test-cb", func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	if n.Count() != 1 {
		t.Fatalf("expected 1 callback, got %d", n.Count())
	}

	ev := Event{Path: "/tmp/x", Op: Write, Timestamp: time.Now()}
	n.Notify(ev)

	// Small sleep to let goroutine execute (no debounce, but still async-safe).
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Op != Write {
		t.Errorf("expected WRITE, got %s", received[0].Op)
	}
}

func TestNotifier_OnChangeFiltered(t *testing.T) {
	n := NewNotifierWithDebounce(0)

	var mu sync.Mutex
	var received []Event

	n.OnChangeFiltered("filtered", func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	}, func(e Event) bool {
		return e.Op == Create
	})

	// Should pass the filter.
	n.Notify(Event{Op: Create, Timestamp: time.Now()})
	// Should NOT pass the filter.
	n.Notify(Event{Op: Remove, Timestamp: time.Now()})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(received))
	}
	if received[0].Op != Create {
		t.Errorf("expected CREATE, got %s", received[0].Op)
	}
}

func TestNotifier_OnChangeOnce(t *testing.T) {
	n := NewNotifierWithDebounce(0)

	var mu sync.Mutex
	var count int

	n.OnChangeOnce("once-cb", func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	ev := Event{Op: Write, Timestamp: time.Now()}
	n.Notify(ev)
	time.Sleep(50 * time.Millisecond)
	n.Notify(ev)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 1 {
		t.Errorf("once callback invoked %d times, want 1", count)
	}
}

func TestNotifier_RemoveCallback(t *testing.T) {
	n := NewNotifierWithDebounce(0)
	n.OnChange("to-remove", func(e Event) {})
	if n.Count() != 1 {
		t.Fatal("expected 1")
	}
	n.RemoveCallback("to-remove")
	if n.Count() != 0 {
		t.Errorf("expected 0, got %d", n.Count())
	}
}

func TestNotifier_Clear(t *testing.T) {
	n := NewNotifierWithDebounce(0)
	n.OnChange("a", func(e Event) {})
	n.OnChange("b", func(e Event) {})
	n.Clear()
	if n.Count() != 0 {
		t.Errorf("expected 0 after Clear, got %d", n.Count())
	}
}

func TestNotifier_SetDebounce(t *testing.T) {
	n := NewNotifier()
	n.SetDebounce(500 * time.Millisecond)
	// Just verify it doesn't panic.
}

func TestNotifier_DebounceCoalesces(t *testing.T) {
	n := NewNotifierWithDebounce(100 * time.Millisecond)

	var mu sync.Mutex
	var count int

	n.OnChange("debounce-test", func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	// Fire multiple events rapidly.
	for i := 0; i < 5; i++ {
		n.Notify(Event{Op: Write, Timestamp: time.Now()})
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce to fire.
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	// Debounce should coalesce: only the first event sets the timer, subsequent
	// ones are dropped while pending is true. So we expect exactly 1 invocation.
	if count != 1 {
		t.Errorf("debounce: expected 1 callback invocation, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// ChangeTracker
// ---------------------------------------------------------------------------

func TestNewChangeTracker(t *testing.T) {
	ct := NewChangeTracker(10)
	if ct == nil {
		t.Fatal("NewChangeTracker returned nil")
	}
	if ct.Count() != 0 {
		t.Errorf("expected 0, got %d", ct.Count())
	}
}

func TestChangeTracker_Record(t *testing.T) {
	ct := NewChangeTracker(5)
	ev := Event{Op: Write, Timestamp: time.Now()}
	ct.Record(ev, map[string]any{"key": "val"})

	if ct.Count() != 1 {
		t.Fatalf("expected 1, got %d", ct.Count())
	}

	h := ct.History()
	if len(h) != 1 {
		t.Fatalf("History len %d, want 1", len(h))
	}
	if h[0].Data["key"] != "val" {
		t.Error("data not preserved")
	}
}

func TestChangeTracker_MaxSize(t *testing.T) {
	ct := NewChangeTracker(3)
	for i := 0; i < 10; i++ {
		ct.Record(Event{Op: Write}, map[string]any{"i": i})
	}
	if ct.Count() != 3 {
		t.Errorf("expected 3 (max), got %d", ct.Count())
	}

	h := ct.History()
	// Should contain the last 3 entries (i=7,8,9).
	if h[0].Data["i"] != 7 {
		t.Errorf("expected oldest retained i=7, got %v", h[0].Data["i"])
	}
}

func TestChangeTracker_Since(t *testing.T) {
	ct := NewChangeTracker(10)

	ct.Record(Event{Op: Write}, nil)
	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)
	ct.Record(Event{Op: Create}, nil)

	results := ct.Since(cutoff)
	if len(results) != 1 {
		t.Fatalf("Since: expected 1, got %d", len(results))
	}
}

func TestChangeTracker_Last(t *testing.T) {
	ct := NewChangeTracker(10)
	for i := 0; i < 5; i++ {
		ct.Record(Event{Op: Write}, map[string]any{"i": i})
	}

	last2 := ct.Last(2)
	if len(last2) != 2 {
		t.Fatalf("Last(2) len %d, want 2", len(last2))
	}
	if last2[0].Data["i"] != 3 {
		t.Errorf("expected i=3, got %v", last2[0].Data["i"])
	}

	// Request more than available.
	all := ct.Last(100)
	if len(all) != 5 {
		t.Errorf("Last(100) len %d, want 5", len(all))
	}
}

func TestChangeTracker_Clear(t *testing.T) {
	ct := NewChangeTracker(10)
	ct.Record(Event{Op: Write}, nil)
	ct.Clear()
	if ct.Count() != 0 {
		t.Errorf("expected 0 after Clear, got %d", ct.Count())
	}
}

// ---------------------------------------------------------------------------
// ConfigReloader
// ---------------------------------------------------------------------------

func TestNewConfigReloader(t *testing.T) {
	loader := func() (map[string]any, error) {
		return map[string]any{"env": "test"}, nil
	}

	cr := NewConfigReloader(loader)
	if cr == nil {
		t.Fatal("NewConfigReloader returned nil")
	}

	cur := cr.Current()
	if len(cur) != 0 {
		t.Errorf("expected empty current config before reload, got %v", cur)
	}
}

func TestConfigReloader_Reload(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	loader := func() (map[string]any, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return map[string]any{"version": callCount}, nil
	}

	cr := NewConfigReloader(loader)

	if err := cr.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// Wait for debounced notification.
	time.Sleep(200 * time.Millisecond)

	cur := cr.Current()
	if cur["version"] != 1 {
		t.Errorf("expected version=1, got %v", cur["version"])
	}

	h := cr.History()
	if len(h) != 1 {
		t.Errorf("expected 1 history record, got %d", len(h))
	}
}

func TestConfigReloader_ReloadError(t *testing.T) {
	loader := func() (map[string]any, error) {
		return nil, os.ErrNotExist
	}

	cr := NewConfigReloader(loader)
	if err := cr.Reload(); err == nil {
		t.Error("expected error from Reload")
	}
}

func TestConfigReloader_OnChange(t *testing.T) {
	loader := func() (map[string]any, error) {
		return map[string]any{"k": "v"}, nil
	}

	cr := NewConfigReloader(loader)

	var mu sync.Mutex
	var called bool

	cr.OnChange("reload-cb", func(e Event) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	if err := cr.Reload(); err != nil {
		t.Fatal(err)
	}

	// Wait for debounced notification.
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Error("OnChange callback was not invoked after Reload")
	}
}

// ---------------------------------------------------------------------------
// DiffDetector
// ---------------------------------------------------------------------------

func TestDiffDetector_NoDiff(t *testing.T) {
	dd := NewDiffDetector()
	old := map[string]any{"a": 1, "b": "two"}
	new := map[string]any{"a": 1, "b": "two"}

	diffs := dd.Diff(old, new)
	if len(diffs) != 0 {
		t.Errorf("expected 0 diffs, got %d", len(diffs))
	}
}

func TestDiffDetector_AddedKey(t *testing.T) {
	dd := NewDiffDetector()
	old := map[string]any{"a": 1}
	new := map[string]any{"a": 1, "b": 2}

	diffs := dd.Diff(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Type != DiffAdd {
		t.Errorf("expected ADD, got %s", diffs[0].Type)
	}
	if diffs[0].Key != "b" {
		t.Errorf("expected key 'b', got %q", diffs[0].Key)
	}
}

func TestDiffDetector_RemovedKey(t *testing.T) {
	dd := NewDiffDetector()
	old := map[string]any{"a": 1, "b": 2}
	new := map[string]any{"a": 1}

	diffs := dd.Diff(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Type != DiffRemove {
		t.Errorf("expected REMOVE, got %s", diffs[0].Type)
	}
}

func TestDiffDetector_ModifiedKey(t *testing.T) {
	dd := NewDiffDetector()
	old := map[string]any{"a": 1}
	new := map[string]any{"a": 2}

	diffs := dd.Diff(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Type != DiffModify {
		t.Errorf("expected MODIFY, got %s", diffs[0].Type)
	}
	if diffs[0].OldValue != 1 || diffs[0].NewValue != 2 {
		t.Errorf("unexpected values: old=%v new=%v", diffs[0].OldValue, diffs[0].NewValue)
	}
}

func TestDiffDetector_NestedMaps(t *testing.T) {
	dd := NewDiffDetector()
	old := map[string]any{
		"server": map[string]any{"host": "localhost", "port": 8080},
	}
	new := map[string]any{
		"server": map[string]any{"host": "0.0.0.0", "port": 8080},
	}

	diffs := dd.Diff(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Key != "server.host" {
		t.Errorf("expected key 'server.host', got %q", diffs[0].Key)
	}
	if diffs[0].Type != DiffModify {
		t.Errorf("expected MODIFY, got %s", diffs[0].Type)
	}
}

func TestDiffDetector_KeyFilters(t *testing.T) {
	dd := NewDiffDetector()
	dd.SetKeyFilters([]string{"server"})

	old := map[string]any{
		"server": map[string]any{"host": "localhost"},
		"db":     map[string]any{"url": "old"},
	}
	new := map[string]any{
		"server": map[string]any{"host": "0.0.0.0"},
		"db":     map[string]any{"url": "new"},
	}

	diffs := dd.Diff(old, new)
	// Only server.host should appear; db changes filtered out.
	if len(diffs) != 1 {
		t.Fatalf("expected 1 filtered diff, got %d", len(diffs))
	}
	if diffs[0].Key != "server.host" {
		t.Errorf("expected 'server.host', got %q", diffs[0].Key)
	}
}

func TestDiffDetector_EmptyMaps(t *testing.T) {
	dd := NewDiffDetector()
	diffs := dd.Diff(map[string]any{}, map[string]any{})
	if len(diffs) != 0 {
		t.Errorf("expected 0 diffs for empty maps, got %d", len(diffs))
	}
}

func TestDiffDetector_NilValues(t *testing.T) {
	dd := NewDiffDetector()
	old := map[string]any{"a": nil}
	new := map[string]any{"a": nil}

	diffs := dd.Diff(old, new)
	if len(diffs) != 0 {
		t.Errorf("nil==nil should produce 0 diffs, got %d", len(diffs))
	}

	// nil vs non-nil
	new2 := map[string]any{"a": "value"}
	diffs2 := dd.Diff(old, new2)
	if len(diffs2) != 1 {
		t.Errorf("nil vs value should produce 1 diff, got %d", len(diffs2))
	}
}

func TestDiffDetector_SliceValues(t *testing.T) {
	dd := NewDiffDetector()
	old := map[string]any{"tags": []any{"a", "b"}}
	new := map[string]any{"tags": []any{"a", "c"}}

	diffs := dd.Diff(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff for slice change, got %d", len(diffs))
	}
	if diffs[0].Type != DiffModify {
		t.Errorf("expected MODIFY, got %s", diffs[0].Type)
	}
}

func TestDiffDetector_SliceLengthDifference(t *testing.T) {
	dd := NewDiffDetector()
	old := map[string]any{"items": []any{1, 2}}
	new := map[string]any{"items": []any{1, 2, 3}}

	diffs := dd.Diff(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff for different-length slices, got %d", len(diffs))
	}
}

func TestDiffDetector_TypeChange(t *testing.T) {
	dd := NewDiffDetector()
	// Value changes from a nested map to a scalar.
	old := map[string]any{"x": map[string]any{"nested": true}}
	new := map[string]any{"x": "flat"}

	diffs := dd.Diff(old, new)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff for type change, got %d", len(diffs))
	}
	if diffs[0].Type != DiffModify {
		t.Errorf("expected MODIFY, got %s", diffs[0].Type)
	}
}
