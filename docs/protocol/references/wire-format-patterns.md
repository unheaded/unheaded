# Wire Format Patterns Reference

A comprehensive reference for wire format documentation patterns extracted from the RFC corpus, with specific focus on patterns relevant to the Unheaded protocol and Monad format.

---

## 1. Diagram Conventions

### 32-Bit Row Format

All wire format diagrams in RFCs follow a standard 32-bit row format:

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

### Bit Numbering

- Bits are numbered 0-31 within each 32-bit (4-octet) row
- Bit 0 is the most significant bit (leftmost)
- Bit 31 is the least significant bit (rightmost)
- This creates big-endian presentation by default

### Field Boundaries

- `+-+-+-+` marks are used at field boundaries
- Each `+` aligns with a nibble (4-bit) boundary in the diagram
- A field spanning multiple rows should have aligned boundaries

### Big-Endian Default

- All wire formats use network byte order (big-endian)
- Multi-byte values are transmitted most significant byte first
- This is sometimes called "network byte order" in protocol specifications

### Variable-Length Fields

For variable-length fields:

1. Show the field boundary extending beyond a single row with vertical bars `|`
2. Indicate length in the description with "(variable)" or "(variable, N octets max)"
3. Include a separate row or table indicating how length is determined
4. Example:

```
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|              Header (variable length, see below)               |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

---

## 2. The Monad Wire Format

### Complete 20-Byte Layout

Monad is a 20-octet (160-bit) extension header designed for the Unheaded protocol, commonly carried as an IPv6 Hop-by-Hop option.

#### Hex Offset Table

```
Offset  Name                    Size    Description
------  ----                    ----    -----------
0x00    version                 1 byte  Protocol version (current: 0x01)
0x01    src_service_id          1 byte  Exponent-encoded source service
0x02    dst_service_id          1 byte  Exponent-encoded destination service
0x03    hop_count               1 byte  Hop counter, decremented each hop (TTL-like)
0x04    qos_class               1 byte  Exponent-encoded QoS classification
0x05    flow_action             1 byte  Exponent-encoded action directive
0x06    circuit_state           1 byte  Exponent-encoded circuit breaker state
0x07    flags                   1 byte  Bitfield (C|Y|T|E|S|M|CUSTOM|RSVD)
0x08-09 latency_hint            2 bytes Latency budget in microseconds
0x0A    deployment_ring         1 byte  Exponent-encoded deployment ring
0x0B    mesh_flags              1 byte  Exponent-encoded mesh-level flags
0x0C    src_prefix_lo           1 byte  Source prefix low byte
0x0D    dst_prefix_lo           1 byte  Destination prefix low byte
0x0E-0F scratch[0-1]            2 bytes Scratch space
0x10-11 scratch[2-3]            2 bytes Scratch space
0x12-13 checksum                2 bytes CRC-16/CCITT over bytes 0x00-0x11
```

#### ASCII Diagram (20 Octets)

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|    version    |src_service_id |dst_service_id |   hop_count   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   qos_class   |  flow_action  | circuit_state |     flags     |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|         latency_hint          |  deploy_ring  |  mesh_flags   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
| src_prefix_lo | dst_prefix_lo |  scratch[0]   |  scratch[1]   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  scratch[2]   |  scratch[3]   |           checksum            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

#### Field Type Summary

- **Fixed-width integers**: version, hop_count, src_prefix_lo, dst_prefix_lo, scratch
- **Exponent-encoded values**: src_service_id, dst_service_id, qos_class, flow_action, circuit_state, deployment_ring, mesh_flags
- **Bitfield**: flags (C|Y|T|E|S|M|CUSTOM|RSVD)
- **Budget**: latency_hint (16-bit unsigned, microseconds)
- **Checksum**: CRC-16/CCITT-FALSE over first 18 octets

---

## 3. IPv6 Extension Header Format

### Monad in IPv6 Hop-by-Hop Header (RFC 8200)

Monad is encapsulated as an IPv6 Hop-by-Hop Option Type within the IPv6 HbH extension header.

#### IPv6 HbH Extension Header Container

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Next Header  |  Hdr Ext Len  |                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
.                           Options                             .
.                                                               .
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

#### Option Type Encoding

An option type is an 8-bit value encoding three pieces of information:

```
 0 1 2 3 4 5 6 7
+-+-+-+-+-+-+-+-+
| act |chg| opt |
+-+-+-+-+-+-+-+-+
```

- **act (bits 0-1)**: Action to take if option is not recognized
  - `00`: Skip this option and continue processing
  - `01`: Discard the packet
  - `10`: Discard the packet and send ICMP Parameter Problem
  - `11`: Discard the packet (if multicast) / send ICMP (if unicast)

- **chg (bit 2)**: Whether option data may change in transit
  - `0`: Option data does not change
  - `1`: Option data may change

- **opt (bits 3-7)**: Option number (31 possible values per act/chg combination)

#### Monad as Option Type

For Monad, if registered with:
- **act = 00** (skip if unrecognized)
- **chg = 1** (may change in transit due to hop_count decrement)
- **opt = 15** (example)

The full option type byte would be: `00 1 01111` = `0x1F` (31)

#### Hdr Ext Len Calculation

`Hdr Ext Len = (total header length in octets - 8) / 8`

For a Hop-by-Hop header containing a single Monad option:
- Monad itself: 20 octets
- Type field: 1 octet
- Length field: 1 octet
- Total: 22 octets
- Padding to 8-octet alignment: 2 octets
- **Total extension header**: 24 octets = 3 × 8 octets
- **Hdr Ext Len = (24 - 8) / 8 = 2**

---

## 4. Encoding Patterns from the RFC Corpus

### 4.1 Fixed-Width Fields

Fixed-width fields represent values in a constant-size container.

#### Pattern

```
|              value_name (N octets, big-endian)                |
```

#### Examples from Monad

**Version field (1 octet)**
- Range: 0-255
- Current Monad version: 1
- Specification: "The version field is set to 1 for this version of the Monad protocol."

**Hop Count (1 octet)**
- Range: 0-255 hops
- Decremented at each hop
- Specification: "The hop_count field is decremented by 1 at each hop. If hop_count reaches 0 before the packet is delivered, it MUST be discarded."

#### Advantages

- Simple to parse
- Constant storage overhead
- Works for all values in the range

#### Disadvantages

- Uses fixed space even for small values
- Not suitable for sparse distributions

---

### 4.2 Exponent Encoding

Exponent encoding compacts metadata by representing values as `base^exponent × multiplier`,
fitting rich semantic values into a single byte per field.

#### Formula and Rationale

In Monad's exponent encoding:
- A value `v` is encoded as a **single octet**: exponent `e` (signed -128 to +127)
- **Actual value = base^exponent × multiplier**
- Base and multiplier are defined per-field in the Sophia dictionary
- Default base is 2; default multiplier is 1

This allows representing values from 1 to base^127 in a single byte.

#### Encoding Format

Exponent-encoded fields are exactly **1 octet (8 bits)**, interpreted as a signed
two's complement integer (-128 to +127).

```
 0 1 2 3 4 5 6 7
+-+-+-+-+-+-+-+-+
|   exponent    |   (signed 8-bit, two's complement)
+-+-+-+-+-+-+-+-+
```

Unlike variable-length schemes, exponent encoding is always fixed at 1 byte
per field, making packet parsing straightforward. The semantic richness comes
from Sophia dictionary lookup, not from the wire format encoding itself.

#### Go Implementation Example

```go
// Decode an exponent-encoded single-byte value
func DecodeExponent(encoded int8, base uint64, multiplier uint64) uint64 {
    if encoded == 0 {
        return multiplier // base^0 * multiplier = multiplier
    }
    if encoded < 0 {
        // Negative exponents produce fractional results;
        // for integer fields, this rounds toward zero
        return 0
    }
    result := uint64(1)
    for i := int8(0); i < encoded; i++ {
        result *= base
    }
    return result * multiplier
}

// Encode a value to single-byte exponent (inverse lookup)
func EncodeExponent(value uint64, base uint64, multiplier uint64) int8 {
    target := value / multiplier
    exp := int8(0)
    current := uint64(1)
    for current < target && exp < 127 {
        current *= base
        exp++
    }
    if current != target {
        return -1 // Value not exactly representable
    }
    return exp
}
```

#### Rust Implementation Example

```rust
/// Decode an exponent-encoded single-byte value
pub fn decode_exponent(encoded: i8, base: u64, multiplier: u64) -> u64 {
    if encoded <= 0 {
        return if encoded == 0 { multiplier } else { 0 };
    }
    let result = base.checked_pow(encoded as u32).unwrap_or(u64::MAX);
    result.saturating_mul(multiplier)
}

/// Encode a value to single-byte exponent
pub fn encode_exponent(value: u64, base: u64, multiplier: u64) -> Option<i8> {
    let target = value / multiplier;
    let mut exp: i8 = 0;
    let mut current: u64 = 1;
    while current < target && exp < 127 {
        current = current.checked_mul(base)?;
        exp += 1;
    }
    if current == target { Some(exp) } else { None }
}
```

#### Test Vectors

For src_service_id (offset 0x01) with Sophia defaults (base=2, multiplier=1):

| Encoded Byte | Exponent | base | multiplier | Decoded Value | Service Name |
|---|---|---|---|---|---|
| 0x00 | 0 | 2 | 1 | 1 | (default) |
| 0x01 | 1 | 2 | 1 | 2 | timeguru |
| 0x02 | 2 | 2 | 1 | 4 | developer |
| 0x03 | 3 | 2 | 1 | 8 | architect |
| 0x04 | 4 | 2 | 1 | 16 | captain |
| 0x07 | 7 | 2 | 1 | 128 | (reserved) |

For qos_class (offset 0x04) with Sophia (base=10, multiplier=1):

| Encoded Byte | Exponent | base | multiplier | Decoded Value | QoS Class |
|---|---|---|---|---|---|
| 0x00 | 0 | 10 | 1 | 1 | default |
| 0x01 | 1 | 10 | 1 | 10 | bulk |
| 0x02 | 2 | 10 | 1 | 100 | interactive |
| 0x03 | 3 | 10 | 1 | 1000 | realtime |

Refer to [UNHEADED-FOUNDATION] Section 6 for complete Sophia dictionary
specification and base/multiplier parameters.

---

### 4.3 Variable-Length Integers

Variable-length integers (varints) minimize space for small values while supporting larger ones. The pattern is from QUIC (RFC 9000).

#### QUIC Varint Format

```
MSB (2 bits) | Meaning | Encoding Space | Decoded Value Range
---|---|---|---
00 | 1-octet | 1 byte | 0-63
01 | 2-octet | 2 bytes | 0-16383
10 | 4-octet | 4 bytes | 0-1073741823
11 | 8-octet | 8 bytes | 0-(2^62-1)
```

#### Encoding Example

```
Value: 25  →  Encoded: 0x19  (00011001)
                Top 2 bits: 00 (1-byte encoding)
                Remaining 6 bits: 011001 = 25

Value: 500 →  Encoded: 0x401F (0100 0000 0001 1111)
                Top 2 bits: 01 (2-byte encoding)
                Remaining 14 bits: 01 1111 0001 11 = 500
```

#### When to Use

- When values are frequently small (< 64)
- When storage is critical and distribution is skewed
- For frame types, stream IDs, or offset fields
- NOT recommended for fixed fields in critical paths

---

### 4.4 Type-Length-Value (TLV)

Type-Length-Value encoding is used for extensible, optional, or variable-length fields. Common in BGP attributes (RFC 4271) and IPv6 options (RFC 8200).

#### Format

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|      Type     |     Length    |          Value (variable)     |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+                               |
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

#### BGP Attribute Encoding (RFC 4271)

```
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|Flags|Type Code|  Length (1 or 2 octets based on flags)       |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|              Value (variable length)                          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

#### IPv6 Option TLV (RFC 8200)

```
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|      Type     |  Data Len     |  Data (variable)
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

#### Advantages

- Supports optional fields
- Allows protocol extension without breaking compatibility
- Can handle variable-length values efficiently

#### Implementation Considerations

- Always validate length field before consuming data
- Handle unknown types gracefully (skip vs. error based on type flags)
- Ensure length is consistent with actual data

---

### 4.5 Bitfield Flags

Bitfields pack multiple boolean or small-value flags into a single octet or multi-byte field.

#### Monad Flags Field (1 octet)

```
 0 1 2 3 4 5 6 7
+-+-+-+-+-+-+-+-+
|C|Y|T|E|S|M|CU|R |
+-+-+-+-+-+-+-+-+
```

- **C (bit 7)**: Chaos — Yaldabaoth chaos injection active
- **Y (bit 6)**: Canary — Canary deployment path
- **T (bit 5)**: Trace — Full trace to Anamnesis
- **E (bit 4)**: Encrypted — Payload encrypted
- **S (bit 3)**: Sampled — Statistical sampling active
- **M (bit 2)**: Mirror — Mirror copy of packet
- **CUSTOM (bit 1)**: Extended encoding mode — scratch and extended registers carry exponent-encoded values per Sophia dictionary
- **RSVD (bit 0)**: Reserved — MUST be zero; reserved for future protocol versions

#### Go Implementation Example

```go
type MonadFlags uint8

const (
    FlagChaos    MonadFlags = 1 << 7  // C: Yaldabaoth chaos injection
    FlagCanary   MonadFlags = 1 << 6  // Y: Canary deployment path
    FlagTrace    MonadFlags = 1 << 5  // T: Full trace to Anamnesis
    FlagEncrypt  MonadFlags = 1 << 4  // E: Payload encrypted
    FlagSampled  MonadFlags = 1 << 3  // S: Statistical sampling
    FlagMirror   MonadFlags = 1 << 2  // M: Mirror copy
    FlagCustom   MonadFlags = 1 << 1  // CUSTOM: Extended encoding mode
    FlagReserved MonadFlags = 1 << 0  // RSVD: Reserved, MUST be zero
)

// Check if flag is set
func (f MonadFlags) Has(flag MonadFlags) bool {
    return (f & flag) != 0
}

// Set flag
func (f MonadFlags) Set(flag MonadFlags) MonadFlags {
    return f | flag
}

// Clear flag
func (f MonadFlags) Clear(flag MonadFlags) MonadFlags {
    return f &^ flag
}
```

#### Rust Implementation Example

```rust
bitflags::bitflags! {
    pub struct MonadFlags: u8 {
        const CHAOS    = 1 << 7;
        const CANARY   = 1 << 6;
        const TRACE    = 1 << 5;
        const ENCRYPT  = 1 << 4;
        const SAMPLED  = 1 << 3;
        const MIRROR   = 1 << 2;
        const CUSTOM   = 1 << 1;
        const RESERVED = 1 << 0;
    }
}
```

#### Documentation Pattern

For each flag in the bitfield:

```markdown
- **Flag Name (bit N)**: Concise description.
  - If set (1): meaning
  - If clear (0): meaning
```

---

### 4.6 Label Stacks (MPLS, RFC 3031)

Label stacks encode a sequence of variable-length labels, common in MPLS and used for routing decision chains.

#### MPLS Label Stack Entry (LSE)

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                 Label (20 bits)        |Exp|S|     TTL     |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

- **Label (20 bits)**: Forwarding label value (0-1048575)
- **Exp (3 bits)**: Experimental (QoS) field
- **S (1 bit)**: Stack bit (1 = bottom of stack, 0 = more labels follow)
- **TTL (8 bits)**: Time to live

#### Variable-Length Stack

```
Multiple LSEs, each 4 octets, in order:
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                      Label 1 LSE                              |  S=0
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                      Label 2 LSE                              |  S=0
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                      Label N LSE                              |  S=1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

#### Parsing Algorithm

1. Read one LSE (4 octets)
2. Extract S bit
3. If S=0, another LSE follows; go to step 1
4. If S=1, stack complete

---

## 5. Checksum Specifications

### 5.1 CRC-16/CCITT-FALSE (Monad)

Monad uses CRC-16/CCITT-FALSE for 16-bit header integrity checking over the first 18 octets.

#### Specification Parameters

```
Algorithm:     CRC-16/CCITT-FALSE (also called CRC-XMODEM, CRC-ZMODEM)
Polynomial:    0x1021 (x^16 + x^12 + x^5 + 1)
Initial Value: 0xFFFF
Reflect Input: No
Reflect Output: No
Final XOR:     0x0000
Scope:         First 18 octets of Monad header (excluding checksum field)
Test Vector:   See below
```

#### CRC-16/CCITT-FALSE Test Vector

```
Input:  0x01 0x01 0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00
        0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00
        (version=1, hop_count=1, rest zeros)

Expected CRC-16 output: 0x4416
```

#### Go Implementation

```go
import "hash/crc32"

func ComputeMonadCRC16(data []byte) uint16 {
    if len(data) < 18 {
        return 0
    }

    const polynomial = 0x1021
    crc := uint16(0xFFFF)

    for _, b := range data[:18] {
        crc ^= uint16(b) << 8
        for i := 0; i < 8; i++ {
            crc <<= 1
            if crc&0x10000 != 0 {
                crc ^= polynomial
                crc &= 0xFFFF
            }
        }
    }

    return crc
}
```

#### Rust Implementation

```rust
pub fn compute_monad_crc16(data: &[u8]) -> u16 {
    const POLYNOMIAL: u16 = 0x1021;
    let mut crc: u16 = 0xFFFF;

    for &byte in &data[..18.min(data.len())] {
        crc ^= (byte as u16) << 8;
        for _ in 0..8 {
            crc <<= 1;
            if crc & 0x10000 != 0 {
                crc ^= POLYNOMIAL;
                crc &= 0xFFFF;
            }
        }
    }

    crc
}
```

---

### 5.2 CRC-32/MPEG-2 (Monad Alternative)

Some implementations of Monad use CRC-32/MPEG-2 for stronger integrity checking.

#### Specification Parameters

```
Algorithm:     CRC-32/MPEG-2
Polynomial:    0x04C11DB7 (standard MPEG-2 polynomial)
Initial Value: 0xFFFFFFFF
Reflect Input: No
Reflect Output: No
Final XOR:     0x00000000
Scope:         First 16 octets of Monad header (excluding 4-octet checksum field)
Test Vector:   See below
```

#### CRC-32/MPEG-2 Test Vector

```
Input:  0x01 0x01 0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00
        0x00 0x00 0x00 0x00 0x00 0x00

Expected CRC-32 output: 0x8E8A6DD1
```

#### Go Implementation

```go
import "hash/crc32"

var crc32Table *crc32.Table

func init() {
    crc32Table = crc32.MakeTable(0x04C11DB7)
}

func ComputeMonadCRC32(data []byte) uint32 {
    if len(data) < 16 {
        return 0
    }
    return crc32.Checksum(data[:16], crc32Table)
}
```

---

### 5.3 Internet Checksum (RFC 1071)

The Internet Checksum is used in IP, UDP, TCP, and ICMP. It is a 1's complement sum.

#### Algorithm

1. Treat the data as a sequence of 16-bit unsigned integers in network byte order
2. Sum all 16-bit values
3. Add any carry bits from the sum back to the lower 16 bits
4. Take the 1's complement (bitwise NOT) of the result

#### Formula

```
Checksum = ~(sum of all 16-bit words, with carries added back)
```

#### Test Vector

```
Data: 0x4500 0x0073 0x0000 0x4000 0x4011 0xC0A8 0x0001 0xC0A8 0x0002
         (IPv4 header example)

Sum (before 1's complement): 0x1FFFF
After carry addition: 0x00020
1's complement: 0xFFDF

Expected Checksum: 0xFFDF
```

#### Go Implementation

```go
func ComputeInternetChecksum(data []byte) uint16 {
    sum := uint32(0)

    // Process 16-bit words
    for i := 0; i < len(data)-1; i += 2 {
        sum += uint32(data[i])<<8 | uint32(data[i+1])
    }

    // Handle odd byte
    if len(data)%2 == 1 {
        sum += uint32(data[len(data)-1]) << 8
    }

    // Add carry
    for sum>>16 > 0 {
        sum = (sum & 0xFFFF) + (sum >> 16)
    }

    return ^uint16(sum)
}
```

#### Rust Implementation

```rust
pub fn compute_internet_checksum(data: &[u8]) -> u16 {
    let mut sum: u32 = 0;

    // Process 16-bit words
    let mut i = 0;
    while i < data.len() - 1 {
        sum += ((data[i] as u32) << 8) | (data[i+1] as u32);
        i += 2;
    }

    // Handle odd byte
    if data.len() % 2 == 1 {
        sum += (data[data.len()-1] as u32) << 8;
    }

    // Add carry
    while sum >> 16 > 0 {
        sum = (sum & 0xFFFF) + (sum >> 16);
    }

    !(sum as u16)
}
```

#### Incremental Update

To update a checksum when only a few fields change:

```
new_checksum = ~(~old_checksum + ~old_value + new_value)
            = ~(~old_checksum - old_value + new_value)
```

---

### 5.4 How to Specify a Checksum in an RFC

When documenting a checksum in an RFC, include all of the following:

1. **Algorithm name**: e.g., "CRC-32/MPEG-2", "Internet Checksum", "CRC-16/XMODEM"

2. **Polynomial** (if CRC-based):
   - Express as hex: `0x1021` or polynomial notation: x^16 + x^12 + x^5 + 1

3. **Initial value (seed)**:
   - Standard initial values are `0x0000` or `0xFFFFFFFF`
   - Specify as hex: `0xFFFF`

4. **Reflect input**:
   - "Yes" or "No" — whether input bytes are bit-reversed
   - Default assumption: "No"

5. **Reflect output**:
   - "Yes" or "No" — whether output is bit-reversed
   - Default assumption: "No"

6. **Final XOR**:
   - Value XORed with result after all processing
   - Common values: `0x0000`, `0xFFFFFFFF`

7. **Scope (data over which checksum is computed)**:
   - Specify exactly which octets are included
   - Example: "octets 0-15 of the header" or "all octets except the checksum field"

8. **Test vector**:
   - Provide hex input and expected output
   - Include a non-trivial example (all zeros should NOT be the vector)
   - Specify the form (raw hex, ASCII, etc.)

#### Template

```markdown
## Checksum Computation

The [HEADER] includes a [N]-bit checksum computed as follows:

- **Algorithm**: [CRC-32/MPEG-2 | Internet Checksum | CRC-16/CCITT-FALSE]
- **Polynomial**: [0xXXXXXXXX | N/A for Internet Checksum]
- **Initial Value**: [0xXXXXXXXX]
- **Reflect Input**: [Yes | No]
- **Reflect Output**: [Yes | No]
- **Final XOR**: [0xXXXXXXXX]
- **Scope**: The checksum is computed over octets [X-Y] of the header.

### Test Vector

```
Input:  [hex bytes]

Expected Checksum: [hex result]
```

The sender MUST compute the checksum and place it in the checksum field.
The receiver MUST verify the checksum and discard the packet if verification fails,
unless a configuration option explicitly allows checksum bypass.
```

---

## 6. Cross-Protocol Design Patterns

### 6.1 8-Octet Alignment

Many protocols require that certain structures begin and end at 8-octet (64-bit) boundaries for efficient memory access and DMA operations.

#### Rationale

- **IPv6 extension headers** (RFC 8200) must be 8-octet aligned
- **Berkeley Packet Filter (BPF)** instructions are 8 octets
- **MPLS labels** (RFC 3031) are 4 octets but often grouped in 8-octet sets
- **Efficient memory access**: ARM, x86, and other ISAs optimize for 8-byte loads

#### Padding Rules

If a variable-length field does not end on an 8-octet boundary, add padding:

```
Total Length = (Length + 7) & ~7  // Round up to nearest multiple of 8
Padding Octets = Total Length - Length
```

#### Example: IPv6 HbH with Monad

Monad (20 octets) + option type/length overhead (2 octets) = 22 octets
Round up to 24 octets, add 2 octets of padding (all zeros)

#### How to Specify in RFC

```markdown
If the [STRUCTURE] is not naturally aligned on an 8-octet boundary,
the sender MUST add padding octets (set to zero) to reach the next
8-octet boundary. The receiver MUST skip over padding octets when
parsing subsequent headers.
```

---

### 6.2 Reserved Fields

Reserved fields are placeholders for future extension without breaking backward compatibility.

#### Standard Language

```markdown
- **Reserved (N octets)**: This field is reserved for future use.
  MUST be set to zero on transmit. MUST be ignored on receipt.
  An implementation that encounters a non-zero reserved field value
  SHOULD log a warning but continue processing.
```

#### Handling Non-Zero Reserved Fields

- **Strict mode**: Discard packet if reserved field is non-zero
- **Lenient mode**: Warn and continue (recommended for robustness)

#### Documentation Example (Monad)

```markdown
- **Reserved (2 octets)**: This field is reserved for future protocol extensions.
  Senders MUST set this field to 0x0000. Receivers MUST ignore this field
  (i.e., the value does not affect packet processing). If a receiver detects
  a non-zero value and is in strict mode, it MAY discard the packet.
```

---

### 6.3 Version Field Handling

Version fields allow protocol evolution. Common approach: reject unknown versions.

#### Standard Pattern

```markdown
- **Version (1 octet)**: Indicates the version of this protocol specification.
  This document describes version 1. An implementation MUST:
  - Accept packets with version = 1
  - Discard (or log and discard) packets with version != 1

  Future versions MAY introduce incompatible changes. Version 0 is reserved
  and MUST NOT be used.
```

#### Alternative: Backward Compatibility

For optional extensions:

```markdown
If an implementation receives a packet with a version greater than the
maximum version it supports, it SHOULD discard the packet. An
implementation MAY support multiple versions if backward compatibility
is required by the deployment environment.
```

#### Version Negotiation

For protocols with phase negotiation:

```markdown
The version field indicates the version of the Monad protocol in use.
Implementations SHOULD advertise supported versions in a separate
capability exchange message. The version in the header MUST match a
version the receiver advertised support for, or the packet MUST be
discarded.
```

---

### 6.4 Next Header Chaining (IPv6 Model)

The IPv6 next header model (RFC 8200) allows flexible protocol stacking.

#### Pattern

Each header contains a "Next Header" field indicating the type of the next header:

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Next Header  |  Hdr Ext Len  |       [Header-specific]      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                                                               |
.                      [Extension Data]                         .
|                                                               |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           Next Header for [Next Extension or Payload]        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

#### Assigned Values (IANA IPv6 Next Header Registry)

```
Value   Name
-----   ----
0       IPv6 Hop-by-Hop Option
1       ICMP
4       IP (in IP)
6       TCP
17      UDP
41      IPv6
43      IPv6 Routing Header
44      IPv6 Fragment Header
50      ESP (IPsec)
51      AH (IPsec)
58      ICMPv6
59      No Next Header
60      IPv6 Destination Options
[Rest]  Refer to IANA registry
```

#### Processing Model

1. Parse the IPv6 header to get initial Next Header value
2. Loop:
   - Match Next Header value against known header types
   - If unknown, check if it's a hop-by-hop option (value < 64); if yes, skip and continue
   - If not skip-capable, send ICMP Parameter Problem and discard
   - Otherwise, parse the header and extract the next Next Header value
   - Continue to next iteration
3. When Next Header = 59 (No Next Header), stop processing
4. Otherwise, deliver payload to the protocol indicated by Next Header

#### Extension Header Template

```markdown
If this extension header is used as a Next Header value, the following table
shows the Next Header field mapping:

[Next Header Value]: [Description of this header type]

After this header (and its variable-length options, if any), the sender places
the type of the next header in the "Next Header" field of this header.
```

---

### 6.5 IANA Registry for Extensibility

IANA (Internet Assigned Numbers Authority) registries formalize extension points.

#### Why Registries

- Prevent collisions when multiple parties extend a protocol
- Create a shared naming space
- Enable coordination without central gatekeeping

#### Common Pattern: Option Type Registry

```markdown
## IANA Considerations

This document requests IANA to create a new registry:
"Monad Extension Option Types"

Allocation Policy: IETF Review (RFC 5226)

Value Range: 0-255

Initial Assignments:

Value   Name              Reference
-----   ----              ---------
0       Reserved          This document
1-119   Unassigned        (available for future use)
120-127 Experimental Use  This document
128-255 Reserved          This document

Each option type document MUST specify:
- Bit layout of the option data
- How the option affects Monad processing
- Whether the option may appear multiple times
- Whether the option must appear in a specific order relative to others
```

#### Checksum Algorithm Registry Example

```markdown
## Monad Checksum Algorithm Registry

Registry Name: Monad Checksum Algorithms
Allocation Policy: Expert Review
Expert: [Name], [Email]

Value   Name              Reference
-----   ----              ---------
0       None              This document
1       CRC-16/CCITT-FALSE This document
2       CRC-32/MPEG-2     This document
3-254   Unassigned        Available for future use
255     Reserved          This document

To request a new checksum algorithm, submit a specification to the
IANA registry contact including:
1. Algorithm name
2. Polynomial (if CRC)
3. All parameters per RFC XXXX (specify reference)
4. Test vector with expected output
```

#### Processing Unknown Registrations

For optional extensions via registry:

```markdown
If an implementation encounters a Monad option type or checksum algorithm
it does not recognize:
- Option types: Handle according to the "act" bits in the option type value
- Checksum algorithms: Discard the packet or skip checksum validation if
  configured to do so
```

---

## 7. Field Documentation Template

Use this template when documenting individual fields in a wire format specification. Fill in all sections for completeness.

### Field Documentation Template

```markdown
#### [Field Name] ([Offset]-[Offset+Size])

**Type**: [Fixed-width | Exponent-encoded | Variable-length | Bitfield | TLV | Other]

**Size**: [N octets | Variable, up to N octets]

**Byte Order**: [Big-endian (network) | Little-endian | N/A]

**Range**: [0-N | All values | Constrained values: ...]

**Default/Initial Value**: [0x00... | N/A | Depends on context]

**Semantics**:
[Description of what this field represents. 1-3 sentences.]

**Processing Rules**:
- [Rule 1]
- [Rule 2]
- [Rule 3]
- [At sender/At receiver/Both]: [specific instruction]

**Valid Values**:
| Value | Meaning |
|-------|---------|
| 0 | ... |
| 1-N | ... |
| 0xFF | Reserved |

**Error Handling**:
- If [condition]: [sender action]
- If [condition]: [receiver action]

**Interaction with Other Fields**:
[Describe any dependencies or relationships with other header fields.]

**Example**:
[Hex example or ASCII representation]

**References**:
[RFCs, external standards, or other specification sections that define or constrain this field]

**Notes**:
[Any additional implementation notes, backward compatibility concerns, etc.]
```

### Minimal Template (for simple fields)

```markdown
#### [Field Name]

- **Type**: [Type]
- **Size**: [Size]
- **Meaning**: [1-2 sentences]
- **Processing**: [Specific rules for sender/receiver]
```

### Completed Example: Monad Version Field

```markdown
#### Version (Offset 0, 1 octet)

**Type**: Fixed-width integer

**Size**: 1 octet

**Byte Order**: N/A (single octet)

**Range**: 0-255

**Default/Initial Value**: Must be set to 1 for this version

**Semantics**:
The version field indicates the version of the Monad protocol specification
that this header conforms to. This document defines version 1. Future versions
may introduce incompatible changes to the wire format.

**Processing Rules**:
- **At sender**: Set to 1
- **At receiver**:
  - If version == 1, continue processing normally
  - If version != 1, discard the packet and log a warning
  - MUST NOT attempt to parse the packet with different parsing rules

**Valid Values**:
| Value | Meaning |
|-------|---------|
| 0 | Reserved (invalid) |
| 1 | Current version (this document) |
| 2-255 | Reserved for future use |

**Error Handling**:
- If sender uses version != 1: The packet is invalid and will be discarded
- If receiver encounters version != 1: Discard the packet silently or log a warning

**Interaction with Other Fields**:
The version field affects the interpretation of all subsequent fields in the
Monad header. Fields added in future versions may alter the header layout.

**Example**:
Version field = 0x01 (1 in decimal) in the first octet of a Monad header

**References**:
This document, Section 2 (Monad Wire Format)

**Notes**:
Implementations MUST handle version mismatches robustly. A version mismatch
is not necessarily a protocol error; it indicates that the packet conforms to
a different specification and should be forwarded unchanged or discarded based
on policy.
```

### Completed Example: Monad Flow ID Field

```markdown
#### Flow ID (Offset 10-11, 2 octets)

**Type**: Fixed-width integer, big-endian

**Size**: 2 octets

**Byte Order**: Big-endian (most significant byte first)

**Range**: 0-65535

**Default/Initial Value**: 0

**Semantics**:
The flow_id field associates this packet with a logical flow or session.
Packets belonging to the same flow carry the same flow_id. This enables
routers and switches to apply per-flow policies (e.g., QoS, rate limiting,
traffic engineering). A flow_id of 0 indicates an unassociated packet.

**Processing Rules**:
- **At sender**: Set to a value that identifies the flow this packet belongs to
- **At receiver/Router**:
  - Use flow_id as a key to look up per-flow state (if available)
  - If flow state exists, apply associated policies
  - If flow state does not exist, use default forwarding

**Valid Values**:
| Value Range | Meaning |
|-------------|---------|
| 0 | No flow association (singleton packet) |
| 1-65534 | Valid flow identifiers |
| 65535 | Reserved for control packets |

**Error Handling**:
- If flow_id == 65535: This packet may be a control packet. The receiver SHOULD
  forward it without applying normal flow policies.
- If flow_id indicates a flow with QoS configuration: Apply the configured
  QoS treatment.

**Interaction with Other Fields**:
- flow_id works in conjunction with the flow_action field to determine forwarding behavior
- When flags.L (label stack present) is set, flow_id may be overridden by label-based forwarding

**Example**:
Flow ID = 0x0456 (1110 in decimal) indicates this packet belongs to flow 1110

**References**:
[RFC XXXX] Section Y (Flow Management)

**Notes**:
Flow IDs are ephemeral and may be reused across different traffic patterns.
Implementations should not assume that a flow ID persists across router boundaries
or time periods. For persistent flow identification, use higher-layer identifiers
(e.g., 5-tuple in TCP/UDP headers).
```

---

## Summary

This reference document provides patterns and templates for documenting wire formats in RFC-style specifications, with emphasis on the Monad protocol and Unheaded extension. Key takeaways:

1. **Diagrams** follow a standard 32-bit row format with consistent bit numbering
2. **Monad** is a 20-octet header with fixed and exponent-encoded fields
3. **IPv6 integration** uses the HbH extension header mechanism with option type encoding
4. **Encoding patterns** include fixed-width, exponent, varints, TLV, bitfields, and label stacks
5. **Checksums** must specify algorithm, polynomial, initial value, reflection, final XOR, scope, and test vectors
6. **Cross-protocol patterns** ensure consistency in alignment, reserved fields, versioning, header chaining, and extensibility via IANA registries
7. **Field documentation** uses a comprehensive template to capture all relevant information

Refer to this document when writing wire format specifications for clarity, consistency, and RFC compliance.
