// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package ip provides enhanced geo-blocking functionality
package ip

import (
	"encoding/binary"
	"net"
	"sort"
	"sync"
	"time"
)

// EnhancedGeoBlocker provides comprehensive geo-blocking with efficient IP lookup
type EnhancedGeoBlocker struct {
	// Country blocking
	blockedCountries map[string]bool
	allowedCountries map[string]bool // If set, only these countries are allowed
	useAllowList     bool

	// IP range storage using a sorted list for binary search
	ipRanges     []IPRange
	rangesSorted bool

	// Continent-based blocking
	blockedContinents map[string]bool
	continentMap      map[string]string // Country code -> Continent code

	// ASN-based blocking
	blockedASNs map[uint32]bool
	asnRanges   []ASNRange

	// Region/state blocking (for specific countries)
	blockedRegions map[string]map[string]bool // country -> region -> blocked

	// Statistics
	stats *GeoStats

	// Configuration
	blockAnonymizers bool // Block known Tor, VPN, proxy ranges
	blockDataCenters bool // Block known datacenter IPs

	mu sync.RWMutex
}

// IPRange represents an IP range with associated country
type IPRange struct {
	StartIP   uint32
	EndIP     uint32
	Country   string
	Region    string
	City      string
	ASN       uint32
	ISP       string
	IsProxy   bool
	IsVPN     bool
	IsTor     bool
	IsHosting bool
}

// ASNRange represents an ASN (Autonomous System Number) range
type ASNRange struct {
	StartIP uint32
	EndIP   uint32
	ASN     uint32
	Name    string
}

// GeoStats holds geo-blocking statistics
type GeoStats struct {
	TotalLookups     int64
	CacheMisses      int64
	CountryBlocks    map[string]int64
	ContinentBlocks  map[string]int64
	AnonymizerBlocks int64
	DataCenterBlocks int64
	LastUpdated      time.Time
	mu               sync.RWMutex
}

// GeoLookupResult represents the result of a geo lookup
type GeoLookupResult struct {
	Country     string
	Region      string
	City        string
	Continent   string
	ASN         uint32
	ISP         string
	IsProxy     bool
	IsVPN       bool
	IsTor       bool
	IsHosting   bool
	IsBlocked   bool
	BlockReason string
}

// NewEnhancedGeoBlocker creates a new enhanced geo blocker
func NewEnhancedGeoBlocker() *EnhancedGeoBlocker {
	return &EnhancedGeoBlocker{
		blockedCountries:  make(map[string]bool),
		allowedCountries:  make(map[string]bool),
		useAllowList:      false,
		ipRanges:          make([]IPRange, 0),
		rangesSorted:      true,
		blockedContinents: make(map[string]bool),
		continentMap:      buildContinentMap(),
		blockedASNs:       make(map[uint32]bool),
		asnRanges:         make([]ASNRange, 0),
		blockedRegions:    make(map[string]map[string]bool),
		stats:             NewGeoStats(),
		blockAnonymizers:  false,
		blockDataCenters:  false,
	}
}

// NewGeoStats creates new geo statistics
func NewGeoStats() *GeoStats {
	return &GeoStats{
		CountryBlocks:   make(map[string]int64),
		ContinentBlocks: make(map[string]int64),
		LastUpdated:     time.Now(),
	}
}

// BlockCountry blocks a country by ISO code
func (gb *EnhancedGeoBlocker) BlockCountry(countryCode string) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	gb.blockedCountries[toUpperASCII(countryCode)] = true
}

// UnblockCountry removes a country from the blocklist
func (gb *EnhancedGeoBlocker) UnblockCountry(countryCode string) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	delete(gb.blockedCountries, toUpperASCII(countryCode))
}

// AllowOnlyCountries sets up an allowlist mode
func (gb *EnhancedGeoBlocker) AllowOnlyCountries(countryCodes []string) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	gb.useAllowList = true
	gb.allowedCountries = make(map[string]bool)
	for _, code := range countryCodes {
		gb.allowedCountries[toUpperASCII(code)] = true
	}
}

// DisableAllowList disables allowlist mode
func (gb *EnhancedGeoBlocker) DisableAllowList() {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	gb.useAllowList = false
}

// BlockContinent blocks an entire continent
func (gb *EnhancedGeoBlocker) BlockContinent(continentCode string) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	gb.blockedContinents[toUpperASCII(continentCode)] = true
}

// UnblockContinent removes a continent from the blocklist
func (gb *EnhancedGeoBlocker) UnblockContinent(continentCode string) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	delete(gb.blockedContinents, toUpperASCII(continentCode))
}

// BlockASN blocks an Autonomous System Number
func (gb *EnhancedGeoBlocker) BlockASN(asn uint32) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	gb.blockedASNs[asn] = true
}

// UnblockASN removes an ASN from the blocklist
func (gb *EnhancedGeoBlocker) UnblockASN(asn uint32) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	delete(gb.blockedASNs, asn)
}

// BlockRegion blocks a specific region within a country
func (gb *EnhancedGeoBlocker) BlockRegion(countryCode, regionCode string) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	country := toUpperASCII(countryCode)
	if gb.blockedRegions[country] == nil {
		gb.blockedRegions[country] = make(map[string]bool)
	}
	gb.blockedRegions[country][toUpperASCII(regionCode)] = true
}

// SetBlockAnonymizers enables/disables anonymizer blocking
func (gb *EnhancedGeoBlocker) SetBlockAnonymizers(block bool) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	gb.blockAnonymizers = block
}

// SetBlockDataCenters enables/disables datacenter blocking
func (gb *EnhancedGeoBlocker) SetBlockDataCenters(block bool) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	gb.blockDataCenters = block
}

// AddIPRange adds an IP range to the database
func (gb *EnhancedGeoBlocker) AddIPRange(startIP, endIP, country, region, city string, asn uint32, isp string) error {
	start := ipToUint32(net.ParseIP(startIP))
	end := ipToUint32(net.ParseIP(endIP))

	if start == 0 || end == 0 || start > end {
		return errorf("invalid IP range: %s - %s", startIP, endIP)
	}

	gb.mu.Lock()
	defer gb.mu.Unlock()

	gb.ipRanges = append(gb.ipRanges, IPRange{
		StartIP: start,
		EndIP:   end,
		Country: toUpperASCII(country),
		Region:  region,
		City:    city,
		ASN:     asn,
		ISP:     isp,
	})
	gb.rangesSorted = false

	return nil
}

// AddIPRangeWithFlags adds an IP range with additional flags
func (gb *EnhancedGeoBlocker) AddIPRangeWithFlags(startIP, endIP, country string, isProxy, isVPN, isTor, isHosting bool) error {
	start := ipToUint32(net.ParseIP(startIP))
	end := ipToUint32(net.ParseIP(endIP))

	if start == 0 || end == 0 || start > end {
		return errorf("invalid IP range: %s - %s", startIP, endIP)
	}

	gb.mu.Lock()
	defer gb.mu.Unlock()

	gb.ipRanges = append(gb.ipRanges, IPRange{
		StartIP:   start,
		EndIP:     end,
		Country:   toUpperASCII(country),
		IsProxy:   isProxy,
		IsVPN:     isVPN,
		IsTor:     isTor,
		IsHosting: isHosting,
	})
	gb.rangesSorted = false

	return nil
}

// sortRanges sorts IP ranges for binary search
func (gb *EnhancedGeoBlocker) sortRanges() {
	if gb.rangesSorted {
		return
	}

	sort.Slice(gb.ipRanges, func(i, j int) bool {
		return gb.ipRanges[i].StartIP < gb.ipRanges[j].StartIP
	})
	gb.rangesSorted = true
}

// Lookup performs a complete geo lookup for an IP
func (gb *EnhancedGeoBlocker) Lookup(ip string) *GeoLookupResult {
	gb.stats.mu.Lock()
	gb.stats.TotalLookups++
	gb.stats.mu.Unlock()

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return &GeoLookupResult{}
	}

	ipNum := ipToUint32(parsedIP)
	if ipNum == 0 {
		return &GeoLookupResult{}
	}

	gb.mu.RLock()
	defer gb.mu.RUnlock()

	// Find the IP range using binary search
	gb.sortRanges()
	ipRange := gb.findIPRange(ipNum)

	result := &GeoLookupResult{}

	if ipRange != nil {
		result.Country = ipRange.Country
		result.Region = ipRange.Region
		result.City = ipRange.City
		result.ASN = ipRange.ASN
		result.ISP = ipRange.ISP
		result.IsProxy = ipRange.IsProxy
		result.IsVPN = ipRange.IsVPN
		result.IsTor = ipRange.IsTor
		result.IsHosting = ipRange.IsHosting

		// Get continent from country
		if continent, ok := gb.continentMap[result.Country]; ok {
			result.Continent = continent
		}
	}

	// Check if blocked
	result.IsBlocked, result.BlockReason = gb.checkBlocked(result, ipNum)

	// Update statistics
	if result.IsBlocked {
		gb.stats.mu.Lock()
		if result.Country != "" {
			gb.stats.CountryBlocks[result.Country]++
		}
		if result.Continent != "" {
			gb.stats.ContinentBlocks[result.Continent]++
		}
		gb.stats.mu.Unlock()
	}

	return result
}

// findIPRange finds the IP range for a given IP using binary search
func (gb *EnhancedGeoBlocker) findIPRange(ipNum uint32) *IPRange {
	left := 0
	right := len(gb.ipRanges) - 1

	for left <= right {
		mid := (left + right) / 2
		r := &gb.ipRanges[mid]

		if ipNum >= r.StartIP && ipNum <= r.EndIP {
			return r
		}

		if ipNum < r.StartIP {
			right = mid - 1
		} else {
			left = mid + 1
		}
	}

	return nil
}

// checkBlocked checks if the lookup result should be blocked
func (gb *EnhancedGeoBlocker) checkBlocked(result *GeoLookupResult, ipNum uint32) (bool, string) {
	// Check allowlist mode first
	if gb.useAllowList {
		if result.Country != "" && !gb.allowedCountries[result.Country] {
			return true, "country_not_in_allowlist"
		}
	}

	// Check blocked countries
	if result.Country != "" && gb.blockedCountries[result.Country] {
		return true, "country_blocked"
	}

	// Check blocked continents
	if result.Continent != "" && gb.blockedContinents[result.Continent] {
		return true, "continent_blocked"
	}

	// Check blocked regions
	if result.Country != "" && result.Region != "" {
		if regions, ok := gb.blockedRegions[result.Country]; ok {
			if regions[result.Region] {
				return true, "region_blocked"
			}
		}
	}

	// Check blocked ASNs
	if result.ASN != 0 && gb.blockedASNs[result.ASN] {
		return true, "asn_blocked"
	}

	// Check anonymizers
	if gb.blockAnonymizers {
		if result.IsProxy {
			gb.stats.mu.Lock()
			gb.stats.AnonymizerBlocks++
			gb.stats.mu.Unlock()
			return true, "proxy_blocked"
		}
		if result.IsVPN {
			gb.stats.mu.Lock()
			gb.stats.AnonymizerBlocks++
			gb.stats.mu.Unlock()
			return true, "vpn_blocked"
		}
		if result.IsTor {
			gb.stats.mu.Lock()
			gb.stats.AnonymizerBlocks++
			gb.stats.mu.Unlock()
			return true, "tor_blocked"
		}
	}

	// Check data centers
	if gb.blockDataCenters && result.IsHosting {
		gb.stats.mu.Lock()
		gb.stats.DataCenterBlocks++
		gb.stats.mu.Unlock()
		return true, "datacenter_blocked"
	}

	return false, ""
}

// IsBlocked checks if an IP is blocked
func (gb *EnhancedGeoBlocker) IsBlocked(ip string) bool {
	result := gb.Lookup(ip)
	return result.IsBlocked
}

// GetBlockReason returns the block reason for an IP
func (gb *EnhancedGeoBlocker) GetBlockReason(ip string) string {
	result := gb.Lookup(ip)
	return result.BlockReason
}

// LookupCountry returns just the country code for an IP
func (gb *EnhancedGeoBlocker) LookupCountry(ip string) string {
	result := gb.Lookup(ip)
	return result.Country
}

// GetStats returns geo-blocking statistics
func (gb *EnhancedGeoBlocker) GetStats() map[string]interface{} {
	gb.stats.mu.RLock()
	defer gb.stats.mu.RUnlock()

	gb.mu.RLock()
	defer gb.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_lookups"] = gb.stats.TotalLookups
	stats["cache_misses"] = gb.stats.CacheMisses
	stats["anonymizer_blocks"] = gb.stats.AnonymizerBlocks
	stats["datacenter_blocks"] = gb.stats.DataCenterBlocks
	stats["blocked_countries"] = len(gb.blockedCountries)
	stats["blocked_continents"] = len(gb.blockedContinents)
	stats["blocked_asns"] = len(gb.blockedASNs)
	stats["ip_ranges"] = len(gb.ipRanges)
	stats["use_allowlist"] = gb.useAllowList
	stats["block_anonymizers"] = gb.blockAnonymizers
	stats["block_datacenters"] = gb.blockDataCenters

	// Copy country blocks
	countryBlocks := make(map[string]int64)
	for k, v := range gb.stats.CountryBlocks {
		countryBlocks[k] = v
	}
	stats["country_blocks"] = countryBlocks

	return stats
}

// ListBlockedCountries returns all blocked country codes
func (gb *EnhancedGeoBlocker) ListBlockedCountries() []string {
	gb.mu.RLock()
	defer gb.mu.RUnlock()

	countries := make([]string, 0, len(gb.blockedCountries))
	for country := range gb.blockedCountries {
		countries = append(countries, country)
	}
	return countries
}

// ListAllowedCountries returns all allowed country codes (if in allowlist mode)
func (gb *EnhancedGeoBlocker) ListAllowedCountries() []string {
	gb.mu.RLock()
	defer gb.mu.RUnlock()

	countries := make([]string, 0, len(gb.allowedCountries))
	for country := range gb.allowedCountries {
		countries = append(countries, country)
	}
	return countries
}

// LoadBuiltinRanges loads some built-in IP ranges for common use cases
func (gb *EnhancedGeoBlocker) LoadBuiltinRanges() {
	gb.mu.Lock()
	defer gb.mu.Unlock()

	// AWS metadata IP (SSRF protection)
	gb.ipRanges = append(gb.ipRanges, IPRange{
		StartIP:   ipToUint32(net.ParseIP("169.254.169.254")),
		EndIP:     ipToUint32(net.ParseIP("169.254.169.254")),
		Country:   "INTERNAL",
		ISP:       "AWS Metadata",
		IsHosting: true,
	})

	// Azure metadata
	gb.ipRanges = append(gb.ipRanges, IPRange{
		StartIP:   ipToUint32(net.ParseIP("169.254.169.254")),
		EndIP:     ipToUint32(net.ParseIP("169.254.169.254")),
		Country:   "INTERNAL",
		ISP:       "Azure Metadata",
		IsHosting: true,
	})

	// GCP metadata
	gb.ipRanges = append(gb.ipRanges, IPRange{
		StartIP:   ipToUint32(net.ParseIP("169.254.169.254")),
		EndIP:     ipToUint32(net.ParseIP("169.254.169.254")),
		Country:   "INTERNAL",
		ISP:       "GCP Metadata",
		IsHosting: true,
	})

	// Common Tor exit ranges (partial list for demonstration)
	torRanges := []struct {
		start, end string
	}{
		{"185.220.100.0", "185.220.103.255"},
		{"185.220.101.0", "185.220.101.255"},
	}

	for _, r := range torRanges {
		gb.ipRanges = append(gb.ipRanges, IPRange{
			StartIP: ipToUint32(net.ParseIP(r.start)),
			EndIP:   ipToUint32(net.ParseIP(r.end)),
			Country: "TOR",
			ISP:     "Tor Network",
			IsTor:   true,
		})
	}

	gb.rangesSorted = false
}

// Clear removes all blocking rules
func (gb *EnhancedGeoBlocker) Clear() {
	gb.mu.Lock()
	defer gb.mu.Unlock()

	gb.blockedCountries = make(map[string]bool)
	gb.allowedCountries = make(map[string]bool)
	gb.blockedContinents = make(map[string]bool)
	gb.blockedASNs = make(map[uint32]bool)
	gb.blockedRegions = make(map[string]map[string]bool)
	gb.useAllowList = false
}

// Helper functions

func ipToUint32(ip net.IP) uint32 {
	if ip == nil {
		return 0
	}
	// Convert to IPv4 if possible
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip4)
}

func toUpperASCII(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= 'a' && ch <= 'z' {
			result[i] = ch - 32
		} else {
			result[i] = ch
		}
	}
	return string(result)
}

func errorf(format string, args ...interface{}) error {
	return &geoError{msg: sprintf(format, args...)}
}

type geoError struct {
	msg string
}

func (e *geoError) Error() string {
	return e.msg
}

func sprintf(format string, args ...interface{}) string {
	if len(args) == 0 {
		return format
	}
	// Simple sprintf implementation for basic cases
	result := format
	for _, arg := range args {
		idx := indexOf(result, "%s")
		if idx >= 0 {
			result = result[:idx] + stringValue(arg) + result[idx+2:]
		}
	}
	return result
}

func stringValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return "<value>"
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// buildContinentMap returns a map of country codes to continent codes
func buildContinentMap() map[string]string {
	return map[string]string{
		// Africa
		"DZ": "AF", "AO": "AF", "BJ": "AF", "BW": "AF", "BF": "AF",
		"BI": "AF", "CM": "AF", "CV": "AF", "CF": "AF", "TD": "AF",
		"KM": "AF", "CD": "AF", "DJ": "AF", "EG": "AF", "GQ": "AF",
		"ER": "AF", "ET": "AF", "GA": "AF", "GM": "AF", "GH": "AF",
		"GN": "AF", "GW": "AF", "CI": "AF", "KE": "AF", "LS": "AF",
		"LR": "AF", "LY": "AF", "MG": "AF", "MW": "AF", "ML": "AF",
		"MR": "AF", "MU": "AF", "MA": "AF", "MZ": "AF", "NA": "AF",
		"NE": "AF", "NG": "AF", "RW": "AF", "ST": "AF", "SN": "AF",
		"SC": "AF", "SL": "AF", "SO": "AF", "ZA": "AF", "SS": "AF",
		"SD": "AF", "SZ": "AF", "TZ": "AF", "TG": "AF", "TN": "AF",
		"UG": "AF", "ZM": "AF", "ZW": "AF",

		// Asia
		"AF": "AS", "AM": "AS", "AZ": "AS", "BH": "AS", "BD": "AS",
		"BT": "AS", "BN": "AS", "KH": "AS", "CN": "AS", "CY": "AS",
		"GE": "AS", "HK": "AS", "IN": "AS", "ID": "AS", "IR": "AS",
		"IQ": "AS", "IL": "AS", "JP": "AS", "JO": "AS", "KZ": "AS",
		"KW": "AS", "KG": "AS", "LA": "AS", "LB": "AS", "MO": "AS",
		"MY": "AS", "MV": "AS", "MN": "AS", "MM": "AS", "NP": "AS",
		"KP": "AS", "OM": "AS", "PK": "AS", "PS": "AS", "PH": "AS",
		"QA": "AS", "SA": "AS", "SG": "AS", "KR": "AS", "LK": "AS",
		"SY": "AS", "TW": "AS", "TJ": "AS", "TH": "AS", "TL": "AS",
		"TR": "AS", "TM": "AS", "AE": "AS", "UZ": "AS", "VN": "AS",
		"YE": "AS",

		// Europe
		"AL": "EU", "AD": "EU", "AT": "EU", "BY": "EU", "BE": "EU",
		"BA": "EU", "BG": "EU", "HR": "EU", "CZ": "EU", "DK": "EU",
		"EE": "EU", "FI": "EU", "FR": "EU", "DE": "EU", "GR": "EU",
		"HU": "EU", "IS": "EU", "IE": "EU", "IT": "EU", "LV": "EU",
		"LI": "EU", "LT": "EU", "LU": "EU", "MK": "EU", "MT": "EU",
		"MD": "EU", "MC": "EU", "ME": "EU", "NL": "EU", "NO": "EU",
		"PL": "EU", "PT": "EU", "RO": "EU", "RU": "EU", "SM": "EU",
		"RS": "EU", "SK": "EU", "SI": "EU", "ES": "EU", "SE": "EU",
		"CH": "EU", "UA": "EU", "GB": "EU", "VA": "EU",

		// North America
		"AG": "NA", "BS": "NA", "BB": "NA", "BZ": "NA", "CA": "NA",
		"CR": "NA", "CU": "NA", "DM": "NA", "DO": "NA", "SV": "NA",
		"GD": "NA", "GT": "NA", "HT": "NA", "HN": "NA", "JM": "NA",
		"MX": "NA", "NI": "NA", "PA": "NA", "KN": "NA", "LC": "NA",
		"VC": "NA", "TT": "NA", "US": "NA",

		// Oceania
		"AU": "OC", "FJ": "OC", "KI": "OC", "MH": "OC", "FM": "OC",
		"NR": "OC", "NZ": "OC", "PW": "OC", "PG": "OC", "WS": "OC",
		"SB": "OC", "TO": "OC", "TV": "OC", "VU": "OC",

		// South America
		"AR": "SA", "BO": "SA", "BR": "SA", "CL": "SA", "CO": "SA",
		"EC": "SA", "GY": "SA", "PY": "SA", "PE": "SA", "SR": "SA",
		"UY": "SA", "VE": "SA",

		// Antarctica
		"AQ": "AN",
	}
}
