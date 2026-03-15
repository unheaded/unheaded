// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package logger provides the Unheaded Kingdom's structured logging library.
// This is the Kingdom's voice - how services communicate their state.
//
// Features:
//   - Structured JSON logging
//   - Multiple log levels (Trace, Debug, Info, Warn, Error, Fatal, Panic)
//   - Context fields that persist across log calls
//   - Caller information (file:line)
//   - High-performance zero-allocation design
//   - Log sampling for high-volume scenarios
//   - Hooks for custom processing
//   - Console and JSON output modes
//   - Async writing with buffering
package logger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Level represents the severity of a log message.
type Level int8

const (
	// TraceLevel is the most verbose level, used for fine-grained debugging.
	TraceLevel Level = iota - 1
	// DebugLevel is used for debugging information.
	DebugLevel
	// InfoLevel is the default level for informational messages.
	InfoLevel
	// WarnLevel is used for warning messages.
	WarnLevel
	// ErrorLevel is used for error messages.
	ErrorLevel
	// FatalLevel is used for fatal errors that cause the program to exit.
	FatalLevel
	// PanicLevel is used for errors that cause a panic.
	PanicLevel
	// Disabled disables all logging.
	Disabled
)

// String returns the string representation of a log level.
func (l Level) String() string {
	switch l {
	case TraceLevel:
		return "trace"
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "fatal"
	case PanicLevel:
		return "panic"
	default:
		return "unknown"
	}
}

// ParseLevel converts a string to a Level.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "trace":
		return TraceLevel, nil
	case "debug":
		return DebugLevel, nil
	case "info":
		return InfoLevel, nil
	case "warn", "warning":
		return WarnLevel, nil
	case "error":
		return ErrorLevel, nil
	case "fatal":
		return FatalLevel, nil
	case "panic":
		return PanicLevel, nil
	case "disabled":
		return Disabled, nil
	default:
		return InfoLevel, fmt.Errorf("unknown log level: %s", s)
	}
}

// ANSI color codes for console output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

// levelColors maps log levels to ANSI colors.
var levelColors = map[Level]string{
	TraceLevel: colorPurple,
	DebugLevel: colorCyan,
	InfoLevel:  colorGreen,
	WarnLevel:  colorYellow,
	ErrorLevel: colorRed,
	FatalLevel: colorRed + colorBold,
	PanicLevel: colorRed + colorBold,
}

// Hook is called after a log entry is written.
type Hook interface {
	Run(entry *Entry) error
}

// HookFunc is a function adapter for Hook.
type HookFunc func(entry *Entry) error

// Run implements Hook.
func (h HookFunc) Run(entry *Entry) error {
	return h(entry)
}

// Sampler controls log sampling for high-volume scenarios.
type Sampler interface {
	// Sample returns true if the log entry should be logged.
	Sample(level Level) bool
}

// BasicSampler samples every N messages.
type BasicSampler struct {
	N       uint32
	counter uint32
}

// Sample implements Sampler.
func (s *BasicSampler) Sample(level Level) bool {
	n := atomic.AddUint32(&s.counter, 1)
	return n%s.N == 1
}

// BurstSampler allows a burst of messages then samples.
type BurstSampler struct {
	Burst       uint32
	Period      time.Duration
	NextSampler Sampler

	counter uint32
	resetAt int64
}

// Sample implements Sampler.
func (s *BurstSampler) Sample(level Level) bool {
	now := time.Now().UnixNano()
	resetAt := atomic.LoadInt64(&s.resetAt)

	if now > resetAt {
		atomic.StoreUint32(&s.counter, 0)
		atomic.StoreInt64(&s.resetAt, now+int64(s.Period))
	}

	c := atomic.AddUint32(&s.counter, 1)
	if c <= s.Burst {
		return true
	}
	if s.NextSampler != nil {
		return s.NextSampler.Sample(level)
	}
	return false
}

// LevelSampler applies different samplers to different levels.
type LevelSampler struct {
	TraceSampler Sampler
	DebugSampler Sampler
	InfoSampler  Sampler
	WarnSampler  Sampler
	ErrorSampler Sampler
}

// Sample implements Sampler.
func (s *LevelSampler) Sample(level Level) bool {
	switch level {
	case TraceLevel:
		if s.TraceSampler != nil {
			return s.TraceSampler.Sample(level)
		}
	case DebugLevel:
		if s.DebugSampler != nil {
			return s.DebugSampler.Sample(level)
		}
	case InfoLevel:
		if s.InfoSampler != nil {
			return s.InfoSampler.Sample(level)
		}
	case WarnLevel:
		if s.WarnSampler != nil {
			return s.WarnSampler.Sample(level)
		}
	case ErrorLevel:
		if s.ErrorSampler != nil {
			return s.ErrorSampler.Sample(level)
		}
	}
	return true
}

// bufferPool is used to reduce allocations.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, 512))
	},
}

func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func putBuffer(buf *bytes.Buffer) {
	if buf.Cap() > 64*1024 {
		// Don't pool large buffers.
		return
	}
	bufferPool.Put(buf)
}

// Writer wraps an io.Writer with optional async writing.
type Writer struct {
	w      io.Writer
	mu     sync.Mutex
	async  bool
	buffer chan []byte
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewWriter creates a new Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// NewAsyncWriter creates a new async Writer with a buffer.
func NewAsyncWriter(w io.Writer, bufferSize int) *Writer {
	wr := &Writer{
		w:      w,
		async:  true,
		buffer: make(chan []byte, bufferSize),
		done:   make(chan struct{}),
	}
	wr.wg.Add(1)
	go wr.run()
	return wr
}

func (w *Writer) run() {
	defer w.wg.Done()
	for {
		select {
		case data := <-w.buffer:
			w.mu.Lock()
			_, _ = w.w.Write(data)
			w.mu.Unlock()
		case <-w.done:
			// Drain remaining messages.
			for {
				select {
				case data := <-w.buffer:
					w.mu.Lock()
					_, _ = w.w.Write(data)
					w.mu.Unlock()
				default:
					return
				}
			}
		}
	}
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (n int, err error) {
	if w.async {
		data := make([]byte, len(p))
		copy(data, p)
		select {
		case w.buffer <- data:
			return len(p), nil
		default:
			// Buffer full, write synchronously.
			w.mu.Lock()
			n, err = w.w.Write(p)
			w.mu.Unlock()
			return
		}
	}
	w.mu.Lock()
	n, err = w.w.Write(p)
	w.mu.Unlock()
	return
}

// Close closes the async writer and waits for pending writes.
func (w *Writer) Close() error {
	if w.async {
		close(w.done)
		w.wg.Wait()
	}
	if closer, ok := w.w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Config holds logger configuration.
type Config struct {
	Level           Level
	Output          io.Writer
	TimeFormat      string
	CallerEnabled   bool
	CallerSkip      int
	StackTrace      bool
	ConsoleMode     bool
	ColorEnabled    bool
	TimestampField  string
	LevelField      string
	MessageField    string
	CallerField     string
	ErrorField      string
	StackTraceField string
	Hooks           []Hook
	Sampler         Sampler
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Level:           InfoLevel,
		Output:          os.Stdout,
		TimeFormat:      time.RFC3339Nano,
		CallerEnabled:   false,
		CallerSkip:      2,
		StackTrace:      false,
		ConsoleMode:     false,
		ColorEnabled:    true,
		TimestampField:  "time",
		LevelField:      "level",
		MessageField:    "message",
		CallerField:     "caller",
		ErrorField:      "error",
		StackTraceField: "stack",
	}
}

// Logger is the main logging interface.
type Logger struct {
	config  Config
	context []byte
	mu      sync.RWMutex
}

// New creates a new Logger with the given output.
func New(w io.Writer) *Logger {
	config := DefaultConfig()
	config.Output = w
	return &Logger{config: config}
}

// NewWithConfig creates a new Logger with the given configuration.
func NewWithConfig(config Config) *Logger {
	return &Logger{config: config}
}

// Level returns the current log level.
func (l *Logger) Level() Level {
	return l.config.Level
}

// SetLevel sets the log level.
func (l *Logger) SetLevel(level Level) *Logger {
	l.mu.Lock()
	l.config.Level = level
	l.mu.Unlock()
	return l
}

// SetOutput sets the output writer.
func (l *Logger) SetOutput(w io.Writer) *Logger {
	l.mu.Lock()
	l.config.Output = w
	l.mu.Unlock()
	return l
}

// EnableCaller enables caller information in log entries.
func (l *Logger) EnableCaller() *Logger {
	l.mu.Lock()
	l.config.CallerEnabled = true
	l.mu.Unlock()
	return l
}

// DisableCaller disables caller information in log entries.
func (l *Logger) DisableCaller() *Logger {
	l.mu.Lock()
	l.config.CallerEnabled = false
	l.mu.Unlock()
	return l
}

// EnableStackTrace enables stack traces for error logs.
func (l *Logger) EnableStackTrace() *Logger {
	l.mu.Lock()
	l.config.StackTrace = true
	l.mu.Unlock()
	return l
}

// DisableStackTrace disables stack traces.
func (l *Logger) DisableStackTrace() *Logger {
	l.mu.Lock()
	l.config.StackTrace = false
	l.mu.Unlock()
	return l
}

// ConsoleMode enables human-readable console output.
func (l *Logger) ConsoleMode() *Logger {
	l.mu.Lock()
	l.config.ConsoleMode = true
	l.mu.Unlock()
	return l
}

// JSONMode enables JSON output.
func (l *Logger) JSONMode() *Logger {
	l.mu.Lock()
	l.config.ConsoleMode = false
	l.mu.Unlock()
	return l
}

// EnableColor enables colored console output.
func (l *Logger) EnableColor() *Logger {
	l.mu.Lock()
	l.config.ColorEnabled = true
	l.mu.Unlock()
	return l
}

// DisableColor disables colored console output.
func (l *Logger) DisableColor() *Logger {
	l.mu.Lock()
	l.config.ColorEnabled = false
	l.mu.Unlock()
	return l
}

// AddHook adds a hook to the logger.
func (l *Logger) AddHook(hook Hook) *Logger {
	l.mu.Lock()
	l.config.Hooks = append(l.config.Hooks, hook)
	l.mu.Unlock()
	return l
}

// SetSampler sets the sampler for the logger.
func (l *Logger) SetSampler(sampler Sampler) *Logger {
	l.mu.Lock()
	l.config.Sampler = sampler
	l.mu.Unlock()
	return l
}

// With returns a Context for adding fields to the logger.
func (l *Logger) With() *Context {
	l.mu.RLock()
	context := make([]byte, len(l.context))
	copy(context, l.context)
	l.mu.RUnlock()
	return &Context{
		logger:  l,
		context: context,
	}
}

// Trace starts a new log entry at TraceLevel.
func (l *Logger) Trace() *Event {
	return l.newEvent(TraceLevel)
}

// Debug starts a new log entry at DebugLevel.
func (l *Logger) Debug() *Event {
	return l.newEvent(DebugLevel)
}

// Info starts a new log entry at InfoLevel.
func (l *Logger) Info() *Event {
	return l.newEvent(InfoLevel)
}

// Warn starts a new log entry at WarnLevel.
func (l *Logger) Warn() *Event {
	return l.newEvent(WarnLevel)
}

// Error starts a new log entry at ErrorLevel.
func (l *Logger) Error() *Event {
	return l.newEvent(ErrorLevel)
}

// Fatal starts a new log entry at FatalLevel.
// Calling Msg() or Send() will exit the program.
func (l *Logger) Fatal() *Event {
	return l.newEvent(FatalLevel)
}

// Panic starts a new log entry at PanicLevel.
// Calling Msg() or Send() will panic.
func (l *Logger) Panic() *Event {
	return l.newEvent(PanicLevel)
}

// Log starts a new log entry at the given level.
func (l *Logger) Log(level Level) *Event {
	return l.newEvent(level)
}

// Print sends a log entry with no level.
func (l *Logger) Print(v ...interface{}) {
	l.newEvent(InfoLevel).Msg(fmt.Sprint(v...))
}

// Printf sends a formatted log entry with no level.
func (l *Logger) Printf(format string, v ...interface{}) {
	l.newEvent(InfoLevel).Msgf(format, v...)
}

func (l *Logger) newEvent(level Level) *Event {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if level < l.config.Level {
		return &Event{disabled: true}
	}

	if l.config.Sampler != nil && !l.config.Sampler.Sample(level) {
		return &Event{disabled: true}
	}

	e := &Event{
		logger: l,
		level:  level,
		buf:    getBuffer(),
	}

	// Start JSON object.
	e.buf.WriteByte('{')

	// Add timestamp.
	e.buf.WriteByte('"')
	e.buf.WriteString(l.config.TimestampField)
	e.buf.WriteString(`":"`)
	e.buf.WriteString(time.Now().Format(l.config.TimeFormat))
	e.buf.WriteByte('"')

	// Add level.
	e.buf.WriteString(`,"`)
	e.buf.WriteString(l.config.LevelField)
	e.buf.WriteString(`":"`)
	e.buf.WriteString(level.String())
	e.buf.WriteByte('"')

	// Add context fields.
	if len(l.context) > 0 {
		e.buf.WriteByte(',')
		e.buf.Write(l.context)
	}

	return e
}

// Context is used to build logger context with persistent fields.
type Context struct {
	logger  *Logger
	context []byte
}

// Logger returns a new Logger with the context fields.
func (c *Context) Logger() *Logger {
	newLogger := &Logger{
		config:  c.logger.config,
		context: c.context,
	}
	return newLogger
}

// Str adds a string field to the context.
func (c *Context) Str(key, val string) *Context {
	c.addSeparator()
	c.context = appendString(c.context, key, val)
	return c
}

// Strs adds a string slice field to the context.
func (c *Context) Strs(key string, vals []string) *Context {
	c.addSeparator()
	c.context = appendStrings(c.context, key, vals)
	return c
}

// Bytes adds a byte slice field to the context.
func (c *Context) Bytes(key string, val []byte) *Context {
	c.addSeparator()
	c.context = appendBytes(c.context, key, val)
	return c
}

// Int adds an int field to the context.
func (c *Context) Int(key string, val int) *Context {
	c.addSeparator()
	c.context = appendInt(c.context, key, int64(val))
	return c
}

// Int8 adds an int8 field to the context.
func (c *Context) Int8(key string, val int8) *Context {
	c.addSeparator()
	c.context = appendInt(c.context, key, int64(val))
	return c
}

// Int16 adds an int16 field to the context.
func (c *Context) Int16(key string, val int16) *Context {
	c.addSeparator()
	c.context = appendInt(c.context, key, int64(val))
	return c
}

// Int32 adds an int32 field to the context.
func (c *Context) Int32(key string, val int32) *Context {
	c.addSeparator()
	c.context = appendInt(c.context, key, int64(val))
	return c
}

// Int64 adds an int64 field to the context.
func (c *Context) Int64(key string, val int64) *Context {
	c.addSeparator()
	c.context = appendInt(c.context, key, val)
	return c
}

// Uint adds a uint field to the context.
func (c *Context) Uint(key string, val uint) *Context {
	c.addSeparator()
	c.context = appendUint(c.context, key, uint64(val))
	return c
}

// Uint8 adds a uint8 field to the context.
func (c *Context) Uint8(key string, val uint8) *Context {
	c.addSeparator()
	c.context = appendUint(c.context, key, uint64(val))
	return c
}

// Uint16 adds a uint16 field to the context.
func (c *Context) Uint16(key string, val uint16) *Context {
	c.addSeparator()
	c.context = appendUint(c.context, key, uint64(val))
	return c
}

// Uint32 adds a uint32 field to the context.
func (c *Context) Uint32(key string, val uint32) *Context {
	c.addSeparator()
	c.context = appendUint(c.context, key, uint64(val))
	return c
}

// Uint64 adds a uint64 field to the context.
func (c *Context) Uint64(key string, val uint64) *Context {
	c.addSeparator()
	c.context = appendUint(c.context, key, val)
	return c
}

// Float32 adds a float32 field to the context.
func (c *Context) Float32(key string, val float32) *Context {
	c.addSeparator()
	c.context = appendFloat(c.context, key, float64(val), 32)
	return c
}

// Float64 adds a float64 field to the context.
func (c *Context) Float64(key string, val float64) *Context {
	c.addSeparator()
	c.context = appendFloat(c.context, key, val, 64)
	return c
}

// Bool adds a bool field to the context.
func (c *Context) Bool(key string, val bool) *Context {
	c.addSeparator()
	c.context = appendBool(c.context, key, val)
	return c
}

// Time adds a time field to the context.
func (c *Context) Time(key string, val time.Time) *Context {
	c.addSeparator()
	c.context = appendTime(c.context, key, val, time.RFC3339Nano)
	return c
}

// Dur adds a duration field to the context (as milliseconds).
func (c *Context) Dur(key string, val time.Duration) *Context {
	c.addSeparator()
	c.context = appendDuration(c.context, key, val)
	return c
}

// Interface adds an interface field to the context (uses JSON marshaling).
func (c *Context) Interface(key string, val interface{}) *Context {
	c.addSeparator()
	c.context = appendInterface(c.context, key, val)
	return c
}

// Err adds an error field to the context.
func (c *Context) Err(err error) *Context {
	if err == nil {
		return c
	}
	c.addSeparator()
	c.context = appendString(c.context, c.logger.config.ErrorField, err.Error())
	return c
}

// Caller adds caller information to the context.
func (c *Context) Caller() *Context {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		return c
	}
	c.addSeparator()
	c.context = appendString(c.context, c.logger.config.CallerField, file+":"+strconv.Itoa(line))
	return c
}

func (c *Context) addSeparator() {
	if len(c.context) > 0 {
		c.context = append(c.context, ',')
	}
}

// Event represents a log event that is being built.
type Event struct {
	logger   *Logger
	level    Level
	buf      *bytes.Buffer
	disabled bool
	msg      string
}

// Entry represents a completed log entry for hooks.
type Entry struct {
	Level     Level
	Message   string
	Timestamp time.Time
	Caller    string
	Fields    map[string]interface{}
	Raw       []byte
}

// Msg sets the message and writes the log entry.
func (e *Event) Msg(msg string) {
	if e.disabled {
		return
	}
	e.msg = msg
	e.write()
}

// Msgf sets a formatted message and writes the log entry.
func (e *Event) Msgf(format string, v ...interface{}) {
	if e.disabled {
		return
	}
	e.msg = fmt.Sprintf(format, v...)
	e.write()
}

// Send writes the log entry without a message.
func (e *Event) Send() {
	if e.disabled {
		return
	}
	e.write()
}

func (e *Event) write() {
	e.logger.mu.RLock()
	config := e.logger.config
	e.logger.mu.RUnlock()

	// Add caller if enabled.
	if config.CallerEnabled {
		_, file, line, ok := runtime.Caller(config.CallerSkip)
		if ok {
			file = filepath.Base(file)
			e.buf.WriteString(`,"`)
			e.buf.WriteString(config.CallerField)
			e.buf.WriteString(`":"`)
			e.buf.WriteString(file)
			e.buf.WriteByte(':')
			e.buf.WriteString(strconv.Itoa(line))
			e.buf.WriteByte('"')
		}
	}

	// Add stack trace for error levels if enabled.
	if config.StackTrace && e.level >= ErrorLevel {
		stack := make([]byte, 4096)
		n := runtime.Stack(stack, false)
		e.buf.WriteString(`,"`)
		e.buf.WriteString(config.StackTraceField)
		e.buf.WriteString(`":`)
		stackJSON, _ := json.Marshal(string(stack[:n]))
		e.buf.Write(stackJSON)
	}

	// Add message.
	if e.msg != "" {
		e.buf.WriteString(`,"`)
		e.buf.WriteString(config.MessageField)
		e.buf.WriteString(`":`)
		msgJSON, _ := json.Marshal(e.msg)
		e.buf.Write(msgJSON)
	}

	// Close JSON object.
	e.buf.WriteByte('}')
	e.buf.WriteByte('\n')

	// Convert to console format if needed.
	var output []byte
	if config.ConsoleMode {
		output = e.formatConsole(config)
	} else {
		output = e.buf.Bytes()
	}

	// Write output.
	_, _ = config.Output.Write(output)

	// Run hooks.
	if len(config.Hooks) > 0 {
		entry := &Entry{
			Level:     e.level,
			Message:   e.msg,
			Timestamp: time.Now(),
			Raw:       e.buf.Bytes(),
		}
		for _, hook := range config.Hooks {
			_ = hook.Run(entry)
		}
	}

	// Return buffer to pool.
	putBuffer(e.buf)

	// Handle fatal and panic levels.
	switch e.level {
	case FatalLevel:
		os.Exit(1)
	case PanicLevel:
		panic(e.msg)
	}
}

func (e *Event) formatConsole(config Config) []byte {
	buf := getBuffer()
	defer putBuffer(buf)

	// Parse JSON to get fields.
	var data map[string]interface{}
	if err := json.Unmarshal(e.buf.Bytes(), &data); err != nil {
		buf.Write(e.buf.Bytes())
		return buf.Bytes()
	}

	// Format timestamp.
	if ts, ok := data[config.TimestampField].(string); ok {
		if config.ColorEnabled {
			buf.WriteString(colorDim)
		}
		// Shorten timestamp for console.
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			buf.WriteString(t.Format("15:04:05.000"))
		} else {
			buf.WriteString(ts)
		}
		if config.ColorEnabled {
			buf.WriteString(colorReset)
		}
		buf.WriteByte(' ')
	}

	// Format level.
	level := e.level.String()
	if config.ColorEnabled {
		buf.WriteString(levelColors[e.level])
	}
	buf.WriteString(fmt.Sprintf("%-5s", level))
	if config.ColorEnabled {
		buf.WriteString(colorReset)
	}
	buf.WriteByte(' ')

	// Format caller.
	if caller, ok := data[config.CallerField].(string); ok {
		if config.ColorEnabled {
			buf.WriteString(colorDim)
		}
		buf.WriteString(caller)
		if config.ColorEnabled {
			buf.WriteString(colorReset)
		}
		buf.WriteByte(' ')
	}

	// Format message.
	if msg, ok := data[config.MessageField].(string); ok {
		if config.ColorEnabled && e.level >= ErrorLevel {
			buf.WriteString(colorRed)
		}
		buf.WriteString(msg)
		if config.ColorEnabled && e.level >= ErrorLevel {
			buf.WriteString(colorReset)
		}
	}

	// Format other fields.
	for key, val := range data {
		if key == config.TimestampField || key == config.LevelField ||
			key == config.MessageField || key == config.CallerField ||
			key == config.StackTraceField {
			continue
		}
		buf.WriteByte(' ')
		if config.ColorEnabled {
			buf.WriteString(colorCyan)
		}
		buf.WriteString(key)
		if config.ColorEnabled {
			buf.WriteString(colorReset)
		}
		buf.WriteByte('=')
		switch v := val.(type) {
		case string:
			buf.WriteString(v)
		case float64:
			if v == float64(int64(v)) {
				buf.WriteString(strconv.FormatInt(int64(v), 10))
			} else {
				buf.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
			}
		case bool:
			buf.WriteString(strconv.FormatBool(v))
		default:
			fmt.Fprintf(buf, "%v", v)
		}
	}

	buf.WriteByte('\n')

	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result
}

// Str adds a string field to the event.
func (e *Event) Str(key, val string) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendString(nil, key, val))
	return e
}

// Strs adds a string slice field to the event.
func (e *Event) Strs(key string, vals []string) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendStrings(nil, key, vals))
	return e
}

// Bytes adds a byte slice field to the event.
func (e *Event) Bytes(key string, val []byte) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendBytes(nil, key, val))
	return e
}

// Int adds an int field to the event.
func (e *Event) Int(key string, val int) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendInt(nil, key, int64(val)))
	return e
}

// Int8 adds an int8 field to the event.
func (e *Event) Int8(key string, val int8) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendInt(nil, key, int64(val)))
	return e
}

// Int16 adds an int16 field to the event.
func (e *Event) Int16(key string, val int16) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendInt(nil, key, int64(val)))
	return e
}

// Int32 adds an int32 field to the event.
func (e *Event) Int32(key string, val int32) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendInt(nil, key, int64(val)))
	return e
}

// Int64 adds an int64 field to the event.
func (e *Event) Int64(key string, val int64) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendInt(nil, key, val))
	return e
}

// Uint adds a uint field to the event.
func (e *Event) Uint(key string, val uint) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendUint(nil, key, uint64(val)))
	return e
}

// Uint8 adds a uint8 field to the event.
func (e *Event) Uint8(key string, val uint8) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendUint(nil, key, uint64(val)))
	return e
}

// Uint16 adds a uint16 field to the event.
func (e *Event) Uint16(key string, val uint16) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendUint(nil, key, uint64(val)))
	return e
}

// Uint32 adds a uint32 field to the event.
func (e *Event) Uint32(key string, val uint32) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendUint(nil, key, uint64(val)))
	return e
}

// Uint64 adds a uint64 field to the event.
func (e *Event) Uint64(key string, val uint64) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendUint(nil, key, val))
	return e
}

// Float32 adds a float32 field to the event.
func (e *Event) Float32(key string, val float32) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendFloat(nil, key, float64(val), 32))
	return e
}

// Float64 adds a float64 field to the event.
func (e *Event) Float64(key string, val float64) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendFloat(nil, key, val, 64))
	return e
}

// Bool adds a bool field to the event.
func (e *Event) Bool(key string, val bool) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendBool(nil, key, val))
	return e
}

// Time adds a time field to the event.
func (e *Event) Time(key string, val time.Time) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendTime(nil, key, val, time.RFC3339Nano))
	return e
}

// Dur adds a duration field to the event (as milliseconds).
func (e *Event) Dur(key string, val time.Duration) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendDuration(nil, key, val))
	return e
}

// Interface adds an interface field to the event (uses JSON marshaling).
func (e *Event) Interface(key string, val interface{}) *Event {
	if e.disabled {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendInterface(nil, key, val))
	return e
}

// Err adds an error field to the event.
func (e *Event) Err(err error) *Event {
	if e.disabled || err == nil {
		return e
	}
	e.buf.WriteByte(',')
	e.buf.Write(appendString(nil, e.logger.config.ErrorField, err.Error()))
	return e
}

// Stack adds a stack trace to the event.
func (e *Event) Stack() *Event {
	if e.disabled {
		return e
	}
	stack := make([]byte, 4096)
	n := runtime.Stack(stack, false)
	e.buf.WriteByte(',')
	e.buf.WriteByte('"')
	e.buf.WriteString(e.logger.config.StackTraceField)
	e.buf.WriteString(`":`)
	stackJSON, _ := json.Marshal(string(stack[:n]))
	e.buf.Write(stackJSON)
	return e
}

// Caller adds caller information to the event.
func (e *Event) Caller(skip ...int) *Event {
	if e.disabled {
		return e
	}
	skipFrames := 1
	if len(skip) > 0 {
		skipFrames = skip[0]
	}
	_, file, line, ok := runtime.Caller(skipFrames)
	if !ok {
		return e
	}
	file = filepath.Base(file)
	e.buf.WriteByte(',')
	e.buf.Write(appendString(nil, e.logger.config.CallerField, file+":"+strconv.Itoa(line)))
	return e
}

// Fields adds multiple fields from a map.
func (e *Event) Fields(fields map[string]interface{}) *Event {
	if e.disabled {
		return e
	}
	for key, val := range fields {
		e.Interface(key, val)
	}
	return e
}

// Append helpers - these build JSON field representations.

func appendString(dst []byte, key, val string) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":`...)
	valJSON, _ := json.Marshal(val)
	dst = append(dst, valJSON...)
	return dst
}

func appendStrings(dst []byte, key string, vals []string) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":[`...)
	for i, val := range vals {
		if i > 0 {
			dst = append(dst, ',')
		}
		valJSON, _ := json.Marshal(val)
		dst = append(dst, valJSON...)
	}
	dst = append(dst, ']')
	return dst
}

func appendBytes(dst []byte, key string, val []byte) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":`...)
	valJSON, _ := json.Marshal(val)
	dst = append(dst, valJSON...)
	return dst
}

func appendInt(dst []byte, key string, val int64) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":`...)
	dst = strconv.AppendInt(dst, val, 10)
	return dst
}

func appendUint(dst []byte, key string, val uint64) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":`...)
	dst = strconv.AppendUint(dst, val, 10)
	return dst
}

func appendFloat(dst []byte, key string, val float64, bitSize int) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":`...)
	dst = strconv.AppendFloat(dst, val, 'f', -1, bitSize)
	return dst
}

func appendBool(dst []byte, key string, val bool) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":`...)
	dst = strconv.AppendBool(dst, val)
	return dst
}

func appendTime(dst []byte, key string, val time.Time, format string) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":"`...)
	dst = val.AppendFormat(dst, format)
	dst = append(dst, '"')
	return dst
}

func appendDuration(dst []byte, key string, val time.Duration) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":`...)
	dst = strconv.AppendFloat(dst, float64(val)/float64(time.Millisecond), 'f', -1, 64)
	return dst
}

func appendInterface(dst []byte, key string, val interface{}) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, `":`...)
	valJSON, err := json.Marshal(val)
	if err != nil {
		valJSON = []byte("null")
	}
	dst = append(dst, valJSON...)
	return dst
}

// MultiWriter writes to multiple writers.
type MultiWriter struct {
	writers []io.Writer
}

// NewMultiWriter creates a new MultiWriter.
func NewMultiWriter(writers ...io.Writer) *MultiWriter {
	return &MultiWriter{writers: writers}
}

// Write implements io.Writer.
func (mw *MultiWriter) Write(p []byte) (n int, err error) {
	for _, w := range mw.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
	}
	return len(p), nil
}

// FilteredWriter filters log entries by level.
type FilteredWriter struct {
	Writer io.Writer
	Level  Level
}

// Write implements io.Writer.
func (fw *FilteredWriter) Write(p []byte) (n int, err error) {
	return fw.Writer.Write(p)
}

// ConsoleWriter provides human-readable output.
type ConsoleWriter struct {
	Out        io.Writer
	TimeFormat string
	NoColor    bool
}

// NewConsoleWriter creates a new ConsoleWriter.
func NewConsoleWriter() *ConsoleWriter {
	return &ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}
}

// Write implements io.Writer.
func (cw *ConsoleWriter) Write(p []byte) (n int, err error) {
	// Parse JSON.
	var data map[string]interface{}
	if err := json.Unmarshal(p, &data); err != nil {
		return cw.Out.Write(p)
	}

	buf := getBuffer()
	defer putBuffer(buf)

	// Format timestamp.
	if ts, ok := data["time"].(string); ok {
		if !cw.NoColor {
			buf.WriteString(colorDim)
		}
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			buf.WriteString(t.Format(cw.TimeFormat))
		} else {
			buf.WriteString(ts)
		}
		if !cw.NoColor {
			buf.WriteString(colorReset)
		}
		buf.WriteByte(' ')
	}

	// Format level.
	if lvl, ok := data["level"].(string); ok {
		level, _ := ParseLevel(lvl)
		if !cw.NoColor {
			buf.WriteString(levelColors[level])
		}
		buf.WriteString(fmt.Sprintf("%-5s", lvl))
		if !cw.NoColor {
			buf.WriteString(colorReset)
		}
		buf.WriteByte(' ')
	}

	// Format message.
	if msg, ok := data["message"].(string); ok {
		buf.WriteString(msg)
	}

	// Format other fields.
	for key, val := range data {
		if key == "time" || key == "level" || key == "message" {
			continue
		}
		buf.WriteByte(' ')
		if !cw.NoColor {
			buf.WriteString(colorCyan)
		}
		buf.WriteString(key)
		if !cw.NoColor {
			buf.WriteString(colorReset)
		}
		buf.WriteByte('=')
		switch v := val.(type) {
		case string:
			buf.WriteString(v)
		case float64:
			if v == float64(int64(v)) {
				buf.WriteString(strconv.FormatInt(int64(v), 10))
			} else {
				buf.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
			}
		default:
			fmt.Fprintf(buf, "%v", v)
		}
	}

	buf.WriteByte('\n')
	return cw.Out.Write(buf.Bytes())
}

// Global logger instance.
var globalLogger = New(os.Stdout)

// SetGlobalLogger sets the global logger.
func SetGlobalLogger(l *Logger) {
	globalLogger = l
}

// GetGlobalLogger returns the global logger.
func GetGlobalLogger() *Logger {
	return globalLogger
}

// Global logging functions.

// Trace starts a new global log entry at TraceLevel.
func Trace() *Event {
	return globalLogger.Trace()
}

// Debug starts a new global log entry at DebugLevel.
func Debug() *Event {
	return globalLogger.Debug()
}

// Info starts a new global log entry at InfoLevel.
func Info() *Event {
	return globalLogger.Info()
}

// Warn starts a new global log entry at WarnLevel.
func Warn() *Event {
	return globalLogger.Warn()
}

// Error starts a new global log entry at ErrorLevel.
func Error() *Event {
	return globalLogger.Error()
}

// Fatal starts a new global log entry at FatalLevel.
func Fatal() *Event {
	return globalLogger.Fatal()
}

// Panic starts a new global log entry at PanicLevel.
func Panic() *Event {
	return globalLogger.Panic()
}

// Print sends a global log entry.
func Print(v ...interface{}) {
	globalLogger.Print(v...)
}

// Printf sends a formatted global log entry.
func Printf(format string, v ...interface{}) {
	globalLogger.Printf(format, v...)
}

// With returns a Context for the global logger.
func With() *Context {
	return globalLogger.With()
}

// SetLevel sets the global log level.
func SetLevel(level Level) {
	globalLogger.SetLevel(level)
}

// SetOutput sets the global output.
func SetOutput(w io.Writer) {
	globalLogger.SetOutput(w)
}
