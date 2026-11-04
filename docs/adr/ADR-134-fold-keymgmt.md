<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-134: Fold keymgmt into unheaded-auth as a registry, never a key generator

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, DP-204, DP-06, ADR-103, ADR-119 open question 6, TRUE-WIRE C7, L5.

## The pitch (decision pack, DP-203)

> ... keymgmt into unheaded-auth.

## What is there

`services/keymgmt` plus `handler`, `lifecycle`, `store`: HTTP on 19007, no auth. **`GenerateKey` generates no key.** It allocates a 16-bit KeyRef and stores a record whose `PublicKeyHex` stays empty, then logs "PQC key generated". `store/sophia_store.go` is an in-memory stand-in. The useful part is the lifecycle: 60-second grace, revocation cascading to SigRefs, KeyRef overflow handled.

## Verdict: right for the registry, wrong for generation

unheaded-auth already publishes public keys (JWKS) and has rotation semantics, so a registry of public keys and KeyRefs fits it. Generation does not:

1. **Custody.** DP-06: daemon keys live on hosts under dedicated accounts; `unheaded-keygen` makes them. Series rule 5 keeps key generation for custody attended. The login-facing issuer must never hold another service's private key.
2. **Blast radius.** The issuer is the most exposed identity service; a key-minting API on it is the wrong place.
3. **No BPF.** `deploy/systemd/unheaded-auth.service` has an empty `CapabilityBoundingSet`, so it can never write Sophia's PQC maps.

## Decision direction

Flag `--feature-keyreg`, default off: `/api/v1/keyreg/` behind admin RBAC, taking **public** keys from `unheaded-keygen` output and assigning KeyRefs. Rename `GenerateKey` to `RegisterKey` so the log stops lying. ADR-119 can then use it or stay on `trust.d`: it works either way.

## Open questions

1. Does anything need KeyRefs before ADR-119 is built? If not, this fold can wait with it.
2. Revocation list: Wotan signed topic, or a file in `trust.d`?
