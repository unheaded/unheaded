// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package httputil

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
)

// ListenAddrs splits a comma-separated listen list ("127.0.0.1:9110,
// [fd00::1]:9110") into its addresses. A service reached from several
// internal networks names each of them, rather than every interface.
func ListenAddrs(list string) []string {
	var out []string
	for _, a := range strings.Split(list, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// ListenAll opens a TCP listener on every address, or none: on an error the
// ones already open are closed, so a service never runs on part of its list.
func ListenAll(addrs []string) ([]net.Listener, error) {
	if len(addrs) == 0 {
		return nil, errors.New("no listen address")
	}
	lns := make([]net.Listener, 0, len(addrs))
	for _, a := range addrs {
		ln, err := net.Listen("tcp", a)
		if err != nil {
			for _, open := range lns {
				_ = open.Close()
			}
			return nil, fmt.Errorf("listen %s: %w", a, err)
		}
		lns = append(lns, ln)
	}
	return lns, nil
}

// ListenAvailable is ListenAll for a service that must run while part of its
// list is not on the host yet, an interface still coming up: an address the
// host does not hold comes back in pending, not as an error. Any other failure
// (a taken port, a malformed address) still closes everything and is returned.
func ListenAvailable(addrs []string) (lns []net.Listener, pending []string, err error) {
	if len(addrs) == 0 {
		return nil, nil, errors.New("no listen address")
	}
	for _, a := range addrs {
		ln, err := net.Listen("tcp", a)
		switch {
		case err == nil:
			lns = append(lns, ln)
		case AddrNotOnHost(err):
			pending = append(pending, a)
		default:
			for _, open := range lns {
				_ = open.Close()
			}
			return nil, nil, fmt.Errorf("listen %s: %w", a, err)
		}
	}
	return lns, pending, nil
}

// AddrNotOnHost reports whether a listen failed because the address is not
// assigned to this host, or not yet usable.
func AddrNotOnHost(err error) bool {
	return errors.Is(err, syscall.EADDRNOTAVAIL)
}

// ReadJSON decodes r's JSON body into v, reading at most limit bytes. On
// failure it returns the status to answer with (413 over the limit, 400 for
// anything else) and the error; on success, 0 and nil.
func ReadJSON(w http.ResponseWriter, r *http.Request, limit int64, v any) (int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusBadRequest, err
	}
	return 0, nil
}
