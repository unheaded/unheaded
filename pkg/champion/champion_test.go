// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package champion

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockStore implements ActionStore for testing.
//
// Thread-safe — `mu` was added 2026-05-04 when TestConcurrentSwapBlocked
// (modelswap_test.go) exposed that the production-side single-flight
// guard works fine but two ModelSwap calls' completeAction calls still
// race the shared mockStore. The mutex protects the slice + counter
// from concurrent reads/writes; serialised access is fine here because
// the fixture is dev-tier mock storage, not a real-DB performance path.
type mockStore struct {
	mu      sync.Mutex
	actions []Action
	nextID  int64
}

func (m *mockStore) LogAction(_ context.Context, action *Action) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	action.ID = m.nextID
	m.actions = append(m.actions, *action)
	return m.nextID, nil
}

func (m *mockStore) UpdateAction(_ context.Context, id int64, status, result, errMsg string, elapsedMs int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.actions {
		if m.actions[i].ID == id {
			m.actions[i].Status = status
			m.actions[i].ResultSummary = result
			m.actions[i].Error = errMsg
			m.actions[i].ElapsedMs = elapsedMs
			return nil
		}
	}
	return nil
}

func (m *mockStore) GetActions(_ context.Context, _ string, _ int) ([]Action, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a defensive copy so callers can iterate without holding the lock
	out := make([]Action, len(m.actions))
	copy(out, m.actions)
	return out, nil
}

func TestNew_RequiresProjectRoot(t *testing.T) {
	_, err := New(Config{}, nil)
	if err == nil {
		t.Fatal("expected error for missing project root")
	}
}

func TestNew_DefaultSessionID(t *testing.T) {
	c, err := New(Config{ProjectRoot: "/tmp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.config.SessionID == "" {
		t.Error("expected default session ID")
	}
}

func TestValidatePath_BlocksTraversal(t *testing.T) {
	c, _ := New(Config{ProjectRoot: "/tmp/test"}, nil)

	tests := []struct {
		path    string
		blocked bool
	}{
		{"/tmp/test/file.txt", false},
		{"/tmp/test/../../../etc/shadow", true},
		{"/tmp/test/subdir/file.go", false},
		{"/etc/shadow", true},
		{"/home/user/.ssh/id_rsa", true},
		{"/tmp/test/.env", true},
		{"/tmp/test/secrets/key", true},
	}

	for _, tt := range tests {
		err := c.validatePath(tt.path)
		if tt.blocked && err == nil {
			t.Errorf("expected %s to be blocked", tt.path)
		}
		if !tt.blocked && err != nil {
			t.Errorf("expected %s to be allowed, got: %v", tt.path, err)
		}
	}
}

func TestValidatePath_AllowedPaths(t *testing.T) {
	c, _ := New(Config{
		ProjectRoot:  "/tmp/test",
		AllowedPaths: []string{"/tmp/test", "/var/zhen"},
	}, nil)

	if err := c.validatePath("/var/zhen/models/test.gguf"); err != nil {
		t.Errorf("expected /var/zhen/ to be allowed: %v", err)
	}
	if err := c.validatePath("/opt/other/file"); err == nil {
		t.Error("expected /opt/other/ to be blocked")
	}
}

func TestReadFile_Success(t *testing.T) {
	// Create temp file
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello kingdom"), 0644)

	store := &mockStore{}
	c, _ := New(Config{
		ProjectRoot:  dir,
		AllowedPaths: []string{dir},
	}, store)

	content, err := c.ReadFile(context.Background(), testFile)
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello kingdom" {
		t.Errorf("expected 'hello kingdom', got %q", content)
	}

	// Verify action was logged
	if len(store.actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(store.actions))
	}
	if store.actions[0].ActionType != "file.read" {
		t.Errorf("expected action type file.read, got %s", store.actions[0].ActionType)
	}
	if store.actions[0].Status != "completed" {
		t.Errorf("expected status completed, got %s", store.actions[0].Status)
	}
}

func TestReadFile_Denied(t *testing.T) {
	store := &mockStore{}
	c, _ := New(Config{
		ProjectRoot:  "/tmp/test",
		AllowedPaths: []string{"/tmp/test"},
	}, store)

	_, err := c.ReadFile(context.Background(), "/etc/shadow")
	if err == nil {
		t.Fatal("expected error reading /etc/shadow")
	}

	// Verify failed action was logged
	if len(store.actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(store.actions))
	}
	if store.actions[0].Status != "failed" {
		t.Errorf("expected status failed, got %s", store.actions[0].Status)
	}
}

func TestReadFile_TraversalAttack(t *testing.T) {
	store := &mockStore{}
	c, _ := New(Config{
		ProjectRoot:  "/tmp/test",
		AllowedPaths: []string{"/tmp/test"},
	}, store)

	_, err := c.ReadFile(context.Background(), "/tmp/test/../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestReadFile_NilStore(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("works without store"), 0644)

	c, _ := New(Config{
		ProjectRoot:  dir,
		AllowedPaths: []string{dir},
	}, nil) // nil store — should still work

	content, err := c.ReadFile(context.Background(), testFile)
	if err != nil {
		t.Fatal(err)
	}
	if content != "works without store" {
		t.Errorf("unexpected content: %q", content)
	}
}

// L7: read_file reads a regular file of at most MaxReadBytes, and refuses
// anything else before it reads a byte past the cap.
// countingZeros never ends, and counts what was taken from it.
type countingZeros struct{ n int64 }

func (z *countingZeros) Read(p []byte) (int, error) {
	if z.n > 1<<20 {
		return 0, errors.New("read far past the limit")
	}
	clear(p)
	z.n += int64(len(p))
	return len(p), nil
}

// A file that grows past what its size said cannot pass the cap: the read
// stops there, whatever is left.
func TestReadUpTo_StopsAtTheLimit(t *testing.T) {
	z := &countingZeros{}
	if _, err := readUpTo(z, 1024); !errors.Is(err, ErrReadTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if z.n != 1025 {
		t.Errorf("took %d bytes for a limit of 1024", z.n)
	}
	if got, err := readUpTo(strings.NewReader("abc"), 3); err != nil || string(got) != "abc" {
		t.Errorf("at the limit: %q, %v", got, err)
	}
}

// L7: what validatePath judged is what gets opened. A directory swapped for a
// link after the checks must not carry the read out of the sandbox.
func TestReadFile_SwapAfterChecksStaysInside(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	c, err := New(Config{ProjectRoot: root}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "notes.txt")
	if err := os.WriteFile(target, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "notes.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := beforeRead
	t.Cleanup(func() { beforeRead = saved })
	beforeRead = func() {
		if err := os.Rename(sub, sub+".old"); err != nil {
			t.Error(err)
		}
		if err := os.Symlink(outside, sub); err != nil {
			t.Error(err)
		}
	}
	if got, err := c.ReadFile(context.Background(), target); err == nil || strings.Contains(got, "outside") {
		t.Fatalf("read through a swapped link: %q, %v", got, err)
	}
}

// A root is opened by the real path the checks found, so a link that has taken
// its place by then is refused the same as one further down.
func TestReadFile_SwappedRootStaysInside(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{ProjectRoot: root, AllowedPaths: []string{root, sub}}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "notes.txt")
	if err := os.WriteFile(target, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "notes.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	saved := beforeRead
	t.Cleanup(func() { beforeRead = saved })
	beforeRead = func() {
		if err := os.Rename(sub, sub+".old"); err != nil {
			t.Error(err)
		}
		if err := os.Symlink(outside, sub); err != nil {
			t.Error(err)
		}
	}
	if got, err := c.ReadFile(context.Background(), target); err == nil || strings.Contains(got, "outside") {
		t.Fatalf("read through a swapped root: %q, %v", got, err)
	}
}

// A link that stays inside the sandbox keeps working: it is judged by where it
// lands, and that real path is what opens.
func TestReadFile_LinkInsideTheSandbox(t *testing.T) {
	root := t.TempDir()
	c, err := New(Config{ProjectRoot: root}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(root, "real.txt")
	if err := os.WriteFile(real, []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ReadFile(context.Background(), link); err != nil || got != "kept" {
		t.Fatalf("link inside the sandbox: %q, %v", got, err)
	}
}

// A sandbox reached through a link of its own is still a sandbox.
func TestReadFile_RootBehindALink(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	if err := os.Mkdir(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "root")
	if err := os.Symlink(realRoot, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "a.txt"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{ProjectRoot: root}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.ReadFile(context.Background(), filepath.Join(root, "a.txt")); err != nil || got != "kept" {
		t.Fatalf("sandbox behind a link: %q, %v", got, err)
	}
}

func TestReadFile_Limits(t *testing.T) {
	root := t.TempDir()
	c, err := New(Config{ProjectRoot: root}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	atCap := filepath.Join(root, "at-cap.txt")
	if err := os.WriteFile(atCap, make([]byte, MaxReadBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ReadFile(ctx, atCap); err != nil || len(got) != MaxReadBytes {
		t.Fatalf("a file at the cap: %d bytes, %v", len(got), err)
	}
	huge := filepath.Join(root, "model.bin")
	f, err := os.Create(huge) // #nosec G304 -- test temp dir
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(9 << 30); err != nil { // sparse: no disk used
		t.Fatal(err)
	}
	_ = f.Close()
	if got, err := c.ReadFile(ctx, huge); !errors.Is(err, ErrReadTooLarge) || got != "" {
		t.Fatalf("a huge file: %d bytes, %v", len(got), err)
	} else if !strings.Contains(err.Error(), strconv.FormatInt(9<<30, 10)) {
		t.Errorf("a huge file was read, not refused on its size: %v", err)
	}
	if _, err := c.ReadFile(ctx, root); err == nil {
		t.Fatal("read a directory")
	}
	fifo := filepath.Join(root, "pipe")
	if out, err := exec.Command("mkfifo", fifo).CombinedOutput(); err != nil {
		t.Skipf("mkfifo: %v %s", err, out)
	}
	done := make(chan error, 1)
	go func() {
		_, err := c.ReadFile(ctx, fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("read a fifo")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a fifo blocked the read")
	}
}
