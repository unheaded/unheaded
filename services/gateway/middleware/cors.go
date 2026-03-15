// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"unheaded/services/gateway/config"
)

// CORSMiddleware handles Cross-Origin Resource Sharing.
type CORSMiddleware struct {
	cfg            *config.CORSConfig
	allowedOrigins map[string]bool
	allowAll       bool
}

// NewCORSMiddleware creates a new CORS middleware.
func NewCORSMiddleware(cfg *config.CORSConfig) *CORSMiddleware {
	m := &CORSMiddleware{
		cfg:            cfg,
		allowedOrigins: make(map[string]bool),
	}

	for _, origin := range cfg.AllowedOrigins {
		if origin == "*" {
			m.allowAll = true
		}
		m.allowedOrigins[origin] = true
	}

	return m
}

// Handler returns the middleware handler.
func (m *CORSMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.cfg.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "" {
			// Not a CORS request
			next.ServeHTTP(w, r)
			return
		}

		// Check if origin is allowed
		if !m.isOriginAllowed(origin) {
			// Origin not allowed, but still process the request
			next.ServeHTTP(w, r)
			return
		}

		// Set CORS headers
		if m.allowAll && !m.cfg.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		if m.cfg.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if len(m.cfg.ExposedHeaders) > 0 {
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(m.cfg.ExposedHeaders, ", "))
		}

		// Handle preflight request
		if r.Method == http.MethodOptions {
			m.handlePreflight(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isOriginAllowed checks if the origin is in the allowed list.
func (m *CORSMiddleware) isOriginAllowed(origin string) bool {
	if m.allowAll {
		return true
	}
	return m.allowedOrigins[origin]
}

// handlePreflight handles CORS preflight requests.
func (m *CORSMiddleware) handlePreflight(w http.ResponseWriter, r *http.Request) {
	// Check Access-Control-Request-Method
	reqMethod := r.Header.Get("Access-Control-Request-Method")
	if reqMethod == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !m.isMethodAllowed(reqMethod) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Set allowed methods
	w.Header().Set("Access-Control-Allow-Methods", strings.Join(m.cfg.AllowedMethods, ", "))

	// Check Access-Control-Request-Headers
	reqHeaders := r.Header.Get("Access-Control-Request-Headers")
	if reqHeaders != "" {
		headers := strings.Split(reqHeaders, ",")
		for _, h := range headers {
			h = strings.TrimSpace(h)
			if !m.isHeaderAllowed(h) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
		}
	}

	// Set allowed headers
	if len(m.cfg.AllowedHeaders) > 0 {
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(m.cfg.AllowedHeaders, ", "))
	}

	// Set max age
	if m.cfg.MaxAge > 0 {
		w.Header().Set("Access-Control-Max-Age", strconv.Itoa(m.cfg.MaxAge))
	}

	w.WriteHeader(http.StatusNoContent)
}

// isMethodAllowed checks if a method is in the allowed list.
func (m *CORSMiddleware) isMethodAllowed(method string) bool {
	for _, allowed := range m.cfg.AllowedMethods {
		if strings.EqualFold(allowed, method) {
			return true
		}
	}
	return false
}

// isHeaderAllowed checks if a header is in the allowed list.
func (m *CORSMiddleware) isHeaderAllowed(header string) bool {
	// Simple headers are always allowed
	simpleHeaders := []string{
		"accept",
		"accept-language",
		"content-language",
		"content-type",
	}

	headerLower := strings.ToLower(header)
	for _, sh := range simpleHeaders {
		if sh == headerLower {
			return true
		}
	}

	for _, allowed := range m.cfg.AllowedHeaders {
		if strings.EqualFold(allowed, header) {
			return true
		}
	}
	return false
}
