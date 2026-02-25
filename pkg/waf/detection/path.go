// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package detection provides path traversal detection for THE SHIELD WAF
// TODO: Marked for Rust rebuild for performance - this Go implementation serves as reference
package detection

import (
	"regexp"
	"strings"
)

// PathTraversalDetector detects path traversal attacks with encoding bypass protection
// TODO: Port to Rust with SIMD-accelerated path normalization for 10x performance
type PathTraversalDetector struct {
	patterns          []*regexp.Regexp
	sensitiveFiles    map[string]int  // File patterns to risk scores
	sensitivePaths    map[string]int  // Path patterns to risk scores
	encodingPatterns  []EncodingLayer // Multi-layer encoding patterns
	strict            bool
}

// EncodingLayer represents an encoding transformation
type EncodingLayer struct {
	Name        string
	Pattern     *regexp.Regexp
	Decode      func(string) string
	Risk        int
}

// PathToken represents a component of a normalized path
type PathToken struct {
	Original   string
	Normalized string
	Encoding   string
	Risk       int
}

// NewPathTraversalDetector creates a new path traversal detector (replaces TraversalDetector)
// TODO: Rust rebuild should use trie-based path matching for O(n) detection
func NewPathTraversalDetector(strict bool) *PathTraversalDetector {
	patterns := []string{
		// Basic traversal patterns
		`\.\.\/`,
		`\.\.\\`,
		`\.\.`,
		`\.\/\.\.`,
		`\.\\\.\.`,

		// URL encoded - single encoding
		`%2e%2e%2f`,      // ../
		`%2e%2e\/`,       // ../
		`\.\.%2f`,        // ../
		`%2e%2e%5c`,      // ..\
		`%2e%2e\\`,       // ..\
		`\.\.%5c`,        // ..\
		`%2e\.%2f`,       // ../
		`\.%2e%2f`,       // ../
		`%2e\.\/`,        // ../
		`\.%2e\/`,        // ../

		// URL encoded - double encoding
		`%252e%252e%252f`, // ../
		`%252e%252e%255c`, // ..\
		`%252e%252e\/`,
		`%252e%252e\\`,

		// URL encoded - triple encoding
		`%25252e%25252e%25252f`,

		// Mixed case URL encoding
		`%2E%2E%2F`,
		`%2E%2E%5C`,

		// Unicode/UTF-8 encoded
		`%c0%ae%c0%ae%c0%af`,     // IIS specific
		`%c0%ae%c0%ae\/`,
		`%c1%9c`,                  // Backslash
		`%c0%af`,                  // Forward slash
		`%c1%1c`,                  // Backslash variant
		`%%32%65%%32%65%%32%66`,  // Overlong encoding
		`%e0%80%ae`,              // Overlong dot
		`%f0%80%80%ae`,           // 4-byte overlong dot

		// Unicode normalization bypass
		`\u002e\u002e\u002f`,
		`\u002e\u002e\u005c`,
		`\x2e\x2e\x2f`,
		`\x2e\x2e\x5c`,

		// Null byte injection (truncation attack)
		`%00`,
		`\x00`,
		`\0`,

		// Backslash normalization
		`\.\.\\\\`,
		`%5c%5c`,
		`\/\/`,

		// Absolute paths - Unix
		`^\/etc\/`,
		`^\/var\/`,
		`^\/tmp\/`,
		`^\/proc\/`,
		`^\/sys\/`,
		`^\/root\/`,
		`^\/home\/`,
		`^\/usr\/`,
		`^\/bin\/`,
		`^\/sbin\/`,
		`^\/opt\/`,
		`^\/dev\/`,
		`^\/mnt\/`,

		// Absolute paths - Windows
		`(?i)^[a-z]:`,
		`(?i)^[a-z]:\\`,
		`(?i)\\\\[a-z]`,  // UNC paths

		// Sensitive file patterns
		`(?i)\/etc\/passwd`,
		`(?i)\/etc\/shadow`,
		`(?i)\/etc\/group`,
		`(?i)\/etc\/gshadow`,
		`(?i)\/etc\/hosts`,
		`(?i)\/etc\/hostname`,
		`(?i)\/etc\/resolv\.conf`,
		`(?i)\/etc\/ssh\/`,
		`(?i)\/etc\/ssl\/`,
		`(?i)\/etc\/nginx\/`,
		`(?i)\/etc\/apache`,
		`(?i)\/etc\/httpd`,
		`(?i)\/etc\/mysql\/`,
		`(?i)\/etc\/postgresql\/`,
		`(?i)\/etc\/redis\/`,
		`(?i)\/etc\/mongod`,
		`(?i)\/etc\/sudoers`,
		`(?i)\/etc\/crontab`,
		`(?i)\/etc\/cron\.`,
		`(?i)\/etc\/profile`,
		`(?i)\/etc\/bash`,
		`(?i)\/etc\/environment`,

		// Linux proc filesystem
		`(?i)\/proc\/self\/`,
		`(?i)\/proc\/\d+\/`,
		`(?i)\/proc\/version`,
		`(?i)\/proc\/cmdline`,
		`(?i)\/proc\/mounts`,
		`(?i)\/proc\/net\/`,
		`(?i)\/proc\/environ`,

		// Linux dev filesystem
		`(?i)\/dev\/null`,
		`(?i)\/dev\/zero`,
		`(?i)\/dev\/random`,
		`(?i)\/dev\/urandom`,
		`(?i)\/dev\/stdin`,
		`(?i)\/dev\/stdout`,
		`(?i)\/dev\/stderr`,
		`(?i)\/dev\/fd\/`,
		`(?i)\/dev\/tcp\/`,
		`(?i)\/dev\/udp\/`,

		// Web server configs
		`(?i)\.htaccess`,
		`(?i)\.htpasswd`,
		`(?i)web\.config`,
		`(?i)httpd\.conf`,
		`(?i)nginx\.conf`,
		`(?i)apache2?\.conf`,

		// Application configs
		`(?i)wp-config\.php`,
		`(?i)config\.php`,
		`(?i)configuration\.php`,
		`(?i)settings\.php`,
		`(?i)database\.yml`,
		`(?i)database\.php`,
		`(?i)config\.yml`,
		`(?i)config\.yaml`,
		`(?i)config\.json`,
		`(?i)secrets\.yml`,
		`(?i)credentials\.json`,
		`(?i)application\.properties`,
		`(?i)application\.yml`,
		`(?i)\.env`,
		`(?i)\.env\.local`,
		`(?i)\.env\.production`,
		`(?i)\.env\.development`,

		// Version control
		`(?i)\.git\/`,
		`(?i)\.git\/config`,
		`(?i)\.git\/HEAD`,
		`(?i)\.gitignore`,
		`(?i)\.svn\/`,
		`(?i)\.svn\/entries`,
		`(?i)\.hg\/`,
		`(?i)\.hgignore`,
		`(?i)\.bzr\/`,
		`(?i)CVS\/`,

		// SSH/Keys
		`(?i)\.ssh\/`,
		`(?i)id_rsa`,
		`(?i)id_dsa`,
		`(?i)id_ecdsa`,
		`(?i)id_ed25519`,
		`(?i)authorized_keys`,
		`(?i)known_hosts`,
		`(?i)\.pem$`,
		`(?i)\.key$`,
		`(?i)\.crt$`,
		`(?i)\.cer$`,
		`(?i)\.p12$`,
		`(?i)\.pfx$`,

		// Windows specific
		`(?i)boot\.ini`,
		`(?i)win\.ini`,
		`(?i)system\.ini`,
		`(?i)autoexec\.bat`,
		`(?i)config\.sys`,
		`(?i)\\windows\\`,
		`(?i)\\system32\\`,
		`(?i)\\system\\`,
		`(?i)\\inetpub\\`,
		`(?i)\\wwwroot\\`,
		`(?i)\\users\\`,
		`(?i)\\documents and settings\\`,
		`(?i)sam$`,
		`(?i)ntds\.dit`,
		`(?i)security$`,
		`(?i)software$`,
		`(?i)system$`,

		// Log files
		`(?i)access\.log`,
		`(?i)error\.log`,
		`(?i)debug\.log`,
		`(?i)application\.log`,
		`(?i)\/var\/log\/`,
		`(?i)\.log$`,

		// Backup files
		`(?i)\.bak$`,
		`(?i)\.backup$`,
		`(?i)\.old$`,
		`(?i)\.orig$`,
		`(?i)\.save$`,
		`(?i)\.swp$`,
		`(?i)\.swo$`,
		`(?i)~$`,
		`(?i)\.copy$`,
		`(?i)\.tmp$`,
		`(?i)\.temp$`,
		`(?i)#.*#$`,

		// Database files
		`(?i)\.sql$`,
		`(?i)\.sqlite$`,
		`(?i)\.sqlite3$`,
		`(?i)\.db$`,
		`(?i)\.mdb$`,
		`(?i)\.accdb$`,
		`(?i)\.dbf$`,
		`(?i)dump\.sql`,
		`(?i)backup\.sql`,

		// Source code and IDE
		`(?i)\.idea\/`,
		`(?i)\.vscode\/`,
		`(?i)\.project$`,
		`(?i)\.classpath$`,
		`(?i)\.settings\/`,
		`(?i)\.DS_Store`,
		`(?i)Thumbs\.db`,

		// Package manager files
		`(?i)package\.json`,
		`(?i)package-lock\.json`,
		`(?i)yarn\.lock`,
		`(?i)composer\.json`,
		`(?i)composer\.lock`,
		`(?i)Gemfile`,
		`(?i)Gemfile\.lock`,
		`(?i)requirements\.txt`,
		`(?i)Pipfile`,
		`(?i)Pipfile\.lock`,
		`(?i)go\.mod`,
		`(?i)go\.sum`,
		`(?i)Cargo\.toml`,
		`(?i)Cargo\.lock`,

		// CI/CD configs
		`(?i)\.travis\.yml`,
		`(?i)\.gitlab-ci\.yml`,
		`(?i)\.github\/`,
		`(?i)Jenkinsfile`,
		`(?i)\.circleci\/`,
		`(?i)azure-pipelines\.yml`,
		`(?i)\.drone\.yml`,

		// Docker/Container
		`(?i)Dockerfile`,
		`(?i)docker-compose\.yml`,
		`(?i)docker-compose\.yaml`,
		`(?i)\.dockerignore`,
		`(?i)kubernetes\.yml`,
		`(?i)k8s\.yml`,

		// Cloud configs
		`(?i)\.aws\/`,
		`(?i)\.azure\/`,
		`(?i)\.gcloud\/`,
		`(?i)credentials$`,
		`(?i)\.boto`,
		`(?i)terraform\.tfstate`,
		`(?i)\.terraform\/`,
	}

	// Sensitive files with risk scores
	sensitiveFiles := map[string]int{
		"passwd":          25, "shadow":      30, "group":       20,
		"hosts":           15, "sudoers":    25, "crontab":    20,
		"id_rsa":          30, "id_dsa":     30, "id_ecdsa":   30,
		"id_ed25519":      30, "authorized_keys": 25,
		"htaccess":        20, "htpasswd":   25, "web.config": 20,
		"wp-config.php":   25, "config.php": 20, ".env":       25,
		"database.yml":    25, "secrets.yml": 30,
		".git/config":     20, "HEAD":       15,
		"boot.ini":        20, "win.ini":   15, "sam":         30,
		"application.properties": 20,
	}

	// Sensitive path prefixes with risk scores
	sensitivePaths := map[string]int{
		"/etc/":   20, "/proc/":   25, "/sys/":    20,
		"/root/":  25, "/var/log/": 15, "/dev/":   20,
		"/.ssh/":  25, "/.git/":   20, "/.svn/":  15,
		"/home/":  15, "/tmp/":    10, "/usr/":   10,
	}

	detector := &PathTraversalDetector{
		patterns:         make([]*regexp.Regexp, 0, len(patterns)),
		sensitiveFiles:   sensitiveFiles,
		sensitivePaths:   sensitivePaths,
		encodingPatterns: buildEncodingPatterns(),
		strict:           strict,
	}

	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err == nil {
			detector.patterns = append(detector.patterns, re)
		}
	}

	return detector
}

// buildEncodingPatterns creates the encoding detection patterns
func buildEncodingPatterns() []EncodingLayer {
	return []EncodingLayer{
		{
			Name:    "url_single",
			Pattern: regexp.MustCompile(`%[0-9a-fA-F]{2}`),
			Decode:  urlDecodeSingle,
			Risk:    3,
		},
		{
			Name:    "url_double",
			Pattern: regexp.MustCompile(`%25[0-9a-fA-F]{2}`),
			Decode:  urlDecodeDouble,
			Risk:    8,
		},
		{
			Name:    "url_triple",
			Pattern: regexp.MustCompile(`%2525[0-9a-fA-F]{2}`),
			Decode:  urlDecodeTriple,
			Risk:    15,
		},
		{
			Name:    "unicode",
			Pattern: regexp.MustCompile(`\\u[0-9a-fA-F]{4}`),
			Decode:  unicodeDecode,
			Risk:    10,
		},
		{
			Name:    "hex",
			Pattern: regexp.MustCompile(`\\x[0-9a-fA-F]{2}`),
			Decode:  hexDecode,
			Risk:    8,
		},
		{
			Name:    "overlong_utf8",
			Pattern: regexp.MustCompile(`%c[01][%0-9a-fA-F]{2}`),
			Decode:  nil, // Just detection
			Risk:    20,
		},
	}
}

// Detect checks if the input contains path traversal
func (d *PathTraversalDetector) Detect(input string) bool {
	// Normalize through multiple encoding layers
	normalized := d.normalizeMultiLayer(input)

	for _, pattern := range d.patterns {
		if pattern.MatchString(normalized) {
			return true
		}
	}

	// Check original input as well (for encoded attacks)
	for _, pattern := range d.patterns {
		if pattern.MatchString(input) {
			return true
		}
	}

	return false
}

// DetectWithDetails returns detailed detection results
// TODO: Rust version should include byte-level position information
func (d *PathTraversalDetector) DetectWithDetails(input string) *DetectionResult {
	result := &DetectionResult{
		Type:    "traversal",
		Input:   input,
		Matches: make([]string, 0),
	}

	// Detect encoding layers used
	encodingRisk := d.analyzeEncodings(input)

	// Normalize through multiple layers
	normalized := d.normalizeMultiLayer(input)

	// Pattern matching on both original and normalized
	for _, pattern := range d.patterns {
		// Check normalized
		matches := pattern.FindAllString(normalized, -1)
		if len(matches) > 0 {
			result.Detected = true
			result.Matches = append(result.Matches, matches...)
		}

		// Check original (catches encoded patterns)
		origMatches := pattern.FindAllString(input, -1)
		if len(origMatches) > 0 {
			result.Detected = true
			for _, m := range origMatches {
				if !contains(result.Matches, m) {
					result.Matches = append(result.Matches, m)
				}
			}
		}
	}

	// Path analysis
	pathRisk := d.analyzePath(normalized)

	// Sensitive file detection
	fileRisk := d.analyzeSensitiveFiles(normalized)

	if pathRisk >= 15 || fileRisk >= 20 || encodingRisk >= 10 {
		result.Detected = true
	}

	if result.Detected {
		result.Score = d.calculateScore(len(result.Matches), pathRisk, fileRisk, encodingRisk)
		result.Message = "Path traversal attempt detected"
	}

	return result
}

// analyzeEncodings checks for suspicious encoding patterns
func (d *PathTraversalDetector) analyzeEncodings(input string) int {
	risk := 0

	for _, enc := range d.encodingPatterns {
		if enc.Pattern.MatchString(input) {
			risk += enc.Risk
		}
	}

	// Check for null bytes (used to truncate strings)
	if strings.Contains(input, "%00") || strings.Contains(input, "\x00") {
		risk += 15
	}

	// Check for mixed encoding (very suspicious)
	encodingCount := 0
	for _, enc := range d.encodingPatterns {
		if enc.Pattern.MatchString(input) {
			encodingCount++
		}
	}
	if encodingCount >= 2 {
		risk += encodingCount * 5
	}

	return risk
}

// analyzePath analyzes the path structure for traversal
func (d *PathTraversalDetector) analyzePath(path string) int {
	risk := 0

	// Count traversal sequences
	traversalCount := strings.Count(path, "../") + strings.Count(path, "..\\")
	if traversalCount > 0 {
		risk += traversalCount * 8
	}

	// Deep traversal is more suspicious
	if traversalCount >= 3 {
		risk += 10
	}

	// Check for sensitive path prefixes
	lower := strings.ToLower(path)
	for prefix, score := range d.sensitivePaths {
		if strings.Contains(lower, prefix) {
			risk += score
		}
	}

	// Check for absolute path indicators
	if strings.HasPrefix(path, "/") || strings.HasPrefix(lower, "c:") || strings.HasPrefix(path, "\\\\") {
		risk += 10
	}

	return risk
}

// analyzeSensitiveFiles checks for sensitive file access
func (d *PathTraversalDetector) analyzeSensitiveFiles(path string) int {
	risk := 0
	lower := strings.ToLower(path)

	for file, score := range d.sensitiveFiles {
		if strings.Contains(lower, strings.ToLower(file)) {
			risk += score
		}
	}

	return risk
}

// normalizeMultiLayer decodes multiple encoding layers
// TODO: Rust version should be iterative with max depth limit
func (d *PathTraversalDetector) normalizeMultiLayer(input string) string {
	result := input
	maxIterations := 5 // Prevent infinite loops

	for i := 0; i < maxIterations; i++ {
		prev := result

		// URL decode
		result = urlDecodeSingle(result)

		// Handle path separators
		result = strings.ReplaceAll(result, "\\", "/")
		result = strings.ReplaceAll(result, "//", "/")

		// Handle null bytes
		result = strings.ReplaceAll(result, "\x00", "")

		// If no changes, we're done
		if result == prev {
			break
		}
	}

	// Final normalization
	result = strings.ToLower(result)

	return result
}

// normalize normalizes the input for detection (compatibility method)
func (d *PathTraversalDetector) normalize(input string) string {
	return d.normalizeMultiLayer(input)
}

// calculateScore calculates the anomaly score
func (d *PathTraversalDetector) calculateScore(matchCount, pathRisk, fileRisk, encodingRisk int) int {
	baseScore := 10

	if matchCount > 0 {
		baseScore += matchCount * 5
	}

	baseScore += pathRisk / 2
	baseScore += fileRisk / 2
	baseScore += encodingRisk

	if baseScore > 100 {
		return 100
	}

	return baseScore
}

// GetPatternCount returns the number of patterns
func (d *PathTraversalDetector) GetPatternCount() int {
	return len(d.patterns)
}

// Helper functions for URL decoding

func urlDecodeSingle(input string) string {
	result := input

	// Common URL encoded characters
	replacements := map[string]string{
		"%2e": ".", "%2E": ".",
		"%2f": "/", "%2F": "/",
		"%5c": "\\", "%5C": "\\",
		"%00": "\x00",
		"%20": " ",
		"%3a": ":", "%3A": ":",
		"%3f": "?", "%3F": "?",
		"%26": "&",
		"%3d": "=", "%3D": "=",
		"%23": "#",
		"%25": "%",
		"%7e": "~", "%7E": "~",
	}

	for encoded, decoded := range replacements {
		result = strings.ReplaceAll(result, encoded, decoded)
	}

	return result
}

func urlDecodeDouble(input string) string {
	result := input

	// Double encoded characters
	replacements := map[string]string{
		"%252e": "%2e", "%252E": "%2E",
		"%252f": "%2f", "%252F": "%2F",
		"%255c": "%5c", "%255C": "%5C",
		"%2500": "%00",
		"%2520": "%20",
		"%2525": "%25",
	}

	for encoded, decoded := range replacements {
		result = strings.ReplaceAll(result, encoded, decoded)
	}

	return urlDecodeSingle(result)
}

func urlDecodeTriple(input string) string {
	result := input

	// Triple encoded characters
	replacements := map[string]string{
		"%25252e": "%252e",
		"%25252f": "%252f",
		"%25255c": "%255c",
		"%252500": "%2500",
		"%252525": "%2525",
	}

	for encoded, decoded := range replacements {
		result = strings.ReplaceAll(result, encoded, decoded)
	}

	return urlDecodeDouble(result)
}

func unicodeDecode(input string) string {
	// Simple unicode escape decoding
	result := input

	unicodeMap := map[string]string{
		"\\u002e": ".", "\\u002E": ".",
		"\\u002f": "/", "\\u002F": "/",
		"\\u005c": "\\", "\\u005C": "\\",
	}

	for encoded, decoded := range unicodeMap {
		result = strings.ReplaceAll(result, encoded, decoded)
	}

	return result
}

func hexDecode(input string) string {
	result := input

	hexMap := map[string]string{
		"\\x2e": ".", "\\x2E": ".",
		"\\x2f": "/", "\\x2F": "/",
		"\\x5c": "\\", "\\x5C": "\\",
		"\\x00": "\x00",
	}

	for encoded, decoded := range hexMap {
		result = strings.ReplaceAll(result, encoded, decoded)
	}

	return result
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// NormalizePath normalizes a path by resolving traversal sequences
// WARNING: This is for analysis only - do not use for actual path resolution
func NormalizePath(path string) string {
	// Split into components
	parts := strings.Split(path, "/")
	var result []string

	for _, part := range parts {
		switch part {
		case "", ".":
			// Skip empty and current dir references
			continue
		case "..":
			// Go up one level if possible
			if len(result) > 0 && result[len(result)-1] != ".." {
				result = result[:len(result)-1]
			} else {
				result = append(result, part)
			}
		default:
			result = append(result, part)
		}
	}

	normalized := strings.Join(result, "/")
	if strings.HasPrefix(path, "/") {
		normalized = "/" + normalized
	}

	return normalized
}

// IsPathSafe checks if a path is safe (no traversal outside base)
func IsPathSafe(basePath, requestedPath string) bool {
	// Normalize both paths
	baseNorm := NormalizePath(basePath)
	reqNorm := NormalizePath(requestedPath)

	// Check if the normalized requested path starts with base path
	return strings.HasPrefix(reqNorm, baseNorm)
}

// CountTraversalDepth counts how many levels up a path attempts to traverse
func CountTraversalDepth(path string) int {
	depth := 0
	maxDepth := 0

	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == ".." {
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		} else if part != "" && part != "." {
			if depth > 0 {
				depth--
			}
		}
	}

	return maxDepth
}

// GetFileExtension safely extracts the file extension
func GetFileExtension(path string) string {
	// Remove query string and fragment
	path = strings.Split(path, "?")[0]
	path = strings.Split(path, "#")[0]

	// Handle null byte truncation attempts
	path = strings.Split(path, "\x00")[0]

	// Find last dot
	lastDot := strings.LastIndex(path, ".")
	lastSlash := strings.LastIndex(path, "/")

	// Ensure dot is after last slash
	if lastDot > lastSlash {
		return strings.ToLower(path[lastDot:])
	}

	return ""
}

// IsDangerousExtension checks if file extension is dangerous
func IsDangerousExtension(ext string) bool {
	dangerous := map[string]bool{
		".php": true, ".phtml": true, ".php3": true, ".php4": true, ".php5": true,
		".asp": true, ".aspx": true, ".ascx": true,
		".jsp": true, ".jspx": true,
		".cgi": true, ".pl": true,
		".py": true, ".pyc": true,
		".rb": true,
		".sh": true, ".bash": true,
		".exe": true, ".dll": true, ".bat": true, ".cmd": true, ".ps1": true,
		".htaccess": true, ".htpasswd": true,
		".ini": true, ".config": true, ".conf": true,
	}

	return dangerous[strings.ToLower(ext)]
}

// DetectPathAnomaly detects anomalies in path structure
func (d *PathTraversalDetector) DetectPathAnomaly(path string) []string {
	var anomalies []string

	// Check for null bytes
	if strings.Contains(path, "\x00") || strings.Contains(path, "%00") {
		anomalies = append(anomalies, "null_byte_injection")
	}

	// Check for double extensions
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		if strings.Count(filename, ".") > 1 {
			anomalies = append(anomalies, "double_extension")
		}
	}

	// Check for very long paths
	if len(path) > 1024 {
		anomalies = append(anomalies, "excessive_length")
	}

	// Check for repeated slashes
	if strings.Contains(path, "//") || strings.Contains(path, "\\\\") {
		anomalies = append(anomalies, "repeated_separators")
	}

	// Check for mixed path separators
	if strings.Contains(path, "/") && strings.Contains(path, "\\") {
		anomalies = append(anomalies, "mixed_separators")
	}

	// Check for control characters
	for _, r := range path {
		if r < 32 && r != '\t' {
			anomalies = append(anomalies, "control_characters")
			break
		}
	}

	return anomalies
}

// TraversalDetector is an alias for backwards compatibility
type TraversalDetector = PathTraversalDetector

// NewTraversalDetector creates a new traversal detector (backwards compatibility)
func NewTraversalDetector(strict bool) *TraversalDetector {
	return NewPathTraversalDetector(strict)
}
