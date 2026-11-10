<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-E1-LIVE progress

## For Stevie

## Evidence

- Steps 1-5: Evening 0 started 2026-11-10, unattended (the plan exempts Evening 0). Hourly cap red then green: `go test -race ./cmd/alert-mailer/ ./cmd/alert-policy/` ok; `alert-policy --check` PASS. Two refusal rows for max_per_hour added to cmd/alert-policy/main_test.go and the metric help names capped-hour (not in the plan text, same change).
- Steps 6-7: alert-mailer holds past max_per_hour 3, logs every send, rule AlertMailVolumeHigh. promtool: SUCCESS 4 rules; tests/runbooks/test_alert_runbooks.py 50 passed; alertmanager.yml regenerated (ticket route for the new alert). promtool ran from the local v3.15.0 image with the network off.
- Steps 8-11: Rulesets carry the 2026-11-03 scope, nft -c clean. Prometheus https per target, east wotan out of the SLO until Evening 10B. Noted, not changed: policy.yaml still describes the wotan SLO as "answers on both hosts"; with one target it measures west alone until 10B puts east back.
- Steps 12-15: psk-gate.sh tested, ceremony runbook rewritten, PG18 thaw uses https. `test-rotate-psk: all passed` (gate block: side script runs when its hash matches the pin, a changed one and ten near misses are refused, every command rotate-psk.sh sends is allowed); tests/runbooks 379 passed, 4 skipped; both runbooks dry-run SUCCESS; manifest rewritten, two lines, not sealed. Departures from the plan text, all for Stevie to read before Evening 10:
  - psk-gate.sh: the `PSK_GATE_SIDE_SHA` override is honoured only when `SSH_CONNECTION` is unset, so a variable sent over ssh cannot move the pin. A changed rotate-psk.sh needs a new side.sha256 on east or every rotation is refused.
  - Ceremony runbook: the format line itself refuses when buildkey.luks exists (`sudo test ! -e ... &&`); luksFormat on a rerun would destroy the seed. Plan Step 181's own line has no such guard. Steps reordered to the evening's order and three added: close-and-prove-second-token (Step 186), copy-encrypted-file-to-backup-host (Step 188, so a rotation refreshes the copy on east), and a manual verification (slot counts and the audit watch need root; /etc/audit is 0750). The shell verification checks only that the volume is closed and build.pubkey is committed signed: the public half sits inside the volume, so it cannot be compared once closed; sign-a-test-cask is the proof it matches. The "never in GitHub secrets" line and the FND-018 sentence were kept. rotate-quarterly opens the existing volume and never formats.
  - Two manual checks of psk-gate.sh on west wrote two `unheaded-psk-gate` lines to west's journal (a refusal and a `true`); nothing else ran.
