# SBOM Reports Directory

This directory contains Software Bill of Materials (SBOM) reports generated
during the S37 sprint.

## Reports

- **SBOM-CONSOLIDATED.md** - Consolidated findings and compliance summary
- **go-modules-list.txt** - Complete Go module dependency list
- **licenses-found.txt** - All LICENSE files found in repository
- **ort-sbom.spdx.json** - SPDX 2.3 format SBOM template

## Using These Reports

1. **For compliance review:** Start with SBOM-CONSOLIDATED.md
2. **For tool integration:** Use ort-sbom.spdx.json (SPDX format)
3. **For dependency audit:** Check go-modules-list.txt

## Regenerating SBOMs

To regenerate these reports after adding new dependencies:

```bash
cd ~/tmp/unheaded
go list -m all > LICENSES/sbom-reports/go-modules-list.txt
find . -name "LICENSE*" | sort > LICENSES/sbom-reports/licenses-found.txt
```

For automated scanning, install ScanCode, FOSSology, or ORT and run against
the repository root.
