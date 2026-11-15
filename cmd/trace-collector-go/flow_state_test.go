// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"unheaded/pkg/metrics"
)

// dropFlowStateFailures stops exporting the gauge's one series.
func dropFlowStateFailures() {
	flowStateInsertFailures.DeletePartialMatch(metrics.Labels{"program": "packet_marker"})
}

// D6 = A: FLOW_STATS keys 7 and 8 (FLOW_EVENTS sent and dropped) are
// retired with the ring; traces.stats no longer names them.
func TestFlowStatsHasNoEventFields(t *testing.T) {
	m := NewMockMapReader()
	putStat(m, 7, 2340)
	putStat(m, 8, 50101)
	putStat(m, 2, 9)
	stats, err := collectFlowStats(m)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"events_sent", "events_dropped"} {
		if strings.Contains(string(raw), gone) {
			t.Errorf("traces.stats still carries %s: %s", gone, raw)
		}
	}
	if stats.FlowsNew != 9 {
		t.Errorf("flows_new = %d, want 9", stats.FlowsNew)
	}
}

// packetMarkerLoader is a stats source that also has packet_marker's STATS.
type packetMarkerLoader struct {
	noStatsMap
	pm MapReader
}

func (l packetMarkerLoader) GetPacketMarkerStatsMap() MapReader { return l.pm }

// D7 = A: a failed FLOW_STATE insert is counted in packet_marker's STATS key
// 8, and the collector exports it each stats tick. A loader without the map
// reports nothing, never a 0 that would read as "no failures".
func TestFlowStateFailuresGauge(t *testing.T) {
	dropFlowStateFailures()
	defer dropFlowStateFailures()
	m := NewMockMapReader()
	putStat(m, 8, 3)
	putStat(m, 0, 1000)
	observeFlowStateFailures(packetMarkerLoader{pm: m})
	if got := flowStateInsertFailures.WithLabelValues("packet_marker").Value(); got != 3 {
		t.Errorf("trace_collector_flow_state_insert_failures = %v, want 3", got)
	}

	dropFlowStateFailures()
	observeFlowStateFailures(noStatsMap{})
	observeFlowStateFailures(packetMarkerLoader{})
	var buf bytes.Buffer
	if err := flowStateInsertFailures.Write(&buf); err != nil || buf.Len() != 0 {
		t.Errorf("a loader without packet_marker's STATS exported %q (%v), want nothing", buf.String(), err)
	}
}
