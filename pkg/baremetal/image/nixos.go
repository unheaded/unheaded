// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package image

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// NixOSBuilder builds NixOS images for bare metal provisioning.
type NixOSBuilder struct {
	cachePath string
}

// NewNixOSBuilder creates a new NixOS builder.
func NewNixOSBuilder(cachePath string) *NixOSBuilder {
	return &NixOSBuilder{
		cachePath: cachePath,
	}
}

// BuildResult holds the result of a NixOS build.
type BuildResult struct {
	// Paths to built artifacts
	KernelPath   string `json:"kernel_path"`
	InitrdPath   string `json:"initrd_path"`
	RootFSPath   string `json:"rootfs_path"`
	KernelParams string `json:"kernel_params"`

	// Build metadata
	FlakeRef  string        `json:"flake_ref"`
	System    string        `json:"system"`
	BuildTime time.Duration `json:"build_time"`
	StorePath string        `json:"store_path"`
}

// Build builds a NixOS image from a flake.
func (b *NixOSBuilder) Build(ctx context.Context, flakeRef, system string) (*BuildResult, error) {
	start := time.Now()

	// Build the NixOS system configuration
	storePath, err := b.buildSystem(ctx, flakeRef, system)
	if err != nil {
		return nil, fmt.Errorf("building system: %w", err)
	}

	// Extract netboot artifacts
	result, err := b.extractNetbootArtifacts(ctx, storePath)
	if err != nil {
		return nil, fmt.Errorf("extracting artifacts: %w", err)
	}

	result.FlakeRef = flakeRef
	result.System = system
	result.BuildTime = time.Since(start)
	result.StorePath = storePath

	return result, nil
}

// buildSystem builds the NixOS system.
func (b *NixOSBuilder) buildSystem(ctx context.Context, flakeRef, system string) (string, error) {
	// Build the toplevel derivation
	attribute := fmt.Sprintf("%s#nixosConfigurations.%s.config.system.build.toplevel", flakeRef, system)

	cmd := exec.CommandContext(ctx, "nix", "build", // #nosec G204 -- fixed binary; path/attr passed as separate argv, no shell
		"--no-link",
		"--print-out-paths",
		attribute,
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("nix build failed: %s", string(exitErr.Stderr))
		}
		return "", err
	}

	storePath := strings.TrimSpace(string(output))
	return storePath, nil
}

// extractNetbootArtifacts extracts netboot artifacts from a built NixOS system.
func (b *NixOSBuilder) extractNetbootArtifacts(ctx context.Context, storePath string) (*BuildResult, error) {
	result := &BuildResult{}

	// Find kernel
	kernelPath := filepath.Join(storePath, "kernel")
	if _, err := os.Stat(kernelPath); err == nil {
		result.KernelPath = kernelPath
	} else {
		// Try alternative location
		kernelPath = filepath.Join(storePath, "bzImage")
		if _, err := os.Stat(kernelPath); err == nil {
			result.KernelPath = kernelPath
		}
	}

	// Find initrd
	initrdPath := filepath.Join(storePath, "initrd")
	if _, err := os.Stat(initrdPath); err == nil {
		result.InitrdPath = initrdPath
	}

	// Read kernel parameters
	kernelParamsPath := filepath.Join(storePath, "kernel-params")
	if data, err := os.ReadFile(kernelParamsPath); err == nil {
		result.KernelParams = strings.TrimSpace(string(data))
	}

	// For rootfs, we build a squashfs
	rootfsPath, err := b.buildSquashFS(ctx, storePath)
	if err != nil {
		return nil, fmt.Errorf("building squashfs: %w", err)
	}
	result.RootFSPath = rootfsPath

	return result, nil
}

// buildSquashFS builds a squashfs from the NixOS store closure.
func (b *NixOSBuilder) buildSquashFS(ctx context.Context, storePath string) (string, error) {
	outputPath := filepath.Join(b.cachePath, fmt.Sprintf("rootfs-%d.squashfs", time.Now().UnixNano()))

	// Get the store closure
	closurePaths, err := b.getStoreClosure(ctx, storePath)
	if err != nil {
		return "", fmt.Errorf("getting store closure: %w", err)
	}

	// Create file list for squashfs
	listPath := filepath.Join(b.cachePath, fmt.Sprintf("closure-%d.list", time.Now().UnixNano()))
	if err := os.WriteFile(listPath, []byte(strings.Join(closurePaths, "\n")), 0644); err != nil {
		return "", fmt.Errorf("writing closure list: %w", err)
	}
	defer func() { _ = os.Remove(listPath) }()

	// Build squashfs
	cmd := exec.CommandContext(ctx, "mksquashfs", // #nosec G204 -- fixed binary; path/attr passed as separate argv, no shell
		"-",
		outputPath,
		"-comp", "zstd",
		"-Xcompression-level", "15",
		"-no-exports",
		"-no-xattrs",
	)
	cmd.Stdin = strings.NewReader(strings.Join(closurePaths, "\n"))

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mksquashfs failed: %w", err)
	}

	return outputPath, nil
}

// getStoreClosure returns the store closure for a path.
func (b *NixOSBuilder) getStoreClosure(ctx context.Context, storePath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "nix-store", "-qR", storePath) // #nosec G204 -- fixed binary; path/attr passed as separate argv, no shell
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	paths := strings.Split(strings.TrimSpace(string(output)), "\n")
	return paths, nil
}

// BuildNetboot builds the netboot derivation directly.
func (b *NixOSBuilder) BuildNetboot(ctx context.Context, flakeRef, system string) (*BuildResult, error) {
	start := time.Now()

	// Build the netboot derivation
	attribute := fmt.Sprintf("%s#nixosConfigurations.%s.config.system.build.netboot", flakeRef, system)

	cmd := exec.CommandContext(ctx, "nix", "build", // #nosec G204 -- fixed binary; path/attr passed as separate argv, no shell
		"--no-link",
		"--print-out-paths",
		attribute,
	)

	output, err := cmd.Output()
	if err != nil {
		// Fall back to building toplevel and extracting
		return b.Build(ctx, flakeRef, system)
	}

	storePath := strings.TrimSpace(string(output))

	result := &BuildResult{
		FlakeRef:   flakeRef,
		System:     system,
		BuildTime:  time.Since(start),
		StorePath:  storePath,
		KernelPath: filepath.Join(storePath, "bzImage"),
		InitrdPath: filepath.Join(storePath, "initrd"),
	}

	// Read kernel params
	paramsPath := filepath.Join(storePath, "netboot-params")
	if data, err := os.ReadFile(paramsPath); err == nil {
		result.KernelParams = strings.TrimSpace(string(data))
	}

	return result, nil
}

// BuildKexec builds a kexec-enabled initrd for in-place provisioning.
func (b *NixOSBuilder) BuildKexec(ctx context.Context, flakeRef, system string) (*BuildResult, error) {
	start := time.Now()

	// Build the kexec derivation
	attribute := fmt.Sprintf("%s#nixosConfigurations.%s.config.system.build.kexec", flakeRef, system)

	cmd := exec.CommandContext(ctx, "nix", "build", // #nosec G204 -- fixed binary; path/attr passed as separate argv, no shell
		"--no-link",
		"--print-out-paths",
		attribute,
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("nix build failed: %s", string(exitErr.Stderr))
		}
		return nil, err
	}

	storePath := strings.TrimSpace(string(output))

	result := &BuildResult{
		FlakeRef:   flakeRef,
		System:     system,
		BuildTime:  time.Since(start),
		StorePath:  storePath,
		KernelPath: filepath.Join(storePath, "bzImage"),
		InitrdPath: filepath.Join(storePath, "initrd"),
	}

	// Kexec script
	kexecScript := filepath.Join(storePath, "kexec-run")
	if _, err := os.Stat(kexecScript); err == nil {
		result.KernelParams = fmt.Sprintf("kexec_script=%s", kexecScript)
	}

	return result, nil
}

// GenerateHardwareConfig generates hardware configuration for a machine.
func (b *NixOSBuilder) GenerateHardwareConfig(ctx context.Context, targetHost string) (string, error) {
	// SSH to the target and run nixos-generate-config
	cmd := exec.CommandContext(ctx, "ssh", targetHost, // #nosec G204 -- fixed binary; path/attr passed as separate argv, no shell
		"nixos-generate-config", "--show-hardware-config",
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("generating hardware config: %w", err)
	}

	return string(output), nil
}

// NixOSInstallerConfig holds configuration for NixOS installer.
type NixOSInstallerConfig struct {
	// DiskDevice is the target disk
	DiskDevice string `json:"disk_device"`

	// Hostname is the target hostname
	Hostname string `json:"hostname"`

	// SSHKeys are authorized SSH keys
	SSHKeys []string `json:"ssh_keys"`

	// FlakeRef is the flake to install
	FlakeRef string `json:"flake_ref"`

	// ExtraConfig is additional NixOS configuration
	ExtraConfig string `json:"extra_config,omitempty"`
}

// GenerateInstallerScript generates an installer script for a machine.
func (b *NixOSBuilder) GenerateInstallerScript(config *NixOSInstallerConfig) string {
	script := `#!/bin/bash
set -euo pipefail

# Generated NixOS installer script
DISK="${DISK_DEVICE}"
HOSTNAME="${HOSTNAME}"

echo "Installing NixOS on ${DISK}..."

# Partition the disk
parted -s "${DISK}" mklabel gpt
parted -s "${DISK}" mkpart ESP fat32 1MiB 512MiB
parted -s "${DISK}" set 1 esp on
parted -s "${DISK}" mkpart primary 512MiB 100%

# Format partitions
mkfs.fat -F 32 -n boot "${DISK}1"
mkfs.ext4 -L nixos "${DISK}2"

# Mount partitions
mount "${DISK}2" /mnt
mkdir -p /mnt/boot
mount "${DISK}1" /mnt/boot

# Install NixOS
nixos-install --no-root-passwd --flake "${FLAKE_REF}#${HOSTNAME}"

echo "Installation complete!"
`

	script = strings.ReplaceAll(script, "${DISK_DEVICE}", config.DiskDevice)
	script = strings.ReplaceAll(script, "${HOSTNAME}", config.Hostname)
	script = strings.ReplaceAll(script, "${FLAKE_REF}", config.FlakeRef)

	return script
}
