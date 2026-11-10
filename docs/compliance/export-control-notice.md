<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Export control notice (EAR, ECCN 5D002)

**Status:** notification drafted 2026-10-27, corrected 2026-11-10 after the
skill review below, **not sent** (escalation E11 of
`docs/battle-plans/S-CLEAN-SLATE-battle-plan.md`). Stevie sends it from his own
address; the sent date is recorded below.

| Field | Value |
|---|---|
| Notification sent | not yet |
| Sent by | (Stevie Bellis, from his own address) |
| Reviewed by (Barrister) | 2026-11-10, skill pass, not a lawyer; note in this file |

## What this repository publishes

`github.com/unheaded/unheaded` is public. Its source uses and configures
encryption, which makes it encryption software under ECCN 5D002 of the U.S.
Export Administration Regulations (EAR):

- TLS 1.3 with hybrid post-quantum key exchange (X25519 + ML-KEM-768,
  FIPS 203): Go `crypto/tls` profiles in `pkg/tls/`, and rustls on the
  aws-lc-rs provider in `crates/zhend/`. `cmd/ebpf-collector` reaches rustls
  (ring provider) through its HTTP client.
- ML-DSA-65 (FIPS 204) signatures: Wotan topic signing, Gungnir sealed
  payloads, evidence seals, via `github.com/cloudflare/circl`.
- SLH-DSA (FIPS 205) signatures via `github.com/cloudflare/circl`.
- Secret envelope encryption with XChaCha20-Poly1305, and an age-style
  recipient format (X25519, HKDF, ChaCha20-Poly1305) written in the tree on
  `golang.org/x/crypto` primitives (`pkg/secrets/encryption/`).
- WireGuard pre-shared key rotation tooling (`scripts/wireguard/`); it drives
  the system's WireGuard and implements no cryptography.

The full, machine-checked list is the crypto inventory
`compliance/crypto/cbom.cdx.json` (`cmd/crypto-inventory`). The algorithms
come from published libraries (Go `crypto/*` and `golang.org/x/crypto`,
`cloudflare/circl`, rustls with aws-lc-rs or ring); no cipher of the project's
own design is known in the tree. The Monad `ENCRYPT` flag marks a payload
carried over TLS and is not a cipher.

## Basis

Text read on the eCFR, title 15 current to 2026-11-06.

- 15 CFR 742.15(b)(1): publicly available encryption source code classified
  under ECCN 5D002 is not subject to the EAR, "subject to the notification
  requirements of paragraph (b)(2)". What counts as publicly available is in
  15 CFR 734.3(b)(3) and 734.7.
- 15 CFR 742.15(b)(2): the notification is required for source code that
  "provides or performs 'non-standard cryptography'". It is an email to BIS
  and to the ENC Encryption Request Coordinator giving the internet location
  of the code. A new one is needed when the location changes, not when the
  code at that location changes.
- 15 CFR 772.1, "non-standard cryptography": an implementation "involving the
  incorporation or use of proprietary or unpublished cryptographic
  functionality, including encryption algorithms or protocols that have not
  been adopted or approved by a duly recognized international standards body
  (e.g., IEEE, IETF, ISO, ITU, ETSI, 3GPP, TIA, and GSMA) and have not
  otherwise been published".
- License Exception TSU no longer carries this: 15 CFR 740.13(e) reads
  "[Reserved]". The notification moved to 742.15(b) in September 2016. BIS:
  notices made under 740.13 before then remain valid.

Publication under these provisions does not authorise export to
comprehensively embargoed destinations or to parties on the OFAC SDN list or
the BIS Entity List, and the project does not knowingly provide software or
services to them.

## Review note, 2026-11-10 (Barrister skill; not legal advice)

This is the reading of an AI skill, not of a lawyer, and no lawyer has read
this file.

1. **Is the email required?** For standard cryptography, no. Since the rule of
   29 March 2021 (86 FR 16482) paragraph (b)(2) asks for it only where the
   code provides or performs non-standard cryptography. Everything in the list
   above is a published algorithm from a published library. BIS's own web
   page ("Encryption items NOT subject to the EAR") still words it as "once
   the email notification per section 742.15(b) is sent", without the
   non-standard limit; the regulation's text governs.
2. **XChaCha20.** Its extended nonce comes from an IRTF draft
   (draft-irtf-cfrg-xchacha) that never became an RFC, so no standards body
   has adopted it. The definition's test has two limbs joined by "and": not
   adopted, and "not otherwise published". XChaCha20 is published (the draft,
   libsodium, `golang.org/x/crypto`). By the text it is not non-standard.
3. **The project's own formats.** The envelope and age-style formats in
   `pkg/secrets/encryption/` and the protocol in `ietf-submission/` combine
   standard primitives in the project's own way. No standards body has
   adopted them; they are published, in this repository and as individual
   Internet-Drafts (the -00 versions, since expired). Whether a format published only by its author is
   "otherwise published" is the one point the text does not settle. This is
   the reason to send the email anyway.
4. **Addresses.** `crypt@bis.doc.gov` and `enc@nsa.gov` are the two named in
   742.15(b)(2) as of 2026-11-06.
5. **Not late.** The earlier draft said the notice was overdue. If none of the
   cryptography is non-standard, nothing was due. If item 3 goes the other
   way, the notice was due at publication and sending it is the remedy. The
   email is the same either way.
6. **Corrected here.** The draft cited License Exception TSU, 15 CFR
   740.13(e), in the basis and in the email text; that paragraph is reserved.
   The email now cites 742.15(b) alone and says the cryptography is standard
   and published.
7. **Other files.** `docs/legal/PQC-DEPENDENCY-LICENSING-ASSESSMENT.md` Part E
   and `docs/compliance/control-matrix/itar-ear-2026-05-06.md` cite 740.9,
   740.17 and 740.13(e) for this. Both are dated records and carry a
   correction note of 2026-11-10 pointing here.
8. **A second repository.** If the archived WAF (`unheaded-waf`) is published,
   it is a separate URL. Its code calls rustls and implements no cryptography
   of its own; the same reading applies, and a notice for it would name its
   own URL.

Recommendation: send it once. It costs one email and closes item 3.

## The email (template; Stevie sends)

- **To:** `crypt@bis.doc.gov`, `enc@nsa.gov`
- **From:** Stevie's own address
- **Subject:** `Publicly available encryption source code notification (15 CFR 742.15(b))`

```
To: crypt@bis.doc.gov, enc@nsa.gov
Subject: Publicly available encryption source code notification (15 CFR 742.15(b))

This is a notification under 15 CFR 742.15(b) of publicly available
encryption source code classified under ECCN 5D002.

Project:     Unheaded (configuration management automation platform)
Source URL:  https://github.com/unheaded/unheaded
Cryptography: TLS 1.3 with X25519 + ML-KEM-768 hybrid key exchange
             (FIPS 203); ML-DSA-65 (FIPS 204) and SLH-DSA (FIPS 205)
             signatures; XChaCha20-Poly1305 envelope encryption.
             Published algorithms from published libraries: Go crypto/tls
             and golang.org/x/crypto, cloudflare/circl, rustls with
             aws-lc-rs.

The source code is publicly available at the URL above at no charge.
The project believes the cryptography is standard and published; this
notification is sent as a precaution. It covers subsequent updates
published at the same URL.

Contact: Stevie Bellis, <address the email is sent from>
```

## After sending

1. Fill in "Notification sent" above with the date (and keep the sent email).
2. Coordinator: plan Step 218, the findings ledger row for CG-21 / DL-06, and
   the ITAR/EAR control matrix's headline gap 1.
