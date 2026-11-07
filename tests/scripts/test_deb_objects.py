# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""The debs that load eBPF objects ship them root-owned, where the loaders look.

build-all-debs.sh runs for real in a copy of the tree. Only the compilers are
stand-ins: build-binaries.sh and cargo write placeholder files, so what is under
test is what lands in the package, with which owner and mode."""
import os
import re
import shutil
import stat
import subprocess
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parents[2]
VERSION = "0.2.0~alpha+test"
OBJECT_DIR = "usr/lib/unheaded/ebpf"
SHIPPED = {
    "unheaded-chaos-controller": ["yaldabaoth-ebpf"],
    "unheaded-heimdall-daemon": ["heimdall-execve", "heimdall-gjallarhorn-xdp", "heimdall-vfs-write"],
}

STAND_IN_BINARIES = """#!/bin/bash
shift; out="$1"; shift
mkdir -p "$out"
for b in "$@"; do printf 'binary\\n' >"$out/$b"; chmod 0755 "$out/$b"; done
"""

# Writes each --bin it is asked for where a real build puts it, and logs the call.
STAND_IN_CARGO = """#!/bin/bash
echo "$(basename "$PWD") $*" >>"$CARGO_LOG"
[ -z "${CARGO_FAILS:-}" ] || exit 1
target=target
bins=()
while [ $# -gt 0 ]; do
  case "$1" in
    --target-dir) target="$2"; shift ;;
    --bin=*) bins+=("${1#--bin=}") ;;
  esac
  shift
done
mkdir -p "$target/bpfel-unknown-none/release"
for b in "${bins[@]}"; do printf 'object\\n' >"$target/bpfel-unknown-none/release/$b"; done
"""


def stand_in(path, body):
    path.write_text(body)
    path.chmod(path.stat().st_mode | stat.S_IXUSR)


@pytest.fixture(scope="module")
def tree(tmp_path_factory):
    root = tmp_path_factory.mktemp("debs") / "tree"
    for d in ("scripts", "deploy/systemd", "deploy/sysusers.d", "runbooks", "man"):
        shutil.copytree(REPO / d, root / d, ignore=shutil.ignore_patterns("__pycache__"))
    for f in ("configs/wotan.yaml", "compliance/retention/classes.yaml", "config/logging/sources.yaml"):
        (root / f).parent.mkdir(parents=True, exist_ok=True)
        shutil.copy(REPO / f, root / f)
    (root / "ebpf").mkdir()
    (root / "crates" / "heimdall-bpf").mkdir(parents=True)
    stand_in(root / "scripts" / "build-binaries.sh", STAND_IN_BINARIES)
    tools = root.parent / "tools"
    tools.mkdir()
    stand_in(tools / "cargo", STAND_IN_CARGO)
    stand_in(tools / "bpf-linker", "#!/bin/bash\nexit 0\n")
    return root


def build(tree, out, path=None, **env):
    log = tree.parent / f"{out}.cargo"
    tools = tree.parent / "tools"
    run_env = {**os.environ, "PATH": path or f"{tools}:/usr/bin:/bin", "CARGO_LOG": str(log),
               "DEB_OUTPUT_DIR": str(tree.parent / out), **env}
    r = subprocess.run(["bash", "scripts/build-all-debs.sh", VERSION], cwd=tree, env=run_env,
                       capture_output=True, text=True, timeout=300, check=False)
    return r, tree.parent / out, log


@pytest.fixture(scope="module")
def built(tree):
    r, out, log = build(tree, "good")
    assert r.returncode == 0, r.stderr
    return out, log


def contents(out, pkg):
    deb = out / f"{pkg}_{VERSION}_amd64.deb"
    listing = subprocess.run(["dpkg-deb", "-c", str(deb)], capture_output=True, text=True, timeout=60, check=True).stdout
    rows = {}
    for line in listing.splitlines():
        mode, owner, _size, _date, _time, name = line.split(None, 5)
        rows[name.removeprefix("./").rstrip("/")] = (mode, owner)
    return rows


@pytest.mark.parametrize("pkg,objects", SHIPPED.items())
def test_each_object_is_root_owned_and_only_root_can_write_it(built, pkg, objects):
    rows = contents(built[0], pkg)
    for name in objects:
        assert rows[f"{OBJECT_DIR}/{name}"] == ("-rw-r--r--", "root/root"), name


@pytest.mark.parametrize("pkg", SHIPPED)
def test_every_directory_above_an_object_is_root_owned_and_closed(built, pkg):
    rows = contents(built[0], pkg)
    path = OBJECT_DIR
    while path != ".":
        assert rows[path] == ("drwxr-xr-x", "root/root"), path
        path = os.path.dirname(path) or "."


def test_no_other_package_ships_an_object(built):
    for deb in built[0].glob("*.deb"):
        pkg = deb.name.split("_")[0]
        if pkg not in SHIPPED and not deb.name.endswith("_all.deb"):
            assert not [n for n in contents(built[0], pkg) if n.startswith(OBJECT_DIR)], pkg


def test_objects_are_built_locked_and_in_release_from_their_own_crate(built):
    calls = built[1].read_text().splitlines()
    assert any(c.startswith("ebpf build ") and "--bin=yaldabaoth-ebpf" in c for c in calls), calls
    assert any(c.startswith("heimdall-bpf build ") and "--bin=heimdall-execve" in c for c in calls), calls
    for call in calls:
        assert "--locked" in call and "--release" in call, call


def test_a_failed_object_build_fails_the_packages(tree):
    r, out, _log = build(tree, "failing", CARGO_FAILS="1")
    assert r.returncode != 0
    assert not list(out.glob("unheaded-chaos-controller_*"))


def test_a_build_without_bpf_linker_says_so(tree):
    bare = tree.parent / "bare"
    bare.mkdir()
    shutil.copy(tree.parent / "tools" / "cargo", bare / "cargo")
    r, _out, _log = build(tree, "nolinker", path=f"{bare}:/usr/bin:/bin")
    assert r.returncode != 0
    assert "bpf-linker" in r.stderr


def test_the_chaos_object_is_where_the_injector_looks():
    packet = (REPO / "services/yaldabaoth/packet/packet.go").read_text()
    assert f'DefaultObject = "/{OBJECT_DIR}/yaldabaoth-ebpf"' in packet


def test_the_heimdall_objects_are_the_ones_the_daemon_opens():
    hooks = (REPO / "cmd/heimdall-daemon/bpf_linux.go").read_text()
    names = re.findall(r'^\s*obj\w+\s*=\s*"([^"]+)"', hooks, re.MULTILINE)
    assert sorted(names) == SHIPPED["unheaded-heimdall-daemon"]
