// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package mcpclient is a Model Context Protocol client over the stdio
// transport, written against the standard library only (ADR-096). It spawns
// an MCP server as a child process, speaks newline-delimited JSON-RPC 2.0 on
// its stdin/stdout, and exposes the server's tools as a zhentools.Provider.
//
// The server is untrusted. Every line it writes is length-capped, malformed
// lines are skipped, and every string that reaches the caller (tool names
// are validated, descriptions, schemas and results are passed through
// SanitizeText) has control, escape and invisible format characters
// replaced. Server stderr is kept in a bounded tail buffer for diagnostics,
// never forwarded.
//
// A Client is safe for concurrent use. Each request gets its own id and its
// own timeout; a cancelled or timed-out call sends notifications/cancelled.
// If the server dies, every pending and future call fails with an error
// wrapping ErrServerExited that carries the exit status and stderr tail.
package mcpclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"unheaded/pkg/zhentools"
)

// Defaults for zero Config fields.
const (
	DefaultInitTimeout     = 30 * time.Second
	DefaultCallTimeout     = 120 * time.Second
	DefaultShutdownGrace   = 2 * time.Second
	DefaultMaxMessageBytes = 8 << 20
	DefaultMaxStderrBytes  = 64 << 10
	DefaultMaxResultBytes  = 1 << 20
	DefaultMaxTools        = 512
	DefaultMaxPages        = 64

	maxDescriptionBytes = 8 << 10
	maxSchemaBytes      = 256 << 10
	stderrInErrorBytes  = 2 << 10
	notifyTimeout       = time.Second
	exitStatusWait      = 500 * time.Millisecond
	serverRequestQueue  = 32
)

var _ zhentools.Provider = (*Client)(nil)

// Config describes the server to spawn and the client's limits. Only
// Command is required.
type Config struct {
	// Command and Args start the server, e.g. "python3",
	// []string{"raft/zhen_mcp_server.py"}. No shell is involved.
	Command string
	Args    []string
	// Env is the server's environment. nil inherits this process's.
	Env []string
	// Dir is the server's working directory. Empty means the current one.
	Dir string

	// ClientName and ClientVersion are sent as clientInfo.
	ClientName    string
	ClientVersion string
	// ProtocolVersion is the MCP revision requested in initialize. It must be
	// one of SupportedProtocolVersions. Default LatestProtocolVersion.
	ProtocolVersion string

	// InitTimeout bounds the initialize handshake.
	InitTimeout time.Duration
	// CallTimeout bounds each tools/list page and tools/call, on top of
	// whatever deadline the caller's context carries.
	CallTimeout time.Duration
	// ShutdownGrace is how long Close waits after closing stdin, and again
	// after SIGTERM, before killing the server.
	ShutdownGrace time.Duration

	// MaxMessageBytes caps one line from the server. A longer line closes
	// the connection with ErrMessageTooLarge.
	MaxMessageBytes int
	// MaxStderrBytes is the size of the stderr tail kept for diagnostics.
	MaxStderrBytes int
	// MaxResultBytes caps Result.Text; longer text is cut and marked.
	MaxResultBytes int
	// MaxTools and MaxPages bound a tools/list walk.
	MaxTools int
	MaxPages int
}

func (cfg *Config) setDefaults() error {
	if cfg.Command == "" {
		return errors.New("mcpclient: Config.Command is empty")
	}
	if cfg.ClientName == "" {
		cfg.ClientName = "unheaded-mcpclient"
	}
	if cfg.ClientVersion == "" {
		cfg.ClientVersion = "0.1.0"
	}
	if cfg.ProtocolVersion == "" {
		cfg.ProtocolVersion = LatestProtocolVersion
	}
	if !supportedVersion(cfg.ProtocolVersion) {
		return fmt.Errorf("%w: Config.ProtocolVersion %q (supported %v)",
			ErrUnsupportedProtocol, cfg.ProtocolVersion, SupportedProtocolVersions)
	}
	setDur := func(d *time.Duration, def time.Duration) {
		if *d <= 0 {
			*d = def
		}
	}
	setInt := func(n *int, def int) {
		if *n <= 0 {
			*n = def
		}
	}
	setDur(&cfg.InitTimeout, DefaultInitTimeout)
	setDur(&cfg.CallTimeout, DefaultCallTimeout)
	setDur(&cfg.ShutdownGrace, DefaultShutdownGrace)
	setInt(&cfg.MaxMessageBytes, DefaultMaxMessageBytes)
	setInt(&cfg.MaxStderrBytes, DefaultMaxStderrBytes)
	setInt(&cfg.MaxResultBytes, DefaultMaxResultBytes)
	setInt(&cfg.MaxTools, DefaultMaxTools)
	setInt(&cfg.MaxPages, DefaultMaxPages)
	return nil
}

// Client is a connection to one MCP server process.
type Client struct {
	cfg    Config
	cmd    *exec.Cmd
	stdin  *os.File // our write end of the server's stdin
	stdout *os.File // our read end of the server's stdout
	stderr *tailBuffer

	writeMu sync.Mutex

	mu            sync.Mutex
	nextID        int64
	pending       map[int64]chan reply
	err           error         // terminal error; set once
	done          chan struct{} // closed when err is set
	malformed     int64
	lastMalformed string
	skippedTools  []string
	info          initializeResult

	serverReqs chan serverRequest
	readerDone chan struct{}
	waitDone   chan struct{}
	waitErr    error
	stopOnce   sync.Once
	killed     bool
}

type serverRequest struct {
	id     json.RawMessage
	method string
}

// Start spawns the server and runs the initialize handshake. ctx bounds the
// handshake only; the server lives until Close or until it exits. On error
// the server has already been stopped.
func Start(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.setDefaults(); err != nil {
		return nil, err
	}
	c, err := spawn(cfg)
	if err != nil {
		return nil, err
	}
	if err := c.initialize(ctx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func spawn(cfg Config) (*Client, error) {
	childIn, parentIn, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("mcpclient: stdin pipe: %w", err)
	}
	parentOut, childOut, err := os.Pipe()
	if err != nil {
		_ = childIn.Close()
		_ = parentIn.Close()
		return nil, fmt.Errorf("mcpclient: stdout pipe: %w", err)
	}

	// #nosec G204 -- the command is the caller's configured MCP server, run
	// without a shell; choosing which program to run is this API's purpose.
	cmd := exec.Command(cfg.Command, cfg.Args...)
	cmd.Stdin = childIn
	cmd.Stdout = childOut
	stderr := newTailBuffer(cfg.MaxStderrBytes)
	cmd.Stderr = stderr
	cmd.Env = cfg.Env
	cmd.Dir = cfg.Dir
	// Bound Wait if a grandchild keeps the stderr pipe open.
	cmd.WaitDelay = cfg.ShutdownGrace

	startErr := cmd.Start()
	// The child has its own copies now (or never will); ours must go so
	// that EOF on stdout means the server is gone.
	_ = childIn.Close()
	_ = childOut.Close()
	if startErr != nil {
		_ = parentIn.Close()
		_ = parentOut.Close()
		return nil, fmt.Errorf("mcpclient: start %q: %w", cfg.Command, startErr)
	}

	c := &Client{
		cfg:        cfg,
		cmd:        cmd,
		stdin:      parentIn,
		stdout:     parentOut,
		stderr:     stderr,
		pending:    make(map[int64]chan reply),
		done:       make(chan struct{}),
		serverReqs: make(chan serverRequest, serverRequestQueue),
		readerDone: make(chan struct{}),
		waitDone:   make(chan struct{}),
	}
	go func() {
		c.waitErr = cmd.Wait()
		close(c.waitDone)
	}()
	go c.readLoop()
	go c.answerLoop()
	return c, nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := initializeParams{
		ProtocolVersion: c.cfg.ProtocolVersion,
		ClientInfo:      implementation{Name: c.cfg.ClientName, Version: c.cfg.ClientVersion},
	}
	// The spec forbids cancelling initialize, so no cancellation notice.
	raw, err := c.roundTrip(ctx, "initialize", params, c.cfg.InitTimeout, false)
	if err != nil {
		return fmt.Errorf("mcpclient: initialize: %w", err)
	}
	var res initializeResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return fmt.Errorf("%w: initialize result: %v", ErrProtocol, err)
	}
	if !supportedVersion(res.ProtocolVersion) {
		return fmt.Errorf("%w: requested %s, server answered %q; client supports %v",
			ErrUnsupportedProtocol, c.cfg.ProtocolVersion,
			truncate(SanitizeText(res.ProtocolVersion), 64), SupportedProtocolVersions)
	}
	if isNull(res.Capabilities.Tools) {
		return fmt.Errorf("%w: server does not advertise the tools capability", ErrProtocol)
	}
	res.ServerInfo.Name = truncate(SanitizeText(res.ServerInfo.Name), 256)
	res.ServerInfo.Version = truncate(SanitizeText(res.ServerInfo.Version), 256)
	res.Instructions = truncate(SanitizeText(res.Instructions), maxDescriptionBytes)
	c.mu.Lock()
	c.info = res
	c.mu.Unlock()

	if err := c.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("mcpclient: notifications/initialized: %w", err)
	}
	return nil
}

// Ping checks the server is responsive.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.roundTrip(ctx, "ping", nil, c.cfg.CallTimeout, true)
	return err
}

// Close shuts the server down: it closes the server's stdin, waits
// ShutdownGrace, sends SIGTERM, waits again, then kills it. Pending calls
// fail with ErrClosed. It is safe to call more than once and concurrently.
// It returns an error only if the server had to be killed.
func (c *Client) Close() error {
	c.fail(ErrClosed)
	c.stop()
	c.mu.Lock()
	killed := c.killed
	c.mu.Unlock()
	if killed {
		return errors.New("mcpclient: server ignored stdin close and SIGTERM; killed")
	}
	return nil
}

// Diagnostics is a snapshot of what the client knows about the server. All
// strings are sanitized server output.
type Diagnostics struct {
	ProtocolVersion   string
	ServerName        string
	ServerVersion     string
	Instructions      string
	Stderr            string // tail of the server's stderr
	StderrDropped     int64  // bytes of stderr dropped from the front
	MalformedMessages int64  // lines skipped as not JSON-RPC 2.0
	LastMalformed     string
	SkippedTools      []string // tools/list entries refused on the last walk
	Err               error    // terminal error, nil while connected
}

// Diagnostics returns a snapshot for logs and error reports.
func (c *Client) Diagnostics() Diagnostics {
	c.mu.Lock()
	defer c.mu.Unlock()
	tail, dropped := c.stderr.snapshot()
	return Diagnostics{
		ProtocolVersion:   c.info.ProtocolVersion,
		ServerName:        c.info.ServerInfo.Name,
		ServerVersion:     c.info.ServerInfo.Version,
		Instructions:      c.info.Instructions,
		Stderr:            SanitizeText(tail),
		StderrDropped:     dropped,
		MalformedMessages: c.malformed,
		LastMalformed:     c.lastMalformed,
		SkippedTools:      append([]string(nil), c.skippedTools...),
		Err:               c.err,
	}
}

// roundTrip sends one request and waits for its response, the timeout, the
// caller's context, or the connection's end, whichever is first.
func (c *Client) roundTrip(ctx context.Context, method string, params any, timeout time.Duration, cancellable bool) (json.RawMessage, error) {
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return nil, err
	}
	c.nextID++
	id := c.nextID
	ch := make(chan reply, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	timeoutErr := fmt.Errorf("%w: %s after %v", ErrTimeout, method, timeout)
	ctx, cancel := context.WithTimeoutCause(ctx, timeout, timeoutErr)
	defer cancel()

	deadline, _ := ctx.Deadline()
	if err := c.write(request{JSONRPC: "2.0", ID: &id, Method: method, Params: params}, deadline); err != nil {
		c.removePending(id)
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		return nil, fmt.Errorf("mcpclient: send %s: %w", method, err)
	}

	select {
	case r := <-ch:
		return r.result, r.err
	case <-c.done:
		select {
		case r := <-ch: // answered just before the end
			return r.result, r.err
		default:
		}
		c.removePending(id)
		return nil, c.terminalErr()
	case <-ctx.Done():
		c.removePending(id)
		cause := context.Cause(ctx)
		if cancellable {
			c.notifyCancelled(id, cause)
		}
		return nil, cause
	}
}

func (c *Client) notify(method string, params any) error {
	return c.write(request{JSONRPC: "2.0", Method: method, Params: params}, time.Now().Add(notifyTimeout))
}

func (c *Client) notifyCancelled(id int64, cause error) {
	reason := "cancelled by client"
	if errors.Is(cause, ErrTimeout) {
		reason = "client timeout"
	}
	_ = c.notify("notifications/cancelled", cancelledParams{RequestID: id, Reason: reason})
}

func (c *Client) removePending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) terminalErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// write sends one message as a single line. A write cut off part way leaves
// the server holding half a message, so that ends the connection; a write
// that sent nothing before its deadline does not.
func (c *Client) write(msg any, deadline time.Time) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("mcpclient: encode: %w", err)
	}
	b = append(b, '\n')

	if err := c.terminalErr(); err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.stdin.SetWriteDeadline(deadline) // pipes support deadlines on Linux/BSD
	n, err := c.stdin.Write(b)
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		if n == 0 {
			return fmt.Errorf("%w: server is not reading its stdin", ErrTimeout)
		}
		err = fmt.Errorf("mcpclient: write to server timed out part way: %w", err)
		c.fail(err)
		return err
	}
	// A broken or closed pipe usually means the server is gone. Give the
	// reader a moment to see EOF so the caller gets the exit status and
	// stderr tail rather than a bare EPIPE.
	t := time.NewTimer(2 * exitStatusWait)
	defer t.Stop()
	select {
	case <-c.done:
		return c.terminalErr()
	case <-t.C:
	}
	err = fmt.Errorf("mcpclient: write to server: %w", err)
	c.fail(err)
	return err
}

// fail records the connection's terminal error (the first one wins), wakes
// every waiter, and starts stopping the server.
func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return
	}
	c.err = err
	c.pending = make(map[int64]chan reply)
	close(c.done)
	c.mu.Unlock()
	go c.stop()
}

func (c *Client) stop() {
	c.stopOnce.Do(func() {
		_ = c.stdin.Close() // EOF on the server's stdin: the spec's shutdown signal
		if !c.waitFor(c.cfg.ShutdownGrace) {
			_ = c.cmd.Process.Signal(syscall.SIGTERM)
			if !c.waitFor(c.cfg.ShutdownGrace) {
				c.mu.Lock()
				c.killed = true
				c.mu.Unlock()
				_ = c.cmd.Process.Kill()
				<-c.waitDone
			}
		}
		// A grandchild may still hold stdout open; closing our end ends
		// the reader regardless.
		_ = c.stdout.Close()
		<-c.readerDone
	})
}

func (c *Client) waitFor(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.waitDone:
		return true
	case <-t.C:
		return false
	}
}

func (c *Client) readLoop() {
	defer close(c.readerDone)
	br := bufio.NewReaderSize(c.stdout, 64<<10)
	for {
		line, err := readLine(br, c.cfg.MaxMessageBytes)
		if err != nil {
			if errors.Is(err, ErrMessageTooLarge) {
				c.fail(fmt.Errorf("%w (limit %d bytes); connection closed", ErrMessageTooLarge, c.cfg.MaxMessageBytes))
			} else {
				c.fail(c.exitError(err))
			}
			return
		}
		if len(line) > 0 {
			c.dispatch(line)
		}
	}
}

// readLine returns the next line without its terminator, never holding more
// than max bytes of it.
func readLine(br *bufio.Reader, maxBytes int) ([]byte, error) {
	var buf []byte
	for {
		frag, err := br.ReadSlice('\n')
		if len(buf)+len(frag) > maxBytes+2 { // +2 for an optional CRLF
			return nil, ErrMessageTooLarge
		}
		buf = append(buf, frag...)
		switch {
		case err == nil:
			buf = buf[:len(buf)-1]
			if n := len(buf); n > 0 && buf[n-1] == '\r' {
				buf = buf[:n-1]
			}
			if len(buf) > maxBytes {
				return nil, ErrMessageTooLarge
			}
			return buf, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && len(buf) > 0:
			return buf, nil // last line had no newline; EOF comes next call
		default:
			return nil, err
		}
	}
}

func (c *Client) dispatch(line []byte) {
	var m incoming
	if err := json.Unmarshal(line, &m); err != nil {
		c.noteMalformed(line, err.Error())
		return
	}
	if m.JSONRPC != "2.0" {
		c.noteMalformed(line, "not a JSON-RPC 2.0 message")
		return
	}
	switch {
	case m.Method != "" && !isNull(m.ID):
		c.queueServerRequest(m)
	case m.Method != "":
		// Notifications (logging, progress, list_changed) need no action:
		// Tools() always asks the server fresh.
	default:
		c.deliver(m, line)
	}
}

func (c *Client) deliver(m incoming, line []byte) {
	var id int64
	if err := json.Unmarshal(m.ID, &id); err != nil {
		c.noteMalformed(line, "response id is not one of ours")
		return
	}
	c.mu.Lock()
	ch, ok := c.pending[id]
	delete(c.pending, id)
	c.mu.Unlock()
	if !ok {
		return // late answer to a cancelled or timed-out request
	}
	switch {
	case m.Error != nil:
		ch <- reply{err: m.Error}
	case m.Result == nil:
		ch <- reply{err: fmt.Errorf("%w: response has neither result nor error", ErrProtocol)}
	default:
		ch <- reply{result: m.Result}
	}
}

func (c *Client) queueServerRequest(m incoming) {
	// Echo the id back only if it is the scalar the spec requires.
	if len(m.ID) > 128 || (m.ID[0] != '"' && m.ID[0] != '-' && (m.ID[0] < '0' || m.ID[0] > '9')) {
		c.noteMalformed(m.ID, "server request id is not a string or number")
		return
	}
	select {
	case c.serverReqs <- serverRequest{id: m.ID, method: m.Method}:
	default: // a server flooding requests gets no answers, not our memory
	}
}

// answerLoop replies to server-initiated requests. It answers ping and
// refuses the rest: this client offers no sampling, roots or elicitation.
func (c *Client) answerLoop() {
	for {
		select {
		case <-c.done:
			return
		case r := <-c.serverReqs:
			resp := response{JSONRPC: "2.0", ID: r.id}
			if r.method == "ping" {
				resp.Result = struct{}{}
			} else {
				resp.Error = &RPCError{Code: codeMethodNotFound, Message: "method not supported by this client"}
			}
			_ = c.write(resp, time.Now().Add(notifyTimeout))
		}
	}
}

func (c *Client) noteMalformed(line []byte, why string) {
	c.mu.Lock()
	c.malformed++
	c.lastMalformed = truncate(SanitizeText(why+": "+string(truncateBytes(line, 256))), 512)
	c.mu.Unlock()
}

// exitError describes the server's end after its stdout closed.
func (c *Client) exitError(readErr error) error {
	status := "still running after closing stdout"
	if c.waitFor(exitStatusWait) {
		if c.waitErr == nil {
			status = "exit status 0"
		} else {
			status = c.waitErr.Error()
		}
	}
	msg := fmt.Sprintf("%v (%s", ErrServerExited, status)
	if !errors.Is(readErr, io.EOF) {
		msg += "; read: " + readErr.Error()
	}
	msg += ")"
	if tail, _ := c.stderr.snapshot(); tail != "" {
		if len(tail) > stderrInErrorBytes {
			tail = tail[len(tail)-stderrInErrorBytes:]
		}
		msg += "\nserver stderr (tail):\n" + SanitizeText(tail)
	}
	return &exitErr{msg: msg}
}

type exitErr struct{ msg string }

func (e *exitErr) Error() string        { return e.msg }
func (e *exitErr) Is(target error) bool { return target == ErrServerExited }

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}

func truncateBytes(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu      sync.Mutex
	buf     []byte
	max     int
	dropped int64
}

func newTailBuffer(maxBytes int) *tailBuffer {
	return &tailBuffer{buf: make([]byte, 0, maxBytes), max: maxBytes}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(p)
	if n >= t.max {
		t.dropped += int64(len(t.buf) + n - t.max)
		t.buf = append(t.buf[:0], p[n-t.max:]...)
		return n, nil
	}
	if over := len(t.buf) + n - t.max; over > 0 {
		t.dropped += int64(over)
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	t.buf = append(t.buf, p...)
	return n, nil
}

func (t *tailBuffer) snapshot() (string, int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf), t.dropped
}
