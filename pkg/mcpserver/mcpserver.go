// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package mcpserver is the server side of MCP over stdio: newline-delimited
// JSON-RPC 2.0 carrying initialize, ping, tools/list and tools/call, the
// subset Claude Code and pkg/mcpclient use. It was extracted from
// cmd/lich-mcp so the Lich and Zhen servers share one transport
// (S-CHAMPION-HARDENING Step 192: first-party, no MCP SDK admitted).
//
// The package knows nothing about tools: a Handler lists and runs them,
// and a tool failure or refusal is a CallToolResult with IsError set (the
// model sees it and may recover), never a JSON-RPC error. JSON-RPC errors
// are reserved for messages the server cannot act on at all.
//
// Attack surface: the reader is bounded per line (MaxMessageBytes); a
// longer line ends the stream, because nothing after it can be trusted to
// be framed. The server binds no port.
package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// LatestProtocolVersion is the MCP revision this server prefers.
const LatestProtocolVersion = "2025-11-25"

// SupportedProtocolVersions are the revisions this server answers with
// when a client asks for one (the same list as pkg/mcpclient). The parts
// used here have the same shape in all of them.
var SupportedProtocolVersions = []string{
	"2025-11-25",
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

// DefaultMaxMessageBytes bounds one JSON-RPC line when Server.MaxMessageBytes is 0.
const DefaultMaxMessageBytes = 8 << 20

// JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
)

// Request is one incoming JSON-RPC message.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// Error is a JSON-RPC error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Response is one outgoing JSON-RPC message.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Tool is one tool as tools/list sends it.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ContentBlock is one block of a tool result. Only text is produced.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// CallToolResult is a tools/call result.
type CallToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

// CallToolParams are tools/call's params. Arguments is never empty when
// it reaches a Handler: a call without arguments arrives as {}.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// TextResult is a successful one-block text result.
func TextResult(text string) CallToolResult {
	return CallToolResult{Content: []ContentBlock{{Type: "text", Text: text}}}
}

// ErrorResult is a one-block text result with IsError set.
func ErrorResult(msg string) CallToolResult {
	return CallToolResult{Content: []ContentBlock{{Type: "text", Text: msg}}, IsError: true}
}

// Handler lists and runs tools. CallTool reports every failure, refusals
// included, as an IsError result.
type Handler interface {
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, p CallToolParams) CallToolResult
}

// Server serves one client over a reader/writer pair.
type Server struct {
	Name            string // serverInfo.name
	Version         string // serverInfo.version
	Instructions    string // optional initialize instructions
	Handler         Handler
	MaxMessageBytes int // default DefaultMaxMessageBytes
}

// Serve runs the loop until EOF (nil), an oversized line, a write error,
// or ctx cancellation. Requests are answered one at a time, in order.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	if s.Handler == nil {
		return errors.New("mcpserver: Server needs a Handler")
	}
	limit := s.MaxMessageBytes
	if limit <= 0 {
		limit = DefaultMaxMessageBytes
	}
	br := bufio.NewReaderSize(r, 64<<10)
	enc := json.NewEncoder(w)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, err := ReadLine(br, limit)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil // the client closed stdin
			}
			return err
		}
		if len(line) == 0 {
			continue
		}
		var in Request
		if jerr := json.Unmarshal(line, &in); jerr != nil {
			if werr := enc.Encode(Response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &Error{Code: CodeParseError, Message: "parse error"}}); werr != nil {
				return werr
			}
			continue
		}
		out, respond := s.Handle(ctx, in)
		if respond {
			if werr := enc.Encode(out); werr != nil {
				return werr
			}
		}
	}
}

// Handle answers one request. respond is false for a notification (no id).
func (s *Server) Handle(ctx context.Context, in Request) (Response, bool) {
	isNotification := len(in.ID) == 0 || string(in.ID) == "null"
	reply := func(result any, e *Error) (Response, bool) {
		if isNotification {
			return Response{}, false
		}
		return Response{JSONRPC: "2.0", ID: in.ID, Result: result, Error: e}, true
	}

	switch in.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(in.Params, &p) // absent or malformed params: answer with the latest
		res := map[string]any{
			"protocolVersion": negotiate(p.ProtocolVersion),
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
		}
		if s.Instructions != "" {
			res["instructions"] = s.Instructions
		}
		return reply(res, nil)
	case "notifications/initialized", "notifications/cancelled":
		return Response{}, false
	case "ping":
		return reply(map[string]any{}, nil)
	case "tools/list":
		tools, err := s.Handler.ListTools(ctx)
		if err != nil {
			return reply(nil, &Error{Code: CodeInvalidRequest, Message: "tools/list failed"})
		}
		if tools == nil {
			tools = []Tool{}
		}
		for i := range tools {
			if len(tools[i].InputSchema) == 0 {
				tools[i].InputSchema = json.RawMessage(`{"type":"object"}`)
			}
		}
		return reply(map[string]any{"tools": tools}, nil)
	case "tools/call":
		var p CallToolParams
		if err := json.Unmarshal(in.Params, &p); err != nil || p.Name == "" {
			return reply(nil, &Error{Code: CodeInvalidParams, Message: "invalid tools/call params"})
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		return reply(s.Handler.CallTool(ctx, p), nil)
	default:
		return reply(nil, &Error{Code: CodeMethodNotFound, Message: "method not found"})
	}
}

// negotiate echoes a supported requested revision, else the latest.
func negotiate(asked string) string {
	for _, v := range SupportedProtocolVersions {
		if v == asked {
			return v
		}
	}
	return LatestProtocolVersion
}

// ReadLine reads one newline-delimited message of at most max bytes,
// without the line ending. A longer line is an error.
func ReadLine(br *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > max {
			return nil, fmt.Errorf("mcpserver: message exceeds %d bytes; stream no longer trusted", max)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			if len(buf) > 0 && errors.Is(err, io.EOF) {
				return trimNL(buf), nil
			}
			return nil, err
		}
		return trimNL(buf), nil
	}
}

func trimNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
