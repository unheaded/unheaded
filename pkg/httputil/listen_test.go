// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package httputil

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestReadJSON(t *testing.T) {
	read := func(body string) (int, map[string]string) {
		var v map[string]string
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		code, _ := ReadJSON(httptest.NewRecorder(), r, 16, &v)
		return code, v
	}
	if code, v := read(`{"a":"b"}`); code != 0 || v["a"] != "b" {
		t.Errorf("small body: code %d, value %v", code, v)
	}
	if code, _ := read(`{"a":`); code != http.StatusBadRequest {
		t.Errorf("bad JSON: code %d, want 400", code)
	}
	if code, _ := read(`{"a":"` + strings.Repeat("b", 64) + `"}`); code != http.StatusRequestEntityTooLarge {
		t.Errorf("body over the limit: code %d, want 413", code)
	}
}

// notOnHost is an address from TEST-NET-1, which no host holds.
const notOnHost = "192.0.2.1:9110"

func skipIfBindable(t *testing.T) {
	t.Helper()
	if ln, err := net.Listen("tcp", notOnHost); err == nil {
		_ = ln.Close()
		t.Skip("this host binds addresses it does not hold (ip_nonlocal_bind)")
	}
}

// L11: an address the host does not hold yet does not cost the rest.
func TestListenAvailable_AddressNotOnHostIsPending(t *testing.T) {
	skipIfBindable(t)
	lns, pending, err := ListenAvailable([]string{"127.0.0.1:0", notOnHost})
	if err != nil {
		t.Fatal(err)
	}
	for _, ln := range lns {
		_ = ln.Close()
	}
	if len(lns) != 1 || len(pending) != 1 || pending[0] != notOnHost {
		t.Fatalf("lns = %d, pending = %q", len(lns), pending)
	}
}

// L11: anything else is still a refusal, with nothing left open.
func TestListenAvailable_OtherFailuresRefuse(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	free, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := free.Addr().String()
	_ = free.Close()
	if _, _, err := ListenAvailable([]string{addr, taken.Addr().String()}); err == nil {
		t.Fatal("no error for a taken port")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("first address left open: %v", err)
	}
	_ = ln.Close()
	if _, _, err := ListenAvailable(nil); err == nil {
		t.Fatal("no error for an empty list")
	}
	if _, _, err := ListenAvailable([]string{"not an address"}); err == nil {
		t.Fatal("no error for a malformed address")
	}
}

// L11: a pending address is bound once the host holds it.
func TestAwaitAddr_BindsOnceTheAddressAppears(t *testing.T) {
	tries := 0
	bind := func(string) (net.Listener, error) {
		if tries++; tries < 3 {
			return nil, &net.OpError{Op: "listen", Err: syscall.EADDRNOTAVAIL}
		}
		return ListenTCP("127.0.0.1:0")
	}
	ln, err := AwaitAddr("late", time.Millisecond, bind, make(chan struct{}))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	if tries != 3 {
		t.Fatalf("tries = %d, want 3", tries)
	}
}

// L11: an address that turns out unusable for any other reason ends the wait
// with that error, as it would have at the start.
func TestAwaitAddr_OtherFailureIsReturned(t *testing.T) {
	bind := func(string) (net.Listener, error) { return nil, syscall.EADDRINUSE }
	if _, err := AwaitAddr("127.0.0.1:1", time.Millisecond, bind, make(chan struct{})); !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("err = %v, want EADDRINUSE", err)
	}
}

func TestAwaitAddr_DoneEndsTheWait(t *testing.T) {
	done := make(chan struct{})
	close(done)
	bind := func(string) (net.Listener, error) { return nil, syscall.EADDRNOTAVAIL }
	if ln, err := AwaitAddr("late", time.Hour, bind, done); ln != nil || err != nil {
		t.Fatalf("AwaitAddr = %v, %v, want nil, nil", ln, err)
	}
}
