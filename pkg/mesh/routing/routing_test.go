// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package routing

import (
	"context"
	"sync"
	"testing"
	"time"
)

// ==================== StringMatch Tests ====================

func TestNewExactMatch(t *testing.T) {
	sm := NewExactMatch("/api/v1/users")
	if sm.Type != MatchExact {
		t.Errorf("expected MatchExact, got %d", sm.Type)
	}
	if sm.Value != "/api/v1/users" {
		t.Errorf("expected /api/v1/users, got %s", sm.Value)
	}
}

func TestNewPrefixMatch(t *testing.T) {
	sm := NewPrefixMatch("/api/")
	if sm.Type != MatchPrefix {
		t.Errorf("expected MatchPrefix, got %d", sm.Type)
	}
	if sm.Value != "/api/" {
		t.Errorf("expected /api/, got %s", sm.Value)
	}
}

func TestNewRegexMatch(t *testing.T) {
	t.Run("ValidPattern", func(t *testing.T) {
		sm, err := NewRegexMatch(`^/api/v\d+/.*`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sm.Type != MatchRegex {
			t.Errorf("expected MatchRegex, got %d", sm.Type)
		}
		if sm.Value != `^/api/v\d+/.*` {
			t.Errorf("unexpected value: %s", sm.Value)
		}
	})

	t.Run("InvalidPattern", func(t *testing.T) {
		_, err := NewRegexMatch(`[invalid`)
		if err == nil {
			t.Error("expected error for invalid regex pattern")
		}
	})
}

func TestNewPresentMatch(t *testing.T) {
	sm := NewPresentMatch()
	if sm.Type != MatchPresent {
		t.Errorf("expected MatchPresent, got %d", sm.Type)
	}
}

func TestStringMatch_Matches(t *testing.T) {
	t.Run("Exact_Match", func(t *testing.T) {
		sm := NewExactMatch("GET")
		if !sm.Matches("GET") {
			t.Error("expected exact match to succeed")
		}
	})

	t.Run("Exact_NoMatch", func(t *testing.T) {
		sm := NewExactMatch("GET")
		if sm.Matches("POST") {
			t.Error("expected exact match to fail")
		}
	})

	t.Run("Prefix_Match", func(t *testing.T) {
		sm := NewPrefixMatch("/api/")
		if !sm.Matches("/api/v1/users") {
			t.Error("expected prefix match to succeed")
		}
	})

	t.Run("Prefix_NoMatch", func(t *testing.T) {
		sm := NewPrefixMatch("/api/")
		if sm.Matches("/health") {
			t.Error("expected prefix match to fail")
		}
	})

	t.Run("Regex_Match", func(t *testing.T) {
		sm, err := NewRegexMatch(`^/api/v\d+/`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !sm.Matches("/api/v1/users") {
			t.Error("expected regex match to succeed")
		}
	})

	t.Run("Regex_NoMatch", func(t *testing.T) {
		sm, err := NewRegexMatch(`^/api/v\d+/`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sm.Matches("/health") {
			t.Error("expected regex match to fail")
		}
	})

	t.Run("Regex_NilRegex_ValidValue", func(t *testing.T) {
		// Test the lazy compilation path when regex is nil
		sm := &StringMatch{
			Type:  MatchRegex,
			Value: `^/api/`,
		}
		if !sm.Matches("/api/v1") {
			t.Error("expected regex lazy compile to succeed")
		}
	})

	t.Run("Regex_NilRegex_InvalidValue", func(t *testing.T) {
		sm := &StringMatch{
			Type:  MatchRegex,
			Value: `[invalid`,
		}
		if sm.Matches("/api/v1") {
			t.Error("expected match to fail with invalid regex")
		}
	})

	t.Run("Present_NonEmpty", func(t *testing.T) {
		sm := NewPresentMatch()
		if !sm.Matches("some-value") {
			t.Error("expected present match to succeed for non-empty string")
		}
	})

	t.Run("Present_Empty", func(t *testing.T) {
		sm := NewPresentMatch()
		if sm.Matches("") {
			t.Error("expected present match to fail for empty string")
		}
	})

	t.Run("UnknownType", func(t *testing.T) {
		sm := &StringMatch{Type: MatchType(99)}
		if sm.Matches("anything") {
			t.Error("expected unknown match type to return false")
		}
	})
}

// ==================== Match Tests ====================

func TestMatch_Matches(t *testing.T) {
	t.Run("URI_Exact", func(t *testing.T) {
		m := &Match{URI: NewExactMatch("/api/v1/users")}
		req := &Request{Path: "/api/v1/users"}
		if !m.Matches(req) {
			t.Error("expected URI exact match to succeed")
		}
	})

	t.Run("URI_NoMatch", func(t *testing.T) {
		m := &Match{URI: NewExactMatch("/api/v1/users")}
		req := &Request{Path: "/api/v1/posts"}
		if m.Matches(req) {
			t.Error("expected URI match to fail")
		}
	})

	t.Run("Method_Match", func(t *testing.T) {
		m := &Match{Method: NewExactMatch("GET")}
		req := &Request{Method: "GET"}
		if !m.Matches(req) {
			t.Error("expected method match to succeed")
		}
	})

	t.Run("Method_NoMatch", func(t *testing.T) {
		m := &Match{Method: NewExactMatch("GET")}
		req := &Request{Method: "POST"}
		if m.Matches(req) {
			t.Error("expected method match to fail")
		}
	})

	t.Run("Header_Exact", func(t *testing.T) {
		m := &Match{
			Headers: map[string]*StringMatch{
				"Content-Type": NewExactMatch("application/json"),
			},
		}
		req := &Request{
			Headers: map[string]string{"Content-Type": "application/json"},
		}
		if !m.Matches(req) {
			t.Error("expected header match to succeed")
		}
	})

	t.Run("Header_NoMatch", func(t *testing.T) {
		m := &Match{
			Headers: map[string]*StringMatch{
				"Content-Type": NewExactMatch("application/json"),
			},
		}
		req := &Request{
			Headers: map[string]string{"Content-Type": "text/html"},
		}
		if m.Matches(req) {
			t.Error("expected header match to fail")
		}
	})

	t.Run("Header_Present_Exists", func(t *testing.T) {
		m := &Match{
			Headers: map[string]*StringMatch{
				"Authorization": NewPresentMatch(),
			},
		}
		req := &Request{
			Headers: map[string]string{"Authorization": "Bearer token"},
		}
		if !m.Matches(req) {
			t.Error("expected header present match to succeed")
		}
	})

	t.Run("Header_Present_Missing", func(t *testing.T) {
		m := &Match{
			Headers: map[string]*StringMatch{
				"Authorization": NewPresentMatch(),
			},
		}
		req := &Request{
			Headers: map[string]string{},
		}
		if m.Matches(req) {
			t.Error("expected header present match to fail when header missing")
		}
	})

	t.Run("SourceNamespace_Match", func(t *testing.T) {
		m := &Match{SourceNamespace: NewExactMatch("production")}
		req := &Request{SourceNamespace: "production"}
		if !m.Matches(req) {
			t.Error("expected source namespace match to succeed")
		}
	})

	t.Run("SourceNamespace_NoMatch", func(t *testing.T) {
		m := &Match{SourceNamespace: NewExactMatch("production")}
		req := &Request{SourceNamespace: "staging"}
		if m.Matches(req) {
			t.Error("expected source namespace match to fail")
		}
	})

	t.Run("AllConditions", func(t *testing.T) {
		m := &Match{
			URI:    NewPrefixMatch("/api/"),
			Method: NewExactMatch("POST"),
			Headers: map[string]*StringMatch{
				"Content-Type": NewExactMatch("application/json"),
			},
			SourceNamespace: NewExactMatch("default"),
		}
		req := &Request{
			Path:            "/api/v1/users",
			Method:          "POST",
			Headers:         map[string]string{"Content-Type": "application/json"},
			SourceNamespace: "default",
		}
		if !m.Matches(req) {
			t.Error("expected all conditions match to succeed")
		}
	})

	t.Run("NilConditions_AlwaysMatch", func(t *testing.T) {
		m := &Match{}
		req := &Request{Path: "/anything", Method: "GET"}
		if !m.Matches(req) {
			t.Error("expected empty match to succeed (no conditions)")
		}
	})
}

// ==================== MatchBuilder Tests ====================

func TestMatchBuilder(t *testing.T) {
	t.Run("URIExact", func(t *testing.T) {
		m := NewMatchBuilder().URIExact("/api/v1/users").Build()
		if m.URI == nil || m.URI.Type != MatchExact || m.URI.Value != "/api/v1/users" {
			t.Error("URIExact did not set correctly")
		}
	})

	t.Run("URIPrefix", func(t *testing.T) {
		m := NewMatchBuilder().URIPrefix("/api/").Build()
		if m.URI == nil || m.URI.Type != MatchPrefix || m.URI.Value != "/api/" {
			t.Error("URIPrefix did not set correctly")
		}
	})

	t.Run("URI_Custom", func(t *testing.T) {
		sm, _ := NewRegexMatch(`^/api/`)
		m := NewMatchBuilder().URI(sm).Build()
		if m.URI == nil || m.URI.Type != MatchRegex {
			t.Error("URI did not set correctly")
		}
	})

	t.Run("Method", func(t *testing.T) {
		m := NewMatchBuilder().Method("POST").Build()
		if m.Method == nil || m.Method.Value != "POST" {
			t.Error("Method did not set correctly")
		}
	})

	t.Run("Header", func(t *testing.T) {
		sm := NewExactMatch("application/json")
		m := NewMatchBuilder().Header("Content-Type", sm).Build()
		if m.Headers["Content-Type"] == nil {
			t.Error("Header did not set correctly")
		}
	})

	t.Run("HeaderExact", func(t *testing.T) {
		m := NewMatchBuilder().HeaderExact("X-Version", "v2").Build()
		if m.Headers["X-Version"] == nil || m.Headers["X-Version"].Value != "v2" {
			t.Error("HeaderExact did not set correctly")
		}
	})

	t.Run("HeaderPresent", func(t *testing.T) {
		m := NewMatchBuilder().HeaderPresent("Authorization").Build()
		if m.Headers["Authorization"] == nil || m.Headers["Authorization"].Type != MatchPresent {
			t.Error("HeaderPresent did not set correctly")
		}
	})

	t.Run("SourceNamespace", func(t *testing.T) {
		m := NewMatchBuilder().SourceNamespace("prod").Build()
		if m.SourceNamespace == nil || m.SourceNamespace.Value != "prod" {
			t.Error("SourceNamespace did not set correctly")
		}
	})

	t.Run("Chaining", func(t *testing.T) {
		m := NewMatchBuilder().
			URIPrefix("/api/").
			Method("GET").
			HeaderExact("Accept", "application/json").
			HeaderPresent("Authorization").
			SourceNamespace("production").
			Build()

		if m.URI == nil {
			t.Error("expected URI to be set")
		}
		if m.Method == nil {
			t.Error("expected Method to be set")
		}
		if len(m.Headers) != 2 {
			t.Errorf("expected 2 headers, got %d", len(m.Headers))
		}
		if m.SourceNamespace == nil {
			t.Error("expected SourceNamespace to be set")
		}
	})
}

// ==================== PathMatcher Tests ====================

func TestPathMatcher(t *testing.T) {
	t.Run("ExactPath", func(t *testing.T) {
		pm := NewPathMatcher("/users/list")
		matched, params := pm.Match("/users/list")
		if !matched {
			t.Error("expected exact path to match")
		}
		if len(params) != 0 {
			t.Errorf("expected 0 params, got %d", len(params))
		}
	})

	t.Run("ExactPath_NoMatch", func(t *testing.T) {
		pm := NewPathMatcher("/users/list")
		matched, _ := pm.Match("/users/create")
		if matched {
			t.Error("expected exact path to not match")
		}
	})

	t.Run("ParamExtraction", func(t *testing.T) {
		pm := NewPathMatcher("/users/:id")
		matched, params := pm.Match("/users/42")
		if !matched {
			t.Error("expected param path to match")
		}
		if params["id"] != "42" {
			t.Errorf("expected param id=42, got %s", params["id"])
		}
	})

	t.Run("MultipleParams", func(t *testing.T) {
		pm := NewPathMatcher("/users/:userId/posts/:postId")
		matched, params := pm.Match("/users/10/posts/99")
		if !matched {
			t.Error("expected multi-param path to match")
		}
		if params["userId"] != "10" {
			t.Errorf("expected userId=10, got %s", params["userId"])
		}
		if params["postId"] != "99" {
			t.Errorf("expected postId=99, got %s", params["postId"])
		}
	})

	t.Run("Wildcard", func(t *testing.T) {
		pm := NewPathMatcher("/static/*")
		matched, params := pm.Match("/static/css/style.css")
		if !matched {
			t.Error("expected wildcard path to match")
		}
		if params["*"] != "css/style.css" {
			t.Errorf("expected wildcard=css/style.css, got %s", params["*"])
		}
	})

	t.Run("Wildcard_SingleSegment", func(t *testing.T) {
		pm := NewPathMatcher("/files/*")
		matched, params := pm.Match("/files/readme.txt")
		if !matched {
			t.Error("expected wildcard to match single segment")
		}
		if params["*"] != "readme.txt" {
			t.Errorf("expected wildcard=readme.txt, got %s", params["*"])
		}
	})

	t.Run("WrongSegmentCount", func(t *testing.T) {
		pm := NewPathMatcher("/users/:id")
		matched, _ := pm.Match("/users/42/extra")
		if matched {
			t.Error("expected mismatch for extra segments")
		}
	})

	t.Run("TooFewSegments", func(t *testing.T) {
		pm := NewPathMatcher("/users/:id/posts")
		matched, _ := pm.Match("/users/42")
		if matched {
			t.Error("expected mismatch for too few segments")
		}
	})

	t.Run("ParamAndWildcard", func(t *testing.T) {
		pm := NewPathMatcher("/api/:version/*")
		matched, params := pm.Match("/api/v2/users/list")
		if !matched {
			t.Error("expected param+wildcard path to match")
		}
		if params["version"] != "v2" {
			t.Errorf("expected version=v2, got %s", params["version"])
		}
		if params["*"] != "users/list" {
			t.Errorf("expected wildcard=users/list, got %s", params["*"])
		}
	})

	t.Run("Wildcard_TooFewSegments", func(t *testing.T) {
		pm := NewPathMatcher("/api/:version/*")
		matched, _ := pm.Match("/api")
		if matched {
			t.Error("expected mismatch when too few segments for wildcard pattern")
		}
	})
}

// ==================== HeaderOperation Tests ====================

func TestHeaderOperation_Apply(t *testing.T) {
	t.Run("Set", func(t *testing.T) {
		op := &HeaderOperation{
			Set: map[string]string{"X-Version": "v2"},
		}
		result := op.Apply(map[string]string{"X-Version": "v1"})
		if result["X-Version"] != "v2" {
			t.Errorf("expected X-Version=v2, got %s", result["X-Version"])
		}
	})

	t.Run("Add_NewHeader", func(t *testing.T) {
		op := &HeaderOperation{
			Add: map[string]string{"X-New": "value"},
		}
		result := op.Apply(map[string]string{})
		if result["X-New"] != "value" {
			t.Errorf("expected X-New=value, got %s", result["X-New"])
		}
	})

	t.Run("Add_ExistingHeader_NoOverwrite", func(t *testing.T) {
		op := &HeaderOperation{
			Add: map[string]string{"X-Existing": "new"},
		}
		result := op.Apply(map[string]string{"X-Existing": "old"})
		if result["X-Existing"] != "old" {
			t.Errorf("expected X-Existing=old (no overwrite), got %s", result["X-Existing"])
		}
	})

	t.Run("Remove", func(t *testing.T) {
		op := &HeaderOperation{
			Remove: []string{"X-Secret"},
		}
		result := op.Apply(map[string]string{"X-Secret": "hidden", "X-Keep": "visible"})
		if _, exists := result["X-Secret"]; exists {
			t.Error("expected X-Secret to be removed")
		}
		if result["X-Keep"] != "visible" {
			t.Error("expected X-Keep to be preserved")
		}
	})

	t.Run("AllOperations", func(t *testing.T) {
		op := &HeaderOperation{
			Set:    map[string]string{"X-Override": "new-val"},
			Add:    map[string]string{"X-Added": "added-val"},
			Remove: []string{"X-Remove"},
		}
		headers := map[string]string{
			"X-Override": "old-val",
			"X-Remove":   "gone",
			"X-Keep":     "stays",
		}
		result := op.Apply(headers)
		if result["X-Override"] != "new-val" {
			t.Errorf("expected X-Override=new-val, got %s", result["X-Override"])
		}
		if result["X-Added"] != "added-val" {
			t.Errorf("expected X-Added=added-val, got %s", result["X-Added"])
		}
		if _, exists := result["X-Remove"]; exists {
			t.Error("expected X-Remove to be removed")
		}
		if result["X-Keep"] != "stays" {
			t.Errorf("expected X-Keep=stays, got %s", result["X-Keep"])
		}
	})

	t.Run("NilInputHeaders", func(t *testing.T) {
		op := &HeaderOperation{
			Set: map[string]string{"X-New": "value"},
		}
		result := op.Apply(nil)
		if result["X-New"] != "value" {
			t.Errorf("expected X-New=value, got %s", result["X-New"])
		}
	})
}

// ==================== Rule Tests ====================

func TestRule_Match(t *testing.T) {
	t.Run("NoMatches_AlwaysMatches", func(t *testing.T) {
		rule := &Rule{
			Destinations: []*Destination{{Service: "svc-a", Weight: 1}},
		}
		req := &Request{Path: "/anything", Method: "GET"}
		match := rule.Match(req)
		if match == nil {
			t.Error("expected rule with no matches to match any request")
		}
	})

	t.Run("MatchingCondition", func(t *testing.T) {
		rule := &Rule{
			Matches: []*Match{
				{URI: NewPrefixMatch("/api/")},
			},
			Destinations: []*Destination{{Service: "api-svc", Weight: 1}},
		}
		req := &Request{Path: "/api/v1/users"}
		match := rule.Match(req)
		if match == nil {
			t.Fatal("expected rule to match") // Fatal — next assertion derefs match
		}
		if match.Destination.Service != "api-svc" {
			t.Errorf("expected destination api-svc, got %s", match.Destination.Service)
		}
	})

	t.Run("NoMatchingCondition", func(t *testing.T) {
		rule := &Rule{
			Matches: []*Match{
				{URI: NewExactMatch("/specific-path")},
			},
			Destinations: []*Destination{{Service: "svc", Weight: 1}},
		}
		req := &Request{Path: "/other-path"}
		match := rule.Match(req)
		if match != nil {
			t.Error("expected rule not to match")
		}
	})

	t.Run("AnyMatch_FirstMatches", func(t *testing.T) {
		rule := &Rule{
			Matches: []*Match{
				{URI: NewExactMatch("/health")},
				{URI: NewPrefixMatch("/api/")},
			},
			Destinations: []*Destination{{Service: "svc", Weight: 1}},
		}
		req := &Request{Path: "/health"}
		match := rule.Match(req)
		if match == nil {
			t.Error("expected first match condition to succeed")
		}
	})

	t.Run("AnyMatch_SecondMatches", func(t *testing.T) {
		rule := &Rule{
			Matches: []*Match{
				{URI: NewExactMatch("/health")},
				{URI: NewPrefixMatch("/api/")},
			},
			Destinations: []*Destination{{Service: "svc", Weight: 1}},
		}
		req := &Request{Path: "/api/v1/data"}
		match := rule.Match(req)
		if match == nil {
			t.Error("expected second match condition to succeed")
		}
	})

	t.Run("NoDestinations_ReturnsNil", func(t *testing.T) {
		rule := &Rule{
			Matches:      []*Match{},
			Destinations: []*Destination{},
		}
		req := &Request{Path: "/anything"}
		match := rule.Match(req)
		if match != nil {
			t.Error("expected nil when no destinations configured")
		}
	})

	t.Run("HeaderOperations_Applied", func(t *testing.T) {
		rule := &Rule{
			Destinations: []*Destination{{Service: "svc", Weight: 1}},
			Headers: &HeaderOperation{
				Set: map[string]string{"X-Routed": "true"},
			},
		}
		req := &Request{
			Path:    "/test",
			Headers: map[string]string{"X-Original": "yes"},
		}
		match := rule.Match(req)
		if match == nil {
			t.Fatal("expected match")
		}
		if match.Headers["X-Routed"] != "true" {
			t.Error("expected X-Routed header to be set")
		}
		if match.Headers["X-Original"] != "yes" {
			t.Error("expected X-Original header to be preserved")
		}
	})
}

func TestRule_WeightedDestination(t *testing.T) {
	// With a single destination, it should always be selected.
	t.Run("SingleDestination", func(t *testing.T) {
		rule := &Rule{
			Destinations: []*Destination{
				{Service: "only-svc", Weight: 100},
			},
		}
		req := &Request{Path: "/test"}
		for i := 0; i < 20; i++ {
			match := rule.Match(req)
			if match == nil {
				t.Fatal("expected match")
			}
			if match.Destination.Service != "only-svc" {
				t.Errorf("expected only-svc, got %s", match.Destination.Service)
			}
		}
	})

	// Multiple destinations: verify both are reachable over many calls.
	t.Run("MultipleDestinations", func(t *testing.T) {
		rule := &Rule{
			Destinations: []*Destination{
				{Service: "svc-a", Weight: 50},
				{Service: "svc-b", Weight: 50},
			},
		}
		req := &Request{Path: "/test"}
		hits := map[string]int{}
		for i := 0; i < 200; i++ {
			match := rule.Match(req)
			if match == nil {
				t.Fatal("expected match")
			}
			hits[match.Destination.Service]++
		}
		if hits["svc-a"] == 0 {
			t.Error("expected svc-a to be selected at least once")
		}
		if hits["svc-b"] == 0 {
			t.Error("expected svc-b to be selected at least once")
		}
	})

	// Destinations with zero/negative weight default to 1.
	t.Run("ZeroWeight_DefaultsToOne", func(t *testing.T) {
		rule := &Rule{
			Destinations: []*Destination{
				{Service: "svc-zero", Weight: 0},
			},
		}
		req := &Request{Path: "/test"}
		match := rule.Match(req)
		if match == nil {
			t.Fatal("expected match even with zero weight")
		}
		if match.Destination.Service != "svc-zero" {
			t.Errorf("expected svc-zero, got %s", match.Destination.Service)
		}
	})
}

// ==================== RuleBuilder Tests ====================

func TestRuleBuilder(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		rule := NewRuleBuilder("test-rule").
			Priority(10).
			AddDestination("svc-a", 80).
			AddDestination("svc-b", 20).
			Timeout(5 * time.Second).
			Build()

		if rule.Name != "test-rule" {
			t.Errorf("expected name test-rule, got %s", rule.Name)
		}
		if rule.Priority != 10 {
			t.Errorf("expected priority 10, got %d", rule.Priority)
		}
		if len(rule.Destinations) != 2 {
			t.Errorf("expected 2 destinations, got %d", len(rule.Destinations))
		}
		if rule.Timeout != 5*time.Second {
			t.Errorf("expected timeout 5s, got %v", rule.Timeout)
		}
	})

	t.Run("WithMatch", func(t *testing.T) {
		m := NewMatchBuilder().URIPrefix("/api/").Method("GET").Build()
		rule := NewRuleBuilder("api-rule").
			AddMatch(m).
			AddDestination("api-svc", 100).
			Build()

		if len(rule.Matches) != 1 {
			t.Errorf("expected 1 match, got %d", len(rule.Matches))
		}
	})

	t.Run("WithRetry", func(t *testing.T) {
		rule := NewRuleBuilder("retry-rule").
			Retry(3, 2*time.Second).
			AddDestination("svc", 1).
			Build()

		if rule.Retry == nil {
			t.Fatal("expected retry config")
		}
		if rule.Retry.Attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", rule.Retry.Attempts)
		}
		if rule.Retry.PerTryTimeout != 2*time.Second {
			t.Errorf("expected 2s per try timeout, got %v", rule.Retry.PerTryTimeout)
		}
	})
}

// ==================== Router Tests ====================

func TestNewRouter(t *testing.T) {
	r := NewRouter()
	if r == nil {
		t.Fatal("expected non-nil router")
	}
	rules := r.ListRules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestRouter_AddRule(t *testing.T) {
	t.Run("SingleRule", func(t *testing.T) {
		r := NewRouter()
		rule := &Rule{
			Name:         "test",
			Priority:     10,
			Destinations: []*Destination{{Service: "svc", Weight: 1}},
		}
		if err := r.AddRule(rule); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rules := r.ListRules()
		if len(rules) != 1 {
			t.Errorf("expected 1 rule, got %d", len(rules))
		}
	})

	t.Run("PriorityOrdering", func(t *testing.T) {
		r := NewRouter()
		r.AddRule(&Rule{Name: "low", Priority: 1, Destinations: []*Destination{{Service: "low-svc", Weight: 1}}})
		r.AddRule(&Rule{Name: "high", Priority: 100, Destinations: []*Destination{{Service: "high-svc", Weight: 1}}})
		r.AddRule(&Rule{Name: "mid", Priority: 50, Destinations: []*Destination{{Service: "mid-svc", Weight: 1}}})

		rules := r.ListRules()
		if len(rules) != 3 {
			t.Fatalf("expected 3 rules, got %d", len(rules))
		}
		if rules[0].Name != "high" {
			t.Errorf("expected first rule to be 'high', got %s", rules[0].Name)
		}
		if rules[1].Name != "mid" {
			t.Errorf("expected second rule to be 'mid', got %s", rules[1].Name)
		}
		if rules[2].Name != "low" {
			t.Errorf("expected third rule to be 'low', got %s", rules[2].Name)
		}
	})

	t.Run("UpdateExisting", func(t *testing.T) {
		r := NewRouter()
		r.AddRule(&Rule{Name: "rule-a", Priority: 10, Destinations: []*Destination{{Service: "old-svc", Weight: 1}}})
		r.AddRule(&Rule{Name: "rule-a", Priority: 10, Destinations: []*Destination{{Service: "new-svc", Weight: 1}}})

		rules := r.ListRules()
		if len(rules) != 1 {
			t.Errorf("expected 1 rule after update, got %d", len(rules))
		}
		if rules[0].Destinations[0].Service != "new-svc" {
			t.Errorf("expected updated destination new-svc, got %s", rules[0].Destinations[0].Service)
		}
	})

	t.Run("UnnamedRule", func(t *testing.T) {
		r := NewRouter()
		r.AddRule(&Rule{Priority: 5, Destinations: []*Destination{{Service: "anon-svc", Weight: 1}}})
		rules := r.ListRules()
		if len(rules) != 1 {
			t.Errorf("expected 1 rule, got %d", len(rules))
		}
	})
}

func TestRouter_RemoveRule(t *testing.T) {
	t.Run("Exists", func(t *testing.T) {
		r := NewRouter()
		r.AddRule(&Rule{Name: "removable", Priority: 10, Destinations: []*Destination{{Service: "svc", Weight: 1}}})
		err := r.RemoveRule("removable")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rules := r.ListRules()
		if len(rules) != 0 {
			t.Errorf("expected 0 rules after removal, got %d", len(rules))
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		r := NewRouter()
		err := r.RemoveRule("nonexistent")
		if err != ErrRouteNotFound {
			t.Errorf("expected ErrRouteNotFound, got %v", err)
		}
	})
}

func TestRouter_GetRule(t *testing.T) {
	t.Run("Exists", func(t *testing.T) {
		r := NewRouter()
		r.AddRule(&Rule{Name: "my-rule", Priority: 5, Destinations: []*Destination{{Service: "svc", Weight: 1}}})

		rule, err := r.GetRule("my-rule")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rule.Name != "my-rule" {
			t.Errorf("expected my-rule, got %s", rule.Name)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		r := NewRouter()
		_, err := r.GetRule("missing")
		if err != ErrRouteNotFound {
			t.Errorf("expected ErrRouteNotFound, got %v", err)
		}
	})
}

func TestRouter_Route(t *testing.T) {
	t.Run("MatchesHighestPriority", func(t *testing.T) {
		r := NewRouter()
		r.AddRule(&Rule{
			Name:     "catch-all",
			Priority: 1,
			Matches:  []*Match{},
			Destinations: []*Destination{
				{Service: "default-svc", Weight: 1},
			},
		})
		r.AddRule(&Rule{
			Name:     "api-rule",
			Priority: 100,
			Matches: []*Match{
				{URI: NewPrefixMatch("/api/")},
			},
			Destinations: []*Destination{
				{Service: "api-svc", Weight: 1},
			},
		})

		ctx := context.Background()
		match, err := r.Route(ctx, &Request{Path: "/api/v1/users"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if match.Destination.Service != "api-svc" {
			t.Errorf("expected api-svc (high priority), got %s", match.Destination.Service)
		}
	})

	t.Run("FallsThrough", func(t *testing.T) {
		r := NewRouter()
		r.AddRule(&Rule{
			Name:     "specific",
			Priority: 100,
			Matches: []*Match{
				{URI: NewExactMatch("/specific")},
			},
			Destinations: []*Destination{
				{Service: "specific-svc", Weight: 1},
			},
		})
		r.AddRule(&Rule{
			Name:     "fallback",
			Priority: 1,
			Matches:  []*Match{}, // matches everything
			Destinations: []*Destination{
				{Service: "fallback-svc", Weight: 1},
			},
		})

		ctx := context.Background()
		match, err := r.Route(ctx, &Request{Path: "/other"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if match.Destination.Service != "fallback-svc" {
			t.Errorf("expected fallback-svc, got %s", match.Destination.Service)
		}
	})

	t.Run("NoMatch", func(t *testing.T) {
		r := NewRouter()
		r.AddRule(&Rule{
			Name:     "specific-only",
			Priority: 10,
			Matches: []*Match{
				{URI: NewExactMatch("/specific")},
			},
			Destinations: []*Destination{
				{Service: "svc", Weight: 1},
			},
		})

		ctx := context.Background()
		_, err := r.Route(ctx, &Request{Path: "/nope"})
		if err != ErrNoMatchingRoute {
			t.Errorf("expected ErrNoMatchingRoute, got %v", err)
		}
	})

	t.Run("EmptyRouter", func(t *testing.T) {
		r := NewRouter()
		ctx := context.Background()
		_, err := r.Route(ctx, &Request{Path: "/anything"})
		if err != ErrNoMatchingRoute {
			t.Errorf("expected ErrNoMatchingRoute, got %v", err)
		}
	})
}

func TestRouter_ListRules(t *testing.T) {
	r := NewRouter()
	r.AddRule(&Rule{Name: "a", Priority: 1, Destinations: []*Destination{{Service: "svc-a", Weight: 1}}})
	r.AddRule(&Rule{Name: "b", Priority: 2, Destinations: []*Destination{{Service: "svc-b", Weight: 1}}})

	rules := r.ListRules()
	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}

	// Verify it returns a copy (modifying should not affect router).
	rules[0] = nil
	original := r.ListRules()
	if original[0] == nil {
		t.Error("expected ListRules to return a copy")
	}
}

// ==================== Concurrency Tests ====================

func TestRouter_ConcurrentAccess(t *testing.T) {
	r := NewRouter()
	r.AddRule(&Rule{
		Name:     "base",
		Priority: 1,
		Destinations: []*Destination{
			{Service: "base-svc", Weight: 1},
		},
	})

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				r.Route(ctx, &Request{Path: "/test"})
				r.ListRules()
				r.GetRule("base")
			}
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				name := "concurrent"
				r.AddRule(&Rule{
					Name:         name,
					Priority:     idx,
					Destinations: []*Destination{{Service: "svc", Weight: 1}},
				})
				r.RemoveRule(name)
			}
		}(i)
	}

	wg.Wait()
}

// ==================== Config Struct Tests ====================

func TestConfigStructs(t *testing.T) {
	t.Run("RetryConfig", func(t *testing.T) {
		rc := &RetryConfig{
			Attempts:      3,
			PerTryTimeout: 5 * time.Second,
			RetryOn:       []string{"5xx", "gateway-error"},
		}
		if rc.Attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", rc.Attempts)
		}
		if rc.PerTryTimeout != 5*time.Second {
			t.Errorf("expected 5s timeout, got %v", rc.PerTryTimeout)
		}
		if len(rc.RetryOn) != 2 {
			t.Errorf("expected 2 retry conditions, got %d", len(rc.RetryOn))
		}
	})

	t.Run("MirrorConfig", func(t *testing.T) {
		mc := &MirrorConfig{
			Service:    "shadow-svc",
			Subset:     "canary",
			Percentage: 10,
		}
		if mc.Service != "shadow-svc" {
			t.Errorf("expected shadow-svc, got %s", mc.Service)
		}
		if mc.Percentage != 10 {
			t.Errorf("expected 10%%, got %d", mc.Percentage)
		}
	})

	t.Run("FaultConfig", func(t *testing.T) {
		fc := &FaultConfig{
			Delay: &DelayFault{
				Duration:   100 * time.Millisecond,
				Percentage: 5,
			},
			Abort: &AbortFault{
				StatusCode: 503,
				Percentage: 1,
			},
		}
		if fc.Delay.Duration != 100*time.Millisecond {
			t.Errorf("expected 100ms delay, got %v", fc.Delay.Duration)
		}
		if fc.Abort.StatusCode != 503 {
			t.Errorf("expected status 503, got %d", fc.Abort.StatusCode)
		}
	})

	t.Run("Destination", func(t *testing.T) {
		d := &Destination{
			Service: "backend",
			Subset:  "v2",
			Weight:  80,
			Port:    8080,
		}
		if d.Service != "backend" {
			t.Errorf("expected backend, got %s", d.Service)
		}
		if d.Subset != "v2" {
			t.Errorf("expected v2, got %s", d.Subset)
		}
		if d.Weight != 80 {
			t.Errorf("expected weight 80, got %d", d.Weight)
		}
		if d.Port != 8080 {
			t.Errorf("expected port 8080, got %d", d.Port)
		}
	})

	t.Run("Request", func(t *testing.T) {
		req := &Request{
			Method:          "POST",
			Path:            "/api/v1/create",
			Host:            "example.com",
			Headers:         map[string]string{"Authorization": "Bearer tok"},
			SourceService:   "frontend",
			SourceNamespace: "production",
			Metadata:        map[string]string{"trace_id": "abc123"},
		}
		if req.Method != "POST" {
			t.Errorf("expected POST, got %s", req.Method)
		}
		if req.Host != "example.com" {
			t.Errorf("expected example.com, got %s", req.Host)
		}
		if req.SourceService != "frontend" {
			t.Errorf("expected frontend, got %s", req.SourceService)
		}
		if req.Metadata["trace_id"] != "abc123" {
			t.Errorf("expected trace_id abc123, got %s", req.Metadata["trace_id"])
		}
	})

	t.Run("Response", func(t *testing.T) {
		resp := &Response{
			Service:    "backend",
			Endpoint:   "10.0.0.1:8080",
			Headers:    map[string]string{"X-Request-Id": "123"},
			StatusCode: 200,
			Body:       []byte(`{"ok":true}`),
		}
		if resp.StatusCode != 200 {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		if resp.Endpoint != "10.0.0.1:8080" {
			t.Errorf("expected 10.0.0.1:8080, got %s", resp.Endpoint)
		}
	})

	t.Run("RouteMatch", func(t *testing.T) {
		rm := &RouteMatch{
			Rule:        &Rule{Name: "test"},
			Destination: &Destination{Service: "backend"},
			Headers:     map[string]string{"X-Match": "true"},
			Metadata:    map[string]string{"key": "val"},
		}
		if rm.Rule.Name != "test" {
			t.Errorf("expected rule name test, got %s", rm.Rule.Name)
		}
		if rm.Metadata["key"] != "val" {
			t.Errorf("expected metadata key=val, got %s", rm.Metadata["key"])
		}
	})
}
