// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ADR-060 T19 (S-CHAMPION-HARDENING Step 216): rollback after a swap that
// timed out or died after stopping the old server; none when the old model
// is still serving or the script refused before stopping anything.

// rollbackScript: "fast" comes up; "slow" never does (0.05 s documented);
// "dies" stops the old server and exits 1; "refused" exits 3 (fit check,
// nothing stopped).
const rollbackScript = `#!/bin/bash
MODEL_FILE[fast]="f.gguf"
MODEL_FILE[slow]="s.gguf"
MODEL_FILE[dies]="d.gguf"
MODEL_FILE[refused]="r.gguf"
d=$(dirname "$0")
echo "$*" >> "$d/calls.log"
if [ "$1" = "--json" ]; then
  a=$(cat "$d/active" 2>/dev/null)
  if [ -n "$a" ]; then a="\"$a\""; else a=null; fi
  printf '{"active":%s,"port":8081,"models":[{"key":"fast","load_seconds":1},{"key":"slow","load_seconds":0.05},{"key":"dies","load_seconds":1},{"key":"refused","load_seconds":1}],"fit":null}\n' "$a"
  exit 0
fi
[ "$1" = refused ] && { echo "will not fit" >&2; exit 3; }
: > "$d/active"
[ "$1" = slow ] && exec sleep 30
[ "$1" = dies ] && { echo "died" >&2; exit 1; }
echo "$1" > "$d/active"
`

func rollbackChampion(t *testing.T, active string) (*Champion, string) {
	t.Helper()
	resetSwapState()
	t.Setenv("GPU_LEASE_TIMEOUT", "0")
	c, _ := newTestChampion(t)
	dir := filepath.Join(c.config.ProjectRoot, "scripts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "switch-model.sh"), []byte(rollbackScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active"), []byte(active+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return c, dir
}

func swapCalls(t *testing.T, dir string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
	return strings.ReplaceAll(strings.TrimSpace(string(b)), "\n", "|")
}

func TestModelSwap_TimeoutRollsBackAndObserves(t *testing.T) {
	c, dir := rollbackChampion(t, "fast")
	var mu sync.Mutex
	var seen []ModelSwapEvent
	c.WithModelSwapObserver(func(ev ModelSwapEvent) {
		mu.Lock()
		seen = append(seen, ev)
		mu.Unlock()
	})
	start := time.Now()
	_, err := c.ModelSwap(context.Background(), "slow")
	if !errors.Is(err, ErrSwapTimeout) || !strings.Contains(err.Error(), "rolled back to fast") {
		t.Fatalf("err %v, want ErrSwapTimeout rolled back to fast", err)
	}
	if time.Since(start) > 15*time.Second {
		t.Errorf("took %s: the 30 s sleep was not killed", time.Since(start))
	}
	if got := swapCalls(t, dir); got != "--json --status|slow|--json --status|fast" {
		t.Errorf("calls %q", got)
	}
	if len(seen) != 1 || seen[0].Key != "slow" || seen[0].From != "fast" || seen[0].Outcome != "rolled_back" || !errors.Is(seen[0].Err, ErrSwapTimeout) {
		t.Errorf("observed %+v", seen)
	}
}

func TestModelSwap_DiedAfterStopRollsBack(t *testing.T) {
	c, dir := rollbackChampion(t, "fast")
	_, err := c.ModelSwap(context.Background(), "dies")
	if err == nil || !strings.Contains(err.Error(), "rolled back to fast") {
		t.Fatalf("err %v", err)
	}
	if got := swapCalls(t, dir); got != "--json --status|dies|--json --status|fast" {
		t.Errorf("calls %q", got)
	}
}

func TestModelSwap_NoRollbackWhenNothingStopped(t *testing.T) {
	c, dir := rollbackChampion(t, "fast")
	_, err := c.ModelSwap(context.Background(), "refused")
	if err == nil || strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("err %v, want a failure with no rollback", err)
	}
	if got := swapCalls(t, dir); got != "--json --status|refused" {
		t.Errorf("calls %q: exit 3 stops nothing, so nothing is restarted", got)
	}
}

func TestModelSwap_NoRollbackWithoutAPreviousModel(t *testing.T) {
	c, dir := rollbackChampion(t, "")
	_, err := c.ModelSwap(context.Background(), "slow")
	if !errors.Is(err, ErrSwapTimeout) || !strings.Contains(err.Error(), "nothing to roll back to") {
		t.Fatalf("err %v", err)
	}
	if got := swapCalls(t, dir); got != "--json --status|slow" {
		t.Errorf("calls %q", got)
	}
}

func TestSwapBudget(t *testing.T) {
	t.Setenv("GPU_LEASE_TIMEOUT", "")
	if got := swapBudget(45); got != 180*time.Second+defaultSwapLeaseWait {
		t.Errorf("45 s model: %s", got)
	}
	if got := swapBudget(0); got != legacySwapTimeout {
		t.Errorf("unknown load time: %s, want the legacy %s", got, legacySwapTimeout)
	}
	t.Setenv("GPU_LEASE_TIMEOUT", "30")
	if got := swapBudget(10); got != 70*time.Second {
		t.Errorf("GPU_LEASE_TIMEOUT=30: %s", got)
	}
}
