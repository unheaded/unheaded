// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package lichtools

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/zhentools"
)

// richProvider builds a provider over a realistic temp tree: harnesses, a
// campaign dir, a results dir with a crash, and a seeded findings register.
func richProvider(t *testing.T) (*Provider, string) {
	t.Helper()
	root := t.TempDir()
	lich := filepath.Join(root, "tomb", "lich")
	hd := filepath.Join(lich, "harnesses")
	results := filepath.Join(lich, "results", "run1")
	crashes := filepath.Join(results, "crashes")
	reg := filepath.Join(root, "compliance", "findings")
	for _, d := range []string{hd, crashes, reg, filepath.Join(lich, "LICH-012-config-convergence")} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	write(filepath.Join(hd, "lich_001_monad_wire_test.go"), "package x\n")
	write(filepath.Join(hd, "lich_002_monad_codec_test.go"), "package x\n")
	write(filepath.Join(lich, "LICH-012-config-convergence", "README.md"),
		"# LICH-012: Configuration Convergence\n\n**Target**: Mimir's Law\n**Status**: OPEN\n")
	write(filepath.Join(crashes, "crash-abc"), "CRASHINPUT-bytes")
	write(filepath.Join(reg, "register.yaml"),
		"findings:\n  - id: FND-001\n    title: seed\n    verify: seed\n    host: west\n"+
			"    severity: low\n    class: app\n    opened: 2026-10-23\n    lockout: none\n    step: 1\n    plan: seed\n")

	cfg := &Config{ProjectRoot: root}
	cfg.Scope.SandboxPrefix = "lich-sbx-"
	cfg.Scope.HarnessDir = "tomb/lich/harnesses"
	cfg.Scope.ResultsDir = "tomb/lich/results"
	cfg.Findings.Register = "compliance/findings/register.yaml"
	if err := cfg.applyDefaultsAndValidate(); err != nil {
		t.Fatalf("config: %v", err)
	}
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return p, root
}

func TestListCampaigns_RealTree(t *testing.T) {
	p, _ := richProvider(t)
	res, err := p.Call(context.Background(), ToolListCampaigns, json.RawMessage(`{}`))
	if err != nil || res.IsError {
		t.Fatalf("list_campaigns: err=%v res=%+v", err, res)
	}
	for _, id := range []string{"LICH-001", "LICH-002", "LICH-012"} {
		if !strings.Contains(res.Text, id) {
			t.Errorf("list_campaigns missing %s: %s", id, res.Text)
		}
	}
	if !strings.Contains(res.Text, "ADR-062") {
		t.Errorf("list_campaigns must cite ADR-062")
	}
	if !strings.Contains(res.Text, "Mimir's Law") {
		t.Errorf("LICH-012 target from README not surfaced")
	}
}

func TestShowCampaign_RealTree(t *testing.T) {
	p, _ := richProvider(t)
	res, _ := p.Call(context.Background(), ToolShowCampaign, json.RawMessage(`{"campaign_id":"LICH-012"}`))
	if res.IsError || !strings.Contains(res.Text, "Configuration Convergence") {
		t.Fatalf("show LICH-012 should return its README, got %+v", res)
	}
	res2, _ := p.Call(context.Background(), ToolShowCampaign, json.RawMessage(`{"campaign_id":"LICH-999"}`))
	if !res2.IsError {
		t.Fatalf("unknown campaign should error, got %+v", res2)
	}
}

func TestReadCrash_NoExec(t *testing.T) {
	p, _ := richProvider(t)
	res, _ := p.Call(context.Background(), ToolReadCrash, json.RawMessage(`{"results_dir":"run1","crash_id":"crash-abc"}`))
	if res.IsError {
		t.Fatalf("read_crash should succeed, got %+v", res)
	}
	for _, want := range []string{"sha256:", "size: 16 bytes", "NEVER executed"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("read_crash output missing %q: %s", want, res.Text)
		}
	}
	// Path escape must be refused.
	esc, _ := p.Call(context.Background(), ToolReadCrash, json.RawMessage(`{"results_dir":"run1","crash_id":"../../etc/passwd"}`))
	if !esc.IsError {
		t.Fatalf("crash_id path escape must be refused")
	}
	out, _ := p.Call(context.Background(), ToolReadCrash, json.RawMessage(`{"results_dir":"../../../etc","crash_id":"x"}`))
	if !out.IsError {
		t.Fatalf("results_dir outside the results tree must be refused")
	}
}

func TestResolveResultsDir_Scope(t *testing.T) {
	p, _ := richProvider(t)
	if _, err := p.resolveResultsDir("run1"); err != nil {
		t.Fatalf("run1 should resolve: %v", err)
	}
	if _, err := p.resolveResultsDir("../escape"); err == nil {
		t.Fatalf("'..' must be refused")
	}
	if _, err := p.resolveResultsDir("/etc"); err == nil {
		t.Fatalf("absolute outside results tree must be refused")
	}
}

func TestFileFinding_AppendsAndDedups(t *testing.T) {
	p, root := richProvider(t)
	regPath := filepath.Join(root, "compliance", "findings", "register.yaml")

	res, _ := p.Call(context.Background(), ToolFileFinding,
		json.RawMessage(`{"severity":"high","summary":"monad parser panics on truncated input","harness_id":"001","repro_sha256":"deadbeef","dedup_key":"key-xyz"}`))
	if res.IsError || !strings.Contains(res.Text, "FND-002") {
		t.Fatalf("file_finding should append FND-002, got %+v", res)
	}
	raw, _ := os.ReadFile(regPath)
	if !strings.Contains(string(raw), "FND-002") || !strings.Contains(string(raw), "lich-dedup:key-xyz") {
		t.Fatalf("register missing the new finding / dedup marker:\n%s", raw)
	}
	// The register must still STRICT-parse (only known keys).
	assertRegisterStrictParses(t, raw)

	// A duplicate dedup key updates rather than duplicating.
	dup, _ := p.Call(context.Background(), ToolFileFinding,
		json.RawMessage(`{"severity":"high","summary":"same crash","dedup_key":"key-xyz"}`))
	if dup.IsError || !strings.Contains(dup.Text, "duplicate") {
		t.Fatalf("duplicate dedup should update, got %+v", dup)
	}
	raw2, _ := os.ReadFile(regPath)
	if strings.Count(string(raw2), "FND-002") != 1 || strings.Contains(string(raw2), "FND-003") {
		t.Fatalf("duplicate must not add a second entry:\n%s", raw2)
	}
	// needs-regen marker written, not auto-published.
	if _, err := os.Stat(filepath.Join(root, "compliance", "findings", ".needs-regen")); err != nil {
		t.Fatalf("needs-regen marker should exist: %v", err)
	}
}

// assertRegisterStrictParses decodes the register with KnownFields(true) into a
// struct carrying exactly the RegisterEntry keys, so an appended entry with an
// unknown key would fail here (mirrors cmd/compliance-evidence's strict decode).
func assertRegisterStrictParses(t *testing.T, raw []byte) {
	t.Helper()
	type entry struct {
		Source   string `yaml:"source,omitempty"`
		ID       string `yaml:"id,omitempty"`
		Title    string `yaml:"title,omitempty"`
		Verify   string `yaml:"verify,omitempty"`
		Host     string `yaml:"host,omitempty"`
		Severity string `yaml:"severity"`
		Class    string `yaml:"class"`
		Opened   string `yaml:"opened"`
		Exposure string `yaml:"exposure,omitempty"`
		Lockout  string `yaml:"lockout"`
		Step     int    `yaml:"step"`
		Plan     string `yaml:"plan,omitempty"`
		Decision string `yaml:"decision,omitempty"`
	}
	var doc struct {
		Findings []entry `yaml:"findings"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("register no longer strict-parses after file_finding: %v", err)
	}
}

// BlackMage (Step 100): a finding whose text tries to inject YAML structure
// must not break the register — yamlScalar quoting contains it.
func TestFileFinding_YAMLInjectionSafe(t *testing.T) {
	p, root := richProvider(t)
	regPath := filepath.Join(root, "compliance", "findings", "register.yaml")
	evil := `pwned"\ninjected_top_level: evil\n- id: FND-666\n  severity: critical`
	payload, _ := json.Marshal(map[string]string{
		"severity": "medium", "summary": evil, "remediation": evil, "dedup_key": "inj1",
	})
	res, _ := p.Call(context.Background(), ToolFileFinding, payload)
	if res.IsError {
		t.Fatalf("file_finding should accept and safely quote the text, got %+v", res)
	}
	raw, _ := os.ReadFile(regPath)
	assertRegisterStrictParses(t, raw) // must still parse with only known keys (no injected top-level key)
	// Structurally: exactly seed + one new entry, and no FND-666 was injected.
	ids := registerIDs(t, raw)
	if len(ids) != 2 {
		t.Fatalf("injection created extra entries; ids=%v", ids)
	}
	for _, id := range ids {
		if id == "FND-666" {
			t.Fatalf("YAML injection created a structural FND-666 entry: %v", ids)
		}
	}
}

func registerIDs(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc struct {
		Findings []struct {
			ID string `yaml:"id"`
		} `yaml:"findings"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse register: %v", err)
	}
	ids := make([]string, 0, len(doc.Findings))
	for _, f := range doc.Findings {
		ids = append(ids, f.ID)
	}
	return ids
}

func TestExtractSeverityAndDedup(t *testing.T) {
	out := "crash 1: MEDIUM\ncrash 2: CRITICAL panic nil deref\nsummary HIGH"
	if got := extractSeverity(out); got != "CRITICAL" {
		t.Fatalf("extractSeverity = %q, want CRITICAL", got)
	}
	a := normalizeStack("panic at 0xDEADBEEF run 20261023_111146\n  at foo")
	b := normalizeStack("panic at 0xCAFEBABE run 20260101_000000\n  at foo")
	if a != b {
		t.Fatalf("normalizeStack should collapse addresses/timestamps:\n%q\n%q", a, b)
	}
}

func TestClampDuration(t *testing.T) {
	p, _ := richProvider(t) // min=5 max=300 after defaults
	if p.clampDuration(0) != p.cfg.MinFuzzSeconds {
		t.Errorf("0 should clamp to min")
	}
	if p.clampDuration(100000) != p.cfg.MaxFuzzSeconds {
		t.Errorf("huge should clamp to max")
	}
	if p.clampDuration(30) != 30 {
		t.Errorf("in-range should pass through")
	}
}

func TestReserveFuzzBudget(t *testing.T) {
	p, _ := richProvider(t)
	p.cfg.MaxDailyFuzzSeconds = 100
	if err := p.reserveFuzzBudget(60); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if err := p.reserveFuzzBudget(60); err == nil {
		t.Fatalf("second reserve should exceed the 100s budget")
	}
}

// A crash id names a file in the crashes directory itself. A link planted
// there must not turn the read into a read of something outside the tree.
func TestReadCrash_RefusesLinks(t *testing.T) {
	p := testProvider(t)
	results := p.cfg.ResultsDirAbs()
	crashes := filepath.Join(results, "run-1", "crashes")
	if err := os.MkdirAll(crashes, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private")
	if err := os.WriteFile(outside, []byte("OUTSIDE-THE-TREE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(crashes, "crash-link")); err != nil {
		t.Fatal(err)
	}
	res, err := p.readCrash(readCrashInput{ResultsDir: "run-1", CrashID: "crash-link"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || strings.Contains(res.Text, hex.EncodeToString([]byte("OUTSIDE"))) {
		t.Fatalf("a link in crashes/ was followed: %+v", res)
	}
}

// Nor may crashes/ itself be a link out of the results tree.
func TestReadCrash_RefusesALinkedCrashesDir(t *testing.T) {
	p := testProvider(t)
	run := filepath.Join(p.cfg.ResultsDirAbs(), "run-2")
	if err := os.MkdirAll(run, 0o750); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "crash-a"), []byte("OUTSIDE-THE-TREE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(run, "crashes")); err != nil {
		t.Fatal(err)
	}
	res, err := p.readCrash(readCrashInput{ResultsDir: "run-2", CrashID: "crash-a"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("a linked crashes/ was followed: %+v", res)
	}
	// A failed read lists what is available: that list stays in the tree too.
	for _, id := range []string{"crash-a", "no-such-crash"} {
		res, err := p.readCrash(readCrashInput{ResultsDir: "run-2", CrashID: id})
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError || strings.Contains(strings.ReplaceAll(res.Text, id, ""), "crash-a") {
			t.Fatalf("names from outside the tree reached the caller: %+v", res)
		}
	}
}

// A wrong id still gets the list of what the real directory holds.
func TestReadCrash_ListsTheCrashesDir(t *testing.T) {
	p := testProvider(t)
	crashes := filepath.Join(p.cfg.ResultsDirAbs(), "run-3", "crashes")
	if err := os.MkdirAll(crashes, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"crash-b", "crash-a"} {
		if err := os.WriteFile(filepath.Join(crashes, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := p.readCrash(readCrashInput{ResultsDir: "run-3", CrashID: "nope"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Text, "[crash-a crash-b]") {
		t.Fatalf("got %+v", res)
	}
}

// Only a regular file is read: a link that stays in crashes/ is refused, and
// a FIFO must not hang the tool.
func TestReadCrash_RegularFilesOnly(t *testing.T) {
	p := testProvider(t)
	crashes := filepath.Join(p.cfg.ResultsDirAbs(), "run-4", "crashes")
	if err := os.MkdirAll(crashes, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(crashes, "crash-real"), []byte("IN-THE-TREE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("crash-real", filepath.Join(crashes, "crash-link")); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(crashes, "crash-fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"crash-link", "crash-fifo"} {
		done := make(chan zhentools.Result, 1)
		go func() {
			res, err := p.readCrash(readCrashInput{ResultsDir: "run-4", CrashID: id})
			if err != nil {
				t.Error(err)
			}
			done <- res
		}()
		select {
		case res := <-done:
			if !res.IsError {
				t.Errorf("%s was read: %+v", id, res)
			}
		case <-time.After(5 * time.Second):
			// Let the blocked open finish, so the goroutine does not outlive the test.
			if f, err := os.OpenFile(fifo, os.O_RDWR, 0); err == nil {
				defer f.Close()
			}
			<-done
			t.Errorf("reading %s hung", id)
		}
	}
}

// A crash log in logs/ that is a link, or a linked logs/, is not read: one line
// of a log reaches the report, and the report reaches the caller.
func TestCrashTriage_ReadsNoLinkedLog(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "tomb", "lich", "crash-triage.sh"))
	if err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "private.log")
	if err := os.WriteFile(secret, []byte("Error: OUTSIDE-THE-TREE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "h.log"), []byte("Error: OUTSIDE-THE-TREE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "h.exitcode"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	triage := func(t *testing.T, results string) string {
		t.Helper()
		out, err := exec.Command("bash", script, results).CombinedOutput()
		if err != nil {
			t.Fatalf("crash-triage: %v\n%s", err, out)
		}
		return string(out)
	}
	real := filepath.Join(t.TempDir(), "real")
	logs := filepath.Join(real, "logs")
	if err := os.MkdirAll(logs, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "own.log"), []byte("Error: IN-THE-TREE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "own.exitcode"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(logs, "linked.log")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "linked.exitcode"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := triage(t, real)
	if strings.Contains(out, "OUTSIDE-THE-TREE") {
		t.Fatalf("a linked log reached the report:\n%s", out)
	}
	if !strings.Contains(out, "IN-THE-TREE") {
		t.Fatalf("a real log did not reach the report:\n%s", out)
	}

	linkedDir := filepath.Join(t.TempDir(), "linked-logs")
	if err := os.Mkdir(linkedDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(linkedDir, "logs")); err != nil {
		t.Fatal(err)
	}
	if out := triage(t, linkedDir); strings.Contains(out, "OUTSIDE-THE-TREE") {
		t.Fatalf("a linked logs/ reached the report:\n%s", out)
	}
}
