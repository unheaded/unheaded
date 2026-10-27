# IETF DATATRACKER SUBMISSION BATTLE PLAN — 4 Phases, 38 Steps

**Date**: 2026-03-19
**Sprint**: S-IETF — Submit 6 Internet-Drafts to IETF Datatracker
**Prerequisite**: All 6 specs audit-complete (S74 Phase 5 verified), XML generated for 5/6 drafts
**Target**: All 6 Internet-Drafts visible on datatracker.ietf.org with status "I-D Exists"
**Executor**: Stevie (human, browser-based workflow)
**Estimated Time**: ~2 hours

---

## LEGEND (Human-Adapted)

```
[HUMAN]     = Manual browser/UI action (click, type, navigate)
[V]         = Verification step (MUST pass before proceeding)
[DECIDE]    = Decision point with recommendation
[B]         = Bash command (terminal)
[SEQ]       = MUST be sequential — do NOT skip ahead
[P]         = Can be done in parallel with other [P] steps
```

---

## VARIABLES

```
$SUBMISSION_DIR = ~/tmp/unheaded/ietf-submission/
$DATATRACKER    = https://datatracker.ietf.org
$SUBMIT_URL     = https://datatracker.ietf.org/submit/
$AUTHOR_EMAIL   = stevie@bellis.tech
$AUTHOR_NAME    = Stevie Bellis
```

---

## SITUATION REPORT

### What We Have (Ready for Upload)

| # | Draft | Version | XML Ready | HTML/TXT | Category |
|---|-------|---------|-----------|----------|----------|
| 1 | draft-bellis-unheaded-protocol-foundation | -06 | YES | YES | Experimental |
| 2 | draft-bellis-unheaded-sophia-dictionary | -03 | YES | YES | Experimental |
| 3 | draft-bellis-unheaded-wotan-memory | -03 | YES | YES | Experimental |
| 4 | draft-bellis-unheaded-mbc-isa | -00 | YES | NO | Experimental |
| 5 | draft-bellis-unheaded-shim | -00 | YES | NO | Experimental |
| 6 | draft-bellis-unheaded-pqc-authentication | -00 | **NO** | NO | Experimental |

### What We Need (Gap Analysis)

| Gap | Severity | Fix |
|-----|----------|-----|
| PQC draft missing XML | P0 | Generate XML with kramdown-rfc before submission |
| MBC-ISA and Shim missing TXT/HTML | P1 | Datatracker generates these on upload; not blocking |

### Submission Metadata (All 6 Drafts)

All specs share:
- **IPR**: `trust200902` (Full IETF Trust provisions)
- **Submission Type**: `independent` (Independent Submission Stream)
- **Category**: `exp` (Experimental)
- **Author**: Stevie Bellis <stevie@bellis.tech>, Unheaded, US

### Submission Order (Dependency-Driven)

```
1. Foundation-06  (no upstream deps — everything else references this)
2. Sophia-03      (references Foundation-06)
3. Wotan-03       (references Foundation-06, Sophia-03)
4. MBC-ISA-00     (references Foundation-06)
5. Shim-00        (references Foundation-06, MBC-ISA-00)
6. PQC-00         (references Foundation-06)
```

Foundation MUST go first. Drafts 2-4 and 6 can technically go in any order after that, but Wotan references Sophia, and Shim references MBC-ISA, so the order above respects all dependency edges.

---

## PREFLIGHT HYPOTHESES

| # | Hypothesis | How to Verify | Expected | If Wrong |
|---|-----------|---------------|----------|----------|
| H1 | Datatracker account exists for stevie@bellis.tech | Log in at $DATATRACKER | Successful login | Create account (Phase 0, Step 2) |
| H2 | All 6 XML files exist | `ls $SUBMISSION_DIR/*.xml` | 6 files | Generate missing XML (Phase 0, Step 5) |
| H3 | kramdown-rfc is installed | `which kramdown-rfc` or `kramdown-rfc --version` | Version output | `gem install kramdown-rfc` |
| H4 | xml2rfc is installed | `which xml2rfc` or `xml2rfc --version` | Version output | `pip install xml2rfc` |
| H5 | IPR is trust200902 in all specs | Check front matter of each .md | `ipr: trust200902` | Fix front matter before generating XML |

---

## PHASE 0: PREFLIGHT — ACCOUNT AND TOOLCHAIN (Steps 1-9)

**Goal**: Datatracker account active, all 6 XML files ready, local validation passing.
**Time**: ~25 minutes

### Account Setup

- [ ] **Step 1** [HUMAN] ~2m: **Check for existing Datatracker account**
  - Open browser: `https://datatracker.ietf.org/accounts/login/`
  - Try logging in with `stevie@bellis.tech`
  - If you have an account and can log in -> Step 3
  - If no account exists -> Step 2

- [ ] **Step 2** [HUMAN] ~5m: **Create IETF Datatracker account** (skip if already have one)
  - Navigate to: `https://datatracker.ietf.org/accounts/create/`
  - Fill in the registration form:
    - **Name**: Stevie Bellis
    - **Email**: stevie@bellis.tech
    - **Organization**: Unheaded
    - **Country**: United States
  - Submit the form
  - Check email for confirmation link and click it
  - Log in to verify account is active

- [ ] **Step 3** [V] ~1m: **Verify Datatracker login works**
  - You should see your name in the top-right corner of the Datatracker
  - Navigate to: `https://datatracker.ietf.org/accounts/profile/`
  - Confirm your name shows as "Stevie Bellis" and email is `stevie@bellis.tech`
  - If pass -> Step 4
  - If fail -> fix account details before proceeding

### Toolchain Verification

- [ ] **Step 4** [B] ~2m: **Verify kramdown-rfc and xml2rfc are installed**
  ```bash
  which kramdown-rfc && kramdown-rfc --version
  which xml2rfc && xml2rfc --version
  ```
  - If either is missing:
    ```bash
    # kramdown-rfc (Ruby gem)
    gem install kramdown-rfc

    # xml2rfc (Python package)
    pip install xml2rfc
    ```

- [ ] **Step 5** [B] ~3m: **Generate missing PQC XML**
  ```bash
  cd ~/tmp/unheaded/ietf-submission/
  kramdown-rfc draft-bellis-unheaded-pqc-authentication-00.md > draft-bellis-unheaded-pqc-authentication-00.xml
  ```
  - If kramdown-rfc fails, check the markdown front matter for syntax errors
  - The PQC draft is the only one missing XML — the other 5 are already built

- [ ] **Step 6** [V] ~1m: **Confirm all 6 XML files exist**
  ```bash
  ls -la ~/tmp/unheaded/ietf-submission/*.xml | wc -l
  ```
  - Expected: 6 files
  - If fewer than 6 -> identify which is missing and regenerate

### Local Validation

- [ ] **Step 7** [B] ~5m: **Validate all 6 XML files with xml2rfc**
  ```bash
  cd ~/tmp/unheaded/ietf-submission/
  for xml in *.xml; do
    echo "=== Validating: $xml ==="
    xml2rfc --check "$xml" 2>&1 | tail -3
    echo ""
  done
  ```
  - Expected: No errors (warnings are OK for Experimental track)
  - If errors -> fix the source .md, regenerate XML, re-validate
  - Common issues: missing references, malformed YAML front matter, unclosed XML tags

- [ ] **Step 8** [DECIDE] ~1m: **Handle xml2rfc warnings**
  - **RECOMMENDATION**: Warnings about "unused references" or "non-ASCII characters" are acceptable for Experimental track. Only fix actual errors.
  - **Override ONLY if**: Warnings indicate structural problems (missing required sections, broken cross-references)

- [ ] **Step 9** [V]: **PHASE 0 EXIT GATE**
  - [ ] Datatracker account active and logged in
  - [ ] All 6 XML files exist in `$SUBMISSION_DIR`
  - [ ] All 6 XML files pass `xml2rfc --check` (errors only, warnings OK)
  - [ ] kramdown-rfc and xml2rfc both installed and working
  - If ALL pass -> Phase 1
  - If ANY fail -> DO NOT PROCEED. Fix before continuing.

---

## PHASE 1: SUBMIT ALL 6 DRAFTS (Steps 10-33)

**Goal**: All 6 Internet-Drafts uploaded to Datatracker and confirmed.
**Prerequisite**: Phase 0 EXIT GATE passed
**Time**: ~60 minutes (10 minutes per draft)

> **IMPORTANT**: Submit in dependency order. Foundation first. Wait for each submission to be confirmed before starting the next.

---

### Draft 1: Foundation-06 (Steps 10-13) [SEQ]

- [ ] **Step 10** [HUMAN] ~3m: **Upload Foundation-06 to Datatracker**
  - Navigate to: `https://datatracker.ietf.org/submit/`
  - You should see the "New Internet-Draft Submission" page
  - **Upload method**: Click "Choose File" or drag-and-drop
  - **File to upload**: `draft-bellis-unheaded-protocol-foundation-06.xml`
    - Path: `~/tmp/unheaded/ietf-submission/draft-bellis-unheaded-protocol-foundation-06.xml`
  - Click **"Upload"** (or equivalent submit button)
  - The Datatracker will process the XML and show you a preview

- [ ] **Step 11** [HUMAN] ~3m: **Verify rendering and metadata**
  - On the submission confirmation page, check:
    - [ ] **Title**: "Unheaded: Protocol Foundation -- A Mapped Data Bus over IPv6 Hop-by-Hop Options"
    - [ ] **Document name**: `draft-bellis-unheaded-protocol-foundation-06`
    - [ ] **Revision**: 06
    - [ ] **Author**: Stevie Bellis <stevie@bellis.tech>
    - [ ] **Abstract**: Present and readable
    - [ ] **Intended status**: Experimental
    - [ ] **Category**: Independent Submission
  - Scroll through the rendered HTML/TXT preview — spot-check section headings, tables, and diagrams
  - If anything looks wrong -> do NOT confirm. Go back and fix the XML.

- [ ] **Step 12** [HUMAN] ~2m: **Confirm submission**
  - Check the box acknowledging the **Note Well** (IETF IPR policy)
    - This confirms: "I have read and understood the IETF Note Well statement"
    - For Experimental/Independent track: this means you represent that any IPR you're aware of has been disclosed (or that you're not aware of any)
    - Since IPR is `trust200902` and this is original work, no special patent disclosures are needed unless you hold patents covering the technology described
  - Click **"Submit"** (or "Post" / "Confirm" — exact button text varies)
  - You should see a confirmation page with a submission ID

- [ ] **Step 13** [V] ~2m: **Verify Foundation-06 is visible on Datatracker**
  - Navigate to: `https://datatracker.ietf.org/doc/draft-bellis-unheaded-protocol-foundation/`
  - Expected: Page exists, shows revision -06, status "I-D Exists" or "Active"
  - Also check your email — Datatracker sends a confirmation email to the author
  - If the page doesn't exist yet, wait 2-3 minutes and refresh (processing can take a moment)
  - If pass -> Step 14 (Sophia)
  - If fail -> check submission status at `$DATATRACKER/submit/status/`

---

### Draft 2: Sophia Dictionary-03 (Steps 14-17) [SEQ]

- [ ] **Step 14** [HUMAN] ~3m: **Upload Sophia-03 to Datatracker**
  - Navigate to: `https://datatracker.ietf.org/submit/`
  - Upload file: `draft-bellis-unheaded-sophia-dictionary-03.xml`
  - Click **"Upload"**

- [ ] **Step 15** [HUMAN] ~3m: **Verify rendering and metadata**
  - Check:
    - [ ] **Title** contains "Sophia" and "Dictionary" / "BPF Map"
    - [ ] **Document name**: `draft-bellis-unheaded-sophia-dictionary-03`
    - [ ] **Revision**: 03
    - [ ] **Author**: Stevie Bellis
    - [ ] **Intended status**: Experimental
    - [ ] **References**: Foundation-06 appears in references section
  - Spot-check the rendered output

- [ ] **Step 16** [HUMAN] ~2m: **Confirm submission (Note Well acknowledgment)**
  - Same Note Well checkbox as Foundation
  - Click **"Submit"**

- [ ] **Step 17** [V] ~2m: **Verify Sophia-03 is visible**
  - Navigate to: `https://datatracker.ietf.org/doc/draft-bellis-unheaded-sophia-dictionary/`
  - Expected: Page exists, revision -03
  - If pass -> Step 18

---

### Draft 3: Wotan Memory-03 (Steps 18-21) [SEQ]

- [ ] **Step 18** [HUMAN] ~3m: **Upload Wotan-03 to Datatracker**
  - Navigate to: `https://datatracker.ietf.org/submit/`
  - Upload file: `draft-bellis-unheaded-wotan-memory-03.xml`
  - Click **"Upload"**

- [ ] **Step 19** [HUMAN] ~3m: **Verify rendering and metadata**
  - Check:
    - [ ] **Title** contains "Wotan" and "Memory"
    - [ ] **Document name**: `draft-bellis-unheaded-wotan-memory-03`
    - [ ] **Revision**: 03
    - [ ] **Author**: Stevie Bellis
    - [ ] **Intended status**: Experimental
    - [ ] **References**: Foundation-06 and Sophia-03 appear in references
  - Spot-check rendered output

- [ ] **Step 20** [HUMAN] ~2m: **Confirm submission (Note Well acknowledgment)**
  - Click **"Submit"**

- [ ] **Step 21** [V] ~2m: **Verify Wotan-03 is visible**
  - Navigate to: `https://datatracker.ietf.org/doc/draft-bellis-unheaded-wotan-memory/`
  - Expected: Page exists, revision -03
  - If pass -> Step 22

---

### Draft 4: MBC-ISA-00 (Steps 22-25) [SEQ]

- [ ] **Step 22** [HUMAN] ~3m: **Upload MBC-ISA-00 to Datatracker**
  - Navigate to: `https://datatracker.ietf.org/submit/`
  - Upload file: `draft-bellis-unheaded-mbc-isa-00.xml`
  - Click **"Upload"**

- [ ] **Step 23** [HUMAN] ~3m: **Verify rendering and metadata**
  - Check:
    - [ ] **Title** contains "MBC" and "Instruction Set"
    - [ ] **Document name**: `draft-bellis-unheaded-mbc-isa-00`
    - [ ] **Revision**: 00
    - [ ] **Author**: Stevie Bellis
    - [ ] **Intended status**: Experimental
    - [ ] **References**: Foundation-06 appears in references
  - This is a -00 draft (first submission) — pay extra attention to the abstract and introduction rendering

- [ ] **Step 24** [HUMAN] ~2m: **Confirm submission (Note Well acknowledgment)**
  - Click **"Submit"**

- [ ] **Step 25** [V] ~2m: **Verify MBC-ISA-00 is visible**
  - Navigate to: `https://datatracker.ietf.org/doc/draft-bellis-unheaded-mbc-isa/`
  - Expected: Page exists, revision -00
  - If pass -> Step 26

---

### Draft 5: Shim-00 (Steps 26-29) [SEQ]

- [ ] **Step 26** [HUMAN] ~3m: **Upload Shim-00 to Datatracker**
  - Navigate to: `https://datatracker.ietf.org/submit/`
  - Upload file: `draft-bellis-unheaded-shim-00.xml`
  - Click **"Upload"**

- [ ] **Step 27** [HUMAN] ~3m: **Verify rendering and metadata**
  - Check:
    - [ ] **Title** contains "Shim" and "Execution Pipeline"
    - [ ] **Document name**: `draft-bellis-unheaded-shim-00`
    - [ ] **Revision**: 00
    - [ ] **Author**: Stevie Bellis
    - [ ] **Intended status**: Experimental
    - [ ] **References**: Foundation-06 and MBC-ISA-00 appear in references
  - Spot-check Dream Ladder conformance levels table renders correctly

- [ ] **Step 28** [HUMAN] ~2m: **Confirm submission (Note Well acknowledgment)**
  - Click **"Submit"**

- [ ] **Step 29** [V] ~2m: **Verify Shim-00 is visible**
  - Navigate to: `https://datatracker.ietf.org/doc/draft-bellis-unheaded-shim/`
  - Expected: Page exists, revision -00
  - If pass -> Step 30

---

### Draft 6: PQC Authentication-00 (Steps 30-33) [SEQ]

- [ ] **Step 30** [HUMAN] ~3m: **Upload PQC-00 to Datatracker**
  - Navigate to: `https://datatracker.ietf.org/submit/`
  - Upload file: `draft-bellis-unheaded-pqc-authentication-00.xml`
  - Click **"Upload"**

- [ ] **Step 31** [HUMAN] ~3m: **Verify rendering and metadata**
  - Check:
    - [ ] **Title** contains "Post-Quantum" or "PQC" and "Authentication"
    - [ ] **Document name**: `draft-bellis-unheaded-pqc-authentication-00`
    - [ ] **Revision**: 00
    - [ ] **Author**: Stevie Bellis
    - [ ] **Intended status**: Experimental
    - [ ] **References**: Foundation-06 appears in references
    - [ ] **Algorithm references**: SLH-DSA (FIPS 205), ML-DSA (FIPS 204), ML-KEM (FIPS 203) mentioned
  - Spot-check that algorithm coverage and implementation status sections render

- [ ] **Step 32** [HUMAN] ~2m: **Confirm submission (Note Well acknowledgment)**
  - Click **"Submit"**

- [ ] **Step 33** [V] ~2m: **Verify PQC-00 is visible**
  - Navigate to: `https://datatracker.ietf.org/doc/draft-bellis-unheaded-pqc-authentication/`
  - Expected: Page exists, revision -00
  - If pass -> Phase 2
  - If fail -> check submission status, wait a few minutes, retry

---

### Phase 1 EXIT GATE

- [ ] All 6 drafts uploaded and confirmed
- [ ] All 6 drafts visible on Datatracker with correct revision numbers
- [ ] Confirmation emails received for all 6
- [ ] No rendering errors in any draft
- If ALL pass -> Phase 2
- If ANY fail -> fix and resubmit the failed draft before proceeding

---

## PHASE 2: NOTE WELL COMPLIANCE AND IPR (Steps 34-36)

**Goal**: Ensure IPR obligations are met for all 6 Experimental-track drafts.
**Prerequisite**: Phase 1 EXIT GATE passed
**Time**: ~10 minutes

- [ ] **Step 34** [DECIDE] ~2m: **Determine if IPR disclosure is needed**
  - **RECOMMENDATION**: No special IPR disclosure is needed.
  - **Rationale**:
    - All 6 drafts use `ipr: trust200902` (standard IETF Trust provisions)
    - All 6 are Experimental track via Independent Submission Stream
    - Stevie is the sole author and creator of the Unheaded protocol
    - Unless you hold patents (granted or pending) that cover any technology described in these drafts, no IPR disclosure filing is required
    - The Note Well checkbox acknowledged during each submission covers the standard obligation
  - **Override ONLY if**: You are aware of patents (yours or third-party) that cover technology in these drafts. In that case, file a disclosure at `https://datatracker.ietf.org/ipr/new/`

- [ ] **Step 35** [HUMAN] ~3m: **Review IPR disclosure page (informational)**
  - Navigate to: `https://datatracker.ietf.org/ipr/`
  - Search for "unheaded" or "bellis" to confirm no existing IPR disclosures reference your drafts
  - This is a sanity check — if nothing comes up, you're clear
  - If someone else has filed an IPR disclosure against your drafts, read it carefully and consult legal counsel if needed

- [ ] **Step 36** [V]: **PHASE 2 EXIT GATE**
  - [ ] Note Well acknowledged on all 6 submissions (done in Phase 1)
  - [ ] No unexpected IPR disclosures found
  - [ ] Decision on filing own IPR disclosure: NONE NEEDED (or filed if applicable)
  - If pass -> Phase 3

---

## PHASE 3: POST-SUBMISSION — ANNOUNCE AND UPDATE (Steps 37-38)

**Goal**: Notify relevant parties, update project tracking, request IANA early review.
**Prerequisite**: Phase 2 EXIT GATE passed
**Time**: ~25 minutes

### Mailing List Announcements

- [ ] **Step 37** [HUMAN] ~15m: **Send submission announcements**

  **37a. Independent Submissions Editor (ISE) notification**
  - Email the ISE directly (current ISE contact is on `https://datatracker.ietf.org/group/independ/about/`)
  - Subject: `6-document Unheaded Protocol Suite — Independent Submission`
  - Body should include:
    - Brief description of the Unheaded protocol suite (1-2 paragraphs)
    - List of all 6 draft names with Datatracker URLs
    - Note that they form a coordinated suite and ideally should be reviewed together
    - Mention Foundation-06 as the base document, with the other 5 depending on it
    - Note all are Experimental track
    - Request coordinated review

  **37b. IANA early review request** (for Foundation-06's 12 registries)
  - Email: iana@iana.org
  - Subject: `Early Review Request: draft-bellis-unheaded-protocol-foundation-06 (12 IANA registries)`
  - Body should include:
    - Link to the Foundation-06 draft on Datatracker
    - Note that it defines 12 new IANA registries
    - List the registry names (Monad Protocol Version Numbers, Monad Flags Bitfield, Monad Flow Actions, Kingdom Mode Values, etc.)
    - Request early review to catch any IANA issues before ISE review completes
    - This is optional but recommended — catches registry formatting issues early

  **37c. Optional: relevant mailing lists**
  - If you want broader visibility, consider posting to:
    - `ietf@ietf.org` (general IETF list) — a brief announcement is appropriate
    - Any BPF/eBPF or IPv6-related lists where the protocol might interest researchers
  - Keep announcements brief: title, abstract, Datatracker link, "comments welcome"

### Project Updates

- [ ] **Step 38** [B] ~10m: **Update project tracking files**
  - Update `~/tmp/unheaded/references/timeline.md` with submission date and Datatracker URLs
  - Update `~/tmp/unheaded/docs/SUBMISSION_SUMMARY.md` "Next Steps" section to reflect completed submissions
  - Consider adding a line to CLAUDE.md noting the drafts are now live on Datatracker
  - Commit the updates:
    ```bash
    cd ~/tmp/unheaded
    git add docs/SUBMISSION_SUMMARY.md references/timeline.md
    git commit -m "docs: mark 6 Internet-Drafts as submitted to IETF Datatracker

    All 6 Unheaded protocol specs now live on datatracker.ietf.org:
    - Foundation-06, Sophia-03, Wotan-03, MBC-ISA-00, Shim-00, PQC-00
    ISE notification sent, IANA early review requested."
    ```

---

### Phase 3 EXIT GATE (FINAL)

- [ ] ISE notified of 6-document suite
- [ ] IANA early review requested for Foundation-06 registries
- [ ] Project tracking files updated
- [ ] All 6 drafts confirmed visible on Datatracker (final check):

| # | Draft | Datatracker URL | Status |
|---|-------|-----------------|--------|
| 1 | Foundation-06 | `https://datatracker.ietf.org/doc/draft-bellis-unheaded-protocol-foundation/` | [ ] Confirmed |
| 2 | Sophia-03 | `https://datatracker.ietf.org/doc/draft-bellis-unheaded-sophia-dictionary/` | [ ] Confirmed |
| 3 | Wotan-03 | `https://datatracker.ietf.org/doc/draft-bellis-unheaded-wotan-memory/` | [ ] Confirmed |
| 4 | MBC-ISA-00 | `https://datatracker.ietf.org/doc/draft-bellis-unheaded-mbc-isa/` | [ ] Confirmed |
| 5 | Shim-00 | `https://datatracker.ietf.org/doc/draft-bellis-unheaded-shim/` | [ ] Confirmed |
| 6 | PQC-00 | `https://datatracker.ietf.org/doc/draft-bellis-unheaded-pqc-authentication/` | [ ] Confirmed |

---

## EMERGENCY PROCEDURES

### E1: Submission Rejected by Datatracker

**Symptom**: Upload fails with error message about formatting, idnits, or metadata.
**Likely Cause**: XML validation issue, draft naming convention, or date in the future.
**Recovery**:
1. Read the error message carefully — Datatracker gives specific line numbers
2. Fix the source .md file
3. Regenerate XML: `kramdown-rfc draft-name.md > draft-name.xml`
4. Validate locally: `xml2rfc --check draft-name.xml`
5. Re-upload

### E2: Draft Appears But Shows Wrong Metadata

**Symptom**: Draft visible but title, author, or revision is wrong.
**Likely Cause**: Stale XML or incorrect YAML front matter.
**Recovery**:
1. You can submit a corrected version (same revision number) within a short window after initial submission
2. If the window has closed, you'll need to submit as the next revision number (e.g., -07 instead of -06)
3. Contact the Secretariat if needed: `ietf-action@ietf.org`

### E3: Confirmation Email Never Arrives

**Symptom**: Submitted but no email received.
**Likely Cause**: Email delay, spam filter, or wrong email in draft metadata.
**Recovery**:
1. Check spam/junk folder
2. Check submission status at `https://datatracker.ietf.org/submit/status/`
3. Verify your email address in the Datatracker profile matches the draft's author email
4. Wait 15 minutes — Datatracker email can be slow

### E4: Note Well / IPR Concern

**Symptom**: Uncertain about patent obligations.
**Likely Cause**: Third-party technology incorporated in the protocol.
**Recovery**:
1. For cloudflare/circl (PQC): This is open-source (BSD-3-Clause), implementing public NIST standards — no IPR issue
2. For eBPF/BPF: Linux kernel technology, no known patent encumbrances
3. For IPv6 Hop-by-Hop: Standard IETF protocol, well-understood IPR landscape
4. If genuinely uncertain about any technology: file a "no patent" IPR disclosure to be safe, or consult an IP attorney

---

## APPENDIX: KEY REFERENCE LINKS

| Resource | URL |
|----------|-----|
| Datatracker Login | `https://datatracker.ietf.org/accounts/login/` |
| Submission Portal | `https://datatracker.ietf.org/submit/` |
| Submission Status | `https://datatracker.ietf.org/submit/status/` |
| IPR Disclosures | `https://datatracker.ietf.org/ipr/` |
| Note Well | `https://www.ietf.org/about/note-well/` |
| ISE Info | `https://datatracker.ietf.org/group/independ/about/` |
| IANA Contact | `iana@iana.org` |
| xml2rfc Docs | `https://xml2rfc.tools.ietf.org/` |
| kramdown-rfc | `https://github.com/cabo/kramdown-rfc` |
| I-D Guidelines | `https://www.ietf.org/standards/ids/guidelines/` |

---

*S-IETF Battle Plan — Forged 2026-03-19*
*4 Phases. 38 Steps. Six drafts walk into the Datatracker.*
*The protocol IS the moat. Now the world gets to see it.*
