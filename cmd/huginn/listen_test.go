// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// L11: huginn has no auth, so unconfigured it binds loopback.
func TestDefaultListenIsLoopback(t *testing.T) {
	if got := defaultConfig().Listen; got != "127.0.0.1:9110" {
		t.Fatalf("default listen = %q", got)
	}
}

// L11: its callers sit on several internal networks, so it serves a list of
// addresses instead of every interface.
func TestServeAll_ServesEveryAddress(t *testing.T) {
	free := func() string {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = ln.Close() }()
		return ln.Addr().String()
	}
	a, b := free(), free()
	srv := &http.Server{
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") }),
		ReadHeaderTimeout: time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- serveAll(srv, a+","+b) }()
	defer func() {
		_ = srv.Close()
		<-done
	}()
	client := &http.Client{Timeout: time.Second}
	for _, addr := range []string{a, b} {
		var resp *http.Response
		var err error
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if resp, err = client.Get("http://" + addr + "/"); err == nil {
				break
			}
		}
		if err != nil {
			t.Fatalf("GET %s: %v", addr, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if string(body) != "ok" {
			t.Errorf("GET %s = %q", addr, body)
		}
	}
}

func TestServeAll_RefusesAnAddressItCannotBind(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	srv := &http.Server{ReadHeaderTimeout: time.Second}
	if err := serveAll(srv, taken.Addr().String()); err == nil {
		t.Fatal("serveAll succeeded on a taken port")
	}
}

// L11: the unit reads the list from the host's env file, loopback without
// one, and the dashboard's health check uses the address its host cards
// already use, which a host can list.
func TestDeployment_BindAndCallers(t *testing.T) {
	read := func(p string) string {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	unit := read("../../deploy/systemd/huginn.service")
	for _, want := range []string{"\nEnvironment=HUGINN_LISTEN=127.0.0.1:9110\n", "-listen ${HUGINN_LISTEN}"} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit lacks %q", want)
		}
	}
	if !strings.Contains(read("../../deploy/systemd/huginn.env.example"), "HUGINN_LISTEN=") {
		t.Error("huginn.env.example does not document HUGINN_LISTEN")
	}
	if !strings.Contains(read("../../config/dashboard/services.txt"), "\nhuginn=192.168.13.2:9110\n") {
		t.Error("dashboard services.txt does not reach huginn on 192.168.13.2")
	}
}
