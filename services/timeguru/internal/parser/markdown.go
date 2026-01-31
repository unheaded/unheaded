// Package parser provides markdown parsing for timeline files
// THE ORACLE'S ANTRE - Where timeline.md becomes prophecy
package parser

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/unheaded/unheaded/services/timeguru/internal/timeline"
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
	phaseHeaderRe = regexp.MustCompile(`^###\s+(Age|Phase|Epoch)\s+(\d+(?:\.\d+)?):?\s+(.+?)\s*(?:\(([A-Z\s]+)\))?$`)

	// Milestone headers: #### Epoch 1.1: The Whispering Void Awakens
	milestoneHeaderRe = regexp.MustCompile(`^####\s+(Epoch|Milestone)\s+(\d+(?:\.\d+)?):?\s+(.+)$`)

	// Checkbox items: - [x] Task description or - [ ] Task description
	checkboxRe = regexp.MustCompile(`^-\s+\[([ xX])\]\s+(.+)$`)

	// Status markers in text: (COMPLETE ✓), (IN PROGRESS 🚀), (PLANNED)
	statusCompleteRe   = regexp.MustCompile(`(?i)\(?COMPLETE[D]?\s*[✓✅]?\)?`)
	statusInProgressRe = regexp.MustCompile(`(?i)\(?IN[\s_-]?PROGRESS\s*[🚀]?\)?`)
	statusPlannedRe    = regexp.MustCompile(`(?i)\(?PLANNED\)?`)
	statusBlockedRe    = regexp.MustCompile(`(?i)\(?BLOCKED\)?`)

	// ETA patterns: ETA: Feb 3-4, 2026 or *ETA: Feb 8, 2026*
	etaRe = regexp.MustCompile(`(?i)ETA:?\s*([A-Za-z]+\s+\d+(?:-\d+)?,?\s*\d{4})`)

	// Risk patterns: Risk: Medium or **Risk:** High
	riskRe = regexp.MustCompile(`(?i)Risk:?\s*(low|medium|high)`)

	// Progress patterns: 25% FORGED or 60% complete
	progressRe = regexp.MustCompile(`(\d+)%\s*(?:FORGED|complete|done)?`)

	// Owner patterns: Owner: Agent 5 or **Owner:** The Architect
	ownerRe = regexp.MustCompile(`(?i)Owner:?\s*(.+?)(?:\s*$|,|\|)`)

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
	defer file.Close()

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
	var currentMilestone *timeline.Milestone
	var currentSection string
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
			if len(phaseMatch) > 4 && phaseMatch[4] != "" {
				currentPhase.Status = parseStatus(phaseMatch[4])
			}

			currentSection = "phase"
			currentMilestone = nil
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

			currentMilestone = &timeline.Milestone{
				ID:       milestoneID,
				Name:     milestoneName,
				Status:   "pending",
				Progress: 0,
				Tasks:    make([]string, 0),
			}

			currentSection = "milestone"
			continue
		}

		// Parse status from line content
		if currentMilestone != nil || currentPhase != nil {
			status := extractStatus(line)
			if status != "" {
				if currentMilestone != nil && currentMilestone.Status == "pending" {
					currentMilestone.Status = status
				} else if currentPhase != nil && currentPhase.Status == "planned" {
					currentPhase.Status = status
				}
			}
		}

		// Parse ETA
		if etaMatch := etaRe.FindStringSubmatch(line); etaMatch != nil {
			if currentMilestone != nil {
				currentMilestone.ETA = parseDate(etaMatch[1])
			}
		}

		// Parse Risk
		if riskMatch := riskRe.FindStringSubmatch(line); riskMatch != nil {
			if currentMilestone != nil {
				currentMilestone.Risk = strings.ToLower(riskMatch[1])
			}
		}

		// Parse Progress
		if progressMatch := progressRe.FindStringSubmatch(line); progressMatch != nil {
			progress := 0
			fmt.Sscanf(progressMatch[1], "%d", &progress)
			if currentMilestone != nil {
				currentMilestone.Progress = progress
			} else if currentPhase != nil {
				currentPhase.Progress = progress
			}
		}

		// Parse Owner
		if ownerMatch := ownerRe.FindStringSubmatch(line); ownerMatch != nil {
			if currentMilestone != nil {
				currentMilestone.Owner = strings.TrimSpace(ownerMatch[1])
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
	text = strings.ToUpper(strings.TrimSpace(text))

	switch {
	case strings.Contains(text, "COMPLETE"):
		return "completed"
	case strings.Contains(text, "PROGRESS"):
		return "in_progress"
	case strings.Contains(text, "BLOCK"):
		return "blocked"
	default:
		return "planned"
	}
}

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

	// Handle range dates by taking first date
	if strings.Contains(dateStr, "-") {
		parts := strings.Split(dateStr, "-")
		dateStr = strings.TrimSpace(parts[0])
		// Append year if missing
		if !strings.Contains(dateStr, "202") {
			// Find year in original
			yearRe := regexp.MustCompile(`\d{4}`)
			if yearMatch := yearRe.FindString(parts[len(parts)-1]); yearMatch != "" {
				dateStr += ", " + yearMatch
			}
		}
	}

	// Try various formats
	formats := []string{
		"Jan 2, 2006",
		"January 2, 2006",
		"Jan 2 2006",
		"January 2 2006",
		"2006-01-02",
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
