// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package output provides output formatting for THE GAUNTLETS CLI.
// Multiple formats supported: table, json, yaml for different use cases.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format represents the output format type.
type Format string

const (
	// FormatTable renders data as human-readable tables.
	FormatTable Format = "table"
	// FormatJSON renders data as JSON.
	FormatJSON Format = "json"
	// FormatYAML renders data as YAML.
	FormatYAML Format = "yaml"
	// FormatWide renders extended table with more columns.
	FormatWide Format = "wide"
)

// ParseFormat parses a format string.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(s) {
	case "table", "":
		return FormatTable, nil
	case "json":
		return FormatJSON, nil
	case "yaml", "yml":
		return FormatYAML, nil
	case "wide":
		return FormatWide, nil
	default:
		return FormatTable, fmt.Errorf("unknown format: %s (valid: table, json, yaml, wide)", s)
	}
}

// Writer handles output formatting and writing.
type Writer struct {
	out    io.Writer
	errOut io.Writer
	format Format
	color  bool
}

// Out returns the underlying output writer.
func (w *Writer) Out() io.Writer {
	return w.out
}

// NewWriter creates a new Writer with default settings.
func NewWriter() *Writer {
	return &Writer{
		out:    os.Stdout,
		errOut: os.Stderr,
		format: FormatTable,
		color:  IsTerminal(),
	}
}

// SetOutput sets the output writer.
func (w *Writer) SetOutput(out io.Writer) *Writer {
	w.out = out
	return w
}

// SetErrorOutput sets the error output writer.
func (w *Writer) SetErrorOutput(errOut io.Writer) *Writer {
	w.errOut = errOut
	return w
}

// SetFormat sets the output format.
func (w *Writer) SetFormat(format Format) *Writer {
	w.format = format
	return w
}

// SetColor enables or disables color output.
func (w *Writer) SetColor(color bool) *Writer {
	w.color = color
	return w
}

// Format returns the current format.
func (w *Writer) Format() Format {
	return w.format
}

// Color returns whether color is enabled.
func (w *Writer) Color() bool {
	return w.color
}

// Write writes formatted output based on the current format.
func (w *Writer) Write(data interface{}) error {
	switch w.format {
	case FormatJSON:
		return w.WriteJSON(data)
	case FormatYAML:
		return w.WriteYAML(data)
	default:
		// For table format, data should implement TableWriter
		if tw, ok := data.(TableWriter); ok {
			return w.WriteTable(tw)
		}
		// Fallback to JSON for complex types
		return w.WriteJSON(data)
	}
}

// WriteJSON writes data as JSON.
func (w *Writer) WriteJSON(data interface{}) error {
	encoder := json.NewEncoder(w.out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// WriteYAML writes data as YAML.
func (w *Writer) WriteYAML(data interface{}) error {
	encoder := yaml.NewEncoder(w.out)
	encoder.SetIndent(2)
	defer encoder.Close()
	return encoder.Encode(data)
}

// WriteTable writes data as a formatted table.
func (w *Writer) WriteTable(tw TableWriter) error {
	table := NewTable(w.out).SetColor(w.color)
	tw.WriteTable(table, w.format == FormatWide)
	table.Render()
	return nil
}

// WriteString writes a plain string.
func (w *Writer) WriteString(s string) error {
	_, err := fmt.Fprint(w.out, s)
	return err
}

// WriteStringln writes a string with newline.
func (w *Writer) WriteStringln(s string) error {
	_, err := fmt.Fprintln(w.out, s)
	return err
}

// WriteError writes an error message.
func (w *Writer) WriteError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if w.color {
		msg = ColorRed + "Error: " + ColorReset + msg
	} else {
		msg = "Error: " + msg
	}
	fmt.Fprintln(w.errOut, msg)
}

// WriteWarning writes a warning message.
func (w *Writer) WriteWarning(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if w.color {
		msg = ColorYellow + "Warning: " + ColorReset + msg
	} else {
		msg = "Warning: " + msg
	}
	fmt.Fprintln(w.errOut, msg)
}

// WriteSuccess writes a success message.
func (w *Writer) WriteSuccess(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if w.color {
		msg = ColorGreen + msg + ColorReset
	}
	fmt.Fprintln(w.out, msg)
}

// WriteInfo writes an info message.
func (w *Writer) WriteInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if w.color {
		msg = ColorCyan + msg + ColorReset
	}
	fmt.Fprintln(w.out, msg)
}

// TableWriter is implemented by types that can render themselves as tables.
type TableWriter interface {
	WriteTable(t *Table, wide bool)
}

// StatusOutput represents a status that can be colorized.
type StatusOutput struct {
	Status string
}

// Colorize returns the status with appropriate color.
func (s StatusOutput) Colorize(color bool) string {
	if !color {
		return s.Status
	}
	switch strings.ToLower(s.Status) {
	case "running", "healthy", "active", "success", "ok":
		return ColorGreen + s.Status + ColorReset
	case "stopped", "error", "failed", "unhealthy":
		return ColorRed + s.Status + ColorReset
	case "pending", "starting", "stopping", "warning":
		return ColorYellow + s.Status + ColorReset
	case "frozen":
		return ColorCyan + s.Status + ColorReset
	default:
		return s.Status
	}
}

// ProgressBar displays a simple progress indicator.
type ProgressBar struct {
	total   int
	current int
	width   int
	out     io.Writer
	color   bool
}

// NewProgressBar creates a new progress bar.
func NewProgressBar(total int, width int) *ProgressBar {
	return &ProgressBar{
		total: total,
		width: width,
		out:   os.Stdout,
		color: IsTerminal(),
	}
}

// Update updates the progress bar.
func (p *ProgressBar) Update(current int) {
	p.current = current
	p.render()
}

// Increment increments the progress bar by 1.
func (p *ProgressBar) Increment() {
	p.current++
	p.render()
}

// Complete marks the progress bar as complete.
func (p *ProgressBar) Complete() {
	p.current = p.total
	p.render()
	fmt.Fprintln(p.out)
}

func (p *ProgressBar) render() {
	percent := float64(p.current) / float64(p.total)
	filled := int(percent * float64(p.width))
	empty := p.width - filled

	bar := strings.Repeat("=", filled) + strings.Repeat("-", empty)
	if p.color {
		fmt.Fprintf(p.out, "\r%s[%s]%s %3.0f%%", ColorCyan, bar, ColorReset, percent*100)
	} else {
		fmt.Fprintf(p.out, "\r[%s] %3.0f%%", bar, percent*100)
	}
}

// Spinner displays a spinning indicator.
type Spinner struct {
	frames   []string
	current  int
	message  string
	out      io.Writer
	color    bool
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewSpinner creates a new spinner.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames:  []string{"|", "/", "-", "\\"},
		message: message,
		out:     os.Stdout,
		color:   IsTerminal(),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Start starts the spinner animation.
func (s *Spinner) Start() {
	go func() {
		defer close(s.doneCh)
		for {
			select {
			case <-s.stopCh:
				return
			default:
				s.render()
				s.current = (s.current + 1) % len(s.frames)
				// Small delay between frames
				select {
				case <-s.stopCh:
					return
				default:
				}
			}
		}
	}()
}

// Stop stops the spinner and clears the line.
func (s *Spinner) Stop() {
	close(s.stopCh)
	<-s.doneCh
	fmt.Fprintf(s.out, "\r%s\r", strings.Repeat(" ", len(s.message)+4))
}

// StopWithMessage stops the spinner and displays a message.
func (s *Spinner) StopWithMessage(msg string) {
	close(s.stopCh)
	<-s.doneCh
	fmt.Fprintf(s.out, "\r%s\n", msg)
}

func (s *Spinner) render() {
	frame := s.frames[s.current]
	if s.color {
		fmt.Fprintf(s.out, "\r%s%s%s %s", ColorCyan, frame, ColorReset, s.message)
	} else {
		fmt.Fprintf(s.out, "\r%s %s", frame, s.message)
	}
}

// Box renders content in a box.
func Box(title string, content string, color bool) string {
	lines := strings.Split(content, "\n")
	maxWidth := len(title)
	for _, line := range lines {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}
	maxWidth += 4 // padding

	var sb strings.Builder

	// Top border
	if color {
		sb.WriteString(ColorCyan)
	}
	sb.WriteString("+" + strings.Repeat("-", maxWidth) + "+\n")

	// Title
	if title != "" {
		padding := maxWidth - len(title) - 2
		sb.WriteString("| " + title + strings.Repeat(" ", padding) + " |\n")
		sb.WriteString("+" + strings.Repeat("-", maxWidth) + "+\n")
	}

	// Content
	for _, line := range lines {
		padding := maxWidth - len(line) - 2
		sb.WriteString("| " + line + strings.Repeat(" ", padding) + " |\n")
	}

	// Bottom border
	sb.WriteString("+" + strings.Repeat("-", maxWidth) + "+")
	if color {
		sb.WriteString(ColorReset)
	}
	sb.WriteString("\n")

	return sb.String()
}
