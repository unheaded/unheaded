// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package runtime

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogOptions defines options for retrieving logs.
type LogOptions struct {
	// Follow follows the log output.
	Follow bool
	// Timestamps includes timestamps.
	Timestamps bool
	// Since shows logs since this time.
	Since time.Time
	// Until shows logs until this time.
	Until time.Time
	// Tail shows only the last N lines.
	Tail int64
	// Stdout includes stdout.
	Stdout bool
	// Stderr includes stderr.
	Stderr bool
}

// logStream represents a log streaming session.
type logStream struct {
	mu sync.Mutex

	containerID string
	options     *LogOptions
	file        *os.File
	reader      *bufio.Reader
	closed      bool
	closeCh     chan struct{}
}

// LogEntry represents a single log entry.
type LogEntry struct {
	// Stream is the stream type (stdout or stderr).
	Stream StreamType
	// Time is the timestamp of the log entry.
	Time time.Time
	// Line is the log line content.
	Line []byte
	// Partial indicates if this is a partial line.
	Partial bool
}

// StreamType represents the type of log stream.
type StreamType int

const (
	// StreamStdin is standard input.
	StreamStdin StreamType = iota
	// StreamStdout is standard output.
	StreamStdout
	// StreamStderr is standard error.
	StreamStderr
)

// String returns the string representation of StreamType.
func (s StreamType) String() string {
	switch s {
	case StreamStdin:
		return "stdin"
	case StreamStdout:
		return "stdout"
	case StreamStderr:
		return "stderr"
	default:
		return "unknown"
	}
}

// GetContainerLogs returns a reader for container logs.
func (r *DefaultRuntime) GetContainerLogs(ctx context.Context, containerID string, options *LogOptions) (io.ReadCloser, error) {
	r.mu.RLock()
	c, exists := r.containers[containerID]
	r.mu.RUnlock()

	if !exists {
		return nil, ErrContainerNotFound
	}

	c.mu.RLock()
	logPath := c.logPath
	c.mu.RUnlock()

	if options == nil {
		options = &LogOptions{
			Stdout: true,
			Stderr: true,
		}
	}

	// Open log file
	file, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty reader if no logs yet
			return &emptyReader{}, nil
		}
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	stream := &logStream{
		containerID: containerID,
		options:     options,
		file:        file,
		reader:      bufio.NewReader(file),
		closeCh:     make(chan struct{}),
	}

	// If tailing, seek to the appropriate position
	if options.Tail > 0 {
		if err := stream.seekToTail(options.Tail); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("failed to seek to tail: %w", err)
		}
	}

	// Store stream
	streamID := fmt.Sprintf("log-%s-%d", containerID, time.Now().UnixNano())
	r.mu.Lock()
	r.logStreams[streamID] = stream
	r.mu.Unlock()

	// If following, start background reader
	if options.Follow {
		go stream.followLogs(ctx, r, containerID)
	}

	return stream, nil
}

// Read implements io.Reader.
func (ls *logStream) Read(p []byte) (int, error) {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.closed {
		return 0, io.EOF
	}

	// Read from file
	n, err := ls.reader.Read(p)
	if err == io.EOF && ls.options.Follow {
		// Wait for more data
		ls.mu.Unlock()
		select {
		case <-ls.closeCh:
			ls.mu.Lock()
			return 0, io.EOF
		case <-time.After(100 * time.Millisecond):
			ls.mu.Lock()
			return 0, nil // Return 0 without error to continue following
		}
	}

	return n, err
}

// Close closes the log stream.
func (ls *logStream) Close() error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	if ls.closed {
		return nil
	}

	ls.closed = true
	close(ls.closeCh)

	if ls.file != nil {
		return ls.file.Close()
	}

	return nil
}

// seekToTail seeks to show only the last N lines.
func (ls *logStream) seekToTail(n int64) error {
	// Get file size
	stat, err := ls.file.Stat()
	if err != nil {
		return err
	}

	if stat.Size() == 0 {
		return nil
	}

	// Read from end to find line positions
	bufSize := int64(8192)
	if bufSize > stat.Size() {
		bufSize = stat.Size()
	}

	var lines int64
	pos := stat.Size()
	buf := make([]byte, bufSize)

	for pos > 0 && lines <= n {
		readSize := bufSize
		if pos < readSize {
			readSize = pos
		}
		pos -= readSize

		_, _ = ls.file.Seek(pos, io.SeekStart)
		bytesRead, err := ls.file.Read(buf[:readSize])
		if err != nil && err != io.EOF {
			return err
		}

		// Count newlines
		for i := bytesRead - 1; i >= 0; i-- {
			if buf[i] == '\n' {
				lines++
				if lines > n {
					// Found the position
					_, _ = ls.file.Seek(pos+int64(i)+1, io.SeekStart)
					ls.reader.Reset(ls.file)
					return nil
				}
			}
		}
	}

	// Not enough lines, start from beginning
	_, _ = ls.file.Seek(0, io.SeekStart)
	ls.reader.Reset(ls.file)
	return nil
}

// followLogs follows log output.
func (ls *logStream) followLogs(ctx context.Context, r *DefaultRuntime, containerID string) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastSize int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-ls.closeCh:
			return
		case <-ticker.C:
			// Check if container is still running
			r.mu.RLock()
			c, exists := r.containers[containerID]
			r.mu.RUnlock()

			if !exists {
				return
			}

			c.mu.RLock()
			state := c.info.State
			c.mu.RUnlock()

			if state != ContainerStateRunning {
				// Container stopped — exit the follower. (We previously read
				// the trailing file size into lastSize here, but lastSize is
				// scoped to this loop and never observed externally — the
				// drain semantics will be added when followLogs gets a real
				// consumer.)
				return
			}

			// Check for new data
			ls.mu.Lock()
			stat, err := ls.file.Stat()
			if err == nil && stat.Size() > lastSize {
				lastSize = stat.Size()
			}
			ls.mu.Unlock()
		}
	}
}

// emptyReader is a reader that always returns EOF.
type emptyReader struct{}

func (e *emptyReader) Read(p []byte) (int, error) {
	return 0, io.EOF
}

func (e *emptyReader) Close() error {
	return nil
}

// LogWriter writes logs in the Docker JSON log format.
type LogWriter struct {
	mu sync.Mutex

	path        string
	file        *os.File
	stream      StreamType
	maxSize     int64
	maxFiles    int
	currentSize int64
}

// NewLogWriter creates a new log writer.
func NewLogWriter(path string, stream StreamType, maxSize int64, maxFiles int) (*LogWriter, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to stat log file: %w", err)
	}

	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10MB default
	}
	if maxFiles <= 0 {
		maxFiles = 5
	}

	return &LogWriter{
		path:        path,
		file:        file,
		stream:      stream,
		maxSize:     maxSize,
		maxFiles:    maxFiles,
		currentSize: stat.Size(),
	}, nil
}

// Write writes a log entry.
func (lw *LogWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	// Check if rotation needed
	if lw.currentSize+int64(len(p)) > lw.maxSize {
		if err := lw.rotate(); err != nil {
			return 0, fmt.Errorf("failed to rotate log: %w", err)
		}
	}

	n, err := lw.file.Write(p)
	if err != nil {
		return n, err
	}

	lw.currentSize += int64(n)
	return n, nil
}

// WriteEntry writes a log entry with timestamp.
func (lw *LogWriter) WriteEntry(line []byte, timestamp time.Time) error {
	entry := formatLogEntry(lw.stream, line, timestamp)
	_, err := lw.Write(entry)
	return err
}

// Close closes the log writer.
func (lw *LogWriter) Close() error {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.file != nil {
		return lw.file.Close()
	}
	return nil
}

// rotate rotates the log file.
func (lw *LogWriter) rotate() error {
	// Close current file
	if err := lw.file.Close(); err != nil {
		return err
	}

	// Rotate existing files
	for i := lw.maxFiles - 1; i > 0; i-- {
		oldPath := fmt.Sprintf("%s.%d", lw.path, i)
		newPath := fmt.Sprintf("%s.%d", lw.path, i+1)
		_ = os.Rename(oldPath, newPath)
	}

	// Rename current to .1
	_ = os.Rename(lw.path, lw.path+".1")

	// Create new file
	file, err := os.OpenFile(lw.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	lw.file = file
	lw.currentSize = 0

	// Remove oldest files
	for i := lw.maxFiles; ; i++ {
		path := fmt.Sprintf("%s.%d", lw.path, i)
		if err := os.Remove(path); err != nil {
			break // No more files
		}
	}

	return nil
}

// formatLogEntry formats a log entry in JSON format.
func formatLogEntry(stream StreamType, line []byte, timestamp time.Time) []byte {
	// Format: {"log":"<line>","stream":"<stream>","time":"<timestamp>"}
	streamName := "stdout"
	if stream == StreamStderr {
		streamName = "stderr"
	}

	// Escape JSON special characters in the line
	escaped := escapeJSON(line)

	return []byte(fmt.Sprintf(`{"log":"%s\n","stream":"%s","time":"%s"}`+"\n",
		escaped, streamName, timestamp.Format(time.RFC3339Nano)))
}

// escapeJSON escapes special characters for JSON strings.
func escapeJSON(s []byte) string {
	result := make([]byte, 0, len(s))
	for _, b := range s {
		switch b {
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
			if b < 32 {
				result = append(result, fmt.Sprintf("\\u%04x", b)...)
			} else {
				result = append(result, b)
			}
		}
	}
	return string(result)
}

// MultiplexedLogReader reads multiplexed log output (combined stdout/stderr).
type MultiplexedLogReader struct {
	mu      sync.Mutex
	readers map[StreamType]io.Reader
	current StreamType
	buffer  []byte
	pos     int
}

// NewMultiplexedLogReader creates a reader that multiplexes stdout and stderr.
func NewMultiplexedLogReader(stdout, stderr io.Reader) *MultiplexedLogReader {
	readers := make(map[StreamType]io.Reader)
	if stdout != nil {
		readers[StreamStdout] = stdout
	}
	if stderr != nil {
		readers[StreamStderr] = stderr
	}

	return &MultiplexedLogReader{
		readers: readers,
		current: StreamStdout,
		buffer:  make([]byte, 0),
	}
}

// Read reads multiplexed output with stream headers.
// Format: [stream_type(1 byte)][reserved(3 bytes)][size(4 bytes)][data]
func (m *MultiplexedLogReader) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.readLocked(p)
}

// readLocked performs the read while the mutex is held. Uses a loop instead of
// recursion to avoid deadlock when switching streams after EOF.
func (m *MultiplexedLogReader) readLocked(p []byte) (int, error) {
	for {
		// First try to read any buffered data
		if m.pos < len(m.buffer) {
			n := copy(p, m.buffer[m.pos:])
			m.pos += n
			return n, nil
		}

		// Read from current stream
		reader, ok := m.readers[m.current]
		if !ok {
			// Try other stream
			if m.current == StreamStdout {
				m.current = StreamStderr
			} else {
				m.current = StreamStdout
			}
			reader, ok = m.readers[m.current]
			if !ok {
				return 0, io.EOF
			}
		}

		// Read data
		data := make([]byte, 4096)
		n, err := reader.Read(data)
		if err != nil && err != io.EOF {
			return 0, err
		}
		if n == 0 && err == io.EOF {
			// Remove this reader
			delete(m.readers, m.current)
			if len(m.readers) == 0 {
				return 0, io.EOF
			}
			continue // Try other stream
		}

		// Build multiplexed frame
		// Header: [stream_type][0][0][0][size_big_endian_32]
		header := make([]byte, 8)
		header[0] = byte(m.current)
		binary.BigEndian.PutUint32(header[4:], uint32(n))

		m.buffer = append(header, data[:n]...)
		m.pos = 0

		copied := copy(p, m.buffer)
		m.pos = copied

		// Alternate streams for fairness
		if m.current == StreamStdout {
			m.current = StreamStderr
		} else {
			m.current = StreamStdout
		}

		return copied, nil
	}
}

// LogCopier copies logs from container streams to log files.
type LogCopier struct {
	stdout io.Reader
	stderr io.Reader
	writer *LogWriter
	closed chan struct{}
	wg     sync.WaitGroup
}

// NewLogCopier creates a log copier.
func NewLogCopier(stdout, stderr io.Reader, logPath string) (*LogCopier, error) {
	writer, err := NewLogWriter(logPath, StreamStdout, 10*1024*1024, 5)
	if err != nil {
		return nil, err
	}

	return &LogCopier{
		stdout: stdout,
		stderr: stderr,
		writer: writer,
		closed: make(chan struct{}),
	}, nil
}

// Start starts copying logs.
func (lc *LogCopier) Start() {
	if lc.stdout != nil {
		lc.wg.Add(1)
		go lc.copy(lc.stdout, StreamStdout)
	}

	if lc.stderr != nil {
		lc.wg.Add(1)
		go lc.copy(lc.stderr, StreamStderr)
	}
}

// copy copies from a reader to the log writer.
func (lc *LogCopier) copy(r io.Reader, stream StreamType) {
	defer lc.wg.Done()

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // Up to 1MB lines

	for scanner.Scan() {
		select {
		case <-lc.closed:
			return
		default:
			_ = lc.writer.WriteEntry(scanner.Bytes(), time.Now())
		}
	}
}

// Stop stops copying logs.
func (lc *LogCopier) Stop() {
	close(lc.closed)
	lc.wg.Wait()
	_ = lc.writer.Close()
}

// Wait waits for copying to complete.
func (lc *LogCopier) Wait() {
	lc.wg.Wait()
}
