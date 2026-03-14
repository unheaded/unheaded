// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// --- validateSpec tests ---

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    *ArtifactSpec
		wantErr error
	}{
		{
			name:    "nil spec",
			spec:    nil,
			wantErr: ErrArtifactInvalid,
		},
		{
			name:    "empty name",
			spec:    &ArtifactSpec{Version: "1.0", Type: ArtifactTypeBinary},
			wantErr: ErrArtifactInvalid,
		},
		{
			name:    "empty version",
			spec:    &ArtifactSpec{Name: "app", Type: ArtifactTypeBinary},
			wantErr: ErrArtifactInvalid,
		},
		{
			name:    "empty type",
			spec:    &ArtifactSpec{Name: "app", Version: "1.0"},
			wantErr: ErrArtifactInvalid,
		},
		{
			name: "valid spec",
			spec: &ArtifactSpec{Name: "app", Version: "1.0", Type: ArtifactTypeBinary},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSpec(tt.spec)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error wrapping %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("expected error wrapping %v, got %v", tt.wantErr, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- generateArtifactID tests ---

func TestGenerateArtifactID(t *testing.T) {
	id := generateArtifactID("myapp", "v1.2.3")
	if id != "myapp:v1.2.3" {
		t.Fatalf("expected myapp:v1.2.3, got %s", id)
	}
}

// --- ArtifactFilter.Match tests ---

func TestArtifactFilterMatch(t *testing.T) {
	art := &Artifact{
		Name:    "webapp",
		Version: "2.0",
		Type:    ArtifactTypeContainer,
		State:   ArtifactStateReady,
		Labels:  map[string]string{"env": "prod", "team": "infra"},
	}

	tests := []struct {
		name   string
		filter ArtifactFilter
		want   bool
	}{
		{"empty filter matches all", ArtifactFilter{}, true},
		{"name match", ArtifactFilter{Name: "webapp"}, true},
		{"name mismatch", ArtifactFilter{Name: "other"}, false},
		{"version match", ArtifactFilter{Version: "2.0"}, true},
		{"version mismatch", ArtifactFilter{Version: "1.0"}, false},
		{"type match", ArtifactFilter{Type: ArtifactTypeContainer}, true},
		{"type mismatch", ArtifactFilter{Type: ArtifactTypeBinary}, false},
		{"state match", ArtifactFilter{State: ArtifactStateReady}, true},
		{"state mismatch", ArtifactFilter{State: ArtifactStatePending}, false},
		{"labels match", ArtifactFilter{Labels: map[string]string{"env": "prod"}}, true},
		{"labels partial match", ArtifactFilter{Labels: map[string]string{"env": "prod", "team": "infra"}}, true},
		{"labels mismatch", ArtifactFilter{Labels: map[string]string{"env": "staging"}}, false},
		{"labels missing key", ArtifactFilter{Labels: map[string]string{"missing": "val"}}, false},
		{"combined match", ArtifactFilter{Name: "webapp", Type: ArtifactTypeContainer, State: ArtifactStateReady}, true},
		{"combined partial mismatch", ArtifactFilter{Name: "webapp", Version: "999"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Match(art)
			if got != tt.want {
				t.Fatalf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArtifactFilterMatchNilLabels(t *testing.T) {
	art := &Artifact{Name: "x", Labels: nil}
	f := &ArtifactFilter{Labels: map[string]string{"k": "v"}}
	if f.Match(art) {
		t.Fatal("expected no match when artifact has nil labels")
	}
}

// --- Manager tests ---

func newTestManager() *Manager {
	return NewManager(NewMemoryStorage(), NewLocalRegistry())
}

func validSpec() *ArtifactSpec {
	return &ArtifactSpec{
		Name:    "myapp",
		Version: "1.0.0",
		Type:    ArtifactTypeBinary,
	}
}

func TestManagerCreate(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	art, err := m.Create(ctx, validSpec())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if art.ID != "myapp:1.0.0" {
		t.Fatalf("unexpected ID: %s", art.ID)
	}
	if art.State != ArtifactStatePending {
		t.Fatalf("unexpected state: %s", art.State)
	}
	if art.Name != "myapp" || art.Version != "1.0.0" || art.Type != ArtifactTypeBinary {
		t.Fatal("fields not copied from spec")
	}
}

func TestManagerCreateDuplicate(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	if _, err := m.Create(ctx, validSpec()); err != nil {
		t.Fatal(err)
	}
	_, err := m.Create(ctx, validSpec())
	if !errors.Is(err, ErrArtifactExists) {
		t.Fatalf("expected ErrArtifactExists, got %v", err)
	}
}

func TestManagerCreateInvalidSpec(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	_, err := m.Create(ctx, nil)
	if !errors.Is(err, ErrArtifactInvalid) {
		t.Fatalf("expected ErrArtifactInvalid, got %v", err)
	}
}

func TestManagerCreateWithLabelsAndBuildInfo(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	spec := &ArtifactSpec{
		Name:        "app",
		Version:     "2.0",
		Type:        ArtifactTypeArchive,
		Labels:      map[string]string{"env": "prod"},
		Annotations: map[string]string{"note": "release"},
		BuildInfo: &BuildInfo{
			GitCommit: "abc123",
			GitBranch: "main",
		},
	}

	art, err := m.Create(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	if art.Labels["env"] != "prod" {
		t.Fatal("labels not set")
	}
	if art.Annotations["note"] != "release" {
		t.Fatal("annotations not set")
	}
	if art.BuildInfo == nil || art.BuildInfo.GitCommit != "abc123" {
		t.Fatal("build info not set")
	}
}

func TestManagerGet(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	art, _ := m.Create(ctx, validSpec())
	got, err := m.Get(ctx, art.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != art.ID {
		t.Fatal("ID mismatch")
	}
}

func TestManagerGetNotFound(t *testing.T) {
	m := newTestManager()
	_, err := m.Get(context.Background(), "nonexistent:1.0")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestManagerGetByNameVersion(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	m.Create(ctx, validSpec())
	got, err := m.GetByNameVersion(ctx, "myapp", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "myapp" {
		t.Fatal("wrong artifact returned")
	}
}

func TestManagerGetByNameVersionNotFound(t *testing.T) {
	m := newTestManager()
	_, err := m.GetByNameVersion(context.Background(), "nope", "0.0.0")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestManagerUploadAndDownload(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	art, _ := m.Create(ctx, validSpec())

	content := "hello artifact content"
	err := m.Upload(ctx, art.ID, strings.NewReader(content))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// Check artifact state after upload.
	got, _ := m.Get(ctx, art.ID)
	if got.State != ArtifactStateReady {
		t.Fatalf("expected state ready, got %s", got.State)
	}
	if got.Size != int64(len(content)) {
		t.Fatalf("expected size %d, got %d", len(content), got.Size)
	}

	// Verify checksum.
	h := sha256.Sum256([]byte(content))
	wantChecksum := hex.EncodeToString(h[:])
	if got.Checksum != wantChecksum {
		t.Fatalf("checksum mismatch: want %s, got %s", wantChecksum, got.Checksum)
	}

	// Download and verify content.
	rc, err := m.Download(ctx, art.ID)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()

	data, _ := io.ReadAll(rc)
	if string(data) != content {
		t.Fatalf("downloaded content mismatch")
	}
}

func TestManagerUploadNotFound(t *testing.T) {
	m := newTestManager()
	err := m.Upload(context.Background(), "nope:1.0", strings.NewReader("data"))
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestManagerUploadStorageFailure(t *testing.T) {
	failStorage := &failingStorage{putErr: errors.New("disk full")}
	m := NewManager(failStorage, nil)
	ctx := context.Background()

	art, _ := m.Create(ctx, validSpec())
	err := m.Upload(ctx, art.ID, strings.NewReader("data"))
	if err == nil {
		t.Fatal("expected error on storage failure")
	}

	got, _ := m.Get(ctx, art.ID)
	if got.State != ArtifactStateFailed {
		t.Fatalf("expected failed state, got %s", got.State)
	}
}

func TestManagerDownloadNotFound(t *testing.T) {
	m := newTestManager()
	_, err := m.Download(context.Background(), "nope:1.0")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestManagerDownloadNotReady(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	art, _ := m.Create(ctx, validSpec())
	// Artifact is in pending state, download should fail.
	_, err := m.Download(ctx, art.ID)
	if err == nil {
		t.Fatal("expected error downloading non-ready artifact")
	}
}

func TestManagerDelete(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	art, _ := m.Create(ctx, validSpec())
	_ = m.Upload(ctx, art.ID, strings.NewReader("data"))

	err := m.Delete(ctx, art.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = m.Get(ctx, art.ID)
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatal("artifact should be gone after delete")
	}
}

func TestManagerDeleteNotFound(t *testing.T) {
	m := newTestManager()
	err := m.Delete(context.Background(), "nope:1.0")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestManagerDeleteNoStorageRef(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	art, _ := m.Create(ctx, validSpec())
	// Delete without uploading (no storage ref).
	err := m.Delete(ctx, art.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestManagerDeleteWithRegistryRef(t *testing.T) {
	storage := NewMemoryStorage()
	registry := NewLocalRegistry()
	m := NewManager(storage, registry)
	ctx := context.Background()

	spec := &ArtifactSpec{Name: "img", Version: "latest", Type: ArtifactTypeContainer}
	art, _ := m.Create(ctx, spec)
	_ = m.Upload(ctx, art.ID, strings.NewReader("image-data"))

	// Push to registry so RegistryRef gets set.
	_ = m.PushToRegistry(ctx, art.ID, []string{"latest"})

	err := m.Delete(ctx, art.ID)
	if err != nil {
		t.Fatalf("Delete with registry ref: %v", err)
	}
}

func TestManagerList(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	// Empty list.
	arts, err := m.List(ctx, nil)
	if err != nil || len(arts) != 0 {
		t.Fatal("expected empty list")
	}

	// Create several artifacts.
	specs := []*ArtifactSpec{
		{Name: "a", Version: "1.0", Type: ArtifactTypeBinary},
		{Name: "b", Version: "1.0", Type: ArtifactTypeContainer},
		{Name: "c", Version: "2.0", Type: ArtifactTypeBinary},
	}
	for _, s := range specs {
		m.Create(ctx, s)
	}

	// List all (nil filter).
	arts, err = m.List(ctx, nil)
	if err != nil || len(arts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(arts))
	}

	// Filter by type.
	arts, _ = m.List(ctx, &ArtifactFilter{Type: ArtifactTypeBinary})
	if len(arts) != 2 {
		t.Fatalf("expected 2 binary artifacts, got %d", len(arts))
	}

	// Filter by name.
	arts, _ = m.List(ctx, &ArtifactFilter{Name: "b"})
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact named b, got %d", len(arts))
	}
}

func TestManagerVerify(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	art, _ := m.Create(ctx, validSpec())
	_ = m.Upload(ctx, art.ID, strings.NewReader("verify-me"))

	err := m.Verify(ctx, art.ID)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestManagerVerifyNotFound(t *testing.T) {
	m := newTestManager()
	err := m.Verify(context.Background(), "nope:1.0")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestManagerVerifyNoStorageRef(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	art, _ := m.Create(ctx, validSpec())
	err := m.Verify(ctx, art.ID)
	if err == nil {
		t.Fatal("expected error verifying artifact without storage ref")
	}
}

func TestManagerVerifyChecksumMismatch(t *testing.T) {
	storage := NewMemoryStorage()
	m := NewManager(storage, nil)
	ctx := context.Background()

	art, _ := m.Create(ctx, validSpec())
	_ = m.Upload(ctx, art.ID, strings.NewReader("original"))

	// Tamper with stored data.
	storage.mu.Lock()
	storage.data[art.ID] = []byte("tampered")
	storage.mu.Unlock()

	err := m.Verify(ctx, art.ID)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("expected ErrChecksumMismatch, got %v", err)
	}
}

func TestManagerPushToRegistry(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	spec := &ArtifactSpec{Name: "img", Version: "v1", Type: ArtifactTypeContainer}
	art, _ := m.Create(ctx, spec)
	_ = m.Upload(ctx, art.ID, strings.NewReader("container-image"))

	err := m.PushToRegistry(ctx, art.ID, []string{"v1", "latest"})
	if err != nil {
		t.Fatalf("PushToRegistry: %v", err)
	}

	got, _ := m.Get(ctx, art.ID)
	if got.RegistryRef == "" {
		t.Fatal("expected RegistryRef to be set after push")
	}
}

func TestManagerPushToRegistryNilRegistry(t *testing.T) {
	m := NewManager(NewMemoryStorage(), nil)
	ctx := context.Background()

	spec := &ArtifactSpec{Name: "img", Version: "v1", Type: ArtifactTypeContainer}
	art, _ := m.Create(ctx, spec)

	err := m.PushToRegistry(ctx, art.ID, []string{"v1"})
	if !errors.Is(err, ErrRegistryUnavailable) {
		t.Fatalf("expected ErrRegistryUnavailable, got %v", err)
	}
}

func TestManagerPushToRegistryNotFound(t *testing.T) {
	m := newTestManager()
	err := m.PushToRegistry(context.Background(), "nope:1.0", []string{"v1"})
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestManagerPushToRegistryWrongType(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	spec := &ArtifactSpec{Name: "bin", Version: "v1", Type: ArtifactTypeBinary}
	art, _ := m.Create(ctx, spec)
	_ = m.Upload(ctx, art.ID, strings.NewReader("binary"))

	err := m.PushToRegistry(ctx, art.ID, []string{"v1"})
	if err == nil {
		t.Fatal("expected error pushing non-container to registry")
	}
}

func TestManagerPullFromRegistry(t *testing.T) {
	storage := NewMemoryStorage()
	registry := NewLocalRegistry()
	m := NewManager(storage, registry)
	ctx := context.Background()

	// Push image to registry first.
	registry.Push(ctx, "myimg", []string{"v1"}, strings.NewReader("image-bytes"))

	art, err := m.PullFromRegistry(ctx, "myimg", "v1")
	if err != nil {
		t.Fatalf("PullFromRegistry: %v", err)
	}
	if art.State != ArtifactStateReady {
		t.Fatalf("expected ready state, got %s", art.State)
	}
	if art.Type != ArtifactTypeContainer {
		t.Fatalf("expected container type, got %s", art.Type)
	}
	if art.Size != int64(len("image-bytes")) {
		t.Fatalf("unexpected size: %d", art.Size)
	}
}

func TestManagerPullFromRegistryAlreadyExists(t *testing.T) {
	storage := NewMemoryStorage()
	registry := NewLocalRegistry()
	m := NewManager(storage, registry)
	ctx := context.Background()

	registry.Push(ctx, "myimg", []string{"v1"}, strings.NewReader("image-bytes"))

	// Pull once.
	art1, _ := m.PullFromRegistry(ctx, "myimg", "v1")
	// Pull again should return existing.
	art2, err := m.PullFromRegistry(ctx, "myimg", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if art1.ID != art2.ID {
		t.Fatal("expected same artifact on second pull")
	}
}

func TestManagerPullFromRegistryNilRegistry(t *testing.T) {
	m := NewManager(NewMemoryStorage(), nil)
	_, err := m.PullFromRegistry(context.Background(), "img", "v1")
	if !errors.Is(err, ErrRegistryUnavailable) {
		t.Fatalf("expected ErrRegistryUnavailable, got %v", err)
	}
}

func TestManagerPullFromRegistryNotInRegistry(t *testing.T) {
	m := newTestManager()
	_, err := m.PullFromRegistry(context.Background(), "nonexistent", "v1")
	if err == nil {
		t.Fatal("expected error pulling non-existent image")
	}
}

func TestManagerConcurrency(t *testing.T) {
	m := newTestManager()
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	// Concurrent creates with distinct names.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			spec := &ArtifactSpec{
				Name:    fmt.Sprintf("app-%d", n),
				Version: "1.0",
				Type:    ArtifactTypeBinary,
			}
			_, err := m.Create(ctx, spec)
			if err != nil {
				errs <- err
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent create error: %v", err)
	}

	arts, _ := m.List(ctx, nil)
	if len(arts) != 50 {
		t.Fatalf("expected 50 artifacts, got %d", len(arts))
	}
}

// --- failingStorage is a test double that returns errors ---

type failingStorage struct {
	putErr    error
	getErr    error
	deleteErr error
}

func (f *failingStorage) Put(_ context.Context, _ string, _ io.Reader) (string, int64, error) {
	if f.putErr != nil {
		return "", 0, f.putErr
	}
	return "ref", 0, nil
}

func (f *failingStorage) Get(_ context.Context, _ string) (io.ReadCloser, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (f *failingStorage) Delete(_ context.Context, _ string) error {
	return f.deleteErr
}

func (f *failingStorage) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}
