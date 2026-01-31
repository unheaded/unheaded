// Package ip provides IP-based WAF functionality
package ip

import (
	"net"
	"net/http"
	"strings"
	"sync"
)

// IPManager manages IP-based access control
type IPManager struct {
	blocklist    *Blocklist
	allowlist    *Allowlist
	geoBlocker   *GeoBlocker
	trustProxies bool
	proxyHeaders []string
	mu           sync.RWMutex
}

// NewIPManager creates a new IP manager
func NewIPManager() *IPManager {
	return &IPManager{
		blocklist:    NewBlocklist(),
		allowlist:    NewAllowlist(),
		geoBlocker:   NewGeoBlocker(),
		trustProxies: true,
		proxyHeaders: []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP"},
	}
}

// SetTrustProxies sets whether to trust proxy headers
func (m *IPManager) SetTrustProxies(trust bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trustProxies = trust
}

// SetProxyHeaders sets the proxy headers to check
func (m *IPManager) SetProxyHeaders(headers []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxyHeaders = headers
}

// GetClientIP extracts the client IP from a request
func (m *IPManager) GetClientIP(r *http.Request) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.trustProxies {
		for _, header := range m.proxyHeaders {
			if value := r.Header.Get(header); value != "" {
				// X-Forwarded-For can contain multiple IPs
				if header == "X-Forwarded-For" {
					parts := strings.Split(value, ",")
					if len(parts) > 0 {
						ip := strings.TrimSpace(parts[0])
						if net.ParseIP(ip) != nil {
							return ip
						}
					}
				} else {
					ip := strings.TrimSpace(value)
					if net.ParseIP(ip) != nil {
						return ip
					}
				}
			}
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// IsAllowed checks if an IP is allowed
func (m *IPManager) IsAllowed(ip string) bool {
	// Check allowlist first (explicit allow)
	if m.allowlist.Contains(ip) {
		return true
	}

	// Check blocklist
	if m.blocklist.Contains(ip) {
		return false
	}

	// Check geo blocking
	if m.geoBlocker.IsBlocked(ip) {
		return false
	}

	return true
}

// CheckRequest checks if a request's IP is allowed
func (m *IPManager) CheckRequest(r *http.Request) (bool, string) {
	ip := m.GetClientIP(r)

	if m.allowlist.Contains(ip) {
		return true, ""
	}

	if m.blocklist.Contains(ip) {
		return false, "IP is blocklisted"
	}

	if m.geoBlocker.IsBlocked(ip) {
		return false, "IP is geo-blocked"
	}

	return true, ""
}

// Blocklist returns the blocklist
func (m *IPManager) Blocklist() *Blocklist {
	return m.blocklist
}

// Allowlist returns the allowlist
func (m *IPManager) Allowlist() *Allowlist {
	return m.allowlist
}

// GeoBlocker returns the geo blocker
func (m *IPManager) GeoBlocker() *GeoBlocker {
	return m.geoBlocker
}

// ParseCIDR parses a CIDR string and returns the network
func ParseCIDR(cidr string) (*net.IPNet, error) {
	_, network, err := net.ParseCIDR(cidr)
	return network, err
}

// IsPrivateIP checks if an IP is private
func IsPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"fc00::/7",
		"fe80::/10",
		"::1/128",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsedIP) {
			return true
		}
	}

	return false
}

// NormalizeIP normalizes an IP address
func NormalizeIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	return parsed.String()
}

// IPRange represents a range of IPs
type IPRange struct {
	Start net.IP
	End   net.IP
}

// Contains checks if an IP is within the range
func (r *IPRange) Contains(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}

	// Convert to 16-byte representation for comparison
	parsed = parsed.To16()
	start := r.Start.To16()
	end := r.End.To16()

	return bytesCompare(parsed, start) >= 0 && bytesCompare(parsed, end) <= 0
}

// bytesCompare compares two IP addresses as byte slices
func bytesCompare(a, b net.IP) int {
	if len(a) != len(b) {
		return len(a) - len(b)
	}
	for i := 0; i < len(a); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// NewIPRange creates a new IP range
func NewIPRange(start, end string) (*IPRange, error) {
	startIP := net.ParseIP(start)
	if startIP == nil {
		return nil, &net.ParseError{Type: "IP address", Text: start}
	}

	endIP := net.ParseIP(end)
	if endIP == nil {
		return nil, &net.ParseError{Type: "IP address", Text: end}
	}

	return &IPRange{Start: startIP, End: endIP}, nil
}
