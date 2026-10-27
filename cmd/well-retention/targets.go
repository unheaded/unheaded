// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import "slices"

const (
	dbApp = "unheaded_app"
	dbOps = "unheaded_ops"
)

// target is everything well-retention may do to one table, as fixed SQL.
// classes.yaml picks a target and an age; it never supplies SQL or a
// name that reaches a query. Each statement here matches a grant in
// migrations 042/043, and nothing else is granted.
//
// Placeholders: $1 the cutoff (rows older than it are eligible), $2 the
// batch size.
type target struct {
	db      string
	actions []string
	// fixedDays: the database enforces this age itself (fixedBy says
	// where); the class must agree.
	fixedDays int
	fixedBy   string

	// purge, archive-purge, purge-function: rows eligible now.
	count string
	// archive-purge: rows past the cutoff the purge keeps (held).
	held string
	// purge: one batch.
	purge string
	// archive-purge: one batch, returning (key, row as JSON).
	archive string
	// purge-function: returns the number of rows deleted.
	purgeFn string

	// partition-check: the oldest value of the table's age column.
	oldest string
}

func (t target) allows(action string) bool { return slices.Contains(t.actions, action) }

var targets = map[string]target{
	// Operator-typed text; no archive (policy §3.9).
	"zhen_conversations": {
		db:      dbApp,
		actions: []string{actionPurge},
		count:   `SELECT count(*) FROM zhen_conversations WHERE created_at < $1`,
		purge: `DELETE FROM zhen_conversations WHERE id IN (
		            SELECT id FROM zhen_conversations WHERE created_at < $1 ORDER BY created_at LIMIT $2)`,
	},
	// 033: purge_after = captured_at + 180 days, and zhen_captures_purge()
	// is the only delete path (quarantined captures go at once; a promoted
	// one stays while an export names it). The cutoff is informational.
	"zhen_captures": {
		db:        dbApp,
		actions:   []string{actionPurgeFunction},
		fixedDays: 180,
		fixedBy:   "033's insert trigger sets purge_after",
		count:     `SELECT count(*) FROM zhen_captures WHERE quarantined OR (NOT promoted AND purge_after <= now())`,
		purgeFn:   `SELECT zhen_captures_purge()`,
	},
	// Soft-deleted cards (ADR-011 Q4 amendment). A card any agent run names
	// is held: runs are never deleted (032) and its foreign key would
	// refuse the delete anyway. Archived cards (archived_at) are not
	// deleted cards.
	"kanban_tasks": {
		db:      dbApp,
		actions: []string{actionArchivePurge},
		count: `SELECT count(*) FROM kanban_tasks k
		         WHERE k.deleted_at < $1
		           AND NOT EXISTS (SELECT 1 FROM zhen_agent_runs r WHERE r.card_id = k.id)`,
		held: `SELECT count(*) FROM kanban_tasks k
		        WHERE k.deleted_at < $1
		          AND EXISTS (SELECT 1 FROM zhen_agent_runs r WHERE r.card_id = k.id)`,
		archive: `DELETE FROM kanban_tasks t WHERE t.id IN (
		              SELECT k.id FROM kanban_tasks k
		               WHERE k.deleted_at < $1
		                 AND NOT EXISTS (SELECT 1 FROM zhen_agent_runs r WHERE r.card_id = k.id)
		               ORDER BY k.deleted_at LIMIT $2)
		          RETURNING t.id, to_jsonb(t)`,
	},
	// The archive's own age (policy §3.4: one year).
	"retention_archive": {
		db:      dbOps,
		actions: []string{actionPurge},
		count:   `SELECT count(*) FROM retention_archive WHERE archived_at < $1`,
		purge: `DELETE FROM retention_archive WHERE id IN (
		            SELECT id FROM retention_archive WHERE archived_at < $1 ORDER BY archived_at LIMIT $2)`,
	},

	// Append-only history: rows are never deleted (015, 030, 032, 039);
	// retention for these is time partitions, detached and archived, once
	// ERDA 5.4's trigger fires. The job measures the trigger.
	"llm_requests": {
		db: dbOps, actions: []string{actionPartitionCheck},
		oldest: `SELECT min(occurred_at) FROM llm_requests`,
	},
	"service_health_transitions": {
		db: dbOps, actions: []string{actionPartitionCheck},
		oldest: `SELECT min(transition_at) FROM service_health_transitions`,
	},
	"audit_events": {
		db: dbOps, actions: []string{actionPartitionCheck},
		oldest: `SELECT min(occurred_at) FROM audit_events`,
	},
	"compliance_findings_history": {
		db: dbOps, actions: []string{actionPartitionCheck},
		oldest: `SELECT min(changed_at) FROM compliance_findings_history`,
	},
	"deploy_history_transitions": {
		db: dbOps, actions: []string{actionPartitionCheck},
		oldest: `SELECT min(occurred_at) FROM deploy_history_transitions`,
	},
	"chaos_journal": {
		db: dbOps, actions: []string{actionPartitionCheck},
		oldest: `SELECT min(created_at) FROM chaos_journal`,
	},
	"zhen_agent_runs": {
		db: dbApp, actions: []string{actionPartitionCheck},
		oldest: `SELECT min(claimed_at) FROM zhen_agent_runs`,
	},
}

// sizeQuery reads a table's estimated live rows and total size; neither
// needs a grant on the table. $1 is the table name (from targets only).
const sizeQuery = `SELECT coalesce(s.n_live_tup, 0), pg_total_relation_size(c.oid)
  FROM pg_class c LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
 WHERE c.oid = to_regclass('public.' || $1)`
