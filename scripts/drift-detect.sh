#!/bin/bash
set -euo pipefail
echo "=== Unheaded Drift Detection ==="
echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
echo ""

DRIFT_FOUND=0

for env in dev staging prod; do
  echo "--- Environment: $env ---"
  for layer in networking compute identity observability ebpf; do
    DIR="deploy/tofu/environments/$env/$layer"
    if [ -d "$DIR" ]; then
      echo "  Checking $env/$layer..."
      cd "$DIR"
      terragrunt plan -detailed-exitcode 2>/dev/null
      EXIT_CODE=$?
      if [ $EXIT_CODE -eq 2 ]; then
        echo "  DRIFT DETECTED in $env/$layer"
        DRIFT_FOUND=1
      elif [ $EXIT_CODE -eq 0 ]; then
        echo "  No drift in $env/$layer"
      else
        echo "  Error checking $env/$layer"
      fi
      cd - > /dev/null
    fi
  done
done

if [ $DRIFT_FOUND -eq 1 ]; then
  echo ""
  echo "=== DRIFT DETECTED — Opening GitHub Issue ==="
  exit 2
fi

echo ""
echo "=== All environments clean ==="
