// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ConfigWatcher polls a service config directory for changes.
// When a config file is added, modified, or removed it invokes registered callbacks.
type ConfigWatcher struct {
	dir      string
	interval time.Duration

	mu        sync.Mutex
	callbacks []func(event ConfigEvent)
	hashes    map[string][32]byte // path → sha256 of last seen content

	cancel context.CancelFunc
}

// ConfigEventType describes what happened to a config file.
type ConfigEventType int

const (
	// ConfigAdded means a new config was discovered.
	ConfigAdded ConfigEventType = iota
	// ConfigModified means an existing config's content changed.
	ConfigModified
	// ConfigRemoved means a previously tracked config was deleted.
	ConfigRemoved
)

// ConfigEvent carries information about a single config change.
type ConfigEvent struct {
	Type        ConfigEventType
	ServiceName string
	Path        string
	Config      *ServiceConfig // nil for ConfigRemoved
}

// NewConfigWatcher creates a watcher that polls dir every interval.
func NewConfigWatcher(dir string, interval time.Duration) *ConfigWatcher {
	return &ConfigWatcher{
		dir:      dir,
		interval: interval,
		hashes:   make(map[string][32]byte),
	}
}

// OnChange registers a callback for config change events.
func (w *ConfigWatcher) OnChange(cb func(ConfigEvent)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, cb)
}

// Start begins polling in a background goroutine.
// Call Stop() to terminate.
func (w *ConfigWatcher) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel

	go w.poll(ctx)
}

// Stop terminates the polling goroutine.
func (w *ConfigWatcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *ConfigWatcher) poll(ctx context.Context) {
	// Do an initial scan immediately.
	w.scan()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.scan()
		}
	}
}

func (w *ConfigWatcher) scan() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return // directory gone or unreadable — skip cycle
	}

	seen := make(map[string]bool)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		configPath := filepath.Join(w.dir, entry.Name(), "config.yaml")
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue // no config.yaml in this subdir
		}

		hash := sha256.Sum256(data)
		seen[configPath] = true

		w.mu.Lock()
		oldHash, exists := w.hashes[configPath]
		w.mu.Unlock()

		if !exists {
			// New config
			cfg, err := LoadServiceConfig(configPath)
			if err != nil {
				continue
			}
			w.mu.Lock()
			w.hashes[configPath] = hash
			w.mu.Unlock()
			w.emit(ConfigEvent{
				Type:        ConfigAdded,
				ServiceName: cfg.Service.Name,
				Path:        configPath,
				Config:      cfg,
			})
		} else if hash != oldHash {
			// Modified config
			cfg, err := LoadServiceConfig(configPath)
			if err != nil {
				continue
			}
			w.mu.Lock()
			w.hashes[configPath] = hash
			w.mu.Unlock()
			w.emit(ConfigEvent{
				Type:        ConfigModified,
				ServiceName: cfg.Service.Name,
				Path:        configPath,
				Config:      cfg,
			})
		}
	}

	// Check for removed configs
	w.mu.Lock()
	for path := range w.hashes {
		if !seen[path] {
			delete(w.hashes, path)
			// Extract service name from path
			svcName := filepath.Base(filepath.Dir(path))
			w.mu.Unlock()
			w.emit(ConfigEvent{
				Type:        ConfigRemoved,
				ServiceName: svcName,
				Path:        path,
			})
			w.mu.Lock()
		}
	}
	w.mu.Unlock()
}

func (w *ConfigWatcher) emit(event ConfigEvent) {
	w.mu.Lock()
	cbs := make([]func(ConfigEvent), len(w.callbacks))
	copy(cbs, w.callbacks)
	w.mu.Unlock()

	for _, cb := range cbs {
		cb(event)
	}
}

// String returns a human-readable description of the event type.
func (t ConfigEventType) String() string {
	switch t {
	case ConfigAdded:
		return "added"
	case ConfigModified:
		return "modified"
	case ConfigRemoved:
		return "removed"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}
