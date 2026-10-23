#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# The one GPU lease on this host (S-TWO-BRAINS Step 233), shared with
# pkg/gpulease: an flock on $GPU_LOCK (default
# ${XDG_RUNTIME_DIR:-/tmp}/unheaded/gpu.lock; dir 0700, file 0600).
#
#   gpu-lease.sh shared    <timeout-s> -- cmd [args...]   # inference
#   gpu-lease.sh exclusive <timeout-s> -- cmd [args...]   # swap a model, train
#   gpu-lease.sh probe     <timeout-s>                    # 0: no exclusive holder
#
# or, sourced: gpu_lease_shared <timeout-s> -- cmd ...; gpu_lease_exclusive ...
#
# Exit: the command's own status; 75 (EX_TEMPFAIL) when the lease was not
# granted within the timeout; 2 on bad usage. The lock descriptor is
# closed before the command runs (flock -o): a child it leaves running,
# like a nohup'd llama-server, never holds the lease. Inside an
# exclusive lease (GPU_LEASE_HELD=exclusive, exported to the command),
# nested calls run their command directly, so forge-train.sh can call
# switch-model.sh without waiting on itself.
#
# Emergency (Appendix A4): `fuser -v "$GPU_LOCK"` names the holder; stop
# that process and the lease frees itself. Never delete the file.

gpu_lease_path() {
	if [[ -n "${GPU_LOCK:-}" ]]; then
		printf '%s\n' "$GPU_LOCK"
	else
		printf '%s/unheaded/gpu.lock\n' "${XDG_RUNTIME_DIR:-/tmp}"
	fi
}

# _gpu_lease <shared|exclusive|probe> <timeout> [-- cmd...]
_gpu_lease() {
	local mode=${1:-} timeout=${2:-}
	if [[ "$mode" != shared && "$mode" != exclusive && "$mode" != probe ]] || [[ ! "$timeout" =~ ^[0-9]+$ ]]; then
		echo "usage: gpu-lease.sh shared|exclusive <timeout-s> -- cmd [args...] | probe <timeout-s>" >&2
		return 2
	fi
	shift 2
	if [[ "$mode" != probe ]]; then
		if [[ "${1:-}" != "--" || $# -lt 2 ]]; then
			echo "gpu-lease.sh: want -- cmd [args...]" >&2
			return 2
		fi
		shift
	fi
	if ! command -v flock >/dev/null 2>&1; then
		echo "gpu-lease.sh: flock (util-linux) is required" >&2
		return 2
	fi
	if [[ "${GPU_LEASE_HELD:-}" == exclusive ]]; then
		# Already inside the exclusive lease: everything is ours.
		[[ "$mode" == probe ]] && return 0
		"$@"
		return
	fi
	local lock dir
	lock=$(gpu_lease_path)
	dir=$(dirname -- "$lock")
	(umask 077 && mkdir -p -- "$dir") || return 2
	if [[ -n "$(find "$dir" -maxdepth 0 -perm /022 2>/dev/null)" || ! -O "$dir" ]]; then
		echo "gpu-lease.sh: $dir is group/world writable or not ours; refusing (anyone could hold the GPU)" >&2
		return 2
	fi
	if [[ -L "$lock" ]]; then
		echo "gpu-lease.sh: $lock is a symlink; refusing" >&2
		return 2
	fi
	(umask 077 && : >>"$lock") || return 2
	case "$mode" in
	probe)
		flock --shared --nonblock --conflict-exit-code 75 "$lock" true
		;;
	shared)
		flock --shared --wait "$timeout" --conflict-exit-code 75 --close "$lock" "$@"
		;;
	exclusive)
		GPU_LEASE_HELD=exclusive flock --exclusive --wait "$timeout" --conflict-exit-code 75 --close "$lock" "$@"
		;;
	esac
}

gpu_lease_shared() { _gpu_lease shared "$@"; }
gpu_lease_exclusive() { _gpu_lease exclusive "$@"; }

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	set -uo pipefail
	_gpu_lease "$@"
	exit $?
fi
