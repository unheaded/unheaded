// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TestWithinDir covers the containment predicate that guards layer extraction.
//
// The previous implementation was
// strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)), which is a
// string-prefix test rather than a path-element test. The "sibling with shared
// prefix" case below is the one it got wrong, and it is the whole reason this
// helper exists.
func TestWithinDir(t *testing.T) {
	cases := []struct {
		name, dir, path string
		want            bool
	}{
		{"nested path", "/var/lib/store", "/var/lib/store/a/b", true},
		{"the dir itself", "/var/lib/store", "/var/lib/store", true},
		{"sibling with shared prefix", "/var/lib/store", "/var/lib/storeEVIL/x", false},
		{"dotdot escape", "/var/lib/store", "/var/lib/store/../../etc/shadow", false},
		{"absolute escape", "/var/lib/store", "/etc/shadow", false},
		{"deep dotdot", "/var/lib/store", "/var/lib/store/a/../../../etc/x", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withinDir(c.dir, c.path); got != c.want {
				t.Errorf("withinDir(%q, %q) = %v, want %v", c.dir, c.path, got, c.want)
			}
		})
	}
}

// TestResolveLinkTarget pins the HOST interpretation of a link target, which is
// the only one that matters for extraction safety.
//
// An earlier version of this test asserted the opposite — that an absolute
// linkname resolves image-relative (destDir + linkname). That reading is
// intuitive but unsafe, and TestExtractLayer_RejectsEscapes caught it: the
// check validated <destDir>/outside while os.Symlink stored the real /outside,
// so a follow-up entry wrote straight through the link. Validating a different
// path from the one you create is not a check at all.
//
// Consequence, stated plainly: an image shipping absolute internal symlinks
// (/bin/sh -> /bin/busybox) has them SKIPPED rather than extracted. That is a
// deliberate trade — the conservative direction, and the one that cannot
// escape. Revisit only alongside openat2/RESOLVE_BENEATH-style extraction.
func TestResolveLinkTarget(t *testing.T) {
	dest := "/img/root"

	if got := resolveLinkTarget(dest, "/img/root/bin/sh", "/bin/busybox"); got != "/bin/busybox" {
		t.Errorf("absolute link should stay host-absolute, got %q", got)
	}
	if withinDir(dest, resolveLinkTarget(dest, "/img/root/bin/sh", "/bin/busybox")) {
		t.Error("absolute link outside destDir must not be treated as contained")
	}
	if got := resolveLinkTarget(dest, "/img/root/bin/sh", "busybox"); got != "/img/root/bin/busybox" {
		t.Errorf("relative link: got %q", got)
	}
	if got := resolveLinkTarget(dest, "/img/root/bin/sh", "../../etc/shadow"); withinDir(dest, got) {
		t.Errorf("escaping relative link resolved to %q, which withinDir accepted", got)
	}
}

// writeLayer builds a gzipped tar from the given entries.
func writeLayer(t *testing.T, path string, entries []*tar.Header, bodies map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, h := range entries {
		if body, ok := bodies[h.Name]; ok {
			h.Size = int64(len(body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatalf("write header %s: %v", h.Name, err)
		}
		if body, ok := bodies[h.Name]; ok {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatalf("write body %s: %v", h.Name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

// TestExtractLayer_RejectsEscapes is the behavioural half: build layers that
// actually attempt each escape and assert nothing lands outside destDir.
func TestExtractLayer_RejectsEscapes(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "rootfs")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{dest, outside} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	// A file outside destDir that a hardlink would try to capture.
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("host-secret"), 0600); err != nil {
		t.Fatal(err)
	}

	layer := filepath.Join(root, "layer.tar.gz")
	writeLayer(t, layer,
		[]*tar.Header{
			// 1. Direct traversal via ../
			{Name: "../escaped-direct", Typeflag: tar.TypeReg, Mode: 0644},
			// 2. Symlink pointing outside, followed by a write "through" it.
			{Name: "evil", Typeflag: tar.TypeSymlink, Linkname: outside, Mode: 0777},
			{Name: "evil/pwn", Typeflag: tar.TypeReg, Mode: 0644},
			// 3. Hardlink to a file outside destDir.
			{Name: "stolen", Typeflag: tar.TypeLink, Linkname: "../outside/secret", Mode: 0644},
			// 4. A legitimate file, to prove extraction still works.
			{Name: "good.txt", Typeflag: tar.TypeReg, Mode: 0644},
		},
		map[string]string{
			"../escaped-direct": "nope",
			"evil/pwn":          "nope",
			"good.txt":          "fine",
		},
	)

	// Extraction may return an error on a rejected entry; what matters is that
	// nothing lands outside destDir.
	_ = extractLayer(layer, dest)

	for _, bad := range []string{
		filepath.Join(root, "escaped-direct"),
		filepath.Join(outside, "pwn"),
		filepath.Join(dest, "stolen"),
	} {
		if _, err := os.Lstat(bad); err == nil {
			t.Errorf("ESCAPE: %s was created outside the intended destination", bad)
		}
	}

	if got, err := os.ReadFile(filepath.Join(dest, "good.txt")); err != nil {
		t.Errorf("legitimate entry was not extracted: %v", err)
	} else if string(got) != "fine" {
		t.Errorf("good.txt = %q, want %q", got, "fine")
	}
}
