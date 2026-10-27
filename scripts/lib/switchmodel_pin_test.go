// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package scriptlib

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// RT-15: switch-model.sh refuses a GGUF whose sha256 is not its pin in
// scripts/lib/model-pins.sh. These run --dry-run (pin + fit check, nothing
// stopped or launched) against sparse fake GGUFs, which never match a pin.

func TestSwitchModelDryRun_RefusesMismatch(t *testing.T) {
	code, out := runSwitchModel(t, fitEnv{vramFree: 11 * gib, ramAvail: 6 * gib,
		gguf: map[string]int64{qwen7b: 4 * gib}}, "--dry-run", "qwen-7b")
	if code != 4 {
		t.Fatalf("bogus qwen-7b file: exit %d, want 4 (pin refused)\n%s", code, out)
	}
	if !strings.Contains(out, "sha256 mismatch for qwen-7b: refusing") {
		t.Errorf("refusal does not name the mismatch:\n%s", out)
	}
	if strings.Contains(out, "fit ok") || strings.Contains(out, "launching") {
		t.Errorf("went past the pin check:\n%s", out)
	}
}

func TestSwitchModelNoVerify_SkipsPinAndLogs(t *testing.T) {
	// A fake logger on PATH records what would go to syslog.
	code, out := runSwitchModel(t, fitEnv{vramFree: 11 * gib, ramAvail: 6 * gib,
		gguf: map[string]int64{qwen7b: 4 * gib}, logger: true}, "--dry-run", "--no-verify", "qwen-7b")
	if code != 0 {
		t.Fatalf("--no-verify dry run: exit %d, want 0\n%s", code, out)
	}
	for _, want := range []string{"WARNING: --no-verify", "LOGGER: -t switch-model", "dry run: would swap to 'qwen-7b'"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

func TestSwitchModelCheck_DoesNotHash(t *testing.T) {
	// --check stays the fast fit-only answer: a bogus file still "fits".
	code, out := runFitCheck(t, fitEnv{vramFree: 11 * gib, ramAvail: 6 * gib,
		gguf: map[string]int64{qwen7b: 4 * gib}}, "qwen-7b")
	if code != 0 || strings.Contains(out, "sha256") {
		t.Fatalf("--check: exit %d, want 0 with no hashing\n%s", code, out)
	}
}

// verify_model_pin's return codes, with a pin set after sourcing.
func TestVerifyModelPin_Codes(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "m.gguf")
	if err := os.WriteFile(f, []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("weights"))
	good := hex.EncodeToString(sum[:])
	run := func(pin, file string) int {
		script := `. ./model-pins.sh; MODEL_SHA256[t]="$1"; [ -n "$1" ] || unset 'MODEL_SHA256[t]'; verify_model_pin t "$2"`
		err := exec.Command("bash", "-c", script, "_", pin, file).Run()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0
	}
	if c := run(good, f); c != 0 {
		t.Errorf("matching pin: %d, want 0", c)
	}
	if c := run(strings.ToUpper(good), f); c != 0 {
		t.Errorf("upper-case pin: %d, want 0", c)
	}
	other := sha256.Sum256([]byte("other"))
	if c := run(hex.EncodeToString(other[:]), f); c != 4 {
		t.Errorf("mismatch: %d, want 4", c)
	}
	if c := run("", f); c != 5 {
		t.Errorf("no pin: %d, want 5", c)
	}
	if c := run(good, filepath.Join(dir, "missing")); c != 6 {
		t.Errorf("missing file: %d, want 6", c)
	}
}

// Every model key switch-model.sh can load has a well-formed pin, and the
// embedder pin and revision are well-formed.
func TestModelPins_EveryModelKeyPinned(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "switch-model.sh"))
	if err != nil {
		t.Fatal(err)
	}
	keys := regexp.MustCompile(`(?m)^MODEL_FILE\[([a-z0-9-]+)\]=`).FindAllStringSubmatch(string(src), -1)
	if len(keys) == 0 {
		t.Fatal("no MODEL_FILE entries")
	}
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, k := range keys {
		out, err := exec.Command("bash", "-c", `. ./model-pins.sh; printf %s "${MODEL_SHA256[$1]:-}"`, "_", k[1]).Output()
		if err != nil {
			t.Fatal(err)
		}
		if !hex64.Match(out) {
			t.Errorf("MODEL_SHA256[%s] = %q, want 64 lower-case hex", k[1], out)
		}
	}
	out, err := exec.Command("bash", "-c", `. ./model-pins.sh; printf '%s %s' "$EMBEDDER_SHA256" "$EMBEDDER_REVISION"`).Output()
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(string(out))
	if len(f) != 2 || !hex64.MatchString(f[0]) || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(f[1]) {
		t.Errorf("embedder pin/revision malformed: %q", out)
	}
}
