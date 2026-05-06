# Sealed Cask Trust Chain — Threat Model

**Date:** 2026-05-06
**Author:** Marshal overnight unattended run (BlackMage finding **BM7** follow-up, F2 in tasks #49)
**Owners (handoff):** BlackMage (offensive), MoatGhost (compliance/policy), Architect (build pipeline)
**Surface scoped:** the deployment trust chain rooted in `scripts/build-sealed-cask.sh` (the forge) and `scripts/verify-binding-rune.sh` (the verifier), as specified by ADR-010 (`docs/adr/ADR-010-sealed-cask-deployment.md`).
**Status:** *initial draft* — doc-only; matches the depth/structure of `docs/security/k8s-threat-model-2026-05-06.md`.

> **Why this exists.** The compliance matrix family (`docs/compliance/control-matrix/`) cites Sealed Cask as MAPPED for **CM-2, CM-8, CM-14, SI-7, SR-3, SR-9, SR-11** (NIST 800-53), **PCI 4.1**, **CIS 11.5**, **ISO A.8.19**, **CMMC SI.L2-3.14.1**, and others. Per scrutiny finding **BM7** (`docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` lines 297-306), the Sealed Cask trust chain has never had a threat model written. The matrices treat it as a strong control; in practice neither the build host nor the signing key location is documented anywhere reachable. This document is the missing model.

---

## 1. Scope and assumptions

### In scope
- **The forge:** `scripts/build-sealed-cask.sh` (145 lines) — Phases 1-7 producing `cask-${TIMESTAMP}.tar.gz` and `cask-${TIMESTAMP}.tar.gz.age`.
- **The verifier:** `scripts/verify-binding-rune.sh` (114 lines) — extracts, hashes, compares, reports.
- **The Binding Rune:** `BINDING_RUNE.sha256` — flat-file SHA-256 manifest produced at `build-sealed-cask.sh:92` (`find . -type f ... | xargs sha256sum > "${MANIFEST}"`).
- **The age envelope:** `cask.age.key` (private) at `${PROJECT_ROOT}/secrets/cask.age.key` (per `build-sealed-cask.sh:106`); recipient public key extracted from same file at `build-sealed-cask.sh:119`.
- **The ADR-010 specification:** the *intended* trust chain (Ed25519 signature over a JSON Binding Rune object, parish field, incarnation epoch, deploy-key-2026-02 signer, content-addressed artifact store).
- **The gap between intent and implementation** — ADR-010 specifies an Ed25519-signed JSON object; the script ships a flat SHA-256 manifest with no signature step. (See §3.7.)

### Out of scope
- Kingdom-internal trust (Sophia LUKS keys, Wotan ML-DSA-65 topic signing) — covered by separate ADRs and the Wotan threat model.
- Operator macOS workstation hardening — separate threat model (task #51, F4).
- Sealed Cask post-decrypt runtime (LXD/NixOS hardening) — covered by ADR-007 / ADR-008.
- The Bone Shell / Soul Chamber LUKS split — ADR-010 explicitly defers to Beta; not yet implemented (`docs/adr/ADR-010-sealed-cask-deployment.md:9-10`).
- The hypothetical Sword build pipeline — does not exist as code today; ADR-010 §"The Soul Vessel Pipeline" is aspirational.

### Assumptions
- The build host is **UNVERIFIED**. There is no CI workflow under `.github/workflows/` that invokes `build-sealed-cask.sh`. The script's `PROJECT_ROOT` resolution at `build-sealed-cask.sh:20` (`"$(cd "$(dirname "$0")/.." && pwd)"`) implies a developer-driven local invocation.
- The signing/encryption key is **UNVERIFIED**. The script writes `cask.age.key` to `${PROJECT_ROOT}/secrets/cask.age.key` (`build-sealed-cask.sh:106`). Tonight's audit confirms `/Users/govan/tmp/unheaded/secrets/` does not exist on the operator's macOS workstation; `/Users/govan/tmp/unheaded/pkg/secrets` and `/Users/govan/tmp/unheaded/deploy/k8s/secrets` do exist but are unrelated. **The location of the age key, if any cask has ever been built, is unknown to this audit.**
- **The build is darwin-broken as of 2026-05-06.** Per BM7 and the Phase A baseline, `go build ./...` fails on darwin because Linux-only eBPF code lacks `//go:build linux` tags. `build-sealed-cask.sh:37` runs `go build ./...` unconditionally. **Therefore, every successful build of Sealed Cask to date has occurred on a Linux host.** Which host is **UNVERIFIED**.
- Adopters of Unheaded receive caskets out-of-band. There is no published verification key. There is no documented public-key distribution channel. (See §3.4.)
- The `set -euo pipefail` discipline at `build-sealed-cask.sh:18` and `verify-binding-rune.sh:11` is honoured; we assume bash itself isn't compromised. **This assumption is load-bearing** — see §4.4.

---

## 2. Trust chain diagram (textual)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Z0 — DEVELOPER WORKSTATION (operator macOS)                                 │
│   * git clone unheaded → ~/tmp/unheaded                                      │
│   * INVARIANT VIOLATED on darwin: go build ./... fails (BM7)                 │
│   * SSH keys to WEST + EAST + GitHub                                         │
│   * Holds Stevie's identity, git-commit privilege                            │
└─────────────────┬───────────────────────────────────────────────────────────┘
                  │ git push  (or scp source)
                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Z1 — BUILD HOST (Linux, UNVERIFIED — likely WEST or EAST bare metal)        │
│   * Runs build-sealed-cask.sh                                                │
│   * Phase 1: go build ./...      (compiles every cmd/* and services/*/cmd/*)│
│   * Phase 2: cargo build --release in ./ebpf/                                │
│   * Phase 3: stages bin/, ebpf/, configs/, nix/, runbooks/, db/             │
│   * Phase 4: sha256sum every staged file → BINDING_RUNE.sha256              │
│   * Phase 5: tar czf cask-<ts>.tar.gz                                        │
│   * Phase 6: age -r <pub> -o cask.tar.gz.age                                 │
│   * Phase 7: rm -rf staging                                                  │
│                                                                              │
│   KEY MATERIAL CO-LOCATED HERE:                                              │
│     - ${PROJECT_ROOT}/secrets/cask.age.key   (private + public)              │
│     - mode 600, owned by build user                                          │
│     - auto-generated on first run if absent (build-sealed-cask.sh:111-117)   │
│     - NEVER ROTATED (no rotation logic exists)                               │
│     - NO BACKUP (no backup logic exists)                                     │
│     - NO HSM (no HSM hook exists)                                            │
└─────────────────┬───────────────────────────────────────────────────────────┘
                  │ scp / rsync / artifact upload
                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Z2 — TRANSIT (SSH, S3, registry, USB key — UNSPECIFIED)                     │
│   * cask-<ts>.tar.gz.age + (implicit) public-key delivery                    │
│   * Integrity: age authenticated encryption (X25519 + ChaCha20-Poly1305)     │
│   * BUT: age recipient list is the BUILD's own public key —                  │
│     the recipient is the BUILDER, not the ADOPTER.                           │
│     This means the script encrypts to ITSELF.  It does NOT establish         │
│     an out-of-band trust path to a third-party adopter.                      │
└─────────────────┬───────────────────────────────────────────────────────────┘
                  │ adopter receives cask.tar.gz.age + cask.age.key (?!)
                  ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│ Z3 — ADOPTER HOST (Linux node, runs verify-binding-rune.sh)                 │
│   * age -d -i cask.age.key cask.tar.gz.age > cask.tar.gz                     │
│     (REQUIRES THE PRIVATE KEY — see §3.4 — broken trust model)               │
│   * verify-binding-rune.sh cask.tar.gz                                       │
│     - extracts to /tmp/verify-cask-$$                                        │
│     - reads BINDING_RUNE.sha256                                              │
│     - re-hashes every file, compares                                         │
│     - reports PASS/MODIFIED/MISSING/EXTRA                                    │
│   * NO SIGNATURE CHECK — manifest itself is unsigned                         │
│   * EXIT 0 = "BINDING RUNE INTACT" → adopter deploys                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Trust flows downward from Z0 to Z3 with no cryptographic link between zones beyond the age symmetric envelope.** The chain has no Ed25519/ML-DSA-65 signature on the manifest, no transparent log of rune hashes, and no public-key infrastructure — despite ADR-010 specifying all three.

---

## 3. Per-component STRIDE

### 3.1 The build host (Z1)

| Threat | Severity | Notes |
|--------|----------|-------|
| **S** — Spoofing of "official" build origin | **Critical** | No `signed_by` / `origin` field in the actual manifest. The script writes `find . | xargs sha256sum` — any host running the script can produce a "valid" Binding Rune. There is no asymmetric proof of origin. ADR-010:91 specifies `"signed_by": "deploy-key-2026-02"` but `build-sealed-cask.sh` writes no such field. |
| **T** — Tampering with build inputs | **High** | If the build host has a tampered `go` toolchain, tampered `cargo`, or tampered source under `${PROJECT_ROOT}`, the resulting cask is poisoned but the SHA-256 manifest still passes (it hashes the poisoned files honestly). Defence requires: reproducible builds (Nix per ADR-010 §"Why NixOS Is Perfect" — **NOT IMPLEMENTED**), source-side signing, or a second independent build. |
| **R** — Repudiation of who built which cask | **High** | The output filename is `cask-${TIMESTAMP}.tar.gz` (`build-sealed-cask.sh:22`) — timestamp only, no builder identity, no git commit, no host fingerprint. ADR-010:91 specifies `"git_commit": "a1b2c3d"` and `"built_at": ...` in the rune; **neither is captured in the implementation**. A forensic question "who built this cask, on what host, from what commit?" has no answer from the cask itself. |
| **I** — Info disclosure on build host | **Medium** | The build host has the source tree, the age private key (mode 600 per `build-sealed-cask.sh:115`), all build outputs, and the operator's git remote credentials. Compromise = full read of all of the above. UNVERIFIED whether the build host is shared with other workloads (e.g., is WEST also running production services? if yes, the blast radius is wider). |
| **D** — DoS of build pipeline | Low | A locally-run script on a single Linux box. DoS-ing it just means the operator can't cut a release; not a security finding for a Kingdom that doesn't sell SLAs. |
| **E** — Elevation via build host compromise | **Critical** | **The build host is the keystone.** Compromise of the Linux box that builds Sealed Cask gives the attacker: (a) a valid age signing key for future caskets, (b) the ability to inject malicious binaries that hash correctly, (c) (transitively) access to whatever SSH keys are co-located. Per BM7's wording: *"Compromising the dev box compromises Sealed Cask's SHA-256 chain (the attacker signs new caskets that match SHA-256 chain because they have the keys)."* |

### 3.2 The signing / encryption key (`cask.age.key`)

| Threat | Severity | Notes |
|--------|----------|-------|
| **Location** | **Critical (UNVERIFIED)** | The script expects the key at `${PROJECT_ROOT}/secrets/cask.age.key` (`build-sealed-cask.sh:106`). On the operator's macOS workstation, that path does not exist (audited tonight). On the build host (UNVERIFIED Linux box), the key may or may not exist. **There is no inventory of where the key lives.** |
| **Auto-generation** | **High** | If the key is absent, `build-sealed-cask.sh:111-117` auto-generates a fresh keypair and continues. **This means a casual rebuild on a different host produces a brand-new "trusted" key, indistinguishable from the original.** An adopter cannot detect that the signing identity changed. There is no key-pinning. |
| **Rotation** | **High** | No rotation logic. ADR-010:91 specifies an `incarnation` field tracking key epoch — not implemented. Once a key is born, it lives forever or is silently replaced (see auto-generation). |
| **Backup** | **High** | No backup logic. If the build host's disk fails, every prior cask becomes unverifiable (the key is gone) AND a new key auto-generated on a replacement host produces "valid" caskets that aren't cryptographically connected to the prior chain. |
| **Recovery** | **High** | No recovery procedure documented. If the key is exfiltrated, there's no revocation channel — adopters have no way to learn "the 2026-04 key is burned, trust only the 2026-05 key." |
| **HSM / hardware protection** | **Critical** | None. The key is a flat-file at mode 600 on a Linux box of unknown hardening posture. ADR-010:131 says *"the deploy signing key, which lives in HSM/Sophia, never on any node."* The implementation contradicts the spec — the key lives **on the build node, in the filesystem, in plaintext.** |
| **Confusion of roles** | **Critical (architectural)** | The `cask.age.key` is used as both the encryption recipient AND (presumably) the signing identity. age does not sign — it only encrypts. **There is no signature step at all.** The script encrypts the cask to its own public key (`build-sealed-cask.sh:119-120`), which means an adopter who decrypts it must possess the private key. **An adopter cannot decrypt the cask without the same private key the builder uses to sign new ones.** This is not a deployment trust chain — it's a backup-encryption-to-self pattern. |

### 3.3 The Binding Rune manifest (SHA-256)

| Threat | Severity | Notes |
|--------|----------|-------|
| **Algorithm strength** | Medium (today) / High (post-quantum horizon) | SHA-256 is unbroken classically but is **not signed**. A post-quantum collision attack on SHA-256 is not realistic in 2026 — the threat is not the hash, it's the absence of a signature over the hash. |
| **No signature on manifest** | **Critical** | `build-sealed-cask.sh:92` produces `BINDING_RUNE.sha256` and stops. Nothing signs it. `verify-binding-rune.sh:42-46` re-reads the manifest and trusts whatever hashes are inside it. **An attacker with write access to the cask post-build can rewrite both the file and its line in the manifest, and verification passes.** The verifier check at `verify-binding-rune.sh:79-84` for `[EXTRA]` files only catches *insertions*, not coordinated *substitutions*. |
| **Path-traversal in manifest entries** | Low-Medium | The manifest stores relative paths (`./ebpf/foo.bpf.o`) and the verifier uses `[ ! -f "${FILE}" ]` (`verify-binding-rune.sh:61`) and `sha256sum "${FILE}"` (`verify-binding-rune.sh:67`). If a maliciously-crafted manifest contains `../../../etc/passwd` style entries, the verifier would attempt to read outside the cask. tar extraction via `tar xzf` (`verify-binding-rune.sh:31`) without `--no-same-owner --no-same-permissions --no-overwrite-dir` flags is itself a tarball-traversal risk. **UNVERIFIED whether modern bsdtar / GNU tar default-rejects these.** |
| **Manifest = own-line problem** | **High** | Line 92 uses `find . -type f -not -name "BINDING_RUNE.sha256"` — the manifest excludes itself. So the manifest itself is **not in the manifest**. The verifier never checks that `BINDING_RUNE.sha256` is the manifest the builder produced. An attacker can replace both the manifest and the files it points to. |
| **awk-parsing of hashes** | Low | `verify-binding-rune.sh:58-59` uses `awk '{print $1}'` and `awk '{print $2}'` to split. Filenames with whitespace cause silent corruption (the second column is truncated). Non-issue for binaries/configs in this project; would matter if user-supplied filenames ever entered the cask. |
| **`grep` based EXTRA detection** | Medium | `verify-binding-rune.sh:80` uses `grep -q "$f" "${MANIFEST}"` — substring match, not exact match. A file `./foo` matches a manifest line for `./foobar`. False negatives on EXTRA detection. |

### 3.4 Public-key distribution to adopters

| Threat | Severity | Notes |
|--------|----------|-------|
| **No published verification key** | **Critical** | There is no `keys/` directory in the repo. `docs/adr/ADR-010-sealed-cask-deployment.md` makes no mention of how adopters obtain a verification key. The README (top-level) does not document the trust root. **The compliance matrices (CM-14, SR-3, SR-11) cite Sealed Cask as a signed-component control. There is no signature, and no public key.** |
| **Out-of-band channel** | **Critical** | None defined. Adopters who receive a `cask.tar.gz.age` need the matching age private key to decrypt. **age requires the adopter to hold the same private key the builder holds** — which means key co-distribution, which means there is no trust boundary between builder and adopter. |
| **Trust-on-first-use** | n/a | Without a published key, even TOFU is impossible. There is nothing for an adopter to pin. |
| **Mirror confusion** | **High** | If adopter Alice and adopter Bob receive caskets from different mirrors, they cannot detect that they were given different artefacts. There is no transparent log (à la sigsum, sigstore Rekor) of (rune-hash, builder, timestamp) tuples that adopters could cross-check. |

### 3.5 The verifier (`verify-binding-rune.sh`)

| Threat | Severity | Notes |
|--------|----------|-------|
| **Bash-itself-trusted** | High | The verifier is a 114-line bash script. Its trust assumes: bash, tar, sha256sum, awk, grep, find, mkdir, rm — all on the adopter's `$PATH` — are uncompromised. A compromised host bash would defeat the verifier silently. **No bootstrapping option (e.g., a signed Go static binary) is provided.** |
| **`/tmp` race** | Medium | `VERIFY_DIR="/tmp/verify-cask-$$"` (`verify-binding-rune.sh:14`). PID-based naming has a small but real race window if `/tmp` is shared multi-tenant. Mitigated by exclusive `mkdir -p` semantics, but `mkdir -p` does not fail on existing directory — an attacker who pre-creates `/tmp/verify-cask-$$` (predictable PID range under `/proc/sys/kernel/pid_max`) can pre-stage files that the verifier then "validates." |
| **`set -e` discipline gaps** | Low | The script uses `set -euo pipefail` (`verify-binding-rune.sh:11`). However, the EXTRA detection block (`verify-binding-rune.sh:79-84`) pipes through `\|\| true` patterns implicitly via `grep -c` exit codes and `\|\| true` on line 84. False negatives possible under unusual filenames. |
| **Cleanup before verdict** | Medium | `rm -rf "${VERIFY_DIR}"` (`verify-binding-rune.sh:103`) happens **before** the verdict (`verify-binding-rune.sh:107`). On a failed verification, the operator can no longer inspect the modified files — forensics is destroyed by the verifier itself. |
| **No decryption step in verifier** | Medium | The verifier expects an unencrypted `cask.tar.gz`. The README in `build-sealed-cask.sh:143` tells the operator to decrypt manually with `age -d -i ${AGE_KEY} ${ENCRYPTED} > cask.tar.gz`. This means the adopter's first interaction with the artefact is decrypting with a private key they shouldn't have. (See §3.4.) |
| **Exit code 1 = adopter knows** | Low | The verdict text at `verify-binding-rune.sh:111-113` is clear and the exit code is correct. This is the one part of the chain that works as advertised. |

### 3.6 The cask archive in transit / at rest

| Threat | Severity | Notes |
|--------|----------|-------|
| **Confidentiality in transit** | Medium → Low (with age) | The `.age` envelope (X25519 + ChaCha20-Poly1305 AEAD) provides authenticated encryption. **If** the adopter holds the key, this is fine. The trust model fails on key distribution (§3.4), not on the crypto. |
| **Integrity in transit** | Medium | Without the age envelope (i.e., if `age` was not installed at build time — `build-sealed-cask.sh:109` makes age optional), the cask travels as a plain `.tar.gz` with no signature. A tarball MITM is undetectable. |
| **At-rest confidentiality** | Medium | `.age` encrypted at rest is fine if the key is not co-located. Today, the key IS co-located with the cask on the build host. Stolen disk image of the build host = both ciphertext and key. |
| **Replay (old-cask substitution)** | **High** | **No timestamp / version constraint inside the cask.** An attacker who serves adopter Alice the **2026-03 cask** (last-known-good), instead of the **2026-05 cask** (which patches a CVE), passes verification successfully. The Binding Rune doesn't include `built_at`, `git_commit`, `incarnation`, or any monotonic counter — those are spec'd in ADR-010:91 but not implemented. The adopter has no way to detect they were rolled back. |
| **Long-tail availability** | Low | A cask is a static tarball; standard CDN problem. Out of scope. |

### 3.8 Compliance-control mapping (for matrix amendment)

This table is the input MoatGhost needs to amend the matrix family (task F12). Each row maps a compliance control currently citing Sealed Cask to the threat-model section that constrains its actual operational state.

| Control | Framework | Current matrix rating | Realistic rating given §3-§4 | Driving threat |
|---|---|---|---|---|
| CM-2 | NIST 800-53 | MAPPED | MAPPED (the manifest exists; it just isn't signed) | n/a — the baseline-configuration claim is honest |
| CM-8 | NIST 800-53 | MAPPED | MAPPED (Doom Range registry is the actual control here, not the cask) | §3.7 |
| CM-14 | NIST 800-53 | MAPPED ("Signed Components") | **PARTIAL** — there is no signature | §3.3, §3.7 |
| SI-7 | NIST 800-53 | MAPPED ("Software, Firmware, Information Integrity") | **PARTIAL** — integrity check exists; authentication of the integrity check does not | §3.3, §3.4 |
| SR-3 | NIST 800-53 | MAPPED ("Supply Chain Controls") | **PARTIAL** — build host undefined; reproducibility not implemented | §3.1, §3.7 |
| SR-7 | NIST 800-53 | PARTIAL | PARTIAL (already honestly rated) | §3.1, §3.2 |
| SR-9 | NIST 800-53 | MAPPED ("Tamper Resistance & Detection") | **PARTIAL** — detection requires a published trusted hash; none exists | §3.4 |
| SR-11 | NIST 800-53 | MAPPED ("Component Authenticity") | **PARTIAL** — authenticity requires a signature; none exists | §3.3, §3.4 |
| 4.1 | PCI DSS | cited as MAPPED | **PARTIAL** — strong cryptography on transmission ≠ unsigned manifest | §3.3 |
| 11.5 | CIS Controls v8 | cited as MAPPED | **PARTIAL** — file-integrity *of artefacts* exists; *of the manifest itself* does not | §3.3 |
| A.8.19 | ISO/IEC 27001:2022 | cited as MAPPED | **PARTIAL** — installation of software on operational systems requires authorisation; signing is the conventional mechanism | §3.4 |
| SI.L2-3.14.1 | CMMC 2.0 | cited as MAPPED | **PARTIAL** — flaw remediation requires verifiable patches; an unsigned cask isn't verifiably from the vendor | §3.4 |
| PS.3 | NIST SP 800-218 SSDF | cited (SR-3 row) | **PARTIAL** — "verify integrity" satisfied; "authenticate integrity" not | §3.3 |

**Net effect on the public matrix push.** Six MAPPED ratings (CM-14, SI-7, SR-3, SR-9, SR-11, plus PCI 4.1 / CIS 11.5 / ISO A.8.19 / CMMC SI.L2-3.14.1 by transitive citation) should be downgraded to PARTIAL pending recommendations §6.3 (sign the rune) and §6.4 (publish the key). Once both land, a follow-up amendment can promote them back to MAPPED with this threat model as the operational evidence. **MoatGhost should not push the matrix family public with the current MAPPED claims** — it overstates the control's effectiveness and (per BM7) becomes a recon advantage for an attacker.

---

## 4. Attack chain walkthroughs

### 4.1 The darwin build-failure implication (BM7's actual finding)

**Setup.** Per BM7 and tonight's Phase A baseline, `go build ./...` fails on darwin because Linux-only eBPF code lacks `//go:build linux` tags. `build-sealed-cask.sh:37` runs `go build ./...` unconditionally and exits via `set -e` if the build fails. **Therefore the script cannot have been run on the operator's macOS workstation. Every successful Sealed Cask build to date has happened on a Linux host.**

**Attack.** The implication isn't "darwin can't build"; it's "the build host is undocumented, undefended, and singular." If the kingdom has only one Linux box capable of producing valid caskets (UNVERIFIED — likely WEST, possibly EAST, possibly an LXD container, possibly a personal VM), then:

1. **Reconnaissance** — an attacker reads the public ADR-010 + the compliance matrix, learns Sealed Cask is the trust root for SR-3/CM-14/SI-7, and learns there's no published build-host inventory. They infer (correctly) that the host is one of the bare-metal machines.
2. **Targeting** — `ssh govan@east` is documented as live (per CLAUDE.md, point-to-point link). EAST has an IP on the public Internet (UNVERIFIED) or is reachable via the operator's home network. Whichever is more exposed becomes the target.
3. **Pivot** — once on the build host, the attacker waits for the next legitimate `bash scripts/build-sealed-cask.sh` invocation. They can:
   - Backdoor the source tree under `${PROJECT_ROOT}` between builds.
   - Read `${PROJECT_ROOT}/secrets/cask.age.key` and exfiltrate it.
   - Pre-position a malicious binary that gets cp'd into staging at `build-sealed-cask.sh:55` or `:62`.
4. **Exit with the keys** — once they hold `cask.age.key`, they can produce signed-looking caskets indefinitely. Adopters cannot distinguish.
5. **No detection** — the operator's macOS workstation cannot rebuild and compare hashes (`go build ./...` fails), so the only sanity check ("does the rune match what I just built?") is unavailable on the operator's primary daily-driver machine. **The darwin build failure isn't a build-system bug; it's a missing verification path.**

**Cost of fix.** Add `//go:build linux` tags to the eBPF-touching packages so darwin compiles. Then the operator can independently rebuild and compare runes on macOS as a sanity check against build-host compromise. This is a **detective** control, not preventive — but it costs a half-day and removes single-host trust.

### 4.2 Build-host compromise → silent supply-chain replacement

1. Attacker compromises the (UNVERIFIED) build host via SSH credential reuse, web app on the host (e.g., does WEST run a public dashboard?), or local-network pivot.
2. They modify, e.g., `cmd/wotan/main.go` to add a backdoor.
3. The next `build-sealed-cask.sh` run hashes the backdoored binary honestly. The Binding Rune is "correct."
4. Adopters verify successfully and deploy the backdoor.
5. Detection requires: an out-of-band reproducible build (not implemented), a transparent log (not implemented), or an adopter who reads the diff (not realistic at 415K LOC).

**Mitigation that exists today:** none.
**Mitigation per ADR-010 spec:** Nix reproducible builds + Ed25519 signature on the rune (`docs/adr/ADR-010-sealed-cask-deployment.md:111-115` "Reproducible builds as a security primitive"). **Not implemented.**

### 4.3 Key exfiltration → forever-valid forged caskets

1. Attacker reads `${PROJECT_ROOT}/secrets/cask.age.key` (mode 600, but they're root or the same user).
2. They walk away with both private and public halves.
3. They build their own caskets on their own infrastructure, encrypted to the same recipient.
4. **There is no rotation, no revocation, no transparent log of "rune hashes the legitimate builder produced."** Adopters cannot tell forged caskets from legitimate ones, even retroactively.

**Mitigation that exists today:** the file-mode 600. That's it.
**Mitigation per spec:** key in HSM/Sophia (ADR-010:131). Not implemented.

### 4.4 Bash-toolchain compromise → verifier turns into a confirmation oracle

1. Adopter's host has a tampered `bash` (e.g., supply-chain attack on a distro mirror, or a compromised package manager).
2. Tampered bash detects the verifier script by content signature and unconditionally exits 0 with `VERDICT: BINDING RUNE INTACT`.
3. Adopter deploys whatever they were given.

**Mitigation that exists today:** none.
**Mitigation reasonable:** ship a statically-linked Go verifier (`cmd/verify-cask` doesn't exist today) that reads its own binary, embeds a known-good public key, and self-checks. This still doesn't defeat a host-kernel compromise but raises the bar substantially.

### 4.5 Replay / rollback to last-known-good cask

1. Adopter Alice routinely deploys the latest cask. The 2026-05 cask patches a Wotan auth bug.
2. Attacker MITMs Alice's cask download (or compromises the mirror) and serves the 2026-03 cask, which still has the bug.
3. Verification passes. The Binding Rune is intact (it's a real rune from a real cask).
4. Alice is rolled back to a vulnerable version with no detection signal.

**Mitigation that exists today:** none.
**Mitigation reasonable:** include `built_at` + `git_commit` + monotonic `incarnation` in the rune, signed; reject caskets whose `incarnation` is less than the adopter's current deployed value.

### 4.6 Tarball-traversal and pre-positioned `/tmp` race

A combined attack: an attacker who gains transient write access to a hosted cask (e.g., an HTTP mirror) can craft a tarball that, when extracted by `verify-binding-rune.sh:31` (`tar xzf "${CASK}" -C "${VERIFY_DIR}"`), writes files outside `${VERIFY_DIR}`. Modern GNU/BSD tar refuses absolute paths and `..` traversal by default but the verifier passes none of `--no-same-owner`, `--no-same-permissions`, `--no-overwrite-dir`. **UNVERIFIED** whether the adopter's tar implementation defaults to safe extraction — this varies by distro and version.

Combined with the `/tmp/verify-cask-$$` predictable-PID problem (§3.5), an attacker with local code execution on the adopter host can pre-stage `/tmp/verify-cask-NNN` directories matching forecasted PID ranges, and the verifier's `mkdir -p` (`verify-binding-rune.sh:30`) silently uses the pre-staged directory. The verifier then "verifies" attacker-controlled files. The race window is small but real.

**Mitigation:** use `mktemp -d` instead of `/tmp/verify-cask-$$`; pass `--no-same-owner --no-same-permissions` to tar; reject manifest entries containing `/`, `..`, or null bytes before re-hashing. (Listed in §6 rec #11.)

### 4.7 Cumulative-failure scenario (the realistic worst case)

The independent threats above compose into a single realistic attack:

1. **Recon** — attacker reads the public compliance matrix, ADR-010, this threat model, and the GitHub repo. They learn: build host is Linux, undocumented; key auto-generates on absence; verifier is bash; no signature; no transparency log.
2. **Initial access** — attacker pivots to EAST (or whichever bare-metal host runs the build) via the documented `ssh govan@east` path (per CLAUDE.md), exploiting either a credentialed attack on Stevie's macOS workstation (separate threat model F4) or a public-facing service vulnerability on the bare-metal host.
3. **Persistence** — attacker copies `${PROJECT_ROOT}/secrets/cask.age.key` to attacker-controlled storage. Even if the kingdom subsequently rotates the key (no rotation logic exists), the attacker holds the historical key forever. Old caskets remain forgeable retroactively.
4. **Trust-chain takeover** — attacker stops the active intrusion (no need to remain present). They can now build their own caskets at leisure on their own infrastructure, encrypted to the same age recipient, hashed by their own copy of the script.
5. **Distribution** — attacker waits for an adopter to request a cask. The kingdom has no out-of-band channel telling adopters which rune-hash is legitimate. The attacker's cask is indistinguishable from a kingdom cask.
6. **Persistence on adopter** — attacker's cask deploys backdoors into the adopter's substrate (Wotan, dashboard-backend, sophia, etc.). The adopter has no way to detect post-deployment because Mímir's drift detection is alerts-only against a baseline that was *itself* derived from the cask.
7. **No detection signal** — if the kingdom ever runs `build-sealed-cask.sh` again on the legitimate host, the resulting rune-hash differs from the attacker's. But there is no published log to compare against, so the discrepancy is invisible to the adopter. The kingdom would have to publish a "the legitimate 2026-05 rune-hash is X" notice on a separate channel — and have set up adopter expectations to check that channel — for the attack to be exposed. **No such channel or convention exists today.**

**Steps 3-7 cost the attacker nothing once step 2 succeeds.** The keystone is step 2: the build host's external attack surface. Recommendations §6.1, §6.10 directly address this; recommendations §6.3, §6.4, §6.8 break the chain at later steps so a single host compromise doesn't equal forever-trust-takeover.

### 4.8 The compliance-matrix-as-recon-doc problem (BM7's tail)

BM7 closes with: *"The matrix family is a comprehensive map of the kingdom's defensive posture. It is also, unintentionally, a comprehensive recon dossier for an attacker."* For Sealed Cask specifically, the public compliance matrix tells an attacker:

- Sealed Cask is MAPPED for CM-14, SI-7, SR-3, SR-9, SR-11.
- The verification mechanism is `verify-binding-rune.sh` (path published).
- Mímir's drift detection (alerts-only, per CLAUDE.md / ADR-043) cannot stop a tampered cask before deployment — it only alerts after.
- ML-DSA-65 signing is restricted to `config.*` Wotan topics (per Wotan Topic Signing notes); it does **not** cover the Sealed Cask trust chain.

So an attacker who reads the matrix learns: the chain is a SHA-256 manifest with optional age encryption, no asymmetric signature, and the verifier is a 114-line bash script. **The matrix is honest about MAPPED-vs-PARTIAL but doesn't disclose that "MAPPED" means "the script exists" rather than "the control is operational."** This document corrects that record for Sealed Cask.

---

### 3.7 ADR-010 specification vs. implementation drift

The single most important finding of this audit is that **ADR-010 specifies a substantially stronger trust chain than `build-sealed-cask.sh` implements.** The compliance matrix maps to ADR-010 as written; the binaries in flight reflect the script as written. Side-by-side:

| Field / behaviour | ADR-010 specification | `build-sealed-cask.sh` reality | Severity of gap |
|---|---|---|---|
| Manifest format | JSON object with named fields (ADR-010:85-97) | Flat `sha256sum` text manifest (line 92) | **High** |
| `origin` field | `"sword-pipeline-prod"` (named build pipeline) | absent | High |
| `build_hash` | SHA-256 of canonical image | implicit, not captured | Medium |
| `nix_hash` | `/nix/store/<hash>-image` (deterministic, reproducible) | absent — Nix not invoked by script | **Critical** (ADR-010:111-115 calls reproducibility "a security primitive") |
| `signed_by` | `"deploy-key-2026-02"` | absent | **Critical** |
| `built_at` | ISO-8601 timestamp | absent (only in tarball filename) | High |
| `git_commit` | short SHA | absent | High |
| `parish` | `"PHYLACTERY_STORAGE"` etc. (parish boundary tie-in per ADR-009) | absent | Medium |
| `incarnation` | monotonic key-epoch counter | absent | High |
| `seal` | Ed25519 signature over above fields | absent — no signature step at all | **Critical** |
| Verification at "every layer" | Shield, Hauberk, Sophia, Anamnesis, Gauntlets each check the rune (ADR-010:101-110) | only one verifier (`verify-binding-rune.sh`) and it doesn't check signatures | **Critical** |
| Key location | "deploy signing key, which lives in HSM/Sophia, never on any node" (ADR-010:131) | `${PROJECT_ROOT}/secrets/cask.age.key`, mode 600, on the build node's filesystem | **Critical** |
| Two-volume model | LUKS Bone Shell + LUKS Soul Chamber, separate keys (ADR-010:137-157) | not implemented; ADR-010:9-10 explicitly defers to Beta | (deferred — accepted) |
| Blue/green Reanimation | rolling vessel replacement with `incarnation` advance (ADR-010:160-171) | not implemented | (deferred — accepted) |
| Anamnesis events | `EVENT_VESSEL_ANIMATE`, `EVENT_RUNE_FORGED_FALSE`, etc. (ADR-010:174-182) | no event emission from script | High (no audit trail) |

**Compliance impact.** The matrices currently rate Sealed Cask MAPPED for CM-14 ("Signed Components"), SR-3 ("Supply Chain Controls"), SR-9 ("Tamper Resistance"), SR-11 ("Component Authenticity"). All four of these controls assume the cryptographic signing the ADR specifies. **None of them is satisfied by the implementation.** They should read PARTIAL until §6 recommendations #3-#5 land.

---

## 5. Threat-vector summary

The seven highest-severity items, in priority order for BlackMage's pen-test attention and the next sprint's hardening backlog:

1. **The build host is undocumented, unhardened, and singular.** Compromise = full supply-chain takeover. (See §3.1, §4.1, §4.2.) No CI-pinned reproducible builder exists.
2. **The age key has no rotation, no backup, no HSM, and is co-located with the build artefacts.** Auto-generation on missing-key means a compromised host silently issues a "new legitimate" key. (See §3.2, §4.3.)
3. **The Binding Rune is unsigned.** SHA-256 with no asymmetric signature. ADR-010 spec'd Ed25519; implementation has none. (See §3.3, §3.7.)
4. **Public-key distribution to adopters is not defined.** No published key, no transparent log, no out-of-band trust path. age requires the adopter to hold the private key, collapsing the trust boundary. (See §3.4.)
5. **Replay / rollback is undetectable.** No `built_at`, no `git_commit`, no `incarnation` field in the actual rune (despite ADR-010:91). (See §3.6, §4.5.)
6. **The verifier is bash-trusted-end-to-end.** No statically-linked verification binary. (See §3.5, §4.4.)
7. **The darwin build failure is a missing detective control.** Operator cannot rebuild on macOS to compare runes against the (single) Linux build host. (See §4.1.)

---

## 6. Recommendations (prioritised)

1. **Document the build host. Today.** Even before hardening — write into `docs/adr/ADR-010-sealed-cask-deployment.md` which Linux machine produces caskets, what hardening it has, who has SSH to it. The undocumented state is itself the worst finding. **Owner:** Architect.
2. **Add `//go:build linux` tags to the eBPF-touching Go packages** so darwin compiles. This restores the detective control of "operator independently rebuilds on macOS and compares the rune." Estimate: half a day. **Owner:** Developer.
3. **Sign the Binding Rune.** Replace the bare `sha256sum > BINDING_RUNE.sha256` step at `build-sealed-cask.sh:92` with: (a) compute manifest, (b) sign manifest with **ML-DSA-65** (already in-tree via Wotan signing in `services/wotan/internal/signing/`) producing `BINDING_RUNE.sha256.sig`, (c) include both in the cask. Update `verify-binding-rune.sh` to verify the signature against a pinned public key before checking hashes. ML-DSA-65 is the kingdom's existing PQC choice — reuse it. **Owner:** Architect + Developer.
4. **Publish the verification public key.** Commit it to the repo at `keys/binding-rune.pub`, document the SHA-256 fingerprint in the README, and re-publish the fingerprint on a second channel (Matrix/IRC announce, Mastodon post, signed git tag). Adopters pin the fingerprint, not the key file. **Owner:** Architect + MoatGhost.
5. **Separate the encryption key from the signing key.** age remains for confidentiality (encrypt to **adopter's** public key, not the builder's), ML-DSA-65 for authenticity. Adopters publish their age recipient pubkeys; the builder encrypts to those. The signing key never leaves the build host. **Owner:** Architect.
6. **Embed `built_at`, `git_commit`, `incarnation`, `host_fingerprint` in the rune** (ADR-010:91 already specifies these). Reject caskets whose `incarnation` rolls backward. **Owner:** Developer.
7. **Replace `verify-binding-rune.sh` with a Go binary** at `cmd/verify-cask/`. Static, embeds the trusted public key, self-attests its own SHA-256. Ship the bash script as a fallback only. **Owner:** Developer.
8. **Add a transparent log.** Even a simple append-only file at `keys/rune-log.txt` (containing `<timestamp> <git_commit> <rune-sha256>` entries, signed daily) lets adopters cross-check what the kingdom claims to have built. Sigsum / Rekor are the maximalist options; a signed git-committed log is the minimalist one. **Owner:** Architect.
9. **Move the signing key off the filesystem.** Even without an HSM, a YubiKey or `age-plugin-yubikey` would prevent disk-image exfiltration. Long-term, ADR-010:131's "HSM/Sophia" target. **Owner:** Architect.
10. **Wire `build-sealed-cask.sh` into a CI workflow** in `.github/workflows/sealed-cask.yml`, even if it only runs on `release` events. The CI run produces a public log of "this cask was built from this commit at this time," which adopters can correlate against the rune log (#8). The CI workflow also forces a documented build host. **Owner:** Architect.
11. **Tarball-traversal hardening** in `verify-binding-rune.sh`: pass `--no-same-owner --no-same-permissions` to `tar xzf` (`verify-binding-rune.sh:31`); reject manifest entries containing `..`, absolute paths, or symlink-target traversal patterns before re-hashing. **Owner:** Developer.
12. **Preserve forensic evidence on verification failure.** Move the `rm -rf "${VERIFY_DIR}"` cleanup at `verify-binding-rune.sh:103` to occur only when `${FAIL} -eq 0 && ${MISSING} -eq 0 && ${EXTRA_COUNT} -eq 0`. On failure, leave the directory + a README explaining what to do with it. **Owner:** Developer.
13. **Update the compliance matrix to reflect implementation reality.** CM-14, SR-3, SR-11 should be downgraded from MAPPED to PARTIAL until the rune is signed (#3) and the public key is published (#4). The current MAPPED rating is aspirational. **Owner:** MoatGhost.

---

### 6.1 Sequencing — what to land first

The 13 recommendations above span doc-only work, code work, ops work, and policy work. A reasonable sequencing for a small team:

**Within one week (doc + code, low-risk):**
- Rec #1 (document the build host) — pure doc, zero code change.
- Rec #2 (`//go:build linux` tags) — half-day code change; restores macOS rebuild as a detective control.
- Rec #11 (tarball-traversal hardening) — small bash change.
- Rec #12 (preserve forensic evidence on failure) — small bash change.
- Rec #13 (downgrade compliance matrix entries) — pure doc, must land before public matrix push.

**Within one sprint (code + key management):**
- Rec #3 (sign the rune with ML-DSA-65) — reuses `services/wotan/internal/signing/`. Estimate: 2-3 days, including verifier work.
- Rec #4 (publish the verification key) — couple of hours once #3 is in.
- Rec #6 (embed `built_at`, `git_commit`, `incarnation`) — 1 day; piggyback onto #3 since the manifest format changes anyway.
- Rec #7 (Go verifier binary) — 2 days, once #3 settles.

**Within one quarter (architecture):**
- Rec #5 (separate encryption from signing keys) — requires adopter-side onboarding flow design.
- Rec #8 (transparent log) — protocol design, even minimalist version touches multiple files.
- Rec #10 (CI workflow for cask builds) — requires deciding what to do with secrets in GitHub Actions vs. self-hosted runner.

**Long-term (architecture + ops):**
- Rec #9 (HSM / YubiKey for signing key) — depends on operator hardware acquisition.

### 6.2 Tie-ins to existing kingdom machinery

The kingdom already has cryptographic primitives that the Sealed Cask chain ignores. Reuse over reinvention:

- **ML-DSA-65 signing** is implemented in `services/wotan/internal/signing/` for `config.*` topics (per CLAUDE.md "Wotan Topic Signing"). Recommendation #3 should reuse the same key infrastructure pattern (key files, library, test harness) rather than introducing Ed25519 in parallel.
- **Mímir** (`cmd/heimdall-daemon/`, `pkg/enkrateia/`) does drift detection against a baseline. Caskets could publish their `BINDING_RUNE.sha256` hash to a Wotan topic on build, so Mímir could detect "running cask doesn't match any known-good rune-hash" — a missing detective control today.
- **Anamnesis events** specified at ADR-010:174-182 (`EVENT_VESSEL_ANIMATE`, `EVENT_RUNE_FORGED_FALSE`, `EVENT_VESSEL_CORRUPTED`, `EVENT_VESSEL_BANISHED`, `EVENT_REANIMATION`, `EVENT_VESSEL_UNRAVELING`) have event codes (`0x50`-`0x55`) but no emission path. Recommendation #3's signed-rune flow should emit these on build / verify outcomes so the audit trail is real.
- **The Doom Range port registry** (`pkg/ports/ports.go`) is the kingdom's CM-8 source of truth. Sealed Cask manifests should optionally include the registry's content-hash as a cross-check that the cask matches a known port topology. Cheap insurance.
- **SBOM at `docs/sbom/`** is the SR-3 supply chain inventory. Sealed Cask manifests should optionally embed the SBOM hash so an adopter verifying the cask gets the SBOM bound to the binaries.

These tie-ins are not blockers; they're cheap ways to compose Sealed Cask with controls the kingdom already has, raising the rating of CM-14 / SR-3 / SR-9 / SR-11 from PARTIAL back toward MAPPED with operational reality behind it.

---

## 7. Hand-off

This document defines the surface; pen-test refines specific axes:

- **BlackMage**: walk attack chains §4.1-§4.5. Specifically, prove or disprove the assumed singular build host. If multiple Linux boxes have built caskets historically, that's a positive finding; if only one has, that's the keystone.
- **MoatGhost**: apply recommendation #13 to the matrix family (`docs/compliance/control-matrix/`). Cross-reference NIST SP 800-218 SSDF practices PS.1 (protect code), PS.2 (verified components), PS.3 (verify integrity) — Sealed Cask's current implementation hits PS.3 only weakly.
- **Architect**: own recommendations #1, #3, #5, #8, #9, #10. Recommendation #1 is doc-only and should land before the public-launch matrix push.
- **Developer**: own recommendations #2, #6, #7, #11, #12.

Once #3 (sign the rune) and #4 (publish the key) land, this document should be re-issued with severities downgraded and the new asymmetric trust chain diagrammed.

---

## 8. Provenance

Read-only audit; no script execution, no remote calls. Sources:

- `scripts/build-sealed-cask.sh` (entire, lines 1-145)
- `scripts/verify-binding-rune.sh` (entire, lines 1-114)
- `docs/adr/ADR-010-sealed-cask-deployment.md` (entire, lines 1-219)
- `docs/security/k8s-threat-model-2026-05-06.md` (referenced for style/structure parity)
- `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` (BM7 finding, lines 271, 277, 297-306, 331, 335)
- `docs/compliance/control-matrix/nist-800-53-2026-05-06.md` (Sealed Cask MAPPED rows: CM-2, CM-8, CM-14, SI-7, SR-3, SR-7, SR-9, SR-11)
- `.github/workflows/release.yml`, `.github/workflows/docker.yml` (confirmed: no Sealed Cask invocation in CI; release workflow uses `ubuntu-latest` runners and signs nothing — `# TODO: Integrate sigstore/cosign for artifact signing` at `release.yml:97-100`)
- `CLAUDE.md` (Sealed Cask cited as "deterministic image builder with SHA256 integrity"; bare-metal posture WEST + EAST)

Items marked **UNVERIFIED** in this document are: the identity of the Linux build host, the present-day location of `cask.age.key`, whether any cask has ever been distributed to a third-party adopter, and whether the operator's `secrets/cask.age.key` has any backup. No live filesystem outside `/Users/govan/tmp/unheaded/` was inspected. Findings in §3.2, §3.4 reflect what the implementation **says** about itself, not what an asset inventory would confirm.
