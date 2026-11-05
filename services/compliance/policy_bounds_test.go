// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package compliance

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"unheaded/pkg/logger"
)

func postPolicy(mux http.Handler, body string) int {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compliance/policies", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// L4: one policy body is bounded.
func TestUpdatePolicy_BodyIsBounded(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	body := `{"src_service_id":"` + strings.Repeat("a", 2<<20) + `","dst_service_id":"b","action":"ALLOW"}`
	if got := postPolicy(mux, body); got != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized policy = %d, want 413", got)
	}
	if n := len(svc.policies); n != 0 {
		t.Errorf("%d policies stored", n)
	}
}

// L4: the table is bounded, and a full one still takes updates.
func TestUpdatePolicy_TableIsBounded(t *testing.T) {
	svc := NewService(logger.New(io.Discard), nil, nil)
	mux := http.NewServeMux()
	svc.registerRoutes(mux)

	for i := 0; i < 9000; i++ {
		body := fmt.Sprintf(`{"src_service_id":"s%d","dst_service_id":"d","action":"ALLOW"}`, i)
		if got := postPolicy(mux, body); got == http.StatusInsufficientStorage {
			break
		}
	}
	if n := len(svc.policies); n >= 9000 {
		t.Fatalf("table reached %d policies with no limit", n)
	}
	if got := postPolicy(mux, `{"src_service_id":"new","dst_service_id":"d","action":"ALLOW"}`); got != http.StatusInsufficientStorage {
		t.Errorf("new policy on a full table = %d, want 507", got)
	}
	if got := postPolicy(mux, `{"src_service_id":"s0","dst_service_id":"d","action":"DENY"}`); got != http.StatusOK {
		t.Errorf("update on a full table = %d, want 200", got)
	}
}
