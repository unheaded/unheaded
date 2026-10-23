// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package sloguard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const policyYAML = `window: 28d
slos:
  - name: wotan
    objective: 99.5
    page: true
    good: avg(up{job="wotan"})
  - name: dashboard
    objective: 99.5
    page: true
    good: x
  - name: kev-feed
    objective: 99.0
    page: false
    good: y
`

func writePolicy(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeVM answers /api/v1/query with the given slo -> value strings.
func fakeVM(t *testing.T, values map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" || r.URL.Query().Get("query") != "slo:error_ratio:5m" {
			http.Error(w, "unexpected query", http.StatusBadRequest)
			return
		}
		var result []map[string]any
		for slo, v := range values {
			result = append(result, map[string]any{"metric": map[string]string{"slo": slo}, "value": []any{1.0, v}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": result}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func guard(t *testing.T, vm string) *Guard {
	t.Helper()
	g, err := New(vm, writePolicy(t, policyYAML))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestLoadPolicy_PageSLOsOnly(t *testing.T) {
	slos, err := LoadPolicy(writePolicy(t, policyYAML))
	if err != nil {
		t.Fatal(err)
	}
	if len(slos) != 2 || slos[0].Name != "wotan" || slos[1].Name != "dashboard" || slos[0].Objective != 99.5 {
		t.Fatalf("page SLOs = %+v", slos)
	}
}

func TestLoadPolicy_Refuses(t *testing.T) {
	for name, body := range map[string]string{
		"no page SLO":     "slos:\n  - name: a\n    objective: 99\n    page: false\n",
		"bad objective":   "slos:\n  - name: a\n    objective: 100\n    page: true\n",
		"zero objective":  "slos:\n  - name: a\n    objective: 0\n    page: true\n",
		"bad name":        "slos:\n  - name: 'a\"}'\n    objective: 99\n    page: true\n",
		"not yaml":        "{{{",
		"duplicate names": "slos:\n  - name: a\n    objective: 99\n    page: true\n  - name: a\n    objective: 99\n    page: true\n",
	} {
		if _, err := LoadPolicy(writePolicy(t, body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := LoadPolicy(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("missing policy accepted")
	}
}

func TestGuard_HealthyPasses(t *testing.T) {
	vm := fakeVM(t, map[string]string{"wotan": "0", "dashboard": "0.01", "kev-feed": "1"})
	v, err := guard(t, vm.URL).Check(context.Background())
	if err != nil || !v.OK {
		t.Fatalf("verdict %+v err %v", v, err)
	}
}

// Fast tier: 14.4 x (1 - 0.995) = 0.072. A non-page SLO never trips it.
func TestGuard_FastBurnFails(t *testing.T) {
	vm := fakeVM(t, map[string]string{"wotan": "0.08", "dashboard": "0", "kev-feed": "1"})
	v, err := guard(t, vm.URL).Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.OK || len(v.Burning) != 1 || v.Burning[0] != "wotan" || !strings.Contains(v.Reason, "wotan") {
		t.Fatalf("verdict %+v, want wotan burning", v)
	}
	vm = fakeVM(t, map[string]string{"wotan": "0.07", "dashboard": "0"})
	if v, _ := guard(t, vm.URL).Check(context.Background()); !v.OK {
		t.Fatalf("just under the fast tier tripped: %+v", v)
	}
}

// ADR-100: a missing series counts as bad.
func TestGuard_MissingSeriesFails(t *testing.T) {
	for name, vals := range map[string]map[string]string{
		"absent": {"wotan": "0"},
		"NaN":    {"wotan": "0", "dashboard": "NaN"},
		"junk":   {"wotan": "0", "dashboard": "lots"},
	} {
		vm := fakeVM(t, vals)
		v, err := guard(t, vm.URL).Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if v.OK || len(v.Missing) != 1 || v.Missing[0] != "dashboard" {
			t.Fatalf("%s: verdict %+v, want dashboard missing", name, v)
		}
	}
}

func TestGuard_VictoriaDownIsAnError(t *testing.T) {
	vm := fakeVM(t, nil)
	g := guard(t, vm.URL)
	vm.Close()
	if v, err := g.Check(context.Background()); err == nil || v.OK {
		t.Fatalf("Victoria down: verdict %+v err %v, want an error and not OK", v, err)
	}
}

func TestNew_Refuses(t *testing.T) {
	p := writePolicy(t, policyYAML)
	for _, u := range []string{"", "ftp://x", "http://u:p@x", "::"} {
		if _, err := New(u, p); err == nil {
			t.Errorf("VM URL %q accepted", u)
		}
	}
}
