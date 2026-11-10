#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Checks scripts/wireguard/rotate-psk.sh (and, at the end, psk-gate.sh, east's
# forced command for the rotation key) against stub wg/ssh/sudo/logger/ping
# on PATH, with two temporary directories standing in for west's and east's
# /etc/wireguard. No root, no network, no real keys: every key is a
# throwaway made here. The stubs record every call, and the run fails if any
# key ever reached a command line or the script's output.
#
#   scripts/wireguard/test-rotate-psk.sh
set -euo pipefail

R="$(cd "$(dirname "$0")/../.." && pwd)"
ROT="$R/scripts/wireguard/rotate-psk.sh"
T="$(mktemp -d)"
trap 'rm -rf -- "$T"' EXIT
fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }

key() { head -c 32 /dev/urandom | base64; }

# --- stubs ------------------------------------------------------------------
mkdir -p "$T/bin"
cat >"$T/bin/wg" <<'EOF'
#!/usr/bin/env bash
# wg stub: per-host state in $STUB_ROOT/state/{west,east}
set -euo pipefail
side="${STUB_SIDE:-west}"
st="$STUB_ROOT/state/$side"
printf '%s wg %s\n' "$side" "$*" >>"$STUB_ROOT/calls"
case "${1:-}" in
genpsk)
	k="$(head -c 32 /dev/urandom | base64)"
	printf '%s\n' "$k" >>"$STUB_ROOT/secrets"
	printf '%s\n' "$k"
	;;
show)
	case "${3:-}" in
	public-key) cat "$st/pub" ;;
	peers) cat "$st/peer" ;;
	preshared-keys) printf '%s\t%s\n' "$(cat "$st/peer")" "$(cat "$st/psk")" ;;
	latest-handshakes)
		# A handshake completes only when both ends hold the same PSK.
		ts=1000
		if [ "${STUB_HANDSHAKE:-fresh}" = fresh ] && cmp -s "$STUB_ROOT/state/west/psk" "$STUB_ROOT/state/east/psk"; then
			ts=$(($(date +%s) + 1))
		fi
		printf '%s\t%s\n' "$(cat "$st/peer")" "$ts"
		;;
	*) exit 1 ;;
	esac
	;;
set) # wg set IF peer PUB preshared-key FILE
	# STUB_FAIL_SET=<side>: that side's first wg set fails, later ones work.
	if [ "${STUB_FAIL_SET:-}" = "$side" ] && [ ! -e "$STUB_ROOT/failed-$side" ]; then
		: >"$STUB_ROOT/failed-$side"
		exit 1
	fi
	[ "${2:-}" = wg0 ] && [ "${3:-}" = peer ] && [ "${4:-}" = "$(cat "$st/peer")" ] && [ "${5:-}" = preshared-key ] || exit 1
	if [ "$6" = /dev/null ]; then echo '(none)' >"$st/psk"; else head -n 1 "$6" >"$st/psk"; fi
	;;
*) exit 1 ;;
esac
EOF
cat >"$T/bin/ssh" <<'EOF'
#!/usr/bin/env bash
# ssh stub: -vv reports the KEX; otherwise run the remote command as east.
printf 'ssh %s\n' "$*" >>"$STUB_ROOT/calls"
printf '%s\n' "${!#}" >>"$STUB_ROOT/remote-cmds" # the remote command alone, for the psk-gate cases
for a in "$@"; do
	if [ "$a" = -vv ]; then
		printf 'debug1: kex: algorithm: %s\r\n' "${STUB_KEX:-mlkem768x25519-sha256}" >&2 # CR LF, as real ssh -vv writes
		exit 0
	fi
done
STUB_SIDE=east exec bash -c "${!#}"
EOF
cat >"$T/bin/sudo" <<'EOF'
#!/usr/bin/env bash
printf '%s sudo %s\n' "${STUB_SIDE:-west}" "$*" >>"$STUB_ROOT/calls"
[ "${1:-}" = -n ] && shift
exec "$@"
EOF
cat >"$T/bin/logger" <<'EOF'
#!/usr/bin/env bash
printf '%s logger %s\n' "${STUB_SIDE:-west}" "$*" >>"$STUB_ROOT/calls"
EOF
cat >"$T/bin/ping" <<'EOF'
#!/usr/bin/env bash
printf '%s ping %s\n' "${STUB_SIDE:-west}" "$*" >>"$STUB_ROOT/calls"
EOF
chmod +x "$T/bin/"*

# --- fixtures ---------------------------------------------------------------
: >"$T/secrets"
: >"$T/outputs"
WEST_PUB="$(key)" EAST_PUB="$(key)"

mkconf() { # file priv peer-pub psk addr peer-addr endpoint
	cat >"$1" <<EOF
[Interface]
PrivateKey = $2
Address = $5/128
ListenPort = 51820

[Peer]
PublicKey = $3
PresharedKey = $4
AllowedIPs = $6/128
Endpoint = $7:51820
PersistentKeepalive = 25
EOF
	chmod 600 "$1"
}

setup() { # fresh west + east sharing one old PSK
	C="$T/case"
	rm -rf -- "$C"
	mkdir -p "$C/west" "$C/east" "$T/state/west" "$T/state/east"
	OLD="$(key)"
	local wp ep
	wp="$(key)" ep="$(key)"
	printf '%s\n%s\n%s\n' "$OLD" "$wp" "$ep" >>"$T/secrets"
	mkconf "$C/west/wg0.conf" "$wp" "$EAST_PUB" "$OLD" fd00:dead:beef::1 fd00:dead:beef::2 192.168.13.1
	mkconf "$C/east/wg0.conf" "$ep" "$WEST_PUB" "$OLD" fd00:dead:beef::2 fd00:dead:beef::1 192.168.13.2
	cp "$C/west/wg0.conf" "$C/west.orig"
	cp "$C/east/wg0.conf" "$C/east.orig"
	printf '%s\n' "$WEST_PUB" >"$T/state/west/pub"
	printf '%s\n' "$EAST_PUB" >"$T/state/west/peer"
	printf '%s\n' "$EAST_PUB" >"$T/state/east/pub"
	printf '%s\n' "$WEST_PUB" >"$T/state/east/peer"
	printf '%s\n' "$OLD" >"$T/state/west/psk"
	printf '%s\n' "$OLD" >"$T/state/east/psk"
	rm -f "$T"/failed-*
	: >"$T/calls"
}

rotate() { # env... -- args: run the tool, output in $OUT, rc in $RC
	local envs=()
	while [ "$1" != -- ]; do envs+=("$1"); shift; done
	shift
	set +e
	OUT="$(env PATH="$T/bin:$PATH" STUB_ROOT="$T" WG_PSK_DIR="$C/west" WG_PSK_REMOTE_DIR="$C/east" \
		WG_PSK_POLL=1 "${envs[@]}" bash "$ROT" "$@" 2>&1)"
	RC=$?
	set -e
	printf '%s\n' "$OUT" >>"$T/outputs"
	cat "$T/calls" >>"$T/all-calls"
}

no_wg_set() { ! grep -q ' wg set ' "$T/calls"; }
untouched() { cmp -s "$C/west.orig" "$C/west/wg0.conf" && cmp -s "$C/east.orig" "$C/east/wg0.conf" &&
	[ -z "$(find "$C/west" "$C/east" -name 'wg0.psk*' -o -name '.wg0.conf.new' | head -n 1)" ]; }
runtime_is() { [ "$(cat "$T/state/west/psk")" = "$1" ] && [ "$(cat "$T/state/east/psk")" = "$1" ]; }

# --- cases ------------------------------------------------------------------

# Dry run: reads only, prints the plan, sets nothing.
setup
rotate --
if [ "$RC" -eq 0 ] && [[ "$OUT" == *"DRY RUN"* ]] && no_wg_set && ! grep -q 'wg genpsk' "$T/calls" && untouched && runtime_is "$OLD"; then
	pass "dry run changes nothing and calls no wg set"
else fail "dry run: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# sntrup761 is a hybrid too.
setup
rotate STUB_KEX=sntrup761x25519-sha512@openssh.com --
if [ "$RC" -eq 0 ]; then pass "dry run accepts an sntrup761x25519 SSH session"; else fail "sntrup761x25519 refused: $(tail -n 1 <<<"$OUT")"; fi

# A classical SSH session to east: refuse before generating anything.
setup
rotate STUB_KEX=curve25519-sha256 -- --apply
if [ "$RC" -eq 3 ] && no_wg_set && ! grep -q 'wg genpsk' "$T/calls" && untouched && [[ "$OUT" == *"not a post-quantum hybrid"* ]]; then
	pass "--apply refuses when ssh to east negotiated classical KEX"
else fail "classical KEX: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# east answers with a different public key than west's peer: wrong host.
setup
key >"$T/state/east/pub"
rotate -- --apply
if [ "$RC" -eq 3 ] && no_wg_set && untouched; then pass "--apply refuses when east is not west's peer"
else fail "wrong east key: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# Hostile interface name never reaches a shell.
setup
rotate -- --apply --if 'wg0;touch /tmp/pwn'
if [ "$RC" -eq 2 ] && [ ! -s "$T/calls" ]; then pass "a hostile interface name is rejected before any call"
else fail "hostile --if: rc=$RC"; fi

# Rotation: east first, then west; a handshake; both files persisted.
setup
rotate -- --apply
NEW="$(tail -n 1 "$T/secrets")"
first_set="$(grep ' wg set ' "$T/calls" | head -n 1 | cut -d' ' -f1)"
if [ "$RC" -eq 0 ] && [ "$first_set" = east ] && grep -q '^west wg set ' "$T/calls"; then pass "--apply sets east before west"
else fail "--apply order: rc=$RC first=$first_set; $(tail -n 1 <<<"$OUT")"; fi
if [ "$NEW" != "$OLD" ] && runtime_is "$NEW"; then pass "both hosts run the new PSK"; else fail "runtime PSK not the new key on both hosts"; fi
persisted=1
for h in west east; do
	f="$C/$h/wg0.conf"
	[ "$(grep -c '^PresharedKey' "$f")" -eq 1 ] && grep -qxF "PresharedKey = $NEW" "$f" || persisted=0
	diff <(grep -v '^PresharedKey' "$C/$h.orig") <(grep -v '^PresharedKey' "$f") >/dev/null || persisted=0
	[ "$(stat -c %a "$f")" = 600 ] && [ "$(stat -c %a "$C/$h/wg0.psk")" = 600 ] || persisted=0
	cmp -s <(printf '%s\n' "$NEW") "$C/$h/wg0.psk" || persisted=0
	[ ! -e "$C/$h/wg0.psk.new" ] && [ ! -e "$C/$h/wg0.psk.old" ] || persisted=0
done
if [ "$persisted" -eq 1 ]; then pass "wg0.conf PresharedKey and wg0.psk (0600) hold the new key on both hosts; nothing else changed"
else fail "persistence after rotation"; fi
if grep -q '^west logger -t unheaded-wg-psk psk-rotated ' "$T/calls"; then pass "journal line psk-rotated"; else fail "no psk-rotated journal line"; fi

# No handshake: both hosts back on the old PSK, nothing persisted.
setup
rotate STUB_HANDSHAKE=never -- --apply --timeout 2
if [ "$RC" -eq 1 ] && runtime_is "$OLD" && untouched && grep -q 'psk-rotation-rolled-back' "$T/calls"; then
	pass "a missing handshake rolls both hosts back to the old PSK"
else fail "no-handshake rollback: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# west's wg set fails after east already switched: east is put back.
setup
rotate STUB_FAIL_SET=west -- --apply
if [ "$RC" -ne 0 ] && runtime_is "$OLD" && untouched; then pass "a failure after east switched rolls east back"
else fail "mid-apply failure: rc=$RC; $(tail -n 1 <<<"$OUT")"; fi

# --- psk-gate.sh: east's forced command for the rotation key (DP-05) --------
# Its sudo is a stub that records the call and swallows stdin: nothing here
# runs as root or touches /etc/wireguard.
GATE="$R/scripts/wireguard/psk-gate.sh"
mkdir -p "$T/gbin"
cat >"$T/gbin/sudo" <<'EOF'
#!/usr/bin/env bash
printf 'gate sudo %s\n' "$*" >>"$STUB_ROOT/gate-calls"
cat >/dev/null
EOF
cp "$T/bin/logger" "$T/gbin/logger"
chmod +x "$T/gbin/"*
# The side script exactly as rotate-psk.sh sends it; the same awk the install
# step uses to make side.sha256.
awk "/^\tcat <<'SIDE'\$/{f=1;next} /^SIDE\$/{f=0} f" "$ROT" >"$T/side.sh"
sha256sum <"$T/side.sh" | cut -d' ' -f1 >"$T/side.sha256"
gate() { # command stdin-file [env...]: rc in $RC, stderr in $OUT, stub calls in $T/gate-calls
	local cmd="$1" in="$2"
	shift 2
	: >"$T/gate-calls"
	set +e
	# -u: this test may itself run in an ssh session, where the gate would
	# rightly ignore PSK_GATE_SIDE_SHA.
	OUT="$(env -u SSH_CONNECTION PATH="$T/gbin:$PATH" STUB_ROOT="$T" PSK_GATE_SIDE_SHA="$T/side.sha256" \
		SSH_ORIGINAL_COMMAND="$cmd" "$@" bash "$GATE" <"$in" 2>&1 >/dev/null)"
	RC=$?
	set -e
}
uncalled() { [ ! -s "$T/gate-calls" ]; }

# shellcheck disable=SC2016 # a literal $action is what the side script holds
if [ -s "$T/side.sh" ] && grep -q '^case "\$action" in$' "$T/side.sh"; then pass "the side script can be read out of rotate-psk.sh"
else fail "could not read the side script out of rotate-psk.sh"; fi

gate id /dev/null
if [ "$RC" -eq 3 ] && [[ "$OUT" == *"psk-gate: refused"* ]] && uncalled; then pass "psk-gate refuses a command rotate-psk.sh never sends"
else fail "psk-gate id: rc=$RC; $OUT"; fi

gate true /dev/null
if [ "$RC" -eq 0 ] && uncalled; then pass "psk-gate answers true (the KEX probe) without sudo"
else fail "psk-gate true: rc=$RC; $OUT"; fi

gate "sudo -n sh -s -- check wg0 /etc/wireguard $WEST_PUB" "$T/side.sh"
if [ "$RC" -eq 0 ] && grep -qxF "gate sudo -n sh -s -- check wg0 /etc/wireguard $WEST_PUB" "$T/gate-calls"; then
	pass "psk-gate runs the pinned side script"
else fail "psk-gate pinned side script: rc=$RC; $OUT"; fi

printf 'id\n' >"$T/side.sh.changed"
cat "$T/side.sh" >>"$T/side.sh.changed"
gate "sudo -n sh -s -- check wg0 /etc/wireguard $WEST_PUB" "$T/side.sh.changed"
if [ "$RC" -eq 3 ] && [[ "$OUT" == *"hash mismatch"* ]] && uncalled; then pass "psk-gate refuses a changed side script"
else fail "psk-gate changed side script: rc=$RC; $OUT"; fi

# One missing final newline is a different script.
head -c -1 "$T/side.sh" >"$T/side.sh.short"
gate "sudo -n sh -s -- check wg0 /etc/wireguard $WEST_PUB" "$T/side.sh.short"
if [ "$RC" -eq 3 ] && uncalled; then pass "psk-gate hashes the side script to the last byte"
else fail "psk-gate short side script: rc=$RC; $OUT"; fi

# Extra words, another interface, another directory, a shell metacharacter.
bad=0
for c in "sudo -n sh -s -- check wg0 /etc/wireguard $WEST_PUB; id" \
	"sudo -n sh -s -- check wg1 /etc/wireguard $WEST_PUB" \
	"sudo -n sh -s -- check wg0 /tmp $WEST_PUB" \
	"sudo -n sh -s -- shell wg0 /etc/wireguard $WEST_PUB" \
	"sudo -n sh -s -- check wg0 /etc/wireguard \$(id)" \
	"sudo -n sh -s" \
	"sudo -n wg show wg0 private-key" \
	"sudo -n sh -c 'umask 077; cat >/etc/wireguard/wg0.conf'" \
	"true; id" ""; do
	gate "$c" "$T/side.sh"
	if [ "$RC" -ne 3 ] || ! uncalled; then
		bad=$((bad + 1))
		echo "  accepted: $c"
	fi
done
if [ "$bad" -eq 0 ]; then pass "psk-gate refuses ten near misses"; else fail "psk-gate accepted $bad near miss(es)"; fi

# Under sshd (SSH_CONNECTION set) the pin cannot be moved by a variable: a
# script whose hash sits in the file PSK_GATE_SIDE_SHA names is still refused.
printf 'id\n' >"$T/other.sh"
sha256sum <"$T/other.sh" | cut -d' ' -f1 >"$T/other.sha256"
gate "sudo -n sh -s -- check wg0 /etc/wireguard $WEST_PUB" "$T/other.sh" \
	PSK_GATE_SIDE_SHA="$T/other.sha256" SSH_CONNECTION="192.0.2.1 50000 192.0.2.2 7375"
if [ "$RC" -eq 3 ] && uncalled; then pass "psk-gate ignores PSK_GATE_SIDE_SHA in an ssh session"
else fail "psk-gate took its pin from the environment in an ssh session: rc=$RC"; fi

# Every command the runs above sent to east, with the test directory put back
# to /etc/wireguard, is one the gate lets through.
sed "s|$C/east|/etc/wireguard|g" "$T/remote-cmds" | sort -u >"$T/remote-shapes"
bad=0
while IFS= read -r c; do
	gate "$c" "$T/side.sh"
	if [ "$RC" -ne 0 ]; then
		bad=$((bad + 1))
		echo "  refused: ${c:0:60}"
	fi
done <"$T/remote-shapes"
kinds="$(sed -E 's/ [A-Za-z0-9+\/]{43}=$//' "$T/remote-shapes" | sort -u | wc -l)"
if [ "$bad" -eq 0 ] && [ "$kinds" -ge 9 ]; then pass "psk-gate allows every command rotate-psk.sh sent ($kinds kinds)"
else fail "psk-gate refused $bad of rotate-psk.sh's own commands ($kinds kinds seen, want 9 or more)"; fi

# Secrecy: no key on any stub's command line or in any output.
grep -v '^$' "$T/secrets" >"$T/secret-patterns"
if [ "$(wc -l <"$T/secret-patterns")" -lt 10 ]; then
	fail "secrecy check has too few keys to mean anything"
elif grep -qFf "$T/secret-patterns" "$T/all-calls" "$T/outputs"; then
	fail "a key reached a command line or the script's output"
else
	pass "no key on any command line or in any output ($(wc -l <"$T/all-calls") calls)"
fi

echo
if [ "$fails" -gt 0 ]; then
	echo "test-rotate-psk: $fails failure(s)"
	exit 1
fi
echo "test-rotate-psk: all passed"
