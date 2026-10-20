// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dns

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// fakeUpstream answers every UDP query with whatever reply builds from it,
// and records the IDs it was sent.
type fakeUpstream struct {
	conn *net.UDPConn
	mu   sync.Mutex
	ids  []uint16
}

func startFakeUpstream(t *testing.T, reply func(q *Message) *Message) *fakeUpstream {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	u := &fakeUpstream{conn: conn}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			q := &Message{}
			if err := q.Unpack(buf[:n]); err != nil {
				continue
			}
			u.mu.Lock()
			u.ids = append(u.ids, q.Header.ID)
			u.mu.Unlock()
			if r := reply(q); r != nil {
				data, err := r.Pack()
				if err == nil {
					_, _ = conn.WriteToUDP(data, addr)
				}
			}
		}
	}()
	return u
}

func (u *fakeUpstream) addr() string { return u.conn.LocalAddr().String() }

func (u *fakeUpstream) seen() []uint16 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]uint16(nil), u.ids...)
}

func answerA(q *Message, ip string) *Message {
	r := NewResponse(q)
	r.Answers = append(r.Answers, *NewARecord(q.Questions[0].Name, 60, net.ParseIP(ip)))
	return r
}

func forwardingResolver(upstream string) *Resolver {
	return NewResolver(&ResolverConfig{
		Upstreams:  []string{upstream},
		Timeout:    300 * time.Millisecond,
		MaxRetries: 0,
	}, NewZoneManager())
}

func clientQuery(name string, id uint16) *Message {
	q := NewQuery(name, TypeA)
	q.Header.ID = id
	q.Header.Flags |= FlagRD
	return q
}

// An off-path attacker who guesses the source port must not also get to
// pick the transaction ID: a reply whose ID is not the one sent is not an
// answer, and must not be returned or cached.
func TestForward_RejectsReplyWithWrongID(t *testing.T) {
	up := startFakeUpstream(t, func(q *Message) *Message {
		r := answerA(q, "6.6.6.6")
		r.Header.ID = q.Header.ID + 1
		return r
	})
	r := forwardingResolver(up.addr())

	resp, _ := r.Resolve(context.Background(), clientQuery("victim.example.", 0x1234))
	if resp != nil && len(resp.Answers) > 0 {
		t.Fatalf("accepted a reply whose ID did not match the query: %v", resp.Answers[0].String())
	}
	if _, ok := r.cache.Get("victim.example.", TypeA, ClassIN); ok {
		t.Fatal("cached a reply whose ID did not match the query")
	}
}

// A reply for a different name (or type) is not an answer to this query,
// even with the right ID.
func TestForward_RejectsReplyForAnotherQuestion(t *testing.T) {
	up := startFakeUpstream(t, func(q *Message) *Message {
		other := NewQuery("attacker.example.", TypeA)
		other.Header.ID = q.Header.ID
		return answerA(other, "6.6.6.6")
	})
	r := forwardingResolver(up.addr())

	resp, _ := r.Resolve(context.Background(), clientQuery("victim.example.", 0x1234))
	if resp != nil && len(resp.Answers) > 0 {
		t.Fatalf("accepted a reply for another question: %v", resp.Answers[0].String())
	}
}

// The ID the upstream sees is chosen here, at random, not copied from the
// client; the client gets its own ID back.
func TestForward_UpstreamIDIsFreshAndClientIDRestored(t *testing.T) {
	up := startFakeUpstream(t, func(q *Message) *Message { return answerA(q, "192.0.2.1") })
	r := forwardingResolver(up.addr())

	for i := 0; i < 2; i++ {
		resp, err := r.Resolve(context.Background(), clientQuery("ok"+string(rune('a'+i))+".example.", 0x1234))
		if err != nil || resp == nil || len(resp.Answers) != 1 {
			t.Fatalf("query %d: resp=%v err=%v", i, resp, err)
		}
		if resp.Header.ID != 0x1234 {
			t.Errorf("query %d: client got ID %#x back, want its own 0x1234", i, resp.Header.ID)
		}
	}
	ids := up.seen()
	if len(ids) != 2 {
		t.Fatalf("upstream saw %d queries, want 2", len(ids))
	}
	if ids[0] == 0x1234 && ids[1] == 0x1234 {
		t.Error("upstream saw the client's ID both times: the ID is the client's, not ours")
	}
}

// Query IDs must not be predictable: a counter lets an attacker who saw one
// ID guess the next.
func TestGenerateID_NotSequential(t *testing.T) {
	const n = 1000
	prev := generateID()
	sequential := 0
	for i := 0; i < n; i++ {
		id := generateID()
		if id == prev+1 {
			sequential++
		}
		prev = id
	}
	if sequential > 5 {
		t.Fatalf("%d of %d consecutive IDs were prev+1: IDs are predictable", sequential, n)
	}
}

// generateID is called from concurrent resolutions (run with -race).
func TestGenerateID_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				_ = generateID()
			}
		}()
	}
	wg.Wait()
}

// A forged datagram arriving first is dropped and the real answer, right
// behind it, is still accepted (RFC 5452 §9.1): one spoof must not be enough
// to deny service either.
func TestForward_ForgedReplyFirstThenRealAnswer(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 4096)
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		q := &Message{}
		if q.Unpack(buf[:n]) != nil {
			return
		}
		forged := answerA(q, "6.6.6.6")
		forged.Header.ID = q.Header.ID ^ 0xffff
		real := answerA(q, "192.0.2.7")
		for _, m := range []*Message{forged, real} {
			if data, err := m.Pack(); err == nil {
				_, _ = conn.WriteToUDP(data, addr)
			}
		}
	}()
	r := forwardingResolver(conn.LocalAddr().String())

	resp, err := r.Resolve(context.Background(), clientQuery("real.example.", 0x4242))
	if err != nil || resp == nil || len(resp.Answers) != 1 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	if got := resp.Answers[0].GetIP().String(); got != "192.0.2.7" {
		t.Fatalf("answer %s, want the real 192.0.2.7", got)
	}
}
