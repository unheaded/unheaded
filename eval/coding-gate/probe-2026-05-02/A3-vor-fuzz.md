---
title: A3 — cs/vor /api parser fuzz
severity: MEDIUM (one DoS vector + several lax-parser warnings; no RCE/data-leak)
date: 2026-05-02
sanitized: true
---

# A3 — cs/vor /api parser fuzz

Quick adversarial probe of `cs serve` (vor's REST API) with malformed inputs. Full CSV at `/tmp/vor-fuzz-results.csv`. 53 probe requests; results categorized below.

## Summary

| HTTP code | count | meaning |
|---|---|---|
| 200 | 41 | accepted (most return "null" because no match) |
| 400 | 11 | "missing q parameter" — vor's structured rejection |
| 404 | 1 | path-not-found on raw `..` traversal |
| TIMEOUT | 1 | 1M-char query body |

## Findings

### F1 — Unbounded query length (DoS) — MEDIUM

A query of 1,000,000 characters caused vor to time out (>10s). vor accepts arbitrarily-long `q=` parameters with no length cap. On localhost this is annoying; if vor is ever exposed beyond loopback (Phase D-A may put it on a service port), this is a single-request DoS — sustained 1M-char queries trivially exhaust CPU.

**Mitigation:** add a `MaxQueryLength` constant in vor's HTTP handler (suggest 4096 chars; queries beyond that get 413 Payload Too Large).

### F2 — Path traversal partially exploitable via fuzzy resolver — MEDIUM

```
GET /api/topics/..%2Fetc%2Fpasswd   → 200, returns content of "docs/binder-book/unheaded-protocol"
GET /api/topics/%2e%2e%2fetc        → 200, returns same content
GET /api/topics/A%00B               → 200, returns content of "llama.cpp/docs/multimodal/MobileVLM"
GET /api/topics/a/b/c               → 200, returns content of "security/access-control-models"
```

vor's fuzzy resolver accepts arbitrary percent-encoded paths and silently matches them to *some* topic. **No filesystem-level path traversal** (an unencoded `../etc/passwd` returns 404 cleanly), but the percent-encoded variants succeed in matching unrelated topics. This is a subtle data-confusion vector — an attacker who can plant a known string in any markdown source can guess paths until vor surfaces it.

**Mitigation:** reject percent-encoded `..`, `/`, and `\0` in the topic-name segment before fuzzy resolution. Treat unresolvable names as 404, not as fuzzy fallback.

### F3 — Method-agnostic endpoints — LOW

`POST /api/search?q=test` returns 200 with the same results as `GET`. No CSRF token, no method enforcement. Standard API hygiene says GET endpoints should reject mutation methods. Not exploitable on its own (the endpoint is read-only) but suggests vor's HTTP handler doesn't switch on `r.Method`.

**Mitigation:** explicit method dispatch; return 405 Method Not Allowed for non-GET on read endpoints.

### F4 — Silent percent-decode failures — LOW

Inputs `%`, `%g`, `%0`, `%X`, `%%` all return `400 missing q parameter`. The Go `net/url` package returns an error, vor catches it broadly and reports as if `q` were absent — confusing. Inputs `%FF` and `%00` decode successfully (with `%00` allowing a null byte through to fuzzy resolution).

**Mitigation:** distinguish "q parameter absent" from "q parameter malformed" in the error response. Reject `%00` (null byte) explicitly as a defense-in-depth measure.

### F5 — Reserved-character handling: ambiguous, but not exploitable — INFO

vor accepts the following un-escaped characters in the `q=` value and returns a sensible "no match" `null`:

```
;  %  ?  #  &  =  +  (newline)  (CR)  (TAB)  \  "  '  <  >  |  `  $  ()  []  {}
```

The earlier Phase C bug — where `;` made vor return 400 — was a side-effect of how the Go HTTP router parsed the query string, not vor's logic. After switching zhen-rag to `url.QueryEscape`, the issue is moot. But the bug class (server's parser disagrees with client's encoding) could recur in future refactors.

**Mitigation:** add a vor regression test asserting `GET /api/search?q=test%3Btest` returns 200 (not 400).

### F6 — Empty-q variants — properly rejected

`q=`, `q`, `Q=test`, `q=&q=second` all return `400 missing q parameter`. Behavior is consistent. No issue.

## Severity overall

No RCE. No filesystem-level data leak. No authentication bypass (the API is unauthenticated by design — localhost-only). The DoS vector (F1) is the most actionable; the path-traversal-via-fuzzy-resolver (F2) is the most concerning if vor ever gets exposed beyond loopback. Both are MEDIUM, easily fixed.

## Recommended LICH-011 campaign

Hand off to a sustained AFL++ or radamsa campaign against `cs serve`, target `/api/search` and `/api/topics/:name`. Run 24h, capture all crash signatures and any non-200/400/404 responses. Coordinate with cs/vor upstream maintainer (Stevie) for fix triage.
