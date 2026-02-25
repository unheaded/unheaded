// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package ebpf provides eBPF program loading and management.
// This test file runs on all platforms but tests the stub implementation on non-Linux.
package ebpf

import (
	"testing"
	"time"
)

func TestProgramSpec(t *testing.T) {
	spec := &ProgramSpec{
		Name:       "test_program",
		Path:       "/opt/unheaded/ebpf/test.bpf.o",
		Type:       TypeXDP,
		AttachTo:   "eth0",
		AttachType: AttachXDPDriver,
		PinPath:    "/sys/fs/bpf/test",
		MapPinPaths: map[string]string{
			"my_map": "/sys/fs/bpf/my_map",
		},
		Constants: map[string]uint64{
			"const1": 42,
		},
		Labels: map[string]string{
			"env": "test",
		},
	}

	if spec.Name != "test_program" {
		t.Errorf("expected name 'test_program', got '%s'", spec.Name)
	}
	if spec.Type != TypeXDP {
		t.Errorf("expected type XDP, got '%s'", spec.Type)
	}
	if spec.AttachTo != "eth0" {
		t.Errorf("expected attach_to 'eth0', got '%s'", spec.AttachTo)
	}
}

func TestProgramTypes(t *testing.T) {
	types := []ProgramType{
		TypeKProbe,
		TypeKRetProbe,
		TypeTracepoint,
		TypeXDP,
		TypeTC,
		TypeCgroupSkb,
		TypeCgroupSock,
		TypeSocketFilter,
		TypeSched,
		TypePerf,
		TypeRawTracepoint,
		TypeFentry,
		TypeFexit,
		TypeLSM,
		TypeIterator,
	}

	for _, pt := range types {
		if pt == "" {
			t.Error("program type should not be empty")
		}
	}
}

func TestAttachTypes(t *testing.T) {
	attachTypes := []AttachType{
		AttachNone,
		AttachCgroupInet4Bind,
		AttachCgroupInet6Bind,
		AttachCgroupSockOps,
		AttachXDPDriver,
		AttachXDPGeneric,
		AttachXDPOffload,
		AttachTCIngress,
		AttachTCEgress,
	}

	for _, at := range attachTypes {
		if at == "" && at != AttachNone {
			t.Error("attach type should not be empty (except AttachNone)")
		}
	}
}

func TestProgramStatus(t *testing.T) {
	statuses := []ProgramStatus{
		StatusUnloaded,
		StatusLoading,
		StatusLoaded,
		StatusAttaching,
		StatusAttached,
		StatusDetaching,
		StatusError,
	}

	for _, s := range statuses {
		if s == "" {
			t.Error("program status should not be empty")
		}
	}
}

func TestMapTypes(t *testing.T) {
	mapTypes := []MapType{
		MapTypeHash,
		MapTypeArray,
		MapTypeProgArray,
		MapTypePerfEventArray,
		MapTypePercpuHash,
		MapTypePercpuArray,
		MapTypeLruHash,
		MapTypeLruPercpuHash,
		MapTypeLpmTrie,
		MapTypeArrayOfMaps,
		MapTypeHashOfMaps,
		MapTypeRingbuf,
		MapTypeBloom,
		MapTypeStack,
		MapTypeQueue,
	}

	for _, mt := range mapTypes {
		if mt == "" {
			t.Error("map type should not be empty")
		}
	}
}

func TestProgramInfo(t *testing.T) {
	info := &ProgramInfo{
		Name:       "test",
		ID:         123,
		Type:       TypeXDP,
		Tag:        "abcd1234",
		JitedSize:  1024,
		XlatedSize: 512,
		LoadTime:   time.Now(),
		Status:     StatusLoaded,
		AttachTo:   "eth0",
		AttachType: AttachXDPDriver,
		Maps:       []string{"map1", "map2"},
		Labels: map[string]string{
			"app": "test",
		},
	}

	if info.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", info.Name)
	}
	if info.ID != 123 {
		t.Errorf("expected ID 123, got %d", info.ID)
	}
	if len(info.Maps) != 2 {
		t.Errorf("expected 2 maps, got %d", len(info.Maps))
	}
}

func TestMapInfo(t *testing.T) {
	mapInfo := &MapInfo{
		Name:       "test_map",
		ID:         456,
		Type:       MapTypeHash,
		KeySize:    4,
		ValueSize:  8,
		MaxEntries: 1024,
		Flags:      0,
		PinPath:    "/sys/fs/bpf/test_map",
	}

	if mapInfo.Name != "test_map" {
		t.Errorf("expected name 'test_map', got '%s'", mapInfo.Name)
	}
	if mapInfo.KeySize != 4 {
		t.Errorf("expected key size 4, got %d", mapInfo.KeySize)
	}
	if mapInfo.ValueSize != 8 {
		t.Errorf("expected value size 8, got %d", mapInfo.ValueSize)
	}
}

func TestProgramMetrics(t *testing.T) {
	metrics := &ProgramMetrics{
		Name:       "test",
		RunCount:   1000,
		RunTimeNs:  5000000,
		AvgRunNs:   5000,
		Recursion:  0,
		SampleTime: time.Now(),
	}

	if metrics.RunCount != 1000 {
		t.Errorf("expected run count 1000, got %d", metrics.RunCount)
	}
	if metrics.AvgRunNs != 5000 {
		t.Errorf("expected avg run 5000ns, got %d", metrics.AvgRunNs)
	}
}

func TestKernelVersion(t *testing.T) {
	kv := KernelVersion{
		Major: 5,
		Minor: 10,
		Patch: 0,
	}

	// On non-Linux, String returns "0.0.0"
	str := kv.String()
	if str == "" {
		t.Error("kernel version string should not be empty")
	}

	// On non-Linux, AtLeast always returns false
	result := kv.AtLeast(5, 10, 0)
	// This is platform dependent, so we just check it doesn't panic
	_ = result
}

func TestKernelFeatures(t *testing.T) {
	features := KernelFeatures{
		Version: KernelVersion{
			Major: 5,
			Minor: 15,
			Patch: 0,
		},
		BTFEnabled:      true,
		BTFPath:         "/sys/kernel/btf/vmlinux",
		RingbufSupport:  true,
		BPFLinkSupport:  true,
		KprobeMulti:     true,
		FentrySupport:   true,
		TCXSupport:      false,
		BPFTokenSupport: false,
		MemcgAccounting: true,
		BPFArena:        false,
		SignalSupport:   true,
	}

	if !features.BTFEnabled {
		t.Error("expected BTF enabled")
	}
	if !features.RingbufSupport {
		t.Error("expected ringbuf support")
	}
}

func TestDefaultLoaderConfig(t *testing.T) {
	cfg := DefaultLoaderConfig()

	if cfg.PinPath != "/sys/fs/bpf" {
		t.Errorf("expected pin path '/sys/fs/bpf', got '%s'", cfg.PinPath)
	}
	if cfg.VerifierLogSize != 64*1024 {
		t.Errorf("expected verifier log size %d, got %d", 64*1024, cfg.VerifierLogSize)
	}
	if !cfg.AllowRlimit {
		t.Error("expected AllowRlimit to be true")
	}
	if !cfg.MetricsEnabled {
		t.Error("expected MetricsEnabled to be true")
	}
}

func TestDefaultPrograms(t *testing.T) {
	programs := DefaultPrograms()

	if len(programs) == 0 {
		t.Error("expected at least one default program")
	}

	for _, p := range programs {
		if p.Name == "" {
			t.Error("program name should not be empty")
		}
		if p.Path == "" {
			t.Error("program path should not be empty")
		}
	}
}

func TestNewLoaderReturnsError(t *testing.T) {
	// On non-Linux platforms, this should return an error
	cfg := DefaultLoaderConfig()
	loader, err := NewLoader(cfg)

	// The behavior is platform-dependent
	// On non-Linux: returns nil, ErrNotSupported
	// On Linux: might return a real loader or an error depending on permissions
	if loader == nil && err == nil {
		t.Error("expected either a loader or an error, got neither")
	}
}

func TestErrors(t *testing.T) {
	errors := []error{
		ErrProgramNotFound,
		ErrProgramExists,
		ErrLoadFailed,
		ErrAttachFailed,
		ErrDetachFailed,
		ErrMapNotFound,
		ErrMapAccessDenied,
		ErrKernelNotSupported,
		ErrVerifierFailed,
		ErrInvalidProgram,
		ErrLoaderClosed,
		ErrBTFNotAvailable,
		ErrInterfaceNotFound,
		ErrPinPathExists,
		ErrUnsupportedMapType,
		ErrELFParseFailed,
		ErrNoInstructions,
		ErrSyscallFailed,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("error should not be nil")
		}
		if err.Error() == "" {
			t.Error("error message should not be empty")
		}
	}
}
