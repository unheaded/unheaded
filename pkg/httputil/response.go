// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package httputil

import (
	"encoding/json"
	"net/http"
	"time"
)

// Response is the standard API response envelope used across all Unheaded services.
// Services should use WriteSuccess and WriteError instead of constructing this directly.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
	Meta    *MetaInfo   `json:"meta,omitempty"`
}

// ErrorInfo contains error details in API responses.
type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// MetaInfo contains response metadata.
type MetaInfo struct {
	Timestamp time.Time `json:"timestamp"`
}

// WriteSuccess writes a successful JSON response with the standard envelope.
func WriteSuccess(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: true,
		Data:    data,
		Meta: &MetaInfo{
			Timestamp: time.Now().UTC(),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// WriteError writes an error JSON response with the standard envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
		},
		Meta: &MetaInfo{
			Timestamp: time.Now().UTC(),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// WriteErrorWithDetails writes an error response including extra detail (e.g. from err.Error()).
func WriteErrorWithDetails(w http.ResponseWriter, status int, code, message, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: &MetaInfo{
			Timestamp: time.Now().UTC(),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

// WriteJSON writes an arbitrary value as JSON. Use this for responses that don't
// fit the standard envelope (e.g. health checks, Prometheus metrics).
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
