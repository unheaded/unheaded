// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"unheaded/pkg/zhencapture"
)

// Filters (S-TWO-BRAINS Step 370).
const (
	MaxPromptBytes = 16 << 10
	MaxAnswerBytes = 8 << 10
)

// ClaudeTextEnv turns -include-claude-text on when it is exactly "1".
const ClaudeTextEnv = "ZHEN_FLYWHEEL_INCLUDE_CLAUDE_TEXT"

// claudeTextNote is written into every manifest.
const claudeTextNote = "Claude-authored text is excluded from every training target unless -include-claude-text: " +
	"Anthropic's terms list using Outputs as training targets as prohibited without written permission; " +
	"S-TWO-BRAINS Step 340 is open (support asked 2026-10-23)."

func includeClaudeDefault() bool { return os.Getenv(ClaudeTextEnv) == "1" }

// A refusal or "I don't know" near the start of an answer.
var refusalRE = regexp.MustCompile(`(?i)\b(i don'?t know|i do not know|i'?m not sure|i am not sure|i can(no|')?t (help|answer|assist)|as an ai\b|i'?m (sorry|unable)|i am (sorry|unable)|i apologi[sz]e)`)

// Outcome sets.
var (
	passKinds = map[string]bool{LabelCheckPass: true, LabelJudgePass: true, LabelAccepted: true}
	failKinds = map[string]bool{LabelCheckFail: true, LabelJudgeFail: true, LabelRejected: true, LabelEscalated: true}
)

// ExportRecord is one SFT example (the text JSONL, strict schema).
type ExportRecord struct {
	ID         string `json:"id"`
	Family     string `json:"family"`
	Split      string `json:"split"`
	AuthorKind string `json:"author_kind"`
	Source     string `json:"source"`
	Question   string `json:"question"`
	Context    string `json:"context"`
	Answer     string `json:"answer"`
}

// PreferencePair is one DPO-shaped pair: same task, a good answer and a
// failed local one. Exported for later; not trained this sprint.
type PreferencePair struct {
	ID               string `json:"id"`
	Family           string `json:"family"`
	Split            string `json:"split"`
	Prompt           string `json:"prompt"`
	Chosen           string `json:"chosen"`
	Rejected         string `json:"rejected"`
	ChosenID         string `json:"chosen_id"`
	RejectedID       string `json:"rejected_id"`
	ChosenAuthorKind string `json:"chosen_author_kind"`
}

// ToolTraceRecord is a local tool-use trace that ended in a passing check.
type ToolTraceRecord struct {
	ID         string          `json:"id"`
	Family     string          `json:"family"`
	Split      string          `json:"split"`
	AuthorKind string          `json:"author_kind"`
	Prompt     string          `json:"prompt"`
	ToolTrace  json.RawMessage `json:"tool_trace"`
	Answer     string          `json:"answer"`
}

// Tokenizer turns a text SFT file into forge records
// ({"tokens":[...],"answer_start":N}, one per line) and an ids file (the
// capture id of each forge line). It reports the ids it could not encode.
type Tokenizer interface {
	Tokenize(ctx context.Context, in, out, ids string) (skipped map[string]string, err error)
}

// Exporter builds one dataset directory from the Well.
type Exporter struct {
	Store             Store
	IncludeClaudeText bool
	Refs              *References
	Tokenizer         Tokenizer // nil: no forge files
	Out               string
	GitHead           string
	Promote           bool // mark exported captures promoted (kept past retention)
	Now               func() time.Time
}

// FileInfo is one exported file in the manifest.
type FileInfo struct {
	SHA256 string `json:"sha256"`
	Lines  int    `json:"lines"`
	Bytes  int    `json:"bytes"`
}

// Manifest describes an export: counts, hashes, the flag, provenance.
// No text.
type Manifest struct {
	Schema                string                    `json:"schema"`
	GeneratedAt           string                    `json:"generated_at"`
	GitHead               string                    `json:"git_head"`
	IncludeClaudeText     bool                      `json:"include_claude_text"`
	ClaudeTextNote        string                    `json:"claude_text_note"`
	SplitRule             string                    `json:"split_rule"`
	Captures              map[string]any            `json:"captures"`
	Counts                map[string]map[string]int `json:"counts"` // dataset -> split -> n
	BySource              map[string]int            `json:"by_source"`
	ByAuthorKind          map[string]int            `json:"by_author_kind"`
	ByFamily              map[string]map[string]int `json:"by_family"` // family -> split -> SFT n
	Excluded              map[string]int            `json:"excluded"`
	ExcludedIDs           map[string][]string       `json:"excluded_ids"`
	RedactionPlaceholders int                       `json:"redaction_placeholders"`
	GroupKeys             map[string][]string       `json:"group_keys"` // split -> sha256(group key)
	Files                 map[string]FileInfo       `json:"files"`
	IDs                   map[string][]string       `json:"ids"` // file -> capture ids in it
}

type candidate struct {
	c      *Capture
	kinds  map[string]bool
	split  string
	group  string
	reason string // "" = kept
}

func (ex *Exporter) Run(ctx context.Context) (*Manifest, error) {
	if ex.Refs == nil {
		return nil, errors.New("export needs the eval references (split, coding-gate prompts)")
	}
	out, err := zhencapture.CheckDir(ex.Out) // outside any repository, 0700, ours
	if err != nil {
		return nil, err
	}
	if ents, err := os.ReadDir(out); err != nil {
		return nil, err
	} else if len(ents) > 0 {
		return nil, fmt.Errorf("%s is not empty; an export is never overwritten (use a new dated directory)", out)
	}
	caps, err := ex.Store.Captures(ctx)
	if err != nil {
		return nil, err
	}
	labels, err := ex.Store.Labels(ctx)
	if err != nil {
		return nil, err
	}
	kindsOf := map[string]map[string]bool{}
	for _, l := range labels {
		if kindsOf[l.CaptureID] == nil {
			kindsOf[l.CaptureID] = map[string]bool{}
		}
		kindsOf[l.CaptureID][l.Kind] = true
	}
	now := time.Now
	if ex.Now != nil {
		now = ex.Now
	}
	m := &Manifest{
		Schema: "zhen-flywheel-export/v1", GeneratedAt: now().UTC().Format(time.RFC3339), GitHead: ex.GitHead,
		IncludeClaudeText: ex.IncludeClaudeText, ClaudeTextNote: claudeTextNote,
		SplitRule: fmt.Sprintf("group = agent run, eval task or conversation; hold-out when sha256(family+group)[0:8] big-endian mod 100 < %d; "+
			"two-brains hold-out tasks stay hold-out, other eval tasks are excluded; a train prompt with >= %.1f overlap of %d-word shingles "+
			"with a coding-gate prompt, a two-brains hold-out task or a flywheel hold-out prompt is excluded; normalized duplicates: hold-out copy wins",
			HoldoutPercent, ContaminationThreshold, shingleWords),
		Counts: map[string]map[string]int{}, BySource: map[string]int{}, ByAuthorKind: map[string]int{},
		ByFamily: map[string]map[string]int{}, Excluded: map[string]int{}, ExcludedIDs: map[string][]string{},
		GroupKeys: map[string][]string{}, Files: map[string]FileInfo{}, IDs: map[string][]string{},
	}
	m.Captures = map[string]any{"total": len(caps)}
	if len(caps) > 0 {
		lo, hi := caps[0].CapturedAt, caps[0].CapturedAt
		for _, c := range caps {
			if c.CapturedAt.Before(lo) {
				lo = c.CapturedAt
			}
			if c.CapturedAt.After(hi) {
				hi = c.CapturedAt
			}
		}
		m.Captures["first_captured_at"], m.Captures["last_captured_at"] = lo.UTC().Format(time.RFC3339), hi.UTC().Format(time.RFC3339)
	}

	// Every capture gets a split (by its group) and, if it is not an SFT
	// example, the first reason it is not.
	cands := make([]*candidate, 0, len(caps))
	for i := range caps {
		c := &caps[i]
		k := kindsOf[c.ID]
		if k == nil {
			k = map[string]bool{}
		}
		cd := &candidate{c: c, kinds: k, group: groupKey(c)}
		cd.split = "train"
		if splitBucket(c.TaskFamily, cd.group) < HoldoutPercent {
			cd.split = "holdout"
		}
		if t := evalTaskOf(c); t != "" {
			if ex.Refs.EvalSplit[t] == "holdout" {
				cd.split = "holdout"
			} else {
				cd.split = ""
			}
		}
		cd.reason = ex.baseReason(cd)
		cands = append(cands, cd)
	}
	exclude := func(cd *candidate, why string) {
		cd.reason = why
	}
	// Normalized duplicates: hold-out first, so a held-out task's twin
	// never trains.
	seen := map[string]bool{}
	for _, split := range []string{"holdout", "train"} {
		for _, cd := range cands {
			if cd.reason != "" || cd.split != split {
				continue
			}
			key := normalized(cd.c.Prompt) + "\x00" + normalized(cd.c.Response)
			if seen[key] {
				exclude(cd, "duplicate")
				continue
			}
			seen[key] = true
		}
	}
	// Contamination of train prompts.
	var holdoutPrompts []string
	for _, cd := range cands {
		if cd.reason == "" && cd.split == "holdout" {
			holdoutPrompts = append(holdoutPrompts, cd.c.Prompt)
		}
	}
	for _, cd := range cands {
		if cd.reason != "" || cd.split != "train" {
			continue
		}
		for _, r := range ex.Refs.Prompts {
			if overlap(cd.c.Prompt, r) >= ContaminationThreshold {
				exclude(cd, "contamination_eval")
				break
			}
		}
		if cd.reason != "" {
			continue
		}
		for _, r := range holdoutPrompts {
			if overlap(cd.c.Prompt, r) >= ContaminationThreshold {
				exclude(cd, "contamination_holdout")
				break
			}
		}
	}

	sft := map[string][]ExportRecord{}
	exported := map[string]bool{}
	for _, cd := range cands {
		if cd.reason != "" {
			m.Excluded[cd.reason]++
			m.ExcludedIDs[cd.reason] = append(m.ExcludedIDs[cd.reason], cd.c.ID)
			continue
		}
		c := cd.c
		sft[cd.split] = append(sft[cd.split], ExportRecord{ID: c.ID, Family: c.TaskFamily, Split: cd.split, AuthorKind: c.AuthorKind,
			Source: c.Source, Question: c.Prompt, Context: "", Answer: c.Response})
		exported[c.ID] = true
		m.BySource[c.Source]++
		m.ByAuthorKind[c.AuthorKind]++
		if m.ByFamily[c.TaskFamily] == nil {
			m.ByFamily[c.TaskFamily] = map[string]int{}
		}
		m.ByFamily[c.TaskFamily][cd.split]++
		m.RedactionPlaceholders += strings.Count(c.Prompt, "[REDACTED:") + strings.Count(c.Response, "[REDACTED:")
	}

	// Preference pairs: same split, group and prompt; chosen is an SFT
	// example, rejected a local answer with a failing outcome.
	prefs := map[string][]PreferencePair{}
	chosenBy := map[string][]*candidate{}
	for _, cd := range cands {
		if cd.reason == "" {
			chosenBy[cd.split+"\x00"+cd.group+"\x00"+normalized(cd.c.Prompt)] = append(chosenBy[cd.split+"\x00"+cd.group+"\x00"+normalized(cd.c.Prompt)], cd)
		}
	}
	for _, rj := range cands {
		if rj.c.AuthorKind != AuthorLocal || rj.c.Quarantined || rj.split == "" || !hasAny(rj.kinds, failKinds) {
			continue
		}
		for _, ch := range chosenBy[rj.split+"\x00"+rj.group+"\x00"+normalized(rj.c.Prompt)] {
			if ch.c.ID == rj.c.ID {
				continue
			}
			sum := sha256.Sum256([]byte(ch.c.ID + rj.c.ID))
			prefs[rj.split] = append(prefs[rj.split], PreferencePair{ID: hex.EncodeToString(sum[:]), Family: rj.c.TaskFamily, Split: rj.split,
				Prompt: rj.c.Prompt, Chosen: ch.c.Response, Rejected: rj.c.Response, ChosenID: ch.c.ID, RejectedID: rj.c.ID,
				ChosenAuthorKind: ch.c.AuthorKind})
			exported[rj.c.ID] = true
		}
	}
	// Tool traces: SFT examples with a trace and a passing check.
	traces := map[string][]ToolTraceRecord{}
	for _, cd := range cands {
		if cd.reason != "" || string(cd.c.ToolTrace) == "[]" || len(cd.c.ToolTrace) == 0 || !cd.kinds[LabelCheckPass] {
			continue
		}
		traces[cd.split] = append(traces[cd.split], ToolTraceRecord{ID: cd.c.ID, Family: cd.c.TaskFamily, Split: cd.split,
			AuthorKind: cd.c.AuthorKind, Prompt: cd.c.Prompt, ToolTrace: cd.c.ToolTrace, Answer: cd.c.Response})
	}

	groups := map[string]map[string]bool{"train": {}, "holdout": {}}
	for _, cd := range cands {
		if cd.reason == "" {
			groups[cd.split][sha256Hex(cd.c.TaskFamily+"\x00"+cd.group)] = true
		}
	}
	for split, g := range groups {
		keys := make([]string, 0, len(g))
		for k := range g {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		m.GroupKeys[split] = keys
	}

	for _, split := range []string{"train", "holdout"} {
		sort.Slice(sft[split], func(i, j int) bool { return sft[split][i].ID < sft[split][j].ID })
		sort.Slice(prefs[split], func(i, j int) bool { return prefs[split][i].ID < prefs[split][j].ID })
		sort.Slice(traces[split], func(i, j int) bool { return traces[split][i].ID < traces[split][j].ID })
		if err := writeJSONL(out, "sft-"+split+".jsonl", sft[split], m, func(r ExportRecord) string { return r.ID }); err != nil {
			return nil, err
		}
		if err := writeJSONL(out, "preference-"+split+".jsonl", prefs[split], m, func(r PreferencePair) string { return r.ChosenID + "," + r.RejectedID }); err != nil {
			return nil, err
		}
		if err := writeJSONL(out, "tool-traces-"+split+".jsonl", traces[split], m, func(r ToolTraceRecord) string { return r.ID }); err != nil {
			return nil, err
		}
		if m.Counts["sft"] == nil {
			m.Counts["sft"], m.Counts["preference"], m.Counts["tool_traces"], m.Counts["tokenized"] = map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
		}
		m.Counts["sft"][split] = len(sft[split])
		m.Counts["preference"][split] = len(prefs[split])
		m.Counts["tool_traces"][split] = len(traces[split])
		if ex.Tokenizer != nil {
			n, err := ex.tokenize(ctx, out, split, m)
			if err != nil {
				return nil, err
			}
			m.Counts["tokenized"][split] = n
		}
	}
	if err := writeManifest(out, m); err != nil {
		return nil, err
	}
	if ex.Promote {
		ids := make([]string, 0, len(exported))
		for id := range exported {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if err := ex.Store.Promote(ctx, ids); err != nil {
			return m, fmt.Errorf("export written, but promoting its captures failed (they may be purged at 180 days): %w", err)
		}
	}
	return m, nil
}

func hasAny(k map[string]bool, set map[string]bool) bool {
	for kind := range k {
		if set[kind] {
			return true
		}
	}
	return false
}

// baseReason is the first filter a capture fails, or "".
func (ex *Exporter) baseReason(cd *candidate) string {
	c := cd.c
	switch {
	case cd.split == "":
		return "eval_task_not_holdout"
	case c.Quarantined:
		return "quarantined"
	case c.AuthorKind == AuthorClaude && !ex.IncludeClaudeText:
		return "claude_text_excluded"
	case c.Truncated || strings.Contains(c.Response, zhencapture.TruncationMarker) || strings.Contains(c.Prompt, zhencapture.TruncationMarker):
		return "truncated"
	case len(c.Prompt) > MaxPromptBytes:
		return "prompt_too_long"
	case strings.TrimSpace(c.Response) == "" || len(c.Response) > MaxAnswerBytes:
		return "answer_length"
	case !utf8.ValidString(c.Prompt) || !utf8.ValidString(c.Response):
		return "invalid_utf8"
	case refusalRE.MatchString(firstN(c.Response, 200)):
		return "refusal"
	case c.AuthorKind == AuthorHuman:
		return "" // Stevie's own text: no outcome needed
	case hasAny(cd.kinds, failKinds):
		return "failing_outcome"
	case !hasAny(cd.kinds, passKinds):
		return "no_passing_outcome"
	}
	return ""
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// writeJSONL writes rows, one JSON object per line, 0600, and records
// the file in the manifest. An empty dataset is an empty file.
func writeJSONL[T any](dir, name string, rows []T, m *Manifest, id func(T) string) error {
	var buf bytes.Buffer
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
		ids = append(ids, id(r))
	}
	if err := writePrivate(filepath.Join(dir, name), buf.Bytes()); err != nil {
		return err
	}
	m.Files[name] = fileInfo(buf.Bytes())
	m.IDs[name] = ids
	return nil
}

func fileInfo(b []byte) FileInfo {
	sum := sha256.Sum256(b)
	return FileInfo{SHA256: hex.EncodeToString(sum[:]), Lines: bytes.Count(b, []byte{'\n'}), Bytes: len(b)}
}

func writePrivate(p string, b []byte) error {
	// #nosec G304 -- a fixed name in the export directory just validated.
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeManifest(dir string, m *Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(dir, "MANIFEST.json"), append(b, '\n'))
}

// tokenize runs the tokenizer on sft-<split>.jsonl; examples it cannot
// encode become exclusions (and leave the ids file, so the forge file
// and the ids stay line-aligned).
func (ex *Exporter) tokenize(ctx context.Context, dir, split string, m *Manifest) (int, error) {
	in := filepath.Join(dir, "sft-"+split+".jsonl")
	tok, ids := split+".tok.jsonl", split+".tok.ids"
	skipped, err := ex.Tokenizer.Tokenize(ctx, in, filepath.Join(dir, tok), filepath.Join(dir, ids))
	if err != nil {
		return 0, fmt.Errorf("tokenize %s: %w", split, err)
	}
	keys := make([]string, 0, len(skipped))
	for id := range skipped {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		why := "tokenize_" + skipped[id]
		m.Excluded[why]++
		m.ExcludedIDs[why] = append(m.ExcludedIDs[why], id)
	}
	n := 0
	for _, name := range []string{tok, ids} {
		// #nosec G304 -- files the tokenizer just wrote in the export directory.
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return 0, err
		}
		if err := os.Chmod(filepath.Join(dir, name), 0o600); err != nil {
			return 0, err
		}
		m.Files[name] = fileInfo(b)
		n = bytes.Count(b, []byte{'\n'})
	}
	idsB, _ := os.ReadFile(filepath.Join(dir, ids)) // #nosec G304 -- as above
	m.IDs[tok] = strings.Fields(string(idsB))
	return n, nil
}

// scriptTokenizer runs scripts/tokenize-coding-for-gemma4.py --strict.
type scriptTokenizer struct {
	Python    string
	Script    string
	MaxTokens int
}

func (s scriptTokenizer) Tokenize(ctx context.Context, in, out, ids string) (map[string]string, error) {
	skippedPath := out + ".skipped"
	// #nosec G304 -- the export's own text file.
	stdin, err := os.Open(in)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stdin.Close() }()
	o, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- the export directory
	if err != nil {
		return nil, err
	}
	defer func() { _ = o.Close() }()
	var stderr bytes.Buffer
	// #nosec G204 -- the operator's python and the repository's tokenizer script; fixed flags
	cmd := exec.CommandContext(ctx, s.Python, "-I", s.Script, "--strict", "--max-tokens", fmt.Sprint(s.MaxTokens),
		"--ids", ids, "--skipped", skippedPath)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, o, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w\n%s", filepath.Base(s.Script), err, strings.TrimSpace(stderr.String()))
	}
	skipped := map[string]string{}
	rows, err := readJSONL[struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}](skippedPath)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		skipped[r.ID] = r.Reason
	}
	return skipped, os.Remove(skippedPath)
}

func cmdExport(ctx context.Context, args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "new export directory, outside any repository (created 0700; must not exist or be empty)")
	root := fs.String("root", ".", "repository root (eval references, tokenizer script)")
	claude := fs.Bool("include-claude-text", includeClaudeDefault(), "let Claude-authored text into training targets (OFF: S-TWO-BRAINS Step 340; env "+ClaudeTextEnv+"=1)")
	python := fs.String("tokenizer-python", envOr("ZHEN_TOKENIZER_PYTHON", "/home/govan/tmp/gemma4-venv/bin/python"), "python with transformers for the Gemma tokenizer")
	maxTok := fs.Int("max-tokens", 384, "forge sequence length; longer examples are excluded, never cut into the answer")
	noTok := fs.Bool("no-tokenize", false, "text files only")
	promote := fs.Bool("promote", true, "mark every exported capture promoted in the Well (kept past the 180-day retention)")
	if err := fs.Parse(args); err != nil {
		return 2, nil
	}
	if *out == "" {
		return 2, errors.New("-out is required")
	}
	refs, err := loadReferences(*root)
	if err != nil {
		return 1, fmt.Errorf("eval references: %w", err)
	}
	st, closeFn, err := openStore(false)
	if err != nil {
		return 1, err
	}
	defer closeFn()
	ex := &Exporter{Store: st, IncludeClaudeText: *claude, Refs: refs, Out: *out, Promote: *promote, GitHead: gitHead(*root)}
	if !*noTok {
		ex.Tokenizer = scriptTokenizer{Python: *python, Script: filepath.Join(*root, "scripts/tokenize-coding-for-gemma4.py"), MaxTokens: *maxTok}
	}
	m, err := ex.Run(ctx)
	if m != nil {
		printJSON(stdout, map[string]any{"out": *out, "include_claude_text": m.IncludeClaudeText, "counts": m.Counts,
			"by_author_kind": m.ByAuthorKind, "by_family": m.ByFamily, "excluded": m.Excluded})
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func gitHead(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output() // #nosec G204 -- fixed arguments
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
