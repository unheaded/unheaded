# cmd/waf Rescue Closure — Already Compiling

**Date:** 2026-05-13
**Author:** Marshal-Drain follow-up
**References:** [battle-plan-marshal-drain-2026-05-08.md](battle-plan-marshal-drain-2026-05-08.md) items 27-32

## Summary

The marshal-drain plan (2026-05-08) anticipated a cascade of compile errors
in `cmd/waf/src/rules/mod.rs` around lines 143, 153, 190, and 530 — raw-string
quoting bugs (`r"..."` containing `\"`) and an unterminated string literal at
:530. Items 27-32 of that plan budgeted a rescue shift.

**Current reality:** zero compile errors. `cargo check` in `cmd/waf/` produces
52 warnings (dead code, unused methods, one stale `cfg(test-cert)`) and the
`shield` binary builds cleanly.

```text
$ cd cmd/waf && cargo check
warning: unexpected `cfg` condition value: `test-cert`
warning: method `as_str` is never used
warning: associated items `with_defaults`, `force_state`, and `metrics` are never used
...
warning: `shield` (bin "shield") generated 51 warnings
    Finished `dev` profile [unoptimized + debuginfo] target(s) in 0.04s
```

The `rules/mod.rs` file is 534 lines (no :530 issue), and the regex literals
at the cited lines correctly use raw-string forms — `r"..."` for patterns
without embedded double-quotes and `r#"..."#` (e.g. line 153, 190) for those
that do embed quotes. **Drain items 27-28 are MOOT** — fixed by prior
shifts, no traceable commit pointer needed since the file already builds.

## Drain Item Resolution

| Drain Item | Status      | Notes                                                |
|-----------|-------------|------------------------------------------------------|
| 27 (raw-string `\"`)   | **MOOT**    | Code uses `r#"..."#` correctly.          |
| 28 (unterminated :530) | **MOOT**    | File ends at 534, no dangling literal.   |
| 29 (warnings audit)    | **OPEN**    | 52 warnings — dead-code cleanup deferred.|
| 30 (CI inclusion)      | **OPEN**    | See below.                               |
| 31 (BlackMage OWASP)   | **ONGOING** | Not a one-shot — rolling responsibility. |
| 32 (rules coverage)    | **DEFERRED**| Awaits OWASP shift cadence.              |

## CI Inclusion (Drain Item 30)

`grep -rn "cmd/waf\|cmd/shield"` across `.github/workflows/` returns nothing.
The only `shield` reference is `shield-ebpf` in `ci-protocol.yml:226` — that
is the eBPF crate, not the WAF userspace binary. **`cmd/waf` is NOT in CI.**
Adding it to `ci.yml` is a small, separate shift; flagged but not done here.

## BlackMage OWASP Pass (Drain Item 31)

Treated as **ongoing**, not a one-shot drain item. WAF rule sets evolve with
the OWASP CRS upstream; cadence belongs in BlackMage's quarterly review, not
the drain backlog. Closing the drain ticket; rolling into BlackMage cadence.

## Action

Close drain items 27, 28, 31 (the latter as ONGOING). Leave 29, 30, 32 OPEN
for a future shift but remove the "rescue" framing — there is nothing to
rescue.

---

*Free to use, free to share.*
