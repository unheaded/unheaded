// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The Champion runbook eval (eval/champion-runbooks): 100 tasks, 20 per
// family, run by this binary with -tasks/-keys/-prompts pointed at it.
const championDir = "eval/champion-runbooks"

func championOptions() *options {
	return &options{root: repoRoot, tasksPath: championDir + "/split.jsonl", keysPath: championDir + "/keys.jsonl",
		promptsPath: championDir + "/prompts.jsonl", split: "all"}
}

func TestChampionRunbooks_LoadsAllHundred(t *testing.T) {
	tasks, keys, prompts, err := loadAll(championOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 100 || len(keys) != 80 || len(prompts) != 80 {
		t.Fatalf("tasks %d keys %d prompts %d", len(tasks), len(keys), len(prompts))
	}
	fam := map[string]int{}
	for _, tk := range tasks {
		fam[tk.Family]++
		if tk.Split != "tune" && tk.Split != "holdout" {
			t.Errorf("%s: split %q", tk.ID, tk.Split)
		}
	}
	for _, f := range []string{"rb-select", "rb-read", "rb-safety", "rb-author", "workflow"} {
		if fam[f] != 20 {
			t.Errorf("family %s has %d tasks, want 20", f, fam[f])
		}
	}
}

// Every key passes its reference answers and fails its wrong ones (and an
// empty answer), so a key that matches anything or nothing is caught here
// rather than in a result.
func TestChampionRunbooks_KeysAgainstReferences(t *testing.T) {
	_, keys, _, err := loadAll(championOptions())
	if err != nil {
		t.Fatal(err)
	}
	type ref struct {
		ID   string   `json:"id"`
		Good []string `json:"good"`
		Bad  []string `json:"bad"`
	}
	refs, err := loadJSONL[ref](filepath.Join(repoRoot, championDir, "refs.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != len(keys) {
		t.Fatalf("refs %d keys %d", len(refs), len(keys))
	}
	for _, r := range refs {
		k := keys[r.ID]
		if k == nil {
			t.Errorf("%s: no key", r.ID)
			continue
		}
		for _, a := range r.Good {
			if ok, d := judgeKey(k, a); !ok {
				t.Errorf("%s: reference %q failed: %s", r.ID, a, d)
			}
		}
		for _, a := range r.Bad {
			if ok, _ := judgeKey(k, a); ok {
				t.Errorf("%s: wrong answer %q passed", r.ID, a)
			}
		}
	}
}

// Every authoring fixture fails its check as shipped and passes with its
// solution.patch applied. Needs bash, patch and python3 with PyYAML.
func TestChampionRunbooks_FixturesFailThenPass(t *testing.T) {
	for _, tool := range []string{"bash", "patch", "python3"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	if exec.Command("python3", "-c", "import yaml").Run() != nil {
		t.Skip("PyYAML not installed")
	}
	tasks, _, _, err := loadAll(championOptions())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, tk := range tasks {
		if tk.Judge != "check" {
			continue
		}
		n++
		src := filepath.Join(repoRoot, tk.Ref)
		dir := filepath.Join(t.TempDir(), tk.ID)
		if out, err := exec.Command("cp", "-r", src, dir).CombinedOutput(); err != nil {
			t.Fatalf("%s: copy: %v %s", tk.ID, err, out)
		}
		if out, err := exec.Command("bash", filepath.Join(dir, "check.sh")).CombinedOutput(); err == nil {
			t.Errorf("%s: check passes before any fix: %s", tk.ID, out)
		}
		p := exec.Command("patch", "-s", "-p1")
		p.Dir = dir
		patch, err := os.Open(filepath.Join(dir, "solution.patch")) // #nosec G304 -- the repo's own fixture
		if err != nil {
			t.Fatal(err)
		}
		p.Stdin = patch
		out, err := p.CombinedOutput()
		_ = patch.Close()
		if err != nil {
			t.Fatalf("%s: patch: %v %s", tk.ID, err, out)
		}
		if out, err := exec.Command("bash", filepath.Join(dir, "check.sh")).CombinedOutput(); err != nil {
			t.Errorf("%s: check fails with the solution: %s", tk.ID, strings.TrimSpace(string(out)))
		}
	}
	if n != 20 {
		t.Errorf("%d check fixtures, want 20", n)
	}
}
