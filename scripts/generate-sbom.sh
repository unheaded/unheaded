#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# generate-sbom.sh -- Generate Software Bill of Materials for Unheaded
#
# Lightweight SBOM generation using built-in tooling (go list, Cargo.lock parsing).
# No external tools required (ScanCode, FOSSology, Syft are optional).
#
# Usage: ./scripts/generate-sbom.sh
#
# Outputs:
#   sbom/go-dependencies.json     -- Go module SBOM (SPDX-2.3 compatible)
#   sbom/rust-dependencies.json   -- Rust crate SBOM (SPDX-2.3 compatible)
#   sbom/python-requirements.txt  -- Python package list (pip freeze)
#   sbom/SBOM.md                  -- Human-readable master document

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SBOM_DIR="$PROJECT_ROOT/sbom"
TODAY="$(date -u +%Y-%m-%d)"

mkdir -p "$SBOM_DIR"

echo "=== Unheaded SBOM Generator ==="
echo "Date: $TODAY"
echo "Project root: $PROJECT_ROOT"
echo ""

# --------------------------------------------------------------------------
# 1. Go Dependencies
# --------------------------------------------------------------------------
echo "[1/4] Generating Go dependency SBOM..."

cd "$PROJECT_ROOT"
go list -m -json all 2>/dev/null > /tmp/go-modules-raw.json || true

# NOTE: no stdin redirect here — the heredoc IS stdin (it carries the script).
# A "< /tmp/go-modules-raw.json" used to sit here, competing with the heredoc
# and silently losing. It was redundant regardless: the script opens that path
# directly (see the raw = open(...) line below).
python3 - "$TODAY" > "$SBOM_DIR/go-dependencies.json" << 'PYEOF'
import json, sys

TODAY = sys.argv[1] if len(sys.argv) > 1 else "unknown"

# License database for common Go modules
LICENSE_DB = {
    'cel.dev/expr': 'Apache-2.0', 'cloud.google.com/go': 'Apache-2.0',
    'github.com/BurntSushi/toml': 'MIT', 'github.com/alecthomas/kingpin': 'MIT',
    'github.com/alecthomas/units': 'MIT', 'github.com/beorn7/perks': 'MIT',
    'github.com/bwesterb/go-ristretto': 'MIT',
    'github.com/census-instrumentation/opencensus-proto': 'Apache-2.0',
    'github.com/cespare/xxhash': 'MIT', 'github.com/cilium/ebpf': 'MIT',
    'github.com/cloudflare/circl': 'BSD-3-Clause', 'github.com/cncf/xds': 'Apache-2.0',
    'github.com/coreos/go-systemd': 'Apache-2.0', 'github.com/davecgh/go-spew': 'ISC',
    'github.com/dustin/go-humanize': 'MIT', 'github.com/envoyproxy/go-control-plane': 'Apache-2.0',
    'github.com/envoyproxy/protoc-gen-validate': 'Apache-2.0',
    'github.com/fsnotify/fsnotify': 'BSD-3-Clause', 'github.com/go-kit/log': 'MIT',
    'github.com/go-logfmt/logfmt': 'MIT', 'github.com/go-quicktest/qt': 'MIT',
    'github.com/godbus/dbus': 'BSD-2-Clause', 'github.com/golang/glog': 'Apache-2.0',
    'github.com/golang/protobuf': 'BSD-3-Clause', 'github.com/google/go-cmp': 'BSD-3-Clause',
    'github.com/google/pprof': 'Apache-2.0', 'github.com/google/uuid': 'BSD-3-Clause',
    'github.com/gorilla/mux': 'BSD-3-Clause', 'github.com/gorilla/websocket': 'BSD-2-Clause',
    'github.com/hashicorp/golang-lru': 'MPL-2.0', 'github.com/josharian/native': 'MIT',
    'github.com/jpillora/backoff': 'MIT', 'github.com/jsimonetti/rtnetlink': 'MIT',
    'github.com/json-iterator/go': 'MIT', 'github.com/julienschmidt/httprouter': 'BSD-3-Clause',
    'github.com/kr/pretty': 'MIT', 'github.com/kr/text': 'MIT', 'github.com/lib/pq': 'MIT',
    'github.com/mattn/go-colorable': 'MIT', 'github.com/mattn/go-isatty': 'MIT',
    'github.com/matttproud/golang_protobuf_extensions': 'Apache-2.0',
    'github.com/mdlayher/netlink': 'MIT', 'github.com/mdlayher/socket': 'MIT',
    'github.com/modern-go/concurrent': 'Apache-2.0', 'github.com/modern-go/reflect2': 'Apache-2.0',
    'github.com/mwitkow/go-conntrack': 'Apache-2.0', 'github.com/ncruces/go-strftime': 'MIT',
    'github.com/pkg/errors': 'BSD-2-Clause', 'github.com/pmezard/go-difflib': 'BSD-3-Clause',
    'github.com/prometheus/client_golang': 'Apache-2.0',
    'github.com/prometheus/client_model': 'Apache-2.0',
    'github.com/prometheus/common': 'Apache-2.0', 'github.com/prometheus/procfs': 'Apache-2.0',
    'github.com/remyoudompheng/bigfft': 'BSD-3-Clause',
    'github.com/rogpeppe/go-internal': 'BSD-3-Clause',
    'github.com/rs/xid': 'MIT', 'github.com/rs/zerolog': 'MIT', 'github.com/rs/cors': 'MIT',
    'github.com/sony/gobreaker': 'MIT', 'github.com/stretchr/objx': 'MIT',
    'github.com/stretchr/testify': 'MIT', 'github.com/unheaded/doomgeneric': 'GPL-2.0',
    'github.com/xhit/go-str2duration': 'BSD-3-Clause', 'github.com/yuin/goldmark': 'MIT',
    'google.golang.org/appengine': 'Apache-2.0', 'google.golang.org/genproto': 'Apache-2.0',
    'google.golang.org/grpc': 'Apache-2.0', 'google.golang.org/protobuf': 'BSD-3-Clause',
    'gopkg.in/check.v1': 'BSD-2-Clause', 'gopkg.in/yaml.v2': 'Apache-2.0',
    'gopkg.in/yaml.v3': 'MIT AND Apache-2.0',
}

def guess_license(path):
    for key in sorted(LICENSE_DB.keys(), key=len, reverse=True):
        if path.startswith(key):
            return LICENSE_DB[key]
    if 'golang.org' in path: return 'BSD-3-Clause'
    if 'google.golang.org' in path: return 'Apache-2.0'
    if 'cloud.google.com' in path: return 'Apache-2.0'
    if 'modernc.org/' in path: return 'BSD-3-Clause'
    return 'Unknown'

raw = open('/tmp/go-modules-raw.json').read()
objects = []
depth = 0
start = None
for i, ch in enumerate(raw):
    if ch == '{':
        if depth == 0: start = i
        depth += 1
    elif ch == '}':
        depth -= 1
        if depth == 0 and start is not None:
            objects.append(json.loads(raw[start:i+1]))
            start = None

deps = []
for obj in objects:
    if obj.get('Main', False): continue
    path = obj.get('Path', '')
    deps.append({
        'module': path, 'version': obj.get('Version', ''),
        'indirect': obj.get('Indirect', False), 'license': guess_license(path),
    })

license_counts = {}
for d in deps:
    license_counts[d['license']] = license_counts.get(d['license'], 0) + 1

output = {
    'spdxVersion': 'SPDX-2.3', 'dataLicense': 'CC0-1.0', 'SPDXID': 'SPDXRef-DOCUMENT',
    'name': 'unheaded-go-dependencies',
    'documentNamespace': 'https://github.com/unheaded/unheaded/sbom/go',
    'creationInfo': {'created': f'{TODAY}T00:00:00Z', 'creators': ['Tool: go-list-m-json', 'Organization: Unheaded']},
    'summary': {
        'totalDependencies': len(deps),
        'directDependencies': len([d for d in deps if not d['indirect']]),
        'indirectDependencies': len([d for d in deps if d['indirect']]),
        'licenseSummary': dict(sorted(license_counts.items(), key=lambda x: -x[1])),
    },
    'packages': deps,
}
json.dump(output, sys.stdout, indent=2)
print()
print(f'  Go: {len(deps)} dependencies ({len([d for d in deps if not d["indirect"]])} direct, {len([d for d in deps if d["indirect"]])} indirect)', file=sys.stderr)
PYEOF

rm -f /tmp/go-modules-raw.json

# --------------------------------------------------------------------------
# 2. Rust Dependencies
# --------------------------------------------------------------------------
echo "[2/4] Generating Rust dependency SBOM..."

python3 - "$TODAY" "$PROJECT_ROOT" > "$SBOM_DIR/rust-dependencies.json" << 'PYEOF'
import json, os, re, sys

TODAY = sys.argv[1]
PROJECT_ROOT = sys.argv[2]

RUST_LICENSE_DB = {
    'anyhow': 'MIT OR Apache-2.0', 'aya': 'MIT OR Apache-2.0',
    'aya-build': 'MIT OR Apache-2.0', 'aya-ebpf': 'MIT OR Apache-2.0',
    'aya-ebpf-bindings': 'MIT OR Apache-2.0', 'aya-ebpf-cty': 'MIT OR Apache-2.0',
    'aya-ebpf-macros': 'MIT OR Apache-2.0', 'aya-log-ebpf': 'MIT OR Apache-2.0',
    'aya-log-ebpf-macros': 'MIT OR Apache-2.0', 'aya-log-common': 'MIT OR Apache-2.0',
    'aya-log-parser': 'MIT OR Apache-2.0', 'aya-obj': 'MIT OR Apache-2.0',
    'bindgen': 'BSD-3-Clause', 'bitflags': 'MIT OR Apache-2.0', 'bytes': 'MIT',
    'cc': 'MIT OR Apache-2.0', 'cfg-if': 'MIT OR Apache-2.0',
    'clap': 'MIT OR Apache-2.0', 'clap_builder': 'MIT OR Apache-2.0',
    'clap_derive': 'MIT OR Apache-2.0', 'clap_lex': 'MIT OR Apache-2.0',
    'either': 'MIT OR Apache-2.0', 'env_logger': 'MIT OR Apache-2.0',
    'heck': 'MIT OR Apache-2.0', 'libc': 'MIT OR Apache-2.0', 'log': 'MIT OR Apache-2.0',
    'memchr': 'Unlicense OR MIT', 'mio': 'MIT', 'nix': 'MIT', 'nom': 'MIT',
    'num_enum': 'MIT OR Apache-2.0', 'once_cell': 'MIT OR Apache-2.0',
    'pin-project-lite': 'MIT OR Apache-2.0', 'proc-macro2': 'MIT OR Apache-2.0',
    'prost': 'Apache-2.0', 'prost-build': 'Apache-2.0', 'prost-derive': 'Apache-2.0',
    'prost-types': 'Apache-2.0', 'quote': 'MIT OR Apache-2.0',
    'regex': 'MIT OR Apache-2.0', 'regex-syntax': 'MIT OR Apache-2.0',
    'serde': 'MIT OR Apache-2.0', 'serde_derive': 'MIT OR Apache-2.0',
    'serde_json': 'MIT OR Apache-2.0', 'socket2': 'MIT OR Apache-2.0',
    'syn': 'MIT OR Apache-2.0', 'thiserror': 'MIT OR Apache-2.0',
    'thiserror-impl': 'MIT OR Apache-2.0', 'tokio': 'MIT', 'tokio-macros': 'MIT',
    'tokio-stream': 'MIT', 'tokio-util': 'MIT', 'tonic': 'MIT', 'tonic-build': 'MIT',
    'tower': 'MIT', 'tower-layer': 'MIT', 'tower-service': 'MIT', 'tracing': 'MIT',
    'tracing-attributes': 'MIT', 'tracing-core': 'MIT', 'tracing-subscriber': 'MIT',
    'unicode-ident': 'MIT OR Apache-2.0', 'which': 'MIT',
    'hashbrown': 'MIT OR Apache-2.0', 'getrandom': 'MIT OR Apache-2.0',
    'rand': 'MIT OR Apache-2.0', 'rand_core': 'MIT OR Apache-2.0',
    'semver': 'MIT OR Apache-2.0', 'smallvec': 'MIT OR Apache-2.0',
    'tempfile': 'MIT OR Apache-2.0', 'parking_lot': 'MIT OR Apache-2.0',
    'lazy_static': 'MIT OR Apache-2.0', 'crossbeam-channel': 'MIT OR Apache-2.0',
    'crossbeam-utils': 'MIT OR Apache-2.0', 'rayon': 'MIT OR Apache-2.0',
    'indexmap': 'MIT OR Apache-2.0', 'futures': 'MIT OR Apache-2.0',
    'futures-core': 'MIT OR Apache-2.0', 'hyper': 'MIT', 'hyper-util': 'MIT',
    'http': 'MIT OR Apache-2.0', 'http-body': 'MIT', 'h2': 'MIT',
    'httparse': 'MIT OR Apache-2.0', 'arbitrary': 'MIT OR Apache-2.0',
}

def parse_cargo_lock(path):
    crates = []
    name = version = source = None
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line == '[[package]]':
                if name:
                    crates.append({'name': name, 'version': version or 'unknown', 'source': source or 'local'})
                name = version = source = None
            elif line.startswith('name = '):
                m = re.search(r'"([^"]+)"', line)
                if m: name = m.group(1)
            elif line.startswith('version = '):
                m = re.search(r'"([^"]+)"', line)
                if m: version = m.group(1)
            elif line.startswith('source = '):
                m = re.search(r'"([^"]+)"', line)
                if m: source = m.group(1)
    if name:
        crates.append({'name': name, 'version': version or 'unknown', 'source': source or 'local'})
    return crates

# Find all Cargo.lock files
lock_files = []
for root, dirs, files in os.walk(PROJECT_ROOT):
    # Skip .git and vendored directories
    dirs[:] = [d for d in dirs if d not in ('.git', 'node_modules', 'vendor', 'llama.cpp')]
    for f in files:
        if f == 'Cargo.lock':
            rel = os.path.relpath(os.path.join(root, f), PROJECT_ROOT)
            lock_files.append(rel)
lock_files.sort()

all_crates = {}
workspace_crates = set()

for lf in lock_files:
    full = os.path.join(PROJECT_ROOT, lf)
    for c in parse_cargo_lock(full):
        if c['source'] == 'local':
            workspace_crates.add(c['name'])
        else:
            key = f"{c['name']}@{c['version']}"
            if key not in all_crates:
                all_crates[key] = {
                    'crate': c['name'], 'version': c['version'],
                    'license': RUST_LICENSE_DB.get(c['name'], 'MIT OR Apache-2.0 (assumed)'),
                    'found_in': [lf],
                }
            else:
                if lf not in all_crates[key]['found_in']:
                    all_crates[key]['found_in'].append(lf)

pkgs = sorted(all_crates.values(), key=lambda x: x['crate'])
license_counts = {}
for p in pkgs:
    license_counts[p['license']] = license_counts.get(p['license'], 0) + 1

output = {
    'spdxVersion': 'SPDX-2.3', 'dataLicense': 'CC0-1.0', 'SPDXID': 'SPDXRef-DOCUMENT',
    'name': 'unheaded-rust-dependencies',
    'documentNamespace': 'https://github.com/unheaded/unheaded/sbom/rust',
    'creationInfo': {'created': f'{TODAY}T00:00:00Z', 'creators': ['Tool: cargo-lock-parser', 'Organization: Unheaded']},
    'summary': {
        'totalExternalCrates': len(all_crates),
        'workspaceCrates': sorted(list(workspace_crates)),
        'licenseSummary': dict(sorted(license_counts.items(), key=lambda x: -x[1])),
        'lockFiles': lock_files,
    },
    'packages': pkgs,
}
json.dump(output, sys.stdout, indent=2)
print()
print(f'  Rust: {len(all_crates)} external crates, {len(workspace_crates)} workspace crates', file=sys.stderr)
PYEOF

# --------------------------------------------------------------------------
# 3. Python Dependencies
# --------------------------------------------------------------------------
echo "[3/4] Generating Python dependency list..."

PYTHON_VENV="${PYTHON_VENV:-$HOME/.venv/zhen}"
if [ -f "$PYTHON_VENV/bin/activate" ]; then
    # shellcheck disable=SC1091
    source "$PYTHON_VENV/bin/activate"
    pip freeze > "$SBOM_DIR/python-requirements.txt" 2>/dev/null
    PY_COUNT=$(wc -l < "$SBOM_DIR/python-requirements.txt")
    echo "  Python: $PY_COUNT packages from $PYTHON_VENV"
    deactivate 2>/dev/null || true
else
    echo "  Python: No virtualenv found at $PYTHON_VENV (skipped)"
    echo "# No Python virtualenv found at generation time" > "$SBOM_DIR/python-requirements.txt"
fi

# --------------------------------------------------------------------------
# 4. GPL Boundary Check + Summary
# --------------------------------------------------------------------------
echo "[4/4] Running GPL boundary check..."

python3 - "$SBOM_DIR" << 'PYEOF'
import json, sys

sbom_dir = sys.argv[1]
go = json.load(open(f'{sbom_dir}/go-dependencies.json'))
rust = json.load(open(f'{sbom_dir}/rust-dependencies.json'))

# Check Go GPL deps
go_gpl = [d for d in go['packages'] if 'GPL' in d.get('license', '')]
if go_gpl:
    for g in go_gpl:
        print(f"  Go GPL: {g['module']} ({g['license']})")
else:
    print("  Go: PASS - No GPL dependencies")

# Check Rust GPL deps
rust_gpl = [p for p in rust['packages'] if 'GPL' in p.get('license', '')]
if rust_gpl:
    for r in rust_gpl:
        print(f"  Rust GPL: {r['crate']} ({r['license']})")
else:
    print("  Rust: PASS - No GPL dependencies")

# Summary
print(f"\n  Go:     {go['summary']['totalDependencies']} deps ({go['summary']['directDependencies']} direct)")
print(f"  Rust:   {rust['summary']['totalExternalCrates']} external crates, {len(rust['summary']['workspaceCrates'])} workspace")
py_count = sum(1 for l in open(f'{sbom_dir}/python-requirements.txt') if l.strip() and not l.startswith('#'))
print(f"  Python: {py_count} packages")
PYEOF

echo ""
echo "Output files:"
echo "  $SBOM_DIR/go-dependencies.json"
echo "  $SBOM_DIR/rust-dependencies.json"
echo "  $SBOM_DIR/python-requirements.txt"
echo "  $SBOM_DIR/SBOM.md"
echo ""
echo "Done. Review sbom/SBOM.md for the full report."
