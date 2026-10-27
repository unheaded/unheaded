// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cuirassdriver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// S-IRON-GATE Phase 5: with an Authorizer the driver sends a minted token
// (Authorization) on writes instead of the API key, and reads go bare.
func TestDriver_AuthorizerOnWrites(t *testing.T) {
	var gotAuthz, gotKey, getAuthz string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getAuthz = r.Header.Get("Authorization")
		} else {
			gotAuthz, gotKey = r.Header.Get("Authorization"), r.Header.Get("X-API-Key")
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	d, err := New(Config{URL: srv.URL, Authorizer: func() (string, error) { return "Bearer tok-1", nil }})
	if err != nil {
		t.Fatalf("New with only an Authorizer: %v", err)
	}
	if err := d.do(context.Background(), http.MethodPost, "/api/v1/reconcile", nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotAuthz != "Bearer tok-1" || gotKey != "" {
		t.Errorf("write sent Authorization %q, X-API-Key %q; want the token only", gotAuthz, gotKey)
	}
	if err := d.do(context.Background(), http.MethodGet, "/api/v1/info", nil, nil); err != nil {
		t.Fatal(err)
	}
	if getAuthz != "" {
		t.Errorf("read sent Authorization %q, want none", getAuthz)
	}
}

func TestDriver_AuthorizerErrorFailsTheWrite(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true }))
	defer srv.Close()
	d, err := New(Config{URL: srv.URL, Authorizer: func() (string, error) { return "", errors.New("key gone") }})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.do(context.Background(), http.MethodPut, "/api/v1/containers/x", map[string]string{}, nil); err == nil {
		t.Error("a write without a token went through")
	}
	if called {
		t.Error("the request was sent although no token could be minted")
	}
}

func TestNew_NeedsACredential(t *testing.T) {
	if _, err := New(Config{URL: "http://127.0.0.1:19006"}); err == nil {
		t.Error("New with neither APIKey nor Authorizer succeeded")
	}
}
