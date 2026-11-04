<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# Decision pack, 2026-11-03: every open question, as a multiple-choice prompt

**Why this file.** The 2026-10-27 sweep counted 146 M-items and 57 numbered decisions (about 70 questions); the 2026-10-31 round table counted 115 decision markers in next.md; the ADRs carry their own open-question lists. On 2026-11-03 Stevie answered the ten blocking questions and about thirty more. This pack is the rest, grouped the way Stevie asked (infrastructure, architecture, design, code), with the process items that are his hands at the end. Each prompt explains the ask, every option, a recommendation and a second choice. Answer by prompt id (`DP-07: B`) in the answer sheet at the bottom, or in chat.

**Already answered (do not re-ask).** 2026-11-03 sitting: focus E1, staging HOLD, churn freely, firewall LAN scope, Pi-hole items, the small fixes, east hardening, UPC D6 not yet, LICH-015 east arm yes, S-LOG-HARVEST D1-D6, ADR-116 Q1-Q6, ADR-114 Q0-Q7, ADR-115 Q2/Q6/Q7. 2026-10-28: True Wire E2 (spec wins: flow actions and Anamnesis event types). 2026-10-27 S24: M-005 drift-detect deleted, M-012 ebtables stopgap, M-033/034/041/081/083/088 closed with the defaults, M-109 privacy pack D1-D10 with defaults, S-ASCEND-2 E1 option K (`pid = slot + 1`, kernel numbering). Those are listed in Appendix A only.

**Grounding.** Sweep: `docs/sweeps/OUTSTANDING-2026-10-27.md` (M-numbers below are its rows). ADR questions: each ADR's "Open questions" section. Sprint escalations: `~/tmp/next.md` older sections and the sprint logs under `references/`. Code markers: `grep TODO|FIXME|stub` over cmd, pkg, crates, ebpf. Everything was re-checked against the tree at `d2befe29` (churn/2026-11-03) on 2026-11-03.

**How to read a prompt.** *Tier* is Stevie's 2026-10-27 priority order: (a) CI green and promotion, (b) public-repo security and dated findings, (c) live-service correctness, (d) platform epics, (e) UPC and Doom. *Unblocks* names what moves once answered. Options are mutually exclusive unless the prompt says "pick any".

---

## 1. Infrastructure and security (hosts, network, keys, findings)

### DP-01 — Credential rotation after going public (M-010, tier b)
**The ask.** The repository is public. Twenty history fingerprints and 25 `.gitleaksignore` entries cover credentials that were committed while the repo was private under a "lab-only" exemption. That exemption is void now: anyone can read the history. The baselined set was: the OPNsense API key and secret, a Grafana admin password, the LXD trust password, and assorted tokens. The 2026-10-29 Pi-hole runbook credential removal and today's OPNsense removal changed the picture: the OPNsense credentials no longer unlock anything (the appliance definitions are gone), and some tokens were for stacks that never ran.
**Options.**
- **A. Rotate everything in the baselined set now, then shrink the baseline.** Every fingerprint gets a rotated secret on the live host or a note that the secret is dead; I then remove the entries and the gitleaks baseline only shrinks. Cost: an hour of your console time (Grafana, LXD, Pi-hole, tokens). Risk: none; the clean end state.
- **B. Rotate only what still unlocks something; mark the rest dead.** Grafana, LXD trust, Pi-hole admin, live tokens get rotated; OPNsense and never-deployed stacks get a "dead, nothing to rotate" line in the baseline and are removed. Cost: 20 minutes. Risk: a mis-classified secret stays live; I verify each by trying it before calling it dead.
- **C. Leave as is; rely on the exemption note.** Cost: nothing. Risk: a public history with working secrets is the thing the findings register exists to prevent; FND rows stay open.
**Recommendation: B.** Second: **A**. Unblocks: the gitleaks baseline shrink, closing the SEC SP-01 and CG-17 rows, and the "zero claimable" line in ADR-098.

### DP-02 — FND-001: how alerts leave west (M-011, tier b, overdue since 10-30) — the relay was decided 10-27 (personal Gmail short-term, Workspace later): answer only if you want to change that; otherwise it is DP-609 item 1
**The ask.** Alertmanager routes everything to a receiver named `null`. `cmd/alert-mailer` exists (dry-run mode) and ADR-116 Q3 fixed the audience (email plus self-hosted push). What is still open is the SMTP path: the mailer needs a relay it can authenticate to, or a direct-to-MX sender. Until this is answered the monitoring stack can be up and still tell nobody anything, which is the week's silent-failure pattern.
**Options.**
- **A. Gmail account with an app password as the relay.** `smtp.gmail.com:587`, password in `ALERT_MAILER_SECRETS_DIR` (0400). Cost: 10 minutes in the Google account; it is a third-party service, but one you already run the project address on. Risk: Google can rate-limit or disable app passwords; deliverability is excellent.
- **B. A relay on orion (bellis.tech already has `mail.bellis.tech` in its DNS).** Postfix on orion accepting authenticated submission from west over WireGuard, sending with SPF/DKIM for bellis.tech. Cost: an evening on orion (DNS records, DKIM key, postfix hardening) plus a new public service on a box the hardening doc says should shrink. Risk: deliverability work and one more exposed daemon.
- **C. alert-mailer sends straight to the recipient's MX, no relay.** Native, no new account. Cost: small code (MX lookup, STARTTLS). Risk: residential IP space is on every blocklist; mail to Gmail will land in spam or be refused. Not fit for paging.
- **D. Push only, no email.** The self-hosted push from ADR-116 Q3 carries everything. Cost: the push service first (not built). Risk: ADR-116 asked for email plus push precisely so one channel failing is seen by the other.
**Recommendation: A.** Second: **B** once orion's own hardening list is done. Unblocks: E1 "done = live with alerts", FND-001, the `alert-route` host probe, ADR-116's daily canary receipt.

### DP-03 — Well PostgreSQL 16 to 18 (M-071, FND-015, tier c)
**The ask.** The Well runs PostgreSQL 16. FND-015 wants 18 for `ssl_groups` (post-quantum hybrid TLS groups on the server side, S-QUANTUM-ARMOR). The runbook `runbooks/data/well-pg18-upgrade.yaml` is written and rehearsed in a scratch container; the upgrade needs a downtime window with the main stack stopped, a backup verified first, and `pg_upgrade --link`.
**Options.**
- **A. Do it inside the E1 window.** The stack is coming up from cold anyway; upgrade first, then bring compose up on 18. Cost: adds about 30 minutes to E1. Risk: two changes in one window; if E1 goes badly it is one more variable. Mitigation: the backup and the rehearsal.
- **B. A separate window after E1 is live and stable for a week.** Cost: a second attended evening; the stack is stopped once more. Risk: lowest per-change; FND-015 stays open a week longer.
- **C. Stay on 16 until Debian/Ubuntu ship 18 as their default.** Cost: nothing now. Risk: FND-015 stays open indefinitely; the PQ TLS story for the Well is incomplete.
**Recommendation: B.** Second: **A**. Unblocks: FND-015, `ssl_groups`, the Quantum Armor Well leg.

### DP-04 — sshd hybrid-only key exchange on west (M-015, E2, tier b)
**The ask.** East already accepts only post-quantum hybrid KEX (done 2026-10-27). West still accepts five classical methods, so a downgrade-capable client gets classical KEX. West is the box you reach from the laptop and the only path to east, so a mistake locks you out of both. The requirement on the client: OpenSSH 9.9 or newer (ML-KEM-768 x25519 hybrid); the macOS you SSH from needs checking (`ssh -V`). Two more clients logged in from 192.168.69.135 and .137 in October and are unidentified; name them (phone? tablet?) before going hybrid-only, since they must support it too.
**Options.**
- **A. Hybrid-only on west now, after a verified client test.** I keep a session open, apply the drop-in, you open a new connection from the laptop; if it fails I revert. Cost: five minutes attended. Risk: contained by the open session and the revert.
- **B. Hybrid-first, classical still allowed, on west.** Keeps the present state (current clients already get ML-KEM). Cost: none. Risk: the downgrade path stays; FND row stays open.
- **C. Hybrid-only on west, plus a break-glass classical sshd on a second port bound to WireGuard only.** Cost: a second sshd instance and unit. Risk: more surface to maintain; WireGuard itself is the recovery path anyway.
**Recommendation: A** (attended, with you present as for E1). Second: **B** until the laptop's OpenSSH is confirmed 9.9+. Unblocks: E2 close, ADR-105 acceptance leg.

### DP-05 — WireGuard PSK rotation cadence on west (M-014, E3)
**The ask.** The first live rotation was done on 2026-10-27 and east's stale key shredded. The weekly timer `unheaded-wg-psk-rotate.timer` is installed on west and **disabled**. Rotation briefly re-handshakes the tunnel; east is reached over it.
**Options.**
- **A. Enable the weekly timer.** The rotation is the designed cadence (ADR-105); each run is logged and alertable. Risk: a failed rotation at 03:00 leaves the tunnel down until the next attended look; the P2P link (192.168.13.0/30) still carries SSH.
- **B. Monthly.** Fewer chances to fail; longer key life. Edit `OnCalendar`.
- **C. Manual only, after each attended session.** No timer; the key ages until you remember.
**Recommendation: A.** Second: **B**. Unblocks: E3 close.

### DP-06 — Key custody: the evidence build key and Heimdall's per-node drift keys (M-061, M-201, M-053, decision 5)
**The ask.** Three keys need a home and an owner. (1) The evidence build key (ML-DSA-65 seed) signs Sealed Casks and evidence packs; the account `unheaded-buildkey` (uid 16737) exists for it, and `build.pubkey` must be published somewhere verifiers can fetch. (2) Heimdall's drift keys, one per node, sign drift reports; the unit is written, orphan today. (3) Wotan topic signing keys for `audit.`, `health.` and `alerts.critical` (E5). ADR-043 #4 wants a quarterly ceremony runbook.
**Options.**
- **A. West is the custody host.** Seeds live under the dedicated accounts on west (0400, `StateDirectory`), an offline copy on an encrypted USB you hold, `build.pubkey` committed to the repo and served from bellis.tech. Cost: one ceremony evening. Risk: west is the busiest, most exposed box; a west compromise is a key compromise, mitigated by the per-account isolation and the offline copy.
- **B. East is the custody host.** East is smaller, reachable only through west, and runs fewer third-party things. Same mechanics. Cost: same evening. Risk: east is also the second node you want to grow; custody and workload on one host again, just a quieter one.
- **C. Hardware token (YubiKey / Nitrokey) held by you; hosts get only public keys and short-lived signing sessions.** Cost: new hardware, PKCS#11 plumbing, and every signing needs you present. Risk: lowest key-theft risk; highest operational friction for a one-person project.
- **D. Keys on your laptop; hosts verify only.** Cost: none. Risk: the laptop is a personal device that travels; signing from it violates the "work devices never touch the repo" line and the hosts cannot sign evidence unattended.
**Recommendation: A** with the offline copy mandatory. Second: **B**. Unblocks: FND-018 (E4), FND-019 (E5), the Sealed Cask ceremony runbook, ADR-043 #4.

### DP-07 — Export-control notice email (M-017, tier b)
**The ask.** Publicly available encryption source code under ECCN 5D002 needs a one-time TSU notification email to BIS and NSA (EAR 740.13(e)). The notice is drafted at `docs/compliance/export-control-notice.md`; the README paragraph is in place. The Barrister's review of the PQC licensing section (§E) is the only open sub-item.
**Options.**
- **A. Send it now from your address, as drafted.** Cost: five minutes. Risk: none worth naming; it is a notification, not a licence application.
- **B. Barrister pass on §E first, then send.** Cost: one review session. Risk: a week's delay on a notice the law wants "at the time of publication", which has already passed.
- **C. Do not send; treat GPL publication as sufficient.** Risk: the EAR exemption for public-domain encryption source is conditional on the notification. Not recommended.
**Recommendation: A.** Second: **B**. Unblocks: the compliance control-matrix ITAR/EAR row.

### DP-08 — Relicense the ~290 MIT-header non-Go files (M-161)
**The ask.** The repository is GPL-3.0-or-later (LICENSE) but about 290 non-Go files (shell, YAML, Python, docs) still carry an MIT SPDX header from the early days, and LICENSE names "Steven". A mechanical pass fixes both. The one judgement call: a few files may be deliberately MIT (small snippets meant to be copied freely).
**Options.**
- **A. Relicense all 290 to GPL-3.0-or-later and fix the name; I list any file that looks deliberately permissive for you to veto.** Cost: one commit, large diff. Risk: none for first-party files.
- **B. Relicense all but a named allowlist you choose now.** Cost: you read my candidate list first.
- **C. Leave the headers; LICENSE governs.** Risk: mixed headers confuse reuse and the SBOM licence scan.
**Recommendation: A.** Second: **B**. Unblocks: the licence-scan row in the SBOM gate.

### DP-09 — journald caps and the host CIS batch (M-105, tier c)
**The ask.** Journals have no size cap today; a chatty day can rotate audit records away. The proposal was `SystemMaxUse=1G`, `RuntimeMaxUse=200M`, east first. Also in the batch: two world-writable ROCm logs on west (`chmod 0640`), AIDE, AppArmor enforce, and retiring `remediate.sh` in favour of `unheaded-remediate`. Context that changed since: the stack is down, so **the journal is the only log store right now**; west's `/var` is a 1.8 TB disk, east's is small.
**Options.**
- **A. 1G / 200M on both hosts as proposed.** Cost: trivial. Risk: with Vector down, west's journal at 1G holds roughly a week of audit records; fine once shipping works.
- **B. West 4G / 500M, east 1G / 200M.** West has the disk and is the box whose records matter most until shipping is live. Cost: same. Risk: none; the numbers can shrink later.
- **C. No cap; rely on shipping.** Risk: the current state, and the reason audit records are at risk.
**Recommendation: B.** Second: **A**. Unblocks: the journald-caps fix in the compliance profile, ADR-092 step 1, the log-caps gate's journald branch on both hosts.

### DP-10 — Enforcing mode strictness: refuse installs only, or starts too (M-106, ADR-098 Q5)
**The ask.** Audit mode is live on both hosts (`unheaded-baseline`). Enforcing mode is not built. The draft refuses `.deb` installs on a host outside its baseline (preinst gate) and never stops a running service. The question is whether enforcing should also refuse to *start* an Unheaded service on an out-of-baseline host.
**Options.**
- **A. Refuse installs only (the draft).** Availability is itself a control; a drifted host keeps serving, and the drift is a finding. Cost: smallest. Risk: a host can run indefinitely out of baseline as long as nothing is reinstalled.
- **B. Refuse installs and starts.** A drifted host cannot start Unheaded services until remediated. Cost: a flapping breaker and a soak are mandatory (both in the plan). Risk: a bad baseline rule takes the Kingdom down at the next reboot; that is the lockout scenario the plan's 7-day soak exists for.
- **C. Refuse starts for the security services only (auth, Heimdall, baseline agent), installs for all.** Cost: a per-unit list. Risk: a middle path that needs explaining in the SSP.
**Recommendation: A.** Second: **C**. Unblocks: M-106's build (typed enforce actions, preinst gate, soak, Lich campaign).

### DP-11 — east: fail2ban and the SSH port (today's Q7)
**The ask.** You said "fail2ban jail to 7375". East has no fail2ban installed and its sshd listens on 22; west is on 7375 with fail2ban. East is reachable only over the P2P link and WireGuard (its input chain accepts 22 from the link as a lockout guard). After today, east is key-only.
**Options.**
- **A. Install fail2ban on east (apt, same package as west), jail on 22, leave the port.** Cost: one package on east (a third party you already run on west). Risk: none; brute force on a link only west can reach is near zero anyway.
- **B. Move east's sshd to 7375 to match west, no fail2ban.** Cost: a drop-in and the east ruleset line. Risk: the ruleset's lockout-guard line must change at the same time; key-only already kills brute force.
- **C. Both: port 7375 and fail2ban.** Symmetry with west.
- **D. Neither.** Key-only on a private link is enough; record it as the accepted state.
**Recommendation: D**, recorded in the host file. Second: **A**. Unblocks: the "fail2ban jail" line in next.md closes either way.

### DP-12 — east: the `lxd-installer` package (today's Q8)
**The ask.** `userdel lxd` is done and the group kept. The `lxd-installer` deb (Ubuntu's stub that installs the LXD snap on first use, owns `/usr/sbin/lxd`) is still installed. East runs no containers.
**Options.**
- **A. Remove the package (`apt remove lxd-installer`), keep the group for now.** Cost: one apt command. Risk: none; nothing on east calls it.
- **B. Leave it.** Risk: a stray `lxd` invocation installs a snap you did not ask for.
**Recommendation: A.** Second: **B**.

### DP-13 — east is key-only and your laptop key is not on it (today's Q9)
**Options.**
- **A. Add the laptop's public key to east's `authorized_keys` next time you are on.** Direct access survives a west outage over the link. Cost: one line.
- **B. Keep east hop-only through west.** One door, as ADR-098 prefers. Risk: when west is down, east needs the console.
**Recommendation: B** (one door; west's own console is the break-glass). Second: **A**.

### DP-14 — The apt repository is unsigned (today's Q10, ADR-026)
**The ask.** `/var/lib/apt-repo` on west has no `InRelease`/`Release.gpg`; east installs with `trusted=yes`; the vhost's key path `/unheaded-apt.gpg` points at nothing. ADR-026 says the repo is signed. Nothing outside the lab consumes it today.
**Options.**
- **A. Sign with a dedicated repo key (reprepro `SignWith`), publish the public key at the vhost path, drop `trusted=yes` on east.** Cost: one key under a custody rule (see DP-06), one reprepro config line, re-export. Risk: none; this is what apt expects.
- **B. Keep unsigned, LAN-only, record it as the lab state.** Risk: a MITM on the LAN can feed east a deb; ADR-026 stays contradicted.
**Recommendation: A**, with the key under the same custody as DP-06. Second: **B** only until DP-06 is answered.

### DP-15 — west sshd still allows password and keyboard-interactive (today's Q14)
**The ask.** The medium findings `host-sshd:west:kbdinteractiveauthentication=no` and `permitrootlogin=no` are open. You log in to west from the laptop with a password today.
**Options.**
- **A. Put the laptop key on west, then key-only on west (as east).** Cost: one key copy, one drop-in, attended. Risk: contained (open session, revert).
- **B. Keep password login on west as your break-glass.** Risk: the medium findings stay open; fail2ban mitigates brute force.
- **C. Key-only, and `PermitRootLogin no` now regardless (nothing logs in as root).** Can be combined with A.
**Recommendation: A + C.** Second: **B + C**.

### DP-16 — (withdrawn) Overlay addressing drift
Resolved 2026-10-20 at your word: the hosts were re-addressed to the repo (west `fd00:dead:beef::1`, east `::2`, BGP Established both ways; FND-014 closed). The 11-03 next.md question 1 was stale and is removed.

### DP-17 — Kubernetes: still a deployment target? (decision 43, parks CG-24, SP-08, SP-23, B-17, K-01..K-06, A-4)
**The ask.** Eleven sweep rows and three ADRs (047, 055, 088) wait on one answer. The repo carries manifests, Helm, ArgoCD apps and a kind bring-up; no cluster runs anywhere, ADR-055 says no k3s on west or east, and today the ingress controller came out. VISION.md points at small shops; ADR-115's Shop is the floor rung (one binary, one deb), and the platform runs on as many hosts as wanted, as few as one (Stevie, 2026-11-04). The plumbing doctrine says the protocol serves several faucets (K8s, standalone, edge) and the project should not pick one.
**Options.**
- **A. Lab faucet, not a target.** Keep the manifests and Helm as a demonstration that the protocol fits k8s; no CI cluster, no k8s findings in the register; the eleven rows close as "lab only". Cost: nothing. Risk: the manifests rot quietly; ADR-117 may still delete the telemetry ones.
- **B. A real target: stand up k3s on west+east (ADR-047 Phase 2), gate manifests in CI with kind.** Cost: a sprint plus two hosts' worth of CPU and memory the stacks already strain; Wotan active/active (ADR-064) then has a home. Risk: the opposite of ADR-115's direction.
- **C. Remove the Kubernetes material (manifests, Helm, ArgoCD, kind, k8s findings).** Cost: one deletion commit. Risk: the "many faucets" argument loses its k8s proof; recoverable from history.
**Recommendation: A.** Second: **C**. Unblocks: all eleven parked rows close, ADR-064 stays parked, ADR-117 Q3 answers itself.

### DP-18 — Live release runner: GitHub Actions or Jenkins (M-062)
**The ask.** Debs are built and signed by `scripts/build-all-debs.sh` and published with reprepro on west. The question is what runs that on a tag: GitHub Actions (the CI already there) or a self-hosted Jenkins (named in old plans, not in LIVE-PATHS).
**Options.**
- **A. GitHub Actions builds and signs; west pulls and publishes.** No new third party. Risk: the signing key would have to be in GitHub secrets, which contradicts DP-06; so the signing step stays on west.
- **B. Jenkins on west.** A new Java service on the busiest host. Risk: everything the third-party policy says.
- **C. Native: a west systemd timer watches the git mirror for a signed tag and runs the build script locally.** No new third party, signing stays home, no key leaves the box. Cost: a small unit and script. Risk: builds depend on west being up (it is the apt host anyway).
**Recommendation: C.** Second: **A** for building and testing, with signing and publishing on west.

### DP-19 — Monitoring stack placement after E1: stays separate compose or folds into the main file (M-033 was closed with "kingdom-startup"; this is the follow-on from ADR-117)
**The ask.** M-033 settled that `kingdom-startup.sh` starts the monitoring stack after the main one. ADR-117 (parked) proposes one Grafana, one metrics store and one log store, which removes most of the monitoring compose file. Decide now whether E1 brings up both stacks as they are, or a trimmed set.
**Options.**
- **A. E1 brings up both stacks exactly as pinned; ADR-117 convergence is its own sprint later.** Cost: more memory on west (about 2.5 GiB of ceilings for the duplicates). Risk: none for E1.
- **B. E1 brings up the main stack plus only alert-mailer and Alertmanager from the monitoring stack; Prometheus/Loki/second Grafana stay down.** Cost: a compose profile. Risk: dashboards that lived in the second Grafana are not visible until ADR-117 ports them.
**Recommendation: A** (E1 is about proving the known state live). Second: **B**.

### DP-20 — Parked programmes: start, re-defer with a date, or drop (sweep 51, 52, 54, 56)
**The ask.** Five programmes are parked with triggers that have passed or never fire. Pick one letter per row.

| Row | Programme | S = start next | D = re-defer to Q1 2027 | X = drop (ADR records it) | Recommendation |
|---|---|---|---|---|---|
| 20.1 | Sleipnir Phase 1 (PXE scaling substrate; "deferred to Q4 2026") | | | | **D** (nothing to scale yet; second S after E1) |
| 20.2 | Yggdrasil task #65, first real overlay build | | | | **D** (needs the build key, DP-06) |
| 20.3 | NetBox on west (ADR-033) | | | | **X** (orb's LLDP page and `compliance/hosts` carry the inventory; second D) |
| 20.4 | zhend Layer 0 and Sabatons IPMI/Redfish | | | | **D** (second X for Sabatons: no BMC hardware) |
| 20.5 | ADR-064 Wotan active/active | | | | **D** until DP-17 (second X) |

### DP-21 — GCP cost alarms (ADR-058, sweep 53)
**Options.** **A.** Thirty minutes in the console now (budget alert at $10/$50, billing export off). **B.** Drop the GCP project entirely if nothing runs there. **C.** Defer. **Recommendation: B** if the project is empty, else **A**.

---

## 2. Architecture (services, stores, protocol surfaces, what stays and what goes)

### DP-201 — (answered 10-27, kept for the record) Service tokens: Ed25519 first, ML-DSA-65 by the switch rule (ADR-103 Q3)
**The ask.** ADR-103 is built: per-service Ed25519 JWTs, 0400 keys, `trust.d`, the built-in OIDC issuer, hashed `uhk_` API keys. S-QUANTUM-ARMOR made ML-DSA-65 available in `pkg/crypto/pqc`. Tokens are short-lived (15 min) and verified only inside the Kingdom, so "harvest now, decrypt later" does not apply to them the way it does to key exchange; what matters is forgery resistance during the token's life.
**Options.**
- **A. Ed25519 now, ML-DSA-65 when the JWKS verifier learns a second algorithm (dual-publish, then flip).** Cost: nothing now; a follow-on sprint item. Risk: none in the token's 15-minute life.
- **B. ML-DSA-65 from day one.** Cost: 3 KB signatures on every service call header, a verifier change, and the Go JOSE path has no standard `alg` for it yet (an Unheaded-private `alg` string). Risk: interoperability with anything outside the Kingdom; bigger headers on the gRPC path.
- **C. Hybrid: Ed25519 plus ML-DSA-65 on every token, verify both.** Cost: both of the above. Risk: twice the signing cost for no threat it addresses today.
**Recommendation: A.** Second: **C** for the `audit.` and `alerts.critical` Wotan topics only (E5), where a forged record outlives the session.

### DP-202 — (answered 10-27) API-key store: the Well, a 0600 file where there is no Well (ADR-103 Q4)
**Options.** **A.** The Well (`unheaded_ops`), hashed, with the owner-role pattern the Well already enforces; revocation is a row update. **B.** A root-owned file per service (0400) reloaded on SIGHUP; no database dependency for auth. **C.** Both: the Well is truth, a file is the fallback when the Well is down (the compliance host-binding pattern). **Recommendation: A.** Second: **C**. Note: "write operations require a database" is standing doctrine; a file-only store would be the one exception.

### DP-203 — The eight parked G9 control planes with no `cmd/` (M-186, after decision 41)
**The ask.** `docs/UNREACHED-GO-PACKAGES.md` lists eight Go packages that were designed as control planes (compliance, firewall, nfv, qos, selfheal, version, keymgmt and two helpers, plus the F6 loadbalancer plane) and never got a binary. They compile, they have tests, nothing imports them. The PQC XDP fast path that `keymgmt` would have served was removed on 2026-10-28 (M-181, below).
**Options.**
- **A. Delete all eight.** The SBOM and the test matrix shrink; history keeps them. Cost: one commit. Risk: a future sprint rewrites something that existed; cheap, since the designs are in the ADRs.
- **B. Keep `keymgmt`, delete the rest** (the 2026-10-27 default). Only matters if DP-204 restores the fast path.
- **C. Build them one at a time per the `cmd/anomaly` recipe.** Each is an M-size sprint under a plan freeze. Risk: eight more services to run, monitor and inventory, on hosts that already strain.
**Recommendation: A.** Second: **B**. Unblocks: UNREACHED-GO-PACKAGES closes; the Lich fuzz matrix drops dead targets.

### DP-204 — PQC_SIG_STATUS and the XDP post-quantum fast path: confirm the deletion (M-181, decision 40)
**The ask.** The Shield declared a PQC signature fast path (a BPF map, a check, and OPTIMISTIC/PESSIMISTIC results) with no producer and no consumer; `cmd/pqc-verifier` was an orphan. On 2026-10-28 S-TRUE-WIRE applied the sweep's default and removed it, reversibly, before you answered.
**Options.** **A.** Confirm the removal; a future PQC sprint re-introduces it with both ends. **B.** Restore it and wire OPTIMISTIC mode plus a populator (needs `keymgmt`, DP-203 B). **C.** Lift the Layer-1 wire freeze and design PQC-at-XDP properly (a sprint, under the freeze). **Recommendation: A.** Second: **C** when the PQC sprint comes.

### DP-205 — AF_XDP: product path or retire (M-187, decision 42)
**The ask.** `pkg/afxdp` wraps a Rust `libaf_xdp` for zero-copy packet I/O through CGo. The live data plane is XDP/TC with the supervised loader; nothing binds an XSK socket in any service.
**Options.** **A.** Retire: delete `pkg/afxdp` and the Rust lib, note in ADR-067's history. **B.** Keep as an experiment behind a build tag. **C.** Product path: loader, XSK bind, a real NIC test on east. **Recommendation: A.** Second: **B**.

### DP-206 — East as a node: the three approvals behind Steps 177 and 435 (M-228, S-2)
**The ask.** Mímir's Law Phases 9/11/13 were never re-run on real L2 and the DISTRIBUTED scorer leg never ran; both need east as a node, which needs: `heimdall-daemon` as root on east, XDP generic mode on `enp2s0`, and a scratch Wotan on the P2P link. You said yes today to the LICH-015 privileged arm on east under guard; this is the rest of the same door.
**Options.** **A.** Approve all three, with the chaos-guard and `upc0` netns pattern as the guard. **B.** Netns fallback on west for the Mímir re-run, east only for LICH-015. **C.** Drop Phase 18 and record ADR-043 as "single host proven". **Recommendation: A.** Second: **B**. Unblocks: W-1, W-2, W-7, K-12, the ADR-043 status contradiction.

### DP-207 — ADR-117: converge to one Grafana, VictoriaMetrics and ClickHouse (five questions, parked; "a ways off")
Pick any. **Q1 dashboards:** (A) port only the ones Grafana's usage stats show used; (B) port all. Rec A. **Q2 paging rules in Prometheus:** (A) move to the native path first (alert-mailer, warden), then retire Prometheus; (B) keep Prometheus only as the rule engine. Rec A. **Q3 k8s telemetry manifests:** follows DP-17. **Q4 sophia-eye's Prometheus:** (A) fold into the main VictoriaMetrics; (B) keep as a research store. Rec A. **Q5 retention:** (A) one number for metrics (90 d) and one for logs (90 d, audit 1 y archive) per retention-policy-v1; (B) per-source. Rec A.

### DP-208 — ADR-118: Pi-hole twice behind one front (five questions, parked; idea only per your word)
Pick any. **Q1 Docker instance placement:** (A) its own compose file under `deploy/pihole/` so it outlives the main stack; (B) in the main stack. Rec A. **Q2 sync:** (A) Teleporter export committed and imported on both; (B) repo-mounted `pihole.toml` + gravity lists. Rec A. **Q3 DNS-over-TCP/DoT on the LAN:** (A) skip the HAProxy leg until a device needs it; (B) build it now. Rec A. **Q4 ADR-116 expectation:** (A) the front is the signal, Pi-holes behind it are sources; (B) each Pi-hole its own expectation. Rec A. **Q5 query-log privacy with two instances:** (A) both ship to the relay (one ClickHouse table, two sources); (B) only the front logs. Rec A.

### DP-209 — zhend scope: four subsystems never spawned (code C-9)
**The ask.** `crates/zhend` runs gRPC only. `main.rs` has four `TODO: spawn` markers (QUIC server, Monad bridge, Li topology observer, strata collector) and the pilgrimage is single-node. CLAUDE.md and the crypto inventory credit it with a QUIC listener on rustls/aws-lc-rs that does not exist.
**Options.** **A.** Declare zhend single-node gRPC-only: drop the QUIC/gossip claims from CLAUDE.md and the CBOM, keep the crate. **B.** Finish the gossip substrate as a named sprint (freeze). **C.** Fold zhend's store into zhen-agentd and the Well and shelve the crate. **Recommendation: A** now, **B** as the Layer 0 sprint when DP-20.4 starts. Second: **C**.

### DP-210 — Two trace-collectors; the Rust one simulates its Wotan publish (code C-14, C-15)
**The ask.** `cmd/trace-collector` (Rust) mints a service token, sleeps 100 µs and reports success without sending; its FlowKey zeroes every IPv6 address, so all Monad traffic correlates into one flow. `cmd/trace-collector-go` has a native BPF loader and runs on both hosts today (16671). The services table lists one trace-collector.
**Options.** **A.** Retire the Rust collector; the Go one is canonical; the ports band and CLAUDE.md say so. **B.** Wire the Rust one's tonic client and widen FlowKey to 128-bit, retire the Go one. **C.** Keep both with a declared split (Rust ingest, Go bridge). **Recommendation: A.** Second: **C**.

### DP-211 — protocol-api `--mode=bpf` is a stub (code C-12)
**The ask.** In bpf mode the gRPC surface returns Unimplemented (until 2026-10-23 it served mock data while claiming bpf). Implementing it the comment's way means cilium/ebpf, a third party that the 2026-10-29 rule puts behind your confirm; the in-house loader has a ReadMap.
**Options.** **A.** Implement with the in-house `pkg/ebpf` loader's map reads (native). **B.** Read from heimdall/ebpf-collector over Wotan instead of touching pins. **C.** Retire `--mode=bpf`; protocol-api stays the mock/spec server. **D.** cilium/ebpf (third party). **Recommendation: A.** Second: **C**.

### DP-212 — Bootstub and the code-store substrate: can a Linux image ever fit (code C-1)
**The ask.** The bootstub works for xv6-class kernels; the Phase 2.0 capacity audit says the ROM map cannot hold a Linux image. Your 2026-07-03 line was light and DIY over vendored Linux.
**Options.** **A.** Declare the bootstub done for xv6-class kernels; the DIY tiny-kernel path (ADR-081 Unheaded Linux) is the rung, not a vendored Linux. **B.** Grow or shard the code store (multi-map paging) so a Linux image fits. **C.** Stream kernel pages on demand over Wotan. **Recommendation: A.** Second: **B** only when ADR-081's own kernel outgrows the map.

### DP-213 — Which dead xv6 syscalls come alive, and where (code C-5)
**The ask.** pipe, chdir, sbrk and open(O_CREATE) are silent no-ops in the BPF dispatch; the owned kernel has them linked and dormant. EVOLUTION-1 chose hybrid syscalls (D1 = c) but never assigned these. `malloc` rides sbrk, so userland beyond ls/cat/echo/wc cannot grow.
**Options.** **A.** Route this set through the in-kernel path (Stage 2 rungs; the dormant code becomes live). **B.** Implement sbrk only, in BPF, so malloc works; pipe/chdir stay dead. **C.** Leave them dead; EXEC-only shell is the declared scope. **D.** All in BPF (verifier budget). **Recommendation: A**, sbrk first. Second: **B**. Ties to DP-503.

---

## 3. Design (product behaviour, Zhen, naming, what a page may do)

### DP-301 — Runbook authorship policy: the eleven drafts (M-120)
**The ask.** Eleven runbooks are `status: draft`, `approved_by: ""`, `authored_by: claude`. ADR-024's pre-registered bar for a local model to author is `rb-author` 0.80 and overall 0.85; the 2026-10-26 hold-out scored 0.796. The runbooks are read-only diagnostic steps, and `check-runbooks.sh` enforces the manifest.
**Options.** **A.** Approve the eleven, and amend ADR-024/106 with "Claude authors, Zhen executes" as the standing policy until a local model clears the bar. **B.** Approve case by case as each is first used. **C.** Wait for a local model at 0.85 and re-run `rb-author`. **Recommendation: A.** Second: **B**. Unblocks: Champion execution of those runbooks, ADR-106's confirm queue.

### DP-302 — claudebridge: ADR-019 L2 amendment and family caps (M-125; the routers question was answered 10-27: both kept behind the Router interface)
**The ask.** `ThresholdRouter` answers "has the eval shown the local model can do this family"; `AutoRouter` answers "how heavy is this work" for the Lich drivers. ADR-019 has a Proposed "Level 2 amendment: branch-sandbox sub-mode" (2026-10-23). Family caps leave headroom for one worst-case request.
**Options.** **Routers:** (A) keep both, distinct questions; (B) fold into one. Rec A. **L2 amendment:** (A) accept (sandboxed branch for Champion edits); (B) reject, keep L2 as read-only. Rec A. **Caps:** (A) keep one-worst-case headroom; (B) double it. Rec A.

### DP-303 — First unattended overnight Zhen run and its caps (M-126, ADR-106 Q2)
**Options.** **When:** (A) after E1 is live with alerts; (B) now. Rec A. **Cap:** (A) `-daily-cap-usd 5`, `max_tokens 16000` (the defaults); (B) $2; (C) $10. Rec A. **Env:** (A) the confirm queue on, no auto-approve beyond `unheaded-baseline` and chaos-controller; (B) wider auto-approve. Rec A.

### DP-304 — One naming sitting with Lore (M-230, ADR-115 Q6, ADR-113 name, the warden)
**The ask.** Three things carry placeholders: "Unheaded Shop" (ADR-115), the third-party verdicts service (ADR-113), and the out-of-stack warden (ADR-116). Each ADR says "Lore decides" or "name first". (unheadnix is settled: no rename or ceremony, 2026-10-28.)
**Options.** **A.** One Lore sitting names all three, and the find-replace lands in one commit. **B.** Name each when its sprint starts. **C.** Keep placeholders until the freeze lifts. **Recommendation: A.** Second: **B**.

### DP-305 — Doom stays in the repo (sweep 50)
**Options.** **A.** Confirm with one line in CLAUDE.md: Doom stays, lowest priority, E15 only in idle windows. **B.** Move it out before the next release. **Recommendation: A.**

### DP-306 — The dashboard's restart endpoint (code C-13)
**The ask.** `POST /api/v1/services/restart/{name}` answers 501 (it used to fake a 202). The dashboard is a human-only live view; the kanban board is the steering wheel.
**Options.** **A.** Delete the route and any UI control. **B.** Keep the 501 as a documented tombstone. **C.** Proxy to Akira `--remediate` behind RBAC (makes the dashboard a control surface). **Recommendation: A.** Second: **B**.

### DP-307 — upc-tty-bridge: the heartbeat poller that injects fake lines (code C-16)
**Options.** **A.** Delete the poller; push ingest is the shipped design; drop the "skeleton" label. **B.** Implement a real TTY_MAP pull in the bridge (needs CAP_BPF on its nologin user). **C.** Wotan topic subscribe (Phase 3+). **Recommendation: A.** Second: **C**.

### DP-308 — Forge: the placeholder GGUF writer and the corpus flags (code C-20, C-21)
**The ask.** `scripts/zlora-to-gguf.py` hand-writes a GGUF header with Mistral-7B's GQA baked in; the forge moved to Gemma-4 E2B, so trained LoRAs cannot be served. `build-coding-corpus.py` advertises `--include-stackoverflow` and `--include-wikipedia` that do nothing; both corpora are CC BY-SA, the project is GPL and never trains on Claude output.
**Options.** **GGUF:** (A) a proper GGUF LoRA writer in-tree, native; (B) llama.cpp's converter (third party, needs confirm); (C) retire the script with the Mistral path; (D) zhenai-forge emits GGUF from Rust. Rec D, second A. **Corpus:** (A) Unheaded-only, remove the flags; (B) add SO/Wikipedia with a licence manifest (share-alike on any released weights); (C) permissively licensed external code only. Rec A, second C.

### DP-309 — Zhen "teach": a model-reachable write into the retrieval corpus (code C-22)
**Options.** **A.** Remove the endpoint and its UI affordance (the Flask UI is retiring anyway, ADR-114 Q7). **B.** Route "teach" through the Champion confirm queue so you approve each addition. **C.** Wire it to write under `~/.config/cs/sources/zhen-taught/` with a redaction gate. **Recommendation: B** (if teach is wanted at all), else **A**. Second: **A**.

### DP-310 — ADR-115 deferred questions (Q1, Q3, Q4, Q5; you deferred them with the batch; "a ways off")
Pick any, or leave until the freeze lifts. **Q1 the protocol in Shop or from Studio:** (A) Studio first, Shop is hosting hardening without Monad; (B) Shop ships the protocol (the moat on every box). Rec A, second B. **Q3 first app runtimes:** (A) static sites and Go binaries, then containers; (B) PHP/Node first (what shops run). Rec B for the audience, second A for preflight simplicity. **Q4 packaging:** (A) one deb on stock Ubuntu/Debian first, Yggdrasil image later; (B) Yggdrasil image only. Rec A. **Q5 upload malware scanning:** (A) first-party signatures only (hash/KEV match), no scanner; (B) none; (C) third-party scanner (sitting). Rec A.

---

## 1b. Infrastructure, continued: the ADR-098 host-hardening batch and the firewall

### DP-22 — Firewall default-deny rollout: east first, then west (ADR-098 Q2, FND firewall-inbound-deny, overdue)
**The ask.** Both hosts run INPUT policy ACCEPT; the `inet unheaded` table is absent on both. The rulesets are written (`deploy/nftables/east.nft`, `west.nft` with today's forward chain), `nft -c` clean, and `scripts/host/nft-apply-confirm.sh` arms a 120 s rollback before applying. Physical access to both hosts is the recovery path. You decided the LAN scope today (ssh 7375, 80/443, DNS 53, dashboards, apt 18888; LXD 8443 laptop only). What is left is the go and the order.
**Options.** **A.** East first with commit-confirm in an attended window, then west the same evening. **B.** East only now; west after E1 is live (its stack ports are then verifiable). **C.** Wait for E1 entirely. **Recommendation: B.** Second: **A**. Unblocks: three high/medium findings on each host; any internet exposure.

### DP-23 — Internal links from plaintext to TLS: when (ADR-098 Q5, FND-017) — largely decided: "TLS Wotan" is inside the E1 stack-up you chose on 11-03; this prompt only confirms the compose/unit scope
**The ask.** S-IRON-GATE Phase 8 code is merged: strict TLS 1.3 with hybrid groups on every listener, per-service dev certs, no plaintext fallback. Compose and host units still run plaintext. E1 generates the certs anyway.
**Options.** **A.** Turn TLS on as part of E1 (`UNHEADED_TLS_ENABLED=true` in the compose env and the host units that have the `Environment=` line already). **B.** E1 plaintext first, TLS the week after. **Recommendation: A** (E1 generates the certs; two bring-ups cost more than one). Second: **B**.

### DP-24 — PGDG apt repository and PostgreSQL 16 packages on west (ADR-098 Q4, FND-010; held for the third-party sitting)
**The ask.** The Well backup, drill and offsite jobs run as root through the Docker socket because west has no `pg_dump` outside the container. PGDG packages would give the `well_backup` account its own client and the drill an unprivileged throwaway cluster. A new apt source is a third-party item, and the plan tied it to your Ubuntu 26.04 upgrade.
**Options.** **A.** Add PGDG at the OS upgrade, as planned. **B.** Use Ubuntu's own `postgresql-client-16` package now (no new source) for backups; keep the drill in a container. **C.** Keep the socket path, accept FND-010. **Recommendation: B.** Second: **A**.

### DP-25 — The ADR-098 low-finding batch on the hosts (Q6-Q14, Q17-Q19): one letter per row
These are the CIS items the audit keeps reporting. Each is small; most need only your answer. "Accept" means a recorded acceptance with the reason, which closes the finding honestly.

| Row | Item | Option A | Option B | Rec. |
|---|---|---|---|---|
| 25.1 | Journal the 2026-10-01 lab fixes through `unheaded-remediate -record-existing` (no file changes expected) | Do it in the next host window | Leave the retired `remediate.sh` as the record | **A** |
| 25.2 | USB storage: does anyone plug USB disks into west or east? | Blacklist `usb-storage` on both | Accept on west (your offline key copy, DP-06), blacklist on east | **B** |
| 25.3 | `nodev,nosuid` on `/var` (Docker and LXD storage live there) | Test on east with a live remount, then fstab on both | Accept as is | **A** |
| 25.4 | `noexec` on west's `/tmp` (7.2 G tmpfs; Go and cargo use it) | Set GOTMPDIR/TMPDIR under `/var/tmp`, then `noexec` | Accept | **A** |
| 25.5 | West's graphical desktop (xserver, graphical.target; no display manager runs) | Purge X and set `multi-user.target` (orion got the same today) | Keep | **A** |
| 25.6 | ISP IPv6 on west: SLAAC from router advertisements | Keep SLAAC; the firewall is the compensating control (DP-22) | `accept_ra=0`, lose public IPv6 | **A** |
| 25.7 | GPU stack: torch 2.6.0 with two CVEs; move to ROCm + torch 2.14.1 and a training smoke, regenerate the pip freeze | Schedule a forge evening after E1 | Keep accepting in `python-audit-accepted.txt` | **A** |
| 25.8 | `UMASK 027` for interactive users (check bind-mount workflows first) | 027 after a check | Keep 022 | **A** |
| 25.9 | Restrict `su` with `pam_wheel` (sudo unaffected) | Restrict (empty sugroup) | Accept | **A** |
| 25.10 | AppArmor `unconfined` userns profiles (east 74, west 165, Ubuntu's defaults) | Accept with reason | Complain-mode one by one | **A** |
| 25.11 | CIS L2 separate mounts on east (/home, /var/log, /var/log/audit, /var/tmp; no free disk) | Accept with reason | Shrink `/var` | **A** |
| 25.12 | Wired LAN for west, radio off (lockout risk high; LAN DNS rides `wlp17s0`) | Accept wireless | Run a cable when convenient, then switch | **A** now, **B** when a cable is there |
| 25.13 | Root-on-west SSH key to east so the weekly PSK timer can run unattended (a trust expansion) | Authorize the key, scoped to the rotate command (`command=` in authorized_keys), enable the timer | Keep rotation manual | **A** |

### DP-26 — GCP cost alarms, the sub-questions (ADR-058 Q1-Q4; only if DP-21 is A)
Pick any. **Q1 billable API audit before alarms:** (A) yes, enumerate first; (B) alarms on the whole project only. Rec A. **Q2 IAM principal review first:** (A) yes; (B) proceed. Rec A. **Q3 WAF/Cloud Armor:** (A) none exists; file a follow-on only if bellis.tech moves to GCP; (B) add Cloud Armor. Rec A. **Q4 scope:** (A) project-level; (B) org-level. Rec A.

---

## 2b. Architecture, continued (from the ADRs)

### DP-214 — ADR-064 Wotan active/active on Kubernetes with hashicorp/raft (held for the sitting; follows DP-17)
**Options.** **A.** Park until DP-17 and the third-party sitting (raft is a new module). **B.** Accept and start Phase 0 (3-pod kind cluster) now. **C.** Supersede: active/passive with Akira failover (ADR-035) is the design for two hosts; a third node would reopen it. **Recommendation: C** if DP-17 is A or C; else **A**.

### DP-215 — Unheaded Linux: kernel language and ABI target (ADR-081 Q1, Q3; Q2 is overtaken, the kernel is 100 % ours)
**Options.** **Language:** (A) C through the proven RV32I to MBC pipeline, Rust only for isolated safety-critical pieces later; (B) Rust now. Rec A. **ABI:** (A) a minimal own ABI first, a Linux-ABI subset only where a program needs it (EVOLUTION-1 D1 direction); (B) a Linux subset from the start; (C) full Linux ABI. Rec A.

### DP-216 — Retrieval architecture (ADR-056 Q2-Q4, ADR-057 Q1, Q3)
Pick any. **Embedding service:** (A) keep the Python sidecar; (B) a Go `cmd/zhen-embed` on onnxruntime-go (new third party, sitting); (C) Go with a native ONNX-free path (llama-server embeddings endpoint, already running). Rec C, second A. **Shard refresh:** (A) refresh on commit for source, snapshot for dumps; (B) all snapshot. Rec A. **Federated timeout:** (A) 2 s per backend, configurable; (B) one global deadline. Rec A. **Structural (call-graph) queries:** (A) separate tools (`code_callers`, `code_definition`), cosine retrieval stays simple; (B) a retriever mode. Rec A. **Multi-repo sharding:** (A) stay flat until it matters; (B) shard by package now. Rec A.

### DP-217 — ADR-113 third-party verdicts service: the thirteen questions (held for the third-party sitting; listed so the sitting has its agenda)
Not for today. The sitting's agenda, with the pack's recommendations: name (Lore); schema in `unheaded_config` unless isolation needs a database (Erda); the four verdicts plus a "replace by" date; CI gate report-only first, canonical = the signed JSON export; HTTP + Wotan publish, `/metrics` open like every other service; writes always audited, debug adds the read trail, SIGHUP toggle; operator proposes, admin decides; agents query via MCP read-only: yes; one source of truth, CycloneDX primary with SPDX rendering, retire `sbom/SBOM.md`; completeness gate after a report-only week; west and east first, unknown package = finding; OSV + KEV, VEX by Stevie only; own timer, email and dashboard.

### DP-218 — Host-a / host-b per-host stacks and host networking (ADR-098 Q15, FND-009 D11)
**The ask.** The two per-host compose stacks never started; 13 host-a services have no image in the repo; shield and unheaded-daemon run on host networking there. Today the firewall VMs went. Cuirass and the main stack are the deployment that works.
**Options.** **A.** Retire both stacks (delete `docker/hosts/`), FND-009 closes to the two routing containers. **B.** Keep as design records, assign uids to the 13 (busy work). **Recommendation: A.** Second: **B**.

---

## 3b. Design, continued (from the ADRs)

### DP-311 — Zhen CLI finishing touches (ADR-059 Q1-Q3)
Pick any. **Tab completion for slash commands:** (A) yes, stdlib-only (the line editor is already native since 10-27); (B) skip. Rec A. **Memory-recall display in the terminal:** (A) one `[memory hit: 0.94]` line above the answer, `--no-memory-recall` to hide; (B) nothing. Rec A. **Session log on exit:** (A) persist to `zhen_conversations` by default with `--no-persist`; (B) opt-in. Rec A.

### DP-312 — The Claude bridge's guard rails (ADR-104 Q3-Q5)
Pick any. **Task budget default:** (A) off until measured; (B) set 20000 now. Rec A. **`-route auto` and redaction:** (A) refuse to escalate any prompt the capture-redaction patterns would have matched (nothing that looks like a secret leaves the Kingdom); (B) the explicit `-local-only` marker is enough. Rec A. **Standard `-check` commands:** (A) ship three (Go snippet compiles, JSON parses, cited file exists); (B) operator-supplied only. Rec A.

### DP-313 — Cross-shard reranker (ADR-056 Q1)
**Options.** **A.** No reranker; trust-priority then cosine (today). **B.** A cross-encoder (bge-reranker-base), one more model on the GPU lease. **Recommendation: A** until a measured quality gap says otherwise. Second: **B**.

### DP-314 — Index Doom code for retrieval (ADR-057 Q4)
**Options.** **A.** Index `crates/doom-runner/` (ours), exclude `doom/doomgeneric/` (GPL-isolated third party). **B.** Exclude both. **Recommendation: A.**

---

## 4. Code (deletions, dependencies, gates, markers in the tree)

### DP-401 — Delete words (M-168): one letter per row
| Row | Candidate | Keep | Delete | Rec. |
|---|---|---|---|---|
| 401.1 | `pkg/waf` (7 packages, kept only because `tests/s77` asserts two files exist) | | amend s77's two assertions | **Delete** |
| 401.2 | `lxd/containers/*.yaml` (39 definitions superseded by Cuirass; Pi-hole runs from its own runbook, not from these) | | | **Delete** (keep `lxd/README.md` as the record) |
| 401.3 | `docker/hosts/host-a`, `host-b` stacks (never started; see DP-218) | | | **Delete** |
| 401.4 | `internal/bpfmap` + `pkg/ebpf/maploader` (parked F5; Phase 13 used the other loader) | | | **Delete** |
| 401.5 | `cmd/ebpf-exporter` (orphan, one of cilium/ebpf's eight importers) | | | **Delete** |
| 401.6 | `scripts/init-project.sh` (56 template TODOs, a scaffold generator nothing calls) | | | **Delete** |

### DP-402 — ADR-090 total source sweep: scope questions (Q1-Q3)
**Q1 vendored code stays skipped** (`llama.cpp/`, `crates/xv6-mbc/upstream/`): (A) skip both; (B) include xv6 upstream now that the kernel is ours (it is a record, not live code). Rec A. **Q2 docs/ (345 K lines):** (A) a separate docs sweep as the Librarian epic, after the freeze; (B) out of scope for good. Rec A. **Q3 the 28 ignored zhenai-forge tests:** (A) delete them; the Gemma-4 GGUF they need is the forge's own acquisition and the tests assert a model the forge no longer targets; (B) restore by re-acquiring the GGUF. Rec A, second B.

### DP-403 — Dependency register thresholds and the four named modules (ADR-096 Q1-Q4, M-162; held for the sitting, listed for its agenda)
**Thresholds:** (A) age 3 years without a release = review; importers ≥ 5 files = "own or record" decision required; any reachable CVE = finding. Rec A. **`gopkg.in/yaml.v3` (archived, 40 files):** (A) `go.yaml.in/yaml/v3` import-path swap (same code, the yaml org's maintained home; v4 is already indirect in go.mod); (B) `sigs.k8s.io/yaml`; (C) `goccy/go-yaml`. Rec A. **`cilium/ebpf` (8 files, one is the orphan exporter):** (A) record (CNCF anchored), own later; (B) own now (big). Rec A. **`yuin/goldmark` (only `cmd/wiki-server`, orphan):** (A) retire with wiki-server; (B) full read. Rec A. **`lib/pq` (13 files, maintenance mode):** (A) keep and record; (B) own a wire client. Rec A.

### DP-404 — Outside-module admissions awaiting your signature (ADR-004: M-163, ADR-104 Q1, ADR-105 Q5; held for the sitting)
**`aws-lc-rs`/`aws-lc-sys`:** (A) admit; it is rustls' only ML-KEM provider. Rec A. **`anthropic-sdk-go` (v1.82.0, +18 compiled modules):** (A) admit with an exit note: replace with a stdlib client (like `pkg/mcpclient`) in a later sprint; (B) admit for good; (C) replace now. Rec A, second C.

### DP-405 — ADR-095 slow-roll review items (Q1-Q3)
**Dependabot for Actions SHA pins:** (A) on, `github-actions` ecosystem only, 14-day cooldown (ADR-102 already decided this for base images); (B) hand-pinned, check-tool-freshness nags. Rec A. **golangci-lint 30/5 thresholds and Rust vetting grain:** (A) keep thresholds; vet `crates/zhend` per workspace; (B) per crate. Rec A. **ADR-004 commercial wording ("selling self-hosted infra undermines the brand"):** (A) amend to the community-first doctrine; (B) leave. Rec A.

### DP-406 — Rust binaries and ADR-111's conventions (ADR-111 Q1)
**Options.** **A.** A shared `usage` module in `crates/monad-mbc` (native, matches `pkg/cmdline`). **B.** `clap` (already a dependency of doom-runner and zhend). **Recommendation: B**, since clap is already in the tree and `--version`/`--help` conventions are its defaults; second **A**.

### DP-407 — Schema placeholders without a Rust map (code C-17)
**Options.** **A.** Delete the four Go placeholder families until a Rust map exists (the package's own rule). **B.** Keep and make them normative in the RFC drafts. **Recommendation: A.**

### DP-408 — latency-probe keys its map with trace-id/pid/tid under 5-tuple field names (code C-18)
**Options.** **A.** A dedicated trace-keyed map type mirrored in bpfschema. **B.** Resolve the real socket in the kprobe (verifier cost). **C.** Document the overload as the ABI. **Recommendation: A.** Second: **C**.

### DP-409 — phase12.rs: three feature-gated empty MMU hooks (code C-7)
**Options.** **A.** Delete the Option B/C stubs and the dead feature flags; Option A lives in `scheduler_context_switch`. **B.** Keep as documentation. **Recommendation: A.**

### DP-410 — mTLS authenticator: half-wired (code C-11)
**Options.** **A.** Delete it; service tokens over TLS are the chosen S2S auth (ADR-103). **B.** Promote and wire it as a first-class authenticator. **C.** Keep dormant, fix the header. **Recommendation: A.** Second: **C**.

### DP-411 — PQC algorithm registry stubs: SLH-DSA, FN-DSA, HQC (code C-10)
**Options.** **A.** Implement SLH-DSA through circl (already admitted; it ships SLH-DSA), drop FN-DSA from the registry (floating point, never in BPF), keep HQC reserved. **B.** Keep all three as reserved stubs. **Recommendation: A**, which also makes the Sovereign 2-of-3 tier reachable. Second: **B**.

### DP-412 — Zhen HbH option type 0x3E in the experimental range (code C-8)
**Options.** **A.** An Unheaded-private option registry in an ADR beside the Monad option type; no IANA ask. **B.** Internet-Draft and IANA allocation (RFC Editor). **C.** Stay experimental. **Recommendation: A** now, **B** with the datatracker upload (DP-606). Second: **C**.

### DP-413 — SYS_pause doubles as yield (code C-4)
**Options.** **A.** Mint `SYS_yield` in the ADR-078 registry now; `pause` keeps its meaning. **B.** Keep the overload and document it as the ABI. **C.** Timer preemption in the BPF scheduler. **Recommendation: A.** Second: **B**.

### DP-414 — Bootstub `.rodata` staging and the hard-coded RV text base (code C-2, C-3)
**`.rodata`:** (A) the loader stages only the translator's `.data` sidecar (translator is the source of truth); (B) linker keeps `.rodata` in text, translator stops emitting the sibling. Rec A. **RV text base:** (A) carry the base in BootParams so kernel and loader agree; (B) derive from the ELF section headers at load; (C) one constant per flow. Rec A.

### DP-415 — Approve the trivy-action re-pin (M-248)
**Options.** **A.** Re-pin `6abccf398f78` to `8f28faec2651` in docker.yml and security.yml. **B.** Leave. **Recommendation: A.**

---

## 5. UPC and protocol

### DP-501 — EVOLUTION-1 D1 reach: the migration ladder is self-funding (M-221)
**The ask.** The verifier delta is negative (ON 911,319 vs OFF 926,542 instructions): moving syscalls in-kernel freed verifier budget rather than spending it. EVOLUTION-1's own rule says a self-funding ladder pushes toward (a), the full in-kernel syscall set.
**Options.** **A.** Re-read D1 as (a): plan Stage 2 rungs (sbrk, pipe, kill, mkdir, unlink, link, chdir, sleep) in-kernel. **B.** Keep D1 = (c) hybrid, rung by rung on measurement. **C.** Re-evaluate after rung 2 (uptime). **Recommendation: A** (it also answers DP-213). Second: **B**.

### DP-502 — FS writes: in-BPF or kernel-side (M-226, sweep 48)
**Options.** **A.** Kernel-side via Stage 2 (the route the ladder funds). **B.** In-BPF (92.7 % of the wall budget). **Recommendation: A.**

### DP-503 — FUZIX-level syscall stubs that lie (code C-6)
**The ask.** About twenty syscalls return fabricated success in both interpreters (open gives fd 3, stat a 4096-byte file, pipe fds with no buffer, uid 0). Doom and the FUZIX ports depend on the permissive answers.
**Options.** **A.** Gate the permissive set behind a cargo feature: ascend builds answer honest `-ENOSYS`, Doom builds stay permissive. **B.** Honest `-ENOSYS` everywhere (Doom breaks until fixed). **C.** Keep the fakes. **D.** Implement a minimal fd table and ramdisk-backed open/stat. **Recommendation: A** now, **D** as a Stage 2 rung. Second: **C**.

### DP-504 — DOT4 ISA sprint, interpreter-cost gates, scorer on real traffic (M-229, sweep 47)
**Options.** **A.** Accept gates #1(a)/#2 as interpreter cost, retrain the scorer on steel-echo flows, no DOT4 sprint. **B.** Fund DOT4. **C.** Neither; leave the synthetic scorer. **Recommendation: A.** Second: **C**.

### DP-505 — One attended UPC run: browser xterm end to end, and the Lich live local-model e2e (M-231)
**Options.** **A.** One attended evening after E1 covers both. **B.** Skip; the console works over SSH. **Recommendation: A.**

### DP-506 — CLAUDE.md's mission: is compute a first-class faucet or the research track (ADR-093 Q1)
**Options.** **A.** Research track that feeds the platform (VISION points at shops; UPC is the lab). **B.** First-class faucet beside K8s, standalone and edge. **Recommendation: A** in CLAUDE.md, with the plumbing doctrine unchanged. Second: **B**.

---

## 6. Process, sign-offs and your hands (minutes each; no design in them)

### DP-601 — Staging promotion timing (M-008; you said HOLD on 11-03)
**Options.** **A.** Batch 1 (S-CLEAN-SLATE) to staging the week E1 is live; staging to main with a signed tag after one clean week. **B.** Hold until the third-party sitting. **C.** Promote now. **Recommendation: A.**

### DP-602 — GitHub settings (M-016): default branch develop, protect main, secret scanning and push protection on
**Options.** **A.** All four now (ten minutes in the repo settings; I record them in `BRANCH_PROTECTION.md`). **B.** Secret scanning and push protection now, the branch items with DP-601. **Recommendation: A.**

### DP-603 — Accept the built ADRs (M-148, plus the six the agent found): one letter per row
Accept as built (A), accept after live proof at E1 (E), or amend first (M).

| ADR | What it is | Live proof needed? | Rec. |
|---|---|---|---|
| 092 | var/log discipline | step 7 (separate /var/log on west) open, see DP-25 | **A** with step 7 noted |
| 093 | What shape is this thing | depends on DP-506 | **A** after DP-506 |
| 094 | Metrics stack | no | **A** |
| 097 | Crosswalk | no | **A** |
| 099 | Remediation as code | enforcing flip after soak (DP-10) | **A**, flip later |
| 101 | Migration ledger | no | **A** |
| 103 | Service auth | AUTH_ENABLED rollout is E1's host windows | **E** |
| 104 | Claude bridge | live since 10-23 | **A** |
| 105 | PQ hybrid KEX + crypto inventory | E1-E5 legs (DP-03..06) | **A**, legs tracked |
| 106 | Two brains, one gate | migrations + first night | **E** |
| 107 | Chaos envelope | unit live on both hosts | **A** |
| 108 | MBC cartridges | gates accepted 10-28 | **A** |
| 111 | Command-line conventions | no | **A** |

### DP-604 — Ratify retention policy v1 and sign the three attestations (M-074, M-100)
**Options.** **A.** Read `docs/policy/retention-policy-v1-2026-05-06.md` and the three attestation drafts (security-policy review, IR-plan review, access review), sign with a signed commit via `scripts/attest.sh` (or whatever the drafts name). **B.** Ratify retention now, attestations after E1. **Recommendation: A** (an hour; "zero claimable" depends on it).

### DP-605 — Live apply of Well migrations 030-043 (ADR-106 Q1, M-070)
**Options.** **A.** Inside the E1 window, rehearsed script, rollback ready. **B.** Separate window. **Recommendation: A.**

### DP-606 — IETF: upload the six -00 drafts and record the Track call (M-147)
**Options.** **A.** Upload now (browser, your account), record URLs in `ietf-submission/`. **B.** After ADR-108 acceptance (DP-603). **C.** Not yet. **Recommendation: B** (same evening as the acceptances).

### DP-607 — The first unattended Zhen night (ADR-106 Q2): the four sub-answers
**Night:** (A) the first night after E1 is live with alerts. **Claude leg:** (A) `fake` first night, `-claude live` cap $3 the second. **Card filter:** (A) only cards owned by `zhen` with a ```zhen block. **Recording:** (A) `-audit-well` (needs DP-605). Rec all A.

### DP-608 — LoRA window and the Claude-text flag (ADR-106 Q4, Q5)
**Flag:** stays OFF (Anthropic's answer governs; no decision needed). **Window:** (A) when the trigger fires (≥ 300 SFT examples across ≥ 3 families), with the `answer_start` fix first; (B) a dated attempt regardless. Rec A.

### DP-609 — Your hands, in order (each a few minutes)
1. Gmail app password for alert-mailer (the relay was decided 10-27), into `ALERT_MAILER_SECRETS_DIR`.
2. Router DHCP DNS to 192.168.69.184 (Pi-hole).
3. Pi-hole admin password rotation.
4. S-LOG-HARVEST D7: the LAN device list (model, IP, syslog yes/no).
5. Lich crash artifact: `gh run download 41039826431 -n lich-crash-fuzz_mbc_execute -D lich-crash` from the laptop.
6. Be present for E1 (keys/certs, trust.d, migrations, TLS Wotan, compose up, alerts live), DP-22 east firewall, DP-04 west sshd.
7. At the next west reboot: `journalctl -k -b 0 | grep "RTC time"` against `uptime -s` (the 6-hour RTC drift).
8. GCP console (DP-21) if the project stays.

---

## Appendix A — answered already (for your memory; nothing to do)

| Ref | Answer, date |
|---|---|
| ADR-103 Q1-Q4 | Own `unheaded-auth` issuer; built-in IdP for the lab; Ed25519 first, ML-DSA-65 by the switch rule; API keys in the Well (0600 file where there is no Well). 2026-10-27 |
| ADR-098 Q1 alert receiver | Email now (personal Gmail short-term, Workspace later), PagerDuty later. 2026-10-27 |
| ADR-081 Q5 naming | No rename or ceremony; "Unheaded Linux" / `unheadnix` / `uhl` all stand. 2026-10-28 |
| ADR-106 Q7 routers | Both kept behind the ADR-053 `Router` interface. 2026-10-27 |
| ADR-104 Q2 confirm modes | `-confirm deny|tty|queue`, default deny. 2026-10-23 |
| S-ASCEND-2 E1 | Kernel numbering, option K (`pid = slot + 1`). 2026-10-28 |
| True Wire E2 | Spec wins for flow actions and Anamnesis event types. 2026-10-28 |
| ADR-074 Q1-Q5 | Option A + allocator A1, landed. |
| ADR-108 gates | Rate-limiter and sampler misses accepted as interpreter cost. 2026-10-28 |
| ADR-109 D6 | Not yet (flip deferred). 2026-11-03 |
| M-005, M-012, M-033, M-034, M-041, M-081, M-083, M-088, M-109 | Closed with the defaults, S24 2026-10-27 |
| ADR-114 Q0-Q7, ADR-115 Q2/Q6/Q7, ADR-116 Q1-Q6, S-LOG-HARVEST D1-D6 | 2026-11-03 sitting (in the ADRs' "Decisions 2026-11-03" sections) |

---

## 7. Sprint escalations still parked in next.md (oldest first; the sprint logs hold the detail)

### DP-701 — Re-enable the main CI workflow (FND-027, since 2026-09-28; your GitHub hands)
`ci.yml` has been `disabled_manually` since 2026-02-20 (go build/test/race/coverage, golangci-lint, go-licenses). The prerequisites were fixed weeks ago. **Options.** **A.** Actions, CI, Enable workflow, watch one run. **B.** Leave disabled. **Recommendation: A.** Same for `docker.yml`, `ebpf.yml`, `release.yml` once CI is green.

### DP-702 — Edge certificate source (ADR-098 answer 6 deferred the edge TLS finding to 2026-12-29)
**The ask.** Traefik is the edge on west; first-party links get dev certs from `scripts/gen-dev-certs.sh`; `pkg/certs/acme` is a stub that fakes success. Public exposure is LAN-only today.
**Options.** **A.** Internal CA (the deployment CA E1 creates) for the LAN edge; ACME only if a public hostname ever points at west. Delete the acme stub. **B.** Let's Encrypt via ACME now (needs a public DNS name and port 80/443 reachable, which contradicts LAN-only). **C.** Self-signed per host. **Recommendation: A.** Second: **C**.

### DP-703 — east: mask `ctrl-alt-del.target` (since 2026-10-12)
East hung for eleven days after a console Ctrl+Alt+Del. **Options.** **A.** Mask it on both hosts. **B.** Leave. **Recommendation: A.**

### DP-704 — Delete the stale 4.46 G trace-collector log on west (since 2026-10-18)
`/var/log/unheaded/trace-collector/trace-collector.log` (sparse, 95 M on disk), root-owned, from a hand launch before the unit existed. **Options.** **A.** Delete. **B.** Keep. **Recommendation: A.**

### DP-705 — Training corpora after Anthropic's "no" on Claude outputs (since 2026-10-23)
**The ask.** The flywheel's `-include-claude-text` is OFF. The grey area: most repo docs and much code are Claude co-authored; the Kingdom corpus and the coding corpus contain them. Verdict labels written by Claude are another.
**Options.** **A.** Exclude chat captures and Claude verdicts; allow committed repo code and docs (they are the project's own GPL-licensed work product, authored under your name and review). **B.** Exclude everything Claude touched (most of the repo; the corpora collapse). **C.** Ask Anthropic for a scoped written authorization for repo content. **Recommendation: A**, with **C** sent in parallel so the position is on paper. Second: **B**.

### DP-706 — wotan-client leftovers: `transport.Connect` and the never-flushed fallback queue (since 2026-10-23)
**Options.** **A.** Delete `transport.Connect` and stop buffering (the gRPC-to-HTTP fallback already covers the outage case). **B.** Finish both (retry the queue with a bound). **Recommendation: A.** Second: **B**.

### DP-707 — East deb packaging gaps (since 2026-10-23)
Pick any. **timeguru's timeline and the dashboard's compliance catalog on east:** (A) ship snapshots inside the debs; (B) point `TIMELINE_PATH` and the compliance root at a checkout on east (the git mirror is bare today). Rec A. **Unit files as conffiles:** (A) yes, so upgrades do not overwrite local edits; (B) no, drop-ins only. Rec B (drop-ins are the house pattern; edits belong in `/etc/systemd/system/<unit>.d/`).

### DP-708 — East's resolver flakes ("server misbehaving" 1 in 4 pulls, since 2026-10-23)
**Options.** **A.** Point east at Pi-hole 10.10.20.252 with public fallbacks, exactly as west is now (east reaches lxdbr0 through west). **B.** Public resolvers only on east. **Recommendation: A.**

### DP-709 — The eleven FLAGGED rows of the REAL-STEEL unreached sweep (since 2026-10-26)
`docs/sweeps/REAL-STEEL-UNREACHED.tsv`: eleven packages kept with evidence pending your read, chiefly `pkg/waf` (DP-401.1) and `pkg/secrets/store` (the only `Store` implementation). **Options.** **A.** Apply DP-401 and keep `pkg/secrets/store`; close the rest per the TSV's own evidence. **B.** Read all eleven yourself first. **Recommendation: A.**

### DP-710 — DMARC and DKIM for bellis.tech (since 2026-10-24; Namecheap DNS, your hands)
alert-mailer sends as bellis.tech through Gmail; DMARC is `p=none` and there is no DKIM record. **Options.** **A.** Enable Google DKIM, then `v=DMARC1; p=quarantine; sp=quarantine; adkim=s; aspf=s; pct=100; rua=mailto:dmarc-reports@bellis.tech`, move to `p=reject` after two clean weeks of reports. **B.** `p=reject` straight away. **Recommendation: A.** Second: **B**.

### DP-711 — steel-echo-v2 x3 left running under LXD (since 2026-10-27)
Three containers run under the LXD project `unheaded` with Cuirass stopped; nothing reconciles them. **Options.** **A.** Delete them now (`lxc delete`), Cuirass recreates on demand. **B.** Keep until the next game day. **Recommendation: A.**

### DP-712 — vor and zhen-agentd: ports and host units (Champion E4, since 2026-10-27)
vor runs ad hoc on :9876 and zhen-agentd on :20105, which collides with SophiaEye in `pkg/ports`; neither has a unit. **Options.** **A.** Assign ports from the free range in `pkg/ports`, ship two host units with the house hardening, swap off stays a unit property. **B.** Keep ad hoc until the Python UI retires. **Recommendation: A.**

### DP-713 — vLLM image choice (Iron Gate Phases 13-14; third-party sitting)
A 30 GB image, no models, runs as its own uid in the GPU compose. **Options.** **A.** Retire vLLM: llama-server is the serving path for the GPU lease. **B.** Keep the pinned image for the research stack. **Recommendation: A** (one GPU server, one path). Second: **B**.

### DP-714 — Revoke the old April Console API key and drop `ALERT_MAILER` from `~/.bashrc` (E6, your hands)
**Options.** **A.** Revoke in the Console; delete the bashrc export (the 0400 file is the home). **Recommendation: A.**

### DP-715 — TRUE-WIRE E3: the 36-row wire decisions table, and the S11 spec questions (since 2026-10-27)
**The ask.** `docs/sweeps/TRUE-WIRE-DECISIONS.md` lists 36 rows with defaults already applied in a dry run; four protocol questions from S11 ride with it: PQC-00 K1|K0 flag bits, §11.17 12-byte vs 6-byte auth value, hop-limit drop vs forward per policy, Next Header 0xFE.
**Options.** **A.** Accept all 36 defaults; answer the four spec questions at the RFC desk sitting with DP-606. **B.** Row by row. **Recommendation: A.**

### DP-716 — TRUE-WIRE E8: Shield check semantics (FND-025)
HMAC, retry and hop checks sit at XDP ingress where packets carry no Monad yet. **Options.** **(a)** Move them to hop-ebpf. **(b)** Define ingress keying. **(c)** Delete the maps. **Recommendation: (a).** Second: (c).

### DP-717 — EVOLUTION-1 Stage 2 E2 stop rule: per rung or cumulative (since 2026-10-28)
Rung 2 costs +3,986 verified instructions on its own; the ladder has freed 49,746 since rung 0; ascend sits at 868,290 of 900,000. `UPC_RUNG2` is off until you answer. **Options.** **A.** Cumulative: a rung may cost as long as the ladder stays net negative and under the ceiling. **B.** Per rung: every rung must free or be neutral. **Recommendation: A** (it is what "self-funding ladder" means; DP-501 agrees). Second: **B**.

### DP-718 — Retire `crates/upc-bootstub` (stage 1 of the frozen Boot Protocol v2, ADR-067)
**Options.** **A.** Keep: DP-212 keeps the bootstub for xv6-class kernels. **B.** Retire and amend ADR-067. **Recommendation: A.**

### DP-719 — Grimoire: retire or bump (Dependabot, 7 alerts; third-party sitting)
**Options.** **A.** Delete `tomb/grimoire/rag/` (Python), keep the threat-model and MITRE corpus, serve search through Zhenai's Go retrieval; six alerts close. **B.** Keep and bump ChromaDB, transformers, sentence-transformers. **Recommendation: A.**

### DP-720 — llama.cpp prompt-cache probe: which model (S-LOW-FRUIT E2; third-party sitting)
**Options.** **A.** qwen-coder-14b (the only GGUF kept after WAVE16). **B.** Skip the probe; TH-2026-005 stays a documented threat. **Recommendation: A** when llama-server is next up.

### DP-721 — Native trivy replacement (third-party sitting; your direction was native)
**Options.** **A.** Build it in stages: image and filesystem CVE match against OSV + KEV (kev-poller already pulls KEV), then the IaC misconfig checks that `.trivyignore` documents, trivy stays pinned until each stage replaces its use. **B.** Keep trivy pinned, record the exception. **C.** Drop image scanning. **Recommendation: A.** Second: **B**.

### DP-722 — Version bumps of admitted dependencies: do they need your confirm? (ruff 0.17.2 and modernc sqlite v1.61.0 held)
**Options.** **A.** Bumps of already-admitted modules and tools are exempt after the 14-day cooldown; only new modules need the confirm. The two held bumps go in. **B.** Every bump needs a yes. **Recommendation: A.**

### DP-723 — Vizio TV: block the AWS IoT endpoint and the shared kinesis/firehose hosts (IOC sweep follow-up)
**Options.** **A.** Leave them (shared hosts; blocking may break casting); the nine ACR denies already stand. **B.** Block. **Recommendation: A.**

---

---

## Answers recorded (sitting of 2026-11-03/04, round 2: 84 prompts answered)

Recorded verbatim-ish from the sitting; the ADRs carry matching "Decisions 2026-11-03" blocks. Ids not listed here are still open (next section).

**Infrastructure and security.** DP-01 rotate everything in the baselined set, then shrink; record the new secrets outside the repo (`~/tmp/` or `~/`) so lab access is never lost. DP-02 relay stays the 10-27 answer (Gmail short-term). DP-03 PG 18 in a separate window after E1 is stable a week. DP-04 west hybrid-only, attended, after a verified client test; identify the .135/.137 clients first. DP-05 enable the weekly PSK timer with a command-scoped root-on-west key on east. DP-06 custody: Stevie's two existing YubiKeys unlock the build-key seed at ceremonies (seed encrypted at rest, decrypted only with the token present); daemon keys (Heimdall drift, Wotan topic signing, apt repo) live on the hosts under their dedicated accounts, rotated quarterly, public halves published. DP-07 Barrister and Librarian pass on the export-control notice, then send tonight. DP-08 relicense all ~290 MIT-header files to GPL-3.0-or-later and fix "Steven"; candidates that look deliberately permissive listed for veto. DP-09 journald caps west 4G/500M, east 1G/200M; ROCm log chmod; retire remediate.sh. DP-10 enforcing refuses installs only; drift pages a finding. DP-11 east sshd moves to 7375, key auth preferred but password allowed, fail2ban on east (there is no laptop key; today's key-only on east is undone). DP-12 remove lxd-installer on east. DP-13 moot. DP-14 sign the apt repo with a host-held key under the DP-06 daemon-key rule, publish the key, drop `trusted=yes` on east. DP-15 generate an ed25519 key on the laptop next time, put it on west, then key-only; `PermitRootLogin no` now. DP-16 withdrawn (resolved 10-20). DP-17 Kubernetes is a lab component in its own repository (unheaded-k8s), not a product target; the platform scales from one host to many. DP-18 both: GitHub Actions builds and tests, west's timer signs and publishes on a signed tag. DP-19 E1 brings both stacks as pinned. DP-20 all as recommended (Sleipnir and Yggdrasil re-deferred, NetBox dropped, zhend Layer 0 and Sabatons re-deferred). DP-21/26 GCP budgets $5/$10/$20, kill switch $50; post-alpha, lower priority than the Doom move; a student Azure account may serve the same purpose later. DP-22 east first in an attended window (with the 7375 change), west after E1 is live. DP-23 all internal links on TLS at E1. DP-24 Ubuntu's postgresql-client-16 now, PGDG at the 26.04 upgrade. DP-25 all thirteen rows as recommended.

**Architecture.** DP-201/202 answered 10-27. DP-203 fold the eight G9 control planes into existing binaries as flag-gated features (compliance into unheaded-baseline/compliance-evidence, firewall/qos/nfv/failover/loadbalancer into cuirass, version into monad, keymgmt into unheaded-auth), one parked ADR per fold before any code; compliance is critical; nothing deleted. DP-204 the PQC fast path stays removed and is designed properly as parked ADR-119; no freeze lift. DP-205 retire AF_XDP if nothing Doom-related depends on it (verify first). DP-209 zhend is single-node gRPC-only; fix CLAUDE.md and the CBOM claims. DP-210 retire the Rust trace-collector; Go is canonical. DP-211 implement protocol-api's bpf mode with the in-house pkg/ebpf map reads. DP-212 bootstub is done for xv6-class kernels; the DIY kernel is the rung. DP-213 dead syscalls come alive in-kernel via the Stage 2 ladder, sbrk first. DP-214 ADR-064 superseded by ADR-035. DP-215 C via the RV32I pipeline; minimal own ABI, Linux subset only where a program needs it. DP-216 Go embedding service on llama-server's endpoint; refresh-on-commit/snapshots; 2 s per-backend deadline; call-graph queries as separate tools; flat sharding. DP-217 sitting agenda, not asked. DP-218 retire the host-a/host-b stacks (DP-401.3).

**Design.** DP-301 approve runbooks case by case as each is first needed. DP-302 accept the ADR-019 L2 amendment; keep one-worst-case cap headroom. DP-303 first unattended night after E1 is live with alerts: fake Claude leg first, live $3 the second night, zhen-owned cards with a zhen block only, `-audit-well` on. DP-304 one Lore sitting names Shop, the verdicts service and the warden. DP-305 Doom moves out of the repo: new repository unheaded/unheaded-doom (runner, demos, docs, runbooks, viewport, the three units, cmd/doom-loader, wotan-ctl's doom.go and load_mem.go, the doomgeneric port); the unheaded/DOOM fork stays the C source upstream; doom.mbc, doom.rv2mbc and the checksum stay as the UPC regression fixture. DP-306 delete the dashboard restart route and its UI control. DP-307 delete the tty-bridge heartbeat poller, drop the skeleton label. DP-308 zhenai-forge emits GGUF from Rust; the corpus is Unheaded-only, the two flags go. DP-309 teach goes through the Champion confirm queue. DP-310 ADR-115 Q1/Q3/Q4/Q5 stay deferred. DP-311 all three CLI touches. DP-312 all three guard rails. DP-313 no reranker until a measured gap. DP-314 exclude all Doom code from the retrieval index now.

**Code.** DP-401 delete pkg/waf (amend tests/s77), lxd/containers/*.yaml (keep lxd/README.md), docker/hosts/host-a and host-b, internal/bpfmap + pkg/ebpf/maploader, cmd/ebpf-exporter; scripts/init-project.sh moved to ~/tmp/projects/scripts/ (done). DP-402 skip vendored; docs sweep is the Librarian epic after the freeze; delete the 28 ignored forge tests. DP-403/404 sitting agenda. DP-405 Dependabot for Actions pins (14-day cooldown); thresholds kept, zhend vetted per workspace; amend ADR-004's wording. DP-406 clap for the Rust binaries. DP-407 delete the four bpfschema placeholders. DP-408 a dedicated trace-keyed map type mirrored in bpfschema. DP-409 delete the phase12 B/C stubs. DP-411 the PQC bar is at or above iMessage PQ3, Signal PQXDH/SPQR and the Microsoft/HKMA roadmaps (hybrid PQ key establishment everywhere, PQ signatures, phased); implement SLH-DSA via circl, drop FN-DSA from the registry, keep HQC reserved. DP-412 private HbH option registry in an ADR now, IANA with the resubmission. DP-413 mint SYS_yield in ADR-078. DP-414 loader stages only the translator's .data sidecar; the RV text base travels in BootParams. DP-415 trivy-action re-pin approved.

**UPC and protocol.** DP-501 D1 re-read as (a): Stage 2 rungs in-kernel. DP-502 FS writes kernel-side via Stage 2. DP-503 FUZIX fakes behind a cargo feature (kernel builds answer honest -ENOSYS); a real fd table is a Stage 2 rung. DP-504 gates accepted as interpreter cost; scorer retrained on steel-echo flows; no DOT4. DP-505 one attended evening after E1. DP-506 compute stays a lab research track; three ideas noted for later: a swarm of honeypots, a monitoring aid, and UPC/UHL as the engine for state enforcement, IaC and compliance.

**Process.** DP-601 staging holds until the third-party sitting. DP-602 all GitHub settings now. DP-603 accept ten ADRs now (092, 094, 097, 099, 101, 104, 105, 107, 108, 111); 103 and 106 at E1; 093 with the DP-506 wording. DP-604 read and sign retention v1 and the three attestations in one sitting before E1. DP-605 migrations 030-043 inside E1. DP-606 the -00 Internet-Drafts were uploaded long ago and expired: update and resubmit new versions. DP-607 covered by DP-303.

**Plan freeze.** Stevie, 2026-11-04: once the pack is answered, the work rolls into a series of warmonger battle plans. That instruction lifts ADR-112's freeze for exactly the plans that roll from this pack; pitch intake as parked ADRs continues.

## Answers recorded, round 3 (2026-11-04)

DP-207 ADR-117: the leanest, cleanest long-term production setup; polish rather than cut corners. Keep every dashboard worth keeping, rebuild paging rules on the native path first and then retire Prometheus, fold sophia-eye into the main VictoriaMetrics, retention per retention-policy-v1. DP-208 ADR-118: all as recommended, idea only. DP-410 delete the mTLS authenticator. DP-608 LoRA when the trigger fires, `answer_start` fix first. DP-701 enable ci.yml and watch one run, then docker, ebpf and release once green. DP-702 edge certificates: Let's Encrypt through orion's existing auto-renew, copied down; lab domain `lab.worldsoldestlobster.com`; back burner; names listed in `~/tmp/lab-certs-domains.md`. DP-703 and DP-704 neither: ctrl-alt-del stays unmasked and the stale log stays. DP-705 Stevie's position: AI-assisted repo code and docs are his work product and fine for the corpora, every commit credits Claude and Anthropic. The capture flag stays off. Anthropic's 10-26 answer still requires prior written authorization for training on Claude outputs and his follow-up is unanswered, so no training run uses repo text until that reply is on file. DP-706 delete `transport.Connect`, stop buffering. DP-707 snapshots in the debs, drop-ins only. DP-708 east resolves through Pi-hole with public fallbacks. DP-709 keep `pkg/secrets/store`, close the rest per the TSV; a WAF stays a future idea as a lowest-priority parked ADR, `pkg/waf` still leaves the repo, and any OSS WAF sits outside the repo after the sitting. DP-710 alert mail moves to bellis.tech (DKIM, DMARC and SPF configured from here), and outbound mail is closely monitored: send-only, no listening relay, a per-hour cap, every send logged, an ADR-116 expectation on volume. DP-711 delete steel-echo-v2 x3. DP-712 ports in `pkg/ports` and two host units for vor and zhen-agentd. DP-713 vLLM needs a better GPU (a used card and a small lab server someday): parked and out of the live tree until then. No record shows it ever served on west; the 2025-12-05 image predates the 2026 fixes (CVE-2026-22807, CVE-2026-27893), no published tampering was found, and nothing runs it. The image is still on west; its removal is a plan step. DP-714 both at the next Console visit. DP-715 accept all 36 defaults; the four spec questions go to the RFC desk with the I-D resubmission. DP-716 Shield checks move to hop-ebpf. DP-717 stop rule is cumulative; UPC_RUNG2 may flip. DP-718 keep upc-bootstub. DP-719 keep Grimoire and bump its libraries; Zhen is Champion and platform territory only, Lich and Grimoire stay on their own local models. DP-720 run the residue probe with qwen-coder-14b when llama-server is next up. DP-721 native scanner in stages, trivy pinned until each stage replaces it. DP-722 security first: bump anything in the SBOM with a vulnerability and patch code to fit. DP-723 do not trust the Vizio TV; done 2026-11-04: `kinesis.us-west-2.amazonaws.com`, `firehose.us-east-2.amazonaws.com`, `o401343.ingest.us.sentry.io` denied (only .130 used them; no AWS IoT host appeared in the logs). Log store: ClickHouse through E1, then a measured week shipping to both under ADR-117, Postgres favoured if close.

## DP-705 clarified (Stevie, 2026-11-04)

Two things stay separate. **Routing:** the goal is to run prompts locally when possible and fail over the harder asks to Claude (ADR-104, ADR-106). That is using the API, and it continues. **Training:** using Claude's answers to improve the local model needs written verification from Anthropic first. Stevie's request is with support and unanswered, so `-include-claude-text` stays off, captures feed no forge or LoRA run, and no training run uses repo text until the reply is on file. Stevie recognises how the same pattern could be misused and states that this project's intent is a local model that handles routine asks, with the failover kept honest.

## Answers recorded, round 4 (2026-11-04, after the plan reviews)

Two prompts rested on wrong facts from the coordinator and were re-asked. DP-402 Q3: 25 of the 28 ignored forge tests assert Gemma-4, the forge's current target, so they are not deleted. Stevie: training is unworkable on west, very little progress came through the experiments, and it goes on the back burner until a machine with enough GPU exists, to focus on tangible features. **The forge moves out to a new repository `unheaded-forge`**, tests and all, so the work is retained without digging through history. That carries DP-308 (GGUF), DP-608 (`answer_start`), the corpus scripts and the kingdom-v2/v3 LoRA finding. DP-706: `transport.Connect` is already gone and the queue has flushed since `c9927f77`, so it stays as it is. `cmd/waf` leaves with `pkg/waf`, after S-CLEAN-CUT confirms what depends on it; ADR-121 records it. DP-216 revised: the swap is deferred; the Python embedder stays, no third unit, the code index and retrieval work continues, and the embed service is parked in ADR-122. Licence: `zhend` and `zhenai-forge` move to GPL-3.0-or-later like everything else, with their README and SECURITY text.

## Still open

None. DP-206 answered 2026-11-04: approve all three on east (heimdall-daemon as root, XDP generic on enp2s0, scratch Wotan on the P2P link), guarded like the chaos-guard and upc0 netns. The sitting agenda (DP-217, DP-403, DP-404) stays for the third-party sitting.


## Answer sheet

Copy, fill, paste back (or answer in chat by id). Blank means "leave as recommended for now, not decided".

```
DP-01: __  DP-02: __  DP-03: __  DP-04: __  DP-05: __  DP-06: __  DP-07: __  DP-08: __
DP-09: __  DP-10: __  DP-11: __  DP-12: __  DP-13: __  DP-14: __  DP-15: __  DP-17: __
DP-18: __  DP-19: __  DP-20: 20.1 __ 20.2 __ 20.3 __ 20.4 __ 20.5 __  DP-21: __
DP-22: __  DP-23: __  DP-24: __  DP-25: 25.1 __ .2 __ .3 __ .4 __ .5 __ .6 __ .7 __ .8 __ .9 __ .10 __ .11 __ .12 __ .13 __
DP-26: Q1 __ Q2 __ Q3 __ Q4 __
DP-203: __  DP-204: __  DP-205: __  DP-206: __  DP-207: Q1 __ Q2 __ Q4 __ Q5 __  DP-208: Q1 __ Q2 __ Q3 __ Q4 __ Q5 __
DP-209: __  DP-210: __  DP-211: __  DP-212: __  DP-213: __  DP-214: __  DP-215: lang __ abi __
DP-216: embed __ refresh __ timeout __ structural __ sharding __  DP-217: (sitting)  DP-218: __
DP-301: __  DP-302: L2 __ caps __  DP-303: when __ cap __ env __  DP-304: __  DP-305: __  DP-306: __  DP-307: __
DP-308: gguf __ corpus __  DP-309: __  DP-310: Q1 __ Q3 __ Q4 __ Q5 __  DP-311: tab __ recall __ log __
DP-312: budget __ redaction __ checks __  DP-313: __  DP-314: __
DP-401: .1 __ .2 __ .3 __ .4 __ .5 __ .6 __  DP-402: Q1 __ Q2 __ Q3 __  DP-403: (sitting)  DP-404: (sitting)
DP-405: dependabot __ thresholds __ adr004 __  DP-406: __  DP-407: __  DP-408: __  DP-409: __  DP-410: __
DP-411: __  DP-412: __  DP-413: __  DP-414: rodata __ base __  DP-415: __
DP-501: __  DP-502: __  DP-503: __  DP-504: __  DP-505: __  DP-506: __
DP-601: __  DP-602: __  DP-603: 092 __ 093 __ 094 __ 097 __ 099 __ 101 __ 103 __ 104 __ 105 __ 106 __ 107 __ 108 __ 111 __
DP-604: __  DP-605: __  DP-606: __  DP-607: night __ claude __ filter __ record __  DP-608: __
DP-701: __  DP-702: __  DP-703: __  DP-704: __  DP-705: __  DP-706: __  DP-707: snapshots __ conffiles __
DP-708: __  DP-709: __  DP-710: __  DP-711: __  DP-712: __  DP-713: __  DP-714: __  DP-715: __
DP-716: __  DP-717: __  DP-718: __  DP-719: __  DP-720: __  DP-721: __  DP-722: __  DP-723: __
```

**Counts.** 112 prompts with options (several carry sub-rows), 4 withdrawn or already answered and kept for the record, 1 agenda block for the third-party sitting (ADR-113's thirteen questions), 8 hands items. Sources: 46 sweep rows still marked for Stevie, 90 ADR open-question blocks (47 already answered are in Appendix A), 75 next.md escalations (8 superseded), 22 decision-shaped code markers.
