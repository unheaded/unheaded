<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-E1-LIVE progress

## For Stevie

## Evidence

- Steps 1-5: Evening 0 started 2026-11-10, unattended (the plan exempts Evening 0). Hourly cap red then green: `go test -race ./cmd/alert-mailer/ ./cmd/alert-policy/` ok; `alert-policy --check` PASS. Two refusal rows for max_per_hour added to cmd/alert-policy/main_test.go and the metric help names capped-hour (not in the plan text, same change).
- Steps 6-7: alert-mailer holds past max_per_hour 3, logs every send, rule AlertMailVolumeHigh. promtool: SUCCESS 4 rules; tests/runbooks/test_alert_runbooks.py 50 passed; alertmanager.yml regenerated (ticket route for the new alert). promtool ran from the local v3.15.0 image with the network off.
