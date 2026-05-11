// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package sources provides configuration source implementations.
package sources

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// FileSource loads configuration from a file (TOML, YAML, JSON).
type FileSource struct {
	path   string
	format string
}

// NewFileSource creates a new file source.
func NewFileSource(path string) (*FileSource, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	format := detectFormat(path)
	if format == "" {
		return nil, fmt.Errorf("unsupported file format: %s", path)
	}

	return &FileSource{
		path:   path,
		format: format,
	}, nil
}

// detectFormat detects the configuration file format from extension.
func detectFormat(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".toml"):
		return "toml"
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		return "yaml"
	case strings.HasSuffix(lower, ".json"):
		return "json"
	default:
		return ""
	}
}

// Load loads configuration from the file.
func (f *FileSource) Load() (map[string]any, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	switch f.format {
	case "json":
		return parseJSON(data)
	case "toml":
		return parseTOML(data)
	case "yaml":
		return parseYAML(data)
	default:
		return nil, fmt.Errorf("unsupported format: %s", f.format)
	}
}

// Name returns the source name.
func (f *FileSource) Name() string {
	return "file:" + f.path
}

// Priority returns the source priority (lower = higher precedence).
func (f *FileSource) Priority() int {
	return 100 // File has lowest priority
}

// Path returns the file path.
func (f *FileSource) Path() string {
	return f.path
}

// parseJSON parses JSON configuration.
func parseJSON(data []byte) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("json parse: %w", err)
	}
	return result, nil
}

// TOMLParser is a hand-rolled TOML parser supporting basic TOML features.
type TOMLParser struct {
	input   string
	pos     int
	line    int
	col     int
	current rune
}

// parseTOML parses TOML configuration.
func parseTOML(data []byte) (map[string]any, error) {
	parser := &TOMLParser{
		input: string(data),
		pos:   0,
		line:  1,
		col:   1,
	}
	return parser.Parse()
}

// Parse parses the TOML input.
func (p *TOMLParser) Parse() (map[string]any, error) {
	result := make(map[string]any)
	currentSection := result
	var currentSectionPath []string
	var arrayOfTables string
	arrayIndex := -1

	for p.pos < len(p.input) {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			break
		}

		ch := p.peek()

		switch ch {
		case '[':
			// Section or array of tables
			p.advance()
			if p.peek() == '[' {
				// Array of tables [[section]]
				p.advance()
				sectionName := p.readUntil(']')
				p.advance() // skip first ]
				if p.peek() == ']' {
					p.advance() // skip second ]
				}

				// Handle array of tables
				if sectionName != arrayOfTables {
					arrayOfTables = sectionName
					arrayIndex = 0
				} else {
					arrayIndex++
				}

				parts := strings.Split(sectionName, ".")
				currentSectionPath = parts

				// Navigate to parent and create array
				parent := result
				for i := 0; i < len(parts)-1; i++ {
					if _, ok := parent[parts[i]]; !ok {
						parent[parts[i]] = make(map[string]any)
					}
					parent = parent[parts[i]].(map[string]any)
				}

				lastKey := parts[len(parts)-1]
				if _, ok := parent[lastKey]; !ok {
					parent[lastKey] = make([]any, 0)
				}

				arr := parent[lastKey].([]any)
				newSection := make(map[string]any)
				arr = append(arr, newSection)
				parent[lastKey] = arr
				currentSection = newSection
			} else {
				// Regular section [section]
				sectionName := p.readUntil(']')
				p.advance() // skip ]

				parts := strings.Split(sectionName, ".")
				currentSectionPath = parts
				arrayOfTables = ""
				arrayIndex = -1

				// Navigate to section, creating intermediate maps
				currentSection = result
				for _, part := range parts {
					if _, ok := currentSection[part]; !ok {
						currentSection[part] = make(map[string]any)
					}
					currentSection = currentSection[part].(map[string]any)
				}
			}

		case '\n', '\r':
			p.advance()

		default:
			// Key-value pair
			key, value, err := p.parseKeyValue()
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", p.line, err)
			}
			if key != "" {
				// Handle dotted keys
				if strings.Contains(key, ".") {
					parts := strings.Split(key, ".")
					target := currentSection
					for i := 0; i < len(parts)-1; i++ {
						if _, ok := target[parts[i]]; !ok {
							target[parts[i]] = make(map[string]any)
						}
						target = target[parts[i]].(map[string]any)
					}
					target[parts[len(parts)-1]] = value
				} else {
					currentSection[key] = value
				}
			}
		}
	}

	_ = currentSectionPath // Used for context
	return result, nil
}

// peek returns the current character without advancing.
func (p *TOMLParser) peek() rune {
	if p.pos >= len(p.input) {
		return 0
	}
	return rune(p.input[p.pos])
}

// advance moves to the next character.
func (p *TOMLParser) advance() rune {
	if p.pos >= len(p.input) {
		return 0
	}
	ch := rune(p.input[p.pos])
	p.pos++
	if ch == '\n' {
		p.line++
		p.col = 1
	} else {
		p.col++
	}
	p.current = ch
	return ch
}

// skipWhitespaceAndComments skips whitespace and comments.
func (p *TOMLParser) skipWhitespaceAndComments() {
	for p.pos < len(p.input) {
		ch := p.peek()
		switch ch {
		case ' ', '\t', '\r':
			p.advance()
		case '#':
			// Skip comment until end of line
			for p.pos < len(p.input) && p.peek() != '\n' {
				p.advance()
			}
		case '\n':
			p.advance()
		default:
			return
		}
	}
}

// readUntil reads until the given character.
func (p *TOMLParser) readUntil(stop rune) string {
	var sb strings.Builder
	for p.pos < len(p.input) && p.peek() != stop {
		sb.WriteRune(p.advance())
	}
	return strings.TrimSpace(sb.String())
}

// parseKeyValue parses a key-value pair.
func (p *TOMLParser) parseKeyValue() (string, any, error) {
	// Skip leading whitespace
	for p.pos < len(p.input) && (p.peek() == ' ' || p.peek() == '\t') {
		p.advance()
	}

	if p.pos >= len(p.input) || p.peek() == '\n' || p.peek() == '#' {
		return "", nil, nil
	}

	// Read key
	key := p.parseKey()
	if key == "" {
		return "", nil, nil
	}

	// Skip whitespace and =
	for p.pos < len(p.input) && (p.peek() == ' ' || p.peek() == '\t') {
		p.advance()
	}

	if p.peek() != '=' {
		return "", nil, fmt.Errorf("expected '=' after key '%s', got '%c'", key, p.peek())
	}
	p.advance() // skip =

	// Skip whitespace after =
	for p.pos < len(p.input) && (p.peek() == ' ' || p.peek() == '\t') {
		p.advance()
	}

	// Parse value
	value, err := p.parseValue()
	if err != nil {
		return "", nil, err
	}

	// Skip to end of line
	for p.pos < len(p.input) && p.peek() != '\n' && p.peek() != '#' {
		if p.peek() == ' ' || p.peek() == '\t' || p.peek() == '\r' {
			p.advance()
		} else {
			break
		}
	}

	return key, value, nil
}

// parseKey parses a TOML key.
func (p *TOMLParser) parseKey() string {
	var sb strings.Builder

	// Handle quoted keys
	if p.peek() == '"' {
		p.advance()
		for p.pos < len(p.input) && p.peek() != '"' {
			if p.peek() == '\\' {
				p.advance()
				sb.WriteRune(p.advance())
			} else {
				sb.WriteRune(p.advance())
			}
		}
		p.advance() // skip closing quote
		return sb.String()
	}

	// Bare key
	for p.pos < len(p.input) {
		ch := p.peek()
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '-' || ch == '.' {
			sb.WriteRune(p.advance())
		} else {
			break
		}
	}

	return sb.String()
}

// parseValue parses a TOML value.
func (p *TOMLParser) parseValue() (any, error) {
	ch := p.peek()

	switch {
	case ch == '"':
		return p.parseString()
	case ch == '\'':
		return p.parseLiteralString()
	case ch == '[':
		return p.parseArray()
	case ch == '{':
		return p.parseInlineTable()
	case ch == 't' || ch == 'f':
		return p.parseBool()
	case ch == '-' || ch == '+' || unicode.IsDigit(ch):
		return p.parseNumber()
	default:
		return nil, fmt.Errorf("unexpected character: %c", ch)
	}
}

// parseString parses a basic TOML string.
func (p *TOMLParser) parseString() (string, error) {
	p.advance() // skip opening quote

	// Check for multiline string
	if p.peek() == '"' {
		p.advance()
		if p.peek() == '"' {
			p.advance()
			return p.parseMultilineString()
		}
		return "", nil // empty string
	}

	var sb strings.Builder
	for p.pos < len(p.input) && p.peek() != '"' {
		if p.peek() == '\\' {
			p.advance()
			escaped, err := p.parseEscapeSequence()
			if err != nil {
				return "", err
			}
			sb.WriteString(escaped)
		} else {
			sb.WriteRune(p.advance())
		}
	}
	p.advance() // skip closing quote
	return sb.String(), nil
}

// parseMultilineString parses a multiline TOML string.
func (p *TOMLParser) parseMultilineString() (string, error) {
	// Skip optional newline after opening quotes
	if p.peek() == '\n' {
		p.advance()
	}

	var sb strings.Builder
	for p.pos < len(p.input) {
		if p.peek() == '"' {
			p.advance()
			if p.peek() == '"' {
				p.advance()
				if p.peek() == '"' {
					p.advance()
					return sb.String(), nil
				}
				sb.WriteString("\"\"")
			} else {
				sb.WriteRune('"')
			}
		} else if p.peek() == '\\' {
			p.advance()
			if p.peek() == '\n' || p.peek() == '\r' {
				// Line ending backslash - skip whitespace
				for p.pos < len(p.input) && (p.peek() == '\n' || p.peek() == '\r' || p.peek() == ' ' || p.peek() == '\t') {
					p.advance()
				}
			} else {
				escaped, err := p.parseEscapeSequence()
				if err != nil {
					return "", err
				}
				sb.WriteString(escaped)
			}
		} else {
			sb.WriteRune(p.advance())
		}
	}
	return "", fmt.Errorf("unterminated multiline string")
}

// parseLiteralString parses a literal TOML string (single quotes).
func (p *TOMLParser) parseLiteralString() (string, error) {
	p.advance() // skip opening quote

	// Check for multiline literal string
	if p.peek() == '\'' {
		p.advance()
		if p.peek() == '\'' {
			p.advance()
			return p.parseMultilineLiteralString()
		}
		return "", nil // empty string
	}

	var sb strings.Builder
	for p.pos < len(p.input) && p.peek() != '\'' {
		sb.WriteRune(p.advance())
	}
	p.advance() // skip closing quote
	return sb.String(), nil
}

// parseMultilineLiteralString parses a multiline literal string.
func (p *TOMLParser) parseMultilineLiteralString() (string, error) {
	// Skip optional newline after opening quotes
	if p.peek() == '\n' {
		p.advance()
	}

	var sb strings.Builder
	for p.pos < len(p.input) {
		if p.peek() == '\'' {
			p.advance()
			if p.peek() == '\'' {
				p.advance()
				if p.peek() == '\'' {
					p.advance()
					return sb.String(), nil
				}
				sb.WriteString("''")
			} else {
				sb.WriteRune('\'')
			}
		} else {
			sb.WriteRune(p.advance())
		}
	}
	return "", fmt.Errorf("unterminated multiline literal string")
}

// parseEscapeSequence parses an escape sequence.
func (p *TOMLParser) parseEscapeSequence() (string, error) {
	ch := p.advance()
	switch ch {
	case 'b':
		return "\b", nil
	case 't':
		return "\t", nil
	case 'n':
		return "\n", nil
	case 'f':
		return "\f", nil
	case 'r':
		return "\r", nil
	case '"':
		return "\"", nil
	case '\\':
		return "\\", nil
	case 'u':
		return p.parseUnicodeEscape(4)
	case 'U':
		return p.parseUnicodeEscape(8)
	default:
		return "", fmt.Errorf("invalid escape sequence: \\%c", ch)
	}
}

// parseUnicodeEscape parses a Unicode escape sequence.
func (p *TOMLParser) parseUnicodeEscape(digits int) (string, error) {
	var sb strings.Builder
	for i := 0; i < digits; i++ {
		sb.WriteRune(p.advance())
	}
	code, err := strconv.ParseInt(sb.String(), 16, 32)
	if err != nil {
		return "", fmt.Errorf("invalid unicode escape: %s", sb.String())
	}
	return string(rune(code)), nil
}

// parseArray parses a TOML array.
func (p *TOMLParser) parseArray() ([]any, error) {
	p.advance() // skip [

	var result []any
	for p.pos < len(p.input) {
		p.skipWhitespaceAndNewlines()

		if p.peek() == ']' {
			p.advance()
			return result, nil
		}

		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		result = append(result, value)

		p.skipWhitespaceAndNewlines()

		if p.peek() == ',' {
			p.advance()
		} else if p.peek() != ']' {
			return nil, fmt.Errorf("expected ',' or ']' in array")
		}
	}
	return nil, fmt.Errorf("unterminated array")
}

// skipWhitespaceAndNewlines skips whitespace including newlines.
func (p *TOMLParser) skipWhitespaceAndNewlines() {
	for p.pos < len(p.input) {
		ch := p.peek()
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '#' {
			if ch == '#' {
				for p.pos < len(p.input) && p.peek() != '\n' {
					p.advance()
				}
			} else {
				p.advance()
			}
		} else {
			break
		}
	}
}

// parseInlineTable parses an inline TOML table.
func (p *TOMLParser) parseInlineTable() (map[string]any, error) {
	p.advance() // skip {

	result := make(map[string]any)
	for p.pos < len(p.input) {
		// Skip whitespace
		for p.pos < len(p.input) && (p.peek() == ' ' || p.peek() == '\t') {
			p.advance()
		}

		if p.peek() == '}' {
			p.advance()
			return result, nil
		}

		key, value, err := p.parseKeyValue()
		if err != nil {
			return nil, err
		}
		result[key] = value

		// Skip whitespace
		for p.pos < len(p.input) && (p.peek() == ' ' || p.peek() == '\t') {
			p.advance()
		}

		if p.peek() == ',' {
			p.advance()
		} else if p.peek() != '}' {
			return nil, fmt.Errorf("expected ',' or '}' in inline table")
		}
	}
	return nil, fmt.Errorf("unterminated inline table")
}

// parseBool parses a TOML boolean.
func (p *TOMLParser) parseBool() (bool, error) {
	var sb strings.Builder
	for p.pos < len(p.input) && unicode.IsLetter(p.peek()) {
		sb.WriteRune(p.advance())
	}

	switch sb.String() {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean: %s", sb.String())
	}
}

// parseNumber parses a TOML number.
func (p *TOMLParser) parseNumber() (any, error) {
	var sb strings.Builder
	hasDecimal := false
	hasExponent := false

	// Handle sign
	if p.peek() == '+' || p.peek() == '-' {
		sb.WriteRune(p.advance())
	}

	// Check for special values
	if p.peek() == 'i' || p.peek() == 'n' {
		return p.parseSpecialFloat(sb.String())
	}

	// Check for hex, octal, binary
	if p.peek() == '0' {
		sb.WriteRune(p.advance())
		switch p.peek() {
		case 'x', 'X':
			sb.WriteRune(p.advance())
			return p.parseHexNumber(sb.String())
		case 'o', 'O':
			sb.WriteRune(p.advance())
			return p.parseOctalNumber(sb.String())
		case 'b', 'B':
			sb.WriteRune(p.advance())
			return p.parseBinaryNumber(sb.String())
		}
	}

	// Parse regular number
	for p.pos < len(p.input) {
		ch := p.peek()
		if unicode.IsDigit(ch) {
			sb.WriteRune(p.advance())
		} else if ch == '_' {
			p.advance() // skip underscores
		} else if ch == '.' && !hasDecimal && !hasExponent {
			hasDecimal = true
			sb.WriteRune(p.advance())
		} else if (ch == 'e' || ch == 'E') && !hasExponent {
			hasExponent = true
			sb.WriteRune(p.advance())
			if p.peek() == '+' || p.peek() == '-' {
				sb.WriteRune(p.advance())
			}
		} else {
			break
		}
	}

	str := sb.String()
	if hasDecimal || hasExponent {
		f, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float: %s", str)
		}
		return f, nil
	}

	i, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid integer: %s", str)
	}
	return int(i), nil
}

// parseSpecialFloat parses inf and nan.
func (p *TOMLParser) parseSpecialFloat(prefix string) (float64, error) {
	var sb strings.Builder
	for p.pos < len(p.input) && unicode.IsLetter(p.peek()) {
		sb.WriteRune(p.advance())
	}

	switch prefix + sb.String() {
	case "inf", "+inf":
		return math.Inf(1), nil // Positive infinity
	case "-inf":
		return math.Inf(-1), nil // Negative infinity
	case "nan", "+nan", "-nan":
		return math.NaN(), nil
	default:
		return 0, fmt.Errorf("invalid special float: %s%s", prefix, sb.String())
	}
}

// parseHexNumber parses a hexadecimal number.
func (p *TOMLParser) parseHexNumber(prefix string) (int, error) {
	var sb strings.Builder
	for p.pos < len(p.input) {
		ch := p.peek()
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			sb.WriteRune(p.advance())
		} else if ch == '_' {
			p.advance()
		} else {
			break
		}
	}
	i, err := strconv.ParseInt(sb.String(), 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid hex number: 0x%s", sb.String())
	}
	return int(i), nil
}

// parseOctalNumber parses an octal number.
func (p *TOMLParser) parseOctalNumber(prefix string) (int, error) {
	var sb strings.Builder
	for p.pos < len(p.input) {
		ch := p.peek()
		if ch >= '0' && ch <= '7' {
			sb.WriteRune(p.advance())
		} else if ch == '_' {
			p.advance()
		} else {
			break
		}
	}
	i, err := strconv.ParseInt(sb.String(), 8, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid octal number: 0o%s", sb.String())
	}
	return int(i), nil
}

// parseBinaryNumber parses a binary number.
func (p *TOMLParser) parseBinaryNumber(prefix string) (int, error) {
	var sb strings.Builder
	binLoop:
	for p.pos < len(p.input) {
		ch := p.peek()
		switch ch {
		case '0', '1':
			sb.WriteRune(p.advance())
		case '_':
			p.advance()
		default:
			break binLoop
		}
	}
	i, err := strconv.ParseInt(sb.String(), 2, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid binary number: 0b%s", sb.String())
	}
	return int(i), nil
}

// YAMLParser is a hand-rolled YAML parser supporting basic YAML features.
type YAMLParser struct {
	lines   []string
	pos     int
	lineNum int
}

// parseYAML parses YAML configuration.
func parseYAML(data []byte) (map[string]any, error) {
	parser := &YAMLParser{
		lines:   strings.Split(string(data), "\n"),
		pos:     0,
		lineNum: 1,
	}
	return parser.Parse()
}

// Parse parses the YAML input.
func (p *YAMLParser) Parse() (map[string]any, error) {
	return p.parseMap(0)
}

// parseMap parses a YAML map at the given indentation level.
func (p *YAMLParser) parseMap(baseIndent int) (map[string]any, error) {
	result := make(map[string]any)

	for p.pos < len(p.lines) {
		line := p.lines[p.pos]

		// Skip empty lines and comments
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			p.pos++
			p.lineNum++
			continue
		}

		// Calculate indentation
		indent := p.getIndent(line)
		if indent < baseIndent {
			break // End of this map
		}
		if indent > baseIndent {
			p.pos++
			p.lineNum++
			continue // Skip over-indented lines (handled by parent)
		}

		// Parse key-value
		key, value, err := p.parseKeyValue(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", p.lineNum, err)
		}

		if key == "" {
			p.pos++
			p.lineNum++
			continue
		}

		// Check if value is a nested map or array
		if value == nil && p.pos+1 < len(p.lines) {
			nextLine := p.lines[p.pos+1]
			nextTrimmed := strings.TrimSpace(nextLine)
			nextIndent := p.getIndent(nextLine)

			if nextIndent > indent && nextTrimmed != "" && !strings.HasPrefix(nextTrimmed, "#") {
				if strings.HasPrefix(nextTrimmed, "- ") || nextTrimmed == "-" {
					// Array
					p.pos++
					p.lineNum++
					arr, err := p.parseArray(nextIndent)
					if err != nil {
						return nil, err
					}
					result[key] = arr
					continue
				} else {
					// Nested map
					p.pos++
					p.lineNum++
					nested, err := p.parseMap(nextIndent)
					if err != nil {
						return nil, err
					}
					result[key] = nested
					continue
				}
			}
		}

		result[key] = value
		p.pos++
		p.lineNum++
	}

	return result, nil
}

// parseArray parses a YAML array at the given indentation level.
func (p *YAMLParser) parseArray(baseIndent int) ([]any, error) {
	var result []any

	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			p.pos++
			p.lineNum++
			continue
		}

		indent := p.getIndent(line)
		if indent < baseIndent {
			break
		}

		if !strings.HasPrefix(trimmed, "-") {
			break
		}

		// Parse array element
		content := strings.TrimPrefix(trimmed, "-")
		content = strings.TrimSpace(content)

		if content == "" {
			// Complex array element (nested map)
			p.pos++
			p.lineNum++
			if p.pos < len(p.lines) {
				nextIndent := p.getIndent(p.lines[p.pos])
				if nextIndent > indent {
					nested, err := p.parseMap(nextIndent)
					if err != nil {
						return nil, err
					}
					result = append(result, nested)
					continue
				}
			}
			result = append(result, nil)
			continue
		}

		// Check if it's a key-value pair
		if strings.Contains(content, ":") {
			// Parse as inline map or single key-value
			if strings.HasPrefix(content, "{") {
				// Inline map (flow style)
				val := p.parseFlowValue(content)
				result = append(result, val)
			} else {
				// Key-value starts inline
				key, val, _ := p.parseInlineKeyValue(content)
				if val == nil && p.pos+1 < len(p.lines) {
					nextIndent := p.getIndent(p.lines[p.pos+1])
					if nextIndent > indent {
						p.pos++
						p.lineNum++
						nested, err := p.parseMap(nextIndent)
						if err != nil {
							return nil, err
						}
						nested[key] = val
						result = append(result, nested)
						continue
					}
				}
				result = append(result, map[string]any{key: val})
			}
		} else {
			// Simple value
			result = append(result, p.parseScalar(content))
		}

		p.pos++
		p.lineNum++
	}

	return result, nil
}

// getIndent returns the indentation level of a line.
func (p *YAMLParser) getIndent(line string) int {
	count := 0
indentLoop:
	for _, ch := range line {
		switch ch {
		case ' ':
			count++
		case '\t':
			count += 2
		default:
			break indentLoop
		}
	}
	return count
}

// parseKeyValue parses a key-value pair from a line.
func (p *YAMLParser) parseKeyValue(line string) (string, any, error) {
	trimmed := strings.TrimSpace(line)

	// Find the colon
	colonIdx := strings.Index(trimmed, ":")
	if colonIdx == -1 {
		return "", nil, nil
	}

	key := strings.TrimSpace(trimmed[:colonIdx])
	rest := ""
	if colonIdx+1 < len(trimmed) {
		rest = strings.TrimSpace(trimmed[colonIdx+1:])
	}

	// Remove quotes from key if present
	key = strings.Trim(key, "\"'")

	if rest == "" {
		return key, nil, nil // Value is on next lines
	}

	return key, p.parseScalar(rest), nil
}

// parseInlineKeyValue parses an inline key-value pair.
func (p *YAMLParser) parseInlineKeyValue(content string) (string, any, error) {
	colonIdx := strings.Index(content, ":")
	if colonIdx == -1 {
		return "", nil, nil
	}

	key := strings.TrimSpace(content[:colonIdx])
	rest := ""
	if colonIdx+1 < len(content) {
		rest = strings.TrimSpace(content[colonIdx+1:])
	}

	key = strings.Trim(key, "\"'")

	if rest == "" {
		return key, nil, nil
	}

	return key, p.parseScalar(rest), nil
}

// parseScalar parses a YAML scalar value.
func (p *YAMLParser) parseScalar(s string) any {
	s = strings.TrimSpace(s)

	// Handle quoted strings
	if (strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		(strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'")) {
		return s[1 : len(s)-1]
	}

	// Handle null
	if s == "null" || s == "~" || s == "" {
		return nil
	}

	// Handle booleans
	lower := strings.ToLower(s)
	if lower == "true" || lower == "yes" || lower == "on" {
		return true
	}
	if lower == "false" || lower == "no" || lower == "off" {
		return false
	}

	// Handle numbers
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return int(i)
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Handle flow sequences [a, b, c]
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return p.parseFlowSequence(s)
	}

	// Handle flow mappings {a: 1, b: 2}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		return p.parseFlowMapping(s)
	}

	return s
}

// parseFlowValue parses a flow-style value.
func (p *YAMLParser) parseFlowValue(s string) any {
	return p.parseScalar(s)
}

// parseFlowSequence parses a flow-style sequence.
func (p *YAMLParser) parseFlowSequence(s string) []any {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	s = strings.TrimSpace(s)

	if s == "" {
		return []any{}
	}

	parts := splitFlow(s, ',')
	result := make([]any, len(parts))
	for i, part := range parts {
		result[i] = p.parseScalar(part)
	}
	return result
}

// parseFlowMapping parses a flow-style mapping.
func (p *YAMLParser) parseFlowMapping(s string) map[string]any {
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	s = strings.TrimSpace(s)

	if s == "" {
		return map[string]any{}
	}

	parts := splitFlow(s, ',')
	result := make(map[string]any)
	for _, part := range parts {
		colonIdx := strings.Index(part, ":")
		if colonIdx == -1 {
			continue
		}
		key := strings.TrimSpace(part[:colonIdx])
		val := ""
		if colonIdx+1 < len(part) {
			val = strings.TrimSpace(part[colonIdx+1:])
		}
		key = strings.Trim(key, "\"'")
		result[key] = p.parseScalar(val)
	}
	return result
}

// splitFlow splits a flow-style string by delimiter, respecting nesting.
func splitFlow(s string, delim rune) []string {
	var result []string
	var current strings.Builder
	depth := 0
	inQuote := false
	quoteChar := rune(0)

	for _, ch := range s {
		if inQuote {
			current.WriteRune(ch)
			if ch == quoteChar {
				inQuote = false
			}
			continue
		}

		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			current.WriteRune(ch)
			continue
		}

		switch ch {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		}

		if ch == delim && depth == 0 {
			result = append(result, strings.TrimSpace(current.String()))
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		result = append(result, strings.TrimSpace(current.String()))
	}

	return result
}
