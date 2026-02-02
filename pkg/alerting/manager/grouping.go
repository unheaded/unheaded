// Package manager provides alert grouping functionality.
package manager

import (
	"sort"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/alerting"
)

// GroupConfig defines how alerts are grouped.
type GroupConfig struct {
	GroupBy       []string      `json:"group_by"`
	GroupWait     time.Duration `json:"group_wait"`
	GroupInterval time.Duration `json:"group_interval"`
}

// Grouper handles grouping of related alerts.
type Grouper struct {
	mu            sync.RWMutex
	groups        map[string]*alerting.AlertGroup
	groupWait     time.Duration
	groupInterval time.Duration
	groupBy       []string
}

// NewGrouper creates a new alert grouper.
func NewGrouper(groupWait, groupInterval time.Duration) *Grouper {
	return &Grouper{
		groups:        make(map[string]*alerting.AlertGroup),
		groupWait:     groupWait,
		groupInterval: groupInterval,
		groupBy:       []string{"alertname", "severity"},
	}
}

// SetGroupBy sets the labels to group by.
func (g *Grouper) SetGroupBy(labels []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.groupBy = labels
}

// Group groups alerts by their labels.
func (g *Grouper) Group(alerts []*alerting.Alert) []*alerting.AlertGroup {
	g.mu.Lock()
	defer g.mu.Unlock()

	groupMap := make(map[string]*alerting.AlertGroup)

	for _, alert := range alerts {
		key := g.generateGroupKey(alert)

		if group, exists := groupMap[key]; exists {
			group.Alerts = append(group.Alerts, alert)
		} else {
			groupMap[key] = &alerting.AlertGroup{
				Key:       key,
				Labels:    g.extractGroupLabels(alert),
				Alerts:    []*alerting.Alert{alert},
				CreatedAt: time.Now(),
			}
		}
	}

	// Convert map to slice
	groups := make([]*alerting.AlertGroup, 0, len(groupMap))
	for _, group := range groupMap {
		// Sort alerts by severity and time
		sortAlerts(group.Alerts)
		groups = append(groups, group)
	}

	return groups
}

// generateGroupKey generates a unique key for grouping.
func (g *Grouper) generateGroupKey(alert *alerting.Alert) string {
	var parts []string

	for _, label := range g.groupBy {
		var value string
		switch label {
		case "alertname":
			value = alert.Name
		case "severity":
			value = string(alert.Severity)
		default:
			value = alert.Labels[label]
		}
		parts = append(parts, label+"="+value)
	}

	return strings.Join(parts, ",")
}

// extractGroupLabels extracts the group labels from an alert.
func (g *Grouper) extractGroupLabels(alert *alerting.Alert) map[string]string {
	labels := make(map[string]string)

	for _, label := range g.groupBy {
		switch label {
		case "alertname":
			labels[label] = alert.Name
		case "severity":
			labels[label] = string(alert.Severity)
		default:
			if v, exists := alert.Labels[label]; exists {
				labels[label] = v
			}
		}
	}

	return labels
}

// AddToGroup adds an alert to an existing group or creates a new one.
func (g *Grouper) AddToGroup(alert *alerting.Alert) *alerting.AlertGroup {
	g.mu.Lock()
	defer g.mu.Unlock()

	key := g.generateGroupKey(alert)

	if group, exists := g.groups[key]; exists {
		// Check if alert already exists in group
		for i, existing := range group.Alerts {
			if existing.Fingerprint == alert.Fingerprint {
				group.Alerts[i] = alert
				return group
			}
		}
		group.Alerts = append(group.Alerts, alert)
		return group
	}

	group := &alerting.AlertGroup{
		Key:       key,
		Labels:    g.extractGroupLabels(alert),
		Alerts:    []*alerting.Alert{alert},
		CreatedAt: time.Now(),
	}
	g.groups[key] = group

	return group
}

// GetGroup retrieves a group by key.
func (g *Grouper) GetGroup(key string) (*alerting.AlertGroup, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	group, exists := g.groups[key]
	return group, exists
}

// RemoveFromGroup removes an alert from its group.
func (g *Grouper) RemoveFromGroup(alert *alerting.Alert) {
	g.mu.Lock()
	defer g.mu.Unlock()

	key := g.generateGroupKey(alert)
	group, exists := g.groups[key]
	if !exists {
		return
	}

	for i, a := range group.Alerts {
		if a.Fingerprint == alert.Fingerprint {
			group.Alerts = append(group.Alerts[:i], group.Alerts[i+1:]...)
			break
		}
	}

	// Remove empty groups
	if len(group.Alerts) == 0 {
		delete(g.groups, key)
	}
}

// GetAllGroups returns all current groups.
func (g *Grouper) GetAllGroups() []*alerting.AlertGroup {
	g.mu.RLock()
	defer g.mu.RUnlock()

	groups := make([]*alerting.AlertGroup, 0, len(g.groups))
	for _, group := range g.groups {
		groups = append(groups, group)
	}

	return groups
}

// CleanupResolvedGroups removes groups that only contain resolved alerts.
func (g *Grouper) CleanupResolvedGroups() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for key, group := range g.groups {
		allResolved := true
		for _, alert := range group.Alerts {
			if alert.State != alerting.AlertStateResolved {
				allResolved = false
				break
			}
		}
		if allResolved {
			delete(g.groups, key)
		}
	}
}

// Deduplicator handles alert deduplication.
type Deduplicator struct {
	mu     sync.RWMutex
	seen   map[string]*alerting.Alert
	ttl    time.Duration
}

// NewDeduplicator creates a new alert deduplicator.
func NewDeduplicator(ttl time.Duration) *Deduplicator {
	return &Deduplicator{
		seen: make(map[string]*alerting.Alert),
		ttl:  ttl,
	}
}

// IsDuplicate checks if an alert is a duplicate.
func (d *Deduplicator) IsDuplicate(alert *alerting.Alert) bool {
	d.mu.RLock()
	existing, exists := d.seen[alert.Fingerprint]
	d.mu.RUnlock()

	if !exists {
		return false
	}

	// Check if the existing alert is still active
	if existing.State == alerting.AlertStateFiring && alert.State == alerting.AlertStateFiring {
		return true
	}

	return false
}

// Track tracks an alert for deduplication.
func (d *Deduplicator) Track(alert *alerting.Alert) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen[alert.Fingerprint] = alert
}

// Remove removes an alert from tracking.
func (d *Deduplicator) Remove(fingerprint string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.seen, fingerprint)
}

// Cleanup removes old entries from the deduplicator.
func (d *Deduplicator) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-d.ttl)
	for fp, alert := range d.seen {
		if alert.State == alerting.AlertStateResolved && alert.ResolvedAt.Before(cutoff) {
			delete(d.seen, fp)
		}
	}
}

// sortAlerts sorts alerts by severity (critical first) then by start time.
func sortAlerts(alerts []*alerting.Alert) {
	sort.Slice(alerts, func(i, j int) bool {
		// Sort by severity
		severityOrder := map[alerting.AlertSeverity]int{
			alerting.SeverityCritical: 0,
			alerting.SeverityWarning:  1,
			alerting.SeverityInfo:     2,
		}

		si := severityOrder[alerts[i].Severity]
		sj := severityOrder[alerts[j].Severity]

		if si != sj {
			return si < sj
		}

		// Then by start time (newer first)
		return alerts[i].StartsAt.After(alerts[j].StartsAt)
	})
}
