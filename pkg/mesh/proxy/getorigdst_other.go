// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build !linux

package proxy

import (
	"fmt"
	"net"
)

// getOriginalDst is not supported on non-Linux platforms.
// SO_ORIGINAL_DST requires Linux with netfilter support.
func getOriginalDst(_ net.Conn) (string, error) {
	return "", fmt.Errorf("GetOriginalDst requires Linux with netfilter SO_ORIGINAL_DST support")
}
