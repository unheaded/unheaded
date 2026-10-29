<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# Third-party inventory (2026-10-29)

Everything outside Unheaded that the repo pulls in, calls, bundles or carries, with the commit that first brought each one in. Stevie fills the **Verdict** column (keep / replace with our own / cut). From 2026-10-29 nothing third-party is added, bumped or bundled without his confirmation (extends ADR-096, which only gates Go and Rust packages).

**How to read "First added":** the oldest commit on develop whose diff adds the name in the files listed (git pickaxe). **By = assistant** means that commit carries a `Co-Authored-By: Claude` trailer. Trailers only exist from 2026-02-07, so a blank **By** before that date means *unknown*, not *Stevie*. Transitive packages are counted, not listed.

**Totals:** 216 direct items, 156 first added in an assistant-co-authored commit. Transitive: 149 Go modules in the build graph, 428 unique Rust crates across the 24 Cargo.lock files.

| Section | Items | Assistant-added |
|---|---:|---:|
| Go modules (direct) | 18 | 6 |
| Rust crates (direct) | 67 | 31 |
| Python packages (direct) | 45 | 45 |
| Browser assets loaded from a CDN | 4 | 4 |
| GitHub Actions | 23 | 17 |
| Container images | 25 | 22 |
| External tools | 34 | 31 |
| Foreign code in the tree | 5 | see below |
| Outside services the code calls | 9 | see below |

## Go modules (direct)

| Item | Version / tag | Where | First added | By | Verdict |
|---|---|---|---|---|---|
| `github.com/gorilla/websocket` | v1.5.3 | go.mod | `7d90a293` 2026-01-26 foundation for Unheaded alpha |  | |
| `github.com/rs/zerolog` | v1.35.1 | go.mod | `7d90a293` 2026-01-26 foundation for Unheaded alpha |  | |
| `golang.org/x/sys` | v0.49.0 | go.mod | `7d90a293` 2026-01-26 foundation for Unheaded alpha |  | |
| `golang.org/x/text` | v0.43.0 | go.mod | `7d90a293` 2026-01-26 foundation for Unheaded alpha |  | |
| `google.golang.org/grpc` | v1.84.0 | go.mod | `7d90a293` 2026-01-26 foundation for Unheaded alpha |  | |
| `google.golang.org/protobuf` | v1.36.12 | go.mod | `7d90a293` 2026-01-26 foundation for Unheaded alpha |  | |
| `gopkg.in/yaml.v3` | v3.0.1 | go.mod | `7d90a293` 2026-01-26 foundation for Unheaded alpha |  | |
| `github.com/cilium/ebpf` | v0.22.0 | go.mod | `77a2c850` 2026-01-29 more churn |  | |
| `golang.org/x/crypto` | v0.58.0 | go.mod | `77a2c850` 2026-01-29 more churn |  | |
| `github.com/google/uuid` | v1.6.0 | go.mod | `a68d6e57` 2026-02-02 AI slop |  | |
| `modernc.org/sqlite` | v1.61.0 | go.mod | `a68d6e57` 2026-02-02 AI slop |  | |
| `golang.org/x/time` | v0.16.0 | go.mod | `31849ec5` 2026-02-06 ai slop prealpha scaffolding |  | |
| `github.com/BurntSushi/toml` | v1.6.0 | go.mod | `9e7b22a1` 2026-02-09 feat: timeline sync tool (JSON/TOML/YAML), PII containment config, QUI | assistant | |
| `github.com/yuin/goldmark` | v1.8.6 | go.mod | `c38e508b` 2026-02-23 feat(ws4): wiki server, documentation, and conference outline — WS4 co | assistant | |
| `github.com/cloudflare/circl` | v1.6.5 | go.mod | `4ae8e1d1` 2026-03-05 feat: kingdom startup/shutdown services, dashboard CPU fix, timeguru O | assistant | |
| `github.com/unheaded/doomgeneric` | v0.0.0 | go.mod | `5ca8c44f` 2026-03-11 feat(champion): add RAFT battle plan + refactor doom to external modul | assistant | |
| `github.com/lib/pq` | v1.12.3 | go.mod | `68ce478b` 2026-03-15 feat(db): add PostgreSQL backend — The Well (Kingdom Persistent Memory | assistant | |
| `github.com/anthropics/anthropic-sdk-go` | v1.82.0 | go.mod | `f32f0391` 2026-10-23 feat(claudebridge): Claude API tool loop behind the Champion gate and  | assistant | |

## Rust crates (direct)

| Item | Version / tag | Where | First added | By | Verdict |
|---|---|---|---|---|---|
| `aya` | 0.13 | cmd ×4, crates | `050a95dc` 2026-01-26 more scaffoling and prep |  | |
| `tokio` | 1 / 1.35 | cmd ×4, crates ×2 | `050a95dc` 2026-01-26 more scaffoling and prep |  | |
| `anyhow` | 1 / 1.0 | cmd ×4, crates ×2 | `77a2c850` 2026-01-29 more churn |  | |
| `aya-ebpf` | 0.1 | cmd, crates, ebpf | `77a2c850` 2026-01-29 more churn |  | |
| `bytes` | 1.5 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `clap` | 4 / 4.4 | cmd ×4, crates ×2 | `77a2c850` 2026-01-29 more churn |  | |
| `crossbeam` | 0.8 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `flate2` | 1.0 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `hostname` | 0.3 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `http-body-util` | 0.1 | cmd ×2 | `77a2c850` 2026-01-29 more churn |  | |
| `hyper` | 1 / 1.1 | cmd ×2 | `77a2c850` 2026-01-29 more churn |  | |
| `hyper-util` | 0.1 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `libc` | 0.2 | cmd ×2, crates | `77a2c850` 2026-01-29 more churn |  | |
| `memmap2` | 0.9 | cmd, crates | `77a2c850` 2026-01-29 more churn |  | |
| `once_cell` | 1.19 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `parking_lot` | 0.12 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `pin-project` | 1.1 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `prometheus` | 0.14 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `prost` | 0.13 | cmd ×2, crates | `77a2c850` 2026-01-29 more churn |  | |
| `prost-types` | 0.13 | cmd | `77a2c850` 2026-01-29 more churn |  | |
| `serde` | 1 / 1.0 | cmd ×4, crates ×2 | `77a2c850` 2026-01-29 more churn |  | |
| `serde_json` | 1 / 1.0 | cmd ×3, crates ×2 | `77a2c850` 2026-01-29 more churn |  | |
| `thiserror` | 1 / 1.0 / 2 | crates ×2, cmd | `77a2c850` 2026-01-29 more churn |  | |
| `toml` | 0.8 | cmd ×2 | `77a2c850` 2026-01-29 more churn |  | |
| `tonic` | 0.12 | cmd ×3, crates | `77a2c850` 2026-01-29 more churn |  | |
| `tonic-build` | 0.12 | cmd ×2, crates | `77a2c850` 2026-01-29 more churn |  | |
| `tracing` | 0.1 | cmd ×2, crates ×2 | `77a2c850` 2026-01-29 more churn |  | |
| `tracing-subscriber` | 0.3 | crates ×2, cmd | `77a2c850` 2026-01-29 more churn |  | |
| `regex` | 1 / 1.10 | cmd ×2 | `5ce8dd90` 2026-01-31 Mountain of AI slop that needs work and cleaning |  | |
| `rustls` | 0.23 | cmd ×4, crates | `5ce8dd90` 2026-01-31 Mountain of AI slop that needs work and cleaning |  | |
| `tokio-rustls` | 0.26 | cmd ×2 | `5ce8dd90` 2026-01-31 Mountain of AI slop that needs work and cleaning |  | |
| `fastrand` | 2.0 | cmd | `a68d6e57` 2026-02-02 AI slop |  | |
| `futures-util` | 0.3 | cmd, crates | `a68d6e57` 2026-02-02 AI slop |  | |
| `tokio-tungstenite` | 0.21 / 0.24 | cmd, crates | `a68d6e57` 2026-02-02 AI slop |  | |
| `ctrlc` | 3 | cmd | `fd972f66` 2026-02-10 feat(ebpf): add aya userspace loader, 4/4 eBPF programs loaded (E1 res | assistant | |
| `env_logger` | 0.11 | cmd ×2 | `fd972f66` 2026-02-10 feat(ebpf): add aya userspace loader, 4/4 eBPF programs loaded (E1 res | assistant | |
| `log` | 0.4 | cmd ×2 | `fd972f66` 2026-02-10 feat(ebpf): add aya userspace loader, 4/4 eBPF programs loaded (E1 res | assistant | |
| `goblin` | 0.8 / 0.9 | crates ×2 | `6fb51bb7` 2026-02-18 feat(mbc): rewrite RV32I→MBC translator to two-pass architecture + ELF | assistant | |
| `ureq` | 3 | cmd | `4e7e8bf8` 2026-02-18 looser files |  | |
| `libfuzzer-sys` | 0.4 | crates ×3 | `3e733bc2` 2026-02-21 feat(doom): S30 sprint — MBC ISA hardening, Go loader, fuzz campaign,  | assistant | |
| `aes-gcm` | 0.10 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `axum` | 0.7 | crates | `6f96943c` 2026-03-29 fix(doom): expand heap to 26MB + relocate WAD/stack for clearance | assistant | |
| `bincode` | 2 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `blake3` | 1 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `byteorder` | 1 | crates | `7995a2a8` 2026-03-29 feat(doom): scaffold doom-runner Aya-based runtime | assistant | |
| `chrono` | 0.4 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `criterion` | 0.5 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `hex` | 0.4 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `hkdf` | 0.12 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `pqcrypto-traits` | 0.3 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `proptest` | 1 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `quinn` | 0.11 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `rand` | 0.8 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `ring` | 0.17 | crates | `ba9454e0` 2026-03-29 feat(zhend): Phase 8 QUIC/HTTP3 edge transport via quinn | assistant | |
| `sha2` | 0.10 | crates ×3, cmd | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `sled` | 0.34 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `tempfile` | 3 | crates ×2 | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `tokio-stream` | 0.1 | crates | `a6374a3c` 2026-03-29 feat(zhend): Phase 6 gRPC service — Ingest + Surface + Status RPCs liv | assistant | |
| `x25519-dalek` | 2 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `zeroize` | 1 | crates | `77f19201` 2026-03-29 feat(zhen): add zhend crate + ADR-021 + PQ threat model + S75 battle p | assistant | |
| `half` | 2.4 | crates | `9516b281` 2026-04-03 feat(zhenai-forge): Phase 1 — GGUF reader + LoRA math + Adam optimizer | assistant | |
| `pqcrypto-mldsa` | 0.1 | crates | `cea6f91a` 2026-05-08 feat(zhend): pqcrypto FIPS 205/203 migration (RUSTSEC-2024-0380/0381) | assistant | |
| `pqcrypto-mlkem` | 0.1 | crates | `cea6f91a` 2026-05-08 feat(zhend): pqcrypto FIPS 205/203 migration (RUSTSEC-2024-0380/0381) | assistant | |
| `reqwest` | 0.12 | cmd ×2 | `f8f0c6d3` 2026-05-10 [PLAN P1.1] Steps 23-42: Phase 2 — BootRunner with full aya-pattern li |  | |
| `arbitrary` | 1 | crates | `f47993e5` 2026-09-09 fix(lich): the S21 fuzz harnesses had never been built, run, or assert | assistant | |
| `aws-lc-rs` | 1 | cmd, crates | `a0662247` 2026-10-23 fix(waf): rustls on aws-lc-rs negotiates X25519MLKEM768; ring dropped; | assistant | |
| `base64` | 0.22 | cmd, crates | `34ce8b61` 2026-10-27 [PLAN S-IRON-GATE] Phase 7 follow-up: Rust trace-collector carries a W | assistant | |

## Python packages (direct)

| Item | Version / tag | Where | First added | By | Verdict |
|---|---|---|---|---|---|
| `anthropic` | ==0.84.0 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `datasets` | ==5.0.1 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `faiss-cpu` | ==1.13.2 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `Flask` | ==3.1.3 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `langchain-text-splitters` | ==1.1.2 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `numpy` | ==2.4.3 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `peft` | ==0.18.1 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `psycopg2-binary` | ==2.9.11 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `pytest` | ==9.0.3 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `PyYAML` | ==6.0.3 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `requests` | ==2.34.2 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `sentence-transformers` | ==5.7.0 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `torch` | ==2.10.0 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `transformers` | ==5.17.0 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `trl` | ==0.29.0 | raft/requirements.txt | `bc6668a8` 2026-08-04 fix(raft): declare the Python dependencies — the SBOM could not see an | assistant | |
| `bandit` | ==1.9.4 | .github/ci-python/requirements.in | `5ea99210` 2026-10-22 ci(python): CI tools installed from one hash-locked requirements file | assistant | |
| `certifi` | >=2026.10.19 | .github/ci-python/requirements.in | `5ea99210` 2026-10-22 ci(python): CI tools installed from one hash-locked requirements file | assistant | |
| `jsonschema` | ==4.26.0 | .github/ci-python/requirements.in | `5ea99210` 2026-10-22 ci(python): CI tools installed from one hash-locked requirements file | assistant | |
| `pip-audit` | ==2.10.1 | .github/ci-python/requirements.in | `5ea99210` 2026-10-22 ci(python): CI tools installed from one hash-locked requirements file | assistant | |
| `pyyaml` | ==6.0.3 | .github/ci-python/requirements.in | `5ea99210` 2026-10-22 ci(python): CI tools installed from one hash-locked requirements file | assistant | |
| `ruff` | ==0.17.1 | .github/ci-python/requirements.in | `5ea99210` 2026-10-22 ci(python): CI tools installed from one hash-locked requirements file | assistant | |
| `yamllint` | ==1.38.0 | .github/ci-python/requirements.in | `5ea99210` 2026-10-22 ci(python): CI tools installed from one hash-locked requirements file | assistant | |
| `pytest` | ==9.1.1 | .github/ci-python/requirements.in | `c9e62ea1` 2026-10-23 [PLAN S-TWO-BRAINS] Steps 271-275: runbook checks in CI, provoked | assistant | |
| `Pygments` | ==2.21.0 | raft/requirements.txt | `b4d4eca5` 2026-10-24 feat(zhen): server-side syntax highlighting with Pygments | assistant | |
| `ansible-core` | ==2.21.5 | .github/ci-python/requirements.in | `d4cc446b` 2026-10-27 [PLAN S-WELL-SPRING] Steps 265-273: new fixes + record-existing, remed | assistant | |
| `beautifulsoup4` | ==4.15.0 | tomb/grimoire/rag/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `chromadb` | ==0.6.3 | tomb/grimoire/rag/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `jupyter` | ==1.1.1 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `jupyterlab` | ==4.6.4 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `markdown` | ==3.11.2 | tomb/grimoire/rag/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `matplotlib` | ==3.11.2 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `nbformat` | ==5.11.1 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `numpy` | ==2.5.4 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `pandas` | ==3.0.6 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `plotly` | ==7.1.0 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `prometheus-api-client` | ==0.7.2 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `psutil` | ==7.2.2 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `pyyaml` | ==6.0.3 | tomb/grimoire/rag/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `scipy` | ==1.18.1 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `seaborn` | ==0.13.2 | notebooks/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `sentence-transformers` | ==2.7.0 | tomb/grimoire/rag/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `tokenizers` | ==0.22.2 | tomb/grimoire/rag/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `torch` | ==2.14.1 | tomb/grimoire/rag/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `tqdm` | ==4.70.1 | tomb/grimoire/rag/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |
| `transformers` | ==4.57.6 | tomb/grimoire/rag/requirements.in | `9c29dcc3` 2026-10-28 [PLAN S-TRUE-WIRE] Step 374 (part): ci-python-lock.sh takes a director | assistant | |

## Browser assets loaded from a CDN

| Item | Version / tag | Where | First added | By | Verdict |
|---|---|---|---|---|---|
| `Google Fonts` |  | raft, skills | `d4e2e84d` 2026-02-27 feat(s72): Marshal skill + battle plan + timeline — Round Table forged | assistant | |
| `xlsx (SheetJS)` | 0.20.3 | skills | `d4e2e84d` 2026-02-27 feat(s72): Marshal skill + battle plan + timeline — Round Table forged | assistant | |
| `xterm` | 5.3.0 | dashboard | `f2965bff` 2026-05-09 feat(upc-tty-bridge): WebSocket bridge + xterm.js Mode A demo surface | assistant | |
| `xterm-addon-fit` | 0.8.0 | dashboard | `f2965bff` 2026-05-09 feat(upc-tty-bridge): WebSocket bridge + xterm.js Mode A demo surface | assistant | |

## GitHub Actions

| Item | Version / tag | Where | First added | By | Verdict |
|---|---|---|---|---|---|
| `actions/cache` |  | .github ×5 | `6fa722c8` 2026-02-17 chore: add telemetry pkg, timeguru container, ADR-008, GEMINI, timelin |  | |
| `actions/checkout` |  | .github ×59 | `6fa722c8` 2026-02-17 chore: add telemetry pkg, timeguru container, ADR-008, GEMINI, timelin |  | |
| `actions/setup-go` |  | .github ×19 | `6fa722c8` 2026-02-17 chore: add telemetry pkg, timeguru container, ADR-008, GEMINI, timelin |  | |
| `dtolnay/rust-toolchain` |  | .github ×13 | `6fa722c8` 2026-02-17 chore: add telemetry pkg, timeguru container, ADR-008, GEMINI, timelin |  | |
| `golangci/golangci-lint-action` |  | .github ×1 | `6fa722c8` 2026-02-17 chore: add telemetry pkg, timeguru container, ADR-008, GEMINI, timelin |  | |
| `actions/download-artifact` |  | .github ×3 | `eb7fa574` 2026-02-18 feat: merge Protocol Foundation from unheaded-protocol | assistant | |
| `actions/upload-artifact` |  | .github ×10 | `eb7fa574` 2026-02-18 feat: merge Protocol Foundation from unheaded-protocol | assistant | |
| `docker/build-push-action` |  | .github ×2 | `eb7fa574` 2026-02-18 feat: merge Protocol Foundation from unheaded-protocol | assistant | |
| `docker/login-action` |  | .github ×2 | `eb7fa574` 2026-02-18 feat: merge Protocol Foundation from unheaded-protocol | assistant | |
| `docker/metadata-action` |  | .github ×2 | `eb7fa574` 2026-02-18 feat: merge Protocol Foundation from unheaded-protocol | assistant | |
| `docker/setup-buildx-action` |  | .github ×2 | `eb7fa574` 2026-02-18 feat: merge Protocol Foundation from unheaded-protocol | assistant | |
| `github/codeql-action/upload-sarif` |  | .github ×6 | `eb7fa574` 2026-02-18 feat: merge Protocol Foundation from unheaded-protocol | assistant | |
| `softprops/action-gh-release` |  | .github ×1 | `eb7fa574` 2026-02-18 feat: merge Protocol Foundation from unheaded-protocol | assistant | |
| `aquasecurity/trivy-action` |  | .github ×5 | `6c895195` 2026-02-27 [PLAN S72] Phases 8-10 + Protocol: GHA hardened, Jenkins scaffolded, M | assistant | |
| `anchore/sbom-action` |  | .github ×1 | `136fd8e6` 2026-03-05 feat(sk8): Kubernetes Convergence — 16 phases, production-grade Kingdo | assistant | |
| `anchore/scan-action` |  | .github ×1 | `136fd8e6` 2026-03-05 feat(sk8): Kubernetes Convergence — 16 phases, production-grade Kingdo | assistant | |
| `gruntwork-io/terragrunt-action` |  | .github ×2 | `136fd8e6` 2026-03-05 feat(sk8): Kubernetes Convergence — 16 phases, production-grade Kingdo | assistant | |
| `opentofu/setup-opentofu` |  | .github ×2 | `136fd8e6` 2026-03-05 feat(sk8): Kubernetes Convergence — 16 phases, production-grade Kingdo | assistant | |
| `actions/setup-python` |  | .github ×9 | `4c1f99e7` 2026-05-03 cmd/shield + raft-python CI: close armor-shield and task-cicd |  | |
| `actions/attest-build-provenance` |  | .github ×1 | `5160faa6` 2026-10-27 [PLAN S-IRON-GATE] Steps 304-311: release.yml builds everything, signs | assistant | |
| `github/codeql-action/analyze` |  | .github ×1 | `7a27fd19` 2026-10-27 [PLAN S-IRON-GATE] Steps 313-315: OpenSSF Scorecard and CodeQL workflo | assistant | |
| `github/codeql-action/init` |  | .github ×1 | `7a27fd19` 2026-10-27 [PLAN S-IRON-GATE] Steps 313-315: OpenSSF Scorecard and CodeQL workflo | assistant | |
| `ossf/scorecard-action` |  | .github ×1 | `7a27fd19` 2026-10-27 [PLAN S-IRON-GATE] Steps 313-315: OpenSSF Scorecard and CodeQL workflo | assistant | |

## Container images

| Item | Version / tag | Where | First added | By | Verdict |
|---|---|---|---|---|---|
| `alpine` | 3.24 | docker ×5, deploy ×2, monitoring, routing | `5ce8dd90` 2026-01-31 Mountain of AI slop that needs work and cleaning |  | |
| `golang` | ${GO_VERSION}-bookworm, 1.27.3-alpine3.2 | docker ×26, deploy ×2, Dockerfile | `5ce8dd90` 2026-01-31 Mountain of AI slop that needs work and cleaning |  | |
| `grafana/grafana` | 10.3.3, 10.4.0, 11.5.1 | docker ×2, monitoring, tomb, docker-compose.yml | `5ce8dd90` 2026-01-31 Mountain of AI slop that needs work and cleaning |  | |
| `clickhouse/clickhouse-server` | 24.12.6.70 | docker-compose.yml | `aa700c29` 2026-02-21 feat(sprint): S27 Alpha gate artifacts + S29 Doom assembly/BPF task li | assistant | |
| `coredns/coredns` | 1.12.0 | docker-compose.yml | `aa700c29` 2026-02-21 feat(sprint): S27 Alpha gate artifacts + S29 Doom assembly/BPF task li | assistant | |
| `timberio/vector` | 0.43.1-debian | docker-compose.yml | `aa700c29` 2026-02-21 feat(sprint): S27 Alpha gate artifacts + S29 Doom assembly/BPF task li | assistant | |
| `traefik` | v3.3.7 | cmd, docker-compose.yml | `aa700c29` 2026-02-21 feat(sprint): S27 Alpha gate artifacts + S29 Doom assembly/BPF task li | assistant | |
| `victoriametrics/victoria-metrics` | v1.108.1, v1.99.0 | docker ×2, docker-compose.yml, monitoring | `aa700c29` 2026-02-21 feat(sprint): S27 Alpha gate artifacts + S29 Doom assembly/BPF task li | assistant | |
| `ghcr.io/huggingface/text-embeddings-inference` | cpu-1.5 | deploy | `c28b4b98` 2026-02-24 feat(swarm): S45-S50 pre-written artifacts — docs, UI, Go, proto, conf | assistant | |
| `ghcr.io/open-webui/open-webui` | main | deploy | `c28b4b98` 2026-02-24 feat(swarm): S45-S50 pre-written artifacts — docs, UI, Go, proto, conf | assistant | |
| `prom/prometheus` | v2.50.1, v2.51.0, v3.15.0 | docker ×3, deploy, monitoring, tomb | `c28b4b98` 2026-02-24 feat(swarm): S45-S50 pre-written artifacts — docs, UI, Go, proto, conf | assistant | |
| `qdrant/qdrant` | v1.19.2 | deploy | `c28b4b98` 2026-02-24 feat(swarm): S45-S50 pre-written artifacts — docs, UI, Go, proto, conf | assistant | |
| `vllm/vllm-openai` | latest-rocm | deploy | `c28b4b98` 2026-02-24 feat(swarm): S45-S50 pre-written artifacts — docs, UI, Go, proto, conf | assistant | |
| `debian` | bookworm-slim | docker ×25 | `06854984` 2026-02-26 feat(age2-docker): Docker IaC — parallel deployment option alongside N | assistant | |
| `gcr.io/distroless/static-debian12` | nonroot | docker ×21, deploy | `06854984` 2026-02-26 feat(age2-docker): Docker IaC — parallel deployment option alongside N | assistant | |
| `grafana/loki` | 2.9.0, 2.9.4, 3.0.0 | docker ×2, monitoring, tomb | `06854984` 2026-02-26 feat(age2-docker): Docker IaC — parallel deployment option alongside N | assistant | |
| `grafana/promtail` | 2.9.0, 2.9.4, 3.0.0 | docker ×3, tomb | `06854984` 2026-02-26 feat(age2-docker): Docker IaC — parallel deployment option alongside N | assistant | |
| `lscr.io/linuxserver/wireguard` | 1.0.20250521 | docker ×2 | `06854984` 2026-02-26 feat(age2-docker): Docker IaC — parallel deployment option alongside N | assistant | |
| `prom/alertmanager` | v0.34.1 | monitoring | `2cf6c93f` 2026-02-26 feat(S67/S68/S69): Wave 1 — observability stack + metrics collectors | assistant | |
| `prom/node-exporter` | v1.7.0, v1.8.0 | docker ×2 | `06854984` 2026-02-26 feat(age2-docker): Docker IaC — parallel deployment option alongside N | assistant | |
| `rocm/pytorch` | ${ROCM_VERSION}-ubuntu22.04 | docker | `06854984` 2026-02-26 feat(age2-docker): Docker IaC — parallel deployment option alongside N | assistant | |
| `ubuntu` | 24.04 | docker ×4, routing | `76ac4073` 2026-02-26 feat(age2-firewall): OPNsense+FRR + IPFire+BIRD ingress/egress — NixOS | assistant | |
| `haproxy` | 2.9.15-alpine | docker | `318abe92` 2026-02-28 feat(infra): HAProxy edge/internal + Nginx per-app LB stack with full  | assistant | |
| `nginx` | 1.25.5-alpine | docker | `318abe92` 2026-02-28 feat(infra): HAProxy edge/internal + Nginx per-app LB stack with full  | assistant | |
| `postgres` | 16.15-alpine, 18.0-alpine | cmd ×2, docker-compose.yml | `68ce478b` 2026-03-15 feat(db): add PostgreSQL backend — The Well (Kingdom Persistent Memory | assistant | |

## External tools

| Item | Version / tag | Where | First added | By | Verdict |
|---|---|---|---|---|---|
| `nix` |  | scripts ×3, tomb ×2 (5 files) | `7d90a293` 2026-01-26 foundation for Unheaded alpha |  | |
| `lxc` |  | nix, scripts (2 files) | `a7351ed0` 2026-01-28 very messy cleaning but has some kaban code |  | |
| `bpftool` |  | scripts ×6 (6 files) | `d7dde1fb` 2026-02-23 feat(doom): S33 hardening sprint — script migration + gosec pinning | assistant | |
| `gosec` |  | scripts ×3, .github, Makefile (5 files) | `7339d438` 2026-02-23 fix(security): P0/P1/P2 quick wins + auth skeleton — Agent C+F complet | assistant | |
| `vllm` |  | docker ×2, deploy (3 files) | `c28b4b98` 2026-02-24 feat(swarm): S45-S50 pre-written artifacts — docs, UI, Go, proto, conf | assistant | |
| `birdc` |  | docker (1 files) | `76ac4073` 2026-02-26 feat(age2-firewall): OPNsense+FRR + IPFire+BIRD ingress/egress — NixOS | assistant | |
| `vtysh` |  | docker (1 files) | `76ac4073` 2026-02-26 feat(age2-firewall): OPNsense+FRR + IPFire+BIRD ingress/egress — NixOS | assistant | |
| `wg` |  | raft ×4, scripts ×3, docker (8 files) | `06854984` 2026-02-26 feat(age2-docker): Docker IaC — parallel deployment option alongside N | assistant | |
| `age` |  | scripts (1 files) | `a08b10e9` 2026-02-27 feat(scripts): add LLM lab server bootstrap for Ubuntu 25.x | assistant | |
| `cosign` |  | scripts ×4, .github ×2 (6 files) | `6c895195` 2026-02-27 [PLAN S72] Phases 8-10 + Protocol: GHA hardened, Jenkins scaffolded, M | assistant | |
| `docker` |  | scripts ×20 (20 files) | `a08b10e9` 2026-02-27 feat(scripts): add LLM lab server bootstrap for Ubuntu 25.x | assistant | |
| `incus` |  | scripts (1 files) | `a08b10e9` 2026-02-27 feat(scripts): add LLM lab server bootstrap for Ubuntu 25.x | assistant | |
| `node_exporter` |  | scripts ×2, deploy (3 files) | `a08b10e9` 2026-02-27 feat(scripts): add LLM lab server bootstrap for Ubuntu 25.x | assistant | |
| `ollama` |  | tomb ×7, scripts (8 files) | `a9a1283b` 2026-02-27 enhance(scripts): mad scientist storage strategy + vLLM/AI tooling upg | assistant | |
| `rsync` |  | nix, scripts (2 files) | `b37dfcff` 2026-02-27 feat(scripts): add vault-to-runway smart model tiering + auto-launch v | assistant | |
| `vector` |  | scripts ×3, deploy, runbooks (5 files) | `a08b10e9` 2026-02-27 feat(scripts): add LLM lab server bootstrap for Ubuntu 25.x | assistant | |
| `buf` |  | scripts ×2, tomb (3 files) | `26515122` 2026-03-01 chore: add battle plans, Cargo.lock, tomb archive; clean stale data fi | assistant | |
| `govulncheck` |  | scripts ×2 (2 files) | `16d23a93` 2026-03-05 feat(s77): Age 2 acceleration — protocol drafts, IaC validation, bench | assistant | |
| `helm` |  | deploy ×3, scripts ×3 (6 files) | `136fd8e6` 2026-03-05 feat(sk8): Kubernetes Convergence — 16 phases, production-grade Kingdo | assistant | |
| `nerdctl` |  | scripts (1 files) | `209652cb` 2026-03-19 feat(infra): Operation Three Crowns — dual-host runtime interop valida | assistant | |
| `jenkins` |  | runbooks (1 files) | `47b13b81` 2026-04-03 feat(ci): .deb packaging pipeline + ADR-026 + Jenkins/APT runbooks | assistant | |
| `rocm-smi` |  | runbooks, scripts (2 files) | `750cd9d1` 2026-04-04 feat(runbooks): GPU monitoring + API key audit — 40 total runbooks | assistant | |
| `llama-server` |  | raft, runbooks, scripts (3 files) | `b25f681d` 2026-04-06 progress: 656 Claude QA pairs, A/B test script, ADR status updates | assistant | |
| `kind` |  | deploy ×2 (2 files) | `1a92f0e0` 2026-05-05 WAVE17: K8s substrate proven on kind (9/9 services running) |  | |
| `golangci-lint` |  | scripts (1 files) | `c5543e54` 2026-05-12 feat(scripts): kingdom-health.sh — unified daily/per-session gate veri | assistant | |
| `trivy` |  | scripts ×2, nix (3 files) | `a4c5524e` 2026-05-12 feat(yggdrasil): task #65 packer flow — provisioners + preseed + scrip | assistant | |
| `gitleaks` |  | .github, scripts (2 files) | `3cf95d57` 2026-07-29 feat(ci): secret scanning + multi-language static analysis | assistant | |
| `hadolint` |  | .github, scripts (2 files) | `3cf95d57` 2026-07-29 feat(ci): secret scanning + multi-language static analysis | assistant | |
| `shellcheck` |  | .github, scripts (2 files) | `3cf95d57` 2026-07-29 feat(ci): secret scanning + multi-language static analysis | assistant | |
| `pip-audit` |  | .github (1 files) | `2cafd868` 2026-10-19 feat(ci): pip-audit ratchet for pinned Python requirements; FND-013 to | assistant | |
| `grype` |  | scripts ×2 (2 files) | `fddbd3df` 2026-10-21 ci(sbom): pinned syft/grype replace curl/sh from main; shared fetch he | assistant | |
| `kubeconform` |  | scripts (1 files) | `52da43eb` 2026-10-21 ci(helm): Helm 3.14.0 -> 4.3.0 pinned; a real per-container security g | assistant | |
| `oxlint` |  | .github, scripts (2 files) | `ea507728` 2026-10-21 ci(js): oxlint replaces eslint; no npm anywhere in the repo | assistant | |
| `syft` |  | .github, scripts (2 files) | `fddbd3df` 2026-10-21 ci(sbom): pinned syft/grype replace curl/sh from main; shared fetch he | assistant | |

## Foreign code in the tree

| Item | Licence | Where | Notes | Verdict |
|---|---|---|---|---|
| xv6-riscv (MIT) | MIT | `crates/xv6-mbc/upstream/` (99 files: kernel 45, user 45, mkfs, LICENSE, README); `crates/xv6-mbc/scripts/vendor-xv6.sh` | Phase 2.4 replaced all 16 MIT kernel files with Unheaded-authored ones, but the vendored tree, its LICENSE/README and the vendoring script are still here | |
| Doom (id Software, GPL-2.0) via doomgeneric | GPL-2.0 | `go.mod`: `replace github.com/unheaded/doomgeneric => ../projects/doomgeneric/unheaded` (**a path outside the repo**); `cmd/wotan-ctl/doom.go`, `demos/doom/`, `internal/doom/`, `crates/doom-runner/` | GPL boundary documented in `THIRD_PARTY.md`; the Go build reaches outside the checkout for it | |
| IETF RFC texts | IETF Trust | `docs/protocol/references/rfcs/` | reference copies | |
| ScanCode output | n/a (tool output) | `docs/legal/scancode-full.json` | produced by the third-party ScanCode tool | |
| llama.cpp checkout | MIT | `./llama.cpp/` (gitignored, not tracked) | built and run on west for Zhen; not in history | |

## Outside services the code calls

| Service | Called from | Verdict |
|---|---|---|
| Anthropic API | `pkg/claudebridge` (anthropic-sdk-go), `cmd/zhen-claude`, raft `anthropic` | |
| CISA KEV feed (`www.cisa.gov`) | `cmd/kev-poller` | |
| GitHub / GitHub API | code + CI (Dependabot, Actions, SARIF upload) | |
| Go module proxy / checksum DB (`proxy.golang.org`, `sum.golang.org`) | build (`GOSUMDB`) | |
| LXD / Ubuntu image servers (`images.lxd.canonical.com`, `cloud-images.ubuntu.com`) | Cuirass driver, VM provisioning | |
| Container registries (Docker Hub, gcr.io, ghcr.io, lscr.io) | every image above | |
| CDNs (`cdn.jsdelivr.net`, `cdn.sheetjs.com`, Google Fonts) | browser pages above | |
| Hugging Face model hub | sentence-transformers / transformers downloads (raft, Grimoire) | |
| `auth.unheaded.dev` | OIDC issuer default in code (our domain; listed so it is checked) | |

## Already in flight

- trivy: replace with our own scanner (Stevie 2026-10-29). Used by `security.yml`, `docker.yml`, `scripts/check-trivy-misconfig.sh`, Jenkinsfiles, `runbooks/security/container-security-scan.yaml` (product-facing), the Yggdrasil evidence pack (`scanner: trivy`, product-facing).
- Grimoire `rag/` (chromadb, sentence-transformers, transformers, torch): proposal to delete and serve the corpus through Zhenai MCP, waiting on Stevie.
- `docs/policy/dependency-watch.md` holds the earlier per-module verdicts for the 18 Go modules.

Regenerate: the generator used for this snapshot is not yet a repo script; making it one (and a gate that fails when a new item appears without a verdict) is the proposed next step.

