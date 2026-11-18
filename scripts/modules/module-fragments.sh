#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# module-fragments.sh: a module's fragments follow its switch (ADR-149).
# unheaded-zhen-module.service runs it inside unheaded-zhen.target:
#
#   module-fragments.sh publish <name>    copy the module's prometheus/,
#       victoria/ and dashboard/ trees (regular files only) to RUN/<name>
#   module-fragments.sh withdraw <name>   remove RUN/<name>
#
# Both end by writing unheaded_module_enabled{module} (1 or 0) and
# unheaded_module_changed_timestamp_seconds{module} to TEXTFILE/module_<name>.prom.
#
#   MODULES_SRC       the modules' source trees  (/usr/local/lib/unheaded/modules)
#   MODULES_RUN       where fragments live while on (/run/unheaded/modules)
#   MODULES_TEXTFILE  node_exporter's textfile directory (/var/lib/unheaded/textfile)
#
# Exit 0 done, 1 no such module, 2 usage. Runs as unheaded-modules, never root.
set -euo pipefail

SRC="${MODULES_SRC:-/usr/local/lib/unheaded/modules}"
RUN="${MODULES_RUN:-/run/unheaded/modules}"
TEXTFILE="${MODULES_TEXTFILE:-/var/lib/unheaded/textfile}"

usage() {
    echo "usage: module-fragments.sh publish|withdraw <name>" >&2
    exit 2
}

[ "$#" -eq 2 ] || usage
verb="$1" name="$2"
case "$verb" in publish|withdraw) ;; *) usage ;; esac
if ! [[ "$name" =~ ^[a-z][a-z0-9-]{0,31}$ ]]; then
    echo "module-fragments: bad module name: '$name'" >&2
    exit 2
fi

write_textfile() {
    local enabled="$1" tmp="${TEXTFILE}/.module_${name}.prom.tmp"
    {
        echo "# HELP unheaded_module_enabled 1 while the module's switch is on (ADR-149)."
        echo "# TYPE unheaded_module_enabled gauge"
        echo "unheaded_module_enabled{module=\"${name}\"} ${enabled}"
        echo "# HELP unheaded_module_changed_timestamp_seconds When the module's fragments were last published or withdrawn."
        echo "# TYPE unheaded_module_changed_timestamp_seconds gauge"
        echo "unheaded_module_changed_timestamp_seconds{module=\"${name}\"} $(date +%s)"
    } > "$tmp"
    chmod 0644 "$tmp"
    mv -f "$tmp" "${TEXTFILE}/module_${name}.prom"
}

case "$verb" in
publish)
    if [ ! -d "${SRC}/${name}" ] || [ -L "${SRC}/${name}" ]; then
        echo "module-fragments: no module '${name}' under ${SRC}" >&2
        exit 1
    fi
    new="${RUN}/.${name}.new" old="${RUN}/.${name}.old"
    rm -rf "$new" "$old"
    trap 'rm -rf "$new"' EXIT
    mkdir -m 0755 "$new"
    for t in prometheus victoria dashboard; do
        [ -d "${SRC}/${name}/${t}" ] || continue
        while IFS= read -r -d '' f; do
            rel="${f#"${SRC}/${name}/"}"
            install -D -m 0644 "$f" "${new}/${rel}"
        done < <(find "${SRC}/${name}/${t}" -type f -print0)
    done
    find "$new" -type d -exec chmod 0755 {} +
    if [ -d "${RUN}/${name}" ]; then
        mv "${RUN}/${name}" "$old"
    fi
    mv "$new" "${RUN}/${name}"
    trap - EXIT
    rm -rf "$old"
    write_textfile 1
    ;;
withdraw)
    old="${RUN}/.${name}.old"
    rm -rf "$old"
    if [ -d "${RUN}/${name}" ]; then
        mv "${RUN}/${name}" "$old"
        rm -rf "$old"
    fi
    write_textfile 0
    ;;
esac
