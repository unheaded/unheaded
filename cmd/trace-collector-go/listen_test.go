// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/auth/authtest"
	"unheaded/pkg/ports"
)

// M10: with no flags the collector binds loopback, in both modes.
func TestDefaultBindIsLoopback(t *testing.T) {
	for name, port := range map[string]int{
		"http-addr":    ports.TraceCollectorHTTP,
		"metrics-addr": ports.TraceCollectorMetrics,
	} {
		if got, want := flag.Lookup(name).DefValue, ports.LoopbackAddr(port); got != want {
			t.Errorf("--%s default = %q, want %q", name, got, want)
		}
	}
}

// M10: a listener that reaches a network needs auth on.
func TestOpenListeners_RefusesOpenBindWithAuthOff(t *testing.T) {
	if _, err := openListeners("127.0.0.1:0,0.0.0.0:0", auth.ServiceAuthConfig{}); !errors.Is(err, auth.ErrOpenListener) {
		t.Fatalf("every-interface bind with auth off: %v, want ErrOpenListener", err)
	}
	ls, err := openListeners("127.0.0.1:0", auth.ServiceAuthConfig{})
	if err != nil {
		t.Fatalf("loopback with auth off: %v", err)
	}
	ls.close()
}

// close closes what openListeners bound.
func (l listeners) close() {
	for _, ln := range l.bound {
		_ = ln.Close()
	}
}

// lateAddr is an address no host holds (TEST-NET-1).
const lateAddr = "192.0.2.1:16671"

func skipIfBindable(t *testing.T) {
	t.Helper()
	if ln, err := net.Listen("tcp", lateAddr); err == nil {
		_ = ln.Close()
		t.Skip("this host binds addresses it does not hold (ip_nonlocal_bind)")
	}
}

// An address not on the host yet, an overlay still down, does not stop the
// collector: its work is the BPF programs.
func TestOpenListeners_AddressNotOnHostDoesNotStopTheStart(t *testing.T) {
	skipIfBindable(t)
	ls, err := openListeners("127.0.0.1:0,"+lateAddr, auth.ServiceAuthConfig{Enabled: true})
	if err != nil {
		t.Fatalf("openListeners: %v", err)
	}
	defer ls.close()
	if len(ls.bound) != 1 || len(ls.pending) != 1 || ls.pending[0] != lateAddr {
		t.Fatalf("bound = %d, pending = %q", len(ls.bound), ls.pending)
	}
}

// A taken port or a bad address still stops the start, and nothing stays open.
func TestOpenListeners_OtherFailuresStopTheStart(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	free := freeAddr(t)
	for name, list := range map[string]string{
		"taken port":  free + "," + taken.Addr().String(),
		"bad address": free + ",not an address",
	} {
		if _, err := openListeners(list, auth.ServiceAuthConfig{Enabled: true}); err == nil {
			t.Fatalf("%s: start was not stopped", name)
		}
		ln, err := net.Listen("tcp", free)
		if err != nil {
			t.Fatalf("%s: %s left open: %v", name, free, err)
		}
		_ = ln.Close()
	}
}

// freeAddr is a loopback address nothing is listening on.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}

// swapBind stands in for the host: bind answers every retry.
func swapBind(t *testing.T, bind func(string) (net.Listener, error)) {
	t.Helper()
	savedBind, savedRetry := bindAddr, listenRetry
	t.Cleanup(func() { bindAddr, listenRetry = savedBind, savedRetry })
	bindAddr, listenRetry = bind, time.Millisecond
}

// A pending address is served once the host holds it.
func TestServeOn_ServesAPendingAddressOnceItAppears(t *testing.T) {
	stand := freeAddr(t)
	var tries atomic.Int32
	swapBind(t, func(addr string) (net.Listener, error) {
		if addr != lateAddr {
			return nil, fmt.Errorf("bound %s, want %s", addr, lateAddr)
		}
		if tries.Add(1) < 3 {
			return nil, &net.OpError{Op: "listen", Err: syscall.EADDRNOTAVAIL}
		}
		return net.Listen("tcp", stand)
	})
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") }),
		ReadHeaderTimeout: time.Second,
	}
	defer func() { _ = srv.Close() }()
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	serveOn(ctx, srv, listeners{pending: []string{lateAddr}, stop: cancel})

	client := &http.Client{Timeout: time.Second}
	var err error
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		var resp *http.Response
		if resp, err = client.Get("http://" + stand + "/"); err == nil {
			_ = resp.Body.Close()
			if ctx.Err() != nil {
				t.Fatalf("run stopped: %v", context.Cause(ctx))
			}
			return
		}
	}
	t.Fatalf("GET %s: %v", stand, err)
}

// A pending address that turns out taken ends the run with a failure, so the
// collector shuts down its BPF programs and the unit restarts it.
func TestServeOn_LateFailureEndsTheRun(t *testing.T) {
	swapBind(t, func(string) (net.Listener, error) { return nil, syscall.EADDRINUSE })
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	serveOn(ctx, &http.Server{ReadHeaderTimeout: time.Second}, listeners{pending: []string{lateAddr}, stop: cancel})
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("run was not ended")
	}
	if err := runFailure(ctx); !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("runFailure = %v, want EADDRINUSE", err)
	}
}

// A signal is a clean stop, not a failure.
func TestRunFailure_SignalIsNotAFailure(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	if err := runFailure(ctx); err != nil {
		t.Fatalf("running: %v", err)
	}
	cancel(nil)
	if err := runFailure(ctx); err != nil {
		t.Fatalf("after a signal: %v", err)
	}
}

// M10: flow data needs a credential; the probes and the scrape do not.
func TestAPIHandler_RequiresAuth(t *testing.T) {
	mux := http.NewServeMux()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, p := range []string{"/health", "/ready", "/metrics", "/api/v1/traces", "/api/v1/stats"} {
		mux.Handle(p, ok)
	}
	f := authtest.New(t, "trace-collector", "unheaded-cli")
	h := apiHandler(mux, f.Config("trace-collector", "unheaded-cli"))

	for _, p := range []string{"/health", "/ready", "/metrics"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusOK {
			t.Errorf("GET %s anonymous: %d, want 200", p, c)
		}
	}
	for _, p := range []string{"/api/v1/traces", "/api/v1/stats"} {
		if c := authtest.Code(h, http.MethodGet, p, "", ""); c != http.StatusUnauthorized {
			t.Errorf("GET %s anonymous: %d, want 401", p, c)
		}
		if c := authtest.Code(h, http.MethodGet, p, "", f.Bearer("unheaded-cli", "trace-collector")); c != http.StatusOK {
			t.Errorf("GET %s with a token: %d, want 200", p, c)
		}
	}

	// A host with no keys yet: the collector keeps running and the flow
	// data stays closed, not open.
	bad := f.Config("trace-collector", "unheaded-cli")
	bad.TrustDir = ""
	h = apiHandler(mux, bad)
	if c := authtest.Code(h, http.MethodGet, "/api/v1/traces", "", ""); c != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/traces with auth on and nothing to verify against: %d, want 401", c)
	}
	if c := authtest.Code(h, http.MethodGet, "/metrics", "", ""); c != http.StatusOK {
		t.Errorf("GET /metrics with auth on and nothing to verify against: %d, want 200", c)
	}
}

// M10: the shipped unit binds loopback unless the host's env file names its
// internal address, and turns auth on.
func TestUnit_InternalBindAndAuth(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/systemd/unheaded-trace-collector.service")
	if err != nil {
		t.Fatal(err)
	}
	unit := string(raw)
	for _, want := range []string{
		"\nEnvironment=AUTH_ENABLED=true\n",
		"\nEnvironment=TRACE_METRICS_ADDR=127.0.0.1:16671\n",
		"--metrics-addr ${TRACE_METRICS_ADDR}",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit lacks %q", want)
		}
	}
}
