// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package hoprules

import (
	"encoding/binary"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// oneWrite records what SetMode wrote.
type oneWrite struct {
	writes [][3]string
	fail   error
}

func (w *oneWrite) Update(mapName string, key, value []byte) error {
	if w.fail != nil {
		return w.fail
	}
	w.writes = append(w.writes, [3]string{mapName, string(key), string(value)})
	return nil
}

func TestSetMode_RefusesEnforceAndWritesNothing(t *testing.T) {
	w := &oneWrite{}
	err := SetMode(w, ModeEnforce)
	if !errors.Is(err, ErrEnforceRefused) {
		t.Fatalf("err = %v, want ErrEnforceRefused", err)
	}
	for _, id := range EnforceBlockers {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("refusal does not name %s: %v", id, err)
		}
	}
	if len(w.writes) != 0 {
		t.Errorf("a refusal wrote %v", w.writes)
	}
}

// off and observe are what CONFIG key 4 takes today, as hop-ebpf reads it: a
// u32 key and a u64 value, little-endian.
func TestSetMode_WritesOffAndObserve(t *testing.T) {
	for _, m := range []CheckMode{ModeOff, ModeObserve} {
		w := &oneWrite{}
		if err := SetMode(w, m); err != nil {
			t.Fatalf("mode %d: %v", m, err)
		}
		if len(w.writes) != 1 || w.writes[0][0] != "CONFIG" {
			t.Fatalf("mode %d: writes %v", m, w.writes)
		}
		if k := []byte(w.writes[0][1]); len(k) != 4 || binary.LittleEndian.Uint32(k) != 4 {
			t.Errorf("mode %d: key %x", m, k)
		}
		if v := []byte(w.writes[0][2]); len(v) != 8 || binary.LittleEndian.Uint64(v) != uint64(m) {
			t.Errorf("mode %d: value %x", m, v)
		}
	}
}

// With no blocker left, enforce writes 2: the refusal is the list, nothing else.
func TestSetMode_EnforceOnceNothingBlocks(t *testing.T) {
	w := &oneWrite{}
	if err := setMode(w, ModeEnforce, nil); err != nil {
		t.Fatal(err)
	}
	if v := []byte(w.writes[0][2]); binary.LittleEndian.Uint64(v) != 2 {
		t.Errorf("value %x", v)
	}
}

func TestSetMode_MapFailureIsReported(t *testing.T) {
	boom := errors.New("boom")
	if err := SetMode(&oneWrite{fail: boom}, ModeObserve); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseMode(t *testing.T) {
	for s, want := range map[string]CheckMode{"off": ModeOff, "observe": ModeObserve, "enforce": ModeEnforce} {
		if got, err := ParseMode(s); err != nil || got != want {
			t.Errorf("%s: %d, %v", s, got, err)
		}
	}
	for _, s := range []string{"", "2", "ENFORCE", "drop"} {
		if _, err := ParseMode(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

// The list follows the register: a finding that closes leaves it in the same
// change, and none leaves it while it is open.
func TestEnforceBlockers_FollowTheRegister(t *testing.T) {
	raw, err := os.ReadFile("../../compliance/findings/register.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Findings []struct {
			ID       string         `yaml:"id"`
			Resolved map[string]any `yaml:"resolved"`
		} `yaml:"findings"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	open := map[string]bool{}
	for _, f := range doc.Findings {
		if f.ID != "" && f.Resolved == nil {
			open[f.ID] = true
		}
	}
	for _, id := range []string{"FND-031", "FND-033", "FND-035"} {
		if listed := slices.Contains(EnforceBlockers, id); listed != open[id] {
			t.Errorf("%s: in EnforceBlockers %v, open in the register %v", id, listed, open[id])
		}
	}
}
