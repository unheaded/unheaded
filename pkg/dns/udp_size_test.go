// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dns

import (
	"context"
	"net"
	"testing"
	"time"
)

// bigHandler answers every query with n A records, from any handler: the
// size limit has to hold for all of them, not only the default resolver.
func bigHandler(n int) HandlerFunc {
	return func(_ context.Context, w ResponseWriter, r *Message) {
		resp := NewResponse(r)
		for i := 0; i < n; i++ {
			resp.Answers = append(resp.Answers, *NewARecord(r.Questions[0].Name, 60,
				net.IPv4(10, 0, byte(i/256), byte(i%256))))
		}
		_ = w.WriteMsg(resp)
	}
}

// exchangeUDP sends q to a server running h and returns the raw reply.
func exchangeUDP(t *testing.T, h Handler, q *Message) ([]byte, *Message) {
	t.Helper()
	cfg := DefaultServerConfig()
	cfg.UDPAddr = "127.0.0.1:0"
	cfg.TCPAddr = "127.0.0.1:0"
	s := NewServer(cfg)
	s.SetHandler(h)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop() })

	conn, err := net.Dial("udp", s.udpListener.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	data, err := q.Pack()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(data); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	m := &Message{}
	if err := m.Unpack(buf[:n]); err != nil {
		t.Fatalf("reply does not parse: %v", err)
	}
	return buf[:n], m
}

func withEDNS(q *Message, size uint16) *Message {
	q.Additional = append(q.Additional, ResourceRecord{Name: ".", Type: TypeOPT, Class: size})
	return q
}

func optOf(m *Message) *ResourceRecord {
	for i := range m.Additional {
		if m.Additional[i].Type == TypeOPT {
			return &m.Additional[i]
		}
	}
	return nil
}

func TestUDP_SizeLimits(t *testing.T) {
	cases := []struct {
		name     string
		edns     uint16 // 0: no OPT in the query
		records  int
		wantMax  int
		wantTC   bool
		wantOPT  bool
		wantAnsw int // -1: any
	}{
		// No EDNS0: 512 bytes (RFC 1035), and the oversize reply is TC with
		// no partial answer set.
		{"no-edns fits", 0, 10, 512, false, false, 10},
		{"no-edns over 512", 0, 60, 512, true, false, 0},
		// EDNS0 advertises what the client can take; we never send more than
		// 1232 (1280 IPv6 minimum MTU - 40 - 8), so a reply never fragments.
		{"edns 4096 fits 1232", 4096, 60, 1232, false, true, 60},
		{"edns 4096 over 1232", 4096, 200, 1232, true, true, 0},
		{"edns 800 is honoured", 800, 60, 800, true, true, 0},
		{"edns below 512 means 512", 100, 10, 512, false, true, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := NewQuery("big.example.", TypeA)
			if c.edns != 0 {
				q = withEDNS(q, c.edns)
			}
			raw, m := exchangeUDP(t, bigHandler(c.records), q)
			if len(raw) > c.wantMax {
				t.Errorf("reply %d bytes, limit %d", len(raw), c.wantMax)
			}
			if got := m.Header.Flags&FlagTC != 0; got != c.wantTC {
				t.Errorf("TC=%v, want %v (reply %d bytes)", got, c.wantTC, len(raw))
			}
			if got := optOf(m) != nil; got != c.wantOPT {
				t.Errorf("OPT in reply=%v, want %v", got, c.wantOPT)
			}
			if c.wantAnsw >= 0 && len(m.Answers) != c.wantAnsw {
				t.Errorf("%d answers, want %d", len(m.Answers), c.wantAnsw)
			}
			if len(m.Questions) != 1 {
				t.Errorf("%d questions, want the query's 1", len(m.Questions))
			}
		})
	}
}

// The default resolver path counts truncations as before.
func TestUDP_TruncationCounted(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.UDPAddr = "127.0.0.1:0"
	cfg.TCPAddr = "127.0.0.1:0"
	s := NewServer(cfg)
	s.SetHandler(bigHandler(60))
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Stop() }()
	conn, err := net.Dial("udp", s.udpListener.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	data, _ := NewQuery("big.example.", TypeA).Pack()
	_, _ = conn.Write(data)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 65535)); err != nil {
		t.Fatal(err)
	}
	if got := s.Stats().TruncatedUDP; got != 1 {
		t.Fatalf("TruncatedUDP=%d, want 1", got)
	}
}
