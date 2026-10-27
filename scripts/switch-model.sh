#!/usr/bin/env bash
# switch-model.sh — atomically swap the inference model.
#
# Stevie's directive (2026-05-04): "unheaded platform should be able to
# interchange different models." This script is the canonical seam for
# that interchange. Every chat path (web UI, CLI, agentd) talks to
# llama-server on :8081 over the OpenAI-compat /v1/chat/completions
# API; swapping the model = killing llama-server and relaunching with
# a different --model. This script does that with health-checked
# rollback on failure.
#
# Usage:
#   ./scripts/switch-model.sh <model-key>
#   ./scripts/switch-model.sh --check <model-key>  # fit check only; swaps nothing
#   ./scripts/switch-model.sh --dry-run <model-key>  # sha256 pin + fit check; swaps nothing
#   ./scripts/switch-model.sh --stop      # stop llama-server on LLAMA_PORT, load nothing
#   ./scripts/switch-model.sh --no-verify <model-key>  # skip the sha256 pin (experiments only; logged)
#   ./scripts/switch-model.sh --status    # what llama-server serves now + the keys; no lease
#   ./scripts/switch-model.sh --json [--status|--list]  # the same as one JSON object
#   ./scripts/switch-model.sh --json --check|--dry-run <model-key>  # verdict as JSON
#
# --json (ADR-060 T19 / Consequences): machine-readable output for
# zhen-agentd, which reads this and never the human lines. One object on
# stdout, the human lines on stderr, the exit code unchanged:
#   {"active":"qwen-7b"|null,"pid":N|null,"port":8081,"gguf":path|null,
#    "models":[{"key","name","file","present","pinned","load_seconds"}],
#    "fit":null|{"key","verdict":"fits"|"refused","weights_mib","need_mib",
#                "budget_mib","cache_ram_mib","ram_avail_mib","vram_free_mib","offload"},
#    "pin":"ok"|"mismatch"|"missing"|"unhashable"|"skipped" (--dry-run only),
#    "decision":"fits"|"would_swap"|"refused_fit"|"refused_pin" (--check/--dry-run)}
# --status/--list take no GPU lease (they read ss and /proc only). --json
# with a real swap is refused (exit 2): a swap's result is its exit code.
#
# Model pins (RT-15): every GGUF's sha256 is pinned in scripts/lib/model-pins.sh.
# A swap (and --dry-run) hashes the file and refuses, exit 4, when the hash
# differs from the pin or the key has no pin, so a GGUF swapped on disk is
# never loaded. Adding a model = add MODEL_FILE/MODEL_FLAGS/MODEL_NAME here and
# its MODEL_SHA256 there. --check stays a fast fit-only answer and hashes
# nothing. --no-verify is for Stevie's local experiments; it is logged to
# stderr and syslog (tag switch-model) and never used by a plan step.
#
# GPU lease (S-TWO-BRAINS Step 233): the whole swap runs under the
# exclusive lease of scripts/lib/gpu-lease.sh, so it never meets an
# inference call (pkg/localllm holds the lease shared per call) or a
# training run (scripts/forge-train.sh). GPU_LEASE_TIMEOUT (default 600 s)
# bounds the wait; exit 75 = the lease was not granted in time.
#
# Defined model keys (extend MODELS in this script to add more):
#   qwen-7b      qwen2.5-coder-7b-instruct  (16k ctx, 4 parallel — light, fast,
#                                            current default; ~6 GB VRAM)
#   deepseek     DeepSeek-Coder-V2-Lite     (16B-MoE, ~10 GB VRAM)
#                                            ⚠ does NOT fit on the 12 GB RX 7700
#                                            XT under zhen_app.py's typical RAG
#                                            prompt sizes (4k ctx prompt
#                                            overflow; 8k ctx OOMs mid-matmul).
#                                            Kept here for the day a 24 GB card
#                                            arrives or zhen_app prompts shrink.
#
# Env overrides per launch:
#   LLAMA_BIN          path to llama-server     (default: ./llama.cpp/build/bin/llama-server)
#   LLAMA_HOST         (default: 127.0.0.1)
#   LLAMA_PORT         (default: 8081)
#   LLAMA_THREADS      (default: 6)
#
# Rollback: on first health-check failure within 90 s the script kills
# the new server and exits non-zero. The previous server is already
# stopped at that point, so manually restart with the prior key.
#
# Note: if any chat clients are mid-stream (zhen_app.py, zhen-cli) they
# will see a connection drop and may surface an error to the operator.
# The swap is not atomic from the user's perspective — there is a
# ~30-90 second gap while the new model loads.

set -euo pipefail

# llama_cache_ram_mib: the --cache-ram a flag string asks for (default 8192).
# shellcheck source=scripts/lib/llama-cache-ram.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/llama-cache-ram.sh"
# MODEL_SHA256 pins and verify_model_pin.
# shellcheck source=scripts/lib/model-pins.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/model-pins.sh"

LLAMA_BIN="${LLAMA_BIN:-/home/govan/tmp/unheaded/llama.cpp/build/bin/llama-server}"
LLAMA_HOST="${LLAMA_HOST:-127.0.0.1}"
LLAMA_PORT="${LLAMA_PORT:-8081}"
# A port, not a pattern: llama_pid matches it inside an awk regex.
[[ "$LLAMA_PORT" =~ ^[0-9]{1,5}$ ]] || { echo "LLAMA_PORT must be a port number, got '$LLAMA_PORT'" >&2; exit 2; }
LLAMA_THREADS="${LLAMA_THREADS:-6}"
MODELS_DIR="${MODELS_DIR:-/var/zhen/models}"

declare -A MODEL_FILE
MODEL_FILE[qwen-7b]="qwen2.5-coder-7b-instruct-q4_k_m.gguf"
MODEL_FILE[deepseek]="DeepSeek-Coder-V2-Lite-Instruct-Q4_K_M.gguf"
# gemma removed 2026-07-31: the E2B GGUF on disk was an 8.7 GiB quant, not the
# 3.2 GB Q4 that ADR-049/WAVE10E actually selected. It did not fit this box's
# 12 GB VRAM and OOM'd the 14 GB host loading it. WAVE16 had already vetted it
# out of the coding gate in favour of qwen-coder-14b. Re-add with a Q4 E2B
# (or on a bigger GPU) — the forge's Gemma 4 support is untouched.
MODEL_FILE[deepseek-cpu]="DeepSeek-Coder-V2-Lite-Instruct-Q4_K_M.gguf"
# WAVE16 candidates downloaded 2026-05-04 — bench-test pending
MODEL_FILE[qwen-coder-14b]="Qwen2.5-Coder-14B-Instruct-Q4_K_M.gguf"

# Per-model launch flags. Different architectures need different settings:
# - deepseek-v2 has MLA attention; flash-attn unsupported on ROCm → can't
#   quantize V cache, must keep parallel slots low to fit the 12 GB VRAM.
# - qwen-7b is dense; comfortable at 16k ctx with 4 parallel slots.
# Every entry states --cache-ram (MiB): llama-server's host-RAM prompt cache,
# 8192 by default, which the fit check counts against MemAvailable. Left at
# the default, a model that fit grew to 9.4 GiB RSS on this 14 GiB host
# (S-CLEAN-SLATE M-037); scripts/lib/switchmodel_fit_test.go checks each
# entry sets it.
declare -A MODEL_FLAGS
MODEL_FLAGS[qwen-7b]="--ctx-size 16384 --cache-ram 2048"
MODEL_FLAGS[deepseek]="--ctx-size 4096 --parallel 1 --cache-type-k q8_0 --cache-ram 1024"
# deepseek-cpu: 27-layer model; first 20 expert layers go to system RAM
# (~6 GB), remaining 7 layers + attention stay on GPU (~5 GB VRAM).
# Speed expectation: 10-15 tok/s vs full-GPU 70 tok/s, but quality +.
MODEL_FLAGS[deepseek-cpu]="--ctx-size 8192 --parallel 1 --cache-type-k q8_0 --n-cpu-moe 20 --cache-ram 1024"
# qwen2.5-coder-14b (dense): partial GPU offload — first 30 of 48 layers on GPU,
# remaining 18 + KV ride RAM. Estimated 5-6 GB VRAM + 4-5 GB RAM.
MODEL_FLAGS[qwen-coder-14b]="--ctx-size 8192 --parallel 1 --n-gpu-layers 30 --cache-ram 1024"

# Friendly model name reported via OpenAI /v1/models. Cosmetic — clients
# can read this back, but llama-server actually serves whatever GGUF is
# loaded. Set ZHEN_MODEL to match for the web UI to display correctly.
declare -A MODEL_NAME
MODEL_NAME[qwen-7b]="qwen2.5-coder-7b-instruct"
MODEL_NAME[deepseek]="deepseek-coder-v2-lite-instruct"
MODEL_NAME[deepseek-cpu]="deepseek-coder-v2-lite-cpu-moe"
MODEL_NAME[qwen-coder-14b]="qwen2.5-coder-14b-instruct"

# Documented load time per key, seconds (ADR-060 T19): zhen-agentd gives a
# swap 4x this, plus the GPU lease wait, then rolls back to the model that
# was serving. Measured 2026-05-04 (ADR-060 Context): qwen-7b ~45 s warm,
# deepseek-cpu ~142 s. deepseek and qwen-coder-14b are estimates (not
# measured here); replace them with a measured "healthy after Ns".
declare -A MODEL_LOAD_SECONDS
MODEL_LOAD_SECONDS[qwen-7b]=45
MODEL_LOAD_SECONDS[deepseek]=90
MODEL_LOAD_SECONDS[deepseek-cpu]=142
MODEL_LOAD_SECONDS[qwen-coder-14b]=120

# PID of the llama-server listening on LLAMA_PORT, or nothing (exit 0 either
# way: under set -e/pipefail a no-match must not end the script).
# The port is matched exactly at the end of ss's local-address field
# (127.0.0.1:8081, [::]:8081, *:8081). Unanchored, ":3901" also matched a
# listener on ":39017", and --stop killed that unrelated process.
llama_pid() {
    { ss -lntp 2>/dev/null || true; } \
        | awk -v p=":${LLAMA_PORT}\$" '$4 ~ p {print $0}' \
        | { grep -oE 'pid=[0-9]+' || true; } | head -1 | cut -d= -f2
}

# Stop the llama-server on LLAMA_PORT and wait until the PROCESS has exited,
# not just until the port closes. llama-server closes its listening socket
# early in shutdown and keeps its VRAM and RAM for seconds afterwards (a 14B
# model took longer than the old 20 s port wait). Launching or fit-checking
# before the exit races that release: the fit check refuses a model that fits,
# or the new server OOMs. TERM, up to 30 s, then KILL, up to 10 s more; fails
# (returns 1) if the process is still alive.
stop_llama() {
    local pid
    pid=$(llama_pid)
    [[ -n "$pid" ]] || return 0
    echo "[switch-model] stopping current llama-server (pid $pid)..."
    kill "$pid" 2>/dev/null || true
    for _ in $(seq 1 30); do
        kill -0 "$pid" 2>/dev/null || return 0
        sleep 1
    done
    echo "[switch-model] pid $pid still running after 30 s; sending KILL" >&2
    kill -KILL "$pid" 2>/dev/null || true
    for _ in $(seq 1 10); do
        kill -0 "$pid" 2>/dev/null || return 0
        sleep 1
    done
    echo "[switch-model] ✗ pid $pid did not exit; not launching over it" >&2
    return 1
}

# model_keys: the MODEL_FILE keys in declaration order (bash associative
# arrays have none; this is the order zhen-agentd's allowlist parser sees).
model_keys() {
    grep -oE '^MODEL_FILE\[[a-z0-9][a-z0-9-]*\]=' "${BASH_SOURCE[0]}" \
        | sed -E 's/^MODEL_FILE\[(.*)\]=$/\1/' | awk '!seen[$0]++'
}

# json_str <s>: s as a JSON string (backslash and quote escaped, control
# characters dropped).
json_str() {
    local s=${1//\\/\\\\}
    s=${s//\"/\\\"}
    s=$(printf '%s' "$s" | tr -d '\001-\037')
    printf '"%s"' "$s"
}

# active_model: ACTIVE_PID, ACTIVE_GGUF and ACTIVE_KEY for the llama-server
# on LLAMA_PORT, read from its /proc cmdline. Two keys can share a GGUF
# (deepseek, deepseek-cpu): the key whose MODEL_FLAGS appear in the command
# line wins, else the first. Empty when nothing listens or it is unreadable.
active_model() {
    ACTIVE_PID=$(llama_pid)
    ACTIVE_GGUF=""
    ACTIVE_KEY=""
    if [[ -z "$ACTIVE_PID" || ! -r "/proc/$ACTIVE_PID/cmdline" ]]; then
        return 0
    fi
    local -a argv=()
    mapfile -d '' argv < "/proc/$ACTIVE_PID/cmdline" 2>/dev/null || true
    local i k first="" joined=" ${argv[*]} "
    for ((i = 0; i + 1 < ${#argv[@]}; i++)); do
        if [[ "${argv[i]}" == --model || "${argv[i]}" == -m ]]; then
            ACTIVE_GGUF=${argv[i+1]}
        fi
    done
    if [[ -z "$ACTIVE_GGUF" ]]; then
        return 0
    fi
    for k in $(model_keys); do
        [[ "$(basename "$ACTIVE_GGUF")" == "${MODEL_FILE[$k]}" ]] || continue
        [[ -n "$first" ]] || first=$k
        if [[ "$joined" == *" ${MODEL_FLAGS[$k]} "* ]]; then
            ACTIVE_KEY=$k
            return 0
        fi
    done
    ACTIVE_KEY=$first
}

# print_status_json [fit-json] [pin] [decision]: the --json object.
print_status_json() {
    active_model
    local k sep="" present pinned active=null gguf=null
    [[ -z "$ACTIVE_KEY" ]] || active=$(json_str "$ACTIVE_KEY")
    [[ -z "$ACTIVE_GGUF" ]] || gguf=$(json_str "$ACTIVE_GGUF")
    printf '{"active":%s,"pid":%s,"port":%s,"gguf":%s,"models":[' \
        "$active" "${ACTIVE_PID:-null}" "$LLAMA_PORT" "$gguf"
    for k in $(model_keys); do
        present=false
        pinned=false
        [[ ! -f "$MODELS_DIR/${MODEL_FILE[$k]}" ]] || present=true
        [[ -z "${MODEL_SHA256[$k]:-}" ]] || pinned=true
        printf '%s{"key":%s,"name":%s,"file":%s,"present":%s,"pinned":%s,"load_seconds":%s}' \
            "$sep" "$(json_str "$k")" "$(json_str "${MODEL_NAME[$k]:-}")" "$(json_str "${MODEL_FILE[$k]}")" \
            "$present" "$pinned" "${MODEL_LOAD_SECONDS[$k]:-0}"
        sep=,
    done
    printf '],"fit":%s' "${1:-null}"
    if [[ -n "${2:-}" ]]; then
        printf ',"pin":%s' "$(json_str "$2")"
    fi
    if [[ -n "${3:-}" ]]; then
        printf ',"decision":%s' "$(json_str "$3")"
    fi
    printf '}\n'
}

# print_status: --status for a person.
print_status() {
    active_model
    local k mark missing
    if [[ -n "$ACTIVE_PID" ]]; then
        echo "llama-server on :$LLAMA_PORT: pid $ACTIVE_PID, ${ACTIVE_KEY:-unknown key} (${ACTIVE_GGUF:-cmdline unreadable})"
    else
        echo "llama-server on :$LLAMA_PORT: not running"
    fi
    for k in $(model_keys); do
        mark=" "
        missing=""
        [[ "$k" != "$ACTIVE_KEY" ]] || mark="*"
        [[ -f "$MODELS_DIR/${MODEL_FILE[$k]}" ]] || missing="  (file missing)"
        printf '%s %-16s %-36s load ~%ss%s\n' "$mark" "$k" "${MODEL_NAME[$k]:-}" "${MODEL_LOAD_SECONDS[$k]:-?}" "$missing"
    done
}

# Read-only modes, before the GPU lease: --status, --list, and --json with
# nothing else. --json elsewhere only with --check or --dry-run.
JSON=0
STATUS=0
WANTS_KEY_MODE=0
for a in "$@"; do
    case "$a" in
        --json) JSON=1 ;;
        --status|--list) STATUS=1 ;;
        --check|--dry-run) WANTS_KEY_MODE=1 ;;
    esac
done
if (( STATUS )) || [[ "$*" == "--json" ]]; then
    if (( JSON )); then
        print_status_json
    else
        print_status
    fi
    exit 0
fi
if (( JSON && ! WANTS_KEY_MODE )); then
    echo "--json works with --status, --list, --check and --dry-run; a swap reports by its exit code" >&2
    exit 2
fi

if [[ "${GPU_LEASE_HELD:-}" != exclusive ]]; then
    exec "$(dirname "${BASH_SOURCE[0]}")/lib/gpu-lease.sh" exclusive "${GPU_LEASE_TIMEOUT:-600}" -- "${BASH_SOURCE[0]}" "$@"
fi

# --json: the object goes to the original stdout (fd 3), everything a
# person reads goes to stderr.
if (( JSON )); then
    exec 3>&1 1>&2
fi

# json_out [fit-json] [pin] [decision]: the --json object, when asked for.
json_out() {
    if (( JSON )); then
        print_status_json "$@" >&3
    fi
}

if [[ "${1:-}" == "--stop" ]]; then
    stop_llama
    exit $?
fi

# --check <key>: run the preflight fit check and exit (0 = fits, 3 = would be
# refused), without stopping or launching anything. Lets the UI or an operator
# ask "would this swap be allowed?" without the 30-90 s outage of trying.
CHECK_ONLY=0
DRY_RUN=0
NO_VERIFY=0
while [[ $# -gt 0 ]]; do
    case "$1" in
        --check) CHECK_ONLY=1; shift ;;
        # --dry-run <key>: the sha256 pin check and the fit check, then exit
        # (0 = would swap, 3 = would not fit, 4 = pin refused). Starts nothing.
        --dry-run) DRY_RUN=1; shift ;;
        --no-verify) NO_VERIFY=1; shift ;;
        --json) shift ;;
        *) break ;;
    esac
done

if [[ $# -lt 1 ]]; then
    echo "usage: $0 [--no-verify] <model-key> | [--json] --check <model-key> | [--json] --dry-run <model-key> | --stop | [--json] --status" >&2
    echo "available keys:" >&2
    for k in "${!MODEL_FILE[@]}"; do
        echo "  $k → $MODELS_DIR/${MODEL_FILE[$k]}" >&2
    done
    exit 2
fi

KEY="$1"
if [[ -z "${MODEL_FILE[$KEY]:-}" ]]; then
    echo "unknown model key: $KEY" >&2
    echo "known: ${!MODEL_FILE[*]}" >&2
    exit 2
fi

GGUF="$MODELS_DIR/${MODEL_FILE[$KEY]}"
if [[ ! -f "$GGUF" ]]; then
    echo "model file missing: $GGUF" >&2
    echo "  download it first; see /var/zhen/models/ for current inventory" >&2
    exit 2
fi

# ─────────────────────── sha256 pin (RT-15) ───────────────────────
# Before anything is stopped, so a refusal leaves the running model up.
PIN_STATE=""
if (( ! CHECK_ONLY )); then
    if (( NO_VERIFY )); then
        PIN_STATE=skipped
        msg="--no-verify: loading '$KEY' ($GGUF) WITHOUT its sha256 pin (user $(id -un))"
        echo "[switch-model] WARNING: $msg" >&2
        logger -t switch-model -- "$msg" 2>/dev/null || true
    else
        echo "[switch-model] verifying sha256 pin for '$KEY'..."
        pin_rc=0
        verify_model_pin "$KEY" "$GGUF" || pin_rc=$?
        if (( pin_rc != 0 )); then
            PIN_STATE=mismatch
            if (( pin_rc == 5 )); then
                PIN_STATE=missing
                echo "[switch-model] ✗ no sha256 pin for $KEY: refusing (add MODEL_SHA256[$KEY] to scripts/lib/model-pins.sh)" >&2
            elif (( pin_rc == 6 )); then
                PIN_STATE=unhashable
                echo "[switch-model] ✗ cannot hash $GGUF for $KEY: refusing" >&2
            else
                echo "[switch-model] ✗ sha256 mismatch for $KEY: refusing ($GGUF is not the pinned file)" >&2
            fi
            json_out null "$PIN_STATE" refused_pin
            exit 4
        fi
        PIN_STATE=ok
        echo "[switch-model] sha256 pin ok"
    fi
fi

FLAGS="${MODEL_FLAGS[$KEY]}"
NAME="${MODEL_NAME[$KEY]}"

# ─────────────────────── Preflight fit check ───────────────────────
# Refuse to launch a model that cannot fit, instead of letting llama-server
# start and take the box down with it. This exists because the UI dropdown is
# a one-click path to loading multi-GB weights: before this, picking a model
# too large for the hardware would thrash into swap and lock the desktop,
# with no way to cancel short of a hard reset. An error message is a much
# better outcome than an unresponsive machine.
#
# The budget is deliberately conservative and deliberately EXCLUDES swap.
# There is 34 GB of swap on this box, which is exactly the resource that
# turns "process gets OOM-killed" (survivable) into "system livelocks"
# (not survivable). Fitting in swap is not fitting.
#
# The estimate is weights + FIT_MARGIN_PCT for KV cache, compute buffers and
# llama.cpp's own overhead, plus the --cache-ram prompt cache, which always
# lives in host RAM (full-GPU or not) and is counted at its cap. It is an
# estimate, not a simulation — the real KV cost depends on ctx-size and the
# architecture's head layout. If it is wrong for a model you know fits, set
# ZHEN_SKIP_FIT_CHECK=1 to bypass.
FIT_MARGIN_PCT="${FIT_MARGIN_PCT:-15}"
# Leave this much VRAM/RAM unclaimed so the desktop and compositor survive.
FIT_RESERVE_BYTES="${FIT_RESERVE_BYTES:-536870912}"  # 512 MiB

if [[ "${ZHEN_SKIP_FIT_CHECK:-0}" != "1" ]]; then
    GGUF_BYTES=$(stat -c %s "$GGUF")
    NEED_BYTES=$(( GGUF_BYTES * (100 + FIT_MARGIN_PCT) / 100 ))

    # Free VRAM on card0. rocm-smi's CSV is the stable interface; the
    # human-readable output has shifted format between ROCm releases.
    VRAM_FREE=0
    if command -v rocm-smi >/dev/null 2>&1; then
        VRAM_FREE=$(rocm-smi --showmeminfo vram --csv 2>/dev/null \
            | awk -F, '$1=="card0" {printf "%d", $2 - $3}')
        [[ -n "$VRAM_FREE" ]] || VRAM_FREE=0
    fi

    # MemAvailable, not MemFree: it accounts for reclaimable page cache.
    # FIT_MEMINFO exists for the tests (a fixed meminfo), never for operators.
    RAM_AVAIL=$(( $(awk '/^MemAvailable:/ {print $2}' "${FIT_MEMINFO:-/proc/meminfo}") * 1024 ))

    if ! CACHE_RAM_MIB=$(llama_cache_ram_mib "$FLAGS"); then
        echo "[switch-model] ✗ REFUSING to load '$KEY': its --cache-ram is unbounded; give it a MiB cap in MODEL_FLAGS." >&2
        json_out null "$PIN_STATE" refused_fit
        exit 3
    fi
    CACHE_RAM_BYTES=$(( CACHE_RAM_MIB * 1024 * 1024 ))

    # The check runs BEFORE the current server is stopped (so a refusal leaves
    # it running), which means free VRAM/RAM above are measured with the old
    # model still loaded. Credit back what stopping it will release, or a swap
    # between two models that each fit alone is refused: 14B -> 7B was, with
    # 5.1 GiB "free" while the 14B held ~6 GiB. VRAM is the KFD figure
    # rocm-smi reports for that pid; RAM is RssAnon only, since file-backed
    # pages stay in the page cache (already counted in MemAvailable).
    RECLAIM_VRAM=0
    RECLAIM_RAM=0
    OLD_PID=$(llama_pid)
    if [[ -n "$OLD_PID" ]]; then
        if command -v rocm-smi >/dev/null 2>&1; then
            RECLAIM_VRAM=$(rocm-smi --showpids --csv 2>/dev/null \
                | awk -F', ' -v p="\"PID${OLD_PID}\"" '$1==p {gsub(/"/,""); print $4}')
            [[ "$RECLAIM_VRAM" =~ ^[0-9]+$ ]] || RECLAIM_VRAM=0
        fi
        RECLAIM_RAM=$(( $(awk '/^RssAnon:/ {print $2}' "/proc/$OLD_PID/status" 2>/dev/null || echo 0) * 1024 ))
        VRAM_FREE=$(( VRAM_FREE + RECLAIM_VRAM ))
        RAM_AVAIL=$(( RAM_AVAIL + RECLAIM_RAM ))
    fi

    # Does this config keep every layer on the GPU? The launch line passes
    # --n-gpu-layers 999, but a per-model --n-gpu-layers or --n-cpu-moe in
    # FLAGS appears later on the command line and wins.
    # The prompt cache is host RAM either way: on its own it must fit in RAM;
    # with partial offload it also shares the VRAM+RAM budget with the
    # weights that spill to RAM.
    if [[ "$FLAGS" == *"--n-cpu-moe"* || "$FLAGS" == *"--n-gpu-layers"* ]]; then
        OFFLOAD="partial"
        BUDGET=$(( VRAM_FREE + RAM_AVAIL - FIT_RESERVE_BYTES ))
        NEED_TOTAL=$(( NEED_BYTES + CACHE_RAM_BYTES ))
    else
        OFFLOAD="full-gpu"
        BUDGET=$(( VRAM_FREE - FIT_RESERVE_BYTES ))
        NEED_TOTAL=$NEED_BYTES
    fi
    RAM_BUDGET=$(( RAM_AVAIL - FIT_RESERVE_BYTES ))

    hb() { numfmt --to=iec-i --suffix=B "$1" 2>/dev/null || echo "${1}B"; }
    mib() { echo $(( $1 / 1048576 )); }
    # fit_json <verdict>: the fit numbers for --json.
    fit_json() {
        printf '{"key":%s,"verdict":"%s","weights_mib":%d,"need_mib":%d,"budget_mib":%d,"cache_ram_mib":%d,"ram_avail_mib":%d,"vram_free_mib":%d,"offload":"%s"}' \
            "$(json_str "$KEY")" "$1" "$(mib "$GGUF_BYTES")" "$(mib "$NEED_TOTAL")" "$(mib "$BUDGET")" \
            "$CACHE_RAM_MIB" "$(mib "$RAM_AVAIL")" "$(mib "$VRAM_FREE")" "$OFFLOAD"
    }

    FIT_LINE="weights_mib=$(mib "$GGUF_BYTES") est_mib=$(mib "$NEED_BYTES") cache_ram_mib=$CACHE_RAM_MIB need_mib=$(mib "$NEED_TOTAL") budget_mib=$(mib "$BUDGET") ram_avail_mib=$(mib "$RAM_AVAIL") vram_free_mib=$(mib "$VRAM_FREE") offload=$OFFLOAD"

    if (( BUDGET <= 0 || NEED_TOTAL > BUDGET || CACHE_RAM_BYTES > RAM_BUDGET )); then
        echo "[switch-model] ✗ REFUSING to load '$KEY' — it will not fit." >&2
        echo "               $FIT_LINE verdict=refused" >&2
        echo "               weights:   $(hb "$GGUF_BYTES")" >&2
        echo "               estimated: $(hb "$NEED_BYTES")  (+${FIT_MARGIN_PCT}% KV/buffers)" >&2
        echo "               cache-ram: $(hb "$CACHE_RAM_BYTES")  (llama-server prompt cache, host RAM; must fit $(hb "$RAM_BUDGET") RAM after reserve)" >&2
        echo "               offload:   $OFFLOAD" >&2
        echo "               free VRAM: $(hb "$VRAM_FREE")" >&2
        echo "               avail RAM: $(hb "$RAM_AVAIL")  (swap deliberately not counted)" >&2
        echo "               budget:    $(hb "$BUDGET")  (after $(hb "$FIT_RESERVE_BYTES") reserve)" >&2
        if [[ -n "$OLD_PID" ]]; then
            echo "               (free VRAM/RAM include $(hb "$RECLAIM_VRAM") / $(hb "$RECLAIM_RAM") the running pid $OLD_PID releases)" >&2
        fi
        echo "" >&2
        echo "  Loading it anyway would swap-thrash and can lock up the desktop." >&2
        echo "  Use a smaller quant, or free VRAM/RAM and retry." >&2
        echo "  To override if you are sure: ZHEN_SKIP_FIT_CHECK=1 $0 $KEY" >&2
        json_out "$(fit_json refused)" "$PIN_STATE" refused_fit
        exit 3
    fi

    echo "[switch-model] fit ok: need $(hb "$NEED_TOTAL") of $(hb "$BUDGET") ($OFFLOAD), cache-ram $(hb "$CACHE_RAM_BYTES") of $(hb "$RAM_BUDGET") RAM"
    echo "[switch-model] $FIT_LINE verdict=fits"
    FIT_JSON=$(fit_json fits)
    if [[ -n "$OLD_PID" ]]; then
        echo "[switch-model]   incl. $(hb "$RECLAIM_VRAM") VRAM + $(hb "$RECLAIM_RAM") RAM released by stopping pid $OLD_PID"
    fi
fi

if (( CHECK_ONLY )); then
    json_out "${FIT_JSON:-null}" "" fits
    exit 0
fi
if (( DRY_RUN )); then
    echo "[switch-model] dry run: would swap to '$KEY' ($NAME); nothing stopped or started"
    json_out "${FIT_JSON:-null}" "$PIN_STATE" would_swap
    exit 0
fi

echo "[switch-model] swapping to '$KEY' ($NAME)"
echo "[switch-model]   gguf:  $GGUF"
echo "[switch-model]   flags: $FLAGS"
echo "[switch-model]   port:  $LLAMA_PORT"

stop_llama || exit 1

# Launch the new model in the background.
LOG="/tmp/llama-${KEY}.log"
echo "[switch-model] launching... (log: $LOG)"
# shellcheck disable=SC2086 # FLAGS is intentionally word-split
nohup "$LLAMA_BIN" \
    --model "$GGUF" \
    --host "$LLAMA_HOST" \
    --port "$LLAMA_PORT" \
    --n-gpu-layers 999 \
    --threads "$LLAMA_THREADS" \
    $FLAGS \
    > "$LOG" 2>&1 &
NEW_PID=$!
disown

# Wait for /health 200, with rollback on death or timeout.
for i in $(seq 1 360); do
    if ! kill -0 "$NEW_PID" 2>/dev/null; then
        echo "[switch-model] ✗ process died at ${i}s — see $LOG" >&2
        tail -20 "$LOG" >&2
        exit 1
    fi
    code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 1 "http://${LLAMA_HOST}:${LLAMA_PORT}/health" 2>/dev/null || echo "000")
    if [[ "$code" == "200" ]]; then
        echo "[switch-model] ✓ healthy after ${i}s"
        echo "[switch-model]   pid=$NEW_PID  port=$LLAMA_PORT  model=$NAME"
        echo ""
        echo "Update web UI to display the new model name:"
        echo "  export ZHEN_MODEL='$NAME' and restart raft/zhen_app.py"
        exit 0
    fi
    sleep 1
done

echo "[switch-model] ✗ /health never returned 200 within 360s" >&2
tail -20 "$LOG" >&2
kill "$NEW_PID" 2>/dev/null || true
exit 1
