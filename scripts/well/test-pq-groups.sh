#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# The Well's TLS key exchange, PostgreSQL 16 against 18 (ADR-105, S-QUANTUM-ARMOR
# Phase 8). Throwaway containers only; never touches the real Well.
#
#   scripts/well/test-pq-groups.sh [pg16-image] [pg18-image]
#
# Two servers, both bootstrapped as compose does (db/init.sh, db/migrations,
# the repo's db/postgresql.conf and pg_hba.conf, TLS from make-tls.sh, uid 70,
# read-only root, no capabilities):
#   PG 16  the Well as deployed. ssl_ecdh_curve offers one curve (P-256), so
#          a hybrid-only client is refused and every client gets P-256.
#   PG 18  the same config plus db/postgresql.pg18.conf.d (ssl_groups), loaded
#          the way the upgrade will load it (include_dir). Must negotiate
#          X25519MLKEM768 with a client that offers it; a classical-only
#          client still gets X25519 (deliberate fallback for old libpq).
# Then the upgrade path itself, as runbooks/data/well-pg18-upgrade.yaml runs
# it: a backup set taken from the PG 16 server by scripts/db-backup.sh
# restores on PG 18 (restore-drill.sh, every row count equal), then into a
# new named volume (--into-volume), on which PG 18 starts with the deployed
# config, keeps its migration ledger and serves the rows over hybrid TLS;
# --into-volume refuses a volume that already exists.
#
# Who negotiated what is read twice: from openssl s_client's own report, and
# from the server's ServerHello (key_share extension, plaintext in TLS 1.3)
# by a relay in front of the server, which is the only way to see what a
# libpq client (psql from each image) or a Go client (cmd/well-migrate, lib/pq
# on crypto/tls, as the services connect; skipped without Go) got. Both must
# agree where both exist.
#
# Exits non-zero if PG 18 does not negotiate the hybrid group, or on any
# other failed check. Every container, network and volume it creates is named
# qa-pg18-*, and removed on exit (anonymous volumes go with their container).
set -euo pipefail

PG16_IMAGE="${1:-postgres:16.15-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea}"
PG18_IMAGE="${2:-postgres:18.6-alpine@sha256:898c60cb390d1085bc97bdd6a1ae968757421df394b071848b52b1c09c89d6a5}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
SUDO=(); ((EUID == 0)) || SUDO=(sudo -n)
command -v openssl >/dev/null && command -v python3 >/dev/null || { echo "needs openssl and python3" >&2; exit 2; }
openssl list -kem-algorithms 2>/dev/null | grep -qi mlkem768 || { echo "host openssl has no ML-KEM (needs 3.5+): $(openssl version)" >&2; exit 2; }

ID="$$"
NET="qa-pg18-net-$ID" S16="qa-pg18-s16-$ID" S18="qa-pg18-s18-$ID" SX="qa-pg18-sx-$ID" VOLX="qa-pg18-vol-$ID"
SUBNET_PREFIX="172.31.$((RANDOM % 200 + 20))"
TMP="$(mktemp -d)"
PW="$(openssl rand -hex 16)"   # per run, throwaway server only
cleanup() {
	"${DOCKER[@]}" ps -aq --filter "name=qa-pg18-.*-$ID" | xargs -r "${DOCKER[@]}" rm -f -v >/dev/null 2>&1 || true
	"${DOCKER[@]}" network rm "$NET" >/dev/null 2>&1 || true
	"${DOCKER[@]}" volume rm "$VOLX" >/dev/null 2>&1 || true
	"${SUDO[@]}" rm -rf -- "$TMP"
}
trap cleanup EXIT

fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }

# --- servers ----------------------------------------------------------------
"${DOCKER[@]}" network create --subnet "$SUBNET_PREFIX.0/24" --gateway "$SUBNET_PREFIX.1" "$NET" >/dev/null
mkdir -p "$TMP/conf16" "$TMP/conf18"
for v in 16 18; do
	cp "$R/db/postgresql.conf" "$TMP/conf$v/postgresql.conf"
	sed "s/172\.28\.1\./$SUBNET_PREFIX./g" "$R/db/pg_hba.conf" >"$TMP/conf$v/pg_hba.conf"
done
# Exactly the line the upgrade appends to db/postgresql.conf.
printf "\ninclude_dir = '/etc/postgresql/conf.d'\n" >>"$TMP/conf18/postgresql.conf"
chmod 0644 "$TMP"/conf*/*
"${SUDO[@]}" "$R/scripts/well/make-tls.sh" "$TMP/tls" >/dev/null

start() { # name image confdir [extra docker args...]
	local n="$1" img="$2" conf="$3"; shift 3
	"${DOCKER[@]}" run -d --rm --name "$n" --network "$NET" -p 127.0.0.1::5432 \
		--user 70:70 --read-only --tmpfs /var/run/postgresql --tmpfs /tmp \
		--cap-drop ALL --security-opt no-new-privileges:true \
		-e POSTGRES_USER=unheaded -e POSTGRES_PASSWORD="$PW" -e POSTGRES_DB=postgres \
		-e POSTGRES_INITDB_ARGS=--data-checksums \
		-v "$R/db/migrations:/migrations:ro" -v "$R/db/init.sh:/docker-entrypoint-initdb.d/00-init.sh:ro" \
		-v "$conf/postgresql.conf:/etc/postgresql/postgresql.conf:ro" \
		-v "$conf/pg_hba.conf:/etc/postgresql/pg_hba.conf:ro" \
		-v "$TMP/tls:/etc/postgresql/tls:ro" "$@" \
		"$img" postgres -c config_file=/etc/postgresql/postgresql.conf >/dev/null
}
ip_of() { "${DOCKER[@]}" inspect -f "{{(index .NetworkSettings.Networks \"$NET\").IPAddress}}" "$1"; }
wait_ready() {
	for _ in $(seq 1 120); do
		# Ready once the entrypoint's temporary server is gone and the real one listens on TCP.
		"${DOCKER[@]}" exec "$1" pg_isready -h "$(ip_of "$1")" -U unheaded -d postgres -q 2>/dev/null && return 0
		"${DOCKER[@]}" inspect -f '{{.State.Running}}' "$1" 2>/dev/null | grep -q true || break
		sleep 1
	done
	"${DOCKER[@]}" logs "$1" 2>&1 | tail -15; return 1
}
port_of() { "${DOCKER[@]}" port "$1" 5432/tcp | head -1 | sed 's/.*://'; }
sq() { "${DOCKER[@]}" exec -i "$1" psql -X -q -At -v ON_ERROR_STOP=1 -U unheaded -d "$2" -c "$3"; }

start "$S16" "$PG16_IMAGE" "$TMP/conf16"
start "$S18" "$PG18_IMAGE" "$TMP/conf18" -v "$R/db/postgresql.pg18.conf.d:/etc/postgresql/conf.d:ro"
wait_ready "$S16" || { echo "PG 16 did not start"; exit 1; }
wait_ready "$S18" || { echo "PG 18 did not start with the repo config + include"; exit 1; }
P16="$(port_of "$S16")" P18="$(port_of "$S18")"

# --- observers --------------------------------------------------------------
# relay: server -> client bytes parsed for the ServerHello's key_share group
# (a HelloRetryRequest's group is replaced by the real ServerHello's). Serves
# every connection a client opens (well-migrate opens one per database) until
# none arrives for 2 s. Writes its port to OUTFILE.port, then the distinct
# groups seen (comma-separated, or "none") to OUTFILE.
relay() { # upstream_port outfile
	python3 -I - "$1" "$2" <<'PY' &
import socket, sys, threading
up_port, out = int(sys.argv[1]), sys.argv[2]
NAMES = {0x11EC: "X25519MLKEM768", 0x11EB: "SecP256r1MLKEM768", 0x11ED: "SecP384r1MLKEM1024",
         0x001D: "X25519", 0x0017: "secp256r1", 0x0018: "secp384r1", 0x0202: "MLKEM1024"}
ls = socket.socket(); ls.bind(("127.0.0.1", 0)); ls.listen(8)
open(out + ".port", "w").write(str(ls.getsockname()[1]))
seen, threads = set(), []
def serve(c):
    try:
        u = socket.create_connection(("127.0.0.1", up_port), timeout=30)
    except OSError:
        c.close(); return
    c.settimeout(30); u.settimeout(30)
    group = ["none"]
    t = threading.Thread(target=pump, args=(c, u, False, group)); t.start()
    pump(u, c, True, group); t.join()
    if group[0] != "none":
        seen.add(group[0])
def pump(src, dst, parse, group):
    buf = b""
    started = False
    try:
        while True:
            d = src.recv(65536)
            if not d:
                break
            dst.sendall(d)
            if parse:
                buf += d
                if not started and buf[:1] == b"S":   # PostgreSQL SSLRequest answer
                    buf, started = buf[1:], True
                while len(buf) >= 5:
                    ln = int.from_bytes(buf[3:5], "big")
                    if len(buf) < 5 + ln:
                        break
                    typ, body, buf = buf[0], buf[5:5 + ln], buf[5 + ln:]
                    if typ != 22 or not body or body[0] != 2:
                        continue
                    sh = body[4:]
                    sidl = sh[34]   # after legacy_version(2) + random(32)
                    p = 35 + sidl + 3
                    end = p + 2 + int.from_bytes(sh[p:p + 2], "big")
                    p += 2
                    while p + 4 <= end:
                        et, el = int.from_bytes(sh[p:p + 2], "big"), int.from_bytes(sh[p + 2:p + 4], "big")
                        if et == 0x33:
                            g = int.from_bytes(sh[p + 4:p + 6], "big")
                            group[0] = NAMES.get(g, "0x%04X" % g)  # a later real ServerHello overrides an HRR's
                        p += 4 + el
    except OSError:
        pass
    finally:
        for s in (src, dst):
            try:
                s.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
ls.settimeout(30)   # the first client may take a while (docker run)
while True:
    try:
        c, _ = ls.accept()
    except OSError:
        break
    ls.settimeout(2)
    th = threading.Thread(target=serve, args=(c,)); th.start(); threads.append(th)
for th in threads:
    th.join()
open(out, "w").write(",".join(sorted(seen)) or "none")
PY
	for _ in $(seq 1 50); do [[ -s "$2.port" ]] && return 0; sleep 0.1; done
	return 1
}
# Each observer runs in a $(...) subshell, so a counter would not survive:
# a fresh name per call instead.
relay_file() { mktemp -u "$TMP/relay.XXXXXX"; }
# s_client OFFER PORT -> "openssl=<group|refused> wire=<group|none>"
# OpenSSL 3.5 prints "Negotiated TLS1.3 group: <name>" for KEM groups and
# "Peer Temp Key: X25519, 253 bits" / "ECDH, prime256v1, 256 bits" for ECDH.
via_openssl() {
	local f o og
	f="$(relay_file)"
	relay "$2" "$f"
	o="$(timeout 20 openssl s_client -starttls postgres -connect "127.0.0.1:$(cat "$f.port")" -groups "$1" \
		-CAfile "$TMP/tls/ca.crt" </dev/null 2>&1 || true)"
	wait
	og="$(sed -n 's/^Negotiated TLS1\.3 group: //p' <<<"$o" | grep -v NULL | head -1)"
	if [[ -z "$og" ]]; then
		og="$(sed -n 's/^Peer Temp Key: //p' <<<"$o" | head -1)"
		case "$og" in X25519,*) og=X25519 ;; "ECDH, prime256v1,"*) og=secp256r1 ;; "ECDH, secp384r1,"*) og=secp384r1 ;; "") og=refused ;; esac
	fi
	echo "openssl=$og wire=$(cat "$f")"
}
# psql from IMAGE through the relay -> "wire=<group> ssl=<version>/<ssl_groups or ecdh curve>"
via_libpq() { # image port setting
	local f o
	f="$(relay_file)"
	relay "$2" "$f"
	o="$("${DOCKER[@]}" run --rm --name "qa-pg18-cli-${f##*.}-$ID" --network host -e PGPASSWORD="$PW" -e PGCONNECT_TIMEOUT=10 \
		-v "$TMP/tls/ca.crt:/ca.crt:ro" "$1" \
		psql -X -At "host=127.0.0.1 port=$(cat "$f.port") user=unheaded dbname=postgres sslmode=verify-full sslrootcert=/ca.crt" \
		-c "SELECT version || '/' || current_setting('$3') FROM pg_stat_ssl WHERE pid = pg_backend_pid()" 2>&1 || true)"
	wait
	echo "wire=$(cat "$f") ssl=$o"
}
# The Go services' path (lib/pq on crypto/tls): cmd/well-migrate status
# through the relay -> "wire=<groups> rc=<exit>". Skipped without a Go toolchain.
GO_OK=0
if command -v go >/dev/null && (cd "$R" && go build -o "$TMP/well-migrate" ./cmd/well-migrate) 2>"$TMP/go-build.log"; then GO_OK=1; fi
via_go() { # port
	local f rc=0
	f="$(relay_file)"
	relay "$1" "$f"
	WELL_HOST=127.0.0.1 WELL_PORT="$(cat "$f.port")" WELL_MIGRATE_USER=unheaded WELL_MIGRATE_PASSWORD="$PW" WELL_SSLMODE=require \
		"$TMP/well-migrate" -dir "$R/db/migrations" status >"$f.out" 2>&1 || rc=$?
	wait
	echo "wire=$(cat "$f") rc=$rc"
}
expect() { # label got want
	[[ "$2" == "$3" ]] && pass "$1: $2" || fail "$1: got '$2', want '$3'"
}

echo "PG 16: $PG16_IMAGE"
echo "PG 18: $PG18_IMAGE"
echo "host:  $(openssl version | cut -d' ' -f1-2)"

# --- (1) PG 16: the Well as deployed -----------------------------------------
expect "pg16 server" "$(sq "$S16" postgres "SELECT current_setting('server_version') || ' ecdh=' || current_setting('ssl_ecdh_curve') || ' min=' || current_setting('ssl_min_protocol_version')")" \
	"16.15 ecdh=prime256v1 min=TLSv1.3"
expect "pg16 openssl offer hybrid,X25519,P-256" "$(via_openssl X25519MLKEM768:X25519:P-256 "$P16")" "openssl=secp256r1 wire=secp256r1"
expect "pg16 openssl offer hybrid only (refused)" "$(via_openssl X25519MLKEM768 "$P16")" "openssl=refused wire=none"
expect "pg16 libpq 16 (psql, PG16 image)" "$(via_libpq "$PG16_IMAGE" "$P16" ssl_ecdh_curve)" "wire=secp256r1 ssl=TLSv1.3/prime256v1"
expect "pg16 libpq 18 (psql, PG18 image)" "$(via_libpq "$PG18_IMAGE" "$P16" ssl_ecdh_curve)" "wire=secp256r1 ssl=TLSv1.3/prime256v1"
if ((GO_OK)); then
	expect "pg16 Go lib/pq (well-migrate status, $(go env GOVERSION))" "$(via_go "$P16")" "wire=secp256r1 rc=0"
fi

# --- (2) PG 18 + 10-pq-groups.conf -------------------------------------------
expect "pg18 server" "$(sq "$S18" postgres "SELECT current_setting('server_version') || ' groups=' || current_setting('ssl_groups') || ' min=' || current_setting('ssl_min_protocol_version') || ' checksums=' || current_setting('data_checksums')")" \
	"18.6 groups=X25519MLKEM768:X25519:prime256v1 min=TLSv1.3 checksums=on"
expect "pg18 openssl offer hybrid,X25519,P-256" "$(via_openssl X25519MLKEM768:X25519:P-256 "$P18")" "openssl=X25519MLKEM768 wire=X25519MLKEM768"
expect "pg18 openssl offer hybrid only" "$(via_openssl X25519MLKEM768 "$P18")" "openssl=X25519MLKEM768 wire=X25519MLKEM768"
expect "pg18 openssl offer X25519 only (classical fallback)" "$(via_openssl X25519 "$P18")" "openssl=X25519 wire=X25519"
expect "pg18 openssl offer P-256 only (classical fallback)" "$(via_openssl P-256 "$P18")" "openssl=secp256r1 wire=secp256r1"
expect "pg18 libpq 18 (psql, PG18 image)" "$(via_libpq "$PG18_IMAGE" "$P18" ssl_groups)" "wire=X25519MLKEM768 ssl=TLSv1.3/X25519MLKEM768:X25519:prime256v1"
expect "pg18 libpq 16 (psql, PG16 image)" "$(via_libpq "$PG16_IMAGE" "$P18" ssl_groups)" "wire=X25519MLKEM768 ssl=TLSv1.3/X25519MLKEM768:X25519:prime256v1"
if ((GO_OK)); then
	expect "pg18 Go lib/pq (well-migrate status, $(go env GOVERSION))" "$(via_go "$P18")" "wire=X25519MLKEM768 rc=0"
else
	echo "[SKIP] Go lib/pq client: no Go toolchain ($(head -c 200 "$TMP/go-build.log" 2>/dev/null))"
fi

# --- (3) the ledger bootstraps on 18 -----------------------------------------
for db in unheaded_app unheaded_ops unheaded_config; do
	n="$(sq "$S18" "$db" "SELECT count(*) FROM well_schema_migrations" 2>&1 || true)"
	[[ "$n" =~ ^[1-9][0-9]*$ && "$n" == "$(sq "$S16" "$db" "SELECT count(*) FROM well_schema_migrations")" ]] &&
		pass "pg18 init.sh: $db ledger has the same $n rows as pg16" || fail "pg18 $db ledger: $n"
done

# --- (4) the upgrade path: a PG 16 set restores on PG 18 ---------------------
sq "$S16" unheaded_app "INSERT INTO kanban_tasks (id, title) SELECT 'pq' || g, 'task ' || g FROM generate_series(1, 25) g" >/dev/null
if WELL_CONTAINER="$S16" WELL_BACKUP_DIR="$TMP/backups" "$R/scripts/db-backup.sh" --keep 1 >"$TMP/backup.log" 2>&1; then
	SET="$(find "$TMP/backups" -mindepth 2 -maxdepth 2 -type d -name 'well-*Z' | sort | tail -1)"
	grep -q '^server_version=16' "$SET/cluster.env" && pass "backup set from pg16 ($(grep -c . "$SET/rowcounts.tsv") rowcount lines)" || fail "cluster.env: $(cat "$SET/cluster.env")"
	if out="$(WELL_DRILL_NAME="qa-pg18-drill-$ID" "$R/scripts/well/restore-drill.sh" --image "$PG18_IMAGE" "$SET" 2>&1)"; then
		pass "pg16 set restores on pg18: $(tail -1 <<<"$out" | sed 's/^\[drill\] //')"
	else
		fail "restore of the pg16 set on pg18"; tail -15 <<<"$out"
	fi
	# --- (5) the runbook's data step, rehearsed end to end -----------------
	# Restore into a new named volume, start PG 18 on it exactly as compose
	# will (repo config + include, uid 70, read-only), set passwords, and
	# check the ledger and the data over hybrid TLS.
	if out="$(WELL_DRILL_NAME="qa-pg18-restore-$ID" "$R/scripts/well/restore-drill.sh" --image "$PG18_IMAGE" --into-volume "$VOLX" "$SET" 2>&1)"; then
		pass "restore into new volume: $(grep -c . <<<"$out") lines, $(grep -o 'restore took [0-9]*s' <<<"$out")"
		start "$SX" "$PG18_IMAGE" "$TMP/conf18" -v "$R/db/postgresql.pg18.conf.d:/etc/postgresql/conf.d:ro" -v "$VOLX:/var/lib/postgresql"
		if wait_ready "$SX"; then
			APW="$(openssl rand -hex 12)"
			WELL_CONTAINER="$SX" POSTGRES_PASSWORD="$PW" APP_KANBAN_PASSWORD="$APW" \
				"$R/scripts/well/set-role-passwords.sh" >/dev/null
			PX="$(port_of "$SX")"
			expect "upgraded volume: server" "$(sq "$SX" postgres "SELECT current_setting('server_version') || ' checksums=' || current_setting('data_checksums')")" "18.6 checksums=on"
			if ((GO_OK)); then
				expect "upgraded volume: well-migrate status (ledger intact)" "$(via_go "$PX")" "wire=X25519MLKEM768 rc=0"
			fi
			n="$("${DOCKER[@]}" run --rm --name "qa-pg18-cli-app-$ID" --network host -e PGPASSWORD="$APW" -v "$TMP/tls/ca.crt:/ca.crt:ro" "$PG18_IMAGE" \
				psql -X -At "host=127.0.0.1 port=$PX user=app_kanban dbname=unheaded_app sslmode=verify-full sslrootcert=/ca.crt" \
				-c "SELECT count(*) FROM kanban_tasks WHERE id LIKE 'pq%'" 2>&1 || true)"
			expect "upgraded volume: app_kanban reads its rows over TLS" "$n" "25"
		else
			fail "PG 18 did not start on the restored volume"
		fi
	else
		fail "restore into a new volume"; tail -15 <<<"$out"
	fi
	if WELL_DRILL_NAME="qa-pg18-refuse-$ID" "$R/scripts/well/restore-drill.sh" --image "$PG18_IMAGE" --into-volume "$VOLX" "$SET" >/dev/null 2>&1; then
		fail "--into-volume wrote into an existing volume"
	else
		pass "--into-volume refuses an existing volume"
	fi
else
	fail "db-backup.sh against pg16"; tail -15 "$TMP/backup.log"
fi

if ((fails > 0)); then echo "$fails check(s) failed"; exit 1; fi
echo "all checks passed"
