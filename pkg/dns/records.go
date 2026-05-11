// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package dns

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
)

// ResourceRecord represents a DNS resource record
type ResourceRecord struct {
	Name  string
	Type  uint16
	Class uint16
	TTL   uint32
	Data  RData
}

// RData is the interface for record data
type RData interface {
	String() string
	pack(buf *bytes.Buffer, offsets map[string]uint16) error
}

// ARecord represents an A (IPv4 address) record
type ARecord struct {
	Address net.IP
}

func (r *ARecord) String() string {
	return r.Address.String()
}

func (r *ARecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	ip := r.Address.To4()
	if ip == nil {
		return fmt.Errorf("invalid IPv4 address")
	}
	buf.Write(ip)
	return nil
}

// AAAARecord represents an AAAA (IPv6 address) record
type AAAARecord struct {
	Address net.IP
}

func (r *AAAARecord) String() string {
	return r.Address.String()
}

func (r *AAAARecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	ip := r.Address.To16()
	if ip == nil {
		return fmt.Errorf("invalid IPv6 address")
	}
	buf.Write(ip)
	return nil
}

// CNAMERecord represents a CNAME (canonical name) record
type CNAMERecord struct {
	Target string
}

func (r *CNAMERecord) String() string {
	return r.Target
}

func (r *CNAMERecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	return packName(buf, r.Target, offsets)
}

// PTRRecord represents a PTR (pointer) record
type PTRRecord struct {
	Target string
}

func (r *PTRRecord) String() string {
	return r.Target
}

func (r *PTRRecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	return packName(buf, r.Target, offsets)
}

// TXTRecord represents a TXT (text) record
type TXTRecord struct {
	Text []string
}

func (r *TXTRecord) String() string {
	return strings.Join(r.Text, " ")
}

func (r *TXTRecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	for _, txt := range r.Text {
		data := []byte(txt)
		// Split into 255-byte chunks
		for len(data) > 0 {
			chunk := data
			if len(chunk) > 255 {
				chunk = data[:255]
			}
			buf.WriteByte(byte(len(chunk)))
			buf.Write(chunk)
			data = data[len(chunk):]
		}
	}
	return nil
}

// SRVRecord represents an SRV (service) record
type SRVRecord struct {
	Priority uint16
	Weight   uint16
	Port     uint16
	Target   string
}

func (r *SRVRecord) String() string {
	return fmt.Sprintf("%d %d %d %s", r.Priority, r.Weight, r.Port, r.Target)
}

func (r *SRVRecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	_ = binary.Write(buf, binary.BigEndian, r.Priority)
	_ = binary.Write(buf, binary.BigEndian, r.Weight)
	_ = binary.Write(buf, binary.BigEndian, r.Port)
	// SRV target should not be compressed
	return packNameNoCompression(buf, r.Target)
}

// MXRecord represents an MX (mail exchange) record
type MXRecord struct {
	Preference uint16
	Exchange   string
}

func (r *MXRecord) String() string {
	return fmt.Sprintf("%d %s", r.Preference, r.Exchange)
}

func (r *MXRecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	_ = binary.Write(buf, binary.BigEndian, r.Preference)
	return packName(buf, r.Exchange, offsets)
}

// NSRecord represents an NS (name server) record
type NSRecord struct {
	NameServer string
}

func (r *NSRecord) String() string {
	return r.NameServer
}

func (r *NSRecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	return packName(buf, r.NameServer, offsets)
}

// SOARecord represents an SOA (start of authority) record
type SOARecord struct {
	MName   string // Primary nameserver
	RName   string // Responsible person email
	Serial  uint32
	Refresh uint32
	Retry   uint32
	Expire  uint32
	Minimum uint32 // Negative caching TTL
}

func (r *SOARecord) String() string {
	return fmt.Sprintf("%s %s %d %d %d %d %d",
		r.MName, r.RName, r.Serial, r.Refresh, r.Retry, r.Expire, r.Minimum)
}

func (r *SOARecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	if err := packName(buf, r.MName, offsets); err != nil {
		return err
	}
	if err := packName(buf, r.RName, offsets); err != nil {
		return err
	}
	_ = binary.Write(buf, binary.BigEndian, r.Serial)
	_ = binary.Write(buf, binary.BigEndian, r.Refresh)
	_ = binary.Write(buf, binary.BigEndian, r.Retry)
	_ = binary.Write(buf, binary.BigEndian, r.Expire)
	_ = binary.Write(buf, binary.BigEndian, r.Minimum)
	return nil
}

// RawRecord represents raw/unknown record data
type RawRecord struct {
	Data []byte
}

func (r *RawRecord) String() string {
	return fmt.Sprintf("[%d bytes]", len(r.Data))
}

func (r *RawRecord) pack(buf *bytes.Buffer, offsets map[string]uint16) error {
	buf.Write(r.Data)
	return nil
}

// packRData packs the record data
func (rr *ResourceRecord) packRData(buf *bytes.Buffer, offsets map[string]uint16) error {
	if rr.Data == nil {
		return nil
	}
	return rr.Data.pack(buf, offsets)
}

// unpackRData unpacks the record data
func (rr *ResourceRecord) unpackRData(data []byte, offset, length int) error {
	switch rr.Type {
	case TypeA:
		if length != 4 {
			return ErrInvalidRecordLen
		}
		rr.Data = &ARecord{Address: net.IP(data[offset : offset+4])}

	case TypeAAAA:
		if length != 16 {
			return ErrInvalidRecordLen
		}
		rr.Data = &AAAARecord{Address: net.IP(data[offset : offset+16])}

	case TypeCNAME:
		name, _, err := unpackName(data, offset)
		if err != nil {
			return err
		}
		rr.Data = &CNAMERecord{Target: name}

	case TypePTR:
		name, _, err := unpackName(data, offset)
		if err != nil {
			return err
		}
		rr.Data = &PTRRecord{Target: name}

	case TypeTXT:
		var texts []string
		end := offset + length
		for offset < end {
			txtLen := int(data[offset])
			offset++
			if offset+txtLen > end {
				return ErrTruncated
			}
			texts = append(texts, string(data[offset:offset+txtLen]))
			offset += txtLen
		}
		rr.Data = &TXTRecord{Text: texts}

	case TypeSRV:
		if length < 6 {
			return ErrInvalidRecordLen
		}
		srv := &SRVRecord{
			Priority: binary.BigEndian.Uint16(data[offset : offset+2]),
			Weight:   binary.BigEndian.Uint16(data[offset+2 : offset+4]),
			Port:     binary.BigEndian.Uint16(data[offset+4 : offset+6]),
		}
		target, _, err := unpackName(data, offset+6)
		if err != nil {
			return err
		}
		srv.Target = target
		rr.Data = srv

	case TypeMX:
		if length < 2 {
			return ErrInvalidRecordLen
		}
		mx := &MXRecord{
			Preference: binary.BigEndian.Uint16(data[offset : offset+2]),
		}
		exchange, _, err := unpackName(data, offset+2)
		if err != nil {
			return err
		}
		mx.Exchange = exchange
		rr.Data = mx

	case TypeNS:
		name, _, err := unpackName(data, offset)
		if err != nil {
			return err
		}
		rr.Data = &NSRecord{NameServer: name}

	case TypeSOA:
		mname, newOffset, err := unpackName(data, offset)
		if err != nil {
			return err
		}
		rname, newOffset, err := unpackName(data, newOffset)
		if err != nil {
			return err
		}
		if newOffset+20 > offset+length {
			return ErrTruncated
		}
		rr.Data = &SOARecord{
			MName:   mname,
			RName:   rname,
			Serial:  binary.BigEndian.Uint32(data[newOffset : newOffset+4]),
			Refresh: binary.BigEndian.Uint32(data[newOffset+4 : newOffset+8]),
			Retry:   binary.BigEndian.Uint32(data[newOffset+8 : newOffset+12]),
			Expire:  binary.BigEndian.Uint32(data[newOffset+12 : newOffset+16]),
			Minimum: binary.BigEndian.Uint32(data[newOffset+16 : newOffset+20]),
		}

	default:
		// Store raw data for unknown types
		rawData := make([]byte, length)
		copy(rawData, data[offset:offset+length])
		rr.Data = &RawRecord{Data: rawData}
	}

	return nil
}

// String returns a string representation of the resource record
func (rr *ResourceRecord) String() string {
	dataStr := ""
	if rr.Data != nil {
		dataStr = rr.Data.String()
	}
	return fmt.Sprintf("%s %d %s %s %s",
		rr.Name, rr.TTL, ClassToString(rr.Class), TypeToString(rr.Type), dataStr)
}

// NewARecord creates an A record
func NewARecord(name string, ttl uint32, ip net.IP) *ResourceRecord {
	return &ResourceRecord{
		Name:  name,
		Type:  TypeA,
		Class: ClassIN,
		TTL:   ttl,
		Data:  &ARecord{Address: ip.To4()},
	}
}

// NewAAAARecord creates an AAAA record
func NewAAAARecord(name string, ttl uint32, ip net.IP) *ResourceRecord {
	return &ResourceRecord{
		Name:  name,
		Type:  TypeAAAA,
		Class: ClassIN,
		TTL:   ttl,
		Data:  &AAAARecord{Address: ip.To16()},
	}
}

// NewCNAMERecord creates a CNAME record
func NewCNAMERecord(name string, ttl uint32, target string) *ResourceRecord {
	return &ResourceRecord{
		Name:  name,
		Type:  TypeCNAME,
		Class: ClassIN,
		TTL:   ttl,
		Data:  &CNAMERecord{Target: target},
	}
}

// NewPTRRecord creates a PTR record
func NewPTRRecord(name string, ttl uint32, target string) *ResourceRecord {
	return &ResourceRecord{
		Name:  name,
		Type:  TypePTR,
		Class: ClassIN,
		TTL:   ttl,
		Data:  &PTRRecord{Target: target},
	}
}

// NewTXTRecord creates a TXT record
func NewTXTRecord(name string, ttl uint32, text ...string) *ResourceRecord {
	return &ResourceRecord{
		Name:  name,
		Type:  TypeTXT,
		Class: ClassIN,
		TTL:   ttl,
		Data:  &TXTRecord{Text: text},
	}
}

// NewSRVRecord creates an SRV record
func NewSRVRecord(name string, ttl uint32, priority, weight, port uint16, target string) *ResourceRecord {
	return &ResourceRecord{
		Name:  name,
		Type:  TypeSRV,
		Class: ClassIN,
		TTL:   ttl,
		Data: &SRVRecord{
			Priority: priority,
			Weight:   weight,
			Port:     port,
			Target:   target,
		},
	}
}

// NewMXRecord creates an MX record
func NewMXRecord(name string, ttl uint32, preference uint16, exchange string) *ResourceRecord {
	return &ResourceRecord{
		Name:  name,
		Type:  TypeMX,
		Class: ClassIN,
		TTL:   ttl,
		Data: &MXRecord{
			Preference: preference,
			Exchange:   exchange,
		},
	}
}

// NewNSRecord creates an NS record
func NewNSRecord(name string, ttl uint32, nameserver string) *ResourceRecord {
	return &ResourceRecord{
		Name:  name,
		Type:  TypeNS,
		Class: ClassIN,
		TTL:   ttl,
		Data:  &NSRecord{NameServer: nameserver},
	}
}

// NewSOARecord creates an SOA record
func NewSOARecord(name string, ttl uint32, mname, rname string, serial, refresh, retry, expire, minimum uint32) *ResourceRecord {
	return &ResourceRecord{
		Name:  name,
		Type:  TypeSOA,
		Class: ClassIN,
		TTL:   ttl,
		Data: &SOARecord{
			MName:   mname,
			RName:   rname,
			Serial:  serial,
			Refresh: refresh,
			Retry:   retry,
			Expire:  expire,
			Minimum: minimum,
		},
	}
}

// packNameNoCompression packs a name without compression (needed for SRV targets)
func packNameNoCompression(buf *bytes.Buffer, name string) error {
	if len(name) > MaxNameLength {
		return ErrNameTooLong
	}

	if name == "." || name == "" {
		buf.WriteByte(0)
		return nil
	}

	name = strings.TrimSuffix(name, ".")
	labels := strings.Split(name, ".")

	for _, label := range labels {
		if len(label) > MaxLabelLength {
			return ErrLabelTooLong
		}
		if len(label) == 0 {
			continue
		}
		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}

	buf.WriteByte(0)
	return nil
}

// GetIP returns the IP address for A or AAAA records
func (rr *ResourceRecord) GetIP() net.IP {
	switch d := rr.Data.(type) {
	case *ARecord:
		return d.Address
	case *AAAARecord:
		return d.Address
	default:
		return nil
	}
}

// GetTarget returns the target for CNAME, PTR, NS, MX, SRV records
func (rr *ResourceRecord) GetTarget() string {
	switch d := rr.Data.(type) {
	case *CNAMERecord:
		return d.Target
	case *PTRRecord:
		return d.Target
	case *NSRecord:
		return d.NameServer
	case *MXRecord:
		return d.Exchange
	case *SRVRecord:
		return d.Target
	default:
		return ""
	}
}

// GetTXT returns the text for TXT records
func (rr *ResourceRecord) GetTXT() []string {
	if d, ok := rr.Data.(*TXTRecord); ok {
		return d.Text
	}
	return nil
}

// GetSRV returns SRV record data
func (rr *ResourceRecord) GetSRV() (priority, weight, port uint16, target string, ok bool) {
	if d, ok := rr.Data.(*SRVRecord); ok {
		return d.Priority, d.Weight, d.Port, d.Target, true
	}
	return 0, 0, 0, "", false
}

// Clone creates a deep copy of the resource record
func (rr *ResourceRecord) Clone() *ResourceRecord {
	clone := &ResourceRecord{
		Name:  rr.Name,
		Type:  rr.Type,
		Class: rr.Class,
		TTL:   rr.TTL,
	}

	// Clone the data
	switch d := rr.Data.(type) {
	case *ARecord:
		ip := make(net.IP, len(d.Address))
		copy(ip, d.Address)
		clone.Data = &ARecord{Address: ip}
	case *AAAARecord:
		ip := make(net.IP, len(d.Address))
		copy(ip, d.Address)
		clone.Data = &AAAARecord{Address: ip}
	case *CNAMERecord:
		clone.Data = &CNAMERecord{Target: d.Target}
	case *PTRRecord:
		clone.Data = &PTRRecord{Target: d.Target}
	case *TXTRecord:
		texts := make([]string, len(d.Text))
		copy(texts, d.Text)
		clone.Data = &TXTRecord{Text: texts}
	case *SRVRecord:
		clone.Data = &SRVRecord{
			Priority: d.Priority,
			Weight:   d.Weight,
			Port:     d.Port,
			Target:   d.Target,
		}
	case *MXRecord:
		clone.Data = &MXRecord{
			Preference: d.Preference,
			Exchange:   d.Exchange,
		}
	case *NSRecord:
		clone.Data = &NSRecord{NameServer: d.NameServer}
	case *SOARecord:
		clone.Data = &SOARecord{
			MName:   d.MName,
			RName:   d.RName,
			Serial:  d.Serial,
			Refresh: d.Refresh,
			Retry:   d.Retry,
			Expire:  d.Expire,
			Minimum: d.Minimum,
		}
	case *RawRecord:
		data := make([]byte, len(d.Data))
		copy(data, d.Data)
		clone.Data = &RawRecord{Data: data}
	}

	return clone
}
