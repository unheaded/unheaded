// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package remediation

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func chain(t *testing.T, n int) []JournalEntry {
	t.Helper()
	var out []JournalEntry
	prev := GenesisHash
	at := time.Date(2026, 10, 1, 1, 0, 0, 0, time.UTC)
	for i := 1; i <= n; i++ {
		e := JournalEntry{
			Host: "east", Seq: int64(i), FixID: "kmod-cis", FixSHA256: strings.Repeat("a", 64),
			ModelCommit: "fb9b54e3", Action: "apply", Outcome: "success", BackupDir: "/var/lib/unheaded/remediate/x",
			Operator: "govan", At: at.Add(time.Duration(i) * time.Minute),
		}
		if err := e.Seal(prev); err != nil {
			t.Fatal(err)
		}
		prev = e.Hash
		out = append(out, e)
	}
	return out
}

func TestJournalChainVerifies(t *testing.T) {
	if err := VerifyChain(chain(t, 5), GenesisHash); err != nil {
		t.Fatal(err)
	}
}

func TestJournalChainDetectsTampering(t *testing.T) {
	for name, mut := range map[string]func([]JournalEntry) []JournalEntry{
		"edited outcome":  func(c []JournalEntry) []JournalEntry { c[2].Outcome = "failure"; return c },
		"edited time":     func(c []JournalEntry) []JournalEntry { c[1].At = c[1].At.Add(time.Second); return c },
		"dropped row":     func(c []JournalEntry) []JournalEntry { return append(c[:2], c[3:]...) },
		"swapped rows":    func(c []JournalEntry) []JournalEntry { c[1], c[2] = c[2], c[1]; return c },
		"rehashed alone":  func(c []JournalEntry) []JournalEntry { c[2].Operator = "mallory"; _ = c[2].Seal(c[1].Hash); return c },
		"wrong genesis":   func(c []JournalEntry) []JournalEntry { return c[1:] },
		"other host":      func(c []JournalEntry) []JournalEntry { c[3].Host = "west"; return c },
		"forged previous": func(c []JournalEntry) []JournalEntry { c[0].PreviousHash = strings.Repeat("1", 64); return c },
	} {
		t.Run(name, func(t *testing.T) {
			if err := VerifyChain(mut(chain(t, 5)), GenesisHash); !errors.Is(err, ErrChainBroken) {
				t.Fatalf("got %v, want ErrChainBroken", err)
			}
		})
	}
}

func TestJournalSealRejectsInvalidEntries(t *testing.T) {
	good := chain(t, 1)[0]
	for name, mut := range map[string]func(*JournalEntry){
		"unknown host":   func(e *JournalEntry) { e.Host = "north" },
		"seq zero":       func(e *JournalEntry) { e.Seq = 0 },
		"bad fix id":     func(e *JournalEntry) { e.FixID = "Rm -rf" },
		"bad fix sha":    func(e *JournalEntry) { e.FixSHA256 = "zz" },
		"bad action":     func(e *JournalEntry) { e.Action = "nuke" },
		"bad outcome":    func(e *JournalEntry) { e.Outcome = "meh" },
		"no operator":    func(e *JournalEntry) { e.Operator = "" },
		"no time":        func(e *JournalEntry) { e.At = time.Time{} },
		"bad prev":       func(e *JournalEntry) { e.PreviousHash = "x" },
		"newline in dir": func(e *JournalEntry) { e.BackupDir = "/x\n/y" },
	} {
		t.Run(name, func(t *testing.T) {
			e := good
			mut(&e)
			prev := e.PreviousHash
			if name != "bad prev" {
				prev = GenesisHash
			}
			if err := e.Seal(prev); !errors.Is(err, ErrInvalidModel) {
				t.Fatalf("got %v, want ErrInvalidModel", err)
			}
		})
	}
}

func TestJournalJSONLRoundTrip(t *testing.T) {
	c := chain(t, 3)
	var buf bytes.Buffer
	for _, e := range c {
		if err := e.WriteJSONL(&buf); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ReadJournal(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%d entries", len(got))
	}
	if err := VerifyChain(got, GenesisHash); err != nil {
		t.Fatalf("round trip broke the chain: %v", err)
	}
	for _, bad := range []string{
		`{"host":"east","seq":1,"extra":true}` + "\n",
		"not json\n",
		strings.Repeat("x", maxJournalLine+1) + "\n",
	} {
		if _, err := ReadJournal(strings.NewReader(bad)); err == nil {
			t.Errorf("accepted %.40q", bad)
		}
	}
}

func TestFixSHA256IsStableAndSensitive(t *testing.T) {
	m := repoModel(t)
	f := m.Fixes[0]
	a, err := f.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.SHA256()
	if a != b || len(a) != 64 {
		t.Fatalf("unstable or malformed: %s %s", a, b)
	}
	f.Actions = append([]Action(nil), f.Actions...)
	f.Note += "!"
	if c, _ := f.SHA256(); c == a {
		t.Error("a changed fix hashes the same")
	}
}
