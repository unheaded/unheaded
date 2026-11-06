// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestGoLoaderLoadsRealObjects loads the objects trace-collector loads through
// this package, on the running kernel, and unloads them without attaching.
// scripts/check-ebpf-loads.sh runs it as root with EBPF_GO_LOAD_DIR set: that
// gate's own loader is Rust, so this is the only proof the Go path works.
func TestGoLoaderLoadsRealObjects(t *testing.T) {
	dir := os.Getenv("EBPF_GO_LOAD_DIR")
	if dir == "" {
		t.Skip("EBPF_GO_LOAD_DIR is not set; scripts/check-ebpf-loads.sh sets it")
	}
	if os.Geteuid() != 0 {
		t.Fatal("EBPF_GO_LOAD_DIR is set but this is not root: the load cannot be proven")
	}
	pin, err := os.MkdirTemp("/sys/fs/bpf", "go-load-gate-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(pin) })

	for _, c := range []struct {
		object string
		typ    ProgramType
	}{{"packet-marker", TypeXDP}, {"flow-tracker", TypeTC}} {
		t.Run(c.object, func(t *testing.T) {
			cfg := DefaultLoaderConfig()
			cfg.PinPath = pin
			cfg.MetricsEnabled = false
			cfg.AllowUntrustedObjectPaths = true // a build tree, owned by whoever built it
			l, err := NewNativeLoader(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			spec := &ProgramSpec{Name: "go_load_gate", Path: filepath.Join(dir, c.object), Type: c.typ}
			if err := l.Load(ctx, spec); err != nil {
				t.Fatalf("load: %.400v", err)
			}
			if err := l.Unload(ctx, spec.Name); err != nil {
				t.Fatalf("unload: %v", err)
			}
		})
	}
}
