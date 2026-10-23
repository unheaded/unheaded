// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tarEntry is one entry for makeTarGz.
type tarEntry struct {
	hdr  tar.Header
	body []byte
	// zeros, when > 0, streams that many zero bytes as the body (for size
	// caps without holding the data in the test either).
	zeros int64
}

func reg(name, body string) tarEntry {
	return tarEntry{hdr: tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))}, body: []byte(body)}
}

func makeTarGz(t testing.TB, entries ...tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := e.hdr
		if e.zeros > 0 {
			h.Size = e.zeros
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatalf("header %s: %v", h.Name, err)
		}
		switch {
		case e.zeros > 0:
			if _, err := io.CopyN(tw, zeroReader{}, e.zeros); err != nil {
				t.Fatal(err)
			}
		case len(e.body) > 0:
			if _, err := tw.Write(e.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func dir(name string) tarEntry {
	return tarEntry{hdr: tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755}}
}

func TestPack_ReadsValidPack(t *testing.T) {
	data := makeTarGz(t,
		dir("manifest/"),
		reg("manifest/manifest.yaml", "schema_version: \"1.0.0\"\n"),
		dir("manifest/overlay-patches/"),
		reg("manifest/overlay-patches/0001-a.patch", "diff\n"),
	)
	p, err := readPack(bytes.NewReader(data), defaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(p.File("manifest.yaml")); got != "schema_version: \"1.0.0\"\n" {
		t.Errorf("manifest.yaml = %q", got)
	}
	if p.File("overlay-patches/0001-a.patch") == nil {
		t.Error("patch missing")
	}
	if p.File("nope") != nil || p.File("../manifest/manifest.yaml") != nil {
		t.Error("File returned data for a name not in the pack")
	}
	if got := strings.Join(p.Names(), ","); got != "manifest.yaml,overlay-patches/0001-a.patch" {
		t.Errorf("Names = %s", got)
	}

	// Through the file path too.
	path := filepath.Join(t.TempDir(), "pack.tar.gz")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPack(path); err != nil {
		t.Fatalf("ReadPack: %v", err)
	}
}

func TestPack_Refusals(t *testing.T) {
	small := limits{MaxFile: 1 << 10, MaxTotal: 4 << 10, MaxEntries: 8}
	many := []tarEntry{dir("manifest/")}
	for i := 0; i < 8; i++ {
		many = append(many, reg(fmt.Sprintf("manifest/f%d", i), "x"))
	}
	cases := []struct {
		name string
		data []byte
		lim  limits
	}{
		{"absolute path", makeTarGz(t, reg("/manifest/manifest.yaml", "x")), defaultLimits},
		{"dotdot component", makeTarGz(t, reg("manifest/../etc/passwd", "x")), defaultLimits},
		{"dotdot hidden in clean", makeTarGz(t, reg("manifest/a/../../x", "x")), defaultLimits},
		{"not clean", makeTarGz(t, reg("manifest//manifest.yaml", "x")), defaultLimits},
		{"dot prefix", makeTarGz(t, reg("./manifest/manifest.yaml", "x")), defaultLimits},
		{"backslash", makeTarGz(t, reg(`manifest\..\x`, "x")), defaultLimits},
		{"symlink", makeTarGz(t, tarEntry{hdr: tar.Header{Name: "manifest/manifest.yaml", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}}), defaultLimits},
		{"hardlink", makeTarGz(t, reg("manifest/a", "x"), tarEntry{hdr: tar.Header{Name: "manifest/b", Typeflag: tar.TypeLink, Linkname: "manifest/a"}}), defaultLimits},
		{"char device", makeTarGz(t, tarEntry{hdr: tar.Header{Name: "manifest/null", Typeflag: tar.TypeChar, Devmajor: 1, Devminor: 3}}), defaultLimits},
		{"fifo", makeTarGz(t, tarEntry{hdr: tar.Header{Name: "manifest/fifo", Typeflag: tar.TypeFifo}}), defaultLimits},
		{"outside manifest", makeTarGz(t, reg("other/manifest.yaml", "x")), defaultLimits},
		{"manifest prefix trick", makeTarGz(t, reg("manifestX/manifest.yaml", "x")), defaultLimits},
		{"bare manifest file", makeTarGz(t, reg("manifest", "x")), defaultLimits},
		{"duplicate name", makeTarGz(t, reg("manifest/a", "x"), reg("manifest/a", "y")), defaultLimits},
		{"file over cap", makeTarGz(t, reg("manifest/big", strings.Repeat("x", 1<<10+1))), small},
		{"total over cap", makeTarGz(t, reg("manifest/a", strings.Repeat("x", 1<<10)), reg("manifest/b", strings.Repeat("x", 1<<10)), reg("manifest/c", strings.Repeat("x", 1<<10)), reg("manifest/d", strings.Repeat("x", 1<<10)), reg("manifest/e", "x")), small},
		{"too many entries", makeTarGz(t, many...), small},
		{"not gzip", []byte("PK\x03\x04 this is a zip"), defaultLimits},
		{"gzip of garbage", func() []byte {
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			_, _ = gz.Write([]byte("not a tar archive at all, just text that is long enough to matter............................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................................"))
			_ = gz.Close()
			return b.Bytes()
		}(), defaultLimits},
		{"truncated", makeTarGz(t, reg("manifest/a", "hello"))[:20], defaultLimits},
		{"empty input", nil, defaultLimits},
		{"empty archive", makeTarGz(t), defaultLimits},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := readPack(bytes.NewReader(tc.data), tc.lim); err == nil {
				t.Error("accepted")
			}
		})
	}
}

// bombTarGz is a valid tar.gz holding one regular file of size zero bytes,
// built as concatenated gzip members (one tiny pre-compressed MiB of zeros
// repeated), so a GiB-scale bomb costs a few hundred KiB and no compression
// time.
func bombTarGz(t testing.TB, name string, size int64) []byte {
	t.Helper()
	member := func(p []byte) []byte {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		if _, err := gz.Write(p); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	var hdr bytes.Buffer
	tw := tar.NewWriter(&hdr)
	if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: size}); err != nil {
		t.Fatal(err)
	}
	out := member(hdr.Bytes())
	mib := member(make([]byte, 1<<20))
	for i := int64(0); i < size>>20; i++ {
		out = append(out, mib...)
	}
	rem := size & (1<<20 - 1)
	pad := (512 - rem%512) % 512
	out = append(out, member(make([]byte, rem+pad+1024))...) // tail, padding, end-of-archive
	return out
}

// The real caps, at their real sizes. The data is zeros, so the compressed
// archives are small: a gzip bomb in miniature. The reader must refuse from
// the header without buffering past the cap. (The total cap is exercised with
// small limits above: tripping the real one means inflating 192+ MiB.)
func TestPack_RealCapsAndGzipBomb(t *testing.T) {
	if defaultLimits.MaxFile != 64<<20 || defaultLimits.MaxTotal != 256<<20 || defaultLimits.MaxEntries != 10000 {
		t.Fatalf("default limits changed: %+v", defaultLimits)
	}
	// Sanity: the helper makes a real archive the reader accepts when small.
	if p, err := readPack(bytes.NewReader(bombTarGz(t, "manifest/ok", 3<<20+7)), defaultLimits); err != nil || len(p.File("ok")) != 3<<20+7 {
		t.Fatalf("bomb helper archive unreadable: %v", err)
	}
	bomb := bombTarGz(t, "manifest/bomb", 1<<30)
	if len(bomb) > 4<<20 { // deflate tops out near 1000:1; this is ~500:1

		t.Fatalf("bomb fixture is %d bytes compressed; expected it to be small", len(bomb))
	}
	if _, err := readPack(bytes.NewReader(bomb), defaultLimits); !errors.Is(err, errPackTooLarge) {
		t.Errorf("1 GiB entry: %v", err)
	}
	if _, err := readPack(bytes.NewReader(bombTarGz(t, "manifest/big", 64<<20+1)), defaultLimits); !errors.Is(err, errPackTooLarge) {
		t.Errorf("64 MiB + 1: %v", err)
	}

	var lots []tarEntry
	for i := 0; i <= 10000; i++ {
		lots = append(lots, reg(fmt.Sprintf("manifest/e%05d", i), ""))
	}
	if _, err := readPack(bytes.NewReader(makeTarGz(t, lots...)), defaultLimits); err == nil {
		t.Error("10 001 entries accepted")
	}
}

func TestReadPack_PathErrors(t *testing.T) {
	if _, err := ReadPack(filepath.Join(t.TempDir(), "missing.tar.gz")); err == nil {
		t.Error("missing file accepted")
	}
	if _, err := ReadPack(t.TempDir()); err == nil {
		t.Error("directory accepted")
	}
}

func FuzzReadPack(f *testing.F) {
	f.Add(makeTarGz(f, dir("manifest/"), reg("manifest/manifest.yaml", "a: 1\n")))
	f.Add(makeTarGz(f, reg("manifest/../x", "x")))
	f.Add(makeTarGz(f, tarEntry{hdr: tar.Header{Name: "manifest/l", Typeflag: tar.TypeSymlink, Linkname: "/"}}))
	f.Add([]byte{0x1f, 0x8b, 0x08, 0x00})
	f.Add([]byte{})
	lim := limits{MaxFile: 1 << 16, MaxTotal: 1 << 18, MaxEntries: 64}
	f.Fuzz(func(t *testing.T, in []byte) {
		p, err := readPack(bytes.NewReader(in), lim)
		if err != nil {
			return
		}
		total := 0
		for _, n := range p.Names() {
			if strings.HasPrefix(n, "/") || n == "" || strings.ContainsAny(n, "\\\x00") {
				t.Fatalf("unsafe name admitted: %q", n)
			}
			for _, c := range strings.Split(n, "/") {
				if c == ".." || c == "." || c == "" {
					t.Fatalf("unsafe name admitted: %q", n)
				}
			}
			b := p.File(n)
			if int64(len(b)) > lim.MaxFile {
				t.Fatalf("%s exceeds the per-file cap", n)
			}
			total += len(b)
		}
		if int64(total) > lim.MaxTotal || len(p.Names()) > lim.MaxEntries {
			t.Fatal("caps exceeded")
		}
	})
}
