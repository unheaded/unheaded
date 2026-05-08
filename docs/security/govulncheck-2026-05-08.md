# govulncheck Sweep — 2026-05-08

**Auditor:** Marshal drain shift (continuation phase, post-push).
**Tool:** `govulncheck` (latest from golang.org/x/vuln, installed via `go install`).
**Host:** WEST (Linux 6.17, Go 1.24.0 → 1.25.10 toolchain pin).
**Scope:** entire kingdom (`./...`).

---

## Headline

| Stage | Vulns affecting kingdom code | Notes |
|-------|------------------------------|-------|
| Initial scan @ go1.24.0 + grpc 1.65.0 | **35** | All stdlib (crypto/x509, net/http, etc.) |
| After toolchain → 1.24.4 | 30 | 5 closed (low-impact patches) |
| After toolchain → 1.25.10 | 1 | 29 closed via stdlib patches |
| After grpc 1.65.0 → 1.79.3 | **0** | Last one closed |

**Final state: zero vulnerabilities affecting kingdom code.**

---

## What changed

### go.mod

```diff
 go 1.24.0
+
+toolchain go1.25.10

-google.golang.org/grpc v1.65.0
+google.golang.org/grpc v1.79.3
```

Plus transitive bumps from the grpc upgrade:
- `golang.org/x/crypto v0.43.0 → v0.46.0`
- `golang.org/x/net v0.46.0 → v0.48.0`
- `golang.org/x/text v0.30.0 → v0.32.0`
- `google.golang.org/genproto/googleapis/rpc → 2025-12-02 snapshot`
- `google.golang.org/protobuf v1.34.1 → v1.36.10`

### Vulnerabilities closed (representative — full list in govulncheck output)

| ID | Component | Impact |
|----|-----------|--------|
| **GO-2025-3749** | crypto/x509 | ExtKeyUsageAny disables policy validation — was reachable via `pkg/mesh/mtls/certs.go::Verify` |
| **GO-2025-3563** | net/http/internal | Request smuggling via invalid chunked encoding — was reachable via `pkg/metrics/metrics.go::Push → io.ReadAll` |
| **GO-2025-3503** | net/http (proxy) | HTTP proxy bypass via IPv6 zone IDs — was reachable via `pkg/loadbalancer/l7.go::GRPCProxy.ServeHTTP` |
| GO-2025-* (×27) | crypto/tls, html/template, net/url, encoding/{asn1,pem}, archive/tar, os, os/exec, net, net/http/httputil, net/textproto, database/sql | Closed via 1.25.10 stdlib patches |
| **GO-2024-3309** | google.golang.org/grpc | The lone grpc CVE; closed via 1.65.0 → 1.79.3 |

---

## Verification

```bash
cd ~/tmp/unheaded
~/go/bin/govulncheck ./...
# → "Your code is affected by 0 vulnerabilities."

go version
# → go version go1.25.10 linux/amd64

go test -short -count=1 -timeout 120s ./...
# → 219 packages PASS, 0 FAIL (pre-this-commit S77 gate already PASS)
```

## Why toolchain 1.25.10 (not 1.24.13)

Two paths to close the bulk of stdlib vulns:

1. **1.24.13** — stays on the 1.24 line; closes most issues but `archive/tar`, `html/template`, and a handful of others are only patched on 1.25.x.
2. **1.25.10** — moves to the next major; closes 33/35 in one bump; aligns with the language version that grpc v1.79.3 requires.

Path 2 chosen. The kingdom's Go code uses no 1.25-only language features today; the toolchain bump is non-disruptive at the source level.

## Cross-reference

- `go.mod` lines 5-12 (toolchain directive + rationale comment).
- ADR-052 (in-tree source-of-truth policy).
- `docs/security/cargo-audit-2026-05-07.md` Wave A — this is the Go-side parallel of the Rust-side CVE closure.
- `docs/security/gosec-sweep-2026-05-08.md` — companion doc for the static-analysis sweep.

## Standing recommendation

Schedule a quarterly govulncheck run (next: 2026-08-08) and bump toolchain to whatever is current at that time. Stdlib CVE backports land continuously; staying on a +6mo old toolchain accrues advisories steadily.
