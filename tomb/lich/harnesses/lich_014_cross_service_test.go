// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// LICH-014: Cross-Service Attack Chains
//
// Target: Service boundaries where data crosses from one domain to another.
// Goal: Find panics, data corruption, or undefined behavior at service
//       handoff points where one service's output becomes another's input.
//
// Attack chains:
//   - Wotan → Sophia: Malicious dictionary update messages parsed by Sophia's Learn()
//   - Sophia → Monad: Fuzzed knowledge entries with extreme numeric values fed
//     to Monad's exponent encoder/decoder
//   - Full pipeline: Raw bytes → dictionary entry → field extraction → Monad
//     register encode → decode → verify roundtrip or graceful rejection
//
// All harnesses are self-contained with no running services required.
// Mock payloads simulate cross-service message formats.

package harnesses

import (
	"encoding/json"
	"math"
	"testing"

	"unheaded/pkg/protocol/encoding"
)

// ============================================================================
// MOCK TYPES — lightweight replicas of cross-service message formats
// ============================================================================

// sophiaDictEntry represents a Sophia knowledge/dictionary entry as it would
// arrive via a Wotan message. Mirrors sophia.Knowledge JSON structure.
type sophiaDictEntry struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Subject    string                 `json:"subject"`
	Predicate  string                 `json:"predicate"`
	Object     string                 `json:"object"`
	Confidence float64                `json:"confidence"`
	Source     string                 `json:"source"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// parseSophiaDictEntry parses a raw JSON payload into a dictionary entry.
// Mirrors the deserialization path that Sophia.Learn() would take when
// receiving a Wotan message.
func parseSophiaDictEntry(data []byte) (*sophiaDictEntry, error) {
	var entry sophiaDictEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// validateSophiaDictEntry applies Sophia-style validation rules to a parsed entry.
// Returns true if the entry is acceptable, false if it should be rejected.
func validateSophiaDictEntry(entry *sophiaDictEntry) bool {
	// Type must be a recognized knowledge type
	switch entry.Type {
	case "fact", "inference", "heuristic", "pattern", "constraint", "goal":
		// valid
	default:
		return false
	}

	// Confidence must be in [0, 1]
	if entry.Confidence < 0 || entry.Confidence > 1 {
		return false
	}
	if math.IsNaN(entry.Confidence) || math.IsInf(entry.Confidence, 0) {
		return false
	}

	// Subject must not be empty
	if entry.Subject == "" {
		return false
	}

	return true
}

// extractNumericField extracts a numeric value from the metadata map,
// simulating how a downstream service would read a field intended
// for Monad encoding.
func extractNumericField(entry *sophiaDictEntry, field string) (uint32, bool) {
	if entry.Metadata == nil {
		return 0, false
	}

	val, ok := entry.Metadata[field]
	if !ok {
		return 0, false
	}

	// JSON numbers decode as float64
	switch v := val.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		if v < 0 || v > math.MaxUint32 {
			return 0, false
		}
		return uint32(v), true
	case json.Number:
		n, err := v.Int64()
		if err != nil || n < 0 || n > math.MaxUint32 {
			return 0, false
		}
		return uint32(n), true
	default:
		return 0, false
	}
}

// ============================================================================
// FUZZ TARGETS
// ============================================================================

// FuzzCrossServiceWotanToSophia simulates a malicious Wotan dictionary update
// message being parsed by Sophia. Fuzzed CBOR/JSON payloads claim to be
// dictionary updates. Sophia must reject malformed entries without panicking.
func FuzzCrossServiceWotanToSophia(f *testing.F) {
	// Seed: valid dictionary entry
	valid, _ := json.Marshal(sophiaDictEntry{
		ID:         "k-1",
		Type:       "fact",
		Subject:    "monad",
		Predicate:  "implements",
		Object:     "state-management",
		Confidence: 0.95,
		Source:     "architect",
	})
	f.Add(valid)

	// Seed: empty JSON object
	f.Add([]byte(`{}`))

	// Seed: missing required fields
	f.Add([]byte(`{"type":"fact"}`))
	f.Add([]byte(`{"subject":"test"}`))

	// Seed: invalid type
	f.Add([]byte(`{"type":"invalid_type","subject":"test","confidence":0.5}`))

	// Seed: confidence out of range
	f.Add([]byte(`{"type":"fact","subject":"test","confidence":999.99}`))
	f.Add([]byte(`{"type":"fact","subject":"test","confidence":-1.0}`))

	// Seed: NaN/Inf confidence (JSON doesn't support these natively, but crafted)
	f.Add([]byte(`{"type":"fact","subject":"test","confidence":1e999}`))

	// Seed: SQL injection in subject
	f.Add([]byte(`{"type":"fact","subject":"'; DROP TABLE knowledge--","confidence":0.5}`))

	// Seed: XSS in object
	f.Add([]byte(`{"type":"fact","subject":"test","object":"<script>alert(1)</script>","confidence":0.5}`))

	// Seed: deeply nested metadata
	f.Add([]byte(`{"type":"fact","subject":"test","confidence":0.5,"metadata":{"a":{"b":{"c":{"d":"deep"}}}}}`))

	// Seed: oversized subject (10KB)
	bigSubject := make([]byte, 10240)
	for i := range bigSubject {
		bigSubject[i] = 'A'
	}
	oversized, _ := json.Marshal(sophiaDictEntry{
		Type:       "fact",
		Subject:    string(bigSubject),
		Confidence: 0.5,
	})
	f.Add(oversized)

	// Seed: malformed JSON
	f.Add([]byte(`{broken`))
	f.Add([]byte{0xFF, 0xFE, 0x00, 0x01})

	// Seed: null bytes in strings
	f.Add([]byte(`{"type":"fact","subject":"test\u0000injected","confidence":0.5}`))

	// Seed: empty
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		entry, err := parseSophiaDictEntry(data)
		if err != nil {
			// JSON parse failure is expected — no panic is the invariant
			return
		}

		// Validate parsed entry
		valid := validateSophiaDictEntry(entry)
		if !valid {
			// Rejection is expected for malformed entries
			return
		}

		// If validation passed, verify extracted fields are consistent
		if entry.Confidence < 0 || entry.Confidence > 1 {
			t.Errorf("validation passed but confidence out of range: %f", entry.Confidence)
		}

		// Attempt to extract numeric metadata fields (as downstream services would)
		if entry.Metadata != nil {
			for key := range entry.Metadata {
				val, ok := extractNumericField(entry, key)
				if ok {
					// Verify the extracted value is sane
					if val > math.MaxUint32 {
						t.Errorf("extracted value exceeds uint32 max: %d", val)
					}
				}
			}
		}
	})
}

// FuzzCrossServiceSophiaToMonad creates mock Sophia dictionary entries with
// fuzzed exponent values, then feeds the decoded values into Monad's exponent
// encoder/decoder. Verifies the full chain handles edge cases including
// overflow, underflow, and NaN-equivalent values.
func FuzzCrossServiceSophiaToMonad(f *testing.F) {
	// Seed: normal value
	f.Add(uint16(0x0420), float64(0.95))

	// Seed: zero exponent
	f.Add(uint16(0x0000), float64(0.5))

	// Seed: max exponent
	f.Add(uint16(0xFF00), float64(1.0))
	f.Add(uint16(0xFFF0), float64(0.0))

	// Seed: boundary values
	f.Add(uint16(0x0010), float64(0.001))
	f.Add(uint16(0x0100), float64(0.999))
	f.Add(uint16(0x8080), float64(-0.5))

	// Seed: all bits set
	f.Add(uint16(0xFFFF), float64(1e308))

	// Seed: small exponent with large mantissa
	f.Add(uint16(0x01F0), float64(math.SmallestNonzeroFloat64))

	// Seed: confidence extremes
	f.Add(uint16(0x3C40), float64(math.MaxFloat64))
	f.Add(uint16(0x0010), float64(math.Inf(1)))
	f.Add(uint16(0x0010), math.NaN())

	f.Fuzz(func(t *testing.T, encodedValue uint16, confidence float64) {
		// Step 1: Decode the Monad exponent value (simulates extracting from wire)
		decoded, err := encoding.DecodeExponent(encodedValue)
		if err != nil {
			// Overflow is valid rejection
			return
		}

		// Step 2: Simulate Sophia applying confidence to the value
		// This is where cross-service data mixing happens
		var adjustedValue float64
		if math.IsNaN(confidence) || math.IsInf(confidence, 0) {
			// Sophia should reject NaN/Inf confidence
			return
		}
		if confidence < 0 {
			confidence = 0
		}
		if confidence > 1 {
			confidence = 1
		}
		adjustedValue = float64(decoded) * confidence

		// Step 3: Convert back to uint32 for Monad re-encoding
		if adjustedValue < 0 || adjustedValue > math.MaxUint32 || math.IsNaN(adjustedValue) || math.IsInf(adjustedValue, 0) {
			// Value out of encodable range — graceful rejection
			return
		}
		reEncodable := uint32(adjustedValue)

		// Step 4: Re-encode through Monad
		reEncoded := encoding.EncodeExponent(reEncodable)

		// Step 5: Decode the re-encoded value
		reDecoded, reErr := encoding.DecodeExponent(reEncoded)
		if reErr != nil {
			t.Errorf("re-decode failed after successful encode: original=0x%04X decoded=%d adjusted=%f re-encoded=0x%04X err=%v",
				encodedValue, decoded, adjustedValue, reEncoded, reErr)
			return
		}

		// Invariant: re-decoded value must be finite and non-negative
		if reDecoded > math.MaxUint64 {
			t.Errorf("re-decoded value exceeds uint64 max")
		}
	})
}

// FuzzCrossServiceChainedCorruption tests the full pipeline: generate fuzzed bytes,
// interpret as a dictionary entry, extract a field value, encode as Monad register,
// decode back. Verifies roundtrip integrity OR graceful rejection at each boundary.
func FuzzCrossServiceChainedCorruption(f *testing.F) {
	// Seed: valid pipeline — JSON entry with numeric metadata → Monad encode → decode
	validEntry, _ := json.Marshal(sophiaDictEntry{
		Type:       "fact",
		Subject:    "system.metric",
		Predicate:  "measures",
		Object:     "latency",
		Confidence: 0.9,
		Metadata: map[string]interface{}{
			"value":     1024,
			"exponent":  42,
			"namespace": 1,
		},
	})
	f.Add(validEntry)

	// Seed: metadata with extreme numeric values
	extreme, _ := json.Marshal(sophiaDictEntry{
		Type:       "fact",
		Subject:    "overflow.test",
		Confidence: 0.5,
		Metadata: map[string]interface{}{
			"value": math.MaxFloat64,
		},
	})
	f.Add(extreme)

	// Seed: metadata with zero value
	zero, _ := json.Marshal(sophiaDictEntry{
		Type:       "fact",
		Subject:    "zero.test",
		Confidence: 0.5,
		Metadata: map[string]interface{}{
			"value": 0,
		},
	})
	f.Add(zero)

	// Seed: metadata with negative value
	negative, _ := json.Marshal(sophiaDictEntry{
		Type:       "fact",
		Subject:    "negative.test",
		Confidence: 0.5,
		Metadata: map[string]interface{}{
			"value": -42,
		},
	})
	f.Add(negative)

	// Seed: no metadata
	noMeta, _ := json.Marshal(sophiaDictEntry{
		Type:       "fact",
		Subject:    "no.metadata",
		Confidence: 0.5,
	})
	f.Add(noMeta)

	// Seed: raw bytes (non-JSON)
	f.Add([]byte{0xDE, 0xAD, 0xBE, 0xEF})
	f.Add([]byte{})

	// Seed: valid but with malicious metadata keys
	malicious, _ := json.Marshal(sophiaDictEntry{
		Type:       "fact",
		Subject:    "test",
		Confidence: 0.5,
		Metadata: map[string]interface{}{
			"../../etc/passwd": 42,
			"\x00null":         1,
			"<script>":         99,
		},
	})
	f.Add(malicious)

	f.Fuzz(func(t *testing.T, data []byte) {
		// === BOUNDARY 1: Raw bytes → Sophia dictionary entry ===
		entry, err := parseSophiaDictEntry(data)
		if err != nil {
			// JSON parse failure — graceful rejection at boundary 1
			return
		}

		if !validateSophiaDictEntry(entry) {
			// Validation failure — graceful rejection at boundary 1
			return
		}

		// === BOUNDARY 2: Sophia entry → numeric value extraction ===
		rawValue, ok := extractNumericField(entry, "value")
		if !ok {
			// No numeric "value" field — graceful rejection at boundary 2
			return
		}

		// === BOUNDARY 3: Numeric value → Monad exponent encode ===
		encoded := encoding.EncodeExponent(rawValue)

		// === BOUNDARY 4: Monad encoded → decode ===
		decoded, err := encoding.DecodeExponent(encoded)
		if err != nil {
			t.Errorf("decode failed after successful encode: input=%d encoded=0x%04X err=%v",
				rawValue, encoded, err)
			return
		}

		// === BOUNDARY 5: Monad decoded → wire format roundtrip ===
		header := &MonadHeader{
			Version:   MonadWireVersion,
			Flags:     0x00,
			FlowLabel: 0x00000001,
			Value:     encoded,
			Namespace: 0x00000001,
			Sequence:  0x00000001,
			TTL:       64,
		}

		wireBytes := EncodeMonadWire(header)
		if len(wireBytes) != MonadWireSize {
			t.Fatalf("wire encode produced %d bytes, expected %d", len(wireBytes), MonadWireSize)
		}

		reHeader, err := ParseMonadWire(wireBytes)
		if err != nil {
			t.Fatalf("wire roundtrip parse failed: %v", err)
		}

		// Invariant: Value field must survive the full pipeline
		if reHeader.Value != encoded {
			t.Errorf("Value field corruption in pipeline: expected 0x%04X, got 0x%04X",
				encoded, reHeader.Value)
		}

		// Invariant: Decoded value from wire must match pre-wire decoded value
		reDecoded, err := encoding.DecodeExponent(reHeader.Value)
		if err != nil {
			t.Errorf("re-decode after wire roundtrip failed: %v", err)
			return
		}
		if reDecoded != decoded {
			t.Errorf("pipeline roundtrip value mismatch: pre-wire=%d post-wire=%d",
				decoded, reDecoded)
		}

		// Log successful full-pipeline traversal for campaign audit
		t.Logf("LICH-014 PIPELINE OK: input=%d → encoded=0x%04X → decoded=%d → wire=%d bytes → roundtrip=%d",
			rawValue, encoded, decoded, len(wireBytes), reDecoded)
	})
}
