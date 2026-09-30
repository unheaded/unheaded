// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ebpf

import (
	"errors"
	"io/fs"
	"testing"
)

type fakeStat map[string]struct {
	uid  uint32
	mode fs.FileMode
}

func (f fakeStat) stat(p string) (uint32, fs.FileMode, error) {
	e, ok := f[p]
	if !ok {
		return 0, 0, fs.ErrNotExist
	}
	return e.uid, e.mode, nil
}

func TestObjectTrustError(t *testing.T) {
	good := fakeStat{
		"/":                        {0, fs.ModeDir | 0o755},
		"/usr":                     {0, fs.ModeDir | 0o755},
		"/usr/lib":                 {0, fs.ModeDir | 0o755},
		"/usr/lib/unheaded":        {0, fs.ModeDir | 0o755},
		"/usr/lib/unheaded/prog.o": {0, 0o644},
	}
	if err := objectTrustError("/usr/lib/unheaded/prog.o", good.stat); err != nil {
		t.Fatalf("root-owned, non-writable chain refused: %v", err)
	}

	for name, mut := range map[string]func(fakeStat){
		"file owned by a user": func(f fakeStat) {
			f["/usr/lib/unheaded/prog.o"] = struct {
				uid  uint32
				mode fs.FileMode
			}{1000, 0o644}
		},
		"file group-writable": func(f fakeStat) {
			f["/usr/lib/unheaded/prog.o"] = struct {
				uid  uint32
				mode fs.FileMode
			}{0, 0o664}
		},
		"parent dir owned by a user": func(f fakeStat) {
			f["/usr/lib/unheaded"] = struct {
				uid  uint32
				mode fs.FileMode
			}{1000, fs.ModeDir | 0o755}
		},
		"grandparent world-writable": func(f fakeStat) {
			f["/usr/lib"] = struct {
				uid  uint32
				mode fs.FileMode
			}{0, fs.ModeDir | 0o777}
		},
		"sticky world-writable (tmp)": func(f fakeStat) {
			f["/usr/lib"] = struct {
				uid  uint32
				mode fs.FileMode
			}{0, fs.ModeDir | fs.ModeSticky | 0o777}
		},
		"root dir writable": func(f fakeStat) {
			f["/"] = struct {
				uid  uint32
				mode fs.FileMode
			}{0, fs.ModeDir | 0o775}
		},
		"stat fails": func(f fakeStat) { delete(f, "/usr") },
	} {
		f := fakeStat{}
		for k, v := range good {
			f[k] = v
		}
		mut(f)
		if err := objectTrustError("/usr/lib/unheaded/prog.o", f.stat); !errors.Is(err, ErrUntrustedObject) {
			t.Errorf("%s: err = %v, want ErrUntrustedObject", name, err)
		}
	}
}
