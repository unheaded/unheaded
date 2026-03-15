// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package detection provides attack detection patterns for THE SHIELD WAF
// TODO: Marked for Rust rebuild for performance - this Go implementation serves as reference
package detection

import (
	"regexp"
	"strings"
	"unicode"
)

// SQLiDetector detects SQL injection attacks using pattern matching and tokenization
// TODO: Port to Rust with SIMD-accelerated pattern matching for 10x performance
type SQLiDetector struct {
	patterns     []*regexp.Regexp
	keywords     map[string]int // SQL keywords with severity weights
	operators    map[string]int // SQL operators with severity weights
	functions    map[string]int // SQL functions with severity weights
	strict       bool
	tokenizer    *SQLTokenizer
}

// SQLTokenizer performs lexical analysis on potential SQL injection payloads
// TODO: Rust rebuild should use nom parser combinator for zero-copy tokenization
type SQLTokenizer struct {
	sqlKeywords   map[string]TokenType
	sqlOperators  map[string]TokenType
	sqlFunctions  map[string]TokenType
}

// TokenType represents the type of SQL token
type TokenType int

const (
	TokenUnknown TokenType = iota
	TokenKeyword
	TokenOperator
	TokenFunction
	TokenString
	TokenNumber
	TokenComment
	TokenIdentifier
	TokenWhitespace
	TokenSpecial
)

// Token represents a parsed SQL token
type Token struct {
	Type    TokenType
	Value   string
	Start   int
	End     int
	Risk    int // Risk score for this token
}

// NewSQLTokenizer creates a new SQL tokenizer
func NewSQLTokenizer() *SQLTokenizer {
	return &SQLTokenizer{
		sqlKeywords: map[string]TokenType{
			"select": TokenKeyword, "insert": TokenKeyword, "update": TokenKeyword,
			"delete": TokenKeyword, "drop": TokenKeyword, "truncate": TokenKeyword,
			"alter": TokenKeyword, "create": TokenKeyword, "grant": TokenKeyword,
			"revoke": TokenKeyword, "from": TokenKeyword, "into": TokenKeyword,
			"where": TokenKeyword, "and": TokenKeyword, "or": TokenKeyword,
			"not": TokenKeyword, "null": TokenKeyword, "like": TokenKeyword,
			"in": TokenKeyword, "between": TokenKeyword, "exists": TokenKeyword,
			"having": TokenKeyword, "group": TokenKeyword, "order": TokenKeyword,
			"by": TokenKeyword, "limit": TokenKeyword, "offset": TokenKeyword,
			"union": TokenKeyword, "all": TokenKeyword, "distinct": TokenKeyword,
			"join": TokenKeyword, "inner": TokenKeyword, "outer": TokenKeyword,
			"left": TokenKeyword, "right": TokenKeyword, "cross": TokenKeyword,
			"table": TokenKeyword, "database": TokenKeyword, "schema": TokenKeyword,
			"index": TokenKeyword, "view": TokenKeyword, "procedure": TokenKeyword,
			"function": TokenKeyword, "trigger": TokenKeyword, "exec": TokenKeyword,
			"execute": TokenKeyword, "set": TokenKeyword, "declare": TokenKeyword,
			"case": TokenKeyword, "when": TokenKeyword, "then": TokenKeyword,
			"else": TokenKeyword, "end": TokenKeyword, "as": TokenKeyword,
			"on": TokenKeyword, "values": TokenKeyword, "desc": TokenKeyword,
			"asc": TokenKeyword, "top": TokenKeyword, "waitfor": TokenKeyword,
			"delay": TokenKeyword, "shutdown": TokenKeyword, "xp_": TokenKeyword,
		},
		sqlOperators: map[string]TokenType{
			"=": TokenOperator, "<>": TokenOperator, "!=": TokenOperator,
			"<": TokenOperator, ">": TokenOperator, "<=": TokenOperator,
			">=": TokenOperator, "||": TokenOperator, "&&": TokenOperator,
			"+": TokenOperator, "-": TokenOperator, "*": TokenOperator,
			"/": TokenOperator, "%": TokenOperator, "&": TokenOperator,
			"|": TokenOperator, "^": TokenOperator, "~": TokenOperator,
		},
		sqlFunctions: map[string]TokenType{
			"concat":       TokenFunction, "concat_ws": TokenFunction,
			"substring":    TokenFunction, "substr": TokenFunction,
			"mid":          TokenFunction, "left": TokenFunction,
			"right":        TokenFunction, "char": TokenFunction,
			"chr":          TokenFunction, "ascii": TokenFunction,
			"ord":          TokenFunction, "hex": TokenFunction,
			"unhex":        TokenFunction, "conv": TokenFunction,
			"cast":         TokenFunction, "convert": TokenFunction,
			"coalesce":     TokenFunction, "ifnull": TokenFunction,
			"nullif":       TokenFunction, "if": TokenFunction,
			"sleep":        TokenFunction, "benchmark": TokenFunction,
			"pg_sleep":     TokenFunction, "waitfor": TokenFunction,
			"load_file":    TokenFunction, "into":  TokenFunction,
			"outfile":      TokenFunction, "dumpfile": TokenFunction,
			"version":      TokenFunction, "database": TokenFunction,
			"user":         TokenFunction, "current_user": TokenFunction,
			"system_user":  TokenFunction, "session_user": TokenFunction,
			"extractvalue": TokenFunction, "updatexml": TokenFunction,
			"xmltype":      TokenFunction, "exp": TokenFunction,
			"floor":        TokenFunction, "rand": TokenFunction,
			"count":        TokenFunction, "sum": TokenFunction,
			"avg":          TokenFunction, "min": TokenFunction,
			"max":          TokenFunction, "group_concat": TokenFunction,
			"string_agg":   TokenFunction, "listagg": TokenFunction,
		},
	}
}

// Tokenize performs lexical analysis on the input string
// TODO: Rust version should return zero-copy slices into original input
func (t *SQLTokenizer) Tokenize(input string) []Token {
	tokens := make([]Token, 0, 32)
	normalized := strings.ToLower(input)
	runes := []rune(normalized)
	length := len(runes)
	pos := 0

	for pos < length {
		// Skip whitespace
		if unicode.IsSpace(runes[pos]) {
			start := pos
			for pos < length && unicode.IsSpace(runes[pos]) {
				pos++
			}
			tokens = append(tokens, Token{
				Type:  TokenWhitespace,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}

		// Check for comments
		if pos+1 < length {
			twoChar := string(runes[pos : pos+2])
			if twoChar == "--" || twoChar == "/*" || twoChar == "#-" {
				start := pos
				if twoChar == "/*" {
					// Multi-line comment
					pos += 2
					for pos+1 < length && string(runes[pos:pos+2]) != "*/" {
						pos++
					}
					if pos+1 < length {
						pos += 2
					}
				} else {
					// Single-line comment
					for pos < length && runes[pos] != '\n' {
						pos++
					}
				}
				tokens = append(tokens, Token{
					Type:  TokenComment,
					Value: string(runes[start:pos]),
					Start: start,
					End:   pos,
					Risk:  5, // Comments in input are suspicious
				})
				continue
			}
		}

		// Check for strings
		if runes[pos] == '\'' || runes[pos] == '"' || runes[pos] == '`' {
			quote := runes[pos]
			start := pos
			pos++
			for pos < length && runes[pos] != quote {
				if runes[pos] == '\\' && pos+1 < length {
					pos += 2
				} else {
					pos++
				}
			}
			if pos < length {
				pos++
			}
			tokens = append(tokens, Token{
				Type:  TokenString,
				Value: string(runes[start:pos]),
				Start: start,
				End:   pos,
			})
			continue
		}

		// Check for numbers (including hex)
		if unicode.IsDigit(runes[pos]) || (runes[pos] == '0' && pos+1 < length && (runes[pos+1] == 'x' || runes[pos+1] == 'X')) {
			start := pos
			if runes[pos] == '0' && pos+1 < length && (runes[pos+1] == 'x' || runes[pos+1] == 'X') {
				// Hex number
				pos += 2
				for pos < length && (unicode.IsDigit(runes[pos]) || (runes[pos] >= 'a' && runes[pos] <= 'f') || (runes[pos] >= 'A' && runes[pos] <= 'F')) {
					pos++
				}
				tokens = append(tokens, Token{
					Type:  TokenNumber,
					Value: string(runes[start:pos]),
					Start: start,
					End:   pos,
					Risk:  3, // Hex numbers can be used for obfuscation
				})
			} else {
				// Decimal number
				for pos < length && (unicode.IsDigit(runes[pos]) || runes[pos] == '.') {
					pos++
				}
				tokens = append(tokens, Token{
					Type:  TokenNumber,
					Value: string(runes[start:pos]),
					Start: start,
					End:   pos,
				})
			}
			continue
		}

		// Check for identifiers and keywords
		if unicode.IsLetter(runes[pos]) || runes[pos] == '_' {
			start := pos
			for pos < length && (unicode.IsLetter(runes[pos]) || unicode.IsDigit(runes[pos]) || runes[pos] == '_') {
				pos++
			}
			value := string(runes[start:pos])

			tokenType := TokenIdentifier
			risk := 0
			if _, ok := t.sqlKeywords[value]; ok {
				tokenType = TokenKeyword
				risk = 8
			} else if _, ok := t.sqlFunctions[value]; ok {
				tokenType = TokenFunction
				risk = 10
			}

			tokens = append(tokens, Token{
				Type:  tokenType,
				Value: value,
				Start: start,
				End:   pos,
				Risk:  risk,
			})
			continue
		}

		// Check for operators
		if pos+1 < length {
			twoChar := string(runes[pos : pos+2])
			if _, ok := t.sqlOperators[twoChar]; ok {
				tokens = append(tokens, Token{
					Type:  TokenOperator,
					Value: twoChar,
					Start: pos,
					End:   pos + 2,
					Risk:  2,
				})
				pos += 2
				continue
			}
		}
		oneChar := string(runes[pos])
		if _, ok := t.sqlOperators[oneChar]; ok {
			tokens = append(tokens, Token{
				Type:  TokenOperator,
				Value: oneChar,
				Start: pos,
				End:   pos + 1,
				Risk:  2,
			})
			pos++
			continue
		}

		// Special characters
		if runes[pos] == '(' || runes[pos] == ')' || runes[pos] == ',' || runes[pos] == ';' {
			risk := 0
			if runes[pos] == ';' {
				risk = 10 // Semicolon could indicate stacked queries
			}
			tokens = append(tokens, Token{
				Type:  TokenSpecial,
				Value: string(runes[pos]),
				Start: pos,
				End:   pos + 1,
				Risk:  risk,
			})
			pos++
			continue
		}

		// Unknown character
		tokens = append(tokens, Token{
			Type:  TokenUnknown,
			Value: string(runes[pos]),
			Start: pos,
			End:   pos + 1,
		})
		pos++
	}

	return tokens
}

// NewSQLiDetector creates a new SQL injection detector
// TODO: Rust rebuild should use Aho-Corasick for multi-pattern matching
func NewSQLiDetector(strict bool) *SQLiDetector {
	patterns := []string{
		// UNION-based injection
		`(?i)union\s+(all\s+)?select`,
		`(?i)union\s*\/\*.*\*\/\s*select`,
		`(?i)union\s*%00\s*select`,
		`(?i)\/\*\*\/union\/\*\*\/select`,

		// Comment-based attacks
		`--\s*$`,
		`--\s+[^\n]+`,
		`#\s*$`,
		`\/\*[^*]*\*+([^/*][^*]*\*+)*\/`,
		`\/\*!.*\*\/`, // MySQL version-specific comments

		// SQL keywords in suspicious contexts
		`(?i)\b(select|insert|update|delete|drop|truncate|alter|create|grant|revoke)\b.*\b(from|into|table|database|schema)\b`,

		// Common injection patterns - single quotes
		`(?i)'\s*(or|and)\s+['0-9]`,
		`(?i)'\s*(or|and)\s+\w+\s*[=<>!]`,
		`(?i)'\s*(or|and)\s+\w+\s+(like|in|between)\s`,
		`(?i)'\s*;\s*--`,
		`(?i)'\s*;\s*(select|insert|update|delete|drop|exec)`,

		// Common injection patterns - double quotes
		`(?i)"\s*(or|and)\s+["0-9]`,
		`(?i)"\s*(or|and)\s+\w+\s*[=<>!]`,

		// Boolean-based blind
		`(?i)'\s*or\s+'[^']*'\s*[=<>]\s*'`,
		`(?i)'\s*and\s+'[^']*'\s*[=<>]\s*'`,
		`(?i)\d+\s*(or|and)\s+\d+\s*[=<>]\s*\d+`,
		`(?i)'\s*or\s+1\s*=\s*1`,
		`(?i)'\s*or\s+'1'\s*=\s*'1'`,
		`(?i)'\s*or\s+true`,
		`(?i)'\s*and\s+1\s*=\s*2`,
		`(?i)'\s*and\s+'1'\s*=\s*'2'`,
		`(?i)'\s*and\s+false`,

		// Time-based blind
		`(?i)(sleep|benchmark|waitfor\s+delay|pg_sleep)\s*\(`,
		`(?i);\s*waitfor\s+delay\s+`,
		`(?i);\s*select\s+sleep\s*\(`,

		// Error-based injection
		`(?i)(extractvalue|updatexml|xmltype)\s*\(`,
		`(?i)exp\s*\(\s*~`,
		`(?i)convert\s*\([^,]+,\s*(signed|unsigned)`,
		`(?i)cast\s*\([^)]+\s+as\s+`,

		// Stacked queries
		`;\s*(select|insert|update|delete|drop|exec|execute|create|alter|truncate)`,

		// LIKE-based injection
		`(?i)like\s+['"]%`,
		`(?i)like\s+0x`,

		// Information schema access
		`(?i)information_schema\.(tables|columns|schemata|processlist|user_privileges)`,
		`(?i)sys\.(databases|tables|columns|objects|procedures)`,
		`(?i)mysql\.(user|db|tables_priv|columns_priv)`,
		`(?i)pg_catalog\.`,
		`(?i)sysobjects`,
		`(?i)syscolumns`,

		// Common bypass attempts
		`(?i)(char|chr|concat|concat_ws)\s*\(`,
		`(?i)0x[0-9a-f]{4,}`, // Hex strings 4+ chars
		`(?i)unhex\s*\(`,
		`(?i)hex\s*\(`,
		`(?i)ascii\s*\(`,
		`(?i)ord\s*\(`,

		// File operations
		`(?i)(load_file|into\s+outfile|into\s+dumpfile)\s*\(?\s*['"]`,
		`(?i)load\s+data\s+infile`,

		// Encoded attacks - URL encoding
		`%27`,          // Single quote
		`%22`,          // Double quote
		`%2527`,        // Double encoded single quote
		`%2522`,        // Double encoded double quote
		`%00`,          // Null byte
		`%252e%252e`,   // Double encoded ..

		// Encoded attacks - HTML entities
		`&#x27;`,       // Single quote
		`&#39;`,        // Single quote
		`&#x22;`,       // Double quote
		`&#34;`,        // Double quote

		// Encoded attacks - Unicode
		`\\x27`,        // Hex escape single quote
		`\\x22`,        // Hex escape double quote
		`\u0027`,       // Unicode single quote
		`\u0022`,       // Unicode double quote

		// Database-specific attacks
		`(?i)@@version`,
		`(?i)@@hostname`,
		`(?i)@@datadir`,
		`(?i)user\s*\(\s*\)`,
		`(?i)current_user\s*\(\s*\)`,
		`(?i)system_user\s*\(\s*\)`,
		`(?i)session_user\s*\(\s*\)`,
		`(?i)database\s*\(\s*\)`,
		`(?i)schema\s*\(\s*\)`,
		`(?i)version\s*\(\s*\)`,

		// PostgreSQL specific
		`(?i)\$\$.*\$\$`,
		`(?i)pg_ls_dir\s*\(`,
		`(?i)pg_read_file\s*\(`,
		`(?i)pg_stat_file\s*\(`,

		// MSSQL specific
		`(?i)xp_cmdshell`,
		`(?i)xp_regread`,
		`(?i)xp_regwrite`,
		`(?i)xp_dirtree`,
		`(?i)xp_filelist`,
		`(?i)sp_executesql`,
		`(?i)sp_oacreate`,
		`(?i)openrowset`,
		`(?i)opendatasource`,

		// Oracle specific
		`(?i)utl_http`,
		`(?i)utl_file`,
		`(?i)dbms_pipe`,
		`(?i)dbms_java`,

		// NoSQL injection patterns
		`(?i)\$where\s*:`,
		`(?i)\$regex\s*:`,
		`(?i)\$gt\s*:`,
		`(?i)\$lt\s*:`,
		`(?i)\$ne\s*:`,
		`(?i)\$or\s*:\s*\[`,
		`(?i)\$and\s*:\s*\[`,
	}

	detector := &SQLiDetector{
		patterns:  make([]*regexp.Regexp, 0, len(patterns)),
		strict:    strict,
		tokenizer: NewSQLTokenizer(),
		keywords: map[string]int{
			"select": 10, "insert": 10, "update": 10, "delete": 10,
			"drop": 15, "truncate": 15, "alter": 12, "create": 10,
			"grant": 15, "revoke": 15, "union": 12, "from": 5,
			"where": 5, "exec": 15, "execute": 15, "into": 8,
			"outfile": 20, "dumpfile": 20, "load_file": 20,
		},
		operators: map[string]int{
			"=": 2, "<>": 3, "!=": 3, "<": 2, ">": 2,
			"<=": 2, ">=": 2, "||": 5, "&&": 5,
		},
		functions: map[string]int{
			"sleep": 20, "benchmark": 20, "pg_sleep": 20, "waitfor": 20,
			"extractvalue": 15, "updatexml": 15, "xmltype": 15,
			"concat": 8, "char": 10, "chr": 10, "hex": 10, "unhex": 12,
			"ascii": 8, "ord": 8, "conv": 10, "cast": 6, "convert": 6,
			"load_file": 20, "user": 8, "database": 8, "version": 8,
		},
	}

	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err == nil {
			detector.patterns = append(detector.patterns, re)
		}
	}

	return detector
}

// Detect checks if the input contains SQL injection
func (d *SQLiDetector) Detect(input string) bool {
	// Normalize input
	normalized := d.normalize(input)

	for _, pattern := range d.patterns {
		if pattern.MatchString(normalized) {
			return true
		}
	}

	// Additional tokenization-based detection
	if d.strict {
		tokens := d.tokenizer.Tokenize(normalized)
		risk := d.analyzeTokens(tokens)
		return risk >= 25
	}

	return false
}

// DetectWithDetails returns detailed detection results
// TODO: Rust version should include byte-level position information
func (d *SQLiDetector) DetectWithDetails(input string) *DetectionResult {
	result := &DetectionResult{
		Type:    "sqli",
		Input:   input,
		Matches: make([]string, 0),
	}

	normalized := d.normalize(input)

	// Pattern matching
	for _, pattern := range d.patterns {
		matches := pattern.FindAllString(normalized, -1)
		if len(matches) > 0 {
			result.Detected = true
			result.Matches = append(result.Matches, matches...)
		}
	}

	// Token analysis
	tokens := d.tokenizer.Tokenize(normalized)
	tokenRisk := d.analyzeTokens(tokens)
	dangerousTokens := d.getDangerousTokenPatterns(tokens)
	result.Matches = append(result.Matches, dangerousTokens...)

	// Structural analysis
	structureRisk := d.analyzeStructure(tokens)

	if tokenRisk >= 20 || structureRisk >= 15 {
		result.Detected = true
	}

	if result.Detected {
		result.Score = d.calculateScore(len(result.Matches), tokenRisk, structureRisk)
		result.Message = "SQL injection attempt detected"
	}

	return result
}

// analyzeTokens analyzes token sequence for SQL injection patterns
// TODO: Rust version should use state machine for O(n) analysis
func (d *SQLiDetector) analyzeTokens(tokens []Token) int {
	risk := 0

	// Look for dangerous sequences
	for i, token := range tokens {
		risk += token.Risk

		// Check for keyword followed by keyword (suspicious)
		if token.Type == TokenKeyword && i+1 < len(tokens) {
			next := tokens[i+1]
			if next.Type == TokenKeyword {
				risk += 5
			}
		}

		// Check for function calls
		if token.Type == TokenFunction {
			// Look for opening paren
			for j := i + 1; j < len(tokens) && j < i+3; j++ {
				if tokens[j].Type == TokenSpecial && tokens[j].Value == "(" {
					if w, ok := d.functions[token.Value]; ok {
						risk += w
					}
					break
				}
			}
		}
	}

	return risk
}

// analyzeStructure analyzes the structural patterns in token sequence
func (d *SQLiDetector) analyzeStructure(tokens []Token) int {
	risk := 0

	// Track state
	inString := false
	hasComment := false
	hasSemicolon := false
	keywordCount := 0
	functionCount := 0

	for _, token := range tokens {
		switch token.Type {
		case TokenString:
			inString = !inString
		case TokenComment:
			hasComment = true
			risk += 5
		case TokenSpecial:
			if token.Value == ";" {
				hasSemicolon = true
				risk += 8
			}
		case TokenKeyword:
			keywordCount++
		case TokenFunction:
			functionCount++
		}
	}

	// Multiple SQL keywords is suspicious
	if keywordCount >= 3 {
		risk += keywordCount * 3
	}

	// Multiple functions
	if functionCount >= 2 {
		risk += functionCount * 4
	}

	// Comment after semicolon (stacked queries with comment)
	if hasComment && hasSemicolon {
		risk += 15
	}

	return risk
}

// getDangerousTokenPatterns extracts dangerous token patterns as strings
func (d *SQLiDetector) getDangerousTokenPatterns(tokens []Token) []string {
	var patterns []string

	for i, token := range tokens {
		if token.Risk >= 8 {
			// Build context
			start := i - 1
			end := i + 2
			if start < 0 {
				start = 0
			}
			if end > len(tokens) {
				end = len(tokens)
			}

			var parts []string
			for j := start; j < end; j++ {
				parts = append(parts, tokens[j].Value)
			}
			patterns = append(patterns, strings.Join(parts, ""))
		}
	}

	return patterns
}

// normalize normalizes the input for detection
// TODO: Rust version should work on bytes directly with SIMD
func (d *SQLiDetector) normalize(input string) string {
	result := input

	// URL decode common patterns - multiple passes for double encoding
	for i := 0; i < 3; i++ {
		prev := result
		result = strings.ReplaceAll(result, "%20", " ")
		result = strings.ReplaceAll(result, "%27", "'")
		result = strings.ReplaceAll(result, "%22", "\"")
		result = strings.ReplaceAll(result, "%3D", "=")
		result = strings.ReplaceAll(result, "%3C", "<")
		result = strings.ReplaceAll(result, "%3E", ">")
		result = strings.ReplaceAll(result, "%2F", "/")
		result = strings.ReplaceAll(result, "%5C", "\\")
		result = strings.ReplaceAll(result, "%2B", "+")
		result = strings.ReplaceAll(result, "%25", "%")
		result = strings.ReplaceAll(result, "%00", "\x00")
		result = strings.ReplaceAll(result, "%0A", "\n")
		result = strings.ReplaceAll(result, "%0D", "\r")
		result = strings.ReplaceAll(result, "%09", "\t")
		if result == prev {
			break
		}
	}

	// Handle plus as space
	result = strings.ReplaceAll(result, "+", " ")

	// Normalize various comment styles
	result = regexp.MustCompile(`/\*[^*]*\*+([^/*][^*]*\*+)*/`).ReplaceAllString(result, " ")
	result = regexp.MustCompile(`--[^\n]*`).ReplaceAllString(result, " ")
	result = regexp.MustCompile(`#[^\n]*`).ReplaceAllString(result, " ")

	// Collapse multiple spaces
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")

	// Remove null bytes (used for WAF bypass)
	result = strings.ReplaceAll(result, "\x00", "")

	return strings.TrimSpace(result)
}

// calculateScore calculates the anomaly score based on all factors
func (d *SQLiDetector) calculateScore(matchCount, tokenRisk, structureRisk int) int {
	baseScore := 10

	// Add match contribution
	if matchCount > 0 {
		baseScore += matchCount * 5
	}

	// Add token risk contribution (scaled down)
	baseScore += tokenRisk / 3

	// Add structure risk contribution (scaled down)
	baseScore += structureRisk / 2

	// Cap at 100
	if baseScore > 100 {
		return 100
	}

	return baseScore
}

// GetPatternCount returns the number of patterns
func (d *SQLiDetector) GetPatternCount() int {
	return len(d.patterns)
}

// DetectionResult holds the result of a detection
type DetectionResult struct {
	Type     string
	Detected bool
	Score    int
	Input    string
	Matches  []string
	Message  string
}
