#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# lib-disks.sh: which physical disks hold a path, and a refusal when a backup
# set would share one with the data it copies (ADR-147 decision 6, the
# Well's rule). Sourced by scripts/backup/{clickhouse,victoria}-backup.sh.
#
# disks_of_dev and disks_of_path are scripts/db-backup.sh:64-83, moved here
# verbatim (db-backup.sh keeps its own copy: the Well's installed script is
# standalone). Disks come from sysfs: through device-mapper/LVM slaves down
# to the whole disk of a partition.

disks_of_dev() {
	local d s found=0
	d="$(readlink -f "/sys/dev/block/$1")" && [[ -d "$d" ]] || return 1
	for s in "$d"/slaves/*; do
		[[ -e "$s/dev" ]] || continue
		found=1
		disks_of_dev "$(<"$s/dev")" || return 1
	done
	((found)) && return 0
	if [[ -f "$d/partition" ]]; then basename "$(dirname "$d")"; else basename "$d"; fi
}
disks_of_path() {
	local mm
	mm="$(findmnt -no MAJ:MIN -T "$1" | tr -d "[:space:]")" && [[ -n "$mm" ]] || return 1
	disks_of_dev "$mm" | sort -u
}

# refuse_shared_disk <data-path> <set-root> <tag>: exit 1 unless the two
# paths are on different physical disks (as db-backup.sh:84-94 does for
# WELL_BACKUP_OTHER_DISK=1).
refuse_shared_disk() {
	local data="$1" root="$2" tag="$3" data_disks set_disks shared
	data_disks="$(disks_of_path "$data" | paste -sd' ')" || true
	set_disks="$(disks_of_path "$root" | paste -sd' ')" || true
	[[ -n "$data_disks" ]] || { echo "[$tag] cannot tell which disk holds $data" >&2; exit 1; }
	[[ -n "$set_disks" ]] || { echo "[$tag] cannot tell which disk holds $root" >&2; exit 1; }
	shared="$(comm -12 <(tr ' ' '\n' <<<"$data_disks") <(tr ' ' '\n' <<<"$set_disks") | paste -sd' ')" || true
	[[ -z "$shared" ]] || { echo "[$tag] refusing: $root and the data ($data) share disk(s): $shared" >&2; exit 1; }
	echo "[$tag] data on $data_disks, sets on $set_disks"
}

# write_prom <dir> <file> <lines...>: a .prom file for huginn, atomically,
# 0644, only # HELP / # TYPE comments.
write_prom() {
	local dir="$1" file="$2" m
	shift 2
	m="$dir/.$file.tmp"
	printf '%s\n' "$@" >"$m" && chmod 0644 "$m" && mv -f "$m" "$dir/$file"
}

# read_meta <file>: KEY=VALUE lines into META_<KEY> variables, accepting only
# ^[A-Z]+=[A-Za-z0-9_:./@-]*$ (no quotes, no spaces, nothing a shell would
# run). Any other line: exit 1.
read_meta() {
	local line k v
	while IFS= read -r line || [[ -n "$line" ]]; do
		[[ "$line" =~ ^([A-Z]+)=([A-Za-z0-9_:./@-]*)$ ]] || { echo "[meta] refusing line in $1: $line" >&2; exit 1; }
		k="${BASH_REMATCH[1]}" v="${BASH_REMATCH[2]}"
		printf -v "META_$k" '%s' "$v"
	done <"$1"
}
