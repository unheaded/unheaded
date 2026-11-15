#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
# Start Zhenai — WAVE15 post-rewire shape.
#
# Pre-rewire this script launched its own llama-server with Mistral-7B
# on port 20100. Post-rewire (WAVE15, commit 45d7aeb7-era) the inference
# substrate is the SHARED qwen-coder-7b llama-server on :8081 — the same
# binary the Go agent stack uses. We do NOT launch our own llama-server
# from this script. If :8081 isn't up, this script fails fast and tells
# the operator to bring it up via the Go-stack runbook.
#
# Pre-rewire this script also expected a 1.76M-vector FAISS index;
# post-rewire retrieval is via vor (cs serve) at :20110. We do NOT
# launch vor either; same fail-fast posture.
#
# What this script DOES launch:
#   • raft/zhen_app.py on a Unix socket (the Python web UI; no TCP port, no TLS)
#   • bin/zhen-ui-front on :20103 (ADR-138): TLS 1.3 with hybrid ML-KEM key
#     exchange only, proxying to that socket. Left alone when the installed
#     unheaded-zhen-ui-front.service already runs.
#
# Units win. When unheaded-llama.service or unheaded-zhen-app.service is active
# (deploy/systemd/), llama-server and the app are theirs: this script leaves
# them alone and says so, as it does for the front. It then starts only what no
# unit runs.
#
# What it starts, it records: one PID file per process in
# $XDG_RUNTIME_DIR/unheaded-zhen/<name>.pid. The stop line at the end names
# those PIDs, and `systemctl stop` for what a unit runs. Never a kill by
# pattern: the pattern also matches the shell that typed it.
#
# The socket: ZHEN_UI_SOCKET, else /run/unheaded-zhen-ui/app.sock when the host
# has made that directory (deploy/systemd/README.md#zhen-ui-front), else
# $XDG_RUNTIME_DIR/unheaded-zhen-ui/app.sock, mode 0600 in a 0700 directory.
# The front's certificate: UNHEADED_TLS_CERT and UNHEADED_TLS_KEY_FILE, else the
# dev pair from `scripts/gen-dev-certs.sh zhen-ui-front`. There is no plaintext
# mode, in dev too. ZHEN_UI_LISTEN (default 127.0.0.1:20103) is the front's
# address; a name or address past loopback also goes in ZHEN_ALLOWED_HOSTS.
#
# Optional: also start cmd/zhen-agentd if -gated is passed. The chat path
# goes through the daemon's gate (S-CLEAN-SLATE M-036), so without a daemon
# on :20111 chat and mutations answer 503 "the gate is required".
#
# Login: everything but the page, /health and the stylesheet needs one. Set AUTH_ISSUER_URL to
# unheaded-auth (its -audiences must list zhen-agentd; AUTH_BFF_DEV_PLAINTEXT=true
# only for a stack without TLS). The UI sends the person's token to the daemon,
# which verifies it, so the daemon needs AUTH_OIDC_ISSUER_URL too. -gated sets
# that from AUTH_ISSUER_URL. Without an issuer nobody can log in.

set -e

echo "=== Starting Zhenai (WAVE15 post-rewire) ==="

# Phase 5 / WAVE15: context-size config preserved for Python's RAG
# pipeline budget calculations even though we don't launch llama-server.
CONTEXT_SIZE="${ZHEN_CONTEXT_SIZE:-16384}"

# Durable logs (ADR-092): never /tmp.
LOGPATH="$HOME/tmp/unheaded/scripts/lib/logpath.sh"

# PID files for what this script starts, and the stop line built from them.
PIDDIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/unheaded-zhen"
install -d -m 0700 "$PIDDIR"
STOP_HINTS=()

record_pid() {  # record_pid NAME PID
    printf '%s\n' "$2" > "$PIDDIR/$1.pid"
    STOP_HINTS+=("kill \$(cat $PIDDIR/$1.pid)")
}

unit_active() {
    systemctl is-active --quiet "$1" 2>/dev/null
}

# A process this script started on an earlier run and that is still alive.
# The PID file alone is not trusted: the PID must still be that program.
started_earlier() {  # started_earlier NAME PROGRAM-WORD
    local pid
    pid="$(cat "$PIDDIR/$1.pid" 2>/dev/null)" || return 1
    [[ "$pid" =~ ^[0-9]+$ ]] || return 1
    tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | grep -q -- "$2"
}

# What to say about a listener this script did not start this time.
stop_hint_for() {  # stop_hint_for NAME PROGRAM-WORD UNIT
    if unit_active "$3"; then
        STOP_HINTS+=("sudo systemctl stop $3")
    elif started_earlier "$1" "$2"; then
        STOP_HINTS+=("kill \$(cat $PIDDIR/$1.pid)")
    fi
}

# 1. Pre-flight: backends MUST already be up.
echo "[1/4] Pre-flight backend checks..."

if ! curl -sf --max-time 3 http://127.0.0.1:8081/health > /dev/null; then
    echo "  ✗ llama-server :8081 not reachable"
    echo "    Bring it up first. Reference invocation:"
    echo ""
    echo "    cd \$HOME/tmp/unheaded/llama.cpp/build && \\"
    echo "    nohup ./bin/llama-server \\"
    echo "      -m /var/zhen/models/qwen2.5-coder-7b-instruct-q4_k_m.gguf \\"
    echo "      --host 127.0.0.1 --port 8081 \\"
    echo "      --n-gpu-layers 999 --ctx-size $CONTEXT_SIZE \\"
    echo "      --threads 6 \\"
    echo "      &> \"\$(\$HOME/tmp/unheaded/scripts/lib/logpath.sh llama-server)\" &"
    echo ""
    exit 1
fi
if unit_active unheaded-llama.service; then
    echo "  ✓ llama-server :8081 (unheaded-llama.service — leaving alone)"
else
    echo "  ✓ llama-server :8081 (qwen-coder-7b)"
fi

if ! curl -sf --max-time 3 http://127.0.0.1:20110/api/health > /dev/null; then
    echo "  ✗ vor :20110 not reachable"
    echo "    Bring it up first, with swap off so a busy box cannot page its index"
    echo "    out (a search then takes 20 s instead of 0.1 s):"
    echo "      systemd-run --user --scope --collect -p MemorySwapMax=0 --unit=unheaded-vor vor serve 20110 &"
    echo "    Verify the unheaded source symlink:"
    echo "    ls -la ~/.config/vor/sources/unheaded → /home/govan/tmp/unheaded"
    exit 1
fi
echo "  ✓ vor :20110"

# 2. Optional: start zhen-agentd for gated mutations (runbook execute,
#    future kanban-from-chat). Without it the UI is read-only on the
#    runbook surface (returns 503). With it, mutations route through
#    pkg/champion's three-rule gate (T6b closed).
GATED_FLAG="${1:-}"
if [[ "$GATED_FLAG" == "-gated" ]]; then
    if ! ss -lnt | awk '$4 ~ /:20111$/ {f=1} END {exit !f}'; then
        echo "[2/4] Starting cmd/zhen-agentd on :20111 (mutation gating)..."
        cd "$HOME/tmp/unheaded"
        AGENTD_LOG="$("$LOGPATH" zhen-agentd)"
        # zhen-agentd exits 2 without auth (M-019). With a key or an issuer in
        # this environment (AUTH_API_KEYS, AUTH_API_KEY_FILE, AUTH_JWT_KEY or
        # AUTH_ISSUER_URL) it runs authenticated; without one it runs -no-auth,
        # which it allows on loopback only. The UI holds no key of its own: it
        # sends each person's login token.
        AGENTD_AUTH=(-no-auth)
        if [[ -n "${AUTH_ISSUER_URL:-}" ]]; then
            export AUTH_OIDC_ISSUER_URL="${AUTH_OIDC_ISSUER_URL:-$AUTH_ISSUER_URL}"
        fi
        if [[ -n "${AUTH_API_KEYS:-}${AUTH_API_KEY_FILE:-}${AUTH_JWT_KEY:-}${AUTH_OIDC_ISSUER_URL:-}" ]]; then
            export AUTH_ENABLED=true
            AGENTD_AUTH=()
        else
            echo "  ! no AUTH_ISSUER_URL/AUTH_API_KEYS/AUTH_API_KEY_FILE/AUTH_JWT_KEY: zhen-agentd runs -no-auth on 127.0.0.1 (dev only; nobody can log in to the UI)"
        fi
        # bin/release/ is what `make binaries` builds and the units run. The
        # old ./bin/zhen-agentd was whatever was last built by hand.
        if [[ ! -x ./bin/release/zhen-agentd ]]; then
            echo "  ✗ bin/release/zhen-agentd missing: run \`make binaries\` first"
            exit 1
        fi
        nohup ./bin/release/zhen-agentd \
            -host 127.0.0.1 \
            -port 20111 \
            -project-root "$HOME/tmp/unheaded" \
            "${AGENTD_AUTH[@]}" \
            > "$AGENTD_LOG" 2>&1 &
        record_pid zhen-agentd "$!"
        echo "  zhen-agentd PID: $!"
        for _ in $(seq 1 10); do
            curl -sf http://127.0.0.1:20111/health > /dev/null && break
            sleep 1
        done
        if curl -sf http://127.0.0.1:20111/health > /dev/null; then
            echo "  ✓ zhen-agentd ready"
        else
            echo "  ✗ zhen-agentd did not bind in 10s; see $AGENTD_LOG"
            exit 1
        fi
    else
        echo "[2/4] zhen-agentd already running on :20111 — leaving alone"
        stop_hint_for zhen-agentd zhen-agentd zhen-agentd.service
    fi
else
    echo "[2/4] Skipping zhen-agentd: chat and mutations answer 503 until one runs on :20111 ('-gated' starts it)"
fi

# 3. Start the Python web UI on its Unix socket.
REPO="$HOME/tmp/unheaded"
if [[ -z "${ZHEN_UI_SOCKET:-}" ]]; then
    if [[ -d /run/unheaded-zhen-ui && -w /run/unheaded-zhen-ui ]]; then
        ZHEN_UI_SOCKET=/run/unheaded-zhen-ui/app.sock
    else
        ZHEN_UI_SOCKET="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/unheaded-zhen-ui/app.sock"
        install -d -m 0700 "$(dirname "$ZHEN_UI_SOCKET")"
    fi
fi
export ZHEN_UI_SOCKET
# The app refuses to start with the retired TCP setting in its environment.
unset ZHEN_BIND_ADDR

app_answers() {
    curl -s -o /dev/null --max-time 3 --unix-socket "$ZHEN_UI_SOCKET" http://localhost/health
}

if unit_active unheaded-zhen-app.service; then
    echo "[3/4] unheaded-zhen-app.service is running — leaving alone"
    STOP_HINTS+=("sudo systemctl stop unheaded-zhen-app.service")
    if [[ "$ZHEN_UI_SOCKET" != /run/unheaded-zhen-ui/app.sock ]]; then
        echo "  ! the unit serves /run/unheaded-zhen-ui/app.sock, but this run looks at $ZHEN_UI_SOCKET"
    fi
elif app_answers; then
    echo "[3/4] Web UI already listening on $ZHEN_UI_SOCKET — leaving alone"
    stop_hint_for zhen-app zhen_app.py unheaded-zhen-app.service
else
    echo "[3/4] Starting Zhen Web UI on $ZHEN_UI_SOCKET..."
    source "$HOME/.venv/zhen/bin/activate" 2>/dev/null || true
    export ZHEN_LOCAL_MAX_TOKENS="$CONTEXT_SIZE"

    # Defaults the rewired RAGPipeline already prefers; setting them
    # explicitly here makes the deployment posture self-documenting.
    export ZHEN_INFERENCE_URL="${ZHEN_INFERENCE_URL:-http://localhost:8081}"
    export ZHEN_AGENTD_URL="${ZHEN_AGENTD_URL:-http://localhost:20111}"
    export ZHEN_MODEL="${ZHEN_MODEL:-qwen2.5-coder-7b-instruct}"
    export VOR_URL="${VOR_URL:-http://localhost:20110}"
    # Memory-embedder pin (RT-15): RAGPipeline hashes the cached weights and
    # refuses to start when they are not the pinned file.
    # shellcheck source=../scripts/lib/model-pins.sh
    . "$HOME/tmp/unheaded/scripts/lib/model-pins.sh"
    export ZHEN_EMBEDDER_SHA256="${ZHEN_EMBEDDER_SHA256:-$EMBEDDER_SHA256}"
    export ZHEN_EMBEDDER_REVISION="${ZHEN_EMBEDDER_REVISION:-$EMBEDDER_REVISION}"
    if [[ -z "${AUTH_ISSUER_URL:-}" ]]; then
        echo "  ! AUTH_ISSUER_URL unset: nobody can log in, so every route but the page and /health answers 401"
    fi

    # The Well (PostgreSQL) — required for memory cache, conversation log,
    # and live-context kanban/audit/runbook intent injection. Without these,
    # the chat path falls back to vor-only retrieval and the model has to
    # guess at "how many tasks are in progress?" instead of reading PG.
    # Defaults match db/migrations/002_create_databases.sql + 008_zhen_memories.
    # No password default: credentials are never stored in this repo. Unset,
    # zhen_app.py logs "The Well not available" and runs without it.
    export ZHEN_DB_NAME="${ZHEN_DB_NAME:-unheaded_app}"
    export ZHEN_DB_USER="${ZHEN_DB_USER:-app_zhen}"
    if [[ -z "${ZHEN_DB_PASSWORD:-}" ]]; then
        echo "  ! ZHEN_DB_PASSWORD unset: The Well disabled (no memory cache, no conversation log, no live kanban context)"
    fi
    export ZHEN_DB_HOST="${ZHEN_DB_HOST:-localhost}"
    export ZHEN_DB_PORT="${ZHEN_DB_PORT:-5432}"
    WEBAPP_LOG="$("$LOGPATH" zhen zhen-webapp.log)"
    cd "$HOME/tmp/unheaded/raft"
    nohup python3 zhen_app.py &> "$WEBAPP_LOG" &
    WEBAPP_PID=$!
    record_pid zhen-app "$WEBAPP_PID"
    echo "  webapp PID: $WEBAPP_PID"
fi

# 4. The TLS front on :20103. Nothing else reaches the app from a browser.
UI_LISTEN="${ZHEN_UI_LISTEN:-127.0.0.1:20103}"
if systemctl is-active --quiet unheaded-zhen-ui-front.service 2>/dev/null; then
    echo "[4/4] unheaded-zhen-ui-front.service is running — leaving alone"
    STOP_HINTS+=("sudo systemctl stop unheaded-zhen-ui-front.service")
    if [[ "$ZHEN_UI_SOCKET" != /run/unheaded-zhen-ui/app.sock ]]; then
        echo "  ! the unit proxies to /run/unheaded-zhen-ui/app.sock, but the app is on $ZHEN_UI_SOCKET"
    fi
elif ss -lnt 2>/dev/null | awk '$4 ~ /:20103$/ {f=1} END {exit !f}'; then
    echo "[4/4] :20103 already bound — leaving alone"
    stop_hint_for zhen-ui-front zhen-ui-front unheaded-zhen-ui-front.service
else
    echo "[4/4] Starting zhen-ui-front on $UI_LISTEN (hybrid-only TLS 1.3)..."
    FRONT="$REPO/bin/zhen-ui-front"
    if [[ ! -x "$FRONT" ]]; then
        echo "  ✗ $FRONT missing: cd $REPO && make build-zhen-ui-front"
        exit 1
    fi
    FRONT_CERT="${UNHEADED_TLS_CERT:-$REPO/configs/certs/dev/zhen-ui-front/zhen-ui-front.pem}"
    FRONT_KEY="${UNHEADED_TLS_KEY_FILE:-$REPO/configs/certs/dev/zhen-ui-front/zhen-ui-front-key.pem}"
    if [[ ! -r "$FRONT_CERT" || ! -r "$FRONT_KEY" ]]; then
        echo "  ✗ no certificate for the front ($FRONT_CERT)"
        echo "    Make the dev pair, as yourself:  cd $REPO && scripts/gen-dev-certs.sh zhen-ui-front"
        echo "    or set UNHEADED_TLS_CERT and UNHEADED_TLS_KEY_FILE. The front has no plaintext mode."
        exit 1
    fi
    FRONT_LOG="$("$LOGPATH" zhen zhen-ui-front.log)"
    UNHEADED_TLS_CERT="$FRONT_CERT" UNHEADED_TLS_KEY_FILE="$FRONT_KEY" nohup "$FRONT" \
        --listen "$UI_LISTEN" \
        --socket "$ZHEN_UI_SOCKET" \
        --metrics 127.0.0.1:20112 \
        &> "$FRONT_LOG" &
    record_pid zhen-ui-front "$!"
    echo "  front PID: $!"
fi

sleep 3
echo ""
echo "=== Zhenai Status ==="
# The app is probed on its socket and the front on its loopback ops listener:
# neither needs a client that speaks the hybrid groups.
UI_URL="https://localhost:20103"
curl -s http://localhost:8081/v1/models 2>/dev/null \
    | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Inference: {d[\"models\"][0][\"name\"]}')" \
    2>/dev/null || echo "Inference: NOT READY"
curl -s http://localhost:20110/api/health 2>/dev/null \
    | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Retrieval: vor — {d[\"sheets\"]} sheets, {d[\"categories\"]} categories')" \
    2>/dev/null || echo "Retrieval: NOT READY"
curl -s --max-time 5 --unix-socket "$ZHEN_UI_SOCKET" http://localhost/health 2>/dev/null \
    | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Web UI:    {d[\"status\"]} (ready={d[\"rag_ready\"]}, well_connected={d[\"well_connected\"]})')" \
    2>/dev/null || echo "Web UI: NOT READY"
if curl -sf --max-time 5 http://127.0.0.1:20112/ready > /dev/null 2>&1; then
    echo "TLS front: ready (hybrid ML-KEM only; reaches the app's socket)"
else
    echo "TLS front: NOT READY (http://127.0.0.1:20112/ready)"
fi
if ss -lnt 2>/dev/null | awk '$4 ~ /:20111$/ {f=1} END {exit !f}'; then
    curl -s http://localhost:20111/health 2>/dev/null \
        | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Daemon:    {d[\"status\"]} (mutation gating ON)')" \
        2>/dev/null || echo "Daemon: present but not responding"
else
    echo "Daemon:    not running (mutation gating OFF; run with '-gated' to enable)"
fi
echo ""
echo "Browse:        $UI_URL  (the client must offer X25519MLKEM768: OpenSSL 3.5+, a current Firefox or Chrome)"
echo "Login:         POST $UI_URL/auth/login (everything below takes the session it gives)"
echo "Chat API:      POST $UI_URL/api/v1/query"
echo "Runbooks API:  POST $UI_URL/api/v1/runbooks/<n>/execute  (operator login)"
echo "Daemon API:    POST http://localhost:20111/api/v1/tool/exec  (when -gated)"
echo ""
if [[ "${#STOP_HINTS[@]}" -gt 0 ]]; then
    echo "Stop:"
    printf '  %s\n' "${STOP_HINTS[@]}"
else
    echo "Stop:  nothing here was started by this script or by a unit it knows; find the listener with 'ss -ltnp' and stop that PID."
fi
