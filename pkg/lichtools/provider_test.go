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
	"time"
)

func testProvider(t *testing.T) *Provider {
	t.Helper()
	root := t.TempDir()
	hd := filepath.Join(root, "harnesses")
	if err := os.MkdirAll(hd, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hd, "lich_001_x_test.go"), []byte("package x\n"), 0o600); err != nil {
		t.Fatalf("write harness: %v", err)
	}
	cfg := &Config{ProjectRoot: root}
	cfg.Scope.SandboxPrefix = "lich-sbx-"
	cfg.Scope.HarnessDir = "harnesses"
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return p
}

func TestProvider_Tools_ListsSix(t *testing.T) {
	p := testProvider(t)
	tools, err := p.Tools(context.Background())
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if len(tools) != 6 {
		t.Fatalf("want 6 tools, got %d", len(tools))
	}
	want := map[string]bool{
		ToolListCampaigns: false, ToolShowCampaign: false, ToolRunHarness: false,
		ToolReadCrash: false, ToolTriage: false, ToolFileFinding: false,
	}
	for _, tool := range tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("unexpected tool %q", tool.Name)
		}
		want[tool.Name] = true
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Errorf("tool %q schema not valid JSON: %v", tool.Name, err)
		}
		if schema["additionalProperties"] != false {
			t.Errorf("tool %q schema must set additionalProperties:false", tool.Name)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestProvider_Call_UnknownFieldRejected(t *testing.T) {
	p := testProvider(t)
	res, err := p.Call(context.Background(), ToolShowCampaign, json.RawMessage(`{"campaign_id":"LICH-001","bogus":1}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text, "invalid tool input") {
		t.Fatalf("unknown field should be a tool error, got %+v", res)
	}
}

func TestProvider_Call_RunHarness_ScopeRefusesEvilTarget(t *testing.T) {
	p := testProvider(t)
	res, err := p.Call(context.Background(), ToolRunHarness, json.RawMessage(`{"harness_id":"evil-host"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("off-allowlist harness must be refused, got %+v", res)
	}
}

func TestProvider_Call_RunHarness_FailsClosedWithoutConfinement(t *testing.T) {
	p := testProvider(t) // no WithConfinement → no sandbox
	// 001 passes scope, but with no confinement substrate wired it must fail
	// closed (never run unconfined).
	res, err := p.Call(context.Background(), ToolRunHarness, json.RawMessage(`{"harness_id":"001"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Text, "no confinement substrate") {
		t.Fatalf("in-scope run_harness with no confinement should fail closed, got %+v", res)
	}
}

func TestProvider_RunHarness_WithSandbox(t *testing.T) {
	p := testProvider(t).WithConfinement(&fakeSandbox{}, NewSemaphore(2), NewKillSwitch("", time.Millisecond, &fakeSandbox{}, nil))
	res, err := p.Call(context.Background(), ToolRunHarness, json.RawMessage(`{"harness_id":"001"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if res.IsError || !strings.Contains(res.Text, "confined sandbox") {
		t.Fatalf("run_harness via sandbox should succeed, got %+v", res)
	}
}

func TestProvider_RunHarness_HaltedRefuses(t *testing.T) {
	ks := NewKillSwitch("", time.Millisecond, &fakeSandbox{}, nil)
	ks.Trip("test")
	p := testProvider(t).WithConfinement(&fakeSandbox{}, NewSemaphore(2), ks)
	res, _ := p.Call(context.Background(), ToolRunHarness, json.RawMessage(`{"harness_id":"001"}`))
	if !res.IsError || !strings.Contains(res.Text, "halted") {
		t.Fatalf("run_harness under a tripped kill switch must refuse, got %+v", res)
	}
}

func TestProvider_RunHarness_AtCapacityRefuses(t *testing.T) {
	sem := NewSemaphore(1)
	if _, err := sem.TryAcquire(); err != nil { // occupy the only slot
		t.Fatalf("pre-acquire: %v", err)
	}
	p := testProvider(t).WithConfinement(&fakeSandbox{}, sem, NewKillSwitch("", time.Millisecond, &fakeSandbox{}, nil))
	res, _ := p.Call(context.Background(), ToolRunHarness, json.RawMessage(`{"harness_id":"001"}`))
	if !res.IsError || !strings.Contains(res.Text, "capacity") {
		t.Fatalf("run_harness at capacity must refuse, got %+v", res)
	}
}

func TestProvider_Call_ReadOnlyStub(t *testing.T) {
	p := testProvider(t)
	res, err := p.Call(context.Background(), ToolListCampaigns, nil)
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_campaigns stub should not be an error, got %+v", res)
	}
}

func TestProvider_TargetFor(t *testing.T) {
	p := testProvider(t)
	tgt, ok, err := p.TargetFor(ToolRunHarness, json.RawMessage(`{"harness_id":"001"}`))
	if err != nil || !ok || tgt.Kind != KindHarness || tgt.Name != "001" {
		t.Fatalf("TargetFor(run_harness) = %+v ok=%v err=%v", tgt, ok, err)
	}
	if _, ok, _ := p.TargetFor(ToolListCampaigns, json.RawMessage(`{}`)); ok {
		t.Fatalf("list_campaigns should carry no scope target")
	}
}
