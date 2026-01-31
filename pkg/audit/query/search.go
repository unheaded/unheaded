package query

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/unheaded/unheaded/pkg/audit"
)

// SearchOptions configures search behavior.
type SearchOptions struct {
	// Query is the search query string.
	Query string `json:"query"`
	// Fields specifies which fields to search.
	Fields []string `json:"fields,omitempty"`
	// CaseSensitive enables case-sensitive search.
	CaseSensitive bool `json:"case_sensitive,omitempty"`
	// UseRegex enables regex matching.
	UseRegex bool `json:"use_regex,omitempty"`
	// Since limits search to events after this time.
	Since time.Time `json:"since,omitempty"`
	// Until limits search to events before this time.
	Until time.Time `json:"until,omitempty"`
	// Limit maximum results.
	Limit int `json:"limit,omitempty"`
	// Offset for pagination.
	Offset int `json:"offset,omitempty"`
}

// Searcher provides full-text search over audit events.
type Searcher struct {
	storage audit.Storage
}

// NewSearcher creates a new Searcher.
func NewSearcher(storage audit.Storage) *Searcher {
	return &Searcher{storage: storage}
}

// Search performs a text search over audit events.
func (s *Searcher) Search(ctx context.Context, opts *SearchOptions) ([]*audit.AuditEvent, error) {
	// Build base query
	query := &audit.AuditQuery{
		StartTime:  opts.Since,
		EndTime:    opts.Until,
		Limit:      0, // Get all for filtering
		Descending: true,
	}

	// Get events from storage
	events, err := s.storage.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	// Filter by search query
	var matcher func(string) bool
	if opts.UseRegex {
		var re *regexp.Regexp
		if opts.CaseSensitive {
			re, err = regexp.Compile(opts.Query)
		} else {
			re, err = regexp.Compile("(?i)" + opts.Query)
		}
		if err != nil {
			return nil, err
		}
		matcher = re.MatchString
	} else {
		searchQuery := opts.Query
		if !opts.CaseSensitive {
			searchQuery = strings.ToLower(opts.Query)
		}
		matcher = func(s string) bool {
			if !opts.CaseSensitive {
				s = strings.ToLower(s)
			}
			return strings.Contains(s, searchQuery)
		}
	}

	// Filter events
	var results []*audit.AuditEvent
	for _, event := range events {
		if s.eventMatches(event, matcher, opts.Fields) {
			results = append(results, event)
		}
	}

	// Apply pagination
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	}
	if opts.Limit > 0 && opts.Limit < len(results) {
		results = results[:opts.Limit]
	}

	return results, nil
}

// eventMatches checks if an event matches the search criteria.
func (s *Searcher) eventMatches(event *audit.AuditEvent, matcher func(string) bool, fields []string) bool {
	// If no fields specified, search all
	if len(fields) == 0 {
		fields = []string{"id", "type", "action", "actor", "resource", "source", "details"}
	}

	for _, field := range fields {
		switch field {
		case "id":
			if matcher(event.ID) {
				return true
			}
		case "type":
			if matcher(string(event.Type)) {
				return true
			}
		case "action":
			if matcher(event.Action) {
				return true
			}
		case "actor":
			if event.Actor != nil {
				if matcher(event.Actor.ID) || matcher(event.Actor.Name) || matcher(event.Actor.Type) {
					return true
				}
			}
		case "resource":
			if event.Resource != nil {
				if matcher(event.Resource.ID) || matcher(event.Resource.Name) || matcher(event.Resource.Type) || matcher(event.Resource.Path) {
					return true
				}
			}
		case "source":
			if event.Source != nil {
				if matcher(event.Source.IP) || matcher(event.Source.Service) || matcher(event.Source.RequestID) {
					return true
				}
			}
		case "details":
			if s.searchDetails(event.Details, matcher) {
				return true
			}
		}
	}

	return false
}

// searchDetails recursively searches through event details.
func (s *Searcher) searchDetails(details map[string]interface{}, matcher func(string) bool) bool {
	for key, value := range details {
		if matcher(key) {
			return true
		}
		if s.searchValue(value, matcher) {
			return true
		}
	}
	return false
}

// searchValue searches a single value.
func (s *Searcher) searchValue(value interface{}, matcher func(string) bool) bool {
	switch v := value.(type) {
	case string:
		return matcher(v)
	case map[string]interface{}:
		return s.searchDetails(v, matcher)
	case []interface{}:
		for _, item := range v {
			if s.searchValue(item, matcher) {
				return true
			}
		}
	}
	return false
}

// SearchResult holds search results with metadata.
type SearchResult struct {
	Events      []*audit.AuditEvent `json:"events"`
	Total       int                 `json:"total"`
	Query       string              `json:"query"`
	SearchTime  time.Duration       `json:"search_time"`
	HasMore     bool                `json:"has_more"`
	NextOffset  int                 `json:"next_offset,omitempty"`
}

// SearchWithMetadata performs a search and returns results with metadata.
func (s *Searcher) SearchWithMetadata(ctx context.Context, opts *SearchOptions) (*SearchResult, error) {
	start := time.Now()

	// Get all matching events first for total count
	allOpts := *opts
	allOpts.Limit = 0
	allOpts.Offset = 0

	allEvents, err := s.Search(ctx, &allOpts)
	if err != nil {
		return nil, err
	}

	// Apply pagination
	var results []*audit.AuditEvent
	if opts.Offset < len(allEvents) {
		end := opts.Offset + opts.Limit
		if opts.Limit == 0 || end > len(allEvents) {
			end = len(allEvents)
		}
		results = allEvents[opts.Offset:end]
	}

	result := &SearchResult{
		Events:     results,
		Total:      len(allEvents),
		Query:      opts.Query,
		SearchTime: time.Since(start),
	}

	if opts.Limit > 0 && opts.Offset+len(results) < len(allEvents) {
		result.HasMore = true
		result.NextOffset = opts.Offset + len(results)
	}

	return result, nil
}

// FilterBuilder provides advanced filtering capabilities.
type FilterBuilder struct {
	filters []func(*audit.AuditEvent) bool
}

// NewFilterBuilder creates a new FilterBuilder.
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{
		filters: make([]func(*audit.AuditEvent) bool, 0),
	}
}

// WithActorRole filters by actor role.
func (f *FilterBuilder) WithActorRole(role string) *FilterBuilder {
	f.filters = append(f.filters, func(e *audit.AuditEvent) bool {
		if e.Actor == nil {
			return false
		}
		for _, r := range e.Actor.Roles {
			if r == role {
				return true
			}
		}
		return false
	})
	return f
}

// WithSourceIP filters by source IP.
func (f *FilterBuilder) WithSourceIP(ip string) *FilterBuilder {
	f.filters = append(f.filters, func(e *audit.AuditEvent) bool {
		return e.Source != nil && e.Source.IP == ip
	})
	return f
}

// WithSourceService filters by source service.
func (f *FilterBuilder) WithSourceService(service string) *FilterBuilder {
	f.filters = append(f.filters, func(e *audit.AuditEvent) bool {
		return e.Source != nil && e.Source.Service == service
	})
	return f
}

// WithResourcePath filters by resource path prefix.
func (f *FilterBuilder) WithResourcePath(pathPrefix string) *FilterBuilder {
	f.filters = append(f.filters, func(e *audit.AuditEvent) bool {
		return e.Resource != nil && strings.HasPrefix(e.Resource.Path, pathPrefix)
	})
	return f
}

// WithDetailKey filters by presence of a detail key.
func (f *FilterBuilder) WithDetailKey(key string) *FilterBuilder {
	f.filters = append(f.filters, func(e *audit.AuditEvent) bool {
		_, ok := e.Details[key]
		return ok
	})
	return f
}

// WithDetailValue filters by detail key-value pair.
func (f *FilterBuilder) WithDetailValue(key string, value interface{}) *FilterBuilder {
	f.filters = append(f.filters, func(e *audit.AuditEvent) bool {
		v, ok := e.Details[key]
		if !ok {
			return false
		}
		return v == value
	})
	return f
}

// Custom adds a custom filter function.
func (f *FilterBuilder) Custom(fn func(*audit.AuditEvent) bool) *FilterBuilder {
	f.filters = append(f.filters, fn)
	return f
}

// Apply applies all filters to the given events.
func (f *FilterBuilder) Apply(events []*audit.AuditEvent) []*audit.AuditEvent {
	if len(f.filters) == 0 {
		return events
	}

	var results []*audit.AuditEvent
	for _, event := range events {
		if f.matches(event) {
			results = append(results, event)
		}
	}
	return results
}

// matches checks if an event passes all filters.
func (f *FilterBuilder) matches(event *audit.AuditEvent) bool {
	for _, filter := range f.filters {
		if !filter(event) {
			return false
		}
	}
	return true
}

// IndexedSearcher provides indexed search for faster lookups.
type IndexedSearcher struct {
	storage audit.Storage
	byActor map[string][]string // actorID -> eventIDs
	byIP    map[string][]string // IP -> eventIDs
	byType  map[audit.EventType][]string
}

// NewIndexedSearcher creates a new indexed searcher.
func NewIndexedSearcher(storage audit.Storage) *IndexedSearcher {
	return &IndexedSearcher{
		storage: storage,
		byActor: make(map[string][]string),
		byIP:    make(map[string][]string),
		byType:  make(map[audit.EventType][]string),
	}
}

// Index builds the search index from storage.
func (s *IndexedSearcher) Index(ctx context.Context) error {
	events, err := s.storage.Query(ctx, &audit.AuditQuery{})
	if err != nil {
		return err
	}

	for _, event := range events {
		if event.Actor != nil {
			s.byActor[event.Actor.ID] = append(s.byActor[event.Actor.ID], event.ID)
		}
		if event.Source != nil && event.Source.IP != "" {
			s.byIP[event.Source.IP] = append(s.byIP[event.Source.IP], event.ID)
		}
		s.byType[event.Type] = append(s.byType[event.Type], event.ID)
	}

	return nil
}

// FindByActor quickly finds events by actor ID using the index.
func (s *IndexedSearcher) FindByActor(ctx context.Context, actorID string) ([]*audit.AuditEvent, error) {
	eventIDs, ok := s.byActor[actorID]
	if !ok {
		return nil, nil
	}

	var events []*audit.AuditEvent
	for _, id := range eventIDs {
		event, err := s.storage.Get(ctx, id)
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// FindByIP quickly finds events by source IP using the index.
func (s *IndexedSearcher) FindByIP(ctx context.Context, ip string) ([]*audit.AuditEvent, error) {
	eventIDs, ok := s.byIP[ip]
	if !ok {
		return nil, nil
	}

	var events []*audit.AuditEvent
	for _, id := range eventIDs {
		event, err := s.storage.Get(ctx, id)
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// FindByType quickly finds events by type using the index.
func (s *IndexedSearcher) FindByType(ctx context.Context, eventType audit.EventType) ([]*audit.AuditEvent, error) {
	eventIDs, ok := s.byType[eventType]
	if !ok {
		return nil, nil
	}

	var events []*audit.AuditEvent
	for _, id := range eventIDs {
		event, err := s.storage.Get(ctx, id)
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}
