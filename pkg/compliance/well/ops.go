// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package well

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/database"
)

// OpsStore is compliance records in unheaded_ops: the apply journal for
// now; findings and evidence follow (COMPLIANCE-WELL steps 26-28).
type OpsStore struct {
	db *sql.DB
}

// NewOpsStore is the store over an unheaded_ops connection.
func NewOpsStore(db *sql.DB) *OpsStore { return &OpsStore{db: db} }

// OpsConfigFromEnv is the unheaded_ops connection: read-only ops_reader by
// default; the ingesting collector sets WELL_OPS_USER=ops_writer. There is
// no default password (FND-007).
func OpsConfigFromEnv() database.Config {
	c := ConfigFromEnv()
	c.User = "ops_reader"
	if v, ok := os.LookupEnv("WELL_OPS_USER"); ok {
		c.User = v
	}
	c.Password = os.Getenv("WELL_OPS_PASSWORD")
	c.DBName = "unheaded_ops"
	if v, ok := os.LookupEnv("WELL_OPS_DB"); ok {
		c.DBName = v
	}
	return c
}

// IngestJournal takes a host's whole journal file (seq 1 onwards) and
// appends what The Well does not hold yet. The whole file must chain from
// genesis, every entry The Well already holds must match it hash for hash,
// and the file may not be shorter than what is held: a rewritten or
// truncated journal is ErrChainBroken, never merged. Concurrent ingests of
// one host serialise on an advisory lock (ops_writer has no UPDATE right,
// so no row lock). Returns how many entries were new.
func (s *OpsStore) IngestJournal(ctx context.Context, host string, entries []remediation.JournalEntry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	if entries[0].Host != host || entries[0].Seq != 1 {
		return 0, fmt.Errorf("%w: journal for %s must start at seq 1 of that host", remediation.ErrChainBroken, host)
	}
	if err := remediation.VerifyChain(entries, remediation.GenesisHash); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	n, err := ingest(ctx, tx, host, entries)
	if err != nil {
		return 0, errors.Join(err, tx.Rollback())
	}
	return n, tx.Commit()
}

func ingest(ctx context.Context, tx *sql.Tx, host string, entries []remediation.JournalEntry) (int, error) {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('compliance_journal:' || $1))`, host); err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT host_seq, hash FROM compliance_apply_journal WHERE host = $1 ORDER BY host_seq`, host)
	if err != nil {
		return 0, err
	}
	held := map[int64]string{}
	var last int64
	for rows.Next() {
		var seq int64
		var hash string
		if err := rows.Scan(&seq, &hash); err != nil {
			return 0, errors.Join(err, rows.Close())
		}
		held[seq] = hash
		last = seq
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, err
	}
	if int64(len(entries)) < last {
		return 0, fmt.Errorf("%w: The Well holds %s up to seq %d, the host's journal ends at %d (truncated)",
			remediation.ErrChainBroken, host, last, len(entries))
	}
	for _, e := range entries[:last] {
		if held[e.Seq] != e.Hash {
			return 0, fmt.Errorf("%w: %s seq %d differs from what The Well ingested (rewritten)",
				remediation.ErrChainBroken, host, e.Seq)
		}
	}
	added := 0
	for _, e := range entries[last:] {
		if _, err := tx.ExecContext(ctx, `INSERT INTO compliance_apply_journal
			(host, host_seq, fix_id, fix_sha256, model_commit, action, outcome, backup_dir, operator, backfilled, at, previous_hash, hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			e.Host, e.Seq, e.FixID, e.FixSHA256, e.ModelCommit, e.Action, e.Outcome, e.BackupDir, e.Operator,
			e.Backfilled, e.At, e.PreviousHash, e.Hash); err != nil {
			return 0, fmt.Errorf("journal %s seq %d: %w", host, e.Seq, err)
		}
		added++
	}
	return added, nil
}
