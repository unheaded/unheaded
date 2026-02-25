package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"unheaded/services/timeguru/internal/timeline"
)

// ============================================================================
// STORE TESTS
// ============================================================================

func createTestTimeline() *timeline.Timeline {
	return &timeline.Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Date(2026, 1, 27, 12, 0, 0, 0, time.UTC),
		Status:      "alpha",
		Vision:      "Production-ready infrastructure in hours, not months.",
		Phases: []*timeline.Phase{
			{
				ID:       "phase-1",
				Name:     "Alpha",
				Status:   "in_progress",
				Progress: 25,
			},
		},
		Milestones: []*timeline.Milestone{
			{
				ID:       "milestone-1",
				Name:     "eBPF Foundation",
				Status:   "in_progress",
				Progress: 25,
				Owner:    "Agent 5",
				Risk:     "medium",
			},
		},
	}
}

func TestNewStore_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Error("NewStore() returned nil store")
	}
}

func TestNewStore_EmptyPath(t *testing.T) {
	store, err := NewStore("")
	if err == nil {
		t.Error("NewStore() expected error for empty path, got nil")
	}
	if store != nil {
		store.Close()
		t.Error("NewStore() expected nil store for error case")
	}
}

func TestNewStore_InvalidPath(t *testing.T) {
	// Path to a directory that doesn't exist and can't be created
	invalidPath := "/nonexistent/deeply/nested/path/test.db"

	store, err := NewStore(invalidPath)
	if err == nil {
		t.Error("NewStore() expected error for invalid path, got nil")
	}
	if store != nil {
		store.Close()
		t.Error("NewStore() expected nil store for error case")
	}
}

func TestStore_SaveTimeline_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	tl := createTestTimeline()
	ctx := context.Background()

	err = store.SaveTimeline(ctx, tl)
	if err != nil {
		t.Errorf("SaveTimeline() error = %v", err)
	}
}

func TestStore_SaveTimeline_NilTimeline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	err = store.SaveTimeline(ctx, nil)
	if err == nil {
		t.Error("SaveTimeline() expected error for nil timeline, got nil")
	}
}

func TestStore_SaveTimeline_InvalidTimeline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	// Invalid timeline: empty version
	tl := &timeline.Timeline{
		Version:     "",
		LastUpdated: time.Now(),
		Status:      "alpha",
	}

	ctx := context.Background()

	err = store.SaveTimeline(ctx, tl)
	if err == nil {
		t.Error("SaveTimeline() expected error for invalid timeline, got nil")
	}
}

func TestStore_SaveTimeline_NilContext(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	tl := createTestTimeline()

	// nil context should use background context internally
	err = store.SaveTimeline(nil, tl)
	if err != nil {
		t.Errorf("SaveTimeline() should handle nil context, error = %v", err)
	}
}

func TestStore_GetTimeline_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	// Save timeline
	tl := createTestTimeline()
	ctx := context.Background()
	err = store.SaveTimeline(ctx, tl)
	if err != nil {
		t.Fatalf("SaveTimeline() error = %v", err)
	}

	// Retrieve timeline
	retrieved, err := store.GetTimeline(ctx)
	if err != nil {
		t.Fatalf("GetTimeline() error = %v", err)
	}

	if retrieved == nil {
		t.Fatal("GetTimeline() returned nil timeline")
	}

	if retrieved.Version != tl.Version {
		t.Errorf("GetTimeline() version = %q, want %q", retrieved.Version, tl.Version)
	}

	if retrieved.Status != tl.Status {
		t.Errorf("GetTimeline() status = %q, want %q", retrieved.Status, tl.Status)
	}

	if len(retrieved.Phases) != len(tl.Phases) {
		t.Errorf("GetTimeline() phases count = %d, want %d", len(retrieved.Phases), len(tl.Phases))
	}

	if len(retrieved.Milestones) != len(tl.Milestones) {
		t.Errorf("GetTimeline() milestones count = %d, want %d", len(retrieved.Milestones), len(tl.Milestones))
	}
}

func TestStore_GetTimeline_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Get timeline before any is saved
	tl, err := store.GetTimeline(ctx)
	if err == nil {
		t.Error("GetTimeline() expected error for not found, got nil")
	}
	if tl != nil {
		t.Error("GetTimeline() expected nil timeline for not found, got non-nil")
	}
}

func TestStore_UpdateMilestone_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	// Save initial timeline
	tl := createTestTimeline()
	ctx := context.Background()
	err = store.SaveTimeline(ctx, tl)
	if err != nil {
		t.Fatalf("SaveTimeline() error = %v", err)
	}

	// Update milestone
	err = store.UpdateMilestone(ctx, "milestone-1", 50, "in_progress")
	if err != nil {
		t.Errorf("UpdateMilestone() error = %v", err)
	}

	// Verify update
	updated, err := store.GetTimeline(ctx)
	if err != nil {
		t.Fatalf("GetTimeline() error = %v", err)
	}

	milestone, found := updated.GetMilestoneByID("milestone-1")
	if !found {
		t.Fatal("UpdateMilestone() milestone not found after update")
	}

	if milestone.Progress != 50 {
		t.Errorf("UpdateMilestone() progress = %d, want 50", milestone.Progress)
	}
}

func TestStore_UpdateMilestone_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	// Save initial timeline
	tl := createTestTimeline()
	ctx := context.Background()
	err = store.SaveTimeline(ctx, tl)
	if err != nil {
		t.Fatalf("SaveTimeline() error = %v", err)
	}

	// Update non-existent milestone
	err = store.UpdateMilestone(ctx, "nonexistent", 50, "in_progress")
	if err == nil {
		t.Error("UpdateMilestone() expected error for non-existent ID, got nil")
	}
}

func TestStore_UpdateMilestone_EmptyID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	err = store.UpdateMilestone(ctx, "", 50, "in_progress")
	if err == nil {
		t.Error("UpdateMilestone() expected error for empty ID, got nil")
	}
}

func TestStore_UpdateMilestone_InvalidProgress(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	// Save initial timeline
	tl := createTestTimeline()
	ctx := context.Background()
	err = store.SaveTimeline(ctx, tl)
	if err != nil {
		t.Fatalf("SaveTimeline() error = %v", err)
	}

	// Test invalid progress values
	tests := []struct {
		name     string
		progress int
	}{
		{"negative", -1},
		{"over 100", 101},
		{"way over", 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.UpdateMilestone(ctx, "milestone-1", tt.progress, "in_progress")
			if err == nil {
				t.Errorf("UpdateMilestone() expected error for progress %d, got nil", tt.progress)
			}
		})
	}
}

func TestStore_UpdateMilestone_InvalidStatus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	// Save initial timeline
	tl := createTestTimeline()
	ctx := context.Background()
	err = store.SaveTimeline(ctx, tl)
	if err != nil {
		t.Fatalf("SaveTimeline() error = %v", err)
	}

	// Test invalid status
	err = store.UpdateMilestone(ctx, "milestone-1", 50, "invalid")
	if err == nil {
		t.Error("UpdateMilestone() expected error for invalid status, got nil")
	}
}

func TestStore_Close_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	err = store.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestStore_Close_MultipleCalls(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	// First close
	err = store.Close()
	if err != nil {
		t.Errorf("Close() first call error = %v", err)
	}

	// Second close - should be safe
	err = store.Close()
	if err != nil {
		t.Errorf("Close() second call error = %v", err)
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestStore_ConcurrentReads(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	// Save timeline
	tl := createTestTimeline()
	ctx := context.Background()
	err = store.SaveTimeline(ctx, tl)
	if err != nil {
		t.Fatalf("SaveTimeline() error = %v", err)
	}

	// Concurrent reads
	const numReads = 50
	done := make(chan bool, numReads)

	for i := 0; i < numReads; i++ {
		go func() {
			_, _ = store.GetTimeline(ctx)
			done <- true
		}()
	}

	for i := 0; i < numReads; i++ {
		<-done
	}
}

func TestStore_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer store.Close()

	// Concurrent writes to different milestones
	const numWrites = 20
	done := make(chan bool, numWrites)

	// Create timeline with multiple milestones
	tl := createTestTimeline()
	for i := 2; i <= numWrites; i++ {
		tl.Milestones = append(tl.Milestones, &timeline.Milestone{
			ID:       filepath.Join("milestone-", string(rune(i))),
			Name:     filepath.Join("Test ", string(rune(i))),
			Status:   "pending",
			Progress: 0,
		})
	}

	ctx := context.Background()
	err = store.SaveTimeline(ctx, tl)
	if err != nil {
		t.Fatalf("SaveTimeline() error = %v", err)
	}

	for i := 1; i <= numWrites; i++ {
		go func(id int) {
			milestoneID := filepath.Join("milestone-", string(rune(id)))
			_ = store.UpdateMilestone(ctx, milestoneID, id*5, "in_progress")
			done <- true
		}(i)
	}

	for i := 0; i < numWrites; i++ {
		<-done
	}
}
