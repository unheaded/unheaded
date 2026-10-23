// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package localllm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"unheaded/pkg/gpulease"
)

// S-TWO-BRAINS Step 235: each call holds a shared GPU lease, so a model
// swap or a training run (exclusive) never meets an inference call, and
// a call made while one runs fails fast with gpulease.ErrTimeout rather
// than hitting a half-loaded server.
func TestChat_HoldsSharedLease(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	var sawBusy atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// While the call is in flight an exclusive lease cannot be taken.
		if _, err := gpulease.Exclusive(context.Background(), lock, 50*time.Millisecond); errors.Is(err, gpulease.ErrTimeout) {
			sawBusy.Store(true)
		}
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"finish_reason":"stop","message":{"content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer s.Close()
	c := &Client{BaseURL: s.URL, GPULock: lock, LeaseTimeout: time.Second}
	a, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil || a.Text != "ok" {
		t.Fatalf("%+v %v", a, err)
	}
	if !sawBusy.Load() {
		t.Error("no shared lease was held during the call")
	}
	if gpulease.Busy(lock) {
		t.Error("the lease outlived the call")
	}
}

func TestChat_ExclusiveHolderMeansErrTimeout(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "u", "gpu.lock")
	ex, err := gpulease.Exclusive(context.Background(), lock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ex.Release() }()
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
	defer s.Close()
	c := &Client{BaseURL: s.URL, GPULock: lock, LeaseTimeout: 100 * time.Millisecond}
	if _, err := c.Chat(context.Background(), Request{}); !errors.Is(err, gpulease.ErrTimeout) {
		t.Errorf("err = %v", err)
	}
	if hits.Load() != 0 {
		t.Error("the request was sent while the GPU was held exclusively")
	}
}
