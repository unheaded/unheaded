// SPDX-License-Identifier: GPL-3.0-or-later
//
// unix.go — --listen-unix (2026-10-30). upc-bootctl boots inside netns upc0,
// whose 127.0.0.1 is not the host's, so it cannot reach the TCP listener.
// A socket file can be reached from any network namespace that sees the
// filesystem. The socket serves only what bootctl calls (health, TTY
// ingest, keyboard drain); its file mode (0660) and directory decide who may
// connect, so the drain handler's loopback rule admits it.

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

type unixSocketKey struct{}

// fromUnixSocket reports whether r arrived on the --listen-unix socket.
func fromUnixSocket(r *http.Request) bool {
	v, _ := r.Context().Value(unixSocketKey{}).(bool)
	return v
}

// unixMux is the socket's handler: health, ingest and drain, each request
// marked as arriving on the socket.
func unixMux(h *Hub) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/api/v1/tty/ingest", h.handleTtyIngest)
	mux.HandleFunc("/api/v1/kbd/drain", h.handleKbdDrain)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), unixSocketKey{}, true)))
	})
}

// listenUnix listens on path with mode 0660. A socket left by a previous run
// is replaced; a socket something still answers on, or any other file, is
// refused and left alone.
func listenUnix(path string) (net.Listener, error) {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
			_ = c.Close()
			return nil, fmt.Errorf("%s is in use by another listener", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o660); err != nil { // #nosec G302 -- a socket: group rw is how bootctl's account connects
		_ = l.Close()
		return nil, err
	}
	return l, nil
}
