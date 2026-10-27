// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"unheaded/pkg/metrics/prom"
)

// ADR-060 T19 (S-CHAMPION-HARDENING Step 216): a swap that does not come up
// within 4x its documented load time is killed and the model that was
// serving is started again; zhen-agentd counts it.

// fakeSwitchScript is switch-model.sh for two keys: "fast" loads at once,
// "slow" (documented at 0.05 s) never opens its port. It logs every call and
// keeps the serving key in a file, emptied when a swap stops the old server.
const fakeSwitchScript = `#!/bin/bash
MODEL_FILE[fast]="fast.gguf"
MODEL_FILE[slow]="slow.gguf"
d=$(dirname "$0")
echo "$*" >> "$d/calls.log"
if [ "$1" = "--json" ]; then
  a=$(cat "$d/active" 2>/dev/null)
  if [ -n "$a" ]; then a="\"$a\""; else a=null; fi
  printf '{"active":%s,"pid":null,"port":8081,"gguf":null,"models":[{"key":"fast","load_seconds":1},{"key":"slow","load_seconds":0.05}],"fit":null}\n' "$a"
  exit 0
fi
: > "$d/active"
if [ "$1" = slow ]; then exec sleep 30; fi
echo "$1" > "$d/active"
echo "[fake] healthy"
`

// metricValue reads one sample from the default registry's exposition.
func metricValue(t *testing.T, series string) float64 {
	t.Helper()
	rec := httptest.NewRecorder()
	prom.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	sc := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, series+" ") {
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, series)), 64)
			if err != nil {
				t.Fatalf("%s: %v", line, err)
			}
			return v
		}
	}
	return 0
}

func TestModelSwap_TimeoutRollsBack(t *testing.T) {
	t.Setenv("GPU_LEASE_TIMEOUT", "0") // the fake takes no lease; budget = 4 x 0.05 s
	root := t.TempDir()
	scripts := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "switch-model.sh"), []byte(fakeSwitchScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "active"), []byte("fast\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	failuresBefore := metricValue(t, `zhen_model_swap_failures_total{reason="timeout"}`)
	countBefore := metricValue(t, `zhen_model_swap_duration_seconds_count{key="slow",outcome="rolled_back"}`)

	pool := newChampionPool(newMetricsActionStore(nopActionStore{}), nil)
	srv := &server{pool: pool, defaultRoot: root, allowed: map[string]struct{}{root: {}}}
	rec := as("stevie", srv.handleToolExec, http.MethodPost, "/api/v1/tool/exec", map[string]any{
		"tool": "model_switch", "args": map[string]any{"key": "slow"}, "emitted_by": "zhen-cli",
		"justification": []map[string]any{{"topic": "user-typed-model_switch", "source_trust": "direct-user"}},
	})
	var resp toolExecResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != http.StatusInternalServerError || resp.Status != "error" ||
		!strings.Contains(resp.Reason, "time budget") || !strings.Contains(resp.Reason, "rolled back to fast") {
		t.Fatalf("swap to slow: %d %+v, want a timeout that rolled back to fast", rec.Code, resp)
	}
	active, _ := os.ReadFile(filepath.Join(scripts, "active"))
	if strings.TrimSpace(string(active)) != "fast" {
		t.Errorf("serving %q after rollback, want fast", active)
	}
	calls, _ := os.ReadFile(filepath.Join(scripts, "calls.log"))
	want := "--json --status\nslow\n--json --status\nfast\n"
	if string(calls) != want {
		t.Errorf("script calls:\n%s\nwant:\n%s", calls, want)
	}
	if got := metricValue(t, `zhen_model_swap_failures_total{reason="timeout"}`); got != failuresBefore+1 {
		t.Errorf("zhen_model_swap_failures_total{reason=timeout} = %v, want %v", got, failuresBefore+1)
	}
	if got := metricValue(t, `zhen_model_swap_duration_seconds_count{key="slow",outcome="rolled_back"}`); got != countBefore+1 {
		t.Errorf("duration count = %v, want %v", got, countBefore+1)
	}

	// A swap that comes up is observed as ok.
	okBefore := metricValue(t, `zhen_model_swap_duration_seconds_count{key="fast",outcome="ok"}`)
	rec = as("stevie", srv.handleToolExec, http.MethodPost, "/api/v1/tool/exec", map[string]any{
		"tool": "model_switch", "args": map[string]any{"key": "fast"}, "emitted_by": "zhen-cli",
		"justification": []map[string]any{{"topic": "user-typed-model_switch", "source_trust": "direct-user"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("swap to fast: %d %s", rec.Code, rec.Body.String())
	}
	if got := metricValue(t, `zhen_model_swap_duration_seconds_count{key="fast",outcome="ok"}`); got != okBefore+1 {
		t.Errorf("ok count = %v, want %v", got, okBefore+1)
	}
}
