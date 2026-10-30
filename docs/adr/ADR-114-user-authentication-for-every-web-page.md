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

## Addendum (Stevie, 2026-10-30): one login applies to every application

> auth to one application should apply to all - research best practice for acomplishing this and roll into ADR for future brainstorming

### What current best practice says (sources below, retrieved 2026-10-30)

1. **Browser apps keep tokens on a backend.**
   - RFC 10017, *OAuth 2.0 for Browser-Based Applications* (BCP 212, August 2026; it was draft-ietf-oauth-browser-based-apps), puts the Backend-for-Frontend pattern first. The browser holds only a session cookie, and the backend is a confidential OAuth client.
   - `pkg/bff` already does this.
2. **Use the authorization code flow with PKCE S256 everywhere, even for confidential clients.**
   - RFC 9700, *OAuth 2.0 Security Best Current Practice* (January 2025), deprecates the implicit grant and `plain` PKCE.
   - Refresh tokens must be sender-constrained or rotated, and a reused refresh token revokes its whole family.
3. **Single sign-on comes from the identity provider's session, not a shared cookie.**
   - The user authenticates once at the IdP, and the IdP's own first-party cookie remembers that.
   - Each application is an OIDC relying party with **its own** session. When it has none, it sends the browser to the IdP with a **top-level redirect**. The IdP sees its session and returns a code at once, with no prompt.
   - The older iframe tricks (`prompt=none` in a hidden iframe, OIDC Session Management's check-session iframe) depend on third-party cookies. Browsers now block those, so they should not be relied on.
4. **Sessions are independent, and logout has to be propagated deliberately.**
   - NIST SP 800-63B-4 §5.2: "the termination of the subscriber's sessions at an IdP and RP are independent of each other", and "the RP SHALL be authoritative as to whether the reauthentication requirements have been met."
   - The OpenID standards for logout: RP-Initiated Logout (the user logs out at one app, which sends them to the IdP's end-session endpoint) and **Back-Channel Logout** (the IdP calls each RP's logout URI server-to-server with a signed logout token).
   - Back-channel is the reliable path. Front-Channel Logout runs in hidden iframes, needs the RP's cookies, and fails under third-party-cookie blocking, so it is best-effort at most.
5. **Lifetimes (NIST SP 800-63B-4, AAL section, final).**

   | Level | Overall reauthentication | Inactivity |
   |---|---|---|
   | AAL1 | SHOULD ≤ 30 days | MAY |
   | AAL2 | SHOULD ≤ 24 h | SHOULD ≤ 1 h |
   | AAL3 | SHALL ≤ 12 h | SHOULD ≤ 15 min |

   - "When either timeout expires, the session SHALL be terminated."
   - Cookie expiry "SHALL NOT be relied upon to enforce session timeouts". The server enforces them.
   - Bearer session secrets "SHOULD NOT be persistent".
   - "Remember my browser" SHALL NOT stand in for authentication.
6. **Cookie scope.**
   - A `__Host-` cookie is Secure, `Path=/` and has no `Domain`, so it is pinned to one host, and sibling subdomains cannot set or shadow it.
   - A shared parent-domain cookie (`Domain=.lab`) is the anti-pattern for single sign-on. Every subdomain receives it and can overwrite it (cookie tossing), so one weak app compromises all of them.
   - **Cookies ignore ports** (RFC 6265 §8.5): apps on the same host but different ports share one cookie jar.

### What this means for the Kingdom: three ways to do it

| | A. IdP session + per-app sessions (OIDC single sign-on) | B. One gateway origin | C. Shared parent-domain cookie |
|---|---|---|---|
| How | unheaded-auth keeps the single sign-on session (its own first-party cookie). Every first-party page (dashboard, kanban, wiki, verdicts) and every OIDC-capable UI (Grafana, Open WebUI) is an RP with its own `__Host-` session. Missing session → top-level redirect → instant code. Logout → issuer end-session → back-channel logout to every RP. | Every page sits behind one host name and the N8 gate, routed by path (`/kanban`, `/grafana`, `/pihole`). One session cookie. | One session cookie with `Domain=` the lab's parent domain, read by every app |
| Standards fit | RFC 10017, RFC 9700, OIDC Core + Back-Channel Logout, NIST 800-63B-4 | Common, but outside the OIDC model | Contrary to them |
| Isolation between apps | Strong: one app's flaw does not hand over another app's session | Weak: all apps share one origin. Script injected into any app runs as the whole lab (an HttpOnly cookie does not stop same-origin calls) | None |
| Single logout | Back-channel to each RP (each app keeps a revocable server-side session) | Trivial (one cookie) | Trivial |
| Third-party UIs | OIDC-capable ones join directly. The rest sit behind the N8 gate, which is itself one RP. | All behind the gate. Each must work under a sub-path (several don't). | Most can't read our cookie |
| Cost | Most work: the N7 code flow, back-channel logout, a session store per RP | Least work | — |
| Claude's recommendation | **Yes** | Acceptable only as a stopgap | **No** |

### The recommendation in detail (springboard)

1. **unheaded-auth is the one place a user authenticates.**
   - It holds the single sign-on session: a `__Host-` cookie on the issuer's origin, stored in the Well, with idle and absolute limits.
   - It runs the N7 code flow (PKCE S256) and adds an end-session endpoint and back-channel logout delivery.
2. **Every app is an RP, all through one client in `pkg/bff`.**
   - The BFF in `pkg/bff` grows the RP side of the code flow, so dashboard, kanban, wiki, the verdicts page and the N8 gate share one code path.
   - Each gets its own cookie name with the `__Host-` prefix, and its own Well-backed session table, revocable by `sid`.
3. **Back-channel logout receiver** (`POST /auth/backchannel-logout`) in `pkg/bff`. It:
   - verifies the issuer-signed logout token (EdDSA; `events` claim; `sid`/`sub`; `jti` replay guard);
   - deletes the matching sessions.

   The issuer reaches the RPs over the internal TLS network, never through the browser.
4. **Lifetimes: aim for AAL2** (password + TOTP or passkey).
   - Issuer session: overall ≤ 24 h, inactivity ≤ 1 h.
   - App sessions: never outlive the issuer session. Each refresh re-checks with the issuer (a short-lived access token, renewed silently by a top-level redirect while the issuer session lives).
   - Admin actions, such as a verdict write (ADR-113), need fresh authentication (`max_age` / `auth_time` ≤ 15 min), the AAL3 inactivity rule applied to sensitive actions.
5. **Same host, different ports.** The lab serves dashboard 20000 and kanban 20001 on one host, so per-app *names* matter as much as the prefix. The longer-term fix is one host name per app through haproxy (`dashboard.lab`, `kanban.lab`, `auth.lab`), which also makes `__Host-` meaningful.
6. **Everything is logged and audited** under the ADR-113 R8 rules:
   - logins, single sign-on issuances (which RP, `sid`), logouts and each back-channel delivery with its result, session expiry, and step-up events;
   - all of it to `ops.login_events` plus Wotan `audit.auth.*`.
7. **Libraries: still zero new.**
   - The code flow, PKCE, logout tokens and session stores are the same JWS/EdDSA and stdlib `net/http` work as N2, N5 and N7.
   - This addendum changes none of N1–N11.

### Found while researching: the dashboard and kanban sessions collide today

`pkg/bff` uses one cookie name, `unheaded_session` (`DefaultCookieName`), and neither dashboard-backend nor kanban-app overrides `CookieName`. They run on the same host on ports 20000 and 20001. Cookies are not separated by port, so:
- logging in to one replaces the other's cookie, which logs the user out of the other;
- each backend receives the other's session id. That id is a bearer secret for the other app, so a compromised kanban-app could replay it against the dashboard.

ADR-103's "the cookie is per origin" does not hold for ports.

- **Small fix:** a distinct `__Host-` name per app, e.g. `__Host-unheaded_dashboard`, `__Host-unheaded_kanban`. The compose stack is down, so this is not live exposure. Under ADR-112 it waits for Stevie's word.

### Sources (retrieved 2026-10-30)

- [RFC 10017 / BCP 212: OAuth 2.0 for Browser-Based Applications](https://www.rfc-editor.org/info/rfc10017) ([datatracker](https://datatracker.ietf.org/doc/draft-ietf-oauth-browser-based-apps/))
- [RFC 9700: OAuth 2.0 Security Best Current Practice](https://datatracker.ietf.org/doc/rfc9700/)
- [NIST SP 800-63B-4, Authenticator Assurance Levels](https://pages.nist.gov/800-63-4/sp800-63b/aal) and [Session Management](https://pages.nist.gov/800-63-4/sp800-63b/session)
- [OpenID Connect Back-Channel Logout 1.0](https://openid.net/specs/openid-connect-backchannel-1_0.xml)
- [OWASP OAuth 2.0 Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/OAuth2_Cheat_Sheet.html)
- Cookie prefixes and port scope: RFC 6265 §8.5 (ports), [cookie attributes reference](https://specification.website/spec/security/cookie-attributes.md)

## Addendum 2 (Stevie, 2026-10-30): how Google, Netflix and Microsoft do it

> check FAANG methods for doing this, netflix, google, etc if you can find any write ups or blogs

Public write-ups found (retrieved 2026-10-30). Meta, Amazon and Apple have published little on internal web single sign-on, and none turned up.

| Who | What they published | The pattern |
|---|---|---|
| **Google: BeyondCorp Access Proxy** (Spear, Cittadini, Saltonstall, *;login:* 2016; with *Design to Deployment*, Osborn et al. 2016) | Every employee HTTP service moved behind one Access Proxy, "the only mechanism that allowed employees to access internal HTTP services". It shares one ACL evaluator (Access Control Engine) with the SSH gateway. "A common logging point for all requests allowed us to perform forensic analysis more effectively." Access depends on user **and device** state, not network location. | Central enforcement point, every request authorized, one log |
| **Google Cloud IAP** (product docs: signed headers) | The proxy authenticates the user, then sends the app a signed JWT in `x-goog-iap-jwt-assertion`: ES256; `aud` = that one backend; `iss`, `exp`/`iat` with 30 s skew; `sub` as the stable user id. Apps must verify it because the signed header gives "secondary security in case someone bypasses IAP", e.g. a disabled proxy or a misconfigured firewall. | Proxy holds the session; apps get a short, per-app, signed identity assertion and verify it |
| **Google: Device Bound Session Credentials** (Google blog, 2026-04-09; W3C WebAppSec) | Session cookies bound to a key in the TPM or Secure Enclave. The server issues **short-lived** cookies only after the browser proves it holds that key, so a stolen cookie expires quickly. Public in Chrome 146 on Windows; macOS next. An open W3C design, with Microsoft. A site adds a registration and a refresh endpoint. | Cookie-theft resistance without a library |
| **Netflix: edge authentication and "Passport"** (Casella, Nelson, Singh, Netflix Tech Blog 2021; QCon talk "User & Device Identity for Microservices @ Netflix Scale") | Authentication moved to the edge (Zuul) and centralized services owned by one team. The edge turns whatever token the client sent into **Passport**, an integrity-protected, token-agnostic identity object carrying user and device information. Downstream services consume Passport and never see client tokens. Stated benefits: simpler services, token handling done by specialists, better auditability. (The blog itself returned 403; this is from its abstract and the talk and slide summaries.) | One internal identity format, minted at the edge |
| **Microsoft: Continuous Access Evaluation** (Entra; OpenID **CAEP** on the Shared Signals Framework, carried as Security Event Tokens, RFC 8417) | Sessions are revoked in near real time on events: account disabled or deleted, password change, location change, user-risk increase. CAEP defines a `session-revoked` event. With CAE, access tokens can live longer because revocation no longer waits for expiry. | Push revocation events instead of waiting for timeouts |
| **BuzzFeed `sso`** (open source, BuzzFeed Infrastructure) | An auth proxy in front of internal services using "double OAuth2": the proxy is an OAuth client of BuzzFeed's own `sso-auth`, which is a client of Google. | Proxy as the RP of a central IdP |

### What the big operators converge on, mapped to the Kingdom

1. **One enforcement front end that holds the session** (BeyondCorp AP, IAP, Netflix edge, BuzzFeed proxy). The Kingdom equivalent is the N8 gate, extended to front *every* page, ours included. This is option B's single enforcement point without B's single origin: each app can keep its own host name and its own `__Host-` cookie, issued by the gate.
2. **One central identity provider behind that front end** (option A): unheaded-auth. The gate is one RP, so single sign-on and single logout happen once, at the gate.
3. **Apps receive a signed, short-lived, per-app identity assertion, never the user's cookie or token**: an "Unheaded passport" in Netflix and IAP terms.
   - The Kingdom already has the machinery: EdDSA JWTs with `aud`, verified by `pkg/auth`, and the RFC 8693 `act` claim (ADR-103).
   - The passport would be minted by the gate per request (or cached per `sid` for under a minute). Claims: `sub`, `roles`, `sid`, `auth_time`, `amr` (password / totp / passkey), `aud` = that app.
   - Like IAP, every app still verifies it, so a bypassed or misconfigured gate fails closed.
4. **Every request authorized, one log** (BeyondCorp). The gate is the natural single audit point for ADR-113 R8's verbose debug trail, and the apps' own logs carry `sid`/`sub` from the passport.
5. **Revocation is pushed, not polled** (CAE/CAEP).
   - The Kingdom already has signed Wotan topics. `audit.auth.session-revoked` (shaped like a CAEP event) drops sessions at the gate within seconds of logout, user disable, password reset or MFA reset.
   - OIDC back-channel logout stays the path for OIDC-capable third-party UIs.
6. **Bind the session to the device** (DBSC, BeyondCorp device trust): later, and optional.
   - Short-lived gate cookies plus DBSC refresh where the browser supports it (ES256 proof checking is stdlib).
   - For `admin`, optionally require a client certificate from an enrolled device. The LXD API already works this way.

### Revised recommendation (springboard)

**Option D = A + B: central IdP + one gate in front of every page + per-app signed passport + pushed revocation.** It is what Google's IAP and Netflix's edge do, at Kingdom scale:
- **Fewer moving parts per app:** each first-party service only verifies a passport, which `pkg/auth` already knows how to do, instead of each running a full OIDC RP. A BFF stays only where an app needs server-side tokens of its own.
- **One place for single sign-on and logout:** login, MFA, step-up, revocation and audit all happen in one front end and one issuer.
- **Same isolation:** no shared cookie, per-app host names, and per-app `aud`.
- **Still zero new libraries:** reverse proxy (stdlib), JWT (first-party), Wotan events (exists).
- **The cost:** the gate becomes a critical component and needs high availability (a gate on each of west and east, sessions in the Well). Everything depends on the firewall making the gate the only path, which is ADR-098 step 6 again.

### Sources (addendum 2)

- [BeyondCorp: The Access Proxy (Google Research)](https://research.google/pubs/beyondcorp-the-access-proxy/)
- [BeyondCorp: Design to Deployment at Google (USENIX ;login:)](https://www.usenix.net/system/files/login/articles/login_spring16_06_osborn.pdf)
- [Google Cloud IAP: securing your app with signed headers](https://docs.cloud.google.com/iap/docs/signed-headers-howto)
- [Google: protecting cookies with Device Bound Session Credentials](https://blog.google/security/protecting-cookies-with-device-bound-session-credentials/)
- [Android Authority: DBSC generally available](https://androidauthority.com/chrome-browser-dbsc-3672363/)
- Netflix Tech Blog: Edge Authentication and Token-Agnostic Identity Propagation (netflixtechblog.com/edge-authentication-and-token-agnostic-identity-propagation-514e47e0b602, 403 to the fetcher). Summaries: [QCon SF: User & Device Identity for Microservices @ Netflix Scale](https://archive.qconsf.com/node/150487), [Netflix AIM slides](https://www.slideshare.net/slideshow/netflix-aim-engineering-manager/200530305)
- [OpenID CAEP 1.0](https://openid.net/specs/openid-caep-1_0-03.html)
- [Microsoft: Continuous Access Evaluation GA](https://techcommunity.microsoft.com/blog/identity/continuous-access-evaluation-in-azure-ad-is-now-generally-available/2464398)
- [BuzzFeed sso (GitHub)](https://github.com/buzzfeed/sso)

## Open questions for the brainstorm

0. **Single sign-on model: D (central IdP + one gate in front of every page + signed per-app passport + pushed revocation; Claude's revised recommendation), A (IdP session + a full OIDC RP in every app), B (one gateway origin), or a staged path B → D?** Also: one host name per app through haproxy; AAL2 as the lab default; DBSC and device certificates for `admin` later?
1. **N4: native-minimal WebAuthn, the `go-webauthn` library, or TOTP only for now?** This is the main library decision.
2. Keep argon2id from `x/crypto` (N1), or move to stdlib PBKDF2 to shed the module? Claude recommends keeping it. x/crypto also supplies other things we use, so shedding it would not drop the module anyway.
3. Should the issuer grow an OIDC code flow (N7), or is Grafana's own login acceptable behind the gate (N8)? The second means two passwords for Grafana.
4. MFA required for every user, or only for `admin` (cross-cutting 5)?
5. Session lifetimes (idle 30 min / absolute 12 h are suggestions).
6. Does the gate (N8) belong in `unheaded-auth`, the dashboard BFF, or its own small service?
7. Should the Zhen Flask UI be retired rather than gated (N10)?

## Decision

None yet. Parked under ADR-112.
