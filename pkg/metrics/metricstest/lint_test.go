// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metricstest

import (
	"bytes"
	"strings"
	"testing"

	"unheaded/pkg/metrics"
)

// The suffix rule must not become a loophole.
func TestSampleDeclared(t *testing.T) {
	d := map[string]string{"s": "summary", "h": "histogram", "c": "counter"}
	for sample, want := range map[string]bool{
		"s": true, "s_sum": true, "s_count": true, "s_bucket": false,
		"h_bucket": true, "h_sum": true, "h_count": true,
		"c": true, "c_sum": false, "c_total": false, "x": false, "x_sum": false,
	} {
		if got := SampleDeclared(d, sample); got != want {
			t.Errorf("SampleDeclared(%q) = %v, want %v", sample, got, want)
		}
	}
}

func TestLint_FindsEachProblem(t *testing.T) {
	for body, want := range map[string]string{
		"x 1\n": "sample x has no preceding # TYPE",
		"# TYPE x counter\n# TYPE x counter\nx 1\n":     "# TYPE for x appears twice",
		"# HELP x a\n# HELP x b\n# TYPE x gauge\nx 1\n": "# HELP for x appears twice",
		"# TYPE x countr\nx 1\n":                        `bad TYPE line`,
		"# TYPE s summary\ns_bucket 1\n":                "sample s_bucket has no preceding # TYPE",
	} {
		got := strings.Join(Lint(body), "; ")
		if !strings.Contains(got, want) {
			t.Errorf("Lint(%q) = %q, want it to mention %q", body, got, want)
		}
	}
}

// What pkg/metrics serves must lint clean, including the go_* summary and
// process_* families the default registry carries.
func TestLint_DefaultRegistryIsClean(t *testing.T) {
	var buf bytes.Buffer
	if err := metrics.DefaultRegistry.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	if p := Lint(buf.String()); p != nil {
		t.Errorf("default registry does not lint clean: %v", p)
	}
	if !strings.Contains(buf.String(), "go_gc_duration_seconds_sum") {
		t.Error("expected a summary in the default registry; the test is not exercising the suffix rule")
	}
}
