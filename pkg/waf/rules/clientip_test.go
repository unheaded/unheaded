// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package rules

import (
	"net/http/httptest"
	"testing"
)

func TestExtractSecureClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
		wantIP     string
	}{
		{
			name:       "RemoteAddr only (external)",
			remoteAddr: "203.0.113.50:12345",
			wantIP:     "203.0.113.50",
		},
		{
			name:       "XFF from loopback is trusted",
			remoteAddr: "127.0.0.1:12345",
			xff:        "1.2.3.4",
			wantIP:     "1.2.3.4",
		},
		{
			name:       "XFF from private IP is trusted",
			remoteAddr: "10.10.10.100:12345",
			xff:        "203.0.113.50",
			wantIP:     "203.0.113.50",
		},
		{
			name:       "XFF from external IP is IGNORED (spoofing attempt)",
			remoteAddr: "8.8.8.8:12345",
			xff:        "192.168.1.1",
			wantIP:     "8.8.8.8",
		},
		{
			name:       "XRI from external IP is IGNORED (spoofing attempt)",
			remoteAddr: "8.8.8.8:12345",
			xri:        "192.168.1.1",
			wantIP:     "8.8.8.8",
		},
		{
			name:       "XFF multiple IPs from trusted proxy",
			remoteAddr: "127.0.0.1:12345",
			xff:        "1.2.3.4, 5.6.7.8",
			wantIP:     "1.2.3.4",
		},
		{
			name:       "XRI from trusted proxy",
			remoteAddr: "10.10.10.1:9999",
			xri:        "203.0.113.75",
			wantIP:     "203.0.113.75",
		},
		{
			name:       "RemoteAddr without port",
			remoteAddr: "1.2.3.4",
			wantIP:     "1.2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			got := extractSecureClientIP(req)
			if got != tt.wantIP {
				t.Errorf("extractSecureClientIP() = %q, want %q", got, tt.wantIP)
			}
		})
	}
}
