// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"os/exec"
	"reflect"
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

// The code-comment reminder and worked example live twice, here and in
// raft/scripts/zhen_rag.py (the live chat path), and must stay byte-identical.
// Python reads its own two values by parsing the file and evaluating just
// those assignments, so the check needs only python3, not zhen_rag's
// dependencies (requests, sentence-transformers).
func TestCodeCommentExample_MatchesPython(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
	const script = `
import ast, json, sys
tree = ast.parse(open(sys.argv[1], encoding="utf-8").read())
want = ("CODE_COMMENT_REMINDER", "CODE_COMMENT_EXAMPLE")
ns = {}
for node in tree.body:
    if isinstance(node, ast.Assign) and any(getattr(t, "id", "") in want for t in node.targets):
        exec(compile(ast.Module([node], []), "zhen_rag.py", "exec"), ns)
print(json.dumps({"reminder": ns["CODE_COMMENT_REMINDER"], "example": ns["CODE_COMMENT_EXAMPLE"]}))
`
	out, err := exec.Command("python3", "-I", "-c", script, "../../raft/scripts/zhen_rag.py").Output()
	if err != nil {
		t.Fatalf("python3: %v", err)
	}
	var py struct {
		Reminder string        `json:"reminder"`
		Example  []chatMessage `json:"example"`
	}
	if err := json.Unmarshal(out, &py); err != nil {
		t.Fatalf("decode %q: %v", out, err)
	}
	if py.Reminder != codeCommentReminder {
		t.Errorf("reminder differs:\n go: %q\n py: %q", codeCommentReminder, py.Reminder)
	}
	if !reflect.DeepEqual(py.Example, codeCommentExample) {
		t.Errorf("example differs:\n go: %q\n py: %q", codeCommentExample, py.Example)
	}
}
