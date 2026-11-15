// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"strings"
	"testing"

	"unheaded/pkg/logger"
)

// TestMockBackendMustBeAskedFor: Cuirass never reconciles a mock because a
// variable is missing (ADR-147 decisions 7, 14). An unset LXD_BACKEND is a
// configuration error; "mock" must be asked for, and says so once at warn.
func TestMockBackendMustBeAskedFor(t *testing.T) {
	t.Run("unset refuses", func(t *testing.T) {
		c, err := newLXDClient(&Config{LXDBackend: ""}, logger.New(&bytes.Buffer{}))
		if err == nil || !strings.Contains(err.Error(), "LXD_BACKEND is not set") {
			t.Fatalf("err = %v, want one naming LXD_BACKEND is not set", err)
		}
		if c != nil {
			t.Fatalf("client = %T, want none", c)
		}
	})
	t.Run("mock when asked, with a warning", func(t *testing.T) {
		var buf bytes.Buffer
		c, err := newLXDClient(&Config{LXDBackend: "mock"}, logger.New(&buf))
		if err != nil {
			t.Fatalf("mock: %v", err)
		}
		if _, ok := c.(*MockLXDClient); !ok {
			t.Fatalf("client = %T, want *MockLXDClient", c)
		}
		out := buf.String()
		if strings.Count(out, "mock LXD backend: nothing here is reconciled against a real LXD") != 1 || !strings.Contains(out, `"level":"warn"`) {
			t.Fatalf("want one warn line, got %q", out)
		}
	})
	t.Run("unknown refuses", func(t *testing.T) {
		if _, err := newLXDClient(&Config{LXDBackend: "lxc"}, logger.New(&bytes.Buffer{})); err == nil {
			t.Fatal("unknown backend accepted")
		}
	})
	t.Run("the environment has no default", func(t *testing.T) {
		t.Setenv("LXD_BACKEND", "")
		if got := loadConfig("").LXDBackend; got != "" {
			t.Fatalf("LXDBackend with LXD_BACKEND unset = %q, want empty", got)
		}
	})
}
