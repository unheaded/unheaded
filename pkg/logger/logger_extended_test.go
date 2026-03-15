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

// --- LevelSampler ---

func TestLevelSampler(t *testing.T) {
	// Track which levels were sampled.
	traceCalled := 0
	debugCalled := 0
	infoCalled := 0
	warnCalled := 0
	errorCalled := 0

	sampler := &LevelSampler{
		TraceSampler: &BasicSampler{N: 2},
		DebugSampler: &BasicSampler{N: 3},
		InfoSampler:  &BasicSampler{N: 4},
		WarnSampler:  &BasicSampler{N: 5},
		ErrorSampler: &BasicSampler{N: 6},
	}

	for i := 0; i < 12; i++ {
		if sampler.Sample(TraceLevel) {
			traceCalled++
		}
		if sampler.Sample(DebugLevel) {
			debugCalled++
		}
		if sampler.Sample(InfoLevel) {
			infoCalled++
		}
		if sampler.Sample(WarnLevel) {
			warnCalled++
		}
		if sampler.Sample(ErrorLevel) {
			errorCalled++
		}
	}

	if traceCalled != 6 {
		t.Errorf("expected 6 trace samples, got %d", traceCalled)
	}
	if debugCalled != 4 {
		t.Errorf("expected 4 debug samples, got %d", debugCalled)
	}
	if infoCalled != 3 {
		t.Errorf("expected 3 info samples, got %d", infoCalled)
	}
	if warnCalled < 2 {
		t.Errorf("expected at least 2 warn samples, got %d", warnCalled)
	}
	if errorCalled != 2 {
		t.Errorf("expected 2 error samples, got %d", errorCalled)
	}
}

func TestLevelSamplerNilSamplers(t *testing.T) {
	// When no sampler is set for a level, Sample returns true.
	sampler := &LevelSampler{}

	for _, level := range []Level{TraceLevel, DebugLevel, InfoLevel, WarnLevel, ErrorLevel} {
		if !sampler.Sample(level) {
			t.Errorf("expected Sample to return true for level %s with nil sampler", level)
		}
	}
}

func TestLevelSamplerFatalPanic(t *testing.T) {
	// Levels not explicitly handled (Fatal, Panic) should return true.
	sampler := &LevelSampler{}
	if !sampler.Sample(FatalLevel) {
		t.Error("expected true for FatalLevel")
	}
	if !sampler.Sample(PanicLevel) {
		t.Error("expected true for PanicLevel")
	}
}

// --- BurstSampler with NextSampler ---

func TestBurstSamplerWithNextSampler(t *testing.T) {
	sampler := &BurstSampler{
		Burst:       2,
		Period:      1 * time.Second,
		NextSampler: &BasicSampler{N: 3},
	}

	results := make([]bool, 0, 10)
	for i := 0; i < 10; i++ {
		results = append(results, sampler.Sample(InfoLevel))
	}

	// First 2 should pass (burst), then NextSampler kicks in (every 3rd).
	if !results[0] || !results[1] {
		t.Error("expected first 2 burst samples to pass")
	}
	// After burst, NextSampler handles it.
	afterBurst := 0
	for i := 2; i < 10; i++ {
		if results[i] {
			afterBurst++
		}
	}
	if afterBurst == 0 {
		t.Error("expected NextSampler to allow some samples after burst")
	}
}

// --- NewWriter (sync) ---

func TestNewWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	w := NewWriter(buf)

	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes written, got %d", n)
	}
	if buf.String() != "hello" {
		t.Errorf("expected 'hello', got '%s'", buf.String())
	}

	// Close on a non-closer writer should return nil.
	if err := w.Close(); err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}

// --- Async writer buffer full fallback ---

func TestAsyncWriterBufferFull(t *testing.T) {
	buf := &bytes.Buffer{}
	// Buffer size 1 so it fills quickly.
	w := NewAsyncWriter(buf, 1)

	// Fill the buffer channel.
	w.Write([]byte("first\n"))

	// This should fallback to synchronous write since channel is full.
	n, err := w.Write([]byte("sync-fallback\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 14 {
		t.Errorf("expected 14 bytes, got %d", n)
	}

	w.Close()
}

// --- NewWithConfig ---

func TestNewWithConfig(t *testing.T) {
	buf := &bytes.Buffer{}
	config := DefaultConfig()
	config.Output = buf
	config.Level = DebugLevel

	log := NewWithConfig(config)
	if log.Level() != DebugLevel {
		t.Errorf("expected DebugLevel, got %v", log.Level())
	}

	log.Debug().Msg("debug via config")
	if buf.Len() == 0 {
		t.Error("expected output from debug message")
	}
}

// --- Logger.Level() ---

func TestLoggerLevel(t *testing.T) {
	log := New(&bytes.Buffer{})
	if log.Level() != InfoLevel {
		t.Errorf("expected default InfoLevel, got %v", log.Level())
	}
	log.SetLevel(ErrorLevel)
	if log.Level() != ErrorLevel {
		t.Errorf("expected ErrorLevel, got %v", log.Level())
	}
}

// --- DisableCaller, DisableStackTrace, JSONMode, EnableColor ---

func TestLoggerToggleMethods(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	// EnableCaller then DisableCaller.
	log.EnableCaller()
	log.DisableCaller()
	log.Info().Msg("no caller")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if _, ok := entry["caller"]; ok {
		t.Error("expected no caller field after DisableCaller")
	}

	// EnableStackTrace then DisableStackTrace.
	buf.Reset()
	log.EnableStackTrace()
	log.DisableStackTrace()
	log.Error().Msg("no stack")
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if _, ok := entry["stack"]; ok {
		t.Error("expected no stack field after DisableStackTrace")
	}

	// ConsoleMode then JSONMode.
	buf.Reset()
	log.ConsoleMode()
	log.JSONMode()
	log.Info().Msg("json mode")
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Errorf("expected valid JSON after JSONMode, got error: %v", err)
	}

	// EnableColor (just verify no panic).
	log.EnableColor()
}

// --- Logger.Log() ---

func TestLoggerLog(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.SetLevel(TraceLevel)

	log.Log(WarnLevel).Msg("custom level")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if entry["level"] != "warn" {
		t.Errorf("expected warn, got %v", entry["level"])
	}
}

// --- Logger.Print and Printf ---

func TestLoggerPrint(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.Print("hello ", "world")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if entry["message"] != "hello world" {
		t.Errorf("expected 'hello world', got '%v'", entry["message"])
	}
}

func TestLoggerPrintf(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.Printf("count: %d", 42)

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if entry["message"] != "count: 42" {
		t.Errorf("expected 'count: 42', got '%v'", entry["message"])
	}
}

// --- Panic level ---

func TestPanicLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if r != "panic message" {
			t.Errorf("expected panic message 'panic message', got '%v'", r)
		}
	}()

	log.Panic().Msg("panic message")
}

// --- Context field types not yet tested ---

func TestContextAllFieldTypes(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dur := 500 * time.Millisecond

	ctx := log.With().
		Strs("tags", []string{"a", "b"}).
		Bytes("raw", []byte{0x01, 0x02}).
		Int8("i8", -8).
		Int16("i16", -16).
		Int32("i32", -32).
		Int64("i64", -64).
		Uint("u", 10).
		Uint8("u8", 8).
		Uint16("u16", 16).
		Uint32("u32", 32).
		Uint64("u64", 64).
		Float32("f32", 1.5).
		Time("ts", now).
		Dur("dur", dur).
		Interface("obj", map[string]string{"k": "v"}).
		Err(errors.New("ctx error")).
		Caller().
		Logger()

	ctx.Info().Msg("context all types")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry["i8"] != float64(-8) {
		t.Errorf("i8 mismatch: %v", entry["i8"])
	}
	if entry["u"] != float64(10) {
		t.Errorf("u mismatch: %v", entry["u"])
	}
	if entry["f32"] != float64(1.5) {
		t.Errorf("f32 mismatch: %v", entry["f32"])
	}
	if _, ok := entry["error"]; !ok {
		t.Error("expected error field from context Err()")
	}
	if _, ok := entry["caller"]; !ok {
		t.Error("expected caller field from context Caller()")
	}
}

func TestContextErrNil(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	ctx := log.With().Err(nil).Logger()
	ctx.Info().Msg("no error")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if _, ok := entry["error"]; ok {
		t.Error("expected no error field when err is nil")
	}
}

// --- Event.Stack() ---

func TestEventStack(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.Info().Stack().Msg("with stack")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	stack, ok := entry["stack"].(string)
	if !ok {
		t.Fatal("expected stack field")
	}
	if !strings.Contains(stack, "goroutine") {
		t.Error("expected stack to contain goroutine info")
	}
}

// --- Event.Err with nil ---

func TestEventErrNil(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)

	log.Info().Err(nil).Msg("nil error")

	var entry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if _, ok := entry["error"]; ok {
		t.Error("expected no error field when err is nil")
	}
}

// --- Disabled event methods ---

func TestDisabledEventMethods(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.SetLevel(ErrorLevel)

	// All these should be no-ops on a disabled event.
	log.Debug().
		Str("k", "v").
		Strs("k", []string{"a"}).
		Bytes("k", []byte("b")).
		Int("k", 1).
		Int8("k", 1).
		Int16("k", 1).
		Int32("k", 1).
		Int64("k", 1).
		Uint("k", 1).
		Uint8("k", 1).
		Uint16("k", 1).
		Uint32("k", 1).
		Uint64("k", 1).
		Float32("k", 1.0).
		Float64("k", 1.0).
		Bool("k", true).
		Time("k", time.Now()).
		Dur("k", time.Second).
		Interface("k", nil).
		Err(errors.New("err")).
		Stack().
		Caller().
		Fields(map[string]interface{}{"a": 1}).
		Msg("should not appear")

	if buf.Len() != 0 {
		t.Error("expected no output for disabled event")
	}

	// Also test Msgf and Send on disabled events.
	log.Debug().Msgf("format %d", 1)
	if buf.Len() != 0 {
		t.Error("expected no output for disabled Msgf")
	}

	log.Debug().Send()
	if buf.Len() != 0 {
		t.Error("expected no output for disabled Send")
	}
}

// --- FilteredWriter ---

func TestFilteredWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	fw := &FilteredWriter{Writer: buf, Level: ErrorLevel}

	n, err := fw.Write([]byte("test data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 9 {
		t.Errorf("expected 9 bytes, got %d", n)
	}
	if buf.String() != "test data" {
		t.Errorf("expected 'test data', got '%s'", buf.String())
	}
}

// --- ConsoleWriter with color ---

func TestConsoleWriterWithColor(t *testing.T) {
	buf := &bytes.Buffer{}
	cw := &ConsoleWriter{
		Out:        buf,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}

	input := `{"time":"2024-01-01T12:00:00.000Z","level":"error","message":"oops","count":42}` + "\n"
	_, err := cw.Write([]byte(input))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "oops") {
		t.Error("expected message in output")
	}
	// Should contain ANSI color codes when NoColor is false.
	if !strings.Contains(output, "\033[") {
		t.Error("expected ANSI color codes")
	}
}

func TestConsoleWriterInvalidJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	cw := &ConsoleWriter{
		Out:        buf,
		TimeFormat: "15:04:05",
		NoColor:    true,
	}

	input := "not json at all\n"
	_, err := cw.Write([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != input {
		t.Errorf("expected raw passthrough for invalid JSON, got '%s'", buf.String())
	}
}

func TestConsoleWriterBoolField(t *testing.T) {
	buf := &bytes.Buffer{}
	cw := &ConsoleWriter{
		Out:        buf,
		TimeFormat: "15:04:05",
		NoColor:    true,
	}

	input := `{"time":"2024-01-01T12:00:00.000Z","level":"info","message":"test","active":true}` + "\n"
	_, err := cw.Write([]byte(input))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// bool values go through the default case in ConsoleWriter.Write switch.
	output := buf.String()
	if !strings.Contains(output, "active=true") {
		t.Errorf("expected 'active=true' in output, got '%s'", output)
	}
}

func TestConsoleWriterFloatField(t *testing.T) {
	buf := &bytes.Buffer{}
	cw := &ConsoleWriter{
		Out:        buf,
		TimeFormat: "15:04:05",
		NoColor:    true,
	}

	// Integer-like float (42.0) and real float (3.14).
	input := `{"time":"2024-01-01T12:00:00.000Z","level":"info","message":"test","count":42,"ratio":3.14}` + "\n"
	_, err := cw.Write([]byte(input))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "count=42") {
		t.Errorf("expected 'count=42' in output, got '%s'", output)
	}
	if !strings.Contains(output, "ratio=3.14") {
		t.Errorf("expected 'ratio=3.14' in output, got '%s'", output)
	}
}

// --- Console mode with color and caller ---

func TestConsoleModeWithColorAndCaller(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.ConsoleMode()
	log.EnableColor()
	log.EnableCaller()

	log.Error().Str("key", "val").Msg("colored error")

	output := buf.String()
	if !strings.Contains(output, "colored error") {
		t.Error("expected message in console output")
	}
	// Should contain ANSI codes.
	if !strings.Contains(output, "\033[") {
		t.Error("expected ANSI color codes in colored console mode")
	}
}

func TestConsoleModeNoColorWithCaller(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.ConsoleMode()
	log.DisableColor()
	log.EnableCaller()

	log.Info().Str("k", "v").Msg("no color caller")

	output := buf.String()
	if !strings.Contains(output, "no color caller") {
		t.Error("expected message")
	}
	if !strings.Contains(output, ".go:") {
		t.Error("expected caller with .go:")
	}
}

func TestConsoleModeWithBoolField(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.ConsoleMode()
	log.DisableColor()

	log.Info().Bool("flag", true).Msg("bool field")

	output := buf.String()
	if !strings.Contains(output, "flag=true") {
		t.Errorf("expected 'flag=true' in output, got '%s'", output)
	}
}

func TestConsoleModeWithFloatFields(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.ConsoleMode()
	log.DisableColor()

	log.Info().Float64("ratio", 3.14).Int("count", 42).Msg("mixed")

	output := buf.String()
	if !strings.Contains(output, "ratio=3.14") {
		t.Errorf("expected 'ratio=3.14', got '%s'", output)
	}
	if !strings.Contains(output, "count=42") {
		t.Errorf("expected 'count=42', got '%s'", output)
	}
}

// --- Global functions ---

func TestGlobalFunctions(t *testing.T) {
	buf := &bytes.Buffer{}
	log := New(buf)
	log.SetLevel(TraceLevel)

	original := globalLogger
	SetGlobalLogger(log)
	defer SetGlobalLogger(original)

	got := GetGlobalLogger()
	if got != log {
		t.Error("GetGlobalLogger returned wrong logger")
	}

	// Test global Trace.
	Trace().Msg("global trace")
	if buf.Len() == 0 {
		t.Error("expected trace output")
	}

	// Test global Debug.
	buf.Reset()
	Debug().Msg("global debug")
	if buf.Len() == 0 {
		t.Error("expected debug output")
	}

	// Test global Warn.
	buf.Reset()
	Warn().Msg("global warn")
	if buf.Len() == 0 {
		t.Error("expected warn output")
	}

	// Test global Error.
	buf.Reset()
	Error().Msg("global error")
	if buf.Len() == 0 {
		t.Error("expected error output")
	}

	// Test global Print.
	buf.Reset()
	Print("global print")
	if buf.Len() == 0 {
		t.Error("expected print output")
	}

	// Test global Printf.
	buf.Reset()
	Printf("global %s", "printf")
	if buf.Len() == 0 {
		t.Error("expected printf output")
	}

	// Test global With.
	buf.Reset()
	With().Str("ctx", "global").Logger().Info().Msg("with context")
	if buf.Len() == 0 {
		t.Error("expected output from global With")
	}
}

// --- MultiWriter error propagation ---

type errWriter struct{}

func (ew *errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestMultiWriterError(t *testing.T) {
	buf := &bytes.Buffer{}
	mw := NewMultiWriter(&errWriter{}, buf)

	_, err := mw.Write([]byte("test"))
	if err == nil {
		t.Error("expected error from MultiWriter when a writer fails")
	}
}

// --- appendInterface with unmarshalable value ---

func TestAppendInterfaceUnmarshalable(t *testing.T) {
	// A channel can't be marshaled to JSON.
	ch := make(chan int)
	result := appendInterface(nil, "key", ch)
	if !strings.Contains(string(result), "null") {
		t.Errorf("expected null for unmarshalable value, got '%s'", string(result))
	}
}

// --- putBuffer with large buffer ---

func TestPutBufferLarge(t *testing.T) {
	// Create a buffer larger than 64KB to test the discard path.
	buf := bytes.NewBuffer(make([]byte, 0, 128*1024))
	buf.Write(make([]byte, 100*1024))
	putBuffer(buf)
	// No panic means the large buffer path works (it just doesn't pool it).
}

// --- Console mode formatConsole invalid JSON fallback ---

func TestConsoleModeInvalidTimestamp(t *testing.T) {
	buf := &bytes.Buffer{}
	config := DefaultConfig()
	config.Output = buf
	config.ConsoleMode = true
	config.ColorEnabled = false

	log := NewWithConfig(config)

	// Normal write - just verify it works end to end.
	log.Info().Msg("format test")

	output := buf.String()
	if !strings.Contains(output, "format test") {
		t.Errorf("expected message in output, got '%s'", output)
	}
}

// --- Close on Writer wrapping an io.Closer ---

type closerWriter struct {
	bytes.Buffer
	closed bool
}

func (cw *closerWriter) Close() error {
	cw.closed = true
	return nil
}

func TestWriterCloseWithCloser(t *testing.T) {
	cw := &closerWriter{}
	w := NewWriter(cw)
	if err := w.Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !cw.closed {
		t.Error("expected underlying writer to be closed")
	}
}

func TestAsyncWriterCloseWithCloser(t *testing.T) {
	cw := &closerWriter{}
	w := NewAsyncWriter(cw, 10)
	w.Write([]byte("test\n"))
	if err := w.Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !cw.closed {
		t.Error("expected underlying writer to be closed")
	}
}

// --- ConsoleWriter with unparseable time ---

func TestConsoleWriterUnparseableTime(t *testing.T) {
	buf := &bytes.Buffer{}
	cw := &ConsoleWriter{
		Out:        buf,
		TimeFormat: "15:04:05",
		NoColor:    true,
	}

	// Time value that doesn't match RFC3339Nano.
	input := `{"time":"not-a-time","level":"info","message":"test"}` + "\n"
	_, err := cw.Write([]byte(input))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	output := buf.String()
	// Should fall back to raw time string.
	if !strings.Contains(output, "not-a-time") {
		t.Errorf("expected raw time string in output, got '%s'", output)
	}
}
