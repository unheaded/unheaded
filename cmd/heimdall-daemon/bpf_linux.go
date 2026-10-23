// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

// bpf_linux.go — loads the three crates/heimdall-bpf objects with
// cilium/ebpf (the Aya objects use the legacy `maps` section, which
// cilium/ebpf reads and bpftool/libbpf 1.x refuses), attaches them, and
// turns both ringbufs into hookEvents.
//
// Every attachment is a bpf_link owned by this process, so the kernel
// detaches all of it when the daemon exits, cleanly or not.
package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"

	ciliumebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
)

// Object file names under --bpf-dir (cargo [[bin]] names).
const (
	objVFSWrite = "heimdall-vfs-write"
	objExecve   = "heimdall-execve"
	objHornXDP  = "heimdall-gjallarhorn-xdp"
)

// HornStat slots in HORN_STATS (crates/heimdall-bpf/src/gjallarhorn_xdp.rs).
var hornStatNames = []string{"seen", "valid", "bad_magic", "bad_kind", "bad_len", "bad_reserved", "rb_full"}

// bpfHooks owns everything loadHooks attached.
type bpfHooks struct {
	mu        sync.Mutex
	colls     []*ciliumebpf.Collection
	links     []link.Link
	readers   []*ringbuf.Reader
	hornStats *ciliumebpf.Map
	events    chan hookEvent
	wg        sync.WaitGroup
	dropped   atomic.Uint64
	bad       atomic.Uint64
	// Verifier is processed-insn counts per program (K1 evidence).
	Verifier map[string]int
}

var processedRE = regexp.MustCompile(`processed (\d+) insns`)

// loadObject loads one Aya object with verifier stats logging on.
func loadObject(path string) (*ciliumebpf.Collection, map[string]int, error) {
	spec, err := ciliumebpf.LoadCollectionSpec(path)
	if err != nil {
		return nil, nil, fmt.Errorf("spec %s: %w", filepath.Base(path), err)
	}
	coll, err := ciliumebpf.NewCollectionWithOptions(spec, ciliumebpf.CollectionOptions{
		Programs: ciliumebpf.ProgramOptions{LogLevel: ciliumebpf.LogLevelStats},
	})
	if err != nil {
		var ve *ciliumebpf.VerifierError
		if errors.As(err, &ve) {
			return nil, nil, fmt.Errorf("verifier %s: %+v", filepath.Base(path), ve)
		}
		return nil, nil, fmt.Errorf("load %s: %w", filepath.Base(path), err)
	}
	counts := map[string]int{}
	for name, p := range coll.Programs {
		if m := processedRE.FindStringSubmatch(p.VerifierLog); m != nil {
			n, _ := strconv.Atoi(m[1])
			counts[name] = n
		}
	}
	return coll, counts, nil
}

// loadHooks loads and attaches what it can. Each failure is returned and
// the rest still loads: the daemon keeps polling without any of it.
func loadHooks(cfg HookConfig) (*bpfHooks, []error) {
	h := &bpfHooks{events: make(chan hookEvent, 1024), Verifier: map[string]int{}}
	var errs []error

	attach := func(obj string, do func(*ciliumebpf.Collection) (link.Link, error), rbName string, horn bool) {
		coll, counts, err := loadObject(filepath.Join(cfg.Dir, obj))
		if err != nil {
			errs = append(errs, err)
			return
		}
		for k, v := range counts {
			h.Verifier[k] = v
		}
		l, err := do(coll)
		if err != nil {
			coll.Close()
			errs = append(errs, fmt.Errorf("attach %s: %w", obj, err))
			return
		}
		rd, err := ringbuf.NewReader(coll.Maps[rbName])
		if err != nil {
			_ = l.Close()
			coll.Close()
			errs = append(errs, fmt.Errorf("ringbuf %s: %w", obj, err))
			return
		}
		h.colls = append(h.colls, coll)
		h.links = append(h.links, l)
		h.readers = append(h.readers, rd)
		if horn {
			h.hornStats = coll.Maps["HORN_STATS"]
		}
		h.wg.Add(1)
		go h.read(rd, horn)
	}

	attach(objVFSWrite, func(c *ciliumebpf.Collection) (link.Link, error) {
		return link.Kprobe("vfs_write", c.Programs["heimdall_vfs_write"], nil)
	}, "EVENTS", false)
	attach(objExecve, func(c *ciliumebpf.Collection) (link.Link, error) {
		return link.AttachRawTracepoint(link.RawTracepointOptions{Name: "sched_process_exec", Program: c.Programs["heimdall_execve"]})
	}, "EVENTS", false)
	if cfg.XDPIface != "" {
		attach(objHornXDP, func(c *ciliumebpf.Collection) (link.Link, error) {
			ifc, err := net.InterfaceByName(cfg.XDPIface)
			if err != nil {
				return nil, err
			}
			flags := link.XDPGenericMode
			if cfg.XDPMode == "native" {
				flags = link.XDPDriverMode
			}
			return link.AttachXDP(link.XDPOptions{Program: c.Programs["heimdall_gjallarhorn"], Interface: ifc.Index, Flags: flags})
		}, "HORN_EVENTS", true)
	}
	if len(h.readers) == 0 {
		h.Close()
		return nil, errs
	}
	return h, errs
}

func (h *bpfHooks) read(rd *ringbuf.Reader, horn bool) {
	defer h.wg.Done()
	for {
		rec, err := rd.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) || errors.Is(err, os.ErrClosed) {
				return
			}
			continue
		}
		var ev hookEvent
		if horn {
			he, err := DecodeHornEvent(rec.RawSample)
			if err != nil {
				h.bad.Add(1)
				continue
			}
			ev.Horn = &he
		} else {
			de, err := DecodeDriftEvent(rec.RawSample)
			if err != nil {
				h.bad.Add(1)
				continue
			}
			ev.Drift = &de
		}
		select {
		case h.events <- ev:
		default:
			h.dropped.Add(1) // a slow daemon never stalls the reader
		}
	}
}

// Events is the merged stream of decoded hook records.
func (h *bpfHooks) Events() <-chan hookEvent { return h.events }

// HornCounters sums the per-CPU XDP observer counters.
func (h *bpfHooks) HornCounters() map[string]uint64 {
	out := map[string]uint64{}
	if h == nil || h.hornStats == nil {
		return out
	}
	for i, name := range hornStatNames {
		var per []uint64
		if err := h.hornStats.Lookup(uint32(i), &per); err != nil {
			continue
		}
		var sum uint64
		for _, v := range per {
			sum += v
		}
		out[name] = sum
	}
	return out
}

// Close detaches and unloads everything, then waits for the readers.
func (h *bpfHooks) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, rd := range h.readers {
		_ = rd.Close()
	}
	h.wg.Wait()
	for _, l := range h.links {
		_ = l.Close()
	}
	for _, c := range h.colls {
		c.Close()
	}
	h.readers, h.links, h.colls = nil, nil, nil
}
