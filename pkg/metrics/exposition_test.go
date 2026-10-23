// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package metrics

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"unheaded/pkg/metrics/metricstest"
)

func gatherString(t *testing.T, r *Registry) string {
	t.Helper()
	var buf bytes.Buffer
	if err := r.Gather(&buf); err != nil {
		t.Fatalf("Gather: %v", err)
	}
	return buf.String()
}

// A newline in help text ended the HELP line early; the rest of the help
// became a "sample" line, and a parser rejects the whole page. The text
// format requires \ and newline escaped in HELP, as client_golang does.
func TestHelpIsEscaped(t *testing.T) {
	r := NewRegistry()
	c := NewCounter("help_escape_total", "first line\nsecond line, a \\ backslash", nil)
	c.Inc()
	r.MustRegister(c)

	out := gatherString(t, r)
	want := `# HELP help_escape_total first line\nsecond line, a \\ backslash` + "\n"
	if !strings.Contains(out, want) {
		t.Fatalf("HELP not escaped; want line %q in:\n%s", want, out)
	}
	if problems := metricstest.Lint(out); problems != nil {
		t.Fatalf("exposition does not lint: %v\n%s", problems, out)
	}
}

// Prometheus rejects a scrape containing a label value that is not valid
// UTF-8 — every series on the page, not just the bad one.
func TestLabelValue_InvalidUTF8DoesNotPoisonThePage(t *testing.T) {
	r := NewRegistry()
	gv := NewGaugeVec("utf8_gauge", "h", nil, []string{"v"})
	r.MustRegister(gv)
	gv.WithLabelValues("ok\xff\xfeend").Set(1)

	out := gatherString(t, r)
	if !utf8.ValidString(out) {
		t.Fatalf("exposition is not valid UTF-8:\n%q", out)
	}
	if !strings.Contains(out, `utf8_gauge{v="ok`+"�"+`end"} 1`) &&
		!strings.Contains(out, `utf8_gauge{v="ok`+"��"+`end"} 1`) {
		t.Fatalf("invalid bytes not replaced:\n%q", out)
	}
}

// Labels.String is the series key. Unquoted, {a="1,b=2", b="3"} and
// {a="1", b="2,b=3"} both keyed as a=1,b=2,b=3: the second call got the
// first series back and its increments were published under the first's
// labels.
func TestVec_LabelValuesContainingSeparatorsAreDistinctSeries(t *testing.T) {
	r := NewRegistry()
	cv := NewCounterVec("sep_total", "h", nil, []string{"a", "b"})
	r.MustRegister(cv)

	cv.WithLabels(Labels{"a": "1,b=2", "b": "3"}).Inc()
	cv.WithLabels(Labels{"a": "1", "b": "2,b=3"}).Add(5)

	out := gatherString(t, r)
	for _, want := range []string{
		`sep_total{a="1,b=2",b="3"} 1`,
		`sep_total{a="1",b="2,b=3"} 5`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
