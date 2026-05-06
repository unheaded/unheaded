# Operator macOS Workstation Threat Model — The Kingdom's Keystone

**Date:** 2026-05-06
**Author:** Marshal, NORTH-STAR Appendix A — Phase F4 (post-scrutiny remediation)
**Triggering finding:** `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` §SEN4 + §BM5 — *"the matrix family never enumerates the operator-laptop as a control surface … per Sentinel SEN4, no threat model exists for it."*
**Scope subject:** the single physical macOS device used by Stevie Bellis to develop, sign, deploy, and publish Unheaded. Per CLAUDE.md the device is a `Stevies-MacBook-Air` running **Darwin 25.4.0 / arm64**.
**Status:** initial draft — doc-only. No live forensic scan performed. Many controls below are marked **UNVERIFIED** because ground truth is reachable only from the device itself, which this Marshal session does not have access to.

> "The kingdom is operated by a single human. The kingdom's keys live on his laptop. Any threat that ends `… and now the attacker has the laptop` ends `… and now the attacker has the kingdom`."

---

## 1. Why this document exists

Three of the kingdom's authoritative artefacts agree the workstation is the highest-leverage attack surface in the project:

- **`01-scrutiny-2026-05-06.md` §BM5 — "The macOS dev box is the keystone."** Enumerates the privileges concentrated on the device and notes the `single-operator GAP — document-accepted-gap` framing in matrices is *"dangerously casual"*.
- **`01-scrutiny-2026-05-06.md` §SEN4** — explicitly demands a *"fourth threat model for the macOS dev workstation as the single-operator's privilege concentration point."*
- **`docs/security/k8s-threat-model-2026-05-06.md`** — covers only the kind cluster and explicitly excludes (as out-of-scope) the workstation that develops it. That model's threats `S—Spoofing`, `T—Tampering`, `E—Escalation` all assume the operator's identity is uncompromised. **This model questions that assumption.**

The K8s threat model (`k8s-threat-model-2026-05-06.md`) is the structural template followed here.

---

## 2. Trust concentration — what compromise of the laptop unlocks

This section is a deliberate, exhaustive list. It is uncomfortable to read; it is meant to be.

### 2.1 Cryptographic signing roots (catastrophic on compromise)

| Asset | Source-of-truth reference | What unlocks it | Blast radius |
|-------|---------------------------|-----------------|--------------|
| **ML-DSA-65 signing keys** for `config.*` Wotan topics | ADR-043 §"Eight Non-Negotiable Hard Conditions" #4: *"ML-DSA-65 signing key for baseline lives in HSM-equivalent storage (TPM, sealed, or filesystem with strict ACL + audit log)"* | Filesystem read of `~/.config/unheaded/keys/` or wherever the dev key actually lives — **UNVERIFIED location** | Forge any baseline message any Heimdall daemon will accept. Plant fake `config.*` on Wotan, plant fake "drift cleared" events, push a malicious Mjölnir manifest into a Reanimation event. Per ADR-043 the spike used *"DOCUMENTED (dev keys for spike)"* — the production HSM-grade requirement is hard-condition #4 but `01-scrutiny`'s `IA-7` / `SC-12` entries flag this as PARAM-GAP. |
| **Sealed Cask SHA-256 signing chain** (Binding Rune Ed25519) | ADR-010 §"The Binding Rune": `signed_by: deploy-key-2026-02` and §"Why No Compilers on Nodes": *"the deploy signing key, which lives in HSM/Sophia, never on any node."* In practice **build hosts and key custody on darwin are UNVERIFIED**. Per `01-scrutiny` §BM7: *"The signing key for Sealed Cask is held… where? On the build host? On Stevie's laptop? In CI secrets? This is undocumented."* | Filesystem read of the deploy key | Forge a Soul Vessel that any Gauntlets node will animate. End-to-end supply chain compromise of every kingdom node that pulls and verifies a cask. |
| **Git commit signing** (GPG/SSH key for signed commits) | UNVERIFIED — CLAUDE.md identity section names `stevie@bellis.tech` as the commit identity but does not commit to a signing posture. `1Password`-style hardware backing is not asserted. | macOS Keychain / GPG keyring on disk | Sign malicious commits as Stevie. Bypass any branch-protection-by-signed-commit rule on `main`. |
| **SSH private keys for `govan@east` and `govan@west`** | CLAUDE.md §"Operational Notes": *"P2P link: EAST is reachable as `govan@east` via direct point-to-point connection (192.168.13.1 ↔ 192.168.13.2)"* | `~/.ssh/id_*`, ssh-agent on the laptop | Direct shell on both bare-metal hosts. From `govan@east` an attacker can disturb every kingdom service running there; with `sudo` (CLAUDE.md confirms sudo is available on the dev machine and presumed on bare metal) they get root. |
| **GitHub web session cookie** (no `gh` CLI per CLAUDE.md) | CLAUDE.md identity section: *"No `gh` CLI: Pure `git` only. User creates repos via GitHub web UI"* | Browser cookie jar (Chrome/Safari/Firefox profile) | All GitHub repo write access. Push to `main`, change branch protection, add deploy keys, transfer ownership, publish releases. **This is the publication channel for the public-release authority cited in BM5.** |
| **age private key** for SOPS repo secrets | CLAUDE.md §"Secrets Management": *"Use SOPS + age for encrypted secrets."* The `age` key location is **UNVERIFIED** — typically `~/.config/sops/age/keys.txt` on macOS. Per `01-scrutiny` §S8 + §BM-various, the SOPS key custody is undocumented. | Filesystem read | Decrypt every encrypted secret in the repo. |

### 2.2 Identity and write access (critical on compromise)

| Asset | Where it lives | Compromise consequence |
|-------|----------------|------------------------|
| GitHub identity `bellistech` (CLAUDE.md) | Browser session cookies + 2FA seed (UNVERIFIED if hardware-backed) | Push to main on `bellistech/*` and `unheaded/*` orgs; modify org membership |
| Wiki write authority | Same GitHub session | Per `01-scrutiny` §BM9: *"the wiki ADR scaffold sweep created 65 new attack-surface signposts."* An attacker who pushes to the wiki can also rewrite history of those signposts to mislead future investigators. |
| Public-release authority (e.g. tagging release versions, publishing the `community-first` doctrine) | Same GitHub session + signing keys above | Publish a malicious tagged release the community trusts because it carries the kingdom's signature chain. |
| Email account `stevie@bellis.tech` | UNVERIFIED — likely Gmail / Fastmail / iCloud — recovers GitHub 2FA fallback | Reset of GitHub MFA / SSH key fingerprint — game-over recovery vector if MFA is email-recoverable. |

### 2.3 Operational privilege (high on compromise)

| Asset | Where it lives | Compromise consequence |
|-------|----------------|------------------------|
| `sudo` on the macOS device | Local user password | Keychain dump (`/usr/bin/security` access), TCC bypass, Full Disk Access grant, kernel extension load |
| Docker daemon socket on the Mac (CLAUDE.md says `sudo docker compose` is required) | UNIX socket | Container escape into host — well-documented Docker daemon = root threat model |
| Browser-stored credentials (any non-GitHub site reused) | Keychain / browser password manager | Pivot from any one stolen creds to identity-fed kingdom services |
| Local clones of every kingdom repo | `~/tmp/unheaded` per env | Read every secret SOPS doesn't cover; read every source-code-level vulnerability before disclosure; tamper without push by replanting on remote later |
| Claude Code / Anthropic API key (this very session) | `~/.claude/` | Drive an LLM agent at kingdom expense; exfiltrate via Claude conversations; tampered conversation history |

### 2.4 Soft assets (medium on compromise)

| Asset | Why it matters |
|-------|----------------|
| Browsing history, screenshots, paste buffer history | Operator-error surface (see §3.8) — secrets often appear in screenshots and pasted-into-LLM prompts |
| iCloud / Time Machine backup credentials | If backups are unencrypted-on-icloud, the keys above are *also* in the backup |
| Slack / Discord / Signal accounts | Pretexting + lateral phishing of any kingdom contributors / reviewers |
| Email archive | Decade of context for next-stage social engineering |

**Summary of the keystone problem:** by `01-scrutiny` §BM5's count and the enumeration above, **at minimum 6 cryptographic signing roots, 4 host-shell credentials, and 3 publication channels** sit on one device with no enumerated threat model in the matrix family until tonight.

---

## 3. STRIDE per attack vector

Each subsection below covers one of the 8 attack vectors the F4 task specified.

### 3.1 Phishing of the operator

| Sub-vector | STRIDE | Severity | Notes |
|------------|--------|----------|-------|
| Spear-phishing email with credential-capture page (fake GitHub login) | **S** (spoofing) + **E** (escalation if creds reused) | **High** | Per `01-scrutiny` §BM5 + the matrix `IA-2(1)` UNVERIFIED entry, MFA on GitHub is **UNVERIFIED**. If single-factor, a captured password + cookie is full GitHub takeover. ATT&CK T1566.002. |
| Malicious-attachment payload (PDF/Office/zip with macOS-targeted Mach-O dropper) | **T** (tamper running OS) + **E** (LPE → full disk access) | **High** | Apple Mail and the system Quick Look pipeline have a long history of CVEs; AppleScript / xattr abuse to bypass quarantine is documented. UNVERIFIED whether the operator opens attachments outside a VM. ATT&CK T1566.001 + T1204.002. |
| OAuth-prompt fraud (e.g. fake "GitHub wants access" consent screen on a typosquatted domain) | **S** + **R** (repudiation — token issued to attacker app, hard to reconstruct) | **High** | Real GitHub OAuth consent fatigue is a well-known TTP (T1528 Steal Application Access Token). Even with MFA, OAuth consent grants bypass MFA on subsequent automated access. |
| Calendar-invite / iCloud share phishing (less classic but macOS-native) | **S** + **I** | Medium | Calendar invites can include URLs that pre-render previews; less likely to land creds but useful for recon. |
| Deepfake voice / SMS pretext claiming to be Anthropic / GitHub / a kingdom contributor | **S** | Medium | Single-operator means no second-pair-of-eyes verification on social-engineering attempts. ATT&CK T1566.004 (impersonation). |
| **Adversary-in-the-middle / Evilginx-style proxy of GitHub login** — captures session token even with TOTP MFA | **S** + **E** | **High** | Industry-leading 2024-2026 TTP; defeats TOTP because the proxy forwards the live OTP to the real site and captures the resulting cookie. Defeated only by origin-bound WebAuthn (i.e. a hardware key per R1). |

**Why phishing matters disproportionately here:** the kingdom has no second person to flag a suspicious request. The matrix's "single-operator GAP" framing accepts this as architectural; phishing is the threat that turns it into compromise. The AiTM sub-vector is specifically called out because it is the threat hardware-key R1 is designed to defeat — TOTP alone does not.

### 3.2 macOS malware paths

| Sub-vector | STRIDE | Severity | Notes |
|------------|--------|----------|-------|
| **Gatekeeper bypass** — `xattr -d com.apple.quarantine` self-application or supply-chain dropped binary that the user runs without quarantine | **T** + **E** | **High** | Per Apple, Gatekeeper checks notarization on first run. Operator-side `xattr -d` (a common workaround for Homebrew-tap binaries / unsigned dev tools) **directly opens this hole**. UNVERIFIED whether the operator habitually removes quarantine on Homebrew-tap binaries — given the heavy custom-tooling profile in CLAUDE.md (`zhen-cli`, `zhenai-forge`, `doom-runner`), almost certainly yes for at least some artifacts. |
| **Signed-but-malicious app** — adversary uses a stolen Apple Developer ID or a typosquatted-dev-id signed app | **S** + **T** | Medium | Apple revokes promptly on report, but window-of-exposure is hours-to-days. macOS notarization does NOT scan for malicious code; it only confirms a registered developer signed it. |
| **Browser drive-by** — V8 / WebKit RCE leading to sandbox escape | **T** + **E** | Medium | Safari/WebKit and Chrome/V8 ship a steady CVE stream; macOS sandbox escapes are rarer but exist (Pwn2Own annually). UNVERIFIED whether the operator pins to current-OS / current-browser; macOS 25 (Darwin 25.4.0) is current per system info. |
| **Malicious macOS package (.pkg)** with postinstall script | **T** + **E** | Medium | Common for legitimate-looking installers. Postinstall runs as root if user enters password. |
| **Malicious LaunchAgent / LaunchDaemon plant** — persistence via `~/Library/LaunchAgents/` | **T** (persistence) | Medium | Once code runs once, persistence is trivial on macOS without SIP-bypass. UNVERIFIED whether the operator runs `lsof`/EDR coverage to detect new launchd entries. |
| **Kernel extension / DriverKit abuse** — requires SIP off or Recovery boot to install historically; on Apple Silicon further restricted | **T** + **E** | Low | SIP is on by default on macOS 14+; UNVERIFIED on this device but assumed on (Darwin 25.4.0 / arm64). |

### 3.3 SSH-agent / Keychain extraction

| Sub-vector | STRIDE | Severity | Notes |
|------------|--------|----------|-------|
| **`ssh-add -L` / `ssh-add -l` from a malicious local process** to enumerate loaded identities | **I** (info disclosure) | High | Once any code runs as the user, `ssh-add` works without re-prompting the user. |
| **Direct ssh-agent socket abuse** — `$SSH_AUTH_SOCK` is just a UNIX socket; any process the user runs can sign challenges with the loaded keys | **E** (escalation: no need to actually steal the key — just use it) | **High** | This is the textbook ssh-agent-hijack threat. Defense: `ssh-add -c` (confirm before each use) — UNVERIFIED whether the operator uses `-c`. Default is unconfirmed. |
| **Agent forwarding to a compromised remote host** (`ssh -A`) | **E** | High | If `~/.ssh/config` has `ForwardAgent yes` for any kingdom host that is itself a target (EAST / WEST), a compromise of that host yields signing oracle access to all loaded keys. UNVERIFIED whether ForwardAgent is on for east/west. **Per `~/.ssh/config` review (NOT performed here) — recommend explicit `ForwardAgent no` default + per-host opt-in.** |
| **Keychain dump** via `security dump-keychain -d login.keychain-db` after sudo / TCC bypass | **I** | High | macOS Keychain is well-protected against unauthorized read but well-known to dump-with-password. ML-DSA-65 / age / GPG keys in Keychain are at risk if Keychain is unlocked + an attacker has interactive code execution. |
| **Browser credential extraction** (Chrome's `Login Data` SQLite, Safari's Keychain entries) | **I** | High | All session cookies, including the one that holds GitHub auth, can be exfiltrated. |
| **`ssh-add -e`-style hardware-key downgrade** if the operator removes a YubiKey / Sec Key for any reason | **E** | Medium | Only relevant if hardware keys are present today — UNVERIFIED. |
| **Persistent shell history mining** — `~/.zsh_history`, `~/.bash_history` | **I** | Medium | Past commands often include `ssh govan@east <one-shot command>`, signed-deploy commands, etc. Also leaks per-host-IP patterns. |

### 3.4 Browser session hijack — GitHub specifically

This vector is *especially* sharp because of CLAUDE.md's `No `gh` CLI` rule: every privileged operation (repo creation, branch protection, releases, secrets management for GitHub Actions) goes through the browser. There is no token-bound-to-CLI alternative path.

| Sub-vector | STRIDE | Severity | Notes |
|------------|--------|----------|-------|
| **Cookie theft** via malware (cf. §3.2) | **S** + **I** | **High** | One stolen `user_session` cookie + the right User-Agent = identity assumption that bypasses MFA (because MFA was already cleared when the cookie was issued). |
| **Browser-extension compromise** (a malicious or hijacked-via-update Chrome/Firefox extension can read every page including GitHub) | **I** + **T** (can also modify form submissions) | **High** | The extension supply chain is itself a high-stakes ecosystem. UNVERIFIED what extensions are installed. Recommend audit. |
| **Session-token leak via screen-share / co-streaming** | **I** | Medium | Single-operator means no peer pair-programming; lower exposure than at-org but non-zero (presentations, demos). |
| **Same-site cookie cross-origin leak** — generally mitigated since 2020 by SameSite=Lax defaults, but mis-set cookies still surface in CVEs | **I** | Low | Modern browsers default to SameSite=Lax; less salient. |
| **Phished OAuth consent → persistent token even after password rotation** | **S** + **R** | High | OAuth tokens survive password reset unless explicitly revoked. UNVERIFIED whether the operator audits `Authorized OAuth Apps` regularly. |
| **GitHub PAT leakage** — if any PAT exists (e.g. for CI integration), stored in Keychain / `.netrc` / `.config/git/credentials` | **I** | High | UNVERIFIED. CLAUDE.md says no `gh`, but Git over HTTPS still uses some credential helper if HTTPS remotes are used; SSH remotes mitigate this. |

### 3.5 Supply-chain via dev tools

| Sub-vector | STRIDE | Severity | Notes |
|------------|--------|----------|-------|
| **VS Code / Cursor / IDE extension compromise** — extension auto-update model is a publish-once-trust-forever channel | **T** | **High** | Recent precedent: 2024-2025 Visual Studio Code marketplace and the broader IDE-extension ecosystem repeatedly lost trust due to typosquats and account takeovers. UNVERIFIED what's installed. Marketplaces don't audit code. **Recommend: extension allowlist, no auto-update, periodic audit.** |
| **Homebrew formula compromise** — formula points to a tarball whose URL is mutable in some taps | **T** | Medium | `brew install` over a hijacked tap → arbitrary post-install scripts. CVE history exists (homebrew-cask, less so homebrew-core which has stronger review). UNVERIFIED what taps are tapped. |
| **Go module tampering** — `go.sum` mitigates by hash-pinning, but `go install` of a compromised tool (e.g. `go install mcr.microsoft.com/...`) without `go.sum` is unprotected | **T** | Medium | The kingdom uses Go heavily; the dev box runs `go install` for tooling. UNVERIFIED tooling list. Module proxy cache + GOSUMDB are mitigations the operator may or may not have on. |
| **Cargo / crates.io compromise** | **T** | Medium | Same shape as Go. The kingdom has substantial Rust (`crates/`). Cargo lock pinning is implicit but the *build script* model in Cargo (`build.rs`) executes arbitrary code at build time → **a single compromised transitive dep with a build script = code execution on the dev box during `cargo build`**. |
| **Python pip / PyPI compromise** (kingdom has `raft/zhen_app.py`, fine-tuning scripts) | **T** | Medium | Typosquatting on PyPI is classic; `pip install` runs setup.py. UNVERIFIED whether installs go to a venv or system Python. |
| **npm / yarn / pnpm** in any frontend tooling chain (`dashboard/`, `kanban/` are vanilla JS but tooling may pull npm) | **T** | Medium | UNVERIFIED scale; vanilla JS reduces but does not eliminate exposure. |
| **Compromised MCP server** (`raft/zhen_mcp_server.py` per CLAUDE.md offers 7 tools to Claude Code) | **T** + **E** | High | An MCP server pulled from a third-party registry could exfiltrate or tamper. The kingdom's own MCP is in-tree (low risk for that one) but other MCPs the operator might add are the surface. |
| **GitHub Actions / CI** transitive supply chain | **T** | Medium-High | If CI deploys, a compromised action (popular pattern: `actions/checkout` typo, marketplace action with malicious update) could exfiltrate the build host's secrets — including, per `01-scrutiny` §BM7, possibly the Sealed Cask signing key. |

### 3.6 Physical access

| Sub-vector | STRIDE | Severity | Notes |
|------------|--------|----------|-------|
| **Laptop theft while traveling** | **I** + **T** + **E** | **High** | If FileVault is on, ciphertext-only — ground-truth UNVERIFIED. macOS default is FileVault prompted on first boot but user can decline. **Recommend: confirm FileVault on; verify recovery key custody.** Per `01-scrutiny` `MP-5 / SC-28` are the relevant control families and they are mostly UNVERIFIED today. |
| **"Evil maid"** — brief unattended access to install a hardware keylogger, swap a peripheral, or boot Recovery to plant a kext / modify boot args | **T** + **I** | Medium | Apple Silicon T2/Secure Enclave + signed boot makes this much harder than Intel-era macOS, but Recovery still exists and exposes the user account if a password is known. |
| **Shoulder surfing / over-the-shoulder camera capture in coffee shop** | **I** | Medium | Privacy filter present? UNVERIFIED. |
| **Border crossing / customs detention** — five-eyes border officials can demand device unlock | **I** + **E** | Low-Medium | Outside US travel risks. Not a daily threat for this operator AFAIK. |
| **Lost device, no remote-wipe** | **I** | Medium | Find My Mac UNVERIFIED. iCloud backup with the same Apple ID = identity-recovery channel which can be used by the finder to unlock if Apple ID password is compromised. |

### 3.7 Network-side attacks

| Sub-vector | STRIDE | Severity | Notes |
|------------|--------|----------|-------|
| **Coffee-shop / hotel WiFi MitM** — captive portal, ARP spoofing, downgrade attacks | **S** + **T** | Medium | TLS 1.3 on the wire mitigates passive sniffing; active MitM relies on cert-pinning bypass / user-clicked-through cert warnings. **Defense: VPN-on-untrusted-network policy** — UNVERIFIED whether the operator runs an always-on VPN. |
| **Malicious VPN provider** — outsourcing trust to a third party | **I** | Medium | If a free / unaudited VPN is used, that provider sees all traffic. Tailscale / Mullvad / a self-hosted WireGuard are different threat surfaces. UNVERIFIED. |
| **DNS hijack** on local network | **S** + **T** | Medium | Pushes traffic to attacker servers presenting fake TLS certs; user must click through. DoH (DNS-over-HTTPS) on by default in modern macOS/Safari mitigates. |
| **Captive-portal credential phish** (free WiFi sign-in page that asks for an email/Apple ID) | **S** + **I** | Low-Medium | Standard travel hygiene. |
| **Rogue P2P endpoint** — if `192.168.13.0/24` is exposed to a hostile network unintentionally (e.g. operator joins a coworking LAN that overlaps), the EAST P2P link's expected peer IP could be impersonated | **S** | Low (per CLAUDE.md the link is direct point-to-point) | If point-to-point is literal cable, this is N/A. If it's a routed link over wifi, it's relevant. UNVERIFIED. |
| **WireGuard overlay (deferred per CLAUDE.md)** — when this lands, key custody on the laptop is added to the keystone | **I** + **S** | Future — TBD |
| **macOS Sharing services** — accidentally-on file sharing / screen sharing / Remote Login | **I** + **T** | Low-Medium | UNVERIFIED state. Default off, but operators frequently turn one on for a session and forget. |

### 3.8 Operator-error / accidental disclosure

This is the hardest category to model and, per industry data, the largest source of real-world incidents.

| Sub-vector | STRIDE | Severity | Notes |
|------------|--------|----------|-------|
| **Secret in screenshot** posted to Slack / Discord / Twitter / a wiki page | **I** | **High** | API keys, JWTs, Wotan tokens all show up in IDE output panes. Shoulder-surfing-by-share is the modern shape of this. The kingdom's `community-first` doctrine *invites* public-facing screenshots. |
| **Paste of secret into ChatGPT / Claude / external LLM for "help debugging"** | **I** | **High** | Per `01-scrutiny` §S3, the kingdom's PII floor is "no first-class PII processing" but operator-side LLM use is the *most* documented PII / secret leakage vector across the broader industry today. **The operator running this very session can paste anything; nothing prevents it.** |
| **`git push` of `.env` / `id_rsa` / SOPS-unencrypted file** | **I** + **R** | High | The repo has `.gitignore` discipline (UNVERIFIED to what extent). `git filter-repo` post-leak only helps if remote is rewritable; GitHub forks complicate. |
| **Wrong-channel post** (private secret pasted into a public Slack channel) | **I** | Medium | Single-operator means no second-channel sanity check before paste. |
| **Public commit of an internal file** (`docs/private/*` accidentally tracked) | **I** | Medium | UNVERIFIED whether `.git/info/exclude` discipline is enforced. |
| **Public-release of an unintended artifact** — the kingdom's stated `public-release authority` (per BM5) is also the surface for releasing-the-wrong-thing. A Sealed Cask intended for `dev` parish but tagged for production. | **T** + **I** | Medium | Defense: pre-publish review checklist — UNVERIFIED. |
| **Voice-assistant transcription leakage** — Siri / Alexa / a Zoom auto-transcript capturing a spoken password | **I** | Low | Edge case; raised because it's increasing in incident reports. |
| **Backup-to-iCloud of `.config/sops` / `.ssh`** | **I** | Medium | If iCloud keychain sync includes SSH keys (off by default but user-toggleable), or if Time Machine to an unencrypted external disk includes the home directory verbatim — both expose the same key material via a different boundary. UNVERIFIED. |

---

## 4. Mitigations in place today (honest inventory)

This section is deliberately curt because the honest answer is: **most of the controls below are UNVERIFIED, several are likely off, and the matrices have already flagged these as gaps without explicitly tying them to the workstation.** The Marshal will not over-claim here. Per `01-scrutiny` §S2 the prior matrices' MAPPED claims were name-dropping; this one will not repeat that error.

| Control | Status | Evidence / source |
|---------|--------|-------------------|
| **SOPS + age** for repo secrets | **MAPPED (partial)** | CLAUDE.md §"Secrets Management" — but the age key custody itself is UNVERIFIED (per `01-scrutiny` §BM7) |
| **SSH key-based auth** to bare-metal hosts (no passwords) | **UNVERIFIED** but assumed | CLAUDE.md `ssh govan@east` reference; key vs password not explicitly stated |
| **Apple Silicon Secure Enclave + T2** for hardware-anchored crypto | **STRUCTURAL** | Implied by Darwin 25.4.0 / arm64 |
| **macOS SIP** (System Integrity Protection) | **UNVERIFIED** but default-on for the OS | Apple default; `csrutil status` would confirm |
| **macOS XProtect / MRT** built-in malware protection | **STRUCTURAL** | Always-on on macOS, signature-based; not a substitute for EDR |
| **macOS Gatekeeper** | **STRUCTURAL** | Default-on; user can override per-binary |
| **FileVault full-disk encryption** | **UNVERIFIED** | Apple ships with FileVault prompt during initial setup — recommend confirming with `fdesetup status` |
| **Notarization on Apple-distributed apps** | **STRUCTURAL** | Default-on via Gatekeeper |
| **`pkg/auth` framework** in repo | **MAPPED** but not relevant to workstation | `pkg/auth` defends *services*, not the operator's laptop. The keystone is upstream of all `pkg/auth` controls. |
| **Sealed Cask verification (`scripts/verify-binding-rune.sh`)** for deploy-side | **MAPPED for deploy, GAP for build-side** | Per `01-scrutiny` §BM7 the build host and key custody are UNVERIFIED |
| **`scripts/verify-gpl-boundary.sh`** | **MAPPED for license hygiene** | Just patched per task brief; protects *project license* discipline, not workstation security |
| **2FA on GitHub** | **UNVERIFIED** | `01-scrutiny` `IA-2(1)` is UNVERIFIED |
| **Hardware security key (YubiKey / Apple Touch ID for SSH)** | **UNVERIFIED — likely NOT in use** | No mention in CLAUDE.md or any ADR |
| **EDR / endpoint security (CrowdStrike / SentinelOne / open-source equivalent)** | **NOT in place** | Single-operator open-source project, no EDR budget |
| **Password manager (1Password / Bitwarden) for non-Keychain secrets** | **UNVERIFIED** | Likely in use given operator profile (10yr IT/SRE) but undocumented |
| **VPN always-on for untrusted networks** | **UNVERIFIED** | Not stated |
| **Homebrew tap audit policy** | **NOT in place** | No documented policy |
| **VS Code / IDE extension allowlist** | **NOT in place** | No documented policy |
| **Shell history hygiene (HISTCONTROL=ignorespace, periodic prune)** | **UNVERIFIED** | Standard zsh defaults likely |
| **Per-host SSH agent confirmation (`ssh-add -c`)** | **UNVERIFIED** | Default is unconfirmed |
| **`ForwardAgent no` policy in `~/.ssh/config`** | **UNVERIFIED** | macOS default is OFF; per-host opt-in pattern unconfirmed |

**Net assessment:** the structural macOS controls (Secure Enclave, SIP, Gatekeeper, XProtect, FileVault-if-on) provide a baseline that's better than the average single-developer workstation. The kingdom-side controls (SOPS, ML-DSA-65, Sealed Cask, `pkg/auth`) defend the *kingdom* from the *workstation*'s outputs but **none of them defend the workstation itself.** That is the matrix family's blind spot in one sentence.

### 4.1 UNVERIFIED-resolution recipe (for operator-side follow-up)

Every UNVERIFIED line above resolves with a single command run on the device. This recipe is the natural next-step a follow-on operator-side session can run; the output should land in a private operator runbook (NOT the public wiki) with the date stamp.

| UNVERIFIED item | One-line check | Expected good answer |
|-----------------|----------------|----------------------|
| FileVault on? | `fdesetup status` | `FileVault is On.` |
| SIP on? | `csrutil status` | `System Integrity Protection status: enabled.` |
| Secure Boot mode? | `bputil -d` (Apple Silicon) | `Secure Boot: Full Security` |
| Firewall on? | `defaults read /Library/Preferences/com.apple.alf globalstate` | `1` or `2` (not `0`) |
| Find My Mac on? | `defaults read /Library/Preferences/com.apple.FindMyMac.plist FMMEnabled` (sudo) | `1` |
| Gatekeeper on? | `spctl --status` | `assessments enabled` |
| Sharing services off? | `launchctl list \| grep -E '(smbd\|afpd\|screensharing\|sshd)'` | empty (no Remote Login on consumer Mac) |
| FileVault recovery key custody | `fdesetup haspersonalrecoverykey; fdesetup hasinstitutionalrecoverykey` | one or both `true`, AND key is documented in operator runbook |
| ssh-agent identity inventory | `ssh-add -L` | review each line; remove anything stale; flag any `sk-*-sk` (hardware) absence |
| ssh-agent confirmation flag | `ssh-add -l \| grep -c '\bC\b'` (no native flag — use `ssh-add -L` and confirm `-c` was set per key) | every kingdom key was added with `-c` |
| `~/.ssh/config` ForwardAgent default | `ssh -G east \| grep -i forwardagent` | `forwardagent no` |
| GitHub MFA status | login → `Settings → Password and authentication` | "Two-factor authentication is enabled" + at least one **security key** registered |
| Authorized OAuth apps | `Settings → Applications → Authorized OAuth Apps` | minimal list; revoke unknowns |
| Personal access tokens | `Settings → Developer settings → Personal access tokens` | empty (kingdom uses SSH remotes per CLAUDE.md no-`gh` rule) |
| Browser extensions | per-browser `chrome://extensions` / `about:addons` | minimal list, all from known publishers, `Auto-update: off` |
| Homebrew taps | `brew tap` | only `homebrew/core`, `homebrew/cask`, plus runbook-justified taps |
| VS Code extensions | `code --list-extensions` | matches operator runbook allowlist |
| age key location | `ls -la ~/.config/sops/age/keys.txt 2>/dev/null \|\| ls -la ~/Library/Application\ Support/sops/age/keys.txt` | exists with mode `600`; documented in operator runbook |
| Sealed Cask deploy key location | (UNDOCUMENTED — locate via `scripts/build-sealed-cask.sh` reading) | resolve and document; if on laptop, advance R3 |
| ML-DSA-65 dev signing key location | grep ADR-043 + `services/wotan/internal/signing/` | resolve and document; if on laptop, advance R3 |
| Apple ID 2FA + trusted devices | `System Settings → [Apple ID] → Sign-in & Security` | trusted devices reviewed; trusted phone numbers minimal |
| iCloud Keychain SSH/secret sync state | `System Settings → Apple ID → iCloud → Passwords & Keychain` | confirm what is synced; flag if sensitive items are syncing to iCloud |
| Time Machine destination encryption | `tmutil destinationinfo` + `diskutil cs list` | encrypted destination |
| EDR / monitoring presence | `system_profiler SPLaunchDaemonsDataType` audit | absence is acceptable for now; presence-of-unknown is a finding |

Each row above closes a specific UNVERIFIED entry in §4 and removes it from the BlackMage opportunity surface.

---

### 4.1 Worked example — the most dangerous attack chain

To make the matrix-style table above concrete, here is the chain the Marshal judges most dangerous *and most realistic* under today's posture. It composes vectors from §3.1, §3.5, §3.3, and §3.4:

1. **Initial foothold (T1195.002 / T1546):** the operator installs or auto-updates a VS Code extension whose maintainer was account-takeover'd this week (precedent: the 2024-2025 marketplace incidents). The extension's `package.json` includes a postinstall step that drops a Mach-O helper into `~/Library/Application Support/<plausible-name>/` and registers a LaunchAgent in `~/Library/LaunchAgents/<plausible-name>.plist`. No prompt fires; the extension was already trusted by the operator.
2. **SSH-agent exploitation (T1552.004):** the helper waits for an interactive terminal session (any kingdom dev work). When `$SSH_AUTH_SOCK` is exported and at least one identity is loaded (`ssh-add -L` returns ≥1 line), the helper signs ssh-agent challenges *as the operator* without ever needing the on-disk key — it just talks to the agent socket. It hits `govan@east` and `govan@west` and validates pivot reachability.
3. **GitHub session siphon (T1539):** in parallel, the helper reads the Chrome / Firefox / Safari cookie store and copies the `user_session` cookie for `github.com`. Because no hardware-bound 2FA gate is in place (current MFA UNVERIFIED, likely TOTP-at-best), and because cookies are issued post-MFA, the attacker can replay this cookie from anywhere and have full repo-write authority.
4. **Sealed Cask + ML-DSA-65 key theft (T1552.001):** the helper enumerates `~/.config/`, `~/.ssh/`, `~/.config/sops/age/`, `~/Library/Keychains/` (read-only without a password unlock; helper waits for an interactive Touch ID / password prompt and grabs Keychain entries during the unlock window). It exfiltrates: the age key (decrypts every SOPS file in the repo), the Sealed Cask deploy key (per ADR-010 §"The Binding Rune", `signed_by: deploy-key-2026-02` — UNVERIFIED location), and the ML-DSA-65 dev signing key (per ADR-043 hard-condition #4 — *"DOCUMENTED (dev keys for spike)"*, custody UNVERIFIED).
5. **Persistence and clean-up (T1546.004 / T1027):** the LaunchAgent stays. Logs are scrubbed. `.zsh_history` is left untouched (suspicious deletions raise flags); the helper's filenames mimic a real Apple service.
6. **Delayed exploitation (T1078):** weeks later, the attacker uses the stolen GitHub cookie to push a single small commit to `main` containing a typosquatted dependency. They issue a forged Sealed Cask whose Binding Rune validates because they have `deploy-key-2026-02`. They forge a `config.*` Wotan topic with the stolen ML-DSA-65 key that any Heimdall daemon will accept. The kingdom's own verification chain — the strongest part of its security posture — now serves the attacker.
7. **Detection lag (BM4 in scrutiny):** Mímir is alerts-only. Sentinel daily summary will catch the anomalous push *after* Stevie reads it the next morning. Per `01-scrutiny` §SEN2, no on-call, no paging. Median time to operator response is hours-to-days. **The attacker had ≥7 hours of unrestricted runtime per intrusion** (BM4 estimate) and the entire signature-chain integrity is now corrupt at root.

**Why this chain is the most dangerous:** every link uses a vector currently *unmitigated or UNVERIFIED*. Every kingdom-side defense (SOPS, Sealed Cask, ML-DSA-65 signing, `pkg/auth`, Champion gate, Mímir drift detection) is bypassed because the attacker holds the *signing roots* — i.e. the kingdom's own verifications return `OK` for attacker-produced artifacts. The blast radius is not "operator account compromised"; it is **the entire kingdom's signature chain is corrupted at the root and every downstream verification falsely passes**.

**Mitigation map:** R1 (hardware key) cuts step 3 and partially step 2. R3 + R10 (off-laptop / hot-cold key separation) cuts step 4. R7 (extension allowlist + no-auto-update) cuts step 1. **R1 + R3 together convert this chain from "feasible today" to "requires physical possession of two distinct hardware tokens".**

---

## 5. Threat-vector summary (highest-leverage for attention)

Ranked by `(blast radius) × (likelihood) / (cost-to-mitigate)`:

1. **Phishing → GitHub session takeover** (no MFA verification + browser-only path = single-stolen-cookie wins). §3.1 + §3.4. **Highest single-stolen-credential blast radius.**
2. **SSH-agent abuse / agent forwarding to a compromised remote** — once any code runs as the user, signing oracle is trivial. §3.3. **Highest leverage from any-process-running-as-user.**
3. **Dev-tool supply chain (IDE extension or Cargo `build.rs`)** — compromise of one transitive dep gives attacker code execution during routine `cargo build` / `go install` / VS Code update. §3.5. **Highest probability of being the first foothold.**
4. **Operator paste-into-LLM** (this very session's agent class is the most-cited industry leakage path of 2024-2026). §3.8. **Highest probability of an operator-side leak.**
5. **Sealed Cask signing key custody on the laptop** — this is the single keystone-key with no documented HSM-backing. §2.1 + ADR-010 §"Why Encrypted at Rest" + `01-scrutiny` §BM7. **Highest individual-key blast radius.**
6. **FileVault verification + iCloud backup posture** — physical theft becomes catastrophic only if FileVault is off OR backups encrypted-with-iCloud-account that's recoverable via email. §3.6. **Highest "is-it-already-fixed?" question.**
7. **ML-DSA-65 dev key location and quarterly rotation discipline** — ADR-043 hard-condition #4 says HSM-grade; reality is "DOCUMENTED (dev keys for spike)". §2.1. **Highest discipline-gap.**

---

## 6. Recommendations (prioritised, evidence-anchored)

The list below maps each recommendation to which §3 / §4 finding it addresses and is ordered by leverage.

### R1. Hardware security key (FIDO2 / WebAuthn) for GitHub + SSH

- **Addresses:** §3.1, §3.4, §3.3, §2.1, §2.2.
- **Specifics:** A YubiKey 5C NFC (or two — one in a safe deposit box / off-site) with:
  - GitHub account: `Security keys` set as the primary 2FA + backup TOTP saved offline.
  - SSH: `ssh-keygen -t ed25519-sk` → resident key on the YubiKey for `govan@east`, `govan@west`, and any `bellistech` org access.
  - Browser passwordless login on GitHub via WebAuthn.
- **Why it's high-leverage:** removes the "stolen-cookie" + "stolen-key" failure modes simultaneously. A phisher cannot present a valid origin to the YubiKey; an attacker with file-read on `~/.ssh/` cannot forge a signature without the device.
- **Owner:** Operator (Stevie). **Cost:** ~$50 + 1 hour setup. **Verification:** `ssh-add -L` shows `sk-ssh-ed25519@openssh.com` lines.

### R2. Confirm and document FileVault, Find My Mac, and recovery key custody

- **Addresses:** §3.6 + §4 (UNVERIFIED line).
- **Specifics:** Run `fdesetup status` and post the output to a private operator-runbook (NOT the public wiki). Confirm Find My Mac is on. Confirm the FileVault recovery key is stored either (a) in iCloud (acceptable if Apple ID has 2FA + a hardware key per R1) or (b) on paper in a physical safe.
- **Why it's high-leverage:** turns physical theft from a P0 into a P3.
- **Owner:** Operator. **Cost:** 15 minutes. **Verification:** runbook exists, `fdesetup status = On`.

### R3. Move Sealed Cask + ML-DSA-65 signing keys off the laptop OR enforce hardware-backing

- **Addresses:** §2.1 (Sealed Cask + ML-DSA-65), `01-scrutiny` §BM7, ADR-043 hard-condition #4.
- **Specifics:** Two paths, pick one:
  - **(a) Hardware-backed on-laptop:** keys held in YubiKey-resident slots (PIV for Ed25519 / RSA; ML-DSA-65 is not yet in standard hardware, so for ML-DSA hold it in the macOS Keychain *with the Keychain itself unlocked only via Touch ID / hardware key*).
  - **(b) Off-laptop:** keys held on a dedicated, network-isolated signing host (e.g. a Mac Mini wiped and locked, accessible only via in-person USB-C). Build pipeline uploads artifacts; signing host signs and emits the Binding Rune.
- **Why it's high-leverage:** today, `read $HOME` = full kingdom signing authority. Either path raises the bar to "physical possession of a hardware key" or "physical possession of a separate machine." The matrix's `IA-7 / SC-12` PARAM-GAP closes.
- **Owner:** Architect + Operator. **Cost:** medium (a day to implement + ADR-010 amendment). **Verification:** ADR-010 §"Why No Compilers on Nodes" amended with explicit key-custody location; `scripts/build-sealed-cask.sh` reads the signing key only via a hardware-bound channel (ssh-add-style or PKCS#11).

### R4. SSH-agent confirmation + ForwardAgent disable as defaults

- **Addresses:** §3.3.
- **Specifics:**
  - Add `Use ssh-add -c` for all keys: `ssh-add -c ~/.ssh/id_ed25519_east` etc. Each `ssh` invocation prompts (Touch ID / system dialog) before signing.
  - In `~/.ssh/config`, set top-level `ForwardAgent no` and only enable per-host where strictly required (none today, AFAICT — east/west don't need agent forwarding).
- **Why it's high-leverage:** changes "any-malicious-process-can-sign" into "every signature requires user gesture". Mitigates the post-foothold pivot.
- **Owner:** Operator. **Cost:** 30 minutes + occasional Touch ID prompts during routine work. **Verification:** `ssh-add -l -c` confirms confirmation flag; `ssh -G east | grep -i forwardagent` returns `no`.

### R5. GitHub MFA enforcement + OAuth audit + PAT inventory

- **Addresses:** §3.1, §3.4, `01-scrutiny` `IA-2(1)`.
- **Specifics:**
  - Confirm GitHub account has MFA enforced (preferably WebAuthn from R1 + TOTP backup).
  - Audit `Settings → Applications → Authorized OAuth Apps` and revoke any not in active use.
  - Inventory any PATs (none should exist if `gh` CLI is genuinely unused; confirm with `git config --global --get credential.helper` returning empty / native osxkeychain).
  - Enable `Settings → Notifications` for sign-in from new device + IP.
- **Why it's high-leverage:** closes the matrix `IA-2(1) UNVERIFIED` against the actual highest-blast-radius identity in the kingdom.
- **Owner:** Operator. **Cost:** 1 hour. **Verification:** GitHub `Security log` shows MFA-required, screenshot the settings page into the operator runbook.

### R6. Browser hardening for the GitHub-write profile

- **Addresses:** §3.4.
- **Specifics:** dedicated browser profile (or separate browser, e.g. Safari for kingdom GitHub-write only). No extensions. Clear cookies on quit. Pin GitHub TLS cert via `Settings → Privacy → HTTPS-Only` + cert-pinning extension if available. Treat all other browsing in a different profile.
- **Why it's high-leverage:** reduces the attack surface that browser-extension compromise (§3.5) and drive-by RCE (§3.2) can leverage to reach the GitHub session.
- **Owner:** Operator. **Cost:** 1 hour + small ergonomic friction. **Verification:** profile exists, extension list is empty.

### R7. Dev-tool supply-chain hygiene checklist

- **Addresses:** §3.5.
- **Specifics:**
  - **VS Code / Cursor:** disable auto-update of extensions; document an extension allowlist in the operator runbook; run `code --list-extensions` periodically; reject anything not on the list.
  - **Homebrew:** `brew list --tap` audit. Use only `homebrew/core` and `homebrew/cask` unless a tap is justified in a runbook entry.
  - **Cargo:** `cargo install` only from crates.io; treat `build.rs` as a code-execution surface — **per-crate review required for net-new direct deps**.
  - **Go:** `GOPROXY=https://proxy.golang.org` (default), `GOSUMDB=sum.golang.org` (default), no `go install` from non-GitHub repos without inspection.
  - **MCP servers:** only `raft/zhen_mcp_server.py` (in-tree) is allowed today; any new MCP requires runbook entry.
- **Why it's high-leverage:** these are the most likely first-foothold paths and they have ~zero current discipline.
- **Owner:** Operator + Architect. **Cost:** 2-3 hours initial + 30 min/quarter audit. **Verification:** runbook published.

### R8. Operator-side LLM-paste discipline (no-secrets policy + scrubbing)

- **Addresses:** §3.8 (the highest-probability operator-error vector).
- **Specifics:** documented rule that no API keys, no `id_*`, no `~/.config/sops/age/keys.txt`, no `.env`, no `kubeconfig` is ever pasted into Claude / ChatGPT / any non-self-hosted LLM. Tooling: a paste-time scrubber (e.g. `git secrets`-style local hook that rejects clipboard content matching common secret patterns before paste — there are open-source tools for this).
- **Why it's high-leverage:** matches industry-leading incident class for 2024-2026.
- **Owner:** Operator. **Cost:** 30 minutes documentation + tooling setup. **Verification:** runbook exists, scrubber installed.

### R9. Always-on VPN policy on untrusted networks

- **Addresses:** §3.7.
- **Specifics:** Tailscale / Mullvad / self-hosted WireGuard on by default when not on the home LAN. macOS `Profiles` can enforce.
- **Why it's medium-leverage:** modern TLS 1.3 closes the obvious passive-sniff window, but DNS hijack + captive-portal phish + cert-warning click-through remain. VPN moves the trust boundary to a known provider.
- **Owner:** Operator. **Cost:** $5/mo + 30 min setup. **Verification:** profile exists.

### R10. Hot/cold key separation for kingdom signing roots

- **Addresses:** §2.1 (multiple), ADR-043 hard-condition #4 (quarterly rotation), ADR-010 (deploy key custody).
- **Specifics:**
  - **Hot keys** (used daily): present on the laptop (or on a YubiKey accessible to the laptop) for routine commit signing, ssh, low-impact Wotan publishes. Limited blast radius by scope (Wotan topic ACLs, Sealed Cask `parish` field).
  - **Cold keys** (used quarterly/never): air-gapped storage (USB drive in safe, paper-printed Shamir splits, or offline signer machine). Used only for: (a) issuing new hot signing keys, (b) Sealed Cask production tag-cutting, (c) ML-DSA-65 ceremony rotation per ADR-043.
- **Why it's high-leverage:** even if a hot key is compromised, the cold root can revoke and reissue. Without separation, every compromise is a reset of the entire trust chain.
- **Owner:** Architect + Operator. **Cost:** 1 day setup + ADR amendment. **Verification:** ADR-010 + ADR-043 amended; cold-key inventory documented (private runbook).

### R11. Periodic threat-model review cadence

- **Addresses:** drift from this document.
- **Specifics:** this threat model is reviewed at every age boundary (Age 2 → Age 3, etc.) and on any material posture change (e.g. WireGuard overlay landing, Sealed Cask production deploy, key-rotation ceremony). Review must check: did any UNVERIFIED resolve? Did any new asset land on the laptop?
- **Why it's medium-leverage:** the blast radius of the laptop will grow as the kingdom grows; the model must keep up.
- **Owner:** Marshal. **Cost:** 1 hour per review. **Verification:** `Last reviewed:` line at the top of this doc, updated each pass.

---

## 7. Coverage map back to scrutiny / matrices

| Scrutiny finding | This document addresses |
|------------------|-------------------------|
| `01-scrutiny` §BM5 (macOS box keystone, MFA UNVERIFIED) | §2.1, §2.2, §3.1, §3.4, §6 R1 + R5 |
| `01-scrutiny` §SEN4 (no threat model exists for workstation) | This entire document |
| `01-scrutiny` §BM7 (Sealed Cask key custody undocumented) | §2.1, §6 R3 + R10 |
| `01-scrutiny` `IA-2(1) UNVERIFIED` (NIST 800-53 MFA) | §6 R1 + R5 |
| `01-scrutiny` `MP-5 / SC-28` (media protection / encryption-at-rest) | §6 R2 |
| `01-scrutiny` PS family (Personnel Security — single-operator gap) | §1, §2 (this whole document is the missing PS-family threat artefact) |
| ADR-043 hard-condition #4 (HSM-grade key + quarterly rotation) | §2.1, §6 R3 + R10 |
| ADR-010 deploy key custody | §2.1, §6 R3 + R10 |

---

## 8. Provenance and explicit non-warranties

**Read-only audit; no probing of the live workstation occurred.** All claims about UNVERIFIED state (FileVault, MFA, browser extensions, ssh-agent posture, Homebrew taps, etc.) are exactly that — UNVERIFIED. The Marshal authoring this session does not have access to the device under threat.

Sources consulted:
- `CLAUDE.md` — operator identity, P2P link to EAST, secrets management posture, `No gh CLI` rule.
- `docs/adr/ADR-010-sealed-cask-deployment.md` — Soul Vessel pipeline, Binding Rune, deploy-key references.
- `docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md` — ML-DSA-65 signing key for `config.*` topics, hard-condition #4 (HSM + quarterly rotation), Phase 13 evidence.
- `scripts/verify-gpl-boundary.sh` — recently patched; cited for licensing hygiene context.
- `docs/security/k8s-threat-model-2026-05-06.md` — structural template (scope, STRIDE-by-component, recommendations, provenance) followed here.
- `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` — §SEN4, §BM5, §BM7 are the triggering findings.

No live shell on the workstation, no `fdesetup status`, no `ssh-add -L`, no `csrutil status`, no `defaults read com.apple.alf globalstate`, no GitHub `Security log` review. **Each of those would resolve one or more UNVERIFIED entries above and is the natural next step for an operator-side follow-on.**

This document supersedes the silent acceptance of `single-operator GAP — document accepted-gap` lines in the matrix family for the workstation specifically. It does **not** discharge those lines for the kingdom-wide PS family, IR family, or business-continuity controls — those remain open per `01-scrutiny`.

---

## 9. Hand-off

- **MoatGhost / matrix family:** downgrade workstation-related MAPPED claims to PARTIAL until R1, R2, R3, R5 land. Reference this document by path in those entries.
- **BlackMage:** this surface is now openly mapped. Run an internal red-team exercise specifically targeting the four highest-leverage vectors in §5 (phishing/GitHub takeover, ssh-agent abuse, dev-tool supply chain, operator-paste). LICH-013 candidate.
- **Architect:** R3 + R10 require ADR amendments to ADR-010 and ADR-043. File those as their own work.
- **Operator (Stevie):** R1, R2, R5 are within personal scope and can land in a single afternoon. R4, R6, R7, R8, R9 are runbook + habit work that takes longer to harden but starts immediately.

The keystone is identified. The mitigations are enumerated. The next pass is execution, not documentation.
