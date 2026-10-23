// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// A bounded, in-memory evidence-pack reader. A pack arrives from outside
// (a build runner, a download, an auditor's archive), so it is hostile until
// verified: nothing is extracted to disk, only regular files and directories
// are admitted, every name must be a clean path under manifest/, and per-file,
// total and entry-count caps trip from the tar headers before data is read, so
// a gzip bomb costs at most the caps in memory.

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// packRoot is the tarball's only top-level directory (ADR-105 decision 7).
const packRoot = "manifest/"

type limits struct {
	MaxFile    int64 // bytes per regular file
	MaxTotal   int64 // bytes across all regular files
	MaxEntries int   // tar entries of any type
}

var defaultLimits = limits{MaxFile: 64 << 20, MaxTotal: 256 << 20, MaxEntries: 10000}

var (
	errPackTooLarge = errors.New("pack exceeds a size cap")
	errPackEntry    = errors.New("pack entry refused")
	errPackFormat   = errors.New("not a gzip-compressed tar evidence pack")
)

// Pack is the regular files of an evidence pack, keyed by their path below
// manifest/ (e.g. "manifest.yaml", "overlay-patches/0001-x.patch").
type Pack struct {
	files map[string][]byte
}

// File returns a file's bytes, or nil if the pack has no such file.
func (p *Pack) File(name string) []byte {
	if p == nil {
		return nil
	}
	return p.files[name]
}

// Has reports whether the pack contains name (even if it is empty).
func (p *Pack) Has(name string) bool {
	if p == nil {
		return false
	}
	_, ok := p.files[name]
	return ok
}

// Names lists the pack's files, sorted.
func (p *Pack) Names() []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.files))
	for n := range p.files {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ReadPack reads an evidence-pack tarball with the default caps.
func ReadPack(file string) (*Pack, error) {
	f, err := os.Open(file) // #nosec G304 -- operator-named pack; contents are validated below
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", file)
	}
	return readPack(f, defaultLimits)
}

func readPack(r io.Reader, lim limits) (*Pack, error) {
	br := bufio.NewReader(r)
	if magic, err := br.Peek(2); err != nil || magic[0] != 0x1f || magic[1] != 0x8b {
		return nil, errPackFormat
	}
	gz, err := gzip.NewReader(br)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errPackFormat, err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	p := &Pack{files: map[string][]byte{}}
	seen := map[string]bool{}
	var total int64
	entries := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errPackFormat, err)
		}
		entries++
		if entries > lim.MaxEntries {
			return nil, fmt.Errorf("%w: more than %d entries", errPackTooLarge, lim.MaxEntries)
		}
		isDir := hdr.Typeflag == tar.TypeDir
		name, err := packName(hdr.Name, isDir)
		if err != nil {
			return nil, err
		}
		if seen[name] {
			return nil, fmt.Errorf("%w: duplicate entry %q", errPackEntry, hdr.Name)
		}
		seen[name] = true
		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg, tar.TypeRegA: //nolint:staticcheck // TypeRegA: old archives still write it
		default:
			return nil, fmt.Errorf("%w: %q is not a regular file or directory (type %q)", errPackEntry, hdr.Name, hdr.Typeflag)
		}
		if name == "" {
			return nil, fmt.Errorf("%w: %q is the pack root, not a file", errPackEntry, hdr.Name)
		}
		if hdr.Size < 0 || hdr.Size > lim.MaxFile {
			return nil, fmt.Errorf("%w: %q is %d bytes, per-file cap %d", errPackTooLarge, hdr.Name, hdr.Size, lim.MaxFile)
		}
		if total+hdr.Size > lim.MaxTotal {
			return nil, fmt.Errorf("%w: total exceeds %d bytes at %q", errPackTooLarge, lim.MaxTotal, hdr.Name)
		}
		b, err := io.ReadAll(io.LimitReader(tr, hdr.Size+1))
		if err != nil {
			return nil, fmt.Errorf("%w: %q: %v", errPackFormat, hdr.Name, err)
		}
		if int64(len(b)) != hdr.Size {
			return nil, fmt.Errorf("%w: %q: read %d bytes, header says %d", errPackFormat, hdr.Name, len(b), hdr.Size)
		}
		total += hdr.Size
		p.files[name] = b
	}
	if len(p.files) == 0 {
		return nil, fmt.Errorf("%w: no files under %s", errPackFormat, packRoot)
	}
	return p, nil
}

// packName checks a tar entry name and returns its path below manifest/ ("" for
// the manifest/ directory itself). The name must already be clean: no leading
// "/" or "./", no "." or ".." components, no "//", no backslash or NUL.
func packName(raw string, isDir bool) (string, error) {
	bad := func(why string) (string, error) {
		return "", fmt.Errorf("%w: %q: %s", errPackEntry, raw, why)
	}
	if strings.ContainsAny(raw, "\\\x00") {
		return bad("backslash or NUL in name")
	}
	name := raw
	if isDir {
		name = strings.TrimSuffix(name, "/")
	}
	if name == strings.TrimSuffix(packRoot, "/") && isDir {
		return "", nil
	}
	if path.IsAbs(name) {
		return bad("absolute path")
	}
	for _, c := range strings.Split(name, "/") {
		if c == ".." {
			return bad("'..' component")
		}
	}
	if path.Clean(name) != name {
		return bad("not a clean path")
	}
	if !strings.HasPrefix(name, packRoot) {
		return bad("outside " + packRoot)
	}
	return strings.TrimPrefix(name, packRoot), nil
}
