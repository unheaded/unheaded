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
  [TBA1] - CHAOS flag
  [TBA2] - CANARY flag
  [TBA3] - TRACED flag
  ...

IANA SHALL replace TBA1 with the bit assigned to the CHAOS flag,
TBA2 with the CANARY flag, and so forth.
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

The normative list is Section 11 of draft-bellis-unheaded-protocol-foundation-06. It creates 13 registries in the "Unheaded Protocol Parameters" group and requests two code points in existing IANA registries. The table below summarizes it; if the two ever disagree, the draft wins.

### New registries

| # | Registry | Policy | Field / range | Initial values |
|---|----------|--------|---------------|----------------|
| 1 | Monad Protocol Version Numbers | Standards Action | Monad octet 0x00, 8 bits | 0x00 reserved; 0x01 = v1; 0x02-0xEF unassigned; 0xF0-0xFE experimental; 0xFF reserved |
| 2 | Monad Flags | Specification Required | Monad octet 0x07, 8 bits | 7 CHAOS, 6 CANARY, 5 TRACED, 4 ENCRYPT, 3 SAMPLED, 2 MIRROR, 1 CUSTOM, 0 RSVD (all assigned) |
| 3 | Monad Flow Actions | Expert Review | Monad octet 0x05 | 0x00 FORWARD, 0x01 TRACE, 0x02 SAMPLE, 0x03 DROP, 0x04 MIRROR, 0x05 RATE_LIMIT, 0x06 REDIRECT, 0x07 INJECT; 0x10-0x14 PQC key ops (KEY_ROTATE, KEY_REVOKE, KEY_DISTRIBUTE, KEY_VERIFY, KEY_CHALLENGE); 0xF0-0xFE experimental; 0xFF reserved |
| 4 | Kingdom Mode Values | Standards Action | 2 bits | 0b00 NORMAL, 0b01 PRIORITY, 0b10 EXPERIMENTAL, 0b11 RESERVED |
| 5 | Sophia Dictionary Namespace | First Come First Served | Dictionary names | Template: name, program, version, organization, contact |
| 6 | Anamnesis Event Types | Specification Required | Event type byte | 0x00 BORN ... 0x08 ANOMALY (9 types) |
| 7 | Error Codes | Specification Required | Error code byte | 0x00-0x0C normative, 0x0D reserved, 0x0E-0x1E unallocated, 0x1F-0xFF private use / greasing ([error-registry.md](../error-registry.md)) |
| 8 | TLV Type Allocations | Specification Required | TLV type byte; bit 7 = critical | 0x00-0x1F Monad, 0x20-0x3F Sophia, 0x40-0x5F Wotan, 0x60-0x7F future; 0x01 Ring Path Counter, 0x2A UNHEADED_METRIC_V1 |
| 9 | PQC Algorithm Identifiers | Specification Required | Algorithm id byte | 0x01 ML-KEM-768, 0x02 ML-KEM-1024, 0x03 ML-DSA-65, 0x04 ML-DSA-87, 0x05 SLH-DSA-SHA2-128s |
| 10 | UPC MBC Opcode Numbers | Specification Required | Bits [31:24] of a 32-bit MBC word | The MBC ISA opcodes |
| 11 | UPC MBC Syscall Numbers | Expert Review | SYSCALL imm16 | Native 0x01-0x0F; Linux-compatible via INT 0x80 |
| 12 | UPC Memory Region Types | Specification Required | 32-bit UPC address space | RAM, KBD_IO, SCREEN, DEBUG, WAD, HEAP, STACK, RAMDISK |
| 13 | UPC Event Types | Specification Required | Anamnesis event byte, compute range | 0x10 COMPUTE_HOP ... 0x19 CONTEXT_SWITCH; 0x1A-0x1F unassigned |

### Code points in existing IANA registries

| Registry | Request | Notes |
|----------|---------|-------|
| IPv6 Destination and Hop-by-Hop Options | Unheaded Monad Option, type TBD (suggested 0x3E) | Format 001xxxxx: skip if unrecognized, data may change en route. Change controller IESG. |
| Assigned Internet Protocol Numbers | Next Header 0xFE (254) | Reserved for future use; the normative path is the HbH option. 254 is an RFC 3692 experimentation value, so a real request needs a different number. |

The IPv6 Flow Label (Section 11.7) is used per RFC 6437 and needs no IANA action.

### Defined elsewhere

| Registry | Defined in |
|----------|-----------|
| Wotan Topic Namespace (Expert Review) | draft-bellis-unheaded-wotan-memory-03 |
| Sophia dictionary registries | draft-bellis-unheaded-sophia-dictionary-03 |

QoS class, circuit state, deployment ring and mesh flag values are not IANA registries: they are exponent-encoded fields whose meaning comes from the active Sophia dictionary.

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

## Wire Format Freeze Status (S67 — February 28, 2026)

### Monad v0x01 Wire Format FROZEN

The 20-byte Monad layout (offsets, sizes, order) is frozen at version 0x01. Registries are not frozen: each one stays open under its own policy (Section 7). The registries that cover Monad octets are full or nearly so:

- **Version** (octet 0x00): only 0x01 assigned; a new version means a new wire format.
- **Flags** (octet 0x07): all 8 bits assigned (CHAOS, CANARY, TRACED, ENCRYPT, SAMPLED, MIRROR, CUSTOM, RSVD).
- **Flow Actions** (octet 0x05): 0x00-0x07 and 0x10-0x14 assigned; 0x08-0x0F and 0x15-0xEF open under Expert Review.

Service identity, QoS, deployment ring, circuit state and mesh flags are Sophia dictionary values, not IANA registries.

### IPR & Implementation Status

- **Disclosure:** Complete, verified against IETF patent database
- **Collision Status:** Zero known conflicts
- **Implementation:** WEST bare metal system operational (Monad v0x01 live)
- **Freeze Date:** 2026-02-28 02:25:00 UTC
- **Backward Compatibility:** 20-byte Monad immutable for Age 1; Age 2 expands via HbH options

### Future Registry Expansion (Age 2+)

New registries for Age 2 (IPv6 native transport) will use Hop-by-Hop TLV option types (RFC 4727) without modifying the core 20-byte envelope. Registry conflicts with future IETF standards are mitigated by the Limited Domain design (RFC 8799 — link-local scope only).

---

## Document Version History

- Version 1.0 (2026-02-20): Initial comprehensive guide including RFC 8126 policies and Unheaded protocol registries
- Version 1.1 (2026-02-28): S67 Wire Format Freeze
- Version 1.2 (2026-10-17): Section 7 and the freeze status rewritten to match draft-06 Section 11 (13 registries + 2 code points; flags at octet 0x07 with the draft-06 meanings)
