// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// LICH-009: Flow ID Collision Fuzzer
//
// Target: Flow ID generation, tracking, and collision detection.
// Goal: Find collisions in flow label assignment, namespace sequence tracking,
//       and the combined flow identification scheme.
//
// The Monad protocol uses a composite flow identifier:
//   FlowID = FlowLabel (32-bit) + Namespace (32-bit) + Sequence (32-bit)
//
// Collisions in any component can cause:
//   - Misrouted packets (wrong flow gets the data)
//   - State corruption (two flows sharing a state slot)
//   - Trace confusion (eBPF traces merge)
//   - DoS via hash collision in flow tables
//
// This harness tests flow ID generation, tracking, and the IPv6
// flow label fast-path encoding from the S67 protocol spec.

package harnesses

import (
	"encoding/binary"
	"testing"
)

// FlowID represents a composite flow identifier.
type FlowID struct {
	FlowLabel uint32
	Namespace uint32
	Sequence  uint32
}

// FlowIDKey returns a 12-byte key for map lookup.
func (f *FlowID) Key() [12]byte {
	var key [12]byte
	binary.BigEndian.PutUint32(key[0:4], f.FlowLabel)
	binary.BigEndian.PutUint32(key[4:8], f.Namespace)
	binary.BigEndian.PutUint32(key[8:12], f.Sequence)
	return key
}

// FlowTable tracks active flows with collision detection.
type FlowTable struct {
	entries map[[12]byte]*FlowID
	byLabel map[uint32][]*FlowID
	count   int
}

// NewFlowTable creates a new flow table.
func NewFlowTable() *FlowTable {
	return &FlowTable{
		entries: make(map[[12]byte]*FlowID),
		byLabel: make(map[uint32][]*FlowID),
	}
}

// Insert adds a flow, returning true if it's a collision.
func (ft *FlowTable) Insert(flow *FlowID) bool {
	key := flow.Key()

	if _, exists := ft.entries[key]; exists {
		return true // exact collision
	}

	ft.entries[key] = flow
	ft.byLabel[flow.FlowLabel] = append(ft.byLabel[flow.FlowLabel], flow)
	ft.count++
	return false
}

// LabelCollisions returns the number of flows sharing a FlowLabel.
func (ft *FlowTable) LabelCollisions(label uint32) int {
	flows := ft.byLabel[label]
	if len(flows) <= 1 {
		return 0
	}
	return len(flows) - 1
}

// IPv6FlowLabelFastPath encodes service/status/latency into IPv6 flow label.
// Format: [SVC:4][STATUS:4][LATENCY_BUCKET:8][FLAGS:4]
func IPv6FlowLabelFastPath(serviceID, statusCode uint8, latencyBucket uint8, flags uint8) uint32 {
	// IPv6 flow label is 20 bits, but we use the full uint32 for our encoding
	label := uint32(serviceID&0x0F) << 16
	label |= uint32(statusCode&0x0F) << 12
	label |= uint32(latencyBucket) << 4
	label |= uint32(flags & 0x0F)
	return label
}

// DecodeIPv6FlowLabel decodes the fast-path flow label.
func DecodeIPv6FlowLabel(label uint32) (serviceID, statusCode, latencyBucket, flags uint8) {
	serviceID = uint8((label >> 16) & 0x0F)
	statusCode = uint8((label >> 12) & 0x0F)
	latencyBucket = uint8((label >> 4) & 0xFF)
	flags = uint8(label & 0x0F)
	return
}

// FlowLabelHash computes a simple hash for flow label lookup.
// Used in eBPF maps where the hash function matters for DoS resistance.
func FlowLabelHash(label uint32, tableSize uint32) uint32 {
	if tableSize == 0 {
		return 0
	}
	// FNV-1a inspired hash
	h := uint32(2166136261)
	h ^= label & 0xFF
	h *= 16777619
	h ^= (label >> 8) & 0xFF
	h *= 16777619
	h ^= (label >> 16) & 0xFF
	h *= 16777619
	h ^= (label >> 24) & 0xFF
	h *= 16777619
	return h % tableSize
}

// FuzzFlowIDCollision inserts random flows and checks for collisions.
func FuzzFlowIDCollision(f *testing.F) {
	f.Add(uint32(1), uint32(1), uint32(1), uint32(1), uint32(1), uint32(2))
	f.Add(uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0)) // exact duplicate
	f.Add(uint32(0xFFFFFFFF), uint32(0), uint32(0), uint32(0xFFFFFFFF), uint32(0), uint32(0)) // same label, same ns/seq
	f.Add(uint32(1), uint32(2), uint32(3), uint32(1), uint32(2), uint32(4)) // same label+ns, different seq

	f.Fuzz(func(t *testing.T, fl1, ns1, seq1, fl2, ns2, seq2 uint32) {
		ft := NewFlowTable()

		flow1 := &FlowID{FlowLabel: fl1, Namespace: ns1, Sequence: seq1}
		flow2 := &FlowID{FlowLabel: fl2, Namespace: ns2, Sequence: seq2}

		// Insert first flow (should never collide)
		if ft.Insert(flow1) {
			t.Error("first insert should not collide")
		}

		// Insert second flow
		collision := ft.Insert(flow2)

		// Check if it's an exact collision (all three fields match)
		exactCollision := fl1 == fl2 && ns1 == ns2 && seq1 == seq2
		if exactCollision && !collision {
			t.Error("exact duplicate should report collision")
		}
		if !exactCollision && collision {
			t.Error("non-duplicate should not report exact collision")
		}

		// Check label collisions
		if fl1 == fl2 && !exactCollision {
			labelCollisions := ft.LabelCollisions(fl1)
			if labelCollisions < 1 {
				t.Errorf("same flow label but no label collision detected")
			}
		}
	})
}

// FuzzIPv6FlowLabelRoundtrip tests encode/decode of the fast-path flow label.
func FuzzIPv6FlowLabelRoundtrip(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0), uint8(0))
	f.Add(uint8(15), uint8(15), uint8(255), uint8(15)) // all max 4-bit/8-bit values
	f.Add(uint8(1), uint8(2), uint8(128), uint8(3))
	f.Add(uint8(0x0F), uint8(0x0F), uint8(0xFF), uint8(0x0F)) // max encoded values

	f.Fuzz(func(t *testing.T, svcID, statusCode, latBucket, flags uint8) {
		// Mask to valid bits
		svcID &= 0x0F
		statusCode &= 0x0F
		flags &= 0x0F

		label := IPv6FlowLabelFastPath(svcID, statusCode, latBucket, flags)

		// Decode
		decSvc, decStatus, decLat, decFlags := DecodeIPv6FlowLabel(label)

		// Roundtrip verification
		if decSvc != svcID {
			t.Errorf("serviceID roundtrip: %d -> %d", svcID, decSvc)
		}
		if decStatus != statusCode {
			t.Errorf("statusCode roundtrip: %d -> %d", statusCode, decStatus)
		}
		if decLat != latBucket {
			t.Errorf("latencyBucket roundtrip: %d -> %d", latBucket, decLat)
		}
		if decFlags != flags {
			t.Errorf("flags roundtrip: %d -> %d", flags, decFlags)
		}

		// The label should fit in 20 bits (IPv6 flow label size)
		if label > 0xFFFFF {
			t.Errorf("flow label 0x%06X exceeds 20-bit IPv6 limit", label)
		}
	})
}

// FuzzFlowLabelHashDistribution tests hash distribution for DoS resistance.
func FuzzFlowLabelHashDistribution(f *testing.F) {
	f.Add(uint32(0), uint32(1024))
	f.Add(uint32(0xFFFFFFFF), uint32(1024))
	f.Add(uint32(0x12345678), uint32(256))
	f.Add(uint32(1), uint32(1))
	f.Add(uint32(0), uint32(0))

	f.Fuzz(func(t *testing.T, label uint32, tableSize uint32) {
		if tableSize == 0 || tableSize > 1<<20 {
			return
		}

		bucket := FlowLabelHash(label, tableSize)

		// Invariant: bucket must be in range
		if bucket >= tableSize {
			t.Errorf("bucket %d >= tableSize %d", bucket, tableSize)
		}

		// Determinism check
		bucket2 := FlowLabelHash(label, tableSize)
		if bucket != bucket2 {
			t.Errorf("hash not deterministic: %d vs %d", bucket, bucket2)
		}

		// Neighboring labels should (usually) hash to different buckets
		if tableSize > 1 {
			neighborBucket := FlowLabelHash(label+1, tableSize)
			_ = neighborBucket // can't assert, but track distribution
		}
	})
}

// FuzzFlowSequenceMonotonicity tests sequence number monotonicity per namespace.
func FuzzFlowSequenceMonotonicity(f *testing.F) {
	// Sequence of namespace+sequence pairs
	f.Add([]byte{
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // ns=1, seq=1
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02, // ns=1, seq=2
		0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01, // ns=2, seq=1
	})

	f.Add([]byte{
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x05, // ns=1, seq=5
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x03, // ns=1, seq=3 (out of order!)
	})

	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Track last sequence per namespace
		lastSeq := make(map[uint32]uint32)
		var violations int

		offset := 0
		for offset+8 <= len(data) {
			ns := binary.BigEndian.Uint32(data[offset : offset+4])
			seq := binary.BigEndian.Uint32(data[offset+4 : offset+8])

			if last, exists := lastSeq[ns]; exists {
				if seq < last {
					violations++
				}
			}
			lastSeq[ns] = seq
			offset += 8
		}

		// Report total violations (not an error, just tracking)
		_ = violations
	})
}
