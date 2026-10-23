// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"sort"
	"sync"
)

// memStore is a fake Well with 033's semantics that the tests rely on:
// captures dedupe by id, labels by (capture, kind, source_ref), a label
// needs its capture.
type memStore struct {
	mu          sync.Mutex
	captures    map[string]Capture
	labels      []Label
	checks      []CheckAction
	escalations []Escalation
	runs        []AgentRun
	noOps       bool // Escalations -> ErrUnavailable
	noRuns      bool // AgentRuns -> ErrUnavailable
}

func newMemStore() *memStore { return &memStore{captures: map[string]Capture{}} }

func (m *memStore) InsertCapture(_ context.Context, c *Capture) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.captures[c.ID]; ok {
		return false, nil
	}
	m.captures[c.ID] = *c
	return true, nil
}

func (m *memStore) InsertLabel(_ context.Context, l Label) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.captures[l.CaptureID]; !ok {
		return false, errMissingCapture
	}
	for _, x := range m.labels {
		if x.CaptureID == l.CaptureID && x.Kind == l.Kind && x.SourceRef == l.SourceRef {
			return false, nil
		}
	}
	m.labels = append(m.labels, l)
	return true, nil
}

func (m *memStore) Captures(context.Context) ([]Capture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Capture, 0, len(m.captures))
	for _, c := range m.captures {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (m *memStore) Labels(context.Context) ([]Label, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Label(nil), m.labels...), nil
}

func (m *memStore) CheckActions(context.Context) ([]CheckAction, error) { return m.checks, nil }

func (m *memStore) Escalations(context.Context) ([]Escalation, error) {
	if m.noOps {
		return nil, ErrUnavailable
	}
	return m.escalations, nil
}

func (m *memStore) AgentRuns(context.Context) ([]AgentRun, error) {
	if m.noRuns {
		return nil, ErrUnavailable
	}
	return m.runs, nil
}

func (m *memStore) Quarantine(_ context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.captures[id]
	if !ok {
		return false, nil
	}
	c.Quarantined = true
	m.captures[id] = c
	return true, nil
}

func (m *memStore) Promote(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if c, ok := m.captures[id]; ok {
			c.Promoted = true
			m.captures[id] = c
		}
	}
	return nil
}

func (m *memStore) labelsOf(id string) map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]int{}
	for _, l := range m.labels {
		if l.CaptureID == id {
			out[l.Kind]++
		}
	}
	return out
}
