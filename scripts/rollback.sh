#!/bin/bash
set -euo pipefail
APP_NAME=${1:?"Usage: rollback.sh <app-name> [revision]"}
REVISION=${2:-""}

echo "=== Rolling back $APP_NAME ==="
if [ -n "$REVISION" ]; then
  argocd app rollback "$APP_NAME" "$REVISION"
else
  echo "Available revisions:"
  argocd app history "$APP_NAME"
  echo ""
  echo "Re-run with: rollback.sh $APP_NAME <revision>"
fi
