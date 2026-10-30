<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-114: User authentication for every web page, and which outside libraries (if any) it may use

**Status:** Proposed (pitch, parked under ADR-112). Brainstorm before acceptance; nothing is built.
**Date:** 2026-10-30
**Grounded on:** `churn/2026-10-30` at `49eb2fca`
**Related:**
- ADR-103 (service auth, the built-in issuer, the BFF)
- ADR-006 (vanilla JS)
- ADR-096 (dependency gate)
- ADR-105 (hybrid ML-KEM TLS)
- ADR-086 (Muninn, `ops.login_events`)
- ADR-113 (verdicts service, which needs this)
- third-party policy of 2026-10-29

## The pitch (Stevie, 2026-10-30, verbatim)

> another ADR for adding user authentication to any web pages - carefully plan this, mainly the desicion of any outside libraries to use

## Context

### What already exists (ADR-103, S-IRON-GATE)

**`cmd/unheaded-auth` (19020) is the built-in issuer.**
- Password login, with argon2id from `golang.org/x/crypto/argon2`.
- Lockout before hashing (`lockout.go`), so locked attempts cost nothing.
- A bound on concurrent hashes (64 MiB each).
- EdDSA access tokens that last 15 minutes, and its own JWKS.
- A host CLI for users (`usercmd.go`).
- JWT signing and verification, JWKS fetch and RBAC are all **first-party code**. `pkg/auth` imports no JWT, OIDC or session library.

**`pkg/bff` is the browser side, used by dashboard-backend and kanban-app.**
- The browser holds only an `unheaded_session` cookie: `HttpOnly; Secure; SameSite=Strict`.
- Tokens stay server-side and are re-verified on every request.
- CSRF protection is SameSite plus `X-Requested-With: unheaded` on writes.
- One shared `/auth/auth.js`.
- Sessions live **in memory**, so a restart logs everyone out. A Well store is planned.

**Gaps ADR-103 left open:**
- Login is the issuer's password `/login`, not an OIDC authorization-code flow. So no third-party UI can use it for single sign-on.
- WebAuthn is mentioned as "optional" and is not built. There is no second factor at all.
- `ops.login_events` (ADR-086) is not in the migrations yet.

### Every web page on west and east today

| Page | Whose | Today | Port / path |
|---|---|---|---|
| Dashboard | ours | BFF login | 20000 |
| Kanban | ours | BFF login | 20001 |
| Wiki (`wiki-server`) | ours | reads open (`OpenReads: "/"`), writes need a token, no login page | 20002 |
| UPC console (`dashboard/upc-console.html` + `upc-tty-bridge`) | ours | loopback only, no login | 26100 |
| Zhen web UI (`raft/zhen_app.py`, Flask) | ours (Python) | **no login**; CSRF interim only (threat model T8) | 20103 |
| Third-party verdicts page (ADR-113) | ours, future | must require login | TBD |
| Grafana | third party | its own admin password (env, fail-loud) | compose |
| Pi-hole admin | third party | its own password, **plain HTTP on the LAN** through haproxy | 80/443 |
| Open WebUI | third party | its own accounts | 20106 |
| Prometheus / VictoriaMetrics / Alertmanager / ClickHouse UIs | third party | none of their own; reach depends on binding | compose |
| haproxy stats | third party | its own config | 21404 / 21405 |
| LXD API / UI | third party | client certificate | 8443 |

The compose stack is down (FND-001), so most rows are about config, not live exposure. The host firewall is still open (`firewall-inbound-deny`).

### The library question

The 2026-10-29 policy: no new third-party library, tool, application or code without discussion and Stevie's confirmation. Native first. `golang.org/x/crypto` (Go team) is already a direct dependency. The Go toolchain is 1.27, and its standard library now covers more of this than it used to:
- `crypto/pbkdf2`, `crypto/hkdf`, `crypto/sha3` and `crypto/mlkem` (1.24);
- `crypto/ecdsa` and `crypto/ed25519` for P-256 and EdDSA signatures;
- `net/http.CrossOriginProtection` (1.25), CSRF protection from `Sec-Fetch-Site`/`Origin`;
- `crypto/hmac` and `crypto/subtle`.

## Brainstorm: the library decision, need by need (Claude's recommendations, nothing decided)

**Summary:** the recommendation adds **zero** new third-party modules. The one real fork in the road is WebAuthn/passkeys (N4).

| # | Need | Native option | Outside option(s) | Recommendation |
|---|---|---|---|---|
| N1 | Password hashing | stdlib `crypto/pbkdf2` (SHA-256, ~600k iterations) | `golang.org/x/crypto/argon2` (**already in tree**) | **Keep argon2id from x/crypto.** It is memory-hard where PBKDF2 is not, already built and tuned, and the module is maintained by the Go team. |
| N2 | Tokens (JWT/JWS), JWKS | already first-party | `golang-jwt/jwt`, `go-jose` | **Stay native.** Reject both: we already verify only EdDSA with fixed claims, and a general JWT library adds algorithm-confusion surface we don't need. |
| N3 | TOTP second factor (RFC 6238/4226) | ~60 lines: `crypto/hmac` + `crypto/sha1`, constant-time compare, ±1 step window, replay guard (last used step per user) | `pquerna/otp` | **Native**, tested against the RFC 6238 Appendix B vectors. Recovery codes are random, argon2id-hashed and single use. QR enrolment: show the `otpauth://` URI and the base32 secret as text. Skip the QR image unless we write a QR encoder; a QR library would be third-party. |
| N4 | **WebAuthn / passkeys** | Minimal native relying party: `none` attestation only; ES256 (`crypto/ecdsa`) and EdDSA (`crypto/ed25519`); a small CBOR/COSE-key decoder for the attested credential data; rpIdHash, challenge, origin, user-present/verified flags; signature-counter checks. Fuzzed, plus a Lich campaign. | `go-webauthn/webauthn` (the de facto Go library; BSD-3). It brings several transitive modules (a CBOR library, a JWT library for metadata, others). Measure the exact set with `go mod graph` in a scratch module before deciding. | **Open: Stevie decides.** Native gives zero dependencies and a narrow scope, but it is security-critical protocol code we would own; it is mitigated by W3C/FIDO test vectors, fuzzing and a deliberately small feature set. The library is broadly used and covers attestation formats and metadata we may never need, at the cost of a supply-chain footprint. Claude leans **native-minimal**, *if* the scope stays at "none" attestation and two algorithms. Or ship TOTP first (N3) and decide N4 later. |
| N5 | Sessions | `pkg/bff` (exists) + `crypto/rand` | `gorilla/sessions`, `alexedwards/scs` | **Native.** Move the session store to the Well (hashed ids, idle 30 min / absolute 12 h, rotate on login and privilege change, revoke-all per user). Reject both libraries. |
| N6 | CSRF | stdlib `http.CrossOriginProtection` + the existing `X-Requested-With` + SameSite=Strict | `gorilla/csrf` | **Native.** Add `CrossOriginProtection` to `pkg/bff` as a second layer. Reject `gorilla/csrf`. |
| N7 | OIDC provider (SSO for third-party UIs: Grafana `generic_oauth`, Open WebUI) | Extend unheaded-auth: discovery, `/authorize` (code flow + **PKCE S256 only**), `/token`, `/userinfo`. Confidential clients registered from config. No implicit or hybrid flow, no dynamic registration. | `zitadel/oidc`, `ory/fosite`; or a whole IdP (Dex, Authentik, Keycloak: ADR-103 open question 2, "big") | **Native, minimal profile.** Our own pages keep the direct BFF login. The code flow exists only for third-party UIs that can speak OIDC. Rejecting the libraries keeps the issuer small. Rejecting a whole IdP keeps one auth component. |
| N8 | Pages that cannot do OIDC (Pi-hole, haproxy stats, Prometheus/VM/Alertmanager/ClickHouse UIs, the UPC console) | A first-party **auth gate**: `net/http/httputil.ReverseProxy` behind `pkg/bff`. Each UI binds to loopback or a container network only, so the gate is the only path. The gate strips client auth headers and injects nothing the UI trusts. | `oauth2-proxy`, Authelia, Pomerium; or haproxy basic auth | **Native gate.** Reject the proxies (third party), and reject basic auth (no MFA, credentials on every request). Pi-hole's own password stays as a second factor behind the gate. |
| N9 | Browser code | vanilla JS (ADR-006): `fetch`, `navigator.credentials.create/get` for passkeys | SimpleWebAuthn browser library, any CDN | **Native.** No CDN (the inventory already lists four CDN assets as a problem). |
| N10 | Python Zhen UI | put it behind the N8 gate, or retire it in favour of a Go page | Flask-Login, Authlib | **Gate it now; retirement is a separate question.** Reject both libraries. |
| N11 | Rate limiting / lockout | `lockout.go` exists; `golang.org/x/time/rate` is already direct (and slated for removal: S-LOW-FRUIT watch list) | none | **Native.** Extend lockout to the second factor, per user and per source address. |

### Cross-cutting recommendations

1. **TLS before any login page.** No password or session over plaintext beyond loopback. The compose stack's `AUTH_BFF_DEV_PLAINTEXT` mode ends when internal TLS lands (S-IRON-GATE Phase 8, ADR-105). The plain-HTTP Pi-hole admin page is the live example of the problem.
2. **One login, many pages.** A session at the gate or BFF covers every page on that origin. Cross-origin pages use the issuer's code flow (N7), so the user logs in once at the issuer.
3. **Roles.** `viewer` / `operator` / `admin` already exist in RBAC. Map each page and each write to a role in one table (`docs/policy/page-roles.md` or code). Verdict writes (ADR-113) are admin only.
4. **First admin.** Created on the host with the existing `unheaded-auth` user CLI. Never through a web page, never with a default password.
5. **MFA policy.** Required for `admin`, optional for others, at first. Decide whether to require it for everyone once passkeys or TOTP exist.
6. **Audit (the logging rules of ADR-113 R8 apply).**
   - Every login, failure, lockout, MFA enrolment or change, session revoke and role change goes to `ops.login_events`, which this work creates. Also an `info` log and a signed Wotan `audit.auth.*` event, always.
   - Debug adds per-request session checks.
   - Never logged: passwords, codes, cookies, tokens, WebAuthn challenges.
7. **Recovery.** Lost second factor: host CLI only (`unheaded-auth user reset-mfa`). No email, so no SMTP account to steal.
8. **Testing.**
   - RFC 6238 vectors (N3).
   - WebAuthn: W3C/FIDO test vectors plus fuzzing of the CBOR/COSE parser (N4), and a Lich campaign: hostile authenticator data, replayed assertions, counter rollback.
   - OIDC: PKCE downgrade and redirect-URI tests (N7).
   - Gate: header-smuggling and path-normalization tests (N8).
9. **Rollout order** (once unfrozen):
   - TLS everywhere;
   - Well-backed sessions + `CrossOriginProtection`;
   - the gate in front of Pi-hole and the metrics UIs;
   - TOTP;
   - the OIDC code flow for Grafana;
   - passkeys (after the N4 decision);
   - the Zhen UI behind the gate.

## Open questions for the brainstorm

1. **N4: native-minimal WebAuthn, the `go-webauthn` library, or TOTP only for now?** This is the main library decision.
2. Keep argon2id from `x/crypto` (N1), or move to stdlib PBKDF2 to shed the module? Claude recommends keeping it. x/crypto also supplies other things we use, so shedding it would not drop the module anyway.
3. Should the issuer grow an OIDC code flow (N7), or is Grafana's own login acceptable behind the gate (N8)? The second means two passwords for Grafana.
4. MFA required for every user, or only for `admin` (cross-cutting 5)?
5. Session lifetimes (idle 30 min / absolute 12 h are suggestions).
6. Does the gate (N8) belong in `unheaded-auth`, the dashboard BFF, or its own small service?
7. Should the Zhen Flask UI be retired rather than gated (N10)?

## Decision

None yet. Parked under ADR-112.
