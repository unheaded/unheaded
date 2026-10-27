// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package logagg

import (
	"bytes"
	"strings"
	"testing"

	"unheaded/pkg/metrics"
)

// goldenExposition is what the hand-rolled Publisher.WriteMetrics produced
// for dropped=7, published=42, failed=3 on service "golden-svc", captured
// before it was deleted (S-WELL-SPRING Step 188). The five services that
// used it now serve the registry view; a scraper must see the same bytes.
const goldenExposition = `# HELP unheaded_logagg_entries_dropped_total Log entries discarded because the logagg publish queue was full
# TYPE unheaded_logagg_entries_dropped_total counter
unheaded_logagg_entries_dropped_total{service="golden-svc"} 7
# HELP unheaded_logagg_entries_published_total Log entries successfully published to Wotan by logagg
# TYPE unheaded_logagg_entries_published_total counter
unheaded_logagg_entries_published_total{service="golden-svc"} 42
# HELP unheaded_logagg_publish_errors_total logagg publish attempts the transport rejected
# TYPE unheaded_logagg_publish_errors_total counter
unheaded_logagg_publish_errors_total{service="golden-svc"} 3
`

func goldenPublisher(t *testing.T) *Publisher {
	t.Helper()
	p := NewPublisher("golden-svc", nil)
	t.Cleanup(p.Close)
	p.dropped.Store(7)
	p.published.Store(42)
	p.failed.Store(3)
	return p
}

// The registry view renders exactly what the hand-rolled writer did.
func TestPublisher_CollectorsExpositionMatchesGolden(t *testing.T) {
	p := goldenPublisher(t)
	reg := metrics.NewRegistry()
	if err := p.RegisterCollectors(reg); err != nil {
		t.Fatalf("RegisterCollectors: %v", err)
	}
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != goldenExposition {
		t.Errorf("registry exposition differs from the WriteMetrics golden\ngot:\n%s\nwant:\n%s", buf.String(), goldenExposition)
	}
}

// RegisterCollectors puts all three counters in the registry; a second
// registration of the same service fails and adds nothing; Unregister
// takes them out again; a nil registry is a no-op both ways.
func TestPublisher_RegisterAndUnregisterCollectors(t *testing.T) {
	p := goldenPublisher(t)
	reg := metrics.NewRegistry()
	if err := p.RegisterCollectors(reg); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := p.RegisterCollectors(reg); err == nil {
		t.Fatal("second register of the same service succeeded, want a conflict")
	}
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "unheaded_logagg_entries_dropped_total{"); n != 1 {
		t.Fatalf("dropped sample appears %d times after a failed re-register, want 1", n)
	}

	p.UnregisterCollectors(reg)
	buf.Reset()
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "unheaded_logagg_") {
		t.Fatalf("collectors still served after Unregister:\n%s", buf.String())
	}

	if err := p.RegisterCollectors(nil); err != nil {
		t.Fatalf("nil registry: %v", err)
	}
	p.UnregisterCollectors(nil)
}

// A partial conflict registers nothing: a service never serves two of the
// three counters.
func TestPublisher_RegisterCollectorsAllOrNone(t *testing.T) {
	p := goldenPublisher(t)
	reg := metrics.NewRegistry()
	// Occupy the third name with an incompatible family.
	reg.MustRegister(metrics.NewGauge("unheaded_logagg_publish_errors_total", "something else", nil))
	if err := p.RegisterCollectors(reg); err == nil {
		t.Fatal("register over a conflicting family succeeded")
	}
	var buf bytes.Buffer
	if err := reg.Gather(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "unheaded_logagg_entries_") {
		t.Fatalf("partial registration left behind:\n%s", buf.String())
	}
}
