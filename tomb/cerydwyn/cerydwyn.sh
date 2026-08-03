#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# cerydwyn.sh — CLI wrapper that queries Ollama with RAG augmentation from the Grimoire
#
# Usage:
#   ./cerydwyn.sh "What are the attack vectors for the Monad wire protocol?"
#   ./cerydwyn.sh --no-rag "Explain CWE-787 in simple terms"
#   ./cerydwyn.sh --context-file /tmp/crash-report.txt "Analyze this crash"
#   echo "query" | ./cerydwyn.sh --stdin
#
# Environment:
#   OLLAMA_URL       Ollama API endpoint (default: http://127.0.0.1:11434)
#   CERYDWYN_MODEL     Model name (default: cerydwyn-mistral)
#   GRIMOIRE_ROOT    Path to Grimoire knowledge base (default: /opt/tomb/grimoire)
#   CERYDWYN_ROOT      Path to Cerydwyn root (default: /opt/tomb/cerydwyn)

set -euo pipefail

# --- Configuration ---
OLLAMA_URL="${OLLAMA_URL:-http://127.0.0.1:11434}"
CERYDWYN_MODEL="${CERYDWYN_MODEL:-cerydwyn-mistral}"
GRIMOIRE_ROOT="${GRIMOIRE_ROOT:-/opt/tomb/grimoire}"
CERYDWYN_ROOT="${CERYDWYN_ROOT:-/opt/tomb/cerydwyn}"
SYSTEM_PROMPT_FILE="${CERYDWYN_ROOT}/prompts/system-cerydwyn.txt"
LOG_DIR="${CERYDWYN_ROOT}/logs"
TOP_K=5
USE_RAG=true
CONTEXT_FILE=""
STDIN_MODE=false
STREAM=true
VERBOSE=false

# --- Argument Parsing ---
QUERY=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --no-rag)      USE_RAG=false; shift ;;
        --top-k)       TOP_K="$2"; shift 2 ;;
        --model)       CERYDWYN_MODEL="$2"; shift 2 ;;
        --context-file) CONTEXT_FILE="$2"; shift 2 ;;
        --stdin)       STDIN_MODE=true; shift ;;
        --no-stream)   STREAM=false; shift ;;
        --verbose|-v)  VERBOSE=true; shift ;;
        --help|-h)
            cat <<'USAGE'
Usage: cerydwyn.sh [OPTIONS] "query"

Options:
  --no-rag           Skip RAG retrieval, use model knowledge only
  --top-k N          Number of Grimoire chunks to retrieve (default: 5)
  --model NAME       Ollama model name (default: cerydwyn-mistral)
  --context-file F   Inject file contents as additional context
  --stdin            Read query from stdin
  --no-stream        Disable streaming output (wait for full response)
  --verbose, -v      Show RAG retrieval details and timing
  --help, -h         Show this help

Examples:
  cerydwyn.sh "What is the Monad wire format attack surface?"
  cerydwyn.sh --no-rag "Explain buffer overflows"
  cerydwyn.sh --context-file /opt/tomb/lich/crashes/crash-001.txt "Analyze this crash"
  echo "List all LICH modules" | cerydwyn.sh --stdin
USAGE
            exit 0
            ;;
        -*)
            echo "[ERROR] Unknown option: $1" >&2
            exit 1
            ;;
        *)
            QUERY="$1"
            shift
            ;;
    esac
done

# Read from stdin if requested
if [[ "$STDIN_MODE" == "true" ]]; then
    QUERY="$(cat)"
fi

if [[ -z "$QUERY" ]]; then
    echo "[ERROR] No query provided. Use: cerydwyn.sh \"your question\"" >&2
    exit 1
fi

# --- Helpers ---
timestamp() { date '+%Y-%m-%d %H:%M:%S'; }

log_verbose() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo "[$(timestamp)] [RAG] $*" >&2
    fi
}

# --- RAG Retrieval ---
# Simple text-based search when ChromaDB/sentence-transformers are unavailable.
# Falls back to grep-based retrieval across the Grimoire.
rag_retrieve() {
    local query="$1"
    local results=""

    log_verbose "Searching Grimoire for: ${query:0:80}..."

    # Extract keywords from query (strip common words, keep substantive terms)
    local keywords
    keywords=$(echo "$query" | tr '[:upper:]' '[:lower:]' | \
        tr -cs '[:alnum:]' '\n' | \
        grep -vE '^(the|is|are|was|were|what|how|why|when|where|who|which|that|this|with|from|for|and|but|not|can|will|does|has|have|had|been|being|into|over|about|than|then|also|just|only|very|more|most|some|any|all|each|both|few|much|many|such|own|same|other|new|old|out|its|our|his|her|their|a|an|of|in|to|on|at|by|do|if|or|so|no|up|it|we|he|me|my|us)$' | \
        sort -u | head -10)

    if [[ -z "$keywords" ]]; then
        log_verbose "No searchable keywords extracted"
        return
    fi

    log_verbose "Keywords: $(echo $keywords | tr '\n' ' ')"

    # Search across Grimoire directories
    local search_dirs=(
        "${GRIMOIRE_ROOT}/kingdom-docs"
        "${GRIMOIRE_ROOT}/threat-models"
        "${GRIMOIRE_ROOT}/history"
        "${GRIMOIRE_ROOT}/mitre-attck"
        "${GRIMOIRE_ROOT}/nvd-cves"
        "${GRIMOIRE_ROOT}/cwe"
    )

    local found=0
    for dir in "${search_dirs[@]}"; do
        [[ -d "$dir" ]] || continue

        for kw in $keywords; do
            local matches
            matches=$(grep -rl --include='*.md' --include='*.txt' --include='*.json' \
                -i "$kw" "$dir" 2>/dev/null | head -3)

            for match_file in $matches; do
                if [[ $found -ge $TOP_K ]]; then
                    break 3
                fi

                # Extract context around keyword match (5 lines before/after)
                local context
                context=$(grep -i -B 2 -A 5 "$kw" "$match_file" 2>/dev/null | head -20)

                if [[ -n "$context" ]]; then
                    results+="--- [Source: ${match_file}] ---"$'\n'
                    results+="${context}"$'\n'$'\n'
                    ((found++))
                    log_verbose "Retrieved chunk $found from $match_file"
                fi
            done
        done
    done

    if [[ $found -eq 0 ]]; then
        log_verbose "No Grimoire documents matched the query"
    else
        log_verbose "Retrieved $found chunks from Grimoire"
    fi

    echo "$results"
}

# --- Build Prompt ---
build_prompt() {
    local query="$1"
    local rag_context="$2"
    local file_context="$3"
    local prompt=""

    # Load system prompt
    if [[ -f "$SYSTEM_PROMPT_FILE" ]]; then
        prompt+="[System Context]"$'\n'
        prompt+="$(cat "$SYSTEM_PROMPT_FILE")"$'\n'$'\n'
    fi

    # Add RAG context
    if [[ -n "$rag_context" ]]; then
        prompt+="[Grimoire Context — Retrieved Documents]"$'\n'
        prompt+="The following excerpts were retrieved from the Kingdom's knowledge base. Ground your analysis in this context."$'\n'$'\n'
        prompt+="$rag_context"$'\n'$'\n'
    fi

    # Add file context
    if [[ -n "$file_context" ]]; then
        prompt+="[Additional Context — Provided File]"$'\n'
        prompt+="$file_context"$'\n'$'\n'
    fi

    prompt+="[Query]"$'\n'
    prompt+="$query"

    echo "$prompt"
}

# --- Query Ollama ---
query_ollama_stream() {
    local prompt="$1"

    curl -sf "${OLLAMA_URL}/api/generate" \
        -d "$(jq -n \
            --arg model "$CERYDWYN_MODEL" \
            --arg prompt "$prompt" \
            '{model: $model, prompt: $prompt, stream: true}')" \
        2>/dev/null | while IFS= read -r line; do
            local token
            token=$(echo "$line" | jq -r '.response // empty' 2>/dev/null)
            if [[ -n "$token" ]]; then
                printf '%s' "$token"
            fi
            # Check for completion
            local done_flag
            done_flag=$(echo "$line" | jq -r '.done // empty' 2>/dev/null)
            if [[ "$done_flag" == "true" ]]; then
                echo ""  # Final newline
                break
            fi
        done
}

query_ollama_batch() {
    local prompt="$1"

    local response
    response=$(curl -sf "${OLLAMA_URL}/api/generate" \
        -d "$(jq -n \
            --arg model "$CERYDWYN_MODEL" \
            --arg prompt "$prompt" \
            '{model: $model, prompt: $prompt, stream: false}')" \
        2>/dev/null)

    if [[ -z "$response" ]]; then
        echo "[ERROR] No response from Ollama at ${OLLAMA_URL}" >&2
        return 1
    fi

    echo "$response" | jq -r '.response // "No response generated"'
}

# --- Logging ---
log_query() {
    local query="$1"
    local rag_used="$2"

    mkdir -p "$LOG_DIR"

    local logfile
    logfile="${LOG_DIR}/cerydwyn-$(date '+%Y-%m-%d').log"
    {
        echo "---"
        echo "timestamp: $(timestamp)"
        echo "model: ${CERYDWYN_MODEL}"
        echo "rag: ${rag_used}"
        echo "query: ${query:0:200}"
        echo "---"
    } >> "$logfile"
}

# --- Main ---
main() {
    # Check Ollama connectivity
    if ! curl -sf "${OLLAMA_URL}/api/tags" &>/dev/null; then
        echo "[ERROR] Cannot reach Ollama at ${OLLAMA_URL}" >&2
        echo "[HINT]  Start Ollama: sudo systemctl start ollama" >&2
        echo "[HINT]  Or run:       OLLAMA_MODELS=/var/lib/ollama/models /opt/ollama/bin/ollama serve &" >&2
        exit 1
    fi

    # RAG retrieval
    local rag_context=""
    if [[ "$USE_RAG" == "true" ]]; then
        rag_context=$(rag_retrieve "$QUERY")
    fi

    # File context
    local file_context=""
    if [[ -n "$CONTEXT_FILE" ]]; then
        if [[ -f "$CONTEXT_FILE" ]]; then
            file_context=$(cat "$CONTEXT_FILE")
            log_verbose "Loaded context file: $CONTEXT_FILE ($(wc -c < "$CONTEXT_FILE") bytes)"
        else
            echo "[WARN] Context file not found: $CONTEXT_FILE" >&2
        fi
    fi

    # Build prompt
    local prompt
    prompt=$(build_prompt "$QUERY" "$rag_context" "$file_context")

    # Log the query
    log_query "$QUERY" "$USE_RAG"

    # Query Ollama
    if [[ "$STREAM" == "true" ]]; then
        query_ollama_stream "$prompt"
    else
        query_ollama_batch "$prompt"
    fi
}

main
