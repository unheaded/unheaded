// SPDX-License-Identifier: GPL-3.0-or-later

package httputil

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"net/netip"
	"time"
)

// RegisterPprof mounts net/http/pprof (index, cmdline, profile, symbol,
// trace and the named profiles under the index) on mux. Only ever call it
// on a mux that StartPprof serves on loopback: heap and goroutine dumps can
// hold service tokens and keys (S-LOW-FRUIT round table, MoatGhost).
func RegisterPprof(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

// CheckLoopback returns an error unless addr is host:port with a loopback
// IP literal or "localhost". An empty host (":6060") listens on every
// interface and is refused.
func CheckLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("pprof address %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip, err := netip.ParseAddr(host); err == nil && ip.Unmap().IsLoopback() {
		return nil
	}
	return fmt.Errorf("pprof address %q is not loopback (use 127.0.0.1:PORT or [::1]:PORT)", addr)
}

// StartPprof serves pprof on its own listener at addr, which must be
// loopback (CheckLoopback). It returns once the listener is bound, so a
// port clash is reported to the caller instead of being logged later; the
// server then runs until the process exits. Off unless a binary's --pprof
// flag names an address.
func StartPprof(addr string) error {
	if err := CheckLoopback(addr); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("pprof listener: %w", err)
	}
	mux := http.NewServeMux()
	RegisterPprof(mux)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("pprof listener on %s exited: %v", addr, err)
		}
	}()
	return nil
}
