// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package api

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"unheaded/pkg/auth"
)

func rewrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// S-TRUE-WIRE Step 154: Reload swaps the allowlists in place, counts
// successes and failures, and a refused reload keeps the old config whole.
func TestTopicConfigReload(t *testing.T) {
	path := writeTopicConfig(t, "topics:\n  auto_approve: [alpha, beta]\n  legacy_display_names: [old-name]\n")
	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	ok0, fail0 := TopicConfigReloadCounts()

	rewrite(t, path, "topics:\n  auto_approve: [beta, gamma, delta]\n")
	res, err := cfg.Reload(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if res.Added != 2 || res.Removed != 2 { // +gamma +delta, -alpha -old-name
		t.Fatalf("diff %+v, want 2 added 2 removed", res)
	}
	for name, want := range map[string]bool{"alpha": false, "beta": true, "gamma": true, "delta": true, "old-name": false} {
		if got := cfg.IsAutoApproved(nil, name); got != want {
			t.Errorf("%s approved = %v after reload, want %v", name, got, want)
		}
	}
	eddsa := &auth.Identity{Subject: "gamma", Extra: map[string]string{"auth_method": "eddsa"}}
	if !cfg.IsAutoApproved(eddsa, "") {
		t.Error("service identity gamma not approved after reload")
	}
	if ok, fail := TopicConfigReloadCounts(); ok != ok0+1 || fail != fail0 {
		t.Fatalf("counts ok=%v fail=%v, want %v %v", ok, fail, ok0+1, fail0)
	}

	// Each refused reload keeps beta/gamma/delta and counts one failure.
	k, _ := testPubKeyB64(t)
	for name, body := range map[string]string{
		"malformed":         ": :\n",
		"bad signed prefix": "topics:\n  auto_approve: [omega]\n  signed_prefixes:\n    - prefx: \"audit.\"\n",
		"new signing key":   fmt.Sprintf("topics:\n  auto_approve: [omega]\n  signing_keys: [%q]\n", k),
		"new signed prefix": fmt.Sprintf("topics:\n  auto_approve: [omega]\n  signed_prefixes:\n    - prefix: \"audit.\"\n      keys: [%q]\n", k),
	} {
		rewrite(t, path, body)
		_, f0 := TopicConfigReloadCounts()
		if _, err := cfg.Reload(path); err == nil {
			t.Errorf("%s: reloaded", name)
		}
		if _, f := TopicConfigReloadCounts(); f != f0+1 {
			t.Errorf("%s: failures %v, want %v", name, f, f0+1)
		}
		if !cfg.IsAutoApproved(nil, "gamma") || cfg.IsAutoApproved(nil, "omega") {
			t.Errorf("%s: a refused reload changed the allowlist", name)
		}
	}

	// A file that vanished is a failure, not a silent deny-all.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Reload(path); err == nil {
		t.Fatal("missing file reloaded")
	}
	if !cfg.IsAutoApproved(nil, "gamma") {
		t.Fatal("a missing file emptied the allowlist")
	}
	if _, err := cfg.Reload(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("nonexistent path reloaded")
	}
}

// Readers and a reload race-free (run under -race).
func TestTopicConfigReloadConcurrent(t *testing.T) {
	path := writeTopicConfig(t, "topics:\n  auto_approve: [a]\n")
	cfg, err := LoadTopicConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					cfg.IsAutoApproved(nil, "a")
					cfg.RequiresSignature("logs.x")
				}
			}
		}()
	}
	for i := 0; i < 50; i++ {
		rewrite(t, path, fmt.Sprintf("topics:\n  auto_approve: [a, n%d]\n", i))
		if _, err := cfg.Reload(path); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
}
