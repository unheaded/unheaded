// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux
// +build linux

// loader_native.go — NativeBPFLoader bridges BPFLoader to pkg/ebpf.NativeLoader.
//
// This file implements the production BPF loader that delegates to the
// Kingdom's native BPF syscall infrastructure (pkg/ebpf). It loads eBPF ELF
// binaries compiled by Aya/Rust and attaches them via XDP, TC, kprobe, or
// tracepoint depending on the program type.
//
// The NativeMapReader wraps pkg/ebpf's map operations (IterateMap, ReadMap,
// DeleteMapEntry) to satisfy the MapReader interface used by TraceReader,
// FlowReader, and LatencyReader.
//
// Per RFC 9669: we use "BPF" (not "eBPF") throughout.
package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"

	"unheaded/pkg/ebpf"
)

// ── NativeBPFLoader ─────────────────────────────────────────────────────────

// NativeBPFLoader wraps pkg/ebpf.NativeLoader for production BPF operations.
// It satisfies the BPFLoader interface and delegates all syscall-level
// operations to the Kingdom's native loader.
type NativeBPFLoader struct {
	mu     sync.Mutex
	loader *ebpf.NativeLoader
	ctx    context.Context

	// loaded programs keyed by logical name (packet_marker, flow_tracker, latency_probe)
	programs map[string]*nativeProgram

	closed bool
}

// nativeProgram tracks a loaded BPF program and its map assignments.
type nativeProgram struct {
	name     string         // logical name (e.g., "packet_marker")
	specName string         // ProgramSpec.Name used with the loader
	progType ebpf.ProgramType
	path     string
	attached bool
}

// NativeBPFLoaderConfig holds configuration for the native BPF loader.
type NativeBPFLoaderConfig struct {
	PinPath string // BPF filesystem pin path (default: /sys/fs/bpf/unheaded)
	Debug   bool   // Enable BPF verifier debug output
}

// DefaultNativeBPFLoaderConfig returns sensible defaults.
func DefaultNativeBPFLoaderConfig() NativeBPFLoaderConfig {
	return NativeBPFLoaderConfig{
		PinPath: "/sys/fs/bpf/unheaded",
	}
}

// NewNativeBPFLoader creates a new native BPF loader backed by pkg/ebpf.NativeLoader.
func NewNativeBPFLoader(ctx context.Context, cfg NativeBPFLoaderConfig) (*NativeBPFLoader, error) {
	loaderCfg := ebpf.DefaultLoaderConfig()
	loaderCfg.PinPath = cfg.PinPath
	loaderCfg.Debug = cfg.Debug
	loaderCfg.AllowRlimit = true
	loaderCfg.MetricsEnabled = true

	native, err := ebpf.NewNativeLoader(loaderCfg)
	if err != nil {
		return nil, fmt.Errorf("create native loader: %w", err)
	}

	return &NativeBPFLoader{
		loader:   native,
		ctx:      ctx,
		programs: make(map[string]*nativeProgram),
	}, nil
}

// ── BPFLoader interface implementation ──────────────────────────────────────

// Load loads a BPF program from an ELF object file.
// The programPath determines the program type:
//   - Contains "packet-marker" or "packet_marker" → XDP
//   - Contains "flow-tracker" or "flow_tracker" → TC
//   - Contains "latency-probe" or "latency_probe" → kprobe
func (n *NativeBPFLoader) Load(programPath string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("loader closed")
	}

	name, progType := classifyProgram(programPath)
	specName := name // use logical name as spec name

	spec := &ebpf.ProgramSpec{
		Name: specName,
		Path: programPath,
		Type: progType,
	}

	// Set attach targets based on program type
	switch progType {
	case ebpf.TypeKProbe:
		spec.AttachTo = "tcp_rcv_established"
	case ebpf.TypeTracepoint:
		spec.AttachTo = "tcp/tcp_probe"
	}

	log.Info().
		Str("name", name).
		Str("path", programPath).
		Str("type", string(progType)).
		Msg("loading BPF program")

	if err := n.loader.Load(n.ctx, spec); err != nil {
		return fmt.Errorf("load %s: %w", name, err)
	}

	n.programs[name] = &nativeProgram{
		name:     name,
		specName: specName,
		progType: progType,
		path:     programPath,
	}

	log.Info().Str("name", name).Msg("BPF program loaded")
	return nil
}

// AttachXDP attaches the loaded XDP program to a network interface.
// Also attaches any TC and kprobe programs that have been loaded.
func (n *NativeBPFLoader) AttachXDP(iface string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return fmt.Errorf("loader closed")
	}

	// Attach all loaded programs
	for name, prog := range n.programs {
		if prog.attached {
			continue
		}

		// For XDP and TC programs, set the interface as the attach target
		switch prog.progType {
		case ebpf.TypeXDP, ebpf.TypeTC:
			if err := n.loader.SetAttachTarget(n.ctx, prog.specName, iface); err != nil {
				log.Error().Err(err).Str("name", name).Msg("failed to set attach target")
				continue
			}
		}

		if err := n.loader.Attach(n.ctx, prog.specName); err != nil {
			log.Error().Err(err).
				Str("name", name).
				Str("type", string(prog.progType)).
				Msg("failed to attach BPF program")
			// Continue attaching other programs
			continue
		}

		prog.attached = true
		log.Info().
			Str("name", name).
			Str("type", string(prog.progType)).
			Str("iface", iface).
			Msg("BPF program attached")
	}

	return nil
}

// GetTraceMap returns a reader for the packet_marker STATS map.
// Note: PACKET_EVENTS is a ringbuf which requires mmap-based reading.
// Until ringbuf support is added, we read from the STATS hash map
// which contains per-event-type counters.
func (n *NativeBPFLoader) GetTraceMap() MapReader {
	return n.mapReader("packet_marker", "STATS")
}

// GetStatsMap returns a reader for the flow_tracker FLOW_STATS map.
func (n *NativeBPFLoader) GetStatsMap() MapReader {
	return n.mapReader("flow_tracker", "FLOW_STATS")
}

// GetFlowStateMap returns a reader for the flow_tracker FLOWS map.
func (n *NativeBPFLoader) GetFlowStateMap() MapReader {
	return n.mapReader("flow_tracker", "FLOWS")
}

// GetLatencyMap returns a reader for the latency_probe LATENCY_MAP.
func (n *NativeBPFLoader) GetLatencyMap() MapReader {
	return n.mapReader("latency_probe", "LATENCY_MAP")
}

// GetPacketEventsCh returns a channel streaming raw packet events from the
// PACKET_EVENTS ring buffer. Uses mmap-based reading via pkg/ebpf.ReadRingbuf.
// Returns nil if the packet_marker program isn't loaded or ringbuf unavailable.
func (n *NativeBPFLoader) GetPacketEventsCh(ctx context.Context) <-chan []byte {
	n.mu.Lock()
	prog, exists := n.programs["packet_marker"]
	n.mu.Unlock()

	if !exists || !prog.attached {
		return nil
	}

	ch, err := n.loader.ReadRingbuf(ctx, prog.specName, "PACKET_EVENTS")
	if err != nil {
		log.Warn().Err(err).Msg("failed to open PACKET_EVENTS ringbuf, falling back to poll mode")
		return nil
	}
	return ch
}

// Close detaches all programs and releases resources.
func (n *NativeBPFLoader) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return nil
	}
	n.closed = true

	log.Info().Int("programs", len(n.programs)).Msg("closing native BPF loader")
	return n.loader.Close()
}

// ── Internal helpers ────────────────────────────────────────────────────────

// mapReader creates a NativeMapReader for a given program and map name.
// Returns nil if the program isn't loaded (readers handle nil gracefully).
func (n *NativeBPFLoader) mapReader(programName, mapName string) MapReader {
	n.mu.Lock()
	prog, exists := n.programs[programName]
	n.mu.Unlock()

	if !exists || !prog.attached {
		return nil
	}

	return &NativeMapReader{
		loader:      n.loader,
		ctx:         n.ctx,
		programName: prog.specName,
		mapName:     mapName,
	}
}

// classifyProgram determines the logical name and BPF program type from a path.
func classifyProgram(path string) (string, ebpf.ProgramType) {
	// Match against known program names
	switch {
	case containsAny(path, "packet-marker", "packet_marker"):
		return "packet_marker", ebpf.TypeXDP
	case containsAny(path, "flow-tracker", "flow_tracker"):
		return "flow_tracker", ebpf.TypeTC
	case containsAny(path, "latency-probe", "latency_probe"):
		return "latency_probe", ebpf.TypeKProbe
	default:
		// Default to XDP for unknown programs
		return "unknown", ebpf.TypeXDP
	}
}

// containsAny returns true if s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// ── NativeMapReader ─────────────────────────────────────────────────────────

// NativeMapReader wraps pkg/ebpf.NativeLoader map operations to satisfy
// the MapReader interface. Each reader is bound to a specific program and map.
type NativeMapReader struct {
	loader      *ebpf.NativeLoader
	ctx         context.Context
	programName string
	mapName     string
}

// Iterate calls fn for each key-value pair in the BPF map.
func (r *NativeMapReader) Iterate(fn func(key, value []byte) error) error {
	return r.loader.IterateMap(r.ctx, r.programName, r.mapName, fn)
}

// Lookup reads the value for a given key from the BPF map.
func (r *NativeMapReader) Lookup(key []byte) ([]byte, error) {
	return r.loader.ReadMap(r.ctx, r.programName, r.mapName, key)
}

// Delete removes a key from the BPF map.
func (r *NativeMapReader) Delete(key []byte) error {
	return r.loader.DeleteMapEntry(r.ctx, r.programName, r.mapName, key)
}

// Len returns the approximate number of entries. This requires a full iteration
// so we return 0 to avoid expensive operations on the hot path.
func (r *NativeMapReader) Len() int {
	return 0
}
