// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeCall(c *Champion, name, content string) ToolCall {
	return ToolCall{
		Name:      "write_file",
		Args:      map[string]any{"path": filepath.Join(c.config.ProjectRoot, name), "content": content},
		EmittedBy: "claude-api/claude-opus-5-5/conv1",
	}
}

func TestPendingCallFor_StableAcrossMapOrder(t *testing.T) {
	c, _ := newTestChampion(t)
	a := ToolCall{Name: "kanban_update", Args: map[string]any{"id": "7", "updates": map[string]any{"status": "done", "owner": "zhen", "title": "x"}}}
	b := ToolCall{Name: "kanban_update", Args: map[string]any{"updates": map[string]any{"title": "x", "owner": "zhen", "status": "done"}, "id": "7"}}
	pa, err := c.PendingCallFor(a, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		pb, err := c.PendingCallFor(b, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if pb.SHA256 != pa.SHA256 || string(pb.Canonical) != string(pa.Canonical) {
			t.Fatalf("hash differs with map order: %s vs %s", pa.Canonical, pb.Canonical)
		}
	}
	if len(pa.SHA256) != 64 || pa.ProjectRoot != c.config.ProjectRoot || pa.Tool != "kanban_update" {
		t.Errorf("pending call = %+v", pa)
	}
	if !pa.ExpiresAt.After(time.Now()) {
		t.Error("no expiry")
	}
}

func TestPendingCallFor_HashBindsToolArgsAndRoot(t *testing.T) {
	c, _ := newTestChampion(t)
	other, _ := newTestChampion(t)
	base, _ := c.PendingCallFor(writeCall(c, "a.txt", "hi"), time.Minute)
	diffArg, _ := c.PendingCallFor(writeCall(c, "a.txt", "hj"), time.Minute)
	tc := writeCall(c, "a.txt", "hi")
	tc.Name = "patch_file"
	tc.Args["old_text"], tc.Args["new_text"] = "a", "b"
	diffTool, _ := c.PendingCallFor(tc, time.Minute)
	diffRoot, _ := other.PendingCallFor(writeCall(c, "a.txt", "hi"), time.Minute)
	for name, p := range map[string]PendingCall{"arg": diffArg, "tool": diffTool, "root": diffRoot} {
		if p.SHA256 == base.SHA256 {
			t.Errorf("changing the %s left the hash unchanged", name)
		}
	}
}

func TestPendingCallFor_RefusesUnconfirmableTools(t *testing.T) {
	c, _ := newTestChampion(t)
	for _, name := range []string{"runbook_execute", "model_switch", "system_command", "delete_file", "read_file", "made_up"} {
		if _, err := c.PendingCallFor(ToolCall{Name: name, Args: map[string]any{}}, time.Minute); err == nil {
			t.Errorf("%s accepted as confirmable", name)
		}
	}
	if _, err := c.PendingCallFor(writeCall(c, "a", "b"), 0); err == nil {
		t.Error("zero TTL accepted")
	}
}

func TestConfirmCall_RunsTheBoundCall(t *testing.T) {
	c, store := newTestChampion(t)
	p, err := c.PendingCallFor(writeCall(c, "out.txt", "approved\n"), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ConfirmCall(context.Background(), p, p.SHA256, "stevie@west"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(c.config.ProjectRoot, "out.txt"))
	if err != nil || string(got) != "approved\n" {
		t.Fatalf("file = %q, %v", got, err)
	}
	var types []string
	for _, a := range store.actions {
		types = append(types, a.ActionType)
	}
	joined := strings.Join(types, ",")
	ia, ib := strings.Index(joined, "confirm_approve"), strings.Index(joined, "file.write")
	if ia < 0 || ib < 0 || ia > ib {
		t.Errorf("actions = %v; want confirm_approve, then the tool's own file.write row", types)
	}
	for _, a := range store.actions {
		if a.ActionType == "confirm_approve" && a.TriggeredBy != "stevie@west" {
			t.Errorf("approval recorded as %q", a.TriggeredBy)
		}
	}
}

func TestConfirmCall_RefusesTamper(t *testing.T) {
	c, _ := newTestChampion(t)
	p, _ := c.PendingCallFor(writeCall(c, "out.txt", "benign"), time.Minute)
	// The stored args are swapped after the hash was taken.
	p.Args["content"] = "evil"
	if _, err := c.ConfirmCall(context.Background(), p, p.SHA256, "stevie"); err == nil {
		t.Fatal("a call whose args no longer match its hash ran")
	}
	// A wrong hash for honest args.
	q, _ := c.PendingCallFor(writeCall(c, "out.txt", "benign"), time.Minute)
	if _, err := c.ConfirmCall(context.Background(), q, strings.Repeat("0", 64), "stevie"); err == nil {
		t.Fatal("a mismatched hash ran")
	}
	if _, err := os.Stat(filepath.Join(c.config.ProjectRoot, "out.txt")); !os.IsNotExist(err) {
		t.Error("the file was written")
	}
}

func TestConfirmCall_RefusesOtherRoot(t *testing.T) {
	a, _ := newTestChampion(t)
	b, _ := newTestChampion(t)
	p, _ := a.PendingCallFor(writeCall(a, "out.txt", "x"), time.Minute)
	if _, err := b.ConfirmCall(context.Background(), p, p.SHA256, "stevie"); err == nil {
		t.Fatal("an approval for root A ran under root B")
	}
}

func TestConfirmCall_RefusesExpired(t *testing.T) {
	c, _ := newTestChampion(t)
	p, _ := c.PendingCallFor(writeCall(c, "out.txt", "x"), time.Minute)
	p.ExpiresAt = time.Now().Add(-time.Second)
	if _, err := c.ConfirmCall(context.Background(), p, p.SHA256, "stevie"); err == nil {
		t.Fatal("an expired call ran")
	}
}

func TestConfirmCall_GatesStillHold(t *testing.T) {
	c, _ := newTestChampion(t)
	destructive, _ := c.PendingCallFor(writeCall(c, "x.sh", "rm -rf /"), time.Minute)
	if _, err := c.ConfirmCall(context.Background(), destructive, destructive.SHA256, "stevie"); err == nil {
		t.Error("Rule 3: a destructive verb ran after approval")
	}
	outside := writeCall(c, "x", "y")
	outside.Args["path"] = "/etc/unheaded-test-outside"
	po, _ := c.PendingCallFor(outside, time.Minute)
	if _, err := c.ConfirmCall(context.Background(), po, po.SHA256, "stevie"); err == nil {
		t.Error("Rule 1: an out-of-sandbox path ran after approval")
	}
	denied := writeCall(c, ".env", "K=V")
	pd, _ := c.PendingCallFor(denied, time.Minute)
	if _, err := c.ConfirmCall(context.Background(), pd, pd.SHA256, "stevie"); err == nil {
		t.Error("Rule 1: a denied path ran after approval")
	}
}

func TestConfirmCall_RefusesSymlinkOutOfSandbox(t *testing.T) {
	c, _ := newTestChampion(t)
	outside := t.TempDir()
	link := filepath.Join(c.config.ProjectRoot, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	p, _ := c.PendingCallFor(writeCall(c, "escape/pwned.txt", "x"), time.Minute)
	if _, err := c.ConfirmCall(context.Background(), p, p.SHA256, "stevie"); err == nil {
		t.Error("a write through a symlink out of the sandbox ran")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); !os.IsNotExist(err) {
		t.Error("the file landed outside the sandbox")
	}
}

func TestCanonicalCall_IsSortedJSON(t *testing.T) {
	c, _ := newTestChampion(t)
	p, _ := c.PendingCallFor(ToolCall{Name: "kanban_create", Args: map[string]any{"task": map[string]any{"title": "t", "owner": "zhen"}}}, time.Minute)
	var m map[string]any
	if err := json.Unmarshal(p.Canonical, &m); err != nil {
		t.Fatal(err)
	}
	if m["tool"] != "kanban_create" || m["project_root"] != c.config.ProjectRoot {
		t.Errorf("canonical = %s", p.Canonical)
	}
}
