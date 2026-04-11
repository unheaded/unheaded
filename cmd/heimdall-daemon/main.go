// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

// Heimdall Daemon — userspace control for Mímir's Law / Gleipnir Phase 0 PoC.
//
// Responsibilities:
//  1. Load Mjölnir baseline manifest + verify its GungnirSeal
//  2. Read BPF ringbuf events from heimdall-bpf kprobes (vfs_write, execve, mmap)
//  3. Byte-level diff against Mjölnir manifest → DriftEvent
//  4. Publish DriftEvents to Wotan (drift.detected.<node_id>) ML-DSA-65 signed
//  5. Listen for Gjallarhorn UPC trigger packets (BootstrapBroadcast / ReverifyUnicast)
//
// Per ADR-043 hard condition #1: NO RESTORE. Alerts only.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"unheaded/pkg/enkrateia"
	"unheaded/pkg/gungnir"
)

// MjolnirManifest is the declarative baseline from references/baseline/mjolnir.yaml.
type MjolnirManifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name     string `yaml:"name"`
		Version  string `yaml:"version"`
		Created  string `yaml:"created"`
		SignedBy string `yaml:"signed_by"`
	} `yaml:"metadata"`
	Spec struct {
		BaseImage struct {
			Distro  string `yaml:"distro"`
			Release string `yaml:"release"`
			Digest  string `yaml:"digest"`
		} `yaml:"base_image"`
		Files []struct {
			Path   string `yaml:"path"`
			SHA256 string `yaml:"sha256"`
			Mode   string `yaml:"mode"`
			Owner  string `yaml:"owner"`
		} `yaml:"files"`
		Packages []struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
		} `yaml:"packages"`
	} `yaml:"spec"`
}

// Heimdall is the daemon state.
type Heimdall struct {
	nodeID     string
	manifest   *MjolnirManifest
	aggregator *enkrateia.Aggregator
	mu         sync.RWMutex
}

// LoadManifest parses a Mjölnir YAML manifest from disk.
// TODO: verify attached GungnirSeal before trusting the manifest.
func LoadManifest(path string) (*MjolnirManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m MjolnirManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Kind != "BaselineManifest" {
		return nil, fmt.Errorf("unexpected kind: %s", m.Kind)
	}
	return &m, nil
}

// ScanFiles walks the manifest file list and reports drift.
// Byte-level diff (v1) — semantic diff is deferred to v2 (BlackMage #5).
func (h *Heimdall) ScanFiles() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.manifest == nil {
		return
	}

	for _, f := range h.manifest.Spec.Files {
		actual, err := hashFile(f.Path)
		if err != nil {
			// File missing → drift
			h.aggregator.HandleDrift(enkrateia.DriftEvent{
				NodeID:     h.nodeID,
				Path:       f.Path,
				Severity:   "alert",
				DetectedAt: time.Now(),
			})
			continue
		}
		if actual != f.SHA256 {
			h.aggregator.HandleDrift(enkrateia.DriftEvent{
				NodeID:       h.nodeID,
				Path:         f.Path,
				HashActual:   []byte(actual),
				HashExpected: []byte(f.SHA256),
				Severity:     "alert",
				DetectedAt:   time.Now(),
			})
		}
	}
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Run starts the daemon loop.
func (h *Heimdall) Run(ctx context.Context, scanInterval time.Duration) error {
	// Alert drain goroutine — publishes to Wotan drift.detected.<node_id>
	// TODO: wire to actual Wotan client with ML-DSA-65 signing
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case alert := <-h.aggregator.Alerts():
				fmt.Printf("[ALERT] %s\n", alert.Message)
			}
		}
	}()

	// TODO: wire BPF ringbuf reader (Aya userspace) for event-driven scanning
	// TODO: wire Gjallarhorn XDP listener for UPC trigger packets

	// Periodic baseline scan (v1 — polling)
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			h.ScanFiles()
		}
	}
}

func main() {
	var (
		manifestPath = flag.String("manifest", "references/baseline/mjolnir.example.yaml", "Path to Mjölnir manifest")
		nodeID       = flag.String("node-id", "", "Node identifier (defaults to hostname)")
		scanInterval = flag.Duration("scan-interval", 30*time.Second, "Baseline scan interval")
	)
	flag.Parse()

	if *nodeID == "" {
		hostname, _ := os.Hostname()
		*nodeID = hostname
	}

	fmt.Printf("Heimdall Daemon starting (node=%s)\n", *nodeID)
	fmt.Printf("ADR-043 hard condition #1: alerts only, no auto-restore\n")

	manifest, err := LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded Mjölnir: %s v%s (%d files, %d packages)\n",
		manifest.Metadata.Name, manifest.Metadata.Version,
		len(manifest.Spec.Files), len(manifest.Spec.Packages))

	// Placeholder key for dev
	_ = gungnir.GenerateDevKey

	h := &Heimdall{
		nodeID:     *nodeID,
		manifest:   manifest,
		aggregator: enkrateia.NewAggregator(1024),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutdown signal received")
		cancel()
	}()

	if err := h.Run(ctx, *scanInterval); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "run: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Heimdall Daemon stopped")
}
