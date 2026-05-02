# ADR-058 — GCP Cost & API Utilization Alarms for bellis.tech

**Status:** Planned
**Date:** 2026-05-02
**Deciders:** Stevie Bellis + unheaded-architect (future) + unheaded-blackmage (cost-DoS threat lens)
**Context owner:** bellis.tech public site (GCP free tier)
**Triggered by:** Stevie's directive 2026-05-02: *"need to set up some alarm/email alert in GCP, API utilization is much below limit of free tier but if someone found an exploit on bellis.tech website they could rack up a bill just to be mean."*

---

## Context

`bellis.tech` is hosted on Google Cloud Platform under the **free tier**. Current API utilization is well below the free-tier ceiling on every metered service — meaning the operational cost today is $0 and the headroom under free-tier quotas is large.

That headroom is the threat surface. A **cost-amplification denial-of-service** attack works like this:

1. Attacker discovers an endpoint or path on `bellis.tech` that triggers a billable GCP API call (Cloud Functions invocation, Cloud Run cold start, Cloud Storage egress, Firestore read, Vertex AI inference, etc.).
2. Attacker scripts a high-rate hammer against that path.
3. Even at "moderate" rates, total monthly API call volume crosses the free-tier ceiling.
4. GCP starts billing per-call against Stevie's payment method.
5. Bill arrives at end of month — the first signal that anything was wrong.

This is the **EDoS** (Economic Denial of Service) class of attack. It's not catastrophic — payment caps and Google's anti-abuse team would likely intervene before $10k of damage — but it's:

- **Trivially executable** by an unfunded adversary (no vulnerabilities required, just a hammer)
- **Personally-painful** in a way no other Unheaded threat is (Stevie pays personally, not "the company")
- **Asymmetric** — attacker spends $0 in compute to make Stevie spend real dollars
- **Underdocumented** in the existing threat surface: `docs/security/application-threat-model.md` covers the agent runtime; `docs/security/threat-register.md` covers CVE feeds; nothing covers GCP billing posture for the personal landing page.

The mitigation is two-part: **detect early** (alarm at a percentage of free-tier ceiling, not after the bill lands) and **cap hard** (budget alert + automatic project-level shutoff at a hard ceiling).

---

## Decision

**Stand up GCP-native cost + utilization alarms for the bellis.tech project. Two-tier alerting: low-threshold email + high-threshold automated kill switch.**

This ADR is **Planned** — full design and execution land in a follow-on commit when activation is triggered (Stevie schedules ~30 min). Recording the requirement here so it doesn't get lost.

### Required components

1. **Per-API utilization alarms** at 50% / 80% / 95% of relevant free-tier ceilings:
   - Cloud Run / Cloud Functions invocations
   - Firestore reads + writes
   - Cloud Storage egress (the most common cost-DoS vector)
   - Cloud Build minutes
   - Whatever else is wired into the bellis.tech deployment when this ADR is activated (audit step required)

2. **Budget alerts** via [Cloud Billing Budgets](https://cloud.google.com/billing/docs/how-to/budgets) at $5 / $25 / $100 thresholds. Email Stevie immediately on each crossing.

3. **Hard kill switch**: at a $200 ceiling, a Cloud Function bound to the budget's Pub/Sub topic disables billing on the project. ([Reference pattern from Google.](https://cloud.google.com/billing/docs/how-to/notify)) Project goes offline rather than continuing to incur charges. Stevie can re-enable manually after investigating.

4. **Notification channel**: email to Stevie's primary address. Secondary: SMS via a notification channel for the kill-switch event (cheaper than waking up to a $200 bill).

5. **Alarm health-check**: a synthetic test that intentionally trips the lowest-tier alarm once per quarter to verify the email actually arrives. Same discipline as smoke-testing fire alarms.

### Out of scope for this ADR

- WAF / rate-limiting at the edge of bellis.tech — separate concern (a follow-on ADR can address). The kill switch here is the **last-resort safety net**, not the primary defense.
- Billing posture for any other GCP project Stevie owns — this ADR is bellis.tech-specific. If `unheaded.com` or other projects also run on GCP, each needs its own alarms (they should follow the same pattern, but each gets its own activation event).
- GCP IAM hardening / service account scope review — assumed already done; cite the service-account audit if one exists.

---

## Consequences

### Positive

- **Bounded blast radius**: the worst-case cost of a successful EDoS shifts from "whatever the attacker scripts in 30 days before Stevie checks the bill" to "$200 + whatever ran between hard ceiling and Stevie's response."
- **Early-warning observability**: the 50% / 80% / 95% utilization alarms create a window for human investigation before any billing impact.
- **Non-disruptive activation**: GCP's cost-alarm tooling is standard, well-documented, and free to set up. No code change required on bellis.tech itself.
- **Auditable**: every alarm crossing leaves a billing-history record, so post-incident forensics is trivial.

### Negative / costs

- **Operational overhead**: the kill switch needs maintenance (Cloud Function to disable billing requires `Billing Account Administrator` IAM, which is a high-privilege role that should be scoped carefully).
- **False positives**: a legitimate traffic spike (e.g., a blog post going viral) trips the same alarms. Acceptable trade-off — a brief false positive is cheaper than a real false negative.
- **Hard kill is hard kill**: the kill switch shuts the project down. If Stevie is asleep when it fires, bellis.tech is offline until he wakes up. Acceptable for a personal site; would NOT be acceptable for a service with paying customers.

### Mitigations

- Set the kill-switch threshold high enough that it only fires on genuine attack-class spending, not legitimate traffic. $200 is a reasonable ceiling for a personal landing page; tune with telemetry once alarms are live.
- Document the manual re-enable steps in a runbook (`runbooks/security/gcp-billing-reenable.yaml` — to be created) so post-incident recovery is mechanical.

---

## Open questions (resolved at activation time)

1. **What's actually wired into bellis.tech?** A pre-activation audit step: enumerate every billable GCP API the site touches today. Set per-API alarms accordingly.
2. **Does Stevie have a current GCP IAM principal review?** If not, do that first — alarms on a project with overly-broad service-account permissions miss the real attack surface (compromised service account → direct billing impact via API keys).
3. **Is there a Cloud Armor / WAF tier in front?** If yes, document the rate-limit config and ensure alarms include WAF block-rate as a leading indicator. If no, file a follow-on ADR for edge protection.
4. **Single-project or org-level alarms?** If Stevie has a GCP organization with multiple projects, billing alarms can be set at org level for blast-radius isolation. Decide at activation.

---

## Implementation outline (when activated)

Activation requires Stevie to schedule ~30 min for the GCP console work. Skeleton:

1. **Audit (10 min):** GCP Console → Billing → Cost breakdown → identify every API in use on bellis.tech in the last 30 days.
2. **Set per-API quotas alarms (5 min each, ~5-15 min total):** for each metered API, create a Cloud Monitoring alarm policy at 50% / 80% / 95% of the free-tier ceiling. Notification channel: email.
3. **Set budget alerts (5 min):** GCP Console → Billing → Budgets & alerts → New budget at $5 / $25 / $100 / $200. Tied to Stevie's email + (optionally) SMS for the $200 trigger.
4. **Wire the kill switch (15 min):** create a Cloud Function subscribed to the budget's Pub/Sub topic. The function calls `cloudbilling.projects.updateBillingInfo` with `billingEnabled: false` when the $200 threshold fires. **Do not test by tripping for real;** test by manually sending a synthetic Pub/Sub message that mimics the budget event payload.
5. **Quarterly synthetic check (calendar reminder):** trip the 50% alarm intentionally; verify email arrives. Document the date/time in a `runbooks/security/quarterly-gcp-alarm-test.yaml` runbook (to be created).

---

## References

- Companion threat doc (this is **not** in the catalog because the bellis.tech project sits adjacent to the Unheaded application surface — but a row referencing this ADR will be added to `docs/security/application-threat-model.md` once activation lands): EDoS class isn't in T1-T10 today.
- GCP cost-alarm reference architecture: <https://cloud.google.com/billing/docs/how-to/notify>
- Auto-disable-billing Cloud Function pattern: <https://cloud.google.com/billing/docs/how-to/notify#cap_disable_billing_to_stop_usage>
- Stevie's directive (2026-05-02): captured verbatim in the "Triggered by" header.
