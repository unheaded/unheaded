// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package lifecycle

import (
	"context"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// newTestServer creates an http.Server listening on a random port.
// Returns the server and a function to confirm it is serving.
func newTestServer(t *testing.T) (*http.Server, string) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			t.Errorf("serve: %v", err)
		}
	}()

	// Wait until the server is accepting connections.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return srv, addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server did not start within 2s")
	return nil, ""
}

func TestWaitForShutdown_SIGINT(t *testing.T) {
	srv, addr := newTestServer(t)

	// Verify server is alive before shutdown.
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("health check before shutdown: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /health, got %d", resp.StatusCode)
	}

	var cleanupCalled atomic.Int32
	done := make(chan struct{})

	go func() {
		WaitForShutdown(srv, 5*time.Second, func() {
			cleanupCalled.Add(1)
		})
		close(done)
	}()

	// Give WaitForShutdown time to register the signal handler.
	time.Sleep(50 * time.Millisecond)

	// Send SIGINT to this process.
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal SIGINT: %v", err)
	}

	select {
	case <-done:
		// success
	case <-time.After(10 * time.Second):
		t.Fatal("WaitForShutdown did not return within 10s")
	}

	if got := cleanupCalled.Load(); got != 1 {
		t.Errorf("expected cleanup called 1 time, got %d", got)
	}

	// Server should no longer accept connections.
	_, err = http.Get("http://" + addr + "/health")
	if err == nil {
		t.Error("expected error connecting to shut-down server")
	}
}

func TestWaitForShutdown_SIGTERM(t *testing.T) {
	srv, _ := newTestServer(t)

	done := make(chan struct{})
	go func() {
		WaitForShutdown(srv, 5*time.Second)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Signal SIGTERM: %v", err)
	}

	select {
	case <-done:
		// success
	case <-time.After(10 * time.Second):
		t.Fatal("WaitForShutdown did not return within 10s after SIGTERM")
	}
}

func TestWaitForShutdown_MultipleCleanupFuncs(t *testing.T) {
	srv, _ := newTestServer(t)

	var mu sync.Mutex
	var order []int

	done := make(chan struct{})
	go func() {
		WaitForShutdown(srv, 5*time.Second,
			func() {
				mu.Lock()
				order = append(order, 1)
				mu.Unlock()
			},
			func() {
				mu.Lock()
				order = append(order, 2)
				mu.Unlock()
			},
			func() {
				mu.Lock()
				order = append(order, 3)
				mu.Unlock()
			},
		)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("WaitForShutdown did not return within 10s")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(order) != 3 {
		t.Fatalf("expected 3 cleanup calls, got %d", len(order))
	}
	// Verify cleanup functions run in order.
	for i, v := range order {
		if v != i+1 {
			t.Errorf("cleanup order[%d] = %d, want %d", i, v, i+1)
		}
	}
}

func TestWaitForShutdown_NoCleanup(t *testing.T) {
	srv, _ := newTestServer(t)

	done := make(chan struct{})
	go func() {
		WaitForShutdown(srv, 5*time.Second)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("WaitForShutdown did not return within 10s")
	}
}

func TestWaitForShutdown_TimeoutExceeded(t *testing.T) {
	// Create a server with a handler that blocks longer than the shutdown timeout.
	mux := http.NewServeMux()
	holdOpen := make(chan struct{})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		<-holdOpen // block until test releases
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			t.Errorf("serve: %v", err)
		}
	}()

	// Wait for server to be ready.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Start a long-running request to keep a connection active during shutdown.
	go func() {
		client := &http.Client{Timeout: 0}
		//nolint:errcheck
		client.Get("http://" + addr + "/slow")
	}()
	// Give the request time to reach the handler.
	time.Sleep(100 * time.Millisecond)

	var cleanupRan atomic.Int32
	done := make(chan struct{})
	go func() {
		// Use a very short timeout so Shutdown returns quickly with an error.
		WaitForShutdown(srv, 100*time.Millisecond, func() {
			cleanupRan.Add(1)
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("WaitForShutdown did not return within 10s")
	}

	// Cleanup should still run even when shutdown times out.
	if cleanupRan.Load() != 1 {
		t.Error("cleanup should run even after timeout")
	}

	// Release the blocked handler.
	close(holdOpen)
}

func TestWaitForShutdown_AlreadyShutdownServer(t *testing.T) {
	srv, _ := newTestServer(t)

	// Shut the server down before WaitForShutdown runs.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("pre-shutdown: %v", err)
	}

	var cleanupRan atomic.Int32
	done := make(chan struct{})
	go func() {
		WaitForShutdown(srv, 5*time.Second, func() {
			cleanupRan.Add(1)
		})
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := p.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Signal: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("WaitForShutdown did not return within 10s")
	}

	// Cleanup runs regardless of server state.
	if cleanupRan.Load() != 1 {
		t.Error("cleanup should run even when server was already shut down")
	}
}
