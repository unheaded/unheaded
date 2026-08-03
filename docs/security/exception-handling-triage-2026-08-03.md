<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Exception-handling triage — `BLE001`, 2026-08-03

Phase 8 of the Staging Ladder (`docs/battle-plans/STAGING-LADDER-2026-08-03.md`).
**This document is the deliverable for the remaining work, not scaffolding.** It exists
so the decision about 134 exception handlers is made once, in writing, rather than 134
times inside a diff.

---

## What is already done

Phase 8 had three rules. Two are closed:

| Rule | Before | After | How |
|---|---|---|---|
| `E722` bare `except:` | 11 | **0** | → `except Exception:` — commit `fb5d7091` |
| `S110` `except: pass` | 30 | **0** | logging added, control flow untouched — commit `6618561e` |
| `BLE001` blind `except Exception` | 123 | **134** | **open — this document** |

`BLE001` went *up* because closing `E722` converted 11 bare handlers into
`except Exception:`, which is what `BLE001` counts. That is an honest re-classification,
not a regression: those handlers were always blind, they were previously hidden behind
the more severe finding.

---

## The population, measured

130 of the 134 are in ordinary `.py` files (4 are notebook cells). Grouped by what the
handler body actually does:

| Shape | Count | What it means |
|---|---|---|
| **SILENT** | **71** | Catches everything, neither logs nor re-raises. Returns a fallback or continues. |
| logs / reports | 59 | Catches everything but says so — a log line, a print, a Flask `abort()`. |

By file, the concentration is extreme:

```
62  raft/zhen_app.py            <- 46% of all of them, one Flask app
10  raft/scripts/distill_qa.py
10  raft/scripts/04_integration_tests.py
 6  tomb/cerydwyn/cerydwyn-daemon.py
 6  raft/zhen_mcp_server.py
 6  raft/scripts/19_context_benchmark.py
    ... 18 more files with 1-4 each
```

---

## Why this cannot be bulk-edited

`except Exception:` is not a defect on its own. It is correct in a supervisor loop that
must not die, in a cleanup path, and in a best-effort probe. It is a defect when it
swallows a `KeyError` from a typo and returns a plausible-looking wrong answer.

Distinguishing the two requires reading what each `try` body can actually raise. There is
no autofix for `BLE001`, and `--unsafe-fixes` must not be used on it — the same caution
`next.md` records for `cargo clippy --fix`, which has previously silenced a real bug by
`_`-prefixing live parameters.

**The 59 that log are the low-priority group.** A broad catch that is loud is a design
choice: you can see it fire, and you can narrow it later with evidence. **The 71 silent
ones are the actual finding**, because when one of those fires, the caller receives a
fallback value and nothing anywhere records that it happened.

---

## Recommended disposition, for Stevie to accept or reject

Three options, in the order I would do them.

### Option A — narrow the 71 silent ones only, leave the 59 loud ones annotated

Work the silent group site by site: name the exceptions the `try` body can raise, replace
`except Exception` with those types, and let anything else propagate. Annotate the 59 loud
ones with `# noqa: BLE001` plus a one-line reason, since they are already observable.

*Cost*: the largest of the three, and it is real per-site work — call it a full session
for `zhen_app.py` alone.
*Benefit*: the class of bug this rule exists to catch actually goes away.
**This is what I would do**, and I did not start it unattended because narrowing a
handler is the one change in this whole ladder that can convert a currently-working path
into a crash, and it needs someone who can say "yes, that endpoint is allowed to 500".

### Option B — make every silent handler loud, narrow nothing

Apply the Phase 8b treatment (add `log.debug`/`log.warning`) to the remaining 71, then
`# noqa: BLE001` the lot with a standing reason. `BLE001` goes to zero and can be
ratcheted.

*Cost*: low, mechanical, and I can do it unattended safely — it does not change control
flow anywhere.
*Benefit*: every swallow becomes visible, which is most of the practical value.
*Weakness*: the handlers are still blind. You would be ratcheting a rule you have
suppressed rather than satisfied, which is exactly the "silenced by baseline" pattern the
findings-remediation policy forbids for gitleaks and gosec.

### Option C — exclude `BLE001` by rule ID from the ruff ratchet, leave the code alone

Honest, cheap, and defensible given that ruff's other 20-odd rules are now at or near
zero. Records `BLE001` as a known, unworked backlog rather than pretending it is handled.

*Cost*: none.
*Benefit*: Phase 14 can flip ruff to gating today without this blocking it.
*Weakness*: nothing improves.

**Recommendation: C now, A later.** Take the ratchet win immediately so ruff can go
gating and stop new findings from arriving, then work the 71 silent handlers as a
dedicated, attended piece of work — most naturally as the first real exercise of the
ADR-090 sweep, since 62 of them are in one file that the sweep will read line by line
anyway.

What I would specifically *not* do is Option B dressed as a fix. Suppressing 134 findings
behind `noqa` and calling the rule clean is the thing the ratchet policy exists to
prevent.

---

## If Option A is chosen: the per-site protocol

For each of the 71, in this order:

1. Read the `try` body. List the exceptions it can raise — from the calls it makes, not
   from imagination.
2. Choose one:
   - **Narrow** — replace with those types. Preferred outcome.
   - **Keep, but log** — supervisor and cleanup paths keep the broad catch and gain a
     `log.exception(...)`.
   - **Keep, annotated** — `# noqa: BLE001` with a reason that says what would break if
     it were narrowed. Last resort.
3. Record the row: `file:line | try body calls | can raise | disposition | why`.
4. Commit **per directory**, so each commit is one coherent area a reviewer can hold in
   their head. `raft/zhen_app.py` gets its own commit, probably several.

Gate after each directory: `python3 -m py_compile` on every touched file, plus any test
covering it, plus `go test ./...` unaffected.

---

## Notes carried from doing 8a and 8b

- **`zhen_app.py` had no module logger at all** until Phase 7C. Every handler that
  "logged" was calling `logging.warning(...)` on the root logger, which reconfigures root
  as a side effect on first use. Anything done to the remaining 62 sites in that file
  should use the `log` defined at its top.
- **Three scripts had no logging at all**, only `print()`. They got module loggers in
  Phase 8b. Debug level was chosen deliberately: these handlers sit inside loops over
  hundreds of candidate files where an unreadable file is expected, so printing each one
  would drown the real output.
- **Two notebook sites are legitimately `pass`** — a malformed line in a proc/sysfs table
  is exactly what the handler exists to skip. Annotated, not logged; a notebook has no
  logging setup.
- **`crates/xv6-mbc/upstream/` is vendored** and contributes findings that are not ours
  (including a genuine `F821` NameError in an upstream error path). The Phase 14 ratchet
  should exclude it by path.
