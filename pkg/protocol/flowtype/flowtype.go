// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package flowtype provides typed flow classification for the Unheaded protocol.
// It implements H6 finding: Flow type classification with control, data, and prefetch types.
package flowtype

import (
	"fmt"

	"unheaded/pkg/logger"
)

// FlowType represents the type of a flow.
type FlowType uint8

// Defined flow types.
const (
	Control  FlowType = 0x00
	Data     FlowType = 0x01
	Prefetch FlowType = 0x02
	Reserved FlowType = 0x03
)

// FlowTypeMask represents the 2-bit mask for flow type in Monad flags.
const FlowTypeMask = 0x03

// String returns the string representation of a FlowType.
func (ft FlowType) String() string {
	switch ft {
	case Control:
		return "Control"
	case Data:
		return "Data"
	case Prefetch:
		return "Prefetch"
	case Reserved:
		return "Reserved"
	default:
		return fmt.Sprintf("FlowType(%d)", ft)
	}
}

// IsControl returns true if the flow type is Control.
func (ft FlowType) IsControl() bool {
	return ft == Control
}

// IsData returns true if the flow type is Data.
func (ft FlowType) IsData() bool {
	return ft == Data
}

// IsPrefetch returns true if the flow type is Prefetch.
func (ft FlowType) IsPrefetch() bool {
	return ft == Prefetch
}

// IsReserved returns true if the flow type is Reserved.
func (ft FlowType) IsReserved() bool {
	return ft == Reserved
}

// Priority returns a priority value for the flow type.
// Lower values have higher priority.
// Control > Data > Prefetch
func (ft FlowType) Priority() int {
	switch ft {
	case Control:
		return 0
	case Data:
		return 1
	case Prefetch:
		return 2
	default:
		return 3
	}
}

// FlowTypeFromFlags extracts the flow type from Monad flags.
// Flow type is encoded in the lowest 2 bits of the flags byte.
func FlowTypeFromFlags(flags uint8) FlowType {
	ft := FlowType(flags & FlowTypeMask)
	logger.Debug().Msgf("extracted flow type from flags: flags=0x%02X type=%v", flags, ft)
	return ft
}

// FlowTypeToFlags sets the flow type in Monad flags.
// This clears the lowest 2 bits and sets them to the flow type value.
// The existing value parameter provides any additional flags to preserve.
func FlowTypeToFlags(ft FlowType, existing uint8) (uint8, error) {
	if ft > Reserved {
		return 0, fmt.Errorf("flowtype: invalid flow type %d", ft)
	}

	// Clear the flow type bits and set the new value
	result := (existing & ^uint8(FlowTypeMask)) | (uint8(ft) & FlowTypeMask)

	logger.Debug().Msgf("set flow type in flags: type=%v existing=0x%02X result=0x%02X", ft, existing, result)

	return result, nil
}

// IsHigherPriority returns true if this flow type has higher priority than another.
func (ft FlowType) IsHigherPriority(other FlowType) bool {
	return ft.Priority() < other.Priority()
}

// Validate returns an error if the flow type is invalid (Reserved should typically not be used).
func (ft FlowType) Validate() error {
	if ft == Reserved {
		return fmt.Errorf("flowtype: reserved flow type should not be used in production")
	}
	if ft > Reserved {
		return fmt.Errorf("flowtype: invalid flow type %d", ft)
	}
	return nil
}
