// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func putContainer(t *testing.T, d *Daemon, id, body string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	d.handleContainer(rec, httptest.NewRequest(http.MethodPut, "/api/v1/containers/"+id, strings.NewReader(body)))
	return rec.Code
}

// The case the store exists for: desired state must survive a restart, or
// a pruning reconcile deletes every managed container as orphaned.
func TestDesiredState_SurvivesRestartUnderPrune(t *testing.T) {
	dir := t.TempDir()
	d, m := safeDaemon(t, true, true)
	d.authEnabled = true
	d.store = newDesiredStore(dir)
	if code := putContainer(t, d, "web", `{"name":"web","image":"nixos"}`); code != http.StatusOK {
		t.Fatalf("PUT: %d", code)
	}
	d.reconcile() // creates web, managed

	// Restart: a new daemon over the same LXD and state dir.
	d2 := newTestDaemon(t)
	d2.config.Enforce, d2.config.Prune = true, true
	d2.lxdClient = m
	d2.store = newDesiredStore(dir)
	desired, err := d2.store.load()
	if err != nil {
		t.Fatal(err)
	}
	d2.stateManager.desired = desired
	d2.reconcile()
	if _, ok := m.containers["web"]; !ok {
		t.Fatal("restart + prune deleted web, which is still desired")
	}
}

func TestDesiredState_NoStoreRefusesWrites(t *testing.T) {
	d := newTestDaemon(t)
	d.authEnabled = true
	d.store = nil
	if code := putContainer(t, d, "web", `{"image":"nixos"}`); code != http.StatusServiceUnavailable {
		t.Errorf("PUT without a store: %d, want 503", code)
	}
	if len(d.stateManager.desired) != 0 {
		t.Error("desired state changed without a store")
	}
}

func TestDesiredState_FailedSaveChangesNothing(t *testing.T) {
	d := newTestDaemon(t)
	d.authEnabled = true
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	d.store = newDesiredStore(filepath.Join(blocker, "sub")) // MkdirAll fails: a file is in the way
	if code := putContainer(t, d, "web", `{"image":"nixos"}`); code != http.StatusServiceUnavailable {
		t.Errorf("PUT with a failing store: %d, want 503", code)
	}
	if len(d.stateManager.desired) != 0 {
		t.Error("desired state changed although the save failed")
	}
}

func TestDesiredState_LoadRejectsDamage(t *testing.T) {
	for name, content := range map[string]string{
		"not json":    `{"web":`,
		"id mismatch": `{"web":{"id":"db"}}`,
		"bad name":    `{"1web":{"id":"1web"}}`,
		"null entry":  `{"web":null}`,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, desiredFile), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := newDesiredStore(dir).load(); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	got, err := newDesiredStore(t.TempDir()).load()
	if err != nil || len(got) != 0 {
		t.Errorf("missing file: %v, %v; want empty, nil", got, err)
	}
}

func TestDesiredState_FileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	s := newDesiredStore(dir)
	if err := s.save(map[string]*ContainerSpec{"web": {ID: "web"}}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
	left, _ := filepath.Glob(filepath.Join(dir, desiredFile+".*"))
	if len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}
