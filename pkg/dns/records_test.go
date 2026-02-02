package dns

import (
	"net"
	"testing"
)

func TestNewARecord(t *testing.T) {
	rr := NewARecord("example.com", 300, net.ParseIP("192.168.1.1"))

	if rr.Name != "example.com" {
		t.Errorf("Name = %s, want example.com", rr.Name)
	}
	if rr.Type != TypeA {
		t.Errorf("Type = %d, want %d", rr.Type, TypeA)
	}
	if rr.Class != ClassIN {
		t.Errorf("Class = %d, want %d", rr.Class, ClassIN)
	}
	if rr.TTL != 300 {
		t.Errorf("TTL = %d, want 300", rr.TTL)
	}

	ip := rr.GetIP()
	if !ip.Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("IP = %v, want 192.168.1.1", ip)
	}

	// Test String()
	if rr.Data.String() != "192.168.1.1" {
		t.Errorf("String() = %s, want 192.168.1.1", rr.Data.String())
	}
}

func TestNewAAAARecord(t *testing.T) {
	ip := net.ParseIP("2001:db8::1")
	rr := NewAAAARecord("ipv6.example.com", 600, ip)

	if rr.Type != TypeAAAA {
		t.Errorf("Type = %d, want %d", rr.Type, TypeAAAA)
	}

	gotIP := rr.GetIP()
	if !gotIP.Equal(ip) {
		t.Errorf("IP = %v, want %v", gotIP, ip)
	}
}

func TestNewCNAMERecord(t *testing.T) {
	rr := NewCNAMERecord("www.example.com", 3600, "example.com")

	if rr.Type != TypeCNAME {
		t.Errorf("Type = %d, want %d", rr.Type, TypeCNAME)
	}

	target := rr.GetTarget()
	if target != "example.com" {
		t.Errorf("Target = %s, want example.com", target)
	}
}

func TestNewPTRRecord(t *testing.T) {
	rr := NewPTRRecord("1.168.192.in-addr.arpa", 3600, "host.example.com")

	if rr.Type != TypePTR {
		t.Errorf("Type = %d, want %d", rr.Type, TypePTR)
	}

	target := rr.GetTarget()
	if target != "host.example.com" {
		t.Errorf("Target = %s, want host.example.com", target)
	}
}

func TestNewTXTRecord(t *testing.T) {
	rr := NewTXTRecord("example.com", 300, "v=spf1 include:_spf.google.com ~all", "key=value")

	if rr.Type != TypeTXT {
		t.Errorf("Type = %d, want %d", rr.Type, TypeTXT)
	}

	txt := rr.GetTXT()
	if len(txt) != 2 {
		t.Fatalf("TXT len = %d, want 2", len(txt))
	}
	if txt[0] != "v=spf1 include:_spf.google.com ~all" {
		t.Errorf("TXT[0] = %s, unexpected", txt[0])
	}
	if txt[1] != "key=value" {
		t.Errorf("TXT[1] = %s, want key=value", txt[1])
	}
}

func TestNewSRVRecord(t *testing.T) {
	rr := NewSRVRecord("_http._tcp.example.com", 300, 10, 100, 8080, "server.example.com")

	if rr.Type != TypeSRV {
		t.Errorf("Type = %d, want %d", rr.Type, TypeSRV)
	}

	priority, weight, port, target, ok := rr.GetSRV()
	if !ok {
		t.Fatal("GetSRV() returned false")
	}
	if priority != 10 {
		t.Errorf("Priority = %d, want 10", priority)
	}
	if weight != 100 {
		t.Errorf("Weight = %d, want 100", weight)
	}
	if port != 8080 {
		t.Errorf("Port = %d, want 8080", port)
	}
	if target != "server.example.com" {
		t.Errorf("Target = %s, want server.example.com", target)
	}

	// Also test GetTarget()
	if rr.GetTarget() != "server.example.com" {
		t.Errorf("GetTarget() = %s, want server.example.com", rr.GetTarget())
	}
}

func TestNewMXRecord(t *testing.T) {
	rr := NewMXRecord("example.com", 3600, 10, "mail.example.com")

	if rr.Type != TypeMX {
		t.Errorf("Type = %d, want %d", rr.Type, TypeMX)
	}

	target := rr.GetTarget()
	if target != "mail.example.com" {
		t.Errorf("Target = %s, want mail.example.com", target)
	}

	mx := rr.Data.(*MXRecord)
	if mx.Preference != 10 {
		t.Errorf("Preference = %d, want 10", mx.Preference)
	}
}

func TestNewNSRecord(t *testing.T) {
	rr := NewNSRecord("example.com", 86400, "ns1.example.com")

	if rr.Type != TypeNS {
		t.Errorf("Type = %d, want %d", rr.Type, TypeNS)
	}

	target := rr.GetTarget()
	if target != "ns1.example.com" {
		t.Errorf("Target = %s, want ns1.example.com", target)
	}
}

func TestNewSOARecord(t *testing.T) {
	rr := NewSOARecord("example.com", 3600, "ns1.example.com", "admin.example.com", 2024010101, 7200, 3600, 1209600, 86400)

	if rr.Type != TypeSOA {
		t.Errorf("Type = %d, want %d", rr.Type, TypeSOA)
	}

	soa := rr.Data.(*SOARecord)
	if soa.MName != "ns1.example.com" {
		t.Errorf("MName = %s, want ns1.example.com", soa.MName)
	}
	if soa.RName != "admin.example.com" {
		t.Errorf("RName = %s, want admin.example.com", soa.RName)
	}
	if soa.Serial != 2024010101 {
		t.Errorf("Serial = %d, want 2024010101", soa.Serial)
	}
	if soa.Refresh != 7200 {
		t.Errorf("Refresh = %d, want 7200", soa.Refresh)
	}
	if soa.Retry != 3600 {
		t.Errorf("Retry = %d, want 3600", soa.Retry)
	}
	if soa.Expire != 1209600 {
		t.Errorf("Expire = %d, want 1209600", soa.Expire)
	}
	if soa.Minimum != 86400 {
		t.Errorf("Minimum = %d, want 86400", soa.Minimum)
	}
}

func TestResourceRecord_Clone(t *testing.T) {
	original := NewARecord("test.com", 300, net.ParseIP("10.0.0.1"))
	clone := original.Clone()

	// Modify original
	original.TTL = 600
	original.Data.(*ARecord).Address = net.ParseIP("10.0.0.2")

	// Clone should be unaffected
	if clone.TTL != 300 {
		t.Errorf("Clone TTL changed: %d", clone.TTL)
	}
	if !clone.GetIP().Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("Clone IP changed: %v", clone.GetIP())
	}
}

func TestResourceRecord_CloneAllTypes(t *testing.T) {
	tests := []struct {
		name   string
		record *ResourceRecord
	}{
		{"A", NewARecord("test.com", 300, net.ParseIP("10.0.0.1"))},
		{"AAAA", NewAAAARecord("test.com", 300, net.ParseIP("::1"))},
		{"CNAME", NewCNAMERecord("www.test.com", 300, "test.com")},
		{"PTR", NewPTRRecord("1.0.0.10.in-addr.arpa", 300, "test.com")},
		{"TXT", NewTXTRecord("test.com", 300, "txt1", "txt2")},
		{"SRV", NewSRVRecord("_http._tcp.test.com", 300, 10, 100, 8080, "server.test.com")},
		{"MX", NewMXRecord("test.com", 300, 10, "mail.test.com")},
		{"NS", NewNSRecord("test.com", 300, "ns1.test.com")},
		{"SOA", NewSOARecord("test.com", 300, "ns1.test.com", "admin.test.com", 1, 3600, 600, 86400, 60)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clone := tt.record.Clone()
			if clone == nil {
				t.Fatal("Clone returned nil")
			}
			if clone.Type != tt.record.Type {
				t.Errorf("Clone type mismatch: %d vs %d", clone.Type, tt.record.Type)
			}
			if clone.Name != tt.record.Name {
				t.Errorf("Clone name mismatch: %s vs %s", clone.Name, tt.record.Name)
			}
		})
	}
}

func TestResourceRecord_String(t *testing.T) {
	rr := NewARecord("example.com", 300, net.ParseIP("192.168.1.1"))
	str := rr.String()

	// Should contain name, TTL, class, type, and data
	if str == "" {
		t.Error("String() returned empty string")
	}
	// Format: "name TTL class type data"
	expected := "example.com 300 IN A 192.168.1.1"
	if str != expected {
		t.Errorf("String() = %s, want %s", str, expected)
	}
}

func TestResourceRecord_GetIPNonAddressRecord(t *testing.T) {
	rr := NewCNAMERecord("www.test.com", 300, "test.com")
	ip := rr.GetIP()
	if ip != nil {
		t.Errorf("GetIP() on CNAME should return nil, got %v", ip)
	}
}

func TestResourceRecord_GetTargetNonTargetRecord(t *testing.T) {
	rr := NewARecord("test.com", 300, net.ParseIP("10.0.0.1"))
	target := rr.GetTarget()
	if target != "" {
		t.Errorf("GetTarget() on A record should return empty, got %s", target)
	}
}

func TestResourceRecord_GetTXTNonTXTRecord(t *testing.T) {
	rr := NewARecord("test.com", 300, net.ParseIP("10.0.0.1"))
	txt := rr.GetTXT()
	if txt != nil {
		t.Errorf("GetTXT() on A record should return nil, got %v", txt)
	}
}

func TestResourceRecord_GetSRVNonSRVRecord(t *testing.T) {
	rr := NewARecord("test.com", 300, net.ParseIP("10.0.0.1"))
	_, _, _, _, ok := rr.GetSRV()
	if ok {
		t.Error("GetSRV() on A record should return false")
	}
}

func TestRawRecord(t *testing.T) {
	raw := &RawRecord{Data: []byte{0x01, 0x02, 0x03}}
	str := raw.String()
	if str != "[3 bytes]" {
		t.Errorf("RawRecord.String() = %s, want [3 bytes]", str)
	}
}

func TestRecordDataStrings(t *testing.T) {
	tests := []struct {
		name     string
		data     RData
		expected string
	}{
		{"A", &ARecord{Address: net.ParseIP("1.2.3.4")}, "1.2.3.4"},
		{"AAAA", &AAAARecord{Address: net.ParseIP("::1")}, "::1"},
		{"CNAME", &CNAMERecord{Target: "target.com"}, "target.com"},
		{"PTR", &PTRRecord{Target: "host.com"}, "host.com"},
		{"TXT", &TXTRecord{Text: []string{"a", "b"}}, "a b"},
		{"SRV", &SRVRecord{Priority: 10, Weight: 20, Port: 80, Target: "srv.com"}, "10 20 80 srv.com"},
		{"MX", &MXRecord{Preference: 10, Exchange: "mail.com"}, "10 mail.com"},
		{"NS", &NSRecord{NameServer: "ns1.com"}, "ns1.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.data.String(); got != tt.expected {
				t.Errorf("String() = %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestSOARecord_String(t *testing.T) {
	soa := &SOARecord{
		MName:   "ns1.example.com",
		RName:   "admin.example.com",
		Serial:  2024010101,
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		Minimum: 60,
	}

	str := soa.String()
	expected := "ns1.example.com admin.example.com 2024010101 3600 600 86400 60"
	if str != expected {
		t.Errorf("SOA.String() = %s, want %s", str, expected)
	}
}
