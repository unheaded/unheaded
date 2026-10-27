// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"testing"
	"time"
)

// FuzzPendingCallFor: two maps with equal contents hash equal whatever
// the insertion order; changing any one byte of an argument changes the
// hash.
func FuzzPendingCallFor(f *testing.F) {
	f.Add("a.txt", "hello\n", "k", "v", 3)
	f.Add("dir/ü.go", "package x\n\nfunc F() {}\n", "mode", "0644", 0)
	f.Add("", "", "", "", 7)
	c, err := New(Config{ProjectRoot: f.TempDir()}, nil)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, path, content, k, v string, flip int) {
		a := map[string]any{"path": path, "content": content, "extra": map[string]any{"k_" + k: v, "z": 1.5}}
		b := map[string]any{}
		// Same contents, other insertion order.
		b["extra"] = map[string]any{"z": 1.5, "k_" + k: v}
		b["content"] = content
		b["path"] = path
		pa, err := c.PendingCallFor(ToolCall{Name: "write_file", Args: a}, time.Minute)
		if err != nil {
			return // too large: refused, nothing to compare
		}
		pb, err := c.PendingCallFor(ToolCall{Name: "write_file", Args: b}, time.Minute)
		if err != nil {
			t.Fatalf("equal map refused: %v", err)
		}
		if pa.SHA256 != pb.SHA256 {
			t.Fatalf("equal maps, different hashes: %s / %s", pa.Canonical, pb.Canonical)
		}
		if len(content) == 0 {
			return
		}
		i := flip % len(content)
		if i < 0 {
			i = -i
		}
		mod := []byte(content)
		mod[i] ^= 0x01
		a["content"] = string(mod)
		pm, err := c.PendingCallFor(ToolCall{Name: "write_file", Args: a}, time.Minute)
		if err != nil {
			return
		}
		if string(mod) != content && pm.SHA256 == pa.SHA256 {
			t.Fatalf("a one-byte change in content kept the hash: %q -> %q", content, mod)
		}
		// RT-03: binding a requester changes the hash; two different
		// requesters never share one.
		a["content"] = content
		pr, err := c.PendingCallFor(ToolCall{Name: "write_file", Args: a, Principal: "ui:" + k}, time.Minute)
		if err != nil {
			return
		}
		ps, err := c.PendingCallFor(ToolCall{Name: "write_file", Args: a, Principal: "tty:" + k}, time.Minute)
		if err != nil {
			return
		}
		if pr.SHA256 == pa.SHA256 || pr.SHA256 == ps.SHA256 {
			t.Fatalf("requester not bound into the hash: %s / %s", pr.Canonical, ps.Canonical)
		}
	})
}
