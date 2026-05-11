// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package watch provides file watching capabilities for hot reload.
package watch

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event represents a file change event.
type Event struct {
	Path      string
	Op        Op
	Timestamp time.Time
}

// Op describes the type of file operation.
type Op uint32

const (
	// Create represents file creation.
	Create Op = 1 << iota
	// Write represents file modification.
	Write
	// Remove represents file deletion.
	Remove
	// Rename represents file renaming.
	Rename
	// Chmod represents permission changes.
	Chmod
)

// String returns a string representation of the operation.
func (op Op) String() string {
	switch op {
	case Create:
		return "CREATE"
	case Write:
		return "WRITE"
	case Remove:
		return "REMOVE"
	case Rename:
		return "RENAME"
	case Chmod:
		return "CHMOD"
	default:
		return "UNKNOWN"
	}
}

// Watcher watches files for changes using polling.
type Watcher struct {
	path      string
	events    chan Event
	errors    chan error
	done      chan struct{}
	interval  time.Duration
	mu        sync.Mutex
	running   bool
	lastInfo  os.FileInfo
	recursive bool
	patterns  []string
}

// NewWatcher creates a new file watcher.
func NewWatcher(path string) (*Watcher, error) {
	return NewWatcherWithOptions(path)
}

// NewWatcherWithOptions creates a new file watcher with options.
func NewWatcherWithOptions(path string, opts ...WatcherOption) (*Watcher, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("absolute path: %w", err)
	}

	w := &Watcher{
		path:     absPath,
		events:   make(chan Event, 100),
		errors:   make(chan error, 10),
		done:     make(chan struct{}),
		interval: 500 * time.Millisecond,
	}

	// Apply options before starting the goroutine.
	for _, opt := range opts {
		opt(w)
	}

	// Get initial file info
	info, err := os.Stat(absPath)
	if err == nil {
		w.lastInfo = info
	}

	go w.watch()

	return w, nil
}

// WatcherOption is a functional option for Watcher.
type WatcherOption func(*Watcher)

// WithInterval sets the polling interval.
func WithInterval(d time.Duration) WatcherOption {
	return func(w *Watcher) {
		w.interval = d
	}
}

// WithRecursive enables recursive directory watching.
func WithRecursive(recursive bool) WatcherOption {
	return func(w *Watcher) {
		w.recursive = recursive
	}
}

// WithPatterns sets file patterns to watch.
func WithPatterns(patterns []string) WatcherOption {
	return func(w *Watcher) {
		w.patterns = patterns
	}
}

// watch polls the file for changes.
func (w *Watcher) watch() {
	w.mu.Lock()
	w.running = true
	w.mu.Unlock()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.checkChanges()
		}
	}
}

// checkChanges checks for file changes.
func (w *Watcher) checkChanges() {
	info, err := os.Stat(w.path)

	if err != nil {
		if os.IsNotExist(err) && w.lastInfo != nil {
			// File was removed
			w.sendEvent(Event{
				Path:      w.path,
				Op:        Remove,
				Timestamp: time.Now(),
			})
			w.lastInfo = nil
		}
		return
	}

	if w.lastInfo == nil {
		// File was created
		w.sendEvent(Event{
			Path:      w.path,
			Op:        Create,
			Timestamp: time.Now(),
		})
		w.lastInfo = info
		return
	}

	// Check for modifications
	if info.ModTime() != w.lastInfo.ModTime() || info.Size() != w.lastInfo.Size() {
		w.sendEvent(Event{
			Path:      w.path,
			Op:        Write,
			Timestamp: time.Now(),
		})
		w.lastInfo = info
		return
	}

	// Check for permission changes
	if info.Mode() != w.lastInfo.Mode() {
		w.sendEvent(Event{
			Path:      w.path,
			Op:        Chmod,
			Timestamp: time.Now(),
		})
		w.lastInfo = info
	}
}

// sendEvent sends an event to the events channel.
func (w *Watcher) sendEvent(e Event) {
	select {
	case w.events <- e:
	default:
		// Channel full, drop event
	}
}

// Events returns the events channel.
func (w *Watcher) Events() <-chan Event {
	return w.events
}

// Errors returns the errors channel.
func (w *Watcher) Errors() <-chan error {
	return w.errors
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	close(w.done)
	w.running = false
	return nil
}

// Add adds a path to watch (for compatibility, stores in patterns).
func (w *Watcher) Add(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	w.mu.Lock()
	w.patterns = append(w.patterns, absPath)
	w.mu.Unlock()

	return nil
}

// Remove removes a path from watching.
func (w *Watcher) Remove(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	newPatterns := make([]string, 0, len(w.patterns))
	for _, p := range w.patterns {
		if p != absPath {
			newPatterns = append(newPatterns, p)
		}
	}
	w.patterns = newPatterns

	return nil
}

// DirectoryWatcher watches a directory for changes.
type DirectoryWatcher struct {
	path      string
	events    chan Event
	errors    chan error
	done      chan struct{}
	interval  time.Duration
	mu        sync.Mutex
	running   bool
	files     map[string]os.FileInfo
	recursive bool
	patterns  []string
	ignores   []string
}

// NewDirectoryWatcher creates a new directory watcher.
func NewDirectoryWatcher(path string) (*DirectoryWatcher, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("absolute path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", absPath)
	}

	w := &DirectoryWatcher{
		path:     absPath,
		events:   make(chan Event, 100),
		errors:   make(chan error, 10),
		done:     make(chan struct{}),
		interval: 500 * time.Millisecond,
		files:    make(map[string]os.FileInfo),
	}

	// Initial scan
	w.scanDirectory()

	go w.watch()

	return w, nil
}

// SetRecursive enables recursive watching.
func (w *DirectoryWatcher) SetRecursive(recursive bool) {
	w.mu.Lock()
	w.recursive = recursive
	w.mu.Unlock()
}

// SetPatterns sets file patterns to watch.
func (w *DirectoryWatcher) SetPatterns(patterns []string) {
	w.mu.Lock()
	w.patterns = patterns
	w.mu.Unlock()
}

// SetIgnores sets patterns to ignore.
func (w *DirectoryWatcher) SetIgnores(ignores []string) {
	w.mu.Lock()
	w.ignores = ignores
	w.mu.Unlock()
}

// scanDirectory scans the directory for files.
func (w *DirectoryWatcher) scanDirectory() {
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip directories if not recursive
		if info.IsDir() && path != w.path && !w.recursive {
			return filepath.SkipDir
		}

		// Check patterns
		if !w.matchesPatterns(path) {
			return nil
		}

		// Check ignores
		if w.isIgnored(path) {
			return nil
		}

		if !info.IsDir() {
			w.files[path] = info
		}

		return nil
	}

	_ = filepath.Walk(w.path, walkFn)
}

// matchesPatterns checks if a path matches the configured patterns.
func (w *DirectoryWatcher) matchesPatterns(path string) bool {
	if len(w.patterns) == 0 {
		return true
	}

	for _, pattern := range w.patterns {
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
	}

	return false
}

// isIgnored checks if a path should be ignored.
func (w *DirectoryWatcher) isIgnored(path string) bool {
	for _, pattern := range w.ignores {
		matched, err := filepath.Match(pattern, filepath.Base(path))
		if err == nil && matched {
			return true
		}
	}
	return false
}

// watch polls the directory for changes.
func (w *DirectoryWatcher) watch() {
	w.mu.Lock()
	w.running = true
	w.mu.Unlock()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.checkChanges()
		}
	}
}

// checkChanges checks for directory changes.
func (w *DirectoryWatcher) checkChanges() {
	w.mu.Lock()
	defer w.mu.Unlock()

	currentFiles := make(map[string]os.FileInfo)

	// Scan current state
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() && path != w.path && !w.recursive {
			return filepath.SkipDir
		}

		if !w.matchesPatterns(path) {
			return nil
		}

		if w.isIgnored(path) {
			return nil
		}

		if !info.IsDir() {
			currentFiles[path] = info
		}

		return nil
	}

	_ = filepath.Walk(w.path, walkFn)

	// Check for new and modified files
	for path, info := range currentFiles {
		oldInfo, exists := w.files[path]
		if !exists {
			w.sendEvent(Event{
				Path:      path,
				Op:        Create,
				Timestamp: time.Now(),
			})
		} else if info.ModTime() != oldInfo.ModTime() || info.Size() != oldInfo.Size() {
			w.sendEvent(Event{
				Path:      path,
				Op:        Write,
				Timestamp: time.Now(),
			})
		}
	}

	// Check for removed files
	for path := range w.files {
		if _, exists := currentFiles[path]; !exists {
			w.sendEvent(Event{
				Path:      path,
				Op:        Remove,
				Timestamp: time.Now(),
			})
		}
	}

	w.files = currentFiles
}

// sendEvent sends an event to the events channel.
func (w *DirectoryWatcher) sendEvent(e Event) {
	select {
	case w.events <- e:
	default:
	}
}

// Events returns the events channel.
func (w *DirectoryWatcher) Events() <-chan Event {
	return w.events
}

// Errors returns the errors channel.
func (w *DirectoryWatcher) Errors() <-chan error {
	return w.errors
}

// Close stops the directory watcher.
func (w *DirectoryWatcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	close(w.done)
	w.running = false
	return nil
}

// MultiWatcher watches multiple paths.
type MultiWatcher struct {
	watchers []*Watcher
	events   chan Event
	errors   chan error
	done     chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewMultiWatcher creates a watcher for multiple paths.
func NewMultiWatcher(paths []string) (*MultiWatcher, error) {
	mw := &MultiWatcher{
		watchers: make([]*Watcher, 0, len(paths)),
		events:   make(chan Event, 100),
		errors:   make(chan error, 10),
		done:     make(chan struct{}),
	}

	for _, path := range paths {
		w, err := NewWatcher(path)
		if err != nil {
			// Close already created watchers
			for _, existing := range mw.watchers {
				_ = existing.Close()
			}
			return nil, fmt.Errorf("watch %s: %w", path, err)
		}
		mw.watchers = append(mw.watchers, w)
	}

	go mw.aggregate()

	return mw, nil
}

// aggregate collects events from all watchers.
func (mw *MultiWatcher) aggregate() {
	mw.mu.Lock()
	mw.running = true
	watchers := make([]*Watcher, len(mw.watchers))
	copy(watchers, mw.watchers)
	mw.mu.Unlock()

	for _, w := range watchers {
		go func(watcher *Watcher) {
			for {
				select {
				case <-mw.done:
					return
				case event := <-watcher.Events():
					select {
					case mw.events <- event:
					default:
					}
				case err := <-watcher.Errors():
					select {
					case mw.errors <- err:
					default:
					}
				}
			}
		}(w)
	}
}

// Events returns the aggregated events channel.
func (mw *MultiWatcher) Events() <-chan Event {
	return mw.events
}

// Errors returns the aggregated errors channel.
func (mw *MultiWatcher) Errors() <-chan error {
	return mw.errors
}

// Close stops all watchers.
func (mw *MultiWatcher) Close() error {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	if !mw.running {
		return nil
	}

	close(mw.done)

	var lastErr error
	for _, w := range mw.watchers {
		if err := w.Close(); err != nil {
			lastErr = err
		}
	}

	mw.running = false
	return lastErr
}

// Add adds a new path to watch.
func (mw *MultiWatcher) Add(path string) error {
	w, err := NewWatcher(path)
	if err != nil {
		return err
	}

	mw.mu.Lock()
	mw.watchers = append(mw.watchers, w)
	mw.mu.Unlock()

	// Start forwarding events
	go func() {
		for {
			select {
			case <-mw.done:
				return
			case event := <-w.Events():
				select {
				case mw.events <- event:
				default:
				}
			case err := <-w.Errors():
				select {
				case mw.errors <- err:
				default:
				}
			}
		}
	}()

	return nil
}
