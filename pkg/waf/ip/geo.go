// Package ip provides basic geo-blocking functionality
package ip

import (
	"net"
	"sync"
)

// GeoBlocker provides basic geo-blocking based on IP ranges
// Note: This is a simplified implementation using known IP ranges
// For production, integrate with a proper GeoIP database
type GeoBlocker struct {
	blockedCountries map[string]bool
	countryRanges    map[string][]*net.IPNet
	mu               sync.RWMutex
}

// NewGeoBlocker creates a new geo blocker
func NewGeoBlocker() *GeoBlocker {
	return &GeoBlocker{
		blockedCountries: make(map[string]bool),
		countryRanges:    make(map[string][]*net.IPNet),
	}
}

// BlockCountry blocks a country by code
func (gb *GeoBlocker) BlockCountry(countryCode string) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	gb.blockedCountries[countryCode] = true
}

// UnblockCountry unblocks a country
func (gb *GeoBlocker) UnblockCountry(countryCode string) {
	gb.mu.Lock()
	defer gb.mu.Unlock()
	delete(gb.blockedCountries, countryCode)
}

// IsCountryBlocked checks if a country is blocked
func (gb *GeoBlocker) IsCountryBlocked(countryCode string) bool {
	gb.mu.RLock()
	defer gb.mu.RUnlock()
	return gb.blockedCountries[countryCode]
}

// AddCountryRange adds an IP range for a country
func (gb *GeoBlocker) AddCountryRange(countryCode, cidr string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	gb.mu.Lock()
	defer gb.mu.Unlock()

	if gb.countryRanges[countryCode] == nil {
		gb.countryRanges[countryCode] = make([]*net.IPNet, 0)
	}
	gb.countryRanges[countryCode] = append(gb.countryRanges[countryCode], network)

	return nil
}

// LookupCountry looks up the country for an IP
// Returns empty string if not found
func (gb *GeoBlocker) LookupCountry(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ""
	}

	gb.mu.RLock()
	defer gb.mu.RUnlock()

	for country, ranges := range gb.countryRanges {
		for _, network := range ranges {
			if network.Contains(parsedIP) {
				return country
			}
		}
	}

	return ""
}

// IsBlocked checks if an IP is geo-blocked
func (gb *GeoBlocker) IsBlocked(ip string) bool {
	country := gb.LookupCountry(ip)
	if country == "" {
		return false
	}

	gb.mu.RLock()
	defer gb.mu.RUnlock()
	return gb.blockedCountries[country]
}

// ListBlockedCountries returns all blocked countries
func (gb *GeoBlocker) ListBlockedCountries() []string {
	gb.mu.RLock()
	defer gb.mu.RUnlock()

	countries := make([]string, 0, len(gb.blockedCountries))
	for country := range gb.blockedCountries {
		countries = append(countries, country)
	}
	return countries
}

// Clear removes all blocked countries and ranges
func (gb *GeoBlocker) Clear() {
	gb.mu.Lock()
	defer gb.mu.Unlock()

	gb.blockedCountries = make(map[string]bool)
	gb.countryRanges = make(map[string][]*net.IPNet)
}

// LoadKnownRanges loads some known IP ranges for testing
// In production, this would load from a GeoIP database
func (gb *GeoBlocker) LoadKnownRanges() {
	// Example ranges - these are just for demonstration
	// In production, use a proper GeoIP database

	// Some example ranges (not exhaustive)
	knownRanges := map[string][]string{
		"US": {
			"3.0.0.0/8",
			"4.0.0.0/8",
		},
		"CN": {
			"1.0.0.0/8",
			"14.0.0.0/8",
			"27.0.0.0/8",
			"36.0.0.0/8",
		},
		"RU": {
			"5.0.0.0/8",
			"31.0.0.0/8",
		},
		"DE": {
			"2.0.0.0/8",
		},
		"GB": {
			"25.0.0.0/8",
		},
	}

	for country, ranges := range knownRanges {
		for _, cidr := range ranges {
			_ = gb.AddCountryRange(country, cidr)
		}
	}
}

// Stats returns statistics about the geo blocker
func (gb *GeoBlocker) Stats() map[string]interface{} {
	gb.mu.RLock()
	defer gb.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["blocked_countries"] = len(gb.blockedCountries)

	rangeCount := 0
	for _, ranges := range gb.countryRanges {
		rangeCount += len(ranges)
	}
	stats["ip_ranges"] = rangeCount
	stats["countries_with_ranges"] = len(gb.countryRanges)

	return stats
}
