// Package http provides a lightweight HTTP router for the Unheaded Kingdom.
// It uses only the Go standard library with no external dependencies.
package http

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// Context wraps the standard http.Request and http.ResponseWriter with
// additional functionality for path parameters, query parsing, and response helpers.
type Context struct {
	// Request is the underlying http.Request
	Request *http.Request

	// Writer is the underlying http.ResponseWriter
	Writer http.ResponseWriter

	// params holds path parameters extracted from the URL
	params map[string]string

	// query holds parsed query parameters (cached)
	query url.Values

	// keys holds user-defined key-value pairs for request-scoped data
	keys map[string]any

	// mu protects the keys map
	mu sync.RWMutex

	// index tracks the current middleware position
	index int

	// handlers is the chain of handlers to execute
	handlers []HandlerFunc

	// aborted indicates if the handler chain was aborted
	aborted bool

	// written tracks if response has been written
	written bool

	// statusCode is the response status code
	statusCode int
}

// contextPool reduces GC pressure by reusing Context objects
var contextPool = sync.Pool{
	New: func() any {
		return &Context{
			params: make(map[string]string),
			keys:   make(map[string]any),
			index:  -1,
		}
	},
}

// acquireContext gets a Context from the pool
func acquireContext(w http.ResponseWriter, r *http.Request) *Context {
	c := contextPool.Get().(*Context)
	c.Writer = w
	c.Request = r
	c.index = -1
	c.aborted = false
	c.written = false
	c.statusCode = http.StatusOK
	c.query = nil
	return c
}

// releaseContext returns a Context to the pool
func releaseContext(c *Context) {
	// Clear maps without reallocating
	for k := range c.params {
		delete(c.params, k)
	}
	for k := range c.keys {
		delete(c.keys, k)
	}
	c.handlers = nil
	c.Request = nil
	c.Writer = nil
	contextPool.Put(c)
}

// reset prepares the context for a new request
func (c *Context) reset(w http.ResponseWriter, r *http.Request) {
	c.Writer = w
	c.Request = r
	c.index = -1
	c.aborted = false
	c.written = false
	c.statusCode = http.StatusOK
	c.query = nil
	c.handlers = nil
	for k := range c.params {
		delete(c.params, k)
	}
	for k := range c.keys {
		delete(c.keys, k)
	}
}

// setParam sets a path parameter
func (c *Context) setParam(key, value string) {
	c.params[key] = value
}

// setHandlers sets the handler chain
func (c *Context) setHandlers(handlers []HandlerFunc) {
	c.handlers = handlers
}

// Next executes the next handler in the chain
func (c *Context) Next() {
	c.index++
	for c.index < len(c.handlers) && !c.aborted {
		c.handlers[c.index](c)
		c.index++
	}
}

// Abort stops the handler chain from continuing
func (c *Context) Abort() {
	c.aborted = true
}

// AbortWithStatus aborts with a specific status code
func (c *Context) AbortWithStatus(code int) {
	c.Status(code)
	c.Abort()
}

// AbortWithError aborts with a status code and error message
func (c *Context) AbortWithError(code int, err error) {
	c.Status(code)
	c.String(code, "%s", err.Error())
	c.Abort()
}

// IsAborted returns whether the handler chain was aborted
func (c *Context) IsAborted() bool {
	return c.aborted
}

// --- Path Parameters ---

// Param returns the value of a path parameter by name
func (c *Context) Param(name string) string {
	return c.params[name]
}

// ParamInt returns a path parameter as an integer
func (c *Context) ParamInt(name string) (int, error) {
	return strconv.Atoi(c.params[name])
}

// ParamInt64 returns a path parameter as an int64
func (c *Context) ParamInt64(name string) (int64, error) {
	return strconv.ParseInt(c.params[name], 10, 64)
}

// ParamDefault returns a path parameter or a default value if not found
func (c *Context) ParamDefault(name, defaultValue string) string {
	v, ok := c.params[name]
	if ok {
		return v
	}
	return defaultValue
}

// Params returns all path parameters as a map
func (c *Context) Params() map[string]string {
	result := make(map[string]string, len(c.params))
	for k, v := range c.params {
		result[k] = v
	}
	return result
}

// --- Query Parameters ---

// Query returns a query parameter by name
func (c *Context) Query(name string) string {
	return c.QueryValues().Get(name)
}

// QueryDefault returns a query parameter or a default value if not found
func (c *Context) QueryDefault(name, defaultValue string) string {
	if v := c.Query(name); v != "" {
		return v
	}
	return defaultValue
}

// QueryInt returns a query parameter as an integer
func (c *Context) QueryInt(name string) (int, error) {
	return strconv.Atoi(c.Query(name))
}

// QueryIntDefault returns a query parameter as an integer or a default
func (c *Context) QueryIntDefault(name string, defaultValue int) int {
	if v, err := c.QueryInt(name); err == nil {
		return v
	}
	return defaultValue
}

// QueryBool returns a query parameter as a boolean
func (c *Context) QueryBool(name string) (bool, error) {
	return strconv.ParseBool(c.Query(name))
}

// QueryBoolDefault returns a query parameter as a boolean or a default
func (c *Context) QueryBoolDefault(name string, defaultValue bool) bool {
	if v, err := c.QueryBool(name); err == nil {
		return v
	}
	return defaultValue
}

// QueryArray returns all values for a query parameter
func (c *Context) QueryArray(name string) []string {
	return c.QueryValues()[name]
}

// QueryValues returns the parsed query parameters
func (c *Context) QueryValues() url.Values {
	if c.query == nil {
		c.query = c.Request.URL.Query()
	}
	return c.query
}

// --- Request Body ---

// Body returns the request body as bytes
func (c *Context) Body() ([]byte, error) {
	return io.ReadAll(c.Request.Body)
}

// BindJSON binds the request body to a struct as JSON
func (c *Context) BindJSON(v any) error {
	decoder := json.NewDecoder(c.Request.Body)
	return decoder.Decode(v)
}

// --- Form Data ---

// FormValue returns a form value by name
func (c *Context) FormValue(name string) string {
	return c.Request.FormValue(name)
}

// FormFile returns a file from multipart form data
func (c *Context) FormFile(name string) (*multipart.FileHeader, error) {
	_, fh, err := c.Request.FormFile(name)
	return fh, err
}

// MultipartForm returns the parsed multipart form
func (c *Context) MultipartForm(maxMemory int64) (*multipart.Form, error) {
	if err := c.Request.ParseMultipartForm(maxMemory); err != nil {
		return nil, err
	}
	return c.Request.MultipartForm, nil
}

// --- Request Info ---

// Method returns the HTTP method
func (c *Context) Method() string {
	return c.Request.Method
}

// Path returns the request path
func (c *Context) Path() string {
	return c.Request.URL.Path
}

// FullPath returns the full request URL including query string
func (c *Context) FullPath() string {
	return c.Request.URL.String()
}

// Host returns the request host
func (c *Context) Host() string {
	return c.Request.Host
}

// RemoteAddr returns the client's remote address
func (c *Context) RemoteAddr() string {
	return c.Request.RemoteAddr
}

// ClientIP attempts to get the real client IP
func (c *Context) ClientIP() string {
	// Check X-Forwarded-For header
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs, take the first
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	addr := c.Request.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// UserAgent returns the User-Agent header
func (c *Context) UserAgent() string {
	return c.Request.UserAgent()
}

// ContentType returns the Content-Type header
func (c *Context) ContentType() string {
	return c.GetHeader("Content-Type")
}

// GetHeader returns a request header value
func (c *Context) GetHeader(key string) string {
	return c.Request.Header.Get(key)
}

// --- Context Values (Request-Scoped Storage) ---

// Set stores a key-value pair in the context
func (c *Context) Set(key string, value any) {
	c.mu.Lock()
	c.keys[key] = value
	c.mu.Unlock()
}

// Get retrieves a value from the context
func (c *Context) Get(key string) (any, bool) {
	c.mu.RLock()
	value, ok := c.keys[key]
	c.mu.RUnlock()
	return value, ok
}

// MustGet retrieves a value and panics if not found
func (c *Context) MustGet(key string) any {
	value, ok := c.Get(key)
	if !ok {
		panic("Key \"" + key + "\" does not exist")
	}
	return value
}

// GetString returns a string value from the context
func (c *Context) GetString(key string) string {
	if v, ok := c.Get(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt returns an int value from the context
func (c *Context) GetInt(key string) int {
	if v, ok := c.Get(key); ok {
		if i, ok := v.(int); ok {
			return i
		}
	}
	return 0
}

// GetInt64 returns an int64 value from the context
func (c *Context) GetInt64(key string) int64 {
	if v, ok := c.Get(key); ok {
		if i, ok := v.(int64); ok {
			return i
		}
	}
	return 0
}

// GetBool returns a bool value from the context
func (c *Context) GetBool(key string) bool {
	if v, ok := c.Get(key); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// --- Standard Context Interface ---

// Context returns the request's context.Context
func (c *Context) Context() context.Context {
	return c.Request.Context()
}

// WithContext returns a shallow copy with a new context.Context
func (c *Context) WithContext(ctx context.Context) *Context {
	c.Request = c.Request.WithContext(ctx)
	return c
}

// Deadline implements context.Context
func (c *Context) Deadline() (deadline any, ok bool) {
	return c.Request.Context().Deadline()
}

// Done implements context.Context
func (c *Context) Done() <-chan struct{} {
	return c.Request.Context().Done()
}

// Err implements context.Context
func (c *Context) Err() error {
	return c.Request.Context().Err()
}

// Value implements context.Context
func (c *Context) Value(key any) any {
	if k, ok := key.(string); ok {
		if v, exists := c.Get(k); exists {
			return v
		}
	}
	return c.Request.Context().Value(key)
}
