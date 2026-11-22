// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package hoprules

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const oneValidator = "validators:\n  - hop: 1\n"

func rulesFile(t *testing.T, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "rules.yaml")
	if err := os.WriteFile(p, []byte(oneValidator), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

// A rules file names the key files and decides what an enforcing hop drops, so
// one that a group or everyone can write is refused before it is parsed.
func TestParseFile_Modes(t *testing.T) {
	cases := []struct {
		mode os.FileMode
		ok   bool
	}{
		{0o600, true},
		{0o400, true},
		{0o640, true},
		{0o644, true},
		{0o444, true},
		{0o660, false},
		{0o664, false},
		{0o646, false},
		{0o602, false},
		{0o666, false},
	}
	for _, c := range cases {
		t.Run(c.mode.String(), func(t *testing.T) {
			p := rulesFile(t, c.mode)
			f, err := ParseFile(p)
			if c.ok {
				if err != nil {
					t.Fatal(err)
				}
				if len(f.Validators) != 1 || f.Validators[0].Hop != 1 {
					t.Errorf("parsed %+v", f)
				}
				return
			}
			if !errors.Is(err, ErrRulesFileMode) {
				t.Fatalf("err = %v, want ErrRulesFileMode", err)
			}
			for _, s := range []string{p, "group- or world-writable"} {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("refusal %q does not say %q", err, s)
				}
			}
		})
	}
}

func TestParseFile_RefusesWhatIsNotARegularFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := ParseFile(dir); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory: err = %v", err)
	}
	if _, err := ParseFile(filepath.Join(dir, "gone.yaml")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: err = %v, want fs.ErrNotExist", err)
	}
}

// A parse error still names the file.
func TestParseFile_NamesTheFileInAParseError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rules.yaml")
	if err := os.WriteFile(p, []byte("rules: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFile(p); err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("err = %v, want the path", err)
	}
}
