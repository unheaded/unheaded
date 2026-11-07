<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-136: BIND zones and a locked-down certificate authority for lab names

**Status:** Proposed (parked). No code until Stevie reads this. BIND is third party, so it needs Stevie's confirmation before anything lands.
**Date:** 2026-11-06
**Related:** DP-702, DP-06, ADR-103, ADR-105, FND-017, the 2026-10-29 rule on third-party software.

## The pitch (Stevie, 2026-11-06)

BIND with proper zone files and domains, self-signing through a locked-down certificate authority, with YubiKey custody, maybe protected with a rotated certificate. It replaces the plan to issue chaos-controller a certificate at S-E1-LIVE: that certificate waits on this ADR.

## What is there

- **Names.** No authoritative DNS server is in the repo: no zone files and no `named.conf`. Services are reached by address or compose service name.
- **Certificates.** Three separate sources. `scripts/gen-dev-certs.sh` makes a development CA and one certificate per compose service. `runbooks/pki/generate-kingdom-certs.yaml` is the hosts' path. `scripts/well/make-tls.sh` makes the Well's private CA and server certificate.
- **Trust.** Go clients trust the system roots plus the deployment CA (`UNHEADED_TLS_CA_CERT`); `pkg/transport/mtls` and Wotan are TLS 1.3 hybrid-only once TLS is on (FND-017).
- **Edge.** DP-702 chose between an internal CA and ACME for the edge.
- **Custody.** DP-06 sets the custody rules for keys. YubiKey custody is the direction taken here.

## Decision direction

1. **Zones.** BIND serves authoritative zones for the lab's own domains, from zone files kept under change control, so a certificate names a host by a real name.
2. **Certificate authority.** One locked-down CA signs for those names. Its root key is held on a YubiKey and is used only in an attended ceremony. Nothing on a host can sign with it.
3. **Rotation.** The working certificates are short-lived and rotated. Whether a rotated certificate also protects the CA's own path is open, below.
4. **chaos-controller** gets its certificate from this CA, naming 127.0.0.1 as well as its host name, once the CA exists.

## What this needs before any code

- **Stevie's confirmation for BIND.** It is a new third-party package: no install, image or module until he says yes, and it then takes a row in the third-party inventory, its own nologin account and a pinned version.
- A written ceremony for the root key, using the custody rules of DP-06.

## Open questions

1. Which domains, and is any zone published outside the LAN? The repo is public, so what a committed zone file may hold is part of this answer.
2. Does the CA replace the Kingdom CA and the Well's private CA, or sit beside them?
3. What does "protected with a rotated certificate" guard: an intermediate between the root and the services, or access to the signing host?
4. Where does BIND run, and what happens to Pi-hole's DNS role on the LAN?
5. Revocation: short lifetimes alone, or a published list as well?

## What this does not do

It changes no host and no resolver. It does not close FND-017, and it does not settle the edge's public certificate.
