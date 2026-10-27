<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# runbook-draft-5-llama-server-swap

Write a draft runbook named `llama-server-swap`: swap the local model served by llama-server.

The model key is a parameter `MODEL` (no default). `scripts/switch-model.sh --dry-run <key>` checks
the model's sha256 pin and that it fits in memory without changing anything; `scripts/switch-model.sh
<key>` performs the swap (it stops the running llama-server, starts the new model and rolls back on
failure). Run the dry run first, then the swap. llama-server answers `GET http://127.0.0.1:8081/health`.
zhen-agentd hashes the model at start, so restart it afterwards (`sudo systemctl restart zhen-agentd`).
Never bypass the pin (`--no-verify`) and never kill processes by hand.
requires_sudo is true.
