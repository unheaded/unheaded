// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package anamnesis provides the Anamnesis — the Kingdom's event memory.
// suricata.go: EVE JSON → Anamnesis ring buffer bridge.
// Reads Suricata EVE JSON alerts for Monad protocol signatures (sid 9000001-9000099)
// and publishes them to the Wotan security.suricata.alert topic.
//
// GPL-2.0 isolation boundary: this file reads only the EVE JSON output file.
// No Suricata source code, headers, or shared libraries are referenced.
package anamnesis

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

const (
	// MonadSIDMin is the minimum Suricata SID for Monad protocol rules.
	MonadSIDMin = 9000001
	// MonadSIDMax is the maximum Suricata SID for Monad protocol rules.
	MonadSIDMax = 9000099

	// RingEntrySize is the fixed size of an Anamnesis ring buffer entry in bytes.
	RingEntrySize = 64

	// WotanTopic is the Wotan pub/sub topic for Suricata Monad alerts.
	WotanTopic = "security.suricata.alert"

	// pollInterval is how often to poll eve.json when inotify is unavailable.
	pollInterval = 500 * time.Millisecond
)

// EVEEvent represents a parsed Suricata EVE JSON event.
// Only fields we care about are parsed — all others ignored.
type EVEEvent struct {
	Timestamp string   `json:"timestamp"`
	EventType string   `json:"event_type"`
	SrcIP     string   `json:"src_ip"`
	DestIP    string   `json:"dest_ip"`
	Alert     EVEAlert `json:"alert"`
}

// EVEAlert represents the alert sub-object in an EVE event.
type EVEAlert struct {
	SignatureID uint32 `json:"signature_id"`
	Signature   string `json:"signature"`
	Severity    uint8  `json:"severity"`
}

// RingEntry is a fixed 64-byte Anamnesis ring buffer entry for Suricata alerts.
// Layout (little-endian):
//
//	[0:8]   timestamp  — Unix nanoseconds
//	[8:12]  sid        — Suricata signature_id
//	[12:28] src_ip     — IPv6 address (16 bytes; IPv4 mapped to IPv4-in-IPv6)
//	[28:44] dest_ip    — IPv6 address (16 bytes)
//	[44]    severity   — Suricata severity (1=high, 2=medium, 3=low)
//	[45:64] pad        — zero padding to 64 bytes
type RingEntry [RingEntrySize]byte

// WotanPublisher is the interface for publishing events to Wotan.
// The concrete implementation uses gRPC; tests use MockPublisher.
type WotanPublisher interface {
	Publish(ctx context.Context, topic string, payload []byte) error
}

// SuricataReader watches the EVE JSON file and publishes Monad alerts to Wotan.
type SuricataReader struct {
	eveJSONPath string
	publisher   WotanPublisher
}

// NewSuricataReader creates a new SuricataReader.
// eveJSONPath is the path to /var/log/suricata/eve.json.
// publisher is the Wotan pub/sub client (use MockPublisher in tests).
func NewSuricataReader(eveJSONPath string, publisher WotanPublisher) *SuricataReader {
	if eveJSONPath == "" {
		eveJSONPath = "/var/log/suricata/eve.json"
	}
	return &SuricataReader{
		eveJSONPath: eveJSONPath,
		publisher:   publisher,
	}
}

// Run starts the EVE JSON tail loop. Blocks until ctx is cancelled.
// Graceful degraded mode: if eve.json does not exist, waits and retries every 5s.
func (r *SuricataReader) Run(ctx context.Context) error {
	// Degraded mode: wait for the file to appear
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := os.Stat(r.eveJSONPath); err == nil {
			break
		}
		// File not found — wait and retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}

	return r.tailEVEJSON(ctx)
}

// tailEVEJSON opens and tails the EVE JSON file, publishing Monad alerts.
func (r *SuricataReader) tailEVEJSON(ctx context.Context) error {
	f, err := os.Open(r.eveJSONPath)
	if err != nil {
		return fmt.Errorf("suricata: open eve.json: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Seek to end of file to only process new events
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("suricata: seek to end: %w", err)
	}

	reader := bufio.NewReaderSize(f, 65536)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return fmt.Errorf("suricata: read line: %w", err)
			}
			// No new data — poll
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
			}
			continue
		}

		if err := r.processLine(ctx, line); err != nil {
			// Log and continue — don't crash on one bad line
			_ = err // In production: log via zerolog
			continue
		}
	}
}

// processLine parses one EVE JSON line and publishes it if it is a Monad alert.
func (r *SuricataReader) processLine(ctx context.Context, line string) error {
	if len(line) < 2 {
		return nil
	}

	var event EVEEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return fmt.Errorf("suricata: unmarshal: %w", err)
	}

	// Filter: only alert events with Monad SIDs
	if !isMonadAlert(event) {
		return nil
	}

	entry, err := buildRingEntry(event)
	if err != nil {
		return fmt.Errorf("suricata: build ring entry: %w", err)
	}

	if err := r.publisher.Publish(ctx, WotanTopic, entry[:]); err != nil {
		return fmt.Errorf("suricata: publish to wotan: %w", err)
	}

	return nil
}

// isMonadAlert returns true if the event is an alert with a Monad SID.
func isMonadAlert(event EVEEvent) bool {
	if event.EventType != "alert" {
		return false
	}
	sid := event.Alert.SignatureID
	return sid >= MonadSIDMin && sid <= MonadSIDMax
}

// buildRingEntry creates a 64-byte Anamnesis ring buffer entry from an EVE event.
func buildRingEntry(event EVEEvent) (RingEntry, error) {
	var entry RingEntry

	// Parse timestamp — EVE format: "2026-02-26T04:15:00.123456+0000"
	// Fallback to now if unparseable
	ts := time.Now().UnixNano()
	if event.Timestamp != "" {
		// Try multiple formats
		for _, layout := range []string{
			"2006-01-02T15:04:05.999999-0700",
			"2006-01-02T15:04:05.999999Z07:00",
			time.RFC3339Nano,
			time.RFC3339,
		} {
			if t, err := time.Parse(layout, event.Timestamp); err == nil {
				ts = t.UnixNano()
				break
			}
		}
	}

	// [0:8] timestamp (little-endian int64 nanoseconds)
	binary.LittleEndian.PutUint64(entry[0:8], uint64(ts))

	// [8:12] signature_id (little-endian uint32)
	binary.LittleEndian.PutUint32(entry[8:12], event.Alert.SignatureID)

	// [12:28] src_ip (16 bytes IPv6)
	srcIP := parseIPToIPv6(event.SrcIP)
	copy(entry[12:28], srcIP)

	// [28:44] dest_ip (16 bytes IPv6)
	dstIP := parseIPToIPv6(event.DestIP)
	copy(entry[28:44], dstIP)

	// [44] severity
	entry[44] = event.Alert.Severity

	// [45:64] pad — zero (already zero from zero-value array)

	return entry, nil
}

// parseIPToIPv6 parses an IP string to a 16-byte IPv6 representation.
// IPv4 addresses are represented as IPv4-in-IPv6 (::ffff:x.x.x.x).
// Invalid or empty IPs become all-zeros.
func parseIPToIPv6(ipStr string) []byte {
	result := make([]byte, 16)
	if ipStr == "" {
		return result
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return result
	}

	ip16 := ip.To16()
	if ip16 == nil {
		return result
	}

	copy(result, ip16)
	return result
}
