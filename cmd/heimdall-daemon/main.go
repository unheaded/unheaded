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
	"unheaded/pkg/gjallarhorn"
	"unheaded/pkg/gungnir"
	wotanClient "unheaded/pkg/wotan-client"
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

	// drift publishes each alert to Wotan, signed (ADR-105); nil prints
	// alerts to stdout only.
	drift *driftSink
	// printAlert writes an alert line; nil means stdout.
	printAlert func(string)
	// logf writes an operational line ([HORN], [BPF]); nil means stdout.
	logf func(format string, args ...any)

	// hookEvents carries decoded BPF ringbuf records (bpf.go); nil = no
	// BPF, polling only.
	hookEvents <-chan hookEvent
	// hornPolicy decides what a Gjallarhorn does; nil = horns ignored.
	hornPolicy *hornPolicy
	// hornCounters reads the XDP observer's counters for log lines.
	hornCounters func() map[string]uint64
	// debounce is the file-event coalescing delay (0 = 250 ms).
	debounce time.Duration

	// metrics holds the drift counters and the scan histogram /metrics
	// serves; nil records nothing.
	metrics *heimdallMetrics

	// lastSeen is the hash each watched path had at the previous scan
	// ("" = missing), so event-driven scans alert on changes only.
	lastSeen map[string]string
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

// ScanFiles walks the manifest file list and reports every drifted file
// (the polling and reverify "reminder" flows).
// Byte-level diff (v1) — semantic diff is deferred to v2 (BlackMage #5).
func (h *Heimdall) ScanFiles() { h.scan(false) }

// ScanChanged reports only files whose drift is new or whose content
// changed since the previous scan (the event-driven flow: a busy host's
// write noise must not re-alert the same drift every 250 ms).
func (h *Heimdall) ScanChanged() { h.scan(true) }

func (h *Heimdall) scan(onlyChanged bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.manifest == nil {
		return
	}
	start := time.Now()
	defer func() { h.metrics.observeScan(time.Since(start)) }()
	if h.lastSeen == nil {
		h.lastSeen = map[string]string{}
	}

	for _, f := range h.manifest.Spec.Files {
		actual, err := hashFile(f.Path)
		prev, known := h.lastSeen[f.Path]
		h.lastSeen[f.Path] = actual // "" when missing
		if onlyChanged && known && prev == actual {
			continue
		}
		if err != nil {
			// File missing → drift
			h.aggregator.HandleDrift(enkrateia.DriftEvent{
				NodeID:       h.nodeID,
				Path:         f.Path,
				HashExpected: []byte(f.SHA256),
				Severity:     "alert",
				DetectedAt:   time.Now(),
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
	// Alert drain goroutine: prints every alert and, with a drift sink,
	// queues it for a signed publish to Wotan drift.detected.<node_id>
	// (ADR-105). Enqueue never blocks, so a slow Wotan cannot stall this.
	printAlert := h.printAlert
	if printAlert == nil {
		printAlert = func(msg string) { fmt.Printf("[ALERT] %s\n", msg) }
	}
	if h.drift != nil {
		go h.drift.run(ctx)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case alert := <-h.aggregator.Alerts():
				printAlert(alert.Message)
				if h.drift != nil {
					h.drift.Enqueue(h.driftRecord(alert))
				}
			}
		}
	}()

	logf := h.logf
	if logf == nil {
		logf = func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	}
	delay := h.debounce
	if delay == 0 {
		delay = 250 * time.Millisecond
	}

	// Three triggers, one loop: the periodic poll, BPF file events
	// (vfs_write / execve, debounced), and Gjallarhorn horns seen by XDP
	// (rate-limited). Any of them can re-check the seal; a seal that stops
	// verifying stops the daemon, whichever trigger found it.
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()
	deb := newDebouncer(delay)
	filter := hookFilter{selfPID: uint32(os.Getpid())} // #nosec G115 -- pids fit in u32 (pid_max <= 2^22)
	var debTimer *time.Timer
	var debC <-chan time.Time
	defer func() {
		if debTimer != nil {
			debTimer.Stop()
		}
	}()
	events := h.hookEvents

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := h.Reload(); err != nil {
				return fmt.Errorf("manifest no longer trusted, stopping: %w", err)
			}
			h.ScanFiles()
		case ev, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			now := time.Now()
			if ev.Horn != nil && h.hornPolicy != nil {
				act := h.hornPolicy.Decide(*ev.Horn, now)
				counters := ""
				if h.hornCounters != nil {
					c := h.hornCounters()
					counters = fmt.Sprintf(" xdp seen=%d valid=%d bad_magic=%d bad_kind=%d bad_len=%d bad_reserved=%d",
						c["seen"], c["valid"], c["bad_magic"], c["bad_kind"], c["bad_len"], c["bad_reserved"])
				}
				logf("[HORN] kind=%d cluster=%d manifest_ptr=0x%016x src=%08x action=%s%s",
					ev.Horn.Kind, ev.Horn.Cluster, ev.Horn.ManifestPtr, ev.Horn.SrcHash, act, counters)
				if act == HornReverify {
					if err := h.Reload(); err != nil {
						return fmt.Errorf("manifest no longer trusted, stopping: %w", err)
					}
					h.ScanFiles()
					logf("[HORN] re-verified manifest and scanned (event-driven)")
				}
			}
			if ev.Drift != nil && filter.Wanted(*ev.Drift) {
				deb.Kick(now)
				if debC == nil {
					debTimer = time.NewTimer(deb.Wait(now))
					debC = debTimer.C
				}
			}
		case <-debC:
			now := time.Now()
			if deb.Due(now) {
				debC = nil
				h.ScanChanged()
			} else {
				debTimer.Reset(deb.Wait(now))
			}
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
		wotanGRPC     = flag.String("wotan-grpc", "", "Wotan gRPC address: publish each drift alert to drift.detected.<node-id>, signed (UNHEADED_GRPC_TLS=1 for TLS)")
		driftSignKey  = flag.String("drift-sign-key", "", "ML-DSA-65 seed (0600) that signs drift events; must not be the --sign-key or --pubkey key")
		publishQueue  = flag.Int("publish-queue", 256, "drift events held for Wotan before the oldest is dropped")
		bpfDir        = flag.String("bpf-dir", "", "directory with the heimdall-bpf objects: attach vfs_write/execve hooks (event-driven scans); needs CAP_BPF")
		xdpIface      = flag.String("xdp-iface", "", "attach the observe-only Gjallarhorn XDP program here (needs --bpf-dir)")
		xdpMode       = flag.String("xdp-mode", "generic", "XDP attach mode: generic | native")
		cluster       = flag.Uint("cluster", 1, "this node's Gjallarhorn cluster ID; reverify horns for other clusters are ignored")
		metricsAddr   = flag.String("metrics-addr", defaultMetricsAddr, "serve GET /metrics here (drift publish counters, scan duration); empty disables")
	)
	flag.Parse()
	wotanClient.InitServiceIdentity("heimdall") // ADR-103 §6: every Wotan call carries heimdall's service token
	if *xdpMode != "generic" && *xdpMode != "native" {
		fmt.Fprintf(os.Stderr, "--xdp-mode must be generic or native, got %q\n", *xdpMode)
		os.Exit(2)
	}
	if *cluster > 0xFFFFFFFF {
		fmt.Fprintln(os.Stderr, "--cluster must fit in 32 bits")
		os.Exit(2)
	}

	// ADR-043 #4: the drift key is never the manifest key, whichever mode
	// this run is in.
	if *driftSignKey != "" && *signKey != "" {
		if _, err := loadDriftSigner(*driftSignKey, *signKey, nil); err != nil {
			fmt.Fprintf(os.Stderr, "drift key: %v\n", err)
			os.Exit(1)
		}
	}

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
	// The node ID names a topic segment: normalize it so a hostname with
	// dots or wildcards cannot address another topic.
	normalized, err := NormalizeNodeID(*nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "node id %q: %v\n", *nodeID, err)
		os.Exit(1)
	}
	*nodeID = normalized

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

	hm := newHeimdallMetrics()
	h := &Heimdall{
		nodeID:       *nodeID,
		manifestPath: *manifestPath,
		trust:        trust,
		manifest:     manifest,
		aggregator:   enkrateia.NewAggregator(1024),
		metrics:      hm,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bound, stopMetrics, err := startMetricsServer(*metricsAddr, hm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "metrics listener %s: %v\n", *metricsAddr, err)
		os.Exit(1)
	}
	if bound != "" {
		fmt.Printf("Metrics on http://%s/metrics\n", bound)
	} else {
		fmt.Println("Metrics listener disabled (--metrics-addr empty)")
	}
	defer func() {
		sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer scancel()
		_ = stopMetrics(sctx)
	}()

	sink, err := setupDriftPublishing(ctx, *wotanGRPC, *driftSignKey, *signKey, trust.PubKey, *nodeID, *publishQueue, hm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "drift publishing: %v\n", err)
		os.Exit(1)
	}
	h.drift = sink

	// BPF hooks are optional: any load/attach failure is logged once and
	// the daemon keeps polling (S5 Step 150).
	if *bpfDir != "" {
		hooks, errs := loadHooks(HookConfig{Dir: *bpfDir, XDPIface: *xdpIface, XDPMode: *xdpMode})
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "[BPF] %v (continuing without it)\n", e)
		}
		if hooks != nil {
			defer hooks.Close()
			h.hookEvents = hooks.Events()
			h.hornCounters = hooks.HornCounters
			for name, n := range hooks.Verifier {
				fmt.Printf("[BPF] %s loaded, verifier processed %d insns\n", name, n)
			}
			defer func() {
				c := hooks.HornCounters()
				fmt.Printf("[BPF] xdp counters at exit: seen=%d valid=%d bad_magic=%d bad_kind=%d bad_len=%d bad_reserved=%d rb_full=%d\n",
					c["seen"], c["valid"], c["bad_magic"], c["bad_kind"], c["bad_len"], c["bad_reserved"], c["rb_full"])
			}()
		}
	} else if *xdpIface != "" {
		fmt.Fprintln(os.Stderr, "--xdp-iface needs --bpf-dir")
		os.Exit(2)
	}
	// A real NIC filters an unjoined multicast group before XDP sees it, so
	// a seed that observes horns on --xdp-iface must hold the group itself.
	if *xdpIface != "" {
		if mc, err := joinHornGroup(*xdpIface); err != nil {
			fmt.Fprintf(os.Stderr, "[HORN] %v: bootstrap horns may not reach the observer on a real NIC\n", err)
		} else {
			defer mc.Close()
			fmt.Printf("[HORN] joined %s on %s\n", gjallarhorn.MulticastGroup, *xdpIface)
		}
	}
	h.hornPolicy = newHornPolicy(uint32(*cluster), time.Now()) // #nosec G115 -- range-checked above

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
