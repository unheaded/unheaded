// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- ArtifactTracker Tests ---

func TestNewArtifactTracker(t *testing.T) {
	t.Run("positive max history", func(t *testing.T) {
		tracker := NewArtifactTracker(50)
		if tracker == nil {
			t.Fatal("expected non-nil tracker")
		}
		if tracker.maxHistory != 50 {
			t.Errorf("expected maxHistory 50, got %d", tracker.maxHistory)
		}
	})

	t.Run("zero max history defaults to 100", func(t *testing.T) {
		tracker := NewArtifactTracker(0)
		if tracker.maxHistory != 100 {
			t.Errorf("expected maxHistory 100, got %d", tracker.maxHistory)
		}
	})

	t.Run("negative max history defaults to 100", func(t *testing.T) {
		tracker := NewArtifactTracker(-1)
		if tracker.maxHistory != 100 {
			t.Errorf("expected maxHistory 100, got %d", tracker.maxHistory)
		}
	})
}

func TestArtifactTrackerSetEventCallback(t *testing.T) {
	tracker := NewArtifactTracker(10)
	called := false
	tracker.SetEventCallback(func(event *ArtifactEvent) {
		called = true
	})
	if tracker.eventCallback == nil {
		t.Fatal("expected eventCallback to be set")
	}
	// We'll verify it fires during Register
	_ = called
}

func TestArtifactTrackerRegister(t *testing.T) {
	tests := []struct {
		name      string
		artifact  *PipelineArtifact
		wantErr   error
		wantState ArtifactState
	}{
		{
			name:    "nil artifact",
			wantErr: ErrArtifactInvalid,
		},
		{
			name:     "empty name",
			artifact: &PipelineArtifact{Version: "1.0"},
			wantErr:  ErrArtifactInvalid,
		},
		{
			name:     "empty version",
			artifact: &PipelineArtifact{Name: "svc"},
			wantErr:  ErrArtifactInvalid,
		},
		{
			name:      "valid with defaults",
			artifact:  &PipelineArtifact{Name: "svc", Version: "1.0"},
			wantState: ArtifactStatePending,
		},
		{
			name: "valid with explicit state",
			artifact: &PipelineArtifact{
				Name:    "svc2",
				Version: "1.0",
				State:   ArtifactStateReady,
			},
			wantState: ArtifactStateReady,
		},
		{
			name: "valid with explicit ID",
			artifact: &PipelineArtifact{
				ID:      "custom-id",
				Name:    "svc3",
				Version: "1.0",
			},
			wantState: ArtifactStatePending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := NewArtifactTracker(10)
			ctx := context.Background()
			err := tracker.Register(ctx, tt.artifact)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.artifact.State != tt.wantState {
				t.Errorf("expected state %s, got %s", tt.wantState, tt.artifact.State)
			}
			if tt.artifact.ID == "" {
				t.Error("expected artifact ID to be generated")
			}
			if tt.artifact.CreatedAt.IsZero() {
				t.Error("expected CreatedAt to be set")
			}
		})
	}
}

func TestArtifactTrackerRegisterDuplicate(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	a1 := &PipelineArtifact{Name: "svc", Version: "1.0"}
	if err := tracker.Register(ctx, a1); err != nil {
		t.Fatal(err)
	}

	a2 := &PipelineArtifact{Name: "svc", Version: "1.0"}
	err := tracker.Register(ctx, a2)
	if !errors.Is(err, ErrArtifactVersionExists) {
		t.Errorf("expected ErrArtifactVersionExists, got %v", err)
	}
}

func TestArtifactTrackerRegisterEventCallback(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	var received *ArtifactEvent
	var wg sync.WaitGroup
	wg.Add(1)
	tracker.SetEventCallback(func(event *ArtifactEvent) {
		received = event
		wg.Done()
	})

	a := &PipelineArtifact{Name: "svc", Version: "1.0"}
	if err := tracker.Register(ctx, a); err != nil {
		t.Fatal(err)
	}

	wg.Wait()
	if received == nil {
		t.Fatal("expected event callback to be called")
	}
	if received.Type != "artifact_registered" {
		t.Errorf("expected event type artifact_registered, got %s", received.Type)
	}
}

func TestArtifactTrackerUpdate(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	a := &PipelineArtifact{Name: "svc", Version: "1.0"}
	if err := tracker.Register(ctx, a); err != nil {
		t.Fatal(err)
	}

	t.Run("update non-existent", func(t *testing.T) {
		err := tracker.Update(ctx, "nonexistent", &ArtifactUpdate{State: ArtifactStateReady})
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("update state", func(t *testing.T) {
		err := tracker.Update(ctx, a.ID, &ArtifactUpdate{State: ArtifactStateReady})
		if err != nil {
			t.Fatal(err)
		}
		if a.State != ArtifactStateReady {
			t.Errorf("expected state ready, got %s", a.State)
		}
	})

	t.Run("update reference and checksum", func(t *testing.T) {
		err := tracker.Update(ctx, a.ID, &ArtifactUpdate{
			Reference: "docker.io/svc:1.0",
			Checksum:  "abc123",
			Size:      1024,
		})
		if err != nil {
			t.Fatal(err)
		}
		if a.Reference != "docker.io/svc:1.0" {
			t.Errorf("expected reference docker.io/svc:1.0, got %s", a.Reference)
		}
		if a.Checksum != "abc123" {
			t.Errorf("expected checksum abc123, got %s", a.Checksum)
		}
		if a.Size != 1024 {
			t.Errorf("expected size 1024, got %d", a.Size)
		}
	})

	t.Run("update build info", func(t *testing.T) {
		build := &ArtifactBuild{BuildID: "build-1", GitCommit: "abc"}
		err := tracker.Update(ctx, a.ID, &ArtifactUpdate{Build: build})
		if err != nil {
			t.Fatal(err)
		}
		if a.Build == nil || a.Build.BuildID != "build-1" {
			t.Error("expected build info to be updated")
		}
	})

	t.Run("update labels", func(t *testing.T) {
		err := tracker.Update(ctx, a.ID, &ArtifactUpdate{
			Labels: map[string]string{"env": "prod"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if a.Labels["env"] != "prod" {
			t.Error("expected label env=prod")
		}
	})

	t.Run("update annotations", func(t *testing.T) {
		err := tracker.Update(ctx, a.ID, &ArtifactUpdate{
			Annotations: map[string]string{"note": "test"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if a.Annotations["note"] != "test" {
			t.Error("expected annotation note=test")
		}
	})

	t.Run("merge annotations on second update", func(t *testing.T) {
		err := tracker.Update(ctx, a.ID, &ArtifactUpdate{
			Annotations: map[string]string{"note2": "test2"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if a.Annotations["note"] != "test" || a.Annotations["note2"] != "test2" {
			t.Error("expected annotations to be merged")
		}
	})
}

func TestArtifactTrackerGet(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	a := &PipelineArtifact{Name: "svc", Version: "1.0"}
	_ = tracker.Register(ctx, a)

	t.Run("existing", func(t *testing.T) {
		got, err := tracker.Get(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "svc" {
			t.Errorf("expected name svc, got %s", got.Name)
		}
	})

	t.Run("non-existent", func(t *testing.T) {
		_, err := tracker.Get(ctx, "nonexistent")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})
}

func TestArtifactTrackerGetByVersion(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	a := &PipelineArtifact{Name: "svc", Version: "1.0"}
	_ = tracker.Register(ctx, a)

	t.Run("existing", func(t *testing.T) {
		got, err := tracker.GetByVersion(ctx, "svc", "1.0")
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != a.ID {
			t.Errorf("expected ID %s, got %s", a.ID, got.ID)
		}
	})

	t.Run("non-existent", func(t *testing.T) {
		_, err := tracker.GetByVersion(ctx, "svc", "2.0")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})
}

func TestArtifactTrackerGetVersions(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	// Register three versions with small time gaps
	for _, v := range []string{"1.0", "2.0", "3.0"} {
		a := &PipelineArtifact{Name: "svc", Version: v}
		_ = tracker.Register(ctx, a)
		time.Sleep(time.Millisecond) // ensure ordering
	}

	t.Run("all versions", func(t *testing.T) {
		versions, err := tracker.GetVersions(ctx, "svc", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(versions) != 3 {
			t.Fatalf("expected 3 versions, got %d", len(versions))
		}
		// Newest first
		if versions[0].Version != "3.0" {
			t.Errorf("expected newest first, got %s", versions[0].Version)
		}
	})

	t.Run("limited versions", func(t *testing.T) {
		versions, err := tracker.GetVersions(ctx, "svc", 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(versions) != 2 {
			t.Fatalf("expected 2 versions, got %d", len(versions))
		}
	})

	t.Run("non-existent name", func(t *testing.T) {
		versions, err := tracker.GetVersions(ctx, "unknown", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(versions) != 0 {
			t.Errorf("expected empty list, got %d", len(versions))
		}
	})
}

func TestArtifactTrackerGetLatest(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("no versions", func(t *testing.T) {
		_, err := tracker.GetLatest(ctx, "unknown")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("with versions", func(t *testing.T) {
		_ = tracker.Register(ctx, &PipelineArtifact{Name: "svc", Version: "1.0"})
		time.Sleep(time.Millisecond)
		_ = tracker.Register(ctx, &PipelineArtifact{Name: "svc", Version: "2.0"})

		latest, err := tracker.GetLatest(ctx, "svc")
		if err != nil {
			t.Fatal(err)
		}
		if latest.Version != "2.0" {
			t.Errorf("expected version 2.0, got %s", latest.Version)
		}
	})
}

func TestArtifactTrackerGetDeployed(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("no deployed", func(t *testing.T) {
		_, err := tracker.GetDeployed(ctx, "svc")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})
}

func TestArtifactTrackerMarkDeploying(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("non-existent artifact", func(t *testing.T) {
		err := tracker.MarkDeploying(ctx, "nonexistent", &ArtifactDeployment{DeploymentID: "d1"})
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("artifact not ready", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc", Version: "1.0"}
		_ = tracker.Register(ctx, a)

		err := tracker.MarkDeploying(ctx, a.ID, &ArtifactDeployment{DeploymentID: "d1"})
		if !errors.Is(err, ErrArtifactNotReady) {
			t.Errorf("expected ErrArtifactNotReady, got %v", err)
		}
	})

	t.Run("successful mark deploying", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc2", Version: "1.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a)

		deployment := &ArtifactDeployment{
			DeploymentID: "d1",
			PipelineID:   "p1",
			Environment:  "prod",
			Strategy:     "rolling",
		}
		err := tracker.MarkDeploying(ctx, a.ID, deployment)
		if err != nil {
			t.Fatal(err)
		}

		if a.State != ArtifactStateDeploying {
			t.Errorf("expected state deploying, got %s", a.State)
		}
		if a.Deployment == nil {
			t.Fatal("expected deployment to be set")
		}
	})

	t.Run("sets previous version", func(t *testing.T) {
		tracker2 := NewArtifactTracker(10)

		// Deploy v1 first
		a1 := &PipelineArtifact{Name: "svc", Version: "1.0", State: ArtifactStateReady}
		_ = tracker2.Register(ctx, a1)
		_ = tracker2.MarkDeploying(ctx, a1.ID, &ArtifactDeployment{DeploymentID: "d1"})
		_ = tracker2.MarkDeployed(ctx, a1.ID, true)

		// Deploy v2
		a2 := &PipelineArtifact{Name: "svc", Version: "2.0", State: ArtifactStateReady}
		_ = tracker2.Register(ctx, a2)
		dep := &ArtifactDeployment{DeploymentID: "d2"}
		_ = tracker2.MarkDeploying(ctx, a2.ID, dep)

		if dep.PreviousVersion != "1.0" {
			t.Errorf("expected previous version 1.0, got %s", dep.PreviousVersion)
		}
	})
}

func TestArtifactTrackerMarkDeployed(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("non-existent", func(t *testing.T) {
		err := tracker.MarkDeployed(ctx, "nonexistent", true)
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("successful deployment", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc", Version: "1.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a)
		_ = tracker.MarkDeploying(ctx, a.ID, &ArtifactDeployment{DeploymentID: "d1"})

		err := tracker.MarkDeployed(ctx, a.ID, true)
		if err != nil {
			t.Fatal(err)
		}

		if a.State != ArtifactStateDeployed {
			t.Errorf("expected state deployed, got %s", a.State)
		}
		if a.DeployedAt.IsZero() {
			t.Error("expected DeployedAt to be set")
		}
		if a.Deployment != nil && !a.Deployment.Healthy {
			t.Error("expected deployment to be healthy")
		}

		// Verify deployed map
		deployed, err := tracker.GetDeployed(ctx, "svc")
		if err != nil {
			t.Fatal(err)
		}
		if deployed.ID != a.ID {
			t.Errorf("expected deployed artifact to match")
		}
	})

	t.Run("updates deployment record in history", func(t *testing.T) {
		tracker2 := NewArtifactTracker(10)
		a := &PipelineArtifact{Name: "svc", Version: "1.0", State: ArtifactStateReady}
		_ = tracker2.Register(ctx, a)
		_ = tracker2.MarkDeploying(ctx, a.ID, &ArtifactDeployment{DeploymentID: "d1"})
		_ = tracker2.MarkDeployed(ctx, a.ID, true)

		history, _ := tracker2.GetHistory(ctx, "svc", 10)
		if len(history) == 0 {
			t.Fatal("expected history entries")
		}
		if history[0].Status != "deployed" {
			t.Errorf("expected status deployed, got %s", history[0].Status)
		}
	})
}

func TestArtifactTrackerMarkFailed(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("non-existent", func(t *testing.T) {
		err := tracker.MarkFailed(ctx, "nonexistent", errors.New("fail"))
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("successful fail marking", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc", Version: "1.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a)
		_ = tracker.MarkDeploying(ctx, a.ID, &ArtifactDeployment{DeploymentID: "d1"})

		err := tracker.MarkFailed(ctx, a.ID, errors.New("health check failed"))
		if err != nil {
			t.Fatal(err)
		}

		if a.State != ArtifactStateFailed {
			t.Errorf("expected state failed, got %s", a.State)
		}

		// Verify history updated
		history, _ := tracker.GetHistory(ctx, "svc", 10)
		found := false
		for _, h := range history {
			if h.Status == "failed" && h.Error == "health check failed" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected failed record in history")
		}
	})
}

func TestArtifactTrackerMarkRollback(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("non-existent version", func(t *testing.T) {
		err := tracker.MarkRollback(ctx, "svc", "1.0", "d1")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("successful rollback mark", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc", Version: "1.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a)

		err := tracker.MarkRollback(ctx, "svc", "1.0", "d2")
		if err != nil {
			t.Fatal(err)
		}

		history, _ := tracker.GetHistory(ctx, "svc", 10)
		found := false
		for _, h := range history {
			if h.Status == "rollback" && h.RollbackTo == "1.0" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected rollback record in history")
		}
	})
}

func TestArtifactTrackerGetHistory(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("empty history", func(t *testing.T) {
		history, err := tracker.GetHistory(ctx, "unknown", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 0 {
			t.Errorf("expected empty history, got %d", len(history))
		}
	})

	t.Run("returns newest first", func(t *testing.T) {
		a1 := &PipelineArtifact{Name: "svc", Version: "1.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a1)
		_ = tracker.MarkDeploying(ctx, a1.ID, &ArtifactDeployment{DeploymentID: "d1"})
		_ = tracker.MarkDeployed(ctx, a1.ID, true)

		a2 := &PipelineArtifact{Name: "svc", Version: "2.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a2)
		_ = tracker.MarkDeploying(ctx, a2.ID, &ArtifactDeployment{DeploymentID: "d2"})

		history, err := tracker.GetHistory(ctx, "svc", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) < 2 {
			t.Fatalf("expected at least 2 history entries, got %d", len(history))
		}
		// Newest first - last appended should be first returned
		if history[0].DeploymentID != "d2" {
			t.Errorf("expected newest first (d2), got %s", history[0].DeploymentID)
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		history, err := tracker.GetHistory(ctx, "svc", 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 1 {
			t.Errorf("expected 1 entry, got %d", len(history))
		}
	})
}

func TestArtifactTrackerGetPreviousDeployed(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("no previous", func(t *testing.T) {
		_, err := tracker.GetPreviousDeployed(ctx, "unknown")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("with previous deployment", func(t *testing.T) {
		a1 := &PipelineArtifact{Name: "svc", Version: "1.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a1)
		_ = tracker.MarkDeploying(ctx, a1.ID, &ArtifactDeployment{DeploymentID: "d1"})
		_ = tracker.MarkDeployed(ctx, a1.ID, true)

		a2 := &PipelineArtifact{Name: "svc", Version: "2.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a2)
		_ = tracker.MarkDeploying(ctx, a2.ID, &ArtifactDeployment{DeploymentID: "d2"})
		_ = tracker.MarkDeployed(ctx, a2.ID, true)

		prev, err := tracker.GetPreviousDeployed(ctx, "svc")
		if err != nil {
			t.Fatal(err)
		}
		if prev.Version != "1.0" {
			t.Errorf("expected previous version 1.0, got %s", prev.Version)
		}
	})
}

func TestArtifactTrackerList(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	_ = tracker.Register(ctx, &PipelineArtifact{Name: "svc", Version: "1.0", Type: ArtifactTypeContainer})
	time.Sleep(time.Millisecond)
	_ = tracker.Register(ctx, &PipelineArtifact{Name: "svc2", Version: "1.0", Type: ArtifactTypeBinary})

	t.Run("no filter", func(t *testing.T) {
		list, err := tracker.List(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 {
			t.Errorf("expected 2, got %d", len(list))
		}
	})

	t.Run("filter by name", func(t *testing.T) {
		list, err := tracker.List(ctx, &ArtifactFilter{Name: "svc"})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1, got %d", len(list))
		}
	})

	t.Run("filter by type", func(t *testing.T) {
		list, err := tracker.List(ctx, &ArtifactFilter{Type: ArtifactTypeContainer})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1, got %d", len(list))
		}
	})

	t.Run("filter by state", func(t *testing.T) {
		list, err := tracker.List(ctx, &ArtifactFilter{State: ArtifactStatePending})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 {
			t.Errorf("expected 2, got %d", len(list))
		}
	})
}

func TestArtifactFilterMatch(t *testing.T) {
	now := time.Now()
	artifact := &PipelineArtifact{
		Name:      "svc",
		Version:   "1.0",
		Type:      ArtifactTypeContainer,
		State:     ArtifactStatePending,
		CreatedAt: now,
		Labels:    map[string]string{"env": "prod", "team": "platform"},
	}

	tests := []struct {
		name   string
		filter *ArtifactFilter
		want   bool
	}{
		{"match all empty filter", &ArtifactFilter{}, true},
		{"match name", &ArtifactFilter{Name: "svc"}, true},
		{"no match name", &ArtifactFilter{Name: "other"}, false},
		{"match type", &ArtifactFilter{Type: ArtifactTypeContainer}, true},
		{"no match type", &ArtifactFilter{Type: ArtifactTypeBinary}, false},
		{"match state", &ArtifactFilter{State: ArtifactStatePending}, true},
		{"no match state", &ArtifactFilter{State: ArtifactStateReady}, false},
		{"match created after", &ArtifactFilter{CreatedAfter: now.Add(-time.Hour)}, true},
		{"no match created after", &ArtifactFilter{CreatedAfter: now.Add(time.Hour)}, false},
		{"match created before", &ArtifactFilter{CreatedBefore: now.Add(time.Hour)}, true},
		{"no match created before", &ArtifactFilter{CreatedBefore: now.Add(-time.Hour)}, false},
		{"match labels", &ArtifactFilter{Labels: map[string]string{"env": "prod"}}, true},
		{"no match labels", &ArtifactFilter{Labels: map[string]string{"env": "staging"}}, false},
		{"match partial labels", &ArtifactFilter{Labels: map[string]string{"team": "platform"}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Match(artifact)
			if got != tt.want {
				t.Errorf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestArtifactTrackerVerify(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	content := []byte("hello world")
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	t.Run("non-existent artifact", func(t *testing.T) {
		err := tracker.Verify(ctx, "nonexistent", bytes.NewReader(content))
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("no checksum set", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc", Version: "1.0"}
		_ = tracker.Register(ctx, a)

		err := tracker.Verify(ctx, a.ID, bytes.NewReader(content))
		if err != nil {
			t.Errorf("expected nil error for no checksum, got %v", err)
		}
	})

	t.Run("checksum match", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc2", Version: "1.0", Checksum: checksum}
		_ = tracker.Register(ctx, a)

		err := tracker.Verify(ctx, a.ID, bytes.NewReader(content))
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc3", Version: "1.0", Checksum: "badchecksum"}
		_ = tracker.Register(ctx, a)

		err := tracker.Verify(ctx, a.ID, bytes.NewReader(content))
		if !errors.Is(err, ErrChecksumMismatch) {
			t.Errorf("expected ErrChecksumMismatch, got %v", err)
		}
	})
}

func TestArtifactTrackerArchive(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("non-existent", func(t *testing.T) {
		err := tracker.Archive(ctx, "nonexistent")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("successful archive", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc", Version: "1.0"}
		_ = tracker.Register(ctx, a)

		err := tracker.Archive(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		if a.State != ArtifactStateArchived {
			t.Errorf("expected state archived, got %s", a.State)
		}
	})
}

func TestArtifactTrackerDelete(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	t.Run("non-existent", func(t *testing.T) {
		err := tracker.Delete(ctx, "nonexistent")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Errorf("expected ErrArtifactNotFound, got %v", err)
		}
	})

	t.Run("cannot delete deployed", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc-del", Version: "1.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a)
		_ = tracker.MarkDeploying(ctx, a.ID, &ArtifactDeployment{DeploymentID: "d1"})
		_ = tracker.MarkDeployed(ctx, a.ID, true)

		err := tracker.Delete(ctx, a.ID)
		if err == nil {
			t.Error("expected error deleting deployed artifact")
		}
		if !strings.Contains(err.Error(), "currently deployed") {
			t.Errorf("expected 'currently deployed' error, got %v", err)
		}
	})

	t.Run("successful delete", func(t *testing.T) {
		a := &PipelineArtifact{Name: "svc-del2", Version: "1.0"}
		_ = tracker.Register(ctx, a)
		id := a.ID

		err := tracker.Delete(ctx, id)
		if err != nil {
			t.Fatal(err)
		}

		_, err = tracker.Get(ctx, id)
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Error("expected artifact to be deleted")
		}

		_, err = tracker.GetByVersion(ctx, "svc-del2", "1.0")
		if !errors.Is(err, ErrArtifactNotFound) {
			t.Error("expected version index to be cleaned up")
		}
	})
}

func TestArtifactTrackerCleanup(t *testing.T) {
	ctx := context.Background()

	t.Run("nil policy", func(t *testing.T) {
		tracker := NewArtifactTracker(10)
		deleted, err := tracker.Cleanup(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 0 {
			t.Errorf("expected 0 deleted, got %d", deleted)
		}
	})

	t.Run("delete by age", func(t *testing.T) {
		tracker := NewArtifactTracker(10)
		a := &PipelineArtifact{Name: "old-svc", Version: "1.0"}
		_ = tracker.Register(ctx, a)
		// Manually set old creation time
		tracker.mu.Lock()
		a.CreatedAt = time.Now().Add(-48 * time.Hour)
		tracker.mu.Unlock()

		deleted, err := tracker.Cleanup(ctx, &RetentionPolicy{MaxAge: 24 * time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 1 {
			t.Errorf("expected 1 deleted, got %d", deleted)
		}
	})

	t.Run("delete archived", func(t *testing.T) {
		tracker := NewArtifactTracker(10)
		a := &PipelineArtifact{Name: "arch-svc", Version: "1.0"}
		_ = tracker.Register(ctx, a)
		_ = tracker.Archive(ctx, a.ID)

		deleted, err := tracker.Cleanup(ctx, &RetentionPolicy{DeleteArchived: true})
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 1 {
			t.Errorf("expected 1 deleted, got %d", deleted)
		}
	})

	t.Run("delete failed", func(t *testing.T) {
		tracker := NewArtifactTracker(10)
		a := &PipelineArtifact{Name: "fail-svc", Version: "1.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a)
		_ = tracker.MarkDeploying(ctx, a.ID, &ArtifactDeployment{DeploymentID: "d1"})
		_ = tracker.MarkFailed(ctx, a.ID, errors.New("fail"))

		deleted, err := tracker.Cleanup(ctx, &RetentionPolicy{DeleteFailed: true})
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 1 {
			t.Errorf("expected 1 deleted, got %d", deleted)
		}
	})

	t.Run("skip deployed artifacts", func(t *testing.T) {
		tracker := NewArtifactTracker(10)
		a := &PipelineArtifact{Name: "dep-svc", Version: "1.0", State: ArtifactStateReady}
		_ = tracker.Register(ctx, a)
		// Manually set old creation time
		tracker.mu.Lock()
		a.CreatedAt = time.Now().Add(-48 * time.Hour)
		tracker.mu.Unlock()
		_ = tracker.MarkDeploying(ctx, a.ID, &ArtifactDeployment{DeploymentID: "d1"})
		_ = tracker.MarkDeployed(ctx, a.ID, true)

		deleted, err := tracker.Cleanup(ctx, &RetentionPolicy{MaxAge: 24 * time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 0 {
			t.Errorf("expected 0 deleted (deployed skipped), got %d", deleted)
		}
	})

	t.Run("max versions cleanup", func(t *testing.T) {
		tracker := NewArtifactTracker(10)
		for i := 0; i < 5; i++ {
			a := &PipelineArtifact{
				Name:    "ver-svc",
				Version: fmt.Sprintf("%d.0", i),
			}
			_ = tracker.Register(ctx, a)
			time.Sleep(time.Millisecond)
		}

		deleted, err := tracker.Cleanup(ctx, &RetentionPolicy{MaxVersions: 2})
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 3 {
			t.Errorf("expected 3 deleted, got %d", deleted)
		}
	})
}

func TestArtifactTrackerGetStats(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	_ = tracker.Register(ctx, &PipelineArtifact{
		Name: "svc", Version: "1.0", Type: ArtifactTypeContainer, Size: 100,
	})
	_ = tracker.Register(ctx, &PipelineArtifact{
		Name: "svc2", Version: "1.0", Type: ArtifactTypeBinary, Size: 200,
	})

	stats := tracker.GetStats(ctx)
	if stats.Total != 2 {
		t.Errorf("expected total 2, got %d", stats.Total)
	}
	if stats.TotalSize != 300 {
		t.Errorf("expected total size 300, got %d", stats.TotalSize)
	}
	if stats.ByState[ArtifactStatePending] != 2 {
		t.Errorf("expected 2 pending, got %d", stats.ByState[ArtifactStatePending])
	}
	if stats.ByType[ArtifactTypeContainer] != 1 {
		t.Errorf("expected 1 container, got %d", stats.ByType[ArtifactTypeContainer])
	}
	if stats.ByService["svc"] != 1 {
		t.Errorf("expected 1 for svc, got %d", stats.ByService["svc"])
	}
}

func TestArtifactTrackerExport(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	a := &PipelineArtifact{Name: "svc", Version: "1.0", State: ArtifactStateReady}
	_ = tracker.Register(ctx, a)
	_ = tracker.MarkDeploying(ctx, a.ID, &ArtifactDeployment{DeploymentID: "d1"})
	_ = tracker.MarkDeployed(ctx, a.ID, true)

	data, err := tracker.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var export struct {
		Artifacts []json.RawMessage          `json:"artifacts"`
		Deployed  map[string]string          `json:"deployed"`
		History   map[string]json.RawMessage `json:"history"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}

	if len(export.Artifacts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(export.Artifacts))
	}
	if export.Deployed["svc"] != a.ID {
		t.Errorf("expected deployed svc=%s, got %s", a.ID, export.Deployed["svc"])
	}
}

func TestArtifactTrackerConcurrentAccess(t *testing.T) {
	tracker := NewArtifactTracker(100)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := &PipelineArtifact{
				Name:    fmt.Sprintf("svc-%d", i),
				Version: "1.0",
			}
			_ = tracker.Register(ctx, a)
			_, _ = tracker.Get(ctx, a.ID)
			_, _ = tracker.GetVersions(ctx, a.Name, 0)
			_, _ = tracker.List(ctx, nil)
			_ = tracker.GetStats(ctx)
		}(i)
	}
	wg.Wait()
}

// --- ArtifactBuilder Tests ---

func TestNewArtifactBuilder(t *testing.T) {
	b := NewArtifactBuilder("svc", "1.0")
	if b == nil {
		t.Fatal("expected non-nil builder")
	}
	a := b.Build()
	if a.Name != "svc" {
		t.Errorf("expected name svc, got %s", a.Name)
	}
	if a.Version != "1.0" {
		t.Errorf("expected version 1.0, got %s", a.Version)
	}
	if a.State != ArtifactStatePending {
		t.Errorf("expected state pending, got %s", a.State)
	}
	if a.Labels == nil {
		t.Error("expected labels to be initialized")
	}
}

func TestArtifactBuilderChaining(t *testing.T) {
	build := &ArtifactBuild{BuildID: "b1", GitCommit: "abc"}
	a := NewArtifactBuilder("svc", "1.0").
		WithType(ArtifactTypeContainer).
		WithReference("docker.io/svc:1.0").
		WithChecksum("abc123").
		WithSize(1024).
		WithBuild(build).
		WithLabel("env", "prod").
		WithAnnotation("note", "test").
		Ready().
		Build()

	if a.Type != ArtifactTypeContainer {
		t.Errorf("expected type container, got %s", a.Type)
	}
	if a.Reference != "docker.io/svc:1.0" {
		t.Errorf("expected reference docker.io/svc:1.0, got %s", a.Reference)
	}
	if a.Checksum != "abc123" {
		t.Errorf("expected checksum abc123, got %s", a.Checksum)
	}
	if a.Size != 1024 {
		t.Errorf("expected size 1024, got %d", a.Size)
	}
	if a.Build == nil || a.Build.BuildID != "b1" {
		t.Error("expected build info")
	}
	if a.Labels["env"] != "prod" {
		t.Error("expected label env=prod")
	}
	if a.Annotations["note"] != "test" {
		t.Error("expected annotation note=test")
	}
	if a.State != ArtifactStateReady {
		t.Errorf("expected state ready, got %s", a.State)
	}
}

// --- Constants Tests ---

func TestArtifactTypeConstants(t *testing.T) {
	types := []ArtifactType{
		ArtifactTypeContainer,
		ArtifactTypeBinary,
		ArtifactTypePackage,
		ArtifactTypeConfig,
		ArtifactTypeNix,
		ArtifactTypeHelm,
	}
	for _, at := range types {
		if at == "" {
			t.Error("artifact type constant should not be empty")
		}
	}
}

func TestArtifactStateConstants(t *testing.T) {
	states := []ArtifactState{
		ArtifactStatePending,
		ArtifactStateBuilding,
		ArtifactStateUploading,
		ArtifactStateReady,
		ArtifactStateDeploying,
		ArtifactStateDeployed,
		ArtifactStateFailed,
		ArtifactStateArchived,
	}
	for _, s := range states {
		if s == "" {
			t.Error("artifact state constant should not be empty")
		}
	}
}

func TestArtifactErrorConstants(t *testing.T) {
	errs := []error{
		ErrArtifactNotFound,
		ErrArtifactVersionExists,
		ErrArtifactInvalid,
		ErrArtifactNotReady,
		ErrChecksumMismatch,
	}
	for _, e := range errs {
		if e == nil {
			t.Error("error constant should not be nil")
		}
	}
}

func TestGenerateArtifactID(t *testing.T) {
	id1 := generateArtifactID("svc", "1.0")
	id2 := generateArtifactID("svc", "1.0")
	if id1 == "" {
		t.Error("expected non-empty ID")
	}
	if !strings.HasPrefix(id1, "svc-1.0-") {
		t.Errorf("expected ID to start with svc-1.0-, got %s", id1)
	}
	// IDs should be unique due to nanosecond timestamps
	// but we don't guarantee it in tests since they may run fast
	_ = id2
}

func TestArtifactTrackerHistoryTrimming(t *testing.T) {
	tracker := NewArtifactTracker(3) // small maxHistory
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		a := &PipelineArtifact{
			Name:    "svc",
			Version: fmt.Sprintf("%d.0", i),
			State:   ArtifactStateReady,
		}
		_ = tracker.Register(ctx, a)
		_ = tracker.MarkDeploying(ctx, a.ID, &ArtifactDeployment{
			DeploymentID: fmt.Sprintf("d%d", i),
		})
	}

	history, _ := tracker.GetHistory(ctx, "svc", 100)
	if len(history) > 3 {
		t.Errorf("expected max 3 history entries, got %d", len(history))
	}
}

func TestArtifactTrackerMarkDeployedNilDeployment(t *testing.T) {
	tracker := NewArtifactTracker(10)
	ctx := context.Background()

	a := &PipelineArtifact{Name: "svc", Version: "1.0"}
	_ = tracker.Register(ctx, a)

	// Directly set state to deploying without deployment info
	tracker.mu.Lock()
	a.State = ArtifactStateDeploying
	tracker.mu.Unlock()

	err := tracker.MarkDeployed(ctx, a.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.State != ArtifactStateDeployed {
		t.Errorf("expected deployed, got %s", a.State)
	}
}
