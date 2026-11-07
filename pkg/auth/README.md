# pkg/auth -- Authentication Skeleton

## Status: Stub (P1 Security Backlog)

This package defines the authentication interfaces and stub implementations
for the Unheaded Kingdom. It is referenced by TODO.md items #16 (no auth on
any endpoint) and #43 (auth middleware missing).

## Design

Two authentication mechanisms are planned:

### 1. JWT (External API Clients)

- Used by: Dashboard UI, CLI (`wotan-ctl`), third-party integrations
- Flow: Client presents `Authorization: Bearer <token>` header
- Validation: JWKS endpoint or pre-shared public key
- Claims: `sub` (subject), `roles` (authorization), `exp` (expiry, required: a token without it is refused)
- Library: `golang-jwt/jwt` (not yet added to go.mod)

### 2. Service tokens (Inter-Service Communication)

- Used by: All service-to-service calls within the Kingdom (ADR-103)
- Flow: each binary mints a 5-minute EdDSA token per target; TLS 1.3 carries it
- Trust: verifiers re-read `trust.d` every 30 seconds, so adding or removing a `.pub` needs no restart, provided the directory existed when the service started: one created later (first `unheaded-keygen` on a host) needs a restart; user-token keys stop being trusted after three JWKS TTLs with no good fetch
- Validation: the caller's `.pub` in `trust.d`, the caller in `AUTH_ALLOW_CALLERS`
- Identity: `sub` = key name (e.g. `timeguru`, `captain`)

### Audit trail

Every refusal, at authentication or at RBAC, over HTTP or gRPC, is recorded as
one JSON line with the service, path (for gRPC, the full method), method (`GRPC`
for gRPC), peer address and subject, never a credential. By default these go to
stderr, the service's journal. `AUTH_AUDIT_LOG=<path>` keeps every decision in
that file, each accepted credential included, with a further line from RBAC
where the service has it. Paths and methods are cut to 512 bytes. Packaged
units declare `LogsDirectory=unheaded/<name>`, so
`AUTH_AUDIT_LOG=/var/log/unheaded/<name>/auth-audit.log` in the unit's env file
works under `ProtectSystem=strict`; any other path needs `ReadWritePaths=`.

The file rotates by size: past 64 MiB it moves to `<path>.1`, older generations
shift up, and `<path>.7` is dropped, so the trail holds at most 512 MiB and
keeps the newest events. Nothing else may rotate it, and copy generations off
the host if the trail must outlive that bound. A rotation that fails loses no
event and drops no extra generation: writing carries on in the current file,
which outgrows 64 MiB until a rotation works, the failure goes to stderr, and
the next attempt waits for another 64 MiB. A `<path>.next` beside the trail is
the empty new file of a rotation that stopped; the next one reuses it.

A log that cannot be opened stops a service built on `NewServiceHandler`,
`NewServiceChain`, `NewServiceMiddleware` or `NewServiceAuthenticator`. On
`SetupMiddleware`, whose callers cannot exit, the service keeps running, refuses
every request outside `/health`, `/ready` and `/metrics`, and records the
refusals on stderr.

### Common Interface

Both backends implement `Authenticator`:

```go
type Authenticator interface {
    Authenticate(ctx context.Context, r *http.Request) (*Identity, error)
}
```

### Middleware Integration

```go
// Example: protect all API routes with JWT
mux.Handle("/api/v1/", auth.Middleware(jwtValidator)(apiHandler))
```

### Identity Propagation

Once authenticated, the `Identity` is stored in the request context:

```go
identity := auth.IdentityFromContext(r.Context())
if identity != nil && identity.HasRole("admin") {
    // authorized
}
```

## Implementation Roadmap

1. **Alpha (current):** Stub implementations, interfaces defined
2. **Beta:** JWT validation with JWKS
3. **GA:** Full RBAC, token rotation, CRL/OCSP, audit logging

## Files

- `auth.go` -- Interfaces, stubs, middleware, Identity type
