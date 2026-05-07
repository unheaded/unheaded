// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package captain

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFileStorage(t *testing.T) {
	// Set CAPTAIN_DATA_DIR so that the empty-path test case uses a writable
	// temp directory instead of the production default /var/lib/unheaded/captain.
	envDir := t.TempDir()
	t.Setenv("CAPTAIN_DATA_DIR", envDir)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "empty path",
			path:    "",
			wantErr: false, // Uses CAPTAIN_DATA_DIR env
		},
		{
			name:    "path too long",
			path:    string(make([]byte, 1025)),
			wantErr: true,
		},
		{
			name:    "valid path",
			path:    t.TempDir(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewFileStorage(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewFileStorage error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFileStorage_SaveAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewFileStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileStorage failed: %v", err)
	}

	ctx := context.Background()
	decision := &Decision{
		ID:       "test-123",
		Title:    "Test Decision",
		Content:  "Test content",
		Owner:    "captain",
		Priority: PriorityHigh,
	}

	// Save decision
	if err := storage.SaveDecision(ctx, decision); err != nil {
		t.Fatalf("SaveDecision failed: %v", err)
	}

	// Verify file exists
	expectedPath := filepath.Join(tmpDir, "test-123.json")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("decision file not created: %v", err)
	}

	// Get decision
	retrieved, err := storage.GetDecision(ctx, "test-123")
	if err != nil {
		t.Fatalf("GetDecision failed: %v", err)
	}

	if retrieved.Title != decision.Title {
		t.Errorf("title mismatch: got %s, want %s", retrieved.Title, decision.Title)
	}
}

func TestFileStorage_SafeFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	storage, _ := NewFileStorage(tmpDir)

	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "empty ID",
			id:      "",
			wantErr: true,
		},
		{
			name:    "ID too long",
			id:      string(make([]byte, 201)),
			wantErr: true,
		},
		{
			name:    "path traversal ..",
			id:      "../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal dot",
			id:      "..",
			wantErr: true,
		},
		{
			name:    "invalid character",
			id:      "test/path",
			wantErr: true,
		},
		{
			name:    "valid ID with underscore",
			id:      "test_id_123",
			wantErr: false,
		},
		{
			name:    "valid ID with hyphen",
			id:      "test-id-123",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := storage.safeFilePath(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("safeFilePath error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFileStorage_DeleteDecision(t *testing.T) {
	tmpDir := t.TempDir()
	storage, _ := NewFileStorage(tmpDir)
	ctx := context.Background()

	decision := &Decision{
		ID:       "test-123",
		Title:    "Test",
		Content:  "Content",
		Owner:    "captain",
		Priority: PriorityHigh,
	}

	// Save decision
	storage.SaveDecision(ctx, decision)

	// Delete decision
	if err := storage.DeleteDecision(ctx, "test-123"); err != nil {
		t.Fatalf("DeleteDecision failed: %v", err)
	}

	// Verify file is gone
	_, err := storage.GetDecision(ctx, "test-123")
	if err == nil {
		t.Error("decision should not exist after deletion")
	}
}

func TestFileStorage_InvalidDecision(t *testing.T) {
	tmpDir := t.TempDir()
	storage, _ := NewFileStorage(tmpDir)
	ctx := context.Background()

	tests := []struct {
		name     string
		decision *Decision
		wantErr  bool
	}{
		{
			name:     "nil decision",
			decision: nil,
			wantErr:  true,
		},
		{
			name: "no ID",
			decision: &Decision{
				Title:    "Test",
				Content:  "Content",
				Owner:    "captain",
				Priority: PriorityHigh,
			},
			wantErr: true,
		},
		{
			name: "invalid decision",
			decision: &Decision{
				ID:      "test",
				Title:   "",
				Content: "Content",
				Owner:   "captain",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := storage.SaveDecision(ctx, tt.decision)
			if (err != nil) != tt.wantErr {
				t.Errorf("SaveDecision error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMemoryStorage(t *testing.T) {
	storage := NewMemoryStorage()
	ctx := context.Background()

	decision := &Decision{
		ID:       "test-123",
		Title:    "Test Decision",
		Content:  "Test content",
		Owner:    "captain",
		Priority: PriorityHigh,
	}

	// Save decision
	if err := storage.SaveDecision(ctx, decision); err != nil {
		t.Fatalf("SaveDecision failed: %v", err)
	}

	// Get decision
	retrieved, err := storage.GetDecision(ctx, "test-123")
	if err != nil {
		t.Fatalf("GetDecision failed: %v", err)
	}

	if retrieved.Title != decision.Title {
		t.Errorf("title mismatch")
	}

	// Delete decision
	if err := storage.DeleteDecision(ctx, "test-123"); err != nil {
		t.Fatalf("DeleteDecision failed: %v", err)
	}

	// Verify deleted
	_, err = storage.GetDecision(ctx, "test-123")
	if err == nil {
		t.Error("decision should not exist after deletion")
	}
}
