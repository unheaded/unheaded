// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package crosswalk

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEvidenceRoundTripAndMerge(t *testing.T) {
	a := Source{Kind: KindGateScript, Ref: "scripts/a.sh"}
	old := []Record{
		{Source: a, Verdict: VerdictPass, ObservedAt: now.Add(-3 * time.Hour)},
		{Source: a, Verdict: VerdictFail, ObservedAt: now.Add(-2 * time.Hour)},
	}
	fresh := []Record{
		{Source: a, Verdict: VerdictPass, ObservedAt: now.Add(-time.Hour)},
		{Source: a, Verdict: VerdictFail, ObservedAt: now.Add(-2 * time.Hour)}, // duplicate of old[1]
	}
	merged := MergeRecords(old, fresh, 2)
	if len(merged) != 2 {
		t.Fatalf("merged %d records, want 2 (dedupe, then keep the 2 newest)", len(merged))
	}
	if !merged[0].ObservedAt.Equal(now.Add(-time.Hour)) || !merged[1].ObservedAt.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("kept %v, %v", merged[0].ObservedAt, merged[1].ObservedAt)
	}

	var buf bytes.Buffer
	if err := WriteEvidence(&buf, merged); err != nil {
		t.Fatal(err)
	}
	back, err := ReadEvidence(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(back) != 2 || back[0].Source != a {
		t.Fatalf("round trip = %+v", back)
	}
}

func TestReadEvidence_Rejects(t *testing.T) {
	for name, in := range map[string]string{
		"not json":      "nope",
		"wrong version": `{"version": 2, "records": []}`,
		"oversized":     `{"version":1,"records":[` + strings.Repeat(" ", MaxEvidenceBytes) + `]}`,
		"unknown field": `{"version":1,"records":[],"extra":1}`,
	} {
		if _, err := ReadEvidence(strings.NewReader(in)); !errors.Is(err, ErrInvalidEvidence) {
			t.Errorf("%s: err = %v, want ErrInvalidEvidence", name, err)
		}
	}
}
