// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package image

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// stubTool puts a program called name on PATH that records its argv, one
// argument per line, and returns the file it records to.
func stubTool(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	rec := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >>" + rec + "; done\necho /nix/store/x\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return rec
}

func recordedArgv(t *testing.T, rec string) []string {
	t.Helper()
	b, err := os.ReadFile(rec)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
}

// A target host is an operand, whatever it starts with.
func TestGenerateHardwareConfig_HostIsNeverAnOption(t *testing.T) {
	rec := stubTool(t, "ssh")
	b := NewNixOSBuilder(t.TempDir())
	const host = "-oControlPath=x"
	if _, err := b.GenerateHardwareConfig(context.Background(), host); err != nil {
		t.Fatal(err)
	}
	argv := recordedArgv(t, rec)
	dash, at := slices.Index(argv, "--"), slices.Index(argv, host)
	if dash < 0 || at != dash+1 {
		t.Fatalf("argv %q: the host does not follow --", argv)
	}
}

// So is a flake reference.
func TestNixBuild_FlakeRefIsNeverAnOption(t *testing.T) {
	rec := stubTool(t, "nix")
	b := NewNixOSBuilder(t.TempDir())
	ref := "--store=x"
	calls := map[string]func() error{
		"toplevel": func() error { _, err := b.buildSystem(context.Background(), ref, "s"); return err },
		"kexec":    func() error { _, err := b.BuildKexec(context.Background(), ref, "s"); return err },
		"netboot":  func() error { _, err := b.BuildNetboot(context.Background(), ref, "s"); return err },
	}
	for name, call := range calls {
		_ = os.Remove(rec)
		_ = call() // the stub's store path has no artifacts; argv is what counts
		argv := recordedArgv(t, rec)
		dash := slices.Index(argv, "--")
		at := slices.IndexFunc(argv, func(a string) bool { return strings.HasPrefix(a, ref) })
		if dash < 0 || at != dash+1 {
			t.Errorf("%s: argv %q: the attribute does not follow --", name, argv)
		}
	}
}

// An image id names one directory under the storage dir, so it cannot lead out
// of it or nest inside it, and nothing is created for one that would.
func TestImportImage_IDIsOneName(t *testing.T) {
	base := t.TempDir()
	storage := filepath.Join(base, "images")
	mgr, err := NewManager(&Config{StoragePath: storage, CachePath: filepath.Join(base, "cache")})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", ".", "..", "../escaped", "nixos-../../escaped-1", "a/b", "/abs"} {
		if _, err := mgr.ImportImage(context.Background(), id, "", "", ""); err == nil {
			t.Errorf("ImportImage(%q) succeeded", id)
		}
	}
	for _, dir := range []string{filepath.Join(base, "escaped"), filepath.Join(storage, "a")} {
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("%s was created", dir)
		}
	}
	if _, err := mgr.ImportImage(context.Background(), "nixos-x86_64-linux-1", "", "", ""); err != nil {
		t.Fatalf("a plain id was refused: %v", err)
	}
}
