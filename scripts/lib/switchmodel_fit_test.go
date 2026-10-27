// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package scriptlib

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// S-CLEAN-SLATE M-037: switch-model.sh's fit check ignored llama-server's
// --cache-ram, the host-RAM prompt cache (default 8192 MiB), and a model
// passed the check and then wedged the 14 GiB host at 9.4 GiB RSS. These run
// `switch-model.sh --check` only: a fake rocm-smi on PATH, a fake meminfo,
// sparse GGUF files and an unused LLAMA_PORT. Nothing is stopped or launched.

const gib = int64(1) << 30

type fitEnv struct {
	vramFree int64 // bytes rocm-smi reports free on card0
	ramAvail int64 // MemAvailable
	gguf     map[string]int64
	logger   bool // put a fake logger on PATH that echoes "LOGGER: <args>"
}

func runFitCheck(t *testing.T, fe fitEnv, key string) (int, string) {
	t.Helper()
	return runSwitchModel(t, fe, "--check", key)
}

// runSwitchModel runs switch-model.sh with args in the fake environment
// (LLAMA_BIN=/nonexistent: nothing can be launched).
func runSwitchModel(t *testing.T, fe fitEnv, args ...string) (int, string) {
	t.Helper()
	for _, tool := range []string{"flock", "ss", "numfmt"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	models := filepath.Join(dir, "models")
	for _, d := range []string{bin, models} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	const total = 12 * gib
	smi := fmt.Sprintf("#!/bin/sh\necho 'device,VRAM Total Memory (B),VRAM Total Used Memory (B)'\necho 'card0,%d,%d'\n",
		total, total-fe.vramFree)
	if err := os.WriteFile(filepath.Join(bin, "rocm-smi"), []byte(smi), 0o755); err != nil {
		t.Fatal(err)
	}
	if fe.logger {
		if err := os.WriteFile(filepath.Join(bin, "logger"), []byte("#!/bin/sh\necho \"LOGGER: $*\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	meminfo := filepath.Join(dir, "meminfo")
	if err := os.WriteFile(meminfo, []byte(fmt.Sprintf("MemTotal: %d kB\nMemAvailable: %d kB\n", 14*gib/1024, fe.ramAvail/1024)), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, size := range fe.gguf {
		f, err := os.Create(filepath.Join(models, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	cmd := exec.Command("bash", append([]string{filepath.Join("..", "switch-model.sh")}, args...)...)
	cmd.Env = []string{
		"PATH=" + bin + ":" + os.Getenv("PATH"),
		"GPU_LOCK=" + filepath.Join(dir, "u", "gpu.lock"),
		"GPU_LEASE_TIMEOUT=5",
		"LLAMA_BIN=/nonexistent",
		"LLAMA_PORT=9",
		"MODELS_DIR=" + models,
		"FIT_MEMINFO=" + meminfo,
	}
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatal(err)
	}
	return 0, string(out)
}

const qwen7b = "qwen2.5-coder-7b-instruct-q4_k_m.gguf"

func TestSwitchModelFit_PrintsCacheRAMAndFits(t *testing.T) {
	code, out := runFitCheck(t, fitEnv{vramFree: 11 * gib, ramAvail: 6 * gib,
		gguf: map[string]int64{qwen7b: 4*gib + 400<<20}}, "qwen-7b")
	if code != 0 {
		t.Fatalf("qwen-7b with 11 GiB VRAM and 6 GiB RAM free: exit %d, want 0\n%s", code, out)
	}
	if !regexp.MustCompile(`cache_ram_mib=[0-9]+`).MatchString(out) {
		t.Errorf("--check does not print cache_ram_mib=:\n%s", out)
	}
}

func TestSwitchModelFit_RefusesWhenCacheRAMDoesNotFitRAM(t *testing.T) {
	// The weights fit in VRAM; the prompt cache does not fit in host RAM.
	code, out := runFitCheck(t, fitEnv{vramFree: 11 * gib, ramAvail: 1 * gib,
		gguf: map[string]int64{qwen7b: 4*gib + 400<<20}}, "qwen-7b")
	if code != 3 {
		t.Fatalf("qwen-7b with 1 GiB RAM free: exit %d, want 3 (refused)\n%s", code, out)
	}
	if !strings.Contains(out, "cache-ram") {
		t.Errorf("refusal does not name the cache-ram:\n%s", out)
	}
}

func TestSwitchModelFit_PartialOffloadCountsCacheRAM(t *testing.T) {
	// deepseek-cpu: weights spill to RAM. 9 GiB * 1.15 = 10.35 GiB fits a
	// 6 + 5.2 - 0.5 = 10.7 GiB budget alone, not with the cache on top.
	code, out := runFitCheck(t, fitEnv{vramFree: 6 * gib, ramAvail: 5*gib + 200<<20,
		gguf: map[string]int64{"DeepSeek-Coder-V2-Lite-Instruct-Q4_K_M.gguf": 9 * gib}}, "deepseek-cpu")
	if code != 3 {
		t.Fatalf("deepseek-cpu at the edge: exit %d, want 3 (cache-ram tips it over)\n%s", code, out)
	}
}

// llama_cache_ram_mib (scripts/lib/llama-cache-ram.sh) reads --cache-ram /
// -cram from a flag string, defaulting to llama-server's own 8192.
func TestLlamaCacheRAM_Parse(t *testing.T) {
	cases := []struct {
		flags string
		want  string
		ok    bool
	}{
		{"--ctx-size 16384", "8192", true},
		{"--ctx-size 4096 --cache-ram 1024 --parallel 1", "1024", true},
		{"-cram 0", "0", true},
		{"--cache-ram=2048", "2048", true},
		{"--cache-ram -1", "", false}, // unbounded: no fit check can pass it
		{"--cache-ram lots", "", false},
	}
	for _, c := range cases {
		cmd := exec.Command("bash", "-c", `. ./llama-cache-ram.sh && llama_cache_ram_mib "$1"`, "_", c.flags)
		out, err := cmd.CombinedOutput()
		got := strings.TrimSpace(string(out))
		if c.ok && (err != nil || got != c.want) {
			t.Errorf("%q: got %q err %v, want %q", c.flags, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("%q: got %q, want an error", c.flags, got)
		}
	}
}

// Every model's launch flags state --cache-ram, so the fit check and the
// launch agree on the number instead of both relying on llama.cpp's default.
func TestSwitchModel_EveryModelSetsCacheRAM(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "switch-model.sh"))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^MODEL_FLAGS\[([a-z0-9-]+)\]="([^"]*)"`)
	ms := re.FindAllStringSubmatch(string(src), -1)
	if len(ms) == 0 {
		t.Fatal("no MODEL_FLAGS entries found")
	}
	for _, m := range ms {
		if !regexp.MustCompile(`(^| )--cache-ram [0-9]+( |$)`).MatchString(m[2]) {
			t.Errorf("MODEL_FLAGS[%s] has no explicit --cache-ram: %q", m[1], m[2])
		}
	}
}
