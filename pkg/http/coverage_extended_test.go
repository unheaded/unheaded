// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package http

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Response: XML, XMLPretty
// =============================================================================

func TestContextXML(t *testing.T) {
	type Item struct {
		XMLName xml.Name `xml:"item"`
		Name    string   `xml:"name"`
		Value   int      `xml:"value"`
	}

	r := NewRouter()
	r.GET("/xml", func(c *Context) {
		c.XML(http.StatusOK, Item{Name: "test", Value: 42})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/xml", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/xml") {
		t.Errorf("Content-Type = %q, want application/xml", ct)
	}
	if !strings.Contains(w.Body.String(), "<name>test</name>") {
		t.Errorf("body does not contain expected XML: %s", w.Body.String())
	}
}

func TestContextXMLPretty(t *testing.T) {
	type Item struct {
		XMLName xml.Name `xml:"item"`
		Name    string   `xml:"name"`
	}

	r := NewRouter()
	r.GET("/xml", func(c *Context) {
		c.XMLPretty(http.StatusOK, Item{Name: "pretty"}, "  ")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/xml", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	// Pretty output should contain newlines from indentation
	if !strings.Contains(w.Body.String(), "\n") {
		t.Error("expected pretty-printed XML with newlines")
	}
}

// =============================================================================
// Response: HTMLTemplate, HTMLTemplateFile
// =============================================================================

func TestContextHTMLTemplate(t *testing.T) {
	tmpl := template.Must(template.New("test").Parse("<h1>Hello, {{.Name}}</h1>"))

	r := NewRouter()
	r.GET("/tmpl", func(c *Context) {
		c.HTMLTemplate(http.StatusOK, tmpl, map[string]string{"Name": "World"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tmpl", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Hello, World") {
		t.Errorf("body = %q, want 'Hello, World'", w.Body.String())
	}
}

func TestContextHTMLTemplateFile(t *testing.T) {
	// Create a temporary template file
	dir := t.TempDir()
	tmplFile := filepath.Join(dir, "test.html")
	if err := os.WriteFile(tmplFile, []byte("<p>{{.Msg}}</p>"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewRouter()
	r.GET("/tmpl", func(c *Context) {
		c.HTMLTemplateFile(http.StatusOK, tmplFile, map[string]string{"Msg": "hi"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tmpl", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<p>hi</p>") {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestContextHTMLTemplateFile_NotFound(t *testing.T) {
	r := NewRouter()
	var gotErr error
	r.GET("/tmpl", func(c *Context) {
		gotErr = c.HTMLTemplateFile(http.StatusOK, "/nonexistent/file.html", nil)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tmpl", nil)
	r.ServeHTTP(w, req)

	if gotErr == nil {
		t.Error("expected error for nonexistent template file")
	}
}

// =============================================================================
// Response: Stream
// =============================================================================

func TestContextStream(t *testing.T) {
	r := NewRouter()
	r.GET("/stream", func(c *Context) {
		reader := strings.NewReader("streamed data")
		c.Stream(http.StatusOK, "text/plain", reader)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "streamed data" {
		t.Errorf("body = %q", w.Body.String())
	}
}

// =============================================================================
// Response: File, FileAttachment, FileInline, FileFromFS
// =============================================================================

func TestContextFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "test.txt")
	os.WriteFile(fpath, []byte("file content"), 0644)

	r := NewRouter()
	r.GET("/file", func(c *Context) {
		c.File(fpath)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/file", nil)
	r.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "file content") {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestContextFileAttachment(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "data.csv")
	os.WriteFile(fpath, []byte("a,b,c"), 0644)

	r := NewRouter()
	r.GET("/dl", func(c *Context) {
		c.FileAttachment(fpath, "download.csv")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dl", nil)
	r.ServeHTTP(w, req)

	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want attachment", cd)
	}
	if !strings.Contains(cd, "download.csv") {
		t.Errorf("Content-Disposition = %q, want download.csv", cd)
	}
}

func TestContextFileInline(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "image.png")
	os.WriteFile(fpath, []byte("PNG"), 0644)

	r := NewRouter()
	r.GET("/inline", func(c *Context) {
		c.FileInline(fpath, "image.png")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/inline", nil)
	r.ServeHTTP(w, req)

	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "inline") {
		t.Errorf("Content-Disposition = %q, want inline", cd)
	}
}

func TestContextFileFromFS(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello fs"), 0644)

	r := NewRouter()
	r.GET("/fs", func(c *Context) {
		err := c.FileFromFS("/readme.txt", http.Dir(dir))
		if err != nil {
			t.Errorf("FileFromFS error: %v", err)
		}
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fs", nil)
	r.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "hello fs") {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestContextFileFromFS_NotFound(t *testing.T) {
	r := NewRouter()
	var gotErr error
	r.GET("/fs", func(c *Context) {
		gotErr = c.FileFromFS("/nonexistent.txt", http.Dir(t.TempDir()))
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fs", nil)
	r.ServeHTTP(w, req)

	if gotErr == nil {
		t.Error("expected error for nonexistent file")
	}
}

// =============================================================================
// Response: Redirect helpers
// =============================================================================

func TestRedirectInvalidCode(t *testing.T) {
	r := NewRouter()
	var gotErr error
	r.GET("/redir", func(c *Context) {
		gotErr = c.Redirect(200, "/new")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/redir", nil)
	r.ServeHTTP(w, req)

	if gotErr == nil {
		t.Error("expected error for non-redirect status code")
	}
}

func TestRedirectPermanent(t *testing.T) {
	r := NewRouter()
	r.GET("/old", func(c *Context) {
		c.RedirectPermanent("/new")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/old", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", w.Code)
	}
}

func TestRedirectTemporary(t *testing.T) {
	r := NewRouter()
	r.GET("/old", func(c *Context) {
		c.RedirectTemporary("/new")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/old", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", w.Code)
	}
}

func TestRedirectSeeOther(t *testing.T) {
	r := NewRouter()
	r.GET("/old", func(c *Context) {
		c.RedirectSeeOther("/new")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/old", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", w.Code)
	}
}

// =============================================================================
// Response: Error helpers (JSONError, BadRequest, Unauthorized, Forbidden, ISE)
// =============================================================================

func TestContextJSONError(t *testing.T) {
	r := NewRouter()
	r.GET("/err", func(c *Context) {
		c.JSONError(http.StatusUnprocessableEntity, "validation failed")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message"] != "validation failed" {
		t.Errorf("message = %v", resp["message"])
	}
}

func TestContextBadRequest(t *testing.T) {
	r := NewRouter()
	r.GET("/bad", func(c *Context) {
		c.BadRequest("bad input")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestContextUnauthorized(t *testing.T) {
	r := NewRouter()
	r.GET("/unauth", func(c *Context) {
		c.Unauthorized("no token")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/unauth", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestContextForbidden(t *testing.T) {
	r := NewRouter()
	r.GET("/forbidden", func(c *Context) {
		c.Forbidden("access denied")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/forbidden", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestContextInternalServerError(t *testing.T) {
	r := NewRouter()
	r.GET("/ise", func(c *Context) {
		c.InternalServerError("something broke")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ise", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// =============================================================================
// Cookie helpers
// =============================================================================

func TestContextCookies(t *testing.T) {
	r := NewRouter()
	r.GET("/setcookie", func(c *Context) {
		c.SetCookieSimple("session", "abc123", 3600, "/", "", false, true)
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/setcookie", nil)
	r.ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected at least one cookie")
	}
	if cookies[0].Name != "session" || cookies[0].Value != "abc123" {
		t.Errorf("cookie = %v", cookies[0])
	}
}

func TestContextCookieValue(t *testing.T) {
	r := NewRouter()
	r.GET("/readcookie", func(c *Context) {
		val := c.CookieValue("token")
		c.String(http.StatusOK, "%s", val)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readcookie", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "xyz"})
	r.ServeHTTP(w, req)

	if w.Body.String() != "xyz" {
		t.Errorf("body = %q, want 'xyz'", w.Body.String())
	}
}

func TestContextCookieValue_Missing(t *testing.T) {
	r := NewRouter()
	r.GET("/readcookie2", func(c *Context) {
		val := c.CookieValue("nonexistent")
		c.String(http.StatusOK, "%s", val)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readcookie2", nil)
	r.ServeHTTP(w, req)

	if w.Body.String() != "" {
		t.Errorf("body = %q, want empty", w.Body.String())
	}
}

func TestContextCookie(t *testing.T) {
	r := NewRouter()
	var gotCookie *http.Cookie
	var gotErr error
	r.GET("/cookie", func(c *Context) {
		gotCookie, gotErr = c.Cookie("session")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/cookie", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	r.ServeHTTP(w, req)

	if gotErr != nil {
		t.Errorf("unexpected error: %v", gotErr)
	}
	if gotCookie.Value != "abc" {
		t.Errorf("cookie value = %q, want 'abc'", gotCookie.Value)
	}
}

func TestContextDeleteCookie(t *testing.T) {
	r := NewRouter()
	r.GET("/delcookie", func(c *Context) {
		c.DeleteCookie("session")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/delcookie", nil)
	r.ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie header for deletion")
	}
	if cookies[0].MaxAge != -1 {
		t.Errorf("MaxAge = %d, want -1", cookies[0].MaxAge)
	}
}

// =============================================================================
// ResponseWriter: Flush, Hijack, Push, duplicate WriteHeader, Write without header
// =============================================================================

func TestResponseWriter_DuplicateWriteHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewResponseWriter(w)

	rw.WriteHeader(http.StatusCreated)
	rw.WriteHeader(http.StatusNotFound) // should be ignored

	if rw.Status() != http.StatusCreated {
		t.Errorf("status = %d, want 201 (first call wins)", rw.Status())
	}
}

func TestResponseWriter_WriteImplicitHeader(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewResponseWriter(w)

	// Write without calling WriteHeader first
	rw.Write([]byte("hello"))

	if rw.Status() != http.StatusOK {
		t.Errorf("status = %d, want 200 (implicit)", rw.Status())
	}
	if !rw.Written() {
		t.Error("should be written")
	}
}

func TestResponseWriter_Flush(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewResponseWriter(w)

	// httptest.ResponseRecorder implements http.Flusher
	rw.Flush() // should not panic
}

func TestResponseWriter_Hijack_NotSupported(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewResponseWriter(w)

	_, _, err := rw.Hijack()
	if err == nil {
		t.Error("expected error since httptest.ResponseRecorder does not implement Hijacker")
	}
}

func TestResponseWriter_Push_NotSupported(t *testing.T) {
	w := httptest.NewRecorder()
	rw := NewResponseWriter(w)

	err := rw.Push("/resource", nil)
	if err == nil {
		t.Error("expected error since httptest.ResponseRecorder does not implement Pusher")
	}
}

// =============================================================================
// Middleware: RateLimiter
// =============================================================================

func TestRateLimiterMiddleware(t *testing.T) {
	r := NewRouter()
	r.Use(RateLimiter(2, 1*time.Minute))
	r.GET("/limited", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/limited", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i+1, w.Code)
		}
	}

	// Third request should be rate limited
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("request 3: status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

// =============================================================================
// Middleware: Gzip (placeholder)
// =============================================================================

func TestGzipMiddleware(t *testing.T) {
	r := NewRouter()
	r.Use(Gzip())
	r.GET("/gz", func(c *Context) {
		c.String(http.StatusOK, "data")
	})

	// Without gzip accept header
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gz", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// With gzip accept header
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/gz", nil)
	req2.Header.Set("Accept-Encoding", "gzip, deflate")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w2.Code)
	}
}

// =============================================================================
// Middleware: Timeout
// =============================================================================

func TestTimeoutMiddleware_Completes(t *testing.T) {
	r := NewRouter()
	r.Use(Timeout(1 * time.Second))
	r.GET("/fast", func(c *Context) {
		c.String(http.StatusOK, "fast")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fast", nil)
	r.ServeHTTP(w, req)

	// Handler completes before timeout — should get 200
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// =============================================================================
// Middleware: CORS advanced paths
// =============================================================================

func TestCORSWithConfig_SpecificOrigin(t *testing.T) {
	r := NewRouter()
	r.Use(CORSWithConfig(CORSConfig{
		AllowOrigins:     []string{"http://allowed.com"},
		AllowMethods:     []string{http.MethodGet},
		AllowHeaders:     []string{"Content-Type"},
		ExposeHeaders:    []string{"X-Custom"},
		AllowCredentials: true,
		MaxAge:           3600,
	}))
	r.GET("/test", func(c *Context) { c.String(http.StatusOK, "ok") })
	r.OPTIONS("/test", func(c *Context) {})

	// Allowed origin
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://allowed.com")
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "http://allowed.com" {
		t.Errorf("origin = %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("expected credentials header")
	}
	if w.Header().Get("Access-Control-Expose-Headers") != "X-Custom" {
		t.Error("expected expose headers")
	}

	// Disallowed origin
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("Origin", "http://evil.com")
	r.ServeHTTP(w2, req2)

	if w2.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("should not set CORS header for disallowed origin, got %q", w2.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSWithConfig_AllowOriginFunc(t *testing.T) {
	r := NewRouter()
	r.Use(CORSWithConfig(CORSConfig{
		AllowOriginFunc: func(origin string) bool {
			return strings.HasSuffix(origin, ".example.com")
		},
		AllowMethods: []string{http.MethodGet},
	}))
	r.GET("/test", func(c *Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://sub.example.com")
	r.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "http://sub.example.com" {
		t.Errorf("expected origin from func, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSWithConfig_AllowAllWithCredentials(t *testing.T) {
	r := NewRouter()
	r.Use(CORSWithConfig(CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{http.MethodGet},
		AllowCredentials: true,
	}))
	r.GET("/test", func(c *Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://any.com")
	r.ServeHTTP(w, req)

	// When AllowCredentials is true and origin is *, should echo the actual origin
	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://any.com" {
		t.Errorf("expected origin echo, got %q", origin)
	}
}

// =============================================================================
// Middleware: BasicAuth with custom Validator
// =============================================================================

func TestBasicAuthWithConfig_CustomValidator(t *testing.T) {
	r := NewRouter()
	r.Use(BasicAuthWithConfig(BasicAuthConfig{
		Realm: "Test",
		Validator: func(user, pass string, c *Context) bool {
			return user == "custom" && pass == "pass"
		},
	}))
	r.GET("/test", func(c *Context) { c.String(http.StatusOK, "ok") })

	// Valid custom auth
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.SetBasicAuth("custom", "pass")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Invalid custom auth
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.SetBasicAuth("custom", "wrong")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w2.Code)
	}
}

// =============================================================================
// Middleware: Recovery with custom handler
// =============================================================================

func TestRecoveryWithConfig_CustomHandler(t *testing.T) {
	r := NewRouter()
	r.Use(RecoveryWithConfig(RecoveryConfig{
		DisablePrintStack: true,
		Handler: func(c *Context, err any) {
			c.JSON(http.StatusInternalServerError, map[string]any{
				"error": fmt.Sprintf("%v", err),
			})
		},
	}))
	r.GET("/panic", func(c *Context) {
		panic("custom panic")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "custom panic" {
		t.Errorf("error = %v", resp["error"])
	}
}

// =============================================================================
// Middleware: Logger advanced paths
// =============================================================================

func TestLoggerWithConfig_SkipPaths(t *testing.T) {
	var logged []string
	r := NewRouter()
	r.Use(LoggerWithConfig(LoggerConfig{
		SkipPaths: []string{"/health"},
		Output: func(format string, args ...any) {
			logged = append(logged, fmt.Sprintf(format, args...))
		},
	}))
	r.GET("/health", func(c *Context) { c.String(http.StatusOK, "ok") })
	r.GET("/api", func(c *Context) { c.String(http.StatusOK, "ok") })

	// /health should be skipped
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)

	// /api should be logged
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api", nil)
	r.ServeHTTP(w2, req2)

	if len(logged) != 1 {
		t.Errorf("expected 1 log entry (skip /health), got %d", len(logged))
	}
}

func TestLoggerWithConfig_CustomFormatter(t *testing.T) {
	var output string
	r := NewRouter()
	r.Use(LoggerWithConfig(LoggerConfig{
		Output: func(format string, args ...any) {
			output = fmt.Sprintf(format, args...)
		},
		Formatter: func(p LogFormatterParams) string {
			return fmt.Sprintf("CUSTOM %s %s %d", p.Method, p.Path, p.StatusCode)
		},
	}))
	r.GET("/custom", func(c *Context) { c.String(http.StatusOK, "ok") })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/custom?q=1", nil)
	r.ServeHTTP(w, req)

	if !strings.HasPrefix(output, "CUSTOM GET /custom?q=1") {
		t.Errorf("output = %q", output)
	}
}

func TestDefaultLogFormatter_StatusColors(t *testing.T) {
	// Test different status code ranges for color coverage
	codes := []int{200, 301, 404, 500}
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, code := range codes {
		for _, method := range methods {
			result := defaultLogFormatter(LogFormatterParams{
				StatusCode: code,
				Method:     method,
				Path:       "/test",
				TimeStamp:  time.Now(),
				Latency:    100 * time.Millisecond,
				ClientIP:   "127.0.0.1",
			})
			if result == "" {
				t.Errorf("empty log for status=%d method=%s", code, method)
			}
		}
	}
}

// =============================================================================
// Server: Serve, Shutdown, GracefulShutdown, ForceShutdown, OnStart, OnShutdown, Running, EnableHTTP2
// =============================================================================

func TestServerServeAndShutdown(t *testing.T) {
	r := NewRouter()
	r.GET("/hello", func(c *Context) {
		c.String(http.StatusOK, "hello")
	})

	s := NewServer(r)

	// Create a listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	startCalled := false
	shutdownCalled := false
	s.OnStart(func() { startCalled = true })
	s.OnShutdown(func() { shutdownCalled = true })

	// Serve in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Serve(ln)
	}()

	// Wait for server to start
	time.Sleep(50 * time.Millisecond)

	if !s.Running() {
		t.Error("server should be running")
	}

	// Make a request
	resp, err := http.Get("http://" + ln.Addr().String() + "/hello")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "hello" {
		t.Errorf("body = %q", string(body))
	}

	// Graceful shutdown
	if err := s.GracefulShutdown(); err != nil {
		t.Errorf("shutdown error: %v", err)
	}

	// Wait for serve to return
	if err := <-errCh; err != nil {
		t.Errorf("serve error: %v", err)
	}

	if !startCalled {
		t.Error("OnStart callback not called")
	}
	if !shutdownCalled {
		t.Error("OnShutdown callback not called")
	}
	if s.Running() {
		t.Error("server should not be running after shutdown")
	}
}

func TestServerShutdownWhenNotRunning(t *testing.T) {
	r := NewRouter()
	s := NewServer(r)

	// Shutdown on a server that never started should be no-op
	err := s.Shutdown(nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestServerDoubleServe(t *testing.T) {
	// Test that calling serve() when already running returns an error.
	// We test the internal serve() method indirectly by manually setting
	// the running flag and calling serve(), avoiding the data race on
	// s.listener that would occur with two concurrent Serve() calls.
	r := NewRouter()
	s := NewServer(r)

	// Manually mark as running
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	// Now calling serve() should fail
	err := s.serve()
	if err == nil {
		t.Error("expected error for double serve")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("error = %q, want 'already running'", err.Error())
	}
}

func TestServerForceShutdown(t *testing.T) {
	r := NewRouter()
	s := NewServer(r)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Serve(ln)
	}()

	time.Sleep(50 * time.Millisecond)

	if err := s.ForceShutdown(); err != nil {
		t.Errorf("ForceShutdown error: %v", err)
	}
	<-errCh
}

func TestServerEnableHTTP2(t *testing.T) {
	r := NewRouter()
	s := NewServer(r)

	// Enable HTTP/2
	s.EnableHTTP2()

	// Check that h2 is in NextProtos
	if s.server.TLSConfig == nil {
		t.Fatal("TLSConfig should not be nil after EnableHTTP2")
	}
	found := false
	for _, p := range s.server.TLSConfig.NextProtos {
		if p == "h2" {
			found = true
		}
	}
	if !found {
		t.Error("h2 not in NextProtos")
	}

	// Enable again — should not duplicate
	s.EnableHTTP2()
	count := 0
	for _, p := range s.server.TLSConfig.NextProtos {
		if p == "h2" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("h2 appears %d times, want 1", count)
	}
}

func TestServerAddr_WithoutListener(t *testing.T) {
	r := NewRouter()
	config := ServerConfig{Addr: ":19999"}
	s := NewServerWithConfig(r, config)

	// Without a listener, Addr() falls back to s.server.Addr
	addr := s.Addr()
	if addr != ":19999" {
		t.Errorf("addr = %q, want ':19999'", addr)
	}
}

// =============================================================================
// Server: Health check readiness with checks
// =============================================================================

func TestHealthCheckReadiness_AllHealthy(t *testing.T) {
	r := NewRouter()
	r.RegisterHealthChecks(HealthCheckConfig{
		Checks: map[string]HealthCheck{
			"db":    func() error { return nil },
			"cache": func() error { return nil },
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ready" {
		t.Errorf("status = %v", resp["status"])
	}
}

func TestHealthCheckReadiness_SomeFailing(t *testing.T) {
	r := NewRouter()
	r.RegisterHealthChecks(HealthCheckConfig{
		Checks: map[string]HealthCheck{
			"db":    func() error { return nil },
			"cache": func() error { return fmt.Errorf("connection refused") },
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "not_ready" {
		t.Errorf("status = %v", resp["status"])
	}
}

func TestHealthCheckCustomPaths(t *testing.T) {
	r := NewRouter()
	r.RegisterHealthChecks(HealthCheckConfig{
		Path:          "/custom/health",
		LivenessPath:  "/custom/live",
		ReadinessPath: "/custom/ready",
	})

	paths := []string{"/custom/health", "/custom/live", "/custom/ready"}
	for _, p := range paths {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", p, w.Code)
		}
	}
}

// =============================================================================
// Server: Metrics endpoint, Record with error codes
// =============================================================================

func TestMetricsRecord_ErrorCount(t *testing.T) {
	m := NewMetrics()
	m.Record("/api", 200, 10*time.Millisecond, 100, 200)
	m.Record("/api", 500, 50*time.Millisecond, 50, 100)
	m.Record("/api", 404, 5*time.Millisecond, 10, 50)

	snap := m.Snapshot()
	if snap["request_count"].(int64) != 3 {
		t.Errorf("request_count = %v", snap["request_count"])
	}
	if snap["error_count"].(int64) != 2 {
		t.Errorf("error_count = %v (400+ and 500+ both count as errors)", snap["error_count"])
	}
}

func TestRegisterMetricsEndpoint(t *testing.T) {
	r := NewRouter()
	m := NewMetrics()
	m.Record("/test", 200, 10*time.Millisecond, 0, 100)

	r.RegisterMetricsEndpoint("", m) // default path /metrics

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["request_count"] == nil {
		t.Error("expected request_count in metrics response")
	}
}

func TestRegisterMetricsEndpoint_CustomPath(t *testing.T) {
	r := NewRouter()
	m := NewMetrics()
	r.RegisterMetricsEndpoint("/custom/metrics", m)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/custom/metrics", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// =============================================================================
// Router: Match, NotFound handler, MethodNotAllowed handler, trailing slash redirect
// =============================================================================

func TestRouterMatch(t *testing.T) {
	r := NewRouter()
	r.Match([]string{http.MethodGet, http.MethodPost}, "/multi", func(c *Context) {
		c.String(http.StatusOK, "%s", c.Method())
	})

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/multi", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s status = %d, want 200", method, w.Code)
		}
	}
}

func TestRouterCustomNotFoundHandler(t *testing.T) {
	r := NewRouter()
	r.NotFound(func(c *Context) {
		c.JSON(http.StatusNotFound, map[string]string{"error": "custom 404"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "custom 404" {
		t.Errorf("error = %v", resp["error"])
	}
}

func TestRouterCustomMethodNotAllowedHandler(t *testing.T) {
	r := NewRouter()
	r.MethodNotAllowed(func(c *Context) {
		c.JSON(http.StatusMethodNotAllowed, map[string]string{"error": "custom 405"})
	})
	r.GET("/resource", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestRouterTrailingSlashRedirect(t *testing.T) {
	r := NewRouter()
	r.RedirectTrailingSlash = true
	r.GET("/users", func(c *Context) {
		c.String(http.StatusOK, "users")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want 301", w.Code)
	}
	if w.Header().Get("Location") != "/users" {
		t.Errorf("Location = %q", w.Header().Get("Location"))
	}
}

// =============================================================================
// Server: NewServerWithConfig defaults
// =============================================================================

func TestNewServerWithConfig_Defaults(t *testing.T) {
	r := NewRouter()
	s := NewServerWithConfig(r, ServerConfig{})

	if s.shutdownTimeout != 30*time.Second {
		t.Errorf("shutdown timeout = %v, want 30s", s.shutdownTimeout)
	}
	if s.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestDefaultServerConfig(t *testing.T) {
	config := DefaultServerConfig()
	if config.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v", config.ReadTimeout)
	}
	if config.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v", config.ShutdownTimeout)
	}
	if config.MaxHeaderBytes != 1<<20 {
		t.Errorf("MaxHeaderBytes = %d", config.MaxHeaderBytes)
	}
}

// =============================================================================
// Group: Static, StaticFile
// =============================================================================

func TestRouteGroupStatic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello from static"), 0644)

	r := NewRouter()
	g := r.Group("/assets")
	g.Static("/files", dir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/files/hello.txt", nil)
	r.ServeHTTP(w, req)

	// The file should be served (may contain the content or trigger ServeFile)
	if w.Code == http.StatusNotFound {
		t.Error("expected file to be found")
	}
}

func TestRouteGroupStaticFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "index.html")
	os.WriteFile(fpath, []byte("<html>home</html>"), 0644)

	r := NewRouter()
	g := r.Group("/web")
	g.StaticFile("/home", fpath)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/web/home", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// =============================================================================
// Group: calculateAbsolutePath edge cases
// =============================================================================

func TestGroupCalculateAbsolutePath_EmptyRelative(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")

	path := g.calculateAbsolutePath("")
	if path != "/api" {
		t.Errorf("path = %q, want '/api'", path)
	}
}

func TestGroupCalculateAbsolutePath_NoLeadingSlash(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api")

	path := g.calculateAbsolutePath("users")
	if path != "/api/users" {
		t.Errorf("path = %q, want '/api/users'", path)
	}
}

// =============================================================================
// Metrics: Snapshot with zero requests
// =============================================================================

func TestMetricsSnapshot_ZeroRequests(t *testing.T) {
	m := NewMetrics()
	snap := m.Snapshot()

	if snap["avg_latency_ms"].(int64) != 0 {
		t.Errorf("avg_latency_ms = %v, want 0", snap["avg_latency_ms"])
	}
}

// =============================================================================
// Server: Running returns false initially
// =============================================================================

func TestServerRunning_Initial(t *testing.T) {
	r := NewRouter()
	s := NewServer(r)

	if s.Running() {
		t.Error("server should not be running initially")
	}
}
