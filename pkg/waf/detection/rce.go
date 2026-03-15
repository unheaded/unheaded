// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package detection provides remote code execution detection
package detection

import (
	"regexp"
	"strings"
)

// RCEDetector detects Remote Code Execution attacks
type RCEDetector struct {
	patterns []*regexp.Regexp
	strict   bool
}

// NewRCEDetector creates a new RCE detector
func NewRCEDetector(strict bool) *RCEDetector {
	patterns := []string{
		// Command chaining
		`;\s*\w`,
		`\|\s*\w`,
		`\|\|\s*\w`,
		`&&\s*\w`,
		`&\s*\w`,

		// Command substitution
		`\$\([^)]+\)`,
		"`[^`]+`",
		`\$\{[^}]+\}`,

		// Common dangerous commands
		`(?i)\b(cat|head|tail|less|more|tac)\s+[\/\w\.]`,
		`(?i)\b(ls|dir|find|locate)\s+[\/\w\.-]*`,
		`(?i)\b(rm|del|unlink)\s+`,
		`(?i)\b(mv|cp|copy|move)\s+`,
		`(?i)\b(chmod|chown|chgrp)\s+`,
		`(?i)\b(wget|curl|fetch)\s+`,
		`(?i)\b(nc|netcat|ncat)\s+`,
		`(?i)\b(bash|sh|zsh|ksh|csh|tcsh|dash)\s+`,
		`(?i)\b(python|python3|perl|ruby|php|node)\s+`,
		`(?i)\bexec\s*\(`,
		`(?i)\bsystem\s*\(`,
		`(?i)\bpassthru\s*\(`,
		`(?i)\bshell_exec\s*\(`,
		`(?i)\bpopen\s*\(`,
		`(?i)\bproc_open\s*\(`,
		`(?i)\bpcntl_exec\s*\(`,
		`(?i)\beval\s*\(`,

		// Reverse shell patterns
		`(?i)\/dev\/tcp\/`,
		`(?i)\/dev\/udp\/`,
		`(?i)mkfifo`,
		`(?i)\bnc\s+-[elp]`,
		`(?i)bash\s+-i`,
		`(?i)\/bin\/(ba)?sh`,

		// Environment manipulation
		`(?i)\bexport\s+\w+=`,
		`(?i)\benv\s+\w+=`,
		`(?i)\bsetenv\s+`,

		// Process control
		`(?i)\bkill\s+-\d+`,
		`(?i)\bkillall\s+`,
		`(?i)\bpkill\s+`,

		// File operations
		`(?i)\b(echo|printf)\s+.*>`,
		`(?i)>\s*\/[a-z]`,
		`(?i)>>\s*\/[a-z]`,

		// Network operations
		`(?i)\bssh\s+`,
		`(?i)\bscp\s+`,
		`(?i)\brsync\s+`,
		`(?i)\bftp\s+`,
		`(?i)\btelnet\s+`,

		// Cron/scheduled tasks
		`(?i)\bcrontab\s+`,
		`(?i)\/etc\/cron`,

		// User/privilege escalation
		`(?i)\bsudo\s+`,
		`(?i)\bsu\s+`,
		`(?i)\buseradd\s+`,
		`(?i)\badduser\s+`,
		`(?i)\bpasswd\s+`,

		// Archive operations
		`(?i)\btar\s+[xczf]`,
		`(?i)\bunzip\s+`,
		`(?i)\bgunzip\s+`,

		// Encoding bypass attempts
		`(?i)\bbase64\s+`,
		`(?i)\bxxd\s+`,
		`(?i)\bhexdump\s+`,

		// Windows specific
		`(?i)\bcmd\s*\/c`,
		`(?i)\bcmd\.exe`,
		`(?i)\bpowershell`,
		`(?i)\bwscript`,
		`(?i)\bcscript`,
		`(?i)\.bat\s*$`,
		`(?i)\.cmd\s*$`,
		`(?i)\.ps1\s*$`,

		// PHP specific
		`(?i)\bpreg_replace\s*\(.*\/[a-z]*e`,
		`(?i)\bassert\s*\(`,
		`(?i)\bcreate_function\s*\(`,

		// Python specific
		`(?i)\b__import__\s*\(`,
		`(?i)\bos\.system\s*\(`,
		`(?i)\bos\.popen\s*\(`,
		`(?i)\bsubprocess\s*\.`,

		// Java specific
		`(?i)Runtime\.getRuntime\(\)\.exec`,
		`(?i)ProcessBuilder`,
	}

	detector := &RCEDetector{
		patterns: make([]*regexp.Regexp, 0, len(patterns)),
		strict:   strict,
	}

	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err == nil {
			detector.patterns = append(detector.patterns, re)
		}
	}

	return detector
}

// Detect checks if the input contains RCE attempts
func (d *RCEDetector) Detect(input string) bool {
	normalized := d.normalize(input)

	for _, pattern := range d.patterns {
		if pattern.MatchString(normalized) {
			return true
		}
	}
	return false
}

// DetectWithDetails returns detailed detection results
func (d *RCEDetector) DetectWithDetails(input string) *DetectionResult {
	result := &DetectionResult{
		Type:    "rce",
		Input:   input,
		Matches: make([]string, 0),
	}

	normalized := d.normalize(input)

	for _, pattern := range d.patterns {
		matches := pattern.FindAllString(normalized, -1)
		if len(matches) > 0 {
			result.Detected = true
			result.Matches = append(result.Matches, matches...)
		}
	}

	if result.Detected {
		result.Score = d.calculateScore(len(result.Matches))
		result.Message = "Remote code execution attempt detected"
	}

	return result
}

// normalize normalizes the input for detection
func (d *RCEDetector) normalize(input string) string {
	result := input

	// URL decode
	result = strings.ReplaceAll(result, "%20", " ")
	result = strings.ReplaceAll(result, "%7c", "|")
	result = strings.ReplaceAll(result, "%26", "&")
	result = strings.ReplaceAll(result, "%3b", ";")
	result = strings.ReplaceAll(result, "%60", "`")
	result = strings.ReplaceAll(result, "%24", "$")
	result = strings.ReplaceAll(result, "%28", "(")
	result = strings.ReplaceAll(result, "%29", ")")
	result = strings.ReplaceAll(result, "%7b", "{")
	result = strings.ReplaceAll(result, "%7d", "}")
	result = strings.ReplaceAll(result, "+", " ")

	// Remove null bytes
	result = strings.ReplaceAll(result, "\x00", "")
	result = strings.ReplaceAll(result, "%00", "")

	return result
}

// calculateScore calculates the anomaly score based on matches
func (d *RCEDetector) calculateScore(matchCount int) int {
	// RCE is critical, high base score
	baseScore := 20
	if matchCount > 1 {
		return baseScore + (matchCount-1)*10
	}
	return baseScore
}

// GetPatternCount returns the number of patterns
func (d *RCEDetector) GetPatternCount() int {
	return len(d.patterns)
}
