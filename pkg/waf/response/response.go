// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package response provides WAF response handling
package response

import (
	"encoding/json"
	"html"
	"html/template"
	"net/http"
	"sync"
)

// Handler handles WAF responses
type Handler struct {
	blockPage    string
	blockCode    int
	redirectURL  string
	jsonResponse bool
	headers      map[string]string
	mu           sync.RWMutex
}

// NewHandler creates a new response handler
func NewHandler() *Handler {
	return &Handler{
		blockCode: http.StatusForbidden,
		headers:   make(map[string]string),
	}
}

// SetBlockPage sets the custom block page HTML
func (h *Handler) SetBlockPage(html string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.blockPage = html
}

// SetBlockCode sets the HTTP status code for blocks
func (h *Handler) SetBlockCode(code int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.blockCode = code
}

// SetRedirectURL sets the redirect URL for redirects
func (h *Handler) SetRedirectURL(url string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.redirectURL = url
}

// SetJSONResponse enables JSON responses
func (h *Handler) SetJSONResponse(enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.jsonResponse = enabled
}

// SetHeader sets a custom header for responses
func (h *Handler) SetHeader(key, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.headers[key] = value
}

// BlockResponse represents a block response
type BlockResponse struct {
	Blocked   bool   `json:"blocked"`
	Reason    string `json:"reason,omitempty"`
	RuleID    string `json:"rule_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// Block sends a block response
func (h *Handler) Block(w http.ResponseWriter, reason, ruleID, requestID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Set custom headers
	for key, value := range h.headers {
		w.Header().Set(key, value)
	}

	if h.jsonResponse {
		h.blockJSON(w, reason, ruleID, requestID)
		return
	}

	h.blockHTML(w, reason, ruleID, requestID)
}

// blockJSON sends a JSON block response
func (h *Handler) blockJSON(w http.ResponseWriter, reason, ruleID, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(h.blockCode)

	resp := BlockResponse{
		Blocked:   true,
		Reason:    reason,
		RuleID:    ruleID,
		RequestID: requestID,
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// blockHTML sends an HTML block response
func (h *Handler) blockHTML(w http.ResponseWriter, reason, ruleID, requestID string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(h.blockCode)

	if h.blockPage != "" {
		// Use custom block page with pre-escaped values to prevent XSS
		// Note: html/template auto-escapes values in {{.Field}} contexts,
		// but we pre-escape here for defense in depth
		tmpl, err := template.New("block").Parse(h.blockPage)
		if err == nil {
			data := map[string]string{
				"Reason":    reason,
				"RuleID":    ruleID,
				"RequestID": requestID,
			}
			_ = tmpl.Execute(w, data)
			return
		}
	}

	// Default block page
	_, _ = w.Write([]byte(defaultBlockPage(reason, requestID)))
}

// Redirect sends a redirect response
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request, url string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Set custom headers
	for key, value := range h.headers {
		w.Header().Set(key, value)
	}

	targetURL := url
	if targetURL == "" {
		targetURL = h.redirectURL
	}

	http.Redirect(w, r, targetURL, http.StatusFound)
}

// Challenge sends a challenge response (e.g., CAPTCHA page)
func (h *Handler) Challenge(w http.ResponseWriter, requestID string) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Set custom headers
	for key, value := range h.headers {
		w.Header().Set(key, value)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)

	_, _ = w.Write([]byte(defaultChallengePage(requestID)))
}

// RateLimited sends a rate limit response
func (h *Handler) RateLimited(w http.ResponseWriter, retryAfter int) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Set custom headers
	for key, value := range h.headers {
		w.Header().Set(key, value)
	}

	w.Header().Set("Retry-After", string(rune(retryAfter)))
	w.WriteHeader(http.StatusTooManyRequests)

	if h.jsonResponse {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"error":       "rate_limited",
			"retry_after": retryAfter,
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	_, _ = w.Write([]byte(defaultRateLimitPage(retryAfter)))
}

// defaultBlockPage returns the default block page HTML
func defaultBlockPage(reason, requestID string) string {
	// Escape requestID to prevent XSS attacks
	escapedRequestID := html.EscapeString(requestID)

	return `<!DOCTYPE html>
<html>
<head>
    <title>Access Denied</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            color: #fff;
        }
        .container {
            text-align: center;
            padding: 2rem;
        }
        h1 {
            font-size: 4rem;
            margin: 0;
            color: #e94560;
        }
        h2 {
            font-size: 1.5rem;
            margin: 1rem 0;
            color: #eee;
        }
        p {
            color: #aaa;
            margin: 0.5rem 0;
        }
        .request-id {
            font-family: monospace;
            font-size: 0.8rem;
            color: #666;
            margin-top: 2rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>403</h1>
        <h2>Access Denied</h2>
        <p>Your request has been blocked by the Web Application Firewall.</p>
        <p class="request-id">Request ID: ` + escapedRequestID + `</p>
    </div>
</body>
</html>`
}

// defaultChallengePage returns the default challenge page HTML
func defaultChallengePage(requestID string) string {
	// Escape requestID to prevent XSS attacks
	escapedRequestID := html.EscapeString(requestID)

	return `<!DOCTYPE html>
<html>
<head>
    <title>Security Check</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: #f5f5f5;
        }
        .container {
            text-align: center;
            padding: 2rem;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        h2 {
            color: #333;
        }
        p {
            color: #666;
        }
        .loader {
            border: 4px solid #f3f3f3;
            border-top: 4px solid #3498db;
            border-radius: 50%;
            width: 40px;
            height: 40px;
            animation: spin 1s linear infinite;
            margin: 20px auto;
        }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="container">
        <h2>Security Check</h2>
        <div class="loader"></div>
        <p>Please wait while we verify your request...</p>
        <p style="font-size: 0.8rem; color: #999;">Request ID: ` + escapedRequestID + `</p>
    </div>
</body>
</html>`
}

// defaultRateLimitPage returns the default rate limit page
func defaultRateLimitPage(retryAfter int) string {
	return `<!DOCTYPE html>
<html>
<head>
    <title>Rate Limited</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: #fff3cd;
        }
        .container {
            text-align: center;
            padding: 2rem;
        }
        h1 {
            font-size: 4rem;
            margin: 0;
            color: #856404;
        }
        h2 {
            color: #856404;
        }
        p {
            color: #856404;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>429</h1>
        <h2>Too Many Requests</h2>
        <p>You have exceeded the rate limit. Please try again later.</p>
    </div>
</body>
</html>`
}
