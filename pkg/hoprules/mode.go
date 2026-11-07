// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package hoprules

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// CheckMode is hop-ebpf's CONFIG key 4 (ADR-125).
type CheckMode uint64

// The modes hop-ebpf knows. Anything else reads as off.
const (
	ModeOff     CheckMode = 0
	ModeObserve CheckMode = 1
	ModeEnforce CheckMode = 2
)

const (
	configMap      = "CONFIG"
	configKeyCheck = uint32(4)
)

// EnforceBlockers are the register entries enforce waits for. Stevie
// 2026-11-06: enforce is refused until they close, because the severity of each
// rises the moment enforce is on. A test holds this list to the register.
var EnforceBlockers = []string{"FND-031", "FND-033", "FND-035"}

// ErrEnforceRefused is SetMode's answer while EnforceBlockers holds an entry.
var ErrEnforceRefused = errors.New("hoprules: enforce refused")

// ParseMode reads a mode name.
func ParseMode(s string) (CheckMode, error) {
	switch s {
	case "off":
		return ModeOff, nil
	case "observe":
		return ModeObserve, nil
	case "enforce":
		return ModeEnforce, nil
	}
	return 0, fmt.Errorf("hoprules: unknown mode %q (want off, observe or enforce)", s)
}

// SetMode writes CONFIG key 4 through w. Enforce is refused, and nothing is
// written, while a blocker is open.
func SetMode(w MapWriter, m CheckMode) error {
	return setMode(w, m, EnforceBlockers)
}

func setMode(w MapWriter, m CheckMode, blockers []string) error {
	if m == ModeEnforce && len(blockers) > 0 {
		return fmt.Errorf("%w: open until %s close", ErrEnforceRefused, strings.Join(blockers, ", "))
	}
	key := binary.LittleEndian.AppendUint32(nil, configKeyCheck)
	value := binary.LittleEndian.AppendUint64(nil, uint64(m))
	if err := w.Update(configMap, key, value); err != nil {
		return fmt.Errorf("hoprules: %s: %w", configMap, err)
	}
	return nil
}
