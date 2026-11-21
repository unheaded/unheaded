#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# install-prerequisites.sh — what a fresh Ubuntu host needs before the build,
# test and compose steps of QUICKSTART.md.
#
# It installs distribution packages by name, two release archives whose
# sha256 is pinned, and a few files from this repository. It starts no part
# of the stack: the script itself starts, enables and restarts nothing (the
# Docker packages start the Docker daemon when they are installed, as Ubuntu's
# service packages do). It applies no kernel setting, fetches no container
# image, and never replaces a file or toolchain that is already there and
# differs: that is reported as "kept", with what to do about it.
#
# Usage:
#   sudo scripts/install-prerequisites.sh [--with PART]... [--without PART]...
#   scripts/install-prerequisites.sh --dry-run [--with PART]... [--without PART]...
#
#   --dry-run       print each step as "would ...", change nothing; any user
#   --with PART     add an optional part
#   --without PART  leave a default part out
#   -h, --help      this text
#
# Parts, in the order they run:
#   packages       default   distribution packages (BASE_PACKAGES)
#   docker         default   deploy/docker/daemon.json to DOCKER_DAEMON_JSON;
#                            Docker Engine with the compose and buildx plugins
#                            (DOCKER_PACKAGES), only when no docker is installed
#   go             default   the Go release go.mod's toolchain line names, from
#                            GO_BASE_URL, to GO_INSTALL_DIR/go; GO_PROFILE puts
#                            it on PATH
#   rust           default   rustup (RUST_PACKAGES); toolchains belong to a
#                            user, so the commands for them are printed, not run
#   ebpf           optional  bpftool (EBPF_PACKAGES), for scripts/load-ebpf.sh
#   node-exporter  optional  node_exporter NODE_EXPORTER_VERSION to
#                            NODE_EXPORTER_BIN, its account
#                            (deploy/sysusers.d/unheaded.conf) and
#                            deploy/systemd/node-exporter.service; not started
#   sysctl         optional  SYSCTL_SOURCE (default: the file named after this
#                            host in deploy/sysctl.d/) to SYSCTL_TARGET; read
#                            by the kernel at the next boot
#
# Each name in capitals is a variable at the top of this file; a value in the
# environment replaces the default:
#   sudo env NODE_EXPORTER_SHA256=<sha256> scripts/install-prerequisites.sh --with node-exporter
#
# For the test (tests/scripts/test_install_prerequisites.sh), not for a host:
#   --root DIR      take every host path under DIR; root is then not required,
#                   and is refused: apt-get and the systemd tools are whatever
#                   PATH gives and are not confined to DIR
#
# Each step prints one line: "ok" (already as wanted), "would" (dry run),
# "changed", or "kept" (there, different, left alone).
# Exit: 0 done (kept lines included), 1 refused or failed, 2 usage.

set -euo pipefail

# ── values: change them here or in the environment ───────────────────────

# Ubuntu releases this script installs on (package names checked on 25.10).
: "${UBUNTU_MIN:=24.04}"

# packages: curl (this script's downloads, QUICKSTART's health checks), git,
# make (Makefile), gcc and libc6-dev (cgo: go test -race; the Rust linker),
# python3 (scripts/gen-dev-keys.sh, scripts/gen-dev-certs.sh), jq (scripts/).
: "${BASE_PACKAGES:=ca-certificates curl git make gcc libc6-dev python3 jq}"

# docker: the distribution's own packages. A host that takes Docker from
# another repository installs it first; this script then leaves it alone.
# DOCKER is the command asked whether Docker is already there.
: "${DOCKER_PACKAGES:=docker.io docker-compose-v2 docker-buildx}"
: "${DOCKER_DAEMON_JSON:=/etc/docker/daemon.json}"
: "${DOCKER:=docker}"

# go: version and sha256 change together; the sha256 is the one go.dev/dl
# lists for go<version>.linux-amd64.tar.gz. The version is go.mod's toolchain
# line (the test fails when they differ).
: "${GO_VERSION:=1.27.4}"
: "${GO_SHA256:=65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40}"
: "${GO_BASE_URL:=https://go.dev/dl}"
: "${GO_INSTALL_DIR:=/usr/local}"
: "${GO_PROFILE:=/etc/profile.d/golang.sh}"

# rust: rustup only. BPF_LINKER_VERSION is what the eBPF workflow installs
# (.github/workflows/ebpf.yml); it is printed, not installed.
: "${RUST_PACKAGES:=rustup}"
: "${BPF_LINKER_VERSION:=0.10.2}"

# ebpf
: "${EBPF_PACKAGES:=bpftool}"

# node-exporter: version and sha256 change together; the sha256 is the one
# the release's sha256sums.txt lists for node_exporter-<version>.linux-amd64.tar.gz.
# No sha256 is recorded in this repository yet, so the part refuses to
# download until one is given.
: "${NODE_EXPORTER_VERSION:=1.12.1}"
: "${NODE_EXPORTER_SHA256:=}"
: "${NODE_EXPORTER_BASE_URL:=https://github.com/prometheus/node_exporter/releases/download}"
: "${NODE_EXPORTER_BIN:=/usr/local/bin/node_exporter}"
: "${NODE_EXPORTER_UNIT:=/etc/systemd/system/node-exporter.service}"
: "${NODE_EXPORTER_USER:=node-exporter}"
: "${SYSUSERS_TARGET:=/etc/sysusers.d/unheaded.conf}"

# sysctl: SYSCTL_SOURCE is a path in this checkout or an absolute path. Unset,
# it is deploy/sysctl.d/99-unheaded-<short host name>.conf (wiki/Host-Settings.md).
: "${SYSCTL_SOURCE:=}"
: "${SYSCTL_TARGET:=/etc/sysctl.d/99-unheaded.conf}"

# ── arguments ────────────────────────────────────────────────────────────

me=install-prerequisites
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

default_parts="packages docker go rust"
optional_parts="ebpf node-exporter sysctl"
parts=" $default_parts "
dry_run=0
root=""

usage_error() {
    echo "$me: $1" >&2
    echo "Try '$me.sh --help' for more information." >&2
    exit 2
}
known_part() { case " $default_parts $optional_parts " in *" $1 "*) return 0 ;; esac; return 1; }

while [ $# -gt 0 ]; do
    case "$1" in
        --dry-run) dry_run=1; shift ;;
        --with|--without)
            [ $# -ge 2 ] || usage_error "$1 needs a part: $default_parts $optional_parts"
            known_part "$2" || usage_error "unknown part '$2' (parts: $default_parts $optional_parts)"
            parts="${parts// $2 / }"
            [ "$1" = --with ] && parts="$parts$2 "
            shift 2 ;;
        --root)
            if [ $# -lt 2 ] || [ -z "$2" ]; then usage_error "--root needs a directory"; fi
            root="${2%/}"; shift 2 ;;
        -h|--help)
            sed -n '/^# install-prerequisites.sh /,/^# Exit:/p' "$0" | sed 's/^# \{0,1\}//'
            exit 0 ;;
        *) usage_error "unknown argument '$1'" ;;
    esac
done

part_on() { case "$parts" in *" $1 "*) return 0 ;; esac; return 1; }
dry() { [ "$dry_run" -eq 1 ]; }
die() { echo "$me: $*" >&2; exit 1; }

# ── refusals: all of them before anything changes ────────────────────────

uid="$(id -u)"
if [ -n "$root" ]; then
    [ "$uid" -ne 0 ] || die "--root is for the test and is refused for root: its package and systemd commands are not confined to $root"
    [ -d "$root" ] || die "--root $root is not a directory"
elif ! dry && [ "$uid" -ne 0 ]; then
    die "run as root (sudo $0), or add --dry-run to see what it would do without changing anything"
fi

os_release="$root/etc/os-release"
[ -r "$os_release" ] || die "cannot read $os_release: this script knows Ubuntu $UBUNTU_MIN and later only"
os_id="$(sed -n 's/^ID=//p' "$os_release" | tr -d '"' | head -n1)"
os_version="$(sed -n 's/^VERSION_ID=//p' "$os_release" | tr -d '"' | head -n1)"
version_at_least() { [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]; }
if [ "$os_id" != ubuntu ] || ! version_at_least "${os_version:-0}" "$UBUNTU_MIN"; then
    die "this host is ${os_id:-unknown} ${os_version:-unknown}; this script knows Ubuntu $UBUNTU_MIN and later only. On another system install what --help lists with its own package manager"
fi

is_sha256() { case "$1" in *[!0-9a-f]*) return 1 ;; esac; [ "${#1}" -eq 64 ]; }

if part_on go || part_on node-exporter; then
    arch="$(uname -m)"
    [ "$arch" = x86_64 ] || die "this host is $arch; the pinned Go and node_exporter archives are linux-amd64 (leave them out: --without go, and no --with node-exporter)"
fi

if part_on docker; then
    [ -f "$repo/deploy/docker/daemon.json" ] || die "$repo/deploy/docker/daemon.json is missing: run this script from a full checkout"
fi

if part_on go; then
    is_sha256 "$GO_SHA256" || die "GO_SHA256 is not a sha256 (64 lowercase hex digits); GO_VERSION and GO_SHA256 change together"
fi

if part_on node-exporter; then
    for f in deploy/sysusers.d/unheaded.conf deploy/systemd/node-exporter.service; do
        [ -f "$repo/$f" ] || die "$repo/$f is missing: run this script from a full checkout"
    done
    if [ ! -e "$root$NODE_EXPORTER_BIN" ] && [ ! -L "$root$NODE_EXPORTER_BIN" ] && ! is_sha256 "$NODE_EXPORTER_SHA256"; then
        die "NODE_EXPORTER_SHA256 is not set to a sha256: take the line for node_exporter-$NODE_EXPORTER_VERSION.linux-amd64.tar.gz from $NODE_EXPORTER_BASE_URL/v$NODE_EXPORTER_VERSION/sha256sums.txt and give it in the environment (see --help); nothing is downloaded unchecked"
    fi
    if ! getent passwd "$NODE_EXPORTER_USER" >/dev/null 2>&1 && [ -e "$root$SYSUSERS_TARGET" ] \
        && ! cmp -s "$repo/deploy/sysusers.d/unheaded.conf" "$root$SYSUSERS_TARGET"; then
        die "the account $NODE_EXPORTER_USER does not exist and $root$SYSUSERS_TARGET differs from deploy/sysusers.d/unheaded.conf; compare the two, install the repository's file, then run again"
    fi
fi

sysctl_src=""
if part_on sysctl; then
    sysctl_src="${SYSCTL_SOURCE:-deploy/sysctl.d/99-unheaded-$(hostname -s).conf}"
    case "$sysctl_src" in /*) ;; *) sysctl_src="$repo/$sysctl_src" ;; esac
    [ -f "$sysctl_src" ] || die "no sysctl source for this host: $sysctl_src (copy deploy/sysctl.d/99-unheaded-west.conf to that name and edit it, or set SYSCTL_SOURCE)"
fi

# ── helpers ──────────────────────────────────────────────────────────────

n_ok=0 n_changed=0 n_kept=0
notes=()
last=""   # what the last step reported: ok, changed or kept

ok() { echo "ok       $*"; n_ok=$((n_ok + 1)); last=ok; }
kept() { echo "kept     $*"; n_kept=$((n_kept + 1)); last=kept; }
changed() { # in a dry run the same line reads "would"
    if dry; then echo "would    $*"; else echo "changed  $*"; fi
    n_changed=$((n_changed + 1)); last=changed
}
note() { notes+=("$*"); }

work=""
stages=()
cleanup() {
    [ -z "$work" ] || rm -rf "$work"
    local s
    for s in "${stages[@]+"${stages[@]}"}"; do rm -rf "$s"; done
}
trap cleanup EXIT

# Files this script writes belong to root; under --root (the test) they keep
# the caller's ownership.
owner=()
[ "$uid" -ne 0 ] || owner=(-o root -g root)

pkg_installed() { [ "$(dpkg-query -W -f='${db:Status-Status}' "$1" 2>/dev/null || true)" = installed ]; }

apt_refreshed=0
install_packages() { # LABEL PACKAGE...
    local label="$1" p missing=()
    shift
    for p in "$@"; do pkg_installed "$p" || missing+=("$p"); done
    if [ "${#missing[@]}" -eq 0 ]; then
        ok "$label: installed ($*)"
        return
    fi
    if ! dry; then
        if [ "$apt_refreshed" -eq 0 ]; then
            apt-get update || die "apt-get update failed"
            apt_refreshed=1
        fi
        DEBIAN_FRONTEND=noninteractive apt-get install -y "${missing[@]}" \
            || die "apt-get install failed: ${missing[*]}"
    fi
    changed "$label: apt-get install ${missing[*]}"
}

place_file() { # LABEL SOURCE TARGET MODE [SHOWN]: install SOURCE as TARGET unless something is there
    local label="$1" src="$2" dst="$3" mode="$4"
    local shown="${5:-${src#"$repo"/}}"
    if [ -e "$dst" ] || [ -L "$dst" ]; then
        if cmp -s "$src" "$dst"; then
            ok "$label: $dst"
        else
            kept "$label: $dst is there and differs from $shown; not replaced. Compare the two and install the one you want"
        fi
        return
    fi
    dry || install -D -m "$mode" "${owner[@]+"${owner[@]}"}" "$src" "$dst"
    changed "$label: $shown to $dst (mode $mode)"
}

fetch_verified() { # URL SHA256 OUT: download, and keep the file only if its sha256 matches
    local url="$1" sha="$2" out="$3"
    curl --proto '=https' --tlsv1.2 -fsSL --retry 3 -o "$out" "$url" || die "download failed: $url"
    if ! echo "$sha  $out" | sha256sum -c --quiet - >/dev/null 2>&1; then
        rm -f "$out"
        die "${url##*/} does not match its pinned sha256 ($sha); nothing was installed from it"
    fi
}

# ── the parts ────────────────────────────────────────────────────────────

do_packages() {
    # shellcheck disable=SC2086  # a list of package names
    install_packages "packages" $BASE_PACKAGES
}

do_docker() {
    # The daemon's config first: a daemon installed below starts with it.
    place_file "docker daemon config" "$repo/deploy/docker/daemon.json" "$root$DOCKER_DAEMON_JSON" 0644
    local config="$last"
    if command -v "$DOCKER" >/dev/null 2>&1; then
        if "$DOCKER" compose version >/dev/null 2>&1; then
            ok "docker: already installed ($("$DOCKER" --version 2>/dev/null | head -n1)), with the compose plugin"
        else
            kept "docker: already installed, without the compose plugin; nothing is installed over it. Add the plugin that belongs to your Docker packages (docker-compose-v2 with docker.io, docker-compose-plugin with docker-ce)"
        fi
        [ "$config" != changed ] || note "Docker is already installed: it reads $DOCKER_DAEMON_JSON at its next restart, which this script does not do."
    else
        # shellcheck disable=SC2086  # a list of package names
        install_packages "docker" $DOCKER_PACKAGES
        [ "$last" != changed ] || note "Docker needs root: run compose with sudo. This script puts nobody in the docker group, which is root-equivalent."
    fi
}

do_go() {
    local goroot="$root$GO_INSTALL_DIR/go" archive="go$GO_VERSION.linux-amd64.tar.gz" have stage
    if [ -e "$goroot" ] || [ -L "$goroot" ]; then
        have="$(head -n1 "$goroot/VERSION" 2>/dev/null || true)"
        if [ "$have" = "go$GO_VERSION" ]; then
            ok "go: go$GO_VERSION in $goroot"
        else
            kept "go: $goroot holds ${have:-something that is not a Go release}, not go$GO_VERSION; not replaced. Go 1.21 and later fetch go.mod's toolchain by themselves; to have the pinned release, move $goroot away and run again"
        fi
    else
        if ! dry; then
            fetch_verified "$GO_BASE_URL/$archive" "$GO_SHA256" "$work/$archive"
            mkdir -p "$root$GO_INSTALL_DIR"
            stage="$(mktemp -d "$root$GO_INSTALL_DIR/.install-prerequisites.XXXXXX")"
            stages+=("$stage")
            tar -xzf "$work/$archive" -C "$stage" --no-same-owner
            [ -x "$stage/go/bin/go" ] || die "$archive holds no go/bin/go; nothing was installed from it"
            mv "$stage/go" "$goroot"
            rmdir "$stage"
        fi
        changed "go: $GO_BASE_URL/$archive, sha256 checked, to $goroot"
    fi

    printf 'export GOROOT=%s/go\nexport GOPATH=$HOME/go\nexport PATH=$GOROOT/bin:$GOPATH/bin:$PATH\n' \
        "$GO_INSTALL_DIR" >"$work/golang.sh"
    place_file "go on PATH" "$work/golang.sh" "$root$GO_PROFILE" 0644 "three export lines (GOROOT, GOPATH, PATH)"
    [ "$last" != changed ] || note "Go is on PATH in a new login shell, or now with: . $GO_PROFILE"
}

do_rust() {
    # The person at the keyboard: whoever called sudo, else the caller.
    local who="${SUDO_USER:-}" home
    [ -n "$who" ] || who="$(id -un)"
    home="$(getent passwd "$who" 2>/dev/null | cut -d: -f6 || true)"
    if [ -n "$home" ] && [ -x "$root$home/.cargo/bin/rustup" ]; then
        ok "rust: $who already has rustup ($home/.cargo/bin/rustup)"
        return
    fi
    # shellcheck disable=SC2086  # a list of package names
    install_packages "rust" $RUST_PACKAGES
    [ "$last" != changed ] || note "Rust toolchains belong to a user. As yourself, not root: rustup default stable   (cargo, for 'make build')"
}

do_ebpf() {
    # shellcheck disable=SC2086  # a list of package names
    install_packages "ebpf" $EBPF_PACKAGES
    note "To build the eBPF programs ('make ebpf'), as yourself, not root: rustup toolchain install nightly --component rust-src && cargo install bpf-linker@$BPF_LINKER_VERSION --locked"
}

do_node_exporter() {
    local bin="$root$NODE_EXPORTER_BIN" name="node_exporter-$NODE_EXPORTER_VERSION.linux-amd64" have
    if getent passwd "$NODE_EXPORTER_USER" >/dev/null 2>&1; then
        ok "account $NODE_EXPORTER_USER: exists"
    else
        place_file "service accounts" "$repo/deploy/sysusers.d/unheaded.conf" "$root$SYSUSERS_TARGET" 0644
        if ! dry; then
            systemd-sysusers "$root$SYSUSERS_TARGET" || die "systemd-sysusers failed on $root$SYSUSERS_TARGET"
            getent passwd "$NODE_EXPORTER_USER" >/dev/null 2>&1 \
                || die "systemd-sysusers ran, and the account $NODE_EXPORTER_USER still does not exist"
        fi
        changed "account $NODE_EXPORTER_USER: systemd-sysusers $SYSUSERS_TARGET (every service account in that file, each without a login)"
    fi

    if [ -e "$bin" ] || [ -L "$bin" ]; then
        have="$("$bin" --version 2>&1 | head -n1 || true)"
        case "$have" in
            *"version $NODE_EXPORTER_VERSION "*) ok "node_exporter $NODE_EXPORTER_VERSION: $bin" ;;
            *) kept "node_exporter: $bin answers '${have:-nothing}', not version $NODE_EXPORTER_VERSION; not replaced. To have the pinned release, move it away and run again" ;;
        esac
    else
        if ! dry; then
            fetch_verified "$NODE_EXPORTER_BASE_URL/v$NODE_EXPORTER_VERSION/$name.tar.gz" "$NODE_EXPORTER_SHA256" "$work/$name.tar.gz"
            tar -xzf "$work/$name.tar.gz" -C "$work" --no-same-owner "$name/node_exporter" \
                || die "$name.tar.gz holds no $name/node_exporter; nothing was installed from it"
            install -D -m 0755 "${owner[@]+"${owner[@]}"}" "$work/$name/node_exporter" "$bin"
        fi
        changed "node_exporter $NODE_EXPORTER_VERSION: $NODE_EXPORTER_BASE_URL/v$NODE_EXPORTER_VERSION/$name.tar.gz, sha256 checked, to $bin"
    fi

    place_file "node-exporter unit" "$repo/deploy/systemd/node-exporter.service" "$root$NODE_EXPORTER_UNIT" 0644
    if [ "$last" = changed ]; then
        dry || systemctl daemon-reload || die "systemctl daemon-reload failed"
        note "node-exporter is installed and not started. It listens on every address, port 9100, and this script sets no firewall. When that is what you want: systemctl enable --now node-exporter"
    fi
}

do_sysctl() {
    place_file "sysctl" "$sysctl_src" "$root$SYSCTL_TARGET" 0644
    [ "$last" != changed ] || note "$SYSCTL_TARGET is read at the next boot. To apply it now: sysctl --system"
}

# ── run ──────────────────────────────────────────────────────────────────

# What is unpacked below must be readable by every user, whatever root's umask.
umask 022
work="$(mktemp -d)"

if dry; then echo "$me: dry run on $os_id $os_version; parts:$parts"; else echo "$me: $os_id $os_version; parts:$parts"; fi

! part_on packages || do_packages
! part_on docker || do_docker
! part_on go || do_go
! part_on rust || do_rust
! part_on ebpf || do_ebpf
! part_on node-exporter || do_node_exporter
! part_on sysctl || do_sysctl

if [ "${#notes[@]}" -gt 0 ] && [ "$n_changed" -gt 0 ]; then
    echo "next:"
    for n in "${notes[@]}"; do echo "  - $n"; done
fi

if dry; then
    echo "$me: dry run, nothing changed ($n_changed would change, $n_ok already in place, $n_kept kept)"
elif [ "$n_changed" -eq 0 ]; then
    echo "$me: nothing to do ($n_ok already in place, $n_kept kept)"
else
    echo "$me: $n_changed changed, $n_ok already in place, $n_kept kept. No part of the stack was started; QUICKSTART.md goes on from here"
fi
