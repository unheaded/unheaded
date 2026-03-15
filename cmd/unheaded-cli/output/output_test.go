// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package output

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// color.go tests
// ---------------------------------------------------------------------------

func TestBold(t *testing.T) {
	got := Bold("hello")
	want := ColorBold + "hello" + ColorReset
	if got != want {
		t.Errorf("Bold(\"hello\") = %q, want %q", got, want)
	}
}

func TestDim(t *testing.T) {
	got := Dim("hello")
	want := ColorDim + "hello" + ColorReset
	if got != want {
		t.Errorf("Dim(\"hello\") = %q, want %q", got, want)
	}
}

func TestColorFunctions(t *testing.T) {
	tests := []struct {
		name  string
		fn    func(string) string
		code  string
	}{
		{"Red", Red, ColorRed},
		{"Green", Green, ColorGreen},
		{"Yellow", Yellow, ColorYellow},
		{"Blue", Blue, ColorBlue},
		{"Cyan", Cyan, ColorCyan},
		{"Magenta", Magenta, ColorMagenta},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn("text")
			want := tt.code + "text" + ColorReset
			if got != want {
				t.Errorf("%s(\"text\") = %q, want %q", tt.name, got, want)
			}
		})
	}
}

func TestColorFunctionsEmptyString(t *testing.T) {
	got := Red("")
	want := ColorRed + "" + ColorReset
	if got != want {
		t.Errorf("Red(\"\") = %q, want %q", got, want)
	}
}

func TestHighlightMatch(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		match   string
		enabled bool
		want    string
	}{
		{
			name:    "enabled with match",
			s:       "hello world",
			match:   "world",
			enabled: true,
			want:    "hello " + ColorYellow + ColorBold + "world" + ColorReset,
		},
		{
			name:    "disabled",
			s:       "hello world",
			match:   "world",
			enabled: false,
			want:    "hello world",
		},
		{
			name:    "empty match",
			s:       "hello world",
			match:   "",
			enabled: true,
			want:    "hello world",
		},
		{
			name:    "match not found",
			s:       "hello world",
			match:   "xyz",
			enabled: true,
			want:    "hello world",
		},
		{
			name:    "multiple occurrences",
			s:       "aXbXc",
			match:   "X",
			enabled: true,
			want:    "a" + ColorYellow + ColorBold + "X" + ColorReset + "b" + ColorYellow + ColorBold + "X" + ColorReset + "c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HighlightMatch(tt.s, tt.match, tt.enabled)
			if got != tt.want {
				t.Errorf("HighlightMatch(%q, %q, %v) = %q, want %q", tt.s, tt.match, tt.enabled, got, tt.want)
			}
		})
	}
}

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	if theme.Primary != ColorCyan {
		t.Errorf("Primary = %q, want %q", theme.Primary, ColorCyan)
	}
	if theme.Success != ColorGreen {
		t.Errorf("Success = %q, want %q", theme.Success, ColorGreen)
	}
	if theme.Error != ColorRed {
		t.Errorf("Error = %q, want %q", theme.Error, ColorRed)
	}
	if theme.Warning != ColorYellow {
		t.Errorf("Warning = %q, want %q", theme.Warning, ColorYellow)
	}
	if theme.Info != ColorBlue {
		t.Errorf("Info = %q, want %q", theme.Info, ColorBlue)
	}
	if theme.Secondary != ColorMagenta {
		t.Errorf("Secondary = %q, want %q", theme.Secondary, ColorMagenta)
	}
	if theme.Muted != ColorDim {
		t.Errorf("Muted = %q, want %q", theme.Muted, ColorDim)
	}
}

func TestColorThemeApply(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		style string
		color string
	}{
		{"primary", ColorCyan},
		{"secondary", ColorMagenta},
		{"success", ColorGreen},
		{"warning", ColorYellow},
		{"error", ColorRed},
		{"info", ColorBlue},
		{"muted", ColorDim},
	}

	for _, tt := range tests {
		t.Run(tt.style, func(t *testing.T) {
			got := theme.Apply(tt.style, "text")
			want := tt.color + "text" + ColorReset
			if got != want {
				t.Errorf("Apply(%q, \"text\") = %q, want %q", tt.style, got, want)
			}
		})
	}
}

func TestColorThemeApplyUnknown(t *testing.T) {
	theme := DefaultTheme()
	got := theme.Apply("unknown", "text")
	if got != "text" {
		t.Errorf("Apply(\"unknown\", \"text\") = %q, want %q", got, "text")
	}
}

func TestBanner(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		color bool
	}{
		{"no color single line", "Hello", false},
		{"with color single line", "Hello", true},
		{"no color multi line", "Hello\nWorld", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Banner(tt.text, tt.color)

			if tt.color {
				if !strings.HasPrefix(got, ColorCyan+ColorBold) {
					t.Error("colored banner should start with color codes")
				}
				if !strings.Contains(got, ColorReset) {
					t.Error("colored banner should contain reset code")
				}
			} else {
				if strings.Contains(got, "\033[") {
					t.Error("uncolored banner should not contain ANSI escape codes")
				}
			}

			// Verify border structure
			stripped := stripANSI(got)
			lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")
			if len(lines) < 3 {
				t.Fatalf("banner should have at least 3 lines, got %d", len(lines))
			}

			// Top and bottom borders should be equal signs
			if !strings.HasPrefix(lines[0], "=") {
				t.Errorf("top border should start with '=', got %q", lines[0])
			}
			if !strings.HasPrefix(lines[len(lines)-1], "=") {
				t.Errorf("bottom border should start with '=', got %q", lines[len(lines)-1])
			}

			// Content lines should have pipes
			for _, line := range lines[1 : len(lines)-1] {
				if !strings.HasPrefix(line, "| ") {
					t.Errorf("content line should start with '| ', got %q", line)
				}
				if !strings.HasSuffix(line, " |") {
					t.Errorf("content line should end with ' |', got %q", line)
				}
			}
		})
	}
}

func TestBannerMultilinePadding(t *testing.T) {
	got := Banner("Hi\nHello", false)
	stripped := stripANSI(got)
	lines := strings.Split(strings.TrimRight(stripped, "\n"), "\n")

	// Both content lines should have the same length
	if len(lines) < 4 {
		t.Fatalf("expected 4 lines, got %d", len(lines))
	}
	if len(lines[1]) != len(lines[2]) {
		t.Errorf("content lines should be same length, got %d and %d", len(lines[1]), len(lines[2]))
	}
}

func TestGradientText(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		enabled bool
	}{
		{"disabled returns plain", "hello", false},
		{"enabled adds colors", "hello world", true},
		{"empty string enabled", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GradientText(tt.text, tt.enabled)
			if !tt.enabled {
				if got != tt.text {
					t.Errorf("disabled gradient should return original, got %q", got)
				}
				return
			}
			if tt.text != "" {
				if !strings.Contains(got, ColorReset) {
					t.Error("enabled gradient should end with color reset")
				}
				// Should contain at least one color code
				if !strings.Contains(got, "\033[") {
					t.Error("enabled gradient should contain ANSI codes")
				}
			}
		})
	}
}

func TestGradientTextPreservesContent(t *testing.T) {
	text := "ABCDEF"
	got := GradientText(text, true)
	stripped := stripANSI(got)
	if stripped != text {
		t.Errorf("stripped gradient = %q, want %q", stripped, text)
	}
}

func TestIconString(t *testing.T) {
	tests := []struct {
		icon Icon
		want string
	}{
		{IconSuccess, "\u2714"},
		{IconError, "\u2718"},
		{IconWarning, "\u26A0"},
		{IconInfo, "\u2139"},
	}

	for _, tt := range tests {
		if got := tt.icon.String(); got != tt.want {
			t.Errorf("icon.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestIconWithColor(t *testing.T) {
	got := IconSuccess.WithColor(ColorGreen)
	want := ColorGreen + string(IconSuccess) + ColorReset
	if got != want {
		t.Errorf("WithColor = %q, want %q", got, want)
	}
}

func TestStatusIcon(t *testing.T) {
	tests := []struct {
		status  string
		enabled bool
		wantOK  func(string) bool
	}{
		{"running", true, func(s string) bool { return strings.Contains(s, ColorGreen) }},
		{"healthy", true, func(s string) bool { return strings.Contains(s, ColorGreen) }},
		{"active", true, func(s string) bool { return strings.Contains(s, ColorGreen) }},
		{"stopped", true, func(s string) bool { return strings.Contains(s, ColorRed) }},
		{"error", true, func(s string) bool { return strings.Contains(s, ColorRed) }},
		{"failed", true, func(s string) bool { return strings.Contains(s, ColorRed) }},
		{"pending", true, func(s string) bool { return strings.Contains(s, ColorYellow) }},
		{"starting", true, func(s string) bool { return strings.Contains(s, ColorYellow) }},
		{"unknown", true, func(s string) bool { return strings.Contains(s, ColorCyan) }},
		{"running", false, func(s string) bool { return s == "" }},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := StatusIcon(tt.status, tt.enabled)
			if !tt.wantOK(got) {
				t.Errorf("StatusIcon(%q, %v) = %q, unexpected", tt.status, tt.enabled, got)
			}
		})
	}
}

func TestColorizeStatus(t *testing.T) {
	tests := []struct {
		status  string
		enabled bool
	}{
		{"running", true},
		{"running", false},
		{"stopped", true},
		{"pending", true},
		{"frozen", true},
		{"other", true},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := ColorizeStatus(tt.status, tt.enabled)
			if !tt.enabled {
				if got != tt.status {
					t.Errorf("disabled ColorizeStatus should return original, got %q", got)
				}
			} else {
				stripped := stripANSI(got)
				if stripped != tt.status {
					t.Errorf("stripped ColorizeStatus = %q, want %q", stripped, tt.status)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Environment-based color detection tests
// ---------------------------------------------------------------------------

func TestIsTerminal_NO_COLOR(t *testing.T) {
	// Save and restore env
	noColor, noColorSet := os.LookupEnv("NO_COLOR")
	forceColor, forceColorSet := os.LookupEnv("FORCE_COLOR")
	defer func() {
		if noColorSet {
			os.Setenv("NO_COLOR", noColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
		if forceColorSet {
			os.Setenv("FORCE_COLOR", forceColor)
		} else {
			os.Unsetenv("FORCE_COLOR")
		}
	}()

	os.Unsetenv("FORCE_COLOR")
	os.Setenv("NO_COLOR", "1")

	if IsTerminal() {
		t.Error("IsTerminal() should return false when NO_COLOR is set")
	}
}

func TestIsTerminal_FORCE_COLOR(t *testing.T) {
	noColor, noColorSet := os.LookupEnv("NO_COLOR")
	forceColor, forceColorSet := os.LookupEnv("FORCE_COLOR")
	defer func() {
		if noColorSet {
			os.Setenv("NO_COLOR", noColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
		if forceColorSet {
			os.Setenv("FORCE_COLOR", forceColor)
		} else {
			os.Unsetenv("FORCE_COLOR")
		}
	}()

	os.Unsetenv("NO_COLOR")
	os.Setenv("FORCE_COLOR", "1")

	if !IsTerminal() {
		t.Error("IsTerminal() should return true when FORCE_COLOR is set")
	}
}

func TestIsTerminal_NO_COLOR_TakesPrecedence(t *testing.T) {
	noColor, noColorSet := os.LookupEnv("NO_COLOR")
	forceColor, forceColorSet := os.LookupEnv("FORCE_COLOR")
	defer func() {
		if noColorSet {
			os.Setenv("NO_COLOR", noColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
		if forceColorSet {
			os.Setenv("FORCE_COLOR", forceColor)
		} else {
			os.Unsetenv("FORCE_COLOR")
		}
	}()

	// When both are set, NO_COLOR is checked first
	os.Setenv("NO_COLOR", "")
	os.Setenv("FORCE_COLOR", "1")

	if IsTerminal() {
		t.Error("IsTerminal() should return false when NO_COLOR is set (even if empty)")
	}
}

func TestIsTerminal_TERM_dumb(t *testing.T) {
	noColor, noColorSet := os.LookupEnv("NO_COLOR")
	forceColor, forceColorSet := os.LookupEnv("FORCE_COLOR")
	term, termSet := os.LookupEnv("TERM")
	defer func() {
		if noColorSet {
			os.Setenv("NO_COLOR", noColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
		if forceColorSet {
			os.Setenv("FORCE_COLOR", forceColor)
		} else {
			os.Unsetenv("FORCE_COLOR")
		}
		if termSet {
			os.Setenv("TERM", term)
		} else {
			os.Unsetenv("TERM")
		}
	}()

	os.Unsetenv("NO_COLOR")
	os.Unsetenv("FORCE_COLOR")
	os.Setenv("TERM", "dumb")

	if IsTerminal() {
		t.Error("IsTerminal() should return false when TERM=dumb")
	}
}

// ---------------------------------------------------------------------------
// output.go tests
// ---------------------------------------------------------------------------

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input   string
		want    Format
		wantErr bool
	}{
		{"table", FormatTable, false},
		{"TABLE", FormatTable, false},
		{"", FormatTable, false},
		{"json", FormatJSON, false},
		{"JSON", FormatJSON, false},
		{"yaml", FormatYAML, false},
		{"YAML", FormatYAML, false},
		{"yml", FormatYAML, false},
		{"wide", FormatWide, false},
		{"WIDE", FormatWide, false},
		{"invalid", FormatTable, true},
		{"csv", FormatTable, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseFormat(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFormat(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseFormat(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWriterDefaults(t *testing.T) {
	w := NewWriter()
	if w.Format() != FormatTable {
		t.Errorf("default format = %q, want %q", w.Format(), FormatTable)
	}
	if w.Out() == nil {
		t.Error("default output writer should not be nil")
	}
}

func TestWriterSetters(t *testing.T) {
	var buf bytes.Buffer
	var errBuf bytes.Buffer

	w := NewWriter().
		SetOutput(&buf).
		SetErrorOutput(&errBuf).
		SetFormat(FormatJSON).
		SetColor(false)

	if w.Format() != FormatJSON {
		t.Errorf("format = %q, want %q", w.Format(), FormatJSON)
	}
	if w.Color() {
		t.Error("color should be false")
	}
}

func TestWriterSetterChaining(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter()
	ret := w.SetOutput(&buf)
	if ret != w {
		t.Error("SetOutput should return the same writer for chaining")
	}
	ret = w.SetFormat(FormatJSON)
	if ret != w {
		t.Error("SetFormat should return the same writer for chaining")
	}
	ret = w.SetColor(true)
	if ret != w {
		t.Error("SetColor should return the same writer for chaining")
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf)

	data := map[string]string{"name": "test", "value": "123"}
	if err := w.WriteJSON(data); err != nil {
		t.Fatalf("WriteJSON error: %v", err)
	}

	var parsed map[string]string
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["name"] != "test" || parsed["value"] != "123" {
		t.Errorf("parsed JSON = %v, want map[name:test value:123]", parsed)
	}
}

func TestWriteJSONPrettyPrinted(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf)

	data := map[string]string{"key": "val"}
	w.WriteJSON(data)

	// Pretty-printed JSON should contain indentation
	if !strings.Contains(buf.String(), "  ") {
		t.Error("JSON output should be indented")
	}
}

func TestWriteYAML(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf)

	data := map[string]string{"name": "test", "value": "123"}
	if err := w.WriteYAML(data); err != nil {
		t.Fatalf("WriteYAML error: %v", err)
	}

	var parsed map[string]string
	if err := yaml.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v", err)
	}
	if parsed["name"] != "test" || parsed["value"] != "123" {
		t.Errorf("parsed YAML = %v, want map[name:test value:123]", parsed)
	}
}

func TestWriteString(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf)

	if err := w.WriteString("hello"); err != nil {
		t.Fatalf("WriteString error: %v", err)
	}
	if buf.String() != "hello" {
		t.Errorf("output = %q, want %q", buf.String(), "hello")
	}
}

func TestWriteStringln(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf)

	if err := w.WriteStringln("hello"); err != nil {
		t.Fatalf("WriteStringln error: %v", err)
	}
	if buf.String() != "hello\n" {
		t.Errorf("output = %q, want %q", buf.String(), "hello\n")
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name  string
		color bool
		want  string
	}{
		{"with color", true, ColorRed + "Error: " + ColorReset + "something broke\n"},
		{"without color", false, "Error: something broke\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errBuf bytes.Buffer
			w := NewWriter().SetErrorOutput(&errBuf).SetColor(tt.color)
			w.WriteError("something %s", "broke")
			if errBuf.String() != tt.want {
				t.Errorf("WriteError output = %q, want %q", errBuf.String(), tt.want)
			}
		})
	}
}

func TestWriteWarning(t *testing.T) {
	tests := []struct {
		name  string
		color bool
		want  string
	}{
		{"with color", true, ColorYellow + "Warning: " + ColorReset + "careful\n"},
		{"without color", false, "Warning: careful\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errBuf bytes.Buffer
			w := NewWriter().SetErrorOutput(&errBuf).SetColor(tt.color)
			w.WriteWarning("careful")
			if errBuf.String() != tt.want {
				t.Errorf("WriteWarning output = %q, want %q", errBuf.String(), tt.want)
			}
		})
	}
}

func TestWriteSuccess(t *testing.T) {
	tests := []struct {
		name  string
		color bool
		want  string
	}{
		{"with color", true, ColorGreen + "done" + ColorReset + "\n"},
		{"without color", false, "done\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewWriter().SetOutput(&buf).SetColor(tt.color)
			w.WriteSuccess("done")
			if buf.String() != tt.want {
				t.Errorf("WriteSuccess output = %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestWriteInfo(t *testing.T) {
	tests := []struct {
		name  string
		color bool
		want  string
	}{
		{"with color", true, ColorCyan + "info msg" + ColorReset + "\n"},
		{"without color", false, "info msg\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewWriter().SetOutput(&buf).SetColor(tt.color)
			w.WriteInfo("info msg")
			if buf.String() != tt.want {
				t.Errorf("WriteInfo output = %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestWriteFormatDispatch(t *testing.T) {
	t.Run("json format", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter().SetOutput(&buf).SetFormat(FormatJSON)

		data := map[string]int{"x": 1}
		if err := w.Write(data); err != nil {
			t.Fatalf("Write error: %v", err)
		}

		var parsed map[string]int
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("not valid JSON: %v", err)
		}
		if parsed["x"] != 1 {
			t.Errorf("parsed = %v, want map[x:1]", parsed)
		}
	})

	t.Run("yaml format", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter().SetOutput(&buf).SetFormat(FormatYAML)

		data := map[string]int{"y": 2}
		if err := w.Write(data); err != nil {
			t.Fatalf("Write error: %v", err)
		}

		var parsed map[string]int
		if err := yaml.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("not valid YAML: %v", err)
		}
		if parsed["y"] != 2 {
			t.Errorf("parsed = %v, want map[y:2]", parsed)
		}
	})

	t.Run("table format falls back to JSON for non-TableWriter", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter().SetOutput(&buf).SetFormat(FormatTable)

		data := map[string]string{"k": "v"}
		if err := w.Write(data); err != nil {
			t.Fatalf("Write error: %v", err)
		}

		// Should fall back to JSON since map doesn't implement TableWriter
		var parsed map[string]string
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("fallback should produce valid JSON: %v, output: %q", err, buf.String())
		}
	})
}

// mockTableWriter implements TableWriter for testing
type mockTableWriter struct {
	headers []string
	rows    [][]string
}

func (m *mockTableWriter) WriteTable(t *Table, wide bool) {
	t.SetHeaders(m.headers...)
	for _, row := range m.rows {
		t.AddRow(row...)
	}
}

func TestWriteWithTableWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf).SetFormat(FormatTable).SetColor(false)

	tw := &mockTableWriter{
		headers: []string{"Name", "Age"},
		rows:    [][]string{{"Alice", "30"}},
	}

	if err := w.Write(tw); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "NAME") {
		t.Error("table output should contain uppercased header NAME")
	}
	if !strings.Contains(output, "Alice") {
		t.Error("table output should contain row data")
	}
}

func TestWriteTable(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf).SetColor(false)

	tw := &mockTableWriter{
		headers: []string{"Col1"},
		rows:    [][]string{{"val1"}},
	}

	if err := w.WriteTable(tw); err != nil {
		t.Fatalf("WriteTable error: %v", err)
	}

	if !strings.Contains(buf.String(), "COL1") {
		t.Error("should contain header")
	}
	if !strings.Contains(buf.String(), "val1") {
		t.Error("should contain row value")
	}
}

func TestStatusOutput_Colorize(t *testing.T) {
	tests := []struct {
		status    string
		color     bool
		wantColor string
	}{
		{"running", true, ColorGreen},
		{"healthy", true, ColorGreen},
		{"active", true, ColorGreen},
		{"success", true, ColorGreen},
		{"ok", true, ColorGreen},
		{"stopped", true, ColorRed},
		{"error", true, ColorRed},
		{"failed", true, ColorRed},
		{"unhealthy", true, ColorRed},
		{"pending", true, ColorYellow},
		{"starting", true, ColorYellow},
		{"stopping", true, ColorYellow},
		{"warning", true, ColorYellow},
		{"frozen", true, ColorCyan},
		{"unknown", true, ""},
		{"running", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			so := StatusOutput{Status: tt.status}
			got := so.Colorize(tt.color)

			if !tt.color {
				if got != tt.status {
					t.Errorf("Colorize(false) = %q, want %q", got, tt.status)
				}
				return
			}

			stripped := stripANSI(got)
			if stripped != tt.status {
				t.Errorf("stripped = %q, want %q", stripped, tt.status)
			}

			if tt.wantColor != "" {
				if !strings.HasPrefix(got, tt.wantColor) {
					t.Errorf("expected color prefix %q, got %q", tt.wantColor, got)
				}
			}
		})
	}
}

func TestBox(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		content string
		color   bool
	}{
		{"simple no color", "Title", "content line", false},
		{"simple with color", "Title", "content line", true},
		{"no title", "", "just content", false},
		{"multiline content", "Info", "line1\nline2\nline3", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Box(tt.title, tt.content, tt.color)
			stripped := stripANSI(got)

			// Should contain border characters
			if !strings.Contains(stripped, "+") {
				t.Error("box should contain '+' corners")
			}
			if !strings.Contains(stripped, "-") {
				t.Error("box should contain '-' horizontal borders")
			}
			if !strings.Contains(stripped, "|") {
				t.Error("box should contain '|' vertical borders")
			}

			if tt.title != "" {
				if !strings.Contains(stripped, tt.title) {
					t.Errorf("box should contain title %q", tt.title)
				}
			}

			// All content should be present
			for _, line := range strings.Split(tt.content, "\n") {
				if !strings.Contains(stripped, line) {
					t.Errorf("box should contain content line %q", line)
				}
			}

			if tt.color {
				if !strings.Contains(got, ColorCyan) {
					t.Error("colored box should contain cyan color code")
				}
			} else {
				if strings.Contains(got, "\033[") {
					t.Error("uncolored box should not contain ANSI codes")
				}
			}
		})
	}
}

func TestBoxConsistentWidth(t *testing.T) {
	got := Box("AB", "ABCDE\nX", false)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	// All lines should have equal length
	lineLen := len(lines[0])
	for i, line := range lines {
		if len(line) != lineLen {
			t.Errorf("line %d length = %d, want %d (line: %q)", i, len(line), lineLen, line)
		}
	}
}

// ---------------------------------------------------------------------------
// table.go tests
// ---------------------------------------------------------------------------

func TestNewTable(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf)
	if tbl == nil {
		t.Fatal("NewTable returned nil")
	}
	if !tbl.Color {
		t.Error("default color should be true")
	}
}

func TestTableSetColor(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false)
	if tbl.Color {
		t.Error("color should be false after SetColor(false)")
	}
}

func TestTableSetHeaders(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetHeaders("A", "B", "C")

	// Verify widths initialized
	if tbl.RowCount() != 0 {
		t.Error("initial row count should be 0")
	}
}

func TestTableAddRow(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetHeaders("Name", "Value")
	tbl.AddRow("foo", "bar")
	tbl.AddRow("baz", "qux")

	if tbl.RowCount() != 2 {
		t.Errorf("RowCount() = %d, want 2", tbl.RowCount())
	}
}

func TestTableRenderNoHeaders(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false)
	tbl.Render() // should not panic
	if buf.String() != "" {
		t.Errorf("empty table render should produce no output, got %q", buf.String())
	}
}

func TestTableRenderNoRows(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("Name", "Value")
	tbl.Render()

	output := buf.String()
	if !strings.Contains(output, "NAME") {
		t.Error("header-only table should still render headers")
	}
	if !strings.Contains(output, "VALUE") {
		t.Error("header-only table should still render headers")
	}
}

func TestTableRenderBasic(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("Name", "Status")
	tbl.AddRow("web", "running")
	tbl.AddRow("db", "stopped")
	tbl.Render()

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// First line is headers
	if !strings.Contains(lines[0], "NAME") {
		t.Errorf("header line should contain NAME, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "STATUS") {
		t.Errorf("header line should contain STATUS, got %q", lines[0])
	}

	// Data rows
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 rows), got %d", len(lines))
	}
	if !strings.Contains(lines[1], "web") {
		t.Errorf("row 1 should contain 'web', got %q", lines[1])
	}
	if !strings.Contains(lines[2], "db") {
		t.Errorf("row 2 should contain 'db', got %q", lines[2])
	}
}

func TestTableRenderWithColor(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(true).SetHeaders("Name")
	tbl.AddRow("test")
	tbl.Render()

	output := buf.String()
	if !strings.Contains(output, ColorBold) {
		t.Error("colored table should use bold for headers")
	}
	if !strings.Contains(output, ColorReset) {
		t.Error("colored table should contain reset code")
	}
}

func TestTableColumnWidthTracking(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("A", "B")
	tbl.AddRow("short", "x")
	tbl.AddRow("a very long cell value", "y")
	tbl.Render()

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// The header line and all rows should reflect the widest cell
	// "a very long cell value" is 21 chars, wider than header "A" (1 char)
	// All lines in column A should pad to that width
	if !strings.Contains(lines[2], "a very long cell value") {
		t.Errorf("row should contain long value, got %q", lines[2])
	}
}

func TestTableSingleColumn(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("Only")
	tbl.AddRow("one")
	tbl.AddRow("two")
	tbl.Render()

	output := buf.String()
	if !strings.Contains(output, "ONLY") {
		t.Error("should contain single header")
	}
	if !strings.Contains(output, "one") {
		t.Error("should contain first row")
	}
	if !strings.Contains(output, "two") {
		t.Error("should contain second row")
	}
}

func TestTableManyColumns(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("A", "B", "C", "D", "E")
	tbl.AddRow("1", "2", "3", "4", "5")
	tbl.Render()

	output := buf.String()
	for _, h := range []string{"A", "B", "C", "D", "E"} {
		if !strings.Contains(output, h) {
			t.Errorf("should contain header %q", h)
		}
	}
	for _, v := range []string{"1", "2", "3", "4", "5"} {
		if !strings.Contains(output, v) {
			t.Errorf("should contain value %q", v)
		}
	}
}

func TestTableFewerCellsThanHeaders(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("A", "B", "C")
	tbl.AddRow("only-one")
	tbl.Render()

	output := buf.String()
	if !strings.Contains(output, "only-one") {
		t.Error("should contain the provided cell")
	}
	// Should not panic with fewer cells than headers
}

func TestTableUnicodeContent(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("Name", "Symbol")
	tbl.AddRow("Check", string(IconSuccess))
	tbl.AddRow("Cross", string(IconError))
	tbl.AddRow("Japanese", "\u6771\u4EAC") // Tokyo in kanji
	tbl.Render()

	output := buf.String()
	if !strings.Contains(output, string(IconSuccess)) {
		t.Error("should contain unicode check mark")
	}
	if !strings.Contains(output, "\u6771\u4EAC") {
		t.Error("should contain unicode kanji")
	}
}

func TestTableWithANSIInCells(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("Name", "Status")
	tbl.AddRow("web", Green("running"))
	tbl.AddRow("db", Red("stopped"))
	tbl.Render()

	output := buf.String()
	stripped := stripANSI(output)

	// After stripping, the data should still be present
	if !strings.Contains(stripped, "running") {
		t.Error("stripped output should contain 'running'")
	}
	if !strings.Contains(stripped, "stopped") {
		t.Error("stripped output should contain 'stopped'")
	}

	// The raw output should contain ANSI codes
	if !strings.Contains(output, ColorGreen) {
		t.Error("raw output should contain green ANSI code")
	}
}

// ---------------------------------------------------------------------------
// RenderWithBorders tests
// ---------------------------------------------------------------------------

func TestTableRenderWithBordersNoHeaders(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false)
	tbl.RenderWithBorders() // should not panic
	if buf.String() != "" {
		t.Errorf("empty bordered table should produce no output, got %q", buf.String())
	}
}

func TestTableRenderWithBordersBasic(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("Name", "Value")
	tbl.AddRow("key1", "val1")
	tbl.AddRow("key2", "val2")
	tbl.RenderWithBorders()

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Should have borders: top + header + middle + row1 + row2 + bottom = 6 lines
	if len(lines) != 6 {
		t.Fatalf("bordered table with 2 rows should have 6 lines, got %d: %v", len(lines), lines)
	}

	// Top and bottom borders should start with +
	if !strings.HasPrefix(lines[0], "+") || !strings.HasSuffix(lines[0], "+") {
		t.Errorf("top border malformed: %q", lines[0])
	}
	if !strings.HasPrefix(lines[len(lines)-1], "+") || !strings.HasSuffix(lines[len(lines)-1], "+") {
		t.Errorf("bottom border malformed: %q", lines[len(lines)-1])
	}

	// Middle border (between header and data)
	if !strings.HasPrefix(lines[2], "+") {
		t.Errorf("middle border should start with +, got %q", lines[2])
	}

	// Data rows should have pipes
	if !strings.Contains(lines[3], "key1") {
		t.Errorf("data row should contain 'key1', got %q", lines[3])
	}
}

func TestTableRenderWithBordersColor(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(true).SetHeaders("H")
	tbl.AddRow("val")
	tbl.RenderWithBorders()

	output := buf.String()
	if !strings.Contains(output, ColorDim) {
		t.Error("colored bordered table should use dim for borders")
	}
	if !strings.Contains(output, ColorBold) {
		t.Error("colored bordered table should use bold for headers")
	}
}

func TestTableRenderWithBordersNoColor(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("H")
	tbl.AddRow("val")
	tbl.RenderWithBorders()

	output := buf.String()
	if strings.Contains(output, "\033[") {
		t.Error("uncolored bordered table should not contain ANSI codes")
	}
}

func TestTableRowCount(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetHeaders("X")

	if tbl.RowCount() != 0 {
		t.Errorf("initial RowCount() = %d, want 0", tbl.RowCount())
	}

	tbl.AddRow("a")
	if tbl.RowCount() != 1 {
		t.Errorf("RowCount() = %d, want 1", tbl.RowCount())
	}

	tbl.AddRow("b")
	tbl.AddRow("c")
	if tbl.RowCount() != 3 {
		t.Errorf("RowCount() = %d, want 3", tbl.RowCount())
	}
}

// ---------------------------------------------------------------------------
// stripANSI tests
// ---------------------------------------------------------------------------

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no ANSI", "hello", "hello"},
		{"bold", ColorBold + "bold" + ColorReset, "bold"},
		{"red text", ColorRed + "error" + ColorReset, "error"},
		{"nested", ColorBold + ColorRed + "both" + ColorReset, "both"},
		{"empty", "", ""},
		{"only codes", ColorBold + ColorReset, ""},
		{"multiple segments", Red("a") + Green("b"), "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(tt.input)
			if got != tt.want {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// KeyValueTable tests
// ---------------------------------------------------------------------------

func TestKeyValueTableBasic(t *testing.T) {
	var buf bytes.Buffer
	kv := NewKeyValueTable(&buf).SetColor(false)
	kv.Add("Name", "Alice")
	kv.Add("Age", "30")
	kv.Render()

	output := buf.String()
	if !strings.Contains(output, "Name") {
		t.Error("should contain key 'Name'")
	}
	if !strings.Contains(output, "Alice") {
		t.Error("should contain value 'Alice'")
	}
	if !strings.Contains(output, ":") {
		t.Error("should contain colon separator")
	}
}

func TestKeyValueTableAlignment(t *testing.T) {
	var buf bytes.Buffer
	kv := NewKeyValueTable(&buf).SetColor(false)
	kv.Add("A", "1")
	kv.Add("LongKey", "2")
	kv.Render()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Both colons should be at the same position (keys right-padded)
	pos1 := strings.Index(lines[0], ":")
	pos2 := strings.Index(lines[1], ":")
	if pos1 != pos2 {
		t.Errorf("colons should be aligned: line1 at %d, line2 at %d", pos1, pos2)
	}
}

func TestKeyValueTableWithColor(t *testing.T) {
	var buf bytes.Buffer
	kv := NewKeyValueTable(&buf).SetColor(true)
	kv.Add("Key", "Val")
	kv.Render()

	output := buf.String()
	if !strings.Contains(output, ColorCyan) {
		t.Error("colored KV table should use cyan for keys")
	}
}

func TestKeyValueTableIndent(t *testing.T) {
	var buf bytes.Buffer
	kv := NewKeyValueTable(&buf).SetColor(false).SetIndent(4)
	kv.Add("Key", "Val")
	kv.Render()

	output := buf.String()
	if !strings.HasPrefix(output, "    ") {
		t.Errorf("indented output should start with 4 spaces, got %q", output)
	}
}

func TestKeyValueTableAddWithStatus(t *testing.T) {
	var buf bytes.Buffer
	kv := NewKeyValueTable(&buf).SetColor(true)
	kv.AddWithStatus("Status", "running")
	kv.Render()

	output := buf.String()
	if !strings.Contains(output, ColorGreen) {
		t.Error("running status should be green")
	}
}

func TestKeyValueTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	kv := NewKeyValueTable(&buf).SetColor(false)
	kv.Render() // no pairs, should not panic
	if buf.String() != "" {
		t.Errorf("empty KV table should produce no output, got %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// ListTable tests
// ---------------------------------------------------------------------------

func TestListTableBasic(t *testing.T) {
	var buf bytes.Buffer
	lt := NewListTable(&buf).SetColor(false)
	lt.Add("item1")
	lt.Add("item2")
	lt.Render()

	output := buf.String()
	if !strings.Contains(output, "- item1") {
		t.Error("should contain '- item1'")
	}
	if !strings.Contains(output, "- item2") {
		t.Error("should contain '- item2'")
	}
}

func TestListTableCustomBullet(t *testing.T) {
	var buf bytes.Buffer
	lt := NewListTable(&buf).SetColor(false).SetBullet("*")
	lt.Add("entry")
	lt.Render()

	output := buf.String()
	if !strings.Contains(output, "* entry") {
		t.Errorf("should contain '* entry', got %q", output)
	}
}

func TestListTableWithColor(t *testing.T) {
	var buf bytes.Buffer
	lt := NewListTable(&buf).SetColor(true)
	lt.Add("item")
	lt.Render()

	output := buf.String()
	if !strings.Contains(output, ColorCyan) {
		t.Error("colored list should use cyan for bullets")
	}
}

func TestListTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	lt := NewListTable(&buf).SetColor(false)
	lt.Render()
	if buf.String() != "" {
		t.Errorf("empty list should produce no output, got %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// TreeTable tests
// ---------------------------------------------------------------------------

func TestTreeTableBasic(t *testing.T) {
	root := &TreeNode{
		Name: "root",
		Children: []*TreeNode{
			{Name: "child1", Value: "v1"},
			{Name: "child2", Value: "v2"},
		},
	}

	var buf bytes.Buffer
	tt := NewTreeTable(&buf, root).SetColor(false)
	tt.Render()

	output := buf.String()
	if !strings.Contains(output, "root") {
		t.Error("should contain root node")
	}
	if !strings.Contains(output, "child1") {
		t.Error("should contain child1")
	}
	if !strings.Contains(output, "child2") {
		t.Error("should contain child2")
	}
	if !strings.Contains(output, "= v1") {
		t.Error("should contain value v1")
	}
}

func TestTreeTableNested(t *testing.T) {
	root := &TreeNode{
		Name: "root",
		Children: []*TreeNode{
			{
				Name: "parent",
				Children: []*TreeNode{
					{Name: "grandchild", Value: "deep"},
				},
			},
		},
	}

	var buf bytes.Buffer
	tt := NewTreeTable(&buf, root).SetColor(false)
	tt.Render()

	output := buf.String()
	if !strings.Contains(output, "grandchild") {
		t.Error("should contain nested grandchild")
	}
	if !strings.Contains(output, "= deep") {
		t.Error("should contain nested value")
	}
}

func TestTreeTableWithColor(t *testing.T) {
	root := &TreeNode{Name: "root", Value: "val"}

	var buf bytes.Buffer
	tt := NewTreeTable(&buf, root).SetColor(true)
	tt.Render()

	output := buf.String()
	if !strings.Contains(output, ColorBold) {
		t.Error("colored tree should use bold for names")
	}
	if !strings.Contains(output, ColorGreen) {
		t.Error("colored tree should use green for values")
	}
}

func TestTreeTableNilRoot(t *testing.T) {
	var buf bytes.Buffer
	tt := NewTreeTable(&buf, nil).SetColor(false)
	tt.Render() // should not panic
	if buf.String() != "" {
		t.Errorf("nil root should produce no output, got %q", buf.String())
	}
}

func TestTreeTableNoValue(t *testing.T) {
	root := &TreeNode{Name: "leaf"}

	var buf bytes.Buffer
	tt := NewTreeTable(&buf, root).SetColor(false)
	tt.Render()

	output := buf.String()
	if strings.Contains(output, "=") {
		t.Error("node without value should not contain '='")
	}
}

func TestTreeTableBranchCharacters(t *testing.T) {
	root := &TreeNode{
		Name: "root",
		Children: []*TreeNode{
			{Name: "first"},
			{Name: "last"},
		},
	}

	var buf bytes.Buffer
	tt := NewTreeTable(&buf, root).SetColor(false)
	tt.Render()

	output := buf.String()
	// Non-last children use |-- and last uses `--
	if !strings.Contains(output, "|-- ") {
		t.Error("non-last child should use '|-- ' branch")
	}
	if !strings.Contains(output, "`-- ") {
		t.Error("last child should use backtick branch")
	}
}

// ---------------------------------------------------------------------------
// ProgressBar tests
// ---------------------------------------------------------------------------

func TestProgressBarRender(t *testing.T) {
	var buf bytes.Buffer
	pb := &ProgressBar{
		total: 100,
		width: 20,
		out:   &buf,
		color: false,
	}

	pb.Update(50)
	output := buf.String()
	if !strings.Contains(output, "50%") {
		t.Errorf("50%% progress should show '50%%', got %q", output)
	}
	if !strings.Contains(output, "[") || !strings.Contains(output, "]") {
		t.Error("progress bar should have brackets")
	}
}

func TestProgressBarComplete(t *testing.T) {
	var buf bytes.Buffer
	pb := &ProgressBar{
		total: 10,
		width: 10,
		out:   &buf,
		color: false,
	}

	pb.Complete()
	output := buf.String()
	if !strings.Contains(output, "100%") {
		t.Errorf("complete should show 100%%, got %q", output)
	}
}

func TestProgressBarIncrement(t *testing.T) {
	var buf bytes.Buffer
	pb := &ProgressBar{
		total:   4,
		current: 0,
		width:   10,
		out:     &buf,
		color:   false,
	}

	pb.Increment()
	if pb.current != 1 {
		t.Errorf("current = %d, want 1", pb.current)
	}

	pb.Increment()
	if pb.current != 2 {
		t.Errorf("current = %d, want 2", pb.current)
	}
}

func TestProgressBarColorMode(t *testing.T) {
	var buf bytes.Buffer
	pb := &ProgressBar{
		total: 10,
		width: 10,
		out:   &buf,
		color: true,
	}

	pb.Update(5)
	output := buf.String()
	if !strings.Contains(output, ColorCyan) {
		t.Error("colored progress bar should contain cyan")
	}
}

// ---------------------------------------------------------------------------
// Table with long strings / edge cases
// ---------------------------------------------------------------------------

func TestTableWithLongStrings(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("Description")
	longStr := strings.Repeat("A", 200)
	tbl.AddRow(longStr)
	tbl.Render()

	output := buf.String()
	if !strings.Contains(output, longStr) {
		t.Error("table should handle long strings without truncation")
	}
}

func TestTableWithEmptyCells(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("A", "B")
	tbl.AddRow("", "")
	tbl.AddRow("x", "")
	tbl.AddRow("", "y")
	tbl.Render()

	if tbl.RowCount() != 3 {
		t.Errorf("RowCount() = %d, want 3", tbl.RowCount())
	}
}

func TestTableWithSpecialCharacters(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("Content")
	tbl.AddRow("line1\ttab")
	tbl.AddRow("pipe|char")
	tbl.AddRow("back\\slash")
	tbl.Render()

	output := buf.String()
	if !strings.Contains(output, "pipe|char") {
		t.Error("should handle pipe character in content")
	}
	if !strings.Contains(output, "back\\slash") {
		t.Error("should handle backslash in content")
	}
}

func TestTableAddRowChaining(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("A", "B")
	ret := tbl.AddRow("1", "2")
	if ret != tbl {
		t.Error("AddRow should return the same table for chaining")
	}
}

func TestTableSetHeadersChaining(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf)
	ret := tbl.SetHeaders("X")
	if ret != tbl {
		t.Error("SetHeaders should return the same table for chaining")
	}
}

func TestTableSetColorChaining(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf)
	ret := tbl.SetColor(false)
	if ret != tbl {
		t.Error("SetColor should return the same table for chaining")
	}
}

// ---------------------------------------------------------------------------
// RenderWithBorders edge cases
// ---------------------------------------------------------------------------

func TestTableRenderWithBordersANSICells(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("Status")
	tbl.AddRow(Green("ok"))
	tbl.RenderWithBorders()

	output := buf.String()
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "ok") {
		t.Error("bordered table should contain cell text after stripping ANSI")
	}
}

func TestTableRenderWithBordersSingleRow(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf).SetColor(false).SetHeaders("X")
	tbl.AddRow("val")
	tbl.RenderWithBorders()

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// top + header + middle + row + bottom = 5
	if len(lines) != 5 {
		t.Fatalf("single-row bordered table should have 5 lines, got %d", len(lines))
	}
}

// ---------------------------------------------------------------------------
// Integration: Writer + Table rendering
// ---------------------------------------------------------------------------

func TestWriterTableFormatRender(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf).SetFormat(FormatTable).SetColor(false)

	tw := &mockTableWriter{
		headers: []string{"ID", "Name", "Status"},
		rows: [][]string{
			{"1", "web", "running"},
			{"2", "api", "stopped"},
			{"3", "db", "pending"},
		},
	}

	if err := w.Write(tw); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	output := buf.String()
	for _, header := range []string{"ID", "NAME", "STATUS"} {
		if !strings.Contains(output, header) {
			t.Errorf("output should contain header %q", header)
		}
	}
	for _, name := range []string{"web", "api", "db"} {
		if !strings.Contains(output, name) {
			t.Errorf("output should contain %q", name)
		}
	}
}

func TestWriterWideFormatPassesWideFlagTrue(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf).SetFormat(FormatWide).SetColor(false)

	// Custom TableWriter that tracks if wide was set
	var gotWide bool
	tw := &wideTracker{gotWide: &gotWide}

	w.Write(tw)
	if !gotWide {
		t.Error("WriteTable with FormatWide should pass wide=true to WriteTable")
	}
}

type wideTracker struct {
	gotWide *bool
}

func (wt *wideTracker) WriteTable(t *Table, wide bool) {
	*wt.gotWide = wide
	t.SetHeaders("Col")
	t.AddRow("data")
}

func TestWriterTableFormatPassesWideFlagFalse(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter().SetOutput(&buf).SetFormat(FormatTable).SetColor(false)

	var gotWide bool
	tw := &wideTracker{gotWide: &gotWide}

	w.Write(tw)
	if gotWide {
		t.Error("WriteTable with FormatTable should pass wide=false")
	}
}
