// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package champion

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
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
