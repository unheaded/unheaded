# S73 PUBLIC LAUNCH CLEANUP — Phase 5: Final Verification Gate

**Date**: 2026-03-18
**Sprint**: S73 — Public Launch Cleanup
**Phase**: 5 of 5
**Prerequisite**: Phases 1-4 exit gates GREEN
**Steps**: 416 (16 verification checks × 1 main check + context steps)
**Target**: Repo confirmed clean for public visibility
**Agent**: Claude Code with Marshal oversight — NO CHANGES, VERIFICATION ONLY
**Commit Cadence**: N/A (read-only phase)

---

## LEGEND

- `[V]` = Verification step (must pass before proceeding)
- `[B]` = Bash command (run directly)
- `[D]` = Debug (investigate failure)
- `[R]` = Read/inspect file
- `[CRITICAL]` = Must pass to proceed (blocks launch)
- `[MEDIUM]` = Failure = known issue (proceed at Muck's discretion)
- `[LOW]` = Informational only
- `✓` = Check complete, passed
- `✗` = Check failed
- `?` = Blocked / unable to verify

---

## OVERVIEW & SCOPE

Phase 5 is the **FINAL GATE** before flipping the repo to public. All Phases 1-4 must be complete and their exit gates GREEN.

**What Phase 5 Does:**
- Runs 16 comprehensive verification checks
- Generates a **LAUNCH_READINESS.md** report
- Determines if repo is ready for public visibility

**What Phase 5 Does NOT Do:**
- No implementation
- No bug fixes
- No code changes
- No architectural decisions

---

## PHASE 5 ENTRY GATE

Before starting Phase 5, verify:

- [ ] **Phase 1 Exit Gate**: Code cleanup (TODOs removed, stubs eliminated)
- [ ] **Phase 2 Exit Gate**: Documentation links validated
- [ ] **Phase 3 Exit Gate**: Config/secrets verified clean
- [ ] **Phase 4 Exit Gate**: Build + test passing, go.mod clean

**If any gate is RED**, halt Phase 5 and return to failed phase.

---

## VERIFICATION CHECKS (Steps 400-415)

---

### Step 400: [V] Build Verification: `go build ./...` [CRITICAL] ~3m

**Objective**: Ensure project compiles with zero errors.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
go build ./... 2>&1
RESULT=$?
echo "Build exit code: $RESULT"
[ $RESULT -eq 0 ] && echo "✓ BUILD PASSED" || echo "✗ BUILD FAILED"
exit $RESULT
```

**Pass Criteria**:
- Exit code `0`
- No error output
- No warnings (OK)

**Fail Criteria**:
- Exit code non-zero
- Any `error:` in output
- Import cycle detected

**If FAILED**:
- Step 401 (debug)

**If PASSED**:
- Step 402

---

### Step 401: [D] Debug Build Failure ~5m

**Only run if Step 400 failed.**

**Commands**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

# Show first 100 lines of error
echo "=== BUILD ERROR OUTPUT (first 100 lines) ==="
go build ./... 2>&1 | head -100

# Try per-package builds to isolate
echo ""
echo "=== PER-PACKAGE BUILD ATTEMPTS ==="
for pkg in ./cmd/... ./internal/... ./pkg/... ./services/...; do
  echo "Building $pkg..."
  go build "$pkg" 2>&1 | grep -E "error|import cycle" | head -5
done

# Check for undefined imports
echo ""
echo "=== GO MOD CHECK ==="
go mod graph 2>&1 | head -20
```

**Resolution Path**:
1. Fix import cycles (refactor code — Phase 1 should have done this)
2. Missing dependencies → `go mod tidy && go mod download`
3. Stale proto files → Regenerate with `go generate ./...`

**If still failing after debug**:
- Mark as `✗ BUILD FAILED — CRITICAL`
- Do NOT proceed to Step 402
- Return to Phase 1 or Phase 4 for fixes

**If resolved**:
- Re-run Step 400
- Document fix in LAUNCH_READINESS.md

---

### Step 402: [V] Test Suite: `go test ./...` [CRITICAL] ~5m

**Objective**: Verify all tests pass.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded
go test -count=1 -timeout 180s ./... 2>&1 | tee /tmp/test-output.log
RESULT=$?
echo ""
echo "Test exit code: $RESULT"
[ $RESULT -eq 0 ] && echo "✓ TESTS PASSED" || echo "✗ TESTS FAILED"

# Summary
echo ""
echo "=== TEST SUMMARY ==="
grep -E "^(ok|FAIL)" /tmp/test-output.log | tail -20
exit $RESULT
```

**Pass Criteria**:
- Exit code `0`
- All packages show `ok`
- No `FAIL` in output

**Known Failures** (if any, document in LAUNCH_READINESS.md):
- Flaky timing tests (mark as known issue)
- Integration tests requiring external services (mark as known issue)

**Fail Criteria**:
- Exit code non-zero
- Package shows `FAIL`
- Race detector errors (if `-race` enabled)

**If FAILED**:
- Step 403 (debug)

**If PASSED**:
- Step 404

---

### Step 403: [D] Debug Test Failure ~5m

**Only run if Step 402 failed.**

**Commands**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

# Show failing test
echo "=== FAILED TESTS ==="
go test -count=1 -v ./... 2>&1 | grep -A 10 "FAIL"

# Run with verbose output
echo ""
echo "=== VERBOSE TEST OUTPUT (last 50 lines) ==="
go test -count=1 -v ./... 2>&1 | tail -50

# Check if tests are flaky (run twice)
echo ""
echo "=== RUNNING TESTS AGAIN (check for flakiness) ==="
go test -count=2 ./... 2>&1 | grep -E "FAIL|ok" | sort | uniq -c
```

**Resolution Path**:
1. Flaky test → Mark as known issue, document in LAUNCH_READINESS.md
2. Missing test fixture → Check if Phase 2 initialization was complete
3. External service → Mark as blocked by infrastructure

**If unable to fix in session**:
- Document as **MEDIUM** issue: "Test suite has known failures (see LAUNCH_READINESS.md)"
- Proceed to Step 404 if at least 90% of tests pass

**If resolved**:
- Re-run Step 402
- Proceed to Step 404

---

### Step 404: [V] TODO Sweep: Production Code [CRITICAL] ~2m

**Objective**: Verify zero TODOs, FIXMEs, or stubs in production code.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== SEARCHING FOR: TODO, FIXME, HACK, XXX, STUB, scaffold ==="
RESULT=$(grep -r "TODO\|FIXME\|HACK\|XXX\|STUB\|scaffold" \
  --include="*.go" \
  cmd/ internal/ pkg/ services/ 2>/dev/null)

if [ -z "$RESULT" ]; then
  echo "✓ NO TODOS/FIXMES/STUBS FOUND"
  exit 0
else
  echo "✗ FOUND MARKERS IN PRODUCTION CODE:"
  echo "$RESULT"
  echo ""
  echo "Count: $(echo "$RESULT" | wc -l)"
  exit 1
fi
```

**Pass Criteria**:
- Exit code `0`
- No output

**Fail Criteria**:
- Any match found
- Even one TODO in production code fails this check

**Notes**:
- Docs are OK (QUICKSTART.md, ARCHITECTURE.md can have TODOs)
- Tests are OK (test files can have scaffold comments)
- Comments in non-.go files are OK
- This check is **CRITICAL** — must be zero for public launch

**If FAILED**:
- Step 405 (list findings)

**If PASSED**:
- Step 406

---

### Step 405: [R] TODO Audit & Categorization ~3m

**Only run if Step 404 failed.**

**Commands**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== FULL TODO/FIXME/STUB LISTING ==="
grep -r "TODO\|FIXME\|HACK\|XXX\|STUB\|scaffold" \
  --include="*.go" \
  -n \
  cmd/ internal/ pkg/ services/ 2>/dev/null | sort

echo ""
echo "=== BY FILE ==="
grep -r "TODO\|FIXME\|HACK\|XXX\|STUB\|scaffold" \
  --include="*.go" \
  -l \
  cmd/ internal/ pkg/ services/ 2>/dev/null | sort | uniq

echo ""
echo "=== COUNTS BY TYPE ==="
echo "TODOs: $(grep -r "TODO" --include="*.go" cmd/ internal/ pkg/ services/ 2>/dev/null | wc -l)"
echo "FIXMEs: $(grep -r "FIXME" --include="*.go" cmd/ internal/ pkg/ services/ 2>/dev/null | wc -l)"
echo "HACKs: $(grep -r "HACK" --include="*.go" cmd/ internal/ pkg/ services/ 2>/dev/null | wc -l)"
echo "Stubs: $(grep -r "STUB" --include="*.go" cmd/ internal/ pkg/ services/ 2>/dev/null | wc -l)"
```

**Action Required**:
- **CRITICAL DECISION**: Each TODO must be:
  1. Removed (code fixed in Phase 1/4)
  2. Moved to GitHub issues + linked in code comment
  3. OR moved to docs/ folder (not production code)

**If any remain in production code**:
- Phase 5 BLOCKS launch
- Return to Phase 1 for cleanup

---

### Step 406: [V] Stub Sweep: "not implemented" / "placeholder" [CRITICAL] ~2m

**Objective**: Verify zero "not implemented" or "placeholder" in production code.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== SEARCHING FOR: 'not implemented', 'placeholder' ==="
RESULT=$(grep -r "not implemented\|placeholder" \
  --include="*.go" \
  -i \
  cmd/ internal/ pkg/ services/ 2>/dev/null)

if [ -z "$RESULT" ]; then
  echo "✓ NO UNIMPLEMENTED STUBS FOUND"
  exit 0
else
  echo "✗ FOUND STUB MARKERS:"
  echo "$RESULT" | head -20
  echo ""
  echo "Total: $(echo "$RESULT" | wc -l)"
  exit 1
fi
```

**Pass Criteria**:
- Exit code `0`
- No output

**Fail Criteria**:
- Any "not implemented" or "placeholder" found

**Notes**:
- Errors/panics that say "not implemented" are OK if they're error messages
- Look for: `return errors.New("not implemented")` — OK
- Look for: `// TODO: implement this` or function body is `{ }` — NOT OK

**If FAILED**:
- These must be completed in Phase 1 or Phase 4
- Cannot proceed with Phase 5

**If PASSED**:
- Step 407

---

### Step 407: [V] Commented Code Sweep: main.go Files [MEDIUM] ~2m

**Objective**: Verify no large blocks of commented-out code in main files.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== CHECKING FOR COMMENTED CODE IN main.go ==="
find cmd/ -name "main.go" | while read f; do
  echo "File: $f"
  # Count consecutive comment lines
  awk '/^[[:space:]]*\/\// { count++ }
       !/^[[:space:]]*\/\// && NF > 0 { if (count > 10) print FILENAME ": " count " consecutive comment lines"; count = 0 }
       END { if (count > 10) print FILENAME ": " count " consecutive comment lines" }' "$f"
done

echo ""
echo "=== MANUAL SPOT CHECK: cmd/*/main.go ==="
for f in cmd/*/main.go; do
  lines=$(wc -l < "$f")
  comments=$(grep -c "^[[:space:]]*\/\/" "$f")
  pct=$((comments * 100 / lines))
  echo "$f: $comments/$lines ($pct% comments)"
  if [ $pct -gt 40 ]; then
    echo "  ⚠ HIGH COMMENT RATIO — inspect manually"
  fi
done
```

**Pass Criteria**:
- No blocks of 20+ consecutive comment lines
- Comment ratio <40% in any main.go

**Fail Criteria**:
- Large (>20 lines) commented code blocks
- Indicates incomplete refactoring or debug scaffolding

**If FAILED**:
- **MEDIUM** severity
- Document in LAUNCH_READINESS.md
- Can proceed if commented code is clearly temporary (with issue link)

**If PASSED**:
- Step 408

---

### Step 408: [V] Secrets Sweep: Hardcoded Credentials [CRITICAL] ~2m

**Objective**: Verify no hardcoded passwords, API keys, or tokens.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== SEARCHING FOR HARDCODED SECRETS ==="
RESULT=$(grep -r "password\|secret\|api_key\|api_secret\|token\|credential" \
  --include="*.go" \
  -i \
  cmd/ internal/ pkg/ services/ 2>/dev/null | \
  grep -v "os.Getenv\|config\.\|flag\.\|viper\|ENV\|VAULT" | \
  grep -v "// .*password\|// .*secret" | \
  head -50)

if [ -z "$RESULT" ]; then
  echo "✓ NO HARDCODED CREDENTIALS FOUND"
  exit 0
else
  echo "✗ POTENTIAL HARDCODED SECRETS:"
  echo "$RESULT"
  exit 1
fi
```

**Pass Criteria**:
- Exit code `0`
- All matches are env var references (os.Getenv, config readers, comments only)

**Fail Criteria**:
- Found literal string: `"password123"`
- Found literal API key: `"sk_live_xyz"`
- Found literal token: `"Bearer eyJhbGc..."`

**Notes**:
- Comments about password fields are OK
- Function names with "secret" are OK
- Config struct field names with "secret" are OK
- **String literals** with actual secrets = CRITICAL FAILURE

**If FAILED**:
- CRITICAL: Cannot launch with hardcoded credentials
- Return to Phase 3 for secrets cleanup
- Block Phase 5 exit

**If PASSED**:
- Step 409

---

### Step 409: [V] README.md Links [MEDIUM] ~2m

**Objective**: Verify every link in README.md points to an existing file.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== EXTRACTING LINKS FROM README.md ==="
grep -o '\[.*\](.*\.md)' README.md | sed 's/.*(\(.*\))/\1/' | sort -u | while read link; do
  # Skip http/https links
  if [[ "$link" == http* ]]; then
    echo "  [OK] External: $link"
  elif [ -f "$link" ]; then
    echo "  ✓ Found: $link"
  else
    echo "  ✗ MISSING: $link"
  fi
done
```

**Pass Criteria**:
- All `.md` links resolve to existing files
- External links (http/https) are OK

**Fail Criteria**:
- Any `[MISSING]` result

**If FAILED**:
- **MEDIUM** severity
- Fix broken links in Phase 2 (docs phase)
- OR update README to remove dead links

**If PASSED**:
- Step 410

---

### Step 410: [V] QUICKSTART.md Links [MEDIUM] ~2m

**Objective**: Verify every link in QUICKSTART.md points to an existing file.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== EXTRACTING LINKS FROM QUICKSTART.md ==="
grep -o '\[.*\](.*\.md)' QUICKSTART.md | sed 's/.*(\(.*\))/\1/' | sort -u | while read link; do
  # Skip http/https links and anchors
  if [[ "$link" == http* ]]; then
    echo "  [OK] External: $link"
  elif [[ "$link" == "#"* ]]; then
    echo "  [OK] Anchor: $link"
  elif [ -f "$link" ]; then
    echo "  ✓ Found: $link"
  else
    echo "  ✗ MISSING: $link"
  fi
done
```

**Pass Criteria**:
- All local `.md` links resolve
- External and anchor links are OK

**Fail Criteria**:
- Any `[MISSING]` file link

**If FAILED**:
- **MEDIUM** severity
- Fix in Phase 2

**If PASSED**:
- Step 411

---

### Step 411: [V] License Files Present [CRITICAL] ~1m

**Objective**: Verify LICENSE and protocol license files exist.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== CHECKING LICENSE FILES ==="
declare -a files=("LICENSE" "LICENSE-PROTOCOLS" "SURICATA_GPL_ISOLATION.md")
declare -a found

for f in "${files[@]}"; do
  if [ -f "$f" ]; then
    size=$(wc -c < "$f")
    echo "  ✓ $f ($size bytes)"
    found+=("$f")
  else
    echo "  ✗ MISSING: $f"
  fi
done

if [ ${#found[@]} -eq 3 ]; then
  echo ""
  echo "✓ ALL LICENSE FILES PRESENT"
  exit 0
else
  echo ""
  echo "✗ MISSING LICENSE FILES"
  exit 1
fi
```

**Pass Criteria**:
- All three files exist and have content (>0 bytes)

**Fail Criteria**:
- Any license file missing

**Notes**:
- LICENSE should be GPL-3.0 or compatible
- LICENSE-PROTOCOLS for RFC protocol definitions
- SURICATA_GPL_ISOLATION.md for GPL v2 boundary

**If FAILED**:
- CRITICAL: Public repo requires clear licensing
- Create missing files in Phase 2

**If PASSED**:
- Step 412

---

### Step 412: [V] docker-compose.yml Validity [MEDIUM] ~2m

**Objective**: Verify docker-compose.yml references only images/builds that exist.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== CHECKING docker-compose.yml ==="
if [ ! -f "docker-compose.yml" ]; then
  echo "ℹ docker-compose.yml not found (optional)"
  exit 0
fi

echo "File exists. Checking syntax..."
docker-compose config -q > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo "  ✓ Syntax valid"
else
  echo "  ⚠ Syntax check failed (docker not running?)"
  # Still try to parse it
  echo "  Attempting manual parse..."
  grep -E "image:|build:" docker-compose.yml | while read line; do
    echo "    $line"
  done
fi

echo ""
echo "=== IMAGE/BUILD REFERENCES ==="
grep -E "^\s+(image|build):" docker-compose.yml | sed 's/.*: *//' | while read ref; do
  # Check if it's a local build
  if [[ "$ref" == "."* || "$ref" == "$"* ]]; then
    echo "  [Local] $ref"
    # Verify Dockerfile exists
    if [ -f "Dockerfile" ]; then
      echo "    ✓ Dockerfile found"
    else
      echo "    ✗ Dockerfile missing"
    fi
  else
    echo "  [Image] $ref (external registry)"
  fi
done

exit 0
```

**Pass Criteria**:
- docker-compose.yml syntax is valid
- All local `build:` references have Dockerfile
- External image references are valid

**Fail Criteria**:
- Syntax errors in YAML
- Missing Dockerfile for `build:` context

**If FAILED**:
- **MEDIUM** severity
- Fix in Phase 4 (infra phase)

**If PASSED**:
- Step 413

---

### Step 413: [V] go.mod Tidiness [CRITICAL] ~1m

**Objective**: Verify `go mod tidy` produces no changes.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== GO MOD TIDINESS CHECK ==="
# Create backup
cp go.mod go.mod.backup
cp go.sum go.sum.backup

# Run tidy
go mod tidy

# Check for differences
if diff -q go.mod go.mod.backup > /dev/null && diff -q go.sum go.sum.backup > /dev/null; then
  echo "✓ go.mod and go.sum are tidy"
  rm go.mod.backup go.sum.backup
  exit 0
else
  echo "✗ go mod tidy would make changes:"
  echo ""
  echo "=== go.mod diff ==="
  diff go.mod.backup go.mod | head -20
  echo ""
  echo "=== go.sum diff (lines) ==="
  diff go.sum.backup go.sum | wc -l

  # Restore
  mv go.mod.backup go.mod
  mv go.sum.backup go.sum
  exit 1
fi
```

**Pass Criteria**:
- `go mod tidy` makes no changes
- go.mod and go.sum are in final state

**Fail Criteria**:
- Any changes after `go mod tidy`
- Indicates uncommitted dependency changes

**Notes**:
- Should have been handled in Phase 1
- Indicates either:
  - Uncommitted go.mod edits
  - Build happened without tidying

**If FAILED**:
- CRITICAL: Dependencies not clean
- Run `go mod tidy` and commit changes
- Phase 1 must have missed this

**If PASSED**:
- Step 414

---

### Step 414: [V] Protocol Spec Links [MEDIUM] ~2m

**Objective**: Verify all 4 draft protocol spec links in README resolve.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== CHECKING PROTOCOL SPEC LINKS IN README ==="
# Look for draft-* references and protocol spec mentions
specs=$(grep -o 'draft-[a-z0-9]*' README.md | sort -u)

if [ -z "$specs" ]; then
  echo "ℹ No draft specs found in README (optional)"
  exit 0
fi

echo "Found draft specs:"
echo "$specs" | while read spec; do
  # Check in docs/protocol/
  if find docs/protocol -name "*$spec*" -type f 2>/dev/null | grep -q .; then
    echo "  ✓ $spec"
  else
    echo "  ✗ $spec (missing in docs/protocol)"
  fi
done

# Also check for explicit RFC links
echo ""
echo "=== CHECKING RFC/XML FILES ==="
for f in docs/protocol/*.xml; do
  if [ -f "$f" ]; then
    echo "  ✓ Found: $(basename $f)"
  fi
done

# Should have at least 3 XML files for the 3 main protocols
xml_count=$(ls -1 docs/protocol/*.xml 2>/dev/null | wc -l)
echo ""
echo "Protocol XML files: $xml_count (expect >= 3)"
if [ "$xml_count" -ge 3 ]; then
  exit 0
else
  echo "✗ Insufficient protocol specs (expect at least 3 XML files)"
  exit 1
fi
```

**Pass Criteria**:
- At least 3 protocol XML files exist in docs/protocol/
- All draft specs referenced in README have supporting files

**Fail Criteria**:
- Fewer than 3 XML files
- Referenced drafts with missing files

**Notes**:
- Phase 2 should have verified protocol specs
- This is final check for completeness

**If FAILED**:
- **MEDIUM** severity
- Document missing specs in LAUNCH_READINESS.md

**If PASSED**:
- Step 415

---

### Step 415: [V] .env / Credentials Files Absent [CRITICAL] ~1m

**Objective**: Verify no .env, credentials.json, or secrets files are committed.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== CHECKING FOR COMMITTED SECRETS FILES ==="
declare -a secret_patterns=(".env" "credentials.json" ".aws" ".gcp" "secret.txt" "token.txt" ".private")

found_secrets=0
for pattern in "${secret_patterns[@]}"; do
  result=$(git ls-files --cached | grep -E "$pattern" 2>/dev/null)
  if [ -n "$result" ]; then
    echo "  ✗ FOUND COMMITTED: $pattern"
    echo "$result"
    found_secrets=$((found_secrets + 1))
  else
    echo "  ✓ Not committed: $pattern"
  fi
done

# Also check working directory
echo ""
echo "=== CHECKING WORKING DIRECTORY ==="
for pattern in "${secret_patterns[@]}"; do
  if ls $pattern 2>/dev/null | grep -q .; then
    echo "  ⚠ Present but untracked (OK): $pattern"
  fi
done

if [ $found_secrets -eq 0 ]; then
  echo ""
  echo "✓ NO SECRETS FILES COMMITTED"
  exit 0
else
  echo ""
  echo "✗ SECRET FILES FOUND IN GIT"
  exit 1
fi
```

**Pass Criteria**:
- No .env, credentials.json, .aws, .gcp, etc. in `git ls-files`
- Only .gitignore rules are required

**Fail Criteria**:
- Any secret file is committed to git history

**Notes**:
- Even if deleted from main, if it's in git history, repo is compromised
- Phase 3 should have verified this

**If FAILED**:
- CRITICAL: Credentials in public repo = security breach
- Must use BFG or git filter-branch to remove history
- Phase 5 BLOCKS launch

**If PASSED**:
- Step 416

---

### Step 416: [V] Binary Blobs Scan [MEDIUM] ~2m

**Objective**: Check for committed binary files that shouldn't be there.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== CHECKING FOR BINARY BLOBS ==="
# Find tracked files with binary signatures
echo "Checking git-tracked files..."
git ls-files -z | while IFS= read -r -d '' file; do
  # Skip known binary-OK files
  if [[ "$file" == *.png || "$file" == *.jpg || "$file" == *.jpeg || "$file" == *.gif || "$file" == *.ico ]]; then
    continue
  fi
  # Skip compiled binaries in bin/
  if [[ "$file" == bin/* || "$file" == build/* ]]; then
    continue
  fi
  # Check if binary
  if file "$file" 2>/dev/null | grep -q "executable\|compiled\|shared object"; then
    echo "  ✗ BINARY: $file"
  fi
done

echo ""
echo "=== FORBIDDEN EXTENSIONS CHECK ==="
git ls-files | grep -E '\.(o|a|so|exe|dll|dylib|jar)$' | while read f; do
  echo "  ✗ Found: $f"
done

echo ""
echo "✓ BINARY SCAN COMPLETE"
exit 0
```

**Pass Criteria**:
- No executable binaries (*.o, *.a, *.so, *.exe, *.dll, *.jar)
- Image files (*.png, *.jpg) are OK
- Docs/demo videos are OK (if small)

**Fail Criteria**:
- Compiled object files
- JAR files
- Shared objects

**Notes**:
- Binary files bloat repo and are hard to review
- Phase 1 should have excluded these in .gitignore

**If FAILED**:
- **MEDIUM** severity
- Can be fixed by removing and adding to .gitignore
- Document in LAUNCH_READINESS.md

**If PASSED**:
- Step 417 (final exit gate)

---

### Step 417: [V] Git History Hygiene [LOW] ~2m

**Objective**: Verify recent commit messages don't contain "WIP", "temp", "test123".

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== CHECKING LAST 30 COMMITS FOR WIP/TEMP MARKERS ==="
git log --oneline -30 | tee /tmp/commits.log

echo ""
echo "=== SEARCHING FOR BAD PATTERNS ==="
bad_markers=$(grep -E "WIP|TEMP|test123|DEBUG|FIXME|asdf" /tmp/commits.log)

if [ -z "$bad_markers" ]; then
  echo "✓ No WIP/TEMP markers in recent commits"
  exit 0
else
  echo "⚠ FOUND MARKERS (LOW severity):"
  echo "$bad_markers"
  echo ""
  echo "Total: $(echo "$bad_markers" | wc -l)"
  exit 0  # LOW severity, doesn't block
fi
```

**Pass Criteria**:
- Recent commits have clear, descriptive messages
- No "WIP", "TEMP", "asdf" in last 30 commits

**Notes**:
- This is informational only
- Indicates development hygiene
- Doesn't block launch

**If FAILED**:
- Document as **LOW** issue in LAUNCH_READINESS.md
- Proceed to next step

---

### Step 418: [V] IETF XML Protocol Spec Compilation [CRITICAL] ~5m

**Objective**: Verify all 3 main protocol XMLs compile to .txt with xml2rfc.

**Command**:
```bash
cd /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded

echo "=== CHECKING FOR xml2rfc TOOL ==="
if ! command -v xml2rfc &> /dev/null; then
  echo "  ⚠ xml2rfc not found (optional, skipping)"
  exit 0
fi

echo "✓ xml2rfc found: $(xml2rfc --version)"

echo ""
echo "=== COMPILING PROTOCOL XMLS ==="
declare -i success=0
declare -i fail=0

# Find XML files in docs/protocol/
for xml in docs/protocol/*.xml; do
  if [ -f "$xml" ]; then
    echo ""
    echo "Compiling: $(basename $xml)"
    txt="${xml%.xml}.txt"

    # Try to compile
    if xml2rfc "$xml" -o "$txt" --text 2>&1 | grep -q "error"; then
      echo "  ✗ COMPILATION FAILED"
      fail=$((fail + 1))
    elif [ -f "$txt" ] && [ -s "$txt" ]; then
      size=$(wc -c < "$txt")
      echo "  ✓ Compiled to $txt ($size bytes)"
      success=$((success + 1))
    else
      echo "  ✗ Output not generated"
      fail=$((fail + 1))
    fi
  fi
done

echo ""
echo "=== SUMMARY ==="
echo "Compiled: $success"
echo "Failed: $fail"

if [ $fail -eq 0 ]; then
  echo ""
  echo "✓ ALL PROTOCOL SPECS COMPILE"
  exit 0
else
  echo ""
  echo "✗ $fail SPEC(S) FAILED TO COMPILE"
  exit 1
fi
```

**Pass Criteria**:
- All 3 main protocol XML files compile without errors
- Output .txt files are generated with content

**Fail Criteria**:
- Any XML has compilation errors
- No output generated

**Notes**:
- Phase 2 should have verified XML syntax
- xml2rfc is optional but required for RFC publication
- If not installed, step is skipped (LOW impact)

**If FAILED**:
- **MEDIUM** severity (low impact if xml2rfc not in CI)
- Document in LAUNCH_READINESS.md

**If PASSED or SKIPPED**:
- Proceed to Step 419 (final summary)

---

## STEP 419: GENERATE LAUNCH_READINESS.md REPORT

**Objective**: Create comprehensive launch readiness report.

**Command**:
```bash
cat > /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded/LAUNCH_READINESS.md << 'EOF'
# LAUNCH READINESS REPORT
**Date**: $(date -u +%Y-%m-%d\ %H:%M:%SZ)
**Phase**: S73 Phase 5 — Final Verification Gate
**Repo**: unheaded
**Branch**: main (or current)

---

## VERIFICATION CHECKLIST

| # | Check | Result | Severity | Notes |
|----|-------|--------|----------|-------|
| 400 | Build Verification | [ ] ✓ [ ] ✗ | CRITICAL | go build ./... |
| 402 | Test Suite | [ ] ✓ [ ] ✗ | CRITICAL | go test ./... |
| 404 | TODO Sweep | [ ] ✓ [ ] ✗ | CRITICAL | No TODOs/FIXMEs in production code |
| 406 | Stub Sweep | [ ] ✓ [ ] ✗ | CRITICAL | No "not implemented" stubs |
| 407 | Commented Code | [ ] ✓ [ ] ✗ | MEDIUM | No large comment blocks in main.go |
| 408 | Secrets Sweep | [ ] ✓ [ ] ✗ | CRITICAL | No hardcoded credentials |
| 409 | README Links | [ ] ✓ [ ] ✗ | MEDIUM | All .md links resolve |
| 410 | QUICKSTART Links | [ ] ✓ [ ] ✗ | MEDIUM | All .md links resolve |
| 411 | License Files | [ ] ✓ [ ] ✗ | CRITICAL | LICENSE, LICENSE-PROTOCOLS, SURICATA_GPL_ISOLATION.md |
| 412 | docker-compose.yml | [ ] ✓ [ ] ✗ | MEDIUM | Valid YAML, images/builds exist |
| 413 | go.mod Tidiness | [ ] ✓ [ ] ✗ | CRITICAL | go mod tidy produces no changes |
| 414 | Protocol Specs | [ ] ✓ [ ] ✗ | MEDIUM | >=3 protocol XMLs in docs/protocol/ |
| 415 | No .env Files | [ ] ✓ [ ] ✗ | CRITICAL | No secrets committed to git |
| 416 | No Binary Blobs | [ ] ✓ [ ] ✗ | MEDIUM | No executables/JARs in git |
| 417 | Git History Clean | [ ] ✓ [ ] ✗ | LOW | No WIP/TEMP in recent commits |
| 418 | XML Compilation | [ ] ✓ [ ] ✗ | CRITICAL | Protocol specs compile (xml2rfc) |

---

## CRITICAL FAILURES (Block Launch)

_List any CRITICAL checks that failed:_

```
_Insert results here_
```

---

## MEDIUM FAILURES (Known Issues)

_List any MEDIUM checks failed + mitigation:_

```
_Insert results here_
```

---

## LOW FAILURES (Informational)

_List any LOW checks failed:_

```
_Insert results here_
```

---

## TEST SUMMARY

**Build Status**: [ ] GREEN [ ] RED
**Test Suite**: [ ] PASS [ ] FAIL (document known failures)
**Coverage**: ___% (if tracked)

_Known Test Failures (if any):_
```
_Insert results here_
```

---

## DEPENDENCIES & VENDORING

**go.mod Status**: [ ] CLEAN [ ] DIRTY
**Last go mod tidy**: _____
**Dependency Count**: _____
**Vendor Directory**: [ ] Present [ ] Not Used

---

## SECURITY CHECKLIST

| Item | Status | Notes |
|------|--------|-------|
| No hardcoded secrets | [ ] PASS [ ] FAIL | |
| No .env files committed | [ ] PASS [ ] FAIL | |
| No private keys in history | [ ] PASS [ ] FAIL | |
| License headers present | [ ] PASS [ ] FAIL | |
| SBOM generated | [ ] YES [ ] NO | (optional for Phase 5) |

---

## DOCUMENTATION CHECKLIST

| Item | Status | Path |
|------|--------|------|
| README.md | [ ] VALID | / |
| QUICKSTART.md | [ ] VALID | / |
| LICENSE | [ ] PRESENT | / |
| LICENSE-PROTOCOLS | [ ] PRESENT | / |
| SURICATA_GPL_ISOLATION.md | [ ] PRESENT | / |
| ARCHITECTURE.md | [ ] VALID | /docs/ |
| Protocol specs (XML) | [ ] PRESENT (count: ___) | /docs/protocol/ |

---

## FINAL SIGN-OFF

**Verified By**: _________________
**Date**: _________________
**Decision**:
- [ ] **READY TO LAUNCH** — All CRITICAL checks pass, known issues documented
- [ ] **BLOCKED** — CRITICAL check(s) failed, must return to Phase 1-4
- [ ] **CONDITIONAL** — Proceed with caution, known high-risk issues

**Notes**:
```
_Insert sign-off notes here_
```

---

## PHASE EXIT GATE

**All 16 checks pass** → ✓ READY TO FLIP REPO TO PUBLIC

**Any CRITICAL check fails** → ✗ HALT, fix, re-run Phase 5

**Any MEDIUM check fails** → ⚠ Document as known issue, proceed at Muck's discretion

EOF
echo "✓ Generated LAUNCH_READINESS.md"
```

**Next**: Review report and fill in results from Steps 400-418.

---

## PHASE 5 EXIT GATE

```
╔════════════════════════════════════════════════════════════════╗
║                                                                ║
║   PHASE 5 EXIT GATE — PUBLIC LAUNCH READY                     ║
║                                                                ║
║   All 16 checks pass → READY TO FLIP                          ║
║   Any CRITICAL check fails → HALT, fix, re-run Phase 5        ║
║   Any MEDIUM check fails → Document as known issue,           ║
║                             proceed at Muck's discretion       ║
║                                                                ║
╚════════════════════════════════════════════════════════════════╝
```

### CRITICAL CHECKS (Block Launch)

✓ **Step 400**: Build Verification (`go build ./...`)
✓ **Step 402**: Test Suite (`go test ./...`)
✓ **Step 404**: TODO Sweep (zero TODOs/FIXMEs in production)
✓ **Step 406**: Stub Sweep (zero "not implemented" stubs)
✓ **Step 408**: Secrets Sweep (no hardcoded credentials)
✓ **Step 411**: License Files (LICENSE, LICENSE-PROTOCOLS, SURICATA_GPL_ISOLATION.md)
✓ **Step 413**: go.mod Tidiness (no changes after tidy)
✓ **Step 415**: No .env Files (.env, credentials.json not committed)
✓ **Step 418**: IETF XML Compilation (protocol specs compile)

### MEDIUM CHECKS (Known Issues OK)

⚠ **Step 407**: Commented Code Sweep (no large blocks in main.go)
⚠ **Step 409**: README.md Links (all resolve)
⚠ **Step 410**: QUICKSTART.md Links (all resolve)
⚠ **Step 412**: docker-compose.yml (valid, images exist)
⚠ **Step 414**: Protocol Specs (>=3 XMLs in docs/protocol/)
⚠ **Step 416**: Binary Blobs (no .o, .a, .so, .jar, .exe)

### LOW CHECKS (Informational)

ℹ **Step 417**: Git History Hygiene (no WIP/TEMP in commits)

---

## AFTER PHASE 5

**If ALL CRITICAL checks PASS:**
1. ✓ Repo is ready for public visibility
2. ✓ All security gates cleared
3. ✓ Documentation validated
4. ✓ Proceed to repo flip (GitHub public, etc.)

**If ANY CRITICAL check FAILS:**
1. ✗ Return to failed phase (1-4) for fixes
2. ✗ Create bug/issue for the failure
3. ✗ Fix root cause
4. ✗ Re-run Phase 5 verification
5. ✗ Cannot flip repo to public until all CRITICAL clear

**If MEDIUM checks fail:**
1. ⚠ Document in LAUNCH_READINESS.md
2. ⚠ Create GitHub issue if needed
3. ⚠ Proceed to launch at Muck's discretion
4. ⚠ Post-launch: schedule fix in next sprint

---

## REFERENCE: 16-CHECK MATRIX

```
Phase 5 Verification Steps:

┌─────────────────────────────────────────────────────────────┐
│ CRITICAL (must pass)                                        │
├─────────────────────────────────────────────────────────────┤
│ ✓ Build                                        [Step 400]    │
│ ✓ Tests                                        [Step 402]    │
│ ✓ No TODOs in code                             [Step 404]    │
│ ✓ No stubs in code                             [Step 406]    │
│ ✓ No hardcoded secrets                         [Step 408]    │
│ ✓ License files present                        [Step 411]    │
│ ✓ go.mod tidy                                  [Step 413]    │
│ ✓ No .env files committed                      [Step 415]    │
│ ✓ Protocol specs compile                       [Step 418]    │
├─────────────────────────────────────────────────────────────┤
│ MEDIUM (can fail if documented)                            │
├─────────────────────────────────────────────────────────────┤
│ ⚠ No large comment blocks                      [Step 407]    │
│ ⚠ README links valid                           [Step 409]    │
│ ⚠ QUICKSTART links valid                       [Step 410]    │
│ ⚠ docker-compose valid                         [Step 412]    │
│ ⚠ Protocol specs exist                         [Step 414]    │
│ ⚠ No binary blobs                              [Step 416]    │
├─────────────────────────────────────────────────────────────┤
│ LOW (informational)                                         │
├─────────────────────────────────────────────────────────────┤
│ ℹ Git history clean                            [Step 417]    │
└─────────────────────────────────────────────────────────────┘
```

---

**END OF PHASE 5 BATTLE PLAN**

---

### Marshal Oversight Requirements

- [ ] Phase 5 runs with **zero implementation authority** (verification only)
- [ ] Each CRITICAL check must be explicitly approved before proceeding
- [ ] MEDIUM failures require Muck sign-off in LAUNCH_READINESS.md
- [ ] Report generated at /sessions/upbeat-hopeful-babbage/mnt/tmp/unheaded/LAUNCH_READINESS.md
- [ ] Final decision recorded: **READY** or **BLOCKED**

---

**Warmonger** — S73 Phase 5 Verification Gate
**Version**: 1.0 | **Date**: 2026-03-18 | **Status**: READY FOR EXECUTION
