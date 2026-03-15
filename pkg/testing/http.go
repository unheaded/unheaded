// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package testing

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// RequestBuilder provides a fluent API for building HTTP test requests.
type RequestBuilder struct {
	method      string
	url         string
	headers     http.Header
	body        io.Reader
	queryParams url.Values
	cookies     []*http.Cookie
	basicAuth   *struct{ user, pass string }
}

// NewRequest creates a new request builder.
func NewRequest(method, urlStr string) *RequestBuilder {
	return &RequestBuilder{
		method:      method,
		url:         urlStr,
		headers:     make(http.Header),
		queryParams: make(url.Values),
	}
}

// Get creates a GET request builder.
func Get(url string) *RequestBuilder {
	return NewRequest(http.MethodGet, url)
}

// Post creates a POST request builder.
func Post(url string) *RequestBuilder {
	return NewRequest(http.MethodPost, url)
}

// Put creates a PUT request builder.
func Put(url string) *RequestBuilder {
	return NewRequest(http.MethodPut, url)
}

// Patch creates a PATCH request builder.
func Patch(url string) *RequestBuilder {
	return NewRequest(http.MethodPatch, url)
}

// Delete creates a DELETE request builder.
func Delete(url string) *RequestBuilder {
	return NewRequest(http.MethodDelete, url)
}

// WithHeader adds a header to the request.
func (r *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	r.headers.Set(key, value)
	return r
}

// WithHeaders adds multiple headers to the request.
func (r *RequestBuilder) WithHeaders(headers map[string]string) *RequestBuilder {
	for k, v := range headers {
		r.headers.Set(k, v)
	}
	return r
}

// WithBody sets the request body.
func (r *RequestBuilder) WithBody(body io.Reader) *RequestBuilder {
	r.body = body
	return r
}

// WithBodyString sets the request body from a string.
func (r *RequestBuilder) WithBodyString(body string) *RequestBuilder {
	r.body = strings.NewReader(body)
	return r
}

// WithBodyBytes sets the request body from bytes.
func (r *RequestBuilder) WithBodyBytes(body []byte) *RequestBuilder {
	r.body = bytes.NewReader(body)
	return r
}

// WithJSON sets the request body as JSON.
func (r *RequestBuilder) WithJSON(v interface{}) *RequestBuilder {
	data, err := json.Marshal(v)
	if err != nil {
		// Store error for later
		r.body = &errorReader{err: err}
		return r
	}
	r.body = bytes.NewReader(data)
	r.headers.Set("Content-Type", "application/json")
	return r
}

// WithForm sets the request body as form data.
func (r *RequestBuilder) WithForm(values url.Values) *RequestBuilder {
	r.body = strings.NewReader(values.Encode())
	r.headers.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

// WithQuery adds a query parameter.
func (r *RequestBuilder) WithQuery(key, value string) *RequestBuilder {
	r.queryParams.Set(key, value)
	return r
}

// WithQueryParams adds multiple query parameters.
func (r *RequestBuilder) WithQueryParams(params map[string]string) *RequestBuilder {
	for k, v := range params {
		r.queryParams.Set(k, v)
	}
	return r
}

// WithCookie adds a cookie to the request.
func (r *RequestBuilder) WithCookie(cookie *http.Cookie) *RequestBuilder {
	r.cookies = append(r.cookies, cookie)
	return r
}

// WithCookieSimple adds a simple cookie to the request.
func (r *RequestBuilder) WithCookieSimple(name, value string) *RequestBuilder {
	return r.WithCookie(&http.Cookie{Name: name, Value: value})
}

// WithBasicAuth sets basic authentication.
func (r *RequestBuilder) WithBasicAuth(user, pass string) *RequestBuilder {
	r.basicAuth = &struct{ user, pass string }{user, pass}
	return r
}

// WithBearer sets a Bearer token.
func (r *RequestBuilder) WithBearer(token string) *RequestBuilder {
	return r.WithHeader("Authorization", "Bearer "+token)
}

// Build creates the http.Request.
func (r *RequestBuilder) Build() (*http.Request, error) {
	// Build URL with query params
	reqURL := r.url
	if len(r.queryParams) > 0 {
		if strings.Contains(reqURL, "?") {
			reqURL += "&" + r.queryParams.Encode()
		} else {
			reqURL += "?" + r.queryParams.Encode()
		}
	}

	req, err := http.NewRequest(r.method, reqURL, r.body)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header = r.headers

	// Set cookies
	for _, c := range r.cookies {
		req.AddCookie(c)
	}

	// Set basic auth
	if r.basicAuth != nil {
		req.SetBasicAuth(r.basicAuth.user, r.basicAuth.pass)
	}

	return req, nil
}

// Request builds and returns the http.Request, panicking on error.
func (r *RequestBuilder) Request() *http.Request {
	req, err := r.Build()
	if err != nil {
		panic(err)
	}
	return req
}

// errorReader is an io.Reader that always returns an error.
type errorReader struct {
	err error
}

func (e *errorReader) Read(p []byte) (int, error) {
	return 0, e.err
}

// NewRecorder creates a new response recorder.
func NewRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

// AssertStatus asserts that the response has the expected status code.
func AssertStatus(t *testing.T, rec *httptest.ResponseRecorder, expected int) bool {
	t.Helper()

	if rec.Code != expected {
		t.Errorf("Expected status %d, got %d", expected, rec.Code)
		return false
	}
	return true
}

// AssertStatusOK asserts that the response has status 200.
func AssertStatusOK(t *testing.T, rec *httptest.ResponseRecorder) bool {
	return AssertStatus(t, rec, http.StatusOK)
}

// AssertStatusCreated asserts that the response has status 201.
func AssertStatusCreated(t *testing.T, rec *httptest.ResponseRecorder) bool {
	return AssertStatus(t, rec, http.StatusCreated)
}

// AssertStatusNoContent asserts that the response has status 204.
func AssertStatusNoContent(t *testing.T, rec *httptest.ResponseRecorder) bool {
	return AssertStatus(t, rec, http.StatusNoContent)
}

// AssertStatusBadRequest asserts that the response has status 400.
func AssertStatusBadRequest(t *testing.T, rec *httptest.ResponseRecorder) bool {
	return AssertStatus(t, rec, http.StatusBadRequest)
}

// AssertStatusUnauthorized asserts that the response has status 401.
func AssertStatusUnauthorized(t *testing.T, rec *httptest.ResponseRecorder) bool {
	return AssertStatus(t, rec, http.StatusUnauthorized)
}

// AssertStatusForbidden asserts that the response has status 403.
func AssertStatusForbidden(t *testing.T, rec *httptest.ResponseRecorder) bool {
	return AssertStatus(t, rec, http.StatusForbidden)
}

// AssertStatusNotFound asserts that the response has status 404.
func AssertStatusNotFound(t *testing.T, rec *httptest.ResponseRecorder) bool {
	return AssertStatus(t, rec, http.StatusNotFound)
}

// AssertStatusInternalServerError asserts that the response has status 500.
func AssertStatusInternalServerError(t *testing.T, rec *httptest.ResponseRecorder) bool {
	return AssertStatus(t, rec, http.StatusInternalServerError)
}

// AssertHeader asserts that the response has the expected header value.
func AssertHeader(t *testing.T, rec *httptest.ResponseRecorder, key, expected string) bool {
	t.Helper()

	actual := rec.Header().Get(key)
	if actual != expected {
		t.Errorf("Expected header %s to be %q, got %q", key, expected, actual)
		return false
	}
	return true
}

// AssertHeaderContains asserts that the response header contains the expected value.
func AssertHeaderContains(t *testing.T, rec *httptest.ResponseRecorder, key, substr string) bool {
	t.Helper()

	actual := rec.Header().Get(key)
	if !strings.Contains(actual, substr) {
		t.Errorf("Expected header %s to contain %q, got %q", key, substr, actual)
		return false
	}
	return true
}

// AssertContentType asserts that the response has the expected content type.
func AssertContentType(t *testing.T, rec *httptest.ResponseRecorder, expected string) bool {
	return AssertHeader(t, rec, "Content-Type", expected)
}

// AssertJSON asserts that the response body equals the expected JSON.
func AssertJSON(t *testing.T, rec *httptest.ResponseRecorder, expected interface{}) bool {
	t.Helper()

	// Marshal expected to JSON
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		t.Errorf("Failed to marshal expected value: %v", err)
		return false
	}

	// Compare
	actualBody := rec.Body.Bytes()

	// Normalize both for comparison
	var expectedMap, actualMap interface{}
	if err := json.Unmarshal(expectedJSON, &expectedMap); err != nil {
		t.Errorf("Failed to unmarshal expected JSON: %v", err)
		return false
	}
	if err := json.Unmarshal(actualBody, &actualMap); err != nil {
		t.Errorf("Failed to unmarshal actual JSON: %v", err)
		return false
	}

	if !deepEqualJSON(expectedMap, actualMap) {
		t.Errorf("JSON mismatch:\nExpected: %s\nActual: %s", expectedJSON, actualBody)
		return false
	}

	return true
}

// AssertJSONPath asserts that a JSON path has the expected value.
func AssertJSONPath(t *testing.T, rec *httptest.ResponseRecorder, path string, expected interface{}) bool {
	t.Helper()

	var data map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
		return false
	}

	actual := getJSONPath(data, path)
	if actual != expected {
		t.Errorf("JSON path %s: expected %v, got %v", path, expected, actual)
		return false
	}

	return true
}

// AssertBodyContains asserts that the response body contains the expected string.
func AssertBodyContains(t *testing.T, rec *httptest.ResponseRecorder, expected string) bool {
	t.Helper()

	body := rec.Body.String()
	if !strings.Contains(body, expected) {
		t.Errorf("Expected body to contain %q, got %q", expected, body)
		return false
	}
	return true
}

// AssertBodyEquals asserts that the response body equals the expected string.
func AssertBodyEquals(t *testing.T, rec *httptest.ResponseRecorder, expected string) bool {
	t.Helper()

	body := rec.Body.String()
	if body != expected {
		t.Errorf("Expected body %q, got %q", expected, body)
		return false
	}
	return true
}

// ParseJSON parses the response body as JSON.
func ParseJSON(rec *httptest.ResponseRecorder, v interface{}) error {
	return json.Unmarshal(rec.Body.Bytes(), v)
}

// getJSONPath retrieves a value from a nested map using dot notation.
func getJSONPath(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}

	return current
}

// deepEqualJSON compares two JSON values for equality.
func deepEqualJSON(a, b interface{}) bool {
	// Handle nil
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle maps
	aMap, aIsMap := a.(map[string]interface{})
	bMap, bIsMap := b.(map[string]interface{})
	if aIsMap && bIsMap {
		if len(aMap) != len(bMap) {
			return false
		}
		for k, v := range aMap {
			if !deepEqualJSON(v, bMap[k]) {
				return false
			}
		}
		return true
	}

	// Handle slices
	aSlice, aIsSlice := a.([]interface{})
	bSlice, bIsSlice := b.([]interface{})
	if aIsSlice && bIsSlice {
		if len(aSlice) != len(bSlice) {
			return false
		}
		for i := range aSlice {
			if !deepEqualJSON(aSlice[i], bSlice[i]) {
				return false
			}
		}
		return true
	}

	// Handle numbers (JSON unmarshals numbers as float64)
	aFloat, aIsFloat := a.(float64)
	bFloat, bIsFloat := b.(float64)
	if aIsFloat && bIsFloat {
		return aFloat == bFloat
	}

	// Default comparison
	return a == b
}

// HTTPClient provides test HTTP client functionality.
type HTTPClient struct {
	baseURL string
	headers http.Header
	client  *http.Client
}

// NewHTTPClient creates a new test HTTP client.
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		headers: make(http.Header),
		client:  &http.Client{},
	}
}

// WithHeader adds a default header for all requests.
func (c *HTTPClient) WithHeader(key, value string) *HTTPClient {
	c.headers.Set(key, value)
	return c
}

// WithClient sets a custom http.Client.
func (c *HTTPClient) WithClient(client *http.Client) *HTTPClient {
	c.client = client
	return c
}

// Do executes a request and returns the response.
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Apply default headers
	for k, v := range c.headers {
		if req.Header.Get(k) == "" {
			req.Header[k] = v
		}
	}
	return c.client.Do(req)
}

// Get performs a GET request.
func (c *HTTPClient) Get(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// PostJSON performs a POST request with JSON body.
func (c *HTTPClient) PostJSON(path string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.Do(req)
}

// TestServer wraps httptest.Server with additional helpers.
type TestServer struct {
	*httptest.Server
	t *testing.T
}

// NewTestServer creates a new test server.
func NewTestServer(t *testing.T, handler http.Handler) *TestServer {
	return &TestServer{
		Server: httptest.NewServer(handler),
		t:      t,
	}
}

// NewTestServerTLS creates a new TLS test server.
func NewTestServerTLS(t *testing.T, handler http.Handler) *TestServer {
	return &TestServer{
		Server: httptest.NewTLSServer(handler),
		t:      t,
	}
}

// Client returns an HTTPClient configured for this server.
func (s *TestServer) Client() *HTTPClient {
	client := NewHTTPClient(s.URL)
	if s.Server.TLS != nil {
		client.WithClient(s.Server.Client())
	}
	return client
}

// URL returns the server URL with the given path.
func (s *TestServer) URLWithPath(path string) string {
	return s.URL + "/" + strings.TrimLeft(path, "/")
}
