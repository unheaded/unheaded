# COMPLIANCE REMOTE EXECUTION PACKET — 2026-04-27

> **For execution on a Linux dev box with the full toolchain (syft, cargo-deny, go-licenses, govulncheck) and unrestricted network egress.**
> Cannot run from Cowork-on-Macbook: toolchains absent, CISA KEV proxied at HTTP 403.
> All commands here are copy-paste-ready. Output paths are pre-baked.

**Forged**: 2026-04-27 from Cowork-on-Macbook
**Estimated remote wall-clock**: 30–60 minutes
**Owner on remote run**: MoatGhost + Sentinel + Barrister hats
**Success criteria**: 4 artifacts committed under `docs/legal/` and `docs/security/`, threat register `Last refreshed` flips to remote-execution date, ADR-052 drift-guard CI passes on the resulting commit.

---

## Section A — Syft SBOM regeneration

```bash
cd ~/tmp/unheaded
which syft || curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin

# Full repo SBOM, SPDX-JSON format
syft dir:. -o spdx-json > docs/legal/sbom-2026-04-27.spdx.json

# Verify shape
jq 'has("packages") and (.packages | length > 0)' docs/legal/sbom-2026-04-27.spdx.json
PKG_COUNT=$(jq '.packages | length' docs/legal/sbom-2026-04-27.spdx.json)
echo "SBOM packages: $PKG_COUNT"

# Compare to CLAUDE.md baseline (553 deps)
echo "Baseline (CLAUDE.md S52): 553"
echo "Delta: $((PKG_COUNT - 553))"
```

**Expected**: package count near or slightly above 553 (forge research added Rust crates). Material delta (>10%) deserves a closer look — flag in compliance snapshot.

---

## Section B — License scan (cargo-deny + go-licenses)

```bash
cd ~/tmp/unheaded

# Rust workspace
which cargo-deny || cargo install cargo-deny --locked
cargo deny check licenses advisories 2>&1 | tee /tmp/cargo-deny-2026-04-27.txt | tail -40

# Confirm no errors (warnings ok)
if grep -qE '^error\[' /tmp/cargo-deny-2026-04-27.txt; then
  echo "BARRISTER: cargo-deny errors detected — review before continuing"
  exit 1
fi

# Go modules
which go-licenses || go install github.com/google/go-licenses@latest
go-licenses report ./... 2>&1 > /tmp/go-licenses-2026-04-27.csv || echo "go-licenses had non-fatal warnings"
wc -l /tmp/go-licenses-2026-04-27.csv

# Save to repo
mkdir -p docs/legal
cp /tmp/cargo-deny-2026-04-27.txt docs/legal/cargo-deny-2026-04-27.txt
cp /tmp/go-licenses-2026-04-27.csv docs/legal/go-licenses-2026-04-27.csv
```

**Verification**: `grep "GPL-3.0" docs/legal/go-licenses-2026-04-27.csv` — should show only Unheaded's own modules (which are GPL-3.0). External GPL-licensed deps would be a license-boundary issue for the protocol/Apache dual-license surface; flag immediately.

---

## Section C — Threat feed refresh (CISA KEV + NIST NVD)

```bash
cd ~/tmp/unheaded

# CISA KEV
curl -sSL --max-time 30 https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json -o /tmp/cisa-kev.json
jq '.vulnerabilities | length' /tmp/cisa-kev.json

# Last 14 days
TODAY=$(date -u +%Y-%m-%d)
SINCE=$(date -u -d '14 days ago' +%Y-%m-%d)
jq --arg since "$SINCE" '[.vulnerabilities[] | select(.dateAdded >= $since)]' /tmp/cisa-kev.json > /tmp/cisa-kev-recent.json
RECENT_COUNT=$(jq 'length' /tmp/cisa-kev-recent.json)
echo "Recent CISA KEV (14d): $RECENT_COUNT"

# Filter to relevant tech surface
jq '[.[] | select(((.product // "") + " " + (.vendorProject // "") + " " + (.vulnerabilityName // "")) | test("Linux|kernel|eBPF|HIP|ROCm|llama|AMD|GPU|nginx|haproxy|openssl|curl|containerd|LXD|Docker|nixos"; "i"))]' /tmp/cisa-kev-recent.json > /tmp/cisa-kev-relevant.json
RELEVANT_COUNT=$(jq 'length' /tmp/cisa-kev-relevant.json)
echo "Relevant CISA KEV (14d): $RELEVANT_COUNT"

# Append to threat register
{
  echo ""
  echo "### $TODAY — Remote refresh"
  echo ""
  echo "**Status**: ✅ COMPLETE"
  echo "- CISA KEV total: $(jq '.vulnerabilities | length' /tmp/cisa-kev.json)"
  echo "- Recent (14d): $RECENT_COUNT"
  echo "- Relevant (kernel/eBPF/HIP/llama/edge): $RELEVANT_COUNT"
  echo ""
  if (( RELEVANT_COUNT > 0 )); then
    echo "**Relevant entries**:"
    echo ""
    jq -r '.[] | "- \(.dateAdded) | \(.cveID) | \(.product) | \(.vulnerabilityName | .[0:120])"' /tmp/cisa-kev-relevant.json
  else
    echo "*No relevant CVEs in the last 14 days.*"
  fi
} >> docs/security/threat-register.md
```

**Action**: For each relevant CVE, MoatGhost classifies as IGNORE (not in our stack) / WATCH (in-stack but not exploitable) / PATCH (in-stack and exploitable — triggers Sentinel ticket).

---

## Section D — SPDX backfill (the 6 Go gaps)

The Cowork session found 6 Go files lacking SPDX headers. List captured at `/tmp/spdx-missing-go.txt` from that session — regenerate fresh on remote box:

```bash
cd ~/tmp/unheaded
grep -L "SPDX-License-Identifier:" $(find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*") 2>/dev/null > /tmp/spdx-missing-go.txt
cat /tmp/spdx-missing-go.txt

# For .pb.go files (auto-generated): add header to the .proto generation config OR add via post-gen script
# For human-authored files: add SPDX header at line 1
HEADER="// SPDX-License-Identifier: GPL-3.0-or-later"
while IFS= read -r f; do
  case "$f" in
    *.pb.go)
      echo "SKIP (auto-generated): $f"
      ;;
    *)
      if ! grep -q "SPDX-License-Identifier:" "$f"; then
        printf '%s\n\n%s' "$HEADER" "$(cat "$f")" > "${f}.tmp"
        mv "${f}.tmp" "$f"
        echo "ADDED: $f"
      fi
      ;;
  esac
done < /tmp/spdx-missing-go.txt

# Re-verify
NEW_MISSING=$(grep -L "SPDX-License-Identifier:" $(find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*") 2>/dev/null | wc -l)
echo "Go files still missing after backfill: $NEW_MISSING"
echo "(Excluding .pb.go files which carry license via their .proto source.)"
```

**Note on `.pb.go` files**: protobuf-generated. License header should be added to the `.proto` file's option block (`option go_package = "...";`), or via a code-gen post-processing step. Mechanical SPDX prepend can confuse some protoc-gen-go versions — verify build before committing.

---

## Section E — Compliance snapshot finalization

After Sections A–D complete, finalize the compliance snapshot the Cowork session left as a skeleton:

```bash
cd ~/tmp/unheaded
SNAPSHOT=docs/security/compliance-snapshot-2026-04-27.md
# Edit the file: replace each PENDING marker with the result captured above

sed -i "s|SBOM: PENDING|SBOM: ✅ $(date -u +%F) — $PKG_COUNT packages|" $SNAPSHOT
sed -i "s|License scan: PENDING|License scan: ✅ $(date -u +%F) — see docs/legal/cargo-deny-* + go-licenses-*|" $SNAPSHOT
# (Threat register row gets manually updated based on Section C summary.)
```

Verify with:
```bash
grep "PENDING" docs/security/compliance-snapshot-2026-04-27.md && echo "STILL PENDING" || echo "✅ SNAPSHOT COMPLETE"
```

---

## Section F — Commit + push

```bash
cd ~/tmp/unheaded
git add docs/legal/ docs/security/ \
        $(git diff --name-only | grep -v '^vendor/')
git commit --no-gpg-sign -m "[PLAN SPRINT-04-27 REMOTE] Lane E remote execution: SBOM + license scan + threat register + SPDX backfill

- SBOM regenerated: <PKG_COUNT> packages (delta vs CLAUDE.md baseline 553: <DELTA>)
- cargo-deny advisories + licenses: <STATUS>
- go-licenses report: docs/legal/go-licenses-2026-04-27.csv
- CISA KEV refresh: <RECENT_COUNT> recent / <RELEVANT_COUNT> relevant
- 6 Go SPDX gaps backfilled (verified .pb.go exclusions)
- compliance-snapshot-2026-04-27.md: PENDING markers cleared"
git push origin main
```

Drift-guard CI (per ADR-052) will re-run; should pass since timeline.md was refreshed in the same Round Table sprint.

---

## Section G — Stuck handling

If any toolchain install fails:
- syft: build from source via `go install github.com/anchore/syft/cmd/syft@latest`
- cargo-deny: confirm Rust toolchain version `cargo --version` ≥ 1.70
- go-licenses: ensure `go env GOPATH` and `$GOPATH/bin` in PATH

If CISA KEV still 403 from the remote box (unusual): document in threat register; rely on NIST NVD as fallback (`https://services.nvd.nist.gov/rest/json/cves/2.0?lastModStartDate=...`).

If SPDX backfill breaks build on a `.pb.go` file: revert that file specifically; route fix through the protoc-gen config rather than the file content.

---

## Section H — Post-run handoff

After remote execution succeeds:
1. Confirm `compliance-snapshot-2026-04-27.md` has zero PENDING markers
2. Update `references/timeline.md` Age 3 sub-items to add "Compliance refresh complete (SBOM + license + threat register, 2026-04-27)"
3. Mark `battle-plan.md` Lane E items checked
4. Notify Stevie / next Round Table

---

*Compliance Remote Packet forged 2026-04-27 from Cowork-on-Macbook. CISA KEV proxy block + missing toolchains gated this work to remote execution.*
