// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux
// +build linux

package netlink

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// =============================================================================
// NlMsgHdr Serialization / Parsing
// =============================================================================

func TestSerializeParseNlMsgHdr(t *testing.T) {
	tests := []struct {
		name string
		hdr  NlMsgHdr
	}{
		{
			name: "zero header",
			hdr:  NlMsgHdr{},
		},
		{
			name: "typical GET request",
			hdr: NlMsgHdr{
				Len:   32,
				Type:  RTM_GETLINK,
				Flags: NLM_F_REQUEST | NLM_F_DUMP,
				Seq:   1,
				Pid:   1234,
			},
		},
		{
			name: "max values",
			hdr: NlMsgHdr{
				Len:   0xFFFFFFFF,
				Type:  0xFFFF,
				Flags: 0xFFFF,
				Seq:   0xFFFFFFFF,
				Pid:   0xFFFFFFFF,
			},
		},
		{
			name: "error message type",
			hdr: NlMsgHdr{
				Len:   20,
				Type:  NLMSG_ERROR,
				Flags: 0,
				Seq:   42,
				Pid:   9999,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := SerializeNlMsgHdr(&tt.hdr)
			if len(buf) != NlMsgHdrLen {
				t.Fatalf("SerializeNlMsgHdr length = %d, want %d", len(buf), NlMsgHdrLen)
			}
			got, err := ParseNlMsgHdr(buf)
			if err != nil {
				t.Fatalf("ParseNlMsgHdr error: %v", err)
			}
			if *got != tt.hdr {
				t.Errorf("round-trip mismatch: got %+v, want %+v", *got, tt.hdr)
			}
		})
	}
}

func TestParseNlMsgHdr_TooShort(t *testing.T) {
	for i := 0; i < NlMsgHdrLen; i++ {
		_, err := ParseNlMsgHdr(make([]byte, i))
		if err == nil {
			t.Errorf("ParseNlMsgHdr(len=%d) expected error, got nil", i)
		}
	}
}

// =============================================================================
// IfInfoMsg Serialization / Parsing
// =============================================================================

func TestSerializeParseIfInfoMsg(t *testing.T) {
	tests := []struct {
		name string
		msg  IfInfoMsg
	}{
		{name: "zero", msg: IfInfoMsg{}},
		{
			name: "loopback",
			msg: IfInfoMsg{
				Family: unix.AF_UNSPEC,
				Type:   0,
				Index:  1,
				Flags:  unix.IFF_UP | unix.IFF_LOOPBACK | unix.IFF_RUNNING,
				Change: 0xFFFFFFFF,
			},
		},
		{
			name: "negative index",
			msg: IfInfoMsg{
				Family: unix.AF_INET,
				Index:  -1,
				Flags:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := SerializeIfInfoMsg(&tt.msg)
			if len(buf) != IfInfoMsgLen {
				t.Fatalf("length = %d, want %d", len(buf), IfInfoMsgLen)
			}
			got, err := ParseIfInfoMsg(buf)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if *got != tt.msg {
				t.Errorf("round-trip mismatch: got %+v, want %+v", *got, tt.msg)
			}
		})
	}
}

func TestParseIfInfoMsg_TooShort(t *testing.T) {
	_, err := ParseIfInfoMsg(make([]byte, IfInfoMsgLen-1))
	if err == nil {
		t.Error("expected error for short buffer")
	}
}

// =============================================================================
// IfAddrMsg Serialization / Parsing
// =============================================================================

func TestSerializeParseIfAddrMsg(t *testing.T) {
	tests := []struct {
		name string
		msg  IfAddrMsg
	}{
		{name: "zero", msg: IfAddrMsg{}},
		{
			name: "ipv4 /24",
			msg: IfAddrMsg{
				Family:    unix.AF_INET,
				PrefixLen: 24,
				Flags:     0,
				Scope:     RT_SCOPE_UNIVERSE,
				Index:     2,
			},
		},
		{
			name: "ipv6 /64",
			msg: IfAddrMsg{
				Family:    unix.AF_INET6,
				PrefixLen: 64,
				Flags:     0x80,
				Scope:     RT_SCOPE_LINK,
				Index:     3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := SerializeIfAddrMsg(&tt.msg)
			if len(buf) != IfAddrMsgLen {
				t.Fatalf("length = %d, want %d", len(buf), IfAddrMsgLen)
			}
			got, err := ParseIfAddrMsg(buf)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if *got != tt.msg {
				t.Errorf("round-trip mismatch: got %+v, want %+v", *got, tt.msg)
			}
		})
	}
}

func TestParseIfAddrMsg_TooShort(t *testing.T) {
	_, err := ParseIfAddrMsg(make([]byte, IfAddrMsgLen-1))
	if err == nil {
		t.Error("expected error for short buffer")
	}
}

// =============================================================================
// RtMsg Serialization / Parsing
// =============================================================================

func TestSerializeParseRtMsg(t *testing.T) {
	tests := []struct {
		name string
		msg  RtMsg
	}{
		{name: "zero", msg: RtMsg{}},
		{
			name: "default route",
			msg: RtMsg{
				Family:   unix.AF_INET,
				DstLen:   0,
				SrcLen:   0,
				Tos:      0,
				Table:    RT_TABLE_MAIN,
				Protocol: RTPROT_STATIC,
				Scope:    RT_SCOPE_UNIVERSE,
				Type:     RTN_UNICAST,
				Flags:    0,
			},
		},
		{
			name: "with flags",
			msg: RtMsg{
				Family:   unix.AF_INET6,
				DstLen:   48,
				SrcLen:   0,
				Table:    RT_TABLE_MAIN,
				Protocol: RTPROT_KERNEL,
				Scope:    RT_SCOPE_LINK,
				Type:     RTN_UNICAST,
				Flags:    0x12345678,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := SerializeRtMsg(&tt.msg)
			if len(buf) != RtMsgLen {
				t.Fatalf("length = %d, want %d", len(buf), RtMsgLen)
			}
			got, err := ParseRtMsg(buf)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if *got != tt.msg {
				t.Errorf("round-trip mismatch: got %+v, want %+v", *got, tt.msg)
			}
		})
	}
}

func TestParseRtMsg_TooShort(t *testing.T) {
	_, err := ParseRtMsg(make([]byte, RtMsgLen-1))
	if err == nil {
		t.Error("expected error for short buffer")
	}
}

// =============================================================================
// TcMsg Serialization / Parsing
// =============================================================================

func TestSerializeParseTcMsg(t *testing.T) {
	tests := []struct {
		name string
		msg  TcMsg
	}{
		{name: "zero", msg: TcMsg{}},
		{
			name: "clsact qdisc",
			msg: TcMsg{
				Family:  unix.AF_UNSPEC,
				Ifindex: 2,
				Handle:  TC_H_CLSACT,
				Parent:  TC_H_CLSACT,
				Info:    0,
			},
		},
		{
			name: "with padding and info",
			msg: TcMsg{
				Family:  unix.AF_UNSPEC,
				Pad1:    0xAB,
				Pad2:    0xCDEF,
				Ifindex: 5,
				Handle:  MakeTcHandle(1, 0),
				Parent:  TC_H_ROOT,
				Info:    0x00010800,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := SerializeTcMsg(&tt.msg)
			if len(buf) != TcMsgLen {
				t.Fatalf("length = %d, want %d", len(buf), TcMsgLen)
			}
			got, err := ParseTcMsg(buf)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if *got != tt.msg {
				t.Errorf("round-trip mismatch: got %+v, want %+v", *got, tt.msg)
			}
		})
	}
}

func TestParseTcMsg_TooShort(t *testing.T) {
	_, err := ParseTcMsg(make([]byte, TcMsgLen-1))
	if err == nil {
		t.Error("expected error for short buffer")
	}
}

// =============================================================================
// GenlMsgHdr Serialization / Parsing
// =============================================================================

func TestSerializeParseGenlMsgHdr(t *testing.T) {
	tests := []struct {
		name string
		hdr  GenlMsgHdr
	}{
		{name: "zero", hdr: GenlMsgHdr{}},
		{
			name: "get family",
			hdr: GenlMsgHdr{
				Cmd:     GENL_CTRL_CMD_GETFAMILY,
				Version: 1,
			},
		},
		{
			name: "max values",
			hdr: GenlMsgHdr{
				Cmd:      0xFF,
				Version:  0xFF,
				Reserved: 0xFFFF,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := SerializeGenlMsgHdr(&tt.hdr)
			if len(buf) != GenlMsgHdrLen {
				t.Fatalf("length = %d, want %d", len(buf), GenlMsgHdrLen)
			}
			got, err := ParseGenlMsgHdr(buf)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if *got != tt.hdr {
				t.Errorf("round-trip mismatch: got %+v, want %+v", *got, tt.hdr)
			}
		})
	}
}

func TestParseGenlMsgHdr_TooShort(t *testing.T) {
	_, err := ParseGenlMsgHdr(make([]byte, GenlMsgHdrLen-1))
	if err == nil {
		t.Error("expected error for short buffer")
	}
}

// =============================================================================
// Attribute Serialization
// =============================================================================

func TestSerializeAttr(t *testing.T) {
	tests := []struct {
		name       string
		attrType   uint16
		data       []byte
		wantMinLen int
	}{
		{
			name:       "empty data",
			attrType:   1,
			data:       []byte{},
			wantMinLen: NlAttrHdrLen,
		},
		{
			name:       "1 byte data aligned to 4",
			attrType:   IFLA_OPERSTATE,
			data:       []byte{0x06},
			wantMinLen: 8,
		},
		{
			name:       "4 byte data already aligned",
			attrType:   IFLA_MTU,
			data:       []byte{0xDC, 0x05, 0x00, 0x00},
			wantMinLen: 8,
		},
		{
			name:       "6 byte MAC address",
			attrType:   IFLA_ADDRESS,
			data:       []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
			wantMinLen: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := SerializeAttr(tt.attrType, tt.data)
			if len(buf) < tt.wantMinLen {
				t.Fatalf("len = %d, want >= %d", len(buf), tt.wantMinLen)
			}
			if len(buf)%4 != 0 {
				t.Errorf("not 4-byte aligned: len = %d", len(buf))
			}
			gotLen := binary.LittleEndian.Uint16(buf[0:2])
			if int(gotLen) != NlAttrHdrLen+len(tt.data) {
				t.Errorf("attr len field = %d, want %d", gotLen, NlAttrHdrLen+len(tt.data))
			}
			gotType := binary.LittleEndian.Uint16(buf[2:4])
			if gotType != tt.attrType {
				t.Errorf("attr type = %d, want %d", gotType, tt.attrType)
			}
		})
	}
}

func TestSerializeAttrU8(t *testing.T) {
	buf := SerializeAttrU8(IFLA_OPERSTATE, 6)
	attrs, err := ParseAttrs(buf)
	if err != nil {
		t.Fatalf("ParseAttrs: %v", err)
	}
	if len(attrs) != 1 {
		t.Fatalf("got %d attrs, want 1", len(attrs))
	}
	val, err := ParseAttrU8(attrs[0].Data)
	if err != nil {
		t.Fatalf("ParseAttrU8: %v", err)
	}
	if val != 6 {
		t.Errorf("val = %d, want 6", val)
	}
}

func TestSerializeAttrU16(t *testing.T) {
	buf := SerializeAttrU16(5, 0xABCD)
	attrs, err := ParseAttrs(buf)
	if err != nil {
		t.Fatalf("ParseAttrs: %v", err)
	}
	val, err := ParseAttrU16(attrs[0].Data)
	if err != nil {
		t.Fatalf("ParseAttrU16: %v", err)
	}
	if val != 0xABCD {
		t.Errorf("val = 0x%X, want 0xABCD", val)
	}
}

func TestSerializeAttrU32(t *testing.T) {
	buf := SerializeAttrU32(IFLA_MTU, 1500)
	attrs, err := ParseAttrs(buf)
	if err != nil {
		t.Fatalf("ParseAttrs: %v", err)
	}
	val, err := ParseAttrU32(attrs[0].Data)
	if err != nil {
		t.Fatalf("ParseAttrU32: %v", err)
	}
	if val != 1500 {
		t.Errorf("val = %d, want 1500", val)
	}
}

func TestSerializeAttrU64(t *testing.T) {
	buf := SerializeAttrU64(7, 0x0102030405060708)
	attrs, err := ParseAttrs(buf)
	if err != nil {
		t.Fatalf("ParseAttrs: %v", err)
	}
	val, err := ParseAttrU64(attrs[0].Data)
	if err != nil {
		t.Fatalf("ParseAttrU64: %v", err)
	}
	if val != 0x0102030405060708 {
		t.Errorf("val = 0x%X, want 0x0102030405060708", val)
	}
}

func TestSerializeAttrString(t *testing.T) {
	tests := []struct {
		name string
		val  string
	}{
		{name: "short name", val: "eth0"},
		{name: "empty string", val: ""},
		{name: "long name", val: "a_very_long_interface_name_12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := SerializeAttrString(IFLA_IFNAME, tt.val)
			attrs, err := ParseAttrs(buf)
			if err != nil {
				t.Fatalf("ParseAttrs: %v", err)
			}
			got := ParseAttrString(attrs[0].Data)
			if got != tt.val {
				t.Errorf("got %q, want %q", got, tt.val)
			}
		})
	}
}

func TestSerializeAttrIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      net.IP
		wantLen int
	}{
		{name: "IPv4", ip: net.ParseIP("192.168.1.1"), wantLen: 4},
		{name: "IPv6", ip: net.ParseIP("fe80::1"), wantLen: 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := SerializeAttrIP(RTA_DST, tt.ip)
			attrs, err := ParseAttrs(buf)
			if err != nil {
				t.Fatalf("ParseAttrs: %v", err)
			}
			if len(attrs[0].Data) != tt.wantLen {
				t.Fatalf("IP data len = %d, want %d", len(attrs[0].Data), tt.wantLen)
			}
			got := ParseAttrIP(attrs[0].Data)
			if !tt.ip.Equal(got) {
				t.Errorf("got %v, want %v", got, tt.ip)
			}
		})
	}
}

// =============================================================================
// ParseAttrs Error Paths
// =============================================================================

func TestParseAttrs_Errors(t *testing.T) {
	tests := []struct {
		name    string
		buf     []byte
		wantErr bool
	}{
		{
			name:    "empty buffer",
			buf:     []byte{},
			wantErr: false, // returns empty slice
		},
		{
			name:    "too short for header",
			buf:     []byte{0x01, 0x02, 0x03},
			wantErr: false, // loop doesn't enter
		},
		{
			name: "attr length too small",
			buf: []byte{
				0x03, 0x00, // len=3, less than NlAttrHdrLen
				0x01, 0x00, // type=1
			},
			wantErr: true,
		},
		{
			name: "attr extends beyond buffer",
			buf: []byte{
				0x08, 0x00, // len=8
				0x01, 0x00, // type=1
				// only 2 bytes of data, needs 4
				0xAA, 0xBB,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseAttrs(tt.buf)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseAttrs_MultipleAttrs(t *testing.T) {
	var buf []byte
	buf = append(buf, SerializeAttrU32(IFLA_MTU, 1500)...)
	buf = append(buf, SerializeAttrString(IFLA_IFNAME, "eth0")...)
	buf = append(buf, SerializeAttrU8(IFLA_OPERSTATE, 6)...)

	attrs, err := ParseAttrs(buf)
	if err != nil {
		t.Fatalf("ParseAttrs: %v", err)
	}
	if len(attrs) != 3 {
		t.Fatalf("got %d attrs, want 3", len(attrs))
	}

	if attrs[0].Type != IFLA_MTU {
		t.Errorf("attr[0].Type = %d, want %d", attrs[0].Type, IFLA_MTU)
	}
	if attrs[1].Type != IFLA_IFNAME {
		t.Errorf("attr[1].Type = %d, want %d", attrs[1].Type, IFLA_IFNAME)
	}
	if attrs[2].Type != IFLA_OPERSTATE {
		t.Errorf("attr[2].Type = %d, want %d", attrs[2].Type, IFLA_OPERSTATE)
	}
}

// =============================================================================
// ParseAttr* Error Paths
// =============================================================================

func TestParseAttrU8_Error(t *testing.T) {
	_, err := ParseAttrU8([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestParseAttrU16_Error(t *testing.T) {
	_, err := ParseAttrU16([]byte{0x01})
	if err == nil {
		t.Error("expected error for 1-byte data")
	}
}

func TestParseAttrU32_Error(t *testing.T) {
	_, err := ParseAttrU32([]byte{0x01, 0x02, 0x03})
	if err == nil {
		t.Error("expected error for 3-byte data")
	}
}

func TestParseAttrU64_Error(t *testing.T) {
	_, err := ParseAttrU64([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	if err == nil {
		t.Error("expected error for 7-byte data")
	}
}

func TestParseAttrString_NoNull(t *testing.T) {
	got := ParseAttrString([]byte("hello"))
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestParseAttrString_WithNull(t *testing.T) {
	got := ParseAttrString([]byte{'h', 'i', 0, 'x'})
	if got != "hi" {
		t.Errorf("got %q, want %q", got, "hi")
	}
}

func TestParseAttrString_Empty(t *testing.T) {
	got := ParseAttrString([]byte{})
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestParseAttrIP_IPv4(t *testing.T) {
	ip := ParseAttrIP([]byte{10, 0, 0, 1})
	if !ip.Equal(net.IPv4(10, 0, 0, 1)) {
		t.Errorf("got %v, want 10.0.0.1", ip)
	}
}

// =============================================================================
// NlMsgBuilder
// =============================================================================

func TestNewNlMsgBuilder_Finalize(t *testing.T) {
	b := NewNlMsgBuilder(RTM_GETLINK, NLM_F_REQUEST|NLM_F_DUMP)
	msg := b.Finalize()

	if len(msg) < NlMsgHdrLen {
		t.Fatalf("message too short: %d", len(msg))
	}

	hdr, err := ParseNlMsgHdr(msg)
	if err != nil {
		t.Fatalf("ParseNlMsgHdr: %v", err)
	}
	if hdr.Len != uint32(len(msg)) {
		t.Errorf("hdr.Len = %d, want %d", hdr.Len, len(msg))
	}
	if hdr.Type != RTM_GETLINK {
		t.Errorf("hdr.Type = %d, want %d", hdr.Type, RTM_GETLINK)
	}
	if hdr.Flags != NLM_F_REQUEST|NLM_F_DUMP {
		t.Errorf("hdr.Flags = 0x%x, want 0x%x", hdr.Flags, NLM_F_REQUEST|NLM_F_DUMP)
	}
}

func TestNlMsgBuilder_AddIfInfoMsg(t *testing.T) {
	b := NewNlMsgBuilder(RTM_GETLINK, NLM_F_REQUEST)
	b.AddIfInfoMsg(&IfInfoMsg{Family: unix.AF_UNSPEC, Index: 1})
	msg := b.Finalize()

	expectedLen := uint32(NlMsgHdrLen + IfInfoMsgLen)
	hdr, _ := ParseNlMsgHdr(msg)
	if hdr.Len != expectedLen {
		t.Errorf("hdr.Len = %d, want %d", hdr.Len, expectedLen)
	}

	ifInfo, err := ParseIfInfoMsg(msg[NlMsgHdrLen:])
	if err != nil {
		t.Fatalf("ParseIfInfoMsg: %v", err)
	}
	if ifInfo.Index != 1 {
		t.Errorf("ifInfo.Index = %d, want 1", ifInfo.Index)
	}
}

func TestNlMsgBuilder_AddIfAddrMsg(t *testing.T) {
	b := NewNlMsgBuilder(RTM_NEWADDR, NLM_F_REQUEST|NLM_F_CREATE)
	b.AddIfAddrMsg(&IfAddrMsg{
		Family:    unix.AF_INET,
		PrefixLen: 24,
		Index:     2,
	})
	msg := b.Finalize()

	ifAddr, err := ParseIfAddrMsg(msg[NlMsgHdrLen:])
	if err != nil {
		t.Fatalf("ParseIfAddrMsg: %v", err)
	}
	if ifAddr.Family != unix.AF_INET {
		t.Errorf("Family = %d, want %d", ifAddr.Family, unix.AF_INET)
	}
	if ifAddr.PrefixLen != 24 {
		t.Errorf("PrefixLen = %d, want 24", ifAddr.PrefixLen)
	}
}

func TestNlMsgBuilder_AddRtMsg(t *testing.T) {
	b := NewNlMsgBuilder(RTM_NEWROUTE, NLM_F_REQUEST|NLM_F_CREATE)
	b.AddRtMsg(&RtMsg{
		Family:   unix.AF_INET,
		DstLen:   24,
		Table:    RT_TABLE_MAIN,
		Protocol: RTPROT_STATIC,
		Type:     RTN_UNICAST,
	})
	msg := b.Finalize()

	rt, err := ParseRtMsg(msg[NlMsgHdrLen:])
	if err != nil {
		t.Fatalf("ParseRtMsg: %v", err)
	}
	if rt.DstLen != 24 {
		t.Errorf("DstLen = %d, want 24", rt.DstLen)
	}
	if rt.Protocol != RTPROT_STATIC {
		t.Errorf("Protocol = %d, want %d", rt.Protocol, RTPROT_STATIC)
	}
}

func TestNlMsgBuilder_AddTcMsg(t *testing.T) {
	b := NewNlMsgBuilder(RTM_NEWQDISC, NLM_F_REQUEST|NLM_F_CREATE)
	b.AddTcMsg(&TcMsg{
		Ifindex: 3,
		Handle:  TC_H_CLSACT,
		Parent:  TC_H_CLSACT,
	})
	msg := b.Finalize()

	tc, err := ParseTcMsg(msg[NlMsgHdrLen:])
	if err != nil {
		t.Fatalf("ParseTcMsg: %v", err)
	}
	if tc.Ifindex != 3 {
		t.Errorf("Ifindex = %d, want 3", tc.Ifindex)
	}
	if tc.Handle != TC_H_CLSACT {
		t.Errorf("Handle = 0x%x, want 0x%x", tc.Handle, TC_H_CLSACT)
	}
}

func TestNlMsgBuilder_AddGenlMsgHdr(t *testing.T) {
	b := NewNlMsgBuilder(GENL_ID_CTRL, NLM_F_REQUEST)
	b.AddGenlMsgHdr(&GenlMsgHdr{Cmd: GENL_CTRL_CMD_GETFAMILY, Version: 1})
	msg := b.Finalize()

	genl, err := ParseGenlMsgHdr(msg[NlMsgHdrLen:])
	if err != nil {
		t.Fatalf("ParseGenlMsgHdr: %v", err)
	}
	if genl.Cmd != GENL_CTRL_CMD_GETFAMILY {
		t.Errorf("Cmd = %d, want %d", genl.Cmd, GENL_CTRL_CMD_GETFAMILY)
	}
}

func TestNlMsgBuilder_AddAttrs(t *testing.T) {
	b := NewNlMsgBuilder(RTM_SETLINK, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfInfoMsg(&IfInfoMsg{Family: unix.AF_UNSPEC, Index: 1})
	b.AddAttrU32(IFLA_MTU, 9000)
	b.AddAttrString(IFLA_IFNAME, "eth0")
	b.AddAttrU8(IFLA_OPERSTATE, 6)
	b.AddAttrU16(5, 0x1234)
	b.AddAttrU64(7, 0xDEADBEEF)
	b.AddAttrIP(RTA_DST, net.ParseIP("10.0.0.0"))
	b.AddAttr(99, []byte{0xAA, 0xBB})

	msg := b.Finalize()
	hdr, _ := ParseNlMsgHdr(msg)
	if hdr.Len != uint32(len(msg)) {
		t.Errorf("hdr.Len = %d, actual len = %d", hdr.Len, len(msg))
	}

	attrs, err := ParseAttrs(msg[NlMsgHdrLen+IfInfoMsgLen:])
	if err != nil {
		t.Fatalf("ParseAttrs: %v", err)
	}
	if len(attrs) != 7 {
		t.Fatalf("got %d attrs, want 7", len(attrs))
	}
}

func TestNlMsgBuilder_NestedAttr(t *testing.T) {
	b := NewNlMsgBuilder(RTM_SETLINK, NLM_F_REQUEST|NLM_F_ACK)
	b.AddIfInfoMsg(&IfInfoMsg{Family: unix.AF_UNSPEC, Index: 1})

	offset := b.AddNestedAttr(IFLA_XDP)
	b.AddAttrU32(IFLA_XDP_FD, 42)
	b.AddAttrU32(IFLA_XDP_FLAGS, XDP_FLAGS_SKB_MODE)
	b.EndNestedAttr(offset)

	msg := b.Finalize()
	attrs, err := ParseAttrs(msg[NlMsgHdrLen+IfInfoMsgLen:])
	if err != nil {
		t.Fatalf("ParseAttrs: %v", err)
	}
	if len(attrs) != 1 {
		t.Fatalf("got %d top-level attrs, want 1", len(attrs))
	}
	if attrs[0].Type != IFLA_XDP {
		t.Errorf("attr type = %d, want %d", attrs[0].Type, IFLA_XDP)
	}

	// Parse nested attributes
	nested, err := ParseAttrs(attrs[0].Data)
	if err != nil {
		t.Fatalf("ParseAttrs nested: %v", err)
	}
	if len(nested) != 2 {
		t.Fatalf("got %d nested attrs, want 2", len(nested))
	}
	fd, _ := ParseAttrU32(nested[0].Data)
	if fd != 42 {
		t.Errorf("XDP_FD = %d, want 42", fd)
	}
	flags, _ := ParseAttrU32(nested[1].Data)
	if flags != XDP_FLAGS_SKB_MODE {
		t.Errorf("XDP_FLAGS = 0x%x, want 0x%x", flags, XDP_FLAGS_SKB_MODE)
	}
}

// =============================================================================
// Utility Functions
// =============================================================================

func TestMakeTcHandle(t *testing.T) {
	tests := []struct {
		name       string
		major      uint16
		minor      uint16
		wantHandle uint32
	}{
		{name: "zero", major: 0, minor: 0, wantHandle: 0},
		{name: "1:0", major: 1, minor: 0, wantHandle: 0x00010000},
		{name: "0:1", major: 0, minor: 1, wantHandle: 0x00000001},
		{name: "ffff:ffff", major: 0xFFFF, minor: 0xFFFF, wantHandle: 0xFFFFFFFF},
		{name: "1:1", major: 1, minor: 1, wantHandle: 0x00010001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeTcHandle(tt.major, tt.minor)
			if got != tt.wantHandle {
				t.Errorf("MakeTcHandle(%d,%d) = 0x%x, want 0x%x",
					tt.major, tt.minor, got, tt.wantHandle)
			}
		})
	}
}

func TestSplitTcHandle(t *testing.T) {
	tests := []struct {
		name      string
		handle    uint32
		wantMajor uint16
		wantMinor uint16
	}{
		{name: "zero", handle: 0, wantMajor: 0, wantMinor: 0},
		{name: "1:0", handle: 0x00010000, wantMajor: 1, wantMinor: 0},
		{name: "0:1", handle: 0x00000001, wantMajor: 0, wantMinor: 1},
		{name: "ffff:ffff", handle: 0xFFFFFFFF, wantMajor: 0xFFFF, wantMinor: 0xFFFF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor := SplitTcHandle(tt.handle)
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Errorf("SplitTcHandle(0x%x) = (%d,%d), want (%d,%d)",
					tt.handle, major, minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestMakeSplitTcHandle_Roundtrip(t *testing.T) {
	for major := uint16(0); major < 10; major++ {
		for minor := uint16(0); minor < 10; minor++ {
			h := MakeTcHandle(major, minor)
			gotMaj, gotMin := SplitTcHandle(h)
			if gotMaj != major || gotMin != minor {
				t.Errorf("roundtrip (%d,%d): got (%d,%d)", major, minor, gotMaj, gotMin)
			}
		}
	}
}

func TestIfaceFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags uint32
		want  string
	}{
		{name: "no flags", flags: 0, want: "NONE"},
		{name: "UP only", flags: unix.IFF_UP, want: "UP"},
		{name: "UP|RUNNING", flags: unix.IFF_UP | unix.IFF_RUNNING, want: "UP|RUNNING"},
		{name: "LOOPBACK|UP|RUNNING", flags: unix.IFF_UP | unix.IFF_LOOPBACK | unix.IFF_RUNNING, want: "UP|LOOPBACK|RUNNING"},
		{name: "PROMISC", flags: unix.IFF_PROMISC, want: "PROMISC"},
		{name: "BROADCAST|MULTICAST", flags: unix.IFF_BROADCAST | unix.IFF_MULTICAST, want: "BROADCAST|MULTICAST"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IfaceFlags(tt.flags)
			if got != tt.want {
				t.Errorf("IfaceFlags(0x%x) = %q, want %q", tt.flags, got, tt.want)
			}
		})
	}
}

func TestOperStateString(t *testing.T) {
	tests := []struct {
		state uint8
		want  string
	}{
		{0, "UNKNOWN"},
		{1, "NOTPRESENT"},
		{2, "DOWN"},
		{3, "LOWERLAYERDOWN"},
		{4, "TESTING"},
		{5, "DORMANT"},
		{6, "UP"},
		{7, "STATE(7)"},
		{255, "STATE(255)"},
	}

	for _, tt := range tests {
		got := OperStateString(tt.state)
		if got != tt.want {
			t.Errorf("OperStateString(%d) = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestRouteTypeString(t *testing.T) {
	tests := []struct {
		typ  uint8
		want string
	}{
		{RTN_UNSPEC, "UNSPEC"},
		{RTN_UNICAST, "UNICAST"},
		{RTN_LOCAL, "LOCAL"},
		{RTN_BROADCAST, "BROADCAST"},
		{RTN_ANYCAST, "ANYCAST"},
		{RTN_MULTICAST, "MULTICAST"},
		{RTN_BLACKHOLE, "BLACKHOLE"},
		{RTN_UNREACHABLE, "UNREACHABLE"},
		{RTN_PROHIBIT, "PROHIBIT"},
		{RTN_THROW, "THROW"},
		{99, "TYPE(99)"},
	}

	for _, tt := range tests {
		got := RouteTypeString(tt.typ)
		if got != tt.want {
			t.Errorf("RouteTypeString(%d) = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestRouteScopeString(t *testing.T) {
	tests := []struct {
		scope uint8
		want  string
	}{
		{RT_SCOPE_UNIVERSE, "UNIVERSE"},
		{RT_SCOPE_SITE, "SITE"},
		{RT_SCOPE_LINK, "LINK"},
		{RT_SCOPE_HOST, "HOST"},
		{RT_SCOPE_NOWHERE, "NOWHERE"},
		{100, "SCOPE(100)"},
	}

	for _, tt := range tests {
		got := RouteScopeString(tt.scope)
		if got != tt.want {
			t.Errorf("RouteScopeString(%d) = %q, want %q", tt.scope, got, tt.want)
		}
	}
}

func TestRouteProtocolString(t *testing.T) {
	tests := []struct {
		proto uint8
		want  string
	}{
		{RTPROT_UNSPEC, "UNSPEC"},
		{RTPROT_REDIRECT, "REDIRECT"},
		{RTPROT_KERNEL, "KERNEL"},
		{RTPROT_BOOT, "BOOT"},
		{RTPROT_STATIC, "STATIC"},
		{RTPROT_DHCP, "DHCP"},
		{RTPROT_BGP, "BGP"},
		{RTPROT_OSPF, "OSPF"},
		{99, "PROTO(99)"},
	}

	for _, tt := range tests {
		got := RouteProtocolString(tt.proto)
		if got != tt.want {
			t.Errorf("RouteProtocolString(%d) = %q, want %q", tt.proto, got, tt.want)
		}
	}
}

func TestMessageTypeName(t *testing.T) {
	tests := []struct {
		msgType uint16
		want    string
	}{
		{NLMSG_NOOP, "NLMSG_NOOP"},
		{NLMSG_ERROR, "NLMSG_ERROR"},
		{NLMSG_DONE, "NLMSG_DONE"},
		{NLMSG_OVERRUN, "NLMSG_OVERRUN"},
		{RTM_NEWLINK, "RTM_NEWLINK"},
		{RTM_DELLINK, "RTM_DELLINK"},
		{RTM_GETLINK, "RTM_GETLINK"},
		{RTM_SETLINK, "RTM_SETLINK"},
		{RTM_NEWADDR, "RTM_NEWADDR"},
		{RTM_DELADDR, "RTM_DELADDR"},
		{RTM_GETADDR, "RTM_GETADDR"},
		{RTM_NEWROUTE, "RTM_NEWROUTE"},
		{RTM_DELROUTE, "RTM_DELROUTE"},
		{RTM_GETROUTE, "RTM_GETROUTE"},
		{RTM_NEWQDISC, "RTM_NEWQDISC"},
		{RTM_DELQDISC, "RTM_DELQDISC"},
		{RTM_GETQDISC, "RTM_GETQDISC"},
		{RTM_NEWTFILTER, "RTM_NEWTFILTER"},
		{RTM_DELTFILTER, "RTM_DELTFILTER"},
		{RTM_GETTFILTER, "RTM_GETTFILTER"},
		{999, "UNKNOWN(999)"},
	}

	for _, tt := range tests {
		got := MessageTypeName(tt.msgType)
		if got != tt.want {
			t.Errorf("MessageTypeName(%d) = %q, want %q", tt.msgType, got, tt.want)
		}
	}
}

// =============================================================================
// NlError
// =============================================================================

func TestNlError_Error(t *testing.T) {
	e := &NlError{Errno: unix.EEXIST, Message: "object exists"}
	got := e.Error()
	if got != "netlink error: object exists (17)" {
		t.Errorf("Error() = %q", got)
	}
}

func TestIsExist(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "EEXIST", err: &NlError{Errno: unix.EEXIST}, want: true},
		{name: "ENOENT", err: &NlError{Errno: unix.ENOENT}, want: false},
		{name: "generic error", err: errors.New("fail"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsExist(tt.err); got != tt.want {
				t.Errorf("IsExist() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsNotExist(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "ENOENT", err: &NlError{Errno: unix.ENOENT}, want: true},
		{name: "ENODEV", err: &NlError{Errno: unix.ENODEV}, want: true},
		{name: "EEXIST", err: &NlError{Errno: unix.EEXIST}, want: false},
		{name: "generic error", err: errors.New("fail"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotExist(tt.err); got != tt.want {
				t.Errorf("IsNotExist() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPermission(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "EPERM", err: &NlError{Errno: unix.EPERM}, want: true},
		{name: "EACCES", err: &NlError{Errno: unix.EACCES}, want: true},
		{name: "ENOENT", err: &NlError{Errno: unix.ENOENT}, want: false},
		{name: "generic error", err: errors.New("fail"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPermission(tt.err); got != tt.want {
				t.Errorf("IsPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// DumpMessage
// =============================================================================

func TestDumpMessage(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		got := DumpMessage([]byte{0x01})
		if got != "invalid message (too short)" {
			t.Errorf("DumpMessage short = %q", got)
		}
	})

	t.Run("header only", func(t *testing.T) {
		hdr := &NlMsgHdr{Len: NlMsgHdrLen, Type: RTM_GETLINK, Flags: NLM_F_REQUEST, Seq: 1, Pid: 100}
		msg := SerializeNlMsgHdr(hdr)
		got := DumpMessage(msg)
		if len(got) == 0 {
			t.Error("DumpMessage returned empty string")
		}
	})

	t.Run("with payload", func(t *testing.T) {
		b := NewNlMsgBuilder(RTM_GETLINK, NLM_F_REQUEST)
		b.AddIfInfoMsg(&IfInfoMsg{Family: unix.AF_UNSPEC})
		msg := b.Finalize()
		got := DumpMessage(msg)
		if len(got) == 0 {
			t.Error("DumpMessage returned empty string")
		}
	})
}

// =============================================================================
// parseLinkMessage
// =============================================================================

func TestParseLinkMessage(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := parseLinkMessage(make([]byte, NlMsgHdrLen+IfInfoMsgLen-1))
		if err == nil {
			t.Error("expected error for short message")
		}
	})

	t.Run("valid message with attributes", func(t *testing.T) {
		b := NewNlMsgBuilder(RTM_NEWLINK, 0)
		b.AddIfInfoMsg(&IfInfoMsg{
			Family: unix.AF_UNSPEC,
			Index:  2,
			Flags:  unix.IFF_UP | unix.IFF_RUNNING,
		})
		b.AddAttrString(IFLA_IFNAME, "eth0")
		b.AddAttrU32(IFLA_MTU, 1500)
		b.AddAttrU8(IFLA_OPERSTATE, 6)
		b.AddAttrU32(IFLA_TXQLEN, 1000)
		b.AddAttrString(IFLA_QDISC, "noqueue")
		b.AddAttrU32(IFLA_MASTER, 0)
		b.AddAttr(IFLA_ADDRESS, []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55})
		msg := b.Finalize()

		link, err := parseLinkMessage(msg)
		if err != nil {
			t.Fatalf("parseLinkMessage: %v", err)
		}
		if link.Index != 2 {
			t.Errorf("Index = %d, want 2", link.Index)
		}
		if link.Name != "eth0" {
			t.Errorf("Name = %q, want %q", link.Name, "eth0")
		}
		if link.MTU != 1500 {
			t.Errorf("MTU = %d, want 1500", link.MTU)
		}
		if link.OperState != 6 {
			t.Errorf("OperState = %d, want 6", link.OperState)
		}
		if link.TxQueueLen != 1000 {
			t.Errorf("TxQueueLen = %d, want 1000", link.TxQueueLen)
		}
		if link.Qdisc != "noqueue" {
			t.Errorf("Qdisc = %q, want %q", link.Qdisc, "noqueue")
		}
		expectedMAC := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
		if link.HardwareAddr.String() != expectedMAC.String() {
			t.Errorf("HardwareAddr = %s, want %s", link.HardwareAddr, expectedMAC)
		}
	})
}

// =============================================================================
// parseAddrMessage
// =============================================================================

func TestParseAddrMessage(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := parseAddrMessage(make([]byte, NlMsgHdrLen+IfAddrMsgLen-1))
		if err == nil {
			t.Error("expected error for short message")
		}
	})

	t.Run("valid message with attributes", func(t *testing.T) {
		b := NewNlMsgBuilder(RTM_NEWADDR, 0)
		b.AddIfAddrMsg(&IfAddrMsg{
			Family:    unix.AF_INET,
			PrefixLen: 24,
			Scope:     RT_SCOPE_UNIVERSE,
			Index:     2,
		})
		b.AddAttr(IFA_ADDRESS, net.ParseIP("192.168.1.100").To4())
		b.AddAttr(IFA_LOCAL, net.ParseIP("192.168.1.100").To4())
		b.AddAttr(IFA_BROADCAST, net.ParseIP("192.168.1.255").To4())
		b.AddAttrString(IFA_LABEL, "eth0")
		msg := b.Finalize()

		addr, err := parseAddrMessage(msg)
		if err != nil {
			t.Fatalf("parseAddrMessage: %v", err)
		}
		if addr.Index != 2 {
			t.Errorf("Index = %d, want 2", addr.Index)
		}
		if addr.Family != unix.AF_INET {
			t.Errorf("Family = %d, want %d", addr.Family, unix.AF_INET)
		}
		if addr.PrefixLen != 24 {
			t.Errorf("PrefixLen = %d, want 24", addr.PrefixLen)
		}
		if !addr.Address.Equal(net.ParseIP("192.168.1.100")) {
			t.Errorf("Address = %v, want 192.168.1.100", addr.Address)
		}
		if !addr.Broadcast.Equal(net.ParseIP("192.168.1.255")) {
			t.Errorf("Broadcast = %v, want 192.168.1.255", addr.Broadcast)
		}
		if addr.Label != "eth0" {
			t.Errorf("Label = %q, want %q", addr.Label, "eth0")
		}
	})
}

// =============================================================================
// parseRouteMessage
// =============================================================================

func TestParseRouteMessage(t *testing.T) {
	t.Run("too short", func(t *testing.T) {
		_, err := parseRouteMessage(make([]byte, NlMsgHdrLen+RtMsgLen-1))
		if err == nil {
			t.Error("expected error for short message")
		}
	})

	t.Run("valid message with attributes", func(t *testing.T) {
		b := NewNlMsgBuilder(RTM_NEWROUTE, 0)
		b.AddRtMsg(&RtMsg{
			Family:   unix.AF_INET,
			DstLen:   24,
			Table:    RT_TABLE_MAIN,
			Protocol: RTPROT_STATIC,
			Scope:    RT_SCOPE_UNIVERSE,
			Type:     RTN_UNICAST,
		})
		b.AddAttr(RTA_DST, net.ParseIP("10.0.0.0").To4())
		b.AddAttr(RTA_GATEWAY, net.ParseIP("10.0.0.1").To4())
		b.AddAttrU32(RTA_OIF, 2)
		b.AddAttrU32(RTA_PRIORITY, 100)
		b.AddAttr(RTA_PREFSRC, net.ParseIP("10.0.0.5").To4())
		msg := b.Finalize()

		route, err := parseRouteMessage(msg)
		if err != nil {
			t.Fatalf("parseRouteMessage: %v", err)
		}
		if route.DstLen != 24 {
			t.Errorf("DstLen = %d, want 24", route.DstLen)
		}
		if !route.Dst.Equal(net.ParseIP("10.0.0.0")) {
			t.Errorf("Dst = %v, want 10.0.0.0", route.Dst)
		}
		if !route.Gateway.Equal(net.ParseIP("10.0.0.1")) {
			t.Errorf("Gateway = %v, want 10.0.0.1", route.Gateway)
		}
		if route.OifIndex != 2 {
			t.Errorf("OifIndex = %d, want 2", route.OifIndex)
		}
		if route.Priority != 100 {
			t.Errorf("Priority = %d, want 100", route.Priority)
		}
		if !route.PrefSrc.Equal(net.ParseIP("10.0.0.5")) {
			t.Errorf("PrefSrc = %v, want 10.0.0.5", route.PrefSrc)
		}
	})
}

// =============================================================================
// Constants Sanity
// =============================================================================

func TestStructureSizes(t *testing.T) {
	if NlMsgHdrLen != 16 {
		t.Errorf("NlMsgHdrLen = %d, want 16", NlMsgHdrLen)
	}
	if NlAttrHdrLen != 4 {
		t.Errorf("NlAttrHdrLen = %d, want 4", NlAttrHdrLen)
	}
	if IfInfoMsgLen != 16 {
		t.Errorf("IfInfoMsgLen = %d, want 16", IfInfoMsgLen)
	}
	if IfAddrMsgLen != 8 {
		t.Errorf("IfAddrMsgLen = %d, want 8", IfAddrMsgLen)
	}
	if RtMsgLen != 12 {
		t.Errorf("RtMsgLen = %d, want 12", RtMsgLen)
	}
	if TcMsgLen != 20 {
		t.Errorf("TcMsgLen = %d, want 20", TcMsgLen)
	}
	if GenlMsgHdrLen != 4 {
		t.Errorf("GenlMsgHdrLen = %d, want 4", GenlMsgHdrLen)
	}
}

func TestRTMConstants(t *testing.T) {
	if RTM_NEWLINK != 16 {
		t.Errorf("RTM_NEWLINK = %d", RTM_NEWLINK)
	}
	if NLM_F_DUMP != NLM_F_ROOT|NLM_F_MATCH {
		t.Errorf("NLM_F_DUMP = 0x%x, want 0x%x", NLM_F_DUMP, NLM_F_ROOT|NLM_F_MATCH)
	}
	if TC_H_CLSACT != TC_H_INGRESS {
		t.Errorf("TC_H_CLSACT != TC_H_INGRESS")
	}
}

// =============================================================================
// Concurrent Serialization (race safety)
// =============================================================================

func TestSerializeNlMsgHdr_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(seq uint32) {
			defer wg.Done()
			hdr := &NlMsgHdr{Len: 32, Type: RTM_GETLINK, Flags: NLM_F_REQUEST, Seq: seq, Pid: seq}
			buf := SerializeNlMsgHdr(hdr)
			got, err := ParseNlMsgHdr(buf)
			if err != nil {
				t.Errorf("parse error: %v", err)
				return
			}
			if got.Seq != seq {
				t.Errorf("seq = %d, want %d", got.Seq, seq)
			}
		}(uint32(i))
	}
	wg.Wait()
}

func TestNlMsgBuilder_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			b := NewNlMsgBuilder(RTM_GETLINK, NLM_F_REQUEST|NLM_F_DUMP)
			b.AddIfInfoMsg(&IfInfoMsg{Family: unix.AF_UNSPEC, Index: int32(idx)})
			b.AddAttrU32(IFLA_MTU, uint32(idx*1000))
			msg := b.Finalize()

			hdr, err := ParseNlMsgHdr(msg)
			if err != nil {
				t.Errorf("parse error: %v", err)
				return
			}
			if hdr.Len != uint32(len(msg)) {
				t.Errorf("length mismatch for idx %d", idx)
			}
		}(i)
	}
	wg.Wait()
}

// =============================================================================
// maskBits helper
// =============================================================================

func TestMaskBits(t *testing.T) {
	tests := []struct {
		name string
		mask net.IPMask
		want int
	}{
		{name: "/24", mask: net.CIDRMask(24, 32), want: 24},
		{name: "/32", mask: net.CIDRMask(32, 32), want: 32},
		{name: "/0", mask: net.CIDRMask(0, 32), want: 0},
		{name: "/64 v6", mask: net.CIDRMask(64, 128), want: 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskBits(tt.mask)
			if got != tt.want {
				t.Errorf("maskBits(%v) = %d, want %d", tt.mask, got, tt.want)
			}
		})
	}
}
