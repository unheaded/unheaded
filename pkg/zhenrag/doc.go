// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

// Package zhenrag owns the text every Zhen chat surface sends to the model
// and the destructive-verb list both the prompts and the Champion gate use
// (S-CHAMPION-HARDENING Phase 4, M-123, RT-07/RT-08).
//
// Before this package the system prompt existed four times (cmd/zhen-rag,
// cmd/zhen-cli, pkg/agent, raft/scripts/zhen_rag.py) and the destructive verbs
// twice (the prompts' prose and pkg/champion's regexp), each copy edited by
// hand. Now:
//
//   - SystemPrompt(mode) renders the prompt for Chat (cmd/zhen-rag,
//     cmd/zhen-cli), Web (raft/scripts/zhen_rag.py, the web UI's direct path
//     and the MCP/CLI users of it) and Agent (pkg/agent, before its
//     "References:" block).
//   - Verbs is the one destructive-verb table. The prompts' prose lists and
//     DestructivePattern (the Champion's Rule 3) are both rendered from it,
//     so a verb cannot be added to one and forgotten in the other.
//   - CodeCommentReminder and CodeCommentExample are the per-turn reminder
//     and the worked exchange cmd/zhen-rag and the web UI send.
//   - Python gets the same values as literals in a generated module,
//     raft/scripts/zhen_prompt_generated.py; TestGenerated_PythonInSync fails
//     when the committed file is stale.
//
// The consolidation moved the text, it did not change it: the goldens under
// testdata/prompts are each surface's prompt captured before the move, and
// the tests assert byte equality. A change to any prompt string is a prompt
// change and needs `make coding-gate-ci` (memory: project_zhenai_coding_gate).
//
// Regenerate the Python module after editing anything here:
//
//	go generate ./pkg/zhenrag/...
//
// Print a rendered prompt (Appendix A1 of the plan):
//
//	go run ./pkg/zhenrag/gen -dump chat|web|agent
package zhenrag

//go:generate go run ./gen -out ../../raft/scripts/zhen_prompt_generated.py
