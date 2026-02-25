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
- Claims: `sub` (subject), `roles` (authorization), `exp` (expiry)
- Library: `golang-jwt/jwt` (not yet added to go.mod)

### 2. mTLS (Inter-Service Communication)

- Used by: All service-to-service calls within the Kingdom
- Flow: Client presents certificate during TLS handshake
- Validation: Certificate chain against Kingdom CA, CN/SAN matching
- Identity: CN = service name (e.g. `timeguru`, `captain`)
- Certificate management: Short-lived certs via SOPS + age, auto-rotation

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

// Example: protect inter-service routes with mTLS
mux.Handle("/internal/", auth.Middleware(mtlsValidator)(internalHandler))
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
2. **Beta:** JWT validation with JWKS, mTLS with auto-generated certs
3. **GA:** Full RBAC, token rotation, CRL/OCSP, audit logging

## Files

- `auth.go` -- Interfaces, stubs, middleware, Identity type
