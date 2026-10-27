// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ScheduleUpdater applies a change to zhen-agentd's runbook schedule
// (cmd/zhen-agentd/scheduler.go): op is add, remove or pause; args are the
// tool call's args. The scheduler validates them (an approved runbook, a
// known interval) and returns what it now holds.
type ScheduleUpdater interface {
	UpdateSchedule(ctx context.Context, op string, args map[string]any) (any, error)
}

// ErrNoScheduler is schedule_update on a Champion without a scheduler
// (zhen-agentd started without -scheduler).
var ErrNoScheduler = errors.New("no scheduler configured (zhen-agentd runs it with -scheduler)")

// WithScheduleUpdater sets the scheduler schedule_update changes.
func (c *Champion) WithScheduleUpdater(u ScheduleUpdater) *Champion {
	c.scheduler = u
	return c
}

// ScheduleUpdate is schedule_update after the gate: one mutation row in
// the action log (refused when it cannot be written), then the change.
func (c *Champion) ScheduleUpdate(ctx context.Context, op string, args map[string]any) (any, error) {
	start := time.Now()
	actionID, auditErr := c.logMutation(ctx, "schedule.update", fmt.Sprintf("Schedule %s: %s", op, summarizeArgs(args)), "user")
	if auditErr != nil {
		return nil, auditErr
	}
	if c.scheduler == nil {
		c.completeAction(ctx, actionID, "failed", "", ErrNoScheduler.Error(), time.Since(start))
		return nil, ErrNoScheduler
	}
	out, err := c.scheduler.UpdateSchedule(ctx, op, args)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return nil, fmt.Errorf("schedule %s: %w", op, err)
	}
	c.completeAction(ctx, actionID, "completed", fmt.Sprintf("schedule %s %s", op, summarizeArgs(args)), "", time.Since(start))
	return out, nil
}
