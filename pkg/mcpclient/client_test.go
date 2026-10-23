// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"unheaded/pkg/zhentools"
)

func fakeConfig(t *testing.T, mode string) Config {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Config{
		Command: exe,
		// The race runtime sleeps 1s at exit by default; the fake need not.
		Env:           append(os.Environ(), fakeServerEnv+"="+mode, "GORACE=atexit_sleep_ms=0"),
		InitTimeout:   5 * time.Second,
		CallTimeout:   5 * time.Second,
		ShutdownGrace: 500 * time.Millisecond,
	}
}

func startFake(t *testing.T, mode string, tweak ...func(*Config)) *Client {
	t.Helper()
	cfg := fakeConfig(t, mode)
	for _, f := range tweak {
		f(&cfg)
	}
	c, err := Start(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Start(%s): %v", mode, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func call(t *testing.T, c *Client, name, args string) zhentools.Result {
	t.Helper()
	res, err := c.Call(context.Background(), name, json.RawMessage(args))
	if err != nil {
		t.Fatalf("Call(%s): %v", name, err)
	}
	return res
}

func noControl(t *testing.T, what, s string) {
	t.Helper()
	if SanitizeText(s) != s {
		t.Fatalf("%s carries unsanitized characters: %q", what, s)
	}
}

func TestHandshake(t *testing.T) {
	c := startFake(t, "ok")
	d := c.Diagnostics()
	if d.ProtocolVersion != LatestProtocolVersion {
		t.Fatalf("protocol %q", d.ProtocolVersion)
	}
	if !strings.HasPrefix(d.ServerName, "fake") || d.ServerVersion != "1.0" {
		t.Fatalf("server info %q %q", d.ServerName, d.ServerVersion)
	}
	noControl(t, "server name", d.ServerName)
	noControl(t, "instructions", d.Instructions)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	// The server saw notifications/initialized before anything else.
	if !strings.Contains(c.Diagnostics().Stderr, "got initialize\nfake: got notifications/initialized\n") {
		t.Fatalf("handshake order: %q", c.Diagnostics().Stderr)
	}
}

func TestHandshakeOlderVersionAccepted(t *testing.T) {
	c := startFake(t, "old_version")
	if got := c.Diagnostics().ProtocolVersion; got != "2025-03-26" {
		t.Fatalf("negotiated %q", got)
	}
}

func TestHandshakeFailures(t *testing.T) {
	cases := []struct {
		mode    string
		want    error
		snippet string
	}{
		{"bad_version", ErrUnsupportedProtocol, "1999-01-01"},
		{"no_tools", ErrProtocol, "tools capability"},
		{"exit_early", ErrServerExited, "No module named"},
		{"silent", ErrTimeout, "initialize"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			cfg := fakeConfig(t, tc.mode)
			cfg.InitTimeout = 500 * time.Millisecond
			_, err := Start(context.Background(), cfg)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.snippet) {
				t.Fatalf("err %q lacks %q", err, tc.snippet)
			}
		})
	}
}

func TestStartBadConfig(t *testing.T) {
	if _, err := Start(context.Background(), Config{}); err == nil {
		t.Fatal("empty command accepted")
	}
	if _, err := Start(context.Background(), Config{Command: "x", ProtocolVersion: "2020-01-01"}); !errors.Is(err, ErrUnsupportedProtocol) {
		t.Fatalf("bad version: %v", err)
	}
	if _, err := Start(context.Background(), Config{Command: "/nonexistent/mcp-server"}); err == nil {
		t.Fatal("missing binary accepted")
	}
}

func TestToolsPaginationAndFiltering(t *testing.T) {
	c := startFake(t, "ok")
	tools, err := c.Tools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range tools {
		names = append(names, tl.Name)
		noControl(t, tl.Name+" description", tl.Description)
		noControl(t, tl.Name+" schema", string(tl.InputSchema))
		var schema map[string]any
		if err := json.Unmarshal(tl.InputSchema, &schema); err != nil {
			t.Fatalf("%s schema: %v", tl.Name, err)
		}
	}
	want := "echo fail slow crash binary hostile ping_me oversize malformed"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("tools = %q, want %q", got, want)
	}
	if string(tools[1].InputSchema) != `{"type":"object"}` {
		t.Fatalf("missing schema not defaulted: %s", tools[1].InputSchema)
	}
	if !strings.Contains(string(tools[0].InputSchema), `"what to echo`) {
		t.Fatalf("schema lost content: %s", tools[0].InputSchema)
	}
	skipped := strings.Join(c.Diagnostics().SkippedTools, "|")
	for _, s := range []string{`"bad name": invalid name`, `"echo": duplicate name`, `"arrayschema": input schema is not a JSON object`} {
		if !strings.Contains(skipped, s) {
			t.Fatalf("skipped %q lacks %q", skipped, s)
		}
	}
}

func TestToolsCursorLoop(t *testing.T) {
	c := startFake(t, "cursor_loop")
	if _, err := c.Tools(context.Background()); !errors.Is(err, ErrProtocol) {
		t.Fatalf("cursor loop: %v", err)
	}
}

func TestCallOK(t *testing.T) {
	c := startFake(t, "ok")
	res := call(t, c, "echo", `{"text":"hello kingdom"}`)
	if res.IsError || res.Text != "hello kingdom" {
		t.Fatalf("echo = %+v", res)
	}
	// nil input becomes {}.
	if res, err := c.Call(context.Background(), "echo", nil); err != nil || res.IsError {
		t.Fatalf("nil args: %+v %v", res, err)
	}
}

func TestCallToolErrors(t *testing.T) {
	c := startFake(t, "ok")
	cases := []struct {
		name, args, want string
	}{
		{"fail", `{}`, "it broke"},
		{"no_such_tool", `{}`, "Unknown tool: no_such_tool"},
		{"bad name!", `{}`, "invalid tool name"},
		{"echo", `[1,2]`, "must be a JSON object"},
		{"echo", `{"text":`, "must be a JSON object"},
	}
	for _, tc := range cases {
		res := call(t, c, tc.name, tc.args)
		if !res.IsError || !strings.Contains(res.Text, tc.want) {
			t.Fatalf("%s %s = %+v, want IsError with %q", tc.name, tc.args, res, tc.want)
		}
	}
}

func TestCallNonTextContent(t *testing.T) {
	c := startFake(t, "ok")
	res := call(t, c, "binary", `{}`)
	for _, want := range []string{
		"header",
		"[image content omitted: image/png, 8 bytes base64]",
		"[resource link: doc file:///x]",
		"[resource file:///t]\ninline body",
		"[embedded resource omitted: file:///b application/zip, 4 bytes base64]",
		"[unsupported content type",
	} {
		if !strings.Contains(res.Text, want) {
			t.Fatalf("binary text %q lacks %q", res.Text, want)
		}
	}
	noControl(t, "binary result", res.Text)
}

func TestCallHostileTextSanitized(t *testing.T) {
	c := startFake(t, "ok")
	res := call(t, c, "hostile", `{}`)
	noControl(t, "hostile result", res.Text)
	if !strings.Contains(res.Text, "cleared\nline2") || strings.ContainsAny(res.Text, "\x1b\r\x00") {
		t.Fatalf("hostile text = %q", res.Text)
	}
}

func TestServerRequestsAnswered(t *testing.T) {
	c := startFake(t, "ok")
	res := call(t, c, "ping_me", `{}`)
	if res.IsError || res.Text != "pong result {}" {
		t.Fatalf("ping_me = %+v", res)
	}
}

func TestConcurrentCalls(t *testing.T) {
	c := startFake(t, "ok")
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			want := fmt.Sprintf("msg-%d", i)
			args := fmt.Sprintf(`{"text":%q,"ms":%d}`, want, (n-i)%7*5)
			res, err := c.Call(context.Background(), "echo", json.RawMessage(args))
			if err != nil {
				errs <- err
				return
			}
			if res.Text != want {
				errs <- fmt.Errorf("call %d got %q", i, res.Text)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestCancellation(t *testing.T) {
	c := startFake(t, "ok")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.Call(ctx, "slow", json.RawMessage(`{"ms":60000}`))
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not return")
	}
	waitStderr(t, c, "cancelled ", "cancelled by client")
	// The connection is still usable.
	if res := call(t, c, "echo", `{"text":"still here"}`); res.Text != "still here" {
		t.Fatalf("after cancel: %+v", res)
	}
}

func TestTimeout(t *testing.T) {
	c := startFake(t, "ok", func(cfg *Config) { cfg.CallTimeout = 300 * time.Millisecond })
	start := time.Now()
	_, err := c.Call(context.Background(), "slow", json.RawMessage(`{"ms":60000}`))
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("timeout took %v", el)
	}
	waitStderr(t, c, "client timeout")
	if res := call(t, c, "echo", `{"text":"ok"}`); res.Text != "ok" {
		t.Fatalf("after timeout: %+v", res)
	}
}

func waitStderr(t *testing.T, c *Client, wants ...string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s := c.Diagnostics().Stderr
		all := true
		for _, w := range wants {
			all = all && strings.Contains(s, w)
		}
		if all {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("stderr %q lacks %q", s, wants)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestServerCrashMidCall(t *testing.T) {
	c := startFake(t, "ok")
	// A slow call is in flight when another call crashes the server.
	slow := make(chan error, 1)
	go func() {
		_, err := c.Call(context.Background(), "slow", json.RawMessage(`{"ms":60000}`))
		slow <- err
	}()
	time.Sleep(100 * time.Millisecond)
	_, err := c.Call(context.Background(), "crash", json.RawMessage(`{}`))
	if !errors.Is(err, ErrServerExited) {
		t.Fatalf("crash call err = %v", err)
	}
	for _, want := range []string{"exit status 3", "segfault in tool crash"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("crash err %q lacks %q", err, want)
		}
	}
	select {
	case err := <-slow:
		if !errors.Is(err, ErrServerExited) {
			t.Fatalf("pending call err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending call not released by crash")
	}
	// Later calls fail fast with the same error.
	if _, err := c.Call(context.Background(), "echo", nil); !errors.Is(err, ErrServerExited) {
		t.Fatalf("post-crash call err = %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close after crash: %v", err)
	}
}

func TestOversizeLine(t *testing.T) {
	c := startFake(t, "ok", func(cfg *Config) { cfg.MaxMessageBytes = 16 << 10 })
	_, err := c.Call(context.Background(), "oversize", nil)
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("err = %v, want ErrMessageTooLarge", err)
	}
	if _, err := c.Call(context.Background(), "echo", nil); !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("connection should stay closed: %v", err)
	}
}

func TestMalformedLinesSkipped(t *testing.T) {
	c := startFake(t, "ok")
	res := call(t, c, "malformed", `{}`)
	if res.Text != "survived" {
		t.Fatalf("malformed = %+v", res)
	}
	d := c.Diagnostics()
	if d.MalformedMessages < 4 {
		t.Fatalf("malformed count %d (last %q)", d.MalformedMessages, d.LastMalformed)
	}
	noControl(t, "last malformed", d.LastMalformed)
}

func TestResultTruncated(t *testing.T) {
	c := startFake(t, "ok", func(cfg *Config) { cfg.MaxResultBytes = 10 })
	res := call(t, c, "echo", `{"text":"0123456789abcdefghij"}`)
	if !strings.HasPrefix(res.Text, "0123456789") || !strings.HasSuffix(res.Text, "[truncated by mcpclient]") {
		t.Fatalf("truncated = %q", res.Text)
	}
}

func TestCloseIdempotentAndFailsCalls(t *testing.T) {
	c := startFake(t, "ok")
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = c.Close() }()
	}
	wg.Wait()
	if _, err := c.Call(context.Background(), "echo", nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("after Close: %v", err)
	}
	if _, err := c.Tools(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Tools after Close: %v", err)
	}
}

func TestCloseKillsStubbornServer(t *testing.T) {
	cfg := fakeConfig(t, "ignore_shutdown")
	cfg.ShutdownGrace = 200 * time.Millisecond
	c, err := Start(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = c.Close()
	if err == nil || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("Close = %v, want killed", err)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("Close took %v", el)
	}
}

func TestTailBuffer(t *testing.T) {
	b := newTailBuffer(8)
	_, _ = b.Write([]byte("abc"))
	_, _ = b.Write([]byte("defgh"))
	_, _ = b.Write([]byte("ij"))
	s, dropped := b.snapshot()
	if s != "cdefghij" || dropped != 2 {
		t.Fatalf("tail = %q dropped %d", s, dropped)
	}
	_, _ = b.Write([]byte("0123456789"))
	if s, dropped = b.snapshot(); s != "23456789" || dropped != 12 {
		t.Fatalf("tail = %q dropped %d", s, dropped)
	}
}
