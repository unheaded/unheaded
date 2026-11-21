# unheaded-auth — Built-in OIDC Issuer

`unheaded-auth` is the small issuer people log in to when no other identity provider is configured ([ADR-103](../docs/adr/ADR-103-service-auth-token-distribution.md) §3). It checks a password, mints a short-lived EdDSA access token with a `roles` claim, and publishes the public key services verify it with. It is off the service-to-service path: services authenticate each other with their own keys (`unheaded-keygen`, trust.d), so if the issuer is down, logins fail and running services keep talking.

## Running

| | |
|---|---|
| Binary | `cmd/unheaded-auth` (`scripts/build-binaries.sh`, deb `unheaded-unheaded-auth`) |
| Port | 19020 (`pkg/ports.Auth`) |
| Account | `unheaded-auth`, uid 16735 (`pkg/uids.Auth`), nologin |
| Host unit | `deploy/systemd/unheaded-auth.service` (not installed anywhere yet) |
| Compose | service `auth` on the `data` network, `https://auth:19020` with a dev certificate (`scripts/gen-dev-certs.sh`); published on the host's loopback only (`127.0.0.1:19020`) for host units and host processes that log people in (wiki server, Zhen web UI, zhen-agentd). Such a host carries `127.0.0.1 auth` in `/etc/hosts`: the name is the issuer string in every token and on the certificate |
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
- **Lockout:** 10 failures for one user from one address, or 25 from one address across users, within 10 minutes answer 429 (an attempt counts from the moment it arrives, so a burst gets no more guesses) with `Retry-After` without hashing anything. There is no per-user lock across addresses, so a caller at one address cannot lock out a user at another. Behind a backend the address is the backend's, so these locks fall on everyone logging in through it: see the login budget below. The address is the TCP peer, never `X-Forwarded-For`.
- **Refuses to start** when its key or users file is readable by group or others, the users file is malformed, or the issuer URL is not https (plain http only to loopback, or anywhere with `-dev-plaintext`).
- **TLS** comes from `pkg/tls` (`UNHEADED_TLS_*`, set in the unit; material under `/etc/unheaded/tls/unheaded-auth/`) with the hybrid ML-KEM Strict profile ([[Post-Quantum Key Exchange|Post-Quantum-Key-Exchange]]). Without TLS and without `-dev-plaintext` only the probes answer.
- **Users:** `unheaded-auth user add <name> --roles operator` (prompts twice, at least 15 characters; `--password-stdin` for scripts), `user list`, `user del <name>`. Roles: `admin`, `operator`, `viewer`, `observer`, `guest` (`service` is refused: it is what service tokens carry). The file is replaced atomically and keeps its owner. Reload with `SIGHUP` (`systemctl kill -s HUP unheaded-auth`, `docker compose kill -s HUP auth`); a broken file keeps the users already loaded. Removing a user does not revoke tokens already issued; they expire within 15 minutes.
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
| Topic permissions | `configs/wotan.yaml` `topics.permissions` (FND-043, 2026-11-11) bounds each approved identity: `subscribe` patterns it may stream, subscribe to and read over `GET .../messages`, and `publish` topics (a service may always read what it may publish). A subscription pattern passes only when an entry covers every topic it could match (`tasks.#` is not covered by `tasks.*`); `#` is granted to the dashboard alone. `unlisted: deny` refuses an approved identity the section does not name; `mode: audit` lets calls through and logs `topic_permission_refused_audit_only` with the counter `unheaded_wotan_topic_permission_refusals_total{action,outcome,caller}`, `mode: enforce` refuses with `TopicAccessDenied` (HTTP 403) or `UnauthorizedPublish` (gRPC PermissionDenied). `caller` is the refused identity as the section spells it, or `other` for anything it does not name; every listed caller's series starts at zero when the config loads. Any refusal raises the ticket `WotanTopicPermissionRefused` (`monitoring/prometheus/rules/wotan.yml`, runbook `runbooks/observe/wotan-topic-refused.yaml`). Only EdDSA service identities are judged; without the section Wotan behaves as before it. Reloaded on SIGHUP |
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
| `scripts/run-runbook.py` | `scripts/svc_token.py` (Python `cryptography`) | the key it is given: `AUTH_KEY_FILE` as the name it is filed under (a Champion run is `zhen-agentd`), else the operator's CLI key as `unheaded-cli`; `RUNBOOK_PROGRESS_IDENTITY` overrides the name. An `https://` `RUNBOOK_PROGRESS_WOTAN` is TLS 1.3 only, checked against `UNHEADED_TLS_CA_CERT` (else the system roots) |
| `cmd/demo-trace-injector` | `pkg/wotan-client` | `unheaded-cli` (an operator's demo tool) |
| `curl` | `unheaded token --aud wotan` prints one 5-minute token from the CLI's key | `unheaded-cli`: `curl -H "Authorization: Bearer $(unheaded token --aud wotan)" ...` |

A missing key is one warning and calls without a token, as for the Go clients. Still anonymous: the `curl`s in `scripts/bare-metal/validate-cross-host.sh` and `runbooks/wotan/cluster-*` (their publishes send a `sender_id` body Wotan does not read, and `cluster/promote` is the admin API); `scripts/demo-start.sh` only reads the open `/health`. None of the Rust clients speaks TLS yet (Phase 8).

## Browser login: the backend-for-frontend

Browsers never hold a bearer token ([ADR-103](../docs/adr/ADR-103-service-auth-token-distribution.md) §4). dashboard-backend (20000), kanban-app (20001) and, when its env file names an issuer, wiki-server (20002) each run `pkg/bff` (each page's own backend logs its users in, under its own cookie name). What follows is the default, where a session ends with the issuer's token; [Server-side sessions](#server-side-sessions) is the other mode, where it does not:

| | |
|---|---|
| Config | `AUTH_ENABLED=true` and `AUTH_ISSUER_URL` (compose: `http://auth:19020`; units: `https://%H:19020`). The issuer must list the service in `-audiences` (compose and unit list `unheaded,dashboard-backend,kanban-app`) |
| `POST /auth/login` | `{"username","password"}` with `X-Requested-With: unheaded`. The backend posts it to the issuer's `/login` with `audience` = its own name, verifies the token against the JWKS, keeps it server-side, and sets the session cookie (per app: `__Host-unheaded_session_<service>`, `unheaded_session_<service>` under `AUTH_BFF_DEV_PLAINTEXT`; 32 random bytes): `HttpOnly; Secure; SameSite=Strict; Path=/`, `Max-Age` = the token's life (15 min). The response is `{sub, roles, expires_at}`, never the token |
| `GET /auth/me` | `{sub, roles, expires_at}` from the session, else 401 |
| `POST /auth/logout` | With `X-Requested-With`; deletes the session and clears the cookie |
| `GET /auth/auth.js` | The page script: login panel on a 401, who-am-I badge, `X-Requested-With` on every same-origin `fetch`. Served even with auth off (then `/auth/me` is 404 and nothing shows) |

- **Every cookie request re-verifies** the stored token against the cached JWKS, so a session ends with its token (15 minutes: every page then asks for the login again), at logout, and when the issuer stops publishing the key that signed it. Expired sessions are evicted every minute. The store is in memory (sessions do not survive a restart) and keyed by SHA-256 of the cookie value. This is the mode a front runs in while `AUTH_SESSION_STORE` is unset.
- **CSRF:** `SameSite=Strict`, plus a second guard: login, logout and every state-changing request that a session cookie authenticates need `X-Requested-With: unheaded` (403 without). Bearer-token callers are not affected.
- **Login rate limit** in front of the issuer's lockout (which only sees the backend's address): 10 attempts a minute per client address and username, 60 per client address. Then the failure budget: see [Login budgets](#login-budgets). A login always issues a new session id and drops a presented one.
- **Roles:** reads take `admin`, `operator`, `observer`, `viewer` or `service`; writes take `admin`, `operator` or `service`; `guest` gets nothing. The dashboard needs a login for every `/api/` call and WebSocket (its pages, scripts and styles stay open); the board stays publicly readable and needs an operator login to write. The compliance profile switch (`POST /api/v1/compliance/profile`) takes `admin` or `operator` only and records the login's `sub` as the change's actor.
- **WebSockets** authenticate by the cookie; on the dashboard a handshake from a foreign `Origin` is refused (403) before authentication.
- **Calling other services for a user:** the backend uses its own service token and signs the user into it as the RFC 8693 `act` claim (`auth.WithActor`; `bff.AuthorizeOutgoing` also sets `X-Unheaded-On-Behalf-Of`). The receiver sees `Identity.Extra["act"]`; it grants nothing and is for audit records.
- **Decision trail:** refusals reach the service's journal. `AUTH_AUDIT_LOG=/var/log/unheaded/<name>/auth-audit.log` in the unit's env file keeps every decision there instead (each packaged unit declares that directory); the file rotates at 64 MiB and keeps 7 generations, 512 MiB at most, oldest dropped (`pkg/auth/README.md`).
- **Issuer over TLS:** the BFF reaches the issuer with the system roots plus the deployment CA when `UNHEADED_TLS_ENABLED=true` (compose: `https://auth:19020`, Secure cookie). `AUTH_BFF_DEV_PLAINTEXT=true` (or dashboard-backend `-dev-plaintext`) remains for a stack without TLS: it drops `Secure` and allows a plain-http issuer; without it a plain-http issuer off loopback refuses to start.
- The login speaks this issuer's `POST /login`. Another OIDC provider would need an authorization-code flow in `pkg/bff`, which is not built.

## Server-side sessions

Off unless a front's environment sets `AUTH_SESSION_STORE` ([ADR-114](../docs/adr/ADR-114-user-authentication-for-every-web-page.md) N5, amendment 2026-11-21; decision pack 2026-11-18, row 44). No shipped unit or compose service sets it. It applies to the three `pkg/bff` fronts; the Zhen web UI's app has its own login and still asks again after 15 minutes.

With it on, a login is a row in the Well instead of a token held in memory. The front checks the issuer's token once, at login, and drops it. The browser's cookie is 32 random bytes; the row is found by the SHA-256 of that value, and neither the value nor a token is stored. A session then ends when unused for the idle time, at its maximum age whatever its use, or when revoked. Restarting a front no longer logs anyone out.

### Keys (a front's environment)

| Key | Default | Meaning |
|---|---|---|
| `AUTH_SESSION_STORE` | unset | unset: in memory, ending with the token (the section above). `well`: rows in the Well. `dev-memory`: the same lifetimes in the front's own memory, for a development stack without a Well; it must be named, logs a warning at start, and nothing outside the process can list or revoke such a session. Anything else: the front does not start. There is no value that tries the Well and settles for memory |
| `AUTH_SESSION_IDLE` | `30m` | A session unused this long ends. Each use moves the end forward, never past the maximum age. At least `1m`, at most the maximum age |
| `AUTH_SESSION_MAX_AGE` | `12h` | A session ends this long after login whatever its use. At most `744h` (31 days; the table refuses more) |
| `AUTH_SESSION_RETENTION` | `24h` | How long an ended session stays on record (`unheaded session list --all`) before its front deletes it. At most `2160h` (90 days); `0` is the default, like unset |
| `AUTH_SESSION_MAX_PER_USER` | `10` | One user's live sessions on this front. A login past it is not refused: that user's oldest session is ended (recorded as `limit`). So one account cannot fill the front, and a user can always log in. `1` to `1000`, and no more than the front's own cap |
| `AUTH_SESSION_MAX_CLOCK_SKEW` | `2m` | How far this host's clock may be from the Well's when a session starts. Past it every login answers 503 and the log says `clock skew`: the front's clock is what later decides when the session ends, so a front with a wrong clock starts none. `1s` to `1h` |
| `AUTH_SESSION_MISS_BUDGET` | `30` | How many unknown or ended session ids one client address may present in a minute. Past it, an id this front has not lately seen live is answered 401 without asking the Well (see "Floods" below). `1` to `100000` |
| `AUTH_SESSION_WELL_PASSWORD` | none | The password of the front's own Well role. Required with `well`; there is no default |
| `AUTH_SESSION_WELL_DB` | `unheaded_app` | The database holding `bff_sessions` |
| `AUTH_SESSION_WELL_SSLMODE` | `verify-full` | How the Well's certificate is checked for this store. `verify-full` or `verify-ca`, with the CA in `WELL_SSLROOTCERT`. `require` and `disable` are taken only for a Well on this host (a loopback address, `localhost`, a socket directory). This is not the front's `WELL_SSLMODE`, which the compose file gives every service as `require`: what the Well answers here decides who is logged in, so a server that merely answers is not enough |
| `AUTH_SESSION_WELL_DEV_UNVERIFIED` | unset | `true` admits `AUTH_SESSION_WELL_SSLMODE=require` to a Well on another host, for a development stack with no CA. It logs a warning at every start. It never admits `disable` |
| `WELL_HOST`, `WELL_PORT`, `WELL_SSLROOTCERT` | as for the front's other Well use | Which Well, and the CA that signed its certificate (on west `/etc/unheaded/well-tls/ca.crt`) |

Durations are Go's (`45m`, `8h`). A value that cannot be read, or any of these limits set while `AUTH_SESSION_STORE` is unset, stops the front from starting: a limit that silently did not apply would be worse.

Two combinations are refused at start, where they used to be a warning or nothing:

- `AUTH_BFF_DEV_PLAINTEXT=true` (or dashboard-backend's `-dev-plaintext`) with `UNHEADED_TLS_ENABLED=true`, in any session mode. The switch drops `Secure` and the `__Host-` prefix from the cookie; beside TLS it is a mistake.
- `AUTH_BFF_DEV_PLAINTEXT=true` with `AUTH_SESSION_STORE=well`: a cookie that lasts hours is not sent without `Secure`. A stack without TLS uses `dev-memory`.

- **The Well role is not a key.** A front logs in as `bff_` plus its own name with `-` as `_`: `bff_dashboard_backend`, `bff_kanban_app`, `bff_wiki_server` (migration 055). Each role is granted one view, `bff_sessions_own`, which shows and accepts only the rows of the front the role is named after (migration 056). One front's password therefore cannot read, forge or end another front's sessions, and no front can touch the table itself. Set the passwords with `scripts/well/set-role-passwords.sh` (`BFF_DASHBOARD_BACKEND_PASSWORD`, `BFF_KANBAN_APP_PASSWORD`, `BFF_WIKI_SERVER_PASSWORD`), then give each front its own as `AUTH_SESSION_WELL_PASSWORD`.
- **The cookie** keeps its name and stays `HttpOnly; Secure; SameSite=Strict; Path=/` with the `__Host-` prefix. It has no `Max-Age` any more: the server ends the session, and the browser drops the cookie when it closes.
- **Every request with a session cookie reads the row**, so an expiry or a revocation holds on the next request. Nothing is cached. A use is written at most once a minute per session, so the idle time is exact to within a minute. A client that hangs up mid-request does not cancel the read (it has its own 5 s deadline).
- **A page that polls never idles out.** The dashboard polls its API, and each call is a use: an open tab keeps its session until the maximum age (12 h), not the idle time. The idle time ends a session whose tab was closed or whose machine slept.
- **A session of one front is not a session anywhere else**: by the view in the Well, and again in the front, which believes a row only if it names this front and sits under the key it asked for.
- **Roles** are read from the issuer's token at login and fixed in the row. A role name the row cannot hold (anything but a lower-case letter followed by up to 31 of `a-z 0-9 _ -`) is dropped at login, with a warning in the front's log: a role not held grants nothing. More than 16 roles refuses the login (502).
- **`/auth/me`** also answers `absolute_expires_at`.
- **Bounds:** at most 10,000 live sessions per front (a login past that answers 503) and `AUTH_SESSION_MAX_PER_USER` per user on it; ended rows are deleted after the retention time, at most 1,000 a minute per front.
- **Floods.** A request with a well-formed session id nobody holds costs the Well one read. Each client address may present `AUTH_SESSION_MISS_BUDGET` such ids a minute (unknown, expired or revoked). Past that, an id this front has not seen live within the idle time is answered 401 unread, and one line a minute says so in the log. Ids the front has seen live, and the id of a login just made, are still read on every request as always: that memory admits a request to the read, it never stands in for it. After a front restarts it has seen nothing, so during a flood from a shared address people there log in once more.
- **Logs** name a session by its ID (the first 16 characters of its hash). The cookie value, the issuer's token and passwords are never logged; a test drives every path with the log captured to hold that.

### Listing and revoking

`unheaded session` on a host that can reach the Well, logged in as `app_auth` (`AUTH_WELL_PASSWORD`, with `WELL_HOST` and the rest as for `unheaded apikey`). That role can read sessions and set `revoked_at`; it cannot create one.

```bash
unheaded session list                          # every session not revoked: ID, subject, front, state, last used, expires
unheaded session list --subject dev --all      # one user's, revoked ones included; -o wide|json|yaml
unheaded session revoke 3f9c2a1b7d4e6f80       # one session, by the ID the list shows
unheaded session revoke-all --subject dev      # every session of one user, on every front
unheaded session revoke-all --subject dev --front kanban-app
```

The ID is the first 16 characters of the hash the row is stored under. It is not the cookie and cannot be turned into it. A revoked session is refused on its next request and the browser is shown the login.

- **STATE is by the clock of the host the command runs on.** The clock that decides is the front's. So `list` shows every session not revoked, marking as `expired` the ones past their expiry by this host's clock and showing the expiry itself; it hides none of them, and `revoke` and `revoke-all` act on every session not yet revoked, expired-looking or not. `--all` adds the revoked ones.
- **What ended a session is on its row** (`-o wide`, `-o json`: `revoked_by`): `logout`, `login` (a new login in the same browser replaced it), `limit` (the per-user cap), or `cli:` and the operating-system user and host the command ran as. That last is a note, not a proof: anyone holding `app_auth`'s password can write any name. The Well does refuse a front that writes a `cli:` actor.
- **There is no command that revokes everyone's sessions.** `revoke-all` takes one user. To end every session of a front at once, the table's owner can: `UPDATE bff_sessions SET revoked_at = now(), revoked_by = 'cli:<you>' WHERE front = '<front>' AND revoked_at IS NULL` as `well_app_owner`, or unset `AUTH_SESSION_STORE` and restart.

**What a revocation does not reach:** a WebSocket that is already open. The dashboard checks the session when the stream is opened and not again, so a stream opened before the revocation (or before the session ran out) keeps delivering until it closes; reopening it needs a live session. This was already so with token-bound sessions. Restart the front to cut open streams at once.

**A session no longer ends with the issuer's token, so two things that used to happen by themselves within 15 minutes are now yours:**

- After `unheaded-auth user del <name>`: `unheaded session revoke-all --subject <name>`.
- After changing a user's roles: the same. Roles are read at login and fixed in the row; the user logs in again with the new ones.

### When the Well is down

- **Nobody can log in:** `POST /auth/login` answers 503 `sessions_unavailable` (the panel says "Login is unavailable (HTTP 503)").
- **A logout cannot end the session:** `POST /auth/logout` clears the browser's cookie and answers 503 `logout_incomplete` instead of 204, and the page says so. The row is still live until it runs out idle, for anyone who copied the id; revoke it when the Well is back if that matters.
- **Nobody stays logged in on faith:** a request with a session cookie is treated as having no session, reads included, until the Well answers again. The dashboard, where every read needs a login, cannot be read by a browser meanwhile. The board's public reads go on. Service tokens are not affected; API keys are if their store is the Well too.
- **No session is lost:** when the Well is back, a session that is still inside both its limits works again. The clock did not stop: one that ran out during the outage is gone.
- **Nothing can be revoked** during the outage (`unheaded session` needs the Well), which is why nothing is accepted during it.
- **At start:** a front whose Well does not answer starts anyway, accepts no session and no login, and retries every second, then less often, up to every 30 seconds. A front whose Well answers and refuses (wrong password, missing role, migrations 055 and 056 not applied, a weak `sslmode`) does not start, and its log says which.

### Switching it on

1. Apply migrations 055 and 056 (`scripts/well/batch-apply.sh`; `--dry-run` first).
2. Set the three role passwords (`scripts/well/set-role-passwords.sh`) and make room in the Well's connection budget: each front opens at most 2 connections ([[The Well|The-Well]]; the budget table in `docs/architecture/THE_WELL_OPERATIONS.md`).
3. Give each front `AUTH_SESSION_STORE=well`, its `AUTH_SESSION_WELL_PASSWORD`, and the Well's CA as `WELL_SSLROOTCERT` (unit: the env file; compose: `environment:`, with the CA file mounted), with `WELL_HOST` and `WELL_PORT` if it has none yet (wiki-server), and restart it. The default `verify-full` needs the Well's certificate to name the host the front dials (`postgres` in compose); where it does not, `AUTH_SESSION_WELL_SSLMODE=verify-ca` still checks the CA. Remove `AUTH_BFF_DEV_PLAINTEXT` if it is set. Everyone logs in once more.
4. Check: `unheaded session list` shows the login; `unheaded session revoke <id>` and the next click asks for the login.

To switch it off, unset `AUTH_SESSION_STORE` and restart the front: sessions are in memory again and end with the token. The rows stay until a front with the key set sweeps them.

Proof: `pkg/bff` and `pkg/bff/sessions` tests (on memory, held to one contract with the Well's store); `scripts/well/test-bff-sessions.sh --with-go` for the table, the view, the grants and the Well's store on a throwaway server.

## Login budgets

The issuer locks an address out after 10 failed logins for one username, or 25 across usernames, within 10 minutes (`cmd/unheaded-auth/lockout.go`). Behind a login front the address the issuer sees is the front's, and every front on a host shares it, so those locks would fall on everyone at once. Each front therefore holds each *client* address to a budget of its own, charged before the issuer answers, and the budgets are sized so that their sum over all the fronts stays under the issuer's two limits: one client address runs out of tries on every front before it can lock anyone else out. Several client addresses together can still reach the issuer's lock.

| Front | Failures per client address and username | Failures per client address | Where the default lives |
|---|---|---|---|
| dashboard-backend (`pkg/bff`) | 2 | 6 | `pkg/bff/failures.go` |
| kanban-app (`pkg/bff`) | 2 | 6 | `pkg/bff/failures.go` |
| wiki-server (`pkg/bff`, when `/etc/unheaded/wiki-server.env` names an issuer) | 2 | 6 | `pkg/bff/failures.go` |
| Zhen web UI's app (`unheaded-zhen-app.service`) | 3 | 5 | `raft/zhen_login.py` |
| **Sum** | **9** (issuer: 10) | **23** (issuer: 25) | |

All of them count a failure for 10 minutes. The numbers were 2 and 9 for the two `pkg/bff` fronts and 5 and 5 for Zhen until the wiki became a fourth front and the sums reached 11 and 32; each front was shrunk to fit (decision pack 2026-11-18, row 28).

- **Keys.** `AUTH_BFF_USER_LOGIN_FAILURES` and `AUTH_BFF_CLIENT_LOGIN_FAILURES` in a front's environment (unit env file, compose `environment:`) set that front's two budgets; unset, the defaults above apply. The Zhen app reads the same two keys. A value that is not a positive integer, or a username budget above the client budget, is refused: a `pkg/bff` front does not start, and the Zhen app answers every login 503 with the reason. No shipped unit or compose service sets them.
- **If you change one,** keep both sums under the issuer's limits yourself: nothing at run time can see the other fronts' settings. A host with fewer fronts has room to raise them (two `pkg/bff` fronts alone: 4 and 12). One thing is refused whatever the file: a front whose own budget alone reaches the issuer's limit (10 per username, 25 per address) does not start (the Zhen app answers logins 503).
- **Behind a reverse proxy the budget is one for everybody.** `pkg/bff` counts by the address of the TCP peer and reads no forwarded header, and it has no trusted-proxy setting. Reached through the nginx sidecars (`docker/docker-compose.loadbalancers.yml`) or haproxy, every browser has the proxy's address: six wrong logins in ten minutes then lock the login of that front for everyone until they age out, and the miss budget of server-side sessions is shared the same way. The stack's own compose file and the host units publish the fronts directly, where each browser has its own address. The Zhen web UI is the exception: its front (`zhen-ui-front`) hands the app the client's address in a header only it can set. A trusted-proxy setting for `pkg/bff` is not built.
- **A new front is counted without a list.** `cmd/unheaded-auth` `TestLoginFronts_BudgetsFitTheIssuerLockout` reads every binary that builds a BFF from the Go source (`pkg/bff/bfftest.GoFronts`), adds the Zhen app's budgets, and fails with the fronts and both sums in the message when they reach the issuer's limits. A front that is not Go is caught by the key it reads: a source file outside Go that names `AUTH_ISSUER_URL`, or a shipped unit or compose service that sets it, must be accounted for in that test file or it fails. `raft/tests/test_zhen_login.py` holds the same sum from the Python side.
- **What that test cannot see.** It reads the repository. A budget set in a host's env file (`/etc/unheaded/*.env`), a drop-in or a compose override is not counted: the sums are then the operator's to keep. It refuses an override in any shipped unit (`Environment=` in either spelling) or compose file. And it counts a binary once: two instances of one front on a host each spend the budget.
- **The issuer's own two limits** are constants in `cmd/unheaded-auth/lockout.go`, not keys.

## Replacing it with another IdP

Any OIDC provider that signs with Ed25519 (EdDSA) and publishes discovery and a JWKS plugs in the same way: point `AUTH_OIDC_ISSUER_URL` at it and put the role names in a `roles` claim. Authentik or Keycloak would be new third-party services (own nologin account, pinned image, ADR-004 review).

---

> **Source:** [cmd/unheaded-auth/](../cmd/unheaded-auth/) · [ADR-103](../docs/adr/ADR-103-service-auth-token-distribution.md) · [S-IRON-GATE Phase 4](../docs/battle-plans/S-IRON-GATE-battle-plan.md)
