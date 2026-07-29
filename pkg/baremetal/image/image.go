// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package image provides OS image management for bare metal provisioning.
package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config holds image management configuration.
type Config struct {
	// StoragePath is where images are stored
	StoragePath string `json:"storage_path"`

	// HTTPServeAddress is where images are served via HTTP
	HTTPServeAddress string `json:"http_serve_address"`

	// CachePath is where build cache is stored
	CachePath string `json:"cache_path"`

	// MaxCacheSize is the maximum cache size in bytes
	MaxCacheSize int64 `json:"max_cache_size"`

	// SkipDirectoryInit skips directory creation (for testing)
	SkipDirectoryInit bool `json:"skip_directory_init,omitempty"`
}

// DefaultConfig returns default image configuration.
func DefaultConfig() *Config {
	return &Config{
		StoragePath:      "/var/lib/baremetal/images",
		HTTPServeAddress: ":17000",
		CachePath:        "/var/lib/baremetal/cache",
		MaxCacheSize:     50 * 1024 * 1024 * 1024, // 50GB
	}
}

// ImageInfo holds information about an OS image.
type ImageInfo struct {
	// ID is the unique image identifier
	ID string `json:"id"`

	// Name is the human-readable name
	Name string `json:"name"`

	// Version is the image version
	Version string `json:"version"`

	// Type is the image type (nixos, ubuntu, etc.)
	Type string `json:"type"`

	// Architecture (x86_64, aarch64, etc.)
	Architecture string `json:"architecture"`

	// Paths to image components
	KernelPath   string `json:"kernel_path"`
	InitrdPath   string `json:"initrd_path"`
	RootFSPath   string `json:"rootfs_path"`
	KernelParams string `json:"kernel_params"`

	// Checksums
	KernelChecksum string `json:"kernel_checksum"`
	InitrdChecksum string `json:"initrd_checksum"`
	RootFSChecksum string `json:"rootfs_checksum"`

	// Size information
	KernelSize int64 `json:"kernel_size"`
	InitrdSize int64 `json:"initrd_size"`
	RootFSSize int64 `json:"rootfs_size"`

	// Metadata
	CreatedAt   time.Time         `json:"created_at"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Manager handles OS image management.
type Manager struct {
	config *Config

	// Image registry
	images map[string]*ImageInfo
	mu     sync.RWMutex

	// NixOS builder
	nixosBuilder *NixOSBuilder
}

// NewManager creates a new image manager.
func NewManager(config *Config) (*Manager, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Ensure directories exist (unless skipped for testing)
	if !config.SkipDirectoryInit {
		for _, dir := range []string{config.StoragePath, config.CachePath} {
			if err := os.MkdirAll(dir, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
				return nil, fmt.Errorf("creating directory %s: %w", dir, err)
			}
		}
	}

	nixosBuilder := NewNixOSBuilder(config.CachePath)

	manager := &Manager{
		config:       config,
		images:       make(map[string]*ImageInfo),
		nixosBuilder: nixosBuilder,
	}

	// Load existing images (skip if directory init was skipped)
	if !config.SkipDirectoryInit {
		if err := manager.loadImages(); err != nil {
			return nil, fmt.Errorf("loading images: %w", err)
		}
	}

	return manager, nil
}

// loadImages loads images from storage.
func (m *Manager) loadImages() error {
	entries, err := os.ReadDir(m.config.StoragePath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		imageID := entry.Name()
		info, err := m.loadImageInfo(imageID)
		if err != nil {
			continue
		}

		m.images[imageID] = info
	}

	return nil
}

// loadImageInfo loads image info from disk.
func (m *Manager) loadImageInfo(imageID string) (*ImageInfo, error) {
	imagePath := filepath.Join(m.config.StoragePath, imageID)

	info := &ImageInfo{
		ID: imageID,
	}

	// Check for kernel
	kernelPath := filepath.Join(imagePath, "kernel")
	if stat, err := os.Stat(kernelPath); err == nil {
		info.KernelPath = kernelPath
		info.KernelSize = stat.Size()
	}

	// Check for initrd
	initrdPath := filepath.Join(imagePath, "initrd")
	if stat, err := os.Stat(initrdPath); err == nil {
		info.InitrdPath = initrdPath
		info.InitrdSize = stat.Size()
	}

	// Check for rootfs
	rootfsPath := filepath.Join(imagePath, "rootfs.squashfs")
	if stat, err := os.Stat(rootfsPath); err == nil {
		info.RootFSPath = rootfsPath
		info.RootFSSize = stat.Size()
	}

	return info, nil
}

// GetImage returns an image by reference.
func (m *Manager) GetImage(ctx context.Context, ref string) (*ImageInfo, error) {
	m.mu.RLock()
	info, exists := m.images[ref]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("image not found: %s", ref)
	}

	return info, nil
}

// ListImages returns all available images.
func (m *Manager) ListImages(ctx context.Context) ([]*ImageInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ImageInfo, 0, len(m.images))
	for _, info := range m.images {
		result = append(result, info)
	}

	return result, nil
}

// ImportImage imports an image from a path.
func (m *Manager) ImportImage(ctx context.Context, id string, kernelPath, initrdPath, rootfsPath string) (*ImageInfo, error) {
	imagePath := filepath.Join(m.config.StoragePath, id)
	if err := os.MkdirAll(imagePath, 0755); err != nil { // #nosec G301 -- 0755 directory — needs traversal; files within carry their own stricter modes
		return nil, fmt.Errorf("creating image directory: %w", err)
	}

	info := &ImageInfo{
		ID:        id,
		CreatedAt: time.Now(),
	}

	// Copy kernel
	if kernelPath != "" {
		destKernel := filepath.Join(imagePath, "kernel")
		checksum, size, err := m.copyFile(kernelPath, destKernel)
		if err != nil {
			return nil, fmt.Errorf("copying kernel: %w", err)
		}
		info.KernelPath = destKernel
		info.KernelChecksum = checksum
		info.KernelSize = size
	}

	// Copy initrd
	if initrdPath != "" {
		destInitrd := filepath.Join(imagePath, "initrd")
		checksum, size, err := m.copyFile(initrdPath, destInitrd)
		if err != nil {
			return nil, fmt.Errorf("copying initrd: %w", err)
		}
		info.InitrdPath = destInitrd
		info.InitrdChecksum = checksum
		info.InitrdSize = size
	}

	// Copy rootfs
	if rootfsPath != "" {
		destRootfs := filepath.Join(imagePath, "rootfs.squashfs")
		checksum, size, err := m.copyFile(rootfsPath, destRootfs)
		if err != nil {
			return nil, fmt.Errorf("copying rootfs: %w", err)
		}
		info.RootFSPath = destRootfs
		info.RootFSChecksum = checksum
		info.RootFSSize = size
	}

	m.mu.Lock()
	m.images[id] = info
	m.mu.Unlock()

	return info, nil
}

// copyFile copies a file and returns its checksum.
func (m *Manager) copyFile(src, dst string) (string, int64, error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = dstFile.Close() }()

	hash := sha256.New()
	writer := io.MultiWriter(dstFile, hash)

	size, err := io.Copy(writer, srcFile)
	if err != nil {
		return "", 0, err
	}

	checksum := hex.EncodeToString(hash.Sum(nil))

	return checksum, size, nil
}

// DeleteImage deletes an image.
func (m *Manager) DeleteImage(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.images[id]; !exists {
		return fmt.Errorf("image not found: %s", id)
	}

	imagePath := filepath.Join(m.config.StoragePath, id)
	if err := os.RemoveAll(imagePath); err != nil {
		return fmt.Errorf("removing image directory: %w", err)
	}

	delete(m.images, id)

	return nil
}

// BuildNixOS builds a NixOS image.
func (m *Manager) BuildNixOS(ctx context.Context, flakeRef, system string) (*ImageInfo, error) {
	result, err := m.nixosBuilder.Build(ctx, flakeRef, system)
	if err != nil {
		return nil, err
	}

	// Import the built image
	id := fmt.Sprintf("nixos-%s-%d", system, time.Now().Unix())
	return m.ImportImage(ctx, id, result.KernelPath, result.InitrdPath, result.RootFSPath)
}

// VerifyImage verifies an image's checksums.
func (m *Manager) VerifyImage(ctx context.Context, id string) error {
	m.mu.RLock()
	info, exists := m.images[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("image not found: %s", id)
	}

	// Verify kernel
	if info.KernelPath != "" && info.KernelChecksum != "" {
		if err := m.verifyChecksum(info.KernelPath, info.KernelChecksum); err != nil {
			return fmt.Errorf("kernel verification failed: %w", err)
		}
	}

	// Verify initrd
	if info.InitrdPath != "" && info.InitrdChecksum != "" {
		if err := m.verifyChecksum(info.InitrdPath, info.InitrdChecksum); err != nil {
			return fmt.Errorf("initrd verification failed: %w", err)
		}
	}

	// Verify rootfs
	if info.RootFSPath != "" && info.RootFSChecksum != "" {
		if err := m.verifyChecksum(info.RootFSPath, info.RootFSChecksum); err != nil {
			return fmt.Errorf("rootfs verification failed: %w", err)
		}
	}

	return nil
}

// verifyChecksum verifies a file's checksum.
func (m *Manager) verifyChecksum(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}

	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

// GetStorageUsage returns storage usage information.
func (m *Manager) GetStorageUsage() (*StorageUsage, error) {
	usage := &StorageUsage{
		Images: make(map[string]int64),
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, info := range m.images {
		size := info.KernelSize + info.InitrdSize + info.RootFSSize
		usage.Images[id] = size
		usage.TotalBytes += size
	}

	return usage, nil
}

// StorageUsage holds storage usage information.
type StorageUsage struct {
	TotalBytes int64            `json:"total_bytes"`
	Images     map[string]int64 `json:"images"`
}
