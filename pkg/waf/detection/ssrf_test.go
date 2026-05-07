// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package detection

import (
	"net"
	"testing"
	"time"
)

func TestSSRFDetector_Detect(t *testing.T) {
	detector := NewSSRFDetector(false)

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Localhost variations
		{"localhost", "http://localhost/admin", true},
		{"localhost with port", "http://localhost:8080/admin", true},
		{"127.0.0.1", "http://127.0.0.1/admin", true},
		{"127.0.0.1 with port", "http://127.0.0.1:8080/admin", true},
		{"127.x.x.x range", "http://127.0.0.2/admin", true},
		{"0.0.0.0", "http://0.0.0.0/admin", true},
		{"IPv6 loopback", "http://[::1]/admin", true},
		{"IPv6 expanded loopback", "http://[0:0:0:0:0:0:0:1]/admin", true},
		{"IPv4-mapped IPv6", "http://[::ffff:127.0.0.1]/admin", true},

		// Private IP ranges - Class A
		{"10.0.0.0/8", "http://10.0.0.1/admin", true},
		{"10.x.x.x", "http://10.255.255.255/admin", true},

		// Private IP ranges - Class B
		{"172.16.0.0/12", "http://172.16.0.1/admin", true},
		{"172.31.255.255", "http://172.31.255.255/admin", true},

		// Private IP ranges - Class C
		{"192.168.0.0/16", "http://192.168.1.1/admin", true},
		{"192.168.255.255", "http://192.168.255.255/admin", true},

		// Link-local addresses
		{"169.254.x.x", "http://169.254.1.1/", true},
		{"fe80::1", "http://[fe80::1]/", true},

		// Cloud metadata endpoints - AWS
		{"AWS metadata IP", "http://169.254.169.254/latest/meta-data/", true},
		{"AWS instance-data", "http://169.254.169.254/latest/instance-data/", true},
		{"AWS user-data", "http://169.254.169.254/latest/user-data/", true},
		{"AWS IAM credentials", "http://169.254.169.254/latest/meta-data/iam/security-credentials/", true},
		{"AWS API token", "http://169.254.169.254/latest/api/token", true},
		{"ec2.internal", "http://ip-172-31-1-2.ec2.internal/", true},

		// Cloud metadata endpoints - GCP
		{"GCP metadata", "http://metadata.google.internal/computeMetadata/v1/", true},
		{"GCP metadata.google.com", "http://metadata.google.com/computeMetadata/v1/", true},

		// Cloud metadata endpoints - Azure
		{"Azure metadata", "http://169.254.169.254/metadata/instance?api-version=2021-02-01", true},

		// Cloud metadata endpoints - DigitalOcean
		{"DigitalOcean metadata", "http://169.254.169.254/metadata/v1/", true},

		// Cloud metadata endpoints - Alibaba
		{"Alibaba metadata", "http://100.100.100.200/latest/meta-data/", true},

		// Kubernetes/Container
		{"Kubernetes default", "http://kubernetes.default.svc.cluster.local/", true},
		{"kube-system namespace", "http://10.96.0.1/api/v1/namespaces/kube-system", true},
		{"Kubernetes dashboard", "http://kubernetes-dashboard.kubernetes-dashboard/", true},

		// Docker
		{"Docker socket", "http://unix:///var/run/docker.sock", true},
		{"Docker internal", "http://host.docker.internal/", true},

		// Internal hostnames
		{".internal domain", "http://app.internal/api", true},
		{".local domain", "http://printer.local/", true},
		{".localhost domain", "http://app.localhost/", true},
		{".corp domain", "http://mail.corp/", true},
		{".lan domain", "http://router.lan/", true},
		{".intranet domain", "http://wiki.intranet/", true},

		// DNS rebinding domains
		{"xip.io", "http://127.0.0.1.xip.io/", true},
		{"nip.io", "http://192.168.1.1.nip.io/", true},
		{"sslip.io", "http://10-0-0-1.sslip.io/", true},
		{"localtest.me", "http://localtest.me/", true},
		{"vcap.me", "http://vcap.me/", true},
		{"lvh.me", "http://lvh.me/", true},

		// Dangerous protocols
		{"file protocol", "file:///etc/passwd", true},
		{"gopher protocol", "gopher://localhost:25/_HELO", true},
		{"dict protocol", "dict://localhost:11211/stat", true},
		{"ldap protocol", "ldap://localhost/", true},

		// Dangerous ports in URLs
		{"SSH port", "http://example.com:22/", true},
		{"MySQL port", "http://example.com:3306/", true},
		{"PostgreSQL port", "http://example.com:5432/", true},
		{"Redis port", "http://example.com:6379/", true},
		{"MongoDB port", "http://example.com:27017/", true},
		{"Elasticsearch port", "http://example.com:9200/", true},
		{"Docker port", "http://example.com:2375/", true},

		// IP encoding bypass - Hex
		{"Hex encoded IP", "http://0x7f000001/", true},
		{"Hex encoded 10.0.0.1", "http://0x0a000001/", true},

		// IP encoding bypass - Decimal
		{"Decimal IP 127.0.0.1", "http://2130706433/", true},

		// IP encoding bypass - Octal
		{"Octal IP", "http://0177.0.0.1/", true},

		// Credentials in URL
		{"Basic auth in URL", "http://admin:password@internal.server/", true},
		{"FTP with credentials", "ftp://user:pass@server/file", true},

		// Clean inputs - should NOT be detected
		{"Normal external URL", "http://example.com/page", false},
		{"HTTPS URL", "https://google.com/search?q=test", false},
		{"API endpoint", "https://api.github.com/users/test", false},
		{"Public IP", "http://8.8.8.8/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Detect(tt.input)
			if result != tt.expected {
				t.Errorf("Detect(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSSRFDetector_DetectWithDetails(t *testing.T) {
	detector := NewSSRFDetector(true)

	tests := []struct {
		name         string
		input        string
		wantDetected bool
		wantScoreMin int
		wantMatchMin int
	}{
		{
			name:         "AWS metadata with path",
			input:        "http://169.254.169.254/latest/meta-data/iam/security-credentials/admin-role",
			wantDetected: true,
			wantScoreMin: 25,
			wantMatchMin: 1,
		},
		{
			name:         "Multiple indicators",
			input:        "http://127.0.0.1:6379/SLAVEOF+evil.com+6379",
			wantDetected: true,
			wantScoreMin: 30,
			wantMatchMin: 2,
		},
		{
			name:         "DNS rebinding with private IP",
			input:        "http://192.168.1.1.nip.io:8080/admin",
			wantDetected: true,
			wantScoreMin: 20,
			wantMatchMin: 1,
		},
		{
			name:         "Clean external URL",
			input:        "https://api.stripe.com/v1/charges",
			wantDetected: false,
			wantScoreMin: 0,
			wantMatchMin: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectWithDetails(tt.input)
			if result.Detected != tt.wantDetected {
				t.Errorf("DetectWithDetails().Detected = %v, want %v", result.Detected, tt.wantDetected)
			}
			if result.Score < tt.wantScoreMin {
				t.Errorf("DetectWithDetails().Score = %d, want >= %d", result.Score, tt.wantScoreMin)
			}
			if len(result.Matches) < tt.wantMatchMin {
				t.Errorf("len(DetectWithDetails().Matches) = %d, want >= %d", len(result.Matches), tt.wantMatchMin)
			}
		})
	}
}

func TestSSRFDetector_IsPrivateIP(t *testing.T) {
	detector := NewSSRFDetector(false)

	tests := []struct {
		ip       string
		expected bool
	}{
		// Private IPs
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"169.254.169.254", true},
		{"0.0.0.0", true},

		// Public IPs
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},
		{"172.32.0.1", false},     // Just outside private range
		{"172.15.255.255", false}, // Just outside private range
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			result := detector.IsPrivateIP(tt.ip)
			if result != tt.expected {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestSSRFDetector_ValidateURL(t *testing.T) {
	detector := NewSSRFDetector(false)

	tests := []struct {
		name      string
		url       string
		wantValid bool
	}{
		// Valid URLs
		{"HTTPS URL", "https://example.com/api", true},
		{"HTTP URL", "http://example.com/page", true},
		{"URL with port", "https://example.com:8443/api", true},
		{"URL with path", "https://api.example.com/v1/users", true},

		// Invalid URLs
		{"localhost", "http://localhost/admin", false},
		{"127.0.0.1", "http://127.0.0.1/", false},
		{"Private IP", "http://192.168.1.1/", false},
		{"Metadata IP", "http://169.254.169.254/", false},
		{"file protocol", "file:///etc/passwd", false},
		{"gopher protocol", "gopher://localhost/", false},
		{".internal domain", "http://app.internal/", false},
		{".local domain", "http://printer.local/", false},
		{"Empty hostname", "http:///path", false},
		{"Dangerous port", "http://example.com:6379/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, reason := detector.ValidateURL(tt.url)
			if valid != tt.wantValid {
				t.Errorf("ValidateURL(%q) = %v (%s), want %v", tt.url, valid, reason, tt.wantValid)
			}
		})
	}
}

func TestSSRFDetector_IsCloudMetadata(t *testing.T) {
	detector := NewSSRFDetector(false)

	tests := []struct {
		host     string
		expected bool
	}{
		{"169.254.169.254", true},
		{"metadata.google.internal", true},
		{"metadata.google.com", true},
		{"100.100.100.200", true},
		{"instance-data", true},
		{"example.com", false},
		{"api.github.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			result := detector.IsCloudMetadata(tt.host)
			if result != tt.expected {
				t.Errorf("IsCloudMetadata(%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}

func TestSSRFDetector_GetDangerousPortInfo(t *testing.T) {
	detector := NewSSRFDetector(false)

	tests := []struct {
		port        int
		wantDanger  bool
		wantService string
	}{
		{22, true, "SSH"},
		{3306, true, "MySQL"},
		{5432, true, "PostgreSQL"},
		{6379, true, "Redis"},
		{27017, true, "MongoDB"},
		{80, false, "unknown"},
		{443, false, "unknown"},
		{8080, false, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.wantService, func(t *testing.T) {
			isDangerous, service := detector.GetDangerousPortInfo(tt.port)
			if isDangerous != tt.wantDanger {
				t.Errorf("GetDangerousPortInfo(%d) danger = %v, want %v", tt.port, isDangerous, tt.wantDanger)
			}
			if isDangerous && service != tt.wantService {
				t.Errorf("GetDangerousPortInfo(%d) service = %q, want %q", tt.port, service, tt.wantService)
			}
		})
	}
}

func TestDNSCache(t *testing.T) {
	cache := NewDNSCache(time.Minute)

	// Test Set and Get
	ips := []net.IP{net.ParseIP("93.184.216.34")}
	cache.Set("example.com", ips, false, 0)

	entry := cache.Get("example.com")
	if entry == nil {
		t.Fatal("Expected cached entry, got nil")
	}

	if entry.Hostname != "example.com" {
		t.Errorf("Entry hostname = %q, want %q", entry.Hostname, "example.com")
	}

	if entry.IsPrivate {
		t.Error("Expected IsPrivate = false")
	}

	// Test non-existent entry
	entry = cache.Get("notcached.com")
	if entry != nil {
		t.Errorf("Expected nil for non-existent entry, got %v", entry)
	}

	// Test Clear
	cache.Clear()
	entry = cache.Get("example.com")
	if entry != nil {
		t.Error("Expected nil after Clear")
	}
}

func TestDNSCache_Expiration(t *testing.T) {
	cache := NewDNSCache(100 * time.Millisecond)

	ips := []net.IP{net.ParseIP("93.184.216.34")}
	cache.Set("example.com", ips, false, 0)

	// Entry should exist immediately
	entry := cache.Get("example.com")
	if entry == nil {
		t.Fatal("Expected cached entry")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Entry should be expired
	entry = cache.Get("example.com")
	if entry != nil {
		t.Error("Expected entry to be expired")
	}
}

func TestParseIPFromDecimal(t *testing.T) {
	tests := []struct {
		decimal  string
		expected string
	}{
		{"2130706433", "127.0.0.1"},
		{"167772161", "10.0.0.1"},
		{"3232235777", "192.168.1.1"},
		{"0", "0.0.0.0"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.decimal, func(t *testing.T) {
			ip := ParseIPFromDecimal(tt.decimal)
			if tt.expected == "" {
				if ip != nil {
					t.Errorf("ParseIPFromDecimal(%q) = %v, want nil", tt.decimal, ip)
				}
			} else {
				if ip == nil || ip.String() != tt.expected {
					t.Errorf("ParseIPFromDecimal(%q) = %v, want %v", tt.decimal, ip, tt.expected)
				}
			}
		})
	}
}

func TestParseIPFromHex(t *testing.T) {
	tests := []struct {
		hex      string
		expected string
	}{
		{"0x7f000001", "127.0.0.1"},
		{"7f000001", "127.0.0.1"},
		{"0x0a000001", "10.0.0.1"},
		{"0xc0a80101", "192.168.1.1"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.hex, func(t *testing.T) {
			ip := ParseIPFromHex(tt.hex)
			if tt.expected == "" {
				if ip != nil {
					t.Errorf("ParseIPFromHex(%q) = %v, want nil", tt.hex, ip)
				}
			} else {
				if ip == nil || ip.String() != tt.expected {
					t.Errorf("ParseIPFromHex(%q) = %v, want %v", tt.hex, ip, tt.expected)
				}
			}
		})
	}
}

func TestParseIPFromOctal(t *testing.T) {
	tests := []struct {
		octal    string
		expected string
	}{
		{"0177.0.0.01", "127.0.0.1"},
		{"012.0.0.01", "10.0.0.1"},
		{"invalid", ""},
		{"300.0.0.1", ""}, // Invalid octal
	}

	for _, tt := range tests {
		t.Run(tt.octal, func(t *testing.T) {
			ip := ParseIPFromOctal(tt.octal)
			if tt.expected == "" {
				if ip != nil {
					t.Errorf("ParseIPFromOctal(%q) = %v, want nil", tt.octal, ip)
				}
			} else {
				if ip == nil || ip.String() != tt.expected {
					t.Errorf("ParseIPFromOctal(%q) = %v, want %v", tt.octal, ip, tt.expected)
				}
			}
		})
	}
}

func TestNormalizeIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Standard format
		{"127.0.0.1", "127.0.0.1"},
		{"10.0.0.1", "10.0.0.1"},
		// Decimal format
		{"2130706433", "127.0.0.1"},
		// Hex format
		{"0x7f000001", "127.0.0.1"},
		// Octal format
		{"0177.0.0.01", "127.0.0.1"},
		// Invalid
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ip := NormalizeIP(tt.input)
			if tt.expected == "" {
				if ip != nil {
					t.Errorf("NormalizeIP(%q) = %v, want nil", tt.input, ip)
				}
			} else {
				if ip == nil || ip.String() != tt.expected {
					t.Errorf("NormalizeIP(%q) = %v, want %v", tt.input, ip, tt.expected)
				}
			}
		})
	}
}

func TestSSRFURLChecker(t *testing.T) {
	checker := NewSSRFURLChecker()

	tests := []struct {
		name     string
		url      string
		wantSafe bool
	}{
		{"Public URL", "https://example.com/api", true},
		{"localhost", "http://localhost/", false},
		{"Private IP", "http://192.168.1.1/", false},
		{"Metadata", "http://169.254.169.254/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safe, _ := checker.IsSafe(tt.url)
			if safe != tt.wantSafe {
				t.Errorf("IsSafe(%q) = %v, want %v", tt.url, safe, tt.wantSafe)
			}
		})
	}
}

func TestSSRFDetector_GetPatternCount(t *testing.T) {
	detector := NewSSRFDetector(false)
	count := detector.GetPatternCount()
	if count < 50 {
		t.Errorf("GetPatternCount() = %d, want >= 50", count)
	}
}

func TestSSRFDetector_ClearDNSCache(t *testing.T) {
	detector := NewSSRFDetector(false)

	// This should not panic
	detector.ClearDNSCache()
}

func BenchmarkSSRFDetector_Detect(b *testing.B) {
	detector := NewSSRFDetector(false)
	input := "http://169.254.169.254/latest/meta-data/iam/security-credentials/"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(input)
	}
}

func BenchmarkSSRFDetector_DetectCleanInput(b *testing.B) {
	detector := NewSSRFDetector(false)
	input := "https://api.example.com/v1/users/12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(input)
	}
}

func BenchmarkSSRFDetector_ValidateURL(b *testing.B) {
	detector := NewSSRFDetector(false)
	url := "https://api.stripe.com/v1/charges"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.ValidateURL(url)
	}
}

func BenchmarkSSRFDetector_IsPrivateIP(b *testing.B) {
	detector := NewSSRFDetector(false)
	ip := "192.168.1.100"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.IsPrivateIP(ip)
	}
}
