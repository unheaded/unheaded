// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package micromanager

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Until 2026-10-23 the store was memory only: a restart lost every task.
func TestPersistence_SurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	a := NewStore()
	if err := a.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	if err := a.Create(NewTask("t-1", "Ship it", "captain")); err != nil {
		t.Fatal(err)
	}
	b := NewStore()
	if err := b.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Get("t-1"); err != nil || got.Title != "Ship it" {
		t.Fatalf("after restart: %+v, %v", got, err)
	}
	if err := b.Delete("t-1"); err != nil {
		t.Fatal(err)
	}
	c := NewStore()
	if err := c.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	if c.Count() != 0 {
		t.Error("delete did not persist")
	}
}

func TestPersistence_WritesRefusedWithout(t *testing.T) {
	s := NewStore()
	api := NewAPI(s, NewService(s, nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"title":"x","owner":"y","priority":3}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.CreateTask(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("POST without persistence: %d, want 503", w.Code)
	}
	if s.Count() != 0 {
		t.Error("write without persistence changed the store")
	}
}

func TestPersistence_FailedSaveIsUndone(t *testing.T) {
	dir := t.TempDir()
	s := NewStore()
	if err := s.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(NewTask("t-1", "Before", "o")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, nil, 0o600); err != nil { // the save's directory is now a file
		t.Fatal(err)
	}
	if err := s.Create(NewTask("t-2", "New", "o")); !errors.Is(err, ErrPersist) {
		t.Errorf("Create: %v, want ErrPersist", err)
	}
	upd, _ := s.GetCopy("t-1")
	upd.Title = "After"
	if err := s.Update(upd); !errors.Is(err, ErrPersist) {
		t.Errorf("Update: %v, want ErrPersist", err)
	}
	if err := s.Delete("t-1"); !errors.Is(err, ErrPersist) {
		t.Errorf("Delete: %v, want ErrPersist", err)
	}
	if got, err := s.Get("t-1"); err != nil || got.Title != "Before" || s.Count() != 1 {
		t.Errorf("failed saves changed memory: %+v %v count=%d", got, err, s.Count())
	}
}

// UpdateTask used to change the stored task in place, so a request whose
// second field was invalid kept its first change.
func TestUpdateTask_InvalidFieldChangesNothing(t *testing.T) {
	s := persistentStore(t)
	if err := s.Create(NewTask("t-1", "Before", "o")); err != nil {
		t.Fatal(err)
	}
	api := NewAPI(s, NewService(s, nil))
	req := httptest.NewRequest(http.MethodPut, "/api/v1/tasks/t-1", strings.NewReader(`{"title":"After","status":"nonsense"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.UpdateTask(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", w.Code)
	}
	if got, _ := s.Get("t-1"); got.Title != "Before" {
		t.Errorf("title = %q after a rejected update, want Before", got.Title)
	}
}

func TestPersistence_DamagedFileRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte(`{"t-1":{"id":"t-2"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewStore().EnablePersistence(dir); err == nil {
		t.Fatal("id-mismatched state file accepted")
	}
}
