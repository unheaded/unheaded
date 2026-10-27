<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-103: Service auth: who holds which key, and how tokens reach callers

**Status:** Proposed (design only; Stevie, 2026-10-22: "Yes, draft the ADR"). Nothing here is built yet.
**Date:** 2026-10-23
**Related:** finding `gate-script:scripts/compliance/check-service-auth.sh` (register step 7, high), ADR-083 (zero user data access / NIST 800-207 wording), ADR-097 (compliance crosswalk), `pkg/auth`, `pkg/uids` (one uid per service), `httputil.SameOriginOrAllowed` (WebSocket origin check, 2026-10-22).

## Context

Every first-party service wraps its mux in `auth.WrapHandler(mux, auth.SetupMiddleware(auth.LoadServiceAuthConfig(name)))`.
With `AUTH_ENABLED` unset, which is true in every deployment today, that is
no auth at all: anyone on the LAN can call every write API (finding above,
high). Stevie settled the modes on 2026-09-30: several (SSO/OIDC, JWT, API
key), selectable per deployment. What is still open is **where the keys and
the identity provider live, and how a caller gets a token**. This ADR answers that.

What `pkg/auth` does when enabled, and why it cannot simply be turned on:

- **One shared HMAC key for everything.** User JWTs and service tokens are
  both HS256 with `AUTH_JWT_KEY`. Every service that can verify a token can
  also mint one: any service compromised, or any process that reads one
  service's environment, can forge an admin user token and a token for any
  other service.
- **Keys in environment variables.** `AUTH_JWT_KEY` and `AUTH_API_KEYS` end up in
  `docker inspect`, `/proc/<pid>/environ` and crash dumps.
- **No issuer.** Nothing mints user tokens, and no service knows how to fetch
  a token for another service. Turning auth on today breaks every caller.
- `/health`, `/ready` and `/metrics` stay unauthenticated (`SkipAuthPaths`),
  which Prometheus and Akira rely on. That stays.

Callers to serve:

| caller | calls | today |
|---|---|---|
| browser (dashboard, kanban, wiki) | dashboard-backend, kanban-app, wiki-server, WebSockets | nothing |
| operator CLI (`unheaded-cli`), scripts, CI smoke tests | daemon (Cuirass), protocol-api, wotan | nothing |
| service → service (dashboard-backend → wotan, services → wotan, zhen-agentd → services) | HTTP APIs, Wotan gRPC | nothing |
| Prometheus, Akira | `/metrics`, `/health`, `/ready` | unauthenticated, stays so |

## Decision

### 1. Asymmetric keys only; the shared HMAC key retires

Tokens are JWTs signed with **Ed25519 (EdDSA)**. Verifiers hold only public
keys, so being able to verify a token no longer lets a service mint one.
`AUTH_JWT_KEY` (HS256) remains only in tests. ML-DSA-65 is the post-quantum
candidate (already used for Wotan `config.*` topic signing, ADR-043), but
its 3.3 KB signature in every request header is a cost to measure first.
See open question 3.

### 2. Service identities: one key pair per service, file-held

- Each service has its own Ed25519 key pair, created at install by a new
  `unheaded-keygen <service>`. The private key is a file,
  `/etc/unheaded/keys/<service>.key`, mode 0400, owned by that service's
  own uid (`pkg/uids`). For compose it is a Docker secret mounted into that
  one container. **Never an environment variable, never in the repo**
  (CLAUDE.md, Secrets Management).
- Public keys go in a trust directory, `/etc/unheaded/trust.d/<service>.pub`,
  readable by all. Public keys are not secrets. They are still per-deployment
  and generated at install, so they don't belong in the repo either.
- A caller mints its own short-lived token (5 minutes, `aud` = target service,
  `sub` = caller, `kid` = key id) for each target and caches it until shortly
  before expiry. No token service sits in the request path, so there is no
  single point of failure.
- A verifier accepts a service token only if its `kid` is in trust.d **and**
  the caller is on that service's allowlist (`auth.allow_callers` in its
  config). Being in trust.d proves who the caller is; the allowlist says
  whether it may call this service.
- **Rotation:** generate a new key with a new `kid`, add its public half to
  trust.d, switch the service to the new key, and remove the old public half
  after one token lifetime. Removing a `.pub` file revokes that key.

### 3. Humans: OIDC as the interface, a small built-in issuer as the default

- Services verify user tokens against a **JWKS** (public keys), fetched from
  the configured issuer's discovery URL and cached. Any OIDC provider plugs in
  this way (Authentik, Keycloak, Dex, a cloud IdP), so it can be chosen per
  deployment without code changes.
- For labs with no IdP, Unheaded ships a minimal first-party issuer: login
  with a password (argon2id hash) and optional WebAuthn, mints EdDSA access
  tokens (15 minutes), and serves its own JWKS. It runs as its own nologin
  user and is the only place user signing keys live. Where it runs is open
  question 1.
- Roles come from a claim (`roles`). The existing `RBACAuthorizer` maps them
  to methods and paths, e.g. `viewer` for GET only and `operator` for Cuirass
  writes.

### 4. Browsers never hold bearer tokens

The dashboard, kanban and wiki use the backend-for-frontend pattern. Their
backend runs the OIDC login, keeps the tokens server-side, and gives the
browser only a session cookie (`HttpOnly; Secure; SameSite=Strict`). An XSS
bug can then use the session while the page is open, but cannot carry a token
away. WebSockets authenticate by that cookie on upgrade, after the Origin
check that already exists. The backend calls other services with its own
service token, passing the user along as a separate signed claim
(`act`/`on_behalf_of`). That keeps the record of who asked for what without
forwarding the user's token.

### 5. API keys for automation

The CLI, scripts and CI use API keys: random 32 bytes with an `uhk_` prefix
so leaks are greppable, shown once at creation, and stored only as a SHA-256
hash with role, owner, created and expiry dates (Well `unheaded_config`, or a
file for hosts without the Well). gitleaks gets a rule for the prefix. Keys
expire within 90 days by default.

### 6. Wotan

Wotan's gRPC clients present a service token in call metadata, with
`aud=wotan`. Topic ACLs (who may publish to and subscribe to which topics)
build on the caller identity this gives, replacing today's `auto_approve`
display-name list. That is a later step, not part of this ADR's first cut.

### 7. Rollout: one service at a time, as the finding's plan says

1. `pkg/auth`: EdDSA sign and verify, trust.d loading, JWKS client, caller
   allowlist; HS256 kept for tests only. Unit-tested and fuzzed, with no
   service changes yet.
2. `unheaded-keygen` and the install steps (compose secrets, deb postinst)
   that create keys with the right owner and mode.
3. Turn auth on **for Cuirass first**: its writes are already refused without
   `AUTH_ENABLED` (2026-10-23), and the CLI is its only writer.
4. Then zhen-agentd, protocol-api, kanban-app, dashboard-backend (BFF login),
   wiki-server, then the rest. Each is its own commit, with a test that an
   unauthenticated write gets 401 and a correctly-tokened one 200.
5. Wotan last: every service talks to it, so it goes after all callers can
   present tokens.
6. `check-service-auth.sh` checks the result on each host: services with
   auth on, and no `AUTH_JWT_KEY` in any unit or compose environment.

## Consequences

- One compromised service can't forge users or other services. It can only
  call what its own allowlists permit.
- No shared secret to distribute. Install generates one private key per
  service, plus the issuer's keys.
- A new moving part, the issuer. It is off the service-to-service path, so
  if it's down, logins fail but running services keep talking.
- More setup per deployment (keygen, trust.d). `unheaded-keygen` and the
  packaging carry it, so no operator copies keys by hand.
- Compliance: this gives evidence for 800-53 IA-2/IA-3/IA-5, AC-3/AC-6 and
  SC-8/SC-23 (with TLS), and SOC 2 CC6.1. The crosswalk entries are added
  when step 6 can fail.

## Open questions for Stevie

1. **Where the built-in issuer runs:** its own service (`unheaded-auth`, new
   port) [recommended: a separate uid and a small attack surface], or inside
   dashboard-backend (one less service, but the dashboard then holds user
   signing keys).
2. **Default IdP for the lab:** the built-in issuer [recommended], or an
   existing one such as Authentik or Keycloak (big, but full SSO/MFA).
3. **Post-quantum service tokens:** start with Ed25519 and switch to ML-DSA-65
   after measuring [recommended], or ML-DSA-65 from day one.
4. **API-key store:** the Well [recommended where it runs], files elsewhere?
   Decided (S-IRON-GATE Step 10, the recommendation): the Well where it
   runs, a 0600 file elsewhere, same record format. Built in Phase 3; see
   "API keys as built" below.

## API keys as built (S-IRON-GATE Phase 3)

`pkg/auth` (`apikey_store.go`): `NewAPIKey()` returns `uhk_` + 43 base64url
characters (32 random bytes). `APIKeyStore` is one interface, `Create`,
`Authenticate`, `Revoke` (by the 12-character prefix or the id), `List`, with
the rules in one place (expiry required and at most 365 days, roles admin,
operator, observer, service, revoked and expired keys refused, 401) over three
backends:

- `NewWellAPIKeyStore`: `unheaded_app.api_keys` (migration 036; the role
  `app_auth`, migration 035). §5 said `unheaded_config`; the table went into
  `unheaded_app` with the plan's Step 10 decision, beside the other
  application data, under its own role. Only `revoked_at` (once) and
  `last_used_at` ever change, and rows are never deleted. Services log in
  with `AUTH_WELL_USER`/`AUTH_WELL_PASSWORD` (`database.AppAuthConfig`), not
  their own Well login.
- `NewFileAPIKeyStore`: JSON lines, written 0600, refused when group- or
  world-writable or world-readable, re-read when it changes, writers locked
  on `<path>.lock`. Default `/etc/unheaded/auth/apikeys.jsonl`.
- `NewMemAPIKeyStore`: tests.

A service accepts keys when `AUTH_APIKEY_STORE` is `well` or `file:<path>`
(`X-API-Key`, or `Authorization: Bearer uhk_...`); the identity is the key's
owner with its role. The plain `AUTH_API_KEYS`/`AUTH_API_KEY_FILE` list still
works for one release and logs a deprecation warning. `unheaded apikey
create|list|revoke` manages keys; gitleaks rule `unheaded-api-key`
(`.gitleaks.toml`) catches a leaked key. Proof: `scripts/well/test-api-keys.sh
--with-go` (CI job well-schema).

## Browser login as built (S-IRON-GATE Phase 6)

`pkg/bff`, used by dashboard-backend and kanban-app (each its own BFF: the
cookie is per origin). `AUTH_ISSUER_URL` names the issuer; `POST
/auth/login` forwards the password to the issuer's `/login` with `audience`
= the service's own name (the issuer's `-audiences` lists it), verifies the
returned token with `JWKSVerifier`, keeps it in a session store keyed by
SHA-256 of the cookie, and sets `unheaded_session` (`HttpOnly; Secure;
SameSite=Strict; Path=/`). Every request with the cookie re-verifies the
stored token. CSRF: login, logout and cookie-authenticated writes need
`X-Requested-With: unheaded` as well. `/auth/auth.js` is the one page
script both backends serve. Sessions are in memory (`bff.Store` for a Well
store later).

Two deviations from the text above. The login is unheaded-auth's password
`/login`, not an OIDC code flow, so another IdP needs that flow added
first. Until internal TLS (S-IRON-GATE Phase 8) the compose stack runs
`AUTH_BFF_DEV_PLAINTEXT=true`: no `Secure` on the cookie, and the JWKS
fetched over plain http (`auth.WithPlainHTTPIssuer`), which is no weaker
than the plaintext login it checks.

`act`: `ServiceTokenMinter.Mint(target, auth.WithActor(sub))` signs the
user into the service token as RFC 8693 `{"act":{"sub":...}}`; the
receiver's identity carries it as `Extra["act"]` for audit only.
`bff.AuthorizeOutgoing` sets it and `X-Unheaded-On-Behalf-Of` from a
session-authenticated request. No dashboard route calls another service
for a user yet.

## Measurements (S-IRON-GATE Step 63)

Measured 2026-10-27 on west (AMD Ryzen 5 7600X, Go 1.27.3,
`cloudflare/circl` `mldsa65`), `go test -bench -benchtime=1s -count=5`,
with three other agents' test runs sharing the host. Medians:

| | Ed25519 (EdDSA) | ML-DSA-65 |
|---|---|---|
| verify | 34.1 µs/op | 11.7 µs/op |
| sign | 14.6 µs/op | 59-226 µs/op (rejection sampling varies) |
| signature | 64 B (86 B base64url) | 3,309 B (4,412 B base64url) |
| service token (`sub=dashboard-backend`, `aud=wotan`, roles) | 308 B | ~4,640 B |
| `Authorization: Bearer …` header | 330 B | ~4,660 B |

Verification cost does not argue against ML-DSA-65: on this CPU it
verifies about three times faster than Ed25519. Signing happens once per
target every 4.5 minutes (the minter's cache), so its cost does not matter
either. The cost is size: about 4.3 KB more on every request, over half
of one nginx default `large_client_header_buffers` buffer (8 KB), and
every proxy hop's header limit would have to be checked before the flip.

**Switch rule.** Flip service tokens to ML-DSA-65 when policy requires
post-quantum signatures on internal requests, or when no hop on the path
(haproxy, nginx sidecars, gRPC metadata) limits a single header below
8 KB and the extra bytes per request are acceptable on the busiest link
(Wotan gRPC). Today `alg` is the constant `AlgEdDSA` in
`pkg/auth/jwt_eddsa.go`; the flip is a second signer/verifier pair keyed
by `alg` (`ML-DSA-65`), with trust.d `.pub` files carrying the key type,
and the verifier still accepting exactly one `alg` per key so the
algorithm-confusion refusal holds.
