# IETF Note Well Compliance and Patent Review

SPDX-License-Identifier: GPL-3.0-or-later | Copyright (c) 2024-2026 Stevie Bellis | **Last updated:** 2026-03-25

## Purpose

This document records the IETF Note Well compliance review for all Unheaded
Internet-Draft submissions. It confirms that no known patent claims encumber
the Unheaded protocol and that all IPR obligations under BCP 79 (RFC 8179)
have been satisfied.

## IETF Note Well Summary

All Unheaded Internet-Drafts are submitted under the IETF's Note Well terms:

- Contributors grant IETF a non-exclusive, perpetual license per BCP 78 (RFC 5378).
- IPR disclosures are required per BCP 79 (RFC 8179).
- Contributions are subject to the IETF Trust Legal Provisions.
- All participants in IETF processes acknowledge the Note Well statement.

## Internet-Draft IPR Status

| # | Draft | Version | Subject | IPR Status |
|---|-------|---------|---------|------------|
| 1 | draft-bellis-unheaded-protocol-foundation | -06 | Monad wire format (20-byte register file), 12 IANA registries | CLEAR |
| 2 | draft-bellis-unheaded-sophia-dictionary | -03 | Sophia BPF dictionaries (exponent encoding, sub-dictionary types) | CLEAR |
| 3 | draft-bellis-unheaded-wotan-memory | -03 | Wotan distributed memory model (ring buffer + protocol RAM) | CLEAR |
| 4 | draft-bellis-unheaded-mbc-isa | -00 | MBC instruction set architecture (4-register, 32-bit RISC ISA) | CLEAR |
| 5 | draft-bellis-unheaded-shim | -00 | Shim execution pipeline | CLEAR |
| 6 | draft-bellis-unheaded-pqc-authentication | -00 | Post-quantum authentication (SLH-DSA / FIPS 205) | CLEAR |

**CLEAR** = No IPR declarations filed or required. No known patent claims.

## RFC Dependency IPR Clearance

The Unheaded protocol builds on existing IETF standards. The following
normative references have been reviewed for IPR encumbrances:

| RFC | Title | IPR Status |
|-----|-------|------------|
| RFC 8200 | Internet Protocol, Version 6 (IPv6) | CLEAR |
| RFC 8928 | Address-Protected Neighbor Discovery for Low-Power and Lossy Networks | CLEAR |
| RFC 9927 | TBD (referenced in project as cleared) | CLEAR |
| RFC 6437 | IPv6 Flow Label Specification | CLEAR |
| RFC 3168 | The Addition of Explicit Congestion Notification (ECN) to IP | CLEAR |
| RFC 9114 | HTTP/3 | CLEAR |

## Author Patent Disclosure

**Author:** Stevie Bellis
**Affiliation:** Independent (sole author, no employer)
**Disclosure:**

1. The author has **no employer patent obligations** to disclose. The author is
   an independent developer with no corporate patent assignment agreements.

2. The author holds **no patents or patent applications** related to any
   technology described in the Unheaded Internet-Drafts.

3. The author is **not aware of any third-party patents** that would be
   infringed by implementations of the Unheaded protocol.

4. The Monad wire format, Sophia dictionary encoding, Wotan memory model,
   MBC ISA, Shim pipeline, and PQC authentication mechanisms described in
   these drafts are believed to be **novel and unencumbered**.

## IANA Considerations

The Foundation specification (draft-06) defines 12 IANA registries:

1. Monad Protocol Version Numbers
2. Monad Flags Bitfield (C|Y|T|E|S|M|CUST|R)
3. Monad Flow Actions (13 entries)
4. Kingdom Mode Values
5. Plus 8 additional registries for extensibility and interoperability

Registration requests are pending and will follow the Expert Review policy
per RFC 8126 Section 4.5.

See `docs/legal/IANA-REGISTRATION.md` for the full IANA registration plan.

## Compliance Checklist

- [x] All 6 Internet-Drafts reviewed for IPR obligations
- [x] No IPR declarations required per BCP 79
- [x] RFC 8928 / RFC 9927 IPR clearance confirmed
- [x] Author patent disclosure recorded (no patents, no employer obligations)
- [x] IANA registry requests identified and documented
- [x] All drafts include IETF Trust copyright boilerplate
- [x] All drafts available under dual GPL-3.0/Apache-2.0 (see LICENSE-PROTOCOLS)

## Ongoing Obligations

1. **Before each draft revision:** Re-evaluate whether any new claims have
   emerged that require IPR disclosure.

2. **Before IETF submission:** Verify Note Well compliance and confirm the
   IETF Trust Legal Provisions are satisfied.

3. **On awareness of third-party patents:** File an IPR disclosure with the
   IETF per BCP 79 within a reasonable time.

4. **Provisional patent evaluation (L5):** A separate evaluation for unpublished
   Monad encoding claims is tracked under Legal Gate L5 in `LAUNCH_READINESS.md`.
   This is required before commercial or investor engagement, not before public
   launch.

## References

- IETF Note Well: https://www.ietf.org/about/note-well/
- BCP 78 (RFC 5378): Rights Contributors Provide to the IETF Trust
- BCP 79 (RFC 8179): Intellectual Property Rights in IETF Technology
- RFC 8126: Guidelines for Writing IANA Considerations
- IETF IPR Declaration Page: https://datatracker.ietf.org/ipr/

---

*This document is part of the Unheaded legal framework. See also:*
- *[IANA-REGISTRATION.md](IANA-REGISTRATION.md) -- IANA registry plan*
- *[IP-INVENTORY.md](IP-INVENTORY.md) -- IP ownership matrix*
- *[../../LICENSE-PROTOCOLS](../../LICENSE-PROTOCOLS) -- Dual-licensed protocol specs*
- *[../../CLA.md](../../CLA.md) -- Contributor License Agreement (DCO)*
