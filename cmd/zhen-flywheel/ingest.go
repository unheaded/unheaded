// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"unheaded/pkg/zhencapture"
)

// maxLineBytes bounds one capture line (the writer caps every string at
// 64 KiB; a record has a handful of them).
const maxLineBytes = 4 << 20

// Ingester moves capture files into the Well: strictly parsed, re-scanned,
// deduplicated by content.
type Ingester struct {
	Store   Store
	Scanner Scanner
	Log     io.Writer
}

// IngestReport counts what one ingest did. Errors and QuarantineReasons
// name file and line, never text.
type IngestReport struct {
	Files             int      `json:"files"`
	Lines             int      `json:"lines"`
	Records           int      `json:"records"`
	Inserted          int      `json:"inserted"`
	Duplicates        int      `json:"duplicates"`
	LabelsInserted    int      `json:"labels_inserted"`
	Quarantined       int      `json:"quarantined"`
	NoAnswer          int      `json:"records_without_answer"`
	Moved             int      `json:"files_moved"`
	Errors            []string `json:"errors,omitempty"`
	QuarantineReasons []string `json:"quarantine_reasons,omitempty"`
}

// IngestDir ingests every zhen-capture-*.jsonl directly in dir. dir must
// pass pkg/zhencapture's rules (outside any git work tree, 0700, ours).
// A file with any bad line stays in place for a human to look at; a
// clean file moves to dir/ingested/.
func (in *Ingester) IngestDir(ctx context.Context, dir string) (IngestReport, error) {
	var rep IngestReport
	real, err := zhencapture.CheckDir(dir)
	if err != nil {
		return rep, err
	}
	files, err := filepath.Glob(filepath.Join(real, "zhen-capture-*.jsonl"))
	if err != nil {
		return rep, err
	}
	sort.Strings(files)
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		rep.Files++
		clean, err := in.ingestFile(ctx, real, f, &rep)
		if err != nil {
			return rep, err
		}
		if clean {
			if err := moveTo(filepath.Join(real, "ingested"), f); err != nil {
				return rep, err
			}
			rep.Moved++
		}
	}
	return rep, nil
}

// checkCaptureFile: a regular file (no symlink), owned by us, 0600.
func checkCaptureFile(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("owned by uid %d, not this user", st.Uid)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		return fmt.Errorf("mode %04o; capture files must be 0600", perm)
	}
	return nil
}

// ingestFile returns whether every line of f was handled (inserted,
// duplicate or quarantined). An error is a store failure: stop.
func (in *Ingester) ingestFile(ctx context.Context, dir, f string, rep *IngestReport) (bool, error) {
	name := filepath.Base(f)
	if err := checkCaptureFile(f); err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s: refused: %v", name, err))
		return false, nil
	}
	// #nosec G304 -- f is a checked regular file in the validated capture dir.
	fh, err := os.OpenFile(f, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s: %v", name, err))
		return false, nil
	}
	defer func() { _ = fh.Close() }()
	r := bufio.NewReaderSize(fh, 64<<10)
	clean := true
	for n := 1; ; n++ {
		line, rerr := readLine(r)
		if rerr == io.EOF && line == nil {
			break
		}
		if rerr != nil && rerr != io.EOF {
			rep.Errors = append(rep.Errors, fmt.Sprintf("%s:%d: %v", name, n, rerr))
			clean = false
			break
		}
		if len(bytes.TrimSpace(line)) == 0 {
			if rerr == io.EOF {
				break
			}
			continue
		}
		rep.Lines++
		ok, err := in.ingestLine(ctx, dir, name, n, line, rep)
		if err != nil {
			return false, err
		}
		clean = clean && ok
		if rerr == io.EOF {
			break
		}
	}
	return clean, nil
}

// readLine reads one line without its newline, refusing one longer than
// maxLineBytes.
func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, isPrefix, err := r.ReadLine()
		buf = append(buf, chunk...)
		if len(buf) > maxLineBytes {
			return nil, fmt.Errorf("line longer than %d bytes", maxLineBytes)
		}
		if err != nil {
			if len(buf) > 0 && err == io.EOF {
				return buf, io.EOF
			}
			return nil, err
		}
		if !isPrefix {
			return buf, nil
		}
	}
}

// ingestLine handles one record: parse strictly, map to captures,
// re-scan, then insert or quarantine. false: the line was bad.
func (in *Ingester) ingestLine(ctx context.Context, dir, file string, n int, line []byte, rep *IngestReport) (bool, error) {
	rec, err := parseRecord(line)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s:%d: %v", file, n, err))
		return false, nil
	}
	caps, labels, err := capturesFromRecord(rec)
	if err != nil {
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s:%d: record %s: %v", file, n, rec.ID, err))
		return false, nil
	}
	rep.Records++
	if len(caps) == 0 {
		rep.NoAnswer++
		return true, nil
	}
	kinds, err := in.rescan(ctx, caps)
	if err != nil {
		// The scanner could not run: nothing from this line may enter.
		rep.Errors = append(rep.Errors, fmt.Sprintf("%s:%d: record %s: re-scan failed: %v", file, n, rec.ID, err))
		return false, nil
	}
	if len(kinds) > 0 {
		if err := quarantine(filepath.Join(dir, "quarantine"), file, n, rec.ID, kinds, line); err != nil {
			return false, err
		}
		rep.Quarantined++
		rep.QuarantineReasons = append(rep.QuarantineReasons, fmt.Sprintf("%s:%d record %s: %s", file, n, rec.ID, kindList(kinds)))
		return true, nil
	}
	for i := range caps {
		ok, err := in.Store.InsertCapture(ctx, &caps[i])
		if err != nil {
			return false, fmt.Errorf("%s:%d: insert: %w", file, n, err)
		}
		if ok {
			rep.Inserted++
		} else {
			rep.Duplicates++
		}
	}
	for _, l := range labels {
		ok, err := in.Store.InsertLabel(ctx, l)
		if err != nil {
			return false, fmt.Errorf("%s:%d: label: %w", file, n, err)
		}
		if ok {
			rep.LabelsInserted++
		}
	}
	return true, nil
}

// parseRecord decodes one line strictly: valid JSON, a known schema, the
// fields a capture needs, every field of the right type. Unknown fields
// are ignored (v1 readers must).
func parseRecord(line []byte) (*zhencapture.Record, error) {
	var rec zhencapture.Record
	dec := json.NewDecoder(bytes.NewReader(line))
	if err := dec.Decode(&rec); err != nil {
		return nil, fmt.Errorf("not a capture record: %v", err)
	}
	if dec.More() {
		return nil, errors.New("trailing data after the record")
	}
	switch {
	case rec.Schema != zhencapture.Schema:
		return nil, fmt.Errorf("schema %q is not %s", rec.Schema, zhencapture.Schema)
	case rec.ID == "" || len(rec.ID) > 128:
		return nil, errors.New("missing or oversized id")
	case rec.Time.IsZero():
		return nil, errors.New("missing ts")
	}
	return &rec, nil
}

func sourceOf(s string) (string, error) {
	switch s {
	case "zhen-claude":
		return SourceZhenClaude, nil
	case "zhen-mcp/local_llm", "local_llm":
		return SourceLocalLLM, nil
	case "zhen-agentd":
		return SourceAgentd, nil
	case "zhen-eval":
		return SourceEval, nil
	}
	return "", fmt.Errorf("unknown source %q", s)
}

func truncatedLeg(rec *zhencapture.Record, l *zhencapture.Leg) bool {
	if rec.Truncated {
		return true
	}
	switch l.StopReason {
	case "length", "max_tokens":
		return true
	}
	return false
}

// capturesFromRecord maps one turn to its captures (one per answer: the
// local leg, the Claude leg, or human text) and the labels the record
// itself carries (its -check result, an escalation).
func capturesFromRecord(rec *zhencapture.Record) ([]Capture, []Label, error) {
	src, err := sourceOf(rec.Source)
	if err != nil {
		return nil, nil, err
	}
	family := rec.TaskFamily
	if family == "" {
		family = UnknownFamily
	}
	if !familyRE.MatchString(family) {
		return nil, nil, fmt.Errorf("task_family %q is not [a-z0-9-]{1,64}", rec.TaskFamily)
	}
	if rec.RunID != "" && !runIDRE.MatchString(rec.RunID) {
		return nil, nil, fmt.Errorf("run_id %q is not 16 hex", rec.RunID)
	}
	conv := rec.ConversationID
	if conv == "" {
		conv = "record:" + rec.ID // v1 records written before the field existed
	}
	if len(conv) > 128 {
		return nil, nil, errors.New("conversation_id longer than 128")
	}
	switch rec.AuthorKind {
	case "", zhencapture.AuthorHuman:
	default:
		return nil, nil, fmt.Errorf("author_kind %q: only %q is valid on a record", rec.AuthorKind, zhencapture.AuthorHuman)
	}
	base := Capture{
		CapturedAt: rec.Time.UTC(), Source: src, TaskFamily: family, ConversationID: conv, RunID: rec.RunID,
		RecordID: rec.ID, Schema: rec.Schema, RedactionVersion: RedactionVersion, Prompt: rec.Prompt,
	}
	var caps []Capture
	var labels []Label
	add := func(c Capture) error {
		trace, err := canonicalTrace(c.ToolTrace)
		if err != nil {
			return err
		}
		c.ToolTrace = trace
		if c.Model == "" || len(c.Model) > 128 {
			return fmt.Errorf("%s answer without a model name", c.AuthorKind)
		}
		if c.ID, err = CaptureID(c.Model, c.Prompt, c.Response, c.ToolTrace); err != nil {
			return err
		}
		caps = append(caps, c)
		return nil
	}
	localIdx := -1
	if l := rec.Local; l != nil && l.Answer != "" {
		c := base
		c.Backend, c.AuthorKind, c.Model, c.Response, c.Truncated = AuthorLocal, AuthorLocal, l.Model, l.Answer, truncatedLeg(rec, l)
		if err := add(c); err != nil {
			return nil, nil, err
		}
		localIdx = len(caps) - 1
	}
	if l := rec.Claude; l != nil && l.Answer != "" {
		c := base
		model := l.Model
		if model == "" {
			model = l.ServedBy
		}
		c.Backend, c.AuthorKind, c.Model, c.Response, c.Truncated = AuthorClaude, AuthorClaude, model, l.Answer, truncatedLeg(rec, l)
		if len(rec.ToolCalls) > 0 {
			raw, err := json.Marshal(rec.ToolCalls)
			if err != nil {
				return nil, nil, err
			}
			c.ToolTrace = raw
		}
		if err := add(c); err != nil {
			return nil, nil, err
		}
	}
	if rec.AuthorKind == zhencapture.AuthorHuman && rec.Final != "" {
		c := base
		c.Backend, c.AuthorKind, c.Model, c.Response, c.Truncated = BackendNone, AuthorHuman, AuthorHuman, rec.Final, rec.Truncated
		if err := add(c); err != nil {
			return nil, nil, err
		}
	}
	if localIdx >= 0 {
		id := caps[localIdx].ID
		if chk := rec.Check; chk != nil {
			kind := LabelCheckFail
			if chk.Accepted {
				kind = LabelCheckPass
			}
			labels = append(labels, Label{CaptureID: id, Kind: kind, SourceRef: "capture:" + rec.ID + ":check",
				Value: jsonObject(map[string]any{"argv": chk.Argv, "exit": chk.Exit})})
		}
		if rec.Route == zhencapture.RouteEscalated {
			labels = append(labels, Label{CaptureID: id, Kind: LabelEscalated, SourceRef: "capture:" + rec.ID + ":route",
				Value: jsonObject(map[string]any{"route": rec.Route})})
		}
	}
	return caps, labels, nil
}

// rescan runs the scanner over every text a capture would store and
// returns the hit kinds (empty: clean).
func (in *Ingester) rescan(ctx context.Context, caps []Capture) (map[string]int, error) {
	hits := map[string]int{}
	seen := map[string]bool{}
	for _, c := range caps {
		for _, s := range []string{c.Prompt, c.Response, string(c.ToolTrace)} {
			if s == "" || s == "[]" || seen[s] {
				continue
			}
			seen[s] = true
			k, err := in.Scanner.Scan(ctx, s)
			if err != nil {
				return nil, err
			}
			for kind, v := range k {
				if v > 0 {
					hits[kind] += v
				}
			}
		}
	}
	return hits, nil
}

func kindList(kinds map[string]int) string {
	keys := make([]string, 0, len(kinds))
	for k := range kinds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, kinds[k])
	}
	return strings.Join(parts, ",")
}

// quarantine appends the line, with why (kinds and counts, never the
// values), to dir/<file>; dir 0700, file 0600.
func quarantine(dir, file string, n int, id string, kinds map[string]int, line []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// #nosec G302 -- a directory needs its x bit; 0700 is owner-only
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	entry, err := json.Marshal(map[string]any{"file": file, "line": n, "record_id": id, "kinds": kinds,
		"record": json.RawMessage(line)})
	if err != nil {
		return err
	}
	p := filepath.Join(dir, file)
	// #nosec G304 -- fixed name inside the private quarantine dir.
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	_, err = f.Write(append(entry, '\n'))
	return err
}

// moveTo renames f into dir (0700), never over an existing file: a
// second copy of a name gets .1, .2, ...
func moveTo(dir, f string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	dst := filepath.Join(dir, filepath.Base(f))
	for i := 1; ; i++ {
		if _, err := os.Lstat(dst); errors.Is(err, os.ErrNotExist) {
			break
		}
		dst = filepath.Join(dir, fmt.Sprintf("%s.%d", filepath.Base(f), i))
	}
	return os.Rename(f, dst)
}
