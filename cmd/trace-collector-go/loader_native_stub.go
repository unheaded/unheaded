// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build !linux
// +build !linux

// loader_native_stub.go — Non-Linux stub for NativeBPFLoader.
//
// On non-Linux platforms, BPF programs cannot be loaded. This stub ensures
// the code compiles and gracefully falls back to MockBPFLoader.
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package main

import (
	"context"
	"fmt"
)

// NativeBPFLoaderConfig holds configuration for the native BPF loader.
type NativeBPFLoaderConfig struct {
	PinPath string
	Debug   bool
}

// DefaultNativeBPFLoaderConfig returns sensible defaults.
func DefaultNativeBPFLoaderConfig() NativeBPFLoaderConfig {
	return NativeBPFLoaderConfig{
		PinPath: "/sys/fs/bpf/unheaded",
	}
}

// NewNativeBPFLoader always returns an error on non-Linux platforms.
func NewNativeBPFLoader(_ context.Context, _ NativeBPFLoaderConfig) (*MockBPFLoader, error) {
	return nil, fmt.Errorf("BPF programs are only supported on Linux")
}
