// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const keptCorpus = "../../tomb/grimoire"

func searchResults(t *testing.T, dir, query string, k int) searchOutput {
	t.Helper()
	var buf bytes.Buffer
	if err := runSearch(context.Background(), dir, query, k, &buf); err != nil {
		t.Fatal(err)
	}
	var got searchOutput
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	return got
}

func TestCorpusThenSearch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "corpus-index")
	var buf bytes.Buffer
	if err := runCorpus(context.Background(), keptCorpus, dir, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "chunks=") {
		t.Fatalf("no summary: %s", buf.String())
	}
	got := searchResults(t, dir, "Rogue BPF Program", 2)
	if got.Query != "Rogue BPF Program" || len(got.Results) == 0 || len(got.Results) > 2 {
		t.Fatalf("%+v", got)
	}
	first := got.Results[0]
	if first.Source != "threat-models/kingdom-threat-model.md" || first.Trust != "local" || first.Score != 1 ||
		!strings.Contains(first.Content, "Rogue BPF") {
		t.Fatalf("%+v", first)
	}
}

func TestSearch_NoIndexIsAnError(t *testing.T) {
	err := runSearch(context.Background(), filepath.Join(t.TempDir(), "none"), "x", 5, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--corpus") {
		t.Fatalf("err %v", err)
	}
}

func TestSearch_NothingFoundIsAnEmptyList(t *testing.T) {
	dir := t.TempDir()
	if err := runCorpus(context.Background(), keptCorpus, dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runSearch(context.Background(), dir, "zzzqqqzzz", 5, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"results":[]`) {
		t.Fatalf("%s", buf.String())
	}
}

// The corpus has its own index: without this a --corpus run with the default
// --out would replace the source shard the agent reads.
func TestCorpusModesNeedTheirOwnIndex(t *testing.T) {
	for _, tc := range []struct {
		corpus, search, out string
		ok                  bool
	}{
		{"", "", defaultOut, true},
		{"", "", "/var/tmp/x", true},
		{"tomb/grimoire", "", defaultOut, false},
		{"", "term", defaultOut, false},
		{"tomb/grimoire", "", defaultOut + "/", false},
		{"tomb/grimoire", "", "/var/zhen/corpus-index", true},
		{"", "term", "/var/zhen/corpus-index", true},
		{"tomb/grimoire", "term", "/var/zhen/corpus-index", false},
	} {
		if err := checkModes(tc.corpus, tc.search, tc.out); (err == nil) != tc.ok {
			t.Errorf("%+v: %v", tc, err)
		}
	}
}

func TestRepoRefreshStillBuildsTheShard(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.email=a@b", "-c", "user.name=a", "commit", "-q", "--no-gpg-sign", "-m", "x"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, a...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	var buf bytes.Buffer
	if err := run(context.Background(), repo, filepath.Join(t.TempDir(), "o"), &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "files=0") {
		t.Fatalf("%s", buf.String())
	}
}

// Run in a network namespace with nothing in it. The helper first shows that
// no address can be reached, so a pass means the build and the search ran with
// no network and not merely that nothing tried it.
func TestCorpusAndSearch_NoNetwork(t *testing.T) {
	if os.Getenv("ZHEN_CODEINDEX_NETLESS") == "1" {
		netlessBody(t)
		return
	}
	if err := exec.Command("unshare", "-rn", "true").Run(); err != nil {
		t.Skipf("no unprivileged network namespace here: %v", err)
	}
	cmd := exec.Command("unshare", "-rn", os.Args[0], "-test.run=^TestCorpusAndSearch_NoNetwork$", "-test.v")
	cmd.Env = append(os.Environ(), "ZHEN_CODEINDEX_NETLESS=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(string(out), "netless ok") {
		t.Fatalf("the helper did not run: %s", out)
	}
}

func netlessBody(t *testing.T) {
	d := net.Dialer{Timeout: time.Second}
	if c, err := d.DialContext(context.Background(), "tcp", "1.1.1.1:53"); err == nil {
		_ = c.Close()
		t.Fatal("this namespace has a route out")
	}
	dir := t.TempDir()
	if err := runCorpus(context.Background(), keptCorpus, dir, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got := searchResults(t, dir, "TA0001", 1)
	if len(got.Results) != 1 || got.Results[0].Source != "mitre-attck/mitre-readme.md" {
		t.Fatalf("%+v", got)
	}
	t.Log("netless ok")
}
