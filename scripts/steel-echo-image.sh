#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# steel-echo-image.sh — build the S-REAL-STEEL game-day images steel-echo-v1..v4.
#
# One static steel-echo binary per version, on Alpine 3.22 with OpenRC
# (battle plan D-14). v1, v2 and v3 are good builds; v4 is the deliberately
# bad one (STEEL_ERROR_RATE=0.3: 30% of / answer 500, /health stays green).
#
# Three modes:
#   (default)     dry run: print every command, change nothing, contact no LXD.
#   --stage DIR   build the four payloads locally into DIR/<ver>/ (binary,
#                 OpenRC service, conf.d knobs). No LXD contact at all.
#   --apply       stage into a temp dir, then for each version: launch a
#                 builder steel-build-<ver> from the Alpine image, push the
#                 payload, enable the service, stop, publish as alias
#                 steel-echo-<ver>, delete the builder. Needs E2.4.
#
# --apply refuses when an alias steel-echo-<ver> already exists (unless
# --replace) and when any steel-build-* instance is left over. It never
# touches an instance whose name is not steel-build-<ver>.
#
# Usage: scripts/steel-echo-image.sh [--apply | --stage DIR] [--replace]
#          [--project default] [--base images:alpine/3.22] [--versions "v1 v2 v3 v4"]
set -euo pipefail

APPLY=0
STAGE=""
REPLACE=0
PROJECT=default            # D-15: the unheaded project shares default's images
BASE=images:alpine/3.22
VERSIONS="v1 v2 v3 v4"
BAD_VERSION=v4
BAD_ERROR_RATE=0.3

while [ $# -gt 0 ]; do
  case "$1" in
    --apply) APPLY=1 ;;
    --stage) STAGE="${2:?--stage needs a directory}"; shift ;;
    --replace) REPLACE=1 ;;
    --project) PROJECT="${2:?}"; shift ;;
    --base) BASE="${2:?}"; shift ;;
    --versions) VERSIONS="${2:?}"; shift ;;
    -h|--help) sed -n '3,25p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "unknown argument: $1 (try --help)" >&2; exit 2 ;;
  esac
  shift
done

[ "$APPLY" = 1 ] && [ -n "$STAGE" ] && { echo "ERROR: --apply and --stage are separate modes" >&2; exit 2; }
[[ "$PROJECT" =~ ^[a-z][a-z0-9-]{0,62}$ ]] || { echo "ERROR: bad --project" >&2; exit 2; }
[[ "$BASE" =~ ^images:alpine/3\.[0-9]+$ ]] || { echo "ERROR: --base must be images:alpine/3.<n> (D-14)" >&2; exit 2; }
for ver in $VERSIONS; do
  [[ "$ver" =~ ^v[0-9]{1,3}$ ]] || { echo "ERROR: version $ver: want v<n>" >&2; exit 2; }
done

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# knobs <ver>: the conf.d body for a version (exported so OpenRC passes it on).
knobs() {
  if [ "$1" = "$BAD_VERSION" ]; then
    printf 'export STEEL_ERROR_RATE=%s\nexport STEEL_LATENCY_MS=0\n' "$BAD_ERROR_RATE"
  else
    printf 'export STEEL_ERROR_RATE=0\nexport STEEL_LATENCY_MS=0\n'
  fi
}

initd() {
  cat <<'EOF'
#!/sbin/openrc-run
# steel-echo: S-REAL-STEEL game-day workload (cmd/steel-echo)
name="steel-echo"
description="steel-echo game-day workload"
supervisor="supervise-daemon"
command="/usr/local/bin/steel-echo"
command_args="serve --addr :8080"
command_user="steel:steel"
respawn_delay=1
output_logger="logger -t steel-echo -p daemon.info"
error_logger="logger -t steel-echo -p daemon.err"

depend() {
  need net
}
EOF
}

# stage <dir>: build every version's payload into <dir>/<ver>/.
stage() {
  local dir="$1" ver
  command -v go >/dev/null || { echo "ERROR: go not found" >&2; exit 1; }
  for ver in $VERSIONS; do
    install -d -m 0755 "$dir/$ver"
    (cd "$ROOT" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$ver" \
      -o "$dir/$ver/steel-echo" ./cmd/steel-echo)
    initd > "$dir/$ver/steel-echo.initd"
    knobs "$ver" > "$dir/$ver/steel-echo.confd"
    echo "  staged $dir/$ver ($(knobs "$ver" | tr '\n' ' '))"
  done
}

run() {
  printf '  %s\n' "$*"
  if [ "$APPLY" = 1 ]; then "$@"; fi
}

if [ -n "$STAGE" ]; then
  echo "== staging payloads into $STAGE (no LXD contact) =="
  stage "$STAGE"
  exit 0
fi

WORK=""
if [ "$APPLY" = 1 ]; then
  command -v lxc >/dev/null || { echo "ERROR: lxc not found" >&2; exit 1; }
  echo "== applying =="
  if lxc list --all-projects -f csv -c n | grep -q '^steel-build-'; then
    echo "ERROR: a steel-build-* instance is left over; remove it first (lxc list --all-projects | grep steel-build)" >&2
    exit 1
  fi
  for ver in $VERSIONS; do
    if lxc image alias list --project "$PROJECT" -f csv | cut -d, -f1 | grep -qx "steel-echo-$ver" && [ "$REPLACE" != 1 ]; then
      echo "ERROR: alias steel-echo-$ver exists (pass --replace to republish it)" >&2
      exit 1
    fi
  done
  WORK="$(mktemp -d -t steel-echo-image.XXXXXX)"
  trap 'rm -rf "$WORK"' EXIT
  stage "$WORK"
else
  echo "== dry run (pass --apply to build and publish, or --stage DIR to build locally only) =="
  WORK='<stage-dir>'
  for ver in $VERSIONS; do
    echo "  CGO_ENABLED=0 go build -trimpath -ldflags \"-s -w -X main.version=$ver\" -o $WORK/$ver/steel-echo ./cmd/steel-echo"
    echo "  # $WORK/$ver/steel-echo.confd: $(knobs "$ver" | tr '\n' ' ')"
  done
fi

for ver in $VERSIONS; do
  b="steel-build-$ver"          # the only instance names this script ever touches
  [[ "$b" =~ ^steel-build-v[0-9]{1,3}$ ]] || { echo "ERROR: builder name $b" >&2; exit 1; }
  echo "-- $ver --"
  run lxc launch "$BASE" "$b" --project "$PROJECT"
  run lxc exec "$b" --project "$PROJECT" -- adduser -S -D -H -s /sbin/nologin steel
  run lxc file push "$WORK/$ver/steel-echo" "$b/usr/local/bin/steel-echo" --project "$PROJECT" --uid 0 --gid 0 --mode 0755
  run lxc file push "$WORK/$ver/steel-echo.initd" "$b/etc/init.d/steel-echo" --project "$PROJECT" --uid 0 --gid 0 --mode 0755
  run lxc file push "$WORK/$ver/steel-echo.confd" "$b/etc/conf.d/steel-echo" --project "$PROJECT" --uid 0 --gid 0 --mode 0644
  run lxc exec "$b" --project "$PROJECT" -- rc-update add steel-echo default
  run lxc stop "$b" --project "$PROJECT"
  if [ "$REPLACE" = 1 ]; then
    run lxc publish "$b" --project "$PROJECT" --alias "steel-echo-$ver" --reuse
  else
    run lxc publish "$b" --project "$PROJECT" --alias "steel-echo-$ver"
  fi
  run lxc delete "$b" --project "$PROJECT"
done

if [ "$APPLY" = 1 ]; then
  echo "== done: $(for v in $VERSIONS; do printf 'steel-echo-%s ' "$v"; done)=="
fi
