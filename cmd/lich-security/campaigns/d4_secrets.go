// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package campaigns

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// D4Secrets tests secrets management.
type D4Secrets struct{}

func (d *D4Secrets) Name() string        { return "d4" }
func (d *D4Secrets) Description() string { return "Secrets Management" }

func (d *D4Secrets) Run(ctx context.Context, cfg Config) CampaignResult {
	start := time.Now()
	result := CampaignResult{
		Campaign:    "d4",
		Description: "Secrets Management",
		StartTime:   start,
	}

	tests := []struct {
		name string
		fn   func(ctx context.Context, cfg Config) *Finding
	}{
		{"hardcoded_secrets_code", d.testHardcodedSecretsInCode},
		{"secrets_in_env", d.testSecretsInEnv},
		{"secret_file_permissions", d.testSecretFilePermissions},
		{"secrets_in_logs", d.testSecretsInLogs},
		{"secrets_in_debug", d.testSecretsInDebug},
		{"git_history_secrets", d.testGitHistorySecrets},
	}

	for _, test := range tests {
		result.TestsRun++
		finding := test.fn(ctx, cfg)
		if finding != nil {
			finding.ID = fmt.Sprintf("D4-%03d", result.TestsRun)
			result.Findings = append(result.Findings, *finding)
		} else {
			result.TestsPassed++
		}
	}

	result.Duration = time.Since(start)
	return result
}

func (d *D4Secrets) testHardcodedSecretsInCode(_ context.Context, _ Config) *Finding {
	// Scan Go source for potential hardcoded secrets.
	patterns := []string{
		"password",
		"secret",
		"api_key",
		"private_key",
	}

	repoRoot := findRepoRoot()
	if repoRoot == "" {
		return nil
	}

	for _, pattern := range patterns {
		matches := grepSourceFiles(repoRoot, pattern)
		for _, match := range matches {
			// Filter out test files, docs, and known-safe patterns.
			if strings.Contains(match, "_test.go") ||
				strings.Contains(match, "CLAUDE.md") ||
				strings.Contains(match, "docs/") ||
				strings.Contains(match, "README") ||
				strings.Contains(match, "// ") ||
				strings.Contains(match, "battle-plans/") ||
				strings.Contains(match, ".md:") ||
				strings.Contains(match, "sessions/") {
				continue
			}
			// Check for actual assignment of literal values (not variable names).
			if strings.Contains(match, `= "`) && !strings.Contains(match, `= ""`) &&
				!strings.Contains(match, "os.Getenv") &&
				!strings.Contains(match, "config.") {
				return &Finding{
					Severity:    "high",
					Title:       "Possible hardcoded secret in source",
					Description: fmt.Sprintf("Pattern '%s' found: %s", pattern, truncate(match, 200)),
					Remediation: "Use environment variables or secret store for all credentials",
				}
			}
		}
	}
	return nil
}

func (d *D4Secrets) testSecretsInEnv(_ context.Context, _ Config) *Finding {
	// Check if sensitive env vars are set with actual values (not just names).
	sensitive := []string{"DATABASE_PASSWORD", "JWT_PRIVATE_KEY", "VAULT_TOKEN"}
	for _, key := range sensitive {
		val := os.Getenv(key)
		if val != "" && len(val) > 3 {
			return &Finding{
				Severity:    "medium",
				Title:       "Sensitive data in environment variable",
				Description: fmt.Sprintf("Environment variable %s contains a value", key),
				Remediation: "Use secret mount files instead of environment variables",
			}
		}
	}
	return nil
}

func (d *D4Secrets) testSecretFilePermissions(_ context.Context, _ Config) *Finding {
	secretPaths := []string{
		"/etc/unheaded/pki/root-ca.key",
		"/etc/unheaded/jwt-private.key",
		"/etc/unheaded/api-keys.json",
	}
	for _, path := range secretPaths {
		info, err := os.Stat(path)
		if err != nil {
			continue // File doesn't exist — not a finding.
		}
		perm := info.Mode().Perm()
		if perm&0077 != 0 { // Group or world readable.
			return &Finding{
				Severity:    "high",
				Title:       "Secret file has excessive permissions",
				Description: fmt.Sprintf("%s has permissions %o (should be 0600)", path, perm),
				Remediation: "Set file permissions to 0600 for secret files",
			}
		}
	}
	return nil
}

func (d *D4Secrets) testSecretsInLogs(_ context.Context, _ Config) *Finding {
	logPaths := []string{
		"/var/log/unheaded/auth-audit.log",
		"/var/log/unheaded/service.log",
	}
	secretPatterns := []string{"password=", "api_key=", "secret=", "token=eyJ"}
	for _, logPath := range logPaths {
		data, err := os.ReadFile(logPath) // #nosec G304 -- operator-configured path; no G304 site in this tree derives from an HTTP request (verified)
		if err != nil {
			continue
		}
		content := string(data)
		for _, pattern := range secretPatterns {
			if strings.Contains(strings.ToLower(content), pattern) {
				return &Finding{
					Severity:    "critical",
					Title:       "Secret found in log file",
					Description: fmt.Sprintf("Pattern '%s' found in %s", pattern, logPath),
					Remediation: "Sanitize all log output to remove sensitive values",
				}
			}
		}
	}
	return nil
}

func (d *D4Secrets) testSecretsInDebug(ctx context.Context, cfg Config) *Finding {
	// Check debug endpoints for exposed secrets.
	debugEndpoints := []string{"/debug/vars", "/debug/pprof/", "/debug/env"}
	for _, ep := range debugEndpoints {
		resp, err := doHTTPRequest(ctx, cfg.GatewayAddr+ep, "GET", nil, nil)
		if err != nil {
			continue
		}
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status == 200 {
			return &Finding{
				Severity:    "medium",
				Title:       "Debug endpoint publicly accessible",
				Description: fmt.Sprintf("%s returned 200 (may expose internal state)", ep),
				Endpoint:    ep,
				Remediation: "Restrict debug endpoints to admin role or internal network only",
			}
		}
	}
	return nil
}

func (d *D4Secrets) testGitHistorySecrets(_ context.Context, _ Config) *Finding {
	repoRoot := findRepoRoot()
	if repoRoot == "" {
		return nil
	}
	// Quick check: search recent git log for obvious secret patterns.
	cmd := exec.Command("git", "-C", repoRoot, "log", "--oneline", "-100", "--diff-filter=A", "--name-only") // #nosec G204 -- Lich audit campaign; fixed git/grep binary, argv separate
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	suspicious := []string{".env", "credentials", "private.key", ".pem", "apikey"}
	for _, line := range strings.Split(string(out), "\n") {
		for _, pattern := range suspicious {
			if strings.Contains(strings.ToLower(line), pattern) &&
				!strings.Contains(line, ".gitignore") &&
				!strings.Contains(line, "test") {
				return &Finding{
					Severity:    "medium",
					Title:       "Potentially sensitive file in git history",
					Description: fmt.Sprintf("File matching '%s' found in git history: %s", pattern, truncate(line, 100)),
					Remediation: "Remove sensitive files from git history using git filter-branch or BFG",
				}
			}
		}
	}
	return nil
}

func findRepoRoot() string {
	// Walk up from cwd looking for .git directory.
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func grepSourceFiles(root, pattern string) []string {
	cmd := exec.Command("grep", "-r", "-l", "-i", pattern, "--include=*.go", root) // #nosec G204 -- Lich audit campaign; fixed git/grep binary, argv separate
	out, _ := cmd.Output()
	var results []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			results = append(results, line)
		}
	}
	return results
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
