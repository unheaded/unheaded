// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

//go:build linux

package champion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"unheaded/pkg/zhenrag"
)

const teachSecret = "hunter2hunter2"

// teachChampion is a Champion with a taught directory beside its root, as
// `zhen-claude confirm approve` builds one.
func teachChampion(t *testing.T) (*Champion, *mockStore, string) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "projects", "default")
	taught := filepath.Join(base, "projects", "zhen-taught")
	for _, d := range []string{root, taught} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	store := &mockStore{}
	c, err := New(Config{ProjectRoot: root, AllowedPaths: []string{root}}, store)
	if err != nil {
		t.Fatal(err)
	}
	return c.WithTaughtDir(taught), store, taught
}

func teachArgs(title, text string) map[string]any {
	return map[string]any{"title": title, "text": text}
}

func teachPending(t *testing.T, c *Champion, args map[string]any) PendingCall {
	t.Helper()
	p, err := c.PendingCallFor(ToolCall{Name: teachTool, Args: args, EmittedBy: "zhen-cli"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func teachApprove(t *testing.T, c *Champion, args map[string]any) (any, error) {
	t.Helper()
	p := teachPending(t, c, args)
	return c.ConfirmCall(context.Background(), p, p.SHA256, QueuePrincipal)
}

func taughtFiles(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		out = append(out, e.Name())
	}
	return out
}

func wantNoTaughtFiles(t *testing.T, dir string) {
	t.Helper()
	if got := taughtFiles(t, dir); len(got) != 0 {
		t.Fatalf("files written: %v", got)
	}
}

// Invariants 1 and 2: every label is held, the strongest included.
func TestTeach_EveryTrustLabelHeldPending(t *testing.T) {
	c, _, taught := teachChampion(t)
	for _, trust := range []string{"canonical", "local", "direct-user", "external", "unavailable"} {
		_, err := c.Dispatch(context.Background(), ToolCall{Name: teachTool, Args: teachArgs("a note", "body"),
			Justification: []Reference{{Topic: "x", SourceTrust: trust, Digest: "d"}}, EmittedBy: "zhen-cli", Principal: "ui:stevie"})
		if !pendingRefusal(err) {
			t.Errorf("%s: want a pending refusal, got %v", trust, err)
		}
	}
	if _, err := c.Dispatch(context.Background(), ToolCall{Name: teachTool, Args: teachArgs("a note", "body")}); !pendingRefusal(err) {
		t.Errorf("empty chain: want a pending refusal, got %v", err)
	}
	wantNoTaughtFiles(t, taught)
}

func TestTeach_BranchSandboxDoesNotAnswerRuleTwo(t *testing.T) {
	c, _, _, agent := sandboxChampion(t, "zhen/card-7")
	if err := c.WithBranchSandbox(agent); err != nil {
		t.Fatal(err)
	}
	_, err := c.Dispatch(context.Background(), ToolCall{Name: teachTool, Args: teachArgs("a note", "body"), EmittedBy: "zhen-agent"})
	if !pendingRefusal(err) {
		t.Fatalf("want a pending refusal, got %v", err)
	}
}

// Invariant 1: no token route, and only the queue principal approves.
func TestTeach_QueueOnly(t *testing.T) {
	c, _, taught := teachChampion(t)
	ctx := context.Background()
	tc := ToolCall{Name: teachTool, Args: teachArgs("a note", "body"), EmittedBy: "zhen-cli", Principal: "ui:stevie"}

	if tok, err := c.IssuePendingConfirmation(tc); !errors.Is(err, ErrQueueOnly) || tok != "" {
		t.Fatalf("IssuePendingConfirmation = %q, %v", tok, err)
	}
	tok, err := c.confirms().IssuePending(tc) // forced past the refusal
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ConfirmPendingToolCall(ctx, tok, "ui:stevie"); !errors.Is(err, ErrQueueOnly) {
		t.Fatalf("forced token: %v", err)
	}
	wantNoTaughtFiles(t, taught)

	for _, requester := range []string{"", "ui:stevie", "tty:stevie"} {
		call := tc
		call.Principal = requester
		p, err := c.PendingCallFor(call, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		for _, by := range []string{"ui:stevie", "tty:stevie", "agent:model"} {
			if _, err := c.ConfirmCall(ctx, p, p.SHA256, by); !errors.Is(err, ErrQueueOnly) {
				t.Errorf("requester %q, confirmer %q: %v", requester, by, err)
			}
		}
	}
	wantNoTaughtFiles(t, taught)

	if _, err := teachApprove(t, c, teachArgs("a note", "body")); err != nil {
		t.Fatal(err)
	}
	if got := taughtFiles(t, taught); len(got) != 1 {
		t.Fatalf("files: %v", got)
	}
}

// The daemon has no taught dir: the gate still holds the call, and only
// the approver's process can run it.
func TestTeach_NoTaughtDir(t *testing.T) {
	c, err := New(Config{ProjectRoot: t.TempDir()}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	args := teachArgs("a note", "body")
	if _, err := c.Dispatch(context.Background(), ToolCall{Name: teachTool, Args: args}); !pendingRefusal(err) {
		t.Fatalf("want a pending refusal, got %v", err)
	}
	if _, err := teachApprove(t, c, args); !errors.Is(err, ErrNoTaughtDir) {
		t.Fatalf("approve: %v", err)
	}
}

// Invariant 3: refused at the gate and again in the body, by every form.
func TestTeach_SecretsRefusedByGateAndBody(t *testing.T) {
	token := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	for name, args := range map[string]map[string]any{
		"plain":           teachArgs("note", "password="+teachSecret),
		"in-title":        teachArgs("password="+teachSecret, "body"),
		"joined":          teachArgs("password", "="+teachSecret),
		"token-cut":       teachArgs(token[:12], token[12:]),
		"no-break-spaces": teachArgs("note", "password = "+teachSecret),
		"fullwidth-equal": teachArgs("note", "password＝"+teachSecret),
	} {
		t.Run(name, func(t *testing.T) {
			c, store, taught := teachChampion(t)
			_, err := c.Dispatch(context.Background(), ToolCall{Name: teachTool, Args: args, EmittedBy: "zhen-cli"})
			if err == nil {
				t.Fatal("the gate let it through")
			}
			var ge *GateError
			if !errors.As(err, &ge) || ge.PendingConfirmation() {
				t.Fatalf("want a hard refusal, got %v", err)
			}
			if _, err := teachApprove(t, c, args); err == nil {
				t.Fatal("the body wrote a forged queue entry")
			}
			wantNoTaughtFiles(t, taught)
			for _, a := range store.actions {
				for _, s := range []string{a.Intent, a.ResultSummary, a.Error} {
					if strings.Contains(s, teachSecret) || strings.Contains(s, token[:12]) {
						t.Fatalf("the audit row repeats an argument: %+v", a)
					}
				}
			}
			if strings.Contains(err.Error(), teachSecret) || strings.Contains(err.Error(), token[:12]) {
				t.Fatalf("the error repeats an argument: %v", err)
			}
		})
	}
}

// Invariant 10.
func TestTeach_InvisibleRunesRefused(t *testing.T) {
	for name, r := range map[string]string{
		"tag":             "\U000E0068",
		"zero-width-join": "‍",
		"rtl-override":    "‮",
		"soft-hyphen":     "­",
		"bom":             "\ufeff",
		"selector-plane":  "\U000E0100",
		"private-use":     "",
		"private-plane":   "\U000F0000",
	} {
		t.Run(name, func(t *testing.T) {
			for _, args := range []map[string]any{teachArgs("no"+r+"te", "body"), teachArgs("note", "bo"+r+"dy")} {
				if _, _, err := teachCheck(args); err == nil {
					t.Errorf("accepted %+q", args)
				}
			}
		})
	}
}

func TestTeach_RefusedShapes(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"title over cap":    teachArgs(strings.Repeat("a", MaxTeachTitleBytes+1), "body"),
		"text over cap":     teachArgs("note", strings.Repeat("a", MaxTeachTextBytes+1)),
		"invalid title":     teachArgs("no\xfete", "body"),
		"invalid text":      teachArgs("note", "bo\xffdy"),
		"control in title":  teachArgs("no\x1bte", "body"),
		"newline in title":  teachArgs("no\nte", "body"),
		"tab in title":      teachArgs("no\tte", "body"),
		"control in text":   teachArgs("note", "bo\x1bdy"),
		"c1 in text":        teachArgs("note", "bo\u0085dy"),
		"nul in text":       teachArgs("note", "bo\x00dy"),
		"empty text":        teachArgs("note", ""),
		"blank text":        teachArgs("note", " \n\t "),
		"empty title":       teachArgs("", "body"),
		"dots title":        teachArgs("...", "body"),
		"non-ascii title":   teachArgs("日本語", "body"),
		"a path":            {"title": "note", "text": "body", "path": "/etc/x"},
		"no text":           {"title": "note"},
		"text not a string": {"title": "note", "text": 7},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := teachCheck(args); err == nil {
				t.Fatalf("accepted %+v", args)
			}
		})
	}
	// Tabs and line ends are ordinary text.
	if _, _, err := teachCheck(teachArgs("note", "a\tb\r\nc\n")); err != nil {
		t.Fatal(err)
	}
}

func TestTeach_DestructiveVerbRefusedByRuleThree(t *testing.T) {
	c, _, taught := teachChampion(t)
	args := teachArgs("note", "then run rm -rf / on it")
	_, err := c.Dispatch(context.Background(), ToolCall{Name: teachTool, Args: args})
	var ge *GateError
	if !errors.As(err, &ge) || ge.PendingConfirmation() {
		t.Fatalf("want a hard refusal, got %v", err)
	}
	if _, err := teachApprove(t, c, args); err == nil {
		t.Fatal("approved a destructive text")
	}
	wantNoTaughtFiles(t, taught)
}

// Invariant 4.
func TestTeach_ChangedArgFailsTheHash(t *testing.T) {
	c, _, taught := teachChampion(t)
	p := teachPending(t, c, teachArgs("a note", "body"))
	bad := p
	bad.Args = teachArgs("a note", "changed")
	if _, err := c.ConfirmCall(context.Background(), bad, p.SHA256, QueuePrincipal); err == nil {
		t.Fatal("changed text ran")
	}
	wantNoTaughtFiles(t, taught)
}

// Invariant 5.
func TestTeach_FileName(t *testing.T) {
	today := time.Now().UTC().Format("20060102")
	for title, want := range map[string]string{
		"../../etc/x":                "etc-x",
		"Hello,  World!":             "hello-world",
		"--a--b--":                   "a-b",
		"café 2":                     "caf-2",
		strings.Repeat("ab ", 80):    strings.TrimSuffix(strings.Repeat("ab-", 27), "-"),
		strings.Repeat("a", 256):     strings.Repeat("a", maxTeachSlugBytes),
		"x" + strings.Repeat("/", 9): "x",
	} {
		c, _, taught := teachChampion(t)
		if _, err := teachApprove(t, c, teachArgs(title, "body")); err != nil {
			t.Fatalf("%q: %v", title, err)
		}
		got := taughtFiles(t, taught)
		if len(got) != 1 || got[0] != today+"-"+want+".md" {
			t.Errorf("%q: files %v, want %s-%s.md", title, got, today, want)
		}
	}
}

func TestTeach_FileBody(t *testing.T) {
	c, _, taught := teachChampion(t)
	text := "line one\r\nline\ttwo\n"
	if _, err := teachApprove(t, c, teachArgs("A Title", text)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(taught, taughtFiles(t, taught)[0]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# A Title\n\n"+text) {
		t.Fatalf("head: %q", data)
	}
	if !regexp.MustCompile(`(?m)^<!-- teach_write text-sha256:[0-9a-f]{64} -->$`).Match(data) {
		t.Fatalf("no digest line: %q", data)
	}
}

func TestTeach_ExistingFileNotOverwritten(t *testing.T) {
	c, _, taught := teachChampion(t)
	name := time.Now().UTC().Format("20060102") + "-a-note.md"
	if err := os.WriteFile(filepath.Join(taught, name), []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := teachApprove(t, c, teachArgs("a note", "second")); !errors.Is(err, os.ErrExist) {
		t.Fatalf("want ErrExist, got %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(taught, name)); string(data) != "first" {
		t.Fatalf("overwritten: %q", data)
	}
}

func TestTeach_SymlinkPlantedBeforeTheOpenRefused(t *testing.T) {
	for name, target := range map[string]func(string) string{
		"live":     func(dir string) string { return filepath.Join(dir, "victim") },
		"dangling": func(dir string) string { return filepath.Join(dir, "nothing-here") },
	} {
		t.Run(name, func(t *testing.T) {
			c, _, taught := teachChampion(t)
			outside := t.TempDir()
			victim := filepath.Join(outside, "victim")
			if err := os.WriteFile(victim, []byte("safe"), 0o600); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(taught, time.Now().UTC().Format("20060102")+"-a-note.md")
			c.beforeOpen = func() {
				if err := os.Symlink(target(outside), file); err != nil {
					t.Error(err)
				}
			}
			if _, err := teachApprove(t, c, teachArgs("a note", "body")); err == nil {
				t.Fatal("wrote through a planted symlink")
			}
			if data, _ := os.ReadFile(victim); string(data) != "safe" {
				t.Fatalf("victim changed: %q", data)
			}
			if _, err := os.Stat(filepath.Join(outside, "nothing-here")); err == nil {
				t.Fatal("the dangling target was created")
			}
		})
	}
}

func TestTeach_MissingDirNotCreated(t *testing.T) {
	c, _, taught := teachChampion(t)
	gone := filepath.Join(taught, "gone")
	c = c.WithTaughtDir(gone)
	if _, err := teachApprove(t, c, teachArgs("a note", "body")); err == nil {
		t.Fatal("approved with no directory")
	}
	if _, err := os.Stat(gone); err == nil {
		t.Fatal("the directory was created")
	}
}

func TestTeach_FailedWriteIsRemoved(t *testing.T) {
	c, store, taught := teachChampion(t)
	c.afterCreate = func(f *os.File) { _ = f.Close() } // the write then fails
	if _, err := teachApprove(t, c, teachArgs("a note", "body")); err == nil {
		t.Fatal("no error from a failed write")
	}
	wantNoTaughtFiles(t, taught)
	if last := store.actions[len(store.actions)-1]; last.ActionType != "corpus.teach" || last.Status != "failed" {
		t.Fatalf("audit row: %+v", last)
	}
}

// Invariant 7.
func TestTeach_ModeIs0640WhateverTheUmask(t *testing.T) {
	for _, mask := range []int{0o077, 0o000, 0o022} {
		c, _, taught := teachChampion(t)
		old := syscall.Umask(mask)
		_, err := teachApprove(t, c, teachArgs("a note", "body"))
		syscall.Umask(old)
		if err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(filepath.Join(taught, taughtFiles(t, taught)[0]))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o640 {
			t.Errorf("umask %o: mode %o", mask, st.Mode().Perm())
		}
	}
}

func TestTeach_FileTakesASetgidDirectorysGroup(t *testing.T) {
	groups, err := os.Getgroups()
	if err != nil {
		t.Skip(err)
	}
	gid := -1
	for _, g := range groups {
		if g != os.Getegid() {
			gid = g
			break
		}
	}
	if gid < 0 {
		t.Skip("needs a secondary group")
	}
	c, _, taught := teachChampion(t)
	if err := os.Chown(taught, -1, gid); err != nil {
		t.Skip(err)
	}
	if err := os.Chmod(taught, 0o750|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	if _, err := teachApprove(t, c, teachArgs("a note", "body")); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(taught, taughtFiles(t, taught)[0]))
	if err != nil {
		t.Fatal(err)
	}
	if got := int(st.Sys().(*syscall.Stat_t).Gid); got != gid {
		t.Fatalf("group %d, want %d", got, gid)
	}
}

// Invariant 8.
func TestTeach_FileToolsRefusedInTaughtDir(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "default")
	taught := filepath.Join(base, "lessons") // no zhen-taught in the name: only WithTaughtDir denies it
	for _, d := range []string{root, taught} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(taught, filepath.Join(root, "via")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(taught, "20261104-a.md")
	if err := os.WriteFile(file, []byte("approved"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{ProjectRoot: root, AllowedPaths: []string{base}}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	c = c.WithTaughtDir(filepath.Join(root, "via")) // the resolved path is denied too
	du := []Reference{{Topic: "x", SourceTrust: "direct-user"}}
	for name, args := range map[string]map[string]any{
		"write_file": {"path": file, "content": "evil"},
		"patch_file": {"path": file, "old_text": "approved", "new_text": "evil"},
		"read_file":  {"path": file},
	} {
		if _, err := c.Dispatch(context.Background(), ToolCall{Name: name, Args: args, Justification: du, Principal: "ui:s"}); err == nil {
			t.Errorf("%s reached the taught dir", name)
		}
	}
	if data, _ := os.ReadFile(file); string(data) != "approved" {
		t.Fatalf("taught file changed: %q", data)
	}
}

// A directory reached by no symlink, under a name no default denies.
func TestTeach_PlainTaughtDirDenied(t *testing.T) {
	base := t.TempDir()
	taught := filepath.Join(base, "lessons")
	if err := os.Mkdir(taught, 0o700); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{ProjectRoot: base}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.validatePath(filepath.Join(taught, "x.md")); err != nil {
		t.Fatalf("before WithTaughtDir: %v", err)
	}
	if err := c.WithTaughtDir(taught).validatePath(filepath.Join(taught, "x.md")); err == nil {
		t.Fatal("validatePath allows the taught dir")
	}
}

func TestTeach_DefaultChampionDeniesTheCorpusStores(t *testing.T) {
	c, err := New(Config{ProjectRoot: t.TempDir()}, &mockStore{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		"/var/zhen/projects/zhen-taught/x.md",
		"/var/zhen/code-index/CURRENT",
		"/var/zhen/vor-source/unheaded/docs/x.md",
	} {
		if err := c.validatePath(p); err == nil {
			t.Errorf("validatePath allows %s", p)
		}
	}
}

func TestTeach_BranchSandboxStaysInTheWorktree(t *testing.T) {
	c, _, wt, agent := sandboxChampion(t, "zhen/card-7")
	if err := c.WithBranchSandbox(agent); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	inside := filepath.Join(wt, "lib.go")
	if _, err := c.Dispatch(ctx, ToolCall{Name: "write_file", Args: map[string]any{"path": inside, "content": "package x\n"}, EmittedBy: "zhen-agent"}); err != nil {
		t.Fatalf("inside the worktree: %v", err)
	}
	for _, p := range []string{
		"/var/zhen/notes.md",
		wt + "-sibling/x.go",
		filepath.Join(agent, "other", "x.go"),
		wt,
	} {
		if _, ok := c.branchSandboxAllows(ToolCall{Name: "write_file", Args: map[string]any{"path": p, "content": "x"}}); ok {
			t.Errorf("sandbox answers Rule 2 for %s", p)
		}
		d, _ := c.AcceptToolCall(ctx, ToolCall{Name: "write_file", Args: map[string]any{"path": p, "content": "x"}, EmittedBy: "zhen-agent"})
		if d.Allow {
			t.Errorf("no confirmation needed for %s", p)
		}
	}
}

// zhen_actions.triggered_by is VARCHAR(20).
func TestTeach_AuditRowFits(t *testing.T) {
	c, store, _ := teachChampion(t)
	if _, err := teachApprove(t, c, teachArgs("a note", "body")); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range store.actions {
		if len(a.TriggeredBy) > 20 && a.ActionType == "corpus.teach" {
			t.Errorf("triggered_by %q is over 20", a.TriggeredBy)
		}
		if a.ActionType == "corpus.teach" {
			found = true
			if a.Status != "completed" || strings.Contains(a.Intent, "body") {
				t.Errorf("row: %+v", a)
			}
		}
	}
	if !found {
		t.Fatal("no corpus.teach row")
	}
}

func TestTeach_Registration(t *testing.T) {
	for name, set := range map[string]map[string]bool{
		"MutatingTools": MutatingTools, "ConfirmableTools": ConfirmableTools, "AlwaysConfirmTools": AlwaysConfirmTools,
		"QueueOnlyTools": QueueOnlyTools, "dispatchable": dispatchable,
	} {
		if !set[teachTool] {
			t.Errorf("%s lacks %s", name, teachTool)
		}
	}
	c, err := New(Config{ProjectRoot: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewToolProvider(c, "x")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := p.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.Name == teachTool {
			t.Error("offered to models")
		}
	}
	if zhenrag.DestructivePattern.MatchString(teachTool) {
		t.Error("named in the verb table")
	}
}

func TestTeach_FoldForChecksOnly(t *testing.T) {
	if got := teachFold("a b　c\nd＝eＡ"); got != "a b c d=eA" {
		t.Fatalf("%q", got)
	}
}
