#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
#
# scripts/vor-fuzz.sh — quick parser fuzz of cs/vor's GET /api/search?q=
#
# Probes for: crash conditions, odd HTTP status codes, parser confusion.
# Records every (input, http_code, body_first_120_chars) in a CSV.

set -euo pipefail

VOR_URL="${VOR_URL:-http://127.0.0.1:9876}"
OUT="${OUT:-/tmp/vor-fuzz-results.csv}"

echo "input,http,body_first120" > "$OUT"

# Probe 1: classic injection chars (un-escaped)
for c in ';' '%' '?' '#' '&' '=' '+' '\n' '\r' '\t' '\\' '"' "'" '<' '>' '|' '`' '$' '(' ')' '[' ']' '{' '}'; do
    enc=$(printf '%s' "$c" | python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.stdin.read()))')
    code=$(curl -s -o /tmp/vor-fuzz-body -w "%{http_code}" "$VOR_URL/api/search?q=test${enc}test" 2>/dev/null)
    body=$(head -c 120 /tmp/vor-fuzz-body | tr '\n\r' '  ')
    printf '%s,%s,"%s"\n' "test${c}test" "$code" "$body" >> "$OUT"
done

# Probe 2: very long queries (DoS surface)
for n in 100 1000 10000 100000 1000000; do
    enc=$(python3 -c "print('q' * $n)")
    code=$(curl -s -o /tmp/vor-fuzz-body -w "%{http_code}" --max-time 10 "$VOR_URL/api/search?q=$enc" 2>/dev/null || echo "TIMEOUT")
    body=$(head -c 120 /tmp/vor-fuzz-body | tr '\n\r' '  ')
    printf 'len=%s,%s,"%s"\n' "$n" "$code" "$body" >> "$OUT"
done

# Probe 3: malformed percent encoding
for v in '%' '%g' '%0' '%X' '%FF' '%%' '%00'; do
    code=$(curl -s -o /tmp/vor-fuzz-body -w "%{http_code}" "$VOR_URL/api/search?q=test${v}test" 2>/dev/null)
    body=$(head -c 120 /tmp/vor-fuzz-body | tr '\n\r' '  ')
    printf 'malformed:%s,%s,"%s"\n' "$v" "$code" "$body" >> "$OUT"
done

# Probe 4: missing q param + odd params
for path in "/api/search" "/api/search?" "/api/search?q=" "/api/search?Q=test" "/api/search?q" "/api/search?q=&q=second"; do
    code=$(curl -s -o /tmp/vor-fuzz-body -w "%{http_code}" "${VOR_URL}${path}" 2>/dev/null)
    body=$(head -c 120 /tmp/vor-fuzz-body | tr '\n\r' '  ')
    printf 'path:%s,%s,"%s"\n' "$path" "$code" "$body" >> "$OUT"
done

# Probe 5: path traversal on /api/topics/<name>
for name in '../etc/passwd' '..%2Fetc%2Fpasswd' '%2e%2e%2fetc' 'a/b/c' 'A%00B' 'normal'; do
    code=$(curl -s -o /tmp/vor-fuzz-body -w "%{http_code}" "$VOR_URL/api/topics/$name" 2>/dev/null)
    body=$(head -c 120 /tmp/vor-fuzz-body | tr '\n\r' '  ')
    printf 'topic:%s,%s,"%s"\n' "$name" "$code" "$body" >> "$OUT"
done

# Probe 6: HTTP method confusion
for method in POST PUT DELETE PATCH OPTIONS TRACE; do
    code=$(curl -s -o /tmp/vor-fuzz-body -w "%{http_code}" -X $method "$VOR_URL/api/search?q=test" 2>/dev/null)
    body=$(head -c 120 /tmp/vor-fuzz-body | tr '\n\r' '  ')
    printf 'method:%s,%s,"%s"\n' "$method" "$code" "$body" >> "$OUT"
done

echo "Done. Results: $OUT"
echo ""
echo "=== summary by HTTP code ==="
tail -n +2 "$OUT" | awk -F, '{print $2}' | sort | uniq -c | sort -rn
