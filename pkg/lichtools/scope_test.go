// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// newTestScope builds a Scope over a temp harness dir containing one real
// harness file (lich_001_example_test.go).
func newTestScope(t *testing.T) (*Scope, string) {
	t.Helper()
	dir := t.TempDir()
	harnessDir := filepath.Join(dir, "harnesses")
	if err := os.MkdirAll(harnessDir, 0o750); err != nil {
		t.Fatalf("mkdir harnesses: %v", err)
	}
	if err := os.WriteFile(filepath.Join(harnessDir, "lich_001_example_test.go"), []byte("package x\n"), 0o600); err != nil {
		t.Fatalf("write harness: %v", err)
	}
	s, err := NewScope("lich-sbx-", harnessDir)
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	return s, harnessDir
}

func TestScope_Validate_Sandbox(t *testing.T) {
	s, _ := newTestScope(t)
	tests := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"prefixed name passes", "lich-sbx-001-ab12", false},
		{"prefixed single suffix", "lich-sbx-x", false},
		{"bare hostname refused", "evil-host", true},
		{"ip refused", "1.1.1.1", true},
		{"unlisted container refused", "my-other-ctr", true},
		{"prefix-only no suffix refused", "lich-sbx-", true},
		{"dotpath refused", "../etc", true},
		{"prefix as substring not front refused", "x-lich-sbx-1", true},
		{"uppercase refused", "lich-sbx-ABC", true},
		{"underscore refused", "lich-sbx-a_b", true},
		{"slash refused", "lich-sbx-a/b", true},
		{"empty refused", "", true},
		{"newline in name refused", "lich-sbx-a\nb", true},
		{"homoglyph prefix refused", "lich-sbx-аbc", true}, // Cyrillic 'а'
		{"trailing hyphen refused", "lich-sbx-a-", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Validate(Target{Kind: KindSandbox, Name: tt.target})
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Fatalf("Validate(%q) err=%v, wantErr=%v", tt.target, err, tt.wantErr)
			}
			if gotErr && !errors.Is(err, ErrOutOfScope) {
				t.Fatalf("Validate(%q) err %v is not ErrOutOfScope", tt.target, err)
			}
		})
	}
}

func TestScope_Validate_Harness(t *testing.T) {
	s, _ := newTestScope(t)
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"existing three-digit id passes", "001", false},
		{"nonexistent id refused", "999", true},
		{"non-numeric refused", "evil-host", true},
		{"path traversal refused", "../001", true},
		{"slash refused", "001/x", true},
		{"two-digit refused", "01", true},
		{"four-digit refused", "0012", true},
		{"empty refused", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Validate(Target{Kind: KindHarness, Name: tt.id})
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Fatalf("Validate(harness %q) err=%v, wantErr=%v", tt.id, err, tt.wantErr)
			}
			if gotErr && !errors.Is(err, ErrOutOfScope) {
				t.Fatalf("Validate(harness %q) err %v is not ErrOutOfScope", tt.id, err)
			}
		})
	}
}

// TestScope_Validate_SymlinkEscape: a harness file that is a symlink pointing
// outside the harness dir must be refused (BlackMage self-review, Step 43).
func TestScope_Validate_SymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on windows")
	}
	s, harnessDir := newTestScope(t)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("sensitive\n"), 0o600); err != nil {
		t.Fatalf("write outside: %v", err)
	}
	link := filepath.Join(harnessDir, "lich_777_evil_test.go")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	err := s.Validate(Target{Kind: KindHarness, Name: "777"})
	if err == nil || !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("symlink-escaping harness must be ErrOutOfScope, got %v", err)
	}
}

func TestScope_Validate_UnknownKind(t *testing.T) {
	s, _ := newTestScope(t)
	if err := s.Validate(Target{Kind: KindUnknown, Name: "lich-sbx-x"}); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("unknown kind must be ErrOutOfScope, got %v", err)
	}
}

func TestNewScope_Rejects_BadInput(t *testing.T) {
	if _, err := NewScope("", "/tmp/h"); err == nil {
		t.Fatal("empty prefix should error")
	}
	if _, err := NewScope("lich-sbx-", "relative/path"); err == nil {
		t.Fatal("relative harness dir should error")
	}
}
