// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the Business Source License 1.1 (BSL 1.1).
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

package http

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// validJSONPCallback matches safe JSONP callback names (alphanumeric, dots, underscores).
// SECURITY: Prevents XSS via malicious callback parameter injection.
var validJSONPCallback = regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z0-9_$.]*$`)

// Common MIME types
const (
	MIMEApplicationJSON            = "application/json"
	MIMEApplicationJSONCharsetUTF8 = "application/json; charset=utf-8"
	MIMEApplicationXML             = "application/xml"
	MIMEApplicationXMLCharsetUTF8  = "application/xml; charset=utf-8"
	MIMETextHTML                   = "text/html"
	MIMETextHTMLCharsetUTF8        = "text/html; charset=utf-8"
	MIMETextPlain                  = "text/plain"
	MIMETextPlainCharsetUTF8       = "text/plain; charset=utf-8"
	MIMEApplicationForm            = "application/x-www-form-urlencoded"
	MIMEMultipartForm              = "multipart/form-data"
	MIMEOctetStream                = "application/octet-stream"
)

// Common HTTP headers
const (
	HeaderContentType        = "Content-Type"
	HeaderContentLength      = "Content-Length"
	HeaderContentDisposition = "Content-Disposition"
	HeaderCacheControl       = "Cache-Control"
	HeaderLocation           = "Location"
	HeaderAuthorization      = "Authorization"
	HeaderAccept             = "Accept"
	HeaderAcceptEncoding     = "Accept-Encoding"
	HeaderContentEncoding    = "Content-Encoding"
	HeaderXRequestID         = "X-Request-ID"
	HeaderXRealIP            = "X-Real-IP"
	HeaderXForwardedFor      = "X-Forwarded-For"
	HeaderXForwardedProto    = "X-Forwarded-Proto"
)

// --- Response Status ---

// Status sets the response status code
func (c *Context) Status(code int) *Context {
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true
	return c
}

// Written returns whether the response has been written
func (c *Context) Written() bool {
	return c.written
}

// StatusCode returns the current response status code
func (c *Context) StatusCode() int {
	return c.statusCode
}

// --- Response Headers ---

// SetHeader sets a response header
func (c *Context) SetHeader(key, value string) *Context {
	c.Writer.Header().Set(key, value)
	return c
}

// AddHeader adds a response header (allows multiple values)
func (c *Context) AddHeader(key, value string) *Context {
	c.Writer.Header().Add(key, value)
	return c
}

// SetContentType sets the Content-Type header
func (c *Context) SetContentType(contentType string) *Context {
	return c.SetHeader(HeaderContentType, contentType)
}

// --- JSON Response ---

// JSON writes a JSON response
func (c *Context) JSON(code int, v any) error {
	c.SetContentType(MIMEApplicationJSONCharsetUTF8)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	encoder := json.NewEncoder(c.Writer)
	return encoder.Encode(v)
}

// JSONPretty writes a pretty-printed JSON response
func (c *Context) JSONPretty(code int, v any, indent string) error {
	c.SetContentType(MIMEApplicationJSONCharsetUTF8)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	encoder := json.NewEncoder(c.Writer)
	encoder.SetIndent("", indent)
	return encoder.Encode(v)
}

// JSONP writes a JSONP response
func (c *Context) JSONP(code int, callback string, v any) error {
	// SECURITY: Validate callback name to prevent XSS injection.
	// Only allow safe JavaScript identifier characters.
	if !validJSONPCallback.MatchString(callback) {
		c.statusCode = http.StatusBadRequest
		c.Writer.WriteHeader(http.StatusBadRequest)
		c.written = true
		_, err := c.Writer.Write([]byte(`{"error":"invalid callback parameter"}`))
		return err
	}

	c.SetContentType("application/javascript; charset=utf-8")
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	_, err = c.Writer.Write([]byte(callback + "("))
	if err != nil {
		return err
	}
	_, err = c.Writer.Write(data)
	if err != nil {
		return err
	}
	_, err = c.Writer.Write([]byte(");"))
	return err
}

// --- XML Response ---

// XML writes an XML response
func (c *Context) XML(code int, v any) error {
	c.SetContentType(MIMEApplicationXMLCharsetUTF8)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	encoder := xml.NewEncoder(c.Writer)
	return encoder.Encode(v)
}

// XMLPretty writes a pretty-printed XML response
func (c *Context) XMLPretty(code int, v any, indent string) error {
	c.SetContentType(MIMEApplicationXMLCharsetUTF8)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	encoder := xml.NewEncoder(c.Writer)
	encoder.Indent("", indent)
	return encoder.Encode(v)
}

// --- HTML Response ---

// HTML writes an HTML response
func (c *Context) HTML(code int, html string) error {
	c.SetContentType(MIMETextHTMLCharsetUTF8)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	_, err := c.Writer.Write([]byte(html))
	return err
}

// HTMLTemplate renders an HTML template
func (c *Context) HTMLTemplate(code int, tmpl *template.Template, data any) error {
	c.SetContentType(MIMETextHTMLCharsetUTF8)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	return tmpl.Execute(c.Writer, data)
}

// HTMLTemplateFile renders an HTML template from a file
func (c *Context) HTMLTemplateFile(code int, filename string, data any) error {
	tmpl, err := template.ParseFiles(filename)
	if err != nil {
		return err
	}
	return c.HTMLTemplate(code, tmpl, data)
}

// --- Text Response ---

// String writes a plain text response
func (c *Context) String(code int, format string, values ...any) error {
	c.SetContentType(MIMETextPlainCharsetUTF8)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	if len(values) > 0 {
		_, err := fmt.Fprintf(c.Writer, format, values...)
		return err
	}
	_, err := c.Writer.Write([]byte(format))
	return err
}

// --- Binary Response ---

// Data writes binary data to the response
func (c *Context) Data(code int, contentType string, data []byte) error {
	c.SetContentType(contentType)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	_, err := c.Writer.Write(data)
	return err
}

// Stream writes a stream to the response
func (c *Context) Stream(code int, contentType string, reader io.Reader) error {
	c.SetContentType(contentType)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	_, err := io.Copy(c.Writer, reader)
	return err
}

// --- File Response ---

// File sends a file as the response
func (c *Context) File(filepath string) error {
	http.ServeFile(c.Writer, c.Request, filepath)
	c.written = true
	return nil
}

// FileAttachment sends a file as an attachment (download)
func (c *Context) FileAttachment(filepath, filename string) error {
	c.SetHeader(HeaderContentDisposition, fmt.Sprintf("attachment; filename=\"%s\"", filename))
	return c.File(filepath)
}

// FileInline sends a file for inline display
func (c *Context) FileInline(filepath, filename string) error {
	c.SetHeader(HeaderContentDisposition, fmt.Sprintf("inline; filename=\"%s\"", filename))
	return c.File(filepath)
}

// FileFromFS serves a file from an http.FileSystem
func (c *Context) FileFromFS(filepath string, fs http.FileSystem) error {
	f, err := fs.Open(filepath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	http.ServeContent(c.Writer, c.Request, stat.Name(), stat.ModTime(), f)
	c.written = true
	return nil
}

// --- Redirect Response ---

// Redirect sends an HTTP redirect
func (c *Context) Redirect(code int, url string) error {
	if code < 300 || code > 308 {
		return fmt.Errorf("invalid redirect status code: %d", code)
	}
	c.SetHeader(HeaderLocation, url)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true
	return nil
}

// RedirectPermanent sends a 301 Moved Permanently redirect
func (c *Context) RedirectPermanent(url string) error {
	return c.Redirect(http.StatusMovedPermanently, url)
}

// RedirectTemporary sends a 302 Found redirect
func (c *Context) RedirectTemporary(url string) error {
	return c.Redirect(http.StatusFound, url)
}

// RedirectSeeOther sends a 303 See Other redirect
func (c *Context) RedirectSeeOther(url string) error {
	return c.Redirect(http.StatusSeeOther, url)
}

// --- Error Response ---

// Error sends an error response
func (c *Context) Error(code int, message string) error {
	c.SetContentType(MIMETextPlainCharsetUTF8)
	c.statusCode = code
	c.Writer.WriteHeader(code)
	c.written = true

	_, err := c.Writer.Write([]byte(message))
	return err
}

// JSONError sends a JSON error response
func (c *Context) JSONError(code int, message string) error {
	return c.JSON(code, map[string]any{
		"error":   http.StatusText(code),
		"message": message,
		"code":    code,
	})
}

// NoContent sends a 204 No Content response
func (c *Context) NoContent() error {
	c.statusCode = http.StatusNoContent
	c.Writer.WriteHeader(http.StatusNoContent)
	c.written = true
	return nil
}

// NotFound sends a 404 Not Found response
func (c *Context) NotFound() error {
	return c.Error(http.StatusNotFound, "Not Found")
}

// BadRequest sends a 400 Bad Request response
func (c *Context) BadRequest(message string) error {
	return c.Error(http.StatusBadRequest, message)
}

// Unauthorized sends a 401 Unauthorized response
func (c *Context) Unauthorized(message string) error {
	return c.Error(http.StatusUnauthorized, message)
}

// Forbidden sends a 403 Forbidden response
func (c *Context) Forbidden(message string) error {
	return c.Error(http.StatusForbidden, message)
}

// InternalServerError sends a 500 Internal Server Error response
func (c *Context) InternalServerError(message string) error {
	return c.Error(http.StatusInternalServerError, message)
}

// --- Cookie Helpers ---

// SetCookie sets a cookie
func (c *Context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.Writer, cookie)
}

// SetCookieSimple sets a simple cookie with name and value
func (c *Context) SetCookieSimple(name, value string, maxAge int, path, domain string, secure, httpOnly bool) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   maxAge,
		Path:     path,
		Domain:   domain,
		Secure:   secure,
		HttpOnly: httpOnly,
	}
	c.SetCookie(cookie)
}

// Cookie returns a cookie by name
func (c *Context) Cookie(name string) (*http.Cookie, error) {
	return c.Request.Cookie(name)
}

// CookieValue returns a cookie value by name
func (c *Context) CookieValue(name string) string {
	cookie, err := c.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// DeleteCookie deletes a cookie by name
func (c *Context) DeleteCookie(name string) {
	c.SetCookie(&http.Cookie{
		Name:   name,
		Value:  "",
		MaxAge: -1,
		Path:   "/",
	})
}

// --- Static File Serving ---

// StaticFile serves a single static file
func StaticFile(path, filepath string) HandlerFunc {
	return func(c *Context) {
		c.File(filepath)
	}
}

// Static returns a handler that serves static files from a directory
func Static(prefix, root string) HandlerFunc {
	// Ensure prefix has trailing slash for proper path matching
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// Create file server
	fs := http.FileServer(http.Dir(root))
	handler := http.StripPrefix(prefix, fs)

	return func(c *Context) {
		// Get the file path
		path := c.Request.URL.Path

		// Check if file exists
		fullPath := filepath.Join(root, strings.TrimPrefix(path, prefix))
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			c.NotFound()
			return
		}

		handler.ServeHTTP(c.Writer, c.Request)
		c.written = true
	}
}

// StaticFS returns a handler that serves static files from an http.FileSystem
func StaticFS(prefix string, fs http.FileSystem) HandlerFunc {
	// Ensure prefix has trailing slash
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	handler := http.StripPrefix(prefix, http.FileServer(fs))

	return func(c *Context) {
		handler.ServeHTTP(c.Writer, c.Request)
		c.written = true
	}
}

// --- Response Writer Wrapper ---

// ResponseWriter wraps http.ResponseWriter to capture status code and size
type ResponseWriter struct {
	http.ResponseWriter
	status      int
	size        int
	written     bool
	wroteHeader bool
}

// NewResponseWriter creates a new ResponseWriter
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

// WriteHeader implements http.ResponseWriter
func (w *ResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.status = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

// Write implements http.ResponseWriter
func (w *ResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.written = true
	n, err := w.ResponseWriter.Write(data)
	w.size += n
	return n, err
}

// Status returns the response status code
func (w *ResponseWriter) Status() int {
	return w.status
}

// Size returns the response body size
func (w *ResponseWriter) Size() int {
	return w.size
}

// Written returns whether any data has been written
func (w *ResponseWriter) Written() bool {
	return w.written
}

// Flush implements http.Flusher
func (w *ResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker
func (w *ResponseWriter) Hijack() (interface{}, interface{}, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("ResponseWriter does not implement http.Hijacker")
}

// Push implements http.Pusher
func (w *ResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return fmt.Errorf("ResponseWriter does not implement http.Pusher")
}
