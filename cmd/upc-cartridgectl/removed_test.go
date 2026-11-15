// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package main

import (
	"errors"
	"strings"
	"testing"
)

// The Sophia HTTP service was removed (ADR-147 decision 7): --sophia is
// refused by name, and the registry is the file store.
func TestSophiaFlagRefused(t *testing.T) {
	t.Parallel()
	g := &globals{sophiaURL: "https://sophia:19005", registryDir: t.TempDir()}
	err := g.refuseRemoved()
	if !errors.Is(err, errSophiaRemoved) || !strings.Contains(err.Error(), "--sophia") {
		t.Fatalf("refuseRemoved = %v, want errSophiaRemoved naming --sophia", err)
	}
	if _, ok := g.registry().(*FileRegistry); !ok {
		t.Fatalf("registry() is %T, want *FileRegistry", g.registry())
	}
}

func TestNoSophiaFlagAccepted(t *testing.T) {
	t.Parallel()
	if err := (&globals{}).refuseRemoved(); err != nil {
		t.Fatalf("refuseRemoved with no --sophia = %v", err)
	}
}
