// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"unheaded/services/wotan/internal/api"
)

// S-TRUE-WIRE Step 155: each SIGHUP on the channel reloads the topic config;
// the loop ends with its context.
func TestReloadTopicConfigOnHUP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wotan.yaml")
	if err := os.WriteFile(path, []byte("topics:\n  auto_approve: [a]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := api.LoadTopicConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	hup := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() { reloadTopicConfigOnHUP(ctx, hup, cfg, path); close(done) }()

	if err := os.WriteFile(path, []byte("topics:\n  auto_approve: [b]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok0, _ := api.TopicConfigReloadCounts()
	hup <- syscall.SIGHUP
	deadline := time.Now().Add(5 * time.Second)
	for !cfg.IsAutoApproved(nil, "b") {
		if time.Now().After(deadline) {
			t.Fatal("SIGHUP did not reload the topic config")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if ok, _ := api.TopicConfigReloadCounts(); ok != ok0+1 {
		t.Fatalf("reloads %v, want %v", ok, ok0+1)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reload loop did not stop with its context")
	}
}
