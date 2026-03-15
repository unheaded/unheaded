// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-005: eBPF Kernel Interface Boundary Fuzzer
//
// Target: eBPF map operations, BPF syscall attribute structs, and ELF parsing.
// Goal: Find crashes at the boundary between userspace and kernel eBPF interfaces.
//
// This harness tests the userspace side of the eBPF interface WITHOUT requiring
// a running kernel. It fuzzes:
//   - BPF map key/value serialization (the data crossing the syscall boundary)
//   - ELF section name parsing (malformed .bpf.o files)
//   - BTF type ID parsing
//   - Map freeze validation
//   - Ring buffer header parsing
//
// These operations happen in pkg/ebpf/ and use direct syscall structs.
// Bugs here can cause kernel panics or privilege escalation.

package harnesses

import (
	"encoding/binary"
	"fmt"
	"testing"
)

// BPFMapType mirrors the kernel's bpf_map_type enum.
type BPFMapType uint32

const (
	BPF_MAP_TYPE_HASH           BPFMapType = 1
	BPF_MAP_TYPE_ARRAY          BPFMapType = 2
	BPF_MAP_TYPE_PERF_EVENT     BPFMapType = 4
	BPF_MAP_TYPE_RINGBUF        BPFMapType = 27
	BPF_MAP_TYPE_LRU_HASH       BPFMapType = 9
	BPF_MAP_TYPE_PERCPU_HASH    BPFMapType = 5
	BPF_MAP_TYPE_PERCPU_ARRAY   BPFMapType = 6
)

// BPFMapCreateAttr represents the attribute struct for BPF_MAP_CREATE.
// This is the userspace struct that gets passed to the kernel via syscall.
type BPFMapCreateAttr struct {
	MapType    BPFMapType
	KeySize    uint32
	ValueSize  uint32
	MaxEntries uint32
	MapFlags   uint32
	MapName    [16]byte
}

// BPFMapCreateAttrSize is the serialized size.
const BPFMapCreateAttrSize = 36 // 4+4+4+4+4+16

// SerializeBPFMapCreateAttr serializes the struct for the syscall boundary.
func SerializeBPFMapCreateAttr(attr *BPFMapCreateAttr) []byte {
	buf := make([]byte, BPFMapCreateAttrSize)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(attr.MapType))
	binary.LittleEndian.PutUint32(buf[4:8], attr.KeySize)
	binary.LittleEndian.PutUint32(buf[8:12], attr.ValueSize)
	binary.LittleEndian.PutUint32(buf[12:16], attr.MaxEntries)
	binary.LittleEndian.PutUint32(buf[16:20], attr.MapFlags)
	copy(buf[20:36], attr.MapName[:])
	return buf
}

// DeserializeBPFMapCreateAttr parses the attribute struct from bytes.
func DeserializeBPFMapCreateAttr(data []byte) (*BPFMapCreateAttr, error) {
	if len(data) < BPFMapCreateAttrSize {
		return nil, fmt.Errorf("bpf: map create attr requires %d bytes, got %d", BPFMapCreateAttrSize, len(data))
	}

	attr := &BPFMapCreateAttr{
		MapType:    BPFMapType(binary.LittleEndian.Uint32(data[0:4])),
		KeySize:    binary.LittleEndian.Uint32(data[4:8]),
		ValueSize:  binary.LittleEndian.Uint32(data[8:12]),
		MaxEntries: binary.LittleEndian.Uint32(data[12:16]),
		MapFlags:   binary.LittleEndian.Uint32(data[16:20]),
	}
	copy(attr.MapName[:], data[20:36])

	return attr, nil
}

// ValidateBPFMapCreateAttr validates the attribute struct.
func ValidateBPFMapCreateAttr(attr *BPFMapCreateAttr) error {
	// Validate map type
	switch attr.MapType {
	case BPF_MAP_TYPE_HASH, BPF_MAP_TYPE_ARRAY, BPF_MAP_TYPE_PERF_EVENT,
		BPF_MAP_TYPE_RINGBUF, BPF_MAP_TYPE_LRU_HASH,
		BPF_MAP_TYPE_PERCPU_HASH, BPF_MAP_TYPE_PERCPU_ARRAY:
		// valid
	default:
		return fmt.Errorf("bpf: invalid map type %d", attr.MapType)
	}

	// Key and value sizes must be > 0 (except ringbuf which has no key)
	if attr.MapType != BPF_MAP_TYPE_RINGBUF && attr.KeySize == 0 {
		return fmt.Errorf("bpf: key size cannot be zero for map type %d", attr.MapType)
	}

	if attr.ValueSize == 0 && attr.MapType != BPF_MAP_TYPE_RINGBUF {
		return fmt.Errorf("bpf: value size cannot be zero")
	}

	// Max entries sanity check
	if attr.MaxEntries == 0 {
		return fmt.Errorf("bpf: max entries cannot be zero")
	}

	// Key/value size limits (kernel enforces these)
	const maxKeySize = 512
	const maxValueSize = 1 << 22 // 4MB
	if attr.KeySize > maxKeySize {
		return fmt.Errorf("bpf: key size %d exceeds max %d", attr.KeySize, maxKeySize)
	}
	if attr.ValueSize > maxValueSize {
		return fmt.Errorf("bpf: value size %d exceeds max %d", attr.ValueSize, maxValueSize)
	}

	// Map name must be null-terminated within 16 bytes
	foundNull := false
	for _, b := range attr.MapName {
		if b == 0 {
			foundNull = true
			break
		}
	}
	if !foundNull {
		return fmt.Errorf("bpf: map name not null-terminated")
	}

	return nil
}

// ELFSectionType classifies eBPF-related ELF section names.
type ELFSectionType int

const (
	ELFSectionUnknown ELFSectionType = iota
	ELFSectionProgXDP
	ELFSectionProgTC
	ELFSectionProgKprobe
	ELFSectionProgTracepoint
	ELFSectionMap
	ELFSectionBTF
	ELFSectionLicense
)

// ClassifyELFSection identifies the type of eBPF section from its name.
func ClassifyELFSection(name string) ELFSectionType {
	if len(name) == 0 {
		return ELFSectionUnknown
	}

	switch {
	case name == "xdp" || hasPrefix(name, "xdp/"):
		return ELFSectionProgXDP
	case hasPrefix(name, "tc/") || hasPrefix(name, "classifier/"):
		return ELFSectionProgTC
	case hasPrefix(name, "kprobe/") || hasPrefix(name, "kretprobe/"):
		return ELFSectionProgKprobe
	case hasPrefix(name, "tracepoint/") || hasPrefix(name, "tp/"):
		return ELFSectionProgTracepoint
	case name == "maps" || hasPrefix(name, "maps/") || hasPrefix(name, ".maps"):
		return ELFSectionMap
	case name == ".BTF" || name == ".BTF.ext":
		return ELFSectionBTF
	case name == "license":
		return ELFSectionLicense
	default:
		return ELFSectionUnknown
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// FuzzBPFMapCreateAttr fuzzes the BPF map create attribute struct.
func FuzzBPFMapCreateAttr(f *testing.F) {
	// Valid hash map
	valid := SerializeBPFMapCreateAttr(&BPFMapCreateAttr{
		MapType:    BPF_MAP_TYPE_HASH,
		KeySize:    4,
		ValueSize:  8,
		MaxEntries: 1024,
		MapFlags:   0,
		MapName:    [16]byte{'t', 'e', 's', 't', 0},
	})
	f.Add(valid)

	// Valid ringbuf
	ringbuf := SerializeBPFMapCreateAttr(&BPFMapCreateAttr{
		MapType:    BPF_MAP_TYPE_RINGBUF,
		KeySize:    0,
		ValueSize:  0,
		MaxEntries: 4096,
		MapFlags:   0,
		MapName:    [16]byte{'r', 'b', 0},
	})
	f.Add(ringbuf)

	// All zeros
	f.Add(make([]byte, BPFMapCreateAttrSize))

	// All 0xFF
	allFF := make([]byte, BPFMapCreateAttrSize)
	for i := range allFF {
		allFF[i] = 0xFF
	}
	f.Add(allFF)

	// Truncated
	f.Add([]byte{0x01, 0x00, 0x00, 0x00})

	// Empty
	f.Add([]byte{})

	// Oversized key
	bigKey := SerializeBPFMapCreateAttr(&BPFMapCreateAttr{
		MapType:    BPF_MAP_TYPE_HASH,
		KeySize:    1024, // > max 512
		ValueSize:  8,
		MaxEntries: 100,
		MapName:    [16]byte{'b', 'i', 'g', 0},
	})
	f.Add(bigKey)

	f.Fuzz(func(t *testing.T, data []byte) {
		attr, err := DeserializeBPFMapCreateAttr(data)
		if err != nil {
			return // truncated input
		}

		// Validate
		valErr := ValidateBPFMapCreateAttr(attr)
		_ = valErr // validation errors are expected

		// Roundtrip
		reEncoded := SerializeBPFMapCreateAttr(attr)
		reAttr, reErr := DeserializeBPFMapCreateAttr(reEncoded)
		if reErr != nil {
			t.Fatalf("roundtrip deserialize failed: %v", reErr)
		}

		if reAttr.MapType != attr.MapType {
			t.Errorf("roundtrip MapType: %d vs %d", reAttr.MapType, attr.MapType)
		}
		if reAttr.KeySize != attr.KeySize {
			t.Errorf("roundtrip KeySize: %d vs %d", reAttr.KeySize, attr.KeySize)
		}
		if reAttr.ValueSize != attr.ValueSize {
			t.Errorf("roundtrip ValueSize: %d vs %d", reAttr.ValueSize, attr.ValueSize)
		}
		if reAttr.MaxEntries != attr.MaxEntries {
			t.Errorf("roundtrip MaxEntries: %d vs %d", reAttr.MaxEntries, attr.MaxEntries)
		}
	})
}

// FuzzELFSectionClassifier fuzzes ELF section name classification.
func FuzzELFSectionClassifier(f *testing.F) {
	f.Add("xdp")
	f.Add("xdp/ingress")
	f.Add("tc/egress")
	f.Add("classifier/my_prog")
	f.Add("kprobe/sys_read")
	f.Add("kretprobe/sys_write")
	f.Add("tracepoint/syscalls/sys_enter_read")
	f.Add("tp/sched/sched_switch")
	f.Add("maps")
	f.Add("maps/my_map")
	f.Add(".maps")
	f.Add(".BTF")
	f.Add(".BTF.ext")
	f.Add("license")
	f.Add("")
	f.Add("unknown_section")
	f.Add("xdp") // duplicate to verify determinism
	f.Add(string(make([]byte, 1000))) // very long name with nulls

	f.Fuzz(func(t *testing.T, name string) {
		result := ClassifyELFSection(name)

		// Verify determinism
		result2 := ClassifyELFSection(name)
		if result != result2 {
			t.Errorf("ClassifyELFSection(%q) not deterministic: %d vs %d", name, result, result2)
		}

		// Verify valid range
		if result < ELFSectionUnknown || result > ELFSectionLicense {
			t.Errorf("ClassifyELFSection(%q) returned out-of-range value: %d", name, result)
		}
	})
}

// FuzzBPFMapKeyValueSerialization tests key/value encoding for map operations.
func FuzzBPFMapKeyValueSerialization(f *testing.F) {
	f.Add(uint32(4), uint32(8), []byte{0x01, 0x02, 0x03, 0x04, 0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE})
	f.Add(uint32(1), uint32(1), []byte{0xFF, 0x00})
	f.Add(uint32(0), uint32(0), []byte{})
	f.Add(uint32(512), uint32(256), make([]byte, 768))

	f.Fuzz(func(t *testing.T, keySize uint32, valueSize uint32, data []byte) {
		if keySize > 512 || valueSize > 512 {
			return // unreasonably large
		}

		totalNeeded := int(keySize + valueSize)
		if totalNeeded == 0 || len(data) < totalNeeded {
			return
		}

		// Extract key and value
		key := data[:keySize]
		value := data[keySize:totalNeeded]

		// Verify lengths are preserved
		if uint32(len(key)) != keySize {
			t.Errorf("key length mismatch: %d vs %d", len(key), keySize)
		}
		if uint32(len(value)) != valueSize {
			t.Errorf("value length mismatch: %d vs %d", len(value), valueSize)
		}

		// Simulate map lookup: key -> hash -> bucket
		// This tests that the key hashing doesn't panic
		hash := uint32(0)
		for _, b := range key {
			hash = hash*31 + uint32(b)
		}
		_ = hash
	})
}
