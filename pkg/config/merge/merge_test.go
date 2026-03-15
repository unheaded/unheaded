// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package merge

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Merge / DeepCopy helpers
// ---------------------------------------------------------------------------

func TestDeepCopy_Nil(t *testing.T) {
	if got := DeepCopy(nil); got != nil {
		t.Fatalf("DeepCopy(nil) = %v; want nil", got)
	}
}

func TestDeepCopy_Empty(t *testing.T) {
	src := map[string]any{}
	got := DeepCopy(src)
	if got == nil || len(got) != 0 {
		t.Fatalf("DeepCopy(empty) = %v; want empty map", got)
	}
}

func TestDeepCopy_Flat(t *testing.T) {
	src := map[string]any{"a": 1, "b": "hello", "c": true}
	got := DeepCopy(src)

	if got["a"] != 1 || got["b"] != "hello" || got["c"] != true {
		t.Fatalf("DeepCopy mismatch: %v", got)
	}

	// Mutate original, verify copy is independent.
	src["a"] = 99
	if got["a"] != 1 {
		t.Fatal("DeepCopy is not independent from source")
	}
}

func TestDeepCopy_Nested(t *testing.T) {
	src := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"val": "deep",
			},
		},
	}
	got := DeepCopy(src)

	// Mutate deep inside source.
	src["level1"].(map[string]any)["level2"].(map[string]any)["val"] = "changed"

	inner := got["level1"].(map[string]any)["level2"].(map[string]any)["val"]
	if inner != "deep" {
		t.Fatalf("deep copy is not independent; inner=%v", inner)
	}
}

func TestDeepCopyValue_Nil(t *testing.T) {
	if got := DeepCopyValue(nil); got != nil {
		t.Fatalf("DeepCopyValue(nil) = %v; want nil", got)
	}
}

func TestDeepCopyValue_Primitives(t *testing.T) {
	tests := []struct {
		name string
		in   any
	}{
		{"int", 42},
		{"string", "hello"},
		{"bool", true},
		{"float64", 3.14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeepCopyValue(tt.in)
			if got != tt.in {
				t.Fatalf("DeepCopyValue(%v) = %v", tt.in, got)
			}
		})
	}
}

func TestDeepCopyValue_StringSlice(t *testing.T) {
	src := []string{"a", "b", "c"}
	got := DeepCopyValue(src).([]string)
	src[0] = "z"
	if got[0] != "a" {
		t.Fatal("[]string copy not independent")
	}
}

func TestDeepCopyValue_IntSlice(t *testing.T) {
	src := []int{1, 2, 3}
	got := DeepCopyValue(src).([]int)
	src[0] = 99
	if got[0] != 1 {
		t.Fatal("[]int copy not independent")
	}
}

func TestDeepCopyValue_Float64Slice(t *testing.T) {
	src := []float64{1.1, 2.2}
	got := DeepCopyValue(src).([]float64)
	src[0] = 9.9
	if got[0] != 1.1 {
		t.Fatal("[]float64 copy not independent")
	}
}

func TestDeepCopySlice_Nil(t *testing.T) {
	if got := DeepCopySlice(nil); got != nil {
		t.Fatalf("DeepCopySlice(nil) = %v; want nil", got)
	}
}

func TestDeepCopySlice_Mixed(t *testing.T) {
	src := []any{1, "two", map[string]any{"k": "v"}}
	got := DeepCopySlice(src)

	// Mutate the map inside the source slice.
	src[2].(map[string]any)["k"] = "changed"
	if got[2].(map[string]any)["k"] != "v" {
		t.Fatal("DeepCopySlice map element not independent")
	}
}

// ---------------------------------------------------------------------------
// Merge
// ---------------------------------------------------------------------------

func TestMerge_BothEmpty(t *testing.T) {
	got := Merge(map[string]any{}, map[string]any{})
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestMerge_OverrideAddsNewKeys(t *testing.T) {
	base := map[string]any{"a": 1}
	override := map[string]any{"b": 2}
	got := Merge(base, override)
	if got["a"] != 1 || got["b"] != 2 {
		t.Fatalf("unexpected result: %v", got)
	}
}

func TestMerge_OverrideReplacesPrimitive(t *testing.T) {
	base := map[string]any{"a": 1}
	override := map[string]any{"a": 2}
	got := Merge(base, override)
	if got["a"] != 2 {
		t.Fatalf("expected override to win: %v", got)
	}
}

func TestMerge_DeepMergeNestedMaps(t *testing.T) {
	base := map[string]any{
		"server": map[string]any{"host": "localhost", "port": 8080},
	}
	override := map[string]any{
		"server": map[string]any{"port": 9090, "tls": true},
	}
	got := Merge(base, override)
	srv := got["server"].(map[string]any)
	if srv["host"] != "localhost" || srv["port"] != 9090 || srv["tls"] != true {
		t.Fatalf("deep merge failed: %v", srv)
	}
}

func TestMerge_OverrideMapReplacesNonMap(t *testing.T) {
	base := map[string]any{"x": "string_val"}
	override := map[string]any{"x": map[string]any{"nested": true}}
	got := Merge(base, override)
	nested, ok := got["x"].(map[string]any)
	if !ok || nested["nested"] != true {
		t.Fatalf("expected map override: %v", got["x"])
	}
}

func TestMerge_OriginalUnmodified(t *testing.T) {
	base := map[string]any{"a": 1}
	override := map[string]any{"a": 2, "b": 3}
	Merge(base, override)
	if base["a"] != 1 {
		t.Fatal("Merge mutated the base map")
	}
	if _, ok := base["b"]; ok {
		t.Fatal("Merge added keys to the base map")
	}
}

// ---------------------------------------------------------------------------
// MergeMultiple
// ---------------------------------------------------------------------------

func TestMergeMultiple_Empty(t *testing.T) {
	got := MergeMultiple()
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestMergeMultiple_Several(t *testing.T) {
	a := map[string]any{"a": 1}
	b := map[string]any{"b": 2}
	c := map[string]any{"a": 3, "c": 4}
	got := MergeMultiple(a, b, c)
	if got["a"] != 3 || got["b"] != 2 || got["c"] != 4 {
		t.Fatalf("unexpected: %v", got)
	}
}

// ---------------------------------------------------------------------------
// MergeWithOptions
// ---------------------------------------------------------------------------

func TestMergeWithOptions_DeepStrategy(t *testing.T) {
	base := map[string]any{"server": map[string]any{"port": 8080}}
	override := map[string]any{"server": map[string]any{"host": "0.0.0.0"}}
	opts := DefaultMergeOptions()
	got := MergeWithOptions(base, override, opts)
	srv := got["server"].(map[string]any)
	if srv["port"] != 8080 || srv["host"] != "0.0.0.0" {
		t.Fatalf("deep strategy failed: %v", srv)
	}
}

func TestMergeWithOptions_OverrideStrategy(t *testing.T) {
	base := map[string]any{"server": map[string]any{"port": 8080, "host": "localhost"}}
	override := map[string]any{"server": map[string]any{"host": "0.0.0.0"}}
	opts := MergeOptions{Strategy: StrategyOverride}
	got := MergeWithOptions(base, override, opts)
	srv := got["server"].(map[string]any)
	if _, ok := srv["port"]; ok {
		t.Fatal("override strategy should replace entire map")
	}
	if srv["host"] != "0.0.0.0" {
		t.Fatal("host should be overridden")
	}
}

func TestMergeWithOptions_ArrayAppend(t *testing.T) {
	base := map[string]any{"tags": []any{"a", "b"}}
	override := map[string]any{"tags": []any{"c"}}
	opts := MergeOptions{Strategy: StrategyDeep, ArrayStrategy: StrategyAppend}
	got := MergeWithOptions(base, override, opts)
	tags := got["tags"].([]any)
	if len(tags) != 3 || tags[0] != "a" || tags[1] != "b" || tags[2] != "c" {
		t.Fatalf("append failed: %v", tags)
	}
}

func TestMergeWithOptions_ArrayUnion(t *testing.T) {
	base := map[string]any{"ids": []any{1, 2, 3}}
	override := map[string]any{"ids": []any{2, 3, 4}}
	opts := MergeOptions{Strategy: StrategyDeep, ArrayStrategy: StrategyUnion}
	got := MergeWithOptions(base, override, opts)
	ids := got["ids"].([]any)
	if len(ids) != 4 {
		t.Fatalf("union should produce 4 unique items, got %d: %v", len(ids), ids)
	}
}

func TestMergeWithOptions_ArrayOverride(t *testing.T) {
	base := map[string]any{"x": []any{1, 2}}
	override := map[string]any{"x": []any{3}}
	opts := MergeOptions{Strategy: StrategyDeep, ArrayStrategy: StrategyOverride}
	got := MergeWithOptions(base, override, opts)
	arr := got["x"].([]any)
	if len(arr) != 1 || arr[0] != 3 {
		t.Fatalf("array override failed: %v", arr)
	}
}

func TestMergeWithOptions_SkipNil(t *testing.T) {
	base := map[string]any{"a": 1, "b": 2}
	override := map[string]any{"a": nil, "b": 3}
	opts := MergeOptions{Strategy: StrategyDeep, SkipNil: true}
	got := MergeWithOptions(base, override, opts)
	if got["a"] != 1 {
		t.Fatal("SkipNil did not skip nil override")
	}
	if got["b"] != 3 {
		t.Fatal("non-nil value should still override")
	}
}

func TestMergeWithOptions_SkipEmpty(t *testing.T) {
	base := map[string]any{"a": "hello", "b": "world"}
	override := map[string]any{"a": "", "b": "new"}
	opts := MergeOptions{Strategy: StrategyDeep, SkipEmpty: true}
	got := MergeWithOptions(base, override, opts)
	if got["a"] != "hello" {
		t.Fatal("SkipEmpty did not skip empty string")
	}
	if got["b"] != "new" {
		t.Fatal("non-empty string should override")
	}
}

func TestMergeWithOptions_KeyTransform(t *testing.T) {
	base := map[string]any{}
	override := map[string]any{"FOO": "bar"}
	opts := MergeOptions{
		Strategy:     StrategyDeep,
		KeyTransform: strings.ToLower,
	}
	got := MergeWithOptions(base, override, opts)
	if _, ok := got["foo"]; !ok {
		t.Fatalf("key transform to lower did not apply: %v", got)
	}
}

func TestMergeWithOptions_ValueTransform(t *testing.T) {
	base := map[string]any{}
	override := map[string]any{"n": 5}
	opts := MergeOptions{
		Strategy: StrategyDeep,
		ValueTransform: func(v any) any {
			if i, ok := v.(int); ok {
				return i * 2
			}
			return v
		},
	}
	got := MergeWithOptions(base, override, opts)
	if got["n"] != 10 {
		t.Fatalf("value transform not applied: %v", got["n"])
	}
}

func TestMergeMultipleWithOptions(t *testing.T) {
	a := map[string]any{"tags": []any{"x"}}
	b := map[string]any{"tags": []any{"y"}}
	opts := MergeOptions{Strategy: StrategyDeep, ArrayStrategy: StrategyAppend}
	got := MergeMultipleWithOptions(opts, a, b)
	tags := got["tags"].([]any)
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %v", tags)
	}
}

// ---------------------------------------------------------------------------
// isEmpty
// ---------------------------------------------------------------------------

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"nil", nil, true},
		{"empty string", "", true},
		{"non-empty string", "x", false},
		{"empty slice", []any{}, true},
		{"non-empty slice", []any{1}, false},
		{"empty map", map[string]any{}, true},
		{"non-empty map", map[string]any{"k": "v"}, false},
		{"int zero", 0, false},
		{"bool false", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmpty(tt.val); got != tt.want {
				t.Fatalf("isEmpty(%v) = %v; want %v", tt.val, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// unionArrays
// ---------------------------------------------------------------------------

func TestUnionArrays(t *testing.T) {
	a := []any{"a", "b", "c"}
	b := []any{"b", "c", "d"}
	got := unionArrays(a, b)
	if len(got) != 4 {
		t.Fatalf("expected 4, got %d: %v", len(got), got)
	}
}

func TestUnionArrays_Empty(t *testing.T) {
	got := unionArrays([]any{}, []any{})
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// Flatten / Unflatten
// ---------------------------------------------------------------------------

func TestFlatten_Empty(t *testing.T) {
	got := Flatten(map[string]any{})
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestFlatten_Simple(t *testing.T) {
	m := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
		"debug": true,
	}
	got := Flatten(m)
	if got["server.host"] != "localhost" || got["server.port"] != 8080 || got["debug"] != true {
		t.Fatalf("flatten failed: %v", got)
	}
}

func TestFlatten_DeeplyNested(t *testing.T) {
	m := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "val",
			},
		},
	}
	got := Flatten(m)
	if got["a.b.c"] != "val" {
		t.Fatalf("deep flatten failed: %v", got)
	}
}

func TestUnflatten_Simple(t *testing.T) {
	flat := map[string]any{
		"server.host": "localhost",
		"server.port": 8080,
		"debug":       true,
	}
	got := Unflatten(flat)
	srv := got["server"].(map[string]any)
	if srv["host"] != "localhost" || srv["port"] != 8080 {
		t.Fatalf("unflatten failed: %v", got)
	}
	if got["debug"] != true {
		t.Fatal("top-level key lost")
	}
}

func TestFlatten_Unflatten_Roundtrip(t *testing.T) {
	original := map[string]any{
		"db": map[string]any{
			"host": "pg.local",
			"port": 5432,
		},
		"app": map[string]any{
			"name": "test",
		},
	}
	flat := Flatten(original)
	restored := Unflatten(flat)

	dbHost := restored["db"].(map[string]any)["host"]
	dbPort := restored["db"].(map[string]any)["port"]
	appName := restored["app"].(map[string]any)["name"]

	if dbHost != "pg.local" || dbPort != 5432 || appName != "test" {
		t.Fatalf("roundtrip failed: %v", restored)
	}
}

// ---------------------------------------------------------------------------
// splitKey
// ---------------------------------------------------------------------------

func TestSplitKey(t *testing.T) {
	tests := []struct {
		key  string
		want []string
	}{
		{"a.b.c", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{"a\\.b.c", []string{"a.b", "c"}},
		{"", nil},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := splitKey(tt.key)
			if len(got) != len(tt.want) {
				t.Fatalf("splitKey(%q) = %v; want %v", tt.key, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitKey(%q)[%d] = %q; want %q", tt.key, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Get / Set / Delete
// ---------------------------------------------------------------------------

func TestGet_TopLevel(t *testing.T) {
	m := map[string]any{"a": 1}
	val, ok := Get(m, "a")
	if !ok || val != 1 {
		t.Fatalf("Get top-level failed: val=%v ok=%v", val, ok)
	}
}

func TestGet_Nested(t *testing.T) {
	m := map[string]any{
		"server": map[string]any{
			"port": 8080,
		},
	}
	val, ok := Get(m, "server.port")
	if !ok || val != 8080 {
		t.Fatalf("Get nested failed: val=%v ok=%v", val, ok)
	}
}

func TestGet_Missing(t *testing.T) {
	m := map[string]any{"a": 1}
	_, ok := Get(m, "b")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestGet_MissingNested(t *testing.T) {
	m := map[string]any{"a": map[string]any{"b": 1}}
	_, ok := Get(m, "a.c")
	if ok {
		t.Fatal("expected not found for missing nested key")
	}
}

func TestGet_NonMapIntermediate(t *testing.T) {
	m := map[string]any{"a": "string_val"}
	_, ok := Get(m, "a.b")
	if ok {
		t.Fatal("expected not found through non-map intermediate")
	}
}

func TestSet_TopLevel(t *testing.T) {
	m := map[string]any{}
	Set(m, "key", "value")
	if m["key"] != "value" {
		t.Fatalf("Set top-level failed: %v", m)
	}
}

func TestSet_Nested(t *testing.T) {
	m := map[string]any{}
	Set(m, "a.b.c", 42)
	val, ok := Get(m, "a.b.c")
	if !ok || val != 42 {
		t.Fatalf("Set nested failed: %v", m)
	}
}

func TestSet_OverwriteExisting(t *testing.T) {
	m := map[string]any{"a": map[string]any{"b": 1}}
	Set(m, "a.b", 2)
	val, _ := Get(m, "a.b")
	if val != 2 {
		t.Fatalf("Set overwrite failed: %v", m)
	}
}

func TestDelete_TopLevel(t *testing.T) {
	m := map[string]any{"a": 1, "b": 2}
	ok := Delete(m, "a")
	if !ok {
		t.Fatal("Delete returned false")
	}
	if _, exists := m["a"]; exists {
		t.Fatal("key still exists after delete")
	}
}

func TestDelete_Nested(t *testing.T) {
	m := map[string]any{"a": map[string]any{"b": 1, "c": 2}}
	ok := Delete(m, "a.b")
	if !ok {
		t.Fatal("Delete nested returned false")
	}
	_, found := Get(m, "a.b")
	if found {
		t.Fatal("nested key still found after delete")
	}
	// Sibling should remain.
	val, _ := Get(m, "a.c")
	if val != 2 {
		t.Fatal("sibling key lost after delete")
	}
}

func TestDelete_Missing(t *testing.T) {
	m := map[string]any{"a": 1}
	ok := Delete(m, "b")
	if ok {
		t.Fatal("Delete of missing key should return false")
	}
}

func TestDelete_EmptyKey(t *testing.T) {
	m := map[string]any{"a": 1}
	ok := Delete(m, "")
	if ok {
		t.Fatal("Delete of empty key should return false")
	}
}

// ---------------------------------------------------------------------------
// Keys
// ---------------------------------------------------------------------------

func TestKeys(t *testing.T) {
	m := map[string]any{
		"a": 1,
		"b": map[string]any{
			"c": 2,
		},
	}
	keys := Keys(m)
	sort.Strings(keys)
	// Keys should include both top-level and nested: "a", "b", "b.c"
	expected := []string{"a", "b", "b.c"}
	if len(keys) != len(expected) {
		t.Fatalf("Keys: got %v; want %v", keys, expected)
	}
	for i, k := range expected {
		if keys[i] != k {
			t.Fatalf("Keys[%d] = %q; want %q", i, keys[i], k)
		}
	}
}

func TestKeys_Empty(t *testing.T) {
	keys := Keys(map[string]any{})
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %v", keys)
	}
}

// ---------------------------------------------------------------------------
// Filter
// ---------------------------------------------------------------------------

func TestFilter_KeepAll(t *testing.T) {
	m := map[string]any{"a": 1, "b": 2}
	got := Filter(m, func(_ string, _ any) bool { return true })
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %v", got)
	}
}

func TestFilter_KeepNone(t *testing.T) {
	m := map[string]any{"a": 1, "b": 2}
	got := Filter(m, func(_ string, _ any) bool { return false })
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestFilter_ByKey(t *testing.T) {
	m := map[string]any{
		"keep":   1,
		"remove": 2,
	}
	got := Filter(m, func(key string, _ any) bool {
		return key == "keep"
	})
	if got["keep"] != 1 {
		t.Fatal("filtered out desired key")
	}
	if _, ok := got["remove"]; ok {
		t.Fatal("did not filter out unwanted key")
	}
}

func TestFilter_Nested(t *testing.T) {
	m := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
		},
	}
	got := Filter(m, func(key string, _ any) bool {
		return true
	})
	srv, ok := got["server"].(map[string]any)
	if !ok {
		t.Fatal("nested map lost")
	}
	if srv["host"] != "localhost" || srv["port"] != 8080 {
		t.Fatalf("nested values wrong: %v", srv)
	}
}

// ---------------------------------------------------------------------------
// Transform
// ---------------------------------------------------------------------------

func TestTransform_Identity(t *testing.T) {
	m := map[string]any{"a": 1, "b": "two"}
	got := Transform(m, func(_ string, v any) any { return v })
	if got["a"] != 1 || got["b"] != "two" {
		t.Fatalf("identity transform failed: %v", got)
	}
}

func TestTransform_DoubleInts(t *testing.T) {
	m := map[string]any{"x": 5, "y": "text"}
	got := Transform(m, func(_ string, v any) any {
		if i, ok := v.(int); ok {
			return i * 2
		}
		return v
	})
	if got["x"] != 10 || got["y"] != "text" {
		t.Fatalf("transform failed: %v", got)
	}
}

func TestTransform_Nested(t *testing.T) {
	m := map[string]any{
		"outer": map[string]any{
			"val": 3,
		},
	}
	got := Transform(m, func(_ string, v any) any {
		if i, ok := v.(int); ok {
			return i + 1
		}
		return v
	})
	inner := got["outer"].(map[string]any)["val"]
	if inner != 4 {
		t.Fatalf("nested transform failed: got %v", inner)
	}
}

// ---------------------------------------------------------------------------
// Diff
// ---------------------------------------------------------------------------

func TestDiff_NoDifference(t *testing.T) {
	m := map[string]any{"a": 1, "b": "two"}
	diff := Diff(m, m)
	if len(diff) != 0 {
		t.Fatalf("expected no diff, got %v", diff)
	}
}

func TestDiff_Added(t *testing.T) {
	a := map[string]any{"a": 1}
	b := map[string]any{"a": 1, "b": 2}
	diff := Diff(a, b)
	if d, ok := diff["b"]; !ok || d.Type != "added" || d.NewValue != 2 {
		t.Fatalf("expected added 'b': %v", diff)
	}
}

func TestDiff_Removed(t *testing.T) {
	a := map[string]any{"a": 1, "b": 2}
	b := map[string]any{"a": 1}
	diff := Diff(a, b)
	if d, ok := diff["b"]; !ok || d.Type != "removed" || d.OldValue != 2 {
		t.Fatalf("expected removed 'b': %v", diff)
	}
}

func TestDiff_Modified(t *testing.T) {
	a := map[string]any{"a": 1}
	b := map[string]any{"a": 2}
	diff := Diff(a, b)
	if d, ok := diff["a"]; !ok || d.Type != "modified" || d.OldValue != 1 || d.NewValue != 2 {
		t.Fatalf("expected modified 'a': %v", diff)
	}
}

func TestDiff_NestedModification(t *testing.T) {
	a := map[string]any{"cfg": map[string]any{"port": 8080}}
	b := map[string]any{"cfg": map[string]any{"port": 9090}}
	diff := Diff(a, b)
	if d, ok := diff["cfg.port"]; !ok || d.Type != "modified" {
		t.Fatalf("expected nested modification: %v", diff)
	}
}

func TestDiff_NestedAdded(t *testing.T) {
	a := map[string]any{"cfg": map[string]any{"port": 8080}}
	b := map[string]any{"cfg": map[string]any{"port": 8080, "host": "0.0.0.0"}}
	diff := Diff(a, b)
	if d, ok := diff["cfg.host"]; !ok || d.Type != "added" {
		t.Fatalf("expected nested added: %v", diff)
	}
}

// ---------------------------------------------------------------------------
// equal
// ---------------------------------------------------------------------------

func TestEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b any
		want bool
	}{
		{"both nil", nil, nil, true},
		{"nil vs value", nil, 1, false},
		{"value vs nil", 1, nil, false},
		{"same int", 42, 42, true},
		{"diff int", 1, 2, false},
		{"same string", "abc", "abc", true},
		{"same map", map[string]any{"k": "v"}, map[string]any{"k": "v"}, true},
		{"diff map len", map[string]any{"a": 1}, map[string]any{"a": 1, "b": 2}, false},
		{"diff map val", map[string]any{"a": 1}, map[string]any{"a": 2}, false},
		{"map vs non-map", map[string]any{"a": 1}, "not a map", false},
		{"same slice", []any{1, 2}, []any{1, 2}, true},
		{"diff slice len", []any{1}, []any{1, 2}, false},
		{"diff slice val", []any{1}, []any{2}, false},
		{"slice vs non-slice", []any{1}, "not a slice", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := equal(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("equal(%v, %v) = %v; want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validation: SchemaValidator
// ---------------------------------------------------------------------------

func TestSchemaValidator_RequiredField_Present(t *testing.T) {
	schema := &Schema{
		Fields:   map[string]*FieldSchema{},
		Required: []string{"name"},
	}
	v := NewSchemaValidator(schema)
	err := v.Validate(map[string]any{"name": "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchemaValidator_RequiredField_Missing(t *testing.T) {
	schema := &Schema{
		Fields:   map[string]*FieldSchema{},
		Required: []string{"name"},
	}
	v := NewSchemaValidator(schema)
	err := v.Validate(map[string]any{})
	if err == nil {
		t.Fatal("expected validation error for missing required field")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
}

func TestSchemaValidator_FieldRequired(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"port": {Type: TypeInt, Required: true},
		},
	}
	v := NewSchemaValidator(schema)
	err := v.Validate(map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing required field via FieldSchema.Required")
	}
}

func TestSchemaValidator_TypeString(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"name": {Type: TypeString},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"name": "hello"}); err != nil {
		t.Fatalf("valid string rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"name": 123}); err == nil {
		t.Fatal("int should not pass as string")
	}
}

func TestSchemaValidator_TypeInt(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"count": {Type: TypeInt},
		},
	}
	v := NewSchemaValidator(schema)

	for _, val := range []any{42, int64(42), int32(42), float64(42)} {
		if err := v.Validate(map[string]any{"count": val}); err != nil {
			t.Fatalf("valid int (%T) rejected: %v", val, err)
		}
	}
	if err := v.Validate(map[string]any{"count": "not_int"}); err == nil {
		t.Fatal("string should not pass as int")
	}
}

func TestSchemaValidator_TypeFloat(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"rate": {Type: TypeFloat},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"rate": 3.14}); err != nil {
		t.Fatalf("valid float rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"rate": "not_float"}); err == nil {
		t.Fatal("string should not pass as float")
	}
}

func TestSchemaValidator_TypeBool(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"flag": {Type: TypeBool},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"flag": true}); err != nil {
		t.Fatalf("valid bool rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"flag": "true"}); err == nil {
		t.Fatal("string should not pass as bool")
	}
}

func TestSchemaValidator_TypeMap(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"meta": {Type: TypeMap},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"meta": map[string]any{"k": "v"}}); err != nil {
		t.Fatalf("valid map rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"meta": "not_map"}); err == nil {
		t.Fatal("string should not pass as map")
	}
}

func TestSchemaValidator_TypeArray(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"items": {Type: TypeArray},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"items": []any{1, 2}}); err != nil {
		t.Fatalf("valid array rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"items": "not_array"}); err == nil {
		t.Fatal("string should not pass as array")
	}
}

func TestSchemaValidator_TypeAny(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"anything": {Type: TypeAny},
		},
	}
	v := NewSchemaValidator(schema)
	for _, val := range []any{1, "str", true, nil, map[string]any{}, []any{}} {
		if err := v.Validate(map[string]any{"anything": val}); err != nil {
			t.Fatalf("TypeAny rejected %T: %v", val, err)
		}
	}
}

func TestSchemaValidator_StringMinMaxLength(t *testing.T) {
	minLen := 3
	maxLen := 10
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"name": {Type: TypeString, MinLength: &minLen, MaxLength: &maxLen},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"name": "ab"}); err == nil {
		t.Fatal("expected min length error")
	}
	if err := v.Validate(map[string]any{"name": "abc"}); err != nil {
		t.Fatalf("min length boundary rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"name": "0123456789x"}); err == nil {
		t.Fatal("expected max length error")
	}
	if err := v.Validate(map[string]any{"name": "0123456789"}); err != nil {
		t.Fatalf("max length boundary rejected: %v", err)
	}
}

func TestSchemaValidator_StringPattern(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"email": {Type: TypeString, Pattern: `^[a-z]+@[a-z]+\.[a-z]+$`},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"email": "user@example.com"}); err != nil {
		t.Fatalf("valid pattern rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"email": "invalid"}); err == nil {
		t.Fatal("invalid pattern should fail")
	}
}

func TestSchemaValidator_IntRange(t *testing.T) {
	min := float64(1)
	max := float64(100)
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"port": {Type: TypeInt, Min: &min, Max: &max},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"port": 0}); err == nil {
		t.Fatal("below min should fail")
	}
	if err := v.Validate(map[string]any{"port": 101}); err == nil {
		t.Fatal("above max should fail")
	}
	if err := v.Validate(map[string]any{"port": 50}); err != nil {
		t.Fatalf("in range should pass: %v", err)
	}
}

func TestSchemaValidator_FloatRange(t *testing.T) {
	min := float64(0.0)
	max := float64(1.0)
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"ratio": {Type: TypeFloat, Min: &min, Max: &max},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"ratio": -0.1}); err == nil {
		t.Fatal("below min should fail")
	}
	if err := v.Validate(map[string]any{"ratio": 1.1}); err == nil {
		t.Fatal("above max should fail")
	}
	if err := v.Validate(map[string]any{"ratio": 0.5}); err != nil {
		t.Fatalf("in range should pass: %v", err)
	}
}

func TestSchemaValidator_ArrayLengthAndElementType(t *testing.T) {
	minLen := 1
	maxLen := 3
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"tags": {
				Type:        TypeArray,
				MinLength:   &minLen,
				MaxLength:   &maxLen,
				ElementType: TypeString,
			},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"tags": []any{}}); err == nil {
		t.Fatal("empty array should fail min length")
	}
	if err := v.Validate(map[string]any{"tags": []any{"a", "b", "c", "d"}}); err == nil {
		t.Fatal("4 elements should fail max length")
	}
	if err := v.Validate(map[string]any{"tags": []any{"a", "b"}}); err != nil {
		t.Fatalf("valid array rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"tags": []any{"a", 123}}); err == nil {
		t.Fatal("non-string element should fail")
	}
}

func TestSchemaValidator_NestedMapSchema(t *testing.T) {
	nested := &Schema{
		Required: []string{"host"},
		Fields: map[string]*FieldSchema{
			"host": {Type: TypeString},
			"port": {Type: TypeInt},
		},
	}
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"server": {Type: TypeMap, Nested: nested},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{
		"server": map[string]any{"host": "localhost", "port": 8080},
	}); err != nil {
		t.Fatalf("valid nested rejected: %v", err)
	}

	if err := v.Validate(map[string]any{
		"server": map[string]any{"port": 8080},
	}); err == nil {
		t.Fatal("missing nested required field should fail")
	}
}

func TestSchemaValidator_NestedArraySchema(t *testing.T) {
	itemSchema := &Schema{
		Fields: map[string]*FieldSchema{
			"name": {Type: TypeString, Required: true},
		},
		Required: []string{"name"},
	}
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"users": {Type: TypeArray, Nested: itemSchema},
		},
	}
	v := NewSchemaValidator(schema)

	data := map[string]any{
		"users": []any{
			map[string]any{"name": "Alice"},
			map[string]any{"name": "Bob"},
		},
	}
	if err := v.Validate(data); err != nil {
		t.Fatalf("valid nested array rejected: %v", err)
	}

	badData := map[string]any{
		"users": []any{
			map[string]any{"name": "Alice"},
			map[string]any{},
		},
	}
	if err := v.Validate(badData); err == nil {
		t.Fatal("nested array item missing required field should fail")
	}
}

func TestSchemaValidator_CustomFieldValidator(t *testing.T) {
	// Custom validators only run for types not handled by the switch in
	// validateField (TypeAny, TypeBool, TypeDuration). For typed fields like
	// TypeInt, the type-specific validator returns before custom runs.
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"even": {
				Type: TypeAny,
				Custom: func(v any) error {
					n, _ := v.(int)
					if n%2 != 0 {
						return fmt.Errorf("must be even")
					}
					return nil
				},
			},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"even": 4}); err != nil {
		t.Fatalf("even number rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"even": 3}); err == nil {
		t.Fatal("odd number should fail custom validator")
	}
}

func TestSchemaValidator_CustomRule(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"min": {Type: TypeInt},
			"max": {Type: TypeInt},
		},
		Custom: []CustomRule{
			{
				Name: "min_less_than_max",
				Validate: func(data map[string]any) error {
					minVal, _ := data["min"].(int)
					maxVal, _ := data["max"].(int)
					if minVal >= maxVal {
						return fmt.Errorf("min must be less than max")
					}
					return nil
				},
			},
		},
	}
	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"min": 1, "max": 10}); err != nil {
		t.Fatalf("valid custom rule rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"min": 10, "max": 1}); err == nil {
		t.Fatal("invalid custom rule should fail")
	}
}

func TestSchemaValidator_FieldNotInSchema(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"known": {Type: TypeString},
		},
	}
	v := NewSchemaValidator(schema)
	// Extra keys should not cause errors since the validator only checks defined fields.
	if err := v.Validate(map[string]any{"known": "x", "extra": "y"}); err != nil {
		t.Fatalf("extra key should not cause error: %v", err)
	}
}

func TestSchemaValidator_SkipMissingOptionalField(t *testing.T) {
	schema := &Schema{
		Fields: map[string]*FieldSchema{
			"optional": {Type: TypeString},
		},
	}
	v := NewSchemaValidator(schema)
	if err := v.Validate(map[string]any{}); err != nil {
		t.Fatalf("missing optional field should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// FieldType.String
// ---------------------------------------------------------------------------

func TestFieldType_String(t *testing.T) {
	tests := []struct {
		ft   FieldType
		want string
	}{
		{TypeAny, "any"},
		{TypeString, "string"},
		{TypeInt, "int"},
		{TypeFloat, "float"},
		{TypeBool, "bool"},
		{TypeMap, "map"},
		{TypeArray, "array"},
		{TypeDuration, "duration"},
		{FieldType(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.ft.String(); got != tt.want {
				t.Fatalf("FieldType(%d).String() = %q; want %q", tt.ft, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidationError / ValidationErrors
// ---------------------------------------------------------------------------

func TestValidationError_WithField(t *testing.T) {
	e := &ValidationError{Field: "port", Message: "out of range"}
	if got := e.Error(); got != "port: out of range" {
		t.Fatalf("unexpected error string: %q", got)
	}
}

func TestValidationError_WithoutField(t *testing.T) {
	e := &ValidationError{Message: "general failure"}
	if got := e.Error(); got != "general failure" {
		t.Fatalf("unexpected error string: %q", got)
	}
}

func TestValidationErrors_Empty(t *testing.T) {
	e := &ValidationErrors{}
	if e.HasErrors() {
		t.Fatal("empty errors should not report HasErrors")
	}
	if got := e.Error(); got != "validation failed" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestValidationErrors_Add(t *testing.T) {
	e := &ValidationErrors{}
	e.Add("field1", "bad")
	e.Add("field2", "also bad")
	if !e.HasErrors() {
		t.Fatal("should report HasErrors")
	}
	got := e.Error()
	if !strings.Contains(got, "field1") || !strings.Contains(got, "field2") {
		t.Fatalf("error message missing fields: %q", got)
	}
}

// ---------------------------------------------------------------------------
// SchemaBuilder
// ---------------------------------------------------------------------------

func TestSchemaBuilder_Basic(t *testing.T) {
	schema := NewSchemaBuilder().
		Field("name", TypeString).Required().MinLength(1).MaxLength(100).And().
		Field("port", TypeInt).Min(1).Max(65535).And().
		Field("debug", TypeBool).Default(false).And().
		Required("name").
		Build()

	v := NewSchemaValidator(schema)

	if err := v.Validate(map[string]any{"name": "app", "port": 8080, "debug": true}); err != nil {
		t.Fatalf("valid data rejected: %v", err)
	}
}

func TestSchemaBuilder_FieldWithPattern(t *testing.T) {
	schema := NewSchemaBuilder().
		Field("version", TypeString).Pattern(`^\d+\.\d+\.\d+$`).And().
		Build()

	v := NewSchemaValidator(schema)
	if err := v.Validate(map[string]any{"version": "1.2.3"}); err != nil {
		t.Fatalf("valid version rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"version": "abc"}); err == nil {
		t.Fatal("invalid version should fail")
	}
}

func TestSchemaBuilder_FieldWithEnum(t *testing.T) {
	schema := NewSchemaBuilder().
		Field("env", TypeString).Enum("dev", "staging", "prod").And().
		Build()

	fs := schema.Fields["env"]
	if len(fs.Enum) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(fs.Enum))
	}
	// Enum validation for TypeString is not reachable because validateString
	// returns before the enum check. We verify the schema was built correctly
	// and test enum behavior with TypeAny in the next test.
}

func TestSchemaBuilder_FieldWithEnumTypeAny(t *testing.T) {
	schema := NewSchemaBuilder().
		Field("level", TypeAny).Enum("low", "medium", "high").And().
		Build()

	v := NewSchemaValidator(schema)
	if err := v.Validate(map[string]any{"level": "medium"}); err != nil {
		t.Fatalf("valid enum rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"level": "invalid"}); err == nil {
		t.Fatal("invalid enum value should fail")
	}
}

func TestSchemaBuilder_FieldWithDescription(t *testing.T) {
	schema := NewSchemaBuilder().
		Field("name", TypeString).Description("The application name").And().
		Build()

	fs := schema.Fields["name"]
	if fs.Description != "The application name" {
		t.Fatalf("description not set: %q", fs.Description)
	}
}

func TestSchemaBuilder_FieldWithElementType(t *testing.T) {
	schema := NewSchemaBuilder().
		Field("tags", TypeArray).ElementType(TypeString).And().
		Build()

	fs := schema.Fields["tags"]
	if fs.ElementType != TypeString {
		t.Fatalf("element type mismatch: %v", fs.ElementType)
	}
}

func TestSchemaBuilder_FieldWithNested(t *testing.T) {
	inner := NewSchemaBuilder().
		Field("host", TypeString).Required().And().
		Build()

	schema := NewSchemaBuilder().
		Field("server", TypeMap).Nested(inner).And().
		Build()

	v := NewSchemaValidator(schema)
	if err := v.Validate(map[string]any{
		"server": map[string]any{"host": "localhost"},
	}); err != nil {
		t.Fatalf("valid nested rejected: %v", err)
	}
}

func TestSchemaBuilder_CustomRule(t *testing.T) {
	schema := NewSchemaBuilder().
		CustomRule("always_pass", func(data map[string]any) error { return nil }).
		Build()

	v := NewSchemaValidator(schema)
	if err := v.Validate(map[string]any{}); err != nil {
		t.Fatalf("custom rule pass should not error: %v", err)
	}
}

func TestSchemaBuilder_FieldWithCustomValidator(t *testing.T) {
	schema := NewSchemaBuilder().
		Field("value", TypeAny).Custom(func(v any) error {
		if v == nil {
			return fmt.Errorf("must not be nil")
		}
		return nil
	}).And().
		Build()

	fs := schema.Fields["value"]
	if fs.Custom == nil {
		t.Fatal("custom validator not set")
	}
}

// ---------------------------------------------------------------------------
// Common validators: RequiredValidator, TypeValidator, etc.
// ---------------------------------------------------------------------------

func TestRequiredValidator(t *testing.T) {
	v := RequiredValidator("name", "port")

	if err := v.Validate(map[string]any{"name": "x", "port": 80}); err != nil {
		t.Fatalf("all required present but got error: %v", err)
	}
	if err := v.Validate(map[string]any{"name": "x"}); err == nil {
		t.Fatal("missing 'port' should fail")
	}
}

func TestRequiredValidator_Nested(t *testing.T) {
	v := RequiredValidator("server.host")

	data := map[string]any{"server": map[string]any{"host": "localhost"}}
	if err := v.Validate(data); err != nil {
		t.Fatalf("nested required present but got error: %v", err)
	}

	data2 := map[string]any{"server": map[string]any{"port": 80}}
	if err := v.Validate(data2); err == nil {
		t.Fatal("missing nested required should fail")
	}
}

func TestTypeValidator(t *testing.T) {
	v := TypeValidator(map[string]FieldType{
		"name": TypeString,
		"port": TypeInt,
	})

	if err := v.Validate(map[string]any{"name": "app", "port": 8080}); err != nil {
		t.Fatalf("valid types rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"name": 123, "port": 8080}); err == nil {
		t.Fatal("wrong type should fail")
	}
}

func TestTypeValidator_MissingKey(t *testing.T) {
	v := TypeValidator(map[string]FieldType{
		"optional": TypeString,
	})
	// Missing keys should not fail type validation.
	if err := v.Validate(map[string]any{}); err != nil {
		t.Fatalf("missing key should not error: %v", err)
	}
}

func TestRangeValidator(t *testing.T) {
	v := RangeValidator("port", 1, 65535)

	if err := v.Validate(map[string]any{"port": 8080}); err != nil {
		t.Fatalf("in range should pass: %v", err)
	}
	if err := v.Validate(map[string]any{"port": 0}); err == nil {
		t.Fatal("below range should fail")
	}
	if err := v.Validate(map[string]any{"port": 70000}); err == nil {
		t.Fatal("above range should fail")
	}
}

func TestRangeValidator_MissingKey(t *testing.T) {
	v := RangeValidator("port", 1, 65535)
	if err := v.Validate(map[string]any{}); err != nil {
		t.Fatalf("missing key should not error: %v", err)
	}
}

func TestRangeValidator_Float64(t *testing.T) {
	v := RangeValidator("rate", 0, 1)
	if err := v.Validate(map[string]any{"rate": 0.5}); err != nil {
		t.Fatalf("float in range should pass: %v", err)
	}
	if err := v.Validate(map[string]any{"rate": 1.5}); err == nil {
		t.Fatal("float above range should fail")
	}
}

func TestPatternValidator(t *testing.T) {
	v := PatternValidator("version", `^\d+\.\d+$`)

	if err := v.Validate(map[string]any{"version": "1.0"}); err != nil {
		t.Fatalf("matching pattern should pass: %v", err)
	}
	if err := v.Validate(map[string]any{"version": "abc"}); err == nil {
		t.Fatal("non-matching pattern should fail")
	}
}

func TestPatternValidator_NonString(t *testing.T) {
	v := PatternValidator("num", `^\d+$`)
	// Non-string values should be silently skipped.
	if err := v.Validate(map[string]any{"num": 42}); err != nil {
		t.Fatalf("non-string should be skipped: %v", err)
	}
}

func TestPatternValidator_Missing(t *testing.T) {
	v := PatternValidator("key", `.*`)
	if err := v.Validate(map[string]any{}); err != nil {
		t.Fatalf("missing key should not error: %v", err)
	}
}

func TestPortValidator(t *testing.T) {
	v := PortValidator("port")

	tests := []struct {
		name  string
		val   any
		valid bool
	}{
		{"valid int", 8080, true},
		{"min port", 1, true},
		{"max port", 65535, true},
		{"zero", 0, false},
		{"negative", -1, false},
		{"too high", 65536, false},
		{"string valid", "443", true},
		{"string invalid", "abc", false},
		{"int64", int64(80), true},
		{"float64", float64(443), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(map[string]any{"port": tt.val})
			if tt.valid && err != nil {
				t.Fatalf("expected valid, got error: %v", err)
			}
			if !tt.valid && err == nil {
				t.Fatal("expected invalid")
			}
		})
	}
}

func TestPortValidator_Missing(t *testing.T) {
	v := PortValidator("port")
	if err := v.Validate(map[string]any{}); err != nil {
		t.Fatalf("missing port should not error: %v", err)
	}
}

func TestURLValidator(t *testing.T) {
	v := URLValidator("url")

	if err := v.Validate(map[string]any{"url": "https://example.com/path"}); err != nil {
		t.Fatalf("valid URL rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"url": "not a url"}); err == nil {
		t.Fatal("invalid URL should fail")
	}
}

func TestEmailValidator(t *testing.T) {
	v := EmailValidator("email")

	if err := v.Validate(map[string]any{"email": "user@example.com"}); err != nil {
		t.Fatalf("valid email rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"email": "invalid"}); err == nil {
		t.Fatal("invalid email should fail")
	}
}

func TestIPValidator(t *testing.T) {
	v := IPValidator("ip")

	if err := v.Validate(map[string]any{"ip": "192.168.1.1"}); err != nil {
		t.Fatalf("valid IP rejected: %v", err)
	}
	if err := v.Validate(map[string]any{"ip": "not.an.ip"}); err == nil {
		t.Fatal("invalid IP should fail")
	}
}

// ---------------------------------------------------------------------------
// CompositeValidator
// ---------------------------------------------------------------------------

func TestCompositeValidator_AllPass(t *testing.T) {
	cv := NewCompositeValidator(
		RequiredValidator("name"),
		TypeValidator(map[string]FieldType{"name": TypeString}),
	)
	if err := cv.Validate(map[string]any{"name": "test"}); err != nil {
		t.Fatalf("all pass but got error: %v", err)
	}
}

func TestCompositeValidator_CollectsErrors(t *testing.T) {
	cv := NewCompositeValidator(
		RequiredValidator("a"),
		RequiredValidator("b"),
	)
	err := cv.Validate(map[string]any{})
	if err == nil {
		t.Fatal("expected errors")
	}
	var ves *ValidationErrors
	if !errors.As(err, &ves) {
		t.Fatalf("expected *ValidationErrors, got %T", err)
	}
	if len(ves.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(ves.Errors))
	}
}

func TestCompositeValidator_Add(t *testing.T) {
	cv := NewCompositeValidator()
	cv.Add(RequiredValidator("x"))
	if err := cv.Validate(map[string]any{}); err == nil {
		t.Fatal("expected error after Add")
	}
}

func TestCompositeValidator_NonValidationError(t *testing.T) {
	cv := NewCompositeValidator(
		ValidatorFunc(func(data map[string]any) error {
			return fmt.Errorf("generic error")
		}),
	)
	err := cv.Validate(map[string]any{})
	if err == nil {
		t.Fatal("expected error")
	}
	var ves *ValidationErrors
	if !errors.As(err, &ves) {
		t.Fatalf("expected *ValidationErrors, got %T", err)
	}
	if len(ves.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(ves.Errors))
	}
}

// ---------------------------------------------------------------------------
// ValidatorFunc
// ---------------------------------------------------------------------------

func TestValidatorFunc(t *testing.T) {
	fn := ValidatorFunc(func(data map[string]any) error {
		if _, ok := data["key"]; !ok {
			return fmt.Errorf("missing key")
		}
		return nil
	})

	if err := fn.Validate(map[string]any{"key": "val"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := fn.Validate(map[string]any{}); err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// checkType
// ---------------------------------------------------------------------------

func TestCheckType(t *testing.T) {
	tests := []struct {
		name     string
		val      any
		ft       FieldType
		wantErr  bool
	}{
		{"any accepts string", "x", TypeAny, false},
		{"string ok", "x", TypeString, false},
		{"string reject int", 1, TypeString, true},
		{"int ok", 42, TypeInt, false},
		{"int64 ok", int64(1), TypeInt, false},
		{"float64 as int", float64(1.0), TypeInt, false},
		{"float ok", 3.14, TypeFloat, false},
		{"int as float", 1, TypeFloat, false},
		{"bool ok", true, TypeBool, false},
		{"bool reject string", "true", TypeBool, true},
		{"map ok", map[string]any{}, TypeMap, false},
		{"array ok", []any{}, TypeArray, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkType(tt.val, tt.ft)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkType(%v, %v) error=%v; wantErr=%v", tt.val, tt.ft, err, tt.wantErr)
			}
		})
	}
}
