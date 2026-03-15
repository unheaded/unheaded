// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package eventbus

import (
	"strings"
	"time"
)

// Filter provides composable event filtering.
type Filter struct {
	fn FilterFunc
}

// NewFilter creates a new filter from a function.
func NewFilter(fn FilterFunc) *Filter {
	return &Filter{fn: fn}
}

// Apply applies the filter to an event.
func (f *Filter) Apply(e Event) bool {
	if f.fn == nil {
		return true
	}
	return f.fn(e)
}

// And combines two filters with logical AND.
func (f *Filter) And(other *Filter) *Filter {
	return &Filter{
		fn: func(e Event) bool {
			return f.Apply(e) && other.Apply(e)
		},
	}
}

// Or combines two filters with logical OR.
func (f *Filter) Or(other *Filter) *Filter {
	return &Filter{
		fn: func(e Event) bool {
			return f.Apply(e) || other.Apply(e)
		},
	}
}

// Not negates a filter.
func (f *Filter) Not() *Filter {
	return &Filter{
		fn: func(e Event) bool {
			return !f.Apply(e)
		},
	}
}

// FilterBuilder provides a fluent API for building filters.
type FilterBuilder struct {
	filters []*Filter
}

// NewFilterBuilder creates a new filter builder.
func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{}
}

// Topic adds a topic filter.
func (b *FilterBuilder) Topic(pattern string) *FilterBuilder {
	b.filters = append(b.filters, TopicFilter(pattern))
	return b
}

// Meta adds a metadata filter.
func (b *FilterBuilder) Meta(key, value string) *FilterBuilder {
	b.filters = append(b.filters, MetaFilter(key, value))
	return b
}

// HasMeta adds a filter for metadata key existence.
func (b *FilterBuilder) HasMeta(key string) *FilterBuilder {
	b.filters = append(b.filters, HasMetaFilter(key))
	return b
}

// After adds a time filter for events after a given time.
func (b *FilterBuilder) After(t time.Time) *FilterBuilder {
	b.filters = append(b.filters, AfterFilter(t))
	return b
}

// Before adds a time filter for events before a given time.
func (b *FilterBuilder) Before(t time.Time) *FilterBuilder {
	b.filters = append(b.filters, BeforeFilter(t))
	return b
}

// Custom adds a custom filter function.
func (b *FilterBuilder) Custom(fn FilterFunc) *FilterBuilder {
	b.filters = append(b.filters, NewFilter(fn))
	return b
}

// Build creates a combined filter from all added filters.
func (b *FilterBuilder) Build() *Filter {
	if len(b.filters) == 0 {
		return NewFilter(nil)
	}

	result := b.filters[0]
	for i := 1; i < len(b.filters); i++ {
		result = result.And(b.filters[i])
	}

	return result
}

// BuildFunc returns the filter as a FilterFunc.
func (b *FilterBuilder) BuildFunc() FilterFunc {
	f := b.Build()
	return f.fn
}

// Common filter constructors

// TopicFilter creates a filter that matches events by topic pattern.
func TopicFilter(pattern string) *Filter {
	return NewFilter(func(e Event) bool {
		return MatchPattern(pattern, e.Topic)
	})
}

// MetaFilter creates a filter that matches events with specific metadata.
func MetaFilter(key, value string) *Filter {
	return NewFilter(func(e Event) bool {
		if e.Meta == nil {
			return false
		}
		return e.Meta[key] == value
	})
}

// HasMetaFilter creates a filter that matches events with a metadata key.
func HasMetaFilter(key string) *Filter {
	return NewFilter(func(e Event) bool {
		if e.Meta == nil {
			return false
		}
		_, ok := e.Meta[key]
		return ok
	})
}

// MetaContainsFilter creates a filter that matches if metadata value contains substring.
func MetaContainsFilter(key, substring string) *Filter {
	return NewFilter(func(e Event) bool {
		if e.Meta == nil {
			return false
		}
		val, ok := e.Meta[key]
		if !ok {
			return false
		}
		return strings.Contains(val, substring)
	})
}

// AfterFilter creates a filter for events after a given time.
func AfterFilter(t time.Time) *Filter {
	return NewFilter(func(e Event) bool {
		return e.Timestamp.After(t)
	})
}

// BeforeFilter creates a filter for events before a given time.
func BeforeFilter(t time.Time) *Filter {
	return NewFilter(func(e Event) bool {
		return e.Timestamp.Before(t)
	})
}

// BetweenFilter creates a filter for events between two times.
func BetweenFilter(start, end time.Time) *Filter {
	return AfterFilter(start).And(BeforeFilter(end))
}

// DataTypeFilter creates a filter based on data type.
func DataTypeFilter[T any]() *Filter {
	return NewFilter(func(e Event) bool {
		_, ok := e.Data.(T)
		return ok
	})
}

// AnyFilter creates a filter that matches if any of the given filters match.
func AnyFilter(filters ...*Filter) *Filter {
	return NewFilter(func(e Event) bool {
		for _, f := range filters {
			if f.Apply(e) {
				return true
			}
		}
		return false
	})
}

// AllFilter creates a filter that matches only if all given filters match.
func AllFilter(filters ...*Filter) *Filter {
	return NewFilter(func(e Event) bool {
		for _, f := range filters {
			if !f.Apply(e) {
				return false
			}
		}
		return true
	})
}

// NoneFilter creates a filter that matches only if no given filters match.
func NoneFilter(filters ...*Filter) *Filter {
	return NewFilter(func(e Event) bool {
		for _, f := range filters {
			if f.Apply(e) {
				return false
			}
		}
		return true
	})
}

// PassAllFilter creates a filter that passes all events.
func PassAllFilter() *Filter {
	return NewFilter(func(e Event) bool {
		return true
	})
}

// BlockAllFilter creates a filter that blocks all events.
func BlockAllFilter() *Filter {
	return NewFilter(func(e Event) bool {
		return false
	})
}

// IDFilter creates a filter matching a specific event ID.
func IDFilter(id string) *Filter {
	return NewFilter(func(e Event) bool {
		return e.ID == id
	})
}

// IDPrefixFilter creates a filter matching events with ID prefix.
func IDPrefixFilter(prefix string) *Filter {
	return NewFilter(func(e Event) bool {
		return strings.HasPrefix(e.ID, prefix)
	})
}
