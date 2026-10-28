// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// metricsText renders one run for node_exporter's textfile collector
// (the restore drill's pattern): when it ran, what each class deleted,
// which tables are due for partitions, which classes failed. Success is
// reported only for a live run in which every class succeeded.
func metricsText(now time.Time, dryRun bool, results []result) string {
	var b strings.Builder
	failed := false
	for _, r := range results {
		if r.Err != "" {
			failed = true
		}
	}
	fmt.Fprintln(&b, "# HELP unheaded_well_retention_last_run_timestamp_seconds Last run of well-retention (dry or live).")
	fmt.Fprintln(&b, "# TYPE unheaded_well_retention_last_run_timestamp_seconds gauge")
	fmt.Fprintf(&b, "unheaded_well_retention_last_run_timestamp_seconds %d\n", now.Unix())
	fmt.Fprintln(&b, "# HELP unheaded_well_retention_dry_run 1 if the last run was a dry run.")
	fmt.Fprintln(&b, "# TYPE unheaded_well_retention_dry_run gauge")
	fmt.Fprintf(&b, "unheaded_well_retention_dry_run %d\n", b2i(dryRun))
	if !dryRun && !failed {
		fmt.Fprintln(&b, "# HELP unheaded_well_retention_last_success_timestamp_seconds Last live run in which every class succeeded.")
		fmt.Fprintln(&b, "# TYPE unheaded_well_retention_last_success_timestamp_seconds gauge")
		fmt.Fprintf(&b, "unheaded_well_retention_last_success_timestamp_seconds %d\n", now.Unix())
	}
	fmt.Fprintln(&b, "# HELP unheaded_well_retention_rows_deleted Rows the last run deleted, per class.")
	fmt.Fprintln(&b, "# TYPE unheaded_well_retention_rows_deleted gauge")
	for _, r := range results {
		if deletes(r.Action) {
			fmt.Fprintf(&b, "unheaded_well_retention_rows_deleted{class=%q} %d\n", r.Class, r.Deleted)
		}
	}
	fmt.Fprintln(&b, "# HELP unheaded_well_retention_partition_due 1 when ERDA 5.4's partition trigger has fired for the table.")
	fmt.Fprintln(&b, "# TYPE unheaded_well_retention_partition_due gauge")
	for _, r := range results {
		if r.Verdict != nil {
			fmt.Fprintf(&b, "unheaded_well_retention_partition_due{class=%q,table=%q} %d\n", r.Class, r.Table, b2i(r.Verdict.Due))
		}
	}
	fmt.Fprintln(&b, "# HELP unheaded_well_retention_class_failed 1 when the class failed in the last run.")
	fmt.Fprintln(&b, "# TYPE unheaded_well_retention_class_failed gauge")
	for _, r := range results {
		fmt.Fprintf(&b, "unheaded_well_retention_class_failed{class=%q} %d\n", r.Class, b2i(r.Err != ""))
	}
	return b.String()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// writeMetrics replaces dir/well_retention.prom atomically.
func writeMetrics(dir, text string) error {
	tmp, err := os.CreateTemp(dir, ".well_retention.prom.")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // #nosec G104 -- best effort: after the rename the temp name no longer exists
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close() // #nosec G104 -- the write error is the one returned
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close() // #nosec G104 -- the chmod error is the one returned
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// #nosec G703 -- dir is the operator's textfile-collector directory; the file name is a constant
	return os.Rename(tmp.Name(), filepath.Join(dir, "well_retention.prom"))
}
