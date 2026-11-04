// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"encoding/binary"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "unheaded/proto/unheaded/v1"
)

func sophiaKey(dict, value uint8) []byte {
	k := make([]byte, 2)
	binary.LittleEndian.PutUint16(k, uint16(dict)<<8|uint16(value))
	return k
}

func sophiaValue(name string) []byte {
	v := make([]byte, sophiaEntrySize)
	copy(v[8:], name)
	return v
}

// withBackend installs b as the BPF-mode backend for one test.
func withBackend(t *testing.T, b Backend) {
	t.Helper()
	old, oldMock := globalBackend, globalMockMode
	globalBackend, globalMockMode = b, false
	t.Cleanup(func() { globalBackend, globalMockMode = old, oldMock })
}

func TestSophiaBPF_ListAndGet(t *testing.T) {
	m := NewMockBackend()
	for _, e := range []struct {
		dict, val uint8
		name      string
	}{{1, 3, "gold"}, {1, 4, "silver"}, {3, 7, ""}} {
		if err := m.WriteMap(mapSophia, sophiaKey(e.dict, e.val), sophiaValue(e.name)); err != nil {
			t.Fatal(err)
		}
	}
	withBackend(t, m)
	s := &sophiaServer{}

	resp, err := s.listBPF(&pb.DictionaryListRequest{})
	if err != nil {
		t.Fatalf("listBPF: %v", err)
	}
	if resp.Total != 2 || len(resp.Dictionaries) != 2 {
		t.Fatalf("total=%d len=%d, want 2/2", resp.Total, len(resp.Dictionaries))
	}
	if resp.Dictionaries[0].Id != 1 || resp.Dictionaries[1].Id != 3 {
		t.Fatalf("ids %d,%d, want 1,3", resp.Dictionaries[0].Id, resp.Dictionaries[1].Id)
	}
	if got := resp.Dictionaries[0].Entries[0].Key; got != "gold" {
		t.Fatalf("entry key %q, want gold", got)
	}
	if got := resp.Dictionaries[1].Entries[0].Key; got != "7" {
		t.Fatalf("unnamed entry key %q, want 7", got)
	}

	got, err := s.getBPF(&pb.DictionaryGetRequest{DictionaryId: 3})
	if err != nil || !got.Found {
		t.Fatalf("getBPF(3) = %v, %v", got, err)
	}
	miss, err := s.getBPF(&pb.DictionaryGetRequest{DictionaryId: 9})
	if err != nil || miss.Found {
		t.Fatalf("getBPF(9) = %v, %v; want found=false", miss, err)
	}
}

func TestSophiaBPF_BadEntryRefused(t *testing.T) {
	m := NewMockBackend()
	if err := m.WriteMap(mapSophia, []byte{1}, sophiaValue("x")); err != nil {
		t.Fatal(err)
	}
	withBackend(t, m)
	if _, err := (&sophiaServer{}).listBPF(&pb.DictionaryListRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("listBPF on a 1-byte key = %v, want Unavailable", err)
	}
}

func TestFlowBPF_List(t *testing.T) {
	key := make([]byte, flowKeySize)
	copy(key[0:4], []byte{10, 0, 0, 1})
	copy(key[4:8], []byte{10, 0, 0, 2})
	val := make([]byte, flowStateSize)
	binary.LittleEndian.PutUint64(val[16:24], 111)
	binary.LittleEndian.PutUint64(val[24:32], 222)
	m := NewMockBackend()
	if err := m.WriteMap(mapFlows, key, val); err != nil {
		t.Fatal(err)
	}
	withBackend(t, m)
	resp, err := (&flowServer{}).listBPF()
	if err != nil {
		t.Fatalf("listBPF: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("total = %d, want 1", resp.Total)
	}
	f := resp.Flows[0]
	if f.CreatedAt != 111 || f.LastPacketAt != 222 || f.SourceAddress[3] != 1 || f.DestinationAddress[3] != 2 {
		t.Fatalf("flow = %+v", f)
	}
}

// The calls with no map behind them stay Unimplemented, in so many words.
func TestBPFMode_NoMapStaysUnimplemented(t *testing.T) {
	withBackend(t, NewMockBackend())
	if _, err := (&flowServer{}).GetState(context.Background(), &pb.ReadRequest{FlowLabel: 1}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("flow get = %v, want Unimplemented", err)
	}
	if _, err := (&flowServer{}).List(context.Background(), &emptypb.Empty{}); err != nil {
		t.Fatalf("flow list on an empty map: %v", err)
	}
}
