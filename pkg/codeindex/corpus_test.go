// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package codeindex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, body := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func sample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/attack-sample.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestChunkMarkdown_OneChunkPerSection(t *testing.T) {
	src := "intro words\n\n# Top\nunder top\n\n## Next\n```sh\n# not a heading\n```\nunder next\n"
	cs := ChunkMarkdown("t/a.md", []byte(src), "blob")
	if len(cs) != 3 {
		t.Fatalf("%d chunks: %+v", len(cs), cs)
	}
	if cs[0].Symbol != "a.md" || cs[1].Symbol != "Top" || cs[2].Symbol != "Next" {
		t.Fatalf("symbols %q %q %q", cs[0].Symbol, cs[1].Symbol, cs[2].Symbol)
	}
	if cs[2].LineStart != 6 || cs[2].LineEnd != 10 || !strings.Contains(cs[2].Content, "not a heading") {
		t.Fatalf("section %+v", cs[2])
	}
	if cs[1].Language != LanguageMarkdown || cs[1].Kind != "section" || cs[1].Path != "t/a.md" || cs[1].BlobSHA != "blob" {
		t.Fatalf("fields %+v", cs[1])
	}
	if cs[0].ID == cs[1].ID || cs[1].ID == cs[2].ID {
		t.Fatalf("ids repeat: %q %q %q", cs[0].ID, cs[1].ID, cs[2].ID)
	}
}

// A chunk is clipped to MaxChunkBytes in the code shard. Corpus text is what
// the search is for, so a long section splits and every word stays findable.
func TestChunkMarkdown_LongSectionKeepsEveryWord(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Long\n")
	for i := range 400 {
		fmt.Fprintf(&b, "line %d of the section\n", i)
	}
	b.WriteString("the last line holds zebrafinch\n")
	cs := ChunkMarkdown("a.md", []byte(b.String()), "")
	if len(cs) < 2 {
		t.Fatalf("%d chunks", len(cs))
	}
	ids := map[string]bool{}
	for _, c := range cs {
		if len(c.Content) > MaxChunkBytes {
			t.Fatalf("chunk of %d bytes", len(c.Content))
		}
		if c.Symbol != "Long" {
			t.Fatalf("symbol %q", c.Symbol)
		}
		if ids[c.ID] {
			t.Fatalf("id %q repeats", c.ID)
		}
		ids[c.ID] = true
	}
	ix := &Index{Chunks: cs}
	hits := ix.Search("zebrafinch", 3)
	if len(hits) != 1 || hits[0].Chunk.LineEnd != 402 {
		t.Fatalf("hits %+v", hits)
	}
}

func TestChunkText_OneLongLineSplitsOnARune(t *testing.T) {
	line := strings.Repeat("é", MaxChunkBytes) + " tail"
	cs := ChunkText("a.txt", []byte(line), "")
	if len(cs) < 2 {
		t.Fatalf("%d chunks", len(cs))
	}
	var joined strings.Builder
	for _, c := range cs {
		if len(c.Content) > MaxChunkBytes {
			t.Fatalf("chunk of %d bytes", len(c.Content))
		}
		if strings.ContainsRune(c.Content, '�') {
			t.Fatal("a rune was cut in half")
		}
		joined.WriteString(c.Content)
	}
	if !strings.Contains(joined.String(), "tail") {
		t.Fatal("the end of the line went missing")
	}
	if cs[0].Language != LanguageText || cs[0].Symbol != "a.txt" {
		t.Fatalf("fields %+v", cs[0])
	}
}

func TestChunkSTIX_Bundle(t *testing.T) {
	cs, err := ChunkSTIX("mitre-attck/s.json", []byte(sample(t)), "blob")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 4 {
		t.Fatalf("%d chunks, want the three object kinds and no relationship: %+v", len(cs), cs)
	}
	byID := map[string]Chunk{}
	for _, c := range cs {
		byID[c.Symbol] = c
		if c.Language != LanguageSTIX || c.Path != "mitre-attck/s.json" || c.BlobSHA != "blob" {
			t.Fatalf("fields %+v", c)
		}
	}
	tech := byID["T1566"]
	if tech.Kind != "attack-pattern" || tech.Parent != "Phishing" {
		t.Fatalf("technique %+v", tech)
	}
	for _, want := range []string{"T1566 - Phishing", "initial-access", "Linux, macOS", "mailed lures", "gateway logs"} {
		if !strings.Contains(tech.Content, want) {
			t.Errorf("technique lacks %q: %s", want, tech.Content)
		}
	}
	if byID["T1566.001"].Parent != "Spearphishing Attachment" {
		t.Fatalf("the mitre-attack reference was not the one read: %+v", byID)
	}
	if c := byID["M1017"]; c.Kind != "course-of-action" || !strings.Contains(c.Content, "Mitigation") {
		t.Fatalf("mitigation %+v", c)
	}
	if c := byID["G9999"]; c.Kind != "intrusion-set" || !strings.Contains(c.Content, "Quartzfinch") {
		t.Fatalf("group %+v", c)
	}
}

func TestChunkSTIX_LongDescriptionCarriesItsHeaderInEveryChunk(t *testing.T) {
	desc := strings.Repeat("a line of a long description\n", 300) + "ending in zebrafinch"
	bundle, err := json.Marshal(map[string]any{"type": "bundle", "objects": []any{map[string]any{
		"type": "attack-pattern", "name": "Long", "description": desc,
		"external_references": []any{map[string]any{"source_name": "mitre-attack", "external_id": "T9"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	cs, err := ChunkSTIX("s.json", bundle, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) < 2 {
		t.Fatalf("%d chunks", len(cs))
	}
	ids := map[string]bool{}
	for _, c := range cs {
		if len(c.Content) > MaxChunkBytes || !strings.Contains(c.Content, "T9 - Long") || ids[c.ID] {
			t.Fatalf("chunk %q: %d bytes, seen %v", c.ID, len(c.Content), ids[c.ID])
		}
		ids[c.ID] = true
	}
	if !strings.Contains(cs[len(cs)-1].Content, "zebrafinch") {
		t.Fatal("the end of the description went missing")
	}
}

func TestChunkSTIX_OtherJSONIsNotACorpusFile(t *testing.T) {
	for _, src := range []string{`{"downloaded":"2026-01-01"}`, `[{"id":"T1"}]`, `{"type":"not-a-bundle","objects":[]}`, `7`} {
		cs, err := ChunkSTIX("x.json", []byte(src), "")
		if err != nil || cs != nil {
			t.Fatalf("%s: %+v %v", src, cs, err)
		}
	}
}

func TestChunkSTIX_BrokenFileIsAnError(t *testing.T) {
	for _, src := range []string{`{"type":"bundle","objects":[{"type":"attack-pattern"`, `{"type":"bundle","objects":[{"type":"attack-pattern","name":7}]}`} {
		if _, err := ChunkSTIX("x.json", []byte(src), ""); err == nil {
			t.Fatalf("%s: no error", src)
		}
	}
}

func TestBuildCorpus(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	writeTree(t, root, map[string]string{
		"threat-models/a.md":           "# Wire\nthe quartzmoth attack\n",
		"mitre-attck/s.json":           sample(t),
		"mitre-attck/fetch-meta.json":  `{"downloaded":"2026-01-01"}`,
		"mitre-attck/fetch.sh":         "#!/bin/sh\necho shellfinch\n",
		"kingdom-docs/c.yaml":          "key: yamlfinch\n",
		"rag/.venv/lib/site/README.md": "hiddenfinch\n",
		"rag/.venv/lib/site/notes.txt": "hiddenfinch\n",
	})
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("escapefinch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "threat-models", "link.md")); err != nil {
		t.Fatal(err)
	}
	st, err := BuildCorpus(context.Background(), root, out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 3 || st.Chunks != 6 {
		t.Fatalf("stats %+v", st)
	}
	ix, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	for word, path := range map[string]string{"quartzmoth": "threat-models/a.md", "T1566.001": "mitre-attck/s.json", "yamlfinch": "kingdom-docs/c.yaml"} {
		if hits := ix.Search(word, 1); len(hits) != 1 || hits[0].Chunk.Path != path {
			t.Errorf("%s: %+v", word, hits)
		}
	}
	for _, word := range []string{"shellfinch", "hiddenfinch", "escapefinch"} {
		if hits := ix.Search(word, 1); len(hits) != 0 {
			t.Errorf("%s reached the index: %+v", word, hits)
		}
	}
	head := ix.Head
	if head == "" {
		t.Fatal("no head")
	}
	if _, err := BuildCorpus(context.Background(), root, out); err != nil {
		t.Fatal(err)
	}
	if again, err := Load(out); err != nil || again.Head != head {
		t.Fatalf("head moved with no file changed: %v %v", again, err)
	}
	writeTree(t, root, map[string]string{"threat-models/a.md": "# Wire\nchanged\n"})
	if _, err := BuildCorpus(context.Background(), root, out); err != nil {
		t.Fatal(err)
	}
	if again, err := Load(out); err != nil || again.Head == head {
		t.Fatalf("head stayed with a file changed: %v", err)
	}
}

// The index a failed build was meant to replace keeps serving: a half-copied
// bundle must not quietly become a corpus with no MITRE data in it.
func TestBuildCorpus_FailureLeavesTheLastIndex(t *testing.T) {
	root, out := t.TempDir(), t.TempDir()
	writeTree(t, root, map[string]string{"threat-models/a.md": "# Wire\nquartzmoth\n"})
	if _, err := BuildCorpus(context.Background(), root, out); err != nil {
		t.Fatal(err)
	}
	writeTree(t, root, map[string]string{"mitre-attck/s.json": `{"type":"bundle","objects":[{"ty`})
	if _, err := BuildCorpus(context.Background(), root, out); err == nil || !strings.Contains(err.Error(), "mitre-attck/s.json") {
		t.Fatalf("err %v", err)
	}
	empty := t.TempDir()
	if _, err := BuildCorpus(context.Background(), empty, out); err == nil {
		t.Fatal("an empty corpus was saved")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := BuildCorpus(ctx, root, out); err == nil {
		t.Fatal("a cancelled build was saved")
	}
	ix, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if hits := ix.Search("quartzmoth", 1); len(hits) != 1 {
		t.Fatalf("hits %+v", hits)
	}
}

func TestBuildCorpus_MissingRoot(t *testing.T) {
	if _, err := BuildCorpus(context.Background(), filepath.Join(t.TempDir(), "no"), t.TempDir()); err == nil {
		t.Fatal("no error")
	}
}

func TestReadCapped(t *testing.T) {
	if b, err := readCapped(bytes.NewReader([]byte("abc")), 3); err != nil || string(b) != "abc" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := readCapped(bytes.NewReader([]byte("abcd")), 3); err == nil {
		t.Fatal("a file over the limit was read")
	}
}

// The Grimoire's VM is air-gapped: the code that builds and searches its index
// must have no way to the network, and this holds that from outside the code.
func TestNoNetworkInTheBuild(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v: %s", err, out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "net" || strings.HasPrefix(dep, "net/") {
			t.Fatalf("pkg/codeindex depends on %s", dep)
		}
	}
	if !strings.Contains(string(out), "encoding/json") {
		t.Fatalf("go list gave no real answer: %s", out)
	}
}
