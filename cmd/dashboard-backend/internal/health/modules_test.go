// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package health

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"unheaded/pkg/modules"
)

func registerKingdom(t *testing.T, m *Monitor, name string) {
	t.Helper()
	if err := m.RegisterTarget(&ServiceTarget{Name: name, HealthURL: "http://127.0.0.1:1/health", Tags: []string{"kingdom"}}); err != nil {
		t.Fatal(err)
	}
}

func targetNamed(m *Monitor, name string) *ServiceTarget {
	for _, tgt := range m.GetTargets() {
		if tgt.Name == name {
			return tgt
		}
	}
	return nil
}

func TestSyncModuleServices_AddsAndRemoves(t *testing.T) {
	m := newTestMonitor(t)
	registerKingdom(t, m, "wotan")

	want := map[string]map[string]string{"zhen": {"zhenai": "127.0.0.1:1"}}
	if a, r := m.SyncModuleServices(want); a != 1 || r != 0 {
		t.Fatalf("first sync = (%d, %d), want (1, 0)", a, r)
	}
	z := targetNamed(m, "zhenai")
	if z == nil {
		t.Fatal("zhenai not registered")
	}
	if !reflect.DeepEqual(z.Tags, []string{"module:zhen"}) {
		t.Fatalf("zhenai tags = %v", z.Tags)
	}
	if a, r := m.SyncModuleServices(want); a != 0 || r != 0 {
		t.Fatalf("second sync = (%d, %d), want (0, 0)", a, r)
	}
	if a, r := m.SyncModuleServices(nil); a != 0 || r != 1 {
		t.Fatalf("empty sync = (%d, %d), want (0, 1)", a, r)
	}
	if targetNamed(m, "zhenai") != nil {
		t.Fatal("zhenai still registered after the module went away")
	}
	if targetNamed(m, "wotan") == nil {
		t.Fatal("wotan was removed by a module sync")
	}
}

func TestSyncModuleServices_NeverTakesAKingdomName(t *testing.T) {
	m := newTestMonitor(t)
	registerKingdom(t, m, "wotan")
	if a, r := m.SyncModuleServices(map[string]map[string]string{"zhen": {"wotan": "127.0.0.1:1"}}); a != 0 || r != 0 {
		t.Fatalf("sync = (%d, %d), want (0, 0)", a, r)
	}
	if w := targetNamed(m, "wotan"); w == nil || !reflect.DeepEqual(w.Tags, []string{"kingdom"}) {
		t.Fatalf("wotan changed: %+v", w)
	}
	// And a module sync with nothing wanted never removes a kingdom target.
	if _, r := m.SyncModuleServices(nil); r != 0 {
		t.Fatalf("removed %d kingdom targets", r)
	}
}

func TestSyncModuleServices_RefusesBadNamesAndAddresses(t *testing.T) {
	m := newTestMonitor(t)
	got, _ := m.SyncModuleServices(map[string]map[string]string{
		"Bad Module": {"x": "127.0.0.1:1"},
		"zhen":       {"": "127.0.0.1:1", "y": ""},
	})
	if got != 0 {
		t.Fatalf("added %d targets from malformed input", got)
	}
}

func TestSystemHealth_ModuleTargetDoesNotDecideOverall(t *testing.T) {
	m := newTestMonitor(t)
	registerKingdom(t, m, "wotan")
	m.SyncModuleServices(map[string]map[string]string{"zhen": {"zhenai": "127.0.0.1:1"}})

	m.healthMu.Lock()
	m.health["wotan"].Status = StatusHealthy
	m.health["zhenai"].Status = StatusUnhealthy
	m.healthMu.Unlock()

	sh := m.GetSystemHealth()
	if sh.Status != StatusHealthy {
		t.Fatalf("status = %s, want healthy (%s)", sh.Status, sh.Message)
	}
	if sh.TotalServices != 1 || sh.UnhealthyCount != 0 {
		t.Fatalf("total=%d unhealthy=%d, want 1 and 0", sh.TotalServices, sh.UnhealthyCount)
	}
	if _, ok := sh.Services["wotan"]; !ok {
		t.Fatal("wotan missing from Services")
	}
	if _, ok := sh.Services["zhenai"]; !ok {
		t.Fatal("zhenai missing from Services")
	}
}

func TestTargetCounts(t *testing.T) {
	m := newTestMonitor(t)
	registerKingdom(t, m, "wotan")
	registerKingdom(t, m, "timeguru")
	m.SyncModuleServices(map[string]map[string]string{"zhen": {"zhenai": "127.0.0.1:1", "llama-server": "127.0.0.1:2"}})
	got := m.TargetCounts()
	want := map[string]int{"": 2, "zhen": 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetCounts = %v, want %v", got, want)
	}
}

func TestParseServiceEndpoints(t *testing.T) {
	text := "# Read while the Zhen module is on.\n\nzhenai=https://172.28.1.1:20112\nllama-server=172.28.1.1:8081\n"
	got, err := ParseServiceEndpoints(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"zhenai": "https://172.28.1.1:20112", "llama-server": "172.28.1.1:8081"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, bad := range []string{"no-equals-sign\n", "=127.0.0.1:1\n", "x=\n"} {
		if _, err := ParseServiceEndpoints(strings.NewReader(bad)); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestModuleEndpoints_FromFragments(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(modules.EnvDir, dir)
	write := func(rel, text string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := ModuleEndpoints(testLogger()); len(got) != 0 {
		t.Fatalf("no module on: %v", got)
	}
	write("zhen/dashboard/services.txt", "zhenai=127.0.0.1:1\n")
	write("broken/dashboard/services.txt", "not a line\n")
	got := ModuleEndpoints(testLogger())
	want := map[string]map[string]string{"zhen": {"zhenai": "127.0.0.1:1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
