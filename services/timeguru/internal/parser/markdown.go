// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package parser provides markdown parsing for timeline files
// THE ORACLE'S ANTRE - Where timeline.md becomes prophecy
package parser

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"unheaded/services/timeguru/internal/timeline"
)

// ============================================================================
// ERRORS
// ============================================================================

var (
	ErrEmptyPath     = errors.New("file path cannot be empty")
	ErrFileNotFound  = errors.New("timeline file not found")
	ErrParseError    = errors.New("failed to parse timeline")
	ErrInvalidFormat = errors.New("invalid timeline format")
)

// ============================================================================
// REGEX PATTERNS (The Parsing Runes)
// ============================================================================

var (
	// Phase headers: ### Age 1: The Alpha Ascension or ### Phase 1: Alpha
	// Also captures a trailing parenthesised status: (COMPLETE ✓),
	// (🔄 IN PROGRESS), (📋 PLANNED), (IN PROGRESS 🚀).
	//
	// The status group matches any parenthesised run containing a status
	// KEYWORD, rather than enumerating the emoji that may sit beside it. The
	// previous class `[A-Z\s✓✅🚀_-]+` listed ✓✅🚀 but not 🔄 or 📋 — the two
	// markers references/timeline.md actually uses — so those headers failed to
	// match the group at all. `(.+?)` then swallowed the marker into the phase
	// name and the status silently fell through to its default.
	//
	// The keywords are word-bounded so that ordinary parenthetical prose does
	// not get eaten as a marker: without \b, "(incomplete)" contains COMPLETE
	// and "(unblocked)" contains BLOCK, and parseStatus would report the
	// opposite of what the heading says while stripping the remark from the
	// phase name.
	phaseHeaderRe = regexp.MustCompile(`^###\s+(Age|Phase|Epoch)\s+(\d+(?:\.\d+)?):?\s+(.+?)\s*(?:\(([^()]*\b(?i:COMPLETED?|IN[ _-]PROGRESS|PROGRESS|PLANNED|BLOCK(?:ED)?)\b[^()]*)\))?$`)

	// Milestone headers: #### Epoch 1.1: The Whispering Void Awakens
	milestoneHeaderRe = regexp.MustCompile(`^####\s+(Epoch|Milestone)\s+(\d+(?:\.\d+)?):?\s+(.+)$`)

	// Checkbox items: - [x] Task description or - [ ] Task description
	checkboxRe = regexp.MustCompile(`^-\s+\[([ xX])\]\s+(.+)$`)

	// Bullet milestones. references/timeline.md stopped using "#### Epoch"
	// headings long before 2026-09-21; work items live as top-level bullets
	// under a phase, grouped by a bold section line:
	//
	//   **Completed sub-items:**      -> completed
	//   **Remaining for Age 3:**      -> planned
	//   (no section line)             -> the phase's own status
	//
	// The parser read none of that, reported milestones=0 on the real file,
	// and the kanban board rendered six phase cards and nothing else.
	bulletSectionRe = regexp.MustCompile(`^\s*\*\*\s*([A-Za-z][^*]*?)\s*:?\s*\*\*\s*:?\s*$`)
	bulletItemRe    = regexp.MustCompile(`^-\s+(?:\[[ xX]\]\s+)?(\S.*)$`)

	// Status markers in text: (COMPLETE ✓), (IN PROGRESS 🚀), (PLANNED)
	statusCompleteRe   = regexp.MustCompile(`(?i)\(?COMPLETE[D]?\s*[✓✅]?\)?`)
	statusInProgressRe = regexp.MustCompile(`(?i)\(?IN[\s_-]?PROGRESS\s*[🚀]?\)?`)
	statusPlannedRe    = regexp.MustCompile(`(?i)\(?PLANNED\)?`)
	statusBlockedRe    = regexp.MustCompile(`(?i)\(?BLOCKED\)?`)

	// ETA patterns: ETA: Feb 3-4, 2026 or *ETA: Feb 8, 2026*
	etaRe = regexp.MustCompile(`(?i)ETA:?\s*([A-Za-z]+\s+\d+(?:-\d+)?,?\s*\d{4})`)

	// Risk patterns: Risk: Medium or **Risk:** High
	riskRe = regexp.MustCompile(`(?i)(?:\*{2})?Risk:(?:\*{2})?\s*(low|medium|high)`)

	// Progress patterns: 25% FORGED or 60% complete.
	//
	// The fractional part is captured so a decimal does not truncate to its
	// fraction: `(\d+)%` matched "7%" inside "73.7%" and reported progress 7.
	progressRe = regexp.MustCompile(`(\d+)(?:\.\d+)?%\s*(?:FORGED|complete|done)?`)

	// An explicit declaration: "**Progress:** ~70%" / "Progress: 70%".
	// This takes precedence over any percentage merely mentioned in the section
	// body — every match used to overwrite the field, so the last incidental
	// number in a section won.
	progressDeclRe = regexp.MustCompile(`(?i)^\s*[*_]{0,2}Progress[*_]{0,2}\s*:?\s*[*_]{0,2}\s*[~≈]?\s*(\d+)(?:\.\d+)?\s*%`)

	// Owner patterns: Owner: Agent 5 or **Owner:** The Architect
	ownerRe = regexp.MustCompile(`(?i)(?:\*{2})?Owner:(?:\*{2})?\s*(.+?)(?:\s*$|,|\|)`)

	// Version from header: **STATUS:** Alpha Development
	versionRe = regexp.MustCompile(`(?i)(?:STATUS|Version):?\s*(.+?)$`)
)

// ============================================================================
// PARSER
// ============================================================================

// MarkdownParser parses timeline.md files into Timeline structs
type MarkdownParser struct {
	filePath string
}

// NewMarkdownParser creates a new parser with defensive validation
func NewMarkdownParser(filePath string) (*MarkdownParser, error) {
	if filePath == "" {
		return nil, ErrEmptyPath
	}

	// Check file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrFileNotFound, filePath)
	}

	return &MarkdownParser{filePath: filePath}, nil
}

// Parse reads and parses the timeline file
func (p *MarkdownParser) Parse() (*timeline.Timeline, error) {
	if p == nil {
		return nil, errors.New("nil receiver")
	}

	file, err := os.Open(p.filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	return p.ParseReader(bufio.NewScanner(file))
}

// ParseContent parses timeline from string content (useful for testing)
func (p *MarkdownParser) ParseContent(content string) (*timeline.Timeline, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	return p.ParseReader(scanner)
}

// ParseReader parses timeline from a scanner
func (p *MarkdownParser) ParseReader(scanner *bufio.Scanner) (*timeline.Timeline, error) {
	tl := &timeline.Timeline{
		Version:     "1.0.0",
		LastUpdated: time.Now(),
		Status:      "alpha",
		Phases:      make([]*timeline.Phase, 0),
		Milestones:  make([]*timeline.Milestone, 0),
		Metadata:    make(map[string]string),
	}

	var currentPhase *timeline.Phase
	// True when the current phase's status came from its header marker, so
	// body text must not override it.
	var phaseStatusFromHeader bool
	// True once the current phase declared its own "Progress: N%" line.
	var phaseProgressDeclared bool
	var milestoneProgressDeclared bool // same guard, one heading level down
	// Status that bullet milestones under the current phase inherit. Reset by
	// every phase header; set by a bold section line.
	var bulletStatus string
	var currentMilestone *timeline.Milestone
	var lineNum int

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Extract version/status from early headers
		if versionMatch := versionRe.FindStringSubmatch(line); versionMatch != nil {
			status := strings.ToLower(strings.TrimSpace(versionMatch[1]))
			if strings.Contains(status, "alpha") {
				tl.Status = "alpha"
			} else if strings.Contains(status, "beta") {
				tl.Status = "beta"
			} else if strings.Contains(status, "mvp") {
				tl.Status = "mvp"
			} else if strings.Contains(status, "production") {
				tl.Status = "production"
			}
		}

		// Parse phase headers
		if phaseMatch := phaseHeaderRe.FindStringSubmatch(line); phaseMatch != nil {
			// Save previous milestone if exists (before resetting phase)
			if currentMilestone != nil {
				tl.Milestones = append(tl.Milestones, currentMilestone)
				if currentPhase != nil {
					currentPhase.Milestones = append(currentPhase.Milestones, currentMilestone.ID)
				}
				currentMilestone = nil
			}

			// Save previous phase if exists
			if currentPhase != nil {
				tl.Phases = append(tl.Phases, currentPhase)
			}

			phaseID := fmt.Sprintf("phase-%s", phaseMatch[2])
			phaseName := strings.TrimSpace(phaseMatch[3])

			currentPhase = &timeline.Phase{
				ID:         phaseID,
				Name:       phaseName,
				Status:     "planned",
				Progress:   0,
				Milestones: make([]string, 0),
			}

			// Check for status in header
			phaseStatusFromHeader = false
			phaseProgressDeclared = false
			bulletStatus = ""
			if len(phaseMatch) > 4 && phaseMatch[4] != "" {
				currentPhase.Status = parseStatus(phaseMatch[4])
				phaseStatusFromHeader = true
			}

			continue
		}

		// Parse milestone headers
		if milestoneMatch := milestoneHeaderRe.FindStringSubmatch(line); milestoneMatch != nil {
			// Save previous milestone if exists
			if currentMilestone != nil {
				tl.Milestones = append(tl.Milestones, currentMilestone)
				if currentPhase != nil {
					currentPhase.Milestones = append(currentPhase.Milestones, currentMilestone.ID)
				}
			}

			milestoneID := fmt.Sprintf("milestone-%s", milestoneMatch[2])
			milestoneName := strings.TrimSpace(milestoneMatch[3])
			milestoneProgressDeclared = false

			currentMilestone = &timeline.Milestone{
				ID:       milestoneID,
				Name:     milestoneName,
				Status:   "pending",
				Progress: 0,
				Tasks:    make([]string, 0),
			}

			continue
		}

		// Parse status from line content
		if currentMilestone != nil || currentPhase != nil {
			status := extractStatus(line)
			if status != "" {
				if currentMilestone != nil && currentMilestone.Status == "pending" {
					currentMilestone.Status = status
				} else if currentPhase != nil && !phaseStatusFromHeader && currentPhase.Status == "planned" {
					// Only infer a phase status from body text when the header did
					// not state one. Without this guard a phase declared PLANNED is
					// flipped to completed by the first body line that happens to
					// contain the word COMPLETE — which is how "The Scaling Era
					// (📋 PLANNED)" came out as status "completed", 0%%.
					currentPhase.Status = status
				}
			}
		}

		// Parse ETA
		if etaMatch := etaRe.FindStringSubmatch(line); etaMatch != nil {
			if currentMilestone != nil {
				// Only set the ETA when the date actually parsed; a zero time
				// would otherwise be indistinguishable from "no ETA" once the
				// pointer is non-nil.
				if d := parseDate(etaMatch[1]); !d.IsZero() {
					currentMilestone.ETA = &d
				}
			}
		}

		// Parse Risk
		if riskMatch := riskRe.FindStringSubmatch(line); riskMatch != nil {
			if currentMilestone != nil {
				currentMilestone.Risk = strings.ToLower(riskMatch[1])
			}
		}

		// Parse Progress. An explicit "Progress: N%" declaration wins and cannot
		// be overwritten by a percentage mentioned later in the same section.
		if declMatch := progressDeclRe.FindStringSubmatch(line); declMatch != nil {
			progress := 0
			_, _ = fmt.Sscanf(declMatch[1], "%d", &progress)
			if currentMilestone != nil {
				currentMilestone.Progress = progress
				milestoneProgressDeclared = true
			} else if currentPhase != nil {
				currentPhase.Progress = progress
				phaseProgressDeclared = true
			}
		} else if progressMatch := progressRe.FindStringSubmatch(line); progressMatch != nil {
			progress := 0
			_, _ = fmt.Sscanf(progressMatch[1], "%d", &progress)
			if currentMilestone != nil {
				if !milestoneProgressDeclared {
					currentMilestone.Progress = progress
				}
			} else if currentPhase != nil && !phaseProgressDeclared {
				currentPhase.Progress = progress
			}
		}

		// Parse Owner
		if ownerMatch := ownerRe.FindStringSubmatch(line); ownerMatch != nil {
			if currentMilestone != nil {
				currentMilestone.Owner = strings.TrimSpace(ownerMatch[1])
			}
		}

		// Bold section lines under a phase decide the status of the bullets
		// that follow them.
		if currentPhase != nil && currentMilestone == nil {
			if secMatch := bulletSectionRe.FindStringSubmatch(line); secMatch != nil {
				bulletStatus = bulletSectionStatus(secMatch[1])
				continue
			}
		}

		// Top-level bullets under a phase with no "####" milestone open are
		// milestones in their own right.
		if currentPhase != nil && currentMilestone == nil {
			if bm := bulletItemRe.FindStringSubmatch(line); bm != nil {
				m := bulletMilestone(currentPhase, bm[1], bulletStatus, line)
				tl.Milestones = append(tl.Milestones, m)
				currentPhase.Milestones = append(currentPhase.Milestones, m.ID)
				continue
			}
		}

		// Parse checkbox tasks
		if checkboxMatch := checkboxRe.FindStringSubmatch(line); checkboxMatch != nil {
			isComplete := strings.ToLower(checkboxMatch[1]) == "x"
			taskName := strings.TrimSpace(checkboxMatch[2])

			if currentMilestone != nil {
				currentMilestone.Tasks = append(currentMilestone.Tasks, taskName)

				// Update progress based on completed tasks ratio
				completedCount := 0
				for _, task := range currentMilestone.Tasks {
					// Check if this task was marked complete in the original
					if isComplete && task == taskName {
						completedCount++
					}
				}
				// Simple heuristic: each completed checkbox contributes to progress
				if len(currentMilestone.Tasks) > 0 && isComplete {
					// Increment progress proportionally
					if currentMilestone.Progress == 0 {
						currentMilestone.Progress = 10
					}
				}
			}
		}
	}

	// Don't forget the last items
	if currentMilestone != nil {
		tl.Milestones = append(tl.Milestones, currentMilestone)
		if currentPhase != nil {
			currentPhase.Milestones = append(currentPhase.Milestones, currentMilestone.ID)
		}
	}
	if currentPhase != nil {
		tl.Phases = append(tl.Phases, currentPhase)
	}

	// Calculate overall progress
	if len(tl.Milestones) > 0 {
		totalProgress := 0
		for _, m := range tl.Milestones {
			totalProgress += m.Progress
		}
		// Store in metadata
		avgProgress := totalProgress / len(tl.Milestones)
		tl.Metadata["overall_progress"] = fmt.Sprintf("%d%%", avgProgress)
	}

	// Validate before returning
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}

	return tl, nil
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// parseStatus converts status text to normalized status string
func parseStatus(text string) string {
	// Word-bounded for the same reason as phaseHeaderRe: "INCOMPLETE" contains
	// "COMPLETE" and "UNBLOCKED" contains "BLOCK".
	switch {
	case statusWordCompleteRe.MatchString(text):
		return "completed"
	case statusWordProgressRe.MatchString(text):
		return "in_progress"
	case statusWordBlockedRe.MatchString(text):
		return "blocked"
	default:
		return "planned"
	}
}

var (
	statusWordCompleteRe = regexp.MustCompile(`(?i)\bCOMPLETED?\b`)
	statusWordProgressRe = regexp.MustCompile(`(?i)\b(?:IN[ _-])?PROGRESS\b`)
	statusWordBlockedRe  = regexp.MustCompile(`(?i)\bBLOCK(?:ED)?\b`)
)

// extractStatus extracts status from line content
func extractStatus(line string) string {
	if statusCompleteRe.MatchString(line) {
		return "completed"
	}
	if statusInProgressRe.MatchString(line) {
		return "in_progress"
	}
	if statusBlockedRe.MatchString(line) {
		return "blocked"
	}
	if statusPlannedRe.MatchString(line) {
		return "planned"
	}
	return ""
}

// parseDate parses date strings like "Feb 3-4, 2026" or "February 8, 2026"
func parseDate(dateStr string) time.Time {
	dateStr = strings.TrimSpace(dateStr)

	// Try ISO format first (before range detection since ISO dates contain hyphens)
	if t, err := time.Parse("2006-01-02", dateStr); err == nil {
		return t
	}

	// Handle range dates by taking first date (e.g., "Feb 3-4, 2026")
	// Only treat as range if hyphen is between digits (day range) not ISO format
	rangeRe := regexp.MustCompile(`^([A-Za-z]+\s+\d+)-(\d+),?\s*(\d{4})$`)
	if rangeMatch := rangeRe.FindStringSubmatch(dateStr); rangeMatch != nil {
		// Reconstruct as "Feb 3, 2026" from "Feb 3-4, 2026"
		dateStr = rangeMatch[1] + ", " + rangeMatch[3]
	}

	// Try various formats
	formats := []string{
		"Jan 2, 2006",
		"January 2, 2006",
		"Jan 2 2006",
		"January 2 2006",
		"02 Jan 2006",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return t
		}
	}

	// Default to zero time if unparseable
	return time.Time{}
}

// ============================================================================
// FILE WATCHER (The Oracle's Eye)
// ============================================================================

// FileWatcher watches a timeline file for changes
type FileWatcher struct {
	filePath    string
	lastModTime time.Time
	parser      *MarkdownParser
}

// NewFileWatcher creates a watcher for timeline file changes
func NewFileWatcher(filePath string) (*FileWatcher, error) {
	parser, err := NewMarkdownParser(filePath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	return &FileWatcher{
		filePath:    filePath,
		lastModTime: info.ModTime(),
		parser:      parser,
	}, nil
}

// HasChanged checks if the file has been modified since last check
func (w *FileWatcher) HasChanged() (bool, error) {
	info, err := os.Stat(w.filePath)
	if err != nil {
		return false, fmt.Errorf("stat file: %w", err)
	}

	if info.ModTime().After(w.lastModTime) {
		w.lastModTime = info.ModTime()
		return true, nil
	}

	return false, nil
}

// Parse parses the watched file
func (w *FileWatcher) Parse() (*timeline.Timeline, error) {
	return w.parser.Parse()
}

// bulletSectionStatus maps a bold section line ("Completed sub-items",
// "Remaining for Age 3") to the status its bullets inherit. Unknown headings
// return "" so the phase's own status applies.
func bulletSectionStatus(heading string) string {
	h := strings.ToUpper(heading)
	switch {
	case strings.Contains(h, "COMPLETE") || strings.Contains(h, "DONE") || strings.Contains(h, "SHIPPED"):
		return "completed"
	case strings.Contains(h, "REMAINING") || strings.Contains(h, "PLANNED") || strings.Contains(h, "TODO") || strings.Contains(h, "NEXT"):
		return "pending" // milestone vocabulary; phases say "planned"
	case strings.Contains(h, "IN PROGRESS") || strings.Contains(h, "IN-PROGRESS") || strings.Contains(h, "ACTIVE"):
		return "in_progress"
	case strings.Contains(h, "BLOCKED"):
		return "blocked"
	}
	return ""
}

// bulletMilestone builds a milestone from one bullet line.
//
// ID: phase id + 8 hex of the bullet text's SHA-256. Stable for as long as
// the text is unchanged, which is what kanban's add/update/remove diff needs;
// an edit reads as remove+add, which is honest. Positional ids would shift
// every card below an insertion.
//
// Name: the bullet's lead clause — up to the first " (" or ": " or " — " —
// with markdown emphasis stripped, capped at 120 runes. The full text is the
// description. A checkbox on the bullet overrides the section status.
func bulletMilestone(phase *timeline.Phase, text, sectionStatus, rawLine string) *timeline.Milestone {
	text = strings.TrimSpace(text)
	sum := sha256.Sum256([]byte(text))
	id := fmt.Sprintf("%s-item-%x", phase.ID, sum[:4])

	status := sectionStatus
	if cb := checkboxRe.FindStringSubmatch(rawLine); cb != nil {
		if strings.EqualFold(cb[1], "x") {
			status = "completed"
		} else {
			status = "pending"
		}
	}
	if status == "" {
		status = phase.Status
	}
	// Milestone.Validate accepts pending/in_progress/completed/blocked; a
	// phase says "planned" for the same thing.
	if status == "planned" || status == "" {
		status = "pending"
	}

	name := text
	for _, sep := range []string{" (", ": ", " — ", " – "} {
		if i := strings.Index(name, sep); i > 0 {
			name = name[:i]
		}
	}
	name = strings.Trim(strings.ReplaceAll(name, "**", ""), " *_")
	if r := []rune(name); len(r) > 120 {
		name = string(r[:117]) + "..."
	}
	if name == "" {
		name = text
	}

	progress := 0
	if status == "completed" {
		progress = 100
	}
	return &timeline.Milestone{
		ID:          id,
		Name:        name,
		Status:      status,
		Progress:    progress,
		Description: text,
	}
}
