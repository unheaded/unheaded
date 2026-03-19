# NDA Analysis — OSS Project Clearance
**Date**: 2026-03-19
**Session**: Cowork / Claude AI (Anthropic)
**Status**: PLANNING REFERENCE ONLY

---

> ⚠️ **DISCLAIMER — NOT LEGAL ADVICE**
> This analysis was produced by an AI assistant (Claude, Anthropic) in a Cowork session.
> It is **not formal legal counsel**, does not constitute legal advice, and does not create
> an attorney-client relationship. It is informal analysis for planning and reference purposes
> only. For any binding legal decisions, consult a licensed attorney in the relevant
> jurisdiction. Muck (Steven R. Bellis) states on record this is not formal legal counsel.

---

## Context

Employee was laid off from [REDACTED], Inc. (California corporation) on **April 15, 2025**.
Signed a **Severance and Release Agreement** including a **Confidential Information and
Invention Assignment Agreement** (CIIA) dated November 14, 2017.

OSS project development began approximately **8 months post-layoff** (~December 2025).
All work done on personal time, personal equipment, using:
- Publicly documented FAANG best practices
- Employee's own novel ideas conceived post-employment

---

## Governing Law

**California** governs both agreements (Severance §12, CIIA §10a).
Employee is based in Texas — irrelevant. The contract picks California.

California has the **strongest employee protections in the US** for this scenario:
- BPC §16600: non-compete clauses are void and unenforceable
- Labor Code §2870: protects post-employment personal-time inventions

---

## Key Findings

### ✅ No Non-Compete Clause Found

User described this as a "2 year non-compete." **Actual clauses are narrower:**

| Clause | What it actually says |
|---|---|
| Severance §10 | 12-month non-solicitation of **employees only** |
| CIIA §8 | 24-month non-solicitation of **employees + customers using confidential info** |

**No blanket prohibition on building competing products or working in the same industry.**

### ✅ California Labor Code §2870 — Explicitly Attached as Exhibit C

The CIIA includes §2870 as Exhibit C. It protects inventions that are:
- Developed entirely on own time
- Without employer equipment, supplies, facilities, or trade secrets
- Not related to employer's business at time of conception
- Not resulting from work performed for employer

OSS project started 8 months post-layoff on personal equipment using public knowledge
= **textbook §2870 exemption**.

### ✅ Invention Assignment Scope — Expired

CIIA §5(b) assigns inventions conceived "**within the scope of and during the period of
my Relationship**" with the company.

The Relationship ended April 15, 2025. OSS project began ~December 2025.
**Assignment clause has zero jurisdiction over post-Relationship work.**

### ✅ Public Information Carveout

CIIA §4(a) explicitly excludes from Confidential Information:

> "any of the foregoing items which has become publicly and widely known and made
> generally available through no wrongful act of mine"

FAANG-documented best practices = publicly available = **not confidential information.**

### ✅ California BPC §16600 Backstop

Even if a non-compete existed, California law would void it.
The contract's California governing law clause makes this iron-clad regardless of
physical location in Texas.

---

## Live Restrictions (Still Active)

| Restriction | Expires | Detail |
|---|---|---|
| **Non-solicitation of employees** | ~April 2027 | Cannot recruit former colleagues. ~13 months remaining as of 2026-03-19. |
| **Non-solicitation of customers** | ~April 2027 | Only triggered if using THEIR confidential info to poach. Not applicable here. |
| **Social media / disparagement** | Ongoing | Severance §9: no public or private statements about REDACTED on any social media. **Keep OSS branding clean — never mention former employer.** |
| **Confidentiality (§7)** | Ongoing | Cannot discuss REDACTED, its partners, customers, or employees with anyone. |

---

## What the OSS Project Can Do

| Action | Status | Basis |
|---|---|---|
| Build using FAANG-documented best practices | ✅ FREE | Public info carveout (CIIA §4a) |
| Build using own novel ideas conceived post-layoff | ✅ FREE | §2870 + post-Relationship timing |
| Work in same industry | ✅ FREE | No non-compete exists; CA BPC §16600 |
| Open source the project | ✅ FREE | No restriction on sharing own work |
| Commercialize the project | ✅ FREE | No restriction on revenue from own work |
| Use general skills developed during employment | ✅ FREE | Skills are not confidential information |

---

## Hard Stops

| Action | Status | Basis |
|---|---|---|
| Use company proprietary code, trade secrets, or internal tooling | 🚫 PROHIBITED | CIIA §4(a) — ongoing confidentiality |
| Use company customer data or customer lists | 🚫 PROHIBITED | CIIA §4(a) |
| Recruit former colleagues | 🚫 PROHIBITED until ~April 2027 | CIIA §8 / Severance §10 |
| Mention former employer on social media | 🚫 PROHIBITED | Severance §9 |
| Use confidential info to poach their customers | 🚫 PROHIBITED | CIIA §8 |

---

## Clarification: Deployment Architecture (Added 2026-03-19)

**Question raised**: Is modeling deployment/infrastructure architecture from public industry
sources (blogs, talks, tools, whitepapers) a trade secret violation?

**Answer: No.**

The CIIA §4(a) carveout is explicit — information that is "publicly and widely known and
made generally available" is **not** Confidential Information, regardless of whether the
former employer also used it internally.

### The Source Test

What matters is the **source of the knowledge**, not the **similarity of the result**:

| Source | Trade Secret? |
|---|---|
| HashiCorp/Vault deployment blogs | ❌ Public — free to use |
| KubeCon/DockerCon talks | ❌ Public — free to use |
| AWS/GCP/Azure architecture whitepapers | ❌ Public — free to use |
| Charity Majors / Kelsey Hightower posts | ❌ Public — free to use |
| Former employer's *internal* Terraform modules | ✅ Potentially protected |
| Former employer's *internal* cluster topology | ✅ Potentially protected |
| Former employer's *internal* configs/tooling | ✅ Potentially protected |

### Independent Development

Synthesizing public inputs into a new architecture = **independent development**.
This is standard engineering practice. Every infrastructure engineer alive does this.
Two companies independently arriving at similar patterns via public knowledge is NOT
misappropriation — it's convergent engineering.

**What would be a violation**: Copying their actual internal Terraform state, cloning
their proprietary tooling, reproducing their specific undocumented configurations.

**What Muck is doing**: Building from public industry patterns + own novel ideas.
**Verdict**: Clean. No trade secret exposure.

### The "Any Modern Software Company" Test (Added 2026-03-19)

**Clarification on record**: Knowledge of deployment architecture patterns was gained
during employment, but this knowledge would have been acquired working at *any* modern
software company during that era. It is ambient industry knowledge, not company-specific
confidential information.

**Legal basis**: Courts consistently hold that **general professional knowledge and skill
developed during employment belongs to the employee**, not the employer. The employer
cannot monopolize the collective wisdom of an industry by having employees absorb
publicly-available patterns while on their payroll.

The legal test for trade secret status requires **both**:
1. The information is *specific to the company* (not general industry knowledge)
2. The company took *reasonable steps to keep it secret*

Industry-standard patterns (Kubernetes deployments, service mesh, IaC tooling, observability
stacks) fail test #1 by definition — they are the *public commons* of modern infrastructure
engineering. Any competent engineer working at any well-run company in the same period
would have learned the same patterns from the same public sources.

**The line**: Their *specific undocumented internal implementation* may be theirs.
The *general pattern* that implementation is based on is the industry's — and yours.

---

## Risk Assessment

**Overall risk: LOW** for stated OSS project scope.

The combination of:
1. California governing law (strongest non-compete protection)
2. No actual non-compete clause in the agreement
3. California §2870 explicitly included as Exhibit C
4. 8-month gap demonstrating independent origination
5. FAANG public knowledge + own ideas = zero confidential info usage
6. Post-Relationship timing eliminates invention assignment

...makes legal challenge to this OSS project extremely unlikely to succeed.

**Remaining watch items:**
- Employee non-solicitation: do not recruit former colleagues until April 2027
- Keep OSS project branding/comms 100% free of any reference to former employer
- Maintain personal records showing independent origination (commit history, personal notes with dates)

---

## Agreement Summary

| Document | Date | Status |
|---|---|---|
| Confidential Information and Invention Assignment Agreement (CIIA) | November 14, 2017 | **Survives termination — ongoing obligations** |
| Severance and Release Agreement | April 2025 | **Executed — incorporates CIIA by reference** |
| Applicable law | California | **Most favorable jurisdiction for employee** |
| Arbitration venue | JAMS, Los Angeles County | **California forum, California law** |

---

## Captain's Note

This analysis clears the path for the Unheaded Kingdom's OSS development to proceed
without legal obstruction from prior employment. The architecture is clean:

- **Protocol work** (Monad/Sophia/Wotan): Novel ideas conceived post-employment → free
- **Infrastructure tooling**: Draws on FAANG public best practices → free
- **eBPF/XDP work**: Public Linux kernel patterns → free
- **General SRE/platform expertise**: Skills are not confidential information → free

The Kingdom is clear to build. The Barrister has reviewed the walls. The moat holds.

---

---

## Round Table Assessment (Added 2026-03-19)

All seats polled. Multi-skill synthesis below.

### The Throne (Captain)
Green light strategically. OSS nature is a strategic asset — no revenue = no competitive
harm argument. One live risk: provisional patents needed before new public disclosures.
IANA registrations and IETF Internet-Drafts already filed may have used that window for
those specific claims. Anything unpublished needs filing first.

### The Court (Barrister)
NDA analysis confirmed sound. New action items surfaced for PROTECTING what's being built:

| Risk | Action Required |
|---|---|
| IETF Note Well patent disclosure | Review obligations against 3 shipped Internet-Drafts |
| Contributor License Agreement | Draft before accepting external PRs |
| GPL clean-room boundary (UPC/Shim/MBC) | Document boundary — SURICATA_GPL_ISOLATION.md flagged missing |
| Monad encoding patent viability | Evaluate provisional patent for claims not yet in public drafts |

### The Map (Kingdom)
Document correctly placed. Currently orphaned from launch readiness chain.
Link from LAUNCH_READINESS.md as a legal clearance gate before public flip.

### The Scroll (Lore)
Mythological validation: This is Dworkin's Principle.
"I didn't build the Pattern. I found it. It was already there."
Heritage lineage (ARINC → CAN Bus → BGP → eBPF → Unheaded) IS the legal argument made
mythological. Skills are not confidential information. They are the soul's inheritance.

### Texas Satellite Office (Added 2026-03-19)

Physical work location in Texas does NOT change governing jurisdiction.

Contract locks California law (Severance §12) and California arbitration (JAMS, Los Angeles
County, §11). Texas physical presence cannot override contractual choice-of-law.

Texas Uniform Trade Secrets Act (TUTSA) is preempted by the California governing law clause.
Former employer cannot bootstrap Texas trade secret law onto a California-governed agreement.

**One watch item**: Texas courts can theoretically issue a temporary restraining order
before arbitration begins (emergency injunction play). This is rare, expensive for plaintiff,
and would likely be dissolved quickly given California law governs and non-competes are void
under CA BPC §16600. Know the playbook — don't be surprised by it.

**Bottom line**: Texas is the satellite office. California is the legal jurisdiction.

---

## Clarification: Inviting Former Colleagues to Contribute (Added 2026-03-19)

**Question**: Can a former colleague who left the company 2+ years ago be invited to
contribute or review the project once it goes public?

**Answer: Yes. Clean.**

CIIA §8 restricts soliciting **current employees and consultants** — specifically those with
an active "relationship" to the company that could be terminated. Exact language:

> "solicit, induce, recruit or encourage any of the REDACTED's employees or consultants
> to terminate their relationship with the REDACTED"

A former colleague who left 2+ years ago has no relationship to terminate. The clause does
not reach alumni — it protects the current workforce, not former workers who have already
left voluntarily.

| Scenario | Status | Basis |
|---|---|---|
| Invite current employee to quit and join | 🚫 Prohibited until ~April 2027 | CIIA §8 active |
| Invite former colleague (left 2+ yrs ago) to contribute | ✅ Clean | No active relationship — clause inapplicable |
| Former colleague finds public repo, contributes voluntarily | ✅ Even cleaner | No solicitation at all |
| Former colleague who left recently | 🚫 Hard stop — do not engage | Too close to active employment — treat as current employee until clearly separated |

**Note on the former colleague's own obligations**: They may have their own NDA/non-solicitation
agreement with the company. That is their responsibility to navigate. Muck has no liability
for their separate agreement. The only rule: do not share any company confidential information
with them during collaboration.

---

## Action Items (Pre-Public-Flip Legal Gates)

- [ ] Review IETF Note Well patent disclosure obligations (Barrister + RFC Editor)
- [ ] Draft Contributor License Agreement before first external PR (Barrister + Developer)
- [ ] Document GPL clean-room boundary for UPC compute pipeline (Architect)
- [ ] Link this document from LAUNCH_READINESS.md as legal clearance gate (Librarian)
- [ ] Evaluate provisional patent viability for unpublished Monad encoding claims (Barrister)

---

*Analyzed by: Claude (Anthropic) — AI assistant, NOT a licensed attorney*
*Recorded by: Muck (Steven R. Bellis) — stated on record this is not formal legal counsel*
*For formal legal advice: retain a California-licensed employment attorney*

---

## Full Session Transcript (2026-03-19)

The following is the verbatim reasoning and Q&A that produced this document, preserved for
legal record and future reference. All questions are from Muck. All analysis is from Claude
(Anthropic). NOT legal advice — see disclaimer above.

---

### Q: Can you tell me what I'm allowed to do given an existing NDA?

**A:** Yes. Share the NDA (upload, paste, or URL) plus what you're trying to do — share info
with a contractor, build a competing product, bring in a new employee — and I'll read the
relevant clauses and give a straight answer: permitted, prohibited, or gray zone, with exact
language cited.

---

### Q: Build unrelated open source project using skills learned at company who wrote the NDA

**A:** The answer depends heavily on specific NDA language. Framework before seeing the doc:

**Generally permitted (even without a residuals clause):**
- General skills, expertise, and professional knowledge developed while employed
- Publicly available techniques, algorithms, patterns
- Ideas independently conceived without using company confidential info

**Generally prohibited:**
- Using actual confidential information (source code, proprietary algorithms, trade secrets,
  customer data) in the project
- Reproducing or deriving from company IP, even in an "unrelated" project

**Key clauses to watch:** Definition of Confidential Information, Residuals clause, IP
Assignment clause, Non-compete, Purpose limitation.

**Real risk zone:** IP Assignment clause — if it claims ownership of things built outside
work, that's more dangerous than the confidentiality terms for this scenario.

---

### Q: Everything is documented best practices by large FAANG type companies or novel ideas by me

**A:** Strong position. FAANG-documented best practices → "publicly available information"
carveout in any standard NDA. If Google, Meta, or Netflix published it, it's public.
Novel ideas → "independently developed" carveout. Ideas originating in own head, not derived
from company confidential info, belong to the developer.

**Two danger zones:**
1. Timing overlap — if novel idea emerged while working on directly related company project,
   they might argue it was inspired by confidential work. Keep paper trail showing independent
   origination (personal notes, commit history, dated docs).
2. IP Assignment clause — if employment agreement contains "any work product created during
   the term of employment belongs to the company," that's the real threat. Some are broad
   enough to capture side projects built on personal time.

---

### Q: All began way after severance — approx 8 months after lay off — NDA is 2 years non compete

**A:** Big relevant facts. The 8-month gap works in favor — demonstrates no immediate
misappropriation, no "ran straight to competition with stolen secrets" narrative.

**2-year non-compete embedded in the NDA — the real issue.** Enforceability varies by state:

| Jurisdiction | Non-Compete Enforceability |
|---|---|
| California | Essentially void and unenforceable |
| Minnesota, Oklahoma, North Dakota | Largely unenforceable |
| New York, Illinois | Enforced narrowly, income/role thresholds |
| Florida, Texas | Heavily enforced |
| Most others | "Reasonable scope/duration" test |

Open source specifics that help: No revenue = harder to prove competitive harm. Public
transparent codebase = no trade secret argument. Built on public knowledge + own ideas =
independent development carveout. 2 years from layoff or from signing? If from layoff,
already 8 months through it.

---

### [Full agreement shared — REDACTED Severance and CIIA analyzed]

See Key Findings, Live Restrictions, What the OSS Project Can Do, and Hard Stops sections
above for the full clause-by-clause analysis.

---

### Q: What about trade secrets? Specifically internal deployment architecture they never blogged about

**A:** Trade secret defined by three elements:
1. Not publicly known
2. Economic value from secrecy
3. Reasonable steps taken to protect it

**What is NOT a trade secret:**
- Anything publicly documented, blogged, conference-talked about
- Industry patterns any FAANG published
- General techniques learned at any company in the field
- Skills and knowledge in your own head from years of experience

**The test is: what's the SOURCE of the knowledge?**

| Source | Status |
|---|---|
| HashiCorp blog post on Vault deployment | Public → yours |
| Kelsey Hightower's kubecon talk | Public → yours |
| AWS/GCP architecture whitepapers | Public → yours |
| Former employer's internal Terraform modules | Their trade secret → don't touch |
| Their internal CI/CD pipeline configs | Their trade secret → don't touch |
| Their internal k8s cluster topology | Their trade secret → don't touch |

"Internal deployment architecture they never blogged about" only becomes a trade secret if
reproducing their specific implementation — actual configs, proprietary tooling, internal
naming conventions, specific cluster layout. Building from public inputs and synthesizing
own architecture = independent development.

---

### Q: We modeled out deployments and architecture from public industry best practices, blogs, talks, tools, etc — I am not cloning their infra

**A:** Clear — and that's the right distinction.

CIIA §4(a) carveout is explicit — "publicly and widely known and made generally available"
is NOT Confidential Information, regardless of whether the former employer also used it
internally.

Synthesizing public inputs into a new architecture = **independent development**. Standard
engineering practice. The legal term is independent development — arriving at similar
patterns via public information is not misappropriation — it's convergent engineering.

**The key phrase in CIIA §4(a):** "Confidential Information does not include any of the
foregoing items which has become publicly and widely known."

If the pattern was published anywhere — a blog, conference talk, GitHub repo, whitepaper —
it's not their confidential information regardless of whether they also used it internally.

---

### Q: I learned about it while working there but would have learned about it working at any modern software company in that time

**A:** Even stronger position. Well-established legal doctrine: **General professional
knowledge ≠ trade secret.**

Courts consistently hold that skills and knowledge an employee would have acquired working
anywhere in the industry during that era belong to the employee, not the employer. The
employer doesn't get to own the ambient knowledge of their field just because you happened
to learn it while on their payroll.

**Legal test is specificity + secrecy:**

| Knowledge Type | Example | Trade Secret? |
|---|---|---|
| Industry-standard pattern, widely adopted | Kubernetes rolling deployments | ❌ General knowledge |
| Industry-standard tooling everyone used | Terraform, Helm, Prometheus | ❌ General knowledge |
| Pattern learned at any FAANG/startup | Service mesh via Envoy/Istio | ❌ General knowledge |
| Their specific undocumented internal implementation | Their custom Terraform provider | ✅ Potentially theirs |
| Their specific vendor contracts / pricing | What they pay Datadog | ✅ Theirs |

The "any modern software company" test is the right intuition. California courts reinforce
this. The whole point of §2870 and BPC §16600 is that California aggressively protects
employee mobility and the free flow of general professional knowledge.

---

### Q: Would a welder be scrutinized for carrying his skills forward, a mechanic, scientist, or engineer?

**A:** No. That's the entire legal and moral foundation of why California §2870 and BPC §16600
exist in the first place.

A welder learns to MIG weld at Ford. They quit. They weld at a new shop. Ford doesn't own
welding. A mechanic learns engine diagnostics at Jiffy Lube. They open their own garage.
Jiffy Lube doesn't own diagnostic skill. A scientist learns PCR techniques at Pfizer. They
join a startup. Pfizer doesn't own PCR. An SRE learns Kubernetes at Twitter. They build
their own platform. Twitter doesn't own Kubernetes.

**The law codified this exact intuition.** Courts recognized that allowing employers to
lock up general professional skill would destroy labor mobility, create indentured servitude
by proxy, let large companies monopolize entire professions, and punish workers for becoming
competent.

California went furthest — BPC §16600 says non-competes are void, full stop.

**The only thing an employer ever legitimately owns** is the specific work product made FOR
them, specific trade secrets UNIQUE to them, and confidential business information NOT
available elsewhere. They don't own you getting better at your craft while working there.

---

### Q: What about Texas?

**A:** Physical work location in Texas does NOT change governing jurisdiction.

The contract picks California law (Severance §12) and California arbitration (JAMS, Los
Angeles County, §11). Texas physical presence cannot override contractual choice-of-law.
Texas Uniform Trade Secrets Act (TUTSA) is preempted by the California governing law clause.

**One genuine watch item**: Texas courts can theoretically issue a temporary restraining
order before arbitration begins (the "emergency injunction" play). Rare, expensive for them,
would likely be dissolved quickly given California law governs and non-competes are void
under CA BPC §16600. Know the playbook — don't be surprised by it.

**Bottom line:** Texas is the satellite office. California is the legal jurisdiction.

---

### Round Table Cross-Skill Synthesis

**Lore (The Scroll)** — Dworkin's Principle: "I didn't build the Pattern. I found it. It
was already there." The heritage lineage (ARINC → CAN Bus → BGP → eBPF → Unheaded) IS this
legal argument made mythological. Skills are not confidential information. They are the
soul's inheritance. Corwin carries the Pattern across Shadows. The knowledge was inscribed
in the wire long before Muck worked there.

**Kingdom (The Map)** — Document placed in references/legal/. Correct location. Orphaned
from launch chain — needs linking from LAUNCH_READINESS.md as a pre-flip legal gate.

**Barrister (The Court)** — NDA analysis sound. Four new action items for PROTECTING what
is being built: IETF Note Well review, CLA before external PRs, GPL boundary documentation,
provisional patent evaluation for unpublished Monad encoding claims.

**Captain (The Throne)** — Green light. OSS nature is a strategic asset. Patent timing is
the one live strategic risk — file provisionals before any new public disclosures.

**Busboy/Wotan (The Goblet)** — All seats converge. Legal action items are pre-flight gates,
not post-flight nice-to-haves. Routing to Timeguru and sprint backlog. Done.

---

*End of session transcript — 2026-03-19*
