# unheaded-auth — Built-in OIDC Issuer

`unheaded-auth` is the small issuer people log in to when no other identity provider is configured ([ADR-103](../docs/adr/ADR-103-service-auth-token-distribution.md) §3). It checks a password, mints a short-lived EdDSA access token with a `roles` claim, and publishes the public key services verify it with. It is off the service-to-service path: services authenticate each other with their own keys (`unheaded-keygen`, trust.d), so if the issuer is down, logins fail and running services keep talking.

## Running

| | |
|---|---|
| Binary | `cmd/unheaded-auth` (`scripts/build-binaries.sh`, deb `unheaded-unheaded-auth`) |
| Port | 19020 (`pkg/ports.Auth`) |
| Account | `unheaded-auth`, uid 16735 (`pkg/uids.Auth`), nologin |
| Host unit | `deploy/systemd/unheaded-auth.service` (not installed anywhere yet) |
| Compose | service `auth` on the `data` network, no host port, `-dev-plaintext` |
| Signing key | `/etc/unheaded/keys/unheaded-auth.key` (0400, `unheaded-keygen -owner unheaded-auth unheaded-auth`); compose: secret `svc_key_unheaded-auth` |
| Users file | `/etc/unheaded/auth/users.json` (0600, owned by `unheaded-auth`); compose: `configs/auth/dev-users.json` |

## Endpoints

| Endpoint | Purpose |
|---|---|
| `POST /login` | `{"username","password"[,"audience"]}` (JSON, at most 4 KiB) → `{"access_token","token_type":"Bearer","expires_in":900}` |
| `GET /jwks.json` | One `OKP`/`Ed25519` key, `kid` = first 16 hex of SHA-256 over the key (the same kid as trust.d) |
| `GET /.well-known/openid-configuration` | `issuer`, `jwks_uri`, `token_endpoint`, `response_types_supported: ["token"]` |
| `GET /health`, `GET /ready`, `GET /metrics` | Unauthenticated; the only paths answered over plain HTTP without `-dev-plaintext` |

Token: header `{"alg":"EdDSA","typ":"JWT","kid":...}`; claims `sub` (user), `iss` (the `-issuer-url`), `aud` (`unheaded`, or another value listed in `-audiences` when the login asks for it), `iat`, `nbf`, `exp` = `iat` + 900 s, `roles`.

## Behaviour worth knowing

- **Passwords** are argon2id (64 MiB, 3 passes, 2 lanes, 16-byte salt). Parameters read back from the users file are bounded, so a bad file cannot make one login allocate gigabytes. At most two hashes run at once. An unknown user costs the same as a wrong password and gets the same answer.
- **Lockout:** 5 failures for one user from one address, or 20 from one address across users, within 10 minutes answer 429 with `Retry-After` without hashing anything. There is no per-user lock across addresses, so nobody can lock another user out. The address is the TCP peer, never `X-Forwarded-For`.
- **Refuses to start** when its key or users file is readable by group or others, the users file is malformed, or the issuer URL is not https (plain http only to loopback, or anywhere with `-dev-plaintext`).
- **TLS** comes from `pkg/tls` (`UNHEADED_TLS_*`, in `/etc/unheaded/auth.env` for the unit) with the hybrid ML-KEM Strict profile ([[Post-Quantum Key Exchange|Post-Quantum-Key-Exchange]]). Without TLS and without `-dev-plaintext` only the probes answer.
- **Users:** `unheaded-auth user add <name> --roles operator` (prompts twice; `--password-stdin` for scripts), `user list`, `user del <name>`. Roles: `admin`, `operator`, `viewer`, `observer`, `guest` (`service` is refused: it is what service tokens carry). The file is replaced atomically and keeps its owner. Reload with `SIGHUP` (`systemctl kill -s HUP unheaded-auth`, `docker compose kill -s HUP auth`); a broken file keeps the users already loaded. Removing a user does not revoke tokens already issued; they expire within 15 minutes.
- **Dev stack:** `scripts/gen-dev-keys.sh` makes the key and one operator `dev` with a random password printed once; `--reset-users` makes a new one. Run it once with `sudo` (after `make binaries`) so uid 16735 can read both files.
- No WebAuthn yet (S-IRON-GATE Step 125: it needs a new module and a browser ceremony).

## How a service verifies

Set `AUTH_OIDC_ISSUER_URL` to the issuer URL. `pkg/auth.JWKSVerifier` fetches discovery and the JWKS, caches the keys for 10 minutes, refetches once for an unknown kid, and checks `alg` = `EdDSA`, `iss`, `exp` and `aud` = the service's own name. A token for a service therefore needs that service in `-audiences` and the login's `audience` field; in practice the dashboard backend (the BFF, ADR-103 §4) logs users in with `audience: dashboard-backend` and calls other services with its own service token.

The verifier fetches over https, or plain http to loopback only. The compose `auth` service runs plain HTTP on the data network, so no other container can verify its tokens until TLS is on every internal link (S-IRON-GATE Phase 8).

## Replacing it with another IdP

Any OIDC provider that signs with Ed25519 (EdDSA) and publishes discovery and a JWKS plugs in the same way: point `AUTH_OIDC_ISSUER_URL` at it and put the role names in a `roles` claim. Authentik or Keycloak would be new third-party services (own nologin account, pinned image, ADR-004 review).

---

> **Source:** [cmd/unheaded-auth/](../cmd/unheaded-auth/) · [ADR-103](../docs/adr/ADR-103-service-auth-token-distribution.md) · [S-IRON-GATE Phase 4](../docs/battle-plans/S-IRON-GATE-battle-plan.md)
