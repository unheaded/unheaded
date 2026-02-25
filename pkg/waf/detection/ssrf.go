// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package detection provides SSRF detection for THE SHIELD WAF
// TODO: Marked for Rust rebuild for performance - this Go implementation serves as reference
package detection

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SSRFDetector detects Server-Side Request Forgery attacks with DNS rebinding protection
// TODO: Port to Rust with async DNS resolution and connection tracking
type SSRFDetector struct {
	patterns          []*regexp.Regexp
	privateNetworks   []*net.IPNet
	cloudMetadata     map[string]int // Cloud metadata endpoints with risk scores
	dangerousProtocols map[string]int // Dangerous protocols with risk scores
	dangerousPorts    map[int]int    // Dangerous ports with risk scores
	dnsCache          *DNSCache      // DNS resolution cache for rebinding protection
	strict            bool
}

// DNSCache provides DNS caching for rebinding attack protection
// TODO: Rust rebuild should use lock-free data structures
type DNSCache struct {
	entries map[string]*DNSCacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

// DNSCacheEntry represents a cached DNS resolution
type DNSCacheEntry struct {
	Hostname  string
	IPs       []net.IP
	Resolved  time.Time
	IsPrivate bool
	Risk      int
}

// NewDNSCache creates a new DNS cache
func NewDNSCache(ttl time.Duration) *DNSCache {
	return &DNSCache{
		entries: make(map[string]*DNSCacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves a cached DNS entry
func (c *DNSCache) Get(hostname string) *DNSCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[strings.ToLower(hostname)]
	if !ok {
		return nil
	}

	// Check TTL
	if time.Since(entry.Resolved) > c.ttl {
		return nil
	}

	return entry
}

// Set stores a DNS entry in the cache
func (c *DNSCache) Set(hostname string, ips []net.IP, isPrivate bool, risk int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[strings.ToLower(hostname)] = &DNSCacheEntry{
		Hostname:  hostname,
		IPs:       ips,
		Resolved:  time.Now(),
		IsPrivate: isPrivate,
		Risk:      risk,
	}
}

// Clear clears the DNS cache
func (c *DNSCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*DNSCacheEntry)
}

// NewSSRFDetector creates a new SSRF detector
// TODO: Rust rebuild should use Aho-Corasick for multi-pattern matching
func NewSSRFDetector(strict bool) *SSRFDetector {
	patterns := []string{
		// Localhost variations - comprehensive
		`(?i)localhost`,
		`(?i)localhost\.`,
		`(?i)localhost\.[a-z]+`,
		`127\.0\.0\.1`,
		`127\.\d{1,3}\.\d{1,3}\.\d{1,3}`,
		`0\.0\.0\.0`,
		`0x7f[0-9a-f]{6}`,          // Hex IP
		`0177\.0+\.0+\.0*1`,        // Octal IP
		`2130706433`,               // Decimal IP (127.0.0.1)
		`017700000001`,             // Octal full
		`0x7f000001`,               // Hex full

		// IPv6 loopback variations
		`::1`,
		`0:0:0:0:0:0:0:1`,
		`::ffff:127\.0\.0\.1`,
		`::ffff:7f00:0001`,
		`0000:0000:0000:0000:0000:0000:0000:0001`,

		// Private IP ranges - Class A (10.x.x.x)
		`10\.\d{1,3}\.\d{1,3}\.\d{1,3}`,
		`0xa[0-9a-f]{6}`,           // Hex 10.x.x.x

		// Private IP ranges - Class B (172.16.x.x - 172.31.x.x)
		`172\.(1[6-9]|2[0-9]|3[0-1])\.\d{1,3}\.\d{1,3}`,

		// Private IP ranges - Class C (192.168.x.x)
		`192\.168\.\d{1,3}\.\d{1,3}`,
		`0xc0a8[0-9a-f]{4}`,        // Hex 192.168.x.x

		// Link-local addresses
		`169\.254\.\d{1,3}\.\d{1,3}`,
		`fe80:[0-9a-f:]+`,

		// Cloud metadata endpoints - AWS
		`(?i)169\.254\.169\.254`,
		`(?i)instance-data`,
		`(?i)\/latest\/meta-data`,
		`(?i)\/latest\/user-data`,
		`(?i)\/latest\/dynamic`,
		`(?i)\/latest\/api\/token`,
		`(?i)iam\/security-credentials`,
		`(?i)ec2\.internal`,
		`(?i)\.compute\.internal`,

		// Cloud metadata endpoints - GCP
		`(?i)metadata\.google\.internal`,
		`(?i)metadata\.google\.com`,
		`(?i)computeMetadata\/v1`,
		`(?i)\/computeMetadata\/`,
		`(?i)metadata-flavor:\s*google`,

		// Cloud metadata endpoints - Azure
		`(?i)169\.254\.169\.254\/metadata`,
		`(?i)\/metadata\/instance`,
		`(?i)\.metadata\.azure\.com`,
		`(?i)metadata\.azure\.com`,
		`(?i)management\.azure\.com`,

		// Cloud metadata endpoints - DigitalOcean
		`(?i)169\.254\.169\.254\/metadata`,
		`(?i)droplet_id`,

		// Cloud metadata endpoints - Alibaba Cloud
		`(?i)100\.100\.100\.200`,

		// Cloud metadata endpoints - Oracle Cloud
		`(?i)169\.254\.169\.254\/opc`,

		// Kubernetes/Container orchestration
		`(?i)kubernetes\.default`,
		`(?i)\.svc\.cluster\.local`,
		`(?i)kube-system`,
		`(?i)\/api\/v1\/namespaces`,
		`(?i)kubernetes-dashboard`,
		`(?i)10\.96\.0\.1`,        // Default k8s service IP

		// Docker
		`(?i)docker\.sock`,
		`(?i)\/var\/run\/docker`,
		`(?i)unix:\/\/`,
		`(?i)docker\.internal`,
		`(?i)host\.docker\.internal`,

		// Internal hostnames
		`(?i)\.internal\b`,
		`(?i)\.local\b`,
		`(?i)\.localhost\b`,
		`(?i)\.localdomain\b`,
		`(?i)\.home\b`,
		`(?i)\.corp\b`,
		`(?i)\.lan\b`,
		`(?i)\.intranet\b`,
		`(?i)\.private\b`,
		`(?i)\.test\b`,
		`(?i)\.example\b`,
		`(?i)\.invalid\b`,

		// DNS rebinding domains
		`(?i)xip\.io`,
		`(?i)nip\.io`,
		`(?i)sslip\.io`,
		`(?i)localtest\.me`,
		`(?i)vcap\.me`,
		`(?i)lvh\.me`,
		`(?i)lacolhost\.com`,
		`(?i)yoogle\.com`,
		`(?i)127-0-0-1\.org`,
		`(?i)localhost\.direct`,

		// Dangerous protocols
		`(?i)file:\/\/`,
		`(?i)gopher:\/\/`,
		`(?i)dict:\/\/`,
		`(?i)ldap:\/\/`,
		`(?i)ldaps:\/\/`,
		`(?i)sftp:\/\/`,
		`(?i)tftp:\/\/`,
		`(?i)jar:\/\/`,
		`(?i)netdoc:\/\/`,

		// FTP with credentials
		`(?i)ftp:\/\/[^:]+:[^@]+@`,

		// URLs with credentials
		`(?i):\/\/[^:]+:[^@]+@`,

		// Common internal service ports in URLs
		`(?i):22\/`,   // SSH
		`(?i):23\/`,   // Telnet
		`(?i):25\/`,   // SMTP
		`(?i):110\/`,  // POP3
		`(?i):143\/`,  // IMAP
		`(?i):389\/`,  // LDAP
		`(?i):636\/`,  // LDAPS
		`(?i):1433\/`, // MSSQL
		`(?i):3306\/`, // MySQL
		`(?i):5432\/`, // PostgreSQL
		`(?i):6379\/`, // Redis
		`(?i):11211\/`, // Memcached
		`(?i):27017\/`, // MongoDB
		`(?i):9200\/`, // Elasticsearch
		`(?i):9300\/`, // Elasticsearch
		`(?i):2375\/`, // Docker
		`(?i):2376\/`, // Docker TLS
		`(?i):5672\/`, // RabbitMQ
		`(?i):15672\/`, // RabbitMQ Management
		`(?i):8500\/`, // Consul
		`(?i):8600\/`, // Consul DNS
		`(?i):2379\/`, // etcd
		`(?i):2380\/`, // etcd

		// Redis commands in URL
		`(?i)slaveof`,
		`(?i)config\s+set`,
		`(?i)flushall`,
		`(?i)flushdb`,

		// IPv4 in decimal format
		`\b\d{8,10}\b`, // Large decimal (could be IP)

		// Hex-encoded IPs
		`0x[0-9a-fA-F]{8}\b`,

		// Unicode encoded hosts
		`(?i)%[0-9a-f]{2}%[0-9a-f]{2}%[0-9a-f]{2}%[0-9a-f]{2}`,
	}

	// Cloud metadata endpoints with risk scores
	cloudMetadata := map[string]int{
		"169.254.169.254":           30, // AWS/Azure/GCP metadata
		"metadata.google.internal":  30,
		"metadata.google.com":       25,
		"100.100.100.200":           25, // Alibaba
		"instance-data":             20,
		"computeMetadata":           25,
	}

	// Dangerous protocols with risk scores
	dangerousProtocols := map[string]int{
		"file":    25, "gopher":  30, "dict":  25,
		"ldap":    20, "ldaps":   20, "sftp":  15,
		"tftp":    20, "jar":     15, "netdoc": 20,
		"php":     25, "phar":    25, "data":   15,
		"glob":    20, "expect":  30, "ssh2":   20,
		"rar":     15, "ogg":     10, "zip":    15,
	}

	// Dangerous ports with risk scores
	dangerousPorts := map[int]int{
		22:    15, // SSH
		23:    20, // Telnet
		25:    15, // SMTP
		110:   15, // POP3
		143:   15, // IMAP
		389:   20, // LDAP
		636:   20, // LDAPS
		1433:  25, // MSSQL
		3306:  25, // MySQL
		5432:  25, // PostgreSQL
		6379:  30, // Redis
		11211: 25, // Memcached
		27017: 25, // MongoDB
		9200:  20, // Elasticsearch
		2375:  30, // Docker
		2376:  30, // Docker TLS
		5672:  20, // RabbitMQ
		8500:  20, // Consul
		2379:  25, // etcd
		6443:  25, // Kubernetes API
		10250: 25, // Kubelet
		10255: 20, // Kubelet read-only
	}

	// Initialize private networks
	privateRanges := []string{
		"10.0.0.0/8",       // Class A private
		"172.16.0.0/12",    // Class B private
		"192.168.0.0/16",   // Class C private
		"127.0.0.0/8",      // Loopback
		"169.254.0.0/16",   // Link-local
		"0.0.0.0/8",        // Current network
		"100.64.0.0/10",    // Carrier-grade NAT
		"192.0.0.0/24",     // IETF Protocol
		"192.0.2.0/24",     // TEST-NET-1
		"198.51.100.0/24",  // TEST-NET-2
		"203.0.113.0/24",   // TEST-NET-3
		"224.0.0.0/4",      // Multicast
		"240.0.0.0/4",      // Reserved
		"255.255.255.255/32", // Broadcast
	}

	detector := &SSRFDetector{
		patterns:           make([]*regexp.Regexp, 0, len(patterns)),
		privateNetworks:    make([]*net.IPNet, 0),
		cloudMetadata:      cloudMetadata,
		dangerousProtocols: dangerousProtocols,
		dangerousPorts:     dangerousPorts,
		dnsCache:           NewDNSCache(5 * time.Minute),
		strict:             strict,
	}

	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err == nil {
			detector.patterns = append(detector.patterns, re)
		}
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			detector.privateNetworks = append(detector.privateNetworks, network)
		}
	}

	return detector
}

// Detect checks if the input contains SSRF attempts
func (d *SSRFDetector) Detect(input string) bool {
	normalized := d.normalize(input)

	// Pattern matching
	for _, pattern := range d.patterns {
		if pattern.MatchString(normalized) {
			return true
		}
	}

	// Check for private IPs
	if d.containsPrivateIP(normalized) {
		return true
	}

	// Parse as URL and check
	if d.isSSRFURL(input) {
		return true
	}

	return false
}

// DetectWithDetails returns detailed detection results
// TODO: Rust version should include async DNS resolution
func (d *SSRFDetector) DetectWithDetails(input string) *DetectionResult {
	result := &DetectionResult{
		Type:    "ssrf",
		Input:   input,
		Matches: make([]string, 0),
	}

	normalized := d.normalize(input)

	// Pattern matching
	for _, pattern := range d.patterns {
		matches := pattern.FindAllString(normalized, -1)
		if len(matches) > 0 {
			result.Detected = true
			result.Matches = append(result.Matches, matches...)
		}
	}

	// Private IP analysis
	privateIPs := d.extractPrivateIPs(normalized)
	if len(privateIPs) > 0 {
		result.Detected = true
		result.Matches = append(result.Matches, privateIPs...)
	}

	// URL analysis
	urlRisk, urlMatches := d.analyzeURL(input)
	result.Matches = append(result.Matches, urlMatches...)

	// Protocol analysis
	protocolRisk := d.analyzeProtocol(input)

	// DNS rebinding check
	rebindRisk := d.checkDNSRebinding(input)

	if urlRisk >= 15 || protocolRisk >= 10 || rebindRisk >= 10 {
		result.Detected = true
	}

	if result.Detected {
		result.Score = d.calculateScore(len(result.Matches), urlRisk, protocolRisk, rebindRisk)
		result.Message = "Server-side request forgery attempt detected"
	}

	return result
}

// containsPrivateIP checks if the input contains private IP addresses
func (d *SSRFDetector) containsPrivateIP(input string) bool {
	ipRegex := regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
	matches := ipRegex.FindAllString(input, -1)

	for _, ipStr := range matches {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}

		for _, network := range d.privateNetworks {
			if network.Contains(ip) {
				return true
			}
		}
	}

	return false
}

// extractPrivateIPs extracts all private IPs from the input
func (d *SSRFDetector) extractPrivateIPs(input string) []string {
	var privateIPs []string

	ipRegex := regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
	matches := ipRegex.FindAllString(input, -1)

	for _, ipStr := range matches {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}

		for _, network := range d.privateNetworks {
			if network.Contains(ip) {
				privateIPs = append(privateIPs, ipStr)
				break
			}
		}
	}

	return privateIPs
}

// analyzeURL analyzes a URL for SSRF indicators
func (d *SSRFDetector) analyzeURL(input string) (int, []string) {
	risk := 0
	var matches []string

	// Try to parse as URL
	parsedURL, err := url.Parse(input)
	if err != nil {
		// Try with scheme prefix
		parsedURL, err = url.Parse("http://" + input)
		if err != nil {
			return risk, matches
		}
	}

	// Check scheme
	scheme := strings.ToLower(parsedURL.Scheme)
	if score, ok := d.dangerousProtocols[scheme]; ok {
		risk += score
		matches = append(matches, "protocol:"+scheme)
	}

	// Check host
	host := parsedURL.Hostname()
	if host != "" {
		// Check if it's a private IP
		if ip := net.ParseIP(host); ip != nil {
			for _, network := range d.privateNetworks {
				if network.Contains(ip) {
					risk += 20
					matches = append(matches, "private_ip:"+host)
					break
				}
			}
		}

		// Check for cloud metadata
		for endpoint, score := range d.cloudMetadata {
			if strings.Contains(strings.ToLower(host), strings.ToLower(endpoint)) {
				risk += score
				matches = append(matches, "cloud_metadata:"+endpoint)
			}
		}

		// Check for internal hostnames
		internalPatterns := []string{".internal", ".local", ".localhost", ".lan", ".corp"}
		for _, pattern := range internalPatterns {
			if strings.HasSuffix(strings.ToLower(host), pattern) {
				risk += 15
				matches = append(matches, "internal_host:"+host)
				break
			}
		}
	}

	// Check port
	port := parsedURL.Port()
	if port != "" {
		portNum, err := strconv.Atoi(port)
		if err == nil {
			if score, ok := d.dangerousPorts[portNum]; ok {
				risk += score
				matches = append(matches, "dangerous_port:"+port)
			}
		}
	}

	// Check for credentials in URL
	if parsedURL.User != nil {
		risk += 15
		matches = append(matches, "url_credentials")
	}

	// Check path for metadata endpoints
	path := strings.ToLower(parsedURL.Path)
	metadataPaths := []string{"/latest/meta-data", "/latest/user-data", "/computeMetadata", "/metadata/instance"}
	for _, mp := range metadataPaths {
		if strings.Contains(path, mp) {
			risk += 25
			matches = append(matches, "metadata_path:"+mp)
		}
	}

	return risk, matches
}

// analyzeProtocol checks for dangerous protocols
func (d *SSRFDetector) analyzeProtocol(input string) int {
	risk := 0
	lower := strings.ToLower(input)

	for protocol, score := range d.dangerousProtocols {
		if strings.Contains(lower, protocol+"://") || strings.Contains(lower, protocol+":") {
			risk += score
		}
	}

	return risk
}

// checkDNSRebinding checks for potential DNS rebinding indicators
func (d *SSRFDetector) checkDNSRebinding(input string) int {
	risk := 0
	lower := strings.ToLower(input)

	// Known rebinding domains
	rebindingDomains := []string{
		"xip.io", "nip.io", "sslip.io", "localtest.me",
		"vcap.me", "lvh.me", "lacolhost.com", "127-0-0-1.org",
		"localhost.direct",
	}

	for _, domain := range rebindingDomains {
		if strings.Contains(lower, domain) {
			risk += 25
		}
	}

	// Check for IP-like patterns in hostname
	// e.g., 127.0.0.1.xip.io, 10-10-10-10.nip.io
	ipInHostPattern := regexp.MustCompile(`\d{1,3}[\.-]\d{1,3}[\.-]\d{1,3}[\.-]\d{1,3}`)
	if ipInHostPattern.MatchString(lower) {
		// Extract and check if it's a private IP
		matches := ipInHostPattern.FindAllString(lower, -1)
		for _, match := range matches {
			// Convert dashes to dots
			normalized := strings.ReplaceAll(match, "-", ".")
			if ip := net.ParseIP(normalized); ip != nil {
				for _, network := range d.privateNetworks {
					if network.Contains(ip) {
						risk += 20
						break
					}
				}
			}
		}
	}

	return risk
}

// normalize normalizes the input for detection
func (d *SSRFDetector) normalize(input string) string {
	result := input

	// URL decode multiple passes
	for i := 0; i < 3; i++ {
		prev := result
		result = strings.ReplaceAll(result, "%3A", ":")
		result = strings.ReplaceAll(result, "%2F", "/")
		result = strings.ReplaceAll(result, "%40", "@")
		result = strings.ReplaceAll(result, "%2E", ".")
		result = strings.ReplaceAll(result, "%3D", "=")
		result = strings.ReplaceAll(result, "%26", "&")
		result = strings.ReplaceAll(result, "%3F", "?")
		result = strings.ReplaceAll(result, "%23", "#")
		result = strings.ReplaceAll(result, "%25", "%")
		if result == prev {
			break
		}
	}

	return result
}

// calculateScore calculates the anomaly score
func (d *SSRFDetector) calculateScore(matchCount, urlRisk, protocolRisk, rebindRisk int) int {
	baseScore := 15

	if matchCount > 0 {
		baseScore += matchCount * 3
	}

	baseScore += urlRisk / 2
	baseScore += protocolRisk
	baseScore += rebindRisk

	if baseScore > 100 {
		return 100
	}

	return baseScore
}

// GetPatternCount returns the number of patterns
func (d *SSRFDetector) GetPatternCount() int {
	return len(d.patterns)
}

// IsPrivateIP checks if an IP address is private
func (d *SSRFDetector) IsPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	for _, network := range d.privateNetworks {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// isSSRFURL checks if a URL is potentially SSRF
func (d *SSRFDetector) isSSRFURL(input string) bool {
	risk, _ := d.analyzeURL(input)
	return risk >= 15
}

// ValidateURL performs SSRF validation on a URL
// Returns (isValid, reason)
func (d *SSRFDetector) ValidateURL(rawURL string) (bool, string) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false, "invalid URL format"
	}

	// Check scheme
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return false, "only HTTP/HTTPS schemes allowed"
	}

	host := parsedURL.Hostname()
	if host == "" {
		return false, "empty hostname"
	}

	// Check for localhost
	if strings.ToLower(host) == "localhost" || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return false, "localhost not allowed"
	}

	// Check if it's an IP address
	if ip := net.ParseIP(host); ip != nil {
		for _, network := range d.privateNetworks {
			if network.Contains(ip) {
				return false, "private IP address not allowed"
			}
		}
	}

	// Check for internal hostnames
	internalSuffixes := []string{".internal", ".local", ".lan", ".corp", ".intranet"}
	for _, suffix := range internalSuffixes {
		if strings.HasSuffix(strings.ToLower(host), suffix) {
			return false, "internal hostname not allowed"
		}
	}

	// Check for cloud metadata endpoints
	for endpoint := range d.cloudMetadata {
		if strings.Contains(strings.ToLower(host), strings.ToLower(endpoint)) {
			return false, "cloud metadata endpoint not allowed"
		}
	}

	// Check port
	port := parsedURL.Port()
	if port != "" {
		portNum, err := strconv.Atoi(port)
		if err == nil {
			if _, isDangerous := d.dangerousPorts[portNum]; isDangerous {
				return false, "dangerous port not allowed"
			}
		}
	}

	return true, ""
}

// ResolveAndValidate resolves a hostname and validates the IP
// This provides DNS rebinding protection by checking resolved IPs
// TODO: Rust version should use async DNS resolution
func (d *SSRFDetector) ResolveAndValidate(hostname string) ([]net.IP, bool, string) {
	// Check cache first
	if cached := d.dnsCache.Get(hostname); cached != nil {
		if cached.IsPrivate {
			return cached.IPs, false, "resolves to private IP (cached)"
		}
		return cached.IPs, true, ""
	}

	// Resolve hostname
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return nil, false, "DNS resolution failed: " + err.Error()
	}

	// Check all resolved IPs
	for _, ip := range ips {
		for _, network := range d.privateNetworks {
			if network.Contains(ip) {
				d.dnsCache.Set(hostname, ips, true, 25)
				return ips, false, "resolves to private IP: " + ip.String()
			}
		}
	}

	// Cache the result
	d.dnsCache.Set(hostname, ips, false, 0)

	return ips, true, ""
}

// ParseIPFromDecimal parses an IP from decimal format (e.g., 2130706433 = 127.0.0.1)
func ParseIPFromDecimal(decimal string) net.IP {
	num, err := strconv.ParseUint(decimal, 10, 32)
	if err != nil {
		return nil
	}

	return net.IPv4(
		byte(num>>24),
		byte(num>>16),
		byte(num>>8),
		byte(num),
	)
}

// ParseIPFromHex parses an IP from hex format (e.g., 0x7f000001 = 127.0.0.1)
func ParseIPFromHex(hex string) net.IP {
	// Remove 0x prefix if present
	hex = strings.TrimPrefix(strings.ToLower(hex), "0x")

	num, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return nil
	}

	return net.IPv4(
		byte(num>>24),
		byte(num>>16),
		byte(num>>8),
		byte(num),
	)
}

// ParseIPFromOctal parses an IP from octal format (e.g., 0177.0.0.1 = 127.0.0.1)
func ParseIPFromOctal(octal string) net.IP {
	parts := strings.Split(octal, ".")
	if len(parts) != 4 {
		return nil
	}

	var ip [4]byte
	for i, part := range parts {
		// Parse each octet, handling octal if it starts with 0
		var base int = 10
		if strings.HasPrefix(part, "0") && len(part) > 1 {
			base = 8
		}
		num, err := strconv.ParseUint(part, base, 8)
		if err != nil {
			return nil
		}
		ip[i] = byte(num)
	}

	return net.IPv4(ip[0], ip[1], ip[2], ip[3])
}

// NormalizeIP normalizes various IP formats to standard dotted decimal
func NormalizeIP(input string) net.IP {
	// Try standard format first
	if ip := net.ParseIP(input); ip != nil {
		return ip
	}

	// Try decimal format
	if ip := ParseIPFromDecimal(input); ip != nil {
		return ip
	}

	// Try hex format
	if ip := ParseIPFromHex(input); ip != nil {
		return ip
	}

	// Try octal format
	if ip := ParseIPFromOctal(input); ip != nil {
		return ip
	}

	return nil
}

// IsCloudMetadata checks if the host is a cloud metadata endpoint
func (d *SSRFDetector) IsCloudMetadata(host string) bool {
	lower := strings.ToLower(host)
	for endpoint := range d.cloudMetadata {
		if strings.Contains(lower, strings.ToLower(endpoint)) {
			return true
		}
	}
	return false
}

// GetDangerousPortInfo returns information about a dangerous port
func (d *SSRFDetector) GetDangerousPortInfo(port int) (bool, string) {
	portServices := map[int]string{
		22:    "SSH", 23: "Telnet", 25: "SMTP", 110: "POP3",
		143:   "IMAP", 389: "LDAP", 636: "LDAPS", 1433: "MSSQL",
		3306:  "MySQL", 5432: "PostgreSQL", 6379: "Redis",
		11211: "Memcached", 27017: "MongoDB", 9200: "Elasticsearch",
		2375:  "Docker", 2376: "Docker TLS", 5672: "RabbitMQ",
		8500:  "Consul", 2379: "etcd", 6443: "Kubernetes API",
		10250: "Kubelet", 10255: "Kubelet Read-only",
	}

	_, isDangerous := d.dangerousPorts[port]
	service := portServices[port]
	if service == "" {
		service = "unknown"
	}

	return isDangerous, service
}

// ClearDNSCache clears the DNS resolution cache
func (d *SSRFDetector) ClearDNSCache() {
	d.dnsCache.Clear()
}

// SSRFURLChecker provides a simple URL checking interface
type SSRFURLChecker struct {
	detector *SSRFDetector
}

// NewSSRFURLChecker creates a new URL checker
func NewSSRFURLChecker() *SSRFURLChecker {
	return &SSRFURLChecker{
		detector: NewSSRFDetector(true),
	}
}

// IsSafe checks if a URL is safe from SSRF perspective
func (c *SSRFURLChecker) IsSafe(rawURL string) (bool, string) {
	return c.detector.ValidateURL(rawURL)
}

// IsSafeWithDNS checks URL safety including DNS resolution
func (c *SSRFURLChecker) IsSafeWithDNS(rawURL string) (bool, string) {
	// First validate URL structure
	valid, reason := c.detector.ValidateURL(rawURL)
	if !valid {
		return false, reason
	}

	// Parse URL to get hostname
	parsedURL, _ := url.Parse(rawURL)
	host := parsedURL.Hostname()

	// If it's not an IP, resolve and validate
	if net.ParseIP(host) == nil {
		_, valid, reason := c.detector.ResolveAndValidate(host)
		if !valid {
			return false, reason
		}
	}

	return true, ""
}
