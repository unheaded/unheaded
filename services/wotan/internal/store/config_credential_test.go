// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNew_PostgresWithoutConnStr_Fails and its Hybrid twin lock in the fix for
// gosec G101: both store types silently fell back to a hardcoded
// "postgres://unheaded:unheaded_dev@...?sslmode=disable" when ConnStr was
// empty. A shipped credential over a cleartext connection, reached by
// misconfiguration rather than intent, is the worst combination — it looks
// healthy on the one host where that database exists.
func TestNew_PostgresWithoutConnStr_Fails(t *testing.T) {
	_, err := New(Config{Type: PostgresStoreType, Capacity: 10})
	if err == nil {
		t.Fatal("expected an error when ConnStr is empty, got nil — a built-in credential fallback has been reintroduced")
	}
	if !strings.Contains(err.Error(), "connection string") {
		t.Errorf("error should name the missing setting, got: %v", err)
	}
}

func TestNew_HybridWithoutConnStr_Fails(t *testing.T) {
	// Hybrid also requires DataDir, so supply one to prove ConnStr is what fails.
	_, err := New(Config{
		Type:     HybridStoreType,
		Capacity: 10,
		DataDir:  t.TempDir(),
	})
	if err == nil {
		t.Fatal("expected an error when ConnStr is empty, got nil")
	}
	if !strings.Contains(err.Error(), "connection string") {
		t.Errorf("error should name the missing setting, got: %v", err)
	}
}

// TestNoHardcodedCredentialInSource is the belt-and-braces half. The behavioural
// tests above pass if someone reintroduces the literal behind a different code
// path; this fails if the credential reappears in the package at all.
//
// Test files are exempt: pg_store_test.go legitimately uses that DSN against a
// throwaway local database.
func TestNoHardcodedCredentialInSource(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		// Split the needle so this test does not itself trip a secret scanner.
		needle := "unheaded:" + "unheaded_dev@"
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, needle) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				t.Errorf("%s: hardcoded credential reintroduced in non-comment code", name)
			}
		}
	}
}
