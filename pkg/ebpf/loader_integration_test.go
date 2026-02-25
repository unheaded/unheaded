// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux && integration

package ebpf

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestParseCompiledBPFPrograms verifies the Go ELF parser can read
// the Aya-compiled BPF programs (which use legacy 'maps' section).
func TestParseCompiledBPFPrograms(t *testing.T) {
	if runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64" {
		t.Skip("BPF programs compiled for specific arch")
	}

	ebpfDir := findEBPFDir(t)
	if ebpfDir == "" {
		t.Skip("eBPF build directory not found")
	}

	programs := []struct {
		name     string
		sections []string // expected sections
	}{
		{"packet-marker", []string{"xdp"}},
		{"flow-tracker", []string{"classifier"}},
		{"latency-probe", []string{"kprobe", "kretprobe"}},
	}

	for _, prog := range programs {
		t.Run(prog.name, func(t *testing.T) {
			path := filepath.Join(ebpfDir, prog.name)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("BPF binary not found: %s", path)
			}

			parsed, err := parseELF(path)
			if err != nil {
				t.Fatalf("failed to parse ELF %s: %v", prog.name, err)
			}

			if len(parsed.Programs) == 0 {
				t.Errorf("no programs found in %s", prog.name)
			}

			for _, sec := range prog.sections {
				if _, ok := parsed.Programs[sec]; !ok {
					t.Errorf("expected section %q not found in %s (found: %v)",
						sec, prog.name, programNames(parsed))
				}
			}

			if len(parsed.Maps) == 0 {
				t.Errorf("no maps found in %s", prog.name)
			}

			t.Logf("%s: %d programs, %d maps", prog.name, len(parsed.Programs), len(parsed.Maps))
		})
	}
}

func findEBPFDir(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), "tmp/unheaded/ebpf/target/bpfel-unknown-none/release"),
	}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return ""
}

func programNames(p *parsedELF) []string {
	names := make([]string, 0, len(p.Programs))
	for name := range p.Programs {
		names = append(names, name)
	}
	return names
}
