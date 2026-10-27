// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package scriptlib

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// S-CHAMPION-HARDENING Step 210 (ADR-060): switch-model.sh --json prints one
// JSON object on stdout, keeps the human lines on stderr, and leaves the exit
// codes as they were.

type switchJSON struct {
	Active *string `json:"active"`
	Port   int     `json:"port"`
	Models []struct {
		Key         string  `json:"key"`
		File        string  `json:"file"`
		Present     bool    `json:"present"`
		Pinned      bool    `json:"pinned"`
		LoadSeconds float64 `json:"load_seconds"`
	} `json:"models"`
	Fit *struct {
		Key     string `json:"key"`
		Verdict string `json:"verdict"`
		NeedMiB int64  `json:"need_mib"`
	} `json:"fit"`
	Pin      string `json:"pin"`
	Decision string `json:"decision"`
}

func runSwitchJSON(t *testing.T, fe fitEnv, args ...string) (int, switchJSON, string) {
	t.Helper()
	cmd := switchModelCmd(t, fe, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	var out switchJSON
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout.String())
		}
	}
	return code, out, stderr.String()
}

func TestSwitchModelJSON_Status(t *testing.T) {
	fe := fitEnv{vramFree: 11 * gib, ramAvail: 6 * gib, gguf: map[string]int64{qwen7b: 4 * gib}}
	for _, args := range [][]string{{"--json", "--status"}, {"--json"}, {"--json", "--list"}} {
		code, out, stderr := runSwitchJSON(t, fe, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d\n%s", args, code, stderr)
		}
		if out.Active != nil || out.Port != 9 || out.Fit != nil {
			t.Errorf("%v: active %v port %d fit %+v; want nothing serving on the fake port 9", args, out.Active, out.Port, out.Fit)
		}
		keys := []string{}
		for _, m := range out.Models {
			keys = append(keys, m.Key)
			if m.Key == "qwen-7b" && (!m.Present || !m.Pinned || m.LoadSeconds != 45) {
				t.Errorf("qwen-7b %+v", m)
			}
			if m.Key == "deepseek" && m.Present {
				t.Errorf("deepseek present without its file: %+v", m)
			}
		}
		if strings.Join(keys, ",") != "qwen-7b,deepseek,deepseek-cpu,qwen-coder-14b" {
			t.Errorf("keys %v, want declaration order", keys)
		}
	}
}

func TestSwitchModelJSON_CheckAndDryRun(t *testing.T) {
	fe := fitEnv{vramFree: 11 * gib, ramAvail: 6 * gib, gguf: map[string]int64{qwen7b: 4 * gib}}
	code, out, stderr := runSwitchJSON(t, fe, "--json", "--check", "qwen-7b")
	if code != 0 || out.Fit == nil || out.Fit.Verdict != "fits" || out.Decision != "fits" || out.Pin != "" {
		t.Fatalf("--check: exit %d %+v\n%s", code, out, stderr)
	}
	if !strings.Contains(stderr, "verdict=fits") {
		t.Errorf("human fit line not on stderr:\n%s", stderr)
	}

	// Too little VRAM: refused, exit 3, the verdict in the JSON.
	small := fitEnv{vramFree: 2 * gib, ramAvail: 6 * gib, gguf: map[string]int64{qwen7b: 4 * gib}}
	code, out, _ = runSwitchJSON(t, small, "--json", "--check", "qwen-7b")
	if code != 3 || out.Fit == nil || out.Fit.Verdict != "refused" || out.Decision != "refused_fit" {
		t.Fatalf("refused --check: exit %d %+v", code, out)
	}

	// --dry-run hashes: the sparse fake file is not the pinned one.
	code, out, _ = runSwitchJSON(t, fe, "--json", "--dry-run", "qwen-7b")
	if code != 4 || out.Pin != "mismatch" || out.Decision != "refused_pin" {
		t.Fatalf("--dry-run: exit %d %+v", code, out)
	}
	code, out, _ = runSwitchJSON(t, fe, "--json", "--dry-run", "--no-verify", "qwen-7b")
	if code != 0 || out.Pin != "skipped" || out.Decision != "would_swap" {
		t.Fatalf("--dry-run --no-verify: exit %d %+v", code, out)
	}
}

func TestSwitchModelJSON_RefusedWithASwap(t *testing.T) {
	fe := fitEnv{vramFree: 11 * gib, ramAvail: 6 * gib, gguf: map[string]int64{qwen7b: 4 * gib}}
	code, _, stderr := runSwitchJSON(t, fe, "--json", "qwen-7b")
	if code != 2 || !strings.Contains(stderr, "--json works with") {
		t.Fatalf("--json with a swap: exit %d\n%s", code, stderr)
	}
	// The human --status works too.
	code, text := runSwitchModel(t, fe, "--status")
	if code != 0 || !strings.Contains(text, "llama-server on :9: not running") || !strings.Contains(text, "qwen-7b") {
		t.Fatalf("--status: exit %d\n%s", code, text)
	}
}
