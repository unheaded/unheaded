// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logger

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBasicLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.Info().Str("service", "wotan").Int("port", 8080).Msg("starting server")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["level"] != "info" {
		t.Errorf("expected level 'info', got '%v'", entry["level"])
	}
	if entry["service"] != "wotan" {
		t.Errorf("expected service 'wotan', got '%v'", entry["service"])
	}
	if entry["port"] != float64(8080) {
		t.Errorf("expected port 8080, got '%v'", entry["port"])
	}
	if entry["message"] != "starting server" {
		t.Errorf("expected message 'starting server', got '%v'", entry["message"])
	}
	if _, ok := entry["time"]; !ok {
		t.Error("expected timestamp field")
	}
}

func TestErrorLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	err := errors.New("file not found")
	log.Error().Err(err).Str("path", "/tmp/test").Msg("file error")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["level"] != "error" {
		t.Errorf("expected level 'error', got '%v'", entry["level"])
	}
	if entry["error"] != "file not found" {
		t.Errorf("expected error 'file not found', got '%v'", entry["error"])
	}
	if entry["path"] != "/tmp/test" {
		t.Errorf("expected path '/tmp/test', got '%v'", entry["path"])
	}
}

func TestContextLogging(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	ctx := log.With().Str("request_id", "abc123").Int("user_id", 42).Logger()
	ctx.Info().Msg("handling request")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["request_id"] != "abc123" {
		t.Errorf("expected request_id 'abc123', got '%v'", entry["request_id"])
	}
	if entry["user_id"] != float64(42) {
		t.Errorf("expected user_id 42, got '%v'", entry["user_id"])
	}
}

func TestLogLevels(t *testing.T) {
	levels := []struct {
		name  string
		level Level
		fn    func(*Logger) *Event
	}{
		{"trace", TraceLevel, func(l *Logger) *Event { return l.Trace() }},
		{"debug", DebugLevel, func(l *Logger) *Event { return l.Debug() }},
		{"info", InfoLevel, func(l *Logger) *Event { return l.Info() }},
		{"warn", WarnLevel, func(l *Logger) *Event { return l.Warn() }},
		{"error", ErrorLevel, func(l *Logger) *Event { return l.Error() }},
	}

	for _, tc := range levels {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := New(buf)
			log.SetLevel(TraceLevel) // Enable all levels

			tc.fn(log).Msg("test message")

			var entry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("failed to parse log entry: %v", err)
			}

			if entry["level"] != tc.name {
				t.Errorf("expected level '%s', got '%v'", tc.name, entry["level"])
			}
		})
	}
}

func TestLevelFiltering(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.SetLevel(WarnLevel)

	log.Debug().Msg("debug message")
	log.Info().Msg("info message")

	if buf.Len() > 0 {
		t.Error("expected no output for filtered levels")
	}

	log.Warn().Msg("warn message")
	if buf.Len() == 0 {
		t.Error("expected output for warn level")
	}
}

func TestCallerInfo(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.EnableCaller()

	log.Info().Msg("test with caller")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	caller, ok := entry["caller"].(string)
	if !ok {
		t.Error("expected caller field")
	}
	// Check that caller contains a Go source file with line number
	if !strings.Contains(caller, ".go:") {
		t.Errorf("expected caller to contain '.go:', got '%s'", caller)
	}
}

func TestCallerOnEvent(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	// Use event-level Caller() which allows explicit skip
	log.Info().Caller(1).Msg("test with explicit caller")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	caller, ok := entry["caller"].(string)
	if !ok {
		t.Error("expected caller field")
	}
	if !strings.Contains(caller, "logger_test.go") {
		t.Errorf("expected caller to contain 'logger_test.go', got '%s'", caller)
	}
}

func TestAllFieldTypes(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	now := time.Now()
	dur := 5 * time.Second

	log.Info().
		Str("string", "value").
		Strs("strings", []string{"a", "b", "c"}).
		Bytes("bytes", []byte("hello")).
		Int("int", -42).
		Int8("int8", int8(-8)).
		Int16("int16", int16(-16)).
		Int32("int32", int32(-32)).
		Int64("int64", int64(-64)).
		Uint("uint", 42).
		Uint8("uint8", uint8(8)).
		Uint16("uint16", uint16(16)).
		Uint32("uint32", uint32(32)).
		Uint64("uint64", uint64(64)).
		Float32("float32", 3.14).
		Float64("float64", 2.718281828).
		Bool("bool", true).
		Time("time", now).
		Dur("duration", dur).
		Interface("interface", map[string]int{"key": 123}).
		Msg("all types")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["string"] != "value" {
		t.Errorf("string field mismatch: %v", entry["string"])
	}
	if entry["int"] != float64(-42) {
		t.Errorf("int field mismatch: %v", entry["int"])
	}
	if entry["uint"] != float64(42) {
		t.Errorf("uint field mismatch: %v", entry["uint"])
	}
	if entry["bool"] != true {
		t.Errorf("bool field mismatch: %v", entry["bool"])
	}
}

func TestConsoleMode(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.ConsoleMode()
	log.DisableColor()

	log.Info().Str("key", "value").Msg("console test")

	output := buf.String()
	if !strings.Contains(output, "info") {
		t.Error("expected console output to contain level")
	}
	if !strings.Contains(output, "console test") {
		t.Error("expected console output to contain message")
	}
	if !strings.Contains(output, "key=value") {
		t.Error("expected console output to contain field")
	}
}

func TestSampling(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.SetSampler(&BasicSampler{N: 10})

	for i := 0; i < 100; i++ {
		log.Info().Int("i", i).Msg("sampled message")
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 sampled messages, got %d", len(lines))
	}
}

func TestHooks(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	hookCalled := false
	log.AddHook(HookFunc(func(entry *Entry) error {
		hookCalled = true
		if entry.Level != InfoLevel {
			t.Errorf("expected level info, got %v", entry.Level)
		}
		if entry.Message != "hook test" {
			t.Errorf("expected message 'hook test', got '%s'", entry.Message)
		}
		return nil
	}))

	log.Info().Msg("hook test")

	if !hookCalled {
		t.Error("hook was not called")
	}
}

func TestAsyncWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := NewAsyncWriter(buf, 100)
	log := New(writer)

	for i := 0; i < 10; i++ {
		log.Info().Int("i", i).Msg("async message")
	}

	// Give async writer time to process.
	time.Sleep(100 * time.Millisecond)

	writer.Close()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 10 {
		t.Errorf("expected 10 messages, got %d", len(lines))
	}
}

func TestMultiWriter(t *testing.T) {
	buf1 := &bytes.Buffer{}
	buf2 := &bytes.Buffer{}
	mw := NewMultiWriter(buf1, buf2)
	log := New(mw)

	log.Info().Msg("multi write")

	if buf1.Len() == 0 {
		t.Error("expected output in first buffer")
	}
	if buf2.Len() == 0 {
		t.Error("expected output in second buffer")
	}
	if buf1.String() != buf2.String() {
		t.Error("expected identical output in both buffers")
	}
}

func TestContextFields(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	ctx := log.With().
		Str("service", "api").
		Int("version", 1).
		Bool("debug", false).
		Float64("threshold", 0.95).
		Logger()

	ctx.Info().Str("endpoint", "/health").Msg("request")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["service"] != "api" {
		t.Errorf("context field 'service' mismatch: %v", entry["service"])
	}
	if entry["version"] != float64(1) {
		t.Errorf("context field 'version' mismatch: %v", entry["version"])
	}
	if entry["endpoint"] != "/health" {
		t.Errorf("event field 'endpoint' mismatch: %v", entry["endpoint"])
	}
}

func TestStackTrace(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.EnableStackTrace()

	log.Error().Msg("error with stack")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	stack, ok := entry["stack"].(string)
	if !ok {
		t.Error("expected stack field")
	}
	if !strings.Contains(stack, "goroutine") {
		t.Error("expected stack trace to contain goroutine info")
	}
}

func TestGlobalLogger(t *testing.T) {
	buf := &bytes.Buffer{}
	SetOutput(buf)
	SetLevel(InfoLevel)

	Info().Str("global", "test").Msg("global message")

	if buf.Len() == 0 {
		t.Error("expected global logger output")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
		hasError bool
	}{
		{"trace", TraceLevel, false},
		{"debug", DebugLevel, false},
		{"info", InfoLevel, false},
		{"warn", WarnLevel, false},
		{"warning", WarnLevel, false},
		{"error", ErrorLevel, false},
		{"fatal", FatalLevel, false},
		{"panic", PanicLevel, false},
		{"disabled", Disabled, false},
		{"invalid", InfoLevel, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			level, err := ParseLevel(tc.input)
			if tc.hasError {
				if err == nil {
					t.Error("expected error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if level != tc.expected {
					t.Errorf("expected %v, got %v", tc.expected, level)
				}
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{TraceLevel, "trace"},
		{DebugLevel, "debug"},
		{InfoLevel, "info"},
		{WarnLevel, "warn"},
		{ErrorLevel, "error"},
		{FatalLevel, "fatal"},
		{PanicLevel, "panic"},
		{Level(100), "unknown"},
	}

	for _, tc := range tests {
		if tc.level.String() != tc.expected {
			t.Errorf("expected %s, got %s", tc.expected, tc.level.String())
		}
	}
}

func TestConsoleWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	cw := NewConsoleWriter()
	cw.Out = buf
	cw.NoColor = true

	input := `{"time":"2024-01-01T12:00:00.000Z","level":"info","message":"test","key":"value"}` + "\n"
	_, err := cw.Write([]byte(input))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "12:00:00") {
		t.Error("expected formatted time")
	}
	if !strings.Contains(output, "info") {
		t.Error("expected level")
	}
	if !strings.Contains(output, "test") {
		t.Error("expected message")
	}
	if !strings.Contains(output, "key=value") {
		t.Error("expected field")
	}
}

func TestBurstSampler(t *testing.T) {
	sampler := &BurstSampler{
		Burst:  5,
		Period: 100 * time.Millisecond,
	}

	// First burst should all pass.
	for i := 0; i < 5; i++ {
		if !sampler.Sample(InfoLevel) {
			t.Errorf("expected sample %d to pass in burst", i)
		}
	}

	// After burst, should fail.
	if sampler.Sample(InfoLevel) {
		t.Error("expected sample after burst to fail")
	}

	// Wait for period reset.
	time.Sleep(150 * time.Millisecond)

	// Should pass again.
	if !sampler.Sample(InfoLevel) {
		t.Error("expected sample after period reset to pass")
	}
}

func TestMsgf(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.Info().Str("user", "alice").Msgf("user %s logged in at %d", "alice", 12345)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	expected := "user alice logged in at 12345"
	if entry["message"] != expected {
		t.Errorf("expected message '%s', got '%v'", expected, entry["message"])
	}
}

func TestSendWithoutMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.Info().Str("event", "heartbeat").Send()

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["event"] != "heartbeat" {
		t.Errorf("expected event 'heartbeat', got '%v'", entry["event"])
	}
	if _, ok := entry["message"]; ok {
		t.Error("expected no message field")
	}
}

func TestFieldsMap(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	fields := map[string]interface{}{
		"string": "value",
		"number": 42,
		"bool":   true,
	}

	log.Info().Fields(fields).Msg("with fields map")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["string"] != "value" {
		t.Errorf("expected string 'value', got '%v'", entry["string"])
	}
	if entry["number"] != float64(42) {
		t.Errorf("expected number 42, got '%v'", entry["number"])
	}
	if entry["bool"] != true {
		t.Errorf("expected bool true, got '%v'", entry["bool"])
	}
}

func BenchmarkLoggerNoFields(b *testing.B) {
	buf := &bytes.Buffer{}
	log := New(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.Info().Msg("benchmark message")
		buf.Reset()
	}
}

func BenchmarkLoggerWithFields(b *testing.B) {
	buf := &bytes.Buffer{}
	log := New(buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.Info().
			Str("service", "benchmark").
			Int("port", 8080).
			Bool("debug", false).
			Msg("benchmark message")
		buf.Reset()
	}
}

func BenchmarkLoggerWithContext(b *testing.B) {
	buf := &bytes.Buffer{}
	log := New(buf)
	ctx := log.With().Str("request_id", "abc123").Int("user_id", 42).Logger()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.Info().Str("action", "benchmark").Msg("context benchmark")
		buf.Reset()
	}
}

func BenchmarkLoggerDisabled(b *testing.B) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.SetLevel(ErrorLevel)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		log.Debug().Str("key", "value").Msg("this will be discarded")
	}
}
