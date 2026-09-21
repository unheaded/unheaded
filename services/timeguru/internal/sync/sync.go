// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package sync provides multi-format timeline serialization and file synchronization.
// Supports JSON, TOML, YAML, and Markdown output from a single Timeline source.
package sync

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gopkg.in/yaml.v3"

	"unheaded/services/timeguru/internal/timeline"
)

// Format represents a supported serialization format.
type Format string

const (
	FormatJSON     Format = "json"
	FormatTOML     Format = "toml"
	FormatYAML     Format = "yaml"
	FormatMarkdown Format = "markdown"
)

var (
	ErrUnsupportedFormat = errors.New("unsupported format")
	ErrNilTimeline       = errors.New("nil timeline")
	ErrEmptyDirectory    = errors.New("output directory cannot be empty")
)

// Syncer writes timeline data to multiple file formats in a target directory.
type Syncer struct {
	dir     string
	formats []Format
}

// NewSyncer creates a Syncer that writes to the given directory in the specified formats.
// If formats is empty, all 4 formats are used.
// Creates the directory if it doesn't exist.
// defaultSyncFormats is what NewSyncer writes when no formats are
// passed explicitly. Markdown is DELIBERATELY EXCLUDED — per the
// triple-mirror strategy in CLAUDE.md, timeline.md is the SOURCE of
// truth and timeline.{json,yaml,toml} are derived FROM it. Letting
// Sync regenerate timeline.md from a stale Timeline struct (e.g. an
// in-memory one with empty Milestones) overwrites Stevie's hand-edited
// content with stubs — see kanban: debt-timeline-sync (2026-05-04).
//
// Callers who genuinely need to render Markdown OUTPUT (e.g. previewing
// what an upcoming hand-edit would produce) can pass FormatMarkdown
// explicitly; the encoder is still functional. Just not by default.
var defaultSyncFormats = []Format{FormatJSON, FormatTOML, FormatYAML}

func NewSyncer(dir string, formats ...Format) (*Syncer, error) {
	if dir == "" {
		return nil, ErrEmptyDirectory
	}
	// Ensure output directory exists
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create sync directory %s: %w", dir, err)
	}
	if len(formats) == 0 {
		formats = defaultSyncFormats
	}
	return &Syncer{dir: dir, formats: formats}, nil
}

// SyncResult holds the outcome of a sync operation.
type SyncResult struct {
	FilesWritten []string
	Errors       []error
}

// Sync writes the timeline to all configured formats.
func (s *Syncer) Sync(tl *timeline.Timeline) *SyncResult {
	result := &SyncResult{}

	if tl == nil {
		result.Errors = append(result.Errors, ErrNilTimeline)
		return result
	}

	for _, f := range s.formats {
		path := filepath.Join(s.dir, "timeline."+string(f))
		if f == FormatMarkdown {
			path = filepath.Join(s.dir, "timeline.md")
		}

		data, err := Encode(tl, f)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", f, err))
			continue
		}

		if err := os.WriteFile(path, data, 0640); err != nil { // #nosec G306 -- 0640 — group-readable only, no world access
			result.Errors = append(result.Errors, fmt.Errorf("write %s: %w", path, err))
			continue
		}

		result.FilesWritten = append(result.FilesWritten, path)
	}

	return result
}

// Encode serializes a Timeline to the given format.
func Encode(tl *timeline.Timeline, f Format) ([]byte, error) {
	if tl == nil {
		return nil, ErrNilTimeline
	}

	switch f {
	case FormatJSON:
		return encodeJSON(tl)
	case FormatTOML:
		return encodeTOML(tl)
	case FormatYAML:
		return encodeYAML(tl)
	case FormatMarkdown:
		return []byte(encodeMarkdown(tl)), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, f)
	}
}

// Decode deserializes a Timeline from the given format.
// Markdown decoding is not supported here (use the parser package).
func Decode(data []byte, f Format) (*timeline.Timeline, error) {
	switch f {
	case FormatJSON:
		return decodeJSON(data)
	case FormatTOML:
		return decodeTOML(data)
	case FormatYAML:
		return decodeYAML(data)
	case FormatMarkdown:
		return nil, fmt.Errorf("%w: use parser package for markdown import", ErrUnsupportedFormat)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, f)
	}
}

// DetectFormat infers format from a file extension.
func DetectFormat(filename string) (Format, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".json":
		return FormatJSON, nil
	case ".toml":
		return FormatTOML, nil
	case ".yaml", ".yml":
		return FormatYAML, nil
	case ".md", ".markdown":
		return FormatMarkdown, nil
	default:
		return "", fmt.Errorf("%w: extension %q", ErrUnsupportedFormat, ext)
	}
}

// LogSyncResult logs the outcome of a sync operation.
func LogSyncResult(r *SyncResult) {
	for _, f := range r.FilesWritten {
		log.Info().Str("file", f).Msg("sync wrote file")
	}
	for _, e := range r.Errors {
		log.Error().Err(e).Msg("sync error")
	}
}

// ============================================================================
// ENCODERS
// ============================================================================

func encodeJSON(tl *timeline.Timeline) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tl); err != nil {
		return nil, fmt.Errorf("json encode: %w", err)
	}
	return buf.Bytes(), nil
}

func encodeTOML(tl *timeline.Timeline) ([]byte, error) {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	if err := enc.Encode(tl); err != nil {
		return nil, fmt.Errorf("toml encode: %w", err)
	}
	return buf.Bytes(), nil
}

func encodeYAML(tl *timeline.Timeline) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(tl); err != nil {
		return nil, fmt.Errorf("yaml encode: %w", err)
	}
	return buf.Bytes(), nil
}

func encodeMarkdown(tl *timeline.Timeline) string {
	var sb strings.Builder

	sb.WriteString("# The Unheaded Chronicles\n\n")
	sb.WriteString("## A Living Grimoire of the Kingdom's Journey\n\n")
	fmt.Fprintf(&sb, "**STATUS:** %s\n", strings.ToUpper(tl.Status))
	fmt.Fprintf(&sb, "**LAST UPDATED:** %s\n\n", tl.LastUpdated.Format("January 2, 2006"))
	sb.WriteString("---\n\n")

	if tl.Vision != "" {
		sb.WriteString("## The Founding Vision\n\n")
		fmt.Fprintf(&sb, "*%s*\n\n", tl.Vision)
		sb.WriteString("---\n\n")
	}

	for i, phase := range tl.Phases {
		if phase == nil {
			continue
		}
		statusEmoji := "📋"
		switch phase.Status {
		case "completed":
			statusEmoji = "✅"
		case "in_progress":
			statusEmoji = "🚀"
		case "blocked":
			statusEmoji = "🚫"
		}

		fmt.Fprintf(&sb, "### Age %d: %s (%s %s)\n\n",
			i, phase.Name, statusEmoji, strings.ToUpper(phase.Status))

		if phase.Description != "" {
			fmt.Fprintf(&sb, "%s\n\n", phase.Description)
		}
		if phase.Progress > 0 {
			fmt.Fprintf(&sb, "**Progress:** %d%%\n\n", phase.Progress)
		}
	}

	if len(tl.Milestones) > 0 {
		sb.WriteString("## Milestones\n\n")

		for _, m := range tl.Milestones {
			if m == nil {
				continue
			}
			statusEmoji := "⏳"
			switch m.Status {
			case "completed":
				statusEmoji = "✅"
			case "in_progress":
				statusEmoji = "🔄"
			case "blocked":
				statusEmoji = "🚫"
			}

			fmt.Fprintf(&sb, "### %s %s\n\n", statusEmoji, m.Name)

			if m.Description != "" {
				fmt.Fprintf(&sb, "%s\n\n", m.Description)
			}
			if m.ETA != nil {
				fmt.Fprintf(&sb, "**ETA:** %s\n", m.ETA.Format("Jan 2, 2006"))
			}
			if m.Owner != "" {
				fmt.Fprintf(&sb, "**Owner:** %s\n", m.Owner)
			}
			if m.Risk != "" {
				fmt.Fprintf(&sb, "**Risk:** %s\n", cases.Title(language.English).String(m.Risk))
			}
			fmt.Fprintf(&sb, "**Progress:** %d%%\n", m.Progress)
			fmt.Fprintf(&sb, "**Status:** %s\n\n", m.Status)

			if len(m.Tasks) > 0 {
				sb.WriteString("**Tasks:**\n")
				for _, task := range m.Tasks {
					checkbox := "[ ]"
					if m.Progress == 100 {
						checkbox = "[x]"
					}
					fmt.Fprintf(&sb, "- %s %s\n", checkbox, task)
				}
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("---\n\n")
	fmt.Fprintf(&sb, "*Synced: %s*\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))

	return sb.String()
}

// ============================================================================
// DECODERS
// ============================================================================

func decodeJSON(data []byte) (*timeline.Timeline, error) {
	var tl timeline.Timeline
	if err := json.Unmarshal(data, &tl); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	if err := tl.Validate(); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	return &tl, nil
}

func decodeTOML(data []byte) (*timeline.Timeline, error) {
	var tl timeline.Timeline
	if err := toml.Unmarshal(data, &tl); err != nil {
		return nil, fmt.Errorf("toml decode: %w", err)
	}
	if err := tl.Validate(); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	return &tl, nil
}

func decodeYAML(data []byte) (*timeline.Timeline, error) {
	var tl timeline.Timeline
	if err := yaml.Unmarshal(data, &tl); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}
	if err := tl.Validate(); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}
	return &tl, nil
}
