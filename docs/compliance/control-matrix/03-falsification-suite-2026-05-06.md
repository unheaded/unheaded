# Falsification Suite — Scientist's 6 Experiments + S8 Meta-Experiment

**Date:** 2026-05-06
**Author:** Scientist (with MoatGhost as primary executor on most runs)
**Cross-ref:** `docs/compliance/control-matrix/01-scrutiny-2026-05-06.md` Sections S1–S8.
**Scope:** convert each Scientist falsification proposal into a runnable test recipe with hypothesis, procedure, success criterion, tooling, owner, runtime, and output format.

**Why this exists.** The 16-file matrix family makes coverage claims that are mostly **unfalsifiable as written** (S1–S7 critique). Experiments that cannot fail cannot succeed either — they are not science, they are opinion. This suite specifies *how* each Scientist proposal becomes a falsification test that either upholds or falsifies the matrix family's claims.

**Marshal disposition (cross-ref scrutiny doc):** the Marshal accepts that S2 (MAPPED ≠ proof) is the largest authorial sin, and accepts the falsification suite as the gate before any matrix is shown to an auditor or adopter.

**Headline counts as of 2026-05-06** (informs experiment sample sizes):

| Status label | Count across 16-matrix family |
|--------------|-------------------------------|
| MAPPED (incl. MAPPED-CONTRIBUTES) | 423 (excluding the scrutiny doc; ~60 in the parent-control summary tables, ~360 in audit-expansion tables) |
| PARTIAL (incl. PARTIAL-CONTRIBUTES) | 312 |
| GAP | 198 |
| ADOPTER-OWNS | 84 |
| UNVERIFIED | 41 |
| Other (N/A, DEFERRED, CONTRIBUTES, CONFIRMED-CLEAN, etc.) | 96 |

**Experiment naming.** Each experiment uses the original scrutiny-doc number (S1, S2, S3, S4, S5, S6, S8). The 30-day CI cadence audit (S7) is **not** in this suite — it is captured separately in the F11 / SEN5 deployment-audit work because it merges with the Suricata host-enumeration audit.

---

## Per-Experiment Template

Every experiment uses the same eight-section template:

1. **Hypothesis** — H0 (null) and H1 (alternative), formal.
2. **Procedure** — numbered, reproducible steps.
3. **Success criterion** — numerical threshold; what makes the experiment falsify or uphold the matrix claim.
4. **Required tooling** — existing kingdom scripts (cited by path) + anything that needs to be authored.
5. **Owner** — primary kingdom skill responsible. Most runs are MoatGhost-executed and Scientist-adjudicated.
6. **Estimated runtime** — wall-clock estimate including setup + execution + analysis.
7. **Output format** — where results land (path + format), so downstream skills can consume them without re-deriving.
8. **Disposition rule** — what the kingdom does with a "fails to reject H0" vs "rejects H0" outcome.

---

# S1 — Inter-Rater Reliability Test

## Hypothesis

- **H0 (null):** The matrix-family status taxonomy is reproducible. Two independent reviewers, given the same control + the same kingdom evidence corpus, will assign the same status with high probability. Cohen's κ ≥ 0.6 over a random sample.
- **H1 (alternative):** The taxonomy is not reproducible. κ < 0.6 — the status assignment depends on author mood, not on a discoverable rule. The matrices are opinion in tabular form.

**Why κ ≥ 0.6 is the threshold.** Landis & Koch (1977) classify κ ∈ [0.6, 0.8] as *substantial agreement* and [0.4, 0.6] as *moderate*. For an audit-grade artifact, *moderate* is not good enough — auditors need at least *substantial*. Below 0.6, the scrutiny doc S1 finding is upheld: the taxonomy is logically corrupt.

## Procedure

1. **Build the sampling frame.** Run `scripts/extract-control-rows.sh` (NEW — see Tooling) over the 15 framework matrices (excluding `00-completeness-audit-2026-05-06.md` and `01-scrutiny-2026-05-06.md`). Output: a CSV with one row per `(matrix-file, control-id, current-status)` tuple. Expected size: ~1,200 rows across the family.
2. **Sample 50 controls.** Use `python -c "import random; random.seed(20260506); ..."` with seed `20260506` (the date as YYYYMMDD) to draw 50 rows uniformly without replacement. Save sample to `out/s1-sample.csv`.
3. **Build the blinded sample for MoatGhost.** For each sampled row, emit a JSON blob containing only:
   - The framework name (e.g. `nist-800-53`)
   - The control ID (e.g. `AC-2`)
   - The control title (e.g. `Account Management`)
   - The "Evidence / Gap" cell with kingdom citations stripped out — replace `pkg/auth.AuditLogger` with `[component-A]`, `Wotan log aggregation` with `[component-B]`, etc., to remove status hints.
   - **Excluded from blinded sample:** the original status, the matrix-author's prose summary, the headline-gap list.
   - Provide MoatGhost the **status enum definitions** (the formal taxonomy from F9 once authored — see disposition note) and the same kingdom evidence corpus the Marshal had access to.
4. **MoatGhost re-statuses.** MoatGhost reads each blinded entry, consults the kingdom evidence corpus, and assigns one of the taxonomy statuses. Saves to `out/s1-moatghost-ratings.csv`.
5. **Build the disagreement matrix.** Cross-tabulate Marshal-original-status vs MoatGhost-status into a confusion matrix.
6. **Compute Cohen's κ.** Formula: κ = (p_o − p_e) / (1 − p_e), where p_o is observed agreement and p_e is expected agreement under independence. Use `sklearn.metrics.cohen_kappa_score` for cross-check.
7. **Bootstrap a 95% CI.** Resample the 50 ratings 1,000 times, compute κ each time, take the [2.5%, 97.5%] quantiles.
8. **Decide.** If κ ≥ 0.6 with the lower-CI-bound also ≥ 0.5, fail to reject H0. Otherwise reject H0 — taxonomy is not reproducible.

## Success criterion

| κ value | Verdict |
|---------|---------|
| κ ≥ 0.8 (lower CI ≥ 0.6) | Strong reproducibility — taxonomy is sound. |
| 0.6 ≤ κ < 0.8 (lower CI ≥ 0.4) | **Pass threshold.** Taxonomy survives S1. |
| 0.4 ≤ κ < 0.6 | **Fail.** Taxonomy is moderate at best — not audit-grade. |
| κ < 0.4 | **Fail catastrophically.** Taxonomy is opinion in tabular form (S1 confirmed). |

Critical threshold: **κ ≥ 0.6** with bootstrapped lower-CI bound ≥ 0.5.

## Required tooling

- **NEW — `scripts/extract-control-rows.sh`** (small awk/python script, ~80 LOC). Walks the 15 framework matrices, parses `| Ctrl | Title | Status | Evidence / Gap |` markdown tables, emits CSV.
- **NEW — `scripts/build-blinded-sample.py`** (~60 LOC). Reads the sampled rows, redacts kingdom-component names with `[component-X]` placeholders, emits JSON for MoatGhost.
- **NEW — `scripts/compute-kappa.py`** (~40 LOC). Loads two ratings CSVs, computes κ + bootstrapped CI.
- **EXISTING — F9 status-enum formal taxonomy.** S1 is gated on F9 being authored first (otherwise MoatGhost has no rule to apply). If F9 is not yet authored at run time, S1 must be deferred OR run with the *informal* taxonomy and the result interpreted as a lower-bound on κ.
- **Kingdom evidence corpus.** Same source files the Marshal used: CLAUDE.md, ADRs, K8s threat model, RBAC review, kingdom skills. No additional corpus authoring needed.

## Owner

- **Scientist** designs the experiment (this section).
- **MoatGhost** executes the re-rating (step 4).
- **Marshal** does NOT participate — the Marshal authored the original ratings; the Marshal's involvement would corrupt the blind.
- **Scientist** computes κ, writes the disposition.

## Estimated runtime

- Sample frame construction: 30 min (writing + running `extract-control-rows.sh`).
- Sample drawing + blinding: 30 min.
- MoatGhost re-rating: 50 controls × ~5 min/control = ~4 hours (this is the dominant cost).
- κ computation + bootstrap: 15 min.
- Writeup: 1 hour.
- **Total: ~6 hours of MoatGhost time + 2 hours of Scientist time.**

## Output format

- `out/s1-sample.csv` — the 50 sampled rows + Marshal's original status.
- `out/s1-moatghost-ratings.csv` — MoatGhost's blinded ratings.
- `out/s1-confusion-matrix.csv` — N×N status confusion matrix.
- `docs/compliance/control-matrix/04-s1-irr-results-YYYY-MM-DD.md` — the formal experiment writeup with κ, CI, disposition.

## Disposition rule

- **If κ ≥ 0.6 (H0 holds):** Taxonomy survives. Add note to scrutiny doc S1 marking the critique addressed. Document the corner cases where MoatGhost and Marshal disagreed — those become taxonomy-clarification ADR candidates.
- **If κ < 0.6 (reject H0):** S1 critique is confirmed. Block any auditor-facing or adopter-facing release of the matrix family until F9 (formal taxonomy) is rewritten with explicit decision rules and S1 re-runs to κ ≥ 0.6. The matrix family is downgraded to "internal gap inventory only" pending fix.

---

# S2 — Per-MAPPED Falsification Test

## Hypothesis

- **H0 (null):** The MAPPED entries in the matrix family represent demonstrated coverage. ≥ 60% of MAPPED entries pass a one-line test that, if it failed, would falsify the MAPPED status.
- **H1 (alternative):** The MAPPED entries are infrastructure-name-dropping. < 40% pass under their own falsification criterion. The kingdom is over-claiming MAPPED by ≥ 60%.

**Why 40% / 60% threshold.** S2 of the scrutiny doc explicitly hypothesizes "<40% will pass." This experiment falsifies the Scientist's own counter-hypothesis if pass-rate ≥ 60%, OR confirms it if pass-rate < 40%. The 20-percentage-point dead-zone (40–60%) is the inconclusive band requiring re-design.

## Procedure

1. **Enumerate MAPPED entries.** Run `scripts/extract-mapped-rows.sh` (NEW — companion to S1's extract). Filter for `Status` ∈ {MAPPED, MAPPED-CONTRIBUTES}. Expected: ~423 rows full count, ~60 if restricted to parent-control rows in the headline summary tables.
   - **For Phase 1** of this experiment, restrict to the ~60 parent-control MAPPED entries to keep the experiment tractable. Phase 2 expands to the full ~423 if Phase 1 results warrant.
2. **Author one falsifier per MAPPED entry.** A falsifier is a one-sentence test of the form: *"If [check] is false, MAPPED is falsified."* Examples:

   | MAPPED entry | Falsifier |
   |--------------|-----------|
   | `AU-2 MAPPED — pkg/auth.AuditLogger; eBPF event firehose; zerolog with trace_id; Wotan log agg` | `pkg/auth/audit*.go` must contain a logger that captures auth-success, auth-failure, privilege-escalation, and privilege-grant events. If `grep -E "auth_(success\|failure)\|priv_(esc\|grant)" pkg/auth/audit*.go` returns 0 matches, MAPPED is falsified. |
   | `AC-3 MAPPED — pkg/auth.RBACAuthorizer middleware on every service; Champion 3-rule gate on mutations` | Every service binary in `cmd/` must register `auth.Middleware(RBACAuthorizer)` on its router OR justify exemption. If `grep -L "auth.Middleware\|RBACAuthorizer" cmd/*/main.go` returns >1 service without an exemption ADR, MAPPED is falsified. |
   | `AC-4 MAPPED — Wotan topic-bus + topic signing on config.* + Champion blocks unauthorized cross-trust flows` | `services/wotan/internal/signing/` must reject an unsigned `config.*` publish in test. If `go test ./services/wotan/internal/signing/... -run TestUnsignedConfigRejected` does not exist OR fails, MAPPED is falsified. |
   | `SC-13 MAPPED — TLS 1.3 mandatory; ML-DSA-65 + SLH-DSA via cloudflare/circl v1.6.3` | `pkg/transport/` must enforce `tls.VersionTLS13` minimum. If `grep -r "MinVersion.*TLS1[12]\|VersionTLS1[012]" pkg/transport/` returns matches, MAPPED is falsified. |
   | `CM-8 MAPPED — Sealed Cask SHA-256 chain + SBOM + ADR-052` | `scripts/build-sealed-cask.sh` must succeed on a clean build host AND `scripts/verify-binding-rune.sh` must verify the result. If either fails on a fresh checkout, MAPPED is falsified. |

   **General falsifier-authoring rule.** A falsifier must be:
   - **Executable** — runnable as a single shell line or `go test` invocation, with binary pass/fail output.
   - **Specific** — names a file, package, function, test, or topic by path/identifier.
   - **Failing-on-falsification** — its failure necessarily means the MAPPED status is over-claimed, not just that documentation is missing.
   - **No tautologies** — `ls pkg/auth/` is NOT a falsifier (S6 critique). The falsifier must verify control *intent*, not file existence.
3. **Run all falsifiers.** Capture output to `out/s2-falsifier-results.csv` with columns: `framework, control_id, falsifier_command, exit_code, stdout_tail, verdict (PASS/FAIL/SKIP)`.
4. **Tally pass-rate.** `pass_rate = PASS / (PASS + FAIL)`. SKIP entries (where the falsifier could not be authored — e.g. for ADOPTER-OWNS or PARAM-GAP entries — are excluded).
5. **Decide.** If `pass_rate ≥ 0.60` → fail to reject H0 (matrix MAPPED claims survive). If `pass_rate < 0.40` → reject H0 (S2 critique confirmed). If `0.40 ≤ pass_rate < 0.60` → inconclusive; design a finer-grained falsifier set and re-run.

## Success criterion

| Pass-rate | Verdict |
|-----------|---------|
| ≥ 0.80 | Strong support for MAPPED claims. |
| 0.60–0.79 | **Pass threshold.** MAPPED claims survive S2 in aggregate. |
| 0.40–0.59 | **Inconclusive.** Re-design falsifiers + re-run. |
| < 0.40 | **Fail.** S2 critique confirmed. Kingdom is over-claiming MAPPED by ≥ 60%. |

## Required tooling

- **NEW — `scripts/extract-mapped-rows.sh`** (sibling of S1's `extract-control-rows.sh`).
- **NEW — `docs/compliance/control-matrix/falsifiers.yaml`** — one entry per MAPPED row with `{control_id, framework, falsifier_command, expected_exit_code: 0}`. This is a kingdom-authored data file, not a generated artifact. Authoring 60 falsifiers is the dominant Phase 1 work.
- **NEW — `scripts/run-s2-falsifiers.sh`** (~50 LOC). Reads `falsifiers.yaml`, runs each command in a sandboxed shell, captures exit code + stdout tail, writes CSV.
- **EXISTING:** Go test suite, `grep`, `ripgrep` if available, `go build`, kingdom CI scripts.

## Owner

- **Scientist** authors the falsifier-authoring rule + reviews falsifier quality.
- **Architect + Developer** co-author the actual `falsifiers.yaml` (one falsifier per MAPPED row — they know the kingdom code best).
- **MoatGhost** runs `run-s2-falsifiers.sh`, tallies results.
- **Marshal** does not author falsifiers (would re-introduce the S2 problem the experiment is designed to expose).

## Estimated runtime

- Sample-frame extraction: 30 min.
- Falsifier authoring (Phase 1, ~60 entries × 10 min/falsifier): **~10 hours** of Architect+Developer time. **This is the dominant cost.**
- Falsifier execution: ~2 hours wall-clock for the full set (some falsifiers run `go test` which takes minutes each).
- Tally + analysis: 1 hour.
- Writeup: 2 hours.
- **Total Phase 1: ~15 hours across multiple owners.** Phase 2 (full ~423 set) would multiply the falsifier-authoring cost by ~7.

## Output format

- `docs/compliance/control-matrix/falsifiers.yaml` — the canonical falsifier data file (kingdom artifact, not output).
- `out/s2-falsifier-results.csv` — execution results (one row per falsifier).
- `docs/compliance/control-matrix/05-s2-mapped-falsification-results-YYYY-MM-DD.md` — formal writeup.

## Disposition rule

- **If pass-rate ≥ 0.60:** S2 critique addressed. The MAPPED entries that failed individual falsifiers are downgraded to PARTIAL with the falsifier output cited as evidence-of-gap.
- **If pass-rate < 0.40:** S2 critique confirmed. **Block** any auditor-facing or adopter-facing release. Run a status-downgrade pass: every MAPPED entry that lacks a passing falsifier is downgraded to PARTIAL or UNVERIFIED. Re-run S1 (IRR test) on the downgraded matrix family to verify the new statuses are themselves reproducible.
- **Inconclusive:** the falsifier set is too coarse or too strict. Designate a falsifier-design retro before re-run.

---

# S3 — PII Telemetry Scan

## Hypothesis

- **H0 (null):** The "Unheaded never sees PII / PHI / CHD / SAD / CUI by architectural design" claim is true. A scan of real Wotan ring-buffer dumps + Anamnesis Lite event streams + Zhen `zhen_conversations` PG table will find **zero** GDPR Art. 4(1) personal-data instances.
- **H1 (alternative):** The architectural-floor claim is overstated. A non-zero fraction of telemetry contains personal data. ≥ 12 matrix files' "N/A-DESIGN" scopings collapse to "PARTIAL-CONTROLLER."

**Why "non-zero" is the threshold.** Personal data in any quantity falsifies a "never sees" claim. The hypothesis is asymmetric — the prosecution needs only one example.

## Procedure

1. **Collect data sources.** Capture three telemetry artifacts from a representative operator session (no test-only synthetic data — must be a real Stevie Zhen session of 30+ min duration, otherwise the experiment under-samples real-world surfaces):
   - **Wotan ring-buffer dump.** `curl -s http://wotan:18000/api/v1/ringbuffer/dump > out/s3-wotan-dump.jsonl` — captures last 10K log entries across all topics.
   - **Anamnesis Lite event stream.** Run `cmd/anamnesis-tap` for 5 minutes during a representative kingdom workload (a Zhen RAG query + a Champion mutation + a runbook execution). Save to `out/s3-anamnesis-stream.jsonl`.
   - **Zhen `zhen_conversations` table.** `psql unheaded_app -c "COPY zhen_conversations TO STDOUT (FORMAT csv, HEADER)" > out/s3-zhen-conversations.csv` — full table.
2. **Define personal-data regex set.** Per GDPR Art. 4(1), personal data includes any data identifying a natural person directly or indirectly. The scan checks for:

   | Class | Regex | Notes |
   |-------|-------|-------|
   | Email | `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b` | RFC 5322 simplified. |
   | IPv4 (excluding kingdom 10.0.0.0/8 + 192.168.0.0/16 + 127/8) | `\b((?!10\.\|192\.168\.\|127\.)\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b` | Public IPs only — kingdom-internal IPs are not Art. 4(1) when scoped to kingdom hosts. |
   | IPv6 (global unicast) | `\b(2[0-9a-f]{3}:[0-9a-f:]+)\b` | Excludes fc00::/7 (ULA) and fe80::/10 (link-local). |
   | Phone (E.164 + US 10-digit) | `\b\+?[1-9]\d{1,14}\b\|\b\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b` | High false-positive rate; manual triage. |
   | Common given+family name pairs | `\b(Alice\|Bob\|...) (Smith\|Jones\|...)\b` | Use NLTK names corpus + Census top-1000 surnames. |
   | National ID | `\b\d{3}-\d{2}-\d{4}\b\|\b[A-Z]{2}\d{6,9}\b` | US SSN, EU national ID patterns. |
   | URLs with query strings containing keys like email/phone/name/id/user | `[?&](email\|phone\|name\|user_?id)=[^&\s]+` | S3 critique flagged this specifically. |
   | Browser User-Agent | `Mozilla\/[0-9.]+ \([^)]+\)` | Tracks identifiable browser fingerprint — Art. 4(1) "online identifier." |

   The regex set is documented in `scripts/pii-scan/patterns.yaml` (NEW).
3. **Run the scan.** `python scripts/pii-scan/scan.py --input out/s3-*.jsonl out/s3-*.csv --patterns scripts/pii-scan/patterns.yaml --output out/s3-findings.csv`. Output: one row per match with columns `source_file, line_number, pattern_class, matched_text, surrounding_context_30char`.
4. **Manual triage of high-FP classes.** Phone + name patterns have high false-positive rates (kingdom IDs that look like phone numbers, etc.). Manually triage the matched set, mark each row TRUE-POSITIVE / FALSE-POSITIVE / KINGDOM-INTERNAL.
5. **Compute presence rates.** For each pattern class: `presence_rate = TRUE-POSITIVE_count / total_telemetry_lines`.
6. **Decide.** If any class has TRUE-POSITIVE count ≥ 1, reject H0.

## Success criterion

| Finding | Verdict |
|---------|---------|
| 0 TRUE-POSITIVE matches across all classes | **H0 upheld.** Architectural-floor claim is currently sound for sampled session. |
| 1–10 TRUE-POSITIVE matches | **H1 confirmed weakly.** Architectural floor needs to be restated as "secondary-exposure-controlled" (per S3's recommended framing). |
| ≥ 11 TRUE-POSITIVE matches | **H1 confirmed strongly.** ≥ 12 matrix files require "N/A-DESIGN" → "PARTIAL-CONTROLLER" downgrade. |

Critical threshold: **any TRUE-POSITIVE match = H0 falsified.**

## Required tooling

- **NEW — `cmd/anamnesis-tap`** (~150 LOC Go binary). Subscribes to the eBPF event stream and writes JSONL to stdout. May already exist in `pkg/anamnesis/` — check first.
- **NEW — `scripts/pii-scan/patterns.yaml`** (~30 lines).
- **NEW — `scripts/pii-scan/scan.py`** (~120 LOC Python with `regex` package + NLTK names corpus).
- **EXISTING — Wotan ring-buffer dump endpoint** (`/api/v1/ringbuffer/dump` per `pkg/logagg/`). Verify endpoint exists; if not, author it before running S3.
- **EXISTING — `psql` access to The Well's `unheaded_app` database** (per CLAUDE.md, The Well has 3 databases + 7 service-scoped users).

## Owner

- **Scientist** designs the experiment.
- **Sentinel** owns the data-collection step (knows the telemetry pipeline best).
- **MoatGhost** does the manual triage (sensitive — needs auditor-grade discretion when handling potential real PII).
- **Sentinel** runs the scan + computes presence rates.
- **Marshal** is excluded — no authoring role; the experiment is designed to falsify the Marshal's claims, so author-bias must be removed.

## Estimated runtime

- Tooling authoring: ~3 hours (assuming `cmd/anamnesis-tap` doesn't already exist).
- Data collection: 30 min for Wotan dump + 5 min Anamnesis stream + 5 min PG dump = 40 min wall.
- Scan execution: 5 min.
- Manual triage: ~2 hours (depends on volume — could blow up to a full day if regex matches are noisy).
- Writeup: 2 hours.
- **Total: ~8 hours.**

## Output format

- `out/s3-wotan-dump.jsonl`, `out/s3-anamnesis-stream.jsonl`, `out/s3-zhen-conversations.csv` — raw captures (handle as **sensitive** — they may literally contain PII; encrypt at rest, delete after experiment closes).
- `out/s3-findings.csv` — match table.
- `docs/compliance/control-matrix/06-s3-pii-telemetry-results-YYYY-MM-DD.md` — formal writeup with redacted examples (no raw PII in the writeup).

**Critical handling rule.** If the scan finds real PII, the raw captures are themselves a breach risk. Treat them as P1 confidential, store in encrypted SOPS-protected location, schedule deletion within 30 days. The writeup must redact all matched text — cite class + count, never reproduce the matched string.

## Disposition rule

- **If 0 TRUE-POSITIVE matches:** Update each matrix's "N/A-DESIGN" entry with a citation: *"Verified by S3 PII telemetry scan dated YYYY-MM-DD; zero matches across [N] telemetry lines."* Re-run quarterly.
- **If ≥ 1 TRUE-POSITIVE match:** **Block** any auditor-facing release of GDPR / HIPAA / PCI / CCPA matrices until each "N/A-DESIGN" → "PARTIAL-CONTROLLER" downgrade is completed AND a remediation plan is in place to filter PII out of the offending telemetry channel. The remediation plan goes to F7 (retention policy) for cross-referencing.

---

# S4 — Cross-Framework Leverage Validation

## Hypothesis

- **H0 (null):** The Phase E shift-log claim "~5 documents close ~30 cross-framework controls" is correct. A single first-pass IR plan candidate, written to satisfy the union of framework requirements, will pass simulated audits across **5–6** of the 6 frameworks evaluated.
- **H1 (alternative):** The cross-framework arithmetic is wrong (S4 critique). A single first-pass IR plan satisfies **2–3** frameworks at most. The "5 docs close 30 controls" claim is overstated by ~2x.

## Procedure

1. **Author one IR plan candidate (the F5 IR-plan v1 deliverable).** This is **not** part of the experiment — F5 is its prerequisite. The experiment runs only after F5 lands. The IR plan candidate must address:
   - Detection (what triggers IR?)
   - Containment (initial response actions)
   - Eradication (remove threat)
   - Recovery (restore service)
   - Post-incident (forensics, RCA, communication)
   - Roles (who does what)
   - Test/maintenance cadence
   - Stakeholder communication procedure
2. **Author the framework-criteria checklists (one per framework).** For each of the 6 frameworks audited, transcribe the framework's IR requirements as a binary checklist. Examples:

   | Framework | Specific IR criteria checklist (excerpt) |
   |-----------|-----------------------------------------|
   | GDPR Art. 33 | (a) 72-hour notification window stated. (b) Notification content fields enumerated (Art. 33(3) lists 4: nature, contact, consequences, measures). (c) Documentation of breach maintained (Art. 33(5)). (d) Article 34 communication-to-data-subject criteria addressed. |
   | PCI 12.10.1 | (a) Documented IR plan exists. (b) Roles + responsibilities defined. (c) Tested annually. (d) Updated based on lessons learned. (e) Includes containment + eradication + recovery + communication. (f) Coverage of card-brand notification (12.10.1.x). |
   | HIPAA §164.308(a)(6) | (a) Sanctions for workforce members violating policy. (b) Reporting procedure for incidents. (c) Response procedure. (d) Document outcomes. (e) Mitigate harmful effects (164.308(a)(6)(ii)). |
   | NIST IR-8 (NIST 800-53) | (a) Stakeholder definition. (b) IR-team roles. (c) Plan tests at defined frequency. (d) Plan updates after changes/tests. (e) Distribution list. (f) Reviewed at defined frequency. |
   | SOC 2 CC7.3 / CC7.4 / CC7.5 | (a) Incidents identified through monitoring. (b) Communicated to responsible parties. (c) Recovery to acceptable state. (d) Lessons-learned process. |
   | ISO 27001 A.5.24 / A.5.25 / A.5.26 / A.5.27 / A.5.28 | (a) Planning + preparation. (b) Assessment + decision. (c) Response. (d) Lessons learned. (e) Evidence collection. |

   Save to `out/s4-framework-checklists.yaml`.
3. **Simulate one audit per framework.** For each framework:
   - Read each criterion in turn.
   - Read the IR plan candidate.
   - Score each criterion as `PASS`, `FAIL`, or `PARTIAL`.
   - Compute framework-pass: a framework is `PASS` if **all** criteria are `PASS`. `FAIL` if any criterion is `FAIL`. `PARTIAL` if mix of `PASS` + `PARTIAL` only.
4. **Tally framework-passes.** Out of 6 frameworks, count how many are `PASS`.
5. **Decide.** If frameworks-passed ≥ 5, fail to reject H0 (Marshal arithmetic correct). If frameworks-passed ≤ 3, reject H0 (S4 critique confirmed). If 4, inconclusive.

## Success criterion

| Frameworks-passed (out of 6) | Verdict |
|------------------------------|---------|
| 5–6 | H0 upheld. "5 docs close 30 controls" is supported. |
| 4 | Inconclusive. |
| 2–3 | **H1 confirmed.** S4 critique is correct. |
| 0–1 | H1 strongly confirmed. The IR plan v1 is critically under-specified. |

## Required tooling

- **EXISTING (after F5 lands) — IR plan v1 document.** Path TBD; expected at `docs/incident-response/ir-plan-v1-YYYY-MM-DD.md`.
- **NEW — `out/s4-framework-checklists.yaml`** — the 6 checklists (~150 lines total).
- **NEW — `scripts/run-simulated-audit.sh`** (~40 LOC). Renders each checklist + IR plan side-by-side for human scoring. Could be done in pure manual review without scripting if scope tight.
- **EXISTING — framework reference docs.** Auditor must read GDPR text + PCI 12.10.1 + HIPAA §164.308 + NIST IR-8 + SOC 2 TSC + ISO 27001 A.5.24-28. All public.

## Owner

- **Scientist** designs the experiment.
- **MoatGhost** acts as the simulated auditor (must be independent of the IR-plan author).
- **Architect** is the IR-plan author for F5 (so cannot be the experiment auditor).
- **Marshal** is excluded.

## Estimated runtime

- Checklist authoring: ~3 hours (one auditor reading framework texts + transcribing).
- Simulated audits: 6 frameworks × ~30 min/audit = 3 hours.
- Tally + analysis: 30 min.
- Writeup: 1 hour.
- **Total: ~7.5 hours** (assumes F5 IR plan already authored — F5 itself is multi-day).

## Output format

- `out/s4-framework-checklists.yaml` — checklist data.
- `out/s4-audit-results.csv` — per-framework, per-criterion scoring.
- `docs/compliance/control-matrix/07-s4-cross-framework-leverage-results-YYYY-MM-DD.md` — formal writeup.

## Disposition rule

- **If 5–6 frameworks pass:** Phase E shift-log claim survives. Document the experiment as evidence in the next adopter handoff.
- **If 2–3 frameworks pass:** S4 critique confirmed. Update Phase E shift log to corrected language: *"~5 documents close ~10–15 cross-framework controls in their union form; the remaining controls require framework-specific addenda."* Author the addenda as F5 follow-on work.
- **Inconclusive (4):** Re-run with refined checklists; the inconclusive zone usually indicates checklist-criterion ambiguity, not plan ambiguity.

---

# S5 — Transparent-Rubric Gap Rank

## Hypothesis

- **H0 (null):** The matrix-family headline-gaps lists are well-selected. Under a transparent severity rubric, the rank-1 gap will appear in 5+ of the per-matrix headlines. The headline lists are aligned with criticality.
- **H1 (alternative):** Headline-gaps are cherry-picked by author salience, not severity. The rank-1 gap (under transparent rubric) appears in ≤ 4 per-matrix headlines, and the union of per-matrix headlines is poorly correlated with the rubric ranking.

## Procedure

1. **Enumerate all unique headline gaps across the matrix family.** For each of the 15 framework matrices, parse the *"Headline Gaps"* (or equivalently named) section. De-duplicate by gap-topic (e.g. "IR plan missing" appears in 9 of 15 matrices — count as 1 unique gap with multiplicity 9). Expected: ~100 unique gaps. Save to `out/s5-unique-gaps.csv` with columns `gap_id, gap_text, frameworks_listing_it_in_headline (semicolon-separated), multiplicity`.
2. **Score each unique gap on the transparent rubric.** Three scalar scores per gap:
   - **C = controls-closed.** Number of framework-controls a remediation of this gap would close. Range 1–60.
   - **E = ease-of-closure.** 1 (multi-quarter program), 2 (multi-week project), 3 (single-week task), 4 (single-day task), 5 (single-hour task). Inverse of effort.
   - **R = risk-if-not-closed.** 1 (cosmetic), 2 (minor finding), 3 (moderate finding), 4 (major finding), 5 (audit-blocker / breach risk).

   Composite score: **`S = (C × E) / R`**, then **negate** so high R inflates rather than deflates the score: **`S' = (C × R) + E`** is the actual rubric used. Larger = higher priority.
   - **Equivalent algebraic form used in F12 application:** `priority = (controls_closed × risk_if_not_closed) + ease_of_closure`. Range 1–305.
3. **Rank by priority.** Sort descending by `S'`. Save to `out/s5-ranked-gaps.csv`.
4. **Compare top-10 against per-matrix headlines.** For each of the top-10 gaps, count how many per-matrix headlines list this gap.
5. **Compute Spearman rank correlation** between rubric-priority-rank and headline-multiplicity-rank across all unique gaps.
6. **Decide.** If rank-1 gap appears in ≥ 5 per-matrix headlines AND Spearman ρ ≥ 0.5, fail to reject H0. Otherwise reject.

## Success criterion

| Outcome | Verdict |
|---------|---------|
| Rank-1 in ≥ 5 headlines AND ρ ≥ 0.5 | H0 upheld. Headline lists are reasonable. |
| Rank-1 in 4 headlines AND/OR ρ ∈ [0.3, 0.5] | Mixed. Refine rubric weights and re-run. |
| Rank-1 in ≤ 3 headlines OR ρ < 0.3 | **H1 confirmed.** Headline lists are misleading on prioritization. |

## Required tooling

- **NEW — `scripts/extract-headline-gaps.sh`** (~50 LOC). Walks the matrices' "Headline Gaps" sections, emits unique gap CSV.
- **NEW — `scripts/score-rubric.py`** (~80 LOC). Reads gap CSV + a kingdom-authored `out/s5-rubric-scores.yaml` (Architect+Scientist co-authored), computes `S'` per gap.
- **NEW — `scripts/compute-spearman.py`** (~30 LOC). Computes the rank correlation.
- **NEW — `out/s5-rubric-scores.yaml`** (~100 entries × 3 scores = 300 numbers). The scoring itself is the dominant work.

## Owner

- **Scientist** designs the rubric.
- **Architect + Sentinel + BlackMage** co-score (each gap's R is best assessed by BlackMage; E by Architect; C by Scientist+MoatGhost).
- **Marshal** is excluded from scoring (would re-introduce the headline-cherry-pick bias the experiment exposes).

## Estimated runtime

- Gap extraction: 30 min.
- Rubric scoring: 100 gaps × 3 scores × ~3 min/score / 3 parallel scorers = ~5 hours.
- Composite score + ranking: 15 min.
- Spearman correlation + analysis: 30 min.
- Writeup: 1.5 hours.
- **Total: ~7.5 hours of multi-skill scoring time.**

## Output format

- `out/s5-unique-gaps.csv`, `out/s5-rubric-scores.yaml`, `out/s5-ranked-gaps.csv`.
- `docs/compliance/control-matrix/08-s5-rubric-rank-results-YYYY-MM-DD.md` — formal writeup with the unified all-frameworks gap ranking (the deliverable artifact valuable beyond just falsification).

## Disposition rule

- **If H0 upheld:** Headline lists were defensible. Document the rubric as a kingdom artifact for future use.
- **If H1 confirmed:** Re-author each per-matrix headline-gaps section. Selection rule becomes: *"Headline gaps are the top-N (by rubric `S'`) gaps in this framework's relevant gap set."* Re-run S5 to verify alignment.

---

# S6 — Reproducibility-Section Functional Verification

## Hypothesis

- **H0 (null):** The "Verification" sections in each matrix are functional. ≥ 50% of the cited commands actually verify the control's intent, not just file/package existence.
- **H1 (alternative):** The verification sections are window-dressing. < 20% are functional verifications. The remaining 80% are nominal — they confirm files exist or packages compile but do not test the control's required behavior.

## Procedure

1. **Enumerate verification commands.** For each of the 15 framework matrices, parse the `## Verification` section. Extract every command. Expected: 5–15 commands per matrix × 15 matrices = ~120 commands.
2. **Score each command** as one of:
   - **FUNCTIONAL** — the command tests control behavior. Examples:
     - `go test ./pkg/auth/... -run TestRBACEnforcesDenyByDefault` — tests RBAC behavior.
     - `bash scripts/verify-binding-rune.sh` — verifies SHA-256 chain integrity (tests intent of CM-8/CM-14).
     - `curl -s http://wotan:18000/api/v1/topics | jq '.[] | select(.signed==false and (.name|startswith("config.")))' | wc -l` — tests that all config.* topics are signed (tests SC-12 behavior).
   - **NOMINAL** — the command tests existence only. Examples:
     - `ls pkg/auth/` — tests directory exists.
     - `ls docs/legal/` — tests files exist.
     - `grep -r "DataIsolationConfig" pkg/` — tests a string appears in source.
   - **AMBIGUOUS** — could be either depending on interpretation. Examples:
     - `bash scripts/verify-gpl-boundary.sh` — could be functional (verifies no GPL leak in non-GPL deps) or nominal (verifies the script runs without error). Resolve by reading the script.
3. **Resolve AMBIGUOUS scores.** For each AMBIGUOUS command, read the underlying script/check definition. Re-classify as FUNCTIONAL or NOMINAL.
4. **Run each FUNCTIONAL command.** Capture exit code + stdout. Confirm the command actually exercises the control (e.g. would it produce a different output if the control were broken?). This is a cross-check on the FUNCTIONAL classification.
5. **Compute functional-rate.** `functional_rate = FUNCTIONAL / (FUNCTIONAL + NOMINAL)`.
6. **Decide.** If `functional_rate ≥ 0.50`, fail to reject H0. If `< 0.20`, reject H0 strongly. Between 0.20 and 0.50: weak rejection.

## Success criterion

| Functional-rate | Verdict |
|-----------------|---------|
| ≥ 0.80 | Strong reproducibility infrastructure. |
| 0.50–0.79 | **Pass threshold.** Verification sections survive S6. |
| 0.20–0.49 | **Weak fail.** Re-author 50%+ of verification sections. |
| < 0.20 | **Strong fail.** S6 critique confirmed; verification sections are window-dressing. |

## Required tooling

- **NEW — `scripts/extract-verification-blocks.sh`** (~30 LOC). Parses the `## Verification` sections out of the 15 matrices.
- **NEW — `scripts/score-verification.py`** (~50 LOC). For each command, applies the classification rule + emits CSV.
- **EXISTING:** Whatever scripts the verification commands themselves invoke.

## Owner

- **Scientist** designs the classification rule + scores.
- **Developer** cross-checks ambiguous classifications by reading underlying scripts.
- **MoatGhost** runs the FUNCTIONAL commands + verifies they would fail under control breakage (this is itself a sub-experiment per command).
- **Marshal** is excluded (authored most of the verification blocks).

## Estimated runtime

- Extraction: 30 min.
- Classification (120 commands × ~3 min/command): 6 hours.
- Ambiguous resolution: ~1 hour.
- FUNCTIONAL command execution: ~30 min wall (most are <1 min runtime).
- Tally + writeup: 1.5 hours.
- **Total: ~9 hours.**

## Output format

- `out/s6-verification-commands.csv` — extracted commands with classification.
- `docs/compliance/control-matrix/09-s6-verification-functional-results-YYYY-MM-DD.md` — formal writeup.

## Disposition rule

- **If functional-rate ≥ 0.50:** S6 critique partially addressed. The NOMINAL commands are flagged as P3 follow-up — re-author at convenience.
- **If functional-rate < 0.20:** S6 critique confirmed. The verification sections are downgraded to "Inventory" sections (descriptive, not verifying), and a new `## Functional Verification` section is authored per matrix with control-specific behavioral tests. This is a multi-week effort across all 15 matrices.

---

# S8 — Completeness-Audit Verification (Meta-Experiment)

## Hypothesis

- **H0 (null):** `00-completeness-audit-2026-05-06.md` is itself complete. An independent reviewer, given 1 hour, will identify **≤ 2** additional omissions not already in the audit.
- **H1 (alternative):** The audit's "we omit nothing" claim is itself false. An independent reviewer will identify **≥ 3** additional omissions in 1 hour.

## Procedure

1. **Brief the independent reviewer.** Give MoatGhost (or another suitable independent kingdom skill) the brief:
   - Read `00-completeness-audit-2026-05-06.md`.
   - Read each of the 15 framework matrices.
   - Identify **omissions** — controls, articles, sub-controls, requirements, points of focus, control enhancements, parameters, regulatory clauses, or framework dimensions that are NOT covered in either the matrix OR the audit's own omission log.
   - Time-box: 60 minutes from start of independent review.
2. **Reviewer logs each candidate omission** to `out/s8-candidate-omissions.csv` with columns `(framework, omission_type, omission_text, where-checked-in-existing-docs, time-found-at-MM:SS)`.
3. **Validate candidate omissions.** A second skill (Scientist) reviews each candidate. For each, classify:
   - **TRUE-OMISSION** — confirmed not in the matrix or audit doc.
   - **ALREADY-COVERED** — actually present, reviewer missed it.
   - **OUT-OF-SCOPE** — by design (e.g. audit doc explicitly says "regulatory machinery omitted by design").
4. **Count TRUE-OMISSION matches.** If ≥ 3 within the 60-minute window, reject H0.
5. **Iterate optional.** Re-run with a third independent reviewer (BlackMage, Architect) to validate the rate. The Scientist's preferred form is **at least 3 independent reviewers**, each time-boxed to 60 min, then aggregate.

## Success criterion

| TRUE-OMISSIONS in 60 min | Verdict |
|--------------------------|---------|
| 0–2 | **H0 upheld.** Audit completeness claim survives. |
| 3–5 | **H1 confirmed.** Audit doc is incomplete — must be amended. |
| ≥ 6 | **H1 strongly confirmed.** Audit doc must be re-authored from scratch with a documented review protocol. |

## Required tooling

- **NEW — None.** This is a pure-human review experiment. The reviewer needs only a markdown viewer + the existing matrix family.
- **EXISTING — kingdom skills with sufficient compliance-domain literacy.** MoatGhost is the obvious primary. BlackMage adds the offensive-completeness lens.

## Owner

- **Scientist** designs + facilitates.
- **MoatGhost (primary), BlackMage, Architect (optional follow-ons)** as independent reviewers.
- **Marshal** is excluded — Marshal authored the audit doc; would corrupt the test.

## Estimated runtime

- Briefing: 5 min.
- Independent review: 60 min (time-boxed per reviewer).
- Validation: 30 min per reviewer's batch.
- Writeup: 30 min.
- **Total: ~2 hours per reviewer.** With 3 reviewers in parallel: ~3 wall hours.

## Output format

- `out/s8-candidate-omissions.csv` — raw reviewer outputs.
- `out/s8-validated-omissions.csv` — Scientist's classification.
- `docs/compliance/control-matrix/10-s8-audit-completeness-results-YYYY-MM-DD.md` — formal writeup.

## Disposition rule

- **If 0–2 TRUE-OMISSIONS:** Audit doc survives meta-completeness check. Re-run quarterly.
- **If ≥ 3 TRUE-OMISSIONS:** Amend `00-completeness-audit-2026-05-06.md` with the additional omissions. Add a Provenance section noting the S8 run + reviewers + date. Re-run S8 with a fresh reviewer to verify the amendment closes the gap.

---

# Cross-Experiment Summary Table

| ID | Experiment | Cost (hours) | Falsification leverage | Pre-req |
|----|------------|--------------|-----------------------|---------|
| S1 | Inter-rater reliability | ~6h MoatGhost + 2h Scientist | Falsifies the entire taxonomy if κ<0.6 — invalidates all status assignments globally. | F9 formal taxonomy |
| S2 | Per-MAPPED falsification | ~15h Phase 1 (60 entries); ~100h Phase 2 (full ~423) | Falsifies the MAPPED claim count quantitatively; produces a downgrade list. | None |
| S3 | PII telemetry scan | ~8h | Falsifies "architectural floor" with single-positive evidence. Massive consequence — collapses 12+ matrix N/A-DESIGN entries. | `cmd/anamnesis-tap` exists or authored |
| S4 | Cross-framework leverage | ~7.5h | Falsifies the "5 docs → 30 controls" arithmetic → forces Phase E shift-log restatement. | F5 IR plan v1 |
| S5 | Transparent-rubric gap rank | ~7.5h | Falsifies headline-gap selection methodology → produces unified all-frameworks priority list. | None |
| S6 | Verification-section function | ~9h | Falsifies the verification-sections-as-evidence claim. | None |
| S8 | Completeness-audit verification | ~2h per reviewer | Falsifies meta-completeness claim. | None |

**Cheapest experiment to run:** **S8** — at ~2 hours per reviewer with no tooling authoring required, S8 is the lowest-cost experiment in the suite.

**Highest-leverage experiment if it falsifies:** **S3 (PII telemetry scan)** — a single TRUE-POSITIVE match falsifies the architectural-floor claim that scopes-out 30+ controls across 12+ matrix files. The downstream consequence — converting "N/A-DESIGN" entries to "PARTIAL-CONTROLLER" across GDPR, HIPAA, PCI, CCPA, ITAR/EAR — would be the most expensive remediation in the matrix family. By the same token, a clean S3 run is the most valuable single piece of evidence the kingdom could publish.

**Recommended execution order:** S8 (cheapest, runs first to scope further work) → S6 (medium cost, no pre-reqs, exposes verification-block weakness early) → S3 (high leverage; should run before any auditor-facing release) → S5 (produces the unified gap-ranking deliverable that informs F5/F12 prioritization) → S1 (gated on F9 — run after F9 lands) → S2 Phase 1 (gated on falsifier authoring; runs after F9 + S1) → S4 (gated on F5 IR plan v1 + S5 ranking) → S2 Phase 2 (only if Phase 1 results warrant the 7x cost increase).

---

# Suite-Level Disposition

The suite as a whole succeeds when **all 7 experiments are runnable** (tooling authored, pre-reqs satisfied) AND **the suite's collective results inform a kingdom-authored amendment to the matrix family** that addresses the scrutiny doc S1–S8 critique with measured evidence rather than counter-rhetoric.

A "passed suite" means: every experiment ran, every hypothesis was tested, and the matrix family's status statements after the suite reflect the experimental evidence (statuses upgraded where evidence is strong, downgraded where evidence is missing).

A "failed suite" means: ≥ 1 experiment was skipped or its disposition rule was not applied. In a failed suite, the matrix family remains "internal gap inventory only" and is not eligible for auditor or adopter release.

---

## Provenance

Authored by Scientist 2026-05-06, in response to scrutiny-doc S1–S8 critique of the Phase E matrix family. Format follows the per-experiment template specified by Marshal in F10. No external sources beyond the cited scrutiny doc + matrix family + CLAUDE.md. The κ threshold (0.6) follows Landis & Koch 1977; the rubric scoring scale (1–5 ordinal) is kingdom-authored. Each experiment was designed to be falsifiable in its own right — every "pass" criterion has a clearly-defined "fail" complement. The Marshal is excluded from execution roles by design — author bias removal is part of the experimental protocol.

The badge is fair. The road is honest. Falsification first.
