// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTopicConfig_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wotan.yaml")
	content := `topics:
  auto_approve:
    - "timeguru"
    - "captain"
    - "monad"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Topics.AutoApprove) != 3 {
		t.Fatalf("expected 3 auto-approve entries, got %d", len(cfg.Topics.AutoApprove))
	}
	if !cfg.IsAutoApproved("timeguru") {
		t.Error("timeguru should be auto-approved")
	}
	if !cfg.IsAutoApproved("captain") {
		t.Error("captain should be auto-approved")
	}
	if !cfg.IsAutoApproved("monad") {
		t.Error("monad should be auto-approved")
	}
}

func TestLoadTopicConfig_MissingFile(t *testing.T) {
	cfg, err := LoadTopicConfig("/nonexistent/path/wotan.yaml")
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	// Default: deny all
	if cfg.IsAutoApproved("timeguru") {
		t.Error("missing config should deny all auto-approvals")
	}
}

func TestLoadTopicConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadTopicConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadTopicConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IsAutoApproved("anything") {
		t.Error("empty config should deny all")
	}
}

func TestIsAutoApproved_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wotan.yaml")
	content := `topics:
  auto_approve:
    - "Timeguru"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name string
		want bool
	}{
		{"timeguru", true},
		{"Timeguru", true},
		{"TIMEGURU", true},
		{"captain", false},
	}
	for _, tc := range cases {
		if got := cfg.IsAutoApproved(tc.name); got != tc.want {
			t.Errorf("IsAutoApproved(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsAutoApproved_Wildcard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wotan.yaml")
	content := `topics:
  auto_approve:
    - "*"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.IsAutoApproved("anything-at-all") {
		t.Error("wildcard should approve everything")
	}
	if !cfg.IsAutoApproved("unknown-service") {
		t.Error("wildcard should approve everything")
	}
}

func TestIsAutoApproved_NilConfig(t *testing.T) {
	var cfg *TopicConfig
	if cfg.IsAutoApproved("test") {
		t.Error("nil config should deny all")
	}
}

func TestIsAutoApproved_NotInList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wotan.yaml")
	content := `topics:
  auto_approve:
    - "timeguru"
    - "captain"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.IsAutoApproved("evil-service") {
		t.Error("unlisted service should not be auto-approved")
	}
	if cfg.IsAutoApproved("") {
		t.Error("empty name should not be auto-approved")
	}
}

func TestIsAutoApproved_WhitespaceHandling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wotan.yaml")
	content := `topics:
  auto_approve:
    - "  timeguru  "
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.IsAutoApproved("timeguru") {
		t.Error("whitespace in config should be trimmed")
	}
	if !cfg.IsAutoApproved("  timeguru  ") {
		t.Error("whitespace in input should be trimmed")
	}
}
