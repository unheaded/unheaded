#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Applies The Well's base schema plus the compliance migrations (013, 014)
# to a throwaway PostgreSQL container (no volume, no network, removed on
# exit) and checks that the constraints refuse what they must: unknown
# fixes in a profile, bindings without a reason, enforcing mode, edits to
# any history or journal row (even by the owner), acceptances over 90 days.
# It never touches the real Well. Needs docker (sudo if not in the group).
#
#   scripts/well/test-compliance-schema.sh [--go] [image]   # default postgres:16-alpine
#
# --go also runs pkg/compliance/well's integration tests against the same
# throwaway database. The container then publishes 5432 on a random
# loopback port (127.0.0.1 only) instead of having no network.
set -euo pipefail

GO=0
if [[ "${1:-}" == --go ]]; then GO=1; shift; fi
IMAGE="${1:-postgres:16-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="wellsql-test-$$"
cleanup() { "${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true; }
trap cleanup EXIT

NET=(--network none)
if ((GO)); then NET=(-p 127.0.0.1::5432); fi
"${DOCKER[@]}" run -d --rm --name "$N" "${NET[@]}" \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="throwaway-$RANDOM" -e POSTGRES_DB=unheaded "$IMAGE" >/dev/null
# Wait on TCP, not the socket: during init the entrypoint runs a temporary
# server on the socket only, before POSTGRES_DB exists.
ready=0
for _ in $(seq 1 60); do
	if "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then ready=1; break; fi
	sleep 1
done
((ready)) || { echo "postgres did not become ready"; exit 1; }

psql_as() { "${DOCKER[@]}" exec -i "$N" psql -X -q -v ON_ERROR_STOP=1 -U "$1" -d "$2"; }
psql_as unheaded unheaded <"$R/db/migrations/002_create_databases.sql" >/dev/null
psql_as unheaded unheaded_ops <"$R/db/migrations/004_ops_schema.sql" >/dev/null
psql_as unheaded unheaded_config <"$R/db/migrations/005_config_schema.sql" >/dev/null
"${DOCKER[@]}" exec -i "$N" psql -X -q -1 -v ON_ERROR_STOP=1 -U unheaded -d unheaded_config <"$R/db/migrations/013_compliance_config.sql"
"${DOCKER[@]}" exec -i "$N" psql -X -q -1 -v ON_ERROR_STOP=1 -U unheaded -d unheaded_ops <"$R/db/migrations/014_compliance_ops.sql"

fails=0
check() { # want(ok|refused) role db label sql
	local want=$1 role=$2 db=$3 label=$4 sql=$5 got=ok
	printf '%s\n' "$sql" | psql_as "$role" "$db" >/dev/null 2>&1 || got=refused
	if [[ "$got" == "$want" ]]; then echo "[PASS] $label ($got)"; else echo "[FAIL] $label: want $want, got $got"; fails=$((fails + 1)); fi
}
H0=$(printf '%064d' 0) H1=$(printf '%064d' 1) H2=$(printf '%064d' 2)
C=unheaded_config O=unheaded_ops
check ok      config_admin $C "admin mirrors a fix" "INSERT INTO compliance_fixes(id,title,class,risk,body,sha256) VALUES ('kmod-cis','t','kmod','none','{}','$H0');"
check refused config_admin $C "profile naming an unknown fix" "INSERT INTO compliance_profiles(name,fixes,updated_by) VALUES ('lab','{kmod-cis,nope}','t');"
check ok      config_admin $C "profiles lab and none" "INSERT INTO compliance_profiles(name,fixes,updated_by) VALUES ('lab','{kmod-cis}','t'),('none','{}','t');"
check refused config_admin $C "profile with all and a list" "INSERT INTO compliance_profiles(name,all_fixes,fixes,updated_by) VALUES ('bad',true,'{kmod-cis}','t');"
check refused config_admin $C "binding without a reason" "INSERT INTO compliance_host_bindings(host,profile,updated_by,change_reason) VALUES ('east','lab','t','');"
check refused config_admin $C "binding in enforcing mode" "INSERT INTO compliance_host_bindings(host,profile,mode,updated_by,change_reason) VALUES ('east','lab','enforcing','t','x');"
check ok      config_admin $C "bind, then switch profile" "INSERT INTO compliance_host_bindings(host,profile,updated_by,change_reason) VALUES ('east','lab','t','seed'); UPDATE compliance_host_bindings SET profile='none', updated_by='t', change_reason='switch' WHERE host='east';"
check refused config_admin $C "delete a binding" "DELETE FROM compliance_host_bindings WHERE host='east';"
check refused config_admin $C "admin edits binding history" "UPDATE compliance_host_bindings_history SET changed_by='mallory';"
check refused unheaded     $C "owner deletes binding history" "DELETE FROM compliance_host_bindings_history;"
check refused unheaded     $C "owner truncates binding history" "TRUNCATE compliance_host_bindings_history;"
check refused config_reader $C "reader writes a profile" "UPDATE compliance_profiles SET fixes='{}';"
check ok      unheaded     $C "switch recorded twice with who and why" "DO \$\$ BEGIN IF (SELECT count(*) FROM compliance_host_bindings_history WHERE changed_by='t' AND change_reason IN ('seed','switch')) <> 2 THEN RAISE EXCEPTION 'history'; END IF; END \$\$;"
J="INSERT INTO compliance_apply_journal(host,host_seq,fix_id,fix_sha256,action,outcome,operator,at,previous_hash,hash) VALUES"
check ok      ops_writer $O "journal append" "$J ('east',1,'kmod-cis','$H0','apply','success','t',now(),'$H0','$H1');"
check refused ops_writer $O "writer edits the journal" "UPDATE compliance_apply_journal SET outcome='failure';"
check refused unheaded   $O "owner deletes the journal" "DELETE FROM compliance_apply_journal;"
check refused unheaded   $O "owner truncates the journal" "TRUNCATE compliance_apply_journal;"
check refused unheaded   $O "owner truncates finding history" "TRUNCATE compliance_findings_history;"
check refused ops_writer $O "duplicate host sequence" "$J ('east',1,'kmod-cis','$H0','apply','success','t',now(),'$H0','$H2');"
F="INSERT INTO compliance_findings(key,severity,class,step,lockout,opened,entry,updated_by,change_reason) VALUES"
check ok      ops_writer $O "finding insert" "$F ('FND-007','medium','app',5,'none','2026-10-01','{}','t','seed');"
check refused ops_writer $O "acceptance over 90 days" "UPDATE compliance_findings SET accepted_on='2026-10-01', accepted_until='2027-01-15', updated_by='t', change_reason='y' WHERE key='FND-007';"
check ok      ops_writer $O "acceptance of 90 days" "UPDATE compliance_findings SET accepted_on='2026-10-01', accepted_until='2026-12-30', updated_by='t', change_reason='y' WHERE key='FND-007';"
check ok      ops_writer $O "resolve a finding" "BEGIN; SET LOCAL compliance.actor='t'; SET LOCAL compliance.reason='evidence passes'; DELETE FROM compliance_findings WHERE key='FND-007'; COMMIT;"
check refused ops_writer $O "writer deletes finding history" "DELETE FROM compliance_findings_history;"
check ok      unheaded   $O "finding history has insert, update, delete" "DO \$\$ BEGIN IF (SELECT string_agg(op, ',' ORDER BY id) FROM compliance_findings_history) <> 'INSERT,UPDATE,DELETE' THEN RAISE EXCEPTION 'history'; END IF; END \$\$;"
check refused ops_writer $O "evidence with an unknown verdict" "INSERT INTO compliance_evidence(source_key,verdict,observed_at) VALUES ('x','maybe',now());"
check refused ops_writer $O "evidence edited" "INSERT INTO compliance_evidence(source_key,verdict,observed_at) VALUES ('x','pass',now()); UPDATE compliance_evidence SET verdict='fail';"

if (( fails > 0 )); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"

if ((GO)); then
	PORT="$("${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"
	# A fresh schema for the Go tests: the checks above left rows behind.
	psql_as unheaded unheaded <<<"DROP DATABASE unheaded_config WITH (FORCE); CREATE DATABASE unheaded_config;" >/dev/null
	psql_as unheaded unheaded_config <"$R/db/migrations/005_config_schema.sql" >/dev/null
	"${DOCKER[@]}" exec -i "$N" psql -X -q -1 -v ON_ERROR_STOP=1 -U unheaded -d unheaded_config <"$R/db/migrations/013_compliance_config.sql"
	psql_as unheaded unheaded <<<"DROP DATABASE unheaded_ops WITH (FORCE); CREATE DATABASE unheaded_ops;" >/dev/null
	psql_as unheaded unheaded_ops <"$R/db/migrations/004_ops_schema.sql" >/dev/null
	"${DOCKER[@]}" exec -i "$N" psql -X -q -1 -v ON_ERROR_STOP=1 -U unheaded -d unheaded_ops <"$R/db/migrations/014_compliance_ops.sql"
	# Passwords below are the dev ones from 002, valid only inside this
	# throwaway container.
	(cd "$R" && WELL_TEST_OPS_DSN="host=127.0.0.1 port=$PORT user=ops_writer password=ops_writer_dev dbname=unheaded_ops sslmode=disable" \
		WELL_TEST_CONFIG_DSN="host=127.0.0.1 port=$PORT user=config_admin password=config_admin_dev dbname=unheaded_config sslmode=disable" \
		go test ./pkg/compliance/well/... -run Integration -count=1 -v | grep -vE '^(=== RUN)')
fi
