// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestConfigWatcher_DetectsNewConfig(t *testing.T) {
	tmpDir := t.TempDir()

	watcher := NewConfigWatcher(tmpDir, 50*time.Millisecond)

	var events []ConfigEvent
	var mu sync.Mutex

	watcher.OnChange(func(e ConfigEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	watcher.Start()
	defer watcher.Stop()

	// Create a valid service config directory
	svcDir := filepath.Join(tmpDir, "test-svc")
	if err := os.MkdirAll(svcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configYAML := `service:
  name: test-svc
  version: "1.0.0"
network:
  port: 19000
  protocol: grpc
deployment:
  tier: presentation
  replicas: 1
runtime:
  binary: test-svc
`
	if err := os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for watcher to pick it up
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Fatal("expected at least one event, got none")
	}

	if events[0].Type != ConfigAdded {
		t.Errorf("expected ConfigAdded, got %v", events[0].Type)
	}
	if events[0].ServiceName != "test-svc" {
		t.Errorf("expected service name test-svc, got %s", events[0].ServiceName)
	}
}

func TestConfigWatcher_DetectsModification(t *testing.T) {
	tmpDir := t.TempDir()

	// Pre-create a config
	svcDir := filepath.Join(tmpDir, "mod-svc")
	if err := os.MkdirAll(svcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	v1 := `service:
  name: mod-svc
  version: "1.0.0"
network:
  port: 19001
  protocol: http
deployment:
  tier: presentation
  replicas: 1
runtime:
  binary: mod-svc
`
	if err := os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	watcher := NewConfigWatcher(tmpDir, 50*time.Millisecond)

	var events []ConfigEvent
	var mu sync.Mutex

	watcher.OnChange(func(e ConfigEvent) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})

	watcher.Start()
	defer watcher.Stop()

	// Wait for initial scan
	time.Sleep(150 * time.Millisecond)

	// Modify the config
	v2 := `service:
  name: mod-svc
  version: "2.0.0"
network:
  port: 19001
  protocol: http
deployment:
  tier: presentation
  replicas: 2
runtime:
  binary: mod-svc
`
	if err := os.WriteFile(filepath.Join(svcDir, "config.yaml"), []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for detection
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, e := range events {
		if e.Type == ConfigModified {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ConfigModified event, got none")
	}
}

func TestConfigEventType_String(t *testing.T) {
	tests := []struct {
		t    ConfigEventType
		want string
	}{
		{ConfigAdded, "added"},
		{ConfigModified, "modified"},
		{ConfigRemoved, "removed"},
		{ConfigEventType(99), "unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.t.String(); got != tt.want {
			t.Errorf("ConfigEventType(%d).String() = %q, want %q", int(tt.t), got, tt.want)
		}
	}
}
