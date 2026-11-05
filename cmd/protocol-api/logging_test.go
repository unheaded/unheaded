// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// A request's path is decoded before it reaches the middleware, so a client
// chooses what it holds. One request must stay one log line.
func TestLoggingMiddleware_OneRequestOneLine(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	on := true
	old := logRequests
	logRequests = &on
	t.Cleanup(func() { logRequests = old })

	h := loggingMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/a%0Asecond-line", nil)
	req.Header.Set("User-Agent", "agent\nthird-line")
	h.ServeHTTP(httptest.NewRecorder(), req)

	got := strings.TrimRight(buf.String(), "\n")
	if strings.Contains(got, "\n") {
		t.Fatalf("one request wrote more than one line: %q", got)
	}
	if !strings.Contains(got, "second-line") {
		t.Fatalf("the path was lost from the line: %q", got)
	}
}
