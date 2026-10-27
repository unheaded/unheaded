// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package agent

import (
	"flag"
	"os"
	"testing"

	"unheaded/pkg/champion"
)

// updateGolden rewrites the golden file from the current code. Use it only
// for an intended prompt change, and run the coding gate afterwards
// (memory: project_zhenai_coding_gate).
var updateGolden = flag.Bool("update-golden", false, "rewrite testdata prompt goldens")

// goldenAgentPrompt is buildSystemPrompt's output captured before the prompt
// moved into pkg/zhenrag (S-CHAMPION-HARDENING Phase 4). The move changes
// where the text lives, not what the model reads: any byte difference here is
// a prompt change and needs the coding gate.
const goldenAgentPrompt = "../zhenrag/testdata/prompts/agent.txt"

func TestBuildSystemPrompt_ByteIdenticalToGolden(t *testing.T) {
	contents := []TopicContent{
		{Ref: champion.Reference{Topic: "bash", Category: "lang", SourceTrust: "canonical"},
			Content: "short canonical body"},
		{Ref: champion.Reference{Topic: "notes", Category: "user", SourceTrust: "external", SourceLabel: "my-src"},
			Content: "0123456789abcdefghij-truncated-tail"},
		{Ref: champion.Reference{Topic: "legacy", Category: "old"},
			Content: "no trust label: defaults to canonical"},
	}
	got := buildSystemPrompt(contents, 20)
	if *updateGolden {
		if err := os.WriteFile(goldenAgentPrompt, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenAgentPrompt)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("agent system prompt drifted from %s (%d vs %d bytes); a prompt change needs the coding gate",
			goldenAgentPrompt, len(got), len(want))
	}
}
