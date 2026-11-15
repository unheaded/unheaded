// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package repolink

import (
	"strings"
	"testing"
)

func TestBlobBase(t *testing.T) {
	if !strings.HasPrefix(BlobBase, "https://github.com/") || !strings.HasSuffix(BlobBase, "/blob/develop/") {
		t.Errorf("BlobBase = %q, want https://github.com/.../blob/develop/", BlobBase)
	}
}

func TestBlob(t *testing.T) {
	cases := []struct{ path, want string }{
		{"docs/adr/ADR-001.md", BlobBase + "docs/adr/ADR-001.md"},
		{"docs/a b.md", BlobBase + "docs/a%20b.md"},
		{"docs/a?b#c.md", BlobBase + "docs/a%3Fb%23c.md"},
		{"cmd/sword", BlobBase + "cmd/sword"},
		{"/docs/x.md", BlobBase + "docs/x.md"},
		{"./docs/x.md", BlobBase + "docs/x.md"},
		{"docs//x.md", BlobBase + "docs/x.md"},
		{"", ""},
		{"/", ""},
		{".", ""},
		{"..", ""},
		{"../etc/passwd", ""},
		{"docs/../../etc", ""},
		{"docs/../x.md", ""},
		{"docs/a..b.md", ""},
		{"docs/x\x00.md", ""},
	}
	for _, c := range cases {
		if got := Blob(c.path); got != c.want {
			t.Errorf("Blob(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
