#!/usr/bin/env bash
set -euo pipefail
n=0
while read line; do
  n=$((n + 1))
  printf '%d: %s\n' "$n" "$line"
done < "${1:-/dev/stdin}"
