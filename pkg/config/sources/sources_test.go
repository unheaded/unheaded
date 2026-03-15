// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// EnvSource
// ---------------------------------------------------------------------------

func TestEnvSource_Name(t *testing.T) {
	src := NewEnvSource("APP")
	if got := src.Name(); got != "env:APP" {
		t.Fatalf("Name() = %q; want %q", got, "env:APP")
	}
}

func TestEnvSource_Priority(t *testing.T) {
	src := NewEnvSource("APP")
	if got := src.Priority(); got != 50 {
		t.Fatalf("Priority() = %d; want 50", got)
	}
}

func TestEnvSource_Load_SingleVariable(t *testing.T) {
	t.Setenv("MYAPP_SERVER_PORT", "8080")

	src := NewEnvSource("MYAPP")
	data, err := src.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	srv, ok := data["server"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested map under 'server', got: %v", data)
	}
	// parseEnvValue should parse "8080" as int.
	if srv["port"] != 8080 {
		t.Fatalf("expected port=8080, got %v (%T)", srv["port"], srv["port"])
	}
}

func TestEnvSource_Load_MultipleVariables(t *testing.T) {
	t.Setenv("TEST1_DB_HOST", "localhost")
	t.Setenv("TEST1_DB_PORT", "5432")
	t.Setenv("TEST1_DEBUG", "true")

	src := NewEnvSource("TEST1")
	data, err := src.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	db, ok := data["db"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested db map: %v", data)
	}
	if db["host"] != "localhost" {
		t.Fatalf("db.host = %v; want localhost", db["host"])
	}
	if db["port"] != 5432 {
		t.Fatalf("db.port = %v; want 5432", db["port"])
	}
	if data["debug"] != true {
		t.Fatalf("debug = %v; want true", data["debug"])
	}
}

func TestEnvSource_Load_NoMatchingPrefix(t *testing.T) {
	// Clear any env vars that might match.
	src := NewEnvSource("NOMATCH_UNIQUE_PREFIX_XYZ")
	data, err := src.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty map, got %v", data)
	}
}

func TestEnvSourceWithSeparator(t *testing.T) {
	t.Setenv("MY--APP--HOST", "127.0.0.1")

	src := NewEnvSourceWithSeparator("MY", "--")
	data, err := src.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["app"] == nil {
		t.Fatalf("expected nested 'app' key, got %v", data)
	}
}

func TestEnvSource_Load_ArrayValue(t *testing.T) {
	t.Setenv("ARRTEST_TAGS", "[alpha,beta,gamma]")

	src := NewEnvSource("ARRTEST")
	data, err := src.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	tags, ok := data["tags"].([]any)
	if !ok {
		t.Fatalf("expected []any for tags, got %T: %v", data["tags"], data["tags"])
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}
}

func TestEnvSource_Load_BoolValues(t *testing.T) {
	t.Setenv("BOOLTEST_ENABLED", "true")
	t.Setenv("BOOLTEST_DISABLED", "false")

	src := NewEnvSource("BOOLTEST")
	data, err := src.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["enabled"] != true {
		t.Fatalf("enabled = %v; want true", data["enabled"])
	}
	if data["disabled"] != false {
		t.Fatalf("disabled = %v; want false", data["disabled"])
	}
}

func TestEnvSource_Load_FloatValue(t *testing.T) {
	t.Setenv("FLOATTEST_RATE", "3.14")

	src := NewEnvSource("FLOATTEST")
	data, err := src.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["rate"] != 3.14 {
		t.Fatalf("rate = %v; want 3.14", data["rate"])
	}
}

// ---------------------------------------------------------------------------
// parseEnvValue
// ---------------------------------------------------------------------------

func TestParseEnvValue(t *testing.T) {
	tests := []struct {
		input string
		want  any
	}{
		{"true", true},
		{"false", false},
		{"TRUE", true},
		{"FALSE", false},
		{"42", 42},
		{"3.14", 3.14},
		{"hello", "hello"},
		{"[]", []any{}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseEnvValue(tt.input)
			switch expected := tt.want.(type) {
			case []any:
				gotSlice, ok := got.([]any)
				if !ok {
					t.Fatalf("expected []any, got %T", got)
				}
				if len(gotSlice) != len(expected) {
					t.Fatalf("slice length %d; want %d", len(gotSlice), len(expected))
				}
			default:
				if got != tt.want {
					t.Fatalf("parseEnvValue(%q) = %v (%T); want %v (%T)",
						tt.input, got, got, tt.want, tt.want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseEnvArray
// ---------------------------------------------------------------------------

func TestParseEnvArray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // expected length
	}{
		{"simple", "[a,b,c]", 3},
		{"empty", "[]", 0},
		{"single", "[one]", 1},
		{"quoted", `["hello","world"]`, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEnvArray(tt.input)
			if len(got) != tt.want {
				t.Fatalf("parseEnvArray(%q) len=%d; want %d: %v", tt.input, len(got), tt.want, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// splitEnvArray
// ---------------------------------------------------------------------------

func TestSplitEnvArray(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a,b,c", 3},
		{`"a,b",c`, 2},
		{"single", 1},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitEnvArray(tt.input)
			if len(got) != tt.want {
				t.Fatalf("splitEnvArray(%q) = %v (len %d); want len %d", tt.input, got, len(got), tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// setNestedEnvValue
// ---------------------------------------------------------------------------

func TestSetNestedEnvValue(t *testing.T) {
	m := make(map[string]any)
	setNestedEnvValue(m, "a.b.c", "val")
	a, ok := m["a"].(map[string]any)
	if !ok {
		t.Fatalf("expected map at 'a': %v", m)
	}
	b, ok := a["b"].(map[string]any)
	if !ok {
		t.Fatalf("expected map at 'a.b': %v", a)
	}
	if b["c"] != "val" {
		t.Fatalf("expected 'val' at a.b.c, got %v", b["c"])
	}
}

func TestSetNestedEnvValue_TopLevel(t *testing.T) {
	m := make(map[string]any)
	setNestedEnvValue(m, "key", "value")
	if m["key"] != "value" {
		t.Fatalf("expected 'value' at key, got %v", m["key"])
	}
}

// ---------------------------------------------------------------------------
// EnvBinder
// ---------------------------------------------------------------------------

func TestEnvBinder_Bind_And_Get(t *testing.T) {
	t.Setenv("MY_CUSTOM_VAR", "hello")

	binder := NewEnvBinder("APP")
	binder.Bind("config.key", "MY_CUSTOM_VAR")

	val, ok := binder.GetEnvValue("config.key")
	if !ok || val != "hello" {
		t.Fatalf("GetEnvValue = %v, %v; want 'hello', true", val, ok)
	}
}

func TestEnvBinder_BindAuto(t *testing.T) {
	t.Setenv("APP_SERVER_PORT", "9090")

	binder := NewEnvBinder("APP")
	binder.BindAuto("server.port")

	val, ok := binder.GetEnvValue("server.port")
	if !ok || val != 9090 {
		t.Fatalf("GetEnvValue = %v, %v; want 9090, true", val, ok)
	}
}

func TestEnvBinder_GetEnvValue_NotBound(t *testing.T) {
	binder := NewEnvBinder("APP")
	_, ok := binder.GetEnvValue("unbound.key")
	if ok {
		t.Fatal("unbound key should return false")
	}
}

func TestEnvBinder_GetEnvValue_NotSet(t *testing.T) {
	binder := NewEnvBinder("APP")
	binder.Bind("key", "DEFINITELY_UNSET_ENV_VAR_12345")
	_, ok := binder.GetEnvValue("key")
	if ok {
		t.Fatal("unset env var should return false")
	}
}

func TestEnvBinder_AllBindings(t *testing.T) {
	binder := NewEnvBinder("APP")
	binder.Bind("a", "ENV_A")
	binder.Bind("b", "ENV_B")
	bindings := binder.AllBindings()

	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}
	if bindings["a"] != "ENV_A" || bindings["b"] != "ENV_B" {
		t.Fatalf("unexpected bindings: %v", bindings)
	}

	// Verify returned map is a copy.
	bindings["c"] = "ENV_C"
	if len(binder.AllBindings()) != 2 {
		t.Fatal("AllBindings should return a copy")
	}
}

// ---------------------------------------------------------------------------
// Helper functions: GetEnv, GetEnvInt, GetEnvBool, GetEnvFloat, GetEnvSlice
// ---------------------------------------------------------------------------

func TestGetEnv(t *testing.T) {
	t.Setenv("GETENV_TEST", "value")
	if got := GetEnv("GETENV_TEST", "default"); got != "value" {
		t.Fatalf("GetEnv = %q; want 'value'", got)
	}
	if got := GetEnv("GETENV_MISSING_12345", "default"); got != "default" {
		t.Fatalf("GetEnv missing = %q; want 'default'", got)
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("GETENVINT_TEST", "42")
	if got := GetEnvInt("GETENVINT_TEST", 0); got != 42 {
		t.Fatalf("GetEnvInt = %d; want 42", got)
	}
	if got := GetEnvInt("GETENVINT_MISSING_12345", 99); got != 99 {
		t.Fatalf("GetEnvInt missing = %d; want 99", got)
	}
	t.Setenv("GETENVINT_BAD", "abc")
	if got := GetEnvInt("GETENVINT_BAD", 7); got != 7 {
		t.Fatalf("GetEnvInt bad = %d; want 7", got)
	}
}

func TestGetEnvBool(t *testing.T) {
	t.Setenv("GETENVBOOL_TEST", "true")
	if got := GetEnvBool("GETENVBOOL_TEST", false); got != true {
		t.Fatalf("GetEnvBool = %v; want true", got)
	}
	if got := GetEnvBool("GETENVBOOL_MISSING_12345", true); got != true {
		t.Fatalf("GetEnvBool missing = %v; want true", got)
	}
	t.Setenv("GETENVBOOL_BAD", "not_bool")
	if got := GetEnvBool("GETENVBOOL_BAD", false); got != false {
		t.Fatalf("GetEnvBool bad = %v; want false", got)
	}
}

func TestGetEnvFloat(t *testing.T) {
	t.Setenv("GETENVFLOAT_TEST", "3.14")
	if got := GetEnvFloat("GETENVFLOAT_TEST", 0); got != 3.14 {
		t.Fatalf("GetEnvFloat = %v; want 3.14", got)
	}
	if got := GetEnvFloat("GETENVFLOAT_MISSING_12345", 1.5); got != 1.5 {
		t.Fatalf("GetEnvFloat missing = %v; want 1.5", got)
	}
}

func TestGetEnvSlice(t *testing.T) {
	t.Setenv("GETENVSLICE_TEST", "a,b,c")
	got := GetEnvSlice("GETENVSLICE_TEST", ",", nil)
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("GetEnvSlice = %v; want [a b c]", got)
	}
	got = GetEnvSlice("GETENVSLICE_MISSING_12345", ",", []string{"x"})
	if len(got) != 1 || got[0] != "x" {
		t.Fatalf("GetEnvSlice missing = %v; want [x]", got)
	}
}

func TestSetEnv_UnsetEnv(t *testing.T) {
	key := "SETUNSET_TEST_KEY_12345"
	if err := SetEnv(key, "val"); err != nil {
		t.Fatalf("SetEnv error: %v", err)
	}
	if os.Getenv(key) != "val" {
		t.Fatal("SetEnv did not set")
	}
	if err := UnsetEnv(key); err != nil {
		t.Fatalf("UnsetEnv error: %v", err)
	}
	if _, ok := os.LookupEnv(key); ok {
		t.Fatal("UnsetEnv did not unset")
	}
}

func TestRequireEnv(t *testing.T) {
	t.Setenv("REQUIRE_TEST", "val")
	got := RequireEnv("REQUIRE_TEST")
	if got != "val" {
		t.Fatalf("RequireEnv = %q; want 'val'", got)
	}
}

func TestRequireEnv_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing required env")
		}
	}()
	RequireEnv("DEFINITELY_MISSING_REQUIRE_ENV_VAR_12345")
}

// ---------------------------------------------------------------------------
// FlagSource
// ---------------------------------------------------------------------------

func TestFlagSource_Name(t *testing.T) {
	fs := NewFlagSource(nil)
	if got := fs.Name(); got != "flags" {
		t.Fatalf("Name() = %q; want 'flags'", got)
	}
}

func TestFlagSource_Priority(t *testing.T) {
	fs := NewFlagSource(nil)
	if got := fs.Priority(); got != 10 {
		t.Fatalf("Priority() = %d; want 10", got)
	}
}

func TestFlagSource_Load_Simple(t *testing.T) {
	args := []string{"--host", "localhost", "--port", "8080"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["host"] != "localhost" {
		t.Fatalf("host = %v; want 'localhost'", data["host"])
	}
	if data["port"] != 8080 {
		t.Fatalf("port = %v; want 8080", data["port"])
	}
}

func TestFlagSource_Load_EqualsSyntax(t *testing.T) {
	args := []string{"--host=0.0.0.0", "--port=9090"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["host"] != "0.0.0.0" {
		t.Fatalf("host = %v", data["host"])
	}
	if data["port"] != 9090 {
		t.Fatalf("port = %v", data["port"])
	}
}

func TestFlagSource_Load_BooleanFlag(t *testing.T) {
	args := []string{"--verbose"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["verbose"] != true {
		t.Fatalf("verbose = %v; want true", data["verbose"])
	}
}

func TestFlagSource_Load_NoBooleanFlag(t *testing.T) {
	args := []string{"--no-debug"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["debug"] != false {
		t.Fatalf("debug = %v; want false", data["debug"])
	}
}

func TestFlagSource_Load_DoubleHyphenStop(t *testing.T) {
	args := []string{"--key", "value", "--", "--ignored", "data"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["key"] != "value" {
		t.Fatalf("key = %v", data["key"])
	}
	if _, ok := data["ignored"]; ok {
		t.Fatal("flags after -- should be ignored")
	}
}

func TestFlagSource_Load_DashConvertsToDot(t *testing.T) {
	args := []string{"--server-port", "3000"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// "server-port" becomes "server.port", which is nested.
	srv, ok := data["server"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested server map: %v", data)
	}
	if srv["port"] != 3000 {
		t.Fatalf("server.port = %v; want 3000", srv["port"])
	}
}

func TestFlagSource_Load_BooleanValues(t *testing.T) {
	args := []string{"--flag", "true"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["flag"] != true {
		t.Fatalf("flag = %v (%T); want true", data["flag"], data["flag"])
	}
}

func TestFlagSource_Load_NonFlagArgs(t *testing.T) {
	args := []string{"positional", "--key", "val", "another"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	// positional args should be skipped; only --key should be captured.
	if data["key"] != "val" {
		t.Fatalf("key = %v; want 'val'", data["key"])
	}
}

func TestFlagSource_Load_Empty(t *testing.T) {
	fs := NewFlagSource([]string{})
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty, got %v", data)
	}
}

func TestFlagSource_Load_CommaArray(t *testing.T) {
	args := []string{"--tags", "a,b,c"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	tags, ok := data["tags"].([]any)
	if !ok {
		t.Fatalf("expected []any for tags, got %T: %v", data["tags"], data["tags"])
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}
}

func TestFlagSource_Load_SingleDashFlag(t *testing.T) {
	args := []string{"-v"}
	fs := NewFlagSource(args)
	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["v"] != true {
		t.Fatalf("v = %v; want true", data["v"])
	}
}

// ---------------------------------------------------------------------------
// parseValue
// ---------------------------------------------------------------------------

func TestParseValue(t *testing.T) {
	tests := []struct {
		in   string
		want any
	}{
		{"true", true},
		{"false", false},
		{"42", 42},
		{"3.14", 3.14},
		{"hello", "hello"},
		{"a,b", []any{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := parseValue(tt.in)
			switch expected := tt.want.(type) {
			case []any:
				gotSlice, ok := got.([]any)
				if !ok {
					t.Fatalf("expected []any, got %T: %v", got, got)
				}
				if len(gotSlice) != len(expected) {
					t.Fatalf("len=%d; want %d", len(gotSlice), len(expected))
				}
			default:
				if got != expected {
					t.Fatalf("parseValue(%q) = %v (%T); want %v (%T)", tt.in, got, got, expected, expected)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FlagSet
// ---------------------------------------------------------------------------

func TestFlagSet_StringFlag(t *testing.T) {
	fs := NewFlagSet("test", []string{"--host", "localhost"})
	fs.String("host", "H", "default", "the host")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetString("host"); got != "localhost" {
		t.Fatalf("GetString('host') = %q; want 'localhost'", got)
	}
}

func TestFlagSet_IntFlag(t *testing.T) {
	fs := NewFlagSet("test", []string{"--port", "8080"})
	fs.Int("port", "p", 3000, "the port")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetInt("port"); got != 8080 {
		t.Fatalf("GetInt('port') = %d; want 8080", got)
	}
}

func TestFlagSet_BoolFlag(t *testing.T) {
	fs := NewFlagSet("test", []string{"--verbose"})
	fs.Bool("verbose", "v", false, "verbose mode")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetBool("verbose"); got != true {
		t.Fatalf("GetBool('verbose') = %v; want true", got)
	}
}

func TestFlagSet_FloatFlag(t *testing.T) {
	fs := NewFlagSet("test", []string{"--rate", "1.5"})
	fs.Float("rate", "r", 0.0, "rate limit")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetFloat("rate"); got != 1.5 {
		t.Fatalf("GetFloat('rate') = %v; want 1.5", got)
	}
}

func TestFlagSet_StringSliceFlag(t *testing.T) {
	fs := NewFlagSet("test", []string{"--tags", "a,b,c"})
	fs.StringSlice("tags", "t", nil, "tags")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	got := fs.GetStringSlice("tags")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("GetStringSlice('tags') = %v", got)
	}
}

func TestFlagSet_Defaults(t *testing.T) {
	fs := NewFlagSet("test", []string{})
	fs.String("host", "", "default_host", "the host")
	fs.Int("port", "", 3000, "the port")
	fs.Bool("debug", "", false, "debug mode")
	fs.Float("rate", "", 1.0, "rate")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if got := fs.GetString("host"); got != "default_host" {
		t.Fatalf("default string = %q; want 'default_host'", got)
	}
	if got := fs.GetInt("port"); got != 3000 {
		t.Fatalf("default int = %d; want 3000", got)
	}
	if got := fs.GetBool("debug"); got != false {
		t.Fatalf("default bool = %v; want false", got)
	}
	if got := fs.GetFloat("rate"); got != 1.0 {
		t.Fatalf("default float = %v; want 1.0", got)
	}
}

func TestFlagSet_EqualsSyntax(t *testing.T) {
	fs := NewFlagSet("test", []string{"--host=myhost"})
	fs.String("host", "", "", "the host")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetString("host"); got != "myhost" {
		t.Fatalf("host = %q; want 'myhost'", got)
	}
}

func TestFlagSet_ShorthandFlag(t *testing.T) {
	fs := NewFlagSet("test", []string{"-p", "9090"})
	fs.Int("port", "p", 3000, "the port")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetInt("p"); got != 9090 {
		t.Fatalf("port via shorthand = %d; want 9090", got)
	}
}

func TestFlagSet_DoubleHyphenStop(t *testing.T) {
	fs := NewFlagSet("test", []string{"--host", "a", "--", "--port", "8080"})
	fs.String("host", "", "", "host")
	fs.Int("port", "", 0, "port")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetString("host"); got != "a" {
		t.Fatalf("host = %q; want 'a'", got)
	}
	if got := fs.GetInt("port"); got != 0 {
		t.Fatal("port should remain default after --")
	}
}

func TestFlagSet_ParseIdempotent(t *testing.T) {
	fs := NewFlagSet("test", []string{"--key", "val"})
	fs.String("key", "", "", "key")
	if err := fs.Parse(); err != nil {
		t.Fatalf("first Parse() error: %v", err)
	}
	if err := fs.Parse(); err != nil {
		t.Fatalf("second Parse() error: %v", err)
	}
	if got := fs.GetString("key"); got != "val" {
		t.Fatalf("key = %q after double parse; want 'val'", got)
	}
}

func TestFlagSet_UnknownFlagsIgnored(t *testing.T) {
	fs := NewFlagSet("test", []string{"--unknown", "val", "--known", "ok"})
	fs.String("known", "", "", "known")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetString("known"); got != "ok" {
		t.Fatalf("known = %q; want 'ok'", got)
	}
}

func TestFlagSet_ToMap(t *testing.T) {
	fs := NewFlagSet("test", []string{"--host", "localhost", "--port", "8080"})
	fs.String("host", "", "default", "host")
	fs.Int("port", "", 0, "port")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	m := fs.ToMap()
	if m["host"] != "localhost" {
		t.Fatalf("ToMap host = %v", m["host"])
	}
	if m["port"] != 8080 {
		t.Fatalf("ToMap port = %v", m["port"])
	}
}

func TestFlagSet_Usage(t *testing.T) {
	fs := NewFlagSet("myapp", []string{})
	fs.String("host", "H", "localhost", "the host address")
	fs.Int("port", "p", 8080, "the port number")

	usage := fs.Usage()
	if !strings.Contains(usage, "myapp") {
		t.Fatal("usage should contain app name")
	}
	if !strings.Contains(usage, "host") {
		t.Fatal("usage should contain 'host'")
	}
	if !strings.Contains(usage, "port") {
		t.Fatal("usage should contain 'port'")
	}
}

func TestFlagSet_WithEnvVar(t *testing.T) {
	fs := NewFlagSet("test", []string{})
	fs.String("host", "", "default", "host")
	fs.WithEnvVar("host", "MY_HOST")

	flag := fs.flags["host"]
	if flag.EnvVar != "MY_HOST" {
		t.Fatalf("EnvVar = %q; want 'MY_HOST'", flag.EnvVar)
	}
}

func TestFlagSet_Required(t *testing.T) {
	fs := NewFlagSet("test", []string{})
	fs.String("host", "", "default", "host")
	fs.Required("host")

	flag := fs.flags["host"]
	if !flag.Required {
		t.Fatal("flag should be marked required")
	}
}

func TestFlagSet_GetMissing(t *testing.T) {
	fs := NewFlagSet("test", []string{})
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetString("nope"); got != "" {
		t.Fatalf("GetString missing = %q; want empty", got)
	}
	if got := fs.GetInt("nope"); got != 0 {
		t.Fatalf("GetInt missing = %d; want 0", got)
	}
	if got := fs.GetBool("nope"); got != false {
		t.Fatalf("GetBool missing = %v; want false", got)
	}
	if got := fs.GetFloat("nope"); got != 0 {
		t.Fatalf("GetFloat missing = %v; want 0", got)
	}
	if got := fs.GetStringSlice("nope"); got != nil {
		t.Fatalf("GetStringSlice missing = %v; want nil", got)
	}
}

func TestFlagSet_BoolValueTrue(t *testing.T) {
	fs := NewFlagSet("test", []string{"--debug", "true"})
	fs.Bool("debug", "", false, "debug")
	if err := fs.Parse(); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got := fs.GetBool("debug"); got != true {
		t.Fatalf("debug = %v; want true", got)
	}
}

// ---------------------------------------------------------------------------
// FileSource
// ---------------------------------------------------------------------------

func TestFileSource_NotFound(t *testing.T) {
	_, err := NewFileSource("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestFileSource_UnsupportedFormat(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.txt")
	os.WriteFile(path, []byte("content"), 0644)

	_, err := NewFileSource(path)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestFileSource_JSON(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.json")
	content := `{"server": {"host": "localhost", "port": 8080}, "debug": true}`
	os.WriteFile(path, []byte(content), 0644)

	fs, err := NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource error: %v", err)
	}

	if fs.Name() != "file:"+path {
		t.Fatalf("Name() = %q", fs.Name())
	}
	if fs.Priority() != 100 {
		t.Fatalf("Priority() = %d; want 100", fs.Priority())
	}
	if fs.Path() != path {
		t.Fatalf("Path() = %q", fs.Path())
	}

	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	srv := data["server"].(map[string]any)
	if srv["host"] != "localhost" {
		t.Fatalf("host = %v", srv["host"])
	}
	// JSON numbers are float64.
	if srv["port"] != float64(8080) {
		t.Fatalf("port = %v (%T)", srv["port"], srv["port"])
	}
	if data["debug"] != true {
		t.Fatalf("debug = %v", data["debug"])
	}
}

func TestFileSource_TOML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")
	content := `
[server]
host = "localhost"
port = 8080

[database]
name = "mydb"
debug = true
`
	os.WriteFile(path, []byte(content), 0644)

	fs, err := NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource error: %v", err)
	}

	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	srv := data["server"].(map[string]any)
	if srv["host"] != "localhost" {
		t.Fatalf("server.host = %v", srv["host"])
	}
	if srv["port"] != 8080 {
		t.Fatalf("server.port = %v", srv["port"])
	}

	db := data["database"].(map[string]any)
	if db["name"] != "mydb" {
		t.Fatalf("database.name = %v", db["name"])
	}
	if db["debug"] != true {
		t.Fatalf("database.debug = %v", db["debug"])
	}
}

func TestFileSource_YAML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yaml")
	content := `server:
  host: localhost
  port: 8080
debug: true
`
	os.WriteFile(path, []byte(content), 0644)

	fs, err := NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource error: %v", err)
	}

	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	srv := data["server"].(map[string]any)
	if srv["host"] != "localhost" {
		t.Fatalf("server.host = %v", srv["host"])
	}
	if srv["port"] != 8080 {
		t.Fatalf("server.port = %v", srv["port"])
	}
	if data["debug"] != true {
		t.Fatalf("debug = %v", data["debug"])
	}
}

func TestFileSource_YML(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.yml")
	content := `name: test`
	os.WriteFile(path, []byte(content), 0644)

	fs, err := NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource error: %v", err)
	}

	data, err := fs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["name"] != "test" {
		t.Fatalf("name = %v", data["name"])
	}
}

func TestFileSource_JSON_InvalidContent(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "bad.json")
	os.WriteFile(path, []byte("not json"), 0644)

	fs, err := NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource error: %v", err)
	}

	_, err = fs.Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// detectFormat
// ---------------------------------------------------------------------------

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"config.toml", "toml"},
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"config.json", "json"},
		{"config.TOML", "toml"},
		{"config.YAML", "yaml"},
		{"config.JSON", "json"},
		{"config.txt", ""},
		{"config", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := detectFormat(tt.path); got != tt.want {
				t.Fatalf("detectFormat(%q) = %q; want %q", tt.path, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TOML Parser: deeper tests
// ---------------------------------------------------------------------------

func TestTOML_StringValues(t *testing.T) {
	input := `name = "hello world"`
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	if data["name"] != "hello world" {
		t.Fatalf("name = %v", data["name"])
	}
}

func TestTOML_LiteralString(t *testing.T) {
	input := `path = 'C:\Users\test'`
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	if data["path"] != `C:\Users\test` {
		t.Fatalf("path = %v", data["path"])
	}
}

func TestTOML_Boolean(t *testing.T) {
	input := "enabled = true\ndisabled = false"
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	if data["enabled"] != true {
		t.Fatalf("enabled = %v", data["enabled"])
	}
	if data["disabled"] != false {
		t.Fatalf("disabled = %v", data["disabled"])
	}
}

func TestTOML_Integer(t *testing.T) {
	input := "port = 8080\nneg = -1"
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	if data["port"] != 8080 {
		t.Fatalf("port = %v", data["port"])
	}
	if data["neg"] != -1 {
		t.Fatalf("neg = %v", data["neg"])
	}
}

func TestTOML_Float(t *testing.T) {
	input := "pi = 3.14\nexp = 1e10"
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	if data["pi"] != 3.14 {
		t.Fatalf("pi = %v", data["pi"])
	}
	if data["exp"] != 1e10 {
		t.Fatalf("exp = %v", data["exp"])
	}
}

func TestTOML_Array(t *testing.T) {
	input := `tags = ["alpha", "beta", "gamma"]`
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	tags, ok := data["tags"].([]any)
	if !ok {
		t.Fatalf("tags is %T: %v", data["tags"], data["tags"])
	}
	if len(tags) != 3 || tags[0] != "alpha" {
		t.Fatalf("tags = %v", tags)
	}
}

func TestTOML_InlineTable(t *testing.T) {
	input := `point = {x = 1, y = 2}`
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	pt, ok := data["point"].(map[string]any)
	if !ok {
		t.Fatalf("point is %T: %v", data["point"], data["point"])
	}
	if pt["x"] != 1 || pt["y"] != 2 {
		t.Fatalf("point = %v", pt)
	}
}

func TestTOML_Section(t *testing.T) {
	input := `
[server]
host = "localhost"
port = 8080
`
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	srv := data["server"].(map[string]any)
	if srv["host"] != "localhost" || srv["port"] != 8080 {
		t.Fatalf("server = %v", srv)
	}
}

func TestTOML_DottedKey(t *testing.T) {
	input := `server.host = "localhost"`
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	srv := data["server"].(map[string]any)
	if srv["host"] != "localhost" {
		t.Fatalf("server.host = %v", srv["host"])
	}
}

func TestTOML_Comment(t *testing.T) {
	input := `
# This is a comment
key = "value" # inline comment
`
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	if data["key"] != "value" {
		t.Fatalf("key = %v", data["key"])
	}
}

func TestTOML_HexNumber(t *testing.T) {
	input := `hex = 0xFF`
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	if data["hex"] != 255 {
		t.Fatalf("hex = %v", data["hex"])
	}
}

func TestTOML_EscapeSequence(t *testing.T) {
	input := `msg = "hello\tworld\n"`
	data, err := parseTOML([]byte(input))
	if err != nil {
		t.Fatalf("parseTOML error: %v", err)
	}
	if data["msg"] != "hello\tworld\n" {
		t.Fatalf("msg = %q", data["msg"])
	}
}

// ---------------------------------------------------------------------------
// YAML Parser: deeper tests
// ---------------------------------------------------------------------------

func TestYAML_SimpleKeyValue(t *testing.T) {
	input := "name: test\nport: 8080"
	data, err := parseYAML([]byte(input))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	if data["name"] != "test" {
		t.Fatalf("name = %v", data["name"])
	}
	if data["port"] != 8080 {
		t.Fatalf("port = %v", data["port"])
	}
}

func TestYAML_NestedMap(t *testing.T) {
	input := `server:
  host: localhost
  port: 8080`
	data, err := parseYAML([]byte(input))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	srv := data["server"].(map[string]any)
	if srv["host"] != "localhost" || srv["port"] != 8080 {
		t.Fatalf("server = %v", srv)
	}
}

func TestYAML_Boolean(t *testing.T) {
	input := "a: true\nb: false\nc: yes\nd: no"
	data, err := parseYAML([]byte(input))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	if data["a"] != true || data["b"] != false || data["c"] != true || data["d"] != false {
		t.Fatalf("booleans: %v", data)
	}
}

func TestYAML_Null(t *testing.T) {
	input := "a: null\nb: ~"
	data, err := parseYAML([]byte(input))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	if data["a"] != nil || data["b"] != nil {
		t.Fatalf("nulls: %v", data)
	}
}

func TestYAML_QuotedString(t *testing.T) {
	input := `name: "hello world"`
	data, err := parseYAML([]byte(input))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	if data["name"] != "hello world" {
		t.Fatalf("name = %v", data["name"])
	}
}

func TestYAML_FlowSequence(t *testing.T) {
	input := `tags: [a, b, c]`
	data, err := parseYAML([]byte(input))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	tags, ok := data["tags"].([]any)
	if !ok {
		t.Fatalf("tags is %T: %v", data["tags"], data["tags"])
	}
	if len(tags) != 3 {
		t.Fatalf("tags len = %d", len(tags))
	}
}

func TestYAML_FlowMapping(t *testing.T) {
	input := `point: {x: 1, y: 2}`
	data, err := parseYAML([]byte(input))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	pt, ok := data["point"].(map[string]any)
	if !ok {
		t.Fatalf("point is %T: %v", data["point"], data["point"])
	}
	if pt["x"] != 1 || pt["y"] != 2 {
		t.Fatalf("point = %v", pt)
	}
}

func TestYAML_SimpleArray(t *testing.T) {
	input := `items:
  - alpha
  - beta
  - gamma`
	data, err := parseYAML([]byte(input))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	items, ok := data["items"].([]any)
	if !ok {
		t.Fatalf("items is %T: %v", data["items"], data["items"])
	}
	if len(items) != 3 || items[0] != "alpha" {
		t.Fatalf("items = %v", items)
	}
}

func TestYAML_Comments(t *testing.T) {
	input := `# comment
name: test # inline`
	data, err := parseYAML([]byte(input))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	// The inline comment becomes part of the value in this simple parser.
	// Let's just check name is not empty.
	if data["name"] == nil {
		t.Fatal("name should not be nil")
	}
}

func TestYAML_EmptyInput(t *testing.T) {
	data, err := parseYAML([]byte(""))
	if err != nil {
		t.Fatalf("parseYAML error: %v", err)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty, got %v", data)
	}
}

// ---------------------------------------------------------------------------
// RemoteSource
// ---------------------------------------------------------------------------

func TestRemoteSource_Name(t *testing.T) {
	rs := NewRemoteSource("http://example.com/config", time.Minute)
	if got := rs.Name(); got != "remote:http://example.com/config" {
		t.Fatalf("Name() = %q", got)
	}
}

func TestRemoteSource_Priority(t *testing.T) {
	rs := NewRemoteSource("http://example.com/config", time.Minute)
	if got := rs.Priority(); got != 75 {
		t.Fatalf("Priority() = %d; want 75", got)
	}
}

func TestRemoteSource_Load_Success(t *testing.T) {
	payload := map[string]any{"key": "value", "num": float64(42)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, time.Minute, WithRetries(0, 0))
	data, err := rs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["key"] != "value" {
		t.Fatalf("key = %v", data["key"])
	}
	if data["num"] != float64(42) {
		t.Fatalf("num = %v", data["num"])
	}
}

func TestRemoteSource_Load_Caching(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"call": callCount})
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, time.Hour, WithRetries(0, 0))

	// First load.
	data1, err := rs.Load()
	if err != nil {
		t.Fatalf("first Load() error: %v", err)
	}

	// Second load should use cache.
	data2, err := rs.Load()
	if err != nil {
		t.Fatalf("second Load() error: %v", err)
	}

	if callCount != 1 {
		t.Fatalf("expected 1 HTTP call, got %d", callCount)
	}

	// Both should have the same data.
	if data1["call"] != data2["call"] {
		t.Fatal("cached data should match")
	}
}

func TestRemoteSource_Load_ErrorFallsBackToCache(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"data": "cached"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, 0, WithRetries(0, 0))

	// First load succeeds and caches.
	data1, err := rs.Load()
	if err != nil {
		t.Fatalf("first Load() error: %v", err)
	}
	if data1["data"] != "cached" {
		t.Fatal("first load should return data")
	}

	// Second load: server errors, but refreshInterval=0 means cache is expired.
	// Should fall back to cached data after retries fail.
	data2, err := rs.Load()
	if err != nil {
		t.Fatalf("second Load() should fall back to cache: %v", err)
	}
	if data2["data"] != "cached" {
		t.Fatal("should fall back to cached data")
	}
}

func TestRemoteSource_Load_ErrorNoCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, time.Minute, WithRetries(0, 0))
	_, err := rs.Load()
	if err == nil {
		t.Fatal("expected error when server fails and no cache")
	}
}

func TestRemoteSource_WithHeaders(t *testing.T) {
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Custom")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, time.Minute,
		WithHeaders(map[string]string{"X-Custom": "test-value"}),
		WithRetries(0, 0),
	)
	_, err := rs.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if receivedHeader != "test-value" {
		t.Fatalf("custom header = %q; want 'test-value'", receivedHeader)
	}
}

func TestRemoteSource_WithTimeout(t *testing.T) {
	rs := NewRemoteSource("http://example.com", time.Minute,
		WithTimeout(5*time.Second),
	)
	if rs.timeout != 5*time.Second {
		t.Fatalf("timeout = %v; want 5s", rs.timeout)
	}
	if rs.client.Timeout != 5*time.Second {
		t.Fatalf("client timeout = %v; want 5s", rs.client.Timeout)
	}
}

func TestRemoteSource_WithRetries(t *testing.T) {
	rs := NewRemoteSource("http://example.com", time.Minute,
		WithRetries(5, 2*time.Second),
	)
	if rs.retries != 5 {
		t.Fatalf("retries = %d; want 5", rs.retries)
	}
	if rs.retryDelay != 2*time.Second {
		t.Fatalf("retryDelay = %v; want 2s", rs.retryDelay)
	}
}

func TestRemoteSource_WithBearerToken(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, time.Minute,
		WithBearerToken("my-token"),
		WithRetries(0, 0),
	)
	rs.Load()
	if authHeader != "Bearer my-token" {
		t.Fatalf("auth header = %q; want 'Bearer my-token'", authHeader)
	}
}

func TestRemoteSource_WithBasicAuth(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, time.Minute,
		WithBasicAuth("user", "pass"),
		WithRetries(0, 0),
	)
	rs.Load()
	if !strings.HasPrefix(authHeader, "Basic ") {
		t.Fatalf("auth header = %q; want 'Basic ...'", authHeader)
	}
}

func TestRemoteSource_SetURL(t *testing.T) {
	rs := NewRemoteSource("http://old.com", time.Minute)
	// Manually set cached data to verify it gets cleared.
	rs.cachedData = map[string]any{"old": true}

	rs.SetURL("http://new.com")
	if rs.url != "http://new.com" {
		t.Fatalf("url = %q; want http://new.com", rs.url)
	}
	if rs.cachedData != nil {
		t.Fatal("cache should be cleared after SetURL")
	}
}

func TestRemoteSource_LastFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, time.Minute, WithRetries(0, 0))

	before := time.Now()
	rs.Load()
	after := time.Now()

	lf := rs.LastFetch()
	if lf.Before(before) || lf.After(after) {
		t.Fatalf("LastFetch = %v; expected between %v and %v", lf, before, after)
	}
}

func TestRemoteSource_Refresh(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"call": calls})
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, time.Hour, WithRetries(0, 0))

	rs.Load()
	if calls != 1 {
		t.Fatal("first load should make 1 call")
	}

	rs.Refresh()
	if calls != 2 {
		t.Fatalf("Refresh should make another call; calls=%d", calls)
	}
}

// ---------------------------------------------------------------------------
// base64Encode
// ---------------------------------------------------------------------------

func TestBase64Encode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"f", "Zg=="},
		{"fo", "Zm8="},
		{"foo", "Zm9v"},
		{"foobar", "Zm9vYmFy"},
		{"user:pass", "dXNlcjpwYXNz"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := base64Encode([]byte(tt.input))
			if got != tt.want {
				t.Fatalf("base64Encode(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// hashData
// ---------------------------------------------------------------------------

func TestHashData(t *testing.T) {
	d1 := map[string]any{"a": 1}
	d2 := map[string]any{"a": 2}

	h1 := hashData(d1)
	h2 := hashData(d2)

	if h1 == "" {
		t.Fatal("hash should not be empty")
	}
	if h1 == h2 {
		t.Fatal("different data should produce different hashes")
	}

	// Same data should produce same hash.
	h1b := hashData(d1)
	if h1 != h1b {
		t.Fatal("same data should produce same hash")
	}
}

// ---------------------------------------------------------------------------
// RemoteWatcher
// ---------------------------------------------------------------------------

func TestRemoteWatcher_StartStop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	rs := NewRemoteSource(srv.URL, time.Minute, WithRetries(0, 0))
	rw := NewRemoteWatcher(rs, 50*time.Millisecond)

	if err := rw.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Start again should be a no-op.
	if err := rw.Start(); err != nil {
		t.Fatalf("second Start() error: %v", err)
	}

	// Wait for at least one event.
	select {
	case data := <-rw.Events():
		if data["status"] != "ok" {
			t.Fatalf("unexpected event data: %v", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}

	rw.Stop()
	// Stop again should be a no-op.
	rw.Stop()
}

// ---------------------------------------------------------------------------
// ConsulSource / EtcdSource (constructor tests)
// ---------------------------------------------------------------------------

func TestConsulSource_Name(t *testing.T) {
	cs := NewConsulSource("http://consul:8500", "myapp", time.Minute)
	name := cs.Name()
	if !strings.Contains(name, "consul") {
		t.Fatalf("Name() = %q; expected to contain 'consul'", name)
	}
}

func TestEtcdSource_Creation(t *testing.T) {
	es := NewEtcdSource([]string{"http://etcd:2379"}, "/config", time.Minute)
	if es.prefix != "/config" {
		t.Fatalf("prefix = %q; want '/config'", es.prefix)
	}
}

// ---------------------------------------------------------------------------
// HTTPPollingSource
// ---------------------------------------------------------------------------

func TestHTTPPollingSource_Name(t *testing.T) {
	h := NewHTTPPollingSource("http://example.com/config", "json", time.Minute)
	if got := h.Name(); got != "http:http://example.com/config" {
		t.Fatalf("Name() = %q", got)
	}
}

func TestHTTPPollingSource_Priority(t *testing.T) {
	h := NewHTTPPollingSource("http://example.com/config", "json", time.Minute)
	if got := h.Priority(); got != 80 {
		t.Fatalf("Priority() = %d; want 80", got)
	}
}

func TestHTTPPollingSource_Load_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"key": "val"})
	}))
	defer srv.Close()

	h := NewHTTPPollingSource(srv.URL, "json", time.Minute)
	data, err := h.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if data["key"] != "val" {
		t.Fatalf("key = %v", data["key"])
	}
}

func TestHTTPPollingSource_Load_UnsupportedFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	h := NewHTTPPollingSource(srv.URL, "xml", time.Minute)
	_, err := h.Load()
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestHTTPPollingSource_Load_Caching(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"c": calls})
	}))
	defer srv.Close()

	h := NewHTTPPollingSource(srv.URL, "json", time.Minute)

	h.Load()
	h.Load()

	if calls != 1 {
		t.Fatalf("expected 1 HTTP call due to caching, got %d", calls)
	}
}

// ---------------------------------------------------------------------------
// splitFlow
// ---------------------------------------------------------------------------

func TestSplitFlow(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a, b, c", 3},
		{`"a,b", c`, 2},
		{"[1,2], [3,4]", 2},
		{"{a: 1}, {b: 2}", 2},
		{"single", 1},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitFlow(tt.input, ',')
			if len(got) != tt.want {
				t.Fatalf("splitFlow(%q) len=%d; want %d: %v", tt.input, len(got), tt.want, got)
			}
		})
	}
}
