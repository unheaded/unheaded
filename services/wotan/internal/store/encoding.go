// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package store

import (
	"encoding/json"
	"fmt"
)

// Encoding version for forward compatibility.
const encodingVersion byte = 1

// EncodeMessage serializes a Message to bytes with a version prefix.
// Format: [version:1][json:N]
func EncodeMessage(msg *Message) ([]byte, error) {
	if msg == nil {
		return nil, ErrInvalidMessage
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("store: encode message: %w", err)
	}
	buf := make([]byte, 1+len(data))
	buf[0] = encodingVersion
	copy(buf[1:], data)
	return buf, nil
}

// DecodeMessage deserializes bytes back to a Message.
func DecodeMessage(data []byte) (*Message, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("store: decode message: data too short (%d bytes)", len(data))
	}
	version := data[0]
	if version != encodingVersion {
		return nil, fmt.Errorf("store: decode message: unsupported version %d", version)
	}
	var msg Message
	if err := json.Unmarshal(data[1:], &msg); err != nil {
		return nil, fmt.Errorf("store: decode message: %w", err)
	}
	return &msg, nil
}
