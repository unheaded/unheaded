# Unheaded S37 Release Checklist

> **Historical (S37, early 2026); license lines corrected 2026-10-27.** `LICENSE` is GPL-3.0-or-later; `LICENSE-PROTOCOLS` is GPL-3.0 with Apache-2.0 as the alternative. The MIT / conversion-date terms below were never the shipped license.

## Pre-Release Verification (Completed)

### Licensing & Compliance (Phase 1-2)
- [x] LICENSE file (GPL-3.0-or-later) created and reviewed
- [x] LICENSE-PROTOCOLS file (GPL-3.0 / Apache-2.0) created for specs
- [x] doom/LICENSE (GPL 2.0) established with clear boundary
- [x] License headers added to 21 key source files
- [x] SBOM generated (manual scan + Go module analysis)
- [x] SBOM consolidated and reviewed
- [x] No GPL-3.0 or AGPL in main codebase
- [x] LICENSES/sbom-reports/ integrated into repo

### DOOM Integration (Phase 3)
- [x] Current doomgeneric state verified
- [x] Audio support scaffolded (SDL2_mixer conditional stubs)
- [x] GPL 2.0 boundary verified (0 cross-boundary imports)
- [x] DOOM_IMPLEMENTATION.md documentation complete

### Pre-Public Audit (Phase 4)
- [x] Secrets scanning completed (no credentials found)
- [x] Code comments audited (no inappropriate content)
- [x] .gitignore comprehensive (188 rules)
- [x] README.md, QUICKSTART.md, SECURITY.md complete
- [x] Full build passes
- [x] All unit/integration tests pass
- [x] Repository clean (0 untracked files)

### Final Verification (Phase 5)
- [x] Final license and SBOM verification
- [x] Full build and test verification
- [x] Documentation and metadata verified
- [x] Release checklist complete (this file)

## Distribution Rights
- **Main Codebase:** GPL-3.0-or-later (`LICENSE`)
- **Protocol Specs:** GPL-3.0, or Apache-2.0 at the implementer's choice (`LICENSE-PROTOCOLS`)
- **DOOM Engine:** GPL 2.0 (fully compliant, isolated from main code)

## Post-Release Tasks
- [ ] Push to public repository
- [ ] Configure CI/CD for automated testing
- [ ] Set up contributor guidelines
- [ ] Complete audio integration in DOOM subsystem
- [ ] Install and run ScanCode/FOSSology/ORT for automated SBOM

## Contact
- Licensing: stevie@bellis.tech
- Security: See SECURITY.md for responsible disclosure

---

**Release Ready:** YES
**Date:** 2026-02-24
**Sprint:** S37
