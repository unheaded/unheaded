// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMarkdownRawHTMLIsNotLive is the regression test for stored XSS in the
// wiki renderer.
//
// goldmark was configured with html.WithUnsafe(), which passes raw HTML through
// verbatim. renderPage then wraps the result in template.HTML(), telling
// html/template the content is already safe so contextual escaping is skipped.
// Net effect: a <script> in any markdown file under the wiki dir executed in
// the browser of whoever viewed that page.
//
// With WithUnsafe() removed, goldmark neutralises raw HTML two different ways
// and the test must account for both:
//   - BLOCK-level HTML (<script>, <iframe>, <div>) becomes "<!-- raw HTML omitted -->"
//   - INLINE HTML (an <img> inside a paragraph) is entity-escaped
//
// So the assertion is "no LIVE tag opener survives", not "the payload text is
// absent" — escaped output legitimately still contains the characters
// onerror=alert('xss') between &lt; and &gt;, where they are inert.
func TestMarkdownRawHTMLIsNotLive(t *testing.T) {
	dir := t.TempDir()

	payloads := map[string]string{
		"script":  "# Title\n\n<script>alert('xss')</script>\n",
		"img":     "# Title\n\n<img src=x onerror=alert('xss')>\n",
		"iframe":  "# Title\n\n<iframe src=\"javascript:alert('xss')\"></iframe>\n",
		"onclick": "# Title\n\n<div onclick=\"alert('xss')\">click</div>\n",
		"svg":     "# Title\n\n<svg/onload=alert('xss')>\n",
	}
	for slug, body := range payloads {
		if err := os.WriteFile(filepath.Join(dir, slug+".md"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}

	ws, err := NewWikiServer(dir)
	if err != nil {
		t.Fatalf("NewWikiServer: %v", err)
	}

	// Live tag openers. If any of these appears unescaped, the browser parses
	// it as markup and the payload runs.
	liveTags := []string{"<script", "<iframe", "<img", "<div", "<svg"}

	for slug := range payloads {
		t.Run(slug, func(t *testing.T) {
			_, out, err := ws.renderPage(slug)
			if err != nil {
				t.Fatalf("renderPage(%q): %v", slug, err)
			}
			low := strings.ToLower(out)
			for _, tag := range liveTags {
				if strings.Contains(low, tag) {
					t.Errorf("STORED XSS: live %q survived rendering:\n%s", tag, out)
				}
			}
			// Sanity: the page rendered something, so we are not passing
			// merely because output is empty.
			if !strings.Contains(out, "<h1") {
				t.Errorf("expected the markdown heading to render, got:\n%s", out)
			}
		})
	}
}
