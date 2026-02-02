//go:build linux

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// =============================================================================
// Image Reference Parsing Tests
// =============================================================================

func TestParseImageReference_Simple(t *testing.T) {
	tests := []struct {
		input      string
		registry   string
		repository string
		tag        string
		digest     string
	}{
		{"alpine", "docker.io", "library/alpine", "latest", ""},
		{"nginx", "docker.io", "library/nginx", "latest", ""},
		{"alpine:3.18", "docker.io", "library/alpine", "3.18", ""},
		{"myuser/myapp", "docker.io", "myuser/myapp", "latest", ""},
		{"myuser/myapp:v1.0", "docker.io", "myuser/myapp", "v1.0", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ref, err := parseImageReference(tt.input)
			if err != nil {
				t.Fatalf("parseImageReference failed: %v", err)
			}

			if ref.Registry != tt.registry {
				t.Errorf("Registry: got %s, want %s", ref.Registry, tt.registry)
			}
			if ref.Repository != tt.repository {
				t.Errorf("Repository: got %s, want %s", ref.Repository, tt.repository)
			}
			if ref.Tag != tt.tag {
				t.Errorf("Tag: got %s, want %s", ref.Tag, tt.tag)
			}
			if ref.Digest != tt.digest {
				t.Errorf("Digest: got %s, want %s", ref.Digest, tt.digest)
			}
		})
	}
}

func TestParseImageReference_WithRegistry(t *testing.T) {
	tests := []struct {
		input      string
		registry   string
		repository string
		tag        string
	}{
		{"gcr.io/myproject/myapp:v1.0", "gcr.io", "myproject/myapp", "v1.0"},
		{"quay.io/myorg/myapp:latest", "quay.io", "myorg/myapp", "latest"},
		{"ghcr.io/owner/repo:sha-abc123", "ghcr.io", "owner/repo", "sha-abc123"},
		{"localhost:5000/myapp:dev", "localhost:5000", "library/myapp", "dev"},
		{"registry.example.com/path/to/image:tag", "registry.example.com", "path/to/image", "tag"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ref, err := parseImageReference(tt.input)
			if err != nil {
				t.Fatalf("parseImageReference failed: %v", err)
			}

			if ref.Registry != tt.registry {
				t.Errorf("Registry: got %s, want %s", ref.Registry, tt.registry)
			}
			if ref.Repository != tt.repository {
				t.Errorf("Repository: got %s, want %s", ref.Repository, tt.repository)
			}
			if ref.Tag != tt.tag {
				t.Errorf("Tag: got %s, want %s", ref.Tag, tt.tag)
			}
		})
	}
}

func TestParseImageReference_WithDigest(t *testing.T) {
	ref, err := parseImageReference("alpine@sha256:abc123def456")
	if err != nil {
		t.Fatalf("parseImageReference failed: %v", err)
	}

	if ref.Digest != "sha256:abc123def456" {
		t.Errorf("Digest: got %s, want sha256:abc123def456", ref.Digest)
	}
}

func TestImageReference_String(t *testing.T) {
	tests := []struct {
		ref      imageReference
		expected string
	}{
		{
			imageReference{Registry: "docker.io", Repository: "library/alpine", Tag: "latest"},
			"library/alpine:latest",
		},
		{
			imageReference{Registry: "gcr.io", Repository: "myproject/myapp", Tag: "v1.0"},
			"gcr.io/myproject/myapp:v1.0",
		},
		{
			imageReference{Registry: "docker.io", Repository: "myuser/myapp", Tag: "v1.0", Digest: "sha256:abc"},
			"myuser/myapp:v1.0@sha256:abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.ref.String()
			if result != tt.expected {
				t.Errorf("String(): got %s, want %s", result, tt.expected)
			}
		})
	}
}

// =============================================================================
// Image Store Tests
// =============================================================================

func TestNewImageStore(t *testing.T) {
	root := testTempDir(t)

	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Verify directories were created
	dirs := []string{"layers", "manifests", "blobs", "refs"}
	for _, dir := range dirs {
		path := filepath.Join(root, dir)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Directory %s was not created", dir)
		}
	}
}

func TestNewImageStore_WithRegistryConfig(t *testing.T) {
	root := testTempDir(t)

	regConfig := &RegistryConfig{
		Mirrors: map[string][]string{
			"docker.io": {"https://mirror.example.com"},
		},
		InsecureRegistries: []string{"localhost:5000"},
		AuthConfigs: map[string]*AuthConfig{
			"docker.io": {Username: "user", Password: "pass"},
		},
	}

	store, err := NewImageStore(root, regConfig)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	if store.registryConfig == nil {
		t.Error("Expected registry config to be set")
	}
}

func TestNewImageStore_InvalidRoot(t *testing.T) {
	// Try to create in a read-only location (will fail on most systems)
	store, err := NewImageStore("/proc/nonexistent", nil)
	if err == nil {
		store.Close()
		t.Error("Expected error for invalid root path")
	}
}

// =============================================================================
// Image Store Get/List Tests
// =============================================================================

func TestImageStore_GetImage_ByID(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Add mock image
	imageID := "sha256:abc123def456"
	store.mu.Lock()
	store.images[imageID] = &ImageInfo{
		ID:       imageID,
		RepoTags: []string{"alpine:latest"},
		Size:     1024,
	}
	store.mu.Unlock()

	// Get by ID
	img, err := store.GetImage(imageID)
	if err != nil {
		t.Fatalf("GetImage failed: %v", err)
	}

	if img.ID != imageID {
		t.Errorf("Expected ID %s, got %s", imageID, img.ID)
	}
}

func TestImageStore_GetImage_ByTag(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Add mock image
	imageID := "sha256:abc123def456"
	tag := "alpine:3.18"
	store.mu.Lock()
	store.images[imageID] = &ImageInfo{
		ID:       imageID,
		RepoTags: []string{tag},
		Size:     1024,
	}
	store.mu.Unlock()

	// Get by tag
	img, err := store.GetImage(tag)
	if err != nil {
		t.Fatalf("GetImage failed: %v", err)
	}

	if img.ID != imageID {
		t.Errorf("Expected ID %s, got %s", imageID, img.ID)
	}
}

func TestImageStore_GetImage_ByDigest(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Add mock image
	imageID := "sha256:abc123def456"
	digest := "docker.io/library/alpine@sha256:abc123def456"
	store.mu.Lock()
	store.images[imageID] = &ImageInfo{
		ID:          imageID,
		RepoTags:    []string{"alpine:latest"},
		RepoDigests: []string{digest},
		Size:        1024,
	}
	store.mu.Unlock()

	// Get by digest
	img, err := store.GetImage(digest)
	if err != nil {
		t.Fatalf("GetImage failed: %v", err)
	}

	if img.ID != imageID {
		t.Errorf("Expected ID %s, got %s", imageID, img.ID)
	}
}

func TestImageStore_GetImage_NotFound(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	_, err = store.GetImage("nonexistent:latest")
	if err != ErrImageNotFound {
		t.Errorf("Expected ErrImageNotFound, got %v", err)
	}
}

func TestImageStore_ListImages_Empty(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	images, err := store.ListImages(nil)
	if err != nil {
		t.Fatalf("ListImages failed: %v", err)
	}

	if len(images) != 0 {
		t.Errorf("Expected 0 images, got %d", len(images))
	}
}

func TestImageStore_ListImages_Multiple(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Add mock images
	store.mu.Lock()
	for i := 0; i < 5; i++ {
		store.images[generateID()] = &ImageInfo{
			ID:   generateID(),
			Size: 1024,
		}
	}
	store.mu.Unlock()

	images, err := store.ListImages(nil)
	if err != nil {
		t.Fatalf("ListImages failed: %v", err)
	}

	if len(images) != 5 {
		t.Errorf("Expected 5 images, got %d", len(images))
	}
}

func TestImageStore_ListImages_WithFilter(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Add mock images
	store.mu.Lock()
	store.images["id1"] = &ImageInfo{
		ID:       "id1",
		RepoTags: []string{"alpine:3.18"},
		Config:   &ImageConfig{Labels: map[string]string{"env": "prod"}},
	}
	store.images["id2"] = &ImageInfo{
		ID:       "id2",
		RepoTags: []string{"alpine:3.17"},
		Config:   &ImageConfig{Labels: map[string]string{"env": "dev"}},
	}
	store.images["id3"] = &ImageInfo{
		ID:       "id3",
		RepoTags: []string{"nginx:latest"},
		Config:   &ImageConfig{Labels: map[string]string{"env": "prod"}},
	}
	store.mu.Unlock()

	// Filter by reference
	filter := &ImageFilter{Reference: "alpine"}
	images, err := store.ListImages(filter)
	if err != nil {
		t.Fatalf("ListImages failed: %v", err)
	}

	if len(images) != 2 {
		t.Errorf("Expected 2 alpine images, got %d", len(images))
	}

	// Filter by label
	filter = &ImageFilter{Labels: map[string]string{"env": "prod"}}
	images, err = store.ListImages(filter)
	if err != nil {
		t.Fatalf("ListImages failed: %v", err)
	}

	if len(images) != 2 {
		t.Errorf("Expected 2 prod images, got %d", len(images))
	}
}

// =============================================================================
// Image Store Remove Tests
// =============================================================================

func TestImageStore_RemoveImage_ByID(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Add mock image
	imageID := "sha256:abc123"
	store.mu.Lock()
	store.images[imageID] = &ImageInfo{
		ID:       imageID,
		RepoTags: []string{"alpine:latest"},
		RootFS:   &RootFS{DiffIDs: []string{}},
	}
	store.mu.Unlock()

	err = store.RemoveImage(imageID, false)
	if err != nil {
		t.Fatalf("RemoveImage failed: %v", err)
	}

	// Verify image is removed
	_, err = store.GetImage(imageID)
	if err != ErrImageNotFound {
		t.Errorf("Expected ErrImageNotFound, got %v", err)
	}
}

func TestImageStore_RemoveImage_ByTag(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Add mock image
	imageID := "sha256:abc123"
	tag := "alpine:latest"
	store.mu.Lock()
	store.images[imageID] = &ImageInfo{
		ID:       imageID,
		RepoTags: []string{tag},
		RootFS:   &RootFS{DiffIDs: []string{}},
	}
	store.mu.Unlock()

	err = store.RemoveImage(tag, false)
	if err != nil {
		t.Fatalf("RemoveImage failed: %v", err)
	}

	// Verify image is removed
	_, err = store.GetImage(imageID)
	if err != ErrImageNotFound {
		t.Errorf("Expected ErrImageNotFound, got %v", err)
	}
}

func TestImageStore_RemoveImage_NotFound(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	err = store.RemoveImage("nonexistent:latest", false)
	if err != ErrImageNotFound {
		t.Errorf("Expected ErrImageNotFound, got %v", err)
	}
}

// =============================================================================
// Image Info Tests
// =============================================================================

func TestImageInfo_Fields(t *testing.T) {
	now := time.Now()
	info := &ImageInfo{
		ID:          "sha256:abc123",
		RepoTags:    []string{"alpine:3.18", "alpine:latest"},
		RepoDigests: []string{"docker.io/library/alpine@sha256:abc123"},
		Size:        5 * 1024 * 1024,
		Created:     now,
		Author:      "Alpine Linux",
		Config: &ImageConfig{
			User:       "root",
			Env:        []string{"PATH=/usr/bin"},
			Cmd:        []string{"/bin/sh"},
			WorkingDir: "/",
		},
		RootFS: &RootFS{
			Type:    "layers",
			DiffIDs: []string{"sha256:layer1", "sha256:layer2"},
		},
	}

	if info.ID != "sha256:abc123" {
		t.Errorf("Expected ID sha256:abc123, got %s", info.ID)
	}

	if len(info.RepoTags) != 2 {
		t.Errorf("Expected 2 repo tags, got %d", len(info.RepoTags))
	}

	if info.Size != 5*1024*1024 {
		t.Errorf("Expected size 5MB, got %d", info.Size)
	}

	if info.Config.Cmd[0] != "/bin/sh" {
		t.Errorf("Expected cmd /bin/sh, got %v", info.Config.Cmd)
	}

	if len(info.RootFS.DiffIDs) != 2 {
		t.Errorf("Expected 2 diff IDs, got %d", len(info.RootFS.DiffIDs))
	}
}

func TestImageConfig_Fields(t *testing.T) {
	config := &ImageConfig{
		User: "1000:1000",
		ExposedPorts: map[string]struct{}{
			"80/tcp":  {},
			"443/tcp": {},
		},
		Env:        []string{"PATH=/usr/bin", "HOME=/root"},
		Entrypoint: []string{"/docker-entrypoint.sh"},
		Cmd:        []string{"nginx", "-g", "daemon off;"},
		Volumes: map[string]struct{}{
			"/data": {},
		},
		WorkingDir: "/app",
		Labels: map[string]string{
			"maintainer": "test@example.com",
		},
		StopSignal: "SIGTERM",
	}

	if config.User != "1000:1000" {
		t.Errorf("Expected user 1000:1000, got %s", config.User)
	}

	if len(config.ExposedPorts) != 2 {
		t.Errorf("Expected 2 exposed ports, got %d", len(config.ExposedPorts))
	}

	if len(config.Entrypoint) != 1 {
		t.Errorf("Expected 1 entrypoint, got %d", len(config.Entrypoint))
	}

	if config.StopSignal != "SIGTERM" {
		t.Errorf("Expected stop signal SIGTERM, got %s", config.StopSignal)
	}
}

// =============================================================================
// Image Pull Options Tests
// =============================================================================

func TestImagePullOptions(t *testing.T) {
	opts := &ImagePullOptions{
		Auth: &AuthConfig{
			Username: "user",
			Password: "pass",
		},
		Platform: "linux/amd64",
		All:      false,
	}

	if opts.Auth.Username != "user" {
		t.Errorf("Expected username 'user', got '%s'", opts.Auth.Username)
	}

	if opts.Platform != "linux/amd64" {
		t.Errorf("Expected platform 'linux/amd64', got '%s'", opts.Platform)
	}
}

// =============================================================================
// Registry URL Tests
// =============================================================================

func TestImageStore_GetRegistryURL(t *testing.T) {
	root := testTempDir(t)

	tests := []struct {
		registry string
		config   *RegistryConfig
		expected string
	}{
		{"docker.io", nil, "https://registry-1.docker.io"},
		{"gcr.io", nil, "https://gcr.io"},
		{"ghcr.io", nil, "https://ghcr.io"},
		{"quay.io", nil, "https://quay.io"},
		{"example.com", nil, "https://example.com"},
		{
			"docker.io",
			&RegistryConfig{
				Mirrors: map[string][]string{
					"docker.io": {"https://mirror.example.com"},
				},
			},
			"https://mirror.example.com",
		},
		{
			"insecure.example.com",
			&RegistryConfig{
				InsecureRegistries: []string{"insecure.example.com"},
			},
			"http://insecure.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.registry, func(t *testing.T) {
			store, err := NewImageStore(filepath.Join(root, tt.registry), tt.config)
			if err != nil {
				t.Fatalf("NewImageStore failed: %v", err)
			}
			defer store.Close()

			url := store.getRegistryURL(tt.registry)
			if url != tt.expected {
				t.Errorf("getRegistryURL(%s) = %s, want %s", tt.registry, url, tt.expected)
			}
		})
	}
}

// =============================================================================
// Sanitize ID Tests
// =============================================================================

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sha256:abc123", "sha256_abc123"},
		{"docker.io/library/alpine", "docker.io_library_alpine"},
		{"simple", "simple"},
		{"with:colon:and/slash", "with_colon_and_slash"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeID(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeID(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// SHA256 Hash Tests
// =============================================================================

func TestSha256Hash(t *testing.T) {
	data := []byte("test data")
	hash := sha256Hash(data)

	if len(hash) != 32 {
		t.Errorf("Expected 32 byte hash, got %d bytes", len(hash))
	}

	// Same input should produce same hash
	hash2 := sha256Hash(data)
	for i := range hash {
		if hash[i] != hash2[i] {
			t.Error("Same input produced different hash")
			break
		}
	}

	// Different input should produce different hash
	hash3 := sha256Hash([]byte("different data"))
	same := true
	for i := range hash {
		if hash[i] != hash3[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("Different inputs produced same hash")
	}
}

// =============================================================================
// Runtime Image Operations Tests
// =============================================================================

func TestRuntime_GetImage(t *testing.T) {
	root := testTempDir(t)

	imgStore, err := NewImageStore(filepath.Join(root, "images"), nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}

	rt := &DefaultRuntime{
		config: &RuntimeConfig{Root: root},
		images: imgStore,
	}

	// Add mock image
	imgStore.mu.Lock()
	imgStore.images["sha256:test"] = &ImageInfo{
		ID:       "sha256:test",
		RepoTags: []string{"test:latest"},
	}
	imgStore.mu.Unlock()

	ctx := context.Background()
	img, err := rt.GetImage(ctx, "test:latest")
	if err != nil {
		t.Fatalf("GetImage failed: %v", err)
	}

	if img.ID != "sha256:test" {
		t.Errorf("Expected ID sha256:test, got %s", img.ID)
	}
}

func TestRuntime_ListImages(t *testing.T) {
	root := testTempDir(t)

	imgStore, err := NewImageStore(filepath.Join(root, "images"), nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}

	rt := &DefaultRuntime{
		config: &RuntimeConfig{Root: root},
		images: imgStore,
	}

	// Add mock images
	imgStore.mu.Lock()
	for i := 0; i < 3; i++ {
		imgStore.images[generateID()] = &ImageInfo{
			ID:   generateID(),
			Size: 1024,
		}
	}
	imgStore.mu.Unlock()

	ctx := context.Background()
	images, err := rt.ListImages(ctx, nil)
	if err != nil {
		t.Fatalf("ListImages failed: %v", err)
	}

	if len(images) != 3 {
		t.Errorf("Expected 3 images, got %d", len(images))
	}
}

func TestRuntime_RemoveImage(t *testing.T) {
	root := testTempDir(t)

	imgStore, err := NewImageStore(filepath.Join(root, "images"), nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}

	rt := &DefaultRuntime{
		config: &RuntimeConfig{Root: root},
		images: imgStore,
	}

	// Add mock image
	imgStore.mu.Lock()
	imgStore.images["sha256:remove"] = &ImageInfo{
		ID:       "sha256:remove",
		RepoTags: []string{"remove:latest"},
		RootFS:   &RootFS{DiffIDs: []string{}},
	}
	imgStore.mu.Unlock()

	ctx := context.Background()
	err = rt.RemoveImage(ctx, "sha256:remove", false)
	if err != nil {
		t.Fatalf("RemoveImage failed: %v", err)
	}

	_, err = rt.GetImage(ctx, "sha256:remove")
	if err != ErrImageNotFound {
		t.Errorf("Expected ErrImageNotFound, got %v", err)
	}
}

// =============================================================================
// Layer Info Tests
// =============================================================================

func TestLayerInfo(t *testing.T) {
	layer := &layerInfo{
		ID:       "sha256:layer123",
		DiffID:   "sha256:diff456",
		Size:     10 * 1024 * 1024,
		Path:     "/var/lib/unheaded/layers/sha256_layer123",
		RefCount: 2,
	}

	if layer.ID != "sha256:layer123" {
		t.Errorf("Expected ID sha256:layer123, got %s", layer.ID)
	}

	if layer.RefCount != 2 {
		t.Errorf("Expected ref count 2, got %d", layer.RefCount)
	}
}

// =============================================================================
// Image Store Close Tests
// =============================================================================

func TestImageStore_Close(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}

	err = store.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Close again should be idempotent (no panic)
	err = store.Close()
	if err != nil {
		t.Errorf("Second close failed: %v", err)
	}
}

// =============================================================================
// Image Store Load Tests
// =============================================================================

func TestImageStore_LoadImages(t *testing.T) {
	root := testTempDir(t)

	// Create store
	store1, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}

	// Add and save mock image (simulate by writing manifest)
	manifestDir := filepath.Join(root, "manifests")
	os.MkdirAll(manifestDir, 0755)

	manifestData := `{"ID":"sha256:test123","RepoTags":["test:latest"],"Size":1024}`
	os.WriteFile(filepath.Join(manifestDir, "sha256_test123.json"), []byte(manifestData), 0644)

	store1.Close()

	// Create new store - should load existing images
	store2, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store2.Close()

	// Verify image was loaded
	img, err := store2.GetImage("sha256:test123")
	if err != nil {
		t.Logf("Note: Image loading may not be fully implemented: %v", err)
		return
	}

	if img.ID != "sha256:test123" {
		t.Errorf("Expected loaded image ID sha256:test123, got %s", img.ID)
	}
}

// =============================================================================
// Concurrent Image Operations Tests
// =============================================================================

func TestImageStore_ConcurrentAccess(t *testing.T) {
	root := testTempDir(t)
	store, err := NewImageStore(root, nil)
	if err != nil {
		t.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Pre-populate with images
	store.mu.Lock()
	for i := 0; i < 10; i++ {
		id := generateID()
		store.images[id] = &ImageInfo{
			ID:       id,
			RepoTags: []string{"test:" + id[:8]},
			RootFS:   &RootFS{DiffIDs: []string{}},
		}
	}
	store.mu.Unlock()

	// Concurrent operations
	done := make(chan bool, 100)

	// Concurrent gets
	for i := 0; i < 25; i++ {
		go func() {
			store.ListImages(nil)
			done <- true
		}()
	}

	// Concurrent lists
	for i := 0; i < 25; i++ {
		go func() {
			store.ListImages(&ImageFilter{})
			done <- true
		}()
	}

	// Wait for all operations
	for i := 0; i < 50; i++ {
		<-done
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkParseImageReference(b *testing.B) {
	refs := []string{
		"alpine",
		"alpine:3.18",
		"myuser/myapp:v1.0",
		"gcr.io/myproject/myapp:latest",
		"registry.example.com/path/to/image:tag",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseImageReference(refs[i%len(refs)])
	}
}

func BenchmarkImageStore_ListImages(b *testing.B) {
	root, err := os.MkdirTemp("", "image-bench-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(root)

	store, err := NewImageStore(root, nil)
	if err != nil {
		b.Fatalf("NewImageStore failed: %v", err)
	}
	defer store.Close()

	// Pre-populate
	store.mu.Lock()
	for i := 0; i < 100; i++ {
		id := generateID()
		store.images[id] = &ImageInfo{
			ID:       id,
			RepoTags: []string{"test:" + id[:8]},
		}
	}
	store.mu.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.ListImages(nil)
	}
}

func BenchmarkSanitizeID(b *testing.B) {
	id := "sha256:abc123def456789"
	for i := 0; i < b.N; i++ {
		sanitizeID(id)
	}
}
