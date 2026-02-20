# Unheaded Protocol Packages Scaffolding

This document describes the three newly created protocol packages based on H2, H3, and Q1 protocol findings.

## Package 1: errors (Protocol Error Registry)

**Location**: `pkg/protocol/errors/`
**Files**: `errors.go`, `errors_test.go`
**Lines**: 290 + 279 = 569 lines

### Core Components

#### ErrorCode (uint16)
- 13 core error codes from UNHD_NO_ERROR (0x0000) to UNHD_INTERNAL_ERROR (0x000c)
- IANA allocation ranges:
  - Standards: 0x0000-0x003F (reserved)
  - Extension: 0x0040-0x00FF (available for extension)
  - Testing: 0x1F*N+0x21 pattern

#### ErrorLevel (uint8)
Three severity levels:
- `FlowLevel` (0): Error specific to a flow
- `DomainLevel` (1): Error affecting a domain or connection
- `SystemLevel` (2): Error affecting the entire system

#### ErrorInfo Struct
```go
type ErrorInfo struct {
    Code              ErrorCode
    Level             ErrorLevel
    Description       string
    RecommendedAction string
    Retryable         bool
}
```

#### Registry Operations
- `Lookup(code ErrorCode) (*ErrorInfo, error)`: Retrieve error metadata
- `Register(code, info)`: Add custom error codes (extensions only)
- Thread-safe with sync.RWMutex

#### ProtocolError
Implements Go's error interface with additional methods:
- `Code() ErrorCode`: Get the error code
- `Level() ErrorLevel`: Get severity level
- `IsRetryable() bool`: Check if retryable
- `Unwrap() error`: Get underlying cause
- `Error() string`: Full formatted message

### All 13 Core Error Codes

| Code | Value | Level | Retryable | Purpose |
|------|-------|-------|-----------|---------|
| UNHD_NO_ERROR | 0x0000 | FLOW | No | Graceful closure |
| UNHD_PROTOCOL_ERROR | 0x0001 | SYSTEM | Yes | Generic protocol error |
| UNHD_INVALID_FRAME | 0x0002 | DOMAIN | No | Invalid frame format |
| UNHD_FLOW_CONTROL_ERROR | 0x0003 | DOMAIN | Yes | Flow control violation |
| UNHD_SETTINGS_TIMEOUT | 0x0004 | SYSTEM | Yes | Settings handshake timeout |
| UNHD_STREAM_CLOSED | 0x0005 | FLOW | Yes | Stream already closed |
| UNHD_FRAME_SIZE_ERROR | 0x0006 | DOMAIN | Yes | Frame exceeds limits |
| UNHD_REFUSED_STREAM | 0x0007 | FLOW | Yes | Stream refused |
| UNHD_CANCEL | 0x0008 | FLOW | Yes | Stream cancelled |
| UNHD_COMPRESSION_ERROR | 0x0009 | DOMAIN | No | Compression failed |
| UNHD_CONNECT_ERROR | 0x000a | SYSTEM | Yes | Connection failed |
| UNHD_ENHANCE_YOUR_CALM | 0x000b | SYSTEM | Yes | Rate limiting |
| UNHD_INTERNAL_ERROR | 0x000c | SYSTEM | Yes | Implementation error |

### Usage Examples

```go
// Create protocol error
err := errors.NewProtocolError(errors.UNHD_FLOW_CONTROL_ERROR, "window exceeded")

// Get error details
if pe, ok := err.(*errors.ProtocolError); ok {
    if pe.IsRetryable() {
        // Retry logic
    }
}

// Register extension code
customInfo := &errors.ErrorInfo{
    Code: 0x0050,
    Level: errors.DomainLevel,
    Description: "Custom extension error",
    RecommendedAction: "Handle gracefully",
    Retryable: true,
}
errors.Register(0x0050, customInfo)
```

---

## Package 2: tlv (TLV Extension Mechanism)

**Location**: `pkg/protocol/tlv/`
**Files**: `tlv.go`, `tlv_test.go`
**Lines**: 221 + 443 = 664 lines

### Core Components

#### TLVType (uint8)
Three type ranges with specific purposes:

| Range | Name | Values | Purpose |
|-------|------|--------|---------|
| Core | Core | 0x00-0x0F | Standard protocol types |
| Negotiated | Extension | 0x10-0x7F | Version negotiation/extension |
| Padding | Padding | 0x80-0xFF | Padding/greasing |

#### 6 Core TLV Types

| Name | Value | Purpose |
|------|-------|---------|
| PRIORITY_HINT | 0x01 | Priority information |
| TRACE_SPAN_ID | 0x02 | Distributed tracing |
| ERROR_CODE | 0x03 | Detailed error information |
| FLOW_SEQUENCE | 0x04 | Flow-level sequence numbering |
| RING_PATH_COUNT | 0x05 | Ring network path information |
| HMAC_TAG | 0x06 | Message integrity check |

#### TLV Structure
```go
type TLV struct {
    Type   TLVType
    Length uint8      // Max 255 bytes
    Value  []byte
}

type TLVBlock []TLV   // Max 4 TLVs per Monad
```

#### Type Classification Methods
- `IsCore() bool`: Check if type is 0x00-0x0F
- `IsNegotiated() bool`: Check if type is 0x10-0x7F
- `IsPadding() bool`: Check if type is 0x80-0xFF
- `IsGreasing() bool`: Check if type matches pattern 0x1F*N+0x21

#### Parsing and Serialization
```go
// Parse byte stream into TLVBlock
block, err := Parse(data []byte)

// Serialize TLVBlock to bytes
data, err := Serialize(block TLVBlock)

// Round-trip validation with full bounds checking
```

**Parsing Validation**:
- Truncated header detection (need 2 bytes minimum)
- Truncated value detection (check actual data available)
- MaxTLVsPerMonad enforcement (4 TLVs limit)
- Zero-length values supported

**Serialization Validation**:
- TLV count limit enforcement
- Length field consistency check
- Value size limits (max 255 bytes)

#### TLVBlock Operations
- `Find(tlvType) *TLV`: Get first TLV of type
- `FindAll(tlvType) []TLV`: Get all TLVs of type
- `ValidateBlock(block) error`: Comprehensive validation

### Usage Examples

```go
// Create TLVs
tlv1, _ := tlv.NewTLV(tlv.PRIORITY_HINT, []byte{0x05})
tlv2, _ := tlv.NewCoreTLV(tlv.TRACE_SPAN_ID, traceID[:])

block := tlv.TLVBlock{*tlv1, *tlv2}

// Serialize to wire format
data, _ := tlv.Serialize(block)

// Parse from wire format
parsed, _ := tlv.Parse(data)

// Find TLVs
spanIDTlv := parsed.Find(tlv.TRACE_SPAN_ID)
if spanIDTlv != nil {
    traceID := spanIDTlv.Value
}

// Validate before use
if err := tlv.ValidateBlock(block); err != nil {
    // Handle invalid block
}
```

---

## Package 3: integrity (HMAC Integrity)

**Location**: `pkg/protocol/integrity/`
**Files**: `hmac.go`, `hmac_test.go`
**Lines**: 118 + 445 = 563 lines

### Core Components

#### HMAC-SHA256 Tag
- Uses HMAC-SHA256 truncated to 8 bytes (64 bits)
- Replaces CRC-16 for integrity verification
- Constant-time comparison to prevent timing attacks

#### HMACComputer
```go
type HMACComputer struct {
    flowSecret [32]byte  // Per-flow secret (immutable)
}
```

#### Flow Secret Derivation
```go
func DeriveFlowSecret(
    nodeSecret []byte,    // System-wide node secret
    flowLabel uint32,     // 32-bit flow identifier
    srcIP net.IP,         // Source IP address
    dstIP net.IP,         // Destination IP address
) ([32]byte, error)
```

**Derivation Process** (HKDF-SHA256):
1. Input Key Material: `nodeSecret`
2. Context: `flowLabel || srcIP || dstIP`
3. Output: 32-byte per-flow secret

**Properties**:
- Deterministic: Same inputs always produce same secret
- Unique: Different flows have different secrets
- Bound to: Flow endpoints, not modifiable

#### HMAC Operations

```go
// Compute HMAC for 18-byte monad header
tag := computer.Compute(monadHeader [18]byte) // Returns [8]byte

// Verify HMAC with constant-time comparison
verified := computer.Verify(monadHeader, tag)   // Returns bool
```

#### Backward Compatibility: CRC-16-CCITT

```go
// Polynomial: x^16 + x^12 + x^5 + 1
crc := CRC16CCITT(data []byte) uint16
verified := VerifyCRC16CCITT(data, expectedCRC uint16) bool
```

**Note**: CRC functions provided for backward compatibility only. New implementations should use HMAC.

### Input Validation

All inputs are validated:
- `nodeSecret`: Non-nil, non-empty
- `srcIP`/`dstIP`: Non-nil, non-empty
- `monadHeader`: Always [18]byte (fixed size)
- `tag`: Always [8]byte (fixed size)

### Constant-Time Verification

The `Verify()` method uses `hmac.Equal()` which:
- Performs comparison in constant time
- Resists timing-based side-channel attacks
- Critical for security

### Usage Examples

```go
import "unheaded/pkg/protocol/integrity"

// Derive flow secret (once per flow)
secret, err := integrity.DeriveFlowSecret(
    nodeSecret,
    flowLabel,
    net.ParseIP("192.168.1.1"),
    net.ParseIP("192.168.1.2"),
)
if err != nil {
    // Handle derivation error
}

// Create computer (keep alive for flow duration)
computer := integrity.NewHMACComputer(secret)

// Compute tag for outgoing message
var header [18]byte
copy(header[:], monadBytes[:18])
tag := computer.Compute(header)

// Verify tag on incoming message
if computer.Verify(header, tag) {
    // Message authentic and not tampered
} else {
    // Message failed verification - discard
}
```

---

## Cross-Package Integration

### Error Codes in TLV
The `ERROR_CODE` TLV type (0x03) carries error codes from the errors package:
```go
errorCodeTLV, _ := tlv.NewCoreTLV(
    tlv.ERROR_CODE,
    []byte{
        byte(code >> 8),
        byte(code),
    },
)
```

### HMAC in TLV
The `HMAC_TAG` TLV type (0x06) carries integrity tags:
```go
hmacTagTLV, _ := tlv.NewCoreTLV(
    tlv.HMAC_TAG,
    tag[:],  // [8]byte from integrity.Compute()
)
```

### Error Recovery
When integrity verification fails (HMAC mismatch), use error codes:
- UNHD_PROTOCOL_ERROR: For general verification failures
- UNHD_INTERNAL_ERROR: For verification implementation issues

---

## Testing Coverage

### errors package
- All 13 core codes registered and retrievable
- Error level classification
- Retryable flag classification
- ProtocolError creation/unwrapping
- Custom code registration validation
- Standards code override prevention

### tlv package
- Type boundary classification
- Valid parsing (single, multiple, empty, zero-length)
- Truncation error detection
- MaxTLVsPerMonad enforcement
- Serialization validation
- Round-trip parse/serialize
- TLV finding and filtering
- Block validation

### integrity package
- Flow secret derivation determinism
- Flow secret uniqueness per parameters
- HMAC computation determinism
- HMAC computation changes with different secrets
- Tag verification (valid/invalid)
- Constant-time verification properties
- CRC-16-CCITT known vectors
- Benchmarks for performance

---

## Defensive Coding Patterns Applied

✓ **All inputs validated** with explicit error returns
✓ **All errors wrapped** with context using fmt.Errorf
✓ **Bounds checking** in parse operations (truncation detection)
✓ **Length validation** in TLV operations (field consistency)
✓ **Constant-time comparison** for HMAC verification
✓ **Thread-safe registry** with sync.RWMutex
✓ **Max limits enforced** (4 TLVs per Monad, 255 bytes per value)
✓ **Nil pointer checks** throughout
✓ **Type safety** with uint8, uint16, uint32 where appropriate
✓ **Struct immutability** patterns (flow secret in [32]byte array)
✓ **No panics** except in MustSerialize (documented)

---

## Building and Testing

All packages follow Go conventions and are ready for testing:

```bash
# Test errors package
go test ./pkg/protocol/errors -v

# Test tlv package
go test ./pkg/protocol/tlv -v

# Test integrity package
go test ./pkg/protocol/integrity -v

# Test all protocol packages
go test ./pkg/protocol/... -v
```

Each package includes comprehensive test cases and benchmarks.
