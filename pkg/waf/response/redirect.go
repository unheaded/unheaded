// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package response provides redirect response functionality
package response

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// RedirectHandler handles redirect responses
type RedirectHandler struct {
	defaultURL    string
	preserveQuery bool
	permanent     bool
	// allowedHosts gates the Host-header-derived redirects. See trustedHost.
	allowedHosts []string
}

// NewRedirectHandler creates a new redirect handler
func NewRedirectHandler(defaultURL string) *RedirectHandler {
	return &RedirectHandler{
		defaultURL:    defaultURL,
		preserveQuery: false,
		permanent:     false,
	}
}

// SetPreserveQuery sets whether to preserve query parameters
func (rh *RedirectHandler) SetPreserveQuery(preserve bool) {
	rh.preserveQuery = preserve
}

// SetPermanent sets whether to use permanent redirects (301)
func (rh *RedirectHandler) SetPermanent(permanent bool) {
	rh.permanent = permanent
}

// SetAllowedHosts declares the hostnames this handler may redirect to when it
// derives the target from the request's Host header.
func (rh *RedirectHandler) SetAllowedHosts(hosts []string) {
	rh.allowedHosts = hosts
}

// trustedHost reports whether r.Host may be used to build a redirect target.
//
// r.Host is the client-supplied Host header. RedirectToHTTPS/WWW/NonWWW used
// to interpolate it straight into a Location, so a request carrying
// `Host: evil.com` produced `301 Location: https://evil.com/...` — an open
// redirect, and a cache-poisoning primitive, served by the WAF itself.
//
// Fails CLOSED: with no allowlist configured, no Host is trusted and callers
// fall back to defaultURL. A redirect that goes somewhere useless is a bug; a
// redirect that goes wherever an attacker asks is a vulnerability.
func (rh *RedirectHandler) trustedHost(host string) bool {
	if host == "" {
		return false
	}
	// Strip any port before comparing.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	for _, allowed := range rh.allowedHosts {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// Redirect performs a redirect
func (rh *RedirectHandler) Redirect(w http.ResponseWriter, r *http.Request, targetURL string) {
	target := targetURL
	if target == "" {
		target = rh.defaultURL
	}

	if rh.preserveQuery && r.URL.RawQuery != "" {
		if strings.Contains(target, "?") {
			target += "&" + r.URL.RawQuery
		} else {
			target += "?" + r.URL.RawQuery
		}
	}

	code := http.StatusFound
	if rh.permanent {
		code = http.StatusMovedPermanently
	}

	http.Redirect(w, r, target, code)
}

// RedirectToLogin redirects to a login page
func (rh *RedirectHandler) RedirectToLogin(w http.ResponseWriter, r *http.Request, loginURL string) {
	target := loginURL

	// Add return URL
	returnURL := r.URL.String()
	if returnURL != "" {
		if strings.Contains(target, "?") {
			target += "&return=" + url.QueryEscape(returnURL)
		} else {
			target += "?return=" + url.QueryEscape(returnURL)
		}
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// RedirectToError redirects to an error page
func (rh *RedirectHandler) RedirectToError(w http.ResponseWriter, r *http.Request, errorURL string, code string) {
	target := errorURL

	if code != "" {
		if strings.Contains(target, "?") {
			target += "&code=" + url.QueryEscape(code)
		} else {
			target += "?code=" + url.QueryEscape(code)
		}
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// RedirectWithMessage redirects with a message parameter
func (rh *RedirectHandler) RedirectWithMessage(w http.ResponseWriter, r *http.Request, targetURL, message string) {
	target := targetURL

	if message != "" {
		if strings.Contains(target, "?") {
			target += "&message=" + url.QueryEscape(message)
		} else {
			target += "?message=" + url.QueryEscape(message)
		}
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// RedirectToHTTPS redirects HTTP to HTTPS
func (rh *RedirectHandler) RedirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	if !rh.trustedHost(r.Host) {
		http.Redirect(w, r, rh.defaultURL, http.StatusMovedPermanently)
		return
	}
	target := "https://" + r.Host + r.RequestURI
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// RedirectToWWW redirects to www subdomain
func (rh *RedirectHandler) RedirectToWWW(w http.ResponseWriter, r *http.Request) {
	if !rh.trustedHost(r.Host) {
		http.Redirect(w, r, rh.defaultURL, http.StatusMovedPermanently)
		return
	}
	host := r.Host
	if !strings.HasPrefix(host, "www.") {
		host = "www." + host
	}

	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}

	target := scheme + "://" + host + r.RequestURI
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// RedirectToNonWWW redirects to non-www
func (rh *RedirectHandler) RedirectToNonWWW(w http.ResponseWriter, r *http.Request) {
	if !rh.trustedHost(r.Host) {
		http.Redirect(w, r, rh.defaultURL, http.StatusMovedPermanently)
		return
	}
	host := strings.TrimPrefix(r.Host, "www.")

	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}

	target := scheme + "://" + host + r.RequestURI
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// SafeRedirect performs a redirect after validating the target URL
func (rh *RedirectHandler) SafeRedirect(w http.ResponseWriter, r *http.Request, targetURL string, allowedHosts []string) bool {
	// Parse the target URL
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	// Check if the host is allowed
	if parsed.Host != "" {
		allowed := false
		for _, host := range allowedHosts {
			if parsed.Host == host || strings.HasSuffix(parsed.Host, "."+host) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	http.Redirect(w, r, targetURL, http.StatusFound)
	return true
}
