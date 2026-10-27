// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestWikiCSP_AllowsExactlyTheRenderedInlineBlocks: the browser hashes the
// inline block as served, so the CSP must carry the hash of the rendered
// text, not just of the template; and nothing else inline is allowed.
func TestWikiCSP_AllowsExactlyTheRenderedInlineBlocks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n\n<script>alert(1)</script>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := buildHandler(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/wiki/", nil))
	csp := w.Header().Get("Content-Security-Policy")
	if strings.Contains(csp, "unsafe-inline") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("CSP = %q", csp)
	}
	body := w.Body.String()
	for _, tag := range []string{"script", "style"} {
		re := regexp.MustCompile(`(?s)<` + tag + `>(.*?)</` + tag + `>`)
		blocks := re.FindAllStringSubmatch(body, -1)
		if len(blocks) == 0 {
			t.Fatalf("no inline <%s> in the page", tag)
		}
		for _, m := range blocks {
			sum := sha256.Sum256([]byte(m[1]))
			if !strings.Contains(csp, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
				t.Errorf("served <%s> block is not allowed by the CSP %q", tag, csp)
			}
		}
	}
	if n := strings.Count(csp, "'sha256-"); n != 2 {
		t.Errorf("CSP allows %d inline blocks, want 2 (the template's style and script)", n)
	}
}
