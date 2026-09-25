#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-suricata-rules.sh — the Monad Suricata rules load, and fire on exactly
# the packets they should, in every Suricata this repository deploys.
#
# Why this exists. Until 2026-09-25 every rule used `ipv6-exthdr`, which is
# not a Suricata keyword, so Suricata loaded ZERO rules everywhere: the K8s
# DaemonSet configured no rule files at all, the Nix module did not even parse,
# and nothing ever ran a rule. An IDS that alerts on nothing looks exactly like
# an IDS on a quiet network. This gate makes that visible.
#
# Checks:
#   1. The K8s ConfigMap's copies of the rules, classification.config and
#      reference.config are byte-identical to routing/suricata/rules/ (the
#      canonical files), and the Nix module installs the canonical files.
#   2. For each Suricata image: the K8s ConfigMap's suricata.yaml passes
#      `suricata -T` with those files, and every packet case from
#      scripts/suricata/mkpcap.py fires EXACTLY its expected rule set (so a
#      rule that stops firing fails, and so does one that starts firing on
#      traffic it should ignore).
#
# Images: the one the DaemonSet runs (read from its manifest, so this tests
# what K8s deploys), and Suricata 8 (nixpkgs ships 8.x; the two majors differ).
#
# Needs docker. Without it this FAILS (exit 2); it never passes by skipping.
#
# Usage: scripts/check-suricata-rules.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RULES_DIR="${REPO_ROOT}/routing/suricata/rules"
CM="${REPO_ROOT}/kubernetes/manifests/base/suricata/configmap.yaml"
DS="${REPO_ROOT}/kubernetes/manifests/base/suricata/daemonset.yaml"
NIX="${REPO_ROOT}/nixos/modules/suricata.nix"
FILES=(unheaded-monad.rules classification.config reference.config)
# Pinned, like the DaemonSet's image, so this gate tests the same bytes on
# every run. 8.0.6 resolved to this digest on 2026-09-25.
NIX_SURICATA_IMAGE="${NIX_SURICATA_IMAGE:-jasonish/suricata:8.0.6@sha256:9872eea68c200cab826b7d62ea2be6eb3df4605ab6e2491a729c7d5fe635e830}"

if ! docker info >/dev/null 2>&1; then
    echo "[FAIL] docker is required to run Suricata; this gate does not pass by skipping." >&2
    exit 2
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
rc=0

# ---- 1. copies ------------------------------------------------------------
if ! python3 - "$CM" "$RULES_DIR" "$WORK/etc" "${FILES[@]}" <<'EOF'; then
import os, sys, yaml
cm, rules_dir, out, *files = sys.argv[1:]
data = yaml.safe_load(open(cm))["data"]
os.makedirs(out, exist_ok=True)
bad = False
for f in files:
    canon = open(os.path.join(rules_dir, f)).read()
    if data.get(f) != canon:
        print(f"[FAIL] ConfigMap key {f!r} differs from routing/suricata/rules/{f}", file=sys.stderr)
        bad = True
for k, v in data.items():  # materialise /etc/suricata as the pod sees it
    open(os.path.join(out, k), "w").write(v)
sys.exit(1 if bad else 0)
EOF
    rc=1
fi
for f in "${FILES[@]}"; do
    if ! grep -qF "source = ../../routing/suricata/rules/${f};" "$NIX"; then
        echo "[FAIL] ${NIX#"$REPO_ROOT"/} does not install routing/suricata/rules/${f}" >&2
        rc=1
    fi
done

# ---- 2. behaviour, per image ---------------------------------------------
DS_IMAGE="$(awk '/^ *image:/ {print $2; exit}' "$DS")"
if [[ -z "$DS_IMAGE" ]]; then
    echo "[FAIL] no image found in ${DS#"$REPO_ROOT"/}" >&2
    exit 1
fi
# A tag can be rebuilt under the same name (7.0 was, on 2026-09-24), so the
# DaemonSet must name a digest: then what this gate tests is what K8s runs.
if [[ "$DS_IMAGE" != *@sha256:* ]]; then
    echo "[FAIL] ${DS#"$REPO_ROOT"/} image is not pinned by digest: ${DS_IMAGE}" >&2
    rc=1
fi

mkdir -p "$WORK/pcaps" "$WORK/rules"
cp "$RULES_DIR"/* "$WORK/rules/"
python3 "${REPO_ROOT}/scripts/suricata/mkpcap.py" "$WORK/pcaps" >"$WORK/expected"
cat >"$WORK/replay.yaml" <<'EOF'
%YAML 1.1
---
vars:
  address-groups:
    HOME_NET: "any"
    EXTERNAL_NET: "any"
classification-file: /rules/classification.config
reference-config-file: /rules/reference.config
outputs:
  - eve-log:
      enabled: yes
      filename: eve.json
      types: [alert]
EOF

n_rules="$(grep -cE '^(alert|drop|reject|pass) ' "${RULES_DIR}/unheaded-monad.rules")"

# configtest <image> <label> <docker -v args...>
configtest() {
    local image="$1" label="$2" log
    shift 2
    log="$(mktemp -p "$WORK")"
    mkdir -p "$WORK/varlog-$$" && chmod 777 "$WORK/varlog-$$"
    if ! docker run --rm --user "$(id -u):$(id -g)" "$@" -v "$WORK/varlog-$$:/var/log/suricata" \
        --entrypoint suricata "$image" -T -v -c /etc/suricata/suricata.yaml -l /tmp >"$log" 2>&1; then
        echo "[FAIL] ${image}: ${label} config fails suricata -T:" >&2
        grep -E "^(E|Error)" "$log" | head -5 | sed 's/^/         /' >&2
        rc=1
    elif ! grep -qE "${n_rules} rules successfully loaded, 0 rules failed" "$log"; then
        echo "[FAIL] ${image}: ${label} config does not load the ${n_rules} Monad rules:" >&2
        grep -E "rules successfully loaded|rule files" "$log" | head -2 | sed 's/^/         /' >&2
        rc=1
    fi
}

# The NixOS module's suricata.yaml, generated by a real NixOS evaluation, with
# the files laid out as the module installs them. Needs nix and <nixpkgs>.
if command -v nix-instantiate >/dev/null 2>&1; then
    mkdir -p "$WORK/nixetc/rules"
    if (cd "$REPO_ROOT" && nix-instantiate --eval --strict --json -E '
        let sys = import <nixpkgs/nixos/lib/eval-config.nix> {
          system = "x86_64-linux";
          modules = [ ./nixos/modules/suricata.nix {
            services.unheaded.suricata.enable = true;
            boot.loader.grub.enable = false;
            fileSystems."/".device = "/dev/null";
            system.stateVersion = "24.11";
          } ];
        };
        in sys.config.environment.etc."suricata/suricata.yaml".text') 2>"$WORK/nix.err" \
        | python3 -c 'import json,sys; open(sys.argv[1],"w").write(json.load(sys.stdin))' "$WORK/nixetc/suricata.yaml"; then
        cp "$RULES_DIR/unheaded-monad.rules" "$WORK/nixetc/rules/"
        cp "$RULES_DIR/classification.config" "$RULES_DIR/reference.config" "$WORK/nixetc/"
    else
        echo "[FAIL] the NixOS module does not evaluate:" >&2
        tail -5 "$WORK/nix.err" | sed 's/^/         /' >&2
        rm -rf "$WORK/nixetc"
        rc=1
    fi
fi

for image in "$DS_IMAGE" "$NIX_SURICATA_IMAGE"; do
    echo "== ${image}"
    if ! docker image inspect "$image" >/dev/null 2>&1 && ! docker pull -q "$image" >/dev/null 2>&1; then
        echo "[FAIL] cannot obtain ${image}" >&2
        rc=1
        continue
    fi
    out="$WORK/out-$(echo "$image" | tr '/:' '__')"
    mkdir -p "$out" && chmod 777 "$out"

    # Each deployment's own suricata.yaml, with its files mounted as that
    # deployment mounts them. -T alone passes with zero rules loaded (the K8s
    # pod ran that way for months), so the rule count must match too.
    configtest "$image" "K8s ConfigMap" -v "$WORK/etc:/etc/suricata:ro"
    configtest "$image" "Docker host-b" \
        -v "${REPO_ROOT}/docker/suricata/suricata.yaml:/etc/suricata/suricata.yaml:ro" \
        -v "${RULES_DIR}:/etc/suricata/rules:ro"
    if [[ "$image" == "$NIX_SURICATA_IMAGE" ]]; then
        if [[ -d "$WORK/nixetc" ]]; then
            configtest "$image" "NixOS module" -v "$WORK/nixetc:/etc/suricata:ro"
        else
            echo "[INFO] nix-instantiate not available: the NixOS module's generated config was NOT exercised" >&2
        fi
    fi

    # As the invoking user, so the trap can delete what Suricata writes.
    docker run --rm --user "$(id -u):$(id -g)" -v "$WORK:/w:ro" -v "$WORK/rules:/rules:ro" -v "$out:/o" --entrypoint sh "$image" -c '
        for p in /w/pcaps/*.pcap; do
            n=$(basename "$p" .pcap); mkdir -p "/o/$n"
            suricata -c /w/replay.yaml -S /rules/unheaded-monad.rules -r "$p" -l "/o/$n" -k none >"/o/$n/run.log" 2>&1 \
                || echo "suricata exited non-zero" >>"/o/$n/run.log"
        done' >/dev/null 2>&1

    while read -r name want; do
        got="$(grep -o '"signature_id":[0-9]*' "$out/$name/eve.json" 2>/dev/null | cut -d: -f2 | sort -nu | xargs)"
        if [[ "$got" != "$want" ]]; then
            echo "[FAIL] ${image}: ${name}: fired [${got}], expected [${want}]" >&2
            grep -E "^(E|Error)|non-zero" "$out/$name/run.log" 2>/dev/null | head -3 | sed 's/^/         /' >&2
            rc=1
        fi
    done <"$WORK/expected"
done

if [[ $rc -ne 0 ]]; then
    exit $rc
fi
configs="K8s ConfigMap, Docker host-b"
[[ -d "$WORK/nixetc" ]] && configs="${configs}, NixOS module"
echo "[PASS] ${n_rules} Monad rules load (${configs}) and fire as expected on ${DS_IMAGE} and ${NIX_SURICATA_IMAGE} ($(wc -l <"$WORK/expected") packet cases each)"
