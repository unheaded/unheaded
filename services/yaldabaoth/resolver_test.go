// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCuirassResolver(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/api/v1/info":
			_, _ = w.Write([]byte(`{"lxd_backend":"https","lxd_project":"unheaded"}`))
		case "/api/v1/state/actual":
			_, _ = w.Write([]byte(`{"steel-echo-v2-0":{"status":"running","ip":"10.10.20.31","managed":true,"labels":{"chaos":"allowed"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	r, err := NewCuirassResolver(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	in, err := r.Instance(context.Background(), "steel-echo-v2-0")
	if err != nil {
		t.Fatal(err)
	}
	if in.Project != "unheaded" || !in.Managed || in.IP != "10.10.20.31" || in.Labels["chaos"] != "allowed" {
		t.Fatalf("instance %+v", in)
	}
	if _, err := r.Instance(context.Background(), "pihole"); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("unknown instance: %v", err)
	}
	// It feeds the allowlist end to end.
	rt, err := testAllowlist(t).Check(context.Background(), delayExp("steel-echo-v2-0", 0), checkEnv(r, "west"))
	if err != nil || rt.IP != "10.10.20.31" {
		t.Fatalf("allowlist through Cuirass: %+v %v", rt, err)
	}
	for _, bad := range []string{"", "ftp://x", "http://u:p@x"} {
		if _, err := NewCuirassResolver(bad); err == nil {
			t.Errorf("URL %q accepted", bad)
		}
	}
}
