// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//
// LICH-016 — the adversary auditing the adversary. S-LICH-AWAKENS Phase 9.
// Fuzz the Lich provider's own tool inputs and assert the safety invariants:
// scope OR gate refuses, no side effect outside the sandbox/register, no panic.
// The campaign dir (tomb/lich/LICH-016-lich-mcp-self/) points here.

package lichtools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// allTools is every tool the provider exposes; the self-fuzz drives each.
var allTools = []string{
	ToolListCampaigns, ToolShowCampaign, ToolRunHarness,
	ToolReadCrash, ToolTriage, ToolFileFinding,
}

// filesOutside counts files under root that are NOT within the register,
// seeds, results or harness trees — i.e. any write that escaped the provider's
// configured surface. A fuzz call must never create one.
func filesOutside(t *testing.T, root string) int {
	t.Helper()
	allowed := []string{
		filepath.Join(root, "compliance"),
		filepath.Join(root, "tomb", "lich", "seeds"),
		filepath.Join(root, "tomb", "lich", "results"),
		filepath.Join(root, "tomb", "lich", "harnesses"),
		filepath.Join(root, "tomb", "lich", "LICH-012-config-convergence"),
		filepath.Join(root, "var"),
	}
	n := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, _ error) error {
		if d == nil || d.IsDir() {
			return nil
		}
		for _, a := range allowed {
			if strings.HasPrefix(path, a) {
				return nil
			}
		}
		n++
		return nil
	})
	return n
}

// adversarialInputs are the malformed / oversized / unicode / nested /
// path-escape / newline-injection tool arguments LICH-016 throws at every tool.
func adversarialInputs() []string {
	big := strings.Repeat("A", 1<<16)
	return []string{
		``,
		`{`,
		`[]`,
		`null`,
		`{"harness_id":"../../../../etc/passwd"}`,
		`{"harness_id":"001\n002"}`,
		`{"harness_id":"00%d"}`,
		`{"campaign_id":"../../../../../../etc"}`,
		`{"results_dir":"../../../../etc","crash_id":"x"}`,
		`{"crash_id":"a\u0000b"}`,
		`{"harness_id":"` + big + `"}`,
		`{"summary":"` + big + `","severity":"high"}`,
		`{"severity":"high","summary":"x","repro_hex":"not-hex-zz"}`,
		"{\"severity\":\"\u202eevil\",\"summary\":\"x\"}", // U+202E RIGHT-TO-LEFT OVERRIDE, escaped
		`{"nested":{"a":{"b":{"c":[1,2,3]}}}}`,
		`{"harness_id":"ʟɪᴄʜ"}`, // homoglyphs
		`{"__proto__":{"x":1},"harness_id":"001"}`,
	}
}

// TestLICH016_SelfFuzzNoPanicNoEscape drives every tool with every adversarial
// input directly against the Provider (the layer under the gate) and asserts
// the invariants that must hold REGARDLESS of the gate: the call never panics,
// and no write ever escapes the provider's configured trees (no path traversal
// out of the register/seeds/results/harness surface). The register-write gate
// is enforced one layer up (confirmGate) and tested in cmd/lich-mcp.
func TestLICH016_SelfFuzzNoPanicNoEscape(t *testing.T) {
	p, root := richProvider(t)
	out0 := filesOutside(t, root)
	// Also bound register growth: the oversized-field hardening must hold even
	// when the provider is called directly (file_finding writes post-gate).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, tool := range allTools {
		for _, in := range adversarialInputs() {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("PANIC in %s(%.40q): %v", tool, in, r)
					}
				}()
				res, err := p.Call(ctx, tool, json.RawMessage(in))
				_ = res
				_ = err
			}()
		}
	}

	if out := filesOutside(t, root); out != out0 {
		t.Fatalf("self-fuzz wrote %d file(s) outside the provider's configured trees (path traversal)", out-out0)
	}
	// The hardening bound: no single register entry may be oversized. The 64KiB
	// adversarial summary must have been clamped.
	raw, _ := os.ReadFile(filepath.Join(root, "compliance", "findings", "register.yaml")) // #nosec G304 -- test temp
	if strings.Contains(string(raw), strings.Repeat("A", maxFindingField+1)) {
		t.Fatalf("an oversized field was written to the register unbounded")
	}
}

// FuzzLichToolInputs is the native-fuzz entry for LICH-016: `go test -fuzz`.
func FuzzLichToolInputs(f *testing.F) {
	for _, in := range adversarialInputs() {
		f.Add(ToolReadCrash, in)
	}
	p, _ := richProvider(&testing.T{}) // provider over a temp tree
	f.Fuzz(func(t *testing.T, tool, in string) {
		if !strings.HasPrefix(tool, "lich_") {
			return
		}
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("PANIC %s(%.40q): %v", tool, in, r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = p.Call(ctx, tool, json.RawMessage(in))
	})
}

// --- the four safety invariants (Phase 9 Steps 225-228) ------------------

// TestSafety_NoAutoPublishPath asserts there is no code path from a finding to
// a public surface: file_finding writes only the configured register, never
// Wotan/HTTP/git. (The source grep in the campaign README complements this.)
func TestSafety_NoAutoPublishPath(t *testing.T) {
	p, root := richProvider(t)
	// auto_publish defaults false.
	if p.cfg.Findings.AutoPublish {
		t.Fatalf("auto_publish must default false")
	}
	regPath := filepath.Join(root, "compliance", "findings", "register.yaml")
	before, _ := os.Stat(regPath)
	payload, _ := json.Marshal(map[string]string{"severity": "high", "summary": "x", "dedup_key": "np"})
	res, _ := p.Call(context.Background(), ToolFileFinding, payload)
	if res.IsError {
		t.Fatalf("file_finding: %+v", res)
	}
	// The finding must be in the register (local, gated) and nowhere else.
	after, _ := os.Stat(regPath)
	if before != nil && after != nil && after.Size() <= before.Size() {
		t.Fatalf("finding did not land in the gated register")
	}
	// No "published" marker file anywhere under the tree.
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() && strings.Contains(strings.ToLower(d.Name()), "publish") {
			t.Fatalf("a publish artifact appeared: %s", path)
		}
		return nil
	})
}

// TestSafety_ConfirmTokenForgery: an approval is bound to the exact CallHash;
// a used ticket cannot replay and a different call cannot inherit it. (Bound in
// Phase 4; re-asserted here as a Phase-9 safety invariant.)
func TestSafety_ConfirmTokenForgery(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := "/proj"
	args := json.RawMessage(`{"severity":"high","summary":"x"}`)
	tkt, err := store.Park(ToolFileFinding, "x", args, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Approve(tkt.ID); err != nil {
		t.Fatal(err)
	}
	// Forgery 1: a different call (different args) must not find the approval.
	other := CallHash(ToolFileFinding, root, json.RawMessage(`{"severity":"low","summary":"y"}`))
	if _, ok, _ := store.FindApproved(other); ok {
		t.Fatalf("a different call inherited an approval (forgery)")
	}
	// The exact call redeems once.
	exact := CallHash(ToolFileFinding, root, args)
	red, ok, err := store.FindApproved(exact)
	if err != nil || !ok {
		t.Fatalf("exact call should redeem: ok=%v err=%v", ok, err)
	}
	if err := store.MarkUsed(red.ID); err != nil {
		t.Fatal(err)
	}
	// Replay: a used ticket cannot be redeemed again.
	if _, ok, _ := store.FindApproved(exact); ok {
		t.Fatalf("used ticket replayed (forgery)")
	}
}
