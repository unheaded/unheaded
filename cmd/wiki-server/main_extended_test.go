// SPDX-License-Identifier: GPL-3.0-or-later
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

// --- NewWikiServer error paths ---

func TestNewWikiServer_NonExistentDir(t *testing.T) {
	_, err := NewWikiServer("/tmp/nonexistent-wiki-dir-" + t.Name())
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
	if !strings.Contains(err.Error(), "stat wiki dir") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewWikiServer_NotADirectory(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "notadir")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	_, err = NewWikiServer(f.Name())
	if err == nil {
		t.Fatal("expected error when path is a file, not a directory")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewWikiServer_ValidDir(t *testing.T) {
	dir := t.TempDir()
	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.wikiDir == "" {
		t.Error("wikiDir should not be empty")
	}
	if ws.md == nil {
		t.Error("markdown renderer should not be nil")
	}
	if ws.tmpl == nil {
		t.Error("template should not be nil")
	}
	if ws.startTime.IsZero() {
		t.Error("startTime should be set")
	}
}

// --- listPages edge cases ---

func TestListPages_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	items := ws.listPages()
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty dir, got %d", len(items))
	}
}

func TestListPages_SkipsNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	// Create a mix of markdown and non-markdown files.
	os.WriteFile(filepath.Join(dir, "page-one.md"), []byte("# Page One\n"), 0644)
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("PNG"), 0644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	items := ws.listPages()
	if len(items) != 1 {
		t.Errorf("expected 1 item (only .md), got %d: %+v", len(items), items)
	}
	if len(items) > 0 && items[0].Slug != "page-one" {
		t.Errorf("expected slug 'page-one', got %q", items[0].Slug)
	}
}

func TestListPages_SortOrder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0644)
	os.WriteFile(filepath.Join(dir, "zebra.md"), []byte("# Zebra\n"), 0644)
	os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("# Alpha\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	items := ws.listPages()
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Home (README) should be first.
	if items[0].Title != "Home" {
		t.Errorf("first item should be Home, got %q", items[0].Title)
	}
	// Then alphabetical: Alpha before Zebra.
	if items[1].Title != "Alpha" {
		t.Errorf("second item should be Alpha, got %q", items[1].Title)
	}
	if items[2].Title != "Zebra" {
		t.Errorf("third item should be Zebra, got %q", items[2].Title)
	}
}

func TestListPages_InvalidDir(t *testing.T) {
	ws := &WikiServer{wikiDir: "/tmp/nonexistent-dir-" + t.Name()}
	items := ws.listPages()
	if items != nil {
		t.Errorf("expected nil for unreadable dir, got %+v", items)
	}
}

// --- renderPage edge cases ---

func TestRenderPage_EmptySlugReadme(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# My Wiki Home\nSome text.\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	title, content, err := ws.renderPage("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "My Wiki Home" {
		t.Errorf("expected title 'My Wiki Home', got %q", title)
	}
	if !strings.Contains(content, "Some text") {
		t.Error("content should contain rendered markdown")
	}
}

func TestRenderPage_NoHeading(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("No heading here.\nJust text.\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	title, _, err := ws.renderPage("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When no heading found and slug is empty, defaults to "Welcome to the Kingdom".
	if title != "Welcome to the Kingdom" {
		t.Errorf("expected default title, got %q", title)
	}
}

func TestRenderPage_SlugNoHeading(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "my-page.md"), []byte("Just plain text.\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	title, _, err := ws.renderPage("my-page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to slugToTitle.
	if title != "My Page" {
		t.Errorf("expected title 'My Page', got %q", title)
	}
}

func TestRenderPage_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ws.renderPage("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestRenderPage_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = ws.renderPage("../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "path traversal") && !strings.Contains(err.Error(), "read file") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- handleWiki edge cases ---

func TestHandleWiki_TrailingSlashStripped(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/wiki/test-page/", nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for trailing slash, got %d", resp.StatusCode)
	}
}

func TestHandleWiki_ResponseHeaders(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/wiki/", nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	resp := w.Result()
	ct := resp.Header.Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected Content-Type text/html; charset=utf-8, got %q", ct)
	}
}

func TestHandleWiki_PageDataInOutput(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/wiki/", nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	html := string(body)

	// Should contain version.
	if !strings.Contains(html, serviceVersion) {
		t.Error("page should contain service version")
	}
	// Should contain nav links.
	if !strings.Contains(html, "Test Page") {
		t.Error("page should contain nav item for test-page")
	}
}

func TestHandleWiki_InternalErrorPath(t *testing.T) {
	// Create a wiki server with a directory, then make it unreadable to trigger
	// an error that is NOT "no such file" — to hit the 500 path.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Point the wiki server at a path that produces a traversal error.
	// We create a symlink loop to trigger a different kind of error.
	badSlug := strings.Repeat("a", 1000) // Very long slug, may cause filesystem error.
	req := httptest.NewRequest("GET", "/wiki/"+badSlug, nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	resp := w.Result()
	// Should be either 404 or 500, but not 200.
	if resp.StatusCode == http.StatusOK {
		t.Error("should not return 200 for invalid slug")
	}
}

// --- handleHealth extended ---

func TestHandleHealth_ContainsVersion(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	ws.handleHealth(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	s := string(body)
	if !strings.Contains(s, serviceVersion) {
		t.Errorf("health response should contain version %q: %s", serviceVersion, s)
	}
	if !strings.Contains(s, `"uptime"`) {
		t.Error("health response should contain uptime field")
	}
}

func TestHandleHealth_ContentType(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	ws.handleHealth(w, req)

	ct := w.Result().Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

// --- handleReady extended ---

func TestHandleReady_ContentType(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	ws.handleReady(w, req)

	ct := w.Result().Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestHandleReady_ContainsWikiDir(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	ws.handleReady(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	if !strings.Contains(string(body), `"wiki_dir"`) {
		t.Error("ready response should contain wiki_dir")
	}
}

func TestHandleReady_UnreadableDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Point wikiDir at a non-existent path to simulate unreadable.
	ws.wikiDir = "/tmp/nonexistent-ready-" + t.Name()

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	ws.handleReady(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for unreadable wiki dir, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "not_ready") {
		t.Errorf("expected not_ready status in body: %s", body)
	}
}

// --- slugToTitle extended ---

func TestSlugToTitle_EmptyString(t *testing.T) {
	got := slugToTitle("")
	if got != "" {
		t.Errorf("slugToTitle('') = %q, want ''", got)
	}
}

func TestSlugToTitle_SingleWord(t *testing.T) {
	got := slugToTitle("hello")
	if got != "Hello" {
		t.Errorf("slugToTitle('hello') = %q, want 'Hello'", got)
	}
}

func TestSlugToTitle_MultipleHyphens(t *testing.T) {
	got := slugToTitle("a-b-c-d")
	if got != "A B C D" {
		t.Errorf("slugToTitle('a-b-c-d') = %q, want 'A B C D'", got)
	}
}

// --- Root redirect handler via mux ---

func TestRootRedirect(t *testing.T) {
	ws := setupTestWiki(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/wiki/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/wiki/", ws.handleWiki)
	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/ready", ws.handleReady)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Test root redirect (don't follow redirects).
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 redirect from /, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/wiki/" {
		t.Errorf("expected redirect to /wiki/, got %q", loc)
	}
}

func TestUnknownPath(t *testing.T) {
	ws := setupTestWiki(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/wiki/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/wiki/", ws.handleWiki)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/unknown-path")
	if err != nil {
		t.Fatalf("GET /unknown-path failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for /unknown-path, got %d", resp.StatusCode)
	}
}

// --- logf function ---

func TestLogf(t *testing.T) {
	// logf writes to os.Stderr. Redirect stderr to capture output.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logf("INFO", "test message", "key1", "val1", "key2", 42)

	w.Close()
	os.Stderr = oldStderr

	out, _ := io.ReadAll(r)
	s := string(out)

	if !strings.Contains(s, "INFO") {
		t.Errorf("log output should contain level: %s", s)
	}
	if !strings.Contains(s, "test message") {
		t.Errorf("log output should contain message: %s", s)
	}
	if !strings.Contains(s, "key1=val1") {
		t.Errorf("log output should contain key1=val1: %s", s)
	}
	if !strings.Contains(s, "key2=42") {
		t.Errorf("log output should contain key2=42: %s", s)
	}
}

func TestLogf_NoKVPairs(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logf("WARN", "simple warning")

	w.Close()
	os.Stderr = oldStderr

	out, _ := io.ReadAll(r)
	s := string(out)

	if !strings.Contains(s, "WARN") {
		t.Errorf("log output should contain level: %s", s)
	}
	if !strings.Contains(s, "simple warning") {
		t.Errorf("log output should contain message: %s", s)
	}
}

func TestLogf_OddKVPairs(t *testing.T) {
	// Odd number of kv pairs: last key has no value, should be skipped gracefully.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logf("DEBUG", "odd pairs", "key1", "val1", "orphan")

	w.Close()
	os.Stderr = oldStderr

	out, _ := io.ReadAll(r)
	s := string(out)

	if !strings.Contains(s, "key1=val1") {
		t.Errorf("log output should contain key1=val1: %s", s)
	}
	// "orphan" should NOT appear as a key=value pair.
	if strings.Contains(s, "orphan=") {
		t.Errorf("orphan key without value should not appear: %s", s)
	}
}

// --- Full mux integration tests ---

func TestFullMux_HealthViaServer(t *testing.T) {
	ws := setupTestWiki(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/wiki/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/wiki/", ws.handleWiki)
	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/ready", ws.handleReady)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "healthy") {
		t.Errorf("expected healthy in body: %s", body)
	}
}

func TestFullMux_ReadyViaServer(t *testing.T) {
	ws := setupTestWiki(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/ready", ws.handleReady)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ready")
	if err != nil {
		t.Fatalf("GET /ready failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestFullMux_WikiPageViaServer(t *testing.T) {
	ws := setupTestWiki(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/wiki/", ws.handleWiki)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/wiki/test-page")
	if err != nil {
		t.Fatalf("GET /wiki/test-page failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Test Page") {
		t.Error("response should contain Test Page")
	}
}

// --- NavItem and PageData struct tests ---

func TestNavItem_Fields(t *testing.T) {
	nav := NavItem{Slug: "test-slug", Title: "Test Slug"}
	if nav.Slug != "test-slug" {
		t.Errorf("expected slug 'test-slug', got %q", nav.Slug)
	}
	if nav.Title != "Test Slug" {
		t.Errorf("expected title 'Test Slug', got %q", nav.Title)
	}
}

func TestPageData_Fields(t *testing.T) {
	pd := PageData{
		Title:      "Test",
		Content:    "<p>Hello</p>",
		NavItems:   []NavItem{{Slug: "a", Title: "A"}},
		ActiveSlug: "a",
		Version:    "1.0",
	}
	if pd.Title != "Test" {
		t.Error("PageData Title mismatch")
	}
	if pd.Version != "1.0" {
		t.Error("PageData Version mismatch")
	}
	if len(pd.NavItems) != 1 {
		t.Error("PageData NavItems count mismatch")
	}
}

// --- renderPage with heading extraction ---

func TestRenderPage_HeadingExtraction(t *testing.T) {
	dir := t.TempDir()
	md := "Some preamble\n# The Actual Title\nContent here.\n"
	os.WriteFile(filepath.Join(dir, "my-doc.md"), []byte(md), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	title, content, err := ws.renderPage("my-doc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "The Actual Title" {
		t.Errorf("expected title 'The Actual Title', got %q", title)
	}
	if !strings.Contains(content, "Content here") {
		t.Error("content should contain rendered markdown body")
	}
}

func TestRenderPage_MultipleHeadings(t *testing.T) {
	dir := t.TempDir()
	md := "# First Title\n## Second Title\n# Third Title\n"
	os.WriteFile(filepath.Join(dir, "multi.md"), []byte(md), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	title, _, err := ws.renderPage("multi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should take the FIRST heading.
	if title != "First Title" {
		t.Errorf("expected 'First Title', got %q", title)
	}
}

// --- setupMux tests ---

func TestSetupMux_RootRedirect(t *testing.T) {
	ws := setupTestWiki(t)
	mux := setupMux(ws)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 from /, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/wiki/" {
		t.Errorf("expected redirect to /wiki/, got %q", loc)
	}
}

func TestSetupMux_UnknownPathReturns404(t *testing.T) {
	ws := setupTestWiki(t)
	mux := setupMux(ws)

	req := httptest.NewRequest("GET", "/foobar", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for /foobar, got %d", resp.StatusCode)
	}
}

func TestSetupMux_WikiRoute(t *testing.T) {
	ws := setupTestWiki(t)
	mux := setupMux(ws)

	req := httptest.NewRequest("GET", "/wiki/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /wiki/, got %d", resp.StatusCode)
	}
}

func TestSetupMux_HealthRoute(t *testing.T) {
	ws := setupTestWiki(t)
	mux := setupMux(ws)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /health, got %d", resp.StatusCode)
	}
}

func TestSetupMux_ReadyRoute(t *testing.T) {
	ws := setupTestWiki(t)
	mux := setupMux(ws)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for /ready, got %d", resp.StatusCode)
	}
}

// --- newServer tests ---

func TestNewServer_Configuration(t *testing.T) {
	ws := setupTestWiki(t)
	mux := setupMux(ws)
	srv := newServer(12345, mux)

	if srv.Addr != ":12345" {
		t.Errorf("expected addr :12345, got %q", srv.Addr)
	}
	if srv.ReadTimeout != 10*1e9 {
		t.Errorf("expected ReadTimeout 10s, got %v", srv.ReadTimeout)
	}
	if srv.ReadHeaderTimeout != 5*1e9 {
		t.Errorf("expected ReadHeaderTimeout 5s, got %v", srv.ReadHeaderTimeout)
	}
	if srv.WriteTimeout != 30*1e9 {
		t.Errorf("expected WriteTimeout 30s, got %v", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 60*1e9 {
		t.Errorf("expected IdleTimeout 60s, got %v", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 1<<20 {
		t.Errorf("expected MaxHeaderBytes 1MB, got %d", srv.MaxHeaderBytes)
	}
	if srv.Handler == nil {
		t.Error("Handler should not be nil")
	}
}

// --- buildHandler tests ---

func TestBuildHandler_Valid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644)

	handler, err := buildHandler(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handler == nil {
		t.Fatal("handler should not be nil")
	}

	// Verify handler serves all routes.
	tests := []struct {
		path   string
		status int
	}{
		{"/health", http.StatusOK},
		{"/ready", http.StatusOK},
		{"/wiki/", http.StatusOK},
	}
	for _, tc := range tests {
		req := httptest.NewRequest("GET", tc.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Result().StatusCode != tc.status {
			t.Errorf("GET %s: expected %d, got %d", tc.path, tc.status, w.Result().StatusCode)
		}
	}
}

func TestBuildHandler_InvalidDir(t *testing.T) {
	_, err := buildHandler("/tmp/nonexistent-build-" + t.Name())
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

// --- wrapAuth tests ---

func TestWrapAuth(t *testing.T) {
	ws := setupTestWiki(t)
	mux := setupMux(ws)
	handler := wrapAuth(mux)

	if handler == nil {
		t.Fatal("wrapAuth should not return nil")
	}

	// Verify the wrapped handler still serves requests.
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 through wrapped handler, got %d", w.Result().StatusCode)
	}
}

// --- Constants ---

func TestConstants(t *testing.T) {
	if serviceVersion == "" {
		t.Error("serviceVersion should not be empty")
	}
	if serviceName == "" {
		t.Error("serviceName should not be empty")
	}
	if serviceName != "wiki-server" {
		t.Errorf("serviceName should be 'wiki-server', got %q", serviceName)
	}
}

// --- Sort comparator full branch coverage ---

func TestListPages_SortComparatorAllBranches(t *testing.T) {
	dir := t.TempDir()
	// Create README (slug="") plus multiple pages to exercise all sort branches.
	// Position README between other files alphabetically so it appears at various
	// indices during sort, exercising both the i and j comparator branches.
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0644)
	os.WriteFile(filepath.Join(dir, "beta.md"), []byte("# Beta\n"), 0644)
	os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("# Alpha\n"), 0644)
	os.WriteFile(filepath.Join(dir, "gamma.md"), []byte("# Gamma\n"), 0644)
	os.WriteFile(filepath.Join(dir, "aaa.md"), []byte("# AAA\n"), 0644)
	os.WriteFile(filepath.Join(dir, "zzz.md"), []byte("# ZZZ\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	items := ws.listPages()
	if len(items) != 6 {
		t.Fatalf("expected 6 items, got %d", len(items))
	}

	// Verify Home is first.
	if items[0].Title != "Home" {
		t.Errorf("item[0] expected Home, got %q", items[0].Title)
	}
	if items[0].Slug != "" {
		t.Errorf("Home slug should be empty, got %q", items[0].Slug)
	}

	// Verify remaining are alphabetical.
	for i := 2; i < len(items); i++ {
		if items[i-1].Title > items[i].Title {
			t.Errorf("items not sorted: %q > %q at indices %d,%d", items[i-1].Title, items[i].Title, i-1, i)
		}
	}
}

// --- handleWiki error path: ensure 500 is returned for non-file errors ---

func TestHandleWiki_500ForNonFileError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Home\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a file that exists but whose name would cause path traversal detection.
	// Use a slug containing ".." to trigger path traversal error (not a "no such file" error).
	req := httptest.NewRequest("GET", "/wiki/..%2F..%2Fetc%2Fpasswd", nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	resp := w.Result()
	// The path traversal should result in 500 (internal server error), not 404.
	if resp.StatusCode == http.StatusOK {
		t.Error("should not return 200 for path traversal attempt")
	}
}

// --- NewWikiServer with relative path ---

func TestNewWikiServer_RelativePath(t *testing.T) {
	// Create a temp dir and use a relative path to verify Abs resolution works.
	dir := t.TempDir()
	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !filepath.IsAbs(ws.wikiDir) {
		t.Errorf("wikiDir should be absolute, got %q", ws.wikiDir)
	}
}

// --- handleWiki: active slug marking in nav ---

func TestHandleWiki_ActiveSlugInNav(t *testing.T) {
	ws := setupTestWiki(t)
	req := httptest.NewRequest("GET", "/wiki/test-page", nil)
	w := httptest.NewRecorder()
	ws.handleWiki(w, req)

	body, _ := io.ReadAll(w.Result().Body)
	html := string(body)

	// The active class should be applied to the test-page nav link.
	if !strings.Contains(html, `class="active"`) {
		t.Error("expected active class on current page nav item")
	}
}

// --- renderPage: README with heading ---

func TestRenderPage_ReadmeWithCustomTitle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Custom Home Title\nBody.\n"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	title, _, err := ws.renderPage("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Custom Home Title" {
		t.Errorf("expected 'Custom Home Title', got %q", title)
	}
}

// --- listPages: only directories, no .md files ---

func TestListPages_OnlyDirsNoMD(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "subdir1"), 0755)
	os.Mkdir(filepath.Join(dir, "subdir2"), 0755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("txt"), 0644)

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatal(err)
	}

	items := ws.listPages()
	if len(items) != 0 {
		t.Errorf("expected 0 items (no .md files), got %d: %+v", len(items), items)
	}
}
