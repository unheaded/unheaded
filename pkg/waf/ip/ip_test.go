// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package ip

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBlocklist_Add(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	// Add single IP
	bl.Add("192.168.1.100", "test block")

	if !bl.Contains("192.168.1.100") {
		t.Error("IP should be in blocklist")
	}

	if bl.Contains("192.168.1.101") {
		t.Error("IP should not be in blocklist")
	}
}

func TestBlocklist_AddWithDuration(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	// Add with short duration
	bl.AddWithDuration("192.168.1.100", "temp block", 100*time.Millisecond)

	if !bl.Contains("192.168.1.100") {
		t.Error("IP should be in blocklist")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	if bl.Contains("192.168.1.100") {
		t.Error("IP should have expired from blocklist")
	}
}

func TestBlocklist_AddNetwork(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	// Add network range
	err := bl.AddNetwork("10.0.0.0/8", "private network")
	if err != nil {
		t.Fatalf("AddNetwork error: %v", err)
	}

	// Test IPs in range
	if !bl.Contains("10.0.0.1") {
		t.Error("10.0.0.1 should be blocked")
	}

	if !bl.Contains("10.255.255.255") {
		t.Error("10.255.255.255 should be blocked")
	}

	// Test IP outside range
	if bl.Contains("11.0.0.1") {
		t.Error("11.0.0.1 should not be blocked")
	}
}

func TestBlocklist_AddNetworkWithCIDR(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	// Add using Add method with CIDR notation
	bl.Add("172.16.0.0/12", "private network B")

	if !bl.Contains("172.16.0.1") {
		t.Error("172.16.0.1 should be blocked")
	}

	if !bl.Contains("172.31.255.255") {
		t.Error("172.31.255.255 should be blocked")
	}

	if bl.Contains("172.32.0.1") {
		t.Error("172.32.0.1 should not be blocked")
	}
}

func TestBlocklist_Remove(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	bl.Add("192.168.1.100", "test")

	// Verify added
	if !bl.Contains("192.168.1.100") {
		t.Error("IP should be in blocklist before removal")
	}

	// Remove
	removed := bl.Remove("192.168.1.100")
	if !removed {
		t.Error("Remove should return true")
	}

	// Verify removed
	if bl.Contains("192.168.1.100") {
		t.Error("IP should not be in blocklist after removal")
	}

	// Remove non-existent
	removed = bl.Remove("192.168.1.100")
	if removed {
		t.Error("Remove should return false for non-existent IP")
	}
}

func TestBlocklist_RemoveNetwork(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	bl.Add("10.0.0.0/8", "test network")

	removed := bl.Remove("10.0.0.0/8")
	if !removed {
		t.Error("Remove should return true for network")
	}

	if bl.Contains("10.0.0.1") {
		t.Error("Network should be removed")
	}
}

func TestBlocklist_GetEntry(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	bl.Add("192.168.1.100", "test reason")

	entry := bl.GetEntry("192.168.1.100")
	if entry == nil {
		t.Fatal("Entry should exist")
	}

	if entry.Reason != "test reason" {
		t.Errorf("Reason = %q, want %q", entry.Reason, "test reason")
	}

	if !entry.Permanent {
		t.Error("Entry should be permanent")
	}

	// Non-existent
	entry = bl.GetEntry("192.168.1.101")
	if entry != nil {
		t.Error("Entry should not exist")
	}
}

func TestBlocklist_GetEntryForNetwork(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	bl.Add("10.0.0.0/8", "test network")

	entry := bl.GetEntry("10.1.2.3")
	if entry == nil {
		t.Fatal("Entry should exist for IP in network")
	}

	if entry.IP != "10.0.0.0/8" {
		t.Errorf("IP = %q, want %q", entry.IP, "10.0.0.0/8")
	}
}

func TestBlocklist_List(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	bl.Add("192.168.1.100", "test 1")
	bl.Add("192.168.1.101", "test 2")
	bl.Add("10.0.0.0/8", "network")

	entries := bl.List()
	if len(entries) != 3 {
		t.Errorf("List() returned %d entries, want 3", len(entries))
	}
}

func TestBlocklist_Count(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	if bl.Count() != 0 {
		t.Error("Initial count should be 0")
	}

	bl.Add("192.168.1.100", "test 1")
	bl.Add("192.168.1.101", "test 2")

	if bl.Count() != 2 {
		t.Errorf("Count = %d, want 2", bl.Count())
	}
}

func TestBlocklist_Clear(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	bl.Add("192.168.1.100", "test 1")
	bl.Add("192.168.1.101", "test 2")

	bl.Clear()

	if bl.Count() != 0 {
		t.Errorf("Count after clear = %d, want 0", bl.Count())
	}

	if bl.Contains("192.168.1.100") {
		t.Error("Blocklist should be empty after clear")
	}
}

func TestBlocklist_LoadFromReader(t *testing.T) {
	bl := NewBlocklist()
	defer bl.Close()

	data := `# This is a comment
192.168.1.100 Blocked for testing
192.168.1.101
10.0.0.0/8 Private network

# Another comment
172.16.0.0/12 Private network B`

	err := bl.LoadFromReader(strings.NewReader(data))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}

	if !bl.Contains("192.168.1.100") {
		t.Error("192.168.1.100 should be loaded")
	}

	if !bl.Contains("10.0.0.1") {
		t.Error("10.0.0.1 should be blocked (network)")
	}

	if bl.Count() != 4 {
		t.Errorf("Count = %d, want 4", bl.Count())
	}
}

func TestBlockEntry_IsExpired(t *testing.T) {
	// Permanent entry
	entry := &BlockEntry{
		IP:        "192.168.1.100",
		Permanent: true,
	}
	if entry.IsExpired() {
		t.Error("Permanent entry should not expire")
	}

	// Not expired
	entry = &BlockEntry{
		IP:        "192.168.1.100",
		Permanent: false,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if entry.IsExpired() {
		t.Error("Entry should not be expired")
	}

	// Expired
	entry = &BlockEntry{
		IP:        "192.168.1.100",
		Permanent: false,
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if !entry.IsExpired() {
		t.Error("Entry should be expired")
	}
}

func TestAllowlist_Add(t *testing.T) {
	al := NewAllowlist()

	al.Add("10.0.0.1", "trusted server")

	if !al.Contains("10.0.0.1") {
		t.Error("IP should be in allowlist")
	}

	if al.Contains("10.0.0.2") {
		t.Error("IP should not be in allowlist")
	}
}

func TestAllowlist_AddNetwork(t *testing.T) {
	al := NewAllowlist()

	err := al.AddNetwork("10.0.0.0/8", "internal network")
	if err != nil {
		t.Fatalf("AddNetwork error: %v", err)
	}

	if !al.Contains("10.0.0.1") {
		t.Error("10.0.0.1 should be allowed")
	}

	if !al.Contains("10.255.255.255") {
		t.Error("10.255.255.255 should be allowed")
	}

	if al.Contains("11.0.0.1") {
		t.Error("11.0.0.1 should not be allowed")
	}
}

func TestAllowlist_AddWithCIDR(t *testing.T) {
	al := NewAllowlist()

	// Add using Add method with CIDR
	al.Add("192.168.0.0/16", "local network")

	if !al.Contains("192.168.1.1") {
		t.Error("192.168.1.1 should be allowed")
	}
}

func TestAllowlist_Remove(t *testing.T) {
	al := NewAllowlist()

	al.Add("10.0.0.1", "test")

	removed := al.Remove("10.0.0.1")
	if !removed {
		t.Error("Remove should return true")
	}

	if al.Contains("10.0.0.1") {
		t.Error("IP should be removed")
	}

	removed = al.Remove("10.0.0.1")
	if removed {
		t.Error("Remove should return false for non-existent IP")
	}
}

func TestAllowlist_GetEntry(t *testing.T) {
	al := NewAllowlist()

	al.Add("10.0.0.1", "test comment")

	entry := al.GetEntry("10.0.0.1")
	if entry == nil {
		t.Fatal("Entry should exist")
	}

	if entry.Comment != "test comment" {
		t.Errorf("Comment = %q, want %q", entry.Comment, "test comment")
	}

	entry = al.GetEntry("10.0.0.2")
	if entry != nil {
		t.Error("Entry should not exist")
	}
}

func TestAllowlist_List(t *testing.T) {
	al := NewAllowlist()

	al.Add("10.0.0.1", "test 1")
	al.Add("10.0.0.2", "test 2")

	entries := al.List()
	if len(entries) != 2 {
		t.Errorf("List() returned %d entries, want 2", len(entries))
	}
}

func TestAllowlist_Count(t *testing.T) {
	al := NewAllowlist()

	if al.Count() != 0 {
		t.Error("Initial count should be 0")
	}

	al.Add("10.0.0.1", "test")
	al.Add("10.0.0.2", "test")

	if al.Count() != 2 {
		t.Errorf("Count = %d, want 2", al.Count())
	}
}

func TestAllowlist_Clear(t *testing.T) {
	al := NewAllowlist()

	al.Add("10.0.0.1", "test")
	al.Add("10.0.0.2", "test")

	al.Clear()

	if al.Count() != 0 {
		t.Errorf("Count after clear = %d, want 0", al.Count())
	}
}

func TestAllowlist_LoadFromReader(t *testing.T) {
	al := NewAllowlist()

	data := `# Trusted servers
10.0.0.1 Primary server
10.0.0.2 Secondary server
192.168.0.0/16 Local network`

	err := al.LoadFromReader(strings.NewReader(data))
	if err != nil {
		t.Fatalf("LoadFromReader error: %v", err)
	}

	if !al.Contains("10.0.0.1") {
		t.Error("10.0.0.1 should be loaded")
	}

	if !al.Contains("192.168.1.1") {
		t.Error("192.168.1.1 should be allowed (network)")
	}
}

func TestAllowlist_AddTrustedProxies(t *testing.T) {
	al := NewAllowlist()

	al.AddTrustedProxies()

	// Check some Cloudflare IPs
	if !al.Contains("173.245.48.1") {
		t.Error("Cloudflare IP should be allowed")
	}

	if !al.Contains("103.21.244.1") {
		t.Error("Cloudflare IP should be allowed")
	}
}

func TestAllowlist_AddLocalhost(t *testing.T) {
	al := NewAllowlist()

	al.AddLocalhost()

	if !al.Contains("127.0.0.1") {
		t.Error("127.0.0.1 should be allowed")
	}

	if !al.Contains("::1") {
		t.Error("::1 should be allowed")
	}

	if !al.Contains("127.0.0.2") {
		t.Error("127.0.0.2 should be allowed (in 127.0.0.0/8)")
	}
}

func TestIPManager_GetClientIP(t *testing.T) {
	manager := NewIPManager()

	tests := []struct {
		name        string
		remoteAddr  string
		xff         string
		xri         string
		cfIP        string
		expectedIP  string
	}{
		{
			name:       "RemoteAddr only",
			remoteAddr: "1.2.3.4:12345",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For single",
			remoteAddr: "127.0.0.1:12345",
			xff:        "1.2.3.4",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "X-Forwarded-For multiple",
			remoteAddr: "127.0.0.1:12345",
			xff:        "1.2.3.4, 5.6.7.8, 9.10.11.12",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "127.0.0.1:12345",
			xri:        "1.2.3.4",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "CF-Connecting-IP",
			remoteAddr: "127.0.0.1:12345",
			cfIP:       "1.2.3.4",
			expectedIP: "1.2.3.4",
		},
		{
			name:       "IPv6 RemoteAddr",
			remoteAddr: "[::1]:12345",
			expectedIP: "::1",
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
			if tt.cfIP != "" {
				req.Header.Set("CF-Connecting-IP", tt.cfIP)
			}

			ip := manager.GetClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("GetClientIP() = %q, want %q", ip, tt.expectedIP)
			}
		})
	}
}

func TestIPManager_CheckRequest(t *testing.T) {
	manager := NewIPManager()

	// Block an IP
	manager.Blocklist().Add("192.168.1.100", "test block")

	// Allow an IP
	manager.Allowlist().Add("10.0.0.1", "trusted")

	tests := []struct {
		name       string
		remoteAddr string
		allowed    bool
	}{
		{
			name:       "Blocked IP",
			remoteAddr: "192.168.1.100:12345",
			allowed:    false,
		},
		{
			name:       "Allowed IP",
			remoteAddr: "10.0.0.1:12345",
			allowed:    true,
		},
		{
			name:       "Unknown IP",
			remoteAddr: "8.8.8.8:12345",
			allowed:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/", nil)
			req.RemoteAddr = tt.remoteAddr

			allowed, _ := manager.CheckRequest(req)
			if allowed != tt.allowed {
				t.Errorf("CheckRequest() = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}

func TestIPManager_Blocklist(t *testing.T) {
	manager := NewIPManager()

	bl := manager.Blocklist()
	if bl == nil {
		t.Fatal("Blocklist should not be nil")
	}

	bl.Add("192.168.1.100", "test")

	// Verify through manager
	bl2 := manager.Blocklist()
	if !bl2.Contains("192.168.1.100") {
		t.Error("Blocklist should contain added IP")
	}
}

func TestIPManager_Allowlist(t *testing.T) {
	manager := NewIPManager()

	al := manager.Allowlist()
	if al == nil {
		t.Fatal("Allowlist should not be nil")
	}

	al.Add("10.0.0.1", "test")

	al2 := manager.Allowlist()
	if !al2.Contains("10.0.0.1") {
		t.Error("Allowlist should contain added IP")
	}
}

func TestGeoBlocker_AddCountryRange(t *testing.T) {
	blocker := NewGeoBlocker()

	err := blocker.AddCountryRange("US", "8.0.0.0/8")
	if err != nil {
		t.Fatalf("AddCountryRange error: %v", err)
	}

	// Lookup
	country := blocker.LookupCountry("8.8.8.8")
	if country != "US" {
		t.Errorf("LookupCountry() = %q, want %q", country, "US")
	}

	// Unknown IP
	country = blocker.LookupCountry("1.2.3.4")
	if country != "" {
		t.Errorf("LookupCountry() = %q, want empty", country)
	}
}

func TestGeoBlocker_BlockCountry(t *testing.T) {
	blocker := NewGeoBlocker()

	blocker.AddCountryRange("CN", "1.0.0.0/8")
	blocker.BlockCountry("CN")

	if !blocker.IsBlocked("1.2.3.4") {
		t.Error("IP in blocked country should be blocked")
	}

	if blocker.IsBlocked("8.8.8.8") {
		t.Error("IP not in blocked country should not be blocked")
	}
}

func TestGeoBlocker_UnblockCountry(t *testing.T) {
	blocker := NewGeoBlocker()

	blocker.AddCountryRange("CN", "1.0.0.0/8")
	blocker.BlockCountry("CN")
	blocker.UnblockCountry("CN")

	if blocker.IsBlocked("1.2.3.4") {
		t.Error("IP in unblocked country should not be blocked")
	}
}

func TestGeoBlocker_ListBlockedCountries(t *testing.T) {
	blocker := NewGeoBlocker()

	blocker.BlockCountry("CN")
	blocker.BlockCountry("RU")

	countries := blocker.ListBlockedCountries()
	if len(countries) != 2 {
		t.Errorf("ListBlockedCountries() returned %d, want 2", len(countries))
	}
}

func BenchmarkBlocklist_Contains(b *testing.B) {
	bl := NewBlocklist()
	defer bl.Close()

	// Add some IPs
	for i := 0; i < 1000; i++ {
		bl.Add("192.168."+string(rune(i/256))+"."+ string(rune(i%256)), "test")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl.Contains("192.168.1.100")
	}
}

func BenchmarkBlocklist_ContainsNetwork(b *testing.B) {
	bl := NewBlocklist()
	defer bl.Close()

	// Add network ranges
	bl.AddNetwork("10.0.0.0/8", "test")
	bl.AddNetwork("172.16.0.0/12", "test")
	bl.AddNetwork("192.168.0.0/16", "test")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl.Contains("10.1.2.3")
	}
}

func BenchmarkAllowlist_Contains(b *testing.B) {
	al := NewAllowlist()

	// Add some IPs
	for i := 0; i < 100; i++ {
		al.Add("10.0.0."+string(rune(i)), "test")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		al.Contains("10.0.0.50")
	}
}

func BenchmarkIPManager_GetClientIP(b *testing.B) {
	manager := NewIPManager()
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "1.2.3.4:12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetClientIP(req)
	}
}

func BenchmarkIPManager_GetClientIPWithXFF(b *testing.B) {
	manager := NewIPManager()
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8, 9.10.11.12")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetClientIP(req)
	}
}

// --- Tests for utility functions ---

func TestParseCIDR(t *testing.T) {
	network, err := ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatalf("ParseCIDR: %v", err)
	}
	if network.String() != "10.0.0.0/8" {
		t.Errorf("got %s, want 10.0.0.0/8", network.String())
	}

	_, err = ParseCIDR("invalid")
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true},
		{"::1", true},
		{"8.8.8.8", false},
		{"1.2.3.4", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := IsPrivateIP(tt.ip); got != tt.private {
				t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
			}
		})
	}
}

func TestNormalizeIP(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"127.0.0.1", "127.0.0.1"},
		{"::1", "::1"},
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		if got := NormalizeIP(tt.input); got != tt.expected {
			t.Errorf("NormalizeIP(%s) = %s, want %s", tt.input, got, tt.expected)
		}
	}
}

func TestSimpleIPRange(t *testing.T) {
	r, err := NewSimpleIPRange("10.0.0.1", "10.0.0.10")
	if err != nil {
		t.Fatalf("NewSimpleIPRange: %v", err)
	}

	if !r.Contains("10.0.0.5") {
		t.Error("range should contain 10.0.0.5")
	}
	if !r.Contains("10.0.0.1") {
		t.Error("range should contain start IP")
	}
	if !r.Contains("10.0.0.10") {
		t.Error("range should contain end IP")
	}
	if r.Contains("10.0.0.11") {
		t.Error("range should not contain 10.0.0.11")
	}
	if r.Contains("9.255.255.255") {
		t.Error("range should not contain 9.255.255.255")
	}
	if r.Contains("invalid") {
		t.Error("range should not contain invalid IP")
	}
}

func TestNewSimpleIPRange_InvalidIPs(t *testing.T) {
	_, err := NewSimpleIPRange("invalid", "10.0.0.10")
	if err == nil {
		t.Error("expected error for invalid start IP")
	}

	_, err = NewSimpleIPRange("10.0.0.1", "invalid")
	if err == nil {
		t.Error("expected error for invalid end IP")
	}
}

func TestIPManager_SetTrustProxies(t *testing.T) {
	manager := NewIPManager()
	manager.SetTrustProxies(false)

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	req.Header.Set("X-Forwarded-For", "5.6.7.8")

	ip := manager.GetClientIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("expected RemoteAddr IP when trust disabled, got %s", ip)
	}
}

func TestIPManager_SetProxyHeaders(t *testing.T) {
	manager := NewIPManager()
	manager.SetProxyHeaders([]string{"X-Custom-IP"})

	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	req.Header.Set("X-Custom-IP", "5.6.7.8")

	ip := manager.GetClientIP(req)
	if ip != "5.6.7.8" {
		t.Errorf("expected custom header IP, got %s", ip)
	}
}

func TestIPManager_IsAllowed(t *testing.T) {
	manager := NewIPManager()
	manager.Allowlist().Add("10.0.0.1", "trusted")
	manager.Blocklist().Add("192.168.1.100", "blocked")

	if !manager.IsAllowed("10.0.0.1") {
		t.Error("allowlisted IP should be allowed")
	}
	if manager.IsAllowed("192.168.1.100") {
		t.Error("blocklisted IP should not be allowed")
	}
	if !manager.IsAllowed("8.8.8.8") {
		t.Error("unknown IP should be allowed")
	}
}

func TestIPManager_GeoBlocker(t *testing.T) {
	manager := NewIPManager()
	gb := manager.GeoBlocker()
	if gb == nil {
		t.Fatal("GeoBlocker should not be nil")
	}
}

func TestIPManager_GetClientIP_NoPort(t *testing.T) {
	manager := NewIPManager()
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "1.2.3.4" // no port

	ip := manager.GetClientIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("expected raw RemoteAddr, got %s", ip)
	}
}

func TestIPManager_GetClientIP_InvalidXFF(t *testing.T) {
	manager := NewIPManager()
	req := httptest.NewRequest("GET", "http://example.com/", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	req.Header.Set("X-Forwarded-For", "not-an-ip")

	ip := manager.GetClientIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("expected RemoteAddr fallback for invalid XFF, got %s", ip)
	}
}

func TestGeoBlocker_IsCountryBlocked(t *testing.T) {
	gb := NewGeoBlocker()
	gb.BlockCountry("CN")

	if !gb.IsCountryBlocked("CN") {
		t.Error("CN should be blocked")
	}
	if gb.IsCountryBlocked("US") {
		t.Error("US should not be blocked")
	}
}

func TestGeoBlocker_Clear(t *testing.T) {
	gb := NewGeoBlocker()
	gb.BlockCountry("CN")
	gb.AddCountryRange("CN", "1.0.0.0/8")

	gb.Clear()

	if gb.IsCountryBlocked("CN") {
		t.Error("CN should not be blocked after clear")
	}
	if gb.LookupCountry("1.2.3.4") != "" {
		t.Error("ranges should be cleared")
	}
}

func TestGeoBlocker_LoadKnownRanges(t *testing.T) {
	gb := NewGeoBlocker()
	gb.LoadKnownRanges()

	// After loading known ranges, US IPs should map to US
	country := gb.LookupCountry("3.0.0.1")
	if country != "US" {
		t.Errorf("expected US for 3.0.0.1, got %q", country)
	}
}

func TestGeoBlocker_Stats(t *testing.T) {
	gb := NewGeoBlocker()
	gb.BlockCountry("CN")
	gb.AddCountryRange("CN", "1.0.0.0/8")

	stats := gb.Stats()
	if stats["blocked_countries"] != 1 {
		t.Errorf("expected 1 blocked country, got %v", stats["blocked_countries"])
	}
}

func TestGeoBlocker_InvalidIP(t *testing.T) {
	gb := NewGeoBlocker()
	country := gb.LookupCountry("not-an-ip")
	if country != "" {
		t.Errorf("expected empty for invalid IP, got %q", country)
	}
}

func TestGeoBlocker_AddCountryRange_InvalidCIDR(t *testing.T) {
	gb := NewGeoBlocker()
	err := gb.AddCountryRange("US", "invalid")
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

// --- Tests for EnhancedGeoBlocker ---

func TestEnhancedGeoBlocker_BlockCountry(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AddIPRange("10.0.0.1", "10.0.0.255", "CN", "", "", 0, "")
	gb.BlockCountry("cn") // lowercase should work

	result := gb.Lookup("10.0.0.50")
	if !result.IsBlocked {
		t.Error("IP in blocked country should be blocked")
	}
	if result.BlockReason != "country_blocked" {
		t.Errorf("reason = %q, want country_blocked", result.BlockReason)
	}
}

func TestEnhancedGeoBlocker_UnblockCountry(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.BlockCountry("CN")
	gb.UnblockCountry("CN")

	countries := gb.ListBlockedCountries()
	if len(countries) != 0 {
		t.Errorf("expected 0 blocked countries after unblock, got %d", len(countries))
	}
}

func TestEnhancedGeoBlocker_AllowOnlyCountries(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AddIPRange("10.0.0.1", "10.0.0.255", "US", "", "", 0, "")
	gb.AddIPRange("20.0.0.1", "20.0.0.255", "CN", "", "", 0, "")
	gb.AllowOnlyCountries([]string{"US"})

	result := gb.Lookup("10.0.0.50")
	if result.IsBlocked {
		t.Error("US IP should not be blocked in allow mode")
	}

	result = gb.Lookup("20.0.0.50")
	if !result.IsBlocked {
		t.Error("CN IP should be blocked in US-only allow mode")
	}
	if result.BlockReason != "country_not_in_allowlist" {
		t.Errorf("reason = %q, want country_not_in_allowlist", result.BlockReason)
	}

	allowed := gb.ListAllowedCountries()
	if len(allowed) != 1 {
		t.Errorf("expected 1 allowed country, got %d", len(allowed))
	}
}

func TestEnhancedGeoBlocker_DisableAllowList(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AllowOnlyCountries([]string{"US"})
	gb.DisableAllowList()

	gb.AddIPRange("20.0.0.1", "20.0.0.255", "CN", "", "", 0, "")
	result := gb.Lookup("20.0.0.50")
	if result.IsBlocked {
		t.Error("IP should not be blocked after disabling allow list")
	}
}

func TestEnhancedGeoBlocker_BlockContinent(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AddIPRange("10.0.0.1", "10.0.0.255", "CN", "", "", 0, "")
	gb.BlockContinent("AS") // CN is in Asia

	result := gb.Lookup("10.0.0.50")
	if !result.IsBlocked {
		t.Error("IP in blocked continent should be blocked")
	}
	if result.BlockReason != "continent_blocked" {
		t.Errorf("reason = %q, want continent_blocked", result.BlockReason)
	}
}

func TestEnhancedGeoBlocker_UnblockContinent(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.BlockContinent("AS")
	gb.UnblockContinent("AS")

	gb.AddIPRange("10.0.0.1", "10.0.0.255", "CN", "", "", 0, "")
	result := gb.Lookup("10.0.0.50")
	if result.IsBlocked {
		t.Error("IP should not be blocked after unblocking continent")
	}
}

func TestEnhancedGeoBlocker_BlockASN(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AddIPRange("10.0.0.1", "10.0.0.255", "US", "", "", 12345, "")
	gb.BlockASN(12345)

	result := gb.Lookup("10.0.0.50")
	if !result.IsBlocked {
		t.Error("IP with blocked ASN should be blocked")
	}
	if result.BlockReason != "asn_blocked" {
		t.Errorf("reason = %q, want asn_blocked", result.BlockReason)
	}

	gb.UnblockASN(12345)
	result = gb.Lookup("10.0.0.50")
	if result.IsBlocked {
		t.Error("IP should not be blocked after unblocking ASN")
	}
}

func TestEnhancedGeoBlocker_BlockRegion(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AddIPRange("10.0.0.1", "10.0.0.255", "US", "TX", "Dallas", 0, "")
	gb.BlockRegion("US", "TX")

	result := gb.Lookup("10.0.0.50")
	if !result.IsBlocked {
		t.Error("IP in blocked region should be blocked")
	}
	if result.BlockReason != "region_blocked" {
		t.Errorf("reason = %q, want region_blocked", result.BlockReason)
	}
}

func TestEnhancedGeoBlocker_BlockAnonymizers(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.SetBlockAnonymizers(true)

	gb.AddIPRangeWithFlags("10.0.0.1", "10.0.0.255", "US", true, false, false, false)
	result := gb.Lookup("10.0.0.50")
	if !result.IsBlocked || result.BlockReason != "proxy_blocked" {
		t.Errorf("proxy should be blocked, got blocked=%v reason=%q", result.IsBlocked, result.BlockReason)
	}

	gb.Clear()
	gb.SetBlockAnonymizers(true)
	gb.AddIPRangeWithFlags("20.0.0.1", "20.0.0.255", "US", false, true, false, false)
	result = gb.Lookup("20.0.0.50")
	if !result.IsBlocked || result.BlockReason != "vpn_blocked" {
		t.Errorf("VPN should be blocked, got blocked=%v reason=%q", result.IsBlocked, result.BlockReason)
	}

	gb.Clear()
	gb.SetBlockAnonymizers(true)
	gb.AddIPRangeWithFlags("30.0.0.1", "30.0.0.255", "US", false, false, true, false)
	result = gb.Lookup("30.0.0.50")
	if !result.IsBlocked || result.BlockReason != "tor_blocked" {
		t.Errorf("Tor should be blocked, got blocked=%v reason=%q", result.IsBlocked, result.BlockReason)
	}
}

func TestEnhancedGeoBlocker_BlockDataCenters(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.SetBlockDataCenters(true)
	gb.AddIPRangeWithFlags("10.0.0.1", "10.0.0.255", "US", false, false, false, true)

	result := gb.Lookup("10.0.0.50")
	if !result.IsBlocked || result.BlockReason != "datacenter_blocked" {
		t.Errorf("datacenter should be blocked, got blocked=%v reason=%q", result.IsBlocked, result.BlockReason)
	}
}

func TestEnhancedGeoBlocker_InvalidIPRange(t *testing.T) {
	gb := NewEnhancedGeoBlocker()

	err := gb.AddIPRange("invalid", "10.0.0.255", "US", "", "", 0, "")
	if err == nil {
		t.Error("expected error for invalid start IP")
	}

	err = gb.AddIPRange("10.0.0.255", "10.0.0.1", "US", "", "", 0, "")
	if err == nil {
		t.Error("expected error when start > end")
	}

	err = gb.AddIPRangeWithFlags("invalid", "10.0.0.255", "US", false, false, false, false)
	if err == nil {
		t.Error("expected error for invalid range with flags")
	}
}

func TestEnhancedGeoBlocker_LookupInvalidIP(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	result := gb.Lookup("invalid")
	if result.Country != "" {
		t.Error("expected empty result for invalid IP")
	}
}

func TestEnhancedGeoBlocker_IsBlocked(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AddIPRange("10.0.0.1", "10.0.0.255", "CN", "", "", 0, "")
	gb.BlockCountry("CN")

	if !gb.IsBlocked("10.0.0.50") {
		t.Error("should be blocked")
	}
	if gb.IsBlocked("20.0.0.1") {
		t.Error("unknown IP should not be blocked")
	}
}

func TestEnhancedGeoBlocker_GetBlockReason(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AddIPRange("10.0.0.1", "10.0.0.255", "CN", "", "", 0, "")
	gb.BlockCountry("CN")

	reason := gb.GetBlockReason("10.0.0.50")
	if reason != "country_blocked" {
		t.Errorf("reason = %q, want country_blocked", reason)
	}
}

func TestEnhancedGeoBlocker_LookupCountry(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AddIPRange("10.0.0.1", "10.0.0.255", "DE", "", "", 0, "")

	country := gb.LookupCountry("10.0.0.50")
	if country != "DE" {
		t.Errorf("country = %q, want DE", country)
	}
}

func TestEnhancedGeoBlocker_GetStats(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.AddIPRange("10.0.0.1", "10.0.0.255", "CN", "", "", 0, "")
	gb.BlockCountry("CN")

	// Trigger a lookup to generate stats
	gb.Lookup("10.0.0.50")

	stats := gb.GetStats()
	if stats["total_lookups"].(int64) < 1 {
		t.Error("expected at least 1 lookup in stats")
	}
	if stats["blocked_countries"].(int) != 1 {
		t.Errorf("expected 1 blocked country, got %v", stats["blocked_countries"])
	}
}

func TestEnhancedGeoBlocker_LoadBuiltinRanges(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.LoadBuiltinRanges()
	gb.SetBlockDataCenters(true)

	// AWS metadata IP should be blocked as datacenter
	result := gb.Lookup("169.254.169.254")
	if !result.IsHosting {
		t.Error("AWS metadata IP should be flagged as hosting")
	}
}

func TestEnhancedGeoBlocker_Clear(t *testing.T) {
	gb := NewEnhancedGeoBlocker()
	gb.BlockCountry("CN")
	gb.BlockContinent("AS")
	gb.BlockASN(12345)
	gb.AllowOnlyCountries([]string{"US"})

	gb.Clear()

	if len(gb.ListBlockedCountries()) != 0 {
		t.Error("blocked countries should be empty after clear")
	}
}
