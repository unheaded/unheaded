// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package well

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/compliance/remediation"
)

// wellFake is a Store that says it is The Well, backed by memory.
type wellFake struct {
	m  *remediation.Model
	ps *remediation.ProfileSet
	bs []remediation.Binding
}

func (f *wellFake) Source() Source                                            { return SourceWell }
func (f *wellFake) Model() *remediation.Model                                 { return f.m }
func (f *wellFake) Profiles(context.Context) (*remediation.ProfileSet, error) { return f.ps, nil }
func (f *wellFake) Bindings(context.Context) ([]remediation.Binding, error)   { return f.bs, nil }
func (f *wellFake) SetBinding(context.Context, Change) error                  { return nil }

// copyCompliance copies the repo's compliance/ tree into a temp repo.
func copyCompliance(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	src := filepath.Join(repo, "compliance")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(repo, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestExportWritesTheWellsStateKeepingHistory(t *testing.T) {
	dir := copyCompliance(t)
	y, err := NewYAMLStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ps, _ := y.Profiles(context.Background())
	bs, _ := y.Bindings(context.Background())
	for i := range bs {
		if bs[i].Host == "east" {
			bs[i].Profile = "none"
		}
	}
	f := &wellFake{m: y.Model(), ps: ps, bs: bs}
	changed, err := Export(context.Background(), f, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !containsStr(changed, "compliance/hosts/east.yaml") || containsStr(changed, "compliance/hosts/west.yaml") {
		t.Errorf("changed %v, want east only (and profiles.yaml reformatted)", changed)
	}
	east, _ := os.ReadFile(filepath.Join(dir, "compliance/hosts/east.yaml"))
	if !strings.Contains(string(east), "\nprofile: none\n") || !strings.Contains(string(east), "fix: sudo-cis") ||
		!strings.HasPrefix(string(east), "# Host binding") {
		t.Errorf("east.yaml lost its comments or applied history, or kept the old profile:\n%s", east)
	}
	// The exported fallback reads back as the same state, and a second
	// export changes nothing.
	y2, err := NewYAMLStore(dir)
	if err != nil {
		t.Fatalf("exported files do not load: %v", err)
	}
	ps2, _ := y2.Profiles(context.Background())
	for _, p := range ps.Profiles {
		if strings.Join(ps2.Resolve(p.Name), ",") != strings.Join(ps.Resolve(p.Name), ",") {
			t.Errorf("profile %s changed in export", p.Name)
		}
	}
	if again, err := Export(context.Background(), f, dir); err != nil || len(again) != 0 {
		t.Errorf("second export changed %v (%v)", again, err)
	}
}

func TestExportRefusesTheFallbackItself(t *testing.T) {
	y, err := NewYAMLStore(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Export(context.Background(), y, t.TempDir()); err == nil {
		t.Fatal("exported the fallback onto itself")
	}
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
