# IANA Considerations Authoring Guide for Unheaded Protocol

## Overview

This guide provides comprehensive guidance for authoring IANA Considerations sections in Internet-Drafts (I-Ds) and RFCs that deal with protocol assignments and registry management. It is based on RFC 8126 "Guidelines for Writing an IANA Considerations Section in RFCs" and extends those guidelines with specific registry definitions for the Unheaded protocol stack.

---

## 1. When IANA Considerations Are Needed

### Rules for Inclusion

Include an IANA Considerations section when your document:

- **Creates a new registry** — Defines a new IANA registry that will be used by future RFCs or implementations
- **Requests registry assignments** — Allocates specific values in existing IANA registries
- **Modifies an existing registry** — Changes policies, adds new fields, or reallocates reserved ranges
- **Requests experimental values** — Allocates code points for experiments or testing
- **Requires designated experts** — Needs expert review of future registrations
- **Establishes assignment procedures** — Defines policies for future allocations

### Rules for Omission

Omit an IANA Considerations section when:

- No registry assignments are needed
- No new registries are created
- No modifications to existing registries are required
- All protocol parameters are fixed at specification time
- The document is informational and makes no normative requirements

When omitted, include the following statement instead:

```
## IANA Considerations

This document has no IANA actions.
```

---

## 2. Creating New Registries

### When to Create

Create a new registry when:

1. Multiple future RFCs will need to register values
2. An extensible set of code points must be managed
3. Different registration policies apply to different value ranges
4. Expert review or approval processes are required

### New Registry Template

Use this template to define a new registry. All fields are required unless otherwise noted:

```
### Registry Name

Name: [Human-readable registry name, e.g., "Unheaded Monad Flags"]

Abbreviation: [Short name, e.g., "Unheaded-Monad-Flags"] (optional)

Type: [Bitfield, Integer, String, or other format]

Purpose: [Clear, concise statement of purpose and use]

Registration Procedure(s): [See Section 4 for policy descriptions]

Initial Values:
  [Value] - [Name] - [Grouping] - [Reference]
  ...

Namespace Description:

[Detailed description of the namespace, including:]
- Valid value ranges
- Semantic meaning of each range
- Allocation strategies
- Reserved ranges and their purposes

Value Specification:

[Define the format and semantics of values:]
- Data type and range
- Encoding format
- Bit/byte order considerations
- Optional field specifications

Reserved Ranges:

- [Range 1]: [Designation] [RFC or other reference]
- [Range 2]: [Designation] [RFC or other reference]

Change Control:

[Who controls registration:] [IETF, Designated Expert, Manufacturer, Private Use]

Reference: [RFC/Draft reference]
```

### Key Field Definitions

**Name**: The official registry name as it will appear in IANA registries.

**Abbreviation**: A short form for use in text.

**Type**: The data structure type (Bitfield, Integer Range, String, Enumeration, etc.).

**Purpose**: What this registry is for and which protocols use it.

**Registration Procedure(s)**: Which RFC 8126 policy/policies govern registration (see Section 4).

**Initial Values**: Pre-assigned values at the time of registration creation. Include:
- The value itself
- Its assigned name
- The grouping it belongs to (if applicable)
- The reference document

**Namespace Description**:
- Define the complete value space
- Specify allocation strategies for different ranges
- Explain reserved and private-use ranges
- Include handling of unspecified values

**Value Specification**:
- Exact format (bits, bytes, strings)
- Encoding and byte order
- Optional vs. mandatory fields
- Constraints and validation rules

**Reserved Ranges**:
- Values reserved for standards action
- Values reserved for experimental use
- Values for private use
- Values reserved for future use

**Change Control**: Who may register new values:
- IETF = IESG Approval
- Designated Expert = Expert Review
- Manufacturer/Organization = First Come First Served
- Private Use = No IANA registration required

---

## 3. Registering Into Existing Registries

### When to Register

Register into an existing registry when:

- Your specification defines values that should be available to all implementations
- The registry has an open registration policy
- Your values will be useful beyond your single implementation

### Registration Template

For each registry entry, provide:

```
Registry: [Name from IANA registry list]

Registry Identifier: [Short code if applicable, e.g., "type-0x3E"]

Value(s):
  [Numeric or string value] - [Name] - [Description] - [Reference]

Range: [If a range is being allocated, specify the range]

Change Controller: [IETF/Organization/Manufacturer]

Required Information for Registration:
  - [Field 1]: [Description and constraints]
  - [Field 2]: [Description and constraints]

Contact/Reference: [RFC/I-D section]

Notes: [Any special considerations or requirements]
```

### Key Considerations

**Registry Identification**: Use the exact name from IANA registry pages.

**Value Format**: Match the registry's defined value format exactly.

**Range Allocation**: If allocating a range rather than individual values, specify the complete range.

**Change Controller**: Designate who manages this registration (usually IETF for standards-track documents).

**Required Information**: State what information registrants must provide for future registrations in this space.

---

## 4. Registration Policies

RFC 8126 defines ten registration policies, ordered by how strictly they control registrations. Choose policies based on your security and interoperability requirements.

### 4.1 Private Use

**Policy**: Private Use values require no IANA registration or approval.

**When to Use**:
- For values that should never be allocated publicly
- For experimental or testing purposes within an organization
- For vendor-specific or implementation-specific parameters

**Characteristics**:
- IANA maintains a range but does not assign specific values
- No approval or review process
- Suitable for implementation variants

**Reserved Range Guidance**: Allocate 25%-50% of namespace to Private Use for flexibility.

**Example**: Private Use IPv6 addresses (fc00::/7)

---

### 4.2 Experimental Use

**Policy**: Experimental Use values are allocated on an unrestricted basis.

**When to Use**:
- For temporary testing and research
- For values needed during I-D development before standards ratification
- For proof-of-concept implementations

**Characteristics**:
- IANA maintains a registry of experimental allocations
- Registrations are approved automatically
- May require periodic renewal or notification

**Reserved Range Guidance**: Allocate 5%-10% of namespace to Experimental Use.

**Documentation Required**:
- Point of contact
- Expected duration of use
- Brief description of experiment

**Example**: DHCP Experimental Options

---

### 4.3 First Come First Served

**Policy**: Registrations are approved in the order received.

**When to Use**:
- For non-critical parameters where rapid allocation is important
- When there are many potential registrants
- When expert review is unnecessary

**Characteristics**:
- Low barrier to entry
- Fast allocation process
- Minimal review or vetting
- IANA performs basic sanity checks only

**Documentation Required**:
- Unique name or identifier
- Brief description
- Point of contact

**No Designated Expert Review**: Registrations are processed automatically.

**Example**: HTTP Content-Type parameters

---

### 4.4 Specification Required

**Policy**: Registrations must be associated with a permanent, publicly available specification.

**When to Use**:
- For parameters that need clear definition
- When implementations must understand the meaning
- For general-purpose parameters affecting interoperability

**Characteristics**:
- Document required but does not need IESG approval
- Can be any permanent, publicly available specification
- Includes I-Ds, RFCs, academic papers, archived documents

**IANA Verification**:
- Specification must exist and be accessible
- Must clearly define the registered item
- Must be permanently available

**Documentation Required**:
- Complete specification document
- Clear description of the parameter
- How to use/interpret the registered value

**Example**: DNS Resource Record Types

---

### 4.5 Expert Review

**Policy**: Registrations require review by designated expert(s).

**When to Use**:
- When expert judgment is essential for registry health
- For parameters requiring consistency review
- When preventing poor registrations improves the ecosystem
- For specialized technical areas

**Characteristics**:
- Designated experts review all proposals
- Experts can request clarifications or revisions
- Experts may reject unsuitable proposals
- Requires clear expert guidelines document

**Expert Responsibilities**:
- Ensure specification is adequate
- Check for namespace conflicts
- Verify technical correctness
- Assess general applicability
- Follow published review guidelines

**Documentation Required**:
- Complete specification
- Use case and motivation
- Interoperability considerations
- Security considerations
- Clear naming following namespace conventions

**Timeline**: Experts typically have 2-4 weeks to review.

**Example**: IPv6 Extension Header Types, Unheaded Protocol Extensions

---

### 4.6 Specification Required + Expert Review

**Policy**: Combination of Specification Required and Expert Review.

**When to Use**:
- For critical parameters requiring both documentation and expert vetting
- When the parameter's impact on interoperability is significant
- For security-sensitive registrations

**Characteristics**:
- All requirements of both Specification Required and Expert Review apply
- Specification must be comprehensive
- Expert review adds additional scrutiny

**Documentation Required**:
- Full technical specification
- Clear use case and motivation
- Interoperability analysis
- Security and privacy implications
- References to related registrations

**Timeline**: Typically 3-6 weeks (expert review time plus IANA processing).

**Example**: BGP Capabilities

---

### 4.7 Standards Action

**Policy**: Registrations require publication as an RFC.

**When to Use**:
- For fundamental protocol parameters
- When the parameter impacts protocol interoperability broadly
- For security-critical assignments
- For values likely to be widely deployed

**Characteristics**:
- RFC must be published (not I-D)
- Must go through IETF standards process
- Includes IESG review and approval
- Highest bar for registration

**Publication Type**: RFC (Standards Track, Informational, or BCP)

**IANA Processing**:
- Registrations occur when RFC is published
- Automatic for values defined in the RFC text
- No additional IANA approval needed beyond IESG approval

**Documentation Requirements**:
- Complete IANA Considerations section in RFC
- All values clearly assigned
- Purpose and semantics fully specified
- Impact assessment on other protocols

**Example**: TCP Port assignments, DNS Record Types

---

### 4.8 IESG Approval

**Policy**: Registrations require explicit IETF Steering Group approval.

**When to Use**:
- For emergency or critical allocations
- When normal Standards Action timeline is too slow
- For strategic protocol decisions
- For allocations needing broad IESG review

**Characteristics**:
- IESG must explicitly approve each registration
- Can occur without RFC publication
- Slower than Standards Action
- Requires strong justification

**When to Choose Over Standards Action**:
- Need faster timeline than RFC publication
- Assignment cannot wait for standards process
- Policy or strategic considerations require IESG attention

**Documentation Requirements**:
- Detailed justification for need
- Complete specification
- Security and interoperability analysis
- Statement of support from relevant Area Directorates

**Example**: Rare, usually for emergency protocol assignments

---

### 4.9 Requires Specific RFC

**Policy**: Registration requires a specific RFC to be published first.

**When to Use**:
- When a registration depends on another specification being finalized
- For parameters that form a coupled ecosystem
- When a prerequisite RFC must exist

**Characteristics**:
- Registration is blocked until the prerequisite RFC is published
- Usually not independent registrations

**Example**: DNSSEC algorithm numbers depending on cryptographic RFC

---

### 4.10 Not Recommended

**Policy**: This classification indicates a registry exists but IANA recommends against new registrations.

**When to Use**:
- For deprecated parameters
- For registries retained for historical reasons
- For superseded protocols or specifications

**Characteristics**:
- Registry remains open for backwards compatibility
- IANA actively discourages new use
- Existing registrations may remain valid

**Example**: Obsolete TCP options

---

## 5. Designated Experts

### Role and Responsibilities

A Designated Expert (DE) is an individual selected to review proposed registrations in an Expert Review registry. The expert acts on behalf of the IETF community to ensure registry quality.

### Selection Process

**Who Selects**:
- For IETF protocols: IESG appoints experts for each registry
- For vendor registries: The registry maintainer selects experts

**Qualifications**:
- Deep technical knowledge of the registry's subject area
- Experience with IETF processes and standards
- Good judgment and fairness
- Availability for timely reviews

**Term Length**: Typically 2-5 years, subject to reappointment.

### Key Responsibilities

1. **Review Specifications**: Evaluate proposed registrations against clear guidelines
2. **Assess Technical Merit**: Verify that proposals meet standards and won't harm the registry
3. **Check Namespace Consistency**: Ensure names and values don't conflict
4. **Request Clarifications**: Ask proposers for additional information
5. **Make Decisions**: Accept, conditionally accept, or reject proposals
6. **Document Decisions**: Provide rationales for the community
7. **Follow Guidelines**: Apply published expert review guidelines consistently
8. **Communicate Timely**: Provide feedback within the promised timeline (typically 2-4 weeks)

### Expert Review Guidelines

Create a document providing guidance to experts. Include:

- **Purpose of Registry**: Why it exists and what it controls
- **Technical Criteria**: What makes a good registration
- **Consistency Requirements**: How proposals should align with existing entries
- **Security Checks**: What security considerations experts should verify
- **Naming Conventions**: How names should be formed
- **Rejection Criteria**: Specific reasons to reject proposals
- **Timeline**: Expected review duration
- **Escalation Path**: How to handle disputed decisions

### Communication with Experts

**IANA Notifies Experts**: IANA staff inform experts of new proposals and deadlines.

**Expert Reviews**: Expert examines specification and interacts with proposer as needed.

**Expert Decision**: Expert informs proposer and IANA of decision and rationale.

**IANA Registration**: Upon expert approval, IANA records the registration.

**Appeals**: If proposer disagrees with rejection, escalate to IESG.

---

## 6. TBD Placeholders

### Purpose

TBD (To Be Determined) placeholders allow authors to write complete I-Ds without allocating specific registry values before IANA approval.

### Usage Rules

**Syntax**: `TBAN` where N is a sequence number starting at 1.

Examples: `TBA1`, `TBA2`, `TBA3`

**Single Registry**: Use one sequence for each registry.

**Consistency**: Use the same `TBAN` reference throughout the document for the same value.

**IANA Instructions**: Clearly tell IANA what each `TBAN` refers to.

### Example in I-D

```
The following new registry is created:

Name: Unheaded Monad Flags
Type: Bitfield
Initial Values:
  0 - C (Crypto) flag - [TBA1]
  1 - Y (Yielding) flag - [TBA2]
  2 - T (Token) flag - [TBA3]
  ...

IANA SHALL replace TBA1 with the bit assigned to the C flag,
TBA2 with the Y flag, and so forth.
```

### Replacement Process

When the RFC is published:

1. IANA receives the RFC with `TBAN` placeholders
2. IANA assigns specific values per the registration procedure
3. IANA publishes the registry with actual values, not TBA references
4. IANA sends confirmation to RFC author and document editor

### When NOT to Use TBA

- **Algorithmic Values**: If values are defined algorithmically, use the algorithm, not TBA
- **Reference Values**: If values are defined by reference to another standard, cite directly
- **Fixed Values**: If specific values must be used for protocol function, don't use TBA

---

## 7. Unheaded Protocol IANA Registries

### Registry 1: IPv6 HbH Option Type

**Name**: IPv6 Hop-by-Hop Option Types (Unheaded)

**Type**: Integer (0-255)

**Purpose**: Identifies option types in IPv6 Hop-by-Hop extension headers used by Unheaded protocols.

**Registration Procedure**: Standards Action

**Initial Values**:
| Value | Name | Reference |
|-------|------|-----------|
| 0x3E | Monad Option | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-255 (8-bit field)
- Even values (bit 0 = 0): Skip unrecognized option
- Odd values (bit 0 = 1): Discard unrecognized option
- High bit (bit 7): Change Flag (1=option changed in transit)
- Remaining bits (bits 1-6): Option type identifier

**Reserved Ranges**:
- 0x00-0x3D: Reserved for other HbH options
- 0x3E: Monad (assigned)
- 0x3F-0xFF: Reserved for future HbH options

**Change Control**: IETF (Standards Action)

**Reference**: [RFC-UNHEADED]

---

### Registry 2: Monad Version Registry

**Name**: Monad Protocol Versions

**Type**: Integer (0-255)

**Purpose**: Identifies versions of the Monad protocol.

**Registration Procedure**: Standards Action

**Initial Values**:
| Version | Name | Description | Reference |
|---------|------|-------------|-----------|
| 0 | Reserved | MUST NOT be used | [RFC-UNHEADED] |
| 1 | Monad v1.0 | Current version (this document) | [RFC-UNHEADED] |
| 2-255 | Unassigned | Reserved for future versions | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-255 (8-bit unsigned integer at Monad offset 0x00)
- Version 0 is reserved and MUST NOT be used
- Version 1 is defined by [RFC-UNHEADED] Foundation
- Versions 2-255 are reserved for future protocol revisions
- Each version number represents a potentially incompatible wire format

**Processing Rules**:
- Senders MUST set version to 1 for this protocol
- Receivers MUST verify version == 1; if not, drop the packet
- Routers MUST NOT attempt to parse packets with unknown versions

**Change Control**: IETF (Standards Action)

**Reference**: [RFC-UNHEADED] Foundation, Section 5.1

---

### Registry 3: Monad Flags Bitfield Registry

**Name**: Monad Flags Bitfield

**Type**: Bitfield (8 bits)

**Purpose**: Defines flag bits in the Monad header flags field.

**Registration Procedure**: Specification Required + Expert Review

**Initial Values**:
| Bit | Position | Name | Meaning | Reference |
|-----|----------|------|---------|-----------|
| 7 | C | CHAOS | Chaos injection active (Yaldabaoth) | [RFC-UNHEADED] |
| 6 | Y | CANARY | Canary deployment path | [RFC-UNHEADED] |
| 5 | T | TRACED | Full trace active (all hops to Anamnesis) | [RFC-UNHEADED] |
| 4 | E | ENCRYPT | Payload encrypted (intra-Kingdom TLS) | [RFC-UNHEADED] |
| 3 | S | SAMPLED | Statistically sampled | [RFC-UNHEADED] |
| 2 | M | MIRROR | Mirror copy (not original) | [RFC-UNHEADED] |
| 1 | CUSTOM | Scratch/checksum exponent-encoded | Scratch and checksum carry exponent values | [RFC-UNHEADED] |
| 0 | RSVD | Reserved | Reserved, MUST be zero | [RFC-UNHEADED] |

**Namespace Description**:
- Each bit represents a separate flag
- Bit 7 is the most significant bit (MSB)
- Bit 0 is the least significant bit (LSB)
- Bits 0-2: Control packet format interpretation
- Bits 3-7: Per-packet processing directives
- Reserved bits must be set to 0 by senders and ignored by receivers

**Reserved Ranges**:
- Bits 0-7: As defined in the Initial Values table
- All bits are currently assigned
- No bits available for future flags without protocol version change

**Change Control**: IETF (Specification Required + Expert Review)

**Expert Review Guidance**:
- Proposed flags must define clear, non-overlapping semantics
- Flags must be backward-compatible with existing Monad implementations
- Proposers must justify why the flag cannot be handled as an option instead
- Specification must include use cases and interoperability impact

**Reference**: [RFC-UNHEADED] Section 5.2

---

### Registry 4: Flow Action Registry

**Name**: Unheaded Flow Actions

**Type**: Integer (0-255)

**Purpose**: Defines actions that can be applied to Monad flows within the Unheaded protocol.

**Registration Procedure**: Expert Review

**Initial Values**:
| Value | Name | Description | Reference |
|-------|------|-------------|-----------|
| 0x00 | FORWARD | Forward packet normally | [RFC-UNHEADED] |
| 0x01 | TRACE | Full trace to Anamnesis | [RFC-UNHEADED] |
| 0x02 | SAMPLE | Statistical sampling | [RFC-UNHEADED] |
| 0x03 | MIRROR | Mirror copy to secondary | [RFC-UNHEADED] |
| 0x04 | RATE_LIMIT | Apply rate limiting | [RFC-UNHEADED] |
| 0x05 | DROP | Drop packet | [RFC-UNHEADED] |
| 0x06-0x0F | RESERVED | Reserved for future standard actions | [RFC-UNHEADED] |
| 0x10 | KEY_ANNOUNCE | PQC key announcement | [RFC-UNHEADED] |
| 0x11 | KEY_ROTATE | PQC key rotation | [RFC-UNHEADED] |
| 0x12 | KEY_REVOKE | PQC key revocation | [RFC-UNHEADED] |
| 0x13 | KEM_ENCAPS | ML-KEM encapsulation request | [RFC-UNHEADED] |
| 0x14 | KEM_DECAPS | ML-KEM decapsulation request | [RFC-UNHEADED] |
| 0x15 | KEY_ACK | PQC key acknowledgment | [RFC-UNHEADED] |
| 0x16 | KEY_REJECT | PQC key rejection | [RFC-UNHEADED] |
| 0x17-0xEF | UNASSIGNED | Future standard actions | [RFC-UNHEADED] |
| 0xF0-0xFE | EXPERIMENTAL | Experimental use | [RFC-UNHEADED] |
| 0xFF | RESERVED | Invalid / reserved | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-255 (8-bit action identifier)
- Values 0x00-0x05: Standard forwarding actions
- Values 0x06-0x0F: Reserved for future standard actions
- Values 0x10-0x16: PQC key lifecycle and cryptographic actions
- Values 0x17-0xEF: Reserved for future standardized actions
- Values 0xF0-0xFE: Experimental use
- Value 0xFF: Reserved (invalid)

**Reserved Ranges**:
- 0x00-0x05: Standard actions (assigned)
- 0x06-0x0F: Future standard actions (Standards Action required)
- 0x10-0x16: PQC lifecycle and cryptographic actions (assigned)
- 0x17-0xEF: Future standardized actions (Standards Action required)
- 0xF0-0xFE: Experimental actions (no registration required)
- 0xFF: Invalid/Reserved

**Change Control**: IETF (Expert Review for 0x13-0xEF)

**Expert Review Guidance**:
- Actions must have clear, deterministic behavior
- Actions must not conflict with existing implementations
- Actions must work with all Monad versions
- Specification must include performance impact assessment
- Specification must define interaction with other actions

**Required Information for Registration**:
- Action name (must be descriptive and follow naming conventions)
- Detailed semantic definition
- Interaction with other actions
- Performance and resource implications
- Example use cases
- Backward compatibility assessment

**Reference**: [RFC-UNHEADED]

---

### Registry 5: QoS Class Registry

**Name**: Unheaded QoS Classes

**Type**: Integer (0-7)

**Purpose**: Defines Quality of Service classes for Monad flow handling.

**Registration Procedure**: Specification Required

**Initial Values**:
| Value | Name | Priority | Meaning | Reference |
|-------|------|----------|---------|-----------|
| 0 | DEFAULT | Low | Best effort / default forwarding | [RFC-UNHEADED] |
| 1 | REALTIME | High | Real-time traffic (voice, low-latency) | [RFC-UNHEADED] |
| 2 | INTERACTIVE | Medium | Interactive traffic (web, API) | [RFC-UNHEADED] |
| 3 | BULK | Low | Bulk transfer (backups, replication) | [RFC-UNHEADED] |
| 4-7 | RESERVED | - | Reserved for future use | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-3 (2-bit field with reserved high bits)
- Classes 0-3: Currently defined
- Classes 4-7: Reserved for future use
- Each class defines default handling expectations
- Implementations should honor class priorities

**Reserved Ranges**:
- 0-3: Defined QoS classes
- 4-7: Reserved for future QoS classes

**Change Control**: IETF (Specification Required)

**Required Information for Registration**:
- QoS class name and priority level
- Handling expectations and SLA implications
- Interaction with network QoS mechanisms
- Resource reservation requirements
- Example applications

**Reference**: [RFC-UNHEADED]

---

### Registry 6: Circuit State Registry

**Name**: Unheaded Circuit States

**Type**: Enumeration (0-31)

**Purpose**: Defines states of virtual circuits in Monad connections.

**Registration Procedure**: Standards Action

**Initial Values**:
| Value | State Name | Description | Reference |
|-------|-----------|-------------|-----------|
| 0 | CLOSED | Circuit breaker closed (normal flow) | [RFC-UNHEADED] |
| 1 | OPEN | Circuit breaker open (failing, reject fast) | [RFC-UNHEADED] |
| 2 | HALF_OPEN | Circuit breaker half-open (probe in progress) | [RFC-UNHEADED] |
| 3-31 | RESERVED | Reserved for future states | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-31 (5-bit field)
- Values 0-2: Currently defined states (circuit breaker states)
- Values 3-31: Reserved for future states
- Each state follows the circuit breaker pattern
- State transitions follow circuit breaker semantics

**Reserved Ranges**:
- 0-2: Defined circuit breaker states
- 3-31: Reserved for future states

**Change Control**: IETF (Standards Action)

**Reference**: [RFC-UNHEADED]

---

### Registry 7: Kingdom Mode Registry

**Name**: Kingdom Modes

**Type**: Integer (0-3)

**Purpose**: Defines operational modes for Kingdom protocol elements. Reserved for v04.

**Registration Procedure**: Standards Action

**Initial Values**:
| Value | Mode | Description | Reference |
|-------|------|-------------|-----------|
| 0 | NORMAL | Normal forwarding mode (default) | [RFC-UNHEADED] |
| 1 | PRIORITY | Priority processing mode | [RFC-UNHEADED] |
| 2 | EXPERIMENTAL | Experimental / custom pipeline | [RFC-UNHEADED] |
| 3 | RESERVED | Reserved for future use | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-3 (2-bit field)
- All four values are fully allocated
- Each mode defines different processing behavior
- Mode determines which protocols/features are enabled
- Note: Kingdom Mode is reserved for future versions

**Reserved Ranges**:
- 0-3: All values allocated

**Change Control**: IETF (Standards Action)

**Reference**: [RFC-UNHEADED]

---

### Registry 8: Sophia Sub-Dictionary Type Registry

**Name**: Sophia Sub-Dictionary Types

**Type**: Integer (0-255)

**Purpose**: Defines types of sub-dictionaries within Sophia knowledge structures.

**Registration Procedure**: Expert Review

**Initial Values**:
| Value | Type | Purpose | Reference |
|-------|------|---------|-----------|
| 0 | ATTRIBUTES | Entity attributes and properties | [RFC-UNHEADED] |
| 1 | RELATIONS | Relationships to other entities | [RFC-UNHEADED] |
| 2 | HISTORY | Historical state information | [RFC-UNHEADED] |
| 3 | METADATA | Metadata about knowledge item | [RFC-UNHEADED] |
| 4 | RULES | Associated inference rules | [RFC-UNHEADED] |
| 5 | REFERENCES | Cross-references | [RFC-UNHEADED] |
| 6 | CONSTRAINTS | Validity constraints | [RFC-UNHEADED] |
| 7 | DERIVATIONS | Derived facts and conclusions | [RFC-UNHEADED] |
| 8 | CONTEXT | Contextual information | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-255 (8-bit field)
- Values 0-8: Standard sub-dictionary types
- Values 9-239: Reserved for future types
- Values 240-255: Experimental and implementation-specific types

**Reserved Ranges**:
- 0-8: Standard types
- 9-239: Future standard types
- 240-255: Experimental/private use

**Change Control**: IETF (Expert Review for 9-239)

**Expert Review Guidance**:
- Types must represent coherent, distinct knowledge categories
- Types must be compatible with Sophia knowledge representation
- Specification must define structure and semantics clearly
- Types must not overlap with existing type semantics

**Required Information for Registration**:
- Type name and numeric value
- Purpose and use cases
- Structure and schema of sub-dictionary
- Example content
- Compatibility with existing types

**Reference**: [RFC-UNHEADED]

---

### Registry 9: Anamnesis Event Type Registry

**Name**: Anamnesis Event Types

**Type**: Integer (0-255)

**Purpose**: Defines types of events in the Anamnesis ring buffer for packet observability.

**Registration Procedure**: Specification Required

**Initial Values**:
| Value | Event Type | Description | Reference |
|-------|-----------|-------------|-----------|
| 0x00 | EVENT_BORN | Packet created at Shield (birth) | [RFC-UNHEADED] |
| 0x01 | EVENT_COMPUTED | Shim executed, Monad updated | [RFC-UNHEADED] |
| 0x02 | EVENT_WOTAN_RD | Wotan memory read | [RFC-UNHEADED] |
| 0x03 | EVENT_WOTAN_WR | Wotan memory write | [RFC-UNHEADED] |
| 0x04 | EVENT_CHAOS | Chaos mode applied | [RFC-UNHEADED] |
| 0x05 | EVENT_ROLLBACK | Monad rolled back | [RFC-UNHEADED] |
| 0x06 | EVENT_DIED | Packet reached Shield (death) | [RFC-UNHEADED] |
| 0x07 | EVENT_KEY_OP | PQC key lifecycle event | [RFC-UNHEADED] |
| 0x08 | EVENT_ANOMALY | Integrity or version error | [RFC-UNHEADED] |
| 0x09-0xEF | UNASSIGNED | Reserved for future use | [RFC-UNHEADED] |
| 0xF0-0xFE | EXPERIMENTAL | Experimental use | [RFC-UNHEADED] |
| 0xFF | RESERVED | Invalid | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-255 (8-bit event type identifier)
- Values 0x00-0x08: Packet lifecycle and state change events
- Values 0x09-0xEF: Reserved for future event types
- Values 0xF0-0xFE: Experimental event types
- Value 0xFF: Invalid (reserved)

**Reserved Ranges**:
- 0x00-0x08: Core packet lifecycle events
- 0x09-0xEF: Future event types (Standards Action required)
- 0xF0-0xFE: Experimental events (no registration required)
- 0xFF: Invalid (reserved)

**Change Control**: IETF (Specification Required for 0x09-0xEF)

**Expert Review Guidance**:
- Events must represent distinct, well-defined temporal occurrences in packet processing
- Events must be relevant to Anamnesis observability and packet tracing
- Events must not duplicate existing event types
- Specification must define when event occurs and required context
- Events should be applicable to packet state transitions

**Required Information for Registration**:
- Event type name and code
- Semantic definition and when event occurs during packet processing
- Required context fields (timestamp, packet identifiers, etc.)
- Temporal properties and causality with other events
- Example scenarios showing event generation
- Relationship to other event types

**Reference**: [RFC-UNHEADED] Section 8

---

### Registry 10: Wotan Error Code Registry

**Name**: Wotan Protocol Error Codes

**Type**: Integer (0-65535)

**Purpose**: Defines error codes in Wotan network protocol messages.

**Registration Procedure**: Expert Review

**Initial Values**:
| Code | Name | Description | Category | Reference |
|------|------|-------------|----------|-----------|
| 0 | SUCCESS | No error | Informational | [RFC-UNHEADED] |
| 1 | INVALID_FORMAT | Message format invalid | Syntax | [RFC-UNHEADED] |
| 2 | CHECKSUM_MISMATCH | Checksum validation failed | Integrity | [RFC-UNHEADED] |
| 3 | VERSION_MISMATCH | Protocol version incompatible | Compatibility | [RFC-UNHEADED] |
| 4 | UNKNOWN_OPTION | Unrecognized option field | Syntax | [RFC-UNHEADED] |
| 5 | UNSUPPORTED_ACTION | Action not supported | Capability | [RFC-UNHEADED] |
| 6 | RESOURCE_EXHAUSTED | Out of memory or buffers | Resource | [RFC-UNHEADED] |
| 7 | TIMEOUT | Operation timeout | Temporal | [RFC-UNHEADED] |
| 8 | DESTINATION_UNREACHABLE | No route to destination | Routing | [RFC-UNHEADED] |
| 9 | AUTHENTICATION_FAILED | Authentication check failed | Security | [RFC-UNHEADED] |
| 10 | AUTHORIZATION_DENIED | Access permission denied | Security | [RFC-UNHEADED] |
| 11 | CIRCUIT_RESET | Virtual circuit reset | Connection | [RFC-UNHEADED] |
| 12 | PROTOCOL_ERROR | Generic protocol violation | Semantic | [RFC-UNHEADED] |
| 13 | NOT_IMPLEMENTED | Feature not implemented | Capability | [RFC-UNHEADED] |
| 14 | CONGESTION | Network congestion detected | Network | [RFC-UNHEADED] |
| 15 | PROCESSING_ERROR | Internal processing error | Internal | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-65535 (16-bit error code field)
- Codes 0-15: Standard error codes (assigned above)
- Codes 16-32767: Reserved for future standard errors
- Codes 32768-65534: Vendor/implementation-specific errors
- Code 65535: Reserved (invalid)

**Namespace Semantics**:
- Code 0: Success (no error condition)
- Codes 1-15: Standard errors defined by protocol
- Codes 16-32767: Future errors requiring RFC specification
- Codes 32768-65534: Implementation-defined errors (no IANA registration)
- Code 65535: Reserved for invalid/unspecified error

**Error Categories**:
- **Syntax**: Message format or parsing errors
- **Integrity**: Checksum, validation, or data integrity failures
- **Compatibility**: Version or feature compatibility issues
- **Capability**: Unsupported features or options
- **Resource**: Resource limitations (memory, buffers, bandwidth)
- **Temporal**: Timeout or timing-related issues
- **Routing**: Network reachability and routing failures
- **Security**: Authentication and authorization failures
- **Connection**: Virtual circuit or connection state issues
- **Semantic**: Protocol logic or semantic violations
- **Network**: Network-level issues (congestion, etc.)
- **Internal**: Implementation-level errors

**Reserved Ranges**:
- 0-15: Standard errors (assigned)
- 16-32767: Future standard errors (Specification Required + Expert Review)
- 32768-65534: Implementation-specific (no registration required)
- 65535: Invalid (reserved)

**Change Control**: IETF (Expert Review for 16-32767)

**Expert Review Guidance**:
- Error codes must represent distinct, unambiguous error conditions
- Codes must not duplicate semantics of existing errors
- Specification must define recovery/remediation actions
- Specification must clarify whether error is fatal or recoverable
- Specification must define error message format and content
- Errors should be applicable across implementations

**Required Information for Registration**:
- Error code number and symbolic name
- Detailed description of error condition
- When error is generated and by which entities
- Applicable Wotan versions
- Recovery/remediation guidance
- Whether error is fatal or recoverable
- Related error codes
- Example scenarios

**Reference**: [RFC-UNHEADED]

---

## 8. IANA Section Template

Use this template to write the IANA Considerations section in your I-D or RFC:

```markdown
## IANA Considerations

### Overview
[Brief statement of what IANA actions this document requires]

### Creating New Registries

#### [Registry Name 1]

Name: [Official name]

Type: [Bitfield|Integer|String|Enumeration]

Purpose: [What this registry is used for]

Registration Procedure: [Policy from Section 4]

Initial Values:
[Table or list of initial values with name, description, and reference]

Namespace Description:
[Detailed description of the namespace, valid ranges, allocation strategies]

Value Specification:
[Define exactly how values are formatted and used]

Reserved Ranges:
[What ranges are reserved for what purposes]

Change Control: [IETF | Organization | Private Use | Other]

Reference: [This RFC Section X.Y]

---

#### [Registry Name 2]

[Repeat for additional registries]

### Registering Into Existing Registries

#### [Existing Registry Name 1]

Registry: [Exact name from IANA registry list]

Value(s):
  - [Code/Value]: [Name] - [Description]
  - [Code/Value]: [Name] - [Description]

Change Controller: IETF

Reference: [This RFC Section X.Y]

---

#### [Existing Registry Name 2]

[Repeat for additional registry entries]

### Designated Experts

[If applicable, describe:]

The following individuals are designated as experts for [registry name(s)]:

- [Name] ([email])
- [Name] ([email])
- [Name] ([email])

Experts SHALL review registrations for [registry name] according to the
following guidelines:

1. [Guideline 1]
2. [Guideline 2]
3. [Guideline 3]

Experts have 2 weeks to review proposals. They SHALL document their
decisions and provide feedback to the proposer and IANA.

### TBA References

This document uses the following placeholders:

- TBA1: [Description of what TBA1 represents]
- TBA2: [Description of what TBA2 represents]
- TBA3: [Description of what TBA3 represents]

Upon publication, IANA SHALL replace these placeholders with assigned values.
```

### Guidance for Using the Template

1. **Create New Registries First**: Present new registries before modifications to existing ones

2. **Use Tables for Clarity**: Format initial values in clear tables with columns for value, name, and reference

3. **Be Complete**: Every required field from Section 2 must be addressed

4. **Reference Clearly**: Every initial value must reference a specific section of the document

5. **Define Procedures**: State explicitly which RFC 8126 policy applies to each registry

6. **Explain Gaps**: If you don't use all values in a numeric range, explain what uses the remaining values

7. **Describe Experts**: If using Expert Review, include the expert selection and review guidelines

8. **Use Consistent Format**: Follow the same format for all registries in your document

9. **Check Completeness**: Before publication, verify that:
   - All registries are fully defined
   - All values are assigned or reserved
   - All TBA references are listed
   - All expert guidelines are clear
   - All change control designations are specified

---

## References

- RFC 8126: Guidelines for Writing an IANA Considerations Section in RFCs
- RFC 6895: Domain Name System (DNS) IANA Considerations
- RFC 6960: Online Certificate Status Protocol - OCSP
- RFC 5226: Assigning Temporary and Permanent Port Numbers

---

## Appendix: Common IANA Mistakes to Avoid

1. **Forgetting TBA Replacement Instructions**: Always tell IANA what each TBA represents

2. **Incomplete Registry Definitions**: Don't leave gaps in namespace descriptions

3. **Ambiguous Policies**: Be explicit about which RFC 8126 policy applies to each registry

4. **Missing Expert Guidelines**: If using Expert Review, provide clear review criteria

5. **Inconsistent Formatting**: Use consistent tables and terminology throughout

6. **Vague Value Descriptions**: Define each initial value's purpose and use clearly

7. **Unmaintainable Namespaces**: Design spaces that can grow cleanly and logically

8. **Missing References**: Reference specific RFC sections for all initial values

9. **Unclear Change Control**: Don't assume who controls future registrations; state it explicitly

10. **Ignoring Backward Compatibility**: Consider how new registrations might affect existing implementations

---

## Document Version History

- Version 1.0 (2026-02-20): Initial comprehensive guide including RFC 8126 policies and Unheaded protocol registries. Applied critical and high-severity fixes: version field to 8-bit, flags bitfield aligned with Foundation-03, Flow Action 0x13/0x14 corrected to KEM_ENCAPS/KEM_DECAPS with KEY_ACK/KEY_REJECT at 0x15/0x16, and Anamnesis Event Type Registry aligned to Foundation-normative definitions.
