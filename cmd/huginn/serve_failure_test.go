// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// lateAddr is an address no host holds (TEST-NET-1), so it starts out pending.
const lateAddr = "192.0.2.1:9110"

func skipIfBindable(t *testing.T) {
	t.Helper()
	if ln, err := net.Listen("tcp", lateAddr); err == nil {
		_ = ln.Close()
		t.Skip("this host binds addresses it does not hold (ip_nonlocal_bind)")
	}
}

// swapBind stands in for the host: bind answers every retry.
func swapBind(t *testing.T, bind func(string) (net.Listener, error)) {
	t.Helper()
	savedBind, savedRetry := bindAddr, listenRetry
	t.Cleanup(func() { bindAddr, listenRetry = savedBind, savedRetry })
	bindAddr, listenRetry = bind, time.Millisecond
}

// serveResult runs serveAll and waits for its answer.
func serveResult(t *testing.T, list string) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- serveAll(&http.Server{ReadHeaderTimeout: time.Second}, list) }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("serveAll did not end")
		return nil
	}
}

// An address that turns out taken once the host holds it ends the run, as it
// would have stopped the start.
func TestServeAll_LateFailureEndsTheRun(t *testing.T) {
	skipIfBindable(t)
	swapBind(t, func(string) (net.Listener, error) { return nil, syscall.EADDRINUSE })
	if err := serveResult(t, freeAddr(t)+","+lateAddr); !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("serveAll = %v, want EADDRINUSE", err)
	}
}

// A listener that dies under Serve ends the run so the unit restarts huginn.
// Left alone, the process lives on with that address dead.
func TestServeAll_DeadListenerEndsTheRun(t *testing.T) {
	skipIfBindable(t)
	bound := make(chan net.Listener, 1)
	swapBind(t, func(string) (net.Listener, error) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			bound <- ln
		}
		return ln, err
	})
	list := freeAddr(t) + "," + lateAddr
	done := make(chan error, 1)
	go func() { done <- serveAll(&http.Server{ReadHeaderTimeout: time.Second}, list) }()
	ln := <-bound
	time.Sleep(50 * time.Millisecond) // let Serve reach Accept
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("serveAll = %v, want the listener's failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a dead listener did not end the run")
	}
}

// runEnding names what the stand-in host does in the child of TestMainEnding.
const runEnding = "HUGINN_TEST_RUN"

// main ends with the status the run earned: a listener failure is a non-zero
// exit, for the unit to restart it, and SIGTERM is left to end the process, which
// systemd counts as clean. The child is this test binary running main with the
// bind swapped out and every sink off, so it pushes nothing anywhere.
func TestMainEnding(t *testing.T) {
	if mode := os.Getenv(runEnding); mode != "" {
		listenRetry = time.Millisecond
		bindAddr = func(string) (net.Listener, error) {
			switch mode {
			case "late-failure":
				return nil, syscall.EADDRINUSE
			case "dead-listener":
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err == nil {
					time.AfterFunc(50*time.Millisecond, func() { _ = ln.Close() })
				}
				return ln, err
			}
			return nil, &net.OpError{Op: "listen", Err: syscall.EADDRNOTAVAIL}
		}
		os.Args = []string{"huginn", "--config", os.Getenv(runEnding + "_CONFIG"), "--listen", lateAddr}
		main()
		return
	}
	skipIfBindable(t)
	config := filepath.Join(t.TempDir(), "huginn.yaml")
	if err := os.WriteFile(config, []byte("sinks:\n  victoria_metrics:\n    enabled: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A child that never ends is a failure too, not a hung test.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	child := func(mode string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMainEnding$")
		cmd.Env = append(os.Environ(), runEnding+"="+mode, runEnding+"_CONFIG="+config)
		return cmd
	}
	for _, mode := range []string{"late-failure", "dead-listener"} {
		t.Run(mode, func(t *testing.T) {
			out, err := child(mode).CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				t.Fatalf("exit = %v, want status 1:\n%s", err, out)
			}
			if !strings.Contains(string(out), "huginn: listen:") {
				t.Fatalf("no listener failure reported:\n%s", out)
			}
		})
	}
	t.Run("signal", func(t *testing.T) {
		var out strings.Builder
		cmd := child("waiting")
		cmd.Stderr = &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(10 * time.Second); !strings.Contains(out.String(), "is not on this host yet"); time.Sleep(20 * time.Millisecond) {
			if time.Now().After(deadline) {
				_ = cmd.Process.Kill()
				t.Fatalf("child never reached serveAll:\n%s", out.String())
			}
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		err := cmd.Wait()
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("child ended with %v, want death by SIGTERM", err)
		}
		if ws, ok := exit.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
			t.Fatalf("child ended with %v, want death by SIGTERM", err)
		}
	})
}
