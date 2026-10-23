// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRegisterBridge_SeveritySchema files one finding at each severity and
// asserts the register still STRICT-parses (the exact keys the generator
// decodes) and carries the severity and an `opened` date. The ADR-098
// generator derives the due date from severity+opened (critical 7d / high 30d /
// medium 90d / low 180d), which its own tests cover; the bridge's contract is a
// schema-exact, severity-valid, dated entry.
func TestRegisterBridge_SeveritySchema(t *testing.T) {
	for _, sev := range []string{"critical", "high", "medium", "low"} {
		p, root := richProvider(t)
		regPath := filepath.Join(root, "compliance", "findings", "register.yaml")
		payload, _ := json.Marshal(map[string]string{
			"severity": sev, "summary": "diff " + sev, "dedup_key": "k-" + sev,
		})
		res, _ := p.Call(context.Background(), ToolFileFinding, payload)
		if res.IsError {
			t.Fatalf("file_finding %s: %+v", sev, res)
		}
		raw, _ := os.ReadFile(regPath) // #nosec G304 -- test temp
		assertRegisterStrictParses(t, raw)
		if !strings.Contains(string(raw), "severity: "+sev) {
			t.Errorf("register missing severity: %s", sev)
		}
		if !strings.Contains(string(raw), "opened: ") {
			t.Errorf("register entry has no opened date for %s", sev)
		}
	}
}

// TestRegressionSeedOnFiling proves ADR-062 regression-seed discipline: filing
// a finding checks in its crashing input (or a pointer) under the campaign's
// seed dir, inert (0600), so the weekly replay re-arms until the evidence
// passes.
func TestRegressionSeedOnFiling(t *testing.T) {
	p, root := richProvider(t)
	seedsBase := filepath.Join(root, "tomb", "lich", "seeds")

	// (1) With repro_hex, the exact bytes are checked in as a .bin seed.
	payload, _ := json.Marshal(map[string]string{
		"severity": "high", "summary": "mbc divergence", "dedup_key": "d1",
		"campaign_id": "LICH-015", "repro_hex": "3a010000ff000000",
	})
	res, _ := p.Call(context.Background(), ToolFileFinding, payload)
	if res.IsError {
		t.Fatalf("file_finding: %+v", res)
	}
	if !strings.Contains(res.Text, "regression-seed=") {
		t.Fatalf("filing should report a regression seed: %s", res.Text)
	}
	dir := filepath.Join(seedsBase, "lich_015")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("seed dir %s not created: %v", dir, err)
	}
	var bin string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".bin") {
			bin = filepath.Join(dir, e.Name())
		}
	}
	if bin == "" {
		t.Fatalf("no .bin regression seed written in %s", dir)
	}
	b, _ := os.ReadFile(bin) // #nosec G304 -- test temp
	want := []byte{0x3a, 0x01, 0x00, 0x00, 0xff, 0x00, 0x00, 0x00}
	if string(b) != string(want) {
		t.Fatalf("seed bytes = % x, want % x", b, want)
	}
	if info, err := os.Stat(bin); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("seed must be 0600 inert data: mode=%v err=%v", info.Mode().Perm(), err)
	}

	// (2) Without repro_hex, a pointer .seed is written so the campaign still re-arms.
	payload2, _ := json.Marshal(map[string]string{
		"severity": "medium", "summary": "no raw input", "dedup_key": "d2",
		"campaign_id": "LICH-015", "harness_id": "001", "repro_sha256": "cafebabe",
	})
	res2, _ := p.Call(context.Background(), ToolFileFinding, payload2)
	if res2.IsError {
		t.Fatalf("file_finding(2): %+v", res2)
	}
	ents2, _ := os.ReadDir(dir)
	var ptr bool
	for _, e := range ents2 {
		if strings.HasSuffix(e.Name(), ".seed") {
			ptr = true
		}
	}
	if !ptr {
		t.Fatalf("no pointer .seed written when repro_hex absent")
	}
}

// TestRegressionSeed_CannotEscapeSeedTree: a campaign_id crafted to escape the
// seeds dir must not write outside it.
func TestRegressionSeed_CannotEscapeSeedTree(t *testing.T) {
	p, root := richProvider(t)
	// campaignSeedSlug drops dots/slashes, so even "../../etc" becomes "etc"
	// and stays under the seeds tree.
	seed, err := p.writeRegressionSeed("FND-900", "d9", fileFindingInput{CampaignID: "../../etc"})
	if err != nil {
		t.Fatalf("writeRegressionSeed: %v", err)
	}
	abs := filepath.Join(root, seed)
	if !strings.Contains(abs, filepath.Join("tomb", "lich", "seeds")) {
		t.Fatalf("seed escaped the seeds tree: %s", abs)
	}
}
