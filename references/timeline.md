# The Unheaded Chronicles

## A Living Grimoire of the Kingdom's Journey

**STATUS:** Age 3 IN PROGRESS — Public Release Sprint, ASCEND-LINUX (Dream Ladder L5 → L6) actively shipping
**LAST UPDATED:** 2026-07-02
**HEAD:** ASCEND-LINUX **Phase 1 COMPLETE** (Gates A/B/C/D + D.1, 2026-06-18 → 2026-07-02) — interactive xv6 sh on the UPC with the full SHELL+5CMDS corpus (init/sh/ls/cat/echo/wc) over a real in-BPF FS reader (ADR-078), exec argv, multi-byte write. `ls` lists the real root dir; `cat README` streams a real inode; `wc README` → `5 49 283 README`. Next: Phase 2 (L6a uClinux) pending two Stevie pair calls (LTS pick; arch/mbc cargo-cult base)
**Drift policy:** ADR-052 — this file MUST be ≤ 7 days from HEAD when HEAD has new commits.

---

### Age 0: The Foundation Stone (✅ COMPLETED)
*Pre-alpha. Crew assembled. Lore established. Protocol vocabulary frozen.*

### Age 1: The Alpha Ascension (✅ COMPLETED)
**Progress:** 100%
*All 10 core services operational. Wotan message bus shipping. eBPF traces end-to-end.*

### Age 2: The Beta Trials (✅ COMPLETED)
**Progress:** 100%
*S36 Four Pillars complete (Port Authority, gRPC-First Transport, Log Aggregation, Service Discovery). S51 Auth Framework. S67 Wire Format Freeze. WEST + EAST bare metal online. Cross-host BPF flow graph operational.*

### Age 3: The Public Release (🔄 IN PROGRESS)
**Progress:** ~70%
**Status:** Public-launch thread stalled mid-flight; multi-wave forge research thread shipping at quality.

**Completed sub-items:**
- S67 Wire Format FROZEN at v0x01 (12 IANA registries, IPR clear)
- S78 LOC audit (415K production, 1,137K total)
- S-WEST + S-EAST bootstrap (both bare-metal hosts online)
- ADR Sweep 2026-04-03 (all 27 ADRs resolved)
- Zhen Champion (pkg/champion/, 18 tests)
- Zhen MCP Server (7 tools for Claude Code)
- 31 Operational Runbooks
- Service Template (pkg/service/, 7 tests)
- BPF CI Gate, Sealed Cask, Binding Rune verification
- ROCm GPU acceleration (PyTorch 2.5.1+rocm6.2)
- RAFT QA generation (~2000 pairs)
- WAVE10C Backprop fixes (3 bugs, 46/46 tests pass)
- Wotan Topic Signing (ML-DSA-65 enforcement)
- Mímir's Law / Gleipnir Phase 0 (2026-04-11, real-metal validated on EAST: zero false positives, 100% drift detection)
- WAVE10F Forge Real-Attention Gemma 4 (2026-04-17, 3000+ LOC, end-to-end Gemma-4 LoRA training)
- Learning Gate (2026-04-20, 4/5 strict experiments PASS, β = 0.27 generalization exponent)
- 24h Consolidation Block (2026-04-20, lr=1e-3 stable operating point identified)
- ADR-048 ForgeBackend Trait (2026-04-21, refactor centralizes Adam optimizer)
- WAVE11 GPU Kernels (4 attention grad kernels, cosine=1.000)
- WAVE12 Kingdom RAFT LoRA (2026-04-23, eval Δ −14.32, ADR-050 GPU-resident activations)
- WAVE13 Phase 1 (2026-04-25, generate-gemma4 subcommand, early quality finding: LoRA under-trained ~10× off target)
- Round Table verification audit (2026-04-27, 19 seats reported, 2 citations issued + cleared)
- WAVE13 Phase 2 quality verdict: **RETRAIN** (2026-04-28, autonomous overnight per Marshal charter; 0/8 LoRA-better, 6/8 immediate-stop, 2/8 mode-collapse `\tif`; ADR-051 Accepted; Phases 4-5 PAUSED until WAVE14 retrain)
- WAVE16 Multi-model selector + overnight model bench (2026-05-04, ADR-060 LIVE in sidebar; 5 model keys; qwen-coder-14b kept as option after passing 14-prompt textbook tier 0/14 truncated)
- WAVE17 K8s substrate proven (2026-05-05, autonomous overnight per Marshal charter; 9/9 services Running on 3-node kind; 2 real bugs found+fixed mid-run: monad service-link env collision + chart volume support; ADR-064 active/active spec landed but impl deferred per Stevie; 9-file doctrine sweep post-c6108fb8; cmd/tools/ scaffold for round-table P0 wedges; wotan main.go review +13 unit tests)
- NORTH-STAR Appendix A Phases A-F (2026-05-06, autonomous overnight per Marshal charter; 9 commits; SBOM delta, K8s threat model, CIS scope, RBAC review, 15 framework control-coverage matrices, 12-track scrutiny remediation; Sophia + Wotan draft-04 ship-or-defer reports)
- Marshal continuation shift 2026-05-07 (autonomous daytime; 9 commits; ADR-065 aya 0.1.x→0.13.x migration plan, kingdom-wide gofmt cleanup 435 files, pre-commit hook installed Go+Rust+SPDX, cargo audit sweep landed 3 CVE-class fixes via rustls-webpki bump in zhend, SPDX coverage 99.50% → 100.00%)
- Marshal drain shift 2026-05-08 (autonomous daytime; 22 commits; ADR-065 Phase A finding REVERSES the migration — aya splits userspace/kernel independently, no migration needed; ADR-066 tonic 0.10→0.12 in trace-collector closes 4 CVEs; pqcrypto Wave B FIPS 205/203 migration — pqcrypto-{kyber,dilithium}→pqcrypto-{mlkem,mldsa} aligns Rust side with Kingdom's existing Go-side ML-DSA-65; sync.Once + DoomState mutex copy vet fixes; 5 missing S77 deliverable docs authored)
- ASCEND-LINUX battle plan ratified (2026-05-08, references/battle-plan-ascend-linux-2026-05-08.md; 10-month phased campaign Phase 0 → Phase 4 IPv6+SSH; quarterly gates with explicit cut-points; xv6-riscv as L5 halfway proof, uClinux as L6a, full Linux+MMU as L6b; three demo surfaces — A browser xterm, B host pty, C SSH over IPv6)
- ASCEND-LINUX Phase 0 complete (2026-05-08; ADR-067 7-decision ABI + ISA spec; 5 new MBC opcodes FENCE/MRET/SRET/LR.W/SC.W shipped; priv_level field added to MbcCpuState; Boot Protocol v2 spec; eBPF interpreter implementation; verifier-budget revalidation; 15 commits across kickoff)
- ASCEND-LINUX Phase 1.1 super-sprint (2026-05-09, autonomous; 12 commits across 12 build iterations; xv6-mbc adapters: start_mbc.c, console-mmio.c, blk-ramdisk.c, syscall_shims.S, Makefile.mbc + linker script; kernel.elf links via RV32 conversion; **xv6-mbc.mbc EMITS** — 11,721 MBC instructions, 46 KB, first kernel image translated end-to-end; cmd/upc-bootctl boot dispatcher with validate + dry-run; cmd/upc-tty-bridge WebSocket on UPC user-app port 26100 + dashboard/upc-console.html xterm.js client; translator gains CSR + MRET/SRET/WFI/SFENCE.VMA + register-aliasing x18-x31→r3-r13 best-effort)
- ASCEND-LINUX **Phase 1 COMPLETE** (2026-05-13 → 2026-07-02; Phases 1.2–1.6 + Phase 1.7 Gates A/B/C/D/D.1): per-pid pgd isolation (ADR-074), process model (ADR-075), `init: starting sh` from user mode (2026-05-14), console stdio Gate A (2026-06-18), exec→`$` Gate B with per-process rv2mbc_base (ADR-077, 2026-06-19), interactive sh fork→exec→wait→prompt Gate C (2026-06-26), FS-reader Gate D (2026-07-02, ADR-078: in-BPF inode walks over fs.img, ilp32e struct-stat fix, exec argv frame @ VA 0x600000, multi-byte write, 64-byte KBD ring), Gate D.1 same-day (RET MBC-vs-RV floor 0x10000→0x20000 + loader guard — full SHELL+5CMDS corpus live: init/sh/ls/cat/echo/wc). Session logs in references/phase1*-*.md; Doom PC-corruption hypothesis parked for a Marshal-supervised doom-runner shift
- Marshal continuation shift 2026-05-09 (autonomous; 4 commits; cleanup stray upc-tty-bridge binary + .gitignore guard; **monad-mbc 65-day screen-test regression closed** — SCREEN_BASE moved 0xC000→0x70000 on 2026-03-03 c7831cad but 3 test fixtures never updated; upc-tty-bridge unreachable code closed; 7 of 12 carry-forward parking-lot entries from 2026-05-07 RESOLVED across drain + this shift; cargo audit zhend 0 vulns, trace-collector 0 vulns, only 4 unmaintained warnings remain across the whole Rust tree)
- Marshal extended-churn shift 2026-05-10/11 (autonomous, ~95 commits, Stevie-authorized 12hr churn): **ZERO LINT achieved across the kingdom — 2362 → 0** (errcheck/gosec/govet/staticcheck/unused/bodyclose all green via per-finding triage). **12 real CVE-class security bug fixes shipped** (RSA assertion crash ×2 sites in cert rotation, ECDSA assertion crash ×4 sites in CA loaders + mTLS, OCI tar Zip Slip in container runtime whiteout, HTTP static-file path-existence info-disclosure oracle, object storage path traversal via unvalidated key, audit `tableName` SQL-injection vector closure, Akira Slowloris HTTP timeouts, container image decompression-bomb cap at 5 GiB/file, Splunk exporter TLS NPE, mTLS Signer assertion crash). **ADR-073** records the lint-policy "zero findings as the new floor" discipline. **Yggdrasil OS-FORK-DISCIPLINE Pillar 5** added (`docs/OS-FORK-DISCIPLINE.md` + `nix/yggdrasil/overlay/upc/` quilt patches + `nix/yggdrasil/overlay/systemd/upc-tty-bridge.service` + `nix/yggdrasil/bin/yggdrasil-doctor-upc` 8-check preflight) for **task #71 UPC-on-Yggdrasil scaffolding**. 242/242 packages still passing tests. Shift reports at `references/marshal-shift-2026-05-11-{zero-prompt-12hr,extended-churn,final-checkpoint}.md`.
- Round-table + Yggdrasil iteration shift 2026-05-11 (~25 commits, post-Stevie wake): **4 NORTH-STAR blockers cleared** (Captain Track A/B/C → Track C hybrid + kingdom-state correction that repo's been public for over a month; Sophia draft-04 + Wotan draft-04 DEFER ratified; branch hygiene 3 stale locals deleted; Phase 1.2 pair-call HOLD per Stevie). **ADR-074** drafted (Phase 1.2 page-table mapping model, 3 options enumerated, Architect addendum surfaces 3 structural issues + 6 pre-work items). **Phase 1.2 pre-work battle plan** forged (47 steps, HOLD pending pair-call). **Yggdrasil tasks #65/#66/#67 scaffolds** advanced: full packer flow (preseed + 6 provisioners + CIS L1 + lynis gate + reproducibility clean), Jenkinsfile + GHA workflow, apt repo + smoke harness, cloud-image template (AWS/GCP/Azure), SELinux research scaffold. **`make health`** unified gate runner landed (8 sections, 11/11 gates at landing). **MARSHAL-COOKBOOK.md** distills 11 patterns from the arc. **Canonical next-session pickup doc** at `references/next-session-pickup-2026-05-11.md`. Shift report at `references/marshal-shift-2026-05-11-yggdrasil-iteration.md`.

**Remaining for Age 3:**
- Captain Track A/B/C decision (Wed 2026-04-29)
- WAVE14 retrain (extended-epoch, rank=16/alpha=32 unchanged, ≥3 epochs ≈ 10704 example-steps; gated on Captain sign-off)
- Branch hygiene execution (3 stale branches, REMOTE for build/test)
- SBOM regen + license scan + threat refresh (REMOTE)
- Sophia draft-03 ship-or-defer
- Wotan draft-03 ship-or-defer
- Demo video + README polish for VC/public readiness
- Sub-50ms latency benchmark (Scientist falsifiability gate)
- Public accessibility (optional auth)

### Age 4: The MVP Era (📋 PLANNED)
*Track-call dependent. Activates after Age 3 public-launch gate.*
- WAVE14 BackwardScratch + KV-cache (gated on Track A or C)
- Performance benchmarking suite
- Customer onboarding flows
- Billing/metering
- Multi-tenant isolation hardening

### Age 5: The Scaling Era (📋 PLANNED)
*Long-horizon. Kingdom Mode rollout, multi-region, federation.*

---

*Synced: 2026-05-11 (Round-table + Yggdrasil iteration shift — 4 blockers cleared, Captain Track C called, ADR-074 + Phase 1.2 pre-work plan, Yggdrasil #65/#66/#67 scaffolds, make health, MARSHAL-COOKBOOK)*
*Next sync trigger: any commit to main + drift-guard CI check*
