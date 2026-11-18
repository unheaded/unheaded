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
# Prometheus takes a wildcard only in the last component of a rule_files or
# scrape_config_files path, so it cannot read RUN/*/prometheus/rules/*.yml
# (it refuses the whole config). publish therefore also copies each *.yml
# directly under the module's prometheus/rules and prometheus/scrape to one
# flat directory per kind, the module's name leading the file name:
#
#   RUN/_prometheus/rules/<name>.<file>    RUN/_prometheus/scrape/<name>.<file>
#
# and withdraw removes them. Those two directories are what
# monitoring/prometheus/prometheus.yml globs; they stay, empty, while no
# module is on. "_prometheus" can never be a module (a name starts with a
# letter). Everything else reads RUN/<name>: pkg/modules, VictoriaMetrics
# (config/victoria/scrape.yml), the dashboard.
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

# The module trees Prometheus loads, each with a flat directory RUN/_<tree>.
FLAT="prometheus/rules prometheus/scrape"

# flat_publish: RUN/_<tree>/<name>.<file> for each *.yml directly under
# RUN/<name>/<tree>, each put in place by one rename (a reader never sees
# half a file, and the temporary name does not end in .yml), then the copies
# of files the module no longer has are removed.
flat_publish() {
    local t dir f base
    for t in $FLAT; do
        dir="${RUN}/_${t}"
        if [ -d "${RUN}/${name}/${t}" ]; then
            mkdir -p "$dir"
            chmod 0755 "${RUN}/_${t%%/*}" "$dir"
            while IFS= read -r -d '' f; do
                base="${f##*/}"
                install -m 0644 "$f" "${dir}/.${name}.${base}.tmp"
                mv -f "${dir}/.${name}.${base}.tmp" "${dir}/${name}.${base}"
            done < <(find "${RUN}/${name}/${t}" -maxdepth 1 -type f -name '*.yml' -print0)
        fi
        [ -d "$dir" ] || continue
        find "$dir" -maxdepth 1 -name ".${name}.*.tmp" -delete
        while IFS= read -r -d '' f; do
            base="${f##*/}"
            [ -f "${RUN}/${name}/${t}/${base#"${name}."}" ] || rm -f "$f"
        done < <(find "$dir" -maxdepth 1 -name "${name}.*" -print0)
    done
}

# flat_withdraw: remove this module's copies. "<name>." cannot begin another
# module's copy: a name holds no dot.
flat_withdraw() {
    local t
    for t in $FLAT; do
        [ -d "${RUN}/_${t}" ] || continue
        find "${RUN}/_${t}" -maxdepth 1 \( -name "${name}.*" -o -name ".${name}.*.tmp" \) -delete
    done
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
    flat_publish
    write_textfile 1
    ;;
withdraw)
    flat_withdraw
    old="${RUN}/.${name}.old"
    rm -rf "$old"
    if [ -d "${RUN}/${name}" ]; then
        mv "${RUN}/${name}" "$old"
        rm -rf "$old"
    fi
    write_textfile 0
    ;;
esac
