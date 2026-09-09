# WAF: three config toggles were parsed and never honored (2026-07-31)

**Component:** `cmd/waf` (THE SHIELD)
**Severity:** Medium — operator-facing controls that silently did nothing
**Found by:** clippy `dead_code`, while working the warning ratchet down from 348
**Status:** FIXED in the working tree (uncommitted, pending review)

## What was wrong

Three `enabled` flags were deserialized from the WAF config, exposed to
operators, documented in `config.example.toml` — and then never read by any
code path:

| Config key | Struct | Effect of setting it |
|---|---|---|
| `[rate_limit] enabled` | `config.rs` `RateLimitConfig` | none — limiter always active |
| `[circuit_breaker] enabled` | `config.rs` `CircuitBreakerConfig` | none — breaker always active |
| `[metrics] enabled` | `config.rs` `MetricsConfig` | none — endpoint always listening |

`main.rs` built each subsystem from its *other* fields (`default_rate`,
`failure_threshold`, the timeouts, `metrics_addr`) and dropped `enabled` on the
floor. A tree-wide grep for `.enabled` found exactly one consumer —
`rules/mod.rs:73`, the per-rule flag, which is a different thing entirely.

## Why it matters

These are not cosmetic. An operator who sets `enabled = false` gets no error,
no warning, and no change in behavior — the strongest possible form of a
misleading control. Concretely:

- **Metrics.** `[metrics] enabled = false` could not close the metrics
  listener. The endpoint exposes backend names, per-path request counts and
  error classes, and there was no supported way to turn it off short of
  firewalling the port.
- **Circuit breaker.** An operator shedding a misbehaving breaker during an
  incident could not disable it; the documented escape hatch was inert.
- **Rate limiter.** Same — no way to disable via config.

The failure direction is "protection stays on", which is not itself dangerous.
The danger is the false belief: the config said the feature was off, and
operators reasonably act on what the config says.

This is the same class as the CI findings in
`findings-remediation-2026-07-29.md` — a control that reads as configured but
cannot actually fail or fire. Nothing had ever tested it.

## Fix

- `circuit.rs` — `CircuitBreakerConfig` gains `enabled` (defaults true);
  `CircuitBreakerManager::is_enabled()` exposes it.
- `proxy.rs` — the breaker is now `Option<Arc<CircuitBreaker>>`. When disabled
  no breaker is fetched, so state is neither consulted **nor recorded**.
  `None` means "breaking is off", never "backend is healthy".
- `router.rs` — `rate_limit_enabled` field + `set_rate_limit_enabled()`; both
  the per-IP pre-filter and rule-driven `RuleDecision::RateLimit` are skipped
  when off. Rules resolving to RateLimit fall through as **allowed** — the
  alternative (treating them as blocks) would make disabling the limiter more
  restrictive than leaving it on.
- `server.rs` — `metrics_enabled` gates `start_metrics_server()` entirely, and
  startup output prints `Metrics: disabled` rather than a live address.
- `main.rs` — passes all three config values through.

## Regression guard

`circuit::tests::enabled_flag_reaches_the_manager` asserts both directions.
Verified by reintroducing the bug (`is_enabled()` hardcoded to `true`) and
watching the test fail with its own message, then restoring and watching it
pass. A green test that has never been seen red is not evidence.

`cmd/waf`: 53 tests pass.

## Second finding: `generate_self_signed()` is doubly unreachable — RESOLVED 2026-08-03

`cmd/waf/src/tls.rs:143` gates a self-signed-certificate helper behind
`#[cfg(feature = "test-cert")]`. That feature cannot be turned on and the
function would not build if it were:

1. **`cmd/waf/Cargo.toml` has no `[features]` section at all**, so
   `--features test-cert` fails with "unknown feature".
2. The body calls `rcgen::generate_simple_self_signed`, and **`rcgen` is not a
   dependency** of the crate. Declaring the feature would just move the failure
   from Cargo to the compiler.

So the code has never compiled once since it was written, and the only signal
was a `unexpected cfg condition value` warning in a job that was non-gating.

Impact is low — it is a test helper, not a request path, and nothing calls it.
It matters as a category: a TLS helper that *looks* available invites someone
to reach for it during an incident and discover it is fiction.

**RESOLVED 2026-08-03: deleted.** Stevie chose deletion over adding `rcgen`.
`crates/zhend/src/api/quic.rs:214` already has a working
`generate_self_signed_cert()` built on rustls types, so the WAF copy was
redundant as well as non-functional. A comment at the old site records why it
went and points at the zhend helper.

## Third finding: the crate shape was hiding everything else — RESOLVED 2026-08-03

`cmd/waf` was a **binary crate with no `lib.rs`**, so `pub` granted no
dead-code exemption and ~49 items read as dead. Split into `src/lib.rs` (the
WAF) plus a thin `src/main.rs` (CLI). **49 → 3**, and the 3 survivors were
real:

1. **`PatternMatcher` compiled every rule pattern twice.** It built a
   `RegexSet` *and* a `Vec<Regex>` "for capture groups (if needed)". They were
   never needed — every method answers from the `RegexSet`. The second compile
   was pure cost at startup and a duplicate automaton resident for the process
   lifetime. Field removed.
2. **`LabeledCounter` / `LabeledHistogram` declared `label_names` and never
   read them.** The doc on `LabelKey` states the invariant — label values are
   "ordered to match its `label_names`" — and nothing enforced it. Prometheus
   requires a consistent label set per metric family, so a call site passing
   different names, or the same names in a different order, would emit series
   that disagree and a scraper may reject the whole family. Now enforced by
   `assert_label_schema` (a `debug_assert`: the label set is fixed by the call
   site at compile time, so a mismatch is a coding error, and this sits in the
   per-request path). Every existing call site was checked and conforms.

   Guarded by `wrong_label_order_is_rejected` and `undeclared_label_is_rejected`,
   both **verified by watching them fail** with the assert neutered before
   restoring it.

## Fourth finding: two `RuleAction` types — RESOLVED 2026-08-03

An earlier draft of this document described "a duplicate `Action` enum parallel
to `RuleAction`". That was wrong, and the truth is worse. `actions.rs` defined
a `RuleAction` **struct**; `matcher.rs` defines a *different* `RuleAction`
**enum**. Same name, different kind, sibling modules — and the enum is the one
the enforcement path actually uses. The struct, its `Action` enum, and its
`to_decision()` were entirely unreachable. All deleted.

Also deleted from `actions.rs`: the five `RuleDecision` predicates
(`is_allow`, `is_block`, `is_rate_limit`, `block_status`, `block_message`),
which had no caller outside the file's own tests — `router.rs` matches on the
enum directly — and `BlockResponse::{text, html}`, which nothing constructed.
Removing `html` took its private helpers `html_escape` and `status_text` with
it. **If an HTML block response is ever reintroduced, the escaping must return
with it**: the message it renders is attacker-influenced. That warning is
recorded in the module header, not just here.

The enforcement path was re-verified intact throughout: `matcher.rs` maps its
`RuleAction` → `RuleDecision`, `router.rs` handles `RuleDecision::Block`. The
WAF still blocks.

Three tests went with the code they covered; three new ones arrived with the
label-schema guard. `cmd/waf`: **53 tests pass**, and the crate now reports
**0** clippy warnings.

## Still open

`generate_self_signed` and the dead rule helpers are gone, but two `from_str`
methods in `config.rs` and `rules/matcher.rs` still shadow the standard
`std::str::FromStr::from_str` without implementing the trait. Cosmetic, but it
is the kind of name collision that makes a call site read as something it is
not. Left for a future pass.

---

# Related: the same pattern in `cmd/trace-collector` (2026-08-03)

Found while taking clippy to zero. Two more controls that read as configured
and did nothing, both surfaced by a single `too_many_arguments` warning on
`run_source_reader`, whose signature carried `_global_stats` and
`_poll_timeout_ms` — accepted and dropped on the floor.

## `poll_timeout_ms` was inert

`MultiSourceConfig::poll_timeout_ms` was settable through **two** separate
builder methods (`MultiSourceReader::with_poll_timeout` and
`MultiSourceReaderBuilder::poll_timeout`), read at the spawn site, passed into
the reader — and discarded. `bpf/ringbuf.rs` hardcoded `self.poll_wait(100)`.

The timeout bounds how long a reader blocks before re-checking the shutdown
flag, so it is also the worst-case shutdown latency. An operator tuning it got
no error and no effect.

**Fixed.** `RingBufReader::run` now takes `poll_timeout_ms` and uses it.
`Config` gained a `poll_timeout_ms` field (serde default 100) so the setting is
real end to end rather than reachable only through the multi-source builder.
The one-shot `dump` path uses an explicit `DUMP_POLL_TIMEOUT_MS` constant —
it is an interactive diagnostic, not something an operator tunes.

## `GlobalStats` always read zero

`MultiSourceReader::global_stats()` is a public accessor over
`total_events` / `total_dropped` / `total_errors` / `total_bytes`. Every source
reader updated its **per-source** `SourceStats` and nothing ever folded those
into the global counters, which were constructed `Default::default()` and never
touched again.

This is the failure direction that matters: a monitoring surface stuck at zero
does not read as broken, it reads as *healthy*. "Zero dropped, zero errors" is
exactly what you want to see, and it was unconditional.

**Fixed.** `run_source_reader` now folds each source's totals into the global
counters when the reader stops, and records an error both for a failed ring
buffer open and for a reader that exits with an error.

## Argument bundling

The original warning is resolved by grouping the eight positional parameters
into a `SourceReaderTask` struct. That shape mattered here: the signature had
two `Arc<...>` and two integers adjacent, which is precisely where a transposed
call site still compiles.

`cmd/trace-collector`: 160 tests pass.
