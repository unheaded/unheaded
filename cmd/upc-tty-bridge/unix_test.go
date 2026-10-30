// SPDX-License-Identifier: GPL-3.0-or-later
//
// --listen-unix (2026-10-30): a boot inside netns upc0 reaches the bridge
// through a socket file, since the host's 127.0.0.1 is not its loopback.

package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func unixClient(path string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		}},
	}
}

func TestListenUnix_ModeStaleAndRefusals(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bridge.sock")

	l, err := listenUnix(p)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSocket == 0 || fi.Mode().Perm() != 0o660 {
		t.Fatalf("socket mode %v, want socket 0660", fi.Mode())
	}
	// A live socket is another bridge's: refuse, do not unlink it.
	if _, err := listenUnix(p); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("second listen on a live socket: %v", err)
	}
	// A stale socket (its listener gone, file left behind) is replaced.
	l.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = l.Close()
	l2, err := listenUnix(p)
	if err != nil {
		t.Fatalf("stale socket not replaced: %v", err)
	}
	_ = l2.Close()

	// Anything else at the path is never removed.
	f := filepath.Join(dir, "file")
	if err := os.WriteFile(f, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenUnix(f); err == nil {
		t.Fatal("regular file at the socket path accepted")
	}
	if b, _ := os.ReadFile(f); string(b) != "keep" {
		t.Fatal("regular file was clobbered")
	}
}

func TestUnixMux_IngestDrainHealthOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bridge.sock")
	l, err := listenUnix(p)
	if err != nil {
		t.Fatal(err)
	}
	hub := newHub()
	hub.kbd.push(111, []byte("ls\n"))
	srv := &http.Server{Handler: unixMux(hub), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	c := unixClient(p)

	resp, err := c.Post("http://bridge/api/v1/tty/ingest", "application/json", strings.NewReader(`{"instance":111,"bytes":[104,105]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("ingest status %d", resp.StatusCode)
	}

	// The drain handler's loopback-only rule admits the socket (its file mode
	// is the access control there).
	resp, err = c.Get("http://bridge/api/v1/kbd/drain?instance=111&max=64")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(buf[:n])) != "[108,115,10]" {
		t.Fatalf("drain %d %q", resp.StatusCode, buf[:n])
	}

	for _, path := range []string{"/console", "/metrics", "/"} {
		resp, err := c.Get("http://bridge" + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s over the socket: status %d, want 404", path, resp.StatusCode)
		}
	}
}

// Marking is per listener: a TCP request with a non-loopback address is
// still refused by drain.
func TestFromUnixSocket_OnlyWhenMarked(t *testing.T) {
	hub := newHub()
	w := drainReq(t, http.HandlerFunc(hub.handleKbdDrain), http.MethodGet, "/api/v1/kbd/drain?instance=1", "192.0.2.1:5555")
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-loopback TCP drain status %d", w.Code)
	}
}
