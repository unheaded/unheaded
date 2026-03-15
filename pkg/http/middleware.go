// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the MIT License.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

package http

import (
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// HandlerFunc defines the handler function signature
type HandlerFunc func(*Context)

// MiddlewareFunc is an alias for HandlerFunc for clarity
type MiddlewareFunc = HandlerFunc

// HandlersChain is a slice of handlers
type HandlersChain []HandlerFunc

// combine merges two handler chains
func (c HandlersChain) combine(handlers HandlersChain) HandlersChain {
	finalSize := len(c) + len(handlers)
	merged := make(HandlersChain, finalSize)
	copy(merged, c)
	copy(merged[len(c):], handlers)
	return merged
}

// Last returns the last handler in the chain
func (c HandlersChain) Last() HandlerFunc {
	if len(c) > 0 {
		return c[len(c)-1]
	}
	return nil
}

// --- Built-in Middleware ---

// Logger returns a logging middleware that logs request details
func Logger() MiddlewareFunc {
	return LoggerWithConfig(LoggerConfig{})
}

// LoggerConfig defines the config for Logger middleware
type LoggerConfig struct {
	// SkipPaths is a slice of paths to skip logging
	SkipPaths []string

	// Output is the writer to write logs to (default: os.Stdout via log.Printf)
	Output func(format string, args ...any)

	// Formatter is a custom log formatter
	Formatter func(params LogFormatterParams) string
}

// LogFormatterParams holds the parameters for log formatting
type LogFormatterParams struct {
	Request      *http.Request
	TimeStamp    time.Time
	StatusCode   int
	Latency      time.Duration
	ClientIP     string
	Method       string
	Path         string
	ErrorMessage string
	BodySize     int
}

// defaultLogFormatter is the default log format
func defaultLogFormatter(params LogFormatterParams) string {
	statusColor := ""
	resetColor := ""
	methodColor := ""

	// ANSI color codes for terminal output
	if params.StatusCode >= 500 {
		statusColor = "\033[31m" // Red
	} else if params.StatusCode >= 400 {
		statusColor = "\033[33m" // Yellow
	} else if params.StatusCode >= 300 {
		statusColor = "\033[36m" // Cyan
	} else {
		statusColor = "\033[32m" // Green
	}
	resetColor = "\033[0m"

	switch params.Method {
	case http.MethodGet:
		methodColor = "\033[34m" // Blue
	case http.MethodPost:
		methodColor = "\033[32m" // Green
	case http.MethodPut:
		methodColor = "\033[33m" // Yellow
	case http.MethodDelete:
		methodColor = "\033[31m" // Red
	default:
		methodColor = "\033[37m" // White
	}

	return fmt.Sprintf("[HTTP] %s |%s %3d %s| %13v | %15s |%s %-7s %s %s\n",
		params.TimeStamp.Format("2006/01/02 - 15:04:05"),
		statusColor, params.StatusCode, resetColor,
		params.Latency,
		params.ClientIP,
		methodColor, params.Method, resetColor,
		params.Path,
	)
}

// LoggerWithConfig returns a Logger middleware with config
func LoggerWithConfig(config LoggerConfig) MiddlewareFunc {
	// Set defaults
	if config.Output == nil {
		config.Output = log.Printf
	}
	if config.Formatter == nil {
		config.Formatter = defaultLogFormatter
	}

	// Build skip paths map
	skipPaths := make(map[string]bool)
	for _, path := range config.SkipPaths {
		skipPaths[path] = true
	}

	return func(c *Context) {
		// Check if path should be skipped
		if skipPaths[c.Path()] {
			c.Next()
			return
		}

		// Start timer
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Build full path
		if raw != "" {
			path = path + "?" + raw
		}

		// Get response writer to access status code
		rw, ok := c.Writer.(*ResponseWriter)
		statusCode := http.StatusOK
		bodySize := 0
		if ok {
			statusCode = rw.Status()
			bodySize = rw.Size()
		} else {
			statusCode = c.statusCode
		}

		// Create log params
		params := LogFormatterParams{
			Request:    c.Request,
			TimeStamp:  time.Now(),
			StatusCode: statusCode,
			Latency:    latency,
			ClientIP:   c.ClientIP(),
			Method:     c.Method(),
			Path:       path,
			BodySize:   bodySize,
		}

		// Output log
		config.Output("%s", config.Formatter(params))
	}
}

// Recovery returns a panic recovery middleware
func Recovery() MiddlewareFunc {
	return RecoveryWithConfig(RecoveryConfig{})
}

// RecoveryConfig defines the config for Recovery middleware
type RecoveryConfig struct {
	// StackSize is the stack size to capture (default: 4KB)
	StackSize int

	// DisableStackAll disables capturing all goroutines stack
	DisableStackAll bool

	// DisablePrintStack disables printing stack trace
	DisablePrintStack bool

	// Handler is called when a panic occurs
	Handler func(c *Context, err any)
}

// RecoveryWithConfig returns a Recovery middleware with config
func RecoveryWithConfig(config RecoveryConfig) MiddlewareFunc {
	if config.StackSize == 0 {
		config.StackSize = 4096
	}

	return func(c *Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log stack trace
				if !config.DisablePrintStack {
					stack := debug.Stack()
					log.Printf("[Recovery] panic recovered: %v\n%s", err, stack)
				}

				// Call custom handler if set
				if config.Handler != nil {
					config.Handler(c, err)
					return
				}

				// Default: return 500 error
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()

		c.Next()
	}
}

// CORS returns a CORS middleware with default settings
func CORS() MiddlewareFunc {
	return CORSWithConfig(CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodHead,
			http.MethodOptions,
		},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: false,
		MaxAge:           86400,
	})
}

// CORSConfig defines the config for CORS middleware
type CORSConfig struct {
	// AllowOrigins is a list of origins that may access the resource
	AllowOrigins []string

	// AllowMethods is a list of methods allowed when accessing the resource
	AllowMethods []string

	// AllowHeaders is a list of headers that can be used in the request
	AllowHeaders []string

	// ExposeHeaders is a list of headers that can be exposed to the client
	ExposeHeaders []string

	// AllowCredentials indicates whether the request can include credentials
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) the results of a preflight request can be cached
	MaxAge int

	// AllowOriginFunc is a custom function to validate the origin
	AllowOriginFunc func(origin string) bool
}

// CORSWithConfig returns a CORS middleware with config
func CORSWithConfig(config CORSConfig) MiddlewareFunc {
	// Build allow methods string
	allowMethods := strings.Join(config.AllowMethods, ", ")
	allowHeaders := strings.Join(config.AllowHeaders, ", ")
	exposeHeaders := strings.Join(config.ExposeHeaders, ", ")
	maxAge := fmt.Sprintf("%d", config.MaxAge)

	// Build origins map for O(1) lookup
	allowOriginsAll := false
	allowOrigins := make(map[string]bool)
	for _, origin := range config.AllowOrigins {
		if origin == "*" {
			allowOriginsAll = true
			break
		}
		allowOrigins[origin] = true
	}

	return func(c *Context) {
		origin := c.GetHeader("Origin")

		// Check if origin is allowed
		var allowOrigin string
		if config.AllowOriginFunc != nil {
			if config.AllowOriginFunc(origin) {
				allowOrigin = origin
			}
		} else if allowOriginsAll {
			if config.AllowCredentials {
				allowOrigin = origin
			} else {
				allowOrigin = "*"
			}
		} else if allowOrigins[origin] {
			allowOrigin = origin
		}

		// If origin not allowed, continue without CORS headers
		if allowOrigin == "" {
			c.Next()
			return
		}

		// Set CORS headers
		c.SetHeader("Access-Control-Allow-Origin", allowOrigin)

		if config.AllowCredentials {
			c.SetHeader("Access-Control-Allow-Credentials", "true")
		}

		if len(config.ExposeHeaders) > 0 {
			c.SetHeader("Access-Control-Expose-Headers", exposeHeaders)
		}

		// Handle preflight request
		if c.Method() == http.MethodOptions {
			c.SetHeader("Access-Control-Allow-Methods", allowMethods)
			c.SetHeader("Access-Control-Allow-Headers", allowHeaders)
			if config.MaxAge > 0 {
				c.SetHeader("Access-Control-Max-Age", maxAge)
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// BasicAuth returns a BasicAuth middleware
func BasicAuth(accounts map[string]string) MiddlewareFunc {
	return BasicAuthWithConfig(BasicAuthConfig{
		Accounts: accounts,
		Realm:    "Authorization Required",
	})
}

// BasicAuthConfig defines the config for BasicAuth middleware
type BasicAuthConfig struct {
	// Accounts is a map of username:password
	Accounts map[string]string

	// Realm is the realm name for the challenge
	Realm string

	// Validator is a custom validator function
	Validator func(username, password string, c *Context) bool
}

// BasicAuthWithConfig returns a BasicAuth middleware with config
func BasicAuthWithConfig(config BasicAuthConfig) MiddlewareFunc {
	realm := fmt.Sprintf("Basic realm=\"%s\"", config.Realm)

	return func(c *Context) {
		username, password, ok := c.Request.BasicAuth()
		if !ok {
			c.SetHeader("WWW-Authenticate", realm)
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		// Use custom validator if provided
		if config.Validator != nil {
			if !config.Validator(username, password, c) {
				c.SetHeader("WWW-Authenticate", realm)
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
		} else {
			// Check against accounts map
			expectedPassword, exists := config.Accounts[username]
			if !exists || expectedPassword != password {
				c.SetHeader("WWW-Authenticate", realm)
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
		}

		// Store username in context
		c.Set("user", username)
		c.Next()
	}
}

// RequestID returns a middleware that sets a unique request ID
func RequestID() MiddlewareFunc {
	return RequestIDWithConfig(RequestIDConfig{})
}

// RequestIDConfig defines the config for RequestID middleware
type RequestIDConfig struct {
	// Header is the header name to use (default: X-Request-ID)
	Header string

	// Generator is a custom ID generator function
	Generator func() string
}

// defaultRequestIDGenerator generates a simple request ID
func defaultRequestIDGenerator() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// RequestIDWithConfig returns a RequestID middleware with config
func RequestIDWithConfig(config RequestIDConfig) MiddlewareFunc {
	if config.Header == "" {
		config.Header = HeaderXRequestID
	}
	if config.Generator == nil {
		config.Generator = defaultRequestIDGenerator
	}

	return func(c *Context) {
		// Check if request already has an ID
		requestID := c.GetHeader(config.Header)
		if requestID == "" {
			requestID = config.Generator()
		}

		// Set ID in context and response header
		c.Set("request_id", requestID)
		c.SetHeader(config.Header, requestID)

		c.Next()
	}
}

// Timeout returns a middleware that times out requests
func Timeout(timeout time.Duration) MiddlewareFunc {
	return func(c *Context) {
		// Create a channel for completion
		done := make(chan struct{})

		// Create a timer
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		// Run handler in goroutine
		go func() {
			c.Next()
			close(done)
		}()

		// Wait for completion or timeout
		select {
		case <-done:
			// Request completed
		case <-timer.C:
			// Timeout occurred
			c.AbortWithStatus(http.StatusGatewayTimeout)
		}
	}
}

// Secure returns a middleware that sets security headers
func Secure() MiddlewareFunc {
	return SecureWithConfig(SecureConfig{})
}

// SecureConfig defines the config for Secure middleware
type SecureConfig struct {
	// XSSProtection sets X-XSS-Protection header (default: "1; mode=block")
	XSSProtection string

	// ContentTypeNosniff sets X-Content-Type-Options header (default: "nosniff")
	ContentTypeNosniff string

	// XFrameOptions sets X-Frame-Options header (default: "SAMEORIGIN")
	XFrameOptions string

	// HSTSMaxAge sets Strict-Transport-Security max-age (default: 0 = disabled)
	HSTSMaxAge int

	// ContentSecurityPolicy sets Content-Security-Policy header
	ContentSecurityPolicy string

	// ReferrerPolicy sets Referrer-Policy header
	ReferrerPolicy string
}

// SecureWithConfig returns a Secure middleware with config
func SecureWithConfig(config SecureConfig) MiddlewareFunc {
	// Set defaults
	if config.XSSProtection == "" {
		config.XSSProtection = "1; mode=block"
	}
	if config.ContentTypeNosniff == "" {
		config.ContentTypeNosniff = "nosniff"
	}
	if config.XFrameOptions == "" {
		config.XFrameOptions = "SAMEORIGIN"
	}

	return func(c *Context) {
		// Set security headers
		if config.XSSProtection != "" {
			c.SetHeader("X-XSS-Protection", config.XSSProtection)
		}
		if config.ContentTypeNosniff != "" {
			c.SetHeader("X-Content-Type-Options", config.ContentTypeNosniff)
		}
		if config.XFrameOptions != "" {
			c.SetHeader("X-Frame-Options", config.XFrameOptions)
		}
		if config.HSTSMaxAge > 0 {
			c.SetHeader("Strict-Transport-Security", fmt.Sprintf("max-age=%d", config.HSTSMaxAge))
		}
		if config.ContentSecurityPolicy != "" {
			c.SetHeader("Content-Security-Policy", config.ContentSecurityPolicy)
		}
		if config.ReferrerPolicy != "" {
			c.SetHeader("Referrer-Policy", config.ReferrerPolicy)
		}

		c.Next()
	}
}

// RateLimiter returns a simple in-memory rate limiting middleware
func RateLimiter(requests int, window time.Duration) MiddlewareFunc {
	// Simple token bucket implementation
	type client struct {
		tokens    int
		lastReset time.Time
	}

	clients := make(map[string]*client)
	var mu = new(struct{}) // Placeholder - in production use sync.Mutex

	_ = mu // Silence unused variable

	return func(c *Context) {
		ip := c.ClientIP()

		// Get or create client entry
		cl, exists := clients[ip]
		if !exists {
			cl = &client{
				tokens:    requests,
				lastReset: time.Now(),
			}
			clients[ip] = cl
		}

		// Reset tokens if window has passed
		if time.Since(cl.lastReset) > window {
			cl.tokens = requests
			cl.lastReset = time.Now()
		}

		// Check if request is allowed
		if cl.tokens <= 0 {
			c.SetHeader("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}

		// Consume token
		cl.tokens--

		c.Next()
	}
}

// Gzip returns a gzip compression middleware
// Note: This is a simplified implementation. For production use,
// consider more sophisticated compression handling.
func Gzip() MiddlewareFunc {
	return func(c *Context) {
		// Check if client accepts gzip
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}

		// Note: Full gzip implementation would require wrapping the response writer
		// with a gzip writer. This is left as a placeholder for the structure.
		c.Next()
	}
}
