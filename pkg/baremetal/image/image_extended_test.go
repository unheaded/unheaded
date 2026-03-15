// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package image

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// ImportImage — error paths for initrd and rootfs copying
// ---------------------------------------------------------------------------

func TestImportImage_BadInitrdSource(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	// Valid kernel, bad initrd
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "kernel"), []byte("k"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := mgr.ImportImage(ctx, "bad-initrd",
		filepath.Join(srcDir, "kernel"),
		"/nonexistent/initrd",
		"",
	)
	if err == nil {
		t.Fatal("expected error for bad initrd source")
	}
	if !strings.Contains(err.Error(), "copying initrd") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestImportImage_BadRootfsSource(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "kernel"), []byte("k"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "initrd"), []byte("i"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := mgr.ImportImage(ctx, "bad-rootfs",
		filepath.Join(srcDir, "kernel"),
		filepath.Join(srcDir, "initrd"),
		"/nonexistent/rootfs.squashfs",
	)
	if err == nil {
		t.Fatal("expected error for bad rootfs source")
	}
	if !strings.Contains(err.Error(), "copying rootfs") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestImportImage_BadDestination(t *testing.T) {
	// Make storage path read-only so MkdirAll fails inside ImportImage.
	storageDir := t.TempDir()
	cacheDir := t.TempDir()
	mgr, err := NewManager(&Config{
		StoragePath: storageDir,
		CachePath:   cacheDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Remove write permission on storage dir so image subdir creation fails.
	if err := os.Chmod(storageDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(storageDir, 0755) })

	_, err = mgr.ImportImage(context.Background(), "no-write", "", "", "")
	if err == nil {
		t.Fatal("expected error when storage dir is read-only")
	}
	if !strings.Contains(err.Error(), "creating image directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// copyFile — destination creation error
// ---------------------------------------------------------------------------

func TestCopyFile_BadDst(t *testing.T) {
	mgr := testManager(t)
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "src")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Destination in a non-existent directory.
	_, _, err := mgr.copyFile(src, "/nonexistent/dir/dst")
	if err == nil {
		t.Fatal("expected error for bad destination path")
	}
}

func TestCopyFile_BadSrc(t *testing.T) {
	mgr := testManager(t)
	_, _, err := mgr.copyFile("/nonexistent/src", "/tmp/dst")
	if err == nil {
		t.Fatal("expected error for bad source path")
	}
}

// ---------------------------------------------------------------------------
// VerifyImage — initrd and rootfs corruption, and missing file
// ---------------------------------------------------------------------------

func TestVerifyImage_CorruptedInitrd(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	for _, name := range []string{"kernel", "initrd"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(name+"-data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	info, err := mgr.ImportImage(ctx, "corrupt-initrd",
		filepath.Join(srcDir, "kernel"),
		filepath.Join(srcDir, "initrd"),
		"",
	)
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the initrd file on disk.
	if err := os.WriteFile(info.InitrdPath, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}

	err = mgr.VerifyImage(ctx, "corrupt-initrd")
	if err == nil {
		t.Fatal("expected initrd verification failure")
	}
	if !strings.Contains(err.Error(), "initrd verification failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyImage_CorruptedRootfs(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	for _, name := range []string{"kernel", "initrd", "rootfs.squashfs"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(name+"-data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	info, err := mgr.ImportImage(ctx, "corrupt-rootfs",
		filepath.Join(srcDir, "kernel"),
		filepath.Join(srcDir, "initrd"),
		filepath.Join(srcDir, "rootfs.squashfs"),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt rootfs only.
	if err := os.WriteFile(info.RootFSPath, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}

	err = mgr.VerifyImage(ctx, "corrupt-rootfs")
	if err == nil {
		t.Fatal("expected rootfs verification failure")
	}
	if !strings.Contains(err.Error(), "rootfs verification failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyImage_MissingFile(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "kernel"), []byte("k"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := mgr.ImportImage(ctx, "missing-kernel", filepath.Join(srcDir, "kernel"), "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Delete the copied kernel so verifyChecksum gets an os.Open error.
	os.Remove(info.KernelPath)

	err = mgr.VerifyImage(ctx, "missing-kernel")
	if err == nil {
		t.Fatal("expected error for missing kernel file")
	}
	if !strings.Contains(err.Error(), "kernel verification failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// verifyChecksum — direct unit tests
// ---------------------------------------------------------------------------

func TestVerifyChecksum_Match(t *testing.T) {
	mgr := testManager(t)

	data := []byte("hello checksum")
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, data, 0644); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(data)
	expected := hex.EncodeToString(h[:])

	if err := mgr.verifyChecksum(f, expected); err != nil {
		t.Fatalf("verifyChecksum should pass: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	mgr := testManager(t)

	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("actual"), 0644); err != nil {
		t.Fatal(err)
	}

	err := mgr.verifyChecksum(f, "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyChecksum_FileNotFound(t *testing.T) {
	mgr := testManager(t)
	err := mgr.verifyChecksum("/nonexistent/file", "abc")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// ---------------------------------------------------------------------------
// DeleteImage — verify directory is actually removed from disk
// ---------------------------------------------------------------------------

func TestDeleteImage_RemovesDirectory(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "kernel"), []byte("del"), 0644); err != nil {
		t.Fatal(err)
	}
	info, err := mgr.ImportImage(ctx, "rm-dir", filepath.Join(srcDir, "kernel"), "", "")
	if err != nil {
		t.Fatal(err)
	}

	imgDir := filepath.Dir(info.KernelPath)

	if err := mgr.DeleteImage(ctx, "rm-dir"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(imgDir); !os.IsNotExist(err) {
		t.Errorf("image directory should have been removed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// loadImages — edge cases
// ---------------------------------------------------------------------------

func TestLoadImages_EmptyImageDir(t *testing.T) {
	storageDir := t.TempDir()
	cacheDir := t.TempDir()

	// Create an empty subdirectory (image with no artifacts).
	imgDir := filepath.Join(storageDir, "empty-image")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		t.Fatal(err)
	}

	mgr, err := NewManager(&Config{
		StoragePath: storageDir,
		CachePath:   cacheDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := mgr.GetImage(context.Background(), "empty-image")
	if err != nil {
		t.Fatalf("empty image dir should still be loadable: %v", err)
	}
	if info.KernelSize != 0 || info.InitrdSize != 0 || info.RootFSSize != 0 {
		t.Error("empty image should have zero sizes")
	}
}

func TestLoadImages_MultipleImages(t *testing.T) {
	storageDir := t.TempDir()
	cacheDir := t.TempDir()

	for _, name := range []string{"img-a", "img-b", "img-c"} {
		dir := filepath.Join(storageDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "kernel"), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mgr, err := NewManager(&Config{
		StoragePath: storageDir,
		CachePath:   cacheDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	images, err := mgr.ListImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 3 {
		t.Errorf("expected 3 images, got %d", len(images))
	}
}

// ---------------------------------------------------------------------------
// loadImageInfo — all component paths detected
// ---------------------------------------------------------------------------

func TestLoadImageInfo_AllComponents(t *testing.T) {
	storageDir := t.TempDir()
	cacheDir := t.TempDir()

	imgDir := filepath.Join(storageDir, "full")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, comp := range []struct {
		name string
		data string
	}{
		{"kernel", "kern-data"},
		{"initrd", "init-data"},
		{"rootfs.squashfs", "root-data"},
	} {
		if err := os.WriteFile(filepath.Join(imgDir, comp.name), []byte(comp.data), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mgr, err := NewManager(&Config{
		StoragePath: storageDir,
		CachePath:   cacheDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := mgr.GetImage(context.Background(), "full")
	if err != nil {
		t.Fatal(err)
	}

	if info.KernelPath == "" || info.InitrdPath == "" || info.RootFSPath == "" {
		t.Error("all component paths should be set")
	}
	if info.KernelSize != int64(len("kern-data")) {
		t.Errorf("KernelSize = %d, want %d", info.KernelSize, len("kern-data"))
	}
	if info.InitrdSize != int64(len("init-data")) {
		t.Errorf("InitrdSize = %d, want %d", info.InitrdSize, len("init-data"))
	}
	if info.RootFSSize != int64(len("root-data")) {
		t.Errorf("RootFSSize = %d, want %d", info.RootFSSize, len("root-data"))
	}
}

// ---------------------------------------------------------------------------
// GetStorageUsage — multiple images
// ---------------------------------------------------------------------------

func TestGetStorageUsage_MultipleImages(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	for _, name := range []string{"kernel", "initrd"} {
		if err := os.WriteFile(filepath.Join(srcDir, name), []byte(name+"-bytes"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := mgr.ImportImage(ctx, "img1", filepath.Join(srcDir, "kernel"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = mgr.ImportImage(ctx, "img2", filepath.Join(srcDir, "kernel"), filepath.Join(srcDir, "initrd"), "")
	if err != nil {
		t.Fatal(err)
	}

	usage, err := mgr.GetStorageUsage()
	if err != nil {
		t.Fatal(err)
	}

	if len(usage.Images) != 2 {
		t.Errorf("expected 2 images in usage, got %d", len(usage.Images))
	}

	kernelLen := int64(len("kernel-bytes"))
	initrdLen := int64(len("initrd-bytes"))
	expectedTotal := kernelLen + kernelLen + initrdLen
	if usage.TotalBytes != expectedTotal {
		t.Errorf("TotalBytes = %d, want %d", usage.TotalBytes, expectedTotal)
	}
}

// ---------------------------------------------------------------------------
// NixOSBuilder — extractNetbootArtifacts paths
// ---------------------------------------------------------------------------

func TestExtractNetbootArtifacts_NoKernelNoBzImage(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	// Store path with no kernel and no bzImage.
	storePath := t.TempDir()

	result, err := b.extractNetbootArtifacts(context.Background(), storePath)
	// Will fail at buildSquashFS (no mksquashfs), but we exercise the
	// code path where neither kernel nor bzImage exist.
	_ = err
	_ = result
}

func TestExtractNetbootArtifacts_WithKernelParams(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	storePath := t.TempDir()
	os.WriteFile(filepath.Join(storePath, "kernel"), []byte("k"), 0644)
	os.WriteFile(filepath.Join(storePath, "initrd"), []byte("i"), 0644)
	os.WriteFile(filepath.Join(storePath, "kernel-params"), []byte("  root=/dev/sda1 quiet  \n"), 0644)

	// Will fail at buildSquashFS but we exercise kernel-params reading + trimming.
	_, _ = b.extractNetbootArtifacts(context.Background(), storePath)
}

func TestExtractNetbootArtifacts_NoKernelParams(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	storePath := t.TempDir()
	os.WriteFile(filepath.Join(storePath, "kernel"), []byte("k"), 0644)
	// No kernel-params file.

	_, _ = b.extractNetbootArtifacts(context.Background(), storePath)
}

// ---------------------------------------------------------------------------
// NixOSBuilder — buildSquashFS error paths
// ---------------------------------------------------------------------------

func TestBuildSquashFS_NoMksquashfs(t *testing.T) {
	cacheDir := t.TempDir()
	b := NewNixOSBuilder(cacheDir)

	// getStoreClosure will fail because nix-store isn't available in test.
	_, err := b.buildSquashFS(context.Background(), "/nonexistent/store")
	if err == nil {
		t.Fatal("expected error when nix-store is not available")
	}
}

// ---------------------------------------------------------------------------
// NixOSBuilder — getStoreClosure error path
// ---------------------------------------------------------------------------

func TestGetStoreClosure_Error(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	_, err := b.getStoreClosure(context.Background(), "/nonexistent/store")
	if err == nil {
		t.Fatal("expected error from nix-store command")
	}
}

// ---------------------------------------------------------------------------
// NixOSBuilder — buildSystem error path
// ---------------------------------------------------------------------------

func TestBuildSystem_NoNix(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	_, err := b.buildSystem(context.Background(), "github:fake/flake", "x86_64-linux")
	if err == nil {
		t.Fatal("expected error when nix is not available")
	}
}

// ---------------------------------------------------------------------------
// NixOSBuilder — Build orchestrator error path
// ---------------------------------------------------------------------------

func TestBuild_BuildSystemFails(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	// nix command not available, so buildSystem fails.
	_, err := b.Build(context.Background(), "github:fake/flake", "x86_64-linux")
	if err == nil {
		t.Fatal("expected error from Build when nix is unavailable")
	}
	if !strings.Contains(err.Error(), "building system") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NixOSBuilder — BuildNetboot error path
// ---------------------------------------------------------------------------

func TestBuildNetboot_NoNix(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	// Both the primary nix build and the fallback Build call will fail.
	_, err := b.BuildNetboot(context.Background(), "github:fake/flake", "x86_64-linux")
	if err == nil {
		t.Fatal("expected error from BuildNetboot when nix is unavailable")
	}
}

// ---------------------------------------------------------------------------
// NixOSBuilder — BuildKexec error path
// ---------------------------------------------------------------------------

func TestBuildKexec_NoNix(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	_, err := b.BuildKexec(context.Background(), "github:fake/flake", "x86_64-linux")
	if err == nil {
		t.Fatal("expected error from BuildKexec when nix is unavailable")
	}
}

// ---------------------------------------------------------------------------
// NixOSBuilder — GenerateHardwareConfig error path
// ---------------------------------------------------------------------------

func TestGenerateHardwareConfig_BadHost(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	_, err := b.GenerateHardwareConfig(context.Background(), "nonexistent-host-12345")
	if err == nil {
		t.Fatal("expected error from GenerateHardwareConfig for unreachable host")
	}
	if !strings.Contains(err.Error(), "generating hardware config") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NixOSBuilder — GenerateInstallerScript edge cases
// ---------------------------------------------------------------------------

func TestGenerateInstallerScript_EmptyFields(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	config := &NixOSInstallerConfig{
		DiskDevice: "",
		Hostname:   "",
		FlakeRef:   "",
	}

	script := b.GenerateInstallerScript(config)

	// Template vars should be replaced with empty strings.
	if strings.Contains(script, "${DISK_DEVICE}") {
		t.Error("script still contains ${DISK_DEVICE}")
	}
	if strings.Contains(script, "${HOSTNAME}") {
		t.Error("script still contains ${HOSTNAME}")
	}
	if strings.Contains(script, "${FLAKE_REF}") {
		t.Error("script still contains ${FLAKE_REF}")
	}

	// Should still be a valid bash script.
	if !strings.Contains(script, "#!/bin/bash") {
		t.Error("script missing shebang")
	}
}

func TestGenerateInstallerScript_SpecialChars(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	config := &NixOSInstallerConfig{
		DiskDevice: "/dev/disk/by-id/scsi-SATA_VBOX_HARDDISK_VB12345678-abcdef01",
		Hostname:   "node-01.cluster.local",
		FlakeRef:   "git+ssh://git@github.com/org/infra.git?ref=main",
		SSHKeys:    []string{"ssh-ed25519 AAAA..."},
	}

	script := b.GenerateInstallerScript(config)

	if !strings.Contains(script, config.DiskDevice) {
		t.Error("script missing long disk device path")
	}
	if !strings.Contains(script, config.Hostname) {
		t.Error("script missing FQDN hostname")
	}
	if !strings.Contains(script, config.FlakeRef) {
		t.Error("script missing complex flake ref")
	}
}

// ---------------------------------------------------------------------------
// BuildNixOS — integration error path (nix unavailable)
// ---------------------------------------------------------------------------

func TestBuildNixOS_NoNix(t *testing.T) {
	mgr := testManager(t)

	_, err := mgr.BuildNixOS(context.Background(), "github:fake/flake", "x86_64-linux")
	if err == nil {
		t.Fatal("expected error from BuildNixOS when nix is unavailable")
	}
}

// ---------------------------------------------------------------------------
// Manager concurrent delete + verify race
// ---------------------------------------------------------------------------

func TestManagerConcurrentDeleteAndVerify(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "kernel"), []byte("race-data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Import several images.
	for i := 0; i < 5; i++ {
		id := "race-" + string(rune('a'+i))
		if _, err := mgr.ImportImage(ctx, id, filepath.Join(srcDir, "kernel"), "", ""); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	// Concurrent deletes and verifies.
	for i := 0; i < 5; i++ {
		id := "race-" + string(rune('a'+i))
		wg.Add(2)
		go func(id string) {
			defer wg.Done()
			_ = mgr.DeleteImage(ctx, id)
		}(id)
		go func(id string) {
			defer wg.Done()
			_ = mgr.VerifyImage(ctx, id)
		}(id)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Manager concurrent import + list + storage usage
// ---------------------------------------------------------------------------

func TestManagerConcurrentImportAndUsage(t *testing.T) {
	mgr := testManager(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "kernel"), []byte("concurrent-usage"), 0644); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "usage-" + string(rune('a'+i))
			_, _ = mgr.ImportImage(ctx, id, filepath.Join(srcDir, "kernel"), "", "")
			_, _ = mgr.GetStorageUsage()
			_, _ = mgr.ListImages(ctx)
		}(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// NewManager — error creating storage dir
// ---------------------------------------------------------------------------

func TestNewManager_BadStoragePath(t *testing.T) {
	_, err := NewManager(&Config{
		StoragePath: "/proc/nonexistent/storage",
		CachePath:   t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected error for invalid storage path")
	}
	if !strings.Contains(err.Error(), "creating directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewManager_BadCachePath(t *testing.T) {
	_, err := NewManager(&Config{
		StoragePath: t.TempDir(),
		CachePath:   "/proc/nonexistent/cache",
	})
	if err == nil {
		t.Fatal("expected error for invalid cache path")
	}
	if !strings.Contains(err.Error(), "creating directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestBuild_CancelledContext(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := b.Build(ctx, "github:fake/flake", "x86_64-linux")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestBuildKexec_CancelledContext(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.BuildKexec(ctx, "github:fake/flake", "x86_64-linux")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestBuildNetboot_CancelledContext(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.BuildNetboot(ctx, "github:fake/flake", "x86_64-linux")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestGenerateHardwareConfig_CancelledContext(t *testing.T) {
	b := NewNixOSBuilder(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.GenerateHardwareConfig(ctx, "some-host")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}
