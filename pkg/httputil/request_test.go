// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"net/http"
	"strings"
	"testing"
)

func TestDecodeJSONBody(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantErr     bool
		errCode     string
	}{
		{
			name:        "valid request",
			contentType: "application/json",
			body:        `{"title":"test"}`,
			wantErr:     false,
		},
		{
			name:        "wrong content type",
			contentType: "text/plain",
			body:        `{"title":"test"}`,
			wantErr:     true,
			errCode:     "INVALID_CONTENT_TYPE",
		},
		{
			name:        "empty body",
			contentType: "application/json",
			body:        "",
			wantErr:     true,
			errCode:     "EMPTY_BODY",
		},
		{
			name:        "invalid json",
			contentType: "application/json",
			body:        `{invalid}`,
			wantErr:     true,
			errCode:     "INVALID_JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)

			var dst map[string]string
			err := DecodeJSONBody(req, DefaultMaxBodySize, &dst)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				reqErr, ok := IsRequestError(err)
				if !ok {
					t.Fatalf("expected RequestError, got %T", err)
				}
				if reqErr.Code != tt.errCode {
					t.Errorf("expected error code %s, got %s", tt.errCode, reqErr.Code)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestDecodeJSONBody_OverLimit: the body used to be cut at the limit and
// parsed, so a JSON value followed by padding past the limit was accepted;
// a body over the limit is now refused with 413 whatever its content.
func TestDecodeJSONBody_OverLimit(t *testing.T) {
	const limit = 64
	for name, body := range map[string]string{
		"valid json then padding": `{"title":"test"}` + strings.Repeat(" ", limit),
		"one byte over":           `{"title":"` + strings.Repeat("a", limit-11) + `"}`,
	} {
		req, _ := http.NewRequest("POST", "/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		var dst map[string]string
		err := DecodeJSONBody(req, limit, &dst)
		reqErr, ok := IsRequestError(err)
		if !ok || reqErr.Status != http.StatusRequestEntityTooLarge {
			t.Errorf("%s (%d bytes, limit %d): err = %v, want 413", name, len(body), limit, err)
		}
	}

	exact := `{"title":"` + strings.Repeat("a", limit-12) + `"}`
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(exact))
	req.Header.Set("Content-Type", "application/json")
	var dst map[string]string
	if err := DecodeJSONBody(req, limit, &dst); err != nil {
		t.Errorf("body of exactly %d bytes (limit %d): %v", len(exact), limit, err)
	}
}
