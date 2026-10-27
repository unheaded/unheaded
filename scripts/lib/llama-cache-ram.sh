#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# llama_cache_ram_mib FLAGS
#   Print the host-RAM prompt cache (MiB) a llama-server launched with FLAGS
#   may grow to: the value of --cache-ram / -cram (also --cache-ram=N), else
#   llama.cpp's default, LLAMA_CACHE_RAM_DEFAULT_MIB (8192; common/arg.cpp
#   `params.cache_ram_mib`). Returns 1 for -1 ("no limit": nothing can be
#   fit-checked against it) or a non-numeric value.
#
# Sourced by scripts/switch-model.sh (S-CLEAN-SLATE M-037): the fit check
# ignored this cache, and a model that "fit" grew to 9.4 GiB RSS on the
# 14 GiB host. Tested by scripts/lib/switchmodel_fit_test.go.

LLAMA_CACHE_RAM_DEFAULT_MIB=8192

llama_cache_ram_mib() {
	local -a words
	read -r -a words <<<"${1:-}"
	local val="" i
	for ((i = 0; i < ${#words[@]}; i++)); do
		case "${words[i]}" in
		--cache-ram | -cram) val="${words[i + 1]:-}" ;;
		--cache-ram=* | -cram=*) val="${words[i]#*=}" ;;
		esac
	done
	if [[ -z "$val" ]]; then
		echo "$LLAMA_CACHE_RAM_DEFAULT_MIB"
		return 0
	fi
	if [[ "$val" =~ ^[0-9]+$ ]]; then
		echo "$val"
		return 0
	fi
	echo "llama_cache_ram_mib: --cache-ram '$val' is unbounded or not a number" >&2
	return 1
}
