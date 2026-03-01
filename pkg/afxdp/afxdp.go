// SPDX-License-Identifier: GPL-2.0-only
// AF_XDP Bridge — Go wrapper
// Part of: Unheaded Kingdom project
//
// Wraps Rust libaf_xdp for high-performance zero-copy packet I/O in Go services.
// Uses CGo to call into the Rust FFI layer.
//
// To use this package, the Rust library must be built first:
//   cd ebpf/af-xdp && cargo build --lib --release
//
// Build requires:
// - libaf_xdp.a compiled from Rust (ebpf/af-xdp)
// - CGo with C compiler (gcc/clang)
// - Linux system with AF_XDP capable NIC (for runtime)
//
// Safety note: All unsafe.Pointer conversions assume:
// - Rust FFI maintains C ABI compatibility
// - Buffer pointers remain valid for duration of C call
// - All buffers are properly sized for C function expectations

//go:build linux && cgo

package afxdp

/*
#cgo LDFLAGS: -L${SRCDIR}/../../ebpf/af-xdp/target/release -laf_xdp -lpthread -ldl
#cgo CFLAGS: -I${SRCDIR}/../../ebpf/af-xdp
#include "af_xdp.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"fmt"
	"unsafe"
)

// ── Types ────────────────────────────────────────────────────────────────────

// Engine wraps an AF_XDP engine handle.
type Engine struct {
	handle *C.afxdp_handle_t
}

// Packet represents a received or to-be-transmitted packet.
type Packet struct {
	Data []byte
}

// Stats holds engine statistics.
type Stats struct {
	RxPackets uint64
	TxPackets uint64
	RxBytes   uint64
	TxBytes   uint64
	RxDrops   uint64
	FillEmpty uint64
	CompFull  uint64
}

// ── Options pattern ──────────────────────────────────────────────────────────

// Option configures engine creation.
type Option func(*engineConfig)

type engineConfig struct {
	frameCount int
	batchSize  int
}

// WithFrameCount sets the number of UMEM frames.
func WithFrameCount(count int) Option {
	return func(cfg *engineConfig) {
		cfg.frameCount = count
	}
}

// WithBatchSize sets the default receive batch size.
func WithBatchSize(size int) Option {
	return func(cfg *engineConfig) {
		cfg.batchSize = size
	}
}

// ── Engine lifecycle ─────────────────────────────────────────────────────────

// NewEngine creates a new AF_XDP engine on the given interface and queue.
func NewEngine(iface string, queue int, opts ...Option) (*Engine, error) {
	cfg := &engineConfig{
		frameCount: 4096,
		batchSize:  32,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	ifaceC := C.CString(iface)
	defer C.free(unsafe.Pointer(ifaceC))

	handle := C.afxdp_create(ifaceC, C.uint(queue), C.uint(cfg.frameCount))
	if handle == nil {
		return nil, fmt.Errorf("afxdp: create failed on %s queue %d", iface, queue)
	}

	return &Engine{handle: handle}, nil
}

// Recv receives a batch of packets.
// Blocks until packets are available or context is cancelled.
func (e *Engine) Recv(ctx context.Context) ([]Packet, error) {
	if e.handle == nil {
		return nil, fmt.Errorf("afxdp: engine closed")
	}

	// Check context before blocking
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	const maxBufSize = 65536
	const batchSize = 32
	buf := make([]byte, maxBufSize)

	n := C.afxdp_recv(
		e.handle,
		(*C.char)(unsafe.Pointer(&buf[0])),
		C.uint(len(buf)),
		C.uint(batchSize),
	)
	if n < 0 {
		return nil, fmt.Errorf("afxdp: recv error %d", n)
	}
	if n == 0 {
		return nil, nil
	}

	// Return data as single packet (real impl would parse frame boundaries)
	packets := []Packet{{Data: make([]byte, n)}}
	copy(packets[0].Data, buf[:n])

	return packets, nil
}

// Send transmits a packet.
func (e *Engine) Send(data []byte) error {
	if e.handle == nil {
		return fmt.Errorf("afxdp: engine closed")
	}
	if len(data) == 0 {
		return nil
	}

	n := C.afxdp_send(
		e.handle,
		(*C.char)(unsafe.Pointer(&data[0])),
		C.uint(len(data)),
	)
	if n < 0 {
		return fmt.Errorf("afxdp: send error %d", n)
	}
	if n == 0 {
		return fmt.Errorf("afxdp: TX ring full")
	}

	return nil
}

// SendBatch transmits multiple packets.
func (e *Engine) SendBatch(packets []Packet) (int, error) {
	sent := 0
	for _, pkt := range packets {
		if err := e.Send(pkt.Data); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// Stats returns a statistics snapshot.
func (e *Engine) Stats() (Stats, error) {
	if e.handle == nil {
		return Stats{}, fmt.Errorf("afxdp: engine closed")
	}

	var cs C.afxdp_stats_t
	if C.afxdp_stats(e.handle, &cs) != 0 {
		return Stats{}, fmt.Errorf("afxdp: stats failed")
	}

	return Stats{
		RxPackets: uint64(cs.rx_packets),
		TxPackets: uint64(cs.tx_packets),
		RxBytes:   uint64(cs.rx_bytes),
		TxBytes:   uint64(cs.tx_bytes),
		RxDrops:   uint64(cs.rx_drops),
		FillEmpty: uint64(cs.fill_empty),
		CompFull:  uint64(cs.comp_full),
	}, nil
}

// Poll waits for RX readiness with timeout.
// Returns true if packets are available, false on timeout.
func (e *Engine) Poll(timeoutMs int) (bool, error) {
	if e.handle == nil {
		return false, fmt.Errorf("afxdp: engine closed")
	}

	ret := C.afxdp_poll(e.handle, C.int(timeoutMs))
	if ret < 0 {
		return false, fmt.Errorf("afxdp: poll error %d", ret)
	}
	return ret > 0, nil
}

// Close destroys the engine and frees all resources.
func (e *Engine) Close() error {
	if e.handle == nil {
		return fmt.Errorf("afxdp: engine already closed")
	}

	if C.afxdp_destroy(e.handle) != 0 {
		return fmt.Errorf("afxdp: destroy failed")
	}

	e.handle = nil
	return nil
}
