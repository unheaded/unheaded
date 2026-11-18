// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package transport

import (
	"strings"
	"testing"
)

// The process's one Wotan client streams over gRPC (ADR-147 Phase 7): both
// addresses give a client; an empty gRPC address is refused by name.
func TestNewWotanClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WotanHTTPAddr = "http://127.0.0.1:1"
	cfg.WotanGRPCAddr = "127.0.0.1:1" // nothing listens: the client starts degraded
	c, err := NewWotanClient(cfg)
	if err != nil {
		t.Fatalf("NewWotanClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if tr := c.Health().Transport; tr == "" {
		t.Error("Health().Transport is empty; a gRPC client names its transport")
	}

	cfg.WotanGRPCAddr = ""
	if _, err := NewWotanClient(cfg); err == nil || !strings.Contains(err.Error(), "WOTAN_GRPC_ADDR") {
		t.Fatalf("empty gRPC address: err = %v, want one naming WOTAN_GRPC_ADDR", err)
	}

	cfg = DefaultConfig()
	cfg.WotanHTTPAddr = ""
	if _, err := NewWotanClient(cfg); err == nil {
		t.Fatal("empty HTTP address: no error")
	}
}
