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
