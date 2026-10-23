// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"unheaded/pkg/zhentools"
)

// Tools walks tools/list, following nextCursor, and returns every tool with
// a valid, unique name. Descriptions and schema strings are sanitized.
// Entries refused (bad name, duplicate, schema not a JSON object) are listed
// in Diagnostics().SkippedTools.
func (c *Client) Tools(ctx context.Context) ([]zhentools.Tool, error) {
	var (
		out     []zhentools.Tool
		skipped []string
		cursor  string
	)
	seen := make(map[string]bool)
	cursors := make(map[string]bool)
	for page := 0; ; page++ {
		if page >= c.cfg.MaxPages {
			return nil, fmt.Errorf("%w: tools/list ran past %d pages", ErrProtocol, c.cfg.MaxPages)
		}
		raw, err := c.roundTrip(ctx, "tools/list", listToolsParams{Cursor: cursor}, c.cfg.CallTimeout, true)
		if err != nil {
			return nil, fmt.Errorf("mcpclient: tools/list: %w", err)
		}
		var res listToolsResult
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, fmt.Errorf("%w: tools/list result: %v", ErrProtocol, err)
		}
		for _, wt := range res.Tools {
			t, why := convertTool(wt)
			if why == "" && seen[t.Name] {
				why = "duplicate name"
			}
			if why != "" {
				skipped = append(skipped, fmt.Sprintf("%q: %s", truncate(SanitizeText(wt.Name), 64), why))
				continue
			}
			if len(out) >= c.cfg.MaxTools {
				return nil, fmt.Errorf("%w: server lists more than %d tools", ErrProtocol, c.cfg.MaxTools)
			}
			seen[t.Name] = true
			out = append(out, t)
		}
		if res.NextCursor == nil || *res.NextCursor == "" {
			break
		}
		if cursors[*res.NextCursor] {
			return nil, fmt.Errorf("%w: tools/list cursor repeated", ErrProtocol)
		}
		cursors[*res.NextCursor] = true
		cursor = *res.NextCursor
	}
	c.mu.Lock()
	c.skippedTools = skipped
	c.mu.Unlock()
	return out, nil
}

// Call runs one tool. A tool-level failure the model can see and recover
// from (isError, a JSON-RPC error such as an unknown tool, arguments that
// are not a JSON object) comes back as Result.IsError with a nil error. The
// error is for transport failures: timeout, cancellation, a dead server.
func (c *Client) Call(ctx context.Context, name string, input json.RawMessage) (zhentools.Result, error) {
	if !validToolName(name) {
		return zhentools.Result{
			Text:    fmt.Sprintf("invalid tool name %q", truncate(SanitizeText(name), 128)),
			IsError: true,
		}, nil
	}
	args := bytes.TrimSpace(input)
	if isNull(args) {
		args = []byte("{}")
	}
	if args[0] != '{' || !json.Valid(args) {
		return zhentools.Result{Text: "tool arguments must be a JSON object", IsError: true}, nil
	}

	raw, err := c.roundTrip(ctx, "tools/call", callToolParams{Name: name, Arguments: args}, c.cfg.CallTimeout, true)
	if err != nil {
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) {
			return zhentools.Result{
				Text:    truncate(SanitizeText(rpcErr.Message), c.cfg.MaxResultBytes),
				IsError: true,
			}, nil
		}
		return zhentools.Result{}, fmt.Errorf("mcpclient: tools/call %s: %w", name, err)
	}
	var res callToolResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return zhentools.Result{}, fmt.Errorf("%w: tools/call result: %v", ErrProtocol, err)
	}
	return zhentools.Result{Text: renderContent(res, c.cfg.MaxResultBytes), IsError: res.IsError}, nil
}

// renderContent flattens content blocks to text. Text blocks pass through
// (sanitized); binary blocks are described, never inlined.
func renderContent(res callToolResult, limit int) string {
	var b strings.Builder
	for i, blk := range res.Content {
		if b.Len() > limit {
			break
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		switch blk.Type {
		case "text":
			b.WriteString(blk.Text)
		case "image", "audio":
			fmt.Fprintf(&b, "[%s content omitted: %s, %d bytes base64]", blk.Type, short(blk.MimeType), len(blk.Data))
		case "resource_link":
			fmt.Fprintf(&b, "[resource link: %s %s]", short(blk.Name), short(blk.URI))
		case "resource":
			switch r := blk.Resource; {
			case r == nil:
				b.WriteString("[embedded resource omitted: no body]")
			case r.Text != nil:
				fmt.Fprintf(&b, "[resource %s]\n%s", short(r.URI), *r.Text)
			default:
				blob := 0
				if r.Blob != nil {
					blob = len(*r.Blob)
				}
				fmt.Fprintf(&b, "[embedded resource omitted: %s %s, %d bytes base64]", short(r.URI), short(r.MimeType), blob)
			}
		default:
			fmt.Fprintf(&b, "[unsupported content type %q omitted]", short(blk.Type))
		}
	}
	if len(res.Content) == 0 && !isNull(res.StructuredContent) {
		b.Write(res.StructuredContent)
	}
	return truncate(SanitizeText(b.String()), limit)
}

func short(s string) string { return truncate(s, 256) }

// convertTool validates one tools/list entry; why is empty when accepted.
func convertTool(wt wireTool) (t zhentools.Tool, why string) {
	if !validToolName(wt.Name) {
		return t, "invalid name"
	}
	schema, err := sanitizeSchema(wt.InputSchema)
	if err != nil {
		return t, err.Error()
	}
	return zhentools.Tool{
		Name:        wt.Name,
		Description: truncate(SanitizeText(wt.Description), maxDescriptionBytes),
		InputSchema: schema,
	}, ""
}

// validToolName applies the MCP spec's tool-name rule: 1-128 characters
// from A-Z a-z 0-9 _ - . (names reach the model and logs verbatim, so they
// are validated, not sanitized).
func validToolName(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9':
		case ch == '_' || ch == '-' || ch == '.':
		default:
			return false
		}
	}
	return true
}

// sanitizeSchema checks the schema is a JSON object of bounded size and
// sanitizes every string in it, keys included: property descriptions reach
// the model as surely as the tool description does.
func sanitizeSchema(raw json.RawMessage) (json.RawMessage, error) {
	if isNull(raw) {
		return json.RawMessage(`{"type":"object"}`), nil
	}
	if len(raw) > maxSchemaBytes {
		return nil, fmt.Errorf("input schema larger than %d bytes", maxSchemaBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, errors.New("input schema is not valid JSON")
	}
	if _, ok := v.(map[string]any); !ok {
		return nil, errors.New("input schema is not a JSON object")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(sanitizeValue(v)); err != nil {
		return nil, errors.New("input schema does not re-encode")
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

func sanitizeValue(v any) any {
	switch x := v.(type) {
	case string:
		return SanitizeText(x)
	case []any:
		for i := range x {
			x[i] = sanitizeValue(x[i])
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[SanitizeText(k)] = sanitizeValue(val)
		}
		return out
	default:
		return v
	}
}
