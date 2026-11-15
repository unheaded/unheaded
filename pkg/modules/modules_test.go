// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package modules

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x=127.0.0.1:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDir_Default(t *testing.T) {
	t.Setenv(EnvDir, "")
	if got := Dir(); got != DefaultDir {
		t.Fatalf("Dir() = %q, want %q", got, DefaultDir)
	}
	if DefaultDir != "/run/unheaded/modules" {
		t.Fatalf("DefaultDir = %q", DefaultDir)
	}
}

func TestDir_FromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	if got := Dir(); got != dir {
		t.Fatalf("Dir() = %q, want %q", got, dir)
	}
}

func TestEnabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	if Enabled("zhen") {
		t.Fatal("empty directory: zhen enabled")
	}
	if err := os.Mkdir(filepath.Join(dir, "zhen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !Enabled("zhen") {
		t.Fatal("zhen/ present: not enabled")
	}
	if err := os.Remove(filepath.Join(dir, "zhen")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "zhen"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if Enabled("zhen") {
		t.Fatal("a regular file named zhen: enabled")
	}
}

func TestEnabled_SymlinkIsNotAModule(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	real := t.TempDir()
	if err := os.Symlink(real, filepath.Join(dir, "zhen")); err != nil {
		t.Fatal(err)
	}
	if Enabled("zhen") {
		t.Fatal("a symlink to a directory: enabled")
	}
}

func TestEnabled_MissingDir(t *testing.T) {
	t.Setenv(EnvDir, filepath.Join(t.TempDir(), "absent"))
	if Enabled("zhen") {
		t.Fatal("missing modules directory: enabled")
	}
}

func TestEnabled_RefusesBadNames(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "modules")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvDir, dir)
	// A directory one level up that a traversal would find.
	if err := os.Mkdir(filepath.Join(base, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../x", "", "Zhen", "a/b", strings.Repeat("a", 33), "-x", "1x", ".", ".."} {
		if Enabled(name) {
			t.Errorf("Enabled(%q) = true", name)
		}
	}
	// "x" is a valid name, absent under dir, present one level up.
	if Enabled("x") {
		t.Error("Enabled(x) found a directory outside the modules directory")
	}
}

func TestFragments(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	a := filepath.Join(dir, "zhen", "dashboard", "services.txt")
	b := filepath.Join(dir, "other", "dashboard", "services.txt")
	writeFile(t, a)
	writeFile(t, b)
	got := Fragments("dashboard/services.txt")
	want := []string{b, a} // sorted: other < zhen
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Fragments = %v, want %v", got, want)
	}

	// A symlink in place of a file is left out.
	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}
	got = Fragments("dashboard/services.txt")
	if !reflect.DeepEqual(got, []string{a}) {
		t.Fatalf("with a symlink: Fragments = %v, want [%s]", got, a)
	}

	for _, rel := range []string{"../x", "/etc/passwd", "", ".", "a/../../x"} {
		if got := Fragments(rel); got != nil {
			t.Errorf("Fragments(%q) = %v, want nil", rel, got)
		}
	}
}

func TestFragments_SkipsBadModuleNames(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	writeFile(t, filepath.Join(dir, "Bad Name", "dashboard", "services.txt"))
	writeFile(t, filepath.Join(dir, ".zhen.new", "dashboard", "services.txt"))
	if got := Fragments("dashboard/services.txt"); got != nil {
		t.Fatalf("Fragments = %v, want nil", got)
	}
}

func TestFragments_NoDir(t *testing.T) {
	t.Setenv(EnvDir, filepath.Join(t.TempDir(), "absent"))
	if got := Fragments("dashboard/services.txt"); got != nil {
		t.Fatalf("Fragments = %v, want nil", got)
	}
}

func TestModuleOf(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvDir, dir)
	if got := ModuleOf(filepath.Join(dir, "zhen", "dashboard", "services.txt")); got != "zhen" {
		t.Fatalf("ModuleOf = %q, want zhen", got)
	}
	for _, p := range []string{"/etc/passwd", filepath.Join(dir, "zhen"), filepath.Join(dir, "..", "zhen", "x"), filepath.Join(dir, "Bad", "x"), ""} {
		if got := ModuleOf(p); got != "" {
			t.Errorf("ModuleOf(%q) = %q, want empty", p, got)
		}
	}
}
