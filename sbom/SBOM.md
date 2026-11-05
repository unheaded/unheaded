# Unheaded Software Bill of Materials (SBOM)

**Generated:** 2026-11-05
**Format:** summary of SPDX-2.3 compatible files
**Main License:** GPL-3.0-or-later

Written by `scripts/generate-sbom.sh`; do not edit by hand. CDN-loaded
JavaScript and runtime components outside these lists are in `THIRD_PARTY.md`.

## Summary

| Ecosystem | Total | Direct | Detail file |
|-----------|-------|--------|-------------|
| Go | 148 | 17 | `sbom/go-dependencies.json` |
| Rust | 352 external, 43 workspace | | `sbom/rust-dependencies.json` |
| Python | 197 | | `sbom/python-pip-freeze.txt`, a `pip freeze` of the Zhen venv, not a manifest |

## Go licenses

| License | Count |
|---------|-------|
| Unknown | 71 |
| BSD-3-Clause | 35 |
| MIT | 20 |
| Apache-2.0 | 16 |
| BSD-2-Clause | 3 |
| ISC | 1 |
| MIT AND Apache-2.0 | 1 |
| MPL-2.0 | 1 |

## Direct Go modules

| Module | Version | License |
|--------|---------|---------|
| github.com/BurntSushi/toml | v1.6.0 | MIT |
| github.com/anthropics/anthropic-sdk-go | v1.82.0 | Unknown |
| github.com/cilium/ebpf | v0.22.0 | MIT |
| github.com/cloudflare/circl | v1.6.5 | BSD-3-Clause |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/gorilla/websocket | v1.5.3 | BSD-2-Clause |
| github.com/lib/pq | v1.12.3 | MIT |
| github.com/rs/zerolog | v1.35.1 | MIT |
| github.com/yuin/goldmark | v1.8.6 | MIT |
| golang.org/x/crypto | v0.58.1-0.20261009181718-2f1f834330b2 | BSD-3-Clause |
| golang.org/x/sys | v0.49.0 | BSD-3-Clause |
| golang.org/x/text | v0.43.0 | BSD-3-Clause |
| golang.org/x/time | v0.16.0 | BSD-3-Clause |
| google.golang.org/grpc | v1.84.0 | Apache-2.0 |
| google.golang.org/protobuf | v1.36.12 | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | MIT AND Apache-2.0 |
| modernc.org/sqlite | v1.61.0 | BSD-3-Clause |

## Rust licenses

"(assumed)" marks a license this script could not read from a manifest.

| License | Count |
|---------|-------|
| MIT OR Apache-2.0 (assumed) | 262 |
| MIT OR Apache-2.0 | 64 |
| MIT | 21 |
| Apache-2.0 | 4 |
| Unlicense OR MIT | 1 |

## GPL boundary

PASS: no GPL-licensed Go module or Rust crate.

The compiled fixture under `doom/` is GPL-2.0. It runs inside the kernel's eBPF
VM and talks through BPF maps, so nothing links it. HAProxy (GPL-2.0) is a
separate process reached over sockets. `hashicorp/golang-lru/v2` is MPL-2.0,
file-level copyleft, compatible with GPL-3.0.
