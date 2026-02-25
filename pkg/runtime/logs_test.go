// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

//go:build linux

package runtime

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// LogOptions Tests
// =============================================================================

func TestLogOptions_Fields(t *testing.T) {
	since := time.Now().Add(-1 * time.Hour)
	until := time.Now()

	opts := &LogOptions{
		Follow:     true,
		Timestamps: true,
		Since:      since,
		Until:      until,
		Tail:       100,
		Stdout:     true,
		Stderr:     true,
	}

	if !opts.Follow {
		t.Error("Expected Follow to be true")
	}

	if !opts.Timestamps {
		t.Error("Expected Timestamps to be true")
	}

	if opts.Tail != 100 {
		t.Errorf("Expected Tail 100, got %d", opts.Tail)
	}

	if !opts.Stdout {
		t.Error("Expected Stdout to be true")
	}

	if !opts.Stderr {
		t.Error("Expected Stderr to be true")
	}

	if opts.Since != since {
		t.Error("Expected Since to match")
	}

	if opts.Until != until {
		t.Error("Expected Until to match")
	}
}

func TestLogOptions_Defaults(t *testing.T) {
	opts := &LogOptions{}

	if opts.Follow {
		t.Error("Expected Follow to be false by default")
	}

	if opts.Tail != 0 {
		t.Errorf("Expected Tail 0 by default, got %d", opts.Tail)
	}
}

// =============================================================================
// LogEntry Tests
// =============================================================================

func TestLogEntry_Fields(t *testing.T) {
	now := time.Now()
	entry := &LogEntry{
		Stream:  StreamStdout,
		Time:    now,
		Line:    []byte("test log line"),
		Partial: false,
	}

	if entry.Stream != StreamStdout {
		t.Errorf("Expected stream stdout, got %d", entry.Stream)
	}

	if entry.Time != now {
		t.Error("Expected time to match")
	}

	if string(entry.Line) != "test log line" {
		t.Errorf("Expected line 'test log line', got '%s'", string(entry.Line))
	}

	if entry.Partial {
		t.Error("Expected Partial to be false")
	}
}

func TestLogEntry_Partial(t *testing.T) {
	entry := &LogEntry{
		Stream:  StreamStderr,
		Line:    []byte("partial line"),
		Partial: true,
	}

	if !entry.Partial {
		t.Error("Expected Partial to be true")
	}
}

// =============================================================================
// StreamType Tests
// =============================================================================

func TestStreamType_Values(t *testing.T) {
	if StreamStdin != 0 {
		t.Errorf("Expected StreamStdin to be 0, got %d", StreamStdin)
	}

	if StreamStdout != 1 {
		t.Errorf("Expected StreamStdout to be 1, got %d", StreamStdout)
	}

	if StreamStderr != 2 {
		t.Errorf("Expected StreamStderr to be 2, got %d", StreamStderr)
	}
}

func TestStreamType_String(t *testing.T) {
	tests := []struct {
		stream   StreamType
		expected string
	}{
		{StreamStdin, "stdin"},
		{StreamStdout, "stdout"},
		{StreamStderr, "stderr"},
		{StreamType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.stream.String() != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, tt.stream.String())
			}
		})
	}
}

// =============================================================================
// logStream Tests
// =============================================================================

func TestLogStream_Fields(t *testing.T) {
	stream := &logStream{
		containerID: "container-123",
		options: &LogOptions{
			Follow: true,
			Tail:   50,
		},
		closed:  false,
		closeCh: make(chan struct{}),
	}

	if stream.containerID != "container-123" {
		t.Errorf("Expected containerID 'container-123', got '%s'", stream.containerID)
	}

	if !stream.options.Follow {
		t.Error("Expected options.Follow to be true")
	}

	if stream.options.Tail != 50 {
		t.Errorf("Expected options.Tail 50, got %d", stream.options.Tail)
	}

	if stream.closed {
		t.Error("Expected closed to be false")
	}
}

func TestLogStream_Close(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	// Create a log file
	if err := os.WriteFile(logPath, []byte("test log\n"), 0644); err != nil {
		t.Fatalf("Failed to create log file: %v", err)
	}

	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}

	stream := &logStream{
		containerID: "container-123",
		options:     &LogOptions{},
		file:        file,
		closed:      false,
		closeCh:     make(chan struct{}),
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !stream.closed {
		t.Error("Expected stream to be marked as closed")
	}

	// Second close should be no-op
	if err := stream.Close(); err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}
}

// =============================================================================
// emptyReader Tests
// =============================================================================

func TestEmptyReader_Read(t *testing.T) {
	r := &emptyReader{}
	buf := make([]byte, 100)

	n, err := r.Read(buf)
	if n != 0 {
		t.Errorf("Expected 0 bytes, got %d", n)
	}

	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
}

func TestEmptyReader_Close(t *testing.T) {
	r := &emptyReader{}

	if err := r.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// =============================================================================
// LogWriter Tests
// =============================================================================

func TestNewLogWriter(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	writer, err := NewLogWriter(logPath, StreamStdout, 10*1024*1024, 5)
	if err != nil {
		t.Fatalf("NewLogWriter failed: %v", err)
	}
	defer writer.Close()

	if writer.path != logPath {
		t.Errorf("Expected path '%s', got '%s'", logPath, writer.path)
	}

	if writer.stream != StreamStdout {
		t.Errorf("Expected stream stdout, got %d", writer.stream)
	}

	if writer.maxSize != 10*1024*1024 {
		t.Errorf("Expected maxSize 10MB, got %d", writer.maxSize)
	}

	if writer.maxFiles != 5 {
		t.Errorf("Expected maxFiles 5, got %d", writer.maxFiles)
	}
}

func TestNewLogWriter_CreatesDirectory(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "subdir", "test.log")

	writer, err := NewLogWriter(logPath, StreamStdout, 10*1024*1024, 5)
	if err != nil {
		t.Fatalf("NewLogWriter failed: %v", err)
	}
	defer writer.Close()

	// Verify directory was created
	if _, err := os.Stat(filepath.Dir(logPath)); os.IsNotExist(err) {
		t.Error("Expected directory to be created")
	}
}

func TestNewLogWriter_Defaults(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	writer, err := NewLogWriter(logPath, StreamStdout, 0, 0)
	if err != nil {
		t.Fatalf("NewLogWriter failed: %v", err)
	}
	defer writer.Close()

	// Should use defaults
	if writer.maxSize != 10*1024*1024 {
		t.Errorf("Expected default maxSize 10MB, got %d", writer.maxSize)
	}

	if writer.maxFiles != 5 {
		t.Errorf("Expected default maxFiles 5, got %d", writer.maxFiles)
	}
}

func TestLogWriter_Write(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	writer, err := NewLogWriter(logPath, StreamStdout, 10*1024*1024, 5)
	if err != nil {
		t.Fatalf("NewLogWriter failed: %v", err)
	}
	defer writer.Close()

	data := []byte("test log line\n")
	n, err := writer.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(data) {
		t.Errorf("Expected %d bytes written, got %d", len(data), n)
	}

	// Verify file content
	writer.Close()
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if string(content) != string(data) {
		t.Errorf("Expected '%s', got '%s'", string(data), string(content))
	}
}

func TestLogWriter_WriteEntry(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	writer, err := NewLogWriter(logPath, StreamStdout, 10*1024*1024, 5)
	if err != nil {
		t.Fatalf("NewLogWriter failed: %v", err)
	}
	defer writer.Close()

	now := time.Now()
	err = writer.WriteEntry([]byte("test message"), now)
	if err != nil {
		t.Fatalf("WriteEntry failed: %v", err)
	}

	// Verify file content
	writer.Close()
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Should be JSON formatted
	if !strings.Contains(string(content), `"log":"test message\n"`) {
		t.Errorf("Expected JSON log entry, got '%s'", string(content))
	}

	if !strings.Contains(string(content), `"stream":"stdout"`) {
		t.Errorf("Expected stream stdout, got '%s'", string(content))
	}
}

func TestLogWriter_Close(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	writer, err := NewLogWriter(logPath, StreamStdout, 10*1024*1024, 5)
	if err != nil {
		t.Fatalf("NewLogWriter failed: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Writing after close should fail
	_, err = writer.Write([]byte("test"))
	if err == nil {
		t.Error("Expected error when writing after close")
	}
}

// =============================================================================
// formatLogEntry Tests
// =============================================================================

func TestFormatLogEntry(t *testing.T) {
	now := time.Date(2026, 1, 26, 12, 0, 0, 0, time.UTC)

	entry := formatLogEntry(StreamStdout, []byte("test message"), now)

	if !strings.Contains(string(entry), `"log":"test message\n"`) {
		t.Errorf("Expected log content, got '%s'", string(entry))
	}

	if !strings.Contains(string(entry), `"stream":"stdout"`) {
		t.Errorf("Expected stream stdout, got '%s'", string(entry))
	}

	if !strings.Contains(string(entry), `"time":"`) {
		t.Errorf("Expected time field, got '%s'", string(entry))
	}
}

func TestFormatLogEntry_Stderr(t *testing.T) {
	entry := formatLogEntry(StreamStderr, []byte("error message"), time.Now())

	if !strings.Contains(string(entry), `"stream":"stderr"`) {
		t.Errorf("Expected stream stderr, got '%s'", string(entry))
	}
}

// =============================================================================
// escapeJSON Tests
// =============================================================================

func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{"plain text", []byte("hello world"), "hello world"},
		{"quotes", []byte(`say "hello"`), `say \"hello\"`},
		{"backslash", []byte(`path\to\file`), `path\\to\\file`},
		{"newline", []byte("line1\nline2"), `line1\nline2`},
		{"tab", []byte("col1\tcol2"), `col1\tcol2`},
		{"carriage return", []byte("line1\rline2"), `line1\rline2`},
		{"control char", []byte{0x01, 0x02}, `\u0001\u0002`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeJSON(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// MultiplexedLogReader Tests
// =============================================================================

func TestNewMultiplexedLogReader(t *testing.T) {
	stdout := bytes.NewBufferString("stdout data")
	stderr := bytes.NewBufferString("stderr data")

	reader := NewMultiplexedLogReader(stdout, stderr)

	if reader == nil {
		t.Fatal("Expected non-nil reader")
	}

	if len(reader.readers) != 2 {
		t.Errorf("Expected 2 readers, got %d", len(reader.readers))
	}
}

func TestNewMultiplexedLogReader_NilReaders(t *testing.T) {
	reader := NewMultiplexedLogReader(nil, nil)

	if reader == nil {
		t.Fatal("Expected non-nil reader")
	}

	if len(reader.readers) != 0 {
		t.Errorf("Expected 0 readers, got %d", len(reader.readers))
	}
}

func TestNewMultiplexedLogReader_OnlyStdout(t *testing.T) {
	stdout := bytes.NewBufferString("stdout data")

	reader := NewMultiplexedLogReader(stdout, nil)

	if len(reader.readers) != 1 {
		t.Errorf("Expected 1 reader, got %d", len(reader.readers))
	}

	if _, ok := reader.readers[StreamStdout]; !ok {
		t.Error("Expected stdout reader")
	}
}

func TestMultiplexedLogReader_Read(t *testing.T) {
	stdout := bytes.NewBufferString("stdout data")

	reader := NewMultiplexedLogReader(stdout, nil)

	buf := make([]byte, 1024)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}

	if n < 8 {
		t.Errorf("Expected at least 8 bytes (header), got %d", n)
	}

	// Verify header format
	streamType := buf[0]
	if streamType != byte(StreamStdout) {
		t.Errorf("Expected stream type stdout, got %d", streamType)
	}

	size := binary.BigEndian.Uint32(buf[4:8])
	if size == 0 {
		t.Error("Expected non-zero size in header")
	}
}

func TestMultiplexedLogReader_ReadBothStreams(t *testing.T) {
	stdout := bytes.NewBufferString("stdout")
	stderr := bytes.NewBufferString("stderr")

	reader := NewMultiplexedLogReader(stdout, stderr)

	// Read multiple times to get both streams
	buf := make([]byte, 1024)
	for i := 0; i < 4; i++ {
		n, err := reader.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if n == 0 {
			continue
		}
	}
}

func TestMultiplexedLogReader_EOF(t *testing.T) {
	reader := NewMultiplexedLogReader(nil, nil)

	buf := make([]byte, 1024)
	_, err := reader.Read(buf)

	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
}

// =============================================================================
// LogCopier Tests
// =============================================================================

func TestNewLogCopier(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	stdout := bytes.NewBufferString("stdout data\n")
	stderr := bytes.NewBufferString("stderr data\n")

	copier, err := NewLogCopier(stdout, stderr, logPath)
	if err != nil {
		t.Fatalf("NewLogCopier failed: %v", err)
	}

	if copier.stdout == nil {
		t.Error("Expected stdout to be set")
	}

	if copier.stderr == nil {
		t.Error("Expected stderr to be set")
	}

	if copier.writer == nil {
		t.Error("Expected writer to be set")
	}

	copier.Stop()
}

func TestLogCopier_StartAndStop(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	stdout := bytes.NewBufferString("stdout line\n")
	stderr := bytes.NewBufferString("stderr line\n")

	copier, err := NewLogCopier(stdout, stderr, logPath)
	if err != nil {
		t.Fatalf("NewLogCopier failed: %v", err)
	}

	copier.Start()

	// Give it time to process
	time.Sleep(100 * time.Millisecond)

	copier.Stop()

	// Verify log file was created and has content
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("Expected log file to have content")
	}
}

func TestLogCopier_Wait(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	// Use empty readers that return EOF immediately
	stdout := bytes.NewBufferString("")
	stderr := bytes.NewBufferString("")

	copier, err := NewLogCopier(stdout, stderr, logPath)
	if err != nil {
		t.Fatalf("NewLogCopier failed: %v", err)
	}

	copier.Start()
	copier.Wait()

	// Should complete without hanging
	copier.Stop()
}

// =============================================================================
// GetContainerLogs Tests
// =============================================================================

func TestGetContainerLogs_NotFound(t *testing.T) {
	root := testTempDir(t)
	rt := setupTestRuntime(t, root)
	defer rt.Close()

	ctx := context.Background()

	_, err := rt.GetContainerLogs(ctx, "nonexistent", nil)
	if err != ErrContainerNotFound {
		t.Errorf("Expected ErrContainerNotFound, got %v", err)
	}
}

// =============================================================================
// Concurrent Log Operations Tests
// =============================================================================

func TestLogWriter_ConcurrentWrites(t *testing.T) {
	tmpDir := testTempDir(t)
	logPath := filepath.Join(tmpDir, "test.log")

	writer, err := NewLogWriter(logPath, StreamStdout, 10*1024*1024, 5)
	if err != nil {
		t.Fatalf("NewLogWriter failed: %v", err)
	}
	defer writer.Close()

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := []byte("concurrent write test\n")
			_, err := writer.Write(data)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("Concurrent write error: %v", err)
	}
}

func TestMultiplexedLogReader_ConcurrentRead(t *testing.T) {
	stdout := bytes.NewBufferString("stdout data for concurrent test")
	stderr := bytes.NewBufferString("stderr data for concurrent test")

	reader := NewMultiplexedLogReader(stdout, stderr)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 1024)
			reader.Read(buf)
		}()
	}

	wg.Wait()
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkLogWriter_Write(b *testing.B) {
	tmpDir := b.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	writer, err := NewLogWriter(logPath, StreamStdout, 100*1024*1024, 5)
	if err != nil {
		b.Fatalf("NewLogWriter failed: %v", err)
	}
	defer writer.Close()

	data := []byte("benchmark log line content\n")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.Write(data)
	}
}

func BenchmarkLogWriter_WriteEntry(b *testing.B) {
	tmpDir := b.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	writer, err := NewLogWriter(logPath, StreamStdout, 100*1024*1024, 5)
	if err != nil {
		b.Fatalf("NewLogWriter failed: %v", err)
	}
	defer writer.Close()

	line := []byte("benchmark log line content")
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.WriteEntry(line, now)
	}
}

func BenchmarkFormatLogEntry(b *testing.B) {
	line := []byte("benchmark log line content")
	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatLogEntry(StreamStdout, line, now)
	}
}

func BenchmarkEscapeJSON(b *testing.B) {
	data := []byte(`benchmark "test" with\nspecial chars\t`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		escapeJSON(data)
	}
}

func BenchmarkMultiplexedLogReader_Read(b *testing.B) {
	stdout := bytes.NewReader(make([]byte, 1024*1024))
	stderr := bytes.NewReader(make([]byte, 1024*1024))

	reader := NewMultiplexedLogReader(stdout, stderr)
	buf := make([]byte, 4096)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader.Read(buf)
	}
}

func BenchmarkStreamType_String(b *testing.B) {
	streams := []StreamType{StreamStdin, StreamStdout, StreamStderr}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = streams[i%3].String()
	}
}
