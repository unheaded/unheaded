// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package response

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewRedirectHandler
// ---------------------------------------------------------------------------

func TestNewRedirectHandler(t *testing.T) {
	rh := NewRedirectHandler("https://example.com/default")
	if rh == nil {
		t.Fatal("NewRedirectHandler returned nil")
	}
	if rh.defaultURL != "https://example.com/default" {
		t.Errorf("defaultURL = %q", rh.defaultURL)
	}
	if rh.preserveQuery {
		t.Error("preserveQuery should default to false")
	}
	if rh.permanent {
		t.Error("permanent should default to false")
	}
}

// ---------------------------------------------------------------------------
// Redirect
// ---------------------------------------------------------------------------

func TestRedirectHandler_Redirect_WithTargetURL(t *testing.T) {
	rh := NewRedirectHandler("https://example.com/default")
	req := httptest.NewRequest("GET", "http://example.com/old", nil)
	w := httptest.NewRecorder()

	rh.Redirect(w, req, "https://example.com/new")

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusFound)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com/new" {
		t.Errorf("Location = %q", loc)
	}
}

func TestRedirectHandler_Redirect_FallbackToDefault(t *testing.T) {
	rh := NewRedirectHandler("https://example.com/default")
	req := httptest.NewRequest("GET", "http://example.com/old", nil)
	w := httptest.NewRecorder()

	rh.Redirect(w, req, "")

	if loc := w.Header().Get("Location"); loc != "https://example.com/default" {
		t.Errorf("Location = %q", loc)
	}
}

func TestRedirectHandler_Redirect_Permanent(t *testing.T) {
	rh := NewRedirectHandler("https://example.com/default")
	rh.SetPermanent(true)
	req := httptest.NewRequest("GET", "http://example.com/old", nil)
	w := httptest.NewRecorder()

	rh.Redirect(w, req, "https://example.com/new")

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMovedPermanently)
	}
}

func TestRedirectHandler_Redirect_PreserveQuery(t *testing.T) {
	rh := NewRedirectHandler("https://example.com/default")
	rh.SetPreserveQuery(true)

	req := httptest.NewRequest("GET", "http://example.com/old?foo=bar&baz=1", nil)
	w := httptest.NewRecorder()

	rh.Redirect(w, req, "https://example.com/new")

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "?foo=bar&baz=1") {
		t.Errorf("Location = %q, should contain query params", loc)
	}
}

func TestRedirectHandler_Redirect_PreserveQuery_TargetHasQuery(t *testing.T) {
	rh := NewRedirectHandler("")
	rh.SetPreserveQuery(true)

	req := httptest.NewRequest("GET", "http://example.com/old?extra=1", nil)
	w := httptest.NewRecorder()

	rh.Redirect(w, req, "https://example.com/new?existing=2")

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "existing=2") {
		t.Errorf("Location = %q, should preserve existing query", loc)
	}
	if !strings.Contains(loc, "extra=1") {
		t.Errorf("Location = %q, should append original query", loc)
	}
	if !strings.Contains(loc, "&") {
		t.Errorf("Location = %q, should join with &", loc)
	}
}

func TestRedirectHandler_Redirect_PreserveQuery_EmptyOriginalQuery(t *testing.T) {
	rh := NewRedirectHandler("")
	rh.SetPreserveQuery(true)

	req := httptest.NewRequest("GET", "http://example.com/old", nil)
	w := httptest.NewRecorder()

	rh.Redirect(w, req, "https://example.com/new")

	loc := w.Header().Get("Location")
	if strings.Contains(loc, "?") {
		t.Errorf("Location = %q, should not have query when original has none", loc)
	}
}

// ---------------------------------------------------------------------------
// RedirectToLogin
// ---------------------------------------------------------------------------

func TestRedirectHandler_RedirectToLogin(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/protected/page", nil)
	w := httptest.NewRecorder()

	rh.RedirectToLogin(w, req, "https://example.com/login")

	if w.Code != http.StatusFound {
		t.Errorf("status = %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://example.com/login") {
		t.Errorf("Location = %q, should start with login URL", loc)
	}
	if !strings.Contains(loc, "return=") {
		t.Errorf("Location = %q, should contain return param", loc)
	}
}

func TestRedirectHandler_RedirectToLogin_LoginURLWithQuery(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/protected", nil)
	w := httptest.NewRecorder()

	rh.RedirectToLogin(w, req, "https://example.com/login?realm=admin")

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "realm=admin") {
		t.Errorf("Location = %q, should preserve login query params", loc)
	}
	if !strings.Contains(loc, "&return=") {
		t.Errorf("Location = %q, should append return with &", loc)
	}
}

// ---------------------------------------------------------------------------
// RedirectToError
// ---------------------------------------------------------------------------

func TestRedirectHandler_RedirectToError(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	rh.RedirectToError(w, req, "https://example.com/error", "403")

	if w.Code != http.StatusFound {
		t.Errorf("status = %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "code=403") {
		t.Errorf("Location = %q, should contain code=403", loc)
	}
}

func TestRedirectHandler_RedirectToError_EmptyCode(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	rh.RedirectToError(w, req, "https://example.com/error", "")

	loc := w.Header().Get("Location")
	if loc != "https://example.com/error" {
		t.Errorf("Location = %q, should be plain error URL when code empty", loc)
	}
}

func TestRedirectHandler_RedirectToError_ErrorURLWithQuery(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	rh.RedirectToError(w, req, "https://example.com/error?type=waf", "403")

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "type=waf") {
		t.Errorf("Location = %q, should preserve existing query", loc)
	}
	if !strings.Contains(loc, "&code=403") {
		t.Errorf("Location = %q, should append code with &", loc)
	}
}

// ---------------------------------------------------------------------------
// RedirectWithMessage
// ---------------------------------------------------------------------------

func TestRedirectHandler_RedirectWithMessage(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	rh.RedirectWithMessage(w, req, "https://example.com/info", "Access denied")

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "message=Access") {
		t.Errorf("Location = %q, should contain message param", loc)
	}
}

func TestRedirectHandler_RedirectWithMessage_EmptyMessage(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	rh.RedirectWithMessage(w, req, "https://example.com/info", "")

	loc := w.Header().Get("Location")
	if loc != "https://example.com/info" {
		t.Errorf("Location = %q, should be plain URL when message empty", loc)
	}
}

func TestRedirectHandler_RedirectWithMessage_TargetWithQuery(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	rh.RedirectWithMessage(w, req, "https://example.com/info?source=waf", "blocked")

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "&message=blocked") {
		t.Errorf("Location = %q, should append message with &", loc)
	}
}

// ---------------------------------------------------------------------------
// RedirectToHTTPS
// ---------------------------------------------------------------------------

func TestRedirectHandler_RedirectToHTTPS(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/path?q=1", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	rh.RedirectToHTTPS(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMovedPermanently)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://example.com") {
		t.Errorf("Location = %q, should start with https://", loc)
	}
}

// ---------------------------------------------------------------------------
// RedirectToWWW
// ---------------------------------------------------------------------------

func TestRedirectHandler_RedirectToWWW_NoWWW(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/path", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	rh.RedirectToWWW(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "www.example.com") {
		t.Errorf("Location = %q, should contain www.", loc)
	}
}

func TestRedirectHandler_RedirectToWWW_AlreadyWWW(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://www.example.com/path", nil)
	req.Host = "www.example.com"
	w := httptest.NewRecorder()

	rh.RedirectToWWW(w, req)

	loc := w.Header().Get("Location")
	// Should not double www
	if strings.Contains(loc, "www.www.") {
		t.Errorf("Location = %q, should not double www", loc)
	}
}

func TestRedirectHandler_RedirectToWWW_HTTPS(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "https://example.com/path", nil)
	req.Host = "example.com"
	req.TLS = &tls.ConnectionState{} // Mark as TLS
	w := httptest.NewRecorder()

	rh.RedirectToWWW(w, req)

	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://www.example.com") {
		t.Errorf("Location = %q, should use https scheme", loc)
	}
}

// ---------------------------------------------------------------------------
// RedirectToNonWWW
// ---------------------------------------------------------------------------

func TestRedirectHandler_RedirectToNonWWW(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "/path", nil)
	req.Host = "www.example.com"
	req.RequestURI = "/path"
	w := httptest.NewRecorder()

	rh.RedirectToNonWWW(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if strings.Contains(loc, "www.") {
		t.Errorf("Location = %q, should not contain www.", loc)
	}
	if !strings.Contains(loc, "example.com") {
		t.Errorf("Location = %q, should contain domain", loc)
	}
}

func TestRedirectHandler_RedirectToNonWWW_AlreadyNonWWW(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/path", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()

	rh.RedirectToNonWWW(w, req)

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "example.com") {
		t.Errorf("Location = %q", loc)
	}
}

func TestRedirectHandler_RedirectToNonWWW_HTTPS(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "https://www.example.com/path", nil)
	req.Host = "www.example.com"
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()

	rh.RedirectToNonWWW(w, req)

	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://example.com") {
		t.Errorf("Location = %q, should use https", loc)
	}
}

// ---------------------------------------------------------------------------
// SafeRedirect
// ---------------------------------------------------------------------------

func TestRedirectHandler_SafeRedirect_AllowedHost(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	ok := rh.SafeRedirect(w, req, "https://example.com/safe", []string{"example.com"})
	if !ok {
		t.Error("should return true for allowed host")
	}
	if w.Code != http.StatusFound {
		t.Errorf("status = %d", w.Code)
	}
}

func TestRedirectHandler_SafeRedirect_SubdomainAllowed(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	ok := rh.SafeRedirect(w, req, "https://sub.example.com/safe", []string{"example.com"})
	if !ok {
		t.Error("should return true for allowed subdomain")
	}
}

func TestRedirectHandler_SafeRedirect_DisallowedHost(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	ok := rh.SafeRedirect(w, req, "https://evil.com/phish", []string{"example.com"})
	if ok {
		t.Error("should return false for disallowed host")
	}
}

func TestRedirectHandler_SafeRedirect_RelativePath(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	ok := rh.SafeRedirect(w, req, "/local/path", []string{"example.com"})
	if !ok {
		t.Error("should return true for relative path (no host)")
	}
}

func TestRedirectHandler_SafeRedirect_InvalidURL(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	ok := rh.SafeRedirect(w, req, "://invalid", []string{"example.com"})
	if ok {
		t.Error("should return false for invalid URL")
	}
}

func TestRedirectHandler_SafeRedirect_EmptyAllowedHosts(t *testing.T) {
	rh := NewRedirectHandler("")
	req := httptest.NewRequest("GET", "http://example.com/page", nil)
	w := httptest.NewRecorder()

	ok := rh.SafeRedirect(w, req, "https://evil.com/phish", []string{})
	if ok {
		t.Error("should return false when allowed hosts is empty and URL has host")
	}
}
