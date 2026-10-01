// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package well

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"unheaded/pkg/compliance/crosswalk"
	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/database"
)

// OpsStore is compliance records in unheaded_ops: the apply journal,
// findings triage and evidence.
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

// --- findings triage ---------------------------------------------------------

// Register reads the findings triage and validates it exactly as the YAML
// register is validated (crosswalk.NewRegister against the catalog).
func (s *OpsStore) Register(ctx context.Context, cat *crosswalk.Catalog) (*crosswalk.Register, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT entry FROM compliance_findings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("findings: %w", err)
	}
	defer rows.Close()
	var entries []*crosswalk.RegisterEntry
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("findings: %w", err)
		}
		e, err := decodeEntry(raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("findings: %w", err)
	}
	return crosswalk.NewRegister(entries, cat)
}

func decodeEntry(raw []byte) (*crosswalk.RegisterEntry, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var e crosswalk.RegisterEntry
	if err := dec.Decode(&e); err != nil {
		return nil, fmt.Errorf("%w: finding entry: %v", ErrInvalidChange, err)
	}
	return &e, nil
}

// UpsertFinding writes one entry's triage, validated against the catalog
// first; the table's trigger records who changed it and why.
func (s *OpsStore) UpsertFinding(ctx context.Context, cat *crosswalk.Catalog, e *crosswalk.RegisterEntry, actor, reason string) error {
	if err := checkWho(actor, reason); err != nil {
		return err
	}
	if _, err := crosswalk.NewRegister([]*crosswalk.RegisterEntry{e}, cat); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidChange, err)
	}
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	var on, until any
	if a := e.Accepted; a != nil {
		on, until = a.AcceptedOn, a.Until
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO compliance_findings
		(key, severity, class, host, step, lockout, opened, accepted_on, accepted_until, entry, updated_by, change_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (key) DO UPDATE SET severity = EXCLUDED.severity, class = EXCLUDED.class, host = EXCLUDED.host,
			step = EXCLUDED.step, lockout = EXCLUDED.lockout, opened = EXCLUDED.opened,
			accepted_on = EXCLUDED.accepted_on, accepted_until = EXCLUDED.accepted_until, entry = EXCLUDED.entry,
			updated_by = EXCLUDED.updated_by, change_reason = EXCLUDED.change_reason`,
		e.Key(), string(e.Severity), e.Class, e.Host, e.Step, e.Lockout, e.Opened, on, until, body, actor, reason)
	if err != nil {
		return fmt.Errorf("finding %s: %w", e.Key(), err)
	}
	return nil
}

// ResolveFinding removes a finding whose evidence passes; the trigger
// keeps the removed entry with who and why.
func (s *OpsStore) ResolveFinding(ctx context.Context, key, actor, reason string) error {
	if err := checkWho(actor, reason); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `SELECT set_config('compliance.actor', $1, true), set_config('compliance.reason', $2, true)`, actor, reason)
	if err == nil {
		_, err = tx.ExecContext(ctx, `DELETE FROM compliance_findings WHERE key = $1`, key)
	}
	if err != nil {
		return errors.Join(fmt.Errorf("resolve %s: %w", key, err), tx.Rollback())
	}
	return tx.Commit()
}

func checkWho(actor, reason string) error {
	c := Change{Actor: actor, Reason: reason}
	if strings.TrimSpace(c.Actor) == "" || len(c.Actor) > maxActor || !printable(c.Actor) ||
		strings.TrimSpace(c.Reason) == "" || len(c.Reason) > maxReason || !printable(c.Reason) {
		return fmt.Errorf("%w: actor and reason must be printable, 1-%d and 1-%d characters", ErrInvalidChange, maxActor, maxReason)
	}
	return nil
}

// --- evidence ---------------------------------------------------------------

const maxDetail = 4000

// AppendEvidence stores collector records. A record the table would refuse
// (unknown verdict, oversized detail) fails the whole batch.
func (s *OpsStore) AppendEvidence(ctx context.Context, recs []crosswalk.Record) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, r := range recs {
		if r.Verdict != crosswalk.VerdictPass && r.Verdict != crosswalk.VerdictFail {
			return errors.Join(fmt.Errorf("%w: %s verdict %q", ErrInvalidChange, r.Source.Key(), r.Verdict), tx.Rollback())
		}
		if len(r.Detail) > maxDetail || r.ObservedAt.IsZero() {
			return errors.Join(fmt.Errorf("%w: %s detail too long or no time", ErrInvalidChange, r.Source.Key()), tx.Rollback())
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO compliance_evidence (source_key, verdict, observed_at, commit, detail)
			VALUES ($1, $2, $3, $4, $5)`, r.Source.Key(), string(r.Verdict), r.ObservedAt, r.Commit, r.Detail); err != nil {
			return errors.Join(fmt.Errorf("evidence %s: %w", r.Source.Key(), err), tx.Rollback())
		}
	}
	return tx.Commit()
}

// LatestEvidence returns the newest record per source.
func (s *OpsStore) LatestEvidence(ctx context.Context) ([]crosswalk.Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT ON (source_key) source_key, verdict, observed_at, commit, detail
		FROM compliance_evidence ORDER BY source_key, observed_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("evidence: %w", err)
	}
	defer rows.Close()
	var out []crosswalk.Record
	for rows.Next() {
		var key, verdict string
		var r crosswalk.Record
		if err := rows.Scan(&key, &verdict, &r.ObservedAt, &r.Commit, &r.Detail); err != nil {
			return nil, fmt.Errorf("evidence: %w", err)
		}
		kind, ref, ok := strings.Cut(key, ":")
		if !ok {
			return nil, fmt.Errorf("%w: evidence key %q", ErrInvalidChange, key)
		}
		r.Source = crosswalk.Source{Kind: kind, Ref: ref}
		r.Verdict = crosswalk.Verdict(verdict)
		r.ObservedAt = r.ObservedAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}
