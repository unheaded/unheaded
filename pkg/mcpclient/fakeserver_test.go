// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package mcpclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The test binary doubles as a fake MCP server: run with fakeServerEnv set,
// TestMain serves MCP on stdin/stdout instead of running tests. The mode
// selects the misbehaviour under test.
const fakeServerEnv = "MCPCLIENT_FAKE_SERVER_MODE"

func TestMain(m *testing.M) {
	if mode, ok := os.LookupEnv(fakeServerEnv); ok {
		runFakeServer(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type fakeServer struct {
	mode    string
	writeMu sync.Mutex
	out     *bufio.Writer

	mu        sync.Mutex
	cancelled map[string]chan struct{} // request id -> closed on cancel
	pingReply chan json.RawMessage
}

func runFakeServer(mode string) {
	s := &fakeServer{
		mode:      mode,
		out:       bufio.NewWriter(os.Stdout),
		cancelled: make(map[string]chan struct{}),
		pingReply: make(chan json.RawMessage, 1),
	}
	if mode == "ignore_shutdown" {
		signal.Ignore(syscall.SIGTERM)
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var wg sync.WaitGroup
	for sc.Scan() {
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			fmt.Fprintf(os.Stderr, "fake: bad line from client: %v\n", err)
			continue
		}
		if msg.Method == "" { // a response to our ping
			s.pingReply <- msg.Result
			continue
		}
		fmt.Fprintf(os.Stderr, "fake: got %s\n", msg.Method)
		switch msg.Method {
		case "initialize":
			s.initialize(msg.ID, msg.Params)
		case "notifications/initialized":
		case "notifications/cancelled":
			var p struct {
				RequestID json.RawMessage `json:"requestId"`
				Reason    string          `json:"reason"`
			}
			_ = json.Unmarshal(msg.Params, &p)
			fmt.Fprintf(os.Stderr, "fake: cancelled %s (%s)\n", p.RequestID, p.Reason)
			s.mu.Lock()
			if ch, ok := s.cancelled[string(p.RequestID)]; ok {
				close(ch)
				delete(s.cancelled, string(p.RequestID))
			}
			s.mu.Unlock()
		case "ping":
			s.result(msg.ID, map[string]any{})
		case "tools/list":
			s.listTools(msg.ID, msg.Params)
		case "tools/call":
			ch := make(chan struct{})
			s.mu.Lock()
			s.cancelled[string(msg.ID)] = ch
			s.mu.Unlock()
			wg.Add(1)
			go func(id, params json.RawMessage) {
				defer wg.Done()
				s.callTool(id, params, ch)
			}(msg.ID, msg.Params)
		default:
			s.send(map[string]any{"jsonrpc": "2.0", "id": msg.ID,
				"error": map[string]any{"code": -32601, "message": "Method not found"}})
		}
	}
	if mode == "ignore_shutdown" {
		time.Sleep(time.Hour)
	}
	wg.Wait()
}

func (s *fakeServer) send(v any) {
	b, _ := json.Marshal(v)
	s.raw(string(b))
}

func (s *fakeServer) raw(line string) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.out.WriteString(line + "\n")
	_ = s.out.Flush()
}

func (s *fakeServer) result(id json.RawMessage, result any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *fakeServer) initialize(id, params json.RawMessage) {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	version := p.ProtocolVersion
	caps := map[string]any{"tools": map[string]any{}}
	switch s.mode {
	case "silent":
		return
	case "exit_early":
		fmt.Fprintln(os.Stderr, "Traceback: ModuleNotFoundError: No module named 'mcp'")
		os.Exit(2)
	case "bad_version":
		version = "1999-01-01"
	case "old_version":
		version = "2025-03-26"
	case "no_tools":
		caps = map[string]any{"logging": map[string]any{}}
	}
	s.result(id, map[string]any{
		"protocolVersion": version,
		"capabilities":    caps,
		"serverInfo":      map[string]any{"name": "fake\x1b[31m", "version": "1.0"},
		"instructions":    "be nice\u202E",
	})
	// Unsolicited notification and a log line; the client must ignore both.
	s.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/message",
		"params": map[string]any{"level": "info", "data": "hello"}})
}

func tool(name, desc string, schema any) map[string]any {
	return map[string]any{"name": name, "description": desc, "inputSchema": schema}
}

var objSchema = map[string]any{"type": "object", "properties": map[string]any{
	"text": map[string]any{"type": "string", "description": "what to echo\x1b[0m"},
	"ms":   map[string]any{"type": "integer"},
}}

func (s *fakeServer) listTools(id, params json.RawMessage) {
	var p struct {
		Cursor string `json:"cursor"`
	}
	_ = json.Unmarshal(params, &p)
	if s.mode == "cursor_loop" {
		s.result(id, map[string]any{"tools": []any{tool("echo", "e", objSchema)}, "nextCursor": "again"})
		return
	}
	switch p.Cursor {
	case "":
		s.result(id, map[string]any{
			"tools": []any{
				tool("echo", "Echo text back.", objSchema),
				tool("fail", "Always reports a tool error.", nil),
			},
			"nextCursor": "page-2",
		})
	case "page-2":
		s.result(id, map[string]any{"tools": []any{
			tool("slow", "Sleeps ms milliseconds.", objSchema),
			tool("crash", "Kills the server.", objSchema),
			tool("binary", "Returns non-text content.", objSchema),
			tool("hostile", "Evil \x1b]0;pwned\x07 desc\U000E0041", objSchema),
			tool("ping_me", "Server pings the client first.", objSchema),
			tool("oversize", "Answers with a huge line.", objSchema),
			tool("malformed", "Writes garbage before answering.", objSchema),
			tool("bad name", "space in name", objSchema),
			tool("echo", "duplicate", objSchema),
			tool("arrayschema", "schema is an array", []any{1}),
		}})
	default:
		s.send(map[string]any{"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": -32602, "message": "bad cursor"}})
	}
}

func text(t string) map[string]any { return map[string]any{"type": "text", "text": t} }

func (s *fakeServer) callTool(id, params json.RawMessage, cancelled chan struct{}) {
	var p struct {
		Name      string `json:"name"`
		Arguments struct {
			Text string `json:"text"`
			MS   int    `json:"ms"`
		} `json:"arguments"`
	}
	_ = json.Unmarshal(params, &p)
	content := func(blocks ...any) map[string]any { return map[string]any{"content": blocks} }
	switch p.Name {
	case "echo":
		if p.Arguments.MS > 0 {
			time.Sleep(time.Duration(p.Arguments.MS) * time.Millisecond)
		}
		s.result(id, content(text(p.Arguments.Text)))
	case "fail":
		s.result(id, map[string]any{"content": []any{text("it broke")}, "isError": true})
	case "slow":
		select {
		case <-time.After(time.Duration(p.Arguments.MS) * time.Millisecond):
			s.result(id, content(text("finally")))
		case <-cancelled:
			// Per spec, no response to a cancelled request.
		}
	case "crash":
		fmt.Fprintln(os.Stderr, "fatal: segfault in tool crash")
		os.Exit(3)
	case "binary":
		s.result(id, content(
			text("header"),
			map[string]any{"type": "image", "mimeType": "image/png", "data": "aGVsbG8="},
			map[string]any{"type": "resource_link", "name": "doc", "uri": "file:///x"},
			map[string]any{"type": "resource", "resource": map[string]any{"uri": "file:///t", "text": "inline body"}},
			map[string]any{"type": "resource", "resource": map[string]any{"uri": "file:///b", "mimeType": "application/zip", "blob": "AAAA"}},
			map[string]any{"type": "hologram\x1b"},
		))
	case "hostile":
		s.result(id, content(text("\x1b[2J\x1b[Hcleared\r\nline2\u202Eevil\x00")))
	case "ping_me":
		s.send(map[string]any{"jsonrpc": "2.0", "id": "srv-1", "method": "ping"})
		s.send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "sampling/createMessage"})
		select {
		case r := <-s.pingReply:
			s.result(id, content(text("pong result "+string(r))))
		case <-time.After(5 * time.Second):
			s.result(id, map[string]any{"content": []any{text("no pong")}, "isError": true})
		}
	case "oversize":
		s.raw(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":{"content":[{"type":"text","text":"` +
			strings.Repeat("A", 64<<10) + `"}]}}`)
	case "malformed":
		s.raw("this is not json")
		s.raw(`{"jsonrpc":"1.0","id":` + string(id) + `}`)
		s.raw(`[{"jsonrpc":"2.0"}]`)
		s.raw(`{"jsonrpc":"2.0","id":{"nested":true},"method":"evil"}`)
		s.raw(`{"jsonrpc":"2.0","id":123456789,"result":{}}`) // nobody's id
		s.result(id, content(text("survived")))
	default:
		s.send(map[string]any{"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": -32602, "message": "Unknown tool: " + p.Name}})
	}
}
