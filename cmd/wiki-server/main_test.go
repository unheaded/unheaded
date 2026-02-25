// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestWiki(t *testing.T) *WikiServer {
	t.Helper()
	dir := t.TempDir()

	// Create test markdown files.
	readme := `# Welcome

This is the homepage.

## Links

- [Test Page](test-page.md)
`
	testPage := `# Test Page

Some content with **bold** and ` + "`code`" + `.

| Col A | Col B |
|-------|-------|
| 1     | 2     |
`
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test-page.md"), []byte(testPage), 0644); err != nil {
		t.Fatal(err)
	}

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestHealthEndpoint(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	ws.handleHealth(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health: got status %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"status":"healthy"`) {
		t.Errorf("health: body missing healthy status: %s", body)
	}
	if !strings.Contains(string(body), `"service":"wiki-server"`) {
		t.Errorf("health: body missing service name: %s", body)
	}
}

func TestReadyEndpoint(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	ws.handleReady(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("ready: got status %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"status":"ready"`) {
		t.Errorf("ready: body missing ready status: %s", body)
	}
}

func TestWikiHomepage(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/wiki/", nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("wiki homepage: got status %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	if !strings.Contains(html, "Welcome") {
		t.Error("wiki homepage: missing 'Welcome' heading")
	}
	if !strings.Contains(html, "Unheaded Wiki") {
		t.Error("wiki homepage: missing sidebar brand")
	}
	if !strings.Contains(html, "Home") {
		t.Error("wiki homepage: missing Home nav item")
	}
}

func TestWikiSubpage(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/wiki/test-page", nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("wiki subpage: got status %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	if !strings.Contains(html, "Test Page") {
		t.Error("wiki subpage: missing 'Test Page' heading")
	}
	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Error("wiki subpage: markdown bold not rendered")
	}
	if !strings.Contains(html, "<code>code</code>") {
		t.Error("wiki subpage: markdown code not rendered")
	}
	if !strings.Contains(html, "<table>") {
		t.Error("wiki subpage: markdown table not rendered")
	}
}

func TestWikiNotFound(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/wiki/nonexistent", nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("wiki 404: got status %d, want 404", resp.StatusCode)
	}
}

func TestPathTraversal(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/wiki/../../etc/passwd", nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	resp := w.Result()
	if resp.StatusCode == http.StatusOK {
		t.Error("wiki path traversal: should not return 200 for ../../etc/passwd")
	}
}

func TestListPages(t *testing.T) {
	ws := setupTestWiki(t)
	items := ws.listPages()

	if len(items) != 2 {
		t.Errorf("listPages: got %d items, want 2", len(items))
	}

	// Home should be first.
	if len(items) > 0 && items[0].Title != "Home" {
		t.Errorf("listPages: first item should be Home, got %q", items[0].Title)
	}
}

func TestSlugToTitle(t *testing.T) {
	tests := []struct {
		slug  string
		title string
	}{
		{"doom-over-ipv6", "Doom Over Ipv6"},
		{"architecture", "Architecture"},
		{"bug-kill-chain", "Bug Kill Chain"},
		{"protocol-specs", "Protocol Specs"},
	}

	for _, tc := range tests {
		got := slugToTitle(tc.slug)
		if got != tc.title {
			t.Errorf("slugToTitle(%q) = %q, want %q", tc.slug, got, tc.title)
		}
	}
}
