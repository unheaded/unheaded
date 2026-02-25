// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package wotanClient

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishWithTimeout_Success(t *testing.T) {
	var callCount int64

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/timeout-ok/publish": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["timeout-ok"] = &Subscriber{
		SubscriberID: "sub-timeout-ok",
		Topic:        "timeout-ok",
		Status:       "approved",
	}
	c.mu.Unlock()

	err := c.PublishWithTimeout(context.Background(), "timeout-ok", []byte("data"), 5*time.Second)
	if err != nil {
		t.Fatalf("PublishWithTimeout: %v", err)
	}

	if atomic.LoadInt64(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", atomic.LoadInt64(&callCount))
	}
}

func TestPublishWithTimeout_ExceedsDeadline(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/timeout-slow/publish": func(w http.ResponseWriter, r *http.Request) {
			// Block longer than the timeout.
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["timeout-slow"] = &Subscriber{
		SubscriberID: "sub-timeout-slow",
		Topic:        "timeout-slow",
		Status:       "approved",
	}
	c.mu.Unlock()

	start := time.Now()
	err := c.PublishWithTimeout(context.Background(), "timeout-slow", []byte("data"), 100*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}

	if !errors.Is(err, ErrMessageTimeout) {
		t.Errorf("expected ErrMessageTimeout, got: %v", err)
	}

	if elapsed > 2*time.Second {
		t.Errorf("timeout should have triggered quickly, took %s", elapsed)
	}
}

func TestPublishWithTimeout_DefaultTimeout(t *testing.T) {
	// Passing 0 should use DefaultMessageTimeout (30s).
	// We can't actually wait 30s, so just test it doesn't panic
	// and returns quickly on a fast server.
	var callCount int64

	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/default-to/publish": func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&callCount, 1)
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["default-to"] = &Subscriber{
		SubscriberID: "sub-default",
		Topic:        "default-to",
		Status:       "approved",
	}
	c.mu.Unlock()

	err := c.PublishWithTimeout(context.Background(), "default-to", []byte("data"), 0)
	if err != nil {
		t.Fatalf("PublishWithTimeout with default: %v", err)
	}
}

func TestPublishWithTimeout_ParentContextCancel(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/parent-ctx/publish": func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Second)
			w.WriteHeader(http.StatusCreated)
		},
	})

	c.mu.Lock()
	c.subscribers["parent-ctx"] = &Subscriber{
		SubscriberID: "sub-parent",
		Topic:        "parent-ctx",
		Status:       "approved",
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := c.PublishWithTimeout(ctx, "parent-ctx", []byte("data"), 30*time.Second)
	if err == nil {
		t.Fatal("expected error from parent context cancellation")
	}
}

func TestPublishWithTimeout_AppError(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"/api/v1/topics/app-err-to/publish": func(w http.ResponseWriter, r *http.Request) {
			errorResponse(w, http.StatusForbidden, "NOT_AUTHORIZED", "denied")
		},
	})

	c.mu.Lock()
	c.subscribers["app-err-to"] = &Subscriber{
		SubscriberID: "sub-app-err",
		Topic:        "app-err-to",
		Status:       "approved",
	}
	c.mu.Unlock()

	err := c.PublishWithTimeout(context.Background(), "app-err-to", []byte("data"), 5*time.Second)
	if err == nil {
		t.Fatal("expected error")
	}

	// App errors should NOT be wrapped as timeout errors.
	if errors.Is(err, ErrMessageTimeout) {
		t.Error("app error should not be ErrMessageTimeout")
	}
}
