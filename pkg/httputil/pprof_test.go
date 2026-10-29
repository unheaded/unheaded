// SPDX-License-Identifier: GPL-3.0-or-later

package httputil

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterPprofServesIndex(t *testing.T) {
	mux := http.NewServeMux()
	RegisterPprof(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "goroutine") {
		t.Fatalf("index: %d %.80q", rec.Code, rec.Body.String())
	}
}

func TestCheckLoopback(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1:6060": true,
		"127.1.2.3:6060": true,
		"[::1]:6060":     true,
		"localhost:6060": true,
		":6060":          false,
		"0.0.0.0:6060":   false,
		"[::]:6060":      false,
		"10.0.0.5:6060":  false,
		"example.com:80": false,
		"127.0.0.1":      false,
	} {
		if err := CheckLoopback(addr); (err == nil) != ok {
			t.Errorf("CheckLoopback(%q) = %v, want ok=%v", addr, err, ok)
		}
	}
}

func TestStartPprofRefusesNonLoopback(t *testing.T) {
	if err := StartPprof("0.0.0.0:0"); err == nil {
		t.Fatal("StartPprof accepted a wildcard address")
	}
}

func TestStartPprofServes(t *testing.T) {
	// Find a free loopback port, then hand it to StartPprof.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	if err := StartPprof(addr); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get("http://" + addr + "/debug/pprof/cmdline")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || len(body) == 0 {
		t.Fatalf("cmdline: %d %q", resp.StatusCode, body)
	}
	if err := StartPprof(addr); err == nil {
		t.Error("a second StartPprof on a bound port should fail")
	}
}
