// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package testing

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestBuilder(t *testing.T) {
	t.Run("basic GET request", func(t *testing.T) {
		req := Get("/api/users").Request()

		if req.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", req.Method)
		}
		if req.URL.Path != "/api/users" {
			t.Errorf("Expected path /api/users, got %s", req.URL.Path)
		}
	})

	t.Run("POST request with JSON", func(t *testing.T) {
		body := map[string]string{"name": "test"}
		req := Post("/api/users").WithJSON(body).Request()

		if req.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", req.Method)
		}

		contentType := req.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", contentType)
		}

		bodyBytes, _ := io.ReadAll(req.Body)
		var parsed map[string]string
		json.Unmarshal(bodyBytes, &parsed)
		if parsed["name"] != "test" {
			t.Errorf("Expected name=test, got %s", parsed["name"])
		}
	})

	t.Run("request with headers", func(t *testing.T) {
		req := Get("/api/users").
			WithHeader("Authorization", "Bearer token123").
			WithHeader("Accept", "application/json").
			Request()

		if req.Header.Get("Authorization") != "Bearer token123" {
			t.Errorf("Expected Authorization header")
		}
		if req.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept header")
		}
	})

	t.Run("request with query params", func(t *testing.T) {
		req := Get("/api/users").
			WithQuery("page", "1").
			WithQuery("limit", "10").
			Request()

		if req.URL.Query().Get("page") != "1" {
			t.Errorf("Expected page=1")
		}
		if req.URL.Query().Get("limit") != "10" {
			t.Errorf("Expected limit=10")
		}
	})

	t.Run("request with cookies", func(t *testing.T) {
		req := Get("/api/users").
			WithCookieSimple("session", "abc123").
			Request()

		cookie, err := req.Cookie("session")
		if err != nil {
			t.Errorf("Expected session cookie: %v", err)
		}
		if cookie.Value != "abc123" {
			t.Errorf("Expected cookie value abc123, got %s", cookie.Value)
		}
	})

	t.Run("request with basic auth", func(t *testing.T) {
		req := Get("/api/users").
			WithBasicAuth("user", "pass").
			Request()

		user, pass, ok := req.BasicAuth()
		if !ok {
			t.Error("Expected basic auth to be set")
		}
		if user != "user" || pass != "pass" {
			t.Errorf("Expected user:pass, got %s:%s", user, pass)
		}
	})

	t.Run("request with bearer token", func(t *testing.T) {
		req := Get("/api/users").
			WithBearer("token123").
			Request()

		auth := req.Header.Get("Authorization")
		if auth != "Bearer token123" {
			t.Errorf("Expected Bearer token123, got %s", auth)
		}
	})
}

func TestAssertStatus(t *testing.T) {
	rec := NewRecorder()
	rec.WriteHeader(http.StatusOK)

	if !AssertStatus(t, rec, http.StatusOK) {
		t.Error("Expected status assertion to pass")
	}
}

func TestAssertHeader(t *testing.T) {
	rec := NewRecorder()
	rec.Header().Set("Content-Type", "application/json")

	if !AssertHeader(t, rec, "Content-Type", "application/json") {
		t.Error("Expected header assertion to pass")
	}
}

func TestAssertJSON(t *testing.T) {
	rec := NewRecorder()
	expected := map[string]interface{}{
		"name": "test",
		"age":  float64(30),
	}
	json.NewEncoder(rec).Encode(expected)

	if !AssertJSON(t, rec, expected) {
		t.Error("Expected JSON assertion to pass")
	}
}

func TestAssertJSONPath(t *testing.T) {
	rec := NewRecorder()
	data := map[string]interface{}{
		"user": map[string]interface{}{
			"name": "test",
		},
	}
	json.NewEncoder(rec).Encode(data)

	if !AssertJSONPath(t, rec, "user.name", "test") {
		t.Error("Expected JSON path assertion to pass")
	}
}

func TestAssertBodyContains(t *testing.T) {
	rec := NewRecorder()
	rec.WriteString("Hello, World!")

	if !AssertBodyContains(t, rec, "World") {
		t.Error("Expected body contains assertion to pass")
	}
}

func TestAssertBodyEquals(t *testing.T) {
	rec := NewRecorder()
	rec.WriteString("Hello")

	if !AssertBodyEquals(t, rec, "Hello") {
		t.Error("Expected body equals assertion to pass")
	}
}

func TestHTTPClient(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			t.Error("Expected custom header")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := NewHTTPClient(server.URL).
		WithHeader("X-Custom", "value")

	resp, err := client.Get("/api/test")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestTestServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := NewTestServer(t, handler)
	defer server.Close()

	client := server.Client()
	resp, err := client.Get("/test")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestParseJSON(t *testing.T) {
	rec := NewRecorder()
	json.NewEncoder(rec).Encode(map[string]string{"key": "value"})

	var result map[string]string
	if err := ParseJSON(rec, &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("Expected key=value, got key=%s", result["key"])
	}
}

func TestStatusHelpers(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		assertFn func(*testing.T, *httptest.ResponseRecorder) bool
	}{
		{"OK", http.StatusOK, AssertStatusOK},
		{"Created", http.StatusCreated, AssertStatusCreated},
		{"NoContent", http.StatusNoContent, AssertStatusNoContent},
		{"BadRequest", http.StatusBadRequest, AssertStatusBadRequest},
		{"Unauthorized", http.StatusUnauthorized, AssertStatusUnauthorized},
		{"Forbidden", http.StatusForbidden, AssertStatusForbidden},
		{"NotFound", http.StatusNotFound, AssertStatusNotFound},
		{"InternalServerError", http.StatusInternalServerError, AssertStatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := NewRecorder()
			rec.WriteHeader(tc.status)
			if !tc.assertFn(t, rec) {
				t.Errorf("Expected assertion to pass for status %d", tc.status)
			}
		})
	}
}

func TestAssertHeaderContains(t *testing.T) {
	rec := NewRecorder()
	rec.Header().Set("Content-Type", "application/json; charset=utf-8")

	if !AssertHeaderContains(t, rec, "Content-Type", "application/json") {
		t.Error("Expected header contains assertion to pass")
	}
}
