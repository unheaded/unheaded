<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-E1-LIVE progress

## For Stevie

## Evidence

- Steps 1-5: Evening 0 started 2026-11-10, unattended (the plan exempts Evening 0). Hourly cap red then green: `go test -race ./cmd/alert-mailer/ ./cmd/alert-policy/` ok; `alert-policy --check` PASS. Two refusal rows for max_per_hour added to cmd/alert-policy/main_test.go and the metric help names capped-hour (not in the plan text, same change).
