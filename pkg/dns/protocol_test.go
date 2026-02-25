// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package dns

import (
	"net"
	"testing"
)

func TestHeader_QueryResponse(t *testing.T) {
	h := Header{Flags: 0}
	if !h.IsQuery() {
		t.Error("New header should be query")
	}
	if h.IsResponse() {
		t.Error("New header should not be response")
	}

	h.Flags |= FlagQR
	if h.IsQuery() {
		t.Error("Header with QR flag should not be query")
	}
	if !h.IsResponse() {
		t.Error("Header with QR flag should be response")
	}
}

func TestHeader_Opcode(t *testing.T) {
	tests := []struct {
		opcode uint8
		name   string
	}{
		{OpcodeQuery, "Query"},
		{OpcodeIQuery, "IQuery"},
		{OpcodeStatus, "Status"},
		{OpcodeNotify, "Notify"},
		{OpcodeUpdate, "Update"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Header{}
			h.SetOpcode(tt.opcode)
			if h.Opcode() != tt.opcode {
				t.Errorf("Opcode() = %d, want %d", h.Opcode(), tt.opcode)
			}
		})
	}
}

func TestHeader_Rcode(t *testing.T) {
	tests := []struct {
		rcode uint8
		name  string
	}{
		{RcodeSuccess, "Success"},
		{RcodeFormatError, "FormatError"},
		{RcodeServerFailure, "ServerFailure"},
		{RcodeNameError, "NameError"},
		{RcodeNotImplemented, "NotImplemented"},
		{RcodeRefused, "Refused"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Header{}
			h.SetRcode(tt.rcode)
			if h.Rcode() != tt.rcode {
				t.Errorf("Rcode() = %d, want %d", h.Rcode(), tt.rcode)
			}
		})
	}
}

func TestNewQuery(t *testing.T) {
	msg := NewQuery("example.com", TypeA)

	if len(msg.Questions) != 1 {
		t.Fatalf("Expected 1 question, got %d", len(msg.Questions))
	}

	q := msg.Questions[0]
	if q.Name != "example.com" {
		t.Errorf("Question name = %s, want example.com", q.Name)
	}
	if q.Type != TypeA {
		t.Errorf("Question type = %d, want %d", q.Type, TypeA)
	}
	if q.Class != ClassIN {
		t.Errorf("Question class = %d, want %d", q.Class, ClassIN)
	}

	// Should have RD flag set
	if msg.Header.Flags&FlagRD == 0 {
		t.Error("Query should have RD flag set")
	}
}

func TestNewResponse(t *testing.T) {
	query := NewQuery("example.com", TypeA)
	query.Header.ID = 12345

	resp := NewResponse(query)

	// Should have same ID
	if resp.Header.ID != 12345 {
		t.Errorf("Response ID = %d, want 12345", resp.Header.ID)
	}

	// Should have QR flag
	if !resp.Header.IsResponse() {
		t.Error("Response should have QR flag")
	}

	// Should have AA flag
	if resp.Header.Flags&FlagAA == 0 {
		t.Error("Response should have AA flag")
	}

	// Should copy questions
	if len(resp.Questions) != 1 {
		t.Errorf("Response should have 1 question, got %d", len(resp.Questions))
	}
}

func TestMessage_PackUnpack(t *testing.T) {
	// Create a query
	query := NewQuery("www.example.com", TypeA)
	query.Header.ID = 54321

	// Pack
	data, err := query.Pack()
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	// Unpack
	unpacked := &Message{}
	err = unpacked.Unpack(data)
	if err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	// Verify
	if unpacked.Header.ID != 54321 {
		t.Errorf("Unpacked ID = %d, want 54321", unpacked.Header.ID)
	}
	if len(unpacked.Questions) != 1 {
		t.Fatalf("Expected 1 question, got %d", len(unpacked.Questions))
	}
	if unpacked.Questions[0].Name != "www.example.com" {
		t.Errorf("Question name = %s, want www.example.com", unpacked.Questions[0].Name)
	}
}

func TestMessage_PackUnpackWithAnswers(t *testing.T) {
	// Create a response with answers
	query := NewQuery("example.com", TypeA)
	resp := NewResponse(query)
	resp.Answers = []ResourceRecord{
		*NewARecord("example.com", 300, net.ParseIP("93.184.216.34")),
		*NewARecord("example.com", 300, net.ParseIP("93.184.216.35")),
	}

	// Pack
	data, err := resp.Pack()
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	// Unpack
	unpacked := &Message{}
	err = unpacked.Unpack(data)
	if err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	// Verify
	if len(unpacked.Answers) != 2 {
		t.Fatalf("Expected 2 answers, got %d", len(unpacked.Answers))
	}
	if unpacked.Answers[0].Type != TypeA {
		t.Errorf("Answer type = %d, want %d", unpacked.Answers[0].Type, TypeA)
	}
}

func TestMessage_PackUnpackAllSections(t *testing.T) {
	query := NewQuery("example.com", TypeA)
	resp := NewResponse(query)

	resp.Answers = []ResourceRecord{
		*NewARecord("example.com", 300, net.ParseIP("10.0.0.1")),
	}
	resp.Authority = []ResourceRecord{
		*NewNSRecord("example.com", 86400, "ns1.example.com"),
	}
	resp.Additional = []ResourceRecord{
		*NewARecord("ns1.example.com", 86400, net.ParseIP("10.0.0.100")),
	}

	data, err := resp.Pack()
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	unpacked := &Message{}
	err = unpacked.Unpack(data)
	if err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	if len(unpacked.Answers) != 1 {
		t.Errorf("Expected 1 answer, got %d", len(unpacked.Answers))
	}
	if len(unpacked.Authority) != 1 {
		t.Errorf("Expected 1 authority, got %d", len(unpacked.Authority))
	}
	if len(unpacked.Additional) != 1 {
		t.Errorf("Expected 1 additional, got %d", len(unpacked.Additional))
	}
}

func TestMessage_PackUnpackSRV(t *testing.T) {
	query := NewQuery("_http._tcp.example.com", TypeSRV)
	resp := NewResponse(query)
	resp.Answers = []ResourceRecord{
		*NewSRVRecord("_http._tcp.example.com", 300, 10, 100, 8080, "server.example.com"),
	}

	data, err := resp.Pack()
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	unpacked := &Message{}
	err = unpacked.Unpack(data)
	if err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	if len(unpacked.Answers) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(unpacked.Answers))
	}

	priority, weight, port, target, ok := unpacked.Answers[0].GetSRV()
	if !ok {
		t.Fatal("Expected SRV record")
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
}

func TestMessage_PackUnpackTXT(t *testing.T) {
	query := NewQuery("example.com", TypeTXT)
	resp := NewResponse(query)
	resp.Answers = []ResourceRecord{
		*NewTXTRecord("example.com", 300, "v=spf1 -all", "key=value"),
	}

	data, err := resp.Pack()
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	unpacked := &Message{}
	err = unpacked.Unpack(data)
	if err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	txt := unpacked.Answers[0].GetTXT()
	if len(txt) != 2 {
		t.Fatalf("Expected 2 TXT strings, got %d", len(txt))
	}
	if txt[0] != "v=spf1 -all" {
		t.Errorf("TXT[0] = %s, want v=spf1 -all", txt[0])
	}
}

func TestMessage_PackUnpackSOA(t *testing.T) {
	query := NewQuery("example.com", TypeSOA)
	resp := NewResponse(query)
	resp.Answers = []ResourceRecord{
		*NewSOARecord("example.com", 3600, "ns1.example.com", "admin.example.com", 2024010101, 7200, 1800, 1209600, 86400),
	}

	data, err := resp.Pack()
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	unpacked := &Message{}
	err = unpacked.Unpack(data)
	if err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	soa, ok := unpacked.Answers[0].Data.(*SOARecord)
	if !ok {
		t.Fatal("Expected SOA record")
	}
	if soa.MName != "ns1.example.com" {
		t.Errorf("MName = %s, want ns1.example.com", soa.MName)
	}
	if soa.Serial != 2024010101 {
		t.Errorf("Serial = %d, want 2024010101", soa.Serial)
	}
}

func TestMessage_PackUnpackMX(t *testing.T) {
	query := NewQuery("example.com", TypeMX)
	resp := NewResponse(query)
	resp.Answers = []ResourceRecord{
		*NewMXRecord("example.com", 3600, 10, "mail.example.com"),
	}

	data, err := resp.Pack()
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	unpacked := &Message{}
	err = unpacked.Unpack(data)
	if err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	mx, ok := unpacked.Answers[0].Data.(*MXRecord)
	if !ok {
		t.Fatal("Expected MX record")
	}
	if mx.Preference != 10 {
		t.Errorf("Preference = %d, want 10", mx.Preference)
	}
	if mx.Exchange != "mail.example.com" {
		t.Errorf("Exchange = %s, want mail.example.com", mx.Exchange)
	}
}

func TestMessage_UnpackErrors(t *testing.T) {
	// Too short
	err := (&Message{}).Unpack([]byte{0, 1, 2})
	if err != ErrInvalidMessage {
		t.Errorf("Expected ErrInvalidMessage for short data, got %v", err)
	}

	// Valid header but truncated question
	truncated := make([]byte, 12)
	truncated[5] = 1 // 1 question
	err = (&Message{}).Unpack(truncated)
	if err != ErrTruncated {
		t.Errorf("Expected ErrTruncated, got %v", err)
	}
}

func TestTypeToString(t *testing.T) {
	tests := []struct {
		t    uint16
		want string
	}{
		{TypeA, "A"},
		{TypeAAAA, "AAAA"},
		{TypeCNAME, "CNAME"},
		{TypeMX, "MX"},
		{TypeTXT, "TXT"},
		{TypeSRV, "SRV"},
		{TypeNS, "NS"},
		{TypeSOA, "SOA"},
		{TypePTR, "PTR"},
		{TypeOPT, "OPT"},
		{TypeANY, "ANY"},
		{999, "TYPE999"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := TypeToString(tt.t); got != tt.want {
				t.Errorf("TypeToString(%d) = %s, want %s", tt.t, got, tt.want)
			}
		})
	}
}

func TestStringToType(t *testing.T) {
	tests := []struct {
		s    string
		want uint16
	}{
		{"A", TypeA},
		{"a", TypeA},
		{"AAAA", TypeAAAA},
		{"CNAME", TypeCNAME},
		{"MX", TypeMX},
		{"TXT", TypeTXT},
		{"SRV", TypeSRV},
		{"NS", TypeNS},
		{"SOA", TypeSOA},
		{"PTR", TypePTR},
		{"OPT", TypeOPT},
		{"ANY", TypeANY},
		{"UNKNOWN", 0},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			if got := StringToType(tt.s); got != tt.want {
				t.Errorf("StringToType(%s) = %d, want %d", tt.s, got, tt.want)
			}
		})
	}
}

func TestClassToString(t *testing.T) {
	tests := []struct {
		c    uint16
		want string
	}{
		{ClassIN, "IN"},
		{ClassCH, "CH"},
		{ClassHS, "HS"},
		{ClassANY, "ANY"},
		{999, "CLASS999"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := ClassToString(tt.c); got != tt.want {
				t.Errorf("ClassToString(%d) = %s, want %s", tt.c, got, tt.want)
			}
		})
	}
}

func TestRcodeToString(t *testing.T) {
	tests := []struct {
		rc   uint8
		want string
	}{
		{RcodeSuccess, "NOERROR"},
		{RcodeFormatError, "FORMERR"},
		{RcodeServerFailure, "SERVFAIL"},
		{RcodeNameError, "NXDOMAIN"},
		{RcodeNotImplemented, "NOTIMP"},
		{RcodeRefused, "REFUSED"},
		{99, "RCODE99"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := RcodeToString(tt.rc); got != tt.want {
				t.Errorf("RcodeToString(%d) = %s, want %s", tt.rc, got, tt.want)
			}
		})
	}
}

func TestNameCompression(t *testing.T) {
	// Create a message with multiple occurrences of the same domain
	// to test name compression
	query := NewQuery("www.example.com", TypeA)
	resp := NewResponse(query)
	resp.Answers = []ResourceRecord{
		*NewARecord("www.example.com", 300, net.ParseIP("10.0.0.1")),
	}
	resp.Authority = []ResourceRecord{
		*NewNSRecord("example.com", 86400, "ns1.example.com"),
		*NewNSRecord("example.com", 86400, "ns2.example.com"),
	}
	resp.Additional = []ResourceRecord{
		*NewARecord("ns1.example.com", 86400, net.ParseIP("10.0.0.100")),
		*NewARecord("ns2.example.com", 86400, net.ParseIP("10.0.0.101")),
	}

	data, err := resp.Pack()
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	// Unpack should work correctly with compression
	unpacked := &Message{}
	err = unpacked.Unpack(data)
	if err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	if len(unpacked.Authority) != 2 {
		t.Errorf("Expected 2 authority records, got %d", len(unpacked.Authority))
	}
	if len(unpacked.Additional) != 2 {
		t.Errorf("Expected 2 additional records, got %d", len(unpacked.Additional))
	}
}

func TestMessage_Flags(t *testing.T) {
	msg := &Message{}

	// Test all flag constants
	msg.Header.Flags = FlagQR | FlagAA | FlagTC | FlagRD | FlagRA | FlagAD | FlagCD

	if !msg.Header.IsResponse() {
		t.Error("Expected response flag")
	}
	if msg.Header.Flags&FlagAA == 0 {
		t.Error("Expected AA flag")
	}
	if msg.Header.Flags&FlagTC == 0 {
		t.Error("Expected TC flag")
	}
	if msg.Header.Flags&FlagRD == 0 {
		t.Error("Expected RD flag")
	}
	if msg.Header.Flags&FlagRA == 0 {
		t.Error("Expected RA flag")
	}
	if msg.Header.Flags&FlagAD == 0 {
		t.Error("Expected AD flag")
	}
	if msg.Header.Flags&FlagCD == 0 {
		t.Error("Expected CD flag")
	}
}

func TestMessage_PackLongTXT(t *testing.T) {
	// TXT records are split into 255-byte chunks
	longText := make([]byte, 300)
	for i := range longText {
		longText[i] = 'a'
	}

	query := NewQuery("example.com", TypeTXT)
	resp := NewResponse(query)
	resp.Answers = []ResourceRecord{
		*NewTXTRecord("example.com", 300, string(longText)),
	}

	data, err := resp.Pack()
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}

	unpacked := &Message{}
	err = unpacked.Unpack(data)
	if err != nil {
		t.Fatalf("Unpack error: %v", err)
	}

	txt := unpacked.Answers[0].GetTXT()
	if len(txt) == 0 {
		t.Fatal("Expected TXT data")
	}
}

func TestHeaderCounts(t *testing.T) {
	msg := &Message{
		Questions: []Question{
			{Name: "q1.com", Type: TypeA, Class: ClassIN},
			{Name: "q2.com", Type: TypeA, Class: ClassIN},
		},
		Answers: []ResourceRecord{
			*NewARecord("a1.com", 300, net.ParseIP("10.0.0.1")),
		},
		Authority: []ResourceRecord{
			*NewNSRecord("auth.com", 300, "ns1.auth.com"),
			*NewNSRecord("auth.com", 300, "ns2.auth.com"),
		},
		Additional: []ResourceRecord{
			*NewARecord("add.com", 300, net.ParseIP("10.0.0.2")),
			*NewARecord("add2.com", 300, net.ParseIP("10.0.0.3")),
			*NewARecord("add3.com", 300, net.ParseIP("10.0.0.4")),
		},
	}

	data, _ := msg.Pack()
	unpacked := &Message{}
	unpacked.Unpack(data)

	if unpacked.Header.QDCount != 2 {
		t.Errorf("QDCount = %d, want 2", unpacked.Header.QDCount)
	}
	if unpacked.Header.ANCount != 1 {
		t.Errorf("ANCount = %d, want 1", unpacked.Header.ANCount)
	}
	if unpacked.Header.NSCount != 2 {
		t.Errorf("NSCount = %d, want 2", unpacked.Header.NSCount)
	}
	if unpacked.Header.ARCount != 3 {
		t.Errorf("ARCount = %d, want 3", unpacked.Header.ARCount)
	}
}
