// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux

// latency_kprobe.go — Direct cilium/ebpf loader for latency-probe kprobes.
// The standard loader only attaches a single kprobe. This uses cilium/ebpf's
// LoadCollectionSpec to properly load all kprobe/kretprobe functions from
// the Aya-compiled ELF and attach them to their kernel functions.
package main

import (
	"fmt"
	"os"

	ciliumebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/rs/zerolog/log"
)

// LatencyKprobeLoader loads and attaches all kprobe/kretprobe functions
// from the latency-probe BPF ELF using cilium/ebpf directly.
type LatencyKprobeLoader struct {
	collection *ciliumebpf.Collection
	links      []link.Link
}

// kprobeAttachment maps BPF program names to kernel function attach targets.
var kprobeAttachments = []struct {
	progName   string
	kernelFunc string
	isRet      bool
}{
	{"tcp_sendmsg_enter", "tcp_sendmsg", false},
	{"tcp_sendmsg_exit", "tcp_sendmsg", true},
	{"tcp_recvmsg_enter", "tcp_recvmsg", false},
	{"tcp_recvmsg_exit", "tcp_recvmsg", true},
	{"tcp_connect_enter", "tcp_v4_connect", false},
	{"tcp_connect_exit", "tcp_v4_connect", true},
}

// LoadLatencyKprobes loads the latency-probe ELF and attaches all kprobe functions.
// Returns the LATENCY_MAP for reading by LatencyReader.
func LoadLatencyKprobes(elfPath string) (*LatencyKprobeLoader, *ciliumebpf.Map, error) {
	f, err := os.Open(elfPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open ELF: %w", err)
	}
	defer f.Close()

	spec, err := ciliumebpf.LoadCollectionSpecFromReader(f)
	if err != nil {
		return nil, nil, fmt.Errorf("load collection spec: %w", err)
	}

	// Log what programs were found
	for name, ps := range spec.Programs {
		log.Info().Str("name", name).Str("type", ps.Type.String()).Str("section", ps.SectionName).Msg("found BPF program in ELF")
	}
	for name := range spec.Maps {
		log.Info().Str("name", name).Msg("found BPF map in ELF")
	}

	// Load the collection (all programs + maps)
	coll, err := ciliumebpf.NewCollection(spec)
	if err != nil {
		return nil, nil, fmt.Errorf("load collection: %w", err)
	}

	loader := &LatencyKprobeLoader{
		collection: coll,
	}

	// Attach each kprobe/kretprobe function
	attached := 0
	for _, ka := range kprobeAttachments {
		prog, ok := coll.Programs[ka.progName]
		if !ok {
			log.Warn().Str("prog", ka.progName).Msg("program not found in collection, skipping")
			continue
		}

		var l link.Link
		if ka.isRet {
			l, err = link.Kretprobe(ka.kernelFunc, prog, nil)
		} else {
			l, err = link.Kprobe(ka.kernelFunc, prog, nil)
		}
		if err != nil {
			log.Error().Err(err).
				Str("prog", ka.progName).
				Str("kernel_func", ka.kernelFunc).
				Bool("kretprobe", ka.isRet).
				Msg("failed to attach kprobe")
			continue
		}

		loader.links = append(loader.links, l)
		attached++

		attachType := "kprobe"
		if ka.isRet {
			attachType = "kretprobe"
		}
		log.Info().
			Str("prog", ka.progName).
			Str("kernel_func", ka.kernelFunc).
			Str("type", attachType).
			Msg("kprobe attached")
	}

	if attached == 0 {
		coll.Close()
		return nil, nil, fmt.Errorf("no kprobes could be attached")
	}

	log.Info().Int("attached", attached).Int("total", len(kprobeAttachments)).Msg("latency kprobes loaded")

	// Return the LATENCY_MAP for reading
	latencyMap, ok := coll.Maps["LATENCY_MAP"]
	if !ok {
		log.Warn().Msg("LATENCY_MAP not found in collection")
	}

	return loader, latencyMap, nil
}

// Close detaches all kprobes and releases the collection.
func (l *LatencyKprobeLoader) Close() {
	for _, lnk := range l.links {
		lnk.Close()
	}
	if l.collection != nil {
		l.collection.Close()
	}
}
