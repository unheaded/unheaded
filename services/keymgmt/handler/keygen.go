// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package handler provides HTTP handlers for the keymgmt service API.
package handler

import (
	"encoding/json"
	"net/http"
)

// KeygenRequest is the request body for POST /api/v1/keys.
type KeygenRequest struct {
	AlgoID    uint8  `json:"algo_id"`
	ParamSet  uint8  `json:"param_set"`
	ServiceID uint16 `json:"service_id"`
}

// KeygenResponse is the response body for key generation.
type KeygenResponse struct {
	KeyID     string `json:"key_id"`
	KeyRef    uint16 `json:"key_ref"`
	AlgoID    uint8  `json:"algo_id"`
	ParamSet  uint8  `json:"param_set"`
	ServiceID uint16 `json:"service_id"`
	Status    string `json:"status"`
}

// DecodeKeygenRequest decodes a keygen request from an HTTP request.
func DecodeKeygenRequest(r *http.Request) (*KeygenRequest, error) {
	var req KeygenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) // #nosec G104 -- response already committed; an encode failure here means the client went away and nothing further can be sent
}
