# Business Source License 1.1 (BSL 1.1) — Unheaded Project

This document explains the Unheaded project's use of BSL 1.1 and what it means for contributors and users.

## Overview

Unheaded is licensed under the **Business Source License 1.1 (BSL 1.1)**. This is a source-available license that:

- Grants broad usage rights for many legitimate use cases
- Protects the project's commercial value during its early growth phase
- Automatically converts to Apache 2.0 on the **Change Date** or at a major milestone (whichever comes first)

## What Is BSL 1.1?

BSL 1.1 is a time-limited license designed to balance:

1. **Developer freedom** — Source code is available; you can read, modify, and study it
2. **Commercial protection** — Revenue-generating services require permission during the exclusivity period
3. **Open source transition** — Automatic conversion to Apache 2.0 ensures eventual freedom

It is **NOT an open source license** according to OSI criteria, because it has restrictions during the exclusivity period. However, it is **source-available** and becomes fully open source on the Change Date.

## Who Can Use Unheaded Without Permission?

### Always Permitted (Unrestricted)

- **Internal use** within your organization (any size)
- **Personal projects** and hobby use
- **Educational and research** use (universities, K-12, non-profits)
- **Government entities** and publicly-funded agencies
- **Open source projects** using any OSI-compatible license
- **Contributions** to the Unheaded project itself
- **Evaluation and testing** (up to 30 days without restrictions)

### Permitted Users (Small Organizations)

Organizations with **fewer than 1,000 employees AND less than $10M annual revenue** may:

- Operate Unheaded as a managed service or SaaS offering
- Build competing services using Unheaded
- Deploy publicly without additional licensing

### Educational & Non-Profit Users

- Accredited schools (K-12, universities, research institutions)
- 501(c)(3) non-profit organizations
- Government agencies

These organizations have **full freedom** to use Unheaded however they wish, including building services, even after Change Date.

## Who Needs Commercial Licensing?

Large enterprises (≥1,000 employees OR ≥$10M revenue) that want to:

1. Operate a managed service or SaaS offering
2. Build a competing product
3. Use Unheaded as the core of a revenue-generating service

**must obtain written permission** from the licensor.

**Contact for commercial licensing:** stevenrbellis@gmail.com

## Change Date Policy

### Automatic Conversion

- **Change Date:** 2029-12-31 (4 years from the 2026-02-24 effective date, re-set to 4 years from each major release)
- **Change License:** Apache License 2.0
- **Trigger:** Change Date is reached OR Kubernetes-scale adoption is achieved, whichever comes first

When the Change Date passes:

1. All BSL 1.1 restrictions are lifted
2. The entire codebase automatically converts to Apache 2.0
3. **No action is required from users**
4. All prior uses remain valid; no retroactive license obligations

### What Happens When It Converts?

After Change Date / Kubernetes adoption:

- Anyone can use Unheaded however they wish
- No commercial licensing required
- Fork, modify, redistribute freely
- Same terms as any other Apache 2.0 project

## Additional Use Grant

Beyond the standard BSL 1.1 terms, the Unheaded licensor may grant **written exceptions** for specific uses. This might include:

- Special arrangements for funded research
- Community partnerships
- Strategic collaborations
- Educational institutions with unique needs

Contact: stevenrbellis@gmail.com

## Why BSL 1.1?

Unheaded chose BSL 1.1 because:

1. **Sustainability** — Protects revenue during early growth, enabling full-time maintenance and features
2. **Transparency** — Source code is available for security audits and community review
3. **Future Open Source** — Guaranteed transition to Apache 2.0, not locked in forever
4. **Broad Permissions** — Most legitimate uses (including non-profits and small businesses) are unrestricted
5. **Avoids GPL complications** — Simpler licensing model than GPL-family licenses

## License Exclusions

Some parts of Unheaded use different licenses:

### `/doom/` — GPL 2.0
- DOOM engine integration (reverse-engineered from Vanilla DOOM)
- Licensed separately under GPL 2.0
- See `/doom/LICENSE` for details

### `/docs/protocol/` — Permissive (Apache 2.0 / MIT)
- Protocol specifications must remain permissive for ecosystem interoperability
- See `LICENSE-PROTOCOLS` for details

## SPDX Identifier

All Go source files in Unheaded include the SPDX header:

```
// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
```

## Frequently Asked Questions

### Q: Can I use Unheaded inside my company?
**A:** Yes, unconditionally. Internal use is always permitted, regardless of company size.

### Q: Can I run a competing service using Unheaded?
**A:** Only if you are a Permitted User (< 1,000 employees, < $10M revenue) or a non-profit. Large enterprises need written permission.

### Q: Can I contribute to Unheaded?
**A:** Yes! By submitting contributions, you agree to license them under BSL 1.1 (same as the project).

### Q: What if I want a different license?
**A:** Contact stevenrbellis@gmail.com for commercial licensing options.

### Q: Will Unheaded ever become fully open source?
**A:** Yes! On 2029-12-31 (or sooner if Kubernetes adoption is reached), Unheaded automatically converts to Apache 2.0.

### Q: Can I fork Unheaded?
**A:** You can maintain a private fork for internal use. Public forks must comply with BSL 1.1 or wait for Apache 2.0 conversion.

### Q: Does BSL 1.1 affect my use of Unheaded's output?
**A:** No. License restrictions apply to the software itself, not to data you process or create with it.

### Q: What about security vulnerabilities?
**A:** Security issues should be reported to stevenrbellis@gmail.com with responsible disclosure practices. BSL 1.1 does not prevent security patches or audits.

## More Information

- **Full License Text:** See `LICENSE` in the repository root
- **Protocol Licensing:** See `LICENSE-PROTOCOLS`
- **DOOM Integration:** See `doom/LICENSE`
- **Official BSL 1.1 Spec:** https://mariadb.com/bsl11/
- **Contact:** stevenrbellis@gmail.com

---

**Last Updated:** 2026-02-25  
**Effective Date:** 2026-02-24  
**Change Date:** 2029-12-31
