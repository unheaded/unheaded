// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package detection provides XSS detection for THE SHIELD WAF
// TODO: Marked for Rust rebuild for performance - this Go implementation serves as reference
package detection

import (
	"regexp"
	"strings"
	"unicode"
)

// XSSDetector detects Cross-Site Scripting attacks with comprehensive pattern coverage
// TODO: Port to Rust with SIMD-accelerated HTML parsing for 10x performance
type XSSDetector struct {
	patterns       []*regexp.Regexp
	eventHandlers  map[string]bool
	dangerousTags  map[string]int  // Tag name to risk score
	dangerousAttrs map[string]int  // Attribute name to risk score
	jsKeywords     map[string]int  // JavaScript keywords with risk scores
	strict         bool
	htmlParser     *HTMLTokenizer
}

// HTMLTokenizer performs lightweight HTML tokenization for XSS detection
// TODO: Rust rebuild should use proper HTML5 spec-compliant parser
type HTMLTokenizer struct {
	tagPatterns map[string]*regexp.Regexp
}

// HTMLToken represents a parsed HTML element
type HTMLToken struct {
	Type       HTMLTokenType
	TagName    string
	Attributes map[string]string
	Content    string
	Risk       int
	Start      int
	End        int
}

// HTMLTokenType represents the type of HTML token
type HTMLTokenType int

const (
	HTMLTokenText HTMLTokenType = iota
	HTMLTokenOpenTag
	HTMLTokenCloseTag
	HTMLTokenSelfClosing
	HTMLTokenComment
	HTMLTokenDoctype
	HTMLTokenCDATA
)

// NewHTMLTokenizer creates a new HTML tokenizer
func NewHTMLTokenizer() *HTMLTokenizer {
	return &HTMLTokenizer{
		tagPatterns: map[string]*regexp.Regexp{
			"tag":       regexp.MustCompile(`(?i)<\s*([a-zA-Z][a-zA-Z0-9]*)[^>]*>`),
			"closeTag":  regexp.MustCompile(`(?i)<\s*/\s*([a-zA-Z][a-zA-Z0-9]*)\s*>`),
			"selfClose": regexp.MustCompile(`(?i)<\s*([a-zA-Z][a-zA-Z0-9]*)[^>]*/\s*>`),
			"comment":   regexp.MustCompile(`<!--[\s\S]*?-->`),
			"attr":      regexp.MustCompile(`(?i)([a-zA-Z][a-zA-Z0-9\-_:]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]*))`),
		},
	}
}

// Tokenize parses HTML and returns tokens
// TODO: Rust version should be streaming parser with zero allocations
func (t *HTMLTokenizer) Tokenize(input string) []HTMLToken {
	tokens := make([]HTMLToken, 0, 32)
	pos := 0
	length := len(input)

	for pos < length {
		// Find next tag start
		tagStart := strings.Index(input[pos:], "<")
		if tagStart == -1 {
			// Rest is text
			if pos < length {
				tokens = append(tokens, HTMLToken{
					Type:    HTMLTokenText,
					Content: input[pos:],
					Start:   pos,
					End:     length,
				})
			}
			break
		}

		// Add text before tag
		if tagStart > 0 {
			tokens = append(tokens, HTMLToken{
				Type:    HTMLTokenText,
				Content: input[pos : pos+tagStart],
				Start:   pos,
				End:     pos + tagStart,
			})
		}
		pos += tagStart

		// Find tag end
		tagEnd := strings.Index(input[pos:], ">")
		if tagEnd == -1 {
			// Incomplete tag
			tokens = append(tokens, HTMLToken{
				Type:    HTMLTokenText,
				Content: input[pos:],
				Start:   pos,
				End:     length,
				Risk:    3, // Incomplete tag is suspicious
			})
			break
		}

		tagContent := input[pos : pos+tagEnd+1]

		// Determine tag type
		token := t.parseTag(tagContent, pos)
		tokens = append(tokens, token)

		pos += tagEnd + 1
	}

	return tokens
}

// parseTag parses a single HTML tag
func (t *HTMLTokenizer) parseTag(tag string, start int) HTMLToken {
	token := HTMLToken{
		Content:    tag,
		Attributes: make(map[string]string),
		Start:      start,
		End:        start + len(tag),
	}

	// Check for comment
	if strings.HasPrefix(tag, "<!--") {
		token.Type = HTMLTokenComment
		return token
	}

	// Check for CDATA
	if strings.HasPrefix(strings.ToUpper(tag), "<![CDATA[") {
		token.Type = HTMLTokenCDATA
		return token
	}

	// Check for closing tag
	if len(tag) > 2 && tag[1] == '/' {
		token.Type = HTMLTokenCloseTag
		matches := t.tagPatterns["closeTag"].FindStringSubmatch(tag)
		if len(matches) > 1 {
			token.TagName = strings.ToLower(matches[1])
		}
		return token
	}

	// Check for self-closing
	if strings.HasSuffix(strings.TrimSpace(tag), "/>") {
		token.Type = HTMLTokenSelfClosing
	} else {
		token.Type = HTMLTokenOpenTag
	}

	// Extract tag name
	matches := t.tagPatterns["tag"].FindStringSubmatch(tag)
	if len(matches) > 1 {
		token.TagName = strings.ToLower(matches[1])
	}

	// Extract attributes
	attrMatches := t.tagPatterns["attr"].FindAllStringSubmatch(tag, -1)
	for _, match := range attrMatches {
		attrName := strings.ToLower(match[1])
		attrValue := ""
		if len(match) > 2 && match[2] != "" {
			attrValue = match[2]
		} else if len(match) > 3 && match[3] != "" {
			attrValue = match[3]
		} else if len(match) > 4 && match[4] != "" {
			attrValue = match[4]
		}
		token.Attributes[attrName] = attrValue
	}

	return token
}

// NewXSSDetector creates a new XSS detector
// TODO: Rust rebuild should use Aho-Corasick for multi-pattern matching
func NewXSSDetector(strict bool) *XSSDetector {
	patterns := []string{
		// Script tags - various forms
		`(?i)<\s*script[^>]*>`,
		`(?i)<\s*\/\s*script\s*>`,
		`(?i)<\s*script[^>]*\/>`,
		`(?i)<\s*script\s+[^>]*src\s*=`,
		`(?i)<\s*script[^>]*>[^<]*<\s*\/\s*script\s*>`,

		// Event handlers - comprehensive list
		`(?i)\s+on(abort|afterprint|animationend|animationiteration|animationstart|beforeprint|beforeunload|blur|canplay|canplaythrough|change|click|contextmenu|copy|cut|dblclick|drag|dragend|dragenter|dragleave|dragover|dragstart|drop|durationchange|emptied|ended|error|focus|focusin|focusout|fullscreenchange|fullscreenerror|hashchange|input|invalid|keydown|keypress|keyup|load|loadeddata|loadedmetadata|loadstart|message|mousedown|mouseenter|mouseleave|mousemove|mouseout|mouseover|mouseup|mousewheel|offline|online|open|pagehide|pageshow|paste|pause|play|playing|popstate|progress|ratechange|reset|resize|scroll|search|seeked|seeking|select|show|stalled|storage|submit|suspend|timeupdate|toggle|touchcancel|touchend|touchmove|touchstart|transitionend|unload|volumechange|waiting|wheel)\s*=`,

		// JavaScript protocol - various encodings
		`(?i)javascript\s*:`,
		`(?i)java\s*script\s*:`,
		`(?i)j\s*a\s*v\s*a\s*s\s*c\s*r\s*i\s*p\s*t\s*:`,
		`(?i)&#x?[0-9a-f]+;*j\s*a\s*v\s*a`,
		`(?i)vbscript\s*:`,
		`(?i)livescript\s*:`,
		`(?i)mocha\s*:`,

		// Data URI with script
		`(?i)data\s*:\s*text\/html`,
		`(?i)data\s*:\s*application\/x-javascript`,
		`(?i)data\s*:\s*application\/javascript`,
		`(?i)data\s*:\s*text\/javascript`,
		`(?i)data\s*:[^,]*;base64`,

		// SVG-based XSS
		`(?i)<\s*svg[^>]*>`,
		`(?i)<\s*svg[^>]*onload`,
		`(?i)<\s*svg[^>]*onerror`,
		`(?i)<\s*svg\/onload`,
		`(?i)<\s*svg[^>]*animate[^>]*>`,
		`(?i)<\s*animate[^>]*>`,
		`(?i)<\s*set[^>]*>`,

		// Object and embed tags
		`(?i)<\s*object[^>]*>`,
		`(?i)<\s*embed[^>]*>`,
		`(?i)<\s*applet[^>]*>`,
		`(?i)<\s*object[^>]*data\s*=`,
		`(?i)<\s*embed[^>]*src\s*=`,

		// iframe injection
		`(?i)<\s*iframe[^>]*>`,
		`(?i)<\s*frame[^>]*>`,
		`(?i)<\s*frameset[^>]*>`,
		`(?i)<\s*iframe[^>]*src\s*=\s*['"]?\s*javascript`,
		`(?i)<\s*iframe[^>]*srcdoc\s*=`,

		// Style-based XSS
		`(?i)<\s*style[^>]*>`,
		`(?i)expression\s*\(`,
		`(?i)url\s*\(\s*['"]?\s*javascript`,
		`(?i)url\s*\(\s*['"]?\s*data:`,
		`(?i)behavior\s*:\s*url`,
		`(?i)-moz-binding\s*:`,
		`(?i)-webkit-binding\s*:`,
		`(?i)@import\s+['"]?javascript`,
		`(?i)@import\s+['"]?data:`,

		// Link injection
		`(?i)<\s*link[^>]*href\s*=\s*['"]?\s*javascript`,
		`(?i)<\s*link[^>]*href\s*=\s*['"]?\s*data:`,
		`(?i)<\s*a[^>]*href\s*=\s*['"]?\s*javascript`,

		// Image-based XSS
		`(?i)<\s*img[^>]*src\s*=\s*['"]?\s*javascript`,
		`(?i)<\s*img[^>]*src\s*=\s*['"]?\s*data:`,
		`(?i)<\s*img[^>]*onerror`,
		`(?i)<\s*img[^>]*onload`,
		`(?i)<\s*img[^>]*dynsrc`,
		`(?i)<\s*img[^>]*lowsrc`,

		// Input-based XSS
		`(?i)<\s*input[^>]*onfocus`,
		`(?i)<\s*input[^>]*autofocus`,
		`(?i)<\s*input[^>]*type\s*=\s*['"]?image`,
		`(?i)<\s*input[^>]*formaction`,

		// Body tag attacks
		`(?i)<\s*body[^>]*onload`,
		`(?i)<\s*body[^>]*onerror`,
		`(?i)<\s*body[^>]*onpageshow`,
		`(?i)<\s*body[^>]*onfocus`,
		`(?i)<\s*body[^>]*onhashchange`,
		`(?i)<\s*body[^>]*onscroll`,

		// Meta refresh and redirect
		`(?i)<\s*meta[^>]*http-equiv\s*=\s*['"]?refresh`,
		`(?i)<\s*meta[^>]*http-equiv\s*=\s*['"]?set-cookie`,
		`(?i)<\s*meta[^>]*content\s*=\s*['"]?[^'"]*url\s*=`,

		// Base tag hijacking
		`(?i)<\s*base[^>]*href`,

		// Form action hijacking
		`(?i)<\s*form[^>]*action\s*=\s*['"]?\s*javascript`,
		`(?i)<\s*form[^>]*action\s*=\s*['"]?\s*data:`,
		`(?i)<\s*button[^>]*formaction`,

		// Video/audio
		`(?i)<\s*video[^>]*>`,
		`(?i)<\s*audio[^>]*>`,
		`(?i)<\s*source[^>]*onerror`,
		`(?i)<\s*track[^>]*>`,

		// Math ML XSS
		`(?i)<\s*math[^>]*>`,
		`(?i)<\s*maction[^>]*>`,

		// Template injection
		`\{\{.*\}\}`,
		`\$\{[^}]*\}`,
		`<%[^%]*%>`,
		`<\?[^?]*\?>`,
		`\[\[.*\]\]`,

		// DOM manipulation
		`(?i)document\s*\.\s*(cookie|domain|write|writeln|body|forms|images|links|location|referrer|title|URL)`,
		`(?i)document\s*\.\s*get(Element|Elements)(By|s)`,
		`(?i)document\s*\.\s*(create|query)`,
		`(?i)window\s*\.\s*(location|open|eval|execScript|setInterval|setTimeout|Function|name)`,
		`(?i)(eval|setTimeout|setInterval|Function|execScript)\s*\(`,
		`(?i)\.innerHTML\s*=`,
		`(?i)\.outerHTML\s*=`,
		`(?i)\.insertAdjacentHTML\s*\(`,
		`(?i)\.write\s*\(`,
		`(?i)\.writeln\s*\(`,

		// Dangerous JS
		`(?i)alert\s*\(`,
		`(?i)confirm\s*\(`,
		`(?i)prompt\s*\(`,
		`(?i)console\s*\.`,
		`(?i)debugger\s*;`,

		// Encoded attacks - URL encoding
		`%3C\s*script`,
		`%3Cscript`,
		`%253Cscript`,
		`%3C%73%63%72%69%70%74`,

		// Encoded attacks - HTML entities
		`(?i)&#x?[0-9a-f]+;\s*[<>]`,
		`(?i)&#60;`,  // <
		`(?i)&#62;`,  // >
		`(?i)&#x3c;`, // <
		`(?i)&#x3e;`, // >
		`(?i)&lt;\s*script`,
		`(?i)&lt;script`,

		// Encoded attacks - Unicode/escaped
		`(?i)\\x3c\s*script`,
		`(?i)\\u003c\s*script`,
		`(?i)\\x3cscript`,
		`(?i)\\u003cscript`,

		// Obfuscation patterns
		`(?i)fromCharCode`,
		`(?i)String\.fromCharCode`,
		`(?i)atob\s*\(`,
		`(?i)btoa\s*\(`,
		`(?i)unescape\s*\(`,
		`(?i)decodeURI(Component)?\s*\(`,
		`(?i)\.charCodeAt\s*\(`,
		`(?i)\.charAt\s*\(`,
		`(?i)\.split\s*\(['"]['"]`,
		`(?i)\.join\s*\(`,
		`(?i)\.reverse\s*\(`,

		// Constructor abuse
		`(?i)\[['"]constructor['"]]\s*\(`,
		`(?i)\.constructor\s*\(`,
		`(?i)__proto__`,
		`(?i)prototype`,

		// HTML5 specific vectors
		`(?i)<\s*details[^>]*ontoggle`,
		`(?i)<\s*marquee[^>]*>`,
		`(?i)<\s*bgsound[^>]*>`,
		`(?i)<\s*keygen[^>]*>`,
		`(?i)<\s*isindex[^>]*>`,
		`(?i)<\s*plaintext[^>]*>`,
		`(?i)<\s*xmp[^>]*>`,
		`(?i)<\s*listing[^>]*>`,

		// XML/XHTML vectors
		`(?i)<\s*xml[^>]*>`,
		`(?i)xmlns\s*=`,
		`(?i)<\s*xss[^>]*>`,

		// CSS injection
		`(?i)@charset\s*['"]`,
		`(?i)@font-face\s*\{`,
		`(?i)@keyframes\s*\{`,
	}

	// Comprehensive event handlers
	eventHandlers := map[string]bool{
		"onabort": true, "onafterprint": true, "onanimationend": true,
		"onanimationiteration": true, "onanimationstart": true,
		"onbeforeprint": true, "onbeforeunload": true, "onblur": true,
		"oncanplay": true, "oncanplaythrough": true, "onchange": true,
		"onclick": true, "oncontextmenu": true, "oncopy": true,
		"oncut": true, "ondblclick": true, "ondrag": true,
		"ondragend": true, "ondragenter": true, "ondragleave": true,
		"ondragover": true, "ondragstart": true, "ondrop": true,
		"ondurationchange": true, "onemptied": true, "onended": true,
		"onerror": true, "onfocus": true, "onfocusin": true,
		"onfocusout": true, "onfullscreenchange": true, "onfullscreenerror": true,
		"onhashchange": true, "oninput": true, "oninvalid": true,
		"onkeydown": true, "onkeypress": true, "onkeyup": true,
		"onload": true, "onloadeddata": true, "onloadedmetadata": true,
		"onloadstart": true, "onmessage": true, "onmousedown": true,
		"onmouseenter": true, "onmouseleave": true, "onmousemove": true,
		"onmouseout": true, "onmouseover": true, "onmouseup": true,
		"onmousewheel": true, "onoffline": true, "ononline": true,
		"onopen": true, "onpagehide": true, "onpageshow": true,
		"onpaste": true, "onpause": true, "onplay": true,
		"onplaying": true, "onpopstate": true, "onprogress": true,
		"onratechange": true, "onreset": true, "onresize": true,
		"onscroll": true, "onsearch": true, "onseeked": true,
		"onseeking": true, "onselect": true, "onshow": true,
		"onstalled": true, "onstorage": true, "onsubmit": true,
		"onsuspend": true, "ontimeupdate": true, "ontoggle": true,
		"ontouchcancel": true, "ontouchend": true, "ontouchmove": true,
		"ontouchstart": true, "ontransitionend": true, "onunload": true,
		"onvolumechange": true, "onwaiting": true, "onwheel": true,
		"onpointerdown": true, "onpointerup": true, "onpointermove": true,
		"onpointerenter": true, "onpointerleave": true, "onpointercancel": true,
		"onpointerover": true, "onpointerout": true, "ongotpointercapture": true,
		"onlostpointercapture": true, "onbeforeinput": true, "onformdata": true,
		"onsecuritypolicyviolation": true, "onslotchange": true,
	}

	// Dangerous HTML tags with risk scores
	dangerousTags := map[string]int{
		"script":    25, "iframe":    20, "frame":    20,
		"frameset":  15, "object":    20, "embed":    20,
		"applet":    20, "svg":       15, "math":     10,
		"style":     12, "link":      10, "base":     15,
		"form":       8, "input":      5, "button":    5,
		"select":     5, "textarea":   5, "video":     8,
		"audio":      8, "source":     5, "track":     5,
		"meta":      10, "body":       5, "img":       5,
		"a":          3, "marquee":   10, "bgsound":  10,
		"keygen":    10, "isindex":   10, "plaintext": 10,
		"xmp":       10, "listing":   10, "xml":       8,
		"details":    5, "template":  10, "slot":      5,
		"xss":       20, "noscript":   8,
	}

	// Dangerous attributes with risk scores
	dangerousAttrs := map[string]int{
		"href":        5, "src":        10, "data":       10,
		"action":     10, "formaction": 15, "srcdoc":     20,
		"poster":      5, "background":  8, "dynsrc":     10,
		"lowsrc":     10, "style":       8, "xmlns":       5,
		"xlink:href": 10, "content":     5, "http-equiv": 10,
		"srcset":      8, "ping":        5, "usemap":      5,
	}

	// JavaScript keywords that indicate code execution
	jsKeywords := map[string]int{
		"eval":            20, "setTimeout":   15, "setInterval": 15,
		"Function":        20, "constructor":  15, "prototype":   10,
		"__proto__":       15, "document":     10, "window":      10,
		"alert":            5, "confirm":      5, "prompt":       5,
		"innerHTML":       15, "outerHTML":   15, "write":       15,
		"writeln":         15, "fromCharCode": 12, "atob":        10,
		"btoa":            10, "unescape":    10, "decodeURI":   10,
		"decodeURIComponent": 10, "execScript": 20, "location":     8,
	}

	detector := &XSSDetector{
		patterns:       make([]*regexp.Regexp, 0, len(patterns)),
		eventHandlers:  eventHandlers,
		dangerousTags:  dangerousTags,
		dangerousAttrs: dangerousAttrs,
		jsKeywords:     jsKeywords,
		strict:         strict,
		htmlParser:     NewHTMLTokenizer(),
	}

	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err == nil {
			detector.patterns = append(detector.patterns, re)
		}
	}

	return detector
}

// Detect checks if the input contains XSS
func (d *XSSDetector) Detect(input string) bool {
	normalized := d.normalize(input)

	for _, pattern := range d.patterns {
		if pattern.MatchString(normalized) {
			return true
		}
	}

	// DOM-based analysis
	if d.strict {
		tokens := d.htmlParser.Tokenize(normalized)
		risk := d.analyzeHTMLTokens(tokens)
		return risk >= 15
	}

	return false
}

// DetectWithDetails returns detailed detection results
// TODO: Rust version should include byte-level position information
func (d *XSSDetector) DetectWithDetails(input string) *DetectionResult {
	result := &DetectionResult{
		Type:    "xss",
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

	// HTML token analysis
	tokens := d.htmlParser.Tokenize(normalized)
	htmlRisk := d.analyzeHTMLTokens(tokens)
	dangerousElements := d.getDangerousElements(tokens)
	result.Matches = append(result.Matches, dangerousElements...)

	// Event handler analysis
	eventRisk := d.analyzeEventHandlers(normalized)

	// JavaScript code analysis
	jsRisk := d.analyzeJavaScript(normalized)

	if htmlRisk >= 12 || eventRisk >= 10 || jsRisk >= 10 {
		result.Detected = true
	}

	if result.Detected {
		result.Score = d.calculateScore(len(result.Matches), htmlRisk, eventRisk, jsRisk)
		result.Message = "Cross-site scripting attempt detected"
	}

	return result
}

// analyzeHTMLTokens analyzes HTML tokens for XSS patterns
// TODO: Rust version should use state machine for O(n) analysis
func (d *XSSDetector) analyzeHTMLTokens(tokens []HTMLToken) int {
	risk := 0

	for _, token := range tokens {
		if token.Type == HTMLTokenOpenTag || token.Type == HTMLTokenSelfClosing {
			// Check for dangerous tags
			if score, ok := d.dangerousTags[token.TagName]; ok {
				risk += score
			}

			// Check attributes
			for attrName, attrValue := range token.Attributes {
				// Event handlers
				if d.eventHandlers[attrName] {
					risk += 15
					// Extra risk for javascript: in event handler
					if containsJavaScript(attrValue) {
						risk += 10
					}
				}

				// Dangerous attributes
				if score, ok := d.dangerousAttrs[attrName]; ok {
					risk += score
					// Extra risk for javascript: or data: in URL attributes
					if containsJavaScript(attrValue) || containsDataURI(attrValue) {
						risk += 15
					}
				}

				// Style attribute with expression or url
				if attrName == "style" {
					if containsExpression(attrValue) || containsJavaScript(attrValue) {
						risk += 20
					}
				}
			}
		}
	}

	return risk
}

// analyzeEventHandlers specifically looks for event handler patterns
func (d *XSSDetector) analyzeEventHandlers(input string) int {
	risk := 0
	lower := strings.ToLower(input)

	for handler := range d.eventHandlers {
		pattern := regexp.MustCompile(`(?i)\b` + handler + `\s*=`)
		if pattern.MatchString(lower) {
			risk += 10
		}
	}

	return risk
}

// analyzeJavaScript looks for JavaScript code patterns
func (d *XSSDetector) analyzeJavaScript(input string) int {
	risk := 0
	lower := strings.ToLower(input)

	for keyword, score := range d.jsKeywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			risk += score
		}
	}

	// Check for obfuscation patterns
	if containsStringObfuscation(input) {
		risk += 10
	}

	// Check for encoded JavaScript
	if containsEncodedJS(input) {
		risk += 15
	}

	return risk
}

// getDangerousElements extracts dangerous element descriptions
func (d *XSSDetector) getDangerousElements(tokens []HTMLToken) []string {
	var elements []string

	for _, token := range tokens {
		if token.Type == HTMLTokenOpenTag || token.Type == HTMLTokenSelfClosing {
			if _, isDangerous := d.dangerousTags[token.TagName]; isDangerous {
				elements = append(elements, "<"+token.TagName+">")
			}

			for attrName := range token.Attributes {
				if d.eventHandlers[attrName] {
					elements = append(elements, attrName+"=...")
				}
				if _, isDangerous := d.dangerousAttrs[attrName]; isDangerous {
					elements = append(elements, attrName+"=...")
				}
			}
		}
	}

	return elements
}

// normalize normalizes the input for detection
// TODO: Rust version should work on bytes directly with SIMD
func (d *XSSDetector) normalize(input string) string {
	result := input

	// URL decode multiple passes
	for i := 0; i < 3; i++ {
		prev := result
		result = strings.ReplaceAll(result, "%3C", "<")
		result = strings.ReplaceAll(result, "%3E", ">")
		result = strings.ReplaceAll(result, "%22", "\"")
		result = strings.ReplaceAll(result, "%27", "'")
		result = strings.ReplaceAll(result, "%2F", "/")
		result = strings.ReplaceAll(result, "%5C", "\\")
		result = strings.ReplaceAll(result, "%3A", ":")
		result = strings.ReplaceAll(result, "%3D", "=")
		result = strings.ReplaceAll(result, "%20", " ")
		result = strings.ReplaceAll(result, "%25", "%")
		result = strings.ReplaceAll(result, "%28", "(")
		result = strings.ReplaceAll(result, "%29", ")")
		result = strings.ReplaceAll(result, "%2B", "+")
		result = strings.ReplaceAll(result, "+", " ")
		if result == prev {
			break
		}
	}

	// Remove null bytes
	result = strings.ReplaceAll(result, "\x00", "")
	result = strings.ReplaceAll(result, "%00", "")

	// Normalize whitespace in tags (remove extra spaces between < and tag name)
	result = regexp.MustCompile(`<\s+`).ReplaceAllString(result, "<")
	result = regexp.MustCompile(`\s+>`).ReplaceAllString(result, ">")

	// Handle common HTML entity decoding
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&apos;", "'")
	result = strings.ReplaceAll(result, "&#60;", "<")
	result = strings.ReplaceAll(result, "&#62;", ">")
	result = strings.ReplaceAll(result, "&#x3c;", "<")
	result = strings.ReplaceAll(result, "&#x3e;", ">")
	result = strings.ReplaceAll(result, "&#x3C;", "<")
	result = strings.ReplaceAll(result, "&#x3E;", ">")

	return result
}

// calculateScore calculates the anomaly score based on all factors
func (d *XSSDetector) calculateScore(matchCount, htmlRisk, eventRisk, jsRisk int) int {
	baseScore := 10

	// Add match contribution
	if matchCount > 0 {
		baseScore += matchCount * 3
	}

	// Add HTML risk
	baseScore += htmlRisk / 2

	// Add event handler risk
	baseScore += eventRisk

	// Add JavaScript risk
	baseScore += jsRisk / 2

	// Cap at 100
	if baseScore > 100 {
		return 100
	}

	return baseScore
}

// GetPatternCount returns the number of patterns
func (d *XSSDetector) GetPatternCount() int {
	return len(d.patterns)
}

// Helper functions

func containsJavaScript(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "javascript:") ||
		strings.Contains(lower, "javascript :") ||
		strings.Contains(lower, "javascript\t:") ||
		strings.Contains(lower, "javascript\n:")
}

func containsDataURI(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "data:text/html") ||
		strings.Contains(lower, "data:text/javascript") ||
		strings.Contains(lower, "data:application/javascript")
}

func containsExpression(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "expression(") ||
		strings.Contains(lower, "expression (")
}

func containsStringObfuscation(s string) bool {
	lower := strings.ToLower(s)
	// Check for string splitting/joining
	if strings.Contains(lower, "fromcharcode") {
		return true
	}
	if strings.Contains(lower, ".split(") && strings.Contains(lower, ".reverse(") {
		return true
	}
	if strings.Contains(lower, ".join(") {
		// Check if there's array manipulation
		if strings.Contains(lower, "[") && strings.Contains(lower, "]") {
			return true
		}
	}
	return false
}

func containsEncodedJS(s string) bool {
	// Check for common JS encoding patterns
	patterns := []string{
		"\\x3c", "\\x3e", "\\u003c", "\\u003e",
		"\\x73\\x63\\x72\\x69\\x70\\x74", // script
	}
	lower := strings.ToLower(s)
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	// Check for Unicode escape sequences
	unicodePattern := regexp.MustCompile(`\\u[0-9a-fA-F]{4}`)
	matches := unicodePattern.FindAllString(s, -1)
	return len(matches) >= 3 // Multiple unicode escapes is suspicious
}

// IsEventHandler checks if the given string is an event handler name
func (d *XSSDetector) IsEventHandler(name string) bool {
	return d.eventHandlers[strings.ToLower(name)]
}

// IsDangerousTag checks if the given tag name is dangerous
func (d *XSSDetector) IsDangerousTag(name string) bool {
	_, ok := d.dangerousTags[strings.ToLower(name)]
	return ok
}

// HasDangerousAttribute checks if any attribute is dangerous
func (d *XSSDetector) HasDangerousAttribute(attrs map[string]string) bool {
	for name := range attrs {
		if d.eventHandlers[strings.ToLower(name)] {
			return true
		}
		if _, ok := d.dangerousAttrs[strings.ToLower(name)]; ok {
			return true
		}
	}
	return false
}

// SanitizeBasic performs basic HTML sanitization by removing dangerous elements
// NOTE: This is NOT a full sanitizer - use a proper library for production
// TODO: Rust rebuild should include high-performance sanitizer
func (d *XSSDetector) SanitizeBasic(input string) string {
	result := input

	// Remove script tags and content
	result = regexp.MustCompile(`(?i)<\s*script[^>]*>[\s\S]*?<\s*/\s*script\s*>`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`(?i)<\s*script[^>]*/?>`).ReplaceAllString(result, "")

	// Remove event handlers
	for handler := range d.eventHandlers {
		pattern := regexp.MustCompile(`(?i)\s*` + handler + `\s*=\s*("[^"]*"|'[^']*'|[^\s>]*)`)
		result = pattern.ReplaceAllString(result, "")
	}

	// Remove javascript: URLs
	result = regexp.MustCompile(`(?i)href\s*=\s*["']?\s*javascript:[^"'>\s]*["']?`).ReplaceAllString(result, "href=\"#\"")
	result = regexp.MustCompile(`(?i)src\s*=\s*["']?\s*javascript:[^"'>\s]*["']?`).ReplaceAllString(result, "")

	// Remove dangerous tags
	for tag := range d.dangerousTags {
		if tag != "img" && tag != "a" { // Keep some common tags
			pattern := regexp.MustCompile(`(?i)<\s*` + tag + `[^>]*>`)
			result = pattern.ReplaceAllString(result, "")
			pattern = regexp.MustCompile(`(?i)<\s*/\s*` + tag + `\s*>`)
			result = pattern.ReplaceAllString(result, "")
		}
	}

	return result
}

// ExtractScriptContent extracts content from script tags for analysis
func (d *XSSDetector) ExtractScriptContent(input string) []string {
	var scripts []string
	pattern := regexp.MustCompile(`(?i)<\s*script[^>]*>([\s\S]*?)<\s*/\s*script\s*>`)
	matches := pattern.FindAllStringSubmatch(input, -1)
	for _, match := range matches {
		if len(match) > 1 {
			content := strings.TrimSpace(match[1])
			if content != "" {
				scripts = append(scripts, content)
			}
		}
	}
	return scripts
}

// DetectDOMXSS specifically detects DOM-based XSS patterns
func (d *XSSDetector) DetectDOMXSS(input string) bool {
	domPatterns := []string{
		`(?i)document\.write`,
		`(?i)document\.writeln`,
		`(?i)\.innerHTML\s*=`,
		`(?i)\.outerHTML\s*=`,
		`(?i)\.insertAdjacentHTML`,
		`(?i)eval\s*\(`,
		`(?i)setTimeout\s*\(.*,`,
		`(?i)setInterval\s*\(.*,`,
		`(?i)new\s+Function\s*\(`,
		`(?i)location\s*=`,
		`(?i)location\.href\s*=`,
		`(?i)location\.replace\s*\(`,
		`(?i)location\.assign\s*\(`,
		`(?i)document\.location`,
		`(?i)window\.location`,
	}

	for _, p := range domPatterns {
		re := regexp.MustCompile(p)
		if re.MatchString(input) {
			return true
		}
	}

	return false
}

// CalculateXSSComplexity estimates the sophistication of an XSS payload
func (d *XSSDetector) CalculateXSSComplexity(input string) int {
	complexity := 0
	lower := strings.ToLower(input)

	// Encoding layers
	if strings.Contains(lower, "%") {
		complexity += 1
	}
	if strings.Contains(lower, "&#") {
		complexity += 2
	}
	if strings.Contains(lower, "\\u") || strings.Contains(lower, "\\x") {
		complexity += 3
	}

	// Obfuscation techniques
	if strings.Contains(lower, "fromcharcode") {
		complexity += 3
	}
	if strings.Contains(lower, "atob") {
		complexity += 2
	}
	if strings.Contains(lower, "eval") {
		complexity += 2
	}

	// Multiple vectors
	var vectorCount int
	if strings.Contains(lower, "<script") {
		vectorCount++
	}
	if strings.Contains(lower, "onerror") || strings.Contains(lower, "onload") {
		vectorCount++
	}
	if strings.Contains(lower, "javascript:") {
		vectorCount++
	}
	if strings.Contains(lower, "data:") {
		vectorCount++
	}
	complexity += vectorCount

	// Polyglot patterns
	if hasMultipleContexts(input) {
		complexity += 5
	}

	return complexity
}

// hasMultipleContexts checks if payload works in multiple contexts
func hasMultipleContexts(input string) bool {
	contexts := 0
	lower := strings.ToLower(input)

	// HTML context
	if strings.Contains(lower, "<") && strings.Contains(lower, ">") {
		contexts++
	}
	// JavaScript context
	if strings.Contains(lower, "alert") || strings.Contains(lower, "eval") || strings.Contains(lower, "function") {
		contexts++
	}
	// URL context
	if strings.Contains(lower, "javascript:") || strings.Contains(lower, "data:") {
		contexts++
	}
	// CSS context
	if strings.Contains(lower, "expression(") || strings.Contains(lower, "url(") {
		contexts++
	}

	// Check for quote balancing (polyglot indicator)
	singleQuotes := strings.Count(input, "'")
	doubleQuotes := strings.Count(input, "\"")
	if singleQuotes > 2 && doubleQuotes > 2 {
		contexts++
	}

	return contexts >= 3
}

// IsControlCharacterAbuse checks for control character abuse in XSS
func IsControlCharacterAbuse(input string) bool {
	for _, r := range input {
		// Check for non-printable control characters (except common whitespace)
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return true
		}
		// Check for Unicode control characters
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
