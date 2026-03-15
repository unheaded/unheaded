// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package controls

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"
)

// EvidenceType represents the type of evidence.
type EvidenceType string

const (
	EvidenceTypeDocument   EvidenceType = "document"
	EvidenceTypeScreenshot EvidenceType = "screenshot"
	EvidenceTypeLog        EvidenceType = "log"
	EvidenceTypeConfig     EvidenceType = "configuration"
	EvidenceTypeReport     EvidenceType = "report"
	EvidenceTypeArtifact   EvidenceType = "artifact"
	EvidenceTypeAttestation EvidenceType = "attestation"
)

// Evidence represents compliance evidence for a control.
type Evidence struct {
	ID           string            `json:"id"`
	ControlID    string            `json:"control_id"`
	Type         EvidenceType      `json:"type"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	Source       string            `json:"source"`
	Content      []byte            `json:"-"`
	ContentHash  string            `json:"content_hash"`
	ContentType  string            `json:"content_type"`
	Size         int64             `json:"size"`
	Metadata     map[string]string `json:"metadata"`
	CollectedAt  time.Time         `json:"collected_at"`
	CollectedBy  string            `json:"collected_by"`
	ValidFrom    time.Time         `json:"valid_from"`
	ValidUntil   *time.Time        `json:"valid_until,omitempty"`
	Tags         []string          `json:"tags"`
}

// EvidenceCollector defines the interface for evidence collection.
type EvidenceCollector interface {
	Collect(ctx context.Context, controlID string) ([]*Evidence, error)
	Name() string
	Supports(control *Control) bool
}

// EvidenceStore manages evidence storage and retrieval.
type EvidenceStore struct {
	mu       sync.RWMutex
	evidence map[string]*Evidence
	byControl map[string][]string
}

// NewEvidenceStore creates a new evidence store.
func NewEvidenceStore() *EvidenceStore {
	return &EvidenceStore{
		evidence:   make(map[string]*Evidence),
		byControl: make(map[string][]string),
	}
}

// Store stores evidence in the store.
func (s *EvidenceStore) Store(ctx context.Context, ev *Evidence) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ev.ID == "" {
		ev.ID = generateEvidenceID()
	}

	if ev.ContentHash == "" && len(ev.Content) > 0 {
		ev.ContentHash = computeHash(ev.Content)
	}

	s.evidence[ev.ID] = ev
	s.byControl[ev.ControlID] = append(s.byControl[ev.ControlID], ev.ID)

	return nil
}

// Get retrieves evidence by ID.
func (s *EvidenceStore) Get(ctx context.Context, id string) (*Evidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ev, ok := s.evidence[id]
	if !ok {
		return nil, fmt.Errorf("evidence not found: %s", id)
	}

	return ev, nil
}

// GetByControl retrieves all evidence for a control.
func (s *EvidenceStore) GetByControl(ctx context.Context, controlID string) ([]*Evidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := s.byControl[controlID]
	if !ok {
		return []*Evidence{}, nil
	}

	evidence := make([]*Evidence, 0, len(ids))
	for _, id := range ids {
		if ev, ok := s.evidence[id]; ok {
			evidence = append(evidence, ev)
		}
	}

	return evidence, nil
}

// Delete removes evidence from the store.
func (s *EvidenceStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ev, ok := s.evidence[id]
	if !ok {
		return fmt.Errorf("evidence not found: %s", id)
	}

	// Remove from control index
	controlIDs := s.byControl[ev.ControlID]
	for i, eid := range controlIDs {
		if eid == id {
			s.byControl[ev.ControlID] = append(controlIDs[:i], controlIDs[i+1:]...)
			break
		}
	}

	delete(s.evidence, id)
	return nil
}

// List lists all evidence with optional filters.
func (s *EvidenceStore) List(ctx context.Context, filters ...EvidenceFilter) ([]*Evidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Evidence, 0, len(s.evidence))
	for _, ev := range s.evidence {
		include := true
		for _, filter := range filters {
			if !filter(ev) {
				include = false
				break
			}
		}
		if include {
			result = append(result, ev)
		}
	}

	return result, nil
}

// EvidenceFilter is a function that filters evidence.
type EvidenceFilter func(*Evidence) bool

// FilterByType filters evidence by type.
func FilterByType(t EvidenceType) EvidenceFilter {
	return func(ev *Evidence) bool {
		return ev.Type == t
	}
}

// FilterByControl filters evidence by control ID.
func FilterByControl(controlID string) EvidenceFilter {
	return func(ev *Evidence) bool {
		return ev.ControlID == controlID
	}
}

// FilterByDateRange filters evidence by collection date.
func FilterByDateRange(start, end time.Time) EvidenceFilter {
	return func(ev *Evidence) bool {
		return ev.CollectedAt.After(start) && ev.CollectedAt.Before(end)
	}
}

// FilterValid filters for currently valid evidence.
func FilterValid() EvidenceFilter {
	return func(ev *Evidence) bool {
		now := time.Now()
		if now.Before(ev.ValidFrom) {
			return false
		}
		if ev.ValidUntil != nil && now.After(*ev.ValidUntil) {
			return false
		}
		return true
	}
}

// EvidenceCollectorRegistry manages evidence collectors.
type EvidenceCollectorRegistry struct {
	mu         sync.RWMutex
	collectors map[string]EvidenceCollector
}

// NewEvidenceCollectorRegistry creates a new collector registry.
func NewEvidenceCollectorRegistry() *EvidenceCollectorRegistry {
	return &EvidenceCollectorRegistry{
		collectors: make(map[string]EvidenceCollector),
	}
}

// Register registers an evidence collector.
func (r *EvidenceCollectorRegistry) Register(name string, collector EvidenceCollector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors[name] = collector
}

// GetCollectors returns collectors that support a control.
func (r *EvidenceCollectorRegistry) GetCollectors(control *Control) []EvidenceCollector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []EvidenceCollector
	for _, collector := range r.collectors {
		if collector.Supports(control) {
			result = append(result, collector)
		}
	}

	return result
}

// CollectAll collects evidence from all applicable collectors.
func (r *EvidenceCollectorRegistry) CollectAll(ctx context.Context, control *Control) ([]*Evidence, error) {
	collectors := r.GetCollectors(control)
	var allEvidence []*Evidence

	for _, collector := range collectors {
		evidence, err := collector.Collect(ctx, control.ID)
		if err != nil {
			continue // Log error but continue collecting
		}
		allEvidence = append(allEvidence, evidence...)
	}

	return allEvidence, nil
}

// ConfigurationCollector collects configuration evidence.
type ConfigurationCollector struct {
	configs map[string][]byte
}

// NewConfigurationCollector creates a new configuration collector.
func NewConfigurationCollector(configs map[string][]byte) *ConfigurationCollector {
	return &ConfigurationCollector{configs: configs}
}

func (c *ConfigurationCollector) Name() string {
	return "configuration_collector"
}

func (c *ConfigurationCollector) Supports(control *Control) bool {
	return true // Configurations can support any control
}

func (c *ConfigurationCollector) Collect(ctx context.Context, controlID string) ([]*Evidence, error) {
	var evidence []*Evidence

	for name, content := range c.configs {
		ev := &Evidence{
			ID:          generateEvidenceID(),
			ControlID:   controlID,
			Type:        EvidenceTypeConfig,
			Title:       fmt.Sprintf("Configuration: %s", name),
			Description: fmt.Sprintf("System configuration for %s", name),
			Source:      name,
			Content:     content,
			ContentHash: computeHash(content),
			ContentType: "application/json",
			Size:        int64(len(content)),
			Metadata:    map[string]string{"config_name": name},
			CollectedAt: time.Now(),
			CollectedBy: "automated",
			ValidFrom:   time.Now(),
		}
		evidence = append(evidence, ev)
	}

	return evidence, nil
}

// LogCollector collects log evidence.
type LogCollector struct {
	logReader io.Reader
	logSource string
}

// NewLogCollector creates a new log collector.
func NewLogCollector(reader io.Reader, source string) *LogCollector {
	return &LogCollector{
		logReader: reader,
		logSource: source,
	}
}

func (c *LogCollector) Name() string {
	return "log_collector"
}

func (c *LogCollector) Supports(control *Control) bool {
	return control.Category == CategoryAuditLogging
}

func (c *LogCollector) Collect(ctx context.Context, controlID string) ([]*Evidence, error) {
	content, err := io.ReadAll(c.logReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read logs: %w", err)
	}

	ev := &Evidence{
		ID:          generateEvidenceID(),
		ControlID:   controlID,
		Type:        EvidenceTypeLog,
		Title:       fmt.Sprintf("Audit Logs: %s", c.logSource),
		Description: "Collected audit log evidence",
		Source:      c.logSource,
		Content:     content,
		ContentHash: computeHash(content),
		ContentType: "text/plain",
		Size:        int64(len(content)),
		Metadata:    map[string]string{"log_source": c.logSource},
		CollectedAt: time.Now(),
		CollectedBy: "automated",
		ValidFrom:   time.Now(),
	}

	return []*Evidence{ev}, nil
}

// Helper functions

func generateEvidenceID() string {
	return fmt.Sprintf("EV-%d", time.Now().UnixNano())
}

func computeHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
