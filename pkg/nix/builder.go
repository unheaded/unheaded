// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package nix provides NixOS container building and management for Unheaded.
// THE FORGE - Where immutable NixOS containers are forged from flake definitions.
package nix

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/lxd"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ============================================================================
// ERRORS - WHEN THE FORGE FAILS
// ============================================================================

var (
	ErrFlakeNotFound      = errors.New("flake.nix not found")
	ErrFlakeParseError    = errors.New("failed to parse flake")
	ErrBuildFailed        = errors.New("nix build failed")
	ErrBuildTimeout       = errors.New("build timed out")
	ErrBuildCancelled     = errors.New("build cancelled")
	ErrImageNotFound      = errors.New("built image not found")
	ErrLXDImportFailed    = errors.New("LXD image import failed")
	ErrBuilderNotRunning  = errors.New("builder not running")
	ErrBuildQueueFull     = errors.New("build queue is full")
	ErrInvalidBuildSpec   = errors.New("invalid build specification")
	ErrCacheCorrupted     = errors.New("build cache corrupted")
	ErrWatcherFailed      = errors.New("flake watcher failed")
)

// ============================================================================
// PROMETHEUS METRICS - OBSERVABILITY
// ============================================================================

var (
	buildsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unheaded_nix_builds_total",
			Help: "Total number of nix builds attempted",
		},
		[]string{"container", "status"},
	)

	buildDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "unheaded_nix_build_duration_seconds",
			Help:    "Duration of nix builds",
			Buckets: []float64{10, 30, 60, 120, 300, 600, 1200},
		},
		[]string{"container"},
	)

	queueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "unheaded_nix_build_queue_depth",
			Help: "Current number of builds in queue",
		},
	)

	cacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "unheaded_nix_cache_hits_total",
			Help: "Total number of build cache hits",
		},
	)

	cacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "unheaded_nix_cache_misses_total",
			Help: "Total number of build cache misses",
		},
	)

	activeBuilds = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "unheaded_nix_active_builds",
			Help: "Number of currently running builds",
		},
	)

	importsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "unheaded_nix_lxd_imports_total",
			Help: "Total number of LXD image imports",
		},
		[]string{"container", "status"},
	)
)

// ============================================================================
// FLAKE CONFIGURATION TYPES
// ============================================================================

// FlakeConfig represents a parsed flake.nix configuration for a container.
type FlakeConfig struct {
	// Path to the flake directory
	Path string `json:"path"`

	// Description from flake
	Description string `json:"description"`

	// Inputs defined in the flake
	Inputs []FlakeInput `json:"inputs"`

	// Outputs available (containers, packages, etc.)
	Outputs []FlakeOutput `json:"outputs"`

	// NixOS configuration modules
	Modules []string `json:"modules"`

	// System architecture
	System string `json:"system"`

	// Lock file hash for change detection
	LockHash string `json:"lock_hash"`

	// Last modification time
	ModTime time.Time `json:"mod_time"`

	// Raw flake metadata from nix flake metadata
	Metadata map[string]interface{} `json:"metadata"`
}

// FlakeInput represents an input dependency in a flake.
type FlakeInput struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Locked bool   `json:"locked"`
	Rev    string `json:"rev,omitempty"`
}

// FlakeOutput represents an available output from a flake.
type FlakeOutput struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // nixosConfiguration, container, package
	System      string `json:"system"`
	Description string `json:"description,omitempty"`
}

// ============================================================================
// BUILD SPECIFICATION TYPES
// ============================================================================

// BuildSpec defines what to build.
type BuildSpec struct {
	// Unique build ID
	ID string `json:"id"`

	// Path to the flake
	FlakePath string `json:"flake_path"`

	// Output attribute to build (e.g., "nixosConfigurations.timeguru")
	Attribute string `json:"attribute"`

	// Container name for the resulting image
	ContainerName string `json:"container_name"`

	// System architecture (default: x86_64-linux)
	System string `json:"system"`

	// Build options
	Options BuildOptions `json:"options"`

	// Priority (higher = more urgent)
	Priority int `json:"priority"`

	// Requester information
	RequestedBy string    `json:"requested_by"`
	RequestedAt time.Time `json:"requested_at"`

	// Callback URL for build completion (optional)
	CallbackURL string `json:"callback_url,omitempty"`
}

// BuildOptions contains optional build configuration.
type BuildOptions struct {
	// Maximum build time
	Timeout time.Duration `json:"timeout"`

	// Number of CPU cores to use
	MaxCores int `json:"max_cores"`

	// Maximum memory in bytes
	MaxMemory int64 `json:"max_memory"`

	// Use binary cache
	UseCache bool `json:"use_cache"`

	// Extra nix build arguments
	ExtraArgs []string `json:"extra_args"`

	// Force rebuild even if cached
	ForceRebuild bool `json:"force_rebuild"`

	// Keep build outputs on failure
	KeepFailed bool `json:"keep_failed"`

	// Show detailed build log
	Verbose bool `json:"verbose"`
}

// BuildStatus represents the current state of a build.
type BuildStatus string

const (
	BuildStatusPending   BuildStatus = "pending"
	BuildStatusQueued    BuildStatus = "queued"
	BuildStatusRunning   BuildStatus = "running"
	BuildStatusSuccess   BuildStatus = "success"
	BuildStatusFailed    BuildStatus = "failed"
	BuildStatusCancelled BuildStatus = "cancelled"
	BuildStatusTimeout   BuildStatus = "timeout"
)

// BuildResult contains the outcome of a build.
type BuildResult struct {
	// Build specification
	Spec BuildSpec `json:"spec"`

	// Build status
	Status BuildStatus `json:"status"`

	// Path to the built image (if successful)
	ImagePath string `json:"image_path,omitempty"`

	// Image hash for verification
	ImageHash string `json:"image_hash,omitempty"`

	// Image size in bytes
	ImageSize int64 `json:"image_size,omitempty"`

	// Store path of the build output
	StorePath string `json:"store_path,omitempty"`

	// Build timings
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt time.Time     `json:"completed_at"`
	Duration    time.Duration `json:"duration"`

	// Error information (if failed)
	Error        string `json:"error,omitempty"`
	ErrorDetails string `json:"error_details,omitempty"`

	// Build log (truncated if too long)
	Log string `json:"log,omitempty"`

	// Nix build output parsing
	OutputPaths []string `json:"output_paths,omitempty"`

	// LXD image alias (if imported)
	LXDAlias string `json:"lxd_alias,omitempty"`
}

// ============================================================================
// BUILD CACHE
// ============================================================================

// BuildCache tracks cached builds for change detection and reuse.
type BuildCache struct {
	mu sync.RWMutex

	// Cache directory
	cacheDir string

	// Entries indexed by cache key
	entries map[string]*CacheEntry

	// Maximum cache size in bytes
	maxSize int64

	// Current cache size
	currentSize int64

	// Maximum age for cache entries
	maxAge time.Duration
}

// CacheEntry represents a cached build.
type CacheEntry struct {
	// Cache key (hash of flake + attribute + inputs)
	Key string `json:"key"`

	// Original build spec
	Spec BuildSpec `json:"spec"`

	// Build result
	Result BuildResult `json:"result"`

	// Flake lock hash at build time
	LockHash string `json:"lock_hash"`

	// When the entry was created
	CreatedAt time.Time `json:"created_at"`

	// When the entry was last accessed
	LastAccess time.Time `json:"last_access"`

	// Entry size in bytes
	Size int64 `json:"size"`
}

// NewBuildCache creates a new build cache.
func NewBuildCache(cacheDir string, maxSize int64, maxAge time.Duration) (*BuildCache, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	cache := &BuildCache{
		cacheDir: cacheDir,
		entries:  make(map[string]*CacheEntry),
		maxSize:  maxSize,
		maxAge:   maxAge,
	}

	// Load existing cache entries
	if err := cache.load(); err != nil {
		// Non-fatal, start fresh
		cache.entries = make(map[string]*CacheEntry)
	}

	return cache, nil
}

// Get retrieves a cache entry if valid.
func (c *BuildCache) Get(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Since(entry.CreatedAt) > c.maxAge {
		return nil, false
	}

	// Update last access time (safe because we return a copy)
	entry.LastAccess = time.Now()
	cacheHits.Inc()

	return entry, true
}

// Put stores a build result in the cache.
func (c *BuildCache) Put(entry *CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old entries if needed
	for c.currentSize+entry.Size > c.maxSize && len(c.entries) > 0 {
		c.evictOldest()
	}

	c.entries[entry.Key] = entry
	c.currentSize += entry.Size
	cacheMisses.Inc()

	return c.persist(entry)
}

// Delete removes an entry from the cache.
func (c *BuildCache) Delete(key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil
	}

	c.currentSize -= entry.Size
	delete(c.entries, key)

	// Remove persisted file
	entryPath := filepath.Join(c.cacheDir, key+".json")
	os.Remove(entryPath)

	return nil
}

// GenerateKey creates a cache key for a build spec.
func (c *BuildCache) GenerateKey(spec *BuildSpec, lockHash string) string {
	h := sha256.New()
	h.Write([]byte(spec.FlakePath))
	h.Write([]byte(spec.Attribute))
	h.Write([]byte(spec.System))
	h.Write([]byte(lockHash))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// load reads cache entries from disk.
func (c *BuildCache) load() error {
	entries, err := os.ReadDir(c.cacheDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(c.cacheDir, e.Name()))
		if err != nil {
			continue
		}

		var entry CacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}

		c.entries[entry.Key] = &entry
		c.currentSize += entry.Size
	}

	return nil
}

// persist writes a cache entry to disk.
func (c *BuildCache) persist(entry *CacheEntry) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	entryPath := filepath.Join(c.cacheDir, entry.Key+".json")
	return os.WriteFile(entryPath, data, 0644)
}

// evictOldest removes the least recently accessed entry.
func (c *BuildCache) evictOldest() {
	var oldest *CacheEntry
	var oldestKey string

	for key, entry := range c.entries {
		if oldest == nil || entry.LastAccess.Before(oldest.LastAccess) {
			oldest = entry
			oldestKey = key
		}
	}

	if oldest != nil {
		c.currentSize -= oldest.Size
		delete(c.entries, oldestKey)
		os.Remove(filepath.Join(c.cacheDir, oldestKey+".json"))
	}
}

// Stats returns cache statistics.
func (c *BuildCache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"entries":      len(c.entries),
		"current_size": c.currentSize,
		"max_size":     c.maxSize,
		"max_age":      c.maxAge.String(),
	}
}

// ============================================================================
// BUILD QUEUE
// ============================================================================

// BuildQueue manages pending and active builds.
type BuildQueue struct {
	mu sync.RWMutex

	// Pending builds
	pending []*BuildSpec

	// Currently running builds
	active map[string]*buildJob

	// Maximum queue size
	maxSize int

	// Maximum concurrent builds
	maxConcurrent int

	// Signal channel for new work
	workCh chan struct{}
}

// buildJob represents an active build.
type buildJob struct {
	spec      *BuildSpec
	cancel    context.CancelFunc
	startedAt time.Time
}

// NewBuildQueue creates a new build queue.
func NewBuildQueue(maxSize, maxConcurrent int) *BuildQueue {
	return &BuildQueue{
		pending:       make([]*BuildSpec, 0, maxSize),
		active:        make(map[string]*buildJob),
		maxSize:       maxSize,
		maxConcurrent: maxConcurrent,
		workCh:        make(chan struct{}, 1),
	}
}

// Enqueue adds a build to the queue.
func (q *BuildQueue) Enqueue(spec *BuildSpec) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) >= q.maxSize {
		return ErrBuildQueueFull
	}

	// Insert by priority (higher priority first)
	inserted := false
	for i, s := range q.pending {
		if spec.Priority > s.Priority {
			q.pending = append(q.pending[:i], append([]*BuildSpec{spec}, q.pending[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		q.pending = append(q.pending, spec)
	}

	queueDepth.Set(float64(len(q.pending)))

	// Signal work available
	select {
	case q.workCh <- struct{}{}:
	default:
	}

	return nil
}

// Dequeue gets the next build to process.
func (q *BuildQueue) Dequeue() (*BuildSpec, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) == 0 || len(q.active) >= q.maxConcurrent {
		return nil, false
	}

	spec := q.pending[0]
	q.pending = q.pending[1:]
	queueDepth.Set(float64(len(q.pending)))

	return spec, true
}

// MarkActive marks a build as active.
func (q *BuildQueue) MarkActive(spec *BuildSpec, cancel context.CancelFunc) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.active[spec.ID] = &buildJob{
		spec:      spec,
		cancel:    cancel,
		startedAt: time.Now(),
	}
	activeBuilds.Set(float64(len(q.active)))
}

// MarkComplete marks a build as complete.
func (q *BuildQueue) MarkComplete(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.active, id)
	activeBuilds.Set(float64(len(q.active)))

	// Signal that a slot is available
	select {
	case q.workCh <- struct{}{}:
	default:
	}
}

// Cancel cancels a running build.
func (q *BuildQueue) Cancel(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if in pending queue
	for i, spec := range q.pending {
		if spec.ID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			queueDepth.Set(float64(len(q.pending)))
			return true
		}
	}

	// Check if active
	if job, ok := q.active[id]; ok {
		job.cancel()
		return true
	}

	return false
}

// GetStatus returns the status of a build.
func (q *BuildQueue) GetStatus(id string) (BuildStatus, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, spec := range q.pending {
		if spec.ID == id {
			return BuildStatusQueued, true
		}
	}

	if _, ok := q.active[id]; ok {
		return BuildStatusRunning, true
	}

	return "", false
}

// Stats returns queue statistics.
func (q *BuildQueue) Stats() map[string]interface{} {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return map[string]interface{}{
		"pending":        len(q.pending),
		"active":         len(q.active),
		"max_size":       q.maxSize,
		"max_concurrent": q.maxConcurrent,
	}
}

// ============================================================================
// FLAKE WATCHER
// ============================================================================

// FlakeWatcher monitors flake directories for changes.
type FlakeWatcher struct {
	mu sync.RWMutex

	// Paths being watched
	paths map[string]*watchedFlake

	// Callback for changes
	onChange func(path string, config *FlakeConfig)

	// Poll interval
	pollInterval time.Duration

	// Running state
	running int32
	stopCh  chan struct{}
}

// watchedFlake tracks a flake being watched.
type watchedFlake struct {
	path     string
	lockHash string
	modTime  time.Time
}

// NewFlakeWatcher creates a new flake watcher.
func NewFlakeWatcher(pollInterval time.Duration, onChange func(string, *FlakeConfig)) *FlakeWatcher {
	return &FlakeWatcher{
		paths:        make(map[string]*watchedFlake),
		onChange:     onChange,
		pollInterval: pollInterval,
		stopCh:       make(chan struct{}),
	}
}

// AddPath adds a flake path to watch.
func (w *FlakeWatcher) AddPath(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Get initial state
	lockHash, err := w.getLockHash(path)
	if err != nil {
		return err
	}

	info, err := os.Stat(filepath.Join(path, "flake.nix"))
	if err != nil {
		return err
	}

	w.paths[path] = &watchedFlake{
		path:     path,
		lockHash: lockHash,
		modTime:  info.ModTime(),
	}

	return nil
}

// RemovePath removes a flake path from watching.
func (w *FlakeWatcher) RemovePath(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.paths, path)
}

// Start begins watching for changes.
func (w *FlakeWatcher) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&w.running, 0, 1) {
		return nil // Already running
	}

	go w.watchLoop(ctx)
	return nil
}

// Stop stops watching.
func (w *FlakeWatcher) Stop() {
	if atomic.CompareAndSwapInt32(&w.running, 1, 0) {
		close(w.stopCh)
	}
}

// watchLoop is the main watch loop.
func (w *FlakeWatcher) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.checkChanges()
		}
	}
}

// checkChanges checks all watched paths for changes.
func (w *FlakeWatcher) checkChanges() {
	w.mu.RLock()
	paths := make([]*watchedFlake, 0, len(w.paths))
	for _, wf := range w.paths {
		paths = append(paths, wf)
	}
	w.mu.RUnlock()

	for _, wf := range paths {
		changed := false

		// Check flake.nix modification time
		info, err := os.Stat(filepath.Join(wf.path, "flake.nix"))
		if err != nil {
			continue
		}
		if info.ModTime().After(wf.modTime) {
			changed = true
		}

		// Check lock file hash
		newHash, err := w.getLockHash(wf.path)
		if err == nil && newHash != wf.lockHash {
			changed = true
		}

		if changed {
			// Update tracked state
			w.mu.Lock()
			if tracked, ok := w.paths[wf.path]; ok {
				tracked.modTime = info.ModTime()
				tracked.lockHash = newHash
			}
			w.mu.Unlock()

			// Parse and notify
			if config, err := ParseFlake(wf.path); err == nil {
				w.onChange(wf.path, config)
			}
		}
	}
}

// getLockHash computes hash of flake.lock.
func (w *FlakeWatcher) getLockHash(path string) (string, error) {
	lockPath := filepath.Join(path, "flake.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return "", err
	}

	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])[:16], nil
}

// ============================================================================
// BUILDER SERVICE
// ============================================================================

// BuilderConfig holds configuration for the Builder service.
type BuilderConfig struct {
	// Nix binary path
	NixBinary string `json:"nix_binary"`

	// Build output directory
	OutputDir string `json:"output_dir"`

	// Cache configuration
	CacheDir  string        `json:"cache_dir"`
	CacheSize int64         `json:"cache_size"`
	CacheAge  time.Duration `json:"cache_age"`

	// Queue configuration
	QueueSize     int `json:"queue_size"`
	MaxConcurrent int `json:"max_concurrent"`

	// Default build timeout
	DefaultTimeout time.Duration `json:"default_timeout"`

	// Watcher poll interval
	WatchInterval time.Duration `json:"watch_interval"`

	// Wotan topic for events
	WotanTopic string `json:"wotan_topic"`
}

// DefaultBuilderConfig returns sensible defaults.
func DefaultBuilderConfig() BuilderConfig {
	return BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      "/var/lib/unheaded/nix-builds",
		CacheDir:       "/var/lib/unheaded/nix-cache",
		CacheSize:      10 * 1024 * 1024 * 1024, // 10GB
		CacheAge:       7 * 24 * time.Hour,       // 1 week
		QueueSize:      100,
		MaxConcurrent:  2,
		DefaultTimeout: 30 * time.Minute,
		WatchInterval:  5 * time.Second,
		WotanTopic:    "nix.builds",
	}
}

// Builder is the main NixOS container builder service.
type Builder struct {
	config BuilderConfig
	cache  *BuildCache
	queue  *BuildQueue
	watcher *FlakeWatcher
	wotan *wotanClient.Client

	// Build results storage
	resultsMu sync.RWMutex
	results   map[string]*BuildResult

	// Running state
	running int32
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewBuilder creates a new Builder service.
func NewBuilder(config BuilderConfig, wotan *wotanClient.Client) (*Builder, error) {
	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	// Initialize cache
	cache, err := NewBuildCache(config.CacheDir, config.CacheSize, config.CacheAge)
	if err != nil {
		return nil, fmt.Errorf("initialize cache: %w", err)
	}

	// Initialize queue
	queue := NewBuildQueue(config.QueueSize, config.MaxConcurrent)

	b := &Builder{
		config:  config,
		cache:   cache,
		queue:   queue,
		wotan:  wotan,
		results: make(map[string]*BuildResult),
		stopCh:  make(chan struct{}),
	}

	// Initialize watcher
	b.watcher = NewFlakeWatcher(config.WatchInterval, b.onFlakeChange)

	return b, nil
}

// Start begins the builder service.
func (b *Builder) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&b.running, 0, 1) {
		return nil // Already running
	}

	// Start watcher
	if err := b.watcher.Start(ctx); err != nil {
		return fmt.Errorf("start watcher: %w", err)
	}

	// Start worker goroutines
	for i := 0; i < b.config.MaxConcurrent; i++ {
		b.wg.Add(1)
		go b.workerLoop(ctx)
	}

	return nil
}

// Stop stops the builder service.
func (b *Builder) Stop() error {
	if !atomic.CompareAndSwapInt32(&b.running, 1, 0) {
		return nil // Not running
	}

	close(b.stopCh)
	b.watcher.Stop()
	b.wg.Wait()

	return nil
}

// IsRunning returns whether the builder is running.
func (b *Builder) IsRunning() bool {
	return atomic.LoadInt32(&b.running) == 1
}

// workerLoop processes builds from the queue.
func (b *Builder) workerLoop(ctx context.Context) {
	defer b.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.stopCh:
			return
		case <-b.queue.workCh:
			b.processQueue(ctx)
		}
	}
}

// processQueue processes builds from the queue.
func (b *Builder) processQueue(ctx context.Context) {
	for {
		spec, ok := b.queue.Dequeue()
		if !ok {
			return
		}

		// Create cancellable context
		buildCtx, cancel := context.WithCancel(ctx)
		b.queue.MarkActive(spec, cancel)

		// Run build
		result := b.executeBuild(buildCtx, spec)

		// Store result
		b.resultsMu.Lock()
		b.results[spec.ID] = result
		b.resultsMu.Unlock()

		// Mark complete
		b.queue.MarkComplete(spec.ID)
		cancel()

		// Publish event
		b.publishBuildEvent(result)
	}
}

// executeBuild runs a single build.
func (b *Builder) executeBuild(ctx context.Context, spec *BuildSpec) *BuildResult {
	result := &BuildResult{
		Spec:      *spec,
		Status:    BuildStatusRunning,
		StartedAt: time.Now(),
	}

	// Validate spec
	if err := b.validateSpec(spec); err != nil {
		result.Status = BuildStatusFailed
		result.Error = err.Error()
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		buildsTotal.WithLabelValues(spec.ContainerName, "failed").Inc()
		return result
	}

	// Check cache
	flakeConfig, err := ParseFlake(spec.FlakePath)
	if err != nil {
		result.Status = BuildStatusFailed
		result.Error = fmt.Sprintf("parse flake: %v", err)
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		buildsTotal.WithLabelValues(spec.ContainerName, "failed").Inc()
		return result
	}

	cacheKey := b.cache.GenerateKey(spec, flakeConfig.LockHash)
	if !spec.Options.ForceRebuild {
		if cached, ok := b.cache.Get(cacheKey); ok {
			result.Status = BuildStatusSuccess
			result.ImagePath = cached.Result.ImagePath
			result.ImageHash = cached.Result.ImageHash
			result.ImageSize = cached.Result.ImageSize
			result.StorePath = cached.Result.StorePath
			result.OutputPaths = cached.Result.OutputPaths
			result.CompletedAt = time.Now()
			result.Duration = result.CompletedAt.Sub(result.StartedAt)
			result.Log = "[cache hit]"
			buildsTotal.WithLabelValues(spec.ContainerName, "cached").Inc()
			return result
		}
	}

	// Set timeout
	timeout := spec.Options.Timeout
	if timeout == 0 {
		timeout = b.config.DefaultTimeout
	}
	buildCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Run nix build
	buildResult, err := b.runNixBuild(buildCtx, spec)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			result.Status = BuildStatusCancelled
			result.Error = "build cancelled"
			buildsTotal.WithLabelValues(spec.ContainerName, "cancelled").Inc()
		} else if errors.Is(err, context.DeadlineExceeded) {
			result.Status = BuildStatusTimeout
			result.Error = "build timed out"
			buildsTotal.WithLabelValues(spec.ContainerName, "timeout").Inc()
		} else {
			result.Status = BuildStatusFailed
			result.Error = err.Error()
			buildsTotal.WithLabelValues(spec.ContainerName, "failed").Inc()
		}
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)
		buildDuration.WithLabelValues(spec.ContainerName).Observe(result.Duration.Seconds())
		return result
	}

	result.Status = BuildStatusSuccess
	result.StorePath = buildResult.storePath
	result.OutputPaths = buildResult.outputPaths
	result.Log = buildResult.log
	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	// Extract container image
	imagePath, imageHash, imageSize, err := b.extractContainerImage(buildResult.storePath, spec.ContainerName)
	if err != nil {
		result.Status = BuildStatusFailed
		result.Error = fmt.Sprintf("extract image: %v", err)
		buildsTotal.WithLabelValues(spec.ContainerName, "failed").Inc()
		buildDuration.WithLabelValues(spec.ContainerName).Observe(result.Duration.Seconds())
		return result
	}

	result.ImagePath = imagePath
	result.ImageHash = imageHash
	result.ImageSize = imageSize

	// Cache the result
	cacheEntry := &CacheEntry{
		Key:        cacheKey,
		Spec:       *spec,
		Result:     *result,
		LockHash:   flakeConfig.LockHash,
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		Size:       imageSize,
	}
	b.cache.Put(cacheEntry)

	buildsTotal.WithLabelValues(spec.ContainerName, "success").Inc()
	buildDuration.WithLabelValues(spec.ContainerName).Observe(result.Duration.Seconds())

	return result
}

// nixBuildResult holds parsed nix build output.
type nixBuildResult struct {
	storePath   string
	outputPaths []string
	log         string
}

// runNixBuild executes nix build command.
func (b *Builder) runNixBuild(ctx context.Context, spec *BuildSpec) (*nixBuildResult, error) {
	args := []string{
		"build",
		fmt.Sprintf("%s#%s", spec.FlakePath, spec.Attribute),
		"--out-link", filepath.Join(b.config.OutputDir, spec.ContainerName),
		"--json",
	}

	// Add optional arguments
	if spec.Options.MaxCores > 0 {
		args = append(args, "--cores", fmt.Sprintf("%d", spec.Options.MaxCores))
	}
	if !spec.Options.UseCache {
		args = append(args, "--no-substitute")
	}
	if spec.Options.KeepFailed {
		args = append(args, "--keep-failed")
	}
	if spec.Options.Verbose {
		args = append(args, "-v")
	}
	args = append(args, spec.Options.ExtraArgs...)

	cmd := exec.CommandContext(ctx, b.config.NixBinary, args...)

	// Capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run build
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBuildFailed, stderr.String())
	}

	// Parse JSON output
	result := &nixBuildResult{
		log: stderr.String(),
	}

	var buildOutput []struct {
		DrvPath string   `json:"drvPath"`
		Outputs map[string]string `json:"outputs"`
	}

	if err := json.Unmarshal([]byte(stdout.String()), &buildOutput); err == nil && len(buildOutput) > 0 {
		for _, out := range buildOutput[0].Outputs {
			result.outputPaths = append(result.outputPaths, out)
			if result.storePath == "" {
				result.storePath = out
			}
		}
	}

	return result, nil
}

// extractContainerImage extracts the container image from a store path.
func (b *Builder) extractContainerImage(storePath, containerName string) (string, string, int64, error) {
	// Look for tarball in store path
	tarballPath := filepath.Join(storePath, "tarball", "nixos-system-"+containerName+"-*.tar.xz")
	matches, err := filepath.Glob(tarballPath)
	if err != nil || len(matches) == 0 {
		// Try alternative patterns
		tarballPath = filepath.Join(storePath, "*.tar.xz")
		matches, err = filepath.Glob(tarballPath)
		if err != nil || len(matches) == 0 {
			// Use store path directly
			matches = []string{storePath}
		}
	}

	imagePath := matches[0]

	// Compute hash
	f, err := os.Open(imagePath)
	if err != nil {
		return "", "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", "", 0, err
	}

	imageHash := hex.EncodeToString(h.Sum(nil))

	info, err := os.Stat(imagePath)
	if err != nil {
		return "", "", 0, err
	}

	return imagePath, imageHash, info.Size(), nil
}

// validateSpec validates a build specification.
func (b *Builder) validateSpec(spec *BuildSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("%w: missing ID", ErrInvalidBuildSpec)
	}
	if spec.FlakePath == "" {
		return fmt.Errorf("%w: missing flake path", ErrInvalidBuildSpec)
	}
	if spec.Attribute == "" {
		return fmt.Errorf("%w: missing attribute", ErrInvalidBuildSpec)
	}
	if spec.ContainerName == "" {
		return fmt.Errorf("%w: missing container name", ErrInvalidBuildSpec)
	}

	// Check flake exists
	flakePath := filepath.Join(spec.FlakePath, "flake.nix")
	if _, err := os.Stat(flakePath); err != nil {
		return fmt.Errorf("%w: %s", ErrFlakeNotFound, flakePath)
	}

	return nil
}

// onFlakeChange is called when a watched flake changes.
func (b *Builder) onFlakeChange(path string, config *FlakeConfig) {
	// Enqueue rebuild for all container outputs
	for _, output := range config.Outputs {
		if output.Type != "nixosConfiguration" && output.Type != "container" {
			continue
		}

		spec := &BuildSpec{
			ID:            fmt.Sprintf("auto-%s-%d", output.Name, time.Now().UnixNano()),
			FlakePath:     path,
			Attribute:     output.Name,
			ContainerName: output.Name,
			System:        config.System,
			Priority:      1, // Low priority for auto-rebuilds
			RequestedBy:   "flake-watcher",
			RequestedAt:   time.Now(),
			Options: BuildOptions{
				UseCache: true,
			},
		}

		b.queue.Enqueue(spec)
	}
}

// publishBuildEvent sends a build event to Wotan.
func (b *Builder) publishBuildEvent(result *BuildResult) {
	if b.wotan == nil {
		fmt.Fprintf(os.Stderr, "nix: wotan unavailable — build event dropped (degraded mode)\n")
		return
	}

	event := map[string]interface{}{
		"type":       "build_complete",
		"build_id":   result.Spec.ID,
		"container":  result.Spec.ContainerName,
		"status":     result.Status,
		"duration":   result.Duration.Seconds(),
		"timestamp":  time.Now().UnixMilli(),
	}

	if result.Error != "" {
		event["error"] = result.Error
	}
	if result.ImagePath != "" {
		event["image_path"] = result.ImagePath
	}

	data, _ := json.Marshal(event)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b.wotan.Publish(ctx, b.config.WotanTopic, data)
}

// ============================================================================
// PUBLIC API
// ============================================================================

// BuildContainer queues a container build.
func (b *Builder) BuildContainer(ctx context.Context, spec *BuildSpec) error {
	if !b.IsRunning() {
		return ErrBuilderNotRunning
	}

	if spec.System == "" {
		spec.System = "x86_64-linux"
	}
	if spec.RequestedAt.IsZero() {
		spec.RequestedAt = time.Now()
	}

	return b.queue.Enqueue(spec)
}

// GetBuildStatus returns the status of a build.
func (b *Builder) GetBuildStatus(id string) (*BuildResult, error) {
	// Check completed results
	b.resultsMu.RLock()
	if result, ok := b.results[id]; ok {
		b.resultsMu.RUnlock()
		return result, nil
	}
	b.resultsMu.RUnlock()

	// Check queue
	status, found := b.queue.GetStatus(id)
	if found {
		return &BuildResult{
			Spec:   BuildSpec{ID: id},
			Status: status,
		}, nil
	}

	return nil, fmt.Errorf("build not found: %s", id)
}

// CancelBuild cancels a pending or running build.
func (b *Builder) CancelBuild(id string) error {
	if b.queue.Cancel(id) {
		// Update result if exists
		b.resultsMu.Lock()
		if result, ok := b.results[id]; ok {
			result.Status = BuildStatusCancelled
		}
		b.resultsMu.Unlock()
		return nil
	}
	return fmt.Errorf("build not found or already completed: %s", id)
}

// WatchFlakes adds paths to the flake watcher.
func (b *Builder) WatchFlakes(ctx context.Context, paths []string) error {
	for _, path := range paths {
		if err := b.watcher.AddPath(path); err != nil {
			return fmt.Errorf("watch %s: %w", path, err)
		}
	}
	return nil
}

// UnwatchFlake removes a path from the flake watcher.
func (b *Builder) UnwatchFlake(path string) {
	b.watcher.RemovePath(path)
}

// ImportToLXD imports a built image into LXD.
func (b *Builder) ImportToLXD(ctx context.Context, result *BuildResult, lxdClient lxd.Client) error {
	if result.Status != BuildStatusSuccess {
		return fmt.Errorf("cannot import failed build")
	}
	if result.ImagePath == "" {
		return ErrImageNotFound
	}

	// Generate alias
	alias := fmt.Sprintf("unheaded-%s-%s", result.Spec.ContainerName, result.ImageHash[:12])

	// Import using lxc image import command (via exec since client interface may not have it)
	cmd := exec.CommandContext(ctx, "lxc", "image", "import", result.ImagePath, "--alias", alias)
	output, err := cmd.CombinedOutput()
	if err != nil {
		importsTotal.WithLabelValues(result.Spec.ContainerName, "failed").Inc()
		return fmt.Errorf("%w: %s", ErrLXDImportFailed, string(output))
	}

	result.LXDAlias = alias
	importsTotal.WithLabelValues(result.Spec.ContainerName, "success").Inc()

	return nil
}

// CleanOldBuilds removes builds older than retention period.
func (b *Builder) CleanOldBuilds(retention time.Duration) (int, error) {
	b.resultsMu.Lock()
	defer b.resultsMu.Unlock()

	cutoff := time.Now().Add(-retention)
	removed := 0

	for id, result := range b.results {
		if result.CompletedAt.Before(cutoff) {
			delete(b.results, id)
			removed++
		}
	}

	return removed, nil
}

// Stats returns builder statistics.
func (b *Builder) Stats() map[string]interface{} {
	b.resultsMu.RLock()
	completedCount := len(b.results)
	b.resultsMu.RUnlock()

	return map[string]interface{}{
		"running":   b.IsRunning(),
		"queue":     b.queue.Stats(),
		"cache":     b.cache.Stats(),
		"completed": completedCount,
	}
}

// ============================================================================
// FLAKE PARSING
// ============================================================================

// ParseFlake parses a flake.nix configuration.
func ParseFlake(path string) (*FlakeConfig, error) {
	flakePath := filepath.Join(path, "flake.nix")
	if _, err := os.Stat(flakePath); err != nil {
		return nil, ErrFlakeNotFound
	}

	config := &FlakeConfig{
		Path:   path,
		System: "x86_64-linux",
	}

	// Get modification time
	info, err := os.Stat(flakePath)
	if err != nil {
		return nil, err
	}
	config.ModTime = info.ModTime()

	// Compute lock hash
	lockPath := filepath.Join(path, "flake.lock")
	if data, err := os.ReadFile(lockPath); err == nil {
		h := sha256.Sum256(data)
		config.LockHash = hex.EncodeToString(h[:])[:16]
	}

	// Get flake metadata using nix command
	cmd := exec.Command("nix", "flake", "metadata", path, "--json")
	output, err := cmd.Output()
	if err != nil {
		// Try to parse flake.nix directly as fallback
		return parseFlakeFile(config)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(output, &metadata); err != nil {
		return parseFlakeFile(config)
	}

	config.Metadata = metadata

	// Extract description
	if desc, ok := metadata["description"].(string); ok {
		config.Description = desc
	}

	// Extract inputs (only direct inputs, not transitive dependencies)
	if locks, ok := metadata["locks"].(map[string]interface{}); ok {
		if nodes, ok := locks["nodes"].(map[string]interface{}); ok {
			// Find direct inputs from the root node
			directInputs := make(map[string]string) // input name -> node name
			if rootData, ok := nodes["root"].(map[string]interface{}); ok {
				if rootInputs, ok := rootData["inputs"].(map[string]interface{}); ok {
					for inputName, nodeRef := range rootInputs {
						if nodeName, ok := nodeRef.(string); ok {
							directInputs[inputName] = nodeName
						}
					}
				}
			}

			for inputName, nodeName := range directInputs {
				nodeData, ok := nodes[nodeName]
				if !ok {
					continue
				}
				node, ok := nodeData.(map[string]interface{})
				if !ok {
					continue
				}

				input := FlakeInput{
					Name:   inputName,
					Locked: true,
				}

				if locked, ok := node["locked"].(map[string]interface{}); ok {
					if rev, ok := locked["rev"].(string); ok {
						input.Rev = rev
					}
				}

				if original, ok := node["original"].(map[string]interface{}); ok {
					if url, ok := original["url"].(string); ok {
						input.URL = url
					} else if typ, ok := original["type"].(string); ok {
						if owner, ok := original["owner"].(string); ok {
							if repo, ok := original["repo"].(string); ok {
								input.URL = fmt.Sprintf("%s:%s/%s", typ, owner, repo)
							}
						}
					}
				}

				config.Inputs = append(config.Inputs, input)
			}
		}
	}

	// Get flake outputs
	cmd = exec.Command("nix", "flake", "show", path, "--json")
	output, err = cmd.Output()
	if err == nil {
		var outputs map[string]interface{}
		if err := json.Unmarshal(output, &outputs); err == nil {
			config.Outputs = parseFlakeOutputs(outputs)
		}
	}

	return config, nil
}

// parseFlakeFile parses flake.nix file directly (fallback).
func parseFlakeFile(config *FlakeConfig) (*FlakeConfig, error) {
	flakePath := filepath.Join(config.Path, "flake.nix")
	data, err := os.ReadFile(flakePath)
	if err != nil {
		return nil, err
	}

	content := string(data)

	// Extract description
	descRe := regexp.MustCompile(`description\s*=\s*"([^"]+)"`)
	if matches := descRe.FindStringSubmatch(content); len(matches) > 1 {
		config.Description = matches[1]
	}

	// Extract inputs (basic parsing)
	inputRe := regexp.MustCompile(`(\w+)\.url\s*=\s*"([^"]+)"`)
	for _, match := range inputRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 2 {
			config.Inputs = append(config.Inputs, FlakeInput{
				Name: match[1],
				URL:  match[2],
			})
		}
	}

	// Look for nixosConfigurations
	if strings.Contains(content, "nixosConfigurations") {
		configRe := regexp.MustCompile(`nixosConfigurations\.(\w+)`)
		for _, match := range configRe.FindAllStringSubmatch(content, -1) {
			if len(match) > 1 {
				config.Outputs = append(config.Outputs, FlakeOutput{
					Name:   fmt.Sprintf("nixosConfigurations.%s", match[1]),
					Type:   "nixosConfiguration",
					System: config.System,
				})
			}
		}
	}

	return config, nil
}

// parseFlakeOutputs parses nix flake show output.
func parseFlakeOutputs(outputs map[string]interface{}) []FlakeOutput {
	var result []FlakeOutput

	var walk func(prefix string, data map[string]interface{})
	walk = func(prefix string, data map[string]interface{}) {
		for key, value := range data {
			fullKey := key
			if prefix != "" {
				fullKey = prefix + "." + key
			}

			if m, ok := value.(map[string]interface{}); ok {
				// Check if this is a leaf node (has type)
				if typ, hasType := m["type"].(string); hasType {
					output := FlakeOutput{
						Name: fullKey,
						Type: typ,
					}
					if desc, ok := m["description"].(string); ok {
						output.Description = desc
					}
					result = append(result, output)
				} else {
					walk(fullKey, m)
				}
			}
		}
	}

	walk("", outputs)
	return result
}

// ============================================================================
// BUILD LOG PARSING
// ============================================================================

// BuildLogEntry represents a parsed build log entry.
type BuildLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Drv       string    `json:"drv,omitempty"`
	Phase     string    `json:"phase,omitempty"`
}

// ParseBuildLog parses nix build output into structured entries.
func ParseBuildLog(log string) []BuildLogEntry {
	var entries []BuildLogEntry

	scanner := bufio.NewScanner(strings.NewReader(log))
	for scanner.Scan() {
		line := scanner.Text()
		entry := BuildLogEntry{
			Timestamp: time.Now(),
			Message:   line,
		}

		// Parse nix-specific patterns
		if strings.HasPrefix(line, "building '") {
			entry.Level = "info"
			entry.Phase = "build"
			// Extract derivation path
			if idx := strings.Index(line, "'"); idx >= 0 {
				rest := line[idx+1:]
				if idx2 := strings.Index(rest, "'"); idx2 >= 0 {
					entry.Drv = rest[:idx2]
				}
			}
		} else if strings.HasPrefix(line, "error:") {
			entry.Level = "error"
		} else if strings.HasPrefix(line, "warning:") {
			entry.Level = "warning"
		} else if strings.Contains(line, "copying") {
			entry.Level = "info"
			entry.Phase = "copy"
		} else if strings.Contains(line, "downloading") {
			entry.Level = "info"
			entry.Phase = "download"
		} else {
			entry.Level = "debug"
		}

		entries = append(entries, entry)
	}

	return entries
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// ComputeFlakeHash computes a hash of a flake for change detection.
func ComputeFlakeHash(path string) (string, error) {
	h := sha256.New()

	// Hash flake.nix
	flakeData, err := os.ReadFile(filepath.Join(path, "flake.nix"))
	if err != nil {
		return "", err
	}
	h.Write(flakeData)

	// Hash flake.lock if exists
	lockData, err := os.ReadFile(filepath.Join(path, "flake.lock"))
	if err == nil {
		h.Write(lockData)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// ListFlakeContainers returns container definitions from a flake.
func ListFlakeContainers(path string) ([]string, error) {
	config, err := ParseFlake(path)
	if err != nil {
		return nil, err
	}

	var containers []string
	for _, output := range config.Outputs {
		if output.Type == "nixosConfiguration" || output.Type == "container" {
			containers = append(containers, output.Name)
		}
	}

	return containers, nil
}

// ValidateFlake validates a flake configuration.
func ValidateFlake(path string) error {
	cmd := exec.Command("nix", "flake", "check", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", ErrFlakeParseError, string(output))
	}
	return nil
}
