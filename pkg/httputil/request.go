// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// DefaultMaxBodySize is the default maximum request body size (1MB).
const DefaultMaxBodySize int64 = 1 * 1024 * 1024

// DecodeJSONBody reads and decodes a JSON request body with a size limit.
// It validates Content-Type, enforces the size limit, and unmarshals into dst.
func DecodeJSONBody(r *http.Request, maxSize int64, dst interface{}) error {
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		return &RequestError{
			Status:  http.StatusUnsupportedMediaType,
			Code:    "INVALID_CONTENT_TYPE",
			Message: "Content-Type must be application/json",
		}
	}

	if maxSize <= 0 {
		maxSize = DefaultMaxBodySize
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxSize))
	if err != nil {
		return &RequestError{
			Status:  http.StatusBadRequest,
			Code:    "READ_BODY_FAILED",
			Message: "failed to read request body",
		}
	}

	if len(body) == 0 {
		return &RequestError{
			Status:  http.StatusBadRequest,
			Code:    "EMPTY_BODY",
			Message: "request body cannot be empty",
		}
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return &RequestError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_JSON",
			Message: fmt.Sprintf("invalid JSON: %v", err),
		}
	}

	return nil
}

// RequestError represents an error during request processing.
// It carries the appropriate HTTP status code and error code.
type RequestError struct {
	Status  int
	Code    string
	Message string
}

func (e *RequestError) Error() string {
	return e.Message
}

// IsRequestError checks if an error is a RequestError and returns it.
func IsRequestError(err error) (*RequestError, bool) {
	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		return reqErr, true
	}
	return nil, false
}

// HandleRequestError writes an appropriate error response for a RequestError.
// If the error is not a RequestError, it writes a 500 Internal Server Error.
func HandleRequestError(w http.ResponseWriter, err error) {
	if reqErr, ok := IsRequestError(err); ok {
		WriteError(w, reqErr.Status, reqErr.Code, reqErr.Message)
		return
	}
	WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
