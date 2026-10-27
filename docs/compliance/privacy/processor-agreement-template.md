<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Processor Agreement Template

**Status:** template, unused. No processor handles personal data for this deployment today. Use this when one is engaged (for example a hosted log store, a mail provider that keeps content, or a DFIR firm handling evidence). It follows GDPR Art. 28(3); it is not legal advice, and counsel reviews it before first use.

---

**Agreement between** Stevie Bellis ("Controller") and `<processor legal name>` ("Processor"), effective `<date>`.

1. **Subject and duration.** The Processor processes personal data only to provide `<service>` for the term of `<main agreement>`, and for no other purpose.
2. **Nature and purpose.** `<what the Processor does with the data, e.g. stores and indexes host logs>`.
3. **Data and subjects.** Categories: `<from docs/compliance/privacy/pii-inventory-*.md>`. Subjects: `<operator; remote parties in logs>`.
4. **Instructions.** The Processor acts only on the Controller's documented instructions, including on transfers outside `<region>`, and says so at once if an instruction appears to break data protection law.
5. **Confidentiality.** Everyone the Processor authorizes to process the data is bound to confidentiality.
6. **Security.** The Processor keeps measures appropriate to the risk (Art. 32), at least: encryption in transit (TLS 1.3) and at rest, access limited by role and logged, tested backups, and vulnerability management. It describes them in an annex and tells the Controller before weakening any.
7. **Sub-processors.** None without the Controller's prior written consent. The Processor binds any sub-processor to these same terms and stays liable for it.
8. **Data subject requests.** The Processor helps the Controller answer access, rectification, erasure and portability requests within `<n>` days.
9. **Breach.** The Processor notifies the Controller without undue delay and within `<24>` hours of becoming aware of a personal data breach, with what is known, and updates as it learns more.
10. **Assistance.** The Processor helps with DPIAs and consultations with supervisory authorities where its processing is involved.
11. **End of service.** At the end, the Processor deletes or returns all personal data, at the Controller's choice, and deletes copies unless law requires keeping them, and confirms in writing.
12. **Audit.** The Processor makes available what is needed to show compliance and allows audits, including inspections, by the Controller or an auditor it mandates, on `<notice>` notice.

**Optional, HIPAA business associate clause.** Only if protected health information is ever involved (it is not today): the Processor agrees to the obligations of a business associate under 45 CFR 164.502(e), 164.504(e) and 164.314(a), including reporting any security incident and any breach of unsecured PHI under 164.410, and returning or destroying PHI at termination.

Signed: Controller `<name, date>` / Processor `<name, title, date>`
