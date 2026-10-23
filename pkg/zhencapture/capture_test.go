// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package zhencapture

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Fake secrets are assembled at run time so no secret-shaped literal sits
// in the tree for gitleaks to (rightly) flag.
func fake(prefix string, n int, fill string) string {
	return prefix + strings.Repeat(fill, n)
}

func TestRedact(t *testing.T) {
	pem := "-----BEGIN " + "RSA PRIVATE KEY-----\nMIIEow" + strings.Repeat("A", 40) + "\n-----END " + "RSA PRIVATE KEY-----"
	cases := []struct {
		name, in  string
		gone      []string // must not survive
		keep      []string // must survive
		wantCount int
	}{
		{"anthropic", "key " + fake("sk-ant-api03-", 30, "x") + " end", []string{"sk-ant-api03-xxx"}, []string{"key ", " end", "[REDACTED:anthropic_key]"}, 1},
		{"openai", "OPENAI=" + fake("sk-proj-", 30, "Z"), []string{"ZZZZZZZZ"}, []string{"[REDACTED:openai_key]"}, 1},
		{"aws access key", "id " + fake("AKIA", 16, "Q") + ".", []string{"AKIAQQQ"}, []string{"[REDACTED:aws_access_key]."}, 1},
		{"aws secret", "aws_secret_access_key = " + fake("", 40, "k"), []string{"kkkkkkkk"}, []string{"aws_secret_access_key = [REDACTED:aws_secret]"}, 1},
		{"pem block", "before\n" + pem + "\nafter", []string{"MIIEow", "AAAA"}, []string{"before\n[REDACTED:private_key]\nafter"}, 1},
		{"pem unterminated", "x " + "-----BEGIN " + "PRIVATE KEY-----\nMIIE" + strings.Repeat("B", 30), []string{"MIIE", "BBBB"}, []string{"x [REDACTED:private_key]"}, 1},
		{"github", "tok " + fake("ghp_", 36, "a"), []string{"ghp_aaaa"}, []string{"[REDACTED:github_token]"}, 1},
		{"slack", fake("xoxb-", 20, "1"), []string{"xoxb-111"}, []string{"[REDACTED:slack_token]"}, 1},
		{"jwt", "eyJ" + fake("", 12, "h") + ".eyJ" + fake("", 12, "p") + "." + fake("", 12, "s"), []string{"eyJhhh"}, []string{"[REDACTED:jwt]"}, 1},
		{"bearer", "Authorization: Bearer " + fake("", 24, "t"), []string{"tttttttt"}, []string{"Authorization: Bearer [REDACTED:bearer]"}, 1},
		{"url password", "postgres://app_zhen:" + fake("", 12, "w") + "@west:5432/db", []string{"wwwwww"}, []string{"postgres://app_zhen:[REDACTED:url_password]@west:5432/db"}, 1},
		{"password assignment", `PGPASSWORD="` + fake("", 10, "p") + `" psql`, []string{"pppppp"}, []string{`PGPASSWORD="[REDACTED:assignment]" psql`}, 1},
		{"yaml secret", "client_secret: " + fake("", 10, "c"), []string{"cccccc"}, []string{"client_secret: [REDACTED:assignment]"}, 1},
		{"plain prose untouched", "the password policy is strict; see secrets_manager.go", nil, []string{"the password policy is strict; see secrets_manager.go"}, 0},
		{"short value untouched", "password=abc", nil, []string{"password=abc"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, n := Redact(c.in)
			for _, g := range c.gone {
				if strings.Contains(got, g) {
					t.Errorf("%q survived: %q", g, got)
				}
			}
			for _, k := range c.keep {
				if !strings.Contains(got, k) {
					t.Errorf("want %q in %q", k, got)
				}
			}
			if n != c.wantCount {
				t.Errorf("count = %d, want %d (%q)", n, c.wantCount, got)
			}
		})
	}
}

func TestRedactIsIdempotent(t *testing.T) {
	in := "password=" + fake("", 12, "q") + " and " + fake("sk-ant-", 20, "r")
	once, _ := Redact(in)
	twice, n := Redact(once)
	if twice != once || n != 0 {
		t.Errorf("second pass changed %q -> %q (%d)", once, twice, n)
	}
}

func TestPatternsFileLoads(t *testing.T) {
	if _, err := loadPatterns([]byte(`{"version":2,"patterns":[]}`)); err == nil {
		t.Error("version 2 accepted")
	}
	if _, err := loadPatterns([]byte(`{"version":1,"patterns":[{"name":"x","regex":"(?<=a)b"}]}`)); err == nil {
		t.Error("lookbehind (not RE2) accepted")
	}
	if len(patterns) < 10 {
		t.Errorf("only %d patterns", len(patterns))
	}
}

func privateDir(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), "cap")
	return d
}

func TestCheckDirRefusesGitWorkTree(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{repo, filepath.Join(repo, "a", "b")} {
		if _, err := CheckDir(d); err == nil || !strings.Contains(err.Error(), "git work tree") {
			t.Errorf("%s: err = %v", d, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, "a")); !os.IsNotExist(err) {
		t.Errorf("refused dir was created anyway: %v", err)
	}
	// A symlink from outside into the repo is resolved and refused too.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckDir(filepath.Join(link, "cap")); err == nil {
		t.Error("symlink into a work tree accepted")
	}
	// This package's own source tree is a git work tree.
	wd, _ := os.Getwd()
	if gitWorkTree(wd) != "" {
		if _, err := CheckDir(filepath.Join(wd, "testdata-capture")); err == nil {
			t.Error("dir inside this repo accepted")
		}
	}
}

func TestCheckDirPermissions(t *testing.T) {
	d := privateDir(t)
	got, err := CheckDir(d)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(got)
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("created with %04o", fi.Mode().Perm())
	}
	if err := os.Chmod(got, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckDir(got); err == nil || !strings.Contains(err.Error(), "chmod 700") {
		t.Errorf("0755 dir: err = %v", err)
	}
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o600)
	if _, err := CheckDir(f); err == nil {
		t.Error("regular file accepted as dir")
	}
	if _, err := CheckDir(""); err == nil {
		t.Error("empty accepted")
	}
}

func readLines(t *testing.T, w *Writer) []map[string]any {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(w.Dir(), "zhen-capture-*.jsonl"))
	var out []map[string]any
	for _, p := range files {
		fi, _ := os.Stat(p)
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %04o", p, fi.Mode().Perm())
		}
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(nil, 1<<22)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Fatalf("bad line %q: %v", sc.Text(), err)
			}
			out = append(out, m)
		}
		_ = f.Close()
	}
	return out
}

func TestWriteRedactsEveryStringAndCaps(t *testing.T) {
	w, err := Open(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	w.MaxStringBytes = 200
	w.now = func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) }
	key := fake("sk-ant-", 30, "k")
	rec := &Record{
		Source: "test", Route: RouteEscalated,
		System: "sys " + key,
		Prompt: "use password=" + fake("", 12, "h"),
		ToolCalls: []ToolCall{{
			Name: "file_read", Input: json.RawMessage(`{"path":"/x","n":3,"token":"` + fake("ghp_", 36, "g") + `"}`),
			Output: strings.Repeat("o", 500),
		}},
		Local:  &Leg{Model: "qwen", Answer: "local " + key, LatencyMS: 5},
		Check:  &Check{Argv: []string{"go", "vet"}, Exit: 1},
		Claude: &Leg{Model: "claude-opus-5-5", Answer: "fine", CostUSD: "0.01"},
		Final:  "fine", AcceptedBy: "claude",
	}
	if err := w.Write(rec); err != nil {
		t.Fatal(err)
	}
	lines := readLines(t, w)
	if len(lines) != 1 {
		t.Fatalf("lines = %d", len(lines))
	}
	raw, _ := json.Marshal(lines[0])
	s := string(raw)
	for _, leak := range []string{"kkkkkkk", "hhhhhhh", "ggggggg"} {
		if strings.Contains(s, leak) {
			t.Errorf("secret %q leaked: %s", leak, s)
		}
	}
	m := lines[0]
	if m["schema"] != Schema || m["route"] != "escalated" || m["ts"] != "2026-10-23T12:00:00Z" || len(m["id"].(string)) != 32 {
		t.Errorf("header fields: %v", m)
	}
	if m["redactions"] != float64(4) || m["truncated"] != true || rec.Redactions != 4 || !rec.Truncated {
		t.Errorf("redactions=%v truncated=%v rec=%d/%v", m["redactions"], m["truncated"], rec.Redactions, rec.Truncated)
	}
	tc := m["tool_calls"].([]any)[0].(map[string]any)
	if in := tc["input"].(map[string]any); in["n"] != float64(3) || in["path"] != "/x" {
		t.Errorf("tool input = %v", in)
	}
	if out := tc["output"].(string); len(out) > 200 || !strings.HasSuffix(out, TruncationMarker) {
		t.Errorf("output not capped: %d bytes", len(out))
	}
	if _, err := os.Stat(filepath.Join(w.Dir(), "zhen-capture-2026-10-23.jsonl")); err != nil {
		t.Error(err)
	}
}

func TestWriteConcurrentLinesStayWhole(t *testing.T) {
	w, err := Open(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.Write(&Record{Source: "t", Route: RouteLocal, Prompt: strings.Repeat("p", 5000+i)}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := len(readLines(t, w)); n != 20 {
		t.Errorf("lines = %d", n)
	}
}

func TestWriteRefusesSymlinkedFile(t *testing.T) {
	w, err := Open(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	w.now = func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Symlink(target, filepath.Join(w.Dir(), "zhen-capture-2026-01-02.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(&Record{Prompt: "x"}); err == nil {
		t.Error("wrote through a symlink")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Error("symlink target created")
	}
}

func TestTruncate(t *testing.T) {
	if s, cut := Truncate("short", 100); s != "short" || cut {
		t.Error(s)
	}
	s, cut := Truncate(strings.Repeat("é", 100), 50)
	if !cut || len(s) > 50 || !strings.HasSuffix(s, TruncationMarker) {
		t.Errorf("%q", s)
	}
}

// The flywheel's linking fields (S-TWO-BRAINS Step 351) are optional:
// written when set, absent when not, so v1 readers are unaffected.
func TestWriteLinkingFields(t *testing.T) {
	w, err := Open(privateDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(&Record{Source: "test", Route: RouteLocal, Prompt: "p", Final: "a",
		ConversationID: "conv-1", RunID: "0123456789abcdef", TaskFamily: "repo-task", AuthorKind: AuthorHuman}); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(&Record{Source: "test", Route: RouteLocal, Prompt: "p2", Final: "b"}); err != nil {
		t.Fatal(err)
	}
	lines := readLines(t, w)
	if len(lines) != 2 {
		t.Fatalf("lines = %d", len(lines))
	}
	m := lines[0]
	if m["conversation_id"] != "conv-1" || m["run_id"] != "0123456789abcdef" || m["task_family"] != "repo-task" || m["author_kind"] != "human" {
		t.Errorf("linking fields: %v", m)
	}
	for _, k := range []string{"conversation_id", "run_id", "task_family", "author_kind"} {
		if _, ok := lines[1][k]; ok {
			t.Errorf("%s present on a record that did not set it", k)
		}
	}
}
