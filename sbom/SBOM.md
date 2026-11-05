# Unheaded Software Bill of Materials (SBOM)

**Generated:** 2026-11-05
**Format:** summary of SPDX-2.3 compatible files
**Main License:** GPL-3.0-or-later

Written by `scripts/generate-sbom.sh`; do not edit by hand. Runtime components
outside these lists are in `THIRD_PARTY.md`.

## Summary

| Ecosystem | Total | Direct | Licenses not read | Detail file |
|-----------|-------|--------|-------------------|-------------|
| Go | 148 | 17 | 32 | `sbom/go-dependencies.json` |
| Rust | 352 external, 43 workspace | | 0 | `sbom/rust-dependencies.json` |
| Python | 197 | | 0 | `sbom/python-dependencies.json`, from `sbom/python-pip-freeze.txt`, a `pip freeze` of the Zhen venv, not a manifest |
| JavaScript | 4 | | 4 | `sbom/javascript-dependencies.json`, CDN-loaded files; nothing is installed |

Rust lock files read: `cmd/crypto-inventory/testdata/rust/Cargo.lock`, `cmd/ebpf-collector/Cargo.lock`, `cmd/ebpf-collector/ebpf-programs/Cargo.lock`, `cmd/ebpf-load-check/Cargo.lock`, `cmd/ebpf-loader/Cargo.lock`, `cmd/upc-bootctl/Cargo.lock`, `crates/heimdall-bpf/Cargo.lock`, `crates/lich-fuzz/Cargo.lock`, `crates/monad-mbc/Cargo.lock`, `crates/monad-mbc/fuzz/Cargo.lock`, `crates/svc-token/Cargo.lock`, `crates/upc-api/Cargo.lock`, `crates/upc-api/fuzz/Cargo.lock`, `crates/upc-bootstub/Cargo.lock`, `crates/upc-cartridge/Cargo.lock`, `crates/xv6-mbc/Cargo.lock`, `crates/zhend/Cargo.lock`, `ebpf/Cargo.lock`, `tomb/lich/LICH-015-mbc-differential/harnesses/diff_harness/Cargo.lock`.

Rust workspace crates: `anomaly-ebpf`, `aws-lc-rs`, `canary-ebpf`, `cartridge-ebpf`, `collector`, `compliance-ebpf`, `ebpf-common`, `ebpf-load-check`, `ebpf-loader`, `ebpf-programs`, `failover-ebpf`, `firewall-ebpf`, `flow-tracker`, `heimdall-bpf`, `hop-ebpf`, `latency-probe`, `lich-fuzz`, `lich015-diff-harness`, `maglev-ebpf`, `monad-common`, `monad-cpu-ebpf`, `monad-mbc`, `monad-mbc-fuzz`, `nfv-ebpf`, `packet-marker`, `pqc-common`, `pqcrypto-mlkem`, `qos-ebpf`, `ring`, `rustls`, `shield-ebpf`, `syscall-tracer`, `unheaded-common`, `unheaded-svc-token`, `upc-api`, `upc-api-fuzz`, `upc-bootctl`, `upc-bootstub`, `upc-cartridge`, `version-ebpf`, `xv6-mbc`, `yaldabaoth-ebpf`, `zhend`.

## Go licenses

Read from each module's license files, in the module cache. " AND " means the
files name each of these; they may be alternatives.

| License | Count |
|---------|-------|
| BSD-3-Clause | 37 |
| MIT | 32 |
| Unknown | 31 |
| Apache-2.0 | 30 |
| Apache-2.0 AND BSD-3-Clause | 6 |
| Apache-2.0 AND MIT | 4 |
| BSD-2-Clause | 3 |
| BSD-3-Clause AND MIT | 2 |
| Apache-2.0 AND BSD-3-Clause AND LicenseRef-public-domain AND MIT AND MPL-2.0 | 1 |
| ISC | 1 |
| MPL-2.0 | 1 |

## Direct Go modules

| Module | Version | License |
|--------|---------|---------|
| github.com/BurntSushi/toml | v1.6.0 | MIT |
| github.com/anthropics/anthropic-sdk-go | v1.82.0 | MIT |
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
| gopkg.in/yaml.v3 | v3.0.1 | Apache-2.0 AND MIT |
| modernc.org/sqlite | v1.61.0 | Apache-2.0 AND BSD-3-Clause AND LicenseRef-public-domain AND MIT AND MPL-2.0 |

## Rust licenses

The license field of each crate's own Cargo.toml, as written.

| License | Count |
|---------|-------|
| MIT OR Apache-2.0 | 193 |
| MIT | 61 |
| Apache-2.0 OR MIT | 23 |
| MIT/Apache-2.0 | 18 |
| Apache-2.0 | 9 |
| Unlicense OR MIT | 7 |
| Apache-2.0 WITH LLVM-exception OR Apache-2.0 OR MIT | 5 |
| BSD-3-Clause | 4 |
| Apache-2.0/MIT | 3 |
| Zlib | 3 |
| BSD-2-Clause OR Apache-2.0 OR MIT | 2 |
| CC0-1.0 OR MIT-0 OR Apache-2.0 | 2 |
| ISC | 2 |
| MIT OR Apache-2.0 OR LGPL-2.1-or-later | 2 |
| Unlicense/MIT | 2 |
| Zlib OR Apache-2.0 OR MIT | 2 |
| (MIT OR Apache-2.0) AND NCSA | 1 |
| (MIT OR Apache-2.0) AND Unicode-3.0 | 1 |
| 0BSD OR MIT OR Apache-2.0 | 1 |
| Apache-2.0 / MIT | 1 |
| Apache-2.0 AND ISC | 1 |
| Apache-2.0 OR ISC OR MIT | 1 |
| CC0-1.0 OR Apache-2.0 OR Apache-2.0 WITH LLVM-exception | 1 |
| CDLA-Permissive-2.0 | 1 |
| ISC AND (Apache-2.0 OR ISC) | 1 |
| ISC AND (Apache-2.0 OR ISC) AND Apache-2.0 AND MIT AND BSD-3-Clause AND (Apache-2.0 OR ISC OR MIT) AND (Apache-2.0 OR ISC OR MIT-0) | 1 |
| MIT AND BSD-3-Clause | 1 |
| MIT OR Apache-2.0 OR BSD-1-Clause | 1 |
| MIT OR Apache-2.0 OR Zlib | 1 |
| MIT OR Zlib OR Apache-2.0 | 1 |

## Python licenses

Read from each package's metadata in the Zhen venv: License-Expression, else
the license classifiers, else the License field, else its license files.

| License | Count |
|---------|-------|
| MIT | 39 |
| MIT License | 33 |
| BSD-3-Clause | 22 |
| Apache-2.0 | 20 |
| BSD License | 18 |
| Apache Software License | 14 |
| Other/Proprietary License | 13 |
| Apache Software License AND BSD License | 2 |
| BSD-2-Clause | 2 |
| Apache 2.0 License | 1 |
| Apache License 2.0 | 1 |
| Apache Software License AND MIT License | 1 |
| Apache-2 | 1 |
| Apache-2.0 AND BSD-3-Clause AND MIT | 1 |
| Apache-2.0 AND CC-BY-4.0 AND LicenseRef-scancode-other-permissive AND LicenseRef-scancode-other-copyleft | 1 |
| Apache-2.0 AND CNRI-Python | 1 |
| Apache-2.0 AND MIT | 1 |
| Apache-2.0 OR BSD-2-Clause | 1 |
| Apache-2.0 OR BSD-3-Clause | 1 |
| Apache-2.0 OR MIT | 1 |
| Artistic License AND GNU General Public License (GPL) AND GNU General Public License v2 or later (GPLv2+) | 1 |
| BSD | 1 |
| BSD-3-Clause AND 0BSD AND MIT AND Zlib AND CC0-1.0 | 1 |
| BSD-3-Clause and Public-Domain | 1 |
| GNU Affero General Public License v3 or later (AGPLv3+) | 1 |
| GNU Lesser General Public License v2 or later (LGPLv2+) | 1 |
| GNU Lesser General Public License v3 or later (LGPLv3+) | 1 |
| GNU Library or Lesser General Public License (LGPL) | 1 |
| GPL-3.0-or-later OR MIT | 1 |
| ISC License (ISCL) | 1 |
| LicenseRef-NVIDIA-Proprietary | 1 |
| LicenseRef-NVIDIA-SOFTWARE-LICENSE | 1 |
| MIT AND BSD-3-Clause | 1 |
| MIT License AND Mozilla Public License 2.0 (MPL 2.0) | 1 |
| MPL-2.0 AND (Apache-2.0 OR MIT) | 1 |
| MPL-2.0 AND MIT | 1 |
| Mozilla Public License 2.0 (MPL 2.0) | 1 |
| NVIDIA Proprietary Software | 1 |
| PSF-2.0 | 1 |
| Python Software Foundation License | 1 |
| apache-2.0 AND bsd-simplified | 1 |
| apache-2.0 AND bsd-simplified-darwin AND (bsd-simplified AND public-domain AND bsd-new AND isc AND (bsd-new OR gpl-1.0-plus) AND bsd-original) | 1 |
| apache-2.0 AND lgpl-2.1 and unrar and brian-gladman-3-clause | 1 |

## JavaScript loaded from CDNs

| Library | Version | Integrity hash | Loaded by |
|---------|---------|----------------|-----------|
| fonts.googleapis.com | none | no | `raft/static/index.html`, `skills/marshal-eval-review.html` |
| xlsx | 0.20.3 | yes | `skills/marshal-eval-review.html` |
| xterm | 5.3.0 | yes | `dashboard/upc-console.html` |
| xterm-addon-fit | 0.8.0 | yes | `dashboard/upc-console.html` |

## Licenses not read

| Ecosystem | Name | Version | Why |
|-----------|------|---------|-----|
| Go | cel.dev/expr | v0.25.2 | no source in the module cache; compiled into no package here |
| Go | github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp | v1.34.0 | no source in the module cache; compiled into no package here |
| Go | github.com/bwesterb/go-ristretto | v1.2.4 | no source in the module cache; compiled into no package here |
| Go | github.com/cncf/xds/go | v0.0.0-20260202195803-dba9d589def2 | no source in the module cache; compiled into no package here |
| Go | github.com/containerd/stargz-snapshotter/estargz | v0.16.3 | no source in the module cache; compiled into no package here |
| Go | github.com/coreos/go-systemd/v22 | v22.7.0 | no source in the module cache; compiled into no package here |
| Go | github.com/creack/pty | v1.1.24 | no source in the module cache; compiled into no package here |
| Go | github.com/docker/cli | v29.2.0+incompatible | no source in the module cache; compiled into no package here |
| Go | github.com/docker/distribution | v2.8.3+incompatible | no source in the module cache; compiled into no package here |
| Go | github.com/docker/docker-credential-helpers | v0.9.3 | no source in the module cache; compiled into no package here |
| Go | github.com/envoyproxy/go-control-plane | v0.14.0 | no source in the module cache; compiled into no package here |
| Go | github.com/envoyproxy/go-control-plane/envoy | v1.37.0 | no source in the module cache; compiled into no package here |
| Go | github.com/envoyproxy/go-control-plane/ratelimit | v0.1.0 | no source in the module cache; compiled into no package here |
| Go | github.com/envoyproxy/protoc-gen-validate | v1.3.3 | no source in the module cache; compiled into no package here |
| Go | github.com/go-jose/go-jose/v4 | v4.1.4 | no source in the module cache; compiled into no package here |
| Go | github.com/golang-jwt/jwt/v5 | v5.3.0 | no source in the module cache; compiled into no package here |
| Go | github.com/golang/glog | v1.2.5 | no source in the module cache; compiled into no package here |
| Go | github.com/google/go-containerregistry | v0.20.6 | no source in the module cache; compiled into no package here |
| Go | github.com/inconshreveable/mousetrap | v1.1.0 | no source in the module cache; compiled into no package here |
| Go | github.com/klauspost/compress | v1.18.0 | no source in the module cache; compiled into no package here |
| Go | github.com/opencontainers/go-digest | v1.0.0 | no source in the module cache; compiled into no package here |
| Go | github.com/opencontainers/image-spec | v1.1.1 | no source in the module cache; compiled into no package here |
| Go | github.com/pkg/errors | v0.9.1 | no source in the module cache; compiled into no package here |
| Go | github.com/planetscale/vtprotobuf | v0.6.1-0.20240319094008-0393e58bdf10 | no source in the module cache; compiled into no package here |
| Go | github.com/rs/xid | v1.6.0 | no source in the module cache; compiled into no package here |
| Go | github.com/spiffe/go-spiffe/v2 | v2.8.1 | no source in the module cache; compiled into no package here |
| Go | github.com/vbatts/tar-split | v0.12.1 | no source in the module cache; compiled into no package here |
| Go | go.opentelemetry.io/contrib/detectors/gcp | v1.44.0 | no source in the module cache; compiled into no package here |
| Go | go.uber.org/goleak | v1.3.0 | no source in the module cache; compiled into no package here |
| Go | google.golang.org/genproto/googleapis/api | v0.0.0-20260706201446-f0a921348800 | no source in the module cache; compiled into no package here |
| Go | gotest.tools/v3 | v3.5.0 | no source in the module cache; compiled into no package here |
| Go | modernc.org/memory | v1.12.1 | LICENSE-LOGO not recognised |
| JavaScript | fonts.googleapis.com |  | loaded from a CDN, no copy on this host |
| JavaScript | xlsx | 0.20.3 | loaded from a CDN, no copy on this host |
| JavaScript | xterm | 5.3.0 | loaded from a CDN, no copy on this host |
| JavaScript | xterm-addon-fit | 0.8.0 | loaded from a CDN, no copy on this host |

## GPL boundary

REVIEW: licenses that name the GPL family, for a person to rule on: r-efi 5.3.0 (MIT OR Apache-2.0 OR LGPL-2.1-or-later), r-efi 6.0.0 (MIT OR Apache-2.0 OR LGPL-2.1-or-later), chardet 5.2.0 (GNU Lesser General Public License v2 or later (LGPLv2+)), extractcode-7z 16.5.210531 (apache-2.0 AND lgpl-2.1 and unrar and brian-gladman-3-clause), gemfileparser2 0.9.4 (GPL-3.0-or-later OR MIT), intbitset 4.1.0 (GNU Lesser General Public License v3 or later (LGPLv3+)), psycopg2-binary 2.9.11 (GNU Library or Lesser General Public License (LGPL)), text-unidecode 1.3 (Artistic License AND GNU General Public License (GPL) AND GNU General Public License v2 or later (GPLv2+)), typecode-libmagic 5.39.210531 (apache-2.0 AND bsd-simplified-darwin AND (bsd-simplified AND public-domain AND bsd-new AND isc AND (bsd-new OR gpl-1.0-plus) AND bsd-original)), wikiextractor 3.0.6 (GNU Affero General Public License v3 or later (AGPLv3+)). 32 of 148 Go modules, 0 of 352 Rust crates, 0 of 197 Python packages and 4 of 4 JavaScript files are not read.

This report reads licenses. It does not rule on whether they fit with
GPL-3.0-or-later. "Name the GPL family" includes a crate offering the LGPL as
one of several choices.

The compiled fixture under `doom/` is GPL-2.0. It runs inside the kernel's eBPF
VM and talks through BPF maps, so nothing links it. HAProxy (GPL-2.0) is a
separate process reached over sockets.
