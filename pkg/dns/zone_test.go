// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package dns

import (
	"net"
	"testing"
	"time"
)

func TestZone_New(t *testing.T) {
	config := DefaultZoneConfig("example.local")
	zone, err := NewZone(config)
	if err != nil {
		t.Fatalf("NewZone error: %v", err)
	}

	if zone.Name != "example.local" {
		t.Errorf("Zone name = %s, want example.local", zone.Name)
	}

	// Check SOA
	soa := zone.GetSOA()
	if soa == nil {
		t.Fatal("SOA is nil")
	}
	if soa.Type != TypeSOA {
		t.Errorf("SOA type = %d, want %d", soa.Type, TypeSOA)
	}

	// Check NS
	ns := zone.GetNS()
	if len(ns) == 0 {
		t.Error("No NS records")
	}
}

func TestZone_NewInvalidConfig(t *testing.T) {
	// Nil config
	_, err := NewZone(nil)
	if err != ErrInvalidZone {
		t.Errorf("Expected ErrInvalidZone for nil config, got %v", err)
	}

	// Empty name
	_, err = NewZone(&ZoneConfig{Name: ""})
	if err != ErrInvalidZone {
		t.Errorf("Expected ErrInvalidZone for empty name, got %v", err)
	}
}

func TestZone_AddRecord(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	err := zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))
	if err != nil {
		t.Fatalf("AddRecord error: %v", err)
	}

	records := zone.GetRecords("www.test.local", TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(records))
	}

	ip := records[0].GetIP()
	if !ip.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("IP = %v, want 10.0.0.1", ip)
	}
}

func TestZone_AddRecordOutsideZone(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	err := zone.AddRecord(NewARecord("www.other.local", 300, net.ParseIP("10.0.0.1")))
	if err != ErrInvalidZone {
		t.Errorf("Expected ErrInvalidZone for out-of-zone record, got %v", err)
	}
}

func TestZone_AddRecordDuplicate(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	rr := NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1"))
	zone.AddRecord(rr)
	zone.AddRecord(rr) // Add same record again

	records := zone.GetRecords("www.test.local", TypeA)
	if len(records) != 1 {
		t.Errorf("Expected 1 record (no duplicates), got %d", len(records))
	}
}

func TestZone_AddRecordDefaultTTL(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	rr := &ResourceRecord{
		Name:  "www.test.local",
		Type:  TypeA,
		Class: ClassIN,
		TTL:   0, // Will be set to default
		Data:  &ARecord{Address: net.ParseIP("10.0.0.1")},
	}
	zone.AddRecord(rr)

	records := zone.GetRecords("www.test.local", TypeA)
	if len(records) != 1 {
		t.Fatal("Record not added")
	}
	if records[0].TTL == 0 {
		t.Error("TTL should have been set to default")
	}
}

func TestZone_RemoveRecord(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	rr := NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1"))
	zone.AddRecord(rr)

	err := zone.RemoveRecord(rr)
	if err != nil {
		t.Fatalf("RemoveRecord error: %v", err)
	}

	records := zone.GetRecords("www.test.local", TypeA)
	if len(records) != 0 {
		t.Errorf("Expected 0 records after removal, got %d", len(records))
	}
}

func TestZone_RemoveRecordNotFound(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	rr := NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1"))
	err := zone.RemoveRecord(rr)
	if err != ErrRecordNotFound {
		t.Errorf("Expected ErrRecordNotFound, got %v", err)
	}
}

func TestZone_RemoveRecordsByName(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))
	zone.AddRecord(NewAAAARecord("www.test.local", 300, net.ParseIP("::1")))
	zone.AddRecord(NewTXTRecord("www.test.local", 300, "test"))

	// Remove only A records
	err := zone.RemoveRecordsByName("www.test.local", TypeA)
	if err != nil {
		t.Fatalf("RemoveRecordsByName error: %v", err)
	}

	// A should be gone
	aRecords := zone.GetRecords("www.test.local", TypeA)
	if len(aRecords) != 0 {
		t.Error("A records should be removed")
	}

	// AAAA should remain
	aaaaRecords := zone.GetRecords("www.test.local", TypeAAAA)
	if len(aaaaRecords) != 1 {
		t.Error("AAAA records should remain")
	}

	// TXT should remain
	txtRecords := zone.GetRecords("www.test.local", TypeTXT)
	if len(txtRecords) != 1 {
		t.Error("TXT records should remain")
	}
}

func TestZone_RemoveRecordsByNameANY(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))
	zone.AddRecord(NewAAAARecord("www.test.local", 300, net.ParseIP("::1")))
	zone.AddRecord(NewTXTRecord("www.test.local", 300, "test"))

	// Remove all records for name
	err := zone.RemoveRecordsByName("www.test.local", TypeANY)
	if err != nil {
		t.Fatalf("RemoveRecordsByName error: %v", err)
	}

	// All should be gone
	records := zone.GetRecords("www.test.local", TypeANY)
	if len(records) != 0 {
		t.Errorf("Expected 0 records, got %d", len(records))
	}
}

func TestZone_GetRecordsANY(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))
	zone.AddRecord(NewAAAARecord("www.test.local", 300, net.ParseIP("::1")))
	zone.AddRecord(NewTXTRecord("www.test.local", 300, "test"))

	records := zone.GetRecords("www.test.local", TypeANY)
	if len(records) != 3 {
		t.Errorf("Expected 3 records for ANY query, got %d", len(records))
	}
}

func TestZone_Lookup(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))

	tests := []struct {
		name     string
		qtype    uint16
		wantLen  int
		wantNX   bool
	}{
		{"existing A", TypeA, 1, false},
		{"non-existing name", TypeA, 0, true},
		{"existing name, wrong type", TypeTXT, 0, false}, // NODATA
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var qname string
			switch tt.name {
			case "existing A", "existing name, wrong type":
				qname = "www.test.local"
			default:
				qname = "noexist.test.local"
			}

			records, exists := zone.Lookup(qname, tt.qtype)
			if tt.wantNX && exists {
				t.Error("Expected NXDOMAIN, got exists=true")
			}
			if len(records) != tt.wantLen {
				t.Errorf("Got %d records, want %d", len(records), tt.wantLen)
			}
		})
	}
}

func TestZone_LookupCNAME(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zone.AddRecord(NewCNAMERecord("alias.test.local", 300, "www.test.local"))

	// Query for A should return CNAME
	records, exists := zone.Lookup("alias.test.local", TypeA)
	if !exists {
		t.Error("Expected exists=true for CNAME lookup")
	}
	if len(records) != 1 {
		t.Fatalf("Expected 1 CNAME record, got %d", len(records))
	}
	if records[0].Type != TypeCNAME {
		t.Errorf("Expected CNAME type, got %d", records[0].Type)
	}
}

func TestZone_LookupWildcard(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zone.AddRecord(NewARecord("*.test.local", 300, net.ParseIP("10.0.0.100")))

	// Query for random subdomain
	records, exists := zone.Lookup("random.test.local", TypeA)
	if !exists {
		t.Error("Wildcard should match")
	}
	if len(records) != 1 {
		t.Fatalf("Expected 1 wildcard record, got %d", len(records))
	}
	// Name should be replaced with actual queried name
	if records[0].Name != "random.test.local" {
		t.Errorf("Wildcard record name = %s, want random.test.local", records[0].Name)
	}
}

func TestZone_LookupOutOfZone(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	records, exists := zone.Lookup("www.other.local", TypeA)
	if exists {
		t.Error("Out-of-zone lookup should return exists=false")
	}
	if len(records) != 0 {
		t.Error("Out-of-zone lookup should return no records")
	}
}

func TestZone_Serial(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	initialSerial := zone.Serial()

	// Add a record should bump serial
	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))

	newSerial := zone.Serial()
	if newSerial <= initialSerial {
		t.Error("Serial should increase after adding record")
	}
}

func TestZone_LastUpdate(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	before := zone.LastUpdate()

	time.Sleep(10 * time.Millisecond)
	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))

	after := zone.LastUpdate()
	if !after.After(before) {
		t.Error("LastUpdate should be updated after adding record")
	}
}

func TestZone_AllRecords(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))
	zone.AddRecord(NewTXTRecord("www.test.local", 300, "test"))

	all := zone.AllRecords()
	// Should include SOA + NS + our 2 records
	if len(all) < 4 {
		t.Errorf("Expected at least 4 records, got %d", len(all))
	}
}

func TestZone_Names(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))
	zone.AddRecord(NewARecord("api.test.local", 300, net.ParseIP("10.0.0.2")))

	names := zone.Names()
	if len(names) < 3 { // zone name + www + api
		t.Errorf("Expected at least 3 names, got %d", len(names))
	}
}

func TestZone_ReplaceRecords(t *testing.T) {
	zone, _ := NewZone(DefaultZoneConfig("test.local"))

	// Add initial records
	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))
	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.2")))

	// Replace with new records
	newRecords := []*ResourceRecord{
		NewARecord("www.test.local", 600, net.ParseIP("192.168.1.1")),
	}
	err := zone.ReplaceRecords("www.test.local", TypeA, newRecords)
	if err != nil {
		t.Fatalf("ReplaceRecords error: %v", err)
	}

	records := zone.GetRecords("www.test.local", TypeA)
	if len(records) != 1 {
		t.Errorf("Expected 1 record after replace, got %d", len(records))
	}
	if !records[0].GetIP().Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("Unexpected IP: %v", records[0].GetIP())
	}
}

func TestZoneManager_AddRemoveZone(t *testing.T) {
	zm := NewZoneManager()

	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	err := zm.AddZone(zone)
	if err != nil {
		t.Fatalf("AddZone error: %v", err)
	}

	// Duplicate should error
	err = zm.AddZone(zone)
	if err != ErrDuplicateZone {
		t.Errorf("Expected ErrDuplicateZone, got %v", err)
	}

	// Remove
	err = zm.RemoveZone("test.local")
	if err != nil {
		t.Fatalf("RemoveZone error: %v", err)
	}

	// Remove non-existent
	err = zm.RemoveZone("test.local")
	if err != ErrZoneNotFound {
		t.Errorf("Expected ErrZoneNotFound, got %v", err)
	}
}

func TestZoneManager_GetZone(t *testing.T) {
	zm := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zm.AddZone(zone)

	got := zm.GetZone("test.local")
	if got == nil {
		t.Error("GetZone returned nil")
	}

	got = zm.GetZone("other.local")
	if got != nil {
		t.Error("GetZone should return nil for non-existent zone")
	}
}

func TestZoneManager_FindZone(t *testing.T) {
	zm := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("example.local"))
	zm.AddZone(zone)

	// Exact match
	found := zm.FindZone("example.local")
	if found == nil {
		t.Error("Should find exact match")
	}

	// Subdomain match
	found = zm.FindZone("www.example.local")
	if found == nil {
		t.Error("Should find zone for subdomain")
	}

	// Deep subdomain
	found = zm.FindZone("deep.sub.www.example.local")
	if found == nil {
		t.Error("Should find zone for deep subdomain")
	}

	// No match
	found = zm.FindZone("www.other.local")
	if found != nil {
		t.Error("Should not find zone for different domain")
	}
}

func TestZoneManager_Lookup(t *testing.T) {
	zm := NewZoneManager()
	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zone.AddRecord(NewARecord("www.test.local", 300, net.ParseIP("10.0.0.1")))
	zm.AddZone(zone)

	records, returnedZone, exists := zm.Lookup("www.test.local", TypeA)
	if !exists {
		t.Error("Lookup should return exists=true")
	}
	if returnedZone == nil {
		t.Error("Lookup should return zone")
	}
	if len(records) != 1 {
		t.Errorf("Expected 1 record, got %d", len(records))
	}

	// Non-existent
	records, returnedZone, exists = zm.Lookup("www.other.local", TypeA)
	if exists {
		t.Error("Lookup should return exists=false for unknown domain")
	}
	if returnedZone != nil {
		t.Error("Lookup should return nil zone for unknown domain")
	}
}

func TestZoneManager_Zones(t *testing.T) {
	zm := NewZoneManager()

	zone1, _ := NewZone(DefaultZoneConfig("alpha.local"))
	zone2, _ := NewZone(DefaultZoneConfig("beta.local"))
	zone3, _ := NewZone(DefaultZoneConfig("gamma.local"))

	zm.AddZone(zone1)
	zm.AddZone(zone2)
	zm.AddZone(zone3)

	zones := zm.Zones()
	if len(zones) != 3 {
		t.Errorf("Expected 3 zones, got %d", len(zones))
	}
	// Should be sorted
	if zones[0] != "alpha.local" {
		t.Error("Zones should be sorted alphabetically")
	}
}

func TestZoneManager_ZoneCount(t *testing.T) {
	zm := NewZoneManager()

	if zm.ZoneCount() != 0 {
		t.Error("Empty manager should have 0 zones")
	}

	zone, _ := NewZone(DefaultZoneConfig("test.local"))
	zm.AddZone(zone)

	if zm.ZoneCount() != 1 {
		t.Errorf("Expected 1 zone, got %d", zm.ZoneCount())
	}
}

func TestDynamicZone_DynamicAddRemove(t *testing.T) {
	config := DefaultZoneConfig("dynamic.local")
	dz, err := NewDynamicZone(config)
	if err != nil {
		t.Fatalf("NewDynamicZone error: %v", err)
	}

	// Track updates
	var updates int
	dz.AddUpdateHook(func(z *Zone, action string, record *ResourceRecord) {
		updates++
	})

	rr := NewARecord("www.dynamic.local", 300, net.ParseIP("10.0.0.1"))

	// Add
	err = dz.DynamicAdd(rr)
	if err != nil {
		t.Fatalf("DynamicAdd error: %v", err)
	}

	// Remove
	err = dz.DynamicRemove(rr)
	if err != nil {
		t.Fatalf("DynamicRemove error: %v", err)
	}

	// Should have 2 hook calls (add + remove)
	if updates != 2 {
		t.Errorf("Expected 2 hook calls, got %d", updates)
	}

	// Check update counter
	if dz.UpdateCount() != 2 {
		t.Errorf("Expected update count 2, got %d", dz.UpdateCount())
	}
}

func TestDefaultZoneConfig(t *testing.T) {
	config := DefaultZoneConfig("example.local")

	if config.Name != "example.local" {
		t.Errorf("Name = %s, want example.local", config.Name)
	}
	if config.PrimaryNS != "ns1.example.local" {
		t.Errorf("PrimaryNS = %s, want ns1.example.local", config.PrimaryNS)
	}
	if config.AdminEmail != "admin.example.local" {
		t.Errorf("AdminEmail = %s, want admin.example.local", config.AdminEmail)
	}
	if config.Refresh != 3600 {
		t.Errorf("Refresh = %d, want 3600", config.Refresh)
	}
	if config.DefaultTTL != 300 {
		t.Errorf("DefaultTTL = %d, want 300", config.DefaultTTL)
	}
}
