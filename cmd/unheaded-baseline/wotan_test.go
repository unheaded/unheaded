// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

// fakeWotan stands in for Wotan's HTTP control plane: subscribe approves
// every name, publish records the payload.
type fakeWotan struct {
	mu        sync.Mutex
	srv       *httptest.Server
	names     []string
	topics    []string
	payloads  []string
	subscribe int
}

func newFakeWotan(t *testing.T) *fakeWotan {
	t.Helper()
	fw := &fakeWotan{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/topics/{topic}/subscribe", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DisplayName string `json:"display_name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fw.mu.Lock()
		fw.subscribe++
		fw.names = append(fw.names, body.DisplayName)
		fw.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"subscriber": map[string]string{
			"subscriber_id": "sub-1", "topic": r.PathValue("topic"),
			"display_name": body.DisplayName, "status": "approved"}})
	})
	mux.HandleFunc("POST /api/v1/topics/{topic}/publish", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SubscriberID string `json:"subscriber_id"`
			Payload      string `json:"payload"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fw.mu.Lock()
		fw.topics = append(fw.topics, r.PathValue("topic"))
		fw.payloads = append(fw.payloads, body.Payload)
		fw.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	})
	fw.srv = httptest.NewServer(mux)
	t.Cleanup(fw.srv.Close)
	return fw
}

func (fw *fakeWotan) addr() string { return strings.TrimPrefix(fw.srv.URL, "http://") }

func TestWotan_PublishesDeviations(t *testing.T) {
	fw := newFakeWotan(t)
	f := newFixture(t)
	f.opts.wotanAddr = fw.addr()
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Deviations != 3 {
		t.Fatalf("deviations = %d, want 3 (fixture changed?)", rep.Deviations)
	}
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if len(fw.payloads) != 1 {
		t.Fatalf("publishes = %d, want 1", len(fw.payloads))
	}
	if fw.topics[0] != "compliance.deviation.east" {
		t.Errorf("topic = %q, want compliance.deviation.east", fw.topics[0])
	}
	if len(fw.names) != 1 || fw.names[0] != "unheaded-baseline" {
		t.Errorf("display names = %v, want [unheaded-baseline]", fw.names)
	}
	var msg deviationMessage
	if err := json.Unmarshal([]byte(fw.payloads[0]), &msg); err != nil {
		t.Fatalf("payload is not JSON: %v: %s", err, fw.payloads[0])
	}
	if msg.Host != "east" || msg.Mode != "audit" || len(msg.Deviations) != 3 {
		t.Errorf("message = %+v", msg)
	}
	if msg.TS != f.opts.now().UnixMilli() {
		t.Errorf("ts = %d, want %d", msg.TS, f.opts.now().UnixMilli())
	}
	raw, err := os.ReadFile(filepath.Join(f.opts.stateDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if msg.ReportSHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("report_sha256 = %s, want the sha256 of report.json", msg.ReportSHA256)
	}
	ids := map[string]bool{}
	for _, d := range msg.Deviations {
		ids[d.ID] = true
		if d.Kind == "" || !strings.HasPrefix(d.ID, d.Kind+":") {
			t.Errorf("deviation %+v: kind does not prefix id", d)
		}
	}
	for _, want := range []string{"host-sysctl:east:fs.suid_dumpable=0", "host-sshd:east:maxauthtries<=4", "host-probe:east:auditd-running"} {
		if !ids[want] {
			t.Errorf("deviation %s missing from %v", want, ids)
		}
	}
	// Metadata only: the observed values in Detail stay in report.json.
	if strings.Contains(fw.payloads[0], "detail") || strings.Contains(fw.payloads[0], "would_enforce") {
		t.Errorf("payload carries check details: %s", fw.payloads[0])
	}
}

func TestWotan_NoPublishWhenClean(t *testing.T) {
	fw := newFakeWotan(t)
	f := newFixture(t)
	if err := os.WriteFile(f.opts.baselinePath,
		[]byte("version: 1\nsources:\n  - {kind: host-sysctl, ref: \"east:kernel.kptr_restrict>=1\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.opts.wotanAddr = fw.addr()
	rep, err := f.run(t)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Deviations != 0 {
		t.Fatalf("deviations = %d, want 0", rep.Deviations)
	}
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.subscribe != 0 || len(fw.payloads) != 0 {
		t.Errorf("clean run talked to Wotan: %d subscribes, %d publishes", fw.subscribe, len(fw.payloads))
	}
}

func TestWotan_NoPublishWithoutAddr(t *testing.T) {
	fw := newFakeWotan(t)
	f := newFixture(t) // wotanAddr empty
	if _, err := f.run(t); err != nil {
		t.Fatal(err)
	}
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.subscribe != 0 || len(fw.payloads) != 0 {
		t.Errorf("run without -wotan-addr talked to Wotan: %d subscribes, %d publishes", fw.subscribe, len(fw.payloads))
	}
}

// A Wotan that is down must not cost the host its report: deviations are
// data, report.json is the record, the failure is logged.
func TestWotan_UnreachableIsLoggedNotFatal(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // refuses connections from here on
	f := newFixture(t)
	f.opts.wotanAddr = addr
	rep, err := f.run(t)
	if err != nil {
		t.Fatalf("run failed on an unreachable Wotan: %v", err)
	}
	if rep.Deviations != 3 {
		t.Errorf("deviations = %d, want 3", rep.Deviations)
	}
	if _, err := os.Stat(filepath.Join(f.opts.stateDir, "report.json")); err != nil {
		t.Errorf("report.json not written: %v", err)
	}
	if !strings.Contains(f.logBuf.String(), "WARN wotan publish") {
		t.Errorf("no WARN line for the failed publish:\n%s", f.logBuf.String())
	}
}

func TestWotanAddrFromEnv(t *testing.T) {
	t.Setenv("WOTAN_ADDR", "")
	if got := wotanAddrFrom("", os.Getenv); got != "" {
		t.Errorf("empty flag, empty env = %q, want off", got)
	}
	t.Setenv("WOTAN_ADDR", "127.0.0.1:18000")
	if got := wotanAddrFrom("", os.Getenv); got != "127.0.0.1:18000" {
		t.Errorf("env not honoured: %q", got)
	}
	if got := wotanAddrFrom("10.0.0.1:18000", os.Getenv); got != "10.0.0.1:18000" {
		t.Errorf("flag must win over env: %q", got)
	}
}

// The shipped Wotan allowlist must auto-approve the agent's name, or every
// publish sits pending and nobody sees a deviation.
func TestWotan_ShippedAllowlistApprovesTheAgent(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "configs", "wotan.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Topics struct {
			AutoApprove []string `yaml:"auto_approve"`
		} `yaml:"topics"`
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	for _, n := range cfg.Topics.AutoApprove {
		if n == wotanDisplayName {
			return
		}
	}
	t.Errorf("configs/wotan.yaml topics.auto_approve lacks %q", wotanDisplayName)
}
