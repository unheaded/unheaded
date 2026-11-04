// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"

	pb "unheaded/proto/unheaded/v1"
)

// Maps --mode=bpf reads, relative to --bpf-maps. cmd/ebpf-loader --pin-maps
// pins a program's maps at <base>/<program>/<MAP>.
const (
	mapSophia = "hop-ebpf/SOPHIA"    // HashMap<u16, SophiaEntry>
	mapFlows  = "flow-tracker/FLOWS" // LruHashMap<FlowKey, FlowState>
)

const (
	sophiaKeySize   = 2
	sophiaEntrySize = 32 // monad_common::SophiaEntry
	flowKeySize     = 16 // bpfschema.FlowKeySize
	flowStateSize   = 72 // bpfschema.FlowStateSize
)

// sophiaDictionaries groups the SOPHIA map by dictionary. The u16 key is
// (dict_id << 8) | field_value in host order (hop-ebpf sophia_key); the entry
// name is the 24-byte NUL-terminated field at offset 8.
func sophiaDictionaries(b Backend) (map[uint32]*pb.Dictionary, error) {
	dicts := map[uint32]*pb.Dictionary{}
	err := b.IterateMap(mapSophia, func(key, value []byte) error {
		if len(key) != sophiaKeySize || len(value) != sophiaEntrySize {
			return fmt.Errorf("SOPHIA entry sizes %d/%d, want %d/%d", len(key), len(value), sophiaKeySize, sophiaEntrySize)
		}
		k := binary.LittleEndian.Uint16(key)
		id := uint32(k >> 8)
		d, ok := dicts[id]
		if !ok {
			d = &pb.Dictionary{Id: id, Name: fmt.Sprintf("dict-%d", id)}
			dicts[id] = d
		}
		name, _, _ := bytes.Cut(value[8:32], []byte{0})
		entry := &pb.DictionaryEntry{Key: fmt.Sprintf("%d", k&0xFF), Value: append([]byte(nil), value...)}
		if len(name) > 0 {
			entry.Key = string(name)
		}
		d.Entries = append(d.Entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, d := range dicts {
		sort.Slice(d.Entries, func(i, j int) bool { return d.Entries[i].Key < d.Entries[j].Key })
	}
	return dicts, nil
}

// flowsFromMap reads FLOWS. The map is keyed by an IPv4 5-tuple and holds no
// flow label, so FlowLabel stays 0; created_at and last_packet_at are
// bpf_ktime_get_ns stamps, not wall clock.
func flowsFromMap(b Backend) ([]*pb.FlowState, error) {
	var flows []*pb.FlowState
	err := b.IterateMap(mapFlows, func(key, value []byte) error {
		if len(key) != flowKeySize || len(value) != flowStateSize {
			return fmt.Errorf("FLOWS entry sizes %d/%d, want %d/%d", len(key), len(value), flowKeySize, flowStateSize)
		}
		flows = append(flows, &pb.FlowState{
			SourceAddress:      append([]byte(nil), key[0:4]...),
			DestinationAddress: append([]byte(nil), key[4:8]...),
			CreatedAt:          binary.LittleEndian.Uint64(value[16:24]),
			LastPacketAt:       binary.LittleEndian.Uint64(value[24:32]),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return flows, nil
}
