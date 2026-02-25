// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package rules provides a custom DSL (Domain Specific Language) for WAF rules
package rules

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DSLParser parses the custom WAF rule language
// Syntax:
//   RULE id "name" {
//     MATCH pattern ON target [AND|OR target...]
//     ACTION block|allow|log|redirect|challenge
//     SEVERITY critical|high|medium|low|info
//     SCORE 10
//     MESSAGE "description"
//     TAGS tag1, tag2, tag3
//
//     CHAIN AND|OR {
//       RULE child_id "child_name" { ... }
//     }
//
//     RATELIMIT 100 PER minute ACTION block
//
//     EXCEPT {
//       IP 192.168.1.0/24
//       PATH /api/health*
//       METHOD GET, POST
//       HEADER X-Internal = "true"
//       TIME 09:00-17:00 MON-FRI
//     }
//   }
type DSLParser struct {
	tokens    []Token
	pos       int
	strict    bool
	variables map[string]string
}

// TokenType represents the type of token
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenRule
	TokenMatch
	TokenOn
	TokenAction
	TokenSeverity
	TokenScore
	TokenMessage
	TokenTags
	TokenChain
	TokenRateLimit
	TokenPer
	TokenExcept
	TokenIP
	TokenPath
	TokenMethod
	TokenHeader
	TokenTime
	TokenCountry
	TokenUserAgent
	TokenAnd
	TokenOr
	TokenNot
	TokenXor
	TokenLBrace
	TokenRBrace
	TokenLParen
	TokenRParen
	TokenComma
	TokenEquals
	TokenString
	TokenNumber
	TokenIdent
	TokenPattern
	TokenTarget
	TokenSeverityValue
	TokenActionValue
	TokenTimeUnit
	TokenVersion
	TokenPriority
	TokenEnabled
	TokenDisabled
)

// Token represents a lexical token
type Token struct {
	Type    TokenType
	Value   string
	Line    int
	Column  int
}

// NewDSLParser creates a new DSL parser
func NewDSLParser(strict bool) *DSLParser {
	return &DSLParser{
		strict:    strict,
		variables: make(map[string]string),
	}
}

// Parse parses DSL from a reader
func (p *DSLParser) Parse(r io.Reader) ([]*ChainedRule, error) {
	// Read all content
	content, err := readAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	// Tokenize
	p.tokens, err = p.tokenize(content)
	if err != nil {
		return nil, fmt.Errorf("tokenization failed: %w", err)
	}
	p.pos = 0

	// Parse rules
	rules := make([]*ChainedRule, 0)
	for !p.isAtEnd() {
		rule, err := p.parseRule()
		if err != nil {
			if p.strict {
				return nil, err
			}
			// Skip to next rule
			p.skipToNextRule()
			continue
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

// ParseString parses DSL from a string
func (p *DSLParser) ParseString(input string) ([]*ChainedRule, error) {
	return p.Parse(strings.NewReader(input))
}

// tokenize converts input into tokens
func (p *DSLParser) tokenize(input string) ([]Token, error) {
	tokens := make([]Token, 0)
	line := 1
	col := 1
	i := 0

	for i < len(input) {
		ch := input[i]

		// Skip whitespace
		if ch == ' ' || ch == '\t' || ch == '\r' {
			col++
			i++
			continue
		}

		// Handle newlines
		if ch == '\n' {
			line++
			col = 1
			i++
			continue
		}

		// Skip comments
		if ch == '#' || (i+1 < len(input) && ch == '/' && input[i+1] == '/') {
			for i < len(input) && input[i] != '\n' {
				i++
			}
			continue
		}

		// Multi-line comments
		if i+1 < len(input) && ch == '/' && input[i+1] == '*' {
			i += 2
			col += 2
			for i+1 < len(input) {
				if input[i] == '*' && input[i+1] == '/' {
					i += 2
					col += 2
					break
				}
				if input[i] == '\n' {
					line++
					col = 1
				} else {
					col++
				}
				i++
			}
			continue
		}

		// Single character tokens
		switch ch {
		case '{':
			tokens = append(tokens, Token{TokenLBrace, "{", line, col})
			i++
			col++
			continue
		case '}':
			tokens = append(tokens, Token{TokenRBrace, "}", line, col})
			i++
			col++
			continue
		case '(':
			tokens = append(tokens, Token{TokenLParen, "(", line, col})
			i++
			col++
			continue
		case ')':
			tokens = append(tokens, Token{TokenRParen, ")", line, col})
			i++
			col++
			continue
		case ',':
			tokens = append(tokens, Token{TokenComma, ",", line, col})
			i++
			col++
			continue
		case '=':
			tokens = append(tokens, Token{TokenEquals, "=", line, col})
			i++
			col++
			continue
		}

		// String literals
		if ch == '"' || ch == '\'' {
			quote := ch
			startCol := col
			i++
			col++
			start := i
			for i < len(input) && input[i] != quote {
				if input[i] == '\\' && i+1 < len(input) {
					i += 2
					col += 2
				} else {
					if input[i] == '\n' {
						line++
						col = 1
					} else {
						col++
					}
					i++
				}
			}
			if i >= len(input) {
				return nil, fmt.Errorf("unterminated string at line %d, column %d", line, startCol)
			}
			value := input[start:i]
			tokens = append(tokens, Token{TokenString, value, line, startCol})
			i++
			col++
			continue
		}

		// Regex patterns (enclosed in /.../)
		if ch == '/' {
			startCol := col
			i++
			col++
			start := i
			for i < len(input) && input[i] != '/' {
				if input[i] == '\\' && i+1 < len(input) {
					i += 2
					col += 2
				} else {
					i++
					col++
				}
			}
			if i >= len(input) {
				return nil, fmt.Errorf("unterminated pattern at line %d, column %d", line, startCol)
			}
			value := input[start:i]
			tokens = append(tokens, Token{TokenPattern, value, line, startCol})
			i++
			col++
			continue
		}

		// Numbers
		if isDigit(ch) {
			startCol := col
			start := i
			for i < len(input) && isDigit(input[i]) {
				i++
				col++
			}
			value := input[start:i]
			tokens = append(tokens, Token{TokenNumber, value, line, startCol})
			continue
		}

		// Identifiers and keywords
		if isAlpha(ch) || ch == '_' {
			startCol := col
			start := i
			for i < len(input) && (isAlphaNumeric(input[i]) || input[i] == '_' || input[i] == '-') {
				i++
				col++
			}
			value := input[start:i]
			tokenType := p.getKeywordToken(value)
			tokens = append(tokens, Token{tokenType, value, line, startCol})
			continue
		}

		// Unknown character
		return nil, fmt.Errorf("unexpected character '%c' at line %d, column %d", ch, line, col)
	}

	tokens = append(tokens, Token{TokenEOF, "", line, col})
	return tokens, nil
}

// getKeywordToken returns the token type for a keyword or identifier
func (p *DSLParser) getKeywordToken(value string) TokenType {
	upper := toUpper(value)
	switch upper {
	case "RULE":
		return TokenRule
	case "MATCH":
		return TokenMatch
	case "ON":
		return TokenOn
	case "ACTION":
		return TokenAction
	case "SEVERITY":
		return TokenSeverity
	case "SCORE":
		return TokenScore
	case "MESSAGE":
		return TokenMessage
	case "TAGS":
		return TokenTags
	case "CHAIN":
		return TokenChain
	case "RATELIMIT":
		return TokenRateLimit
	case "PER":
		return TokenPer
	case "EXCEPT", "EXCEPTION":
		return TokenExcept
	case "IP":
		return TokenIP
	case "PATH":
		return TokenPath
	case "METHOD":
		return TokenMethod
	case "HEADER":
		return TokenHeader
	case "TIME":
		return TokenTime
	case "COUNTRY":
		return TokenCountry
	case "USERAGENT", "UA":
		return TokenUserAgent
	case "AND":
		return TokenAnd
	case "OR":
		return TokenOr
	case "NOT":
		return TokenNot
	case "XOR":
		return TokenXor
	case "VERSION":
		return TokenVersion
	case "PRIORITY":
		return TokenPriority
	case "ENABLED":
		return TokenEnabled
	case "DISABLED":
		return TokenDisabled
	// Targets
	case "URL", "QUERY", "BODY", "HEADERS", "COOKIES", "ALL":
		return TokenTarget
	// Actions
	case "BLOCK", "ALLOW", "LOG", "REDIRECT", "CHALLENGE":
		return TokenActionValue
	// Severities
	case "CRITICAL", "HIGH", "MEDIUM", "LOW", "INFO":
		return TokenSeverityValue
	// Time units
	case "SECOND", "SECONDS", "MINUTE", "MINUTES", "HOUR", "HOURS", "DAY", "DAYS":
		return TokenTimeUnit
	default:
		return TokenIdent
	}
}

// parseRule parses a single rule definition
func (p *DSLParser) parseRule() (*ChainedRule, error) {
	// Expect RULE keyword
	if !p.match(TokenRule) {
		return nil, fmt.Errorf("expected RULE at line %d", p.current().Line)
	}

	// Parse rule ID
	if !p.check(TokenIdent) && !p.check(TokenString) {
		return nil, fmt.Errorf("expected rule ID at line %d", p.current().Line)
	}
	ruleID := p.advance().Value

	// Parse rule name
	if !p.check(TokenString) {
		return nil, fmt.Errorf("expected rule name string at line %d", p.current().Line)
	}
	ruleName := p.advance().Value

	// Expect opening brace
	if !p.match(TokenLBrace) {
		return nil, fmt.Errorf("expected '{' at line %d", p.current().Line)
	}

	// Create base rule
	baseRule := &Rule{
		ID:       ruleID,
		Name:     ruleName,
		Targets:  []Target{TargetAll},
		Action:   ActionBlock,
		Severity: SeverityMedium,
		Score:    5,
		Enabled:  true,
	}

	chainedRule := NewChainedRule(baseRule)

	// Parse rule body
	for !p.check(TokenRBrace) && !p.isAtEnd() {
		err := p.parseRuleProperty(chainedRule)
		if err != nil {
			if p.strict {
				return nil, err
			}
			// Skip to next property
			p.skipToNextProperty()
		}
	}

	// Expect closing brace
	if !p.match(TokenRBrace) {
		return nil, fmt.Errorf("expected '}' at line %d", p.current().Line)
	}

	// Compile the pattern
	if chainedRule.RawPattern != "" {
		re, err := regexp.Compile("(?i)" + chainedRule.RawPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern '%s': %w", chainedRule.RawPattern, err)
		}
		chainedRule.Pattern = re
	}

	return chainedRule, nil
}

// parseRuleProperty parses a single property in a rule
func (p *DSLParser) parseRuleProperty(rule *ChainedRule) error {
	switch {
	case p.match(TokenMatch):
		return p.parseMatch(rule)
	case p.match(TokenAction):
		return p.parseAction(rule)
	case p.match(TokenSeverity):
		return p.parseSeverity(rule)
	case p.match(TokenScore):
		return p.parseScore(rule)
	case p.match(TokenMessage):
		return p.parseMessage(rule)
	case p.match(TokenTags):
		return p.parseTags(rule)
	case p.match(TokenChain):
		return p.parseChain(rule)
	case p.match(TokenRateLimit):
		return p.parseRateLimit(rule)
	case p.match(TokenExcept):
		return p.parseException(rule)
	case p.match(TokenVersion):
		return p.parseVersion(rule)
	case p.match(TokenPriority):
		return p.parsePriority(rule)
	case p.match(TokenEnabled):
		rule.Enabled = true
		return nil
	case p.match(TokenDisabled):
		rule.Enabled = false
		return nil
	default:
		return fmt.Errorf("unexpected token '%s' at line %d", p.current().Value, p.current().Line)
	}
}

// parseMatch parses a MATCH clause
func (p *DSLParser) parseMatch(rule *ChainedRule) error {
	// Parse pattern
	if p.check(TokenPattern) {
		rule.RawPattern = p.advance().Value
	} else if p.check(TokenString) {
		rule.RawPattern = p.advance().Value
	} else {
		return fmt.Errorf("expected pattern at line %d", p.current().Line)
	}

	// Parse ON target(s)
	if p.match(TokenOn) {
		targets, err := p.parseTargets()
		if err != nil {
			return err
		}
		rule.Targets = targets
	}

	return nil
}

// parseTargets parses target specifications
func (p *DSLParser) parseTargets() ([]Target, error) {
	targets := make([]Target, 0)

	for {
		target, err := p.parseTarget()
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)

		// Check for AND/OR/comma
		if p.match(TokenAnd) || p.match(TokenOr) || p.match(TokenComma) {
			continue
		}
		break
	}

	return targets, nil
}

// parseTarget parses a single target
func (p *DSLParser) parseTarget() (Target, error) {
	token := p.advance()
	value := toUpper(token.Value)

	switch value {
	case "URL":
		return TargetURL, nil
	case "PATH":
		return TargetPath, nil
	case "QUERY":
		return TargetQuery, nil
	case "BODY":
		return TargetBody, nil
	case "HEADERS":
		return TargetHeaders, nil
	case "COOKIES":
		return TargetCookies, nil
	case "METHOD":
		return TargetMethod, nil
	case "IP":
		return TargetIP, nil
	case "USERAGENT", "UA":
		return TargetUserAgent, nil
	case "ALL", "*":
		return TargetAll, nil
	default:
		return TargetAll, fmt.Errorf("unknown target '%s' at line %d", token.Value, token.Line)
	}
}

// parseAction parses an ACTION clause
func (p *DSLParser) parseAction(rule *ChainedRule) error {
	token := p.advance()
	value := toUpper(token.Value)

	switch value {
	case "BLOCK":
		rule.Action = ActionBlock
	case "ALLOW":
		rule.Action = ActionAllow
	case "LOG":
		rule.Action = ActionLog
	case "REDIRECT":
		rule.Action = ActionRedirect
		// Parse redirect URL if provided
		if p.check(TokenString) {
			rule.RedirectURL = p.advance().Value
		}
	case "CHALLENGE":
		rule.Action = ActionChallenge
	default:
		return fmt.Errorf("unknown action '%s' at line %d", token.Value, token.Line)
	}

	return nil
}

// parseSeverity parses a SEVERITY clause
func (p *DSLParser) parseSeverity(rule *ChainedRule) error {
	token := p.advance()
	value := toUpper(token.Value)

	switch value {
	case "CRITICAL":
		rule.Severity = SeverityCritical
		rule.Score = 20
	case "HIGH":
		rule.Severity = SeverityHigh
		rule.Score = 10
	case "MEDIUM", "MED":
		rule.Severity = SeverityMedium
		rule.Score = 5
	case "LOW":
		rule.Severity = SeverityLow
		rule.Score = 2
	case "INFO":
		rule.Severity = SeverityInfo
		rule.Score = 1
	default:
		return fmt.Errorf("unknown severity '%s' at line %d", token.Value, token.Line)
	}

	return nil
}

// parseScore parses a SCORE clause
func (p *DSLParser) parseScore(rule *ChainedRule) error {
	if !p.check(TokenNumber) {
		return fmt.Errorf("expected number after SCORE at line %d", p.current().Line)
	}

	score, err := strconv.Atoi(p.advance().Value)
	if err != nil {
		return fmt.Errorf("invalid score value: %w", err)
	}

	rule.Score = score
	return nil
}

// parseMessage parses a MESSAGE clause
func (p *DSLParser) parseMessage(rule *ChainedRule) error {
	if !p.check(TokenString) {
		return fmt.Errorf("expected string after MESSAGE at line %d", p.current().Line)
	}

	rule.Message = p.advance().Value
	return nil
}

// parseTags parses a TAGS clause
func (p *DSLParser) parseTags(rule *ChainedRule) error {
	tags := make([]string, 0)

	for {
		if p.check(TokenIdent) || p.check(TokenString) {
			tags = append(tags, p.advance().Value)
		} else {
			break
		}

		if !p.match(TokenComma) {
			break
		}
	}

	rule.Tags = tags
	return nil
}

// parseChain parses a CHAIN clause
func (p *DSLParser) parseChain(rule *ChainedRule) error {
	// Parse operator
	token := p.advance()
	value := toUpper(token.Value)

	switch value {
	case "AND":
		rule.Operator = ChainAND
	case "OR":
		rule.Operator = ChainOR
	case "NOT":
		rule.Operator = ChainNOT
	case "XOR":
		rule.Operator = ChainXOR
	default:
		return fmt.Errorf("unknown chain operator '%s' at line %d", token.Value, token.Line)
	}

	// Expect opening brace
	if !p.match(TokenLBrace) {
		return fmt.Errorf("expected '{' after chain operator at line %d", p.current().Line)
	}

	// Parse child rules
	for !p.check(TokenRBrace) && !p.isAtEnd() {
		childRule, err := p.parseRule()
		if err != nil {
			if p.strict {
				return err
			}
			p.skipToNextRule()
			continue
		}
		rule.AddChild(childRule)
	}

	// Expect closing brace
	if !p.match(TokenRBrace) {
		return fmt.Errorf("expected '}' at line %d", p.current().Line)
	}

	return nil
}

// parseRateLimit parses a RATELIMIT clause
func (p *DSLParser) parseRateLimit(rule *ChainedRule) error {
	// Parse count
	if !p.check(TokenNumber) {
		return fmt.Errorf("expected number after RATELIMIT at line %d", p.current().Line)
	}
	count, _ := strconv.Atoi(p.advance().Value)

	// Expect PER
	if !p.match(TokenPer) {
		return fmt.Errorf("expected PER after rate limit count at line %d", p.current().Line)
	}

	// Parse time unit
	token := p.advance()
	window := p.parseTimeUnit(token.Value)

	// Parse action if specified
	action := ActionBlock
	if p.match(TokenAction) {
		actionToken := p.advance()
		switch toUpper(actionToken.Value) {
		case "BLOCK":
			action = ActionBlock
		case "LOG":
			action = ActionLog
		case "CHALLENGE":
			action = ActionChallenge
		}
	}

	rule.SetRateLimit(count, window, action)
	return nil
}

// parseTimeUnit parses a time unit string
func (p *DSLParser) parseTimeUnit(unit string) time.Duration {
	switch toUpper(unit) {
	case "SECOND", "SECONDS", "S":
		return time.Second
	case "MINUTE", "MINUTES", "M":
		return time.Minute
	case "HOUR", "HOURS", "H":
		return time.Hour
	case "DAY", "DAYS", "D":
		return 24 * time.Hour
	default:
		return time.Minute
	}
}

// parseException parses an EXCEPT clause
func (p *DSLParser) parseException(rule *ChainedRule) error {
	exception := &ExceptionRule{
		ID:      generateExceptionID(),
		Enabled: true,
	}

	// Expect opening brace
	if !p.match(TokenLBrace) {
		return fmt.Errorf("expected '{' after EXCEPT at line %d", p.current().Line)
	}

	// Parse exception conditions
	for !p.check(TokenRBrace) && !p.isAtEnd() {
		switch {
		case p.match(TokenIP):
			ips := p.parseStringList()
			exception.SourceIPs = append(exception.SourceIPs, ips...)
		case p.match(TokenPath):
			paths := p.parseStringList()
			exception.Paths = append(exception.Paths, paths...)
		case p.match(TokenMethod):
			methods := p.parseStringList()
			exception.Methods = append(exception.Methods, methods...)
		case p.match(TokenHeader):
			key := p.advance().Value
			if p.match(TokenEquals) {
				value := p.advance().Value
				if exception.Headers == nil {
					exception.Headers = make(map[string]string)
				}
				exception.Headers[key] = value
			}
		case p.match(TokenCountry):
			countries := p.parseStringList()
			exception.Countries = append(exception.Countries, countries...)
		case p.match(TokenUserAgent):
			uas := p.parseStringList()
			exception.UserAgents = append(exception.UserAgents, uas...)
		case p.match(TokenTime):
			// Parse time range (e.g., "09:00-17:00")
			if p.check(TokenString) {
				timeStr := p.advance().Value
				tr, err := p.parseTimeRange(timeStr)
				if err == nil {
					exception.TimeRanges = append(exception.TimeRanges, tr)
				}
			}
		default:
			p.advance() // Skip unknown token
		}
	}

	// Expect closing brace
	if !p.match(TokenRBrace) {
		return fmt.Errorf("expected '}' at line %d", p.current().Line)
	}

	rule.AddException(exception)
	return nil
}

// parseStringList parses a comma-separated list of strings or identifiers
func (p *DSLParser) parseStringList() []string {
	list := make([]string, 0)

	for {
		if p.check(TokenString) || p.check(TokenIdent) || p.check(TokenPattern) {
			list = append(list, p.advance().Value)
		} else {
			break
		}

		if !p.match(TokenComma) {
			break
		}
	}

	return list
}

// parseTimeRange parses a time range string like "09:00-17:00"
func (p *DSLParser) parseTimeRange(s string) (TimeRange, error) {
	parts := splitByChar(s, '-')
	if len(parts) != 2 {
		return TimeRange{}, errors.New("invalid time range format")
	}

	start, err := p.parseTimeOfDay(parts[0])
	if err != nil {
		return TimeRange{}, err
	}

	end, err := p.parseTimeOfDay(parts[1])
	if err != nil {
		return TimeRange{}, err
	}

	return TimeRange{Start: start, End: end}, nil
}

// parseTimeOfDay parses a time string like "09:00"
func (p *DSLParser) parseTimeOfDay(s string) (time.Duration, error) {
	parts := splitByChar(s, ':')
	if len(parts) != 2 {
		return 0, errors.New("invalid time format")
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}

	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}

	return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute, nil
}

// parseVersion parses a VERSION clause
func (p *DSLParser) parseVersion(rule *ChainedRule) error {
	if !p.check(TokenNumber) {
		return fmt.Errorf("expected number after VERSION at line %d", p.current().Line)
	}

	version, _ := strconv.Atoi(p.advance().Value)
	rule.Version = version
	return nil
}

// parsePriority parses a PRIORITY clause
func (p *DSLParser) parsePriority(rule *ChainedRule) error {
	if !p.check(TokenNumber) {
		return fmt.Errorf("expected number after PRIORITY at line %d", p.current().Line)
	}

	priority, _ := strconv.Atoi(p.advance().Value)
	rule.Priority = priority
	return nil
}

// Helper methods for parser

func (p *DSLParser) current() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *DSLParser) advance() Token {
	if !p.isAtEnd() {
		p.pos++
	}
	return p.tokens[p.pos-1]
}

func (p *DSLParser) check(tokenType TokenType) bool {
	if p.isAtEnd() {
		return false
	}
	return p.current().Type == tokenType
}

func (p *DSLParser) match(tokenType TokenType) bool {
	if p.check(tokenType) {
		p.advance()
		return true
	}
	return false
}

func (p *DSLParser) isAtEnd() bool {
	return p.current().Type == TokenEOF
}

func (p *DSLParser) skipToNextRule() {
	for !p.isAtEnd() {
		if p.current().Type == TokenRule {
			return
		}
		p.advance()
	}
}

func (p *DSLParser) skipToNextProperty() {
	braceCount := 0
	for !p.isAtEnd() {
		switch p.current().Type {
		case TokenLBrace:
			braceCount++
		case TokenRBrace:
			braceCount--
			if braceCount < 0 {
				return
			}
		case TokenMatch, TokenAction, TokenSeverity, TokenScore, TokenMessage,
			TokenTags, TokenChain, TokenRateLimit, TokenExcept:
			if braceCount == 0 {
				return
			}
		}
		p.advance()
	}
}

// Helper functions

func readAll(r io.Reader) (string, error) {
	var builder strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			builder.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return builder.String(), nil
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isAlphaNumeric(ch byte) bool {
	return isAlpha(ch) || isDigit(ch)
}

func toUpper(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= 'a' && ch <= 'z' {
			result[i] = ch - 32
		} else {
			result[i] = ch
		}
	}
	return string(result)
}

func splitByChar(s string, sep byte) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

var exceptionCounter int64

func generateExceptionID() string {
	exceptionCounter++
	return fmt.Sprintf("EXC-%d-%d", time.Now().Unix(), exceptionCounter)
}

// CompileDSL compiles DSL source into a RuleSet
func CompileDSL(source string) (*RuleSet, error) {
	parser := NewDSLParser(true)
	chainedRules, err := parser.ParseString(source)
	if err != nil {
		return nil, err
	}

	ruleSet := NewRuleSet("dsl-rules", "Rules compiled from DSL")
	for _, cr := range chainedRules {
		ruleSet.AddRule(cr.Rule)
	}

	return ruleSet, nil
}

// CompileDSLToChains compiles DSL source into ChainedRules
func CompileDSLToChains(source string) ([]*ChainedRule, error) {
	parser := NewDSLParser(true)
	return parser.ParseString(source)
}
