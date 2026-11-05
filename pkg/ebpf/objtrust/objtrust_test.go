// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package objtrust

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

// noLinks is readlink for a tree with no symlinks in it.
func noLinks(p string) (string, error) { return "", fs.ErrInvalid }

func TestChainError(t *testing.T) {
	good := fakeStat{
		"/":                        {0, fs.ModeDir | 0o755},
		"/usr":                     {0, fs.ModeDir | 0o755},
		"/usr/lib":                 {0, fs.ModeDir | 0o755},
		"/usr/lib/unheaded":        {0, fs.ModeDir | 0o755},
		"/usr/lib/unheaded/prog.o": {0, 0o644},
	}
	if err := chainError("/usr/lib/unheaded/prog.o", good.stat, noLinks); err != nil {
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
		if err := chainError("/usr/lib/unheaded/prog.o", f.stat, noLinks); !errors.Is(err, ErrUntrusted) {
			t.Errorf("%s: err = %v, want ErrUntrusted", name, err)
		}
	}
}

// A symlink is a pointer someone can move, so whoever could repoint it could
// choose the object. Every link on the way counts, not only where they lead.
func TestChainError_Symlinks(t *testing.T) {
	const (
		rootDir  = fs.ModeDir | 0o755
		rootLink = fs.ModeSymlink | 0o777
	)
	type entry = struct {
		uid  uint32
		mode fs.FileMode
	}
	base := func() fakeStat {
		return fakeStat{
			"/":             {0, rootDir},
			"/usr":          {0, rootDir},
			"/usr/lib":      {0, rootDir},
			"/usr/lib/prog": {0, 0o644},
			"/home":         {0, rootDir},
			"/home/u":       {1000, rootDir},
			"/tmp":          {0, fs.ModeDir | fs.ModeSticky | 0o777},
		}
	}
	for _, tc := range []struct {
		name  string
		path  string
		stat  map[string]entry
		links map[string]string
		ok    bool
	}{
		{
			name:  "root's link to root's file",
			path:  "/lib/prog",
			stat:  map[string]entry{"/lib": {0, rootLink}},
			links: map[string]string{"/lib": "usr/lib"},
			ok:    true,
		},
		{
			name:  "root's link with .. in its target",
			path:  "/usr/lib/up",
			stat:  map[string]entry{"/usr/lib/up": {0, rootLink}},
			links: map[string]string{"/usr/lib/up": "../lib/prog"},
			ok:    true,
		},
		{
			name:  "a link in a user's directory",
			path:  "/home/u/prog",
			stat:  map[string]entry{"/home/u/prog": {1000, rootLink}},
			links: map[string]string{"/home/u/prog": "/usr/lib/prog"},
		},
		{
			name:  "root's link in a user's directory",
			path:  "/home/u/prog",
			stat:  map[string]entry{"/home/u/prog": {0, rootLink}},
			links: map[string]string{"/home/u/prog": "/usr/lib/prog"},
		},
		{
			name:  "root's link in a world-writable directory",
			path:  "/tmp/prog",
			stat:  map[string]entry{"/tmp/prog": {0, rootLink}},
			links: map[string]string{"/tmp/prog": "/usr/lib/prog"},
		},
		{
			name:  "a user's link in root's directory",
			path:  "/usr/prog",
			stat:  map[string]entry{"/usr/prog": {1000, rootLink}},
			links: map[string]string{"/usr/prog": "/usr/lib/prog"},
		},
		{
			name: "a hop through a user's directory",
			path: "/usr/prog",
			stat: map[string]entry{
				"/usr/prog":   {0, rootLink},
				"/home/u/hop": {0, rootLink},
			},
			links: map[string]string{"/usr/prog": "/home/u/hop", "/home/u/hop": "/usr/lib/prog"},
		},
		{
			name:  "a directory link into a user's directory",
			path:  "/usr/d/prog",
			stat:  map[string]entry{"/usr/d": {0, rootLink}, "/home/u/prog": {0, 0o644}},
			links: map[string]string{"/usr/d": "/home/u"},
		},
		{
			name:  "a link to itself",
			path:  "/usr/loop",
			stat:  map[string]entry{"/usr/loop": {0, rootLink}},
			links: map[string]string{"/usr/loop": "/usr/loop"},
		},
		{
			name:  "a link that cannot be read",
			path:  "/usr/gone",
			stat:  map[string]entry{"/usr/gone": {0, rootLink}},
			links: map[string]string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := base()
			for p, e := range tc.stat {
				f[p] = e
			}
			readlink := func(p string) (string, error) {
				target, ok := tc.links[p]
				if !ok {
					return "", fs.ErrNotExist
				}
				return target, nil
			}
			err := chainError(tc.path, f.stat, readlink)
			if tc.ok && err != nil {
				t.Fatalf("refused: %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrUntrusted) {
				t.Fatalf("err = %v, want ErrUntrusted", err)
			}
		})
	}
}
