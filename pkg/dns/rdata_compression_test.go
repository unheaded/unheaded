// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dns

import (
	"net"
	"testing"
)

// Names first written inside RDATA (an NS target, an MX exchange, SOA
// names) must be recorded at their offset in the message. They were packed
// into a scratch buffer and recorded at scratch offsets, so a later pointer
// to them aimed into the header: garbage for any message ID >= 256, and a
// silent wrong name ("") below that.
func TestPack_RDataNamesCompressAgainstTheMessage(t *testing.T) {
	for _, id := range []uint16{0, 1, 255, 256, 0x1234, 0xffff} {
		query := NewQuery("www.example.com", TypeA)
		query.Header.ID = id
		resp := NewResponse(query)
		resp.Answers = []ResourceRecord{
			*NewARecord("www.example.com", 300, net.ParseIP("10.0.0.1")),
			*NewMXRecord("example.com", 300, 10, "mail.example.com"),
		}
		resp.Authority = []ResourceRecord{
			*NewNSRecord("example.com", 86400, "ns1.example.com"),
			*NewNSRecord("example.com", 86400, "ns2.example.com"),
			*NewSOARecord("example.com", 300, "ns1.example.com", "hostmaster.example.com", 1, 2, 3, 4, 5),
		}
		resp.Additional = []ResourceRecord{
			*NewARecord("ns1.example.com", 86400, net.ParseIP("10.0.0.100")),
			*NewARecord("mail.example.com", 86400, net.ParseIP("10.0.0.25")),
		}
		data, err := resp.Pack()
		if err != nil {
			t.Fatalf("id %#x: pack: %v", id, err)
		}
		got := &Message{}
		if err := got.Unpack(data); err != nil {
			t.Fatalf("id %#x: unpack: %v", id, err)
		}
		want := []string{"ns1.example.com", "mail.example.com"}
		if n := got.Additional[0].Name; n != want[0] {
			t.Errorf("id %#x: additional[0] name %q, want %q", id, n, want[0])
		}
		if n := got.Additional[1].Name; n != want[1] {
			t.Errorf("id %#x: additional[1] name %q, want %q", id, n, want[1])
		}
		if ns := got.Authority[1].Data.String(); ns != "ns2.example.com" {
			t.Errorf("id %#x: second NS %q, want ns2.example.com", id, ns)
		}
		if mx := got.Answers[1].Data.String(); mx == "" {
			t.Errorf("id %#x: MX lost its exchange", id)
		}
	}
}
