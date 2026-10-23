#!/usr/bin/env bash
set -euo pipefail
SRC=$1
DEST=$2
mkdir -p $DEST
cp $SRC $DEST/
echo "copied $SRC to $DEST"
