# unheaded-auth — Built-in OIDC Issuer

`unheaded-auth` is the small issuer people log in to when no other identity provider is configured ([ADR-103](../docs/adr/ADR-103-service-auth-token-distribution.md) §3). It checks a password, mints a short-lived EdDSA access token with a `roles` claim, and publishes the public key services verify it with. It is off the service-to-service path: services authenticate each other with their own keys (`unheaded-keygen`, trust.d), so if the issuer is down, logins fail and running services keep talking.

## Running

| | |
|---|---|
| Binary | `cmd/unheaded-auth` (`scripts/build-binaries.sh`, deb `unheaded-unheaded-auth`) |
| Port | 19020 (`pkg/ports.Auth`) |
| Account | `unheaded-auth`, uid 16735 (`pkg/uids.Auth`), nologin |
| Host unit | `deploy/systemd/unheaded-auth.service` (not installed anywhere yet) |
| Compose | service `auth` on the `data` network, no host port, `https://auth:19020` with a dev certificate (`scripts/gen-dev-certs.sh`) |
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
- **TLS** comes from `pkg/tls` (`UNHEADED_TLS_*`, set in the unit; material under `/etc/unheaded/tls/unheaded-auth/`) with the hybrid ML-KEM Strict profile ([[Post-Quantum Key Exchange|Post-Quantum-Key-Exchange]]). Without TLS and without `-dev-plaintext` only the probes answer.
- **Users:** `unheaded-auth user add <name> --roles operator` (prompts twice; `--password-stdin` for scripts), `user list`, `user del <name>`. Roles: `admin`, `operator`, `viewer`, `observer`, `guest` (`service` is refused: it is what service tokens carry). The file is replaced atomically and keeps its owner. Reload with `SIGHUP` (`systemctl kill -s HUP unheaded-auth`, `docker compose kill -s HUP auth`); a broken file keeps the users already loaded. Removing a user does not revoke tokens already issued; they expire within 15 minutes.
- **Dev stack:** `scripts/gen-dev-keys.sh` makes the key and one operator `dev` with a random password printed once; `--reset-users` makes a new one. Run it once with `sudo` (after `make binaries`) so uid 16735 can read both files.
- No WebAuthn yet (S-IRON-GATE Step 125: it needs a new module and a browser ceremony).

## How a service verifies

Set `AUTH_OIDC_ISSUER_URL` to the issuer URL. `pkg/auth.JWKSVerifier` fetches discovery and the JWKS, caches the keys for 10 minutes, refetches once for an unknown kid, and checks `alg` = `EdDSA`, `iss`, `exp` and `aud` = the service's own name. A token for a service therefore needs that service in `-audiences` and the login's `audience` field; the browser-facing backends log users in with their own name as `audience` (below) and call other services with their own service tokens.

The verifier fetches over https, or plain http to loopback only. With `UNHEADED_TLS_ENABLED=true` it trusts the system roots plus the deployment CA (`UNHEADED_TLS_CA_CERT`), so the compose issuer at `https://auth:19020` verifies with the dev CA (S-IRON-GATE Phase 8).

## Wotan

Wotan is the last service behind auth (S-IRON-GATE Phase 7, ADR-103 §6). With `AUTH_ENABLED=true` every HTTP route and every gRPC call needs a service token for `aud=wotan` from a caller in its `AUTH_ALLOW_CALLERS`; `/health`, `/ready`, `/metrics` and `grpc.health.v1` stay open. Topic reads need a token too: topics carry every service's logs and health reports.

| | |
|---|---|
| Client side | `pkg/wotan-client` sends the token on every HTTP request and, as per-RPC credentials, every gRPC call. Each binary names its identity once in `main()`: `wotanClient.InitServiceIdentity("<name>")` (key from `AUTH_KEY_FILE`, else `/etc/unheaded/keys/<name>.key`); every client in the process then presents it, including the ones `pkg/healthreport`, `pkg/discovery` and `pkg/logagg` build. `WithServiceToken` / `WithStreamServiceToken` give one client another identity; `AuthorizeRequest` covers hand-built requests. No key: a warning, and calls carry no token |
| Approval | `configs/wotan.yaml` `topics.auto_approve` lists service identities. With auth on, only a verified EdDSA service token's `sub` is matched; the `display_name` a client sends is a label. Members are named by the `sub`, and a publish through a `subscriber_id`/`sender_id` is refused when that member belongs to another identity |
| Auth off | The display name is matched against `auto_approve` and `legacy_display_names` (the names clients sent before Phase 7), with a warning. This keeps the Appendix A3 rollback (`AUTH_ENABLED` unset on Wotan only) working |
| Allowlist | compose `wotan` and `unheaded-wotan.service` allow every identity a binary presents; `services/wotan/cmd/wotan/allowlist_test.go` fails when a new `InitServiceIdentity` name (or a Rust client's `pub const IDENTITY`) is missing from either, or from `auto_approve` |
| Transport | With `UNHEADED_TLS_ENABLED=true` every Go client dials Wotan gRPC and HTTP over TLS 1.3 (Strict hybrid ML-KEM, the deployment CA, server name verified; no plaintext fallback). A token travels over TLS only: on a plaintext link it is withheld (a WARN once) unless the peer is loopback or the client was built `WithInsecureTransport()`, which is refused under TLS (S-IRON-GATE Phase 8). `scripts/run-runbook.py` can use an https Wotan (`UNHEADED_TLS_CA_CERT`); the Rust ebpf-collector has no TLS yet, so it cannot reach a TLS Wotan |
| Admin API | Still off unless Wotan runs with `--admin`; with auth on it needs an `admin` identity as well as the admin key |

A signed publish (`drift.*`, `config.*`) covers the sender Wotan authorized: with auth on, the caller's `sub`, so a signing publisher uses its identity as its display name (Heimdall: `heimdall`).

### Clients that are not Go

They mint the same tokens, byte for byte (Ed25519 is deterministic), and `tests/svctoken` holds them to it: Go makes a key pair at run time, the foreign minter signs, and the token must equal `auth.SignEdDSA`'s for the same claims and pass `auth.EdDSAVerifier`.

| Client | Minter | Identity and key |
|---|---|---|
| Rust `cmd/ebpf-collector` | `crates/svc-token` (ring) | `ebpf-collector`, on Wotan's allowlist and `auto_approve`; `AUTH_KEY_FILE`, else `/etc/unheaded/keys/ebpf-collector.key` (made by hand: no unit or compose service runs it). Token on every gRPC call (tonic interceptor) and HTTP request |
| Rust `cmd/trace-collector` | `crates/svc-token` | `trace-collector`, same key rule. Its publish RPC is still a simulation, and production traces come from `cmd/trace-collector-go` |
| `scripts/run-runbook.py` | `scripts/svc_token.py` (Python `cryptography`) | the key it is given: `AUTH_KEY_FILE` as the name it is filed under (a Champion run is `zhen-agentd`), else the operator's CLI key as `unheaded-cli`; `RUNBOOK_PROGRESS_IDENTITY` overrides the name. An `https://` `RUNBOOK_PROGRESS_WOTAN` is TLS 1.3 only, checked against `UNHEADED_TLS_CA_CERT` (else the system roots) |
| `cmd/demo-trace-injector` | `pkg/wotan-client` | `unheaded-cli` (an operator's demo tool) |
| `curl` | `unheaded token --aud wotan` prints one 5-minute token from the CLI's key | `unheaded-cli`: `curl -H "Authorization: Bearer $(unheaded token --aud wotan)" ...` |

A missing key is one warning and calls without a token, as for the Go clients. Still anonymous: the `curl`s in `scripts/bare-metal/validate-cross-host.sh` and `runbooks/wotan/cluster-*` (their publishes send a `sender_id` body Wotan does not read, and `cluster/promote` is the admin API); `scripts/demo-start.sh` only reads the open `/health`. None of the Rust clients speaks TLS yet (Phase 8).

## Browser login: the backend-for-frontend

Browsers never hold a bearer token ([ADR-103](../docs/adr/ADR-103-service-auth-token-distribution.md) §4). dashboard-backend (20000) and kanban-app (20001) each run `pkg/bff` (cookies are per origin, so each page's own backend logs its users in):

| | |
|---|---|
| Config | `AUTH_ENABLED=true` and `AUTH_ISSUER_URL` (compose: `http://auth:19020`; units: `https://%H:19020`). The issuer must list the service in `-audiences` (compose and unit list `unheaded,dashboard-backend,kanban-app`) |
| `POST /auth/login` | `{"username","password"}` with `X-Requested-With: unheaded`. The backend posts it to the issuer's `/login` with `audience` = its own name, verifies the token against the JWKS, keeps it server-side, and sets `unheaded_session` (32 random bytes): `HttpOnly; Secure; SameSite=Strict; Path=/`, `Max-Age` = the token's life (15 min). The response is `{sub, roles, expires_at}`, never the token |
| `GET /auth/me` | `{sub, roles, expires_at}` from the session, else 401 |
| `POST /auth/logout` | With `X-Requested-With`; deletes the session and clears the cookie |
| `GET /auth/auth.js` | The page script: login panel on a 401, who-am-I badge, `X-Requested-With` on every same-origin `fetch`. Served even with auth off (then `/auth/me` is 404 and nothing shows) |

- **Every cookie request re-verifies** the stored token against the cached JWKS, so a session ends with its token, at logout, and when the issuer stops publishing the key that signed it. Expired sessions are evicted every minute. The store is in memory (sessions do not survive a restart) and keyed by SHA-256 of the cookie value; a Well-backed store can implement `bff.Store`.
- **CSRF:** `SameSite=Strict`, plus a second guard: login, logout and every state-changing request that a session cookie authenticates need `X-Requested-With: unheaded` (403 without). Bearer-token callers are not affected.
- **Login rate limit** in front of the issuer's lockout (which only sees the backend's address): 10 attempts a minute per client address and username, 60 per client address. A login always issues a new session id and drops a presented one.
- **Roles:** reads take `admin`, `operator`, `observer`, `viewer` or `service`; writes take `admin`, `operator` or `service`; `guest` gets nothing. The dashboard needs a login for every `/api/` call and WebSocket (its pages, scripts and styles stay open); the board stays publicly readable and needs an operator login to write. The compliance profile switch (`POST /api/v1/compliance/profile`) takes `admin` or `operator` only and records the login's `sub` as the change's actor.
- **WebSockets** authenticate by the cookie; on the dashboard a handshake from a foreign `Origin` is refused (403) before authentication.
- **Calling other services for a user:** the backend uses its own service token and signs the user into it as the RFC 8693 `act` claim (`auth.WithActor`; `bff.AuthorizeOutgoing` also sets `X-Unheaded-On-Behalf-Of`). The receiver sees `Identity.Extra["act"]`; it grants nothing and is for audit records.
- **Issuer over TLS:** the BFF reaches the issuer with the system roots plus the deployment CA when `UNHEADED_TLS_ENABLED=true` (compose: `https://auth:19020`, Secure cookie). `AUTH_BFF_DEV_PLAINTEXT=true` (or dashboard-backend `-dev-plaintext`) remains for a stack without TLS: it drops `Secure` and allows a plain-http issuer; without it a plain-http issuer off loopback refuses to start.
- The login speaks this issuer's `POST /login`. Another OIDC provider would need an authorization-code flow in `pkg/bff`, which is not built.

## Replacing it with another IdP

Any OIDC provider that signs with Ed25519 (EdDSA) and publishes discovery and a JWKS plugs in the same way: point `AUTH_OIDC_ISSUER_URL` at it and put the role names in a `roles` claim. Authentik or Keycloak would be new third-party services (own nologin account, pinned image, ADR-004 review).

---

> **Source:** [cmd/unheaded-auth/](../cmd/unheaded-auth/) · [ADR-103](../docs/adr/ADR-103-service-auth-token-distribution.md) · [S-IRON-GATE Phase 4](../docs/battle-plans/S-IRON-GATE-battle-plan.md)
