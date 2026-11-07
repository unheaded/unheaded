// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package retrieval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/codeindex"
)

// buildCorpus indexes root the way zhen-codeindex does and returns the backend.
func buildCorpus(t *testing.T, root string) *CorpusBackend {
	t.Helper()
	out := t.TempDir()
	if _, err := codeindex.BuildCorpus(context.Background(), root, out); err != nil {
		t.Fatal(err)
	}
	return NewCorpusBackend(codeindex.NewStore(out))
}

// The corpus kept in this repository answers through the same Federated the
// agent asks, from files alone: the backend has no client to reach anything with.
func TestCorpus_KnownTermsFindTheirFiles(t *testing.T) {
	f := New([]Backend{buildCorpus(t, filepath.Join("..", "..", "tomb", "grimoire"))}, time.Second, nil)
	for query, want := range map[string]string{
		"Rogue BPF Program": "threat-models/kingdom-threat-model.md",
		"exponent-encoded":  "threat-models/monad-protocol-threats.md",
		"TA0001":            "mitre-attck/mitre-readme.md",
	} {
		refs, contents, err := f.Retrieve(context.Background(), query, 3)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if len(refs) == 0 || len(contents) == 0 || refs[0].SourcePath != want {
			t.Errorf("%s: want %s first, got %+v", query, want, refs)
			continue
		}
		if !strings.Contains(strings.ToLower(contents[0].Content), strings.ToLower(strings.Fields(query)[0])) {
			t.Errorf("%s: the text served does not hold it: %q", query, contents[0].Content)
		}
	}
}

func writeCorpusFile(t *testing.T, root, file, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stixCorpus(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "codeindex", "testdata", "attack-sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "mitre-attck"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "mitre-attck", "enterprise-attack.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCorpus_TechniqueIDFindsItsObject(t *testing.T) {
	refs, contents, err := buildCorpus(t, stixCorpus(t)).Retrieve(context.Background(), "T1566.001", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 1 || len(refs) == 0 || refs[0].SourcePath != "mitre-attck/enterprise-attack.json" {
		t.Fatalf("refs %+v, %d texts", refs, len(contents))
	}
	if !strings.Contains(contents[0].Content, "T1566.001 - Spearphishing Attachment") {
		t.Fatalf("content %q", contents[0].Content)
	}
}

// Text from outside the project never justifies a write. What the project
// wrote carries the trust of the source shard, and passes the chain check.
func TestCorpus_Trust(t *testing.T) {
	root := stixCorpus(t)
	writeCorpusFile(t, root, "threat-models/own.md", "# Own\nquartzmoth\n")
	b := buildCorpus(t, root)
	if b.Name() != "grimoire" {
		t.Fatalf("name %q", b.Name())
	}

	refs, _, err := b.Retrieve(context.Background(), "T1566", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatal("no refs")
	}
	for _, r := range refs {
		if champion.TrustedSourceLabels[r.SourceTrust] {
			t.Fatalf("third-party text is trusted: %+v", r)
		}
	}
	if tc := (&champion.ToolCall{Name: "write_file", Justification: refs}); !tc.HasUntrustedJustification() {
		t.Fatalf("Rule 2 would pass %+v", refs)
	}
	if refs[0].SourceLabel != "aux_grimoire" || refs[0].Category != "grimoire" {
		t.Fatalf("labels %+v", refs[0])
	}

	refs, contents, err := b.Retrieve(context.Background(), "quartzmoth", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].SourceTrust != "local" || refs[0].SourcePath != "threat-models/own.md" {
		t.Fatalf("refs %+v", refs)
	}
	r := refs[0]
	r.Digest = champion.ReferenceDigest(contents[0].Content)
	if err := champion.VerifyReference(r, func(string) (string, bool) { return contents[0].Content, true }); err != nil {
		t.Fatal(err)
	}
}

func TestCorpus_MissingIndexIsNoHits(t *testing.T) {
	b := NewCorpusBackend(codeindex.NewStore(filepath.Join(t.TempDir(), "none")))
	refs, contents, err := b.Retrieve(context.Background(), "anything", 5)
	if err != nil || refs != nil || contents != nil {
		t.Fatalf("%+v %+v %v", refs, contents, err)
	}
}

func TestCorpus_BrokenIndexIsAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CURRENT"), []byte("nonsense\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewCorpusBackend(codeindex.NewStore(dir)).Retrieve(context.Background(), "x", 5); err == nil {
		t.Fatal("no error")
	}
}

func TestCorpus_FederatesBesideTheCodeShard(t *testing.T) {
	code := NewCodeBackend(codeindex.NewStore(saveIndex(t, chunk("a.go::Open", "func Open() { quartzmoth() }"))))
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "own.md"), []byte("# Own\nquartzmoth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	refs, contents, err := New([]Backend{code, buildCorpus(t, root)}, time.Second, nil).Retrieve(context.Background(), "quartzmoth", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || len(contents) != 2 {
		t.Fatalf("each backend gets its share: %+v", refs)
	}
}

// bundleWith is a STIX bundle of one technique whose description is text.
func bundleWith(text string) string {
	return `{"type":"bundle","objects":[{"type":"attack-pattern","name":"Sample","description":"` + text +
		`","external_references":[{"source_name":"mitre-attack","external_id":"T9999"}]}]}`
}

// Text from outside the project past the cut still reaches the gate: k bounds
// what the model reads, and a hit it never reads can still hold a write.
func TestCorpus_UntrustedHitPastKReachesTheGate(t *testing.T) {
	root := t.TempDir()
	// Both project chunks hold both words, so they outrank the STIX chunk.
	writeCorpusFile(t, root, "kingdom-docs/own.md", "# One\nquartzmoth lantern\n\n# Two\nquartzmoth lantern\n")
	writeCorpusFile(t, root, "mitre-attck/enterprise-attack.json", bundleWith("quartzmoth"))
	b := buildCorpus(t, root)

	refs, contents, err := b.Retrieve(context.Background(), "quartzmoth lantern", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 1 || contents[0].Ref.SourceTrust != "local" {
		t.Fatalf("k is 1 and the model got %+v", contents)
	}
	if tc := (&champion.ToolCall{Name: "write_file", Justification: refs}); !tc.HasUntrustedJustification() {
		t.Fatalf("Rule 2 would pass %+v", refs)
	}

	code := NewCodeBackend(codeindex.NewStore(saveIndex(t, chunk("a.go::Open", "func Open() { quartzmoth(); lantern() }"))))
	refs, _, err = New([]Backend{code, b}, time.Second, nil).Retrieve(context.Background(), "quartzmoth lantern", 2)
	if err != nil {
		t.Fatal(err)
	}
	if tc := (&champion.ToolCall{Name: "write_file", Justification: refs}); !tc.HasUntrustedJustification() {
		t.Fatalf("federated: Rule 2 would pass %+v", refs)
	}
}

// A trusted chunk past k adds nothing the gate needs, and the code shard, all
// of it trusted, still stops at k.
func TestShard_TrustedHitsStopAtK(t *testing.T) {
	root := t.TempDir()
	writeCorpusFile(t, root, "kingdom-docs/own.md", "# One\nquartzmoth\n\n# Two\nquartzmoth\n\n# Three\nquartzmoth\n")
	refs, contents, err := buildCorpus(t, root).Retrieve(context.Background(), "quartzmoth", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 || len(contents) != 2 {
		t.Fatalf("%d refs, %d texts", len(refs), len(contents))
	}
}

// Trust follows who wrote the file, not its format: only the directories this
// project fills itself are local, and everything else is external by default.
func TestCorpus_TrustFollowsTheDirectory(t *testing.T) {
	root := t.TempDir()
	want := map[string]string{
		"threat-models/a.md":      "local",
		"threat-models/deep/b.md": "local",
		"kingdom-docs/c.yaml":     "local",
		"mitre-attck/README.md":   "external",
		"mitre-attck/USAGE.txt":   "external",
		"history/report.md":       "external",
		"nvd-cves/notes.txt":      "external",
		"cwe/notes.md":            "external",
		"own.md":                  "external",
		"threat-models-old/d.md":  "external",
		"kingdom-docs.md":         "external",
	}
	for file := range want {
		writeCorpusFile(t, root, file, "# Own\nquartzmoth\n")
	}
	refs, _, err := buildCorpus(t, root).Retrieve(context.Background(), "quartzmoth", len(want))
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != len(want) {
		t.Fatalf("%d refs for %d files", len(refs), len(want))
	}
	for _, r := range refs {
		if r.SourceTrust != want[r.SourcePath] {
			t.Errorf("%s: %s", r.SourcePath, r.SourceTrust)
		}
		if ok := r.SourceTrust == "local"; ok != champion.TrustedSourceLabels[r.SourceTrust] {
			t.Errorf("%s: %s is not what the gate reads it as", r.SourcePath, r.SourceTrust)
		}
	}
}

// A corpus that reaches the shard's directory by a symlink or a moved path
// must not take the shard's trust: corpus text is never local through it.
func TestCodeBackend_CorpusTextIsNeverLocal(t *testing.T) {
	root := stixCorpus(t)
	writeCorpusFile(t, root, "mitre-attck/README.md", "# Readme\nquartzmoth\n")
	out := t.TempDir()
	if _, err := codeindex.BuildCorpus(context.Background(), root, out); err != nil {
		t.Fatal(err)
	}
	code := NewCodeBackend(codeindex.NewStore(out))
	for _, query := range []string{"T1566", "quartzmoth"} {
		refs, _, err := code.Retrieve(context.Background(), query, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) == 0 {
			t.Fatalf("%s: no refs", query)
		}
		if tc := (&champion.ToolCall{Name: "write_file", Justification: refs}); !tc.HasUntrustedJustification() {
			t.Errorf("%s: Rule 2 would pass %+v", query, refs)
		}
	}
}
