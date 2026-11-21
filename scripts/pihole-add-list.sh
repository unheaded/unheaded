#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# pihole-add-list.sh — add one blocklist to a Pi-hole that this host itself
# resolves through, and keep it only if nothing that matters stops resolving.
#
# The Pi-hole runs in an LXD instance (ADR-022). The shell that starts this
# script resolves names through the same Pi-hole, so the script finishes by
# itself: start it detached and read its log. It puts everything back (the
# list's row removed, gravity.db restored from the copy it took, DNS
# reloaded) when a name on the MUST list stops resolving, when a recently
# used name matching CRITICAL is newly blocked, or when the gravity run fails
# or loads nothing.
#
#   setsid nohup scripts/pihole-add-list.sh <list-url> "<comment>" >/dev/null 2>&1 &
#   tail -f ~/.local/state/unheaded/pihole-lists/pihole-list-*/log.txt
#
# It records the blocked share of the last 24 h before the change
# (rate-before.txt), the names answered in the last 7 days (used.txt) and the
# ones the new list blocks (newly-blocked.txt). Read the share again a day
# later to have the after.
#
# Settings, each a variable with a default:
#   PIHOLE_INSTANCE    LXD instance name (pihole)
#   PIHOLE_IP          its address, asked directly with dig (10.10.20.252)
#   LXC                the lxc client (/snap/bin/lxc; never /usr/sbin/lxc, the installer wrapper)
#   PIHOLE_LIST_STATE  where each run's directory goes (~/.local/state/unheaded/pihole-lists)
#   MUST               names that must keep resolving
#   CRITICAL           regular expression: a used name matching it, newly blocked, reverts the list
#   TELEMETRY_OK       names under CRITICAL that a blocklist is meant to block
# Needs sudo -n for the lxc client, and dig. Lookups are spaced (about five a
# second): the Pi-hole refuses a client everything for a while once it passes
# 1,000 queries a minute.
#
# Exit: 0 kept (or already there), 1 refused before any change, 3 reverted.
set -uo pipefail
URL="${1:?list url}"; COMMENT="${2:?comment}"
RUN="${PIHOLE_LIST_STATE:-$HOME/.local/state/unheaded/pihole-lists}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="$RUN/pihole-list-$STAMP"; mkdir -p "$OUT"
LOG="$OUT/log.txt"
PIHOLE_IP="${PIHOLE_IP:-10.10.20.252}"
PIHOLE_INSTANCE="${PIHOLE_INSTANCE:-pihole}"
LX=(sudo -n "${LXC:-/snap/bin/lxc}" exec "$PIHOLE_INSTANCE" --)
# Names that must keep resolving, whatever a list says.
MUST="${MUST:-api.anthropic.com claude.ai console.anthropic.com github.com raw.githubusercontent.com proxy.golang.org sum.golang.org registry-1.docker.io auth.docker.io ghcr.io crates.io static.crates.io index.crates.io pypi.org files.pythonhosted.org archive.ubuntu.com security.ubuntu.com}"
# A recently used name matching this, newly blocked, also reverts.
CRITICAL="${CRITICAL:-}"; [ -n "$CRITICAL" ] || CRITICAL='(^|\.)(anthropic\.com|claude\.ai|claude\.com|github\.com|githubusercontent\.com|githubassets\.com|golang\.org|docker\.io|docker\.com|ghcr\.io|crates\.io|pypi\.org|pythonhosted\.org|ubuntu\.com|canonical\.com|snapcraft\.io|letsencrypt\.org|bellis\.tech|unheaded\.org)$'
say() { echo "$(date -u +%FT%TZ) $*" | tee -a "$LOG"; }
sql() { "${LX[@]}" pihole-FTL sqlite3 "$1" "$2"; }
# The first address the Pi-hole gives for a name (CNAME lines skipped); one
# retry after a pause, in case this host is being rate-limited.
answer() {
    local a
    a="$(dig +short +time=3 +tries=1 "@$PIHOLE_IP" "$1" A 2>/dev/null | grep -E '^[0-9.]+$' | head -n1)"
    if [ -z "$a" ]; then sleep 12; a="$(dig +short +time=3 +tries=1 "@$PIHOLE_IP" "$1" A 2>/dev/null | grep -E '^[0-9.]+$' | head -n1)"; fi
    printf '%s\n' "$a"
}
# One lookup, no retry: a name that no longer exists answers nothing, which
# is not "blocked".
blocked() { [ "$(dig +short +time=2 +tries=1 "@$PIHOLE_IP" "$1" A 2>/dev/null | grep -E '^[0-9.]+$' | head -n1)" = "0.0.0.0" ]; }

say "start: list $URL ($COMMENT)"
"${LX[@]}" true || { say "FAIL: no access to the LXD instance $PIHOLE_INSTANCE"; exit 1; }
[ -n "$(answer github.com)" ] || { say "FAIL: the Pi-hole does not answer at $PIHOLE_IP before any change"; exit 1; }
if [ "$(sql /etc/pihole/gravity.db "SELECT count(*) FROM adlist WHERE address = '$URL'")" != 0 ]; then say "the list is already there: nothing to do"; exit 0; fi

# 1. Before: the blocked rate of the last 24 h, the lists, the names in use.
NOW="$(date +%s)"
sql /etc/pihole/pihole-FTL.db "SELECT count(*), sum(CASE WHEN status IN (1,4,5,6,7,8,9,10,11,15,16) THEN 1 ELSE 0 END) FROM queries WHERE timestamp > $((NOW-86400))" > "$OUT/rate-before.txt"
say "before: queries and blocked in the last 24 h: $(tr '|' ' ' < "$OUT/rate-before.txt"); gravity $(sql /etc/pihole/gravity.db 'SELECT count(*) FROM gravity') domains"
# Names answered (not blocked) in the last 7 days, most used first.
sql /etc/pihole/pihole-FTL.db "SELECT domain FROM queries WHERE timestamp > $((NOW-604800)) AND status IN (2,3,12,13,14,17) GROUP BY domain ORDER BY count(*) DESC LIMIT 4000" > "$OUT/used.txt"
say "names in use (7 days, answered): $(wc -l < "$OUT/used.txt")"
# The names were chosen as answered, so none is blocked now; no pass over
# them before the change. (A first version asked the Pi-hole about each
# name twice, 3,700 queries in a minute: its rate limit, 1,000 a minute for
# a client, then refused this host everything for ten seconds.)
for d in $MUST; do [ -n "$(answer "$d")" ] && ! blocked "$d" || { say "FAIL: $d does not resolve before any change; not starting"; exit 1; }; done

# 2. Keep what is there.
"${LX[@]}" cp -p /etc/pihole/gravity.db "/etc/pihole/gravity.db.before-$STAMP" || { say "FAIL: no copy of gravity.db"; exit 1; }
say "kept: /etc/pihole/gravity.db.before-$STAMP in the instance"

revert() {
    say "REVERT: $*"
    "${LX[@]}" sh -c "cp -p /etc/pihole/gravity.db.before-$STAMP /etc/pihole/gravity.db && chown pihole:pihole /etc/pihole/gravity.db; pihole-FTL sqlite3 /etc/pihole/gravity.db \"DELETE FROM adlist WHERE address = '$URL'\"; pihole reloaddns" >> "$LOG" 2>&1
    sleep 5
    for d in $MUST; do [ -n "$(answer "$d")" ] && ! blocked "$d" || say "AFTER REVERT STILL FAILING: $d"; done
    say "reverted: lists $(sql /etc/pihole/gravity.db 'SELECT count(*) FROM adlist'), gravity $(sql /etc/pihole/gravity.db 'SELECT count(*) FROM gravity')"
    exit 3
}

# 3. Add the list and run gravity (it builds a new database and swaps it in).
sql /etc/pihole/gravity.db "INSERT INTO adlist (address, enabled, comment) VALUES ('$URL', 1, '$COMMENT')" || revert "could not add the list row"
if ! timeout 900 "${LX[@]}" pihole -g > "$OUT/gravity.txt" 2>&1; then revert "pihole -g failed or took over 15 minutes (see gravity.txt)"; fi
sleep 5
ROW="$(sql /etc/pihole/gravity.db "SELECT number, invalid_domains, status FROM adlist WHERE address = '$URL'")"
say "after gravity: the list's row (domains|invalid|status): $ROW; gravity $(sql /etc/pihole/gravity.db 'SELECT count(*) FROM gravity') entries"
case "$ROW" in ""|0\|*) revert "the list loaded no domain (wrong address or a failed download)";; esac

# 4. Checks. The MUST names first, then every name that was in use.
for d in $MUST; do
    a="$(answer "$d")"
    if [ -z "$a" ] || [ "$a" = "0.0.0.0" ]; then revert "$d no longer resolves (answer '${a:-none}')"; fi
done
: > "$OUT/newly-blocked.txt"
# At most about five a second: far under the Pi-hole's rate limit, with room
# for this host's own lookups.
while read -r d; do
    [ -n "$d" ] || continue
    if blocked "$d"; then echo "$d" >> "$OUT/newly-blocked.txt"; fi
    sleep 0.2
done < "$OUT/used.txt"
N="$(wc -l < "$OUT/newly-blocked.txt")"
say "names in use that the new list blocks: $N (newly-blocked.txt)"
# Telemetry hosts under a CRITICAL domain that a blocklist is meant to block
# (seen 2026-11-21: collector.github.com is GitHub's analytics collector).
TELEMETRY_OK="${TELEMETRY_OK:-}"; [ -n "$TELEMETRY_OK" ] || TELEMETRY_OK='^(collector\.github\.com)$'
CRIT="$(grep -E -- "$CRITICAL" "$OUT/newly-blocked.txt" | grep -v -E -- "$TELEMETRY_OK" | head -5 | tr '\n' ' ')"
[ -z "$CRIT" ] || revert "the list blocks names in use that matter: $CRIT"

say "KEPT: $URL is live. Used names now blocked: $N. Undo by hand: delete the adlist row, copy gravity.db.before-$STAMP back, pihole reloaddns."
exit 0
