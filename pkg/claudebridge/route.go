// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import "context"

// Weight is ADR-053's task class: HEAVY work goes to Claude, MINOR churn
// to local Zhenai.
type Weight string

// Task classes from ADR-053.
const (
	WeightHeavy Weight = "heavy" // synthesis, audit, novel ADRs, multi-skill review
	WeightMinor Weight = "minor" // template fill, lookup, status, runbook execution
)

// Task is what a Router classifies.
type Task struct {
	Prompt string
	// Hints are caller-supplied labels (a workflow template name, a
	// Kanban card type) a router may use instead of reading the prompt.
	Hints map[string]string
}

// Router is the ADR-053 routing hook: it decides whether a task goes to
// Claude (WeightHeavy) or to local Zhenai (WeightMinor). Interface only;
// no implementation ships with ADR-104. A router must not send a task
// to Claude that holds data the operator has not cleared for an
// external API (zero user data access).
type Router interface {
	Route(ctx context.Context, t Task) (Weight, error)
}
