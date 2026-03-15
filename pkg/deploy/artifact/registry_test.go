// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package artifact

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"
	"time"
)

// --- LocalRegistry tests ---

func TestLocalRegistryPushPull(t *testing.T) {
	r := NewLocalRegistry()
	ctx := context.Background()

	ref, err := r.Push(ctx, "myimage", []string{"v1", "latest"}, strings.NewReader("image-data"))
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if ref != "myimage:v1" {
		t.Fatalf("expected ref myimage:v1, got %s", ref)
	}

	// Pull by first tag.
	rc, meta, err := r.Pull(ctx, "myimage", "v1")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "image-data" {
		t.Fatalf("expected image-data, got %s", string(data))
	}
	if meta.Reference != "myimage:v1" {
		t.Fatalf("unexpected reference in metadata: %s", meta.Reference)
	}
	if meta.Size != int64(len("image-data")) {
		t.Fatalf("unexpected size: %d", meta.Size)
	}

	// Pull by second tag.
	rc2, _, err := r.Pull(ctx, "myimage", "latest")
	if err != nil {
		t.Fatalf("Pull latest: %v", err)
	}
	data2, _ := io.ReadAll(rc2)
	rc2.Close()
	if string(data2) != "image-data" {
		t.Fatalf("latest tag content mismatch")
	}
}

func TestLocalRegistryPullNotFound(t *testing.T) {
	r := NewLocalRegistry()
	_, _, err := r.Pull(context.Background(), "noimg", "v1")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestLocalRegistryDelete(t *testing.T) {
	r := NewLocalRegistry()
	ctx := context.Background()

	r.Push(ctx, "delimg", []string{"v1"}, strings.NewReader("data"))
	err := r.Delete(ctx, "delimg:v1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, _, err = r.Pull(ctx, "delimg", "v1")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatal("expected not found after delete")
	}
}

func TestLocalRegistryDeleteNotFound(t *testing.T) {
	r := NewLocalRegistry()
	err := r.Delete(context.Background(), "noimg:v1")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound, got %v", err)
	}
}

func TestLocalRegistryTags(t *testing.T) {
	r := NewLocalRegistry()
	ctx := context.Background()

	r.Push(ctx, "tagimg", []string{"v1"}, strings.NewReader("a"))
	r.Push(ctx, "tagimg", []string{"v2"}, strings.NewReader("b"))
	r.Push(ctx, "other", []string{"v1"}, strings.NewReader("c"))

	tags, err := r.Tags(ctx, "tagimg")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	sort.Strings(tags)
	if len(tags) != 2 || tags[0] != "v1" || tags[1] != "v2" {
		t.Fatalf("unexpected tags: %v", tags)
	}

	// No tags for unknown image.
	tags, _ = r.Tags(ctx, "unknown")
	if len(tags) != 0 {
		t.Fatalf("expected 0 tags for unknown, got %d", len(tags))
	}
}

// --- NewContainerRegistry tests ---

func TestNewContainerRegistryDefaults(t *testing.T) {
	cfg := &RegistryConfig{URL: "https://registry.example.com"}
	cr := NewContainerRegistry(cfg)

	if cr.config.URL != "https://registry.example.com" {
		t.Fatal("URL not set")
	}
	if cr.httpClient.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %s", cr.httpClient.Timeout)
	}
}

func TestNewContainerRegistryCustomTimeout(t *testing.T) {
	cfg := &RegistryConfig{URL: "https://example.com", Timeout: 60 * time.Second}
	cr := NewContainerRegistry(cfg)
	if cr.httpClient.Timeout != 60*time.Second {
		t.Fatalf("expected 60s timeout, got %s", cr.httpClient.Timeout)
	}
}

// --- RegistryMirror tests ---

func TestRegistryMirrorPullCaches(t *testing.T) {
	upstream := NewLocalRegistry()
	cache := NewMemoryStorage()
	mirror := NewRegistryMirror(upstream, cache)
	ctx := context.Background()

	upstream.Push(ctx, "mirrorimg", []string{"v1"}, strings.NewReader("mirror-data"))

	// First pull: from upstream, cached.
	rc, meta, err := mirror.Pull(ctx, "mirrorimg", "v1")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "mirror-data" {
		t.Fatalf("expected mirror-data, got %s", data)
	}
	if meta == nil {
		t.Fatal("expected metadata")
	}

	// Second pull: should come from cache.
	rc2, meta2, err := mirror.Pull(ctx, "mirrorimg", "v1")
	if err != nil {
		t.Fatalf("cached Pull: %v", err)
	}
	data2, _ := io.ReadAll(rc2)
	rc2.Close()
	if string(data2) != "mirror-data" {
		t.Fatal("cached content mismatch")
	}
	if meta2.Reference != "mirrorimg:v1" {
		t.Fatalf("unexpected cached metadata ref: %s", meta2.Reference)
	}
}

func TestRegistryMirrorPush(t *testing.T) {
	upstream := NewLocalRegistry()
	cache := NewMemoryStorage()
	mirror := NewRegistryMirror(upstream, cache)
	ctx := context.Background()

	ref, err := mirror.Push(ctx, "pushimg", []string{"v1"}, strings.NewReader("push-data"))
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if ref != "pushimg:v1" {
		t.Fatalf("unexpected ref: %s", ref)
	}

	// Verify upstream has it.
	rc, _, err := upstream.Pull(ctx, "pushimg", "v1")
	if err != nil {
		t.Fatal("upstream should have the pushed image")
	}
	rc.Close()
}

func TestRegistryMirrorDelete(t *testing.T) {
	upstream := NewLocalRegistry()
	cache := NewMemoryStorage()
	mirror := NewRegistryMirror(upstream, cache)
	ctx := context.Background()

	upstream.Push(ctx, "delimg", []string{"v1"}, strings.NewReader("data"))

	// Pull to cache.
	rc, _, _ := mirror.Pull(ctx, "delimg", "v1")
	rc.Close()

	// Delete should remove from both.
	err := mirror.Delete(ctx, "delimg:v1")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Upstream should no longer have it.
	_, _, err = upstream.Pull(ctx, "delimg", "v1")
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Fatal("expected upstream image to be deleted")
	}
}

func TestRegistryMirrorTags(t *testing.T) {
	upstream := NewLocalRegistry()
	cache := NewMemoryStorage()
	mirror := NewRegistryMirror(upstream, cache)
	ctx := context.Background()

	upstream.Push(ctx, "tagimg", []string{"v1"}, strings.NewReader("a"))
	upstream.Push(ctx, "tagimg", []string{"v2"}, strings.NewReader("b"))

	tags, err := mirror.Tags(ctx, "tagimg")
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
}

func TestRegistryMirrorPullUpstreamFails(t *testing.T) {
	upstream := NewLocalRegistry()
	cache := NewMemoryStorage()
	mirror := NewRegistryMirror(upstream, cache)
	ctx := context.Background()

	_, _, err := mirror.Pull(ctx, "noimg", "v1")
	if err == nil {
		t.Fatal("expected error pulling non-existent image")
	}
}
