# Notes: Open Source Projects, “Consumer Use,” and Releasing Code Made with Gemini Code Assist

_Last updated: 2026-01-30_

This file bundles the guidance from our chat about:

1. Whether an open source Git project counts as “consumer use” in licensing terms like Google Gemini.
2. Whether code written with **Gemini Code Assist** can be released under **GPL/MIT/Apache** open-source licenses.

> Not legal advice. If this is high-stakes (company policy, revenue, compliance), have counsel review.

---

## 1) Is an open source Git project considered “consumer use”?

**Not automatically.**  
“Open source on GitHub” describes **how you license/distribute your code**, not **whether your use is “consumer” or “business/professional”** under someone else’s terms.

### How “consumer” is typically defined
In Google’s general Terms, a **consumer** is usually an **individual using services for personal, non-commercial purposes** outside of their trade, business, craft, or profession.

**Source:** Google Terms — https://policies.google.com/terms

### Why an OSS repo can fall into either bucket
An open-source repo can be:

- **Consumer/personal**: a hobby project, not tied to work/clients/revenue.
- **Business/professional**: maintained as part of a job, for a company, for a product/service, or supporting users.

### Special note: Gemini API / AI Studio “not for consumer use”
If you mean **integrating Gemini via API / AI Studio**, Google’s **Gemini API Additional Terms** say the service is intended for **professional or business purposes**, **not consumer use**.

**Source:** Gemini API Additional Terms — https://ai.google.dev/gemini-api/terms

**Implication:** Even if your repo is open source, **don’t assume** it qualifies as “consumer use” for Gemini API / AI Studio. The applicable terms are about *how/why you’re using the service*, not whether your repo is open source.

---

## 2) Can code written using Gemini Code Assist be released as GPL/MIT/Apache open source?

**Usually yes**, *if you have the rights to license the code* (i.e., it isn’t substantially copied/derivative from third-party copyrighted code under incompatible terms).

### What Google’s docs/terms generally indicate
Google’s Code Assist / Generative AI service terms generally take the stance that:

- You retain rights to the materials you develop using the tool, **but** you must not violate third-party rights.
- Google generally **does not claim ownership** over generated output.

Useful references:
- Plugin license/resources: https://developers.google.com/gemini-code-assist/resources/plugin-license  
- How Code Assist works / “works cited” behavior: https://developers.google.com/gemini-code-assist/docs/works  
- Google Cloud service terms (generative AI services posture): https://cloud.google.com/terms/service-terms  
- Gemini API terms (ownership posture; different product surface): https://ai.google.dev/gemini-api/terms

### The catch: third-party code and license compatibility
Your ability to publish under **MIT/Apache/GPL** depends on whether the output is actually yours to license.

If a generated suggestion is effectively a copy (or close derivative) of third-party code:
- **That upstream license still applies.**
- You may need to preserve required notices (e.g., Apache-2.0 attribution).
- You may be unable to relicense incompatible code (e.g., GPL code relabeled as MIT).

Code Assist may sometimes provide **citations** to sources and the applicable licenses. Treat those as a bright “pay attention here” signal.

**Source:** Works/citations documentation — https://developers.google.com/gemini-code-assist/docs/works

---

## Practical “ship it safely” checklist 🧰

1. **Review AI output like code from a new contributor**  
   Test it, security-check it, refactor it, and make it truly *yours*.
2. **Watch for citations or “this looks familiar” blocks**  
   If it cites a repo/license, follow it and comply.
3. **Scan for license and provenance issues**  
   Tools: ScanCode, FOSSology, OSS Review Toolkit (ORT), etc.
4. **Keep Apache-style notices if you include Apache-licensed components**  
   (and similar requirements for other licenses).
5. **Add a CONTRIBUTING note** (optional but smart)  
   Example: “Do not submit code copied from third-party sources you can’t relicense.”

---

## Quick takeaway

- **Open source ≠ automatically “consumer use.”**  
  “Consumer” vs “business/professional” depends on the terms and your usage context.
- **Yes, you can usually open-source code created with Gemini Code Assist**,  
  **as long as it doesn’t infringe or copy third-party code in a way that brings along incompatible obligations.**

