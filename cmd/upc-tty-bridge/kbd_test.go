// SPDX-License-Identifier: GPL-3.0-or-later
//
// S5 Phase 4: keystroke queue + loopback-only drain endpoint.

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestKbdQueue_PushDrainInOrder(t *testing.T) {
	t.Parallel()
	q := newKbdQueue(4096)
	q.push(111, []byte("echo "))
	q.push(111, []byte("hello\n"))
	if got := string(q.drain(111, 64)); got != "echo hello\n" {
		t.Fatalf("drain = %q", got)
	}
	if got := q.drain(111, 64); len(got) != 0 {
		t.Fatalf("second drain should be empty, got %q", got)
	}
}

func TestKbdQueue_DrainHonorsMax(t *testing.T) {
	t.Parallel()
	q := newKbdQueue(4096)
	q.push(1, bytes.Repeat([]byte{'a'}, 200))
	if got := len(q.drain(1, 64)); got != 64 {
		t.Fatalf("first drain = %d bytes, want 64", got)
	}
	if got := len(q.drain(1, 1000)); got != 136 {
		t.Fatalf("second drain = %d bytes, want 136", got)
	}
}

func TestKbdQueue_PerInstance(t *testing.T) {
	t.Parallel()
	q := newKbdQueue(4096)
	q.push(1, []byte("a"))
	q.push(2, []byte("b"))
	if got := string(q.drain(2, 10)); got != "b" {
		t.Fatalf("instance 2 drain = %q", got)
	}
	if got := string(q.drain(1, 10)); got != "a" {
		t.Fatalf("instance 1 drain = %q", got)
	}
}

func TestKbdQueue_OverflowDropsNewestAndCounts(t *testing.T) {
	t.Parallel()
	q := newKbdQueue(8)
	q.push(1, []byte("12345"))
	q.push(1, []byte("6789AB")) // 3 fit, 3 dropped
	if got := string(q.drain(1, 100)); got != "12345678" {
		t.Fatalf("drain = %q, want the oldest 8 kept", got)
	}
	if got := q.dropped.Value(); got != 3 {
		t.Fatalf("dropped = %v, want 3", got)
	}
}

func TestKbdQueue_DrainZeroOrNegativeMax(t *testing.T) {
	t.Parallel()
	q := newKbdQueue(16)
	q.push(1, []byte("ab"))
	if got := q.drain(1, 0); len(got) != 0 {
		t.Fatalf("drain(0) = %q", got)
	}
	if got := q.drain(1, -1); len(got) != 0 {
		t.Fatalf("drain(-1) = %q", got)
	}
	if got := string(q.drain(1, 2)); got != "ab" {
		t.Fatalf("queue lost bytes: %q", got)
	}
}

func TestKbdQueue_ConcurrentPushDrain(t *testing.T) {
	t.Parallel()
	q := newKbdQueue(1 << 20)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				q.push(7, []byte("x"))
				_ = q.drain(7, 3)
			}
		}()
	}
	wg.Wait()
}

func drainReq(t *testing.T, h http.Handler, method, target, remote string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = remote
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestHandleKbdDrain_ReturnsJSONNumbers(t *testing.T) {
	t.Parallel()
	hub := newHub()
	hub.kbd.push(111, []byte("ls\n"))
	h := http.HandlerFunc(hub.handleKbdDrain)
	w := drainReq(t, h, http.MethodGet, "/api/v1/kbd/drain?instance=111&max=2", "127.0.0.1:5555")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "[108,115]" {
		t.Fatalf("body = %s, want [108,115]", got)
	}
	w = drainReq(t, h, http.MethodGet, "/api/v1/kbd/drain?instance=111&max=64", "[::1]:5555")
	if got := strings.TrimSpace(w.Body.String()); got != "[10]" {
		t.Fatalf("body = %s, want [10]", got)
	}
	w = drainReq(t, h, http.MethodGet, "/api/v1/kbd/drain?instance=111", "127.0.0.1:5555")
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Fatalf("empty drain body = %s, want []", got)
	}
}

func TestHandleKbdDrain_RejectsNonGet(t *testing.T) {
	t.Parallel()
	hub := newHub()
	w := drainReq(t, http.HandlerFunc(hub.handleKbdDrain), http.MethodPost,
		"/api/v1/kbd/drain?instance=1", "127.0.0.1:5555")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

func TestHandleKbdDrain_LoopbackOnly(t *testing.T) {
	t.Parallel()
	hub := newHub()
	hub.kbd.push(1, []byte("secret"))
	for _, remote := range []string{"192.168.13.1:4000", "10.10.10.5:80", "[fd00::1]:9", "garbage", ""} {
		w := drainReq(t, http.HandlerFunc(hub.handleKbdDrain), http.MethodGet,
			"/api/v1/kbd/drain?instance=1", remote)
		if w.Code != http.StatusForbidden {
			t.Fatalf("remote %q: status = %d, want 403", remote, w.Code)
		}
	}
	if got := string(hub.kbd.drain(1, 64)); got != "secret" {
		t.Fatalf("queue = %q after refused drains", got)
	}
}

func TestParseDrainMax(t *testing.T) {
	t.Parallel()
	cases := map[string]int{"": 64, "abc": 64, "0": 64, "-5": 64, "1": 1, "64": 64, "999999": 4096}
	for raw, want := range cases {
		if got := parseDrainMax(raw); got != want {
			t.Fatalf("parseDrainMax(%q) = %d, want %d", raw, got, want)
		}
	}
}

func TestIsLoopbackRemote(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"127.0.0.1:1": true, "[::1]:2": true, "127.8.9.10:3": true,
		"192.168.1.1:4": false, "[fe80::1]:5": false, "": false, "nonsense": false,
	}
	for in, want := range cases {
		if got := isLoopbackRemote(in); got != want {
			t.Fatalf("isLoopbackRemote(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestConsoleKeystrokesReachQueue(t *testing.T) {
	t.Parallel()
	hub := newHub()
	srv := httptest.NewServer(http.HandlerFunc(hub.handleWs))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/console?instance=111"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("echo hi\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var got []byte
	for time.Now().Before(deadline) {
		got = append(got, hub.kbd.drain(111, 64)...)
		if string(got) == "echo hi\n" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("queue got %q, want %q", got, "echo hi\n")
}
