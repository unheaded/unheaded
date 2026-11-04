// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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

func TestCheckWireListen(t *testing.T) {
	for _, c := range []struct {
		addr string
		auth bool
		ok   bool
	}{
		{"127.0.0.1:19015", false, true},
		{"[::1]:19015", false, true},
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
