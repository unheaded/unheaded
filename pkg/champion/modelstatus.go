// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// ModelStatus is scripts/switch-model.sh --json: what llama-server serves
// now, the allowlisted models, and (with --check or --dry-run) the fit
// verdict for one key. ADR-060's mitigation for the script coupling: Go
// reads this JSON, never the human output.
type ModelStatus struct {
	// Active is the key llama-server on Port is serving, "" when nothing
	// listens or the running GGUF matches no key.
	Active string      `json:"active"`
	PID    int         `json:"pid"`
	Port   int         `json:"port"`
	GGUF   string      `json:"gguf"`
	Models []ModelInfo `json:"models"`
	// Fit, Pin and Decision are set by --check and --dry-run only.
	Fit      *ModelFit `json:"fit"`
	Pin      string    `json:"pin,omitempty"`      // ok | mismatch | missing | unhashable | skipped
	Decision string    `json:"decision,omitempty"` // fits | would_swap | refused_fit | refused_pin
}

// ModelInfo is one MODEL_FILE key.
type ModelInfo struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	File    string `json:"file"`
	Present bool   `json:"present"`
	Pinned  bool   `json:"pinned"`
	// LoadSeconds is the documented load time (MODEL_LOAD_SECONDS); a
	// swap gets 4x this before it is rolled back (ADR-060 T19).
	LoadSeconds float64 `json:"load_seconds"`
}

// ModelFit is the preflight fit check's numbers for one key.
type ModelFit struct {
	Key         string `json:"key"`
	Verdict     string `json:"verdict"` // fits | refused
	WeightsMiB  int64  `json:"weights_mib"`
	NeedMiB     int64  `json:"need_mib"`
	BudgetMiB   int64  `json:"budget_mib"`
	CacheRAMMiB int64  `json:"cache_ram_mib"`
	RAMAvailMiB int64  `json:"ram_avail_mib"`
	VRAMFreeMiB int64  `json:"vram_free_mib"`
	Offload     string `json:"offload"`
}

// ParseModelStatus decodes switch-model.sh --json output. It refuses
// anything without a models list, so a script that predates --json (and
// prints its usage or a swap log) is an error, not an empty status.
func ParseModelStatus(raw []byte) (*ModelStatus, error) {
	var st ModelStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("switch-model.sh --json: %w", err)
	}
	if len(st.Models) == 0 {
		return nil, errors.New("switch-model.sh --json: no models listed")
	}
	for _, m := range st.Models {
		if !allowlistKey.MatchString(m.Key) {
			return nil, fmt.Errorf("switch-model.sh --json: bad model key %q", m.Key)
		}
	}
	return &st, nil
}

// modelStatusTimeout bounds a --json --status run (ss and /proc reads; it
// takes no GPU lease).
const modelStatusTimeout = 10 * time.Second

// ModelStatus runs the project's switch-model.sh --json --status.
func (c *Champion) ModelStatus(ctx context.Context) (*ModelStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, modelStatusTimeout)
	defer cancel()
	script := filepath.Join(c.config.ProjectRoot, "scripts", "switch-model.sh")
	cmd := exec.CommandContext(ctx, script, "--json", "--status") // #nosec G204 -- fixed script under the project root, fixed arguments
	cmd.Dir = c.config.ProjectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("switch-model.sh --json --status: %w", err)
	}
	return ParseModelStatus(out)
}

// loadSeconds returns key's documented load time from st, 0 if unknown.
func (st *ModelStatus) loadSeconds(key string) float64 {
	if st == nil {
		return 0
	}
	for _, m := range st.Models {
		if m.Key == key {
			return m.LoadSeconds
		}
	}
	return 0
}
