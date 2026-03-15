# Protocol Encoding Fuzzing Harnesses

This directory contains Go fuzzing targets for the Unheaded protocol's encoding functions, using Go's native fuzzing framework (available in Go 1.18+).

## Fuzzing Targets

### FuzzVarintRoundtrip (`fuzz_encoding_test.go`)

**Objective:** Fuzz EncodeVarint/DecodeVarint roundtrip to ensure encoding/decoding are proper inverses.

**What it fuzzes:**
- Varint encoding/decoding with all edge cases
- Single-byte values (0-127)
- Multi-byte values up to u64::MAX
- Continuation bit correctness
- Minimal encoding verification

**Expected to catch:**
- Encoding not producing valid varint format
- Decoding failures or panics
- Roundtrip violations: Decode(Encode(x)) != x
- Invalid continuation markers
- Premature termination or overflow

**Seed data includes:**
- 0 (minimum)
- 127 (max single-byte)
- 128 (min two-byte)
- All significant boundaries up to 2^64-1

**Validation checks:**
- Roundtrip identity: Decode(Encode(x)) == x
- Continuation bits correct (high bit for non-final bytes)
- Final byte has no continuation bit
- Encoded output is minimal (no unnecessary bytes)

---

### FuzzExponentRoundtrip (`fuzz_encoding_test.go`)

**Objective:** Fuzz EncodeExponent/DecodeExponent roundtrip for floating-point exponent encoding.

**What it fuzzes:**
- Exponent encoding/decoding for i32 values
- Boundary values (-2^31, 0, 2^31-1)
- Edge cases (large negative/positive exponents)

**Expected to catch:**
- Exponent parsing errors
- Sign handling issues
- Roundtrip violations
- Encoding size inconsistencies

**Seed data includes:**
- -2147483648 (i32::MIN)
- -1000000, -100, -10, -1
- 0
- 1, 10, 100, 1000000
- 2147483647 (i32::MAX)

**Validation checks:**
- Roundtrip identity: Decode(Encode(x)) == x
- Encoding is exactly 4 bytes
- Sign preservation
- Value preservation through encoding

---

### FuzzCRC16CCITT (`fuzz_encoding_test.go`)

**Objective:** Fuzz CRC-16-CCITT computation to verify collision resistance and determinism.

**What it fuzzes:**
- CRC computation on arbitrary byte sequences
- Single-bit change sensitivity
- Length change sensitivity
- Pattern variations

**Expected to catch:**
- Non-deterministic CRC (differs between calls)
- Collision on single-bit changes
- Collision on length changes
- Invalid CRC range (>16-bit)
- Weakness in collision detection

**Seed data includes:**
- Empty bytes
- Single bytes (0x00, 0xFF)
- Sequential and reverse patterns
- Repeated patterns (0xAA, 0x55)
- Text messages

**Validation checks:**
- Reproducibility: CRC(data) always returns same value
- Single-bit sensitivity: CRC(data XOR 0x01) != CRC(data)
- Length sensitivity: CRC(data) != CRC(data + 0x00)
- Range validation: CRC <= 0xFFFF (16-bit)

**Implementation:** CRC-16-CCITT polynomial x^16 + x^12 + x^5 + 1 (0x1021)

---

### FuzzTLVRoundtrip (`fuzz_encoding_test.go`)

**Objective:** Fuzz EncodeTLV/DecodeTLV roundtrip for Type-Length-Value encoding.

**What it fuzzes:**
- TLV encoding/decoding with arbitrary types and values
- Variable-length payloads
- Edge cases (empty values, large values)
- Nested TLV structures

**Expected to catch:**
- Type field corruption
- Length field errors
- Value truncation or padding
- Encoding size miscalculations
- Nested TLV corruption

**Seed data includes:**
- Type 0, empty value
- Single-byte values
- Max type (255) with multi-byte values
- Named TLV structures
- Binary data
- Large values (256+ bytes)

**Validation checks:**
- Type preservation: DecodedType == OriginalType
- Value preservation: DecodedValue == OriginalValue
- Length accuracy: len(DecodedValue) == ExpectedLength
- Nested encoding: Nested(TLV) > TLV (larger)

---

## Running the Fuzzers

### Prerequisites
- Go 1.18+ (for native fuzzing support)

### Run native fuzzing (Go 1.18+)
```bash
# Run a specific fuzzer with time limit
go test -fuzz=FuzzVarintRoundtrip -fuzztime=60s ./pkg/protocol/fuzz

# Run all fuzzers
go test -fuzz=. -fuzztime=60s ./pkg/protocol/fuzz

# Run with parallelism
go test -fuzz=FuzzVarintRoundtrip -fuzztime=60s -parallel=4 ./pkg/protocol/fuzz
```

### Run with coverage
```bash
go test -fuzz=FuzzVarintRoundtrip -fuzztime=60s -cover ./pkg/protocol/fuzz
```

### Run with specific corpus
```bash
# Go automatically uses testdata/fuzz/<FuzzName>/* as initial corpus
mkdir -p testdata/fuzz/FuzzVarintRoundtrip
# Add seed corpus files to testdata/fuzz/FuzzVarintRoundtrip/
go test -fuzz=FuzzVarintRoundtrip -fuzztime=60s ./pkg/protocol/fuzz
```

### Run specific test from corpus
```bash
# Reproduce a specific crash (crash cache stored by Go)
go test -run=FuzzVarintRoundtrip/hash-of-failing-input ./pkg/protocol/fuzz
```

---

## Success Criteria

### FuzzVarintRoundtrip
- All seed values pass roundtrip (100%)
- Fuzz-generated values never violate roundtrip
- Continuation bits always correct
- Minimal encoding maintained (no padding)
- Coverage of all 64-bit boundaries

### FuzzExponentRoundtrip
- All seed values pass roundtrip (100%)
- Size always 4 bytes
- Sign always preserved
- Boundaries never cause overflow

### FuzzCRC16CCITT
- CRC is deterministic (always same for same input)
- Single-bit changes always produce different CRC
- Length changes always produce different CRC
- No collisions on generated inputs
- All values in range [0, 0xFFFF]

### FuzzTLVRoundtrip
- Type always preserved
- Value always preserved
- Length always accurate
- Nested encoding larger than base TLV
- No truncation or corruption

---

## Corpus Preparation

Go's native fuzzing automatically manages corpus in `testdata/fuzz/<FuzzName>/`:

### testdata/fuzz/FuzzVarintRoundtrip/
```
00-seed-0
01-seed-127
02-seed-128
...
```

### testdata/fuzz/FuzzExponentRoundtrip/
```
00-seed-min
01-seed-zero
02-seed-max
...
```

### testdata/fuzz/FuzzCRC16CCITT/
```
00-empty
01-single-zero
02-single-ff
...
```

### testdata/fuzz/FuzzTLVRoundtrip/
```
00-empty-value
01-single-byte
02-max-type
...
```

Corpus entries can be text (decoded) or binary (raw bytes).

---

## Integration with Protocol Implementation

These fuzzers test the following functions (stubs provided):

- `EncodeVarint(buf *bytes.Buffer, v uint64)` - Variable-length integer encoding
- `DecodeVarint(r *bytes.Reader) (uint64, error)` - Variable-length integer decoding
- `EncodeExponent(exp int32) []byte` - 4-byte exponent encoding
- `DecodeExponent(data []byte) (int32, error)` - 4-byte exponent decoding
- `ComputeCRC16CCITT(data []byte) uint16` - CRC-16-CCITT computation
- `EncodeTLV(tlvType uint8, value []byte) []byte` - Type-Length-Value encoding
- `DecodeTLV(data []byte) (uint8, []byte, error)` - Type-Length-Value decoding

All function implementations are stubs in `fuzz_encoding_test.go` and should be replaced with actual protocol implementations.

---

## Monitoring and Metrics

Go's fuzzing framework provides:
- Automatic corpus management
- Failure reproduction via testdata/fuzz crashes
- Coverage tracking (with -cover flag)
- Parallel execution support
- Minimized test cases for failures

Check `testdata/fuzz/<FuzzName>/` for:
- Crash cache (failing inputs)
- Regression tests

---

## Related Documentation

- LICH Campaign Documentation: `docs/security/lich-campaigns.md`
- Dark Grimoire Addendum: `docs/security/dark-grimoire-addendum.md`
- Protocol Encoding Spec: `docs/protocol/encoding.md` (reference)

