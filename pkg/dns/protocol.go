// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package dns implements a DNS server and resolver for Unheaded Kingdom service discovery.
package dns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
)

// DNS message opcodes
const (
	OpcodeQuery  uint8 = 0
	OpcodeIQuery uint8 = 1
	OpcodeStatus uint8 = 2
	OpcodeNotify uint8 = 4
	OpcodeUpdate uint8 = 5
)

// DNS response codes
const (
	RcodeSuccess        uint8 = 0
	RcodeFormatError    uint8 = 1
	RcodeServerFailure  uint8 = 2
	RcodeNameError      uint8 = 3 // NXDOMAIN
	RcodeNotImplemented uint8 = 4
	RcodeRefused        uint8 = 5
)

// DNS record types
const (
	TypeA     uint16 = 1
	TypeNS    uint16 = 2
	TypeCNAME uint16 = 5
	TypeSOA   uint16 = 6
	TypePTR   uint16 = 12
	TypeMX    uint16 = 15
	TypeTXT   uint16 = 16
	TypeAAAA  uint16 = 28
	TypeSRV   uint16 = 33
	TypeOPT   uint16 = 41
	TypeANY   uint16 = 255
)

// DNS classes
const (
	ClassIN  uint16 = 1
	ClassCH  uint16 = 3
	ClassHS  uint16 = 4
	ClassANY uint16 = 255
)

// Header flags
const (
	FlagQR uint16 = 1 << 15 // Query/Response
	FlagAA uint16 = 1 << 10 // Authoritative Answer
	FlagTC uint16 = 1 << 9  // Truncated
	FlagRD uint16 = 1 << 8  // Recursion Desired
	FlagRA uint16 = 1 << 7  // Recursion Available
	FlagAD uint16 = 1 << 5  // Authenticated Data
	FlagCD uint16 = 1 << 4  // Checking Disabled
)

// MaxUDPSize is the maximum UDP DNS message size
const MaxUDPSize = 512

// MaxNameLength is the maximum DNS name length
const MaxNameLength = 255

// MaxLabelLength is the maximum label length
const MaxLabelLength = 63

// MaxCompressionOffset is the largest offset a DNS name-compression pointer can
// encode. RFC 1035 s4.1.4 gives the pointer 14 bits, with the top two bits set
// to 11 as the marker, so any offset >= 0x4000 is unrepresentable.
//
// This matters because `ptr := 0xC000 | offset` does not fail on an oversized
// offset — it silently drops the overlapping bits. An offset of 0x4000 ORs to
// 0xC000, i.e. a pointer to byte 0. For messages larger than 16 KiB (reachable
// over TCP or with EDNS0) that produced pointers aimed at the wrong name, and
// a resolver following them sees a malformed message or a compression loop.
const MaxCompressionOffset = 0x4000

// Errors
var (
	ErrInvalidMessage   = errors.New("dns: invalid message format")
	ErrNameTooLong      = errors.New("dns: name exceeds maximum length")
	ErrLabelTooLong     = errors.New("dns: label exceeds maximum length")
	ErrInvalidLabel     = errors.New("dns: invalid label")
	ErrPointerLoop      = errors.New("dns: pointer loop detected")
	ErrTruncated        = errors.New("dns: message truncated")
	ErrInvalidRecordLen = errors.New("dns: invalid record data length")
)

// Header represents a DNS message header
type Header struct {
	ID      uint16
	Flags   uint16
	QDCount uint16 // Question count
	ANCount uint16 // Answer count
	NSCount uint16 // Authority count
	ARCount uint16 // Additional count
}

// IsQuery returns true if this is a query message
func (h *Header) IsQuery() bool {
	return h.Flags&FlagQR == 0
}

// IsResponse returns true if this is a response message
func (h *Header) IsResponse() bool {
	return h.Flags&FlagQR != 0
}

// Opcode returns the message opcode
func (h *Header) Opcode() uint8 {
	return uint8((h.Flags >> 11) & 0x0F)
}

// SetOpcode sets the message opcode
func (h *Header) SetOpcode(op uint8) {
	h.Flags = (h.Flags & 0x87FF) | (uint16(op&0x0F) << 11)
}

// Rcode returns the response code
func (h *Header) Rcode() uint8 {
	return uint8(h.Flags & 0x0F)
}

// SetRcode sets the response code
func (h *Header) SetRcode(rc uint8) {
	h.Flags = (h.Flags & 0xFFF0) | uint16(rc&0x0F)
}

// Question represents a DNS question
type Question struct {
	Name  string
	Type  uint16
	Class uint16
}

// Message represents a complete DNS message
type Message struct {
	Header     Header
	Questions  []Question
	Answers    []ResourceRecord
	Authority  []ResourceRecord
	Additional []ResourceRecord
}

// NewQuery creates a new DNS query message
func NewQuery(name string, qtype uint16) *Message {
	return &Message{
		Header: Header{
			ID:      generateID(),
			Flags:   FlagRD,
			QDCount: 1,
		},
		Questions: []Question{
			{Name: name, Type: qtype, Class: ClassIN},
		},
	}
}

// NewResponse creates a response message for a query
func NewResponse(query *Message) *Message {
	return &Message{
		Header: Header{
			ID:    query.Header.ID,
			Flags: FlagQR | FlagAA | (query.Header.Flags & FlagRD) | FlagRA,
		},
		Questions: query.Questions,
	}
}

// Pack serializes a DNS message to wire format
func (m *Message) Pack() ([]byte, error) {
	buf := &bytes.Buffer{}

	// Update counts
	m.Header.QDCount = uint16(len(m.Questions))  // #nosec G115 -- bounded by the length of an already-allocated slice
	m.Header.ANCount = uint16(len(m.Answers))    // #nosec G115 -- bounded by the length of an already-allocated slice
	m.Header.NSCount = uint16(len(m.Authority))  // #nosec G115 -- bounded by the length of an already-allocated slice
	m.Header.ARCount = uint16(len(m.Additional)) // #nosec G115 -- bounded by the length of an already-allocated slice

	// Pack header
	if err := binary.Write(buf, binary.BigEndian, &m.Header); err != nil {
		return nil, err
	}

	// Compression map for name pointers
	nameOffsets := make(map[string]uint16)

	// Pack questions
	for _, q := range m.Questions {
		if err := packName(buf, q.Name, nameOffsets); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.BigEndian, q.Type); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.BigEndian, q.Class); err != nil {
			return nil, err
		}
	}

	// Pack resource records
	for _, rr := range m.Answers {
		if err := packResourceRecord(buf, &rr, nameOffsets); err != nil {
			return nil, err
		}
	}
	for _, rr := range m.Authority {
		if err := packResourceRecord(buf, &rr, nameOffsets); err != nil {
			return nil, err
		}
	}
	for _, rr := range m.Additional {
		if err := packResourceRecord(buf, &rr, nameOffsets); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// Unpack deserializes a DNS message from wire format
func (m *Message) Unpack(data []byte) error {
	if len(data) < 12 {
		return ErrInvalidMessage
	}

	// Unpack header
	m.Header.ID = binary.BigEndian.Uint16(data[0:2])
	m.Header.Flags = binary.BigEndian.Uint16(data[2:4])
	m.Header.QDCount = binary.BigEndian.Uint16(data[4:6])
	m.Header.ANCount = binary.BigEndian.Uint16(data[6:8])
	m.Header.NSCount = binary.BigEndian.Uint16(data[8:10])
	m.Header.ARCount = binary.BigEndian.Uint16(data[10:12])

	offset := 12

	// Unpack questions
	m.Questions = make([]Question, 0, m.Header.QDCount)
	for i := uint16(0); i < m.Header.QDCount; i++ {
		name, newOffset, err := unpackName(data, offset)
		if err != nil {
			return err
		}
		offset = newOffset

		if offset+4 > len(data) {
			return ErrTruncated
		}

		q := Question{
			Name:  name,
			Type:  binary.BigEndian.Uint16(data[offset : offset+2]),
			Class: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		}
		m.Questions = append(m.Questions, q)
		offset += 4
	}

	// Unpack answers
	m.Answers = make([]ResourceRecord, 0, m.Header.ANCount)
	for i := uint16(0); i < m.Header.ANCount; i++ {
		rr, newOffset, err := unpackResourceRecord(data, offset)
		if err != nil {
			return err
		}
		m.Answers = append(m.Answers, *rr)
		offset = newOffset
	}

	// Unpack authority
	m.Authority = make([]ResourceRecord, 0, m.Header.NSCount)
	for i := uint16(0); i < m.Header.NSCount; i++ {
		rr, newOffset, err := unpackResourceRecord(data, offset)
		if err != nil {
			return err
		}
		m.Authority = append(m.Authority, *rr)
		offset = newOffset
	}

	// Unpack additional
	m.Additional = make([]ResourceRecord, 0, m.Header.ARCount)
	for i := uint16(0); i < m.Header.ARCount; i++ {
		rr, newOffset, err := unpackResourceRecord(data, offset)
		if err != nil {
			return err
		}
		m.Additional = append(m.Additional, *rr)
		offset = newOffset
	}

	return nil
}

// packName packs a DNS name with compression
func packName(buf *bytes.Buffer, name string, offsets map[string]uint16) error {
	if len(name) > MaxNameLength {
		return ErrNameTooLong
	}

	// Handle root domain
	if name == "." || name == "" {
		buf.WriteByte(0)
		return nil
	}

	// Remove trailing dot
	name = strings.TrimSuffix(name, ".")

	// Check for existing compression pointer
	if offset, ok := offsets[name]; ok {
		ptr := 0xC000 | offset
		_ = binary.Write(buf, binary.BigEndian, uint16(ptr))
		return nil
	}

	// Store offset for compression, but only if it is representable in the
	// 14 bits a pointer allows. Beyond that we simply forgo compressing this
	// name — a longer message is correct; a bad pointer is not.
	if buf.Len() < MaxCompressionOffset {
		offsets[name] = uint16(buf.Len()) // #nosec G115 -- bounded by MaxCompressionOffset above
	}

	// Split into labels
	labels := strings.Split(name, ".")
	for i, label := range labels {
		if len(label) > MaxLabelLength {
			return ErrLabelTooLong
		}
		if len(label) == 0 {
			continue
		}

		// Check for suffix compression
		suffix := strings.Join(labels[i:], ".")
		if i > 0 {
			if offset, ok := offsets[suffix]; ok {
				ptr := 0xC000 | offset
				_ = binary.Write(buf, binary.BigEndian, uint16(ptr))
				return nil
			}
			if buf.Len() < MaxCompressionOffset {
				offsets[suffix] = uint16(buf.Len()) // #nosec G115 -- bounded by MaxCompressionOffset above
			}
		}

		buf.WriteByte(byte(len(label))) // #nosec G115 -- bounded by construction; see the surrounding guard
		buf.WriteString(label)
	}

	buf.WriteByte(0)
	return nil
}

// unpackName unpacks a DNS name with compression pointer support
func unpackName(data []byte, offset int) (string, int, error) {
	var labels []string
	visited := make(map[int]bool)
	originalOffset := offset
	jumped := false

	for {
		if offset >= len(data) {
			return "", 0, ErrTruncated
		}

		length := int(data[offset])

		if length == 0 {
			offset++
			break
		}

		// Check for compression pointer
		if length&0xC0 == 0xC0 {
			if offset+1 >= len(data) {
				return "", 0, ErrTruncated
			}

			ptr := int(binary.BigEndian.Uint16(data[offset:offset+2])) & 0x3FFF
			if visited[ptr] {
				return "", 0, ErrPointerLoop
			}
			visited[ptr] = true

			if !jumped {
				originalOffset = offset + 2
				jumped = true
			}
			offset = ptr
			continue
		}

		offset++
		if offset+length > len(data) {
			return "", 0, ErrTruncated
		}

		labels = append(labels, string(data[offset:offset+length]))
		offset += length
	}

	name := strings.Join(labels, ".")
	if jumped {
		return name, originalOffset, nil
	}
	return name, offset, nil
}

// packResourceRecord packs a resource record
func packResourceRecord(buf *bytes.Buffer, rr *ResourceRecord, offsets map[string]uint16) error {
	if err := packName(buf, rr.Name, offsets); err != nil {
		return err
	}

	if err := binary.Write(buf, binary.BigEndian, rr.Type); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, rr.Class); err != nil {
		return err
	}
	if err := binary.Write(buf, binary.BigEndian, rr.TTL); err != nil {
		return err
	}

	// Pack RData
	rdataBuf := &bytes.Buffer{}
	if err := rr.packRData(rdataBuf, offsets); err != nil {
		return err
	}

	rdata := rdataBuf.Bytes()
	// RDLENGTH is a 16-bit field. Truncating here would emit a length that
	// disagrees with the bytes that follow, desynchronising every parser
	// reading the rest of the message.
	if len(rdata) > math.MaxUint16 {
		return fmt.Errorf("dns: rdata %d bytes exceeds the 16-bit RDLENGTH field", len(rdata))
	}
	if err := binary.Write(buf, binary.BigEndian, uint16(len(rdata))); err != nil { // #nosec G115 -- bounded by the MaxUint16 check above
		return err
	}
	buf.Write(rdata)

	return nil
}

// unpackResourceRecord unpacks a resource record
func unpackResourceRecord(data []byte, offset int) (*ResourceRecord, int, error) {
	name, newOffset, err := unpackName(data, offset)
	if err != nil {
		return nil, 0, err
	}
	offset = newOffset

	if offset+10 > len(data) {
		return nil, 0, ErrTruncated
	}

	rr := &ResourceRecord{
		Name:  name,
		Type:  binary.BigEndian.Uint16(data[offset : offset+2]),
		Class: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		TTL:   binary.BigEndian.Uint32(data[offset+4 : offset+8]),
	}

	rdlen := binary.BigEndian.Uint16(data[offset+8 : offset+10])
	offset += 10

	if offset+int(rdlen) > len(data) {
		return nil, 0, ErrTruncated
	}

	if err := rr.unpackRData(data, offset, int(rdlen)); err != nil {
		return nil, 0, err
	}

	return rr, offset + int(rdlen), nil
}

// TypeToString converts a record type to string
func TypeToString(t uint16) string {
	switch t {
	case TypeA:
		return "A"
	case TypeNS:
		return "NS"
	case TypeCNAME:
		return "CNAME"
	case TypeSOA:
		return "SOA"
	case TypePTR:
		return "PTR"
	case TypeMX:
		return "MX"
	case TypeTXT:
		return "TXT"
	case TypeAAAA:
		return "AAAA"
	case TypeSRV:
		return "SRV"
	case TypeOPT:
		return "OPT"
	case TypeANY:
		return "ANY"
	default:
		return fmt.Sprintf("TYPE%d", t)
	}
}

// StringToType converts a string to record type
func StringToType(s string) uint16 {
	switch strings.ToUpper(s) {
	case "A":
		return TypeA
	case "NS":
		return TypeNS
	case "CNAME":
		return TypeCNAME
	case "SOA":
		return TypeSOA
	case "PTR":
		return TypePTR
	case "MX":
		return TypeMX
	case "TXT":
		return TypeTXT
	case "AAAA":
		return TypeAAAA
	case "SRV":
		return TypeSRV
	case "OPT":
		return TypeOPT
	case "ANY":
		return TypeANY
	default:
		return 0
	}
}

// ClassToString converts a class to string
func ClassToString(c uint16) string {
	switch c {
	case ClassIN:
		return "IN"
	case ClassCH:
		return "CH"
	case ClassHS:
		return "HS"
	case ClassANY:
		return "ANY"
	default:
		return fmt.Sprintf("CLASS%d", c)
	}
}

// RcodeToString converts a response code to string
func RcodeToString(rc uint8) string {
	switch rc {
	case RcodeSuccess:
		return "NOERROR"
	case RcodeFormatError:
		return "FORMERR"
	case RcodeServerFailure:
		return "SERVFAIL"
	case RcodeNameError:
		return "NXDOMAIN"
	case RcodeNotImplemented:
		return "NOTIMP"
	case RcodeRefused:
		return "REFUSED"
	default:
		return fmt.Sprintf("RCODE%d", rc)
	}
}

var idCounter uint16

// generateID generates a unique message ID
func generateID() uint16 {
	idCounter++
	return idCounter
}
