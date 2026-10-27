// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/enkrateia"
	"unheaded/pkg/gungnir"
	"unheaded/pkg/wotansig"
)

// fakePublisher records signed publishes; block, when set, holds every
// publish until it is closed (a Wotan that stopped answering).
type fakePublisher struct {
	mu    sync.Mutex
	calls []fakeCall
	err   error
	block chan struct{}
}

type fakeCall struct {
	topic   string
	payload []byte
	signer  *wotansig.TopicSigner
}

func (f *fakePublisher) PublishSigned(ctx context.Context, topic string, payload []byte, signer *wotansig.TopicSigner) error {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{topic, append([]byte(nil), payload...), signer})
	return f.err
}

func (f *fakePublisher) snapshot() []fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeCall(nil), f.calls...)
}

func driftSigner(t *testing.T) (*wotansig.TopicSigner, string) {
	t.Helper()
	seed, _ := keyPair(t, t.TempDir(), 0x42)
	s, err := loadDriftSigner(seed, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, seed
}

func waitCalls(t *testing.T, f *fakePublisher, n int) []fakeCall {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c := f.snapshot(); len(c) >= n {
			return c
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("got %d publishes, want %d", len(f.snapshot()), n)
	return nil
}

func TestNormalizeNodeID(t *testing.T) {
	for in, want := range map[string]string{
		"east":                  "east",
		"East.Example.COM":      "east-example-com",
		"n1.config":             "n1-config",
		"node_01":               "node-01",
		"--a..b--":              "a-b",
		"west\x00evil":          "west-evil",
		"drift.detected.*":      "drift-detected",
		"héimdall":              "h-imdall",
		strings.Repeat("a", 64): strings.Repeat("a", 64),
	} {
		got, err := NormalizeNodeID(in)
		if err != nil || got != want {
			t.Errorf("NormalizeNodeID(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "*", "...", "#", strings.Repeat("a", 254)} {
		if got, err := NormalizeNodeID(bad); err == nil {
			t.Errorf("NormalizeNodeID(%q) = %q, want an error", bad, got)
		}
	}
}

// A node ID cannot add topic segments: whatever the hostname, the topic is
// exactly drift.detected.<one segment>.
func FuzzNormalizeNodeID(f *testing.F) {
	for _, s := range []string{"east", "a.b.c", "*", "#", "", "x\x00y", "ÄÖ"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		id, err := NormalizeNodeID(in)
		if err != nil {
			return
		}
		if id == "" || len(id) > maxNodeIDLen || strings.HasPrefix(id, "-") || strings.HasSuffix(id, "-") {
			t.Fatalf("NormalizeNodeID(%q) = %q", in, id)
		}
		for _, r := range id {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				t.Fatalf("NormalizeNodeID(%q) = %q has %q", in, id, r)
			}
		}
		if topic := DriftTopic(id); strings.Count(topic, ".") != 2 {
			t.Fatalf("topic %q has extra segments", topic)
		}
	})
}

// One alert becomes one signed publish on drift.detected.<node> whose body
// carries the drift record, and the signature verifies for that topic.
func TestDriftSink_PublishesOneSignedRecord(t *testing.T) {
	signer, _ := driftSigner(t)
	pub := &fakePublisher{}
	sink := newDriftSink(pub, signer, "drift.detected.east", 8, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sink.run(ctx)

	at := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	sink.Enqueue(DriftRecord{NodeID: "east", Path: "/etc/ssh/sshd_config", Kind: "modified",
		ExpectedSHA256: "aa", ActualSHA256: "bb", DetectedAt: at, ManifestName: "kingdom", ManifestVersion: "0.1.0"})
	calls := waitCalls(t, pub, 1)
	if calls[0].topic != "drift.detected.east" || calls[0].signer != signer {
		t.Fatalf("publish %+v", calls[0])
	}
	var got map[string]any
	if err := json.Unmarshal(calls[0].payload, &got); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]any{"node_id": "east", "path": "/etc/ssh/sshd_config", "kind": "modified",
		"expected_sha256": "aa", "actual_sha256": "bb", "detected_at": "2026-10-23T12:00:00Z",
		"manifest_name": "kingdom", "manifest_version": "0.1.0"} {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if len(got) != 8 {
		t.Errorf("body has %d fields, want 8: %v", len(got), got)
	}
	if sink.published.Value() != 1 {
		t.Errorf("published = %v", sink.published.Value())
	}
}

// Wotan refusing or failing a publish costs that alert's publish, counted;
// the next alert still goes out.
func TestDriftSink_PublishFailureCounted(t *testing.T) {
	signer, _ := driftSigner(t)
	pub := &fakePublisher{err: errors.New("unavailable")}
	sink := newDriftSink(pub, signer, "drift.detected.east", 8, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sink.run(ctx)
	sink.Enqueue(DriftRecord{NodeID: "east", Path: "/a"})
	sink.Enqueue(DriftRecord{NodeID: "east", Path: "/b"})
	waitCalls(t, pub, 2)
	deadline := time.Now().Add(time.Second)
	for sink.failed.Value() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if sink.failed.Value() != 2 {
		t.Fatalf("heimdall_drift_publish_failed_total = %v, want 2", sink.failed.Value())
	}
}

// A Wotan that hangs cannot block the scan loop: Enqueue never waits, the
// queue holds at most its size, and the oldest records go first, counted.
func TestDriftSink_BoundedQueueDropsOldest(t *testing.T) {
	signer, _ := driftSigner(t)
	pub := &fakePublisher{block: make(chan struct{})}
	sink := newDriftSink(pub, signer, "drift.detected.east", 4, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sink.run(ctx)

	sink.Enqueue(DriftRecord{Path: "/in-flight"})
	deadline := time.Now().Add(time.Second)
	for sink.Len() != 0 && time.Now().Before(deadline) { // the run loop has taken it and is blocked
		time.Sleep(5 * time.Millisecond)
	}
	start := time.Now()
	for i := 0; i < 10; i++ {
		sink.Enqueue(DriftRecord{Path: "/p" + string(rune('0'+i))})
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("Enqueue blocked on a hung publisher")
	}
	if sink.Len() != 4 {
		t.Fatalf("queue holds %d, want 4", sink.Len())
	}
	if sink.dropped.Value() != 6 {
		t.Fatalf("dropped = %v, want 6", sink.dropped.Value())
	}
	close(pub.block)
	calls := waitCalls(t, pub, 5)
	var paths []string
	for _, c := range calls {
		var r DriftRecord
		_ = json.Unmarshal(c.payload, &r)
		paths = append(paths, r.Path)
	}
	if strings.Join(paths, ",") != "/in-flight,/p6,/p7,/p8,/p9" {
		t.Fatalf("published %v: the newest records must survive", paths)
	}
}

// Run with a hung publisher: every alert is still printed and the scan loop
// keeps going (it returns at the deadline, not stuck in a publish).
func TestRun_DriftPublisherDownDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mjolnir.yaml")
	missing := strings.Replace(sealedManifest, "path: /etc/hostname", "path: "+filepath.Join(dir, "gone"), 1)
	if err := os.WriteFile(path, []byte(missing), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path, Trust{AllowUnsigned: true})
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := driftSigner(t)
	pub := &fakePublisher{block: make(chan struct{})}
	defer close(pub.block)
	h := &Heimdall{nodeID: "east", manifestPath: path, trust: Trust{AllowUnsigned: true}, manifest: m,
		aggregator: enkrateia.NewAggregator(16), drift: newDriftSink(pub, signer, "drift.detected.east", 2, nil)}
	var printed sync.WaitGroup
	printed.Add(1)
	var once sync.Once
	h.printAlert = func(string) { once.Do(printed.Done) }

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	if err := h.Run(ctx, 10*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run: %v", err)
	}
	printed.Wait()
	if h.drift.dropped.Value() == 0 {
		t.Fatal("expected the bounded queue to drop while the publisher hung")
	}
}

// ADR-043 #4 key separation: the drift key must not be the manifest signing
// key (--sign-key) or the key that verifies the manifest (--pubkey).
func TestLoadDriftSigner_KeySeparation(t *testing.T) {
	seed, pub := keyPair(t, t.TempDir(), 0x11)
	if _, err := loadDriftSigner(seed, seed, nil); !errors.Is(err, ErrKeyReuse) {
		t.Fatalf("drift key = --sign-key: %v", err)
	}
	same, _ := keyPair(t, t.TempDir(), 0x11) // same seed, other file
	if _, err := loadDriftSigner(same, seed, nil); !errors.Is(err, ErrKeyReuse) {
		t.Fatalf("drift key = --sign-key (copy): %v", err)
	}
	if _, err := loadDriftSigner(seed, "", pub); !errors.Is(err, ErrKeyReuse) {
		t.Fatalf("drift key = --pubkey: %v", err)
	}
	other, otherPub := keyPair(t, t.TempDir(), 0x22)
	if s, err := loadDriftSigner(other, seed, pub); err != nil || s == nil {
		t.Fatalf("separate key refused: %v", err)
	}
	if _, err := loadDriftSigner(seed, other, otherPub); err != nil {
		t.Fatalf("separate keys refused: %v", err)
	}
}

func TestLoadDriftSigner_RefusesReadableKey(t *testing.T) {
	seed, _ := keyPair(t, t.TempDir(), 0x33)
	if err := os.Chmod(seed, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDriftSigner(seed, "", nil); !errors.Is(err, gungnir.ErrKeyFilePerms) {
		t.Fatalf("group-readable drift key: %v", err)
	}
	if _, err := loadDriftSigner(filepath.Join(t.TempDir(), "missing"), "", nil); err == nil {
		t.Fatal("missing key file accepted")
	}
}

// The signer loaded from the seed file signs drift events that verify with
// the seed's public key, and nothing else.
func TestLoadDriftSigner_SignsVerifiably(t *testing.T) {
	seed, pub := keyPair(t, t.TempDir(), 0x44)
	s, err := loadDriftSigner(seed, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	sg, err := s.Sign("drift.detected.east", heimdallSender, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := wotansig.VerifyEvent("drift.detected.east", heimdallSender, []byte("{}"), sg, [][]byte{pub.Bytes()}); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

// ADR-043 #1: Heimdall reports drift and never repairs it. No code in this
// package writes, renames, removes or re-permissions a file, except the
// seal writer (signManifest), which writes <manifest>.seal.
func TestNoRestorePath(t *testing.T) {
	forbidden := map[string]bool{
		"WriteFile": true, "Rename": true, "Remove": true, "RemoveAll": true, "Chmod": true, "Chown": true,
		"Lchown": true, "Chtimes": true, "Create": true, "OpenFile": true, "Truncate": true, "Symlink": true,
		"Link": true, "Mkdir": true, "MkdirAll": true, "CopyFS": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || (pkg.Name != "os" && pkg.Name != "ioutil") || !forbidden[sel.Sel.Name] {
					return true
				}
				if fn.Name.Name == "signManifest" && sel.Sel.Name == "WriteFile" {
					return true
				}
				t.Errorf("%s: %s calls %s.%s: Heimdall is alerts-only (ADR-043 #1)", fset.Position(call.Pos()), fn.Name.Name, pkg.Name, sel.Sel.Name)
				return true
			})
		}
	}
	if checked == 0 {
		t.Fatal("no package sources found; refusing to pass vacuously")
	}
}
