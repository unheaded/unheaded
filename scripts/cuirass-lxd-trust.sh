#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# cuirass-lxd-trust.sh — give Cuirass a restricted LXD identity for LXD_BACKEND=https.
#
# What it does (each step skips itself when already done):
#   1. creates the LXD project `unheaded` (restricted=true; shares the default
#      project's images and profiles), so Cuirass sees only its own instances
#      and pihole stays invisible to it;
#   2. makes Cuirass a P-384 key and self-signed client certificate;
#   3. trusts that certificate restricted to the `unheaded` project;
#   4. saves LXD's server certificate, which Cuirass pins;
#   5. prints the settings for the daemon.
#
# Dry run by default: prints the commands and changes nothing. --apply runs them.
#
# Usage: scripts/cuirass-lxd-trust.sh [--apply] [--dir /etc/unheaded/cuirass] [--uid 16740] [--url https://host:8443]
set -euo pipefail

APPLY=0
DIR=/etc/unheaded/cuirass
CUIRASS_UID=16740          # pkg/uids: the compose cuirass container's uid
PROJECT=unheaded
URL=""

while [ $# -gt 0 ]; do
  case "$1" in
    --apply) APPLY=1 ;;
    --dir) DIR="$2"; shift ;;
    --uid) CUIRASS_UID="$2"; shift ;;
    --url) URL="$2"; shift ;;
    -h|--help) sed -n '3,18p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1 (try --help)" >&2; exit 2 ;;
  esac
  shift
done

command -v lxc >/dev/null || { echo "ERROR: lxc not found" >&2; exit 1; }
command -v openssl >/dev/null || { echo "ERROR: openssl not found" >&2; exit 1; }
[[ "$CUIRASS_UID" =~ ^[0-9]+$ ]] || { echo "ERROR: --uid must be numeric" >&2; exit 2; }

https_addr="$(lxc config get core.https_address)"
[ -n "$https_addr" ] || { echo "ERROR: LXD has no HTTPS listener (core.https_address is empty); setting one is a separate host decision" >&2; exit 1; }
if [ -z "$URL" ]; then
  port="${https_addr##*:}"
  URL="https://127.0.0.1:${port}"
fi

run() {
  printf '  %s\n' "$*"
  if [ "$APPLY" = 1 ]; then "$@"; fi
}

[ "$APPLY" = 1 ] && echo "== applying ==" || echo "== dry run (pass --apply to make these changes) =="

echo "-- 1. project $PROJECT --"
if lxc project list -f csv | cut -d, -f1 | sed 's/ (current)//' | grep -qx "$PROJECT"; then
  echo "  exists"
else
  run lxc project create "$PROJECT" -c restricted=true -c features.images=false -c features.profiles=false \
    -c "user.unheaded.purpose=Cuirass-managed instances"
fi

echo "-- 2. client key + certificate in $DIR --"
if [ -f "$DIR/client.crt" ] && [ -f "$DIR/client.key" ]; then
  echo "  exist"
else
  run sudo install -d -m 0750 -o root -g "$CUIRASS_UID" "$DIR"
  run sudo openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:secp384r1 -nodes -days 3650 \
    -subj "/CN=cuirass@$(hostname -s)" -keyout "$DIR/client.key" -out "$DIR/client.crt"
  run sudo chown "$CUIRASS_UID:$CUIRASS_UID" "$DIR/client.key" "$DIR/client.crt"
  run sudo chmod 0400 "$DIR/client.key"
  run sudo chmod 0444 "$DIR/client.crt"
fi

echo "-- 3. trust, restricted to $PROJECT --"
if lxc config trust list -f csv | grep -q ",cuirass,"; then
  echo "  a certificate named cuirass is already trusted"
else
  run lxc config trust add "$DIR/client.crt" --name cuirass --restricted --projects "$PROJECT"
fi

echo "-- 4. server certificate to pin --"
if [ -f "$DIR/server.crt" ]; then
  echo "  exists"
else
  echo "  lxc query /1.0 | jq -r .environment.certificate > $DIR/server.crt"
  if [ "$APPLY" = 1 ]; then
    lxc query /1.0 | jq -r .environment.certificate | sudo tee "$DIR/server.crt" >/dev/null
    sudo chmod 0444 "$DIR/server.crt"
  fi
fi

cat <<EOF
-- 5. daemon settings (compose: mount $DIR:$DIR:ro on cuirass) --
  LXD_BACKEND=https
  LXD_URL=$URL
  LXD_PROJECT=$PROJECT
  LXD_CLIENT_CERT=$DIR/client.crt
  LXD_CLIENT_KEY=$DIR/client.key
  LXD_SERVER_CERT=$DIR/server.crt
  (CUIRASS_ENFORCE and CUIRASS_PRUNE stay unset: observe first, read the planned actions)
  In a compose container 127.0.0.1 is the container: add
  extra_hosts: ["host.docker.internal:host-gateway"] and use LXD_URL=https://host.docker.internal:${URL##*:}
  (the pin is the certificate itself, so the name does not matter)

Undo: lxc config trust remove <fingerprint of cuirass>; lxc project delete $PROJECT (only when empty); rm -r $DIR
EOF
