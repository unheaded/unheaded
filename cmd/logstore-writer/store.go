// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
)

// One statement a batch: the rows travel as nine arrays, so the batch is
// one round trip and one implicit transaction. COPY would be a little
// faster and cannot skip a row that is already there; Vector sends a batch
// again after a timeout, and ON CONFLICT DO NOTHING makes that harmless.
// log_writer holds INSERT only, which is all this statement needs.
const insertSQL = `
INSERT INTO log_events (ts, host, source, unit, level, retention, message, entry, fields)
SELECT * FROM unnest($1::timestamptz[], $2::text[], $3::text[], $4::text[], $5::text[],
                     $6::text[], $7::text[], $8::text[], $9::text[])
ON CONFLICT DO NOTHING`

// wellStore is the Store on the Well, as the role log_writer.
type wellStore struct{ db *sql.DB }

func (s wellStore) Insert(ctx context.Context, rows []Row) (int64, error) {
	n := len(rows)
	ts := make([]string, n)
	cols := [8][]string{}
	for c := range cols {
		cols[c] = make([]string, n)
	}
	for i, r := range rows {
		ts[i] = r.TS.UTC().Format(time.RFC3339Nano)
		cols[0][i], cols[1][i], cols[2][i], cols[3][i] = r.Host, r.Source, r.Unit, r.Level
		cols[4][i], cols[5][i], cols[6][i], cols[7][i] = r.Retention, r.Message, r.Entry, r.Fields
	}
	res, err := s.db.ExecContext(ctx, insertSQL, pq.Array(ts),
		pq.Array(cols[0]), pq.Array(cols[1]), pq.Array(cols[2]), pq.Array(cols[3]),
		pq.Array(cols[4]), pq.Array(cols[5]), pq.Array(cols[6]), pq.Array(cols[7]))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s wellStore) Maintain(ctx context.Context) (int64, error) {
	var bytes int64
	err := s.db.QueryRowContext(ctx, `SELECT log_events_maintain()`).Scan(&bytes)
	return bytes, err
}
