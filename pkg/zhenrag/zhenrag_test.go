// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package zhenrag

import (
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// golden reads a prompt captured from its surface before the prompts moved
// here. Every SystemPrompt mode must reproduce its golden byte for byte: the
// consolidation moved text, it did not change it.
func golden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/prompts/" + name)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(b)
}

func TestSystemPrompt_ChatByteIdenticalToZhenRagGolden(t *testing.T) {
	t.Parallel()
	if got, want := SystemPrompt(Chat), golden(t, "zhen-rag.txt"); got != want {
		t.Errorf("Chat prompt drifted from cmd/zhen-rag's pre-move text (%d vs %d bytes)", len(got), len(want))
	}
}

func TestSystemPrompt_WebByteIdenticalToPythonGolden(t *testing.T) {
	t.Parallel()
	if got, want := SystemPrompt(Web), golden(t, "zhen_rag_py.txt"); got != want {
		t.Errorf("Web prompt drifted from zhen_rag.py's pre-move DEFAULT_SYSTEM_PROMPT (%d vs %d bytes)", len(got), len(want))
	}
}

func TestSystemPrompt_AgentIsGoldenPrefix(t *testing.T) {
	t.Parallel()
	// The agent golden is buildSystemPrompt with a fixture; the mode is the
	// part before "References:".
	g := golden(t, "agent.txt")
	i := strings.LastIndex(g, "References:")
	if i < 0 {
		t.Fatal("agent golden has no References: block")
	}
	if got := SystemPrompt(Agent); got != g[:i] {
		t.Errorf("Agent prompt drifted from pkg/agent's pre-move text (%d vs %d bytes)", len(got), i)
	}
}

func TestSystemPrompt_ContainsCodeCommentsRule(t *testing.T) {
	t.Parallel()
	for _, m := range []Mode{Chat, Web, Agent} {
		if !strings.Contains(SystemPrompt(m), codeCommentsRule) {
			t.Errorf("%s prompt lacks the CODE COMMENTS rule", m)
		}
	}
}

func TestSystemPrompt_UnknownModePanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("SystemPrompt(99) did not panic")
		}
	}()
	_ = SystemPrompt(Mode(99))
}

func TestParseMode_RoundTrip(t *testing.T) {
	t.Parallel()
	for _, m := range []Mode{Chat, Web, Agent} {
		got, err := ParseMode(m.String())
		if err != nil || got != m {
			t.Errorf("ParseMode(%q) = %v, %v", m.String(), got, err)
		}
	}
	if _, err := ParseMode("review"); err == nil {
		t.Error("ParseMode(review) accepted an unknown mode")
	}
	if s := Mode(42).String(); s != "Mode(42)" {
		t.Errorf("Mode(42).String() = %q", s)
	}
}

func TestCodeCommentReminderAndExample_MatchGoldens(t *testing.T) {
	t.Parallel()
	if got, want := CodeCommentReminder, golden(t, "code-comment-reminder.txt"); got != want {
		t.Errorf("reminder drifted:\n got %q\nwant %q", got, want)
	}
	var want []Message
	if err := json.Unmarshal([]byte(golden(t, "code-comment-example.json")), &want); err != nil {
		t.Fatal(err)
	}
	if got := CodeCommentExample(); !reflect.DeepEqual(got, want) {
		t.Errorf("example drifted:\n got %q\nwant %q", got, want)
	}
}

func TestCodeCommentExample_FreshCopy(t *testing.T) {
	t.Parallel()
	a := CodeCommentExample()
	a[0].Content = "mutated"
	if CodeCommentExample()[0].Content == "mutated" {
		t.Error("CodeCommentExample shares its backing array between callers")
	}
}

// Every seed matches the gate pattern of its own row, and every gated row
// has a seed: a verb cannot sit in the table untested.
func TestDestructivePattern_EveryRowSeedMatches(t *testing.T) {
	t.Parallel()
	for _, v := range Verbs {
		if v.Pattern == "" {
			continue
		}
		if len(v.Seeds) == 0 {
			t.Errorf("row %q has a pattern but no seed", v.Pattern)
		}
		for _, s := range v.Seeds {
			if !DestructivePattern.MatchString(s) {
				t.Errorf("seed %q (row %q) not matched", s, v.Pattern)
			}
		}
	}
}

// The prose and the gate come from the same rows: every verb a prompt
// names in a gated row is itself matched by the gate (case-insensitively),
// with arguments where the form needs them.
func TestDestructiveProse_GatedVerbsMatchPattern(t *testing.T) {
	t.Parallel()
	args := map[string]string{"rm -rf": " /x", "dd if=": "/dev/zero", "dd": " if=/dev/zero", "> /dev/": "sda", "mkfs": ".ext4"}
	for _, v := range Verbs {
		if v.Pattern == "" {
			continue
		}
		for _, name := range []string{v.Chat, v.Agent} {
			if name == "" {
				continue
			}
			s := name + args[name]
			if !DestructivePattern.MatchString(s) || !DestructivePattern.MatchString(strings.ToUpper(s)) {
				t.Errorf("prompt verb %q (as %q) is not matched by the gate", name, s)
			}
		}
	}
}

// The gate's behaviour before the move (pkg/champion/toolcall.go's
// destructivePattern) is preserved: these matched then and match now, and
// the harmless look-alikes still pass.
func TestDestructivePattern_PreMoveBehaviour(t *testing.T) {
	t.Parallel()
	hit := []string{
		"rm -rf /", "rm -f x", "rm /etc/passwd", "drop table t", "DROP DATABASE d",
		"mkfs.ext4 /dev/sda1", "dd if=/dev/zero of=x", "wipe", "shutdown now",
		"reboot", "kill -9 42", "truncate -s0 f", "unlink f",
		"git push --force", "git reset --hard", "chmod 000 f", "chmod -R 000 d",
		"cat x > /dev/sda", "echo >/dev/sdb",
	}
	miss := []string{
		"rebootstrap the cluster", "format-string bug", "rm file.txt", "delete the card",
		"ls -la", "git push origin main", "git reset --soft HEAD~1", "echo hi > /dev/null",
		"chmod 0644 f", "kill -15 42", "departed", "shredder", "fdisking",
		"dmsetup ls", "nvme list",
	}
	for _, s := range hit {
		if !DestructivePattern.MatchString(s) {
			t.Errorf("should match: %q", s)
		}
	}
	for _, s := range miss {
		if DestructivePattern.MatchString(s) {
			t.Errorf("should not match: %q", s)
		}
	}
}

func TestDestructiveVerbsProse_Shape(t *testing.T) {
	t.Parallel()
	if got := DestructiveVerbsProse([]string{"a", "b c"}); got != "`a`, `b c`" {
		t.Errorf("got %q", got)
	}
	if got := len(ChatVerbs()); got != 17 {
		t.Errorf("chat prompt names %d verbs, golden has 17", got)
	}
	if got := len(AgentVerbs()); got != 16 {
		t.Errorf("agent prompt names %d verbs, golden has 16", got)
	}
}

// The committed Python module is what the generator writes now.
func TestGenerated_PythonInSync(t *testing.T) {
	t.Parallel()
	const path = "../../raft/scripts/zhen_prompt_generated.py"
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run go generate ./pkg/zhenrag/...)", path, err)
	}
	if string(got) != string(PythonModule()) {
		t.Errorf("%s is stale: run go generate ./pkg/zhenrag/...", path)
	}
}

// Python reads the generated literals with the values Go meant. Needs only
// python3 (the module has no imports).
func TestGenerated_PythonValuesEqualGo(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
	const script = `
import json, runpy, sys
ns = runpy.run_path(sys.argv[1])
print(json.dumps({k: ns[k] for k in ("DEFAULT_SYSTEM_PROMPT", "DESTRUCTIVE_VERBS", "CODE_COMMENT_REMINDER", "CODE_COMMENT_EXAMPLE")}))
`
	out, err := exec.Command("python3", "-I", "-c", script, "../../raft/scripts/zhen_prompt_generated.py").Output()
	if err != nil {
		t.Fatalf("python3: %v", err)
	}
	var py struct {
		Prompt   string    `json:"DEFAULT_SYSTEM_PROMPT"`
		Verbs    []string  `json:"DESTRUCTIVE_VERBS"`
		Reminder string    `json:"CODE_COMMENT_REMINDER"`
		Example  []Message `json:"CODE_COMMENT_EXAMPLE"`
	}
	if err := json.Unmarshal(out, &py); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if py.Prompt != SystemPrompt(Web) {
		t.Error("Python DEFAULT_SYSTEM_PROMPT != SystemPrompt(Web)")
	}
	if !reflect.DeepEqual(py.Verbs, ChatVerbs()) {
		t.Errorf("Python DESTRUCTIVE_VERBS %q != ChatVerbs %q", py.Verbs, ChatVerbs())
	}
	if py.Reminder != CodeCommentReminder {
		t.Error("Python CODE_COMMENT_REMINDER != CodeCommentReminder")
	}
	if !reflect.DeepEqual(py.Example, CodeCommentExample()) {
		t.Error("Python CODE_COMMENT_EXAMPLE != CodeCommentExample()")
	}
}
