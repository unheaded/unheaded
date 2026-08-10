// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package runtime

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ImageInfo contains information about an image.
type ImageInfo struct {
	// ID is the unique identifier (digest) of the image.
	ID string
	// RepoTags are the repository tags.
	RepoTags []string
	// RepoDigests are the repository digests.
	RepoDigests []string
	// Size is the size of the image in bytes.
	Size int64
	// Created is when the image was created.
	Created time.Time
	// Author is the author of the image.
	Author string
	// Config is the image configuration.
	Config *ImageConfig
	// RootFS describes the root filesystem.
	RootFS *RootFS
}

// ImageConfig contains image configuration.
type ImageConfig struct {
	// User is the default user.
	User string
	// ExposedPorts are the exposed ports.
	ExposedPorts map[string]struct{}
	// Env are the default environment variables.
	Env []string
	// Entrypoint is the default entrypoint.
	Entrypoint []string
	// Cmd is the default command.
	Cmd []string
	// Volumes are the default volumes.
	Volumes map[string]struct{}
	// WorkingDir is the default working directory.
	WorkingDir string
	// Labels are the image labels.
	Labels map[string]string
	// StopSignal is the default stop signal.
	StopSignal string
}

// RootFS describes the root filesystem.
type RootFS struct {
	Type    string
	DiffIDs []string
}

// ImagePullOptions contains options for pulling images.
type ImagePullOptions struct {
	// Auth is the authentication configuration.
	Auth *AuthConfig
	// Platform is the target platform.
	Platform string
	// All pulls all tags.
	All bool
}

// ImageFilter defines filters for listing images.
type ImageFilter struct {
	// Reference is the image reference to filter by.
	Reference string
	// Dangling filters to dangling images.
	Dangling bool
	// Labels filters by labels.
	Labels map[string]string
}

// ImageStore manages images.
type ImageStore struct {
	mu sync.RWMutex

	root           string
	registryConfig *RegistryConfig
	images         map[string]*ImageInfo
	layers         map[string]*layerInfo
	httpClient     *http.Client
}

// layerInfo contains information about a layer.
type layerInfo struct {
	ID       string
	DiffID   string
	Size     int64
	Path     string
	RefCount int
}

// NewImageStore creates a new image store.
func NewImageStore(root string, registryConfig *RegistryConfig) (*ImageStore, error) {
	if err := os.MkdirAll(root, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
		return nil, fmt.Errorf("failed to create image store root: %w", err)
	}

	// Create subdirectories
	for _, subdir := range []string{"layers", "manifests", "blobs", "refs"} {
		if err := os.MkdirAll(filepath.Join(root, subdir), 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
			return nil, fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	store := &ImageStore{
		root:           root,
		registryConfig: registryConfig,
		images:         make(map[string]*ImageInfo),
		layers:         make(map[string]*layerInfo),
		httpClient: &http.Client{
			Timeout: 30 * time.Minute,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	// Best-effort: image manifest may be missing on first boot.
	_ = store.loadImages()

	return store, nil
}

// loadImages loads existing images from disk.
func (s *ImageStore) loadImages() error {
	manifestsDir := filepath.Join(s.root, "manifests")
	entries, err := os.ReadDir(manifestsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			manifestPath := filepath.Join(manifestsDir, entry.Name())
			data, err := os.ReadFile(manifestPath) // #nosec G304 -- container store path derived from the runtime root
			if err != nil {
				continue
			}

			var img ImageInfo
			if err := json.Unmarshal(data, &img); err != nil {
				continue
			}

			s.images[img.ID] = &img
		}
	}

	return nil
}

// PullImage pulls an image from a registry.
func (r *DefaultRuntime) PullImage(ctx context.Context, ref string, options *ImagePullOptions) (*ImageInfo, error) {
	return r.images.PullImage(ctx, ref, options)
}

// PullImage pulls an image from a registry.
func (s *ImageStore) PullImage(ctx context.Context, ref string, options *ImagePullOptions) (*ImageInfo, error) {
	// Parse image reference
	parsed, err := parseImageReference(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference: %w", err)
	}

	// Check if already exists
	s.mu.RLock()
	for _, img := range s.images {
		for _, tag := range img.RepoTags {
			if tag == ref || tag == parsed.String() {
				s.mu.RUnlock()
				return img, nil
			}
		}
	}
	s.mu.RUnlock()

	// Get authentication
	var auth *AuthConfig
	if options != nil && options.Auth != nil {
		auth = options.Auth
	} else if s.registryConfig != nil {
		auth = s.registryConfig.AuthConfigs[parsed.Registry]
	}

	// Fetch manifest
	manifest, err := s.fetchManifest(ctx, parsed, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Fetch and store layers
	var layerDiffIDs []string
	var totalSize int64

	for _, layer := range manifest.Layers {
		layerPath, diffID, err := s.fetchLayer(ctx, parsed, layer, auth)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch layer %s: %w", layer.Digest, err)
		}

		info, err := os.Stat(layerPath)
		if err == nil {
			totalSize += info.Size()
		}

		s.mu.Lock()
		s.layers[layer.Digest] = &layerInfo{
			ID:       layer.Digest,
			DiffID:   diffID,
			Size:     layer.Size,
			Path:     layerPath,
			RefCount: 1,
		}
		s.mu.Unlock()

		layerDiffIDs = append(layerDiffIDs, diffID)
	}

	// Fetch config
	configData, err := s.fetchBlob(ctx, parsed, manifest.Config.Digest, auth)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}

	var ociConfig ociImageConfig
	if err := json.Unmarshal(configData, &ociConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Create image info
	imageID := manifest.Config.Digest
	img := &ImageInfo{
		ID:          imageID,
		RepoTags:    []string{parsed.String()},
		RepoDigests: []string{parsed.Registry + "/" + parsed.Repository + "@" + imageID},
		Size:        totalSize,
		Created:     ociConfig.Created,
		Author:      ociConfig.Author,
		Config: &ImageConfig{
			User:         ociConfig.Config.User,
			ExposedPorts: ociConfig.Config.ExposedPorts,
			Env:          ociConfig.Config.Env,
			Entrypoint:   ociConfig.Config.Entrypoint,
			Cmd:          ociConfig.Config.Cmd,
			Volumes:      ociConfig.Config.Volumes,
			WorkingDir:   ociConfig.Config.WorkingDir,
			Labels:       ociConfig.Config.Labels,
			StopSignal:   ociConfig.Config.StopSignal,
		},
		RootFS: &RootFS{
			Type:    ociConfig.RootFS.Type,
			DiffIDs: layerDiffIDs,
		},
	}

	// Save manifest
	manifestPath := filepath.Join(s.root, "manifests", sanitizeID(imageID)+".json")
	manifestData, err := json.MarshalIndent(img, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil { // #nosec G306 -- 0644 — non-sensitive artifact; secrets in this tree are written 0600
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}

	s.mu.Lock()
	s.images[imageID] = img
	s.mu.Unlock()

	return img, nil
}

// GetImage returns information about an image.
func (r *DefaultRuntime) GetImage(ctx context.Context, ref string) (*ImageInfo, error) {
	return r.images.GetImage(ref)
}

// GetImage returns information about an image.
func (s *ImageStore) GetImage(ref string) (*ImageInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try direct ID lookup
	if img, ok := s.images[ref]; ok {
		return img, nil
	}

	// Try tag lookup
	for _, img := range s.images {
		for _, tag := range img.RepoTags {
			if tag == ref {
				return img, nil
			}
		}
		for _, digest := range img.RepoDigests {
			if digest == ref {
				return img, nil
			}
		}
	}

	return nil, ErrImageNotFound
}

// ListImages lists images matching the filter.
func (r *DefaultRuntime) ListImages(ctx context.Context, filter *ImageFilter) ([]*ImageInfo, error) {
	return r.images.ListImages(filter)
}

// ListImages lists images matching the filter.
func (s *ImageStore) ListImages(filter *ImageFilter) ([]*ImageInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*ImageInfo

	for _, img := range s.images {
		if filter != nil {
			if filter.Reference != "" {
				match := false
				for _, tag := range img.RepoTags {
					if strings.Contains(tag, filter.Reference) {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}

			if len(filter.Labels) > 0 && img.Config != nil {
				match := true
				for k, v := range filter.Labels {
					if img.Config.Labels[k] != v {
						match = false
						break
					}
				}
				if !match {
					continue
				}
			}
		}

		// Return a copy
		imgCopy := *img
		result = append(result, &imgCopy)
	}

	return result, nil
}

// RemoveImage removes an image.
func (r *DefaultRuntime) RemoveImage(ctx context.Context, ref string, force bool) error {
	return r.images.RemoveImage(ref, force)
}

// RemoveImage removes an image.
func (s *ImageStore) RemoveImage(ref string, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var imageID string
	var img *ImageInfo

	// Find image by ID or tag
	if i, ok := s.images[ref]; ok {
		imageID = ref
		img = i
	} else {
		for id, i := range s.images {
			for _, tag := range i.RepoTags {
				if tag == ref {
					imageID = id
					img = i
					break
				}
			}
			if imageID != "" {
				break
			}
		}
	}

	if imageID == "" {
		return ErrImageNotFound
	}

	// Remove manifest
	manifestPath := filepath.Join(s.root, "manifests", sanitizeID(imageID)+".json")
	_ = os.Remove(manifestPath)

	// Decrement layer references and remove if unused
	if img.RootFS != nil {
		for _, diffID := range img.RootFS.DiffIDs {
			for layerID, layer := range s.layers {
				if layer.DiffID == diffID {
					layer.RefCount--
					if layer.RefCount <= 0 {
						_ = os.Remove(layer.Path)
						delete(s.layers, layerID)
					}
					break
				}
			}
		}
	}

	delete(s.images, imageID)
	return nil
}

// ExtractImage extracts an image to a directory.
func (s *ImageStore) ExtractImage(imageID, destDir string) error {
	s.mu.RLock()
	img, ok := s.images[imageID]
	s.mu.RUnlock()

	if !ok {
		return ErrImageNotFound
	}

	if img.RootFS == nil {
		return fmt.Errorf("image has no rootfs")
	}

	// Extract layers in order
	for _, diffID := range img.RootFS.DiffIDs {
		s.mu.RLock()
		var layerPath string
		for _, layer := range s.layers {
			if layer.DiffID == diffID {
				layerPath = layer.Path
				break
			}
		}
		s.mu.RUnlock()

		if layerPath == "" {
			return fmt.Errorf("layer %s not found", diffID)
		}

		if err := extractLayer(layerPath, destDir); err != nil {
			return fmt.Errorf("failed to extract layer %s: %w", diffID, err)
		}
	}

	return nil
}

// maxExtractedFileSize caps any single file extracted from a container image.
// 5 GiB is well above any realistic single-file artifact (root filesystems
// rarely have files > 1 GiB) and prevents decompression-bomb DoS where a
// crafted gzip layer expands to hundreds of GiB and OOMs the runtime.
const maxExtractedFileSize int64 = 5 << 30

// extractLayer extracts a layer tarball to a directory.
// withinDir reports whether path is contained by dir.
//
// This replaces a `strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest))`
// check, which is a well-known false-containment bug: with dest="/var/lib/store",
// the path "/var/lib/storeEVIL/x" passes HasPrefix while living entirely outside
// the store. filepath.Rel compares path *elements*, so it does not.
func withinDir(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// resolveLinkTarget resolves a tar link target the way the HOST filesystem will
// during extraction — which is the only interpretation that matters for safety.
//
// A previous version treated an absolute linkname as image-relative
// (destDir + linkname). That validated one path while os.Symlink stored a
// different one: an absolute link to /outside passed the check as
// <destDir>/outside, then a later entry writing "evil/pwn" followed the real
// symlink and landed outside destDir. Validating something other than what you
// create is not a check.
//
// So: absolute links are treated as host-absolute, relative links resolve
// against the directory holding the link. An image that ships absolute internal
// symlinks will have them skipped rather than extracted — the conservative
// direction, and the one that cannot escape.
func resolveLinkTarget(destDir, linkPath, linkname string) string {
	if filepath.IsAbs(linkname) {
		return filepath.Clean(linkname)
	}
	return filepath.Join(filepath.Dir(linkPath), linkname)
}

func extractLayer(layerPath, destDir string) error {
	f, err := os.Open(layerPath) // #nosec G304 -- container store path derived from the runtime root
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var reader io.Reader = f

	// Check for gzip compression
	gzReader, err := gzip.NewReader(f)
	if err == nil {
		reader = gzReader
		defer func() { _ = gzReader.Close() }()
	} else {
		// Seek back to beginning for uncompressed tar
		_, _ = f.Seek(0, io.SeekStart)
	}

	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Handle whiteout files (OCI layer deletion markers)
		name := header.Name
		if strings.HasPrefix(filepath.Base(name), ".wh.") {
			// This is a whiteout file - delete the corresponding file
			target := filepath.Join(destDir, filepath.Dir(name), strings.TrimPrefix(filepath.Base(name), ".wh."))
			// Guard against malicious whiteout-name path traversal (Zip Slip).
			if !withinDir(destDir, target) {
				continue
			}
			_ = os.RemoveAll(target)
			continue
		}

		targetPath := filepath.Join(destDir, name) // #nosec G305 -- extraction guarded by withinDir() plus symlink/hardlink target validation (see image.go)

		// Validate path to prevent directory traversal
		if !withinDir(destDir, targetPath) {
			continue // Skip files outside destDir
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil { // #nosec G115 -- UNFS inode field; bounded by the filesystem image size
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
				return err
			}
			// Remove any existing entry first, exactly as the TypeSymlink and
			// TypeLink cases below do.
			//
			// O_NOFOLLOW makes OpenFile return ELOOP when targetPath is already
			// a symlink — which is the normal state in a multi-layer image,
			// where a later layer replaces an earlier layer's symlink with a
			// regular file (/bin/sh, /etc/mtab, alternatives targets). Without
			// this Remove, extractLayer returns that ELOOP and ExtractImage
			// fails the whole image rather than the intended outcome of
			// refusing to FOLLOW the link while writing.
			_ = os.Remove(targetPath)
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, os.FileMode(header.Mode)) // #nosec G304,G115 -- container store path derived from the runtime root
			if err != nil {
				return err
			}
			if n, err := io.CopyN(outFile, tarReader, maxExtractedFileSize); err != nil && err != io.EOF {
				_ = outFile.Close()
				return err
			} else if n == maxExtractedFileSize {
				_ = outFile.Close()
				return fmt.Errorf("image: file %s exceeds %d-byte extraction cap (decompression-bomb guard)", targetPath, maxExtractedFileSize)
			}
			_ = outFile.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
				return err
			}
			// The link TARGET must be validated, not just the link's own path.
			// Without this, a layer can ship `evil -> /etc/cron.d` and then a
			// later entry `evil/pwn`, whose targetPath passes the lexical check
			// above while the write follows the symlink outside destDir.
			if !withinDir(destDir, resolveLinkTarget(destDir, targetPath, header.Linkname)) {
				continue // Skip symlinks escaping destDir
			}
			_ = os.Remove(targetPath) // Remove existing
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return err
			}
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
				return err
			}
			linkTarget := filepath.Join(destDir, header.Linkname) // #nosec G305 -- extraction guarded by withinDir() plus symlink/hardlink target validation (see image.go)
			// Unvalidated, `../../../etc/shadow` would hardlink a host file into
			// the container rootfs — readable by anything running in it.
			if !withinDir(destDir, linkTarget) {
				continue // Skip hardlinks escaping destDir
			}
			_ = os.Remove(targetPath)
			if err := os.Link(linkTarget, targetPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// Close closes the image store.
func (s *ImageStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.httpClient.CloseIdleConnections()
	return nil
}

// imageReference represents a parsed image reference.
type imageReference struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

func (r *imageReference) String() string {
	result := r.Repository
	if r.Registry != "" && r.Registry != "docker.io" {
		result = r.Registry + "/" + result
	}
	if r.Tag != "" {
		result = result + ":" + r.Tag
	}
	if r.Digest != "" {
		result = result + "@" + r.Digest
	}
	return result
}

// parseImageReference parses an image reference.
func parseImageReference(ref string) (*imageReference, error) {
	parsed := &imageReference{
		Registry: "docker.io",
		Tag:      "latest",
	}

	// Check for digest
	if idx := strings.Index(ref, "@"); idx != -1 {
		parsed.Digest = ref[idx+1:]
		ref = ref[:idx]
	}

	// Check for tag
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		// Make sure it's not a port number
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			parsed.Tag = afterColon
			ref = ref[:idx]
		}
	}

	// Parse registry and repository
	parts := strings.Split(ref, "/")
	if len(parts) == 1 {
		// Just image name, use library namespace
		parsed.Repository = "library/" + parts[0]
	} else if len(parts) == 2 {
		// Could be registry/image or namespace/image
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
			parsed.Registry = parts[0]
			parsed.Repository = "library/" + parts[1]
		} else {
			parsed.Repository = ref
		}
	} else {
		// registry/namespace/image or more
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
			parsed.Registry = parts[0]
			parsed.Repository = strings.Join(parts[1:], "/")
		} else {
			parsed.Repository = ref
		}
	}

	return parsed, nil
}

// OCI types for manifest and config
type ociManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType"`
	Config        ociDescriptor     `json:"config"`
	Layers        []ociDescriptor   `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

type ociDescriptor struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type ociImageConfig struct {
	Created      time.Time          `json:"created"`
	Author       string             `json:"author,omitempty"`
	Architecture string             `json:"architecture"`
	OS           string             `json:"os"`
	Config       ociContainerConfig `json:"config"`
	RootFS       ociRootFS          `json:"rootfs"`
	History      []ociHistory       `json:"history,omitempty"`
}

type ociContainerConfig struct {
	User         string              `json:"User,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	Env          []string            `json:"Env,omitempty"`
	Entrypoint   []string            `json:"Entrypoint,omitempty"`
	Cmd          []string            `json:"Cmd,omitempty"`
	Volumes      map[string]struct{} `json:"Volumes,omitempty"`
	WorkingDir   string              `json:"WorkingDir,omitempty"`
	Labels       map[string]string   `json:"Labels,omitempty"`
	StopSignal   string              `json:"StopSignal,omitempty"`
}

type ociRootFS struct {
	Type    string   `json:"type"`
	DiffIDs []string `json:"diff_ids"`
}

type ociHistory struct {
	Created    time.Time `json:"created"`
	CreatedBy  string    `json:"created_by,omitempty"`
	Comment    string    `json:"comment,omitempty"`
	EmptyLayer bool      `json:"empty_layer,omitempty"`
}

// fetchManifest fetches the image manifest from a registry.
func (s *ImageStore) fetchManifest(ctx context.Context, ref *imageReference, auth *AuthConfig) (*ociManifest, error) {
	// Get registry URL
	registryURL := s.getRegistryURL(ref.Registry)

	// Build manifest URL
	tagOrDigest := ref.Tag
	if ref.Digest != "" {
		tagOrDigest = ref.Digest
	}
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", registryURL, ref.Repository, tagOrDigest)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return nil, err
	}

	// Add accept headers for OCI and Docker manifests
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
	}, ","))

	// Add authentication
	if auth != nil {
		s.addAuth(req, auth)
	}

	// Handle Docker Hub authentication
	if ref.Registry == "docker.io" {
		token, err := s.getDockerHubToken(ctx, ref.Repository, auth)
		if err == nil && token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	// Make request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch manifest: %s - %s", resp.Status, string(body))
	}

	// Parse manifest
	var manifest ociManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// fetchLayer fetches a layer and returns its path and diff ID.
func (s *ImageStore) fetchLayer(ctx context.Context, ref *imageReference, layer ociDescriptor, auth *AuthConfig) (string, string, error) {
	layerPath := filepath.Join(s.root, "layers", sanitizeID(layer.Digest))

	// Check if layer already exists
	if _, err := os.Stat(layerPath); err == nil {
		// Calculate diff ID
		diffID, err := calculateDiffID(layerPath)
		if err != nil {
			return "", "", err
		}
		return layerPath, diffID, nil
	}

	// Fetch layer
	data, err := s.fetchBlob(ctx, ref, layer.Digest, auth)
	if err != nil {
		return "", "", err
	}

	// Write layer
	if err := os.WriteFile(layerPath, data, 0644); err != nil { // #nosec G306 -- 0644 — non-sensitive artifact; secrets in this tree are written 0600
		return "", "", err
	}

	// Calculate diff ID
	diffID, err := calculateDiffID(layerPath)
	if err != nil {
		_ = os.Remove(layerPath)
		return "", "", err
	}

	return layerPath, diffID, nil
}

// fetchBlob fetches a blob from the registry.
func (s *ImageStore) fetchBlob(ctx context.Context, ref *imageReference, digest string, auth *AuthConfig) ([]byte, error) {
	// Check if blob already exists
	blobPath := filepath.Join(s.root, "blobs", sanitizeID(digest))
	if data, err := os.ReadFile(blobPath); err == nil { // #nosec G304 -- container store path derived from the runtime root
		return data, nil
	}

	// Get registry URL
	registryURL := s.getRegistryURL(ref.Registry)

	// Build blob URL
	blobURL := fmt.Sprintf("%s/v2/%s/blobs/%s", registryURL, ref.Repository, digest)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", blobURL, nil)
	if err != nil {
		return nil, err
	}

	// Add authentication
	if auth != nil {
		s.addAuth(req, auth)
	}

	// Handle Docker Hub authentication
	if ref.Registry == "docker.io" {
		token, err := s.getDockerHubToken(ctx, ref.Repository, auth)
		if err == nil && token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	// Make request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch blob: %s - %s", resp.Status, string(body))
	}

	// Read body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Verify digest
	computedDigest := "sha256:" + hex.EncodeToString(sha256Hash(data))
	if computedDigest != digest {
		return nil, fmt.Errorf("digest mismatch: expected %s, got %s", digest, computedDigest)
	}

	// Best-effort: blob cache write; the data is already in memory.
	_ = os.WriteFile(blobPath, data, 0644) // #nosec G306 -- 0644 — non-sensitive artifact; secrets in this tree are written 0600

	return data, nil
}

// getRegistryURL returns the URL for a registry.
func (s *ImageStore) getRegistryURL(registry string) string {
	// Check for mirrors
	if s.registryConfig != nil && len(s.registryConfig.Mirrors[registry]) > 0 {
		return s.registryConfig.Mirrors[registry][0]
	}

	// Handle well-known registries
	switch registry {
	case "docker.io":
		return "https://registry-1.docker.io"
	case "gcr.io":
		return "https://gcr.io"
	case "ghcr.io":
		return "https://ghcr.io"
	case "quay.io":
		return "https://quay.io"
	default:
		// Check for insecure registries
		if s.registryConfig != nil {
			for _, insecure := range s.registryConfig.InsecureRegistries {
				if insecure == registry {
					return "http://" + registry
				}
			}
		}
		return "https://" + registry
	}
}

// addAuth adds authentication to a request.
func (s *ImageStore) addAuth(req *http.Request, auth *AuthConfig) {
	if auth.IdentityToken != "" {
		req.Header.Set("Authorization", "Bearer "+auth.IdentityToken)
	} else if auth.Auth != "" {
		req.Header.Set("Authorization", "Basic "+auth.Auth)
	} else if auth.Username != "" && auth.Password != "" {
		req.SetBasicAuth(auth.Username, auth.Password)
	}
}

// getDockerHubToken gets a token for Docker Hub.
func (s *ImageStore) getDockerHubToken(ctx context.Context, repository string, auth *AuthConfig) (string, error) {
	tokenURL := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repository)

	req, err := http.NewRequestWithContext(ctx, "GET", tokenURL, nil)
	if err != nil {
		return "", err
	}

	if auth != nil && auth.Username != "" && auth.Password != "" {
		req.SetBasicAuth(auth.Username, auth.Password)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get token: %s", resp.Status)
	}

	var tokenResp struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	if tokenResp.Token != "" {
		return tokenResp.Token, nil
	}
	return tokenResp.AccessToken, nil
}

// calculateDiffID calculates the diff ID for a layer.
func calculateDiffID(layerPath string) (string, error) {
	f, err := os.Open(layerPath) // #nosec G304 -- container store path derived from the runtime root
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	// Try to decompress gzip
	gzReader, err := gzip.NewReader(f)
	if err != nil {
		// Not gzipped, hash the file directly
		_, _ = f.Seek(0, io.SeekStart)
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
	}
	defer func() { _ = gzReader.Close() }()

	// Hash the uncompressed content (capped to defuse decompression bombs).
	h := sha256.New()
	if n, err := io.CopyN(h, gzReader, maxExtractedFileSize); err != nil && err != io.EOF {
		return "", err
	} else if n == maxExtractedFileSize {
		return "", fmt.Errorf("image: layer exceeds %d-byte hash cap (decompression-bomb guard)", maxExtractedFileSize)
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// sha256Hash computes SHA256 hash.
func sha256Hash(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// sanitizeID sanitizes an ID for use as a filename.
func sanitizeID(id string) string {
	return strings.ReplaceAll(strings.ReplaceAll(id, ":", "_"), "/", "_")
}
