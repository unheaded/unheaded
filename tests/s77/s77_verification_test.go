// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package s77 verifies all S77 battle plan deliverables exist.
// Run with: go test ./tests/s77/ -v
package s77

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// projectRoot returns the project root by finding CLAUDE.md.
func projectRoot(t *testing.T) string {
	t.Helper()
	// Start from the test file location and walk up
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	dir := filepath.Dir(filename)
	// Walk up from tests/s77/ to project root
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("cannot find project root (CLAUDE.md) from %s", filename)
	return ""
}

func fileExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		t.Errorf("MISSING: %s", path)
		return
	}
	if err != nil {
		t.Errorf("ERROR checking %s: %v", path, err)
		return
	}
	if info.Size() == 0 {
		t.Errorf("EMPTY: %s (0 bytes)", path)
	}
}

func fileContains(t *testing.T, path string, substrings ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("cannot read %s: %v", path, err)
		return
	}
	content := string(data)
	for _, sub := range substrings {
		if !strings.Contains(content, sub) {
			t.Errorf("%s missing expected content: %q", filepath.Base(path), sub)
		}
	}
}

// --- Phase 1: Triage & Hardening ---

func TestPhase1_BugFixes(t *testing.T) {
	root := projectRoot(t)

	t.Run("Bug17_XFF_Spoofing", func(t *testing.T) {
		fileExists(t, filepath.Join(root, "pkg/waf/rules/engine.go"))
		fileContains(t, filepath.Join(root, "pkg/waf/rules/engine.go"), "extractSecureClientIP")
		fileExists(t, filepath.Join(root, "pkg/waf/rules/clientip_test.go"))
		fileContains(t, filepath.Join(root, "pkg/waf/rules/clientip_test.go"), "TestExtractSecureClientIP")
		fileExists(t, filepath.Join(root, "pkg/waf/detection/bot_test.go"))
	})

	t.Run("Bug19_Wotan_Degradation", func(t *testing.T) {
		fileContains(t, filepath.Join(root, "pkg/wotan-client/client.go"), "degraded")
	})

	t.Run("Bug25_DoubleCheckLocking", func(t *testing.T) {
		fileContains(t, filepath.Join(root, "pkg/wotan-client/client.go"), "RLock")
	})

	t.Run("Documentation", func(t *testing.T) {
		fileExists(t, filepath.Join(root, "docs/P1-BUG-FIXES-S77.md"))
	})
}

// --- Phase 2: WireGuard IPv6 + Performance ---

func TestPhase2_WireGuard(t *testing.T) {
	root := projectRoot(t)

	t.Run("SetupScript", func(t *testing.T) {
		path := filepath.Join(root, "scripts/wireguard/setup-wireguard.sh")
		fileExists(t, path)
		fileContains(t, path, "#!/bin/bash", "wg", "fd00:dead:beef")
	})

	t.Run("VerifyScript", func(t *testing.T) {
		path := filepath.Join(root, "scripts/wireguard/verify-wireguard.sh")
		fileExists(t, path)
		fileContains(t, path, "#!/bin/bash", "ping")
	})

	t.Run("DesignDoc", func(t *testing.T) {
		path := filepath.Join(root, "docs/WIREGUARD-DESIGN-S77.md")
		fileExists(t, path)
		fileContains(t, path, "fd00:dead:beef", "WireGuard", "ChaCha20")
	})
}

func TestPhase2_Benchmark(t *testing.T) {
	root := projectRoot(t)

	t.Run("Framework", func(t *testing.T) {
		path := filepath.Join(root, "pkg/benchmark/benchmark.go")
		fileExists(t, path)
		fileContains(t, path, "package benchmark", "Suite", "Run")
	})

	t.Run("Tests", func(t *testing.T) {
		fileExists(t, filepath.Join(root, "pkg/benchmark/benchmark_test.go"))
	})

	t.Run("PerfHarness", func(t *testing.T) {
		path := filepath.Join(root, "cmd/perf-benchmark/main.go")
		fileExists(t, path)
		fileContains(t, path, "package main", "p2pDirectLatency", "wireguardIPv6Latency")
	})

	t.Run("PerformanceDoc", func(t *testing.T) {
		path := filepath.Join(root, "docs/PERFORMANCE-S77.md")
		fileExists(t, path)
		fileContains(t, path, "P2P", "WireGuard", "latency")
	})
}

// --- Phase 3: SBOM + CI/CD Fortress ---

func TestPhase3_CICD(t *testing.T) {
	root := projectRoot(t)

	t.Run("GPLBoundaryScript", func(t *testing.T) {
		path := filepath.Join(root, "scripts/verify-gpl-boundary.sh")
		fileExists(t, path)
		fileContains(t, path, "#!/bin/bash", "GPL")
	})

	t.Run("GPLBoundaryWorkflow", func(t *testing.T) {
		path := filepath.Join(root, ".github/workflows/gpl-boundary.yml")
		fileExists(t, path)
		fileContains(t, path, "GPL Boundary")
	})

	t.Run("CILocalScript", func(t *testing.T) {
		path := filepath.Join(root, "scripts/ci-local.sh")
		fileExists(t, path)
		fileContains(t, path, "#!/bin/bash", "go build", "go test")
	})

	t.Run("CICDStrategy", func(t *testing.T) {
		fileExists(t, filepath.Join(root, "docs/CI-CD-STRATEGY-S77.md"))
	})

	t.Run("MakefileTargets", func(t *testing.T) {
		path := filepath.Join(root, "Makefile")
		fileContains(t, path, "ci-local:", "ci-security:", "sbom-verify:")
	})

	t.Run("JenkinsfileLicenseCheck", func(t *testing.T) {
		path := filepath.Join(root, "Jenkinsfile")
		fileContains(t, path, "License Check", "verify-gpl-boundary")
	})
}

// --- Phase 4: Protocol Spec Advancement ---

func TestPhase4_ProtocolSpecs(t *testing.T) {
	root := projectRoot(t)
	protoDir := filepath.Join(root, "docs/protocol")

	t.Run("Foundation_Draft06", func(t *testing.T) {
		path := filepath.Join(protoDir, "draft-bellis-unheaded-protocol-foundation-06.md")
		fileExists(t, path)
		fileContains(t, path,
			"draft-bellis-unheaded-protocol-foundation-06",
			"IANA", "UNHEADED_METRIC_V1", "FROZEN")
	})

	t.Run("Sophia_Draft03", func(t *testing.T) {
		path := filepath.Join(protoDir, "draft-bellis-unheaded-sophia-dictionary-03.md")
		fileExists(t, path)
		fileContains(t, path,
			"draft-bellis-unheaded-sophia-dictionary-03",
			"Sub-Dictionary", "QPACK", "LEAF", "BRANCH", "COMPOSITE")
	})

	t.Run("Wotan_Draft03", func(t *testing.T) {
		path := filepath.Join(protoDir, "draft-bellis-unheaded-wotan-memory-03.md")
		fileExists(t, path)
		fileContains(t, path,
			"draft-bellis-unheaded-wotan-memory-03",
			"Error Code", "Severity", "HEALTHY", "DEGRADED")
	})

	t.Run("CrossReferenceMatrix", func(t *testing.T) {
		path := filepath.Join(protoDir, "CROSS-REFERENCE-MATRIX-S77.md")
		fileExists(t, path)
		fileContains(t, path, "Foundation", "Sophia", "Wotan", "draft-06", "draft-03")
	})

	t.Run("Changelog", func(t *testing.T) {
		path := filepath.Join(protoDir, "CHANGELOG-S77.md")
		fileExists(t, path)
		fileContains(t, path, "Foundation", "Sophia", "Wotan", "FROZEN")
	})
}

// --- Phase 5: Interface Contracts ---

func TestPhase5_IaCRenderer(t *testing.T) {
	root := projectRoot(t)

	t.Run("Interface", func(t *testing.T) {
		path := filepath.Join(root, "pkg/iac/iac.go")
		fileExists(t, path)
		fileContains(t, path,
			"Renderer interface",
			"Render(config ServiceConfig)",
			"RenderAll(configs []ServiceConfig)",
			"Validate(config ServiceConfig) error",
			"Diff(current, desired ServiceConfig)")
	})

	t.Run("AllBackends", func(t *testing.T) {
		backends := []string{"ansible.go", "terraform.go", "puppet.go",
			"kubernetes.go", "chef.go", "salt.go"}
		for _, b := range backends {
			fileExists(t, filepath.Join(root, "pkg/iac", b))
		}
	})

	t.Run("Validation", func(t *testing.T) {
		path := filepath.Join(root, "pkg/iac/validate.go")
		fileExists(t, path)
		fileContains(t, path, "ValidateRenderedOutput", "DiffConfigs")
	})

	t.Run("IntegrationTests", func(t *testing.T) {
		fileExists(t, filepath.Join(root, "pkg/iac/integration_test.go"))
	})
}

func TestPhase5_ObservabilityAdapter(t *testing.T) {
	root := projectRoot(t)

	t.Run("Interface", func(t *testing.T) {
		path := filepath.Join(root, "pkg/observability/observability.go")
		fileExists(t, path)
		fileContains(t, path,
			"Adapter interface", "EmitMetric", "EmitLog",
			"EmitTrace", "EmitAlert", "Pipeline")
	})

	t.Run("AllAdapters", func(t *testing.T) {
		adapters := []string{"prometheus.go", "elk.go", "grafana.go",
			"fluentd.go", "jaeger.go", "nagios.go", "loki.go", "zerolog.go"}
		for _, a := range adapters {
			fileExists(t, filepath.Join(root, "pkg/observability", a))
		}
	})

	t.Run("IntegrationTests", func(t *testing.T) {
		fileExists(t, filepath.Join(root, "pkg/observability/integration_test.go"))
	})
}

func TestPhase5_Documentation(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "docs/INTERFACE-CONTRACTS-S77.md")
	fileExists(t, path)
	fileContains(t, path,
		"IaC Renderer", "Observability Adapter",
		"Ansible", "Terraform", "Kubernetes",
		"Prometheus", "ELK", "Jaeger")
}

// --- Summary: Total Deliverable Count ---

func TestS77_AllDeliverables(t *testing.T) {
	root := projectRoot(t)

	deliverables := []string{
		// Phase 1
		"pkg/waf/rules/clientip_test.go",
		"docs/P1-BUG-FIXES-S77.md",
		// Phase 2
		"scripts/wireguard/setup-wireguard.sh",
		"scripts/wireguard/verify-wireguard.sh",
		"pkg/benchmark/benchmark.go",
		"pkg/benchmark/benchmark_test.go",
		"cmd/perf-benchmark/main.go",
		"docs/WIREGUARD-DESIGN-S77.md",
		"docs/PERFORMANCE-S77.md",
		// Phase 3
		"scripts/verify-gpl-boundary.sh",
		"scripts/ci-local.sh",
		".github/workflows/gpl-boundary.yml",
		"docs/CI-CD-STRATEGY-S77.md",
		// Phase 4
		"docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md",
		"docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md",
		"docs/protocol/draft-bellis-unheaded-wotan-memory-03.md",
		"docs/protocol/CROSS-REFERENCE-MATRIX-S77.md",
		"docs/protocol/CHANGELOG-S77.md",
		// Phase 5
		"docs/INTERFACE-CONTRACTS-S77.md",
		"pkg/iac/validate.go",
		"pkg/iac/integration_test.go",
		"pkg/observability/integration_test.go",
	}

	missing := 0
	for _, d := range deliverables {
		path := filepath.Join(root, d)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("MISSING: %s", d)
			missing++
		}
	}

	total := len(deliverables)
	present := total - missing
	t.Logf("S77 Battle Plan: %d/%d deliverables present (%d missing)", present, total, missing)

	if missing > 0 {
		t.Fatalf("%d deliverables missing — battle plan incomplete", missing)
	}
}
