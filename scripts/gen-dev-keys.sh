#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# gen-dev-keys.sh: per-service Ed25519 keys for the compose stack (ADR-103,
# S-IRON-GATE Phase 2). One key pair per first-party compose service (the
# services with a `build:`), plus unheaded-cli and unheaded-auth, created
# with unheaded-keygen:
#
#   configs/keys/dev/<identity>.key   private, 0400   (gitignored; never commit)
#   configs/trust.d/dev/<identity>.pub public, 0444   (gitignored; per deployment)
#
# <identity> is the name the binary passes to auth.LoadServiceAuthConfig:
# the compose service name, except cuirass (unheaded-daemon) and auth
# (unheaded-auth). An existing key is never overwritten (rotate with
# unheaded-keygen -rotate). Prints the kid table.
#
# Containers run as their own uid (pkg/uids) and read their key as a Docker
# secret, which keeps the host file's owner and mode. Run once with sudo
# (`sudo scripts/gen-dev-keys.sh`) to chown each key to its service's uid;
# as yourself the keys stay yours, which is enough until auth is turned on
# (S-IRON-GATE Phases 5-7).
#
# Usage: scripts/gen-dev-keys.sh [--keys-dir D] [--trust-dir T]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT" || exit 2
KEYS_DIR="configs/keys/dev"
TRUST_DIR="configs/trust.d/dev"
while [[ $# -gt 0 ]]; do
	case "$1" in
	--keys-dir) KEYS_DIR="$2"; shift 2 ;;
	--trust-dir) TRUST_DIR="$2"; shift 2 ;;
	-h|--help) sed -n '2,24p' "$0"; exit 0 ;;
	*) echo "gen-dev-keys: unknown argument $1" >&2; exit 2 ;;
	esac
done

# The keygen binary: $KEYGEN, or built from this tree into a private temp dir.
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
if [[ -z "${KEYGEN:-}" ]]; then
	KEYGEN="$WORK/unheaded-keygen"
	go build -o "$KEYGEN" ./cmd/unheaded-keygen
fi

# identity uid, one per line: first-party compose services, then the two
# identities that are not (or not only) compose services.
python3 - docker-compose.yml >"$WORK/ids" <<'PY'
import sys, yaml
identity = {"cuirass": "unheaded-daemon", "auth": "unheaded-auth"}
doc = yaml.safe_load(open(sys.argv[1]))
seen = set()
for name, svc in sorted(doc["services"].items()):
    if "build" not in svc:
        continue
    ident = identity.get(name, name)
    uid = str(svc.get("user", "")).split(":")[0] or "-"
    seen.add(ident)
    print(ident, uid)
for extra in ("unheaded-cli", "unheaded-auth"):
    if extra not in seen:
        print(extra, "16735" if extra == "unheaded-auth" else "-")
PY

printf '%-20s %-16s %s\n' IDENTITY KID OWNER
while read -r ident uid; do
	if [[ ! -e "$KEYS_DIR/$ident.key" ]]; then
		"$KEYGEN" -dir "$KEYS_DIR" -trust-dir "$TRUST_DIR" "$ident" >/dev/null
	fi
	kid="$("$KEYGEN" -dir "$KEYS_DIR" -trust-dir "$TRUST_DIR" -print-kid "$ident")"
	owner="$(id -un)"
	if [[ "$(id -u)" -eq 0 ]]; then
		# The CLI key belongs to whoever ran sudo; service keys to the
		# service's compose uid.
		if [[ "$uid" == "-" ]]; then
			owner="${SUDO_UID:?run with sudo, not as root directly (SUDO_UID names the CLI key owner)}:${SUDO_GID:?}"
		else
			owner="$uid:$uid"
		fi
		chown "$owner" "$KEYS_DIR/$ident.key"
	fi
	printf '%-20s %-16s %s\n' "$ident" "$kid" "$owner"
done <"$WORK/ids"
