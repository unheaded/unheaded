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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
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
	nodeID       string
	manifestPath string
	trust        Trust
	manifest     *MjolnirManifest
	aggregator   *enkrateia.Aggregator
	mu           sync.RWMutex
}

// Trust says how a manifest is checked before Heimdall believes it.
type Trust struct {
	// PubKey verifies the manifest's detached seal (<manifest>.seal, a
	// JSON gungnir.GungnirSeal over the manifest's exact bytes).
	PubKey *mldsa65.PublicKey
	// AllowUnsigned loads a manifest with no key to check it against: a
	// lab run only. Without a key and without this, loading fails.
	AllowUnsigned bool
}

// ErrUnverified refuses a manifest nothing vouches for.
var ErrUnverified = errors.New("no public key to verify the manifest's seal (--pubkey), and --allow-unsigned not given")

// SealPath is where a manifest's detached seal lives.
func SealPath(manifest string) string { return manifest + ".seal" }

// LoadManifest reads a Mjölnir manifest and, with trust.PubKey, verifies its
// ML-DSA-65 seal over the exact bytes before parsing them (ADR-043: the
// baseline is the thing an attacker most wants to swap; drift detection
// against a forged baseline reports nothing).
func LoadManifest(path string, trust Trust) (*MjolnirManifest, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-configured path; no G304 site in this tree derives from an HTTP request (verified)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	switch {
	case trust.PubKey != nil:
		raw, err := os.ReadFile(SealPath(path)) // #nosec G304 -- next to the operator-configured manifest
		if err != nil {
			return nil, fmt.Errorf("read seal: %w", err)
		}
		var seal gungnir.GungnirSeal
		if err := json.Unmarshal(raw, &seal); err != nil {
			return nil, fmt.Errorf("parse seal: %w", err)
		}
		if err := gungnir.Verify(data, &seal, trust.PubKey); err != nil {
			return nil, fmt.Errorf("manifest seal: %w", err)
		}
	case !trust.AllowUnsigned:
		return nil, ErrUnverified
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
	data, err := os.ReadFile(path) // #nosec G304 -- operator-configured path; no G304 site in this tree derives from an HTTP request (verified)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Reload reads and verifies the manifest again. A seal checked once at
// startup would keep vouching for the baseline long after it expired;
// re-checking it every scan makes expiry (and a swapped manifest or seal)
// stop the daemon, and lets a re-signed manifest take effect without a
// restart.
func (h *Heimdall) Reload() error {
	m, err := LoadManifest(h.manifestPath, h.trust)
	if err != nil {
		return err
	}
	h.mu.Lock()
	h.manifest = m
	h.mu.Unlock()
	return nil
}

// Run starts the daemon loop. It returns an error when the manifest stops
// verifying (Reload).
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
			if err := h.Reload(); err != nil {
				return fmt.Errorf("manifest no longer trusted, stopping: %w", err)
			}
			h.ScanFiles()
		}
	}
}

// signManifest writes manifest's detached seal with the private key seed in
// keyPath (gungnir.LoadSeedFile refuses a key file others can read).
func signManifest(manifest, keyPath string, ttl time.Duration) error {
	priv, err := gungnir.LoadSeedFile(keyPath)
	if err != nil {
		return err
	}
	signer, err := gungnir.NewSignerWithID(priv)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(manifest) // #nosec G304 -- operator-supplied path
	if err != nil {
		return err
	}
	seal, err := signer.SignFor(data, ttl)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(seal, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SealPath(manifest), append(out, '\n'), 0o644) // #nosec G306 G703 -- a seal is public; the path is the operator's own --manifest flag plus ".seal"
}

func main() {
	var (
		manifestPath  = flag.String("manifest", "references/baseline/mjolnir.example.yaml", "Path to Mjölnir manifest")
		nodeID        = flag.String("node-id", "", "Node identifier (defaults to hostname)")
		scanInterval  = flag.Duration("scan-interval", 30*time.Second, "Baseline scan interval")
		pubKeyPath    = flag.String("pubkey", "", "ML-DSA-65 public key file: verify <manifest>.seal before trusting the manifest")
		allowUnsigned = flag.Bool("allow-unsigned", false, "load a manifest without verifying a seal (lab runs only)")
		signKey       = flag.String("sign-key", "", "sign --manifest with this private key seed, write <manifest>.seal, exit")
		signTTL       = flag.Duration("sign-ttl", 30*24*time.Hour, "how long a seal written by --sign-key stays valid")
	)
	flag.Parse()

	if *signKey != "" {
		if err := signManifest(*manifestPath, *signKey, *signTTL); err != nil {
			fmt.Fprintf(os.Stderr, "sign manifest: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", SealPath(*manifestPath))
		return
	}

	if *nodeID == "" {
		hostname, _ := os.Hostname()
		*nodeID = hostname
	}

	fmt.Printf("Heimdall Daemon starting (node=%s)\n", *nodeID)
	fmt.Printf("ADR-043 hard condition #1: alerts only, no auto-restore\n")

	trust := Trust{AllowUnsigned: *allowUnsigned}
	if *pubKeyPath != "" {
		pub, err := gungnir.LoadPublicKeyFile(*pubKeyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "load public key: %v\n", err)
			os.Exit(1)
		}
		trust.PubKey = pub
	} else if *allowUnsigned {
		fmt.Fprintln(os.Stderr, "WARNING: --allow-unsigned: the manifest is not verified; drift against a forged baseline goes unseen")
	}
	manifest, err := LoadManifest(*manifestPath, trust)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded Mjölnir: %s v%s (%d files, %d packages)\n",
		manifest.Metadata.Name, manifest.Metadata.Version,
		len(manifest.Spec.Files), len(manifest.Spec.Packages))

	h := &Heimdall{
		nodeID:       *nodeID,
		manifestPath: *manifestPath,
		trust:        trust,
		manifest:     manifest,
		aggregator:   enkrateia.NewAggregator(1024),
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
