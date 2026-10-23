#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Checks unheaded_app.zhen_captures and zhen_capture_labels
# (db/migrations/033) on a throwaway PostgreSQL server bootstrapped by
# db/init.sh from the manifest, never the real Well:
#   1. the bootstrap records 033; both tables and the purge function
#      belong to well_app_owner; app_kanban cannot read captures;
#   2. app_zhen (a real TCP login) inserts a capture; purge_after is set
#      by the trigger (captured_at + 180 days), not by the writer;
#   3. dedupe: a duplicate id is refused; ON CONFLICT DO NOTHING inserts 0;
#   4. a capture over 256 KiB is refused (bytes is generated, CHECKed);
#   5. CHECKs: id not 64 hex, author/backend disagreeing, a bad family,
#      an empty response, an unknown source;
#   6. text is immutable (app_zhen has no grant; the owner is refused by
#      the trigger); promoted moves both ways; quarantined only one way;
#   7. DELETE (app_zhen, owner) and TRUNCATE are refused outside the purge;
#   8. labels: insert works; the same (capture, kind, source_ref) twice is
#      refused, ON CONFLICT DO NOTHING inserts 0; UPDATE and DELETE of a
#      label are refused (app_zhen and owner); an unknown kind is refused;
#   9. zhen_captures_purge() as app_zhen removes exactly the expired
#      unpromoted and the quarantined captures, with their labels, and
#      leaves the rest.
#  10. --with-ingest: cmd/zhen-flywheel's Well store (build tag wellint)
#      against the same server as app_zhen (WELL_TEST_APP_DSN).
#
#   scripts/well/test-zhen-captures.sh [--with-ingest] [image]   # default postgres:16.15-alpine
#
# WELL_MIGRATIONS_DIR (default db/migrations) points the bootstrap at
# another copy of the migrations, so a provocation can be tested.
# Needs docker (sudo if not in the group).
set -euo pipefail

GO=0
if [[ "${1:-}" == --with-ingest ]]; then GO=1; shift; fi
IMAGE="${1:-postgres:16.15-alpine}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
SRC="${WELL_MIGRATIONS_DIR:-$R/db/migrations}"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
N="qa-zhen-captures-$$"
TMP="$(mktemp -d)"
PW="throwaway-$RANDOM$RANDOM"
ZPW="zhen-$RANDOM$RANDOM"
KPW="kanban-$RANDOM$RANDOM"
cleanup() {
	"${DOCKER[@]}" stop "$N" >/dev/null 2>&1 || true
	rm -rf -- "$TMP"
}
trap cleanup EXIT

mkdir -p "$TMP/db"
cp -a "$SRC" "$TMP/db/migrations"
cp "$R/db/init.sh" "$TMP/db/init.sh"
"${DOCKER[@]}" run -d --rm --name "$N" -p 127.0.0.1::5432 \
	-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
	-v "$TMP/db/migrations:/migrations:ro" -v "$TMP/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
	"$IMAGE" >/dev/null
ready=0
for _ in $(seq 1 120); do
	if "${DOCKER[@]}" exec "$N" pg_isready -h 127.0.0.1 -U unheaded -q 2>/dev/null; then ready=1; break; fi
	sleep 1
done
((ready)) || { "${DOCKER[@]}" logs "$N" 2>&1 | tail -20; echo "postgres did not become ready"; exit 1; }
PORT="$("${DOCKER[@]}" port "$N" 5432/tcp | head -1 | sed 's/.*://')"

sq() { "${DOCKER[@]}" exec -i "$N" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d unheaded_app -c "$1"; }
zh() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$ZPW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_zhen -d unheaded_app -c "$1"; }
kb() { "${DOCKER[@]}" exec -i -e PGPASSWORD="$KPW" "$N" psql -X -q -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_kanban -d unheaded_app -c "$1"; }
own() { sq "SET ROLE well_app_owner; $1"; }
# The passwords exist only inside this throwaway container.
sq "ALTER ROLE app_zhen PASSWORD '$ZPW'; ALTER ROLE app_kanban PASSWORD '$KPW';" >/dev/null

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }
ok() { # label cmd...
	local label=$1; shift
	if "$@" >"$TMP/out" 2>&1; then pass "$label"; else fail "$label"; sed 's/^/    /' "$TMP/out" | tail -4; fi
}
refused() { # label cmd...
	local label=$1; shift
	if "$@" >"$TMP/out" 2>&1; then fail "$label: was allowed"; else pass "$label refused"; fi
}
is() { # label want got
	if [[ "$2" == "$3" ]]; then pass "$1"; else fail "$1: want [$2] got [$3]"; fi
}

hex() { printf '%064x' "$1"; } # a 64-hex capture id from a number
cap() { # n [author] [backend] [captured_at] [response] [family] [extra cols] [extra vals]
	local id; id=$(hex "$1")
	echo "INSERT INTO zhen_captures (id, captured_at, source, backend, model, author_kind, task_family, conversation_id,
	        record_id, schema, redaction_version, prompt, response, truncated${7:+, $7})
	      VALUES ('$id', ${4:-now()}, 'zhen-claude', '${3:-local}', 'qwen-coder-14b', '${2:-local}', '${6:-repo-task}',
	              'conv-$1', 'rec-$1', 'zhen-capture/v1', 1, 'prompt $1', '${5-answer $1}', false${8:+, $8})"
}
label() { # n kind ref
	echo "INSERT INTO zhen_capture_labels (capture_id, kind, value, source_ref) VALUES ('$(hex "$1")', '$2', '{}', '$3')"
}

# 1. Ledger, owners, readers.
is "bootstrap recorded 033 in unheaded_app" 1 \
	"$(sq "SELECT count(*) FROM well_schema_migrations WHERE filename = '033_app_zhen_captures.sql'")"
is "tables owned by well_app_owner" "well_app_owner well_app_owner" \
	"$(sq "SELECT string_agg(pg_get_userbyid(relowner), ' ' ORDER BY relname) FROM pg_class WHERE relname IN ('zhen_captures', 'zhen_capture_labels')")"
is "purge function owned by well_app_owner, SECURITY DEFINER" "well_app_owner true" \
	"$(sq "SELECT pg_get_userbyid(proowner) || ' ' || prosecdef FROM pg_proc WHERE proname = 'zhen_captures_purge'")"
refused "app_kanban SELECT zhen_captures" kb "SELECT count(*) FROM zhen_captures"

# 2. Insert as app_zhen; the trigger sets purge_after.
ok "app_zhen INSERT a capture" zh "$(cap 1)"
is "purge_after = captured_at + 180 days" t \
	"$(sq "SELECT purge_after = captured_at + interval '180 days' FROM zhen_captures WHERE id = '$(hex 1)'")"
ok "a writer's own purge_after is overridden" zh "$(cap 2 local local "now()" "answer 2" repo-task purge_after "now() + interval '10 years'")"
is "  ... to captured_at + 180 days" t \
	"$(sq "SELECT purge_after = captured_at + interval '180 days' FROM zhen_captures WHERE id = '$(hex 2)'")"
refused "a capture born promoted" zh "$(cap 3 local local "now()" "answer 3" repo-task promoted true)"

# 3. Dedupe by content hash.
refused "duplicate id" zh "$(cap 1)"
is "ON CONFLICT DO NOTHING inserts 0" "INSERT 0 0" \
	"$("${DOCKER[@]}" exec -i -e PGPASSWORD="$ZPW" "$N" psql -X -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_zhen -d unheaded_app -c "$(cap 1) ON CONFLICT (id) DO NOTHING")"

# 4. Size cap (bytes is generated from the stored text).
refused "a 300 KiB capture" zh "$(cap 4 local local "now()" "' || repeat('x', 300 * 1024) || '")"
ok "a 200 KiB capture" zh "$(cap 5 local local "now()" "' || repeat('x', 200 * 1024) || '")"
is "  ... bytes counts the stored text" t \
	"$(sq "SELECT bytes = octet_length(prompt) + octet_length(response) + octet_length(tool_trace::text) FROM zhen_captures WHERE id = '$(hex 5)'")"

# 5. CHECKs.
refused "id not 64 hex" zh "$(cap 6 | sed "s/'$(hex 6)'/'XYZ'/")"
refused "claude author on the local backend" zh "$(cap 7 claude local)"
refused "human author on the claude backend" zh "$(cap 8 human claude)"
ok "human author, backend none" zh "$(cap 9 human none)"
refused "family with a space" zh "$(cap 10 local local "now()" "a" "repo task")"
refused "empty response" zh "$(cap 11 local local "now()" "")"
refused "unknown source" zh "$(cap 12 | sed "s/'zhen-claude'/'gpt'/")"
refused "run_id not 16 hex" zh "$(cap 13 local local "now()" "a" repo-task run_id "'run-1'")"

# 6. Immutable text; promoted both ways; quarantined one way.
refused "app_zhen UPDATE of response" zh "UPDATE zhen_captures SET response = 'edited' WHERE id = '$(hex 1)'"
refused "owner UPDATE of response" own "UPDATE zhen_captures SET response = 'edited' WHERE id = '$(hex 1)'"
refused "owner UPDATE of author_kind" own "UPDATE zhen_captures SET author_kind = 'human', backend = 'none' WHERE id = '$(hex 1)'"
refused "owner UPDATE of purge_after" own "UPDATE zhen_captures SET purge_after = now() + interval '10 years' WHERE id = '$(hex 1)'"
ok "app_zhen promotes" zh "UPDATE zhen_captures SET promoted = true WHERE id = '$(hex 1)'"
ok "app_zhen un-promotes (export retired)" zh "UPDATE zhen_captures SET promoted = false WHERE id = '$(hex 1)'"
ok "app_zhen quarantines" zh "UPDATE zhen_captures SET quarantined = true WHERE id = '$(hex 9)'"
refused "un-quarantine" zh "UPDATE zhen_captures SET quarantined = false WHERE id = '$(hex 9)'"

# 7. No delete outside the purge.
refused "app_zhen DELETE" zh "DELETE FROM zhen_captures WHERE id = '$(hex 2)'"
refused "owner DELETE" own "DELETE FROM zhen_captures WHERE id = '$(hex 2)'"
refused "owner TRUNCATE" own "TRUNCATE zhen_captures CASCADE"

# 8. Labels.
ok "app_zhen INSERT a label" zh "$(label 1 check_pass zhen_actions:7)"
refused "the same label twice" zh "$(label 1 check_pass zhen_actions:7)"
is "re-labelling with ON CONFLICT DO NOTHING inserts 0" "INSERT 0 0" \
	"$("${DOCKER[@]}" exec -i -e PGPASSWORD="$ZPW" "$N" psql -X -At -v ON_ERROR_STOP=1 -h 127.0.0.1 -U app_zhen -d unheaded_app -c "$(label 1 check_pass zhen_actions:7) ON CONFLICT DO NOTHING")"
ok "another source for the same kind" zh "$(label 1 check_pass zhen_actions:8)"
refused "unknown kind" zh "$(label 1 thumbs_up ui:1)"
refused "label on a missing capture" zh "$(label 999 check_pass x:1)"
refused "app_zhen UPDATE of a label" zh "UPDATE zhen_capture_labels SET kind = 'check_fail'"
refused "owner UPDATE of a label" own "UPDATE zhen_capture_labels SET kind = 'check_fail'"
refused "app_zhen DELETE of a label" zh "DELETE FROM zhen_capture_labels"
refused "owner DELETE of a label" own "DELETE FROM zhen_capture_labels"
refused "owner TRUNCATE of labels" own "TRUNCATE zhen_capture_labels"

# 9. The purge: expired+unpromoted and quarantined go, with their labels.
ok "expired, unpromoted (20)" zh "$(cap 20 local local "now() - interval '200 days'")"
ok "expired, promoted (21)" zh "$(cap 21 local local "now() - interval '200 days'") ; UPDATE zhen_captures SET promoted = true WHERE id = '$(hex 21)'"
ok "fresh (22)" zh "$(cap 22)"
ok "fresh, quarantined (23)" zh "$(cap 23) ; UPDATE zhen_captures SET quarantined = true WHERE id = '$(hex 23)'"
for n in 20 21 22 23; do zh "$(label "$n" judge_pass "eval:$n")" >/dev/null; done
before=$(sq "SELECT count(*) FROM zhen_captures")
is "zhen_captures_purge() as app_zhen deletes 3 (20, 23 and quarantined 9)" 3 "$(zh "SELECT zhen_captures_purge()")"
is "  ... 3 fewer captures" "$((before - 3))" "$(sq "SELECT count(*) FROM zhen_captures")"
is "  ... the expired promoted and the fresh one stay" 2 \
	"$(sq "SELECT count(*) FROM zhen_captures WHERE id IN ('$(hex 21)', '$(hex 22)')")"
is "  ... the expired unpromoted and the quarantined are gone" 0 \
	"$(sq "SELECT count(*) FROM zhen_captures WHERE id IN ('$(hex 20)', '$(hex 23)', '$(hex 9)')")"
is "  ... their labels went with them" 0 \
	"$(sq "SELECT count(*) FROM zhen_capture_labels WHERE capture_id IN ('$(hex 20)', '$(hex 23)', '$(hex 9)')")"
is "  ... the others' labels stay" 2 \
	"$(sq "SELECT count(*) FROM zhen_capture_labels WHERE capture_id IN ('$(hex 21)', '$(hex 22)')")"
refused "the purge flag does not stay on" own "DELETE FROM zhen_captures WHERE id = '$(hex 22)'"

# 10. The Go Well store against this server.
if ((GO)); then
	export WELL_TEST_APP_DSN="postgres://app_zhen:$ZPW@127.0.0.1:$PORT/unheaded_app?sslmode=disable"
	if (cd "$R" && go test -count=1 -tags wellint ./cmd/zhen-flywheel/ -run Well -v >"$TMP/go.out" 2>&1); then
		pass "go test -tags wellint ./cmd/zhen-flywheel -run Well"
	else
		fail "go Well-store test"; tail -25 "$TMP/go.out" | sed 's/^/    /'
	fi
	unset WELL_TEST_APP_DSN
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
