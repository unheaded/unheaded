// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package observe

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// AccessLogEntry represents a single access log entry.
type AccessLogEntry struct {
	// Timestamp
	Timestamp time.Time

	// Request info
	Method       string
	Path         string
	Protocol     string
	Host         string
	UserAgent    string
	RemoteAddr   string
	RequestSize  int64

	// Response info
	StatusCode   int
	ResponseSize int64
	Latency      time.Duration

	// Service mesh info
	UpstreamService   string
	UpstreamNamespace string
	UpstreamAddress   string
	UpstreamLatency   time.Duration

	// mTLS info
	PeerSPIFFEID string
	TLSVersion   string

	// Tracing
	TraceID  string
	SpanID   string

	// Additional fields
	Error  string
	Retry  int
	Custom map[string]string
}

// AccessLogger logs access entries.
type AccessLogger struct {
	mu       sync.Mutex
	writer   io.Writer
	format   AccessLogFormat
	buffer   *bytes.Buffer
	filters  []AccessLogFilter
	async    bool
	queue    chan *AccessLogEntry
	done     chan struct{}
}

// AccessLogFormat defines the log format.
type AccessLogFormat int

const (
	FormatJSON AccessLogFormat = iota
	FormatCommon
	FormatCombined
	FormatCustom
)

// AccessLogFilter filters log entries.
type AccessLogFilter func(*AccessLogEntry) bool

// AccessLogConfig configures the access logger.
type AccessLogConfig struct {
	Writer     io.Writer
	Format     AccessLogFormat
	BufferSize int
	Async      bool
	QueueSize  int
	Filters    []AccessLogFilter
}

// DefaultAccessLogConfig returns a default configuration.
func DefaultAccessLogConfig() *AccessLogConfig {
	return &AccessLogConfig{
		Writer:     os.Stdout,
		Format:     FormatJSON,
		BufferSize: 4096,
		Async:      true,
		QueueSize:  10000,
	}
}

// NewAccessLogger creates a new access logger.
func NewAccessLogger(config *AccessLogConfig) *AccessLogger {
	if config == nil {
		config = DefaultAccessLogConfig()
	}

	logger := &AccessLogger{
		writer:  config.Writer,
		format:  config.Format,
		buffer:  bytes.NewBuffer(make([]byte, 0, config.BufferSize)),
		filters: config.Filters,
		async:   config.Async,
	}

	if config.Async {
		logger.queue = make(chan *AccessLogEntry, config.QueueSize)
		logger.done = make(chan struct{})
		go logger.processQueue()
	}

	return logger
}

// Log logs an access entry.
func (l *AccessLogger) Log(entry *AccessLogEntry) {
	// Apply filters
	for _, filter := range l.filters {
		if !filter(entry) {
			return
		}
	}

	if l.async && l.queue != nil {
		select {
		case l.queue <- entry:
		default:
			// Queue full, drop entry
		}
		return
	}

	l.writeEntry(entry)
}

// Close closes the logger.
func (l *AccessLogger) Close() error {
	if l.async && l.done != nil {
		close(l.done)
	}
	return nil
}

func (l *AccessLogger) processQueue() {
	for {
		select {
		case <-l.done:
			// Drain remaining entries
			for {
				select {
				case entry := <-l.queue:
					l.writeEntry(entry)
				default:
					return
				}
			}
		case entry := <-l.queue:
			l.writeEntry(entry)
		}
	}
}

func (l *AccessLogger) writeEntry(entry *AccessLogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.buffer.Reset()

	switch l.format {
	case FormatJSON:
		l.formatJSON(entry)
	case FormatCommon:
		l.formatCommon(entry)
	case FormatCombined:
		l.formatCombined(entry)
	default:
		l.formatJSON(entry)
	}

	l.buffer.WriteByte('\n')
	l.writer.Write(l.buffer.Bytes())
}

func (l *AccessLogger) formatJSON(entry *AccessLogEntry) {
	l.buffer.WriteString(`{"timestamp":"`)
	l.buffer.WriteString(entry.Timestamp.Format(time.RFC3339Nano))
	l.buffer.WriteString(`","method":"`)
	l.buffer.WriteString(entry.Method)
	l.buffer.WriteString(`","path":"`)
	l.buffer.WriteString(escapeJSON(entry.Path))
	l.buffer.WriteString(`","protocol":"`)
	l.buffer.WriteString(entry.Protocol)
	l.buffer.WriteString(`","host":"`)
	l.buffer.WriteString(entry.Host)
	l.buffer.WriteString(`","remote_addr":"`)
	l.buffer.WriteString(entry.RemoteAddr)
	l.buffer.WriteString(`","status":`)
	l.buffer.WriteString(itoa(entry.StatusCode))
	l.buffer.WriteString(`,"request_size":`)
	l.buffer.WriteString(itoa64(entry.RequestSize))
	l.buffer.WriteString(`,"response_size":`)
	l.buffer.WriteString(itoa64(entry.ResponseSize))
	l.buffer.WriteString(`,"latency_ms":`)
	l.buffer.WriteString(fmt.Sprintf("%.3f", float64(entry.Latency.Microseconds())/1000.0))

	if entry.UpstreamService != "" {
		l.buffer.WriteString(`,"upstream_service":"`)
		l.buffer.WriteString(entry.UpstreamService)
		l.buffer.WriteByte('"')
	}

	if entry.UpstreamNamespace != "" {
		l.buffer.WriteString(`,"upstream_namespace":"`)
		l.buffer.WriteString(entry.UpstreamNamespace)
		l.buffer.WriteByte('"')
	}

	if entry.UpstreamAddress != "" {
		l.buffer.WriteString(`,"upstream_address":"`)
		l.buffer.WriteString(entry.UpstreamAddress)
		l.buffer.WriteByte('"')
	}

	if entry.UpstreamLatency > 0 {
		l.buffer.WriteString(`,"upstream_latency_ms":`)
		l.buffer.WriteString(fmt.Sprintf("%.3f", float64(entry.UpstreamLatency.Microseconds())/1000.0))
	}

	if entry.PeerSPIFFEID != "" {
		l.buffer.WriteString(`,"peer_spiffe_id":"`)
		l.buffer.WriteString(entry.PeerSPIFFEID)
		l.buffer.WriteByte('"')
	}

	if entry.TLSVersion != "" {
		l.buffer.WriteString(`,"tls_version":"`)
		l.buffer.WriteString(entry.TLSVersion)
		l.buffer.WriteByte('"')
	}

	if entry.TraceID != "" {
		l.buffer.WriteString(`,"trace_id":"`)
		l.buffer.WriteString(entry.TraceID)
		l.buffer.WriteByte('"')
	}

	if entry.SpanID != "" {
		l.buffer.WriteString(`,"span_id":"`)
		l.buffer.WriteString(entry.SpanID)
		l.buffer.WriteByte('"')
	}

	if entry.Error != "" {
		l.buffer.WriteString(`,"error":"`)
		l.buffer.WriteString(escapeJSON(entry.Error))
		l.buffer.WriteByte('"')
	}

	if entry.Retry > 0 {
		l.buffer.WriteString(`,"retry":`)
		l.buffer.WriteString(itoa(entry.Retry))
	}

	if entry.UserAgent != "" {
		l.buffer.WriteString(`,"user_agent":"`)
		l.buffer.WriteString(escapeJSON(entry.UserAgent))
		l.buffer.WriteByte('"')
	}

	l.buffer.WriteByte('}')
}

func (l *AccessLogger) formatCommon(entry *AccessLogEntry) {
	// Common Log Format: host ident authuser date request status bytes
	l.buffer.WriteString(entry.RemoteAddr)
	l.buffer.WriteString(" - - [")
	l.buffer.WriteString(entry.Timestamp.Format("02/Jan/2006:15:04:05 -0700"))
	l.buffer.WriteString(`] "`)
	l.buffer.WriteString(entry.Method)
	l.buffer.WriteByte(' ')
	l.buffer.WriteString(entry.Path)
	l.buffer.WriteByte(' ')
	l.buffer.WriteString(entry.Protocol)
	l.buffer.WriteString(`" `)
	l.buffer.WriteString(itoa(entry.StatusCode))
	l.buffer.WriteByte(' ')
	l.buffer.WriteString(itoa64(entry.ResponseSize))
}

func (l *AccessLogger) formatCombined(entry *AccessLogEntry) {
	// Combined Log Format: Common + referer + user-agent
	l.formatCommon(entry)
	l.buffer.WriteString(` "-" "`)
	l.buffer.WriteString(escapeJSON(entry.UserAgent))
	l.buffer.WriteByte('"')
}

func escapeJSON(s string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			result = append(result, '\\', '"')
		case '\\':
			result = append(result, '\\', '\\')
		case '\n':
			result = append(result, '\\', 'n')
		case '\r':
			result = append(result, '\\', 'r')
		case '\t':
			result = append(result, '\\', 't')
		default:
			if c < 32 {
				result = append(result, '\\', 'u', '0', '0', hexDigit(c>>4), hexDigit(c&0xf))
			} else {
				result = append(result, c)
			}
		}
	}
	return string(result)
}

func hexDigit(n byte) byte {
	if n < 10 {
		return '0' + n
	}
	return 'a' + n - 10
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

func itoa64(i int64) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

// Predefined filters

// FilterByStatus returns a filter that only logs entries with status in the given range.
func FilterByStatus(min, max int) AccessLogFilter {
	return func(entry *AccessLogEntry) bool {
		return entry.StatusCode >= min && entry.StatusCode <= max
	}
}

// FilterErrors returns a filter that only logs errors (status >= 400).
func FilterErrors() AccessLogFilter {
	return func(entry *AccessLogEntry) bool {
		return entry.StatusCode >= 400
	}
}

// FilterByLatency returns a filter that only logs entries with latency above threshold.
func FilterByLatency(threshold time.Duration) AccessLogFilter {
	return func(entry *AccessLogEntry) bool {
		return entry.Latency >= threshold
	}
}

// FilterByPath returns a filter that only logs entries matching the path prefix.
func FilterByPath(prefix string) AccessLogFilter {
	return func(entry *AccessLogEntry) bool {
		if len(entry.Path) < len(prefix) {
			return false
		}
		return entry.Path[:len(prefix)] == prefix
	}
}

// FilterExcludePath returns a filter that excludes entries matching the path prefix.
func FilterExcludePath(prefix string) AccessLogFilter {
	return func(entry *AccessLogEntry) bool {
		if len(entry.Path) < len(prefix) {
			return true
		}
		return entry.Path[:len(prefix)] != prefix
	}
}

// SampledFilter returns a filter that samples a percentage of entries.
func SampledFilter(rate float64) AccessLogFilter {
	var counter uint64
	interval := uint64(1.0 / rate)
	if interval == 0 {
		interval = 1
	}

	var mu sync.Mutex
	return func(entry *AccessLogEntry) bool {
		mu.Lock()
		counter++
		sample := counter%interval == 0
		mu.Unlock()
		return sample
	}
}

// BufferedAccessLogger buffers entries and writes in batches.
type BufferedAccessLogger struct {
	*AccessLogger
	entries   []*AccessLogEntry
	maxSize   int
	flushTime time.Duration
	mu        sync.Mutex
	ticker    *time.Ticker
	done      chan struct{}
}

// NewBufferedAccessLogger creates a buffered access logger.
func NewBufferedAccessLogger(config *AccessLogConfig, batchSize int, flushInterval time.Duration) *BufferedAccessLogger {
	bl := &BufferedAccessLogger{
		AccessLogger: NewAccessLogger(config),
		entries:      make([]*AccessLogEntry, 0, batchSize),
		maxSize:      batchSize,
		flushTime:    flushInterval,
		ticker:       time.NewTicker(flushInterval),
		done:         make(chan struct{}),
	}

	go bl.flushLoop()

	return bl
}

// Log adds an entry to the buffer.
func (bl *BufferedAccessLogger) Log(entry *AccessLogEntry) {
	bl.mu.Lock()
	bl.entries = append(bl.entries, entry)
	shouldFlush := len(bl.entries) >= bl.maxSize
	bl.mu.Unlock()

	if shouldFlush {
		bl.Flush()
	}
}

// Flush writes all buffered entries.
func (bl *BufferedAccessLogger) Flush() {
	bl.mu.Lock()
	entries := bl.entries
	bl.entries = make([]*AccessLogEntry, 0, bl.maxSize)
	bl.mu.Unlock()

	for _, entry := range entries {
		bl.AccessLogger.Log(entry)
	}
}

// Close flushes and closes the logger.
func (bl *BufferedAccessLogger) Close() error {
	close(bl.done)
	bl.ticker.Stop()
	bl.Flush()
	return bl.AccessLogger.Close()
}

func (bl *BufferedAccessLogger) flushLoop() {
	for {
		select {
		case <-bl.done:
			return
		case <-bl.ticker.C:
			bl.Flush()
		}
	}
}
