// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package hoprules

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// oneWrite records what SetMode wrote.
type oneWrite struct {
	writes [][3]string
	fail   error
}

func (w *oneWrite) Update(mapName string, key, value []byte) error {
	if w.fail != nil {
		return w.fail
	}
	w.writes = append(w.writes, [3]string{mapName, string(key), string(value)})
	return nil
}

func TestSetMode_RefusesEnforceAndWritesNothing(t *testing.T) {
	w := &oneWrite{}
	err := SetMode(w, ModeEnforce)
	if !errors.Is(err, ErrEnforceRefused) {
		t.Fatalf("err = %v, want ErrEnforceRefused", err)
	}
	for _, id := range EnforceBlockers {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("refusal does not name %s: %v", id, err)
		}
	}
	if len(w.writes) != 0 {
		t.Errorf("a refusal wrote %v", w.writes)
	}
}

// off and observe are what CONFIG key 4 takes today, as hop-ebpf reads it: a
// u32 key and a u64 value, little-endian.
func TestSetMode_WritesOffAndObserve(t *testing.T) {
	for _, m := range []CheckMode{ModeOff, ModeObserve} {
		w := &oneWrite{}
		if err := SetMode(w, m); err != nil {
			t.Fatalf("mode %d: %v", m, err)
		}
		if len(w.writes) != 1 || w.writes[0][0] != "CONFIG" {
			t.Fatalf("mode %d: writes %v", m, w.writes)
		}
		if k := []byte(w.writes[0][1]); len(k) != 4 || binary.LittleEndian.Uint32(k) != 4 {
			t.Errorf("mode %d: key %x", m, k)
		}
		if v := []byte(w.writes[0][2]); len(v) != 8 || binary.LittleEndian.Uint64(v) != uint64(m) {
			t.Errorf("mode %d: value %x", m, v)
		}
	}
}

// With no blocker left, enforce writes 2: the refusal is the list, nothing else.
func TestSetMode_EnforceOnceNothingBlocks(t *testing.T) {
	w := &oneWrite{}
	if err := setMode(w, ModeEnforce, nil); err != nil {
		t.Fatal(err)
	}
	if v := []byte(w.writes[0][2]); binary.LittleEndian.Uint64(v) != 2 {
		t.Errorf("value %x", v)
	}
}

func TestSetMode_MapFailureIsReported(t *testing.T) {
	boom := errors.New("boom")
	if err := SetMode(&oneWrite{fail: boom}, ModeObserve); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseMode(t *testing.T) {
	for s, want := range map[string]CheckMode{"off": ModeOff, "observe": ModeObserve, "enforce": ModeEnforce} {
		if got, err := ParseMode(s); err != nil || got != want {
			t.Errorf("%s: %d, %v", s, got, err)
		}
	}
	for _, s := range []string{"", "2", "ENFORCE", "drop"} {
		if _, err := ParseMode(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

// The list follows the register: a finding that closes leaves it in the same
// change, and none leaves it while it is open.
func TestEnforceBlockers_FollowTheRegister(t *testing.T) {
	raw, err := os.ReadFile("../../compliance/findings/register.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Findings []struct {
			ID       string         `yaml:"id"`
			Resolved map[string]any `yaml:"resolved"`
		} `yaml:"findings"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	open := map[string]bool{}
	for _, f := range doc.Findings {
		if f.ID != "" && f.Resolved == nil {
			open[f.ID] = true
		}
	}
	for _, id := range []string{"FND-031", "FND-033", "FND-035"} {
		if listed := slices.Contains(EnforceBlockers, id); listed != open[id] {
			t.Errorf("%s: in EnforceBlockers %v, open in the register %v", id, listed, open[id])
		}
	}
}

// A hop CONFIG write outside this package names the map in code, or its pin
// path in anything a person or a unit would run.
var (
	configInCode = regexp.MustCompile(`"CONFIG"`)
	configPin    = regexp.MustCompile(`hop-ebpf/CONFIG`)
)

// configWritersUnder lists the files under root naming the CONFIG map, leaving
// out tests, this package and tests/hopchecks, which proves enforce in the lab.
func configWritersUnder(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "target", "node_modules", "hoprules", "hopchecks", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if !slices.Contains([]string{".go", ".rs", ".sh", ".py", ".yaml", ".yml", ".service"}, ext) ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if configPin.Match(raw) || ((ext == ".go" || ext == ".rs") && configInCode.Match(raw)) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// SetMode stays the one writer of key 4. The packaged hop_xdp refuses enforce
// on its own since 2026-11-09, but a hop-enforce build honours whatever the key
// says. A second writer in the tree fails here, whether it is code, a script or
// a unit. Root writing the pin by hand meets the object's own refusal
// (tests/hopchecks TestDefaultObjectRefusesEnforce), as long as packages ship the
// default build (TestHopEnforce_OnlyTheTestPath).
func TestConfigMap_OneWriter(t *testing.T) {
	for _, dir := range []string{"cmd", "pkg", "scripts", "deploy", "runbooks", "debian", "tests"} {
		for _, path := range configWritersUnder(t, filepath.Join("..", "..", dir)) {
			t.Errorf("%s names hop-ebpf's CONFIG map: key 4 goes through SetMode", path)
		}
	}
}

// The walk above must see a writer when there is one.
func TestConfigMap_OneWriterSeesASecondWriter(t *testing.T) {
	dir := t.TempDir()
	sample := "package x\n\nconst name = \"CONFIG\"\n"
	if err := os.WriteFile(filepath.Join(dir, "second.go"), []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pin.sh"), []byte("x /sys/fs/bpf/hop-ebpf/CONFIG\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := configWritersUnder(t, dir); len(got) != 2 {
		t.Fatalf("found %v, want both files", got)
	}
}

// hopEnforceAllowed are the only files that may build hop-ebpf with the
// hop-enforce feature: the load gate, which builds the lab object, and the
// root test runner that hands it to tests/hopchecks. Every packaged object is
// the default build, which refuses enforce (ADR-125, 2026-11-09).
var hopEnforceAllowed = []string{
	"scripts/check-ebpf-loads.sh",
	"scripts/test-go-root.sh",
}

// hopEnforceBuildsUnder lists the build files under root, relative to base,
// that name the hop-enforce feature.
func hopEnforceBuildsUnder(t *testing.T, base, root string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(filepath.Join(base, root), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "target", "node_modules", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !slices.Contains([]string{".sh", ".yml", ".yaml", ".mk", ".nix", ".service"}, filepath.Ext(name)) &&
			name != "Makefile" && name != "rules" && name != "Dockerfile" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), "hop-enforce") {
			rel, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// Packages, releases and CI ship the refusing object: only the test path
// builds the one that drops.
func TestHopEnforce_OnlyTheTestPath(t *testing.T) {
	base := filepath.Join("..", "..")
	var found []string
	for _, dir := range []string{"scripts", ".github", "debian", "deploy", "docker", "nix"} {
		if _, err := os.Stat(filepath.Join(base, dir)); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		found = append(found, hopEnforceBuildsUnder(t, base, dir)...)
	}
	if raw, err := os.ReadFile(filepath.Join(base, "Makefile")); err != nil {
		t.Fatal(err)
	} else if strings.Contains(string(raw), "hop-enforce") {
		found = append(found, "Makefile")
	}
	for _, path := range found {
		if !slices.Contains(hopEnforceAllowed, path) {
			t.Errorf("%s names hop-enforce: only %v may build the object that drops", path, hopEnforceAllowed)
		}
	}
	for _, path := range hopEnforceAllowed {
		if !slices.Contains(found, path) {
			t.Errorf("%s no longer names hop-enforce: tests/hopchecks' enforce rows lose their object", path)
		}
	}
}

// The walk above must see a build file that names the feature.
func TestHopEnforce_WalkSeesABuild(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "scripts"), 0o700); err != nil {
		t.Fatal(err)
	}
	sample := "cargo build -p hop-ebpf --features hop-enforce\n"
	if err := os.WriteFile(filepath.Join(base, "scripts", "build.sh"), []byte(sample), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := hopEnforceBuildsUnder(t, base, "scripts"); !slices.Equal(got, []string{"scripts/build.sh"}) {
		t.Fatalf("found %v, want scripts/build.sh", got)
	}
}
