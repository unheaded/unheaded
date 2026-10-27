<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# LICH-013: the model selector (`model_switch`)

**Pre-registered:** ADR-060 §"Lich campaign (LICH-013)", 2026-05-04.
**How-to-run written:** 2026-10-27 (S-CHAMPION-HARDENING Step 216).
**State:** not yet run. Nothing below has been measured; the expectations are
ADR-060's, unchanged.

## Target

`POST /api/v1/tool/exec` with `tool=model_switch` on `cmd/zhen-agentd`, which
dispatches to `pkg/champion.ModelSwap`, which runs
`<project_root>/scripts/switch-model.sh <key>`. Also `zhen-cli /model <key>`,
which only sends that request.

## Expectations (ADR-060, unchanged)

| Attack | Expectation |
|---|---|
| Mutation fuzz: byte permutations of valid keys, 1 M requests over 24 h | 100% refused at the gate or the allowlist; zero script invocations |
| Path-traversal grammar: `../`, `..\\`, URL-encoded forms, NUL bytes | same |
| Concurrent swaps: 100 parallel requests | exactly one runs; 99 refused with "another swap is already running" |
| Allowlist drift: a key added to `switch-model.sh` without an ADR-060 T11 amendment | CI fails (not built yet: no check exists) |

Known gaps to record, not fix here: zhen-agentd answers an allowlist refusal
and a refused concurrent swap with HTTP 500 `status: error` (the reason names
the sentinel error), not the 400 and 429 ADR-060 T11 and T13 ask for.

Procedure smoke, 2026-10-27 (not the campaign): steps 2-3 below with 3 refused
keys and 20 parallel `slow` swaps. The refused keys reached the script 0 times;
of the 20, one ran, timed out after 200 ms and rolled back to `fast`, 19 were
refused as "another swap is already running"; `calls.log` held `--json --status`,
`slow`, `--json --status`, `fast`.

## How to run

Never point this at the real project root: there `switch-model.sh` stops
llama-server on :8081 and loads another model. The campaign runs against a
throwaway root whose `switch-model.sh` is a fake that logs every call.

1. Unit coverage that already exists (allowlist, injection, traversal, NUL,
   script hash, concurrency, timeout rollback, metrics):

   ```bash
   go test ./pkg/champion/ -run 'Allowlist|ScriptHash|ConcurrentSwap|ModelSwap|SwapBudget' -race -count=1
   go test ./cmd/zhen-agentd/ -run 'ModelSwap|Models' -race -count=1
   ```

2. A throwaway daemon with the fake script (the same fake as
   `cmd/zhen-agentd/modelswap_test.go`'s `fakeSwitchScript`: keys `fast` and
   `slow`, `slow` never comes up):

   ```bash
   ROOT=$(mktemp -d) && mkdir -p "$ROOT/scripts"
   # paste fakeSwitchScript from cmd/zhen-agentd/modelswap_test.go:
   $EDITOR "$ROOT/scripts/switch-model.sh" && chmod 755 "$ROOT/scripts/switch-model.sh"
   echo fast > "$ROOT/scripts/active"
   go build -o "$ROOT/zhen-agentd" ./cmd/zhen-agentd
   PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
   GPU_LEASE_TIMEOUT=0 "$ROOT/zhen-agentd" -no-auth -host 127.0.0.1 -port "$PORT" \
       -project-root "$ROOT" -rate-limit 0 &
   ```

3. Drive it. Each request is a direct-user call (the CLI's shape):

   ```bash
   swap() {
     curl -s -X POST "http://127.0.0.1:$PORT/api/v1/tool/exec" -H 'Content-Type: application/json' \
       -d "$(jq -cn --arg k "$1" '{tool:"model_switch",args:{key:$k},emitted_by:"lich-013",
             justification:[{topic:"lich-013",source_trust:"direct-user"}]}')"
   }
   swap 'fast; id'; swap '../../etc/passwd'; swap "$(printf 'fast\x01x')"   # refused
   # one runs; no bare `wait` here, it would also wait for the daemon
   for i in $(seq 100); do swap slow & done | sort | uniq -c
   sort "$ROOT/scripts/calls.log" | uniq -c   # only --json --status and allowlisted keys
   kill %1                                    # the daemon
   ```

   The pass condition for the fuzz and traversal rounds is that `calls.log`
   holds only `--json --status` lines and allowlisted keys. For the timeout
   path, `swap slow` returns `status: error` with "rolled back to fast", and
   `curl -s http://127.0.0.1:$PORT/metrics | grep zhen_model_swap` shows the
   failure and the `rolled_back` duration sample.

4. Record results in `results/` with the date, request count, refusals,
   script invocations and any finding; findings go to the register through the
   coordinator, not from here.
