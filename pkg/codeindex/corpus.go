// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package codeindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Language of a corpus chunk. STIX is the one written outside this project.
const (
	LanguageMarkdown = "markdown"
	LanguageText     = "text"
	LanguageSTIX     = "stix"
)

// maxCorpusFile bounds one read; a whole ATT&CK bundle fits with room to spare.
const maxCorpusFile = 256 << 20

// maxSTIXHeader bounds the header line repeated in each chunk of one object.
const maxSTIXHeader = 256

// span is lines first to last of a source, joined.
type span struct {
	text        string
	first, last int
}

// pack joins lines into spans of at most limit bytes. No line is dropped: one
// longer than limit splits at a space, or at a rune when it has none.
func pack(lines []string, first, limit int) []span {
	var (
		out      []span
		cur      strings.Builder
		at, last = first, first
	)
	flush := func() {
		if text := strings.TrimRightFunc(cur.String(), unicode.IsSpace); strings.TrimSpace(text) != "" {
			out = append(out, span{text, at, last})
		}
		cur.Reset()
	}
	for i, line := range lines {
		n := first + i
		if cur.Len() > 0 && cur.Len()+len(line)+1 > limit {
			flush()
		}
		if cur.Len() == 0 {
			at = n
		}
		if strings.TrimSpace(line) != "" {
			last = n
		}
		for len(line) > limit {
			head, rest := cut(line, limit)
			cur.WriteString(head)
			flush()
			line = rest
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	flush()
	return out
}

// cut splits s at most limit bytes in, at the last space if there is one,
// always on a rune boundary, and always moves forward.
func cut(s string, limit int) (string, string) {
	i := limit
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	if sp := strings.LastIndexByte(s[:i], ' '); sp > 0 {
		i = sp + 1
	}
	if i == 0 {
		_, i = utf8.DecodeRuneInString(s)
	}
	return s[:i], s[i:]
}

// textChunk is one span of a corpus file. n, the chunk's place in its file,
// keeps the ID unique when a long line gives two chunks one start.
func textChunk(file, language, kind, symbol, blob string, n int, s span) Chunk {
	return Chunk{
		ID: fmt.Sprintf("%s::L%d-%d", file, s.first, n), Path: file, Language: language, Kind: kind, Symbol: symbol,
		LineStart: s.first, LineEnd: s.last, Content: s.text, BlobSHA: blob,
	}
}

func splitLines(src []byte) []string {
	return strings.Split(strings.ToValidUTF8(string(src), "�"), "\n")
}

// ChunkMarkdown gives one chunk per section, or several for a section past
// MaxChunkBytes. A section's Symbol is its heading; text above the first
// heading takes the file's name. A # inside a code fence is not a heading.
func ChunkMarkdown(file string, src []byte, blob string) []Chunk {
	var (
		out     []Chunk
		lines   = splitLines(src)
		heading = path.Base(file)
		start   = 0
		fenced  = false
	)
	flush := func(end int) {
		for _, s := range pack(lines[start:end], start+1, MaxChunkBytes) {
			out = append(out, textChunk(file, LanguageMarkdown, "section", heading, blob, len(out), s))
		}
	}
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		title, ok := mdHeading(line)
		if fenced || !ok {
			continue
		}
		flush(i)
		heading, start = title, i
	}
	flush(len(lines))
	return out
}

func mdHeading(line string) (string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level == len(line) || line[level] != ' ' {
		return "", false
	}
	title := strings.TrimSpace(line[level:])
	return title, title != ""
}

// ChunkText gives a plain file's lines in chunks of at most MaxChunkBytes.
func ChunkText(file string, src []byte, blob string) []Chunk {
	var out []Chunk
	for _, s := range pack(splitLines(src), 1, MaxChunkBytes) {
		out = append(out, textChunk(file, LanguageText, "text", path.Base(file), blob, len(out), s))
	}
	return out
}

type stixObject struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Detection   string   `json:"x_mitre_detection"`
	Platforms   []string `json:"x_mitre_platforms"`
	Aliases     []string `json:"aliases"`
	Phases      []struct {
		Chain string `json:"kill_chain_name"`
		Phase string `json:"phase_name"`
	} `json:"kill_chain_phases"`
	Refs []struct {
		Source string `json:"source_name"`
		ID     string `json:"external_id"`
	} `json:"external_references"`
}

func (o *stixObject) externalID() string {
	for _, r := range o.Refs {
		if r.Source == "mitre-attack" {
			return r.ID
		}
	}
	return ""
}

// body is an object's header and text, or ok false for a kind not indexed.
func (o *stixObject) body(id string) (header, text string, ok bool) {
	switch o.Type {
	case "attack-pattern":
		var tactics []string
		for _, p := range o.Phases {
			if p.Chain == "mitre-attack" {
				tactics = append(tactics, p.Phase)
			}
		}
		text = "Tactics: " + strings.Join(tactics, ", ") + "\nPlatforms: " + strings.Join(o.Platforms, ", ") + "\n\n" + o.Description
		if o.Detection != "" {
			text += "\n\nDetection:\n" + o.Detection
		}
		return "MITRE ATT&CK Technique: " + id + " - " + o.Name, text, true
	case "course-of-action":
		return "MITRE ATT&CK Mitigation: " + id + " - " + o.Name, o.Description, true
	case "intrusion-set":
		text = o.Description
		if len(o.Aliases) > 0 {
			text = "Aliases: " + strings.Join(o.Aliases, ", ") + "\n\n" + text
		}
		return "MITRE ATT&CK Group: " + id + " - " + o.Name, text, true
	}
	return "", "", false
}

// ChunkSTIX gives the techniques, mitigations and groups of a STIX bundle,
// each chunk under its object's header. Valid JSON that is not a bundle
// holds nothing to index and gives no chunks; a bundle that does not parse
// is an error, so a half-copied file is seen and not skipped.
func ChunkSTIX(file string, src []byte, blob string) ([]Chunk, error) {
	if !json.Valid(src) {
		return nil, errors.New("not valid JSON")
	}
	var kind struct {
		Type any `json:"type"`
	}
	if err := json.Unmarshal(src, &kind); err != nil || kind.Type != "bundle" {
		return nil, nil //nolint:nilerr // JSON of another shape: metadata beside the bundles
	}
	var bundle struct {
		Objects []stixObject `json:"objects"`
	}
	if err := json.Unmarshal(src, &bundle); err != nil {
		return nil, err
	}
	var out []Chunk
	for i := range bundle.Objects {
		o := &bundle.Objects[i]
		id := o.externalID()
		header, text, ok := o.body(id)
		if !ok {
			continue
		}
		if id == "" {
			id = o.Name
		}
		if len(header) > maxSTIXHeader {
			header, _ = cut(header, maxSTIXHeader)
		}
		lines := strings.Split(strings.ToValidUTF8(text, "�"), "\n")
		for _, s := range pack(lines, 1, MaxChunkBytes-len(header)-1) {
			out = append(out, Chunk{
				ID: fmt.Sprintf("%s::%s-%d", file, id, len(out)), Path: file, Language: LanguageSTIX, Kind: o.Type,
				Symbol: id, Parent: o.Name, Content: header + "\n" + s.text, BlobSHA: blob,
			})
		}
	}
	return out, nil
}

func chunkCorpusFile(file string, src []byte, blob string) ([]Chunk, error) {
	switch strings.ToLower(path.Ext(file)) {
	case ".md":
		return ChunkMarkdown(file, src, blob), nil
	case ".txt", ".yaml", ".yml", ".toml", ".nix":
		return ChunkText(file, src, blob), nil
	case ".json":
		return ChunkSTIX(file, src, blob)
	}
	return nil, nil
}

func readCapped(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("larger than %d bytes", limit)
	}
	return b, nil
}

func readCorpusFile(fsys fs.FS, file string) ([]byte, error) {
	f, err := fsys.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return readCapped(f, maxCorpusFile)
}

// BuildCorpus indexes the plain files under root into dir: no git, no model,
// no network, so it runs on an air-gapped host. Regular files only: a symlink
// is not followed and a dot directory (a Python venv, say) is not entered.
// Head is a digest of every indexed path and its content. A file that cannot
// be read, a corpus with nothing in it or a cancelled ctx saves nothing, and
// the index already in dir keeps serving.
func BuildCorpus(ctx context.Context, root, dir string) (Stats, error) {
	var st Stats
	r, err := os.OpenRoot(root)
	if err != nil {
		return st, err
	}
	defer func() { _ = r.Close() }()
	fsys := r.FS()
	ix := &Index{Files: map[string]string{}}
	head := sha256.New()
	err = fs.WalkDir(fsys, ".", func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if file != "." && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		src, err := readCorpusFile(fsys, file)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		sum := sha256.Sum256(src)
		blob := hex.EncodeToString(sum[:])
		chunks, err := chunkCorpusFile(file, src, blob)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		if len(chunks) == 0 {
			return nil
		}
		ix.Files[file] = blob
		ix.Chunks = append(ix.Chunks, chunks...)
		_, _ = fmt.Fprintf(head, "%s\x00%s\n", file, blob)
		return nil
	})
	if err != nil {
		return st, err
	}
	if len(ix.Chunks) == 0 {
		return st, fmt.Errorf("no corpus text under %s", root)
	}
	ix.Head = hex.EncodeToString(head.Sum(nil))
	st.Files, st.Parsed, st.Chunks = len(ix.Files), len(ix.Files), len(ix.Chunks)
	return st, Save(dir, ix)
}
