// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package httputil

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
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

// encodeFailureBody is the response when a value will not marshal. It is a
// constant so that sending it cannot fail the same way.
const encodeFailureBody = `{"success":false,"error":{"code":"internal_error","message":"response could not be encoded"}}` + "\n"

// WriteSuccess writes a successful JSON response with the standard envelope.
func WriteSuccess(w http.ResponseWriter, status int, data interface{}) {
	WriteJSON(w, status, Response{
		Success: true,
		Data:    data,
		Meta: &MetaInfo{
			Timestamp: time.Now().UTC(),
		},
	})
}

// WriteError writes an error JSON response with the standard envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteErrorWithDetails(w, status, code, message, "")
}

// WriteErrorWithDetails writes an error response including extra detail (e.g. from err.Error()).
func WriteErrorWithDetails(w http.ResponseWriter, status int, code, message, details string) {
	WriteJSON(w, status, Response{
		Success: false,
		Error: &ErrorInfo{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: &MetaInfo{
			Timestamp: time.Now().UTC(),
		},
	})
}

// WriteJSON writes v as JSON with the given status, byte for byte what
// json.Encoder writes. Every JSON response goes through here: it marshals
// before anything is sent, so a value that will not marshal answers 500 and
// is logged, where an Encoder on the ResponseWriter left an empty 200 or the
// handler's status with no body. A Content-Type the handler already set is
// kept.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Error().Err(err).Int("status", status).Msg("response not encoded; answering 500")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, encodeFailureBody)
		return
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	// A failed write means the client left: the status is sent and nothing
	// reaches it.
	_, _ = w.Write(append(body, '\n'))
}
