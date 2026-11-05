// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package httputil

import (
	"net"
	"reflect"
	"testing"
)

func TestListenAddrs(t *testing.T) {
	got := ListenAddrs(" 127.0.0.1:1, [::1]:2 ,,")
	if want := []string{"127.0.0.1:1", "[::1]:2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ListenAddrs = %q, want %q", got, want)
	}
	if got := ListenAddrs(""); got != nil {
		t.Fatalf("ListenAddrs(\"\") = %q, want none", got)
	}
}

func TestListenAll_OpensEveryAddress(t *testing.T) {
	lns, err := ListenAll([]string{"127.0.0.1:0", "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, ln := range lns {
			_ = ln.Close()
		}
	}()
	if len(lns) != 2 {
		t.Fatalf("%d listeners, want 2", len(lns))
	}
}

func TestListenAll_AllOrNone(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	freeAddr := free.Addr().String()
	_ = free.Close()

	if _, err := ListenAll([]string{freeAddr, taken.Addr().String()}); err == nil {
		t.Fatal("ListenAll succeeded with one address taken")
	}
	again, err := net.Listen("tcp", freeAddr)
	if err != nil {
		t.Fatalf("the first address stayed open after the failure: %v", err)
	}
	_ = again.Close()
}

func TestListenAll_RefusesEmptyList(t *testing.T) {
	if _, err := ListenAll(nil); err == nil {
		t.Fatal("ListenAll(nil) succeeded")
	}
}
