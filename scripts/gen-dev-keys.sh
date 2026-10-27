#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# gen-dev-keys.sh: per-service Ed25519 keys and the dev issuer user for the
# compose stack (ADR-103, S-IRON-GATE Phases 2 and 4). One key pair per
# first-party compose service (the services with a `build:`), plus
# unheaded-cli and unheaded-auth, created with unheaded-keygen:
#
#   configs/keys/dev/<identity>.key    private, 0400   (gitignored; never commit)
#   configs/trust.d/dev/<identity>.pub public, 0444    (gitignored; per deployment)
#   configs/auth/dev-users.json        0600, one operator user "dev" (gitignored)
#
# <identity> is the name the binary passes to auth.LoadServiceAuthConfig:
# the compose service name, except cuirass (unheaded-daemon) and auth
# (unheaded-auth). An existing key is never overwritten (rotate with
# unheaded-keygen -rotate). The dev user's password is random, printed once
# ("dev password: ...") and written nowhere else; --reset-users makes a new
# one (the old users file is replaced). Prints the kid table.
#
# Containers run as their own uid (pkg/uids) and read their key (and the
# issuer its users file) through bind mounts that keep the host file's
# owner and mode. Run once with sudo so each file belongs to its service's
# uid; as root the script never runs the Go toolchain, so build first:
#
#   make binaries && sudo scripts/gen-dev-keys.sh
#
# As yourself the files stay yours, which is enough until auth is turned on
# (S-IRON-GATE Phases 5-7).
#
# Usage: scripts/gen-dev-keys.sh [--reset-users] [--keys-dir D] [--trust-dir T] [--users-file F]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT" || exit 2
KEYS_DIR="configs/keys/dev"
TRUST_DIR="configs/trust.d/dev"
USERS_FILE="configs/auth/dev-users.json"
RESET_USERS=0
while [[ $# -gt 0 ]]; do
	case "$1" in
	--keys-dir) KEYS_DIR="$2"; shift 2 ;;
	--trust-dir) TRUST_DIR="$2"; shift 2 ;;
	--users-file) USERS_FILE="$2"; shift 2 ;;
	--reset-users) RESET_USERS=1; shift ;;
	-h|--help) sed -n '2,31p' "$0"; exit 0 ;;
	*) echo "gen-dev-keys: unknown argument $1" >&2; exit 2 ;;
	esac
done

AUTH_UID=16735 # pkg/uids Auth
IS_ROOT=0
[[ "$(id -u)" -eq 0 ]] && IS_ROOT=1

# The tools: $KEYGEN / $AUTHBIN, else built from this tree into a private
# temp dir (as yourself) or taken from bin/release (as root: make binaries).
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
if [[ $IS_ROOT -eq 1 ]]; then
	KEYGEN="${KEYGEN:-bin/release/unheaded-keygen}"
	AUTHBIN="${AUTHBIN:-bin/release/unheaded-auth}"
	for b in "$KEYGEN" "$AUTHBIN"; do
		[[ -x "$b" ]] || { echo "gen-dev-keys: $b missing; run 'make binaries' as yourself first (root does not run the Go toolchain)" >&2; exit 2; }
	done
else
	if [[ -z "${KEYGEN:-}" ]]; then
		KEYGEN="$WORK/unheaded-keygen"
		go build -o "$KEYGEN" ./cmd/unheaded-keygen
	fi
	if [[ -z "${AUTHBIN:-}" ]]; then
		AUTHBIN="$WORK/unheaded-auth"
		go build -o "$AUTHBIN" ./cmd/unheaded-auth
	fi
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
	if [[ $IS_ROOT -eq 1 ]]; then
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

# The dev issuer user: one operator, random password, shown once.
if [[ $RESET_USERS -eq 1 ]]; then
	rm -f "$USERS_FILE"
fi
if [[ ! -e "$USERS_FILE" ]]; then
	mkdir -p "$(dirname "$USERS_FILE")"
	chmod 0700 "$(dirname "$USERS_FILE")"
	pw="$(head -c 96 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 20)"
	[[ ${#pw} -eq 20 ]] || { echo "gen-dev-keys: could not draw a password" >&2; exit 1; }
	printf '%s\n' "$pw" | "$AUTHBIN" user add dev --roles operator --password-stdin -users-file "$USERS_FILE" >/dev/null
	echo "dev password: $pw"
	echo "  (shown once, stored nowhere; scripts/gen-dev-keys.sh --reset-users makes a new one)"
fi
if [[ $IS_ROOT -eq 1 ]]; then
	chown "$AUTH_UID:$AUTH_UID" "$USERS_FILE"
fi
echo "users: $USERS_FILE ($("$AUTHBIN" user list -users-file "$USERS_FILE" 2>/dev/null | wc -l) user(s); restart or HUP the auth service after a reset)"
