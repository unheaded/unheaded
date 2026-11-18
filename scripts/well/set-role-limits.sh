#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Sets The Well's role connection limits from the environment, as
# set-role-passwords.sh sets passwords. The ledger gives each role a default
# (app_zhen 20 in 051, app_auth 40 in 035, log_writer 4 in 048, ...); a
# deployment that runs more or fewer clients changes it here and not in a
# migration (Stevie, 2026-11-17: a value another deployment may want to
# change is not hard-coded).
#
#   <ROLE>_CONNECTION_LIMIT=<n>    the role's name in capitals; n is 1 to
#                                  9999, or -1 for no limit
#
#   APP_ZHEN_CONNECTION_LIMIT=30 scripts/well/set-role-limits.sh
#   set -a; . ./.env; set +a; scripts/well/set-role-limits.sh --print
#
# Only roles whose variable is set are touched. --print shows each change
# and applies none. A limit counts the role's connections in every database,
# and the sum of all pools must still fit max_connections (the budget table
# in docs/architecture/THE_WELL_OPERATIONS.md). A new limit holds for new
# connections; open ones stay.
#
# Exit 0 done, 1 the Well refused or a role does not exist, 2 a value is
# not a limit (nothing was changed).
set -euo pipefail
PRINT=0
case "${1:-}" in
"") ;;
--print) PRINT=1 ;;
*)
	echo "usage: $0 [--print]" >&2
	exit 2
	;;
esac
C="${WELL_CONTAINER:-unheaded-postgres}"

# Every value is checked before any role is changed.
vars=()
while read -r var; do
	[[ -n "$var" ]] || continue
	if [[ ! "${!var}" =~ ^(-1|[1-9][0-9]{0,3})$ ]]; then
		echo "[well] $var: want 1 to 9999, or -1 for no limit; nothing was changed" >&2
		exit 2
	fi
	vars+=("$var")
done < <(compgen -v | grep -E '^[A-Z][A-Z0-9_]*_CONNECTION_LIMIT$' | sort || true)

if [[ ${#vars[@]} -eq 0 ]]; then
	echo "[well] 0 role limit(s) set"
	exit 0
fi

DOCKER=(docker)
docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
SU="$("${DOCKER[@]}" exec "$C" printenv POSTGRES_USER)"

n=0
for var in "${vars[@]}"; do
	role="${var%_CONNECTION_LIMIT}"
	role="${role,,}"
	limit="${!var}"
	# The role and the number go to psql as variables, never as SQL text.
	if [[ "$PRINT" -eq 1 ]]; then
		before="$("${DOCKER[@]}" exec -i "$C" psql -X -q -At -v ON_ERROR_STOP=1 -v role="$role" -U "$SU" -d postgres <<'SQL'
SELECT rolconnlimit FROM pg_roles WHERE rolname = :'role';
SQL
		)" || {
			echo "[well] $role: the Well refused the read" >&2
			exit 1
		}
	else
		before="$("${DOCKER[@]}" exec -i "$C" psql -X -q -At -v ON_ERROR_STOP=1 -v role="$role" -v limit="$limit" -U "$SU" -d postgres <<'SQL'
SELECT rolconnlimit FROM pg_roles WHERE rolname = :'role';
ALTER ROLE :"role" CONNECTION LIMIT :limit;
SQL
		)" || {
			echo "[well] $role: no such role, or the Well refused ($var)" >&2
			exit 1
		}
	fi
	if [[ -z "$before" ]]; then
		echo "[well] $role: no such role ($var)" >&2
		exit 1
	fi
	if [[ "$PRINT" -eq 1 ]]; then
		echo "[well] $role: connection limit $before -> $limit ($var), not applied"
	else
		echo "[well] $role: connection limit $before -> $limit ($var)"
		n=$((n + 1))
	fi
done
echo "[well] $n role limit(s) set"
