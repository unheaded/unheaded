// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package mcpclient

import (
	"encoding/json"
	"errors"
	"fmt"
)

// LatestProtocolVersion is the MCP revision this client asks for first.
const LatestProtocolVersion = "2025-11-25"

// SupportedProtocolVersions are the MCP revisions this client can speak.
// The parts it uses (initialize, tools/list, tools/call, ping, cancellation)
// have the same shape in all of them. A server that answers initialize with
// any other revision is refused.
var SupportedProtocolVersions = []string{
	"2025-11-25",
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

func supportedVersion(v string) bool {
	for _, s := range SupportedProtocolVersions {
		if v == s {
			return true
		}
	}
	return false
}

// JSON-RPC 2.0 error codes used on the wire.
const (
	codeMethodNotFound = -32601
)

// Sentinel errors. Errors from this package wrap one of these where it
// applies, so callers can test with errors.Is.
var (
	// ErrClosed: the client was closed, or the connection failed earlier.
	ErrClosed = errors.New("mcpclient: client closed")
	// ErrServerExited: the server process exited or closed its stdout.
	ErrServerExited = errors.New("mcpclient: server exited")
	// ErrMessageTooLarge: the server sent a line longer than MaxMessageBytes.
	// The connection is closed, since the stream can no longer be trusted.
	ErrMessageTooLarge = errors.New("mcpclient: server message too large")
	// ErrUnsupportedProtocol: version negotiation failed.
	ErrUnsupportedProtocol = errors.New("mcpclient: unsupported MCP protocol version")
	// ErrTimeout: a request got no response within its timeout.
	ErrTimeout = errors.New("mcpclient: request timed out")
	// ErrProtocol: the server broke the protocol in a way that ends a request
	// (a response with neither result nor error, a result that does not
	// decode, a tools/list cursor loop).
	ErrProtocol = errors.New("mcpclient: protocol violation")
)

// RPCError is a JSON-RPC error object returned by the server. Message and
// Data are server-controlled; Error() sanitizes the message.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("mcpclient: server error %d: %s", e.Code, truncate(SanitizeText(e.Message), 1024))
}

// request is an outgoing JSON-RPC request or notification (ID nil).
type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// response is an outgoing reply to a server-initiated request.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// incoming is any message from the server: a response to one of ours, a
// request, or a notification.
type incoming struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}

// reply is what the reader hands a waiting caller.
type reply struct {
	result json.RawMessage
	err    error
}

type implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    struct{}       `json:"capabilities"`
	ClientInfo      implementation `json:"clientInfo"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	Capabilities    struct {
		Tools json.RawMessage `json:"tools"`
	} `json:"capabilities"`
	ServerInfo   implementation `json:"serverInfo"`
	Instructions string         `json:"instructions"`
}

type listToolsParams struct {
	Cursor string `json:"cursor,omitempty"`
}

type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type listToolsResult struct {
	Tools      []wireTool `json:"tools"`
	NextCursor *string    `json:"nextCursor"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
	URI      string `json:"uri"`
	Name     string `json:"name"`
	Resource *struct {
		URI      string  `json:"uri"`
		MimeType string  `json:"mimeType"`
		Text     *string `json:"text"`
		Blob     *string `json:"blob"`
	} `json:"resource"`
}

type callToolResult struct {
	Content           []contentBlock  `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	IsError           bool            `json:"isError"`
}

type cancelledParams struct {
	RequestID int64  `json:"requestId"`
	Reason    string `json:"reason,omitempty"`
}
