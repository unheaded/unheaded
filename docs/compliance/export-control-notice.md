<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Export control notice (EAR, ECCN 5D002)

**Status:** notification drafted 2026-10-27, **not sent** (escalation E11 of
`docs/battle-plans/S-CLEAN-SLATE-battle-plan.md`). Stevie sends it from his own
address; the sent date is recorded below.

| Field | Value |
|---|---|
| Notification sent | not yet |
| Sent by | (Stevie Bellis, from his own address) |
| Reviewed by (Barrister) | pending |

## What this repository publishes

`github.com/unheaded/unheaded` is public. Its source uses and configures
encryption, which makes it encryption software under ECCN 5D002 of the U.S.
Export Administration Regulations (EAR):

- TLS 1.3 with hybrid post-quantum key exchange (X25519 + ML-KEM-768,
  FIPS 203): Go `crypto/tls` profiles in `pkg/tls/`, and rustls on the
  aws-lc-rs provider in `crates/zhend/` and the WAF.
- ML-DSA-65 (FIPS 204) signatures: Wotan topic signing, Gungnir sealed
  payloads, evidence seals, via `github.com/cloudflare/circl`.
- SLH-DSA (FIPS 205) signatures via `github.com/cloudflare/circl`.
- Secret envelope encryption with XChaCha20-Poly1305 and age-compatible
  encryption (`pkg/secrets/encryption/`, via `golang.org/x/crypto`).
- WireGuard pre-shared key rotation tooling (`scripts/wireguard/`).

The full, machine-checked list is the crypto inventory
`compliance/crypto/cbom.cdx.json` (`cmd/crypto-inventory`). The algorithms
come from published libraries (Go `crypto/*` and `golang.org/x/crypto`,
`cloudflare/circl`, rustls/aws-lc-rs); no cipher of the project's own design
is known in the tree. The Monad `ENCRYPT` flag marks a payload carried over
TLS and is not a cipher.

## Basis

Publicly available encryption source code is released under the EAR's
publicly-available provisions: 15 CFR 734.3(b)(3) and 734.7, with the
notification of 15 CFR 742.15(b), the provision License Exception TSU
(15 CFR 740.13(e)) points to. The notification is a one-time email per
repository URL, sent no later than the time of publication; the repository
is already public, so it is late, and sending it now is the remedy.

**For the Barrister review** (a drafting note, to be checked against the current CFR text): since the 2021 encryption rule (86 FR 16482,
29 March 2021), the 742.15(b) email is required only for publicly available
source code that implements "non-standard cryptography". Everything listed
above is standard (XChaCha20 is the one to confirm: its extended nonce is
specified in an IRTF draft, not an RFC), so the email may not be strictly
required; sending it is the conservative path and costs nothing. Confirm
before or after sending.
`docs/legal/PQC-DEPENDENCY-LICENSING-ASSESSMENT.md` Part E cites EAR 740.9 and
740.17 for this; neither is the right provision (740.9 covers temporary
exports, 740.17 is License Exception ENC). Part E needs correcting to the
citations above.

Publication under these provisions does not authorise export to
comprehensively embargoed destinations or to parties on the OFAC SDN list or
the BIS Entity List, and the project does not knowingly provide software or
services to them.

## The email (template; Stevie sends)

- **To:** `crypt@bis.doc.gov`, `enc@nsa.gov`
- **From:** Stevie's own address
- **Subject:** `Publicly available encryption source code notification (15 CFR 742.15(b))`

```
To: crypt@bis.doc.gov, enc@nsa.gov
Subject: Publicly available encryption source code notification (15 CFR 742.15(b))

This is a notification under 15 CFR 742.15(b) (License Exception TSU,
15 CFR 740.13(e)) for publicly available encryption source code
classified under ECCN 5D002.

Project:     Unheaded (configuration management automation platform)
Source URL:  https://github.com/unheaded/unheaded
Cryptography: TLS 1.3 with X25519 + ML-KEM-768 hybrid key exchange
             (FIPS 203); ML-DSA-65 (FIPS 204) and SLH-DSA (FIPS 205)
             signatures; XChaCha20-Poly1305 envelope encryption.
             Published algorithms from published libraries: Go crypto/tls
             and golang.org/x/crypto, cloudflare/circl, rustls with
             aws-lc-rs.

The source code is publicly available at the URL above at no charge.
This notification covers subsequent updates published at the same URL.

Contact: Stevie Bellis, <address the email is sent from>
```

## After sending

1. Fill in "Notification sent" above with the date (and keep the sent email).
2. Coordinator: plan Step 218, the findings ledger row for CG-21 / DL-06, and
   the ITAR/EAR control matrix's headline gap 1.
