// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dns

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Zone errors
var (
	ErrZoneNotFound   = errors.New("dns: zone not found")
	ErrRecordNotFound = errors.New("dns: record not found")
	ErrDuplicateZone  = errors.New("dns: zone already exists")
	ErrInvalidZone    = errors.New("dns: invalid zone configuration")
)

// Zone represents a DNS zone
type Zone struct {
	mu          sync.RWMutex
	Name        string            // Zone name (e.g., "kingdom.local")
	SOA         *ResourceRecord   // SOA record
	NS          []*ResourceRecord // NS records
	records     map[string]map[uint16][]*ResourceRecord
	serial      uint32
	lastUpdate  time.Time
	primaryNS   string
	adminEmail  string
	refresh     uint32
	retry       uint32
	expire      uint32
	minTTL      uint32
	defaultTTL  uint32
	notifyAddrs []string // Addresses to notify on zone updates
}

// ZoneConfig holds zone configuration
type ZoneConfig struct {
	Name       string
	PrimaryNS  string
	AdminEmail string
	Refresh    uint32 // Refresh interval (seconds)
	Retry      uint32 // Retry interval (seconds)
	Expire     uint32 // Expire time (seconds)
	MinTTL     uint32 // Minimum TTL / negative cache TTL
	DefaultTTL uint32 // Default TTL for records
}

// DefaultZoneConfig returns default zone configuration
func DefaultZoneConfig(name string) *ZoneConfig {
	return &ZoneConfig{
		Name:       name,
		PrimaryNS:  "ns1." + name,
		AdminEmail: "admin." + name,
		Refresh:    3600,  // 1 hour
		Retry:      600,   // 10 minutes
		Expire:     86400, // 24 hours
		MinTTL:     60,    // 1 minute
		DefaultTTL: 300,   // 5 minutes
	}
}

// NewZone creates a new DNS zone
func NewZone(config *ZoneConfig) (*Zone, error) {
	if config == nil || config.Name == "" {
		return nil, ErrInvalidZone
	}

	name := strings.ToLower(strings.TrimSuffix(config.Name, "."))

	z := &Zone{
		Name:        name,
		records:     make(map[string]map[uint16][]*ResourceRecord),
		serial:      uint32(time.Now().Unix()),
		lastUpdate:  time.Now(),
		primaryNS:   config.PrimaryNS,
		adminEmail:  config.AdminEmail,
		refresh:     config.Refresh,
		retry:       config.Retry,
		expire:      config.Expire,
		minTTL:      config.MinTTL,
		defaultTTL:  config.DefaultTTL,
		notifyAddrs: make([]string, 0),
	}

	// Create SOA record
	z.SOA = NewSOARecord(
		name,
		config.DefaultTTL,
		config.PrimaryNS,
		config.AdminEmail,
		z.serial,
		config.Refresh,
		config.Retry,
		config.Expire,
		config.MinTTL,
	)

	// Create NS record for primary
	z.NS = []*ResourceRecord{
		NewNSRecord(name, config.DefaultTTL, config.PrimaryNS),
	}

	return z, nil
}

// AddRecord adds a record to the zone
func (z *Zone) AddRecord(rr *ResourceRecord) error {
	z.mu.Lock()
	defer z.mu.Unlock()

	name := strings.ToLower(strings.TrimSuffix(rr.Name, "."))

	// Ensure name is within zone
	if !z.isInZone(name) {
		return ErrInvalidZone
	}

	// Set default TTL if not specified
	if rr.TTL == 0 {
		rr.TTL = z.defaultTTL
	}

	// Add to records map
	if z.records[name] == nil {
		z.records[name] = make(map[uint16][]*ResourceRecord)
	}

	// Check for duplicates
	existing := z.records[name][rr.Type]
	for _, r := range existing {
		if z.recordsEqual(r, rr) {
			return nil // Already exists
		}
	}

	z.records[name][rr.Type] = append(existing, rr.Clone())
	z.updateSerial()

	return nil
}

// RemoveRecord removes a specific record from the zone
func (z *Zone) RemoveRecord(rr *ResourceRecord) error {
	z.mu.Lock()
	defer z.mu.Unlock()

	name := strings.ToLower(strings.TrimSuffix(rr.Name, "."))

	if z.records[name] == nil {
		return ErrRecordNotFound
	}

	existing := z.records[name][rr.Type]
	var remaining []*ResourceRecord

	found := false
	for _, r := range existing {
		if !z.recordsEqual(r, rr) {
			remaining = append(remaining, r)
		} else {
			found = true
		}
	}

	if !found {
		return ErrRecordNotFound
	}

	if len(remaining) == 0 {
		delete(z.records[name], rr.Type)
		if len(z.records[name]) == 0 {
			delete(z.records, name)
		}
	} else {
		z.records[name][rr.Type] = remaining
	}

	z.updateSerial()
	return nil
}

// RemoveRecordsByName removes all records of a type for a name
func (z *Zone) RemoveRecordsByName(name string, qtype uint16) error {
	z.mu.Lock()
	defer z.mu.Unlock()

	name = strings.ToLower(strings.TrimSuffix(name, "."))

	if z.records[name] == nil {
		return ErrRecordNotFound
	}

	if qtype == TypeANY {
		delete(z.records, name)
	} else {
		if _, exists := z.records[name][qtype]; !exists {
			return ErrRecordNotFound
		}
		delete(z.records[name], qtype)
		if len(z.records[name]) == 0 {
			delete(z.records, name)
		}
	}

	z.updateSerial()
	return nil
}

// GetRecords retrieves records matching name and type
func (z *Zone) GetRecords(name string, qtype uint16) []*ResourceRecord {
	z.mu.RLock()
	defer z.mu.RUnlock()

	name = strings.ToLower(strings.TrimSuffix(name, "."))

	typeMap := z.records[name]
	if typeMap == nil {
		return nil
	}

	if qtype == TypeANY {
		var result []*ResourceRecord
		for _, records := range typeMap {
			for _, rr := range records {
				result = append(result, rr.Clone())
			}
		}
		return result
	}

	records := typeMap[qtype]
	if len(records) == 0 {
		return nil
	}

	result := make([]*ResourceRecord, len(records))
	for i, rr := range records {
		result[i] = rr.Clone()
	}
	return result
}

// Lookup performs a DNS lookup within the zone
func (z *Zone) Lookup(name string, qtype uint16) ([]*ResourceRecord, bool) {
	z.mu.RLock()
	defer z.mu.RUnlock()

	name = strings.ToLower(strings.TrimSuffix(name, "."))

	// Check if name is in zone
	if !z.isInZone(name) {
		return nil, false
	}

	// Direct match
	if records := z.getRecordsLocked(name, qtype); len(records) > 0 {
		return records, true
	}

	// Check for CNAME
	if qtype != TypeCNAME {
		if cnames := z.getRecordsLocked(name, TypeCNAME); len(cnames) > 0 {
			return cnames, true
		}
	}

	// Check for wildcard
	wildcard := z.findWildcard(name)
	if wildcard != "" {
		if records := z.getRecordsLocked(wildcard, qtype); len(records) > 0 {
			// Replace wildcard name with actual name
			result := make([]*ResourceRecord, len(records))
			for i, rr := range records {
				clone := rr.Clone()
				clone.Name = name
				result[i] = clone
			}
			return result, true
		}
	}

	// Name exists but no records of requested type
	if z.nameExists(name) {
		return nil, true // NODATA response
	}

	return nil, false // NXDOMAIN
}

// nameExists checks if any records exist for a name
func (z *Zone) nameExists(name string) bool {
	return len(z.records[name]) > 0
}

// getRecordsLocked retrieves records without locking
func (z *Zone) getRecordsLocked(name string, qtype uint16) []*ResourceRecord {
	typeMap := z.records[name]
	if typeMap == nil {
		return nil
	}

	if qtype == TypeANY {
		var result []*ResourceRecord
		for _, records := range typeMap {
			for _, rr := range records {
				result = append(result, rr.Clone())
			}
		}
		return result
	}

	records := typeMap[qtype]
	if len(records) == 0 {
		return nil
	}

	result := make([]*ResourceRecord, len(records))
	for i, rr := range records {
		result[i] = rr.Clone()
	}
	return result
}

// findWildcard finds a matching wildcard name
func (z *Zone) findWildcard(name string) string {
	labels := strings.Split(name, ".")

	for i := 0; i < len(labels)-1; i++ {
		wildcard := "*." + strings.Join(labels[i+1:], ".")
		if z.records[wildcard] != nil {
			return wildcard
		}
	}

	return ""
}

// isInZone checks if a name is within this zone
func (z *Zone) isInZone(name string) bool {
	return name == z.Name || strings.HasSuffix(name, "."+z.Name)
}

// updateSerial increments the zone serial number
func (z *Zone) updateSerial() {
	z.serial++
	z.lastUpdate = time.Now()

	// Update SOA record
	if soa, ok := z.SOA.Data.(*SOARecord); ok {
		soa.Serial = z.serial
	}
}

// recordsEqual checks if two records are equal
func (z *Zone) recordsEqual(a, b *ResourceRecord) bool {
	if a.Name != b.Name || a.Type != b.Type || a.Class != b.Class {
		return false
	}

	// Compare data based on type
	switch ad := a.Data.(type) {
	case *ARecord:
		if bd, ok := b.Data.(*ARecord); ok {
			return ad.Address.Equal(bd.Address)
		}
	case *AAAARecord:
		if bd, ok := b.Data.(*AAAARecord); ok {
			return ad.Address.Equal(bd.Address)
		}
	case *CNAMERecord:
		if bd, ok := b.Data.(*CNAMERecord); ok {
			return strings.EqualFold(ad.Target, bd.Target)
		}
	case *PTRRecord:
		if bd, ok := b.Data.(*PTRRecord); ok {
			return strings.EqualFold(ad.Target, bd.Target)
		}
	case *TXTRecord:
		if bd, ok := b.Data.(*TXTRecord); ok {
			if len(ad.Text) != len(bd.Text) {
				return false
			}
			for i := range ad.Text {
				if ad.Text[i] != bd.Text[i] {
					return false
				}
			}
			return true
		}
	case *SRVRecord:
		if bd, ok := b.Data.(*SRVRecord); ok {
			return ad.Priority == bd.Priority &&
				ad.Weight == bd.Weight &&
				ad.Port == bd.Port &&
				strings.EqualFold(ad.Target, bd.Target)
		}
	case *MXRecord:
		if bd, ok := b.Data.(*MXRecord); ok {
			return ad.Preference == bd.Preference &&
				strings.EqualFold(ad.Exchange, bd.Exchange)
		}
	case *NSRecord:
		if bd, ok := b.Data.(*NSRecord); ok {
			return strings.EqualFold(ad.NameServer, bd.NameServer)
		}
	}

	return false
}

// GetSOA returns the zone's SOA record
func (z *Zone) GetSOA() *ResourceRecord {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.SOA.Clone()
}

// GetNS returns the zone's NS records
func (z *Zone) GetNS() []*ResourceRecord {
	z.mu.RLock()
	defer z.mu.RUnlock()

	result := make([]*ResourceRecord, len(z.NS))
	for i, ns := range z.NS {
		result[i] = ns.Clone()
	}
	return result
}

// Serial returns the current zone serial
func (z *Zone) Serial() uint32 {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.serial
}

// LastUpdate returns the last update time
func (z *Zone) LastUpdate() time.Time {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.lastUpdate
}

// AllRecords returns all records in the zone
func (z *Zone) AllRecords() []*ResourceRecord {
	z.mu.RLock()
	defer z.mu.RUnlock()

	var result []*ResourceRecord

	// Add SOA
	result = append(result, z.SOA.Clone())

	// Add NS records
	for _, ns := range z.NS {
		result = append(result, ns.Clone())
	}

	// Add all other records
	for _, typeMap := range z.records {
		for _, records := range typeMap {
			for _, rr := range records {
				result = append(result, rr.Clone())
			}
		}
	}

	return result
}

// Names returns all unique names in the zone
func (z *Zone) Names() []string {
	z.mu.RLock()
	defer z.mu.RUnlock()

	names := make(map[string]bool)
	names[z.Name] = true

	for name := range z.records {
		names[name] = true
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)

	return result
}

// ZoneManager manages multiple DNS zones
type ZoneManager struct {
	mu    sync.RWMutex
	zones map[string]*Zone
}

// NewZoneManager creates a new zone manager
func NewZoneManager() *ZoneManager {
	return &ZoneManager{
		zones: make(map[string]*Zone),
	}
}

// AddZone adds a zone to the manager
func (zm *ZoneManager) AddZone(zone *Zone) error {
	zm.mu.Lock()
	defer zm.mu.Unlock()

	name := strings.ToLower(zone.Name)
	if _, exists := zm.zones[name]; exists {
		return ErrDuplicateZone
	}

	zm.zones[name] = zone
	return nil
}

// RemoveZone removes a zone from the manager
func (zm *ZoneManager) RemoveZone(name string) error {
	zm.mu.Lock()
	defer zm.mu.Unlock()

	name = strings.ToLower(name)
	if _, exists := zm.zones[name]; !exists {
		return ErrZoneNotFound
	}

	delete(zm.zones, name)
	return nil
}

// GetZone retrieves a zone by name
func (zm *ZoneManager) GetZone(name string) *Zone {
	zm.mu.RLock()
	defer zm.mu.RUnlock()

	return zm.zones[strings.ToLower(name)]
}

// FindZone finds the most specific zone for a name
func (zm *ZoneManager) FindZone(name string) *Zone {
	zm.mu.RLock()
	defer zm.mu.RUnlock()

	name = strings.ToLower(strings.TrimSuffix(name, "."))

	// Try exact match first
	if zone := zm.zones[name]; zone != nil {
		return zone
	}

	// Walk up the name hierarchy
	labels := strings.Split(name, ".")
	for i := 1; i < len(labels); i++ {
		zoneName := strings.Join(labels[i:], ".")
		if zone := zm.zones[zoneName]; zone != nil {
			return zone
		}
	}

	return nil
}

// Lookup performs a lookup across all zones
func (zm *ZoneManager) Lookup(name string, qtype uint16) ([]*ResourceRecord, *Zone, bool) {
	zone := zm.FindZone(name)
	if zone == nil {
		return nil, nil, false
	}

	records, exists := zone.Lookup(name, qtype)
	return records, zone, exists
}

// Zones returns all zone names
func (zm *ZoneManager) Zones() []string {
	zm.mu.RLock()
	defer zm.mu.RUnlock()

	names := make([]string, 0, len(zm.zones))
	for name := range zm.zones {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// ZoneCount returns the number of zones
func (zm *ZoneManager) ZoneCount() int {
	zm.mu.RLock()
	defer zm.mu.RUnlock()
	return len(zm.zones)
}

// DynamicZone extends Zone with dynamic update support
type DynamicZone struct {
	*Zone
	updateCounter uint64
	hooks         []ZoneUpdateHook
	hooksMu       sync.RWMutex
}

// ZoneUpdateHook is called when the zone is updated
type ZoneUpdateHook func(zone *Zone, action string, record *ResourceRecord)

// NewDynamicZone creates a new dynamic zone
func NewDynamicZone(config *ZoneConfig) (*DynamicZone, error) {
	zone, err := NewZone(config)
	if err != nil {
		return nil, err
	}

	return &DynamicZone{
		Zone:  zone,
		hooks: make([]ZoneUpdateHook, 0),
	}, nil
}

// AddUpdateHook adds a hook to be called on zone updates
func (dz *DynamicZone) AddUpdateHook(hook ZoneUpdateHook) {
	dz.hooksMu.Lock()
	dz.hooks = append(dz.hooks, hook)
	dz.hooksMu.Unlock()
}

// DynamicAdd adds a record and triggers hooks
func (dz *DynamicZone) DynamicAdd(rr *ResourceRecord) error {
	if err := dz.Zone.AddRecord(rr); err != nil {
		return err
	}

	atomic.AddUint64(&dz.updateCounter, 1)
	dz.triggerHooks("add", rr)
	return nil
}

// DynamicRemove removes a record and triggers hooks
func (dz *DynamicZone) DynamicRemove(rr *ResourceRecord) error {
	if err := dz.Zone.RemoveRecord(rr); err != nil {
		return err
	}

	atomic.AddUint64(&dz.updateCounter, 1)
	dz.triggerHooks("remove", rr)
	return nil
}

// UpdateCount returns the number of dynamic updates
func (dz *DynamicZone) UpdateCount() uint64 {
	return atomic.LoadUint64(&dz.updateCounter)
}

// triggerHooks calls all registered hooks
func (dz *DynamicZone) triggerHooks(action string, rr *ResourceRecord) {
	dz.hooksMu.RLock()
	hooks := make([]ZoneUpdateHook, len(dz.hooks))
	copy(hooks, dz.hooks)
	dz.hooksMu.RUnlock()

	for _, hook := range hooks {
		hook(dz.Zone, action, rr)
	}
}

// ReplaceRecords atomically replaces all records of a type for a name
func (z *Zone) ReplaceRecords(name string, qtype uint16, records []*ResourceRecord) error {
	z.mu.Lock()
	defer z.mu.Unlock()

	name = strings.ToLower(strings.TrimSuffix(name, "."))

	if !z.isInZone(name) {
		return ErrInvalidZone
	}

	if z.records[name] == nil {
		z.records[name] = make(map[uint16][]*ResourceRecord)
	}

	cloned := make([]*ResourceRecord, len(records))
	for i, rr := range records {
		if rr.TTL == 0 {
			rr.TTL = z.defaultTTL
		}
		cloned[i] = rr.Clone()
	}

	z.records[name][qtype] = cloned
	z.updateSerial()

	return nil
}
