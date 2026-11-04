// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"errors"

	"unheaded/services/compliance"
)

// consumeAuditRing is replaced by the ring consumer in Phase 5.
func consumeAuditRing(_ context.Context, _ string, _ *compliance.WireComplianceService) error {
	return errors.New("--audit-ring is not built yet")
}
