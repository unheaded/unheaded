#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-host-settings.sh: one file in this repository for every host setting,
# and a check that fails when a host drifts from it (S-WEST-GROUND, ADR-147;
# Architect A5, A12). Each host has a manifest, deploy/host-settings/<host>.tsv,
# with one row per file:
#
#   host_path<TAB>repo_path<TAB>mode
#
#   exact     the host file equals the repository file, byte for byte
#   contains  every repository line that is not blank and not a # comment is a
#             whole line of the host file (for /etc/fstab: the host holds more)
#   redact    both sides with PrivateKey, PresharedKey, PublicKey values and
#             "password <x>" masked, and comment (#, !) and blank lines dropped,
#             are equal (WireGuard: the repository keeps placeholders)
#   absent    the host path does not exist (repo_path is -)
#
# Two ways to run it:
#
#   check-host-settings.sh [--repo DIR]
#       The repository self-check (CI, the meta-gate): every manifest is well
#       formed, every repo_path exists, west's sysctl source reserves no huge
#       pages (decision 12), no slice named in a manifest carries a share
#       (decision 13: plan 8 sets the shares), and the drift-check unit
#       neither runs in its own network namespace (the net.* sysctls it reads
#       must be the host's) nor counts exit 1 or 2 as success. Exit 0 or 1; no manifest
#       directory, or no manifest in it, is exit 2, never a pass.
#
#   check-host-settings.sh --host NAME [--repo DIR] [--root DIR]
#                          [--textfile DIR] [--allow-skip]
#       Compares the host with the repository, one line per row:
#       OK|DRIFT|SKIP <TAB> mode <TAB> host path. SKIP: the file exists (or may
#       exist behind a directory this user cannot search) and cannot be read.
#       Without --root it also reads the running host: every key of an exact
#       /etc/sysctl.d row against `sysctl -n`, every bind line of the
#       /etc/fstab row against findmnt, and the active swap list.
#       Exit 2 on a usage error or a malformed manifest; else 4 on any DRIFT;
#       else 3 on any SKIP without --allow-skip; else 0. Exit 1 is never a
#       verdict: it is a command that failed under `set -e` (mktemp or mv in
#       the textfile directory), so the unit counts only 3 and 4 as success.
#       --textfile DIR writes DIR/unheaded_host_settings.prom (drift and skip
#       counts, the time of the check) for node_exporter, whatever the exit
#       code of a completed check (0, 3 or 4). A run that stops on exit 1 or
#       2 writes nothing, so HostSettingsCheckStale catches it.
#
# It never prints a line of any file it reads: keys and passwords live in some
# of them. Only verdicts, modes, paths and sysctl key names reach the output.
set -euo pipefail

SCRIPT_REPO="$(cd "$(dirname "$0")/.." && pwd)"
HOST=""
REPO=""
ROOT=""
TEXTFILE=""
ALLOW_SKIP=false

usage_error() {
	printf 'check-host-settings: %s\n' "$1" >&2
	echo "Try 'check-host-settings.sh --help' for more information." >&2
	exit 2
}

help() {
	cat <<'EOF'
Usage: check-host-settings.sh [--repo DIR]
       check-host-settings.sh --host NAME [--repo DIR] [--root DIR] [--textfile DIR] [--allow-skip]
Compare a host's settings with their files in this repository (deploy/host-settings/NAME.tsv).

  --host NAME      compare host NAME (manifest deploy/host-settings/NAME.tsv)
  --repo DIR       the repository checkout (default: this script's)
  --root DIR       prefix for every host path (tests); turns off the live reads
  --textfile DIR   write unheaded_host_settings.prom there for node_exporter
  --allow-skip     a file that cannot be read does not fail the run
  --help           this text

Examples:
  scripts/check-host-settings.sh
  sudo /usr/local/lib/unheaded/check-host-settings.sh --host west --repo /home/govan/tmp/unheaded
EOF
}

while (($# > 0)); do
	case "$1" in
	--host | --repo | --root | --textfile)
		(($# >= 2)) || usage_error "$1 needs a value"
		[[ -n "$2" ]] || usage_error "$1 needs a value"
		case "$1" in
		--host) HOST="$2" ;;
		--repo) REPO="$2" ;;
		--root) ROOT="$2" ;;
		--textfile) TEXTFILE="$2" ;;
		esac
		shift 2
		;;
	--allow-skip)
		ALLOW_SKIP=true
		shift
		;;
	-h | --help)
		help
		exit 0
		;;
	*) usage_error "unknown argument: $1" ;;
	esac
done

REPO="${REPO:-${SCRIPT_REPO}}"
[[ -d "${REPO}" ]] || usage_error "--repo ${REPO}: not a directory"
if [[ -n "${ROOT}" ]]; then
	[[ -d "${ROOT}" ]] || usage_error "--root ${ROOT}: not a directory"
	ROOT="${ROOT%/}"
fi
if [[ -n "${TEXTFILE}" ]]; then
	[[ -n "${HOST}" ]] || usage_error "--textfile needs --host"
	[[ -d "${TEXTFILE}" ]] || usage_error "--textfile ${TEXTFILE}: not a directory"
fi
if [[ -z "${HOST}" ]] && { [[ -n "${ROOT}" ]] || [[ "${ALLOW_SKIP}" == true ]]; }; then
	usage_error "--root and --allow-skip need --host"
fi

TAB=$'\t'
ROWS_H=()
ROWS_R=()
ROWS_M=()

# A path must not climb out with a .. component.
has_dotdot() {
	case "/$1/" in
	*/../*) return 0 ;;
	esac
	return 1
}

# parse_manifest FILE: fills ROWS_*; prints the first problem and returns 1.
parse_manifest() {
	local file="$1" line n=0 tabs hp rest rp mode
	ROWS_H=()
	ROWS_R=()
	ROWS_M=()
	while IFS= read -r line || [[ -n "${line}" ]]; do
		n=$((n + 1))
		[[ "${line}" =~ ^[[:space:]]*(#|$) ]] && continue
		tabs="${line//[^${TAB}]/}"
		if ((${#tabs} != 2)); then
			echo "${file}:${n}: want three tab-separated fields, found $((${#tabs} + 1))"
			return 1
		fi
		hp="${line%%"${TAB}"*}"
		rest="${line#*"${TAB}"}"
		rp="${rest%%"${TAB}"*}"
		mode="${rest#*"${TAB}"}"
		if [[ -z "${hp}" || -z "${rp}" || -z "${mode}" ]]; then
			echo "${file}:${n}: an empty field"
			return 1
		fi
		if [[ "${hp}" != /* ]] || has_dotdot "${hp}"; then
			echo "${file}:${n}: host_path must be absolute, with no .. component"
			return 1
		fi
		case "${mode}" in
		absent)
			if [[ "${rp}" != "-" ]]; then
				echo "${file}:${n}: an absent row has repo_path -"
				return 1
			fi
			;;
		exact | contains | redact)
			if [[ "${rp}" == "-" || "${rp}" == /* ]] || has_dotdot "${rp}"; then
				echo "${file}:${n}: repo_path must be relative to the repository, with no .. component"
				return 1
			fi
			;;
		*)
			echo "${file}:${n}: unknown mode '${mode}' (exact, contains, redact, absent)"
			return 1
			;;
		esac
		ROWS_H+=("${hp}")
		ROWS_R+=("${rp}")
		ROWS_M+=("${mode}")
	done <"${file}"
	if ((${#ROWS_H[@]} == 0)); then
		echo "${file}: no rows"
		return 1
	fi
	return 0
}

# ---------------------------------------------------------------------------
# The repository self-check.
# ---------------------------------------------------------------------------
self_check() {
	local dir="${REPO}/deploy/host-settings" fail=0 m problem i rp manifests=() nrows=0
	local sysctl_src="${REPO}/deploy/sysctl.d/99-unheaded-west.conf"
	local unit="${REPO}/deploy/systemd/unheaded-host-settings.service"
	local share_re='^(CPUWeight|CPUQuota|MemoryMin|MemoryLow|MemoryHigh|MemoryMax|MemorySwapMax|IOWeight|TasksMax)='
	if [[ ! -d "${dir}" ]]; then
		echo "check-host-settings: no manifest directory ${dir}" >&2
		exit 2
	fi
	shopt -s nullglob
	manifests=("${dir}"/*.tsv)
	shopt -u nullglob
	if ((${#manifests[@]} == 0)); then
		echo "check-host-settings: no manifest in ${dir}" >&2
		exit 2
	fi
	for m in "${manifests[@]}"; do
		if ! problem="$(parse_manifest "${m}")"; then
			echo "FAIL: ${problem}"
			fail=1
			continue
		fi
		parse_manifest "${m}" >/dev/null
		nrows=$((nrows + ${#ROWS_H[@]}))
		for i in "${!ROWS_H[@]}"; do
			rp="${ROWS_R[$i]}"
			[[ "${rp}" == "-" ]] && continue
			if [[ ! -f "${REPO}/${rp}" ]]; then
				echo "FAIL: ${m##*/}: ${rp} does not exist"
				fail=1
				continue
			fi
			if [[ "${rp}" == *.slice ]] && grep -qE "${share_re}" "${REPO}/${rp}"; then
				echo "FAIL: ${rp} sets a share ($(grep -oE "${share_re}" "${REPO}/${rp}" | tr -d '=' | sort -u | tr '\n' ' ')); ADR-147 decision 13: plan 8 sets the shares"
				fail=1
			fi
		done
	done
	if [[ ! -f "${sysctl_src}" ]]; then
		echo "FAIL: ${sysctl_src#"${REPO}"/} does not exist"
		fail=1
	elif ! grep -qx 'vm.nr_hugepages = 0' "${sysctl_src}"; then
		echo "FAIL: ${sysctl_src#"${REPO}"/} must say 'vm.nr_hugepages = 0' (ADR-147 decision 12: no huge pages reserved)"
		fail=1
	fi
	# The unit that runs the live check. net.* sysctls are per network
	# namespace: in a private one `sysctl -n` reads that namespace's defaults
	# (somaxconn 4096, tcp_keepalive_time 7200), not the host's, and every
	# run would report drift (review REV-1). Exit 1 is what `set -e` exits
	# with, so the unit must not count it as success (REV-2; drift is 4).
	if [[ ! -f "${unit}" ]]; then
		echo "FAIL: ${unit#"${REPO}"/} does not exist"
		fail=1
	else
		if grep -qE '^[[:space:]]*(PrivateNetwork[[:space:]]*=[[:space:]]*(yes|true|on|1)|NetworkNamespacePath[[:space:]]*=[[:space:]]*[^[:space:]]|JoinsNamespaceOf[[:space:]]*=[[:space:]]*[^[:space:]])' "${unit}"; then
			echo "FAIL: ${unit#"${REPO}"/} runs in its own network namespace (PrivateNetwork, NetworkNamespacePath or JoinsNamespaceOf): the net.* sysctls it reads would not be the host's; use RestrictAddressFamilies and IPAddressDeny instead"
			fail=1
		fi
		if grep -qE '^[[:space:]]*SuccessExitStatus[[:space:]]*=(.*[[:space:]=])?[12]([[:space:]]|$)' "${unit}"; then
			echo "FAIL: ${unit#"${REPO}"/} counts exit 1 or 2 as success (SuccessExitStatus): those are a crash and a broken manifest; drift is 4, a skip 3"
			fail=1
		fi
	fi
	if ((fail)); then
		echo "check-host-settings: FAIL"
		exit 1
	fi
	echo "check-host-settings: PASS (${#manifests[@]} manifest(s), ${nrows} rows)"
	exit 0
}

if [[ -z "${HOST}" ]]; then
	self_check
fi

# ---------------------------------------------------------------------------
# A host against its manifest.
# ---------------------------------------------------------------------------
[[ "${HOST}" =~ ^[a-z0-9][a-z0-9-]{0,62}$ ]] || usage_error "--host: want a lower-case host name"
MANIFEST="${REPO}/deploy/host-settings/${HOST}.tsv"
[[ -f "${MANIFEST}" ]] || usage_error "no manifest ${MANIFEST}"
if ! problem="$(parse_manifest "${MANIFEST}")"; then
	echo "check-host-settings: ${problem}" >&2
	exit 2
fi
parse_manifest "${MANIFEST}" >/dev/null
for i in "${!ROWS_R[@]}"; do
	rp="${ROWS_R[$i]}"
	if [[ "${rp}" != "-" && ! -f "${REPO}/${rp}" ]]; then
		echo "check-host-settings: ${rp} (row for ${ROWS_H[$i]}) does not exist in ${REPO}" >&2
		exit 2
	fi
done

DRIFTS=0
SKIPS=0
verdict() { # status, mode, what
	printf '%s\t%s\t%s\n' "$1" "$2" "$3"
	case "$1" in
	DRIFT) DRIFTS=$((DRIFTS + 1)) ;;
	SKIP) SKIPS=$((SKIPS + 1)) ;;
	esac
}

# Masks keys and passwords and drops comment and blank lines. Output goes only
# to cmp, never to the terminal.
redact() {
	sed -E 's/^([[:space:]]*(PrivateKey|PresharedKey|PublicKey))[[:space:]]*=.*/\1 = <redacted>/; s/(password)[[:space:]]+[^[:space:]]+/\1 <redacted>/' "$1" |
		grep -v -E '^[[:space:]]*(#|!|$)' || true
}

# The repository lines a contains row requires.
required_lines() {
	grep -v -E '^[[:space:]]*(#|$)' "$1" || true
}

# missing (the path does not exist), unreadable, or ok.
host_file_state() {
	local p="$1" parent
	if [[ ! -e "$p" && ! -L "$p" ]]; then
		parent="$(dirname "$p")"
		if [[ -d "${parent}" && ! -x "${parent}" ]]; then
			echo unreadable
		else
			echo missing
		fi
		return
	fi
	if [[ ! -f "$p" ]]; then
		echo missing
	elif [[ ! -r "$p" ]]; then
		echo unreadable
	else
		echo ok
	fi
}

for i in "${!ROWS_H[@]}"; do
	hp_name="${ROWS_H[$i]}"
	hp="${ROOT}${hp_name}"
	mode="${ROWS_M[$i]}"
	if [[ "${mode}" == absent ]]; then
		if [[ -e "${hp}" || -L "${hp}" ]]; then
			verdict DRIFT absent "${hp_name}"
		elif [[ -d "$(dirname "${hp}")" && ! -x "$(dirname "${hp}")" ]]; then
			verdict SKIP absent "${hp_name}"
		else
			verdict OK absent "${hp_name}"
		fi
		continue
	fi
	rp="${REPO}/${ROWS_R[$i]}"
	case "$(host_file_state "${hp}")" in
	missing)
		verdict DRIFT "${mode}" "${hp_name}"
		continue
		;;
	unreadable)
		verdict SKIP "${mode}" "${hp_name}"
		continue
		;;
	esac
	same=false
	case "${mode}" in
	exact)
		cmp -s -- "${hp}" "${rp}" && same=true
		;;
	contains)
		missing_lines="$({ grep -vxF -f "${hp}" -- <(required_lines "${rp}") || true; } | wc -l)"
		[[ "${missing_lines}" -eq 0 ]] && same=true
		;;
	redact)
		cmp -s <(redact "${hp}") <(redact "${rp}") && same=true
		;;
	esac
	if [[ "${same}" == true ]]; then
		verdict OK "${mode}" "${hp_name}"
	else
		verdict DRIFT "${mode}" "${hp_name}"
	fi
done

# ---------------------------------------------------------------------------
# The running host (only without --root).
# ---------------------------------------------------------------------------
fold_ws() {
	tr -s ' \t' '  ' | sed -E 's/^ //; s/ $//'
}

if [[ -z "${ROOT}" ]]; then
	for i in "${!ROWS_H[@]}"; do
		hp_name="${ROWS_H[$i]}"
		rp="${REPO}/${ROWS_R[$i]}"
		mode="${ROWS_M[$i]}"
		# Every key of an exact sysctl row, as the kernel runs it now. The last
		# line for a key wins, as sysctl applies them.
		if [[ "${mode}" == exact && "${hp_name}" == /etc/sysctl.d/*.conf ]]; then
			declare -A want=()
			order=()
			while IFS= read -r line || [[ -n "${line}" ]]; do
				[[ "${line}" =~ ^[[:space:]]*(#|;|$) ]] && continue
				if [[ "${line}" =~ ^[[:space:]]*-?([A-Za-z0-9_.-]+)[[:space:]]*=[[:space:]]*(.*)$ ]]; then
					key="${BASH_REMATCH[1]}"
					[[ -v "want[${key}]" ]] || order+=("${key}")
					want["${key}"]="$(printf '%s' "${BASH_REMATCH[2]}" | fold_ws)"
				fi
			done <"${rp}"
			for key in "${order[@]}"; do
				if ! live="$(sysctl -n -- "${key}" 2>/dev/null)"; then
					verdict SKIP sysctl "${key}"
					continue
				fi
				live="$(printf '%s' "${live}" | fold_ws)"
				if [[ "${live}" == "${want[${key}]}" ]]; then
					verdict OK sysctl "${key}"
				else
					verdict DRIFT sysctl "${key}"
				fi
			done
			unset want order
		fi
		# Every bind line of the fstab row is mounted.
		if [[ "${mode}" == contains && "${hp_name}" == /etc/fstab ]]; then
			while read -r _src target _fstype opts _rest; do
				[[ -z "${target:-}" ]] && continue
				[[ ",${opts:-}," == *,bind,* ]] || continue
				if ! command -v findmnt >/dev/null 2>&1; then
					verdict SKIP mount "${target}"
				elif [[ -n "$(findmnt -no SOURCE --mountpoint "${target}" 2>/dev/null || true)" ]]; then
					verdict OK mount "${target}"
				else
					verdict DRIFT mount "${target}"
				fi
			done < <(required_lines "${rp}")
		fi
	done
	# The active swap list. West's is the NVMe file alone (decision 12); a host
	# with no entry here has no swap row (east's is plan 6's).
	expect_swaps=""
	case "${HOST}" in
	west) expect_swaps="/swap.img" ;;
	esac
	if [[ -n "${expect_swaps}" ]]; then
		if [[ ! -r /proc/swaps ]]; then
			verdict SKIP swap /proc/swaps
		elif [[ "$(awk 'NR>1{print $1}' /proc/swaps | sort | tr '\n' ' ' | sed 's/ $//')" == "${expect_swaps}" ]]; then
			verdict OK swap "${expect_swaps}"
		else
			verdict DRIFT swap /proc/swaps
		fi
	fi
fi

if [[ -n "${TEXTFILE}" ]]; then
	tmp="$(mktemp "${TEXTFILE}/.unheaded_host_settings.prom.XXXXXX")"
	{
		echo '# HELP unheaded_host_settings_drift_files Host settings that differ from their file in the repository (scripts/check-host-settings.sh).'
		echo '# TYPE unheaded_host_settings_drift_files gauge'
		printf 'unheaded_host_settings_drift_files{host="%s"} %d\n' "${HOST}" "${DRIFTS}"
		echo '# HELP unheaded_host_settings_skipped_files Host settings the check could not read.'
		echo '# TYPE unheaded_host_settings_skipped_files gauge'
		printf 'unheaded_host_settings_skipped_files{host="%s"} %d\n' "${HOST}" "${SKIPS}"
		echo '# HELP unheaded_host_settings_checked_timestamp_seconds When the host-settings check last completed.'
		echo '# TYPE unheaded_host_settings_checked_timestamp_seconds gauge'
		printf 'unheaded_host_settings_checked_timestamp_seconds{host="%s"} %d\n' "${HOST}" "$(date +%s)"
	} >"${tmp}"
	chmod 0644 "${tmp}"
	mv -f "${tmp}" "${TEXTFILE}/unheaded_host_settings.prom"
fi

if ((DRIFTS > 0)); then
	exit 4
fi
if ((SKIPS > 0)) && [[ "${ALLOW_SKIP}" != true ]]; then
	exit 3
fi
exit 0
