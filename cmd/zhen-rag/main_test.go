// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClipContent_NoOpsUnderLimit(t *testing.T) {
	t.Parallel()
	if got := clipContent("hello", 10); got != "hello" {
		t.Errorf("under-limit clip mutated: %q", got)
	}
}

func TestClipContent_NoOpsOnNonPositiveMax(t *testing.T) {
	t.Parallel()
	for _, max := range []int{0, -1, -100} {
		if got := clipContent("hello", max); got != "hello" {
			t.Errorf("max=%d should not clip; got %q", max, got)
		}
	}
}

func TestClipContent_TruncatesWithMarker(t *testing.T) {
	t.Parallel()
	want := "hell\n\n…[truncated]"
	if got := clipContent("hello world", 4); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEnvOr_Fallback(t *testing.T) {
	if got := envOr("ZHEN_RAG_NEVER_SET", "fb"); got != "fb" {
		t.Errorf("unset: got %q, want fb", got)
	}
}

func TestEnvOr_TrimsWhitespace(t *testing.T) {
	t.Setenv("ZHEN_RAG_TEST", "   ")
	if got := envOr("ZHEN_RAG_TEST", "fb"); got != "fb" {
		t.Errorf("whitespace-only should fall back, got %q", got)
	}
}

func TestEnvOr_UsesValue(t *testing.T) {
	t.Setenv("ZHEN_RAG_TEST", "real")
	if got := envOr("ZHEN_RAG_TEST", "fb"); got != "real" {
		t.Errorf("got %q, want real", got)
	}
}

// The code-comment reminder and worked example come from pkg/zhenrag, and the
// web UI (raft/scripts/zhen_rag.py, the live chat path) reads them from the
// module generated from the same source; both must stay byte-identical to
// what this binary sends. Python reads the generated values with runpy, so
// the check needs only python3, not zhen_rag's dependencies (requests,
// sentence-transformers).
const pyGenerated = "../../raft/scripts/zhen_prompt_generated.py"

func pythonPromptValues(t *testing.T) (prompt, reminder string, example []chatMessage) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
	const script = `
import json, runpy, sys
ns = runpy.run_path(sys.argv[1])
print(json.dumps({"prompt": ns["DEFAULT_SYSTEM_PROMPT"], "reminder": ns["CODE_COMMENT_REMINDER"], "example": ns["CODE_COMMENT_EXAMPLE"]}))
`
	out, err := exec.Command("python3", "-I", "-c", script, pyGenerated).Output()
	if err != nil {
		t.Fatalf("python3: %v", err)
	}
	var py struct {
		Prompt   string        `json:"prompt"`
		Reminder string        `json:"reminder"`
		Example  []chatMessage `json:"example"`
	}
	if err := json.Unmarshal(out, &py); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	return py.Prompt, py.Reminder, py.Example
}

func TestCodeCommentExample_MatchesPython(t *testing.T) {
	t.Parallel()
	_, reminder, example := pythonPromptValues(t)
	if reminder != codeCommentReminder {
		t.Errorf("reminder differs:\n go: %q\n py: %q", codeCommentReminder, reminder)
	}
	if !reflect.DeepEqual(example, codeCommentExample) {
		t.Errorf("example differs:\n go: %q\n py: %q", codeCommentExample, example)
	}
}

// The web prompt is the chat prompt with the web persona's identity
// paragraph in place of "You are zhen, an assistant for the Unheaded
// Kingdom.": everything from the first rule on is the same bytes.
func TestPrompt_ByteIdentity_GoVsPython(t *testing.T) {
	t.Parallel()
	pyPrompt, _, _ := pythonPromptValues(t)
	goPrompt, err := resolveSystemPrompt("")
	if err != nil {
		t.Fatal(err)
	}
	const firstRule = "For Unheaded-specific facts"
	gi, pi := strings.Index(goPrompt, firstRule), strings.Index(pyPrompt, firstRule)
	if gi < 0 || pi < 0 {
		t.Fatalf("first rule missing: go@%d py@%d", gi, pi)
	}
	if goPrompt[gi:] != pyPrompt[pi:] {
		t.Error("chat prompt body differs between cmd/zhen-rag and the web UI")
	}
}

// What this binary sends by default is byte-identical to its prompt before
// the move into pkg/zhenrag (golden captured from the old literal).
func TestResolveSystemPrompt(t *testing.T) {
	t.Parallel()
	want, err := os.ReadFile("../../pkg/zhenrag/testdata/prompts/zhen-rag.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := resolveSystemPrompt(""); err != nil || got != string(want) {
		t.Errorf("default prompt drifted from the pre-move golden (err=%v)", err)
	}
	if got, err := resolveSystemPrompt("-"); err != nil || got != "" {
		t.Errorf(`"-" = %q, %v; want no system message`, got, err)
	}
	f := filepath.Join(t.TempDir(), "p.txt")
	if err := os.WriteFile(f, []byte("custom"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveSystemPrompt(f); err != nil || got != "custom" {
		t.Errorf("file = %q, %v", got, err)
	}
	if _, err := resolveSystemPrompt(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing file accepted")
	}
}
