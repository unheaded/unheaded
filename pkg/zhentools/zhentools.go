// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package zhentools is the seam between tool sources (Zhen's MCP server,
// the Champion's own tools) and the models that call them (local Zhenai,
// the Claude API). A model client sees only Provider; it never learns
// whether a tool runs over MCP stdio or in-process.
package zhentools

import (
	"context"
	"encoding/json"
)

// Tool describes one callable tool. InputSchema is a JSON Schema object,
// passed to the model as is.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Result is what one call returns. IsError marks a tool-level failure the
// model should see and may recover from (bad arguments, file not found,
// refused by a gate); a transport failure is the error from Call instead.
type Result struct {
	Text    string
	IsError bool
}

// Provider lists tools and runs them. Implementations must be safe for
// concurrent use: a model may issue several tool calls in one turn.
type Provider interface {
	Tools(ctx context.Context) ([]Tool, error)
	Call(ctx context.Context, name string, input json.RawMessage) (Result, error)
}
