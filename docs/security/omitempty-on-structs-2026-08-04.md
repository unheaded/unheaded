<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# `omitempty` on a struct field does nothing — 66 fields across 24 files

**Found:** 2026-08-04, while fixing `services/timeguru` (`081e5152`)
**Status:** one package fixed; **the other 24 files are not**
**Severity:** misleading output, not a logic fault. See "What this is not" below.

## The rule

`encoding/json` omits a field tagged `omitempty` only when its value is the zero
of a *basic* kind — false, 0, "", a nil pointer/interface, or an empty
map/slice/array. **A struct is never "empty" to the encoder.** So

```go
ExpiresAt time.Time `json:"expires_at,omitempty"`
```

always serialises, and an unset value emits

```json
"expires_at": "0001-01-01T00:00:00Z"
```

The tag states an intent the encoder silently ignores. A consumer cannot tell
"no value" from "the year 1", and anything that formats the field renders
January 1st, year 1. `gopkg.in/yaml.v3` and the TOML encoders behave the same way
for struct values.

## Scope

66 fields across 24 files. 55 are `time.Time`; 11 are project structs.

Full list with line numbers, regenerate with the snippet at the bottom:

| File | Count |
|---|---|
| `pkg/container/runtime.go` | 13 |
| `pkg/alerting/alerting.go` | 6 |
| `pkg/deploy/pipeline/artifact.go` | 6 |
| `cmd/dashboard-backend/internal/events/events.go` | 4 |
| `cmd/kanban-app/timeline.go` | 3 |
| `pkg/deploy/pipeline/pipeline.go` | 3 |
| `pkg/deploy/rollback/history.go` | 3 |
| `pkg/events/events.go` | 3 |
| `pkg/secrets/rotation/keymanager.go` | 3 |
| `services/hauberk/hauberk.go` | 3 |
| `pkg/audit/audit.go`, `pkg/audit/query/search.go`, `pkg/deploy/deploy.go`, `pkg/network/policy_controller.go`, `pkg/secrets/secrets.go` | 2 each |
| 9 more files | 1 each |

The 11 non-`time.Time` ones are whole nested config blocks — `ResourceSpec`,
`NetworkSpec`, `SecuritySpec`, `DNSSpec`, `CapSpec`, `RetryPolicy`, `CBPolicy`,
`LoadBalancer`. Those emit a fully-populated tree of zero values where the author
meant to emit nothing, which is noisier than a stray date.

## What this is not

**Not a logic fault.** Go code guarding these fields uses `.IsZero()`, which is
correct and unaffected. The defect lives only at the serialisation boundary,
which is exactly why it survived — every unit test passes.

Two candidates were checked specifically and are **fine**:

- `Secret.IsExpired()` guards `if s.ExpiresAt.IsZero() { return false }`, matching
  its field comment "zero means never".
- `Lease.IsExpired()` has no such guard, but its only construction site always
  sets `ExpiresAt: time.Now().Add(duration)`, and for a lease "no expiry set →
  treat as expired" is fail-closed anyway.

## The fix, and the hazard

Proven in `services/timeguru` (`081e5152`): change the field to `*time.Time`
(or `*T`), so nil is genuinely omitted.

**The hazard is real and bit immediately.** `time.Time.IsZero` has a *value*
receiver, so after the change `!m.ETA.IsZero()` on a nil pointer dereferences nil
and panics:

```
panic: runtime error: invalid memory address or nil pointer dereference
services/timeguru/internal/sync/sync.go:276
```

Every `.IsZero()` guard on a converted field must become a nil check. In timeguru
that was 5 sites for 3 fields. Extrapolating, 66 fields is on the order of 100
call sites — which is why this is recorded rather than done in one unattended
pass. It wants a package at a time, with `go test -race` after each.

## Regenerating the list

```bash
python3 - <<'PY'
import re, subprocess
files = [f for f in subprocess.run(['git','ls-files','*.go'],
         capture_output=True, text=True).stdout.split()
         if '/vendor/' not in f and 'llama.cpp' not in f]
src = {f: open(f, encoding='utf-8', errors='replace').read() for f in files}
kind = {}
for s in src.values():
    for m in re.finditer(r'^type\s+(\w+)\s+(struct\b|\[\]|map\[|\*|chan\b|func\b|interface\b|\w+)', s, re.M):
        kind[m.group(1)] = 'struct' if m.group(2).startswith('struct') else m.group(2)
pat = re.compile(r'^\s*([A-Z]\w*)\s+((?:time\.Time|[A-Z]\w*|\w+\.[A-Z]\w*))\s+`[^`]*omitempty[^`]*`')
for f, s in src.items():
    for i, l in enumerate(s.split('\n'), 1):
        m = pat.match(l)
        if m and (m.group(2) == 'time.Time' or kind.get(m.group(2).split('.')[-1]) == 'struct'):
            print(f"{f}:{i}\t{m.group(1)} {m.group(2)}")
PY
```

Note the heuristic resolves a named type by grepping its `type X ...`
declaration. It classifies `net.IP` and `json.RawMessage` correctly (slice kinds,
where `omitempty` works) and treats an unresolvable name as safe, so it
**under**-reports rather than over-reports. A `go/types`-based check would be
exact and is the right basis if this ever becomes a gate.
