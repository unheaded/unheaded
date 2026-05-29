# Unheaded Protocol Packages

Complete implementation of five core protocol packages for the Unheaded protocol.

## Packages

### 1. sequence - Namespace Sequence Counters (Q2)
**Path**: `./sequence/`
**Purpose**: Monotonic sequence counter tracking per namespace

**Key Types**:
- `NamespaceCounter`: Per-namespace uint32 counter
- `SequenceTracker`: Multi-namespace aggregator

**Key Functions**:
- `Increment() uint32` - Increment counter
- `Current() uint32` - Get current value
- `DetectGap(expected, received) (lost, reordered)` - Detect packet loss/reordering
- `ValidateMonotonicity(namespace, received)` - Validate sequence order

**Thread Safety**: Yes (sync.RWMutex)
**Tests**: 11 test cases including concurrent safety

---

### 2. amplification - Ring Path Counter (Q4)
**Path**: `./amplification/`
**Purpose**: Amplification attack protection via ring path counting

**Key Types**:
- `RingPathCounter` (uint16): 2-byte Monad field
- `AmplificationTracker`: Packet sent/received tracking

**Constants**:
- `MaxAmplificationRatio = 3` (QUIC-compatible)
- `DefaultMaxHopCount = 16`

**Key Functions**:
- `Increment() (RingPathCounter, error)` - Error if exceeds max
- `ShouldDrop(ringCount, threshold) bool` - Drop decision
- `ValidateAmplification(sent, received) bool` - Ratio validation

**Thread Safety**: Yes (internal state protected)
**Tests**: 13 test cases including ratio enforcement

---

### 3. migration - Flow Migration & Shield Retry (Q5, Q6)
**Path**: `./migration/`
**Purpose**: Flow migration tokens and address validation

**Key Types**:
- `FlowMigrationToken`: Migration token with paths
- `ShieldRetryToken`: HMAC-SHA256 based validation
- `AddressValidator`: ICMPv6 challenge-response validator

**Key Functions**:
- `GenerateToken(flowLabel, seq) []byte` - Generate migration token
- `ValidateToken(token, secret) bool` - Validate token
- `GenerateRetryToken(shieldSecret, srcIP) []byte` - 16-byte token
- `ValidateRetryToken(token, secret, srcIP, maxAge) bool` - Time-limited validation
- `ChallengeAddress(srcIPStr, srcIP) []byte` - Issue challenge
- `ValidateAddress(srcIPStr, srcIP, token) bool` - Validate response

**Cryptography**: HMAC-SHA256
**Thread Safety**: Yes (AddressValidator uses maps with care)
**Tests**: 15 test cases including expiry and HMAC validation

---

### 4. prefetch - Explicit Prefetch Hints (H9)
**Path**: `./prefetch/`
**Purpose**: Prefetch hint management with limits

**Key Types**:
- `PrefetchHint`: Hint with flow, address, page count
- `CancelPrefetch`: Cancellation request
- `PrefetchManager`: Lifecycle and limit enforcement

**Constants**:
- `MaxOutstandingPrefetch = 16` (SETTINGS-configurable)

**Key Functions**:
- `AddHint(flowLabel, baseAddr, pageCount) (hintID, error)` - Add hint
- `GetHint(hintID) (*PrefetchHint, bool)` - Retrieve hint
- `CancelHint(hintID) error` - Cancel single hint
- `CancelFlowHints(flowLabel) error` - Cancel all hints for flow
- `GetFlowHints(flowLabel) []*PrefetchHint` - Get flow hints
- `OutstandingCount() uint16` - Get active hint count

**Limits**:
- Global: 16 hints (configurable)
- Per-flow: 8 hints (configurable)

**Thread Safety**: Yes (sync.RWMutex)
**Tests**: 16 test cases including limit enforcement

---

### 5. intermediary - Hop Validation (H10, H11)
**Path**: `./intermediary/`
**Purpose**: Packet validation and malformation detection

**Key Types**:
- `MalformedReason`: Malformation type (6 types)
- `IntermediaryPolicy`: Validation policy configuration
- `FieldValidator`: Known ID tracking
- `AuthorityValidator`: Authority verification
- `PacketValidator`: Comprehensive validator

**Malformation Types**:
- `InvalidVersion`
- `InvalidCRC`
- `UnknownDictID`
- `InvalidFieldID`
- `DictAuthViolation`
- `NonZeroReserved`

**Key Functions**:
- `ValidateMonad(data) ([]MalformedReason, error)` - Full validation
- `CrossProtocolCheck(data) bool` - Reserved bits == 0
- `ValidateAndProcess(data) (bool, []MalformedReason, error)` - Policy-driven validation
- `RegisterDictID/FieldID/Authority()` - Registration methods
- `ValidateDictID/FieldID/Authority()` - Validation methods

**Thread Safety**: Yes (IntermediaryPolicy and validators use maps)
**Tests**: 21 test cases covering all malformations and boundaries

---

## Building

```bash
# Build all packages
go build ./...

# Test all packages
go test ./...

# Test with verbose output
go test -v ./...

# Build specific package
go build ./sequence
```

## Dependencies

All packages use only Go standard library. No external dependencies.

## Testing

Comprehensive test coverage:
- 80+ test cases total
- Unit tests for all public APIs
- Edge case and boundary testing
- Concurrent access validation
- Integration scenarios
- Error condition handling

Run tests:
```bash
go test -v ./...
```

## Code Quality

- Thread-safe design (sync.RWMutex where needed)
- Comprehensive input validation
- Clear error handling
- No panics on invalid input
- Proper documentation
- Consistent Go conventions

## Version

- Go 1.25.0
- Module: `unheaded`

## Related Files

- `SCAFFOLDING_SUMMARY.md` - Detailed package documentation
- `../CREATION_REPORT.md` - Complete creation report
