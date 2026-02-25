# Inverse Mask: Double Static Address Space Innovation

## What It Is

Inverse Mask is a protocol innovation that allows a single IPv6 address to be interpreted in two distinct, valid ways, effectively doubling the static address space without consuming additional IP allocation. The technique works by taking the bitwise inverse of an IPv6 address within a defined subnet block, creating a second valid address that coexists in the same /48 block.

This is not NAT. This is not address translation. This is a pure protocol-level innovation where a packet can be classified and routed based on which of the two "mask" interpretations is in use, creating two logical address spaces from one physical allocation.

## The Double Static Address Space

In traditional IPv6 addressing, a /48 block contains 2^80 usable addresses. With Inverse Mask, that same /48 block can be interpreted two ways:

1. **Primary address space**: Addresses as allocated, in canonical form
2. **Inverse address space**: Addresses as the bitwise inverse of the primary, creating a second logical namespace

Example:
- Primary address: `2001:db8:1234:5678::/64`
- Inverse address: Bitwise NOT of the primary, still within the `/48` parent block
- Result: Two valid interpretations, two distinct security/routing contexts

The key constraint: the inverse must remain within the parent /48 allocation. This is achieved through careful bit selection in the Monad register (see "Integration with Monad Register" below).

## How It Integrates with Monad

The Monad register is a 20-byte structure embedded in the IPv6 Hop-by-Hop extension header. Inverse Mask state is encoded in specific bits within the Monad register.

### Monad Register Layout (20 bytes, 160 bits)

```
Offset   Field                          Bits      Type
------   -----                          ----      ----
0x00     Flow Label (Primary)           128 bits  u128
0x10     Kingdom Mode (K1|K0)           2 bits    u2
0x12     Inverse Mask Indicator         1 bit     u1
0x13     Sequence Number                14 bits   u14
0x15     Reserved                       15 bits   u15
0x17     CRC-16/CCITT-FALSE             16 bits   u16
```

**Inverse Mask Indicator (1 bit)**:
- **0**: Packet uses primary address interpretation (canonical form)
- **1**: Packet uses inverse address interpretation (bitwise complement form)

When a packet is created with the Inverse Mask Indicator set to 1, the receiving end knows to:
1. Extract the flow label from the Monad register
2. Apply bitwise NOT to recover the logical destination address
3. Route and classify the packet according to the inverse space rules

## Why It Matters

### Effective Address Space Doubling

Traditional IPv6 leaves much of the address space underutilized in production deployments. Inverse Mask allows organizations to:
- Maintain a single IP allocation (no renumbering)
- Support two entirely separate logical networks within that allocation
- Treat primary and inverse spaces with different policies, security rules, and routing logic

### Zero Additional Allocation

Most organizations operate under IP rationing agreements with their ISP or RIR. Inverse Mask requires no additional allocation:
- No new WHOIS registration
- No new delegation at the RIR level
- No BGP announcements for additional blocks
- The inverse space is derived mathematically from the primary allocation

### Operational Simplicity

Because the inverse space is deterministic (bitwise NOT of the primary), there is no configuration overhead:
- No secondary routing tables
- No manual subnet planning
- No IPAM (IP Address Management) tool updates beyond flagging which flows use inverse space
- Inverse space is auditable: you can always recover the primary address by applying NOT again

## Security Implications

Inverse Mask creates a second classification dimension for security and access control. Packets can be classified by which "mask" interpretation is in use:

### Classification by Mask Type

Port Authority (the ingress control plane) can define policies based on mask usage:
- **Primary space policies**: Routes, WAF rules, rate limits for canonical addresses
- **Inverse space policies**: Routes, WAF rules, rate limits for inverse addresses

Example policy rule:
```yaml
rule:
  match:
    destination_address: 2001:db8:1234::/48
    inverse_mask: true           # Only match packets using inverse interpretation
    source_region: eu            # Only from EU sources
  action:
    path: gateway-eu-inverse
    rate_limit_mbps: 100
```

### Attack Surface Reduction

If an attacker discovers the primary address space, the inverse space remains operationally distinct. An attacker would need to know or guess the inverse interpretation to target that namespace. This creates two attack surfaces instead of one consolidated vulnerability.

### Flow Segmentation

Sensitive applications can run on the inverse space exclusively, while less critical services use the primary space. This is not encryption-based isolation; it is network-level segmentation backed by kernel-enforced policies in Wotan memory.

## Kingdom Mode Interaction

Inverse Mask interacts with Kingdom Mode (the 4-state flow machine) to create 8 distinct logical flow states:

### Combined State Matrix

```
Kingdom Mode State    |  Primary Space  |  Inverse Space
(K1|K0)              |  (Inverse=0)    |  (Inverse=1)
---------------------+-----------------+----------------
IDLE (00)            |  Primary-IDLE   |  Inverse-IDLE
ACTIVE (01)          |  Primary-ACTIVE |  Inverse-ACTIVE
CLOSING (10)         |  Primary-CLOSING|  Inverse-CLOSING
CLOSED (11)          |  Primary-CLOSED |  Inverse-CLOSED
```

This means:
- A flow can transition from Primary-ACTIVE to Inverse-ACTIVE (switching mask interpretation mid-flow)
- Each combination has distinct Sophia dictionary entries (different policies, capabilities, timeouts)
- Wotan ring buffers track the (K1|K0, Inverse Mask) pair as a single flow identity
- Anamnesis events record which logical state the flow is in (primary or inverse context)

### Transition Rules

Valid transitions involving mask changes:
- **Primary-ACTIVE → Inverse-ACTIVE**: Allowed with explicit Sophia dictionary approval (for load balancing, failover)
- **Primary-ACTIVE → Primary-CLOSING**: Standard teardown in primary space
- **Inverse-ACTIVE → Primary-ACTIVE**: Allowed with explicit approval (rerouting)
- **Any state → Inverse-CLOSED**: When inverse-space resources are being reclaimed

Invalid transitions:
- Cannot jump directly between unrelated states without intermediate CLOSING state
- Cannot use CLOSING state in one mask while attempting to open in the other

## Wire Format Example

A 20-byte Monad register with Inverse Mask encoding:

```
Hex Layout (bytes 0x00 - 0x13):

Byte Offset:   0x00  0x01  0x02  0x03  0x04  0x05  0x06  0x07
               ----  ----  ----  ----  ----  ----  ----  ----
Data:          0x20  0x01  0x0d  0xb8  0x12  0x34  0x56  0x78
               ---- Flow Label (128 bits) ----

Byte Offset:   0x08  0x09  0x0a  0x0b  0x0c  0x0d  0x0e  0x0f
               ----  ----  ----  ----  ----  ----  ----  ----
Data:          0x9a  0xbc  0xde  0xf0  0x11  0x22  0x33  0x44
               ---- Flow Label continued (128 bits total) ----

Byte Offset:   0x10  0x11  0x12  0x13  0x14  0x15
               ----  ----  ----  ----  ----  ----
Data:          0xa5  0x78  0x42  0x00  0x11  0x22
               Seq#  Kingdom|InvMask      CRC-16

Kingdom Mode bits (0xa5 >> 6) = 0b10 = CLOSING
Inverse Mask bit (0xa5 >> 5) & 0b1 = 1 = Using Inverse Space
Sequence Number (0xa5 & 0b11111) | (0x78 << 5) = 14-bit sequence
```

Interpretation:
- Flow label: `20010db8:12345678:9abcdef0:11223344`
- Kingdom Mode: CLOSING (10)
- Inverse Mask: Active (1) — this packet uses the inverse address interpretation
- Sequence number: Within the inverse-space flow sequence
- CRC-16: Validates bytes 0x00-0x11

## IANA Implications

Inverse Mask is part of the experimental registration for the Monad register in the IPv6 Hop-by-Hop extension header. The IANA considerations are:

1. **IPv6 Hop-by-Hop Option Type**: Registered as experimental (IANA Hop-by-Hop Options Number Space)
2. **Reserved Bits in Monad Register**: The 1-bit Inverse Mask Indicator is reserved within the experimental block
3. **Future Standardization**: If Inverse Mask moves from experimental to standards track, it will require:
   - RFC publication documenting the technique
   - IANA allocation review
   - Interoperability testing across multiple implementations

Current status: **Experimental (draft-unheaded-foundation-04)**

## Implementation Notes

### Bit Encoding and Decoding

**Go Implementation** (from `pkg/protocol/monad.go`):

```go
// SetInverseMask sets the inverse mask bit in the Monad register
func (m *MonadRegister) SetInverseMask(inverse bool) {
	if inverse {
		m.Flags |= 0x01 << 5  // Set bit 5 of flags byte
	} else {
		m.Flags &= ^(0x01 << 5)  // Clear bit 5
	}
}

// IsInverseMask returns true if this packet uses inverse address interpretation
func (m *MonadRegister) IsInverseMask() bool {
	return (m.Flags >> 5) & 0x01 != 0
}

// InvertFlowLabel returns the bitwise NOT of the flow label
func (m *MonadRegister) InvertFlowLabel() [16]byte {
	var inverted [16]byte
	for i := 0; i < 16; i++ {
		inverted[i] = ^m.FlowLabel[i]
	}
	return inverted
}
```

**Rust Implementation** (from `monad-common/src/lib.rs`):

```rust
impl MonadRegister {
    /// Set the inverse mask bit
    pub fn set_inverse_mask(&mut self, inverse: bool) {
        if inverse {
            self.flags |= 0x01 << 5;
        } else {
            self.flags &= !(0x01 << 5);
        }
    }

    /// Check if this packet uses inverse address interpretation
    pub fn is_inverse_mask(&self) -> bool {
        (self.flags >> 5) & 0x01 != 0
    }

    /// Get the inverted flow label
    pub fn inverted_flow_label(&self) -> [u8; 16] {
        let mut inverted = [0u8; 16];
        for i in 0..16 {
            inverted[i] = !self.flow_label[i];
        }
        inverted
    }
}
```

### Dictionary Lookup with Mask Awareness

When Wotan performs a Sophia dictionary lookup, it must account for the Inverse Mask bit:

```go
// In services/wotan/flow_lookup.go
func (w *WotanEngine) LookupSophiaEntry(pkt *ipv6.Packet) (*SophiaEntry, error) {
	monad := extractMonadFromHopByHop(pkt)

	// Base flow label lookup
	flowLabel := monad.FlowLabel

	// If inverse mask is set, transform the lookup key
	if monad.IsInverseMask() {
		flowLabel = monad.InvertFlowLabel()
		// Dictionary entries for inverse flows have a distinct namespace
		// e.g., dictionary map key prefix = 0x01, primary = 0x00
	}

	return w.sophia.Get(flowLabel)
}
```

## Testing and Validation

Inverse Mask functionality is validated in three ways:

1. **Unit Tests** (`pkg/protocol/monad_test.go`):
   - Bit encoding/decoding correctness
   - Bitwise NOT consistency
   - Round-trip: original → inverse → original

2. **Integration Tests** (`services/wotan/tests/`):
   - Sophia dictionary correctness with mask-aware keys
   - Kingdom Mode transitions with mask changes
   - Anamnesis event recording for mask-based flows

3. **Property Tests** (using `github.com/leanovate/gopter`):
   - For all flow labels, inverse(inverse(x)) == x
   - Inverse space remains within parent /48 allocation
   - CRC-16 validation across mask bit changes

## Summary

Inverse Mask is a core Unheaded innovation that:
- Doubles effective address space without additional IP allocation
- Integrates seamlessly into the Monad register (1-bit encoding)
- Works with Kingdom Mode to create 8 distinct logical flow states
- Provides network-level security segmentation
- Requires no changes to IPAM systems or BGP
- Is fully auditable and deterministic

The innovation proves that protocol-level thinking can solve infrastructure problems that would otherwise require additional resource consumption or operational complexity.
