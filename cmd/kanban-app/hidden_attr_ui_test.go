// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package main

import (
	"regexp"
	"strings"
	"testing"
)

// TestHiddenAttributeHidesStyledElements: the page hides parts of its forms
// with the HTML `hidden` attribute (the GUID row of the task form, the
// detail sections). The browser's own rule for [hidden] is display:none at
// the lowest priority, so any class that sets `display` shows the element
// again. The New Task form showed an empty read-only "GUID" box that way
// from 2026-04-05 (`.form-group { display: flex }`), which read as a field
// the person had to fill (Stevie's card of 2026-11-11, "what is GUID field
// for?"). The stylesheet must say that hidden means hidden.
func TestHiddenAttributeHidesStyledElements(t *testing.T) {
	page, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`id="guidGroup"[^>]*\shidden[\s>]`).Match(page) {
		t.Fatal("index.html: the GUID row no longer starts hidden; this test guards the hidden attribute, look again")
	}
	css, err := staticFiles.ReadFile("static/css/main.css")
	if err != nil {
		t.Fatal(err)
	}
	rule := regexp.MustCompile(`(?s)\[hidden\]\s*\{[^}]*display\s*:\s*none\s*!important`)
	if !rule.Match(css) {
		t.Fatal("main.css has no `[hidden] { display: none !important; }`: a class that sets display (.form-group does) shows a hidden element")
	}
	// The rule must not sit inside a media query or another block that
	// limits it: it is found at the top level, before the first '@media'.
	text := string(css)
	if at := strings.Index(text, "@media"); at >= 0 && rule.FindStringIndex(text)[0] > at {
		t.Fatal("main.css: the [hidden] rule comes after the first @media block; keep it at the top level near the reset")
	}
}
