// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── LoadManifest ────────────────────────────────────────────────────────────

func TestLoadManifest_ParsesValidBaselineManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "mjolnir.yaml")
	body := `apiVersion: v1
kind: BaselineManifest
metadata:
  name: kingdom-baseline
  version: "0.1.0"
  created: 2026-05-09
  signed_by: heimdall
spec:
  base_image:
    distro: nixos
    release: "25.05"
    digest: sha256:abc
  files:
    - path: /etc/nixos/configuration.nix
      sha256: deadbeef
      mode: "0644"
      owner: root
  packages:
    - name: bash
      version: "5.2"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Kind != "BaselineManifest" {
		t.Errorf("Kind = %q, want BaselineManifest", m.Kind)
	}
	if m.Metadata.Name != "kingdom-baseline" {
		t.Errorf("Metadata.Name = %q", m.Metadata.Name)
	}
	if len(m.Spec.Files) != 1 || m.Spec.Files[0].SHA256 != "deadbeef" {
		t.Errorf("Spec.Files unexpected: %+v", m.Spec.Files)
	}
	if len(m.Spec.Packages) != 1 || m.Spec.Packages[0].Name != "bash" {
		t.Errorf("Spec.Packages unexpected: %+v", m.Spec.Packages)
	}
}

func TestLoadManifest_RejectsWrongKind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	body := `apiVersion: v1
kind: NotABaselineManifest
metadata:
  name: x
spec:
  base_image: {}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManifest(path)
	if err == nil {
		t.Fatal("expected error for wrong kind, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected kind") {
		t.Errorf("expected 'unexpected kind' in error, got: %v", err)
	}
}

func TestLoadManifest_MissingFileWraps(t *testing.T) {
	t.Parallel()
	_, err := LoadManifest("/nonexistent/mjolnir.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read manifest") {
		t.Errorf("expected wrapped 'read manifest' error, got: %v", err)
	}
}

func TestLoadManifest_InvalidYAMLWraps(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.yaml")
	if err := os.WriteFile(path, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadManifest(path)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "parse manifest") {
		t.Errorf("expected wrapped 'parse manifest' error, got: %v", err)
	}
}

// ─── hashFile ────────────────────────────────────────────────────────────────

func TestHashFile_ReturnsLowercaseHexSHA256(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	content := []byte("kingdom marches as one")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}

	expected := sha256.Sum256(content)
	want := hex.EncodeToString(expected[:])
	if got != want {
		t.Errorf("hashFile mismatch:\n  got:  %s\n  want: %s", got, want)
	}
	if len(got) != 64 {
		t.Errorf("expected 64-char hex digest, got %d chars", len(got))
	}
	if strings.ToLower(got) != got {
		t.Errorf("expected lowercase hex, got %q", got)
	}
}

func TestHashFile_EmptyFileMatchesEmptySHA256(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != emptySHA256 {
		t.Errorf("hashFile(empty) = %s, want %s", got, emptySHA256)
	}
}

func TestHashFile_MissingFileErrors(t *testing.T) {
	t.Parallel()
	_, err := hashFile("/nonexistent/file.bin")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
