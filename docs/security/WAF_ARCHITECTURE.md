# WAF Architecture: Two-Tier Go Reference + Rust Acceleration

## Overview

THE SHIELD WAF uses a two-tier architecture strategy:

1. **Go Reference Implementation** (current) - Complete, correct, well-tested WAF with
   full detection coverage for SQLi, XSS, SSRF, path traversal, bot detection, and
   anomaly scoring. Serves as the functional specification and correctness baseline.

2. **Rust Acceleration Layer** (planned) - High-performance Rust implementations of
   hot-path detection routines, linked via CGo/FFI. Targets 10x throughput improvement
   on pattern matching and input normalization while preserving identical detection
   semantics.

## Migration Strategy

The Rust acceleration is additive, not a rewrite. The Go implementation remains the
source of truth for detection logic. Rust modules accelerate specific bottlenecks:

| Component | Go Baseline | Rust Target | Technique |
|-----------|-------------|-------------|-----------|
| Pattern matching | `regexp` | Aho-Corasick (multi-pattern) | `aho-corasick` crate, SIMD-enabled |
| HTML tokenization | String scanning | HTML5 spec-compliant parser | `lol_html` or custom SIMD parser |
| SQL tokenization | Rune-by-rune lexer | `nom` parser combinators | Zero-copy tokenization |
| Input normalization | `strings.ReplaceAll` | Byte-level SIMD transforms | `memchr` + vectorized decode |
| Path normalization | Multi-pass decode | Iterative with depth limit | Single-pass byte scanner |
| DNS resolution | `net.LookupIP` (sync) | Async resolution + caching | `trust-dns` async resolver |
| Scoring engine | Mutex-guarded maps | Lock-free atomic counters | `crossbeam` + `atomic` |
| Bot detection | Regex + map lookups | ML model inference | `tract` or `candle` for models |
| Behavior tracking | `sync.RWMutex` maps | Probabilistic structures | HyperLogLog, Bloom filters |
| Correlation engine | Map-based graph | Graph-based correlation | `petgraph` adjacency lists |

## FFI Bridge Design

```
Go (detection logic + orchestration)
  |
  v
CGo FFI boundary (pkg/waf/accel/)
  |
  v
Rust shared library (libwaf_accel.so)
  - Pattern matching engine
  - Input normalizers
  - HTML/SQL tokenizers
```

The Go side calls Rust for hot-path operations (pattern matching, normalization) and
handles orchestration, scoring decisions, and HTTP integration in Go. This keeps the
architecture simple: Rust does the byte-crunching, Go does the decision-making.

## Performance Targets

| Metric | Go Baseline | Rust Target |
|--------|-------------|-------------|
| SQLi detection (per request) | ~500us | ~50us |
| XSS detection (per request) | ~800us | ~80us |
| Pattern matching (100 patterns) | ~1ms | ~100us |
| Input normalization | ~200us | ~20us |
| Overall WAF latency (p99) | ~5ms | ~500us |

## File Organization

```
pkg/waf/
  waf.go              - Shield orchestrator (Go, stays Go)
  scoring.go          - Anomaly scoring engine (Go, atomic counters in Rust)
  detection/
    sqli.go           - SQL injection detection (Go reference)
    xss.go            - XSS detection (Go reference)
    ssrf.go           - SSRF detection (Go reference)
    bot.go            - Bot detection (Go reference)
    path.go           - Path traversal detection (Go reference)
    rce.go            - RCE detection (Go reference)
  accel/              - (planned) Rust FFI bridge
    bridge.go         - CGo bindings
    libwaf_accel.so   - Compiled Rust library
```

## Correctness Guarantee

All Rust accelerated paths must produce byte-identical results to the Go reference
implementation. The test suite runs both paths and compares outputs. Any divergence
is a bug in the Rust implementation, not a reason to change Go.
