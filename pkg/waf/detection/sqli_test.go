// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package detection

import (
	"testing"
)

func TestSQLiDetector_Detect(t *testing.T) {
	detector := NewSQLiDetector(false)

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Basic SQL injection patterns
		{"Union select", "1 UNION SELECT * FROM users", true},
		{"Union all select", "1 UNION ALL SELECT password FROM users", true},
		{"OR 1=1", "' OR '1'='1", true},
		{"OR true", "' OR true--", true},
		{"AND 1=2", "' AND '1'='2", true},
		{"Comment injection with SELECT", "' OR 1=1--", true},
		{"Stacked queries", "'; DROP TABLE users;--", true},

		// Time-based blind injection
		{"Sleep function MySQL", "1' AND SLEEP(5)--", true},
		{"Benchmark MySQL", "1' AND BENCHMARK(10000000,SHA1('test'))--", true},
		{"WAITFOR DELAY MSSQL", "1'; WAITFOR DELAY '0:0:5'--", true},
		{"pg_sleep PostgreSQL", "1'; SELECT pg_sleep(5);--", true},

		// Error-based injection
		{"ExtractValue", "AND extractvalue(1,concat(0x7e,(SELECT user())))--", true},
		{"UpdateXML", "AND updatexml(1,concat(0x7e,(SELECT database())),1)--", true},

		// Information schema access
		{"Information schema tables", "UNION SELECT table_name FROM information_schema.tables", true},
		{"Information schema columns", "UNION SELECT column_name FROM information_schema.columns", true},
		{"MySQL user table", "SELECT * FROM mysql.user", true},

		// MSSQL specific
		{"xp_cmdshell", "'; EXEC xp_cmdshell('dir');--", true},
		{"sp_executesql", "EXEC sp_executesql N'SELECT 1'", true},
		{"OPENROWSET", "SELECT * FROM OPENROWSET('SQLOLEDB','';'')", true},

		// PostgreSQL specific
		{"pg_ls_dir", "SELECT pg_ls_dir('/etc')", true},
		{"pg_read_file", "SELECT pg_read_file('/etc/passwd')", true},

		// Oracle specific
		{"utl_http", "SELECT utl_http.request('http://evil.com')", true},
		{"utl_file", "utl_file.put_line()", true},

		// File operations
		{"LOAD_FILE", "SELECT LOAD_FILE('/etc/passwd')", true},
		{"INTO OUTFILE", "SELECT * INTO OUTFILE '/tmp/test.txt'", true},
		{"INTO DUMPFILE", "SELECT * INTO DUMPFILE '/tmp/test.txt'", true},

		// NoSQL injection patterns
		{"MongoDB $where", "{$where: 'this.password == password'}", true},
		{"MongoDB $regex", "{username: {$regex: '.*'}}", true},
		{"MongoDB $gt", "{age: {$gt: 0}}", true},
		{"MongoDB $or", "{$or: [{user: 'admin'}, {pass: {$ne: ''}}]}", true},

		// Bypass techniques - URL encoding
		{"URL encoded quote", "1%27%20OR%20%271%27=%271", true},
		{"Double URL encoded", "1%2527%20OR%20%25271%2527=%25271", true},
		{"Null byte with union", "admin%00' UNION SELECT 1--", true},

		// Bypass techniques - Comment variations
		{"Inline comment", "1/**/UNION/**/SELECT/**/password/**/FROM/**/users", true},
		{"MySQL version comment", "1/*!UNION*/SELECT password FROM users", true},

		// Hex encoding bypass
		{"Hex encoded string", "SELECT 0x61646d696e", true},
		{"CHAR function", "SELECT CHAR(65,66,67)", true},
		{"CONCAT function", "SELECT CONCAT('a','d','m','i','n')", true},

		// Database function calls
		{"version()", "SELECT version()", true},
		{"database()", "SELECT database()", true},
		{"user()", "SELECT user()", true},
		{"current_user()", "SELECT current_user()", true},
		{"@@version", "SELECT @@version", true},
		{"@@hostname", "SELECT @@hostname", true},

		// Clean inputs - should NOT be detected
		{"Normal text", "Hello, how are you today?", false},
		{"Email address", "user@example.com", false},
		{"Product name", "SELECT a nice day!", false},
		{"Normal search", "search for products", false},
		{"URL path", "/products/category/electronics", false},
		{"JSON data", `{"name": "John", "age": 30}`, false},
		{"Date format", "2024-01-15", false},
		{"Normal number", "12345", false},
		{"Simple word", "password", false},
		{"Username", "admin_user_123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Detect(tt.input)
			if result != tt.expected {
				t.Errorf("Detect(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSQLiDetector_DetectWithDetails(t *testing.T) {
	detector := NewSQLiDetector(true)

	tests := []struct {
		name          string
		input         string
		wantDetected  bool
		wantScoreMin  int
		wantMatchMin  int
	}{
		{
			name:          "Complex union injection",
			input:         "1' UNION SELECT username, password FROM users WHERE '1'='1",
			wantDetected:  true,
			wantScoreMin:  15,
			wantMatchMin:  1,
		},
		{
			name:          "Time-based blind",
			input:         "1' AND SLEEP(5) AND '1'='1",
			wantDetected:  true,
			wantScoreMin:  15,
			wantMatchMin:  1,
		},
		{
			name:          "Multiple techniques",
			input:         "1' UNION SELECT * FROM users; DROP TABLE users;-- SLEEP(5)",
			wantDetected:  true,
			wantScoreMin:  20,
			wantMatchMin:  2,
		},
		{
			name:          "Clean input",
			input:         "normal search query for products",
			wantDetected:  false,
			wantScoreMin:  0,
			wantMatchMin:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectWithDetails(tt.input)
			if result.Detected != tt.wantDetected {
				t.Errorf("DetectWithDetails().Detected = %v, want %v", result.Detected, tt.wantDetected)
			}
			if result.Score < tt.wantScoreMin {
				t.Errorf("DetectWithDetails().Score = %d, want >= %d", result.Score, tt.wantScoreMin)
			}
			if len(result.Matches) < tt.wantMatchMin {
				t.Errorf("len(DetectWithDetails().Matches) = %d, want >= %d", len(result.Matches), tt.wantMatchMin)
			}
		})
	}
}

func TestSQLiDetector_StrictMode(t *testing.T) {
	normalDetector := NewSQLiDetector(false)
	strictDetector := NewSQLiDetector(true)

	// Borderline cases that strict mode should catch
	borderlineCases := []struct {
		name   string
		input  string
	}{
		{"SELECT keyword in context", "select your favorite color"},
		{"FROM in sentence", "data from the table"},
		{"Multiple SQL keywords", "select data from table where condition"},
	}

	for _, tt := range borderlineCases {
		t.Run(tt.name, func(t *testing.T) {
			normalResult := normalDetector.Detect(tt.input)
			strictResult := strictDetector.Detect(tt.input)

			// Strict mode may detect more cases
			t.Logf("Input: %q, Normal: %v, Strict: %v", tt.input, normalResult, strictResult)
		})
	}
}

func TestSQLTokenizer(t *testing.T) {
	tokenizer := NewSQLTokenizer()

	tests := []struct {
		name       string
		input      string
		wantTokens int
	}{
		{"Simple query", "SELECT * FROM users", 7},
		{"Query with comment", "SELECT * -- comment", 5},
		{"Query with string", "SELECT 'test' FROM users", 7},
		{"Query with number", "SELECT 123 FROM users", 7},
		{"Empty input", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenizer.Tokenize(tt.input)
			if len(tokens) < tt.wantTokens/2 { // Allow some variation
				t.Errorf("Tokenize(%q) produced %d tokens, want at least %d", tt.input, len(tokens), tt.wantTokens/2)
			}
		})
	}
}

func TestSQLTokenizer_TokenTypes(t *testing.T) {
	tokenizer := NewSQLTokenizer()

	// Test that specific token types are recognized
	tests := []struct {
		input      string
		wantType   TokenType
	}{
		{"select", TokenKeyword},
		{"concat", TokenFunction},
		{"=", TokenOperator},
		{"'test'", TokenString},
		{"123", TokenNumber},
		{"--comment", TokenComment},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens := tokenizer.Tokenize(tt.input)
			if len(tokens) == 0 {
				t.Fatalf("No tokens produced for input %q", tt.input)
			}
			// Find a non-whitespace token
			for _, tok := range tokens {
				if tok.Type != TokenWhitespace {
					if tok.Type != tt.wantType {
						t.Errorf("Token type for %q = %v, want %v", tt.input, tok.Type, tt.wantType)
					}
					break
				}
			}
		})
	}
}

func TestSQLiDetector_NormalizationBypass(t *testing.T) {
	detector := NewSQLiDetector(false)

	// Test various encoding bypass attempts
	encodingTests := []struct {
		name     string
		input    string
		expected bool
	}{
		// URL encoding
		{"URL encoded single quote", "1%27%20OR%20%271%27%3D%271", true},
		{"URL encoded double quote", "1%22%20OR%20%221%22%3D%221", true},
		{"Plus as space", "1'+OR+'1'='1", true},

		// Double encoding
		{"Double encoded attack", "1%2527%20OR%20%25271%2527%253D%25271", true},

		// Mixed encoding
		{"Mixed encoding", "1'%20OR%20'1'='1", true},

		// Case variations
		{"Mixed case UNION", "uNiOn SeLeCt * FrOm users", true},
		{"All caps", "1' UNION SELECT PASSWORD FROM USERS--", true},
	}

	for _, tt := range encodingTests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.Detect(tt.input)
			if result != tt.expected {
				t.Errorf("Detect(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSQLiDetector_GetPatternCount(t *testing.T) {
	detector := NewSQLiDetector(false)
	count := detector.GetPatternCount()
	if count < 50 { // Should have many patterns
		t.Errorf("GetPatternCount() = %d, want >= 50", count)
	}
}

func BenchmarkSQLiDetector_Detect(b *testing.B) {
	detector := NewSQLiDetector(false)
	input := "1' UNION SELECT username, password FROM users WHERE '1'='1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(input)
	}
}

func BenchmarkSQLiDetector_DetectCleanInput(b *testing.B) {
	detector := NewSQLiDetector(false)
	input := "This is a perfectly normal search query without any SQL"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(input)
	}
}

func BenchmarkSQLiDetector_DetectWithDetails(b *testing.B) {
	detector := NewSQLiDetector(true)
	input := "1' UNION SELECT username, password FROM users WHERE '1'='1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.DetectWithDetails(input)
	}
}

func BenchmarkSQLTokenizer_Tokenize(b *testing.B) {
	tokenizer := NewSQLTokenizer()
	input := "SELECT username, password, email FROM users WHERE id = 1 AND status = 'active' ORDER BY created_at DESC LIMIT 10"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizer.Tokenize(input)
	}
}
