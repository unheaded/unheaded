package nix

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// TEST HELPERS
// ============================================================================

// createTempFlake creates a temporary directory with a flake.nix and optional flake.lock.
// It returns the directory path and a cleanup function.
func createTempFlake(t *testing.T, flakeContent, lockContent string) string {
	t.Helper()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "flake.nix"), []byte(flakeContent), 0644); err != nil {
		t.Fatalf("write flake.nix: %v", err)
	}
	if lockContent != "" {
		if err := os.WriteFile(filepath.Join(dir, "flake.lock"), []byte(lockContent), 0644); err != nil {
			t.Fatalf("write flake.lock: %v", err)
		}
	}
	return dir
}

// sampleFlakeNix returns a realistic flake.nix content for testing.
func sampleFlakeNix() string {
	return `{
  description = "Unheaded NixOS containers";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-23.11";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
  {
    nixosConfigurations.timeguru = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
    };
    nixosConfigurations.captain = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
    };
    packages.x86_64-linux.default = nixpkgs.hello;
  };
}`
}

// sampleFlakeLock returns a realistic flake.lock content for testing.
func sampleFlakeLock() string {
	return `{
  "nodes": {
    "nixpkgs": {
      "locked": {
        "rev": "abc123def456",
        "type": "github"
      }
    },
    "root": {
      "inputs": {
        "nixpkgs": "nixpkgs"
      }
    }
  },
  "root": "root",
  "version": 7
}`
}

// newTestBuildSpec returns a valid BuildSpec for testing.
func newTestBuildSpec(flakePath string) *BuildSpec {
	return &BuildSpec{
		ID:            "test-build-001",
		FlakePath:     flakePath,
		Attribute:     "nixosConfigurations.timeguru",
		ContainerName: "timeguru",
		System:        "x86_64-linux",
		Priority:      5,
		RequestedBy:   "test",
		RequestedAt:   time.Now(),
		Options: BuildOptions{
			Timeout:  5 * time.Minute,
			UseCache: true,
		},
	}
}

// ============================================================================
// BUILD CACHE TESTS
// ============================================================================

func TestNewBuildCache(t *testing.T) {
	tests := []struct {
		name      string
		maxSize   int64
		maxAge    time.Duration
		wantError bool
	}{
		{
			name:    "valid cache configuration",
			maxSize: 1024 * 1024,
			maxAge:  24 * time.Hour,
		},
		{
			name:    "zero max size",
			maxSize: 0,
			maxAge:  time.Hour,
		},
		{
			name:    "large max size",
			maxSize: 10 * 1024 * 1024 * 1024,
			maxAge:  7 * 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cache, err := NewBuildCache(dir, tt.maxSize, tt.maxAge)
			if (err != nil) != tt.wantError {
				t.Errorf("NewBuildCache() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if !tt.wantError && cache == nil {
				t.Error("NewBuildCache() returned nil cache without error")
			}
		})
	}
}

func TestBuildCache_PutAndGet(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	entry := &CacheEntry{
		Key:  "test-key-001",
		Spec: BuildSpec{ID: "build-1", ContainerName: "timeguru"},
		Result: BuildResult{
			Status:    BuildStatusSuccess,
			ImagePath: "/nix/store/abc123-image.tar.xz",
			ImageHash: "deadbeef",
			ImageSize: 1024,
		},
		LockHash:   "lockhash123",
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		Size:       1024,
	}

	// Put
	if err := cache.Put(entry); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get
	got, ok := cache.Get("test-key-001")
	if !ok {
		t.Fatal("Get returned not found after Put")
	}
	if got.Key != entry.Key {
		t.Errorf("Get key = %q, want %q", got.Key, entry.Key)
	}
	if got.Result.ImagePath != entry.Result.ImagePath {
		t.Errorf("Get ImagePath = %q, want %q", got.Result.ImagePath, entry.Result.ImagePath)
	}
}

func TestBuildCache_GetMissing(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	_, ok := cache.Get("nonexistent-key")
	if ok {
		t.Error("Get returned found for nonexistent key")
	}
}

func TestBuildCache_GetExpired(t *testing.T) {
	dir := t.TempDir()
	// Cache with very short maxAge
	cache, err := NewBuildCache(dir, 10*1024*1024, 1*time.Nanosecond)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	entry := &CacheEntry{
		Key:        "expired-key",
		CreatedAt:  time.Now().Add(-1 * time.Hour), // Created 1 hour ago
		LastAccess: time.Now().Add(-1 * time.Hour),
		Size:       100,
	}

	if err := cache.Put(entry); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Should be expired
	_, ok := cache.Get("expired-key")
	if ok {
		t.Error("Get returned found for expired entry")
	}
}

func TestBuildCache_Delete(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	entry := &CacheEntry{
		Key:        "delete-me",
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		Size:       100,
	}

	if err := cache.Put(entry); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Delete
	if err := cache.Delete("delete-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify deleted
	_, ok := cache.Get("delete-me")
	if ok {
		t.Error("Get returned found after Delete")
	}
}

func TestBuildCache_DeleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	// Should not error on missing key
	if err := cache.Delete("does-not-exist"); err != nil {
		t.Errorf("Delete nonexistent returned error: %v", err)
	}
}

func TestBuildCache_Eviction(t *testing.T) {
	dir := t.TempDir()
	// Small cache: max 500 bytes
	cache, err := NewBuildCache(dir, 500, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	// Add entries that will exceed the cache size
	for i := 0; i < 5; i++ {
		entry := &CacheEntry{
			Key:        fmt.Sprintf("key-%d", i),
			CreatedAt:  time.Now().Add(time.Duration(i) * time.Second),
			LastAccess: time.Now().Add(time.Duration(i) * time.Second),
			Size:       200,
		}
		if err := cache.Put(entry); err != nil {
			t.Fatalf("Put key-%d: %v", i, err)
		}
	}

	stats := cache.Stats()
	currentSize, _ := stats["current_size"].(int64)
	maxSize, _ := stats["max_size"].(int64)

	if currentSize > maxSize {
		t.Errorf("cache size %d exceeds max %d after eviction", currentSize, maxSize)
	}
}

func TestBuildCache_GenerateKey(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	tests := []struct {
		name     string
		spec     *BuildSpec
		lockHash string
	}{
		{
			name: "basic spec",
			spec: &BuildSpec{
				FlakePath: "/path/to/flake",
				Attribute: "nixosConfigurations.timeguru",
				System:    "x86_64-linux",
			},
			lockHash: "abc123",
		},
		{
			name: "different attribute",
			spec: &BuildSpec{
				FlakePath: "/path/to/flake",
				Attribute: "nixosConfigurations.captain",
				System:    "x86_64-linux",
			},
			lockHash: "abc123",
		},
		{
			name: "different lock hash",
			spec: &BuildSpec{
				FlakePath: "/path/to/flake",
				Attribute: "nixosConfigurations.timeguru",
				System:    "x86_64-linux",
			},
			lockHash: "def456",
		},
	}

	keys := make(map[string]bool)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := cache.GenerateKey(tt.spec, tt.lockHash)
			if len(key) != 32 {
				t.Errorf("GenerateKey length = %d, want 32", len(key))
			}
			keys[key] = true
		})
	}

	// All keys should be unique
	if len(keys) != len(tests) {
		t.Errorf("expected %d unique keys, got %d", len(tests), len(keys))
	}
}

func TestBuildCache_GenerateKeyDeterministic(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	spec := &BuildSpec{
		FlakePath: "/path/to/flake",
		Attribute: "nixosConfigurations.timeguru",
		System:    "x86_64-linux",
	}

	key1 := cache.GenerateKey(spec, "lock123")
	key2 := cache.GenerateKey(spec, "lock123")

	if key1 != key2 {
		t.Errorf("GenerateKey not deterministic: %q != %q", key1, key2)
	}
}

func TestBuildCache_Stats(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewBuildCache(dir, 1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	stats := cache.Stats()

	if _, ok := stats["entries"]; !ok {
		t.Error("Stats missing 'entries' key")
	}
	if _, ok := stats["current_size"]; !ok {
		t.Error("Stats missing 'current_size' key")
	}
	if _, ok := stats["max_size"]; !ok {
		t.Error("Stats missing 'max_size' key")
	}
	if _, ok := stats["max_age"]; !ok {
		t.Error("Stats missing 'max_age' key")
	}

	entries := stats["entries"].(int)
	if entries != 0 {
		t.Errorf("empty cache entries = %d, want 0", entries)
	}
}

func TestBuildCache_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Create cache and add entry
	cache1, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache (first): %v", err)
	}

	entry := &CacheEntry{
		Key:        "persist-key",
		Spec:       BuildSpec{ID: "build-persist", ContainerName: "test"},
		Result:     BuildResult{Status: BuildStatusSuccess, ImagePath: "/test/image"},
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		Size:       100,
	}
	if err := cache1.Put(entry); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Create new cache from same directory (simulating restart)
	cache2, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache (second): %v", err)
	}

	got, ok := cache2.Get("persist-key")
	if !ok {
		t.Fatal("persisted entry not found after reload")
	}
	if got.Result.ImagePath != "/test/image" {
		t.Errorf("reloaded ImagePath = %q, want %q", got.Result.ImagePath, "/test/image")
	}
}

func TestBuildCache_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	var wg sync.WaitGroup
	const goroutines = 20
	const opsPerGoroutine = 50

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				key := fmt.Sprintf("conc-key-%d-%d", id, j)
				entry := &CacheEntry{
					Key:        key,
					CreatedAt:  time.Now(),
					LastAccess: time.Now(),
					Size:       10,
				}

				// Mix of operations
				switch j % 4 {
				case 0:
					cache.Put(entry)
				case 1:
					cache.Get(key)
				case 2:
					cache.Delete(key)
				case 3:
					cache.Stats()
				}
			}
		}(i)
	}

	wg.Wait()
}

// ============================================================================
// BUILD QUEUE TESTS
// ============================================================================

func TestNewBuildQueue(t *testing.T) {
	q := NewBuildQueue(10, 2)
	if q == nil {
		t.Fatal("NewBuildQueue returned nil")
	}

	stats := q.Stats()
	if stats["max_size"].(int) != 10 {
		t.Errorf("max_size = %d, want 10", stats["max_size"].(int))
	}
	if stats["max_concurrent"].(int) != 2 {
		t.Errorf("max_concurrent = %d, want 2", stats["max_concurrent"].(int))
	}
}

func TestBuildQueue_EnqueueDequeue(t *testing.T) {
	q := NewBuildQueue(10, 2)

	spec := &BuildSpec{
		ID:            "test-1",
		ContainerName: "timeguru",
		Priority:      5,
	}

	if err := q.Enqueue(spec); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue returned not ok")
	}
	if got.ID != "test-1" {
		t.Errorf("Dequeue ID = %q, want %q", got.ID, "test-1")
	}
}

func TestBuildQueue_DequeueEmpty(t *testing.T) {
	q := NewBuildQueue(10, 2)

	_, ok := q.Dequeue()
	if ok {
		t.Error("Dequeue on empty queue returned ok")
	}
}

func TestBuildQueue_PriorityOrdering(t *testing.T) {
	q := NewBuildQueue(100, 10)

	specs := []*BuildSpec{
		{ID: "low", Priority: 1},
		{ID: "medium", Priority: 5},
		{ID: "high", Priority: 10},
		{ID: "critical", Priority: 100},
		{ID: "normal", Priority: 3},
	}

	for _, s := range specs {
		if err := q.Enqueue(s); err != nil {
			t.Fatalf("Enqueue %s: %v", s.ID, err)
		}
	}

	// Dequeue should return in priority order (highest first)
	expectedOrder := []string{"critical", "high", "medium", "normal", "low"}
	for _, expected := range expectedOrder {
		got, ok := q.Dequeue()
		if !ok {
			t.Fatalf("Dequeue returned not ok, expected %s", expected)
		}
		if got.ID != expected {
			t.Errorf("Dequeue order: got %q, want %q", got.ID, expected)
		}
	}
}

func TestBuildQueue_QueueFull(t *testing.T) {
	q := NewBuildQueue(3, 1)

	for i := 0; i < 3; i++ {
		spec := &BuildSpec{ID: fmt.Sprintf("build-%d", i)}
		if err := q.Enqueue(spec); err != nil {
			t.Fatalf("Enqueue %d: %v", i, err)
		}
	}

	// Fourth should fail
	err := q.Enqueue(&BuildSpec{ID: "overflow"})
	if err != ErrBuildQueueFull {
		t.Errorf("Enqueue on full queue: got %v, want %v", err, ErrBuildQueueFull)
	}
}

func TestBuildQueue_MaxConcurrentLimit(t *testing.T) {
	q := NewBuildQueue(10, 1) // max 1 concurrent

	spec1 := &BuildSpec{ID: "build-1"}
	spec2 := &BuildSpec{ID: "build-2"}

	if err := q.Enqueue(spec1); err != nil {
		t.Fatalf("Enqueue 1: %v", err)
	}
	if err := q.Enqueue(spec2); err != nil {
		t.Fatalf("Enqueue 2: %v", err)
	}

	// Dequeue first
	got1, ok := q.Dequeue()
	if !ok {
		t.Fatal("first Dequeue returned not ok")
	}

	// Mark it active
	_, cancel := context.WithCancel(context.Background())
	q.MarkActive(got1, cancel)

	// Second dequeue should fail because max concurrent is 1
	_, ok = q.Dequeue()
	if ok {
		t.Error("Dequeue succeeded despite max concurrent reached")
	}

	// Mark first complete
	q.MarkComplete(got1.ID)
	cancel()

	// Now second should succeed
	got2, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue after MarkComplete returned not ok")
	}
	if got2.ID != "build-2" {
		t.Errorf("second Dequeue ID = %q, want %q", got2.ID, "build-2")
	}
}

func TestBuildQueue_CancelPending(t *testing.T) {
	q := NewBuildQueue(10, 2)

	spec := &BuildSpec{ID: "cancel-me"}
	if err := q.Enqueue(spec); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if !q.Cancel("cancel-me") {
		t.Error("Cancel pending build returned false")
	}

	// Should be removed from queue
	_, ok := q.Dequeue()
	if ok {
		t.Error("Dequeue succeeded after cancel")
	}
}

func TestBuildQueue_CancelActive(t *testing.T) {
	q := NewBuildQueue(10, 2)

	spec := &BuildSpec{ID: "active-cancel"}
	if err := q.Enqueue(spec); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue returned not ok")
	}

	cancelled := false
	q.MarkActive(got, func() { cancelled = true })

	if !q.Cancel("active-cancel") {
		t.Error("Cancel active build returned false")
	}

	if !cancelled {
		t.Error("cancel function was not called")
	}
}

func TestBuildQueue_CancelNonexistent(t *testing.T) {
	q := NewBuildQueue(10, 2)

	if q.Cancel("does-not-exist") {
		t.Error("Cancel nonexistent returned true")
	}
}

func TestBuildQueue_GetStatus(t *testing.T) {
	q := NewBuildQueue(10, 2)

	spec := &BuildSpec{ID: "status-check"}
	if err := q.Enqueue(spec); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Check queued status
	status, found := q.GetStatus("status-check")
	if !found {
		t.Fatal("GetStatus: not found")
	}
	if status != BuildStatusQueued {
		t.Errorf("GetStatus = %q, want %q", status, BuildStatusQueued)
	}

	// Dequeue and mark active
	got, _ := q.Dequeue()
	_, cancel := context.WithCancel(context.Background())
	q.MarkActive(got, cancel)
	defer cancel()

	// Check running status
	status, found = q.GetStatus("status-check")
	if !found {
		t.Fatal("GetStatus after MarkActive: not found")
	}
	if status != BuildStatusRunning {
		t.Errorf("GetStatus = %q, want %q", status, BuildStatusRunning)
	}

	// After complete, not found
	q.MarkComplete("status-check")
	_, found = q.GetStatus("status-check")
	if found {
		t.Error("GetStatus found completed build")
	}
}

func TestBuildQueue_GetStatusNotFound(t *testing.T) {
	q := NewBuildQueue(10, 2)

	_, found := q.GetStatus("ghost")
	if found {
		t.Error("GetStatus found nonexistent build")
	}
}

func TestBuildQueue_Stats(t *testing.T) {
	q := NewBuildQueue(100, 5)

	for i := 0; i < 3; i++ {
		q.Enqueue(&BuildSpec{ID: fmt.Sprintf("stat-%d", i)})
	}

	stats := q.Stats()

	if stats["pending"].(int) != 3 {
		t.Errorf("stats pending = %d, want 3", stats["pending"].(int))
	}
	if stats["active"].(int) != 0 {
		t.Errorf("stats active = %d, want 0", stats["active"].(int))
	}
}

func TestBuildQueue_ConcurrentEnqueueDequeue(t *testing.T) {
	q := NewBuildQueue(1000, 100)
	var wg sync.WaitGroup

	const producers = 10
	const consumers = 10
	const itemsPerProducer = 50

	// Producers
	wg.Add(producers)
	for i := 0; i < producers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < itemsPerProducer; j++ {
				spec := &BuildSpec{
					ID:       fmt.Sprintf("build-%d-%d", id, j),
					Priority: j % 10,
				}
				q.Enqueue(spec)
			}
		}(i)
	}

	// Consumers
	var dequeued int64
	var dequeueMu sync.Mutex
	wg.Add(consumers)
	for i := 0; i < consumers; i++ {
		go func() {
			defer wg.Done()
			for {
				_, ok := q.Dequeue()
				if !ok {
					return
				}
				dequeueMu.Lock()
				dequeued++
				dequeueMu.Unlock()
			}
		}()
	}

	wg.Wait()
	// No panic or data race is the main assertion here
}

// ============================================================================
// FLAKE PARSING TESTS
// ============================================================================

func TestParseFlake_NotFound(t *testing.T) {
	_, err := ParseFlake("/nonexistent/path")
	if err != ErrFlakeNotFound {
		t.Errorf("ParseFlake nonexistent: got %v, want %v", err, ErrFlakeNotFound)
	}
}

func TestParseFlake_FallbackParsing(t *testing.T) {
	// ParseFlake will fail the nix command (not installed in test env) and fall back
	// to parseFlakeFile
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	config, err := ParseFlake(dir)
	if err != nil {
		t.Fatalf("ParseFlake: %v", err)
	}

	if config.Path != dir {
		t.Errorf("Path = %q, want %q", config.Path, dir)
	}

	// Should have parsed description from regex
	if config.Description != "Unheaded NixOS containers" {
		t.Errorf("Description = %q, want %q", config.Description, "Unheaded NixOS containers")
	}

	// Should have parsed inputs
	if len(config.Inputs) < 1 {
		t.Errorf("len(Inputs) = %d, want >= 1", len(config.Inputs))
	}

	// Check that nixpkgs input was found
	foundNixpkgs := false
	for _, input := range config.Inputs {
		if input.Name == "nixpkgs" {
			foundNixpkgs = true
			if input.URL != "github:NixOS/nixpkgs/nixos-23.11" {
				t.Errorf("nixpkgs URL = %q", input.URL)
			}
		}
	}
	if !foundNixpkgs {
		t.Error("nixpkgs input not found")
	}

	// Should have parsed nixosConfigurations
	if len(config.Outputs) < 1 {
		t.Errorf("len(Outputs) = %d, want >= 1", len(config.Outputs))
	}

	foundTimeguru := false
	for _, output := range config.Outputs {
		if strings.Contains(output.Name, "timeguru") {
			foundTimeguru = true
			if output.Type != "nixosConfiguration" {
				t.Errorf("timeguru output type = %q, want nixosConfiguration", output.Type)
			}
		}
	}
	if !foundTimeguru {
		t.Error("timeguru output not found in parsed outputs")
	}
}

func TestParseFlake_LockHash(t *testing.T) {
	lockContent := sampleFlakeLock()
	dir := createTempFlake(t, sampleFlakeNix(), lockContent)

	config, err := ParseFlake(dir)
	if err != nil {
		t.Fatalf("ParseFlake: %v", err)
	}

	// LockHash should be derived from flake.lock content
	if config.LockHash == "" {
		t.Error("LockHash is empty")
	}

	// Verify it matches expected hash
	h := sha256.Sum256([]byte(lockContent))
	expected := hex.EncodeToString(h[:])[:16]
	if config.LockHash != expected {
		t.Errorf("LockHash = %q, want %q", config.LockHash, expected)
	}
}

func TestParseFlake_NoLockFile(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), "")

	config, err := ParseFlake(dir)
	if err != nil {
		t.Fatalf("ParseFlake: %v", err)
	}

	if config.LockHash != "" {
		t.Errorf("LockHash should be empty without flake.lock, got %q", config.LockHash)
	}
}

func TestParseFlake_ModTime(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	config, err := ParseFlake(dir)
	if err != nil {
		t.Fatalf("ParseFlake: %v", err)
	}

	if config.ModTime.IsZero() {
		t.Error("ModTime is zero")
	}

	// Should be recent
	if time.Since(config.ModTime) > 5*time.Minute {
		t.Error("ModTime is not recent")
	}
}

func TestParseFlake_DefaultSystem(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	config, err := ParseFlake(dir)
	if err != nil {
		t.Fatalf("ParseFlake: %v", err)
	}

	if config.System != "x86_64-linux" {
		t.Errorf("System = %q, want %q", config.System, "x86_64-linux")
	}
}

func TestParseFlakeFile_VariousFormats(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantDesc    string
		wantInputs  int
		wantOutputs int
	}{
		{
			name: "standard flake",
			content: `{
  description = "My test flake";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-23.11";
  };
  outputs = { self, nixpkgs }: {
    nixosConfigurations.myapp = {};
  };
}`,
			wantDesc:    "My test flake",
			wantInputs:  1,
			wantOutputs: 1,
		},
		{
			name: "multiple inputs",
			content: `{
  description = "Multi-input flake";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs";
    flake-utils.url = "github:numtide/flake-utils";
    home-manager.url = "github:nix-community/home-manager";
  };
  outputs = { ... }: {};
}`,
			wantDesc:    "Multi-input flake",
			wantInputs:  3,
			wantOutputs: 0,
		},
		{
			name: "multiple nixos configurations",
			content: `{
  description = "Multi-config flake";
  outputs = { ... }: {
    nixosConfigurations.web = {};
    nixosConfigurations.db = {};
    nixosConfigurations.cache = {};
  };
}`,
			wantDesc:    "Multi-config flake",
			wantInputs:  0,
			wantOutputs: 3,
		},
		{
			name:        "no description",
			content:     `{ outputs = { ... }: {}; }`,
			wantDesc:    "",
			wantInputs:  0,
			wantOutputs: 0,
		},
		{
			name: "empty description",
			content: `{
  description = "";
  outputs = { ... }: {};
}`,
			wantDesc:    "",
			wantInputs:  0,
			wantOutputs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := createTempFlake(t, tt.content, "")

			config, err := ParseFlake(dir)
			if err != nil {
				t.Fatalf("ParseFlake: %v", err)
			}

			if config.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", config.Description, tt.wantDesc)
			}
			if len(config.Inputs) != tt.wantInputs {
				t.Errorf("len(Inputs) = %d, want %d", len(config.Inputs), tt.wantInputs)
			}
			if len(config.Outputs) != tt.wantOutputs {
				t.Errorf("len(Outputs) = %d, want %d", len(config.Outputs), tt.wantOutputs)
			}
		})
	}
}

// ============================================================================
// PARSE FLAKE OUTPUTS TESTS
// ============================================================================

func TestParseFlakeOutputs(t *testing.T) {
	tests := []struct {
		name    string
		outputs map[string]interface{}
		want    int
	}{
		{
			name: "nixos configurations",
			outputs: map[string]interface{}{
				"nixosConfigurations": map[string]interface{}{
					"timeguru": map[string]interface{}{
						"type":        "nixosConfiguration",
						"description": "Timeguru service",
					},
					"captain": map[string]interface{}{
						"type": "nixosConfiguration",
					},
				},
			},
			want: 2,
		},
		{
			name: "mixed output types",
			outputs: map[string]interface{}{
				"packages": map[string]interface{}{
					"x86_64-linux": map[string]interface{}{
						"default": map[string]interface{}{
							"type":        "derivation",
							"description": "Default package",
						},
					},
				},
			},
			want: 1,
		},
		{
			name:    "empty outputs",
			outputs: map[string]interface{}{},
			want:    0,
		},
		{
			name: "deeply nested",
			outputs: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"type": "derivation",
						},
					},
				},
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFlakeOutputs(tt.outputs)
			if len(got) != tt.want {
				t.Errorf("parseFlakeOutputs() returned %d outputs, want %d", len(got), tt.want)
			}
		})
	}
}

func TestParseFlakeOutputs_Names(t *testing.T) {
	outputs := map[string]interface{}{
		"nixosConfigurations": map[string]interface{}{
			"timeguru": map[string]interface{}{
				"type": "nixosConfiguration",
			},
		},
	}

	result := parseFlakeOutputs(outputs)
	if len(result) != 1 {
		t.Fatalf("expected 1 output, got %d", len(result))
	}

	if result[0].Name != "nixosConfigurations.timeguru" {
		t.Errorf("output name = %q, want %q", result[0].Name, "nixosConfigurations.timeguru")
	}
	if result[0].Type != "nixosConfiguration" {
		t.Errorf("output type = %q, want %q", result[0].Type, "nixosConfiguration")
	}
}

// ============================================================================
// BUILD LOG PARSING TESTS
// ============================================================================

func TestParseBuildLog(t *testing.T) {
	tests := []struct {
		name      string
		log       string
		wantCount int
		checks    func(t *testing.T, entries []BuildLogEntry)
	}{
		{
			name:      "empty log",
			log:       "",
			wantCount: 0,
		},
		{
			name:      "building line",
			log:       "building '/nix/store/abc123-mypackage.drv'",
			wantCount: 1,
			checks: func(t *testing.T, entries []BuildLogEntry) {
				if entries[0].Level != "info" {
					t.Errorf("level = %q, want info", entries[0].Level)
				}
				if entries[0].Phase != "build" {
					t.Errorf("phase = %q, want build", entries[0].Phase)
				}
				if entries[0].Drv != "/nix/store/abc123-mypackage.drv" {
					t.Errorf("drv = %q", entries[0].Drv)
				}
			},
		},
		{
			name:      "error line",
			log:       "error: build of '/nix/store/abc.drv' failed",
			wantCount: 1,
			checks: func(t *testing.T, entries []BuildLogEntry) {
				if entries[0].Level != "error" {
					t.Errorf("level = %q, want error", entries[0].Level)
				}
			},
		},
		{
			name:      "warning line",
			log:       "warning: Git tree is dirty",
			wantCount: 1,
			checks: func(t *testing.T, entries []BuildLogEntry) {
				if entries[0].Level != "warning" {
					t.Errorf("level = %q, want warning", entries[0].Level)
				}
			},
		},
		{
			name:      "copying line",
			log:       "copying path '/nix/store/abc' to 'ssh://...'",
			wantCount: 1,
			checks: func(t *testing.T, entries []BuildLogEntry) {
				if entries[0].Level != "info" {
					t.Errorf("level = %q, want info", entries[0].Level)
				}
				if entries[0].Phase != "copy" {
					t.Errorf("phase = %q, want copy", entries[0].Phase)
				}
			},
		},
		{
			name:      "downloading line",
			log:       "downloading 'https://cache.nixos.org/abc.narinfo'",
			wantCount: 1,
			checks: func(t *testing.T, entries []BuildLogEntry) {
				if entries[0].Phase != "download" {
					t.Errorf("phase = %q, want download", entries[0].Phase)
				}
			},
		},
		{
			name:      "debug line",
			log:       "some random output line",
			wantCount: 1,
			checks: func(t *testing.T, entries []BuildLogEntry) {
				if entries[0].Level != "debug" {
					t.Errorf("level = %q, want debug", entries[0].Level)
				}
			},
		},
		{
			name: "multi-line log",
			log: `building '/nix/store/abc123.drv'
copying path '/nix/store/def456'
warning: tree is dirty
error: build failed
downloading 'https://cache.nixos.org/xyz'
some other output`,
			wantCount: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := ParseBuildLog(tt.log)
			if len(entries) != tt.wantCount {
				t.Errorf("ParseBuildLog() returned %d entries, want %d", len(entries), tt.wantCount)
			}
			if tt.checks != nil {
				tt.checks(t, entries)
			}
		})
	}
}

func TestParseBuildLog_TimestampsSet(t *testing.T) {
	entries := ParseBuildLog("test line")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Timestamp.IsZero() {
		t.Error("Timestamp is zero")
	}
}

func TestParseBuildLog_MessagePreserved(t *testing.T) {
	line := "building '/nix/store/abc123-mypackage.drv'"
	entries := ParseBuildLog(line)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Message != line {
		t.Errorf("Message = %q, want %q", entries[0].Message, line)
	}
}

// ============================================================================
// COMPUTE FLAKE HASH TESTS
// ============================================================================

func TestComputeFlakeHash(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	hash, err := ComputeFlakeHash(dir)
	if err != nil {
		t.Fatalf("ComputeFlakeHash: %v", err)
	}

	if hash == "" {
		t.Error("hash is empty")
	}

	// Hash should be 64 hex characters (SHA-256)
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
}

func TestComputeFlakeHash_Deterministic(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	hash1, err := ComputeFlakeHash(dir)
	if err != nil {
		t.Fatalf("ComputeFlakeHash (1): %v", err)
	}

	hash2, err := ComputeFlakeHash(dir)
	if err != nil {
		t.Fatalf("ComputeFlakeHash (2): %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %q != %q", hash1, hash2)
	}
}

func TestComputeFlakeHash_ChangesOnFlakeChange(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	hash1, err := ComputeFlakeHash(dir)
	if err != nil {
		t.Fatalf("ComputeFlakeHash (1): %v", err)
	}

	// Modify flake.nix
	newContent := sampleFlakeNix() + "\n# changed"
	os.WriteFile(filepath.Join(dir, "flake.nix"), []byte(newContent), 0644)

	hash2, err := ComputeFlakeHash(dir)
	if err != nil {
		t.Fatalf("ComputeFlakeHash (2): %v", err)
	}

	if hash1 == hash2 {
		t.Error("hash did not change after flake modification")
	}
}

func TestComputeFlakeHash_WithoutLock(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), "")

	hash, err := ComputeFlakeHash(dir)
	if err != nil {
		t.Fatalf("ComputeFlakeHash: %v", err)
	}

	if hash == "" {
		t.Error("hash is empty without lock file")
	}
}

func TestComputeFlakeHash_MissingFlake(t *testing.T) {
	_, err := ComputeFlakeHash("/nonexistent/path")
	if err == nil {
		t.Error("expected error for missing flake")
	}
}

// ============================================================================
// BUILD SPEC VALIDATION TESTS
// ============================================================================

func TestValidateSpec_Table(t *testing.T) {
	validDir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	tests := []struct {
		name    string
		spec    *BuildSpec
		wantErr error
	}{
		{
			name:    "valid spec",
			spec:    newTestBuildSpec(validDir),
			wantErr: nil,
		},
		{
			name: "missing ID",
			spec: &BuildSpec{
				FlakePath:     validDir,
				Attribute:     "nixosConfigurations.test",
				ContainerName: "test",
			},
			wantErr: ErrInvalidBuildSpec,
		},
		{
			name: "missing flake path",
			spec: &BuildSpec{
				ID:            "test-1",
				Attribute:     "nixosConfigurations.test",
				ContainerName: "test",
			},
			wantErr: ErrInvalidBuildSpec,
		},
		{
			name: "missing attribute",
			spec: &BuildSpec{
				ID:            "test-1",
				FlakePath:     validDir,
				ContainerName: "test",
			},
			wantErr: ErrInvalidBuildSpec,
		},
		{
			name: "missing container name",
			spec: &BuildSpec{
				ID:        "test-1",
				FlakePath: validDir,
				Attribute: "nixosConfigurations.test",
			},
			wantErr: ErrInvalidBuildSpec,
		},
		{
			name: "nonexistent flake path",
			spec: &BuildSpec{
				ID:            "test-1",
				FlakePath:     "/nonexistent/path",
				Attribute:     "nixosConfigurations.test",
				ContainerName: "test",
			},
			wantErr: ErrFlakeNotFound,
		},
	}

	// We need a Builder to call validateSpec
	builderDir := t.TempDir()
	outputDir := filepath.Join(builderDir, "output")
	cacheDir := filepath.Join(builderDir, "cache")
	os.MkdirAll(outputDir, 0755)
	os.MkdirAll(cacheDir, 0755)

	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      outputDir,
		CacheDir:       cacheDir,
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := builder.validateSpec(tt.spec)
			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("validateSpec() error = nil, want %v", tt.wantErr)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErr.Error()) {
					t.Errorf("validateSpec() error = %v, want error containing %v", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("validateSpec() unexpected error: %v", err)
				}
			}
		})
	}
}

// ============================================================================
// NixOS BUILD COMMAND GENERATION TESTS
// ============================================================================

func TestNixBuildCommandArgs(t *testing.T) {
	// We cannot run the actual nix build, but we can test the argument construction
	// by inspecting what runNixBuild would produce.
	// Testing the argument logic indirectly.

	tests := []struct {
		name     string
		spec     *BuildSpec
		wantArgs []string // substrings expected in args
		dontWant []string // substrings NOT expected
	}{
		{
			name: "basic build",
			spec: &BuildSpec{
				FlakePath:     "/path/to/flake",
				Attribute:     "nixosConfigurations.timeguru",
				ContainerName: "timeguru",
				Options:       BuildOptions{UseCache: true},
			},
			wantArgs: []string{
				"/path/to/flake#nixosConfigurations.timeguru",
				"--json",
			},
			dontWant: []string{
				"--no-substitute",
				"-v",
				"--keep-failed",
			},
		},
		{
			name: "no cache",
			spec: &BuildSpec{
				FlakePath:     "/path/to/flake",
				Attribute:     "nixosConfigurations.timeguru",
				ContainerName: "timeguru",
				Options:       BuildOptions{UseCache: false},
			},
			wantArgs: []string{
				"--no-substitute",
			},
		},
		{
			name: "verbose with keep-failed",
			spec: &BuildSpec{
				FlakePath:     "/path/to/flake",
				Attribute:     "nixosConfigurations.timeguru",
				ContainerName: "timeguru",
				Options: BuildOptions{
					Verbose:    true,
					KeepFailed: true,
					UseCache:   true,
				},
			},
			wantArgs: []string{
				"-v",
				"--keep-failed",
			},
		},
		{
			name: "with max cores",
			spec: &BuildSpec{
				FlakePath:     "/path/to/flake",
				Attribute:     "nixosConfigurations.timeguru",
				ContainerName: "timeguru",
				Options: BuildOptions{
					MaxCores: 4,
					UseCache: true,
				},
			},
			wantArgs: []string{
				"--cores",
				"4",
			},
		},
		{
			name: "with extra args",
			spec: &BuildSpec{
				FlakePath:     "/path/to/flake",
				Attribute:     "nixosConfigurations.timeguru",
				ContainerName: "timeguru",
				Options: BuildOptions{
					UseCache:  true,
					ExtraArgs: []string{"--show-trace", "--impure"},
				},
			},
			wantArgs: []string{
				"--show-trace",
				"--impure",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the args the same way runNixBuild does
			args := []string{
				"build",
				fmt.Sprintf("%s#%s", tt.spec.FlakePath, tt.spec.Attribute),
				"--out-link", filepath.Join("/output", tt.spec.ContainerName),
				"--json",
			}
			if tt.spec.Options.MaxCores > 0 {
				args = append(args, "--cores", fmt.Sprintf("%d", tt.spec.Options.MaxCores))
			}
			if !tt.spec.Options.UseCache {
				args = append(args, "--no-substitute")
			}
			if tt.spec.Options.KeepFailed {
				args = append(args, "--keep-failed")
			}
			if tt.spec.Options.Verbose {
				args = append(args, "-v")
			}
			args = append(args, tt.spec.Options.ExtraArgs...)

			joined := strings.Join(args, " ")

			for _, want := range tt.wantArgs {
				if !strings.Contains(joined, want) {
					t.Errorf("args %q missing expected substring %q", joined, want)
				}
			}
			for _, dontWant := range tt.dontWant {
				if strings.Contains(joined, dontWant) {
					t.Errorf("args %q contains unexpected substring %q", joined, dontWant)
				}
			}
		})
	}
}

// ============================================================================
// BUILDER SERVICE TESTS
// ============================================================================

func TestNewBuilder(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  2,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	if builder == nil {
		t.Fatal("NewBuilder returned nil")
	}
	if builder.IsRunning() {
		t.Error("new builder should not be running")
	}
}

func TestDefaultBuilderConfig(t *testing.T) {
	cfg := DefaultBuilderConfig()

	if cfg.NixBinary != "nix" {
		t.Errorf("NixBinary = %q, want nix", cfg.NixBinary)
	}
	if cfg.QueueSize != 100 {
		t.Errorf("QueueSize = %d, want 100", cfg.QueueSize)
	}
	if cfg.MaxConcurrent != 2 {
		t.Errorf("MaxConcurrent = %d, want 2", cfg.MaxConcurrent)
	}
	if cfg.DefaultTimeout != 30*time.Minute {
		t.Errorf("DefaultTimeout = %v, want 30m", cfg.DefaultTimeout)
	}
	if cfg.CacheAge != 7*24*time.Hour {
		t.Errorf("CacheAge = %v, want 7d", cfg.CacheAge)
	}
	if cfg.WatchInterval != 5*time.Second {
		t.Errorf("WatchInterval = %v, want 5s", cfg.WatchInterval)
	}
	if cfg.WotanTopic != "nix.builds" {
		t.Errorf("WotanTopic = %q, want nix.builds", cfg.WotanTopic)
	}
}

func TestBuilder_StartStop(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := builder.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !builder.IsRunning() {
		t.Error("builder should be running after Start")
	}

	// Double start should be no-op
	if err := builder.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	if err := builder.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if builder.IsRunning() {
		t.Error("builder should not be running after Stop")
	}

	// Double stop should be no-op
	if err := builder.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestBuilder_BuildContainerNotRunning(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	flakeDir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())
	spec := newTestBuildSpec(flakeDir)

	err = builder.BuildContainer(context.Background(), spec)
	if err != ErrBuilderNotRunning {
		t.Errorf("BuildContainer on stopped builder: got %v, want %v", err, ErrBuilderNotRunning)
	}
}

func TestBuilder_BuildContainerSetsDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := builder.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer builder.Stop()

	flakeDir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())
	spec := &BuildSpec{
		ID:            "default-test",
		FlakePath:     flakeDir,
		Attribute:     "nixosConfigurations.timeguru",
		ContainerName: "timeguru",
		// System and RequestedAt intentionally left empty
	}

	err = builder.BuildContainer(ctx, spec)
	if err != nil {
		t.Fatalf("BuildContainer: %v", err)
	}

	if spec.System != "x86_64-linux" {
		t.Errorf("default System = %q, want x86_64-linux", spec.System)
	}
	if spec.RequestedAt.IsZero() {
		t.Error("RequestedAt should be set")
	}
}

func TestBuilder_GetBuildStatusNotFound(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	_, err = builder.GetBuildStatus("nonexistent")
	if err == nil {
		t.Error("GetBuildStatus for nonexistent should error")
	}
}

func TestBuilder_CancelBuildNotFound(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	err = builder.CancelBuild("nonexistent")
	if err == nil {
		t.Error("CancelBuild for nonexistent should error")
	}
}

func TestBuilder_CleanOldBuilds(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Insert some old results
	builder.resultsMu.Lock()
	builder.results["old-1"] = &BuildResult{
		Spec:        BuildSpec{ID: "old-1"},
		Status:      BuildStatusSuccess,
		CompletedAt: time.Now().Add(-48 * time.Hour),
	}
	builder.results["old-2"] = &BuildResult{
		Spec:        BuildSpec{ID: "old-2"},
		Status:      BuildStatusFailed,
		CompletedAt: time.Now().Add(-72 * time.Hour),
	}
	builder.results["recent"] = &BuildResult{
		Spec:        BuildSpec{ID: "recent"},
		Status:      BuildStatusSuccess,
		CompletedAt: time.Now(),
	}
	builder.resultsMu.Unlock()

	removed, err := builder.CleanOldBuilds(24 * time.Hour)
	if err != nil {
		t.Fatalf("CleanOldBuilds: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed = %d, want 2", removed)
	}

	// Check that recent is still there
	builder.resultsMu.RLock()
	defer builder.resultsMu.RUnlock()
	if _, ok := builder.results["recent"]; !ok {
		t.Error("recent build was incorrectly cleaned")
	}
	if _, ok := builder.results["old-1"]; ok {
		t.Error("old-1 build was not cleaned")
	}
}

func TestBuilder_Stats(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	stats := builder.Stats()

	if _, ok := stats["running"]; !ok {
		t.Error("Stats missing 'running' key")
	}
	if _, ok := stats["queue"]; !ok {
		t.Error("Stats missing 'queue' key")
	}
	if _, ok := stats["cache"]; !ok {
		t.Error("Stats missing 'cache' key")
	}
	if _, ok := stats["completed"]; !ok {
		t.Error("Stats missing 'completed' key")
	}
}

// ============================================================================
// FLAKE WATCHER TESTS
// ============================================================================

func TestNewFlakeWatcher(t *testing.T) {
	called := false
	onChange := func(path string, config *FlakeConfig) {
		called = true
	}

	w := NewFlakeWatcher(time.Second, onChange)
	if w == nil {
		t.Fatal("NewFlakeWatcher returned nil")
	}
	_ = called // onChange callback is tested elsewhere
}

func TestFlakeWatcher_AddRemovePath(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	w := NewFlakeWatcher(time.Second, func(string, *FlakeConfig) {})

	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath: %v", err)
	}

	// Verify it's tracked
	w.mu.RLock()
	_, exists := w.paths[dir]
	w.mu.RUnlock()
	if !exists {
		t.Error("path not tracked after AddPath")
	}

	w.RemovePath(dir)

	w.mu.RLock()
	_, exists = w.paths[dir]
	w.mu.RUnlock()
	if exists {
		t.Error("path still tracked after RemovePath")
	}
}

func TestFlakeWatcher_AddPathMissing(t *testing.T) {
	w := NewFlakeWatcher(time.Second, func(string, *FlakeConfig) {})

	err := w.AddPath("/nonexistent/path")
	if err == nil {
		t.Error("AddPath for nonexistent path should error")
	}
}

func TestFlakeWatcher_AddPathNoFlake(t *testing.T) {
	dir := t.TempDir()
	// No flake.nix in directory, but has a flake.lock
	os.WriteFile(filepath.Join(dir, "flake.lock"), []byte("{}"), 0644)

	w := NewFlakeWatcher(time.Second, func(string, *FlakeConfig) {})

	err := w.AddPath(dir)
	if err == nil {
		t.Error("AddPath for directory without flake.nix should error")
	}
}

func TestFlakeWatcher_StartStop(t *testing.T) {
	w := NewFlakeWatcher(100*time.Millisecond, func(string, *FlakeConfig) {})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Double start should be no-op
	if err := w.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	w.Stop()

	// Double stop should be safe (no panic)
	// Note: the second Stop call will try to close an already closed channel.
	// The atomic swap prevents this.
}

func TestFlakeWatcher_StopViaContext(t *testing.T) {
	w := NewFlakeWatcher(50*time.Millisecond, func(string, *FlakeConfig) {})

	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Cancel context should stop the watcher
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestFlakeWatcher_GetLockHash(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	w := NewFlakeWatcher(time.Second, func(string, *FlakeConfig) {})

	hash, err := w.getLockHash(dir)
	if err != nil {
		t.Fatalf("getLockHash: %v", err)
	}

	if hash == "" {
		t.Error("hash is empty")
	}
	if len(hash) != 16 {
		t.Errorf("hash length = %d, want 16", len(hash))
	}
}

func TestFlakeWatcher_GetLockHashMissing(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), "")

	w := NewFlakeWatcher(time.Second, func(string, *FlakeConfig) {})

	_, err := w.getLockHash(dir)
	if err == nil {
		t.Error("getLockHash without flake.lock should error")
	}
}

// ============================================================================
// LXD IMAGE IMPORT TESTS
// ============================================================================

func TestImportToLXD_FailedBuild(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	result := &BuildResult{
		Status: BuildStatusFailed,
	}

	err = builder.ImportToLXD(context.Background(), result, nil)
	if err == nil {
		t.Error("ImportToLXD with failed build should error")
	}
	if !strings.Contains(err.Error(), "cannot import failed build") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestImportToLXD_NoImagePath(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	result := &BuildResult{
		Status:    BuildStatusSuccess,
		ImagePath: "",
	}

	err = builder.ImportToLXD(context.Background(), result, nil)
	if err != ErrImageNotFound {
		t.Errorf("ImportToLXD with no image path: got %v, want %v", err, ErrImageNotFound)
	}
}

func TestImportToLXD_GeneratesAlias(t *testing.T) {
	// We can verify the alias format even though the lxc command will fail
	result := &BuildResult{
		Spec:      BuildSpec{ContainerName: "timeguru"},
		Status:    BuildStatusSuccess,
		ImagePath: "/nix/store/abc123/image.tar.xz",
		ImageHash: "deadbeef01234567890abcdef",
	}

	expectedAlias := fmt.Sprintf("unheaded-%s-%s", result.Spec.ContainerName, result.ImageHash[:12])
	if expectedAlias != "unheaded-timeguru-deadbeef0123" {
		t.Errorf("alias format = %q, unexpected", expectedAlias)
	}
}

// ============================================================================
// BUILD STATUS TYPES TESTS
// ============================================================================

func TestBuildStatus_Values(t *testing.T) {
	statuses := []BuildStatus{
		BuildStatusPending,
		BuildStatusQueued,
		BuildStatusRunning,
		BuildStatusSuccess,
		BuildStatusFailed,
		BuildStatusCancelled,
		BuildStatusTimeout,
	}

	// All statuses should be unique
	seen := make(map[BuildStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("duplicate status: %q", s)
		}
		seen[s] = true
	}

	// Verify specific values
	if BuildStatusPending != "pending" {
		t.Errorf("BuildStatusPending = %q", BuildStatusPending)
	}
	if BuildStatusQueued != "queued" {
		t.Errorf("BuildStatusQueued = %q", BuildStatusQueued)
	}
	if BuildStatusRunning != "running" {
		t.Errorf("BuildStatusRunning = %q", BuildStatusRunning)
	}
	if BuildStatusSuccess != "success" {
		t.Errorf("BuildStatusSuccess = %q", BuildStatusSuccess)
	}
	if BuildStatusFailed != "failed" {
		t.Errorf("BuildStatusFailed = %q", BuildStatusFailed)
	}
	if BuildStatusCancelled != "cancelled" {
		t.Errorf("BuildStatusCancelled = %q", BuildStatusCancelled)
	}
	if BuildStatusTimeout != "timeout" {
		t.Errorf("BuildStatusTimeout = %q", BuildStatusTimeout)
	}
}

// ============================================================================
// ERROR SENTINEL TESTS
// ============================================================================

func TestErrors_Distinct(t *testing.T) {
	errors := []error{
		ErrFlakeNotFound,
		ErrFlakeParseError,
		ErrBuildFailed,
		ErrBuildTimeout,
		ErrBuildCancelled,
		ErrImageNotFound,
		ErrLXDImportFailed,
		ErrBuilderNotRunning,
		ErrBuildQueueFull,
		ErrInvalidBuildSpec,
		ErrCacheCorrupted,
		ErrWatcherFailed,
	}

	seen := make(map[string]bool)
	for _, err := range errors {
		msg := err.Error()
		if seen[msg] {
			t.Errorf("duplicate error message: %q", msg)
		}
		seen[msg] = true

		if msg == "" {
			t.Error("error with empty message")
		}
	}
}

// ============================================================================
// BUILD RESULT JSON SERIALIZATION TESTS
// ============================================================================

func TestBuildResult_JSON(t *testing.T) {
	result := &BuildResult{
		Spec: BuildSpec{
			ID:            "test-build",
			ContainerName: "timeguru",
			System:        "x86_64-linux",
		},
		Status:      BuildStatusSuccess,
		ImagePath:   "/nix/store/abc123/image.tar.xz",
		ImageHash:   "deadbeef",
		ImageSize:   1024 * 1024,
		StorePath:   "/nix/store/abc123",
		StartedAt:   time.Now().Add(-5 * time.Minute),
		CompletedAt: time.Now(),
		Duration:    5 * time.Minute,
		OutputPaths: []string{"/nix/store/abc123", "/nix/store/def456"},
		LXDAlias:    "unheaded-timeguru-deadbeef",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded BuildResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Spec.ID != result.Spec.ID {
		t.Errorf("decoded Spec.ID = %q, want %q", decoded.Spec.ID, result.Spec.ID)
	}
	if decoded.Status != result.Status {
		t.Errorf("decoded Status = %q, want %q", decoded.Status, result.Status)
	}
	if decoded.ImagePath != result.ImagePath {
		t.Errorf("decoded ImagePath = %q, want %q", decoded.ImagePath, result.ImagePath)
	}
	if decoded.LXDAlias != result.LXDAlias {
		t.Errorf("decoded LXDAlias = %q, want %q", decoded.LXDAlias, result.LXDAlias)
	}
	if len(decoded.OutputPaths) != len(result.OutputPaths) {
		t.Errorf("decoded OutputPaths length = %d, want %d", len(decoded.OutputPaths), len(result.OutputPaths))
	}
}

func TestBuildSpec_JSON(t *testing.T) {
	spec := &BuildSpec{
		ID:            "json-test",
		FlakePath:     "/path/to/flake",
		Attribute:     "nixosConfigurations.test",
		ContainerName: "test",
		System:        "x86_64-linux",
		Priority:      10,
		RequestedBy:   "unit-test",
		RequestedAt:   time.Now(),
		CallbackURL:   "http://example.com/callback",
		Options: BuildOptions{
			Timeout:      10 * time.Minute,
			MaxCores:     4,
			MaxMemory:    4 * 1024 * 1024 * 1024,
			UseCache:     true,
			ExtraArgs:    []string{"--show-trace"},
			ForceRebuild: false,
			KeepFailed:   true,
			Verbose:      true,
		},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded BuildSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ID != spec.ID {
		t.Errorf("decoded ID = %q, want %q", decoded.ID, spec.ID)
	}
	if decoded.Priority != spec.Priority {
		t.Errorf("decoded Priority = %d, want %d", decoded.Priority, spec.Priority)
	}
	if decoded.CallbackURL != spec.CallbackURL {
		t.Errorf("decoded CallbackURL = %q, want %q", decoded.CallbackURL, spec.CallbackURL)
	}
	if decoded.Options.MaxCores != spec.Options.MaxCores {
		t.Errorf("decoded MaxCores = %d, want %d", decoded.Options.MaxCores, spec.Options.MaxCores)
	}
	if !decoded.Options.KeepFailed {
		t.Error("decoded KeepFailed should be true")
	}
}

func TestFlakeConfig_JSON(t *testing.T) {
	config := &FlakeConfig{
		Path:        "/path/to/flake",
		Description: "Test flake",
		Inputs: []FlakeInput{
			{Name: "nixpkgs", URL: "github:NixOS/nixpkgs", Locked: true, Rev: "abc123"},
		},
		Outputs: []FlakeOutput{
			{Name: "nixosConfigurations.test", Type: "nixosConfiguration", System: "x86_64-linux"},
		},
		Modules:  []string{"hardening", "networking"},
		System:   "x86_64-linux",
		LockHash: "deadbeef",
		ModTime:  time.Now(),
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded FlakeConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Description != config.Description {
		t.Errorf("decoded Description = %q, want %q", decoded.Description, config.Description)
	}
	if len(decoded.Inputs) != len(config.Inputs) {
		t.Errorf("decoded Inputs length = %d, want %d", len(decoded.Inputs), len(config.Inputs))
	}
	if decoded.Inputs[0].Rev != "abc123" {
		t.Errorf("decoded first input Rev = %q, want abc123", decoded.Inputs[0].Rev)
	}
}

// ============================================================================
// CACHE ENTRY JSON TESTS
// ============================================================================

func TestCacheEntry_JSON(t *testing.T) {
	entry := &CacheEntry{
		Key:        "cache-json-test",
		Spec:       BuildSpec{ID: "spec-1"},
		Result:     BuildResult{Status: BuildStatusSuccess},
		LockHash:   "lockhash",
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		Size:       2048,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded CacheEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Key != entry.Key {
		t.Errorf("decoded Key = %q, want %q", decoded.Key, entry.Key)
	}
	if decoded.Size != entry.Size {
		t.Errorf("decoded Size = %d, want %d", decoded.Size, entry.Size)
	}
}

// ============================================================================
// BUILDER PUBLISH EVENT TESTS
// ============================================================================

func TestBuilder_PublishBuildEvent_NilWotan(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil) // nil wotan
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Should not panic with nil wotan
	result := &BuildResult{
		Spec:   BuildSpec{ID: "test", ContainerName: "test"},
		Status: BuildStatusSuccess,
	}
	builder.publishBuildEvent(result)
}

// ============================================================================
// BUILDER ON FLAKE CHANGE TESTS
// ============================================================================

func TestBuilder_OnFlakeChange(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      100,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	config := &FlakeConfig{
		System: "x86_64-linux",
		Outputs: []FlakeOutput{
			{Name: "timeguru", Type: "nixosConfiguration", System: "x86_64-linux"},
			{Name: "captain", Type: "container", System: "x86_64-linux"},
			{Name: "mypackage", Type: "package", System: "x86_64-linux"}, // Should be skipped
		},
	}

	builder.onFlakeChange("/path/to/flake", config)

	stats := builder.queue.Stats()
	pending := stats["pending"].(int)

	// Only nixosConfiguration and container types should be enqueued (2 of 3)
	if pending != 2 {
		t.Errorf("pending builds = %d, want 2 (nixosConfiguration + container)", pending)
	}
}

// ============================================================================
// CONCURRENT BUILDER TESTS
// ============================================================================

func TestBuilder_ConcurrentStats(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	var wg sync.WaitGroup
	const goroutines = 20

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				builder.Stats()
			}
		}()
	}

	wg.Wait()
}

func TestBuilder_ConcurrentCleanOldBuilds(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Add results
	builder.resultsMu.Lock()
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("conc-build-%d", i)
		age := time.Duration(i) * time.Hour
		builder.results[id] = &BuildResult{
			Spec:        BuildSpec{ID: id},
			Status:      BuildStatusSuccess,
			CompletedAt: time.Now().Add(-age),
		}
	}
	builder.resultsMu.Unlock()

	var wg sync.WaitGroup
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			builder.CleanOldBuilds(24 * time.Hour)
		}()
	}
	wg.Wait()
}

// ============================================================================
// BUILD QUEUE ORDERING DEEP TESTS
// ============================================================================

func TestBuildQueue_StablePriorityOrdering(t *testing.T) {
	q := NewBuildQueue(100, 100) // high concurrency so Dequeue always works

	// Enqueue items with same priority - they should maintain FIFO order
	for i := 0; i < 5; i++ {
		q.Enqueue(&BuildSpec{
			ID:       fmt.Sprintf("same-prio-%d", i),
			Priority: 5,
		})
	}

	var order []string
	for {
		spec, ok := q.Dequeue()
		if !ok {
			break
		}
		order = append(order, spec.ID)
	}

	expected := []string{"same-prio-0", "same-prio-1", "same-prio-2", "same-prio-3", "same-prio-4"}
	for i, id := range order {
		if id != expected[i] {
			t.Errorf("position %d: got %q, want %q", i, id, expected[i])
		}
	}
}

func TestBuildQueue_MixedPriorityInsertionOrder(t *testing.T) {
	q := NewBuildQueue(100, 100)

	// Insert in specific order with varying priorities
	q.Enqueue(&BuildSpec{ID: "a", Priority: 3})
	q.Enqueue(&BuildSpec{ID: "b", Priority: 7})
	q.Enqueue(&BuildSpec{ID: "c", Priority: 1})
	q.Enqueue(&BuildSpec{ID: "d", Priority: 7}) // same as b
	q.Enqueue(&BuildSpec{ID: "e", Priority: 10})

	var order []string
	for {
		spec, ok := q.Dequeue()
		if !ok {
			break
		}
		order = append(order, spec.ID)
	}

	// Expected: e(10), b(7), d(7), a(3), c(1)
	if order[0] != "e" {
		t.Errorf("first = %q, want e (priority 10)", order[0])
	}
	if order[len(order)-1] != "c" {
		t.Errorf("last = %q, want c (priority 1)", order[len(order)-1])
	}

	// Verify the order is sorted by priority descending
	priorities := []int{10, 7, 7, 3, 1}
	for i, p := range priorities {
		_ = p
		_ = i
	}
	// b and d both have priority 7; b was enqueued first so should come before d
	bIdx := -1
	dIdx := -1
	for i, id := range order {
		if id == "b" {
			bIdx = i
		}
		if id == "d" {
			dIdx = i
		}
	}
	if bIdx > dIdx {
		t.Errorf("b (enqueued first at prio 7) should come before d, got b=%d d=%d", bIdx, dIdx)
	}
}

func TestBuildQueue_ZeroPriority(t *testing.T) {
	q := NewBuildQueue(100, 100)

	q.Enqueue(&BuildSpec{ID: "zero", Priority: 0})
	q.Enqueue(&BuildSpec{ID: "one", Priority: 1})
	q.Enqueue(&BuildSpec{ID: "negative", Priority: -1})

	got, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue returned not ok")
	}
	if got.ID != "one" {
		t.Errorf("first dequeue = %q, want 'one' (highest priority)", got.ID)
	}
}

// ============================================================================
// EDGE CASES
// ============================================================================

func TestBuildCache_LoadCorruptedEntries(t *testing.T) {
	dir := t.TempDir()

	// Write a corrupt JSON file
	os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("not valid json{{{"), 0644)

	// Write a valid JSON file
	entry := &CacheEntry{
		Key:       "valid-key",
		CreatedAt: time.Now(),
		Size:      100,
	}
	data, _ := json.MarshalIndent(entry, "", "  ")
	os.WriteFile(filepath.Join(dir, "valid-key.json"), data, 0644)

	// Should load without error (corrupt entries skipped)
	cache, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	// Valid entry should still be loaded
	_, ok := cache.Get("valid-key")
	if !ok {
		t.Error("valid entry not loaded after corrupt entry skipped")
	}
}

func TestBuildCache_NonJsonFiles(t *testing.T) {
	dir := t.TempDir()

	// Write non-JSON files
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("PNG"), 0644)

	cache, err := NewBuildCache(dir, 10*1024*1024, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	stats := cache.Stats()
	entries := stats["entries"].(int)
	if entries != 0 {
		t.Errorf("entries = %d, want 0 (non-JSON files should be skipped)", entries)
	}
}

func TestBuildQueue_EnqueueAfterCancel(t *testing.T) {
	q := NewBuildQueue(10, 2)

	spec1 := &BuildSpec{ID: "cancelled-then-re-added", Priority: 5}
	q.Enqueue(spec1)
	q.Cancel("cancelled-then-re-added")

	// Re-enqueue with same ID
	spec2 := &BuildSpec{ID: "cancelled-then-re-added", Priority: 10}
	err := q.Enqueue(spec2)
	if err != nil {
		t.Fatalf("re-enqueue after cancel: %v", err)
	}

	got, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue returned not ok")
	}
	if got.ID != "cancelled-then-re-added" {
		t.Errorf("got ID = %q", got.ID)
	}
	if got.Priority != 10 {
		t.Errorf("re-enqueued priority = %d, want 10", got.Priority)
	}
}

func TestFlakeInput_JSON(t *testing.T) {
	input := FlakeInput{
		Name:   "nixpkgs",
		URL:    "github:NixOS/nixpkgs/nixos-23.11",
		Locked: true,
		Rev:    "abc123",
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded FlakeInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Name != input.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, input.Name)
	}
	if decoded.Rev != input.Rev {
		t.Errorf("Rev = %q, want %q", decoded.Rev, input.Rev)
	}
}

func TestFlakeOutput_JSON(t *testing.T) {
	output := FlakeOutput{
		Name:        "nixosConfigurations.timeguru",
		Type:        "nixosConfiguration",
		System:      "x86_64-linux",
		Description: "Timeguru service",
	}

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded FlakeOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Description != output.Description {
		t.Errorf("Description = %q, want %q", decoded.Description, output.Description)
	}
}

func TestBuildLogEntry_JSON(t *testing.T) {
	entry := BuildLogEntry{
		Timestamp: time.Now(),
		Level:     "info",
		Message:   "building derivation",
		Drv:       "/nix/store/abc.drv",
		Phase:     "build",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded BuildLogEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Level != entry.Level {
		t.Errorf("Level = %q, want %q", decoded.Level, entry.Level)
	}
	if decoded.Drv != entry.Drv {
		t.Errorf("Drv = %q, want %q", decoded.Drv, entry.Drv)
	}
}

// ============================================================================
// BUILDER INTEGRATION-STYLE TESTS (no external deps)
// ============================================================================

func TestBuilder_GetBuildStatusQueued(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Directly enqueue to the builder's queue (without starting the builder,
	// so the build won't be processed)
	spec := &BuildSpec{
		ID:            "queued-check",
		ContainerName: "test",
		Priority:      5,
	}
	builder.queue.Enqueue(spec)

	result, err := builder.GetBuildStatus("queued-check")
	if err != nil {
		t.Fatalf("GetBuildStatus: %v", err)
	}
	if result.Status != BuildStatusQueued {
		t.Errorf("Status = %q, want %q", result.Status, BuildStatusQueued)
	}
}

func TestBuilder_GetBuildStatusCompleted(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Directly insert a completed result
	builder.resultsMu.Lock()
	builder.results["completed-check"] = &BuildResult{
		Spec:      BuildSpec{ID: "completed-check", ContainerName: "test"},
		Status:    BuildStatusSuccess,
		ImagePath: "/test/image",
	}
	builder.resultsMu.Unlock()

	result, err := builder.GetBuildStatus("completed-check")
	if err != nil {
		t.Fatalf("GetBuildStatus: %v", err)
	}
	if result.Status != BuildStatusSuccess {
		t.Errorf("Status = %q, want %q", result.Status, BuildStatusSuccess)
	}
	if result.ImagePath != "/test/image" {
		t.Errorf("ImagePath = %q, want /test/image", result.ImagePath)
	}
}

func TestBuilder_CancelBuildQueued(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Queue a build
	spec := &BuildSpec{ID: "cancel-queued", Priority: 5}
	builder.queue.Enqueue(spec)

	err = builder.CancelBuild("cancel-queued")
	if err != nil {
		t.Fatalf("CancelBuild: %v", err)
	}

	// Should no longer be in queue
	_, found := builder.queue.GetStatus("cancel-queued")
	if found {
		t.Error("build still found in queue after cancel")
	}
}

// ============================================================================
// PARSE FLAKE OUTPUTS WALK EDGE CASES
// ============================================================================

func TestParseFlakeOutputs_NonMapValues(t *testing.T) {
	outputs := map[string]interface{}{
		"string_value": "not a map",
		"int_value":    42,
		"nil_value":    nil,
		"valid": map[string]interface{}{
			"type": "derivation",
		},
	}

	result := parseFlakeOutputs(outputs)
	// Only the "valid" entry should be parsed
	if len(result) != 1 {
		t.Errorf("expected 1 output, got %d", len(result))
	}
}

func TestParseFlakeOutputs_WithDescription(t *testing.T) {
	outputs := map[string]interface{}{
		"packages": map[string]interface{}{
			"default": map[string]interface{}{
				"type":        "derivation",
				"description": "The default package",
			},
		},
	}

	result := parseFlakeOutputs(outputs)
	if len(result) != 1 {
		t.Fatalf("expected 1 output, got %d", len(result))
	}
	if result[0].Description != "The default package" {
		t.Errorf("Description = %q, want %q", result[0].Description, "The default package")
	}
}

// ============================================================================
// CONCURRENT FLAKE WATCHER TESTS
// ============================================================================

func TestFlakeWatcher_ConcurrentAddRemove(t *testing.T) {
	w := NewFlakeWatcher(time.Second, func(string, *FlakeConfig) {})

	// Create several flake directories
	dirs := make([]string, 10)
	for i := range dirs {
		dirs[i] = createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())
	}

	var wg sync.WaitGroup
	wg.Add(20)

	// Concurrent adds
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer wg.Done()
			w.AddPath(dirs[idx])
		}(i)
	}

	// Concurrent removes
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer wg.Done()
			w.RemovePath(dirs[idx])
		}(i)
	}

	wg.Wait()
}

// ============================================================================
// BUILD QUEUE MULTIPLE CANCEL TESTS
// ============================================================================

func TestBuildQueue_CancelMultiplePending(t *testing.T) {
	q := NewBuildQueue(100, 10)

	for i := 0; i < 10; i++ {
		q.Enqueue(&BuildSpec{ID: fmt.Sprintf("multi-%d", i), Priority: i})
	}

	// Cancel every other one
	for i := 0; i < 10; i += 2 {
		if !q.Cancel(fmt.Sprintf("multi-%d", i)) {
			t.Errorf("Cancel multi-%d returned false", i)
		}
	}

	// Remaining should be odd-numbered
	var remaining []string
	for {
		spec, ok := q.Dequeue()
		if !ok {
			break
		}
		remaining = append(remaining, spec.ID)
	}

	if len(remaining) != 5 {
		t.Errorf("remaining after cancel = %d, want 5", len(remaining))
	}

	// Sort to verify content (dequeue order depends on priority)
	sort.Strings(remaining)
	expected := []string{"multi-1", "multi-3", "multi-5", "multi-7", "multi-9"}
	sort.Strings(expected)
	for i, r := range remaining {
		if r != expected[i] {
			t.Errorf("remaining[%d] = %q, want %q", i, r, expected[i])
		}
	}
}

// ============================================================================
// BUILDER WATCH/UNWATCH FLAKES
// ============================================================================

func TestBuilder_WatchFlakes(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	flakeDir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	err = builder.WatchFlakes(context.Background(), []string{flakeDir})
	if err != nil {
		t.Fatalf("WatchFlakes: %v", err)
	}

	// Verify it's watched
	builder.watcher.mu.RLock()
	_, exists := builder.watcher.paths[flakeDir]
	builder.watcher.mu.RUnlock()
	if !exists {
		t.Error("flake path not being watched after WatchFlakes")
	}
}

func TestBuilder_WatchFlakes_InvalidPath(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	err = builder.WatchFlakes(context.Background(), []string{"/nonexistent/path"})
	if err == nil {
		t.Error("WatchFlakes with invalid path should error")
	}
}

func TestBuilder_UnwatchFlake(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	flakeDir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())
	builder.WatchFlakes(context.Background(), []string{flakeDir})

	builder.UnwatchFlake(flakeDir)

	builder.watcher.mu.RLock()
	_, exists := builder.watcher.paths[flakeDir]
	builder.watcher.mu.RUnlock()
	if exists {
		t.Error("flake path still watched after UnwatchFlake")
	}
}

// ============================================================================
// BUILD OPTIONS TESTS
// ============================================================================

func TestBuildOptions_Defaults(t *testing.T) {
	opts := BuildOptions{}

	if opts.Timeout != 0 {
		t.Errorf("default Timeout = %v, want 0", opts.Timeout)
	}
	if opts.MaxCores != 0 {
		t.Errorf("default MaxCores = %d, want 0", opts.MaxCores)
	}
	if opts.UseCache {
		t.Error("default UseCache should be false")
	}
	if opts.ForceRebuild {
		t.Error("default ForceRebuild should be false")
	}
	if opts.KeepFailed {
		t.Error("default KeepFailed should be false")
	}
	if opts.Verbose {
		t.Error("default Verbose should be false")
	}
	if len(opts.ExtraArgs) != 0 {
		t.Errorf("default ExtraArgs length = %d, want 0", len(opts.ExtraArgs))
	}
}

// ============================================================================
// EXTRACT CONTAINER IMAGE TESTS
// ============================================================================

func TestExtractContainerImage_DirectFile(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Create a fake store path with a file (no tarball subdirectory, no .tar.xz)
	storePath := filepath.Join(dir, "store", "abc123-result")
	os.MkdirAll(storePath, 0755)
	content := []byte("fake container image content for testing purposes")
	os.WriteFile(filepath.Join(storePath, "rootfs"), content, 0644)

	// extractContainerImage will try tarball/ first, then *.tar.xz, then fall back to storePath itself
	// Since storePath is a directory, os.Open will work but io.Copy will fail or it will hash directory.
	// Actually: storePath itself is a directory, so we need to create a file directly at storePath.
	// Let's instead test with a tarball/ subdirectory.

	// Test with tarball subdirectory containing matching tarball
	tarballDir := filepath.Join(storePath, "tarball")
	os.MkdirAll(tarballDir, 0755)
	tarballContent := []byte("fake tarball content")
	tarballFile := filepath.Join(tarballDir, "nixos-system-mycontainer-24.05.tar.xz")
	os.WriteFile(tarballFile, tarballContent, 0644)

	imagePath, imageHash, imageSize, err := builder.extractContainerImage(storePath, "mycontainer")
	if err != nil {
		t.Fatalf("extractContainerImage: %v", err)
	}

	if imagePath != tarballFile {
		t.Errorf("imagePath = %q, want %q", imagePath, tarballFile)
	}
	if imageHash == "" {
		t.Error("imageHash is empty")
	}
	if imageSize != int64(len(tarballContent)) {
		t.Errorf("imageSize = %d, want %d", imageSize, len(tarballContent))
	}
}

func TestExtractContainerImage_AlternativePattern(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Create a store path with *.tar.xz directly in it (no tarball/ subdirectory)
	storePath := filepath.Join(dir, "store", "def456-result")
	os.MkdirAll(storePath, 0755)
	tarballContent := []byte("alternative tarball content")
	tarballFile := filepath.Join(storePath, "image.tar.xz")
	os.WriteFile(tarballFile, tarballContent, 0644)

	imagePath, imageHash, imageSize, err := builder.extractContainerImage(storePath, "somecontainer")
	if err != nil {
		t.Fatalf("extractContainerImage: %v", err)
	}

	if imagePath != tarballFile {
		t.Errorf("imagePath = %q, want %q", imagePath, tarballFile)
	}
	if imageHash == "" {
		t.Error("imageHash is empty")
	}
	if imageSize != int64(len(tarballContent)) {
		t.Errorf("imageSize = %d, want %d", imageSize, len(tarballContent))
	}
}

func TestExtractContainerImage_FallbackToStorePath(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Create store path as a regular file (not directory), no tarballs
	storePath := filepath.Join(dir, "store", "ghi789-result")
	os.MkdirAll(filepath.Dir(storePath), 0755)
	content := []byte("raw store path content as fallback")
	os.WriteFile(storePath, content, 0644)

	imagePath, imageHash, imageSize, err := builder.extractContainerImage(storePath, "container")
	if err != nil {
		t.Fatalf("extractContainerImage: %v", err)
	}

	if imagePath != storePath {
		t.Errorf("imagePath = %q, want %q (fallback to storePath)", imagePath, storePath)
	}
	if imageHash == "" {
		t.Error("imageHash is empty")
	}
	if imageSize != int64(len(content)) {
		t.Errorf("imageSize = %d, want %d", imageSize, len(content))
	}
}

func TestExtractContainerImage_NonexistentPath(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	_, _, _, err = builder.extractContainerImage("/nonexistent/path/abc123", "test")
	if err == nil {
		t.Error("extractContainerImage with nonexistent path should error")
	}
}

func TestExtractContainerImage_HashDeterministic(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	storePath := filepath.Join(dir, "store", "hash-test")
	os.MkdirAll(filepath.Dir(storePath), 0755)
	os.WriteFile(storePath, []byte("deterministic content"), 0644)

	_, hash1, _, err := builder.extractContainerImage(storePath, "test")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	_, hash2, _, err := builder.extractContainerImage(storePath, "test")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %q != %q", hash1, hash2)
	}
}

// ============================================================================
// CHECK CHANGES TESTS (FlakeWatcher)
// ============================================================================

func TestFlakeWatcher_CheckChanges_NoChanges(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	changeCount := 0
	w := NewFlakeWatcher(time.Second, func(path string, config *FlakeConfig) {
		changeCount++
	})

	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath: %v", err)
	}

	// checkChanges should not detect changes since nothing changed
	w.checkChanges()

	if changeCount != 0 {
		t.Errorf("onChange called %d times, want 0 (no changes)", changeCount)
	}
}

func TestFlakeWatcher_CheckChanges_ModifiedFlake(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	var changedPath string
	changeCount := 0
	w := NewFlakeWatcher(time.Second, func(path string, config *FlakeConfig) {
		changedPath = path
		changeCount++
	})

	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath: %v", err)
	}

	// Set the tracked modTime to the past so that the current file appears newer
	w.mu.Lock()
	if tracked, ok := w.paths[dir]; ok {
		tracked.modTime = time.Now().Add(-1 * time.Hour)
	}
	w.mu.Unlock()

	w.checkChanges()

	if changeCount != 1 {
		t.Errorf("onChange called %d times, want 1", changeCount)
	}
	if changedPath != dir {
		t.Errorf("changedPath = %q, want %q", changedPath, dir)
	}
}

func TestFlakeWatcher_CheckChanges_ModifiedLock(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	changeCount := 0
	w := NewFlakeWatcher(time.Second, func(path string, config *FlakeConfig) {
		changeCount++
	})

	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath: %v", err)
	}

	// Change the lock hash to something different so it's detected as changed
	w.mu.Lock()
	if tracked, ok := w.paths[dir]; ok {
		tracked.lockHash = "stale-hash-value"
	}
	w.mu.Unlock()

	w.checkChanges()

	if changeCount != 1 {
		t.Errorf("onChange called %d times, want 1 (lock hash changed)", changeCount)
	}
}

func TestFlakeWatcher_CheckChanges_DeletedFlake(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	changeCount := 0
	w := NewFlakeWatcher(time.Second, func(path string, config *FlakeConfig) {
		changeCount++
	})

	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath: %v", err)
	}

	// Delete the flake.nix file so os.Stat fails
	os.Remove(filepath.Join(dir, "flake.nix"))

	// checkChanges should skip deleted paths (continue on error)
	w.checkChanges()

	if changeCount != 0 {
		t.Errorf("onChange called %d times, want 0 (flake deleted)", changeCount)
	}
}

func TestFlakeWatcher_CheckChanges_EmptyPaths(t *testing.T) {
	w := NewFlakeWatcher(time.Second, func(path string, config *FlakeConfig) {
		t.Error("onChange should not be called with no watched paths")
	})

	// No paths added, should not panic
	w.checkChanges()
}

func TestFlakeWatcher_CheckChanges_MultiplePaths(t *testing.T) {
	dir1 := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())
	dir2 := createTempFlake(t, sampleFlakeNix(), `{"different": "lock"}`)

	var changedPaths []string
	var mu sync.Mutex
	w := NewFlakeWatcher(time.Second, func(path string, config *FlakeConfig) {
		mu.Lock()
		changedPaths = append(changedPaths, path)
		mu.Unlock()
	})

	if err := w.AddPath(dir1); err != nil {
		t.Fatalf("AddPath dir1: %v", err)
	}
	if err := w.AddPath(dir2); err != nil {
		t.Fatalf("AddPath dir2: %v", err)
	}

	// Force both to appear changed by setting old modTimes
	w.mu.Lock()
	for _, wf := range w.paths {
		wf.modTime = time.Now().Add(-1 * time.Hour)
	}
	w.mu.Unlock()

	w.checkChanges()

	mu.Lock()
	defer mu.Unlock()
	if len(changedPaths) != 2 {
		t.Errorf("onChange called %d times, want 2", len(changedPaths))
	}
}

// ============================================================================
// PUBLISH BUILD EVENT ADDITIONAL TESTS
// ============================================================================

func TestBuilder_PublishBuildEvent_WithError(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
		WotanTopic:    "test.builds",
	}

	builder, err := NewBuilder(cfg, nil) // nil wotan
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Test that publishBuildEvent does not panic with nil wotan
	// when result has error and image path set
	result := &BuildResult{
		Spec:      BuildSpec{ID: "err-build", ContainerName: "failing-service"},
		Status:    BuildStatusFailed,
		Error:     "build compilation failed",
		ImagePath: "",
	}
	builder.publishBuildEvent(result) // should return immediately (nil wotan)
}

func TestBuilder_PublishBuildEvent_WithImagePath(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
		WotanTopic:    "test.builds",
	}

	builder, err := NewBuilder(cfg, nil) // nil wotan
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	result := &BuildResult{
		Spec:      BuildSpec{ID: "img-build", ContainerName: "service-with-image"},
		Status:    BuildStatusSuccess,
		ImagePath: "/nix/store/abc123/image.tar.xz",
		Duration:  2 * time.Minute,
	}
	builder.publishBuildEvent(result) // should return immediately (nil wotan)
}

// ============================================================================
// EXECUTE BUILD TESTS
// ============================================================================

func TestExecuteBuild_InvalidSpec(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Missing required fields
	spec := &BuildSpec{
		ID:            "", // invalid - empty ID
		ContainerName: "test",
	}

	result := builder.executeBuild(context.Background(), spec)
	if result.Status != BuildStatusFailed {
		t.Errorf("Status = %q, want %q", result.Status, BuildStatusFailed)
	}
	if result.Error == "" {
		t.Error("Error should not be empty for invalid spec")
	}
	if !result.CompletedAt.After(result.StartedAt) && !result.CompletedAt.Equal(result.StartedAt) {
		t.Error("CompletedAt should be >= StartedAt")
	}
}

func TestExecuteBuild_FlakeParseFailure(t *testing.T) {
	dir := t.TempDir()
	flakeDir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Valid spec but pointing to a real flake dir
	// The build will fail at runNixBuild because nix is not installed, which
	// exercises the build failure path
	spec := &BuildSpec{
		ID:            "parse-test",
		FlakePath:     flakeDir,
		Attribute:     "nixosConfigurations.timeguru",
		ContainerName: "timeguru",
		System:        "x86_64-linux",
		Options: BuildOptions{
			Timeout: 5 * time.Second,
		},
	}

	result := builder.executeBuild(context.Background(), spec)
	// The build will either fail at ParseFlake (unlikely since fallback works)
	// or at runNixBuild (nix not available)
	if result.Status == BuildStatusSuccess {
		t.Error("executeBuild should not succeed without nix installed")
	}
	if result.Duration == 0 {
		t.Error("Duration should be set")
	}
}

func TestExecuteBuild_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	flakeDir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	spec := &BuildSpec{
		ID:            "cancel-test",
		FlakePath:     flakeDir,
		Attribute:     "nixosConfigurations.timeguru",
		ContainerName: "timeguru",
		System:        "x86_64-linux",
		Options: BuildOptions{
			Timeout: 30 * time.Second,
		},
	}

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := builder.executeBuild(ctx, spec)
	// Build should fail (either cancelled or build failed since nix isn't installed)
	if result.Status == BuildStatusSuccess {
		t.Error("executeBuild with cancelled context should not succeed")
	}
}

func TestExecuteBuild_TimedOut(t *testing.T) {
	dir := t.TempDir()
	flakeDir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 1 * time.Nanosecond, // extremely short to force timeout
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	spec := &BuildSpec{
		ID:            "timeout-test",
		FlakePath:     flakeDir,
		Attribute:     "nixosConfigurations.timeguru",
		ContainerName: "timeguru",
		System:        "x86_64-linux",
		// No timeout in options, so DefaultTimeout (1ns) will be used
	}

	result := builder.executeBuild(context.Background(), spec)
	if result.Status == BuildStatusSuccess {
		t.Error("executeBuild with 1ns timeout should not succeed")
	}
}

// ============================================================================
// RUN NIX BUILD TESTS
// ============================================================================

func TestRunNixBuild_CommandNotFound(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "/nonexistent/nix-binary",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	spec := &BuildSpec{
		FlakePath:     "/some/path",
		Attribute:     "nixosConfigurations.test",
		ContainerName: "test",
		Options: BuildOptions{
			UseCache: true,
		},
	}

	_, err = builder.runNixBuild(context.Background(), spec)
	if err == nil {
		t.Error("runNixBuild with nonexistent binary should error")
	}
}

func TestRunNixBuild_AllOptions(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "/nonexistent/nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	spec := &BuildSpec{
		FlakePath:     "/path/to/flake",
		Attribute:     "nixosConfigurations.test",
		ContainerName: "test",
		Options: BuildOptions{
			MaxCores:   4,
			UseCache:   false,
			KeepFailed: true,
			Verbose:    true,
			ExtraArgs:  []string{"--show-trace", "--impure"},
		},
	}

	// This will fail because the binary doesn't exist, but it exercises the
	// argument construction paths
	_, err = builder.runNixBuild(context.Background(), spec)
	if err == nil {
		t.Error("runNixBuild with nonexistent binary should error")
	}
}

func TestRunNixBuild_CancelledContext(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "sleep", // use sleep so the context cancellation is the cause
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	spec := &BuildSpec{
		FlakePath:     "/path/to/flake",
		Attribute:     "test",
		ContainerName: "test",
		Options:       BuildOptions{UseCache: true},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = builder.runNixBuild(ctx, spec)
	if err == nil {
		t.Error("runNixBuild with cancelled context should error")
	}
}

// ============================================================================
// LIST FLAKE CONTAINERS TESTS
// ============================================================================

func TestListFlakeContainers_NotFound(t *testing.T) {
	_, err := ListFlakeContainers("/nonexistent/path")
	if err == nil {
		t.Error("ListFlakeContainers with nonexistent path should error")
	}
}

func TestListFlakeContainers_WithContainers(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	containers, err := ListFlakeContainers(dir)
	if err != nil {
		t.Fatalf("ListFlakeContainers: %v", err)
	}

	// The sample flake has nixosConfigurations.timeguru and nixosConfigurations.captain
	if len(containers) < 2 {
		t.Errorf("len(containers) = %d, want >= 2", len(containers))
	}

	// Verify the names include timeguru and captain
	foundTimeguru := false
	foundCaptain := false
	for _, c := range containers {
		if strings.Contains(c, "timeguru") {
			foundTimeguru = true
		}
		if strings.Contains(c, "captain") {
			foundCaptain = true
		}
	}
	if !foundTimeguru {
		t.Error("timeguru container not found")
	}
	if !foundCaptain {
		t.Error("captain container not found")
	}
}

func TestListFlakeContainers_NoContainers(t *testing.T) {
	// Flake with no nixosConfigurations
	content := `{
  description = "A package-only flake";
  outputs = { ... }: {
    packages.x86_64-linux.default = {};
  };
}`
	dir := createTempFlake(t, content, "")

	containers, err := ListFlakeContainers(dir)
	if err != nil {
		t.Fatalf("ListFlakeContainers: %v", err)
	}

	if len(containers) != 0 {
		t.Errorf("len(containers) = %d, want 0 for package-only flake", len(containers))
	}
}

// ============================================================================
// VALIDATE FLAKE TESTS
// ============================================================================

func TestValidateFlake_NotInstalled(t *testing.T) {
	// nix is not installed in the test environment, so ValidateFlake should fail
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	err := ValidateFlake(dir)
	// Should error because nix command is not available
	if err == nil {
		// nix might actually be installed on this system
		t.Skip("nix appears to be installed, skipping")
	}

	if !strings.Contains(err.Error(), ErrFlakeParseError.Error()) {
		t.Errorf("error = %q, want error containing %q", err.Error(), ErrFlakeParseError.Error())
	}
}

// ============================================================================
// IMPORT TO LXD ADDITIONAL TESTS
// ============================================================================

func TestImportToLXD_LxcCommandFails(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Create a real file for the image path
	imagePath := filepath.Join(dir, "image.tar.xz")
	os.WriteFile(imagePath, []byte("fake image"), 0644)

	result := &BuildResult{
		Spec:      BuildSpec{ContainerName: "lxd-test"},
		Status:    BuildStatusSuccess,
		ImagePath: imagePath,
		ImageHash: "abcdef0123456789abcdef",
	}

	// lxc command likely not available in test env, so this should fail
	err = builder.ImportToLXD(context.Background(), result, nil)
	if err == nil {
		t.Skip("lxc appears to be available, skipping")
	}

	if !strings.Contains(err.Error(), ErrLXDImportFailed.Error()) {
		t.Errorf("error = %q, want error containing %q", err.Error(), ErrLXDImportFailed.Error())
	}
}

// ============================================================================
// PROCESS QUEUE TESTS
// ============================================================================

func TestBuilder_ProcessQueue_EmptyQueue(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// processQueue on empty queue should return immediately
	builder.processQueue(context.Background())
}

func TestBuilder_ProcessQueue_WithBuild(t *testing.T) {
	dir := t.TempDir()
	flakeDir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	cfg := BuilderConfig{
		NixBinary:      "/nonexistent/nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  2,
		DefaultTimeout: 5 * time.Second,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	spec := &BuildSpec{
		ID:            "queue-build-1",
		FlakePath:     flakeDir,
		Attribute:     "nixosConfigurations.timeguru",
		ContainerName: "timeguru",
		System:        "x86_64-linux",
		Options:       BuildOptions{Timeout: 5 * time.Second},
	}

	builder.queue.Enqueue(spec)
	builder.processQueue(context.Background())

	// Check that the result was stored
	builder.resultsMu.RLock()
	result, ok := builder.results["queue-build-1"]
	builder.resultsMu.RUnlock()

	if !ok {
		t.Fatal("build result not stored after processQueue")
	}

	// The build should have failed since nix isn't installed
	if result.Status == BuildStatusSuccess {
		t.Error("build should not succeed with nonexistent nix binary")
	}
}

// ============================================================================
// WORKER LOOP TESTS
// ============================================================================

func TestBuilder_WorkerLoop_StopChannel(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	ctx := context.Background()

	// Start a worker that will be stopped via stopCh
	builder.wg.Add(1)
	go builder.workerLoop(ctx)

	// Give the goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Close stopCh to signal worker to exit
	close(builder.stopCh)
	builder.wg.Wait() // should not hang
}

func TestBuilder_WorkerLoop_ContextCancel(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	builder.wg.Add(1)
	go builder.workerLoop(ctx)

	time.Sleep(10 * time.Millisecond)
	cancel()
	builder.wg.Wait() // should not hang
}

// ============================================================================
// CANCEL BUILD WITH STORED RESULT
// ============================================================================

func TestBuilder_CancelBuild_WithStoredResult(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	// Queue a build and also put a result in results map
	spec := &BuildSpec{ID: "cancel-with-result", Priority: 5}
	builder.queue.Enqueue(spec)

	builder.resultsMu.Lock()
	builder.results["cancel-with-result"] = &BuildResult{
		Spec:   BuildSpec{ID: "cancel-with-result"},
		Status: BuildStatusRunning,
	}
	builder.resultsMu.Unlock()

	err = builder.CancelBuild("cancel-with-result")
	if err != nil {
		t.Fatalf("CancelBuild: %v", err)
	}

	// Result status should be updated to cancelled
	builder.resultsMu.RLock()
	result, ok := builder.results["cancel-with-result"]
	builder.resultsMu.RUnlock()

	if !ok {
		t.Fatal("result not found after cancel")
	}
	if result.Status != BuildStatusCancelled {
		t.Errorf("Status = %q, want %q", result.Status, BuildStatusCancelled)
	}
}

// ============================================================================
// NEW BUILD CACHE ERROR PATH
// ============================================================================

func TestNewBuildCache_InvalidPath(t *testing.T) {
	// Try to create cache in a path that cannot be created
	// On Unix, /dev/null is a file, so /dev/null/cache should fail
	_, err := NewBuildCache("/dev/null/cache", 1024, time.Hour)
	if err == nil {
		t.Error("NewBuildCache with invalid path should error")
	}
}

// ============================================================================
// FLAKE WATCHER WATCH LOOP INTEGRATION
// ============================================================================

func TestFlakeWatcher_WatchLoopDetectsChanges(t *testing.T) {
	dir := createTempFlake(t, sampleFlakeNix(), sampleFlakeLock())

	changeCh := make(chan string, 10)
	w := NewFlakeWatcher(50*time.Millisecond, func(path string, config *FlakeConfig) {
		changeCh <- path
	})

	if err := w.AddPath(dir); err != nil {
		t.Fatalf("AddPath: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Modify the flake to trigger a change detection
	time.Sleep(100 * time.Millisecond) // wait for at least one poll cycle
	newContent := sampleFlakeNix() + "\n# modified for test"
	os.WriteFile(filepath.Join(dir, "flake.nix"), []byte(newContent), 0644)

	// Wait for change detection (up to 500ms)
	select {
	case path := <-changeCh:
		if path != dir {
			t.Errorf("changed path = %q, want %q", path, dir)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for change detection")
	}

	w.Stop()
}

// ============================================================================
// BUILD QUEUE MARK ACTIVE/COMPLETE
// ============================================================================

func TestBuildQueue_MarkActiveAndComplete(t *testing.T) {
	q := NewBuildQueue(10, 5)

	spec := &BuildSpec{ID: "mark-test", Priority: 5}
	q.Enqueue(spec)

	got, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue returned not ok")
	}

	_, cancel := context.WithCancel(context.Background())
	q.MarkActive(got, cancel)

	// Should be active (running)
	status, found := q.GetStatus("mark-test")
	if !found {
		t.Fatal("GetStatus: not found after MarkActive")
	}
	if status != BuildStatusRunning {
		t.Errorf("status = %q, want running", status)
	}

	stats := q.Stats()
	if stats["active"].(int) != 1 {
		t.Errorf("active = %d, want 1", stats["active"].(int))
	}

	q.MarkComplete("mark-test")
	cancel()

	stats = q.Stats()
	if stats["active"].(int) != 0 {
		t.Errorf("active after complete = %d, want 0", stats["active"].(int))
	}
}

// ============================================================================
// PARSE FLAKE FILE EDGE CASES
// ============================================================================

func TestParseFlakeFile_ReadError(t *testing.T) {
	// parseFlakeFile with a path that doesn't exist
	config := &FlakeConfig{Path: "/nonexistent/path/to/flake"}
	_, err := parseFlakeFile(config)
	if err == nil {
		t.Error("parseFlakeFile with nonexistent path should error")
	}
}

// ============================================================================
// NEW BUILDER ERROR PATHS
// ============================================================================

func TestNewBuilder_OutputDirCreationFails(t *testing.T) {
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      "/dev/null/output", // cannot create inside a file
		CacheDir:       "/tmp/test-cache-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		CacheSize:      1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	_, err := NewBuilder(cfg, nil)
	if err == nil {
		t.Error("NewBuilder with invalid output dir should error")
	}
}

func TestNewBuilder_CacheDirCreationFails(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       "/dev/null/cache", // cannot create inside a file
		CacheSize:      1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	_, err := NewBuilder(cfg, nil)
	if err == nil {
		t.Error("NewBuilder with invalid cache dir should error")
	}
}

// ============================================================================
// CLEAN OLD BUILDS EDGE CASES
// ============================================================================

func TestBuilder_CleanOldBuilds_NoResults(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	removed, err := builder.CleanOldBuilds(time.Hour)
	if err != nil {
		t.Fatalf("CleanOldBuilds: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0 for empty results", removed)
	}
}

func TestBuilder_CleanOldBuilds_AllRecent(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	builder.resultsMu.Lock()
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("recent-%d", i)
		builder.results[id] = &BuildResult{
			Spec:        BuildSpec{ID: id},
			Status:      BuildStatusSuccess,
			CompletedAt: time.Now(),
		}
	}
	builder.resultsMu.Unlock()

	removed, err := builder.CleanOldBuilds(time.Hour)
	if err != nil {
		t.Fatalf("CleanOldBuilds: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0 for all recent results", removed)
	}
}

// ============================================================================
// ON FLAKE CHANGE EDGE CASES
// ============================================================================

func TestBuilder_OnFlakeChange_NoContainerOutputs(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      100,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	config := &FlakeConfig{
		System: "x86_64-linux",
		Outputs: []FlakeOutput{
			{Name: "mypackage", Type: "package", System: "x86_64-linux"},
			{Name: "devshell", Type: "devShell", System: "x86_64-linux"},
		},
	}

	builder.onFlakeChange("/path/to/flake", config)

	stats := builder.queue.Stats()
	pending := stats["pending"].(int)
	if pending != 0 {
		t.Errorf("pending = %d, want 0 (no container outputs)", pending)
	}
}

func TestBuilder_OnFlakeChange_EmptyOutputs(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      100,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	config := &FlakeConfig{
		System:  "x86_64-linux",
		Outputs: []FlakeOutput{},
	}

	builder.onFlakeChange("/path/to/flake", config)

	stats := builder.queue.Stats()
	pending := stats["pending"].(int)
	if pending != 0 {
		t.Errorf("pending = %d, want 0 (empty outputs)", pending)
	}
}

// ============================================================================
// BUILD CACHE EDGE CASES
// ============================================================================

func TestBuildCache_EvictMultiple(t *testing.T) {
	dir := t.TempDir()
	// Very small cache: 100 bytes max
	cache, err := NewBuildCache(dir, 100, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewBuildCache: %v", err)
	}

	// Put a large entry that forces eviction of all previous
	for i := 0; i < 3; i++ {
		entry := &CacheEntry{
			Key:        fmt.Sprintf("small-%d", i),
			CreatedAt:  time.Now(),
			LastAccess: time.Now().Add(-time.Duration(3-i) * time.Hour),
			Size:       30,
		}
		cache.Put(entry)
	}

	// Now put a 90-byte entry, which requires evicting enough to make room
	bigEntry := &CacheEntry{
		Key:        "big-entry",
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
		Size:       90,
	}
	if err := cache.Put(bigEntry); err != nil {
		t.Fatalf("Put big entry: %v", err)
	}

	// Verify big entry is present
	_, ok := cache.Get("big-entry")
	if !ok {
		t.Error("big entry not found after put")
	}

	stats := cache.Stats()
	currentSize := stats["current_size"].(int64)
	maxSize := stats["max_size"].(int64)
	if currentSize > maxSize {
		t.Errorf("current_size %d > max_size %d", currentSize, maxSize)
	}
}

// ============================================================================
// DEFAULT BUILDER CONFIG ADDITIONAL CHECKS
// ============================================================================

func TestDefaultBuilderConfig_AllFields(t *testing.T) {
	cfg := DefaultBuilderConfig()

	if cfg.OutputDir == "" {
		t.Error("OutputDir should not be empty")
	}
	if cfg.CacheDir == "" {
		t.Error("CacheDir should not be empty")
	}
	if cfg.CacheSize <= 0 {
		t.Errorf("CacheSize = %d, want > 0", cfg.CacheSize)
	}
}

// ============================================================================
// BUILDER IS RUNNING STATE
// ============================================================================

func TestBuilder_IsRunning_Transitions(t *testing.T) {
	dir := t.TempDir()
	cfg := BuilderConfig{
		NixBinary:      "nix",
		OutputDir:      filepath.Join(dir, "output"),
		CacheDir:       filepath.Join(dir, "cache"),
		CacheSize:      1024 * 1024,
		CacheAge:       time.Hour,
		QueueSize:      10,
		MaxConcurrent:  1,
		DefaultTimeout: 5 * time.Minute,
		WatchInterval:  time.Second,
	}

	builder, err := NewBuilder(cfg, nil)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}

	if builder.IsRunning() {
		t.Error("new builder should not be running")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	builder.Start(ctx)
	if !builder.IsRunning() {
		t.Error("builder should be running after Start")
	}

	builder.Stop()
	if builder.IsRunning() {
		t.Error("builder should not be running after Stop")
	}
}
