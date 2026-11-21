#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test_install_prerequisites.sh — scripts/install-prerequisites.sh against a
# throwaway directory tree, with the package manager, docker, curl and the
# systemd tools replaced by stubs on PATH. No root, no network, nothing
# outside its temp directory.
#
# Holds the script to: a dry run changes nothing and names each step; a real
# run without root refuses in one line; a second run changes nothing and says
# so; a download that does not match its sha256 is refused and leaves nothing;
# a file or toolchain already on the host that differs is kept, not replaced;
# nothing is started; no floating image tag, no pipe into a shell and no
# retired component in its text; its Go pin is go.mod's toolchain line.

set -euo pipefail
cd "$(dirname "$0")/../.."

script=scripts/install-prerequisites.sh
fail() { echo "FAIL: $*" >&2; exit 1; }

[ -f "$script" ] || fail "$script does not exist"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# ── stubs ────────────────────────────────────────────────────────────────
# Each stub writes its name and arguments to $STUB_LOG. State (which packages
# are "installed", which accounts "exist") is kept in files under $STATE, so
# a second run of the script sees what the first one did.
stubs="$tmp/stubs"
mkdir -p "$stubs"
real_uname="$(command -v uname)"
real_getent="$(command -v getent)"

mkstub() { # NAME, body on stdin
    { printf '#!/usr/bin/env bash\necho "%s $*" >>"$STUB_LOG"\n' "$1"; cat; } >"$stubs/$1"
    chmod +x "$stubs/$1"
}

mkstub apt-get <<'EOF'
[ "${1:-}" = install ] || exit 0
shift
for a in "$@"; do
    case "$a" in -*) continue ;; esac
    echo "$a" >>"$STATE/pkgs"
    # Installing Docker puts a docker on PATH, as the package would.
    if [ "$a" = docker.io ]; then cp "$STATE/docker.stub" "$(dirname "$0")/docker"; fi
done
EOF

mkstub dpkg-query <<'EOF'
pkg="${!#}"
grep -qxF "$pkg" "$STATE/pkgs" 2>/dev/null || exit 1
printf 'installed'
EOF

mkstub curl <<'EOF'
out="" url=""
while [ $# -gt 0 ]; do
    case "$1" in
        -o) out="$2"; shift 2 ;;
        http*) url="$1"; shift ;;
        *) shift ;;
    esac
done
[ -n "$out" ] && [ -f "$FIXTURES/${url##*/}" ] || exit 22
cp "$FIXTURES/${url##*/}" "$out"
EOF

mkstub systemctl <<'EOF'
exit 0
EOF

mkstub systemd-sysusers <<'EOF'
# The account the unit runs as, as the real tool would make it from the file.
echo node-exporter >>"$STATE/users"
EOF

mkstub sysctl <<'EOF'
exit 0
EOF

cat >"$stubs/getent" <<EOF
#!/usr/bin/env bash
if [ "\${1:-}" = passwd ] && [ -n "\${2:-}" ]; then
    grep -qxF "\$2" "\$STATE/users" 2>/dev/null && { echo "\$2:x:16000:16000::/:/usr/sbin/nologin"; exit 0; }
    exit 2
fi
exec "$real_getent" "\$@"
EOF
cat >"$stubs/uname" <<EOF
#!/usr/bin/env bash
if [ "\${1:-}" = -m ]; then echo "\${STUB_ARCH:-x86_64}"; exit 0; fi
exec "$real_uname" "\$@"
EOF
cat >"$stubs/hostname" <<'EOF'
#!/usr/bin/env bash
echo testhost
EOF
chmod +x "$stubs/getent" "$stubs/uname" "$stubs/hostname"

# ── fixtures: two small archives shaped like the releases ────────────────
go_version="$(sed -n 's/^toolchain go//p' go.mod)"
[ -n "$go_version" ] || fail "go.mod has no toolchain line"
ne_version=9.9.9

fixtures="$tmp/fixtures"
mkdir -p "$fixtures/src/go/bin" "$fixtures/src/node_exporter-$ne_version.linux-amd64"
echo "go$go_version" >"$fixtures/src/go/VERSION"
printf '#!/bin/sh\necho "go version go%s linux/amd64"\n' "$go_version" >"$fixtures/src/go/bin/go"
printf '#!/bin/sh\necho "node_exporter, version %s (branch: HEAD, revision: fixture)"\n' "$ne_version" \
    >"$fixtures/src/node_exporter-$ne_version.linux-amd64/node_exporter"
chmod +x "$fixtures/src/go/bin/go" "$fixtures/src/node_exporter-$ne_version.linux-amd64/node_exporter"
tar -czf "$fixtures/go$go_version.linux-amd64.tar.gz" -C "$fixtures/src" go
tar -czf "$fixtures/node_exporter-$ne_version.linux-amd64.tar.gz" -C "$fixtures/src" "node_exporter-$ne_version.linux-amd64"
go_sha="$(sha256sum "$fixtures/go$go_version.linux-amd64.tar.gz" | cut -d' ' -f1)"
ne_sha="$(sha256sum "$fixtures/node_exporter-$ne_version.linux-amd64.tar.gz" | cut -d' ' -f1)"

# ── a fresh "host" ───────────────────────────────────────────────────────
new_host() { # DIR [ID VERSION_ID]
    rm -rf "$1" "$tmp/state" "$stubs/docker"
    mkdir -p "$1/etc" "$tmp/state"
    printf 'ID=%s\nVERSION_ID="%s"\nPRETTY_NAME="fixture"\n' "${2:-ubuntu}" "${3:-25.10}" >"$1/etc/os-release"
    : >"$tmp/log"
    cat >"$tmp/state/docker.stub" <<'EOF'
#!/usr/bin/env bash
echo "docker $*" >>"$STUB_LOG"
case "${1:-}" in
    --version) echo "Docker version 0.0.0, build fixture" ;;
    compose) [ "${STUB_NO_COMPOSE:-0}" = 1 ] && exit 1; echo "Docker Compose version v0.0.0" ;;
esac
EOF
    chmod +x "$tmp/state/docker.stub"
}

# The script under the stubs, with the fixture pins. DOCKER names the stub by
# path: a docker that happens to be on this machine is never asked anything.
# SUDO_USER is unset so the run never looks into a real home directory.
run() {
    env -u SUDO_USER \
        PATH="$stubs:$PATH" STUB_LOG="$tmp/log" STATE="$tmp/state" FIXTURES="$fixtures" \
        DOCKER="$stubs/docker" \
        GO_SHA256="$go_sha" NODE_EXPORTER_VERSION="$ne_version" NODE_EXPORTER_SHA256="$ne_sha" \
        "$@"
}
all_parts=(--with ebpf --with node-exporter --with sysctl)
tree() { (cd "$1" && find . -printf '%p %y %m %s\n' | LC_ALL=C sort; find . -type f -exec sha256sum {} + | LC_ALL=C sort); }
changing_calls() { grep -E '^(apt-get|curl|systemctl|systemd-sysusers|sysctl) ' "$tmp/log" || true; }

host="$tmp/host"
sysctl_src="$tmp/99-unheaded-testhost.conf"
printf '# fixture\nvm.swappiness = 10\n' >"$sysctl_src"

# ── --help ───────────────────────────────────────────────────────────────
help="$(bash "$script" --help)" || fail "--help failed"
for word in Usage --dry-run --with --without packages docker go rust ebpf node-exporter sysctl; do
    printf '%s\n' "$help" | grep -q -- "$word" || fail "--help does not mention $word"
done
[ "$(bash "$script" -h)" = "$help" ] || fail "-h and --help differ"

# ── usage errors: exit 2, one line, nothing run ──────────────────────────
new_host "$host"
for bad in "--nope" "--with nonsense" "--without nonsense" "--with" "--root" "extra"; do
    rc=0
    # shellcheck disable=SC2086  # $bad is two words on purpose
    run bash "$script" --root "$host" $bad >"$tmp/out" 2>"$tmp/err" || rc=$?
    [ "$rc" -eq 2 ] || fail "'$bad': exit $rc, want 2"
    [ "$(wc -l <"$tmp/err")" -le 2 ] || fail "'$bad': more than a line and a hint on stderr"
    [ ! -s "$tmp/log" ] || fail "'$bad': a command ran before the usage error: $(cat "$tmp/log")"
done

# ── a real run without root refuses in one line ──────────────────────────
if [ "$(id -u)" -ne 0 ]; then
    new_host "$host"
    rc=0
    run bash "$script" >"$tmp/out" 2>"$tmp/err" || rc=$?
    [ "$rc" -eq 1 ] || fail "non-root real run: exit $rc, want 1"
    [ "$(wc -l <"$tmp/err")" -eq 1 ] || fail "non-root refusal is not one line: $(cat "$tmp/err")"
    grep -q 'root' "$tmp/err" || fail "non-root refusal does not say root"
    grep -q -- '--dry-run' "$tmp/err" || fail "non-root refusal does not name --dry-run"
    [ ! -s "$tmp/out" ] || fail "non-root refusal printed a plan"
    [ -z "$(changing_calls)" ] || fail "non-root refusal ran: $(changing_calls)"
else
    echo "skip: running as root, the non-root refusal is not exercised"
fi

# ── --dry-run: names each step, changes nothing ──────────────────────────
new_host "$host"
before="$(tree "$host")"
run env SYSCTL_SOURCE="$sysctl_src" bash "$script" --root "$host" --dry-run "${all_parts[@]}" >"$tmp/out" 2>"$tmp/err" \
    || fail "dry run failed: $(cat "$tmp/err")"
[ "$(tree "$host")" = "$before" ] || fail "the dry run changed the tree"
[ -z "$(changing_calls)" ] || fail "the dry run ran: $(changing_calls)"
for step in \
    'packages' 'ca-certificates' 'docker.io docker-compose-v2 docker-buildx' '/etc/docker/daemon.json' \
    "go$go_version" '/etc/profile.d/golang.sh' 'rustup' 'bpftool' \
    "node_exporter $ne_version" '/etc/sysusers.d/unheaded.conf' '/etc/systemd/system/node-exporter.service' \
    '/etc/sysctl.d/99-unheaded.conf'; do
    grep -qF -- "$step" "$tmp/out" || fail "the dry run does not name: $step"
done
grep -q '^would ' "$tmp/out" || fail "the dry run has no 'would' line"
if grep -q '^changed ' "$tmp/out"; then fail "the dry run claims a change"; fi
grep -qi 'dry run' "$tmp/out" || fail "the dry run does not say it is one"
grep -q 'nothing changed' "$tmp/out" || fail "the dry run does not say nothing changed"

# Default parts only: the optional ones are not in the plan.
new_host "$host"
run bash "$script" --root "$host" --dry-run >"$tmp/out" 2>"$tmp/err" || fail "default dry run failed: $(cat "$tmp/err")"
if grep -q 'bpftool\|node_exporter [0-9]\|sysctl.d' "$tmp/out"; then fail "an optional part is in the default plan"; fi
# --without removes a default part.
run bash "$script" --root "$host" --dry-run --without docker --without rust >"$tmp/out" 2>"$tmp/err" || fail "--without failed"
if grep -q 'docker.io\|daemon.json\|rustup' "$tmp/out"; then fail "--without left the part in the plan"; fi
grep -qF "go$go_version" "$tmp/out" || fail "--without removed a part it did not name"

# ── a real run on a fresh host (under --root, with the stubs) ────────────
new_host "$host"
run env SYSCTL_SOURCE="$sysctl_src" bash "$script" --root "$host" "${all_parts[@]}" >"$tmp/out" 2>"$tmp/err" \
    || fail "real run failed: $(cat "$tmp/err")"
cmp -s deploy/docker/daemon.json "$host/etc/docker/daemon.json" || fail "daemon.json is not the repository's"
[ "$(cat "$host/usr/local/go/VERSION")" = "go$go_version" ] || fail "Go is not in place"
grep -qx 'export GOROOT=/usr/local/go' "$host/etc/profile.d/golang.sh" || fail "golang.sh does not name the Go it installed"
grep -q '^export PATH=\$GOROOT/bin:' "$host/etc/profile.d/golang.sh" || fail "golang.sh does not put Go on PATH"
[ -x "$host/usr/local/bin/node_exporter" ] || fail "node_exporter is not in place or not executable"
cmp -s deploy/systemd/node-exporter.service "$host/etc/systemd/system/node-exporter.service" || fail "the unit is not the repository's"
cmp -s deploy/sysusers.d/unheaded.conf "$host/etc/sysusers.d/unheaded.conf" || fail "the sysusers file is not the repository's"
cmp -s "$sysctl_src" "$host/etc/sysctl.d/99-unheaded.conf" || fail "the sysctl file is not the source"
for pkg in ca-certificates curl git make gcc docker.io docker-compose-v2 docker-buildx rustup bpftool; do
    grep -qxF "$pkg" "$tmp/state/pkgs" || fail "package not installed: $pkg"
done
grep -q '^apt-get update' "$tmp/log" || fail "the package lists were not refreshed before installing"
grep -q '^systemctl daemon-reload' "$tmp/log" || fail "systemd was not told about the new unit"
# It installs; it starts nothing, applies no kernel setting, creates no network.
if grep -E '^systemctl .*(start|enable|restart|reload-or|--now)' "$tmp/log"; then fail "a unit was started or enabled"; fi
if grep -E '^sysctl ' "$tmp/log"; then fail "a kernel setting was applied"; fi
if grep -E '^docker ' "$tmp/log" | grep -vE '^docker (--version|compose version)$'; then fail "docker was asked to do something"; fi
if grep -E '^apt-get ' "$tmp/log" | grep -vE '^apt-get (update$|install )'; then fail "apt-get did more than update and install"; fi
if grep -E '^apt-get .*(upgrade|autoremove|purge|remove)' "$tmp/log"; then fail "packages were upgraded or removed"; fi
grep -q '^changed ' "$tmp/out" || fail "the real run reports no change"
left="$(find "$host" -name '.install-prerequisites.*' | wc -l)"
[ "$left" -eq 0 ] || fail "a staging directory was left behind"

# ── the second run changes nothing and says so ───────────────────────────
before="$(tree "$host")"
: >"$tmp/log"
run env SYSCTL_SOURCE="$sysctl_src" bash "$script" --root "$host" "${all_parts[@]}" >"$tmp/out" 2>"$tmp/err" \
    || fail "second run failed: $(cat "$tmp/err")"
[ "$(tree "$host")" = "$before" ] || fail "the second run changed the tree"
[ -z "$(changing_calls)" ] || fail "the second run ran: $(changing_calls)"
if grep -q '^changed \|^would ' "$tmp/out"; then fail "the second run reports a change"; fi
grep -q 'nothing to do' "$tmp/out" || fail "the second run does not say there was nothing to do"

# ── a download that does not match its sha256 is refused ─────────────────
new_host "$host"
rc=0
run env GO_SHA256="$(printf '0%.0s' $(seq 64))" bash "$script" --root "$host" --without docker >"$tmp/out" 2>"$tmp/err" || rc=$?
[ "$rc" -eq 1 ] || fail "bad Go sha256: exit $rc, want 1"
grep -q 'sha256' "$tmp/err" || fail "bad Go sha256: the refusal does not say sha256"
[ ! -e "$host/usr/local/go" ] || fail "bad Go sha256: something was installed"
left="$(find "$host" -name '.install-prerequisites.*' | wc -l)"
[ "$left" -eq 0 ] || fail "bad Go sha256: a staging directory was left behind"

new_host "$host"
rc=0
run env NODE_EXPORTER_SHA256="$(printf 'f%.0s' $(seq 64))" bash "$script" --root "$host" --with node-exporter >"$tmp/out" 2>"$tmp/err" || rc=$?
[ "$rc" -eq 1 ] || fail "bad node_exporter sha256: exit $rc, want 1"
[ ! -e "$host/usr/local/bin/node_exporter" ] || fail "bad node_exporter sha256: the binary was installed"

# ── no sha256 for node_exporter: refused before anything changes ─────────
for mode in "" "--dry-run"; do
    new_host "$host"
    before="$(tree "$host")"
    rc=0
    # shellcheck disable=SC2086  # $mode is empty or one word
    run env NODE_EXPORTER_SHA256= bash "$script" --root "$host" $mode --with node-exporter >"$tmp/out" 2>"$tmp/err" || rc=$?
    [ "$rc" -eq 1 ] || fail "no node_exporter sha256 ($mode): exit $rc, want 1"
    grep -q 'NODE_EXPORTER_SHA256' "$tmp/err" || fail "no node_exporter sha256 ($mode): the refusal does not name the variable"
    [ "$(tree "$host")" = "$before" ] || fail "no node_exporter sha256 ($mode): the tree changed"
    [ -z "$(changing_calls)" ] || fail "no node_exporter sha256 ($mode): ran $(changing_calls)"
done
# A malformed sha256 is refused the same way.
new_host "$host"
rc=0
run env GO_SHA256=abc123 bash "$script" --root "$host" --dry-run >"$tmp/out" 2>"$tmp/err" || rc=$?
[ "$rc" -eq 1 ] || fail "malformed Go sha256: exit $rc, want 1"

# ── what is already on the host and differs is kept, not replaced ────────
new_host "$host"
mkdir -p "$host/etc/docker" "$host/usr/local/go" "$host/usr/local/bin" "$host/etc/profile.d"
echo '{"log-driver": "journald"}' >"$host/etc/docker/daemon.json"
echo 'go1.21.0' >"$host/usr/local/go/VERSION"
echo 'export PATH=/somewhere/else:$PATH' >"$host/etc/profile.d/golang.sh"
printf '#!/bin/sh\necho "node_exporter, version 1.0.0 (branch: HEAD)"\n' >"$host/usr/local/bin/node_exporter"
chmod +x "$host/usr/local/bin/node_exporter"
before="$(tree "$host")"
run bash "$script" --root "$host" --without packages --without docker --without rust >"$tmp/out" 2>"$tmp/err" \
    || fail "run over an existing Go failed: $(cat "$tmp/err")"
[ "$(tree "$host")" = "$before" ] || fail "an existing Go or profile file was replaced"
grep -q '^kept .*go1.21.0' "$tmp/out" || fail "the existing Go is not reported as kept"
grep -q '^kept .*golang.sh' "$tmp/out" || fail "the existing profile file is not reported as kept"
[ -z "$(changing_calls)" ] || fail "run over an existing Go ran: $(changing_calls)"

cp "$tmp/state/docker.stub" "$stubs/docker"   # a Docker that was there before
run bash "$script" --root "$host" --without packages --without go --without rust --with node-exporter >"$tmp/out" 2>"$tmp/err" \
    || fail "run over an existing Docker failed: $(cat "$tmp/err")"
grep -q '"journald"' "$host/etc/docker/daemon.json" || fail "an existing daemon.json was replaced"
grep -q '^kept .*daemon.json' "$tmp/out" || fail "the existing daemon.json is not reported as kept"
grep -q 'version 1.0.0' "$host/usr/local/bin/node_exporter" || fail "an existing node_exporter was replaced"
grep -q '^kept .*node_exporter' "$tmp/out" || fail "the existing node_exporter is not reported as kept"
if grep -q 'docker.io' "$tmp/state/pkgs" 2>/dev/null; then fail "Docker packages were installed over an existing Docker"; fi
if grep -q '^curl ' "$tmp/log"; then fail "a download ran although the binary was already there"; fi

# A Docker without the compose plugin is reported, and no package is put over it.
new_host "$host"
cp "$tmp/state/docker.stub" "$stubs/docker"
run env STUB_NO_COMPOSE=1 bash "$script" --root "$host" --without packages --without go --without rust >"$tmp/out" 2>"$tmp/err" \
    || fail "run over a Docker without compose failed: $(cat "$tmp/err")"
grep -q '^kept .*compose' "$tmp/out" || fail "a Docker without the compose plugin is not reported"
if grep -q '^apt-get install' "$tmp/log"; then fail "packages were installed over a Docker without compose"; fi
rm -f "$stubs/docker"

# ── hosts it does not know are refused, dry run included ─────────────────
for os in "debian 13" "ubuntu 22.04" "fedora 43"; do
    # shellcheck disable=SC2086  # two words on purpose
    new_host "$host" $os
    rc=0
    run bash "$script" --root "$host" --dry-run >"$tmp/out" 2>"$tmp/err" || rc=$?
    [ "$rc" -eq 1 ] || fail "$os: exit $rc, want 1"
    [ -z "$(changing_calls)" ] || fail "$os: ran $(changing_calls)"
done
new_host "$host"
rc=0
run env STUB_ARCH=aarch64 bash "$script" --root "$host" --dry-run >"$tmp/out" 2>"$tmp/err" || rc=$?
[ "$rc" -eq 1 ] || fail "aarch64: exit $rc, want 1 (the pins are for x86_64)"

# An explicit sysctl source that is not there is refused; so is none for this host.
new_host "$host"
rc=0
run bash "$script" --root "$host" --dry-run --with sysctl >"$tmp/out" 2>"$tmp/err" || rc=$?
[ "$rc" -eq 1 ] || fail "no sysctl source for the host: exit $rc, want 1"
grep -q '99-unheaded-testhost.conf' "$tmp/err" || fail "the refusal does not name the file it looked for"

# ── the text of the script ───────────────────────────────────────────────
if grep -n ':latest' "$script"; then fail "a floating tag"; fi
if grep -n -i -w -E 'loki|promtail|alloy|jenkins|suricata|ollama|rocm' "$script"; then fail "a retired component is named"; fi
if grep -n -E '\|[[:space:]]*(sudo[[:space:]]+)?(ba)?sh\b' "$script"; then fail "something is piped into a shell"; fi
if grep -n -E 'docker (pull|run|network|compose up)' "$script"; then fail "it pulls, runs or creates with docker"; fi
if grep -n -E '(dist-)?upgrade|pip3? install|snap install|usermod' "$script"; then fail "it does more than install the named packages"; fi
grep -q '^set -euo pipefail$' "$script" || fail "no set -euo pipefail"
head -3 "$script" | grep -q 'SPDX-License-Identifier: GPL-3.0-or-later' || fail "no SPDX header"

# The Go it installs is the Go go.mod asks for.
pin="$(env -u GO_VERSION bash "$script" --root "$host" --dry-run --without packages --without docker --without rust | sed -n 's/.*\(go[0-9][0-9.]*\)\.linux-amd64.*/\1/p' | head -1)"
[ "$pin" = "go$go_version" ] || fail "the script pins ${pin:-nothing}, go.mod's toolchain is go$go_version"

echo "PASS: install-prerequisites (dry run, refusals, a fresh host, a second run, kept files)"
