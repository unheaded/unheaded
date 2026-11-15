// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/services/compliance"
)

func TestRunWire_OffByDefault(t *testing.T) {
	var out bytes.Buffer
	if code := runWire(nil, &out, io.Discard); code != 0 {
		t.Fatalf("runWire = %d, want 0", code)
	}
	if got := strings.TrimSpace(out.String()); got != wireOffMessage {
		t.Fatalf("output = %q, want %q", got, wireOffMessage)
	}
}

func TestRunWire_RefusesOperands(t *testing.T) {
	var errOut bytes.Buffer
	if code := runWire([]string{"now"}, io.Discard, &errOut); code != 2 {
		t.Fatalf("runWire(operand) = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "--help") {
		t.Fatalf("stderr = %q, want the --help hint", errOut.String())
	}
}

// ADR-111: --help on stdout, exit 0.
func TestRunWire_Help(t *testing.T) {
	var out bytes.Buffer
	if code := runWire([]string{"--help"}, &out, io.Discard); code != 0 {
		t.Fatalf("runWire(--help) = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "--serve") {
		t.Fatalf("help output has no --serve: %q", out.String())
	}
}

// wire has no --wotan: it presents no Wotan identity until it has its own
// nologin account (ADR-127).
func TestRunWire_NoWotanFlag(t *testing.T) {
	var errOut bytes.Buffer
	if code := runWire([]string{"--wotan", "127.0.0.1:19001"}, io.Discard, &errOut); code != 2 {
		t.Fatalf("runWire(--wotan) = %d, want 2", code)
	}
	var out bytes.Buffer
	if code := runWire([]string{"--help"}, &out, io.Discard); code != 0 {
		t.Fatalf("runWire(--help) = %d, want 0", code)
	}
	if strings.Contains(strings.ToLower(out.String()), "wotan") {
		t.Fatalf("help output names Wotan: %q", out.String())
	}
}

// The package cannot build a Wotan client, so serveWire's service has none and
// its subscriptions (services/compliance, as compliance-service) never run.
func TestPackageHasNoWotanClient(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		seen++
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range parsed.Imports {
			if imp.Path.Value == `"unheaded/pkg/wotan-client"` {
				t.Errorf("%s imports the Wotan client", f)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no source files found: the test is not looking at the package")
	}
}

func TestCheckWireListen(t *testing.T) {
	for _, c := range []struct {
		addr string
		auth bool
		ok   bool
	}{
		{"127.0.0.1:19015", false, true},
		{"[::1]:19015", false, true},
		{"[::ffff:127.0.0.1]:19015", false, true},
		{"[::1%lo]:19015", false, false},
		{":19015", false, false},
		{"0.0.0.0:19015", false, false},
		{"192.168.13.2:19015", false, false},
		{"192.168.13.2:19015", true, true},
		{"nonsense", true, false},
	} {
		err := checkWireListen(c.addr, c.auth)
		if (err == nil) != c.ok {
			t.Errorf("checkWireListen(%q, %v) = %v, want ok=%v", c.addr, c.auth, err, c.ok)
		}
	}
}

func TestServeWire_NoDataUntilEvaluated(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "")
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- serveWire(ctx, wireOptions{listen: "127.0.0.1:0"}, ready) }()
	var addr string
	select {
	case addr = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("listener not up after 5 s")
	}
	resp, err := http.Get("http://" + addr + "/api/v1/compliance/score")
	if err != nil {
		t.Fatalf("GET score: %v", err)
	}
	defer resp.Body.Close()
	var score struct {
		ScorePct float64 `json:"score_pct"`
		NoData   bool    `json:"no_data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&score); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !score.NoData || score.ScorePct != 0 {
		t.Fatalf("score = %+v, want no_data true and score_pct 0", score)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveWire: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveWire did not stop")
	}
}

// startWire runs serveWire on a loopback port and returns its address, its
// cancel and the channel its result arrives on.
func startWire(t *testing.T, o wireOptions) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	t.Setenv("AUTH_ENABLED", "")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	o.listen = "127.0.0.1:0"
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- serveWire(ctx, o, ready) }()
	select {
	case addr := <-ready:
		return addr, cancel, done
	case err := <-done:
		t.Fatalf("serveWire ended before it listened: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("listener not up after 5 s")
	}
	return "", nil, nil
}

func wireMetrics(t *testing.T, addr string) string {
	t.Helper()
	resp, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /metrics: %v", err)
	}
	return string(body)
}

// A ring consumer that fails used to print one stderr line and vanish: the
// plane then served a score that no packet could ever move. The failure is on
// /metrics now.
func TestServeWire_AuditRingFailureIsOnMetrics(t *testing.T) {
	addr, cancel, done := startWire(t, wireOptions{auditRing: filepath.Join(t.TempDir(), "no-such-pin")})
	const up, errs = "\ncompliance_audit_ring_up 0\n", "\ncompliance_audit_ring_errors_total 1\n"
	var body string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if body = wireMetrics(t, addr); strings.Contains(body, up) && strings.Contains(body, errs) {
			break
		}
	}
	if !strings.Contains(body, up) || !strings.Contains(body, errs) {
		t.Fatalf("/metrics does not show the failed ring consumer:\n%s", body)
	}
	if n := strings.Count(body, "# TYPE compliance_score_pct "); n != 1 {
		t.Fatalf("/metrics has %d compliance_score_pct families, want 1: one /metrics path", n)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveWire: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveWire did not stop")
	}
}

// The ring is optional: with no --audit-ring there is no ring to be up or down.
func TestServeWire_NoAuditRingNoGauge(t *testing.T) {
	addr, _, _ := startWire(t, wireOptions{})
	if body := wireMetrics(t, addr); strings.Contains(body, "compliance_audit_ring") {
		t.Fatalf("/metrics names the audit ring with no --audit-ring:\n%s", body)
	}
}

// serveWire returns only after the ring consumer has: a consumer still
// unmapping the ring when the process exits is a consumer nobody joined.
func TestServeWire_JoinsAuditRingConsumer(t *testing.T) {
	release := make(chan struct{})
	var returned atomic.Bool
	orig := consumeRing
	consumeRing = func(ctx context.Context, _ string, _ *compliance.WireComplianceService) error {
		<-ctx.Done()
		<-release
		returned.Store(true)
		return nil
	}
	t.Cleanup(func() { consumeRing = orig })

	addr, cancel, done := startWire(t, wireOptions{auditRing: "/unused"})
	if body := wireMetrics(t, addr); !strings.Contains(body, "\ncompliance_audit_ring_up 1\n") ||
		!strings.Contains(body, "\ncompliance_audit_ring_errors_total 0\n") {
		t.Fatalf("/metrics does not show a running ring consumer:\n%s", body)
	}
	cancel()
	// The consumer is held until release closes. A serveWire that does not
	// join it returns first, with returned still false.
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(release)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveWire: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveWire did not stop")
	}
	if !returned.Load() {
		t.Fatal("serveWire returned before the ring consumer goroutine did")
	}
}

// An early error return must not wait for ctx: the consumer's context ends
// with serveWire, and serveWire still joins it.
func TestServeWire_ListenErrorStopsAuditRingConsumer(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "")
	var returned atomic.Bool
	orig := consumeRing
	consumeRing = func(ctx context.Context, _ string, _ *compliance.WireComplianceService) error {
		<-ctx.Done()
		returned.Store(true)
		return nil
	}
	t.Cleanup(func() { consumeRing = orig })

	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	done := make(chan error, 1)
	// ctx is never cancelled: only serveWire's own return can end the consumer.
	go func() {
		done <- serveWire(context.Background(), wireOptions{listen: busy.Addr().String(), auditRing: "/unused"}, nil)
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("serveWire on a busy address = nil, want an error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveWire did not return from a listen error: it is waiting on the ring consumer")
	}
	if !returned.Load() {
		t.Fatal("serveWire returned before the ring consumer goroutine did")
	}
}
