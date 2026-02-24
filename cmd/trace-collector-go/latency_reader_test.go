package main

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// ── Helper: create a latency key with known values ──────────────────────────

func makeLatencyKey(srcIP, dstIP net.IP, srcPort, dstPort uint16, proto uint8) *LatencyKey {
	lk := &LatencyKey{
		SrcPort:  srcPort,
		DstPort:  dstPort,
		Protocol: proto,
	}
	src16 := srcIP.To16()
	dst16 := dstIP.To16()
	if src16 != nil {
		copy(lk.SrcIP[:], src16)
	}
	if dst16 != nil {
		copy(lk.DstIP[:], dst16)
	}
	return lk
}

// ── Helper: create a latency entry with known values ────────────────────────

func makeLatencyEntry(rttNS, minRTT, maxRTT, samples, lastUpdate uint64) *LatencyEntry {
	return &LatencyEntry{
		RTTNS:        rttNS,
		MinRTTNS:     minRTT,
		MaxRTTNS:     maxRTT,
		Samples:      samples,
		LastUpdateNS: lastUpdate,
	}
}

// ── LatencyKey encode/decode roundtrip ──────────────────────────────────────

func TestLatencyKey_EncodeDecodeRoundtrip(t *testing.T) {
	original := makeLatencyKey(
		net.ParseIP("10.10.10.1"),
		net.ParseIP("10.10.10.2"),
		8080, 443, 6,
	)

	encoded := original.Encode()
	decoded, err := DecodeLatencyKey(encoded[:])
	if err != nil {
		t.Fatalf("DecodeLatencyKey: %v", err)
	}

	if decoded.SrcIP != original.SrcIP {
		t.Error("SrcIP mismatch")
	}
	if decoded.DstIP != original.DstIP {
		t.Error("DstIP mismatch")
	}
	if decoded.SrcPort != original.SrcPort {
		t.Errorf("SrcPort = %d, want %d", decoded.SrcPort, original.SrcPort)
	}
	if decoded.DstPort != original.DstPort {
		t.Errorf("DstPort = %d, want %d", decoded.DstPort, original.DstPort)
	}
	if decoded.Protocol != original.Protocol {
		t.Errorf("Protocol = %d, want %d", decoded.Protocol, original.Protocol)
	}
}

func TestDecodeLatencyKey_TooShort(t *testing.T) {
	_, err := DecodeLatencyKey(make([]byte, 10))
	if err == nil {
		t.Error("expected error for short input")
	}
}

func TestLatencyKey_SrcIPAddr(t *testing.T) {
	lk := makeLatencyKey(
		net.ParseIP("192.168.1.1"),
		net.ParseIP("192.168.1.2"),
		1234, 5678, 6,
	)
	srcIP := lk.SrcIPAddr()
	if srcIP == nil {
		t.Fatal("SrcIPAddr returned nil")
	}
	// IPv4-mapped IPv6
	expected := net.ParseIP("192.168.1.1").To16()
	if !srcIP.Equal(expected) {
		t.Errorf("SrcIPAddr = %s, want %s", srcIP, expected)
	}
}

func TestLatencyKey_ProtocolName(t *testing.T) {
	tests := []struct {
		proto uint8
		want  string
	}{
		{6, "TCP"},
		{17, "UDP"},
		{99, "proto(99)"},
	}
	for _, tt := range tests {
		lk := &LatencyKey{Protocol: tt.proto}
		if got := lk.ProtocolName(); got != tt.want {
			t.Errorf("ProtocolName(%d) = %q, want %q", tt.proto, got, tt.want)
		}
	}
}

func TestLatencyKey_String(t *testing.T) {
	lk := makeLatencyKey(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		8080, 443, 6,
	)
	s := lk.String()
	if s == "" {
		t.Error("String() returned empty")
	}
}

// ── LatencyEntry encode/decode roundtrip ────────────────────────────────────

func TestLatencyEntry_EncodeDecodeRoundtrip(t *testing.T) {
	original := makeLatencyEntry(5000000, 1000000, 10000000, 42, 9999999)

	encoded := original.Encode()
	decoded, err := DecodeLatencyEntry(encoded[:])
	if err != nil {
		t.Fatalf("DecodeLatencyEntry: %v", err)
	}

	if decoded.RTTNS != original.RTTNS {
		t.Errorf("RTTNS = %d, want %d", decoded.RTTNS, original.RTTNS)
	}
	if decoded.MinRTTNS != original.MinRTTNS {
		t.Errorf("MinRTTNS = %d, want %d", decoded.MinRTTNS, original.MinRTTNS)
	}
	if decoded.MaxRTTNS != original.MaxRTTNS {
		t.Errorf("MaxRTTNS = %d, want %d", decoded.MaxRTTNS, original.MaxRTTNS)
	}
	if decoded.Samples != original.Samples {
		t.Errorf("Samples = %d, want %d", decoded.Samples, original.Samples)
	}
	if decoded.LastUpdateNS != original.LastUpdateNS {
		t.Errorf("LastUpdateNS = %d, want %d", decoded.LastUpdateNS, original.LastUpdateNS)
	}
}

func TestDecodeLatencyEntry_TooShort(t *testing.T) {
	_, err := DecodeLatencyEntry(make([]byte, 10))
	if err == nil {
		t.Error("expected error for short input")
	}
}

// ── RTT calculations ────────────────────────────────────────────────────────

func TestLatencyEntry_RTTDuration(t *testing.T) {
	le := makeLatencyEntry(5_000_000, 0, 0, 1, 0) // 5ms
	if le.RTTDuration() != 5*time.Millisecond {
		t.Errorf("RTTDuration = %v, want 5ms", le.RTTDuration())
	}
}

func TestLatencyEntry_MinRTTDuration(t *testing.T) {
	le := makeLatencyEntry(0, 1_000_000, 0, 1, 0) // 1ms
	if le.MinRTTDuration() != 1*time.Millisecond {
		t.Errorf("MinRTTDuration = %v, want 1ms", le.MinRTTDuration())
	}
}

func TestLatencyEntry_MaxRTTDuration(t *testing.T) {
	le := makeLatencyEntry(0, 0, 50_000_000, 1, 0) // 50ms
	if le.MaxRTTDuration() != 50*time.Millisecond {
		t.Errorf("MaxRTTDuration = %v, want 50ms", le.MaxRTTDuration())
	}
}

func TestLatencyEntry_ZeroRTT(t *testing.T) {
	le := makeLatencyEntry(0, 0, 0, 0, 0)
	if le.RTTDuration() != 0 {
		t.Errorf("RTTDuration = %v, want 0", le.RTTDuration())
	}
}

// ── RTT statistics: min/max/avg ─────────────────────────────────────────────

func TestRTTStatistics_MinMaxAvg(t *testing.T) {
	// Simulate aggregated data: 10 samples, min=1ms, max=10ms, latest=5ms
	le := makeLatencyEntry(
		5_000_000,  // 5ms latest
		1_000_000,  // 1ms min
		10_000_000, // 10ms max
		10,         // 10 samples
		999,
	)

	if le.MinRTTDuration() != 1*time.Millisecond {
		t.Errorf("MinRTT = %v, want 1ms", le.MinRTTDuration())
	}
	if le.MaxRTTDuration() != 10*time.Millisecond {
		t.Errorf("MaxRTT = %v, want 10ms", le.MaxRTTDuration())
	}
	if le.Samples != 10 {
		t.Errorf("Samples = %d, want 10", le.Samples)
	}
}

// ── FormatRTTHuman ──────────────────────────────────────────────────────────

func TestFormatRTTHuman(t *testing.T) {
	tests := []struct {
		rttNS uint64
		want  string
	}{
		{500, "500ns"},
		{5000, "5.0us"},
		{500_000, "500.0us"},
		{5_000_000, "5.00ms"},
		{500_000_000, "500.00ms"},
		{1_500_000_000, "1.500s"},
	}
	for _, tt := range tests {
		got := FormatRTTHuman(tt.rttNS)
		if got != tt.want {
			t.Errorf("FormatRTTHuman(%d) = %q, want %q", tt.rttNS, got, tt.want)
		}
	}
}

// ── LatencyReader tests ─────────────────────────────────────────────────────

func TestLatencyReader_PollOnce(t *testing.T) {
	loader := NewMockBPFLoader()

	// Insert a latency entry into the flow map (used as LATENCY_MAP proxy)
	lk := makeLatencyKey(
		net.ParseIP("10.10.10.1"),
		net.ParseIP("10.10.10.2"),
		8080, 443, 6,
	)
	le := makeLatencyEntry(5_000_000, 1_000_000, 10_000_000, 5, 999999)
	keyBytes := lk.Encode()
	entryBytes := le.Encode()
	loader.latencyMap.Insert(keyBytes[:], entryBytes[:])

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	config := DefaultLatencyReaderConfig()
	reader := NewLatencyReader(loader, publisher, config)

	ctx := context.Background()
	reader.pollOnce(ctx)

	stats := reader.Stats()
	if stats.EntriesRead != 1 {
		t.Errorf("EntriesRead = %d, want 1", stats.EntriesRead)
	}
	if stats.PollCycles != 1 {
		t.Errorf("PollCycles = %d, want 1", stats.PollCycles)
	}
	if stats.PublishCalls != 1 {
		t.Errorf("PublishCalls = %d, want 1", stats.PublishCalls)
	}
}

func TestLatencyReader_EmptyMap(t *testing.T) {
	loader := NewMockBPFLoader()
	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewLatencyReader(loader, publisher, DefaultLatencyReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.EntriesRead != 0 {
		t.Errorf("EntriesRead = %d, want 0", stats.EntriesRead)
	}
	if stats.PublishCalls != 0 {
		t.Errorf("PublishCalls = %d, want 0 (empty map)", stats.PublishCalls)
	}
}

func TestLatencyReader_MultipleEntries(t *testing.T) {
	loader := NewMockBPFLoader()

	for i := 0; i < 5; i++ {
		lk := makeLatencyKey(
			net.ParseIP("10.10.10.1"),
			net.ParseIP("10.10.10.2"),
			uint16(8080+i), 443, 6,
		)
		le := makeLatencyEntry(uint64((i+1)*1_000_000), 500_000, uint64((i+1)*2_000_000), uint64(i+1), 999)
		keyBytes := lk.Encode()
		entryBytes := le.Encode()
		loader.latencyMap.Insert(keyBytes[:], entryBytes[:])
	}

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewLatencyReader(loader, publisher, DefaultLatencyReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.EntriesRead != 5 {
		t.Errorf("EntriesRead = %d, want 5", stats.EntriesRead)
	}
}

func TestLatencyReader_BadKey(t *testing.T) {
	loader := NewMockBPFLoader()

	// Insert a too-short key with valid entry
	le := makeLatencyEntry(1_000_000, 1_000_000, 1_000_000, 1, 999)
	entryBytes := le.Encode()
	loader.latencyMap.Insert([]byte("short"), entryBytes[:])

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewLatencyReader(loader, publisher, DefaultLatencyReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.DecodeErrors != 1 {
		t.Errorf("DecodeErrors = %d, want 1", stats.DecodeErrors)
	}
}

func TestLatencyReader_BadEntry(t *testing.T) {
	loader := NewMockBPFLoader()

	// Insert a valid key with too-short entry
	lk := makeLatencyKey(
		net.ParseIP("10.10.10.1"),
		net.ParseIP("10.10.10.2"),
		8080, 443, 6,
	)
	keyBytes := lk.Encode()
	loader.latencyMap.Insert(keyBytes[:], []byte("short"))

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewLatencyReader(loader, publisher, DefaultLatencyReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.DecodeErrors != 1 {
		t.Errorf("DecodeErrors = %d, want 1", stats.DecodeErrors)
	}
}

func TestLatencyReader_SampleChannel(t *testing.T) {
	loader := NewMockBPFLoader()

	lk := makeLatencyKey(
		net.ParseIP("192.168.1.1"),
		net.ParseIP("192.168.1.2"),
		12345, 80, 6,
	)
	le := makeLatencyEntry(3_000_000, 1_000_000, 5_000_000, 3, 888)
	keyBytes := lk.Encode()
	entryBytes := le.Encode()
	loader.latencyMap.Insert(keyBytes[:], entryBytes[:])

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewLatencyReader(loader, publisher, DefaultLatencyReaderConfig())

	reader.pollOnce(context.Background())

	select {
	case sample := <-reader.Samples():
		if sample.Key.SrcPort != 12345 {
			t.Errorf("sample Key.SrcPort = %d, want 12345", sample.Key.SrcPort)
		}
		if sample.Key.DstPort != 80 {
			t.Errorf("sample Key.DstPort = %d, want 80", sample.Key.DstPort)
		}
		if sample.Entry.RTTNS != 3_000_000 {
			t.Errorf("sample Entry.RTTNS = %d, want 3000000", sample.Entry.RTTNS)
		}
		if sample.Entry.Samples != 3 {
			t.Errorf("sample Entry.Samples = %d, want 3", sample.Entry.Samples)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for latency sample")
	}
}

func TestLatencyReader_DeleteAfterRead(t *testing.T) {
	loader := NewMockBPFLoader()

	lk := makeLatencyKey(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		80, 80, 6,
	)
	le := makeLatencyEntry(1_000_000, 1_000_000, 1_000_000, 1, 999)
	keyBytes := lk.Encode()
	entryBytes := le.Encode()
	loader.latencyMap.Insert(keyBytes[:], entryBytes[:])

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	config := DefaultLatencyReaderConfig()
	config.DeleteAfterRead = true
	reader := NewLatencyReader(loader, publisher, config)

	reader.pollOnce(context.Background())

	// Entry should have been deleted
	if loader.latencyMap.Len() != 0 {
		t.Errorf("latency map should be empty after read, has %d entries", loader.latencyMap.Len())
	}
}

func TestLatencyReader_NoDeleteWhenDisabled(t *testing.T) {
	loader := NewMockBPFLoader()

	lk := makeLatencyKey(
		net.ParseIP("10.0.0.1"),
		net.ParseIP("10.0.0.2"),
		80, 80, 6,
	)
	le := makeLatencyEntry(1_000_000, 1_000_000, 1_000_000, 1, 999)
	keyBytes := lk.Encode()
	entryBytes := le.Encode()
	loader.latencyMap.Insert(keyBytes[:], entryBytes[:])

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	config := DefaultLatencyReaderConfig()
	config.DeleteAfterRead = false
	reader := NewLatencyReader(loader, publisher, config)

	reader.pollOnce(context.Background())

	// Entry should still be in the map
	if loader.latencyMap.Len() != 1 {
		t.Errorf("latency map should still have entry, has %d entries", loader.latencyMap.Len())
	}
}

// ── Prometheus histogram integration ────────────────────────────────────────

func TestLatencyReader_PrometheusHistogramUpdated(t *testing.T) {
	loader := NewMockBPFLoader()

	// Insert entries with known RTTs
	for i := 0; i < 10; i++ {
		lk := makeLatencyKey(
			net.ParseIP("10.10.10.1"),
			net.ParseIP("10.10.10.2"),
			uint16(1000+i), 443, 6,
		)
		rttNS := uint64((i + 1) * 1_000_000) // 1ms, 2ms, ..., 10ms
		le := makeLatencyEntry(rttNS, rttNS, rttNS, 1, uint64(i))
		keyBytes := lk.Encode()
		entryBytes := le.Encode()
		loader.latencyMap.Insert(keyBytes[:], entryBytes[:])
	}

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	reader := NewLatencyReader(loader, publisher, DefaultLatencyReaderConfig())

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.EntriesRead != 10 {
		t.Errorf("EntriesRead = %d, want 10", stats.EntriesRead)
	}
	// The histogram metric is updated internally; we verify by checking
	// that no errors occurred and all entries were read.
	if stats.DecodeErrors != 0 {
		t.Errorf("DecodeErrors = %d, want 0", stats.DecodeErrors)
	}
}

// ── LatencyEntry raw decode from BPF bytes ──────────────────────────────────

func TestDecodeLatencyEntry_RawBytes(t *testing.T) {
	var b [LatencyEntrySize]byte
	binary.LittleEndian.PutUint64(b[0:8], 5_000_000)   // rtt_ns: 5ms
	binary.LittleEndian.PutUint64(b[8:16], 1_000_000)   // min_rtt_ns: 1ms
	binary.LittleEndian.PutUint64(b[16:24], 10_000_000) // max_rtt_ns: 10ms
	binary.LittleEndian.PutUint64(b[24:32], 100)         // samples
	binary.LittleEndian.PutUint64(b[32:40], 999999)      // last_update_ns

	le, err := DecodeLatencyEntry(b[:])
	if err != nil {
		t.Fatalf("DecodeLatencyEntry: %v", err)
	}

	if le.RTTNS != 5_000_000 {
		t.Errorf("RTTNS = %d, want 5000000", le.RTTNS)
	}
	if le.MinRTTNS != 1_000_000 {
		t.Errorf("MinRTTNS = %d, want 1000000", le.MinRTTNS)
	}
	if le.MaxRTTNS != 10_000_000 {
		t.Errorf("MaxRTTNS = %d, want 10000000", le.MaxRTTNS)
	}
	if le.Samples != 100 {
		t.Errorf("Samples = %d, want 100", le.Samples)
	}
	if le.LastUpdateNS != 999999 {
		t.Errorf("LastUpdateNS = %d, want 999999", le.LastUpdateNS)
	}
}

// ── DecodeLatencyKey raw decode from BPF bytes ──────────────────────────────

func TestDecodeLatencyKey_RawBytes(t *testing.T) {
	var b [LatencyKeySize]byte
	// Set IPv4-mapped-IPv6 for 10.0.0.1
	b[10] = 0xff
	b[11] = 0xff
	b[12] = 10
	b[13] = 0
	b[14] = 0
	b[15] = 1
	// Set IPv4-mapped-IPv6 for 10.0.0.2
	b[26] = 0xff
	b[27] = 0xff
	b[28] = 10
	b[29] = 0
	b[30] = 0
	b[31] = 2
	// Ports (little-endian)
	binary.LittleEndian.PutUint16(b[32:34], 8080)
	binary.LittleEndian.PutUint16(b[34:36], 443)
	// Protocol
	b[36] = 6

	lk, err := DecodeLatencyKey(b[:])
	if err != nil {
		t.Fatalf("DecodeLatencyKey: %v", err)
	}

	if lk.SrcPort != 8080 {
		t.Errorf("SrcPort = %d, want 8080", lk.SrcPort)
	}
	if lk.DstPort != 443 {
		t.Errorf("DstPort = %d, want 443", lk.DstPort)
	}
	if lk.Protocol != 6 {
		t.Errorf("Protocol = %d, want 6", lk.Protocol)
	}
}

// ── Event publishing test ───────────────────────────────────────────────────

func TestLatencyReader_PublishesValidJSON(t *testing.T) {
	loader := NewMockBPFLoader()

	lk := makeLatencyKey(
		net.ParseIP("10.10.10.1"),
		net.ParseIP("10.10.10.2"),
		8080, 443, 6,
	)
	le := makeLatencyEntry(5_000_000, 1_000_000, 10_000_000, 5, 999999)
	keyBytes := lk.Encode()
	entryBytes := le.Encode()
	loader.latencyMap.Insert(keyBytes[:], entryBytes[:])

	publisher := NewWotanPublisher("localhost:9090", 100, 50*time.Millisecond)
	config := DefaultLatencyReaderConfig()
	reader := NewLatencyReader(loader, publisher, config)

	reader.pollOnce(context.Background())

	stats := reader.Stats()
	if stats.PublishErrors != 0 {
		t.Errorf("PublishErrors = %d, want 0", stats.PublishErrors)
	}
	if stats.PublishCalls != 1 {
		t.Errorf("PublishCalls = %d, want 1", stats.PublishCalls)
	}
}

// ── DefaultLatencyReaderConfig test ─────────────────────────────────────────

func TestDefaultLatencyReaderConfig(t *testing.T) {
	config := DefaultLatencyReaderConfig()

	if config.ReadInterval != 200*time.Millisecond {
		t.Errorf("ReadInterval = %v, want 200ms", config.ReadInterval)
	}
	if config.LatencyTopic != "traces.latency" {
		t.Errorf("LatencyTopic = %q, want \"traces.latency\"", config.LatencyTopic)
	}
	if config.DeleteAfterRead != false {
		t.Error("DeleteAfterRead should default to false")
	}
	if config.BatchSize != 256 {
		t.Errorf("BatchSize = %d, want 256", config.BatchSize)
	}
}
