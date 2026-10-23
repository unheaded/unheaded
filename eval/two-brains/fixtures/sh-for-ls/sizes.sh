#!/usr/bin/env bash
set -euo pipefail
for f in $(ls *.log); do
  wc -c "$f"
done
