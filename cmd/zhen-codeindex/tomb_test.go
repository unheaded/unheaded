// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const tombDir = "../../tomb"

// The Tomb VM gets its search from this binary (DP-719): nothing under tomb/
// installs, calls or configures the Python vector stack it replaced.
func TestTomb_NoVectorStackLeft(t *testing.T) {
	gone := regexp.MustCompile(`(?i)chroma|sentence.transformers|all-minilm|rag-query|rag-index|rag-deps`)
	err := filepath.WalkDir(tombDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "packages" {
			return fs.SkipDir
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if m := gone.FindString(line); m != "" {
				t.Errorf("%s:%d: %s", path, i+1, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tombDir, "grimoire", "rag")); !os.IsNotExist(err) {
		t.Errorf("tomb/grimoire/rag: %v", err)
	}
}

// Every way of provisioning the VM builds the index, and from the corpus root.
func TestTomb_EveryPathBuildsTheIndex(t *testing.T) {
	for _, f := range []string{"provision.sh", "ansible/playbook-grimoire.yml", "grimoire/setup-grimoire.sh"} {
		b, err := os.ReadFile(filepath.Join(tombDir, f))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "zhen-codeindex") || !strings.Contains(string(b), "--corpus") {
			t.Errorf("%s does not build the corpus index", f)
		}
	}
}

// grimoirePhases runs body in a copy of setup-grimoire.sh at tree/tomb/grimoire
// with its main call cut: root is the install, under a test directory.
func grimoirePhases(t *testing.T, tree, root, body string, env ...string) ([]byte, error) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(tombDir, "grimoire", "setup-grimoire.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script, ok := strings.CutSuffix(string(b), "main \"$@\"\n")
	if !ok {
		t.Fatal("setup-grimoire.sh no longer ends in its main call")
	}
	dir := filepath.Join(tree, "tomb", "grimoire")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(dir, "setup-grimoire.sh")
	if err := os.WriteFile(copied, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	const setup = `source "$1"; GRIMOIRE_ROOT="$2"; CODEINDEX="$2/bin/zhen-codeindex"; LOG_FILE=/dev/null; `
	cmd := exec.Command("bash", "-c", setup+body, "phases", copied, root)
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}

func stubBinary(t *testing.T, file string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// An install made before this binary existed has no bin/, and phase 1 leaves an
// existing root alone: the upgrade still has to land the binary.
func TestTomb_SetupInstallsOverAnOldGrimoire(t *testing.T) {
	tree, root := t.TempDir(), filepath.Join(t.TempDir(), "grimoire")
	if err := os.MkdirAll(filepath.Join(root, "kingdom-docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubBinary(t, filepath.Join(tree, "tomb", "grimoire", "bin", "zhen-codeindex"))
	if out, err := grimoirePhases(t, tree, root, "create_directory_structure; install_codeindex"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "bin", "zhen-codeindex")); err != nil {
		t.Fatal(err)
	}
}

// What make stage-grimoire builds is what the script finds, run from tomb/grimoire.
func TestTomb_SetupFindsTheStagedBinary(t *testing.T) {
	mk, err := exec.LookPath("make")
	if err != nil {
		t.Skip("no make")
	}
	dry, err := exec.Command(mk, "-n", "-C", tombDir, "stage-grimoire").CombinedOutput()
	if err != nil {
		t.Fatalf("make -n: %v: %s", err, dry)
	}
	const staged = "packages/grimoire/bin/zhen-codeindex"
	if !strings.Contains(string(dry), "/"+staged+" ") {
		t.Fatalf("stage-grimoire no longer builds %s: %s", staged, dry)
	}
	tree, root := t.TempDir(), filepath.Join(t.TempDir(), "grimoire")
	stubBinary(t, filepath.Join(tree, "tomb", filepath.FromSlash(staged)))
	if out, err := grimoirePhases(t, tree, root, "install_codeindex"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "bin", "zhen-codeindex")); err != nil {
		t.Fatal(err)
	}
}

// A binary named by hand is the one meant: a wrong name is an error, not a
// quiet fall back to whatever is staged.
func TestTomb_SetupRefusesAMissingNamedBinary(t *testing.T) {
	tree, root := t.TempDir(), filepath.Join(t.TempDir(), "grimoire")
	stubBinary(t, filepath.Join(tree, "tomb", "packages", "grimoire", "bin", "zhen-codeindex"))
	out, err := grimoirePhases(t, tree, root, "install_codeindex", "ZHEN_CODEINDEX="+filepath.Join(tree, "none"))
	if err == nil || !strings.Contains(string(out), "not found") {
		t.Fatalf("%v: %s", err, out)
	}
}

// ATT&CK is staged as its bundles. The whole repository would index each
// technique twice and bring its third-party prose along.
func TestTomb_AttackDataComesFromTheFetchScript(t *testing.T) {
	for _, f := range []string{"Makefile", "provision.sh"} {
		b, err := os.ReadFile(filepath.Join(tombDir, f))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "mitre/cti") {
			t.Errorf("%s points at the whole mitre/cti repository", f)
		}
		if !strings.Contains(string(b), "fetch-mitre-data.sh") {
			t.Errorf("%s does not name fetch-mitre-data.sh", f)
		}
	}
}

// ragCerydwyn is the Python script provision.sh writes onto the VM, undone
// from the double-quoted string it travels in.
func ragCerydwyn(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(tombDir, "provision.sh"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(b), "<< 'RAG_CERYDWYN_EOF'\n")
	if !ok {
		t.Fatal("provision.sh no longer writes rag-cerydwyn.py")
	}
	script, _, ok := strings.Cut(rest, "\nRAG_CERYDWYN_EOF")
	if !ok {
		t.Fatal("no end to rag-cerydwyn.py")
	}
	return strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\$`, `$`).Replace(script)
}

// Cerydwyn's pipeline and this binary meet at the JSON: run the real script
// against the real binary, so a renamed key cannot quietly empty its context.
func TestTomb_CerydwynReadsTheSearchOutput(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "zhen-codeindex")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v %s", err, out)
	}
	index := filepath.Join(dir, "index")
	if err := runCorpus(context.Background(), keptCorpus, index, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "rag_cerydwyn.py")
	if err := os.WriteFile(script, []byte(ragCerydwyn(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := json.Marshal(map[string]any{"grimoire_search": bin, "grimoire_index": index, "top_k": 2})
	if err != nil {
		t.Fatal(err)
	}
	const driver = `
import importlib.util, json, sys
spec = importlib.util.spec_from_file_location("rag_cerydwyn", sys.argv[1])
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
cfg = json.loads(sys.argv[2])
docs = m.retrieve_context("Rogue BPF Program", cfg)
print(m.build_prompt("q", docs, cfg))
print("MISSING:" + str(m.retrieve_context("x", {**cfg, "grimoire_index": sys.argv[3]})))
`
	out, err := exec.Command(py, "-I", "-c", driver, script, string(cfg), filepath.Join(dir, "none")).CombinedOutput()
	if err != nil {
		t.Fatalf("python: %v %s", err, out)
	}
	for _, want := range []string{"[Source: threat-models/kingdom-threat-model.md]", "(relevance: 1.00)", "Rogue BPF Program", "MISSING:[]"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("no %q in %s", want, out)
		}
	}
}
