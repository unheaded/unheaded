// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package architect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Until 2026-10-23 architect kept services, nodes and decisions in memory:
// every restart lost them.
func TestPersistence_SurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	a := New()
	if err := a.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := a.AddService(ctx, &Service{ServiceID: "svc-1", Name: "Wotan", Type: "bus", Status: "running", Port: 18000}); err != nil {
		t.Fatal(err)
	}
	if err := a.AddNetworkNode(ctx, &NetworkNode{NodeID: "n-1", Name: "west", Type: "host", CIDR: "10.10.10.0/24"}); err != nil {
		t.Fatal(err)
	}
	if err := a.LogDecision(ctx, &ArchitectureDecision{DecisionID: "ADR-1", Title: "t", Component: "c", Status: "approved", Description: "r"}); err != nil {
		t.Fatal(err)
	}

	b := New()
	if err := b.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := b.GetService(ctx, "svc-1"); err != nil {
		t.Errorf("service lost across restart: %v", err)
	}
	if _, err := b.GetNetworkNode(ctx, "n-1"); err != nil {
		t.Errorf("node lost across restart: %v", err)
	}
	if d, _ := b.ListDecisions(ctx); len(d) != 1 {
		t.Errorf("decisions after restart: %d, want 1", len(d))
	}
}

func TestPersistence_WritesRefusedWithout(t *testing.T) {
	h := NewHTTPHandler(New())
	body, _ := json.Marshal(Service{ServiceID: "svc-1", Name: "x", Type: "y", Status: "running", Port: 1})
	w := httptest.NewRecorder()
	h.AddService(w, jsonPost("/infrastructure/services", body))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("POST without persistence: %d, want 503", w.Code)
	}
	if s, _ := h.service.ListServices(context.Background()); len(s) != 0 {
		t.Error("write without persistence changed state")
	}
}

func TestPersistence_FailedSaveIsUndone(t *testing.T) {
	a := New()
	dir := t.TempDir()
	if err := a.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	// Make the save fail: the state file's directory becomes a file.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHTTPHandler(a)
	body, _ := json.Marshal(Service{ServiceID: "svc-1", Name: "x", Type: "y", Status: "running", Port: 1})
	w := httptest.NewRecorder()
	h.AddService(w, jsonPost("/infrastructure/services", body))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("POST with a failing save: %d, want 503", w.Code)
	}
	if _, err := a.GetService(context.Background(), "svc-1"); err == nil {
		t.Error("failed save left the service in memory")
	}
	if err := a.LogDecision(context.Background(), &ArchitectureDecision{DecisionID: "d", Title: "t", Component: "c", Status: "approved", Description: "r"}); !errors.Is(err, ErrPersist) {
		t.Errorf("LogDecision with a failing save: %v, want ErrPersist", err)
	}
	if d, _ := a.ListDecisions(context.Background()); len(d) != 0 {
		t.Error("failed save left the decision in memory")
	}
}

func TestPersistence_DamagedFileRefused(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, stateFileName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := New().EnablePersistence(dir); err == nil {
		t.Fatal("damaged state file accepted")
	}
	if err := New().EnablePersistence(""); err == nil {
		t.Fatal("empty dir accepted")
	}
}

func jsonPost(path string, body []byte) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}
